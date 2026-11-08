package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 76 (alpha.130, mailbox recovery address and reset link,
// docs/MAILBOX-RECOVERY-REVIEW.md): two new tables. A release before it
// refuses a ledger that carries it; going back is the pre-update snapshot
// restore (D-025).
const mailboxRecoveryMigrationVersion = 76

func TestOlderReleaseRefusesALedgerWithTheMailboxRecoveryEntry(t *testing.T) {
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
		if migration.version < mailboxRecoveryMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, mailboxRecoveryMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMailboxRecoveryRowsFollowTheirMailbox(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (9601, 'c', 'x', 'c@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (9602, 9601, 'C');
		INSERT INTO domains (id, subscription_id, name) VALUES (9603, 9602, 'example.test');
		INSERT INTO email_accounts (id, domain_id, address, password_hash) VALUES (9604, 9603, 'anna@example.test', 'managed-by-agent');
		INSERT INTO mailbox_recovery_addresses (email_account_id, address, subscription_id, owner_user_id, set_by, set_at)
		VALUES (9604, 'anna@elsewhere.test', 9602, 9601, 9601, '2026-11-08T00:00:00Z');
		INSERT INTO mailbox_recovery_links (purpose, email_account_id, address, token_hash, created_at, expires_at)
		VALUES ('confirm', 9604, 'anna@elsewhere.test', 'h1', '2026-11-08T00:00:00Z', '2026-11-09T00:00:00Z');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO mailbox_recovery_addresses (email_account_id, address, subscription_id, owner_user_id, set_at)
		VALUES (9604, 'other@elsewhere.test', 9602, 9601, '2026-11-08T00:00:00Z')`); err == nil {
		t.Fatal("a mailbox got two recovery addresses")
	}
	if _, err := db.Exec(`INSERT INTO mailbox_recovery_links (purpose, email_account_id, address, token_hash, created_at, expires_at)
		VALUES ('reset', 9604, 'anna@elsewhere.test', 'h1', '2026-11-08T00:00:00Z', '2026-11-08T00:30:00Z')`); err == nil {
		t.Fatal("two links share one token hash")
	}
	// The person who set it may go; the address stays, visible on the mailbox.
	if _, err := db.Exec(`UPDATE mailbox_recovery_addresses SET set_by = NULL WHERE email_account_id = 9604`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM email_accounts WHERE id = 9604`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM mailbox_recovery_addresses) + (SELECT COUNT(*) FROM mailbox_recovery_links)`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("rows left after the mailbox went: %d, %v", left, err)
	}
}
