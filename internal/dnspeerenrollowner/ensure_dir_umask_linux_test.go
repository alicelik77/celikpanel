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

// Native lab 2026-10-30, second run: the Agent's Group=celikpanel made every
// channel file root:celikpanel, and the inspector (which requires root:root)
// refused its own policy. New directories and files are root:root whatever
// group the caller or a setgid parent would give them.
func TestEnsureDirAndWriteExclusiveAreRootRootWhateverTheGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned directory test")
	}
	root, err := os.MkdirTemp("/root", "peer-owner-group-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	// A setgid parent with another group stands in for the Agent's group.
	if err := os.Chown(root, 0, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0755|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "channel")
	if err := ensureDir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "policy.json")
	if err := writeExclusive(file, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{dir, file} {
		st, err := os.Lstat(name)
		if err != nil {
			t.Fatal(err)
		}
		owner := st.Sys().(*syscall.Stat_t)
		if owner.Uid != 0 || owner.Gid != 0 {
			t.Fatalf("%s is %d:%d, want root:root", name, owner.Uid, owner.Gid)
		}
	}
}
