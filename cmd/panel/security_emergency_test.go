package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Security emergency (alpha.117, D-043). The fixture is the suspension
// fixture: admin 9701, reseller 9702 with customer 9703 (subscription 9802),
// admin-managed customer 9704 (subscription 9803). PHP 8.5 sites: 9910
// (9703) and 9911 (9704); a PHP 8.4 site 9912 (9703); the Node.js site 9930
// (9703) runs 22.11.0. The server side is a fake that behaves like the
// Agent's compare-and-set calls and records what it was asked.

const emergencyTestPassword = "correct horse battery"

type fakeEmergencyNative struct {
	mu        sync.Mutex
	panel     *Panel
	apps      *fakeSuspensionNative
	units     map[string]transport.EmergencyUnitState
	pages     map[int]string // domain -> page rendered last
	renders   map[int]int
	lockErr   error
	moveFail  map[int]error
	removed   []string
	removeErr error
	// appLocks: site -> the withdrawn Node.js version its start lock refuses.
	appLocks    map[int]string
	editedLocks map[int]bool
}

func (f *fakeEmergencyNative) renderSite(ctx context.Context, domainID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	req, err := f.panel.buildVhostRequest(ctx, domainID, nil)
	if err != nil {
		return err
	}
	page := "site"
	switch {
	case req.Suspended:
		page = "suspended"
	case req.Maintenance:
		page = "maintenance"
	}
	f.pages[domainID] = page
	f.renders[domainID]++
	return nil
}

func (f *fakeEmergencyNative) inspectUnits(_ context.Context, service, version string) (transport.EmergencyUnitsInspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := transport.EmergencyUnitsInspectResponse{Units: []transport.EmergencyUnitObservation{}}
	for unit, state := range f.units {
		resp.Units = append(resp.Units, transport.EmergencyUnitObservation{Unit: unit, State: state})
	}
	return resp, nil
}

func (f *fakeEmergencyNative) setUnitLock(_ context.Context, req transport.SetEmergencyUnitLockRequest) (transport.SetEmergencyUnitLockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var resp transport.SetEmergencyUnitLockResponse
	if f.lockErr != nil {
		return resp, f.lockErr
	}
	locked := transport.EmergencyUnitState{Present: true, Masked: true}
	for unit, current := range f.units {
		target := locked
		from := req.Before[unit]
		if !req.Lock {
			target, from = req.Before[unit], locked
		}
		applied, change, conflict := casDecide(current, target, from)
		if req.Lock {
			applied, change, conflict = true, current != locked, false
		}
		if conflict {
			resp.Conflict = true
			resp.Error = unit + " was changed"
			continue
		}
		if change {
			f.units[unit] = target
			resp.Changed = true
		}
		resp.Applied = applied
		resp.Units = append(resp.Units, transport.EmergencyUnitObservation{Unit: unit, State: f.units[unit]})
	}
	if resp.Conflict {
		resp.Applied = false
	}
	return resp, nil
}

func (f *fakeEmergencyNative) inspectApp(ctx context.Context, siteID int) (transport.AppUnitState, error) {
	resp, err := f.apps.inspect(ctx, transport.SuspensionInspectRequest{AppSites: []int{siteID}})
	if err != nil {
		return transport.AppUnitState{}, err
	}
	return resp.AppUnits[0].State, nil
}

func (f *fakeEmergencyNative) setAppUnit(ctx context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	return f.apps.setAppUnit(ctx, req)
}

func (f *fakeEmergencyNative) movePHP(ctx context.Context, domainID int, from, to string, _ bool) error {
	f.mu.Lock()
	err := f.moveFail[domainID]
	f.mu.Unlock()
	if err != nil {
		return err
	}
	_, err = f.panel.db.GetDB().ExecContext(ctx, `UPDATE sites SET php_version = ? WHERE domain_id = ? AND php_version = ?`, to, domainID, from)
	return err
}

func (f *fakeEmergencyNative) moveNode(ctx context.Context, domainID int, from, to string) (string, error) {
	_, err := f.panel.db.GetDB().ExecContext(ctx, `UPDATE sites SET runtime_version = ? WHERE domain_id = ? AND runtime_version = ?`, to, domainID, from)
	return "", err
}

// setAppLock keeps the start locks by site; edited marks a lock the owner
// changed on the server.
func (f *fakeEmergencyNative) setAppLock(_ context.Context, req transport.SetEmergencyAppLockRequest) (transport.SetEmergencyAppLockResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := transport.SetEmergencyAppLockResponse{Path: "/etc/systemd/system/celikapp.service.d/celikpanel-security-emergency.conf"}
	if f.appLocks == nil {
		f.appLocks = map[int]string{}
	}
	if f.editedLocks[req.SiteID] {
		resp.Conflict, resp.Error = true, "changed on the server"
		return resp, nil
	}
	current, present := f.appLocks[req.SiteID]
	if present && current != req.NodeVersion {
		resp.Conflict, resp.Error = true, "changed on the server"
		return resp, nil
	}
	if req.Lock && !present {
		f.appLocks[req.SiteID] = req.NodeVersion
		resp.Changed = true
	}
	if !req.Lock && present {
		delete(f.appLocks, req.SiteID)
		resp.Changed = true
	}
	resp.Applied = true
	return resp, nil
}

func (f *fakeEmergencyNative) removePackage(_ context.Context, service, version string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return "", f.removeErr
	}
	f.removed = append(f.removed, service+":"+version)
	return "package removed; customer files, databases and mailboxes are kept", nil
}

type emergencyFixture struct {
	accountsFixture
	sus    *fakeSuspensionNative
	native *fakeEmergencyNative
}

func newEmergencyFixture(t *testing.T) emergencyFixture {
	t.Helper()
	sf := newSuspensionFixture(t)
	native := &fakeEmergencyNative{panel: sf.panel, apps: sf.native,
		units: map[string]transport.EmergencyUnitState{"php8.5-fpm": {Present: true, Active: true}},
		pages: map[int]string{}, renders: map[int]int{}, moveFail: map[int]error{}}
	sf.panel.emergencyNative = native
	// Mail is not available unless a test says so.
	oldHost := accountLinkPanelHost
	accountLinkPanelHost = func() string { return "" }
	t.Cleanup(func() { accountLinkPanelHost = oldHost; resetAccountMailCache() })
	resetAccountMailCache()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", sf.panel.handleUserByID)
	mux.HandleFunc("/api/v1/subscriptions/", sf.panel.handleSubscriptionEntitlements)
	mux.HandleFunc(securityEmergenciesPath, sf.panel.handleSecurityEmergencies)
	mux.HandleFunc(securityEmergenciesPath+"/", sf.panel.handleSecurityEmergencies)
	mux.HandleFunc(securityNoticesPath, sf.panel.handleSecurityNotices)
	mux.HandleFunc(securityNoticesPath+"/", sf.panel.handleSecurityNotices)
	mux.HandleFunc("/api/v1/domains/", sf.panel.handleDomainSubroute)
	f := emergencyFixture{accountsFixture: sf.accountsFixture, sus: sf.native, native: native}
	f.handler = sf.panel.requireAuth(mux)
	hash, err := auth.HashPassword(emergencyTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE users SET password_hash = ? WHERE id = 9701`, hash)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES
		(9910, 9802, 'php85.matrix.test', 'active'), (9911, 9803, 'outsider85.matrix.test', 'active'), (9912, 9802, 'php84.matrix.test', 'active')`)
	f.exec(t, `INSERT INTO sites (id, domain_id, document_root, project_type, php_version) VALUES
		(9940, 9910, '/srv/a', 'php', '8.5'), (9941, 9911, '/srv/b', 'php', '8.5'), (9942, 9912, '/srv/c2', 'php', '8.4')`)
	f.exec(t, `UPDATE sites SET runtime_version = '22.11.0', app_port = 3001, start_command = 'node app.js' WHERE id = 9930`)
	return f
}

func (f emergencyFixture) run(t *testing.T) {
	t.Helper()
	if err := f.panel.runPendingEmergencyOperations(context.Background()); err != nil {
		t.Logf("worker: %v", err)
	}
}

func (f emergencyFixture) declare(t *testing.T, component, version, recommended string) emergencyDetailView {
	t.Helper()
	body := `{"component":"` + component + `","version":"` + version + `","reason":"CVE-2026-0001","recommended":"` + recommended + `"}`
	rec := f.as(t, 9701, http.MethodPost, securityEmergenciesPath, body)
	wantStatus(t, rec, http.StatusCreated, "")
	var v emergencyDetailView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func confirmBody(name, extra string) string {
	return `{"reason":"CVE-2026-0001","confirm_name":"` + name + `","password":"` + emergencyTestPassword + `"` + extra + `}`
}

func (f emergencyFixture) step(t *testing.T, id, action, body string, status int, code string) emergencyDetailView {
	t.Helper()
	rec := f.as(t, 9701, http.MethodPost, securityEmergenciesPath+"/"+id+"/"+action, body)
	wantStatus(t, rec, status, code)
	var v emergencyDetailView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return v
}

func (f emergencyFixture) detail(t *testing.T, id string) emergencyDetailView {
	t.Helper()
	rec := f.as(t, 9701, http.MethodGet, securityEmergenciesPath+"/"+id, "")
	wantStatus(t, rec, http.StatusOK, "")
	var v emergencyDetailView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (f emergencyFixture) maintenance(t *testing.T, domainID int) bool {
	t.Helper()
	req, err := f.panel.buildVhostRequest(context.Background(), domainID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req.Maintenance
}

func TestEmergencyWithdrawRefusesNewChoiceToEveryone(t *testing.T) {
	f := newEmergencyFixture(t)
	v := f.declare(t, "php", "8.5", "8.4")
	if v.Emergency.Stage != emergencyStageWithdrawn || !v.Emergency.Withdrawn || v.ConfirmName != "PHP 8.5" || len(v.Preview.Sites) != 2 {
		t.Fatalf("withdraw: %+v", v)
	}
	// A second declaration answers with the open one.
	wantStatus(t, f.as(t, 9701, http.MethodPost, securityEmergenciesPath, `{"component":"php","version":"8.5","reason":"again"}`), http.StatusConflict, errCodeEmergencyExists)
	// Reason required; unknown targets refused.
	wantStatus(t, f.as(t, 9701, http.MethodPost, securityEmergenciesPath, `{"component":"php","version":"8.3","reason":" "}`), http.StatusBadRequest, errCodeEmergencyReason)
	wantStatus(t, f.as(t, 9701, http.MethodPost, securityEmergenciesPath, `{"component":"nginx","version":"","reason":"x"}`), http.StatusBadRequest, errCodeEmergencyBadTarget)

	policy, err := f.panel.loadOfferedVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if policy.offered("php", "8.5") || !policy.offered("php", "8.4") {
		t.Fatal("PHP 8.5 is still offered after the withdrawal")
	}
	// The administrator is refused too; a site keeping 8.5 is not.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	if !f.panel.refuseWithdrawnVersion(rec, req, "php", "8.4", "8.5") || !strings.Contains(rec.Body.String(), errCodeVersionWithdrawn) {
		t.Fatalf("withdrawn choice not refused: %s", rec.Body.String())
	}
	if f.panel.refuseWithdrawnVersion(httptest.NewRecorder(), req, "php", "8.5", "8.5") {
		t.Fatal("a site keeping its version was refused")
	}
	// Existing use keeps running: nothing was rendered, nothing stopped.
	if len(f.native.renders) != 0 || !f.native.units["php8.5-fpm"].Active || f.maintenance(t, 9910) {
		t.Fatal("the withdrawal changed the server")
	}
	// Reversible.
	v = f.step(t, v.Emergency.ID, "cancel", `{"reason":"false alarm"}`, http.StatusOK, "")
	if v.Emergency.Stage != emergencyStageResolved {
		t.Fatalf("cancel: %+v", v.Emergency)
	}
	policy, _ = f.panel.loadOfferedVersions(context.Background())
	if !policy.offered("php", "8.5") {
		t.Fatal("PHP 8.5 stays withdrawn after the cancel")
	}
	if f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'security.emergency.withdraw%'`) != 2 {
		t.Fatal("withdraw and its cancel were not audited")
	}
}

// A whole database engine withdrawn: no new database on it, for anyone.
func TestEmergencyWithdrawnEngineRefusesNewDatabases(t *testing.T) {
	f := newEmergencyFixture(t)
	f.declare(t, "mariadb", "", "")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: 9701, Role: roleAdmin}))
	rec := httptest.NewRecorder()
	if f.panel.requireSubscriptionOffering(rec, r, 9802, databaseEngineOffering("mariadb"), "mariadb", true) {
		t.Fatal("a new MariaDB database was allowed while MariaDB is withdrawn")
	}
	wantStatus(t, rec, http.StatusConflict, errCodeVersionWithdrawn)
	if !strings.Contains(rec.Body.String(), "MariaDB is withdrawn") {
		t.Fatalf("refusal: %s", rec.Body.String())
	}
	if !f.panel.requireSubscriptionOffering(httptest.NewRecorder(), r, 9802, databaseEngineOffering("postgresql"), "postgresql", true) {
		t.Fatal("PostgreSQL was refused")
	}
}

func TestEmergencyConfirmationRefusals(t *testing.T) {
	f := newEmergencyFixture(t)
	v := f.declare(t, "php", "8.5", "")
	id := v.Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.4", ""), http.StatusBadRequest, errCodeEmergencyNameMismatch)
	f.step(t, id, "stop", confirmBody("php 8.5", ""), http.StatusBadRequest, errCodeEmergencyNameMismatch)
	f.step(t, id, "stop", `{"reason":"x","confirm_name":"PHP 8.5","password":"wrong"}`, http.StatusForbidden, errCodeEmergencyWrongPassword)
	f.step(t, id, "stop", `{"reason":"","confirm_name":"PHP 8.5","password":"`+emergencyTestPassword+`"}`, http.StatusBadRequest, errCodeEmergencyReason)
	// Reseller and customer never reach the handler.
	wantStatus(t, f.as(t, 9702, http.MethodPost, securityEmergenciesPath+"/"+id+"/stop", confirmBody("PHP 8.5", "")), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, securityEmergenciesPath, ""), http.StatusForbidden, "")
	// "Log in as" a customer: the administrator's routes are closed.
	token, err := f.panel.sessions.CreateImpersonation(context.Background(), 9704, 9701)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, f.send(t, http.MethodPost, securityEmergenciesPath+"/"+id+"/stop", token, confirmBody("PHP 8.5", "")), http.StatusForbidden, "")
	// And the handler refuses any impersonated session itself, the correct
	// password and name notwithstanding.
	r := httptest.NewRequest(http.MethodPost, securityEmergenciesPath+"/"+id+"/stop", strings.NewReader(confirmBody("PHP 8.5", "")))
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: 9701, Role: roleAdmin, ImpersonatorID: 9709}))
	rec := httptest.NewRecorder()
	f.panel.handleSecurityEmergencies(rec, r)
	wantStatus(t, rec, http.StatusForbidden, errCodeEmergencyImpersonation)
	// Nothing changed.
	if f.count(t, `SELECT COUNT(*) FROM security_emergency_operations`) != 0 || f.native.units["php8.5-fpm"].Masked {
		t.Fatal("a refused confirmation changed something")
	}
	if f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'security.emergency.confirm_refused%'`) != 1 {
		t.Fatal("the wrong password was not audited")
	}
	var details string
	if err := f.panel.db.GetDB().QueryRow(`SELECT COALESCE(GROUP_CONCAT(details), '') FROM audit_logs`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(details, emergencyTestPassword) || strings.Contains(details, "wrong") {
		t.Fatal("a password reached the audit trail")
	}
}

func TestEmergencyPreviewListsSitesCustomersResellersAndPlans(t *testing.T) {
	f := newEmergencyFixture(t)
	f.exec(t, `INSERT INTO rights_sets (owner_kind, owner_id, all_software) VALUES ('subscription', 9803, 0)`)
	f.exec(t, `INSERT INTO rights_set_items (set_id, item) SELECT id, 'component:php-fpm:8.5' FROM rights_sets WHERE owner_kind = 'subscription' AND owner_id = 9803`)
	rec := f.as(t, 9701, http.MethodGet, securityEmergenciesPath+"/preview?component=php&version=8.5", "")
	wantStatus(t, rec, http.StatusOK, "")
	var p emergencyPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	domains := []string{}
	for _, site := range p.Sites {
		domains = append(domains, site.Domain+"/"+site.Customer+"/"+site.Reseller)
	}
	if strings.Join(domains, ",") != "outsider85.matrix.test/matrix-outsider/,php85.matrix.test/matrix-customer/matrix-reseller" {
		t.Fatalf("sites: %v", domains)
	}
	if len(p.Customers) != 2 || len(p.Resellers) != 1 || p.Resellers[0].Name != "matrix-reseller" {
		t.Fatalf("parties: %+v %+v", p.Customers, p.Resellers)
	}
	found := false
	for _, item := range p.Offering {
		found = found || item == "Custom subscription: outsider subscription"
	}
	if !found {
		t.Fatalf("offered by: %v", p.Offering)
	}
	wantStatus(t, f.as(t, 9701, http.MethodGet, securityEmergenciesPath+"/preview?component=php&version=8", ""), http.StatusBadRequest, errCodeEmergencyBadTarget)
}

func TestEmergencyStopShowsMaintenanceAndReenableRestoresExactly(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "8.4").Emergency.ID
	v := f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	if v.Operation == nil || v.Operation.State != "running" || v.Guidance.WhoActs != "nobody" {
		t.Fatalf("accepted stop: %+v", v)
	}
	// A repeated request and polling start nothing new.
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	for i := 0; i < 3; i++ {
		f.detail(t, id)
	}
	if f.count(t, `SELECT COUNT(*) FROM security_emergency_operations`) != 1 || len(f.native.renders) != 0 {
		t.Fatal("a repeated request or a poll started work")
	}
	f.run(t)
	v = f.detail(t, id)
	if v.Operation.State != "completed" || v.Emergency.Stage != emergencyStageStopped {
		t.Fatalf("stop: %+v", v.Operation)
	}
	if u := f.native.units["php8.5-fpm"]; u.Active || !u.Masked {
		t.Fatalf("unit after stop: %+v", u)
	}
	if f.native.pages[9910] != "maintenance" || f.native.pages[9911] != "maintenance" || f.maintenance(t, 9912) {
		t.Fatalf("pages: %+v", f.native.pages)
	}
	// Notices: the two customers and the reseller; mail is not available.
	if n := f.count(t, `SELECT COUNT(*) FROM security_notices WHERE kind = 'emergency'`); n != 3 {
		t.Fatalf("notices: %d", n)
	}
	if v.Notices.NotAvailable != 3 || !strings.Contains(v.Guidance.Result, "panel only") {
		t.Fatalf("mail summary: %+v %q", v.Notices, v.Guidance.Result)
	}
	// Re-enable needs the password.
	f.step(t, id, "reenable", `{"reason":"patched","password":"nope"}`, http.StatusForbidden, errCodeEmergencyWrongPassword)
	f.step(t, id, "reenable", `{"reason":"patched 8.5.2","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
	f.run(t)
	v = f.detail(t, id)
	if v.Operation.State != "completed" || v.Emergency.Stage != emergencyStageResolved || v.Emergency.Withdrawn {
		t.Fatalf("re-enable: %+v %+v", v.Operation, v.Emergency)
	}
	if u := f.native.units["php8.5-fpm"]; !u.Active || u.Masked {
		t.Fatalf("unit after re-enable: %+v", u)
	}
	if f.native.pages[9910] != "site" || f.native.pages[9911] != "site" || f.maintenance(t, 9910) {
		t.Fatalf("pages after re-enable: %+v", f.native.pages)
	}
	if f.count(t, `SELECT COUNT(*) FROM security_notices WHERE kind = 'resolved'`) != 3 ||
		f.count(t, `SELECT COUNT(*) FROM security_notices WHERE kind = 'emergency' AND seen_at IS NULL`) != 0 {
		t.Fatal("resolved notices")
	}
	if f.count(t, `SELECT COUNT(*) FROM security_emergency_before_images`) != 0 {
		t.Fatal("before-images were not spent")
	}
	if f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'security.emergency.finished:%'`) != 2 {
		t.Fatal("finished steps were not audited")
	}
}

func TestEmergencyUnlockLeavesTheOwnersChange(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	// The owner unmasked it by hand.
	f.native.units["php8.5-fpm"] = transport.EmergencyUnitState{Present: true}
	f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
	f.run(t)
	v := f.detail(t, id)
	if v.Operation.Effects[0].State != effectKept || f.native.units["php8.5-fpm"].Active {
		t.Fatalf("owner change: %+v %+v", v.Operation.Effects[0], f.native.units)
	}
}

func TestEmergencyAndSuspensionCoexistAndRestoreExactly(t *testing.T) {
	f := newEmergencyFixture(t)
	sub := "/api/v1/subscriptions/9802/suspension"
	id := f.declare(t, "php", "8.5", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	if err := f.panel.runPendingSuspensionOperations(context.Background()); err != nil {
		t.Logf("suspension: %v", err)
	}
	req, _ := f.panel.buildVhostRequest(context.Background(), 9910, nil)
	if !req.Suspended || !req.Maintenance {
		t.Fatalf("both: %+v", req)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"resume"}`), http.StatusAccepted, "")
	if err := f.panel.runPendingSuspensionOperations(context.Background()); err != nil {
		t.Logf("suspension: %v", err)
	}
	req, _ = f.panel.buildVhostRequest(context.Background(), 9910, nil)
	if req.Suspended || !req.Maintenance {
		t.Fatalf("after resume the emergency must stay: %+v", req)
	}
	f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
	f.run(t)
	req, _ = f.panel.buildVhostRequest(context.Background(), 9910, nil)
	if req.Suspended || req.Maintenance {
		t.Fatalf("after both ended: %+v", req)
	}
}

// A Node.js app stopped by both a suspension and an emergency starts again
// only when the last of them ends, in either order.
func TestEmergencyNodeAppHandsOverWithSuspension(t *testing.T) {
	for _, order := range []string{"emergency-first", "suspension-first"} {
		t.Run(order, func(t *testing.T) {
			f := newEmergencyFixture(t)
			sub := "/api/v1/subscriptions/9802/suspension"
			runSuspension := func() {
				if err := f.panel.runPendingSuspensionOperations(context.Background()); err != nil {
					t.Logf("suspension: %v", err)
				}
			}
			id := f.declare(t, "node", "22.11.0", "").Emergency.ID
			if order == "emergency-first" {
				f.step(t, id, "stop", confirmBody("Node.js 22.11.0", ""), http.StatusAccepted, "")
				f.run(t)
				wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
				runSuspension()
			} else {
				wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
				runSuspension()
				f.step(t, id, "stop", confirmBody("Node.js 22.11.0", ""), http.StatusAccepted, "")
				f.run(t)
			}
			if f.sus.apps[9930].Active {
				t.Fatal("the app runs under both")
			}
			// The first to end leaves it stopped.
			if order == "emergency-first" {
				wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"resume"}`), http.StatusAccepted, "")
				runSuspension()
			} else {
				f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
				f.run(t)
			}
			if f.sus.apps[9930].Active {
				t.Fatal("the app started while the other still holds it")
			}
			// The last to end starts it, as it was before both.
			if order == "emergency-first" {
				f.step(t, id, "reenable", `{"reason":"patched","password":"`+emergencyTestPassword+`"}`, http.StatusAccepted, "")
				f.run(t)
			} else {
				wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"resume"}`), http.StatusAccepted, "")
				runSuspension()
			}
			if app := f.sus.apps[9930]; !app.Active || !app.Enabled {
				t.Fatalf("app after both ended: %+v", app)
			}
		})
	}
}

func TestEmergencyBulkMoveListsSitesThatCouldNotMove(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "8.4").Emergency.ID
	// Moving before the stop is refused.
	f.step(t, id, "move", confirmBody("PHP 8.5", `,"target_version":"8.4"`), http.StatusConflict, errCodeEmergencyWrongStage)
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	f.native.moveFail[9911] = verifiedFailure("the site's pool file was changed on the server")
	f.step(t, id, "move", confirmBody("PHP 8.5", `,"target_version":"8.5"`), http.StatusBadRequest, errCodeEmergencyBadMoveTarget)
	f.step(t, id, "move", confirmBody("PHP 8.5", `,"target_version":"8.4"`), http.StatusAccepted, "")
	f.run(t)
	v := f.detail(t, id)
	if v.Operation.State != "partial" || !strings.Contains(v.Guidance.Result, "1 site(s) stay") {
		t.Fatalf("partial move: %+v %q", v.Operation, v.Guidance.Result)
	}
	if f.maintenance(t, 9910) || f.native.pages[9910] != "site" || !f.maintenance(t, 9911) {
		t.Fatalf("pages: %+v", f.native.pages)
	}
	moved := f.count(t, `SELECT COUNT(*) FROM sites WHERE id = 9940 AND php_version = '8.4'`)
	kept := f.count(t, `SELECT COUNT(*) FROM sites WHERE id = 9941 AND php_version = '8.5'`)
	if moved != 1 || kept != 1 {
		t.Fatal("site versions after a partial move")
	}
	failed := false
	for _, effect := range v.Operation.Effects {
		if effect.Target == "outsider85.matrix.test" {
			failed = effect.State == effectFailed && strings.Contains(effect.Detail, "not moved")
		}
	}
	if !failed {
		t.Fatalf("the site that could not move is not listed: %+v", v.Operation.Effects)
	}
	// Moving again takes only the site still on the maintenance page.
	delete(f.native.moveFail, 9911)
	f.step(t, id, "move", confirmBody("PHP 8.5", `,"target_version":"8.4"`), http.StatusAccepted, "")
	f.run(t)
	v = f.detail(t, id)
	if v.Operation.State != "completed" || v.Operation.Total != 1 || f.maintenance(t, 9911) {
		t.Fatalf("second move: %+v", v.Operation)
	}
}

func TestEmergencyRemoveBypassesInUseGuardOnlyUnderAnEmergency(t *testing.T) {
	f := newEmergencyFixture(t)
	// The ordinary guard still names the sites.
	count, _, err := runtimeVersionBlockers(context.Background(), f.panel.db.GetDB(), "php-fpm", "8.5")
	if err != nil || count != 2 {
		t.Fatalf("blockers: %d %v", count, err)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, securityEmergenciesPath+"/0123456789abcdef/remove", confirmBody("PHP 8.5", "")), http.StatusNotFound, "")
	id := f.declare(t, "php", "8.5", "").Emergency.ID
	// Withdrawn only: removal waits for the stop.
	f.step(t, id, "remove", confirmBody("PHP 8.5", ""), http.StatusConflict, errCodeEmergencyWrongStage)
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	f.step(t, id, "remove", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	v := f.detail(t, id)
	if v.Emergency.Stage != emergencyStageRemoved || strings.Join(f.native.removed, ",") != "php-fpm:8.5" {
		t.Fatalf("remove: %+v %v", v.Emergency, f.native.removed)
	}
	// Customer data stays, the sites stay on the maintenance page.
	if f.count(t, `SELECT COUNT(*) FROM sites WHERE id IN (9940, 9941)`) != 2 || !f.maintenance(t, 9910) || !f.maintenance(t, 9911) {
		t.Fatal("removal touched the sites")
	}
	// A repeat answers with the same operation.
	f.step(t, id, "remove", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	if len(f.native.removed) != 1 || f.count(t, `SELECT COUNT(*) FROM security_emergency_operations WHERE kind = 'remove'`) != 1 {
		t.Fatal("a repeated removal ran twice")
	}
}

func TestEmergencyResumesTheSameOperationAfterACrash(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "").Emergency.ID
	f.native.lockErr = errors.New("agent connection reset")
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	v := f.detail(t, id)
	if v.Operation.State != "running" || v.Operation.ErrorKind != "unknown" || f.native.renders[9910] != 1 {
		t.Fatalf("after the lost answer: %+v", v.Operation)
	}
	// "Restart": a new worker over the same database continues the same
	// operation; the pages already done are not rendered again.
	f.native.lockErr = nil
	f.run(t)
	v = f.detail(t, id)
	if v.Operation.State != "completed" || f.native.renders[9910] != 1 || !f.native.units["php8.5-fpm"].Masked {
		t.Fatalf("after the restart: %+v renders=%v", v.Operation, f.native.renders)
	}
	if f.count(t, `SELECT COUNT(*) FROM security_emergency_operations`) != 1 {
		t.Fatal("a second operation was started")
	}
}

func TestEmergencyFailedStepWaitsForContinue(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "php", "8.5", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	f.native.removeErr = verifiedFailure("dpkg was interrupted")
	f.step(t, id, "remove", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	v := f.detail(t, id)
	if v.Operation.State != "failed" || !v.Can["continue"] || !strings.Contains(v.Guidance.NextAction, "dpkg was interrupted") {
		t.Fatalf("failed: %+v %+v", v.Operation, v.Guidance)
	}
	f.native.removeErr = nil
	f.step(t, id, "continue", `{}`, http.StatusAccepted, "")
	f.run(t)
	if v = f.detail(t, id); v.Emergency.Stage != emergencyStageRemoved {
		t.Fatalf("continued: %+v", v.Emergency)
	}
}

func TestEmergencyNoticesWithMailAndOneClickSwitch(t *testing.T) {
	f := newEmergencyFixture(t)
	var mu sync.Mutex
	sent := map[string]string{}
	oldHost, oldProbe, oldSend := accountLinkPanelHost, accountMailProbe, accountMailSend
	t.Cleanup(func() { accountLinkPanelHost, accountMailProbe, accountMailSend = oldHost, oldProbe, oldSend })
	accountLinkPanelHost = func() string { return "panel.example.test" }
	accountMailProbe = func(context.Context) error { return nil }
	accountMailSend = func(_ context.Context, _, to string, message []byte) error {
		mu.Lock()
		defer mu.Unlock()
		sent[to] = string(message)
		return nil
	}
	resetAccountMailCache()
	f.exec(t, `UPDATE users SET email = '' WHERE id = 9704`)
	id := f.declare(t, "php", "8.5", "8.4").Emergency.ID
	f.step(t, id, "stop", confirmBody("PHP 8.5", ""), http.StatusAccepted, "")
	f.run(t)
	if len(sent) != 2 || sent["matrix-customer@example.test"] == "" || sent["matrix-reseller@example.test"] == "" {
		t.Fatalf("mail: %v", sent)
	}
	v := f.detail(t, id)
	if v.Notices.MailSent != 2 || v.Notices.NoAddress != 1 {
		t.Fatalf("summary: %+v", v.Notices)
	}
	// The customer sees its notice with the one-click switch to 8.4.
	rec := f.as(t, 9703, http.MethodGet, securityNoticesPath, "")
	wantStatus(t, rec, http.StatusOK, "")
	var got struct {
		Notices []securityNoticeView `json:"notices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Notices) != 1 || got.Notices[0].Body.Reason != "CVE-2026-0001" || len(got.Notices[0].Actions) != 1 ||
		got.Notices[0].Actions[0].To != "8.4" || got.Notices[0].Actions[0].DomainID != 9910 {
		t.Fatalf("customer notice: %s", rec.Body.String())
	}
	// The reseller sees its customer's site, named.
	rec = f.as(t, 9702, http.MethodGet, securityNoticesPath, "")
	if !strings.Contains(rec.Body.String(), `"customer":"matrix-customer"`) || !strings.Contains(rec.Body.String(), `"audience":"reseller"`) {
		t.Fatalf("reseller notice: %s", rec.Body.String())
	}
	// Another account cannot mark it seen.
	wantStatus(t, f.as(t, 9704, http.MethodPost, securityNoticesPath+"/"+itoa(got.Notices[0].ID)+"/seen", ""), http.StatusNotFound, "")
	// The customer's own switch (the ordinary path) takes the site off the list.
	f.exec(t, `UPDATE sites SET php_version = '8.4' WHERE id = 9940`)
	f.panel.noteEmergencySiteMoved(context.Background(), 9910)
	if f.maintenance(t, 9910) || f.count(t, `SELECT COUNT(*) FROM security_emergency_sites WHERE domain_id = 9910 AND maintenance = 0 AND moved_to = '8.4'`) != 1 {
		t.Fatal("the customer's switch did not leave the maintenance list")
	}
	wantStatus(t, f.as(t, 9703, http.MethodPost, securityNoticesPath+"/"+itoa(got.Notices[0].ID)+"/seen", ""), http.StatusOK, "")
	rec = f.as(t, 9703, http.MethodGet, securityNoticesPath, "")
	if strings.Contains(rec.Body.String(), "CVE") {
		t.Fatalf("seen notice still listed: %s", rec.Body.String())
	}
}

func TestEmergencyStoppedAppIsNotStartedByHand(t *testing.T) {
	f := newEmergencyFixture(t)
	id := f.declare(t, "node", "22.11.0", "").Emergency.ID
	f.step(t, id, "stop", confirmBody("Node.js 22.11.0", ""), http.StatusAccepted, "")
	f.run(t)
	if f.sus.apps[9930].Active || f.native.pages[9902] != "maintenance" {
		t.Fatalf("node stop: %+v %+v", f.sus.apps[9930], f.native.pages)
	}
	for _, action := range []string{"start", "restart"} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/domains/9902/app/"+action, nil)
		rec := httptest.NewRecorder()
		if !f.panel.refuseAppStartUnderEmergency(rec, r, 9902, action) || !strings.Contains(rec.Body.String(), errCodeEmergencyMaintenance) {
			t.Fatalf("%s not refused: %s", action, rec.Body.String())
		}
	}
	if f.panel.refuseAppStartUnderEmergency(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), 9902, "stop") {
		t.Fatal("stop was refused")
	}
}
