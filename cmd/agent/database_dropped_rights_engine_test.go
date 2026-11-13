package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 on a real MariaDB (the private engine of
// database_access_users_engine_test.go; skipped without one).

const droppedRightsPassword = "Dropped-Passw0rd!12"

// Deleting a database takes every right naming it from every grantee - a
// hand-made account of another name, a role - and leaves a pattern that only
// matches the name.
func TestDroppedDatabaseRightsGoFromEveryGranteeOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	cleanup := "DROP DATABASE IF EXISTS a134drop_x; DROP USER IF EXISTS 'a134hand'@'localhost'; DROP ROLE IF EXISTS a134role;"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE a134drop_x; CREATE TABLE a134drop_x.t (i INT);"+
		" CREATE USER 'a134hand'@'localhost' IDENTIFIED BY '"+droppedRightsPassword+"'; CREATE ROLE a134role;"+
		" GRANT SELECT ON `a134drop_x`.* TO 'a134hand'@'localhost'; GRANT SELECT ON `a134drop\\_x`.* TO a134role;"+
		" GRANT SELECT ON a134drop_x.t TO 'a134hand'@'localhost';"+
		" GRANT SELECT ON `a134drop\\_%`.* TO 'a134hand'@'localhost';"+
		" DROP DATABASE a134drop_x;"))
	var resp transport.DroppedDatabaseRightsResponse
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "a134drop_x"}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Applied || strings.Join(resp.Grantees, ",") != "a134hand@localhost,a134role" {
		t.Fatalf("resp %+v", resp)
	}
	left := mustRun(t, "left", e.maria("", "", "SELECT CONCAT('n=', (SELECT COUNT(*) FROM mysql.db WHERE Db IN ('a134drop_x', CONCAT('a134drop', CHAR(92), '_x'))) + (SELECT COUNT(*) FROM mysql.tables_priv WHERE Db = 'a134drop_x'));"))
	if left != "n=0" {
		t.Fatalf("rights left: %s", left)
	}
	pattern := mustRun(t, "pattern", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 'a134hand' AND Db = CONCAT('a134drop', CHAR(92), '_%');"))
	if pattern != "n=1" {
		t.Fatalf("the pattern that only matches the name was touched: %s", pattern)
	}
	// The accounts and the role stay.
	if n := mustRun(t, "accounts", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User IN ('a134hand', 'a134role');")); n != "n=2" {
		t.Fatalf("accounts: %s", n)
	}
}
