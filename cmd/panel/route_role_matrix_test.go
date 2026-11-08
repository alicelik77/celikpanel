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

	"github.com/alicelik/celikpanel/internal/core"
)

// The route × role matrix (ROADMAP B2 addendum, alpha.114). The test reads
// the REAL route table — every http.HandleFunc/http.Handle registration in
// main.go plus registerRecoveryRoutes and registerAccountRecoveryRoutes — from
// the package source, and sends a
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
	"/":                              gatePublic, // the web interface (SPA)
	"/api/v1/admin/store-catalog":    gateAdmin,
	"/api/v1/admin/store-catalog/":   gateAdmin,
	"/api/v1/admin/service-apps":     gateAdmin, // alpha.118: offering AI and automation apps
	"/api/v1/admin/service-apps/":    gateAdmin,
	"/api/v1/admin/runtime-apps":     gateAdmin, // alpha.119: offering runtime apps
	"/api/v1/admin/runtime-apps/":    gateAdmin,
	"/api/v1/apps":                   gateTenant, // catalog read; install is ownership-checked
	"/api/v1/audit-logs":             gateAdmin,
	"/api/v1/auth/2fa/disable":       gateTenant, // own account only
	"/api/v1/auth/2fa/enable":        gateTenant, // own account only
	"/api/v1/auth/2fa/setup":         gateTenant, // own account only
	"/api/v1/auth/2fa/status":        gateTenant, // own account only
	"/api/v1/auth/demo":              gatePublic, // empty unless --demo
	"/api/v1/auth/login":             gatePublic,
	"/api/v1/auth/login/totp":        gatePublic,
	"/api/v1/auth/logout":            gateTenant,
	"/api/v1/auth/me":                gateTenant,
	"/api/v1/auth/password":          gateTenant, // own account only
	"/api/v1/auth/unimpersonate":     gateTenant, // only with the operator's own session cookie
	"/api/v1/config":                 gateAdmin,
	"/api/v1/config/mysql":           gateAdmin,
	"/api/v1/config/php":             gateAdmin,
	"/api/v1/dashboard":              gateAdmin,
	"/api/v1/database-grants/":       gateTenant,  // ownership-checked
	"/api/v1/database-servers":       gateTenant,  // registration admin-only in handler
	"/api/v1/database-servers/":      gateTenant,  // ownership-checked
	"/api/v1/database-users/":        gateTenant,  // ownership-checked
	"/api/v1/database-remote/":       gateAccount, // alpha.129: ownership-checked; server switch administrator only
	"/api/v1/databases/":             gateTenant,  // ownership-checked
	"/api/v1/domains":                gateTenant,  // ownership-filtered list
	"/api/v1/domains/":               gateTenant,  // single ownership chokepoint + team grants
	"/api/v1/domains/create":         gateAccount,
	"/api/v1/dovecot/stats":          gateAdmin,
	"/api/v1/fail2ban/banned":        gateAdmin,
	"/api/v1/fail2ban/config":        gateAdmin,
	"/api/v1/fail2ban/jails":         gateAdmin,
	"/api/v1/firewall":               gateAdmin,
	"/api/v1/hosting/capabilities":   gateAccount,
	"/api/v1/import/cpanel/apply":    gateAdmin,
	"/api/v1/import/cpanel/inspect":  gateAdmin,
	"/api/v1/mail/configure":         gateAdmin,
	"/api/v1/mail/passwordless":      gateAdmin,
	"/api/v1/mail/outgoing":          gateAdmin,
	"/api/v1/mail/relay":             gateAdmin,
	"/api/v1/mail/relay/test":        gateAdmin,
	"/api/v1/mail/server-settings":   gateAdmin,
	"/api/v1/mail/subaddressing":     gateAdmin,
	"/api/v1/mail/sender-rules":      gateAdmin,
	"/api/v1/mail/outgoing/report":   gateAdmin,
	"/api/v1/mail/plan-limits":       gateAdmin,
	"/api/v1/mail-outgoing-notices":  gateTenant, // the caller's own notices
	"/api/v1/mail-outgoing-notices/": gateTenant, // marks one of the caller's own as seen
	"/api/v1/mail/policy":            gateAdmin,
	"/api/v1/managed-services":       gateAdmin,
	"/api/v1/managed-services/scan":  gateAdmin,
	"/api/v1/metrics/history":        gateAdmin,
	"/api/v1/metrics/problems":       gateAdmin,

	// alpha.128: offering additional nginx directives (admin through /api/v1/nginx/).
	"/api/v1/nginx/directives-offer": gateAdmin,

	// alpha.127: monitoring stage 2 and 3 (admin through the /api/v1/metrics/ prefix).
	"/api/v1/metrics/services":        gateAdmin,
	"/api/v1/metrics/sites":           gateAdmin,
	"/api/v1/metrics/alerts":          gateAdmin,
	"/api/v1/metrics/alerts/settings": gateAdmin,
	"/api/v1/metrics/alerts/seen":     gateAdmin,

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
	"/api/v1/network/ip-addresses":  gateAdmin,
	"/api/v1/postfix/summary":       gateAdmin,
	"/api/v1/products":              gateAccount,
	"/api/v1/reseller-plans":        gateAccount, // administrator only (handler)
	"/api/v1/reseller-plans/":       gateAccount, // administrator only (handler)
	"/api/v1/rights/":               gateAccount, // catalog for each account's own scope; notices admin + reseller (handler)
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
	"/api/v1/security-emergencies":  gateAdmin,   // D-043; also refused under "Log in as" (handler)
	"/api/v1/security-emergencies/": gateAdmin,   // D-043; steps need name + password (handler)
	"/api/v1/security-notices":      gateTenant,  // the caller's own notices only (handler)
	"/api/v1/security-notices/":     gateTenant,  // marks the caller's own notice seen (handler)
	"/api/v1/usage-notices":         gateTenant,  // D-044: the caller's own notices only (handler)
	"/api/v1/usage-notices/":        gateTenant,  // D-044: marks the caller's own notice seen (handler)
	"/api/v1/usage/status":          gateTenant,  // D-044: administrator only (handler)
	"/api/v1/suspensions/deletable": gateAccount, // admin + reseller (handler); customers refused
	"/api/v1/transfer-requests":     gateAccount, // admin all, reseller its own, customer its own (handler)
	"/api/v1/transfer-requests/":    gateAccount, // approver decides, requester withdraws (handler)
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
	"/api/v1/me/guide-state":             gateTenant, // alpha.119: the caller's own tour state only
	"/api/v1/me/activity":                gateTenant, // alpha.128 ops G23: own rows (+ customers' for a reseller) only (handler)
	"/api/v1/maintenance/restarts":       gateTenant, // admin-only in handler
	"/api/v1/offered-versions":           gateTenant, // read for all; changed by admin (handler)
	"/api/v1/panel/access-address":       gatePublic, // GET is public (how to reach the panel)
	"/api/v1/panel/availability":         gateTenant, // recovery availability (D-025)
	"/api/v1/server/os-release":          gateTenant, // admin-only writes in handler
	"/api/v1/server/os-release/reviewed": gateTenant, // admin-only in handler
	"/api/v1/support-table":              gateTenant, // the dated support table, for everyone signed in

	// alpha.128 ops: G29 server maintenance.
	"/api/v1/maintenance/clock":     gateTenant, // admin-only in handler
	"/api/v1/maintenance/processes": gateTenant, // admin-only in handler
	"/api/v1/maintenance/reboot":    gateTenant, // admin-only in handler
	// alpha.128 ops: G31 certificates overview and requests.
	"/api/v1/maintenance/certificates":          gateTenant, // admin-only in handler
	"/api/v1/maintenance/certificate-requests":  gateTenant, // admin-only in handler
	"/api/v1/maintenance/certificate-requests/": gateTenant, // admin-only in handler

	// alpha.114: forgot password, invitations, address confirmation, recovery codes.
	"/api/v1/auth/2fa/recovery/regenerate":    gateTenant, // own account only (signed-in self)
	"/api/v1/auth/email/verification":         gateTenant, // own address only
	"/api/v1/auth/recovery/accept-invitation": gatePublic, // POST with a single-use link token
	"/api/v1/auth/recovery/link":              gatePublic, // POST with a single-use link token
	"/api/v1/auth/recovery/mail-status":       gatePublic, // GET: can this panel send mail at all
	"/api/v1/auth/recovery/request":           gatePublic, // POST, rate-limited, same answer for every name
	"/api/v1/auth/recovery/reset":             gatePublic, // POST with a single-use link token
	"/api/v1/auth/recovery/verify-email":      gatePublic, // POST with a single-use link token
	// alpha.130: a mailbox's forgotten password (mailbox_recovery_handlers.go).
	"/api/v1/auth/mailbox-recovery/request": gatePublic,  // POST, rate-limited, same answer for every address
	"/api/v1/auth/mailbox-recovery/confirm": gatePublic,  // POST with a single-use link token
	"/api/v1/auth/mailbox-recovery/link":    gatePublic,  // POST with a single-use link token
	"/api/v1/auth/mailbox-recovery/reset":   gatePublic,  // POST with a single-use link token
	"/api/v1/invitations":                   gateAccount, // only invitees the caller manages (handler); team members refused
	"/api/v1/invitations/":                  gateAccount, // resend/void only for invitees the caller manages (handler)
	"/api/v1/panel/port":                    gateAdmin,
	"/api/v1/security/os-updates/mode":      gateAdmin,
	"/api/v1/security/ssh-password-login":   gateAdmin,
	"/api/v1/service/file-access-tools":     gateAdmin,
	"/api/v1/security/os-updates/summary":   gateAdmin,

	// Server, DNS infrastructure, setup and updates: administrator only.
	"/api/v1/dns/engine":                       gateAdmin,
	"/api/v1/dns/engine/reconcile":             gateAdmin,
	"/api/v1/dns/engine/switch":                gateAdmin,
	"/api/v1/dns/engine/switch/preview":        gateAdmin,
	"/api/v1/dns/infrastructure-zones":         gateAdmin,
	"/api/v1/dns/infrastructure-zones/":        gateAdmin,
	"/api/v1/dns/overview":                     gateAdmin,
	"/api/v1/dns/peer-inspector-tool":          gateAdmin, // 6 Nov 2026: the backup's DNS check tool (admin in handler)
	"/api/v1/dns/server-hostname":              gateAdmin, // alpha.121: read-only, admin in handler too
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
	"/api/v1/service/install-plan":             gateAdmin,
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

	// alpha.118: domain ownership, suspension and the wordpress_tools /
	// malware_scan rights are checked in the handlers; the schedule is
	// administrator-only there.
	"/api/v1/wordpress/": gateTenant,
	"/api/v1/malware/":   gateTenant,

	// alpha.127: the live step of a change; only its sender reads it (the
	// request identity row's actor).
	"/api/v1/operation-progress/": gateTenant,

	// alpha.118: remote backup destinations are account routes (additional
	// users never); the handlers check the backups and
	// remote_backup_destinations rights and keep every-site schedules
	// administrator-only.
	"/api/v1/remote-backup/": gateAccount,

	// alpha.128 (G15): the account's own API tokens (team members never;
	// a token never, api_token_auth.go) and the administrator's switch.
	"/api/v1/account/api-tokens":  gateAccount,
	"/api/v1/account/api-tokens/": gateAccount,
	"/api/v1/panel/remote-api":    gateAdmin,

	// alpha.128 ops: G18 notifications. The list and the seen mark read and
	// change only the caller's own rows; the email choice is the caller's
	// own (refused under "Log in as"); the server-wide settings are the
	// administrator's (/api/v1/panel/ prefix, and the handler).
	"/api/v1/notifications":               gateTenant,
	"/api/v1/notifications/":              gateTenant,
	"/api/v1/notification-preferences":    gateTenant,
	"/api/v1/panel/notification-settings": gateAdmin,

	// alpha.128 ops: G4 sessions. The own-session routes read and end only
	// the caller's own sessions (ending is refused under "Log in as"); every
	// account's sessions and the panel access settings are the
	// administrator's (/api/v1/panel/ prefix, and the handlers).
	"/api/v1/auth/activity":            gateTenant,
	"/api/v1/auth/sessions":            gateTenant,
	"/api/v1/auth/sessions/end":        gateTenant,
	"/api/v1/auth/sessions/end-others": gateTenant,
	"/api/v1/panel/sessions":           gateAdmin,
	"/api/v1/panel/sessions/end":       gateAdmin,
	"/api/v1/panel/access":             gateAdmin,
	// A reseller's customers' sessions (handler: reseller only; scope:
	// own customers and their team members).
	"/api/v1/reseller/sessions":     gateAccount,
	"/api/v1/reseller/sessions/end": gateAccount,
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
			if fn, ok := decl.(*ast.FuncDecl); ok && (fn.Name.Name == "registerRecoveryRoutes" || fn.Name.Name == "registerAccountRecoveryRoutes") {
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

// alpha.114 domain sub-routes. They sit under the /api/v1/domains/ row (one
// ownership chokepoint: the administrator, the owning reseller and customer);
// a team member reaches them only through the capability named here.
// alpha.114 alan adı alt rotaları: sahiplik denetimi tek noktada; ekip üyesi
// yalnız burada adı geçen yetkiyle ulaşır.
func TestAlpha114DomainSubroutesFollowTheirTeamCapability(t *testing.T) {
	rows := []struct {
		method, path, kind string
		capability         core.TeamCapability
		mode               core.TeamPermissionMode
	}{
		{http.MethodGet, "/api/v1/domains/7/protected-dirs", "site-protect", core.TeamCapabilityFiles, core.TeamPermissionView},
		{http.MethodPost, "/api/v1/domains/7/protected-dirs", "site-protect", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodDelete, "/api/v1/domains/7/protected-dirs", "site-protect", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodPost, "/api/v1/domains/7/protected-dirs/users", "site-protect-users", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodDelete, "/api/v1/domains/7/protected-dirs/users", "site-protect-users", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodGet, "/api/v1/domains/7/redirects", "site-redirects", core.TeamCapabilityFiles, core.TeamPermissionView},
		{http.MethodPost, "/api/v1/domains/7/redirects", "site-redirects", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodDelete, "/api/v1/domains/7/redirects", "site-redirects", core.TeamCapabilityFiles, core.TeamPermissionManage},
		{http.MethodGet, "/api/v1/domains/7/mail/autoresponder", "mail", core.TeamCapabilityMail, core.TeamPermissionView},
		{http.MethodPut, "/api/v1/domains/7/mail/autoresponder", "mail", core.TeamCapabilityMail, core.TeamPermissionManage},
	}
	for _, row := range rows {
		match, ok := matchDomainSubroute(httptest.NewRequest(row.method, row.path, nil))
		if !ok || match.kind != row.kind || match.domainID != 7 {
			t.Errorf("%s %s: matched %+v, %v; want kind %s", row.method, row.path, match, ok, row.kind)
			continue
		}
		requirement, ok := teamMemberDomainRequirements[teamMemberRouteKey{kind: row.kind, method: row.method}]
		if !ok || requirement.capability != row.capability || requirement.mode != row.mode {
			t.Errorf("%s %s: team requirement %+v, %v; want %s/%s", row.method, row.path, requirement, ok, row.capability, row.mode)
		}
	}
}
