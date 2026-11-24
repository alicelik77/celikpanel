//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// S4 R1: webmail's tree under its own account. The private parts (the
// configuration with des_key, the SQLite database with sessions, temp with
// attachments, logs) are closed before the tree's own directory lets others
// through, so nothing private is readable to others on the way; the
// published code is not touched (S4 R1 fix round: no recursive change).
func TestRoundcubeOwnAccountClosesPrivatePartsBeforeOpeningTheTree(t *testing.T) {
	base := filepath.Join(t.TempDir(), "celikpanel-webmail")
	for _, dir := range []string{"config", "db", "temp", "logs", "public_html", "program", "skins"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(base, 0o750); err != nil {
		t.Fatal(err)
	}
	var applied []string
	saved := setPHPToolPathMeta
	setPHPToolPathMeta = func(path string, _ bool, _, _ int, mode os.FileMode) error {
		rel, _ := filepath.Rel(base, path)
		applied = append(applied, rel)
		return nil
	}
	t.Cleanup(func() { setPHPToolPathMeta = saved })
	if err := applyRoundcubeOwnAccountPermissions(base, 990, 991); err != nil {
		t.Fatal(err)
	}
	if len(applied) == 0 || applied[len(applied)-1] != "." {
		t.Fatalf("applied %v: the tree's own directory comes last", applied)
	}
	for _, rel := range applied {
		switch rel {
		case "public_html", "program", "skins":
			t.Fatalf("published code was changed: %v", applied)
		}
	}
}
