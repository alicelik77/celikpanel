//go:build linux

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerjournal"
	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Incident of 12 Nov 2026 (Frankfurt, PowerDNS primary with a BIND backup):
// the journal still held the consumed challenge of a deletion finished on
// 6 Nov. Before a new deletion's first challenge, prepare retires it. Until
// alpha.133 prepare ran the full local DNS recheck before the reconciliation
// and passed the same full recheck to it, which ran it once before Retire,
// four times inside Retire and once after: seven rechecks of about 1.5 s
// each (one recheck made prepare take 1.5-2 s on 6 Nov). Prepare's 8 s ran
// out during the fourth recheck inside Retire, before its exchange, on every
// attempt: the journal kept the old challenge and every retry started the
// retirement again ("stopped at step prepare as dns_peer_proof_timeout ...
// active_engine differed ... signal: killed", prepare=8.006s).

// observedLocalRecheck is the cost of one full PowerDNS local recheck
// observed in the incident.
const observedLocalRecheck = 1500 * time.Millisecond

// retireCallsPerRetirement is how often dnspeerjournal.Retire calls its
// verifier on success (TestRetireVerifiesTerminalFourTimes there).
const retireCallsPerRetirement = 4

type staleChallengeJournal struct {
	stored  *dnspeerjournal.RecordV1
	retires int
}

func (j *staleChallengeJournal) read() (dnspeerjournal.RecordV1, error) {
	if j.stored == nil {
		return dnspeerjournal.RecordV1{}, dnspeerjournal.StateError{Code: dnspeerjournal.Missing}
	}
	return *j.stored, nil
}

// retire mirrors dnspeerjournal.Retire: exact identity, the verifier before
// and around the exchange, then the canonical name is gone.
func (j *staleChallengeJournal) retire(request dnspeerproof.RequestV1, enrollment, digest string, attempt uint64, verify dnspeerjournal.VerifyCurrent) error {
	j.retires++
	if j.stored == nil || j.stored.Request != request || j.stored.EnrollmentSHA256 != enrollment ||
		j.stored.RequestSHA256 != digest || j.stored.LedgerAttempt != attempt {
		return dnspeerjournal.StateError{Code: dnspeerjournal.Mismatch}
	}
	for i := 0; i < retireCallsPerRetirement; i++ {
		if verify() != nil {
			return dnspeerjournal.StateError{Code: dnspeerjournal.Unknown}
		}
	}
	j.stored = nil
	return nil
}

func staleChallengeChecks(cost time.Duration, local *int) *nativePeerCurrentChecks {
	return &nativePeerCurrentChecks{
		attempt:    func() error { return nil },
		enrollment: func() error { return nil },
		local: func(ctx context.Context) error {
			*local++
			// Each recheck starts commands under ctx (exec.CommandContext).
			if err := sleepUnder(ctx, cost); err != nil {
				return dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckActiveEngine,
					"systemctl is-active: signal: killed")
			}
			return nil
		},
	}
}

func TestStaleChallengeRetirementFitsPrepareBudget(t *testing.T) {
	logs := captureAgentLog(t)
	previous, ledger, nextPlan := historicalBINDPeerChallengeFixture(t)
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	budget := dnsPeerProofSteps // the production bounds

	// The alpha.132 order, authority and prepare bound (8 s), reproduced:
	// full recheck, then the reconciliation under the same full recheck.
	alpha132 := budget
	alpha132.prepare = 8 * time.Second
	journal := &staleChallengeJournal{stored: &previous}
	locals := 0
	checks := staleChallengeChecks(observedLocalRecheck, &locals)
	run := newDNSPeerProofRun(context.Background(), alpha132, nextPlan.Changed.Domain, "BIND")
	started := time.Now()
	err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
		if err := checks.at(ctx)(); err != nil {
			return err
		}
		return checks.journalOpWith(checks.at(ctx), "previous challenge reconciliation", func(verify func() error) error {
			return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify, load, journal.read, journal.retire)
		})
	})
	if code := pendingDNSPeerCode(err); code != "dns_peer_proof_timeout:local_check" || journal.stored == nil {
		t.Fatalf("the alpha.132 order did not reproduce the incident: code=%q retired=%t after %s (%d rechecks)",
			code, journal.stored == nil, time.Since(started), locals)
	}
	if locals != 6 || journal.retires != 1 {
		t.Fatalf("the alpha.132 order ran %d local rechecks and %d retirements before its deadline, want 6 and 1",
			locals, journal.retires)
	}

	// The same state through the production prepare sequence.
	journal = &staleChallengeJournal{stored: &previous}
	locals = 0
	checks = staleChallengeChecks(observedLocalRecheck, &locals)
	run = newDNSPeerProofRun(context.Background(), budget, nextPlan.Changed.Domain, "BIND")
	started = time.Now()
	err = run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
		return checks.prepareJournal(ctx, func(verify func() error) error {
			return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify, load, journal.read, journal.retire)
		})
	})
	elapsed := time.Since(started)
	if err != nil || journal.stored != nil || journal.retires != 1 || locals != 1 {
		t.Fatalf("prepare with a stale challenge: err=%v retired=%t retires=%d rechecks=%d",
			err, journal.stored == nil, journal.retires, locals)
	}
	if elapsed > observedLocalRecheck+budget.prepare/4 {
		t.Fatalf("prepare with a stale challenge took %s", elapsed)
	}
	if !strings.Contains(logs.String(), "retired the challenge of an earlier finished deletion (old.example.test, ledger attempt 1)") {
		t.Fatalf("retirement not logged:\n%s", logs)
	}
}

// A prepare whose full recheck runs out of time after the retirement keeps
// the retirement: the next attempt does not retire again and completes.
// This is the field state after the update too: the stale challenge is
// retired by the first attempt the Panel's own re-check starts.
func TestStaleChallengeRetirementPersistsAcrossTimedOutAttempt(t *testing.T) {
	captureAgentLog(t)
	previous, ledger, nextPlan := historicalBINDPeerChallengeFixture(t)
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	budget := scaledPeerProofBudget()
	journal := &staleChallengeJournal{stored: &previous}
	reconcile := func(verify func() error) error {
		return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify, load, journal.read, journal.retire)
	}

	locals := 0
	slow := staleChallengeChecks(budget.prepare*2, &locals)
	run := newDNSPeerProofRun(context.Background(), budget, nextPlan.Changed.Domain, "BIND")
	err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
		return slow.prepareJournal(ctx, reconcile)
	})
	if code := pendingDNSPeerCode(err); code != "dns_peer_proof_timeout:local_check" {
		t.Fatalf("slow recheck: %q (%v)", code, err)
	}
	if journal.stored != nil || journal.retires != 1 {
		t.Fatalf("the retirement did not survive the timed-out attempt: retires=%d", journal.retires)
	}

	fast := staleChallengeChecks(time.Millisecond, &locals)
	run = newDNSPeerProofRun(context.Background(), budget, nextPlan.Changed.Domain, "BIND")
	if err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
		return fast.prepareJournal(ctx, reconcile)
	}); err != nil || journal.retires != 1 {
		t.Fatalf("next attempt: err=%v retires=%d", err, journal.retires)
	}
}

// The held checks still guard the retirement: a lost lock or ledger attempt
// keeps its reviewed owner-edit code and retires nothing; a changed
// enrollment keeps dns_peer_enrollment_changed.
func TestStaleChallengeRetirementKeepsItsAuthority(t *testing.T) {
	captureAgentLog(t)
	previous, ledger, nextPlan := historicalBINDPeerChallengeFixture(t)
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	for _, tc := range []struct {
		name       string
		attempt    error
		enrollment error
		want       string
	}{
		{name: "lost attempt", attempt: errors.New("native BIND proof lost its host or publication lock"),
			want: transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckOperationAttempt)},
		{name: "changed enrollment", enrollment: errors.New("native BIND peer enrollment changed"),
			want: transport.DNSPeerPendingEnrollmentChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			journal := &staleChallengeJournal{stored: &previous}
			locals := 0
			checks := &nativePeerCurrentChecks{
				attempt:    func() error { return tc.attempt },
				enrollment: func() error { return tc.enrollment },
				local:      func(context.Context) error { locals++; return nil },
			}
			err := checks.prepareJournal(context.Background(), func(verify func() error) error {
				return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify, load, journal.read, journal.retire)
			})
			if pendingDNSPeerCode(err) != tc.want || journal.stored == nil || locals != 0 {
				t.Fatalf("code=%q retired=%t rechecks=%d", pendingDNSPeerCode(err), journal.stored == nil, locals)
			}
		})
	}
	// A challenge whose operation is not terminal in the ledger is never
	// retired, whatever the held checks say.
	changed := cloneServiceMutationLedger(ledger)
	changed.Jobs[previous.Request.MutationRequestID].Attempt++
	journal := &staleChallengeJournal{stored: &previous}
	checks := staleChallengeChecks(time.Millisecond, new(int))
	err := checks.prepareJournal(context.Background(), func(verify func() error) error {
		return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify,
			func() (serviceMutationLedger, error) { return changed, nil }, journal.read, journal.retire)
	})
	if pendingDNSPeerCode(err) != transport.DNSPeerPendingJournalUnknown || journal.stored == nil {
		t.Fatalf("nonterminal prior operation: code=%q retired=%t", pendingDNSPeerCode(err), journal.stored == nil)
	}
}

// Which side ran out of time travels with the code: this server's own steps,
// the secondary's exchange, and the post-inspection step (both servers).
func TestNativePeerProofTimeoutNamesItsSide(t *testing.T) {
	captureAgentLog(t)
	budget := scaledPeerProofBudget()
	for step, want := range map[dnsPeerProofStep]string{
		dnsPeerProofStepPrepare:        "dns_peer_proof_timeout:local_check",
		dnsPeerProofStepChallengeWrite: "dns_peer_proof_timeout:local_check",
		dnsPeerProofStepExchange:       "dns_peer_proof_timeout:peer_exchange",
		dnsPeerProofStepPostInspection: transport.DNSPeerPendingProofTimeout,
	} {
		run := newDNSPeerProofRun(context.Background(), budget, "gone.example.test", "BIND")
		err := run.step(step, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
		if code := pendingDNSPeerCode(err); code != want || dnsZoneV3PendingLedgerCode(code) != want {
			t.Fatalf("%s: %q, want %q", step, code, want)
		}
	}
}
