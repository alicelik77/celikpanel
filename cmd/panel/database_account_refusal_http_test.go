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
