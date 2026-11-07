package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// alpha.128 (docs/SITE-LIMITS.md): limits of site settings follow the rights
// chain, plans hold ceilings within their grantor's field, the overuse policy
// and the overselling choice.

func TestSettingLimitsFollowTheChainAndNameWhoSetThem(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	server, err := subscriptionSettingLimits(ctx, db, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if got := server[settingPHPMemoryMB]; got.Ceiling == nil || *got.Ceiling != 512 || got.SetBy != settingSetByServer {
		t.Fatalf("server level: %+v", got)
	}
	// A plan above the server's limit is refused, naming the server.
	refused := f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Big","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":1024}}`)
	wantStatus(t, refused, http.StatusConflict, errCodeSettingAboveLimit)
	if !strings.Contains(refused.Body.String(), `"set_by":"server"`) {
		t.Fatalf("refusal does not name the server: %s", refused.Body.String())
	}
	// An unknown key is refused, not stored.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Odd","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"php.allow_url_include":1}}`),
		http.StatusBadRequest, errCodeSettingUnknown)
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Small","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":256,"php.max_children":4},"overuse_policy":"notify","notify_percent_disk":90}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	limits, err := subscriptionSettingLimits(ctx, db, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if got := limits[settingPHPMemoryMB]; got.Ceiling == nil || *got.Ceiling != 256 || got.SetBy != settingSetByPlan || got.SetByName != "Small" {
		t.Fatalf("plan level: %+v", got)
	}
	if got := limits[settingPHPTimeS]; got.Ceiling == nil || *got.Ceiling != 300 || got.SetBy != settingSetByServer {
		t.Fatalf("a key the plan leaves out stays at the server's: %+v", got)
	}
	php := phpLimitsFor(limits)
	if !php.Known || php.MemoryMB != 256 || php.InheritMemoryMB != 256 || php.InheritTimeS != 0 || php.Processes != 4 {
		t.Fatalf("pool limits %+v", php)
	}
	policy, disk, traffic, err := subscriptionOverusePolicy(ctx, db, 9802)
	if err != nil || policy != overusePolicyNotify || disk != 90 || traffic != 80 {
		t.Fatalf("policy %q %d %d %v", policy, disk, traffic, err)
	}
	// Leaving the plan keeps its ceilings and policy as the subscription's own.
	leave := f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/subscriptions/9802/rights", `{"revoke":["permission:dns_edit"],"confirm":true}`)
	wantStatus(t, leave, http.StatusOK, "")
	limits, err = subscriptionSettingLimits(ctx, db, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if got := limits[settingPHPMemoryMB]; got.Ceiling == nil || *got.Ceiling != 256 || got.SetBy != settingSetBySubscription {
		t.Fatalf("custom level after leaving the plan: %+v", got)
	}
	if policy, _, _, _ := subscriptionOverusePolicy(ctx, db, 9802); policy != overusePolicyNotify {
		t.Fatalf("policy after leaving the plan: %q", policy)
	}
}

func TestPlanEditPreviewsLoweredCeilings(t *testing.T) {
	f := newRightsFixture(t)
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lower","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":512}}`)
	preview := f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans/"+itoa(plan)+"/preview",
		`{"name":"Lower","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":128}}`)
	wantStatus(t, preview, http.StatusOK, "")
	if !strings.Contains(preview.Body.String(), `"ceilings_lowered":["php.memory_limit_mb"]`) {
		t.Fatalf("preview = %s", preview.Body.String())
	}
	// A raised ceiling lowers nothing.
	if lowered := loweredCeilings(map[string]int{settingPHPMemoryMB: 128}, map[string]int{settingPHPMemoryMB: 256}); len(lowered) != 0 {
		t.Fatalf("raised ceiling counted as lowered: %v", lowered)
	}
	if lowered := loweredCeilings(map[string]int{settingCronMinIntervalMin: 5}, map[string]int{settingCronMinIntervalMin: 15}); len(lowered) != 1 {
		t.Fatalf("a larger minimum interval is stricter: %v", lowered)
	}
}

func TestServerSettingLimitsAreTheAdministratorsOnly(t *testing.T) {
	f := newRightsFixture(t)
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"php.max_children","server_ceiling":50}`), http.StatusForbidden, errCodeAdminOnly)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"php.max_children","server_ceiling":500}`), http.StatusBadRequest, errCodeSettingUnknown)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"php.max_children","server_ceiling":40,"customer_editable":true}`), http.StatusOK, "")
	read := f.call(t, authzMatrixResellerID, http.MethodGet, "/api/v1/rights/setting-limits", "")
	wantStatus(t, read, http.StatusOK, "")
	if !strings.Contains(read.Body.String(), `"scope":"reseller"`) || !strings.Contains(read.Body.String(), `"key":"php.max_children"`) {
		t.Fatalf("reseller field = %s", read.Body.String())
	}
}

func TestNginxDirectivesIsClosedUntilOffered(t *testing.T) {
	f := newRightsFixture(t)
	body := `{"name":"Dir","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"rights":{"all_permissions":false,"all_software":true,"permissions":["nginx_directives"],"software":[]}}`
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans", body), http.StatusConflict, errCodeRightsOutside)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/offered-permissions", `{"permission":"nginx_directives","offered":true}`), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans", body), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/rights/offered-permissions", `{"permission":"nginx_directives","offered":true}`), http.StatusForbidden, errCodeAdminOnly)
}

func TestNotifyLevelAndPolicy(t *testing.T) {
	if usageLevelAt(85, 100, 90) != 0 || usageLevelAt(90, 100, 90) != 80 || usageLevelAt(100, 100, 90) != 100 {
		t.Fatal("notice level 90 % not applied")
	}
	if usageLevelAt(80, 100, 0) != 80 {
		t.Fatal("an unset level is 80 %")
	}
}

func TestOversellingNotAllowedRefusesBeyondThePool(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	f.exec(t, `INSERT INTO reseller_pools (reseller_id, max_domains, overselling) VALUES (?, 3, 'not_allowed')
		ON CONFLICT(reseller_id) DO UPDATE SET max_domains = 3, overselling = 'not_allowed'`, authzMatrixResellerID)
	over, err := oversellingRefusal(ctx, db, authzMatrixResellerID, 0, planLimits{Domains: 5})
	if err != nil || len(over) == 0 || !strings.Contains(over[0], "domains") {
		t.Fatalf("over = %v %v", over, err)
	}
	if over, _ := oversellingRefusal(ctx, db, authzMatrixResellerID, 0, planLimits{}); len(over) == 0 {
		t.Fatal("an unlimited allocation was taken under a limited pool")
	}
	f.exec(t, `UPDATE reseller_pools SET overselling = 'allowed' WHERE reseller_id = ?`, authzMatrixResellerID)
	if over, _ := oversellingRefusal(ctx, db, authzMatrixResellerID, 0, planLimits{Domains: 50}); len(over) != 0 {
		t.Fatalf("K2 (allowed) refused: %v", over)
	}
}

func TestCronMinIntervalReadsTheSchedule(t *testing.T) {
	for schedule, want := range map[string]int{
		"* * * * *": 1, "*/5 * * * *": 5, "0 * * * *": 60, "0,30 * * * *": 30, "15 3 * * *": 1440,
		"@hourly": 60, "@daily": 1440, "0 */2 * * *": 120, "5-10 * * * *": 1, "0 9-17 * * 1-5": 60,
	} {
		got, ok := cronMinIntervalMinutes(schedule)
		if !ok || got != want {
			t.Errorf("%q = %d %v, want %d", schedule, got, ok, want)
		}
	}
	if _, ok := cronMinIntervalMinutes("not a schedule"); ok {
		t.Error("an unreadable schedule was read")
	}
}

func TestCustomResellerKeepsItsOwnLimitsAndOverselling(t *testing.T) {
	f := newRightsFixture(t)
	body := `{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},` +
		`"quantities":{"max_domains":10},"ceilings":{"php.memory_limit_mb":384},"overselling":"not_allowed","confirm":true}`
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/rights", body), http.StatusOK, "")
	read := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/setting-limits?reseller_id=9702", "")
	wantStatus(t, read, http.StatusOK, "")
	if !strings.Contains(read.Body.String(), `"overselling":"not_allowed"`) || !strings.Contains(read.Body.String(), `"ceiling":384,"set_by":"reseller"`) {
		t.Fatalf("reseller field = %s", read.Body.String())
	}
	// A reseller's own plan may not go above the reseller's own limit.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"R","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":512}}`),
		http.StatusConflict, errCodeSettingAboveLimit)
	// Only the administrator reads another account's limits.
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodGet, "/api/v1/rights/setting-limits?reseller_id=9702", ""), http.StatusForbidden, errCodeAdminOnly)
}
