package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Service apps (alpha.118). The fixture is the authz matrix: reseller 9702's
// customer 9703 owns subscription 9802 and domain 9902
// (customer.matrix.test); the administrator's own customer 9704 owns
// subscription 9803 and domain 9903 (outsider.matrix.test). n8n is closed to
// resellers and their customers by its licence, so its installs run on 9903. The server side is a fake that behaves like the
// Agent: prepare starts an install that succeeds, activate starts a unit
// that serves on loopback (or, when told, exposes itself).

type fakeServiceAppNative struct {
	mu        sync.Mutex
	probeApps []transport.ServiceAppAvailability
	prepares  []transport.ServiceAppPrepareRequest
	activates int
	controls  []string
	renders   []int
	removed   []transport.ServiceAppRemoveRequest
	installed map[int]bool
	units     map[int]transport.AppUnitState
	exposed   bool
	// proof is the sandbox proof a running unit returns (nil: a passed one);
	// generations the sandbox generation of each unit file (default current).
	proof        *transport.SandboxProof
	generations  map[int]int
	sandboxReads int
}

func newFakeServiceAppNative() *fakeServiceAppNative {
	f := &fakeServiceAppNative{installed: map[int]bool{}, units: map[int]transport.AppUnitState{}}
	for _, app := range serviceapps.Catalog() {
		f.probeApps = append(f.probeApps, transport.ServiceAppAvailability{AppID: app.ID, Available: app.Installable()})
	}
	return f
}

func (f *fakeServiceAppNative) probe(context.Context) (transport.ServiceAppProbeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return transport.ServiceAppProbeResponse{Apps: append([]transport.ServiceAppAvailability(nil), f.probeApps...)}, nil
}

func (f *fakeServiceAppNative) prepare(_ context.Context, req transport.ServiceAppPrepareRequest) (transport.ServiceAppStepResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prepares = append(f.prepares, req)
	f.installed[req.Target.InstallID] = true
	return transport.ServiceAppStepResponse{Done: true}, nil
}

func (f *fakeServiceAppNative) status(_ context.Context, req transport.ServiceAppStatusRequest) (transport.ServiceAppStatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	unit := f.units[req.Target.InstallID]
	resp := transport.ServiceAppStatusResponse{Install: "none", Installed: f.installed[req.Target.InstallID], Unit: unit, UnitFile: "ours"}
	resp.SandboxGeneration = sitesandbox.Generation
	if g, ok := f.generations[req.Target.InstallID]; ok {
		resp.SandboxGeneration = g
	}
	if unit.Active {
		resp.ActiveState = "active"
		if f.exposed {
			resp.Exposed = []string{"0.0.0.0:5678"}
		} else {
			resp.Serving = true
		}
		if req.Proof {
			resp.SandboxProof = &transport.SandboxProof{OK: true, Code: "ok"}
			if f.proof != nil {
				resp.SandboxProof = f.proof
			}
		}
	}
	return resp, nil
}

func (f *fakeServiceAppNative) activate(_ context.Context, req transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activates++
	if req.KeepStopped {
		f.units[req.Target.InstallID] = transport.AppUnitState{Present: true}
	} else {
		f.units[req.Target.InstallID] = transport.AppUnitState{Present: true, Active: true, Enabled: true}
	}
	delete(f.generations, req.Target.InstallID)
	return transport.ServiceAppStepResponse{Done: true}, nil
}

func (f *fakeServiceAppNative) finalize(context.Context, transport.ServiceAppActivateRequest) (transport.ServiceAppStepResponse, error) {
	return transport.ServiceAppStepResponse{Done: true}, nil
}

func (f *fakeServiceAppNative) control(_ context.Context, req transport.ServiceAppControlRequest) (transport.ServiceAppStepResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.controls = append(f.controls, req.Action)
	unit := f.units[req.Target.InstallID]
	unit.Active, unit.Enabled = req.Action != "stop", req.Action != "stop"
	f.units[req.Target.InstallID] = unit
	return transport.ServiceAppStepResponse{Done: true}, nil
}

func (f *fakeServiceAppNative) logs(context.Context, transport.ServiceAppLogsRequest) (transport.ServiceAppLogsResponse, error) {
	return transport.ServiceAppLogsResponse{Lines: []string{"started"}}, nil
}

func (f *fakeServiceAppNative) secrets(context.Context, transport.ServiceAppSecretsRequest) (transport.ServiceAppStepResponse, error) {
	return transport.ServiceAppStepResponse{Done: true}, nil
}

func (f *fakeServiceAppNative) remove(_ context.Context, req transport.ServiceAppRemoveRequest) (transport.ServiceAppRemoveResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, req)
	delete(f.units, req.Target.InstallID)
	return transport.ServiceAppRemoveResponse{ServiceAppStepResponse: transport.ServiceAppStepResponse{Done: true}}, nil
}

func (f *fakeServiceAppNative) unitState(_ context.Context, id int) (transport.ServiceAppUnitStateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return transport.ServiceAppUnitStateResponse{State: f.units[id]}, nil
}

func (f *fakeServiceAppNative) setSuspension(_ context.Context, req transport.SetServiceAppSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var resp transport.SetAppUnitSuspensionResponse
	current := f.units[req.InstallID]
	target := transport.AppUnitState{Present: true}
	from := req.Before
	if !req.Suspend {
		target, from = req.Before, transport.AppUnitState{Present: true}
	}
	applied, change, conflict := casDecide(current, target, from)
	resp.Applied, resp.Changed, resp.Conflict = applied, change, conflict
	if change {
		f.units[req.InstallID] = target
	}
	resp.State = f.units[req.InstallID]
	return resp, nil
}

func (f *fakeServiceAppNative) sandboxCaps(context.Context) (transport.SandboxCapsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sandboxReads++
	resp := transport.SandboxCapsResponse{Level: "full", Gaps: []string{}, Generation: sitesandbox.Generation, SMTPRejects: 0}
	for id := range f.units {
		generation := sitesandbox.Generation
		if g, ok := f.generations[id]; ok {
			generation = g
		}
		resp.Units = append(resp.Units, transport.SandboxUnit{InstallID: id, Unit: serviceapps.UnitName(id) + ".service", File: "ours", Generation: generation})
	}
	return resp, nil
}

func (f *fakeServiceAppNative) renderSite(_ context.Context, domainID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renders = append(f.renders, domainID)
	return nil
}

type serviceAppFixture struct {
	accountsFixture
	native *fakeServiceAppNative
}

func newServiceAppFixture(t *testing.T) serviceAppFixture {
	t.Helper()
	base := newAccountsFixture(t)
	native := newFakeServiceAppNative()
	serviceAppNativeOverride = native
	resetServiceAppProbeCache()
	previousPoll := serviceAppPollInterval
	serviceAppPollInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		serviceAppNativeOverride = nil
		serviceAppPollInterval = previousPoll
		resetServiceAppProbeCache()
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/domains/", base.panel.handleDomainSubroute)
	mux.HandleFunc(serviceAppsAdminPath, base.panel.handleServiceAppsAdmin)
	mux.HandleFunc(serviceAppsAdminPath+"/", base.panel.handleServiceAppsAdmin)
	base.handler = base.panel.requireAuth(mux)
	f := serviceAppFixture{accountsFixture: base, native: native}
	f.exec(t, `INSERT INTO sites (id, domain_id, document_root, project_type, ssl_enabled, ssl_cert_path, ssl_key_path)
		VALUES (9940, 9902, '/var/www/celikpanel/subscriptions/9802/sites/9902/public_html', 'php', 1, '/etc/ssl/c.pem', '/etc/ssl/c.key')`)
	f.exec(t, `INSERT INTO sites (id, domain_id, document_root, project_type, ssl_enabled, ssl_cert_path, ssl_key_path)
		VALUES (9941, 9903, '/var/www/celikpanel/subscriptions/9803/sites/9903/public_html', 'php', 1, '/etc/ssl/o.pem', '/etc/ssl/o.key')`)
	return f
}

func (f serviceAppFixture) offer(t *testing.T, appID string) {
	t.Helper()
	app, _ := serviceapps.Lookup(appID)
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/"+appID+"/offer", `{"risk_digest":"`+app.RiskDigest()+`"}`), http.StatusOK, "")
}

func (f serviceAppFixture) waitOperation(t *testing.T, id string) serviceAppOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		op, err := f.panel.serviceAppOperationByID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.State != "running" {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation still running at step %s", op.Step)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const n8nInstallBody = `{"app":"n8n","operation_id":"op-n8n-0001","owner_email":"owner@outsider.matrix.test","owner_password":"correct horse battery","confirm_replace":true}`

const webuiInstallBody = `{"app":"open-webui","operation_id":"op-webui-0001","owner_email":"owner@customer.matrix.test","owner_password":"correct horse battery","confirm_replace":true}`

func TestServiceAppIsClosedUntilTheAdministratorOffersIt(t *testing.T) {
	f := newServiceAppFixture(t)
	path := "/api/v1/domains/9902/service-apps"

	// Installed is not offered: closed by default, for the customer and the
	// administrator alike.
	rec := f.as(t, 9703, http.MethodGet, path, "")
	wantStatus(t, rec, http.StatusOK, "")
	var view serviceAppsDomainView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	for _, app := range view.Apps {
		if app.Offered || (app.ID == "n8n" && app.RefusedBy != rightsLayerLicence) || (app.ID == "open-webui" && app.RefusedBy != rightsLayerAdministrator) {
			t.Fatalf("an app is open before it was offered: %+v", app)
		}
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, path+"/install", webuiInstallBody), http.StatusConflict, errCodeNotOffered)
	wantStatus(t, f.as(t, 9701, http.MethodPost, path+"/install", webuiInstallBody), http.StatusConflict, errCodeNotOffered)

	// The administrator offers only with the current risk summary.
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/n8n/offer", `{"risk_digest":"stale"}`), http.StatusConflict, errCodeServiceApp)
	wantStatus(t, f.as(t, 9703, http.MethodGet, serviceAppsAdminPath, ""), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/openclaw/offer", `{"risk_digest":"x"}`), http.StatusConflict, errCodeServiceApp)
	f.offer(t, "open-webui")

	// The reseller narrows: without app:open-webui in the reseller's set the
	// customer is refused, and the reseller is named as the layer.
	reseller := newRightsSet(true, false, "component:php-fpm:8.3")
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}
	rec = f.as(t, 9703, http.MethodPost, path+"/install", webuiInstallBody)
	wantStatus(t, rec, http.StatusConflict, errCodeNotOffered)
	if !strings.Contains(rec.Body.String(), `"reason":"reseller"`) {
		t.Fatalf("the reseller layer must be named: %s", rec.Body.String())
	}
	reseller.Items["app:open-webui"] = true
	if err := saveRightsSet(context.Background(), f.panel.db.GetDB(), rightsOwnerReseller, 9702, reseller); err != nil {
		t.Fatal(err)
	}

	// A security hold refuses new installs with its own reason.
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/open-webui/hold", `{"reason":"CVE-2026-0001"}`), http.StatusOK, "")
	rec = f.as(t, 9703, http.MethodPost, path+"/install", webuiInstallBody)
	wantStatus(t, rec, http.StatusConflict, errCodeServiceApp)
	if !strings.Contains(rec.Body.String(), `"reason":"held"`) {
		t.Fatalf("held: %s", rec.Body.String())
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, serviceAppsAdminPath+"/open-webui/release", `{}`), http.StatusOK, "")
	rec = f.as(t, 9703, http.MethodPost, path+"/install", webuiInstallBody)
	wantStatus(t, rec, http.StatusAccepted, "")
	f.waitOperation(t, "op-webui-0001")
}

func TestServiceAppInstallIsOneOperationAndKeepsNoSecret(t *testing.T) {
	f := newServiceAppFixture(t)
	f.offer(t, "n8n")
	path := "/api/v1/domains/9903/service-apps/install"

	// Without confirming that the domain changes, nothing starts.
	wantStatus(t, f.as(t, 9704, http.MethodPost, path, strings.Replace(n8nInstallBody, `"confirm_replace":true`, `"confirm_replace":false`, 1)), http.StatusBadRequest, errCodeServiceApp)

	wantStatus(t, f.as(t, 9704, http.MethodPost, path, n8nInstallBody), http.StatusAccepted, "")
	// The same request again (a lost answer) answers the same operation.
	rec := f.as(t, 9704, http.MethodPost, path, n8nInstallBody)
	wantStatus(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"id":"op-n8n-0001"`) {
		t.Fatalf("repeat: %s", rec.Body.String())
	}
	op := f.waitOperation(t, "op-n8n-0001")
	if op.State != "succeeded" {
		t.Fatalf("operation: %+v", op)
	}
	f.native.mu.Lock()
	prepares, activates := len(f.native.prepares), f.native.activates
	var prepared transport.ServiceAppPrepareRequest
	if prepares > 0 {
		prepared = f.native.prepares[0]
	}
	f.native.mu.Unlock()
	if prepares != 1 || activates != 1 {
		t.Fatalf("the install ran %d prepares and %d activations", prepares, activates)
	}
	// n8n receives a bcrypt hash of the owner's password, never the password.
	if !strings.HasPrefix(prepared.OwnerSecret, "$2a$") || strings.Contains(prepared.OwnerSecret, "correct horse") {
		t.Fatalf("owner secret: %q", prepared.OwnerSecret)
	}
	if prepared.Port < serviceapps.PortMin || prepared.AuxPort != prepared.Port+1 {
		t.Fatalf("ports: %d %d", prepared.Port, prepared.AuxPort)
	}
	var projectType string
	var port, forceHTTPS int
	if err := f.panel.db.GetDB().QueryRow(`SELECT project_type, app_port, force_https FROM sites WHERE id = 9941`).Scan(&projectType, &port, &forceHTTPS); err != nil {
		t.Fatal(err)
	}
	if projectType != "service_app" || port != prepared.Port || forceHTTPS != 1 {
		t.Fatalf("site: %s %d %d", projectType, port, forceHTTPS)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs WHERE domain_id = 9903 AND state = 'running' AND version = '2.46.4'`); n != 1 {
		t.Fatalf("install row: %d", n)
	}
	// No secret is written to the Panel's database.
	for _, query := range []string{`SELECT COALESCE(GROUP_CONCAT(message || reason || step), '') FROM service_app_operations`,
		`SELECT COALESCE(GROUP_CONCAT(COALESCE(details, '')), '') FROM audit_logs`} {
		var text string
		if err := f.panel.db.GetDB().QueryRow(query).Scan(&text); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "correct horse") || strings.Contains(text, "$2a$") {
			t.Fatalf("a secret reached the database: %s", text)
		}
	}
	// A second app on the same domain is refused.
	wantStatus(t, f.as(t, 9704, http.MethodPost, path, `{"app":"n8n","owner_email":"a@b.cd","owner_password":"another long password","confirm_replace":true}`), http.StatusConflict, errCodeServiceApp)

	// Removal puts the site back as it was and keeps the data unless asked.
	rec = f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/uninstall", `{"operation_id":"op-n8n-remove","remove_data":true}`)
	wantStatus(t, rec, http.StatusBadRequest, errCodeServiceApp)
	wantStatus(t, f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/uninstall", `{"operation_id":"op-n8n-remove"}`), http.StatusAccepted, "")
	if op := f.waitOperation(t, "op-n8n-remove"); op.State != "succeeded" {
		t.Fatalf("uninstall: %+v", op)
	}
	if err := f.panel.db.GetDB().QueryRow(`SELECT project_type FROM sites WHERE id = 9941`).Scan(&projectType); err != nil || projectType != "php" {
		t.Fatalf("site type after removal: %s %v", projectType, err)
	}
	f.native.mu.Lock()
	removedData := f.native.removed[0].RemoveData
	f.native.mu.Unlock()
	if removedData {
		t.Fatal("the data folder must be kept unless removal of data was confirmed")
	}
}

func TestServiceAppRefusedWhereTheOSCannotRunIt(t *testing.T) {
	f := newServiceAppFixture(t)
	f.native.mu.Lock()
	for i := range f.native.probeApps {
		if f.native.probeApps[i].AppID == "open-webui" {
			f.native.probeApps[i] = transport.ServiceAppAvailability{AppID: "open-webui", Reason: serviceapps.ReasonOSUnsupported, Detail: "needs Python 3.11 or 3.12; this system has Python 3.13"}
		}
	}
	f.native.mu.Unlock()
	f.offer(t, "open-webui")
	rec := f.as(t, 9703, http.MethodPost, "/api/v1/domains/9902/service-apps/install",
		`{"app":"open-webui","owner_email":"owner@customer.matrix.test","owner_password":"correct horse battery","confirm_replace":true}`)
	wantStatus(t, rec, http.StatusConflict, errCodeServiceApp)
	if !strings.Contains(rec.Body.String(), `"reason":"os_unsupported"`) || !strings.Contains(rec.Body.String(), "Python 3.13") {
		t.Fatalf("refusal: %s", rec.Body.String())
	}
	if n := f.count(t, `SELECT COUNT(*) FROM service_app_installs`); n != 0 {
		t.Fatal("a refused install left a record")
	}
	// A site without a certificate is an unmet prerequisite.
	f.exec(t, `UPDATE sites SET ssl_enabled = 0 WHERE id = 9941`)
	f.offer(t, "n8n")
	rec = f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/install", n8nInstallBody)
	if !strings.Contains(rec.Body.String(), `"reason":"certificate_missing"`) {
		t.Fatalf("certificate: %s", rec.Body.String())
	}
}

func TestServiceAppThatExposesItselfIsStopped(t *testing.T) {
	f := newServiceAppFixture(t)
	f.native.exposed = true
	f.offer(t, "n8n")
	wantStatus(t, f.as(t, 9704, http.MethodPost, "/api/v1/domains/9903/service-apps/install", n8nInstallBody), http.StatusAccepted, "")
	op := f.waitOperation(t, "op-n8n-0001")
	if op.State != "failed" || op.Class != transport.ServiceAppVerifiedFailure || op.Reason != "exposed" {
		t.Fatalf("operation: %+v", op)
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.controls) != 1 || f.native.controls[0] != "stop" {
		t.Fatalf("the exposed app must be stopped: %v", f.native.controls)
	}
	var projectType string
	if err := f.panel.db.GetDB().QueryRow(`SELECT project_type FROM sites WHERE id = 9941`).Scan(&projectType); err != nil || projectType != "php" {
		t.Fatalf("an exposed app must not be published: %s", projectType)
	}
}

func TestSuspensionStopsServiceAppsAndRestoresThem(t *testing.T) {
	f := newSuspensionFixture(t)
	native := newFakeServiceAppNative()
	serviceAppNativeOverride = native
	t.Cleanup(func() { serviceAppNativeOverride = nil })
	f.exec(t, `INSERT INTO service_app_installs (id, subscription_id, domain_id, site_id, app_id, version, target_version, port, aux_port, state)
		VALUES (77, 9802, 9902, 9930, 'n8n', '2.46.4', '2.46.4', 21000, 21001, 'running')`)
	native.units[77] = transport.AppUnitState{Present: true, Active: true, Enabled: true}

	v := f.view(t, 9802)
	if !strings.Contains(strings.Join(v.Preview.Apps, ","), "n8n on customer.matrix.test") {
		t.Fatalf("preview: %v", v.Preview.Apps)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	f.run(t)
	if native.units[77] != (transport.AppUnitState{Present: true}) {
		t.Fatalf("suspended app unit: %+v", native.units[77])
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if native.units[77] != (transport.AppUnitState{Present: true, Active: true, Enabled: true}) {
		t.Fatalf("resumed app unit: %+v", native.units[77])
	}
}
