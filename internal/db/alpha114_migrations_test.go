package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// alpha.114 ships five migrations, applied in this order on top of 046:
// 047_panel_port_change, 048_accounts_plans_resellers, 049_account_recovery,
// 050_site_protect_redirect_recovery_codes and 051_mail_autoresponders.
// Every one is additive. A release before one of them refuses a ledger that
// carries it; a return is the pre-update snapshot restore, as for every
// migration (there is no down migration).
// alpha.114 beÅŸ gÃ¶Ã§ taÅŸÄ±r (047â€“051); hepsi eklemelidir, var olan satÄ±r deÄŸiÅŸmez.
const (
	panelPortChangeMigrationVersion = 47
	newestMigrationVersion          = panelOperationsMigrationVersion
)

// requireMigrationsFrom proves that the embedded list ends at the newest
// migration this release ships and that it holds every version from `from`
// to that newest one without a gap, so `older` is exactly what the release
// before `from` knew.
func requireMigrationsFrom(t *testing.T, embedded, older []embeddedMigration, from int) {
	t.Helper()
	if len(embedded) == 0 || embedded[len(embedded)-1].version != newestMigrationVersion {
		t.Fatalf("the newest embedded migration is not %d", newestMigrationVersion)
	}
	if len(older) != len(embedded)-(newestMigrationVersion-from+1) {
		t.Fatalf("the embedded migrations from %d are not exactly %d to %d", from, from, newestMigrationVersion)
	}
}

func TestOlderReleaseRefusesALedgerWithThePanelPortChangeEntry(t *testing.T) {
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
		if migration.version < panelPortChangeMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, panelPortChangeMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}

// A schema-46 database (alpha.113, with its ledger) opened by this release
// takes migrations 47 to 51 in order and keeps every row and value it had.
// Åema-46 veritabanÄ± 47â€“51 gÃ¶Ã§lerini sÄ±rayla alÄ±r; her satÄ±r aynen kalÄ±r.
func TestSchema46DatabaseUpgradesThroughMigration51WithEveryRowIntact(t *testing.T) {
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := map[int]string{
		47: "047_panel_port_change.sql",
		48: "048_accounts_plans_resellers.sql",
		49: "049_account_recovery.sql",
		50: "050_site_protect_redirect_recovery_codes.sql",
		51: "051_mail_autoresponders.sql",
	}
	for _, migration := range embedded {
		if want, ok := wantFiles[migration.version]; ok && migration.filename != want {
			t.Fatalf("migration %d is %s, want %s", migration.version, migration.filename, want)
		}
	}

	path := filepath.Join(t.TempDir(), "panel46.sqlite")
	raw, err := sql.Open("sqlite", fmt.Sprintf("%s?_pragma=foreign_keys(1)", path))
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(createSchemaMigrationsSQL); err != nil {
		t.Fatal(err)
	}
	for _, migration := range embedded {
		if migration.version > postgresqlClustersMigrationVersion {
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
		INSERT INTO users (id, username, password_hash, email, role, parent_id) VALUES
			(500, 'old-reseller', 'hash-r', 'old-reseller@example.test', 'reseller', NULL),
			(501, 'old-customer', 'hash-c', 'old-customer@example.test', 'customer', 500);
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ('hash-501', 501, '2999-01-01T00:00:00Z');
		INSERT INTO audit_logs (user_id, action) VALUES (500, 'user.create:old-customer/customer');
		INSERT INTO subscriptions (id, owner_id, name) VALUES (502, 501, 'kept-subscription');
		INSERT INTO domains (id, subscription_id, name) VALUES (503, 502, 'kept.example.test');
		INSERT INTO sites (id, domain_id, document_root) VALUES (504, 503, '/srv/kept.example.test');
		INSERT INTO email_accounts (id, domain_id, address, password_hash) VALUES (505, 503, 'box@kept.example.test', 'hash-m');
	`); err != nil {
		t.Fatal(err)
	}
	tables := []string{"users", "sessions", "audit_logs", "subscriptions", "domains", "sites", "email_accounts"}
	before := make(map[string]string, len(tables))
	columns := make(map[string][]string, len(tables))
	for _, table := range tables {
		columns[table] = tableColumns(t, raw, table)
		if table == "subscriptions" {
			// 057 (alpha.118) sets the never-applied traffic limit to 0;
			// that one value is checked below.
			kept := columns[table][:0]
			for _, column := range columns[table] {
				if column != "bandwidth_quota_mb" {
					kept = append(kept, column)
				}
			}
			columns[table] = kept
		}
		before[table] = dumpRows(t, raw, table, columns[table])
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := NewSQLiteDB(path)
	if err != nil {
		t.Fatalf("open the schema-46 database with this release: %v", err)
	}
	defer database.Close()
	upgraded := database.GetDB()
	for _, table := range tables {
		if after := dumpRows(t, upgraded, table, columns[table]); after != before[table] {
			t.Errorf("%s changed by the upgrade:\nbefore %s\nafter  %s", table, before[table], after)
		}
	}
	var bandwidth int
	if err := upgraded.QueryRow(`SELECT bandwidth_quota_mb FROM subscriptions WHERE id = 502`).Scan(&bandwidth); err != nil || bandwidth != 0 {
		t.Errorf("the never-applied traffic limit after 057 = %d, %v; want 0 (unlimited)", bandwidth, err)
	}
	for version, filename := range wantFiles {
		var recorded string
		if err := upgraded.QueryRow(`SELECT filename FROM schema_migrations WHERE version = ?`, version).Scan(&recorded); err != nil || recorded != filename {
			t.Errorf("ledger entry %d = %q, %v", version, recorded, err)
		}
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM panel_port_changes`,
		`SELECT COUNT(*) FROM reseller_pools`,
		`SELECT COUNT(*) FROM sessions WHERE impersonator_id IS NOT NULL`,
		`SELECT COUNT(*) FROM audit_logs WHERE acting_as IS NOT NULL`,
		`SELECT COUNT(*) FROM account_links`,
		`SELECT COUNT(*) FROM users WHERE email_verified_at IS NOT NULL`,
		`SELECT COUNT(*) FROM site_protected_dirs`,
		`SELECT COUNT(*) FROM site_redirects`,
		`SELECT COUNT(*) FROM two_factor_recovery_codes`,
		`SELECT COUNT(*) FROM email_autoresponders`,
	} {
		var count int
		if err := upgraded.QueryRow(query).Scan(&count); err != nil || count != 0 {
			t.Errorf("%s = %d, %v", query, count, err)
		}
	}
}

func tableColumns(t *testing.T, database *sql.DB, table string) []string {
	t.Helper()
	rows, err := database.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil || len(names) == 0 {
		t.Fatalf("columns of %s: %v %v", table, names, err)
	}
	return names
}

func dumpRows(t *testing.T, database *sql.DB, table string, columns []string) string {
	t.Helper()
	rows, err := database.Query(`SELECT ` + strings.Join(columns, ", ") + ` FROM ` + table + ` ORDER BY rowid`)
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer rows.Close()
	var out strings.Builder
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for index := range values {
		pointers[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&out, "%v;", values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
