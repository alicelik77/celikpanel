//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// S4 R1, second fix round (B2): with webmail on its own account, a website
// account reaches nothing under Roundcube's tree: not the configuration, not
// a nested plugin configuration, not a .bak or ~ copy, not a plugin folder
// that is a link to elsewhere, not even the published code. Webmail's own
// account (by the root's access-control-list entry) reads its code and
// configuration and writes its private folders; the web server (the root's
// group) reads the published static files through public_html's links.
// Real files, real owners, real access checks: root only, on a file system
// with access-control lists (the guest's own). Fails on 193b87001, where the
// root let every account through (o=x) and these files were readable.
func TestASiteAccountReachesNothingUnderWebmailsTree(t *testing.T) {
	webGID := webServerGID()
	if os.Geteuid() != 0 || webGID < 0 {
		t.Skip("needs root and a web server group")
	}
	parent := t.TempDir()
	// t.TempDir's own two folders (<temp>/<test name><n>/<nnn>) may be 0700:
	// let the test's accounts walk to the tree's parent (never touching the
	// system's temporary folder itself).
	for _, dir := range []string{parent, filepath.Dir(parent)} {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	base := filepath.Join(parent, "celikpanel-webmail")
	outside := filepath.Join(parent, "elsewhere")
	files := map[string]os.FileMode{
		"program/include/iniset.php":            0o644,
		"skins/elastic/styles/styles.min.css":   0o644,
		"public_html/index.php":                 0o644,
		"config/config.inc.php":                 0o640,
		"db/roundcube.sqlite3":                  0o660,
		"temp/upload":                           0o640,
		"logs/errors.log":                       0o640,
		"plugins/x/config/config.inc.php":       0o644,
		"plugins/password/config.inc.php.bak":   0o644,
		"plugins/password/config.inc.php~":      0o644,
		"plugins/password/config.inc.php":       0o640,
		"plugins/managesieve/config.inc.php":    0o644,
		"../elsewhere/secret/config.inc.php":    0o644,
		"plugins/enigma/home/keys/private.asc":  0o644,
		"plugins/markasjunk/credentials.php":    0o644,
		"plugins/archive/config.php.old":        0o644,
		"plugins/zipdownload/localization/a.js": 0o644,
	}
	for rel, mode := range files {
		path := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("secret"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(base, "plugins", "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../skins", filepath.Join(base, "public_html", "skins")); err != nil {
		t.Fatal(err)
	}
	// As the install leaves it: root:<web group>, the root 0750, the private
	// folders the web group's.
	if out, err := exec.Command("chown", "-R", "root:"+webServerGroup(), base).CombinedOutput(); err != nil {
		t.Fatalf("chown: %v %s", err, out)
	}
	for _, rel := range []string{".", "db", "temp", "logs"} {
		mode := os.FileMode(0o770)
		if rel == "." {
			mode = 0o750
		}
		if err := os.Chmod(filepath.Join(base, rel), mode); err != nil {
			t.Fatal(err)
		}
	}
	const account, site = 4242, 4300
	if err := applyRoundcubeOwnAccountPermissions(base, account, account); err != nil {
		t.Fatal(err)
	}

	reads := func(uid, gid int, path string) bool {
		command := exec.Command("/bin/cat", path)
		command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{uint32(gid)}}}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Logf("uid %d: %v %s", uid, err, out)
		}
		return err == nil
	}
	for rel := range files {
		if strings.HasPrefix(rel, "../") {
			continue
		}
		if reads(site, site, filepath.Join(base, rel)) {
			t.Errorf("a website account reads %s", rel)
		}
	}
	if reads(site, site, filepath.Join(base, "plugins", "linked", "config.inc.php")) {
		t.Error("a website account reads a linked plugin folder's configuration")
	}
	for _, rel := range []string{"program/include/iniset.php", "config/config.inc.php", "plugins/managesieve/config.inc.php", "plugins/password/config.inc.php"} {
		if !reads(account, account, filepath.Join(base, rel)) {
			t.Errorf("webmail's own account cannot read %s", rel)
		}
	}
	touch := exec.Command("/usr/bin/touch", filepath.Join(base, "db", "roundcube.sqlite3-journal"))
	touch.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: account, Gid: account, Groups: []uint32{account}}}
	if out, err := touch.CombinedOutput(); err != nil {
		t.Errorf("webmail's own account cannot write db/: %v %s", err, out)
	}
	web := webGID
	if !reads(33, web, filepath.Join(base, "public_html", "skins", "elastic", "styles", "styles.min.css")) {
		t.Error("the web server cannot read the published styles through public_html")
	}
	if reads(33, web, filepath.Join(base, "config", "config.inc.php")) {
		t.Error("the web server's account still reads webmail's configuration")
	}
	info, err := os.Stat(base)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o007 != 0 {
		t.Fatalf("the tree's root is %04o: open to others", info.Mode().Perm())
	}
}
