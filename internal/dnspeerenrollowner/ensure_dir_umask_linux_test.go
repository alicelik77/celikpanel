//go:build linux

package dnspeerenrollowner

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Native lab 2026-10-30: the Agent (umask 027) created
// /etc/ssh/pdns-peer-inspector as 0750 and then refused its own directory as
// unsafe. A directory ensureDir creates gets exactly its reviewed mode under
// any umask; an existing directory with another mode is still refused.
func TestEnsureDirCreatesTheReviewedModeUnderARestrictiveUmask(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned directory test")
	}
	root, err := os.MkdirTemp("/root", "peer-ensure-dir-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	old := syscall.Umask(027)
	defer syscall.Umask(old)
	for _, mode := range []os.FileMode{0755, 0750} {
		name := filepath.Join(root, "dir-"+mode.String())
		if err := ensureDir(name, mode); err != nil {
			t.Fatalf("%v: %v", mode, err)
		}
		if st, err := os.Lstat(name); err != nil || st.Mode().Perm() != mode {
			t.Fatalf("%v: created %v %v", mode, st.Mode().Perm(), err)
		}
		if err := ensureDir(name, mode); err != nil {
			t.Fatalf("%v again: %v", mode, err)
		}
	}
	other := filepath.Join(root, "owner")
	if err := os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ensureDir(other, 0755); err == nil {
		t.Fatal("an existing directory with another mode was accepted")
	}
	if st, _ := os.Lstat(other); st.Mode().Perm() != 0700 {
		t.Fatal("an existing directory was changed")
	}
}
