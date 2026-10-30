package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Final alpha.113 acceptance run 3: the first, transient answer of a paired
// deletion's peer proof (dns_peer_inspection_unknown) was the state a read
// saw while the same request was about to check again, and its guidance
// named people who must act. A read during the request says the deletion is
// running and re-checked automatically; once the request has ended the
// reviewed reason and its actions are shown as before.
func TestDomainDeletionStatusSaysRunningWhileTheRequestRuns(t *testing.T) {
	p := newDNSPanelForTest(t)
	id, lease := seedExactDeletionStatusForEngine(t, p, transport.DNSEnginePowerDNS)
	agent := newDNSZoneV3TestAgent()
	agent.durableMutationRPCFixture.jobs = map[string]*ServiceOperationMutationJob{}
	phase, err := dnsZoneSyncV3PendingPhase(lease.identity())
	if err != nil {
		t.Fatal(err)
	}
	agent.durableMutationRPCFixture.jobs[lease.RequestID] = &ServiceOperationMutationJob{
		RequestID: lease.RequestID, OwnerID: lease.OwnerID,
		Kind: "dns_zone_sync", Target: lease.ZoneName, PackageName: lease.Qualifier,
		Status: agentMutationPending, Phase: phase,
		ErrorCode: transport.DNSPeerPendingInspectionUnknown,
	}
	attachDNSZoneV3TestAgent(t, p, agent)

	end := p.deletionsRunning.begin(id)
	status, body := readDeletionStatusForTest(t, p, id)
	if status != http.StatusOK || body.Status != domainDeletionPendingStatus ||
		body.Stage != domainDeletionRunningStage || body.Reason != domainDeletionRunningReason {
		t.Fatalf("running deletion status=%d body=%+v", status, body)
	}
	for _, want := range []string{"running now", "checked again automatically", "Nobody needs to do anything"} {
		if !strings.Contains(body.Message, want) {
			t.Fatalf("running message lacks %q: %s", want, body.Message)
		}
	}
	if strings.Contains(body.Message, "must check") || strings.Contains(body.Message, "SSH") {
		t.Fatalf("running message escalates: %s", body.Message)
	}
	// Another domain is not affected.
	if p.deletionsRunning.isRunning(id + 1) {
		t.Fatal("another domain reported running")
	}

	end()
	end() // idempotent
	_, body = readDeletionStatusForTest(t, p, id)
	if body.Reason != transport.DNSPeerPendingInspectionUnknown || body.Stage != "dns_cleanup" {
		t.Fatalf("after the request the reviewed reason must be shown: %+v", body)
	}

	// Two overlapping requests: running until both ended.
	first, second := p.deletionsRunning.begin(id), p.deletionsRunning.begin(id)
	first()
	if !p.deletionsRunning.isRunning(id) {
		t.Fatal("ended while a request still runs")
	}
	second()
	if p.deletionsRunning.isRunning(id) {
		t.Fatal("still running after both requests ended")
	}
}
