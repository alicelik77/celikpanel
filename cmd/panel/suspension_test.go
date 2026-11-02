package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Subscription suspension (alpha.117). The fixture is the authz matrix:
// admin 9701, reseller 9702 with customer 9703 (subscription 9802, domain
// 9902 customer.matrix.test), admin-managed customer 9704 (subscription
// 9803). The server side is a fake that behaves like the Agent's
// compare-and-set calls and records what it was asked.

type fakeSuspensionNative struct {
	mu          sync.Mutex
	renders     []int
	renderErr   error
	siteExpiry  map[string]string // domain -> shadow expiry
	siteCalls   int
	siteFailAt  int               // fail (unknown result) on this site call number, after applying
	domainLine  map[string]string // domain -> map line, "" = absent
	deferred    map[string]bool   // domain -> its mail answered 450 (suspended)
	nologin     map[string]bool
	mailCalls   int
	mailFail    func() error
	dbLocked    map[string]bool
	dbCalls     []transport.SetDatabaseUserSuspensionRequest
	vpnSyncs    int
	apps        map[int]transport.AppUnitState // site id -> app unit
	statusAtWeb []bool
	panel       *Panel
}

func newFakeSuspensionNative(p *Panel) *fakeSuspensionNative {
	return &fakeSuspensionNative{
		siteExpiry: map[string]string{"customer.matrix.test": ""},
		domainLine: map[string]string{"customer.matrix.test": "customer.matrix.test OK"},
		nologin:    map[string]bool{"box@customer.matrix.test": false},
		deferred:   map[string]bool{},
		apps:       map[int]transport.AppUnitState{},
		dbLocked:   map[string]bool{"cust_user": false, "owner_locked": true},
		panel:      p,
	}
}

func (f *fakeSuspensionNative) renderSite(ctx context.Context, domainID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.renderErr != nil {
		return f.renderErr
	}
	var subID int
	if err := f.panel.db.GetDB().QueryRowContext(ctx, `SELECT subscription_id FROM domains WHERE id = ?`, domainID).Scan(&subID); err != nil {
		return err
	}
	suspended, err := f.panel.subscriptionSuspended(ctx, subID)
	if err != nil {
		return err
	}
	f.renders = append(f.renders, domainID)
	f.statusAtWeb = append(f.statusAtWeb, suspended)
	return nil
}

func (f *fakeSuspensionNative) inspect(_ context.Context, req transport.SuspensionInspectRequest) (transport.SuspensionInspectResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var resp transport.SuspensionInspectResponse
	for _, a := range req.SiteAccounts {
		expiry, ok := f.siteExpiry[a.Domain]
		resp.SiteAccounts = append(resp.SiteAccounts, transport.SiteAccountObservation{Account: a, State: transport.SiteAccountState{Present: ok, Expiry: expiry}})
	}
	for _, d := range req.MailDomains {
		state := transport.MailDomainState{LoginRefused: map[string]bool{}}
		state.DomainLine = f.domainLine[d.Domain]
		state.DomainListed = state.DomainLine != ""
		state.MailDeferred = f.deferred[d.Domain]
		for _, m := range d.Mailboxes {
			if v, ok := f.nologin[m]; ok {
				state.LoginRefused[m] = v
			}
		}
		resp.MailDomains = append(resp.MailDomains, transport.MailDomainObservation{Domain: d, State: state})
	}
	for _, u := range req.DatabaseUsers {
		locked, ok := f.dbLocked[u.Username]
		resp.DatabaseUsers = append(resp.DatabaseUsers, transport.DatabaseUserObservation{User: u, State: transport.DatabaseUserState{Present: ok, Locked: locked}})
	}
	for _, site := range req.AppSites {
		resp.AppUnits = append(resp.AppUnits, transport.AppUnitObservation{SiteID: site, State: f.apps[site]})
	}
	return resp, nil
}

func casDecide[T comparable](current, target, from T) (applied, change, conflict bool) {
	if current == target {
		return true, false, false
	}
	if current == from {
		return true, true, false
	}
	return false, false, true
}

func (f *fakeSuspensionNative) setSiteAccount(_ context.Context, req transport.SetSiteAccountSuspensionRequest) (transport.SetSiteAccountSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.siteCalls++
	var resp transport.SetSiteAccountSuspensionResponse
	current, ok := f.siteExpiry[req.Account.Domain]
	if !ok {
		resp.Error = "site account is absent: no account"
		return resp, nil
	}
	target, from := transport.SuspensionSiteAccountExpiry, req.Before.Expiry
	if !req.Suspend {
		target, from = req.Before.Expiry, transport.SuspensionSiteAccountExpiry
	}
	applied, change, conflict := casDecide(current, target, from)
	if conflict {
		resp.Conflict = true
		resp.Error = "expiry is " + current
		return resp, nil
	}
	if change {
		f.siteExpiry[req.Account.Domain] = target
	}
	if f.siteFailAt == f.siteCalls {
		// The change happened, the answer was lost.
		return resp, errors.New("agent connection reset")
	}
	resp.Applied, resp.Changed = applied, change
	resp.State = transport.SiteAccountState{Present: true, Expiry: f.siteExpiry[req.Account.Domain]}
	return resp, nil
}

func (f *fakeSuspensionNative) setMailDomain(_ context.Context, req transport.SetMailDomainSuspensionRequest) (transport.SetMailDomainSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mailCalls++
	var resp transport.SetMailDomainSuspensionResponse
	if f.mailFail != nil {
		if err := f.mailFail(); err != nil {
			return resp, err
		}
	}
	d := req.Domain.Domain
	if req.Suspend {
		// The domain stays hosted; its mail is answered with a temporary 450.
		f.deferred[d] = true
		for _, m := range req.Domain.Mailboxes {
			if _, ok := f.nologin[m]; ok {
				f.nologin[m] = true
			}
		}
	} else {
		f.deferred[d] = req.Before.MailDeferred
		for _, m := range req.Domain.Mailboxes {
			if _, ok := f.nologin[m]; ok {
				f.nologin[m] = req.Before.LoginRefused[m]
			}
		}
	}
	resp.Applied, resp.Changed = true, true
	return resp, nil
}

func (f *fakeSuspensionNative) setDatabaseUser(_ context.Context, req transport.SetDatabaseUserSuspensionRequest) (transport.SetDatabaseUserSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dbCalls = append(f.dbCalls, req)
	var resp transport.SetDatabaseUserSuspensionResponse
	current, ok := f.dbLocked[req.User.Username]
	if !ok {
		resp.Error = "database user is absent: gone"
		return resp, nil
	}
	target, from := true, req.Before.Locked
	if !req.Suspend {
		target, from = req.Before.Locked, true
	}
	applied, change, conflict := casDecide(current, target, from)
	if conflict {
		resp.Conflict, resp.Error = true, "changed"
		return resp, nil
	}
	if change {
		f.dbLocked[req.User.Username] = target
	}
	resp.Applied, resp.Changed = applied, change
	return resp, nil
}

func (f *fakeSuspensionNative) setAppUnit(_ context.Context, req transport.SetAppUnitSuspensionRequest) (transport.SetAppUnitSuspensionResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var resp transport.SetAppUnitSuspensionResponse
	current, ok := f.apps[req.SiteID]
	if !ok || !current.Present {
		resp.Error = "app unit is absent: none"
		return resp, nil
	}
	target := transport.AppUnitState{Present: true}
	if !req.Suspend {
		target = req.Before
	}
	resp.Applied, resp.Changed = true, current != target
	f.apps[req.SiteID] = target
	resp.State = target
	return resp, nil
}

func (f *fakeSuspensionNative) syncVPN(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vpnSyncs++
	return nil
}

type suspensionFixture struct {
	accountsFixture
	native *fakeSuspensionNative
}

func newSuspensionFixture(t *testing.T) suspensionFixture {
	t.Helper()
	base := newAccountsFixture(t)
	native := newFakeSuspensionNative(base.panel)
	base.panel.suspensionNative = native
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users/", base.panel.handleUserByID)
	mux.HandleFunc("/api/v1/subscriptions/", base.panel.handleSubscriptionEntitlements)
	mux.HandleFunc(suspensionDeletablePath, base.panel.handleSuspensionDeletable)
	mux.HandleFunc("/api/v1/auth/me", base.panel.handleMe)
	mux.HandleFunc("/api/v1/domains", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	base.handler = base.panel.requireAuth(mux)
	f := suspensionFixture{accountsFixture: base, native: native}
	// Subscription 9802's resources: a site, a mailbox, two local database
	// users (one the owner had locked) and a VPN peer.
	var typeID int
	if err := base.panel.db.GetDB().QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	// A Node.js site whose app runs and starts at boot.
	f.exec(t, `INSERT INTO sites (id, domain_id, document_root, project_type) VALUES (9930, 9902, '/srv/c', 'node')`)
	native.apps[9930] = transport.AppUnitState{Present: true, Active: true, Enabled: true}
	f.exec(t, `INSERT INTO email_accounts (domain_id, address, password_hash) VALUES (9902, 'box@customer.matrix.test', 'x')`)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (9921, 9802, ?, 'local', 'localhost', 3306)`, typeID)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (9922, 9802, ?, 'remote', 'db.example.net', 3306)`, typeID)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9921, 9802, 'cust_user', 'x'), (9921, 9802, 'owner_locked', 'x'), (9922, 9802, 'far_user', 'x')`)
	f.exec(t, `INSERT INTO vpn_peers (subscription_id, name, public_key, preshared_key, ip, desired_state) VALUES (9802, 'phone', 'pk', 'psk', '10.66.0.2', 'active')`)
	return f
}

func (f suspensionFixture) run(t *testing.T) {
	t.Helper()
	if err := f.panel.runPendingSuspensionOperations(context.Background()); err != nil {
		t.Logf("worker: %v", err)
	}
}

func (f suspensionFixture) status(t *testing.T, sub int) string {
	t.Helper()
	var status string
	if err := f.panel.db.GetDB().QueryRow(`SELECT status FROM subscriptions WHERE id = ?`, sub).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f suspensionFixture) view(t *testing.T, sub int) subscriptionSuspensionView {
	t.Helper()
	rec := f.as(t, 9701, http.MethodGet, "/api/v1/subscriptions/"+strconv.Itoa(sub)+"/suspension", "")
	wantStatus(t, rec, http.StatusOK, "")
	var v subscriptionSuspensionView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSuspensionSuspendAndResumeRestoresExactly(t *testing.T) {
	f := newSuspensionFixture(t)
	path := "/api/v1/subscriptions/9802/suspension"

	// Preview before anything changes.
	v := f.view(t, 9802)
	if !v.CanSuspend || v.CanResume || v.Preview.Action != "suspend" || v.Status != "active" {
		t.Fatalf("initial view: %+v", v)
	}
	if strings.Join(v.Preview.Sites, ",") != "customer.matrix.test" || v.Preview.Mailboxes != 1 ||
		len(v.Preview.DatabaseUsers) != 3 || v.Preview.VPNPeers != 1 || len(v.Preview.SiteAccounts) != 1 {
		t.Fatalf("preview: %+v", v.Preview)
	}

	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"suspend"}`), http.StatusBadRequest, errCodeSuspensionReasonRequired)
	if f.status(t, 9802) != "active" {
		t.Fatal("a refused request changed the status")
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	if f.status(t, 9802) != "suspended" {
		t.Fatal("status did not change at acceptance")
	}
	// A GET only reads: nothing ran on the server yet.
	f.view(t, 9802)
	if len(f.native.renders) != 0 || f.native.siteCalls != 0 {
		t.Fatal("a GET started work")
	}
	f.run(t)
	v = f.view(t, 9802)
	if v.Operation == nil || v.Operation.State != "completed" || v.Operation.Kind != "suspend" {
		t.Fatalf("operation: %+v", v.Operation)
	}
	states := map[string]string{}
	for _, e := range v.Operation.Effects {
		states[e.Kind+" "+e.Target] = e.State
	}
	if states["database mysql:far_user@db.example.net:0"] != effectManual {
		t.Fatalf("remote database user: %v", states)
	}
	if len(f.native.renders) != 1 || !f.native.statusAtWeb[0] {
		t.Fatalf("web render: %v %v", f.native.renders, f.native.statusAtWeb)
	}
	if f.native.siteExpiry["customer.matrix.test"] != "1" || !f.native.deferred["customer.matrix.test"] || f.native.domainLine["customer.matrix.test"] != "customer.matrix.test OK" ||
		!f.native.nologin["box@customer.matrix.test"] || !f.native.dbLocked["cust_user"] || f.native.vpnSyncs != 1 ||
		f.native.apps[9930] != (transport.AppUnitState{Present: true}) {
		t.Fatalf("server state after suspend: %+v", f.native)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM suspension_before_images WHERE subscription_id = 9802`); n != 5 {
		t.Fatalf("before-images: %d", n)
	}
	if !v.CanResume || v.CanSuspend || v.Preview.Action != "resume" || !v.Causes.Manual || v.Reason != "unpaid" {
		t.Fatalf("suspended view: %+v", v)
	}

	// Suspending again changes nothing and starts nothing.
	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"suspend","reason":"again"}`), http.StatusAccepted, "")
	if n := f.count(t, `SELECT COUNT(*) FROM suspension_operations WHERE subscription_id = 9802`); n != 1 {
		t.Fatalf("operations after a repeated suspend: %d", n)
	}

	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"resume"}`), http.StatusAccepted, "")
	if f.status(t, 9802) != "active" {
		t.Fatal("status did not return to active")
	}
	f.run(t)
	v = f.view(t, 9802)
	if v.Operation.State != "completed" || v.Operation.Kind != "resume" {
		t.Fatalf("resume: %+v", v.Operation)
	}
	if f.native.siteExpiry["customer.matrix.test"] != "" || f.native.domainLine["customer.matrix.test"] != "customer.matrix.test OK" || f.native.deferred["customer.matrix.test"] ||
		f.native.nologin["box@customer.matrix.test"] || f.native.dbLocked["cust_user"] ||
		f.native.apps[9930] != (transport.AppUnitState{Present: true, Active: true, Enabled: true}) {
		t.Fatalf("not restored exactly: %+v", f.native)
	}
	// The owner had locked this user before: it stays locked.
	if !f.native.dbLocked["owner_locked"] {
		t.Fatal("resume unlocked a user the owner had locked")
	}
	if len(f.native.statusAtWeb) != 2 || f.native.statusAtWeb[1] {
		t.Fatalf("web render on resume: %v", f.native.statusAtWeb)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM suspension_before_images WHERE subscription_id = 9802`); n != 0 {
		t.Fatalf("before-images left after resume: %d", n)
	}
}

func TestSuspensionResumesTheSameOperationAfterACrash(t *testing.T) {
	f := newSuspensionFixture(t)
	// The site account changes, but the answer is lost (a crash or a broken
	// connection after the change).
	f.native.siteFailAt = 1
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	if err := f.panel.runSuspensionOperation(context.Background(), firstOperationID(t, f)); err != nil {
		t.Fatal(err)
	}
	v := f.view(t, 9802)
	if v.Operation.State != "running" || v.Operation.Guidance.Result != "in_progress" || v.Operation.Guidance.WhoActs != "panel" {
		t.Fatalf("after the lost answer: %+v", v.Operation)
	}
	// A new panel process: the worker continues the same operation.
	f.run(t)
	v = f.view(t, 9802)
	if v.Operation.State != "completed" {
		t.Fatalf("not continued: %+v", v.Operation)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM suspension_operations`); n != 1 {
		t.Fatalf("a second operation was started: %d", n)
	}
	// The web step finished before the crash and was not repeated; the site
	// step ran again against the stored before-image and found it done.
	if len(f.native.renders) != 1 || f.native.siteCalls != 2 {
		t.Fatalf("renders %d, site calls %d", len(f.native.renders), f.native.siteCalls)
	}
	var before string
	if err := f.panel.db.GetDB().QueryRow(`SELECT before FROM suspension_before_images WHERE kind = 'site_account'`).Scan(&before); err != nil || !strings.Contains(before, `"expiry":""`) {
		t.Fatalf("before-image %q %v: the expired state must never be recorded as the original", before, err)
	}
	// Resume restores the original, not the state found after the crash.
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if f.native.siteExpiry["customer.matrix.test"] != "" {
		t.Fatalf("expiry after resume: %q", f.native.siteExpiry["customer.matrix.test"])
	}
}

func firstOperationID(t *testing.T, f suspensionFixture) string {
	t.Helper()
	var id string
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM suspension_operations ORDER BY created_at, rowid LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSuspensionFailureIsBoundedAndRetryContinues(t *testing.T) {
	f := newSuspensionFixture(t)
	f.native.mailFail = func() error { return errors.New("connection refused") }
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	for i := 0; i < suspensionAutoAttempts+2; i++ {
		f.run(t)
	}
	v := f.view(t, 9802)
	if v.Operation.State != "failed" || v.Operation.Attempts != suspensionAutoAttempts || v.Operation.Guidance.Result != "unknown" ||
		v.Operation.Guidance.WhoActs != "administrator" || !strings.Contains(v.Operation.Guidance.Reason, "connection refused") {
		t.Fatalf("bounded failure: %+v", v.Operation)
	}
	if f.native.mailCalls != suspensionAutoAttempts {
		t.Fatalf("mail calls %d", f.native.mailCalls)
	}
	// The opposite action while it is stopped is allowed; while running it
	// is refused. Here: retry continues the same operation.
	f.native.mailFail = nil
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"retry"}`), http.StatusAccepted, "")
	f.run(t)
	v = f.view(t, 9802)
	if v.Operation.State != "completed" || len(f.native.renders) != 1 {
		t.Fatalf("retry: %+v renders=%d", v.Operation, len(f.native.renders))
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"retry"}`), http.StatusConflict, "")
}

func TestSuspensionKeptOwnerChangeIsNotOverwritten(t *testing.T) {
	f := newSuspensionFixture(t)
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	f.run(t)
	// While suspended, the server owner sets an expiry of their own.
	f.native.siteExpiry["customer.matrix.test"] = "20500"
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	v := f.view(t, 9802)
	if v.Operation.State != "completed" || f.native.siteExpiry["customer.matrix.test"] != "20500" {
		t.Fatalf("owner change: %+v %q", v.Operation, f.native.siteExpiry["customer.matrix.test"])
	}
	kept := false
	for _, e := range v.Operation.Effects {
		kept = kept || (e.Kind == suspensionEffectSiteAccount && e.State == effectKept)
	}
	if !kept || v.Operation.Guidance.WhoActs != "administrator" {
		t.Fatalf("kept item not reported: %+v", v.Operation)
	}
}

func TestSuspensionBusyRefusesTheOppositeAction(t *testing.T) {
	f := newSuspensionFixture(t)
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusConflict, errCodeSuspensionBusy)
	if f.status(t, 9802) != "suspended" {
		t.Fatal("a refused resume changed the status")
	}
}

func TestSuspensionResellerCascadeKeepsManualCause(t *testing.T) {
	f := newSuspensionFixture(t)
	// A second customer of the reseller, suspended by hand first.
	f.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES (9706, 'matrix-second', 'x', 'second@example.test', 'customer', 9702, 'active')`)
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, status) VALUES (9806, 9706, 'second subscription', 'active')`)
	wantStatus(t, f.as(t, 9702, http.MethodPost, "/api/v1/users/9706/suspension", `{"action":"suspend","reason":"own decision"}`), http.StatusAccepted, "")
	f.run(t)

	// Only the administrator manages the reseller.
	wantStatus(t, f.as(t, 9702, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"suspend","reason":"x"}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"suspend","reason":"reseller unpaid"}`), http.StatusAccepted, "")
	f.run(t)
	for _, sub := range []int{9801, 9802, 9806} {
		if f.status(t, sub) != "suspended" {
			t.Fatalf("subscription %d not suspended by the reseller cascade", sub)
		}
	}
	v := f.view(t, 9802)
	if v.Causes.ResellerID != 9702 || v.Causes.Reseller != "matrix-reseller" || v.CanResume || v.ResumeBlocked == "" {
		t.Fatalf("cascade view: %+v", v)
	}
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusConflict, errCodeSuspensionCauseReseller)

	// The suspended reseller sees only the suspended screen.
	wantStatus(t, f.as(t, 9702, http.MethodGet, "/api/v1/domains", ""), http.StatusForbidden, errCodeSubscriptionSuspended)

	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if f.status(t, 9801) != "active" || f.status(t, 9802) != "active" {
		t.Fatal("reseller resume did not resume its subscriptions")
	}
	if f.status(t, 9806) != "suspended" {
		t.Fatal("a customer suspended by hand was resumed with its reseller")
	}
	if f.native.siteExpiry["customer.matrix.test"] != "" {
		t.Fatal("cascade resume did not restore the site account")
	}
}

func TestSuspensionEndDateSuspendsOncePerDate(t *testing.T) {
	f := newSuspensionFixture(t)
	ctx := context.Background()
	// 052's ends_at is NULL: it never ends, nothing expires.
	if err := f.panel.checkSubscriptionExpiry(ctx); err != nil {
		t.Fatal(err)
	}
	if f.status(t, 9802) != "active" {
		t.Fatal("expired without an end date")
	}
	f.exec(t, `UPDATE subscriptions SET ends_at = '2026-01-01 00:00:00' WHERE id = 9802`)
	f.exec(t, `UPDATE subscriptions SET ends_at = '2999-01-01 00:00:00' WHERE id = 9803`)
	if err := f.panel.checkSubscriptionExpiry(ctx); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.status(t, 9802) != "suspended" || f.status(t, 9803) != "active" {
		t.Fatalf("expiry: %s %s", f.status(t, 9802), f.status(t, 9803))
	}
	v := f.view(t, 9802)
	if !v.Causes.Expired || v.Operation.Trigger != suspensionTriggerExpired || v.EndsAt == nil || !v.CanResume {
		t.Fatalf("expired view: %+v", v)
	}
	// An operator resumes: the same end date does not suspend it again.
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/subscriptions/9802/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)
	if err := f.panel.checkSubscriptionExpiry(ctx); err != nil {
		t.Fatal(err)
	}
	if f.status(t, 9802) != "active" {
		t.Fatal("the waived end date suspended it again")
	}
	// A new end date that passes suspends again.
	f.exec(t, `UPDATE subscriptions SET ends_at = '2026-06-01 00:00:00' WHERE id = 9802`)
	if err := f.panel.checkSubscriptionExpiry(ctx); err != nil {
		t.Fatal(err)
	}
	if f.status(t, 9802) != "suspended" {
		t.Fatal("a new passed end date did not suspend")
	}
	f.run(t)
	// Billing moves the end date into the future: the expiry cause clears.
	f.exec(t, `UPDATE subscriptions SET ends_at = '2999-01-01 00:00:00' WHERE id = 9802`)
	if err := f.panel.checkSubscriptionExpiry(ctx); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.status(t, 9802) != "active" {
		t.Fatal("an extended end date did not resume")
	}
}

func TestSuspensionPermissions(t *testing.T) {
	f := newSuspensionFixture(t)
	sub := "/api/v1/subscriptions/9802/suspension"
	outsider := "/api/v1/subscriptions/9803/suspension"
	// Reseller: its own customer yes, the administrator's customer no.
	wantStatus(t, f.as(t, 9702, http.MethodGet, sub, ""), http.StatusOK, "")
	wantStatus(t, f.as(t, 9702, http.MethodGet, outsider, ""), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9702, http.MethodPost, outsider, `{"action":"suspend","reason":"x"}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9702, http.MethodGet, "/api/v1/users/9704/suspension", ""), http.StatusNotFound, "")
	// Customer: never its own suspension.
	wantStatus(t, f.as(t, 9703, http.MethodGet, sub, ""), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9703, http.MethodPost, sub, `{"action":"resume"}`), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, "/api/v1/users/9703/suspension", ""), http.StatusForbidden, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, suspensionDeletablePath, ""), http.StatusForbidden, "")
	// Team member: refused by the middleware on every suspension route.
	for _, path := range []string{sub, "/api/v1/users/9703/suspension", suspensionDeletablePath} {
		wantStatus(t, f.send(t, http.MethodGet, path, f.memberToken, ""), http.StatusForbidden, errCodeAdditionalUserScope)
	}
	// Admin: anyone but an administrator.
	wantStatus(t, f.as(t, 9701, http.MethodGet, outsider, ""), http.StatusOK, "")
	wantStatus(t, f.as(t, 9701, http.MethodGet, "/api/v1/users/9701/suspension", ""), http.StatusNotFound, "")
	wantStatus(t, f.as(t, 9701, http.MethodDelete, sub, ""), http.StatusMethodNotAllowed, "")

	// Deletable: the administrator sees every suspended subscription, the
	// reseller only its customers'.
	wantStatus(t, f.as(t, 9701, http.MethodPost, sub, `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	wantStatus(t, f.as(t, 9701, http.MethodPost, outsider, `{"action":"suspend","reason":"r"}`), http.StatusAccepted, "")
	read := func(user int) []deletableEntry {
		rec := f.as(t, user, http.MethodGet, suspensionDeletablePath, "")
		wantStatus(t, rec, http.StatusOK, "")
		var body struct {
			Subscriptions []deletableEntry `json:"subscriptions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Subscriptions
	}
	if got := read(9701); len(got) != 2 {
		t.Fatalf("admin deletable: %+v", got)
	}
	got := read(9702)
	if len(got) != 1 || got[0].SubscriptionID != 9802 || got[0].Contents.Sites != 1 || got[0].Contents.Mailboxes != 1 ||
		got[0].Contents.DatabaseUsers != 3 || got[0].Contents.VPNPeers != 1 || got[0].DeleteHint == "" || got[0].Reseller != "matrix-reseller" {
		t.Fatalf("reseller deletable: %+v", got)
	}
}

func TestSuspensionOwnerSeesOnlyTheSuspendedScreen(t *testing.T) {
	f := newSuspensionFixture(t)
	wantStatus(t, f.as(t, 9703, http.MethodGet, "/api/v1/domains", ""), http.StatusNoContent, "")
	wantStatus(t, f.as(t, 9702, http.MethodPost, "/api/v1/users/9703/suspension", `{"action":"suspend","reason":"invoice 42 unpaid"}`), http.StatusAccepted, "")
	for _, token := range []string{f.tokens[9703], f.memberToken} {
		wantStatus(t, f.send(t, http.MethodGet, "/api/v1/domains", token, ""), http.StatusForbidden, errCodeSubscriptionSuspended)
		rec := f.send(t, http.MethodGet, "/api/v1/auth/me", token, "")
		wantStatus(t, rec, http.StatusOK, "")
		var me struct {
			Suspended *meSuspension `json:"suspended"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
			t.Fatal(err)
		}
		if me.Suspended == nil || me.Suspended.Kind != "subscription" || me.Suspended.Reason != "invoice 42 unpaid" ||
			me.Suspended.Contact.Kind != "reseller" || me.Suspended.Contact.Name != "matrix-reseller" {
			t.Fatalf("me: %s", rec.Body.String())
		}
	}
	// The reseller still manages it; other accounts are untouched.
	wantStatus(t, f.as(t, 9702, http.MethodGet, "/api/v1/domains", ""), http.StatusNoContent, "")
	wantStatus(t, f.as(t, 9704, http.MethodGet, "/api/v1/domains", ""), http.StatusNoContent, "")
	// Nothing new can be added to a suspended subscription.
	if err := f.panel.checkSubscriptionQuota(context.Background(), 9802, quotaMail); !errors.Is(err, errSubscriptionSuspendedCreate) {
		t.Fatalf("create gate: %v", err)
	}
	// The opposite action waits until the running operation ends.
	wantStatus(t, f.as(t, 9702, http.MethodPost, "/api/v1/users/9703/suspension", `{"action":"resume"}`), http.StatusConflict, errCodeSuspensionBusy)
	f.run(t)
	wantStatus(t, f.as(t, 9702, http.MethodPost, "/api/v1/users/9703/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	wantStatus(t, f.as(t, 9703, http.MethodGet, "/api/v1/domains", ""), http.StatusNoContent, "")
}

func TestSuspendedSiteRendersTheSuspendedVhost(t *testing.T) {
	f := newSuspensionFixture(t)
	f.exec(t, `UPDATE sites SET php_fpm_socket = '/run/php/site.sock' WHERE domain_id = 9902`)
	req, err := f.panel.buildVhostRequest(context.Background(), 9902, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Suspended {
		t.Fatal("an active site rendered suspended")
	}
	f.exec(t, `UPDATE subscriptions SET status = 'suspended' WHERE id = 9802`)
	req, err = f.panel.buildVhostRequest(context.Background(), 9902, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !req.Suspended {
		t.Fatal("a suspended subscription's site did not render suspended")
	}
}

// A customer with two subscriptions, one suspended: the suspended one's
// domains are refused after ownership is proven; a stranger still gets 404.
func TestSuspendedSubscriptionDomainsAreRefusedToTheirOwner(t *testing.T) {
	f := newSuspensionFixture(t)
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, status) VALUES (9807, 9703, 'second', 'active')`)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9907, 9807, 'second.matrix.test', 'active')`)
	f.exec(t, `UPDATE subscriptions SET status = 'suspended' WHERE id = 9802`)
	check := func(token string, domainID int) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/domains/"+strconv.Itoa(domainID)+"/logs", nil)
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		rec := httptest.NewRecorder()
		f.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if f.panel.authorizeDomainSubroute(w, r, domainID, "logs") {
				w.WriteHeader(http.StatusNoContent)
			}
		})).ServeHTTP(rec, r)
		if rec.Code == http.StatusForbidden && !strings.Contains(rec.Body.String(), errCodeSubscriptionSuspended) {
			t.Fatalf("403 without the suspension code: %s", rec.Body.String())
		}
		return rec.Code
	}
	if got := check(f.tokens[9703], 9902); got != http.StatusForbidden {
		t.Fatalf("owner on the suspended domain: %d", got)
	}
	if got := check(f.tokens[9703], 9907); got != http.StatusNoContent {
		t.Fatalf("owner on the active domain: %d", got)
	}
	if got := check(f.tokens[9704], 9902); got != http.StatusNotFound {
		t.Fatalf("a stranger learned about the suspension: %d", got)
	}
	if got := check(f.tokens[9702], 9902); got != http.StatusNoContent {
		t.Fatalf("the reseller cannot manage it: %d", got)
	}
}

// One read of subscriptions tells the state: suspension mirrors its cause
// into 052's suspend_reason and suspended_at, and clears both on resume.
func TestSuspensionMirrorsReasonIntoSubscriptions(t *testing.T) {
	f := newSuspensionFixture(t)
	read := func(sub int) (status, reason, since string) {
		t.Helper()
		var r, s sql.NullString
		if err := f.panel.db.GetDB().QueryRow(`SELECT status, suspend_reason, suspended_at FROM subscriptions WHERE id = ?`, sub).Scan(&status, &r, &s); err != nil {
			t.Fatal(err)
		}
		return status, r.String, s.String
	}
	path := "/api/v1/subscriptions/9803/suspension"
	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"suspend","reason":"unpaid"}`), http.StatusAccepted, "")
	if status, reason, since := read(9803); status != "suspended" || reason != suspendReasonManual || since == "" {
		t.Fatalf("manual: %s %q %q", status, reason, since)
	}
	if l, err := readSubscriptionLifecycle(context.Background(), f.panel.db.GetDB(), 9803); err != nil || l.SuspendReason != suspendReasonManual || l.SuspendedAt == nil {
		t.Fatalf("lifecycle: %+v %v", l, err)
	}
	f.run(t)
	wantStatus(t, f.as(t, 9701, http.MethodPost, path, `{"action":"resume"}`), http.StatusAccepted, "")
	if status, reason, since := read(9803); status != "active" || reason != "" || since != "" {
		t.Fatalf("resumed: %s %q %q", status, reason, since)
	}
	f.run(t)

	// The reseller's suspension cascades as manual (052 has no reseller value).
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"suspend","reason":"reseller unpaid"}`), http.StatusAccepted, "")
	if status, reason, since := read(9802); status != "suspended" || reason != suspendReasonManual || since == "" {
		t.Fatalf("cascade: %s %q %q", status, reason, since)
	}
	f.run(t)
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"resume"}`), http.StatusAccepted, "")
	f.run(t)

	// A passed end date reads as expired.
	f.exec(t, `UPDATE subscriptions SET ends_at = '2026-01-01 00:00:00' WHERE id = 9803`)
	if err := f.panel.checkSubscriptionExpiry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status, reason, since := read(9803); status != "suspended" || reason != suspendReasonExpired || since == "" {
		t.Fatalf("expired: %s %q %q", status, reason, since)
	}
	if got := suspendReasonFor(suspensionCauses{Manual: true, Expired: true}); got != suspendReasonManual {
		t.Fatalf("manual and expired: %q", got)
	}
}

// An account created under a suspended reseller starts suspended with the
// reseller as cause; a transfer out of that reseller's field clears it.
func TestNewAccountUnderSuspendedResellerStartsSuspended(t *testing.T) {
	f := newSuspensionFixture(t)
	ctx := context.Background()
	wantStatus(t, f.as(t, 9701, http.MethodPost, "/api/v1/users/9702/suspension", `{"action":"suspend","reason":"reseller unpaid"}`), http.StatusAccepted, "")
	f.run(t)

	// What finishNewAccount does for a new customer of the reseller.
	f.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES (9790, 'late-customer', 'x', 'late@example.test', 'customer', 9702, 'active')`)
	tx, err := f.panel.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	subID, err := insertBareSubscription(ctx, tx, 9790, builtinUnlimitedName, planLimits{})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := f.panel.syncNewAccountResellerSuspension(ctx, tx, 9790, 9702)
	if err != nil || !changed {
		t.Fatalf("sync: %v %v", changed, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if f.status(t, subID) != "suspended" {
		t.Fatal("a new account under a suspended reseller started active")
	}
	row, err := loadSuspensionRow(ctx, f.panel.db.GetDB(), subID)
	if err != nil || row.Causes.ResellerID != 9702 || row.Causes.Manual || row.Reason != "reseller unpaid" {
		t.Fatalf("cause: %+v %v", row, err)
	}
	f.run(t)

	// A customer of an active parent (the admin) starts active.
	f.exec(t, `INSERT INTO users (id, username, password_hash, email, role, parent_id, status) VALUES (9791, 'free-customer', 'x', 'free@example.test', 'customer', 9701, 'active')`)
	free, err := insertBareSubscription(ctx, f.panel.db.GetDB(), 9791, builtinUnlimitedName, planLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.panel.syncResellerSuspensionCause(ctx, free, 9701); err != nil || f.status(t, free) != "active" {
		t.Fatalf("free customer: %s %v", f.status(t, free), err)
	}

	// Transferred to the admin's customer: the reseller cause goes with the
	// old field and the subscription resumes.
	f.exec(t, `UPDATE subscriptions SET owner_id = 9791 WHERE id = ?`, subID)
	if err := f.panel.syncResellerSuspensionCause(ctx, subID, 9701); err != nil {
		t.Fatal(err)
	}
	if f.status(t, subID) != "active" {
		t.Fatal("moved out of the suspended reseller's field but still suspended")
	}
	// And back into it: suspended again.
	f.exec(t, `UPDATE subscriptions SET owner_id = 9790 WHERE id = ?`, subID)
	if err := f.panel.syncResellerSuspensionCause(ctx, subID, 9701); err != nil || f.status(t, subID) != "suspended" {
		t.Fatalf("moved into the suspended reseller's field: %s %v", f.status(t, subID), err)
	}
}
