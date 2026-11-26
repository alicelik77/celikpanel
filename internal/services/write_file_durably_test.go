package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteFileDurablySyncsBeforeTheRename (S4 R2a third fix round, review
// N4): the writer the Agent's move uses for unit files is the managed site
// files' own: the whole text and the mode are in a new file beside the target
// (synced; atomicWriteSiteFile) before the rename, and a failed rename leaves
// the earlier file and no temporary file.
func TestWriteFileDurablySyncsBeforeTheRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "celikrt-5.service")
	if err := os.WriteFile(target, []byte("earlier\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := siteFileRename
	t.Cleanup(func() { siteFileRename = old })
	var seen string
	siteFileRename = func(from, to string) error {
		raw, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		info, _ := os.Stat(from)
		if filepath.Dir(from) != dir || !strings.HasPrefix(filepath.Base(from), ".celikrt-5.service.celikpanel-") || info.Mode().Perm() != 0o644 {
			t.Errorf("temporary file %s mode %v", from, info.Mode())
		}
		seen = string(raw)
		return os.Rename(from, to)
	}
	if err := WriteFileDurably(target, []byte("[Service]\nSlice=celiksite_7.slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if seen != "[Service]\nSlice=celiksite_7.slice\n" {
		t.Fatalf("the text before the rename: %q", seen)
	}
	if raw, _ := os.ReadFile(target); string(raw) != seen {
		t.Fatalf("target: %q", raw)
	}
	siteFileRename = func(string, string) error { return os.ErrPermission }
	if err := WriteFileDurably(target, []byte("other\n"), 0o644); err == nil {
		t.Fatal("a failed rename answered done")
	}
	if raw, _ := os.ReadFile(target); string(raw) != seen {
		t.Fatalf("a failed rename changed the target: %q", raw)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("a temporary file was left: %v", entries)
	}
}
