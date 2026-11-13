package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
)

// alpha.134: rights by exact name. The panel proves from its records which
// pattern-form grants are CelikPanel's and rewrites only those, one at a
// time, after an update is verified; everything else is listed. The engine
// is faked here (internal/mariadbgrant has the statement-level tests; the
// real engines are the lab's).

// fakeGrantEngine is one MariaDB as the loop sees it: its pattern rows and
// what each convert did.
type fakeGrantEngine struct {
	scanResult  mariadbgrant.Scan
	remote      map[string][]string
	scanErr     error
	convertErr  error
	converted   []mariadbgrant.ConvertRequest
	outcomeFor  func(mariadbgrant.ConvertRequest) string
	afterSwitch func()
}

func (f *fakeGrantEngine) scan(context.Context) (mariadbgrant.Scan, map[string][]string, error) {
	return f.scanResult, f.remote, f.scanErr
}

func (f *fakeGrantEngine) convert(_ context.Context, req mariadbgrant.ConvertRequest) (string, error) {
	if f.convertErr != nil {
		return "", f.convertErr
	}
	f.converted = append(f.converted, req)
	// A converted row is gone from the next read.
	kept := f.scanResult.Patterns[:0]
	for _, row := range f.scanResult.Patterns {
		if !(row.User == req.User && row.Host == req.Host && row.Stored == req.Database) {
			kept = append(kept, row)
		}
	}
	f.scanResult.Patterns = kept
	if f.outcomeFor != nil {
		return f.outcomeFor(req), nil
	}
	return mariadbgrant.Converted, nil
}

const allFlags = "YYYYYYNYYYYYYYYYYYYYY"

func grantFormsFixture(t *testing.T) (visibleDatabasesFixture, *fakeGrantEngine) {
	t.Helper()
	f := newVisibleDatabasesFixture(t)
	engine := &fakeGrantEngine{scanResult: mariadbgrant.Scan{Version: "11.8.6-MariaDB-0+deb13u1", MariaDB: true, Patterns: []mariadbgrant.Row{
		// CelikPanel's: cust_user on customer_matrix_test_shop (recorded),
		// on localhost; its pattern also covers another customer's database.
		{User: "cust_user", Host: "localhost", Stored: "customer_matrix_test_shop", Flags: allFlags, Privileges: "ALL PRIVILEGES",
			Reached: []string{"customer1matrix_test_shop"}},
		// CelikPanel's remote copy of wp_user (recorded by the Agent).
		{User: "wp_user", Host: "203.0.113.7", Stored: "customer_matrix_test_blog", Flags: allFlags, Privileges: "ALL PRIVILEGES"},
		// A hand-made account: not provable, listed.
		{User: "hand", Host: "%", Stored: "customer_matrix_test_shop", Flags: allFlags, Privileges: "ALL PRIVILEGES"},
		// A recorded pair on a host CelikPanel never made: listed.
		{User: "cust_user", Host: "10.%", Stored: "customer_matrix_test_shop", Flags: allFlags, Privileges: "ALL PRIVILEGES"},
		// A recorded pair holding the grant option: the owner's.
		{User: "other_user", Host: "localhost", Stored: "other_shop", Flags: allFlags, Privileges: "ALL PRIVILEGES", GrantOption: true},
	}}, remote: map[string][]string{"wp_user": {"203.0.113.7"}}}
	saved := []any{databaseGrantFormsLocal, databaseGrantFormsUpdatePhase, databaseGrantFormsHostQuiet, databaseGrantFormsNow,
		databaseGrantFormsCommit, databaseGrantFormsRemote}
	databaseGrantFormsLocal = func(*Panel) grantFormsEngineAccess { return engine }
	databaseGrantFormsCommit = func() string { return strings.Repeat("a", 40) }
	// The fixture's registered MariaDB (another machine) has nothing to do.
	databaseGrantFormsRemote = func(*Panel, *core.DatabaseServer) (grantFormsEngineAccess, error) {
		return &fakeGrantEngine{scanResult: mariadbgrant.Scan{Version: "10.11.14-MariaDB", MariaDB: true}}, nil
	}
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "succeeded", true }
	databaseGrantFormsHostQuiet = func(*Panel, context.Context) bool { return true }
	t.Cleanup(func() {
		databaseGrantFormsLocal = saved[0].(func(*Panel) grantFormsEngineAccess)
		databaseGrantFormsUpdatePhase = saved[1].(func(string) (string, bool))
		databaseGrantFormsHostQuiet = saved[2].(func(*Panel, context.Context) bool)
		databaseGrantFormsNow = saved[3].(func() time.Time)
		databaseGrantFormsCommit = saved[4].(func() string)
		databaseGrantFormsRemote = saved[5].(func(*Panel, *core.DatabaseServer) (grantFormsEngineAccess, error))
	})
	return f, engine
}

func longAgo() time.Time { return time.Now().Add(-time.Hour) }

func grantFormRowState(t *testing.T, f visibleDatabasesFixture, user, host, stored string) (kind, state, reason string) {
	t.Helper()
	if err := f.panel.db.GetDB().QueryRow(`SELECT kind, state, reason FROM database_grant_forms WHERE username = ? AND host = ? AND stored_name = ?`,
		user, host, stored).Scan(&kind, &state, &reason); err != nil {
		t.Fatalf("%s@%s %s: %v", user, host, stored, err)
	}
	return
}

func grantFormEngineState(t *testing.T, f visibleDatabasesFixture) (state, reason string) {
	t.Helper()
	if err := f.panel.db.GetDB().QueryRow(`SELECT state, reason FROM database_grant_form_engines WHERE engine_key = 'mariadb@this-machine:3306'`).Scan(&state, &reason); err != nil {
		t.Fatal(err)
	}
	return
}

// While an update of the panel is not verified, the round reads and writes
// the before-images but changes nothing on the engine.
func TestGrantFormsWaitForAVerifiedUpdate(t *testing.T) {
	f, engine := grantFormsFixture(t)
	for _, phase := range []string{"running", "recovering", "failed"} {
		databaseGrantFormsUpdatePhase = func(string) (string, bool) { return phase, true }
		f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
		if len(engine.converted) != 0 {
			t.Fatalf("%s: the engine was changed", phase)
		}
		if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonUpdate {
			t.Fatalf("%s: engine %s/%s", phase, state, reason)
		}
	}
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "", false }
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonUpdateUnknown || len(engine.converted) != 0 {
		t.Fatalf("unknown update: engine %s/%s, %d converted", state, reason, len(engine.converted))
	}
	// Within the start-up window too.
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "succeeded", true }
	f.panel.runDatabaseGrantFormsRound(context.Background(), time.Now())
	if state, reason := grantFormEngineState(t, f); reason != grantFormsReasonStarting || state != grantFormsWaiting || len(engine.converted) != 0 {
		t.Fatalf("start-up: engine %s/%s", state, reason)
	}
	// The before-image is there already.
	if kind, state, _ := grantFormRowState(t, f, "cust_user", "localhost", "customer_matrix_test_shop"); kind != "ours" || state != "captured" {
		t.Fatalf("before-image %s/%s", kind, state)
	}
}

// Once the update is verified: only the rows proven CelikPanel's are
// rewritten, with the captured flags; the others are listed; the
// administrators are told once.
func TestGrantFormsRewriteOnlyWhatTheRecordsProve(t *testing.T) {
	f, engine := grantFormsFixture(t)
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	got := map[string]bool{}
	for _, req := range engine.converted {
		got[req.User+"@"+req.Host+" "+req.Database] = true
		if req.Flags != allFlags {
			t.Fatalf("convert without the captured flags: %+v", req)
		}
	}
	if len(got) != 2 || !got["cust_user@localhost customer_matrix_test_shop"] || !got["wp_user@203.0.113.7 customer_matrix_test_blog"] {
		t.Fatalf("converted %v", got)
	}
	for _, c := range []struct{ user, host, stored, kind, state string }{
		{"cust_user", "localhost", "customer_matrix_test_shop", "ours", "done"},
		{"wp_user", "203.0.113.7", "customer_matrix_test_blog", "ours", "done"},
		{"hand", "%", "customer_matrix_test_shop", "not_ours", "left"},
		{"cust_user", "10.%", "customer_matrix_test_shop", "not_ours", "left"},
		{"other_user", "localhost", "other_shop", "owner_row", "left"},
	} {
		if kind, state, _ := grantFormRowState(t, f, c.user, c.host, c.stored); kind != c.kind || state != c.state {
			t.Fatalf("%s@%s %s: %s/%s, want %s/%s", c.user, c.host, c.stored, kind, state, c.kind, c.state)
		}
	}
	if state, _ := grantFormEngineState(t, f); state != grantFormsDone {
		t.Fatalf("engine %s", state)
	}
	notices := func() int {
		var n int
		_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM account_notices WHERE kind = ?`, noticeDatabasePatternRights).Scan(&n)
		return n
	}
	first := notices()
	if first == 0 {
		t.Fatal("the administrators were not told about the rows left as they are")
	}
	// A second round: nothing more to rewrite, no second notification.
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if len(engine.converted) != 2 || notices() != first {
		t.Fatalf("second round: %d converted, %d notices (first %d)", len(engine.converted), notices(), first)
	}

	// The administrator's view says what was narrowed, with what it reached,
	// and what is left, with the statement under Details; nobody else sees it.
	recorder := grantFormsRequest(t, f, "admin")
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", recorder.Code, recorder.Body.String())
	}
	var view grantFormsView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Engines) != 2 || view.Engines[0].Narrowed != 2 || view.Engines[0].Left != 3 || view.Engines[0].Waiting != 0 || view.Engines[0].State != grantFormsDone {
		t.Fatalf("engines %+v", view.Engines)
	}
	if len(view.Narrowed) != 1 || view.Narrowed[0].Account != "cust_user@localhost" || strings.Join(view.Narrowed[0].Reached, ",") != "customer1matrix_test_shop" {
		t.Fatalf("narrowed %+v", view.Narrowed)
	}
	var hand *grantFormsRowView
	for i := range view.Left {
		if view.Left[i].Account == "hand@%" {
			hand = &view.Left[i]
		}
	}
	if hand == nil || !strings.Contains(hand.Statement, "GRANT ALL PRIVILEGES ON `customer\\_matrix\\_test\\_shop`.* TO 'hand'@'%';") {
		t.Fatalf("left %+v", view.Left)
	}
	for _, who := range []string{"customer", "reseller", "member"} {
		if code := grantFormsRequest(t, f, who).Code; code != http.StatusForbidden {
			t.Fatalf("%s got %d", who, code)
		}
	}
}

// An engine whose Agent is another build waits; one that fails to convert
// three times is blocked and keeps its before-image; an engine that is not
// MariaDB is only listed.
func TestGrantFormsWaitBlockAndListOnly(t *testing.T) {
	f, engine := grantFormsFixture(t)
	engine.scanErr = errGrantFormsBuildMismatch
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonAgentVersion {
		t.Fatalf("mismatch: %s/%s", state, reason)
	}
	engine.scanErr = &grantFormsAgentError{errors.New("rpc: can't find method Agent.DatabaseGrantFormsScanV1")}
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, f); state != grantFormsWaiting || reason != grantFormsReasonAgentVersion {
		t.Fatalf("older Agent: %s/%s", state, reason)
	}
	engine.scanErr = nil
	engine.convertErr = errors.New("the database server did not write the exact right")
	for i := 0; i < grantFormsRowFailuresToBlock; i++ {
		f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	}
	if _, state, reason := grantFormRowState(t, f, "cust_user", "localhost", "customer_matrix_test_shop"); state != "blocked" || reason != grantFormsReasonConvertFailed {
		t.Fatalf("row %s/%s", state, reason)
	}
	if state, reason := grantFormEngineState(t, f); state != grantFormsBlocked || reason != grantFormsReasonConvertFailed {
		t.Fatalf("engine %s/%s", state, reason)
	}
	// It is not tried again within the day, and is after it (the cause gone).
	engine.convertErr = nil
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if len(engine.converted) != 0 {
		t.Fatal("a blocked row was tried again within the day")
	}
	databaseGrantFormsNow = func() time.Time { return time.Now().Add(25 * time.Hour) }
	f.panel.runDatabaseGrantFormsRound(context.Background(), time.Now().Add(-time.Hour))
	if _, state, _ := grantFormRowState(t, f, "cust_user", "localhost", "customer_matrix_test_shop"); state != "done" {
		t.Fatalf("after the day: %s", state)
	}

	g, other := grantFormsFixture(t)
	other.scanResult.MariaDB, other.scanResult.Version = false, "8.0.36"
	g.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if state, reason := grantFormEngineState(t, g); state != grantFormsListOnly || reason != grantFormsReasonNotMariaDB || len(other.converted) != 0 {
		t.Fatalf("MySQL: %s/%s, %d converted", state, reason, len(other.converted))
	}
	if _, state, reason := grantFormRowState(t, g, "cust_user", "localhost", "customer_matrix_test_shop"); state != "left" || reason != grantFormsReasonNotMariaDB {
		t.Fatalf("MySQL row %s/%s", state, reason)
	}
}

// A row captured, then rewritten on the engine while the panel stopped
// before it recorded the result: the next round asks the engine, which
// answers it is already exact, and records it done.
func TestGrantFormsInterruptedAfterTheEngineChangedIsFinishedFromTheEngine(t *testing.T) {
	f, engine := grantFormsFixture(t)
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "running", true }
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	// The engine was changed (by a run that was cut off): the pattern row is
	// gone from its read.
	engine.scanResult.Patterns = engine.scanResult.Patterns[1:]
	engine.outcomeFor = func(req mariadbgrant.ConvertRequest) string {
		if req.User == "cust_user" {
			return mariadbgrant.AlreadyExact
		}
		return mariadbgrant.Converted
	}
	databaseGrantFormsUpdatePhase = func(string) (string, bool) { return "succeeded", true }
	f.panel.runDatabaseGrantFormsRound(context.Background(), longAgo())
	if _, state, reason := grantFormRowState(t, f, "cust_user", "localhost", "customer_matrix_test_shop"); state != "done" || reason != mariadbgrant.AlreadyExact {
		t.Fatalf("%s/%s", state, reason)
	}
}

// grantFormsRequest reads the administrator's view as who.
func grantFormsRequest(t *testing.T, f visibleDatabasesFixture, who string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(databaseGrantFormsPath, f.panel.handleDatabaseGrantForms)
	request := httptest.NewRequest(http.MethodGet, databaseGrantFormsPath, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[who]})
	recorder := httptest.NewRecorder()
	f.panel.requireAuth(mux).ServeHTTP(recorder, request)
	return recorder
}
