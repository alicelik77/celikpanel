//go:build linux

package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026), on a real engine: a PostgreSQL database made with
// the default rights (as before alpha.141) is closed to another account's
// user, while its owner, its read user, its read-and-write users (the web
// tool signs in as one of them, through the maintenance database first) and
// a user allowed in from outside still connect; a database with a role
// outside the records is left exactly as it is; the undo gives back exactly
// what the close changed. The private cluster of the test environment
// (CELIKPANEL_TEST_ENGINES, CELIKPANEL_TEST_PG_PORT); skipped without it.

func TestS1bClosingAnOldDatabaseKeepsItsUsersAndRefusesOthers(t *testing.T) {
	e := requireTestEngines(t)
	if e.pgPort == "" {
		t.Skip("CELIKPANEL_TEST_PG_PORT is not set")
	}
	installTestEngines(t, e)
	port, _ := strconv.Atoi(e.pgPort)
	roles := []string{"s1p_owner", "s1p_reader", "s1p_writer", "s1p_remote", "s1p_stranger"}
	clean := "DROP DATABASE IF EXISTS s1p_shop;\nDROP DATABASE IF EXISTS s1p_foreign;\nDROP ROLE IF EXISTS " + strings.Join(roles, ", ") + ";"
	mustRun(t, "clean", e.pg("", "", "", clean))
	t.Cleanup(func() { _ = e.pg("", "", "", clean) })
	setup := []string{}
	for _, role := range roles {
		setup = append(setup, "CREATE ROLE "+role+" LOGIN PASSWORD '"+engineOwnerPassword+"';")
	}
	// As before alpha.141: CREATE DATABASE with PostgreSQL's default rights.
	setup = append(setup, "CREATE DATABASE s1p_shop OWNER s1p_owner;", "CREATE DATABASE s1p_foreign OWNER s1p_owner;")
	mustRun(t, "setup", e.pg("", "", "", strings.Join(setup, "\n")))
	mustRun(t, "shop", e.pg("", "", "s1p_shop", "SET ROLE s1p_owner; CREATE TABLE public.t (id int); INSERT INTO public.t VALUES (1);\n"+
		"GRANT SELECT ON public.t TO s1p_reader; GRANT ALL ON public.t TO s1p_writer, s1p_remote;"))
	mustRun(t, "foreign", e.pg("", "", "s1p_foreign", "CREATE TABLE public.f (id int); ALTER TABLE public.f OWNER TO s1p_stranger;"))
	connects := func(role, database, statement string) engineResult {
		return e.pg(role, engineOwnerPassword, database, statement)
	}
	mustRun(t, "another account before the close", connects("s1p_stranger", "s1p_shop", "SELECT 1"))

	recordedShop := []string{"s1p_owner", "s1p_reader", "s1p_writer", "s1p_remote"}
	var scan transport.PostgreSQLPublicAccessScanResponse
	if err := (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Port: port, Databases: []transport.PostgreSQLPublicAccessDatabase{
		{Name: "s1p_shop", Recorded: recordedShop}, {Name: "s1p_foreign", Recorded: []string{"s1p_owner"}}}}, &scan); err != nil || scan.Error != "" || len(scan.Databases) != 2 {
		t.Fatalf("scan: %v %+v", err, scan)
	}
	shop, foreign := scan.Databases[0], scan.Databases[1]
	if !shop.Exists || shop.ACL != "" || !shop.PublicConnect || !shop.PublicTemporary || shop.Owner != "s1p_owner" ||
		strings.Join(shop.Roles, ",") != "s1p_owner,s1p_reader,s1p_remote,s1p_writer" {
		t.Fatalf("shop as read: %+v", shop)
	}
	if !containsString(foreign.Roles, "s1p_stranger") {
		t.Fatalf("the foreign role inside s1p_foreign was not seen: %+v", foreign)
	}

	// The foreign database is not closed: a close that expects only the
	// recorded roles inside it changes nothing.
	var refused transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1p_foreign", ExpectACL: "",
		ExpectRoles: []string{"s1p_owner"}, NoOtherSessions: true, Recorded: []string{"s1p_owner"}}, &refused)
	if refused.Closed || !refused.Changed {
		t.Fatalf("a database with a role outside the records was closed: %+v", refused)
	}
	if acl := mustRun(t, "foreign acl", e.pg("", "", "", "SELECT coalesce(datacl::text, 'untouched') FROM pg_database WHERE datname = 's1p_foreign';")); acl != "untouched" {
		t.Fatalf("s1p_foreign was changed: %s", acl)
	}
	mustRun(t, "the foreign database is left as it is", connects("s1p_stranger", "s1p_foreign", "SELECT 1"))

	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1p_shop", ExpectACL: "", ExpectRoles: shop.Roles,
		Grants:          []transport.PostgreSQLPublicAccessGrant{{Role: "s1p_reader"}, {Role: "s1p_writer", Temporary: true}, {Role: "s1p_remote", Temporary: true}},
		NoOtherSessions: true, Recorded: recordedShop}, &closed); err != nil || !closed.Closed || closed.Change == nil {
		t.Fatalf("close: %v %+v", err, closed)
	}
	if strings.Join(closed.Change.Added, ",") != "s1p_reader:CONNECT,s1p_remote:CONNECT,s1p_remote:TEMPORARY,s1p_writer:CONNECT,s1p_writer:TEMPORARY" {
		t.Fatalf("added = %v", closed.Change.Added)
	}
	mustFail(t, "another account's user after the close", connects("s1p_stranger", "s1p_shop", "SELECT 1"))
	mustRun(t, "the owner", connects("s1p_owner", "s1p_shop", "SELECT count(*) FROM public.t"))
	mustRun(t, "the read user", connects("s1p_reader", "s1p_shop", "SELECT count(*) FROM public.t"))
	mustFail(t, "the read user making a temporary table", connects("s1p_reader", "s1p_shop", "CREATE TEMP TABLE x (i int)"))
	mustRun(t, "the read-and-write user", connects("s1p_writer", "s1p_shop", "CREATE TEMP TABLE x (i int); INSERT INTO public.t VALUES (2)"))
	// The web tool signs in to the maintenance database first, then opens
	// the user's database.
	mustRun(t, "the web tool's first sign-in", connects("s1p_writer", "postgres", "SELECT 1"))
	mustRun(t, "the user let in from outside", connects("s1p_remote", "s1p_shop", "SELECT count(*) FROM public.t"))

	// An undo of rights changed since the close changes nothing.
	var stale transport.PostgreSQLPublicAccessUndoResponse
	_ = (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Port: port, Name: "s1p_shop", ExpectACL: "{}", PublicConnect: true, PublicTemporary: true}, &stale)
	if stale.Undone || !stale.Changed {
		t.Fatalf("a stale undo: %+v", stale)
	}
	var undone transport.PostgreSQLPublicAccessUndoResponse
	if err := (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Port: port, Name: "s1p_shop", ExpectACL: closed.Change.AfterACL,
		PublicConnect: true, PublicTemporary: true, Added: closed.Change.Added}, &undone); err != nil || !undone.Undone {
		t.Fatalf("undo: %v %+v", err, undone)
	}
	mustRun(t, "another account after the undo", connects("s1p_stranger", "s1p_shop", "SELECT 1"))
	left := mustRun(t, "rights after the undo", e.pg("", "", "", "SELECT coalesce(string_agg(DISTINCT pg_get_userbyid(a.grantee), ','), '') FROM pg_database d, aclexplode(d.datacl) a "+
		"WHERE d.datname = 's1p_shop' AND a.grantee <> 0 AND a.grantee <> d.datdba;"))
	if left != "" {
		t.Fatalf("the undo left rights the close gave: %s", left)
	}
}
