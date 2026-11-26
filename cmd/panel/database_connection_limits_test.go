package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.128 (G5): database users get their subscription's connection limit
// on the engine, when created and when the limit changes. alpha.149: through
// the one function (database_user_limits.go).

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
	var typeID int
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, root_password_encrypted, status) VALUES (9941, 9802, ?, 'limits', '11.4', '127.0.0.1', 3306, ?, 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'sub9802_app', 'x')`)

	// No limit: a new user is not touched.
	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "sub9802_app"); outcome.State != dbLimitApplied || len(driver.set) != 0 {
		t.Fatalf("no limit wrote %v (%+v)", driver.set, outcome)
	}
	f.exec(t, `UPDATE setting_policy SET server_ceiling = 25 WHERE key = 'db.max_user_connections'`)
	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "sub9802_app"); outcome.State != dbLimitApplied || driver.set["sub9802_app"] != 25 {
		t.Fatalf("new user: %v (%+v)", driver.set, outcome)
	}

	// Existing users are updated through the same path when a plan lowers it.
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'sub9802_old', 'x')`)
	f.exec(t, `INSERT INTO service_plans (id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES (9951, 'DB', 0, 0, 0, 0, 0)`)
	f.exec(t, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9951, 'db.max_user_connections', 10)`)
	f.exec(t, `UPDATE subscriptions SET plan_id = 9951 WHERE id = 9802`)
	subs, err := f.panel.subscriptionsReachedBy(ctx, rightsOwnerPlan, 9951)
	if err != nil || len(subs) != 1 || subs[0] != 9802 {
		t.Fatalf("subscriptions on the plan %v (%v)", subs, err)
	}
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, subs, nil); err != nil || driver.set["sub9802_old"] != 10 {
		t.Fatalf("existing user: %v (%v)", driver.set, err)
	}
	// Lifting the limit removes it (0 = none on the engine).
	f.exec(t, `DELETE FROM setting_ceilings`)
	f.exec(t, `UPDATE setting_policy SET server_ceiling = NULL WHERE key = 'db.max_user_connections'`)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, subs, nil); err != nil || driver.set["sub9802_old"] != 0 {
		t.Fatalf("lifted limit: %v (%v)", driver.set, err)
	}
	if !dbConnectionLimitChanged(map[string]int{}, map[string]int{settingDBConnections: 5}) || dbConnectionLimitChanged(map[string]int{settingPHPMemoryMB: 1}, map[string]int{settingPHPMemoryMB: 2}) {
		t.Fatal("change detection")
	}
}

// pgAgentRequests records the Agent's role-limit calls (alpha.149) and
// answers them from an in-memory role.
type pgAgentRequests struct {
	mu       sync.Mutex
	requests []transport.PostgreSQLRoleLimitsRequest
	conn     map[string]int
	settings map[string]map[string]string
	answer   func(req *transport.PostgreSQLRoleLimitsRequest) (transport.PostgreSQLRoleLimitsResponse, error)
}

func withPGAgentLimits(t *testing.T) *pgAgentRequests {
	t.Helper()
	agent := &pgAgentRequests{conn: map[string]int{}, settings: map[string]map[string]string{}}
	previous := pgRoleLimitsThroughAgent
	pgRoleLimitsThroughAgent = func(_ *Panel, _ context.Context, req *transport.PostgreSQLRoleLimitsRequest) (transport.PostgreSQLRoleLimitsResponse, error) {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		agent.requests = append(agent.requests, *req)
		if agent.answer != nil {
			return agent.answer(req)
		}
		settings := agent.settings[req.Role]
		if settings == nil {
			settings = map[string]string{}
			agent.settings[req.Role] = settings
		}
		if _, ok := agent.conn[req.Role]; !ok {
			agent.conn[req.Role] = -1
		}
		for _, write := range req.Writes {
			switch {
			case write.Key == transport.PGRoleLimitConnection && write.Value == 0:
				agent.conn[req.Role] = -1
			case write.Key == transport.PGRoleLimitConnection:
				agent.conn[req.Role] = write.Value
			case write.Value == 0:
				delete(settings, write.Key)
			default:
				settings[write.Key] = dbRoleWrittenText(write.Key, write.Value)
			}
		}
		out := map[string]string{}
		for _, key := range services.RoleKeys {
			out[key] = settings[key]
		}
		return transport.PostgreSQLRoleLimitsResponse{Applied: true, ConnectionLimit: agent.conn[req.Role], Settings: out}, nil
	}
	t.Cleanup(func() { pgRoleLimitsThroughAgent = previous })
	return agent
}

// refusingDriver fails the test when the panel's own account is used to
// change a PostgreSQL role (alpha.149, lab D10).
type refusingDriver struct {
	services.DatabaseDriver
	t *testing.T
}

func (d *refusingDriver) SetUserConnectionLimit(username string, _ int) error {
	d.t.Errorf("the panel's own account changed %s's connection limit", username)
	return nil
}

// Integration (alpha.128): a PostgreSQL CONNECTION LIMIT goes to the role's
// own running cluster, by the alpha.127 placement rule, never to whichever
// cluster answers on the record's port. Boston's shape: 16/main removed with
// its data kept (still configured on 5432), 18/main running on 5433.
// alpha.149 (lab D10): on this machine it goes through the Agent as the
// superuser, never through the panel's own account.
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
	previous := newDatabaseDriver
	newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) {
		return &refusingDriver{t: t}, nil
	}
	t.Cleanup(func() { newDatabaseDriver = previous })
	agent := withPGAgentLimits(t)
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
	// CelikPanel's own records (its own account; alpha.149 finding 8).
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, cluster, root_password_encrypted, admin_username, status) VALUES (9961, 9802, ?, 'pg18', '18', '127.0.0.1', 5432, '18/main', ?, 'celikpanel_admin', 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, cluster, root_password_encrypted, admin_username, status) VALUES (9962, 9802, ?, 'pg16', '16', '127.0.0.1', 5431, '16/main', ?, 'celikpanel_admin', 'active')`, typeID, sealed)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'sub9802_pg18', 'x')`)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9962, 9802, 'sub9802_pg16', 'x')`)
	f.exec(t, `UPDATE setting_policy SET server_ceiling = 12 WHERE key = 'db.max_user_connections'`)

	outcomes, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if agent.conn["sub9802_pg18"] != 12 {
		t.Fatalf("the role of 18/main did not get its limit: %+v", agent.requests)
	}
	for _, req := range agent.requests {
		if req.Port != 5433 || req.Role != "sub9802_pg18" {
			t.Fatalf("a limit went elsewhere than 18/main's port: %+v", agent.requests)
		}
	}
	// The role on the removed cluster's record: nothing written, reported as
	// an unmet prerequisite.
	states := map[string]string{}
	for _, outcome := range outcomes {
		states[outcome.User] = outcome.State
	}
	if states["sub9802_pg18"] != dbLimitApplied || states["sub9802_pg16"] != dbLimitPrerequisite {
		t.Fatalf("outcomes: %+v", outcomes)
	}

	// Without postgresql-common the record is used as it is.
	clusters = transport.PostgreSQLClustersResponse{}
	agent.requests = nil
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9962, 9802, 'sub9802_plain', 'x')`)
	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9962, "sub9802_plain"); outcome.State != dbLimitApplied || agent.conn["sub9802_plain"] != 12 {
		t.Fatalf("one PostgreSQL without clusters: %+v %+v", outcome, agent.requests)
	}
	for _, req := range agent.requests {
		if req.Port != 5431 {
			t.Fatalf("without clusters the record's port: %+v", agent.requests)
		}
	}
}
