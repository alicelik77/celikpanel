//go:build linux

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// alpha.144: the log folder CelikPanel made at 0700 or 0750 gets
// pass-through for others once (0711, 0751); the traffic folder at 0755
// becomes 0711; the certificate tool's folder stays 0700; any other mode is
// the owner's; the receipt keeps a later owner's mode.
func TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the correction changes only root-owned folders; run as root")
	}
	now := func() time.Time { return time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		rootMode, trafficMode os.FileMode
		wantRoot, wantTraffic os.FileMode
	}{
		{0o750, 0o755, 0o751, 0o711}, // the owner's server, 22 Nov 2026
		{0o700, 0o755, 0o711, 0o711}, // made first by the certificate tool's logs
		{0o755, 0o700, 0o755, 0o700}, // the owner's modes: kept
	} {
		base := t.TempDir()
		root, traffic, certbot := filepath.Join(base, "celikpanel"), filepath.Join(base, "celikpanel", "traffic"), filepath.Join(base, "celikpanel", "certbot")
		for dir, mode := range map[string]os.FileMode{root: c.rootMode} {
			if err := os.Mkdir(dir, mode); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(traffic, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(certbot, 0o700); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(root, c.rootMode)
		_ = os.Chmod(traffic, c.trafficMode)
		receipt := filepath.Join(base, "receipt")
		lines, ran := passLogRoot(receipt, root, traffic, certbot, now)
		if !ran {
			t.Fatal("did not run")
		}
		for dir, want := range map[string]os.FileMode{root: c.wantRoot, traffic: c.wantTraffic, certbot: 0o700} {
			if info, _ := os.Stat(dir); info.Mode().Perm() != want {
				t.Errorf("%04o/%04o: %s is %04o, want %04o (%v)", c.rootMode, c.trafficMode, filepath.Base(dir), info.Mode().Perm(), want, lines)
			}
		}
		if !strings.Contains(strings.Join(lines, "\n"), "certificate tool's logs stay closed") {
			t.Errorf("the certificate tool's folder is not named: %v", lines)
		}
		// The owner changes it later: never changed back.
		_ = os.Chmod(root, 0o750)
		if _, again := passLogRoot(receipt, root, traffic, certbot, now); again {
			t.Fatal("ran twice")
		}
		if info, _ := os.Stat(root); info.Mode().Perm() != 0o750 {
			t.Fatalf("a later mode was changed back: %04o", info.Mode().Perm())
		}
	}
}

// The workers of nginx (not root, not in the folder's group) reach a log in
// the traffic folder after the correction, and could not before it.
func TestWebServerWorkerReachesTrafficLogAfterTheCorrection(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to act as another account")
	}
	base := t.TempDir()
	// The test's temporary folders are 0700: passable for the worker too.
	for _, dir := range []string{filepath.Dir(base), base} {
		if err := os.Chmod(dir, 0o711); err != nil {
			t.Fatal(err)
		}
	}
	root, traffic := filepath.Join(base, "celikpanel"), filepath.Join(base, "celikpanel", "traffic")
	_ = os.Mkdir(root, 0o750)
	_ = os.Chmod(root, 0o750)
	_ = os.Mkdir(traffic, 0o755)
	_ = os.Chmod(traffic, 0o755)
	// A group root is not in, like celikpanel for www-data.
	const folderGroup = 54321
	_ = os.Chown(root, 0, folderGroup)
	_ = os.Chown(traffic, 0, folderGroup)
	logFile := filepath.Join(traffic, "7.log")
	if err := os.WriteFile(logFile, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	const worker = 65534 // nobody: like www-data, not in the folder's group
	_ = os.Chown(logFile, worker, -1)
	open := func() error {
		return asOtherAccount(worker, func() error {
			file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0)
			if err == nil {
				file.Close()
			}
			return err
		})
	}
	if err := open(); err == nil {
		t.Fatal("the worker reached the log through a 0750 folder")
	}
	passLogRoot(filepath.Join(base, "receipt"), root, traffic, filepath.Join(root, "certbot"), time.Now)
	if err := open(); err != nil {
		t.Fatalf("after the correction: %v", err)
	}
	// Listing the traffic folder stays closed to it.
	if err := asOtherAccount(worker, func() error { _, err := os.ReadDir(traffic); return err }); err == nil {
		t.Fatal("the worker can list the traffic folder")
	}
}

// asOtherAccount runs fn with the thread's file-system identity set to uid
// (and gid uid); Linux applies setfsuid per thread and drops the file
// capabilities while it is not root.
func asOtherAccount(uid int, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, _, errno := syscall.RawSyscall(syscall.SYS_SETFSUID, uintptr(uid), 0, 0); errno != 0 {
		return errno
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_SETFSGID, uintptr(uid), 0, 0); errno != 0 {
		return errno
	}
	defer syscall.RawSyscall(syscall.SYS_SETFSGID, 0, 0, 0)
	defer syscall.RawSyscall(syscall.SYS_SETFSUID, 0, 0, 0)
	return fn()
}
