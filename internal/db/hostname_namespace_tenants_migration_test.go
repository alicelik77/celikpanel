package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 80 (security release, 20 November 2026): the one name rule in
// the database. Triggers only: no table, column or row changes, so a
// database that already holds an overlap made before the rule upgrades, and
// an overlap never breaks a write that does not give a name. Going back is
// the pre-update snapshot restore, as for every migration (D-025).
const hostnameNamespaceTenantsMigrationVersion = 80

func TestOlderReleaseRefusesALedgerWithTheNameRuleEntry(t *testing.T) {
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
		if migration.version < hostnameNamespaceTenantsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, hostnameNamespaceTenantsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// A schema-79 database whose customers already overlap (bob holds
// shop.victim.test under alice's victim.test, and the infrastructure zone
// infra.test) upgrades with every row intact; unrelated writes keep working;
// a new overlap is refused.
func TestSchema79DatabaseWithOverlapsUpgradesAndKeepsWorking(t *testing.T) {
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "panel79.sqlite")
	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(createSchemaMigrationsSQL); err != nil {
		t.Fatal(err)
	}
	for _, migration := range embedded {
		if migration.version >= hostnameNamespaceTenantsMigrationVersion {
			break
		}
		if _, err := raw.Exec(string(migration.content)); err != nil {
			t.Fatalf("apply %s: %v", migration.filename, err)
		}
		if _, err := raw.Exec(`INSERT INTO schema_migrations (version, filename, sha256) VALUES (?, ?, ?)`,
			migration.version, migration.filename, migration.sha256); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES
			(1, 'admin', 'h', 'admin@example.test', 'admin'),
			(2, 'alice', 'h', 'alice@example.test', 'customer'),
			(3, 'bob', 'h', 'bob@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (1, 1, 'admin'), (2, 2, 'alice'), (3, 3, 'bob'), (4, 3, 'bob-two');
		INSERT INTO panel_settings (key, value) VALUES ('nameserver1', 'ns1.celikhost.com'), ('server_setup_infrastructure_zone:infra.test', '{}');
		INSERT INTO pdns_domains (name, type) VALUES ('infra.test', 'NATIVE');
		INSERT INTO domains (id, subscription_id, name) VALUES
			(10, 2, 'victim.test'),
			(11, 3, 'shop.victim.test'),
			(12, 3, 'infra.test'),
			(13, 3, 'mail.celikhost.com');
		INSERT INTO domain_aliases (domain_id, alias) VALUES (11, 'alias.victim.test');
	`); err != nil {
		t.Fatal(err)
	}
	tables := []string{"domains", "domain_aliases", "hostname_reservations", "subscriptions", "pdns_domains", "panel_settings"}
	before := map[string]string{}
	columns := map[string][]string{}
	for _, table := range tables {
		columns[table] = tableColumns(t, raw, table)
		before[table] = dumpRows(t, raw, table, columns[table])
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatalf("open the schema-79 database holding overlaps with this release: %v", err)
	}
	defer database.Close()
	db := database.GetDB()
	for _, table := range tables {
		if after := dumpRows(t, db, table, columns[table]); after != before[table] {
			t.Errorf("%s changed by the upgrade:\nbefore %s\nafter  %s", table, before[table], after)
		}
	}
	var recorded string
	if err := db.QueryRow(`SELECT filename FROM schema_migrations WHERE version = ?`, hostnameNamespaceTenantsMigrationVersion).Scan(&recorded); err != nil ||
		recorded != "080_hostname_namespace_tenants.sql" {
		t.Fatalf("ledger entry = %q, %v", recorded, err)
	}
	// Writes that give no new name keep working on the overlapping rows.
	for _, stmt := range []string{
		`UPDATE domains SET status = 'suspended' WHERE id = 11`,
		`UPDATE domains SET status = 'active' WHERE id = 11`,
		`INSERT INTO domain_aliases (domain_id, alias) VALUES (11, 'bob-own.test')`,
		`DELETE FROM domain_aliases WHERE alias = 'alias.victim.test'`,
		`INSERT INTO domains (subscription_id, name) VALUES (3, 'bob.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'blog.victim.test')`,
		`UPDATE subscriptions SET name = 'renamed' WHERE id = 3`,
		`DELETE FROM domains WHERE id = 13`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	// A name given now is held to the rule.
	for _, stmt := range []string{
		`INSERT INTO domains (subscription_id, name) VALUES (3, 'vpn.victim.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (3, 'x.infra.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (3, 'celikhost.com')`,
		`INSERT INTO domain_aliases (domain_id, alias) VALUES (11, 'www3.victim.test')`,
		`UPDATE subscriptions SET owner_id = 2 WHERE id = 3`,
		`UPDATE domains SET name = 'new.victim.test' WHERE name = 'bob.test'`,
	} {
		if _, err := db.Exec(stmt); err == nil || !strings.Contains(err.Error(), "hostname namespace conflict") {
			t.Fatalf("%s was accepted: %v", stmt, err)
		}
	}
	// The administrator's own subscription may hold such a name on purpose.
	if _, err := db.Exec(`INSERT INTO domains (subscription_id, name) VALUES (1, 'shop2.victim.test')`); err != nil {
		t.Fatal(err)
	}
	// A subscription whose names meet only its new owner's moves.
	if _, err := db.Exec(`INSERT INTO domains (subscription_id, name) VALUES (4, 'bob-two.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE subscriptions SET owner_id = 2 WHERE id = 4`); err != nil {
		t.Fatal(err)
	}
}
