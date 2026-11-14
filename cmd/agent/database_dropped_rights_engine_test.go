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
	cleanup := "DROP DATABASE IF EXISTS a134drop_x; DROP DATABASE IF EXISTS a134dropZx; DROP USER IF EXISTS 'a134hand'@'localhost', 'a134wide'@'localhost', ''@'%';" +
		" DROP ROLE IF EXISTS a134role;"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE a134drop_x; CREATE TABLE a134drop_x.t (i INT);"+
		" CREATE USER 'a134hand'@'localhost' IDENTIFIED BY '"+droppedRightsPassword+"'; CREATE ROLE a134role;"+
		" GRANT SELECT ON `a134drop_x`.* TO 'a134hand'@'localhost'; GRANT SELECT ON `a134drop\\_x`.* TO a134role;"+
		" GRANT SELECT ON a134drop_x.t TO 'a134hand'@'localhost';"+
		" GRANT SELECT ON `a134drop\\_%`.* TO 'a134hand'@'localhost';"+
		// review: the anonymous account's row is named ''@%, never a role
		// named after its host; a raw-form row that also reaches the existing
		// a134dropZx is kept and reported.
		" CREATE USER ''@'%'; GRANT SELECT ON `a134drop\\_x`.* TO ''@'%';"+
		" CREATE DATABASE a134dropZx; CREATE USER 'a134wide'@'localhost' IDENTIFIED BY '"+droppedRightsPassword+"';"+
		" GRANT SELECT ON `a134drop_x`.* TO 'a134wide'@'localhost';"+
		" DROP DATABASE a134drop_x;"))
	var resp transport.DroppedDatabaseRightsResponse
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "a134drop_x"}, &resp); err != nil {
		t.Fatal(err)
	}
	// a134hand's and a134wide's raw-form rows also match a134dropZx: kept.
	// a134hand still loses its table right.
	if !resp.Applied || strings.Join(resp.Grantees, ",") != "''@%,a134hand@localhost,a134role" {
		t.Fatalf("resp %+v", resp)
	}
	kept := map[string]string{}
	for _, row := range resp.Kept {
		kept[row.Account] = strings.Join(row.Reaches, ",")
	}
	if kept["a134wide@localhost"] != "a134dropZx" || kept["a134hand@localhost"] != "a134dropZx" || len(kept) != 2 {
		t.Fatalf("kept %+v", resp.Kept)
	}
	left := mustRun(t, "left", e.maria("", "", "SELECT CONCAT('n=', (SELECT COUNT(*) FROM mysql.db WHERE Db = CONCAT('a134drop', CHAR(92), '_x')) + (SELECT COUNT(*) FROM mysql.tables_priv WHERE Db = 'a134drop_x'));"))
	if left != "n=0" {
		t.Fatalf("rights left: %s", left)
	}
	pattern := mustRun(t, "pattern", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 'a134hand' AND Db = CONCAT('a134drop', CHAR(92), '_%');"))
	if pattern != "n=1" {
		t.Fatalf("the pattern that only matches the name was touched: %s", pattern)
	}
	// The accounts and the role stay.
	if n := mustRun(t, "accounts", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User IN ('a134hand', 'a134role', 'a134wide') OR (User = '' AND Host = '%');")); n != "n=4" {
		t.Fatalf("accounts: %s", n)
	}
}

// Second review (S1): deleting a database takes the pattern-form right
// CelikPanel gave its own user away even where that pattern also reaches
// another customer's database (s5__x reaches s51_x) - as alpha.133 did. Only
// an account that is not on record for the deleted database keeps such a row
// (it is listed).
func TestDroppedDatabaseOwnPatternRightGoesEvenWhereItReachesAnotherOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	cleanup := "DROP DATABASE IF EXISTS s5__x; DROP DATABASE IF EXISTS s51_x; DROP USER IF EXISTS 's5_u'@'localhost', 'a134hand5'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE s5__x; CREATE DATABASE s51_x;"+
		" CREATE USER 's5_u'@'localhost' IDENTIFIED BY '"+droppedRightsPassword+"'; CREATE USER 'a134hand5'@'localhost' IDENTIFIED BY '"+droppedRightsPassword+"';"+
		// As alpha.133 wrote it (pattern form), conversion not yet run.
		" GRANT SELECT ON `s5__x`.* TO 's5_u'@'localhost'; GRANT SELECT ON `s5__x`.* TO 'a134hand5'@'localhost';"+
		" DROP DATABASE s5__x;"))
	// The shared user's step (it stays; its rights on s5__x go).
	resp := accessUser(t, false, transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "s5__x", Username: "s5_u", DatabaseDropped: true, RightsOnly: true})
	if !resp.Applied {
		t.Fatalf("shared user step: %+v", resp)
	}
	own := "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 's5_u';"
	if n := mustRun(t, "own", e.maria("", "", own)); n != "n=0" {
		t.Fatalf("CelikPanel's own pattern right on the deleted database survived (it reaches s51_x): %s", n)
	}
	if len(resp.RightsKept) != 1 || resp.RightsKept[0].Account != "a134hand5@localhost" {
		t.Fatalf("kept: %+v", resp.RightsKept)
	}
	// The final sweep with the owners on record keeps the hand-made row only.
	var swept transport.DroppedDatabaseRightsResponse
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "s5__x", Owners: []string{"s5_u"}}, &swept); err != nil {
		t.Fatal(err)
	}
	if !swept.Applied || len(swept.Kept) != 1 || swept.Kept[0].Account != "a134hand5@localhost" {
		t.Fatalf("sweep: %+v", swept)
	}
	if n := mustRun(t, "hand", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = 'a134hand5' AND Db = 's5__x';")); n != "n=1" {
		t.Fatalf("the hand-made row that reaches s51_x: %s", n)
	}
}
