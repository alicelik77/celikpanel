//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// The site-home walk (docs/SITE-BACKUP-CONTENTS-DESIGN.md). One walk both
// measures (tw nil: the backup screen's "what a backup holds", before it
// starts) and archives, with the same rules, so the sizes shown are the sizes
// archived. Every directory is held by a descriptor and every entry is opened
// relative to its parent without following a link: a site's account that
// swaps a folder for a link mid-walk cannot make root read elsewhere.
//
// Site ev dizini yürüyüşü: ölçüm ve arşiv aynı kurallarla yapılır; hiçbir
// bağlantı izlenmez.

// openSiteHomeForBackup opens the site home without following a link and
// reads its owner (the site's account). ok is false when it does not exist.
func openSiteHomeForBackup(home string) (fd int, uid, gid uint32, setgid bool, ok bool, err error) {
	if _, statErr := os.Lstat(home); errors.Is(statErr, os.ErrNotExist) {
		return -1, 0, 0, false, false, nil
	}
	fd, err = openDirectoryNoFollow(home)
	if err != nil {
		return -1, 0, 0, false, false, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		_ = unix.Close(fd)
		return -1, 0, 0, false, false, err
	}
	return fd, st.Uid, st.Gid, st.Mode&unix.S_ISGID != 0, true, nil
}

// detectAreas finds the areas of a site home: app/ and each folder of apps/,
// when they are real folders of the site's account. Anything else in their
// place is named as left out.
func (w *homeWalk) detectAreas(homeFD int) ([]string, error) {
	areas := make([]string, 0, 2)
	var st unix.Stat_t
	err := unix.Fstatat(homeFD, backupspec.AreaApp, &st, unix.AT_SYMLINK_NOFOLLOW)
	switch {
	case errors.Is(err, unix.ENOENT):
	case err != nil:
		return nil, err
	case st.Mode&unix.S_IFMT != unix.S_IFDIR:
		w.left.add(backupspec.LeftOut{Path: backupspec.AreaApp, Reason: backupspec.LeftOutNotAFolder})
	case st.Uid != w.siteUID:
		w.left.add(backupspec.LeftOut{Path: backupspec.AreaApp, Reason: backupspec.LeftOutNotSiteOwned})
	default:
		areas = append(areas, backupspec.AreaApp)
	}
	err = unix.Fstatat(homeFD, homeAreaApps, &st, unix.AT_SYMLINK_NOFOLLOW)
	switch {
	case errors.Is(err, unix.ENOENT):
		return areas, nil
	case err != nil:
		return nil, err
	case st.Mode&unix.S_IFMT != unix.S_IFDIR:
		w.left.add(backupspec.LeftOut{Path: homeAreaApps, Reason: backupspec.LeftOutNotAFolder})
		return areas, nil
	case st.Uid != w.siteUID:
		w.left.add(backupspec.LeftOut{Path: homeAreaApps, Reason: backupspec.LeftOutNotSiteOwned})
		return areas, nil
	}
	appsFD, err := unix.Openat(homeFD, homeAreaApps, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", homeAreaApps, err)
	}
	defer unix.Close(appsFD)
	names, err := directoryNamesAt(appsFD)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		entry := homeAreaApps + "/" + name
		if err := unix.Fstatat(appsFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return nil, err
		}
		switch {
		case st.Mode&unix.S_IFMT != unix.S_IFDIR || !validAreaName(entry):
			w.left.add(backupspec.LeftOut{Path: entry, Reason: backupspec.LeftOutNotAFolder, Bytes: regularSize(st)})
		case st.Uid != w.siteUID:
			w.left.add(backupspec.LeftOut{Path: entry, Reason: backupspec.LeftOutNotSiteOwned})
		default:
			areas = append(areas, entry)
		}
	}
	sort.Strings(areas)
	return areas, nil
}

func regularSize(st unix.Stat_t) int64 {
	if st.Mode&unix.S_IFMT == unix.S_IFREG {
		return st.Size
	}
	return 0
}

// tick counts an entry against the limit a restore enforces, and the
// measurement's time limit.
func (w *homeWalk) tick() error {
	w.entries++
	if w.entries > maxFilesArchiveEntries {
		return fmt.Errorf("the site's program folders hold more than %d files and folders, more than a restore accepts", maxFilesArchiveEntries)
	}
	if !w.deadline.IsZero() && w.entries%256 == 0 && time.Now().After(w.deadline) {
		return errHomeWalkTimedOut
	}
	return nil
}

// walkArea walks one area from the site home.
func (w *homeWalk) walkArea(homeFD int, area string) error {
	areaFD, err := openStageDirectoryAt(homeFD, strings.Split(area, "/"), false)
	if err != nil {
		return fmt.Errorf("open %s: %w", area, err)
	}
	defer unix.Close(areaFD)
	w.areas = append(w.areas, backupspec.Area{Name: area})
	w.current = &w.areas[len(w.areas)-1]
	if err := w.writeDirectory(areaFD, area); err != nil {
		return err
	}
	return w.walk(areaFD, area, area, true)
}

func (w *homeWalk) walk(dirFD int, area, prefix string, top bool) error {
	names, err := directoryNamesAt(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		rel := prefix + "/" + name
		if !validPackageEntryName(rel) {
			return fmt.Errorf("unsafe name in the site's folder: %q", rel)
		}
		if err := w.tick(); err != nil {
			return err
		}
		var st unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue // removed while the walk ran
			}
			return err
		}
		kind := st.Mode & unix.S_IFMT
		if top {
			if reason := areaExclusion(area, name, kind == unix.S_IFDIR); reason != "" {
				bytes, err := w.measureExcluded(dirFD, name)
				if err != nil {
					return err
				}
				w.left.add(backupspec.LeftOut{Path: rel, Reason: reason, Bytes: bytes})
				continue
			}
		}
		if st.Uid != w.siteUID {
			w.left.add(backupspec.LeftOut{Path: rel, Reason: backupspec.LeftOutNotSiteOwned, Bytes: regularSize(st)})
			continue
		}
		switch kind {
		case unix.S_IFDIR:
			childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return fmt.Errorf("open %s: %w", rel, err)
			}
			err = w.writeDirectory(childFD, rel)
			if err == nil {
				err = w.walk(childFD, area, rel, false)
			}
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
		case unix.S_IFREG:
			if err := w.file(dirFD, name, rel, st); err != nil {
				return err
			}
		case unix.S_IFLNK:
			if err := w.link(dirFD, name, rel); err != nil {
				return err
			}
		default:
			w.left.add(backupspec.LeftOut{Path: rel, Reason: backupspec.LeftOutSpecial})
		}
	}
	return nil
}

func (w *homeWalk) file(dirFD int, name, rel string, st unix.Stat_t) error {
	if w.tw == nil {
		w.current.Bytes += st.Size
		w.current.Files++
		return nil
	}
	fd, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", rel, err)
	}
	file := os.NewFile(uintptr(fd), rel)
	if file == nil {
		_ = unix.Close(fd)
		return errors.New("could not wrap file descriptor")
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return err
	}
	// The descriptor decides, not the name read before: a file swapped
	// for another account's between the two reads is left out.
	if opened.Mode&unix.S_IFMT != unix.S_IFREG {
		w.left.add(backupspec.LeftOut{Path: rel, Reason: backupspec.LeftOutSpecial})
		return nil
	}
	if opened.Uid != w.siteUID {
		w.left.add(backupspec.LeftOut{Path: rel, Reason: backupspec.LeftOutNotSiteOwned, Bytes: opened.Size})
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	normalizeHomeHeader(header, rel, info.Mode().Perm())
	if err := w.tw.WriteHeader(header); err != nil {
		return err
	}
	// Exactly the size the header names: a file the running program
	// appends to is taken as it was when it was opened.
	written, err := io.CopyN(w.tw, file, info.Size())
	if err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%s became shorter while it was archived; make the backup again", rel)
		}
		return err
	}
	w.current.Bytes += written
	w.current.Files++
	return nil
}

func (w *homeWalk) link(dirFD int, name, rel string) error {
	buffer := make([]byte, maxLinkTarget+1)
	n, err := unix.Readlinkat(dirFD, name, buffer)
	if err != nil {
		return fmt.Errorf("read link %s: %w", rel, err)
	}
	target := string(buffer[:n])
	if n == 0 || n > maxLinkTarget || strings.ContainsRune(target, 0) {
		w.left.add(backupspec.LeftOut{Path: rel, Reason: backupspec.LeftOutSpecial})
		return nil
	}
	if w.tw == nil {
		return nil
	}
	return w.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeSymlink, Name: rel, Linkname: target, Mode: 0o777,
		ModTime: backupNow().UTC(), Format: tar.FormatPAX,
	})
}

func (w *homeWalk) writeDirectory(fd int, rel string) error {
	if w.tw == nil {
		return nil
	}
	dup, err := unix.Dup(fd)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(dup), rel)
	if file == nil {
		_ = unix.Close(dup)
		return errors.New("could not wrap directory descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	normalizeHomeHeader(header, rel, info.Mode().Perm())
	return w.tw.WriteHeader(header)
}

// normalizeHomeHeader keeps a name, the permission bits (never setuid,
// setgid or sticky) and the time; owners are not stored (a restore gives
// everything to the site's account).
func normalizeHomeHeader(header *tar.Header, name string, perm os.FileMode) {
	header.Name = name
	header.Linkname = ""
	header.Uid, header.Gid = 0, 0
	header.Uname, header.Gname = "", ""
	header.Mode = int64(perm)
	header.Format = tar.FormatPAX
}

// measureExcluded sums the files under a left-out entry, never following a
// link, within the walk's time limit.
func (w *homeWalk) measureExcluded(dirFD int, name string) (int64, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return regularSize(st), nil
	}
	childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return 0, err
	}
	defer unix.Close(childFD)
	names, err := directoryNamesAt(childFD)
	if err != nil {
		return 0, err
	}
	var total int64
	for i, child := range names {
		if !w.deadline.IsZero() && i%256 == 255 && time.Now().After(w.deadline) {
			return total, errHomeWalkTimedOut
		}
		size, err := w.measureExcluded(childFD, child)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return total, err
		}
		total += size
	}
	return total, nil
}

// safeCreateHomeArchive archives a site home's areas into destination. wrote
// is false (and nothing is created) when the site has no area and nothing to
// name: the package then stays version 2.
func safeCreateHomeArchive(home, destination string) (walk homeWalk, wrote bool, returnErr error) {
	homeFD, uid, _, _, ok, err := openSiteHomeForBackup(home)
	if err != nil {
		return homeWalk{}, false, fmt.Errorf("the site's folder: %w", err)
	}
	if !ok {
		return homeWalk{}, false, nil
	}
	defer unix.Close(homeFD)
	w := &homeWalk{siteUID: uid}
	areas, err := w.detectAreas(homeFD)
	if err != nil {
		return homeWalk{}, false, err
	}
	if len(areas) == 0 && len(w.left.items) == 0 {
		return homeWalk{}, false, nil
	}
	out, err := openPrivateExclusive(destination)
	if err != nil {
		return homeWalk{}, false, err
	}
	defer func() {
		if returnErr != nil {
			_ = out.Close()
			_ = os.Remove(destination)
		}
	}()
	gz := gzip.NewWriter(out)
	w.tw = tar.NewWriter(gz)
	for _, area := range areas {
		if err := w.walkArea(homeFD, area); err != nil {
			_ = w.tw.Close()
			_ = gz.Close()
			return homeWalk{}, false, err
		}
	}
	if err := w.tw.Close(); err != nil {
		return homeWalk{}, false, err
	}
	if err := gz.Close(); err != nil {
		return homeWalk{}, false, err
	}
	if err := out.Sync(); err != nil {
		return homeWalk{}, false, err
	}
	if err := out.Close(); err != nil {
		return homeWalk{}, false, err
	}
	w.tw, w.current = nil, nil
	return *w, true, nil
}

// measureSiteBackup is what a files backup of the site would hold now:
// public_html's files and each area, with the archive's rules, within
// timeLimit (Complete false past it: the sizes are "at least").
func measureSiteBackup(docroot, home string, timeLimit time.Duration) backupspec.ContentsResponse {
	resp := backupspec.ContentsResponse{Areas: []backupspec.Area{}, LeftOut: []backupspec.LeftOut{}, Complete: true}
	deadline := time.Now().Add(timeLimit)
	site := &homeWalk{deadline: deadline}
	if bytes, files, err := site.measureTree(docroot); err != nil {
		if !errors.Is(err, errHomeWalkTimedOut) {
			resp.Error = "the site's files could not be measured: " + err.Error()
			return resp
		}
		resp.Complete = false
		resp.SiteBytes, resp.SiteFiles = bytes, files
	} else {
		resp.SiteBytes, resp.SiteFiles = bytes, files
	}
	homeFD, uid, _, _, ok, err := openSiteHomeForBackup(home)
	if err != nil {
		resp.Error = "the site's folder could not be read: " + err.Error()
		return resp
	}
	if ok {
		defer unix.Close(homeFD)
		w := &homeWalk{siteUID: uid, deadline: deadline}
		areas, err := w.detectAreas(homeFD)
		if err != nil {
			resp.Error = "the site's folder could not be read: " + err.Error()
			return resp
		}
		for _, area := range areas {
			if !resp.Complete {
				break
			}
			if err := w.walkArea(homeFD, area); err != nil {
				if !errors.Is(err, errHomeWalkTimedOut) {
					resp.Error = "the site's program folders could not be measured: " + err.Error()
					return resp
				}
				resp.Complete = false
			}
		}
		resp.Areas = append(resp.Areas, w.areas...)
		resp.LeftOut = append(resp.LeftOut, w.left.items...)
		resp.LeftOutMore, resp.LeftOutBytes = w.left.more, w.left.bytes
	}
	resp.BackupFreeBytes = backupFreeBytes()
	return resp
}

// measureTree sums public_html's regular files without following a link.
func (w *homeWalk) measureTree(root string) (int64, int64, error) {
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	fd, err := openDirectoryNoFollow(root)
	if err != nil {
		return 0, 0, err
	}
	defer unix.Close(fd)
	var bytes, files int64
	var walk func(int) error
	walk = func(dirFD int) error {
		names, err := directoryNamesAt(dirFD)
		if err != nil {
			return err
		}
		for _, name := range names {
			w.entries++
			if w.entries%256 == 0 && time.Now().After(w.deadline) {
				return errHomeWalkTimedOut
			}
			var st unix.Stat_t
			if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				if errors.Is(err, unix.ENOENT) {
					continue
				}
				return err
			}
			switch st.Mode & unix.S_IFMT {
			case unix.S_IFREG:
				bytes += st.Size
				files++
			case unix.S_IFDIR:
				child, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
				if err != nil {
					return err
				}
				err = walk(child)
				_ = unix.Close(child)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	err = walk(fd)
	return bytes, files, err
}

// backupFreeBytes is the free space for root on the backup disk (the nearest
// existing folder of the backup directory), or -1.
func backupFreeBytes() int64 {
	dir := backupBaseDir
	for {
		var fs unix.Statfs_t
		if err := unix.Statfs(dir, &fs); err == nil {
			return int64(fs.Bavail) * int64(fs.Bsize)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return -1
		}
		dir = parent
	}
}

// prepareAreaRestore stages one area of a home payload beside the area's
// folder in the site home, as public_html is staged: owner the site's
// account, modes from the archive, links made again and never followed. A
// missing area folder is made first (the site's account's, 0700 until the
// staged one with the archived mode replaces it). made reports that, so a
// restore that stops before publishing can remove the empty folder again.
func prepareAreaRestore(archivePath, home, area string) (staged *stagedFilesRestore, made bool, err error) {
	if !restorableArea(area) {
		return nil, false, fmt.Errorf("area %s is not restored by this release", area)
	}
	homeFD, uid, gid, _, ok, err := openSiteHomeForBackup(home)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, errors.New("the site's folder does not exist; nothing was restored")
	}
	defer unix.Close(homeFD)
	var st unix.Stat_t
	statErr := unix.Fstatat(homeFD, area, &st, unix.AT_SYMLINK_NOFOLLOW)
	switch {
	case errors.Is(statErr, unix.ENOENT):
		if err := unix.Mkdirat(homeFD, area, 0o700); err != nil {
			return nil, false, err
		}
		made = true
		if err := unix.Fchownat(homeFD, area, int(uid), int(gid), unix.AT_SYMLINK_NOFOLLOW); err != nil {
			removeMadeArea(home, area)
			return nil, false, err
		}
	case statErr != nil:
		return nil, false, statErr
	case st.Mode&unix.S_IFMT != unix.S_IFDIR:
		return nil, false, fmt.Errorf("%s in the site's folder is not a folder; nothing was restored", area)
	}
	staged, err = prepareStagedRestore(archivePath, filepath.Join(home, filepath.FromSlash(area)), stageOptions{
		extract:          extractOptions{prefix: area, links: true},
		archivedRootMode: made,
		owner:            &[2]int{int(uid), int(gid)},
	})
	if err != nil {
		if made {
			removeMadeArea(home, area)
		}
		return nil, false, err
	}
	return staged, made, nil
}

// removeMadeArea removes an area folder this restore made, only while it is
// still the empty folder (never anything the site put there meanwhile).
func removeMadeArea(home, area string) {
	homeFD, err := openDirectoryNoFollow(home)
	if err != nil {
		return
	}
	defer unix.Close(homeFD)
	_ = unix.Unlinkat(homeFD, area, unix.AT_REMOVEDIR)
}
