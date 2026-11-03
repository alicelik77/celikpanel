//go:build linux

package main

import (
	"os"
	"syscall"
)

// trafficFileID is a file's identity: its device and inode. A rotation
// gives the log's name a new inode.
type trafficFileID struct {
	Dev uint64 `json:"dev"`
	Ino uint64 `json:"ino"`
}

func statTrafficFile(path string) (trafficFileID, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return trafficFileID{}, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return trafficFileID{}, info.Size(), nil
	}
	return trafficFileID{Dev: uint64(stat.Dev), Ino: uint64(stat.Ino)}, info.Size(), nil
}
