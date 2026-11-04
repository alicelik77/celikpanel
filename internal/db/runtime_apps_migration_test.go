package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 63 (alpha.119): runtime apps (offerings, apps, operations).
// Additive: three tables and their indexes; nothing existing changes. A
// release before it refuses a ledger that carries it; going back is the
// pre-update snapshot restore (D-025).
const runtimeAppsMigrationVersion = 63

func TestOlderReleaseRefusesALedgerWithTheRuntimeAppsEntry(t *testing.T) {
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
		if migration.version < runtimeAppsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, runtimeAppsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestRuntimeAppsMigrationCreatesItsTablesAndChecks(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	for _, table := range []string{"runtime_offerings", "runtime_apps", "runtime_app_operations"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s is missing: %v", table, err)
		}
	}
	good := []string{
		`INSERT INTO runtime_offerings (offering_id, kind, memory_mb, cpu_percent, tasks_max) VALUES ('runtime:program', 'program', 512, 100, 256)`,
		`INSERT INTO runtime_offerings (offering_id, kind, memory_mb, cpu_percent, tasks_max) VALUES ('component:python:3.13', 'python', 512, 100, 256)`,
		`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port) VALUES (1, 2, 3, 'program', 'runtime:program', 'server', 30000)`,
		`INSERT INTO runtime_app_operations (id, app_id, kind) VALUES ('op-00000001', 1, 'deploy')`,
	}
	for _, statement := range good {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	bad := []string{
		`INSERT INTO runtime_offerings (offering_id, kind, memory_mb, cpu_percent, tasks_max) VALUES ('runtime:node', 'program', 512, 100, 256)`,
		`INSERT INTO runtime_offerings (offering_id, kind, memory_mb, cpu_percent, tasks_max, accepted_level) VALUES ('component:python:3.12', 'python', 512, 100, 256, 'none')`,
		`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port) VALUES (1, 4, 3, 'program', 'runtime:program', 'server', 29999)`,
		`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port) VALUES (1, 5, 3, 'program', 'runtime:program', 'server', 30000)`,
		`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port) VALUES (1, 2, 3, 'program', 'runtime:program', 'server', 30001)`,
		`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port) VALUES (1, 6, 3, 'node', 'runtime:program', 'server', 30002)`,
		`INSERT INTO runtime_app_operations (id, app_id, kind) VALUES ('op-00000002', 1, 'deploy')`,
		`INSERT INTO runtime_app_operations (id, app_id, kind) VALUES ('op-00000003', 2, 'start')`,
	}
	for _, statement := range bad {
		if _, err := db.Exec(statement); err == nil {
			t.Fatalf("accepted: %s", statement)
		}
	}
}
