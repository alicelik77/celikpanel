//go:build linux

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerjournal"
	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Lab of 12 Nov 2026 (alpha.133 candidate, PowerDNS primary with a BIND
// backup, loaded host): after the earlier challenge was retired, six attempts
// in a row stopped at challenge_write after 12.0 s ("prepare=3.1-5.8s
// challenge_write=12.0s"); idle, challenge_write cost 6.8-7.5 s. Publish
// called its verifier four times (TestPublishAndConsumeVerifyFourTimes) and
// every call was a full local recheck.

// journalVerifiesPerWrite is how often publish and consume-once call their
// verifier on success.
const journalVerifiesPerWrite = 4

var errLostTestLock = errors.New("native BIND proof lost its host or publication lock")

// loadedLocalRecheck is one full local recheck on the loaded lab host
// (prepare=3.1-5.8 s ran one).
const loadedLocalRecheck = 3100 * time.Millisecond

// sameOperationJournal holds this operation's own challenge once a publish
// reached its rename, as the real store does (the lab's 6th attempt left a
// 1082-byte outstanding challenge of sil7.test).
type sameOperationJournal struct {
	stored    *dnspeerjournal.RecordV1
	published int
}

func (j *sameOperationJournal) read() (dnspeerjournal.RecordV1, error) {
	if j.stored == nil {
		return dnspeerjournal.RecordV1{}, dnspeerjournal.StateError{Code: dnspeerjournal.Missing}
	}
	return *j.stored, nil
}

// publish mirrors dnspeerjournal.Publish: verify, then writeAt's verify
// before staging, after staging (the rename follows) and after the write.
func (j *sameOperationJournal) publish(record dnspeerjournal.RecordV1, verify func() error) error {
	for i := 0; i < journalVerifiesPerWrite; i++ {
		if verify() != nil {
			return dnspeerjournal.StateError{Code: dnspeerjournal.Unknown}
		}
		if i == 2 {
			j.stored = &record
			j.published++
		}
	}
	return nil
}

func TestChallengeWriteRunsOneFullRecheckAndFitsItsBound(t *testing.T) {
	captureAgentLog(t)
	previous, ledger, nextPlan := historicalBINDPeerChallengeFixture(t)
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	own := previous
	own.State = dnspeerjournal.StateOutstanding
	own.Request.MutationRequestID = nextPlan.Operation.RequestID
	own.Request.MutationOwnerID = nextPlan.Operation.OwnerID
	own.Request.DeletedZone = nextPlan.Changed.Domain
	own.Request.DeletionQualifier = nextPlan.Operation.Qualifier
	budget := dnsPeerProofSteps
	journal := &sameOperationJournal{}
	retired := 0
	reconcile := func(verify func() error) error {
		return reconcileHistoricalBINDPeerChallengeAt(nextPlan, verify, load, cleanBINDJournal, journal.read,
			func(_ dnspeerproof.RequestV1, _, _ string, _ uint64, _ dnspeerjournal.VerifyCurrent) error {
				retired++
				return nil
			})
	}

	// The alpha.132 composition: every journal boundary a full recheck.
	locals := 0
	checks := staleChallengeChecks(loadedLocalRecheck, &locals)
	run := newDNSPeerProofRun(context.Background(), budget, nextPlan.Changed.Domain, "BIND")
	err := run.step(dnsPeerProofStepChallengeWrite, func(ctx context.Context) error {
		return checks.journalOpWith(checks.at(ctx), "challenge publish", func(verify func() error) error {
			return journal.publish(own, verify)
		})
	})
	if code := pendingDNSPeerCode(err); code != "dns_peer_proof_timeout:local_check" || journal.published != 1 {
		t.Fatalf("the alpha.132 composition did not reproduce the lab: code=%q published=%d", code, journal.published)
	}

	// The next attempt finds this operation's own outstanding challenge:
	// prepare neither retires nor rechecks for it (no spiral), and the
	// production challenge_write runs one full recheck.
	for attempt := 1; attempt <= 3; attempt++ {
		locals = 0
		run = newDNSPeerProofRun(context.Background(), budget, nextPlan.Changed.Domain, "BIND")
		started := time.Now()
		if err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
			return checks.prepareJournal(ctx, reconcile)
		}); err != nil || locals != 1 || retired != 0 {
			t.Fatalf("attempt %d prepare: err=%v rechecks=%d retirements=%d", attempt, err, locals, retired)
		}
		if elapsed := time.Since(started); elapsed > loadedLocalRecheck+time.Second {
			t.Fatalf("attempt %d prepare took %s", attempt, elapsed)
		}
		locals = 0
		started = time.Now()
		if err := run.step(dnsPeerProofStepChallengeWrite, func(ctx context.Context) error {
			return checks.writeChallenge(ctx, func(verify func() error) error { return journal.publish(own, verify) })
		}); err != nil || locals != 1 {
			t.Fatalf("attempt %d challenge_write: err=%v rechecks=%d", attempt, err, locals)
		}
		if elapsed := time.Since(started); elapsed > budget.challengeWrite/3 {
			t.Fatalf("attempt %d challenge_write took %s of its %s", attempt, elapsed, budget.challengeWrite)
		}
	}
}

// A real local difference is still found by the one recheck after the
// publish, with its own reviewed check; a lost lock or ledger attempt is
// still found at every journal boundary.
func TestChallengeWriteKeepsWhatItProves(t *testing.T) {
	captureAgentLog(t)
	previous, _, _ := historicalBINDPeerChallengeFixture(t)
	journal := &sameOperationJournal{}
	edited := &nativePeerCurrentChecks{
		attempt: func() error { return nil }, enrollment: func() error { return nil },
		local: func(context.Context) error {
			return dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckNativeBinding, "pdns_server restarted")
		},
	}
	err := edited.writeChallenge(context.Background(), func(verify func() error) error { return journal.publish(previous, verify) })
	want := transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckNativeBinding)
	if pendingDNSPeerCode(err) != want {
		t.Fatalf("local difference after publish: %q", pendingDNSPeerCode(err))
	}
	held := 0
	lost := &nativePeerCurrentChecks{
		attempt: func() error {
			held++
			if held == 3 {
				return errLostTestLock
			}
			return nil
		},
		enrollment: func() error { return nil },
		local:      func(context.Context) error { t.Fatal("full recheck after a lost lock"); return nil },
	}
	journal = &sameOperationJournal{}
	err = lost.writeChallenge(context.Background(), func(verify func() error) error { return journal.publish(previous, verify) })
	if pendingDNSPeerCode(err) != transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckOperationAttempt) {
		t.Fatalf("lost lock during publish: %q", pendingDNSPeerCode(err))
	}
}
