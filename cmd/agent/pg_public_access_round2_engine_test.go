//go:build linux

package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of the alpha.143 fix round (22 November 2026), on a real
// engine (the private cluster of CELIKPANEL_TEST_ENGINES and
// CELIKPANEL_TEST_PG_PORT; skipped without it): a close that waits on a
// database row another session holds ends inside the engine or at the
// Agent's deadline and leaves nothing half done; the running check sees it
// while it runs; the database owner's own settings do not reach the
// administrator's session; the close's session guard names the role it
// caught.

// pgHolder is a second session that holds a transaction open until it is
// released.
type pgHolder struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func pgHold(t *testing.T, e testEngines, application string, statements string) *pgHolder {
	t.Helper()
	cmd := exec.Command(filepath.Join(e.dir, "root/usr/lib/postgresql/17/bin/psql"), "-h", filepath.Join(e.dir, "run"), "-p", e.pgPort, "-U", "postgres", "--no-psqlrc", "-At")
	cmd.Env = append(e.env(), "PGAPPNAME="+application)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	holder := &pgHolder{cmd: cmd, stdin: stdin}
	t.Cleanup(holder.release)
	if _, err := io.WriteString(stdin, statements+"\n"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		out := mustRun(t, "the holder", e.pg("", "", "", "SELECT count(*) FROM pg_stat_activity WHERE application_name = '"+application+"' AND state = 'idle in transaction';"))
		if out == "1" {
			return holder
		}
		if time.Now().After(deadline) {
			t.Fatal("the second session did not take the row")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (h *pgHolder) release() {
	if h.stdin != nil {
		_, _ = io.WriteString(h.stdin, "ROLLBACK;\n")
		_ = h.stdin.Close()
		h.stdin = nil
		_ = h.cmd.Wait()
	}
}

func pgRunning(t *testing.T, port int, name string) transport.PostgreSQLPublicAccessRunningResponse {
	t.Helper()
	var resp transport.PostgreSQLPublicAccessRunningResponse
	if err := (&Agent{}).PostgreSQLPublicAccessRunningV1(&transport.PostgreSQLPublicAccessRunningRequest{Port: port, Name: name}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// B1: the close waits on the database's row; the Agent's deadline ends it
// (unknown, with the time); the running check sees it while it runs and
// confirms it gone after; nothing was changed; with the deadline long, the
// close's own lock_timeout ends it first, whatever the owner set for his
// database.
func TestS1bRound2ACloseThatWaitsIsEndedAndLeavesNothingHalfDone(t *testing.T) {
	e, port, connects := pgFixEngine(t)
	pgFixSetup(t, e, []string{"s1r_held"}, []string{"s1r_owner", "s1r_reader", "s1r_stranger"}, "CREATE DATABASE s1r_held OWNER s1r_owner;")
	facts := pgFixScan(t, port, "s1r_held", []string{"s1r_owner", "s1r_reader"})
	request := &transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1r_held", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "s1r_reader"}}, NoOtherSessions: true, Recorded: []string{"s1r_owner", "s1r_reader"}}
	if running := pgRunning(t, port, "s1r_held"); running.Running || running.Error != "" {
		t.Fatalf("before any close: %+v", running)
	}

	holder := pgHold(t, e, "s1r-holder", "BEGIN;\nALTER DATABASE s1r_held CONNECTION LIMIT 50;")
	savedDeadline := pgPublicActDeadline
	pgPublicActDeadline = 4 * time.Second
	t.Cleanup(func() { pgPublicActDeadline = savedDeadline })
	done := make(chan transport.PostgreSQLPublicAccessCloseResponse)
	started := time.Now()
	go func() {
		var closed transport.PostgreSQLPublicAccessCloseResponse
		_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(request, &closed)
		done <- closed
	}()
	time.Sleep(1500 * time.Millisecond)
	if running := pgRunning(t, port, "s1r_held"); !running.Running {
		t.Fatalf("the running check did not see the waiting close: %+v", running)
	}
	closed := <-done
	took := time.Since(started)
	t.Logf("the waiting close was answered after %s: %+v", took.Round(time.Millisecond), closed)
	if !closed.Unknown || closed.Closed || !strings.Contains(closed.Error, "the client was ended at") || took > 20*time.Second {
		t.Fatalf("a close ended at its deadline: %+v after %s", closed, took)
	}
	deadline := time.Now().Add(15 * time.Second)
	for pgRunning(t, port, "s1r_held").Running {
		if time.Now().After(deadline) {
			t.Fatal("the ended close is still seen running")
		}
		time.Sleep(200 * time.Millisecond)
	}
	holder.release()
	time.Sleep(500 * time.Millisecond)
	if acl := pgFixACL(t, e, "s1r_held"); acl != "" {
		t.Fatalf("the ended close changed the rights: %s", acl)
	}
	mustRun(t, "another account after the ended close", connects("s1r_stranger", "s1r_held", "SELECT 1"))

	// The deadline long, the owner's own lock_timeout off for his database:
	// the close's SET LOCAL lock_timeout ends the wait inside the engine.
	mustRun(t, "the owner's setting", e.pg("", "", "", "ALTER DATABASE s1r_held SET lock_timeout = 0; ALTER DATABASE s1r_held SET statement_timeout = 0;"))
	pgPublicActDeadline = savedDeadline
	savedLock := pgPublicLockTimeout
	pgPublicLockTimeout = "2s"
	t.Cleanup(func() { pgPublicLockTimeout = savedLock })
	facts = pgFixScan(t, port, "s1r_held", []string{"s1r_owner", "s1r_reader"})
	request.ExpectACL, request.ExpectRoles = facts.ACL, facts.Roles
	holder = pgHold(t, e, "s1r-holder", "BEGIN;\nALTER DATABASE s1r_held CONNECTION LIMIT 60;")
	started = time.Now()
	var bounded transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(request, &bounded)
	took = time.Since(started)
	t.Logf("the close bounded by its lock_timeout was answered after %s: %+v", took.Round(time.Millisecond), bounded)
	if bounded.Closed || bounded.Unknown || !strings.Contains(bounded.Detail, "lock timeout") || took > 20*time.Second {
		t.Fatalf("a close bounded by its lock_timeout: %+v after %s", bounded, took)
	}
	holder.release()
	if acl := pgFixACL(t, e, "s1r_held"); acl != "" {
		t.Fatalf("the bounded close changed the rights: %s", acl)
	}
	// Released: the close goes through, and its undo.
	var final transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(request, &final); err != nil || !final.Closed {
		t.Fatalf("the close after the release: %v %+v", err, final)
	}
	pgFixUndo(t, e, port, "s1r_held", final.Change.Added)
}

// B5: what a database's owner sets for his database (read-only mode, the
// role, timeouts, exit_on_error) does not reach the administrator's session
// inside it.
func TestS1bRound2TheOwnersDatabaseSettingsDoNotReachTheAdministratorSession(t *testing.T) {
	e, port, _ := pgFixEngine(t)
	pgFixSetup(t, e, []string{"s1r_tuned"}, []string{"s1r_owner", "s1r_reader"}, "CREATE DATABASE s1r_tuned OWNER s1r_owner;")
	mustRun(t, "the owner's settings", e.pg("", "", "", strings.Join([]string{
		"ALTER DATABASE s1r_tuned SET default_transaction_read_only = on;",
		"ALTER DATABASE s1r_tuned SET role = 's1r_owner';",
		"ALTER DATABASE s1r_tuned SET exit_on_error = on;",
		"ALTER DATABASE s1r_tuned SET lock_timeout = '1ms';",
		"ALTER DATABASE s1r_tuned SET idle_in_transaction_session_timeout = '1ms';",
	}, "\n")))
	// The owner's own session gets them (they are real).
	if out := mustRun(t, "the owner's session", e.pg("s1r_owner", engineOwnerPassword, "s1r_tuned", "SHOW default_transaction_read_only;")); out != "on" {
		t.Fatalf("the owner's setting did not apply to his own session: %q", out)
	}
	run, err := pgPublicAccessRunner(port, "celikpanel-pgpublic-test", time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	out, err := run(`\connect s1r_tuned` + "\n" + pgPublicSessionPins + "\nSELECT 'who=' OPERATOR(pg_catalog.||) current_user, 'ro=' OPERATOR(pg_catalog.||) pg_catalog.current_setting('default_transaction_read_only');\n" +
		"SELECT 'lock=' OPERATOR(pg_catalog.||) pg_catalog.current_setting('lock_timeout'), 'exit=' OPERATOR(pg_catalog.||) pg_catalog.current_setting('exit_on_error');")
	if err != nil {
		t.Fatalf("the administrator session: %v %s", err, out)
	}
	for _, want := range []string{"who=postgres", "ro=off", "lock=0", "exit=off"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("the administrator session inside the owner's database lacks %s:\n%s", want, out)
		}
	}
	facts := pgFixScan(t, port, "s1r_tuned", []string{"s1r_owner", "s1r_reader"})
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1r_tuned", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "s1r_reader"}}, NoOtherSessions: true, Recorded: []string{"s1r_owner", "s1r_reader"}}, &closed); err != nil || !closed.Closed {
		t.Fatalf("the close of a database whose owner tuned it: %v %+v", err, closed)
	}
	pgFixUndo(t, e, port, "s1r_tuned", closed.Change.Added)
	// alpha.143 lab, P3: the undo leaves PostgreSQL's default rights written
	// out for the owner (the Panel's pgPublicDefaultACL takes it as the
	// default; cmd/panel TestPGPublicTheDefaultRightsWrittenOutAreTheDefault).
	acl := pgFixACL(t, e, "s1r_tuned")
	items := strings.Split(strings.Trim(acl, "{}"), ",")
	sort.Strings(items)
	t.Logf("the rights after the undo: %s", acl)
	if strings.Join(items, ",") != "=Tc/s1r_owner,s1r_owner=CTc/s1r_owner" {
		t.Fatalf("the rights after the undo are not the default written out: %s", acl)
	}
}

// B8: the close's session guard names the role it caught.
func TestS1bRound2TheClosesSessionGuardNamesTheRoleItCaught(t *testing.T) {
	e, port, _ := pgFixEngine(t)
	pgFixSetup(t, e, []string{"s1r_seen"}, []string{"s1r_owner", "s1r_stranger"}, "CREATE DATABASE s1r_seen OWNER s1r_owner;")
	facts := pgFixScan(t, port, "s1r_seen", []string{"s1r_owner"})
	stranger := exec.Command(filepath.Join(e.dir, "root/usr/lib/postgresql/17/bin/psql"), "-h", "127.0.0.1", "-p", e.pgPort, "-U", "s1r_stranger", "-d", "s1r_seen", "--no-psqlrc", "-c", "SELECT pg_sleep(30)")
	stranger.Env = append(e.env(), "PGPASSWORD="+engineOwnerPassword)
	if err := stranger.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stranger.Process.Signal(os.Kill); _ = stranger.Wait() })
	deadline := time.Now().Add(15 * time.Second)
	for mustRun(t, "the stranger's session", e.pg("", "", "", "SELECT count(*) FROM pg_stat_activity WHERE usename = 's1r_stranger' AND datname = 's1r_seen';")) != "1" {
		if time.Now().After(deadline) {
			t.Fatal("the stranger did not connect")
		}
		time.Sleep(100 * time.Millisecond)
	}
	var closed transport.PostgreSQLPublicAccessCloseResponse
	_ = (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: port, Name: "s1r_seen", ExpectACL: facts.ACL, ExpectRoles: facts.Roles,
		NoOtherSessions: true, Recorded: []string{"s1r_owner"}}, &closed)
	if !closed.Changed || closed.Closed || !reflect.DeepEqual(closed.Sessions, []string{"s1r_stranger"}) {
		t.Fatalf("the guard's refusal: %+v", closed)
	}
	if acl := pgFixACL(t, e, "s1r_seen"); acl != "" {
		t.Fatalf("the refused close changed the rights: %s", acl)
	}
}
