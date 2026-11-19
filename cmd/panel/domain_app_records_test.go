package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.138: the domain list says which domains hold a program or Python app
// record and which an AI app record, so the domain page can draw the card
// that removes what keeps the domain from being deleted, whatever the plan
// or the site type say. The fields are additive (absent when false); a team
// member, who is never shown those cards, never gets them.

type domainAppRecordsRow struct {
	ID           int  `json:"id"`
	HoldsProgram bool `json:"holds_program"`
	HoldsApp     bool `json:"holds_app"`
}

func listDomainAppRecords(t *testing.T, fixture authzMatrixFixture, caller *Caller) (map[int]domainAppRecordsRow, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := teamMemberAuthzTestRequest(http.MethodGet, "/api/v1/domains", caller)
	if caller.Role != core.EffectiveRoleAdditionalUser {
		request = requestAs(http.MethodGet, "/api/v1/domains", caller, "")
	}
	fixture.panel.handleDomains(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list as %s: %d %s", caller.Role, recorder.Code, recorder.Body.String())
	}
	var rows []domainAppRecordsRow
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	out := map[int]domainAppRecordsRow{}
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, recorder.Body.String()
}

func TestDomainListSaysWhichDomainsHoldAProgramOrAnApp(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	grantTeamMemberDomainCapability(t, &fixture, authzMatrixCustomerDomainID, core.TeamCapabilityFiles, core.TeamPermissionManage)
	db := fixture.panel.db.GetDB()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	program := func(on bool) {
		exec(`DELETE FROM runtime_apps WHERE domain_id = ?`, authzMatrixCustomerDomainID)
		if on {
			// Created only, never deployed: the site is still PHP.
			exec(`INSERT INTO runtime_apps (subscription_id, domain_id, site_id, kind, offering_id, entry, port, state, deployed)
				VALUES (?, ?, 1, 'program', 'runtime:program', 'server', 30010, 'created', 0)`, authzMatrixCustomerSubID, authzMatrixCustomerDomainID)
		}
	}
	app := func(on bool) {
		exec(`DELETE FROM service_app_installs WHERE domain_id = ?`, authzMatrixCustomerDomainID)
		if on {
			// A first install that failed: a record without a version.
			exec(`INSERT INTO service_app_installs (subscription_id, domain_id, site_id, app_id, version, target_version, port, aux_port, state)
				VALUES (?, ?, 1, 'open-webui', '', '0.6.0', 21010, 0, 'failed')`, authzMatrixCustomerSubID, authzMatrixCustomerDomainID)
		}
	}
	callers := map[string]*Caller{
		"administrator": {ID: authzMatrixAdminID, Role: roleAdmin},
		"reseller":      {ID: authzMatrixResellerID, Role: roleReseller},
		"customer":      {ID: authzMatrixCustomerID, Role: roleCustomer},
	}
	for _, state := range []struct {
		name         string
		program, app bool
	}{{"none", false, false}, {"program only", true, false}, {"app only", false, true}, {"both", true, true}} {
		program(state.program)
		app(state.app)
		for name, caller := range callers {
			rows, _ := listDomainAppRecords(t, fixture, caller)
			row, listed := rows[authzMatrixCustomerDomainID]
			if !listed {
				t.Fatalf("%s, %s: the domain is not listed", state.name, name)
			}
			if row.HoldsProgram != state.program || row.HoldsApp != state.app {
				t.Fatalf("%s, %s: holds_program=%v holds_app=%v", state.name, name, row.HoldsProgram, row.HoldsApp)
			}
			// Another domain of the list is not marked by this one.
			if other, ok := rows[authzMatrixOutsiderDomainID]; ok && (other.HoldsProgram || other.HoldsApp) {
				t.Fatalf("%s, %s: the outsider's domain is marked", state.name, name)
			}
		}
		// A team member sees the domain, never these facts.
		rows, body := listDomainAppRecords(t, fixture, teamMemberAuthzTestCaller())
		if _, listed := rows[authzMatrixCustomerDomainID]; !listed {
			t.Fatalf("%s: the team member's domain is not listed: %s", state.name, body)
		}
		if rows[authzMatrixCustomerDomainID].HoldsProgram || rows[authzMatrixCustomerDomainID].HoldsApp {
			t.Fatalf("%s: a team member was told what the domain holds: %s", state.name, body)
		}
	}
	// Absent when false: an older screen reads nothing new.
	program(false)
	app(false)
	_, body := listDomainAppRecords(t, fixture, callers["customer"])
	for _, field := range []string{"holds_program", "holds_app"} {
		if json.Valid([]byte(body)) && anyRowHasKey(body, field) {
			t.Fatalf("%s is written when false: %s", field, body)
		}
	}
}

func anyRowHasKey(body, key string) bool {
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return false
	}
	for _, row := range rows {
		if _, ok := row[key]; ok {
			return true
		}
	}
	return false
}
