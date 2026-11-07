package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 69 (alpha.128, G24; 070 on its workstream branch): visitor
// statistics per site and day. Additive: one new table. A release before it
// refuses a ledger that carries it; going back is the pre-update snapshot
// restore (D-025).
const siteVisitorStatsMigrationVersion = 69

func TestOlderReleaseRefusesALedgerWithTheSiteVisitorStatsEntry(t *testing.T) {
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
		if migration.version < siteVisitorStatsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, siteVisitorStatsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
