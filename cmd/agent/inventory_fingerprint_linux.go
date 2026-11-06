//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// The fingerprints of the cached inventory (inventory_cache.go). Each is the
// boot plus inode, size, mtime and ctime of the files whose change can change
// what was read. "" means "cannot say", and then nothing is cached.
// Önbellekli envanterin parmak izleri: değişimi okunanı değiştirebilecek
// dosyaların inode, boyut, mtime ve ctime değerleri. Boşsa önbellek kullanılmaz.

// Paths are variables for tests.
var (
	inventoryBootIDPath    = "/proc/sys/kernel/random/boot_id"
	inventoryOSReleasePath = "/etc/os-release"
	inventoryDpkgPaths     = []string{"/var/lib/dpkg/status", "/var/lib/dpkg/updates"}
	inventoryPacmanPaths   = []string{"/var/lib/pacman/local"}
	// The system unit search path of systemd (systemd.unit(5)). A drop-in or
	// .wants directory inside one is fingerprinted too.
	inventoryUnitDirs = []string{
		"/etc/systemd/system.control", "/run/systemd/system.control", "/run/systemd/transient",
		"/run/systemd/generator.early", "/etc/systemd/system", "/etc/systemd/system.attached",
		"/run/systemd/system", "/run/systemd/system.attached", "/run/systemd/generator",
		"/usr/local/lib/systemd/system", "/usr/lib/systemd/system", "/lib/systemd/system",
		"/run/systemd/generator.late",
	}
)

func inventoryBootID() (string, bool) {
	raw, err := os.ReadFile(inventoryBootIDPath)
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(string(raw))
	return id, id != ""
}

// writeStatFingerprint writes one path's identity; an absent path is written
// as absent, any other failure makes the fingerprint unusable.
func writeStatFingerprint(b *strings.Builder, path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			b.WriteString(path + "|absent\n")
			return true
		}
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	b.WriteString(path)
	for _, v := range []int64{int64(stat.Dev), int64(stat.Ino), stat.Size, stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec, int64(stat.Mode)} {
		b.WriteByte('|')
		b.WriteString(strconv.FormatInt(v, 10))
	}
	b.WriteByte('\n')
	return true
}

func fingerprintOf(paths []string, withSubdirectories bool) string {
	boot, ok := inventoryBootID()
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString(boot + "\n")
	for _, path := range paths {
		if !writeStatFingerprint(&b, path) {
			return ""
		}
		if !withSubdirectories {
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return ""
		}
		for _, entry := range entries {
			// .wants, .requires and .d directories: what they hold changes
			// their own mtime, not the parent's.
			if entry.IsDir() {
				if !writeStatFingerprint(&b, filepath.Join(path, entry.Name())) {
					return ""
				}
			}
		}
	}
	return b.String()
}

func hostProfileFingerprint() string {
	return fingerprintOf([]string{inventoryOSReleasePath}, false)
}

func packageDatabaseFingerprint(family string) string {
	switch family {
	case "apt":
		return fingerprintOf(inventoryDpkgPaths, false)
	case "pacman":
		return fingerprintOf(inventoryPacmanPaths, false)
	}
	return ""
}

func systemdUnitFingerprint() string {
	return fingerprintOf(inventoryUnitDirs, true)
}
