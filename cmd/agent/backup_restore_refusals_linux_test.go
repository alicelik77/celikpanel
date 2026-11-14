//go:build linux

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 lab findings. Defect 8: a restored program folder took the site
// home's group (the web server's) instead of the site account's own group.
// Defect 9: a backup that is not this site's, or a damaged one, answered
// "internal error" instead of saying what happened.

// The program's folder comes back with the site account's own group, as the
// Programs tab's deploy makes it, never with the site home's group (the web
// server's), and keeps its modes; public_html keeps its web group.
func TestProgramRestoreGivesTheSiteAccountsOwnGroupNotTheWebServers(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, docroot, map[string]string{"index.php": "site"})
	writeTree(t, home, map[string]string{"app/bin/sayac": "ELF", "app/data/ziyaret.txt": "91"})
	modes := map[string]os.FileMode{"app": 0o750, "app/bin": 0o750, "app/data": 0o750, "app/bin/sayac": 0o755, "app/data/ziyaret.txt": 0o600}
	for name, mode := range modes {
		if err := os.Chmod(filepath.Join(home, filepath.FromSlash(name)), mode); err != nil {
			t.Fatal(err)
		}
	}
	chownTree(t, home, testSiteUID) // skips without root
	// The lab's layout: the site home and public_html belong to the web
	// server's group, the program's tree to the account's own group.
	const siteGID, webGID = testSiteUID + 1, testSiteUID + 2
	for _, name := range []string{"", "public_html", "public_html/index.php"} {
		if err := os.Lchown(filepath.Join(home, filepath.FromSlash(name)), testSiteUID, webGID); err != nil {
			t.Fatal(err)
		}
	}
	for name := range modes {
		if err := os.Lchown(filepath.Join(home, filepath.FromSlash(name)), testSiteUID, siteGID); err != nil {
			t.Fatal(err)
		}
	}
	serviceAppLookupAccount = func(transport.ServiceAppTarget) (serviceAppAccount, error) {
		return serviceAppAccount{Name: "sayac_dr_test", Group: "sayac_dr_test", UID: testSiteUID, GID: siteGID, Home: home}, nil
	}
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	check := func(when string) {
		t.Helper()
		for name, mode := range modes {
			st, err := os.Lstat(filepath.Join(home, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("%s: %s: %v", when, name, err)
			}
			sys := st.Sys().(*syscall.Stat_t)
			if sys.Uid != testSiteUID || sys.Gid != siteGID {
				t.Fatalf("%s: %s is %d:%d, want the site account and its own group %d:%d", when, name, sys.Uid, sys.Gid, testSiteUID, siteGID)
			}
			if st.Mode().Perm() != mode {
				t.Fatalf("%s: %s mode %v, want %v", when, name, st.Mode().Perm(), mode)
			}
		}
		for _, name := range []string{"public_html", "public_html/index.php"} {
			st, err := os.Lstat(filepath.Join(home, filepath.FromSlash(name)))
			if err != nil || st.Sys().(*syscall.Stat_t).Gid != webGID {
				t.Fatalf("%s: %s lost the web server's group: %v", when, name, err)
			}
		}
	}
	if err := os.Remove(filepath.Join(home, "app", "data", "ziyaret.txt")); err != nil {
		t.Fatal(err)
	}
	resp := &backupspec.RestoreResponse{}
	if err := agent.restoreBackup(restoreRequest(info.Name), resp); err != nil {
		t.Fatal(err)
	}
	check("over the existing folder")
	// The folder gone entirely: the restore makes it, with the same owner.
	if err := os.RemoveAll(filepath.Join(home, "app")); err != nil {
		t.Fatal(err)
	}
	if err := agent.restoreBackup(restoreRequest(info.Name), &backupspec.RestoreResponse{}); err != nil {
		t.Fatal(err)
	}
	check("into a made folder")
}

// Without the site's account (missing, or not this site's) nothing is
// replaced and the refusal says so.
func TestProgramRestoreWithoutTheSiteAccountChangesNothing(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	home := filepath.Dir(docroot)
	writeTree(t, docroot, map[string]string{"index.html": "backed-up"})
	writeTree(t, home, map[string]string{"app/bin/server": "backed-up"})
	agent := &Agent{}
	info, err := agent.createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, docroot, map[string]string{"index.html": "live"})
	writeTree(t, home, map[string]string{"app/bin/server": "live"})
	serviceAppLookupAccount = func(transport.ServiceAppTarget) (serviceAppAccount, error) {
		return serviceAppAccount{}, errors.New("the site's system account example_test is missing")
	}
	resp := &backupspec.RestoreResponse{}
	if err := agent.RestoreBackup(restoreRequest(info.Name), resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || resp.Code != backupspec.RestoreCodeSiteAccount {
		t.Fatalf("answer = %#v", resp)
	}
	for _, name := range []string{"public_html/index.html", "app/bin/server"} {
		if data, _ := os.ReadFile(filepath.Join(home, filepath.FromSlash(name))); string(data) != "live" {
			t.Fatalf("%s was changed: %q", name, data)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(home, ".restore-*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging left behind: %v", leftovers)
	}
}

// otherScope is a second site of the test subscription.
func otherScope() backupScope {
	scope := testScope()
	scope.DomainID = 10
	scope.DomainName = "other.test"
	return scope
}

func inspectOn(t *testing.T, scope backupScope, name string) backupspec.InspectResponse {
	t.Helper()
	resp := backupspec.InspectResponse{}
	req := &backupspec.InspectRequest{ProtocolVersion: scope.ProtocolVersion, SubscriptionID: scope.SubscriptionID,
		DomainID: scope.DomainID, DomainName: scope.DomainName, BackupName: name}
	if err := (&Agent{}).InspectBackup(req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func restoreOn(t *testing.T, scope backupScope, name string) backupspec.RestoreResponse {
	t.Helper()
	resp := backupspec.RestoreResponse{}
	req := &backupspec.RestoreRequest{ProtocolVersion: scope.ProtocolVersion, SubscriptionID: scope.SubscriptionID,
		DomainID: scope.DomainID, DomainName: scope.DomainName, BackupName: name}
	if err := (&Agent{}).RestoreBackup(req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// Another site's backup named on this site (lab H.7, first attempt: this
// site had no backup folder yet), and a name gone from this site's folder:
// not found, nothing read or changed.
func TestAnotherSitesBackupNameIsNotFoundHere(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	writeTree(t, docroot, map[string]string{"index.html": "site"})
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	if inspected := inspectOn(t, otherScope(), info.Name); inspected.Success || inspected.Code != backupspec.RestoreCodeNotFound {
		t.Fatalf("inspect on a site without a backup folder = %#v", inspected)
	}
	if restored := restoreOn(t, otherScope(), info.Name); restored.Success || restored.Code != backupspec.RestoreCodeNotFound || restored.SafetyBackup != nil {
		t.Fatalf("restore on a site without a backup folder = %#v", restored)
	}
	// This site's folder exists, the backup was deleted from it.
	if err := os.Remove(info.Path); err != nil {
		t.Fatal(err)
	}
	if inspected := inspectOn(t, testScope(), info.Name); inspected.Code != backupspec.RestoreCodeNotFound {
		t.Fatalf("inspect of a deleted backup = %#v", inspected)
	}
	if restored := restoreOn(t, testScope(), info.Name); restored.Code != backupspec.RestoreCodeNotFound || restored.SafetyBackup != nil {
		t.Fatalf("restore of a deleted backup = %#v", restored)
	}
}

// Another site's package put into this site's backup folder (lab H.7, second
// attempt): refused as another site's, the site unchanged.
func TestAnotherSitesPackageInThisSitesFolderIsRefused(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	writeTree(t, docroot, map[string]string{"index.html": "theirs"})
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	// The second site shares the test's document root; its files say "ours".
	oldDocumentRoot := backupDocumentRoot
	backupDocumentRoot = func(int, int) (string, error) { return docroot, nil }
	t.Cleanup(func() { backupDocumentRoot = oldDocumentRoot })
	writeTree(t, docroot, map[string]string{"index.html": "ours"})
	dir, err := ensureBackupDir(otherScope())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, info.Name), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if inspected := inspectOn(t, otherScope(), info.Name); inspected.Success || inspected.Code != backupspec.RestoreCodeOtherSite {
		t.Fatalf("inspect = %#v", inspected)
	}
	restored := restoreOn(t, otherScope(), info.Name)
	if restored.Success || restored.Code != backupspec.RestoreCodeOtherSite || restored.SafetyBackup != nil {
		t.Fatalf("restore = %#v", restored)
	}
	if got, _ := os.ReadFile(filepath.Join(docroot, "index.html")); string(got) != "ours" {
		t.Fatalf("the site was changed: %q", got)
	}
}

// A package whose payload no longer matches its checksum (damaged on disk or
// on the way back from remote storage) is refused as failing its checks,
// before anything is replaced.
func TestDamagedPackageIsRefusedAsInvalid(t *testing.T) {
	_, docroot := installBackupTestPaths(t)
	writeTree(t, docroot, map[string]string{"index.html": "backed-up"})
	info, err := (&Agent{}).createBackup(testCreateRequest(backupspec.TypeFiles))
	if err != nil {
		t.Fatal(err)
	}
	corruptPayload(t, info.Path, filesPayloadName)
	writeTree(t, docroot, map[string]string{"index.html": "live"})
	restored := restoreOn(t, testScope(), info.Name)
	if restored.Success || restored.Code != backupspec.RestoreCodeInvalid {
		t.Fatalf("restore = %#v", restored)
	}
	if got, _ := os.ReadFile(filepath.Join(docroot, "index.html")); string(got) != "live" {
		t.Fatalf("the site was changed: %q", got)
	}
}

// corruptPayload rewrites a package with one byte of payload flipped (same
// size, so only its checksum tells).
func corruptPayload(t *testing.T, packagePath, payload string) {
	t.Helper()
	in, err := os.Open(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(in)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	tw := tar.NewWriter(zw)
	tr := tar.NewReader(gz)
	flipped := false
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == payload && len(data) > 20 {
			data[len(data)/2] ^= 0xff
			flipped = true
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	_ = in.Close()
	if !flipped {
		t.Fatalf("payload %s not found", payload)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packagePath, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
