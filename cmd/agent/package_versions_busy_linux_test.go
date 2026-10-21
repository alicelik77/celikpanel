//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLockTableNamesInode(t *testing.T) {
	watched := map[uint64]struct{}{1311: {}}
	for table, want := range map[string]bool{
		"": false,
		"1: POSIX  ADVISORY  WRITE 1234 08:01:999 0 EOF\n":  false,
		"1: POSIX  ADVISORY  WRITE 1234 08:01:1311 0 EOF\n": true,
		"1: POSIX  ADVISORY  WRITE 1234 08:01:999 0 EOF\n" +
			"1: -> POSIX  ADVISORY  WRITE 4321 08:01:1311 0 EOF\n": true,
		"2: OFDLCK ADVISORY  READ  -1 00:15:1311 0 EOF\n": true,
	} {
		got, err := lockTableNamesInode(table, watched)
		if err != nil || got != want {
			t.Fatalf("%q: held = %v, %v; want %v", table, got, err, want)
		}
	}
	if _, err := lockTableNamesInode("1: POSIX garbage\n", watched); err == nil {
		t.Fatal("an unreadable lock table line was taken as idle")
	}
}

// The probe reads the kernel's lock table; it never locks the file itself.
func TestLinuxLockFilesHeldOnlyReads(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "lock")
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var stat unix.Stat_t
	if err := unix.Stat(lock, &stat); err != nil {
		t.Fatal(err)
	}
	proc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(proc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "locks"), []byte("1: POSIX  ADVISORY  WRITE 1 08:01:"+uitoa(stat.Ino)+" 0 EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := linuxLockFilesHeld(proc, []string{lock, filepath.Join(dir, "missing")})
	if err != nil || !held {
		t.Fatalf("held = %v, %v", held, err)
	}
	if err := os.WriteFile(filepath.Join(proc, "locks"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if held, err := linuxLockFilesHeld(proc, []string{lock}); err != nil || held {
		t.Fatalf("idle: held = %v, %v", held, err)
	}
	if held, err := linuxLockFilesHeld(proc, []string{filepath.Join(dir, "missing")}); err != nil || held {
		t.Fatalf("no lock files: held = %v, %v", held, err)
	}
}

func uitoa(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
