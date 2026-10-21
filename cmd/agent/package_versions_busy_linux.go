//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// realPackageDatabaseReadBusy decides whether the package database may be
// read for versions now. It only reads: the process table (a package tool
// running, or a PackageKit daemon not provably idle) and the kernel's lock
// table for the apt/dpkg lock files (any holder or waiter), plus the presence
// of pacman's lock file. Unlike the mutation readiness probe it never takes a
// lock itself, not even for a moment.
//
// realPackageDatabaseReadBusy yalnız okur: süreç tablosu, apt/dpkg kilit
// dosyalarının çekirdek kilit tablosu ve pacman kilit dosyasının varlığı.
// Kendisi hiçbir kilidi, bir an için bile, almaz.
func realPackageDatabaseReadBusy() (bool, error) {
	busy, err := linuxPackageProcessBusy()
	if err != nil || busy {
		return busy, err
	}
	held, err := linuxLockFilesHeld("/proc", linuxPackageKitTransactionLockPaths())
	if err != nil || held {
		return held, err
	}
	if _, err := os.Lstat("/var/lib/pacman/db.lck"); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect pacman lock: %w", err)
	}
	return false, nil
}

// linuxLockFilesHeld reports whether any process holds or waits for a lock on
// one of lockPaths, read from <procRoot>/locks by inode. A missing lock file
// is not held.
func linuxLockFilesHeld(procRoot string, lockPaths []string) (bool, error) {
	inodes := map[uint64]struct{}{}
	for _, path := range lockPaths {
		var stat unix.Stat_t
		if err := unix.Stat(path, &stat); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return false, err
		}
		inodes[stat.Ino] = struct{}{}
	}
	if len(inodes) == 0 {
		return false, nil
	}
	raw, err := os.ReadFile(filepath.Join(procRoot, "locks"))
	if err != nil {
		return false, err
	}
	return lockTableNamesInode(string(raw), inodes)
}

// lockTableNamesInode reads /proc/locks lines ("1: POSIX ADVISORY WRITE 1234
// 08:01:1311 0 EOF", "1: -> POSIX …" for a waiter) and answers whether one
// names a watched inode. An unreadable line is an error, never "idle".
func lockTableNamesInode(table string, inodes map[uint64]struct{}) (bool, error) {
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		index := 1
		if len(fields) > index && fields[index] == "->" {
			index++
		}
		if len(fields) < index+5 {
			return false, fmt.Errorf("unexpected lock table line %q", line)
		}
		identity := strings.Split(fields[index+4], ":")
		inode, err := strconv.ParseUint(identity[len(identity)-1], 10, 64)
		if err != nil || len(identity) != 3 {
			return false, fmt.Errorf("unexpected lock table line %q", line)
		}
		if _, watched := inodes[inode]; watched {
			return true, nil
		}
	}
	return false, nil
}
