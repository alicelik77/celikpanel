package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Remote API tokens (alpha.128, G15). These tests drive the real
// authentication middleware. The route matrix sends every registered route,
// every domain sub-route the dispatcher knows (read from its source) and a
// set of unusual spellings, with every method, as a read and a manage token
// of the administrator, a reseller and a customer, and requires that exactly
// the routes of apiTokenExpectedRoutes - a list written here, independently
// of the product's own - reach a handler.
//
// Uzak API jetonları: rota matrisi her kayıtlı rotayı, her alan adı alt
// rotasını ve olağandışı yazımları her yöntemle her rol ve kapsamdan dener;
// yalnız burada bağımsız yazılmış listedeki rotalar işleyiciye ulaşır.

const apiTokenTestDomain = authzMatrixCustomerDomainID // 9902, the customer's

// apiTokenExpectedRoutes: what a token reaches, by scope ("read" routes are
// reached by both scopes, "manage" ones by a manage token only).
var apiTokenExpectedRoutes = map[string]string{
	"GET /api/v1/domains":                             "read",
	"POST /api/v1/domains/create":                     "manage",
	"DELETE /api/v1/domains/9902":                     "manage",
	"GET /api/v1/domains/9902/hosting":                "read",
	"GET /api/v1/domains/9902/usage":                  "read",
	"GET /api/v1/domains/9902/dns/records":            "read",
	"POST /api/v1/domains/9902/dns/records":           "manage",
	"PUT /api/v1/domains/9902/dns/records":            "manage",
	"DELETE /api/v1/domains/9902/dns/records":         "manage",
	"GET /api/v1/domains/9902/mail/accounts":          "read",
	"POST /api/v1/domains/9902/mail/accounts":         "manage",
	"PUT /api/v1/domains/9902/mail/accounts":          "manage",
	"DELETE /api/v1/domains/9902/mail/accounts":       "manage",
	"PUT /api/v1/domains/9902/mail/accounts/password": "manage",
	"GET /api/v1/domains/9902/databases":              "read",
	"POST /api/v1/domains/9902/databases":             "manage",
	"DELETE /api/v1/domains/9902/databases/5":         "manage",
	"GET /api/v1/domains/9902/backups":                "read",
	"POST /api/v1/domains/9902/backups":               "manage",
	"DELETE /api/v1/domains/9902/backups":             "manage",
}

var apiTokenTestMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

type apiTokenFixture struct {
	authzMatrixFixture
	memberSession string
	// minted: "admin/read" -> the full token.
	minted map[string]string
	rowIDs map[string]int64
}

var apiTokenFixtureOwners = map[string]int{
	"admin": authzMatrixAdminID, "reseller": authzMatrixResellerID, "customer": authzMatrixCustomerID,
}

func newAPITokenFixture(t *testing.T) *apiTokenFixture {
	t.Helper()
	base := newAuthzMatrixFixture(t)
	member := seedAdditionalUserSession(t, &base)
	f := &apiTokenFixture{authzMatrixFixture: base, memberSession: member, minted: map[string]string{}, rowIDs: map[string]int64{}}
	if err := f.panel.setSetting(context.Background(), settingRemoteAPIEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	for role, id := range apiTokenFixtureOwners {
		for _, scope := range []string{apiTokenScopeRead, apiTokenScopeManage} {
			full, rowID := mintTestAPIToken(t, f.panel, id, scope, time.Now().Add(24*time.Hour))
			f.minted[role+"/"+scope] = full
			f.rowIDs[role+"/"+scope] = rowID
		}
	}
	return f
}

// mintTestAPIToken stores a token the way createAPIToken does.
func mintTestAPIToken(t *testing.T, p *Panel, userID int, scope string, expires time.Time) (string, int64) {
	t.Helper()
	tokenID, secret, full, err := newAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	created := expires.Add(-48 * time.Hour).Unix()
	result, err := p.db.GetDB().Exec(`
		INSERT INTO api_tokens (user_id, name, token_id, secret_sha256, scope, auth_epoch, created_at, expires_at)
		SELECT id, 'test', ?, ?, ?, auth_epoch, ?, ? FROM users WHERE id = ?`,
		tokenID, hashAPITokenSecret(secret), scope, created, expires.Unix(), userID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return full, id
}

// unlimitedAPITokenGuards lifts the per-token rate so a matrix of thousands
// of requests is about routes, not the limit (the limit has its own test).
func unlimitedAPITokenGuards(p *Panel) {
	guards := newAPITokenGuards()
	guards.calls = newRateLimiter(1<<30, time.Minute)
	guards.failures = newRateLimiter(1<<30, time.Minute)
	apiTokenGuardsByPanel.Store(p, guards)
}

type apiTokenProbe struct {
	reached bool
	caller  *Caller
}

func (probe *apiTokenProbe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.reached = true
		probe.caller = currentCaller(r)
		w.WriteHeader(http.StatusNoContent)
	})
}

func tokenRequest(method, target, token string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func apiErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

// domainSubrouteKeys reads every literal key of matchDomainSubroute's switch
// from main.go, so a sub-route added later joins the matrix by itself.
func domainSubrouteKeys(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "matchDomainSubroute" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if value, err := strconv.Unquote(lit.Value); err == nil {
						keys[value] = true
					}
				}
			}
			return true
		})
	}
	if len(keys) < 60 {
		t.Fatalf("read only %d domain sub-route keys from matchDomainSubroute", len(keys))
	}
	// The forms built from segments (the default branch).
	hex := strings.Repeat("ab", 16)
	for _, key := range []string{"aliases/www.example.test", "apps/" + hex, "apps/" + hex + "/retry", "apps/" + hex + "/credentials", "databases/5"} {
		keys[key] = true
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// apiTokenCorpus is every path the matrix sends.
func apiTokenCorpus(t *testing.T) []string {
	t.Helper()
	paths := map[string]bool{}
	for _, route := range registeredPanelRoutes(t) {
		switch {
		case route == "/":
			for _, p := range []string{"/", "/index.html", "/assets/app.js", "/api", "/api/v1/unknown", "/dbtool/x"} {
				paths[p] = true
			}
		case route == "/api/v1/domains/":
			// expanded below
		case strings.HasSuffix(route, "/"):
			paths[route+"1"] = true
			paths[route+"1/x"] = true
			paths[route+strconv.Itoa(apiTokenTestDomain)] = true
		default:
			paths[route] = true
			paths[route+"/"] = true
		}
	}
	domain := "/api/v1/domains/" + strconv.Itoa(apiTokenTestDomain)
	for _, key := range domainSubrouteKeys(t) {
		if key == "" {
			paths[domain] = true
			continue
		}
		paths[domain+"/"+key] = true
		paths[domain+"/"+key+"/"] = true
	}
	// Unusual spellings of listed routes: none may reach a handler.
	for _, p := range []string{
		"/api/v1/domains/",
		"/api/v1/domains/0/dns/records",
		"/api/v1/domains/-1/dns/records",
		"/api/v1/domains/abc/dns/records",
		"/api/v1/domains//9902/dns/records",
		"/api/v1/domains/9902//dns/records",
		"/api/v1/domains/9902/./dns/records",
		"/api/v1/domains/9902/../9903/dns/records",
		"/api/v1/domains/9902/dns/records/x",
		"/api/v1/domains/9902/databases/abc",
		"/api/v1/domains/9902/databases/5/x",
		"/api/v1/domains/9902/DNS/records",
		"/API/v1/domains",
		"/api/v1/domains/create/x",
		"/api/v1/domains/9902/%2e%2e/9903",
		"/api/v1/domains/9902/dns%2frecords",
		"//api/v1/domains",
	} {
		paths[p] = true
	}
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func TestAPITokenRouteMatrixReachesOnlyTheListedRoutes(t *testing.T) {
	f := newAPITokenFixture(t)
	unlimitedAPITokenGuards(f.panel)
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	corpus := apiTokenCorpus(t)
	if len(corpus) < 300 {
		t.Fatalf("the corpus has only %d paths", len(corpus))
	}
	reachedListed := map[string]bool{}
	sent := 0
	for _, path := range corpus {
		for _, method := range apiTokenTestMethods {
			want, listed := apiTokenExpectedRoutes[method+" "+path]
			for _, role := range []string{"admin", "reseller", "customer"} {
				for _, scope := range []string{apiTokenScopeRead, apiTokenScopeManage} {
					probe.reached, probe.caller = false, nil
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, tokenRequest(method, path, f.minted[role+"/"+scope]))
					sent++
					expect := listed && (want == apiTokenScopeRead || scope == apiTokenScopeManage)
					if probe.reached != expect {
						t.Errorf("%s %s as %s %s token: reached=%v, want %v (status %d %s)", method, path, role, scope,
							probe.reached, expect, recorder.Code, recorder.Body.String())
						continue
					}
					if expect {
						reachedListed[method+" "+path] = true
						c := probe.caller
						if c == nil || c.ID != apiTokenFixtureOwners[role] || c.APIToken == nil || c.APIToken.Scope != scope || c.ImpersonatorID != 0 {
							t.Errorf("%s %s as %s %s token: caller %+v", method, path, role, scope, c)
						}
						continue
					}
					if method == http.MethodOptions && strings.HasPrefix(path, "/api/") {
						// The preflight answer: no data, no handler.
						if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
							t.Errorf("OPTIONS %s: %d %q", path, recorder.Code, recorder.Body.String())
						}
						continue
					}
					code := apiErrorCode(t, recorder)
					wantCode := errCodeAPITokenRouteRefused
					if listed {
						wantCode = errCodeAPITokenScope
					}
					if recorder.Code != http.StatusForbidden || code != wantCode {
						t.Errorf("%s %s as %s %s token: %d %s, want 403 %s", method, path, role, scope, recorder.Code, code, wantCode)
					}
				}
			}
		}
	}
	for route := range apiTokenExpectedRoutes {
		if !reachedListed[route] {
			t.Errorf("the listed route %s was never reached; the corpus or the list is wrong", route)
		}
	}
	t.Logf("%d requests over %d paths", sent, len(corpus))
}

// The product's list and this test's list are the same list.
func TestAPITokenProductListMatchesTheTestList(t *testing.T) {
	product := map[string]string{}
	for key, route := range apiTokenExactRoutes {
		product[key] = map[bool]string{false: "read", true: "manage"}[route.write]
	}
	domain := "/api/v1/domains/" + strconv.Itoa(apiTokenTestDomain)
	for key, route := range apiTokenDomainRoutes {
		method, tail, _ := strings.Cut(key, " ")
		path := domain
		if tail == "databases/{id}" {
			path += "/databases/5"
		} else if tail != "" {
			path += "/" + tail
		}
		product[method+" "+path] = map[bool]string{false: "read", true: "manage"}[route.write]
		if route.write == (method == http.MethodGet) {
			t.Errorf("%s: a GET must read and every other method must change", key)
		}
	}
	if fmt.Sprint(product) != fmt.Sprint(apiTokenExpectedRoutes) {
		t.Fatalf("the product lists %v\nthe test expects %v", product, apiTokenExpectedRoutes)
	}
}

func TestAPITokenTeamMemberNeverHoldsAToken(t *testing.T) {
	f := newAPITokenFixture(t)
	unlimitedAPITokenGuards(f.panel)
	full, _ := mintTestAPIToken(t, f.panel, authzMatrixAdditionalUserID, apiTokenScopeManage, time.Now().Add(time.Hour))
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	for route := range apiTokenExpectedRoutes {
		method, path, _ := strings.Cut(route, " ")
		probe.reached = false
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(method, path, full))
		if probe.reached || recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenTeamMember {
			t.Errorf("%s as a team member's token: reached=%v %d %s", route, probe.reached, recorder.Code, recorder.Body.String())
		}
	}
	// A team member's session cannot reach the token screens either.
	mux := http.NewServeMux()
	mux.HandleFunc(apiTokensPath, f.panel.handleAPITokens)
	recorder := requestWithToken(f.panel.requireAuth(mux), http.MethodGet, apiTokensPath, f.memberSession)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("a team member read the token screen: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPITokenWithASessionCookieIsRefused(t *testing.T) {
	f := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	for _, cookie := range []*http.Cookie{
		{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]},
		{Name: sessionCookieName, Value: "not-a-session"},
		{Name: impersonatorCookieName, Value: f.tokens[authzMatrixAdminID]},
	} {
		probe.reached = false
		request := tokenRequest(http.MethodGet, "/api/v1/domains", f.minted["customer/read"])
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if probe.reached || recorder.Code != http.StatusBadRequest || apiErrorCode(t, recorder) != errCodeAPITokenWithSession {
			t.Errorf("cookie %s with a token: reached=%v %d %s", cookie.Name, probe.reached, recorder.Code, recorder.Body.String())
		}
	}
	// Two Authorization headers, a bearer among them, are refused.
	request := tokenRequest(http.MethodGet, "/api/v1/domains", f.minted["customer/read"])
	request.Header.Add("Authorization", "Basic dXNlcjpwYXNz")
	recorder := httptest.NewRecorder()
	probe.reached = false
	handler.ServeHTTP(recorder, request)
	if probe.reached || recorder.Code != http.StatusUnauthorized {
		t.Fatalf("two Authorization headers: reached=%v %d", probe.reached, recorder.Code)
	}
	// A proxy's Basic sign-in is not a token: the session path decides.
	request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
	recorder = httptest.NewRecorder()
	probe.reached = false
	handler.ServeHTTP(recorder, request)
	if !probe.reached {
		t.Fatalf("a session behind a Basic proxy sign-in was refused: %d %s", recorder.Code, recorder.Body.String())
	}
}

// CSRF: the origin check is skipped only for a token without a session
// cookie; every cookie session keeps it.
func TestAPITokenCSRFRulesForCookieSessionsAreUnchanged(t *testing.T) {
	f := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	handler := csrfProtect(f.panel.requireAuth(probe.handler()))
	send := func(build func(*http.Request)) (*httptest.ResponseRecorder, bool) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/9902/dns/records", nil)
		build(request)
		recorder := httptest.NewRecorder()
		probe.reached = false
		handler.ServeHTTP(recorder, request)
		return recorder, probe.reached
	}
	if recorder, reached := send(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.minted["customer/manage"]) }); !reached {
		t.Fatalf("a token request without an Origin was refused: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder, reached := send(func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
	}); reached || recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "cross-origin") {
		t.Fatalf("a cookie session without an Origin passed: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder, reached := send(func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
		r.Header.Set("Authorization", "Bearer "+f.minted["customer/manage"])
	}); reached || recorder.Code != http.StatusForbidden {
		t.Fatalf("a cookie plus a token without an Origin passed: %d %s", recorder.Code, recorder.Body.String())
	}
	// A wrong token without an Origin skips the origin check and is refused
	// by the token layer.
	if recorder, reached := send(func(r *http.Request) { r.Header.Set("Authorization", "Bearer cpk_wrong") }); reached || recorder.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong token: reached=%v %d", reached, recorder.Code)
	}
	// Another bearer credential (a remote DNS machine's shape) keeps the
	// origin check.
	if recorder, reached := send(func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32)+"."+strings.Repeat("b", 64))
	}); reached ||
		recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "cross-origin") {
		t.Fatalf("a non-token bearer skipped the origin check: %d %s", recorder.Code, recorder.Body.String())
	}
	// A token on a public route (sign-in) is refused, never passed through.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.Header.Set("Authorization", "Bearer "+f.minted["admin/manage"])
	recorder := httptest.NewRecorder()
	probe.reached = false
	handler.ServeHTTP(recorder, request)
	if probe.reached || recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenRouteRefused {
		t.Fatalf("a token reached the public sign-in route: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPITokenStatesEndATokenAndNameTheReason(t *testing.T) {
	f := newAPITokenFixture(t)
	// More than ten refused tokens are sent from one address here; the
	// sign-in limit has its own test.
	unlimitedAPITokenGuards(f.panel)
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	call := func(token string) (*httptest.ResponseRecorder, bool) {
		probe.reached = false
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(http.MethodGet, "/api/v1/domains", token))
		return recorder, probe.reached
	}
	expect := func(name, token string, status int, code string) {
		t.Helper()
		recorder, reached := call(token)
		if reached || recorder.Code != status || apiErrorCode(t, recorder) != code {
			t.Errorf("%s: reached=%v %d %s, want %d %s", name, reached, recorder.Code, recorder.Body.String(), status, code)
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control %q", name, got)
		}
	}
	if recorder, reached := call(f.minted["customer/read"]); !reached {
		t.Fatalf("a working token was refused: %d %s", recorder.Code, recorder.Body.String())
	}

	// Expired.
	expired, _ := mintTestAPIToken(t, f.panel, authzMatrixCustomerID, apiTokenScopeRead, time.Now().Add(-time.Second))
	expect("expired", expired, http.StatusUnauthorized, errCodeAPITokenExpired)
	// The clock reaches the end of a working token.
	soon, _ := mintTestAPIToken(t, f.panel, authzMatrixCustomerID, apiTokenScopeRead, time.Now().Add(time.Hour))
	apiTokenNow = func() time.Time { return time.Now().Add(2 * time.Hour) }
	expect("expired by the clock", soon, http.StatusUnauthorized, errCodeAPITokenExpired)
	apiTokenNow = time.Now
	defer func() { apiTokenNow = time.Now }()

	// Revoked through the screen's route.
	mux := http.NewServeMux()
	mux.HandleFunc(apiTokensPath+"/", f.panel.handleAPITokenItem)
	session := f.panel.requireAuth(mux)
	id := f.rowIDs["customer/manage"]
	// Another account cannot remove it.
	if recorder := requestWithToken(session, http.MethodDelete, fmt.Sprintf("%s/%d", apiTokensPath, id), f.tokens[authzMatrixOutsiderID]); recorder.Code != http.StatusNotFound {
		t.Fatalf("another account removed the token: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder, reached := call(f.minted["customer/manage"]); !reached {
		t.Fatalf("a token another account tried to remove stopped working: %d", recorder.Code)
	}
	if recorder := requestWithToken(session, http.MethodDelete, fmt.Sprintf("%s/%d", apiTokensPath, id), f.tokens[authzMatrixCustomerID]); recorder.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", recorder.Code, recorder.Body.String())
	}
	expect("revoked", f.minted["customer/manage"], http.StatusUnauthorized, errCodeAPITokenRevoked)

	// The owner's password or two-factor state changed (credential epoch).
	if _, err := f.panel.db.GetDB().Exec(`UPDATE users SET auth_epoch = auth_epoch + 1 WHERE id = ?`, authzMatrixResellerID); err != nil {
		t.Fatal(err)
	}
	expect("ended by a credential change", f.minted["reseller/read"], http.StatusUnauthorized, errCodeAPITokenEnded)

	// A suspended owner.
	suspended, _ := mintTestAPIToken(t, f.panel, authzMatrixSuspendedID, apiTokenScopeRead, time.Now().Add(time.Hour))
	expect("suspended owner", suspended, http.StatusForbidden, errCodeAccountSuspended)

	// Malformed and unknown tokens.
	known := f.minted["admin/read"]
	for name, bad := range map[string]string{
		"short":         "cpk_abc",
		"bad secret":    known[:17] + strings.Repeat("!", 43),
		"wrong secret":  known[:len(known)-1] + map[bool]string{true: "A", false: "B"}[known[len(known)-1] != 'A'],
		"unknown id":    "cpk_" + strings.Repeat("a", 12) + known[16:],
		"upper-case id": "cpk_" + strings.ToUpper(known[4:16]) + known[16:],
		"no separator":  known[:16] + "x" + known[17:],
	} {
		expect(name, bad, http.StatusUnauthorized, errCodeAPITokenInvalid)
	}
	// A bearer header that is not a CelikPanel token is not a token request:
	// without a session it is the ordinary "sign in" answer.
	for _, header := range []string{"Bearer", "Bearer   ", "bearer x"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
		request.Header.Set("Authorization", header)
		recorder := httptest.NewRecorder()
		probe.reached = false
		handler.ServeHTTP(recorder, request)
		if probe.reached || recorder.Code != http.StatusUnauthorized || apiErrorCode(t, recorder) != errCodeAuthRequired {
			t.Errorf("header %q: reached=%v %d %s", header, probe.reached, recorder.Code, recorder.Body.String())
		}
	}

	// The server switch off: every token stops, and is kept.
	if err := f.panel.setSetting(context.Background(), settingRemoteAPIEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	expect("switch off", f.minted["admin/read"], http.StatusForbidden, errCodeAPIDisabled)
	if _, err := f.panel.db.GetDB().Exec(`DELETE FROM panel_settings WHERE key = ?`, settingRemoteAPIEnabled); err != nil {
		t.Fatal(err)
	}
	expect("switch never set", f.minted["admin/read"], http.StatusForbidden, errCodeAPIDisabled)
	if err := f.panel.setSetting(context.Background(), settingRemoteAPIEnabled, "1"); err != nil {
		t.Fatal(err)
	}
	if recorder, reached := call(f.minted["admin/read"]); !reached {
		t.Fatalf("the token did not work again with the switch on: %d", recorder.Code)
	}
}

func TestAPITokenUnknownStoredScopeOnlyReads(t *testing.T) {
	f := newAPITokenFixture(t)
	if _, err := f.panel.db.GetDB().Exec(`UPDATE api_tokens SET scope = 'MANAGE' WHERE id = ?`, f.rowIDs["customer/manage"]); err != nil {
		t.Fatal(err)
	}
	probe := &apiTokenProbe{}
	recorder := httptest.NewRecorder()
	f.panel.requireAuth(probe.handler()).ServeHTTP(recorder, tokenRequest(http.MethodPost, "/api/v1/domains/9902/dns/records", f.minted["customer/manage"]))
	if probe.reached || apiErrorCode(t, recorder) != errCodeAPITokenScope {
		t.Fatalf("an unknown stored scope changed something: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPITokenPermissionFollowsThePlanLayers(t *testing.T) {
	f := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	reach := func(token string) (*httptest.ResponseRecorder, bool) {
		probe.reached = false
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(http.MethodGet, "/api/v1/domains", token))
		return recorder, probe.reached
	}
	ctx := context.Background()
	db := f.panel.db.GetDB()

	// The customer's subscription without the permission.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	limits, err := loadSubscriptionLimits(ctx, tx, authzMatrixCustomerSubID)
	if err != nil {
		t.Fatal(err)
	}
	if err := setSubscriptionCustom(ctx, tx, authzMatrixCustomerSubID, newRightsSet(false, true, everyPermissionBut(rightRemoteAPI)...), limits); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	recorder, reached := reach(f.minted["customer/read"])
	if reached || recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenNotPermitted {
		t.Fatalf("a customer without the permission: reached=%v %d %s", reached, recorder.Code, recorder.Body.String())
	}
	// The screen says the same, and who opens it.
	mux := http.NewServeMux()
	mux.HandleFunc(apiTokensPath, f.panel.handleAPITokens)
	list := requestWithToken(f.panel.requireAuth(mux), http.MethodGet, apiTokensPath, f.tokens[authzMatrixCustomerID])
	var answer apiTokensAnswer
	if err := json.Unmarshal(list.Body.Bytes(), &answer); err != nil || answer.CanCreate || answer.RefusedBy != rightsLayerSubscription || answer.GrantorKind != "provider" || answer.GrantorName != "matrix-reseller" || !answer.ServerEnabled {
		t.Fatalf("list for a customer without the permission: %s", list.Body.String())
	}

	// A reseller whose own rights lack it.
	if err := saveRightsSet(ctx, db, rightsOwnerReseller, authzMatrixResellerID, newRightsSet(false, true, everyPermissionBut(rightRemoteAPI)...)); err != nil {
		t.Fatal(err)
	}
	if recorder, reached := reach(f.minted["reseller/read"]); reached || apiErrorCode(t, recorder) != errCodeAPITokenNotPermitted {
		t.Fatalf("a reseller without the permission: reached=%v %d %s", reached, recorder.Code, recorder.Body.String())
	}

	// The administrator hides it on the server: every reseller and customer
	// is closed; the administrator keeps it.
	if err := f.panel.saveHiddenOfferings(ctx, map[string]bool{permissionItemHead + rightRemoteAPI: true}); err != nil {
		t.Fatal(err)
	}
	if err := saveRightsSet(ctx, db, rightsOwnerReseller, authzMatrixResellerID, newRightsSet(true, true)); err != nil {
		t.Fatal(err)
	}
	if recorder, reached := reach(f.minted["reseller/read"]); reached || apiErrorCode(t, recorder) != errCodeAPITokenNotPermitted {
		t.Fatalf("a reseller with the permission hidden by the administrator: reached=%v %d", reached, recorder.Code)
	}
	outsider, _ := mintTestAPIToken(t, f.panel, authzMatrixOutsiderID, apiTokenScopeRead, time.Now().Add(time.Hour))
	if recorder, reached := reach(outsider); reached || apiErrorCode(t, recorder) != errCodeAPITokenNotPermitted {
		t.Fatalf("a customer with the permission hidden by the administrator: reached=%v %d", reached, recorder.Code)
	}
	if recorder, reached := reach(f.minted["admin/read"]); !reached {
		t.Fatalf("the administrator lost the remote API: %d %s", recorder.Code, recorder.Body.String())
	}
}

// The real handlers behind the token: ownership still decides.
func TestAPITokenCannotReachAnotherTenant(t *testing.T) {
	f := newAPITokenFixture(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/domains", f.panel.handleDomains)
	mux.HandleFunc("/api/v1/domains/", f.panel.handleDomainSubroute)
	handler := f.panel.requireAuth(mux)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, tokenRequest(http.MethodGet, "/api/v1/domains", f.minted["customer/read"]))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "customer.matrix.test") ||
		strings.Contains(recorder.Body.String(), "outsider.matrix.test") || strings.Contains(recorder.Body.String(), "reseller.matrix.test") {
		t.Fatalf("a customer's token listed other domains: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/domains/9903/dns/records"},
		{http.MethodDelete, "/api/v1/domains/9903/dns/records"},
		{http.MethodGet, "/api/v1/domains/9903/mail/accounts"},
		{http.MethodPost, "/api/v1/domains/9903/mail/accounts"},
		{http.MethodGet, "/api/v1/domains/9901/backups"},
		{http.MethodDelete, "/api/v1/domains/9903"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(request.method, request.path, f.minted["customer/manage"]))
		if recorder.Code != http.StatusForbidden && recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s with a customer's token reached another tenant: %d %s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestAPITokenCreateShowsTheTokenOnceAndKeepsOnlyItsHash(t *testing.T) {
	f := newAPITokenFixture(t)
	f.panel.requestIdentities = newRequestIdentityGuard(f.panel.db.GetDB())
	mux := http.NewServeMux()
	mux.HandleFunc(apiTokensPath, f.panel.handleAPITokens)
	mux.HandleFunc(apiTokensPath+"/", f.panel.handleAPITokenItem)
	mux.HandleFunc("/api/v1/domains", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := f.panel.requireAuth(f.panel.requestIdentities.wrap(mux))
	create := func(session, body, id string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, apiTokensPath, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(requestIdentityHeader, id)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	requestID := strings.Repeat("c", 32)
	recorder := create(f.tokens[authzMatrixCustomerID], `{"name":"deploy bot","scope":"manage","expires_days":30}`, requestID)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	var made struct {
		Token string       `json:"token"`
		Item  apiTokenView `json:"item"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}
	tokenID, secret, ok := parseAPIToken(made.Token)
	if !ok || made.Item.Prefix != apiTokenPrefix+tokenID || made.Item.Scope != apiTokenScopeManage || made.Item.State != "active" ||
		made.Item.ExpiresAt-made.Item.CreatedAt != 30*24*3600 {
		t.Fatalf("created: %+v", made)
	}
	// The same request again: answered from the first, no second token, and
	// the token is not shown again.
	again := create(f.tokens[authzMatrixCustomerID], `{"name":"deploy bot","scope":"manage","expires_days":30}`, requestID)
	if strings.Contains(again.Body.String(), secret) || again.Code == http.StatusCreated {
		t.Fatalf("a replay showed the token again: %d %s", again.Code, again.Body.String())
	}
	var count int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE name = 'deploy bot'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("tokens made: %d %v", count, err)
	}
	// The database keeps the hash only; the secret is nowhere in it.
	var storedHash string
	if err := f.panel.db.GetDB().QueryRow(`SELECT secret_sha256 FROM api_tokens WHERE token_id = ?`, tokenID).Scan(&storedHash); err != nil || storedHash != hashAPITokenSecret(secret) {
		t.Fatalf("stored hash %q, %v", storedHash, err)
	}
	requireSecretNowhereInDatabase(t, f.panel, secret)
	// The list never carries it.
	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, apiTokensPath, nil)
	listRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
	handler.ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), secret) || !strings.Contains(list.Body.String(), made.Item.Prefix) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	// It works, and its use is written once a minute.
	use := func() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(http.MethodGet, "/api/v1/domains", made.Token))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("the new token did not work: %d %s", recorder.Code, recorder.Body.String())
		}
	}
	lastUsed := func() int64 {
		var at sql.NullInt64
		if err := f.panel.db.GetDB().QueryRow(`SELECT last_used_at FROM api_tokens WHERE token_id = ?`, tokenID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at.Int64
	}
	base := time.Now()
	apiTokenNow = func() time.Time { return base }
	defer func() { apiTokenNow = time.Now }()
	use()
	first := lastUsed()
	apiTokenNow = func() time.Time { return base.Add(30 * time.Second) }
	use()
	if lastUsed() != first {
		t.Fatal("the last use was written twice within a minute")
	}
	apiTokenNow = func() time.Time { return base.Add(61 * time.Second) }
	use()
	if lastUsed() != base.Add(61*time.Second).Unix() {
		t.Fatal("the last use was not written after a minute")
	}

	// Refusals: bad body, unknown field, no expiry, impersonation.
	for _, body := range []string{
		`{"name":"","scope":"read","expires_days":30}`,
		`{"name":"x","scope":"admin","expires_days":30}`,
		`{"name":"x","scope":"read","expires_days":0}`,
		`{"name":"x","scope":"read","expires_days":3650}`,
		`{"name":"x","scope":"read","expires_days":30,"admin":true}`,
		`{"name":"` + strings.Repeat("n", 65) + `","scope":"read","expires_days":30}`,
		`{"name":"a\u0000b","scope":"read","expires_days":30}`,
	} {
		if recorder := create(f.tokens[authzMatrixCustomerID], body, newTestRequestID(t)); recorder.Code != http.StatusBadRequest {
			t.Errorf("body %s: %d %s", body, recorder.Code, recorder.Body.String())
		}
	}
	impersonation, err := f.panel.sessions.CreateImpersonation(context.Background(), authzMatrixCustomerID, authzMatrixAdminID)
	if err != nil {
		t.Fatal(err)
	}
	if recorder := create(impersonation, `{"name":"x","scope":"read","expires_days":30}`, newTestRequestID(t)); recorder.Code != http.StatusForbidden ||
		apiErrorCode(t, recorder) != errCodeAPITokenImpersonation {
		t.Fatalf("a token was made under Log in as: %d %s", recorder.Code, recorder.Body.String())
	}
	// A token cannot make a token.
	request := httptest.NewRequest(http.MethodPost, apiTokensPath, strings.NewReader(`{"name":"x","scope":"read","expires_days":30}`))
	request.Header.Set("Authorization", "Bearer "+made.Token)
	request.Header.Set(requestIdentityHeader, newTestRequestID(t))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenRouteRefused {
		t.Fatalf("a token made a token: %d %s", recorder.Code, recorder.Body.String())
	}
	// With the switch off nothing is made, and the screen says why.
	if err := f.panel.setSetting(context.Background(), settingRemoteAPIEnabled, "0"); err != nil {
		t.Fatal(err)
	}
	if recorder := create(f.tokens[authzMatrixCustomerID], `{"name":"x","scope":"read","expires_days":30}`, newTestRequestID(t)); recorder.Code != http.StatusForbidden ||
		apiErrorCode(t, recorder) != errCodeAPIDisabled {
		t.Fatalf("a token was made with the switch off: %d %s", recorder.Code, recorder.Body.String())
	}
}

var testRequestIDCounter int

func newTestRequestID(t *testing.T) string {
	t.Helper()
	testRequestIDCounter++
	return fmt.Sprintf("%032x", testRequestIDCounter+0xabc000)
}

// requireSecretNowhereInDatabase reads every text value of every table.
func requireSecretNowhereInDatabase(t *testing.T, p *Panel, secret string) {
	t.Helper()
	db := p.db.GetDB()
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}
	rows.Close()
	for _, table := range tables {
		data, err := db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, _ := data.Columns()
		for data.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := data.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				var text string
				switch v := value.(type) {
				case string:
					text = v
				case []byte:
					text = string(v)
				}
				if strings.Contains(text, secret) {
					t.Errorf("table %s column %s holds the token's secret", table, columns[i])
				}
			}
		}
		data.Close()
	}
}

func TestAPITokenRateLimits(t *testing.T) {
	f := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	handler := f.panel.requireAuth(probe.handler())
	send := func(token, address string) *httptest.ResponseRecorder {
		request := tokenRequest(http.MethodGet, "/api/v1/domains", token)
		request.RemoteAddr = address + ":4000"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	for i := 0; i < apiTokenCallsPerMinute; i++ {
		if recorder := send(f.minted["customer/read"], "198.51.100.1"); recorder.Code != http.StatusNoContent {
			t.Fatalf("call %d: %d %s", i+1, recorder.Code, recorder.Body.String())
		}
	}
	recorder := send(f.minted["customer/read"], "198.51.100.1")
	if recorder.Code != http.StatusTooManyRequests || apiErrorCode(t, recorder) != errCodeAPITokenRateLimited || recorder.Header().Get("Retry-After") != "60" {
		t.Fatalf("call 121: %d %s", recorder.Code, recorder.Body.String())
	}
	// Another token of the same account is counted on its own.
	if recorder := send(f.minted["customer/manage"], "198.51.100.1"); recorder.Code != http.StatusNoContent {
		t.Fatalf("another token: %d", recorder.Code)
	}
	// Ten failed sign-ins from one address close it for the minute, even for
	// a right token; another address is not affected.
	wrong := f.minted["admin/read"][:len(f.minted["admin/read"])-2] + "zz"
	for i := 0; i < apiTokenFailuresPerMinute; i++ {
		if recorder := send(wrong, "203.0.113.9"); recorder.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d", i+1, recorder.Code)
		}
	}
	recorder = send(f.minted["admin/read"], "203.0.113.9")
	if recorder.Code != http.StatusTooManyRequests || apiErrorCode(t, recorder) != errCodeAPITokenSignInLimit {
		t.Fatalf("after ten failures: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := send(f.minted["admin/read"], "203.0.113.10"); recorder.Code != http.StatusNoContent {
		t.Fatalf("another address: %d", recorder.Code)
	}
}

func TestAPITokenAudit(t *testing.T) {
	f := newAPITokenFixture(t)
	f.panel.requestIdentities = newRequestIdentityGuard(f.panel.db.GetDB())
	mux := http.NewServeMux()
	mux.HandleFunc(apiTokensPath, f.panel.handleAPITokens)
	mux.HandleFunc(apiTokensPath+"/", f.panel.handleAPITokenItem)
	mux.HandleFunc("/api/v1/domains", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/api/v1/domains/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			f.panel.audit(r, "dns.record.add", "domain", apiTokenTestDomain)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := f.panel.requireAuth(f.panel.requestIdentities.wrap(mux))

	request := httptest.NewRequest(http.MethodPost, apiTokensPath, strings.NewReader(`{"name":"ci","scope":"manage","expires_days":7}`))
	request.Header.Set(requestIdentityHeader, newTestRequestID(t))
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var made struct {
		Token string       `json:"token"`
		Item  apiTokenView `json:"item"`
	}
	if recorder.Code != http.StatusCreated || json.Unmarshal(recorder.Body.Bytes(), &made) != nil {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	_, secret, _ := parseAPIToken(made.Token)

	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/domains"},
		{http.MethodGet, "/api/v1/domains/9902/dns/records"},
		{http.MethodPost, "/api/v1/domains/9902/dns/records"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, tokenRequest(call.method, call.path, made.Token))
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("%s %s: %d %s", call.method, call.path, recorder.Code, recorder.Body.String())
		}
	}
	deleteRequest := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%d", apiTokensPath, made.Item.ID), nil)
	deleteRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, deleteRequest)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", recorder.Code)
	}

	rows, err := f.panel.db.GetDB().Query(`SELECT action, user_id, COALESCE(details, '') FROM audit_logs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var action, details string
		var user int
		if err := rows.Scan(&action, &user, &details); err != nil {
			t.Fatal(err)
		}
		seen[action]++
		if strings.Contains(details, secret) || strings.Contains(action, secret) {
			t.Errorf("audit row %s holds the secret", action)
		}
		if user != authzMatrixCustomerID {
			t.Errorf("audit row %s names user %d", action, user)
		}
		switch action {
		case "api_token.call":
			if !strings.Contains(details, `"api_token":"`+made.Item.Prefix+`"`) || !strings.Contains(details, `"method":"POST"`) ||
				!strings.Contains(details, `"path":"/api/v1/domains/9902/dns/records"`) {
				t.Errorf("call row details: %s", details)
			}
		case "dns.record.add":
			// The handler's own row names the token too.
			if !strings.Contains(details, made.Item.Prefix) {
				t.Errorf("the handler's row does not name the token: %s", details)
			}
		case "api_token.create", "api_token.revoke":
			if !strings.Contains(details, made.Item.Prefix) {
				t.Errorf("%s details: %s", action, details)
			}
		}
	}
	for action, want := range map[string]int{"api_token.create": 1, "api_token.revoke": 1, "api_token.call": 1, "dns.record.add": 1} {
		if seen[action] != want {
			t.Errorf("audit %s: %d rows, want %d (all: %v)", action, seen[action], want, seen)
		}
	}
	requireSecretNowhereInDatabase(t, f.panel, secret)
}

func TestRemoteAPISwitchIsTheAdministratorsAndAudited(t *testing.T) {
	f := newAPITokenFixture(t)
	mux := http.NewServeMux()
	mux.HandleFunc(remoteAPISettingPath, f.panel.handleRemoteAPISetting)
	handler := csrfProtect(f.panel.requireAuth(mux))
	put := func(session, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, remoteAPISettingPath, strings.NewReader(body))
		request.Header.Set("Origin", "http://example.com")
		request.Host = "example.com"
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	for _, id := range []int{authzMatrixResellerID, authzMatrixCustomerID} {
		if recorder := put(f.tokens[id], `{"enabled":false}`); recorder.Code != http.StatusForbidden {
			t.Fatalf("user %d turned the switch: %d", id, recorder.Code)
		}
	}
	// An administrator's token cannot turn it either.
	request := httptest.NewRequest(http.MethodPut, remoteAPISettingPath, strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Authorization", "Bearer "+f.minted["admin/manage"])
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenRouteRefused {
		t.Fatalf("an administrator's token turned the switch: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `{"enabled":true,"x":1}`} {
		if recorder := put(f.tokens[authzMatrixAdminID], body); recorder.Code != http.StatusBadRequest {
			t.Errorf("body %s: %d", body, recorder.Code)
		}
	}
	recorder = put(f.tokens[authzMatrixAdminID], `{"enabled":false}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"enabled":false`) {
		t.Fatalf("turn off: %d %s", recorder.Code, recorder.Body.String())
	}
	if enabled, err := f.panel.remoteAPIEnabled(context.Background()); err != nil || enabled {
		t.Fatalf("still on: %v %v", enabled, err)
	}
	recorder = put(f.tokens[authzMatrixAdminID], `{"enabled":true}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"working_tokens":6`) {
		t.Fatalf("turn on: %d %s", recorder.Code, recorder.Body.String())
	}
	var actions []string
	rows, err := f.panel.db.GetDB().Query(`SELECT action FROM audit_logs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var action string
		_ = rows.Scan(&action)
		actions = append(actions, action)
	}
	rows.Close()
	if strings.Join(actions, ",") != "remote_api.disable,remote_api.enable" {
		t.Fatalf("audit: %v", actions)
	}
}

func TestAPITokenFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tokenID, secret, full, err := newAPIToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(full) != apiTokenLength || seen[tokenID] {
			t.Fatalf("token %q", full)
		}
		seen[tokenID] = true
		id, parsedSecret, ok := parseAPIToken(full)
		if !ok || id != tokenID || parsedSecret != secret {
			t.Fatalf("round trip of %q: %q %q %v", full, id, parsedSecret, ok)
		}
		if len(hashAPITokenSecret(secret)) != 64 || hashAPITokenSecret(secret) == hashAPITokenSecret(secret+"x") {
			t.Fatal("hash")
		}
	}
	for _, header := range []struct {
		value   string
		present bool
		token   string
	}{
		{"Bearer cpk_x", true, "cpk_x"},
		{"bearer   cpk_x  ", true, "cpk_x"},
		{"Bearer", false, ""},
		{"Bearer eyJhbGciOiJSUzI1NiJ9.e30.sig", false, ""},
		{"Bearer " + strings.Repeat("a", 32) + "." + strings.Repeat("b", 64), false, ""},
		{"Basic abc", false, ""},
		{"Token cpk_x", false, ""},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
		request.Header.Set("Authorization", header.value)
		token, present := apiTokenCredential(request)
		if present != header.present || token != header.token {
			t.Errorf("%q: %q %v", header.value, token, present)
		}
	}
	// A token in the query string is never read.
	request := httptest.NewRequest(http.MethodGet, "/api/v1/domains?access_token=cpk_x&token=cpk_x", nil)
	if _, present := apiTokenCredential(request); present {
		t.Fatal("a query-string token was read")
	}
}

// An identity proxy in front of the panel (oauth2-proxy and the like) sends
// its own "Authorization: Bearer <ID token>". That header is not a CelikPanel
// token: the sign-in route, the web interface and a signed-in session behave
// exactly as without it, whether the remote API is on or off.
func TestAPITokenOtherBearerHeadersAreIgnored(t *testing.T) {
	f := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	handler := csrfProtect(f.panel.requireAuth(probe.handler()))
	idToken := "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJvd25lciJ9.c2lnbmF0dXJl"
	for _, switchValue := range []string{"1", "0"} {
		if err := f.panel.setSetting(context.Background(), settingRemoteAPIEnabled, switchValue); err != nil {
			t.Fatal(err)
		}
		send := func(method, path string, session bool) (*httptest.ResponseRecorder, bool) {
			request := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", idToken)
			request.Header.Set("Origin", "http://example.com")
			if session {
				request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[authzMatrixCustomerID]})
			}
			recorder := httptest.NewRecorder()
			probe.reached = false
			handler.ServeHTTP(recorder, request)
			return recorder, probe.reached
		}
		// The sign-in route stays public.
		if recorder, reached := send(http.MethodPost, "/api/v1/auth/login", false); !reached {
			t.Errorf("switch %s: sign-in with a proxy bearer header: %d %s", switchValue, recorder.Code, recorder.Body.String())
		}
		// The web interface stays public.
		if recorder, reached := send(http.MethodGet, "/", false); !reached {
			t.Errorf("switch %s: the web interface with a proxy bearer header: %d %s", switchValue, recorder.Code, recorder.Body.String())
		}
		// A signed-in session with the header works as before.
		if recorder, reached := send(http.MethodGet, "/api/v1/auth/me", true); !reached || probe.caller == nil || probe.caller.APIToken != nil {
			t.Errorf("switch %s: a session with a proxy bearer header: %d %s", switchValue, recorder.Code, recorder.Body.String())
		}
		// Without a session it is the ordinary answer, not a token refusal.
		if recorder, reached := send(http.MethodGet, "/api/v1/domains", false); reached || recorder.Code != http.StatusUnauthorized || apiErrorCode(t, recorder) != errCodeAuthRequired {
			t.Errorf("switch %s: no session: %d %s", switchValue, recorder.Code, recorder.Body.String())
		}
	}
}
