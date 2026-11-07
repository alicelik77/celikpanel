package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
