package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/recoveryobs"
)

// alpha.114 lab E-new-5: the candidate's installer stopped, the server's
// recovery returned it to the previous release and verified that, yet the
// status still said state=recovery_required and named no cause. The answer of
// the previous release's Panel now says rolled_back and keeps the cause line;
// recovery_required stays only while it is true.
func TestPanelUpdateStatusSaysRolledBackOnlyAfterAVerifiedRollback(t *testing.T) {
	withSystemUpdateBuild(t) // this Panel is the previous release, not the target
	withPlanClock(t, planTestCreated.Add(130*time.Second))
	fixture := newSystemUpdateTestFixture(t)
	record := recoveryPlanStatus("recovered", "rollback_verified", "update_failed", "", "")
	fixture.panel.recoveryStatus = func(id string) recoveryobs.Status {
		if id != updateTestRequestID {
			t.Fatalf("recovery read for %q", id)
		}
		return record
	}
	const cause = "installer stopped: panel.env line 3 has an unsupported key: CELIKPANEL_LAB_ROLLBACK_PROBE (remove the line, then retry)"
	fixture.agent.status = planTestStatus("failed")
	fixture.agent.status.Error = "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=" + cause + " detail=ERROR / HATA: panel.env line 3 has an unsupported key"

	body := readPlanStatus(t, fixture)
	if body.Status != "failed" || body.Phase != panelUpdatePhaseRolledBack {
		t.Fatalf("status/phase = %q/%q", body.Status, body.Phase)
	}
	if strings.Contains(body.Summary, "recovery_required") || !strings.Contains(body.Summary, "state=rolled_back reason="+cause+" detail=") {
		t.Fatalf("summary = %q", body.Summary)
	}

	// Negative controls: recovery still running, or a recovery the owner must
	// finish, keeps the updater's own state; so does an unreadable record.
	for _, other := range []recoveryobs.Status{
		recoveryPlanStatus("recovering", "none", "update_failed", "", ""),
		recoveryPlanStatus("recovery_required", "none", "recovery_failed", "", ""),
		{Observation: "unavailable", TerminalProof: "none"},
	} {
		record = other
		body = readPlanStatus(t, fixture)
		if body.Phase != "" || !strings.Contains(body.Summary, "state=recovery_required reason="+cause) {
			t.Fatalf("%s/%s: phase %q summary %q", other.Phase, other.Observation, body.Phase, body.Summary)
		}
	}

	// A verified rollback with the historical generic line keeps it (the
	// page then says the server recorded no more specific cause).
	record = recoveryPlanStatus("recovered", "rollback_verified", "update_failed", "", "")
	fixture.agent.status.Error = "reviewed updater failed: exit status 1: !! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=updater command failed; inspect the update worker log detail="
	body = readPlanStatus(t, fixture)
	if body.Phase != panelUpdatePhaseRolledBack || !strings.HasSuffix(body.Summary, "state=rolled_back reason=updater command failed; inspect the update worker log detail=") {
		t.Fatalf("generic rollback: phase %q summary %q", body.Phase, body.Summary)
	}
}

func TestPanelUpdateRolledBackSummaryTouchesOnlyTheReviewedLine(t *testing.T) {
	for raw, want := range map[string]string{
		"!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=recovery_required reason=x detail=": "!! CELIKPANEL_UPDATE_FAILURE code=update_failed state=rolled_back reason=x detail=",
		"a plain line with state=recovery_required reason=x":                                       "a plain line with state=recovery_required reason=x",
		"": "",
	} {
		if got := panelUpdateRolledBackSummary(raw); got != want {
			t.Fatalf("%q -> %q, want %q", raw, got, want)
		}
	}
}
