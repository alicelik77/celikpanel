package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
)

// Frankfurt 2026-10-16: the update's last step (turning Certbot renewal back
// on) did not confirm and the server's automatic recovery finished it. While
// recovery owns the request the confirmation is retrying, never failed; a
// verified recovery makes every step done. A real recovery_required stays a
// failed confirmation, and without a readable record nothing changes.

func recoveryPlanStatus(phase, proof, previous, code, automatic string) recoveryobs.Status {
	return recoveryobs.Status{
		Schema: recoveryobs.StatusSchema, RequestID: updateTestRequestID, Observation: "known",
		Phase: phase, TerminalProof: proof, PreviousFailure: previous, FailureCode: code,
		AutomaticRecovery: automatic, ObservedAt: "2026-10-16T09:02:10Z",
	}
}

func TestPanelUpdatePlanRetriesWhileRecoveryOwnsTheRequest(t *testing.T) {
	now := planTestCreated.Add(3 * time.Minute)
	base := panelUpdatePlanInput{Status: "failed", CreatedAt: planTestCreated, Now: now, RunningTarget: true,
		DownloadedAt: planTestCreated.Add(10 * time.Second), VerifiedAt: planTestCreated.Add(20 * time.Second),
		PanelOpenedAt: planTestCreated.Add(100 * time.Second)}
	done := []string{"download=done", "verify=done", "prepare=done", "restart=done"}
	for _, tc := range []struct {
		name    string
		status  recoveryobs.Status
		confirm string
		attempt int
	}{
		{"first attempt", recoveryPlanStatus("recovering", "none", "update_failed", "update_finishing_scheduler", ""), "retrying", 1},
		{"failed before the runner", recoveryPlanStatus("failed", "none", "update_failed", "update_finishing_scheduler", ""), "retrying", 0},
		{"later attempt", recoveryPlanStatus("recovering", "none", "recovery_failed", "", ""), "retrying", 0},
		{"between attempts", recoveryPlanStatus("recovery_required", "none", "recovery_failed", "", "retry_scheduled"), "retrying", 0},
		{"pause pending", recoveryPlanStatus("recovery_required", "none", "recovery_failed", "", "pause_pending"), "retrying", 0},
		// Negative controls: the owner must act, the record is final, or unknown.
		{"paused", recoveryPlanStatus("recovery_required", "none", "recovery_failed", "", "paused_retry_limit"), "failed", 0},
		{"recovery required", recoveryPlanStatus("recovery_required", "none", "recovery_failed", "", ""), "failed", 0},
		{"preflight stop", recoveryPlanStatus("failed", "none", "update_failed", "update_preflight_refused", ""), "failed", 0},
		{"unavailable", recoveryobs.Status{Observation: "unavailable", TerminalProof: "none"}, "failed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			applyPanelUpdateRecovery(&in, tc.status)
			plan := buildPanelUpdatePlan(in)
			assertPlanStates(t, plan, append(append([]string{}, done...), "confirm="+tc.confirm)...)
			confirm := plan.Steps[4]
			if confirm.Attempt != tc.attempt || (tc.attempt > 0) != (confirm.MaxAttempts == 3) {
				t.Fatalf("attempt = %d of %d, want %d", confirm.Attempt, confirm.MaxAttempts, tc.attempt)
			}
			if tc.confirm == "retrying" && (confirm.ElapsedSeconds == nil || *confirm.ElapsedSeconds != 80) {
				t.Fatalf("retrying confirmation timing = %#v", confirm)
			}
		})
	}

	// A verified recovery is the update's success: every step done.
	in := base
	applyPanelUpdateRecovery(&in, recoveryPlanStatus("succeeded", "update_verified", "update_failed", "update_finishing_scheduler", ""))
	plan := buildPanelUpdatePlan(in)
	assertPlanStates(t, plan, append(append([]string{}, done...), "confirm=done")...)
	if plan.Steps[4].FinishedAt != "2026-10-16T09:02:10Z" {
		t.Fatalf("confirmation end = %#v", plan.Steps[4])
	}

	// Not the target build: the failure is not placed on a step (rollback side).
	in = base
	in.RunningTarget = false
	applyPanelUpdateRecovery(&in, recoveryPlanStatus("recovering", "none", "update_failed", "", ""))
	assertPlanStates(t, buildPanelUpdatePlan(in), "download=done", "verify=done", "prepare=pending", "restart=pending", "confirm=pending")
}

func TestPanelUpdateStatusReadsRecoveryForAFailedTargetRecord(t *testing.T) {
	withSystemUpdateBuild(t)
	withPlanClock(t, planTestCreated.Add(130*time.Second))
	fixture := newSystemUpdateTestFixture(t)
	buildVersion, buildCommit = updateTestTargetVersion, updateTestTargetCommit
	fixture.agent.version.Version, fixture.agent.version.Commit = updateTestTargetVersion, updateTestTargetCommit
	reads := 0
	fixture.panel.recoveryStatus = func(id string) recoveryobs.Status {
		reads++
		if id != updateTestRequestID {
			t.Fatalf("recovery read for %q", id)
		}
		return recoveryPlanStatus("recovering", "none", "update_failed", "update_finishing_scheduler", "")
	}
	fixture.agent.status = planTestStatus("failed")
	fixture.agent.status.VerifiedAt = "2026-10-16T09:00:20Z"
	fixture.agent.status.Error = "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_finishing_scheduler state=recovery_required reason=Certbot renewal scheduler state could not be restored detail="
	body := readPlanStatus(t, fixture)
	if body.Plan == nil || reads != 1 {
		t.Fatalf("plan = %#v, recovery reads = %d", body.Plan, reads)
	}
	assertPlanStates(t, *body.Plan, "download=done", "verify=done", "prepare=done", "restart=done", "confirm=retrying")
	raw, err := json.Marshal(body.Plan.Steps[4])
	if err != nil || !strings.Contains(string(raw), `"state":"retrying"`) || !strings.Contains(string(raw), `"attempt":1,"max_attempts":3`) {
		t.Fatalf("retrying step JSON = %s (%v)", raw, err)
	}
	raw, _ = json.Marshal(body.Plan.Steps[3])
	if strings.Contains(string(raw), "attempt") {
		t.Fatalf("a step outside the retry carries an attempt: %s", raw)
	}

	// A running record never reads the recovery observation.
	fixture.agent.status = planTestStatus("running")
	fixture.agent.status.Error = ""
	_ = readPlanStatus(t, fixture)
	if reads != 1 {
		t.Fatalf("recovery read for a running record: %d", reads)
	}
}
