package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Migration 52 (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md section 9): the
// rights tables, the subscription lifecycle columns and the migration of
// every existing account shape. No existing value changes; the only new rows
// outside the new tables are the Unlimited subscriptions of customers that
// had none. The release before it refuses a ledger that carries entry 52; a
// return to it is the pre-update snapshot restore, as for every migration.
const rightsSetsMigrationVersion = 52

func openSchema51WithOldShapes(t *testing.T) (*sql.DB, embeddedMigration) {
	t.Helper()
	embedded, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	dsn := fmt.Sprintf("%s?_pragma=foreign_keys(1)", filepath.Join(t.TempDir(), "panel51.sqlite"))
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	var target *embeddedMigration
	for i := range embedded {
		if embedded[i].version == rightsSetsMigrationVersion {
			target = &embedded[i]
			continue
		}
		if embedded[i].version > rightsSetsMigrationVersion {
			continue
		}
		if _, err := raw.Exec(string(embedded[i].content)); err != nil {
			t.Fatalf("apply %s: %v", embedded[i].filename, err)
		}
	}
	if target == nil {
		t.Fatal("migration 52 is not embedded")
	}
	// Every account shape the alpha.114-116 releases can hold:
	//   1   administrator with its own workspace (no plan)
	//   20  reseller with a pool; 21 reseller without a pool
	//   30  customer of 20, subscription 300 bound to plan 10, VPN add-on
	//   31  customer of 21, subscription 310 without a plan, VPN add-on
	//       active and the app installer add-on suspended
	//   32  customer without a subscription
	//   33  customer without a plan and without add-ons (subscription 330)
	//   34  team member of 33 (never gets a subscription)
	//   35  suspended customer without a subscription
	if _, err := raw.Exec(`
		INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES
			(1, 'admin', 'h', 'admin@example.test', 'admin', NULL, 'active'),
			(20, 'res-pool', 'h', 'r20@example.test', 'reseller', NULL, 'active'),
			(21, 'res-free', 'h', 'r21@example.test', 'reseller', NULL, 'active'),
			(30, 'cust-plan', 'h', 'c30@example.test', 'customer', 20, 'active'),
			(31, 'cust-custom', 'h', 'c31@example.test', 'customer', 21, 'active'),
			(32, 'cust-none', 'h', 'c32@example.test', 'customer', 1, 'active'),
			(33, 'cust-bare', 'h', 'c33@example.test', 'customer', NULL, 'active'),
			(35, 'cust-suspended', 'h', 'c35@example.test', 'customer', NULL, 'suspended');
		INSERT INTO users (id, username, password_hash, email, role, parent_id, status, account_type) VALUES
			(34, 'member', 'h', 'm34@example.test', 'customer', 33, 'active', 'additional_user');
		INSERT INTO service_plans (id, owner_id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES
			(10, NULL, 'Basic', 3, 3, 3, 300, 0),
			(11, 20, 'Reseller own', 2, 2, 2, 200, 0);
		INSERT INTO subscriptions (id, owner_id, name, plan_id, max_domains, max_databases, max_email_accounts, disk_quota_mb, status, expires_at) VALUES
			(100, 1, 'Admin Subscription', NULL, 999, 999, 999, 999999, 'active', NULL),
			(300, 30, 'Basic', 10, 3, 3, 3, 300, 'active', NULL),
			(310, 31, 'hand made', NULL, 3, 4, 5, 600, 'active', '2030-01-01T00:00:00Z'),
			(330, 33, 'bare', NULL, 1, 1, 1, 100, 'suspended', NULL);
		INSERT INTO reseller_pools (reseller_id, max_customers, max_domains, max_databases, max_email_accounts, disk_quota_mb) VALUES
			(20, 5, 50, 0, 0, 1000);
		INSERT INTO subscription_entitlements (subscription_id, product_id, status) VALUES
			(300, 'vpn', 'active'),
			(310, 'vpn', 'active'),
			(310, 'app_installer', 'suspended');
	`); err != nil {
		t.Fatal(err)
	}
	return raw, *target
}

func setItems(t *testing.T, db *sql.DB, kind string, owner int) (allPermissions, allSoftware int, items []string, found bool) {
	t.Helper()
	var id int
	err := db.QueryRow(`SELECT id, all_permissions, all_software FROM rights_sets WHERE owner_kind = ? AND owner_id = ?`, kind, owner).
		Scan(&id, &allPermissions, &allSoftware)
	if err == sql.ErrNoRows {
		return 0, 0, nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT item FROM rights_set_items WHERE set_id = ? ORDER BY item`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	return allPermissions, allSoftware, items, true
}

func TestMigration52MapsEveryExistingAccountShape(t *testing.T) {
	raw, migration := openSchema51WithOldShapes(t)
	oldColumns := tableColumns(t, raw, "subscriptions")
	subscriptionsBefore := dumpRows(t, raw, "subscriptions", oldColumns)
	if _, err := raw.Exec(string(migration.content)); err != nil {
		t.Fatalf("apply migration 52 to a schema-51 database: %v", err)
	}

	// Plans: both keep their numbers, both offer everything.
	for _, plan := range []int{10, 11} {
		allPermissions, allSoftware, items, found := setItems(t, raw, "plan", plan)
		if !found || allPermissions != 1 || allSoftware != 1 || len(items) != 0 {
			t.Fatalf("plan %d set = %v %d %d %v, want everything offered", plan, found, allPermissions, allSoftware, items)
		}
	}
	// Plan-bound subscription stays bound and has no own set.
	if _, _, _, found := setItems(t, raw, "subscription", 300); found {
		t.Fatal("a plan-bound subscription was given its own set")
	}
	var planID sql.NullInt64
	if err := raw.QueryRow(`SELECT plan_id FROM subscriptions WHERE id = 300`).Scan(&planID); err != nil || planID.Int64 != 10 {
		t.Fatalf("subscription 300 left its plan: %v %v", planID, err)
	}
	// The administrator's own workspace stays Unlimited.
	if _, _, _, found := setItems(t, raw, "subscription", 100); found {
		t.Fatal("the administrator's workspace was made Custom")
	}
	// Plan-less subscriptions become Custom: all software, every permission
	// except the add-on ones they do not hold actively.
	base := []string{"cron", "databases", "dns_edit", "dnssec", "file_access", "file_manager", "backups",
		"mail", "site_protect_redirects", "site_proxy", "ssh", "ssl_acme", "ssl_custom", "team_members"}
	want := func(extra ...string) []string {
		out := []string{}
		for _, id := range append(append([]string{}, base...), extra...) {
			out = append(out, "permission:"+id)
		}
		sort.Strings(out)
		return out
	}
	for sub, expected := range map[int][]string{310: want("vpn"), 330: want()} {
		allPermissions, allSoftware, items, found := setItems(t, raw, "subscription", sub)
		if !found || allPermissions != 0 || allSoftware != 1 {
			t.Fatalf("subscription %d set = %v %d %d", sub, found, allPermissions, allSoftware)
		}
		if strings.Join(items, ",") != strings.Join(expected, ",") {
			t.Fatalf("subscription %d items = %v, want %v", sub, items, expected)
		}
	}
	// Customers without a subscription get an Unlimited one, with a notice
	// for the administrator; the team member gets nothing.
	for _, customer := range []int{32, 35} {
		var name string
		var domains, databases, mailboxes, disk, bandwidth int
		var plan sql.NullInt64
		var startsAt sql.NullString
		if err := raw.QueryRow(`SELECT name, plan_id, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb, starts_at
			FROM subscriptions WHERE owner_id = ?`, customer).Scan(&name, &plan, &domains, &databases, &mailboxes, &disk, &bandwidth, &startsAt); err != nil {
			t.Fatalf("customer %d has no subscription: %v", customer, err)
		}
		if name != "Unlimited" || plan.Valid || domains != 0 || databases != 0 || mailboxes != 0 || disk != 0 || bandwidth != 0 || !startsAt.Valid {
			t.Fatalf("customer %d subscription = %s %v %d %d %d %d %d %v", customer, name, plan, domains, databases, mailboxes, disk, bandwidth, startsAt)
		}
		var notices int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM rights_notices WHERE kind = 'unlimited_after_migration' AND user_id = ? AND acknowledged_at IS NULL`, customer).Scan(&notices); err != nil || notices != 1 {
			t.Fatalf("customer %d notices = %d, %v", customer, notices, err)
		}
	}
	var memberSubs, totalNotices int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM subscriptions WHERE owner_id = 34`).Scan(&memberSubs); err != nil || memberSubs != 0 {
		t.Fatalf("the team member got a subscription: %d %v", memberSubs, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM rights_notices`).Scan(&totalNotices); err != nil || totalNotices != 2 {
		t.Fatalf("notices = %d, %v; want exactly the two customers", totalNotices, err)
	}
	// Resellers: with a pool -> Custom reseller with everything offered; the
	// pool numbers stay; without a pool -> Unlimited reseller (no rows).
	if allPermissions, allSoftware, items, found := setItems(t, raw, "reseller", 20); !found || allPermissions != 1 || allSoftware != 1 || len(items) != 0 {
		t.Fatalf("reseller 20 set = %v %d %d %v", found, allPermissions, allSoftware, items)
	}
	if _, _, _, found := setItems(t, raw, "reseller", 21); found {
		t.Fatal("a reseller without a pool was made Custom")
	}
	var poolDomains int
	if err := raw.QueryRow(`SELECT max_domains FROM reseller_pools WHERE reseller_id = 20`).Scan(&poolDomains); err != nil || poolDomains != 50 {
		t.Fatalf("pool changed: %d %v", poolDomains, err)
	}
	// Lifecycle: the unused 001 expiry is copied into ends_at; nothing else
	// of the existing rows changes.
	var endsAt sql.NullString
	if err := raw.QueryRow(`SELECT ends_at FROM subscriptions WHERE id = 310`).Scan(&endsAt); err != nil || endsAt.String != "2030-01-01T00:00:00Z" {
		t.Fatalf("ends_at = %v, %v", endsAt, err)
	}
	after := dumpRows(t, raw, "subscriptions WHERE id IN (100, 300, 310, 330)", oldColumns)
	if !strings.HasPrefix(subscriptionsBefore, after) || after == "" {
		t.Fatalf("existing subscription values changed:\n%s\n%s", subscriptionsBefore, after)
	}
}

func TestMigration52SetsFollowTheirOwnersAndRefuseBadRows(t *testing.T) {
	raw, migration := openSchema51WithOldShapes(t)
	if _, err := raw.Exec(string(migration.content)); err != nil {
		t.Fatal(err)
	}
	for name, statement := range map[string]string{
		"unknown owner kind": `INSERT INTO rights_sets (owner_kind, owner_id) VALUES ('team', 1)`,
		"flag 2":             `INSERT INTO rights_sets (owner_kind, owner_id, all_software) VALUES ('reseller', 21, 2)`,
		"duplicate owner":    `INSERT INTO rights_sets (owner_kind, owner_id) VALUES ('plan', 10)`,
		"negative quantity":  `INSERT INTO reseller_plans (name, max_domains) VALUES ('x', -1)`,
		"empty plan name":    `INSERT INTO reseller_plans (name) VALUES ('  ')`,
	} {
		if _, err := raw.Exec(statement); err == nil {
			t.Fatalf("%s was stored", name)
		}
	}
	// A bound reseller plan cannot be deleted; the binding ends with the reseller.
	if _, err := raw.Exec(`INSERT INTO reseller_plans (id, name, max_customers) VALUES (7, 'Gold', 3);
		INSERT INTO rights_sets (owner_kind, owner_id, all_permissions, all_software) VALUES ('reseller_plan', 7, 1, 0);
		INSERT INTO reseller_plan_bindings (reseller_id, reseller_plan_id) VALUES (21, 7);`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DELETE FROM reseller_plans WHERE id = 7`); err == nil {
		t.Fatal("a bound reseller plan was deleted")
	}
	count := func(query string) int {
		var n int
		if err := raw.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Deleting a subscription, a plan or a reseller removes its set.
	if _, err := raw.Exec(`DELETE FROM subscriptions WHERE id = 330`); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM rights_sets WHERE owner_kind = 'subscription' AND owner_id = 330`); n != 0 {
		t.Fatal("a deleted subscription's set remains")
	}
	if _, err := raw.Exec(`UPDATE subscriptions SET plan_id = NULL WHERE plan_id = 11; DELETE FROM service_plans WHERE id = 11`); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM rights_sets WHERE owner_kind = 'plan' AND owner_id = 11`); n != 0 {
		t.Fatal("a deleted plan's set remains")
	}
	if _, err := raw.Exec(`DELETE FROM subscriptions WHERE owner_id IN (SELECT id FROM users WHERE parent_id = 20);
		DELETE FROM service_plans WHERE owner_id = 20;
		DELETE FROM users WHERE parent_id = 20; DELETE FROM users WHERE id = 20`); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM rights_sets WHERE owner_kind = 'reseller' AND owner_id = 20`); n != 0 {
		t.Fatal("a deleted reseller's set remains")
	}
	if _, err := raw.Exec(`DELETE FROM users WHERE parent_id = 21; DELETE FROM users WHERE id = 21`); err != nil {
		t.Fatal(err)
	}
	if n := count(`SELECT COUNT(*) FROM reseller_plan_bindings`); n != 0 {
		t.Fatal("a deleted reseller's binding remains")
	}
	if _, err := raw.Exec(`DELETE FROM reseller_plans WHERE id = 7`); err != nil {
		t.Fatalf("an unbound reseller plan could not be deleted: %v", err)
	}
	if n := count(`SELECT COUNT(*) FROM rights_sets WHERE owner_kind = 'reseller_plan'`); n != 0 {
		t.Fatal("a deleted reseller plan's set remains")
	}
	// One open notice per kind, account and subject.
	if _, err := raw.Exec(`INSERT INTO rights_notices (kind, user_id, subject) VALUES ('reseller_pool_80', 1, 'domains')`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO rights_notices (kind, user_id, subject) VALUES ('reseller_pool_80', 1, 'domains')`); err == nil {
		t.Fatal("a second open notice was stored")
	}
}

func TestOlderReleaseRefusesALedgerWithTheRightsSetsEntry(t *testing.T) {
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
		if migration.version < rightsSetsMigrationVersion {
			older = append(older, migration)
		}
	}
	requireMigrationsFrom(t, embedded, older, rightsSetsMigrationVersion)
	err = database.verifyMigrationLedgerCoverage(t.Context(), older)
	database.Close()
	if err == nil || !strings.Contains(err.Error(), "no matching embedded migration") {
		t.Fatalf("an older release accepted the newer ledger: %v", err)
	}
}
