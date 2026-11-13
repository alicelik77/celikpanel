//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerjournal"
	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The production post-mint sequence (runNativePeerProofTail) with one local
// recheck per step (alpha.133 review): what each recheck still prevents.

type tailRecorder struct {
	published, inspected, consumed int
}

func (r *tailRecorder) tail() nativePeerProofTail {
	return nativePeerProofTail{
		publish: func(verify func() error) error {
			r.published++
			for i := 0; i < journalVerifiesPerWrite; i++ {
				if err := verify(); err != nil {
					return err
				}
			}
			return nil
		},
		recheckEnrollment: func() error { return nil },
		inspect:           func(context.Context) error { r.inspected++; return nil },
		afterInspection:   func(context.Context, func() error) error { return nil },
		verifyAnswer:      func(consume func(string) error) error { return consume("accepted") },
		consume: func(_ string, verify func() error) error {
			for i := 0; i < journalVerifiesPerWrite; i++ {
				if err := verify(); err != nil {
					return err
				}
			}
			r.consumed++
			return nil
		},
	}
}

// localEditAt fails the full local recheck from its n-th call on.
func localEditAt(n int, calls *int) *nativePeerCurrentChecks {
	return &nativePeerCurrentChecks{
		attempt: func() error { return nil }, enrollment: func() error { return nil },
		local: func(context.Context) error {
			*calls++
			if *calls >= n {
				return dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckNativeBinding, "pdns_server restarted")
			}
			return nil
		},
	}
}

var nativeBindingEdit = transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckNativeBinding)

// (a) A local edit found by the recheck after the publish: nothing is sent.
func TestNativePeerTailSendsNothingAfterALocalEditAtPublish(t *testing.T) {
	captureAgentLog(t)
	calls := 0
	recorder := &tailRecorder{}
	run := newDNSPeerProofRun(context.Background(), dnsPeerProofSteps, "gone.example.test", "BIND")
	err := runNativePeerProofTail(run, localEditAt(1, &calls), recorder.tail())
	if pendingDNSPeerCode(err) != nativeBindingEdit || recorder.published != 1 || recorder.inspected != 0 || recorder.consumed != 0 {
		t.Fatalf("code=%q published=%d inspected=%d consumed=%d", pendingDNSPeerCode(err),
			recorder.published, recorder.inspected, recorder.consumed)
	}
	if calls != 1 {
		t.Fatalf("full rechecks before the stop: %d", calls)
	}
}

// (b) A local edit between the post-inspection recheck and the final one:
// the challenge is consumed, the result is the owner edit, never success.
// The rechecks of one attempt: after publish (1), after the round trip (2),
// after consume-once (3).
func TestNativePeerTailEditAfterInspectionIsConsumedOwnerEditNotSuccess(t *testing.T) {
	captureAgentLog(t)
	calls := 0
	recorder := &tailRecorder{}
	run := newDNSPeerProofRun(context.Background(), dnsPeerProofSteps, "gone.example.test", "BIND")
	err := runNativePeerProofTail(run, localEditAt(3, &calls), recorder.tail())
	if err == nil || pendingDNSPeerCode(err) != nativeBindingEdit {
		t.Fatalf("edit after inspection: %v (%q)", err, pendingDNSPeerCode(err))
	}
	if recorder.inspected != 1 || recorder.consumed != 1 || calls != 3 {
		t.Fatalf("inspected=%d consumed=%d rechecks=%d", recorder.inspected, recorder.consumed, calls)
	}
	// Without an edit the same sequence succeeds with three full rechecks
	// after minting (prepare's is the fourth).
	calls = 0
	recorder = &tailRecorder{}
	run = newDNSPeerProofRun(context.Background(), dnsPeerProofSteps, "gone.example.test", "BIND")
	if err := runNativePeerProofTail(run, localEditAt(99, &calls), recorder.tail()); err != nil || calls != 3 {
		t.Fatalf("clean attempt: %v rechecks=%d", err, calls)
	}
}

// (b) on the real store: the consumed challenge of (b) is superseded by the
// next attempt's publish. Runs only where the agent-private journal is an
// empty, private test directory (the isolated WSL run sets
// CELIKPANEL_TEST_REAL_PEER_JOURNAL=1 and bind-mounts one); never against a
// live host's journal.
func TestNativePeerTailConsumedChallengeIsSupersededOnTheRealStore(t *testing.T) {
	if os.Getenv("CELIKPANEL_TEST_REAL_PEER_JOURNAL") != "1" || os.Geteuid() != 0 {
		t.Skip("needs root and an isolated, empty agent-private directory (CELIKPANEL_TEST_REAL_PEER_JOURNAL=1)")
	}
	if _, err := dnspeerjournal.Read(); !dnspeerjournal.IsCode(err, dnspeerjournal.Missing) {
		t.Skipf("the agent-private journal is not an empty test directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(dnspeerjournal.Path) })
	captureAgentLog(t)
	previous, _, _ := historicalBINDPeerChallengeFixture(t)
	request := func(attempt uint64) (dnspeerproof.RequestV1, string) {
		r := previous.Request
		var nonce [32]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			t.Fatal(err)
		}
		r.Nonce, r.Attempt = hex.EncodeToString(nonce[:]), attempt
		r.IssuedAtUnix, r.ExpiresAtUnix = time.Now().Add(-time.Second).Unix(), time.Now().Add(30*time.Second).Unix()
		digest, err := dnspeerproof.RequestSHA256(r)
		if err != nil {
			t.Fatal(err)
		}
		return r, digest
	}
	enrollment := strings.Repeat("d", 64)
	realTail := func(r dnspeerproof.RequestV1, digest string) nativePeerProofTail {
		return nativePeerProofTail{
			publish: func(verify func() error) error {
				return dnspeerjournal.Publish(r, nil, enrollment, 1, verify)
			},
			recheckEnrollment: func() error { return nil },
			inspect:           func(context.Context) error { return nil },
			afterInspection:   func(context.Context, func() error) error { return nil },
			verifyAnswer:      func(consume func(string) error) error { return consume(digest) },
			consume: func(digest string, verify func() error) error {
				return dnspeerjournal.ConsumeOnce(r, enrollment, digest, 1, verify)
			},
		}
	}
	calls := 0
	first, firstDigest := request(1)
	run := newDNSPeerProofRun(context.Background(), dnsPeerProofSteps, previous.Request.DeletedZone, "BIND")
	err := runNativePeerProofTail(run, localEditAt(3, &calls), realTail(first, firstDigest))
	stored, readErr := dnspeerjournal.Read()
	if pendingDNSPeerCode(err) != nativeBindingEdit || readErr != nil || stored.State != dnspeerjournal.StateConsumed {
		t.Fatalf("first attempt: code=%q stored=%+v readErr=%v", pendingDNSPeerCode(err), stored, readErr)
	}
	calls = 0
	second, secondDigest := request(2)
	run = newDNSPeerProofRun(context.Background(), dnsPeerProofSteps, previous.Request.DeletedZone, "BIND")
	if err := runNativePeerProofTail(run, localEditAt(99, &calls), realTail(second, secondDigest)); err != nil {
		t.Fatalf("next attempt over the consumed challenge: %v", err)
	}
	if stored, err := dnspeerjournal.Read(); err != nil || stored.Request.Attempt != 2 || stored.State != dnspeerjournal.StateConsumed {
		t.Fatalf("next attempt stored=%+v err=%v", stored, err)
	}
}

// A post-inspection overrun names the side it waited for.
func TestPostInspectionDeadlineNamesItsSide(t *testing.T) {
	captureAgentLog(t)
	budget := scaledPeerProofBudget()
	for side, want := range map[string]string{
		transport.DNSPeerProofTimeoutLocalCheck:   "dns_peer_proof_timeout:local_check",
		transport.DNSPeerProofTimeoutPeerExchange: "dns_peer_proof_timeout:peer_exchange",
		"": transport.DNSPeerPendingProofTimeout,
	} {
		run := newDNSPeerProofRun(context.Background(), budget, "gone.example.test", "BIND")
		err := run.step(dnsPeerProofStepPostInspection, func(ctx context.Context) error {
			<-ctx.Done()
			if side == "" {
				return dnsPeerProofDeadline(ctx.Err())
			}
			return dnsPeerProofDeadlineAt(side, ctx.Err())
		})
		if code := pendingDNSPeerCode(err); code != want {
			t.Fatalf("side %q: %q, want %q", side, code, want)
		}
	}
	// The local recheck names this server when its step runs out.
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	checks := &nativePeerCurrentChecks{
		attempt: func() error { return nil }, enrollment: func() error { return nil },
		local: func(ctx context.Context) error { return ctx.Err() },
	}
	if side := dnsPeerProofDeadlineSide(checks.at(expired)()); side != transport.DNSPeerProofTimeoutLocalCheck {
		t.Fatalf("local recheck deadline side %q", side)
	}
}
