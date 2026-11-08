package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// alpha.130: every row of the Offered tab changes the store its item is
// really offered in, or says why it cannot - never a 200 that changes
// nothing (the administrator could not tick "Remote access to databases").
// An entry an earlier release wrote into the wrong store is folded into its
// own store as a withdrawn offer on the first read; nothing is opened.

type offeredAnswer struct {
	HiddenOfferings []string          `json:"hidden_offerings"`
	Item            *offeredItemState `json:"item"`
}

func putOffered(t *testing.T, f rightsFixture, item string, offered bool) (*httptest.ResponseRecorder, offeredAnswer) {
	t.Helper()
	body, _ := json.Marshal(offeredVersionChange{OfferingID: item, Offered: offered})
	recorder := f.call(t, authzMatrixAdminID, http.MethodPut, offeredVersionsPath, string(body))
	var answer offeredAnswer
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatalf("answer: %v %s", err, recorder.Body.String())
		}
	}
	return recorder, answer
}

func storedSetting(t *testing.T, f rightsFixture, key string) string {
	t.Helper()
	var value string
	_ = f.panel.db.GetDB().QueryRow(`SELECT value FROM panel_settings WHERE key = ?`, key).Scan(&value)
	return value
}

func TestOfferedTabChangesAnOptInPermissionInItsOwnStoreBothWays(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	for _, permission := range []string{rightDatabaseRemote, rightNginxDirectives} {
		recorder, answer := putOffered(t, f, permissionItemHead+permission, true)
		if recorder.Code != http.StatusOK || answer.Item == nil || answer.Item.ID != permissionItemHead+permission || !answer.Item.Offered {
			t.Fatalf("%s offer = %d %s", permission, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(storedSetting(t, f, settingHiddenOfferings), permission) {
			t.Fatalf("%s went to the hidden list", permission)
		}
		offered, _ := f.panel.loadOfferedOptInPermissions(ctx)
		if !offered[permission] {
			t.Fatalf("%s not in the opt-in store: %s", permission, storedSetting(t, f, settingOfferedPermissions))
		}
	}
	// Its own switches read the same store.
	if on, _ := f.panel.databaseRemoteOffered(ctx); !on {
		t.Fatal("the remote database switch does not follow the Offered tab")
	}
	if !f.panel.nginxDirectivesOffered(ctx) {
		t.Fatal("the nginx directives card does not follow the Offered tab")
	}
	var audits int
	f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'rights.offer_permission:database_remote'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("audit rows = %d", audits)
	}

	// The other direction: the nginx card withdraws, the Offered tab reads it.
	req := serviceStateAtomicRequest(http.MethodPut, "/api/v1/nginx/directives-offer", `{"offered":false}`, authzMatrixAdminID)
	f.panel.handleNginxDirectivesOffer(httptest.NewRecorder(), req)
	catalog := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/catalog", "")
	var view struct {
		Rights effectiveRightsView `json:"rights"`
	}
	if err := json.Unmarshal(catalog.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	states := map[string]bool{}
	for _, item := range view.Rights.Permissions {
		states[item.ID] = item.Granted
	}
	if states[rightNginxDirectives] || !states[rightDatabaseRemote] {
		t.Fatalf("catalog after the card = %v", states)
	}

	// Withdrawing from the tab closes the switch.
	if recorder, answer := putOffered(t, f, permissionItemHead+rightDatabaseRemote, false); recorder.Code != http.StatusOK || answer.Item.Offered {
		t.Fatalf("withdraw = %d %s", recorder.Code, recorder.Body.String())
	}
	if on, _ := f.panel.databaseRemoteOffered(ctx); on {
		t.Fatal("withdrawn in the tab, still on at the switch")
	}
}

func TestAStaleHiddenEntryStaysClosedAndIsMovedToItsOwnStore(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	// A 129 install: both opt-ins offered by their switches, then unticked in
	// the Offered tab (hidden), and uploaded programs unticked there too.
	for key, value := range map[string]string{
		settingOfferedPermissions: `["database_remote","nginx_directives"]`,
		settingHiddenOfferings:    `["component:mariadb","permission:database_remote","permission:dns_edit","runtime:program"]`,
	} {
		if _, err := db.Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO runtime_offerings (offering_id, kind, offered, memory_mb, cpu_percent, tasks_max) VALUES ('runtime:program', 'program', 1, 512, 100, 256)`); err != nil {
		t.Fatal(err)
	}
	// Before any fold: what the hidden entry did, it still does.
	if offered, _ := f.panel.loadOfferedOptInPermissions(ctx); offered[rightDatabaseRemote] || !offered[rightNginxDirectives] {
		t.Fatalf("opt-in before the fold = %v", offered)
	}
	hidden, err := f.panel.loadHiddenOfferings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 2 || !hidden["component:mariadb"] || !hidden["permission:dns_edit"] {
		t.Fatalf("hidden after the fold = %v", hidden)
	}
	if got := storedSetting(t, f, settingOfferedPermissions); got != `["nginx_directives"]` {
		t.Fatalf("opt-in store after the fold = %s", got)
	}
	var runtimeOffered int
	db.QueryRow(`SELECT offered FROM runtime_offerings WHERE offering_id = 'runtime:program'`).Scan(&runtimeOffered)
	if runtimeOffered != 0 {
		t.Fatal("uploaded programs were not withdrawn in Catalog management")
	}
	var details string
	if err := db.QueryRow(`SELECT details FROM audit_logs WHERE action = 'offering.stale_entries_moved'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(details, "permission:database_remote") || !strings.Contains(details, "runtime:program") {
		t.Fatalf("audit = %s", details)
	}
	layer, err := f.panel.loadAdminOfferLayer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if layer.permits(rightDatabaseRemote) || layer.offers("runtime:program") || !layer.permits(rightNginxDirectives) {
		t.Fatal("the fold opened or closed something")
	}
	// The remote database switch now opens it for real (the stale entry no
	// longer overrides it).
	if err := f.panel.saveOfferedOptInPermissions(ctx, map[string]bool{rightDatabaseRemote: true, rightNginxDirectives: true}); err != nil {
		t.Fatal(err)
	}
	if layer, _ := f.panel.loadAdminOfferLayer(ctx); !layer.permits(rightDatabaseRemote) {
		t.Fatal("switched on after the fold, still closed")
	}
	// A second read finds nothing to move.
	f.panel.loadHiddenOfferings(ctx)
	var rows int
	db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'offering.stale_entries_moved'`).Scan(&rows)
	if rows != 1 {
		t.Fatalf("fold audits = %d", rows)
	}
}

func TestSavingAnOptInOfferRemovesItsStaleHiddenEntry(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	if _, err := f.panel.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, settingHiddenOfferings, `["permission:nginx_directives"]`); err != nil {
		t.Fatal(err)
	}
	if err := f.panel.saveOfferedOptInPermissions(ctx, map[string]bool{rightNginxDirectives: true}); err != nil {
		t.Fatal(err)
	}
	if got := storedSetting(t, f, settingHiddenOfferings); got != `[]` {
		t.Fatalf("hidden = %s", got)
	}
	if !f.panel.nginxDirectivesOffered(ctx) {
		t.Fatal("not offered after the switch")
	}
}

func TestOfferedTabRefusesItemsOfferedInCatalogManagement(t *testing.T) {
	f := newRightsFixture(t)
	for _, item := range []string{"runtime:program", "component:python:3.12", "app:n8n"} {
		recorder, _ := putOffered(t, f, item, true)
		var body apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		if recorder.Code != http.StatusConflict || body.Code != errCodeOfferedInCatalog || body.Action != offeredInCatalogAction {
			t.Fatalf("%s = %d %s", item, recorder.Code, recorder.Body.String())
		}
		if strings.Contains(storedSetting(t, f, settingHiddenOfferings), item) {
			t.Fatalf("%s was stored", item)
		}
	}
}

func TestOfferedTabRefusesAWithdrawnVersionAndKeepsItsHiding(t *testing.T) {
	f := newRightsFixture(t)
	if _, err := f.panel.db.GetDB().Exec(`INSERT INTO security_emergencies (id, component, version, reason) VALUES ('emergency-php83-000001', 'php', '8.3', 'CVE-2026-0001')`); err != nil {
		t.Fatal(err)
	}
	recorder, _ := putOffered(t, f, "component:php-fpm:8.3", true)
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusConflict || body.Code != errCodeVersionWithdrawn || body.Reason != "offer" || body.Vars["name"] != "PHP 8.3" {
		t.Fatalf("withdrawn offer = %d %s", recorder.Code, recorder.Body.String())
	}
	var audits int
	f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'versions.customer_offer:%'`).Scan(&audits)
	if audits != 0 {
		t.Fatal("a refused offer was audited as a change")
	}
	// Hiding it is recorded, so it stays hidden after the emergency.
	recorder, answer := putOffered(t, f, "component:php-fpm:8.3", false)
	if recorder.Code != http.StatusOK || answer.Item == nil || answer.Item.Offered {
		t.Fatalf("hide = %d %s", recorder.Code, recorder.Body.String())
	}
	policy, _ := f.panel.loadOfferedVersions(context.Background())
	if len(policy.Hidden["php"]) != 1 || policy.Hidden["php"][0] != "8.3" {
		t.Fatalf("hidden = %v", policy.Hidden)
	}
	// The catalog marks the version withdrawn.
	catalog := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/catalog", "")
	if !strings.Contains(catalog.Body.String(), `"id":"component:php-fpm:8.3","granted":false,"refused_by":"administrator","grantor":"the administrator","withdrawn":true`) {
		t.Fatalf("catalog = %s", catalog.Body.String())
	}
}

func TestEveryOfferedChangeAnswersTheItemsStateAfterIt(t *testing.T) {
	f := newRightsFixture(t)
	for _, tc := range []struct {
		item    string
		offered bool
	}{
		{"component:mariadb", false}, {"component:mariadb", true},
		{"permission:dns_edit", false}, {"component:node:22", false},
		{"integration:acme:zerossl", false}, {"app:wordpress", false},
	} {
		recorder, answer := putOffered(t, f, tc.item, tc.offered)
		if recorder.Code != http.StatusOK || answer.Item == nil || answer.Item.ID != tc.item || answer.Item.Offered != tc.offered {
			t.Fatalf("%s=%v: %d %s", tc.item, tc.offered, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTheCatalogSaysTheRemoteAPIIsOffAtTheServer(t *testing.T) {
	f := newRightsFixture(t)
	catalog := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/catalog", "")
	if !strings.Contains(catalog.Body.String(), `"id":"remote_api","name":"Remote API","granted":true,"server_off":true`) {
		t.Fatalf("catalog = %s", catalog.Body.String())
	}
	if _, err := f.panel.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES(?, '1')`, settingRemoteAPIEnabled); err != nil {
		t.Fatal(err)
	}
	catalog = f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/catalog", "")
	if strings.Contains(catalog.Body.String(), `server_off`) {
		t.Fatalf("catalog = %s", catalog.Body.String())
	}
}

func TestOnlySettingsWithACustomersOwnValueCanBeFixed(t *testing.T) {
	f := newRightsFixture(t)
	refused := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"cron.max_jobs","server_ceiling":20,"customer_editable":false}`)
	if refused.Code != http.StatusBadRequest || errorCode(t, refused) != errCodeSettingNotCustomerChosen {
		t.Fatalf("cron fixed = %d %s", refused.Code, refused.Body.String())
	}
	if ok := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"cron.max_jobs","server_ceiling":20,"customer_editable":true}`); ok.Code != http.StatusOK {
		t.Fatalf("cron limit = %d %s", ok.Code, ok.Body.String())
	}
	if ok := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/rights/setting-limits", `{"key":"php.memory_limit_mb","server_ceiling":512,"customer_editable":false}`); ok.Code != http.StatusOK {
		t.Fatalf("php fixed = %d %s", ok.Code, ok.Body.String())
	}
	// A "fixed" an earlier release stored for a limit-only key fixes nothing.
	if _, err := f.panel.db.GetDB().Exec(`UPDATE setting_policy SET customer_editable = 0 WHERE key = 'db.max_user_connections'`); err != nil {
		t.Fatal(err)
	}
	read := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/rights/setting-limits", "")
	var body struct {
		Limits []settingLimit     `json:"limits"`
		Policy []settingPolicyRow `json:"policy"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, row := range body.Policy {
		want := strings.HasPrefix(row.Key, "php.")
		if row.CustomerChoice != want {
			t.Fatalf("%s customer_choice = %v", row.Key, row.CustomerChoice)
		}
		if !want && !row.CustomerEditable {
			t.Fatalf("%s reads as fixed", row.Key)
		}
		if row.Key == settingPHPMemoryMB && row.CustomerEditable {
			t.Fatal("php memory not fixed")
		}
	}
	for _, limit := range body.Limits {
		if limit.Fixed != (limit.Key == settingPHPMemoryMB) {
			t.Fatalf("%s fixed = %v", limit.Key, limit.Fixed)
		}
	}
}
