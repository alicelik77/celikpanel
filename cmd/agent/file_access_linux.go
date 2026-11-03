//go:build linux

package main

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/alicelik/celikpanel/internal/fileaccess"
	"github.com/alicelik/celikpanel/internal/hostingpath"
	"golang.org/x/sys/unix"
)

func statFileAccessChain(paths []string) ([]fileaccess.DirInfo, error) {
	chain := make([]fileaccess.DirInfo, 0, len(paths))
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("no owner for %s", p)
		}
		chain = append(chain, fileaccess.DirInfo{
			Path: p, IsDir: info.IsDir() && info.Mode()&os.ModeSymlink == 0,
			UID: stat.Uid, Mode: uint32(info.Mode().Perm()),
		})
	}
	return chain, nil
}

// ---------------------------------------------------------------------------
// The command-line jail
//
// The jail lives in the subscription's sites folder, beside the numbered site
// folders: bin, lib, usr ... are copies of the host's programs and their
// libraries at the same paths, with the host's symbolic links reproduced.
// Copies lose every setuid and setgid bit. /proc is not there, so no process
// of another account can be seen. The copies are refreshed on every save of
// an account with the command line; an operating system update reaches them
// at the next save.

type fileAccessJail struct {
	root    string
	visited map[string]bool
	libDirs []string
}

func ensureFileAccessJail(chroot string, users []fileaccess.JailUser) ([]string, bool, error) {
	root := filepath.Clean(chroot)
	if !strings.HasPrefix(root, hostingpath.SubscriptionsRoot()+"/") || filepath.Base(root) != "sites" {
		return nil, false, fmt.Errorf("refusing a jail outside the hosting root: %s", chroot)
	}
	jail := &fileAccessJail{root: root, visited: map[string]bool{}, libDirs: fileAccessLibraryDirs()}
	var missing []string
	for _, command := range fileaccess.JailCommands {
		hostPath := findHostCommand(command)
		if hostPath == "" {
			missing = append(missing, command)
			continue
		}
		if err := jail.addProgram(hostPath); err != nil {
			return missing, false, fmt.Errorf("copy %s: %w", command, err)
		}
	}
	// Merged-/usr hosts (Debian 13, Ubuntu, Arch) have /bin, /sbin and /lib*
	// as links into /usr. The tools above are found under /usr/bin, so the
	// jail had no /bin and sshd could not start the account's /bin/bash or
	// the sftp subsystem through it (lab run 2 of alpha.117). The jail gets
	// the same top-level links as the host.
	if err := jail.mirrorTopLevelLinks(); err != nil {
		return missing, false, err
	}
	// SFTP and scp for accounts that have the command line run the host's
	// sftp-server inside the jail.
	for _, candidate := range []string{"/usr/lib/openssh/sftp-server", "/usr/lib/ssh/sftp-server", "/usr/libexec/openssh/sftp-server"} {
		if _, err := os.Stat(candidate); err == nil {
			if err := jail.addProgram(candidate); err != nil {
				return missing, false, fmt.Errorf("copy sftp-server: %w", err)
			}
		}
	}
	for _, base := range []string{"/usr/share/terminfo", "/lib/terminfo", "/etc/terminfo"} {
		for _, name := range fileaccess.JailTerminfo {
			hostPath := filepath.Join(base, name)
			if info, err := os.Stat(hostPath); err == nil && info.Mode().IsRegular() {
				if _, err := jail.copyPath(hostPath, 0); err != nil {
					return missing, false, fmt.Errorf("copy terminfo %s: %w", name, err)
				}
			}
		}
	}
	if err := jail.writeEtc(users); err != nil {
		return missing, false, err
	}
	if err := jail.ensureDir("/tmp", 0o777|os.ModeSticky); err != nil {
		return missing, false, err
	}
	if err := jail.ensureDevices(); err != nil {
		return missing, false, err
	}
	return missing, mountHasNoDev(root), nil
}

// fileAccessTopLevelLinks are the root entries a merged-/usr host keeps as
// links into /usr.
var fileAccessTopLevelLinks = []string{"bin", "sbin", "lib", "lib32", "lib64", "libx32"}

// fileAccessHostRoot is "/" except in tests.
var fileAccessHostRoot = "/"

// mirrorTopLevelLinks copies each top-level link of the host (for example
// /bin -> usr/bin) into the jail. Only a link whose target stays inside /usr
// is mirrored; a real directory on the host is left to the program copies.
func (j *fileAccessJail) mirrorTopLevelLinks() error {
	for _, name := range fileAccessTopLevelLinks {
		hostPath := filepath.Join(fileAccessHostRoot, name)
		info, err := os.Lstat(hostPath)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(hostPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", hostPath, err)
		}
		clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(target)), "/")
		if clean != "usr" && !strings.HasPrefix(clean, "usr/") {
			continue
		}
		if err := j.ensureSymlink("/"+name, target); err != nil {
			return fmt.Errorf("link %s in the jail: %w", name, err)
		}
	}
	return nil
}

func findHostCommand(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

func fileAccessLibraryDirs() []string {
	dirs := []string{}
	add := func(dir string) {
		dir = filepath.Clean(dir)
		for _, existing := range dirs {
			if existing == dir {
				return
			}
		}
		dirs = append(dirs, dir)
	}
	var readConf func(file string, depth int)
	readConf = func(file string, depth int) {
		content, err := os.ReadFile(file)
		if err != nil || depth > 3 {
			return
		}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if rest, ok := strings.CutPrefix(line, "include "); ok {
				matches, _ := filepath.Glob(strings.TrimSpace(rest))
				for _, match := range matches {
					readConf(match, depth+1)
				}
				continue
			}
			if strings.HasPrefix(line, "/") {
				add(line)
			}
		}
	}
	readConf("/etc/ld.so.conf", 0)
	triplet := map[string]string{"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu"}[runtime.GOARCH]
	if triplet != "" {
		add("/lib/" + triplet)
		add("/usr/lib/" + triplet)
	}
	for _, dir := range []string{"/lib64", "/usr/lib64", "/lib", "/usr/lib"} {
		add(dir)
	}
	return dirs
}

// addProgram copies a program and, for an ELF file, its interpreter and every
// library it needs.
func (j *fileAccessJail) addProgram(hostPath string) error {
	real, err := j.copyPath(hostPath, 0)
	if err != nil {
		return err
	}
	return j.addLibraries(real)
}

func (j *fileAccessJail) addLibraries(hostPath string) error {
	if j.visited["elf:"+hostPath] {
		return nil
	}
	j.visited["elf:"+hostPath] = true
	file, err := elf.Open(hostPath)
	if err != nil {
		return nil // a script, not a program
	}
	defer file.Close()
	for _, prog := range file.Progs {
		if prog.Type != elf.PT_INTERP {
			continue
		}
		data, err := io.ReadAll(prog.Open())
		if err != nil {
			return err
		}
		interp := string(bytes.TrimRight(data, "\x00"))
		if _, err := j.copyPath(interp, 0); err != nil {
			return fmt.Errorf("interpreter %s: %w", interp, err)
		}
	}
	needed, err := file.ImportedLibraries()
	if err != nil {
		return nil
	}
	search := append([]string{}, j.libDirs...)
	for _, tag := range []elf.DynTag{elf.DT_RUNPATH, elf.DT_RPATH} {
		if values, err := file.DynString(tag); err == nil {
			for _, value := range values {
				for _, dir := range strings.Split(value, ":") {
					if strings.HasPrefix(dir, "/") {
						search = append([]string{dir}, search...)
					}
				}
			}
		}
	}
	for _, library := range needed {
		found := ""
		for _, dir := range search {
			candidate := filepath.Join(dir, library)
			if sameELFKind(candidate, file) {
				found = candidate
				break
			}
		}
		if found == "" {
			return fmt.Errorf("library %s of %s not found", library, hostPath)
		}
		real, err := j.copyPath(found, 0)
		if err != nil {
			return err
		}
		if err := j.addLibraries(real); err != nil {
			return err
		}
	}
	return nil
}

func sameELFKind(candidate string, program *elf.File) bool {
	library, err := elf.Open(candidate)
	if err != nil {
		return false
	}
	defer library.Close()
	return library.Class == program.Class && library.Machine == program.Machine
}

// copyPath reproduces hostPath inside the jail: directories are created,
// symbolic links are reproduced with the same target, and the final regular
// file is copied without setuid/setgid bits. It returns the host path of the
// final file.
func (j *fileAccessJail) copyPath(hostPath string, depth int) (string, error) {
	if depth > 16 {
		return "", errors.New("too many symbolic links")
	}
	clean := filepath.Clean(hostPath)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("not an absolute path: %s", hostPath)
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	current := "/"
	for index, part := range parts {
		next := path.Join(current, part)
		info, err := os.Lstat(next)
		if err != nil {
			return "", err
		}
		last := index == len(parts)-1
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(next)
			if err != nil {
				return "", err
			}
			if err := j.ensureSymlink(next, target); err != nil {
				return "", err
			}
			resolved := target
			if !filepath.IsAbs(resolved) {
				resolved = path.Join(current, target)
			}
			rest := path.Join(append([]string{resolved}, parts[index+1:]...)...)
			return j.copyPath(rest, depth+1)
		case info.IsDir():
			if last {
				return "", fmt.Errorf("%s is a directory", next)
			}
			if err := j.ensureDir(next, 0o755); err != nil {
				return "", err
			}
		case info.Mode().IsRegular() && last:
			return next, j.copyFile(next, info)
		default:
			return "", fmt.Errorf("unexpected file type at %s", next)
		}
		current = next
	}
	return "", fmt.Errorf("nothing to copy at %s", hostPath)
}

// jailPath maps a host path into the jail and refuses any symbolic link on
// the way inside the jail, so a write never leaves it.
func (j *fileAccessJail) jailPath(hostPath string, allowLastLink bool) (string, error) {
	clean := filepath.Clean(hostPath)
	target := filepath.Join(j.root, clean)
	if !strings.HasPrefix(target, j.root+"/") {
		return "", fmt.Errorf("path escapes the jail: %s", hostPath)
	}
	current := j.root
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 && !(allowLastLink && index == len(parts)-1) {
			return "", fmt.Errorf("the jail has a symbolic link at %s", current)
		}
	}
	return target, nil
}

func (j *fileAccessJail) ensureDir(hostPath string, mode os.FileMode) error {
	target, err := j.jailPath(hostPath, false)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", target)
		}
		return os.Chmod(target, mode)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}

func (j *fileAccessJail) ensureSymlink(hostPath, linkTarget string) error {
	target, err := j.jailPath(hostPath, true)
	if err != nil {
		return err
	}
	if existing, err := os.Readlink(target); err == nil {
		if existing == linkTarget {
			return nil
		}
		if err := os.Remove(target); err != nil {
			return err
		}
	} else if info, statErr := os.Lstat(target); statErr == nil {
		return fmt.Errorf("%s exists and is not a link (%s)", target, info.Mode().Type())
	}
	return os.Symlink(linkTarget, target)
}

func (j *fileAccessJail) copyFile(hostPath string, info os.FileInfo) error {
	if j.visited["file:"+hostPath] {
		return nil
	}
	j.visited["file:"+hostPath] = true
	target, err := j.jailPath(hostPath, false)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(hostPath)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm() &^ 0o022
	if existing, err := os.ReadFile(target); err == nil && bytes.Equal(existing, content) {
		return os.Chmod(target, mode)
	}
	return writeJailFile(target, content, mode)
}

// writeJailFile replaces a file in a root-owned jail directory atomically.
func writeJailFile(target string, content []byte, mode os.FileMode) error {
	temp := target + ".celikpanel-new"
	_ = os.Remove(temp)
	file, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := os.Chmod(temp, mode); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return os.Rename(temp, target)
}

func (j *fileAccessJail) writeEtc(users []fileaccess.JailUser) error {
	if err := j.ensureDir("/etc", 0o755); err != nil {
		return err
	}
	webGroup, webGID := "", 0
	if web, ok := hostingpath.WebServerAccount(); ok && len(web.GIDs) > 0 {
		webGroup, webGID = web.Name, int(web.GIDs[0])
	}
	files := map[string]string{
		"/etc/passwd":        fileaccess.RenderJailPasswd(users),
		"/etc/group":         fileaccess.RenderJailGroup(users, webGroup, webGID),
		"/etc/profile":       fileaccess.RenderJailProfile(users),
		"/etc/nsswitch.conf": "passwd: files\ngroup: files\n",
	}
	for name, content := range files {
		target, err := j.jailPath(name, false)
		if err != nil {
			return err
		}
		if existing, err := os.ReadFile(target); err == nil && string(existing) == content {
			continue
		}
		if err := writeJailFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write the jail's %s: %w", name, err)
		}
	}
	return nil
}

var fileAccessJailDevices = []struct {
	name         string
	major, minor uint32
}{
	{"null", 1, 3}, {"zero", 1, 5}, {"random", 1, 8}, {"urandom", 1, 9}, {"tty", 5, 0},
}

func (j *fileAccessJail) ensureDevices() error {
	if err := j.ensureDir("/dev", 0o755); err != nil {
		return err
	}
	for _, device := range fileAccessJailDevices {
		target, err := j.jailPath("/dev/"+device.name, false)
		if err != nil {
			return err
		}
		want := unix.Mkdev(device.major, device.minor)
		var stat unix.Stat_t
		if err := unix.Lstat(target, &stat); err == nil {
			if stat.Mode&unix.S_IFMT == unix.S_IFCHR && uint64(stat.Rdev) == want {
				continue
			}
			if err := os.Remove(target); err != nil {
				return err
			}
		}
		if err := unix.Mknod(target, unix.S_IFCHR|0o666, int(want)); err != nil {
			return fmt.Errorf("create the jail's /dev/%s: %w", device.name, err)
		}
		if err := os.Chmod(target, 0o666); err != nil {
			return err
		}
	}
	return nil
}

// mountHasNoDev reports whether the jail's filesystem is mounted nodev, in
// which case /dev/null inside it does not work. The mount is the owner's
// choice and is not changed; the page says so.
func mountHasNoDev(dir string) bool {
	content, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	best, noDev := "", false
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		mountPoint := strings.ReplaceAll(fields[4], `\040`, " ")
		if dir != mountPoint && !strings.HasPrefix(dir, strings.TrimSuffix(mountPoint, "/")+"/") {
			continue
		}
		if len(mountPoint) >= len(best) {
			best = mountPoint
			noDev = false
			for _, option := range strings.Split(fields[5], ",") {
				if option == "nodev" {
					noDev = true
				}
			}
		}
	}
	return noDev
}
