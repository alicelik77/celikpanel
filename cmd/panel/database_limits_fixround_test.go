package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149: the fix round after the independent review of PR #370
// (findings 3 to 13 and 19). Each test fails without its fix.

// Finding 3: the server owner's hand-set PostgreSQL connection limit is kept
// (the reviewer's probe: 25 became -1 when only the statement time changed).
func TestTheOwnersPostgreSQLConnectionLimitIsKept(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'cl_u', 'x'), (9961, 9802, 'cl_new', 'x')`)
	agent.conn["cl_u"] = 25 // the owner's, by hand
	setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if agent.conn["cl_u"] != 25 {
		t.Fatalf("the owner's connection limit became %d", agent.conn["cl_u"])
	}
	// The plan sets a limit: still the owner's; the card names it.
	setPlanDBLimit(t, f, settingDBConnections, 10)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if agent.conn["cl_u"] != 25 || agent.conn["cl_new"] != 10 {
		t.Fatalf("connection limits %v", agent.conn)
	}
	found := false
	for _, s := range f.panel.dbOwnerSettingsOf(ctx, 9802) {
		found = found || (s.User == "cl_u" && s.Key == settingDBConnections && s.Value == "25" && s.Origin == "owner")
	}
	if !found {
		t.Fatalf("the owner's connection limit is not on the card: %+v", f.panel.dbOwnerSettingsOf(ctx, 9802))
	}
	// The panel's own value follows the plan; a change of another key does
	// not write it again.
	setPlanDBLimit(t, f, settingDBConnections, 15)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if agent.conn["cl_new"] != 15 {
		t.Fatalf("the plan's change did not reach the panel's value: %v", agent.conn)
	}
	agent.requests = nil
	setPlanDBLimit(t, f, settingDBStatementSeconds, 6)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	for _, req := range agent.requests {
		for _, write := range req.Writes {
			if write.Key == transport.PGRoleLimitConnection {
				t.Fatalf("an unchanged connection limit was written: %+v", req)
			}
		}
	}
	// The owner changes the panel's value by hand: from then on it is the
	// owner's, also when the plan's limit is lifted.
	agent.conn["cl_new"] = 40
	setPlanDBLimit(t, f, settingDBConnections, 0)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if agent.conn["cl_new"] != 40 {
		t.Fatalf("the owner's later value was removed: %v", agent.conn)
	}
}

// unknownWriteEngine writes (or not) like limitsEngine but answers as if the
// answer was lost after the write.
type unknownWriteEngine struct {
	*limitsEngine
	land bool
}

func (e unknownWriteEngine) SetUserRoleSetting(username, key string, value int) error {
	if e.land {
		_ = e.limitsEngine.SetUserRoleSetting(username, key, value)
	}
	return errors.New("read tcp 127.0.0.1:3306: i/o timeout")
}

// Finding 4: a write whose answer was lost stays the panel's: the next round
// reaches it, and the line stays until a read-back confirms (the reviewer's
// probe: the panel's 5s became "the owner's" for good).
func TestALostWriteAnswerStaysThePanels(t *testing.T) {
	for _, land := range []bool{true, false} {
		f, engine, _ := dbLimitsFixture(t)
		ctx := context.Background()
		f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'uw_u', 'x')`)
		setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
		previous := newDatabaseDriver
		newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) {
			return unknownWriteEngine{limitsEngine: engine, land: land}, nil
		}
		first := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "uw_u")
		newDatabaseDriver = previous
		if first.State != dbLimitUnknown {
			t.Fatalf("land=%v: first outcome %+v", land, first)
		}
		if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 1 || problems[0].Name != dbLimitUnknown {
			t.Fatalf("land=%v: the unknown line: %+v", land, problems)
		}
		setPlanDBLimit(t, f, settingDBStatementSeconds, 9)
		if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
			t.Fatal(err)
		}
		if engine.settings["uw_u"][services.RoleKeyStatementSeconds] != "9s" {
			t.Fatalf("land=%v: the plan's new value did not reach the user: %v", land, engine.settings)
		}
		if owners := f.panel.dbOwnerSettingsOf(ctx, 9802); len(owners) != 0 {
			t.Fatalf("land=%v: the panel's value was taken for the owner's: %+v", land, owners)
		}
		if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
			t.Fatalf("land=%v: the line stayed after a confirmed write: %+v", land, problems)
		}
	}
}

// Finding 5: an earlier user's record of the same name is forgotten, also by
// a creation while no limit is set, and goes when the user is removed (the
// reviewer's probe: a later limit was silently skipped).
func TestAnEarlierUsersRecordIsForgotten(t *testing.T) {
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'st_u', 'x')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
	if o := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "st_u"); o.State != dbLimitApplied {
		t.Fatalf("%+v", o)
	}
	f.exec(t, `DELETE FROM database_users WHERE username = 'st_u'`)
	delete(engine.settings, "st_u")
	setPlanDBLimit(t, f, settingDBStatementSeconds, 0)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'st_u', 'x')`)
	_ = f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "st_u")
	setPlanDBLimit(t, f, settingDBStatementSeconds, 7)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if engine.settings["st_u"][services.RoleKeyStatementSeconds] != "7s" {
		t.Fatalf("the later limit did not reach the new user: %v", engine.settings)
	}
	// Removed: both records go.
	f.panel.noteDBLimitOutcome(ctx, dbLimitOutcome{ServerID: 9941, SubscriptionID: 9802, User: "st_u", State: dbLimitFailed})
	f.exec(t, `DELETE FROM database_users WHERE username = 'st_u'`)
	f.panel.pruneDBLimitRecords(ctx)
	if record := f.panel.loadDBRoleRecord(ctx)[dbRoleRecordKey(9941, "st_u")]; record != nil {
		t.Fatalf("the removed user's record stayed: %+v", record)
	}
	if raw := f.panel.setting(ctx, settingDBLimitsUnapplied); strings.Contains(raw, "st_u") {
		t.Fatalf("the removed user's entry stayed: %s", raw)
	}
}

// Finding 6: a run's users are written as pending before it starts; after a
// restart (no live run) they are "not known yet" with Apply again, never done,
// and a page following a run it no longer sees can tell.
func TestARestartLeavesARunsUsersNotKnownYet(t *testing.T) {
	f, _, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'p1_u', 'x'), (9941, 9802, 'p2_u', 'x')`)
	run := dbLimitsRunID()
	beginDBLimitsLiveRun(run)
	f.panel.markDBLimitsPending(ctx, []dbLimitUser{{subscription: 9802, server: 9941, username: "p1_u"}, {subscription: 9802, server: 9941, username: "p2_u"}}, run)
	if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
		t.Fatalf("a running run's users were listed: %+v", problems)
	}
	f.panel.noteDBLimitOutcome(ctx, dbLimitOutcome{ServerID: 9941, SubscriptionID: 9802, User: "p1_u", State: dbLimitApplied})
	endDBLimitsLiveRun(run) // the panel restarted before p2_u was reached
	problems := f.panel.databaseLimitsProblems(ctx)
	if len(problems) != 1 || problems[0].Name != dbLimitUnknown || problems[0].Reason != dbReasonNotKnownYet ||
		problems[0].Subject != "p2_u" || problems[0].Action.Kind != "apply_db_limits" {
		t.Fatalf("after a restart: %+v", problems)
	}
	// Each run has an id: a page that started run A and now reads another
	// (or none) knows it is not A's answer.
	first, _ := beginDBLimitsRun(1)
	endDBLimitsRun(0)
	second, _ := beginDBLimitsRun(1)
	endDBLimitsRun(0)
	if first.ID == "" || first.ID == second.ID || dbLimitsRunNow().ID != second.ID {
		t.Fatalf("run ids %q %q", first.ID, second.ID)
	}
}

// pgOwnRecordDriver is a PostgreSQL driver on the record's own account.
type pgOwnRecordDriver struct {
	services.DatabaseDriver
	mu   sync.Mutex
	conn map[string]int
}

func (d *pgOwnRecordDriver) SetUserConnectionLimit(username string, limit int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conn[username] = limit
	return nil
}

// Finding 8: only CelikPanel's own local PostgreSQL goes through the Agent's
// superuser path; a local record of the administrator's own account keeps it.
func TestOnlyCelikPanelsOwnPostgreSQLGoesThroughTheAgent(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	driver := &pgOwnRecordDriver{conn: map[string]int{}}
	previous := newDatabaseDriver
	newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) { return driver, nil }
	t.Cleanup(func() { newDatabaseDriver = previous })
	var pg int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM database_server_types WHERE name = 'postgresql'`).Scan(&pg)
	sealed, err := f.panel.secrets.Encrypt("owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, root_password_encrypted, admin_username, status) VALUES (9963, 9802, ?, 'docker pg', '16', '127.0.0.1', 5440, ?, 'postgres', 'active')`, pg, sealed)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9963, 9802, 'own_u', 'x')`)
	setPlanDBLimit(t, f, settingDBConnections, 6)
	if outcome := f.panel.applyRecordedUserLimits(ctx, 9802, 9963, "own_u", false); outcome.State != dbLimitApplied {
		t.Fatalf("outcome %+v", outcome)
	}
	if driver.conn["own_u"] != 6 {
		t.Fatalf("the record's own account was not used: %v", driver.conn)
	}
	for _, req := range agent.requests {
		if req.Role == "own_u" {
			t.Fatalf("the Agent's superuser path reached the administrator's PostgreSQL: %+v", req)
		}
	}
}

// Finding 10: the hourly round tries at most dbLimitsRetryPerRound users in
// one run, never two at once, and starts nothing while a run goes on.
func TestTheHourlyRoundIsOneBoundedRun(t *testing.T) {
	f, _, _ := dbLimitsFixture(t)
	ctx := context.Background()
	for i := 0; i < dbLimitsRetryPerRound+5; i++ {
		name := "hr_" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, ?, 'x')`, name)
		f.panel.noteDBLimitOutcome(ctx, dbLimitOutcome{ServerID: 9941, SubscriptionID: 9802, User: name, State: dbLimitPrerequisite})
	}
	var runs []func()
	previous := startDBLimitsRun
	startDBLimitsRun = func(run func()) { runs = append(runs, run) }
	t.Cleanup(func() { startDBLimitsRun = previous })
	t.Cleanup(func() { endDBLimitsRun(0) })
	f.panel.retryDBLimitsRound(ctx)
	if len(runs) != 1 || dbLimitsRunNow().Users != dbLimitsRetryPerRound {
		t.Fatalf("one round: %d runs, %+v", len(runs), dbLimitsRunNow())
	}
	f.panel.retryDBLimitsRound(ctx)
	w := httptest.NewRecorder()
	f.panel.handleDatabaseLimitsApply(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/database-limits/apply", nil))
	if len(runs) != 1 || w.Code != http.StatusAccepted {
		t.Fatalf("a second run started beside the first: %d", len(runs))
	}
	runs[0]()
	if dbLimitsRunNow().Running {
		t.Fatal("the run did not end")
	}
}

// Finding 11: a creation waits at most its bound; a user still being given
// its limits is answered as pending (not listed while its run goes on) and
// gets them in the run.
func TestACreationWaitsAtMostItsBound(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9961, 9802, 'slow_u', 'x')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
	previousWait := dbLimitsCreateWait
	dbLimitsCreateWait = 50 * time.Millisecond
	t.Cleanup(func() { dbLimitsCreateWait = previousWait })
	release := make(chan struct{})
	agent.answer = func(req *transport.PostgreSQLRoleLimitsRequest) (transport.PostgreSQLRoleLimitsResponse, error) {
		<-release
		out := map[string]string{}
		for _, write := range req.Writes {
			out[write.Key] = dbRoleWrittenText(write.Key, write.Value)
		}
		return transport.PostgreSQLRoleLimitsResponse{Applied: true, ConnectionLimit: -1, Settings: out}, nil
	}
	started := time.Now()
	outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9961, "slow_u")
	if outcome.State != dbLimitPending || time.Since(started) > 5*time.Second {
		t.Fatalf("outcome %+v after %s", outcome, time.Since(started))
	}
	if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
		t.Fatalf("a user still being given its limits was listed: %+v", problems)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && strings.Contains(f.panel.setting(ctx, settingDBLimitsUnapplied), "slow_u") {
		time.Sleep(10 * time.Millisecond)
	}
	if raw := f.panel.setting(ctx, settingDBLimitsUnapplied); strings.Contains(raw, "slow_u") {
		t.Fatalf("the pending entry stayed after the run reached the user: %s", raw)
	}
	// The plan's Save shows its step while it waits (the editor's lock and
	// steps), never a bare loading button.
	if source := readPanelSource(t, "../../web/src/components/PlanRights.tsx"); !strings.Contains(source, "plans.dbLimits.step") {
		t.Fatal("the plan's Save does not say what it waits for")
	}
}

// Finding 12: an unreadable record is never written over and is a line of
// its own; Apply again starts it afresh from every database user.
func TestAnUnreadableRecordHidesNothing(t *testing.T) {
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'cr_u', 'x')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 4)
	if err := f.panel.setSetting(ctx, settingDBLimitsUnapplied, "{not json"); err != nil {
		t.Fatal(err)
	}
	f.panel.noteDBLimitOutcome(ctx, dbLimitOutcome{ServerID: 9941, SubscriptionID: 9802, User: "cr_u", State: dbLimitFailed})
	if raw := f.panel.setting(ctx, settingDBLimitsUnapplied); raw != "{not json" {
		t.Fatalf("an unreadable record was written over: %q", raw)
	}
	problems := f.panel.databaseLimitsProblems(ctx)
	if len(problems) != 1 || problems[0].Reason != dbReasonRecordUnusable {
		t.Fatalf("an unreadable record: %+v", problems)
	}
	var runs []func()
	previous := startDBLimitsRun
	startDBLimitsRun = func(run func()) { runs = append(runs, run) }
	t.Cleanup(func() { startDBLimitsRun = previous })
	t.Cleanup(func() { endDBLimitsRun(0) })
	w := httptest.NewRecorder()
	f.panel.handleDatabaseLimitsApply(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/database-limits/apply", nil))
	var status dbLimitsRunStatus
	_ = json.Unmarshal(w.Body.Bytes(), &status)
	if w.Code != http.StatusAccepted || status.Users != 1 || len(runs) != 1 {
		t.Fatalf("Apply again on an unreadable record: %d %+v", w.Code, status)
	}
	runs[0]()
	if engine.settings["cr_u"][services.RoleKeyStatementSeconds] != "4s" {
		t.Fatalf("the user was not reached: %v", engine.settings)
	}
	if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
		t.Fatalf("the record was not started afresh: %+v", problems)
	}
}

// Finding 13: a statement time changed on the user itself is said to be so,
// not to be the server owner's; a real limit stays the owner's.
func TestAValueChangedOnTheUserItselfSaysSo(t *testing.T) {
	if dbOwnerSettingOrigin(services.RoleKeyStatementSeconds) != "on_user" || dbOwnerSettingOrigin(services.RoleKeyIdleInTransactionSeconds) != "on_user" ||
		dbOwnerSettingOrigin(services.RoleKeyTempFileMB) != "owner" {
		t.Fatal("origins")
	}
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'ou_u', 'x')`)
	engine.settings["ou_u"] = map[string]string{services.RoleKeyStatementSeconds: "30s"}
	setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	owners := f.panel.dbOwnerSettingsOf(ctx, 9802)
	if len(owners) != 1 || owners[0].Origin != "on_user" || engine.settings["ou_u"][services.RoleKeyStatementSeconds] != "30s" {
		t.Fatalf("owners %+v engine %v", owners, engine.settings)
	}
}

// Finding 19: Apply again under the cross-site gate of alpha.148 (a sign-in
// cookie sent from another site is refused before the handler) and with an
// API token (decided by the token layer; a read-only token never writes).
func TestApplyAgainUnderTheCrossSiteGateAndWithAToken(t *testing.T) {
	const route = "/api/v1/metrics/database-limits/apply"
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	reached := false
	handler := fixture.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusAccepted)
	}))
	cookie := &http.Cookie{Name: hostSessionCookieName, Value: fixture.tokens[signinAdminID]}
	for site, want := range map[string]bool{"cross-site": false, "same-site": false, "same-origin": true} {
		reached = false
		request := httptest.NewRequest(http.MethodPost, route, nil)
		request.AddCookie(cookie)
		request.Header.Set("Sec-Fetch-Site", site)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if reached != want || (!want && signinErrorCode(t, recorder) != errCodeCrossSiteRequest) {
			t.Fatalf("Sec-Fetch-Site %s: reached=%v %d %s", site, reached, recorder.Code, recorder.Body.String())
		}
	}

	tokens := newAPITokenFixture(t)
	probe := &apiTokenProbe{}
	tokenHandler := csrfProtect(tokens.panel.requireAuth(probe.handler()))
	send := func(token string) (*httptest.ResponseRecorder, bool) {
		probe.reached = false
		recorder := httptest.NewRecorder()
		tokenHandler.ServeHTTP(recorder, tokenRequest(http.MethodPost, route, token))
		return recorder, probe.reached
	}
	// Established: Apply again is not one of the Remote API's addresses, so
	// every token - an administrator's manage token too - is refused by the
	// token layer and never reaches the handler; it is a panel action.
	for _, minted := range []string{"admin/manage", "admin/read", "customer/manage"} {
		if recorder, ok := send(tokens.minted[minted]); ok || recorder.Code != http.StatusForbidden || apiErrorCode(t, recorder) != errCodeAPITokenRouteRefused {
			t.Fatalf("%s token: reached=%v %d %s", minted, ok, recorder.Code, recorder.Body.String())
		}
	}
}
