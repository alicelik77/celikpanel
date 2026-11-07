//go:build linux

package main

import (
	"os"
	"syscall"
)

func certificateRequestOwnedBySelf(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

// openCertificateRequestKeyNoFollow opens the key file without following a
// symbolic link in its last component.
func openCertificateRequestKeyNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
}
