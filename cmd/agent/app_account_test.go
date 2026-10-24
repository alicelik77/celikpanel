package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// alpha.107 (D-038 §6): an application's files go from the shared web user
// to the site's user, exactly those, recorded, and come back exactly.

func ownerOf(t *testing.T, path string) (int, int) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	return int(stat.Uid), int(stat.Gid)
}

func TestAnApplicationsFilesMoveAndComeBackExactly(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing owners needs root")
	}
	dir := t.TempDir()
	const web, site, other = 4201, 4202, 4203
	for name, uid := range map[string]int{"server.js": web, "logs/app.log": web, "README": other} {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("x"), 0o644)
		os.Lchown(path, uid, uid)
	}
	os.Lchown(filepath.Join(dir, "logs"), web, web)
	os.Symlink("/etc/passwd", filepath.Join(dir, "link"))
	os.Lchown(filepath.Join(dir, "link"), web, web)

	record, err := moveAppFilesOwner(7, dir, web, web, site, site)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 4 {
		t.Fatalf("paths = %v", record.Paths)
	}
	if uid, gid := ownerOf(t, filepath.Join(dir, "server.js")); uid != site || gid != site {
		t.Fatalf("server.js = %d:%d", uid, gid)
	}
	if uid, _ := ownerOf(t, filepath.Join(dir, "README")); uid != other {
		t.Fatal("a file of another owner was changed")
	}
	if uid, _ := ownerOf(t, "/etc/passwd"); uid != 0 {
		t.Fatal("a symbolic link was followed")
	}
	if err := restoreAppFilesOwner(record); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"server.js", "logs", "logs/app.log", "link"} {
		if uid, gid := ownerOf(t, filepath.Join(dir, name)); uid != web || gid != web {
			t.Fatalf("%s back = %d:%d", name, uid, gid)
		}
	}
}

func TestAnApplicationDirectoryOutsideTheHostingRootIsRefused(t *testing.T) {
	for _, dir := range []string{"/etc", "/var/www/celikpanel/subscriptions", "relative/dir", "/var/www/celikpanel/subscriptions/1/../../../../etc"} {
		if err := validAppWorkDir(dir); err == nil {
			t.Fatalf("%s was accepted", dir)
		}
	}
}
