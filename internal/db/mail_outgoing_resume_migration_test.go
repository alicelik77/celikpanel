package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 73 (alpha.129): a resumed mailbox
// starts a fresh spike window. Additive: two columns with a default of 0 on
// mailbox_outgoing. A release before it refuses a ledger that carries it;
// going back is the pre-update snapshot restore (D-025).
const mailOutgoingResumeMigrationVersion = 73

func TestOlderReleaseRefusesALedgerWithTheMailOutgoingResumeEntry(t *testing.T) {
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
		if migration.version < mailOutgoingResumeMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, mailOutgoingResumeMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMailOutgoingResumeMigrationKeepsOldRowsAndChecksItsValues(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (71, 'c', 'x', 'c@example.net', 'customer')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (73, 71, 's')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (74, 73, 'example.com')`,
		`INSERT INTO email_accounts (id, domain_id, address, password_hash) VALUES (75, 74, 'a@example.com', 'x')`,
		// An alpha.128-style row (no baseline columns named) reads 0.
		`INSERT INTO mailbox_outgoing (email_account_id, resumed_at) VALUES (75, '2026-11-07 10:20:00')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	var recipients, refusals int
	if err := db.QueryRow(`SELECT resume_base_recipients, resume_base_refusals FROM mailbox_outgoing WHERE email_account_id = 75`).Scan(&recipients, &refusals); err != nil || recipients != 0 || refusals != 0 {
		t.Fatalf("old row baseline = %d %d (%v)", recipients, refusals, err)
	}
	if _, err := db.Exec(`UPDATE mailbox_outgoing SET resume_base_recipients = 600, resume_base_refusals = 4 WHERE email_account_id = 75`); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`UPDATE mailbox_outgoing SET resume_base_recipients = -1 WHERE email_account_id = 75`,
		`UPDATE mailbox_outgoing SET resume_base_refusals = -1 WHERE email_account_id = 75`,
		`UPDATE mailbox_outgoing SET resume_base_recipients = NULL WHERE email_account_id = 75`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("accepted: %s", stmt)
		}
	}
}
