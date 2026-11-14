//go:build linux

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	backupRename             = os.Rename
	backupRemovePublishedOld = os.RemoveAll
)

// The outer package contains only a small, fixed number of payloads, but a
// legitimate website can contain far more than 10,000 files. Keep a separate
// high ceiling for the inner files archive while retaining the total-byte
// limit enforced below.
const maxFilesArchiveEntries = 1_000_000

func secureOpenRegular(filePath string) (*os.File, os.FileInfo, error) {
	if err := rejectSymlinkPath(filepath.Dir(filePath)); err != nil {
		return nil, nil, err
	}
	fd, err := syscall.Open(filePath, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), filePath)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, nil, errors.New("could not wrap file descriptor")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	lstat, err := os.Lstat(filePath)
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || lstat.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, lstat) {
		_ = file.Close()
		return nil, nil, errors.New("path is not a stable regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || stat.Nlink != 1 {
		_ = file.Close()
		return nil, nil, errors.New("hard-linked files are not allowed")
	}
	return file, info, nil
}

func openPrivateExclusive(filePath string) (*os.File, error) {
	if err := rejectSymlinkPath(filepath.Dir(filePath)); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(
		filePath,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), filePath)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not wrap file descriptor")
	}
	return file, nil
}

func secureRemoveRegular(filePath string) error {
	file, opened, err := secureOpenRegular(filePath)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	current, err := os.Lstat(filePath)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return errors.New("backup changed before deletion")
	}
	return os.Remove(filePath)
}

func rejectSymlinkPath(candidate string) error {
	clean, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	clean = filepath.Clean(clean)
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(clean, current), string(filepath.Separator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component: %s", current)
		}
	}
	return nil
}

func secureMkdirAll(candidate string, mode os.FileMode) error {
	clean, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	clean = filepath.Clean(clean)
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(clean, current), string(filepath.Separator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(current, mode); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe directory component: %s", current)
		}
	}
	return nil
}

func atomicPublishFile(tmpPath, finalPath string) error {
	if err := rejectSymlinkPath(filepath.Dir(finalPath)); err != nil {
		return err
	}
	if err := os.Link(tmpPath, finalPath); err != nil {
		return err
	}
	if err := os.Remove(tmpPath); err != nil {
		_ = os.Remove(finalPath)
		return err
	}
	return nil
}

// openDirectoryNoFollow opens an absolute directory path with openat2
// RESOLVE_NO_SYMLINKS: a link at any component is refused, so a site's
// account cannot redirect the descriptor anywhere else.
func openDirectoryNoFollow(directory string) (int, error) {
	clean, err := filepath.Abs(directory)
	if err != nil {
		return -1, err
	}
	relative := strings.TrimPrefix(filepath.Clean(clean), "/")
	if relative == "" {
		relative = "."
	}
	rootFD, err := unix.Open("/", unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	defer unix.Close(rootFD)
	fd, err := unix.Openat2(rootFD, relative, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return -1, fmt.Errorf("open %s without following links: %w", directory, err)
	}
	return fd, nil
}

// readRegularBeneath reads relative (slash-separated) below directory as
// root: no link at any component, the file must be regular (opened
// non-blocking, so a FIFO cannot hang the caller) and at most maximum bytes.
func readRegularBeneath(directory, relative string, maximum int64) ([]byte, error) {
	dirFD, err := openDirectoryNoFollow(directory)
	if err != nil {
		return nil, err
	}
	defer unix.Close(dirFD)
	fd, err := unix.Openat2(dirFD, relative, &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), relative)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("could not wrap file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errors.New("not a bounded regular file")
	}
	content, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, errors.New("not a bounded regular file")
	}
	return content, nil
}

// directoryNamesAt lists a directory descriptor's entries, sorted.
func directoryNamesAt(dirFD int) ([]string, error) {
	dup, err := unix.Dup(dirFD)
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(dup), "directory")
	if dir == nil {
		_ = unix.Close(dup)
		return nil, errors.New("could not wrap directory descriptor")
	}
	defer dir.Close()
	if _, err := unix.Seek(dup, 0, 0); err != nil {
		return nil, err
	}
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// safeCreateFilesArchive archives a document root as root. The walk holds a
// descriptor for every directory and opens each entry relative to its
// parent without following a link, so a site's account that swaps a folder
// for a link mid-walk cannot make root read outside the document root.
func safeCreateFilesArchive(sourceDir, destination string) (returnErr error) {
	if err := rejectSymlinkPath(sourceDir); err != nil {
		return err
	}
	rootFD, err := openDirectoryNoFollow(sourceDir)
	if err != nil {
		return errors.New("document root is not a safe directory")
	}
	defer unix.Close(rootFD)
	out, err := openPrivateExclusive(destination)
	if err != nil {
		return err
	}
	defer func() {
		if returnErr != nil {
			_ = out.Close()
			_ = os.Remove(destination)
		}
	}()
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	if err := archiveDirectoryAt(tw, rootFD, ""); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func archiveDirectoryAt(tw *tar.Writer, dirFD int, prefix string) error {
	names, err := directoryNamesAt(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		archiveName := name
		if prefix != "" {
			archiveName = prefix + "/" + name
		}
		if !validPackageEntryName(archiveName) {
			return errors.New("unsafe document root path")
		}
		var st unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFLNK:
			return fmt.Errorf("symlink in document root: %s", archiveName)
		case unix.S_IFDIR:
			childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return fmt.Errorf("open %s: %w", archiveName, err)
			}
			err = archiveEntryAt(tw, childFD, archiveName, true)
			if err == nil {
				err = archiveDirectoryAt(tw, childFD, archiveName)
			}
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
		case unix.S_IFREG:
			fileFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return fmt.Errorf("open %s: %w", archiveName, err)
			}
			err = archiveEntryAt(tw, fileFD, archiveName, false)
			_ = unix.Close(fileFD)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type in document root: %s", archiveName)
		}
	}
	return nil
}

// archiveEntryAt writes one entry from an open descriptor (closed by the
// caller); a regular file's content is copied from the same descriptor.
func archiveEntryAt(tw *tar.Writer, fd int, archiveName string, directory bool) error {
	dup, err := unix.Dup(fd)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(dup), archiveName)
	if file == nil {
		_ = unix.Close(dup)
		return errors.New("could not wrap file descriptor")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if directory != info.IsDir() || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("unsupported file type in document root: %s", archiveName)
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !directory && (!ok || stat.Nlink != 1) {
		return errors.New("hard-linked files are not allowed")
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = archiveName
	header.Linkname = ""
	header.Uid, header.Gid = 0, 0
	header.Uname, header.Gname = "", ""
	header.Mode = int64(info.Mode().Perm())
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if directory {
		return nil
	}
	written, err := io.Copy(tw, file)
	if err != nil {
		return err
	}
	if written != info.Size() {
		return errors.New("file changed while archiving")
	}
	return nil
}

type extractedDirectoryMode struct {
	name string
	mode os.FileMode
}

// openStageDirectoryAt opens (and with create, makes) the directory
// components below base, one at a time and never through a link. The
// returned descriptor is the caller's to close.
func openStageDirectoryAt(base int, components []string, create bool) (int, error) {
	current, err := unix.Dup(base)
	if err != nil {
		return -1, err
	}
	for _, component := range components {
		if create {
			if err := unix.Mkdirat(current, component, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, err
			}
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(current)
		if err != nil {
			return -1, fmt.Errorf("restore staging folder %s: %w", component, err)
		}
		current = next
	}
	return current, nil
}

func safeExtractFilesArchive(source, destination string) error {
	if err := secureMkdirAll(destination, 0o700); err != nil {
		return err
	}
	stageFD, err := openDirectoryNoFollow(destination)
	if err != nil {
		return err
	}
	defer unix.Close(stageFD)
	return extractFilesArchiveAt(stageFD, source)
}

// extractOptions widen extraction for a site-home area
// (docs/SITE-BACKUP-CONTENTS-DESIGN.md): only the entries below prefix are
// taken (with prefix removed; the prefix folder's own entry gives the root's
// mode), and symbolic links are made again as links, never followed. The
// zero value is public_html's extraction, unchanged.
type extractOptions struct {
	prefix string
	links  bool
}

// maxLinkTarget bounds a link's target text (PATH_MAX).
const maxLinkTarget = 4095

// extractFilesArchiveAt extracts into the directory stageFD names. Every
// folder and file is made and set relative to a held descriptor, never by
// path, so a renamed or replaced staging folder cannot redirect root.
func extractFilesArchiveAt(stageFD int, source string) error {
	_, _, err := extractArchiveAt(stageFD, source, extractOptions{})
	return err
}

// extractArchiveAt is extractFilesArchiveAt with options. rootMode is the
// mode the archive gives the prefix folder (hasRoot false when it names none).
func extractArchiveAt(stageFD int, source string, opts extractOptions) (rootMode os.FileMode, hasRoot bool, returnErr error) {
	existing, err := directoryNamesAt(stageFD)
	if err != nil {
		return 0, false, err
	}
	if len(existing) != 0 {
		return 0, false, errors.New("restore staging directory is not empty")
	}
	file, _, err := secureOpenRegular(source)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return 0, false, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := make(map[string]bool)
	directoryModes := make([]extractedDirectoryMode, 0)
	entryCount := 0
	var totalSize int64
	for {
		header, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return 0, false, nextErr
		}
		entryCount++
		if entryCount > maxFilesArchiveEntries {
			return 0, false, errors.New("files archive entry limit exceeded")
		}
		name, rootEntry, err := cleanFilesArchiveName(header.Name, header.Typeflag)
		if err != nil {
			return 0, false, err
		}
		if rootEntry {
			continue
		}
		if opts.prefix != "" {
			if name == opts.prefix {
				if header.Typeflag != tar.TypeDir {
					return 0, false, errors.New("the area's own entry is not a folder")
				}
				rootMode, hasRoot = os.FileMode(header.Mode)&os.ModePerm, true
				continue
			}
			rest, under := strings.CutPrefix(name, opts.prefix+"/")
			if !under {
				// Another area of the same archive.
				continue
			}
			name = rest
		}
		if seen[name] {
			return 0, false, errors.New("duplicate files archive entry")
		}
		seen[name] = true
		components := strings.Split(name, "/")
		for _, component := range components {
			if component == "" || component == "." || component == ".." {
				return 0, false, errors.New("files archive escapes staging directory")
			}
		}
		mode := os.FileMode(header.Mode) & os.ModePerm
		switch header.Typeflag {
		case tar.TypeDir:
			if mode == 0 {
				mode = 0o700
			}
			dirFD, err := openStageDirectoryAt(stageFD, components, true)
			if err != nil {
				return 0, false, err
			}
			_ = unix.Close(dirFD)
			directoryModes = append(directoryModes, extractedDirectoryMode{name, mode})
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || totalSize > maxPackagePayloadBytes-header.Size {
				return 0, false, errors.New("files archive size limit exceeded")
			}
			totalSize += header.Size
			if mode == 0 {
				mode = 0o600
			}
			parentFD, err := openStageDirectoryAt(stageFD, components[:len(components)-1], true)
			if err != nil {
				return 0, false, err
			}
			err = writeStagedFileAt(parentFD, components[len(components)-1], tr, header.Size, mode)
			_ = unix.Close(parentFD)
			if err != nil {
				return 0, false, err
			}
		case tar.TypeSymlink:
			if !opts.links {
				return 0, false, errors.New("links and special files are not allowed in backups")
			}
			target := header.Linkname
			if target == "" || len(target) > maxLinkTarget || strings.ContainsRune(target, 0) {
				return 0, false, errors.New("invalid link in the backup")
			}
			parentFD, err := openStageDirectoryAt(stageFD, components[:len(components)-1], true)
			if err != nil {
				return 0, false, err
			}
			// symlinkat never follows the new name; it fails when the name
			// already exists.
			err = unix.Symlinkat(target, parentFD, components[len(components)-1])
			_ = unix.Close(parentFD)
			if err != nil {
				return 0, false, fmt.Errorf("restore link %s: %w", name, err)
			}
		default:
			return 0, false, errors.New("links and special files are not allowed in backups")
		}
	}
	sort.Slice(directoryModes, func(i, j int) bool {
		return strings.Count(directoryModes[i].name, "/") > strings.Count(directoryModes[j].name, "/")
	})
	for _, directory := range directoryModes {
		dirFD, err := openStageDirectoryAt(stageFD, strings.Split(directory.name, "/"), false)
		if err != nil {
			return 0, false, err
		}
		err = unix.Fchmod(dirFD, uint32(directory.mode))
		_ = unix.Close(dirFD)
		if err != nil {
			return 0, false, err
		}
	}
	return rootMode, hasRoot, nil
}

// writeStagedFileAt creates name below parentFD (O_EXCL, no link followed),
// copies size bytes and sets mode on the open file.
func writeStagedFileAt(parentFD int, name string, content io.Reader, size int64, mode os.FileMode) error {
	fd, err := unix.Openat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(fd), name)
	if out == nil {
		_ = unix.Close(fd)
		return errors.New("could not wrap file descriptor")
	}
	written, copyErr := io.Copy(out, content)
	syncErr := out.Sync()
	chmodErr := out.Chmod(mode)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	if chmodErr != nil {
		return chmodErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != size {
		return errors.New("truncated files archive entry")
	}
	return nil
}

func cleanFilesArchiveName(name string, typeflag byte) (string, bool, error) {
	if strings.Contains(name, `\`) || path.IsAbs(name) {
		return "", false, errors.New("unsafe files archive path")
	}
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	if typeflag == tar.TypeDir {
		name = strings.TrimRight(name, "/")
	}
	if name == "" || name == "." {
		if typeflag == tar.TypeDir {
			return "", true, nil
		}
		return "", false, errors.New("unsafe files archive root entry")
	}
	clean := path.Clean(name)
	if clean != name || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, errors.New("unsafe files archive path")
	}
	return clean, false, nil
}

type stagedFilesRestore struct {
	stage     string
	target    string
	old       string
	published bool
}

func prepareFilesRestore(archivePath, target string) (*stagedFilesRestore, error) {
	return prepareFilesRestoreEdited(archivePath, target, nil)
}

// prepareFilesRestoreEdited extracts archivePath into a new staging folder
// beside target. The site's home is the site account's own folder, so that
// account can rename the staging folder or put a link in its place: every
// step after mkdir works on a held descriptor of the folder root made
// (checked to be root's own and not a link), never by path. edit (optional)
// runs on that descriptor while the tree is still root's and 0700; then the
// tree is given target's owner and mode.
func prepareFilesRestoreEdited(archivePath, target string, edit func(stageFD int) error) (*stagedFilesRestore, error) {
	return prepareStagedRestore(archivePath, target, stageOptions{edit: edit})
}

// stageOptions: edit as in prepareFilesRestoreEdited; extract widens the
// extraction (a site-home area); archivedRootMode gives the staged root the
// mode the archive names instead of the target's (an area folder that did
// not exist and was just made).
type stageOptions struct {
	edit             func(stageFD int) error
	extract          extractOptions
	archivedRootMode bool
	// owner gives the staged tree this uid:gid instead of the target's (an
	// area belongs to the site home's account whoever owns its folder now).
	owner *[2]int
}

func prepareStagedRestore(archivePath, target string, opts stageOptions) (*stagedFilesRestore, error) {
	if err := rejectSymlinkPath(target); err != nil {
		return nil, err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("restore target is not a safe directory")
	}
	stat, ok := targetInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, errors.New("could not read restore target ownership")
	}
	parent := filepath.Dir(target)
	parentFD, err := openDirectoryNoFollow(parent)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parentFD)
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	name := ".restore-stage-" + hex.EncodeToString(random)
	if err := unix.Mkdirat(parentFD, name, 0o700); err != nil {
		return nil, err
	}
	result := &stagedFilesRestore{stage: filepath.Join(parent, name), target: target}
	stageFD, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		result.Cleanup()
		return nil, fmt.Errorf("open the restore staging folder: %w", err)
	}
	defer unix.Close(stageFD)
	var stageStat unix.Stat_t
	if err := unix.Fstat(stageFD, &stageStat); err != nil {
		result.Cleanup()
		return nil, err
	}
	if stageStat.Uid != uint32(os.Geteuid()) {
		// Someone else's folder took the name between mkdir and open.
		return nil, errors.New("the restore staging folder was replaced; nothing was restored")
	}
	if err := unix.Fchmod(stageFD, 0o700); err != nil {
		result.Cleanup()
		return nil, err
	}
	archivedMode, hasArchivedMode, err := extractArchiveAt(stageFD, archivePath, opts.extract)
	if err != nil {
		result.Cleanup()
		return nil, err
	}
	if opts.edit != nil {
		if err := opts.edit(stageFD); err != nil {
			result.Cleanup()
			return nil, err
		}
	}
	uid, gid := int(stat.Uid), int(stat.Gid)
	if opts.owner != nil {
		uid, gid = opts.owner[0], opts.owner[1]
	}
	if err := chownTreeLinksAt(stageFD, uid, gid, opts.extract.links); err != nil {
		result.Cleanup()
		return nil, err
	}
	// A site's folders carry the setgid bit so new files belong to the web
	// server's group. Folders only: on a file that bit is a privilege.
	setgid := targetInfo.Mode()&os.ModeSetgid != 0
	if setgid {
		if err := setgidDirectoriesAt(stageFD); err != nil {
			result.Cleanup()
			return nil, err
		}
	}
	rootMode := uint32(targetInfo.Mode().Perm())
	if opts.archivedRootMode && hasArchivedMode {
		rootMode = uint32(archivedMode)
	}
	if setgid {
		rootMode |= unix.S_ISGID
	}
	if err := unix.Fchmod(stageFD, rootMode); err != nil {
		result.Cleanup()
		return nil, err
	}
	return result, nil
}

// setgidDirectoriesAt adds the setgid bit to every directory below dirFD
// through held descriptors, never following a link. Files are not touched.
func setgidDirectoriesAt(dirFD int) error {
	names, err := directoryNamesAt(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if st.Mode&unix.S_IFMT != unix.S_IFDIR {
			continue
		}
		childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		err = unix.Fchmod(childFD, (st.Mode&0o777)|unix.S_ISGID)
		if err == nil {
			err = setgidDirectoriesAt(childFD)
		}
		_ = unix.Close(childFD)
		if err != nil {
			return err
		}
	}
	return nil
}

// chownTreeAt gives the tree below dirFD (and dirFD) uid:gid through held
// descriptors, never following a link.
func chownTreeAt(dirFD int, uid, gid int) error {
	return chownTreeLinksAt(dirFD, uid, gid, false)
}

// chownTreeLinksAt is chownTreeAt that, with links, also gives each link
// (not what it names) uid:gid; without links a link is an error.
func chownTreeLinksAt(dirFD int, uid, gid int, links bool) error {
	if err := unix.Fchown(dirFD, uid, gid); err != nil {
		return err
	}
	names, err := directoryNamesAt(dirFD)
	if err != nil {
		return err
	}
	for _, name := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			err = chownTreeLinksAt(childFD, uid, gid, links)
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
		case unix.S_IFREG:
			if err := unix.Fchownat(dirFD, name, uid, gid, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
		case unix.S_IFLNK:
			if !links {
				return errors.New("symlink appeared in restore staging")
			}
			if err := unix.Fchownat(dirFD, name, uid, gid, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return err
			}
		default:
			return errors.New("symlink appeared in restore staging")
		}
	}
	return nil
}

func (s *stagedFilesRestore) Publish() error {
	if s == nil || s.published {
		return errors.New("invalid staged restore state")
	}
	parent := filepath.Dir(s.target)
	placeholder, err := os.MkdirTemp(parent, ".restore-old-")
	if err != nil {
		return err
	}
	if err := os.Remove(placeholder); err != nil {
		return err
	}
	s.old = placeholder
	if err := backupRename(s.target, s.old); err != nil {
		return err
	}
	if err := backupRename(s.stage, s.target); err != nil {
		rollbackErr := backupRename(s.old, s.target)
		if rollbackErr != nil {
			return fmt.Errorf("publish failed: %v; target rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	s.published = true
	if err := syncDirectory(parent); err != nil {
		rollbackErr := s.Rollback()
		if rollbackErr != nil {
			return fmt.Errorf("publish sync failed: %v; target rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	return nil
}

func (s *stagedFilesRestore) Commit() error {
	if s == nil || !s.published {
		return errors.New("staged restore was not published")
	}
	if err := backupRemovePublishedOld(s.old); err != nil {
		return err
	}
	s.old = ""
	return syncDirectory(filepath.Dir(s.target))
}

func (s *stagedFilesRestore) Rollback() error {
	if s == nil || !s.published || s.old == "" {
		return nil
	}
	parent := filepath.Dir(s.target)
	failed, err := os.MkdirTemp(parent, ".restore-failed-")
	if err != nil {
		return err
	}
	if err := os.Remove(failed); err != nil {
		return err
	}
	if err := backupRename(s.target, failed); err != nil {
		return err
	}
	if err := backupRename(s.old, s.target); err != nil {
		_ = backupRename(failed, s.target)
		return err
	}
	s.published = false
	s.old = ""
	_ = os.RemoveAll(failed)
	return syncDirectory(parent)
}

func (s *stagedFilesRestore) Cleanup() {
	if s == nil {
		return
	}
	if !s.published && s.stage != "" {
		_ = os.RemoveAll(s.stage)
	}
}
