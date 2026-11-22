//go:build linux

package services

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func withTestLogRoot(t *testing.T, root string) {
	t.Helper()
	savedRoot, savedTraffic, savedIDs := celikPanelLogRoot, trafficLogDirectory, webServerIDs
	celikPanelLogRoot, trafficLogDirectory = root, filepath.Join(root, "traffic")
	webServerIDs = func() (int, int, bool) { return os.Geteuid(), os.Getegid(), true }
	t.Cleanup(func() { celikPanelLogRoot, trafficLogDirectory, webServerIDs = savedRoot, savedTraffic, savedIDs })
}

// alpha.144: CelikPanel's log folder is made passable (0711) and the traffic
// folder under it passable for the web server's group only (0710), whatever
// the umask, also when the traffic folder is the first thing made. Before,
// the umask 0027 left the log folder 0750 and the workers of nginx could
// not reach their logs.
func TestLogRootIsMadePassableWhateverTheUmask(t *testing.T) {
	old := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(old) })
	root := filepath.Join(t.TempDir(), "log", "celikpanel")
	withTestLogRoot(t, root)
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]os.FileMode{root: 0o711, filepath.Join(root, "traffic"): 0o710} {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s: %v %v, want %04o", dir, info.Mode().Perm(), err, want)
		}
	}
	// An existing folder at another mode is left as it is.
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureCelikPanelLogRoot(); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(root); info.Mode().Perm() != 0o700 {
		t.Fatalf("an existing folder was changed: %04o", info.Mode().Perm())
	}
}

// Review of PR #355, finding 1: a site's first traffic log is made closed
// (0640) before nginx opens it, never at nginx's own 0644.
func TestASitesFirstTrafficLogIsMadeClosed(t *testing.T) {
	old := syscall.Umask(0)
	t.Cleanup(func() { syscall.Umask(old) })
	root := filepath.Join(t.TempDir(), "celikpanel")
	withTestLogRoot(t, root)
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatal(err)
	}
	path := TrafficLogPath(7)
	if err := ensureTrafficLogFile(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("the first log: %v %v", info.Mode().Perm(), err)
	}
	// An existing log is left as it is; a link in its place is not followed.
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(target, TrafficLogPath(8)); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrafficLogFile(TrafficLogPath(8)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatal("the link was followed")
	}
}

// Independent review of the last commit, B4: an existing traffic folder and
// an existing log need no lookup of the web server's account; only making
// them does, and a failed lookup then makes nothing.
func TestTrafficLogNeedsTheWebServerAccountOnlyToMakeIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "celikpanel")
	withTestLogRoot(t, root)
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatal(err)
	}
	path := TrafficLogPath(9)
	if err := ensureTrafficLogFile(path); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	webServerIDs = func() (int, int, bool) { lookups++; return 0, 0, false }
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatalf("an existing folder: %v", err)
	}
	if err := ensureTrafficLogFile(path); err != nil {
		t.Fatalf("an existing log: %v", err)
	}
	if lookups != 0 {
		t.Fatalf("looked up %d times for what exists", lookups)
	}
	// Making them needs the account; without it nothing is made.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(TrafficLogDir()); err != nil {
		t.Fatal(err)
	}
	if err := ensureTrafficLogDir(); !errors.Is(err, errWebServerAccountUnknown) {
		t.Fatalf("making the folder without the account: %v", err)
	}
	if _, err := os.Lstat(TrafficLogDir()); !os.IsNotExist(err) {
		t.Fatalf("the folder was made without its group: %v", err)
	}
}

// Review of PR #355, finding 2: a log folder that is a link the owner made
// is not a failure: traffic counting goes on through it as before.
func TestALogFolderThatIsTheOwnersLinkIsUsedAsBefore(t *testing.T) {
	base := t.TempDir()
	disk := filepath.Join(base, "disk")
	if err := os.MkdirAll(disk, 0o750); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "celikpanel")
	if err := os.Symlink(disk, root); err != nil {
		t.Fatal(err)
	}
	withTestLogRoot(t, root)
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatalf("through the owner's link: %v", err)
	}
	if info, err := os.Stat(filepath.Join(disk, "traffic")); err != nil || !info.IsDir() {
		t.Fatalf("the traffic folder through the link: %v", err)
	}
	if info, _ := os.Stat(disk); info.Mode().Perm() != 0o750 {
		t.Fatalf("the owner's folder was changed: %04o", info.Mode().Perm())
	}
	// A link that leads nowhere is still an error, as before.
	dangling := filepath.Join(base, "nowhere")
	_ = os.Symlink(filepath.Join(base, "missing"), dangling)
	if _, err := ensureLogFolder(dangling, LogRootMode, -1); err == nil {
		t.Fatal("a dangling link was taken for a folder")
	}
}
