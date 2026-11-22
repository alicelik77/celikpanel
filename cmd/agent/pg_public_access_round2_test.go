package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of the alpha.143 fix round (22 November 2026), the Agent's
// side without an engine (pg_public_access_round2_engine_test.go runs the
// same on one).

// B1: the close and the undo bound themselves inside the engine before their
// first statement that can wait, and set the timeouts the connection options
// cannot pin in every version right after connecting.
func TestPGPublicCloseAndUndoBoundThemselvesBeforeTheyCanWait(t *testing.T) {
	closeScript, err := pgPublicCloseScript(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"},
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}}, NoOtherSessions: true, Recorded: []string{"shop_owner", "reader"}})
	if err != nil {
		t.Fatal(err)
	}
	undoScript, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", PublicConnect: true, Added: []string{"reader:CONNECT"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{"close": closeScript, "undo": undoScript} {
		order := []string{`\connect "shop"`, "idle_session_timeout','transaction_timeout'", "BEGIN;", "SET LOCAL lock_timeout = '30s';", "SET LOCAL statement_timeout = '60s';", "pg_catalog.pg_database d", "COMMIT;"}
		at := 0
		for _, part := range order {
			i := strings.Index(script[at:], part)
			if i < 0 {
				t.Fatalf("%s: %q missing or out of order in:\n%s", name, part, script)
			}
			at += i + len(part)
		}
		if strings.Index(script, "SET LOCAL statement_timeout") > strings.Index(script, "GRANT") && strings.Contains(script, "GRANT") {
			t.Fatalf("%s: a statement that can wait comes before the bound", name)
		}
	}
}

// B1: a client the Agent had to end at its deadline is "unknown", with the
// time it was ended; never a verified failure.
func TestPGPublicAClientEndedAtItsDeadlineIsUnknown(t *testing.T) {
	ended := time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC)
	pgPublicRunner(t, func(string) ([]byte, error) {
		return []byte("began=1\nERROR:  canceling statement due to user request\n"), &pgClientEndedError{at: ended}
	})
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"}}, &closed); err != nil {
		t.Fatal(err)
	}
	if !closed.Unknown || closed.Closed || closed.Changed || !strings.Contains(closed.Error, "the client was ended at 2026-11-22T03:00:00Z") {
		t.Fatalf("an ended close: %+v", closed)
	}
	var undone transport.PostgreSQLPublicAccessUndoResponse
	if err := (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", PublicConnect: true}, &undone); err != nil {
		t.Fatal(err)
	}
	if !undone.Unknown || undone.Undone || !strings.Contains(undone.Error, "the client was ended at") {
		t.Fatalf("an ended undo: %+v", undone)
	}
}

// B8: the close's session guard names the roles it found.
func TestPGPublicTheSessionGuardNamesTheRolesItFound(t *testing.T) {
	script, _ := pgPublicCloseScript(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"}, NoOtherSessions: true, Recorded: []string{"shop_owner"}})
	if !strings.Contains(script, "RAISE EXCEPTION 'celikpanel-changed sessions=%', found") || !strings.Contains(script, "pg_catalog.quote_ident(x)") {
		t.Fatalf("the session guard does not name the roles:\n%s", script)
	}
	out := []byte("began=1\npsql:<stdin>:12: ERROR:  celikpanel-changed sessions=reporting \"odd \"\"name\" zz\nCONTEXT:  PL/pgSQL function inline_code_block line 1 at RAISE\n")
	if got := pgPublicGuardSessions(out); !reflect.DeepEqual(got, []string{"reporting", `odd "name`, "zz"}) {
		t.Fatalf("roles read = %q", got)
	}
	pgPublicRunner(t, func(string) ([]byte, error) { return out, errors.New("exit status 3") })
	var closed transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"}, NoOtherSessions: true, Recorded: []string{"shop_owner"}}, &closed)
	if !closed.Changed || !reflect.DeepEqual(closed.Sessions, []string{"reporting", `odd "name`, "zz"}) {
		t.Fatalf("the close's answer: %+v", closed)
	}
}

// B5: whether each database exists is asked in the session's first
// connection, before any database is entered.
func TestPGPublicTheScanAsksWhichDatabasesExistBeforeEnteringAny(t *testing.T) {
	scripts := pgPublicRunner(t, func(script string) ([]byte, error) {
		return []byte("celikpanel-header-done\ncelikpanel-database=0\nabsent=1\ncelikpanel-database=1\nabsent=1\ncelikpanel-database=end\n"), nil
	})
	var scan transport.PostgreSQLPublicAccessScanResponse
	_ = (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Databases: []transport.PostgreSQLPublicAccessDatabase{{Name: "one"}, {Name: "two"}}}, &scan)
	if len(*scripts) != 1 {
		t.Fatalf("sessions: %d", len(*scripts))
	}
	script := (*scripts)[0]
	probe, connect := strings.Index(script, "AS celikpanel_exists_1"), strings.Index(script, `\connect`)
	if probe < 0 || connect < 0 || probe > connect || !strings.Contains(script, `\if :celikpanel_exists_0`) {
		t.Fatalf("a database's existence is asked inside another database:\n%s", script)
	}
}

// B5: a scan whose time ran out keeps the facts it read and says which
// databases it did not reach; it starts no further session.
func TestPGPublicAScanWhoseTimeRanOutKeepsWhatItRead(t *testing.T) {
	scripts := pgPublicRunner(t, func(script string) ([]byte, error) {
		return []byte("celikpanel-header-done\ncelikpanel-database=0\nowner=a\nacl=\npublic=true,true\ncelikpanel-database=1\nowner=b\n"), &pgClientEndedError{at: time.Now()}
	})
	var scan transport.PostgreSQLPublicAccessScanResponse
	_ = (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Databases: []transport.PostgreSQLPublicAccessDatabase{{Name: "one"}, {Name: "two"}, {Name: "three"}}}, &scan)
	if len(*scripts) != 1 || scan.Error != "" || len(scan.Databases) != 3 {
		t.Fatalf("sessions %d, scan %+v", len(*scripts), scan)
	}
	if one := scan.Databases[0]; one.Error != "" || one.Owner != "a" || !one.PublicConnect {
		t.Fatalf("the database read before the time ran out lost its facts: %+v", one)
	}
	for _, f := range scan.Databases[1:] {
		if !strings.Contains(f.Error, "time ran out") {
			t.Fatalf("not read: %+v", f)
		}
	}
}
