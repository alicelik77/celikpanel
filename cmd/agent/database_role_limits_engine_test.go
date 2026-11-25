//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 fix round B on a real PostgreSQL (CELIKPANEL_TEST_ENGINES, the
// private engines of database_access_users_engine_test.go; skipped without
// them): the lab's D10 as PostgreSQL 16 and later have it - the panel's own
// CREATEROLE account cannot change a role the superuser made - and the
// Agent's statement, run as the superuser, writing a site role's limits and
// refusing every privileged role in the same transaction.
func TestPGRoleLimitsOnARealEngine(t *testing.T) {
	e := requireTestEngines(t)
	if e.pgPort == "" {
		t.Skip("CELIKPANEL_TEST_PG_PORT is not set")
	}
	roles := []string{"rl_site", "rl_super", "rl_creator", "rl_repl", "rl_bypass", "rl_member", "rl_admin"}
	cleanup := ""
	for _, role := range roles {
		cleanup += "DROP ROLE IF EXISTS " + role + ";\n"
	}
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "roles", e.pg("", "", "", "CREATE ROLE rl_site LOGIN PASSWORD '"+engineOwnerPassword+"';\n"+
		"CREATE ROLE rl_super SUPERUSER;\nCREATE ROLE rl_creator CREATEROLE;\nCREATE ROLE rl_repl REPLICATION;\n"+
		"CREATE ROLE rl_bypass BYPASSRLS;\nCREATE ROLE rl_member;\nGRANT rl_creator TO rl_member;\n"+
		"CREATE ROLE rl_admin LOGIN CREATEDB CREATEROLE PASSWORD '"+engineOwnerPassword+"';\n"+
		"ALTER ROLE rl_site IN DATABASE postgres SET work_mem = '4MB';"))

	// The lab's D10: the panel's account (CREATEROLE, not a superuser) may not
	// change the role the superuser made, on PostgreSQL 16 and later.
	version := mustRun(t, "version", e.pg("", "", "", "SHOW server_version_num;"))
	denied := e.pg("rl_admin", engineOwnerPassword, "postgres", `ALTER ROLE rl_site SET statement_timeout = '1s';`)
	if atoiOrZero(version) >= 160000 && (denied.err == nil || !strings.Contains(string(denied.out), "permission denied")) {
		t.Fatalf("PostgreSQL %s let a CREATEROLE account change another's role: %s", version, denied.out)
	}

	writes := []transport.PostgreSQLRoleLimitWrite{
		{Key: transport.PGRoleLimitConnection, Value: 12},
		{Key: transport.PGRoleLimitStatementSeconds, Value: 5},
		{Key: transport.PGRoleLimitTempFileMB, Value: 64},
	}
	script, err := pgRoleLimitsScript("rl_site", writes)
	if err != nil {
		t.Fatal(err)
	}
	out := []byte(mustRun(t, "the site role's limits", e.pg("", "", "", script)))
	if conn, _ := markerValue(out, "conn"); conn != "12" {
		t.Fatalf("connection limit %q:\n%s", conn, out)
	}
	if v, _ := markerValue(out, transport.PGRoleLimitStatementSeconds); v != "5s" {
		t.Fatalf("statement_timeout %q:\n%s", v, out)
	}
	if v, _ := markerValue(out, transport.PGRoleLimitTempFileMB); v != "64MB" {
		t.Fatalf("temp_file_limit %q:\n%s", v, out)
	}
	if v, ok := markerValue(out, transport.PGRoleLimitIdleInTransactionSeconds); !ok || v != "" {
		t.Fatalf("idle_in_transaction_session_timeout %q:\n%s", v, out)
	}
	// The role's own session has the limits; its setting for one database is
	// the owner's and stays.
	if got := mustRun(t, "the role's session", e.pg("rl_site", engineOwnerPassword, "postgres", "SHOW temp_file_limit;")); got != "64MB" {
		t.Fatalf("the role's session has temp_file_limit %q", got)
	}
	if got := mustRun(t, "the owner's setting", e.pg("", "", "", "SELECT array_to_string(s.setconfig, ',') FROM pg_db_role_setting s JOIN pg_roles r ON r.oid = s.setrole WHERE r.rolname = 'rl_site' AND s.setdatabase <> 0;")); got != "work_mem=4MB" {
		t.Fatalf("the owner's setting for one database: %q", got)
	}
	// Removing: the connection limit back to none, the settings reset.
	script, _ = pgRoleLimitsScript("rl_site", []transport.PostgreSQLRoleLimitWrite{
		{Key: transport.PGRoleLimitConnection, Value: 0}, {Key: transport.PGRoleLimitStatementSeconds, Value: 0},
	})
	out = []byte(mustRun(t, "removal", e.pg("", "", "", script)))
	if conn, _ := markerValue(out, "conn"); conn != "-1" {
		t.Fatalf("connection limit after removal %q", conn)
	}
	if v, _ := markerValue(out, transport.PGRoleLimitStatementSeconds); v != "" {
		t.Fatalf("statement_timeout after removal %q", v)
	}

	// Every privileged role is refused, in the same transaction: nothing changes.
	for _, role := range []string{"rl_super", "rl_creator", "rl_repl", "rl_bypass", "rl_member", "postgres"} {
		script, _ := pgRoleLimitsScript(role, writes)
		r := e.pg("", "", "", script)
		if r.err == nil || pgRoleLimitsRefusalIn(r.out) != transport.PGRoleLimitRefusedPrivileged {
			t.Errorf("%s was not refused: %v %s", role, r.err, r.out)
		}
		if got := mustRun(t, "unchanged", e.pg("", "", "", "SELECT rolconnlimit FROM pg_roles WHERE rolname = '"+role+"';")); got != "-1" {
			t.Errorf("%s was changed: connection limit %s", role, got)
		}
	}
	script, _ = pgRoleLimitsScript("rl_absent", writes)
	if r := e.pg("", "", "", script); r.err == nil || pgRoleLimitsRefusalIn(r.out) != transport.PGRoleLimitRefusedAbsent {
		t.Fatalf("an absent role: %v %s", r.err, r.out)
	}
}
