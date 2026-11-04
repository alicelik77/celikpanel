//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestRuntimeAppEntryFactsNeverFollowALink reads real files with openat2: a
// link as the entry, a link in the middle of the path and a linked app folder
// are reported as links, a FIFO does not stall the read, and a regular file's
// header is read.
func TestRuntimeAppEntryFactsNeverFollowALink(t *testing.T) {
	home := t.TempDir()
	app := filepath.Join(home, "app")
	bin := filepath.Join(app, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "server"), []byte("\x7fELF\x02\x01\x01rest"), 0o750); err != nil {
		t.Fatal(err)
	}
	facts, err := readRuntimeAppEntryFacts(app, "bin/server")
	if err != nil || !facts.Regular || facts.Link || facts.Mode != 0o750 || string(facts.Header[:4]) != "\x7fELF" {
		t.Fatalf("regular: %+v %v", facts, err)
	}
	if err := os.Symlink("/etc/shadow", filepath.Join(bin, "shadow")); err != nil {
		t.Fatal(err)
	}
	if facts, err := readRuntimeAppEntryFacts(app, "bin/shadow"); err != nil || !facts.Link {
		t.Fatalf("link entry: %+v %v", facts, err)
	}
	if err := os.Symlink("/usr/bin", filepath.Join(app, "sys")); err != nil {
		t.Fatal(err)
	}
	if facts, err := readRuntimeAppEntryFacts(app, "sys/env"); err != nil || !facts.Link {
		t.Fatalf("link in the path: %+v %v", facts, err)
	}
	if facts, err := readRuntimeAppEntryFacts(app, "bin/missing"); !os.IsNotExist(err) {
		t.Fatalf("missing: %+v %v", facts, err)
	}
	if err := syscall.Mkfifo(filepath.Join(bin, "fifo"), 0o600); err == nil {
		if facts, err := readRuntimeAppEntryFacts(app, "bin/fifo"); err != nil || facts.Regular {
			t.Fatalf("fifo: %+v %v", facts, err)
		}
	}
	other := t.TempDir()
	linkedHome := t.TempDir()
	if err := os.Symlink(other, filepath.Join(linkedHome, "app")); err != nil {
		t.Fatal(err)
	}
	if facts, err := readRuntimeAppEntryFacts(filepath.Join(linkedHome, "app"), "bin/server"); err != nil || !facts.FolderLink {
		t.Fatalf("linked folder: %+v %v", facts, err)
	}
}
