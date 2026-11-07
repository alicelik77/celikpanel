package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 74 (alpha.129, remote access to databases,
// docs/DATABASE-REMOTE-ACCESS.md): database_remote_rules is new. A release before it refuses a ledger that carries it; going
// back is the pre-update snapshot restore (D-025).
const databaseRemoteMigrationVersion = 74

func TestOlderReleaseRefusesALedgerWithTheDatabaseRemoteEntry(t *testing.T) {
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
		if migration.version < databaseRemoteMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, databaseRemoteMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestDatabaseRemoteRulesFollowTheirDatabaseAndUser(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (9501, 'r', 'x', 'r@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (9502, 9501, 'R');
		INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (9503, 9502, (SELECT id FROM database_server_types WHERE name = 'mariadb'), 'M', 'localhost', 3306);
		INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (9504, 9503, 9502, 'shop');
		INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9505, 9503, 9502, 'shop_u', 'x');
		INSERT INTO database_remote_rules (database_id, user_id, address, created_by) VALUES (9504, 9505, '203.0.113.7/32', 9501);`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (9504, 9505, '203.0.113.7/32')`); err == nil {
		t.Fatal("the same rule was stored twice")
	}
	if _, err := db.Exec(`DELETE FROM database_users WHERE id = 9505`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_remote_rules`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("rules left after the user went: %d, %v", left, err)
	}
}
