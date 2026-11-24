package main

import (
	"net/http"
	"strings"
	"testing"
)

// A row of a key this release does not know (a later release's, kept after
// going back one release; alpha.147 brought this rule for the site limits,
// whose site.* and db.* keys this release knows and edits itself): every plan
// and account save keeps it, it is not shown, and it goes along when an
// account leaves its plan for Custom. The keys planted below are not
// registered by this release (site.disk_mb is not a key of the site limits,
// docs/SITE-LIMITS.md section 4a).

func storedCeiling(t *testing.T, f rightsFixture, kind string, id int, key string) int {
	t.Helper()
	return f.count(t, `SELECT COALESCE((SELECT value FROM setting_ceilings WHERE owner_kind = ? AND owner_id = ? AND key = ?), -1)`, kind, id, key)
}

func plantLaterReleaseCeilings(t *testing.T, f rightsFixture, kind string, id int) {
	t.Helper()
	f.exec(t, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES (?, ?, 'site.disk_mb', 8192), (?, ?, 'later.release_key', 300)`,
		kind, id, kind, id)
}

func TestPlanAndAccountSavesKeepCeilingsThisReleaseDoesNotKnow(t *testing.T) {
	f := newRightsFixture(t)
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Later","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":256}}`)
	plantLaterReleaseCeilings(t, f, rightsOwnerPlan, plan)

	// A plan save: the known key changes as today, the others stay.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Renamed","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"php.memory_limit_mb":300}}`), http.StatusOK, "")
	if got := storedCeiling(t, f, rightsOwnerPlan, plan, settingPHPMemoryMB); got != 300 {
		t.Fatalf("known key after the save = %d", got)
	}
	if storedCeiling(t, f, rightsOwnerPlan, plan, "site.disk_mb") != 8192 || storedCeiling(t, f, rightsOwnerPlan, plan, "later.release_key") != 300 {
		t.Fatal("a plan save deleted the ceilings of keys this release does not know")
	}
	// A save without the known key removes it, as today; the others stay.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Renamed","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{}}`), http.StatusOK, "")
	if got := storedCeiling(t, f, rightsOwnerPlan, plan, settingPHPMemoryMB); got != -1 {
		t.Fatalf("a removed known key stayed: %d", got)
	}
	if storedCeiling(t, f, rightsOwnerPlan, plan, "site.disk_mb") != 8192 {
		t.Fatal("a save without ceilings deleted an unknown key")
	}
	// Not shown.
	if list := f.call(t, authzMatrixAdminID, http.MethodGet, "/api/v1/plans", ""); strings.Contains(list.Body.String(), "site.disk_mb") {
		t.Fatalf("an unknown key is shown: %s", list.Body.String())
	}

	// The account leaves the plan for Custom: the plan's rows go with it.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPatch, "/api/v1/subscriptions/9802/rights", `{"revoke":["permission:dns_edit"],"confirm":true}`), http.StatusOK, "")
	if storedCeiling(t, f, rightsOwnerSubscription, 9802, "site.disk_mb") != 8192 || storedCeiling(t, f, rightsOwnerSubscription, 9802, "later.release_key") != 300 {
		t.Fatal("leaving the plan dropped the plan's ceilings of keys this release does not know")
	}
	// An account save of the Custom subscription keeps them.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/subscriptions/9802/rights",
		`{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},"quantities":{"max_domains":4},"ceilings":{"php.memory_limit_mb":200},"confirm":true}`), http.StatusOK, "")
	if got := storedCeiling(t, f, rightsOwnerSubscription, 9802, settingPHPMemoryMB); got != 200 {
		t.Fatalf("the account's known key = %d", got)
	}
	if storedCeiling(t, f, rightsOwnerSubscription, 9802, "site.disk_mb") != 8192 {
		t.Fatal("an account save deleted the ceilings of keys this release does not know")
	}

	// A Custom reseller's save keeps them too.
	plantLaterReleaseCeilings(t, f, rightsOwnerReseller, authzMatrixResellerID)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/"+itoa(authzMatrixResellerID)+"/rights",
		`{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},"quantities":{"max_domains":10},"ceilings":{"php.memory_limit_mb":384},"overselling":"not_allowed","confirm":true}`), http.StatusOK, "")
	if storedCeiling(t, f, rightsOwnerReseller, authzMatrixResellerID, settingPHPMemoryMB) != 384 ||
		storedCeiling(t, f, rightsOwnerReseller, authzMatrixResellerID, "site.disk_mb") != 8192 {
		t.Fatal("a reseller save deleted the ceilings of keys this release does not know")
	}
}

// TestResellerLeavingItsResellerPlanKeepsUnknownCeilings: a reseller bound to
// a reseller plan that has rows of keys this release does not know gets them
// copied once when it becomes Custom; saving it again neither duplicates nor
// drops them, and they go with the reseller plan when it is deleted.
func TestResellerLeavingItsResellerPlanKeepsUnknownCeilings(t *testing.T) {
	f := newRightsFixture(t)
	created := f.call(t, authzMatrixAdminID, http.MethodPost, resellerPlansPath,
		`{"name":"Gold","max_customers":3,"max_domains":30,"max_databases":0,"max_email_accounts":0,"disk_quota_mb":0,"rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},"ceilings":{"php.memory_limit_mb":384}}`)
	wantStatus(t, created, http.StatusOK, "")
	gold := int(decodeBody(t, created)["id"].(float64))
	plantLaterReleaseCeilings(t, f, rightsOwnerResellerPlan, gold)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/"+itoa(authzMatrixResellerID)+"/rights",
		`{"state":"plan","plan_id":`+itoa(gold)+`,"confirm":true}`), http.StatusOK, "")
	if n := f.count(t, `SELECT COUNT(*) FROM setting_ceilings WHERE owner_kind = 'reseller' AND owner_id = ?`, authzMatrixResellerID); n != 0 {
		t.Fatalf("a bound reseller has %d rows of its own", n)
	}
	custom := `{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},"quantities":{"max_domains":10},"ceilings":{"php.memory_limit_mb":256},"overselling":"not_allowed","confirm":true}`
	for round := 0; round < 2; round++ {
		wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/"+itoa(authzMatrixResellerID)+"/rights", custom), http.StatusOK, "")
		if storedCeiling(t, f, rightsOwnerReseller, authzMatrixResellerID, "site.disk_mb") != 8192 ||
			storedCeiling(t, f, rightsOwnerReseller, authzMatrixResellerID, "later.release_key") != 300 ||
			storedCeiling(t, f, rightsOwnerReseller, authzMatrixResellerID, settingPHPMemoryMB) != 256 {
			t.Fatalf("round %d: the reseller's rows after leaving its reseller plan are wrong", round)
		}
		if n := f.count(t, `SELECT COUNT(*) FROM setting_ceilings WHERE owner_kind = 'reseller' AND owner_id = ? AND key IN ('site.disk_mb', 'later.release_key')`, authzMatrixResellerID); n != 2 {
			t.Fatalf("round %d: %d unknown rows", round, n)
		}
	}
	// The reseller plan's own rows go with it (the 066 trigger).
	f.exec(t, `DELETE FROM reseller_plans WHERE id = ?`, gold)
	if n := f.count(t, `SELECT COUNT(*) FROM setting_ceilings WHERE owner_kind = 'reseller_plan' AND owner_id = ?`, gold); n != 0 {
		t.Fatalf("%d rows outlived their reseller plan", n)
	}
}
