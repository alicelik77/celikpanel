package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// alpha.130 lab: while the panel is still starting, closing remote database
// access stays possible; opening keeps PANEL_STARTING.

func dbrStartingCall(p *Panel, caller *Caller, method, path, body string) (int, string) {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	p.handleDatabaseRemoteWhileStarting(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func TestDatabaseRemoteClosesWhileThePanelStarts(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	admin := dbrCaller(dbrAdmin, roleAdmin)
	customer := dbrCaller(dbrCustomerA, roleCustomer)
	if _, err := p.db.GetDB().Exec(`INSERT INTO database_remote_rules (id, database_id, user_id, address) VALUES (91, 7530, 7540, '203.0.113.7/32'), (92, 7530, 7540, '203.0.113.8/32')`); err != nil {
		t.Fatal(err)
	}

	// Adding (opening) waits.
	if code, body := dbrStartingCall(p, customer, http.MethodPost, "/api/v1/database-remote/databases/7530/rules", `{"user_id": 7540, "address": "203.0.113.9"}`); code != http.StatusServiceUnavailable || !strings.Contains(body, "PANEL_STARTING") {
		t.Fatalf("add while starting: %d %s", code, body)
	}
	// A later end waits; a sooner one is saved.
	if code, _ := dbrStartingCall(p, customer, http.MethodPut, "/api/v1/database-remote/rules/92", `{"duration": "7d"}`); code != http.StatusOK {
		t.Fatalf("a sooner end than none: %d", code)
	}
	if code, body := dbrStartingCall(p, customer, http.MethodPut, "/api/v1/database-remote/rules/92", `{"duration": "permanent"}`); code != http.StatusServiceUnavailable || dbrRuleEnd(t, p, 92) == "" {
		t.Fatalf("a later end while starting: %d %s", code, body)
	}
	// Removing closes at the filter, without a full apply.
	if code, body := dbrStartingCall(p, customer, http.MethodDelete, "/api/v1/database-remote/rules/91", ""); code != http.StatusOK || !strings.Contains(body, `"starting":true`) {
		t.Fatalf("remove while starting: %d %s", code, body)
	}
	var left int
	_ = p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM database_remote_rules WHERE id = 91`).Scan(&left)
	applies, narrows := recorder.counts()
	if left != 0 || applies != 0 || narrows != 1 {
		t.Fatalf("remove: row left %d, applies %d, narrowings %d", left, applies, narrows)
	}
	// Another customer's rule stays not found.
	if code, _ := dbrStartingCall(p, dbrCaller(dbrCustomerB, roleCustomer), http.MethodDelete, "/api/v1/database-remote/rules/92", ""); code != http.StatusNotFound {
		t.Fatalf("another customer's rule: %d", code)
	}
	// The administrator: only turning the switch off.
	for _, body := range []string{`{"offered": true}`, `{"apply": true}`, `{"mariadb_skip_name_resolve": true}`, `{"offered": false, "apply": true}`} {
		if code, _ := dbrStartingCall(p, admin, http.MethodPut, "/api/v1/database-remote/server", body); code != http.StatusServiceUnavailable {
			t.Fatalf("%s while starting: %d", body, code)
		}
	}
	if code, body := dbrStartingCall(p, admin, http.MethodPut, "/api/v1/database-remote/server", `{"offered": false}`); code != http.StatusOK {
		t.Fatalf("turning off while starting: %d %s", code, body)
	}
	if offered, _ := p.databaseRemoteOffered(context.Background()); offered {
		t.Fatal("the switch is still on")
	}
	if applies, narrows := recorder.counts(); applies != 0 || narrows != 2 {
		t.Fatalf("turning off: applies %d, narrowings %d", applies, narrows)
	}
	// Reads are not part of this path.
	if code, _ := dbrStartingCall(p, admin, http.MethodGet, "/api/v1/database-remote/server", ""); code != http.StatusServiceUnavailable {
		t.Fatalf("a read through the start-up path: %d", code)
	}
}

// The start-up handler lets only the closing routes through; everything else
// keeps PANEL_STARTING.
func TestStartupHandlerLetsOnlyRemoteAccessClosingThrough(t *testing.T) {
	p, token := newAuthHandlerTestPanel(t, true)
	handler := p.startupRecoveryHandler(t.TempDir(), "", "")
	for _, c := range []struct {
		method, path string
		through      bool
	}{
		{http.MethodDelete, "/api/v1/database-remote/rules/" + strconv.Itoa(999), true},
		{http.MethodPut, "/api/v1/database-remote/rules/999", true},
		{http.MethodPut, "/api/v1/database-remote/server", true},
		{http.MethodPost, "/api/v1/database-remote/databases/1/rules", false},
		{http.MethodGet, "/api/v1/database-remote/databases/1", false},
		{http.MethodGet, "/api/v1/database-remote/server", false},
		{http.MethodDelete, "/api/v1/domains/1", false},
		{http.MethodPut, "/api/v1/database-remote/rules/abc", false},
	} {
		rr := requestWithToken(handler, c.method, c.path, token)
		if (rr.Code != http.StatusServiceUnavailable) != c.through {
			t.Errorf("%s %s: %d (through %v)", c.method, c.path, rr.Code, c.through)
		}
	}
}
