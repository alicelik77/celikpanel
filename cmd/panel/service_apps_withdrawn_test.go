package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.141: Open WebUI was withdrawn from the catalogue (owner, 20 November
// 2026). Nobody can choose, offer, be given, install or update it, also from
// an older page; an install that exists keeps running and keeps its card with
// stop, start, log and removal.

const webuiWithdrawnInstallBody = `{"app":"open-webui","operation_id":"op-webui-0001","owner_email":"owner@outsider.matrix.test","owner_password":"Correct horse battery","confirm_replace":true}`

func TestOpenWebUIIsWithdrawnFromTheCatalogue(t *testing.T) {
	app, ok := serviceapps.Lookup("open-webui")
	if !ok || app.Withdrawn != "2026-11-20" || app.Listed() || app.Installable() {
		t.Fatalf("Open WebUI's entry: listed %v installable %v withdrawn %q", app.Listed(), app.Installable(), app.Withdrawn)
	}
	for _, other := range serviceapps.Catalog() {
		if other.ID != "open-webui" && other.Withdrawn != "" {
			t.Fatalf("%s is withdrawn too", other.ID)
		}
	}
	if n8n, _ := serviceapps.Lookup("n8n"); !n8n.Listed() || !n8n.Installable() {
		t.Fatal("n8n stays in the catalogue")
	}

	f := newServiceAppFixture(t)
	// An offer recorded before the withdrawal (with the summary it was made
	// against) no longer gives it.
	f.exec(t, `INSERT INTO service_app_offerings (app_id, offered, risk_digest, memory_mb, cpu_percent, tasks_max)
		VALUES ('open-webui', 1, '`+app.RiskDigest()+`', 3072, 100, 512)`)

	// Listed for nobody: Catalog management and the domain screens.
	rec := f.as(t, 9701, http.MethodGet, serviceAppsAdminPath, "")
	wantStatus(t, rec, http.StatusOK, "")
	if strings.Contains(rec.Body.String(), `"id":"open-webui"`) {
		t.Fatalf("Catalog management lists a withdrawn app: %s", rec.Body.String())
	}
	for _, caller := range []struct {
		user   int
		domain string
	}{{9701, "9903"}, {9704, "9903"}, {9703, "9902"}} {
		if _, shown := f.domainApps(t, caller.user, caller.domain)["open-webui"]; shown {
			t.Fatalf("user %d sees a withdrawn app on domain %s", caller.user, caller.domain)
		}
	}
	// Not given, not a rights candidate.
	layer, err := f.panel.loadAdminOfferLayer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if layer.offers("app:open-webui") {
		t.Fatal("an earlier offer still gives a withdrawn app")
	}
	for _, candidate := range f.panel.serviceAppCandidateOfferingsHere(context.Background()) {
		if candidate == "app:open-webui" {
			t.Fatal("a withdrawn app is a rights candidate")
		}
	}

	// An older page's offer and install are refused with the truth; nothing is recorded.
	rec = f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/open-webui/offer", `{"risk_digest":"`+app.RiskDigest()+`"}`)
	assertWithdrawnRefusal(t, rec)
	for _, caller := range []int{9704, 9701} {
		rec = f.as(t, caller, http.MethodPost, "/api/v1/domains/9903/service-apps/install", webuiWithdrawnInstallBody)
		assertWithdrawnRefusal(t, rec)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs`); n != 0 {
		t.Fatal("a refused install left a record")
	}
	f.native.mu.Lock()
	prepares := len(f.native.prepares)
	f.native.mu.Unlock()
	if prepares != 0 {
		t.Fatal("a withdrawn app reached the server")
	}
}

func assertWithdrawnRefusal(t *testing.T, rec interface {
	Result() *http.Response
}) {
	t.Helper()
	res := rec.Result()
	var body apiErrorBody
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusConflict || body.Code != errCodeServiceApp || body.Reason != serviceapps.ReasonWithdrawn ||
		body.Vars["app"] != "Open WebUI" || !strings.Contains(body.Error, "keeps running") {
		t.Fatalf("refusal: %d %+v", res.StatusCode, body)
	}
}

// Whatever exists stays: the install keeps its card, stop and start, its
// log and its removal; it gets no update.
func TestAnExistingOpenWebUIInstallStaysManageableAndRemovable(t *testing.T) {
	f := newServiceAppFixture(t)
	f.exec(t, `INSERT INTO service_app_installs (id, subscription_id, domain_id, site_id, app_id, version, target_version, port, aux_port, state, previous_project_type)
		VALUES (91, 9803, 9903, 9941, 'open-webui', '0.12.4', '0.12.4', 21100, 0, 'running', 'php')`)
	f.exec(t, `UPDATE sites SET project_type = 'service_app' WHERE id = 9941`)
	f.native.mu.Lock()
	f.native.installed[91] = true
	f.native.units[91] = transport.AppUnitState{Present: true, Active: true, Enabled: true}
	f.native.mu.Unlock()
	path := "/api/v1/domains/9903/service-apps"

	// The card is there, with the entry marked withdrawn and no update.
	rec := f.as(t, 9704, http.MethodGet, path, "")
	wantStatus(t, rec, http.StatusOK, "")
	var view serviceAppsDomainView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Install == nil || view.Install.AppID != "open-webui" || view.Install.Name != "Open WebUI" || view.Install.UpdateAvailable != "" {
		t.Fatalf("install view: %+v", view.Install)
	}
	entry := map[string]serviceAppAppView{}
	for _, app := range view.Apps {
		entry[app.ID] = app
	}
	if webui, ok := entry["open-webui"]; !ok || webui.Withdrawn != "2026-11-20" || webui.Offered || webui.Reason != serviceapps.ReasonWithdrawn {
		t.Fatalf("the domain's own app entry: %+v", entry["open-webui"])
	}

	// No update, with the truth.
	assertWithdrawnRefusal(t, f.as(t, 9704, http.MethodPost, path+"/update", `{}`))

	// Stop, start and the log work as before.
	for _, action := range []string{"stop", "start"} {
		wantStatus(t, f.as(t, 9704, http.MethodPost, path+"/control", `{"action":"`+action+`"}`), http.StatusOK, "")
	}
	wantStatus(t, f.as(t, 9704, http.MethodGet, path+"/logs", ""), http.StatusOK, "")
	f.native.mu.Lock()
	controls := strings.Join(f.native.controls, ",")
	f.native.mu.Unlock()
	if controls != "stop,start" {
		t.Fatalf("controls: %s", controls)
	}

	// An install or update queued before the withdrawal ends with the reason,
	// and the app keeps running.
	f.exec(t, `INSERT INTO service_app_operations (id, install_id, kind, step) VALUES ('op-webui-queued', 91, 'update', 'prepare')`)
	if err := f.panel.runServiceAppOperation(context.Background(), "op-webui-queued", &serviceAppSecretInput{}); err != nil {
		t.Fatal(err)
	}
	op, err := f.panel.serviceAppOperationByID(context.Background(), "op-webui-queued")
	if err != nil || op.State != "failed" || op.Reason != serviceapps.ReasonWithdrawn {
		t.Fatalf("queued update: %+v %v", op, err)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs WHERE id = 91 AND state = 'running' AND version = '0.12.4'`); n != 1 {
		t.Fatal("the running install was changed by a refused update")
	}

	// Removal works as for any app.
	rec = f.as(t, 9704, http.MethodPost, path+"/uninstall", `{"operation_id":"op-webui-remove"}`)
	wantStatus(t, rec, http.StatusAccepted, "")
	if done := f.waitOperation(t, "op-webui-remove"); done.State != "succeeded" {
		t.Fatalf("removal: %+v", done)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs WHERE id = 91`); n != 0 {
		t.Fatal("the removed install is still recorded")
	}
	f.native.mu.Lock()
	removed := len(f.native.removed)
	f.native.mu.Unlock()
	if removed != 1 {
		t.Fatalf("removal calls: %d", removed)
	}
}
