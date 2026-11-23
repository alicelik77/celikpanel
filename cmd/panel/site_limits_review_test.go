package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a fix round: the tests the independent review of PR #349 required
// (docs/SITE-SANDBOX.md section 8, docs/SITE-LIMITS.md section 4a).

// TestOnlyTheSitesOwnLimitBlamesThePlan (review 4, 5): a kill at a program's
// own smaller memory limit, a host-wide shortage and a process refused at a
// program's own limit are recorded as "other" events, never as the site's
// limit, and never make the Problems now line about the plan's limit.
func TestOnlyTheSitesOwnLimitBlamesThePlan(t *testing.T) {
	now := time.Date(2026, 11, 21, 14, 0, 0, 0, time.UTC)
	ledger := siteLimitLedger{}
	read := func(at time.Time, c transport.SiteSliceCounters) {
		observeSiteSlices(ledger, []transport.SiteSliceStatus{{SiteID: 4, CountersRead: true, Counters: c}}, false, at)
	}
	read(now, transport.SiteSliceCounters{})
	// F3b: a program's own MemoryMax: hierarchical oom and oom_kill, no local oom.
	read(now.Add(5*time.Minute), transport.SiteSliceCounters{MemoryOOMAll: 1, OOMKills: 1})
	// F3d: a host-wide shortage: oom_kill only.
	read(now.Add(10*time.Minute), transport.SiteSliceCounters{MemoryOOMAll: 1, OOMKills: 2})
	// F2b: a program's own TasksMax.
	read(now.Add(15*time.Minute), transport.SiteSliceCounters{MemoryOOMAll: 1, OOMKills: 2, TasksOtherHits: 3})
	events := ledger["4"].Events
	if events["memory"] != nil || events["tasks"] != nil {
		t.Fatalf("a program's own limit or a host-wide shortage was blamed on the site's limit: %+v %+v", events["memory"], events["tasks"])
	}
	if events["memory_other"] == nil || events["memory_other"].Count != 2 || len(events["memory_other"].Hits) != 2 || events["tasks_other"] == nil {
		t.Fatalf("other events: %+v %+v", events["memory_other"], events["tasks_other"])
	}
	// F3a: the site's own limit: the slice's local oom rises.
	read(now.Add(20*time.Minute), transport.SiteSliceCounters{MemoryLimitHits: 1, MemoryOOMAll: 2, OOMKills: 3, TasksOtherHits: 3})
	if events = ledger["4"].Events; events["memory"] == nil || events["memory"].Count != 1 || events["memory_other"].Count != 2 {
		t.Fatalf("the site's own limit: %+v %+v", events["memory"], events["memory_other"])
	}

	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID)
	at := func(d time.Duration) string { return metricsTime(time.Now().UTC().Add(-d)) }
	other := siteLimitLedger{itoa(siteID): &siteLimitRecord{Events: map[string]*siteLimitEvent{
		"memory_other": {Count: 5, FirstAt: at(time.Hour), LastAt: at(time.Minute), Hits: []string{at(time.Hour), at(time.Minute)}},
		"tasks_other":  {Count: 5, FirstAt: at(time.Hour), LastAt: at(time.Minute), Hits: []string{at(time.Hour), at(time.Minute)}},
	}}}
	raw, _ := json.Marshal(other)
	_ = f.panel.setSetting(context.Background(), settingSiteLimitEvents, string(raw))
	for _, problem := range f.panel.siteLimitProblems(context.Background(), time.Now()) {
		if strings.HasSuffix(problem.Kind, "_repeated") {
			t.Fatalf("an event not at the site's limit made a Problems now line: %+v", problem)
		}
	}
}

// fakeRoleLimiter is an engine's role settings in memory.
type fakeRoleLimiter struct {
	keys     []string
	settings map[string]string
	writes   []string
}

func (f *fakeRoleLimiter) RoleKeysSupported() []string { return f.keys }

func (f *fakeRoleLimiter) ReadUserRoleSettings(string) (services.RoleSettings, error) {
	out := services.RoleSettings{}
	for _, key := range f.keys {
		out[key] = f.settings[key]
	}
	return out, nil
}

func (f *fakeRoleLimiter) SetUserRoleSetting(_ string, key string, value int) error {
	f.writes = append(f.writes, fmt.Sprintf("%s=%d", key, value))
	if value == 0 {
		delete(f.settings, key)
		return nil
	}
	unit := "s"
	if key == services.RoleKeyTempFileMB {
		unit = "MB"
	}
	f.settings[key] = fmt.Sprintf("%d%s", value, unit)
	return nil
}

// TestADatabaseUsersOwnSettingsSurvive (review 2): a value the server owner
// set by hand, before the panel wrote anything or over the panel's own value,
// is kept and recorded as the owner's; a change of another key writes
// nothing; the panel changes and clears only what it wrote itself.
func TestADatabaseUsersOwnSettingsSurvive(t *testing.T) {
	f := newRightsFixture(t)
	ctx := context.Background()
	engine := &fakeRoleLimiter{keys: services.RoleKeys, settings: map[string]string{services.RoleKeyStatementSeconds: "30s"}}
	apply := func(limits services.RoleLimits) {
		t.Helper()
		if err := f.panel.applyDBRoleLimits(ctx, engine, 3, "shop_user", limits, false); err != nil {
			t.Fatal(err)
		}
	}
	// The owner's statement time, set before the panel wrote anything, is
	// kept under a plan that sets none and under one that sets another.
	apply(services.RoleLimits{})
	apply(services.RoleLimits{StatementSeconds: 60})
	if engine.settings[services.RoleKeyStatementSeconds] != "30s" || len(engine.writes) != 0 {
		t.Fatalf("the owner's value was written over: %v %v", engine.settings, engine.writes)
	}
	record := f.panel.loadDBRoleRecord(ctx)[dbRoleRecordKey(3, "shop_user")]
	if record == nil || record.Owner[services.RoleKeyStatementSeconds] != "30s" {
		t.Fatalf("the owner's value is not recorded as the owner's: %+v", record)
	}
	// The plan's temporary-file limit is the panel's to write.
	apply(services.RoleLimits{StatementSeconds: 60, TempFileMB: 1024})
	if engine.settings[services.RoleKeyTempFileMB] != "1024MB" || len(engine.writes) != 1 {
		t.Fatalf("the plan's value: %v %v", engine.settings, engine.writes)
	}
	// A change of another key (db.connections) writes nothing here.
	apply(services.RoleLimits{StatementSeconds: 60, TempFileMB: 1024})
	if len(engine.writes) != 1 {
		t.Fatalf("an unchanged value was written again: %v", engine.writes)
	}
	// The owner raises the panel's value by hand: kept from then on.
	engine.settings[services.RoleKeyTempFileMB] = "2GB"
	apply(services.RoleLimits{TempFileMB: 512})
	if engine.settings[services.RoleKeyTempFileMB] != "2GB" || len(engine.writes) != 1 {
		t.Fatalf("the owner's change of the panel's value was written over: %v %v", engine.settings, engine.writes)
	}
	// What the panel wrote itself it clears when the plan sets none.
	engine.settings = map[string]string{}
	if err := f.panel.applyDBRoleLimits(ctx, engine, 3, "new_user", services.RoleLimits{IdleInTransactionSeconds: 600}, true); err != nil {
		t.Fatal(err)
	}
	if err := f.panel.applyDBRoleLimits(ctx, engine, 3, "new_user", services.RoleLimits{}, false); err != nil {
		t.Fatal(err)
	}
	if _, set := engine.settings[services.RoleKeyIdleInTransactionSeconds]; set {
		t.Fatalf("the panel's own value was not cleared: %v", engine.settings)
	}
}

// TestALoweredSiteLimitAsksFirst (review 9): lowering a plan's memory below
// what a site's programs use now is refused with the list until the change
// is confirmed in the panel's own confirmation; nothing else changes.
func TestALoweredSiteLimitAsksFirst(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID)
	oldRead, oldApply, oldStart, oldNeeds := readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent
	t.Cleanup(func() {
		readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent = oldRead, oldApply, oldStart, oldNeeds
	})
	siteLimitReadNeedsAgent, startSiteSliceApply = false, func(run func()) { run() }
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		status := siteLimitsStatus(siteID, 0, 0)
		status.Sites[0].MemoryCurrentMB = 900
		return status, nil
	}
	applied := 0
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		applied++
		return transport.ApplySiteSlicesResponse{}, nil
	}
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":1024}}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	applied = 0
	body := `{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":512}`
	refused := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), body+`}`)
	wantStatus(t, refused, http.StatusConflict, errCodePlanNarrowingConfirm)
	if !strings.Contains(refused.Body.String(), `"sites_stopped":["customer.matrix.test: memory 900 MB in use, new limit 512 MB"]`) || applied != 0 {
		t.Fatalf("refusal: %s (applied %d)", refused.Body.String(), applied)
	}
	// The confirmation names the list it confirms (second fix round).
	var refusal struct {
		Preview rightsPreview `json:"preview"`
	}
	_ = json.Unmarshal(refused.Body.Bytes(), &refusal)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), body+`,"confirm_narrowing":true,"confirm_stopped":"`+refusal.Preview.StoppedToken+`"}`), http.StatusOK, "")
	if applied != 1 {
		t.Fatalf("the confirmed change was not applied (%d)", applied)
	}
	// Raising it asks nothing.
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan),
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":2048}}`), http.StatusOK, "")
}

// TestTheCardTellsWhetherTheLimitsApply (review 8, 14): an absent slice file
// and programs all outside the limits are said as such (no "stayed within
// their limits", no live values); the administrator reads the owner's
// drop-ins and the database users' own settings; a reseller is the provider.
func TestTheCardTellsWhetherTheLimitsApply(t *testing.T) {
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID)
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	reading := siteLimitsStatus(siteID, 0, 0)
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	handler := siteLimitsMux(f)
	path := "/api/v1/domains/9902/site-limits"
	get := func(user int) siteLimitsView {
		t.Helper()
		w := siteLimitsCall(t, f, handler, user, http.MethodGet, path, "")
		wantStatus(t, w, http.StatusOK, "")
		var view siteLimitsView
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
	// Third fix round (review W1): the customer reads one neutral state for
	// everything it cannot act on; the administrator reads the reason.
	if view := get(authzMatrixCustomerID); view.Applies != siteLimitsNotYet || view.Live != nil {
		t.Fatalf("programs outside the limits: %+v", view)
	}
	if view := get(authzMatrixAdminID); view.Applies != siteLimitsOutside || view.Live != nil {
		t.Fatalf("programs outside the limits (administrator): %+v", view)
	}
	reading.Sites[0].File = transport.SiteFileAbsent
	reading.Sites[0].Named = true // a unit runs in the slice: the file was there
	reading.Sites[0].DropIns = []string{"owner.conf"}
	reading.Sites[0].Units[0].Placement, reading.Sites[0].Units[0].Slice = transport.SiteUnitInSlice, reading.Sites[0].Slice
	if view := get(authzMatrixCustomerID); view.Applies != siteLimitsNotYet || view.Live != nil || len(view.DropIns) != 0 {
		t.Fatalf("no slice file: %+v", view)
	}
	admin := get(authzMatrixAdminID)
	if admin.Applies != siteLimitsNoFile || admin.SliceFile != transport.SiteFileAbsent || len(admin.DropIns) != 1 || admin.DropIns[0] != "owner.conf" {
		t.Fatalf("administrator: %+v", admin)
	}
	reseller := get(authzMatrixResellerID)
	if !reseller.Reseller || reseller.Admin || reseller.Applies != siteLimitsNotYet {
		t.Fatalf("reseller: %+v", reseller)
	}
	reading.Sites[0].File = transport.SiteFileManagedUnchanged
	reading.Sites[0].KernelState = transport.SiteSliceInForce // review N3: the kernel holds them
	if view := get(authzMatrixCustomerID); view.Applies != siteLimitsApply || view.Live == nil {
		t.Fatalf("inside the limits: %+v", view)
	}
	reading.Unsupported = "cgroup v2"
	if view := get(authzMatrixCustomerID); view.Applies != siteLimitsNotYet || view.Unsupported != "" {
		t.Fatalf("unsupported host: %+v", view)
	}
	if view := get(authzMatrixAdminID); view.Applies != siteLimitsUnsupported || view.Unsupported != "cgroup v2" {
		t.Fatalf("unsupported host (administrator): %+v", view)
	}
}

// TestProgramsTheMoveCannotTakeAreNotCountedOutside (review 10): a program
// whose file the owner changed, one in an earlier protection and one an
// owner's drop-in places are named on the card with who acts, but never keep
// the "outside" line; a stopped one whose drop-in names another slice is the
// owner's, not "would start outside".
func TestProgramsTheMoveCannotTakeAreNotCountedOutside(t *testing.T) {
	slice := "celiksite_7.slice"
	status := transport.SiteSliceStatus{SiteID: 7, Slice: slice, Units: []transport.SiteSliceUnit{
		{Unit: "celikapp-7.service", Placement: transport.SiteUnitOutside, Slice: "system.slice", File: "edited"},
		{Unit: "celikrt-5.service", Placement: transport.SiteUnitOutside, Slice: "system.slice", File: "older"},
		{Unit: "celiksvc-6.service", Placement: transport.SiteUnitOutside, Slice: "system.slice", File: "ours", DropInSlice: "system.slice"},
		{Unit: "celikrt-8.service", Placement: transport.SiteUnitStopped, Slice: "owner.slice", File: "ours", DropInSlice: "owner.slice"},
	}}
	if n := siteUnitsOutside(status); n != 0 {
		t.Fatalf("%d programs the move cannot take were counted outside", n)
	}
	want := map[string]string{"celikapp-7.service": "edited", "celikrt-5.service": "older", "celiksvc-6.service": "owner_slice", "celikrt-8.service": "owner_slice"}
	for _, u := range status.Units {
		if got := siteUnitBlocked(u); got != want[u.Unit] {
			t.Errorf("%s blocked = %q, want %q", u.Unit, got, want[u.Unit])
		}
	}
	if got := siteUnitPlacement(status.Units[3], slice); got != transport.SiteUnitElsewhere {
		t.Errorf("stopped in the owner's slice: %s", got)
	}
	status.Units = append(status.Units, transport.SiteSliceUnit{Unit: "celikrt-9.service", Placement: transport.SiteUnitStopped, Slice: "system.slice", File: "ours"})
	if n := siteUnitsOutside(status); n != 1 {
		t.Fatalf("a stopped program that would start outside: %d", n)
	}
}

// TestTheMemoryDefaultIsBoundToTheHost (review 11): without any level, a
// site's memory limit is the smaller of 3072 MB and 75 % of the host's.
func TestTheMemoryDefaultIsBoundToTheHost(t *testing.T) {
	for host, want := range map[int]int{0: 3072, 2048: 1536, 3900: 2925, 8192: 3072, 64: 64} {
		useHostMemory(t, host)
		if got := siteMemoryCodeDefault(); got != want {
			t.Errorf("host %d MB: %d, want %d", host, got, want)
		}
	}
	useHostMemory(t, 2048)
	if got := siteLimitValueOf(settingLimits{}, settingSiteMemoryMB); got.Value != 1536 || got.SetBy != "default" {
		t.Fatalf("value without a level: %+v", got)
	}
}

// TestAHostWithoutLimitsIsOneLine (review "not confirmed"): a host without
// the unified cgroup hierarchy or its controllers gives ONE administrator line
// naming what is missing; it goes when the host can enforce limits.
func TestAHostWithoutLimitsIsOneLine(t *testing.T) {
	f := newRightsFixture(t)
	now := time.Now()
	f.panel.noteSiteLimitHost(context.Background(), "memory, pids", now)
	lines := siteLimitHostProblem(f.panel.setting(context.Background(), settingSiteLimitHost))
	if len(lines) != 1 || lines[0].Kind != "site_limits_unsupported" || lines[0].Actor != "administrator" || lines[0].Subject != "memory, pids" {
		t.Fatalf("lines = %+v", lines)
	}
	if _, ordered := problemKindOrder["site_limits_unsupported"]; !ordered {
		t.Error("no place in the problem order")
	}
	if en, _ := alertLine(pendingAlertMail{kind: "site_limits_unsupported", subject: "cgroup v2"}); !strings.Contains(en, "cgroup v2") {
		t.Errorf("alert line: %q", en)
	}
	f.panel.noteSiteLimitHost(context.Background(), "", now)
	if lines := siteLimitHostProblem(f.panel.setting(context.Background(), settingSiteLimitHost)); len(lines) != 0 {
		t.Fatalf("the line stayed: %+v", lines)
	}
}

// TestAResellerCannotSetASiteLimitAboveItsOwn (review "not confirmed"): a
// reseller's own plan and its customer's Custom set are both refused above
// the reseller's ceiling.
func TestAResellerCannotSetASiteLimitAboveItsOwn(t *testing.T) {
	f := newRightsFixture(t)
	body := `{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},` +
		`"quantities":{"max_domains":10},"ceilings":{"site.memory_mb":1024,"site.tasks":128},"overselling":"not_allowed","confirm":true}`
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/rights", body), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"R","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":2048}}`),
		http.StatusConflict, errCodeSettingAboveLimit)
	custom := `{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},` +
		`"quantities":{"max_domains":1},"ceilings":{"site.tasks":4096},"confirm":true}`
	if w := f.call(t, authzMatrixResellerID, http.MethodPut, "/api/v1/subscriptions/9802/rights", custom); w.Code == http.StatusOK {
		t.Fatalf("a reseller set a customer's site limit above its own: %s", w.Body.String())
	}
	limits, _ := subscriptionSettingLimits(context.Background(), f.panel.db.GetDB(), 9802)
	if got := siteLimitValueOf(limits, settingSiteTasks); got.Value > 128 {
		t.Fatalf("the customer's site processes are %+v, above the reseller's 128", got)
	}
}

// TestFiveThousandSitesAreReadAndWritten (review 12): a reading of 5000 sites
// goes in pages, never refused; a limit change goes in batches of at most 100
// sites; a customer polling the card writes nothing when nothing changed.
func TestFiveThousandSitesAreReadAndWritten(t *testing.T) {
	f := newRightsFixture(t)
	db := f.panel.db.GetDB()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		res, err := tx.Exec(`INSERT INTO domains (subscription_id, name) VALUES (9802, ?)`, fmt.Sprintf("s%04d.matrix.test", i))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO sites (domain_id, document_root, project_type) VALUES (?, '/srv/n', 'node')`, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	oldRead, oldApply, oldNeeds := readSiteSlicesCall, applySiteSlicesCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { readSiteSlicesCall, applySiteSlicesCall, siteLimitReadNeedsAgent = oldRead, oldApply, oldNeeds })
	siteLimitReadNeedsAgent = false
	reads, largest := 0, 0
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		reads++
		if len(req.Sites) > largest {
			largest = len(req.Sites)
		}
		resp := transport.SiteSlicesStatusResponse{}
		for _, q := range req.Sites {
			resp.Sites = append(resp.Sites, transport.SiteSliceStatus{SiteID: q.SiteID, Slice: "celiksite_" + itoa(q.SiteID) + ".slice",
				File: transport.SiteFileManagedUnchanged, CountersRead: true,
				Units: []transport.SiteSliceUnit{{Unit: q.Units[0], Placement: transport.SiteUnitOutside, Slice: "system.slice"}}})
		}
		return resp, nil
	}
	started := time.Now()
	if !f.panel.readAllSiteSlices(context.Background()) {
		t.Fatal("the reading of 5001 sites failed")
	}
	if reads != 6 || largest > siteSliceReadPage {
		t.Fatalf("%d reads, the largest of %d sites", reads, largest)
	}
	var outside siteLimitOutside
	_ = json.Unmarshal([]byte(f.panel.setting(context.Background(), settingSiteLimitOutside)), &outside)
	if len(outside.Sites) != 5001 || len(f.panel.loadSiteLimitLedger(context.Background())) != 5001 {
		t.Fatalf("recorded %d outside, %d in the ledger", len(outside.Sites), len(f.panel.loadSiteLimitLedger(context.Background())))
	}
	batches, biggest := 0, 0
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		batches++
		if len(req.Sites) > biggest {
			biggest = len(req.Sites)
		}
		return transport.ApplySiteSlicesResponse{}, nil
	}
	if err := f.panel.reapplySiteSlicesNow(context.Background(), "test", []int{9802}); err != nil {
		t.Fatal(err)
	}
	if batches != 51 || biggest > siteSliceApplyBatch {
		t.Fatalf("%d batches, the biggest of %d sites", batches, biggest)
	}
	if done, total := siteLimitApplying.read(); done != 0 || total != 0 {
		t.Fatalf("the progress stayed after the end: %d/%d", done, total)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Minute {
		t.Fatalf("5001 sites took %s", elapsed)
	}
	// A customer polling the card: nothing changed, nothing is written.
	f.exec(t, `UPDATE panel_settings SET updated_at = '2000-01-01 00:00:00' WHERE key IN ('site_limit_events_v1', 'site_limit_outside_v1')`)
	handler := siteLimitsMux(f)
	for i := 0; i < 3; i++ {
		wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/site-limits", ""), http.StatusOK, "")
	}
	var rewritten int
	_ = db.QueryRow(`SELECT COUNT(*) FROM panel_settings WHERE key IN ('site_limit_events_v1', 'site_limit_outside_v1') AND updated_at <> '2000-01-01 00:00:00'`).Scan(&rewritten)
	if rewritten != 0 {
		t.Fatalf("reading the card rewrote %d records", rewritten)
	}
}
