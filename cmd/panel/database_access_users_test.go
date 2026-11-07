package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129: the users of one domain database.

type accessUsersFixture struct {
	rotationFixture
	handler http.Handler
}

func newAccessUsersFixture(t *testing.T) accessUsersFixture {
	t.Helper()
	f := newRotationFixture(t)
	// Database 9961 of domain 9902 on the local MariaDB record 9921, made
	// with cust_user; wp_user reads it. Database 9962 of the same domain
	// has far-away grants only for wp_user.
	f.exec(t, `INSERT INTO databases_v2 (id, server_id, subscription_id, domain_id, name) VALUES
		(9961, 9921, 9802, 9902, 'customer_matrix_test_shop'), (9962, 9921, 9802, 9902, 'customer_matrix_test_blog')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES
		(9961, 9941, 'ALL'), (9961, 9942, 'SELECT'), (9962, 9942, 'ALL')`)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/domains/", f.panel.handleDomainSubroute)
	return accessUsersFixture{rotationFixture: f, handler: f.panel.requireAuth(mux)}
}

func (f accessUsersFixture) call(t *testing.T, userID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[userID]})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func TestDomainDatabaseUserPasswordNeedsAGrantOnThatDatabase(t *testing.T) {
	f := newAccessUsersFixture(t)
	const strong = `{"password":"N3w-Passw0rd!xyz"}`
	// cust_user holds no grant on the blog database: not found, nothing set.
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/databases/9962/users/9941/password", strong); got.Code != http.StatusNotFound {
		t.Fatalf("user of another database: %d %s", got.Code, got.Body.String())
	}
	// Another customer does not reach the domain at all.
	if got := f.call(t, 9708, http.MethodPost, "/api/v1/domains/9902/databases/9961/users/9941/password", strong); got.Code == http.StatusOK {
		t.Fatalf("other customer reached it: %d", got.Code)
	}
	if len(f.native.passwords) != 0 {
		t.Fatalf("a refused request changed a password: %v", f.native.passwords)
	}
	// A weak password is refused by the rule.
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/databases/9961/users/9941/password", `{"password":"short"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("weak password: %d", got.Code)
	}
	got := f.call(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/databases/9961/users/9941/password", strong)
	if got.Code != http.StatusOK {
		t.Fatalf("owner: %d %s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "N3w-Passw0rd") {
		t.Fatal("a sent password was echoed")
	}
	var stored string
	_ = f.panel.db.GetDB().QueryRow(`SELECT password FROM database_users WHERE id = 9941`).Scan(&stored)
	if plain, _ := f.box.Decrypt(stored); plain != "N3w-Passw0rd!xyz" {
		t.Fatal("the new password is not the one stored")
	}
	if len(f.native.passwords) != 1 || f.native.passwords[0] != "N3w-Passw0rd!xyz" {
		t.Fatalf("engine calls %v", f.native.passwords)
	}
}

func TestDomainDatabaseUserPasswordNamesTheApplicationItUpdated(t *testing.T) {
	f := newAccessUsersFixture(t)
	got := f.call(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/databases/9961/users/9942/password", `{"password":"N3w-Passw0rd!xyz"}`)
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	var answer map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &answer)
	if answer["app_domain"] != "customer.matrix.test" || answer["app_kind"] != "wordpress" || answer["app_updated"] != true {
		t.Fatalf("answer %v", answer)
	}
}

func TestDomainDatabasesListEveryUserWithItsAccessAndTheEngine(t *testing.T) {
	f := newAccessUsersFixture(t)
	got := f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/databases", "")
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	var answer struct {
		Databases []DatabaseInfo `json:"databases"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	byName := map[string]DatabaseInfo{}
	for _, db := range answer.Databases {
		byName[db.Name] = db
	}
	shop := byName["customer_matrix_test_shop"]
	if len(shop.Users) != 2 || shop.Users[0].Username != "cust_user" || shop.Users[0].Access != "owner" ||
		shop.Users[1].Username != "wp_user" || shop.Users[1].Access != "read" {
		t.Fatalf("shop users %+v", shop.Users)
	}
	if shop.Engine != "mariadb" || shop.EngineVersion != "10.11" || shop.User != "cust_user" {
		t.Fatalf("shop %+v", shop)
	}
}

func TestDatabaseEngineLabel(t *testing.T) {
	for _, c := range []struct{ serverType, version, engine, major string }{
		{"mariadb", "11.8.6-MariaDB-0+deb13u1 from Debian", "mariadb", "11.8"},
		{"mariadb", "10.11", "mariadb", "10.11"},
		{"mariadb", "", "mariadb", ""},
		{"mariadb", "8.0.36", "mysql", "8.0"},
		{"postgresql", "17.11 (Debian 17.11-0+deb13u1)", "postgresql", "17"},
		{"postgresql", "16", "postgresql", "16"},
		{"postgresql", "unknown", "postgresql", ""},
	} {
		engine, major := databaseEngineLabel(c.serverType, c.version)
		if engine != c.engine || major != c.major {
			t.Errorf("%s %q = %s %q, want %s %q", c.serverType, c.version, engine, major, c.engine, c.major)
		}
	}
}

type fakeDatabaseAccessNative struct {
	created, removed []transport.DatabaseAccessUserRequest
	exists           bool
	failRemove       bool
}

func (f *fakeDatabaseAccessNative) create(_ context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if f.exists {
		return transport.DatabaseAccessUserResponse{Exists: true, Error: "exists"}, nil
	}
	f.created = append(f.created, req)
	return transport.DatabaseAccessUserResponse{Applied: true}, nil
}

func (f *fakeDatabaseAccessNative) remove(_ context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	if f.failRemove {
		return transport.DatabaseAccessUserResponse{Error: "engine said no"}, nil
	}
	f.removed = append(f.removed, req)
	return transport.DatabaseAccessUserResponse{Applied: true}, nil
}

func TestDomainDatabaseAccessUsersAddAndRemove(t *testing.T) {
	f := newAccessUsersFixture(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	const path = "/api/v1/domains/9902/databases/9961/users"

	for _, bad := range []string{
		`{"name":"user","password":"Str0ng-Passw0rd!","access":"read"}`,
		`{"name":"Bad-Name","password":"Str0ng-Passw0rd!","access":"read"}`,
		`{"name":"reader","password":"Str0ng-Passw0rd!","access":"admin"}`,
		`{"name":"reader","password":"weak","access":"read"}`,
	} {
		if got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, bad); got.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", bad, got.Code, got.Body.String())
		}
	}
	if len(native.created) != 0 {
		t.Fatal("a refused request reached the engine")
	}

	got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"reader","password":"Str0ng-Passw0rd!","access":"read"}`)
	if got.Code != http.StatusOK {
		t.Fatalf("add: %d %s", got.Code, got.Body.String())
	}
	if strings.Contains(got.Body.String(), "Str0ng-Passw0rd") {
		t.Fatal("the password was echoed")
	}
	if len(native.created) != 1 || native.created[0].Username != "customer_matrix_test_shop_reader" || native.created[0].Engine != "mysql" ||
		native.created[0].Access != "read" || native.created[0].Database != "customer_matrix_test_shop" {
		t.Fatalf("engine request %+v", native.created)
	}
	// The database's other added users go with it (wp_user reads it); the
	// user made with the database is not one of them.
	if got := native.created[0]; len(got.Readers) != 1 || got.Readers[0] != "wp_user" || len(got.Writers) != 0 {
		t.Fatalf("other users %+v / %+v", got.Readers, got.Writers)
	}
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"editor","password":"Str0ng-Passw0rd!","access":"write"}`); got.Code != http.StatusOK {
		t.Fatalf("writer: %d %s", got.Code, got.Body.String())
	}
	if got := native.created[1]; got.Access != "write" || strings.Join(got.Readers, ",") != "wp_user,customer_matrix_test_shop_reader" || len(got.Writers) != 0 {
		t.Fatalf("writer's other users %+v / %+v", got.Readers, got.Writers)
	}
	native.created = native.created[:1]
	var userID int
	var privileges, sealed string
	if err := f.panel.db.GetDB().QueryRow(`SELECT u.id, g.privileges, u.password FROM database_users u JOIN database_user_grants g ON g.user_id = u.id
		WHERE u.username = 'customer_matrix_test_shop_reader' AND g.database_id = 9961 AND u.server_id = 9921 AND u.subscription_id = 9802`).Scan(&userID, &privileges, &sealed); err != nil {
		t.Fatal(err)
	}
	if plain, _ := f.box.Decrypt(sealed); privileges != "SELECT" || plain != "Str0ng-Passw0rd!" {
		t.Fatalf("recorded %q", privileges)
	}
	// The same name again is refused before the engine.
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"reader","password":"Str0ng-Passw0rd!","access":"write"}`); got.Code != http.StatusConflict ||
		!strings.Contains(got.Body.String(), errCodeDatabaseUserExists) || len(native.created) != 1 {
		t.Fatalf("same name: %d %s", got.Code, got.Body.String())
	}
	// A name the engine has already (made outside CelikPanel) changes nothing.
	native.exists = true
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"outside","password":"Str0ng-Passw0rd!","access":"write"}`); got.Code != http.StatusConflict {
		t.Fatalf("engine name taken: %d %s", got.Code, got.Body.String())
	}
	native.exists = false
	var count int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_users WHERE username = 'customer_matrix_test_shop_outside'`).Scan(&count)
	if count != 0 {
		t.Fatal("a user the engine refused was recorded")
	}

	// The list says read only.
	list := f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/databases", "")
	if !strings.Contains(list.Body.String(), `"username":"customer_matrix_test_shop_reader","access":"read"`) {
		t.Fatalf("list %s", list.Body.String())
	}

	// The user made with the database is not removed from here, nor one with
	// access to another database.
	if got := f.call(t, authzMatrixCustomerID, http.MethodDelete, path+"/9941", ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), errCodeDatabaseUserIsOwner) {
		t.Fatalf("owner removal: %d %s", got.Code, got.Body.String())
	}
	if got := f.call(t, authzMatrixCustomerID, http.MethodDelete, path+"/9942", ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), errCodeDatabaseUserElsewhere) {
		t.Fatalf("shared user removal: %d %s", got.Code, got.Body.String())
	}
	// An engine refusal keeps the record.
	native.failRemove = true
	if got := f.call(t, authzMatrixCustomerID, http.MethodDelete, path+"/"+strconv.Itoa(userID), ""); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), errCodeDatabaseUserEngine) {
		t.Fatalf("engine refusal: %d %s", got.Code, got.Body.String())
	}
	native.failRemove = false
	if got := f.call(t, authzMatrixCustomerID, http.MethodDelete, path+"/"+strconv.Itoa(userID), ""); got.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", got.Code, got.Body.String())
	}
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_users WHERE id = ?`, userID).Scan(&count)
	if count != 0 || len(native.removed) != 1 || native.removed[0].Username != "customer_matrix_test_shop_reader" {
		t.Fatalf("after removal: %d users, engine %+v", count, native.removed)
	}
	// The database and its first user stay.
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM databases_v2 d JOIN database_user_grants g ON g.database_id = d.id WHERE d.id = 9961`).Scan(&count)
	if count != 3 {
		t.Fatalf("the database's other grants changed: %d", count)
	}
	// Another customer reaches none of it.
	if got := f.call(t, 9708, http.MethodPost, path, `{"name":"x","password":"Str0ng-Passw0rd!","access":"read"}`); got.Code == http.StatusOK {
		t.Fatalf("other customer: %d", got.Code)
	}
}
