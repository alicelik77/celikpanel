//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 audit: root work inside a folder the site's account owns must
// never be redirected by a link the account plants or swaps in.

// The restore staging folder lives in the site's home, which the site's
// account may rename entries in. After extraction it swaps the folder for a
// link to its own folder whose entries are links to root's files; the owner
// and mode steps must still land only on the real staged tree.
func TestRestoreStagingIgnoresASwappedFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "home", "public_html")
	for _, dir := range []string{filepath.Join(source, "x"), target} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "big"), []byte("data"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(source, "big"), 0o666); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "files.tar.gz")
	if err := safeCreateFilesArchive(source, archive); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, "victim-shadow")
	if err := os.WriteFile(victim, []byte("root only"), 0o600); err != nil {
		t.Fatal(err)
	}
	victimDir := filepath.Join(root, "victim-etc")
	if err := os.Mkdir(victimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	attacker := filepath.Join(root, "attacker")
	if err := os.Mkdir(attacker, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(attacker, "big")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victimDir, filepath.Join(attacker, "x")); err != nil {
		t.Fatal(err)
	}
	swapped := false
	staged, err := prepareFilesRestoreEdited(archive, target, func(int) error {
		matches, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".restore-stage-*"))
		if len(matches) != 1 {
			t.Fatalf("staging folders: %v", matches)
		}
		if err := os.Rename(matches[0], matches[0]+".moved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(attacker, matches[0]); err != nil {
			t.Fatal(err)
		}
		swapped = true
		return nil
	})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer staged.Cleanup()
	if !swapped {
		t.Fatal("the swap did not run")
	}
	for path, mode := range map[string]os.FileMode{victim: 0o600, victimDir: 0o755} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("%s was changed through the swapped folder: %v %v", path, info.Mode(), err)
		}
	}
	moved, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".restore-stage-*.moved", "big"))
	if len(moved) != 1 {
		t.Fatalf("the real staged file is missing: %v", moved)
	}
	if info, err := os.Lstat(moved[0]); err != nil || info.Mode().Perm() != 0o666 {
		t.Fatalf("the real staged file did not get its mode: %v %v", info, err)
	}
}

// The archive walk holds a descriptor per folder: a folder that is a link
// is refused, never followed into root's files.
func TestFilesArchiveRefusesALinkedFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	secret := filepath.Join(root, "secret")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "shadow"), []byte("root:hash"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(source, "a")); err != nil {
		t.Fatal(err)
	}
	if err := safeCreateFilesArchive(source, filepath.Join(root, "out.tar.gz")); err == nil {
		t.Fatal("a linked folder was archived")
	}
	if _, err := os.Lstat(filepath.Join(root, "out.tar.gz")); !os.IsNotExist(err) {
		t.Fatal("a refused archive was left behind")
	}
}

// The clone rewrites wp-config.php through the staging folder's descriptor;
// a link in its place is refused and its target untouched.
func TestStagedWordPressConfigRefusesALink(t *testing.T) {
	stage := t.TempDir()
	victim := filepath.Join(t.TempDir(), "other-wp-config.php")
	original := "<?php define( 'DB_NAME', 'victim' ); define( 'DB_USER', 'v' ); define( 'DB_PASSWORD', 'p' );\n"
	if err := os.WriteFile(victim, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(stage, "wp-config.php")); err != nil {
		t.Fatal(err)
	}
	fd, err := openDirectoryNoFollow(stage)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	req := &transport.WordPressCloneRequest{TargetDatabase: "copy", TargetDBUser: "copy", TargetDBPass: "secret"}
	if err := rewriteStagedWordPressConfig(fd, req); err == nil {
		t.Fatal("a linked wp-config.php was rewritten")
	}
	if raw, _ := os.ReadFile(victim); string(raw) != original {
		t.Fatal("the link's target was changed")
	}
	// A regular file is rewritten in place (0600).
	if err := os.Remove(filepath.Join(stage, "wp-config.php")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "wp-config.php"), []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := rewriteStagedWordPressConfig(fd, req); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(stage, "wp-config.php"))
	info, _ := os.Lstat(filepath.Join(stage, "wp-config.php"))
	if !strings.Contains(string(raw), "'copy'") || info.Mode().Perm() != 0o600 {
		t.Fatalf("rewritten: %q %v", raw, info.Mode())
	}
}

// configHolds reads beneath the document root: no link at any component,
// no FIFO (non-blocking) and nothing over the bound.
func TestReadRegularBeneathRefusesLinksFIFOsAndLargeFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "config.php"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularBeneath(root, "config/config.php", 1<<20); err == nil {
		t.Fatal("a linked folder was followed")
	}
	if err := os.Remove(filepath.Join(root, "config")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "config", "config.php"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readRegularBeneath(root, "config/config.php", 1<<20)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a FIFO was read")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a FIFO hung the reader")
	}
	if err := os.Remove(filepath.Join(root, "config", "config.php")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.php"), []byte(strings.Repeat("x", 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularBeneath(root, "config/config.php", 10); err == nil {
		t.Fatal("a file over the bound was read")
	}
	if got, err := readRegularBeneath(root, "config/config.php", 1<<20); err != nil || len(got) != 100 {
		t.Fatalf("a regular file: %d %v", len(got), err)
	}
}

// The quota warning's hourly scan runs as root in a folder vmail owns: the
// marker is made with mkdir, so a planted link creates or truncates nothing.
func TestQuotaWarningMarkerNeverFollowsALink(t *testing.T) {
	state := t.TempDir()
	script := filepath.Join(t.TempDir(), "warn.sh")
	if err := os.WriteFile(script, []byte(sealQuotaWarning("#!/bin/sh\n", quotaWarningScriptBody(true, "/bin/true", "", "", state))), 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "nologin")
	if err := os.Symlink(victim, filepath.Join(state, "box@ex.test")); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(t.TempDir(), "shadow")
	if err := os.WriteFile(existing, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(existing, filepath.Join(state, "two@ex.test")); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"box@ex.test", "two@ex.test", "new@ex.test"} {
		if out, err := exec.Command("/bin/sh", script, "warn", "95", user).CombinedOutput(); err != nil {
			t.Fatalf("warn %s: %v %s", user, err, out)
		}
	}
	if _, err := os.Lstat(victim); !os.IsNotExist(err) {
		t.Fatal("a dangling link's target was created")
	}
	if raw, _ := os.ReadFile(existing); string(raw) != "keep" {
		t.Fatal("a linked file was truncated")
	}
	if info, err := os.Lstat(filepath.Join(state, "new@ex.test")); err != nil || !info.IsDir() {
		t.Fatalf("no marker for a new warning: %v", err)
	}
}
