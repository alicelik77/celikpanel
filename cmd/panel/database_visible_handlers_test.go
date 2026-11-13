package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133: the left-menu Databases page lists every database the caller may
// see, and its two row actions (delete a database, a new password for one of
// its users) are authorized by the visibility of the database's owner.

type visibleDatabasesFixture struct {
	accessUsersFixture
	handler http.Handler
	tokens  map[string]string
}

// Owners (accounts fixture): matrix-admin 9701; matrix-reseller 9702 with
// its customers matrix-customer 9703 and second-customer 9706; matrix-outsider
// 9704, the administrator's customer; other-reseller 9707 with its customer
// other-customer 9708; matrix-member 9710, a team member of 9703.
func newVisibleDatabasesFixture(t *testing.T) visibleDatabasesFixture {
	t.Helper()
	f := newAccessUsersFixture(t)
	var mariadb, postgresql int
	db := f.panel.db.GetDB()
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&mariadb); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'postgresql'`).Scan(&postgresql); err != nil {
		t.Fatal(err)
	}
	sealed, err := f.box.Encrypt("the-old-password")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, status) VALUES
		(9923, 9804, ?1, 'local', '10.11', 'localhost', 3306, 'active'),
		(9924, 9805, ?2, 'local', '17', 'localhost', 5432, 'active'),
		(9925, 9801, ?1, 'local', '10.11', 'localhost', 3306, 'active'),
		(9926, 9803, ?1, 'local', '10.11', 'localhost', 3306, 'active')`, mariadb, postgresql)
	f.exec(t, `INSERT INTO databases_v2 (id, server_id, subscription_id, domain_id, name) VALUES
		(9963, 9923, 9804, 9906, 'other_shop'),
		(9964, 9924, 9805, NULL, 'admin_tools'),
		(9965, 9925, 9801, 9901, 'reseller_site'),
		(9966, 9926, 9803, 9903, 'outsider_db')`)
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES
		(9944, 9923, 9804, 'other_user', ?1), (9945, 9924, 9805, 'admin_user', ?1),
		(9946, 9925, 9801, 'reseller_user', ?1), (9947, 9926, 9803, 'outsider_user', ?1)`, sealed)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES
		(9963, 9944, 'ALL'), (9964, 9945, 'ALL'), (9965, 9946, 'ALL'), (9966, 9947, 'ALL')`)

	saved := readDatabaseEngineServices
	readDatabaseEngineServices = func(*Panel, context.Context) ([]core.Service, error) {
		return []core.Service{
			{Name: "nginx", Status: "active (running)"},
			{Name: "mariadb", Status: "active (running)"},
			{Name: "postgresql", Status: "active (exited)"},
			{Name: "postgresql@17-main", Status: "failed (failed)"},
		}, nil
	}
	t.Cleanup(func() { readDatabaseEngineServices = saved })
	// alpha.133 round 5: a database on this machine is deleted through the
	// Agent (database_delete.go), never with a record's stored credential.
	savedDriver := newDatabaseDriver
	newDatabaseDriver = func(services.DriverConfig) (services.DatabaseDriver, error) {
		return nil, errors.New("a local engine was reached with a record's stored credential")
	}
	t.Cleanup(func() { newDatabaseDriver = savedDriver })
	savedAgent := databaseDeleteAgent
	databaseDeleteAgent = func(_ *Panel, _ context.Context, _ transport.DeleteDatabaseRequest, resp *transport.DeleteDatabaseResponse) error {
		resp.Success = true
		return nil
	}
	t.Cleanup(func() { databaseDeleteAgent = savedAgent })
	f.panel.databaseAccessNative = &fakeDatabaseAccessNative{}

	impersonation, err := f.panel.sessions.CreateImpersonation(context.Background(), authzMatrixCustomerID, authzMatrixAdminID)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{
		"admin":        f.tokens[authzMatrixAdminID],
		"reseller":     f.tokens[authzMatrixResellerID],
		"customer":     f.tokens[authzMatrixCustomerID],
		"outsider":     f.tokens[authzMatrixOutsiderID],
		"member":       f.tokens[authzMatrixAdditionalUserID],
		"impersonated": impersonation,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(databasesVisiblePath, f.panel.handleVisibleDatabases)
	mux.HandleFunc("/api/v1/databases/", func(w http.ResponseWriter, r *http.Request) {
		f.panel.handleDatabaseSubroute(w, r, "/api/v1/databases/")
	})
	mux.HandleFunc("/api/v1/database-users/", func(w http.ResponseWriter, r *http.Request) {
		f.panel.handleDatabaseSubroute(w, r, "/api/v1/database-users/")
	})
	return visibleDatabasesFixture{accessUsersFixture: f, handler: f.panel.requireAuth(mux), tokens: tokens}
}

func (f visibleDatabasesFixture) as(t *testing.T, who, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	token, ok := f.tokens[who]
	if !ok {
		t.Fatalf("no session for %s", who)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f visibleDatabasesFixture) list(t *testing.T, who string) (visibleDatabasesAnswer, string) {
	t.Helper()
	got := f.as(t, who, http.MethodGet, databasesVisiblePath, "")
	if got.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", who, got.Code, got.Body.String())
	}
	var answer visibleDatabasesAnswer
	if err := json.Unmarshal(got.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s: %v", who, err)
	}
	return answer, got.Body.String()
}

func visibleNames(answer visibleDatabasesAnswer) []string {
	names := []string{}
	for _, db := range answer.Databases {
		names = append(names, db.Owner.Username+"/"+db.Name)
	}
	return names
}

func TestVisibleDatabasesFollowTheOneVisibilityRule(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	for _, c := range []struct {
		who  string
		want []string
	}{
		// Sorted by owner username, then name.
		{"admin", []string{"matrix-admin/admin_tools", "matrix-customer/customer_matrix_test_blog",
			"matrix-customer/customer_matrix_test_shop", "matrix-outsider/outsider_db",
			"matrix-reseller/reseller_site", "other-customer/other_shop"}},
		{"reseller", []string{"matrix-customer/customer_matrix_test_blog", "matrix-customer/customer_matrix_test_shop",
			"matrix-reseller/reseller_site"}},
		{"customer", []string{"matrix-customer/customer_matrix_test_blog", "matrix-customer/customer_matrix_test_shop"}},
		{"outsider", []string{"matrix-outsider/outsider_db"}},
		// "Log in as" sees what the account sees, not what the operator sees.
		{"impersonated", []string{"matrix-customer/customer_matrix_test_blog", "matrix-customer/customer_matrix_test_shop"}},
		// A team member saw nothing on this page before; still nothing.
		{"member", []string{}},
	} {
		answer, _ := f.list(t, c.who)
		if got := visibleNames(answer); strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s sees %v, want %v", c.who, got, c.want)
		}
	}
}

func TestVisibleDatabasesNameOwnerDomainAndUsers(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	answer, _ := f.list(t, "admin")
	byID := map[int]visibleDatabase{}
	for _, db := range answer.Databases {
		byID[db.ID] = db
	}
	shop := byID[9961]
	if shop.Engine != "mariadb" || shop.Owner.ID != authzMatrixCustomerID || shop.Owner.Role != "customer" ||
		shop.Owner.Parent == nil || *shop.Owner.Parent != "matrix-reseller" || shop.Owner.Self {
		t.Fatalf("shop %+v (parent %v)", shop, shop.Owner.Parent)
	}
	if shop.Domain == nil || shop.Domain.ID != 9902 || shop.Domain.Name != "customer.matrix.test" {
		t.Fatalf("shop domain %+v", shop.Domain)
	}
	if len(shop.Users) != 2 || shop.Users[0].Username != "cust_user" || shop.Users[1].Username != "wp_user" {
		t.Fatalf("shop users %+v", shop.Users)
	}
	if shop.SizeBytes != nil || shop.CreatedAt == "" {
		t.Fatalf("shop size %v created %q", shop.SizeBytes, shop.CreatedAt)
	}
	tools := byID[9964]
	if tools.Engine != "postgresql" || !tools.Owner.Self || tools.Owner.Role != "admin" || tools.Owner.Parent != nil || tools.Domain != nil {
		t.Fatalf("admin_tools %+v", tools)
	}
	// The administrator's own customer has no reseller above it.
	if outsider := byID[9966]; outsider.Owner.Parent != nil || outsider.Owner.Self {
		t.Fatalf("outsider %+v", outsider.Owner)
	}
	if other := byID[9963]; other.Owner.Parent == nil || *other.Owner.Parent != "other-reseller" {
		t.Fatalf("other %+v", other.Owner)
	}
	if reseller, _ := f.list(t, "reseller"); len(reseller.Databases) == 0 ||
		reseller.Databases[len(reseller.Databases)-1].Name != "reseller_site" || !reseller.Databases[len(reseller.Databases)-1].Owner.Self {
		t.Fatalf("reseller's own database is not marked as its own: %+v", reseller.Databases)
	}
}

// The answer carries names only: every key is a listed one and no stored
// password (sealed or plain) appears anywhere.
func TestVisibleDatabasesCarryNoSecret(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	_, body := f.list(t, "admin")
	if strings.Contains(strings.ToLower(body), "password") || strings.Contains(body, "the-old-password") {
		t.Fatalf("the answer names a password: %s", body)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	keys := func(value any) string {
		object, _ := value.(map[string]any)
		out := []string{}
		for key := range object {
			out = append(out, key)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	if got := keys(raw); got != "databases,engines,may_create" {
		t.Fatalf("top keys %s", got)
	}
	for _, item := range raw["databases"].([]any) {
		db := item.(map[string]any)
		if got := keys(db); got != "created_at,domain,engine,id,name,owner,size_bytes,users" {
			t.Fatalf("database keys %s", got)
		}
		if got := keys(db["owner"]); got != "id,parent,role,self,username" {
			t.Fatalf("owner keys %s", got)
		}
		if db["domain"] != nil {
			if got := keys(db["domain"]); got != "id,name" {
				t.Fatalf("domain keys %s", got)
			}
		}
		for _, user := range db["users"].([]any) {
			if got := keys(user); got != "id,username" {
				t.Fatalf("user keys %s", got)
			}
		}
	}
	for _, engine := range raw["engines"].([]any) {
		if got := keys(engine); got != "engine,installed,running" {
			t.Fatalf("engine keys %s", got)
		}
	}
}

// The engines are the server's, not the caller's subscription registrations:
// a reseller with no PostgreSQL record is still told PostgreSQL is
// installed. A PostgreSQL whose cluster unit failed is installed, not
// running. When the Agent cannot be asked, engines is null (not known).
func TestVisibleDatabasesEnginesComeFromTheServer(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	for _, who := range []string{"admin", "reseller", "customer"} {
		answer, _ := f.list(t, who)
		want := []visibleDatabaseEngine{{"mariadb", true, true}, {"postgresql", true, false}}
		if len(answer.Engines) != 2 || answer.Engines[0] != want[0] || answer.Engines[1] != want[1] {
			t.Fatalf("%s engines %+v", who, answer.Engines)
		}
	}
	if member, body := f.list(t, "member"); member.Engines == nil || len(member.Engines) != 0 || member.MayCreate {
		t.Fatalf("team member: %s", body)
	}
	readDatabaseEngineServices = func(*Panel, context.Context) ([]core.Service, error) {
		return nil, errors.New("agent unavailable")
	}
	if _, body := f.list(t, "customer"); !strings.Contains(body, `"engines":null`) ||
		!strings.Contains(body, "customer_matrix_test_shop") {
		t.Fatalf("an unreachable Agent: %s", body)
	}
}

func TestVisibleDatabasesMayCreateFollowsThePlan(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	for _, who := range []string{"admin", "reseller", "customer"} {
		if answer, _ := f.list(t, who); !answer.MayCreate {
			t.Fatalf("%s may not create", who)
		}
	}
	// The customer's plan is full: no other domain of theirs takes one.
	f.exec(t, `UPDATE subscriptions SET max_databases = 2 WHERE id = 9802`)
	if answer, _ := f.list(t, "customer"); answer.MayCreate {
		t.Fatal("a full plan may create")
	}
	// The reseller still has its own domain with room.
	if answer, _ := f.list(t, "reseller"); !answer.MayCreate {
		t.Fatal("reseller lost its own room")
	}
	f.exec(t, `UPDATE subscriptions SET max_databases = 10 WHERE id = 9802`)
	f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(rightDatabases)...))
	if answer, _ := f.list(t, "customer"); answer.MayCreate {
		t.Fatal("a plan without databases may create")
	}
}

// The two row actions: authorized by the visibility of the database's owner.
func TestVisibleDatabaseRowActionsRoleMatrix(t *testing.T) {
	const strong = `{"password":"N3w-Passw0rd!xyz"}`
	rows := []struct {
		name     string
		who      string
		database int // deleted
		user     int // given a new password
		want     int
	}{
		{"administrator on a reseller's customer", "admin", 9963, 9944, http.StatusOK},
		{"administrator on its own", "admin", 9964, 9945, http.StatusOK},
		{"reseller on its own customer", "reseller", 9961, 9941, http.StatusOK},
		{"reseller on its own", "reseller", 9965, 9946, http.StatusOK},
		{"reseller on a stranger", "reseller", 9963, 9944, http.StatusNotFound},
		{"reseller on the administrator's customer", "reseller", 9966, 9947, http.StatusNotFound},
		{"customer on its own", "customer", 9961, 9941, http.StatusOK},
		{"customer on its reseller", "customer", 9965, 9946, http.StatusNotFound},
		{"customer on a stranger", "customer", 9963, 9944, http.StatusNotFound},
		{"team member", "member", 9961, 9941, http.StatusNotFound},
		{"impersonated session on the account's own", "impersonated", 9961, 9941, http.StatusOK},
		{"impersonated session on what only the operator sees", "impersonated", 9963, 9944, http.StatusNotFound},
		{"missing", "admin", 99999, 99999, http.StatusNotFound},
	}
	var notFoundDelete, notFoundPassword string
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			f := newVisibleDatabasesFixture(t)
			f.native.passwords = nil
			// The password first: deleting the database removes the users
			// only it uses (alpha.133 round 5).
			password := f.as(t, row.who, http.MethodPost, "/api/v1/database-users/"+strconv.Itoa(row.user)+"/password", strong)
			if password.Code != row.want {
				t.Fatalf("password: %d %s", password.Code, password.Body.String())
			}
			deleted := f.as(t, row.who, http.MethodDelete, "/api/v1/databases/"+strconv.Itoa(row.database), "")
			if deleted.Code != row.want {
				t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
			}
			remaining := f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = ?`, row.database)
			if row.want == http.StatusOK {
				if remaining != 0 || len(f.native.passwords) != 1 {
					t.Fatalf("not done: %d left, passwords %v", remaining, f.native.passwords)
				}
				return
			}
			if row.database != 99999 && remaining != 1 {
				t.Fatal("a refused delete removed the record")
			}
			if len(f.native.passwords) != 0 {
				t.Fatalf("a refused request changed a password: %v", f.native.passwords)
			}
			// A missing and a foreign record answer the same.
			if notFoundDelete == "" {
				notFoundDelete, notFoundPassword = deleted.Body.String(), password.Body.String()
			} else if deleted.Body.String() != notFoundDelete || password.Body.String() != notFoundPassword {
				t.Fatalf("not-found answers differ: %q/%q vs %q/%q", deleted.Body.String(), password.Body.String(), notFoundDelete, notFoundPassword)
			}
		})
	}
}

// The audit rows name who acted (and, inside "Log in as", whom they acted
// as) and whose database it was.
func TestVisibleDatabaseRowActionsAuditActorAndOwner(t *testing.T) {
	f := newVisibleDatabasesFixture(t)
	if got := f.as(t, "reseller", http.MethodDelete, "/api/v1/databases/9961", ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	if got := f.as(t, "impersonated", http.MethodPost, "/api/v1/database-users/9942/password", `{"password":"N3w-Passw0rd!xyz"}`); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	read := func(action string) (int, int, map[string]any) {
		var userID, actingAs int
		var details string
		if err := f.panel.db.GetDB().QueryRow(`SELECT user_id, COALESCE(acting_as, 0), COALESCE(details, '') FROM audit_logs WHERE action = ?`, action).
			Scan(&userID, &actingAs, &details); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		var parsed map[string]any
		_ = json.Unmarshal([]byte(details), &parsed)
		if strings.Contains(details, "N3w-Passw0rd") {
			t.Fatal("the audit row holds the password")
		}
		return userID, actingAs, parsed
	}
	if user, as, details := read("database.delete:customer_matrix_test_shop"); user != authzMatrixResellerID || as != 0 ||
		details["owner"] != "matrix-customer" || details["owner_id"] != float64(authzMatrixCustomerID) {
		t.Fatalf("delete row: %d %d %v", user, as, details)
	}
	if user, as, details := read("database.user.password:wp_user"); user != authzMatrixAdminID || as != authzMatrixCustomerID ||
		details["owner"] != "matrix-customer" {
		t.Fatalf("password row: %d %d %v", user, as, details)
	}
}
