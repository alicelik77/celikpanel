package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Native peer proof step budget (2026-10-01, pair 6 P6-1).
//
// Until this change the completion wave and its one native peer proof shared
// one 15 s context (dnsPairProofLimit). Writing the challenge, the SSH
// exchange and the post-inspection probes used most of it, and consume-once
// then ran its current-evidence rechecks under an expired context: a
// positive, authenticated answer was discarded as dns_peer_journal_unknown.
//
// The wave's own DNS answer probes keep dnsPairProofLimit. The native proof
// now runs from the request context, not the wave's, and each step has its
// own bound:
//
//	step             measured (pair 5/6, batch 11)   bound (alpha.133: dnsPeerProofSteps)
//	prepare          about 1-3 s                     8 s (10 s)
//	challenge_write  3.5-4.1 s                       12 s
//	exchange         2.5-4 s                         10 s (dnspeertransport.ExchangeLimit)
//	post_inspection  about 2-3 s                     10 s
//	consume          3 rechecks + write, 3-4 s       20 s (18 s), fresh context
//
// Total wall bound 60 s. Once a challenge is minted, every step before
// acceptance also ends expiryReserve before the challenge expires, so that
// consume-once still reaches its own expiry check (the challenge lives 30 s).
// A step that runs out of time before the peer's answer was accepted is
// dns_peer_proof_timeout, with the side that ran out of time
// (dnsPeerProofStep.timeoutSide). After dnspeerproof.Verify accepted the answer,
// consume-once and the final recheck run under a fresh context derived from
// context.WithoutCancel of the request context: a positive answer is never
// discarded because a clock of an earlier step ran out.
type dnsPeerProofStep string

const (
	dnsPeerProofStepPrepare        dnsPeerProofStep = "prepare"
	dnsPeerProofStepChallengeWrite dnsPeerProofStep = "challenge_write"
	dnsPeerProofStepExchange       dnsPeerProofStep = "exchange"
	dnsPeerProofStepPostInspection dnsPeerProofStep = "post_inspection"
	dnsPeerProofStepConsume        dnsPeerProofStep = "consume"
)

type dnsPeerProofBudget struct {
	prepare        time.Duration
	challengeWrite time.Duration
	exchange       time.Duration
	postInspection time.Duration
	consume        time.Duration
	// expiryReserve: the latest acceptance lies this long before the
	// challenge expires.
	expiryReserve time.Duration
}

// dnsPeerProofSteps is the production budget. Tests scale it down.
//
// alpha.133 (lab of 12 Nov 2026, PowerDNS primary with a BIND backup, idle):
// prepare 1.6-2.6 s, challenge_write 6.8-7.5 s (once 11.5 s, once timed out
// at 12.2 s), exchange 0.5-0.8 s, post_inspection 3.1-4.0 s, consume
// 8.1-9.9 s; loaded host: prepare 3.1-5.8 s (once 8.1 s) and challenge_write
// over 12 s six times in a row. With one full local recheck per step
// (nativePeerCurrentChecks, "Where the full local recheck runs") the expected
// idle cost is about one recheck plus the journal write: prepare,
// challenge_write and consume about 2 s, post_inspection about 3 s. Every
// bound is at least three times that; prepare takes 10 s (its loaded cost
// reached 5.8-8.1 s) and consume gives up 2 s for it (18 s, nine times its
// new idle cost). The hard upper bound stays the 60 s total.
var dnsPeerProofSteps = dnsPeerProofBudget{
	prepare:        10 * time.Second,
	challengeWrite: 12 * time.Second,
	exchange:       10 * time.Second, // = dnspeertransport.ExchangeLimit (linux test)
	postInspection: 10 * time.Second,
	consume:        18 * time.Second,
	expiryReserve:  5 * time.Second,
}

// timeoutSide names which side of the proof a step's deadline concerns:
// prepare and challenge_write run only this server's checks; the exchange is
// the secondary's inspector; post_inspection probes both servers, so the
// check that ran out of time names the side (dnsPeerProofDeadlineSide):
// this server's recheck, or the wait for the secondary's catalog transfer.
func (step dnsPeerProofStep) timeoutSide() string {
	switch step {
	case dnsPeerProofStepPrepare, dnsPeerProofStepChallengeWrite:
		return transport.DNSPeerProofTimeoutLocalCheck
	case dnsPeerProofStepExchange:
		return transport.DNSPeerProofTimeoutPeerExchange
	default:
		return ""
	}
}

func (b dnsPeerProofBudget) limit(step dnsPeerProofStep) time.Duration {
	switch step {
	case dnsPeerProofStepPrepare:
		return b.prepare
	case dnsPeerProofStepChallengeWrite:
		return b.challengeWrite
	case dnsPeerProofStepExchange:
		return b.exchange
	case dnsPeerProofStepPostInspection:
		return b.postInspection
	case dnsPeerProofStepConsume:
		return b.consume
	default:
		return 0
	}
}

// dnsPeerProofRun is one attempt of a native peer proof: its step bounds,
// the per-step timing and the Agent log lines of its outcome.
type dnsPeerProofRun struct {
	base     context.Context
	budget   dnsPeerProofBudget
	zone     string
	peer     string
	started  time.Time
	acceptBy time.Time
	timings  []string
	stopLog  bool

	fresh       context.Context
	cancelFresh context.CancelFunc
}

func newDNSPeerProofRun(base context.Context, budget dnsPeerProofBudget, zone, peer string) *dnsPeerProofRun {
	return &dnsPeerProofRun{base: base, budget: budget, zone: zone, peer: peer, started: time.Now()}
}

// challengeMinted bounds every later step before acceptance by the minted
// challenge's expiry less the reserve consume-once needs.
func (r *dnsPeerProofRun) challengeMinted(expiresAt time.Time) {
	r.acceptBy = expiresAt.Add(-r.budget.expiryReserve)
}

func (r *dnsPeerProofRun) record(step dnsPeerProofStep, elapsed time.Duration) {
	r.timings = append(r.timings, fmt.Sprintf("%s=%s", step, elapsed.Round(time.Millisecond)))
}

// step runs one step before the peer's answer is accepted under the step's
// own bound (and the acceptance deadline once a challenge exists).
func (r *dnsPeerProofRun) step(step dnsPeerProofStep, fn func(context.Context) error) error {
	return r.stepWithin(step, 0, fn)
}

// stepWithin is step with a tighter bound when limit is positive and lower
// than the step's own (the exchange passes the enrollment's transport bound,
// so the transport never outlives the step and its deadline is the step's).
func (r *dnsPeerProofRun) stepWithin(step dnsPeerProofStep, limit time.Duration, fn func(context.Context) error) error {
	own := r.budget.limit(step)
	if limit <= 0 || limit > own {
		limit = own
	}
	ctx, cancel := context.WithTimeout(r.base, limit)
	defer cancel()
	if !r.acceptBy.IsZero() {
		var stop context.CancelFunc
		ctx, stop = context.WithDeadline(ctx, r.acceptBy)
		defer stop()
	}
	started := time.Now()
	err := fn(ctx)
	r.record(step, time.Since(started))
	if err == nil || errors.Is(err, errDNSProducerCatalogRestampedBeforeChallenge) {
		return err
	}
	// An outcome that already carries a reviewed code of its own state is an
	// observation the step made, not a deadline, even when it came back after
	// the step's time ran out (alpha.133 review: an unresolved check record
	// behind a slow fsync must not come back as an automatic proof timeout).
	if code := pendingDNSPeerCode(err); dnsPeerCodeOutlivesDeadline(code) {
		r.stopped(step, code, err)
		return err
	}
	if r.base.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// Before acceptance: nothing of the proof was consumed (the deletion
		// itself is already committed on this server); the same publication
		// is retried as a new attempt. The code names which side ran out of
		// time, so the owner is not sent to the secondary for this server's
		// own checks (incident of 12 Nov 2026).
		side := step.timeoutSide()
		if side == "" {
			// post_inspection: the check that ran out of time names it.
			side = dnsPeerProofDeadlineSide(err)
		}
		code := transport.DNSPeerPendingProofTimeoutWithDetail(side)
		r.stopped(step, code, err)
		return pendingDNSPeerCause(code, err)
	}
	r.stopped(step, pendingDNSPeerCode(err), err)
	return err
}

// dnsPeerCodeOutlivesDeadline reports a reviewed code that names a state the
// step found (this server's check record, the enrollment, the Agent's own
// precondition, the secondary's transfer refusal), which a deadline cannot
// have caused. The inspection and owner-edit codes are excluded: the
// transport and the probes report a check that could not complete under
// them, and after the step's deadline that is the deadline's doing.
func dnsPeerCodeOutlivesDeadline(code string) bool {
	reason, _, ok := transport.SplitDNSPeerPendingCode(code)
	if !ok {
		return false
	}
	switch reason {
	case transport.DNSPeerPendingJournalUnresolved, transport.DNSPeerPendingJournalUnknown,
		transport.DNSPeerPendingEnrollmentRequired, transport.DNSPeerPendingEnrollmentChanged,
		transport.DNSPeerPendingProofInternal, transport.DNSPeerPendingCatalogTransferRefused:
		return true
	default:
		return false
	}
}

// accepted returns the consume context. It is created on the first call,
// which the proof makes only inside dnspeerproof.Verify's consume callback,
// that is after the peer's answer was accepted. Its parent is the request
// context without its cancellation; it has its own bound.
func (r *dnsPeerProofRun) accepted() context.Context {
	if r.fresh == nil {
		r.fresh, r.cancelFresh = context.WithTimeout(context.WithoutCancel(r.base), r.budget.consume)
	}
	return r.fresh
}

// consume runs the acceptance step: fn calls the proof's Verify, whose
// consume callback and the final recheck use r.accepted(). A failure that
// carries no reviewed code (a journal failure, or the fresh bound running
// out) is dns_peer_journal_unknown, never dns_peer_proof_timeout; a recheck
// that found a real difference keeps its own code (owner edit with its
// check token).
func (r *dnsPeerProofRun) consume(fn func() error) error {
	started := time.Now()
	err := fn()
	r.record(dnsPeerProofStepConsume, time.Since(started))
	if r.cancelFresh != nil {
		r.cancelFresh()
	}
	if err == nil {
		return nil
	}
	if pendingDNSPeerCode(err) == "" {
		err = pendingDNSPeerCause(transport.DNSPeerPendingJournalUnknown, err)
	}
	r.stopped(dnsPeerProofStepConsume, pendingDNSPeerCode(err), err)
	return err
}

// stopped logs, once per attempt, the step and the underlying error of a
// pending outcome (bounded, product-authored, no key material).
func (r *dnsPeerProofRun) stopped(step dnsPeerProofStep, code string, err error) {
	if r.stopLog {
		return
	}
	r.stopLog = true
	if code == "" {
		code = "unclassified"
	}
	cause := dnsPeerPendingCause(err)
	if cause == nil {
		cause = err
	}
	log.Printf("DNS peer proof for %s (%s secondary) stopped at step %s as %s: %s",
		r.zone, r.peer, step, code, boundedDNSPeerProofLogText(cause))
}

// finish logs the per-step elapsed times and the outcome of the attempt.
func (r *dnsPeerProofRun) finish(err error) {
	outcome := "verified"
	switch {
	case err == nil:
	case errors.Is(err, errDNSProducerCatalogRestampedBeforeExchange):
		outcome = "challenge superseded before its exchange (catalog re-stamped; the wave proves the pair again)"
	case errors.Is(err, errDNSProducerCatalogRestampedBeforeChallenge):
		outcome = "no challenge (catalog re-stamped; the wave proves the pair again)"
	default:
		code := pendingDNSPeerCode(err)
		if code == "" {
			code = "unclassified"
		}
		outcome = "pending " + code
		if !r.stopLog {
			r.stopped("verifier", code, err)
		}
	}
	timings := strings.Join(r.timings, " ")
	if timings == "" {
		timings = "none"
	}
	log.Printf("DNS peer proof for %s (%s secondary) step times: %s; total %s; outcome %s",
		r.zone, r.peer, timings, time.Since(r.started).Round(time.Millisecond), outcome)
}

// logAnswer records the inspector's answer fields (bounded tokens) and
// whether Verify accepted it (accepted: Verify called consume-once).
func (r *dnsPeerProofRun) logAnswer(catalogState, memberState, nativeState string, accepted bool, verifyErr error) {
	verdict := "accepted"
	if !accepted {
		verdict = "not accepted: " + boundedDNSPeerProofLogText(verifyErr)
	}
	log.Printf("DNS peer inspector answer for %s (%s secondary): catalog_state=%s member_state=%s native_state=%s; %s",
		r.zone, r.peer, boundedDNSPeerAnswerField(catalogState), boundedDNSPeerAnswerField(memberState),
		boundedDNSPeerAnswerField(nativeState), verdict)
}

// boundedDNSPeerAnswerField keeps a peer-supplied state token printable and
// short: lowercase letters, digits and underscore, at most 32 bytes.
func boundedDNSPeerAnswerField(value string) string {
	if value == "" {
		return "(empty)"
	}
	var out strings.Builder
	for i, r := range value {
		if i >= 32 {
			out.WriteString("...")
			break
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('?')
		}
	}
	return out.String()
}

// nativePeerCurrentChecks is the current-evidence recheck of one native
// proof, bound to the context of the step that runs it.
type nativePeerCurrentChecks struct {
	attempt    func() error
	enrollment func() error
	local      func(context.Context) error
	last       error
}

// at returns the recheck under ctx. A local check that failed after ctx
// ended observed nothing: it is a deadline, never an owner edit.
func (c *nativePeerCurrentChecks) at(ctx context.Context) func() error {
	return func() error {
		err := peerCurrentPendingCodeAt(c.attempt, c.enrollment, func() error {
			if c.local == nil {
				return dnsPeerProofInternal(errors.New("local evidence recheck is missing"))
			}
			err := c.local(ctx)
			if err != nil && ctx.Err() != nil {
				return dnsPeerProofDeadlineAt(transport.DNSPeerProofTimeoutLocalCheck, err)
			}
			return err
		})
		c.last = err
		return err
	}
}

// held is the recheck of what authorizes journal work without the local DNS
// evidence: the exact active ledger attempt under both held external locks
// (attempt) and the unchanged enrollment. It runs no command and takes no
// context.
func (c *nativePeerCurrentChecks) held() error {
	err := peerCurrentPendingCodeAt(c.attempt, c.enrollment, func() error { return nil })
	c.last = err
	return err
}

// Where the full local recheck runs (alpha.133, lab of 12 Nov 2026).
//
// A full local recheck (at) re-reads this server's DNS evidence: the engine
// state receipt twice, the native binding (unit processes, database file)
// twice, that only the managed DNS service is active, the exact deletion
// receipt and the producer catalog; about 1.5-2 s on an idle PowerDNS
// primary and 3 s or more on a loaded one. Until alpha.133 every journal
// write ran it at each of its internal boundaries: publish 4 times
// (challenge_write 7 s idle, over 12 s loaded: the lab's timeouts), consume
// 4 times plus once after (9-10 s idle), post_inspection twice: 12 per
// attempt.
//
// The journal operations need the held checks at their boundaries: both
// external locks and the exact active ledger attempt (what authorizes any
// journal write) and the unchanged enrollment. The local evidence is a
// property of the proof, judged against the attempt's recorded plan, not of
// the write; it is checked once at each point the proof depends on it:
//
//	prepare          before the challenge is minted (prepareJournal)
//	challenge_write  after the challenge is durable, before it is sent
//	                 (writeChallenge). It admits a daemon re-stamp into the
//	                 record; the PowerDNS-secondary path then does not send
//	                 the re-stamped challenge (restamped); the BIND-secondary
//	                 path does not make that check (open, pre-existing)
//	post_inspection  after the network round trip, before the catalog and
//	                 zone probes (and again on a probe retry)
//	consume          after consume-once, before terminal success (final)
//
// Each compares with the same recorded evidence, so a lasting change at any
// moment between mint and success is seen by the next one; the dropped
// rechecks re-sampled the same comparison milliseconds apart. Four per
// attempt instead of twelve. The one trade-off: an owner edit in the 1-3 s
// between the post-inspection recheck and the final one is now found after
// consume-once, not before it. The result is still pending (owner edit) and
// retried, never success; the next attempt's challenge supersedes the
// consumed record (the journal's successor rule does not compare State).

// journalOpHeld runs one private challenge journal operation whose rechecks
// are the held ones. A recheck that found a reviewed difference keeps its own
// code; anything else is dns_peer_journal_unknown with its cause logged.
func (c *nativePeerCurrentChecks) journalOpHeld(what string, op func(func() error) error) error {
	return c.journalOpWith(c.held, what, op)
}

// prepareJournal is the journal part of a native proof's prepare step:
// reconcile retires an earlier terminal operation's challenge under the held
// checks (incident of 12 Nov 2026: seven full local rechecks overran
// prepare's 8 s on every attempt and the retirement never completed), then
// the full current-evidence recheck runs under ctx before any challenge is
// minted. A challenge of this same operation left by an earlier attempt
// costs nothing here: the next publish supersedes it.
func (c *nativePeerCurrentChecks) prepareJournal(ctx context.Context, reconcile func(func() error) error) error {
	if err := c.journalOpHeld("previous challenge reconciliation", reconcile); err != nil {
		return err
	}
	return c.at(ctx)()
}

// writeChallenge is the challenge_write step's journal work: publish under
// the held checks, then one full recheck under ctx once the challenge is
// durable and before it is sent.
func (c *nativePeerCurrentChecks) writeChallenge(ctx context.Context, publish func(func() error) error) error {
	if err := c.journalOpHeld("challenge publish", publish); err != nil {
		return err
	}
	return c.at(ctx)()
}

func (c *nativePeerCurrentChecks) journalOpWith(verify func() error, what string, op func(func() error) error) error {
	c.last = nil
	err := op(verify)
	if err == nil {
		return nil
	}
	if last := c.last; last != nil {
		if pendingDNSPeerCode(last) != "" || isDNSPeerProofDeadline(last) {
			return last
		}
		err = errors.Join(err, last)
	} else if pendingDNSPeerCode(err) != "" {
		// The operation's own reviewed outcome (the reconciliation's
		// dns_peer_journal_unresolved), its held checks having passed.
		return err
	}
	return pendingDNSPeerCause(transport.DNSPeerPendingJournalUnknown, fmt.Errorf("%s: %w", what, err))
}

// nativePeerProofTail is what a native proof does once its challenge is
// minted. Both native verifiers (BIND and PowerDNS secondaries) supply their
// typed journal, transport and answer verification; runNativePeerProofTail
// owns the order of the steps and where the full local recheck runs.
type nativePeerProofTail struct {
	// publish writes the minted challenge durably (journal Publish).
	publish func(verify func() error) error
	// restamped reports a daemon re-stamp admitted since the challenge's
	// serial, given the challenge write's outcome; nil on a path that does
	// not judge it here.
	restamped         func(writeErr error) bool
	recheckEnrollment func() error
	exchangeLimit     time.Duration
	// inspect is the authenticated exchange with the secondary's inspector.
	inspect func(ctx context.Context) error
	// afterInspection runs the post-inspection probes with the full recheck.
	afterInspection func(ctx context.Context, verifyCurrent func() error) error
	// verifyAnswer validates the inspector's answer and calls consume for an
	// accepted one only; it returns the validation error.
	verifyAnswer func(consume func(digest string) error) error
	// consume marks the published challenge consumed (journal ConsumeOnce).
	consume func(digest string, verify func() error) error
}

// runNativePeerProofTail runs challenge_write, exchange, post_inspection and
// consume. Nothing is sent when the challenge write or its recheck failed; a
// challenge is consumed only for an accepted answer; success needs the final
// full recheck after consume-once.
func runNativePeerProofTail(run *dnsPeerProofRun, checks *nativePeerCurrentChecks, tail nativePeerProofTail) error {
	if err := run.step(dnsPeerProofStepChallengeWrite, func(ctx context.Context) error {
		err := checks.writeChallenge(ctx, tail.publish)
		if tail.restamped != nil && tail.restamped(err) {
			return errDNSProducerCatalogRestampedBeforeExchange
		}
		if err != nil {
			return err
		}
		if err := tail.recheckEnrollment(); err != nil {
			return pendingDNSPeerCause(transport.DNSPeerPendingEnrollmentChanged, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := run.stepWithin(dnsPeerProofStepExchange, tail.exchangeLimit, tail.inspect); err != nil {
		return err
	}
	if err := run.step(dnsPeerProofStepPostInspection, func(ctx context.Context) error {
		verifyCurrent := checks.at(ctx)
		if err := verifyCurrent(); err != nil {
			return err
		}
		// No second full recheck here: consume's final recheck, after
		// consume-once and before success, makes the same comparison
		// ("Where the full local recheck runs").
		return tail.afterInspection(ctx, verifyCurrent)
	}); err != nil {
		return err
	}
	return run.consume(func() error {
		var consumeErr error
		err := tail.verifyAnswer(func(digest string) error {
			consumeErr = checks.journalOpHeld("consume-once", func(verify func() error) error {
				return tail.consume(digest, verify)
			})
			return consumeErr
		})
		if consumeErr != nil {
			return consumeErr
		}
		if err != nil {
			return pendingDNSPeerCause(transport.DNSPeerPendingNativeUnknown, err)
		}
		// The answer was accepted: the final recheck runs under the fresh
		// consume context, never under an earlier step's clock.
		return checks.at(run.accepted())()
	})
}
