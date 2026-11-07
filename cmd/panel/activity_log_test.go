package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.128 G23: each account reads its own action log, filtered on the
// server by rights; details and other people's addresses never leave.

type activityTestBody struct {
	Entries    []activityEntry `json:"entries"`
	NextBefore *int            `json:"next_before"`
}

func readActivityAs(t *testing.T, fixture authzMatrixFixture, caller *Caller, query string) (int, activityTestBody, string) {
	t.Helper()
	request := teamMemberAuthzTestRequest(http.MethodGet, activityLogPath+query, caller)
	recorder := httptest.NewRecorder()
	fixture.panel.handleActivityLog(recorder, request)
	var body activityTestBody
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return recorder.Code, body, recorder.Body.String()
}

func seedActivityRows(t *testing.T, fixture authzMatrixFixture) map[string]int {
	t.Helper()
	rows := []struct {
		key      string
		user     any
		actingAs any
		action   string
		ip       string
	}{
		{"customer", authzMatrixCustomerID, nil, "domain.create:customer.matrix.test — reason with /secret/path", "10.0.0.3"},
		{"adminAsCustomer", authzMatrixAdminID, authzMatrixCustomerID, "backup.create:files", "10.0.0.1"},
		{"member", authzMatrixAdditionalUserID, nil, "mail.account.create:a@customer.matrix.test", "10.0.0.9"},
		{"reseller", authzMatrixResellerID, nil, "user.create:matrix-customer/x", "10.0.0.2"},
		{"outsider", authzMatrixOutsiderID, nil, "domain.create:outsider.matrix.test", "10.0.0.4"},
		{"admin", authzMatrixAdminID, nil, "config.write:/etc/nginx/nginx.conf", "10.0.0.1"},
		{"adminAsReseller", authzMatrixAdminID, authzMatrixResellerID, "plan.update", "10.0.0.1"},
		{"resellerAsCustomer", authzMatrixResellerID, authzMatrixCustomerID, "dns.record.add:customer.matrix.test", "10.0.0.2"},
	}
	ids := map[string]int{}
	for _, row := range rows {
		result, err := fixture.panel.db.GetDB().Exec(`
			INSERT INTO audit_logs (user_id, acting_as, action, resource_type, resource_id, ip_address, details)
			VALUES (?, ?, ?, 'domain', 9902, ?, '{"password":"never"}')`, row.user, row.actingAs, row.action, row.ip)
		if err != nil {
			t.Fatalf("seed %s: %v", row.key, err)
		}
		id, _ := result.LastInsertId()
		ids[row.key] = int(id)
	}
	return ids
}

func activityByID(entries []activityEntry) map[int]activityEntry {
	out := map[int]activityEntry{}
	for _, e := range entries {
		out[e.ID] = e
	}
	return out
}

func TestActivityLogIsFilteredByRightsOnTheServer(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	ids := seedActivityRows(t, fixture)

	admin := &Caller{ID: authzMatrixAdminID, Role: roleAdmin, AccountType: core.AccountTypeAccount}
	reseller := &Caller{ID: authzMatrixResellerID, Role: roleReseller, AccountType: core.AccountTypeAccount}
	customer := &Caller{ID: authzMatrixCustomerID, Role: roleCustomer, AccountType: core.AccountTypeAccount, CustomerID: authzMatrixCustomerID}
	member := teamMemberAuthzTestCaller()

	want := map[string][]string{
		"customer": {"customer", "adminAsCustomer", "member", "resellerAsCustomer"},
		"reseller": {"customer", "adminAsCustomer", "member", "reseller", "adminAsReseller", "resellerAsCustomer"},
		"member":   {"member"},
		"admin":    {"adminAsCustomer", "admin", "adminAsReseller"},
	}
	callers := map[string]*Caller{"customer": customer, "reseller": reseller, "member": member, "admin": admin}
	seen := map[string]map[int]activityEntry{}
	for name, caller := range callers {
		code, body, raw := readActivityAs(t, fixture, caller, "")
		if code != http.StatusOK {
			t.Fatalf("%s: status %d %s", name, code, raw)
		}
		if strings.Contains(raw, "secret") || strings.Contains(raw, "password") || strings.Contains(raw, "/etc/") {
			t.Fatalf("%s: details or a path left the server: %s", name, raw)
		}
		got := []int{}
		for _, e := range body.Entries {
			got = append(got, e.ID)
		}
		expected := []int{}
		for _, key := range want[name] {
			expected = append(expected, ids[key])
		}
		sort.Sort(sort.Reverse(sort.IntSlice(expected)))
		if fmt.Sprint(got) != fmt.Sprint(expected) {
			t.Fatalf("%s sees %v, want %v (%s)", name, got, expected, raw)
		}
		seen[name] = activityByID(body.Entries)
	}

	// The customer: its own row with its address, the administrator and the
	// provider by role (no name, no address), its team member by name.
	c := seen["customer"]
	if e := c[ids["customer"]]; e.Actor.Kind != "you" || e.IP != "10.0.0.3" || e.Action != "domain.create:customer.matrix.test" {
		t.Fatalf("customer own row = %+v", e)
	}
	if e := c[ids["adminAsCustomer"]]; e.Actor.Kind != "administrator" || e.Actor.Name != "" || e.IP != "" || e.As != "" {
		t.Fatalf("administrator as the customer = %+v", e)
	}
	if e := c[ids["resellerAsCustomer"]]; e.Actor.Kind != "provider" || e.IP != "" {
		t.Fatalf("provider as the customer = %+v", e)
	}
	if e := c[ids["member"]]; e.Actor.Kind != "team_member" || e.Actor.Name != "matrix-member" || e.IP != "" {
		t.Fatalf("team member row = %+v", e)
	}
	// The reseller: its customer by name, its own "Log in as" with the
	// account, the administrator acting as its customer with that account.
	r := seen["reseller"]
	if e := r[ids["customer"]]; e.Actor.Kind != "account" || e.Actor.Name != "matrix-customer" || e.IP != "" {
		t.Fatalf("reseller sees customer row = %+v", e)
	}
	if e := r[ids["resellerAsCustomer"]]; e.Actor.Kind != "you" || e.As != "matrix-customer" || e.IP != "10.0.0.2" {
		t.Fatalf("reseller's own Log in as = %+v", e)
	}
	if e := r[ids["adminAsCustomer"]]; e.Actor.Kind != "administrator" || e.As != "matrix-customer" {
		t.Fatalf("administrator as the reseller's customer = %+v", e)
	}
	if e := r[ids["reseller"]]; e.Action != "user.create" {
		t.Fatalf("a target that is not plain is left out: %+v", e)
	}
	if e := seen["admin"][ids["admin"]]; e.Action != "config.write" {
		t.Fatalf("a path target is left out: %+v", e)
	}
}

func TestActivityLogPagesWithoutRepeating(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	seedActivityRows(t, fixture)
	reseller := &Caller{ID: authzMatrixResellerID, Role: roleReseller, AccountType: core.AccountTypeAccount}

	all := map[int]bool{}
	query := "?limit=2"
	for page := 0; page < 10; page++ {
		code, body, raw := readActivityAs(t, fixture, reseller, query)
		if code != http.StatusOK || len(body.Entries) == 0 || len(body.Entries) > 2 {
			t.Fatalf("page %d: %d %s", page, code, raw)
		}
		for _, e := range body.Entries {
			if all[e.ID] {
				t.Fatalf("row %d repeated", e.ID)
			}
			all[e.ID] = true
		}
		if body.NextBefore == nil {
			break
		}
		query = fmt.Sprintf("?limit=2&before=%d", *body.NextBefore)
	}
	if len(all) != 6 {
		t.Fatalf("paged %d rows, want 6", len(all))
	}
	for _, bad := range []string{"?limit=0", "?limit=101", "?limit=x", "?before=0", "?before=-3"} {
		if code, _, _ := readActivityAs(t, fixture, reseller, bad); code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", bad, code)
		}
	}
	request := teamMemberAuthzTestRequest(http.MethodPost, activityLogPath, reseller)
	recorder := httptest.NewRecorder()
	fixture.panel.handleActivityLog(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status %d", recorder.Code)
	}
}

func TestActivityActionKeepsOnlyThePlainNameAndTarget(t *testing.T) {
	for raw, want := range map[string]string{
		"domain.create:example.com":                  "domain.create:example.com",
		"backup.create.failed:scheduled — disk full": "backup.create.failed:scheduled",
		"service.uninstall:nginx:nginx-core":         "service.uninstall:nginx",
		"config.read:/etc/shadow":                    "config.read",
		"mail.account.create:a@example.com extra":    "mail.account.create:a@example.com",
		"Not An Action":                              "",
		"auth.login":                                 "auth.login",
		"x.y:<script>":                               "x.y",
		strings.Repeat("a", 81):                      "",
	} {
		if got := activityAction(raw); got != want {
			t.Errorf("activityAction(%q) = %q, want %q", raw, got, want)
		}
	}
}
