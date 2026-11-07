package db

import (
	"path/filepath"
	"strings"
	"testing"
)

// Migration 71 (alpha.128, G16; 72 on its workstream branch): additional FTP/SFTP logins. Additive: one
// new table. A release before it refuses a ledger that carries it; going back
// is the pre-update snapshot restore (D-025). The number is the workstream's
// placeholder; the release integration renumbers it.
const ftpLoginsMigrationVersion = 71

func TestOlderReleaseRefusesALedgerWithTheFTPLoginsEntry(t *testing.T) {
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
		if migration.version < ftpLoginsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, ftpLoginsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

func TestFTPLoginsMigrationChecksItsRows(t *testing.T) {
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	db := database.GetDB()
	if _, err := db.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (9101, 'ftp-owner', 'x', 'ftp-owner@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES (9102, 9101, 'Files', 'active');
		INSERT INTO domains (id, subscription_id, name, status) VALUES (9103, 9102, 'example.com', 'active');`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ftp_logins (domain_id, login, folder, created_by) VALUES (9103, 'anna-up', '/public_html/uploads', 9101)`); err != nil {
		t.Fatalf("a valid login was refused: %v", err)
	}
	for _, bad := range []string{
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'anna-up', '/')`,     // the name is the server's: unique
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'example_com', '/')`, // no hyphen: a site account's form
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'Anna-up', '/')`,     // upper case
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'bob-x', 'public')`,  // not a clean folder
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'bob-y', '/a/../b')`, // ..
		`INSERT INTO ftp_logins (domain_id, login, folder) VALUES (9103, 'bob-z', '/a//b')`,   // empty part
	} {
		if _, err := db.Exec(bad); err == nil {
			t.Fatalf("accepted: %s", bad)
		}
	}
	// Rows go with their domain.
	if _, err := db.Exec(`DELETE FROM domains WHERE id = 9103`); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ftp_logins`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("rows outlived their domain: %d %v", left, err)
	}
}
