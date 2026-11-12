package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab (12 Nov 2026): while the Panel's own re-check of a pending
// DNS removal ran, the exact Agent job was running, not pending, and every
// such poll answered "this deletion stopped ... nothing more happens by
// itself", between two truthful answers. The status now keeps the reason the
// reconciler last read while it re-checks; a running job the reconciler does
// not track keeps the earlier conservative answer
// (TestDomainDeletionStatusNamesAgentHold).
func TestDomainDeletionStatusStaysTruthfulDuringRecheck(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, lease := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)
	agent := newDNSZoneV3TestAgent()
	agent.durableMutationRPCFixture.jobs = map[string]*ServiceOperationMutationJob{}
	phase, err := dnsZoneSyncV3PendingPhase(lease.identity())
	if err != nil {
		t.Fatal(err)
	}
	code := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck)
	job := &ServiceOperationMutationJob{
		RequestID: lease.RequestID, OwnerID: lease.OwnerID,
		Kind: "dns_zone_sync", Target: lease.ZoneName, PackageName: lease.Qualifier,
		Status: agentMutationPending, Phase: phase, ErrorCode: code,
	}
	agent.durableMutationRPCFixture.jobs[lease.RequestID] = job
	attachDNSZoneV3TestAgent(t, p, agent)

	// Pending: the reviewed reason.
	_, pending := readDeletionStatusForTest(t, p, id)
	if pending.Reason != transport.DNSPeerPendingProofTimeout || pending.Detail != transport.DNSPeerProofTimeoutLocalCheck {
		t.Fatalf("pending: %+v", pending)
	}

	// The reconciler read the reason, then its re-check runs: the same
	// answer as while pending, on every read.
	p.dnsPendingPublications().observeCode(lease, code)
	job.Status = agentMutationRunning
	job.ErrorCode = ""
	var body domainDeletionStatusResponse
	for i := 0; i < 3; i++ {
		_, body = readDeletionStatusForTest(t, p, id)
		if body.Reason != pending.Reason || body.Detail != pending.Detail || body.Stage != "dns_cleanup" ||
			!strings.Contains(body.Message, "finishes the deletion by itself") ||
			strings.Contains(body.Message, "stopped before it finished") {
			t.Fatalf("re-check in flight, read %d: %+v", i, body)
		}
	}

	// A running job of another operation discloses nothing.
	job.OwnerID = strings.Repeat("d", 32)
	_, body = readDeletionStatusForTest(t, p, id)
	if body.Reason != domainDeletionIncompleteReason {
		t.Fatalf("foreign running job: %+v", body)
	}
}

// The next-step sentence follows the reconciler's state: stuck (the
// administrators were notified) or automatic checks ended; never "finishes
// by itself" when a person must act (alpha.133 review).
func TestDomainDeletionStatusNextStepFollowsTheReconciler(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, lease := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)
	agent := newDNSZoneV3TestAgent()
	agent.durableMutationRPCFixture.jobs = map[string]*ServiceOperationMutationJob{}
	phase, err := dnsZoneSyncV3PendingPhase(lease.identity())
	if err != nil {
		t.Fatal(err)
	}
	code := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	job := &ServiceOperationMutationJob{
		RequestID: lease.RequestID, OwnerID: lease.OwnerID,
		Kind: "dns_zone_sync", Target: lease.ZoneName, PackageName: lease.Qualifier,
		Status: agentMutationPending, Phase: phase, ErrorCode: code,
	}
	agent.durableMutationRPCFixture.jobs[lease.RequestID] = job
	attachDNSZoneV3TestAgent(t, p, agent)
	scheduler := p.dnsPendingPublications()
	scheduler.observeCode(lease, code)
	for _, tc := range []struct {
		name          string
		stuck, stop   bool
		running       bool
		want, notWant string
	}{
		{name: "automatic", want: "finishes the deletion by itself", notWant: "notified"},
		{name: "stuck", stuck: true, want: "who was notified in the panel", notWant: "nothing more happens"},
		{name: "stuck, re-check running", stuck: true, running: true, want: "who was notified in the panel", notWant: "nothing more happens"},
		{name: "stopped", stop: true, want: "nothing more happens by itself", notWant: "by itself (at most every 5 minutes)"},
	} {
		scheduler.mu.Lock()
		scheduler.entries[lease.RequestID].Stuck = tc.stuck
		scheduler.entries[lease.RequestID].Stopped = tc.stop
		scheduler.mu.Unlock()
		job.Status, job.ErrorCode = agentMutationPending, code
		if tc.running {
			job.Status, job.ErrorCode = agentMutationRunning, ""
		}
		_, body := readDeletionStatusForTest(t, p, id)
		if body.Reason != transport.DNSPeerPendingProofTimeout || !strings.Contains(body.Message, tc.want) ||
			strings.Contains(body.Message, tc.notWant) {
			t.Fatalf("%s: %+v", tc.name, body)
		}
	}
}
