package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 re-run P6, the P4-like sequence without a restart: BIND's install
// fails on the main server (no installation candidate), the Agent proves the
// install placed nothing, the same runner pass records the rollback as a
// verified failure with both next actions, the owner chooses PowerDNS in a
// revised plan and the new run installs it. No panel restart, no second BIND
// attempt, and polling the stopped run starts nothing.
//
// Before the fix the Agent's evidence for a never-placed BIND was "runtime
// unsealed" (no masks to prove), so the step stayed paused at
// server_setup_dns_agent_failed until startup recovery released it; the
// Agent's own test (TestDNSEngineRollbackEvidenceProvesAFirstInstallThatNeverPlacedAPackage)
// pins that proof. Here the fake Agent reports what the fixed one reports.
func TestServerSetupBINDFailureRollsBackAndPowerDNSContinuesWithoutRestart(t *testing.T) {
	openPDNSPairedPrimaryGateForTest(t)
	agent := newDNSEngineTestAgent()
	agent.switchError = "install BIND in no-start mode: the package manager could not show what installing bind9 would do, so nothing was installed"
	// The old Agent: the never-placed install cannot be proven.
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackRuntimeUnsealed
	x := newSetupDNSOutcomeFixture(t, setupDNSTestDraft(), agent)
	x.poll(t)
	if x.execution.Status != "waiting" || x.execution.Error == nil || x.execution.Error.Code != serverSetupDNSAgentFailedCode {
		t.Fatalf("an unproven rollback: %+v", x.execution)
	}
	// The fixed Agent proves it; the next pass records it, no restart.
	agent.mu.Lock()
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackSafe
	agent.mu.Unlock()
	x.poll(t)
	if x.execution.Status != "failed" || x.execution.Error == nil || x.execution.Error.Code != "server_setup_dns_rolled_back" ||
		!strings.Contains(x.execution.Error.Message, "no restart is needed") ||
		!strings.Contains(x.execution.Error.Message, "choose the other DNS software in the plan") {
		t.Fatalf("the rollback is not a verified failure with its next actions: %+v", x.execution)
	}
	state, err := readDNSEngineDBState(context.Background(), x.f.database.GetDB())
	if err != nil || state.CurrentSwitchID != "" || state.ActiveEngine != "" {
		t.Fatalf("the rolled-back switch was kept: %+v %v", state, err)
	}
	// Reading the stopped run again changes nothing and starts nothing.
	for poll := 0; poll < 2; poll++ {
		w := httptest.NewRecorder()
		x.f.panel.handleServerSetupOperation(w, serviceOperationAdminRequest(t, http.MethodGet, "/api/v1/setup/operation", "", x.f.userID))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "server_setup_dns_rolled_back") {
			t.Fatalf("poll %d: %d %s", poll, w.Code, w.Body.String())
		}
	}
	if calls := x.switchCalls(); calls != 1 {
		t.Fatalf("host switch calls after the failure: %d", calls)
	}
	// The owner revises the plan and chooses PowerDNS.
	setup, err := x.f.panel.loadServerSetup(context.Background())
	if err != nil || setup.Status != "draft" {
		t.Fatalf("the failed run did not release its draft: %+v %v", setup, err)
	}
	draft := setup.Draft
	draft.DNSEngine = "pdns"
	setup, err = x.f.panel.saveServerSetupDraft(context.Background(), setup.Revision, draft)
	if err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	agent.switchError = ""
	agent.mu.Unlock()
	plan := serverSetupPlan{Version: serverSetupPlanVersion, Revision: setup.Revision, Draft: setup.Draft, Purpose: setup.Draft.Purpose,
		Actor: serviceOperationActor{UserID: x.f.userID},
		Steps: []serverSetupPlanStep{{ID: "01-dns", Kind: "dns", Target: "local"}, {ID: "02-verify", Kind: "verify", Target: setup.Draft.Purpose}}}
	plan.ID = serverSetupPlanIdentity(plan)
	raw, _ := json.Marshal(plan)
	if _, err := x.f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, plan.ID, plan.Revision, string(raw), x.f.userID, "now"); err != nil {
		t.Fatal(err)
	}
	// A new run of the revised plan, as the start handler creates it (the
	// fixture's plan is hand-made, so the review identity is not rebuilt).
	execution := serverSetupExecution{ID: strings.Repeat("7", 32), RequestID: strings.Repeat("7", 32), PlanID: plan.ID, Status: "running"}
	for _, step := range plan.Steps {
		execution.Steps = append(execution.Steps, serverSetupExecutionStep{serverSetupPlanStep: step, Status: "pending",
			RequestID: serverSetupID(execution.ID, step.ID, "request"), OwnerID: serverSetupID(execution.ID, step.ID, "owner")})
	}
	raw, _ = json.Marshal(execution)
	if _, err := x.f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,'now2','now2')`,
		execution.ID, execution.RequestID, plan.ID, execution.Status, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.database.GetDB().Exec(`UPDATE server_setup_state SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Steps[0].Status != "succeeded" {
		t.Fatalf("PowerDNS did not install in the revised run: %+v", execution)
	}
	state, err = readDNSEngineDBState(context.Background(), x.f.database.GetDB())
	if err != nil || state.ActiveEngine != transport.DNSEnginePowerDNS {
		t.Fatalf("active engine after the revised run: %+v %v", state, err)
	}
	if calls := x.switchCalls(); calls != 2 {
		t.Fatalf("host switch calls: %d (one BIND attempt, one PowerDNS install)", calls)
	}
}
