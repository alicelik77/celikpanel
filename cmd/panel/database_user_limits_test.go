package main

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/secrets"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 fix round B: the plan's database limits reach every database
// user of a site (the alpha.148 lab, D9 and D10).

// limitsEngine is a MariaDB engine's users in memory: the connection limit
// and the role settings, with reads that can be made to fail.
type limitsEngine struct {
	services.DatabaseDriver
	mu        sync.Mutex
	conn      map[string]int
	settings  map[string]map[string]string
	readErr   error
	afterErr  error
	reads     int
	connCalls int
	roleCalls []string
}

func newLimitsEngine() *limitsEngine {
	return &limitsEngine{conn: map[string]int{}, settings: map[string]map[string]string{}}
}

func (e *limitsEngine) SetUserConnectionLimit(username string, limit int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.connCalls++
	e.conn[username] = limit
	return nil
}

func (e *limitsEngine) RoleKeysSupported() []string {
	return []string{services.RoleKeyStatementSeconds}
}

func (e *limitsEngine) ReadUserRoleSettings(username string) (services.RoleSettings, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reads++
	if e.readErr != nil {
		return nil, e.readErr
	}
	if e.afterErr != nil && len(e.roleCalls) > 0 {
		return nil, e.afterErr
	}
	return services.RoleSettings{services.RoleKeyStatementSeconds: e.settings[username][services.RoleKeyStatementSeconds]}, nil
}

func (e *limitsEngine) SetUserRoleSetting(username, key string, value int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.roleCalls = append(e.roleCalls, username+":"+key)
	if e.settings[username] == nil {
		e.settings[username] = map[string]string{}
	}
	if value == 0 {
		delete(e.settings[username], key)
	} else {
		e.settings[username][key] = dbRoleWrittenText(key, value)
	}
	return nil
}

// dbLimitsFixture is a subscription (9802) with a MariaDB record (9941) and
// a local PostgreSQL record (9961) whose plan (9951) has no limit yet.
func dbLimitsFixture(t *testing.T) (rightsFixture, *limitsEngine, *pgAgentRequests) {
	t.Helper()
	f := newRightsFixture(t)
	box, err := secrets.LoadOrCreate(filepath.Join(t.TempDir(), "database-secrets.key"))
	if err != nil {
		t.Fatal(err)
	}
	f.panel.secrets = box
	sealed, err := box.Encrypt("root-secret")
	if err != nil {
		t.Fatal(err)
	}
	engine := newLimitsEngine()
	previous := newDatabaseDriver
	newDatabaseDriver = func(config services.DriverConfig) (services.DatabaseDriver, error) {
		if config.Port != 3306 {
			return &refusingDriver{t: t}, nil
		}
		return engine, nil
	}
	t.Cleanup(func() { newDatabaseDriver = previous })
	agent := withPGAgentLimits(t)
	previousClusters := readPGClustersFromAgent
	readPGClustersFromAgent = func(*Panel, context.Context) (transport.PostgreSQLClustersResponse, error) {
		return transport.PostgreSQLClustersResponse{}, nil
	}
	t.Cleanup(func() { readPGClustersFromAgent = previousClusters })
	db := f.panel.db.GetDB()
	var maria, pg int
	_ = db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'mariadb'`).Scan(&maria)
	_ = db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'postgresql'`).Scan(&pg)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, root_password_encrypted, status) VALUES (9941, 9802, ?, 'maria', '11.8', '127.0.0.1', 3306, ?, 'active')`, maria, sealed)
	// CelikPanel's own local PostgreSQL record (its own account, alpha.149
	// finding 8: only such a record goes through the Agent).
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, root_password_encrypted, admin_username, status) VALUES (9961, 9802, ?, 'pg17', '17', '127.0.0.1', 5432, ?, 'celikpanel_admin', 'active')`, pg, sealed)
	f.exec(t, `INSERT INTO service_plans (id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb) VALUES (9951, 'DB', 0, 0, 0, 0, 0)`)
	f.exec(t, `UPDATE subscriptions SET plan_id = 9951 WHERE id = 9802`)
	return f, engine, agent
}

func setPlanDBLimit(t *testing.T, f rightsFixture, key string, value int) {
	t.Helper()
	f.exec(t, `DELETE FROM setting_ceilings WHERE owner_kind = 'plan' AND owner_id = 9951 AND key = ?`, key)
	if value > 0 {
		f.exec(t, `INSERT INTO setting_ceilings (owner_kind, owner_id, key, value) VALUES ('plan', 9951, ?, ?)`, key, value)
	}
}

// D9: a user created while only a time limit is set gets it (alpha.148 gave
// it nothing: creation applied limits only when the connection limit was
// above 0); D10: a PostgreSQL user gets every limit, through the Agent.
func TestANewDatabaseUserGetsEveryLimitThePlanSets(t *testing.T) {
	f, engine, agent := dbLimitsFixture(t)
	ctx := context.Background()
	setPlanDBLimit(t, f, settingDBStatementSeconds, 1)
	setPlanDBLimit(t, f, settingDBTempFileMB, 64)
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'shop_new', 'x'), (9961, 9802, 'pg_shop', 'x')`)

	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "shop_new"); outcome.State != dbLimitApplied {
		t.Fatalf("MariaDB: %+v", outcome)
	}
	if engine.settings["shop_new"][services.RoleKeyStatementSeconds] != "1s" || engine.connCalls != 0 {
		t.Fatalf("MariaDB user: %v, connection writes %d", engine.settings, engine.connCalls)
	}
	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9961, "pg_shop"); outcome.State != dbLimitApplied {
		t.Fatalf("PostgreSQL: %+v", outcome)
	}
	got := agent.settings["pg_shop"]
	if got[services.RoleKeyStatementSeconds] != "1s" || got[services.RoleKeyTempFileMB] != "64MB" || got[services.RoleKeyIdleInTransactionSeconds] != "" {
		t.Fatalf("PostgreSQL role: %v (%+v)", got, agent.requests)
	}
	for _, req := range agent.requests {
		if req.Port != 5432 || req.Role != "pg_shop" || req.ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
			t.Fatalf("request %+v", req)
		}
		for _, write := range req.Writes {
			if write.Key == transport.PGRoleLimitConnection {
				t.Fatalf("a new user without a connection limit was given one: %+v", req)
			}
		}
	}
	// The panel recorded what it wrote, so the plan's next change reaches it.
	setPlanDBLimit(t, f, settingDBStatementSeconds, 3)
	if _, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil); err != nil {
		t.Fatal(err)
	}
	if engine.settings["shop_new"][services.RoleKeyStatementSeconds] != "3s" || agent.settings["pg_shop"][services.RoleKeyStatementSeconds] != "3s" {
		t.Fatalf("the plan's change: %v %v", engine.settings, agent.settings)
	}
}

// Decision 5: a state that cannot be read writes nothing - no connection
// limit removed or raised - and the user is reported, never skipped.
func TestAnUnreadableUserWritesNothingAndIsReported(t *testing.T) {
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'shop_old', 'x')`)
	engine.readErr = errors.New("Error 1045 (28000): Access denied for user 'celikpanel_admin'@'localhost'")
	// The plan's connection limit was lifted: alpha.148 removed it before
	// reading anything.
	outcomes, err := f.panel.reapplyDBConnectionLimitsNow(ctx, []int{9802}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if engine.connCalls != 0 || len(engine.roleCalls) != 0 {
		t.Fatalf("an unreadable user was written: %d %v", engine.connCalls, engine.roleCalls)
	}
	// alpha.149 (finding 7): a refused credential is the server refusing
	// the connection, its own outcome, with its reason code on the line.
	if len(outcomes) != 1 || outcomes[0].State != dbLimitConnection || outcomes[0].Code != dbReasonConnection {
		t.Fatalf("outcomes %+v", outcomes)
	}
	problems := f.panel.databaseLimitsProblems(ctx)
	if len(problems) != 1 || problems[0].Kind != "database_limits_unapplied" || problems[0].Name != dbLimitConnection || problems[0].Reason != dbReasonConnection ||
		problems[0].Subject != "shop_old" || problems[0].Value != 1 || problems[0].Action.Kind != "apply_db_limits" {
		t.Fatalf("Problems now: %+v", problems)
	}
	// The user reached later leaves the record.
	engine.readErr = nil
	if outcome := f.panel.applyRecordedUserLimits(ctx, 9802, 9941, "shop_old", false); outcome.State != dbLimitApplied {
		t.Fatalf("again: %+v", outcome)
	}
	if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
		t.Fatalf("a reached user is still listed: %+v", problems)
	}
}

// The three outcomes are told apart, each on its own line with its count and
// first users; a deleted user is not listed.
func TestDatabaseLimitOutcomesAreToldApart(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{&pgClusterRefusalError{refusal: transport.DatabaseClusterRefusal{Reason: "stopped"}}, dbLimitPrerequisite},
		{errors.New("dial tcp 127.0.0.1:3306: connect: connection refused"), dbLimitPrerequisite},
		{errors.New("dial unix /run/mysqld/mysqld.sock: connect: no such file or directory"), dbLimitPrerequisite},
		{&dbLimitError{state: dbLimitPrerequisite, err: errors.New("x")}, dbLimitPrerequisite},
		{errors.New("read tcp 127.0.0.1:3306: i/o timeout"), dbLimitUnknown},
		{context.DeadlineExceeded, dbLimitUnknown},
		{errors.New("invalid connection: broken pipe"), dbLimitUnknown},
		{errors.New("Error 1227 (42000): Access denied; you need (at least one of) the SUPER privilege(s)"), dbLimitFailed},
		{errors.New("ERROR: permission denied to alter role (SQLSTATE 42501)"), dbLimitFailed},
		// alpha.149 (review of PR #370, finding 7): what is actually known.
		{errors.New("Error 1045 (28000): Access denied for user 'celikpanel_admin'@'localhost' (using password: YES)"), dbLimitConnection},
		{errors.New("Error 1040: Too many connections"), dbLimitConnection},
		{&dbLimitError{state: dbLimitAgent, code: dbReasonAgent, err: errors.New("x")}, dbLimitAgent},
		{&dbLimitError{state: dbLimitRefused, code: dbReasonPrivileged, err: errors.New("x")}, dbLimitRefused},
	} {
		if got := dbLimitStateOf(c.err); got != c.want {
			t.Errorf("%v: %s, want %s", c.err, got, c.want)
		}
	}
	f, _, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'a_u', 'x'), (9941, 9802, 'b_u', 'x'), (9961, 9802, 'c_u', 'x'), (9961, 9802, 'd_u', 'x'), (9941, 9802, 'e_u', 'x')`)
	for _, o := range []dbLimitOutcome{
		{ServerID: 9941, SubscriptionID: 9802, User: "a_u", State: dbLimitFailed},
		{ServerID: 9941, SubscriptionID: 9802, User: "b_u", State: dbLimitFailed},
		{ServerID: 9961, SubscriptionID: 9802, User: "c_u", State: dbLimitPrerequisite},
		{ServerID: 9961, SubscriptionID: 9802, User: "d_u", State: dbLimitUnknown},
		{ServerID: 9941, SubscriptionID: 9802, User: "gone_u", State: dbLimitFailed},
	} {
		f.panel.noteDBLimitOutcome(ctx, o)
	}
	problems := f.panel.databaseLimitsProblems(ctx)
	byState := map[string]monitoringProblem{}
	for _, p := range problems {
		byState[p.Name] = p
	}
	if len(problems) != 3 || byState[dbLimitFailed].Value != 2 || !strings.Contains(byState[dbLimitFailed].Subject, "a_u") ||
		strings.Contains(byState[dbLimitFailed].Subject, "gone_u") || byState[dbLimitPrerequisite].Actor != "server_owner" ||
		byState[dbLimitUnknown].Subject != "d_u" || byState[dbLimitFailed].Key == byState[dbLimitUnknown].Key {
		t.Fatalf("lines: %+v", problems)
	}
	// The line takes part in Problems now, in its kind's place.
	if _, ok := problemKindOrder["database_limits_unapplied"]; !ok {
		t.Fatal("the kind has no place in Problems now")
	}
	report := f.panel.collectMonitoringProblems(ctx, time.Now())
	found := 0
	for _, p := range report.Problems {
		if p.Kind == "database_limits_unapplied" {
			found++
		}
	}
	if found != 3 {
		t.Fatalf("Problems now has %d database limit lines: %+v", found, report.Problems)
	}
}

// Apply again runs once; a second press while it runs follows the same run;
// reading the list or the run's state starts nothing.
func TestApplyAgainNeverStartsTwice(t *testing.T) {
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'shop_u', 'x')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 2)
	f.panel.noteDBLimitOutcome(ctx, dbLimitOutcome{ServerID: 9941, SubscriptionID: 9802, User: "shop_u", State: dbLimitPrerequisite})
	var pending []func()
	previous := startDBLimitsRun
	startDBLimitsRun = func(run func()) { pending = append(pending, run) }
	t.Cleanup(func() { startDBLimitsRun = previous })
	t.Cleanup(func() { endDBLimitsRun(0) })

	post := func() dbLimitsRunStatus {
		w := httptest.NewRecorder()
		f.panel.handleDatabaseLimitsApply(w, httptest.NewRequest(http.MethodPost, "/api/v1/metrics/database-limits/apply", nil))
		if w.Code != http.StatusAccepted {
			t.Fatalf("POST: %d %s", w.Code, w.Body.String())
		}
		var status dbLimitsRunStatus
		_ = json.Unmarshal(w.Body.Bytes(), &status)
		return status
	}
	if status := post(); !status.Running || status.Users != 1 {
		t.Fatalf("first press: %+v", status)
	}
	if status := post(); !status.Running || len(pending) != 1 {
		t.Fatalf("a second press started a second run: %d runs, %+v", len(pending), status)
	}
	_ = f.panel.collectMonitoringProblems(ctx, time.Now())
	w := httptest.NewRecorder()
	f.panel.handleDatabaseLimitsApply(w, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/database-limits/apply", nil))
	if len(pending) != 1 || len(engine.roleCalls) != 0 {
		t.Fatalf("a read started a change: %d runs, %v", len(pending), engine.roleCalls)
	}
	pending[0]()
	if status := dbLimitsRunNow(); status.Running || status.NotApplied != 0 || engine.settings["shop_u"][services.RoleKeyStatementSeconds] != "2s" {
		t.Fatalf("after the run: %+v %v", status, engine.settings)
	}
	if problems := f.panel.databaseLimitsProblems(ctx); len(problems) != 0 {
		t.Fatalf("the line stays after Apply again reached the user: %+v", problems)
	}
}

// Decision 4: a plan's Save says which users did not get the plan's
// database limits.
func TestAPlansSaveNamesTheUsersTheLimitsDidNotReach(t *testing.T) {
	f, engine, agent := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'shop_u', 'x'), (9961, 9802, 'pg_u', 'x')`)
	agent.answer = func(*transport.PostgreSQLRoleLimitsRequest) (transport.PostgreSQLRoleLimitsResponse, error) {
		return transport.PostgreSQLRoleLimitsResponse{Refused: transport.PGRoleLimitRefusedPrivileged, Error: "privileged"}, nil
	}
	before := map[string]int{}
	setPlanDBLimit(t, f, settingDBStatementSeconds, 5)
	answer := f.panel.reapplyDBLimitsAfterAnswer(ctx, rightsOwnerPlan, 9951, before, map[string]int{settingDBStatementSeconds: 5})
	// alpha.149 (finding 7): the Agent's refusal is its own outcome.
	if answer == nil || answer.Users != 2 || answer.Running != 0 || answer.NotAppliedCount != 1 ||
		len(answer.NotApplied) != 1 || answer.NotApplied[0].User != "pg_u" || answer.NotApplied[0].State != dbLimitRefused {
		t.Fatalf("answer %+v", answer)
	}
	if engine.settings["shop_u"][services.RoleKeyStatementSeconds] != "5s" {
		t.Fatalf("the MariaDB user: %v", engine.settings)
	}
	if f.panel.reapplyDBLimitsAfterAnswer(ctx, rightsOwnerPlan, 9951, map[string]int{settingPHPMemoryMB: 1}, map[string]int{settingPHPMemoryMB: 1}) != nil {
		t.Fatal("an unchanged plan answered about its database users")
	}
	// The plan's handler sends it.
	source, err := os.ReadFile("plan_handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `answer["db_limits"] = dbLimits`) {
		t.Fatal("the plan's Save does not answer about the database users")
	}
}

// A PostgreSQL database moved to another cluster keeps its users' limits as
// the panel's: what the panel wrote is carried to the new record, so the
// role's copied values are not taken for the owner's and the plan reaches
// them there.
func TestAClusterMoveCarriesThePanelsLimits(t *testing.T) {
	f, _, agent := dbLimitsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	var pg int
	_ = db.QueryRow(`SELECT id FROM database_server_types WHERE name = 'postgresql'`).Scan(&pg)
	f.exec(t, `INSERT INTO database_servers (id, subscription_id, type_id, name, version, host, port, status) VALUES (9962, 9802, ?, 'pg18', '18', '127.0.0.1', 5433, 'active')`, pg)
	f.exec(t, `INSERT INTO database_users (id, server_id, subscription_id, username, password) VALUES (99701, 9961, 9802, 'mv_u', 'x'), (99702, 9962, 9802, 'mv_u', 'x')`)
	f.exec(t, `INSERT INTO databases_v2 (id, server_id, subscription_id, name) VALUES (99711, 9962, 9802, 'mv')`)
	f.exec(t, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (99711, 99702, 'ALL')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 4)
	if outcome := f.panel.applyRecordedUserLimits(ctx, 9802, 9961, "mv_u", true); outcome.State != dbLimitApplied {
		t.Fatalf("on the source: %+v", outcome)
	}
	// The move copied the role with its settings to the other cluster: the
	// Agent's role there holds the same value (one in-memory role here).
	setPlanDBLimit(t, f, settingDBStatementSeconds, 9)
	// alpha.149 (finding 11): the users follow in a run apart from the
	// switch; here the run is waited for.
	var runs []func()
	previous := startDBLimitsRun
	startDBLimitsRun = func(run func()) { runs = append(runs, run) }
	t.Cleanup(func() { startDBLimitsRun = previous })
	f.panel.dbLimitsFollowSwitch(ctx, 99711, 9961, 9962)
	if len(runs) != 1 {
		t.Fatalf("the switch did not hand its users to one run: %d", len(runs))
	}
	if agent.settings["mv_u"][services.RoleKeyStatementSeconds] == "9s" {
		t.Fatal("the switch waited for its users' limits")
	}
	runs[0]()
	if agent.settings["mv_u"][services.RoleKeyStatementSeconds] != "9s" {
		t.Fatalf("the moved role kept the earlier value as the owner's: %v", agent.settings)
	}
	record := f.panel.loadDBRoleRecord(ctx)[dbRoleRecordKey(9962, "mv_u")]
	if record == nil || len(record.Owner) != 0 || record.Panel[services.RoleKeyStatementSeconds].Value != 9 {
		t.Fatalf("the record on the new server: %+v", record)
	}
	last := agent.requests[len(agent.requests)-1]
	if last.Port != 5433 {
		t.Fatalf("the moved role was reached at %d", last.Port)
	}
}

// A value the engine accepted whose read-back failed is recorded as the
// panel's own text, never as "" (which the next round would take for the
// owner's), and the outcome is unknown.
func TestAReadBackThatFailsKeepsThePanelsValue(t *testing.T) {
	f, engine, _ := dbLimitsFixture(t)
	ctx := context.Background()
	f.exec(t, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (9941, 9802, 'rb_u', 'x')`)
	setPlanDBLimit(t, f, settingDBStatementSeconds, 7)
	engine.afterErr = errors.New("read tcp: i/o timeout")
	if outcome := f.panel.applyNewDatabaseUserLimits(ctx, 9802, 9941, "rb_u"); outcome.State != dbLimitUnknown {
		t.Fatalf("outcome %+v", outcome)
	}
	record := f.panel.loadDBRoleRecord(ctx)[dbRoleRecordKey(9941, "rb_u")]
	if record == nil || record.Panel[services.RoleKeyStatementSeconds].Raw != "7s" {
		t.Fatalf("record %+v", record)
	}
	engine.afterErr = nil
	setPlanDBLimit(t, f, settingDBStatementSeconds, 8)
	if outcome := f.panel.applyRecordedUserLimits(ctx, 9802, 9941, "rb_u", false); outcome.State != dbLimitApplied || engine.settings["rb_u"][services.RoleKeyStatementSeconds] != "8s" {
		t.Fatalf("the next change: %+v %v", outcome, engine.settings)
	}
}

// Decision 1: every path that creates or re-creates a database user calls
// the one function. The creating calls are found in the source; each must be
// listed here with the function that applies the limits, and that function
// must call it. A new creating path fails this test until it does.
func TestEveryPathThatCreatesADatabaseUserAppliesItsLimits(t *testing.T) {
	applies := map[string]string{
		// creating function            -> the function that applies the limits
		"Panel.handleCreateDatabase":           "Panel.handleCreateDatabase",        // a site's Databases section
		"panelDatabaseAccessNative.create":     "Panel.handleAddDatabaseAccessUser", // a database's added user
		"Panel.recordDatabaseAccessUser":       "Panel.handleAddDatabaseAccessUser", // (its record)
		"Panel.handleCreateDatabaseV2":         "Panel.handleCreateDatabaseV2",      // Database Servers screen
		"Panel.handleCreateDatabaseV2User":     "Panel.handleCreateDatabaseV2User",  // Database Servers screen, a user
		"Panel.handleAppInstall":               "Panel.handleAppInstall",            // Apps (WordPress)
		"Panel.storeAppDatabase":               "Panel.handleAppInstall",            // (its record; also the WordPress copy)
		"Panel.handleCatalogAppInstall":        "Panel.handleCatalogAppInstall",     // one-click apps
		"Panel.storeCatalogAppOperation":       "Panel.handleCatalogAppInstall",     // (its record)
		"Panel.runWordPressClone":              "Panel.runWordPressClone",           // WordPress tools: copy
		"Panel.siteCopyCreateDatabase":         "Panel.siteCopyCreateDatabase",      // site copy
		"Panel.handleDatabaseCopy":             "Panel.handleDatabaseCopy",          // database tools: copy
		"Panel.insertDatabaseToolsCopyRecords": "Panel.handleDatabaseCopy",          // (its record)
		"Panel.pgSwitchRecord":                 "Panel.handlePGMoveAction",          // PostgreSQL cluster move
	}
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{}
	creating := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		source, _ := os.ReadFile(name)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				recv := fn.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if ident, ok := recv.(*ast.Ident); ok {
					key = ident.Name + "." + key
				}
			}
			body := string(source[fset.Position(fn.Body.Pos()).Offset:fset.Position(fn.Body.End()).Offset])
			bodies[key] = body
			if strings.Contains(body, `"Agent.CreateDatabase"`) || strings.Contains(body, `"Agent.CreateDatabaseAccessUserV1"`) ||
				strings.Contains(body, "driver.CreateUser(") || strings.Contains(body, "INSERT INTO database_users") {
				creating[key] = true
			}
		}
	}
	found := []string{}
	for key := range creating {
		found = append(found, key)
		if _, listed := applies[key]; !listed {
			t.Errorf("%s creates or records a database user and is not listed with the function that applies its limits", key)
		}
	}
	sort.Strings(found)
	for key, applier := range applies {
		if !creating[key] {
			t.Errorf("%s is listed but no longer creates a database user (found: %v)", key, found)
		}
		body := bodies[applier]
		call := "applyNewDatabaseUserLimits("
		if key == "Panel.pgSwitchRecord" {
			call = "dbLimitsFollowSwitch("
		}
		if !strings.Contains(body, call) {
			t.Errorf("%s (for %s) does not call %s", applier, key, call)
		}
	}
	// Each creating handler applies the limits after the user is recorded
	// (alpha.149, review of PR #370 finding 14: every path the source shows
	// it for; the cluster move records in pgSwitchRecord inside the switch's
	// transaction, before dbLimitsFollowSwitch, which the call order in
	// handlePGMoveAction shows below). Every apply call must follow the record.
	for _, c := range []struct{ fn, record string }{
		{"Panel.handleCreateDatabase", "tx.Commit()"},
		{"Panel.handleCreateDatabaseV2", "tx.Commit()"},
		{"Panel.handleCreateDatabaseV2User", "userRepo.Create(ctx, user)"},
		{"Panel.handleAddDatabaseAccessUser", "p.recordDatabaseAccessUser("},
		{"Panel.siteCopyCreateDatabase", "tx.Commit()"},
		{"Panel.handleDatabaseCopy", "p.insertDatabaseToolsCopyRecords("},
		// One-click apps record the user with the operation before the Agent
		// creates it (the review read storeCatalogAppOperation's body, not its
		// call, which comes first).
		{"Panel.handleCatalogAppInstall", "p.storeCatalogAppOperation("},
		{"Panel.handleAppInstall", "p.storeAppDatabase("},
		{"Panel.runWordPressClone", "p.storeAppDatabase("},
	} {
		body := bodies[c.fn]
		record := strings.Index(body, c.record)
		if record < 0 || strings.Count(body, "applyNewDatabaseUserLimits(") == 0 || strings.Index(body, "applyNewDatabaseUserLimits(") < record {
			t.Errorf("%s applies the limits before it records the user", c.fn)
		}
	}
	if move := bodies["Panel.handlePGMoveAction"]; !strings.Contains(move, "dbLimitsFollowSwitch(") {
		t.Error("the cluster move does not hand its users' limits to the one function")
	}
}

// alpha.149 (review of PR #370, finding 14): the users the Agent and the
// drivers create. Every string literal in cmd/agent, internal/services and
// internal/dbremote that creates an account is on this list with why its
// user gets the limits (or is no site user); a new one fails until it is.
func TestEveryAccountTheAgentOrADriverCreatesIsAccountedFor(t *testing.T) {
	allowed := map[string]int{
		// The site's database user: the panel records it, then applies.
		"../agent/database_rpc.go": 3,
		// An added user: the panel records it, then applies.
		"../agent/database_access_users.go": 2,
		// cPanel import: a temporary load account, dropped after the load.
		"../agent/cpmove_rpc.go": 1,
		// CelikPanel's own account, no site user.
		"../agent/database_admin_account.go": 2,
		// The cluster move re-creates a role as the source defines it
		// (dbLimitsFollowSwitch).
		"../agent/pg_move.go": 2,
		// A remote copy carries the local account's limits.
		"../../internal/dbremote/mariadb.go": 1,
		// The Database Servers screen (driver.CreateUser, the contract above).
		"../../internal/services/mariadb_driver.go":    1,
		"../../internal/services/postgresql_driver.go": 1,
	}
	found := map[string]int{}
	for _, dir := range []string{"../agent", "../../internal/services", "../../internal/dbremote"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING &&
					(strings.Contains(lit.Value, "CREATE USER") || strings.Contains(lit.Value, "CREATE ROLE")) {
					found[filepath.ToSlash(name)]++
				}
				return true
			})
		}
	}
	for name, count := range found {
		if allowed[name] != count {
			t.Errorf("%s creates accounts in %d places, %d accounted for: add the new path to the limits (and here)", name, count, allowed[name])
		}
	}
	for name := range allowed {
		if found[name] == 0 {
			t.Errorf("%s is accounted for but creates no account any more", name)
		}
	}
}
