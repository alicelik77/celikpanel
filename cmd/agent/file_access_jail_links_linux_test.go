package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Lab run 2 of alpha.117: on a merged-/usr host the jail had no /bin, so the
// account's /bin/bash and the sftp subsystem could not start.
func TestFileAccessJailMirrorsMergedUsrLinks(t *testing.T) {
	host := t.TempDir()
	for _, dir := range []string{"usr/bin", "usr/lib64", "sbin"} {
		if err := os.MkdirAll(filepath.Join(host, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{"bin": "usr/bin", "lib64": "usr/lib64", "lib32": "/etc"}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(host, name)); err != nil {
			t.Fatal(err)
		}
	}
	previous := fileAccessHostRoot
	fileAccessHostRoot = host
	t.Cleanup(func() { fileAccessHostRoot = previous })

	root := t.TempDir()
	jail := &fileAccessJail{root: root, visited: map[string]bool{}}
	if err := jail.mirrorTopLevelLinks(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"bin": "usr/bin", "lib64": "usr/lib64"} {
		got, err := os.Readlink(filepath.Join(root, name))
		if err != nil || got != want {
			t.Fatalf("jail %s -> %q (%v), want %q", name, got, err, want)
		}
	}
	// A real host directory and a link that leaves /usr are not mirrored.
	for _, name := range []string{"sbin", "lib32"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("jail %s must not be created: %v", name, err)
		}
	}
	// Running again keeps the links as they are.
	if err := jail.mirrorTopLevelLinks(); err != nil {
		t.Fatal(err)
	}
}
