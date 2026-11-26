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
