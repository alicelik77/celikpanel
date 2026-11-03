package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 lab D1: setup accepted its DNS switch, the Agent refused the
// lease (nothing in its ledger), and the step showed
// server_setup_dns_result_unknown forever with the switch "activating".

func setDNSTestAgentBusy(agent *dnsEngineTestAgent, busy bool) {
	agent.durableMutationRPCFixture.mu.Lock()
	defer agent.durableMutationRPCFixture.mu.Unlock()
	if !busy {
		agent.busyRefusal = nil
		return
	}
	agent.busyRefusal = func(*ServiceOperationMutationBeginRequest) (string, *ServiceOperationMutationJob, bool) {
		return transport.HostMutationReasonPackageManager, nil, true
	}
}

func dnsTestAgentHasJob(agent *dnsEngineTestAgent, requestID string) bool {
	agent.durableMutationRPCFixture.mu.Lock()
	defer agent.durableMutationRPCFixture.mu.Unlock()
	return agent.jobs[requestID] != nil
}

func dnsTestAgentSwitchCalls(agent *dnsEngineTestAgent) int {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return agent.switchCalls
}

func requireAttachedActivatingSwitch(t *testing.T, p *Panel, requestID string) persistedDNSEngineSwitch {
	t.Helper()
	ctx := context.Background()
	persisted, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), requestID)
	if err != nil || persisted.Phase != "activating" {
		t.Fatalf("switch: %+v %v", persisted, err)
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil || state.CurrentSwitchID != persisted.SwitchID || state.ActiveEngine != "" {
		t.Fatalf("state: %+v %v", state, err)
	}
	return persisted
}

func TestSetupDNSLeaseRefusedAfterAcceptanceWaitsAndResumesSameRequest(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	// The real Agent proves no rollback without a failed job for the request.
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackIdentityMismatch
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	ctx := context.Background()
	id := strings.Repeat("c", 32)
	actor := serviceOperationActor{UserID: 1}

	// The lab interleaving: the switch is accepted, then the lease is refused.
	setDNSTestAgentBusy(agent, true)
	err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, actor)
	var notAdmitted *serverSetupDNSNotAdmittedError
	if !errors.As(err, &notAdmitted) || !notAdmitted.busy() || errors.Is(err, errServerSetupDNSReconciliationRequired) {
		t.Fatalf("refused lease after acceptance: %v", err)
	}
	persisted := requireAttachedActivatingSwitch(t, p, id)
	if dnsTestAgentHasJob(agent, id) || dnsTestAgentSwitchCalls(agent) != 0 {
		t.Fatal("a refused lease recorded a job or reached the switch")
	}
	var audits int
	if err := p.db.GetDB().QueryRow(`SELECT count(*) FROM audit_logs WHERE action LIKE 'dns.engine.switch.setup.reconcile_required %'`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("a proven refusal was audited as an unknown result: %d %v", audits, err)
	}

	// Still busy: the retry proves "never admitted" again and waits; it is
	// never an unknown result and never a second switch.
	err = p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, actor)
	if !errors.As(err, &notAdmitted) || errors.Is(err, errServerSetupDNSReconciliationRequired) {
		t.Fatalf("busy retry: %v", err)
	}
	if again := requireAttachedActivatingSwitch(t, p, id); again.SwitchID != persisted.SwitchID {
		t.Fatal("retry replaced the accepted switch")
	}

	// The host is free: the same request and owner identity are admitted.
	setDNSTestAgentBusy(agent, false)
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, actor); err != nil {
		t.Fatalf("resume: %v", err)
	}
	final, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), id)
	if err != nil || final.Phase != "committed" || final.SwitchID != persisted.SwitchID || final.OwnerID != persisted.OwnerID {
		t.Fatalf("resumed switch: %+v %v", final, err)
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil || state.ActiveEngine != transport.DNSEngineBIND || state.CurrentSwitchID != "" {
		t.Fatalf("state: %+v %v", state, err)
	}
	if calls := dnsTestAgentSwitchCalls(agent); calls != 1 {
		t.Fatalf("switch calls: %d", calls)
	}
	// Replay of the finished step installs nothing again.
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, actor); err != nil || dnsTestAgentSwitchCalls(agent) != 1 {
		t.Fatalf("replay: %v calls=%d", err, dnsTestAgentSwitchCalls(agent))
	}
}

func TestSetupDNSNeverAdmittedProofRefusesAnyHostEvidence(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	// The real Agent proves no rollback without a failed job for the request.
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackIdentityMismatch
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	ctx := context.Background()
	id := strings.Repeat("d", 32)
	setDNSTestAgentBusy(agent, true)
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); err == nil {
		t.Fatal("refused lease succeeded")
	}
	persisted := requireAttachedActivatingSwitch(t, p, id)
	if err := attachDNSEngineOperationAction(ctx, p.db.GetDB(), &persisted); err != nil {
		t.Fatal(err)
	}
	if proven, err := p.setupDNSSwitchNeverAdmitted(ctx, persisted); err != nil || !proven {
		t.Fatalf("baseline proof: %v %v", proven, err)
	}

	// A running job for the request: a genuinely running mutation is never
	// admitted a second time.
	agent.durableMutationRPCFixture.mu.Lock()
	agent.jobs[id] = &ServiceOperationMutationJob{RequestID: id, OwnerID: persisted.OwnerID, Kind: dnsEngineSwitchKind,
		Target: string(persisted.TargetEngine), PackageName: persisted.Qualifier, Status: agentMutationRunning}
	agent.durableMutationRPCFixture.mu.Unlock()
	if proven, _ := p.setupDNSSwitchNeverAdmitted(ctx, persisted); proven {
		t.Fatal("a running job was treated as never admitted")
	}
	err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1})
	var notAdmitted *serverSetupDNSNotAdmittedError
	if errors.As(err, &notAdmitted) || dnsTestAgentSwitchCalls(agent) != 0 {
		t.Fatalf("running job retried: %v calls=%d", err, dnsTestAgentSwitchCalls(agent))
	}
	agent.durableMutationRPCFixture.mu.Lock()
	delete(agent.jobs, id)
	agent.durableMutationRPCFixture.mu.Unlock()

	// A retained switch journal or a running engine is host evidence: the
	// result stays unknown (the earlier, conservative path).
	agent.mu.Lock()
	agent.retainedJournalRequestID = id
	agent.mu.Unlock()
	if proven, _ := p.setupDNSSwitchNeverAdmitted(ctx, persisted); proven {
		t.Fatal("a retained journal was treated as never admitted")
	}
	agent.mu.Lock()
	agent.retainedJournalRequestID = ""
	agent.runtimes[transport.DNSEngineBIND] = transport.DNSBackendRuntimeState{Engine: transport.DNSEngineBIND, Unit: "bind9.service", Installed: true, Running: true}
	agent.mu.Unlock()
	if proven, _ := p.setupDNSSwitchNeverAdmitted(ctx, persisted); proven {
		t.Fatal("a running engine was treated as never admitted")
	}
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); !errors.Is(err, errServerSetupDNSReconciliationRequired) {
		t.Fatalf("unproven absence must stay unknown: %v", err)
	}
	if dnsTestAgentSwitchCalls(agent) != 0 {
		t.Fatal("unproven absence started the switch")
	}
}

func TestSetupDNSNotAdmittedRunnerWaitsThenReleasesAndANewPlanInstalls(t *testing.T) {
	for _, busy := range []bool{true, false} {
		name := "busy"
		if !busy {
			name = "other_refusal"
		}
		t.Run(name, func(t *testing.T) {
			p := newDNSPanelForTest(t)
			seedSetupDNSOwner(t, p)
			agent := newDNSEngineTestAgent()
			agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackIdentityMismatch
			attachDNSEngineTestAgent(t, p, agent)
			t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
			ctx := context.Background()
			id := strings.Repeat("e", 32)
			other := strings.Repeat("f", 32)
			if busy {
				setDNSTestAgentBusy(agent, true)
			} else {
				// Another Agent job holds the lease: a plain refusal.
				agent.durableMutationRPCFixture.mu.Lock()
				agent.jobs = map[string]*ServiceOperationMutationJob{other: {RequestID: other, OwnerID: other, Kind: "service_install", Target: "redis", Status: agentMutationRunning}}
				agent.active = other
				agent.durableMutationRPCFixture.mu.Unlock()
			}
			err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1})
			var notAdmitted *serverSetupDNSNotAdmittedError
			if !errors.As(err, &notAdmitted) || notAdmitted.busy() != busy {
				t.Fatalf("refusal: %v", err)
			}
			step := &serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{ID: "01-dns", Kind: "dns", Target: "local"}, RequestID: id}
			now := time.Now()
			waitErr := p.serverSetupAgentRequestRefusedBusy(ctx, step, err)
			var wait *serverSetupPeerWait
			wantWait := serverSetupDNSNotAdmittedCode
			if busy {
				wantWait = serverSetupHostBusyCode
			}
			if !errors.As(waitErr, &wait) || wait.Code != wantWait || step.BusyAttempts != 1 {
				t.Fatalf("wait: %v %+v", waitErr, step)
			}
			requireAttachedActivatingSwitch(t, p, id)

			// Past the bound the reservation is released as unchanged.
			step.BusySince = now.Add(-serverSetupHostBusyBound - time.Minute).UTC().Format(time.RFC3339Nano)
			failErr := p.serverSetupAgentRequestRefusedBusy(ctx, step, err)
			var released *serverSetupDNSNotAdmittedReleased
			if !errors.As(failErr, &released) {
				t.Fatalf("past bound: %v", failErr)
			}
			failure := serverSetupFailureForStep(*step, failErr)
			wantCode := serverSetupDNSNotAdmittedFailedCode
			if busy {
				wantCode = serverSetupHostStayedBusyCode
			}
			if failure.Code != wantCode || !strings.Contains(failure.Message, "Review a revised plan") {
				t.Fatalf("failure: %+v", failure)
			}
			old, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), id)
			if err != nil || old.Phase != "rolled_back" {
				t.Fatalf("released switch: %+v %v", old, err)
			}
			state, err := readDNSEngineDBState(ctx, p.db.GetDB())
			if err != nil || state.CurrentSwitchID != "" || state.ActiveEngine != "" {
				t.Fatalf("state after release: %+v %v", state, err)
			}
			var audits int
			if err := p.db.GetDB().QueryRow(`SELECT count(*) FROM audit_logs WHERE action LIKE 'dns.engine.switch.change_not_committed %'`).Scan(&audits); err != nil || audits != 1 {
				t.Fatalf("release audit: %d %v", audits, err)
			}

			// The host is free and a revised plan (new request) installs DNS.
			setDNSTestAgentBusy(agent, false)
			agent.durableMutationRPCFixture.mu.Lock()
			agent.active = ""
			agent.jobs = nil
			agent.durableMutationRPCFixture.mu.Unlock()
			if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), strings.Repeat("9", 32), serviceOperationActor{UserID: 1}); err != nil {
				t.Fatalf("revised plan: %v", err)
			}
			if calls := dnsTestAgentSwitchCalls(agent); calls != 1 {
				t.Fatalf("switch calls: %d", calls)
			}
		})
	}
}

// The owner's DNS reconcile (Server -> DNS) proves the rollback from Agent
// evidence, which a never-admitted lease cannot have: it now releases the
// switch unchanged instead of staying "unverified".
func TestDNSEngineOwnerReconcileReleasesNeverAdmittedSwitch(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackIdentityMismatch
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	ctx := context.Background()
	id := strings.Repeat("b", 32)
	setDNSTestAgentBusy(agent, true)
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); err == nil {
		t.Fatal("refused lease succeeded")
	}
	setDNSTestAgentBusy(agent, false)
	persisted := requireAttachedActivatingSwitch(t, p, id)

	response := reconcileDNSEngineForTest(t, p, "POST")
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"reconciled":true`) {
		t.Fatalf("reconcile: %d %s", response.Code, response.Body.String())
	}
	released, err := readDNSEngineSwitchByRequest(ctx, p.db.GetDB(), id)
	if err != nil || released.Phase != "rolled_back" || released.SwitchID != persisted.SwitchID {
		t.Fatalf("released: %+v %v", released, err)
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil || state.CurrentSwitchID != "" || state.ActiveEngine != "" {
		t.Fatalf("state: %+v %v", state, err)
	}
	if dnsTestAgentSwitchCalls(agent) != 0 {
		t.Fatal("release reached the host switch")
	}
	// Setup then shows the verified "undone, review a revised plan".
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); !errors.Is(err, errServerSetupDNSRolledBack) {
		t.Fatalf("setup after release: %v", err)
	}
}

// A panel restart (for example the owner's update of a server left in the
// D1 state) already finishes such a switch from the runtime proof without
// degrading the service subsystem; pinned here for the update path.
func TestDNSEngineStartupFinishesNeverAdmittedSetupSwitchWithoutDegrading(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackIdentityMismatch
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	ctx := context.Background()
	id := strings.Repeat("a", 32)
	setDNSTestAgentBusy(agent, true)
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); err == nil {
		t.Fatal("refused lease succeeded")
	}
	setDNSTestAgentBusy(agent, false)
	requireAttachedActivatingSwitch(t, p, id)
	if _, err := p.recoverInterruptedServiceOperations(ctx); err != nil {
		t.Fatalf("startup degraded on a never-admitted switch: %v", err)
	}
	state, err := readDNSEngineDBState(ctx, p.db.GetDB())
	if err != nil || state.CurrentSwitchID != "" {
		t.Fatalf("state: %+v %v", state, err)
	}
	if err := p.startServerSetupDNS(ctx, setupDNSTestDraft(), id, serviceOperationActor{UserID: 1}); !errors.Is(err, errServerSetupDNSRolledBack) {
		t.Fatalf("setup after restart: %v", err)
	}
	if dnsTestAgentSwitchCalls(agent) != 0 {
		t.Fatal("startup reached the host switch")
	}
}
