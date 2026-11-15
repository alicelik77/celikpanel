package main

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// D-047 stage 1: a licence that does not allow full use refuses only
// creating new things and changing settings; security, backups, deleting,
// one's own account, updates, the licence screen and the DNS pair work in
// every licence state. These tests use the repository's own signed test
// receipts (testPanelLicense); nothing reaches the licence service (the
// licensing package refuses any test request to it).

// productionCountCheckLicenceStatus is the count check's own reading; the
// package's TestMain replaces countCheckLicenceStatus so handler tests that
// build a Panel without a licence manager keep creating things.
var productionCountCheckLicenceStatus = countCheckLicenceStatus

func useTestCountCheckLicenceStatus() {
	countCheckLicenceStatus = func(p *Panel) licensing.Status {
		if p.license == nil {
			return licensing.Status{State: "active", Observation: licensing.ObservationKnown, CanProvision: true}
		}
		return productionCountCheckLicenceStatus(p)
	}
}

// ---------- the route inventory ----------

// licenceTestSources parses the package's production files.
func licenceTestSources(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = file
	}
	return fset, files
}

// licenceTestStringConstants resolves the package's string constants and
// variables built from literals and other such names joined with +.
func licenceTestStringConstants(files map[string]*ast.File) map[string]string {
	pending := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Values) != len(value.Names) {
					continue
				}
				for i, name := range value.Names {
					pending[name.Name] = value.Values[i]
				}
			}
		}
	}
	resolved := map[string]string{}
	for changed := true; changed; {
		changed = false
		for name, expr := range pending {
			if value, ok := licenceTestEval(expr, resolved); ok {
				resolved[name] = value
				delete(pending, name)
				changed = true
			}
		}
	}
	return resolved
}

func licenceTestEval(expr ast.Expr, known map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(e.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := known[e.Name]
		return value, ok
	case *ast.ParenExpr:
		return licenceTestEval(e.X, known)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := licenceTestEval(e.X, known)
		if !ok {
			return "", false
		}
		right, ok := licenceTestEval(e.Y, known)
		return left + right, ok
	}
	return "", false
}

// licenceTestRegisteredPatterns: every pattern main.go, the account-link
// routes and the startup recovery mux register.
func licenceTestRegisteredPatterns(t *testing.T, files map[string]*ast.File, constants map[string]string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, name := range []string{"main.go", "account_recovery_handlers.go", "recovery_access.go"} {
		file := files[name]
		if file == nil {
			t.Fatalf("%s not found", name)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle") {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || (receiver.Name != "http" && receiver.Name != "mux") {
				return true
			}
			pattern, ok := licenceTestEval(call.Args[0], constants)
			if !ok {
				t.Errorf("%s: a registered pattern cannot be read: %#v", name, call.Args[0])
				return true
			}
			seen[pattern] = true
			return true
		})
	}
	patterns := make([]string, 0, len(seen))
	for pattern := range seen {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	if len(patterns) < 200 {
		t.Fatalf("only %d registered patterns found; the inventory does not see the routes", len(patterns))
	}
	return patterns
}

// licenceTestSwitchCases returns the case values of the switch on tag inside
// the named function.
func licenceTestSwitchCases(t *testing.T, files map[string]*ast.File, constants map[string]string, function, tag string) []string {
	t.Helper()
	var out []string
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != function {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				sw, ok := node.(*ast.SwitchStmt)
				if !ok || sw.Tag == nil || licenceTestExprText(sw.Tag) != tag {
					return true
				}
				for _, stmt := range sw.Body.List {
					clause := stmt.(*ast.CaseClause)
					for _, value := range clause.List {
						text, ok := licenceTestEval(value, constants)
						if !ok {
							t.Errorf("%s: case %#v cannot be read", function, value)
							continue
						}
						out = append(out, text)
					}
				}
				return false
			})
		}
	}
	if len(out) == 0 {
		t.Fatalf("no switch on %s in %s", tag, function)
	}
	return out
}

func licenceTestExprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return licenceTestExprText(e.X) + "." + e.Sel.Name
	}
	return ""
}

var licenceTestWriteMethods = []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// Every registered route, every domain subroute kind and every database
// subroute kind has a class for every method; the table names nothing that is
// not registered. A new route without a class fails here.
func TestLicenceRouteInventoryEveryRouteHasAClass(t *testing.T) {
	_, files := licenceTestSources(t)
	constants := licenceTestStringConstants(files)
	patterns := licenceTestRegisteredPatterns(t, files, constants)
	registered := map[string]bool{}
	for _, pattern := range patterns {
		registered[pattern] = true
		if _, ok := licenceRouteTable[pattern]; !ok {
			t.Errorf("registered route %q has no licence class in licenceRouteTable", pattern)
			continue
		}
		for _, method := range append([]string{http.MethodGet}, licenceTestWriteMethods...) {
			if _, ok := licenceRouteClassOf(httptest.NewRequest(method, pattern, nil)); !ok {
				t.Errorf("%s %s has no licence class", method, pattern)
			}
		}
	}
	for pattern := range licenceRouteTable {
		if !registered[pattern] {
			t.Errorf("licenceRouteTable names %q, which no registration serves", pattern)
		}
	}

	// Domain subroutes: every key of the matcher's switch and every shape of
	// its default branch, with every method it accepts.
	samples := []string{}
	for _, key := range licenceTestSwitchCases(t, files, constants, "matchDomainSubroute", "key") {
		samples = append(samples, key)
	}
	op := strings.Repeat("a", 32)
	samples = append(samples, "aliases/alias.example", "apps/"+op, "apps/"+op+"/retry", "apps/"+op+"/credentials",
		"access/logins/anna-uploads", "databases/5", "databases/5/users", "databases/5/users/6", "databases/5/users/6/password")
	for tool := range databaseToolsKinds {
		samples = append(samples, "databases/5/"+tool)
	}
	produced := map[string]bool{}
	for _, tail := range samples {
		path := "/api/v1/domains/7"
		if tail != "" {
			path += "/" + tail
		}
		match, ok := matchDomainSubroute(httptest.NewRequest(http.MethodGet, path, nil))
		if !ok {
			t.Errorf("sample %s is not a domain subroute", path)
			continue
		}
		produced[match.kind] = true
		for _, method := range match.methods {
			if method == http.MethodOptions {
				continue
			}
			class, ok := licenceRouteClassOf(httptest.NewRequest(method, path, nil))
			if !ok || class == "" {
				t.Errorf("%s %s (kind %s) has no licence class", method, path, match.kind)
			}
		}
	}
	dispatched := map[string]bool{}
	for _, kind := range licenceTestSwitchCases(t, files, constants, "handleDomainSubroute", "match.kind") {
		dispatched[kind] = true
		if !produced[kind] {
			t.Errorf("domain subroute kind %q is dispatched but no inventory sample reaches it; add its path shape", kind)
		}
	}
	for kind := range licenceDomainKindClasses {
		if !dispatched[kind] {
			t.Errorf("licenceDomainKindClasses names kind %q, which is not dispatched", kind)
		}
	}

	// Database subroutes.
	produced = map[string]bool{}
	for _, sample := range []struct{ prefix, rest string }{
		{"/api/v1/database-servers/", "3"}, {"/api/v1/database-servers/", "3/databases"}, {"/api/v1/database-servers/", "3/users"},
		{"/api/v1/database-servers/", "3/admin-account"}, {"/api/v1/databases/", "3"}, {"/api/v1/databases/", "3/grants"},
		{"/api/v1/database-users/", "3"}, {"/api/v1/database-users/", "3/password"}, {"/api/v1/database-grants/", "3"},
	} {
		path := sample.prefix + sample.rest
		match, ok := matchDatabaseSubroute(httptest.NewRequest(http.MethodGet, path, nil), sample.prefix)
		if !ok {
			t.Errorf("sample %s is not a database subroute", path)
			continue
		}
		produced[match.kind] = true
		for _, method := range match.methods {
			if _, ok := licenceRouteClassOf(httptest.NewRequest(method, path, nil)); !ok {
				t.Errorf("%s %s (kind %s) has no licence class", method, path, match.kind)
			}
		}
	}
	dispatched = map[string]bool{}
	for _, kind := range licenceTestSwitchCases(t, files, constants, "handleDatabaseSubroute", "match.kind") {
		dispatched[kind] = true
		if !produced[kind] {
			t.Errorf("database subroute kind %q is dispatched but no inventory sample reaches it", kind)
		}
	}
	for kind := range licenceDatabaseKindClasses {
		if !dispatched[kind] {
			t.Errorf("licenceDatabaseKindClasses names kind %q, which is not dispatched", kind)
		}
	}
}

// The classes of the routes the owner named and of the routes whose class
// was decided here (towards security), so a later edit cannot move them
// silently.
func TestLicenceRouteClassesOfNamedRoutes(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		want         licenceRouteClass
	}{
		{"POST", "/api/v1/firewall", licenceClassSecurity},
		{"GET", osUpdatesPath, licenceClassRead},
		{"POST", osUpdatesPath, licenceClassSecurity},
		{"POST", osUpdatesOriginsPath, licenceClassSecurity},
		{"PUT", osUpdatesModePath, licenceClassSecurity},
		{"DELETE", databaseRemotePath + "rule", licenceClassSecurity},
		{"POST", securityEmergenciesPath, licenceClassSecurity},
		{"POST", securityEmergenciesPath + "/4/stop", licenceClassSecurity},
		{"POST", "/api/v1/domains/9/ssl/letsencrypt", licenceClassSecurity},
		{"POST", "/api/v1/domains/9/protected-dirs", licenceClassSecurity},
		{"POST", "/api/v1/users/9/suspension", licenceClassSecurity},
		{"POST", "/api/v1/subscriptions/9/suspension", licenceClassSecurity},
		{"POST", "/api/v1/database-servers/3/admin-account", licenceClassSecurity},
		{"DELETE", "/api/v1/database-grants/3", licenceClassDelete},
		{"POST", "/api/v1/domains/9/backups", licenceClassBackup},
		{"POST", "/api/v1/domains/9/backups/restore", licenceClassBackup},
		{"POST", "/api/v1/domains/9/databases/5/export", licenceClassBackup},
		{"POST", remoteBackupPathPrefix + "destinations", licenceClassBackup},
		{"DELETE", "/api/v1/domains/9", licenceClassDelete},
		{"DELETE", "/api/v1/domains/9/mail/accounts", licenceClassDelete},
		{"DELETE", "/api/v1/users/9", licenceClassDelete},
		{"DELETE", holdingPath + "/" + strings.Repeat("a", 32), licenceClassDelete},
		{"PUT", ownEmailPath, licenceClassAccount},
		{"POST", "/api/v1/auth/2fa/disable", licenceClassAccount},
		{"POST", panelUpdateStartPath, licenceClassUpdate},
		{"PUT", panelUpdateDiscoveryPath, licenceClassUpdate},
		{"POST", panelLicensePath, licenceClassUpdate},
		{"POST", "/api/v1/dns/remote/receiver/publish", licenceClassPair},
		{"POST", "/api/v1/dns/remote/enrollments", licenceClassPair},
		{"POST", serverSetupPeerConnectPath, licenceClassPair},
		{"POST", "/api/v1/domains/create", licenceClassGrowth},
		{"POST", "/api/v1/users", licenceClassGrowth},
		{"POST", "/api/v1/team-members", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/mail/accounts", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/mail/forwardings", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/databases", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/databases/5/copy", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/aliases", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/access/logins", licenceClassGrowth},
		{"POST", "/api/v1/domains/9/apps/install", licenceClassGrowth},
		{"POST", "/api/v1/database-servers/3/databases", licenceClassGrowth},
		{"POST", "/api/v1/import/cpanel/apply", licenceClassGrowth},
		{"POST", "/api/v1/subscriptions/9/entitlements", licenceClassGrowth},
		{"PUT", "/api/v1/domains/9/mail/accounts", licenceClassChange},
		{"PUT", "/api/v1/domains/9/hosting", licenceClassChange},
		{"POST", "/api/v1/service/install", licenceClassChange},
		{"POST", serverSetupPath + "/start", licenceClassChange},
		{"GET", "/dbtool/phpmyadmin/", licenceClassChange},
		{"POST", "/api/v1/users/9/plan", licenceClassChange},
	} {
		got, ok := licenceRouteClassOf(httptest.NewRequest(tc.method, tc.path, nil))
		if !ok || got != tc.want {
			t.Errorf("%s %s: class %q (%v), want %q", tc.method, tc.path, got, ok, tc.want)
		}
	}
	// An escaped path is never sorted towards an open class.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/firewall%2Fx", nil)
	if got, _ := licenceRouteClassOf(r); got != licenceClassChange {
		t.Fatalf("escaped path classified %q", got)
	}
}

// ---------- the gate on the production path ----------

type licenceGateSample struct {
	method, path string
	class        licenceRouteClass
}

var licenceGateSamples = []licenceGateSample{
	{"PUT", ownEmailPath, licenceClassAccount},
	{"POST", "/api/v1/auth/2fa/disable", licenceClassAccount},
	{"POST", allSessionEndPath, licenceClassAccount},
	{"POST", panelUpdateStartPath, licenceClassUpdate},
	{"PUT", panelUpdateDiscoveryPath, licenceClassUpdate},
	{"POST", panelLicensePath, licenceClassUpdate},
	{"POST", "/api/v1/firewall", licenceClassSecurity},
	{"POST", osUpdatesPath, licenceClassSecurity},
	{"POST", osUpdatesOriginsPath, licenceClassSecurity},
	{"PUT", osUpdatesModePath, licenceClassSecurity},
	{"POST", securityEmergenciesPath, licenceClassSecurity},
	{"PUT", databaseRemotePath + "server", licenceClassSecurity},
	{"POST", "/api/v1/fail2ban/banned", licenceClassSecurity},
	{"POST", "/api/v1/domains/9902/ssl/letsencrypt", licenceClassSecurity},
	{"POST", "/api/v1/domains/9902/backups", licenceClassBackup},
	{"POST", "/api/v1/domains/9902/backups/restore", licenceClassBackup},
	{"GET", "/api/v1/domains/9902/backups/download", licenceClassRead},
	{"POST", remoteBackupPathPrefix + "destinations", licenceClassBackup},
	{"DELETE", "/api/v1/domains/9902", licenceClassDelete},
	{"DELETE", "/api/v1/domains/9902/mail/accounts", licenceClassDelete},
	{"DELETE", "/api/v1/databases/5", licenceClassDelete},
	{"POST", "/api/v1/dns/remote/enrollments", licenceClassPair},
	{"DELETE", "/api/v1/dns/remote/connections", licenceClassPair},
	{"POST", serverSetupJoinCodesPath, licenceClassPair},
	{"GET", "/api/v1/domains", licenceClassRead},
	{"GET", "/api/v1/firewall", licenceClassRead},
	{"POST", "/api/v1/import/cpanel/inspect", licenceClassRead},
	{"POST", "/api/v1/service/install", licenceClassChange},
	{"PUT", "/api/v1/domains/9902/hosting", licenceClassChange},
	{"POST", "/api/v1/domains/9902/dns/records", licenceClassChange},
	{"POST", serverSetupPath + "/start", licenceClassChange},
	{"GET", "/dbtool/phpmyadmin/", licenceClassChange},
	{"POST", "/api/v1/domains/create", licenceClassGrowth},
	{"POST", "/api/v1/users", licenceClassGrowth},
	{"POST", "/api/v1/domains/9902/mail/accounts", licenceClassGrowth},
	{"POST", "/api/v1/domains/9902/databases", licenceClassGrowth},
	{"POST", "/api/v1/database-servers/3/databases", licenceClassGrowth},
	{"POST", "/api/v1/import/cpanel/apply", licenceClassGrowth},
}

// licenceTestStates: every licence state the panel knows, plus a licence that
// ends within 30 days (expiring) and an unconfigured manager.
var licenceTestStates = []string{"active", "expiring", "expired", "missing", "invalid", "rejected", "verification_unavailable", "unconfigured"}

func licenceTestManager(t *testing.T, state string) *licensing.Manager {
	t.Helper()
	switch state {
	case "unconfigured":
		return nil
	case "expiring":
		// testPanelLicense's active receipt ends one day from now.
		return testPanelLicense(t, "active")
	case "active":
		return testPanelLicense(t, "active_year")
	}
	return testPanelLicense(t, state)
}

func licenceTestWantRefusal(state string) (int, string) {
	switch state {
	case "active", "expiring":
		return 0, ""
	case "verification_unavailable":
		return http.StatusServiceUnavailable, errCodeLicenseVerificationUnavailable
	case "unconfigured":
		return http.StatusServiceUnavailable, errCodeLicenseStatusUnavailable
	}
	return http.StatusForbidden, "license_required"
}

// Every class in every licence state for every role, through requireAuth
// (sessions; API tokens pass the same admitCallerGates).
func TestLicenceGateAdmitsByClassInEveryStateAndRole(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	for _, state := range licenceTestStates {
		t.Run(state, func(t *testing.T) {
			fixture.panel.license = licenceTestManager(t, state)
			wantStatus, wantCode := licenceTestWantRefusal(state)
			reached := 0
			handler := fixture.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(204) }))
			for _, id := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, authzMatrixAdditionalUserID} {
				for _, sample := range licenceGateSamples {
					before := reached
					w := requestWithToken(handler, sample.method, sample.path, fixture.tokens[id])
					if id != authzMatrixAdminID && isAdminOnlyPath(sample.path) {
						if w.Code != http.StatusForbidden || reached != before || strings.Contains(w.Body.String(), "license") {
							t.Fatalf("role %d %s %s: not refused by role: %d %s", id, sample.method, sample.path, w.Code, w.Body.String())
						}
						continue
					}
					if id == authzMatrixAdditionalUserID && isAdditionalUserRestrictedPath(sample.path) {
						continue
					}
					locked := sample.class.needsFullLicence() && wantStatus != 0
					if !locked {
						if reached != before+1 || w.Code != 204 {
							t.Fatalf("role %d %s %s (%s): refused in state %s: %d %s", id, sample.method, sample.path, sample.class, state, w.Code, w.Body.String())
						}
						continue
					}
					var problem struct{ Code, Error string }
					_ = json.Unmarshal(w.Body.Bytes(), &problem)
					if reached != before || w.Code != wantStatus || problem.Code != wantCode {
						t.Fatalf("role %d %s %s (%s): state %s answered %d %s", id, sample.method, sample.path, sample.class, state, w.Code, w.Body.String())
					}
					admin := id == authzMatrixAdminID
					if admin != strings.Contains(strings.ToLower(problem.Error), "licen") {
						t.Fatalf("role %d: the refusal names the licence wrongly: %s", id, problem.Error)
					}
				}
			}
		})
	}
}

// The owner's named security actions are accepted while the licence has
// ended; a growth action is refused with the code and the sentence.
func TestLicenceExpiredLeavesSecurityActionsAndRefusesGrowth(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	fixture.panel.license = testPanelLicense(t, "expired")
	reached := map[string]int{}
	handler := fixture.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached[r.Method+" "+r.URL.Path]++
		w.WriteHeader(204)
	}))
	admin := fixture.tokens[authzMatrixAdminID]
	for _, item := range []struct{ method, path string }{
		{"POST", "/api/v1/firewall"},
		{"GET", osUpdatesPath},
		{"GET", osUpdatesSummaryPath},
		{"POST", osUpdatesPath},
		{"POST", osUpdatesOriginsPath},
		{"PUT", osUpdatesModePath},
		{"PUT", databaseRemotePath + "server"},
		{"DELETE", databaseRemotePath + "rule"},
		{"POST", securityEmergenciesPath},
		{"POST", securityEmergenciesPath + "/1/stop"},
	} {
		w := requestWithToken(handler, item.method, item.path, admin)
		if w.Code != 204 || reached[item.method+" "+item.path] != 1 {
			t.Fatalf("%s %s refused while the licence has ended: %d %s", item.method, item.path, w.Code, w.Body.String())
		}
	}
	w := requestWithToken(handler, "POST", "/api/v1/domains/create", admin)
	var problem struct{ Code, Error string }
	_ = json.Unmarshal(w.Body.Bytes(), &problem)
	if w.Code != http.StatusForbidden || problem.Code != "license_required" || reached["POST /api/v1/domains/create"] != 0 ||
		!strings.Contains(problem.Error, "Creating new things and changing settings need an active server license") ||
		!strings.Contains(problem.Error, "security, backups, deleting, panel updates and your own account still work") {
		t.Fatalf("growth while ended: %d %s", w.Code, w.Body.String())
	}
	w = requestWithToken(handler, "POST", "/api/v1/domains/9902/mail/accounts", fixture.tokens[authzMatrixCustomerID])
	problem.Code, problem.Error = "", ""
	_ = json.Unmarshal(w.Body.Bytes(), &problem)
	if w.Code != http.StatusForbidden || problem.Code != "license_required" ||
		!strings.HasPrefix(problem.Error, "New domains, mailboxes and databases cannot be created") || strings.Contains(strings.ToLower(problem.Error), "licen") {
		t.Fatalf("customer growth while ended: %d %s", w.Code, w.Body.String())
	}
}

// ---------- the count check ----------

func TestLicenceCountCheckRefusesGrowthIndependentlyOfRoutes(t *testing.T) {
	// The production reading, also for a Panel without a manager.
	previous := countCheckLicenceStatus
	countCheckLicenceStatus = productionCountCheckLicenceStatus
	t.Cleanup(func() { countCheckLicenceStatus = previous })
	fixture := newAuthzMatrixFixture(t)
	ctx := context.WithValue(context.Background(), callerKey, &Caller{ID: authzMatrixCustomerID, Role: roleCustomer})
	for _, state := range licenceTestStates {
		fixture.panel.license = licenceTestManager(t, state)
		wantStatus, wantCode := licenceTestWantRefusal(state)
		for _, kind := range []quotaKind{quotaDomains, quotaDBs, quotaMail, quotaDisk} {
			err := fixture.panel.checkSubscriptionQuota(ctx, 9802, kind)
			var licence *licenceGrowthRefusal
			refused := errors.As(err, &licence)
			if want := wantStatus != 0 && kind != quotaDisk; refused != want {
				t.Fatalf("%s %s: licence refusal %v (%v)", state, kind, refused, err)
			}
			if !refused {
				continue
			}
			w := httptest.NewRecorder()
			writeQuotaRefusal(w, err)
			var problem struct{ Code, Error string }
			_ = json.Unmarshal(w.Body.Bytes(), &problem)
			if w.Code != wantStatus || problem.Code != wantCode || strings.Contains(strings.ToLower(problem.Error), "licen") {
				t.Fatalf("%s %s: answered %d %s", state, kind, w.Code, w.Body.String())
			}
		}
	}
	// The production reading: a Panel without a manager is unknown, never
	// full use.
	status := productionCountCheckLicenceStatus(&Panel{})
	if status.CanProvision || status.Observation != licensing.ObservationUnavailable {
		t.Fatalf("a missing manager reads as %+v", status)
	}
	// The visible-databases answer says "no new database" instead of failing.
	fixture.panel.license = testPanelLicense(t, "expired")
	r := httptest.NewRequest(http.MethodGet, databasesVisiblePath, nil)
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: authzMatrixAdminID, Role: roleAdmin}))
	if may, err := fixture.panel.callerMayCreateDatabase(r.Context(), r, nil, true); err != nil || may {
		t.Fatalf("may_create while ended: %v %v", may, err)
	}
}

// ---------- what the browser reads ----------

func TestLicenceAccessAnswerLevelAndTermEnd(t *testing.T) {
	for _, state := range licenceTestStates {
		p := &Panel{license: licenceTestManager(t, state)}
		for _, role := range []string{roleAdmin, roleReseller, roleCustomer} {
			r := httptest.NewRequest(http.MethodGet, panelLicenseAccessPath, nil)
			r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{ID: 1, Role: role}))
			w := httptest.NewRecorder()
			p.handleLicenseAccess(w, r)
			var answer map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			full := state == "active" || state == "expiring"
			if (answer["level"] == "full") != full || (answer["level"] == "limited") == full || answer["can_use_panel"] != full {
				t.Fatalf("%s %s: %v", state, role, answer)
			}
			left, present := answer["term_ends_in_seconds"].(float64)
			hasTerm := state != "missing" && state != "invalid" && state != "unconfigured"
			if present != (role == roleAdmin && hasTerm) {
				t.Fatalf("%s %s: term end shown=%v: %v", state, role, present, answer)
			}
			if present && ((state == "expiring" && (left <= 0 || left > 86400)) || (state == "active" && left < 300*86400) || (state == "expired" && left != 0)) {
				t.Fatalf("%s: term end %v", state, left)
			}
		}
	}
}
