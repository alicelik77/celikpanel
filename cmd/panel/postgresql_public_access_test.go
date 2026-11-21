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
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S1b (20 November 2026): PostgreSQL databases made before alpha.141 are
// closed to other accounts by themselves only when their rights are the
// untouched default and nothing inside belongs to a role outside the
// records; the rest is listed, closed by the administrator, and every close
// can be undone exactly.

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
	entry := pgPublicDecide(record, clean, nil, nil)
	if len(entry.Found) != 0 {
		t.Fatalf("an untouched database found %v", entry.Found)
	}
	if !reflect.DeepEqual(entry.Grants, []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}, {Role: "writer", Temporary: true}}) {
		t.Fatalf("grants (the owner holds its rights as owner) = %+v", entry.Grants)
	}
	for _, c := range []struct {
		name       string
		facts      func(*transport.PostgreSQLPublicAccessFacts)
		monitoring []string
		marker     *pgPublicMarker
		want       string
	}{
		{"changed rights", func(f *transport.PostgreSQLPublicAccessFacts) { f.ACL = "{=Tc/shop_owner,other=c/shop_owner}" }, nil, nil, "changed_rights"},
		{"a foreign role inside", func(f *transport.PostgreSQLPublicAccessFacts) { f.Roles = append(f.Roles, "other_customer") }, nil, nil, "other_roles"},
		{"a foreign session", func(f *transport.PostgreSQLPublicAccessFacts) { f.Sessions = []string{"other_customer"} }, nil, nil, "other_sessions"},
		{"a monitoring role", func(*transport.PostgreSQLPublicAccessFacts) {}, []string{"metrics"}, nil, "monitoring_roles"},
		{"undone before", func(*transport.PostgreSQLPublicAccessFacts) {}, nil, &pgPublicMarker{State: pgPublicUndone}, "undone"},
	} {
		facts := clean
		facts.Roles = append([]string{}, clean.Roles...)
		c.facts(&facts)
		if entry := pgPublicDecide(record, facts, c.monitoring, c.marker); !containsString(entry.Found, c.want) {
			t.Fatalf("%s: found %v, want %s", c.name, entry.Found, c.want)
		}
	}
}

// pgPublicFakeAgent answers the three calls from what each database holds.
type pgPublicFakeAgent struct {
	facts      map[string]transport.PostgreSQLPublicAccessFacts
	monitoring []string
	closes     []transport.PostgreSQLPublicAccessCloseRequest
	undos      []transport.PostgreSQLPublicAccessUndoRequest
	loseClose  bool
	refuse     bool
}

func (a *pgPublicFakeAgent) call(_ *Panel, _ context.Context, method string, req, resp any) error {
	switch method {
	case "Agent.PostgreSQLPublicAccessScanV1":
		in, out := req.(*transport.PostgreSQLPublicAccessScanRequest), resp.(*transport.PostgreSQLPublicAccessScanResponse)
		out.Monitoring = a.monitoring
		for _, database := range in.Databases {
			f, ok := a.facts[database.Name]
			if !ok {
				f = transport.PostgreSQLPublicAccessFacts{Name: database.Name}
			}
			out.Databases = append(out.Databases, f)
		}
	case "Agent.PostgreSQLPublicAccessCloseV1":
		in, out := req.(*transport.PostgreSQLPublicAccessCloseRequest), resp.(*transport.PostgreSQLPublicAccessCloseResponse)
		a.closes = append(a.closes, *in)
		if a.loseClose {
			return errors.New("connection reset")
		}
		f := a.facts[in.Name]
		if a.refuse || f.ACL != in.ExpectACL || !reflect.DeepEqual(nonNilStrings(f.Roles), in.ExpectRoles) {
			out.Changed, out.Error = true, "the database is no longer as it was read; nothing was changed"
			return nil
		}
		f.ACL, f.PublicConnect, f.PublicTemporary = "{owner=CTc/owner}", false, false
		a.facts[in.Name] = f
		out.Closed, out.Change = true, &transport.PostgreSQLPublicAccessChange{BeforeACL: in.ExpectACL, AfterACL: f.ACL, Added: []string{}}
		for _, grant := range in.Grants {
			out.Change.Added = append(out.Change.Added, grant.Role+":CONNECT")
		}
	case "Agent.PostgreSQLPublicAccessUndoV1":
		in, out := req.(*transport.PostgreSQLPublicAccessUndoRequest), resp.(*transport.PostgreSQLPublicAccessUndoResponse)
		a.undos = append(a.undos, *in)
		f := a.facts[in.Name]
		f.ACL, f.PublicConnect, f.PublicTemporary = "{=Tc/owner,owner=CTc/owner}", true, true
		a.facts[in.Name] = f
		out.Undone, out.AfterACL = true, f.ACL
	default:
		return errors.New("unexpected call " + method)
	}
	return nil
}

func pgPublicFixture(t *testing.T) (visibleDatabasesFixture, *pgPublicFakeAgent) {
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
	saved := pgPublicAgent
	pgPublicAgent = agent.call
	t.Cleanup(func() { pgPublicAgent = saved })
	return f, agent
}

func TestPGPublicClosesByItselfOnlyUntouchedDatabasesAndListsTheRest(t *testing.T) {
	f, agent := pgPublicFixture(t)
	ctx := context.Background()

	// Without the admission nothing is changed.
	view, err := f.panel.pgPublicView(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(agent.closes) != 0 || len(view.Open) != 3 {
		t.Fatalf("a read changed something or missed one: closes %d, open %+v", len(agent.closes), view.Open)
	}
	f.panel.storePGPublicList(ctx, view)
	if problems := pgPublicProblems(ctx, f.panel.db.GetDB()); len(problems) != 1 || problems[0].Name != "foreign_db" || problems[0].Value != 1 {
		t.Fatalf("Problems now lists only what the administrator decides: %+v", problems)
	}

	view, err = f.panel.pgPublicView(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	closed := map[string]transport.PostgreSQLPublicAccessCloseRequest{}
	for _, c := range agent.closes {
		closed[c.Name] = c
	}
	if len(closed) != 2 || closed["foreign_db"].Name != "" {
		t.Fatalf("closed by itself: %+v", agent.closes)
	}
	clean := closed["clean_db"]
	if !clean.NoOtherSessions || clean.ExpectACL != "" || !reflect.DeepEqual(clean.Grants, []transport.PostgreSQLPublicAccessGrant{{Role: "reader"}}) {
		t.Fatalf("clean_db's close = %+v (the owner admin_user is given nothing; reader CONNECT only)", clean)
	}
	if len(view.Open) != 1 || view.Open[0].Name != "foreign_db" || !reflect.DeepEqual(view.Open[0].OtherRoles, []string{"other_customer"}) {
		t.Fatalf("left open: %+v", view.Open)
	}
	if len(view.Closed) != 2 {
		t.Fatalf("closed list: %+v", view.Closed)
	}
	marker, err := f.panel.readPGPublicMarker(ctx, 9981)
	if err != nil || marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByPanel || !marker.PublicConnect || !marker.PublicTemporary ||
		!reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) {
		t.Fatalf("clean_db's record = %+v, %v", marker, err)
	}

	// The next round changes nothing more.
	before := len(agent.closes)
	if _, err := f.panel.pgPublicView(ctx, true); err != nil || len(agent.closes) != before {
		t.Fatalf("a second round closed again: %d -> %d (%v)", before, len(agent.closes), err)
	}
}

func TestPGPublicALostAnswerIsFinishedFromTheEngine(t *testing.T) {
	f, agent := pgPublicFixture(t)
	ctx := context.Background()
	agent.loseClose = true
	if _, err := f.panel.pgPublicView(ctx, true); err != nil {
		t.Fatal(err)
	}
	marker, _ := f.panel.readPGPublicMarker(ctx, 9981)
	if marker == nil || marker.State != pgPublicSending {
		t.Fatalf("the close is recorded before it is sent: %+v", marker)
	}
	// The engine did it; the answer was lost.
	facts := agent.facts["clean_db"]
	facts.ACL, facts.PublicConnect, facts.PublicTemporary = "{admin_user=CTc/admin_user,reader=c/admin_user}", false, false
	agent.facts["clean_db"] = facts
	agent.loseClose = false
	sent := len(agent.closes)
	if _, err := f.panel.pgPublicView(ctx, false); err != nil {
		t.Fatal(err)
	}
	marker, _ = f.panel.readPGPublicMarker(ctx, 9981)
	if marker == nil || marker.State != pgPublicClosed || !reflect.DeepEqual(marker.Added, []string{"reader:CONNECT"}) || len(agent.closes) != sent {
		t.Fatalf("finished from the engine: %+v (closes %d -> %d)", marker, sent, len(agent.closes))
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

func TestPGPublicTheAdministratorClosesAndUndoesWhatWasListed(t *testing.T) {
	f, agent := pgPublicFixture(t)
	for _, who := range []string{"reseller", "customer", "outsider", "member"} {
		if rec := pgPublicPost(t, f, who, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("%s read the list: %d", who, rec.Code)
		}
		if rec := pgPublicPost(t, f, who, map[string]any{"action": "close", "database_id": 9982}); rec.Code != http.StatusForbidden || len(agent.closes) != 0 {
			t.Fatalf("%s closed a database: %d", who, rec.Code)
		}
	}
	rec := pgPublicPost(t, f, "admin", nil)
	var view pgPublicView
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &view) != nil || len(view.Open) != 3 {
		t.Fatalf("the administrator's list: %d %s", rec.Code, rec.Body.String())
	}
	if len(agent.closes) != 0 {
		t.Fatal("reading the list closed a database")
	}

	// What the administrator read no longer holds: refused, nothing changed.
	rec = pgPublicPost(t, f, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user"}})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodePGPublicChanged) {
		t.Fatalf("a close of what changed: %d %s", rec.Code, rec.Body.String())
	}
	if marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982); marker != nil {
		t.Fatalf("a refused close left a record: %+v", marker)
	}

	rec = pgPublicPost(t, f, "admin", map[string]any{"action": "close", "database_id": 9982, "acl": "", "roles": []string{"admin_user", "other_customer"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("the administrator's close: %d %s", rec.Code, rec.Body.String())
	}
	last := agent.closes[len(agent.closes)-1]
	if last.NoOtherSessions || last.Name != "foreign_db" || len(last.Grants) != 0 {
		t.Fatalf("the administrator's close = %+v (the owner admin_user is its only recorded user)", last)
	}
	marker, _ := f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicClosed || marker.By != pgPublicByAdministrator {
		t.Fatalf("record = %+v", marker)
	}
	if problems := pgPublicProblems(context.Background(), f.panel.db.GetDB()); len(problems) != 0 {
		t.Fatalf("Problems now still lists a closed database: %+v", problems)
	}

	rec = pgPublicPost(t, f, "admin", map[string]any{"action": "undo", "database_id": 9982})
	if rec.Code != http.StatusOK || len(agent.undos) != 1 {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	undo := agent.undos[0]
	if undo.ExpectACL != "{owner=CTc/owner}" || !undo.PublicConnect || !undo.PublicTemporary || len(undo.Added) != 0 {
		t.Fatalf("undo request = %+v", undo)
	}
	marker, _ = f.panel.readPGPublicMarker(context.Background(), 9982)
	if marker == nil || marker.State != pgPublicUndone {
		t.Fatalf("after undo: %+v", marker)
	}
	// An undone database is never closed again by itself.
	sent := len(agent.closes)
	view, err := f.panel.pgPublicView(context.Background(), true)
	if err != nil || len(agent.closes) != sent+2 {
		// admin_tools and clean_db close by themselves; foreign_db stays.
		t.Fatalf("round after undo: %v, closes %d -> %d", err, sent, len(agent.closes))
	}
	for _, c := range agent.closes[sent:] {
		if c.Name == "foreign_db" {
			t.Fatal("an undone database was closed again by itself")
		}
	}
	if len(view.Open) != 1 || !containsString(view.Open[0].Found, "undone") {
		t.Fatalf("the undone database is listed: %+v", view.Open)
	}
}
