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

// alpha.149 fix round A: the Panel's side of what the alpha.148 lab found in
// the per-site limits (D6, O3, D3, CHECK 2, D5).

// TestAProgramThatDoesNotRunAfterAMoveIsNamed (D6, decision 1): a kept move
// whose program was put back but does not run is ONE line that names the
// program (the page then says it is not running, who acts and the next step);
// a kept move with no such program keeps the general line.
func TestAProgramThatDoesNotRunAfterAMoveIsNamed(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	oldRead, oldNeeds, oldStart := readSiteSlicesCall, siteLimitReadNeedsAgent, startSiteLimitRead
	t.Cleanup(func() { readSiteSlicesCall, siteLimitReadNeedsAgent, startSiteLimitRead = oldRead, oldNeeds, oldStart })
	siteLimitReadNeedsAgent, startSiteLimitRead = false, func(func()) {}
	unit := "celikapp-" + itoa(siteID) + ".service"
	reading := siteLimitsStatus(siteID, 0, 0)
	reading.Sites[0].MoveInterrupted = &transport.SiteSliceMoveRecovery{At: "2026-11-26T08:00:00Z", Kept: true, Units: map[string]string{unit: transport.SiteMoveFailed}}
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return reading, nil
	}
	handler := siteLimitsMux(f)
	line := func() monitoringProblem {
		t.Helper()
		_ = round3Card(t, f, handler, authzMatrixAdminID)
		var found []monitoringProblem
		for _, p := range f.panel.siteLimitProblems(ctx, time.Now()) {
			if p.Kind == "site_move_kept" {
				found = append(found, p)
			}
		}
		if len(found) != 1 {
			t.Fatalf("%d lines for the kept move", len(found))
		}
		return found[0]
	}
	if p := line(); p.Name != unit || p.Actor != "administrator" || p.Action.To != "/domains/customer.matrix.test" {
		t.Fatalf("a program that does not run is not named: %+v", p)
	}
	// Still starting (not known to be stopped): the general line.
	reading.Sites[0].MoveInterrupted = &transport.SiteSliceMoveRecovery{At: "2026-11-26T08:01:00Z", Kept: true, Units: map[string]string{unit: transport.SiteMoveStarting}}
	if p := line(); p.Name != "" {
		t.Fatalf("a program still starting is named as not running: %+v", p)
	}
}

// TestAnEventKeepsTheLimitThatHeldThen (CHECK 2, decision 6): an event is
// recorded with the limit in force when it happened; a later change of the
// limits does not change what the card says of it.
func TestAnEventKeepsTheLimitThatHeldThen(t *testing.T) {
	ledger := siteLimitLedger{}
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	status := siteLimitsStatus(7, 0, 0).Sites[0]
	status.Kernel, status.KernelState = transport.SiteSliceLimitsRead{MemoryMB: 128, Tasks: 64, CPUPercent: 50}, transport.SiteSliceInForce
	observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now) // the baseline
	status.Counters.MemoryLimitHits, status.Counters.MemoryOOMAll, status.Counters.OOMKills = 1, 1, 2
	observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now.Add(time.Minute))
	// The plan is raised back to the default afterwards.
	status.Live = transport.SiteSliceLimitsRead{MemoryMB: 3072, Tasks: 512, CPUPercent: 200}
	status.Kernel = status.Live
	observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now.Add(2*time.Minute))
	views := siteLimitEventViews(ledger["7"], now.Add(3*time.Minute))
	if len(views) != 1 || views[0].Kind != "memory" || views[0].Limit != 128 {
		t.Fatalf("the event lost the limit that held then: %+v", views)
	}
}

// TestAReachedProcessLimitSeenByThePeak (D3, decision 5): on a kernel without
// pids.events.local the slice's peak reaching its limit since the last
// reading is a "tasks_peak" event without a count; a peak that stays at the
// limit is not counted again.
func TestAReachedProcessLimitSeenByThePeak(t *testing.T) {
	ledger := siteLimitLedger{}
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	status := siteLimitsStatus(7, 0, 0).Sites[0]
	status.Counters.TasksRule = "units"
	status.Kernel, status.KernelState = transport.SiteSliceLimitsRead{MemoryMB: 256, Tasks: 64, CPUPercent: 50}, transport.SiteSliceInForce
	observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now)
	status.Counters.TasksPeakAtLimit = true
	if !observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now.Add(time.Minute)) {
		t.Fatal("the peak at the limit was not recorded")
	}
	event := ledger["7"].Events["tasks_peak"]
	if event == nil || event.Count != 0 || event.Limit != 64 || event.LastAt == "" {
		t.Fatalf("the reached process limit: %+v", event)
	}
	if observeSiteSlices(ledger, []transport.SiteSliceStatus{status}, true, now.Add(2*time.Minute)) {
		t.Fatal("a peak that stays at the limit was counted again")
	}
}

// TestTheResellersBoundIsNotTheAdministratorsLimit (CHECK 2, decision 6):
// under a reseller, where neither the server nor the reseller set a ceiling,
// the server's default bounds the site; the card names it as that default,
// not as a limit the administrator set.
func TestTheResellersBoundIsNotTheAdministratorsLimit(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	old := readSiteSlicesCall
	t.Cleanup(func() { readSiteSlicesCall = old })
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		return siteLimitsStatus(siteID, 0, 0), nil
	}
	view := round3Card(t, f, siteLimitsMux(f), authzMatrixAdminID)
	if len(view.Limits) != 3 {
		t.Fatalf("limits: %+v", view.Limits)
	}
	for _, l := range view.Limits {
		if l.SetBy != siteLimitSetByResellerBound || l.SetByName != "" {
			t.Fatalf("%s: set by %q (%q)", l.Key, l.SetBy, l.SetByName)
		}
	}
}

// TestALoweredLimitsSitesAreValues (CHECK 2, decision 2): the preview sends
// each site's use and the new limit as values the page words in its own
// language; the English lines the token names stay as they were.
func TestALoweredLimitsSitesAreValues(t *testing.T) {
	useHostMemory(t, 4096)
	t.Cleanup(func() { setSiteMemoryFrozen(0) })
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	f.panel.freezeSiteMemoryDefault(ctx)
	useHostMemory(t, 2048)
	oldRead, oldNeeds := readSiteSlicesCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { readSiteSlicesCall, siteLimitReadNeedsAgent = oldRead, oldNeeds })
	siteLimitReadNeedsAgent = false
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		status := siteLimitsStatus(siteID, 0, 0)
		status.Sites[0].MemoryCurrentMB = 2000
		return status, nil
	}
	mux := f.panel.requireAuth(http.HandlerFunc(f.panel.handleRightsRoutes))
	preview := siteLimitsCall(t, f, mux, authzMatrixAdminID, http.MethodPost, "/api/v1/rights/site-memory-default", `{"action":"preview"}`)
	wantStatus(t, preview, http.StatusOK, "")
	var answer struct {
		Preview rightsPreview `json:"preview"`
	}
	_ = json.Unmarshal(preview.Body.Bytes(), &answer)
	items := answer.Preview.SitesStoppedItems
	if len(items) != 1 || items[0].Domain != "customer.matrix.test" || items[0].MemoryMB != 2000 || items[0].MemoryLimitMB != 1536 || items[0].TasksLimit != 0 {
		t.Fatalf("items: %s", preview.Body.String())
	}
	if len(answer.Preview.SitesStopped) != 1 || !strings.Contains(answer.Preview.SitesStopped[0], "memory 2000 MB in use, new limit 1536 MB") {
		t.Fatalf("the token's lines changed: %+v", answer.Preview.SitesStopped)
	}
}

// TestTheUnappliedLineCarriesACode (O3, decisions 2 and 3): the recorded
// reason of a change that did not reach every site is a code the page words;
// the English detail goes to the journal.
func TestTheUnappliedLineCarriesACode(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	ctx := context.Background()
	oldApply, oldNeeds := applySiteSlicesCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { applySiteSlicesCall, siteLimitReadNeedsAgent = oldApply, oldNeeds })
	siteLimitReadNeedsAgent = false
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		return transport.ApplySiteSlicesResponse{Sites: []transport.SiteSliceApplied{{SiteID: siteID, Outcome: transport.SiteFileOutcomeFailed,
			Reason: "kernel_differs", Detail: "the limits file is written, but the running programs do not have the limits yet: the kernel still holds the earlier memory.max"}}}, nil
	}
	if err := f.panel.reapplySiteSlicesNow(ctx, "Lean", []int{9802}); err != nil {
		t.Fatal(err)
	}
	lines := siteLimitApplyProblem(f.panel.setting(ctx, settingSiteLimitApply))
	if len(lines) != 1 || lines[0].Name != "kernel_differs" {
		t.Fatalf("lines = %+v", lines)
	}
}

// TestAnUpdateIsNotStartedDuringAMove (D5, decision 8b): while a site's
// programs are being moved into their limits the panel refuses its own
// update with a code the update page words (wait for the move's window,
// then press update again); the agent's own refusal is answered the same way.
func TestAnUpdateIsNotStartedDuringAMove(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	siteMoveMu.Lock()
	siteMoveRunning = true
	siteMoveMu.Unlock()
	t.Cleanup(func() {
		siteMoveMu.Lock()
		siteMoveRunning = false
		siteMoveMu.Unlock()
	})
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateStart(recorder, systemUpdateRequest(
		http.MethodPost, panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion), roleAdmin,
	))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "PANEL_UPDATE_SITE_MOVE_RUNNING") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	fixture.agent.mu.Lock()
	starts := fixture.agent.startCalls
	fixture.agent.mu.Unlock()
	if starts != 0 {
		t.Fatalf("the agent was asked to start the update %d times during a move", starts)
	}
	writer := httptest.NewRecorder()
	writePanelUpdateSiteMoveRunning(writer)
	if writer.Code != http.StatusConflict || !strings.Contains(writer.Body.String(), "PANEL_UPDATE_SITE_MOVE_RUNNING") {
		t.Fatalf("the agent's refusal: %d %s", writer.Code, writer.Body.String())
	}
}
