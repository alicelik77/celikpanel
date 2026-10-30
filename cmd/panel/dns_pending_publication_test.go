package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Component tests of the automatic re-check of pending zone-sync V3
// publications (2026-10-30). Fake Agent; no native DNS.

type pendingPublicationClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *pendingPublicationClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *pendingPublicationClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// useTestPendingScheduler installs a fresh scheduler (as after a Panel
// restart) with a controlled clock and no jitter.
func useTestPendingScheduler(p *Panel, clock *pendingPublicationClock) *dnsPendingPublicationScheduler {
	scheduler := newDNSPendingPublicationScheduler()
	scheduler.now = clock.Now
	scheduler.jitter = func(d time.Duration) time.Duration { return d }
	p.dnsPendingPublicationsOnce.Do(func() {})
	p.dnsPendingPublicationsState = scheduler
	return scheduler
}

func setPendingJobCode(t *testing.T, agent *dnsZoneV3TestAgent, requestID, code string) {
	t.Helper()
	agent.durableMutationRPCFixture.mu.Lock()
	defer agent.durableMutationRPCFixture.mu.Unlock()
	job := agent.durableMutationRPCFixture.jobs[requestID]
	if job == nil {
		t.Fatalf("no fixture job %s", requestID)
	}
	job.ErrorCode = code
}

// configurePendingWithCode makes every V3 publication end pending with code,
// as the Agent does when the backup has not caught up within its check.
func configurePendingWithCode(t *testing.T, agent *dnsZoneV3TestAgent, code string) {
	configureDNSZoneV3PendingSync(agent, false)
	base := agent.syncResponseHook
	agent.syncResponseHook = func(request transport.SyncDNSZoneV3Request, response *transport.SyncDNSZoneV3Response) error {
		if err := base(request, response); err != nil {
			return err
		}
		setPendingJobCode(t, agent, request.MutationRequestID, code)
		response.PendingCode = code
		return nil
	}
}

// recoverOutcome scripts Agent.RecoverDNSZoneV3: while pending() is true the
// exact operation stays pending with code(); otherwise it completes.
func recoverOutcome(t *testing.T, agent *dnsZoneV3TestAgent, pending func() bool, code func() string) {
	agent.recoverHook = func(request transport.RecoverDNSZoneV3Request, response *transport.RecoverDNSZoneV3Response) error {
		if pending() {
			if err := agent.markDNSZoneV3Pending(request.MutationRequestID, request.MutationOwnerID, request.Domain, request.Qualifier); err != nil {
				response.Error = err.Error()
				return nil
			}
			setPendingJobCode(t, agent, request.MutationRequestID, code())
			response.RecoveryPending = true
			response.PendingCode = code()
			return nil
		}
		if err := agent.markDNSZoneV3Succeeded(request.MutationRequestID, request.MutationOwnerID, request.Domain, request.Qualifier); err != nil {
			response.Error = err.Error()
			return nil
		}
		response.Recovered = true
		return nil
	}
}

func pendingAgentCalls(agent *dnsZoneV3TestAgent) ([]transport.SyncDNSZoneV3Request, []transport.RecoverDNSZoneV3Request) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return append([]transport.SyncDNSZoneV3Request(nil), agent.requests...),
		append([]transport.RecoverDNSZoneV3Request(nil), agent.recoverRequests...)
}

func leaseCount(t *testing.T, p *Panel) int {
	t.Helper()
	leases, err := listDNSZoneEngineLeases(context.Background(), p.db.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	return len(leases)
}

func newPendingPublicationPanel(t *testing.T, zones ...string) (*Panel, *dnsZoneV3TestAgent, *pendingPublicationClock, *dnsPendingPublicationScheduler) {
	t.Helper()
	previousWait := dnsPendingPublicationRepublishWait
	dnsPendingPublicationRepublishWait = 0
	t.Cleanup(func() { dnsPendingPublicationRepublishWait = previousWait })
	panel := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, panel, "standalone")
	activateDNSEngineForTest(t, panel, string(transport.DNSEngineBIND))
	for _, zone := range zones {
		seedStrictDNSZone(t, panel, zone)
	}
	agent := newDNSZoneV3TestAgent()
	attachDNSZoneV3TestAgent(t, panel, agent)
	clock := &pendingPublicationClock{now: time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)}
	scheduler := useTestPendingScheduler(panel, clock)
	return panel, agent, clock, scheduler
}

func TestDNSPendingPublicationBackoffDoublesToFiveMinutes(t *testing.T) {
	scheduler := newDNSPendingPublicationScheduler()
	scheduler.jitter = func(d time.Duration) time.Duration { return d }
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for attempts, delay := range want {
		if got := scheduler.backoff(attempts); got != delay {
			t.Fatalf("backoff(%d)=%s want %s", attempts, got, delay)
		}
	}
	jittered := newDNSPendingPublicationScheduler()
	for i := 0; i < 200; i++ {
		if got := jittered.backoff(1); got < 24*time.Second || got > 36*time.Second {
			t.Fatalf("jittered 30 s backoff out of ±20%%: %s", got)
		}
	}
}

// A transient pending publication (backup not caught up) completes by itself:
// first re-check after 15 s, then after a longer wait, always the same exact
// operation; no new Sync mutation is started.
func TestDNSPendingPublicationTransientResolvesByItself(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "a-pending.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })

	err := panel.syncZoneToDNS(context.Background(), "a-pending.example", false)
	if !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial publication err=%v", err)
	}
	guidance, ok := dnsPublicationGuidanceAPIError(err)
	if !ok || guidance.Reason != transport.DNSPeerPendingCatchingUp ||
		!strings.Contains(guidance.Error, "Nobody needs to do anything") {
		t.Fatalf("pending guidance=%+v ok=%v", guidance, ok)
	}
	lease, err := readDNSZoneEngineLease(context.Background(), panel.db.GetDB(), "a-pending.example")
	if err != nil {
		t.Fatal(err)
	}

	// "Backup has not caught up" after the Agent's whole wave: no second
	// wave at once (lab 2026-10-30: it exceeded the request's budget and
	// lost the reason); the reconciler re-checks.
	ctx := context.Background()
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 0 {
		t.Fatalf("initial publication recoveries=%d", len(recoveries))
	}
	panel.reconcilePendingDNSPublicationsOnce(ctx) // discovers (reads the reason), not due yet
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 0 {
		t.Fatalf("re-checked before the first delay: %d", len(recoveries))
	}
	views, err := panel.dnsPendingPublicationViews(ctx)
	if err != nil || len(views) != 1 || views[0].Zone != "a-pending.example" || !views[0].Automatic ||
		views[0].NextCheckAt == "" || views[0].Reason != transport.DNSPeerPendingCatchingUp {
		t.Fatalf("pending view=%+v err=%v", views, err)
	}

	clock.Advance(15 * time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx) // attempt 1: still pending
	_, recoveries := pendingAgentCalls(agent)
	if len(recoveries) != 1 {
		t.Fatalf("first re-check recoveries=%d", len(recoveries))
	}
	entry, ok := scheduler.entry(lease.RequestID)
	if !ok || entry.Attempts != 1 || entry.Code != transport.DNSPeerPendingCatchingUp || !entry.Automatic ||
		entry.NextAt.Sub(clock.Now()) != 30*time.Second {
		t.Fatalf("after attempt 1 entry=%+v", entry)
	}
	views, _ = panel.dnsPendingPublicationViews(ctx)
	if len(views) != 1 || views[0].Reason != transport.DNSPeerPendingCatchingUp ||
		views[0].NextCheckAt != clock.Now().Add(30*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("view after attempt 1=%+v", views)
	}

	clock.Advance(20 * time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx) // not due
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 1 {
		t.Fatalf("re-checked before its backoff: %d", len(recoveries))
	}

	backupDown = false // the backup is back
	clock.Advance(10 * time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	syncs, recoveries := pendingAgentCalls(agent)
	if len(syncs) != 1 || len(recoveries) != 2 {
		t.Fatalf("syncs=%d recoveries=%d", len(syncs), len(recoveries))
	}
	for _, recovery := range recoveries {
		if recovery.MutationRequestID != syncs[0].MutationRequestID ||
			recovery.MutationOwnerID != syncs[0].MutationOwnerID ||
			recovery.Domain != "a-pending.example" || recovery.Qualifier != lease.Qualifier {
			t.Fatalf("re-check changed the exact operation identity: %+v want %+v", recovery, lease)
		}
	}
	state, err := readDNSZoneSyncState(ctx, panel.db.GetDB(), "a-pending.example")
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration || leaseCount(t, panel) != 0 {
		t.Fatalf("resolved state=%+v leases=%d", state, leaseCount(t, panel))
	}
	if _, ok := scheduler.entry(lease.RequestID); ok {
		t.Fatal("resolved publication kept its schedule entry")
	}
	if views, _ := panel.dnsPendingPublicationViews(ctx); len(views) != 0 {
		t.Fatalf("resolved publication still shown: %+v", views)
	}
}

// After a Panel restart the schedule is rebuilt from the durable lease alone.
func TestDNSPendingPublicationSurvivesPanelRestart(t *testing.T) {
	panel, agent, clock, _ := newPendingPublicationPanel(t, "restart-pending.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })
	if err := panel.syncZoneToDNS(context.Background(), "restart-pending.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	backupDown = false
	// Restart: no in-memory state survives.
	scheduler := useTestPendingScheduler(panel, clock)
	panel.reconcilePendingDNSPublicationsOnce(context.Background())
	if len(scheduler.entries) != 1 {
		t.Fatalf("restart did not rediscover the pending publication: %+v", scheduler.entries)
	}
	clock.Advance(dnsPendingPublicationFirstDelay)
	panel.reconcilePendingDNSPublicationsOnce(context.Background())
	syncs, recoveries := pendingAgentCalls(agent)
	if len(syncs) != 1 || len(recoveries) != 1 || recoveries[0].MutationRequestID != syncs[0].MutationRequestID ||
		recoveries[0].MutationOwnerID != syncs[0].MutationOwnerID || leaseCount(t, panel) != 0 {
		t.Fatalf("after restart syncs=%d recoveries=%+v leases=%d", len(syncs), recoveries, leaseCount(t, panel))
	}
}

// Causes that need a person are not retried in a loop; when the cause changes
// to a transient one, the re-check resumes.
func TestDNSPendingPublicationOwnerActionCauseIsNotRetried(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "owner-pending.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingEnrollmentRequired)
	code := transport.DNSPeerPendingEnrollmentRequired
	recoverOutcome(t, agent, func() bool { return true }, func() string { return code })
	if err := panel.syncZoneToDNS(context.Background(), "owner-pending.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		clock.Advance(time.Minute)
	}
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 1 {
		t.Fatalf("owner-action cause was retried %d time(s) after the publication's own attempt", len(recoveries)-1)
	}
	views, _ := panel.dnsPendingPublicationViews(ctx)
	if len(views) != 1 || views[0].Automatic || views[0].Reason != transport.DNSPeerPendingEnrollmentRequired || views[0].NextCheckAt != "" {
		t.Fatalf("owner-action view=%+v", views)
	}
	// The cause changes (for example the owner set up inspection access and
	// the Agent now reports a proof timeout): automatic re-checks resume.
	lease, _ := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "owner-pending.example")
	setPendingJobCode(t, agent, lease.RequestID, transport.DNSPeerPendingProofTimeout)
	code = transport.DNSPeerPendingProofTimeout
	clock.Advance(dnsPendingPublicationOwnerRecheck)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 2 {
		t.Fatalf("transient cause after change re-checked %d time(s)", len(recoveries)-1)
	}
	if entry, _ := scheduler.entry(lease.RequestID); !entry.Automatic || entry.Code != transport.DNSPeerPendingProofTimeout {
		t.Fatalf("entry after cause change=%+v", entry)
	}
	for _, retried := range []string{transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingProofTimeout, transport.DNSPeerPendingJournalUnknown, ""} {
		if !dnsPeerPendingRetriesAutomatically(retried) {
			t.Fatalf("%q must be re-checked automatically", retried)
		}
	}
	for _, person := range []string{
		transport.DNSPeerPendingEnrollmentRequired, transport.DNSPeerPendingEnrollmentChanged,
		transport.DNSPeerPendingInspectionUnknown, transport.DNSPeerPendingNativeUnknown,
		transport.DNSPeerPendingOwnerEditUnknown, transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckProducerCatalog),
		transport.DNSPeerPendingCatalogTransferRefused, transport.DNSPeerPendingProofInternal,
	} {
		if dnsPeerPendingRetriesAutomatically(person) {
			t.Fatalf("%q needs a person and must not be retried in a loop", person)
		}
	}
}

// Repeated failures that are not a reviewed pending result end automatic
// attempts (RESILIENCE-CONTRACT 4).
func TestDNSPendingPublicationRepeatedFailureEndsAutomaticAttempts(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "failing-pending.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	recoverOutcome(t, agent, func() bool { return true }, func() string { return transport.DNSPeerPendingCatchingUp })
	if err := panel.syncZoneToDNS(context.Background(), "failing-pending.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	// The Agent's receipt no longer matches the exact pending phase: every
	// exact reconciliation refuses it (a failure, not a pending result).
	var statusReads int
	agent.statusHook = func(_ string, job *ServiceOperationMutationJob) {
		statusReads++
		if job != nil {
			job.Phase = "commit/dns-zone-sync/v3/unexpected"
		}
	}
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		clock.Advance(dnsPendingPublicationMaxDelay)
		panel.reconcilePendingDNSPublicationsOnce(ctx)
	}
	_, recoveries := pendingAgentCalls(agent)
	lease, _ := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "failing-pending.example")
	entry, _ := scheduler.entry(lease.RequestID)
	if len(recoveries) != 0 || entry.Automatic || !entry.Stopped ||
		entry.Failures != dnsPendingPublicationFailureLimit || entry.Attempts != dnsPendingPublicationFailureLimit {
		t.Fatalf("entry after repeated failures=%+v (recoveries=%d)", entry, len(recoveries))
	}
	// After stopping, only the read-only status is looked at again.
	if views, _ := panel.dnsPendingPublicationViews(ctx); len(views) != 1 || views[0].Automatic {
		t.Fatalf("stopped view=%+v", views)
	}
	if statusReads == 0 {
		t.Fatal("no status was read")
	}
}

// A change to another zone first makes one bounded attempt at the pending
// publication; when it completes, the new change is published at once.
func TestDNSHeldChangeCompletesPendingPublicationThenProceeds(t *testing.T) {
	panel, agent, _, _ := newPendingPublicationPanel(t, "a-held.example", "b-held.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })
	if err := panel.syncZoneToDNS(context.Background(), "a-held.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	backupDown = false
	agent.syncResponseHook = nil // the backup is back: B publishes normally
	if err := panel.syncZoneToDNS(context.Background(), "b-held.example", false); err != nil {
		t.Fatalf("held change after completing the pending one: %v", err)
	}
	syncs, recoveries := pendingAgentCalls(agent)
	if len(recoveries) != 1 || recoveries[0].Domain != "a-held.example" ||
		recoveries[0].MutationRequestID != syncs[0].MutationRequestID {
		t.Fatalf("recoveries=%+v syncs=%+v", recoveries, syncs)
	}
	if len(syncs) != 2 || syncs[1].Domain != "b-held.example" {
		t.Fatalf("B was not published after A completed: %+v", syncs)
	}
	for _, zone := range []string{"a-held.example", "b-held.example"} {
		state, err := readDNSZoneSyncState(context.Background(), panel.db.GetDB(), zone)
		if err != nil || state.Status != "applied" {
			t.Fatalf("%s state=%+v err=%v", zone, state, err)
		}
	}
}

// When the pending publication stays pending, the new change is held with
// its cause, re-checks are not repeated for every change, and once the
// backup is back the reconciler completes the earlier publication and
// publishes the held change by itself.
func TestDNSHeldChangeIsHeldThenPublishedByItself(t *testing.T) {
	panel, agent, clock, _ := newPendingPublicationPanel(t, "a-wait.example", "b-wait.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingCatchingUp)
	backupDown := true
	recoverOutcome(t, agent, func() bool { return backupDown }, func() string { return transport.DNSPeerPendingCatchingUp })
	ctx := context.Background()
	if err := panel.syncZoneToDNS(ctx, "a-wait.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	err := panel.syncZoneToDNS(ctx, "b-wait.example", false)
	var held *dnsPublicationHeldError
	var publicationErr *dnsAgentPublicationError
	if !errors.As(err, &held) || !errors.As(err, &publicationErr) || held.Zone != "a-wait.example" ||
		held.Code != transport.DNSPeerPendingCatchingUp || !held.Automatic {
		t.Fatalf("held err=%v held=%+v", err, held)
	}
	body := held.apiBody()
	if body.Code != errCodeDNSPublicationHeld || body.Reason != transport.DNSPeerPendingCatchingUp ||
		body.Vars["zone"] != "a-wait.example" || !strings.Contains(body.Error, "Nobody needs to do anything") ||
		!strings.Contains(body.Error, "publishes this one too") {
		t.Fatalf("held body=%+v", body)
	}
	// Within the short memory a second held change does not re-check again.
	if err := panel.syncZoneToDNS(ctx, "b-wait.example", false); !errors.As(err, &held) {
		t.Fatalf("second held err=%v", err)
	}
	syncs, recoveries := pendingAgentCalls(agent)
	if len(recoveries) != 1 || len(syncs) != 1 {
		t.Fatalf("held path syncs=%d recoveries=%d", len(syncs), len(recoveries))
	}
	bState, _ := readDNSZoneSyncState(ctx, panel.db.GetDB(), "b-wait.example")
	if bState.Status != "pending" {
		t.Fatalf("held B state=%+v", bState)
	}

	backupDown = false
	agent.syncResponseHook = nil
	clock.Advance(dnsPendingPublicationMaxDelay)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	syncs, recoveries = pendingAgentCalls(agent)
	if len(recoveries) != 2 || len(syncs) != 2 || syncs[1].Domain != "b-wait.example" {
		t.Fatalf("after backup returned syncs=%+v recoveries=%d", syncs, len(recoveries))
	}
	for _, recovery := range recoveries {
		if recovery.MutationRequestID != syncs[0].MutationRequestID {
			t.Fatalf("re-check of A used another identity: %+v", recovery)
		}
	}
	for _, zone := range []string{"a-wait.example", "b-wait.example"} {
		state, err := readDNSZoneSyncState(ctx, panel.db.GetDB(), zone)
		if err != nil || state.Status != "applied" || state.AppliedGeneration != state.DesiredGeneration {
			t.Fatalf("%s state=%+v err=%v", zone, state, err)
		}
	}
	// Nothing further to publish: another pass starts no mutation.
	clock.Advance(dnsPendingPublicationMaxDelay)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if again, _ := pendingAgentCalls(agent); len(again) != 2 {
		t.Fatalf("an idle pass started %d publication(s)", len(again)-2)
	}
}

// A held change on a cause that needs a person is refused without an attempt.
func TestDNSHeldChangeBehindOwnerActionCauseMakesNoAttempt(t *testing.T) {
	panel, agent, _, _ := newPendingPublicationPanel(t, "a-owner.example", "b-owner.example")
	configurePendingWithCode(t, agent, transport.DNSPeerPendingEnrollmentRequired)
	recoverOutcome(t, agent, func() bool { return true }, func() string { return transport.DNSPeerPendingEnrollmentRequired })
	ctx := context.Background()
	if err := panel.syncZoneToDNS(ctx, "a-owner.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	err := panel.syncZoneToDNS(ctx, "b-owner.example", false)
	var held *dnsPublicationHeldError
	if !errors.As(err, &held) || held.Automatic || held.heldReason() != transport.DNSPeerPendingEnrollmentRequired {
		t.Fatalf("held err=%v held=%+v", err, held)
	}
	if body := held.apiBody(); body.Error != dnsPublicationHeldMessage || !strings.Contains(body.Error, "publishes this change by itself") {
		t.Fatalf("owner-action held body=%+v", body)
	}
	if _, recoveries := pendingAgentCalls(agent); len(recoveries) != 1 {
		t.Fatalf("owner-action cause re-checked %d time(s) for the held change", len(recoveries)-1)
	}
}

// The HTTP answer of a held change and of the sync-all conflict path.
func TestDNSPublicationHeldResponses(t *testing.T) {
	held := &dnsPublicationHeldError{Zone: "a.example", Code: transport.DNSPeerPendingCatchingUp, Automatic: true}
	w := httptest.NewRecorder()
	writeDNSPublicationHeld(w, held)
	var body apiErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusConflict || body.Code != errCodeDNSPublicationHeld ||
		body.Reason != transport.DNSPeerPendingCatchingUp || body.Vars["zone"] != "a.example" ||
		body.Action != "/dns-infrastructure" {
		t.Fatalf("held response %d %+v", w.Code, body)
	}
	conflict := &dnsPublicationError{Result: dnsSyncAllResult{Attempted: 2, Failures: []dnsZoneSyncFailure{
		{Zone: "b.example", Err: &dnsAgentPublicationError{Err: held}},
		{Zone: "c.example", Err: &dnsAgentPublicationError{Err: errors.New(dnsAgentPendingRecoveryRefusal)}},
	}}}
	w = httptest.NewRecorder()
	if !writeDNSPublicationConflict(w, conflict, "generic") {
		t.Fatal("conflict not handled")
	}
	if !strings.Contains(w.Body.String(), `"reason":"dns_peer_catching_up"`) {
		t.Fatalf("sync-all held body=%s", w.Body.String())
	}
	pairWait := &dnsPublicationHeldError{Automatic: true}
	if pairWait.heldReason() != transport.DNSPeerPendingCatchingUp || pairWait.apiBody().Vars != nil ||
		!strings.Contains(pairWait.englishMessage(), "an earlier DNS change is waiting for the backup DNS server to catch up") {
		t.Fatalf("pair-wait held=%+v", pairWait.apiBody())
	}
	unknown := &dnsPublicationHeldError{Zone: "a.example", Automatic: true}
	if unknown.heldReason() != dnsPublicationHeldPendingVerification {
		t.Fatalf("unclassified automatic reason=%q", unknown.heldReason())
	}
	reason, _, message, ok := dnsPeerPendingGuidance(transport.DNSPeerPendingCatchingUp)
	if !ok || reason != transport.DNSPeerPendingCatchingUp || !strings.Contains(message, "has not caught up") ||
		!strings.Contains(message, "TCP port 53") || strings.Contains(message, "Nothing retries by itself") {
		t.Fatalf("catching-up guidance %q %q %v", reason, message, ok)
	}
	for _, code := range []string{transport.DNSPeerPendingProofTimeout, transport.DNSPeerPendingJournalUnknown} {
		if _, _, message, _ := dnsPeerPendingGuidance(code); strings.Contains(message, "Nothing retries by itself") ||
			!strings.Contains(message, "re-checks this same publication by itself") {
			t.Fatalf("%s guidance still says nothing retries: %q", code, message)
		}
	}
}
