//go:build linux

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// S4 R1 fix round, finding 5: phpMyAdmin already on its own account. A
// package upgrade (dbconfig-common) gives config-db.php back to the web
// group (root:<web group> 0640); the next start's check gives it to the
// account's group again, and a file the owner set otherwise is kept. Root
// only (it changes groups).
func TestPHPMyAdminFilesGivenBackToTheWebGroupArePutRight(t *testing.T) {
	webGID := webServerGID()
	if os.Geteuid() != 0 || webGID < 0 {
		t.Skip("needs root and a web server group")
	}
	dir := t.TempDir()
	saved := []string{phpMyAdminMainConfig, phpMyAdminConfDir, phpMyAdminOwnAccountConf, phpMyAdminDBConfig, phpMyAdminBlowfishSecret, phpToolFilesRecordDir}
	t.Cleanup(func() {
		phpMyAdminMainConfig, phpMyAdminConfDir, phpMyAdminOwnAccountConf, phpMyAdminDBConfig, phpMyAdminBlowfishSecret, phpToolFilesRecordDir = saved[0], saved[1], saved[2], saved[3], saved[4], saved[5]
	})
	phpMyAdminConfDir = filepath.Join(dir, "conf.d")
	phpMyAdminMainConfig = filepath.Join(dir, "config.inc.php")
	phpMyAdminOwnAccountConf = filepath.Join(phpMyAdminConfDir, "celikpanel-own-account.php")
	phpMyAdminDBConfig = filepath.Join(dir, "config-db.php")
	phpMyAdminBlowfishSecret = filepath.Join(dir, "blowfish_secret.inc.php")
	phpToolFilesRecordDir = filepath.Join(dir, "private")
	if err := os.WriteFile(phpMyAdminMainConfig, []byte("<?php foreach (glob('"+phpMyAdminConfDir+"/*.php') as $f) include $f;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const toolGID = 4243
	for path, gid := range map[string]int{phpMyAdminDBConfig: webGID, phpMyAdminBlowfishSecret: 0} {
		if err := os.WriteFile(path, []byte("<?php\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(path, 0, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := preparePHPMyAdminOwnAccount(4242, toolGID, true); err != nil {
		t.Fatal(err)
	}
	group := func(path string) int {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return int(info.Sys().(*syscall.Stat_t).Gid)
	}
	if group(phpMyAdminDBConfig) != toolGID {
		t.Fatal("config-db.php given back to the web group was not put right")
	}
	if group(phpMyAdminBlowfishSecret) != 0 {
		t.Fatal("a file the owner set otherwise was changed")
	}
}
