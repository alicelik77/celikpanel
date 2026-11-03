package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 58 (alpha.118): the application catalog's operations. Additive:
// one table and three indexes; nothing existing changes. 057 belongs to a
// parallel workstream of the same release.
const appCatalogMigrationVersion = 58

func TestAppCatalogOperationsSchema(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	if _, err := raw.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, app, status string, domain int) error {
		_, err := raw.Exec(`INSERT INTO app_catalog_operations (operation_id, app_id, app_version, domain_id, subscription_id, site_id,
			database_server_id, database_name, database_user, cleanup_token_encrypted, status) VALUES (?, ?, '1.0', ?, 1, 1, 1, 'd', 'u', 'x', ?)`,
			id, app, domain, status)
		return err
	}
	if err := insert(strings.Repeat("a", 32), "nextcloud", "applied", 7); err != nil {
		t.Fatal(err)
	}
	// One live application per domain, enforced by the database.
	if err := insert(strings.Repeat("b", 32), "drupal", "reserved", 7); err == nil {
		t.Fatal("a second live application on the same domain was accepted")
	}
	// A failed or removed one does not hold the domain.
	if err := insert(strings.Repeat("c", 32), "drupal", "failed", 7); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []error{
		insert("short", "drupal", "reserved", 8),
		insert(strings.Repeat("d", 32), "Drupal!", "reserved", 9),
		insert(strings.Repeat("e", 32), "drupal", "installed", 10),
	} {
		if bad == nil {
			t.Fatal("an invalid row was accepted")
		}
	}
}

func TestOlderReleaseRefusesALedgerWithTheAppCatalogEntry(t *testing.T) {
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
		if migration.version < appCatalogMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, appCatalogMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
