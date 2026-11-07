package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 67 (alpha.128): outgoing mail limits and plan mail settings.
// Additive: five new tables, one seeded row, triggers. A release before it
// refuses a ledger that carries it; going back is the pre-update snapshot
// restore (D-025).
const mailOutgoingMigrationVersion = 67

func TestOlderReleaseRefusesALedgerWithTheMailOutgoingEntry(t *testing.T) {
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
		if migration.version < mailOutgoingMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, mailOutgoingMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMailOutgoingMigrationSeedsChecksAndFollowsOwners(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	var enabled, mailbox, domain, sub int
	if err := db.QueryRow(`SELECT enabled, mailbox_per_hour, domain_per_hour, subscription_per_hour FROM mail_outgoing_settings WHERE id = 1`).Scan(&enabled, &mailbox, &domain, &sub); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || mailbox != 500 || domain != 750 || sub != 1000 {
		t.Fatalf("seed = %d %d %d %d", enabled, mailbox, domain, sub)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, email, role) VALUES (71, 'c', 'x', 'c@example.net', 'customer')`,
		`INSERT INTO service_plans (id, name) VALUES (72, 'P')`,
		`INSERT INTO subscriptions (id, owner_id, name) VALUES (73, 71, 's')`,
		`INSERT INTO domains (id, subscription_id, name) VALUES (74, 73, 'example.com')`,
		`INSERT INTO email_accounts (id, domain_id, address, password_hash) VALUES (75, 74, 'a@example.com', 'x')`,
		`INSERT INTO mailbox_outgoing (email_account_id, per_hour, paused_at, paused_reason) VALUES (75, 0, 'x', 'spike')`,
		`INSERT INTO mail_plan_limits (owner_kind, owner_id, mailbox_size_mb, outgoing_mailbox) VALUES ('plan', 72, 2048, 1000)`,
		`INSERT INTO mail_plan_limits (owner_kind, owner_id, outgoing_domain) VALUES ('subscription', 73, 0)`,
		`INSERT INTO mail_outgoing_notices (user_id, audience, kind, address, subscription_id, period) VALUES (71, 'customer', 'paused', 'a@example.com', 73, 'p')`,
		`INSERT INTO mail_outgoing_hourly (address, hour, recipients) VALUES ('a@example.com', '2026-11-07T10:00:00Z', 5)`,
		`INSERT INTO domain_mail_options (domain_id, incoming_off, unknown_reply) VALUES (74, 0, 'No such user here')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	bad := []string{
		`INSERT INTO mail_outgoing_settings (id) VALUES (2)`,
		`UPDATE mail_outgoing_settings SET spike_factor = 1 WHERE id = 1`,
		`INSERT INTO mailbox_outgoing (email_account_id, per_hour) VALUES (999999, 5)`,
		`UPDATE mailbox_outgoing SET paused_reason = 'whim' WHERE email_account_id = 75`,
		`INSERT INTO mail_plan_limits (owner_kind, owner_id) VALUES ('reseller', 1)`,
		`INSERT INTO mail_outgoing_notices (user_id, audience, kind, address, period) VALUES (71, 'customer', 'paused', 'a@example.com', 'p')`,
		`INSERT INTO mail_outgoing_hourly (address, hour, limit_kind) VALUES ('b@example.com', 'h', 'planet')`,
		`UPDATE domain_mail_options SET incoming_off = 2 WHERE domain_id = 74`,
		`UPDATE domain_mail_options SET unknown_reply = '' WHERE domain_id = 74`,
	}
	for _, stmt := range bad {
		if _, err := db.Exec(stmt); err == nil {
			t.Errorf("accepted: %s", stmt)
		}
	}
	// Owners take their rows with them.
	if _, err := db.Exec(`DELETE FROM email_accounts WHERE id = 75`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM subscriptions WHERE id = 73`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM service_plans WHERE id = 72`); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"mailbox_outgoing": 0, "mail_plan_limits": 0, "mail_outgoing_notices": 0, "mail_outgoing_hourly": 1} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != want {
			t.Errorf("%s rows = %d (%v), want %d", table, n, err, want)
		}
	}
}
