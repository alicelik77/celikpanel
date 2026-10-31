package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 46 (alpha.108, PostgreSQL majors side by side): one NULL column
// on database_servers (which cluster a record is; NULL for every record made
// before it, which the Panel ties to the cluster on its port when it first
// reads the clusters) and one new table of database moves with their proof.
// No row is changed. The release before it (schema 45) refuses a ledger that
// carries entry 46; a return to it is the pre-update snapshot restore, as for
// every migration (there is no down migration).
const postgresqlClustersMigrationVersion = 46

func TestMigration46AddsTheClusterColumnAndTheMovesTableAndChangesNoRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.sqlite")
	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	sqlDB := database.GetDB()
	var column int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('database_servers') WHERE name='cluster'`).Scan(&column); err != nil || column != 1 {
		t.Fatalf("database_servers.cluster = %d, %v", column, err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO postgresql_database_moves (database_name, source_cluster, target_cluster, source_port, target_port, state, proof) VALUES ('shop', '17/main', '18/main', 5432, 5433, 'copied', '{"equal":true}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO postgresql_database_moves (database_name, source_cluster, target_cluster, source_port, target_port, state, proof) VALUES ('shop', '17/main', '18/main', 5432, 5433, 'converted', '{}')`); err == nil {
		t.Fatal("an unknown state was stored")
	}
	if _, err := sqlDB.Exec(`INSERT INTO postgresql_database_moves (database_name, source_cluster, target_cluster, source_port, target_port, state, proof) VALUES ('shop', '17/main', '18/main', 5432, 5433, 'copied', 'not json')`); err == nil {
		t.Fatal("a proof that is not JSON was stored")
	}
	if _, err := sqlDB.Exec(`INSERT INTO postgresql_database_moves (database_name, source_cluster, target_cluster, source_port, target_port, state, proof) VALUES ('shop', '17/main', '18/main', 0, 5433, 'copied', '{}')`); err == nil {
		t.Fatal("a port of 0 was stored")
	}
	var clusterCheck string
	if err := sqlDB.QueryRow(`SELECT sql FROM sqlite_schema WHERE name='database_servers'`).Scan(&clusterCheck); err != nil || !strings.Contains(clusterCheck, "cluster GLOB") {
		t.Fatalf("database_servers sql = %q, %v", clusterCheck, err)
	}
}

func TestOlderReleaseRefusesALedgerWithThePostgreSQLClustersEntry(t *testing.T) {
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
		if migration.version < postgresqlClustersMigrationVersion {
			older = append(older, migration)
		}
	}
	// Migration 47 (alpha.114) came after it.
	if len(older) != len(embedded)-2 || embedded[len(embedded)-1].version != accountRecoveryMigrationVersion {
		t.Fatalf("the embedded migrations from %d are not exactly 46 and 47", postgresqlClustersMigrationVersion)
	}
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
