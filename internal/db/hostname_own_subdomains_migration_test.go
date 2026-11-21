package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Migration 81 (fix round after the alpha.142 lab, 21 November 2026): a
// subdomain of one's own domain is an ordinary subdomain also when a server
// name lies above that domain. Triggers only: the six triggers of 080 are
// created again with the same names; no table, column or row changes, so a
// database that holds overlaps upgrades. Going back is the pre-update
// snapshot restore, as for every migration (D-025).
const hostnameOwnSubdomainsMigrationVersion = 81

func TestOlderReleaseRefusesALedgerWithTheOwnSubdomainEntry(t *testing.T) {
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
		if migration.version < hostnameOwnSubdomainsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, hostnameOwnSubdomainsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// A schema-80 database shaped like the alpha.142 lab (customers' domains
// under the server's zone dr.test, made before the rule; the server's mail
// host name inside a customer's domain) upgrades with every row intact.
// Afterwards a customer may add a name inside a domain it already holds under
// the server's zone, and every other server-name case stays refused.
func TestSchema80DatabaseWithServerZoneOverlapsUpgradesAndLetsOwnSubdomainsIn(t *testing.T) {
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "panel80.sqlite")
	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(createSchemaMigrationsSQL); err != nil {
		t.Fatal(err)
	}
	for _, migration := range embedded {
		if migration.version >= hostnameOwnSubdomainsMigrationVersion {
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
	// The names are given first, the server's names after them: what an
	// installation made before migration 80 holds.
	if _, err := raw.Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES
			(1, 'admin', 'h', 'admin@example.test', 'admin'),
			(2, 'alice', 'h', 'alice@example.test', 'customer'),
			(3, 'bob', 'h', 'bob@example.test', 'customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (1, 1, 'admin'), (2, 2, 'alice'), (3, 3, 'bob'), (4, 3, 'bob-two');
		INSERT INTO domains (id, subscription_id, name) VALUES
			(10, 2, 'kahvedukkani.dr.test'),
			(11, 3, 'pastane.dr.test'),
			(12, 2, 'example.com');
		INSERT INTO domain_aliases (id, domain_id, alias) VALUES (30, 12, 'al.dr.test');
		INSERT INTO pdns_domains (name, type) VALUES ('dr.test', 'NATIVE');
		INSERT INTO panel_settings (key, value) VALUES
			('server_setup_infrastructure_zone:dr.test', '{}'),
			('mail_hostname', 'mail.example.com'),
			('nameserver1', 'ns1.zone.example.com');
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
	// Under 080 alone the lab's own subdomain is refused.
	if _, err := raw.Exec(`INSERT INTO domains (subscription_id, name) VALUES (2, 'blog.kahvedukkani.dr.test')`); err == nil {
		t.Fatal("the schema-80 database accepted the lab's subdomain: the case does not reproduce")
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatalf("open the schema-80 database holding overlaps with this release: %v", err)
	}
	defer database.Close()
	db := database.GetDB()
	for _, table := range tables {
		if after := dumpRows(t, db, table, columns[table]); after != before[table] {
			t.Errorf("%s changed by the upgrade:\nbefore %s\nafter  %s", table, before[table], after)
		}
	}
	var recorded string
	if err := db.QueryRow(`SELECT filename FROM schema_migrations WHERE version = ?`, hostnameOwnSubdomainsMigrationVersion).Scan(&recorded); err != nil ||
		recorded != "081_hostname_own_subdomains.sql" {
		t.Fatalf("ledger entry = %q, %v", recorded, err)
	}
	var triggers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type = 'trigger' AND name LIKE '%namespace_tenant%'`).Scan(&triggers); err != nil || triggers != 6 {
		t.Fatalf("name rule triggers = %d, %v", triggers, err)
	}
	// Inside a domain the account holds under the server's zone, also
	// through an alias and a second subscription of the same account.
	for _, stmt := range []string{
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'blog.kahvedukkani.dr.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'x.y.kahvedukkani.dr.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'shop.al.dr.test')`,
		`INSERT INTO domain_aliases (domain_id, alias) VALUES (10, 'www2.kahvedukkani.dr.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (4, 'cake.pastane.dr.test')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	// Still refused: the server name itself, a name under it without such
	// a domain, a name inside the account's own domain with a server name
	// under it (the nameserver ns1.zone.example.com), the server's mail host
	// name inside the
	// customer's domain (between the domain and the name), another
	// account's domain under the zone, a rename that leaves the zone without
	// the domain it relied on, an alias moved to a name only its own row
	// covered, and a subscription whose names meet only the old owner's.
	for _, stmt := range []string{
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'dr.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'other.dr.test')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'zone.example.com')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'mail.example.com')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'x.mail.example.com')`,
		`INSERT INTO domains (subscription_id, name) VALUES (2, 'blog.pastane.dr.test')`,
		`INSERT INTO domain_aliases (domain_id, alias) VALUES (10, 'x.pastane.dr.test')`,
		`UPDATE domains SET name = 'sub.kahvedukkani.dr.test' WHERE id = 10`,
		`UPDATE domain_aliases SET alias = 'deep.al.dr.test' WHERE id = 30`,
		`UPDATE subscriptions SET owner_id = 3 WHERE id = 2`,
	} {
		if _, err := db.Exec(stmt); err == nil || !strings.Contains(err.Error(), "hostname namespace conflict") {
			t.Fatalf("%s was accepted: %v", stmt, err)
		}
	}
	// The administrator's own subscription may hold such a name on purpose.
	if _, err := db.Exec(`INSERT INTO domains (subscription_id, name) VALUES (1, 'admin.dr.test')`); err != nil {
		t.Fatal(err)
	}
}
