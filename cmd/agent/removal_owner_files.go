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
//
// First lab finding: Postfix's purge script deleted
// /etc/systemd/system/postfix.service.d/ whole, with the owner's drop-in in
// it, and the answer did not name it. Second finding (re-verification): the
// first fix put back everything no dpkg package owns into the component's
// own folders, so files the maintainer scripts and CelikPanel had written
// (main.cf, master.cf, dynamicmaps.cf, the *.proto copies, vmailbox,
// virtual, /etc/postgresql-common/createcluster.conf, root.crt) came back as
// "your own files" and /etc/postfix kept eleven files.
//
// The rule now, simple and safe:
//
//   - A unit's systemd drop-in folders (/etc/systemd/system/<unit>.d) are
//     where owners customise and packages never put their own files: an
//     owner's file there that the purge deleted is put back in place.
//   - The component's own folders under /etc (listed per component below,
//     and only when a removed package owns files in them) are emptied and
//     removed. Nothing is put back there. A file that may be the owner's (no
//     removed package owns it, ucf does not register it, and it is not one
//     CelikPanel or the package's maintainer scripts write) is copied first
//     to /var/backups/celikpanel/kept/<component>-<time>/ (root only), and
//     the answer says "a copy of N files you may have added is in …".
//   - A package configuration file the owner changed is purged as purge
//     means; a copy is kept in the same folder and named.
//   - Elsewhere (shared folders, or a component not listed) a file that may
//     be the owner's is copied too; the copy is deleted again when the file
//     is still in place after the purge, and kept and named when the purge
//     deleted it.
//
// Sahibin dosyaları: birim drop-in klasörlerinde sahibin dosyası yerine
// konur; bileşenin kendi klasörleri boşaltılıp kaldırılır, sahibin eklemiş
// olabileceği dosyaların kopyası /var/backups/celikpanel/kept altında kalır
// ve yanıt yolu söyler.

var (
	dpkgInfoDir     = "/var/lib/dpkg/info"
	ucfRegistryPath = "/var/lib/ucf/registry"
	keptFilesRoot   = "/var/backups/celikpanel/kept"
	systemdEtcDir   = "/etc/systemd/system"
	etcDir          = "/etc"
)

// ownerFilesWalkCap bounds how many entries one removal looks at.
const ownerFilesWalkCap = 5000

// componentOwnDirs are the folders under /etc that belong to one component
// alone: emptied and removed with its purge.
var componentOwnDirs = map[string][]string{
	"postfix":    {"postfix"},
	"dovecot":    {"dovecot"},
	"postgresql": {"postgresql-common"},
}

// componentWrittenFiles are the files in a component's own folders that the
// package's maintainer scripts or CelikPanel write (paths relative to the
// folder, shell patterns). They go with the folder without a copy.
var componentWrittenFiles = map[string][]string{
	// Postfix's postinst writes main.cf, master.cf, dynamicmaps.cf and the
	// *.proto copies; CelikPanel writes the virtual mailbox maps.
	"postfix": {"main.cf", "master.cf", "main.cf.*", "master.cf.*", "dynamicmaps.cf", "dynamicmaps.cf.d/*",
		"postfix-files", "postfix-files.d/*", "*.proto", "makedefs.out", "sasl/smtpd.conf",
		"vmailbox", "vmailbox.*", "vmailbox_domains", "vmailbox_domains.*", "virtual", "virtual.*"},
	// Dovecot's postinst makes its certificate; CelikPanel writes the
	// mailbox password file.
	"dovecot": {"users", "private/*", "dh.pem", "dovecot.pem", "dovecot.key"},
	// postgresql-common's postinst writes these.
	"postgresql-common": {"createcluster.conf", "root.crt", "user_clusters", "environment", "pg_ctl.conf",
		"start.conf", "supported_versions", "pg_upgradecluster.d/*"},
}

// sharedEtcDirs are /etc folders many packages share.
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
	// kind: "dropin" (put back in place), "added" (may be the owner's;
	// copy only), "changed" (a package file the owner changed; copy only).
	kind string
	// cleared: in a component's own folder, removed with it.
	cleared bool
	mode    fs.FileMode
	uid     int
	gid     int
}

// ownerFilesKeep is what one removal kept aside and what it clears.
type ownerFilesKeep struct {
	root  string
	files []keptOwnerFile
	// ownDirs are the component's own folders to empty; inventory is every
	// file seen in them before the purge (only those are removed after it).
	ownDirs   []string
	inventory []string
}

// ownerFilesOutcome is what the answer says.
type ownerFilesOutcome struct {
	Restored []string // owner drop-ins put back after the purge deleted them
	InPlace  []string // owner drop-ins the purge left alone
	Copied   []string // files that may be the owner's, copied (component folders emptied)
	Changed  []string // package files the owner changed, copied before the purge
	Left     []string // files that appeared in a component folder during the removal, left
	Cleared  []string // the component's folders removed
	Notes    []string
	CopyDir  string
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

// ucfRegistered reads ucf's registry: the configuration files maintainer
// scripts manage through ucf ("<md5> <package> <path>").
func ucfRegistered() map[string]bool {
	registered := map[string]bool{}
	file, err := os.Open(ucfRegistryPath)
	if err != nil {
		return registered
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 8<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && strings.HasPrefix(fields[len(fields)-1], "/") {
			registered[filepath.Clean(fields[len(fields)-1])] = true
		}
	}
	return registered
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

// unitDropInDirs are the systemd drop-in folders of the units.
func unitDropInDirs(units []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, unit := range units {
		unit = strings.TrimSuffix(unit, ".service")
		if unit == "" || strings.ContainsAny(unit, "/") {
			continue
		}
		candidates := []string{filepath.Join(systemdEtcDir, unit+".service.d")}
		if at := strings.IndexByte(unit, '@'); at > 0 {
			candidates = append(candidates, filepath.Join(systemdEtcDir, unit[:at+1]+".service.d"))
		}
		for _, dir := range candidates {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	sort.Strings(dirs)
	return dirs
}

// otherEtcDirs are the non-shared /etc folders the packages keep
// configuration files in, other than the component's own folders.
func otherEtcDirs(conffiles map[string]string, own map[string]bool) []string {
	seen := map[string]bool{}
	var dirs []string
	for path := range conffiles {
		rel, err := filepath.Rel(etcDir, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		dir := filepath.Join(etcDir, top)
		if top == rel || sharedEtcDirs[top] || own[dir] || seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

// isCelikPanelFile: CelikPanel's own files carry its name.
func isCelikPanelFile(path string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(path)), "celikpanel")
}

// writtenByScripts: a file the package's maintainer scripts or CelikPanel
// write into the component's own folder.
func writtenByScripts(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, pattern := range componentWrittenFiles[filepath.Base(dir)] {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
	}
	return false
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

// keepOwnerFiles reads, before a purge, what the purge may take from the
// owner and copies it aside. component names the copies' folder.
func keepOwnerFiles(component string, packages, units []string, status io.Reader) (*ownerFilesKeep, error) {
	owned := dpkgOwnedFiles(packages)
	conffiles := dpkgConffiles(status, packages)
	ucf := ucfRegistered()
	keep := &ownerFilesKeep{root: filepath.Join(keptFilesRoot, fmt.Sprintf("%s-%s", component, pgLeftoverNow().UTC().Format("20060102T150405Z")))}
	var candidates []keptOwnerFile
	consider := func(path, kind string, cleared bool) {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		file := keptOwnerFile{path: path, kind: kind, cleared: cleared, mode: info.Mode().Perm()}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			file.uid, file.gid = int(stat.Uid), int(stat.Gid)
		}
		candidates = append(candidates, file)
	}
	// The component's own folders: only those a removed package owns
	// files in (a single PostgreSQL version's removal does not reach
	// /etc/postgresql-common).
	own := map[string]bool{}
	for _, name := range componentOwnDirs[component] {
		dir := filepath.Join(etcDir, name)
		usedByRemoved := false
		for path := range owned {
			if strings.HasPrefix(path, dir+"/") {
				usedByRemoved = true
				break
			}
		}
		if usedByRemoved && realDirectory(dir) {
			own[dir] = true
			keep.ownDirs = append(keep.ownDirs, dir)
		}
	}
	sort.Strings(keep.ownDirs)
	for path, sum := range conffiles {
		if isCelikPanelFile(path) {
			continue
		}
		if current, err := fileMD5(path); err == nil && current != sum {
			inOwn := false
			for dir := range own {
				inOwn = inOwn || strings.HasPrefix(path, dir+"/")
			}
			consider(path, "changed", inOwn)
		}
	}
	entries := 0
	walk := func(dir string, visit func(path string)) {
		if !realDirectory(dir) {
			return
		}
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			entries++
			if entries > ownerFilesWalkCap {
				return fs.SkipAll
			}
			if entry.Type().IsRegular() || entry.Type()&fs.ModeSymlink != 0 {
				visit(filepath.Clean(path))
			}
			return nil
		})
	}
	mayBeOwners := func(path string) bool {
		if owned[path] || ucf[path] || isCelikPanelFile(path) {
			return false
		}
		_, isConffile := conffiles[path]
		return !isConffile
	}
	for _, dir := range unitDropInDirs(units) {
		walk(dir, func(path string) {
			if mayBeOwners(path) {
				consider(path, "dropin", false)
			}
		})
	}
	for _, dir := range keep.ownDirs {
		walk(dir, func(path string) {
			keep.inventory = append(keep.inventory, path)
			if mayBeOwners(path) && !writtenByScripts(dir, path) {
				consider(path, "added", true)
			}
		})
	}
	for _, dir := range otherEtcDirs(conffiles, own) {
		walk(dir, func(path string) {
			if mayBeOwners(path) {
				consider(path, "added", false)
			}
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
		_ = os.RemoveAll(keep.root)
		removeEmptyDir(keptFilesRoot)
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

// finish runs after the purge: owner drop-ins the purge deleted go back,
// the component's own folders are emptied (only the files seen before the
// purge) and removed, and the copies that are still needed stay. After a
// purge that failed (purged false) nothing is emptied: the package may still
// be there and use its folder.
func (k *ownerFilesKeep) finish(purged bool) ownerFilesOutcome {
	var out ownerFilesOutcome
	if k == nil {
		return out
	}
	for _, file := range k.files {
		switch file.kind {
		case "dropin":
			if _, err := os.Lstat(file.path); err == nil {
				out.InPlace = append(out.InPlace, file.path)
				_ = os.Remove(file.copy)
				continue
			} else if !errors.Is(err, fs.ErrNotExist) {
				out.Notes = append(out.Notes, file.path+" could not be checked; its copy is in "+file.copy)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
				out.Notes = append(out.Notes, file.path+" could not be put back ("+err.Error()+"); its copy is in "+file.copy)
				continue
			}
			if err := copyFileMode(file.copy, file.path, file.mode); err != nil {
				out.Notes = append(out.Notes, file.path+" could not be put back ("+err.Error()+"); its copy is in "+file.copy)
				continue
			}
			_ = os.Chown(file.path, file.uid, file.gid)
			_ = os.Chmod(file.path, file.mode)
			out.Restored = append(out.Restored, file.path)
			_ = os.Remove(file.copy)
		case "changed":
			out.Changed = append(out.Changed, file.path)
		default: // "added"
			if _, err := os.Lstat(file.path); err == nil && (!file.cleared || !purged) {
				// Outside the component's folders and still in place.
				_ = os.Remove(file.copy)
				continue
			}
			out.Copied = append(out.Copied, file.path)
		}
	}
	// The component's own folders: the files seen before the purge go,
	// then the folders when they are empty.
	if !purged {
		pruneEmptyDirs(k.root)
		removeEmptyDir(keptFilesRoot)
		if len(out.Copied)+len(out.Changed) > 0 {
			out.CopyDir = k.root
		}
		return out
	}
	seen := map[string]bool{}
	for _, path := range k.inventory {
		seen[path] = true
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			out.Notes = append(out.Notes, path+" could not be removed: "+err.Error())
		}
	}
	for _, dir := range k.ownDirs {
		if !realDirectory(dir) {
			out.Cleared = append(out.Cleared, dir)
			continue
		}
		pruneEmptyDirs(dir)
		if !realDirectory(dir) {
			out.Cleared = append(out.Cleared, dir)
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && !seen[path] {
				out.Left = append(out.Left, path)
			}
			return nil
		})
	}
	pruneEmptyDirs(k.root)
	removeEmptyDir(keptFilesRoot)
	if len(out.Copied)+len(out.Changed) > 0 {
		out.CopyDir = k.root
	}
	return out
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

// detail is the removal answer's sentence about them.
func (o ownerFilesOutcome) detail() string {
	var parts []string
	if len(o.Restored) > 0 {
		parts = append(parts, "your own systemd drop-ins the package's purge deleted were put back: "+strings.Join(o.Restored, ", "))
	}
	if len(o.InPlace) > 0 {
		parts = append(parts, "your own systemd drop-ins were left in place: "+strings.Join(o.InPlace, ", "))
	}
	if len(o.Cleared) > 0 {
		parts = append(parts, "its configuration folders were removed: "+strings.Join(o.Cleared, ", "))
	}
	if len(o.Copied) > 0 {
		parts = append(parts, fmt.Sprintf("a copy of %d file(s) you may have added is in %s (%s)", len(o.Copied), o.CopyDir, strings.Join(o.Copied, ", ")))
	}
	if len(o.Changed) > 0 {
		parts = append(parts, "the package's configuration files you had changed were purged; copies are in "+o.CopyDir+": "+strings.Join(o.Changed, ", "))
	}
	if len(o.Left) > 0 {
		parts = append(parts, "files that appeared during the removal were left: "+strings.Join(o.Left, ", "))
	}
	parts = append(parts, o.Notes...)
	return strings.Join(parts, "; ")
}
