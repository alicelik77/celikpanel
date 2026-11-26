package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of PR #370, findings 9 and 16: an Agent refusal of a database
// account travels to the screen as its code (DATABASE_ACCOUNT_REFUSED; the
// refusal as the reason; the refused account as vars.user), and it names the
// account the Agent refused - on a create the database's owner or one of its
// other users, never the new user (which does not exist).
func TestADatabaseAccountRefusalTravelsAsItsCodeAndNamesTheRefusedAccount(t *testing.T) {
	f := newAccessUsersFixture(t)
	native := &fakeDatabaseAccessNative{}
	f.panel.databaseAccessNative = native
	const path = "/api/v1/domains/9902/databases/9961/users"
	decode := func(t *testing.T, body string) apiErrorBody {
		t.Helper()
		var answer apiErrorBody
		if err := json.Unmarshal([]byte(body), &answer); err != nil {
			t.Fatalf("%v: %s", err, body)
		}
		return answer
	}

	native.refused = &transport.DatabaseAccessUserResponse{AccountRefused: transport.DatabaseAccountRefusedPrivileged, AccountRefusedName: "app_owner",
		Error: "the database user app_owner holds administrator rights"}
	got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"x","password":"Str0ng-Passw0rd!","access":"write"}`)
	answer := decode(t, got.Body.String())
	if got.Code != http.StatusConflict || answer.Code != errCodeDatabaseAccountRefused || answer.Reason != transport.DatabaseAccountRefusedPrivileged ||
		answer.Vars["user"] != "app_owner" || strings.Contains(got.Body.String(), "customer_matrix_test_shop_x") || !strings.Contains(answer.Error, "Nothing was changed") {
		t.Fatalf("create: %d %s", got.Code, got.Body.String())
	}

	// A removal names the account the Agent named, else the user removed.
	native.refused = nil
	if got := f.call(t, authzMatrixCustomerID, http.MethodPost, path, `{"name":"reader","password":"Str0ng-Passw0rd!","access":"read"}`); got.Code != http.StatusOK {
		t.Fatalf("add: %d %s", got.Code, got.Body.String())
	}
	var userID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM database_users WHERE username = 'customer_matrix_test_shop_reader'`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, want, code string }{
		{"", "customer_matrix_test_shop_reader", transport.DatabaseAccountRefusedUnsupportedEngine},
		{"postgres", "postgres", transport.DatabaseAccountRefusedPrivileged},
		// Third review of PR #370, H1: the user owns objects in a database
		// of celikpanel_admin; the sentence says so, not "could not be read".
		{"customer_matrix_test_shop_reader", "customer_matrix_test_shop_reader", transport.DatabaseAccountRefusedOwnsObjects},
	} {
		native.refused = &transport.DatabaseAccessUserResponse{AccountRefused: c.code, AccountRefusedName: c.name, Error: "refused"}
		got := f.call(t, authzMatrixCustomerID, http.MethodDelete, path+"/"+strconv.Itoa(userID), "")
		answer := decode(t, got.Body.String())
		if got.Code != http.StatusConflict || answer.Code != errCodeDatabaseAccountRefused || answer.Reason != c.code || answer.Vars["user"] != c.want {
			t.Fatalf("remove (%q): %d %s", c.name, got.Code, got.Body.String())
		}
		if c.code == transport.DatabaseAccountRefusedOwnsObjects && (!strings.Contains(answer.Error, "owns tables or other objects") || strings.Contains(answer.Error, "could not be read")) {
			t.Fatalf("owns objects: %s", answer.Error)
		}
	}
	// Nothing was removed from CelikPanel's list.
	if f.count(t, `SELECT COUNT(*) FROM database_users WHERE id = ?`, userID) != 1 {
		t.Fatal("a refused removal removed the record")
	}
}

// alpha.150: deleting a database whose users the Agent refuses for a reason
// only the server's administrator can lift. The database is deleted; the
// answer names the accounts left and why, says that the server's
// administrator acts and that Delete finishes after that, and never asks for
// another press (it would meet the same refusal). The records stay, so the
// database stays listed. An absent account or the system database is not
// such a refusal: the answer is the earlier one.
func TestDatabaseDeleteWithARefusedUserSaysWhoActs(t *testing.T) {
	for _, c := range []struct {
		code, reason, words string
		sweepRefused        bool
	}{
		{transport.DatabaseAccountRefusedPrivileged, "refused_privileged", "they hold administrator rights there", false},
		// The sweep of the rights naming it is refused for the same owners.
		{transport.DatabaseAccountRefusedPrivileged, "refused_privileged", "takes those rights away", true},
		{transport.DatabaseAccountRefusedMember, "refused_member", "members of another PostgreSQL role", false},
		{transport.DatabaseAccountRefusedOwnsObjects, "refused_owns_objects", "own tables or other objects", false},
		{transport.DatabaseAccountRefusedUnreadable, "refused_unreadable", "checks that the database server answers", false},
		{transport.DatabaseAccountRefusedUnsupportedEngine, "refused_unsupported_engine", "runs MySQL in MariaDB's place", false},
		{"a_later_code", "refused", "refused to change them", false},
		{transport.DatabaseAccountRefusedAbsent, "mixed", "press Delete again", false},
	} {
		f := newVisibleDatabasesFixture(t)
		drops := recordDatabaseDrops(t)
		if c.sweepRefused {
			drops.sweepAnswer = func(resp *transport.DroppedDatabaseRightsResponse) error {
				resp.AccountRefused, resp.Error = c.code, "refused"
				return nil
			}
		}
		f.panel.databaseAccessNative = &fakeDatabaseAccessNative{refused: &transport.DatabaseAccessUserResponse{AccountRefused: c.code, Error: "refused"}}
		got := f.as(t, "admin", http.MethodDelete, "/api/v1/databases/9961", "")
		var answer apiErrorBody
		_ = json.Unmarshal(got.Body.Bytes(), &answer)
		if got.Code != http.StatusConflict || answer.Code != errCodeDatabaseDeleteUsersLeft || answer.Reason != c.reason || !answer.PartialSuccess ||
			answer.Vars["name"] != "customer_matrix_test_shop" || !strings.Contains(answer.Error, c.words) {
			t.Fatalf("%s: %d %s", c.code, got.Code, got.Body.String())
		}
		if c.reason != "mixed" {
			if answer.Vars["refused"] != "cust_user, wp_user" || !strings.Contains(answer.Error, "customer_matrix_test_shop was deleted.") ||
				!strings.Contains(answer.Error, "cust_user, wp_user were left") || !strings.Contains(answer.Error, "the server's administrator") ||
				!strings.HasSuffix(answer.Error, "; then Delete finishes.") || strings.Contains(strings.ToLower(answer.Error), "again") {
				t.Fatalf("%s: %s", c.code, got.Body.String())
			}
		} else if _, ok := answer.Vars["refused"]; ok {
			t.Fatalf("%s: %s", c.code, got.Body.String())
		}
		if f.count(t, `SELECT COUNT(*) FROM databases_v2 WHERE id = 9961`) != 1 || len(drops.requests) != 1 {
			t.Fatalf("%s: the record went or the drop did not run once", c.code)
		}
	}
}
