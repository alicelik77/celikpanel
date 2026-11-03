package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Lab round 3: PrestaShop 9's own installer renames admin/ to admin + 19
// random characters, next to its admin-api/ folder.
func TestPrestaShopAdminPathFindsTheInstallersOwnRename(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"admin243uel9a76t8zr4wjhm", "admin-api", "app", "var"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path, err := prestashopAdminPath(catalogSite{docRoot: root})
	if err != nil || path != "/admin243uel9a76t8zr4wjhm/" {
		t.Fatalf("path = %q, err = %v", path, err)
	}
}

func TestPrestaShopAdminPathRefusesTwoCandidates(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"adminabcdefgh1234", "admin243uel9a76t8zr4wjhm"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := prestashopAdminPath(catalogSite{docRoot: root}); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrestaShopAdminPathIgnoresPlainAdminAndAdminAPI(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"admin", "admin-api"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := prestashopAdminPath(catalogSite{docRoot: root}); err == nil {
		t.Fatal("plain admin/ and admin-api/ must not count as the renamed folder")
	}
}
