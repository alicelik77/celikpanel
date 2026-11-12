package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 78 (alpha.134, docs/CERTIFICATE-COVERAGE.md): which name a
// domain's mail programs are told. A release before it refuses a ledger that
// carries it; going back is the pre-update snapshot restore (D-025). The
// integrator renumbers it when the release's migrations are joined; the tests
// find it by name.
// Posta programlarına söylenen ad göçü; eski sürüm bu kaydı taşıyan defteri reddeder.
const domainMailClientNameMigrationVersion = 78

func domainMailClientNameMigration(t *testing.T) (int, []embeddedMigration) {
	t.Helper()
	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for index, migration := range migrations {
		if strings.HasSuffix(migration.filename, "_domain_mail_client_name.sql") {
			return index, migrations
		}
	}
	t.Fatal("domain mail client name migration not embedded")
	return -1, nil
}

func TestOlderReleaseRefusesALedgerWithTheDomainMailClientNameEntry(t *testing.T) {
	index, embedded := domainMailClientNameMigration(t)
	version := embedded[index].version
	if version != domainMailClientNameMigrationVersion {
		t.Fatalf("migration version = %d", version)
	}
	database, err := NewSQLiteDB(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	older := make([]embeddedMigration, 0, len(embedded))
	for _, migration := range embedded {
		if migration.version < version {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, version)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// Only a domain whose active certificate secured mail before the update keeps
// telling mail.<domain>; every other domain, and a domain whose secured
// certificate is no longer active, tells the server's mail name. Nothing else
// in the rows changes.
func TestDomainMailClientNameMigrationKeepsTheOldChoiceOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.sqlite")
	database, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	index, migrations := domainMailClientNameMigration(t)
	for _, migration := range migrations[:index] {
		if _, err := database.Exec(string(migration.content)); err != nil {
			t.Fatalf("apply %s: %v", migration.filename, err)
		}
	}
	for _, stmt := range []string{
		`INSERT INTO domains (id, subscription_id, name) VALUES (1, 1, 'chose.example'), (2, 1, 'plain.example'), (3, 1, 'old.example')`,
		`INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, expires_at, status, secure_mail)
		 VALUES (1, 'letsencrypt', '/c1', '/k1', '2027-01-01', 'active', 1),
		        (2, 'letsencrypt', '/c2', '/k2', '2027-01-01', 'active', 0),
		        (3, 'letsencrypt', '/c3', '/k3', '2027-01-01', 'expired', 1)`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(string(migrations[index].content)); err != nil {
		t.Fatalf("apply %s: %v", migrations[index].filename, err)
	}
	got := map[string]int{}
	rows, err := database.Query(`SELECT name, mail_clients_use_own_name FROM domains ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		var own int
		if err := rows.Scan(&name, &own); err != nil {
			t.Fatal(err)
		}
		got[name] = own
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if got["chose.example"] != 1 || got["plain.example"] != 0 || got["old.example"] != 0 {
		t.Fatalf("after the upgrade: %v", got)
	}
	var secure int
	if err := database.QueryRow(`SELECT COUNT(*) FROM ssl_certificates WHERE secure_mail = 1`).Scan(&secure); err != nil || secure != 2 {
		t.Fatalf("certificate rows changed: %d %v", secure, err)
	}
	if _, err := database.Exec(`INSERT INTO domains (id, subscription_id, name) VALUES (4, 1, 'new.example')`); err != nil {
		t.Fatal(err)
	}
	var own int
	if err := database.QueryRow(`SELECT mail_clients_use_own_name FROM domains WHERE id = 4`).Scan(&own); err != nil || own != 0 {
		t.Fatalf("a new domain starts with %d %v", own, err)
	}
	if _, err := database.Exec(`UPDATE domains SET mail_clients_use_own_name = 2 WHERE id = 4`); err == nil {
		t.Fatal("a value other than 0 or 1 was accepted")
	}
}
