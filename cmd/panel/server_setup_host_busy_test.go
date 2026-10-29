package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Beta-1 third re-run, side effect: a start while a Components install ran
// created a run whose first component the Agent refused at admission
// (HOST_MUTATION_BUSY) and the run failed. The start is now refused before
// a run exists; a run that meets the busy state waits and asks again under a
// new child identity instead of failing.
func TestServerSetupHostBusyRefusesTheStartAndHoldsTheRun(t *testing.T) {
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	ctx := context.Background()
	other, err := f.panel.createServiceOperationRequest(ctx, serviceOperationKindInstall, "redis", "", strings.Repeat("1", 32), serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32)); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), serverSetupHostBusyCode) {
		t.Fatalf("a start while another change runs: %d %s", w.Code, w.Body.String())
	}
	if execution, err := f.panel.latestServerSetupExecution(ctx); err != nil || execution != nil {
		t.Fatalf("a run was created: %+v %v", execution, err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE service_operations SET status='succeeded' WHERE id=?`, other.ID); err != nil {
		t.Fatal(err)
	}
	w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	var execution serverSetupExecution
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &execution) != nil {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	// Up to its first component.
	for pass := 0; pass < 4 && serverSetupFirstRunningService(execution) < 0; pass++ {
		if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
			t.Fatal(err)
		}
	}
	index := serverSetupFirstRunningService(execution)
	if index < 0 {
		t.Fatalf("no component step started: %+v", execution)
	}
	first := execution.Steps[index]
	// The Agent refused that child at admission: nothing ran. (The fake
	// host ends the child first; its record then says what the Agent said.)
	waitForServiceOperation(t, f.panel, f.userID, first.OperationID, "failed")
	if _, err := f.database.GetDB().Exec(`UPDATE service_operations SET status='failed',error_code=?,error_message='package manager busy' WHERE id=?`, errCodeHostMutationBusy, first.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	step := execution.Steps[index]
	if execution.Status != "waiting" || execution.Error == nil || execution.Error.Code != serverSetupHostBusyCode || step.Status != "running" || step.ChildAttempt != 1 || step.OperationID != "" {
		t.Fatalf("a refused child failed the run: %s %+v %+v", execution.Status, execution.Error, step)
	}
	// While another change runs the step waits and creates nothing.
	another, err := f.panel.createServiceOperationRequest(ctx, serviceOperationKindInstall, "redis", "", strings.Repeat("2", 32), serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	if execution.Error == nil || execution.Error.Code != serverSetupHostBusyCode || execution.Steps[index].OperationID != "" {
		t.Fatalf("busy host: %+v %+v", execution.Error, execution.Steps[index])
	}
	if _, err := f.database.GetDB().Exec(`UPDATE service_operations SET status='succeeded' WHERE id=?`, another.ID); err != nil {
		t.Fatal(err)
	}
	// Free again: a new child under a new identity.
	if _, err := f.panel.advanceServerSetupExecution(plan, &execution); err != nil {
		t.Fatal(err)
	}
	retry := execution.Steps[index]
	if retry.OperationID == "" || retry.OperationID == first.OperationID {
		t.Fatalf("the step did not ask again: %+v %+v", retry, execution.Error)
	}
	op, err := f.panel.serviceOperationByRequestID(ctx, serverSetupChildRequestID(retry))
	if err != nil || op.ID != retry.OperationID || serverSetupChildRequestID(retry) == retry.RequestID {
		t.Fatalf("the new child's identity: %+v %v", op, err)
	}
}

func serverSetupFirstRunningService(execution serverSetupExecution) int {
	for index, step := range execution.Steps {
		if step.Kind == "service" && step.OperationID != "" {
			return index
		}
	}
	return -1
}
