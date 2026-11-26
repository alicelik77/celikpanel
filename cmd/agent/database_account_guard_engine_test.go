//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 security fix on real engines (CELIKPANEL_TEST_ENGINES, the private
// MariaDB and PostgreSQL of database_access_users_engine_test.go; skipped
// without them - a lab step): the shared account rule's read and the guard in
// the write's own transaction (PostgreSQL) and compound statement (MariaDB)
// refuse every administrator account and serve a site account.
func TestTheAccountGuardOnRealEngines(t *testing.T) {
	e := requireTestEngines(t)
	if e.pgPort != "" {
		roles := []string{"ag_site", "ag_db", "ag_grp", "ag_inside", "ag_creator", "ag_mid", "ag_chain", "ag_monitor"}
		cleanup := ""
		for _, role := range roles {
			cleanup += "DROP ROLE IF EXISTS " + role + ";\n"
		}
		mustRun(t, "clean", e.pg("", "", "", cleanup))
		t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
		// ag_chain is a member of ag_creator through ag_mid (a chain).
		mustRun(t, "roles", e.pg("", "", "", "CREATE ROLE ag_site LOGIN;\nCREATE ROLE ag_db LOGIN CREATEDB;\nCREATE ROLE ag_grp;\n"+
			"CREATE ROLE ag_inside LOGIN;\nGRANT ag_grp TO ag_inside;\nCREATE ROLE ag_creator CREATEROLE;\nCREATE ROLE ag_mid;\n"+
			"GRANT ag_creator TO ag_mid;\nCREATE ROLE ag_chain LOGIN;\nGRANT ag_mid TO ag_chain;\n"+
			"CREATE ROLE ag_monitor LOGIN;\nGRANT pg_monitor TO ag_monitor;"))
		run := func(statement string) ([]byte, error) { r := e.pg("", "", "", statement); return r.out, r.err }
		for role, want := range map[string]string{
			"ag_site": "", "ag_inside": "", "ag_db": "privileged", "ag_chain": "privileged", "ag_monitor": "privileged",
			"postgres": "privileged", "ag_absent": "absent",
		} {
			if got := databaseAccountRefusalOf(readDatabaseAccountState("postgresql", run, role, pgAccountOrdinary, false)); got != want {
				t.Errorf("PostgreSQL %s: %q, want %q", role, got, want)
			}
			statement, _ := pgGuarded(role, pgAccountOrdinary, "ALTER ROLE "+role+" CONNECTION LIMIT 3;")
			out, err := run(statement)
			if want == "" && err != nil {
				t.Errorf("PostgreSQL %s was refused: %s", role, out)
			}
			if want != "" && (err == nil || databaseAccountRefusalIn(out) != want) {
				t.Errorf("PostgreSQL %s was not refused in the transaction: %v %s", role, err, out)
			}
		}
		if got := databaseAccountRefusalOf(readDatabaseAccountState("postgresql", run, "ag_inside", pgAccountNoMembership, false)); got != transport.DatabaseAccountRefusedMember {
			t.Errorf("the limits rule took a member of a role: %q", got)
		}
		if got := mustRun(t, "unchanged", e.pg("", "", "", "SELECT rolconnlimit FROM pg_roles WHERE rolname = 'ag_db';")); got != "-1" {
			t.Errorf("a refused role was changed: %s", got)
		}
		// Second review, finding 1: a database's owner. The shared rule for
		// any owner; CelikPanel's own account (here ag_admin, LOGIN CREATEDB
		// CREATEROLE as celikpanel_admin is) passes until it is a superuser or
		// a member of a pg_* role.
		if got := databaseAccountRefusalOf(readDatabaseOwnerState(run, "postgres")); got != "privileged" {
			t.Errorf("a superuser owner: %q", got)
		}
		if got := databaseAccountRefusalOf(readDatabaseOwnerState(run, "ag_site")); got != "" {
			t.Errorf("a site role owner: %q", got)
		}
		adminCleanup := "DROP ROLE IF EXISTS ag_admin;"
		mustRun(t, "admin", e.pg("", "", "", adminCleanup+"\nCREATE ROLE ag_admin LOGIN CREATEDB CREATEROLE;"))
		t.Cleanup(func() { _ = e.pg("", "", "", adminCleanup) })
		adminState := func() string {
			return databaseAccountRefusalOf(readGuardState(run, "SELECT 'guard=' || "+pgDatabaseOwnerStateExpr("'ag_admin'", true)+";", "ag_admin"))
		}
		guarded := func() error {
			_, err := run("BEGIN;\n" + pgGuardBlockOf(pgDatabaseOwnerStateExpr("'ag_admin'", true), "'ag_admin'") + "\nCOMMIT;")
			return err
		}
		if got := adminState(); got != "" || guarded() != nil {
			t.Errorf("the panel's own account as it is made: %q", got)
		}
		mustRun(t, "admin member", e.pg("", "", "", "GRANT pg_read_server_files TO ag_admin;"))
		if got := adminState(); got != "privileged" || guarded() == nil {
			t.Errorf("the panel's own account in pg_read_server_files: %q", got)
		}
		mustRun(t, "admin superuser", e.pg("", "", "", "REVOKE pg_read_server_files FROM ag_admin;\nALTER ROLE ag_admin SUPERUSER;"))
		if got := adminState(); got != "privileged" || guarded() == nil {
			t.Errorf("the panel's own account as a superuser: %q", got)
		}
		// Third review, I2: its own REPLICATION or BYPASSRLS fails it too.
		for _, attribute := range []string{"REPLICATION", "BYPASSRLS"} {
			mustRun(t, "admin "+attribute, e.pg("", "", "", "ALTER ROLE ag_admin NOSUPERUSER NOREPLICATION NOBYPASSRLS;\nALTER ROLE ag_admin "+attribute+";"))
			if got := adminState(); got != "privileged" || guarded() == nil {
				t.Errorf("the panel's own account with %s: %q", attribute, got)
			}
		}
	}

	if e.pgPort != "" {
		nothingGoesToCelikPanelsOwnAccount(t, e)
	}

	accounts := []string{"ag_site", "ag_global", "ag_grant", "ag_role", "ag_mysqldb"}
	cleanup := ""
	for _, account := range accounts {
		cleanup += "DROP USER IF EXISTS '" + account + "'@'localhost';\n"
	}
	cleanup += "DROP ROLE IF EXISTS ag_r;\nDROP DATABASE IF EXISTS ag_shop;\n"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "accounts", e.maria("", "", "CREATE DATABASE ag_shop;\n"+
		"CREATE USER 'ag_site'@'localhost' IDENTIFIED BY 'x1-Long-enough';\nGRANT ALL PRIVILEGES ON `ag\\_shop`.* TO 'ag_site'@'localhost';\n"+
		"CREATE USER 'ag_global'@'localhost';\nGRANT PROCESS ON *.* TO 'ag_global'@'localhost';\n"+
		"CREATE USER 'ag_grant'@'localhost';\nGRANT USAGE ON *.* TO 'ag_grant'@'localhost' WITH GRANT OPTION;\n"+
		"CREATE ROLE ag_r;\nCREATE USER 'ag_role'@'localhost';\nGRANT ag_r TO 'ag_role'@'localhost';\n"+
		"CREATE USER 'ag_mysqldb'@'localhost';\nGRANT SELECT ON mysql.* TO 'ag_mysqldb'@'localhost';"))
	run := func(statement string) ([]byte, error) { r := e.maria("", "", statement); return r.out, r.err }
	for account, want := range map[string]string{
		"ag_site": "", "ag_global": "privileged", "ag_grant": "privileged", "ag_role": "privileged", "ag_mysqldb": "privileged",
		"root": "privileged", "ag_absent": "absent",
	} {
		if got := databaseAccountRefusalOf(readDatabaseAccountState("mysql", run, account, pgAccountOrdinary, false)); got != want {
			t.Errorf("MariaDB %s: %q, want %q", account, got, want)
		}
		statement, _ := mysqlGuarded(account, false, "ALTER USER '"+account+"'@'localhost' WITH MAX_USER_CONNECTIONS 3;")
		out, err := run(statement)
		if want == "" && err != nil {
			t.Errorf("MariaDB %s was refused: %s", account, out)
		}
		if want != "" && (err == nil || databaseAccountRefusalIn(out) != want) {
			t.Errorf("MariaDB %s was not refused in the compound statement: %v %s", account, err, out)
		}
	}
	if got := mustRun(t, "unchanged", e.maria("", "", "SELECT max_user_connections FROM mysql.user WHERE User = 'ag_global';")); strings.TrimSpace(got) != "0" {
		t.Errorf("a refused account was changed: %s", got)
	}
}

// nothingGoesToCelikPanelsOwnAccount is the third review's H1 probe on a real
// PostgreSQL: a database owned by celikpanel_admin (as releases before the
// Agent-side create made them), a read-and-write user added to it that makes
// a SECURITY DEFINER function and a table with an index on an expression
// calling its own function. Removing that user is refused (owns_objects) and
// every object is still the user's; after the administrator's REASSIGN to the
// database's own user the removal goes through, as it does for a user that
// owns nothing.
func nothingGoesToCelikPanelsOwnAccount(t *testing.T, e testEngines) {
	t.Helper()
	installTestEngines(t, e)
	cleanup := "DROP DATABASE IF EXISTS h1shop;\nDROP ROLE IF EXISTS h1shop_maker;\nDROP ROLE IF EXISTS h1shop_idle;\nDROP ROLE IF EXISTS h1shop_user;\nDROP ROLE IF EXISTS celikpanel_admin;"
	mustRun(t, "h1 clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "h1 roles", e.pg("", "", "", "CREATE ROLE celikpanel_admin LOGIN CREATEDB CREATEROLE;\nCREATE ROLE h1shop_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "h1 database", e.pg("", "", "", "CREATE DATABASE h1shop OWNER celikpanel_admin;"))
	base := transport.DatabaseAccessUserRequest{Engine: "postgresql", Port: 5432, Database: "h1shop"}
	for _, user := range []string{"h1shop_maker", "h1shop_idle"} {
		req := base
		req.Username, req.Password, req.Access = user, engineWritePassword, transport.DatabaseAccessWrite
		if resp := accessUser(t, true, req); !resp.Applied {
			t.Fatalf("add %s: %+v", user, resp)
		}
	}
	mustRun(t, "h1 objects", e.pg("h1shop_maker", engineWritePassword, "h1shop",
		"CREATE FUNCTION public.h1_definer() RETURNS int LANGUAGE sql SECURITY DEFINER AS 'SELECT 1';\n"+
			"CREATE FUNCTION public.h1_key(int) RETURNS int LANGUAGE sql IMMUTABLE AS 'SELECT $1';\n"+
			"CREATE TABLE public.h1_items (v int);\nCREATE INDEX h1_items_key ON public.h1_items (public.h1_key(v));"))
	owners := func() string {
		return mustRun(t, "h1 owners", e.pg("", "", "h1shop", "SELECT string_agg(pg_catalog.pg_get_userbyid(o), ',') FROM ("+
			"SELECT proowner AS o FROM pg_catalog.pg_proc WHERE proname IN ('h1_definer', 'h1_key')"+
			" UNION ALL SELECT relowner FROM pg_catalog.pg_class WHERE relname IN ('h1_items', 'h1_items_key')) s;"))
	}
	roleCount := func(role string) string {
		return mustRun(t, "h1 role", e.pg("", "", "", "SELECT count(*) FROM pg_catalog.pg_roles WHERE rolname = '"+role+"';"))
	}
	req := base
	req.Username = "h1shop_maker"
	if resp := accessUser(t, false, req); resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedOwnsObjects || resp.AccountRefusedName != "h1shop_maker" {
		t.Fatalf("removing a user that owns objects in celikpanel_admin's database: %+v", resp)
	}
	if got := owners(); got != "h1shop_maker,h1shop_maker,h1shop_maker,h1shop_maker" || roleCount("h1shop_maker") != "1" {
		t.Fatalf("a refused removal changed something: owners %q", got)
	}
	// The administrator's command from the user guide, then the removal again.
	mustRun(t, "h1 hand over", e.pg("", "", "h1shop", `REASSIGN OWNED BY "h1shop_maker" TO "h1shop_user";`))
	if resp := accessUser(t, false, req); !resp.Applied || resp.AccountRefused != "" || roleCount("h1shop_maker") != "0" {
		t.Fatalf("removal after the administrator handed the objects over: %+v", resp)
	}
	if got := owners(); got != "h1shop_user,h1shop_user,h1shop_user,h1shop_user" {
		t.Fatalf("owners after the hand-over: %q", got)
	}
	// A user that owns nothing there (only its default privileges) is removed.
	req.Username = "h1shop_idle"
	if resp := accessUser(t, false, req); !resp.Applied || resp.AccountRefused != "" || roleCount("h1shop_idle") != "0" {
		t.Fatalf("removing a user that owns nothing: %+v", resp)
	}
	// celikpanel_admin received nothing.
	if got := mustRun(t, "h1 admin objects", e.pg("", "", "h1shop", "SELECT count(*) FROM pg_catalog.pg_shdepend d JOIN pg_catalog.pg_database db ON db.oid = d.dbid"+
		" WHERE db.datname = 'h1shop' AND d.deptype = 'o' AND d.refobjid = (SELECT oid FROM pg_catalog.pg_roles WHERE rolname = 'celikpanel_admin')"+
		" AND d.classid <> 'pg_catalog.pg_default_acl'::pg_catalog.regclass;")); got != "0" {
		t.Fatalf("celikpanel_admin owns %s objects in the database", got)
	}
}
