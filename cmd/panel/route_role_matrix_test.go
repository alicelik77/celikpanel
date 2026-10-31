package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The route × role matrix (ROADMAP B2 addendum, alpha.114). The test reads
// the REAL route table — every http.HandleFunc/http.Handle registration in
// main.go plus registerRecoveryRoutes — from the package source, and sends a
// request for every route as an anonymous visitor, the administrator, a
// reseller, a customer and a team member (additional user) through the real
// authentication middleware. Each route must be listed in routeRoleMatrix
// with the gate it has; a route registered without a row, or a row whose
// route is gone, fails the test. Handler-level rules (manager-only account
// routes, ownership filters) are listed beside each row and driven with the
// real handlers in TestAccountRoutesRoleMatrix.
//
// Rota × rol matrisi. Test GERÇEK rota tablosunu kaynaktan okur ve her rotayı
// anonim, yönetici, bayi, müşteri ve ek kullanıcı olarak gerçek kimlik
// doğrulama ara katmanından geçirir. Satırı olmayan yeni rota testi düşürür.

// Gate classes (what the middleware does before any handler):
//   - public:  everyone reaches the handler, signed in or not
//   - admin:   anonymous 401; only the administrator reaches the handler
//   - account: anonymous 401; administrator, reseller and customer reach it;
//     a team member is refused (additional-user scope)
//   - tenant:  anonymous 401; every signed-in role reaches the handler, which
//     filters by ownership (and by team-member grants)
const (
	gatePublic  = "public"
	gateAdmin   = "admin"
	gateAccount = "account"
	gateTenant  = "tenant"
)

// routeRoleMatrix: registered pattern → gate. The comment after a row names
// the handler's own rule where it narrows the gate further.
var routeRoleMatrix = map[string]string{
	"/":                             gatePublic, // the web interface (SPA)
	"/api/v1/admin/store-catalog":   gateAdmin,
	"/api/v1/admin/store-catalog/":  gateAdmin,
	"/api/v1/apps":                  gateTenant, // catalog read; install is ownership-checked
	"/api/v1/audit-logs":            gateAdmin,
	"/api/v1/auth/2fa/disable":      gateTenant, // own account only
	"/api/v1/auth/2fa/enable":       gateTenant, // own account only
	"/api/v1/auth/2fa/setup":        gateTenant, // own account only
	"/api/v1/auth/2fa/status":       gateTenant, // own account only
	"/api/v1/auth/demo":             gatePublic, // empty unless --demo
	"/api/v1/auth/login":            gatePublic,
	"/api/v1/auth/login/totp":       gatePublic,
	"/api/v1/auth/logout":           gateTenant,
	"/api/v1/auth/me":               gateTenant,
	"/api/v1/auth/password":         gateTenant, // own account only
	"/api/v1/auth/unimpersonate":    gateTenant, // only with the operator's own session cookie
	"/api/v1/config":                gateAdmin,
	"/api/v1/config/mysql":          gateAdmin,
	"/api/v1/config/php":            gateAdmin,
	"/api/v1/dashboard":             gateAdmin,
	"/api/v1/database-grants/":      gateTenant, // ownership-checked
	"/api/v1/database-servers":      gateTenant, // registration admin-only in handler
	"/api/v1/database-servers/":     gateTenant, // ownership-checked
	"/api/v1/database-users/":       gateTenant, // ownership-checked
	"/api/v1/databases/":            gateTenant, // ownership-checked
	"/api/v1/domains":               gateTenant, // ownership-filtered list
	"/api/v1/domains/":              gateTenant, // single ownership chokepoint + team grants
	"/api/v1/domains/create":        gateAccount,
	"/api/v1/dovecot/stats":         gateAdmin,
	"/api/v1/fail2ban/banned":       gateAdmin,
	"/api/v1/fail2ban/config":       gateAdmin,
	"/api/v1/fail2ban/jails":        gateAdmin,
	"/api/v1/firewall":              gateAdmin,
	"/api/v1/hosting/capabilities":  gateAccount,
	"/api/v1/import/cpanel/apply":   gateAdmin,
	"/api/v1/import/cpanel/inspect": gateAdmin,
	"/api/v1/mail/configure":        gateAdmin,
	"/api/v1/mail/policy":           gateAdmin,
	"/api/v1/managed-services":      gateAdmin,
	"/api/v1/managed-services/scan": gateAdmin,
	"/api/v1/metrics/history":       gateAdmin,
	"/api/v1/my/subscription":       gateAccount, // own subscriptions only
	"/api/v1/nginx/global":          gateAdmin,
	"/api/v1/nginx/ratelimits":      gateAdmin,
	"/api/v1/nginx/ssl":             gateAdmin,
	"/api/v1/panel/certificate":     gateAdmin,
	"/api/v1/panel/version":         gateAdmin,
	"/api/v1/pdns/enable":           gateAdmin,
	"/api/v1/php/config":            gateAdmin,
	"/api/v1/php/extended-config":   gateAdmin,
	"/api/v1/php/extensions":        gateAdmin,
	"/api/v1/php/pool-config":       gateAdmin,
	"/api/v1/php/pools":             gateAdmin,
	"/api/v1/plans":                 gateAccount, // admin + reseller (handler); reseller sees global + own
	"/api/v1/plans/":                gateAccount, // admin any plan, reseller own plans (handler)
	"/api/v1/postfix/queue":         gateAdmin,
	"/api/v1/postfix/summary":       gateAdmin,
	"/api/v1/products":              gateAccount,
	"/api/v1/repo":                  gateAdmin,
	"/api/v1/runtimes/node":         gateAccount, // admin-only in handler
	"/api/v1/runtimes/node/":        gateAccount, // admin-only in handler
	"/api/v1/service/action":        gateAdmin,
	"/api/v1/service/candidate":     gateAdmin,
	"/api/v1/service/install":       gateAdmin,
	"/api/v1/service/logs":          gateAdmin,
	"/api/v1/service/operation":     gateAdmin,
	"/api/v1/service/status":        gateAdmin,
	"/api/v1/settings/dns-cluster":  gateTenant, // admin-only in handler
	"/api/v1/settings/dns-setup":    gateTenant, // admin-only in handler
	"/api/v1/settings/nameservers":  gateTenant, // admin-only in handler
	"/api/v1/ssl/providers":         gateTenant,
	"/api/v1/store":                 gateAccount,
	"/api/v1/store/":                gateAccount,
	"/api/v1/subscriptions":         gateAccount, // ownership-filtered list
	"/api/v1/subscriptions/":        gateAccount, // ownership-checked entitlements
	"/api/v1/system-databases":      gateAdmin,
	"/api/v1/system-databases/":     gateAdmin,
	"/api/v1/system/check":          gateAdmin,
	"/api/v1/system/stats":          gateAccount,
	"/api/v1/team-members":          gateAccount, // customers manage their own team (handler)
	"/api/v1/team-members/":         gateAccount, // customers manage their own team (handler)
	"/api/v1/users":                 gateAccount, // admin + reseller (handler)
	"/api/v1/users/":                gateAccount, // admin any, reseller own customers (handler)
	"/api/v1/vpn/peers":             gateAccount, // ownership-checked
	"/api/v1/vpn/peers/":            gateAccount, // ownership-checked
	"/api/v1/vpn/setup":             gateAccount, // admin-only in handler
	"/api/v1/vpn/status":            gateAccount,
	"/api/v1/vpn/sync":              gateAccount, // admin-only in handler
	"/dbtool/":                      gateAccount, // engine credentials are the real gate
	"/webmail/":                     gatePublic,  // a redirect to webmail's own address

	// Own account, read-only facts and handler-checked surfaces.
	"/api/v1/auth/email":                 gateTenant, // own address only
	"/api/v1/host-mutation-readiness":    gateTenant, // read-only readiness
	"/api/v1/license/access":             gateTenant, // the caller's own access state
	"/api/v1/maintenance/restarts":       gateTenant, // admin-only in handler
	"/api/v1/offered-versions":           gateTenant, // read for all; changed by admin (handler)
	"/api/v1/panel/access-address":       gatePublic, // GET is public (how to reach the panel)
	"/api/v1/panel/availability":         gateTenant, // recovery availability (D-025)
	"/api/v1/server/os-release":          gateTenant, // admin-only writes in handler
	"/api/v1/server/os-release/reviewed": gateTenant, // admin-only in handler
	"/api/v1/support-table":              gateTenant, // the dated support table, for everyone signed in

	// Server, DNS infrastructure, setup and updates: administrator only.
	"/api/v1/dns/engine":                       gateAdmin,
	"/api/v1/dns/engine/reconcile":             gateAdmin,
	"/api/v1/dns/engine/switch":                gateAdmin,
	"/api/v1/dns/engine/switch/preview":        gateAdmin,
	"/api/v1/dns/infrastructure-zones":         gateAdmin,
	"/api/v1/dns/infrastructure-zones/":        gateAdmin,
	"/api/v1/dns/overview":                     gateAdmin,
	"/api/v1/dns/remote/accept":                gateAdmin, // machine-verified peers bypass sessions
	"/api/v1/dns/remote/clients":               gateAdmin,
	"/api/v1/dns/remote/connections":           gateAdmin,
	"/api/v1/dns/remote/enrollments":           gateAdmin,
	"/api/v1/dns/remote/receiver/host-record":  gateAdmin, // machine-verified peers bypass sessions
	"/api/v1/dns/remote/receiver/publish":      gateAdmin, // machine-verified peers bypass sessions
	"/api/v1/dns/remote/receiver/status":       gateAdmin, // machine-verified peers bypass sessions
	"/api/v1/nginx/default-https":              gateAdmin,
	"/api/v1/panel/license":                    gateAdmin,
	"/api/v1/panel/update/abandon":             gateAdmin,
	"/api/v1/panel/update/check":               gateAdmin,
	"/api/v1/panel/update/discovery":           gateAdmin,
	"/api/v1/panel/update/discovery/refresh":   gateAdmin,
	"/api/v1/panel/update/start":               gateAdmin,
	"/api/v1/panel/update/status":              gateAdmin,
	"/api/v1/recovery/status":                  gateAdmin,
	"/api/v1/repo/confine-preview":             gateAdmin,
	"/api/v1/repo/lane":                        gateAdmin,
	"/api/v1/security/audit":                   gateAdmin,
	"/api/v1/security/os-updates":              gateAdmin,
	"/api/v1/security/os-updates/origins":      gateAdmin,
	"/api/v1/service/install-preview":          gateAdmin,
	"/api/v1/service/php-tools":                gateAdmin,
	"/api/v1/service/php-tools/bind":           gateAdmin,
	"/api/v1/service/php-tools/prove":          gateAdmin,
	"/api/v1/service/postgresql/":              gateAdmin,
	"/api/v1/service/profile/install":          gateAdmin,
	"/api/v1/service/removal-impact":           gateAdmin,
	"/api/v1/service/roundcube/address":        gateAdmin,
	"/api/v1/service/roundcube/address/check":  gateAdmin,
	"/api/v1/service/roundcube/address/remove": gateAdmin,
	"/api/v1/service/roundcube/update":         gateAdmin,
	"/api/v1/service/uninstall":                gateAdmin,
	"/api/v1/setup":                            gateAdmin,
	"/api/v1/setup/complete":                   gateAdmin,
	"/api/v1/setup/components":                 gateAdmin,
	"/api/v1/setup/delegation":                 gateAdmin,
	"/api/v1/setup/extend":                     gateAdmin,
	"/api/v1/setup/guidance":                   gateAdmin,
	"/api/v1/setup/join-codes":                 gateAdmin,
	"/api/v1/setup/mail-enrollment/continue":   gateAdmin,
	"/api/v1/setup/mail-enrollment/retry":      gateAdmin,
	"/api/v1/setup/operation":                  gateAdmin,
	"/api/v1/setup/package-updates":            gateAdmin,
	"/api/v1/setup/peer":                       gateAdmin,
	"/api/v1/setup/peer/connect":               gateAdmin,
	"/api/v1/setup/peer/retire":                gateAdmin,
	"/api/v1/setup/plan":                       gateAdmin,
	"/api/v1/setup/provider-records":           gateAdmin,
	"/api/v1/setup/public-records":             gateAdmin,
	"/api/v1/setup/publisher":                  gateAdmin,
	"/api/v1/setup/revise":                     gateAdmin,
	"/api/v1/setup/start":                      gateAdmin,
}

// registeredPanelRoutes parses the package source and returns every route
// pattern main registers, with constants resolved.
func registeredPanelRoutes(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	consts := map[string]ast.Expr{}
	files := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, path, source, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = file
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i < len(value.Values) {
						consts[name.Name] = value.Values[i]
					}
				}
			}
		}
	}
	var eval func(ast.Expr, int) (string, bool)
	eval = func(expr ast.Expr, depth int) (string, bool) {
		if depth > 20 {
			return "", false
		}
		switch e := expr.(type) {
		case *ast.BasicLit:
			if e.Kind != token.STRING {
				return "", false
			}
			s, err := strconv.Unquote(e.Value)
			return s, err == nil
		case *ast.Ident:
			value, ok := consts[e.Name]
			if !ok {
				return "", false
			}
			return eval(value, depth+1)
		case *ast.ParenExpr:
			return eval(e.X, depth+1)
		case *ast.BinaryExpr:
			if e.Op != token.ADD {
				return "", false
			}
			left, ok := eval(e.X, depth+1)
			if !ok {
				return "", false
			}
			right, ok := eval(e.Y, depth+1)
			return left + right, ok
		}
		return "", false
	}
	routes := map[string]bool{}
	collect := func(node ast.Node, receiver string) {
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "HandleFunc" && selector.Sel.Name != "Handle") {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); !ok || ident.Name != receiver {
				return true
			}
			path, ok := eval(call.Args[0], 0)
			if !ok {
				t.Errorf("%s: a route pattern this test cannot resolve: %T", fset.Position(call.Pos()), call.Args[0])
				return true
			}
			routes[path] = true
			return true
		})
	}
	mainFile, ok := files["main.go"]
	if !ok {
		t.Fatal("main.go not found")
	}
	collect(mainFile, "http")
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "registerRecoveryRoutes" {
				collect(fn.Body, "mux")
			}
		}
	}
	out := make([]string, 0, len(routes))
	for route := range routes {
		out = append(out, route)
	}
	sort.Strings(out)
	return out
}

// gateOf classifies the five answers (anonymous, admin, reseller, customer,
// team member) of one route.
func gateOf(codes [5]int) string {
	pass := func(code int) bool { return code == http.StatusNoContent }
	switch {
	case pass(codes[0]) && pass(codes[1]) && pass(codes[2]) && pass(codes[3]) && pass(codes[4]):
		return gatePublic
	case codes[0] == http.StatusUnauthorized && pass(codes[1]) && pass(codes[2]) && pass(codes[3]) && pass(codes[4]):
		return gateTenant
	case codes[0] == http.StatusUnauthorized && pass(codes[1]) && pass(codes[2]) && pass(codes[3]) && codes[4] == http.StatusForbidden:
		return gateAccount
	case codes[0] == http.StatusUnauthorized && pass(codes[1]) && codes[2] == http.StatusForbidden &&
		codes[3] == http.StatusForbidden && codes[4] == http.StatusForbidden:
		return gateAdmin
	}
	return fmt.Sprintf("unexpected %v", codes)
}

func TestRouteRoleMatrixCoversTheRealRouteTable(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	memberToken := seedAdditionalUserSession(t, &fixture)
	pass := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := fixture.panel.requireAuth(pass)

	registered := registeredPanelRoutes(t)
	if len(registered) < 100 {
		t.Fatalf("only %d routes were found in main.go; the parser lost the route table", len(registered))
	}
	seen := map[string]bool{}
	dump := []string{}
	for _, route := range registered {
		seen[route] = true
		var codes [5]int
		for i, token := range []string{"", fixture.tokens[authzMatrixAdminID], fixture.tokens[authzMatrixResellerID],
			fixture.tokens[authzMatrixCustomerID], memberToken} {
			request := httptest.NewRequest(http.MethodGet, route, nil)
			if token != "" {
				request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			codes[i] = recorder.Code
		}
		got := gateOf(codes)
		dump = append(dump, fmt.Sprintf("%q: %q,", route, got))
		want, listed := routeRoleMatrix[route]
		if !listed {
			t.Errorf("route %s is registered in main.go but missing from routeRoleMatrix (its gate is %s); add a row", route, got)
			continue
		}
		if got != want {
			t.Errorf("route %s: gate %s, matrix says %s", route, got, want)
		}
	}
	for route := range routeRoleMatrix {
		if !seen[route] {
			t.Errorf("routeRoleMatrix lists %s, which main.go no longer registers; remove the row", route)
		}
	}
	if t.Failed() && os.Getenv("CELIKPANEL_ROUTE_MATRIX_DUMP") != "" {
		t.Log("\n" + strings.Join(dump, "\n"))
	}
}
