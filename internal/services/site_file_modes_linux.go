//go:build linux

package services

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// openRegularNoFollow opens path for reading without following a link at its
// last component and without blocking on a FIFO.
func openRegularNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
}

// rootOwned: the file belongs to uid 0, or to the process's own uid when it
// does not run as root (a test).
func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(stat.Uid) == 0 || int(stat.Uid) == os.Geteuid()
}
