package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/rpc"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 third re-run, side effect: a start while a Components install ran
// created a run whose first component the Agent refused at admission. The
// start is now refused before a run exists.
func TestServerSetupHostBusyRefusesTheStart(t *testing.T) {
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
	if w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32)); w.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
}

func busyRunToFirstComponentForTest(t *testing.T) (serviceOperationTestFixture, serverSetupPlan, *serverSetupExecution) {
	t.Helper()
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	serverSetupRunners.Store(f.panel, true)
	t.Cleanup(func() { serverSetupRunners.Delete(f.panel) })
	w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	var execution serverSetupExecution
	if w.Code != http.StatusAccepted || json.Unmarshal(w.Body.Bytes(), &execution) != nil {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	return f, plan, &execution
}

func firstServiceStepIndex(execution serverSetupExecution) int {
	for index, step := range execution.Steps {
		if step.Kind == "service" {
			return index
		}
	}
	return -1
}

// advanceUntilChildForTest advances until the step at index has a child
// operation, and waits for that child to end.
func advanceUntilChildForTest(t *testing.T, f serviceOperationTestFixture, plan serverSetupPlan, execution *serverSetupExecution, index int, want string) serviceOperation {
	t.Helper()
	for pass := 0; pass < 6 && execution.Steps[index].OperationID == ""; pass++ {
		if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
			t.Fatal(err)
		}
	}
	id := execution.Steps[index].OperationID
	if id == "" {
		t.Fatalf("no child for step %d: %+v", index, execution)
	}
	waitForServiceOperation(t, f.panel, f.userID, id, want)
	op, err := f.panel.serviceOperationByRequestID(context.Background(), execution.Steps[index].RequestID)
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// Review fix (items 1 and 9): a child is admitted again only with proof
// that nothing ran (refused at its first lease, no ledger job), under its
// own identity and operation row, attempts kept on the step and no audit
// row per attempt; the bound is time, and past it the step fails naming
// what kept the server busy.
func TestServerSetupChildRefusedAtAdmissionIsAdmittedAgainOnlyWithProof(t *testing.T) {
	f, plan, execution := busyRunToFirstComponentForTest(t)
	index := firstServiceStepIndex(*execution)
	f.agent.mu.Lock()
	f.agent.beginBusyReason = transport.HostMutationReasonPackageManager
	f.agent.mu.Unlock()
	first := advanceUntilChildForTest(t, f, plan, execution, index, "failed")
	if first.Phase != "lease_failed" || first.Error == nil || first.Error.Code != errCodeHostMutationBusy {
		t.Fatalf("the refused child: %+v", first)
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	step := execution.Steps[index]
	if execution.Status != "waiting" || execution.Error == nil || execution.Error.Code != serverSetupHostBusyCode || step.Status != "running" || step.BusyAttempts != 1 || !step.BusyRetry || step.BusySince == "" {
		t.Fatalf("after one refusal: %s %+v %+v", execution.Status, execution.Error, step)
	}
	// The next pass admits the same child again; still refused.
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	if execution.Steps[index].OperationID != first.ID {
		t.Fatalf("a new child was made: %s != %s", execution.Steps[index].OperationID, first.ID)
	}
	waitForServiceOperation(t, f.panel, f.userID, first.ID, "failed")
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	if execution.Steps[index].BusyAttempts != 2 {
		t.Fatalf("attempts: %+v", execution.Steps[index])
	}
	var rows, audits int
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM service_operations WHERE service_id=?`, execution.Steps[index].Target).Scan(&rows)
	f.database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'server.setup.child.failed:%'`).Scan(&audits)
	if rows != 1 || audits != 0 {
		t.Fatalf("one row and no audit per attempt: rows=%d audits=%d", rows, audits)
	}
	// Past the bound: a verified failure naming the package manager.
	execution.Steps[index].BusySince = time.Now().Add(-serverSetupHostBusyBound - time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	waitForServiceOperation(t, f.panel, f.userID, first.ID, "failed")
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.Error == nil || execution.Error.Code != serverSetupHostStayedBusyCode || !strings.Contains(execution.Error.Message, "package manager") {
		t.Fatalf("past the bound: %s %+v", execution.Status, execution.Error)
	}
}

// Without the proof nothing is admitted again: a child whose request the
// Agent's ledger knows (a resumed child), and a mail profile whose mail TLS
// sub-step was refused after the install (its own code), are verified
// failures.
func TestServerSetupChildWithoutTheProofIsNeverAdmittedAgain(t *testing.T) {
	f, plan, execution := busyRunToFirstComponentForTest(t)
	index := firstServiceStepIndex(*execution)
	f.agent.mu.Lock()
	f.agent.beginBusyReason = transport.HostMutationReasonPackageManager
	f.agent.mu.Unlock()
	child := advanceUntilChildForTest(t, f, plan, execution, index, "failed")
	// The Agent's ledger knows this request: it is not proof that nothing ran.
	f.agent.mu.Lock()
	f.agent.mutationJobs[child.RequestID] = &ServiceOperationMutationJob{RequestID: child.RequestID, OwnerID: strings.Repeat("0", 32), Kind: child.Kind, Target: child.ServiceID, Status: agentMutationFailed}
	f.agent.mu.Unlock()
	if f.panel.serverSetupChildRefusedAtAdmission(context.Background(), child) {
		t.Fatal("a child the ledger knows was taken for one that never ran")
	}
	if _, err := f.panel.advanceServerSetupExecution(plan, execution); err != nil {
		t.Fatal(err)
	}
	if execution.Status != "failed" || execution.Steps[index].BusyRetry {
		t.Fatalf("without proof: %s %+v", execution.Status, execution.Steps[index])
	}
	// The mail TLS sub-step's refusal after an install has its own code.
	failure := mailTLSSyncFailure(&hostMutationBusyError{reason: transport.HostMutationReasonPackageManager})
	if failure.Code != errCodeMailTLSSyncHostBusy {
		t.Fatalf("mail TLS sub-step: %+v", failure)
	}
	ran := serviceOperation{RequestID: strings.Repeat("6", 32), Status: serviceOperationFailed, Phase: "mail-tls", Error: &serviceOperationError{Code: failure.Code}}
	if f.panel.serverSetupChildRefusedAtAdmission(context.Background(), ran) {
		t.Fatal("a mail profile that ran was taken for a refused admission")
	}
}

// Review fix (item 4): the start's host check answers with a code, never a
// bare 500: a busy Agent, a starting host and an Agent that does not answer.
func TestServerSetupStartHostRefusalsAreCoded(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{&hostMutationBusyError{reason: transport.HostMutationReasonPackageManager}, serverSetupHostBusyCode},
		{bootingHostErrorForTest(), serverSetupHostStartingCode},
		{fmt.Errorf("%w: setup child: %w", errAgentRPCPlatformIdentityUnavailable, errAgentRPCClientUnavailable), serverSetupAgentUnreachableCode},
		{fmt.Errorf("%w: dial", errServerSetupAgentUnreachable), serverSetupAgentUnreachableCode},
	} {
		if code, message, ok := serverSetupStartHostRefusal(test.err); !ok || code != test.code || message == "" {
			t.Fatalf("%v: %q %v", test.err, code, ok)
		}
	}
	if _, _, ok := serverSetupStartHostRefusal(errors.New("database is locked")); ok {
		t.Fatal("a database error was coded as a host state")
	}
	// The handler: an Agent that cannot be reached refuses the start with
	// its code and the command to check.
	f, state := setupOperationFixture(t)
	plan := saveSetupPlanForTest(t, f, state)
	f.panel.agentClient = transport.NewReconnectingClientWithContextConnector(nil, func(context.Context) (*rpc.Client, error) { return nil, errors.New("dial unix /run/celikpanel/agent.sock: connect: no such file or directory") })
	f.panel.pkgFamilyMu.Lock()
	f.panel.hostPlatformKnown = false
	f.panel.pkgFamilyMu.Unlock()
	w := postSetupStartForTest(t, f, plan.ID, strings.Repeat("9", 32))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), serverSetupAgentUnreachableCode) || !strings.Contains(w.Body.String(), "systemctl status celikpanel-agent") {
		t.Fatalf("an unreachable Agent: %d %s", w.Code, w.Body.String())
	}
}
