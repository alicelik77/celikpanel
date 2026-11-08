package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 79 (alpha.130, sending mail through another server,
// docs/MAIL-RELAY.md): mail_relay_settings is new, with its one row. A
// release before it refuses a ledger that carries it; going back is the
// pre-update snapshot restore (D-025). The integrator renumbers it when the
// release's migrations are joined.
const mailRelayMigrationVersion = 79

func TestOlderReleaseRefusesALedgerWithTheMailRelayEntry(t *testing.T) {
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
		if migration.version < mailRelayMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, mailRelayMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestMailRelaySettingsHoldOneRowThatStartsOff(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	var enabled, port int
	var host, sealed string
	if err := db.QueryRow(`SELECT enabled, host, port, password_sealed FROM mail_relay_settings WHERE id = 1`).Scan(&enabled, &host, &port, &sealed); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || host != "" || port != 587 || sealed != "" {
		t.Fatalf("seeded row = %d %q %d %q", enabled, host, port, sealed)
	}
	for _, stmt := range []string{
		`INSERT INTO mail_relay_settings (id) VALUES (2)`,
		`UPDATE mail_relay_settings SET enabled = 2 WHERE id = 1`,
	} {
		if _, err := db.Exec(stmt); err == nil {
			t.Fatalf("accepted: %s", stmt)
		}
	}
}
