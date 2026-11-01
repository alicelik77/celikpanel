package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Lab run 4, I1 (2026-11-01): setup stopped at the webmail mail profile with
// firewall_sync_failed because the Agent refused the post-operation firewall
// child at admission while another change kept the host busy. A refusal
// proven to have started nothing is a wait: the sync is tried again.

func shortenFirewallSyncHostBusyWait(t *testing.T, bound time.Duration) {
	t.Helper()
	oldBound, oldInterval := firewallSyncHostBusyBound, firewallSyncHostBusyInterval
	firewallSyncHostBusyBound, firewallSyncHostBusyInterval = bound, time.Millisecond
	t.Cleanup(func() {
		firewallSyncHostBusyBound, firewallSyncHostBusyInterval = oldBound, oldInterval
	})
}

// refuseFirewallChildBusy refuses the first n firewall_sync begins as busy
// (package manager), recording each refused request identity.
func refuseFirewallChildBusy(agent *firewallSyncTestAgent, n int, refused *[]string) {
	agent.durableMutationRPCFixture.busyRefusal = func(req *ServiceOperationMutationBeginRequest) (string, *ServiceOperationMutationJob, bool) {
		if req.Kind != "firewall_sync" || len(*refused) >= n {
			return "", nil, false
		}
		*refused = append(*refused, req.RequestID)
		return transport.HostMutationReasonPackageManager, nil, true
	}
}

func TestFirewallSyncAfterSetupChildWaitsWhileTheHostIsBusy(t *testing.T) {
	shortenFirewallSyncHostBusyWait(t, time.Minute)
	agent := setupFirewallAgent()
	p, _ := newPanelCertificateSagaTestPanel(t, agent)
	op, _ := setupFirewallChildFixture(t, p, "service")
	var refused []string
	refuseFirewallChildBusy(agent, 2, &refused)

	if failure := p.syncFirewallAfterOperation(op); failure != nil {
		t.Fatalf("busy host failed the operation: %+v (%v)", failure, failure.Cause)
	}
	agent.mu.Lock()
	applies, begins := agent.applyCalls, agent.beginCalls
	var applied string
	if len(agent.applyRequests) == 1 {
		applied = agent.applyRequests[0].MutationRequestID
	}
	agent.mu.Unlock()
	if len(refused) != 2 || begins != 3 || applies != 1 {
		t.Fatalf("refused=%d begins=%d applies=%d, want 2/3/1", len(refused), begins, applies)
	}
	if applied == "" || applied == refused[0] || applied == refused[1] || refused[0] == refused[1] {
		t.Fatalf("each try must send a fresh child: refused=%v applied=%s", refused, applied)
	}
	row, err := p.serviceOperationByID(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if identity, ok := parseFirewallChildPhase(row.Phase); !ok || identity.RequestID != applied {
		t.Fatalf("row phase %q does not name the applied child %s", row.Phase, applied)
	}
}

func TestFirewallSyncAfterComponentsInstallFailsNamingTheBusyHostPastTheBound(t *testing.T) {
	shortenFirewallSyncHostBusyWait(t, 30*time.Millisecond)
	agent := setupFirewallAgent()
	p, _ := newPanelCertificateSagaTestPanel(t, agent)
	op, err := p.createServiceOperationRequest(context.Background(), serviceOperationKindInstall, "nginx", "", mustServiceOperationRequestID(t), serviceOperationActor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.markServiceOperationRunning(context.Background(), op.ID, "installed"); err != nil {
		t.Fatal(err)
	}
	op.Status = serviceOperationRunning
	var refused []string
	refuseFirewallChildBusy(agent, 1<<30, &refused)

	failure := p.syncFirewallAfterOperation(op)
	if failure == nil || failure.Code != firewallSyncHostBusyCode {
		t.Fatalf("failure=%+v, want %s", failure, firewallSyncHostBusyCode)
	}
	if !strings.Contains(failure.Message, "its package manager was busy") || !strings.Contains(failure.Message, "You, the owner") {
		t.Fatalf("message does not name the cause and next step: %s", failure.Message)
	}
	agent.mu.Lock()
	applies := agent.applyCalls
	agent.mu.Unlock()
	if applies != 0 || len(refused) < 2 {
		t.Fatalf("applies=%d refused=%d", applies, len(refused))
	}
	row, err := p.serviceOperationByID(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Phase != "firewall_waiting" && !strings.HasPrefix(row.Phase, firewallChildPhasePrefix) {
		t.Fatalf("row phase %q", row.Phase)
	}
}

func TestFirewallSyncBusyRefusalWithALedgerJobKeepsTheFailure(t *testing.T) {
	shortenFirewallSyncHostBusyWait(t, time.Minute)
	agent := setupFirewallAgent()
	p, _ := newPanelCertificateSagaTestPanel(t, agent)
	op, _ := setupFirewallChildFixture(t, p, "service")
	fixture := &agent.durableMutationRPCFixture
	refusals := 0
	fixture.busyRefusal = func(req *ServiceOperationMutationBeginRequest) (string, *ServiceOperationMutationJob, bool) {
		if req.Kind != "firewall_sync" {
			return "", nil, false
		}
		refusals++
		// The ledger holds a job for this very request: not proof that
		// nothing ran, so no retry.
		fixture.jobs[req.RequestID] = &ServiceOperationMutationJob{RequestID: req.RequestID, OwnerID: req.OwnerID,
			Kind: req.Kind, Target: req.Target, PackageName: req.PackageName, Status: agentMutationFailed}
		return transport.HostMutationReasonPackageManager, nil, true
	}
	failure := p.syncFirewallAfterOperation(op)
	if failure == nil || failure.Code != "firewall_sync_failed" || refusals != 1 {
		t.Fatalf("failure=%+v refusals=%d", failure, refusals)
	}
}

func TestFirewallSyncStartupRecoveryDoesNotWaitForABusyHost(t *testing.T) {
	shortenFirewallSyncHostBusyWait(t, time.Hour)
	agent := setupFirewallAgent()
	p, _ := newPanelCertificateSagaTestPanel(t, agent)
	op, _ := setupFirewallChildFixture(t, p, "service")
	var refused []string
	refuseFirewallChildBusy(agent, 1<<30, &refused)
	failure := p.syncFirewallAfterOperationWithin(op, 0)
	if failure == nil || failure.Code != firewallSyncHostBusyCode || len(refused) != 1 {
		t.Fatalf("failure=%+v refused=%d", failure, len(refused))
	}
}
