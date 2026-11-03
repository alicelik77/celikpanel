package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 57 (alpha.118, D-044, owner decisions of 3 November 2026): plan
// limits made real. New tables, three new columns and one deliberate value
// change: the never-applied traffic limits become 0 (unlimited).
const usageLimitsMigrationVersion = 57

func TestUsageLimitsSchema(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	for _, good := range []string{
		`INSERT INTO traffic_daily (domain_id, day, subscription_id, bytes_out) VALUES (1, '2026-11-03', 2, 100)`,
		`INSERT INTO traffic_web_blocks (subscription_id, period, cause) VALUES (2, '2026-11', 'pool')`,
		`INSERT INTO subscription_disk_usage (subscription_id, web_bytes, state) VALUES (2, 10, 'measured')`,
		`INSERT INTO usage_notices (user_id, audience, kind, subscription_id, period) VALUES (3, 'customer', 'disk_100', 2, 'x')`,
		`INSERT INTO traffic_sources (domain_id, state, reason) VALUES (1, 'unknown', 'vhost_kept')`,
		`UPDATE traffic_collect_state SET agent_epoch = 'e', last_seq = 4 WHERE id = 1`,
	} {
		if _, err := raw.Exec(good); err != nil {
			t.Fatalf("refused %s: %v", good, err)
		}
	}
	for _, bad := range []string{
		// One row per website, day and source: the collector adds to it.
		`INSERT INTO traffic_daily (domain_id, day, subscription_id) VALUES (1, '2026-11-03', 2)`,
		`INSERT INTO traffic_daily (domain_id, day, subscription_id, bytes_out) VALUES (1, '2026-11-04', 2, -1)`,
		`INSERT INTO traffic_web_blocks (subscription_id, period, cause) VALUES (3, '2026-11', 'manual')`,
		`INSERT INTO subscription_disk_usage (subscription_id, state) VALUES (3, 'guess')`,
		`INSERT INTO usage_notices (user_id, audience, kind, subscription_id, period) VALUES (3, 'customer', 'disk_100', 2, 'x')`,
		`INSERT INTO usage_notices (user_id, audience, kind, period) VALUES (3, 'customer', 'mail_full', 'x')`,
		`INSERT INTO traffic_collect_state (id) VALUES (2)`,
		`INSERT INTO reseller_pools (reseller_id, traffic_quota_mb) VALUES (9, -1)`,
	} {
		if _, err := raw.Exec(bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
	var cause int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('subscription_suspensions') WHERE name = 'cause_disk'`).Scan(&cause); err != nil || cause != 1 {
		t.Fatalf("subscription_suspensions.cause_disk = %d, %v", cause, err)
	}
}

func TestUsageLimitsMigrationSetsTheNeverAppliedTrafficLimitsToUnlimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel56.sqlite")
	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var older []embeddedMigration
	for _, migration := range embedded {
		if migration.version < usageLimitsMigrationVersion {
			older = append(older, migration)
		}
	}
	for _, migration := range older {
		if _, err := raw.Exec(string(migration.content)); err != nil {
			t.Fatalf("%s: %v", migration.filename, err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (700, 'boss', 'h', 'boss@example.test', 'admin');
		INSERT INTO users (id, username, password_hash, email, role) VALUES (701, 'cust', 'h', 'cust@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name, bandwidth_quota_mb) VALUES (710, 701, 'old', 102400);
		INSERT INTO service_plans (id, name, bandwidth_quota_mb) VALUES (720, 'old-plan', 51200);
	`); err != nil {
		t.Fatal(err)
	}
	var migration embeddedMigration
	for _, m := range embedded {
		if m.version == usageLimitsMigrationVersion {
			migration = m
		}
	}
	if _, err := raw.Exec(string(migration.content)); err != nil {
		t.Fatal(err)
	}
	var sub, plan int
	if err := raw.QueryRow(`SELECT bandwidth_quota_mb FROM subscriptions WHERE id = 710`).Scan(&sub); err != nil || sub != 0 {
		t.Fatalf("subscription traffic limit = %d, %v; want 0", sub, err)
	}
	if err := raw.QueryRow(`SELECT bandwidth_quota_mb FROM service_plans WHERE id = 720`).Scan(&plan); err != nil || plan != 0 {
		t.Fatalf("plan traffic limit = %d, %v; want 0", plan, err)
	}
	var notices int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM rights_notices WHERE kind = 'traffic_measured' AND user_id = 700`).Scan(&notices); err != nil || notices != 1 {
		t.Fatalf("administrator notices = %d, %v; want 1", notices, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM rights_notices WHERE kind = 'traffic_measured' AND user_id = 701`).Scan(&notices); err != nil || notices != 0 {
		t.Fatalf("customer notices = %d, %v; want 0", notices, err)
	}
}

func TestOlderReleaseRefusesALedgerWithTheUsageLimitsEntry(t *testing.T) {
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
		if migration.version < usageLimitsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, usageLimitsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
