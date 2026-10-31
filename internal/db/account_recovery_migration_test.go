package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// accountRecoveryMigrationVersion is 047_account_recovery.sql (alpha.114).
const accountRecoveryMigrationVersion = 47

// An older release must refuse a ledger that already holds this migration.
func TestOlderReleaseRefusesALedgerWithTheAccountRecoveryEntry(t *testing.T) {
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
		if migration.version < accountRecoveryMigrationVersion {
			older = append(older, migration)
		}
	}
	if len(older) != len(embedded)-1 {
		t.Fatalf("migration %d is not the newest embedded migration", accountRecoveryMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// The account-links migration (v0.1.0-alpha.114, P6) is additive: an existing
// database keeps every user and value, every existing address starts
// unconfirmed, and any later change of an address clears its confirmation.
// The migration is found by name so a renumbering at integration keeps the test.
// Hesap bağlantıları göçü eklemelidir; var olan adresler onaysız başlar,
// adres değişince onay düşer.
func TestAccountRecoveryMigrationUpgradesAndClearsConfirmationOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.sqlite")
	database, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	target := -1
	for index, migration := range migrations {
		if strings.HasSuffix(migration.filename, "_account_recovery.sql") {
			target = index
			break
		}
	}
	if target < 0 {
		t.Fatal("account recovery migration not embedded")
	}
	for _, migration := range migrations[:target] {
		if _, err := database.Exec(string(migration.content)); err != nil {
			t.Fatalf("apply %s: %v", migration.filename, err)
		}
	}
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash, email, role, status)
		VALUES (1, 'kept', 'hash-kept', 'kept@example.test', 'customer', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(migrations[target].content)); err != nil {
		t.Fatalf("apply %s: %v", migrations[target].filename, err)
	}

	var username, hash, email string
	var verified sql.NullString
	if err := database.QueryRow(`SELECT username, password_hash, email, email_verified_at FROM users WHERE id = 1`).
		Scan(&username, &hash, &email, &verified); err != nil {
		t.Fatal(err)
	}
	if username != "kept" || hash != "hash-kept" || email != "kept@example.test" || verified.Valid {
		t.Fatalf("existing row after upgrade: %s %s %s %v", username, hash, email, verified)
	}

	confirm := func() {
		t.Helper()
		if _, err := database.Exec(`UPDATE users SET email_verified_at = '2026-10-31T00:00:00Z' WHERE id = 1`); err != nil {
			t.Fatal(err)
		}
	}
	confirmed := func() bool {
		t.Helper()
		var at sql.NullString
		if err := database.QueryRow(`SELECT email_verified_at FROM users WHERE id = 1`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at.Valid
	}
	confirm()
	if _, err := database.Exec(`UPDATE users SET email = email, password_hash = 'other' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if !confirmed() {
		t.Fatal("an unchanged address lost its confirmation")
	}
	if _, err := database.Exec(`UPDATE users SET email = 'new@example.test' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if confirmed() {
		t.Fatal("a changed address kept its confirmation")
	}

	// Links go with their account, and a token hash is stored once.
	if _, err := database.Exec(`INSERT INTO account_links (purpose, user_id, token_hash, email, auth_epoch, expires_at)
		VALUES ('password_reset', 1, 'h1', 'new@example.test', 0, '2026-10-31T00:30:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO account_links (purpose, user_id, token_hash, email, auth_epoch, expires_at)
		VALUES ('invitation', 1, 'h1', 'new@example.test', 0, '2026-10-31T00:30:00Z')`); err == nil {
		t.Fatal("a token hash was stored twice")
	}
	if _, err := database.Exec(`DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var links int
	if err := database.QueryRow(`SELECT COUNT(*) FROM account_links`).Scan(&links); err != nil || links != 0 {
		t.Fatalf("links after the account was deleted: %d %v", links, err)
	}
}
