package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The owner's explicit "Check requirements again" that sees every check pass
// wakes the run waiting at its final verification: setup completes on the
// runner's next pass instead of after the rest of its 20 s sleep. A page poll
// (plain check read or operation read) wakes nothing and starts no mutation;
// the handler never completes setup itself (owner report 2026-10-14).
// Sahibin acik kontrolu bekleyen son dogrulamayi hemen tamamlatir; sayfa
// sorgusu hicbir sey baslatmaz.
func TestServerSetupManualCheckWakesWaitingVerification(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := serverSetupPlan{Version: serverSetupPlanVersion, Revision: state.Revision, Draft: state.Draft, Purpose: "web", Actor: serviceOperationActor{UserID: f.userID}, Steps: []serverSetupPlanStep{}}
	plan.ID = serverSetupPlanIdentity(plan)
	raw, _ := json.Marshal(plan)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_plans(id,revision,plan_json,requested_by,created_at) VALUES(?,?,?,?,?)`, plan.ID, plan.Revision, string(raw), f.userID, "now"); err != nil {
		t.Fatal(err)
	}
	execution := serverSetupExecution{ID: strings.Repeat("7", 32), RequestID: strings.Repeat("7", 32), PlanID: plan.ID, Status: "waiting", Phase: "verification", Steps: []serverSetupExecutionStep{}}
	raw, _ = json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`INSERT INTO server_setup_executions(id,request_id,plan_id,status,execution_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, execution.ID, execution.RequestID, plan.ID, execution.Status, string(raw), "now", "now"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='running' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	var ready, probes atomic.Int32
	f.panel.serverSetupProbe = func(context.Context, serverSetupDraft) ([]serverSetupCheck, error) {
		probes.Add(1)
		if ready.Load() == 1 {
			return []serverSetupCheck{{ID: "mail_identity", State: "ready", Code: "ready"}}, nil
		}
		return []serverSetupCheck{{ID: "mail_identity", State: "action_required", Code: "mail_identity_required"}}, nil
	}
	latest := func() *serverSetupExecution {
		t.Helper()
		value, err := f.panel.latestServerSetupExecution(context.Background())
		if err != nil || value == nil {
			t.Fatalf("latest execution: %+v %v", value, err)
		}
		return value
	}
	operationRows := func() int {
		t.Helper()
		var rows int
		if err := f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM service_operations`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	rowsBefore := operationRows()

	t.Cleanup(func() {
		// Let a still-sleeping runner end before the fixture's database closes.
		ready.Store(1)
		f.panel.wakeServerSetupRunner()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, running := serverSetupRunners.Load(f.panel); !running {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	f.panel.launchServerSetupExecution()
	// The runner's first pass finds an unmet check and goes to sleep.
	deadline := time.Now().Add(5 * time.Second)
	for len(latest().Checks) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("runner did not record its first verification pass")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := latest(); got.Status != "waiting" || got.Phase != "verification" {
		t.Fatalf("first pass did not wait at verification: %+v", got)
	}
	ready.Store(1)

	// Page polls: a plain check read and an operation read. Neither wakes the
	// runner, so the run is still waiting after them.
	w := httptest.NewRecorder()
	f.panel.handleServerSetup(w, serviceOperationAdminRequest(t, http.MethodGet, "/api/v1/setup?check=1", "", f.userID))
	if w.Code != http.StatusOK {
		t.Fatalf("plain check read: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	f.panel.handleServerSetupOperation(w, serviceOperationAdminRequest(t, http.MethodGet, "/api/v1/setup/operation", "", f.userID))
	if w.Code != http.StatusOK {
		t.Fatalf("operation read: %d %s", w.Code, w.Body.String())
	}
	probesAfterPoll := probes.Load()
	time.Sleep(400 * time.Millisecond)
	if got := latest(); got.Status != "waiting" {
		t.Fatalf("a page poll completed or advanced the run: %+v", got)
	}
	if probes.Load() != probesAfterPoll {
		t.Fatal("a page poll woke the runner")
	}
	if loaded, err := f.panel.loadServerSetup(context.Background()); err != nil || loaded.Status == "ready" {
		t.Fatalf("a page poll completed setup: %+v %v", loaded, err)
	}

	// The owner's explicit check passes every requirement: the runner's next
	// pass completes setup well before the 20 s unattended interval.
	started := time.Now()
	w = httptest.NewRecorder()
	f.panel.handleServerSetup(w, serviceOperationAdminRequest(t, http.MethodGet, "/api/v1/setup?check=1&manual=1", "", f.userID))
	if w.Code != http.StatusOK {
		t.Fatalf("manual check: %d %s", w.Code, w.Body.String())
	}
	deadline = time.Now().Add(5 * time.Second)
	for latest().Status != "succeeded" {
		if time.Now().After(deadline) {
			t.Fatalf("manual check did not wake the waiting run: %+v", latest())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if elapsed := time.Since(started); elapsed >= serverSetupWaitingInterval/2 {
		t.Fatalf("completion took %v; the wake did not shorten the wait", elapsed)
	}
	if loaded, err := f.panel.loadServerSetup(context.Background()); err != nil || loaded.Status != "ready" {
		t.Fatalf("setup not marked ready by the runner: %+v %v", loaded, err)
	}
	if f.agent.installCalls.Load() != 0 || operationRows() != rowsBefore {
		t.Fatal("a check read started a mutation")
	}
}

// A manual check that still sees an unmet requirement, or that arrives while
// no run waits at verification, leaves no pending wake behind.
func TestServerSetupManualCheckWakesOnlyPassingVerification(t *testing.T) {
	f, _ := setupOperationFixture(t)
	drain := func() bool {
		select {
		case <-f.panel.serverSetupWakeChannel():
			return true
		default:
			return false
		}
	}
	drain()
	f.panel.wakeServerSetupVerification(context.Background(), []serverSetupCheck{{ID: "mail_identity", State: "action_required"}})
	if drain() {
		t.Fatal("an unmet check woke the runner")
	}
	f.panel.wakeServerSetupVerification(context.Background(), []serverSetupCheck{{ID: "mail_identity", State: "ready"}})
	if drain() {
		t.Fatal("a passing check woke the runner with no run waiting at verification")
	}
	f.panel.wakeServerSetupRunner()
	f.panel.wakeServerSetupRunner()
	if !drain() || drain() {
		t.Fatal("wake is not a single non-blocking poke")
	}
}
