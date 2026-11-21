//go:build linux

package main

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.143 fix round (independent review of S1b, 21 November 2026), on a
// real engine (the private cluster of CELIKPANEL_TEST_ENGINES and
// CELIKPANEL_TEST_PG_PORT; skipped without it). These tests use only what
// the Agent's calls had before the fix round, so they also run, and fail,
// against the reviewed code: a close whose answer is lost after COMMIT is
// answered as unknown; a role that holds only a column right, a
// per-database setting, a policy or a user mapping counts as inside the
// database; a role named to end a dollar-quoted body is carried safely.
// Every case ends with the undo.

func pgFixEngine(t *testing.T) (testEngines, int, func(role, database, statement string) engineResult) {
	t.Helper()
	e := requireTestEngines(t)
	if e.pgPort == "" {
		t.Skip("CELIKPANEL_TEST_PG_PORT is not set")
	}
	installTestEngines(t, e)
	port, _ := strconv.Atoi(e.pgPort)
	connects := func(role, database, statement string) engineResult {
		return e.pg(role, engineOwnerPassword, database, statement)
	}
	return e, port, connects
}

func pgFixSetup(t *testing.T, e testEngines, databases []string, roles []string, setup ...string) {
	t.Helper()
	var clean []string
	for _, database := range databases {
		clean = append(clean, "DROP DATABASE IF EXISTS "+database+" WITH (FORCE);")
	}
	for _, role := range roles {
		clean = append(clean, "DROP ROLE IF EXISTS "+role+";")
	}
	mustRun(t, "clean", e.pg("", "", "", strings.Join(clean, "\n")))
	t.Cleanup(func() { _ = e.pg("", "", "", strings.Join(clean, "\n")) })
	var create []string
	for _, role := range roles {
		create = append(create, "CREATE ROLE "+role+" LOGIN PASSWORD '"+engineOwnerPassword+"';")
	}
	mustRun(t, "roles", e.pg("", "", "", strings.Join(append(create, setup...), "\n")))
}

func pgFixScan(t *testing.T, port int, name string, recorded []string) transport.PostgreSQLPublicAccessFacts {
	t.Helper()
	var scan transport.PostgreSQLPublicAccessScanResponse
	if err := (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Port: port,
		Databases: []transport.PostgreSQLPublicAccessDatabase{{Name: name, Recorded: recorded}}}, &scan); err != nil || scan.Error != "" || len(scan.Databases) != 1 || scan.Databases[0].Error != "" {
		t.Fatalf("scan of %s: %v %+v", name, err, scan)
	}
	return scan.Databases[0]
}

func pgFixACL(t *testing.T, e testEngines, name string) string {
	t.Helper()
	return mustRun(t, "rights of "+name, e.pg("", "", "", "SELECT coalesce(datacl::text, '') FROM pg_database WHERE datname = '"+name+"';"))
}

func pgFixUndo(t *testing.T, e testEngines, port int, name string, added []string) {
	t.Helper()
	var undone transport.PostgreSQLPublicAccessUndoResponse
	if err := (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Port: port, Name: name, ExpectACL: pgFixACL(t, e, name),
		PublicConnect: true, PublicTemporary: true, Added: added}, &undone); err != nil || !undone.Undone {
		t.Fatalf("undo of %s: %v %+v", name, err, undone)
	}
}

// Finding 4: the close committed, and its answer was lost after COMMIT.
func TestS1bFixACloseWhoseAnswerIsLostAfterCommitIsUnknown(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	pgFixSetup(t, e, []string{"s1f_lost"}, []string{"s1f_owner", "s1f_reader", "s1f_stranger"}, "CREATE DATABASE s1f_lost OWNER s1f_owner;")
	facts := pgFixScan(t, port, "s1f_lost", []string{"s1f_owner", "s1f_reader"})
	saved := pgPublicAccessRunner
	pgPublicAccessRunner = func(p int) (func(string) ([]byte, error), error) {
		run, err := saved(p)
		if err != nil {
			return nil, err
		}
		return func(script string) ([]byte, error) {
			out, err := run(script)
			if err != nil || !strings.Contains(script, "COMMIT;") {
				return out, err
			}
			// The engine committed; everything after COMMIT is lost.
			if at := bytes.Index(out, []byte("after=")); at >= 0 {
				out = out[:at]
			}
			return out, errors.New("signal: killed")
		}, nil
	}
	var closed transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1f_lost", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "s1f_reader"}}, NoOtherSessions: true, Recorded: []string{"s1f_owner", "s1f_reader"}}, &closed)
	pgPublicAccessRunner = saved
	if closed.Closed || closed.Changed || !strings.Contains(closed.Error, "not known") {
		t.Fatalf("a close whose answer was lost after COMMIT was answered as known: %+v", closed)
	}
	// It is in fact closed, with its grant.
	mustFail(t, "another account after the lost answer", connects("s1f_stranger", "s1f_lost", "SELECT 1"))
	mustRun(t, "the recorded reader", connects("s1f_reader", "s1f_lost", "SELECT 1"))
	// The undo of what the record says the close gave.
	pgFixUndo(t, e, port, "s1f_lost", []string{"s1f_reader:CONNECT"})
	mustRun(t, "another account after the undo", connects("s1f_stranger", "s1f_lost", "SELECT 1"))
	if acl := pgFixACL(t, e, "s1f_lost"); strings.Contains(acl, "s1f_reader") {
		t.Fatalf("the undo left the close's grant: %s", acl)
	}
}

// Finding 6: roles that use a database without owning anything in it.
func TestS1bFixColumnRightsSettingsPoliciesAndMappingsCountAsInside(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	roles := []string{"s1f_owner", "s1f_column", "s1f_setting", "s1f_policy", "s1f_mapped", "s1f_stranger"}
	pgFixSetup(t, e, []string{"s1f_inside"}, roles, "CREATE DATABASE s1f_inside OWNER s1f_owner;",
		"ALTER ROLE s1f_setting IN DATABASE s1f_inside SET work_mem = '8MB';")
	mustRun(t, "objects", e.pg("", "", "s1f_inside", "SET ROLE s1f_owner; CREATE TABLE public.t (id int, secret text); GRANT SELECT (id) ON public.t TO s1f_column;\n"+
		"ALTER TABLE public.t ENABLE ROW LEVEL SECURITY; CREATE POLICY p ON public.t FOR SELECT TO s1f_policy USING (true); RESET ROLE;\n"+
		"CREATE FOREIGN DATA WRAPPER s1f_wrapper; CREATE SERVER s1f_server FOREIGN DATA WRAPPER s1f_wrapper; CREATE USER MAPPING FOR s1f_mapped SERVER s1f_server;"))
	mustRun(t, "the column grantee before", connects("s1f_column", "s1f_inside", "SELECT id FROM public.t"))
	facts := pgFixScan(t, port, "s1f_inside", []string{"s1f_owner"})
	for _, role := range []string{"s1f_column", "s1f_setting", "s1f_policy", "s1f_mapped"} {
		if !containsString(facts.Roles, role) {
			t.Fatalf("%s uses s1f_inside but was not seen inside it: %v", role, facts.Roles)
		}
	}
	if containsString(facts.Roles, "s1f_stranger") {
		t.Fatalf("a role with nothing in it counted as inside: %v", facts.Roles)
	}
	// The administrator's close, after reading them, and its undo.
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1f_inside", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Recorded: []string{"s1f_owner"}}, &closed); err != nil || !closed.Closed {
		t.Fatalf("close: %v %+v", err, closed)
	}
	mustFail(t, "the column grantee after the close", connects("s1f_column", "s1f_inside", "SELECT id FROM public.t"))
	pgFixUndo(t, e, port, "s1f_inside", closed.Change.Added)
	mustRun(t, "the column grantee after the undo", connects("s1f_column", "s1f_inside", "SELECT id FROM public.t"))
}

// Finding 9: a role whose name would end the guards' dollar-quoted body.
func TestS1bFixARoleNamedToBreakOutIsCarriedSafely(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	role := `"s1f_x$celikpanel$"`
	pgFixSetup(t, e, []string{"s1f_breakout"}, []string{"s1f_owner", role, "s1f_stranger"}, "CREATE DATABASE s1f_breakout OWNER s1f_owner;",
		"GRANT CONNECT ON DATABASE s1f_breakout TO "+role+";")
	mustRun(t, "the role's table", e.pg("", "", "s1f_breakout", "CREATE TABLE public.t (id int); ALTER TABLE public.t OWNER TO "+role+";"))
	facts := pgFixScan(t, port, "s1f_breakout", []string{"s1f_owner"})
	if !containsString(facts.Roles, "s1f_x$celikpanel$") || !strings.Contains(facts.ACL, "s1f_x$celikpanel$") {
		t.Fatalf("the role was not read: %+v", facts)
	}
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1f_breakout", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Recorded: []string{"s1f_owner"}}, &closed); err != nil || !closed.Closed {
		t.Fatalf("close with the role's name in the guards: %v %+v", err, closed)
	}
	mustFail(t, "another account after the close", connects("s1f_stranger", "s1f_breakout", "SELECT 1"))
	pgFixUndo(t, e, port, "s1f_breakout", closed.Change.Added)
	mustRun(t, "another account after the undo", connects("s1f_stranger", "s1f_breakout", "SELECT 1"))
}
