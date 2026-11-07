package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.128 G22: the domain health strip reads stored answers only, says
// "not checked yet" for what it does not know, and gives each viewer the fix
// that viewer can do (or whom to contact).

type domainHealthTestBody struct {
	Domain  string              `json:"domain"`
	Contact string              `json:"contact"`
	Checks  []domainHealthCheck `json:"checks"`
}

func (b domainHealthTestBody) check(id string) *domainHealthCheck {
	for i := range b.Checks {
		if b.Checks[i].ID == id {
			return &b.Checks[i]
		}
	}
	return nil
}

func readDomainHealth(t *testing.T, fixture authzMatrixFixture, caller *Caller, domainID int) (int, domainHealthTestBody) {
	t.Helper()
	request := teamMemberAuthzTestRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/health", domainID), caller)
	recorder := httptest.NewRecorder()
	fixture.panel.handleDomainSubroute(recorder, request)
	var body domainHealthTestBody
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (%s)", err, recorder.Body.String())
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q", got)
		}
	}
	return recorder.Code, body
}

func seedDomainHealthSite(t *testing.T, fixture authzMatrixFixture, now time.Time) {
	t.Helper()
	db := fixture.panel.db.GetDB()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO sites (domain_id, document_root, php_version, project_type) VALUES (?, '/srv/customer.matrix.test', '8.3', 'php')`,
			[]any{authzMatrixCustomerDomainID}},
		{`INSERT INTO ssl_certificates (domain_id, type, cert_path, key_path, expires_at, status) VALUES (?, 'letsencrypt', '/c/f.pem', '/c/k.pem', ?, 'active')`,
			[]any{authzMatrixCustomerDomainID, now.Add(10 * 24 * time.Hour).Format(time.RFC3339)}},
		{`INSERT INTO site_uptime (domain, checked_at, state, scheme, reason, down_since) VALUES ('customer.matrix.test', ?, 'down', 'https', 'timeout', ?)`,
			[]any{now.Add(-time.Minute).Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339)}},
		{`UPDATE subscriptions SET disk_quota_mb = 100 WHERE id = ?`, []any{authzMatrixCustomerSubID}},
		{`INSERT INTO subscription_disk_usage (subscription_id, web_bytes, state, measured_at) VALUES (?, ?, 'measured', ?)`,
			[]any{authzMatrixCustomerSubID, 90 * mib, now.Format(time.RFC3339)}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed %q: %v", statement.query, err)
		}
	}
}

func TestDomainHealthStripIsRoleAwareAndStoredOnly(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	now := time.Now().UTC()
	seedDomainHealthSite(t, fixture, now)

	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: authzMatrixCustomerID}
	reseller := &Caller{ID: authzMatrixResellerID, Role: roleReseller, AccountType: core.AccountTypeAccount}
	outsider := &Caller{ID: authzMatrixOutsiderID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: authzMatrixOutsiderID}

	code, body := readDomainHealth(t, fixture, admin, authzMatrixCustomerDomainID)
	if code != http.StatusOK {
		t.Fatalf("admin status %d", code)
	}
	site := body.check("site")
	if site == nil || site.State != "problem" || site.Code != "down" || site.Reason != "timeout" ||
		site.Fix == nil || site.Fix.Kind != "page" || site.Fix.Path != "/monitoring" {
		t.Fatalf("admin site check = %+v", site)
	}
	certificate := body.check("certificate")
	if certificate == nil || certificate.State != "problem" || certificate.Code != "expiring" ||
		certificate.Days == nil || *certificate.Days > 10 || certificate.Fix == nil || certificate.Fix.Tab != "hosting" || certificate.Fix.Sub != "ssl" {
		t.Fatalf("admin certificate check = %+v", certificate)
	}
	disk := body.check("disk")
	if disk == nil || disk.State != "problem" || disk.Code != "near" || disk.Percent == nil || *disk.Percent != 90 ||
		disk.Fix == nil || disk.Fix.Kind != "page" || disk.Fix.Path != "/users" {
		t.Fatalf("admin disk check = %+v", disk)
	}
	backup := body.check("backup")
	if backup == nil || backup.State != "problem" || backup.Code != "none" || backup.Fix == nil || backup.Fix.Sub != "backups" {
		t.Fatalf("admin backup check = %+v", backup)
	}
	if body.check("mail") != nil {
		t.Fatalf("a domain without mailboxes and without a mail answer has no mail line: %+v", body.Checks)
	}

	// The customer of a reseller: the server's problem goes to the provider.
	code, body = readDomainHealth(t, fixture, customer, authzMatrixCustomerDomainID)
	if code != http.StatusOK || body.Contact != "provider" {
		t.Fatalf("customer status %d contact %q", code, body.Contact)
	}
	if site := body.check("site"); site == nil || site.Fix == nil || site.Fix.Kind != "contact" {
		t.Fatalf("customer site fix = %+v", site)
	}
	if disk := body.check("disk"); disk == nil || disk.Fix == nil || disk.Fix.Kind == "page" {
		t.Fatalf("customer may not change the plan: %+v", disk)
	}

	// The reseller sets its customer's limits.
	code, body = readDomainHealth(t, fixture, reseller, authzMatrixCustomerDomainID)
	if code != http.StatusOK || body.Contact != "administrator" {
		t.Fatalf("reseller status %d contact %q", code, body.Contact)
	}
	if disk := body.check("disk"); disk == nil || disk.Fix == nil || disk.Fix.Kind != "page" {
		t.Fatalf("reseller disk fix = %+v", disk)
	}

	if code, _ := readDomainHealth(t, fixture, outsider, authzMatrixCustomerDomainID); code != http.StatusNotFound {
		t.Fatalf("outsider status %d, want 404", code)
	}
}

func TestDomainHealthUnknownIsNeverGreen(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	db := fixture.panel.db.GetDB()
	if _, err := db.Exec(`INSERT INTO sites (domain_id, document_root, project_type) VALUES (?, '/srv/x', 'php')`, authzMatrixCustomerDomainID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO email_accounts (domain_id, address, password_hash) VALUES (?, 'a@customer.matrix.test', 'x')`, authzMatrixCustomerDomainID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO backup_schedules (domain_id, frequency) VALUES (?, 'daily')`, authzMatrixCustomerDomainID); err != nil {
		t.Fatal(err)
	}
	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	_, body := readDomainHealth(t, fixture, admin, authzMatrixCustomerDomainID)
	want := map[string]string{"site": "not_checked", "mail": "not_checked", "disk": "not_measured", "backup": "first_pending"}
	for id, code := range want {
		check := body.check(id)
		if check == nil || check.State != "unknown" || check.Code != code {
			t.Errorf("%s = %+v, want unknown/%s", id, check, code)
		}
	}
	if certificate := body.check("certificate"); certificate == nil || certificate.State != "problem" || certificate.Code != "none" {
		t.Errorf("certificate = %+v, want problem/none", certificate)
	}

	// A stale site check is not the current state.
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO site_uptime (domain, checked_at, state, scheme) VALUES ('customer.matrix.test', ?, 'up', 'http')`, old); err != nil {
		t.Fatal(err)
	}
	_, body = readDomainHealth(t, fixture, admin, authzMatrixCustomerDomainID)
	if site := body.check("site"); site == nil || site.State != "unknown" || site.Code != "stale" {
		t.Errorf("stale site = %+v", site)
	}
}

func TestDomainHealthMailReadsTheLastAnswerOnly(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	previous := displayChecksEnabled.Load()
	displayChecksEnabled.Store(true)
	t.Cleanup(func() { displayChecksEnabled.Store(previous) })

	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	key := fmt.Sprintf("%d|%s|%s", authzMatrixCustomerDomainID, "customer.matrix.test", setupDNSModeLocal)
	// No answer kept: the strip asks nothing (no mailbox, no line).
	_, body := readDomainHealth(t, fixture, admin, authzMatrixCustomerDomainID)
	if body.check("mail") != nil {
		t.Fatalf("mail line without an answer or mailbox: %+v", body.Checks)
	}
	_, _, err := fixture.panel.displayChecks.ready().mailHealth.get(context.Background(), key,
		func(context.Context) (mailHealthReport, error) {
			return mailHealthReport{Checks: []healthCheck{
				{ID: "spf", Status: "ok"}, {ID: "dkim", Status: "fail"}, {ID: "dmarc", Status: "fail"},
				{ID: "port25", Status: "ok"}, {ID: "rbl", Status: "ok"},
			}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	_, body = readDomainHealth(t, fixture, admin, authzMatrixCustomerDomainID)
	mail := body.check("mail")
	if mail == nil || mail.State != "problem" || mail.Code != "auth_missing" || len(mail.Items) != 2 ||
		mail.Items[0] != "dkim" || mail.Fix == nil || mail.Fix.Tab != "mail" {
		t.Fatalf("mail = %+v", mail)
	}
}

func TestDomainHealthTeamMemberSeesOnlyGrantedChecks(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	seedDomainHealthSite(t, fixture, time.Now().UTC())
	member := teamMemberAuthzTestCaller()

	// No grant at all: the domain is hidden.
	if code, _ := readDomainHealth(t, fixture, member, authzMatrixCustomerDomainID); code != http.StatusNotFound {
		t.Fatalf("ungranted member status %d, want 404", code)
	}
	grantTeamMemberDomainCapability(t, &fixture, authzMatrixCustomerDomainID, core.TeamCapabilitySSL, core.TeamPermissionView)
	code, body := readDomainHealth(t, fixture, member, authzMatrixCustomerDomainID)
	if code != http.StatusOK || body.Contact != "owner" {
		t.Fatalf("member status %d contact %q", code, body.Contact)
	}
	if len(body.Checks) != 1 || body.Checks[0].ID != "certificate" {
		t.Fatalf("member checks = %+v, want the certificate only", body.Checks)
	}
	if fix := body.Checks[0].Fix; fix == nil || fix.Kind != "contact" {
		t.Fatalf("a view-only member is told whom to contact: %+v", fix)
	}
	if _, err := fixture.panel.db.GetDB().Exec(`UPDATE additional_user_domain_permissions SET mode = 'manage' WHERE user_id = ?`, authzMatrixAdditionalUserID); err != nil {
		t.Fatal(err)
	}
	_, body = readDomainHealth(t, fixture, member, authzMatrixCustomerDomainID)
	if fix := body.Checks[0].Fix; fix == nil || fix.Kind != "tab" || fix.Sub != "ssl" {
		t.Fatalf("a managing member gets the tab: %+v", fix)
	}
}

func TestDomainHealthRouteIsGetOnly(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		match, ok := matchDomainSubroute(httptest.NewRequest(method, "/api/v1/domains/7/health", nil))
		if !ok || match.kind != domainHealthKind || routeAllows(method, match.methods) {
			t.Fatalf("%s health: %+v %v", method, match, ok)
		}
	}
	if _, mapped := teamMemberDomainRequirementFor(domainHealthKind, http.MethodPost); mapped {
		t.Fatal("no change is mapped for the health strip")
	}
}
