//go:build linux

package dnsunitidentity

import (
	"os"
	"syscall"
)

// hostLibIsUsrLibAlias proves the merged-/usr layout read-only: /lib must be
// a root:root symlink whose target is exactly usr/lib (or /usr/lib), and
// /usr and /usr/lib must be real root:root directories that are not group or
// other writable. Anything else answers false, which keeps the /lib unit name
// refused.
func hostLibIsUsrLibAlias() bool {
	info, err := os.Lstat("/lib")
	if err != nil || info.Mode()&os.ModeSymlink == 0 || !rootOwned(info) {
		return false
	}
	target, err := os.Readlink("/lib")
	if err != nil || (target != "usr/lib" && target != "/usr/lib") {
		return false
	}
	for _, directory := range []string{"/usr", "/usr/lib"} {
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			!rootOwned(info) || info.Mode().Perm()&0o022 != 0 {
			return false
		}
	}
	return true
}

func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && stat.Gid == 0
}
