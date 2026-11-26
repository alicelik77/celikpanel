package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a second fix round: the Panel's side of what the second review of PR
// #349 found (review B2, B3, B5, B7, B9, C3, finding 9's leftovers, and the
// reseller's default bound).

func round2Site(t *testing.T, f rightsFixture) int {
	t.Helper()
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	return siteID
}

// TestTheCardSaysWhyTheLimitsDoNotApply (B2): right after the update an
// existing site's programs run outside and no slice file was ever written:
// the card says the programs are outside (not that a file is missing), and
// the administrator's move is offered. Only where a unit names the slice (or
// runs in it) is an absent file "missing", and then the site counts in the
// "outside" line and the move (which writes the file) is offered too.
func TestTheCardSaysWhyTheLimitsDoNotApply(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	// The real state of an existing site after the update.
	reading := transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{{
		SiteID: siteID, Slice: "celiksite_" + itoa(siteID) + ".slice", File: transport.SiteFileAbsent, Named: false,
		Units: []transport.SiteSliceUnit{{Unit: "celikapp-" + itoa(siteID) + ".service", Placement: transport.SiteUnitOutside, Slice: "system.slice", File: "ours"}},
	}}}
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	handler := siteLimitsMux(f)
	get := func(user int) siteLimitsView {
		t.Helper()
		w := siteLimitsCall(t, f, handler, user, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
		wantStatus(t, w, http.StatusOK, "")
		var view siteLimitsView
		_ = json.Unmarshal(w.Body.Bytes(), &view)
		return view
	}
	// Third fix round (review W1): the customer reads the neutral state.
	if view := get(authzMatrixCustomerID); view.Applies != siteLimitsNotYet {
		t.Fatalf("an existing site after the update: %q", view.Applies)
	}
	if view := get(authzMatrixAdminID); view.Applies != siteLimitsOutside || view.NeedsMove != 1 {
		t.Fatalf("administrator after the update: %+v", view)
	}
	// A unit runs in the slice, and the file is gone.
	reading.Sites[0].Named = true
	reading.Sites[0].Units[0].Placement, reading.Sites[0].Units[0].Slice = transport.SiteUnitInSlice, reading.Sites[0].Slice
	if view := get(authzMatrixAdminID); view.Applies != siteLimitsNoFile || view.NeedsMove != 1 {
		t.Fatalf("a file that was there and is gone: %+v", view)
	}
	var outside siteLimitOutside
	_ = json.Unmarshal([]byte(f.panel.setting(context.Background(), settingSiteLimitOutside)), &outside)
	if outside.Sites[itoa(siteID)] != 1 {
		t.Fatalf("the site without its file is not in the outside line: %+v", outside)
	}
}

// TestAResellersSiteLimitsAreBoundedByDefault (reseller bound): without a
// ceiling from the administrator, a reseller's field for the site keys is the
// server's default (host-bound for memory), set by the server administrator;
// a reseller plan above it is refused, and so is a customer's site above it
// even where an older plan says more. The administrator raises it by giving
// the reseller a ceiling.
func TestAResellersSiteLimitsAreBoundedByDefault(t *testing.T) {
	useHostMemory(t, 4096)
	f := newRightsFixture(t)
	ctx := context.Background()
	db := f.panel.db.GetDB()
	field, err := resellerSettingLimits(ctx, db, authzMatrixResellerID)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{settingSiteMemoryMB: 3072, settingSiteTasks: 512, settingSiteCPUPercent: 200} {
		if l := field[key]; l.Ceiling == nil || *l.Ceiling != want || l.SetBy != settingSetByServer {
			t.Errorf("%s in the reseller's field: %+v, want %d set by the server", key, l, want)
		}
	}
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Big","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":65536}}`),
		http.StatusConflict, errCodeSettingAboveLimit)
	if limits, _ := subscriptionSettingLimits(ctx, db, 9802); siteLimitValueOf(limits, settingSiteCPUPercent).Value > 200 {
		t.Fatalf("a reseller's customer above the default: %+v", siteLimitValueOf(limits, settingSiteCPUPercent))
	}
	// The administrator gives the reseller more.
	body := `{"state":"custom","rights":{"all_permissions":true,"all_software":true,"permissions":[],"software":[]},` +
		`"quantities":{"max_domains":10},"ceilings":{"site.memory_mb":8192},"overselling":"not_allowed","confirm":true}`
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9702/rights", body), http.StatusOK, "")
	wantStatus(t, f.call(t, authzMatrixResellerID, http.MethodPost, "/api/v1/plans",
		`{"name":"Big","max_domains":1,"max_databases":1,"max_email_accounts":1,"disk_quota_mb":1,"bandwidth_quota_mb":0,"ceilings":{"site.memory_mb":6144}}`),
		http.StatusOK, "")
}

// TestALimitChangeThatDidNotReachEverySiteIsALine (B3): a batch the Agent did
// not answer, and a site whose write failed, are recorded; the administrator
// reads one line (which change, how many sites, why); the hourly round writes
// them again and the line goes. Two changes never interleave their batches.
func TestALimitChangeThatDidNotReachEverySiteIsALine(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	oldApply, oldNeeds := applySiteSlicesCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { applySiteSlicesCall, siteLimitReadNeedsAgent = oldApply, oldNeeds })
	siteLimitReadNeedsAgent = false
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		return transport.ApplySiteSlicesResponse{}, errors.New("timeout")
	}
	if err := f.panel.reapplySiteSlicesNow(ctx, "Lean", []int{9802}); err != nil {
		t.Fatal(err)
	}
	lines := siteLimitApplyProblem(f.panel.setting(ctx, settingSiteLimitApply))
	if len(lines) != 1 || lines[0].Kind != "site_limits_unapplied" || lines[0].Value != 1 || lines[0].Subject != "Lean" ||
		lines[0].Name != "agent_unreachable:1" || lines[0].Actor != "administrator" { // alpha.149: codes with their sites, worded by the page
		t.Fatalf("lines = %+v", lines)
	}
	problems := f.panel.siteLimitProblems(ctx, time.Now())
	found := false
	for _, p := range problems {
		found = found || p.Kind == "site_limits_unapplied"
	}
	if !found {
		t.Fatalf("Problems now has no line for it: %+v", problems)
	}
	// A failed write of the site is recorded the same way.
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		return transport.ApplySiteSlicesResponse{Sites: []transport.SiteSliceApplied{{SiteID: siteID, Outcome: transport.SiteFileOutcomeFailed, Detail: "verify"}}}, nil
	}
	f.panel.retrySiteLimitApply(ctx)
	if lines := siteLimitApplyProblem(f.panel.setting(ctx, settingSiteLimitApply)); len(lines) != 1 || lines[0].Name != "failed:1" { // an Agent without a code
		t.Fatalf("a failed write: %+v", lines)
	}
	// The hourly round reaches it: the line goes.
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		return transport.ApplySiteSlicesResponse{Sites: []transport.SiteSliceApplied{{SiteID: siteID, Outcome: transport.SiteFileOutcomeWritten}}}, nil
	}
	f.panel.retrySiteLimitApply(ctx)
	if raw := f.panel.setting(ctx, settingSiteLimitApply); raw != "" {
		t.Fatalf("the record stayed: %s", raw)
	}
	// Two changes at once: the later one waits for the first.
	var mu sync.Mutex
	var order []string
	release := make(chan struct{})
	first := true
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		mu.Lock()
		me := "second"
		if first {
			me, first = "first", false
		}
		order = append(order, me+" start")
		mu.Unlock()
		if me == "first" {
			<-release
		}
		mu.Lock()
		order = append(order, me+" end")
		mu.Unlock()
		return transport.ApplySiteSlicesResponse{}, nil
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = f.panel.reapplySiteSlicesNow(ctx, "A", []int{9802}) }()
	time.Sleep(50 * time.Millisecond)
	go func() { defer wg.Done(); _ = f.panel.reapplySiteSlicesNow(ctx, "B", []int{9802}) }()
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if strings.Join(order, ",") != "first start,first end,second start,second end" {
		t.Fatalf("two changes interleaved: %v", order)
	}
}

// TestTheDefaultDoesNotMoveByItself (B5): the host-bound default is fixed
// when first computed; a later change of the host's memory changes nothing by
// itself and is one line whose action previews the sites a lower default
// stops, refuses an apply without that list, and applies with it.
func TestTheDefaultDoesNotMoveByItself(t *testing.T) {
	useHostMemory(t, 4096)
	t.Cleanup(func() { setSiteMemoryFrozen(0) })
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	f.panel.freezeSiteMemoryDefault(ctx)
	if got := siteMemoryCodeDefault(); got != 3072 {
		t.Fatalf("the fixed default: %d", got)
	}
	useHostMemory(t, 2048)
	if got := siteMemoryCodeDefault(); got != 3072 {
		t.Fatalf("the default moved with the host's memory: %d", got)
	}
	f.panel.freezeSiteMemoryDefault(ctx) // a restart keeps it
	if got := siteMemoryCodeDefault(); got != 3072 {
		t.Fatalf("a restart moved the default: %d", got)
	}
	lines := f.panel.siteMemoryDefaultProblem(ctx)
	if len(lines) != 1 || lines[0].Kind != "site_memory_default" || lines[0].Value != 1536 || lines[0].Subject != "3072" || lines[0].Action.Kind != "apply_default" {
		t.Fatalf("lines = %+v", lines)
	}
	oldRead, oldApply, oldStart, oldNeeds := readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent
	t.Cleanup(func() {
		readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent = oldRead, oldApply, oldStart, oldNeeds
	})
	siteLimitReadNeedsAgent, startSiteSliceApply = false, func(run func()) { run() }
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		status := siteLimitsStatus(siteID, 0, 0)
		status.Sites[0].MemoryCurrentMB = 2000
		return status, nil
	}
	var sent []transport.SiteSlice
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		sent = append(sent, req.Sites...)
		return transport.ApplySiteSlicesResponse{}, nil
	}
	mux := f.panel.requireAuth(http.HandlerFunc(f.panel.handleRightsRoutes))
	path := "/api/v1/rights/site-memory-default"
	wantStatus(t, siteLimitsCall(t, f, mux, authzMatrixResellerID, http.MethodPost, path, `{"action":"preview"}`), http.StatusForbidden, errCodeAdminOnly)
	preview := siteLimitsCall(t, f, mux, authzMatrixAdminID, http.MethodPost, path, `{"action":"preview"}`)
	wantStatus(t, preview, http.StatusOK, "")
	var answer struct {
		Preview rightsPreview `json:"preview"`
	}
	_ = json.Unmarshal(preview.Body.Bytes(), &answer)
	if answer.Preview.SitesStoppedCount != 1 || answer.Preview.StoppedToken == "" {
		t.Fatalf("preview: %s", preview.Body.String())
	}
	wantStatus(t, siteLimitsCall(t, f, mux, authzMatrixAdminID, http.MethodPost, path, `{"action":"apply"}`), http.StatusConflict, errCodeRightsConfirm)
	if len(sent) != 0 || siteMemoryCodeDefault() != 3072 {
		t.Fatalf("an apply without the list changed something: %v %d", sent, siteMemoryCodeDefault())
	}
	wantStatus(t, siteLimitsCall(t, f, mux, authzMatrixAdminID, http.MethodPost, path, `{"action":"apply","confirm_stopped":"`+answer.Preview.StoppedToken+`"}`), http.StatusOK, "")
	if siteMemoryCodeDefault() != 1536 || len(sent) != 1 || sent[0].MemoryMB != 1536 {
		t.Fatalf("applied: default %d, sent %+v", siteMemoryCodeDefault(), sent)
	}
	if lines := f.panel.siteMemoryDefaultProblem(ctx); len(lines) != 0 {
		t.Fatalf("the line stayed: %+v", lines)
	}
}

// TestALoweringWithoutTheListIsRefused (finding 9's leftovers): a lowered
// site limit below what sites use now is saved only with the token of the
// list the server answers now; a confirmation without it (or with an earlier
// list's) is refused with the list. The preview says what a lowered process
// limit does: new processes are refused, running ones keep running.
func TestALoweringWithoutTheListIsRefused(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	oldRead, oldApply, oldStart, oldNeeds := readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent
	t.Cleanup(func() {
		readSiteSlicesCall, applySiteSlicesCall, startSiteSliceApply, siteLimitReadNeedsAgent = oldRead, oldApply, oldStart, oldNeeds
	})
	siteLimitReadNeedsAgent, startSiteSliceApply = false, func(run func()) { run() }
	tasks := int64(300)
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		status := siteLimitsStatus(siteID, 0, 0)
		status.Sites[0].TasksCurrent = tasks
		return status, nil
	}
	applied := 0
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		applied++
		return transport.ApplySiteSlicesResponse{}, nil
	}
	plan := createPlan(t, f.accountsFixture, authzMatrixAdminID,
		`{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.tasks":512}}`)
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/users/9703/plan", `{"plan_id":`+itoa(plan)+`}`), http.StatusOK, "")
	applied = 0
	body := `{"name":"Lean","max_domains":4,"max_databases":4,"max_email_accounts":4,"disk_quota_mb":400,"bandwidth_quota_mb":0,"ceilings":{"site.tasks":128},"confirm_narrowing":true`
	blind := f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), body+`}`)
	wantStatus(t, blind, http.StatusConflict, errCodePlanNarrowingConfirm)
	var refusal struct {
		Preview rightsPreview `json:"preview"`
	}
	_ = json.Unmarshal(blind.Body.Bytes(), &refusal)
	if refusal.Preview.StoppedToken == "" || applied != 0 || !strings.Contains(refusal.Preview.Message, "new processes are refused and running ones keep running") {
		t.Fatalf("a blind confirmation: %s (applied %d)", blind.Body.String(), applied)
	}
	// The sites' use changed since the list: its token is refused again.
	tasks = 400
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), body+`,"confirm_stopped":"`+refusal.Preview.StoppedToken+`"}`),
		http.StatusConflict, errCodePlanNarrowingConfirm)
	tasks = 300
	wantStatus(t, f.call(t, authzMatrixAdminID, http.MethodPut, "/api/v1/plans/"+itoa(plan), body+`,"confirm_stopped":"`+refusal.Preview.StoppedToken+`"}`), http.StatusOK, "")
	if applied != 1 {
		t.Fatalf("the confirmed change was not applied (%d)", applied)
	}
}

// TestTheEventRecordIsBounded (B7, C3): readings that rise every second add
// at most one hit per kind every five minutes and never more than a day's;
// the counts stay whole; the memory count is programs stopped, not the raw
// oom counter.
func TestTheEventRecordIsBounded(t *testing.T) {
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	ledger := siteLimitLedger{}
	read := func(at time.Time, oom, kills, tasks int64) {
		observeSiteSlices(ledger, []transport.SiteSliceStatus{{SiteID: 4, CountersRead: true,
			Counters: transport.SiteSliceCounters{MemoryLimitHits: oom, OOMKills: kills, TasksLimitHits: tasks}}}, false, at)
	}
	read(now, 0, 0, 0)
	// F3c: one stopped program while the kernel counted 10576 oom events.
	read(now.Add(time.Second), 10576, 1, 0)
	if e := ledger["4"].Events["memory"]; e == nil || e.Count != 1 {
		t.Fatalf("one stopped program counted as %+v", e)
	}
	for i := int64(1); i <= 3*24*3600/10; i++ {
		read(now.Add(time.Duration(i)*10*time.Second), 10576, 1, i)
	}
	e := ledger["4"].Events["tasks"]
	if e == nil || e.Count != 3*24*3600/10 || len(e.Hits) > siteLimitMaxHits {
		t.Fatalf("tasks: count %d, %d hits", e.Count, len(e.Hits))
	}
	if gap, _ := time.Parse(time.RFC3339, e.Hits[1]); gap.Sub(mustTime(t, e.Hits[0])) < siteLimitHitEvery {
		t.Fatalf("two hits within five minutes: %v", e.Hits[:2])
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

// TestReadsArePagedByPrograms (B9): sites with many programs are read in
// pages bounded by programs too; no site's programs make a reading fail.
func TestReadsArePagedByPrograms(t *testing.T) {
	f := newRightsFixture(t)
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	var pages []int
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		units := 0
		resp := transport.SiteSlicesStatusResponse{}
		for _, q := range req.Sites {
			units += len(q.Units)
			resp.Sites = append(resp.Sites, transport.SiteSliceStatus{SiteID: q.SiteID})
		}
		pages = append(pages, units)
		return resp, nil
	}
	var sites []siteLimitSite
	for i := 1; i <= 3; i++ {
		s := siteLimitSite{SiteID: i}
		for u := 1; u <= 3000; u++ {
			s.Units = append(s.Units, siteLimitUnit{Unit: "celikrt-" + itoa(i*10000+u) + ".service"})
		}
		sites = append(sites, s)
	}
	statuses, err := f.panel.readSiteSlicesPaged(context.Background(), sites)
	if err != nil || len(statuses) != 3 || len(pages) != 3 {
		t.Fatalf("err %v, %d statuses, pages %v", err, len(statuses), pages)
	}
	for _, units := range pages {
		if units > siteSliceReadPageUnits {
			t.Fatalf("a page of %d programs", units)
		}
	}
}
