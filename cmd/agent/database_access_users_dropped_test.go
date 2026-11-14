package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab finding (MariaDB 11.8) and its review, widened in alpha.134:
// when a database is deleted, every right that names it goes from EVERY
// grantee - CelikPanel's users, accounts made by hand under any name and
// roles (their rows have the empty host); the accounts and roles stay. Only
// while no database of that name exists; database level by its two name
// forms (row by row), table, column and routine level by the literal name,
// compared as hexadecimal literals; FLUSH on its own; checked afterwards
// against what the server has loaded and stored. A right on another
// database or a wildcard pattern is never touched. (alpha.133's lab result
// "a same-named role's rows are left" is changed on purpose: a re-created
// name must not hand its tables to the role's members.)

const (
	hand5     = "pastane_dr_test_hand5"
	hand5User = "pastane_dr_test_hand5_user"
	// The two forms of the name as hexadecimal literals: as written, and
	// with `_` escaped.
	hand5Hex      = "X'70617374616E655F64725F746573745F68616E6435'"
	hand5ExactHex = "X'70617374616E655C5F64725C5F746573745C5F68616E6435'"
	hand5Absent   = " AND NOT EXISTS (SELECT 1 FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + hand5Hex + ")"
)

// hand5Read: the hand-made account (raw form), an account of another name
// and a role (exact form) hold a database-level right naming it; the
// hand-made account also a table right. No other database exists.
const hand5Read = "l=0\n" +
	"d=70617374616E655F64725F746573745F68616E64355F75736572/31302E25/70617374616E655F64725F746573745F68616E6435\n" +
	"d=6F746865725F6F776E6572/6C6F63616C686F7374/70617374616E655C5F64725C5F746573745C5F68616E6435\n" +
	"d=7265706F7274696E67//70617374616E655C5F64725C5F746573745C5F68616E6435\n" +
	"g=70617374616E655F64725F746573745F68616E64355F75736572/31302E25\n"

// The exact removal: row by row for the database level, every grantee, never
// while the database exists.
var hand5Deletes = []string{
	"DELETE FROM mysql.db WHERE User = X'70617374616E655F64725F746573745F68616E64355F75736572' AND Host = X'31302E25' AND Db = " + hand5Hex + hand5Absent + ";",
	"DELETE FROM mysql.db WHERE User = X'6F746865725F6F776E6572' AND Host = X'6C6F63616C686F7374' AND Db = " + hand5ExactHex + hand5Absent + ";",
	"DELETE FROM mysql.db WHERE User = X'7265706F7274696E67' AND Host = '' AND Db = " + hand5ExactHex + hand5Absent + ";",
	"DELETE FROM mysql.tables_priv WHERE Db = " + hand5Hex + hand5Absent + ";",
	"DELETE FROM mysql.columns_priv WHERE Db = " + hand5Hex + hand5Absent + ";",
	"DELETE FROM mysql.procs_priv WHERE Db = " + hand5Hex + hand5Absent + ";",
}

func droppedRemoval(t *testing.T, rightsOnly bool, answer func(string) string) (transport.DatabaseAccessUserResponse, []string) {
	t.Helper()
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.%\nhost=localhost\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA"):
			if out := answer(statement); out != "" {
				return out
			}
			return "n=0\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('l='"):
			return hand5Read
		case strings.HasPrefix(statement, "SELECT CONCAT('a='"):
			if out := answer(statement); out != "" {
				return out
			}
			return "a=0\nb=0\nc=0\n"
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

func TestDroppedDatabaseLeavesNoRightOnAnyGrantee(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, false, func(string) string { return "" })
	if !resp.Applied || !resp.Kept || resp.RightsLeft || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	if strings.Contains(strings.Join(got, "\n"), "DROP USER") {
		t.Fatal("an account with a host its owner made was dropped")
	}
	// Exactly these statements: no account filter beyond the rows read
	// (every grantee, roles too), never while the database exists.
	if deletes := deleteStatements(got); strings.Join(deletes, "\n") != strings.Join(hand5Deletes, "\n") {
		t.Fatalf("removal statements:\n%s\nwant:\n%s", strings.Join(deletes, "\n"), strings.Join(hand5Deletes, "\n"))
	}
	// alpha.134 review: whose rights went is the step's answer too.
	if strings.Join(resp.RightsRemovedFrom, ",") != "other_owner@localhost,pastane_dr_test_hand5_user@10.%,reporting" {
		t.Fatalf("rights removed from %v", resp.RightsRemovedFrom)
	}
	var loaded string
	flushed := false
	for _, statement := range got {
		if strings.HasPrefix(statement, "SELECT CONCAT('a='") {
			loaded = statement
		}
		flushed = flushed || statement == "FLUSH PRIVILEGES;"
	}
	for _, want := range []string{
		"BINARY TABLE_SCHEMA = " + hand5ExactHex,
		"BINARY TABLE_SCHEMA = " + hand5Hex,
		"information_schema.TABLE_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + hand5Hex,
		"information_schema.COLUMN_PRIVILEGES WHERE BINARY TABLE_SCHEMA = " + hand5Hex,
		"FROM mysql.procs_priv WHERE Db = " + hand5Hex,
		"FROM mysql.columns_priv WHERE Db = " + hand5Hex,
		"FROM mysql.db WHERE Db IN (" + hand5Hex + ", " + hand5ExactHex + ")",
	} {
		if !strings.Contains(loaded, want) {
			t.Fatalf("the loaded-rights check lacks %q: %s", want, loaded)
		}
	}
	if !flushed {
		t.Fatal("no FLUSH PRIVILEGES")
	}
}

// A shared user (another database uses it): rights only - no DROP USER, no
// check of other rights; the same removal from every grantee.
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
	if len(resp.RightsRemovedFrom) != 3 {
		t.Fatalf("rights removed from %v", resp.RightsRemovedFrom)
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
		if strings.HasPrefix(statement, "SELECT CONCAT('a='") {
			return "a=1\nb=0\nc=0\n"
		}
		return ""
	})
	if resp.Applied || !resp.RightsLeft || resp.Error == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

// The Agent's own step after a deletion: every grantee, answered by name
// (a role by its name alone); a re-created name changes nothing.
func TestRemoveDroppedDatabaseRightsNamesEveryGrantee(t *testing.T) {
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('l='"):
			return hand5Read
		case strings.HasPrefix(statement, "SELECT CONCAT('a='"):
			return "a=0\nb=0\nc=0\n"
		}
		return "n=0\n"
	})
	var resp transport.DroppedDatabaseRightsResponse
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Database: hand5}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || strings.Join(resp.Grantees, ",") != "other_owner@localhost,pastane_dr_test_hand5_user@10.%,reporting" {
		t.Fatalf("resp = %+v", resp)
	}
	if deletes := deleteStatements(*got); strings.Join(deletes, "\n") != strings.Join(hand5Deletes, "\n") {
		t.Fatalf("removal statements %v", deletes)
	}

	exists := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA") {
			return "n=1\n"
		}
		return "n=0\n"
	})
	resp = transport.DroppedDatabaseRightsResponse{}
	_ = (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Database: hand5}, &resp)
	if resp.Applied || !resp.DatabaseExists || deleteStatements(*exists) != nil {
		t.Fatalf("a re-created name: resp = %+v statements %v", resp, *exists)
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
		"BINARY TABLE_SCHEMA NOT IN (X'73686F705F61', X'73686F705C5F61')",
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
// there, compared exactly; nothing is deleted from the grant tables. When
// the account is kept for other rights, CelikPanel's grant on this database
// goes from its own hosts in each form the host holds it (alpha.134: the
// exact form `shop\_a`.* written since alpha.134, the pattern form
// `shop_a`.* written before); hosts the owner made keep theirs. When the
// account has no other rights it is dropped with everything.
func TestRemovingAUserFromALiveDatabaseTouchesOnlyCelikPanelsHosts(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.%\nhost=localhost\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('f=', HEX(Db))") && strings.Contains(statement, "Host = X'6C6F63616C686F7374'"):
			return "f=73686F70\n"
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

	// Both forms on localhost (an exact grant and one written before
	// alpha.134): both go, the exact one by its escaped name.
	both := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.%\nhost=localhost\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('f=', HEX(Db))"):
			return "f=73686F705F61\nf=73686F705C5F61\n"
		}
		return "n=0\n"
	})
	resp = transport.DatabaseAccessUserResponse{}
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop_a", Username: "shop_a_user"}, &resp)
	all = strings.Join(*both, "\n")
	if !resp.Applied || !resp.Kept ||
		!strings.Contains(all, "REVOKE ALL PRIVILEGES ON `shop_a`.* FROM 'shop_a_user'@'localhost';") ||
		!strings.Contains(all, "REVOKE ALL PRIVILEGES ON `shop\\_a`.* FROM 'shop_a_user'@'localhost';") ||
		strings.Contains(all, "@'10.%'") {
		t.Fatalf("resp = %+v statements %v", resp, *both)
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
