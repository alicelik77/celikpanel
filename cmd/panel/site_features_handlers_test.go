package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// applyRecorder replaces the vhost apply: it records the request the Agent
// would have received and answers with the next error of its script.
type applyRecorder struct {
	requests []applyVhostRPCRequest
	script   []error
}

func (a *applyRecorder) install(t *testing.T, p *Panel) {
	t.Helper()
	saved := siteFeatureApplyVhost
	siteFeatureApplyVhost = func(p *Panel, ctx context.Context, domainID int) error {
		req, err := p.buildVhostRequest(ctx, domainID, nil)
		if err != nil {
			return err
		}
		a.requests = append(a.requests, req)
		if len(a.script) == 0 {
			return nil
		}
		next := a.script[0]
		a.script = a.script[1:]
		return next
	}
	t.Cleanup(func() { siteFeatureApplyVhost = saved })
}

func newSiteFeatureFixture(t *testing.T) (*Panel, int, *Caller, *applyRecorder) {
	t.Helper()
	p, domainID := newDomainAliasFixture(t)
	var ownerID int
	if err := p.db.GetDB().QueryRow(`
		SELECT s.owner_id FROM subscriptions s JOIN domains d ON d.subscription_id = s.id WHERE d.id = ?`,
		domainID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	owner := &Caller{ID: ownerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: ownerID}
	recorder := &applyRecorder{}
	recorder.install(t, p)
	return p, domainID, owner, recorder
}

func featureRequest(p *Panel, domainID int, caller *Caller, method, route, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1/domains/"+featureNum(domainID)+"/"+route, strings.NewReader(body))
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	p.handleDomainSubroute(recorder, request)
	return recorder
}

func featureNum(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func featureCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

func readFeatureView(t *testing.T, p *Panel, domainID int, caller *Caller) siteFeatureView {
	t.Helper()
	recorder := featureRequest(p, domainID, caller, http.MethodGet, "protected-dirs", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var view siteFeatureView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func TestProtectedFolderIsSavedAppliedAndNeverShowsAHash(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	created := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private/","realm":"Staff only","username":"alice","password":"correct-horse-battery"}`)
	if created.Code != http.StatusOK {
		t.Fatalf("POST status = %d; body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "$2y$") || strings.Contains(created.Body.String(), "correct-horse") {
		t.Fatalf("the answer carries a hash or a password: %s", created.Body.String())
	}
	if len(applied.requests) != 1 {
		t.Fatalf("vhost applied %d times, want once", len(applied.requests))
	}
	dirs := applied.requests[0].ProtectedDirs
	if len(dirs) != 1 || dirs[0].Path != "/private" || dirs[0].Realm != "Staff only" ||
		len(dirs[0].Users) != 1 || dirs[0].Users[0].Username != "alice" ||
		!strings.HasPrefix(dirs[0].Users[0].PasswordHash, "$2y$10$") || len(dirs[0].Users[0].PasswordHash) != 60 {
		t.Fatalf("the Agent was sent %#v", dirs)
	}
	view := readFeatureView(t, p, domainID, owner)
	if len(view.Dirs) != 1 || view.Dirs[0].Path != "/private" || len(view.Dirs[0].Users) != 1 || !view.ProtectAvailable {
		t.Fatalf("view = %#v", view)
	}

	// A second user, a changed password, and the last user cannot go.
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs/users",
		`{"dir_id":`+featureNum(view.Dirs[0].ID)+`,"username":"bob","password":"another-long-password"}`); r.Code != http.StatusOK {
		t.Fatalf("add user status = %d; body=%s", r.Code, r.Body.String())
	}
	before := applied.requests[len(applied.requests)-1].ProtectedDirs[0].Users
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs/users",
		`{"dir_id":`+featureNum(view.Dirs[0].ID)+`,"username":"alice","password":"a-brand-new-password"}`); r.Code != http.StatusOK {
		t.Fatalf("change password status = %d; body=%s", r.Code, r.Body.String())
	}
	after := applied.requests[len(applied.requests)-1].ProtectedDirs[0].Users
	if len(after) != 2 || after[0].PasswordHash == before[0].PasswordHash {
		t.Fatalf("password was not changed: %#v -> %#v", before, after)
	}
	remove := func(user string) *httptest.ResponseRecorder {
		return featureRequest(p, domainID, owner, http.MethodDelete,
			"protected-dirs/users?id="+featureNum(view.Dirs[0].ID)+"&user="+user, "")
	}
	if r := remove("bob"); r.Code != http.StatusOK {
		t.Fatalf("remove user status = %d; body=%s", r.Code, r.Body.String())
	}
	if r := remove("alice"); r.Code != http.StatusConflict || featureCode(t, r) != errCodeSiteFeatureLastUser {
		t.Fatalf("removing the last user: status %d code %q", r.Code, featureCode(t, r))
	}
	if r := featureRequest(p, domainID, owner, http.MethodDelete, "protected-dirs?id="+featureNum(view.Dirs[0].ID), ""); r.Code != http.StatusOK {
		t.Fatalf("remove folder status = %d; body=%s", r.Code, r.Body.String())
	}
	if last := applied.requests[len(applied.requests)-1]; len(last.ProtectedDirs) != 0 {
		t.Fatalf("a removed folder is still sent to the Agent: %#v", last.ProtectedDirs)
	}
}

func TestEveryRenderOfTheSiteCarriesItsFoldersAndRedirects(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private","username":"alice","password":"correct-horse-battery"}`); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	if r := featureRequest(p, domainID, owner, http.MethodPost, "redirects",
		`{"match":"prefix","source":"/blog","target":"https://news.example.org/","code":308,"keep_query":true}`); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	// A render from another flow (SSL, startup, certificate renewal) is built by
	// the same function.
	request, err := p.buildVhostRequest(context.Background(), domainID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.ProtectedDirs) != 1 || len(request.Redirects) != 1 ||
		request.Redirects[0] != (transport.SiteRedirectRequest{Match: "prefix", Source: "/blog", Target: "https://news.example.org/", Code: 308, KeepQuery: true}) {
		t.Fatalf("request = %#v", request)
	}
}

func TestFailedApplyPutsTheSavedStateAndTheVhostBack(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	applied.script = []error{errors.New("nginx validation failed")}
	r := featureRequest(p, domainID, owner, http.MethodPost, "redirects",
		`{"match":"exact","source":"/old","target":"/new","code":301}`)
	if r.Code == http.StatusOK {
		t.Fatalf("a failed apply was reported as done: %s", r.Body.String())
	}
	if len(applied.requests) != 2 {
		t.Fatalf("vhost applied %d times, want the change and its undoing", len(applied.requests))
	}
	if len(applied.requests[0].Redirects) != 1 || len(applied.requests[1].Redirects) != 0 {
		t.Fatalf("the undoing did not send the previous state: %#v", applied.requests)
	}
	var count int
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM site_redirects`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("redirect rows after a failed apply = %d, %v", count, err)
	}
}

func TestOwnerEditedVhostKeepsTheChangeOutAndSaysSo(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	applied.script = []error{&siteFileHeldError{Domain: "ssl-state.example", File: transport.SiteFileResult{
		Kind: transport.SiteFileKindNginxVhost, State: transport.SiteFileOwnerEdited, Outcome: transport.SiteFileOutcomeKept,
	}}}
	r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private","username":"alice","password":"correct-horse-battery"}`)
	if r.Code != http.StatusConflict || featureCode(t, r) != "SITE_CONFIG_OWNER_EDITED" {
		t.Fatalf("status %d code %q; body=%s", r.Code, featureCode(t, r), r.Body.String())
	}
	if len(applied.requests) != 1 {
		t.Fatalf("a held file was applied again: %d applies", len(applied.requests))
	}
	if view := readFeatureView(t, p, domainID, owner); len(view.Dirs) != 0 {
		t.Fatalf("the screen shows a folder nginx does not serve: %#v", view.Dirs)
	}
}

func TestPasswordFileTheOwnerChangedIsReportedWithItsNextStep(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	applied.script = []error{errors.New("the password file /etc/nginx/celikpanel-auth.d/x/protect-1.htpasswd is not CelikPanel's unchanged text (owner_edited); it was kept as it is and this change was not applied")}
	r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private","username":"alice","password":"correct-horse-battery"}`)
	if r.Code != http.StatusConflict || featureCode(t, r) != errCodeSiteAuthFileKept {
		t.Fatalf("status %d code %q; body=%s", r.Code, featureCode(t, r), r.Body.String())
	}
	if len(applied.requests) != 1 {
		t.Fatalf("applies = %d, want 1 (a kept file is not applied again)", len(applied.requests))
	}
}

func TestSiteFeatureInputThatCouldBecomeNginxSyntaxNeverReachesTheAgent(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	bodies := map[string][]string{
		"protected-dirs": {
			`{"path":"/a;b","username":"alice","password":"correct-horse-battery"}`,
			`{"path":"/a\nb","username":"alice","password":"correct-horse-battery"}`,
			`{"path":"/a{b}","username":"alice","password":"correct-horse-battery"}`,
			`{"path":"/ok","realm":"x\"; return 200; #","username":"alice","password":"correct-horse-battery"}`,
			`{"path":"/ok","username":"alice:root","password":"correct-horse-battery"}`,
			`{"path":"/ok","username":"alice\nroot","password":"correct-horse-battery"}`,
			`{"path":"/ok","username":"alice","password":"short"}`,
			`{"path":"/ok","username":"alice","password":"` + strings.Repeat("x", 73) + `"}`,
			`{"path":"/","username":"alice","password":"correct-horse-battery"}`,
			`{"path":"/.well-known","username":"alice","password":"correct-horse-battery"}`,
		},
		"redirects": {
			`{"match":"exact","source":"/a;return 200","target":"/b","code":301}`,
			`{"match":"exact","source":"/a","target":"/b;return 200","code":301}`,
			`{"match":"exact","source":"/a","target":"https://x.example/\nSet-Cookie: a=b","code":301}`,
			`{"match":"exact","source":"/a","target":"/b{","code":301}`,
			`{"match":"exact","source":"/a","target":"//evil.example","code":301}`,
			`{"match":"exact","source":"/a","target":"/b c","code":301}`,
			`{"match":"exact","source":"/a","target":"/b$host","code":301}`,
			`{"match":"exact","source":"/a","target":"/b","code":200}`,
			`{"match":"regex","source":"/a","target":"/b","code":301}`,
			`{"match":"exact","source":"/a","target":"/a","code":301}`,
		},
	}
	for route, list := range bodies {
		for _, body := range list {
			r := featureRequest(p, domainID, owner, http.MethodPost, route, body)
			if r.Code != http.StatusBadRequest || featureCode(t, r) != errCodeSiteFeatureInvalid {
				t.Errorf("%s %s: status %d code %q", route, body, r.Code, featureCode(t, r))
			}
		}
	}
	if len(applied.requests) != 0 {
		t.Fatalf("refused input reached the apply %d times", len(applied.requests))
	}
}

func TestSiteFeatureDuplicatesLimitsAndUnavailableTypes(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	add := `{"path":"/private","username":"alice","password":"correct-horse-battery"}`
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs", add); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs", add); r.Code != http.StatusConflict || featureCode(t, r) != errCodeSiteFeatureDuplicate {
		t.Fatalf("duplicate folder: status %d code %q", r.Code, featureCode(t, r))
	}
	// A redirect on the protected folder's own path would be a second location of the same name.
	if r := featureRequest(p, domainID, owner, http.MethodPost, "redirects",
		`{"match":"exact","source":"/private","target":"/x","code":301}`); r.Code != http.StatusBadRequest {
		t.Fatalf("redirect on a protected path: status %d", r.Code)
	}
	redirect := `{"match":"exact","source":"/old","target":"/new","code":301}`
	if r := featureRequest(p, domainID, owner, http.MethodPost, "redirects", redirect); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	if r := featureRequest(p, domainID, owner, http.MethodPost, "redirects", redirect); r.Code != http.StatusConflict || featureCode(t, r) != errCodeSiteFeatureDuplicate {
		t.Fatalf("duplicate redirect: status %d code %q", r.Code, featureCode(t, r))
	}
	if r := featureRequest(p, domainID, owner, http.MethodDelete, "redirects?id=999", ""); r.Code != http.StatusNotFound || featureCode(t, r) != errCodeSiteFeatureNotFound {
		t.Fatalf("remove of a missing redirect: status %d code %q", r.Code, featureCode(t, r))
	}

	// A proxy site has no protected folders; a forwarding site has no redirects.
	if _, err := p.db.GetDB().Exec(`UPDATE sites SET project_type = 'proxy', forward_to = 'http://127.0.0.1:3000' WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/other","username":"alice","password":"correct-horse-battery"}`); r.Code != http.StatusConflict || featureCode(t, r) != errCodeSiteFeatureUnavail {
		t.Fatalf("protected folder on a proxy site: status %d code %q", r.Code, featureCode(t, r))
	}
	view := readFeatureView(t, p, domainID, owner)
	if view.ProtectAvailable || !view.RedirectsAvailable {
		t.Fatalf("view of a proxy site = %#v", view)
	}
}

func TestChangingTheSiteTypeIsRefusedWhileItsFoldersWouldHaveNoHome(t *testing.T) {
	p, domainID, owner, _ := newSiteFeatureFixture(t)
	if r := featureRequest(p, domainID, owner, http.MethodPost, "protected-dirs",
		`{"path":"/private","username":"alice","password":"correct-horse-battery"}`); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	for projectType, wantRefused := range map[string]bool{"php": false, "static": false, "node": true, "proxy": true, "forwarding": true} {
		message, err := p.refuseProjectTypeWithSiteFeatures(context.Background(), domainID, projectType)
		if err != nil {
			t.Fatal(err)
		}
		if (message != "") != wantRefused {
			t.Errorf("%s: refusal %q, want refused=%v", projectType, message, wantRefused)
		}
	}
	if r := featureRequest(p, domainID, owner, http.MethodPost, "redirects",
		`{"match":"exact","source":"/old","target":"/new","code":301}`); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	if _, err := p.db.GetDB().Exec(`DELETE FROM site_protected_dirs`); err != nil {
		t.Fatal(err)
	}
	if message, _ := p.refuseProjectTypeWithSiteFeatures(context.Background(), domainID, "forwarding"); !strings.Contains(message, "Redirects") {
		t.Fatalf("refusal of forwarding = %q", message)
	}
}

func TestSiteFeatureRoutesBelongToTheDomainsCustomerAndNoOneElse(t *testing.T) {
	p, domainID, owner, applied := newSiteFeatureFixture(t)
	stranger := &Caller{ID: 7999, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: 7999}
	for _, call := range []struct{ method, route, body string }{
		{http.MethodGet, "protected-dirs", ""},
		{http.MethodPost, "protected-dirs", `{"path":"/private","username":"alice","password":"correct-horse-battery"}`},
		{http.MethodDelete, "protected-dirs?id=1", ""},
		{http.MethodPost, "protected-dirs/users", `{"dir_id":1,"username":"bob","password":"correct-horse-battery"}`},
		{http.MethodDelete, "protected-dirs/users?id=1&user=bob", ""},
		{http.MethodGet, "redirects", ""},
		{http.MethodPost, "redirects", `{"match":"exact","source":"/old","target":"/new","code":301}`},
		{http.MethodDelete, "redirects?id=1", ""},
	} {
		if r := featureRequest(p, domainID, stranger, call.method, call.route, call.body); r.Code != http.StatusNotFound {
			t.Errorf("stranger %s %s: status %d, want 404", call.method, call.route, r.Code)
		}
	}
	if len(applied.requests) != 0 {
		t.Fatal("a stranger's request reached the apply")
	}
	// Methods outside each route's set are refused before authorization.
	if r := featureRequest(p, domainID, owner, http.MethodPut, "redirects", "{}"); r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT redirects: status %d, want 405", r.Code)
	}
	if r := featureRequest(p, domainID, owner, http.MethodGet, "protected-dirs/users", ""); r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET protected-dirs/users: status %d, want 405", r.Code)
	}
}

func TestAdditionalUsersNeedTheFilesCapabilityForFoldersAndRedirects(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	if _, err := fixture.panel.db.GetDB().Exec(`
		INSERT INTO sites (domain_id, document_root) VALUES (?, '/srv/matrix/customer/public_html')`,
		authzMatrixCustomerDomainID); err != nil {
		t.Fatal(err)
	}
	applied := &applyRecorder{}
	applied.install(t, fixture.panel)
	caller := teamMemberAuthzTestCaller()
	do := func(method, route, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/api/v1/domains/9902/"+route, strings.NewReader(body))
		request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
		recorder := httptest.NewRecorder()
		fixture.panel.handleDomainSubroute(recorder, request)
		return recorder
	}
	add := `{"path":"/private","username":"alice","password":"correct-horse-battery"}`

	// No grant at all is indistinguishable from a domain that is not theirs.
	if r := do(http.MethodGet, "redirects", ""); r.Code != http.StatusNotFound {
		t.Fatalf("no grant: status %d, want 404", r.Code)
	}
	// A different capability does not open it.
	grantTeamMemberDomainCapability(t, &fixture, authzMatrixCustomerDomainID, core.TeamCapabilityMail, core.TeamPermissionManage)
	if r := do(http.MethodGet, "protected-dirs", ""); r.Code != http.StatusForbidden {
		t.Fatalf("mail capability: status %d, want 403", r.Code)
	}
	// Files view reads but does not change.
	grantTeamMemberDomainCapability(t, &fixture, authzMatrixCustomerDomainID, core.TeamCapabilityFiles, core.TeamPermissionView)
	if r := do(http.MethodGet, "protected-dirs", ""); r.Code != http.StatusOK {
		t.Fatalf("files view GET: status %d; body=%s", r.Code, r.Body.String())
	}
	for _, call := range []struct{ method, route, body string }{
		{http.MethodPost, "protected-dirs", add},
		{http.MethodDelete, "protected-dirs?id=1", ""},
		{http.MethodPost, "protected-dirs/users", `{"dir_id":1,"username":"bob","password":"correct-horse-battery"}`},
		{http.MethodDelete, "protected-dirs/users?id=1&user=bob", ""},
		{http.MethodPost, "redirects", `{"match":"exact","source":"/old","target":"/new","code":301}`},
		{http.MethodDelete, "redirects?id=1", ""},
	} {
		if r := do(call.method, call.route, call.body); r.Code != http.StatusForbidden {
			t.Errorf("files view %s %s: status %d, want 403", call.method, call.route, r.Code)
		}
	}
	if len(applied.requests) != 0 {
		t.Fatal("a view-only user reached the apply")
	}
	// Files manage changes.
	if _, err := fixture.panel.db.GetDB().Exec(`
		UPDATE additional_user_domain_permissions SET mode = 'manage'
		WHERE capability = 'files' AND domain_id = ?`, authzMatrixCustomerDomainID); err != nil {
		t.Fatal(err)
	}
	if r := do(http.MethodPost, "protected-dirs", add); r.Code != http.StatusOK {
		t.Fatalf("files manage POST: status %d; body=%s", r.Code, r.Body.String())
	}
	if len(applied.requests) != 1 {
		t.Fatalf("applies = %d, want 1", len(applied.requests))
	}
	// A domain of someone else stays hidden.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/9903/redirects", nil)
	request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
	recorder := httptest.NewRecorder()
	fixture.panel.handleDomainSubroute(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("foreign domain: status %d, want 404", recorder.Code)
	}
}
