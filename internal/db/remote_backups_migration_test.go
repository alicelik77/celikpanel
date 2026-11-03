package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 60 (alpha.118): remote backup destinations, schedules and operations. Additive: three tables and their indexes; nothing existing
// changes. A release before it refuses a ledger that carries it; going back
// is the pre-update snapshot restore (D-025).
const remoteBackupsMigrationVersion = 60

func TestOlderReleaseRefusesALedgerWithTheRemoteBackupsEntry(t *testing.T) {
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
		if migration.version < remoteBackupsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, remoteBackupsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestRemoteBackupsMigrationCreatesItsTables(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, table := range []string{"remote_backup_destinations", "remote_backup_policies", "remote_backup_operations"} {
		var name string
		if err := database.GetDB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s is missing: %v", table, err)
		}
	}
}
