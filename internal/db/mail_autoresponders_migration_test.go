package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 47 (alpha.114, out-of-office replies): one new table keyed by
// the mailbox; no existing row is changed. The release before it (schema 46)
// refuses a ledger that carries entry 47; a return to it is the pre-update
// snapshot restore, as for every migration.
const mailAutorespondersMigrationVersion = 47

func TestMigration47AddsTheAutoresponderTableKeyedByMailbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sqlDB := database.GetDB()
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (1, 'owner', 'hash', 'owner@example.test', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (1, 1, 'subscription')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (1, 1, 'example.test')`,
	} {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if _, err := sqlDB.Exec(`INSERT INTO email_accounts (id, domain_id, address, password_hash) VALUES (7, 1, 'a@example.test', 'x')`); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO email_autoresponders (account_id, enabled, subject, message, start_date, end_date, interval_days, addresses) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := sqlDB.Exec(insert, 7, 1, "Away", "Back soon", "2026-11-01", "2026-11-09", 3, `["b@example.test"]`); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]any{
		"bad date":      {7, 1, "Away", "x", "1 Nov", nil, 1, "[]"},
		"interval 0":    {7, 1, "Away", "x", nil, nil, 0, "[]"},
		"not an array":  {7, 1, "Away", "x", nil, nil, 1, `{"a":1}`},
		"empty subject": {7, 1, "", "x", nil, nil, 1, "[]"},
	} {
		if _, err := sqlDB.Exec(`DELETE FROM email_autoresponders`); err != nil {
			t.Fatal(err)
		}
		if _, err := sqlDB.Exec(insert, args...); err == nil {
			t.Fatalf("%s was stored", name)
		}
	}
	if _, err := sqlDB.Exec(insert, 7, 0, "Away", "", nil, nil, 1, "[]"); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`DELETE FROM email_accounts WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM email_autoresponders`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("the reply outlived its mailbox: %d, %v", left, err)
	}
}

func TestOlderReleaseRefusesALedgerWithTheAutoresponderEntry(t *testing.T) {
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
		if migration.version < mailAutorespondersMigrationVersion {
			older = append(older, migration)
		}
	}
	if len(older) != len(embedded)-1 {
		t.Fatalf("migration %d is not the newest embedded migration", mailAutorespondersMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
