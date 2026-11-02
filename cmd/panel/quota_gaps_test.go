package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Two quota gaps: the domain-tab database creation must meet the same
// subscription limit as the Databases page, and editing a plan below what a
// subscriber uses must be confirmed like giving a plan to an account is.
// İki kota açığı: alan adı sekmesinden veritabanı oluşturma da abonelik
// sınırına uymalı; plan düzenlemesi kullanımın altına inerse onaylanmalı.

func TestDomainTabDatabaseCreationRefusedAtSubscriptionLimit(t *testing.T) {
	agent := &databaseAdminAccountAgent{services: adminAccountServices()}
	panel, database := newDatabaseAdminAccountFixture(t, agent)
	ctx := context.Background()
	pool := database.GetDB()

	result, err := pool.Exec(`INSERT INTO domains (subscription_id, name, status) VALUES (?, 'quota-tab.example.test', 'active')`, adminAccountSubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	domainID64, _ := result.LastInsertId()
	domainID := int(domainID64)

	// One database already exists on the subscription; its limit is 1.
	if err := panel.ensureInstalledDBServers(ctx, adminAccountSubscriptionID); err != nil {
		t.Fatal(err)
	}
	server := onlyDatabaseServer(t, panel)
	if _, err := pool.Exec(`INSERT INTO databases_v2 (server_id, subscription_id, name) VALUES (?, ?, 'existing_db')`,
		server.ID, adminAccountSubscriptionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(`UPDATE subscriptions SET max_databases = 1 WHERE id = ?`, adminAccountSubscriptionID); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"name":"extra","type":"mysql","password":"Sup3r-secret-pass"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/1/databases", body)
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: adminAccountUserID, Role: roleAdmin}))
	recorder := httptest.NewRecorder()
	panel.handleCreateDatabase(recorder, request, domainID)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", recorder.Code, recorder.Body.String())
	}
	var answer struct {
		Code   string `json:"code"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Code != errCodeQuotaDatabases || answer.Action != myPlanPagePath {
		t.Fatalf("refusal = %+v, want %s with the My plan action", answer, errCodeQuotaDatabases)
	}
	var count int
	if err := pool.QueryRow(`SELECT COUNT(*) FROM databases_v2 WHERE subscription_id = ?`, adminAccountSubscriptionID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("databases after refusal = %d (%v), want 1", count, err)
	}
}

func TestPlanEditBelowUsageNeedsConfirmation(t *testing.T) {
	f := newAccountsFixture(t)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9904, 9802, 'second.matrix.test', 'active')`)
	plan := createPlan(t, f, authzMatrixAdminID, `{"name":"Edit me","max_domains":10,"max_databases":10,"max_email_accounts":10,"disk_quota_mb":1000,"bandwidth_quota_mb":0}`)
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	path := "/api/v1/plans/" + itoa(plan)
	lowered := `{"name":"Edit me","max_domains":1,"max_databases":10,"max_email_accounts":10,"disk_quota_mb":1000,"bandwidth_quota_mb":0`

	// Below usage: refused with the list, nothing changed.
	recorder := f.as(t, authzMatrixAdminID, http.MethodPut, path, lowered+`}`)
	wantStatus(t, recorder, http.StatusConflict, errCodePlanBelowUsage)
	for _, want := range []string{"matrix-customer", "domains: 2 in use, limit 1"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("the refusal does not list %q: %s", want, recorder.Body.String())
		}
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_plans WHERE id = ? AND max_domains = 10`, plan); n != 1 {
		t.Fatal("a refused edit changed the plan")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE id = 9802 AND max_domains = 10`); n != 1 {
		t.Fatal("a refused edit changed the subscription")
	}

	// A lowered limit that still fits the usage needs no confirmation.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, path,
		`{"name":"Edit me","max_domains":2,"max_databases":10,"max_email_accounts":10,"disk_quota_mb":1000,"bandwidth_quota_mb":0}`), http.StatusOK, "")

	// Confirmed: saved, nothing deleted, audited with the list, new domains refused.
	recorder = f.as(t, authzMatrixAdminID, http.MethodPut, path, lowered+`,"accept_over_usage":true}`)
	wantStatus(t, recorder, http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE id = 9802 AND max_domains = 1`); n != 1 {
		t.Fatal("the confirmed edit did not reach the subscription")
	}
	if n := f.count(t, `SELECT COUNT(*) FROM domains WHERE subscription_id = 9802`); n != 2 {
		t.Fatalf("domains after confirmed edit = %d, want 2 (nothing deleted)", n)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action = 'plan.update:Edit me' AND details LIKE '%over_limit%' AND details LIKE '%domains: 2 in use, limit 1%'`); n != 1 {
		t.Fatalf("confirmed edit audit rows with the over list = %d, want 1", n)
	}
	if err := f.panel.checkSubscriptionQuota(context.Background(), 9802, quotaDomains); err == nil {
		t.Fatal("a new domain was allowed above the lowered limit")
	}

	// 0 (unlimited) is never over.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, path,
		`{"name":"Edit me","max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0}`), http.StatusOK, "")
}
