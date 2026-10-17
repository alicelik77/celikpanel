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
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Update discovery (D-035): the scheduled read-only check, its stored
// result, the owner's switch and the status read that makes no Agent call.

var discoveryTestNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func withDiscoveryClock(t *testing.T, now time.Time) {
	t.Helper()
	old := panelUpdateNow
	panelUpdateNow = func() time.Time { return now }
	t.Cleanup(func() { panelUpdateNow = old })
}

func discoveryRecordOf(t *testing.T, fixture systemUpdateTestFixture) (panelUpdateDiscoveryRecord, bool) {
	t.Helper()
	// What is written to panel_settings, not a result kept in memory.
	return fixture.panel.storedPanelUpdateDiscovery(context.Background())
}

func availableCheck() panelUpdateCheckResponse {
	target := panelUpdateTarget{
		Version: updateTestTargetVersion, Commit: updateTestTargetCommit, Sequence: "14",
		OS: "linux", Arch: "amd64", ArchiveSHA256: updateTestTargetSHA, ArchiveSize: "1048576",
	}
	return panelUpdateCheckResponse{
		Supported: true, Available: true, CurrentVersion: updateTestCurrentVersion,
		CurrentCommit: updateTestCurrentCommit, Target: &target,
	}
}

// fakeDiscoveryScheduler drives the scheduler with a fixed clock, no jitter
// unless asked, and counted dependencies.
type fakeDiscoveryScheduler struct {
	*panelUpdateDiscoveryScheduler
	mu        sync.Mutex
	enabledOn bool
	skipWith  string
	results   []*panelUpdateDiscoveryFailure
	discovers int
	records   []string
	flushes   int
	logs      []string
}

func newFakeDiscoveryScheduler() *fakeDiscoveryScheduler {
	fake := &fakeDiscoveryScheduler{enabledOn: true}
	scheduler := &panelUpdateDiscoveryScheduler{
		now:    func() time.Time { return discoveryTestNow },
		jitter: func(time.Duration) time.Duration { return 0 },
		enabled: func(context.Context) bool {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			return fake.enabledOn
		},
		skip: func(context.Context, time.Time) string {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			return fake.skipWith
		},
		discover: func(context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.discovers++
			if len(fake.results) > 0 {
				next := fake.results[0]
				fake.results = fake.results[1:]
				if next != nil {
					return panelUpdateCheckResponse{}, next
				}
			}
			return availableCheck(), nil
		},
		save: func(_ context.Context, _ time.Time, response panelUpdateCheckResponse, failure *panelUpdateDiscoveryFailure) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			switch {
			case failure != nil:
				fake.records = append(fake.records, "failed:"+failure.reason)
			case response.Available:
				fake.records = append(fake.records, "available")
			default:
				fake.records = append(fake.records, "current")
			}
		},
		flush: func(context.Context, time.Time) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.flushes++
		},
		logf: func(format string, args ...any) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.logs = append(fake.logs, fmt.Sprintf(format, args...))
		},
		wake: make(chan struct{}, 1),
	}
	fake.panelUpdateDiscoveryScheduler = scheduler
	return fake
}

func TestPanelUpdateDiscoveryDelaysStayInsideTheirJitterBounds(t *testing.T) {
	scheduler := newPanelUpdateDiscoveryScheduler(&Panel{})
	for i := 0; i < 2000; i++ {
		first := scheduler.firstDelay()
		if first < 90*time.Second || first > 150*time.Second {
			t.Fatalf("first check delay %s outside 2 min +- 30 s", first)
		}
		period := scheduler.periodDelay()
		if period < 11*time.Hour+30*time.Minute || period > 12*time.Hour+30*time.Minute {
			t.Fatalf("period %s outside 12 h +- 30 min", period)
		}
	}
}

func TestPanelUpdateDiscoveryBackoffIsOneThenThreeThenTwelveHours(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	failure := &panelUpdateDiscoveryFailure{reason: panelUpdateReasonReleaseUnverified, err: errors.New("offline")}
	fake.results = []*panelUpdateDiscoveryFailure{failure, failure, failure, failure, nil, failure}
	want := []time.Duration{time.Hour, 3 * time.Hour, 12 * time.Hour, 12 * time.Hour, 12 * time.Hour, time.Hour}
	for index, expected := range want {
		if got := fake.runOnce(context.Background()); got != expected {
			t.Fatalf("attempt %d: next delay %s, want %s", index+1, got, expected)
		}
	}
	if fake.discovers != len(want) {
		t.Fatalf("discovers=%d", fake.discovers)
	}
	// One log line per change of state, not one per attempt: failed, then
	// available, then failed again.
	if len(fake.logs) != 3 {
		t.Fatalf("logs=%q", fake.logs)
	}
}

func TestPanelUpdateDiscoverySkipsWithoutNetworkRead(t *testing.T) {
	for _, reason := range []string{
		panelUpdateReasonUpdateActive, panelUpdateReasonSetupRunning,
		panelUpdateReasonServerBusy, panelUpdateReasonAgentUnavailable,
	} {
		fake := newFakeDiscoveryScheduler()
		fake.skipWith = reason
		if got := fake.runOnce(context.Background()); got != panelUpdateDiscoverySkipDelay {
			t.Fatalf("%s: delay %s", reason, got)
		}
		if fake.discovers != 0 {
			t.Fatalf("%s: a skipped check reached the release read", reason)
		}
		// A skip is kept in memory only: nothing is saved, nothing flushed.
		if len(fake.records) != 0 || fake.flushes != 0 {
			t.Fatalf("%s: records=%q flushes=%d", reason, fake.records, fake.flushes)
		}
		if _, got := fake.LastSkip(); got != reason {
			t.Fatalf("%s: last skip %q", reason, got)
		}
	}
}

func TestPanelUpdateDiscoveryAgentNotAnsweringIsSkippedNotFailed(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	unreachable := &panelUpdateDiscoveryFailure{reason: panelUpdateReasonAgentUnavailable, err: errPanelUpdateAgentUnreachable}
	fake.results = []*panelUpdateDiscoveryFailure{unreachable, unreachable}
	for i := 0; i < 2; i++ {
		if got := fake.runOnce(context.Background()); got != panelUpdateDiscoverySkipDelay {
			t.Fatalf("delay %s", got)
		}
	}
	if _, reason := fake.LastSkip(); fake.failures != 0 || len(fake.records) != 0 || reason != panelUpdateReasonAgentUnavailable {
		t.Fatalf("failures=%d records=%q skip=%q", fake.failures, fake.records, reason)
	}
}

func TestPanelUpdateDiscoverySwitchedOffMakesNoReadAndRecordsNothing(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	fake.enabledOn = false
	fake.runOnce(context.Background())
	if fake.discovers != 0 || len(fake.records) != 0 {
		t.Fatalf("discovers=%d records=%q", fake.discovers, fake.records)
	}
}

func TestPanelUpdateDiscoveryShutdownDuringCheckRecordsNothing(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	ctx, cancel := context.WithCancel(context.Background())
	fake.discover = func(context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure) {
		cancel()
		return panelUpdateCheckResponse{}, &panelUpdateDiscoveryFailure{reason: panelUpdateReasonTimeout, err: context.Canceled}
	}
	fake.runOnce(ctx)
	if len(fake.records) != 0 || fake.failures != 0 {
		t.Fatalf("records=%q failures=%d", fake.records, fake.failures)
	}
}

// The loop waits the first delay, runs, then the period; the owner's switch
// stops it and starts it again; the process's stop signal ends it.
func TestPanelUpdateDiscoveryLoopFollowsTimerSwitchAndShutdown(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	fake.jitter = func(max time.Duration) time.Duration { return max } // the upper bound
	type armed struct {
		delay time.Duration
		fire  chan time.Time
	}
	timers := make(chan armed, 8)
	fake.newTimer = func(delay time.Duration) (<-chan time.Time, func()) {
		fire := make(chan time.Time, 1)
		timers <- armed{delay, fire}
		return fire, func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { fake.loop(ctx); close(done) }()
	wait := func() armed {
		select {
		case timer := <-timers:
			return timer
		case <-time.After(5 * time.Second):
			t.Fatal("the loop armed no timer")
		}
		return armed{}
	}
	first := wait()
	if first.delay != 150*time.Second {
		t.Fatalf("first delay %s", first.delay)
	}
	if next := fake.Next(); !next.Equal(discoveryTestNow.Add(150 * time.Second)) {
		t.Fatalf("next=%s", next)
	}
	first.fire <- discoveryTestNow
	second := wait()
	if second.delay != 12*time.Hour+30*time.Minute {
		t.Fatalf("delay after a check %s", second.delay)
	}
	fake.mu.Lock()
	if fake.discovers != 1 {
		fake.mu.Unlock()
		t.Fatalf("discovers=%d", fake.discovers)
	}
	fake.enabledOn = false
	fake.mu.Unlock()
	fake.Wake()
	deadline := time.Now().Add(5 * time.Second)
	for !fake.Next().IsZero() {
		if time.Now().After(deadline) {
			t.Fatal("switching off did not clear the next check")
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case timer := <-timers:
		t.Fatalf("a switched-off loop armed a timer of %s", timer.delay)
	case <-time.After(50 * time.Millisecond):
	}
	fake.mu.Lock()
	fake.enabledOn = true
	fake.mu.Unlock()
	fake.Wake()
	// Switched on again right after a check: the next read is a period after
	// that check, not two minutes later (review item 9).
	if again := wait(); again.delay != 12*time.Hour+30*time.Minute {
		t.Fatalf("delay after switching on %s", again.delay)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not stop with the process")
	}
	if fake.discovers != 1 {
		t.Fatalf("discovers=%d", fake.discovers)
	}
}

func TestPanelUpdateDiscoverySkipReasonsOnThePanel(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow); reason != "" {
		t.Fatalf("idle server skipped: %s", reason)
	}
	fixture.panel.markPanelUpdateAdmitted(discoveryTestNow)
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(time.Minute)); reason != panelUpdateReasonUpdateActive {
		t.Fatalf("admitted update: %q", reason)
	}
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(61*time.Minute)); reason != "" {
		t.Fatalf("after the admission window: %q", reason)
	}
	for readiness, want := range map[string]string{
		transport.HostMutationReasonHostLock:        panelUpdateReasonServerBusy,
		transport.HostMutationReasonAgentMutation:   panelUpdateReasonServerBusy,
		transport.HostMutationReasonPackageManager:  "",
		transport.HostMutationReasonStateUnverified: panelUpdateReasonStateUnverified,
	} {
		fixture.agent.mu.Lock()
		fixture.agent.readiness = transport.HostMutationReadinessResponse{Code: transport.HostMutationBusy, Reason: readiness}
		if readiness == transport.HostMutationReasonStateUnverified {
			fixture.agent.readiness.Code = transport.HostMutationUnavailable
		}
		fixture.agent.mu.Unlock()
		if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(time.Hour)); reason != want {
			t.Fatalf("readiness %s: %q, want %q", readiness, reason, want)
		}
	}
	fixture.agent.mu.Lock()
	fixture.agent.readiness = transport.HostMutationReadinessResponse{Ready: true}
	fixture.agent.mu.Unlock()
	database := fixture.database.GetDB()
	if _, err := database.Exec(`INSERT INTO users (id, username, password_hash, email, role) VALUES (7, 'owner', 'x', 'owner@example.test', 'admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES('plan-d',1,'{}',7,'now')`); err != nil {
		t.Fatal(err)
	}
	// A setup run waiting for a DNS record does not hold discovery; one that
	// changes the server does (serverSetupExecutionMutating).
	waiting, _ := json.Marshal(serverSetupExecution{ID: "exec-d", Status: "waiting", Steps: []serverSetupExecutionStep{busyRuleStep("dns", "primary_dns", "waiting")}})
	if _, err := database.Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES('exec-d','exec-d','plan-d','waiting',?,'now','now')`, string(waiting)); err != nil {
		t.Fatal(err)
	}
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(time.Hour)); reason != "" {
		t.Fatalf("a waiting setup run held discovery: %q", reason)
	}
	running, _ := json.Marshal(serverSetupExecution{ID: "exec-d", Status: "running", Steps: []serverSetupExecutionStep{busyRuleStep("web", "components", "running")}})
	if _, err := database.Exec(`UPDATE server_setup_executions SET status='running', execution_json=? WHERE id='exec-d'`, string(running)); err != nil {
		t.Fatal(err)
	}
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(time.Hour)); reason != panelUpdateReasonSetupRunning {
		t.Fatalf("setup run: %q", reason)
	}
	fixture.panel.agentClient = nil
	fixture.panel.updateAdmittedAt.Store(0)
	if reason := fixture.panel.panelUpdateDiscoverySkipReason(ctx, discoveryTestNow.Add(time.Hour)); reason != panelUpdateReasonAgentUnavailable {
		t.Fatalf("no Agent: %q", reason)
	}
}

func TestPanelUpdateDiscoveryAutomaticCheckIsRecordedAndKeepsTheFoundTarget(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	scheduler := newPanelUpdateDiscoveryScheduler(fixture.panel)
	scheduler.now = func() time.Time { return discoveryTestNow }
	scheduler.logf = func(string, ...any) {}
	if delay := scheduler.runOnce(context.Background()); delay < 11*time.Hour {
		t.Fatalf("delay %s", delay)
	}
	record, ok := discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeAvailable || !record.Automatic || record.Target == nil ||
		record.Target.Version != updateTestTargetVersion || record.FoundAt != "2026-10-17T12:00:00Z" {
		t.Fatalf("record=%+v ok=%v", record, ok)
	}
	// A later failed check keeps the found release and says it failed.
	fixture.agent.mu.Lock()
	fixture.agent.check.Error = "release server unreachable"
	fixture.agent.mu.Unlock()
	scheduler.now = func() time.Time { return discoveryTestNow.Add(12 * time.Hour) }
	if delay := scheduler.runOnce(context.Background()); delay != time.Hour {
		t.Fatalf("delay after a failure %s", delay)
	}
	record, ok = discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeFailed || record.ReasonCode != panelUpdateReasonReleaseUnverified ||
		record.Target == nil || record.FoundAt != "2026-10-17T12:00:00Z" || record.CheckedAt != "2026-10-18T00:00:00Z" {
		t.Fatalf("record after failure=%+v", record)
	}
	// Nothing is started: the Agent saw only reads.
	fixture.agent.mu.Lock()
	defer fixture.agent.mu.Unlock()
	if fixture.agent.startCalls != 0 || fixture.agent.abandonCalls != 0 || fixture.agent.checkCalls != 2 {
		t.Fatalf("checks=%d starts=%d abandons=%d", fixture.agent.checkCalls, fixture.agent.startCalls, fixture.agent.abandonCalls)
	}
}

func TestPanelUpdateManualCheckWritesTheSameRecord(t *testing.T) {
	withSystemUpdateBuild(t)
	withDiscoveryClock(t, discoveryTestNow)
	fixture := newSystemUpdateTestFixture(t)
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	record, ok := discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeAvailable || record.Automatic {
		t.Fatalf("record=%+v ok=%v", record, ok)
	}
	fixture.agent.mu.Lock()
	fixture.agent.check = transport.SystemUpdateCheckResponse{
		Supported: true, CurrentVersion: updateTestCurrentVersion, CurrentCommit: updateTestCurrentCommit,
	}
	fixture.agent.mu.Unlock()
	recorder = httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
	record, ok = discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeCurrent || record.Automatic || record.Target != nil {
		t.Fatalf("record=%+v", record)
	}
	// A non-administrator's request records nothing.
	if _, err := fixture.database.GetDB().Exec(`DELETE FROM panel_settings WHERE key = ?`, panelUpdateDiscoveryKey); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleCustomer))
	if _, ok := discoveryRecordOf(t, fixture); ok || recorder.Code != http.StatusForbidden {
		t.Fatalf("customer request recorded a check: status=%d", recorder.Code)
	}
	// A failed manual check keeps the button's own answer and is recorded.
	fixture.agent.mu.Lock()
	fixture.agent.check.Error = "offline"
	fixture.agent.mu.Unlock()
	recorder = httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "PANEL_UPDATE_UNAVAILABLE") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	record, ok = discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeFailed || record.ReasonCode != panelUpdateReasonReleaseUnverified || record.Automatic {
		t.Fatalf("record=%+v", record)
	}
}

func TestPanelUpdateDiscoveryRecordIsClearedWhenTheBuildReachesTheTarget(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()
	fixture.panel.savePanelUpdateDiscovery(ctx, discoveryTestNow, true, availableCheck(), nil)
	fixture.panel.clearStalePanelUpdateDiscovery(ctx)
	if _, ok := discoveryRecordOf(t, fixture); !ok {
		t.Fatal("a record of this build was cleared")
	}
	// The update installed the target: the new build clears it at start.
	buildVersion, buildCommit = updateTestTargetVersion, updateTestTargetCommit
	fixture.panel.clearStalePanelUpdateDiscovery(ctx)
	if fixture.panel.setting(ctx, panelUpdateDiscoveryKey) != "" {
		t.Fatal("the record of the installed target was not cleared")
	}
	// A record naming a release older than the running build is never shown.
	buildVersion, buildCommit = "v0.1.0-alpha.15", strings.Repeat("e", 40)
	older := availableCheck()
	older.CurrentVersion, older.CurrentCommit = buildVersion, buildCommit
	fixture.panel.savePanelUpdateDiscovery(ctx, discoveryTestNow, true, older, nil)
	record, ok := discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeCurrent || record.Target != nil {
		t.Fatalf("an older release was kept as available: %+v", record)
	}
}

func TestPanelUpdateDiscoveryStatusIsAdminOnlyAndMakesNoAgentCall(t *testing.T) {
	withSystemUpdateBuild(t)
	withDiscoveryClock(t, discoveryTestNow.Add(2*time.Hour))
	fixture := newSystemUpdateTestFixture(t)
	fixture.panel.lastUpdateAttempt = func(commit string) (recoveryobs.Attempt, bool) { return recoveryobs.Attempt{}, false }
	fixture.panel.savePanelUpdateDiscovery(context.Background(), discoveryTestNow, true, availableCheck(), nil)
	fixture.panel.updateDiscovery = newPanelUpdateDiscoveryScheduler(fixture.panel)
	fixture.panel.updateDiscovery.setNext(discoveryTestNow.Add(2*time.Hour + 90*time.Minute))
	fixture.agent.mu.Lock()
	before := fixture.agent.versionCalls + fixture.agent.checkCalls + fixture.agent.statusCalls + fixture.agent.readinessCalls
	fixture.agent.mu.Unlock()
	for _, role := range []string{roleReseller, roleCustomer, "additional_user"} {
		recorder := httptest.NewRecorder()
		fixture.panel.handlePanelUpdateDiscovery(recorder, systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryPath, "", role))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("role %s status=%d", role, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscovery(recorder, systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryPath, "", roleAdmin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var view panelUpdateDiscoveryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.AutoCheck || !view.Available || view.Last == nil || view.Last.Target == nil ||
		view.CheckedAgeSeconds == nil || *view.CheckedAgeSeconds != 7200 ||
		view.FoundAgeSeconds == nil || *view.FoundAgeSeconds != 7200 ||
		view.NextCheckInSeconds == nil || *view.NextCheckInSeconds != 5400 ||
		view.IntervalHours != 12 || view.ExpectedRestartSeconds != panelUpdateRestartDefaultSeconds {
		t.Fatalf("view=%s", recorder.Body.String())
	}
	fixture.agent.mu.Lock()
	after := fixture.agent.versionCalls + fixture.agent.checkCalls + fixture.agent.statusCalls + fixture.agent.readinessCalls
	fixture.agent.mu.Unlock()
	if after != before {
		t.Fatalf("the status read called the Agent %d time(s)", after-before)
	}
	if !licenseRecoveryRequest(systemUpdateRequest(http.MethodGet, panelUpdateDiscoveryPath, "", roleAdmin)) {
		t.Fatal("the stored update check is not readable while the licence is locked")
	}
	if licenseRecoveryRequest(systemUpdateRequest(http.MethodPut, panelUpdateDiscoveryPath, `{"auto_check":false}`, roleAdmin)) {
		t.Fatal("the switch must not pass the licence lock")
	}
}

func TestPanelUpdateDiscoverySwitchIsSavedAndStopsTheSchedule(t *testing.T) {
	withSystemUpdateBuild(t)
	withDiscoveryClock(t, discoveryTestNow)
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()
	if !fixture.panel.panelUpdateAutoCheck(ctx) {
		t.Fatal("automatic checks are not on by default")
	}
	for _, body := range []string{`{}`, `{"auto_check":"no"}`, `{"auto_check":false,"x":1}`} {
		recorder := httptest.NewRecorder()
		fixture.panel.handlePanelUpdateDiscovery(recorder, systemUpdateRequest(http.MethodPut, panelUpdateDiscoveryPath, body, roleAdmin))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", body, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscovery(recorder, systemUpdateRequest(http.MethodPut, panelUpdateDiscoveryPath, `{"auto_check":false}`, roleCustomer))
	if recorder.Code != http.StatusForbidden || !fixture.panel.panelUpdateAutoCheck(ctx) {
		t.Fatalf("a customer changed the switch: %d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	fixture.panel.handlePanelUpdateDiscovery(recorder, systemUpdateRequest(http.MethodPut, panelUpdateDiscoveryPath, `{"auto_check":false}`, roleAdmin))
	if recorder.Code != http.StatusOK || fixture.panel.panelUpdateAutoCheck(ctx) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var view panelUpdateDiscoveryResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || view.AutoCheck || view.NextCheckInSeconds != nil {
		t.Fatalf("view=%s", recorder.Body.String())
	}
	// Switched off, a scheduled check reads nothing.
	scheduler := newPanelUpdateDiscoveryScheduler(fixture.panel)
	scheduler.runOnce(ctx)
	fixture.agent.mu.Lock()
	checks := fixture.agent.checkCalls
	fixture.agent.mu.Unlock()
	if checks != 0 {
		t.Fatalf("a switched-off schedule read the release server %d time(s)", checks)
	}
	// The button still works.
	recorder = httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
	if recorder.Code != http.StatusOK {
		t.Fatalf("manual check while off: %d", recorder.Code)
	}
}

func TestPanelUpdateDiscoveryRouteIsRegistered(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"http.HandleFunc(panelUpdateDiscoveryPath, panel.handlePanelUpdateDiscovery)",
		"panel.startPanelUpdateDiscovery(discoveryCtx)",
		"signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)",
	} {
		if !strings.Contains(string(source), line) {
			t.Fatalf("main.go lacks %q", line)
		}
	}
	// The scheduler is assigned and the stale record cleared before
	// management opens (no request reads them while they are set).
	text := string(source)
	prepare := strings.Index(text, "panel.preparePanelUpdateDiscovery(discoveryCtx)")
	open := strings.Index(text, "startupGate.Open()")
	if prepare < 0 || open < 0 || prepare > open {
		t.Fatalf("the scheduler is prepared after management opens: prepare=%d open=%d", prepare, open)
	}
}

func TestComparePanelUpdateVersions(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"v0.1.0-alpha.96", "v0.1.0-alpha.95", 1},
		{"v0.1.0-alpha.95", "v0.1.0-alpha.96", -1},
		{"v0.1.0-alpha.100", "v0.1.0-alpha.99", 1},
		{"v0.1.0-alpha.9", "v0.1.0-alpha.9", 0},
		{"v0.1.0", "v0.1.0-alpha.99", 1},
		{"v0.2.0-alpha.1", "v0.1.9", 1},
		{"v0.1.0-alpha", "v0.1.0-alpha.1", -1},
		{"v0.1.0-beta.1", "v0.1.0-alpha.9", 1},
	} {
		got, ok := comparePanelUpdateVersions(test.left, test.right)
		if !ok || got != test.want {
			t.Fatalf("%s vs %s = %d,%v want %d", test.left, test.right, got, ok, test.want)
		}
	}
	if _, ok := comparePanelUpdateVersions("dev", "v0.1.0"); ok {
		t.Fatal("dev compared as a release")
	}
}

// The updater's read-only preflight stops if the Panel's database is written
// while it holds its pinned copy (independent review of the alpha.96
// candidate). Discovery never writes while an update is admitted or the
// server reports a change in progress: the result is kept in memory and
// written on the next idle pass.
func TestPanelUpdateDiscoveryNeverWritesWhileAnUpdateIsAdmittedOrRunning(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()
	fixture.panel.updateDiscovery = newPanelUpdateDiscoveryScheduler(fixture.panel)
	stored := func() string {
		var raw string
		_ = fixture.database.GetDB().QueryRow(`SELECT COALESCE(MAX(value), '') FROM panel_settings WHERE key = ?`, panelUpdateDiscoveryKey).Scan(&raw)
		return raw
	}
	// Admitted: kept in memory, shown, not written.
	fixture.panel.markPanelUpdateAdmitted(discoveryTestNow)
	if _, written := fixture.panel.savePanelUpdateDiscovery(ctx, discoveryTestNow.Add(time.Minute), false, availableCheck(), nil); written || stored() != "" {
		t.Fatal("a result was written while an update was admitted")
	}
	if record, ok := fixture.panel.currentPanelUpdateDiscovery(ctx); !ok || record.Outcome != panelUpdateOutcomeAvailable {
		t.Fatalf("the result kept in memory is not shown: %+v", record)
	}
	// The server reports the host lock (the updater holds it): still not written.
	fixture.panel.updateAdmittedAt.Store(0)
	fixture.agent.mu.Lock()
	fixture.agent.readiness = transport.HostMutationReadinessResponse{Code: transport.HostMutationBusy, Reason: transport.HostMutationReasonHostLock}
	fixture.agent.mu.Unlock()
	if fixture.panel.flushPanelUpdateDiscovery(ctx, discoveryTestNow.Add(2*time.Hour)) || stored() != "" {
		t.Fatal("a result was written while the server held its lock")
	}
	// A Panel operation holds the mutation lease: not written either.
	fixture.agent.mu.Lock()
	fixture.agent.readiness = transport.HostMutationReadinessResponse{Ready: true}
	fixture.agent.mu.Unlock()
	fixture.panel.serviceMutationMu.Lock()
	if fixture.panel.flushPanelUpdateDiscovery(ctx, discoveryTestNow.Add(2*time.Hour)) || stored() != "" {
		fixture.panel.serviceMutationMu.Unlock()
		t.Fatal("a result was written while the mutation lease was held")
	}
	fixture.panel.serviceMutationMu.Unlock()
	// Idle: the next pass writes it.
	if !fixture.panel.flushPanelUpdateDiscovery(ctx, discoveryTestNow.Add(2*time.Hour)) {
		t.Fatal("the kept result was not written on an idle pass")
	}
	if record, ok := discoveryRecordOf(t, fixture); !ok || record.Outcome != panelUpdateOutcomeAvailable || record.Automatic {
		t.Fatalf("written=%+v", record)
	}
	if fixture.panel.updateDiscoveryPending != nil {
		t.Fatal("the written result is still kept in memory")
	}
}

// A check that completes after an update was admitted while it ran keeps its
// result in memory: admission is read again right before the write.
func TestPanelUpdateDiscoveryReadsAdmissionAgainBeforeWriting(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	scheduler := newPanelUpdateDiscoveryScheduler(fixture.panel)
	fixture.panel.updateDiscovery = scheduler
	scheduler.now = func() time.Time { return discoveryTestNow }
	scheduler.logf = func(string, ...any) {}
	inner := scheduler.discover
	scheduler.discover = func(ctx context.Context) (panelUpdateCheckResponse, *panelUpdateDiscoveryFailure) {
		response, failure := inner(ctx)
		fixture.panel.markPanelUpdateAdmitted(discoveryTestNow) // a start admitted meanwhile
		return response, failure
	}
	scheduler.runOnce(context.Background())
	if _, ok := discoveryRecordOf(t, fixture); ok {
		t.Fatal("the check wrote its result after an update was admitted")
	}
	if record, ok := fixture.panel.currentPanelUpdateDiscovery(context.Background()); !ok || record.Outcome != panelUpdateOutcomeAvailable {
		t.Fatalf("kept=%+v ok=%v", record, ok)
	}
}

// A skip is never written and never replaces a known result; the view says
// it separately.
func TestPanelUpdateDiscoverySkipKeepsTheKnownResult(t *testing.T) {
	withSystemUpdateBuild(t)
	withDiscoveryClock(t, discoveryTestNow.Add(3*time.Hour))
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()
	scheduler := newPanelUpdateDiscoveryScheduler(fixture.panel)
	fixture.panel.updateDiscovery = scheduler
	current := availableCheck()
	current.Available, current.Target = false, nil
	if _, written := fixture.panel.savePanelUpdateDiscovery(ctx, discoveryTestNow, true, current, nil); !written {
		t.Fatal("an idle result was not written")
	}
	before, _ := discoveryRecordOf(t, fixture)
	scheduler.now = func() time.Time { return discoveryTestNow.Add(2 * time.Hour) }
	scheduler.logf = func(string, ...any) {}
	fixture.panel.markPanelUpdateAdmitted(discoveryTestNow.Add(2 * time.Hour))
	if delay := scheduler.runOnce(ctx); delay != panelUpdateDiscoverySkipDelay {
		t.Fatalf("delay %s", delay)
	}
	after, ok := discoveryRecordOf(t, fixture)
	if !ok || after != before {
		t.Fatalf("a skip changed the stored result: %+v -> %+v", before, after)
	}
	view := fixture.panel.panelUpdateDiscoveryView(ctx, discoveryTestNow.Add(3*time.Hour))
	if view.Last == nil || view.Last.Outcome != panelUpdateOutcomeCurrent || view.LastSkip == nil ||
		view.LastSkip.ReasonCode != panelUpdateReasonUpdateActive || view.LastSkip.AgeSeconds != 3600 {
		t.Fatalf("view=%+v skip=%+v", view, view.LastSkip)
	}
	// A completed check later ends the skip.
	scheduler.NoteChecked(discoveryTestNow.Add(150 * time.Minute))
	if view := fixture.panel.panelUpdateDiscoveryView(ctx, discoveryTestNow.Add(3*time.Hour)); view.LastSkip != nil {
		t.Fatalf("an old skip is still said: %+v", view.LastSkip)
	}
}

// Switching on resumes from the last completed check, not two minutes later.
func TestPanelUpdateDiscoveryResumeFollowsTheLastCheck(t *testing.T) {
	fake := newFakeDiscoveryScheduler()
	if got := fake.resumeDelay(); got != panelUpdateDiscoveryFirstDelay {
		t.Fatalf("no check yet: %s", got)
	}
	fake.NoteChecked(discoveryTestNow.Add(-time.Hour))
	if got := fake.resumeDelay(); got != 11*time.Hour {
		t.Fatalf("an hour after a check: %s", got)
	}
	fake.NoteChecked(discoveryTestNow.Add(-13 * time.Hour))
	if got := fake.resumeDelay(); got != panelUpdateDiscoveryFirstDelay {
		t.Fatalf("a check older than the period: %s", got)
	}
}

// A rebuild with the same version and another commit is what the Agent says
// it is: the stored outcome matches the manual answer.
func TestPanelUpdateDiscoverySameVersionRebuildFollowsTheAgent(t *testing.T) {
	withSystemUpdateBuild(t)
	withDiscoveryClock(t, discoveryTestNow)
	fixture := newSystemUpdateTestFixture(t)
	fixture.agent.mu.Lock()
	fixture.agent.check.TargetVersion = updateTestCurrentVersion
	fixture.agent.mu.Unlock()
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateCheck(recorder, systemUpdateRequest(http.MethodGet, panelUpdateCheckPath, "", roleAdmin))
	var answer panelUpdateCheckResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || !answer.Available {
		t.Fatalf("manual answer=%s", recorder.Body.String())
	}
	record, ok := discoveryRecordOf(t, fixture)
	if !ok || record.Outcome != panelUpdateOutcomeAvailable || record.Target == nil || record.Target.Commit != updateTestTargetCommit {
		t.Fatalf("stored=%+v ok=%v", record, ok)
	}
}
