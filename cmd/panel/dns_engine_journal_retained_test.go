package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Arch PowerDNS drill apd-e (2026-10-30): after a cut, the Agent's undo
// stopped and it kept the switch journal; the Panel's startup recovery still
// recorded the switch as rolled back from a stopped-engine observation, and
// setup said "CelikPanel undid it". Component tests only, not native evidence.

// Startup recovery never records a rollback while the Agent keeps the
// journal; it stays attached and operator-gated (not degraded). Once the
// journal is gone, the same proof records the rollback.
func TestDNSEngineStartupRecoveryKeepsSwitchWhileAgentKeepsJournal(t *testing.T) {
	panel := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, panel, "standalone")
	agent := newDNSEngineTestAgent()
	attachDNSEngineTestAgent(t, panel, agent)
	persisted := persistEmptyDNSEngineSwitchForTest(t, panel, transport.DNSEngineBIND, strings.Repeat("c", 32))
	job := terminalDNSEngineJob(persisted, agentMutationFailed)
	job.Phase = "interrupted"
	job.ErrorCode = dnsengineartifact.ReleasedNativeUnknownCode
	setDNSEngineMutationJobForTest(t, agent, job, false)
	agent.mu.Lock()
	agent.retainedJournalRequestID = persisted.RequestID
	agent.mu.Unlock()

	handled, err := panel.recoverDNSEngineSwitchLocked(context.Background(), job)
	var retained *dnsSwitchJournalRetainedError
	if !handled || !errors.Is(err, errAgentMutationRecoveryRequired) || !errors.As(err, &retained) || retained.RequestID != persisted.RequestID {
		t.Fatalf("kept journal: handled=%v err=%v", handled, err)
	}
	state, err := readDNSEngineDBState(context.Background(), panel.db.GetDB())
	if err != nil || state.CurrentSwitchID != persisted.SwitchID {
		t.Fatalf("kept journal released the switch: %+v %v", state, err)
	}
	current, err := readDNSEngineSwitchByRequest(context.Background(), panel.db.GetDB(), persisted.RequestID)
	if err != nil || current.Phase == "rolled_back" {
		t.Fatalf("kept journal recorded as rolled back: %+v %v", current, err)
	}
	// The panel start path keeps running: operator-gated, not degraded.
	if _, err := panel.recoverInterruptedServiceOperations(context.Background()); err != nil {
		t.Fatalf("startup degraded on a kept journal: %v", err)
	}

	agent.mu.Lock()
	agent.retainedJournalRequestID = ""
	agent.mu.Unlock()
	if handled, err := panel.recoverDNSEngineSwitchLocked(context.Background(), job); !handled || err != nil {
		t.Fatalf("retired journal: handled=%v err=%v", handled, err)
	}
	current, err = readDNSEngineSwitchByRequest(context.Background(), panel.db.GetDB(), persisted.RequestID)
	if err != nil || current.Phase != "rolled_back" {
		t.Fatalf("retired journal not recorded: %+v %v", current, err)
	}
}

// An unreadable journal is not an absent one.
func TestRetainedDNSSwitchJournalObservation(t *testing.T) {
	id := strings.Repeat("d", 32)
	for name, tc := range map[string]struct {
		response   transport.DNSBackendReadinessResponse
		want       bool
		unverified bool
	}{
		"older agent": {},
		"retained":    {response: transport.DNSBackendReadinessResponse{SwitchJournalRequestID: id}, want: true},
		"unverified":  {response: transport.DNSBackendReadinessResponse{SwitchJournalUnverified: true}, want: true, unverified: true},
		"malformed":   {response: transport.DNSBackendReadinessResponse{SwitchJournalRequestID: "x"}, want: true, unverified: true},
	} {
		got := retainedDNSSwitchJournal(tc.response)
		if (got != nil) != tc.want || got != nil && got.Unverified != tc.unverified {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

// The drill's exact sequence through setup: the Agent ends the job released
// with its journal kept, the Panel restarts, and the step shows the held
// guidance, never "undone", and never starts a second host mutation.
func TestServerSetupDNSKeptJournalAfterRestartIsHeldNotUndone(t *testing.T) {
	openPDNSPairedPrimaryGateForTest(t)
	agent := newDNSEngineTestAgent()
	agent.switchError = "DNS engine switch did not complete; inspect the agent log"
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackJournalPresent
	x := newSetupDNSOutcomeFixture(t, freshPDNSPrimaryDraft(), agent)
	x.poll(t)
	requestID := x.execution.Steps[0].RequestID
	agent.mu.Lock()
	job := agent.jobs[requestID]
	if job == nil {
		agent.mu.Unlock()
		t.Fatal("fixture has no Agent job for the request")
	}
	job.Phase = "interrupted"
	job.ErrorCode = dnsengineartifact.ReleasedNativeUnknownCode
	job.ErrorMessage = "The Agent could not prove that this first PowerDNS install as paired primary never started, so it did not undo it and released its lease; the journal is kept."
	agent.retainedJournalRequestID = requestID
	agent.mu.Unlock()

	// The panel restarts (the drill's forced reboot).
	if _, err := x.f.panel.recoverInterruptedServiceOperations(context.Background()); err != nil {
		t.Fatalf("startup recovery: %v", err)
	}
	for poll := 0; poll < 2; poll++ {
		x.poll(t)
		if x.execution.Status != "waiting" || x.execution.Steps[0].Status != "running" || x.execution.Error == nil ||
			x.execution.Error.Code != serverSetupDNSRecoveryHeldCode ||
			strings.Contains(x.execution.Error.Message, "undid") ||
			!strings.Contains(x.execution.Error.Message, "It is not reported as undone.") ||
			!strings.Contains(x.execution.Error.Message, "The server administrator should run `sudo /usr/libexec/celikpanel/recovery dns-switch-status --quiesced --request-id "+requestID+"`") ||
			!strings.Contains(x.execution.Error.Message, "Setup rechecks this same operation automatically") {
			t.Fatalf("poll %d: kept journal not shown as held: %+v", poll, x.execution)
		}
	}
	persisted, err := readDNSEngineSwitchByRequest(context.Background(), x.f.database.GetDB(), requestID)
	if err != nil || persisted.Phase == "rolled_back" {
		t.Fatalf("kept journal recorded as rolled back: %+v %v", persisted, err)
	}
	if calls := x.switchCalls(); calls != 1 {
		t.Fatalf("polling started another host mutation: %d", calls)
	}

	// The owner's recovery retires the journal: the same step records the
	// proven rollback and only then says it was undone.
	agent.mu.Lock()
	agent.retainedJournalRequestID = ""
	agent.rollbackEvidenceOutcome = transport.DNSEngineRollbackSafe
	agent.mu.Unlock()
	x.poll(t)
	if x.execution.Status != "failed" || x.execution.Error == nil || x.execution.Error.Code != "server_setup_dns_rolled_back" {
		t.Fatalf("proven rollback not shown after the journal was retired: %+v", x.execution)
	}
}

// A new DNS install that a kept journal would refuse waits with that reason:
// nothing is persisted or sent to the Agent, and it starts by itself once the
// journal is resolved (drill apd-e retry sat at "running").
func TestServerSetupDNSInstallWaitsForRetainedJournal(t *testing.T) {
	agent := newDNSEngineTestAgent()
	earlier := strings.Repeat("e", 32)
	agent.retainedJournalRequestID = earlier
	x := newSetupDNSOutcomeFixture(t, setupDNSTestDraft(), agent)
	for poll := 0; poll < 2; poll++ {
		x.poll(t)
		if x.execution.Status != "waiting" || x.execution.Steps[0].Status != "running" || x.execution.Error == nil ||
			x.execution.Error.Code != serverSetupDNSJournalRetainedCode ||
			!strings.Contains(x.execution.Error.Message, "(request "+earlier+")") ||
			!strings.Contains(x.execution.Error.Message, "Nothing was started.") ||
			!strings.Contains(x.execution.Error.Message, "recovery dns-switch-status --quiesced --request-id "+earlier) ||
			!strings.Contains(x.execution.Error.Message, "starts the installation once that record is resolved") ||
			strings.Contains(x.execution.Error.Message, "undid") {
			t.Fatalf("poll %d: retained journal not shown as a wait: %+v", poll, x.execution)
		}
	}
	if calls := x.switchCalls(); calls != 0 {
		t.Fatalf("a refused install reached the Agent: %d", calls)
	}
	if _, err := readDNSEngineSwitchByRequest(context.Background(), x.f.database.GetDB(), x.execution.Steps[0].RequestID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("a refused install was persisted: %v", err)
	}
	unverified := serverSetupDNSJournalRetainedMessage(&serverSetupDNSJournalRetainedError{retained: &dnsSwitchJournalRetainedError{Unverified: true}})
	if unverified.Code != serverSetupDNSJournalRetainedCode || strings.Contains(unverified.Message, "--request-id") ||
		!strings.Contains(unverified.Message, "could not confirm") {
		t.Fatalf("unverified journal message: %+v", unverified)
	}

	agent.mu.Lock()
	agent.retainedJournalRequestID = ""
	agent.mu.Unlock()
	x.poll(t)
	if calls := x.switchCalls(); calls != 1 {
		t.Fatalf("install did not start once the journal was resolved: calls=%d execution=%+v", calls, x.execution)
	}
}
