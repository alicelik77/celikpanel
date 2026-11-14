package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.134 on a real MariaDB (the private engine of
// database_access_users_engine_test.go; skipped without one). The two
// customers of the brief: subscription 5 asked for "_x" (s5__x), subscription
// 51 for "x" (s51_x). The grant alpha.133 wrote for s5__x's user is a pattern
// that also covers s51_x; the Agent's rewrite ends that without the user
// losing its own database at any point, also when it is cut off between the
// steps, and under NO_BACKSLASH_ESCAPES.

const crossPassword = "Cross-Passw0rd!12"

func TestCrossCustomerRightsEndOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	cleanup := "SET GLOBAL sql_mode = 'STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION';" +
		" DROP DATABASE IF EXISTS s5__x; DROP DATABASE IF EXISTS s51_x; DROP DATABASE IF EXISTS s7_shop;" +
		" DROP USER IF EXISTS 's5__x_user'@'localhost', 's7_shop_user'@'localhost', 's7_shop_ro'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	// As alpha.133 left it: the pattern form.
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE s5__x; CREATE DATABASE s51_x;"+
		" CREATE USER 's5__x_user'@'localhost' IDENTIFIED BY '"+crossPassword+"';"+
		" GRANT ALL PRIVILEGES ON `s5__x`.* TO 's5__x_user'@'localhost';"))
	as := func(statement string) engineResult { return e.maria("s5__x_user", crossPassword, statement) }
	mustRun(t, "the defect, before", as("CREATE TABLE s51_x.before_t (i INT);"))

	var scan transport.DatabaseGrantFormsScanResponse
	if err := (&Agent{}).DatabaseGrantFormsScanV1(&transport.DatabaseGrantFormsScanRequest{}, &scan); err != nil || scan.Error != "" {
		t.Fatalf("scan: %v %s", err, scan.Error)
	}
	var row *mariadbgrant.Row
	for i := range scan.Scan.Patterns {
		if scan.Scan.Patterns[i].User == "s5__x_user" {
			row = &scan.Scan.Patterns[i]
		}
	}
	if !scan.Scan.MariaDB || row == nil || row.Stored != "s5__x" || row.Privileges != "ALL PRIVILEGES" || !strings.Contains(strings.Join(row.Reached, ","), "s51_x") {
		t.Fatalf("scan found %+v (row %+v)", scan.Scan, row)
	}

	convert := func() transport.DatabaseGrantFormConvertResponse {
		var resp transport.DatabaseGrantFormConvertResponse
		if err := (&Agent{}).DatabaseGrantFormConvertV1(&transport.DatabaseGrantFormConvertRequest{
			ConvertRequest: mariadbgrant.ConvertRequest{User: row.User, Host: row.Host, Database: "s5__x", Flags: row.Flags}}, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	// Cut off before the grant, then between the grant and the revoke: the
	// user keeps its own database each time.
	for _, step := range []string{"grant", "revoke"} {
		databaseGrantFormFault = func(at string) error {
			if at == step {
				return errors.New("cut off")
			}
			return nil
		}
		if resp := convert(); resp.Error == "" {
			t.Fatalf("cut off at %s was not reported: %+v", step, resp)
		}
		mustRun(t, "own database after a cut at "+step, as("CREATE TABLE s5__x.t_"+step+" (i INT);"))
	}
	databaseGrantFormFault = nil
	t.Cleanup(func() { databaseGrantFormFault = nil })
	if resp := convert(); resp.Error != "" || resp.Outcome != mariadbgrant.Converted {
		t.Fatalf("convert: %+v", resp)
	}
	if resp := convert(); resp.Outcome != mariadbgrant.AlreadyExact {
		t.Fatalf("again: %+v", resp)
	}
	mustRun(t, "own database after", as("CREATE TABLE s5__x.after_t (i INT);"))
	mustFail(t, "the other customer's database after", as("CREATE TABLE s51_x.after_t (i INT);"))
	grants := mustRun(t, "grants", e.maria("", "", "SHOW GRANTS FOR 's5__x_user'@'localhost';"))
	if !strings.Contains(grants, "s5\\\\_\\\\_x") && !strings.Contains(grants, "s5\\_\\_x") {
		t.Fatalf("no exact grant: %s", grants)
	}

	// NO_BACKSLASH_ESCAPES: a new user is given its database exactly, the
	// same way; the exact grant still reaches only its own database.
	mustRun(t, "mode", e.maria("", "", "SET GLOBAL sql_mode = CONCAT(@@GLOBAL.sql_mode, ',NO_BACKSLASH_ESCAPES'); CREATE DATABASE s7_shop;"))
	resp := accessUser(t, true, transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "s7_shop", Username: "s7_shop_ro",
		Password: engineReadPassword, Access: transport.DatabaseAccessWrite})
	if !resp.Applied {
		t.Fatalf("create under NO_BACKSLASH_ESCAPES: %+v", resp)
	}
	ro := func(statement string) engineResult { return e.maria("s7_shop_ro", engineReadPassword, statement) }
	mustRun(t, "own database under NO_BACKSLASH_ESCAPES", ro("CREATE TABLE s7_shop.t (i INT);"))
	mustRun(t, "a look-alike", e.maria("", "", "CREATE DATABASE IF NOT EXISTS s71shop;"))
	t.Cleanup(func() { _ = e.maria("", "", "DROP DATABASE IF EXISTS s71shop;") })
	mustFail(t, "a look-alike database under NO_BACKSLASH_ESCAPES", ro("CREATE TABLE s71shop.t (i INT);"))
	// Removing it from the live database takes the exact grant away.
	resp = accessUser(t, false, transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "s7_shop", Username: "s7_shop_ro"})
	if !resp.Applied {
		t.Fatalf("remove under NO_BACKSLASH_ESCAPES: %+v", resp)
	}
}

// Measured on MariaDB 11.8.6 (alpha.134 review): the exact form of a name of
// 60 characters with six `_` is 66 characters; GRANT accepts it and
// mysql.db.Db keeps it cut to 64 - a right on another, shorter name. Such a
// database is never made, and a user is never given an existing one by its
// exact form; every GRANT is read back. (CelikPanel's own names are at most
// 63 characters.)
func TestTooLongExactNamesAreRefusedOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	long := "abc_efg_ijk_mno_qrs_uvw_yzabcdefghijklmnopqrstuvwxyzabcdefgh"
	cleanup := "DROP DATABASE IF EXISTS " + long + "; DROP USER IF EXISTS 'a134long_u'@'localhost', 'a134long_r'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "mysql", Name: long, User: "a134long_u", Password: crossPassword})
	if resp.Success || !resp.NameTooLong {
		t.Fatalf("created: %+v", resp)
	}
	if n := mustRun(t, "made", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '"+long+"'; ")); n != "n=0" {
		t.Fatalf("the database was made: %s", n)
	}
	// An existing database of such a name (made outside CelikPanel).
	mustRun(t, "outside", e.maria("", "", "CREATE DATABASE "+long+";"))
	add := accessUser(t, true, transport.DatabaseAccessUserRequest{Engine: "mysql", Database: long, Username: "a134long_r",
		Password: engineReadPassword, Access: transport.DatabaseAccessRead})
	if add.Applied || !add.NameTooLong {
		t.Fatalf("added: %+v", add)
	}
	if n := mustRun(t, "account", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 'a134long_r';")); n != "n=0" {
		t.Fatalf("an account was made: %s", n)
	}
	// Every grant CelikPanel writes carries NO_AUTO_CREATE_USER in its
	// session: a GRANT to an account that is gone is refused, never made
	// with an empty password.
	out := e.maria("", "", "SET SESSION sql_mode = 'STRICT_TRANS_TABLES'; "+mariadbgrant.NoAutoCreateUser+" GRANT SELECT ON `nosuchdb`.* TO 'a134ghost'@'localhost';")
	if out.err == nil {
		t.Fatalf("a GRANT re-created a missing account: %s", out.out)
	}
	if n := mustRun(t, "ghost", e.maria("", "", "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = 'a134ghost';")); n != "n=0" {
		_ = e.maria("", "", "DROP USER IF EXISTS 'a134ghost'@'localhost';")
		t.Fatalf("the account was made: %s", n)
	}
}
