package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134: a site deletion leaves nothing of the site behind: its crontab,
// its logs and its challenge folder go; its backups stay and are measured.

func writeCleanupTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func TestRemoveSiteLogsTakesTheSiteLogsAndTheirRotationsOnly(t *testing.T) {
	dir := t.TempDir()
	saved := siteNginxLogDir
	siteNginxLogDir = dir
	t.Cleanup(func() { siteNginxLogDir = saved })
	for _, name := range []string{
		"example.test-access.log", "example.test-access.log.1", "example.test-access.log.2.gz",
		"example.test-access.log-20261112", "example.test-access.log-20261111.gz",
		"example.test-error.log", "example.test-error.log.1",
		// Another site whose name starts like this one's, and nginx's own logs.
		"example.test-access.log.tr-access.log", "other.test-access.log", "access.log", "error.log",
	} {
		writeCleanupTestFile(t, filepath.Join(dir, name), "line\n")
	}
	if err := removeSiteLogs("example.test", 0); err != nil {
		t.Fatal(err)
	}
	want := []string{"access.log", "error.log", "example.test-access.log.tr-access.log", "other.test-access.log"}
	if got := dirNames(t, dir); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("left = %v, want %v", got, want)
	}
	// Absent logs are done.
	if err := removeSiteLogs("example.test", 0); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

func TestRemoveLogFamilyRefusesALinkAndNamesIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	writeCleanupTestFile(t, target, "keep")
	if err := os.Symlink(target, filepath.Join(dir, "7.log")); err != nil {
		t.Skip("symbolic links are not available here")
	}
	writeCleanupTestFile(t, filepath.Join(dir, "7.log.1"), "old")
	writeCleanupTestFile(t, filepath.Join(dir, "70.log"), "another site")
	err := removeLogFamily(dir, "7.log")
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(dirNames(t, dir), ","); got != "7.log,70.log" {
		t.Fatalf("left = %s", got)
	}
	if content, _ := os.ReadFile(target); string(content) != "keep" {
		t.Fatal("the link's target was touched")
	}
}

func TestRemoveCrontabSpoolFilesRemovesEveryKnownPlace(t *testing.T) {
	root := t.TempDir()
	saved := siteCronSpoolPaths
	siteCronSpoolPaths = func(username string) []string {
		return []string{filepath.Join(root, "crontabs", username), filepath.Join(root, username)}
	}
	t.Cleanup(func() { siteCronSpoolPaths = saved })
	writeCleanupTestFile(t, filepath.Join(root, "crontabs", "example_test"), "* * * * * true\n")
	writeCleanupTestFile(t, filepath.Join(root, "example_test"), "* * * * * true\n")
	writeCleanupTestFile(t, filepath.Join(root, "crontabs", "other_test"), "* * * * * true\n")
	if err := removeCrontabSpoolFiles("example_test"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{filepath.Join(root, "crontabs", "example_test"), filepath.Join(root, "example_test")} {
		if _, err := os.Lstat(gone); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still there", gone)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "crontabs", "other_test")); err != nil {
		t.Fatal("another account's crontab was touched")
	}
	if err := removeCrontabSpoolFiles("example_test"); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

func TestRemoveSiteChallengeRootRemovesTheFolderAndRefusesALink(t *testing.T) {
	root := t.TempDir()
	saved := siteChallengeRoot
	siteChallengeRoot = func(subscriptionID, domainID int) (string, error) {
		return filepath.Join(root, "subscriptions", "7", "domains", "11"), nil
	}
	t.Cleanup(func() { siteChallengeRoot = saved })
	writeCleanupTestFile(t, filepath.Join(root, "subscriptions", "7", "domains", "11", ".well-known", "acme-challenge", "token"), "x")
	if err := removeSiteChallengeRoot(7, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "subscriptions", "7", "domains", "11")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("challenge folder kept")
	}
	if _, err := os.Lstat(filepath.Join(root, "subscriptions", "7", "domains")); err != nil {
		t.Fatal("the subscription's folder was removed")
	}
	if err := removeSiteChallengeRoot(7, 11); err != nil {
		t.Fatalf("second run: %v", err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "subscriptions", "7", "domains", "11")); err != nil {
		t.Skip("symbolic links are not available here")
	}
	if err := removeSiteChallengeRoot(7, 11); err == nil {
		t.Fatal("a link in place of the folder was accepted")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("the link's target was touched")
	}
}

func TestMeasureKeptBackupsSaysWhereHowManyAndHowLarge(t *testing.T) {
	root := t.TempDir()
	saved := siteBackupDir
	siteBackupDir = func(subscriptionID, domainID int) (string, error) {
		return filepath.Join(root, "subscriptions", "7", "domains", "11"), nil
	}
	t.Cleanup(func() { siteBackupDir = saved })
	if kept := measureKeptBackups(7, 11); kept != nil {
		t.Fatalf("no backups measured as %+v", kept)
	}
	writeCleanupTestFile(t, filepath.Join(root, "subscriptions", "7", "domains", "11", "a.tar.gz"), "12345")
	writeCleanupTestFile(t, filepath.Join(root, "subscriptions", "7", "domains", "11", "b", "c.json"), "123")
	kept := measureKeptBackups(7, 11)
	if kept == nil || kept.Files != 2 || kept.Bytes != 8 || kept.Unknown ||
		kept.Path != filepath.Join(root, "subscriptions", "7", "domains", "11") {
		t.Fatalf("kept = %+v", kept)
	}
	// Measuring removed nothing.
	if kept := measureKeptBackups(7, 11); kept == nil || kept.Files != 2 {
		t.Fatalf("second measure = %+v", kept)
	}
}

func TestDeleteSiteRemovesCrontabLogsAndChallengeAndMeasuresBackups(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleDeleteRequest(t)
	fake := &fakeSiteLifecycle{home: req.SiteHome, userExists: true}
	ops := fake.operations()
	ops.removeCrontab = func(username string) error {
		if username != req.Username {
			t.Fatalf("crontab of %q", username)
		}
		return fake.call("remove-crontab")
	}
	ops.removeSiteLogs = func(domain string, domainID int) error {
		if domain != req.Domain || domainID != req.DomainID {
			t.Fatalf("logs of %s/%d", domain, domainID)
		}
		return fake.call("remove-logs")
	}
	ops.removeChallengeRoot = func(subscriptionID, domainID int) error {
		if subscriptionID != req.SubscriptionID || domainID != req.DomainID {
			t.Fatalf("challenge of %d/%d", subscriptionID, domainID)
		}
		return fake.call("remove-challenge")
	}
	ops.measureBackups = func(int, int) *transport.KeptBackups {
		return &transport.KeptBackups{Path: "/var/backups/x", Files: 3, Bytes: 42}
	}
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = ops
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success || resp.KeptBackups == nil || resp.KeptBackups.Files != 3 {
		t.Fatalf("response = %#v", resp)
	}
	calls := strings.Join(fake.calls, ",")
	crontab, user := strings.Index(calls, "remove-crontab"), strings.Index(calls, "delete-user")
	if crontab < 0 || user < 0 || crontab > user {
		t.Fatalf("the crontab must go before the account: %s", calls)
	}
	if !strings.Contains(calls, "remove-logs") || !strings.Contains(calls, "remove-challenge") {
		t.Fatalf("calls = %s", calls)
	}
}

func TestDeleteSiteNamesAScheduledTaskFailureAndStaysRetryable(t *testing.T) {
	withLifecycleTestBuild(t)
	req := validLifecycleDeleteRequest(t)
	fake := &fakeSiteLifecycle{
		home: req.SiteHome, userExists: true,
		failures: map[string]error{"remove-crontab": errors.New("crontab: private detail")},
	}
	ops := fake.operations()
	ops.removeCrontab = func(string) error { return fake.call("remove-crontab") }
	ops.removeSiteLogs = func(string, int) error { return fake.call("remove-logs") }
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps = ops
	resp := &DeleteSiteResponse{}
	if err := agent.DeleteSite(&req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || !strings.Contains(resp.Error, "scheduled tasks") || strings.Contains(resp.Error, "private detail") {
		t.Fatalf("response = %#v", resp)
	}
	// The rest still ran, so a retry has less to do.
	if !strings.Contains(strings.Join(fake.calls, ","), "remove-logs") {
		t.Fatalf("calls = %v", fake.calls)
	}
}
