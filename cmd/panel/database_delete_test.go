package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
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
	// N (review): an attempt whose result is not known is recorded as such.
	var unknownRows int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'database.delete.unknown:customer_matrix_test_shop'`).Scan(&unknownRows); err != nil || unknownRows != 1 {
		t.Fatalf("unknown attempt audit rows %d %v", unknownRows, err)
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
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	assertDatabaseDroppedUsersKeptOnServer(t, f, driver, got.Body.Bytes())
}

// W-A (review): on an engine reached with the record's own account the
// database goes and every user stays on the engine (said, with its reason);
// CelikPanel's records of the users only it used go.
func assertDatabaseDroppedUsersKeptOnServer(t *testing.T, f visibleDatabasesFixture, driver *remoteDeleteDriver, body []byte) {
	t.Helper()
	if strings.Join(driver.dropped, ",") != "customer_matrix_test_shop" || len(driver.usersDropped) != 0 {
		t.Fatalf("dropped %v users %v", driver.dropped, driver.usersDropped)
	}
	var answer struct {
		Removed  []string `json:"users_removed"`
		Kept     []string `json:"users_kept"`
		OnServer []string `json:"users_kept_on_server"`
	}
	_ = json.Unmarshal(body, &answer)
	if len(answer.Removed) != 0 || strings.Join(answer.OnServer, ",") != "cust_user" || strings.Join(answer.Kept, ",") != "wp_user" {
		t.Fatalf("answer %s", body)
	}
	if f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9941`) != 0 || f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = 9942`) != 1 {
		t.Fatal("records of the users are not as expected")
	}
}

// S1 (review of round 5): the Agent's door reaches the machine's own engine
// only. A record an administrator registered on this machine with their own
// account and another port (a container, a tunnel) is reached with that
// account; nothing goes to the Agent.
func TestDatabaseDeleteOfAnAdministratorsOwnLocalRecordNeverUsesTheAgent(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	drops := recordDatabaseDrops(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	sealed, err := f.box.Encrypt("the-administrators-own-password")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE database_servers SET host = '127.0.0.1', port = 3307, admin_username = 'dbadmin', root_password_encrypted = ? WHERE id = 9921`, sealed)
	driver := &remoteDeleteDriver{}
	var configs []services.DriverConfig
	saved := newDatabaseDriver
	newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) {
		configs = append(configs, config)
		return driver, nil
	}
	t.Cleanup(func() { newDatabaseDriver = saved })
	// The question says it in advance: the user only this database uses stays
	// on the server; the shared one stays because another database uses it.
	fates := f.panel.databaseUserFates(context.Background(), []int{9961, 9962})
	if fates[9961][9941] != databaseUserServer || fates[9961][9942] != databaseUserShared || fates[9962][9942] != databaseUserShared {
		t.Fatalf("fates %v", fates)
	}
	got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if len(drops.requests) != 0 || len(native.removed) != 0 {
		t.Fatalf("the Agent was asked: drops %+v users %+v", drops.requests, native.removed)
	}
	if len(configs) == 0 || configs[0].Port != 3307 || configs[0].Username != "dbadmin" || configs[0].Password != "the-administrators-own-password" {
		t.Fatalf("driver configs %+v", configs)
	}
	assertDatabaseDroppedUsersKeptOnServer(t, f, driver, got.Body.Bytes())
}

func TestDatabaseRecordIsOurLocalEngine(t *testing.T) {
	for _, c := range []struct {
		host, engine, admin, sealed string
		port                        int
		want                        bool
	}{
		{"localhost", "mariadb", "", "", 3306, true},
		{"[::1]", "mariadb", "", "", 3306, true},
		{"::1", "mariadb", transport.DatabaseAdminAccountName, "x", 0, true},
		{"127.0.0.1", "mariadb", "dbadmin", "x", 3307, false},
		{"127.0.0.1", "mariadb", transport.DatabaseAdminAccountName, "x", 3307, false},
		{"localhost", "mariadb", "", "x", 3306, false}, // a password without an account: before R-057, the administrator's
		{"db.example.net", "mariadb", "", "", 3306, false},
		{"localhost", "postgresql", transport.DatabaseAdminAccountName, "x", 5433, true},
		{"localhost", "postgresql", "postgres", "x", 5432, false},
	} {
		server := &core.DatabaseServer{Host: c.host, TypeName: c.engine, AdminUsername: c.admin, AdminPasswordEncrypted: c.sealed, Port: c.port}
		if got := databaseRecordIsOurLocalEngine(server); got != c.want {
			t.Errorf("%+v: %v, want %v", c, got, c.want)
		}
	}
	if !localDatabaseHost("[::1]") {
		t.Error("localDatabaseHost does not know [::1]")
	}
}

// S2 (review of round 5): the sentences of these two codes name the engine
// ({engine}); every answer with them carries it, from either writer, and
// nothing else writes them.
func TestDatabaseEngineRefusalsAlwaysNameTheEngine(t *testing.T) {
	refused := services.WrapDatabaseEngineFailure(errors.New("exit status 1"), []byte("ERROR 1045 (28000): Access denied for user 'x'@'localhost' (using password: YES)"))
	down := services.WrapDatabaseEngineFailure(errors.New("exit status 1"), []byte("ERROR 2002 (HY000): Can't connect to local server through socket"))
	for _, c := range []struct {
		engine, want string
		err          error
	}{{"mariadb", "MariaDB", refused}, {"postgresql", "PostgreSQL", down}} {
		rec := httptest.NewRecorder()
		writeDatabaseEngineErrorFor(rec, httptest.NewRequest(http.MethodGet, "/", nil), &core.DatabaseServer{TypeName: c.engine}, c.err)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		vars, _ := body["vars"].(map[string]any)
		if rec.Code != http.StatusConflict || vars["engine"] != c.want || body["reason"] != nil || body["action"] != nil {
			t.Fatalf("%s: %d %s", c.engine, rec.Code, rec.Body.String())
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "httperr.go" || name == "database_delete.go" {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "errCodeDatabaseEngineCredentialRefused") || strings.Contains(string(source), "errCodeDatabaseEngineUnreachable") {
			t.Errorf("%s writes a database engine refusal itself; use writeDatabaseEngineErrorFor", name)
		}
	}
}

// W-A on the domain route: the same deletion, the same answer.
func TestDatabaseDeleteOnTheDomainPageOfAnAdministratorsRecordKeepsTheUsers(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	recordDatabaseDrops(t)
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}
	sealed, err := f.box.Encrypt("the-administrators-own-password")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE database_servers SET host = '127.0.0.1', port = 3307, admin_username = 'dbadmin', root_password_encrypted = ? WHERE id = 9921`, sealed)
	driver := &remoteDeleteDriver{}
	saved := newDatabaseDriver
	newDatabaseDriver = func(services.DriverConfig) (services.DatabaseDriver, error) { return driver, nil }
	t.Cleanup(func() { newDatabaseDriver = saved })
	got := f.call(t, authzMatrixCustomerID, http.MethodDelete, "/api/v1/domains/9902/databases/9961", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	assertDatabaseDroppedUsersKeptOnServer(t, f, driver, got.Body.Bytes())
}
