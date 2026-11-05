package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// a124 lab: changing the PHP version of the setup-created static main site
// answered 500 `invalid PHP version ""`. A site whose hosting type runs no
// PHP has no version to change: a typed 409 names the type and sends the
// person to the site's Hosting page, and nothing is changed or asked of the
// Agent. A PHP site is not affected by this check.
func TestChangingPHPOnASiteWithoutPHPIsATypedRefusal(t *testing.T) {
	panel, _, database, adminID := newServiceStateAtomicPanel(t)
	seedPHPSite(t, panel, adminID, "8.4")
	for _, statement := range []string{
		`INSERT INTO domains (id, subscription_id, name, status) VALUES (9212, 9201, 'static.example', 'active')`,
		`INSERT INTO sites (id, domain_id, document_root, project_type, php_version, status)
		VALUES (9213, 9212, '/var/www/celikpanel/subscriptions/9201/sites/9212/public_html', 'static', '', 'active')`,
	} {
		if _, err := database.GetDB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	for _, request := range []*http.Request{
		serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9212/php", `{"php_version":"8.4"}`, adminID),
		customerRequest(http.MethodPost, "/api/v1/domains/9212/php", `{"php_version":"8.4"}`, 77),
	} {
		recorder := httptest.NewRecorder()
		panel.handleDomainPHPSettings(recorder, request)
		if recorder.Code != http.StatusConflict {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var body apiErrorBody
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != errCodeSiteHasNoPHP || body.Reason != "static" ||
			body.Action != "/domains/static.example?tab=hosting" || body.Vars["domain"] != "static.example" ||
			!strings.Contains(body.Error, "nothing was changed") || !strings.Contains(body.Error, "hosting type to PHP") {
			t.Fatalf("refusal = %+v", body)
		}
	}
	var version, projectType string
	if err := database.GetDB().QueryRow(`SELECT php_version, project_type FROM sites WHERE id = 9213`).Scan(&version, &projectType); err != nil {
		t.Fatal(err)
	}
	if version != "" || projectType != "static" {
		t.Fatalf("static site changed: php %q type %q", version, projectType)
	}

	// The PHP site goes on past this check (to discovery, which this fake
	// Agent does not answer).
	recorder := httptest.NewRecorder()
	panel.handleDomainPHPSettings(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9202/php", `{"php_version":"8.3"}`, adminID))
	if errorCode(t, recorder) == errCodeSiteHasNoPHP {
		t.Fatalf("a PHP site was refused as having no PHP: %s", recorder.Body.String())
	}
}
