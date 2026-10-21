//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// packageVersionLockTableLimit bounds the read of /proc/locks. A larger table
// is not read further: the probe then cannot say "idle", so it answers busy.
const packageVersionLockTableLimit = 1 << 20

// realPackageDatabaseReadBusy decides whether the package database may be
// read for versions now. It only reads: the process table (a package tool
// running, among them pacman, or a PackageKit daemon not provably idle) and
// the kernel's lock table for the apt/dpkg lock files (any holder or waiter).
// Unlike the mutation readiness probe it never takes a lock itself, not even
// for a moment.
//
// pacman's /var/lib/pacman/db.lck is deliberately not a reason by itself:
// pacman removes it on exit, but a pacman that was killed leaves it behind,
// and then every later check would be "busy" for ever and the versions would
// never be read again. A live pacman (or makepkg) process is what the process
// table check finds; without one the lock file is stale and the read-only
// `pacman -Q` reads the local database as it is. A host that stays busy keeps
// its earlier reading, marked, and says how long (see the Panel).
//
// realPackageDatabaseReadBusy yalnız okur: süreç tablosu (pacman dahil paket
// araçları) ve apt/dpkg kilit dosyalarının çekirdek kilit tablosu. Kendisi
// hiçbir kilidi, bir an için bile, almaz. Canlı pacman süreci olmadan kalan
// db.lck bayattır ve tek başına meşgul sayılmaz.
func realPackageDatabaseReadBusy() (bool, error) {
	busy, err := linuxPackageProcessBusy()
	if err != nil || busy {
		return busy, err
	}
	return linuxLockFilesHeld("/proc", linuxPackageKitTransactionLockPaths())
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
	file, err := os.Open(filepath.Join(procRoot, "locks"))
	if err != nil {
		return false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, packageVersionLockTableLimit+1))
	if err != nil {
		return false, err
	}
	if len(raw) > packageVersionLockTableLimit {
		return false, fmt.Errorf("lock table is larger than %d bytes", packageVersionLockTableLimit)
	}
	return lockTableNamesInode(string(raw), inodes)
}

// lockTableNamesInode reads /proc/locks lines ("1: POSIX ADVISORY WRITE 1234
// 08:01:1311 0 EOF", "1: -> POSIX …" for a waiter) and answers whether one
// names a watched inode. A lock on an object without an inode, which the
// kernel prints as "<none>" (an anonymous file), cannot be one of the lock
// files and is skipped; any other unreadable line is an error, never "idle".
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
		if strings.HasPrefix(fields[index+4], "<none>") {
			continue
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
