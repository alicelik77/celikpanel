package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// alpha.128 lab: an update re-hashed every Postfix .db while no source had
// changed. An index not older than its source is kept.
func TestPostfixIndexCurrentKeepsAnUpToDateIndex(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "virtual")
	if err := os.WriteFile(source, []byte("a@example.com b@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if postfixIndexCurrent(source, "hash") {
		t.Fatal("a missing index was taken as current")
	}
	if err := os.WriteFile(source+".db", []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(source, old, old)
	if !postfixIndexCurrent(source, "hash") {
		t.Fatal("an index newer than its source was rebuilt")
	}
	if postfixIndexCurrent(source, "lmdb") {
		t.Fatal("another map type's index was taken as current")
	}
	if postfixIndexCurrent(source, "texthash") {
		t.Fatal("texthash has no index")
	}
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(source, future, future)
	if postfixIndexCurrent(source, "hash") {
		t.Fatal("an index older than a changed source was kept")
	}
}
