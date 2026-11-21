package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The owner's own link rule wins: when he set disable_symlinks for the site
// (his include directory, or the administrator's directives), the vhost leaves
// its line out, because nginx refuses the directive twice in one block.
func TestTenantIsolationOwnerSymlinkRuleIsKept(t *testing.T) {
	data := proofVhostVariants()["php https"]
	data.OwnerSymlinks = true
	if vhost := renderTestVhost(t, data); strings.Contains(vhost, "disable_symlinks") {
		t.Fatalf("the Panel's line was written beside the owner's:\n%s", vhost)
	}

	previous := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = previous })
	if OwnerSetsSymlinks("ornek.com", "") {
		t.Fatal("no include directory, no directives: nothing is the owner's")
	}
	if !OwnerSetsSymlinks("ornek.com", "disable_symlinks on;\nlocation /x { return 200; }") {
		t.Fatal("the administrator's server-level rule was not seen")
	}
	if OwnerSetsSymlinks("ornek.com", "location /x/ { disable_symlinks off; }") {
		t.Fatal("a location's rule does not clash with the server-level line")
	}
	dir := OwnerIncludeDir("ornek.com")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "links.conf"), []byte("# mine\ndisable_symlinks on from=$document_root;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !OwnerSetsSymlinks("ornek.com", "") {
		t.Fatal("the owner's include was not seen")
	}
	// A site setting file the owner edited is his; CelikPanel's own copy of a
	// customer's text is not (it is refused and removed in the same render).
	if err := os.Remove(filepath.Join(dir, "links.conf")); err != nil {
		t.Fatal(err)
	}
	part := SitePartPath("ornek.com", SiteDirectivesPartName)
	if err := os.MkdirAll(filepath.Dir(part), 0o755); err != nil {
		t.Fatal(err)
	}
	ours := SealManagedText(RenderSiteDirectivesPart("ornek.com", "disable_symlinks off;"))
	if err := os.WriteFile(part, []byte(ours), 0o640); err != nil {
		t.Fatal(err)
	}
	if OwnerSetsSymlinks("ornek.com", "") {
		t.Fatal("CelikPanel's copy of a customer's text was taken for the owner's rule")
	}
	if err := os.WriteFile(part, []byte(ours+"# edited by the owner\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if !OwnerSetsSymlinks("ornek.com", "") {
		t.Fatal("the owner's edited setting file was not seen")
	}
	if err := os.Remove(part); err != nil {
		t.Fatal(err)
	}
	// A file the directive parser does not take is read line by line.
	if err := os.WriteFile(filepath.Join(dir, "links.conf"), []byte("if ($x) {\n  disable_symlinks off;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !OwnerSetsSymlinks("ornek.com", "") {
		t.Fatal("an unparsable owner file must count, on the side of nginx accepting it")
	}
}

// Files written by earlier releases: only CelikPanel's unchanged files at the
// exact 0644 those releases wrote become 0640; the owner's files and modes
// are kept.
func TestTenantIsolationEarlierFilesAreTightenedOnce(t *testing.T) {
	previousNginx, previousPHP := nginxDir, phpEtcDir
	nginxDir, phpEtcDir = t.TempDir(), t.TempDir()
	t.Cleanup(func() { nginxDir, phpEtcDir = previousNginx, previousPHP })
	write := func(path, content string, mode os.FileMode) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	sealed := SealManagedText("server {}\n")
	ours := write(filepath.Join(nginxDir, "sites-available", "a.test.conf"), sealed, 0o644)
	pending := write(filepath.Join(nginxDir, "sites-available", "b.test.conf"+SiteFilePendingSuffix), sealed, 0o644)
	part := write(filepath.Join(nginxDir, panelManagedDirName, "a.test", "acme-http-01.conf"), SealManagedText("location / {}\n"), 0o644)
	pool := write(filepath.Join(phpEtcDir, "8.4", "fpm", "pool.d", "a_test.conf"), SealManagedTextWith(ManagedHeaderPrefixINI, "[a_test]\n"), 0o644)
	edited := write(filepath.Join(nginxDir, "sites-available", "c.test.conf"), sealed+"# my change\n", 0o644)
	ownerMode := write(filepath.Join(nginxDir, "sites-available", "d.test.conf"), sealed, 0o664)
	private := write(filepath.Join(nginxDir, "sites-available", "e.test.conf"), sealed, 0o600)
	notOurs := write(filepath.Join(nginxDir, "sites-available", "default.conf"), "server {}\n", 0o644)
	if err := os.Symlink(ours, filepath.Join(nginxDir, "sites-available", "f.test.conf")); err != nil {
		t.Fatal(err)
	}

	changes := TightenManagedSiteFileModes()
	mode := func(path string) os.FileMode {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	for _, path := range []string{ours, pending, part, pool} {
		if mode(path) != 0o640 {
			t.Errorf("%s is %04o, want 0640", path, mode(path))
		}
	}
	for path, want := range map[string]os.FileMode{edited: 0o644, ownerMode: 0o664, private: 0o600, notOurs: 0o644} {
		if mode(path) != want {
			t.Errorf("%s is %04o, the owner's %04o was not kept", path, mode(path), want)
		}
	}
	kept := 0
	for _, change := range changes {
		if change.Kept {
			kept++
			if change.Path != ownerMode {
				t.Errorf("unexpected kept entry %+v", change)
			}
		}
	}
	if kept != 1 || len(changes) != 5 {
		t.Fatalf("changes: %+v", changes)
	}
}

// A customer's directives the rules refuse are not written, and CelikPanel's
// earlier copy of them is removed so no part stays in force; an owner-edited
// copy is kept.
func TestTenantIsolationRefusedDirectivesLeaveNothingInForce(t *testing.T) {
	previous := nginxDir
	nginxDir = t.TempDir()
	t.Cleanup(func() { nginxDir = previous })
	domain := "ornek.com"
	path := SitePartPath(domain, SiteDirectivesPartName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	earlier := SealManagedText(RenderSiteDirectivesPart(domain, "location /x/ { disable_symlinks off; }"))
	if err := os.WriteFile(path, []byte(earlier), 0o644); err != nil {
		t.Fatal(err)
	}
	item := ManagedVhostItem{Domain: domain, SiteParts: []SitePart{{
		Name: SiteDirectivesPartName, Path: path,
		Body:    RenderSiteDirectivesPart(domain, "location /x/ { disable_symlinks off; }"),
		Refused: "line 1: disable_symlinks is not allowed here",
	}}}
	outcomes := map[string]string{}
	_, err := planSitePartsOf(item, outcomes, &webHTTPPlan{})
	if err == nil || !strings.Contains(err.Error(), "not written") {
		t.Fatalf("the refusal is not said: %v", err)
	}
	if outcomes[SiteDirectivesPartName] != transport.SitePartRefused {
		t.Fatalf("outcome %q", outcomes[SiteDirectivesPartName])
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("CelikPanel's copy of the refused text is still in force: %v", statErr)
	}

	// The owner's edited copy is his: kept.
	if err := os.WriteFile(path, []byte(earlier+"# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outcomes = map[string]string{}
	_, _ = planSitePartsOf(item, outcomes, &webHTTPPlan{})
	if outcomes[SiteDirectivesPartName] != transport.SiteFileChallengeKept {
		t.Fatalf("owner's copy: outcome %q", outcomes[SiteDirectivesPartName])
	}
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Fatalf("the owner's copy was removed: %v", statErr)
	}
}

// A PHP-FPM pool is created 0640; a systemd unit stays 0644.
func TestTenantIsolationPoolsArePrivateUnitsStayReadable(t *testing.T) {
	if mode := (ManagedSiteFile{Kind: transport.SiteFileKindPHPPool}).newFileMode(); mode != 0o640 {
		t.Fatalf("pool %04o", mode)
	}
	if mode := (ManagedSiteFile{Kind: transport.SiteFileKindAppUnit}).newFileMode(); mode != 0o644 {
		t.Fatalf("unit %04o", mode)
	}
}
