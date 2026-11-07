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
