//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchMariaDBBindAddressWrittenOnceUnlessNamed(t *testing.T) {
	dir := t.TempDir()
	old := archMariaDBDropInDir
	t.Cleanup(func() { archMariaDBDropInDir = old })
	archMariaDBDropInDir = dir
	if err := os.WriteFile(filepath.Join(dir, "server.cnf"), []byte("[server]\n[mariadb-13.0]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareArchMariaDBBindAddress(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, archMariaDBBindFile))
	if err != nil || !strings.Contains(string(got), "bind-address = 127.0.0.1") {
		t.Fatalf("drop-in=%q err=%v", got, err)
	}
	// An owner's address (any file) keeps CelikPanel's file out.
	other := t.TempDir()
	archMariaDBDropInDir = other
	if err := os.WriteFile(filepath.Join(other, "server.cnf"), []byte("[mysqld]\nbind_address = 0.0.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareArchMariaDBBindAddress(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, archMariaDBBindFile)); !os.IsNotExist(err) {
		t.Fatalf("written beside an owner's address: %v", err)
	}
}

func TestArchFail2banDefaultsWrittenOnce(t *testing.T) {
	dir := t.TempDir()
	old := archFail2banDefaultsPath
	t.Cleanup(func() { archFail2banDefaultsPath = old })
	archFail2banDefaultsPath = filepath.Join(dir, "celikpanel-defaults.conf")
	if err := prepareArchFail2ban(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(archFail2banDefaultsPath); !strings.Contains(string(got), "_SYSTEMD_UNIT=sshd.service") {
		t.Fatalf("defaults=%q", got)
	}
	if err := os.WriteFile(archFail2banDefaultsPath, []byte("owner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareArchFail2ban(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(archFail2banDefaultsPath); string(got) != "owner\n" {
		t.Fatalf("an existing file was rewritten: %q", got)
	}
}
