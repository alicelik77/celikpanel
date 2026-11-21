//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
)

// S4 R2a on real engines (CELIKPANEL_TEST_ENGINES; skipped without them): a
// database user's time and temporary-file limits, written as the panel
// writes them, hold for the user's own sessions where the engine makes them
// a limit, and never cut the panel's own load of a file as that user (a
// restore here; a copy and an import go through the same loadDatabaseSQL).

// TestR2aPostgreSQLRoleLimitsDoNotCutARestore: statement_timeout and
// idle_in_transaction_session_timeout are defaults the role's session can
// change; temp_file_limit is a limit it cannot. A restore whose payload runs
// longer than the statement time and sorts more than the temporary-file
// limit is loaded, and the lift for its database is gone afterwards.
func TestR2aPostgreSQLRoleLimitsDoNotCutARestore(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS r2ap; DROP ROLE IF EXISTS r2ap_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "role", e.pg("", "", "", "CREATE ROLE r2ap_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE r2ap OWNER r2ap_user;"))
	mustRun(t, "public closed", e.pg("", "", "", "REVOKE ALL ON DATABASE r2ap FROM PUBLIC;"))
	// The statements the panel's driver writes (services.postgreSQLRoleLimitSQL).
	mustRun(t, "limits", e.pg("", "", "", `ALTER ROLE "r2ap_user" SET statement_timeout = '1s';`+
		` ALTER ROLE "r2ap_user" SET idle_in_transaction_session_timeout = '30s';`+
		` ALTER ROLE "r2ap_user" SET temp_file_limit = '1MB';`))
	mustRun(t, "no override yet", e.pg("", "", "", services.PostgreSQLResetLoadOverridesSQL("'r2ap_user'")+";"))

	sort := "SET work_mem = '64kB'; SELECT count(*) FROM (SELECT g FROM generate_series(1, 300000) g ORDER BY md5(g::text)) s;"
	// The role's own sessions: the statement time is a default it can lift,
	// the temporary-file limit holds and cannot be lifted.
	mustFail(t, "a statement longer than the role's statement time", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SELECT pg_sleep(1.5);"))
	mustRun(t, "the session lifts its statement time", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; SELECT pg_sleep(1.5);"))
	mustFail(t, "a sort above the temporary-file limit", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; "+sort))
	mustFail(t, "the session lifts the temporary-file limit", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET temp_file_limit = -1;"))

	database := backupspec.DatabaseIdentity{ID: 61, Name: "r2ap", Type: "postgresql", ClusterPort: atoiOrZero(e.pgPort)}
	account := databaseTestAccount{"r2ap_user", engineOwnerPassword}
	payload := "SELECT pg_sleep(1.5);\nCREATE TABLE public.big AS SELECT g, md5(g::text) AS m FROM generate_series(1, 300000) g;\n" +
		"SET maintenance_work_mem = '1MB';\nCREATE INDEX big_m ON public.big (m);\n"
	resp := s1RestorePackage(t, database, account, payload)
	if !resp.Success {
		t.Fatalf("the restore was cut by the user's limits: %+v", resp)
	}
	if got := mustRun(t, "rows", e.pg("", "", "r2ap", "SELECT count(*) FROM public.big;")); got != "300000" {
		t.Fatalf("restored rows = %s", got)
	}
	// The lift was for the load only.
	overrides := mustRun(t, "overrides", e.pg("", "", "", "SELECT count(*) FROM pg_db_role_setting s JOIN pg_roles r ON r.oid = s.setrole "+
		"WHERE r.rolname = 'r2ap_user' AND s.setdatabase <> 0;"))
	if overrides != "0" {
		t.Fatalf("the load left %s role-in-database settings", overrides)
	}
	own := mustRun(t, "own", e.pg("", "", "", "SELECT array_to_string(setconfig, ',') FROM pg_db_role_setting s JOIN pg_roles r ON r.oid = s.setrole "+
		"WHERE r.rolname = 'r2ap_user' AND s.setdatabase = 0;"))
	if !strings.Contains(own, "temp_file_limit=1MB") || !strings.Contains(own, "statement_timeout=1s") {
		t.Fatalf("the role's own limits changed: %s", own)
	}
	mustFail(t, "the limit holds again after the load", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; "+sort))
}

// TestR2aMariaDBStatementTimeDoesNotCutARestore: MAX_STATEMENT_TIME is the
// session's default (the session can lift it); a restore whose payload runs
// longer than it is loaded.
func TestR2aMariaDBStatementTimeDoesNotCutARestore(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS r2am; DROP USER IF EXISTS 'r2am_user'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE r2am; CREATE USER 'r2am_user'@'localhost' IDENTIFIED BY '"+engineOwnerPassword+"';"+
		" GRANT ALL PRIVILEGES ON `r2am`.* TO 'r2am_user'@'localhost';"))
	// The statement the panel's driver writes (services.mariaDBRoleLimitSQL).
	mustRun(t, "limit", e.maria("", "", "ALTER USER 'r2am_user'@'localhost' WITH MAX_STATEMENT_TIME 1;"))
	slow := "SELECT BENCHMARK(40000000, MD5('celikpanel'));"
	mustFail(t, "a statement longer than the user's statement time", e.maria("r2am_user", engineOwnerPassword, "USE r2am; "+slow))
	mustRun(t, "the session lifts its statement time", e.maria("r2am_user", engineOwnerPassword, "USE r2am; SET SESSION max_statement_time = 0; "+slow))

	database := backupspec.DatabaseIdentity{ID: 62, Name: "r2am", Type: "mysql"}
	account := databaseTestAccount{"r2am_user", engineOwnerPassword}
	resp := s1RestorePackage(t, database, account, "CREATE TABLE t (id INT);\nINSERT INTO t VALUES (1);\n"+slow+"\n")
	if !resp.Success {
		t.Fatalf("the restore was cut by the user's statement time: %+v", resp)
	}
	if got := mustRun(t, "rows", e.maria("", "", "SELECT count(*) FROM r2am.t;")); got != "1" {
		t.Fatalf("restored rows = %s", got)
	}
	mustFail(t, "the limit holds again for the user's own session", e.maria("r2am_user", engineOwnerPassword, "USE r2am; "+slow))
}
