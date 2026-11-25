//go:build linux

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// S4 R2a on real engines (CELIKPANEL_TEST_ENGINES; skipped without them): a
// database user's time and temporary-file limits, written as the panel
// writes them, hold for the user's own sessions where the engine makes them
// a limit, and never cut the panel's own load of a file as that user (a
// restore here; a copy and an import go through the same loadDatabaseSQL).

// TestR2aPostgreSQLTemporaryFileLimitHoldsDuringARestore (review findings
// 1-3): statement_timeout and idle_in_transaction_session_timeout are
// defaults the role's session can change, so a restore sets them to 0 for
// itself; temp_file_limit is a limit nothing lifts. A session the role opens
// while a restore runs has the role's own limit; a restore that needs more
// temporary space than the limit is rolled back whole and says which limit
// stopped it; the owner's own role settings (role-wide and for one database)
// are exactly as before.
func TestR2aPostgreSQLTemporaryFileLimitHoldsDuringARestore(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS r2ap; DROP ROLE IF EXISTS r2ap_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "role", e.pg("", "", "", "CREATE ROLE r2ap_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE r2ap OWNER r2ap_user;"))
	mustRun(t, "public closed", e.pg("", "", "", "REVOKE ALL ON DATABASE r2ap FROM PUBLIC;"))
	// The statements the panel's driver writes (services.postgreSQLRoleSettingSQL).
	mustRun(t, "limits", e.pg("", "", "", `ALTER ROLE "r2ap_user" SET statement_timeout = '1s';`+
		` ALTER ROLE "r2ap_user" SET idle_in_transaction_session_timeout = '30s';`+
		` ALTER ROLE "r2ap_user" SET temp_file_limit = '1MB';`))
	// A setting the server owner made by hand for this database.
	mustRun(t, "owner's own setting", e.pg("", "", "", `ALTER ROLE "r2ap_user" IN DATABASE "r2ap" SET work_mem = '4MB';`))
	settings := func() string {
		return mustRun(t, "settings", e.pg("", "", "", "SELECT string_agg(s.setdatabase::text || ':' || array_to_string(s.setconfig, ','), ' ' ORDER BY s.setdatabase) "+
			"FROM pg_db_role_setting s JOIN pg_roles r ON r.oid = s.setrole WHERE r.rolname = 'r2ap_user';"))
	}
	before := settings()

	sort := "SET work_mem = '64kB'; SELECT count(*) FROM (SELECT g FROM generate_series(1, 300000) g ORDER BY md5(g::text)) s;"
	mustFail(t, "a statement longer than the role's statement time", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SELECT pg_sleep(1.5);"))
	mustRun(t, "the session lifts its statement time", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; SELECT pg_sleep(1.5);"))
	mustFail(t, "a sort above the temporary-file limit", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; "+sort))
	mustFail(t, "the session lifts the temporary-file limit", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET temp_file_limit = -1;"))

	database := backupspec.DatabaseIdentity{ID: 61, Name: "r2ap", Type: "postgresql", ClusterPort: atoiOrZero(e.pgPort)}
	account := databaseTestAccount{"r2ap_user", engineOwnerPassword}
	// A restore longer than the statement time, during which the role opens
	// another session: that session has the role's temporary-file limit.
	during := make(chan string, 1)
	go func() {
		time.Sleep(700 * time.Millisecond)
		r := e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SHOW temp_file_limit;")
		out := string(r.out)
		if r.err != nil {
			out = "error: " + r.err.Error()
		}
		during <- strings.TrimSpace(out)
	}()
	resp := s1RestorePackage(t, database, account, "SELECT pg_sleep(2);\nCREATE TABLE public.small AS SELECT g FROM generate_series(1, 1000) g;\n")
	if !resp.Success {
		t.Fatalf("the restore was cut by the user's statement time: %+v", resp)
	}
	if got := <-during; got != "1MB" {
		t.Fatalf("a session opened during the restore has temp_file_limit %q, not the role's 1MB", got)
	}
	// A restore that needs more temporary space than the limit: rolled back,
	// the earlier content is still there, the answer names the limit.
	big := "CREATE TABLE public.big AS SELECT g, md5(g::text) AS m FROM generate_series(1, 300000) g;\n" +
		"SET maintenance_work_mem = '1MB';\nCREATE INDEX big_m ON public.big (m);\n"
	resp = s1RestorePackage(t, database, account, big)
	if resp.Success || !strings.Contains(resp.Error+" "+resp.Detail, "Temporary files of one database session") {
		t.Fatalf("a restore above the temporary-file limit: %+v", resp)
	}
	if got := mustRun(t, "rows", e.pg("", "", "r2ap", "SELECT count(*) FROM public.small;")); got != "1000" {
		t.Fatalf("the earlier content after the stopped restore: %s rows", got)
	}
	if after := settings(); after != before {
		t.Fatalf("the role's settings changed: before %q, after %q", before, after)
	}
	mustFail(t, "the limit holds after the loads", e.pg("r2ap_user", engineOwnerPassword, "r2ap", "SET statement_timeout = 0; "+sort))
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
	// The statement the panel's driver writes (services.mariaDBStatementTimeSQL).
	mustRun(t, "limit", e.maria("", "", "ALTER USER 'r2am_user'@'localhost' WITH MAX_STATEMENT_TIME 1;"))
	// SLEEP, not a CPU loop: its length does not depend on the machine.
	slow := "SELECT SLEEP(2);"
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
