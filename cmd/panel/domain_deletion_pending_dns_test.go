package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129: deleting a domain while its DNS publication is pending on a DNS
// pair (alpha.128 lab, round 1: a domain made with an API token answered 409
// DNS_PUBLICATION_PENDING; the token's deletion then stopped at dns_cleanup
// and needed "Retry this deletion"). These tests drive the real deletion
// request, the read-only status, the pending-publication reconciler and the
// deletion reconciler against a fake Agent whose backup DNS server stays
// behind, then catches up or the removal fails. A token deletion and a panel
// deletion are the same request (handleDeleteDomain); the token adds only
// confirm_domain in front of it (api_tokens.go).

type pendingDeletionFixture struct {
	panel    *Panel
	agent    *dnsZoneV3TestAgent
	clock    *pendingPublicationClock
	domain   string
	domainID int
	down     atomic.Bool
	fail     atomic.Bool
}

func newPendingDeletionFixture(t *testing.T, domain string) *pendingDeletionFixture {
	t.Helper()
	panel, agent, clock, _ := newPendingPublicationPanel(t)
	f := &pendingDeletionFixture{panel: panel, agent: agent, clock: clock, domain: domain}
	f.domainID, _ = seedDomainDeletionLedger(t, panel, domain, "dnsonly")
	seedStrictDNSZone(t, panel, domain)
	// While the backup is down every publication ends pending (the Agent's
	// propagation check did not see the backup catch up); afterwards it
	// completes. A failing recovery ends the exact operation failed.
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	pending := agent.syncResponseHook
	agent.syncResponseHook = func(request transport.SyncDNSZoneV3Request, response *transport.SyncDNSZoneV3Response) error {
		if f.down.Load() {
			return pending(request, response)
		}
		return nil
	}
	agent.recoverHook = func(request transport.RecoverDNSZoneV3Request, response *transport.RecoverDNSZoneV3Response) error {
		switch {
		case f.fail.Load():
			if err := markDNSZoneV3FailedForTest(agent, request.MutationRequestID); err != nil {
				response.Error = err.Error()
			}
			return nil
		case f.down.Load():
			if err := agent.markDNSZoneV3Pending(request.MutationRequestID, request.MutationOwnerID, request.Domain, request.Qualifier); err != nil {
				response.Error = err.Error()
				return nil
			}
			setPendingJobCode(t, agent, request.MutationRequestID, transport.DNSPeerPendingCatchingUp)
			response.RecoveryPending = true
			response.PendingCode = transport.DNSPeerPendingCatchingUp
			return nil
		}
		if err := agent.markDNSZoneV3Succeeded(request.MutationRequestID, request.MutationOwnerID, request.Domain, request.Qualifier); err != nil {
			response.Error = err.Error()
			return nil
		}
		response.Recovered = true
		return nil
	}
	return f
}

func markDNSZoneV3FailedForTest(agent *dnsZoneV3TestAgent, requestID string) error {
	agent.durableMutationRPCFixture.mu.Lock()
	defer agent.durableMutationRPCFixture.mu.Unlock()
	job := agent.durableMutationRPCFixture.jobs[requestID]
	if job == nil {
		return nil
	}
	job.Status = agentMutationFailed
	job.ErrorMessage = "the backup DNS server refused the zone removal"
	job.LeaseExpiresAt = time.Time{}
	if agent.durableMutationRPCFixture.active == requestID {
		agent.durableMutationRPCFixture.active = ""
	}
	return nil
}

type pendingDeletionAnswer struct {
	Status    string `json:"status"`
	Stage     string `json:"stage"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	StatusURL string `json:"status_url"`
}

func (f *pendingDeletionFixture) delete(t *testing.T) (int, pendingDeletionAnswer) {
	t.Helper()
	recorder := deleteDomainForSagaTest(t, f.panel, f.domainID)
	var answer pendingDeletionAnswer
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	return recorder.Code, answer
}

func (f *pendingDeletionFixture) calls() (syncs, recoveries int) {
	s, r := pendingAgentCalls(f.agent)
	return len(s), len(r)
}

// settle runs the background reconcilers as the running panel does: the
// pending-publication re-check (after its pause) and the deletion finisher.
func (f *pendingDeletionFixture) settle(t *testing.T, rounds int) {
	t.Helper()
	for i := 0; i < rounds; i++ {
		f.clock.Advance(dnsPendingPublicationMaxDelay)
		f.panel.reconcilePendingDNSPublicationsOnce(context.Background())
		f.panel.reconcileLocalDNSDeletionsOnce(context.Background())
	}
}

func (f *pendingDeletionFixture) domainLeft(t *testing.T) bool {
	t.Helper()
	var n int
	if err := f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM domains WHERE id = ?`, f.domainID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n != 0
}

// pollStatusChangesNothing reads the status several times and proves that
// no read started an Agent call (D-024: polling never mutates).
func (f *pendingDeletionFixture) pollStatusChangesNothing(t *testing.T) domainDeletionStatusResponse {
	t.Helper()
	syncs, recoveries := f.calls()
	var body domainDeletionStatusResponse
	for i := 0; i < 5; i++ {
		var status int
		status, body = readDeletionStatusForTest(t, f.panel, f.domainID)
		if status != http.StatusOK {
			t.Fatalf("status read %d answered %d", i, status)
		}
	}
	if s, r := f.calls(); s != syncs || r != recoveries {
		t.Fatalf("status reads started Agent work: syncs %d->%d recoveries %d->%d", syncs, s, recoveries, r)
	}
	return body
}

func requirePendingDeletionAnswer(t *testing.T, code int, answer pendingDeletionAnswer, f *pendingDeletionFixture) {
	t.Helper()
	if code != http.StatusAccepted || answer.Status != domainDeletionPendingStatus || answer.Stage != "dns_cleanup" ||
		answer.Reason != transport.DNSPeerPendingCatchingUp ||
		!strings.Contains(answer.Message, "finishes the deletion by itself") ||
		answer.StatusURL != domainDeletionStatusURL(f.domainID) {
		t.Fatalf("pending deletion answer = %d %+v", code, answer)
	}
}

// The lab case: the domain's own creation is still waiting for the backup
// when the deletion is sent. The deletion answers 202 with what happens
// next, the status says the same (not "stopped"), and once the backup
// catches up the removal is published and the deletion finishes by itself:
// one creation and one removal reach the Agent, nothing twice.
func TestDeletionWhileTheDomainsCreationIsPendingFinishesByItself(t *testing.T) {
	f := newPendingDeletionFixture(t, "token-made.example")
	f.down.Store(true)
	if err := f.panel.syncZoneToDNS(context.Background(), f.domain, false); err == nil {
		t.Fatal("the creation did not stay pending")
	}

	code, answer := f.delete(t)
	requirePendingDeletionAnswer(t, code, answer, f)
	body := f.pollStatusChangesNothing(t)
	if body.Status != domainDeletionPendingStatus || body.Stage != "dns_cleanup" ||
		body.Reason != transport.DNSPeerPendingCatchingUp || !strings.Contains(body.Message, "finishes the deletion by itself") {
		t.Fatalf("status while the creation is pending = %+v", body)
	}
	// Still pending after a re-check: nothing is finished early.
	f.settle(t, 1)
	if !f.domainLeft(t) {
		t.Fatal("the deletion finished before the backup confirmed anything")
	}

	f.down.Store(false)
	f.settle(t, 3)
	if f.domainLeft(t) {
		_, body := readDeletionStatusForTest(t, f.panel, f.domainID)
		t.Fatalf("the deletion did not finish by itself: %+v", body)
	}
	syncs, _ := pendingAgentCalls(f.agent)
	if len(syncs) != 2 || syncs[0].Delete || !syncs[1].Delete {
		t.Fatalf("Agent publications = %d (want the creation, then one removal)", len(syncs))
	}
	if status, _ := readDeletionStatusForTest(t, f.panel, f.domainID); status != http.StatusNoContent {
		t.Fatalf("status after the deletion = %d", status)
	}
}

// The removal itself waits for the backup: answered 202, then finished by
// itself once the backup confirms (the fix of ddbd6975b, end to end).
func TestDeletionWhoseRemovalIsPendingFinishesByItself(t *testing.T) {
	f := newPendingDeletionFixture(t, "removal-pending.example")
	if err := f.panel.syncZoneToDNS(context.Background(), f.domain, false); err != nil {
		t.Fatalf("creation: %v", err)
	}
	f.down.Store(true)
	code, answer := f.delete(t)
	requirePendingDeletionAnswer(t, code, answer, f)
	body := f.pollStatusChangesNothing(t)
	if body.Reason != transport.DNSPeerPendingCatchingUp || !strings.Contains(body.Message, "finishes the deletion by itself") {
		t.Fatalf("status while the removal is pending = %+v", body)
	}
	// A second press while it continues by itself starts nothing.
	syncs, recoveries := f.calls()
	if code, again := f.delete(t); code != http.StatusAccepted || again.Reason != domainDeletionAlreadyRunningReason {
		t.Fatalf("second press = %d %+v", code, again)
	}
	if s, r := f.calls(); s != syncs || r != recoveries {
		t.Fatalf("a second press started Agent work: syncs %d->%d recoveries %d->%d", syncs, s, recoveries, r)
	}
	f.down.Store(false)
	f.settle(t, 2)
	if f.domainLeft(t) {
		_, body := readDeletionStatusForTest(t, f.panel, f.domainID)
		t.Fatalf("the deletion did not finish by itself: %+v", body)
	}
	if s, _ := f.calls(); s != 2 {
		t.Fatalf("Agent publications = %d (want the creation and one removal)", s)
	}
}

// The removal waits, then the Agent ends it failed: the deletion stops with
// a plain reason and the owner's retry, it is not finished by itself, and
// the retry (with the backup back) completes it.
func TestDeletionWhoseRemovalFailsStopsWithAPlainReasonAndTheRetry(t *testing.T) {
	f := newPendingDeletionFixture(t, "removal-fails.example")
	if err := f.panel.syncZoneToDNS(context.Background(), f.domain, false); err != nil {
		t.Fatalf("creation: %v", err)
	}
	f.down.Store(true)
	code, answer := f.delete(t)
	requirePendingDeletionAnswer(t, code, answer, f)
	f.fail.Store(true)
	f.settle(t, 3)
	if !f.domainLeft(t) {
		t.Fatal("a failed removal was finished as if it had succeeded")
	}
	body := f.pollStatusChangesNothing(t)
	if body.Status != domainDeletionPendingStatus || body.Stage != "dns_cleanup" ||
		body.Reason != domainDeletionDNSRemovalFailedReason || !strings.Contains(body.Message, "Retry this deletion") ||
		strings.Contains(body.Message, "refused the zone removal") {
		t.Fatalf("status after a failed removal = %+v", body)
	}
	// Nothing more happens by itself.
	syncs, recoveries := f.calls()
	f.settle(t, 3)
	if s, r := f.calls(); s != syncs || r != recoveries || !f.domainLeft(t) {
		t.Fatalf("a failed removal was retried by itself: syncs %d->%d recoveries %d->%d", syncs, s, recoveries, r)
	}
	// The owner's retry, with the backup back.
	f.fail.Store(false)
	f.down.Store(false)
	if code, answer := f.delete(t); code != http.StatusOK || answer.Status != "deleted" {
		t.Fatalf("retry = %d %+v", code, answer)
	}
	if f.domainLeft(t) {
		t.Fatal("the retry did not delete the domain")
	}
}

// A token reaches the read-only deletion status of its own domains, so the
// 202's status_url can be followed with the same token.
func TestAPITokenReachesTheDeletionStatus(t *testing.T) {
	route, ok := apiTokenDomainRoutes["GET deletion-status"]
	if !ok || route.write || route.family != "domains" {
		t.Fatalf("deletion-status token route = %+v %v", route, ok)
	}
}
