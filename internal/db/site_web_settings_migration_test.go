package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 68 (alpha.128): a site's web server settings, Node.js
// environment, log retention and git deployment. Additive: five new tables.
// A release before it refuses a ledger that carries it; going back is the
// pre-update snapshot restore (D-025).
const siteWebSettingsMigrationVersion = 68

func TestOlderReleaseRefusesALedgerWithTheSiteWebSettingsEntry(t *testing.T) {
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
		if migration.version < siteWebSettingsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, siteWebSettingsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
