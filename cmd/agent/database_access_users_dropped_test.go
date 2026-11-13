package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab finding (MariaDB 11.8) and its review: when a database is
// deleted, a user that stays (kept for its owner's rights, or shared with
// another database) loses every right that names the dropped database, on
// every LISTED host of its name (never the empty host of a MariaDB role of
// the same name); only while no database of that name exists; database
// level by its two pattern forms, table, column and routine level by the
// literal name; checked afterwards against what the server has loaded. A
// right on another database or a wildcard pattern is never touched.

const (
	hand5         = "pastane_dr_test_hand5"
	hand5User     = "pastane_dr_test_hand5_user"
	hand5Owner    = " WHERE User = 'pastane_dr_test_hand5_user' AND Host IN ('10.%', 'localhost')"
	hand5Absent   = " AND NOT EXISTS (SELECT 1 FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = 'pastane_dr_test_hand5')"
	hand5Patterns = `('pastane_dr_test_hand5', 'pastane\\_dr\\_test\\_hand5')`
)

// The exact removal for the hand5 user on hosts '10.%' and localhost.
var hand5Deletes = []string{
	"DELETE FROM mysql.db" + hand5Owner + " AND Db IN " + hand5Patterns + hand5Absent + ";",
	"DELETE FROM mysql.tables_priv" + hand5Owner + " AND Db = 'pastane_dr_test_hand5'" + hand5Absent + ";",
	"DELETE FROM mysql.columns_priv" + hand5Owner + " AND Db = 'pastane_dr_test_hand5'" + hand5Absent + ";",
	"DELETE FROM mysql.procs_priv" + hand5Owner + " AND Db = 'pastane_dr_test_hand5'" + hand5Absent + ";",
	"FLUSH PRIVILEGES;",
}

func droppedRemoval(t *testing.T, rightsOnly bool, answer func(string) string) (transport.DatabaseAccessUserResponse, []string) {
	t.Helper()
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			// A MariaDB role of the same name has Host '' and is not listed.
			return "host=10.%\nhost=localhost\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA"):
			if out := answer(statement); out != "" {
				return out
			}
			return "n=0\n"
		}
		if out := answer(statement); out != "" {
			return out
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: hand5,
		Username: hand5User, DatabaseDropped: true, RightsOnly: rightsOnly}, &resp)
	return resp, *got
}

func deleteStatements(got []string) []string {
	for _, statement := range got {
		if strings.HasPrefix(statement, "DELETE FROM") {
			return strings.Split(statement, "\n")
		}
	}
	return nil
}

func TestDroppedDatabaseLeavesNoRightOnAHandMadeHost(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, false, func(string) string { return "" })
	if !resp.Applied || !resp.Kept || resp.RightsLeft || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	if strings.Contains(strings.Join(got, "\n"), "DROP USER") {
		t.Fatal("an account with a host its owner made was dropped")
	}
	// Exactly these statements: the listed hosts only (a role's empty host
	// is not among them), exact name forms per table, never while the
	// database exists. A widened WHERE fails here.
	if deletes := deleteStatements(got); strings.Join(deletes, "\n") != strings.Join(hand5Deletes, "\n") {
		t.Fatalf("removal statements:\n%s\nwant:\n%s", strings.Join(deletes, "\n"), strings.Join(hand5Deletes, "\n"))
	}
	// What the server LOADED after the flush is read, not the tables again.
	var loaded string
	for _, statement := range got {
		if strings.HasPrefix(statement, "SELECT CONCAT('n=', (SELECT COUNT(*) FROM information_schema.SCHEMA_PRIVILEGES") {
			loaded = statement
		}
	}
	for _, want := range []string{
		"BINARY TABLE_SCHEMA IN " + hand5Patterns,
		"information_schema.TABLE_PRIVILEGES WHERE GRANTEE IN (",
		"information_schema.COLUMN_PRIVILEGES WHERE GRANTEE IN (",
		"BINARY TABLE_SCHEMA = 'pastane_dr_test_hand5'",
		"FROM mysql.procs_priv" + hand5Owner + " AND Db = 'pastane_dr_test_hand5'",
	} {
		if !strings.Contains(loaded, want) {
			t.Fatalf("the loaded-rights check lacks %q: %s", want, loaded)
		}
	}
}

// A shared user (another database uses it): rights only - no DROP USER, no
// check of other rights; the same exact removal.
func TestDroppedDatabaseSharedUserLosesOnlyItsRightsOnIt(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, true, func(string) string { return "" })
	all := strings.Join(got, "\n")
	if !resp.Applied || resp.Kept || strings.Contains(all, "DROP USER") || strings.Contains(all, "information_schema.USER_PRIVILEGES") {
		t.Fatalf("resp = %+v statements %v", resp, got)
	}
	if deletes := deleteStatements(got); strings.Join(deletes, "\n") != strings.Join(hand5Deletes, "\n") {
		t.Fatalf("removal statements %v", deletes)
	}
}

func TestDroppedDatabaseThatStillExistsChangesNothing(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, true, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA") {
			return "n=1\n"
		}
		return ""
	})
	if resp.Applied || !resp.DatabaseExists || deleteStatements(got) != nil || strings.Contains(strings.Join(got, "\n"), "DROP USER") {
		t.Fatalf("resp = %+v statements %v", resp, got)
	}
}

func TestDroppedDatabaseRightThatIsStillLoadedIsSaidAsLeft(t *testing.T) {
	withOwnHosts(t)
	resp, _ := droppedRemoval(t, true, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('n=', (SELECT COUNT(*) FROM information_schema.SCHEMA_PRIVILEGES") {
			return "n=1\n"
		}
		return ""
	})
	if resp.Applied || !resp.RightsLeft || resp.Error == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

// The other-rights check compares schema names BINARY (information_schema
// is case-insensitive: a hand-made right on SHOP_A must not pass for shop_a),
// with the pattern forms at database level and the literal name elsewhere;
// a wildcard pattern such as `shop\_%` is neither and counts as another
// right.
func TestOtherRightsCheckComparesSchemaNamesExactly(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return "host=localhost\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop_a", Username: "shop_a_user"}, &resp)
	var check string
	for _, statement := range *got {
		if strings.Contains(statement, "information_schema.USER_PRIVILEGES") {
			check = statement
		}
	}
	for _, want := range []string{
		"BINARY TABLE_SCHEMA NOT IN ('shop_a', 'shop\\\\_a')",
		"TABLE_PRIVILEGES WHERE GRANTEE IN ('\\'shop_a_user\\'@\\'localhost\\'') AND BINARY TABLE_SCHEMA <> 'shop_a'",
		"COLUMN_PRIVILEGES WHERE GRANTEE IN ('\\'shop_a_user\\'@\\'localhost\\'') AND BINARY TABLE_SCHEMA <> 'shop_a'",
		"AND Db <> 'shop_a'",
	} {
		if !strings.Contains(check, want) {
			t.Fatalf("the other-rights check lacks %q:\n%s", want, check)
		}
	}
}

// The domain page's "remove added user" acts on a database that still
// exists (no DatabaseDropped): only CelikPanel's own hosts lose their grant
// there, compared exactly; nothing is deleted from the grant tables. An
// escaped-form grant the owner wrote by hand on localhost (`shop\_a`.*)
// names this database as well: when the account has no other rights it is
// dropped with everything (removing the user from this database is what was
// asked); when it is kept for other rights, only CelikPanel's own grant
// (written `shop_a`.*) is revoked and the owner's hand-made line stays - it
// is the owner's change on a live database.
func TestRemovingAUserFromALiveDatabaseTouchesOnlyCelikPanelsHosts(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.%\nhost=localhost\n"
		case strings.Contains(statement, "BINARY TABLE_SCHEMA = 'shop'"):
			return "n=1\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	all := strings.Join(*got, "\n")
	if !resp.Applied || !resp.Kept || strings.Contains(all, "DELETE FROM mysql.") || strings.Contains(all, "FROM 'shop_user'@'10.%'") ||
		!strings.Contains(all, "REVOKE ALL PRIVILEGES ON `shop`.* FROM 'shop_user'@'localhost';") {
		t.Fatalf("resp = %+v statements %v", resp, *got)
	}

	escaped := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return "host=localhost\n"
		}
		return "n=0\n"
	})
	resp = transport.DatabaseAccessUserResponse{}
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop_a", Username: "shop_a_user"}, &resp)
	if !resp.Applied || resp.Kept || !strings.Contains(strings.Join(*escaped, "\n"), "DROP USER 'shop_a_user'@'localhost'") {
		t.Fatalf("resp = %+v statements %v", resp, *escaped)
	}
}
