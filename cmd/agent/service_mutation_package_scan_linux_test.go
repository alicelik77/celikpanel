//go:build linux

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// withProcReaders replaces the scan's per-process readers for one test: reads
// of a path ending in one of the keys fail with that error, as a real /proc
// answers for a process that ends while it is being read.
func withProcReaders(t *testing.T, failures map[string]error) {
	t.Helper()
	fail := func(path string) error {
		for suffix, err := range failures {
			if strings.HasSuffix(path, suffix) {
				return &fs.PathError{Op: "open", Path: path, Err: err}
			}
		}
		return nil
	}
	readFile, open := linuxProcReadFile, linuxProcOpen
	linuxProcReadFile = func(path string) ([]byte, error) {
		if err := fail(path); err != nil {
			return nil, err
		}
		return readFile(path)
	}
	linuxProcOpen = func(path string) (*os.File, error) {
		if err := fail(path); err != nil {
			return nil, err
		}
		return open(path)
	}
	t.Cleanup(func() { linuxProcReadFile, linuxProcOpen = readFile, open })
}

// alpha.136 lab: one idle probe in about a thousand refused with "read package
// process identity: read /proc/786484/comm: no such process". A process that is
// ending answers ESRCH; it is gone, exactly like one already removed (ENOENT).
func TestPackageScanTreatsAnEndingProcessAsGone(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"ending (ESRCH)", syscall.ESRCH},
		{"already removed (ENOENT)", syscall.ENOENT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPackageKitFixture(t)
			fixture.process(t, "4242", "sh", 1)
			withProcReaders(t, map[string]error{"/4242/comm": tc.err})
			if fixture.busy(t) {
				t.Fatal("a process that ended during the scan was counted as package activity")
			}
		})
	}
	t.Run("an ending process beside real package activity", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		fixture.process(t, "4242", "sh", 1)
		fixture.process(t, "4243", "dpkg", 1)
		withProcReaders(t, map[string]error{"/4242/comm": syscall.ESRCH})
		if !fixture.busy(t) {
			t.Fatal("dpkg was hidden by a process that ended during the scan")
		}
	})
	t.Run("an ending process while the daemon's parents are read", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		withProcReaders(t, map[string]error{"/900/status": syscall.ESRCH})
		if fixture.busy(t) {
			t.Fatal("a process that ended while PackageKit's children were looked for kept the daemon busy")
		}
	})
}

// Any other read failure is not an ended process: the scan still refuses.
func TestPackageScanStillRefusesAnUnreadableProcess(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.EACCES, syscall.EIO} {
		fixture := newPackageKitFixture(t)
		fixture.process(t, "4242", "sh", 1)
		withProcReaders(t, map[string]error{"/4242/comm": errno})
		fixture.file(t, "locks", "")
		busy, err := linuxPackageProcessBusyWith(fixture.root, fixture.locks)
		if err == nil || busy {
			t.Fatalf("%v: busy=%v err=%v, want the refusal", errno, busy, err)
		}
	}
}

// A PackageKit daemon that ends after it was listed is gone. ESRCH proves it;
// a missing maps file proves it only together with a missing process
// directory, so a live daemon with an unreadable map stays busy (the existing
// "backend map unreadable" case).
func TestPackageScanTreatsAnEndingPackageKitDaemonAsGone(t *testing.T) {
	t.Run("maps answers ESRCH", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		fixture.file(t, "1721/maps", "55d000000000-55d000001000 r-xp 00000000 08:01 17 /usr/libexec/packagekitd\n")
		withProcReaders(t, map[string]error{"/1721/maps": syscall.ESRCH})
		if fixture.busy(t) {
			t.Fatal("a PackageKit daemon that was ending was counted as busy")
		}
	})
	t.Run("process directory gone", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		if err := os.RemoveAll(filepath.Join(fixture.root, "1721")); err != nil {
			t.Fatal(err)
		}
		if !packageKitDaemonProvablyIdle(fixture.root, 1721, fixture.locks) {
			t.Fatal("a PackageKit daemon whose process directory is gone was counted as busy")
		}
	})
	t.Run("maps missing while the process directory remains", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		withProcReaders(t, map[string]error{"/1721/maps": syscall.ENOENT})
		if !fixture.busy(t) {
			t.Fatal("a live PackageKit daemon with a missing map was taken for an ended one")
		}
	})
}

// apt-daily.service and apt-daily-upgrade.service run apt.systemd.daily (kernel
// name "apt.systemd.dai"), apt-daily after "apt-helper wait-online". A run is
// package activity from its first moment, before apt-get or unattended-upgrade
// appear, so the updater's bounded wait sees it instead of a later check.
func TestPackageScanCountsTheAptDailyRunFromItsStart(t *testing.T) {
	for _, comm := range []string{"apt.systemd.dai", "apt-helper"} {
		t.Run(comm, func(t *testing.T) {
			fixture := newPackageKitFixture(t)
			fixture.process(t, "5100", comm, 1)
			if !fixture.busy(t) {
				t.Fatalf("%s was not counted as package activity", comm)
			}
		})
	}
	// Ubuntu Pro's apt-news.service and esm-cache.service (both "python3"),
	// which apt's update hook starts, use no system apt/dpkg lock; they are
	// not package activity by name (the lock probes still see any lock).
	t.Run("Ubuntu Pro news and ESM cache readers", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		fixture.process(t, "5200", "python3", 1)
		fixture.process(t, "5201", "python3", 1)
		if fixture.busy(t) {
			t.Fatal("a python3 reader was counted as package activity")
		}
	})
}

// alpha.137 review: on a hosting server any account can run a process named
// apt or dpkg (a copy of sleep is enough). Only a process with a root user id
// can be the host's package manager; one whose owner cannot be read counts as
// before (fail safe), and one that ended while its owner was read is gone.
func TestPackageScanCountsOnlyRootOwnedPackageProcesses(t *testing.T) {
	withOwner := func(t *testing.T, fixture *packageKitFixture, pid, comm, uids string) {
		t.Helper()
		fixture.file(t, pid+"/comm", comm+"\n")
		fixture.file(t, pid+"/status", "Name:\t"+comm+"\nState:\tS (sleeping)\nPid:\t"+pid+"\nPPid:\t1\nUid:\t"+uids+"\nGid:\t0\t0\t0\t0\n")
	}
	for _, comm := range []string{"apt", "apt-get", "dpkg", "apt.systemd.dai", "apt-helper", "rpm", "pacman"} {
		t.Run(comm+" of an ordinary account", func(t *testing.T) {
			fixture := newPackageKitFixture(t)
			withOwner(t, fixture, "6100", comm, "1001\t1001\t1001\t1001")
			if fixture.busy(t) {
				t.Fatalf("an ordinary account's %s held the update back", comm)
			}
		})
		t.Run(comm+" of root", func(t *testing.T) {
			fixture := newPackageKitFixture(t)
			withOwner(t, fixture, "6100", comm, "0\t0\t0\t0")
			if !fixture.busy(t) {
				t.Fatalf("root's %s was not counted", comm)
			}
		})
	}
	// Any root id counts (a set-user-id or a process changing its ids).
	for _, uids := range []string{"1001\t0\t0\t0", "0\t1001\t1001\t1001", "1001\t1001\t0\t1001", "1001\t1001\t1001\t0"} {
		fixture := newPackageKitFixture(t)
		withOwner(t, fixture, "6100", "dpkg", uids)
		if !fixture.busy(t) {
			t.Fatalf("dpkg with ids %q was not counted", uids)
		}
	}
	t.Run("an owner that cannot be read counts", func(t *testing.T) {
		for _, status := range []string{
			"Name:\tdpkg\nPid:\t6100\nPPid:\t1\n",                   // no Uid line
			"Name:\tdpkg\nPid:\t6100\nPPid:\t1\nUid:\t1001\n",       // malformed
			"Name:\tdpkg\nPid:\t6100\nPPid:\t1\nUid:\tx\tx\tx\tx\n", // unparsable
		} {
			fixture := newPackageKitFixture(t)
			fixture.file(t, "6100/comm", "dpkg\n")
			fixture.file(t, "6100/status", status)
			if !fixture.busy(t) {
				t.Fatalf("dpkg with status %q was not counted", status)
			}
		}
		for _, errno := range []syscall.Errno{syscall.EACCES, syscall.EIO} {
			fixture := newPackageKitFixture(t)
			withOwner(t, fixture, "6100", "dpkg", "1001\t1001\t1001\t1001")
			withProcReaders(t, map[string]error{"/6100/status": errno})
			if !fixture.busy(t) {
				t.Fatalf("dpkg whose status answers %v was not counted", errno)
			}
		}
	})
	t.Run("a process that ended while its owner was read is gone", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		withOwner(t, fixture, "6100", "dpkg", "0\t0\t0\t0")
		withProcReaders(t, map[string]error{"/6100/status": syscall.ESRCH})
		if fixture.busy(t) {
			t.Fatal("a dpkg that ended during the scan was counted")
		}
	})
	t.Run("a PackageKit daemon name of an ordinary account", func(t *testing.T) {
		fixture := newPackageKitFixture(t)
		withOwner(t, fixture, "6200", "packagekitd", "1001\t1001\t1001\t1001")
		fixture.file(t, "6200/maps", "")
		if fixture.busy(t) {
			t.Fatal("an ordinary account's packagekitd held the update back")
		}
	})
}
