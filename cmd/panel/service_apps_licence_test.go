package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/serviceapps"
)

// n8n is closed to resellers and their customers by its licence (alpha.118,
// the owner's decision of 3 Nov 2026): the administrator may offer it to the
// administrator's own customers; a reseller's set, a reseller plan, a plan a
// reseller owns and a subscription in a reseller's tree cannot name it, and
// nothing in a reseller's tree installs or updates it. Open WebUI is not
// affected. Stop, logs and removal stay open.
func TestN8nIsClosedToResellersByItsLicence(t *testing.T) {
	f := newServiceAppFixture(t)
	f.offer(t, "n8n")
	ctx := context.Background()
	db := f.panel.db.GetDB()

	app, closed := serviceapps.ResellerClosedOffering("app:n8n")
	if !closed || app.ID != "n8n" {
		t.Fatal("n8n must be closed to resellers")
	}
	if _, closed := serviceapps.ResellerClosedOffering("app:open-webui"); closed {
		t.Fatal("Open WebUI is not closed to resellers")
	}

	// The reseller's customer sees why, and the administrator is named.
	rec := f.as(t, 9703, http.MethodGet, "/api/v1/domains/9902/service-apps", "")
	wantStatus(t, rec, http.StatusOK, "")
	var view serviceAppsDomainView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Apps {
		if item.ID == "n8n" && (item.Offered || item.RefusedBy != rightsLayerLicence) {
			t.Fatalf("n8n for a reseller's customer: %+v", item)
		}
	}

	// Install is refused for the customer and for the administrator acting
	// for that account alike, with the licence reason.
	body := strings.Replace(n8nInstallBody, "op-n8n-0001", "op-n8n-lic1", 1)
	for _, caller := range []int{9703, 9701} {
		rec = f.as(t, caller, http.MethodPost, "/api/v1/domains/9902/service-apps/install", body)
		wantStatus(t, rec, http.StatusConflict, errCodeServiceApp)
		if !strings.Contains(rec.Body.String(), `"reason":"`+serviceAppRefusalLicenceReseller+`"`) {
			t.Fatalf("caller %d: %s", caller, rec.Body.String())
		}
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs WHERE domain_id = 9902`); n != 0 {
		t.Fatal("a refused install left a record")
	}

	// Update of an install made before is refused too; stop and removal are not.
	f.exec(t, `INSERT INTO service_app_installs (id, subscription_id, domain_id, site_id, app_id, version, target_version, port, aux_port, state)
		VALUES (78, 9802, 9902, 9940, 'n8n', '2.40.0', '2.40.0', 21002, 21003, 'running')`)
	rec = f.as(t, 9703, http.MethodPost, "/api/v1/domains/9902/service-apps/update", `{}`)
	wantStatus(t, rec, http.StatusConflict, errCodeServiceApp)
	if !strings.Contains(rec.Body.String(), serviceAppRefusalLicenceReseller) {
		t.Fatalf("update: %s", rec.Body.String())
	}
	rec = f.as(t, 9703, http.MethodPost, "/api/v1/domains/9902/service-apps/control", `{"action":"stop"}`)
	if strings.Contains(rec.Body.String(), serviceAppRefusalLicenceReseller) {
		t.Fatalf("stop must never be refused for the licence: %s", rec.Body.String())
	}
	rec = f.as(t, 9703, http.MethodPost, "/api/v1/domains/9902/service-apps/uninstall", `{"operation_id":"op-n8n-lic-remove"}`)
	if strings.Contains(rec.Body.String(), serviceAppRefusalLicenceReseller) {
		t.Fatalf("removal must never be refused for the licence: %s", rec.Body.String())
	}

	// Rights: a reseller's set, a reseller plan and a subscription in a
	// reseller's tree cannot name n8n; the administrator's own customer can.
	withN8n := newRightsSet(true, false, "app:n8n")
	for _, c := range []struct {
		kind string
		id   int
	}{{rightsOwnerReseller, 9702}, {rightsOwnerResellerPlan, 1}, {rightsOwnerSubscription, 9802}} {
		err := saveRightsSet(ctx, db, c.kind, c.id, withN8n)
		var licence *serviceAppLicenceClosedError
		if !errors.As(err, &licence) {
			t.Fatalf("%s %d: %v", c.kind, c.id, err)
		}
	}
	if err := saveRightsSet(ctx, db, rightsOwnerSubscription, 9803, withN8n); err != nil {
		t.Fatalf("the administrator's own customer: %v", err)
	}
	// Open WebUI stays open to resellers.
	if err := saveRightsSet(ctx, db, rightsOwnerReseller, 9702, newRightsSet(true, false, "app:open-webui")); err != nil {
		t.Fatalf("Open WebUI for a reseller: %v", err)
	}
	// "Everything offered" names nothing; taking an item out of it never
	// names n8n for a reseller.
	if err := saveRightsSet(ctx, db, rightsOwnerReseller, 9702, newRightsSet(true, true)); err != nil {
		t.Fatal(err)
	}
	if got := withoutLicenceClosed([]string{"app:n8n", "app:open-webui", "component:mariadb"}); strings.Join(got, ",") != "app:open-webui,component:mariadb" {
		t.Fatalf("candidates: %v", got)
	}

	// The refusal reaches any screen as a coded 409, never a server error.
	recorder := httptest.NewRecorder()
	writeServerError(recorder, &serviceAppLicenceClosedError{App: app})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), serviceAppRefusalLicenceReseller) {
		t.Fatalf("mapped: %d %s", recorder.Code, recorder.Body.String())
	}
}

// A plan a reseller owns cannot name n8n; the administrator's plan can.
func TestN8nInPlansFollowsThePlanOwner(t *testing.T) {
	f := newServiceAppFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	f.exec(t, `INSERT INTO service_plans (id, owner_id, name) VALUES (7101, 9702, 'reseller plan'), (7102, 9701, 'admin plan')`)
	withN8n := newRightsSet(true, false, "app:n8n")
	var licence *serviceAppLicenceClosedError
	if err := saveRightsSet(ctx, db, rightsOwnerPlan, 7101, withN8n); !errors.As(err, &licence) {
		t.Fatalf("a reseller's plan: %v", err)
	}
	if err := saveRightsSet(ctx, db, rightsOwnerPlan, 7102, withN8n); err != nil {
		t.Fatalf("the administrator's plan: %v", err)
	}
}
