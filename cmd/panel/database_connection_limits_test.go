package main

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G5): database users get their subscription's connection limit
// on the engine, when created and when the limit changes.

type connectionLimitDriver struct {
	services.DatabaseDriver
	mu  sync.Mutex
	set map[string]int
}

func (d *connectionLimitDriver) SetUserConnectionLimit(username string, limit int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.set[username] = limit
	return nil
}

func TestDatabaseUsersFollowTheConnectionLimit(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "database-secrets.key"))
	if err != nil {
		t.Fatal(err)
	}
	f.panel.secrets = box
	sealed, err := box.Encrypt("root-secret")
	if err != nil {
		t.Fatal(err)
	}
	driver := &connectionLimitDriver{set: map[string]int{}}
	previous := newDatabaseDriver
	newDatabaseDriver = func(services.DriverConfig) (services.DatabaseDriver, error) { return driver, nil }
	t.Cleanup(func() { newDatabaseDriver = previous })

	// No limit: a new user is not touched.
	if err := f.panel.applyNewUserConnectionLimit(ctx, &core.DatabaseServer{TypeName: "mariadb", Host: "127.0.0.1", Port: 3306}, driver, 9802, "sub9802_app"); err != nil || len(driver.set) != 0 {
		t.Fatalf("no limit wrote %v (%v)", driver.set, err)
	}
	f.exec(t, `UPDATE setting_policy SET server_ceiling = 25 WHERE key = 'db.max_user_connections'`)
	if err := f.panel.applyNewUserConnectionLimit(ctx, &core.DatabaseServer{TypeName: "mariadb", Host: "127.0.0.1", Port: 3306}, driver, 9802, "sub9802_app"); err != nil || driver.set["sub9802_app"] != 25 {
		t.Fatalf("new user: %v (%v)", driver.set, err)
	}

	// Existing users are updated through the same path when a plan lowers it.
	var typeID int
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, root_password_encrypted, status) VALUES (9941, 9802, ?, 'limits', '11.4', '127.0.0.1', 3306, ?, 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'sub9802_old', 'x')`)
	f.exec(t, `INSERT INTO service_plans (id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES (9951, 'DB', 0, 0, 0, 0, 0)`)
	f.exec(t, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9951, 'db.max_user_connections', 10)`)
	f.exec(t, `UPDATE subscriptions SET plan_id = 9951 WHERE id = 9802`)
	subs, err := f.panel.subscriptionsReachedBy(ctx, rightsOwnerPlan, 9951)
	if err != nil || len(subs) != 1 || subs[0] != 9802 {
		t.Fatalf("subscriptions on the plan %v (%v)", subs, err)
	}
	if err := f.panel.reapplyDBConnectionLimitsNow(ctx, subs); err != nil || driver.set["sub9802_old"] != 10 {
		t.Fatalf("existing user: %v (%v)", driver.set, err)
	}
	// Lifting the limit removes it (0 = none on the engine).
	f.exec(t, `DELETE FROM setting_ceilings`)
	f.exec(t, `UPDATE setting_policy SET server_ceiling = NULL WHERE key = 'db.max_user_connections'`)
	if err := f.panel.reapplyDBConnectionLimitsNow(ctx, subs); err != nil || driver.set["sub9802_old"] != 0 {
		t.Fatalf("lifted limit: %v (%v)", driver.set, err)
	}
	if !dbConnectionLimitChanged(map[string]int{}, map[string]int{settingDBConnections: 5}) || dbConnectionLimitChanged(map[string]int{settingPHPMemoryMB: 1}, map[string]int{settingPHPMemoryMB: 2}) {
		t.Fatal("change detection")
	}
}

// portLimitDriver records, per port it was opened for, the limits written.
type portLimitDriver struct {
	services.DatabaseDriver
	port   int
	writes *map[int]map[string]int
	mu     *sync.Mutex
}

func (d *portLimitDriver) SetUserConnectionLimit(username string, limit int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if (*d.writes)[d.port] == nil {
		(*d.writes)[d.port] = map[string]int{}
	}
	(*d.writes)[d.port][username] = limit
	return nil
}

// Integration (alpha.128): a PostgreSQL CONNECTION LIMIT goes to the role's
// own running cluster, by the alpha.127 placement rule, never to whichever
// cluster answers on the record's port. Boston's shape: 16/main removed with
// its data kept (still configured on 5432), 18/main running on 5433.
func TestAPostgreSQLConnectionLimitGoesToTheRolesOwnRunningCluster(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "database-secrets.key"))
	if err != nil {
		t.Fatal(err)
	}
	f.panel.secrets = box
	sealed, err := box.Encrypt("pg-secret")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	writes := map[int]map[string]int{}
	previous := newDatabaseDriver
	newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) {
		return &portLimitDriver{port: config.Port, writes: &writes, mu: &mu}, nil
	}
	t.Cleanup(func() { newDatabaseDriver = previous })
	clusters := transport.PostgreSQLClustersResponse{Supported: true, Clusters: []transport.PostgreSQLCluster{
		{Version: "16", Name: "main", Port: 5432, Status: "down,binaries_missing", Online: false, DataDirectory: "/var/lib/postgresql/16/main"},
		{Version: "18", Name: "main", Port: 5433, Status: "online", Online: true},
	}}
	previousClusters := readPGClustersFromAgent
	readPGClustersFromAgent = func(*Panel, context.Context) (transport.PostgreSQLClustersResponse, error) { return clusters, nil }
	t.Cleanup(func() { readPGClustersFromAgent = previousClusters })

	var typeID int
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'postgresql'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	// Record 9961 names 18/main but still carries the old port 5432; record
	// 9962 names the removed 16/main (a stale port 5431 stored with it).
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, cluster, root_password_encrypted, status) VALUES (9961, 9802, ?, 'pg18', '18', '127.0.0.1', 5432, '18/main', ?, 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, cluster, root_password_encrypted, status) VALUES (9962, 9802, ?, 'pg16', '16', '127.0.0.1', 5431, '16/main', ?, 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'sub9802_pg18', 'x')`)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9962, 9802, 'sub9802_pg16', 'x')`)
	f.exec(t, `UPDATE setting_policy SET server_ceiling = 12 WHERE key = 'db.max_user_connections'`)

	if err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}); err != nil {
		t.Fatal(err)
	}
	if writes[5433]["sub9802_pg18"] != 12 {
		t.Fatalf("the role of 18/main did not get its limit on 18/main's port: %v", writes)
	}
	if _, wrote := writes[5432]; wrote {
		t.Fatalf("a limit went to port 5432, where the removed 16/main is configured: %v", writes)
	}

	// A new user on the removed cluster's record is refused, nothing written.
	server := &core.DatabaseServer{ID: 9962, SubscriptionID: 9802, TypeName: "postgresql", Host: "127.0.0.1", Port: 5431}
	stale := &portLimitDriver{port: 5431, writes: &writes, mu: &mu}
	err = f.panel.applyNewUserConnectionLimit(ctx, server, stale, 9802, "sub9802_new16")
	if refusal := (*pgClusterRefusalError)(nil); err == nil || !errors.As(err, &refusal) || refusal.refusal.Reason != "removed" {
		t.Fatalf("a limit for a role on the removed cluster: %v", err)
	}
	if _, wrote := writes[5431]; wrote {
		t.Fatalf("a limit went to the removed cluster's record port: %v", writes)
	}

	// Without postgresql-common the record is used as it is.
	clusters = transport.PostgreSQLClustersResponse{}
	plain := &portLimitDriver{port: 5431, writes: &writes, mu: &mu}
	if err := f.panel.applyNewUserConnectionLimit(ctx, server, plain, 9802, "sub9802_plain"); err != nil || writes[5431]["sub9802_plain"] != 12 {
		t.Fatalf("one PostgreSQL without clusters: %v (%v)", writes, err)
	}
}
