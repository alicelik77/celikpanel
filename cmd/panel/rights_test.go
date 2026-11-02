package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Rights sets (D-042, alpha.117): the intersection rule, the gates of every
// endpoint family, reseller narrowing, the real-use pool, the account
// creation default, plan -> Custom and rebinding with a preview. The fixture
// is the authz matrix one: admin 9701, reseller 9702 (own subscription 9801,
// domain 9901), its customer 9703 (subscription 9802, domain 9902) and the
// administrator's customer 9704 (subscription 9803, domain 9903).

type rightsFixture struct {
	accountsFixture
	rights http.Handler
}

func newRightsFixture(t *testing.T) rightsFixture {
	t.Helper()
	base := newAccountsFixture(t)
	base.panel.rightsInstalledOfferings = func(context.Context) ([]string, error) {
		return []string{"component:php-fpm:8.3", "component:php-fpm:8.5", "component:node:20", "component:node:22",
			"component:mariadb", "component:postgresql", "integration:acme:letsencrypt", "integration:acme:zerossl"}, nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", base.panel.handleUsers)
	mux.HandleFunc("/api/v1/users/", base.panel.handleUserByID)
	mux.HandleFunc("/api/v1/plans", base.panel.handlePlans)
	mux.HandleFunc("/api/v1/plans/", base.panel.handlePlanByID)
	mux.HandleFunc("/api/v1/subscriptions/", base.panel.handleSubscriptionEntitlements)
	mux.HandleFunc(resellerPlansPath, base.panel.handleResellerPlans)
	mux.HandleFunc(resellerPlansPath+"/", base.panel.handleResellerPlanByID)
	mux.HandleFunc(rightsPathPrefix, base.panel.handleRightsRoutes)
	mux.HandleFunc(mySubscriptionPath, base.panel.handleMySubscription)
	mux.HandleFunc(offeredVersionsPath, base.panel.handleOfferedVersions)
	return rightsFixture{accountsFixture: base, rights: base.panel.requireAuth(mux)}
}

func (f rightsFixture) call(t *testing.T, userID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[userID]})
	recorder := httptest.NewRecorder()
	f.rights.ServeHTTP(recorder, request)
	return recorder
}

// setCustom stores a Custom set for a subscription (plan_id cleared).
func (f rightsFixture) setCustom(t *testing.T, subID int, set rightsSet) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.panel.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	limits, err := loadSubscriptionLimits(ctx, tx, subID)
	if err != nil {
		t.Fatal(err)
	}
	if err := setSubscriptionCustom(ctx, tx, subID, set, limits); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func (f rightsFixture) setReseller(t *testing.T, resellerID int, set rightsSet) {
	t.Helper()
	ctx := context.Background()
	if err := saveRightsSet(ctx, f.panel.db.GetDB(), rightsOwnerReseller, resellerID, set); err != nil {
		t.Fatal(err)
	}
}

func everyPermissionBut(except ...string) []string {
	skip := map[string]bool{}
	for _, id := range except {
		skip[id] = true
	}
	out := []string{}
	for _, id := range rightPermissionIDs {
		if !skip[id] {
			out = append(out, permissionItemHead+id)
		}
	}
	return out
}

func requestAs(method, path string, caller *Caller, body string) *http.Request {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	return request.WithContext(context.WithValue(request.Context(), callerKey, caller))
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	return out
}

func TestRightsSetMatchingAndValidation(t *testing.T) {
	s := newRightsSet(false, false, "component:node", "component:php-fpm:8.3", "permission:mail")
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"whole component covers its versions", s.offers("component:node:22"), true},
		{"a version is exact", s.offers("component:php-fpm:8.5"), false},
		{"the named version", s.offers("component:php-fpm:8.3"), true},
		{"any version of php", s.offersAnyVersion("component:php-fpm"), true},
		{"no postgresql", s.offersAnyVersion("component:postgresql"), false},
		{"permission", s.permits(rightMail), true},
		{"missing permission", s.permits(rightDNSEdit), false},
		{"all software", allOfferedRightsSet().offers("extension:acme:tool"), true},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	for _, bad := range []rightsPayload{
		{Permissions: []string{"root"}},
		{Software: []string{"component:../x"}},
		{Software: []string{"Component:PHP"}},
	} {
		if _, err := bad.set(); !errors.Is(err, errUnknownRightsItem) {
			t.Errorf("%+v was accepted", bad)
		}
	}
	// Taking an item out of "everything" lists the part first.
	out, err := applyGrantRevoke(allOfferedRightsSet(), nil, []string{"permission:ssh", "component:php-fpm:8.5"},
		[]string{"component:php-fpm:8.3", "component:php-fpm:8.5"}, false)
	if err != nil || out.AllPermissions || out.AllSoftware || out.permits(rightSSH) || !out.permits(rightMail) ||
		out.offers("component:php-fpm:8.5") || !out.offers("component:php-fpm:8.3") {
		t.Fatalf("revoke from everything = %+v, %v", out, err)
	}
	if _, err := applyGrantRevoke(allOfferedRightsSet(), nil, []string{"component:node:22"}, nil, true); !errors.Is(err, errRightsInstalledUnknown) {
		t.Fatalf("revoking from everything with an unknown installed list = %v", err)
	}
	if grant := newlyGranted(newRightsSet(false, false, "permission:mail"), newRightsSet(true, false, "permission:mail", "component:node:22")); len(grant.Items) != 1 || !grant.Items["component:node:22"] {
		t.Fatalf("newly granted = %+v", grant)
	}
}

func TestEffectiveRightsAreTheIntersectionOfEveryLayer(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	p := f.panel
	// Customer 9703 of reseller 9702: subscription narrowed to PHP 8.3 and
	// 8.5, mail and DNS editing; reseller without PHP 8.5 and without DNS
	// editing; administrator hides Node.js 20 and SSH.
	f.setCustom(t, 9802, newRightsSet(false, false, "component:php-fpm:8.3", "component:php-fpm:8.5", "component:node",
		"permission:mail", "permission:dns_edit", "permission:ssh"))
	f.setReseller(t, 9702, newRightsSet(false, false, append(everyPermissionBut(rightDNSEdit),
		"component:php-fpm:8.3", "component:node")...))
	if err := p.saveOfferedVersions(ctx, offeredVersions{Hidden: map[string][]string{"node": {"20"}}}); err != nil {
		t.Fatal(err)
	}
	if err := p.saveHiddenOfferings(ctx, map[string]bool{"permission:ssh": true}); err != nil {
		t.Fatal(err)
	}
	rights, err := p.effectiveSubscriptionRights(ctx, 9802)
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		item, layer string
	}{
		{"permission:mail", ""},
		{"permission:dns_edit", rightsLayerReseller},
		{"permission:ssh", rightsLayerAdministrator},
		{"permission:cron", rightsLayerSubscription},
		{"permission:vpn", rightsLayerSubscription},
		{"component:php-fpm:8.3", ""},
		{"component:php-fpm:8.5", rightsLayerReseller},
		{"component:node:22", ""},
		{"component:node:20", rightsLayerAdministrator},
		{"component:mariadb", rightsLayerReseller},
	}
	for _, row := range rows {
		var layer string
		if id, ok := strings.CutPrefix(row.item, permissionItemHead); ok {
			layer = rights.permissionRefusal(id)
			has, err := p.subscriptionHasRight(ctx, 9802, id)
			if err != nil || has != (row.layer == "") {
				t.Errorf("subscriptionHasRight(%s) = %v, %v", id, has, err)
			}
		} else {
			layer = rights.offeringRefusal(row.item)
			offered, err := p.subscriptionOffers(ctx, 9802, row.item)
			if err != nil || offered != (row.layer == "") {
				t.Errorf("subscriptionOffers(%s) = %v, %v", row.item, offered, err)
			}
		}
		if layer != row.layer {
			t.Errorf("%s refused by %q, want %q", row.item, layer, row.layer)
		}
	}
	if rights.grantor(rightsLayerSubscription) != "your provider (matrix-reseller)" || rights.grantor(rightsLayerReseller) != "the administrator" {
		t.Fatalf("grantors = %q / %q", rights.grantor(rightsLayerSubscription), rights.grantor(rightsLayerReseller))
	}
	if _, err := p.subscriptionHasRight(ctx, 9802, "root"); !errors.Is(err, errUnknownRightsItem) {
		t.Fatalf("an unknown right = %v", err)
	}
	// The administrator's own workspace is unrestricted.
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, max_domains) VALUES (9810, 9701, 'Admin Subscription', 999)`)
	if has, err := p.subscriptionHasRight(ctx, 9810, rightSSH); err != nil || !has {
		t.Fatalf("admin workspace ssh = %v, %v", has, err)
	}
	// VPN and the app installer still need the add-on grant.
	f.setCustom(t, 9803, allOfferedRightsSet())
	if layer := mustEffective(t, p, 9803).permissionRefusal(rightVPN); layer != rightsLayerAddOn {
		t.Fatalf("vpn without the add-on refused by %q", layer)
	}
	// The view lists every permission and the installed software.
	view, err := p.subscriptionEffectiveRights(ctx, 9802, []string{"component:php-fpm:8.3", "component:php-fpm:8.5"})
	if err != nil || len(view.Permissions) != len(rightPermissionIDs) || len(view.Software) != 2 || view.State != rightsStateCustom ||
		!view.Software[0].Granted || view.Software[1].Granted || view.Software[1].RefusedBy != rightsLayerReseller {
		t.Fatalf("view = %+v, %v", view, err)
	}
}

func mustEffective(t *testing.T, p *Panel, subID int) *effectiveRights {
	t.Helper()
	e, err := p.effectiveSubscriptionRights(context.Background(), subID)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSubscriptionStatesAreReadFromTheRecords(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	// Fixture rows carry numbers and no set: an implicit Custom.
	state, err := subscriptionRightsState(ctx, db, 9802)
	if err != nil || state.State != rightsStateCustom || !state.Implicit || !state.Set.AllSoftware {
		t.Fatalf("fixture state = %+v, %v", state, err)
	}
	f.exec(t, `UPDATE subscriptions SET max_domains = 0, max_databases = 0, max_email_accounts = 0, disk_quota_mb = 0 WHERE id = 9802`)
	if state, _ = subscriptionRightsState(ctx, db, 9802); state.State != rightsStateUnlimited {
		t.Fatalf("all-zero subscription = %+v", state)
	}
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID, `{"name":"Basic","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"rights":{"all_permissions":false,"all_software":false,"permissions":["mail"],"software":["component:php-fpm:8.3"]}}`)
	f.exec(t, `UPDATE subscriptions SET plan_id = ? WHERE id = 9802`, plan)
	if state, _ = subscriptionRightsState(ctx, db, 9802); state.State != rightsStatePlan || state.PlanID != plan || !state.Set.permits(rightMail) || state.Set.permits(rightCron) {
		t.Fatalf("plan state = %+v", state)
	}
	if state.stateLabel() != "plan:"+itoa(plan) {
		t.Fatalf("label = %s", state.stateLabel())
	}
	// Resellers: no pool -> Unlimited reseller; a pool -> implicit Custom.
	if state, _ = resellerRightsState(ctx, db, 9702); state.State != rightsStateUnlimited {
		t.Fatalf("reseller without pool = %+v", state)
	}
	f.exec(t, `INSERT INTO reseller_pools (reseller_id, max_domains) VALUES (9702, 3)`)
	if state, _ = resellerRightsState(ctx, db, 9702); state.State != rightsStateCustom || !state.Implicit {
		t.Fatalf("reseller with pool = %+v", state)
	}
}

// Every endpoint family refuses new use without its permission, keeps reads
// open where the read is not the use, and never refuses the administrator.
func TestDomainSubrouteFamiliesAreGatedByTheirPermission(t *testing.T) {
	f := newRightsFixture(t)
	rows := []struct {
		kind, method, right string
	}{
		{"dns", http.MethodPost, rightDNSEdit},
		{"dns", http.MethodPut, rightDNSEdit},
		{"dns", http.MethodDelete, rightDNSEdit},
		{"dns", http.MethodGet, ""},
		{"dnssec", http.MethodPost, rightDNSSEC},
		{"dnssec", http.MethodGet, ""},
		{"mail", http.MethodPost, rightMail},
		{"mail", http.MethodPut, rightMail},
		{"mail", http.MethodDelete, ""},
		{"mail", http.MethodGet, ""},
		{"databases", http.MethodPost, rightDatabases},
		{"databases", http.MethodGet, ""},
		{"cron", http.MethodPost, rightCron},
		{"cron", http.MethodPut, rightCron},
		{"cron", http.MethodOptions, ""},
		{"files", http.MethodGet, rightFileManager},
		{"files", http.MethodPost, rightFileManager},
		{"files-download", http.MethodGet, rightFileManager},
		{"backups", http.MethodPost, rightBackups},
		{"backups", http.MethodGet, ""},
		{"backup-schedule", http.MethodPut, rightBackups},
		{"backup-restore", http.MethodPost, rightBackups},
		{"backup-download", http.MethodGet, rightBackups},
		{"ssl-letsencrypt", http.MethodPost, rightSSLACME},
		{"ssl-upload", http.MethodPost, rightSSLCustom},
		{"site-protect", http.MethodPost, rightSiteProtect},
		{"site-protect-users", http.MethodPost, rightSiteProtect},
		{"site-redirects", http.MethodPost, rightSiteProtect},
		{"site-redirects", http.MethodGet, ""},
		{"app-install", http.MethodPost, rightAppInstaller},
		{"usage", http.MethodGet, ""},
		{"php", http.MethodPost, ""},
	}
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer}
	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin}
	for _, row := range rows {
		if got := domainSubrouteRightFor(row.kind, row.method); got != row.right {
			t.Errorf("%s %s needs %q, want %q", row.method, row.kind, got, row.right)
			continue
		}
		if row.right == "" {
			continue
		}
		// Without the permission: refused with the code and who can grant it.
		f.setCustom(t, 9802, newRightsSet(false, true, everyPermissionBut(row.right)...))
		recorder := httptest.NewRecorder()
		if f.panel.enforceDomainSubrouteRight(recorder, requestAs(row.method, "/api/v1/domains/9902/x", customer, ""), 9902, row.kind) {
			t.Errorf("%s %s passed without %s", row.method, row.kind, row.right)
			continue
		}
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), `"code":"`+errCodeRightNotGranted+`"`) ||
			!strings.Contains(recorder.Body.String(), "your provider (matrix-reseller)") {
			t.Errorf("%s %s refusal = %d %s", row.method, row.kind, recorder.Code, recorder.Body.String())
		}
		// The administrator is not refused.
		if !f.panel.enforceDomainSubrouteRight(httptest.NewRecorder(), requestAs(row.method, "/api/v1/domains/9902/x", admin, ""), 9902, row.kind) {
			t.Errorf("%s %s refused the administrator", row.method, row.kind)
		}
		// With the permission it passes (the add-on aside).
		if row.right != rightAppInstaller {
			f.setCustom(t, 9802, newRightsSet(false, true, permissionItemHead+row.right))
			if !f.panel.enforceDomainSubrouteRight(httptest.NewRecorder(), requestAs(row.method, "/api/v1/domains/9902/x", customer, ""), 9902, row.kind) {
				t.Errorf("%s %s refused with %s granted", row.method, row.kind, row.right)
			}
		}
	}
	// The add-on still decides the app installer: without a grant the old
	// 402 answer, unchanged.
	f.setCustom(t, 9802, allOfferedRightsSet())
	recorder := httptest.NewRecorder()
	if f.panel.enforceDomainSubrouteRight(recorder, requestAs(http.MethodPost, "/x", customer, ""), 9902, "app-install") ||
		recorder.Code != http.StatusPaymentRequired || !strings.Contains(recorder.Body.String(), errCodeEntitlement) {
		t.Fatalf("app installer without the add-on = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestEndpointGatesOutsideTheDomainDispatcher(t *testing.T) {
	f := newRightsFixture(t)
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer}
	f.setCustom(t, 9802, newRightsSet(false, false, "permission:databases", "component:mariadb", "integration:acme:letsencrypt"))
	cases := []struct {
		name string
		run  func(w http.ResponseWriter) bool
		code string
	}{
		{"proxy site type", func(w http.ResponseWriter) bool {
			return f.panel.requireDomainRight(w, requestAs(http.MethodPut, "/x", customer, ""), 9902, rightSiteProxy)
		}, errCodeRightNotGranted},
		{"team members", func(w http.ResponseWriter) bool {
			return f.panel.requireTeamMembersRight(w, requestAs(http.MethodPost, "/x", customer, ""), 9703, &teamMemberAccessRequest{})
		}, errCodeRightNotGranted},
		{"vpn", func(w http.ResponseWriter) bool {
			return f.panel.requireSubscriptionRight(w, requestAs(http.MethodPost, "/x", customer, ""), 9802, rightVPN)
		}, errCodeRightNotGranted},
		{"postgresql engine", func(w http.ResponseWriter) bool {
			return f.panel.requireSubscriptionOffering(w, requestAs(http.MethodPost, "/x", customer, ""), 9802, databaseEngineOffering("postgresql"), "postgresql", true)
		}, errCodeNotOffered},
		{"zerossl provider", func(w http.ResponseWriter) bool {
			return f.panel.requireSubscriptionOffering(w, requestAs(http.MethodPost, "/x", customer, ""), 9802, "integration:acme:zerossl", "ZeroSSL", false)
		}, errCodeNotOffered},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		if c.run(recorder) || !strings.Contains(recorder.Body.String(), `"code":"`+c.code+`"`) {
			t.Errorf("%s: passed or wrong refusal: %d %s", c.name, recorder.Code, recorder.Body.String())
		}
	}
	for name, ok := range map[string]bool{
		"mariadb engine": f.panel.requireSubscriptionOffering(httptest.NewRecorder(), requestAs(http.MethodPost, "/x", customer, ""), 9802, "component:mariadb", "mariadb", true),
		"letsencrypt":    f.panel.requireSubscriptionOffering(httptest.NewRecorder(), requestAs(http.MethodPost, "/x", customer, ""), 9802, "integration:acme:letsencrypt", "", false),
		"databases":      f.panel.requireSubscriptionRight(httptest.NewRecorder(), requestAs(http.MethodPost, "/x", customer, ""), 9802, rightDatabases),
	} {
		if !ok {
			t.Errorf("%s was refused although granted", name)
		}
	}
	// Choosers list only what is open: ACME providers for the customer's
	// only subscription.
	recorder := httptest.NewRecorder()
	f.panel.handleACMEProviders(recorder, requestAs(http.MethodGet, "/api/v1/ssl/providers", customer, ""))
	body := recorder.Body.String()
	if !strings.Contains(body, `"letsencrypt"`) || strings.Contains(body, `"zerossl"`) {
		t.Fatalf("providers listed = %s", body)
	}
}

func TestVersionChoicesFollowTheSubscriptionsRights(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer}
	f.setCustom(t, 9802, newRightsSet(false, false, "component:php-fpm:8.3", "component:node:22"))
	refuse := func(component, current, requested string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		f.panel.refuseVersionNotOffered(recorder, requestAs(http.MethodPost, "/x", customer, ""), 9902, component, current, requested)
		return recorder
	}
	if r := refuse("php", "8.3", "8.5"); r.Code != http.StatusConflict || !strings.Contains(r.Body.String(), `"code":"`+errCodeNotOffered+`"`) {
		t.Fatalf("PHP 8.5 outside the plan = %d %s", r.Code, r.Body.String())
	}
	if r := refuse("php", "8.5", "8.5"); r.Body.Len() != 0 {
		t.Fatalf("a site keeping its version was refused: %s", r.Body.String())
	}
	if r := refuse("node", "22.1.0", "22.4.0"); r.Body.Len() != 0 {
		t.Fatalf("another patch of the running line was refused: %s", r.Body.String())
	}
	if r := refuse("node", "", "22.4.0"); r.Body.Len() != 0 {
		t.Fatalf("an offered line was refused: %s", r.Body.String())
	}
	// The administrator hiding a version answers with its own code.
	if err := f.panel.saveOfferedVersions(ctx, offeredVersions{Hidden: map[string][]string{"php": {"8.3"}}}); err != nil {
		t.Fatal(err)
	}
	if r := refuse("php", "", "8.3"); !strings.Contains(r.Body.String(), `"code":"`+errCodeVersionNotOffered+`"`) {
		t.Fatalf("a hidden version = %s", r.Body.String())
	}
	policy, rights, err := f.panel.subscriptionVersionPolicy(ctx, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.filter("php", []string{"8.5", "8.3"}); len(got) != 0 {
		t.Fatalf("listed PHP = %v", got)
	}
	if got := projectTypesFor(rights, policy, []string{"8.5", "8.3"}); strings.Join(got, ",") != "static,dnsonly,forwarding,node" {
		t.Fatalf("site types = %v", got)
	}
	if version, ok := customerSitePHPVersion("", []string{"8.5", "8.3"}, policy); ok || version != "" {
		t.Fatalf("new site PHP = %q %v", version, ok)
	}
	// The administrator is never narrowed.
	recorder := httptest.NewRecorder()
	if f.panel.refuseVersionNotOffered(recorder, requestAs(http.MethodPost, "/x", &Caller{ID: authzMatrixAdminID, Role: roleAdmin}, ""), 9902, "php", "", "8.5") {
		t.Fatal("the administrator was refused a version")
	}
}

func TestResellerNarrowingReachesItsCustomersButNotRunningThings(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	f.exec(t, `UPDATE subscriptions SET max_domains = 0, max_databases = 0, max_email_accounts = 0, disk_quota_mb = 0 WHERE id = 9802`)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type, runtime_version) VALUES (9902, '/srv/n', 'node', '22.3.0')`)
	// Unlimited customer under an Unlimited reseller: Node.js 22 is open.
	if ok, _ := f.panel.subscriptionOffers(ctx, 9802, "component:node:22"); !ok {
		t.Fatal("Node.js 22 closed for an Unlimited customer")
	}
	// The administrator takes Node.js from the reseller (with a preview).
	body := `{"revoke":["component:node"]}`
	preview := f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/users/9702/rights/preview", body)
	wantStatus(t, preview, http.StatusOK, "")
	if !strings.Contains(preview.Body.String(), `"component:node:22"`) || !strings.Contains(preview.Body.String(), `"what":"sites"`) {
		t.Fatalf("preview does not name the site that keeps running: %s", preview.Body.String())
	}
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/users/9702/rights", body), http.StatusConflict, errCodeRightsConfirm)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/users/9702/rights", `{"revoke":["component:node"],"confirm":true}`), http.StatusOK, "")
	if ok, _ := f.panel.subscriptionOffers(ctx, 9802, "component:node:22"); ok {
		t.Fatal("the reseller's narrowing did not reach its customer")
	}
	// The site stays; nothing that runs was touched.
	if n := f.count(t, `SELECT COUNT(*) FROM sites WHERE domain_id = 9902 AND runtime_version = '22.3.0'`); n != 1 {
		t.Fatal("a running site was changed")
	}
	// The reseller cannot give its customer what it no longer has.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/subscriptions/9802/rights",
		`{"state":"custom","rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["component:node:22"]},"confirm":true}`),
		http.StatusConflict, errCodeRightsOutside)
	// Inside its field the reseller narrows its customer freely.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/subscriptions/9802/rights",
		`{"state":"custom","rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["component:php-fpm:8.3"]},"confirm":true}`),
		http.StatusOK, "")
	// ...nor a plan with it.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Dev","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["component:node:22"]}}`),
		http.StatusConflict, errCodeRightsOutside)
	// A reseller does not change its own subscription's rights, nor another
	// tree's.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/subscriptions/9801/rights", `{"state":"unlimited","confirm":true}`), http.StatusForbidden, errCodeAdminOnly)
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodGet, "/api/v1/subscriptions/9803/rights", ""), http.StatusNotFound, "")
	// The customer reads its own rights but does not change them.
	read := f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/subscriptions/9802/rights", "")
	wantStatus(t, read, http.StatusOK, "")
	if !strings.Contains(read.Body.String(), `"state":"custom"`) || !strings.Contains(read.Body.String(), `"refused_by":"reseller"`) {
		t.Fatalf("customer view = %s", read.Body.String())
	}
	wantStatus(t, f.call(t, authzMatrixCustomerID, http.MethodPut, "/api/v1/subscriptions/9802/rights", `{"state":"unlimited","confirm":true}`), http.StatusForbidden, "")
}

func TestPlanBoundChangeBecomesCustomAndRebindShowsTheDifference(t *testing.T) {
	f := newRightsFixture(t)
	db := f.panel.db.GetDB()
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID, `{"name":"Basic","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	// One change on a plan-bound subscription asks first...
	change := `{"revoke":["permission:dns_edit"]}`
	refusal := f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/subscriptions/9802/rights", change)
	wantStatus(t, refusal, http.StatusConflict, errCodeRightsLeavesPlan)
	if !strings.Contains(refusal.Body.String(), `leaves the \"Basic\" plan and becomes Custom`) {
		t.Fatalf("the question does not name the plan: %s", refusal.Body.String())
	}
	var planID sql.NullInt64
	if err := db.QueryRow(`SELECT plan_id FROM subscriptions WHERE id = 9802`).Scan(&planID); err != nil || !planID.Valid {
		t.Fatal("an unconfirmed change left the plan")
	}
	// ...then copies the plan's set and applies the change.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/subscriptions/9802/rights", `{"revoke":["permission:dns_edit"],"confirm":true}`), http.StatusOK, "")
	state, err := subscriptionRightsState(context.Background(), db, 9802)
	if err != nil || state.State != rightsStateCustom || state.Set.permits(rightDNSEdit) || !state.Set.permits(rightMail) || !state.Set.AllSoftware {
		t.Fatalf("after leaving the plan = %+v, %v", state, err)
	}
	if limits, _ := loadSubscriptionLimits(context.Background(), db, 9802); limits.Domains != 4 {
		t.Fatalf("the plan's numbers were not kept: %+v", limits)
	}
	// Later plan edits do not reach it.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Basic","max_domains":9,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0}`), http.StatusOK, "")
	if limits, _ := loadSubscriptionLimits(context.Background(), db, 9802); limits.Domains != 4 {
		t.Fatal("a plan edit reached a Custom subscription")
	}
	// Binding back: preview first (what returns, numbers), then confirm.
	rebind := `{"state":"plan","plan_id":` + itoa(plan) + `}`
	preview := f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/subscriptions/9802/rights/preview", rebind)
	wantStatus(t, preview, http.StatusOK, "")
	got := decodeBody(t, preview)["preview"].(map[string]any)
	if got["to"] != "plan:"+itoa(plan) || got["from"] != rightsStateCustom || !strings.Contains(preview.Body.String(), `"added":["permission:dns_edit"]`) {
		t.Fatalf("rebind preview = %s", preview.Body.String())
	}
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9802/rights", rebind), http.StatusConflict, errCodeRightsConfirm)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9802/rights", `{"state":"plan","plan_id":`+itoa(plan)+`,"confirm":true}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM rights_sets WHERE owner_kind = 'subscription' AND owner_id = 9802`); n != 0 {
		t.Fatal("the Custom set outlived the rebinding")
	}
	if limits, _ := loadSubscriptionLimits(context.Background(), db, 9802); limits.Domains != 9 {
		t.Fatalf("rebinding did not copy the plan's numbers: %+v", limits)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'subscription.rights:%' AND resource_id = 9802`); n != 2 {
		t.Fatalf("rights audit rows = %d", n)
	}
	// Unlimited: every number 0, everything offered.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9802/rights", `{"state":"unlimited","confirm":true}`), http.StatusOK, "")
	if state, _ := subscriptionRightsState(context.Background(), db, 9802); state.State != rightsStateUnlimited {
		t.Fatalf("unlimited state = %+v", state)
	}
}

func TestPlanEditThatNarrowsShowsWhatIsInUseFirst(t *testing.T) {
	f := newRightsFixture(t)
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID, `{"name":"Dev","max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0}`)
	f.exec(t, `UPDATE subscriptions SET plan_id = ? WHERE id = 9802`, plan)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type, php_version) VALUES (9902, '/srv/p', 'php', '8.5')`)
	narrow := `{"name":"Dev","max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"bandwidth_quota_mb":0,"rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["component:php-fpm:8.3"]}}`
	refusal := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), narrow)
	wantStatus(t, refusal, http.StatusConflict, errCodePlanNarrowingConfirm)
	body := refusal.Body.String()
	if !strings.Contains(body, `"item":"component:php-fpm:8.5","count":1,"what":"sites"`) || !strings.Contains(body, `"subscriptions":1`) {
		t.Fatalf("narrowing preview = %s", body)
	}
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans/"+itoa(plan)+"/preview", narrow), http.StatusOK, "")
	confirmed := strings.Replace(narrow, `"bandwidth_quota_mb":0,`, `"bandwidth_quota_mb":0,"confirm_narrowing":true,`, 1)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), confirmed), http.StatusOK, "")
	if ok, _ := f.panel.subscriptionOffers(context.Background(), 9802, "component:php-fpm:8.5"); ok {
		t.Fatal("the plan edit did not reach its subscription")
	}
	// Built-in names are reserved; the listing names the built-in plan.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans", `{"name":"unlimited","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusConflict, errCodePlanNameReserved)
	list := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/plans", "")
	if !strings.Contains(list.Body.String(), `"builtin":[{"key":"unlimited","name":"Unlimited","kind":"customer"}]`) ||
		!strings.Contains(list.Body.String(), `"software":["component:php-fpm:8.3"]`) {
		t.Fatalf("plan list = %s", list.Body.String())
	}
}

func TestResellerPoolLimitsRealUseNotAllocation(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	// The tree: 9801 (domain 9901) and 9802 (domain 9902) — two real domains,
	// while 5 + 5 are handed out by the fixture's defaults.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":5,"max_domains":3,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	// Handing out more than the pool is allowed (K2).
	big := createPlan(t, f.accountsFixture, authzMatrixAdminID, `{"name":"Big","max_domains":20,"max_databases":5,"max_email_accounts":5,"disk_quota_mb":500,"bandwidth_quota_mb":0}`)
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(big)+`}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Wide","max_domains":0,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`), http.StatusOK, "")
	// Real use 2 of 3: one more fits and leaves an 80%+ notice.
	if err := f.panel.checkSubscriptionQuota(ctx, 9802, quotaDomains); err != nil {
		t.Fatalf("a domain that fits the pool was refused: %v", err)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM rights_notices WHERE user_id = 9702 AND kind = 'reseller_pool_100' AND subject = 'domains'`); n != 1 {
		t.Fatalf("100%% notice rows = %d", n)
	}
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9904, 9802, 'third.matrix.test', 'active')`)
	err := f.panel.checkSubscriptionQuota(ctx, 9801, quotaDomains)
	var exceeded *resellerPoolExceededError
	if !errors.As(err, &exceeded) || exceeded.Used != 3 || exceeded.Limit != 3 {
		t.Fatalf("a full pool did not refuse: %v", err)
	}
	recorder := httptest.NewRecorder()
	writeQuotaRefusal(recorder, err)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeResellerPoolExceeded) {
		t.Fatalf("refusal = %d %s", recorder.Code, recorder.Body.String())
	}
	// The administrator's customer is not in the reseller's tree.
	if err := f.panel.checkSubscriptionQuota(ctx, 9803, quotaDomains); err != nil {
		t.Fatalf("an unrelated subscription was refused: %v", err)
	}
	// The pool answer shows real use and what is over.
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9905, 9802, 'fourth.matrix.test', 'active')`)
	answer := f.as(t, authzMatrixAdminID, http.MethodGet, "/api/v1/users/9702/pool", "")
	if !strings.Contains(answer.Body.String(), `"used":{"customers":1,"domains":4`) || !strings.Contains(answer.Body.String(), `"over_limit":["domains: 4 in use, pool 3"]`) {
		t.Fatalf("pool answer = %s", answer.Body.String())
	}
	// Customer count stays a real count.
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":1,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	wantStatus(t, f.as(t, authzMatrixResellerID, http.MethodPost, "/api/v1/users",
		`{"username":"one-more","email":"one-more@example.test","password":"long-enough-1","role":"customer"}`), http.StatusConflict, errCodeResellerCustomersLimit)
	// Notices: the administrator sees all, the reseller its own, and either
	// may acknowledge.
	notices := f.call(t, authzMatrixResellerID, http.MethodGet, "/api/v1/rights/notices", "")
	wantStatus(t, notices, http.StatusOK, "")
	list := decodeBody(t, notices)["notices"].([]any)
	if len(list) == 0 {
		t.Fatal("the reseller sees no pool notice")
	}
	id := int(list[0].(map[string]any)["id"].(float64))
	wantStatus(t, f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/rights/notices", ""), http.StatusForbidden, "")
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/rights/notices/"+itoa(id)+"/acknowledge", ""), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/rights/notices/"+itoa(id)+"/acknowledge", ""), http.StatusNotFound, "")
}

func TestAccountCreationRightsChoice(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	create := func(caller int, body string) *httptest.ResponseRecorder {
		return f.call(t, caller, http.MethodPost, "/api/v1/users", body)
	}
	// Default: Unlimited, named in the answer, with one all-unlimited subscription.
	recorder := create(authzMatrixResellerID, `{"username":"plain","email":"plain@example.test","password":"long-enough-1","role":"customer"}`)
	wantStatus(t, recorder, http.StatusOK, "")
	if !strings.Contains(recorder.Body.String(), `"rights":"unlimited"`) {
		t.Fatalf("answer = %s", recorder.Body.String())
	}
	var subID int
	if err := db.QueryRow(`SELECT s.id FROM subscriptions s JOIN users u ON u.id = s.owner_id WHERE u.username = 'plain'`).Scan(&subID); err != nil {
		t.Fatal(err)
	}
	if state, _ := subscriptionRightsState(ctx, db, subID); state.State != rightsStateUnlimited {
		t.Fatalf("default state = %+v", state)
	}
	// The reseller's "Unlimited" customer gets the reseller's field, no more.
	f.setReseller(t, 9702, newRightsSet(false, false, "permission:mail", "component:php-fpm:8.3"))
	if ok, _ := f.panel.subscriptionHasRight(ctx, int64(subID), rightCron); ok {
		t.Fatal("an Unlimited customer of a narrowed reseller got cron")
	}
	// Custom inside the reseller's set; outside is refused before anything exists.
	wantStatus(t, create(authzMatrixResellerID, `{"username":"too-much","email":"too-much@example.test","password":"long-enough-1","role":"customer",
		"rights":{"state":"custom","rights":{"all_permissions":false,"all_software":false,"permissions":["cron"],"software":[]}}}`), http.StatusConflict, errCodeRightsOutside)
	if n := f.count(t, `SELECT COUNT(*) FROM users WHERE username = 'too-much'`); n != 0 {
		t.Fatal("a refused account was created")
	}
	wantStatus(t, create(authzMatrixResellerID, `{"username":"narrow","email":"narrow@example.test","password":"long-enough-1","role":"customer",
		"rights":{"state":"custom","rights":{"all_permissions":false,"all_software":false,"permissions":["mail"],"software":["component:php-fpm:8.3"]},"quantities":{"max_domains":2}}}`), http.StatusOK, "")
	var narrowSub, domains int
	if err := db.QueryRow(`SELECT s.id, s.max_domains FROM subscriptions s JOIN users u ON u.id = s.owner_id WHERE u.username = 'narrow'`).Scan(&narrowSub, &domains); err != nil || domains != 2 {
		t.Fatalf("custom subscription = %d %d %v", narrowSub, domains, err)
	}
	if state, _ := subscriptionRightsState(ctx, db, narrowSub); state.State != rightsStateCustom || !state.Set.permits(rightMail) || state.Set.permits(rightCron) {
		t.Fatalf("custom state = %+v", state)
	}
	// A plan by the rights choice (and the old plan_id still works).
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID, `{"name":"Basic","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0}`)
	wantStatus(t, create(authzMatrixAdminID, `{"username":"planned","email":"planned@example.test","password":"long-enough-1","role":"customer","rights":{"state":"plan","plan_id":`+itoa(plan)+`}}`), http.StatusOK, "")
	wantStatus(t, create(authzMatrixAdminID, `{"username":"oldway","email":"oldway@example.test","password":"long-enough-1","role":"customer","plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM subscriptions WHERE plan_id = ?`, plan); n != 2 {
		t.Fatalf("plan subscriptions = %d", n)
	}
	wantStatus(t, create(authzMatrixAdminID, `{"username":"mixed","email":"mixed@example.test","password":"long-enough-1","role":"customer","plan_id":`+itoa(plan)+`,"rights":{"state":"unlimited"}}`), http.StatusBadRequest, "")
	// Resellers: a reseller plan binds and mirrors its pool; Custom stores both.
	created := f.call(t, authzMatrixAdminID, http.MethodPost, resellerPlansPath,
		`{"name":"Gold","max_customers":3,"max_domains":30,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":false,"permissions":[],"software":["component:php-fpm:8.3"]}}`)
	wantStatus(t, created, http.StatusOK, "")
	gold := int(decodeBody(t, created)["id"].(float64))
	wantStatus(t, create(authzMatrixAdminID, `{"username":"gold-r","email":"gold-r@example.test","password":"long-enough-1","role":"reseller","rights":{"state":"plan","plan_id":`+itoa(gold)+`}}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM reseller_plan_bindings b JOIN users u ON u.id = b.reseller_id JOIN reseller_pools p ON p.reseller_id = u.id
		WHERE u.username = 'gold-r' AND b.reseller_plan_id = ? AND p.max_customers = 3 AND p.max_domains = 30`, gold); n != 1 {
		t.Fatal("the reseller plan was not bound with its pool")
	}
	wantStatus(t, create(authzMatrixAdminID, `{"username":"free-r","email":"free-r@example.test","password":"long-enough-1","role":"reseller"}`), http.StatusOK, "")
	var freeID int
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'free-r'`).Scan(&freeID); err != nil {
		t.Fatal(err)
	}
	if state, _ := resellerRightsState(ctx, db, freeID); state.State != rightsStateUnlimited {
		t.Fatalf("a reseller created without a choice = %+v", state)
	}
}

func TestResellerPlansAndBuiltInPlansAreFixed(t *testing.T) {
	f := newRightsFixture(t)
	created := f.call(t, authzMatrixAdminID, http.MethodPost, resellerPlansPath,
		`{"name":"Silver","max_customers":2,"max_domains":10,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}`)
	wantStatus(t, created, http.StatusOK, "")
	silver := itoa(int(decodeBody(t, created)["id"].(float64)))
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodGet, resellerPlansPath, ""), http.StatusForbidden, errCodeAdminOnly)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, resellerPlansPath, `{"name":"Unlimited reseller","max_customers":0,"max_domains":0,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}`), http.StatusConflict, errCodePlanNameReserved)
	// Bind reseller 9702 (preview, confirm), then edit the plan: the pool follows.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/rights", `{"state":"plan","plan_id":`+silver+`}`), http.StatusConflict, errCodeRightsConfirm)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/rights", `{"state":"plan","plan_id":`+silver+`,"confirm":true}`), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, resellerPlansPath+"/"+silver,
		`{"name":"Silver","max_customers":2,"max_domains":12,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}`), http.StatusOK, "")
	if n := f.count(t, `SELECT max_domains FROM reseller_pools WHERE reseller_id = 9702`); n != 12 {
		t.Fatalf("bound pool = %d", n)
	}
	// Narrowing a reseller plan asks first.
	narrow := `{"name":"Silver","max_customers":2,"max_domains":12,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":false,"all_software":true,"permissions":["mail"],"software":[]}}`
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, resellerPlansPath+"/"+silver, narrow), http.StatusConflict, errCodePlanNarrowingConfirm)
	// A bound plan cannot be deleted.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodDelete, resellerPlansPath+"/"+silver, ""), http.StatusConflict, errCodeResellerPlanInUse)
	// The pool endpoint on a bound reseller makes it Custom (copy, then change).
	wantStatus(t, f.as(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/pool",
		`{"pool":{"max_customers":2,"max_domains":5,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0}}`), http.StatusOK, "")
	state, err := resellerRightsState(context.Background(), f.panel.db.GetDB(), 9702)
	if err != nil || state.State != rightsStateCustom || state.Implicit {
		t.Fatalf("after a pool change = %+v, %v", state, err)
	}
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodDelete, resellerPlansPath+"/"+silver, ""), http.StatusOK, "")
	list := f.call(t, authzMatrixAdminID, http.MethodGet, resellerPlansPath, "")
	if !strings.Contains(list.Body.String(), `"builtin":[{"key":"unlimited","name":"Unlimited reseller","kind":"reseller"}]`) {
		t.Fatalf("reseller plan list = %s", list.Body.String())
	}
	// The administrator's workspace has no rights to change.
	f.exec(t, `INSERT INTO subscriptions (id, owner_id, name, max_domains) VALUES (9810, 9701, 'Admin Subscription', 999)`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9810/rights", `{"state":"unlimited","confirm":true}`), http.StatusConflict, errCodeRightsAdminWorkspace)
}

func TestRightsCatalogAndAdministratorOfferLayer(t *testing.T) {
	f := newRightsFixture(t)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, offeredVersionsPath, `{"offering_id":"integration:acme:zerossl","offered":false}`), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, offeredVersionsPath, `{"offering_id":"component:php-fpm:8.5","offered":false}`), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, offeredVersionsPath, `{"offering_id":"bad id","offered":false}`), http.StatusBadRequest, "")
	wantStatus(t, f.call(t, authzMatrixCustomerID, http.MethodPut, offeredVersionsPath, `{"offering_id":"component:roundcube","offered":false}`), http.StatusForbidden, "")
	read := f.call(t, authzMatrixCustomerID, http.MethodGet, offeredVersionsPath, "")
	if !strings.Contains(read.Body.String(), `"hidden_offerings":["integration:acme:zerossl"]`) || !strings.Contains(read.Body.String(), `"php":["8.5"]`) {
		t.Fatalf("offered layer = %s", read.Body.String())
	}
	catalog := f.call(t, authzMatrixCustomerID, http.MethodGet, "/api/v1/rights/catalog", "")
	wantStatus(t, catalog, http.StatusOK, "")
	body := catalog.Body.String()
	if !strings.Contains(body, `"scope":"subscription"`) || !strings.Contains(body, `"id":"integration:acme:zerossl","granted":false,"refused_by":"administrator"`) ||
		!strings.Contains(body, `"id":"component:php-fpm:8.3","granted":true`) {
		t.Fatalf("customer catalog = %s", body)
	}
	if reseller := f.call(t, authzMatrixResellerID, http.MethodGet, "/api/v1/rights/catalog", ""); !strings.Contains(reseller.Body.String(), `"scope":"reseller"`) {
		t.Fatalf("reseller catalog = %s", reseller.Body.String())
	}
	if admin := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/catalog", ""); !strings.Contains(admin.Body.String(), `"scope":"administrator"`) {
		t.Fatalf("admin catalog = %s", admin.Body.String())
	}
	// "My plan" names the state.
	mine := f.call(t, authzMatrixCustomerID, http.MethodGet, mySubscriptionPath, "")
	if !strings.Contains(mine.Body.String(), `"rights":"custom"`) {
		t.Fatalf("my subscription = %s", mine.Body.String())
	}
}

func TestSubscriptionLifecycleReading(t *testing.T) {
	ns := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	l := lifecycleFromRow(ns("active"), ns(""), ns("2026-01-02 03:04:05"), ns("2026-06-01T00:00:00Z"), ns(""), ns(""))
	if l.StartsAt == nil || l.StartsAt.Year() != 2026 || l.EndsAt == nil || l.SuspendReason != "" {
		t.Fatalf("lifecycle = %+v", l)
	}
	if l.state(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) != lifecycleActive || l.state(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) != lifecycleEnded {
		t.Fatal("an end date is not read")
	}
	never := lifecycleFromRow(ns("active"), ns("2026-01-01T00:00:00Z"), ns(""), ns(""), ns(""), ns(""))
	if never.EndsAt != nil || never.state(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)) != lifecycleActive {
		t.Fatal("an empty end date ended")
	}
	suspended := lifecycleFromRow(ns("suspended"), ns(""), ns(""), ns(""), ns("weird"), ns("2026-02-02T00:00:00Z"))
	if suspended.state(time.Now()) != lifecycleSuspended || suspended.SuspendReason != suspendReasonManual || suspended.SuspendedAt == nil {
		t.Fatalf("suspended = %+v", suspended)
	}
	if expired := lifecycleFromRow(ns("expired"), ns(""), ns(""), ns(""), ns(""), ns("")); expired.SuspendReason != suspendReasonExpired || expired.state(time.Now()) != lifecycleEnded {
		t.Fatalf("expired = %+v", expired)
	}
	if bad := lifecycleFromRow(ns("active"), ns("not a time"), ns(""), ns(""), ns(""), ns("")); bad.StartsAt != nil {
		t.Fatal("an unreadable time became a time")
	}
	f := newRightsFixture(t)
	f.exec(t, `UPDATE subscriptions SET status = 'suspended', suspend_reason = 'payment', suspended_at = '2026-03-03T00:00:00Z', ends_at = '2026-04-04T00:00:00Z' WHERE id = 9802`)
	read, err := readSubscriptionLifecycle(context.Background(), f.panel.db.GetDB(), 9802)
	if err != nil || read.SuspendReason != suspendReasonPayment || read.EndsAt == nil || read.StartsAt == nil {
		t.Fatalf("stored lifecycle = %+v, %v", read, err)
	}
	if _, err := readSubscriptionLifecycle(context.Background(), f.panel.db.GetDB(), 1); !errors.Is(err, errLifecycleSubscriptionMissing) {
		t.Fatalf("missing subscription = %v", err)
	}
}
