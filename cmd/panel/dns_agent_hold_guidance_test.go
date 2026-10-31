package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Defect D5 (alpha.113 final lab, 2026-10-31): a deletion whose exact DNS
// publication the Agent could not verify left the Agent holding every change.
// "Retry this deletion" then answered "DNS V3 publication is already in
// progress" and the saved deletion read "status unknown": neither named the
// hold nor the owner's next step (restart the Agent). These tests pin the
// guidance and that the same deletion completes once the Agent's own restart
// recovery finished the exact operation.

func seedHeldDeletionJob(t *testing.T, p *Panel) (int, dnsZoneEngineLease, *dnsZoneV3TestAgent) {
	t.Helper()
	id, lease := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)
	agent := newDNSZoneV3TestAgent()
	agent.publisher = transport.DNSEnginePowerDNS
	agent.durableMutationRPCFixture.jobs = map[string]*ServiceOperationMutationJob{
		lease.RequestID: {
			RequestID: lease.RequestID, OwnerID: lease.OwnerID,
			Kind: "dns_zone_sync", Target: lease.ZoneName, PackageName: lease.Qualifier,
			Status:  agentMutationRunning,
			Phase:   "commit/dns-zone-sync/v3/applied/" + lease.RequestID + "/" + lease.ZoneName + "/" + lease.Qualifier,
			Attempt: 1, LeaseExpiresAt: time.Now().Add(time.Minute), DeadlineAt: time.Now().Add(time.Hour),
		},
	}
	agent.durableMutationRPCFixture.active = lease.RequestID
	agent.mutationHold = transport.MutationHoldLedgerAmbiguous
	attachDNSZoneV3TestAgent(t, p, agent)
	return id, lease, agent
}

func TestHeldAgentDNSPublicationNamesTheHoldInsteadOfInProgress(t *testing.T) {
	p := newDNSPanelForTest(t)
	_, lease, agent := seedHeldDeletionJob(t, p)
	ctx := context.Background()

	for _, waitActive := range []bool{false, true} {
		done, err := p.reconcileDNSZoneEngineLease(ctx, lease, waitActive)
		var publicationErr *dnsAgentPublicationError
		if done || !errors.As(err, &publicationErr) || !errors.Is(err, errAgentMutationHeld) {
			t.Fatalf("wait=%v held reconcile done=%v err=%v", waitActive, done, err)
		}
		if strings.Contains(err.Error(), "already in progress") {
			t.Fatalf("wait=%v held publication reported as in progress: %v", waitActive, err)
		}
		guidance, ok := dnsPublicationGuidanceAPIError(err)
		if !ok || guidance.Code != errCodeDNSPublicationFailed ||
			guidance.Reason != dnsPublicationAgentHeldReason ||
			!strings.Contains(guidance.Error, "systemctl restart celikpanel-agent") ||
			!strings.Contains(guidance.Error, "Retry this deletion") ||
			strings.Contains(guidance.Error, lease.RequestID) {
			t.Fatalf("wait=%v held guidance=%+v ok=%v", waitActive, guidance, ok)
		}
	}
	if leaseCount(t, p) != 1 {
		t.Fatal("a held publication lost its exact lease")
	}

	// Without a hold an active job is still genuinely in progress.
	agent.mutationHold = ""
	_, err := p.reconcileDNSZoneEngineLease(ctx, lease, false)
	if err == nil || errors.Is(err, errAgentMutationHeld) || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("unheld active publication err=%v", err)
	}

	// The owner restarts the Agent; its startup recovery proves and completes
	// the exact operation. The same retry now records it: no new mutation.
	if err := agent.markDNSZoneV3Succeeded(lease.RequestID, lease.OwnerID, lease.ZoneName, lease.Qualifier); err != nil {
		t.Fatal(err)
	}
	done, err := p.reconcileDNSZoneEngineLease(ctx, lease, false)
	if err != nil || !done {
		t.Fatalf("recovered publication done=%v err=%v", done, err)
	}
	if leaseCount(t, p) != 0 {
		t.Fatal("recorded publication kept its lease")
	}
	if syncs, recoveries := pendingAgentCalls(agent); len(syncs) != 0 || len(recoveries) != 0 {
		t.Fatalf("recording a finished operation started syncs=%d recoveries=%d", len(syncs), len(recoveries))
	}
}

func TestDomainDeletionStatusNamesAgentHold(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, lease, agent := seedHeldDeletionJob(t, p)
	status, body := readDeletionStatusForTest(t, p, id)
	if status != http.StatusOK || body.Status != domainDeletionPendingStatus ||
		body.Stage != "dns_cleanup" || body.Reason != dnsPublicationAgentHeldReason ||
		!strings.Contains(body.Message, "systemctl restart celikpanel-agent") ||
		strings.Contains(body.Message, lease.RequestID) {
		t.Fatalf("held deletion status=%d body=%+v", status, body)
	}
	// A foreign job under the same hold discloses nothing.
	agent.durableMutationRPCFixture.mu.Lock()
	agent.durableMutationRPCFixture.jobs[lease.RequestID].OwnerID = strings.Repeat("d", 32)
	agent.durableMutationRPCFixture.mu.Unlock()
	if _, body := readDeletionStatusForTest(t, p, id); body.Reason != "" || body.Status != "unknown" {
		t.Fatalf("foreign held job disclosed a reason: %+v", body)
	}
	agent.durableMutationRPCFixture.mu.Lock()
	agent.durableMutationRPCFixture.jobs[lease.RequestID].OwnerID = lease.OwnerID
	agent.durableMutationRPCFixture.mu.Unlock()
	// No hold: an active job keeps the earlier conservative answer.
	agent.mutationHold = ""
	if _, body := readDeletionStatusForTest(t, p, id); body.Reason != "" || body.Status != "unknown" {
		t.Fatalf("unheld active job reported a reason: %+v", body)
	}
}

// After repeated failed attempts (a held Agent) the reconciler stops trying;
// once the Agent itself finished the exact operation, the reconciler records
// that receipt by itself without starting anything on the server.
func TestDNSPendingPublicationRecordsAgentFinishedOperationAfterStop(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "stopped-pending.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingEnrollmentRequired)
	recoverOutcome(t, agent, func() bool { return true }, func() string { return transport.DNSPeerPendingEnrollmentRequired })
	ctx := context.Background()
	if err := panel.syncZoneToDNS(ctx, "stopped-pending.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "stopped-pending.example")
	if err != nil {
		t.Fatal(err)
	}
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if entry, _ := scheduler.entry(lease.RequestID); entry.Automatic {
		t.Fatalf("owner-action entry is automatic: %+v", entry)
	}
	syncsBefore, recoveriesBefore := pendingAgentCalls(agent)

	if err := agent.markDNSZoneV3Succeeded(lease.RequestID, lease.OwnerID, lease.ZoneName, lease.Qualifier); err != nil {
		t.Fatal(err)
	}
	clock.Advance(dnsPendingPublicationOwnerRecheck)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	syncs, recoveries := pendingAgentCalls(agent)
	if len(syncs) != len(syncsBefore) || len(recoveries) != len(recoveriesBefore) {
		t.Fatalf("recording started syncs=%d->%d recoveries=%d->%d",
			len(syncsBefore), len(syncs), len(recoveriesBefore), len(recoveries))
	}
	state, err := readDNSZoneSyncState(ctx, panel.db.GetDB(), "stopped-pending.example")
	if err != nil || state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration || leaseCount(t, panel) != 0 {
		t.Fatalf("finished operation not recorded: state=%+v leases=%d err=%v", state, leaseCount(t, panel), err)
	}
}
