//go:build linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// S4 R1, third fix round, finding 2: the undo of a first phpMyAdmin move
// removes the conf.d file the move created while it still holds what the
// move wrote, and a recorded file that vanished is not reported as changed
// on the server. Both fail on b6d3c832e: the created file was kept and named
// as changed, and so was the vanished one.
func TestTheUndoRemovesAFileTheMoveCreatedAndSaysNothingOfOneAlreadyGone(t *testing.T) {
	saved := phpToolFilesRecordDir
	t.Cleanup(func() { phpToolFilesRecordDir = saved })
	phpToolFilesRecordDir = t.TempDir()
	dir := filepath.Join(t.TempDir(), "conf.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	uid, gid := os.Getuid(), os.Getgid()
	created := filepath.Join(dir, "celikpanel-own-account.php")
	content := []byte("<?php\n$cfg['blowfish_secret'] = 'test';\n")
	vanished := filepath.Join(dir, "sess_temp")
	if err := os.WriteFile(vanished, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	journal, err := loadPHPToolFileJournal(phpToolPHPMyAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.note([]phpToolPlanned{
		{Path: vanished, After: phpToolFileMeta{UID: uid, GID: gid, Mode: 0o640}},
		{Path: created, WithContent: true, After: phpToolFileMeta{UID: uid, GID: gid, Mode: 0o640, ContentSHA: contentDigest(content)}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(vanished, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, content, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(created, 0o640); err != nil {
		t.Fatal(err)
	}
	// The tool removes its own temporary file before the move fails.
	if err := os.Remove(vanished); err != nil {
		t.Fatal(err)
	}
	if err := journal.undo(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if _, err := os.Lstat(created); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the file the move created is still there: %v", err)
	}
	if _, err := os.Stat(phpToolFilesRecordPath(phpToolPHPMyAdmin)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the record was kept after a complete undo")
	}

	// A created file changed since is kept and named.
	journal, err = loadPHPToolFileJournal(phpToolPHPMyAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.note([]phpToolPlanned{{Path: created, WithContent: true, After: phpToolFileMeta{UID: uid, GID: gid, Mode: 0o640, ContentSHA: contentDigest(content)}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created, append(content, "// the owner's line\n"...), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(created, 0o640); err != nil {
		t.Fatal(err)
	}
	var kept *errChangedOutsideTheMove
	if err := journal.undo(); !errors.As(err, &kept) || len(kept.paths) != 1 || kept.paths[0] != created {
		t.Fatalf("a changed file: %v", err)
	}
	if _, err := os.Stat(created); err != nil {
		t.Fatal("the owner's changed file was removed")
	}
}
