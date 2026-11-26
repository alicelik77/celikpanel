//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
)

// fileIdentity is a file's device and inode ("dev:ino").
func fileIdentity(info os.FileInfo) (string, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), true
}
