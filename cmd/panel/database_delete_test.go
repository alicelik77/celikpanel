package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5: deleting a database removes the database and every user
// recorded for it that no other database uses, through one function for the
// left-menu Databases page and the domain's Databases section. An engine on
// this machine is reached through the Agent, never with the credential a
// subscription's server record holds (that copy can be older than the
// account's password: the lab refusal DATABASE_ENGINE_CREDENTIAL_REFUSED).
// The tests go through the production routes and authorization with the
// Agent faked; the WSL test guest has no MariaDB or PostgreSQL to run.

type recordedDatabaseDrops struct {
	requests []transport.DeleteDatabaseRequest
	answer   func(*transport.DeleteDatabaseResponse) error
}

func recordDatabaseDrops(t *testing.T) *recordedDatabaseDrops {
	t.Helper()
	drops := &recordedDatabaseDrops{answer: func(resp *transport.DeleteDatabaseResponse) error { resp.Success = true; return nil }}
	saved := databaseDeleteAgent
	databaseDeleteAgent = func(_ *Panel, _ context.Context, req transport.DeleteDatabaseRequest, resp *transport.DeleteDatabaseResponse) error {
		drops.requests = append(drops.requests, req)
		return drops.answer(resp)
	}
	t.Cleanup(func() { databaseDeleteAgent = saved })
	return drops
}

func removedUsernames(native *fakeDatabaseAccessNative) string {
	names := []string{}
	for _, req := range native.removed {
		names = append(names, req.Username)
	}
	return strings.Join(names, ",")
}

func TestDatabaseDeleteRemovesItsOwnUsersAndKeepsASharedOne(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	// A read-only user only this database uses goes with it as well.
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9948, 9921, 9802, 'shop_rapor', 'x')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9961, 9948, 'SELECT')`)

	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if len(drops.requests) != 1 || drops.requests[0].Name != "customer_matrix_test_shop" || drops.requests[0].Type != "mysql" || drops.requests[0].User != "" {
		t.Fatalf("drop requests %+v", drops.requests)
	}
	// cust_user and shop_rapor only reach this database; wp_user also reads
	// the blog database and stays.
	if removed := removedUsernames(native); removed != "cust_user,shop_rapor" {
		t.Fatalf("removed %q", removed)
	}
	for _, req := range native.removed {
		if req.Database != "customer_matrix_test_shop" || req.Engine != "mysql" || req.ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
			t.Fatalf("user removal %+v", req)
		}
	}
	var answer struct {
		Removed []string `json:"users_removed"`
		Kept    []string `json:"users_kept"`
	}
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if strings.Join(answer.Removed, ",") != "cust_user,shop_rapor" || strings.Join(answer.Kept, ",") != "wp_user" {
		t.Fatalf("answer %s", got.Body.String())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`); n != 0 {
		t.Fatal("the database record stayed")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM database_users WHERE id IN (9941, 9948)`); n != 0 {
		t.Fatal("a removed user's record stayed")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = 9942 AND database_id = 9962`); n != 1 {
		t.Fatal("the shared user lost its other database")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM database_user_grants WHERE database_id = 9961`); n != 0 {
		t.Fatal("grants of the deleted database stayed")
	}
}

func TestDatabaseDeleteOfTheAdministratorsPostgreSQLDatabaseNamesItsCluster(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9964", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if len(drops.requests) != 1 || drops.requests[0].Type != "postgresql" || drops.requests[0].Port != 5432 || drops.requests[0].Name != "admin_tools" {
		t.Fatalf("drop %+v", drops.requests)
	}
	if len(native.removed) != 1 || native.removed[0].Username != "admin_user" || native.removed[0].Engine != "postgresql" || native.removed[0].Port != 5432 {
		t.Fatalf("user removal %+v", native.removed)
	}
}

func TestDatabaseDeleteInterruptedAfterTheDropIsFinishedByDeletingAgain(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{failRemove: true}
	f.panel.databaseAccessNative = native

	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusConflict {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &body)
	if body["code"] != errCodeDatabaseDeleteUsersLeft || body["partial_success"] != true || body["mutation_applied"] != true {
		t.Fatalf("answer %s", got.Body.String())
	}
	if vars, _ := body["vars"].(map[string]any); vars["users"] != "cust_user" || vars["name"] != "customer_matrix_test_shop" {
		t.Fatalf("vars %v", body["vars"])
	}
	// The records stay until the users are gone: the list still shows it.
	if n := f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`); n != 1 {
		t.Fatal("the record went while a user is left")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9941`); n != 1 {
		t.Fatal("the user's record went while it is on the engine")
	}

	// Pressing Delete again runs the same steps over what is left.
	native.failRemove = false
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("again: %d %s", got.Code, got.Body.String())
	}
	if len(drops.requests) != 2 || removedUsernames(native) != "cust_user" {
		t.Fatalf("second run: drops %d, removed %q", len(drops.requests), removedUsernames(native))
	}
	if n := f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) + f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9941`); n != 0 {
		t.Fatal("records stayed after the finished deletion")
	}
}

func TestDatabaseDeleteWithoutAnAnswerIsUnknownAndChangesNoRecord(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	drops.answer = func(*transport.DeleteDatabaseResponse) error { return errors.New("read: connection reset mid-call") }
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusBadGateway || !strings.Contains(got.Body.String(), errCodeDatabaseDeleteUnconfirmed) {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if len(native.removed) != 0 || f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 {
		t.Fatal("an unknown drop went on to users or records")
	}
	// A verified refusal of the drop: nothing changed, said as such.
	drops.answer = func(resp *transport.DeleteDatabaseResponse) error { resp.Error = "failed to drop database"; return nil }
	got = f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), errCodeDatabaseDeleteFailed) {
		t.Fatalf("refused: %d %s", got.Code, got.Body.String())
	}
	if len(native.removed) != 0 || f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 {
		t.Fatal("a refused drop went on to users or records")
	}
}

func TestDatabaseDeleteOnTheDomainPageIsTheSameDeletion(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	got := f.call(t, authzMatrixCustomerID, http.MethodDelete, "/api/v1/domains/9902/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	// The domain page used to drop the first user on the engine whatever
	// else used it: wp_user (also on the blog database) now stays.
	if len(drops.requests) != 1 || drops.requests[0].User != "" || removedUsernames(native) != "cust_user" {
		t.Fatalf("drops %+v removed %q", drops.requests, removedUsernames(native))
	}
	if f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9942`) != 1 {
		t.Fatal("the shared user's record went")
	}
}

func TestDatabaseDeleteNeverRemovesTheEnginesOrCelikPanelsOwnAccount(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9949, 9921, 9802, ?, 'x'), (9950, 9921, 9802, 'root', 'x')`, transport.DatabaseAdminAccountName)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9961, 9949, 'ALL'), (9961, 9950, 'ALL')`)
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if removed := removedUsernames(native); removed != "cust_user" {
		t.Fatalf("removed %q", removed)
	}
}

// An engine on another machine has no Agent door: its record's credential is
// used, and a refusal of it is said in the product's shape, with the page
// only for an administrator.
type remoteDeleteDriver struct {
	authzGrantIsolationDriver
	refuse       bool
	dropped      []string
	usersDropped []string
}

func (d *remoteDeleteDriver) DeleteDatabase(name string) error {
	if d.refuse {
		return services.WrapDatabaseEngineFailure(errors.New("exit status 1"), []byte("ERROR 1045 (28000): Access denied for user 'celikpanel_admin'@'x' (using password: YES)"))
	}
	d.dropped = append(d.dropped, name)
	return nil
}

func (d *remoteDeleteDriver) DeleteUser(name string) error {
	d.usersDropped = append(d.usersDropped, name)
	return nil
}

func TestDatabaseDeleteOnAnotherMachineUsesTheRecordAndSaysARefusalPlainly(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	f.exec(t, `UPDATE database_servers SET host = 'far-db.example.net' WHERE id = 9921`)
	driver := &remoteDeleteDriver{refuse: true}
	saved := newDatabaseDriver
	newDatabaseDriver = func(services.DriverConfig) (services.DatabaseDriver, error) { return driver, nil }
	t.Cleanup(func() { newDatabaseDriver = saved })

	for _, who := range []string{"admin", "customer"} {
		got := f.as(t, who, http.MethodDelete, "/api/v1/databases/9961", "")
		var body map[string]any
		_ = json.Unmarshal(got.Body.Bytes(), &body)
		if got.Code != http.StatusConflict || body["code"] != errCodeDatabaseEngineCredentialRefused {
			t.Fatalf("%s: %d %s", who, got.Code, got.Body.String())
		}
		if vars, _ := body["vars"].(map[string]any); vars["engine"] != "MariaDB" {
			t.Fatalf("%s: vars %v", who, body["vars"])
		}
		admin := who == "admin"
		if (body["reason"] == "administrator") != admin || (body["action"] == "/components/mariadb") != admin {
			t.Fatalf("%s: reason %v action %v", who, body["reason"], body["action"])
		}
		if strings.Contains(got.Body.String(), "Access denied") {
			t.Fatal("the engine's own words reached the answer")
		}
	}
	if len(drops.requests) != 0 || f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 {
		t.Fatal("a refused remote drop changed something or went to the Agent")
	}

	driver.refuse = false
	if got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if strings.Join(driver.dropped, ",") != "customer_matrix_test_shop" || strings.Join(driver.usersDropped, ",") != "cust_user" {
		t.Fatalf("remote drops %v users %v", driver.dropped, driver.usersDropped)
	}
}
