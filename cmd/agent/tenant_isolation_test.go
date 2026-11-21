//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
)

// A file a customer's directive names must be a regular file reached without
// a link: nginx's worker reads it with the web server's rights at every
// request, and the "inside the site's home" check is only lexical.
func TestTenantIsolationDirectiveFileMustNotBeALink(t *testing.T) {
	home := t.TempDir()
	documentRoot := filepath.Join(home, "public_html")
	if err := os.MkdirAll(documentRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(home, ".htpasswd")
	if err := os.WriteFile(real, []byte("anna:$2y$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyzABCDEF\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "another-site.htpasswd")
	if err := os.WriteFile(foreign, []byte("bob:x\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "linked.htpasswd")
	if err := os.Symlink(foreign, link); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(home, "conf")
	if err := os.Symlink(filepath.Dir(foreign), linkedDir); err != nil {
		t.Fatal(err)
	}
	text := func(file string) string {
		return "location /p { auth_basic \"x\"; auth_basic_user_file " + file + "; }"
	}
	// The lexical rule alone takes all three (it is what the Panel checks).
	for _, file := range []string{real, link, filepath.Join(linkedDir, filepath.Base(foreign))} {
		if err := services.ValidateSiteDirectives(text(file), false, home); err != nil {
			t.Fatalf("%s: lexical check: %v", file, err)
		}
	}
	if err := customerDirectivesRefusal(text(real), documentRoot); err != nil {
		t.Fatalf("the site's own file was refused: %v", err)
	}
	for _, file := range []string{link, filepath.Join(linkedDir, filepath.Base(foreign))} {
		if err := customerDirectivesRefusal(text(file), documentRoot); err == nil {
			t.Errorf("%s reaches another file through a link and was taken", file)
		}
	}
	// The administrator's text is not checked against the forbidden list.
	if err := customerDirectivesRefusal("location /x/ { disable_symlinks off; }", documentRoot); err == nil {
		t.Fatal("a customer's text lifting the link rule was taken")
	}
}

// The one-time correction: CelikPanel's per-site files and the hosting base at
// the earlier modes are tightened once; a receipt keeps a later owner's choice
// from being changed back.
func TestTenantIsolationModesAreTightenedOnceThenLeftToTheOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the correction acts on root-owned directories; run as root")
	}
	dir := t.TempDir()
	receipt := filepath.Join(dir, "state", tenantFileModesReceiptName)
	if err := os.MkdirAll(filepath.Dir(receipt), 0o700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "celikpanel")
	subscriptions := filepath.Join(base, "subscriptions")
	ownerDir := filepath.Join(dir, "owners")
	for path, mode := range map[string]os.FileMode{base: 0o755, subscriptions: 0o755, ownerDir: 0o775} {
		if err := os.MkdirAll(path, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	files := func() []services.ManagedFileModeChange {
		return []services.ManagedFileModeChange{{Path: "/etc/nginx/sites-available/a.test.conf"},
			{Path: "/etc/nginx/sites-available/b.test.conf", Kept: true, Why: "mode 0664 is the owner's"}}
	}
	now := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC) }
	lines, ran := tightenTenantVisibleModes(receipt, files, []string{base, subscriptions, ownerDir}, now)
	if !ran {
		t.Fatalf("did not run: %v", lines)
	}
	mode := func(path string) os.FileMode {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	if mode(base) != 0o711 || mode(subscriptions) != 0o711 || mode(ownerDir) != 0o775 {
		t.Fatalf("modes base %04o subscriptions %04o owner's %04o\n%s", mode(base), mode(subscriptions), mode(ownerDir), strings.Join(lines, "\n"))
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"0644 -> 0640 /etc/nginx/sites-available/a.test.conf", "kept /etc/nginx/sites-available/b.test.conf",
		"0755 -> 0711 " + base, "kept " + ownerDir, "3 changed, 2 kept"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log lacks %q:\n%s", want, joined)
		}
	}
	// The owner opens the base again; the next start leaves it.
	if err := os.Chmod(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ran := tightenTenantVisibleModes(receipt, func() []services.ManagedFileModeChange {
		t.Fatal("the site files were looked at again")
		return nil
	}, []string{base}, now); ran || mode(base) != 0o755 {
		t.Fatalf("the correction ran again (ran %v, base %04o)", ran, mode(base))
	}
}
