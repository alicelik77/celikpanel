package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/services"
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
	if err := f.panel.applyNewUserConnectionLimit(ctx, driver, 9802, "sub9802_app"); err != nil || len(driver.set) != 0 {
		t.Fatalf("no limit wrote %v (%v)", driver.set, err)
	}
	f.exec(t, `UPDATE setting_policy SET server_ceiling = 25 WHERE key = 'db.max_user_connections'`)
	if err := f.panel.applyNewUserConnectionLimit(ctx, driver, 9802, "sub9802_app"); err != nil || driver.set["sub9802_app"] != 25 {
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
