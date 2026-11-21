package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026), corrected in the alpha.143 fix round (independent
// review, 21 November 2026): PostgreSQL databases made before alpha.141 are
// closed to other accounts by themselves only after a day's rounds saw
// nobody else use them; an unknown answer is not "nothing changed"; a failed
// close is shown and not retried every hour; closes are serialised; a
// database an administrator opened again is the administrator's choice; the
// page reads the last scan.

func TestPGPublicReadsTheRightsAsPostgreSQLPrintsThem(t *testing.T) {
	got := pgACLPrivileges(`{=Tc/postgres,postgres=CTc/postgres,shop_user=c/postgres,"\"odd name\"=T/postgres"}`)
	want := map[string]string{"": "Tc", "postgres": "CTc", "shop_user": "c", "odd name": "T"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rights = %#v, want %#v", got, want)
	}
	for acl, want := range map[string][2]bool{
		"":                              {true, true},
		"{=c/owner,owner=CTc/owner}":    {true, false},
		"{=T/owner}":                    {false, true},
		"{owner=CTc/owner,u=c/owner}":   {false, false},
		`{"\"a=b\"=c/owner",=Tc/owner}`: {true, true},
	} {
		connect, temporary := pgPublicRights(acl)
		if connect != want[0] || temporary != want[1] {
			t.Fatalf("PUBLIC in %q = %v,%v, want %v", acl, connect, temporary, want)
		}
	}
	if added := pgPublicAddedFrom("{=Tc/o,shop_user=c/o}", []transport.PostgreSQLPublicAccessGrant{{Role: "shop_user", Temporary: true}, {Role: "reader"}}); !reflect.DeepEqual(added, []string{"reader:CONNECT", "shop_user:TEMPORARY"}) {
		t.Fatalf("added = %v", added)
	}
}

func TestPGPublicUndoStatementsGiveBackExactlyWhatTheCloseChanged(t *testing.T) {
	got := pgPublicUndoStatements(&pgPublicMarker{Name: "shop", PublicConnect: true, PublicTemporary: true, Added: []string{"reader:CONNECT", "writer:CONNECT", "writer:TEMPORARY"}})
	want := []string{
		`GRANT CONNECT, TEMPORARY ON DATABASE "shop" TO PUBLIC;`,
		`REVOKE CONNECT ON DATABASE "shop" FROM "reader";`,
		`REVOKE CONNECT ON DATABASE "shop" FROM "writer";`,
		`REVOKE TEMPORARY ON DATABASE "shop" FROM "writer";`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("undo statements:\n%s", strings.Join(got, "\n"))
	}
	if got := pgPublicUndoStatements(&pgPublicMarker{Name: "shop", PublicConnect: true}); !reflect.DeepEqual(got, []string{`GRANT CONNECT ON DATABASE "shop" TO PUBLIC;`}) {
		t.Fatalf("PUBLIC had CONNECT only: %v", got)
	}
}

func TestPGPublicDecideSaysWhyADatabaseIsLeftOpen(t *testing.T) {
	record := pgPublicRecord{id: 1, name: "shop", port: 5432, users: map[string]bool{"shop_owner": true, "reader": false, "writer": true}, userIn: []string{"shop_owner", "reader", "writer"}}
	clean := transport.PostgreSQLPublicAccessFacts{Name: "shop", Exists: true, Owner: "shop_owner", PublicConnect: true, PublicTemporary: true, Roles: []string{"shop_owner", "writer"}}
	entry := pgPublicDecide(record, clean, pgPublicCluster{}, nil)
	if len(entry.Found) != 0 {
		t.Fatalf("an untouched database found %v", entry.Found)
	}
	if !reflect.DeepEqual(entry.Grants, []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}) {
		t.Fatalf("grants (the owner holds its rights as owner) = %+v", entry.Grants)
	}
	for _, c := range []struct {
		name    string
		facts   func(*transport.PostgreSQLPublicAccessFacts)
		cluster pgPublicCluster
		marker  *pgPublicMarker
		want    string
	}{
		{"changed rights", func(f *transport.PostgreSQLPublicAccessFacts) { f.ACL = "{=Tc/shop_owner,other=c/shop_owner}" }, pgPublicCluster{}, nil, "changed_rights"},
		{"a foreign role inside", func(f *transport.PostgreSQLPublicAccessFacts) { f.Roles = append(f.Roles, "other_customer") }, pgPublicCluster{}, nil, "other_roles"},
		{"a foreign session", func(f *transport.PostgreSQLPublicAccessFacts) { f.Sessions = []string{"other_customer"} }, pgPublicCluster{}, nil, "other_sessions"},
		{"a foreign session seen before", func(*transport.PostgreSQLPublicAccessFacts) {}, pgPublicCluster{}, &pgPublicMarker{State: pgPublicWatch, SeenRoles: []string{"reporting"}}, "other_sessions"},
		{"a recorded user without its role", func(f *transport.PostgreSQLPublicAccessFacts) { f.Missing = []string{"reader"} }, pgPublicCluster{}, nil, "missing_users"},
		{"a monitoring role", func(*transport.PostgreSQLPublicAccessFacts) {}, pgPublicCluster{monitoring: []string{"metrics"}}, nil, "monitoring_roles"},
		{"a replication role", func(*transport.PostgreSQLPublicAccessFacts) {}, pgPublicCluster{replication: []string{"subscriber"}}, nil, "replication_roles"},
	} {
		facts := clean
		facts.Roles = append([]string{}, clean.Roles...)
		c.facts(&facts)
		if entry := pgPublicDecide(record, facts, c.cluster, c.marker); !containsString(entry.Found, c.want) {
			t.Fatalf("%s: found %v, want %s", c.name, entry.Found, c.want)
		}
	}
}

// pgPublicFakeAgent answers the three calls from what each database holds.
type pgPublicFakeAgent struct {
	mu          sync.Mutex
	facts       map[string]transport.PostgreSQLPublicAccessFacts
	monitoring  []string
	replication []string
	scans       int
	closes      []transport.PostgreSQLPublicAccessCloseRequest
	undos       []transport.PostgreSQLPublicAccessUndoRequest
	// close, when set, answers a close instead of the engine's own rule;
	// closeGate, when set, holds the close of gateName until it is closed.
	close     func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error
	closeGate chan struct{}
	gateName  string
	scanGate  chan struct{}
}

func (a *pgPublicFakeAgent) closeOnEngine(name string) {
	f := a.facts[name]
	f.ACL, f.PublicConnect, f.PublicTemporary = "{admin_user=CTc/admin_user}", false, false
	a.facts[name] = f
}

func (a *pgPublicFakeAgent) call(_ *Panel, _ context.Context, method string, req, resp any) error {
	switch method {
	case "Agent.PostgreSQLPublicAccessScanV1":
		if a.scanGate != nil {
			<-a.scanGate
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		a.scans++
		in, out := req.(*transport.PostgreSQLPublicAccessScanRequest), resp.(*transport.PostgreSQLPublicAccessScanResponse)
		out.Monitoring, out.Replication = a.monitoring, a.replication
		for _, database := range in.Databases {
			f, ok := a.facts[database.Name]
			if !ok {
				f = transport.PostgreSQLPublicAccessFacts{Name: database.Name}
			}
			out.Databases = append(out.Databases, f)
		}
	case "Agent.PostgreSQLPublicAccessCloseV1":
		if in := req.(*transport.PostgreSQLPublicAccessCloseRequest); a.closeGate != nil && in.Name == a.gateName {
			<-a.closeGate
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		in, out := req.(*transport.PostgreSQLPublicAccessCloseRequest), resp.(*transport.PostgreSQLPublicAccessCloseResponse)
		a.closes = append(a.closes, *in)
		if a.close != nil {
			return a.close(in, out)
		}
		f := a.facts[in.Name]
		if f.ACL != in.ExpectACL || !reflect.DeepEqual(nonNilStrings(f.Roles), in.ExpectRoles) || !f.PublicConnect {
			out.Changed, out.Error = true, "the database is no longer as it was read; nothing was changed"
			return nil
		}
		a.closeOnEngine(in.Name)
		out.Closed, out.Change = true, &transport.PostgreSQLPublicAccessChange{BeforeACL: in.ExpectACL, AfterACL: a.facts[in.Name].ACL, Added: []string{}}
		for _, grant := range in.Grants {
			out.Change.Added = append(out.Change.Added, grant.Role+":CONNECT")
		}
	case "Agent.PostgreSQLPublicAccessUndoV1":
		a.mu.Lock()
		defer a.mu.Unlock()
		in, out := req.(*transport.PostgreSQLPublicAccessUndoRequest), resp.(*transport.PostgreSQLPublicAccessUndoResponse)
		a.undos = append(a.undos, *in)
		f := a.facts[in.Name]
		f.ACL, f.PublicConnect, f.PublicTemporary = "{=Tc/admin_user,admin_user=CTc/admin_user}", true, true
		a.facts[in.Name] = f
		out.Undone, out.AfterACL = true, f.ACL
	default:
		return errors.New("unexpected call " + method)
	}
	return nil
}

func (a *pgPublicFakeAgent) closesOf(name string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.closes {
		if c.Name == name {
			n++
		}
	}
	return n
}

// pgPublicClock is the tests' time: each reading moves a second on; a round
// moves an hour on.
type pgPublicClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *pgPublicClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Second)
	return c.now
}

func (c *pgPublicClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type pgPublicTest struct {
	visibleDatabasesFixture
	agent *pgPublicFakeAgent
	clock *pgPublicClock
}

func pgPublicFixture(t *testing.T) pgPublicTest {
	t.Helper()
	f := newVisibleDatabasesFixture(t)
	// On the local PostgreSQL record 9924 (admin_tools with admin_user):
	// clean_db (reader reads it), foreign_db (another account owns a table
	// in it) and gone_db (no longer on the engine).
	f.exec(t, `INSERT INTO databases_v2 (id, server_id, subscription_id, domain_id, name) VALUES
		(9981, 9924, 9805, NULL, 'clean_db'), (9982, 9924, 9805, NULL, 'foreign_db'), (9983, 9924, 9805, NULL, 'gone_db')`)
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9984, 9924, 9805, 'reader', 'x')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9981, 9945, 'ALL'), (9981, 9984, 'SELECT'), (9982, 9945, 'ALL')`)
	open := func(name string, roles ...string) transport.PostgreSQLPublicAccessFacts {
		return transport.PostgreSQLPublicAccessFacts{Name: name, Exists: true, Owner: "admin_user", PublicConnect: true, PublicTemporary: true, Roles: roles, Sessions: []string{}}
	}
	agent := &pgPublicFakeAgent{facts: map[string]transport.PostgreSQLPublicAccessFacts{
		"admin_tools": open("admin_tools", "admin_user"),
		"clean_db":    open("clean_db", "admin_user", "reader"),
		"foreign_db":  open("foreign_db", "admin_user", "other_customer"),
	}}
	clock := &pgPublicClock{now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	savedAgent, savedNow := pgPublicAgent, pgPublicNow
	pgPublicAgent, pgPublicNow = agent.call, clock.read
	t.Cleanup(func() { pgPublicAgent, pgPublicNow = savedAgent, savedNow })
	return pgPublicTest{f, agent, clock}
}

// round is one hourly round with the admission given.
func (f pgPublicTest) round(t *testing.T) pgPublicView {
	t.Helper()
	f.clock.advance(time.Hour)
	ctx := context.Background()
	view, err := f.panel.pgPublicRead(ctx, pgPublicNow(), pgPublicPass{write: true, round: true, closeNow: true})
	if err != nil {
		t.Fatal(err)
	}
	f.panel.storePGPublicList(ctx, view)
	return view
}

func pgPublicEntryOf(entries []pgPublicEntry, name string) *pgPublicEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

func pgPublicProblemNames(t *testing.T, f pgPublicTest) string {
	t.Helper()
	problems := pgPublicProblems(context.Background(), f.panel.db.GetDB())
	if len(problems) == 0 {
		return ""
	}
	return problems[0].Name
}

func TestPGPublicClosesByItselfOnlyAfterADayWithNobodyElse(t *testing.T) {
	f := pgPublicFixture(t)
	// The page's first read scans and changes nothing on the engine.
	view, err := f.panel.pgPublicRead(context.Background(), time.Time{}, pgPublicPass{write: true})
	if err != nil || len(f.agent.closes) != 0 || len(view.Open) != 3 {
		t.Fatalf("a read changed something or missed one: %v, closes %d, open %+v", err, len(f.agent.closes), view.Open)
	}
	// The watch began at that reading; 23 rounds later nothing is closed.
	for round := 1; round < pgPublicWatchRounds; round++ {
		view = f.round(t)
		if len(f.agent.closes) != 0 {
			t.Fatalf("round %d closed before a day's rounds: %+v", round, f.agent.closes)
		}
	}
	clean := pgPublicEntryOf(view.Open, "clean_db")
	if clean == nil || !reflect.DeepEqual(clean.Found, []string{"observing"}) || clean.WatchSince == "" || clean.ClosesAt == "" || clean.Rounds != pgPublicWatchRounds-1 {
		t.Fatalf("the watched database: %+v", clean)
	}
	since, _ := time.Parse(time.RFC3339, clean.WatchSince)
	if closes, _ := time.Parse(time.RFC3339, clean.ClosesAt); !closes.Equal(since.Add(24 * time.Hour)) {
		t.Fatalf("closes at %s, watched since %s", clean.ClosesAt, clean.WatchSince)
	}
	// Problems now lists only what the administrator decides.
	if names := pgPublicProblemNames(t, f); names != "foreign_db" {
		t.Fatalf("Problems now: %q", names)
	}
	view = f.round(t) // a full day after the watch began
	if f.agent.closesOf("clean_db") != 1 || f.agent.closesOf("admin_tools") != 1 || f.agent.closesOf("foreign_db") != 0 {
		t.Fatalf("closed by itself: %+v", f.agent.closes)
	}
	last := f.agent.closes[len(f.agent.closes)-1]
	if last.Name == "clean_db" && (!last.NoOtherSessions || last.ExpectACL != "" || !reflect.DeepEqual(last.Grants, []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}})) {
		t.Fatalf("clean_db's close = %+v", last)
	}
	marker, err := f.panel.readPGPublicMarker(context.Background(), 9981)
	if err != nil || marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || !marker.PublicConnect || !marker.PublicTemporary ||
		!reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) {
		t.Fatalf("clean_db's record = %+v, %v", marker, err)
	}
	if len(view.Closed) != 2 || len(view.Open) != 1 {
		t.Fatalf("view after the day: open %+v closed %+v", view.Open, view.Closed)
	}
	before := len(f.agent.closes)
	f.round(t)
	if len(f.agent.closes) != before {
		t.Fatal("a later round closed again")
	}
}

func TestPGPublicAChangeStartsTheWatchAgain(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= 12; round++ {
		f.round(t)
	}
	// A new recorded user of clean_db: the facts changed, the count starts again.
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (9985, 9924, 9805, 'writer', 'x')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (9981, 9985, 'ALL')`)
	view := f.round(t)
	if clean := pgPublicEntryOf(view.Open, "clean_db"); clean == nil || clean.Rounds != 1 {
		t.Fatalf("after a change: %+v", clean)
	}
	for round := 1; round <= 13; round++ {
		f.round(t)
	}
	if f.agent.closesOf("clean_db") != 0 || f.agent.closesOf("admin_tools") != 1 {
		t.Fatalf("closes: %+v", f.agent.closes)
	}
}

func TestPGPublicAForeignSessionDuringTheWatchAsksTheAdministrator(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= 10; round++ {
		f.round(t)
	}
	clean := f.agent.facts["clean_db"]
	clean.Sessions = []string{"reporting"}
	f.agent.facts["clean_db"] = clean
	view := f.round(t)
	entry := pgPublicEntryOf(view.Open, "clean_db")
	if entry == nil || !containsString(entry.Found, "other_sessions") || !reflect.DeepEqual(entry.Sessions, []string{"reporting"}) || !reflect.DeepEqual(entry.SessionsNow, []string{"reporting"}) {
		t.Fatalf("a foreign session: %+v", entry)
	}
	// It disconnects; the database still waits for the administrator.
	clean.Sessions = []string{}
	f.agent.facts["clean_db"] = clean
	for round := 1; round <= 30; round++ {
		view = f.round(t)
	}
	if f.agent.closesOf("clean_db") != 0 {
		t.Fatal("a database a foreign role used was closed by itself")
	}
	entry = pgPublicEntryOf(view.Open, "clean_db")
	if entry == nil || !reflect.DeepEqual(entry.Sessions, []string{"reporting"}) || len(entry.SessionsNow) != 0 || !strings.Contains(pgPublicProblemNames(t, f), "clean_db") {
		t.Fatalf("after it left: %+v, problems %q", entry, pgPublicProblemNames(t, f))
	}
}

func TestPGPublicAnUnknownAnswerKeepsTheRecordAndReadsTheEngine(t *testing.T) {
	f := pgPublicFixture(t)
	// The close committed, but its answer ended inside the script.
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		f.agent.closeOnEngine(in.Name)
		out.Unknown, out.Error = true, "the database server's answer ended inside the close, so whether it was closed is not known"
		return nil
	}
	rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("an unknown answer that closed: %d %s", rec.Code, rec.Body.String())
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByAdministrator {
		t.Fatalf("the record after an unknown answer: %+v", marker)
	}
	// The undo still has its record.
	f.agent.close = nil
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "undo", "database_id": 9982}); rec.Code != http.StatusOK || len(f.agent.undos) != 1 {
		t.Fatalf("undo after an unknown answer: %d %s", rec.Code, rec.Body.String())
	}

	// An unknown answer that did not close: a failure with its reason.
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		out.Unknown, out.Error = true, `the database server's answer ended inside the close: ERROR: role "reader" does not exist`
		return nil
	}
	rec = pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9981, "acl": "", "roles": []string{"admin_user", "reader"}})
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), errCodePGPublicFailed) || !strings.Contains(rec.Body.String(), `role \"reader\" does not exist`) {
		t.Fatalf("an unknown answer that changed nothing: %d %s", rec.Code, rec.Body.String())
	}
	marker, _ = f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || marker.State != pgPublicWatch || marker.FailedAt == "" || marker.FailedBy != pgPublicByAdministrator {
		t.Fatalf("the failed close's record: %+v", marker)
	}
}

func TestPGPublicALostAnswerIsSettledFromTheEngineUnderTheSameConditions(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	// The panel's own close: the answer is lost and the engine did it.
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, _ *transport.PostgreSQLPublicAccessCloseResponse) error {
		if in.Name == "clean_db" {
			f.agent.closeOnEngine(in.Name)
		}
		return errors.New("connection reset")
	}
	view := f.round(t)
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || !reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) || pgPublicEntryOf(view.Closed, "clean_db") == nil {
		t.Fatalf("a lost answer the engine did: %+v", marker)
	}
	// admin_tools: lost, and the engine did nothing. The same conditions as
	// a first close hold, so it is closed again; first a foreign role
	// appears, and it is not.
	tools := f.agent.facts["admin_tools"]
	tools.Roles = append(tools.Roles, "intruder")
	f.agent.facts["admin_tools"] = tools
	f.agent.close = nil
	sent := f.agent.closesOf("admin_tools")
	view = f.round(t)
	if f.agent.closesOf("admin_tools") != sent {
		t.Fatal("a lost close was done again although a foreign role appeared")
	}
	if entry := pgPublicEntryOf(view.Open, "admin_tools"); entry == nil || !containsString(entry.Found, "other_roles") {
		t.Fatalf("admin_tools after a lost answer: %+v", entry)
	}

	// An administrator's close whose answer was lost and which did nothing
	// is not redone by the round: it is a failure the administrator sees.
	f.agent.close = func(*transport.PostgreSQLPublicAccessCloseRequest, *transport.PostgreSQLPublicAccessCloseResponse) error {
		return errors.New("connection reset")
	}
	rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), errCodePGPublicFailed) {
		t.Fatalf("an administrator's lost close: %d %s", rec.Code, rec.Body.String())
	}
	f.agent.close = nil
	sent = f.agent.closesOf("foreign_db")
	view = f.round(t)
	entry := pgPublicEntryOf(view.Open, "foreign_db")
	if f.agent.closesOf("foreign_db") != sent || entry == nil || entry.Found[0] != "close_failed" || entry.Failure == "" {
		t.Fatalf("the administrator's lost close after a round: %+v", entry)
	}
}

func TestPGPublicTwoClosesAtOnceActOnce(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	f.agent.closeGate, f.agent.gateName = make(chan struct{}), "clean_db"
	roundDone := make(chan pgPublicView)
	go func() {
		f.clock.advance(time.Hour)
		view, _ := f.panel.pgPublicRead(context.Background(), pgPublicNow(), pgPublicPass{write: true, round: true, closeNow: true})
		roundDone <- view
	}()
	// The round's first close is held; the administrator presses Close on
	// the same database meanwhile.
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
	pressed := make(chan *httptest.ResponseRecorder)
	go func() {
		pressed <- pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9981, "acl": "", "roles": []string{"admin_user", "reader"}})
	}()
	time.Sleep(50 * time.Millisecond)
	close(f.agent.closeGate)
	<-roundDone
	rec := <-pressed
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"closed"`) {
		t.Fatalf("the administrator's press: %d %s", rec.Code, rec.Body.String())
	}
	if n := f.agent.closesOf("clean_db"); n != 1 {
		t.Fatalf("clean_db was closed %d times", n)
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9981)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || !reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) {
		t.Fatalf("the winner's record: %+v", marker)
	}
}

func TestPGPublicAFailedCloseIsShownAndNotRetriedEveryHour(t *testing.T) {
	f := pgPublicFixture(t)
	for round := 1; round <= pgPublicWatchRounds; round++ {
		f.round(t)
	}
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		out.Error = "the database server did not close it; nothing was changed: ERROR: permission denied"
		return nil
	}
	view := f.round(t)
	entry := pgPublicEntryOf(view.Open, "clean_db")
	if entry == nil || entry.Found[0] != "close_failed" || !strings.Contains(entry.Failure, "permission denied") || entry.FailedAt == "" {
		t.Fatalf("a failed close: %+v", entry)
	}
	if names := pgPublicProblemNames(t, f); !strings.Contains(names, "clean_db") {
		t.Fatalf("Problems now: %q", names)
	}
	sent := f.agent.closesOf("clean_db")
	for round := 1; round <= 5; round++ {
		f.round(t)
	}
	if f.agent.closesOf("clean_db") != sent {
		t.Fatal("a failed close was tried again every hour")
	}
	// The administrator presses: tried again.
	f.agent.close = nil
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9981, "acl": "", "roles": []string{"admin_user", "reader"}}); rec.Code != http.StatusOK {
		t.Fatalf("the administrator's close after a failure: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPGPublicAFailedCloseIsTriedAgainWhenSomethingChanged(t *testing.T) {
	f := pgPublicFixture(t)
	f.agent.close = func(in *transport.PostgreSQLPublicAccessCloseRequest, out *transport.PostgreSQLPublicAccessCloseResponse) error {
		out.Error = "the database server did not close it; nothing was changed"
		return nil
	}
	for round := 1; round <= pgPublicWatchRounds+1; round++ {
		f.round(t)
	}
	if f.agent.closesOf("clean_db") != 1 {
		t.Fatalf("closes: %d", f.agent.closesOf("clean_db"))
	}
	f.agent.close = nil
	// The administrator removes the read user from the database: the facts
	// changed, the watch starts again and closes after another day.
	f.exec(t, `DELETE FROM database_user_grants WHERE database_id = 9981 AND user_id = 9984`)
	clean := f.agent.facts["clean_db"]
	clean.Roles = []string{"admin_user"}
	f.agent.facts["clean_db"] = clean
	for round := 1; round <= pgPublicWatchRounds+1; round++ {
		f.round(t)
	}
	if f.agent.closesOf("clean_db") != 2 {
		t.Fatalf("a changed database was not tried again: %d", f.agent.closesOf("clean_db"))
	}
}

func TestPGPublicARecordedUserWithoutItsRoleAsksTheAdministrator(t *testing.T) {
	f := pgPublicFixture(t)
	clean := f.agent.facts["clean_db"]
	clean.Missing = []string{"reader"}
	f.agent.facts["clean_db"] = clean
	var view pgPublicView
	for round := 1; round <= pgPublicWatchRounds+2; round++ {
		view = f.round(t)
	}
	if f.agent.closesOf("clean_db") != 0 {
		t.Fatal("closed by itself with a recorded user's role missing")
	}
	if entry := pgPublicEntryOf(view.Open, "clean_db"); entry == nil || !reflect.DeepEqual(entry.Found, []string{"missing_users"}) || !reflect.DeepEqual(entry.Missing, []string{"reader"}) {
		t.Fatalf("listed: %+v", entry)
	}
}

func TestPGPublicTheAdministratorClosesAndUndoesWhatWasListed(t *testing.T) {
	f := pgPublicFixture(t)
	for _, who := range []string{"reseller", "customer", "outsider", "member"} {
		if rec := pgPublicPost(t, f.visibleDatabasesFixture, who, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("%s read the list: %d", who, rec.Code)
		}
		if rec := pgPublicPost(t, f.visibleDatabasesFixture, who, map[string]any{"action": "close", "database_id": 9982}); rec.Code != http.StatusForbidden || len(f.agent.closes) != 0 {
			t.Fatalf("%s closed a database: %d", who, rec.Code)
		}
	}
	rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", nil)
	var view pgPublicView
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &view) != nil || len(view.Open) != 3 || view.CheckedAt == "" {
		t.Fatalf("the administrator's list: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.agent.closes) != 0 {
		t.Fatal("reading the list closed a database")
	}

	// What the administrator read no longer holds: refused, nothing changed.
	rec = pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user"}})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodePGPublicChanged) {
		t.Fatalf("a close of what changed: %d %s", rec.Code, rec.Body.String())
	}
	if marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982); marker != nil {
		t.Fatalf("a refused close left a record: %+v", marker)
	}

	rec = pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("the administrator's close: %d %s", rec.Code, rec.Body.String())
	}
	last := f.agent.closes[len(f.agent.closes)-1]
	if last.NoOtherSessions || last.Name != "foreign_db" || len(last.Grants) != 0 {
		t.Fatalf("the administrator's close = %+v (the owner admin_user is its only recorded user)", last)
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByAdministrator {
		t.Fatalf("record = %+v", marker)
	}
	if names := pgPublicProblemNames(t, f); names != "" {
		t.Fatalf("Problems now still lists a closed database: %q", names)
	}

	rec = pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "undo", "database_id": 9982})
	if rec.Code != http.StatusOK || len(f.agent.undos) != 1 {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	undo := f.agent.undos[0]
	if undo.ExpectACL != "{admin_user=CTc/admin_user}" || !undo.PublicConnect || !undo.PublicTemporary || len(undo.Added) != 0 {
		t.Fatalf("undo request = %+v", undo)
	}
	marker, _ = f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicUndone {
		t.Fatalf("after undo: %+v", marker)
	}
}

func TestPGPublicADatabaseTheAdministratorOpenedAgainIsTheAdministratorsChoice(t *testing.T) {
	f := pgPublicFixture(t)
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}}); rec.Code != http.StatusOK {
		t.Fatalf("close: %d %s", rec.Code, rec.Body.String())
	}
	if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "undo", "database_id": 9982}); rec.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	var view pgPublicView
	for round := 1; round <= pgPublicWatchRounds+2; round++ {
		view = f.round(t)
	}
	if f.agent.closesOf("foreign_db") != 1 {
		t.Fatal("an undone database was closed again by itself")
	}
	reopened := pgPublicEntryOf(view.Reopened, "foreign_db")
	if reopened == nil || pgPublicEntryOf(view.Open, "foreign_db") != nil || reopened.By != pgPublicByAdministrator || !reopened.PublicConnect || !reopened.PublicTemporary {
		t.Fatalf("the reopened database: %+v, open %+v", reopened, view.Open)
	}
	if names := pgPublicProblemNames(t, f); strings.Contains(names, "foreign_db") {
		t.Fatalf("Problems now lists the administrator's choice: %q", names)
	}
}

func TestPGPublicThePageReadsTheLastScan(t *testing.T) {
	f := pgPublicFixture(t)
	for i := 0; i < 3; i++ {
		if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", nil); rec.Code != http.StatusOK {
			t.Fatalf("read %d: %d", i, rec.Code)
		}
	}
	if f.agent.scans != 1 {
		t.Fatalf("three reads of the page made %d scans", f.agent.scans)
	}
	// Read again: one scan for the presses that come while it runs.
	f.agent.scanGate = make(chan struct{})
	var wg sync.WaitGroup
	press := func() {
		defer wg.Done()
		if rec := pgPublicPost(t, f.visibleDatabasesFixture, "admin", map[string]any{"action": "read"}); rec.Code != http.StatusOK {
			t.Errorf("read again: %d %s", rec.Code, rec.Body.String())
		}
	}
	waiters := func() int {
		pgPublicScans.Lock()
		defer pgPublicScans.Unlock()
		if flight := pgPublicScans.running[f.panel]; flight != nil {
			return flight.waiters
		}
		return -1
	}
	wg.Add(1)
	go press()
	deadline := time.Now().Add(10 * time.Second)
	for waiters() < 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	wg.Add(2)
	go press()
	go press()
	for waiters() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	close(f.agent.scanGate)
	wg.Wait()
	if f.agent.scans != 2 {
		t.Fatalf("three presses of Read again made %d scans in all", f.agent.scans)
	}
}

func pgPublicPost(t *testing.T, f visibleDatabasesFixture, who string, body any) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(pgPublicPath, f.panel.handlePostgreSQLPublicAccess)
	method, reader := http.MethodGet, &bytes.Reader{}
	if body != nil {
		raw, _ := json.Marshal(body)
		method, reader = http.MethodPost, bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, pgPublicPath, reader)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[who]})
	recorder := httptest.NewRecorder()
	f.panel.requireAuth(mux).ServeHTTP(recorder, request)
	return recorder
}
