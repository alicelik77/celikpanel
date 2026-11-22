package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of the alpha.143 fix round (22 November 2026), PostgreSQL
// items B1 and B5–B9, on the Panel's side (the Agent's side and the real
// engine: cmd/agent/pg_public_access_round2*_test.go).

// B1: a close whose call failed without an answer stays "sending" while the
// Agent cannot confirm that it ended: no reading turns it into a watch or a
// failure, nothing is sent again, and once it is confirmed ended the
// database's state decides, with the close's full record (its Undo).
func TestPGPublicACloseThatMayStillRunStaysSendingAndIsNeverSentAgain(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	f.agent.running = map[string]bool{"clean_db": true}
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, _ *transport.PostgreSQLPublicAccessCloseResponse) error {
		if in.Name == "clean_db" {
			return errors.New("call timed out")
		}
		return nil
	}
	f.round(t) // the close of clean_db is sent; its answer does not arrive
	f.agent.close = nil
	sent := f.agent.closesOf("clean_db")
	if sent != 1 {
		t.Fatalf("closes of clean_db: %d", sent)
	}
	// The engine still shows it open (the close waits); every reading leaves
	// the record as it is while the Agent says a client of it runs.
	for round := 1; round <= 3; round++ {
		view := f.round(t)
		marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
		if marker == nil || marker.State != pgPublicSending || f.agent.closesOf("clean_db") != sent {
			t.Fatalf("round %d turned a close that may still run into %+v (closes %d)", round, marker, f.agent.closesOf("clean_db"))
		}
		if entry := pgPublicEntryOf(view.Open, "clean_db"); entry == nil || entry.Found[0] != "unknown_result" {
			t.Fatalf("round %d: the page does not say the result is unknown: %+v", round, entry)
		}
	}
	// "Read again" and an administrator's Close settle nothing either, and
	// the Close sends nothing.
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9981, "acl": "", "roles": []string{"admin_user", "reader"}}); rec.Code != http.StatusConflict ||
		!strings.Contains(rec.Body.String(), errCodePGPublicUnknown) || f.agent.closesOf("clean_db") != sent {
		t.Fatalf("an administrator's close while one may still run: %d %s", rec.Code, rec.Body.String())
	}
	// The Agent cannot answer the check: still nothing is settled.
	f.agent.running, f.agent.runningErr = nil, errors.New("connection refused")
	f.round(t)
	if marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981); marker == nil || marker.State != pgPublicSending {
		t.Fatalf("settled without the Agent's confirmation: %+v", marker)
	}
	// The other session ends, the waiting close commits, the client ends.
	f.agent.runningErr = nil
	f.agent.mu.Lock()
	f.agent.closeOnEngine("clean_db")
	f.agent.mu.Unlock()
	view := f.round(t)
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || !reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) || !marker.PublicConnect ||
		pgPublicEntryOf(view.Closed, "clean_db") == nil || len(pgPublicEntryOf(view.Closed, "clean_db").UndoStatements) == 0 {
		t.Fatalf("the close confirmed ended and applied: %+v", marker)
	}
	if f.agent.closesOf("clean_db") != sent {
		t.Fatal("the close was sent again")
	}
}

// B1: an undo whose answer did not arrive keeps the close's record and is
// settled at the next reading once nothing of it runs.
func TestPGPublicAnUndoWhoseAnswerIsLostKeepsTheCloseUntilItIsConfirmed(t *testing.T) {
	f := pgPublicFixture(t)
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}}); rec.Code != http.StatusOK {
		t.Fatalf("close: %d %s", rec.Code, rec.Body.String())
	}
	saved := pgPublicAgent
	lost := true
	pgPublicAgent = func(p *Panel, ctx context.Context, method string, req, resp any) error {
		if method == "Agent.PostgreSQLPublicAccessUndoV1" && lost {
			_ = f.agent.call(p, ctx, method, req, &transport.PostgreSQLPublicAccessUndoResponse{})
			return errors.New("call timed out")
		}
		return f.agent.call(p, ctx, method, req, resp)
	}
	t.Cleanup(func() { pgPublicAgent = saved })
	f.agent.running = map[string]bool{"foreign_db": true}
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "undo", "database_id": 9982}); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), errCodePGPublicUnknown) {
		t.Fatalf("an undo whose answer was lost: %d %s", rec.Code, rec.Body.String())
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicClosed || marker.UndoSentAt == "" {
		t.Fatalf("the close's record after a lost undo: %+v", marker)
	}
	view := f.round(t)
	if entry := pgPublicEntryOf(view.Closed, "foreign_db"); entry == nil {
		t.Fatalf("while the undo may still run the database is not shown as the close's record: %+v", view)
	}
	lost = false
	f.agent.running = nil
	view = f.round(t)
	marker, _ = f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicUndone || marker.UndoSentAt != "" || pgPublicEntryOf(view.Reopened, "foreign_db") == nil {
		t.Fatalf("the undo confirmed: %+v", marker)
	}
}

// B1: a watch that holds a sent close that was found not applied, and a
// database found closed later: that close's full record, never deleted.
func TestPGPublicAWatchHoldingASentCloseFoundClosedLaterIsThatClosesRecord(t *testing.T) {
	f := pgPublicFixture(t)
	sent := &pgPublicMarker{State: pgPublicSending, By: pgPublicByPanel, Port: 5432, Name: "clean_db", PublicConnect: true, PublicTemporary: true,
		Grants: []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}}, At: "2026-11-22T01:00:00Z"}
	if err := f.panel.writePGPublicMarker(context.Background(), 9981, &pgPublicMarker{State: pgPublicWatch, WatchSince: "2026-11-21T00:00:00Z", Rounds: 30, Sent: sent}); err != nil {
		t.Fatal(err)
	}
	f.agent.closeOnEngine("clean_db")
	view := f.round(t)
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || marker.At != "2026-11-22T01:00:00Z" || !reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) ||
		pgPublicEntryOf(view.Closed, "clean_db") == nil {
		t.Fatalf("the watch's sent close: %+v", marker)
	}
}

// B6: one database's close holds up neither another database's action nor
// "Read again".
func TestPGPublicOneDatabasesCloseDoesNotHoldUpOthers(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	f.agent.closeGate, f.agent.gateName = make(chan struct{}), "clean_db"
	roundDone := make(chan struct{})
	go func() {
		defer close(roundDone)
		f.clock.advance(time.Hour)
		_, _ = f.panel.pgPublicRead(context.Background(), pgPublicNow(), pgPublicPass{write: true, round: true, closeNow: true})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981); marker != nil && marker.State == pgPublicSending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the round did not start its close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	answered := make(chan *httptest.ResponseRecorder, 2)
	go func() {
		answered <- pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	}()
	go func() {
		answered <- pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "read"})
	}()
	for i := 0; i < 2; i++ {
		select {
		case rec := <-answered:
			if rec.Code != http.StatusOK {
				t.Errorf("an action beside a held close: %d %s", rec.Code, rec.Body.String())
			}
		case <-time.After(10 * time.Second):
			close(f.agent.closeGate)
			<-roundDone
			t.Fatal("another database's close or Read again waited behind one database's close")
		}
	}
	close(f.agent.closeGate)
	<-roundDone
	if marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982); marker == nil || marker.State != pgPublicClosed {
		t.Fatalf("foreign_db: %+v", marker)
	}
}

// B7: a round whose scan began before an administrator's close does not list
// the closed database as opened again.
func TestPGPublicARoundSkipsARecordChangedAfterItsScanBegan(t *testing.T) {
	f := pgPublicFixture(t)
	scan, err := f.panel.pgPublicFreshScan(context.Background(), pgPublicNow())
	if err != nil {
		t.Fatal(err)
	}
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}}); rec.Code != http.StatusOK {
		t.Fatalf("close: %d %s", rec.Code, rec.Body.String())
	}
	view, err := f.panel.pgPublicBuild(context.Background(), scan, pgPublicPass{write: true, round: true, closeNow: true})
	if err != nil {
		t.Fatal(err)
	}
	f.panel.storePGPublicList(context.Background(), view)
	if pgPublicEntryOf(view.Open, "foreign_db") != nil || pgPublicEntryOf(view.Closed, "foreign_db") == nil {
		t.Fatalf("the stale scan listed the closed database as open: open %+v", view.Open)
	}
	if strings.Contains(pgPublicProblemNames(t, f), "foreign_db") {
		t.Fatalf("Problems now lists a closed database: %q", pgPublicProblemNames(t, f))
	}
}

// B8: a session the close's own guard caught is recorded and named like one
// a round saw, and ends the watch.
func TestPGPublicASessionTheClosesGuardCaughtIsRecordedAndEndsTheWatch(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		out.Changed, out.Error = true, "the database is no longer as it was read; nothing was changed"
		if in.Name == "clean_db" {
			out.Sessions = []string{"reporting"}
		}
		return nil
	}
	view := f.round(t)
	entry := pgPublicEntryOf(view.Open, "clean_db")
	if entry == nil || !containsString(entry.Found, "other_sessions") || !reflect.DeepEqual(entry.Sessions, []string{"reporting"}) {
		t.Fatalf("the guard's session: %+v", entry)
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || !reflect.DeepEqual(marker.SeenRoles, []string{"reporting"}) || marker.SeenAt == "" || marker.WatchSince != "" {
		t.Fatalf("the record: %+v", marker)
	}
	sent := f.agent.closesOf("clean_db")
	for round := 1; round <= pgPublicWatchRounds+1; round++ {
		f.round(t)
	}
	if f.agent.closesOf("clean_db") != sent {
		t.Fatal("the close was sent again after the guard caught a session")
	}
	if !strings.Contains(pgPublicProblemNames(t, f), "clean_db") {
		t.Fatalf("Problems now: %q", pgPublicProblemNames(t, f))
	}
}

// B5: a database the rounds could not read more than three times in a row is
// listed in Problems now as not readable, with the reason.
func TestPGPublicADatabaseUnreadForMoreThanThreeRoundsIsListed(t *testing.T) {
	f := pgPublicFixture(t)
	clean := f.agent.facts["clean_db"]
	clean.Error = "the database could not be read: FATAL: too many connections"
	f.agent.facts["clean_db"] = clean
	unreadable := func() *monitoringProblem {
		for _, problem := range pgPublicProblems(context.Background(), f.panel.db.GetDB()) {
			if problem.Kind == "postgresql_databases_unreadable" {
				return &problem
			}
		}
		return nil
	}
	ctx := context.Background()
	for round := 1; round <= 4; round++ {
		f.clock.advance(time.Hour)
		f.panel.runPostgreSQLPublicRound(ctx, time.Time{})
		if problem := unreadable(); (problem != nil) != (round > pgPublicUnreadRounds) {
			t.Fatalf("round %d: %+v", round, problem)
		}
	}
	problem := unreadable()
	if problem.Actor != "administrator" || !strings.Contains(problem.Name, "clean_db (") || !strings.Contains(problem.Name, "too many connections") {
		t.Fatalf("the line: %+v", problem)
	}
	// Read again (not a round) keeps the count; a reading that reads it ends it.
	f.panel.storePGPublicList(ctx, pgPublicView{Unknown: []pgPublicEntry{{DatabaseID: 9981, Name: "clean_db", Reason: "again"}}})
	if unreadable() == nil {
		t.Fatal("Read again reset the count of a database it could not read either")
	}
	clean.Error = ""
	f.agent.facts["clean_db"] = clean
	f.clock.advance(time.Hour)
	f.panel.runPostgreSQLPublicRound(ctx, time.Time{})
	if problem := unreadable(); problem != nil {
		t.Fatalf("a database read again is still listed: %+v", problem)
	}
}

// B9: the record of a database that is gone goes with it.
func TestPGPublicTheRecordOfARemovedDatabaseIsRemoved(t *testing.T) {
	f := pgPublicFixture(t)
	for _, id := range []int{9981, 99999} {
		if err := f.panel.writePGPublicMarker(context.Background(), id, &pgPublicMarker{State: pgPublicWatch, WatchSince: "2026-11-21T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	f.panel.runPostgreSQLPublicRound(context.Background(), time.Time{})
	if marker, _ := f.panel.readPGPublicMarker(context.Background(), 99999); marker != nil {
		t.Fatalf("the record of a removed database stayed: %+v", marker)
	}
}
