package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/core"
)

// Accounts, plans and resellers (alpha.114): quota meaning, plan assignment,
// reseller pools, suspension, "Log in as" accountability, password change
// and "My plan". Every request goes through the real middleware and the real
// handlers on the authz matrix fixture: admin 9701, reseller 9702, its
// customer 9703 (subscription 9802, domain 9902), an admin-owned customer
// 9704 (subscription 9803).

type accountsFixture struct {
	authzMatrixFixture
	handler     http.Handler
	memberToken string
}

func newAccountsFixture(t *testing.T) accountsFixture {
	t.Helper()
	base := newAuthzMatrixFixture(t)
	memberToken := seedAdditionalUserSession(t, &base)
	db := base.panel.db.GetDB()
	if _, err := db.Exec(`UPDATE users SET parent_id = 9701 WHERE id = 9704`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", base.panel.handleUsers)
	mux.HandleFunc("/api/v1/users/", base.panel.handleUserByID)
	mux.HandleFunc("/api/v1/plans", base.panel.handlePlans)
	mux.HandleFunc("/api/v1/plans/", base.panel.handlePlanByID)
	mux.HandleFunc("/api/v1/subscriptions", base.panel.handleSubscriptions)
	mux.HandleFunc(mySubscriptionPath, base.panel.handleMySubscription)
	mux.HandleFunc("/api/v1/auth/me", base.panel.handleMe)
	mux.HandleFunc("/api/v1/auth/password", base.panel.handleChangeOwnPassword)
	mux.HandleFunc("/api/v1/auth/unimpersonate", base.panel.handleUnimpersonate)
	// An audited action, to see who the trail names inside "Log in as".
	mux.HandleFunc("/api/v1/test/audited", func(w http.ResponseWriter, r *http.Request) {
		base.panel.audit(r, "user.create:probe/customer", "user", 0)
		w.WriteHeader(http.StatusNoContent)
	})
	return accountsFixture{authzMatrixFixture: base, handler: base.panel.requireAuth(mux), memberToken: memberToken}
}

func (f accountsFixture) send(t *testing.T, method, path, token, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func (f accountsFixture) as(t *testing.T, userID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.send(t, method, path, f.tokens[userID], body)
}

func wantStatus(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	if code != "" && !strings.Contains(recorder.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("body = %s, want code %s", recorder.Body.String(), code)
	}
}

func (f accountsFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.panel.db.GetDB().Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func (f accountsFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.panel.db.GetDB().QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// seedUsage gives subscription 9802 one domain (9902), one database, one
// mailbox and 1 MB of measured disk.
func (f accountsFixture) seedUsage(t *testing.T) {
	t.Helper()
	var typeID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (9921, 9802, ?, 'quota-server', '127.0.0.1', 3306)`, typeID)
	f.exec(t, `INSERT INTO databases_v2 (server_id, subscription_id, name) VALUES (9921, 9802, 'quota_db')`)
	f.exec(t, `INSERT INTO email_accounts (domain_id, address, password_hash) VALUES (9902, 'box@customer.matrix.test', 'x')`)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type, disk_usage_bytes) VALUES (9902, '/srv/quota', 'php', 1048576)`)
}

func TestQuotaZeroMeansUnlimitedForEveryKind(t *testing.T) {
	f := newAccountsFixture(t)
	f.seedUsage(t)
	ctx := context.Background()
	kinds := []struct {
		kind quotaKind
		code string
	}{
		{quotaDomains, errCodeQuotaDomains},
		{quotaDBs, errCodeQuotaDatabases},
		{quotaMail, errCodeQuotaMailboxes},
		{quotaDisk, errCodeQuotaDisk},
	}

	f.exec(t, `UPDATE subscriptions SET max_domains = 0, max_databases = 0, max_email_accounts = 0, disk_quota_mb = 0 WHERE id = 9802`)
	for _, k := range kinds {
		if err := f.panel.checkSubscriptionQuota(ctx, 9802, k.kind); err != nil {
			t.Fatalf("%s with limit 0 (unlimited): %v", k.kind, err)
		}
	}

	// At the limit: one in use, limit one (disk: 1 MB used of 1 MB).
	f.exec(t, `UPDATE subscriptions SET max_domains = 1, max_databases = 1, max_email_accounts = 1, disk_quota_mb = 1 WHERE id = 9802`)
	for _, k := range kinds {
		err := f.panel.checkSubscriptionQuota(ctx, 9802, k.kind)
		var exceeded *quotaExceededError
		if !errors.As(err, &exceeded) {
			t.Fatalf("%s at its limit: %v, want a quota refusal", k.kind, err)
		}
		if exceeded.code() != k.code {
			t.Fatalf("%s refusal code = %s, want %s", k.kind, exceeded.code(), k.code)
		}
		recorder := httptest.NewRecorder()
		writeQuotaRefusal(recorder, err)
		wantStatus(t, recorder, http.StatusConflict, k.code)
		if !strings.Contains(recorder.Body.String(), `"action":"/account"`) {
			t.Fatalf("%s refusal does not point at My plan: %s", k.kind, recorder.Body.String())
		}
	}

	f.exec(t, `UPDATE subscriptions SET max_domains = 2, max_databases = 2, max_email_accounts = 2, disk_quota_mb = 2 WHERE id = 9802`)
	for _, k := range kinds {
		if err := f.panel.checkSubscriptionQuota(ctx, 9802, k.kind); err != nil {
			t.Fatalf("%s under its limit: %v", k.kind, err)
		}
	}

	for _, k := range kinds {
		err := f.panel.checkSubscriptionQuota(ctx, 424242, k.kind)
		if !errors.Is(err, errQuotaSubscriptionMissing) {
			t.Fatalf("%s on a missing subscription: %v", k.kind, err)
		}
		recorder := httptest.NewRecorder()
		writeQuotaRefusal(recorder, err)
		wantStatus(t, recorder, http.StatusConflict, errCodeNoSubscription)
	}

	usage, err := f.panel.subscriptionUsageFor(ctx, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Domains != 1 || usage.Databases != 1 || usage.MailAccounts != 1 || usage.DiskUsedBytes != 1048576 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestPlanFormZeroIsUnlimitedAndNegativeIsRefused(t *testing.T) {
	f := newAccountsFixture(t)
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Open","max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Bad","max_domains":-1,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0}`), http.StatusBadRequest, "")
	// What the list sends back (owner, mine, subscribers) is accepted and ignored.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Echo","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"owner_id":9702,"owner":"x","mine":true,"subscribers":3}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM service_plans WHERE name = 'Echo' AND owner_id IS NULL`); n != 1 {
		t.Fatalf("an owner from the request body was stored (%d global Echo plans)", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'plan.create:%'`); n != 2 {
		t.Fatalf("plan.create audit rows = %d, want 2", n)
	}
}

func createPlan(t *testing.T, f accountsFixture, userID int, body string) int {
	t.Helper()
	recorder := f.as(t, userID, http.MethodPost, "/api/v1/plans", body)
	wantStatus(t, recorder, http.StatusOK, "")
	var answer struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.ID == 0 {
		t.Fatalf("plan answer %s: %v", recorder.Body.String(), err)
	}
	return answer.ID
}

func TestPlanAssignmentChangesLimitsAndWarnsBelowUsage(t *testing.T) {
	f := newAccountsFixture(t)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9904, 9802, 'second.matrix.test', 'active')`)
	small := createPlan(t, f, authzMatrixAdminID, `{"name":"Small","max_domains":1,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":100,"bandwidth_quota_mb":0}`)
	large := createPlan(t, f, authzMatrixAdminID, `{"name":"Large","max_domains":10,"max_databases":10,"max_email_accounts":10,"disk_quota_mb":1000,"bandwidth_quota_mb":0}`)

	// Below usage: refused with the list, nothing changed.
	recorder := f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(small)+`}`)
	wantStatus(t, recorder, http.StatusConflict, errCodePlanBelowUsage)
	if !strings.Contains(recorder.Body.String(), "domains: 2 in use, limit 1") {
		t.Fatalf("the refusal does not list what is over: %s", recorder.Body.String())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE id = 9802 AND plan_id IS NULL`); n != 1 {
		t.Fatal("a refused downgrade changed the subscription")
	}

	// Confirmed: applied, nothing deleted, new domains refused.
	recorder = f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(small)+`,"accept_over_usage":true}`)
	wantStatus(t, recorder, http.StatusOK, "")
	if !strings.Contains(recorder.Body.String(), "domains: 2 in use, limit 1") {
		t.Fatalf("the answer does not repeat the warning: %s", recorder.Body.String())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM domains WHERE subscription_id = 9802`); n != 2 {
		t.Fatalf("domains after downgrade = %d, want 2 (nothing deleted)", n)
	}
	var exceeded *quotaExceededError
	if err := f.panel.checkSubscriptionQuota(context.Background(), 9802, quotaDomains); !errors.As(err, &exceeded) {
		t.Fatalf("a new domain over the downgraded plan: %v", err)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'subscription.plan_change:matrix-customer' AND user_id = 9702`); n != 1 {
		t.Fatalf("plan change audit rows = %d", n)
	}

	// Upgrade: limits re-copied.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(large)+`}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE id = 9802 AND plan_id = ? AND max_domains = 10 AND disk_quota_mb = 1000`, large); n != 1 {
		t.Fatal("the upgrade did not copy the plan's limits")
	}

	// A reseller never reaches an account outside its tree; a customer never
	// changes plans.
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9704/plan", `{"plan_id":`+itoa(large)+`}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, authzMatrixCustomerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(large)+`}`), http.StatusForbidden, "")
	wantStatus(t, f.send(t, http.MethodPut, "/api/v1/users/9703/plan", f.memberToken, `{"plan_id":1}`), http.StatusForbidden, errCodeAdditionalUserScope)

	// An account without a subscription is assigned one.
	f.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES (9706, 'fresh-customer', 'x', 'fresh@example.test', 'customer', 9702, 'active')`)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9706/plan", `{"plan_id":`+itoa(small)+`}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE owner_id = 9706 AND plan_id = ? AND max_domains = 1`, small); n != 1 {
		t.Fatal("the assignment did not create the subscription")
	}

	// A reseller's own plan goes only to that reseller's customers.
	own := createPlan(t, f, authzMatrixResellerID, `{"name":"Reseller own","max_domains":3,"max_databases":3,"max_email_accounts":3,"disk_quota_mb":300,"bandwidth_quota_mb":0}`)
	if n := f.count(t, `SELECT COUNT(*) FROM service_plans WHERE id = ? AND owner_id = 9702`, own); n != 1 {
		t.Fatal("a reseller's plan is not owned by the reseller")
	}
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9704/plan", `{"plan_id":`+itoa(own)+`}`), http.StatusConflict, errCodePlanNotAvailable)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(own)+`}`), http.StatusOK, "")
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestPlanVisibilityAndEditingFollowOwnership(t *testing.T) {
	f := newAccountsFixture(t)
	global := createPlan(t, f, authzMatrixAdminID, `{"name":"Global","max_domains":5,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":500,"bandwidth_quota_mb":0}`)
	own := createPlan(t, f, authzMatrixResellerID, `{"name":"Mine","max_domains":2,"max_databases":2,"max_email_accounts":2,"disk_quota_mb":200,"bandwidth_quota_mb":0}`)
	f.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES (9707, 'other-reseller', 'x', 'other-reseller@example.test', 'reseller', 9701, 'active')`)
	token, err := f.panel.sessions.Create(context.Background(), 9707)
	if err != nil {
		t.Fatal(err)
	}
	other := f.send(t, http.MethodGet, "/api/v1/plans", token, "")
	wantStatus(t, other, http.StatusOK, "")
	if strings.Contains(other.Body.String(), `"Mine"`) || !strings.Contains(other.Body.String(), `"Global"`) {
		t.Fatalf("another reseller's list: %s", other.Body.String())
	}
	wantStatus(t, f.send(t, http.MethodPut, "/api/v1/plans/"+itoa(own), token,
		`{"name":"Taken","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/plans/"+itoa(global),
		`{"name":"Taken","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodGet, "/api/v1/plans", ""), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/plans", ""), http.StatusForbidden, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodDelete, "/api/v1/plans/"+itoa(own), ""), http.StatusOK, "")
}

func TestResellerPoolRefusesOverAllocation(t *testing.T) {
	f := newAccountsFixture(t)
	big := createPlan(t, f, authzMatrixAdminID, `{"name":"Big","max_domains":20,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":500,"bandwidth_quota_mb":0}`)
	small := createPlan(t, f, authzMatrixAdminID, `{"name":"Small","max_domains":2,"max_databases":2,"max_email_accounts":2,"disk_quota_mb":100,"bandwidth_quota_mb":0}`)
	// The reseller's tree today: its own subscription 9801 and its
	// customer's 9802, both with the schema defaults (5 domains each).
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":1,"max_domains":15,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9702/pool", `{"pool":null}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/pool", `{"pool":null}`), http.StatusBadRequest, "")

	create := `{"username":"pool-customer","email":"pool@example.test","password":"long-enough-1","role":"customer"}`
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", create), http.StatusConflict, errCodeResellerCustomersLimit)

	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":5,"max_domains":15,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	withBig := `{"username":"pool-customer","email":"pool@example.test","password":"long-enough-1","role":"customer","plan_id":` + itoa(big) + `}`
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", withBig), http.StatusConflict, errCodeResellerPoolExceeded)
	if n := f.count(t, `SELECT COUNT(*) FROM users WHERE username = 'pool-customer'`); n != 0 {
		t.Fatal("a refused account was created")
	}
	withSmall := `{"username":"pool-customer","email":"pool@example.test","password":"long-enough-1","role":"customer","plan_id":` + itoa(small) + `}`
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", withSmall), http.StatusOK, "")

	// Raising an existing customer past the pool is refused too.
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(big)+`}`), http.StatusConflict, errCodeResellerPoolExceeded)

	// A reseller's own plan stays inside the pool (unlimited is not inside a limited pool).
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Too big","max_domains":16,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusConflict, errCodePlanExceedsPool)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Unlimited","max_domains":0,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusConflict, errCodePlanExceedsPool)

	// Editing a global plan that would push the tree over the pool is refused.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(small),
		`{"name":"Small","max_domains":30,"max_databases":2,"max_email_accounts":2,"disk_quota_mb":100,"bandwidth_quota_mb":0}`), http.StatusConflict, errCodeResellerPoolExceeded)

	// Lowering the pool below what is handed out is stored with a warning.
	recorder := f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":5,"max_domains":3,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`)
	wantStatus(t, recorder, http.StatusOK, "")
	if !strings.Contains(recorder.Body.String(), `"over_limit":["domains:`) {
		t.Fatalf("a pool below the allocation carries no warning: %s", recorder.Body.String())
	}
	// ...and a change that does not grow the tree still works under it.
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(small)+`}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'reseller.pool:matrix-reseller'`); n != 3 {
		t.Fatalf("pool audit rows = %d, want 3", n)
	}
}

func TestResellerSuspendsAndResumesOwnCustomer(t *testing.T) {
	f := newAccountsFixture(t)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703", `{"status":"suspended"}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixCustomerID, http.MethodGet, mySubscriptionPath, ""), http.StatusUnauthorized, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703", `{"status":"active"}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9704", `{"status":"suspended"}`), http.StatusNotFound, "")
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE user_id = 9702 AND action IN ('user.suspend:matrix-customer', 'user.resume:matrix-customer')`); n != 2 {
		t.Fatalf("suspend/resume audit rows = %d, want 2", n)
	}
}

func TestHandleCreateUserRoleRules(t *testing.T) {
	f := newAccountsFixture(t)
	body := func(name, role string) string {
		return `{"username":"` + name + `","email":"` + name + `@example.test","password":"long-enough-1","role":"` + role + `"}`
	}
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", body("sub-reseller", "reseller")), http.StatusBadRequest, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", body("sub-admin", "admin")), http.StatusBadRequest, "")
	wantStatus(t, f.as(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/users", body("by-customer", "customer")), http.StatusForbidden, "")
	wantStatus(t, f.send(t, http.MethodPost, "/api/v1/users", f.memberToken, body("by-member", "customer")), http.StatusForbidden, errCodeAdditionalUserScope)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", body("pool-free", "customer")), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users", body("pool-free", "customer")), http.StatusConflict, "")
	if n := f.count(t, `SELECT COUNT(*) FROM users WHERE username = 'pool-free' AND parent_id = 9702 AND role = 'customer'`); n != 1 {
		t.Fatal("the reseller's customer is not under the reseller")
	}
	// A pool only from the administrator, only for a reseller.
	withPool := `{"username":"pooled","email":"pooled@example.test","password":"long-enough-1","role":"customer","pool":{"max_customers":1}}`
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPost, "/api/v1/users", withPool), http.StatusBadRequest, "")
	reseller := `{"username":"pooled","email":"pooled@example.test","password":"long-enough-1","role":"reseller","pool":{"max_customers":2,"max_domains":10}}`
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPost, "/api/v1/users", reseller), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM reseller_pools rp JOIN users u ON u.id = rp.reseller_id WHERE u.username = 'pooled' AND rp.max_customers = 2 AND rp.max_domains = 10`); n != 1 {
		t.Fatal("the new reseller's pool was not stored")
	}
}

func TestCanManageUser(t *testing.T) {
	reseller := 2
	other := 3
	admin := &Caller{ID: 1, Role: roleAdmin}
	res := &Caller{ID: reseller, Role: roleReseller}
	cases := []struct {
		caller *Caller
		target core.User
		want   bool
	}{
		{admin, core.User{ID: 9, Role: roleAdmin}, false},
		{admin, core.User{ID: 9, Role: roleReseller}, true},
		{admin, core.User{ID: 9, Role: roleCustomer, ParentID: &reseller}, true},
		{res, core.User{ID: 9, Role: roleCustomer, ParentID: &reseller}, true},
		{res, core.User{ID: 9, Role: roleCustomer, ParentID: &other}, false},
		{res, core.User{ID: 9, Role: roleCustomer}, false},
		{res, core.User{ID: reseller, Role: roleReseller}, false},
	}
	for i, c := range cases {
		target := c.target
		if got := canManageUser(c.caller, &target); got != c.want {
			t.Errorf("case %d: canManageUser = %v, want %v", i, got, c.want)
		}
	}
}

func impersonate(t *testing.T, f accountsFixture, operatorID int, path string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	recorder := f.as(t, operatorID, http.MethodPost, path, "")
	wantStatus(t, recorder, http.StatusOK, "")
	var session, original *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		switch cookie.Name {
		case sessionCookieName:
			session = cookie
		case impersonatorCookieName:
			original = cookie
		}
	}
	if session == nil || original == nil {
		t.Fatalf("impersonation cookies missing: %v", recorder.Result().Cookies())
	}
	return session, original
}

func TestImpersonationIsAuditedWithActingAs(t *testing.T) {
	f := newAccountsFixture(t)
	session, original := impersonate(t, f, authzMatrixAdminID, "/api/v1/users/9703/impersonate")
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'impersonate.start:matrix-customer' AND user_id = 9701 AND acting_as = 9703`); n != 1 {
		t.Fatalf("impersonate.start rows = %d", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM sessions WHERE user_id = 9703 AND impersonator_id = 9701`); n != 1 {
		t.Fatal("the session does not record its operator")
	}

	me := f.send(t, http.MethodGet, "/api/v1/auth/me", session.Value, "", original)
	wantStatus(t, me, http.StatusOK, "")
	if !strings.Contains(me.Body.String(), `"impersonating":true`) || !strings.Contains(me.Body.String(), `"impersonator":"matrix-admin"`) {
		t.Fatalf("/auth/me during impersonation: %s", me.Body.String())
	}

	// Actions inside "Log in as" name the real person and the account.
	wantStatus(t, f.send(t, http.MethodPost, "/api/v1/test/audited", session.Value, ""), http.StatusNoContent, "")
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'user.create:probe/customer' AND user_id = 9701 AND acting_as = 9703`); n != 1 {
		t.Fatalf("an action under impersonation is not attributed to the operator (%d rows)", n)
	}

	// The way back restores the operator and is recorded.
	back := f.send(t, http.MethodPost, "/api/v1/auth/unimpersonate", session.Value, "", original)
	wantStatus(t, back, http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'impersonate.stop' AND user_id = 9701 AND acting_as = 9703`); n != 1 {
		t.Fatalf("impersonate.stop rows = %d", n)
	}
	wantStatus(t, f.send(t, http.MethodGet, "/api/v1/auth/me", session.Value, ""), http.StatusUnauthorized, "")
}

func TestImpersonationEndsWithTheOperatorsAuthority(t *testing.T) {
	f := newAccountsFixture(t)
	// Nested "Log in as" is refused.
	asReseller, _ := impersonate(t, f, authzMatrixAdminID, "/api/v1/users/9702/impersonate")
	wantStatus(t, f.send(t, http.MethodPost, "/api/v1/users/9703/impersonate", asReseller.Value, ""), http.StatusConflict, "")

	// A reseller's session as its customer dies when the reseller is suspended.
	asCustomer, _ := impersonate(t, f, authzMatrixResellerID, "/api/v1/users/9703/impersonate")
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, asCustomer.Value, ""), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702", `{"status":"suspended"}`), http.StatusOK, "")
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, asCustomer.Value, ""), http.StatusUnauthorized, "")
	if n := f.count(t, `SELECT COUNT(*) FROM sessions WHERE impersonator_id = 9702`); n != 0 {
		t.Fatalf("the suspended operator's impersonation sessions remain (%d)", n)
	}

	// A session whose operator lost authority over the account (moved to
	// another parent) is refused even though the row still exists.
	f.exec(t, `UPDATE users SET status = 'active' WHERE id = 9702`)
	token, err := f.panel.sessions.Create(context.Background(), authzMatrixResellerID)
	if err != nil {
		t.Fatal(err)
	}
	f.tokens[authzMatrixResellerID] = token
	moved, _ := impersonate(t, f, authzMatrixResellerID, "/api/v1/users/9703/impersonate")
	f.exec(t, `UPDATE users SET parent_id = 9701 WHERE id = 9703`)
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, moved.Value, ""), http.StatusUnauthorized, "")
}

func TestPasswordChangeDropsOtherSessions(t *testing.T) {
	f := newAccountsFixture(t)
	hash, err := auth.HashPassword("old-password-1")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE users SET password_hash = ? WHERE id IN (9702, 9703)`, hash)
	other, err := f.panel.sessions.Create(context.Background(), authzMatrixCustomerID)
	if err != nil {
		t.Fatal(err)
	}
	// A "Log in as" session the reseller opened for the customer.
	asCustomer, _ := impersonate(t, f, authzMatrixResellerID, "/api/v1/users/9703/impersonate")

	wantStatus(t, f.as(t, authzMatrixCustomerID, http.MethodPost, "/api/v1/auth/password",
		`{"current_password":"old-password-1","new_password":"new-password-1"}`), http.StatusOK, "")
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, other, ""), http.StatusUnauthorized, "")
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, asCustomer.Value, ""), http.StatusUnauthorized, "")

	// The operator's own password change ends the sessions it opened as others.
	again, _ := impersonate(t, f, authzMatrixResellerID, "/api/v1/users/9703/impersonate")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/auth/password",
		`{"current_password":"old-password-1","new_password":"new-password-1"}`), http.StatusOK, "")
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, again.Value, ""), http.StatusUnauthorized, "")
}

func TestMySubscriptionShowsPlanUsageAndContact(t *testing.T) {
	f := newAccountsFixture(t)
	f.seedUsage(t)
	plan := createPlan(t, f, authzMatrixAdminID, `{"name":"Starter","max_domains":3,"max_databases":0,"max_email_accounts":4,"disk_quota_mb":0,"bandwidth_quota_mb":50000}`)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")

	recorder := f.as(t, authzMatrixCustomerID, http.MethodGet, mySubscriptionPath, "")
	wantStatus(t, recorder, http.StatusOK, "")
	var answer struct {
		Subscriptions []struct {
			ID    int        `json:"id"`
			Plan  *myPlanRef `json:"plan"`
			Usage struct {
				Domains        int `json:"domains"`
				DomainsLimit   int `json:"domains_limit"`
				DatabasesLimit int `json:"databases_limit"`
				MailAccounts   int `json:"mail_accounts"`
			} `json:"usage"`
		} `json:"subscriptions"`
		Contact   myContact       `json:"contact"`
		Bandwidth map[string]bool `json:"bandwidth"`
		Pool      json.RawMessage `json:"pool"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Subscriptions) != 1 || answer.Subscriptions[0].Plan == nil || answer.Subscriptions[0].Plan.Name != "Starter" {
		t.Fatalf("subscriptions: %s", recorder.Body.String())
	}
	u := answer.Subscriptions[0].Usage
	if u.Domains != 1 || u.DomainsLimit != 3 || u.DatabasesLimit != 0 || u.MailAccounts != 1 {
		t.Fatalf("usage: %s", recorder.Body.String())
	}
	if answer.Contact.Kind != "reseller" || answer.Contact.Name != "matrix-reseller" {
		t.Fatalf("contact: %+v", answer.Contact)
	}
	if answer.Bandwidth["measured"] {
		t.Fatal("traffic is not measured, but the answer says it is")
	}
	if len(answer.Pool) != 0 {
		t.Fatal("a customer was shown a reseller pool")
	}

	reseller := f.as(t, authzMatrixResellerID, http.MethodGet, mySubscriptionPath, "")
	wantStatus(t, reseller, http.StatusOK, "")
	if !strings.Contains(reseller.Body.String(), `"pool":{"limits":null`) || !strings.Contains(reseller.Body.String(), `"kind":"administrator"`) {
		t.Fatalf("reseller answer: %s", reseller.Body.String())
	}
	admin := f.as(t, authzMatrixAdminID, http.MethodGet, mySubscriptionPath, "")
	wantStatus(t, admin, http.StatusOK, "")
	if !strings.Contains(admin.Body.String(), `"kind":"none"`) {
		t.Fatalf("admin answer: %s", admin.Body.String())
	}
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, f.memberToken, ""), http.StatusForbidden, errCodeAdditionalUserScope)
	wantStatus(t, f.send(t, http.MethodGet, mySubscriptionPath, "", ""), http.StatusUnauthorized, "")
}

// TestAccountRoutesRoleMatrix drives the account routes' real handlers for
// every role (the handler layer under the gate the route matrix proves).
func TestAccountRoutesRoleMatrix(t *testing.T) {
	type cell struct {
		method, path, body string
	}
	const anon, member = -1, -2
	roles := []int{anon, authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, member}
	rows := []struct {
		cell
		want [5]int
	}{
		{cell{http.MethodGet, "/api/v1/users", ""}, [5]int{401, 200, 200, 403, 403}},
		{cell{http.MethodGet, "/api/v1/plans", ""}, [5]int{401, 200, 200, 403, 403}},
		{cell{http.MethodGet, "/api/v1/subscriptions", ""}, [5]int{401, 200, 200, 200, 403}},
		{cell{http.MethodGet, mySubscriptionPath, ""}, [5]int{401, 200, 200, 200, 403}},
		{cell{http.MethodGet, "/api/v1/users/9702/pool", ""}, [5]int{401, 200, 404, 403, 403}},
		{cell{http.MethodPut, "/api/v1/users/9703/plan", "plan"}, [5]int{401, 200, 200, 403, 403}},
		{cell{http.MethodPut, "/api/v1/users/9704/plan", "plan"}, [5]int{401, 200, 404, 403, 403}},
		{cell{http.MethodPut, "/api/v1/users/9703", `{"email":"matrix-customer@example.test"}`}, [5]int{401, 200, 200, 403, 403}},
		{cell{http.MethodPut, "/api/v1/users/9704", `{"email":"matrix-outsider@example.test"}`}, [5]int{401, 200, 404, 403, 403}},
		{cell{http.MethodPut, "/api/v1/users/9701", `{"email":"matrix-admin@example.test"}`}, [5]int{401, 404, 404, 403, 403}},
	}
	for _, row := range rows {
		for i, role := range roles {
			f := newAccountsFixture(t)
			plan := createPlan(t, f, authzMatrixAdminID, `{"name":"Matrix","max_domains":9,"max_databases":9,"max_email_accounts":9,"disk_quota_mb":900,"bandwidth_quota_mb":0}`)
			body := row.body
			if body == "plan" {
				body = `{"plan_id":` + itoa(plan) + `}`
			}
			token := ""
			switch role {
			case anon:
			case member:
				token = f.memberToken
			default:
				token = f.tokens[role]
			}
			recorder := f.send(t, row.method, row.path, token, body)
			if recorder.Code != row.want[i] {
				t.Errorf("%s %s as role #%d: %d, want %d; body=%s", row.method, row.path, i, recorder.Code, row.want[i], recorder.Body.String())
			}
		}
	}
}
