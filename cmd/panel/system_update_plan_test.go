package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The update plan (owner design 2026-10-16). Each test fails if a step is
// named finished, running or failed beyond the evidence, or if the restart
// measurement is not kept exactly once per update.

var planTestCreated = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

func planStates(plan panelUpdatePlan) []string {
	states := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		states = append(states, step.ID+"="+step.State)
	}
	return states
}

func assertPlanStates(t *testing.T, plan panelUpdatePlan, want ...string) {
	t.Helper()
	got := planStates(plan)
	if len(got) != len(want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("steps = %v, want %v", got, want)
		}
	}
}

func TestPanelUpdatePlanNamesOnlyTheEvidencedSteps(t *testing.T) {
	now := planTestCreated.Add(2 * time.Minute)
	base := panelUpdatePlanInput{Status: "running", CreatedAt: planTestCreated, ArchiveSize: 200, Now: now}

	queued := base
	queued.Status = "queued"
	assertPlanStates(t, buildPanelUpdatePlan(queued),
		"download=running", "verify=pending", "prepare=pending", "restart=pending", "confirm=pending")

	downloading := base
	downloading.Downloaded, downloading.HaveBytes = 50, true
	plan := buildPanelUpdatePlan(downloading)
	assertPlanStates(t, plan, "download=running", "verify=pending", "prepare=pending", "restart=pending", "confirm=pending")
	if plan.Steps[0].Progress == nil || *plan.Steps[0].Progress != 0.25 {
		t.Fatalf("download progress = %v", plan.Steps[0].Progress)
	}
	if plan.Steps[0].ElapsedSeconds == nil || *plan.Steps[0].ElapsedSeconds != 120 || plan.Steps[0].StartedAt != "2026-10-16T09:00:00Z" {
		t.Fatalf("download timing = %#v", plan.Steps[0])
	}
	// Without bytes the download is indeterminate: no progress at all.
	if plan := buildPanelUpdatePlan(base); plan.Steps[0].Progress != nil {
		t.Fatalf("indeterminate download has progress %v", *plan.Steps[0].Progress)
	}

	downloaded := base
	downloaded.DownloadedAt = planTestCreated.Add(20 * time.Second)
	plan = buildPanelUpdatePlan(downloaded)
	assertPlanStates(t, plan, "download=done", "verify=running", "prepare=pending", "restart=pending", "confirm=pending")
	if plan.Steps[0].FinishedAt != "2026-10-16T09:00:20Z" || *plan.Steps[0].ElapsedSeconds != 20 || *plan.Steps[1].ElapsedSeconds != 100 {
		t.Fatalf("download/verify timing = %#v %#v", plan.Steps[0], plan.Steps[1])
	}

	verified := downloaded
	verified.VerifiedAt = planTestCreated.Add(25 * time.Second)
	assertPlanStates(t, buildPanelUpdatePlan(verified),
		"download=done", "verify=done", "prepare=running", "restart=pending", "confirm=pending")

	// A retry of the same release has no verified_at; the target build
	// answering is still proof that everything before it finished.
	restarted := base
	restarted.RunningTarget = true
	restarted.PanelOpenedAt = planTestCreated.Add(90 * time.Second)
	plan = buildPanelUpdatePlan(restarted)
	assertPlanStates(t, plan, "download=done", "verify=done", "prepare=done", "restart=done", "confirm=running")
	if plan.Steps[3].FinishedAt != "2026-10-16T09:01:30Z" || plan.Steps[3].ElapsedSeconds != nil || plan.Steps[4].StartedAt != "2026-10-16T09:01:30Z" {
		t.Fatalf("restart/confirm timing = %#v %#v", plan.Steps[3], plan.Steps[4])
	}
	if plan.Steps[1].FinishedAt != "" || plan.Steps[2].StartedAt != "" {
		t.Fatalf("unknown moments were invented: %#v %#v", plan.Steps[1], plan.Steps[2])
	}

	succeeded := restarted
	succeeded.Status = "succeeded"
	succeeded.UpdatedAt = planTestCreated.Add(100 * time.Second)
	plan = buildPanelUpdatePlan(succeeded)
	assertPlanStates(t, plan, "download=done", "verify=done", "prepare=done", "restart=done", "confirm=done")
	if plan.Steps[4].FinishedAt != "2026-10-16T09:01:40Z" || *plan.Steps[4].ElapsedSeconds != 10 {
		t.Fatalf("confirm timing = %#v", plan.Steps[4])
	}

	failedEarly := verified
	failedEarly.Status = "failed"
	assertPlanStates(t, buildPanelUpdatePlan(failedEarly),
		"download=done", "verify=done", "prepare=pending", "restart=pending", "confirm=pending")
	failedLate := restarted
	failedLate.Status = "failed"
	assertPlanStates(t, buildPanelUpdatePlan(failedLate),
		"download=done", "verify=done", "prepare=done", "restart=done", "confirm=failed")
}

func TestPanelUpdateExpectedRestartFollowsTheLastMeasurement(t *testing.T) {
	for measured, want := range map[int]int{0: 90, 40: 90, 60: 90, 61: 92, 100: 150, 240: 360, 900: 360} {
		if got := panelUpdateExpectedRestartSeconds(measured); got != want {
			t.Fatalf("expected(%d) = %d, want %d", measured, got, want)
		}
	}
	plan := buildPanelUpdatePlan(panelUpdatePlanInput{Status: "queued", CreatedAt: planTestCreated, Now: planTestCreated})
	if plan.ExpectedRestartSeconds != 90 || plan.RestartBoundSeconds != 360 {
		t.Fatalf("defaults = %d/%d", plan.ExpectedRestartSeconds, plan.RestartBoundSeconds)
	}
}

func planTestStatus(status string) transport.SystemUpdateStatusResponse {
	return transport.SystemUpdateStatusResponse{
		Found: true, RequestID: updateTestRequestID, Status: status,
		TargetVersion: updateTestTargetVersion, TargetCommit: updateTestTargetCommit,
		TargetSequence: "14", TargetOS: "linux", TargetArch: "amd64",
		TargetArchiveSHA256: updateTestTargetSHA, TargetArchiveSize: "1048576",
		CreatedAt: "2026-10-16T09:00:00Z", UpdatedAt: "2026-10-16T09:00:01Z",
	}
}

func readPlanStatus(t *testing.T, fixture systemUpdateTestFixture) panelUpdateStatusResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateStatus(recorder, systemUpdateRequest(
		http.MethodGet, panelUpdateStatusPath+"?request_id="+updateTestRequestID, "", roleAdmin,
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body panelUpdateStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func withPlanClock(t *testing.T, now time.Time) {
	t.Helper()
	previous := panelUpdateNow
	panelUpdateNow = func() time.Time { return now }
	t.Cleanup(func() { panelUpdateNow = previous })
}

func TestPanelUpdateStatusCarriesThePlanAndKeepsTheRestartOnce(t *testing.T) {
	withSystemUpdateBuild(t)
	withPlanClock(t, planTestCreated.Add(30*time.Second))
	fixture := newSystemUpdateTestFixture(t)

	// The old Panel, during the download: half the archive written.
	fixture.agent.status = planTestStatus("running")
	fixture.agent.status.DownloadedBytes = "524288"
	body := readPlanStatus(t, fixture)
	if body.Plan == nil {
		t.Fatal("status answer has no plan")
	}
	assertPlanStates(t, *body.Plan, "download=running", "verify=pending", "prepare=pending", "restart=pending", "confirm=pending")
	if body.Plan.Steps[0].Progress == nil || *body.Plan.Steps[0].Progress != 0.5 || *body.Plan.Steps[0].ElapsedSeconds != 30 {
		t.Fatalf("download step = %#v", body.Plan.Steps[0])
	}
	if body.Plan.ExpectedRestartSeconds != 90 {
		t.Fatalf("expected restart without a measurement = %d", body.Plan.ExpectedRestartSeconds)
	}

	// The new Panel: it is the target build and opened at 09:01:40.
	buildVersion, buildCommit = updateTestTargetVersion, updateTestTargetCommit
	fixture.agent.version.Version, fixture.agent.version.Commit = updateTestTargetVersion, updateTestTargetCommit
	gate := newPanelHTTPStartupGate(nil)
	gate.openedAt.Store(planTestCreated.Add(100 * time.Second).UnixNano())
	gate.ready.Store(true)
	fixture.panel.startupGate = gate
	fixture.agent.status = planTestStatus("running")
	fixture.agent.status.VerifiedAt = "2026-10-16T09:00:20Z"
	withPlanClock(t, planTestCreated.Add(105*time.Second))
	body = readPlanStatus(t, fixture)
	assertPlanStates(t, *body.Plan, "download=done", "verify=done", "prepare=done", "restart=done", "confirm=running")
	if body.Phase != "verifying" || body.Plan.Steps[3].FinishedAt != "2026-10-16T09:01:40Z" {
		t.Fatalf("restart end = %#v phase=%q", body.Plan.Steps[3], body.Phase)
	}
	// Measured 80 s (09:00:20 -> 09:01:40), kept with the request.
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 80 || got.RequestID != updateTestRequestID {
		t.Fatalf("recorded restart = %#v", got)
	}
	if body.Plan.ExpectedRestartSeconds != 120 {
		t.Fatalf("expected restart after the measurement = %d", body.Plan.ExpectedRestartSeconds)
	}

	// The same update read again later, with a later opening, is not measured twice.
	gate.openedAt.Store(planTestCreated.Add(300 * time.Second).UnixNano())
	fixture.agent.status = planTestStatus("succeeded")
	fixture.agent.status.VerifiedAt = "2026-10-16T09:00:20Z"
	body = readPlanStatus(t, fixture)
	assertPlanStates(t, *body.Plan, "download=done", "verify=done", "prepare=done", "restart=done", "confirm=done")
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 80 {
		t.Fatalf("restart measured twice: %#v", got)
	}
}

func TestPanelUpdateRestartIsNotMeasuredWithoutItsEvidence(t *testing.T) {
	withSystemUpdateBuild(t)
	withPlanClock(t, planTestCreated.Add(2*time.Minute))
	fixture := newSystemUpdateTestFixture(t)
	gate := newPanelHTTPStartupGate(nil)
	gate.openedAt.Store(planTestCreated.Add(100 * time.Second).UnixNano())
	fixture.panel.startupGate = gate

	// The old Panel (not the target build) never measures, even with verified_at.
	fixture.agent.status = planTestStatus("running")
	fixture.agent.status.VerifiedAt = "2026-10-16T09:00:20Z"
	readPlanStatus(t, fixture)
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 0 {
		t.Fatalf("old Panel measured a restart: %#v", got)
	}
	// The new Panel without verified_at (a retry of the same release) does not either.
	buildVersion, buildCommit = updateTestTargetVersion, updateTestTargetCommit
	fixture.agent.version.Version, fixture.agent.version.Commit = updateTestTargetVersion, updateTestTargetCommit
	fixture.agent.status = planTestStatus("running")
	readPlanStatus(t, fixture)
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 0 {
		t.Fatalf("restart measured without verified_at: %#v", got)
	}
	// Control: with verified_at it is measured.
	fixture.agent.status.VerifiedAt = "2026-10-16T09:00:20Z"
	readPlanStatus(t, fixture)
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 80 {
		t.Fatalf("control restart = %#v", got)
	}
	// A stored value that is not canonical is ignored.
	if err := fixture.panel.setSetting(context.Background(), panelUpdateLastRestartKey, `{"request_id":"x","seconds":80}`); err != nil {
		t.Fatal(err)
	}
	if got := fixture.panel.lastPanelUpdateRestart(context.Background()); got.Seconds != 0 {
		t.Fatalf("non-canonical record read: %#v", got)
	}
}

func TestPanelUpdateNotFoundAndStartAnswers(t *testing.T) {
	withSystemUpdateBuild(t)
	withPlanClock(t, planTestCreated)
	fixture := newSystemUpdateTestFixture(t)
	if body := readPlanStatus(t, fixture); body.Found || body.Plan != nil {
		t.Fatalf("not-found answer carries a plan: %#v", body)
	}
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateStart(recorder, systemUpdateRequest(
		http.MethodPost, panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion), roleAdmin,
	))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var started panelUpdateStartResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Plan == nil {
		t.Fatal("start answer declares no plan")
	}
	assertPlanStates(t, *started.Plan, "download=running", "verify=pending", "prepare=pending", "restart=pending", "confirm=pending")
}
