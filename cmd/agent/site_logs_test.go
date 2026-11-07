package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G27): only the site's own rotated files older than the limit go;
// the live logs, other sites, links and other names stay.
func TestSiteLogRetentionRemovesOnlyOldRotatedFilesOfTheSite(t *testing.T) {
	dir := t.TempDir()
	savedDir, savedNow, savedRules := siteLogDir, siteLogNow, siteLogRotateRules
	siteLogDir = dir
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	siteLogNow = func() time.Time { return now }
	t.Cleanup(func() { siteLogDir, siteLogNow, siteLogRotateRules = savedDir, savedNow, savedRules })
	old := now.Add(-10 * 24 * time.Hour)
	files := map[string]time.Time{
		"shop.example-access.log":            old, // live: never
		"shop.example-error.log":             old, // live: never
		"shop.example-access.log.1":          old,
		"shop.example-access.log.2.gz":       old,
		"shop.example-error.log-20261020.gz": old,
		"shop.example-access.log.3.gz":       now.Add(-24 * time.Hour), // young
		"other.example-access.log.2.gz":      old,                      // other site
		"shop.example-access.log.bak":        old,                      // other name
		"access.log.1":                       old,
	}
	for name, mod := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(path, mod, mod)
	}
	_ = os.Symlink("/etc/hostname", filepath.Join(dir, "shop.example-access.log.4.gz"))
	var resp transport.SiteLogRetentionResponse
	_ = (&Agent{}).SiteLogRetentionV1(&transport.SiteLogRetentionRequest{Items: []transport.SiteLogRetentionItem{{Domain: "shop.example", KeepDays: 7}}}, &resp)
	if resp.Error != "" || resp.Removed["shop.example"] != 3 {
		t.Fatalf("response: %+v", resp)
	}
	for _, gone := range []string{"shop.example-access.log.1", "shop.example-access.log.2.gz", "shop.example-error.log-20261020.gz"} {
		if _, err := os.Lstat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s stayed", gone)
		}
	}
	for _, kept := range []string{"shop.example-access.log", "shop.example-error.log", "shop.example-access.log.3.gz",
		"other.example-access.log.2.gz", "shop.example-access.log.bak", "access.log.1", "shop.example-access.log.4.gz"} {
		if _, err := os.Lstat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("%s was removed", kept)
		}
	}

	rule := filepath.Join(t.TempDir(), "nginx")
	_ = os.WriteFile(rule, []byte("/var/log/nginx/*.log {\n\tdaily\n\tmissingok\n\trotate 14\n\tcompress\n}\n"), 0o644)
	siteLogRotateRules = []string{rule}
	var status transport.SiteLogStatusResponse
	_ = (&Agent{}).SiteLogStatusV1(&transport.SiteLogStatusRequest{Domain: "shop.example"}, &status)
	if !status.System.Found || status.System.Frequency != "daily" || status.System.Rotate != 14 || len(status.Files) != 3 {
		t.Fatalf("status: %+v", status)
	}
}
