//go:build linux

package services

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// alpha.144: CelikPanel's log folder is made passable (0711) whatever the
// umask, also when the traffic folder under it is the first thing made, and
// the traffic folder is passable only (0711). Before, the umask 0027 left
// the log folder 0750 and the workers of nginx could not reach their logs.
func TestLogRootIsMadePassableWhateverTheUmask(t *testing.T) {
	old := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(old) })
	root := filepath.Join(t.TempDir(), "log", "celikpanel")
	savedRoot, savedTraffic := celikPanelLogRoot, trafficLogDirectory
	celikPanelLogRoot, trafficLogDirectory = root, filepath.Join(root, "traffic")
	t.Cleanup(func() { celikPanelLogRoot, trafficLogDirectory = savedRoot, savedTraffic })
	if err := ensureTrafficLogDir(); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]os.FileMode{root: 0o711, filepath.Join(root, "traffic"): 0o711} {
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
