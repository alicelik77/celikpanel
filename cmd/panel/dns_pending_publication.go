package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Self-recovery of pending zone-sync V3 publications (2026-10-30).
//
// On a DNS pair a publication whose backup did not catch up within the Agent's
// propagation check is left pending: the change is in effect on this server,
// the Agent keeps the exact accepted operation (request and owner identity,
// immutable host receipt) and refuses every later DNS change until that
// operation is completed. Before this file nothing re-checked it until the
// same zone was published again or the Panel restarted.
//
// The Panel now re-checks every pending publication by itself when its cause
// is transient, and only through the existing exact recovery path
// (reconcileDNSZoneEngineLease -> recoverPendingDNSZoneV3 ->
// Agent.RecoverDNSZoneV3). That path resumes the same accepted operation: the
// same request and owner identity, resumed only from the Agent's exact pending
// ledger receipt, re-verifying the already committed host receipt. It never
// prepares a new plan, advances a serial or re-applies records, so it is not a
// second mutation. Single flight: every attempt holds serviceMutationMu and
// dnsPublicationMu (the same order as every publication) and the Agent admits
// one resume of one exact pending job at a time.
//
// Causes that need a person (dnsPeerPendingRetriesAutomatically) are never
// retried in a loop; they keep their guidance until the owner retries or the
// cause changes. Repeated failures that are not a reviewed pending result end
// automatic attempts (RESILIENCE-CONTRACT invariant 4).
//
// Bekleyen V3 yayını, nedeni geçiciyse panel tarafından aynı kabul edilmiş
// işlemle, sınırlı aralıklarla kendiliğinden yeniden denetlenir; kişi gerektiren
// nedenler döngüde denenmez.

// Variables only so tests can shorten them.
var (
	dnsPendingPublicationFirstDelay    = 15 * time.Second
	dnsPendingPublicationMaxDelay      = 5 * time.Minute
	dnsPendingPublicationScanInterval  = 5 * time.Second
	dnsPendingPublicationOwnerRecheck  = time.Minute
	dnsPendingPublicationHeldDebounce  = 10 * time.Second
	dnsPendingPublicationAttemptLimit  = 45 * time.Second
	dnsPendingPublicationHeldAttempt   = 25 * time.Second
	dnsPendingPublicationFailureLimit  = 6
	dnsPendingPublicationRepublishWait = 30 * time.Second
)

type dnsPendingPublicationEntry struct {
	Lease dnsZoneEngineLease
	// Code is the last reviewed pending code read from the exact Agent job;
	// "" while unknown or unclassified.
	Code      string
	Observed  bool // the exact job's status was read at least once
	Automatic bool
	// Stopped: automatic attempts ended after repeated failures that were
	// not a reviewed pending result. Cleared when the observed code changes.
	Stopped     bool
	Attempts    int
	Failures    int
	NextAt      time.Time
	LastAttempt time.Time
	// LastPending: the last attempt ended with the same exact operation still
	// pending (as opposed to an error or no attempt yet).
	LastPending bool
}

type dnsPendingPublicationScheduler struct {
	mu      sync.Mutex
	entries map[string]*dnsPendingPublicationEntry // by lease request ID
	// republishArmed is set when a pending publication was resolved (or a
	// change was held behind one): zones saved while held are then published
	// by the same reconciler once no exact publication is pending.
	republishArmed    bool
	republishNextAt   time.Time
	republishAttempts int
	// republishSkipped keeps zones whose republication failed with a
	// non-publication error at the desired generation that failed, so a
	// deterministic error is not repeated until the zone changes.
	republishSkipped map[string]int64
	now              func() time.Time
	jitter           func(time.Duration) time.Duration
	wake             chan struct{}
}

func newDNSPendingPublicationScheduler() *dnsPendingPublicationScheduler {
	return &dnsPendingPublicationScheduler{
		entries:          make(map[string]*dnsPendingPublicationEntry),
		republishSkipped: make(map[string]int64),
		now:              time.Now,
		jitter: func(d time.Duration) time.Duration {
			// ±20 % so several panels (or zones) do not re-check in step.
			spread := int64(d) / 5
			if spread <= 0 {
				return d
			}
			return d - time.Duration(spread) + time.Duration(rand.Int63n(2*spread+1))
		},
		wake: make(chan struct{}, 1),
	}
}

func (p *Panel) dnsPendingPublications() *dnsPendingPublicationScheduler {
	p.dnsPendingPublicationsOnce.Do(func() {
		if p.dnsPendingPublicationsState == nil {
			p.dnsPendingPublicationsState = newDNSPendingPublicationScheduler()
		}
	})
	return p.dnsPendingPublicationsState
}

// backoff is the wait after the given number of attempts: the first re-check
// comes 15 s after the publication went pending, then 30 s, 60 s, ... up to
// 5 min, with jitter.
func (s *dnsPendingPublicationScheduler) backoff(attempts int) time.Duration {
	delay := dnsPendingPublicationFirstDelay
	for i := 0; i < attempts && delay < dnsPendingPublicationMaxDelay; i++ {
		delay *= 2
	}
	if delay > dnsPendingPublicationMaxDelay {
		delay = dnsPendingPublicationMaxDelay
	}
	return s.jitter(delay)
}

func (s *dnsPendingPublicationScheduler) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *dnsPendingPublicationScheduler) armRepublish() {
	s.mu.Lock()
	s.republishArmed = true
	s.mu.Unlock()
}

// syncLeases makes the entries exactly the durable leases. A lease that
// disappeared was completed (by this reconciler, the owner's retry, startup or
// a later publication of the same zone): zones held behind it may now be
// published.
func (s *dnsPendingPublicationScheduler) syncLeases(leases []dnsZoneEngineLease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	present := make(map[string]bool, len(leases))
	for _, lease := range leases {
		present[lease.RequestID] = true
		if entry, ok := s.entries[lease.RequestID]; ok && entry.Lease == lease {
			continue
		}
		s.entries[lease.RequestID] = &dnsPendingPublicationEntry{
			Lease: lease, Automatic: true,
			NextAt: now.Add(s.jitter(dnsPendingPublicationFirstDelay)),
		}
	}
	for requestID := range s.entries {
		if !present[requestID] {
			delete(s.entries, requestID)
			s.republishArmed = true
		}
	}
}

func (s *dnsPendingPublicationScheduler) entry(requestID string) (dnsPendingPublicationEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[requestID]
	if !ok {
		return dnsPendingPublicationEntry{}, false
	}
	return *entry, true
}

// observeCode records the code read from the exact pending Agent job.
func (s *dnsPendingPublicationScheduler) observeCode(lease dnsZoneEngineLease, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[lease.RequestID]
	if entry == nil {
		entry = &dnsPendingPublicationEntry{Lease: lease, NextAt: s.now().Add(s.jitter(dnsPendingPublicationFirstDelay))}
		s.entries[lease.RequestID] = entry
	}
	if entry.Code != code {
		entry.Stopped = false
		entry.Failures = 0
	}
	entry.Observed = true
	entry.Code = code
	entry.Automatic = dnsPeerPendingRetriesAutomatically(code) && !entry.Stopped
}

// dnsPendingPublicationOutcome classifies one exact recovery attempt.
type dnsPendingPublicationOutcome int

const (
	dnsPendingPublicationResolved dnsPendingPublicationOutcome = iota
	dnsPendingPublicationStillPending
	dnsPendingPublicationFailed
)

func classifyDNSPendingPublicationAttempt(err error) (dnsPendingPublicationOutcome, string, bool) {
	if err == nil {
		return dnsPendingPublicationResolved, "", false
	}
	if errors.Is(err, errDNSZoneV3PropagationDeferred) {
		var pending *dnsZoneV3PropagationPendingError
		if errors.As(err, &pending) && pending.Exact {
			return dnsPendingPublicationStillPending, pending.Code, true
		}
		return dnsPendingPublicationStillPending, "", false
	}
	return dnsPendingPublicationFailed, "", false
}

func (s *dnsPendingPublicationScheduler) recordAttempt(
	lease dnsZoneEngineLease,
	outcome dnsPendingPublicationOutcome,
	code string,
	codeKnown bool,
) dnsPendingPublicationEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	entry := s.entries[lease.RequestID]
	if outcome == dnsPendingPublicationResolved {
		delete(s.entries, lease.RequestID)
		s.republishArmed = true
		return dnsPendingPublicationEntry{Lease: lease}
	}
	if entry == nil {
		entry = &dnsPendingPublicationEntry{Lease: lease, Automatic: true}
		s.entries[lease.RequestID] = entry
	}
	entry.Attempts++
	entry.LastAttempt = now
	switch outcome {
	case dnsPendingPublicationStillPending:
		entry.LastPending = true
		entry.Failures = 0
		if codeKnown {
			if entry.Code != code {
				entry.Stopped = false
			}
			entry.Code = code
		}
		entry.Automatic = dnsPeerPendingRetriesAutomatically(entry.Code) && !entry.Stopped
	default:
		entry.LastPending = false
		entry.Failures++
		if entry.Failures >= dnsPendingPublicationFailureLimit {
			entry.Stopped = true
			entry.Automatic = false
		}
	}
	if entry.Automatic {
		entry.NextAt = now.Add(s.backoff(entry.Attempts))
	} else {
		entry.NextAt = now.Add(dnsPendingPublicationOwnerRecheck)
	}
	return *entry
}

func (s *dnsPendingPublicationScheduler) markObserved(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[requestID]; entry != nil {
		entry.Observed = true
	}
}

func (s *dnsPendingPublicationScheduler) postpone(requestID string, delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[requestID]; entry != nil {
		entry.NextAt = s.now().Add(delay)
	}
}

func listDNSZoneEngineLeases(ctx context.Context, db *sql.DB) ([]dnsZoneEngineLease, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT zone_name, engine, engine_epoch, request_id, owner_id,
		       desired_generation, desired_action, desired_zone_type,
		       qualifier, expires_at
		FROM dns_zone_engine_leases ORDER BY zone_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leases []dnsZoneEngineLease
	for rows.Next() {
		var lease dnsZoneEngineLease
		if err := rows.Scan(
			&lease.ZoneName, &lease.Engine, &lease.EngineEpoch,
			&lease.RequestID, &lease.OwnerID, &lease.DesiredGeneration,
			&lease.DesiredAction, &lease.DesiredZoneType,
			&lease.Qualifier, &lease.ExpiresAt,
		); err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

// readPendingDNSPublicationCode is a read-only look at the exact Agent job of
// a lease: its reviewed pending code when the job is the exact pending one.
func (p *Panel) readPendingDNSPublicationCode(
	ctx context.Context,
	lease dnsZoneEngineLease,
) (code string, pending bool, err error) {
	code, pending, _, err = p.readPendingDNSPublicationStatus(ctx, lease)
	return code, pending, err
}

// readPendingDNSPublicationStatus also reports an exact job that is running
// (a recovery attempt already in flight, for example the owner's retry).
func (p *Panel) readPendingDNSPublicationStatus(
	ctx context.Context,
	lease dnsZoneEngineLease,
) (code string, pending bool, active bool, err error) {
	statusCtx, cancel := context.WithTimeout(ctx, panelMutationFinishTimeout)
	defer cancel()
	job, err := p.statusAgentMutation(statusCtx, lease.RequestID)
	if err != nil {
		return "", false, false, err
	}
	if job != nil && agentMutationActive(job.Status) &&
		validateAgentMutationIdentity(job, lease.identity()) == nil {
		return "", false, true, nil
	}
	if job == nil || validateDNSZoneSyncV3PendingJob(job, lease.identity()) != nil {
		return "", false, false, nil
	}
	return dnsZoneV3PendingCodeFromJob(job), true, false, nil
}

// exactDNSPublicationSucceeded is a read-only look: the exact Agent job of the
// lease has a verified terminal success receipt.
func (p *Panel) exactDNSPublicationSucceeded(
	ctx context.Context,
	lease dnsZoneEngineLease,
) (bool, error) {
	statusCtx, cancel := context.WithTimeout(ctx, panelMutationFinishTimeout)
	defer cancel()
	job, err := p.statusAgentMutation(statusCtx, lease.RequestID)
	if err != nil || job == nil || job.Status != agentMutationSucceeded {
		return false, err
	}
	identity := lease.identity()
	if validateAgentMutationIdentity(job, identity) != nil ||
		validateAgentMutationSucceededReceipt(job, identity) != nil {
		return false, nil
	}
	return true, nil
}

// resumePendingDNSPublicationLocked makes one exact recovery attempt of the
// lease. The caller holds serviceMutationMu and dnsPublicationMu. A lease that
// changed since it was read is not touched.
func (p *Panel) resumePendingDNSPublicationLocked(
	ctx context.Context,
	lease dnsZoneEngineLease,
	waitActive bool,
) (done bool, stale bool, err error) {
	current, readErr := readDNSZoneEngineLease(ctx, p.db.GetDB(), lease.ZoneName)
	if errors.Is(readErr, sql.ErrNoRows) {
		return false, true, nil
	}
	if readErr != nil {
		return false, false, fmt.Errorf("re-read pending DNS publication lease: %w", readErr)
	}
	if current != lease {
		return false, true, nil
	}
	done, err = p.reconcileDNSZoneEngineLease(ctx, lease, waitActive)
	return done, false, err
}

// reconcilePendingDNSPublicationsOnce is one pass of the background
// reconciler: it re-checks every due pending publication whose cause is
// transient, then publishes zones held behind a completed one.
func (p *Panel) reconcilePendingDNSPublicationsOnce(ctx context.Context) {
	scheduler := p.dnsPendingPublications()
	leases, err := listDNSZoneEngineLeases(ctx, p.db.GetDB())
	if err != nil {
		log.Printf("pending DNS publications: list exact leases: %v", err)
		return
	}
	scheduler.syncLeases(leases)
	for _, lease := range leases {
		entry, ok := scheduler.entry(lease.RequestID)
		if ok && !entry.Observed {
			// Read only, once at discovery: the DNS card names the reason
			// before the first re-check is due.
			if code, pending, err := p.readPendingDNSPublicationCode(ctx, lease); err == nil && pending {
				scheduler.observeCode(lease, code)
			} else {
				scheduler.markObserved(lease.RequestID)
			}
			entry, ok = scheduler.entry(lease.RequestID)
		}
		if !ok || scheduler.now().Before(entry.NextAt) {
			continue
		}
		code, pending, statusErr := p.readPendingDNSPublicationCode(ctx, lease)
		if statusErr != nil {
			// The Agent could not be asked: nothing is attempted and this is
			// not a failure of the operation; it is asked about again later.
			scheduler.postpone(lease.RequestID, scheduler.backoff(entry.Attempts+1))
			log.Printf("pending DNS publication %s: read exact status: %v", lease.ZoneName, statusErr)
			continue
		}
		if pending {
			scheduler.observeCode(lease, code)
		}
		if refreshed, _ := scheduler.entry(lease.RequestID); !refreshed.Automatic {
			if succeeded, err := p.exactDNSPublicationSucceeded(ctx, lease); err == nil && succeeded {
				// The Agent itself finished this exact operation (for example
				// its startup recovery after the owner restarted it). Recording
				// that receipt starts nothing on the server.
				// Agent bu islemi kendisi bitirdi; makbuzu kaydetmek sunucuda
				// hicbir sey baslatmaz.
				p.attemptPendingDNSPublication(ctx, lease)
				continue
			}
			// A person must act (or automatic attempts ended): keep the
			// guidance, look again later for a changed cause.
			scheduler.postpone(lease.RequestID, dnsPendingPublicationOwnerRecheck)
			continue
		}
		// Not the exact pending job (succeeded, failed, never begun or still
		// active): the same exact reconciliation finalizes it.
		p.attemptPendingDNSPublication(ctx, lease)
	}
	p.republishHeldDNSZones(ctx)
}

func (p *Panel) attemptPendingDNSPublication(ctx context.Context, lease dnsZoneEngineLease) {
	scheduler := p.dnsPendingPublications()
	if !p.serviceMutationMu.TryLock() {
		// Another host mutation runs; this is not an attempt.
		scheduler.postpone(lease.RequestID, dnsPendingPublicationScanInterval)
		return
	}
	defer p.serviceMutationMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()
	attemptCtx, cancel := context.WithTimeout(ctx, dnsPendingPublicationAttemptLimit)
	defer cancel()
	done, stale, err := p.resumePendingDNSPublicationLocked(attemptCtx, lease, true)
	if stale {
		scheduler.recordAttempt(lease, dnsPendingPublicationResolved, "", false)
		return
	}
	outcome, code, known := classifyDNSPendingPublicationAttempt(err)
	entry := scheduler.recordAttempt(lease, outcome, code, known)
	switch outcome {
	case dnsPendingPublicationResolved:
		log.Printf("pending DNS publication %s completed by its automatic re-check (exact=%t)", lease.ZoneName, done)
		if !done {
			// The exact operation is complete; the zone's desired state
			// moved on while it was pending (or the operation never began).
			publishCtx, cancelPublish := context.WithTimeout(ctx, dnsPendingPublicationAttemptLimit)
			p.publishZoneAfterPendingLocked(publishCtx, lease)
			cancelPublish()
		}
	case dnsPendingPublicationStillPending:
		log.Printf("pending DNS publication %s still waits (reason=%s); next automatic re-check in %s",
			lease.ZoneName, dnsPendingReasonForLog(entry.Code), entry.NextAt.Sub(scheduler.now()).Round(time.Second))
	default:
		log.Printf("pending DNS publication %s: automatic re-check failed (%d in a row): %v",
			lease.ZoneName, entry.Failures, err)
	}
}

func dnsPendingReasonForLog(code string) string {
	if code == "" {
		return "unclassified"
	}
	return code
}

// publishZoneAfterPendingLocked publishes a zone whose exact pending
// operation completed while its desired state had advanced. A verified
// failure or a new pending result stays recorded by the publication itself.
func (p *Panel) publishZoneAfterPendingLocked(ctx context.Context, lease dnsZoneEngineLease) {
	state, err := readDNSZoneSyncState(ctx, p.db.GetDB(), lease.ZoneName)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("pending DNS publication %s: read follow-up state: %v", lease.ZoneName, err)
		return
	}
	if state.Status != "pending" {
		return
	}
	if err := p.syncZoneToDNSLocked(ctx, lease.ZoneName, state.DesiredAction == "delete"); err != nil {
		log.Printf("pending DNS publication %s: follow-up publication: %v", lease.ZoneName, err)
	}
}

// heldDNSZonesForRepublish lists zones saved but not published because an
// earlier publication was pending: no lease of either version, status pending
// (or an Agent refusal recorded as error), unchanged for a short while so an
// in-flight request publishes its own change.
func (p *Panel) heldDNSZonesForRepublish(ctx context.Context) ([]dnsZoneSyncState, error) {
	settle := int(dnsPendingPublicationRepublishWait / time.Second)
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT zone_name FROM dns_zone_sync_state s
		WHERE s.lease_request_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM dns_zone_engine_leases l WHERE l.zone_name = s.zone_name)
		  AND (s.status = 'pending'
		       OR (s.status = 'error' AND s.last_error LIKE ?))
		  AND s.updated_at <= datetime('now', ?)
		ORDER BY s.zone_name`,
		"%"+dnsAgentPendingRecoveryRefusal+"%", fmt.Sprintf("-%d seconds", settle),
	)
	if err != nil {
		return nil, err
	}
	var zones []string
	for rows.Next() {
		var zone string
		if err := rows.Scan(&zone); err != nil {
			rows.Close()
			return nil, err
		}
		zones = append(zones, zone)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	states := make([]dnsZoneSyncState, 0, len(zones))
	for _, zone := range zones {
		state, err := readDNSZoneSyncState(ctx, p.db.GetDB(), zone)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

// republishHeldDNSZones publishes zones held behind a pending publication once
// none is pending any more. It runs only after a pending publication was
// resolved or a change was held, and only while the publisher is ready; a
// zone's own verified failure or new pending result stays recorded.
func (p *Panel) republishHeldDNSZones(ctx context.Context) {
	scheduler := p.dnsPendingPublications()
	scheduler.mu.Lock()
	armed := scheduler.republishArmed
	due := !scheduler.now().Before(scheduler.republishNextAt)
	pending := len(scheduler.entries)
	scheduler.mu.Unlock()
	if !armed || !due || pending != 0 {
		return
	}
	retryLater := func() {
		scheduler.mu.Lock()
		scheduler.republishAttempts++
		scheduler.republishNextAt = scheduler.now().Add(scheduler.backoff(scheduler.republishAttempts))
		scheduler.mu.Unlock()
	}
	zones, err := p.heldDNSZonesForRepublish(ctx)
	if err != nil {
		log.Printf("held DNS publications: list: %v", err)
		retryLater()
		return
	}
	scheduler.mu.Lock()
	var candidates []dnsZoneSyncState
	for _, state := range zones {
		if generation, skipped := scheduler.republishSkipped[state.ZoneName]; skipped && generation == state.DesiredGeneration {
			continue
		}
		candidates = append(candidates, state)
	}
	scheduler.mu.Unlock()
	if len(candidates) == 0 {
		// Nothing held (or only zones that changed too recently): stay armed
		// while a recent change may still settle, otherwise disarm.
		recent, recentErr := p.recentlyChangedPendingDNSZones(ctx)
		scheduler.mu.Lock()
		if recentErr == nil && !recent {
			scheduler.republishArmed = false
			scheduler.republishAttempts = 0
		} else {
			scheduler.republishNextAt = scheduler.now().Add(dnsPendingPublicationFirstDelay)
		}
		scheduler.mu.Unlock()
		return
	}
	if _, ready, readyErr := p.activeDNSPublisher(ctx); readyErr != nil || !ready {
		retryLater()
		return
	}
	if !p.serviceMutationMu.TryLock() {
		return
	}
	defer p.serviceMutationMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ZoneName < candidates[j].ZoneName })
	for _, state := range candidates {
		if leases, err := listDNSZoneEngineLeases(ctx, p.db.GetDB()); err != nil || len(leases) != 0 {
			// A publication became pending again: the exact re-check above
			// owns it; held zones wait for it.
			retryLater()
			return
		}
		publishCtx, cancel := context.WithTimeout(ctx, dnsPendingPublicationAttemptLimit)
		err := p.syncZoneToDNSLocked(publishCtx, state.ZoneName, state.DesiredAction == "delete")
		cancel()
		if err == nil {
			log.Printf("held DNS change for %s published after the earlier publication completed", state.ZoneName)
			continue
		}
		var publicationErr *dnsAgentPublicationError
		if errors.As(err, &publicationErr) {
			// Recorded by the publication itself (verified failure, pending,
			// or held again): nothing to repeat here.
			log.Printf("held DNS change for %s: publication did not complete: %v", state.ZoneName, err)
			if errors.Is(err, errDNSZoneV3PropagationDeferred) {
				retryLater()
				return
			}
			continue
		}
		log.Printf("held DNS change for %s: not republished automatically: %v", state.ZoneName, err)
		scheduler.mu.Lock()
		scheduler.republishSkipped[state.ZoneName] = state.DesiredGeneration
		scheduler.mu.Unlock()
	}
	scheduler.mu.Lock()
	scheduler.republishAttempts = 0
	scheduler.mu.Unlock()
}

func (p *Panel) recentlyChangedPendingDNSZones(ctx context.Context) (bool, error) {
	settle := int(dnsPendingPublicationRepublishWait / time.Second)
	var count int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT count(*) FROM dns_zone_sync_state
		WHERE status = 'pending' AND updated_at > datetime('now', ?)`,
		fmt.Sprintf("-%d seconds", settle),
	).Scan(&count)
	return count > 0, err
}

// startDNSPendingPublicationReconciler runs the reconciler until ctx ends.
// It starts after startup recovery and only reads until a lease is due.
func (p *Panel) startDNSPendingPublicationReconciler(ctx context.Context) {
	scheduler := p.dnsPendingPublications()
	go func() {
		timer := time.NewTimer(dnsPendingPublicationScanInterval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			case <-scheduler.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			p.reconcilePendingDNSPublicationsOnce(ctx)
			timer.Reset(dnsPendingPublicationScanInterval)
		}
	}()
}

// dnsPublicationHeldError: a publication was not started because an earlier
// exact publication is pending (or, on a paired main, the backup has not yet
// confirmed this server's current zone list). The change is saved; it is
// published by the reconciler once the earlier one completes.
type dnsPublicationHeldError struct {
	Zone      string // zone of the earlier pending publication; "" for the pair wait alone
	Code      string // its reviewed pending code; "" unknown
	Automatic bool
}

func (e *dnsPublicationHeldError) Error() string {
	if e.Zone == "" {
		return "DNS publication is held until the backup DNS server confirms the current zone list"
	}
	return "DNS publication is held behind the pending publication of " + e.Zone
}

// heldReason is the reviewed reason token sent with DNS_PUBLICATION_HELD.
func (e *dnsPublicationHeldError) heldReason() string {
	if e.Zone == "" {
		return transport.DNSPeerPendingCatchingUp
	}
	if reason, _, ok := transport.SplitDNSPeerPendingCode(e.Code); ok {
		return reason
	}
	if e.Automatic {
		return dnsPublicationHeldPendingVerification
	}
	return ""
}

// dnsPublicationHeldPendingVerification names an earlier publication whose
// result is not classified yet but which is re-checked automatically.
const dnsPublicationHeldPendingVerification = "pending_verification"

func (e *dnsPublicationHeldError) englishMessage() string {
	earlier := "an earlier DNS change"
	if e.Zone != "" {
		earlier = "an earlier DNS change to " + e.Zone
	}
	switch {
	case e.Zone == "" || e.heldReason() == transport.DNSPeerPendingCatchingUp:
		return "The change is saved on this server but not published yet: " + earlier + " is waiting for the backup DNS server to catch up, and DNS changes are published one at a time. Nobody needs to do anything: CelikPanel re-checks the earlier change by itself and then publishes this one too; Server → DNS and peer servers shows the next check. DNS keeps answering with the published records meanwhile. If this lasts, the server administrator checks that the backup DNS server is running and reachable from this server on TCP port 53."
	case e.Automatic:
		return "The change is saved on this server but not published yet: " + earlier + " is still being verified on the backup DNS server, and DNS changes are published one at a time. Nobody needs to do anything now: CelikPanel re-checks the earlier change by itself and then publishes this one too; Server → DNS and peer servers shows the next check, and that domain's page says more if it lasts. DNS keeps answering with the published records meanwhile."
	default:
		return dnsPublicationHeldMessage
	}
}

func (e *dnsPublicationHeldError) apiBody() apiErrorBody {
	body := apiErrorBody{
		Error: e.englishMessage(), Code: errCodeDNSPublicationHeld,
		Action: "/dns-infrastructure", Reason: e.heldReason(),
	}
	if e.Zone != "" {
		body.Vars = map[string]string{"zone": e.Zone}
	}
	return body
}

// settleOtherPendingDNSPublicationsLocked runs before a zone is published. The
// Agent publishes one exact DNS change at a time; when another zone's
// publication is pending, one bounded attempt completes it first (the same
// exact recovery as the reconciler, at most once per short interval and never
// for a cause that needs a person). If it stays pending, the new change is
// held: saved, published by the reconciler later. Caller holds
// serviceMutationMu and dnsPublicationMu.
func (p *Panel) settleOtherPendingDNSPublicationsLocked(ctx context.Context, zone string) error {
	leases, err := listDNSZoneEngineLeases(ctx, p.db.GetDB())
	if err != nil {
		return fmt.Errorf("list pending DNS publications: %w", err)
	}
	scheduler := p.dnsPendingPublications()
	for _, lease := range leases {
		if lease.ZoneName == zone {
			continue
		}
		if held := p.attemptHeldPendingPublicationLocked(ctx, scheduler, lease); held != nil {
			scheduler.armRepublish()
			return &dnsAgentPublicationError{Err: held}
		}
	}
	return nil
}

func (p *Panel) attemptHeldPendingPublicationLocked(
	ctx context.Context,
	scheduler *dnsPendingPublicationScheduler,
	lease dnsZoneEngineLease,
) *dnsPublicationHeldError {
	heldFrom := func(entry dnsPendingPublicationEntry) *dnsPublicationHeldError {
		automatic := entry.Automatic
		if entry.Lease.RequestID == "" {
			// Not scheduled yet (just after a restart): the reconciler
			// picks it up by itself.
			automatic = true
		}
		return &dnsPublicationHeldError{
			Zone: lease.ZoneName, Code: entry.Code, Automatic: automatic,
		}
	}
	code, pending, active, statusErr := p.readPendingDNSPublicationStatus(ctx, lease)
	if statusErr != nil || active {
		// Unreadable, or its exact recovery is already running: hold without
		// starting another attempt.
		entry, _ := scheduler.entry(lease.RequestID)
		return heldFrom(entry)
	}
	if pending {
		scheduler.observeCode(lease, code)
		entry, _ := scheduler.entry(lease.RequestID)
		if !entry.Automatic {
			return heldFrom(entry)
		}
		if entry.LastPending && !entry.LastAttempt.IsZero() &&
			scheduler.now().Sub(entry.LastAttempt) < dnsPendingPublicationHeldDebounce {
			// Just re-checked and still pending: do not re-check again for
			// every change that arrives (single flight with a short memory).
			return heldFrom(entry)
		}
	}
	attemptCtx, cancel := context.WithTimeout(ctx, dnsPendingPublicationHeldAttempt)
	defer cancel()
	_, stale, err := p.resumePendingDNSPublicationLocked(attemptCtx, lease, false)
	if stale {
		scheduler.recordAttempt(lease, dnsPendingPublicationResolved, "", false)
		return nil
	}
	outcome, observed, known := classifyDNSPendingPublicationAttempt(err)
	entry := scheduler.recordAttempt(lease, outcome, observed, known)
	if outcome == dnsPendingPublicationResolved {
		// Completed. Its own newer desired state (if any) is published by the
		// reconciler; the change that arrived proceeds now.
		log.Printf("pending DNS publication %s completed before a held change", lease.ZoneName)
		return nil
	}
	log.Printf("DNS change held behind pending publication %s: %v", lease.ZoneName, err)
	return heldFrom(entry)
}

// dnsPendingPublicationView is the pending state the DNS engine card shows:
// which zone waits, the reviewed reason, whether CelikPanel re-checks it by
// itself and when next.
type dnsPendingPublicationView struct {
	Zone        string `json:"zone"`
	Reason      string `json:"reason,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Automatic   bool   `json:"automatic"`
	NextCheckAt string `json:"next_check_at,omitempty"`
}

const dnsPendingPublicationViewLimit = 5

func (p *Panel) dnsPendingPublicationViews(ctx context.Context) ([]dnsPendingPublicationView, error) {
	leases, err := listDNSZoneEngineLeases(ctx, p.db.GetDB())
	if err != nil || len(leases) == 0 {
		return nil, err
	}
	scheduler := p.dnsPendingPublications()
	views := make([]dnsPendingPublicationView, 0, len(leases))
	for _, lease := range leases {
		if len(views) == dnsPendingPublicationViewLimit {
			break
		}
		view := dnsPendingPublicationView{Zone: lease.ZoneName, Automatic: true}
		if entry, ok := scheduler.entry(lease.RequestID); ok {
			if reason, detail, ok := transport.SplitDNSPeerPendingCode(entry.Code); ok {
				view.Reason, view.Detail = reason, detail
			}
			view.Automatic = entry.Automatic
			if entry.Automatic && !entry.NextAt.IsZero() {
				view.NextCheckAt = entry.NextAt.UTC().Format(time.RFC3339)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

// dnsHeldStatusIsPairWait reports a primary publisher that is not ready only
// because its backup has not confirmed the current zone list.
func dnsHeldStatusIsPairWait(publisher dnsPublisherIdentity, ready bool) bool {
	return !ready && publisher.PairRole == transport.DNSPairRolePrimary &&
		transport.ValidDNSEngine(publisher.Engine) && publisher.Epoch >= 1
}
