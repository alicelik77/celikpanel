package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 55 (alpha.117, D-042 section 8): the subscription transfer
// record. Additive: one table, two indexes, one trigger. The 029 guards on
// subscriptions.owner_id are unchanged; these tests prove the transfer
// order (remove the old owner's grants, then change the owner, in one
// transaction) passes them and that the guards still refuse an owner change
// that would leave a grant behind.
const transfersMigrationVersion = 55

func TestTransfersSchemaAndOwnerGuards(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	mustExec(`INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES
		(70, 'old-owner', 'x', 'old@example.test', 'customer', NULL, 'active'),
		(71, 'new-owner', 'x', 'new@example.test', 'customer', NULL, 'active')`)
	mustExec(`INSERT INTO users (id, username, password_hash, email, role, account_type, parent_id, status)
		VALUES (72, 'old-member', 'x', 'member@example.test', 'customer', 'additional_user', 70, 'active')`)
	mustExec(`INSERT INTO subscriptions (id, owner_id, name, status) VALUES (700, 70, 'moving', 'active')`)
	mustExec(`INSERT INTO domains (id, subscription_id, name, status) VALUES (7000, 700, 'moving.test', 'active')`)
	mustExec(`INSERT INTO additional_user_subscription_permissions (user_id, subscription_id, capability, mode) VALUES (72, 700, 'files', 'manage')`)
	mustExec(`INSERT INTO additional_user_domain_permissions (user_id, domain_id, capability, mode) VALUES (72, 7000, 'dns', 'view')`)

	// The 029 guards refuse an owner change while the old owner's grants exist.
	if _, err := raw.Exec(`UPDATE subscriptions SET owner_id = 71 WHERE id = 700`); err == nil ||
		!strings.Contains(err.Error(), "additional-user grants") {
		t.Fatalf("an owner change with grants was accepted: %v", err)
	}
	// The transfer order passes them in one transaction.
	tx, err := raw.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`DELETE FROM additional_user_domain_permissions WHERE domain_id IN (SELECT id FROM domains WHERE subscription_id = 700)`,
		`DELETE FROM additional_user_subscription_permissions WHERE subscription_id = 700`,
		`UPDATE subscriptions SET owner_id = 71 WHERE id = 700 AND owner_id = 70`,
		`INSERT INTO subscription_transfers (request_id, subscription_id, status, from_owner_id, to_owner_id, requested_by, before_image, decided_at)
		 VALUES ('transfer-0001', 700, 'completed', 70, 71, 1, '{}', datetime('now'))`,
	} {
		if _, err := tx.Exec(query); err != nil {
			tx.Rollback()
			t.Fatalf("%s: %v", query, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// A completed row needs its receiver and before-image.
	if _, err := raw.Exec(`INSERT INTO subscription_transfers (request_id, subscription_id, status, from_owner_id, requested_by)
		VALUES ('transfer-0002', 700, 'completed', 71, 1)`); err == nil {
		t.Fatal("a completed transfer without a before-image was stored")
	}
	// One open request per subscription; the same request id twice is refused.
	mustExec(`INSERT INTO subscription_transfers (request_id, subscription_id, status, from_owner_id, requested_by)
		VALUES ('request-0001', 700, 'requested', 71, 71)`)
	if _, err := raw.Exec(`INSERT INTO subscription_transfers (request_id, subscription_id, status, from_owner_id, requested_by)
		VALUES ('request-0002', 700, 'requested', 71, 71)`); err == nil {
		t.Fatal("a second open request was stored")
	}
	if _, err := raw.Exec(`INSERT INTO subscription_transfers (request_id, subscription_id, status, from_owner_id, requested_by)
		VALUES ('transfer-0001', 700, 'cancelled', 71, 71)`); err == nil {
		t.Fatal("a request id was stored twice")
	}
	// A deleted subscription closes its open request; the rows stay.
	mustExec(`DELETE FROM subscriptions WHERE id = 700`)
	var open, kept int
	if err := raw.QueryRow(`SELECT
		(SELECT COUNT(*) FROM subscription_transfers WHERE status = 'requested'),
		(SELECT COUNT(*) FROM subscription_transfers WHERE subscription_id = 700)`).Scan(&open, &kept); err != nil {
		t.Fatal(err)
	}
	if open != 0 || kept != 2 {
		t.Fatalf("after delete: open=%d kept=%d", open, kept)
	}
}

func TestOlderReleaseRefusesALedgerWithTheTransfersEntry(t *testing.T) {
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
		if migration.version < transfersMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, transfersMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
