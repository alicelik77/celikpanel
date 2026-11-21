package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026), corrected in the alpha.143 fix round: the Agent's
// close and undo of a PostgreSQL database's default rights, without an
// engine (pg_public_access_engine_test.go runs them on one).

func pgPublicRunner(t *testing.T, answer func(script string) ([]byte, error)) *[]string {
	t.Helper()
	var scripts []string
	saved := pgPublicAccessRunner
	pgPublicAccessRunner = func(int) (func(string) ([]byte, error), error) {
		return func(script string) ([]byte, error) {
			scripts = append(scripts, script)
			return answer(script)
		}, nil
	}
	t.Cleanup(func() { pgPublicAccessRunner = saved })
	return &scripts
}

func TestPGPublicCloseIsOneGuardedTransactionThatReadsBack(t *testing.T) {
	script, err := pgPublicCloseScript(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectACL: "", ExpectRoles: []string{"shop_owner", "reader"},
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}, NoOtherSessions: true, Recorded: []string{"shop_owner", "reader", "writer"}})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{`\connect "shop"`, "BEGIN;", "'began=1'", "celikpanel.expect_roles', (ARRAY['shop_owner','reader']::pg_catalog.text[])", "'before='", "'had='", "'celikpanel-changed'",
		"pg_stat_activity", `GRANT CONNECT ON DATABASE "shop" TO "reader";`, `GRANT CONNECT, TEMPORARY ON DATABASE "shop" TO "writer";`,
		`REVOKE CONNECT, TEMPORARY ON DATABASE "shop" FROM PUBLIC;`, "'celikpanel-readback'", "COMMIT;", "'after='"}
	at := 0
	for _, part := range order {
		i := strings.Index(script[at:], part)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", part, script)
		}
		at += i + len(part)
	}
	if strings.Contains(script, "REVOKE ALL") {
		t.Fatal("the close takes more from PUBLIC than an undo gives back")
	}
	for _, bad := range []transport.PostgreSQLPublicAccessCloseRequest{
		{Name: "shop", ExpectRoles: []string{"x'; DROP ROLE y; --"}},
		{Name: "shop", Grants: []transport.PostgreSQLPublicAccessGrant{{Role: `a"b`}}},
		{Name: "shop", Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "a$b"}}},
		{Name: `sh"op`},
	} {
		if _, err := pgPublicCloseScript(&bad); err == nil {
			t.Fatalf("a request with a name CelikPanel does not carry was scripted: %+v", bad)
		}
	}
}

// Finding 9: a role name that would end a dollar-quoted body is never
// placed inside one; the guards read the names as transaction settings.
func TestPGPublicNamesNeverEndADollarQuotedBody(t *testing.T) {
	breakout := "x$celikpanel$y"
	acl := `{=Tc/shop_owner,"x$celikpanel$y"=c/shop_owner}`
	close, err := pgPublicCloseScript(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectACL: acl, ExpectRoles: []string{"shop_owner", breakout},
		NoOtherSessions: true, Recorded: []string{"shop_owner", breakout}})
	if err != nil {
		t.Fatal(err)
	}
	undo, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", ExpectACL: acl, PublicConnect: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{close, undo} {
		bodies := 0
		for _, line := range strings.Split(script, "\n") {
			if !strings.HasPrefix(line, "DO $celikpanel$") {
				continue
			}
			bodies++
			if strings.Contains(line, breakout) || strings.Count(line, "$celikpanel$") != 2 {
				t.Fatalf("a name inside a dollar-quoted body:\n%s", line)
			}
		}
		if bodies == 0 || !strings.Contains(script, "set_config('celikpanel.expect_acl'") {
			t.Fatalf("the guards do not read their values as settings:\n%s", script)
		}
	}
}

func TestPGPublicCloseAnswersWhatItChanged(t *testing.T) {
	scripts := pgPublicRunner(t, func(string) ([]byte, error) {
		return []byte("began=1\nbefore=\nhad=writer:CONNECT\nafter={shop_owner=CTc/shop_owner,writer=Tc/shop_owner,reader=c/shop_owner}\n"), nil
	})
	var resp transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{"shop_owner"},
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}, Recorded: []string{"shop_owner"}}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Closed || resp.Change == nil || !reflect.DeepEqual(resp.Change.Added, []string{"reader:CONNECT", "writer:TEMPORARY"}) || len(*scripts) != 1 {
		t.Fatalf("close = %+v %+v", resp, resp.Change)
	}
	for _, c := range []struct {
		name    string
		out     string
		err     error
		changed bool
		unknown bool
	}{
		{"the guard refused", "began=1\nbefore=\nERROR:  celikpanel-changed\n", errors.New("exit status 3"), true, false},
		{"the read-back refused (rolled back)", "began=1\nbefore=\nERROR:  celikpanel-readback\n", errors.New("exit status 3"), false, false},
		{"the database could not be entered", `psql:<stdin>:1: error: \connect: connection to server failed: FATAL:  database "shop" does not exist` + "\n", errors.New("exit status 2"), false, false},
		// Finding 4: past BEGIN, the answer is unknown, never "nothing changed".
		{"a grant failed after BEGIN", "began=1\nbefore=\n" + `ERROR:  role "gone" does not exist` + "\n", errors.New("exit status 3"), false, true},
		{"the connection ended after COMMIT", "began=1\nbefore=\n" + "server closed the connection unexpectedly\n", errors.New("exit status 2"), false, true},
		{"killed with nothing printed", "", errors.New("signal: killed"), false, true},
		{"no read-back after a clean exit", "began=1\nbefore=\n", nil, false, true},
	} {
		pgPublicRunner(t, func(string) ([]byte, error) { return []byte(c.out), c.err })
		resp = transport.PostgreSQLPublicAccessCloseResponse{}
		_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Name: "shop", ExpectRoles: []string{}, Recorded: []string{}}, &resp)
		if resp.Closed || resp.Changed != c.changed || resp.Unknown != c.unknown || resp.Error == "" {
			t.Fatalf("%s: %+v", c.name, resp)
		}
	}
}

func TestPGPublicUndoGivesBackExactlyWhatWasRecorded(t *testing.T) {
	script, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", ExpectACL: "{shop_owner=CTc/shop_owner,reader=c/shop_owner}",
		PublicConnect: true, PublicTemporary: true, Added: []string{"reader:CONNECT"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"BEGIN;", "'celikpanel-changed'", `GRANT CONNECT, TEMPORARY ON DATABASE "shop" TO PUBLIC;`, `REVOKE CONNECT ON DATABASE "shop" FROM "reader";`, "'celikpanel-readback'", "COMMIT;"} {
		if !strings.Contains(script, part) {
			t.Fatalf("%q missing in:\n%s", part, script)
		}
	}
	if _, err := pgPublicUndoScript(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", Added: []string{"reader:SUPERUSER"}}); err == nil {
		t.Fatal("an undo took away a right a close never gives")
	}
	pgPublicRunner(t, func(string) ([]byte, error) { return []byte("began=1\nserver closed the connection unexpectedly\n"), errors.New("exit status 2") })
	var resp transport.PostgreSQLPublicAccessUndoResponse
	_ = (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Name: "shop", PublicConnect: true}, &resp)
	if resp.Undone || !resp.Unknown {
		t.Fatalf("an undo whose answer ended after BEGIN: %+v", resp)
	}
}

// Finding 13: one session reads every database of the cluster; a database
// that cannot be entered ends that session and the next one starts after it.
func TestPGPublicScanReadsEveryDatabaseInOneSession(t *testing.T) {
	facts := "owner=shop_owner\nacl=\npublic=true,true\nrole=other\nrole=shop_owner\nsession=other\n"
	scripts := pgPublicRunner(t, func(script string) ([]byte, error) {
		if strings.Contains(script, "celikpanel-header-done") {
			// shop is read, gone is answered as gone, broken cannot be
			// entered: the session ends there.
			return []byte("monitor=metrics\nreplication=subscriber\nknown=shop_owner\ncelikpanel-header-done\n" +
				"celikpanel-database=0\n" + facts + "celikpanel-database=1\nabsent=1\ncelikpanel-database=2\n" +
				`psql:<stdin>:9: error: \connect: connection to server failed: FATAL:  database "broken" is not currently accepting connections` + "\n"), errors.New("exit status 2")
		}
		return []byte("celikpanel-database=3\n" + facts + "celikpanel-database=end\n"), nil
	})
	var resp transport.PostgreSQLPublicAccessScanResponse
	_ = (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Databases: []transport.PostgreSQLPublicAccessDatabase{
		{Name: "shop", Recorded: []string{"shop_owner", "gone_user"}}, {Name: "gone"}, {Name: "broken"}, {Name: "two"}}}, &resp)
	if resp.Error != "" || !reflect.DeepEqual(resp.Monitoring, []string{"metrics"}) || !reflect.DeepEqual(resp.Replication, []string{"subscriber"}) || len(resp.Databases) != 4 {
		t.Fatalf("scan = %+v", resp)
	}
	if len(*scripts) != 2 || strings.Count((*scripts)[0], `\connect`) != 4 || strings.Count((*scripts)[1], `\connect`) != 1 || strings.Contains((*scripts)[1], "header") {
		t.Fatalf("sessions: %q", *scripts)
	}
	shop := resp.Databases[0]
	if !shop.Exists || shop.Owner != "shop_owner" || shop.ACL != "" || !shop.PublicConnect || !shop.PublicTemporary ||
		!reflect.DeepEqual(shop.Roles, []string{"other", "shop_owner"}) || !reflect.DeepEqual(shop.Sessions, []string{"other"}) || !reflect.DeepEqual(shop.Missing, []string{"gone_user"}) {
		t.Fatalf("shop = %+v", shop)
	}
	if resp.Databases[1].Exists || resp.Databases[1].Error != "" {
		t.Fatalf("gone = %+v", resp.Databases[1])
	}
	if broken := resp.Databases[2]; !broken.Exists || !strings.Contains(broken.Error, "not currently accepting connections") {
		t.Fatalf("broken = %+v", broken)
	}
	if two := resp.Databases[3]; two.Error != "" || two.Owner != "shop_owner" {
		t.Fatalf("two = %+v", two)
	}
}
