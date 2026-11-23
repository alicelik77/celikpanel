package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a: the site's limits in the rights chain, the site's card, the
// administrator's move and Problems now (site_limits.go,
// site_limits_view.go).

func siteLimitsMux(f rightsFixture) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", f.rights)
	mux.HandleFunc("/api/v1/domains/", f.panel.handleDomainSubroute)
	return f.panel.requireAuth(mux)
}

func siteLimitsCall(t *testing.T, f rightsFixture, handler http.Handler, userID int, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: f.tokens[userID]})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// TestSiteLimitKeysJoinTheChain: without any level the slice gets the code's
// defaults (the largest limits a site's programs had before); a plan's
// ceiling is the value and names the plan; the database keys are off until
// a level sets them.
func TestSiteLimitKeysJoinTheChain(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	limits, err := subscriptionSettingLimits(ctx, db, 9802)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{settingSiteMemoryMB: 3072, settingSiteTasks: 512, settingSiteCPUPercent: 200} {
		if got := siteLimitValueOf(limits, key); got.Value != want || got.SetBy != "server_default" || limits[key].Default == nil || *limits[key].Default != want {
			t.Errorf("%s = %+v, want %d from the default", key, got, want)
		}
	}
	if roles := dbRoleLimitsFrom(limits); roles.StatementSeconds != 0 || roles.IdleInTransactionSeconds != 0 || roles.TempFileMB != 0 {
		t.Fatalf("database limits are on by default: %+v", roles)
	}
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,`+
			`"ceilings":{"site.memory_mb":1024,"site.tasks":128,"db.statement_seconds":300,"db.temp_file_mb":2048}}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	limits, err = subscriptionSettingLimits(ctx, db, 9802)
	if err != nil {
		t.Fatal(err)
	}
	if got := siteLimitValueOf(limits, settingSiteMemoryMB); got.Value != 1024 || got.SetBy != settingSetByPlan || got.SetByName != "Lean" {
		t.Fatalf("plan memory: %+v", got)
	}
	slice := siteSliceFromLimits(7, "Shop.Example.com", limits)
	if slice == nil || *slice != (transport.SiteSlice{SiteID: 7, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 128, CPUPercent: 200}) {
		t.Fatalf("slice = %+v", slice)
	}
	if roles := dbRoleLimitsFrom(limits); roles.StatementSeconds != 300 || roles.TempFileMB != 2048 || roles.IdleInTransactionSeconds != 0 {
		t.Fatalf("database limits: %+v", roles)
	}
	// The server's own default applies where no level sets a ceiling.
	if _, err := db.Exec(`INSERT INTO setting_policy (key, server_ceiling, server_default, customer_editable) VALUES ('site.cpu_percent', NULL, 100, 1)`); err != nil {
		t.Fatal(err)
	}
	limits, _ = subscriptionSettingLimits(ctx, db, 9802)
	if got := siteLimitValueOf(limits, settingSiteCPUPercent); got.Value != 100 || got.SetBy != "server_default" {
		t.Fatalf("server default: %+v", got)
	}
	// A key out of range is refused as every key is.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPost, "/api/v1/plans",
		`{"name":"Odd","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":10}}`),
		http.StatusBadRequest, errCodeSettingUnknown)
}

// TestChangedSiteLimitsReachTheSlices: a plan's changed site limit is
// written into the slices of its sites at once; an unrelated change is not.
func TestChangedSiteLimitsReachTheSlices(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	oldCall, oldStart, oldNeeds, oldRead := applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent, readSiteSlicesCall
	t.Cleanup(func() {
		applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent, readSiteSlicesCall = oldCall, oldStart, oldNeeds, oldRead
	})
	// The lowered limit is above what the site uses now: nothing to confirm.
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return siteLimitsStatus(req.Sites[0].SiteID, 0, 0), nil
	}
	var sent []transport.ApplySiteSlicesRequest
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		sent = append(sent, *req)
		return transport.ApplySiteSlicesResponse{}, nil
	}
	startSiteSliceApply, siteLimitReadNeedsAgent = func(run func()) { run() }, false
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":1024}}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	sent = nil
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":768}}`),
		http.StatusOK, "")
	if len(sent) != 1 || len(sent[0].Sites) != 1 || sent[0].Sites[0].MemoryMB != 768 || sent[0].Sites[0].Domain == "" {
		t.Fatalf("slices written: %+v", sent)
	}
	sent = nil
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Lean","max_domains":5,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":768}}`),
		http.StatusOK, "")
	if len(sent) != 0 {
		t.Fatalf("a change without a site limit rewrote the slices: %+v", sent)
	}
}

// useHostMemory sets the host's memory the defaults are bound to.
func useHostMemory(t *testing.T, mb int) {
	t.Helper()
	old := hostMemoryMB
	hostMemoryMB = func() int { return mb }
	t.Cleanup(func() { hostMemoryMB = old })
}

// siteLimitsStatus is the Agent's reading of the fixture's site: memoryLimit
// times the site's own memory limit was reached (each with one kill),
// tasksLimit processes refused at the site's own process limit.
func siteLimitsStatus(siteID int, memoryLimit, tasksLimit int64) transport.SiteSlicesStatusResponse {
	return transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{{
		SiteID: siteID, Slice: "celiksite_" + itoa(siteID) + ".slice", File: transport.SiteFileManagedUnchanged, Active: true,
		Live: transport.SiteSliceLimitsRead{MemoryMB: 3072, Tasks: 512, CPUPercent: 200}, MemoryCurrentMB: 300, TasksCurrent: 20,
		CountersRead: true, Counters: transport.SiteSliceCounters{MemoryLimitHits: memoryLimit, MemoryOOMAll: memoryLimit, OOMKills: memoryLimit, TasksLimitHits: tasksLimit},
		Units: []transport.SiteSliceUnit{{Unit: "celikapp-" + itoa(siteID) + ".service", Placement: transport.SiteUnitOutside,
			Own: transport.SiteSliceLimitsRead{MemoryMB: 1536, Tasks: 512, CPUPercent: 200}}},
	}}}
}

// TestSiteLimitsCardSaysWhatHappened: the customer reads the limits and when
// its site reached one, never who sets them by name or what only the
// administrator can act on; the administrator also reads which programs run
// outside the limits.
func TestSiteLimitsCardSaysWhatHappened(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":1024}}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	reading := siteLimitsStatus(siteID, 0, 0)
	var asked []transport.SiteSliceQuery
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		asked = req.Sites
		return reading, nil
	}
	handler := siteLimitsMux(f)
	path := "/api/v1/domains/9902/site-limits"
	first := siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, path, "")
	wantStatus(t, first, http.StatusOK, "")
	if len(asked) != 1 || asked[0].SiteID != siteID || len(asked[0].Units) != 1 || asked[0].Units[0] != "celikapp-"+itoa(siteID)+".service" {
		t.Fatalf("asked %+v", asked)
	}
	// The site's programs used all their memory once since.
	reading = siteLimitsStatus(siteID, 1, 0)
	customer := siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, path, "")
	wantStatus(t, customer, http.StatusOK, "")
	var view siteLimitsView
	if err := json.Unmarshal(customer.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Known || view.Admin || len(view.Limits) != 3 || view.Limits[0].Value != 1024 || view.Limits[0].SetBy != settingSetByPlan ||
		view.Limits[0].SetByName != "" || len(view.Units) != 0 || view.SliceFile != "" || view.NeedsMove != 0 {
		t.Fatalf("customer view: %s", customer.Body.String())
	}
	if len(view.Events) != 1 || view.Events[0].Kind != "memory" || view.Events[0].Count != 1 || view.Events[0].LastAt == "" || view.Events[0].Repeated {
		t.Fatalf("customer events: %+v", view.Events)
	}
	admin := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodGet, path, "")
	wantStatus(t, admin, http.StatusOK, "")
	view = siteLimitsView{}
	_ = json.Unmarshal(admin.Body.Bytes(), &view)
	if !view.Admin || view.Limits[0].SetByName != "Lean" || view.NeedsMove != 1 || len(view.Units) != 1 || !view.Units[0].Lower ||
		view.Units[0].Placement != transport.SiteUnitOutside || view.SliceFile != transport.SiteFileManagedUnchanged {
		t.Fatalf("administrator view: %s", admin.Body.String())
	}
	// The server could not be read: the card says so and keeps the limits.
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return transport.SiteSlicesStatusResponse{}, errorString("no answer")
	}
	unread := siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, path, "")
	view = siteLimitsView{}
	_ = json.Unmarshal(unread.Body.Bytes(), &view)
	if view.Known || view.ReadError == "" || len(view.Limits) != 3 {
		t.Fatalf("unread: %s", unread.Body.String())
	}
}

// TestSiteLimitsMoveIsTheAdministratorsOneAction: a customer cannot move; the
// administrator's move asks the Agent once and a repeated request id gets
// the first answer.
func TestSiteLimitsMoveIsTheAdministratorsOneAction(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	old, oldTasks := moveSiteSliceCall, moveSiteTasksCall
	t.Cleanup(func() { moveSiteSliceCall, moveSiteTasksCall = old, oldTasks })
	// S4 R2b second fix round: the programs part runs only after a tasks
	// part whose result is known.
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}, nil
	}
	calls := 0
	var got *transport.MoveSiteSliceRequest
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		calls++
		got = req
		return transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{{Unit: req.Units[0], Outcome: transport.SiteMoveMoved}}}, nil
	}
	handler := siteLimitsMux(f)
	path := "/api/v1/domains/9902/site-limits/move"
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodPost, path, `{"request_id":"move-0001"}`), http.StatusForbidden, "")
	if calls != 0 {
		t.Fatal("a customer's request reached the Agent")
	}
	first := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, path, `{"request_id":"move-0001"}`)
	wantStatus(t, first, http.StatusOK, "")
	again := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, path, `{"request_id":"move-0001"}`)
	if calls != 1 || again.Body.String() != first.Body.String() {
		t.Fatalf("calls=%d first=%s again=%s", calls, first.Body.String(), again.Body.String())
	}
	if got == nil || got.SubscriptionID != 9802 || got.DomainID != 9902 || got.Site.MemoryMB != 3072 || len(got.Units) != 1 {
		t.Fatalf("move request: %+v", got)
	}
}

// TestSiteLimitReachedTwiceIsAProblem: a site whose memory limit was reached
// at two readings in a day is named in Problems now; once is the card's
// business only; a counter that starts again is not counted twice.
func TestSiteLimitReachedTwiceIsAProblem(t *testing.T) {
	now := time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)
	ledger := siteLimitLedger{}
	read := func(at time.Time, oom, tasks int64, throttled int64) {
		observeSiteSlices(ledger, []transport.SiteSliceStatus{{SiteID: 4, CountersRead: true,
			Counters: transport.SiteSliceCounters{MemoryLimitHits: oom, OOMKills: oom, TasksLimitHits: tasks, CPUThrottledUSec: throttled}}}, false, at)
	}
	read(now, 5, 0, 0) // the baseline: earlier kills are not given a time
	if ledger["4"].Events != nil {
		t.Fatalf("the baseline made events: %+v", ledger["4"].Events)
	}
	read(now.Add(5*time.Minute), 6, 0, 0)
	read(now.Add(10*time.Minute), 6, 3, 40_000_000)
	read(now.Add(15*time.Minute), 1, 3, 40_000_000) // the slice started again: one new kill
	events := ledger["4"].Events
	if events["memory"] == nil || events["memory"].Count != 2 || len(events["memory"].Hits) != 2 ||
		events["tasks"] == nil || events["tasks"].Count != 3 || events["cpu"] == nil || events["cpu"].Count != 1 {
		t.Fatalf("events: %+v %+v %+v", events["memory"], events["tasks"], events["cpu"])
	}
	// A day later the old hits no longer count as repeated (nothing new was
	// reached, so nothing was written either).
	if changed := observeSiteSlices(ledger, []transport.SiteSliceStatus{{SiteID: 4, CountersRead: true,
		Counters: transport.SiteSliceCounters{MemoryLimitHits: 1, OOMKills: 1, TasksLimitHits: 3, CPUThrottledUSec: 40_000_000}}}, false, now.Add(25*time.Hour)); changed {
		t.Fatal("a reading that reached nothing changed the ledger")
	}
	for _, view := range siteLimitEventViews(ledger["4"], now.Add(25*time.Hour)) {
		if view.Repeated {
			t.Fatalf("old hits counted as repeated: %+v", view)
		}
	}
}

// TestSiteLimitProblemsNameTheSite: the recorded readings make one line per
// site and limit, naming the domain and opening its page; a single hit makes
// none; a Panel without its Agent starts no background reading.
func TestSiteLimitProblemsNameTheSite(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	var domain string
	_ = f.panel.db.GetDB().QueryRow(`SELECT name FROM domains WHERE id = 9902`).Scan(&domain)
	now := time.Now().UTC()
	at := func(d time.Duration) string { return metricsTime(now.Add(-d)) }
	ledger := siteLimitLedger{itoa(siteID): &siteLimitRecord{ReadAt: at(0), Events: map[string]*siteLimitEvent{
		"memory": {Count: 3, FirstAt: at(2 * time.Hour), LastAt: at(time.Minute), Hits: []string{at(2 * time.Hour), at(time.Minute)}},
		"tasks":  {Count: 1, FirstAt: at(time.Hour), LastAt: at(time.Hour), Hits: []string{at(time.Hour)}},
	}}}
	raw, _ := json.Marshal(ledger)
	if err := f.panel.setSetting(context.Background(), settingSiteLimitEvents, string(raw)); err != nil {
		t.Fatal(err)
	}
	started := false
	oldStart := startSiteLimitRead
	t.Cleanup(func() { startSiteLimitRead = oldStart })
	startSiteLimitRead = func(func()) { started = true }
	problems := f.panel.siteLimitProblems(context.Background(), now)
	if started {
		t.Fatal("a Panel without its Agent started a reading")
	}
	if len(problems) != 1 || problems[0].Kind != "site_memory_repeated" || problems[0].Subject != domain || problems[0].Value != 2 ||
		problems[0].Action.To != "/domains/"+domain || problems[0].Actor != "administrator" {
		t.Fatalf("problems = %+v", problems)
	}
}

// TestSitesWithProgramsOutsideTheirLimitsAreOneLine: after an update nothing
// moves by itself, so the administrator reads ONE line naming the sites whose
// own programs still run (or would start) outside their limits, opening the
// first; a site whose programs are all inside, stopped inside or gone is not
// named; the line goes away by itself when the site's card reads it moved.
func TestSitesWithProgramsOutsideTheirLimitsAreOneLine(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	var domain string
	_ = f.panel.db.GetDB().QueryRow(`SELECT name FROM domains WHERE id = 9902`).Scan(&domain)
	oldRead, oldStart, oldNeeds := readSiteSlicesCall, startSiteLimitRead, siteLimitReadNeedsAgent
	t.Cleanup(func() { readSiteSlicesCall, startSiteLimitRead, siteLimitReadNeedsAgent = oldRead, oldStart, oldNeeds })
	startSiteLimitRead, siteLimitReadNeedsAgent = func(run func()) { run() }, false
	unit := "celikapp-" + itoa(siteID) + ".service"
	slice := "celiksite_" + itoa(siteID) + ".slice"
	status := func(units ...transport.SiteSliceUnit) transport.SiteSlicesStatusResponse {
		return transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{{SiteID: siteID, Slice: slice, Units: units}}}
	}
	reading := status(transport.SiteSliceUnit{Unit: unit, Placement: transport.SiteUnitOutside, Slice: "system.slice"})
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	lines := func() []monitoringProblem {
		siteLimitProblemReading = siteLimitReading{}
		var out []monitoringProblem
		for _, problem := range f.panel.siteLimitProblems(context.Background(), time.Now()) {
			if problem.Kind == "site_limits_outside" {
				out = append(out, problem)
			}
		}
		return out
	}
	// The background reading finds the running program outside the limits.
	_ = lines()
	got := lines()
	if len(got) != 1 || got[0].Key != "site_limits_outside" || got[0].Subject != domain || got[0].Value != 1 ||
		got[0].Severity != "warning" || got[0].Actor != "administrator" || got[0].Action.To != "/domains/"+domain {
		t.Fatalf("lines = %+v", got)
	}
	// A stopped program whose file still names the system slice would start
	// outside: still named.
	reading = status(transport.SiteSliceUnit{Unit: unit, Placement: transport.SiteUnitStopped, Slice: "system.slice"})
	_ = lines()
	if got := lines(); len(got) != 1 {
		t.Fatalf("stopped outside: %+v", got)
	}
	// The administrator moved the site: its card's reading takes the line away.
	reading = status(transport.SiteSliceUnit{Unit: unit, Placement: transport.SiteUnitInSlice, Slice: slice})
	wantStatus(t, siteLimitsCall(t, f, siteLimitsMux(f), authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits", ""), http.StatusOK, "")
	siteLimitReadNeedsAgent = true // no background reading: the card's alone
	if got := lines(); len(got) != 0 {
		t.Fatalf("after the move: %+v", got)
	}
	// Stopped inside and gone are not named either.
	siteLimitReadNeedsAgent = false
	reading = status(transport.SiteSliceUnit{Unit: unit, Placement: transport.SiteUnitStopped, Slice: slice})
	_ = lines()
	if got := lines(); len(got) != 0 {
		t.Fatalf("stopped inside: %+v", got)
	}
	if _, ordered := problemKindOrder["site_programs_outside"]; !ordered {
		t.Error("site_programs_outside has no place in the problem order")
	}
	if en, _ := alertLine(pendingAlertMail{kind: "site_programs_outside", subject: domain, value: 1}); strings.HasPrefix(en, "A monitored problem") {
		t.Error("site_programs_outside has no alert line")
	}
}

// TestSitesOutsideLineNamesTheFirstSites: count and the first names, sorted,
// "…" after five.
func TestSitesOutsideLineNamesTheFirstSites(t *testing.T) {
	names := map[string]string{}
	state := siteLimitOutside{CheckedAt: "2026-11-21T10:00:00Z", Sites: map[string]int{}}
	for i, name := range []string{"g.test", "b.test", "f.test", "a.test", "e.test", "d.test", "c.test"} {
		names[itoa(i+1)] = name
		state.Sites[itoa(i+1)] = 1
	}
	state.Sites["99"] = 2 // a site that is gone
	raw, _ := json.Marshal(state)
	got := siteLimitsOutsideProblem(string(raw), names)
	if len(got) != 1 || got[0].Value != 7 || got[0].Subject != "a.test, b.test, c.test, d.test, e.test, …" || got[0].Action.To != "/domains/a.test" {
		t.Fatalf("line = %+v", got)
	}
	if siteLimitsOutsideProblem(`{"checked_at":"x"}`, names) != nil || siteLimitsOutsideProblem("", names) != nil {
		t.Fatal("a line without sites")
	}
}

// TestDiskLimitsCountedLine: one line for the server where the hosting file
// system has no project quota; nothing before a measurement said so, and
// nothing where it has one.
func TestDiskLimitsCountedLine(t *testing.T) {
	if diskEnforcementProblems("") != nil {
		t.Fatal("a line before any measurement")
	}
	if diskEnforcementProblems(`{"mounted":true,"observed_at":"2026-11-21T10:00:00Z"}`) != nil {
		t.Fatal("a line with project quotas")
	}
	lines := diskEnforcementProblems(`{"mounted":false,"observed_at":"2026-11-21T10:00:00Z"}`)
	if len(lines) != 1 || lines[0].Kind != "disk_limits_counted" || lines[0].Actor != "server_owner" || lines[0].Key != "disk_limits_counted" {
		t.Fatalf("lines = %+v", lines)
	}
	for _, kind := range []string{"site_memory_repeated", "site_tasks_repeated", "disk_limits_counted"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Errorf("%s has no place in the problem order", kind)
		}
		if en, _ := alertLine(pendingAlertMail{kind: kind, subject: "shop.example.com", name: "memory", value: 2}); strings.HasPrefix(en, "A monitored problem") {
			t.Errorf("%s has no alert line", kind)
		}
	}
}
