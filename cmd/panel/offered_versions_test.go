package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// alpha.108 (D-038 §14): the administrator chooses which versions customers
// are offered; a customer choosing another is refused with a coded answer,
// an administrator never is, and a site keeps the version it already runs.

func customerRequest(method, target, body string, userID int) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), callerKey, &Caller{ID: userID, Role: roleCustomer}))
}

func seedPHPSite(t *testing.T, panel *Panel, userID int, version string) {
	t.Helper()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO subscriptions (id, owner_id, name, max_databases, status) VALUES (9201, ?, 'Sites', 1, 'active')`, []any{userID}},
		{`INSERT INTO domains (id, subscription_id, name, status) VALUES (9202, 9201, 'php.example', 'active')`, nil},
		{`INSERT INTO sites (id, domain_id, document_root, project_type, php_version, status)
		VALUES (9203, 9202, '/var/www/celikpanel/subscriptions/9201/sites/9202/public_html', 'php', ?, 'active')`, []any{version}},
	} {
		if _, err := panel.db.GetDB().Exec(statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func setOffered(t *testing.T, panel *Panel, adminID int, component, version string, offered bool) {
	t.Helper()
	body, _ := json.Marshal(offeredVersionChange{Component: component, Version: version, Offered: offered})
	recorder := httptest.NewRecorder()
	panel.handleOfferedVersions(recorder, serviceStateAtomicRequest(http.MethodPut, offeredVersionsPath, string(body), adminID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("set offered: %d %s", recorder.Code, recorder.Body.String())
	}
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

func TestOfferedVersionsDefaultToEverythingAndAChangeIsTheAdministratorsAndAudited(t *testing.T) {
	panel, _, database, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "7.4")

	recorder := httptest.NewRecorder()
	panel.handleOfferedVersions(recorder, customerRequest(http.MethodGet, offeredVersionsPath, "", 77))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"php":[]`) || !strings.Contains(recorder.Body.String(), `"node":[]`) || !strings.Contains(recorder.Body.String(), `"restricted":true`) {
		t.Fatalf("default = %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	panel.handleOfferedVersions(recorder, customerRequest(http.MethodPut, offeredVersionsPath, `{"component":"php","version":"7.4","offered":false}`, 77))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer change = %d", recorder.Code)
	}

	setOffered(t, panel, adminID, "php", "7.4", false)
	setOffered(t, panel, adminID, "node", "20.11.1", false)
	recorder = httptest.NewRecorder()
	panel.handleOfferedVersions(recorder, customerRequest(http.MethodGet, offeredVersionsPath, "", 77))
	if !strings.Contains(recorder.Body.String(), `"php":["7.4"]`) || !strings.Contains(recorder.Body.String(), `"node":["20"]`) {
		t.Fatalf("hidden = %s", recorder.Body.String())
	}

	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs WHERE action LIKE 'versions.customer_offer:php%'`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "versions.customer_offer:php:7.4:hidden" || !strings.Contains(details, `"sites_on_version":1`) || !strings.Contains(details, `"sites_changed":0`) {
		t.Fatalf("audit = %q %s", action, details)
	}
	// The site on the hidden version was not touched.
	var version string
	database.GetDB().QueryRow(`SELECT php_version FROM sites WHERE id = 9203`).Scan(&version)
	if version != "7.4" {
		t.Fatalf("site moved to %q", version)
	}

	// The same answer again changes nothing and records nothing new.
	setOffered(t, panel, adminID, "php", "7.4", false)
	var rows int
	database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'versions.customer_offer:php%'`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("audit rows = %d", rows)
	}

	recorder = httptest.NewRecorder()
	panel.handleOfferedVersions(recorder, serviceStateAtomicRequest(http.MethodPut, offeredVersionsPath, `{"component":"mariadb","version":"11.8","offered":false}`, adminID))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown component = %d", recorder.Code)
	}
}

func TestACustomerChoosingAPHPVersionThatIsNotOfferedIsRefusedAndNothingChanges(t *testing.T) {
	panel, _, database, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "8.4")
	setOffered(t, panel, adminID, "php", "7.4", false)

	recorder := httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, customerRequest(http.MethodPost, "/api/v1/domains/9202/php", `{"php_version":"7.4","confirm_unsupported":true}`, 77))
	if recorder.Code != http.StatusConflict || errorCode(t, recorder) != errCodeVersionNotOffered {
		t.Fatalf("refusal = %d %s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if body.Vars["name"] != "PHP" || body.Vars["version"] != "7.4" || !strings.Contains(body.Error, "ask your administrator") {
		t.Fatalf("refusal body = %+v", body)
	}
	var version string
	database.GetDB().QueryRow(`SELECT php_version FROM sites WHERE id = 9203`).Scan(&version)
	if version != "8.4" {
		t.Fatalf("site changed to %q", version)
	}
}

func TestAnAdministratorIsNeverRestrictedAndASiteKeepsAHiddenVersion(t *testing.T) {
	panel, _, _, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "7.4")
	setOffered(t, panel, adminID, "php", "7.4", false)
	setOffered(t, panel, adminID, "php", "8.1", false)

	// The administrator's choice of a hidden version passes the offer check
	// (it then goes on to discovery, which this fake Agent does not answer).
	recorder := httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9202/php", `{"php_version":"8.1","confirm_unsupported":true}`, adminID))
	if errorCode(t, recorder) == errCodeVersionNotOffered {
		t.Fatalf("administrator refused: %s", recorder.Body.String())
	}
	// A customer whose site already runs the hidden 7.4 keeps it.
	recorder = httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, customerRequest(http.MethodPost, "/api/v1/domains/9202/php", `{"php_version":"7.4"}`, 77))
	if errorCode(t, recorder) == errCodeVersionNotOffered {
		t.Fatalf("keeping the current version refused: %s", recorder.Body.String())
	}
	// ...but cannot move to another hidden one.
	recorder = httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, customerRequest(http.MethodPost, "/api/v1/domains/9202/php", `{"php_version":"8.1"}`, 77))
	if errorCode(t, recorder) != errCodeVersionNotOffered {
		t.Fatalf("another hidden version: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestANodeSiteMayTakeAPatchOfItsHiddenLineButNotAnotherHiddenLine(t *testing.T) {
	panel, _, _, adminID := newServiceStateAtomicPanel(t)
	seedNodeSite(t, panel, adminID, nil) // runs 22.14.0
	setOffered(t, panel, adminID, "node", "22", false)
	setOffered(t, panel, adminID, "node", "20", false)
	restoreMove, restoreProve := transitionHostingForMove, proveAppAnswers
	t.Cleanup(func() { transitionHostingForMove, proveAppAnswers = restoreMove, restoreProve })
	var moves []string
	transitionHostingForMove = func(_ *Panel, _ context.Context, _ int, settings hostingSettings) error {
		moves = append(moves, settings.RuntimeVersion)
		return nil
	}
	proveAppAnswers = func(context.Context, int) error { return nil }

	recorder := httptest.NewRecorder()
	panel.handleDomainAppNodeVersion(recorder, customerRequest(http.MethodPost, "/api/v1/domains/9102/app/node-version", `{"version":"20.18.1"}`, 77), 9102)
	if recorder.Code != http.StatusConflict || errorCode(t, recorder) != errCodeVersionNotOffered || len(moves) != 0 {
		t.Fatalf("hidden line: %d %s moves=%v", recorder.Code, recorder.Body.String(), moves)
	}
	// A security patch of the line the site runs is never refused.
	recorder = httptest.NewRecorder()
	panel.handleDomainAppNodeVersion(recorder, customerRequest(http.MethodPost, "/api/v1/domains/9102/app/node-version", `{"version":"22.23.4"}`, 77), 9102)
	if recorder.Code != http.StatusOK || strings.Join(moves, ",") != "22.23.4" {
		t.Fatalf("patch of its line: %d %s moves=%v", recorder.Code, recorder.Body.String(), moves)
	}
}

func TestACustomersNewSiteTakesTheNewestOfferedPHPVersion(t *testing.T) {
	policy := offeredVersions{Hidden: map[string][]string{"php": {"8.5"}}}.normalized()
	installed := []string{"8.5", "8.4", "8.2"}
	if version, ok := customerSitePHPVersion("", installed, policy); !ok || version != "8.4" {
		t.Fatalf("default = %q %v", version, ok)
	}
	if _, ok := customerSitePHPVersion("8.5", installed, policy); ok {
		t.Fatal("a hidden version was accepted")
	}
	if version, ok := customerSitePHPVersion("8.2", installed, policy); !ok || version != "8.2" {
		t.Fatalf("offered = %q %v", version, ok)
	}
	none := offeredVersions{Hidden: map[string][]string{"php": {"8.5", "8.4", "8.2"}}}.normalized()
	if _, ok := customerSitePHPVersion("", installed, none); ok {
		t.Fatal("nothing offered still chose a version")
	}
	if got := offeredVersionLine("node", "v22.14.0"); got != "22" {
		t.Fatalf("node line = %q", got)
	}
	if got := offeredVersionLine("php", "8.4"); got != "8.4" {
		t.Fatalf("php line = %q", got)
	}
}
