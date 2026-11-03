package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// The read-only facts the screens draw menus and tabs from (alpha.117,
// D-042 P6): a domain lists the closed permissions of its subscription, and
// /auth/me narrows a customer's navigation by its rights. Nothing here
// changes a right; the gates stay where they are.

func TestRightsFactsClosedPermissionsFollowTheSubscription(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(rightDNSEdit, rightDatabases)...))
	closed, err := f.panel.closedPermissions(ctx, 9802)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{rightDNSEdit, rightDatabases} {
		if !slices.Contains(closed, id) {
			t.Fatalf("closed %v: %s must be closed", closed, id)
		}
	}
	if slices.Contains(closed, rightMail) || slices.Contains(closed, rightCron) {
		t.Fatalf("closed %v: mail and scheduled tasks stay open", closed)
	}
	// The administrator's own workspace is never narrowed.
	var adminSub int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM subscriptions WHERE owner_id = 9701 ORDER BY id LIMIT 1`).Scan(&adminSub); err == nil {
		if _, err := f.panel.db.GetDB().Exec(`UPDATE subscriptions SET name = 'Admin Subscription', plan_id = NULL WHERE id = ?`, adminSub); err != nil {
			t.Fatal(err)
		}
		if closed, err := f.panel.closedPermissions(ctx, adminSub); err != nil || len(closed) != 0 {
			t.Fatalf("admin workspace closed %v, %v; want none", closed, err)
		}
	}
}

func TestRightsFactsDomainListCarriesClosedRights(t *testing.T) {
	f := newRightsFixture(t)
	f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(rightMail)...))
	list := func(caller *Caller) map[int][]string {
		t.Helper()
		recorder := httptest.NewRecorder()
		f.panel.handleDomains(recorder, requestAs(http.MethodGet, "/api/v1/domains", caller, ""))
		if recorder.Code != http.StatusOK {
			t.Fatalf("list: %d %s", recorder.Code, recorder.Body.String())
		}
		var rows []struct {
			ID           int      `json:"id"`
			RightsClosed []string `json:"rights_closed"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		out := map[int][]string{}
		for _, row := range rows {
			out[row.ID] = row.RightsClosed
		}
		return out
	}
	customer := list(&Caller{ID: authzMatrixCustomerID, Role: roleCustomer})
	closed, listed := customer[9902]
	if !listed {
		t.Fatalf("the customer's domain 9902 is not listed: %v", customer)
	}
	if !slices.Contains(closed, rightMail) {
		t.Fatalf("domain 9902 rights_closed %v: mail must be closed", closed)
	}
	if slices.Contains(closed, rightCron) {
		t.Fatalf("domain 9902 rights_closed %v: scheduled tasks stay open", closed)
	}
	// The administrator acting as itself is never narrowed: nothing is listed.
	admin := list(&Caller{ID: authzMatrixAdminID, Role: roleAdmin})
	if got := admin[9902]; len(got) != 0 {
		t.Fatalf("administrator sees rights_closed %v; want none", got)
	}
}

func TestRightsFactsNavigationFollowsTheCustomersRights(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer}

	features := map[string]bool{"team_members": true, "vpn": true}
	f.panel.narrowFeaturesByRights(ctx, customer, features)
	if !features["databases"] || !features["team_members"] {
		t.Fatalf("everything offered: databases and team members stay open, got %v", features)
	}

	f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(rightDatabases, rightTeamMembers)...))
	features = map[string]bool{"team_members": true, "vpn": false}
	f.panel.narrowFeaturesByRights(ctx, customer, features)
	if features["databases"] || features["team_members"] {
		t.Fatalf("closed databases and team members must not be drawn, got %v", features)
	}
	if features["vpn"] {
		t.Fatalf("a feature that was off stays off, got %v", features)
	}

	// The administrator and a reseller keep their pages.
	for _, caller := range []*Caller{{ID: authzMatrixAdminID, Role: roleAdmin}, {ID: 9702, Role: roleReseller}} {
		features := map[string]bool{}
		f.panel.narrowFeaturesByRights(ctx, caller, features)
		if !features["databases"] {
			t.Fatalf("%s: the Databases page stays, got %v", caller.Role, features)
		}
	}

	// /auth/me says it.
	recorder := f.send(t, http.MethodGet, "/api/v1/auth/me", f.tokens[authzMatrixCustomerID], "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("/auth/me: %d %s", recorder.Code, recorder.Body.String())
	}
	var me struct {
		Features map[string]bool `json:"features"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if databases, present := me.Features["databases"]; !present || databases {
		t.Fatalf("/auth/me features %v: databases must be present and false", me.Features)
	}
	if me.Features["team_members"] {
		t.Fatalf("/auth/me features %v: team members closed", me.Features)
	}
}
