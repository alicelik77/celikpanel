package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.103 (owner observation 2026-10-23): Frankfurt's update screen offered
// alpha.101, the result of the 12-hour schedule, while alpha.102 had been
// published; only "Check for updates" showed alpha.102. Opening the update
// screen now checks again when the known result is older than ten minutes and
// automatic checks are on, through the same discovery, one check at a time,
// under the schedule's skip and write rules. It never starts anything.

const newerTestTargetVersion = "v0.1.0-alpha.15"

var newerTestTargetCommit = strings.Repeat("e", 40)

// openFixture is a Panel whose stored result names alpha.14, found and
// checked two hours before discoveryTestNow, and whose Agent now answers
// alpha.15.
func openFixture(t *testing.T) systemUpdateTestFixture {
	t.Helper()
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	fixture.panel.lastUpdateAttempt = func(string) (recoveryobs.Attempt, bool) { return recoveryobs.Attempt{}, false }
	fixture.panel.updateDiscovery = newPanelUpdateDiscoveryScheduler(fixture.panel)
	if _, written := fixture.panel.savePanelUpdateDiscovery(context.Background(), discoveryTestNow.Add(-2*time.Hour), true, availableCheck(), nil); !written {
		t.Fatal("the earlier result was not written")
	}
	fixture.agent.mu.Lock()
	fixture.agent.check.TargetVersion, fixture.agent.check.TargetCommit, fixture.agent.check.TargetSequence = newerTestTargetVersion, newerTestTargetCommit, "15"
	fixture.agent.mu.Unlock()
	return fixture
}

func openScreen(t *testing.T, fixture systemUpdateTestFixture, role string) (*httptest.ResponseRecorder, panelUpdateDiscoveryResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscoveryRefresh(recorder, systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryRefreshPath, "", role))
	var view panelUpdateDiscoveryResponse
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
	}
	return recorder, view
}

// openScreenFrom is openScreen for another goroutine: it reports instead of
// stopping the test (t.Fatal must run on the test's own goroutine).
func openScreenFrom(fixture systemUpdateTestFixture, role string) (panelUpdateDiscoveryResponse, error) {
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscoveryRefresh(recorder, systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryRefreshPath, "", role))
	var view panelUpdateDiscoveryResponse
	if recorder.Code != http.StatusOK {
		return view, fmt.Errorf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	err := json.Unmarshal(recorder.Body.Bytes(), &view)
	return view, err
}

func sameDiscoveryRecord(left, right panelUpdateDiscoveryRecord) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func agentCounts(fixture systemUpdateTestFixture) (checks, starts, abandons int) {
	fixture.agent.mu.Lock()
	defer fixture.agent.mu.Unlock()
	return fixture.agent.checkCalls, fixture.agent.startCalls, fixture.agent.abandonCalls
}

func TestPanelUpdateOpeningTheScreenChecksAStaleResultAgain(t *testing.T) {
	fixture := openFixture(t)
	clock := discoveryTestNow
	old := panelUpdateNow
	panelUpdateNow = func() time.Time { return clock }
	t.Cleanup(func() { panelUpdateNow = old })

	recorder, view := openScreen(t, fixture, roleAdmin)
	if recorder.Code != http.StatusOK || view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshChecked {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !view.Available || view.Last == nil || view.Last.Target == nil || view.Last.Target.Version != newerTestTargetVersion ||
		!view.Last.Automatic || view.CheckedAgeSeconds == nil || *view.CheckedAgeSeconds != 0 {
		t.Fatalf("the newest release is not what the screen offers: %s", recorder.Body.String())
	}
	if record, ok := discoveryRecordOf(t, fixture); !ok || record.Target == nil || record.Target.Version != newerTestTargetVersion {
		t.Fatalf("stored=%+v ok=%v", record, ok)
	}
	// The plain read (sidebar, Settings item, Dashboard row) follows it.
	plain := fixture.panel.panelUpdateDiscoveryView(context.Background(), clock)
	if plain.Last == nil || plain.Last.Target == nil || plain.Last.Target.Version != newerTestTargetVersion || plain.Refresh != nil {
		t.Fatalf("the shared read does not follow: %+v", plain.Last)
	}
	// Within ten minutes another opening does not ask again.
	clock = discoveryTestNow.Add(9 * time.Minute)
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshFresh {
		t.Fatalf("a fresh result was checked again: %+v", view.Refresh)
	}
	if checks, _, _ := agentCounts(fixture); checks != 1 {
		t.Fatalf("release reads=%d, want 1", checks)
	}
	// After ten minutes it does, once.
	clock = discoveryTestNow.Add(11 * time.Minute)
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshChecked {
		t.Fatalf("a stale result was not checked: %+v", view.Refresh)
	}
	checks, starts, abandons := agentCounts(fixture)
	if checks != 2 || starts != 0 || abandons != 0 {
		t.Fatalf("checks=%d starts=%d abandons=%d", checks, starts, abandons)
	}
}

func TestPanelUpdateOpeningTheScreenWithAutomaticChecksOffMakesNoContact(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	if err := fixture.panel.setSetting(context.Background(), panelUpdateAutoCheckKey, "off"); err != nil {
		t.Fatal(err)
	}
	fixture.agent.mu.Lock()
	before := fixture.agent.versionCalls + fixture.agent.checkCalls + fixture.agent.readinessCalls + fixture.agent.statusCalls
	fixture.agent.mu.Unlock()
	recorder, view := openScreen(t, fixture, roleAdmin)
	if recorder.Code != http.StatusOK || view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshOff || view.AutoCheck {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	// The stored result is shown with its age; the Agent was not asked.
	if view.Last == nil || view.Last.Target == nil || view.Last.Target.Version != updateTestTargetVersion ||
		view.CheckedAgeSeconds == nil || *view.CheckedAgeSeconds != 7200 {
		t.Fatalf("view=%s", recorder.Body.String())
	}
	fixture.agent.mu.Lock()
	after := fixture.agent.versionCalls + fixture.agent.checkCalls + fixture.agent.readinessCalls + fixture.agent.statusCalls
	fixture.agent.mu.Unlock()
	if after != before {
		t.Fatalf("opening the screen with automatic checks off called the Agent %d time(s)", after-before)
	}
}

func TestPanelUpdateOpeningTheScreenNeverChecksDuringAnUpdate(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	before, _ := discoveryRecordOf(t, fixture)
	// An update start admitted by this Panel in the last hour.
	fixture.panel.markPanelUpdateAdmitted(discoveryTestNow.Add(-10 * time.Minute))
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshSkipped ||
		view.Refresh.ReasonCode != panelUpdateReasonUpdateActive {
		t.Fatalf("refresh=%+v", view.Refresh)
	}
	// The updater holds the host lock.
	fixture.panel.updateAdmittedAt.Store(0)
	fixture.agent.mu.Lock()
	fixture.agent.readiness = transport.HostMutationReadinessResponse{Code: transport.HostMutationBusy, Reason: transport.HostMutationReasonHostLock}
	fixture.agent.mu.Unlock()
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshSkipped ||
		view.Refresh.ReasonCode != panelUpdateReasonServerBusy {
		t.Fatalf("refresh=%+v", view.Refresh)
	}
	// A busy package manager does not hold the read (as on the schedule).
	fixture.agent.mu.Lock()
	fixture.agent.readiness = transport.HostMutationReadinessResponse{Code: transport.HostMutationBusy, Reason: transport.HostMutationReasonPackageManager}
	fixture.agent.mu.Unlock()
	if checks, _, _ := agentCounts(fixture); checks != 0 {
		t.Fatalf("a release read was made during an update: %d", checks)
	}
	after, _ := discoveryRecordOf(t, fixture)
	if !sameDiscoveryRecord(after, before) {
		t.Fatalf("a skip changed the stored result: %+v -> %+v", before, after)
	}
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshChecked {
		t.Fatalf("a busy package manager held the release read: %+v", view.Refresh)
	}
}

func TestPanelUpdateOpeningTheScreenKeepsTheOfferWhenTheCheckFails(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	fixture.agent.mu.Lock()
	fixture.agent.check.Error = "release server unreachable"
	fixture.agent.mu.Unlock()
	_, view := openScreen(t, fixture, roleAdmin)
	if view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshChecked || view.Last == nil ||
		view.Last.Outcome != panelUpdateOutcomeFailed || view.Last.ReasonCode != panelUpdateReasonReleaseUnverified ||
		!view.Available || view.Last.Target == nil || view.Last.Target.Version != updateTestTargetVersion ||
		view.FoundAgeSeconds == nil || *view.FoundAgeSeconds != 7200 {
		t.Fatalf("view=%+v last=%+v", view.Refresh, view.Last)
	}
	// An Agent that does not answer is a skip, as on the schedule: nothing kept.
	before, _ := discoveryRecordOf(t, fixture)
	withDiscoveryClock(t, discoveryTestNow.Add(20*time.Minute))
	fixture.agent.mu.Lock()
	fixture.agent.check.Error = ""
	fixture.agent.checkErr = errors.New("agent did not answer")
	fixture.agent.mu.Unlock()
	_, view = openScreen(t, fixture, roleAdmin)
	if view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshSkipped || view.Refresh.ReasonCode != panelUpdateReasonAgentUnavailable {
		t.Fatalf("refresh=%+v", view.Refresh)
	}
	if after, _ := discoveryRecordOf(t, fixture); !sameDiscoveryRecord(after, before) {
		t.Fatalf("an unanswered check changed the stored result: %+v -> %+v", before, after)
	}
}

// One check at a time: the button's check holds the gate; an opening waits
// for it and then uses its fresh result instead of a second release read.
func TestPanelUpdateOpeningTheScreenNeverRunsASecondConcurrentCheck(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	entered, gate := make(chan struct{}, 4), make(chan struct{})
	fixture.agent.mu.Lock()
	fixture.agent.checkEntered, fixture.agent.checkGate = entered, gate
	fixture.agent.mu.Unlock()
	button := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
		button <- recorder.Code
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the button's check did not reach the Agent")
	}
	type openedView struct {
		view panelUpdateDiscoveryResponse
		err  error
	}
	opened := make(chan openedView, 2)
	for range 2 {
		go func() {
			view, err := openScreenFrom(fixture, roleAdmin)
			opened <- openedView{view, err}
		}()
	}
	// The scheduled check waits for the gate too.
	scheduled := make(chan time.Duration, 1)
	go func() {
		scheduler := fixture.panel.updateDiscovery
		scheduler.now = func() time.Time { return discoveryTestNow }
		scheduler.logf = func(string, ...any) {}
		scheduled <- scheduler.runOnce(context.Background())
	}()
	select {
	case <-entered:
		close(gate)
		t.Fatal("a second release check ran beside the first")
	case <-opened:
		close(gate)
		t.Fatal("an opening answered before the running check ended")
	case <-time.After(300 * time.Millisecond):
	}
	fixture.agent.mu.Lock()
	fixture.agent.checkGate = nil
	fixture.agent.mu.Unlock()
	close(gate)
	if code := <-button; code != http.StatusOK {
		t.Fatalf("button status=%d", code)
	}
	for range 2 {
		select {
		case got := <-opened:
			if got.err != nil {
				t.Fatalf("an opening failed: %v", got.err)
			}
			view := got.view
			if view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshFresh || view.Last == nil ||
				view.Last.Target == nil || view.Last.Target.Version != newerTestTargetVersion {
				t.Fatalf("an opening did not use the result of the check it waited for: %+v", view.Refresh)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("an opening did not answer")
		}
	}
	select {
	case delay := <-scheduled:
		// It waited for the button's check, found it completed just now and
		// did not read again: this period's check is done.
		if delay < 11*time.Hour {
			t.Fatalf("the scheduled check after the button's: delay %s", delay)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduled check did not finish")
	}
	// Only the button's check read the release: not the openings, not the
	// schedule that waited for it.
	if checks, starts, _ := agentCounts(fixture); checks != 1 || starts != 0 {
		t.Fatalf("checks=%d starts=%d", checks, starts)
	}
}

// A request that ends while it waits keeps nothing and says the result is
// not known.
func TestPanelUpdateOpeningTheScreenIsBounded(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	leave, err := fixture.panel.updateCheckGate.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	before, _ := discoveryRecordOf(t, fixture)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	refresh := fixture.panel.refreshPanelUpdateDiscoveryOnOpen(ctx)
	if refresh.Outcome != panelUpdateRefreshUnknown || refresh.ReasonCode != panelUpdateReasonTimeout {
		t.Fatalf("refresh=%+v", refresh)
	}
	if after, _ := discoveryRecordOf(t, fixture); !sameDiscoveryRecord(after, before) {
		t.Fatal("an unfinished opening changed the stored result")
	}
	if checks, _, _ := agentCounts(fixture); checks != 0 {
		t.Fatalf("checks=%d", checks)
	}
	if panelUpdateDiscoveryOpenTimeout <= panelUpdateDiscoveryTimeout || panelUpdateDiscoveryOpenFresh != 10*time.Minute {
		t.Fatalf("bounds: open=%s check=%s window=%s", panelUpdateDiscoveryOpenTimeout, panelUpdateDiscoveryTimeout, panelUpdateDiscoveryOpenFresh)
	}
	// A result dated in the future (a clock set back) does not hold it off.
	future := availableCheck()
	fixture.panel.updateDiscoveryPending = nil
	if _, err := fixture.database.GetDB().Exec(`DELETE FROM panel_settings WHERE key = ?`, panelUpdateDiscoveryKey); err != nil {
		t.Fatal(err)
	}
	fixture.panel.savePanelUpdateDiscovery(context.Background(), discoveryTestNow.Add(2*time.Hour), true, future, nil)
	if fixture.panel.panelUpdateDiscoveryFresh(context.Background(), discoveryTestNow) {
		t.Fatal("a result from the future counted as fresh")
	}
}

func TestPanelUpdateOpeningTheScreenIsAdminOnlyAndRegistered(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	for _, role := range []string{roleReseller, roleCustomer, "additional_user"} {
		if recorder, _ := openScreen(t, fixture, role); recorder.Code != http.StatusForbidden {
			t.Fatalf("role %s status=%d", role, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscoveryRefresh(recorder, systemUpdateRequest(http.MethodPost, panelUpdateDiscoveryRefreshPath, "{}", roleAdmin))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", recorder.Code)
	}
	if checks, _, _ := agentCounts(fixture); checks != 0 {
		t.Fatalf("a refused request reached the Agent: %d", checks)
	}
	// Readable while the licence is locked, as the plain read and the button are.
	if !licenseRecoveryRequest(systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryRefreshPath, "", roleAdmin)) {
		t.Fatal("the opening check is not available while the licence is locked")
	}
	if licenseRecoveryRequest(systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryRefreshPath, "", roleCustomer)) {
		t.Fatal("a customer passed the licence lock")
	}
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "http.HandleFunc(panelUpdateDiscoveryRefreshPath, panel.handlePanelUpdateDiscoveryRefresh)") {
		t.Fatal("main.go does not register the opening check")
	}
}

// A scheduled check that waited while a start was admitted does not run.
func TestPanelUpdateScheduledCheckReadsAdmissionAgainAfterWaiting(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	scheduler := fixture.panel.updateDiscovery
	scheduler.now = func() time.Time { return discoveryTestNow }
	scheduler.logf = func(string, ...any) {}
	leave, err := fixture.panel.updateCheckGate.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan time.Duration, 1)
	go func() { done <- scheduler.runOnce(context.Background()) }()
	time.Sleep(200 * time.Millisecond)
	fixture.panel.markPanelUpdateAdmitted(discoveryTestNow) // admitted while it waited
	leave()
	select {
	case delay := <-done:
		if delay != panelUpdateDiscoverySkipDelay {
			t.Fatalf("delay %s", delay)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scheduled check did not finish")
	}
	if checks, _, _ := agentCounts(fixture); checks != 0 {
		t.Fatalf("a release read was made during an admitted start: %d", checks)
	}
	if _, reason := scheduler.LastSkip(); reason != panelUpdateReasonUpdateActive {
		t.Fatalf("skip=%q", reason)
	}
}

// The opening check is kept when the browser leaves before it finishes: a
// reload does not read the release again.
func TestPanelUpdateOpeningTheScreenKeepsACompletedCheckWhenTheBrowserLeaves(t *testing.T) {
	fixture := openFixture(t)
	withDiscoveryClock(t, discoveryTestNow)
	entered, gate := make(chan struct{}, 2), make(chan struct{})
	fixture.agent.mu.Lock()
	fixture.agent.checkEntered, fixture.agent.checkGate = entered, gate
	fixture.agent.mu.Unlock()
	request := systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryRefreshPath, "", roleAdmin)
	ctx, cancel := context.WithCancel(request.Context()) // keeps the signed-in caller
	request = request.WithContext(ctx)
	finished := make(chan struct{})
	go func() {
		fixture.panel.handlePanelUpdateDiscoveryRefresh(httptest.NewRecorder(), request)
		close(finished)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(gate)
		t.Fatal("the opening check did not reach the Agent")
	}
	cancel() // the browser leaves
	fixture.agent.mu.Lock()
	fixture.agent.checkGate = nil
	fixture.agent.mu.Unlock()
	close(gate)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the opening check did not finish")
	}
	record, ok := discoveryRecordOf(t, fixture)
	if !ok || record.Target == nil || record.Target.Version != newerTestTargetVersion || record.CheckedAt != "2026-10-17T12:00:00Z" {
		t.Fatalf("the completed check was not kept: %+v ok=%v", record, ok)
	}
	// The reload finds it fresh.
	if _, view := openScreen(t, fixture, roleAdmin); view.Refresh == nil || view.Refresh.Outcome != panelUpdateRefreshFresh {
		t.Fatalf("refresh=%+v", view.Refresh)
	}
	if checks, _, _ := agentCounts(fixture); checks != 1 {
		t.Fatalf("checks=%d", checks)
	}
}

// The button's release read is bounded like the schedule's.
func TestPanelUpdateButtonCheckIsBounded(t *testing.T) {
	source, err := os.ReadFile("system_update_handlers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "checkCtx, cancelCheck := context.WithTimeout(r.Context(), panelUpdateDiscoveryTimeout)") {
		t.Fatal("the button's discovery is not bounded at panelUpdateDiscoveryTimeout")
	}
}
