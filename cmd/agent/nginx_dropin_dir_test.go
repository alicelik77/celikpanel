//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNginxDropInDirectoryIsRootOwned(t *testing.T) {
	var chowned []string
	old := nginxDropInChown
	t.Cleanup(func() { nginxDropInChown = old })
	nginxDropInChown = func(path string, uid, gid int) error {
		if uid != 0 || gid != 0 {
			t.Fatalf("chown %s to %d:%d", path, uid, gid)
		}
		chowned = append(chowned, path)
		return nil
	}
	dir := t.TempDir()
	created := filepath.Join(dir, "sites-available")
	if err := ensureNginxDropInDirectory(created); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(created); err != nil || !info.IsDir() || info.Mode().Perm() != 0o755 {
		t.Fatalf("created: %v %v", info, err)
	}
	if len(chowned) != 1 || chowned[0] != created {
		t.Fatalf("chowned=%v", chowned)
	}
	// An existing directory owned by someone else stays as it is.
	chowned = nil
	foreign := filepath.Join(dir, "conf.d")
	if err := os.Mkdir(foreign, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(foreign, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := ensureNginxDropInDirectory(foreign); err != nil || len(chowned) != 0 {
		t.Fatalf("group-writable directory touched: %v %v", err, chowned)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureNginxDropInDirectory(file); err == nil {
		t.Fatal("a file was accepted as a directory")
	}
}
