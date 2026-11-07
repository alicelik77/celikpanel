package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 66 (alpha.128): site settings within limits. Additive: two new
// tables, NOT NULL columns with defaults and two rights items per listed
// set. A release before it refuses a ledger that carries it; going back is
// the pre-update snapshot restore (D-025).
const siteSettingLimitsMigrationVersion = 66

func TestOlderReleaseRefusesALedgerWithTheSiteSettingLimitsEntry(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	older := make([]embeddedMigration, 0, len(embedded))
	for _, migration := range embedded {
		if migration.version < siteSettingLimitsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, siteSettingLimitsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestSiteSettingLimitsMigrationSeedsTheOwnersCeilingsAndKeepsOldRows(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	want := map[string]int{"php.memory_limit_mb": 512, "php.max_execution_time_s": 300, "php.upload_max_mb": 256, "php.max_children": 20}
	for key, ceiling := range want {
		var got int
		if err := db.QueryRow(`SELECT server_ceiling FROM setting_policy WHERE key = ? AND server_default IS NULL AND customer_editable = 1`, key).Scan(&got); err != nil || got != ceiling {
			t.Fatalf("%s: ceiling %d, %v (want %d)", key, got, err, ceiling)
		}
	}
	// An older release's plan insert keeps working and gets today's rule.
	if _, err := db.Exec(`INSERT INTO service_plans (name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES ('Basic', 1, 1, 1, 100, 0)`); err != nil {
		t.Fatal(err)
	}
	var policy string
	var disk, traffic int
	if err := db.QueryRow(`SELECT overuse_policy, notify_percent_disk, notify_percent_traffic FROM service_plans WHERE name = 'Basic'`).Scan(&policy, &disk, &traffic); err != nil ||
		policy != "close" || disk != 80 || traffic != 80 {
		t.Fatalf("old-style plan: %q %d %d %v", policy, disk, traffic, err)
	}
	bad := []string{
		`UPDATE service_plans SET overuse_policy = 'ignore'`,
		`UPDATE service_plans SET notify_percent_disk = 99`,
		`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('team', 1, 'php.max_children', 2)`,
		`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 1, 'php.max_children', -1)`,
		`INSERT INTO setting_policy (key) VALUES ('NoDot')`,
		`INSERT INTO reseller_plans (name, overselling) VALUES ('R', 'maybe')`,
	}
	for _, statement := range bad {
		if _, err := db.Exec(statement); err == nil {
			t.Fatalf("accepted %s", statement)
		}
	}
	// A ceiling goes with its plan.
	var planID int
	if err := db.QueryRow(`SELECT id FROM service_plans WHERE name = 'Basic'`).Scan(&planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', ?, 'php.max_children', 5)`, planID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM service_plans WHERE id = ?`, planID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM setting_ceilings`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("ceilings left after the plan went: %d %v", left, err)
	}
}
