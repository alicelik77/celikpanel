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

// Migration 75 (alpha.130, timed remote access): one nullable column; every
// rule of alpha.129 keeps no end. A release before it refuses a ledger that
// carries it; going back is the pre-update snapshot restore (D-025).
const databaseRemoteExpiryMigrationVersion = 75

func TestOlderReleaseRefusesALedgerWithTheRemoteExpiryEntry(t *testing.T) {
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
		if migration.version < databaseRemoteExpiryMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, databaseRemoteExpiryMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestDatabaseRemoteRulesKeepNoEndUnlessGivenOne(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (9601, 'r', 'x', 'r@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (9602, 9601, 'R');
		INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (9603, 9602, (SELECT id FROM database_server_types WHERE name = 'mariadb'), 'M', 'localhost', 3306);
		INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (9604, 9603, 9602, 'shop');
		INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9605, 9603, 9602, 'shop_u', 'x');
		INSERT INTO database_remote_rules (database_id, user_id, address) VALUES (9604, 9605, '203.0.113.7/32');
		INSERT INTO database_remote_rules (database_id, user_id, address, expires_at) VALUES (9604, 9605, '203.0.113.8/32', '2026-11-09 10:00:00');`); err != nil {
		t.Fatal(err)
	}
	var none, timed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_remote_rules WHERE expires_at IS NULL`).Scan(&none); err != nil || none != 1 {
		t.Fatalf("rules without an end: %d, %v", none, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM database_remote_rules WHERE expires_at <= '2026-11-09 10:00:00'`).Scan(&timed); err != nil || timed != 1 {
		t.Fatalf("rules past the end: %d, %v", timed, err)
	}
}
