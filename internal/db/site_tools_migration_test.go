package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 59 (alpha.118): WordPress tools and malware scanning of hosted
// sites. Additive: five tables and their indexes. 57 and 58 are the parallel
// alpha.118 workstreams; 59 follows them.
const siteToolsMigrationVersion = 59

func TestSiteToolsSchema(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	raw := database.GetDB()
	op := strings.Repeat("a", 32)
	if _, err := raw.Exec(`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, scope, status)
		VALUES (?, 10, 20, 'update', 'plugin', 'updating')`, op); err != nil {
		t.Fatal(err)
	}
	// One open operation per site; a finished one does not block.
	if _, err := raw.Exec(`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status)
		VALUES (?, 10, 20, 'update', 'queued')`, strings.Repeat("b", 32)); err == nil {
		t.Fatal("a second open operation for the same site was accepted")
	}
	if _, err := raw.Exec(`UPDATE wordpress_operations SET status = 'applied' WHERE operation_id = ?`, op); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status, target_domain_id)
		VALUES (?, 10, 20, 'clone', 'cloning', 11)`, strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	// An open copy also holds its target.
	if _, err := raw.Exec(`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status, target_domain_id)
		VALUES (?, 12, 20, 'clone', 'queued', 11)`, strings.Repeat("d", 32)); err == nil {
		t.Fatal("a second open copy into the same target was accepted")
	}
	if _, err := raw.Exec(`INSERT INTO malware_scans (subscription_id, domain_id, status) VALUES (20, NULL, 'queued')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO malware_scans (subscription_id, domain_id, status) VALUES (20, NULL, 'running')`); err == nil {
		t.Fatal("a second open scan of the same subscription was accepted")
	}
	if _, err := raw.Exec(`INSERT INTO malware_scans (subscription_id, domain_id, status) VALUES (20, 10, 'queued')`); err != nil {
		t.Fatal("a site scan beside a subscription scan was refused: ", err)
	}
	for _, bad := range []string{
		`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status) VALUES ('short', 1, 1, 'update', 'queued')`,
		`INSERT INTO wordpress_operations (operation_id, domain_id, subscription_id, kind, status) VALUES ('` + strings.Repeat("e", 32) + `', 1, 1, 'push', 'queued')`,
		`INSERT INTO malware_scans (subscription_id, status) VALUES (1, 'deleted')`,
		`INSERT INTO malware_findings (scan_id, subscription_id, domain_id, path, signature, state) VALUES (1, 1, 1, 'a.php', 'X', 'auto_deleted')`,
		`INSERT INTO malware_notices (scan_id, user_id, audience, body) VALUES (1, 1, 'everyone', '{}')`,
	} {
		if _, err := raw.Exec(bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
}

func TestOlderReleaseRefusesALedgerWithTheSiteToolsEntry(t *testing.T) {
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
		if migration.version < siteToolsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, siteToolsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
