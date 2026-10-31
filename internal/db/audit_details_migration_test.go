package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 45 (alpha.107, D-038 §16 and §6): audit_logs.details holds one
// bounded JSON object, sites.app_run_as names a Node application's account;
// both are NULL for every row written before it, so the upgrade changes no
// row. The release before it (schema 44) refuses a ledger that carries entry
// 45; a return to it is the pre-update snapshot restore, as for every
// migration (there is no down migration).
const auditDetailsMigrationVersion = 45

func TestMigration45AddsTwoNullColumnsAndChangesNoRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sqlDB := database.GetDB()
	if _, err := sqlDB.Exec(`INSERT INTO audit_logs (action) VALUES ('before')`); err != nil {
		t.Fatal(err)
	}
	var details any
	if err := sqlDB.QueryRow(`SELECT details FROM audit_logs WHERE action='before'`).Scan(&details); err != nil || details != nil {
		t.Fatalf("details = %v, %v", details, err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO audit_logs (action, details) VALUES ('with', '{"component":"php-fpm","from":"8.3","to":"8.4"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO audit_logs (action, details) VALUES ('bad', 'not json')`); err == nil {
		t.Fatal("details that are not JSON were stored")
	}
	if _, err := sqlDB.Exec(`INSERT INTO audit_logs (action, details) VALUES ('big', ?)`, `{"x":"`+strings.Repeat("a", 9000)+`"}`); err == nil {
		t.Fatal("oversized details were stored")
	}
	var column int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sites') WHERE name='app_run_as'`).Scan(&column); err != nil || column != 1 {
		t.Fatalf("sites.app_run_as = %d, %v", column, err)
	}
}

func TestOlderReleaseRefusesALedgerWithTheAuditDetailsEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	older := make([]embeddedMigration, 0, len(embedded))
	for _, migration := range embedded {
		if migration.version < auditDetailsMigrationVersion {
			older = append(older, migration)
		}
	}
	// Migrations 46 (alpha.108) and 47 (alpha.114) came after it.
	if len(older) != len(embedded)-3 || embedded[len(embedded)-1].version != accountRecoveryMigrationVersion {
		t.Fatalf("the embedded migrations from %d are not exactly 45, 46 and 47", auditDetailsMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
