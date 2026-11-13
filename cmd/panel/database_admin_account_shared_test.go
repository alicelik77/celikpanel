package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5. CelikPanel's account on an engine of this machine is one
// account; every subscription's record of that engine holds a copy of its
// credential. The lab's refusal (DATABASE_ENGINE_CREDENTIAL_REFUSED when the
// administrator deleted a customer's database) was a record whose copy was
// older than the account's password: each later opening of the account, for
// another subscription or from the component page, re-keyed it and wrote
// only its own record. Every record of the engine now follows.
func TestThePanelsEngineAccountIsRecordedOnEveryRecordOfThatEngine(t *testing.T) {
	agent := &databaseAdminAccountAgent{services: adminAccountServices()}
	panel, database := newDatabaseAdminAccountFixture(t, agent)
	db := database.GetDB()
	const customerID, customerSubscription, ownerSubscription = 9831, 9832, 9833
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (?, 'shared-account-customer', 'x', 'shared@example.test', 'customer', 'active')`, customerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subscriptions (id, owner_id, name, status) VALUES (?1, ?2, 'customer', 'active'), (?3, ?2, 'owner-registered', 'active')`, customerSubscription, customerID, ownerSubscription); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := panel.ensureInstalledDBServers(ctx, adminAccountSubscriptionID); err != nil {
		t.Fatal(err)
	}
	// A record an administrator registered with a password of their own
	// (before R-057: no account name) is theirs and is never overwritten.
	sealedOwn, err := panel.secrets.Encrypt("the-owners-own-root-password")
	if err != nil {
		t.Fatal(err)
	}
	var mariadb int
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&mariadb); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, status, root_password_encrypted) VALUES (?, ?, 'mine', '11.4', 'localhost', 3306, 'active', ?)`, ownerSubscription, mariadb, sealedOwn); err != nil {
		t.Fatal(err)
	}
	// The customer's first listing claims their record and opens the account
	// again with a new password (as in the lab).
	if err := panel.ensureInstalledDBServers(ctx, customerSubscription); err != nil {
		t.Fatal(err)
	}
	if len(agent.provisioned) != 2 {
		t.Fatalf("the account was opened %d times, want twice", len(agent.provisioned))
	}
	repo := repositories.NewPostgresDatabaseServerRepository(db)
	record := func(subscription int) (string, string) {
		t.Helper()
		servers, err := repo.ListByType(ctx, subscription, "mariadb")
		if err != nil || len(servers) != 1 {
			t.Fatalf("records of %d: %v %d", subscription, err, len(servers))
		}
		if servers[0].AdminPasswordEncrypted == "" {
			return servers[0].AdminUsername, ""
		}
		plain, err := panel.secrets.Decrypt(servers[0].AdminPasswordEncrypted)
		if err != nil {
			t.Fatal(err)
		}
		return servers[0].AdminUsername, plain
	}
	latest := agent.provisioned[len(agent.provisioned)-1].Password
	for _, subscription := range []int{adminAccountSubscriptionID, customerSubscription} {
		if name, password := record(subscription); name != transport.DatabaseAdminAccountName || password != latest {
			t.Fatalf("record of %d holds %q and an older password (want the latest)", subscription, name)
		}
	}
	if name, password := record(ownerSubscription); name != "" || password != "the-owners-own-root-password" {
		t.Fatal("the administrator's own credential was overwritten")
	}

	// A new password from the component page (the administrator's record)
	// reaches the customer's record too.
	admin := onlyDatabaseServer(t, panel)
	response := httptest.NewRecorder()
	panel.handleProvisionDatabaseAdminAccount(response, adminAccountRequest(http.MethodPost, "/api/v1/database-servers/"+strconv.Itoa(admin.ID)+"/admin-account"))
	if response.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", response.Code, response.Body.String())
	}
	latest = agent.provisioned[len(agent.provisioned)-1].Password
	if _, password := record(customerSubscription); password != latest {
		t.Fatal("the customer's record kept the old password after a new one was given")
	}

	// Removing the account clears it from every record that held it.
	response = httptest.NewRecorder()
	panel.handleRemoveDatabaseAdminAccount(response, adminAccountRequest(http.MethodDelete, "/api/v1/database-servers/"+strconv.Itoa(admin.ID)+"/admin-account"))
	if response.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", response.Code, response.Body.String())
	}
	if name, password := record(customerSubscription); name != "" || password != "" {
		t.Fatal("the customer's record kept a credential for a removed account")
	}
	if name, password := record(ownerSubscription); name != "" || password != "the-owners-own-root-password" {
		t.Fatal("removing the panel's account touched the administrator's own credential")
	}
}
