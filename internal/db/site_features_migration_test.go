package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 47 (alpha.114): four new tables (a site's password-protected
// folders and their users, its redirects, and an account's two-factor recovery
// codes). Nothing existing is changed, so the upgrade from 46 keeps every row;
// the release before it (schema 46) refuses a ledger that carries entry 47, and
// a return to it is the pre-update snapshot restore, as for every migration.
const siteFeaturesMigrationVersion = 47

func TestMigration47AddsTheSiteFeatureAndRecoveryCodeTables(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sqlDB := database.GetDB()
	for _, statement := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (501, 'owner', 'x', 'o@example.test', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (502, 501, 'plan')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (503, 502, 'example.test')`,
		`INSERT INTO sites (id, domain_id, document_root) VALUES (504, 503, '/srv/example.test')`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	hash := "$2y$10$" + strings.Repeat("a", 53)
	good := []string{
		`INSERT INTO site_protected_dirs (id, site_id, path) VALUES (1, 504, '/private')`,
		`INSERT INTO site_protected_users (dir_id, username, password_hash) VALUES (1, 'alice', '` + hash + `')`,
		`INSERT INTO site_redirects (site_id, match_type, source_path, target_url, status_code) VALUES (504, 'exact', '/old', '/new', 301)`,
		`INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (501, '` + strings.Repeat("a", 64) + `')`,
	}
	for _, statement := range good {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	bad := map[string]string{
		"a folder path without a slash":     `INSERT INTO site_protected_dirs (site_id, path) VALUES (504, 'private')`,
		"the same folder twice":             `INSERT INTO site_protected_dirs (site_id, path) VALUES (504, '/private')`,
		"a folder of no site":               `INSERT INTO site_protected_dirs (site_id, path) VALUES (9999, '/x')`,
		"a user twice":                      `INSERT INTO site_protected_users (dir_id, username, password_hash) VALUES (1, 'alice', '` + hash + `')`,
		"a hash of the wrong length":        `INSERT INTO site_protected_users (dir_id, username, password_hash) VALUES (1, 'bob', 'plain')`,
		"an unknown match type":             `INSERT INTO site_redirects (site_id, match_type, source_path, target_url, status_code) VALUES (504, 'regex', '/a', '/b', 301)`,
		"a status code that is no redirect": `INSERT INTO site_redirects (site_id, match_type, source_path, target_url, status_code) VALUES (504, 'exact', '/a', '/b', 200)`,
		"the same redirect twice":           `INSERT INTO site_redirects (site_id, match_type, source_path, target_url, status_code) VALUES (504, 'exact', '/old', '/other', 302)`,
		"a code digest of the wrong length": `INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (501, 'short')`,
		"the same code digest twice":        `INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (501, '` + strings.Repeat("a", 64) + `')`,
		"a code of no account":              `INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (9999, '` + strings.Repeat("b", 64) + `')`,
	}
	for name, statement := range bad {
		if _, err := sqlDB.Exec(statement); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
	// Removing a site removes its folders, their users and its redirects; an
	// account's codes go with the account.
	if _, err := sqlDB.Exec(`DELETE FROM sites WHERE id = 504`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO users (id, username, password_hash, email, role) VALUES (601, 'other', 'x', 'p@example.test', 'customer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO two_factor_recovery_codes (user_id, code_hash) VALUES (601, '` + strings.Repeat("c", 64) + `')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`DELETE FROM users WHERE id = 601`); err != nil {
		t.Fatal(err)
	}
	var codes int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM two_factor_recovery_codes WHERE user_id = 601`).Scan(&codes); err != nil || codes != 0 {
		t.Errorf("recovery codes of a removed account = %d, %v", codes, err)
	}
	for table, want := range map[string]int{"site_protected_dirs": 0, "site_protected_users": 0, "site_redirects": 0} {
		var count int
		if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != want {
			t.Errorf("%s rows after the site was removed = %d, %v", table, count, err)
		}
	}
}

func TestOlderReleaseRefusesALedgerWithTheSiteFeaturesEntry(t *testing.T) {
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
		if migration.version < siteFeaturesMigrationVersion {
			older = append(older, migration)
		}
	}
	if len(older) != len(embedded)-(embedded[len(embedded)-1].version-siteFeaturesMigrationVersion+1) {
		t.Fatalf("migration %d is not part of the embedded migrations as expected", siteFeaturesMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
