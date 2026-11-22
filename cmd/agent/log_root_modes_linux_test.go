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

	"github.com/alicelik/celikpanel/internal/services"
)

const (
	testWebGroup   = 54321 // the web server's group, like www-data
	testWorkerUser = 54321 // the web server's account
	testOtherUser  = 54322 // a site's account: neither root nor in that group
)

func folderGroup(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return int(info.Sys().(*syscall.Stat_t).Gid)
}

// alpha.144: the log folder CelikPanel made at 0700 or 0750 gets pass-through
// for others once (0711, 0751); the traffic folder at 0755 (or 0711 from a
// build before the review) becomes 0710 with the web server's group; a log
// readable by others loses the others' bits; the certificate tool's folder
// stays 0700; any other mode is the owner's; the receipt keeps a later
// owner's mode.
func TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the correction changes only root-owned folders; run as root")
	}
	now := func() time.Time { return time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		rootMode, trafficMode   os.FileMode
		wantRoot, wantTraffic   os.FileMode
		trafficGroup, wantGroup int
		wantLog                 os.FileMode
	}{
		{0o750, 0o755, 0o751, 0o710, 0, testWebGroup, 0o640},            // the owner's server, 22 Nov 2026
		{0o700, 0o711, 0o711, 0o710, 0, testWebGroup, 0o640},            // certbot first; a pre-review build's 0711
		{0o755, 0o700, 0o755, 0o700, 0, 0, 0o644},                       // the owner's modes: kept, logs untouched
		{0o751, 0o710, 0o751, 0o710, testWebGroup, testWebGroup, 0o640}, // already closed: nothing to change
	} {
		base := t.TempDir()
		root, traffic, certbot := filepath.Join(base, "celikpanel"), filepath.Join(base, "celikpanel", "traffic"), filepath.Join(base, "celikpanel", "certbot")
		for _, dir := range []string{root, traffic, certbot} {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		_ = os.Chmod(root, c.rootMode)
		_ = os.Chown(traffic, 0, c.trafficGroup)
		_ = os.Chmod(traffic, c.trafficMode)
		logFile := filepath.Join(traffic, "7.log")
		mode := os.FileMode(0o644)
		if c.trafficMode == 0o710 {
			mode = 0o640
		}
		if err := os.WriteFile(logFile, nil, mode); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(logFile, mode)
		receipt := filepath.Join(base, "receipt")
		lines, ran := passLogRoot(receipt, root, traffic, certbot, testWebGroup, now)
		if !ran {
			t.Fatal("did not run")
		}
		for dir, want := range map[string]os.FileMode{root: c.wantRoot, traffic: c.wantTraffic, certbot: 0o700, logFile: c.wantLog} {
			if info, _ := os.Stat(dir); info.Mode().Perm() != want {
				t.Errorf("%04o/%04o: %s is %04o, want %04o (%v)", c.rootMode, c.trafficMode, filepath.Base(dir), info.Mode().Perm(), want, lines)
			}
		}
		if got := folderGroup(t, traffic); got != c.wantGroup {
			t.Errorf("%04o/%04o: traffic group %d, want %d", c.rootMode, c.trafficMode, got, c.wantGroup)
		}
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "certificate tool's logs stay closed") || !strings.Contains(joined, "holds: certbot (0700), traffic (") {
			t.Errorf("the journal does not say what the folder holds: %v", lines)
		}
		_ = os.Chmod(root, 0o750)
		if _, again := passLogRoot(receipt, root, traffic, certbot, testWebGroup, now); again {
			t.Fatal("ran twice")
		}
		if info, _ := os.Stat(root); info.Mode().Perm() != 0o750 {
			t.Fatalf("a later mode was changed back: %04o", info.Mode().Perm())
		}
	}
}

// A link the owner made in place of a folder is kept and named.
func TestLogRootKeepsTheOwnersLinks(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("run as root")
	}
	base := t.TempDir()
	real := filepath.Join(base, "disk", "traffic")
	_ = os.MkdirAll(real, 0o755)
	root := filepath.Join(base, "celikpanel")
	_ = os.Mkdir(root, 0o750)
	traffic := filepath.Join(root, "traffic")
	if err := os.Symlink(real, traffic); err != nil {
		t.Fatal(err)
	}
	lines, _ := passLogRoot(filepath.Join(base, "receipt"), root, traffic, filepath.Join(root, "certbot"), testWebGroup, time.Now)
	if info, _ := os.Stat(real); info.Mode().Perm() != 0o755 || !strings.Contains(strings.Join(lines, "\n"), "a link the owner made") {
		t.Fatalf("the owner's link: %04o %v", info.Mode().Perm(), lines)
	}
}

// A site's account cannot open another site's traffic log by name, before
// and after a rotation; the web server's workers can.
func TestTrafficLogsReachOnlyTheWebServerWorkers(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to act as other accounts")
	}
	base := t.TempDir()
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
	// A group root's own supplementary groups do not hold, like celikpanel
	// for www-data on the owner's server.
	_ = os.Chown(root, 0, 54320)
	_ = os.Chown(traffic, 0, 54320)
	logFile := filepath.Join(traffic, "7.log")
	if err := os.WriteFile(logFile, []byte("1 100 50\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(logFile, 0o644) // nginx's own mode for the first log
	_ = os.Chown(logFile, testWorkerUser, testWebGroup)
	open := func(uid, gid int, path string, flag int) error {
		return asOtherAccount(uid, gid, func() error {
			file, err := os.OpenFile(path, flag, 0)
			if err == nil {
				file.Close()
			}
			return err
		})
	}
	// Before the correction the workers cannot pass (the lab's [emerg]).
	if err := open(testWorkerUser, testWebGroup, logFile, os.O_WRONLY|os.O_APPEND); err == nil {
		t.Fatal("the worker reached the log through a 0750 folder")
	}
	passLogRoot(filepath.Join(base, "receipt"), root, traffic, filepath.Join(root, "certbot"), testWebGroup, time.Now)
	check := func(stage string) {
		t.Helper()
		if err := open(testWorkerUser, testWebGroup, logFile, os.O_WRONLY|os.O_APPEND); err != nil {
			t.Fatalf("%s: the worker cannot write its log: %v", stage, err)
		}
		if err := open(testOtherUser, testOtherUser, logFile, os.O_RDONLY); err == nil {
			t.Fatalf("%s: another site's account opened the log by name", stage)
		}
		if err := asOtherAccount(testOtherUser, testOtherUser, func() error { _, err := os.ReadDir(traffic); return err }); err == nil {
			t.Fatalf("%s: another site's account listed the traffic folder", stage)
		}
		if err := asOtherAccount(testWorkerUser, testWebGroup, func() error { _, err := os.ReadDir(traffic); return err }); err == nil {
			t.Fatalf("%s: the worker listed the traffic folder", stage)
		}
	}
	check("after the correction")
	// A rotation as logrotate's create does it: the file renamed, a new one
	// with the old one's mode and owner.
	info, _ := os.Stat(logFile)
	if err := os.Rename(logFile, logFile+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, nil, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(logFile, info.Mode().Perm())
	_ = os.Chown(logFile, testWorkerUser, testWebGroup)
	check("after a rotation")
	if err := open(testOtherUser, testOtherUser, logFile+".1", os.O_RDONLY); err == nil {
		t.Fatal("another site's account opened the rotated log")
	}
}

// asOtherAccount runs fn with the thread's file-system identity set to uid
// and gid; Linux applies setfsuid per thread and drops the file capabilities
// while it is not root.
func asOtherAccount(uid, gid int, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, _, errno := syscall.RawSyscall(syscall.SYS_SETFSGID, uintptr(gid), 0, 0); errno != 0 {
		return errno
	}
	if _, _, errno := syscall.RawSyscall(syscall.SYS_SETFSUID, uintptr(uid), 0, 0); errno != 0 {
		return errno
	}
	defer syscall.RawSyscall(syscall.SYS_SETFSGID, 0, 0, 0)
	defer syscall.RawSyscall(syscall.SYS_SETFSUID, 0, 0, 0)
	return fn()
}

// Review of PR #355, finding 2: CelikPanel's log folder as a link the owner
// made does not stop the certificate tool's storage (issuance goes on).
func TestCertbotStorageGoesOnThroughTheOwnersLogLink(t *testing.T) {
	base := t.TempDir()
	disk := filepath.Join(base, "disk")
	if err := os.MkdirAll(disk, 0o750); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "celikpanel")
	if err := os.Symlink(disk, root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(services.SetCelikPanelLogRootForTests(root))
	storage := certbotStorage{ConfigDir: filepath.Join(base, "config"), WorkDir: filepath.Join(base, "work"), LogsDir: filepath.Join(root, "certbot")}
	if err := ensureIsolatedSiteCertbotStorage(storage); err != nil {
		t.Fatalf("through the owner's link: %v", err)
	}
	if info, err := os.Stat(filepath.Join(disk, "certbot")); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the certificate tool's logs: %v", err)
	}
}
