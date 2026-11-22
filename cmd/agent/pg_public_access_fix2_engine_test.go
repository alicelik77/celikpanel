//go:build linux

package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.143 fix round, on real engines, with the answers the fix round adds
// (Unknown, Missing, Replication): a recorded user whose role is missing; a
// logical replication subscriber; one scan session for 300 databases.

// Finding 5: a recorded user whose role was removed by hand.
func TestS1bFixARecordedUserWithoutItsRoleIsSeenAndItsGrantIsNotNothingChanged(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	pgFixSetup(t, e, []string{"s1f_missing"}, []string{"s1f_owner", "s1f_stranger"}, "CREATE DATABASE s1f_missing OWNER s1f_owner;")
	facts := pgFixScan(t, port, "s1f_missing", []string{"s1f_owner", "s1f_gone"})
	if strings.Join(facts.Missing, ",") != "s1f_gone" {
		t.Fatalf("the missing role was not seen: %+v", facts)
	}
	var closed transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1f_missing", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "s1f_gone"}}, Recorded: []string{"s1f_owner", "s1f_gone"}}, &closed)
	// After BEGIN: unknown to the Agent; the Panel reads the database and
	// finds it unchanged (a failure with this reason).
	if closed.Closed || !closed.Unknown || !strings.Contains(closed.Error, `role "s1f_gone" does not exist`) {
		t.Fatalf("a grant to a missing role: %+v", closed)
	}
	if acl := pgFixACL(t, e, "s1f_missing"); acl != "" {
		t.Fatalf("the failed close changed the rights: %s", acl)
	}
	mustRun(t, "another account: nothing was changed", connects("s1f_stranger", "s1f_missing", "SELECT 1"))
}

// Finding 6: a logical replication subscriber's role (REPLICATION) and its
// connection to the published database.
func TestS1bFixAReplicationSubscriberIsSeen(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	second := os.Getenv("CELIKPANEL_TEST_PG_PORT2")
	if second == "" {
		t.Skip("a second private PostgreSQL cluster is needed (CELIKPANEL_TEST_PG_PORT2)")
	}
	subscriber := testEngines{dir: e.dir, pgPort: second}
	cleanSubscriber := "DROP DATABASE IF EXISTS s1f_copy WITH (FORCE);"
	mustRun(t, "subscriber clean", subscriber.pg("", "", "", cleanSubscriber))
	// Registered after the publisher's own cleanup, so it runs first: the
	// slot is dropped before the published database can go.
	pgFixSetup(t, e, []string{"s1f_published"}, []string{"s1f_owner", "s1f_repl"}, "ALTER ROLE s1f_repl REPLICATION;", "CREATE DATABASE s1f_published OWNER s1f_owner;")
	t.Cleanup(func() {
		_ = subscriber.pg("", "", "s1f_copy", "ALTER SUBSCRIPTION s1f_sub DISABLE; ALTER SUBSCRIPTION s1f_sub SET (slot_name = NONE); DROP SUBSCRIPTION s1f_sub;")
		_ = subscriber.pg("", "", "", cleanSubscriber)
		_ = e.pg("", "", "", "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE usename = 's1f_repl';")
		time.Sleep(time.Second)
		_ = e.pg("", "", "", "SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = 's1f_sub';")
	})
	mustRun(t, "publication", e.pg("", "", "s1f_published", "CREATE TABLE public.t (id int PRIMARY KEY); ALTER TABLE public.t OWNER TO s1f_owner; INSERT INTO public.t VALUES (1);\n"+
		"CREATE PUBLICATION s1f_pub FOR TABLE public.t;"))
	mustRun(t, "subscription", subscriber.pg("", "", "", "CREATE DATABASE s1f_copy;"))
	mustRun(t, "subscription", subscriber.pg("", "", "s1f_copy", "CREATE TABLE public.t (id int PRIMARY KEY);\n"+
		fmt.Sprintf("CREATE SUBSCRIPTION s1f_sub CONNECTION 'host=127.0.0.1 port=%s dbname=s1f_published user=s1f_repl password=%s' PUBLICATION s1f_pub WITH (copy_data = false);", e.pgPort, engineOwnerPassword)))
	deadline := time.Now().Add(30 * time.Second)
	for {
		if out := mustRun(t, "the subscriber's connection", e.pg("", "", "", "SELECT count(*) FROM pg_stat_activity WHERE usename = 's1f_repl' AND datname = 's1f_published';")); out != "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the subscriber did not connect")
		}
		time.Sleep(500 * time.Millisecond)
	}
	var scan transport.PostgreSQLPublicAccessScanResponse
	if err := (&Agent{}).PostgreSQLPublicAccessScanV1(&transport.PostgreSQLPublicAccessScanRequest{Port: port,
		Databases: []transport.PostgreSQLPublicAccessDatabase{{Name: "s1f_published", Recorded: []string{"s1f_owner"}}}}, &scan); err != nil || scan.Error != "" || len(scan.Databases) != 1 {
		t.Fatalf("scan: %v %+v", err, scan)
	}
	published := scan.Databases[0]
	if !containsString(scan.Replication, "s1f_repl") || !containsString(published.Sessions, "s1f_repl") {
		t.Fatalf("the subscriber was not seen: replication %v, sessions %v", scan.Replication, published.Sessions)
	}
	// The administrator closes after reading it; the subscriber's next
	// connection is refused; the undo lets it in again.
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1f_published", ExpectACL: published.ACL, ExpectRoles: published.Roles,
		Recorded: []string{"s1f_owner"}}, &closed); err != nil || !closed.Closed {
		t.Fatalf("close: %v %+v", err, closed)
	}
	mustFail(t, "the subscriber's role after the close", connects("s1f_repl", "s1f_published", "SELECT 1"))
	pgFixUndo(t, e, port, "s1f_published", closed.Change.Added)
	mustRun(t, "the subscriber's role after the undo", connects("s1f_repl", "s1f_published", "SELECT 1"))
}

// Finding 13: 300 recorded databases are read in one session; the time is
// logged against the Panel's 60-second read bound.
func TestS1bFixAScanOf300DatabasesIsOneSession(t *testing.T) {
	e, port, _ := pgFixEngine(t)
	const n = 300
	var names, create, drop []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("s1f_many_%03d", i)
		names = append(names, name)
		create = append(create, "CREATE DATABASE "+name+" OWNER s1f_owner STRATEGY FILE_COPY;")
		drop = append(drop, "DROP DATABASE IF EXISTS "+name+";")
	}
	pgFixSetup(t, e, nil, []string{"s1f_owner"})
	t.Cleanup(func() { _ = e.pg("", "", "", strings.Join(drop, "\n")) })
	started := time.Now()
	mustRun(t, "300 databases", e.pg("", "", "", strings.Join(create, "\n")))
	t.Logf("made %d databases in %s", n, time.Since(started).Round(time.Millisecond))
	sessions := 0
	saved := pgPublicAccessRunner
	pgPublicAccessRunner = func(p int, application string, deadline time.Time) (func(string) ([]byte, error), error) {
		run, err := saved(p, application, deadline)
		return func(script string) ([]byte, error) { sessions++; return run(script) }, err
	}
	defer func() { pgPublicAccessRunner = saved }()
	req := &transport.PostgreSQLPublicAccessScanRequest{Port: port}
	for _, name := range names {
		req.Databases = append(req.Databases, transport.PostgreSQLPublicAccessDatabase{Name: name, Recorded: []string{"s1f_owner"}})
	}
	var scan transport.PostgreSQLPublicAccessScanResponse
	started = time.Now()
	if err := (&Agent{}).PostgreSQLPublicAccessScanV1(req, &scan); err != nil || scan.Error != "" || len(scan.Databases) != n {
		t.Fatalf("scan: %v %s", err, scan.Error)
	}
	took := time.Since(started)
	for _, f := range scan.Databases {
		if !f.Exists || f.Error != "" || f.Owner != "s1f_owner" || !f.PublicConnect {
			t.Fatalf("a database was not read: %+v", f)
		}
	}
	t.Logf("scan of %d databases: %d session(s), %s", n, sessions, took.Round(time.Millisecond))
	if sessions != 1 {
		t.Fatalf("%d sessions for one scan", sessions)
	}
}
