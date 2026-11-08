package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// The owner's files around a purged package (alpha.130 lab, Ubuntu 24.04).
// Removing Postfix ran the distribution's purge script, which deletes
// /etc/systemd/system/postfix.service.d/ whole: an owner's own drop-in in it
// was gone, and the answer did not name it. Owner files are never lost.
//
// Before a purge (after the simulation accepted it and the units stopped),
// the Agent looks where a purge reaches:
//
//   - the systemd drop-in folders of the component's units;
//   - the component's own folders under /etc (those that hold a
//     configuration file of a removed package; shared folders such as
//     /etc/default or /etc/systemd are not walked).
//
// A file there that no removed package owns and that is not CelikPanel's
// is the owner's: it is copied aside, and after the purge it is put back
// where it was if the purge deleted it. A package's configuration file the
// owner changed (its checksum differs from the package's) is purged as
// purge means, but a copy is kept first. Copies live under
// /var/backups/celikpanel/kept/<component>-<time>/ (root only); a copy of a
// file that is back in place is deleted, so only what the owner may still
// need stays, and the removal's answer names every file.
//
// Arındırmadan önce sahibin dosyaları kenara kopyalanır, arındırma sildiyse
// yerine konur; sahibin değiştirdiği paket yapılandırması kopyalanarak
// saklanır. Yanıt hepsini adıyla söyler.

var (
	dpkgInfoDir   = "/var/lib/dpkg/info"
	keptFilesRoot = "/var/backups/celikpanel/kept"
	systemdEtcDir = "/etc/systemd/system"
	etcDir        = "/etc"
)

// ownerFilesWalkCap bounds how many entries one removal looks at.
const ownerFilesWalkCap = 5000

// sharedEtcDirs are /etc folders many packages share: a purge removes only
// its own files there, which the conffile check covers.
var sharedEtcDirs = map[string]bool{
	"systemd": true, "default": true, "init.d": true, "logrotate.d": true, "cron.d": true,
	"cron.daily": true, "cron.hourly": true, "cron.weekly": true, "cron.monthly": true,
	"apparmor.d": true, "rsyslog.d": true, "pam.d": true, "security": true, "sudoers.d": true,
	"profile.d": true, "sysctl.d": true, "modprobe.d": true, "network": true, "apt": true,
	"dpkg": true, "ufw": true, "ssl": true, "alternatives": true, "init": true, "xdg": true,
	"bash_completion.d": true, "ld.so.conf.d": true, "tmpfiles.d": true, "insserv.conf.d": true,
	"ppp": true, "X11": true, "fonts": true, "ca-certificates": true, "update-motd.d": true,
	"rc0.d": true, "rc1.d": true, "rc2.d": true, "rc3.d": true, "rc4.d": true, "rc5.d": true,
	"rc6.d": true, "rcS.d": true, "celikpanel": true,
}

// keptOwnerFile is one file kept around a purge.
type keptOwnerFile struct {
	path string
	copy string
	// restore: the owner's own file, put back when the purge deleted it;
	// otherwise a changed package file, kept as a copy only.
	restore bool
	mode    fs.FileMode
	uid     int
	gid     int
}

// ownerFilesKeep is what one removal kept aside.
type ownerFilesKeep struct {
	root  string
	files []keptOwnerFile
}

// dpkgOwnedFiles reads the paths the packages own (dpkg's .list files).
func dpkgOwnedFiles(packages []string) map[string]bool {
	owned := map[string]bool{}
	for _, pkg := range packages {
		name := packageBaseName(pkg)
		matches, _ := filepath.Glob(filepath.Join(dpkgInfoDir, name+".list"))
		archMatches, _ := filepath.Glob(filepath.Join(dpkgInfoDir, name+":*.list"))
		for _, list := range append(matches, archMatches...) {
			file, err := os.Open(list)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(io.LimitReader(file, 32<<20))
			scanner.Buffer(make([]byte, 64<<10), 1<<20)
			for scanner.Scan() {
				if line := strings.TrimSpace(scanner.Text()); line != "" {
					owned[filepath.Clean(line)] = true
				}
			}
			file.Close()
		}
	}
	return owned
}

// dpkgConffiles reads the packages' configuration files with dpkg's
// checksums from dpkg's status file.
func dpkgConffiles(r io.Reader, packages []string) map[string]string {
	wanted := map[string]bool{}
	for _, pkg := range packages {
		wanted[packageBaseName(pkg)] = true
	}
	conffiles := map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	name, inConffiles := "", false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			name, inConffiles = "", false
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
			inConffiles = false
		case strings.HasPrefix(line, "Conffiles:"):
			inConffiles = true
		case inConffiles && strings.HasPrefix(line, " "):
			if !wanted[name] {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.HasPrefix(fields[0], "/") {
				conffiles[filepath.Clean(fields[0])] = fields[1]
			}
		default:
			inConffiles = false
		}
	}
	return conffiles
}

// ownerWatchedDirs are the folders a purge of the packages may reach.
func ownerWatchedDirs(units []string, conffiles map[string]string) []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, unit := range units {
		unit = strings.TrimSuffix(unit, ".service")
		if unit == "" || strings.ContainsAny(unit, "/") {
			continue
		}
		add(filepath.Join(systemdEtcDir, unit+".service.d"))
		if at := strings.IndexByte(unit, '@'); at > 0 {
			add(filepath.Join(systemdEtcDir, unit[:at+1]+".service.d"))
		}
	}
	for path := range conffiles {
		rel, err := filepath.Rel(etcDir, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		if top == rel || sharedEtcDirs[top] {
			continue // a file directly in /etc, or a shared folder
		}
		add(filepath.Join(etcDir, top))
	}
	sort.Strings(dirs)
	return dirs
}

// isCelikPanelFile: CelikPanel's own files carry its name; the component
// cleanup takes them.
func isCelikPanelFile(path string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(path)), "celikpanel")
}

func fileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := md5.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// keepOwnerFiles copies the owner's files and the changed package files
// aside before a purge. component names the copies' folder.
func keepOwnerFiles(component string, packages, units []string, status io.Reader) (*ownerFilesKeep, error) {
	owned := dpkgOwnedFiles(packages)
	conffiles := dpkgConffiles(status, packages)
	keep := &ownerFilesKeep{root: filepath.Join(keptFilesRoot, fmt.Sprintf("%s-%s", component, pgLeftoverNow().UTC().Format("20060102T150405Z")))}
	var candidates []keptOwnerFile
	consider := func(path string, restore bool) {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		file := keptOwnerFile{path: path, restore: restore, mode: info.Mode().Perm()}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			file.uid, file.gid = int(stat.Uid), int(stat.Gid)
		}
		candidates = append(candidates, file)
	}
	for path, sum := range conffiles {
		if isCelikPanelFile(path) {
			continue
		}
		if current, err := fileMD5(path); err == nil && current != sum {
			consider(path, false)
		}
	}
	entries := 0
	for _, dir := range ownerWatchedDirs(units, conffiles) {
		if !realDirectory(dir) {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			entries++
			if entries > ownerFilesWalkCap {
				return fs.SkipAll
			}
			if !entry.Type().IsRegular() || owned[filepath.Clean(path)] || isCelikPanelFile(path) {
				return nil
			}
			if _, isConffile := conffiles[filepath.Clean(path)]; isConffile {
				return nil
			}
			consider(path, true)
			return nil
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	for _, file := range candidates {
		file.copy = filepath.Join(keep.root, strings.TrimPrefix(file.path, "/"))
		if err := os.MkdirAll(filepath.Dir(file.copy), 0o700); err != nil {
			keep.discard()
			return nil, err
		}
		if err := copyFileMode(file.path, file.copy, 0o600); err != nil {
			keep.discard()
			return nil, fmt.Errorf("%s could not be kept aside: %v", file.path, err)
		}
		keep.files = append(keep.files, file)
	}
	if len(keep.files) == 0 {
		keep.discard()
	}
	return keep, nil
}

func copyFileMode(from, to string, mode fs.FileMode) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}

// discard removes the copies (a refused removal changed nothing).
func (k *ownerFilesKeep) discard() {
	if k == nil || k.root == "" {
		return
	}
	_ = os.RemoveAll(k.root)
	removeEmptyDir(keptFilesRoot)
}

// restore puts back every owner file the purge deleted, deletes the copies
// of the files that are in place, and says what it did.
func (k *ownerFilesKeep) restore() (restored, inPlace, copies []string, notes []string) {
	if k == nil || len(k.files) == 0 {
		return nil, nil, nil, nil
	}
	for _, file := range k.files {
		if !file.restore {
			copies = append(copies, file.path)
			continue
		}
		if _, err := os.Lstat(file.path); err == nil {
			inPlace = append(inPlace, file.path)
			_ = os.Remove(file.copy)
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			notes = append(notes, file.path+" could not be checked; its copy is in "+file.copy)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			notes = append(notes, file.path+" could not be put back ("+err.Error()+"); its copy is in "+file.copy)
			continue
		}
		if err := copyFileMode(file.copy, file.path, file.mode); err != nil {
			notes = append(notes, file.path+" could not be put back ("+err.Error()+"); its copy is in "+file.copy)
			continue
		}
		_ = os.Chown(file.path, file.uid, file.gid)
		_ = os.Chmod(file.path, file.mode)
		restored = append(restored, file.path)
		_ = os.Remove(file.copy)
	}
	// Folders left empty by deleted copies go.
	pruneEmptyDirs(k.root)
	removeEmptyDir(keptFilesRoot)
	return restored, inPlace, copies, notes
}

// pruneEmptyDirs removes empty folders under (and including) root.
func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		removeEmptyDir(dirs[i])
	}
}

// keptOwnerFilesDetail is the removal answer's sentence about them.
func keptOwnerFilesDetail(root string, restored, inPlace, copies, notes []string) string {
	var parts []string
	if len(restored) > 0 {
		parts = append(parts, "your own files the package's purge deleted were put back: "+strings.Join(restored, ", "))
	}
	if len(inPlace) > 0 {
		parts = append(parts, "your own files were left in place: "+strings.Join(inPlace, ", "))
	}
	if len(copies) > 0 {
		parts = append(parts, "the package's configuration files you had changed were purged; copies are kept in "+root+": "+strings.Join(copies, ", "))
	}
	parts = append(parts, notes...)
	return strings.Join(parts, "; ")
}
