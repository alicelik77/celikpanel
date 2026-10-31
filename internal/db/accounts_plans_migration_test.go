package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 47 (alpha.114, accounts, plans and resellers): one new table
// (reseller_pools) and two NULL columns (sessions.impersonator_id,
// audit_logs.acting_as). No row is changed: a database at schema 46 with
// users, sessions and audit rows upgrades with every row as it was, every
// existing session still valid and no reseller limited (no pool row). The
// release before it refuses a ledger that carries entry 47; a return to it is
// the pre-update snapshot restore, as for every migration.
const accountsPlansMigrationVersion = 47

func TestMigration47UpgradesASchema46DatabaseAndChangesNoRow(t *testing.T) {
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	dsn := fmt.Sprintf("%s?_pragma=foreign_keys(1)", filepath.Join(t.TempDir(), "panel46.sqlite"))
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)
	var migration47 *embeddedMigration
	for i := range embedded {
		if embedded[i].version == accountsPlansMigrationVersion {
			migration47 = &embedded[i]
			continue
		}
		if embedded[i].version > accountsPlansMigrationVersion {
			continue
		}
		if _, err := raw.Exec(string(embedded[i].content)); err != nil {
			t.Fatalf("apply %s: %v", embedded[i].filename, err)
		}
	}
	if migration47 == nil {
		t.Fatal("migration 47 is not embedded")
	}
	if _, err := raw.Exec(`
		INSERT INTO users (id, username, password_hash, email, role, parent_id) VALUES
			(500, 'old-reseller', 'x', 'old-reseller@example.test', 'reseller', NULL),
			(501, 'old-customer', 'x', 'old-customer@example.test', 'customer', 500);
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('hash-501', 501, '2999-01-01T00:00:00Z');
		INSERT INTO audit_logs (user_id, action) VALUES (500, 'user.create:old-customer/customer');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(string(migration47.content)); err != nil {
		t.Fatalf("apply migration 47 to a schema-46 database: %v", err)
	}
	var users, pools int
	var impersonator, actingAs sql.NullInt64
	if err := raw.QueryRow(`SELECT COUNT(*) FROM users WHERE id IN (500, 501)`).Scan(&users); err != nil || users != 2 {
		t.Fatalf("users after upgrade = %d, %v", users, err)
	}
	if err := raw.QueryRow(`SELECT impersonator_id FROM sessions WHERE token_hash = 'hash-501'`).Scan(&impersonator); err != nil || impersonator.Valid {
		t.Fatalf("an existing session changed: %v, %v", impersonator, err)
	}
	if err := raw.QueryRow(`SELECT acting_as FROM audit_logs WHERE action = 'user.create:old-customer/customer'`).Scan(&actingAs); err != nil || actingAs.Valid {
		t.Fatalf("an existing audit row changed: %v, %v", actingAs, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM reseller_pools`).Scan(&pools); err != nil || pools != 0 {
		t.Fatalf("an existing reseller was given a pool: %d, %v", pools, err)
	}
	if _, err := raw.Exec(`INSERT INTO reseller_pools (reseller_id, max_customers) VALUES (500, -1)`); err == nil {
		t.Fatal("a negative pool limit was stored")
	}
	if _, err := raw.Exec(`INSERT INTO reseller_pools (reseller_id, max_customers, max_domains) VALUES (500, 3, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at, impersonator_id) VALUES ('hash-imp', 501, '2999-01-01T00:00:00Z', 500)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at, impersonator_id) VALUES ('hash-bad', 501, '2999-01-01T00:00:00Z', 0)`); err == nil {
		t.Fatal("an impersonator id of 0 was stored")
	}
	// The pool ends with its reseller.
	if _, err := raw.Exec(`DELETE FROM users WHERE id = 501; DELETE FROM users WHERE id = 500`); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM reseller_pools`).Scan(&pools); err != nil || pools != 0 {
		t.Fatalf("a deleted reseller's pool remains: %d, %v", pools, err)
	}
}

func TestMigration47IsTheNewestAndAnOlderReleaseRefusesItsLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	older := make([]embeddedMigration, 0, len(embedded))
	for _, migration := range embedded {
		if migration.version < accountsPlansMigrationVersion {
			older = append(older, migration)
		}
	}
	if len(older) != len(embedded)-1 {
		t.Fatalf("migration %d is not the newest embedded migration", accountsPlansMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
