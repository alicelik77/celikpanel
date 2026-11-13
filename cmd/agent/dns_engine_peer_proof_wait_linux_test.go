//go:build linux

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeertransport"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5 (lab w9, 13 Nov 2026): with the backup's SSH port
// blocked, two re-checks ended as dns_peer_proof_timeout:peer_exchange and
// the third as dns_peer_inspection_unknown (exchange=10s). The dialer's own
// poller deadline and the step's context deadline are two timers for the
// same instant; when the poller's fired first, the transport returned before
// ctx.Err() was set and the step kept the inspection code.

// returnAtBound returns err a moment before ctx's deadline, while ctx.Err()
// is still nil: the racing shape.
func returnAtBound(t *testing.T, err error) func(context.Context) error {
	return func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("step has no deadline")
		}
		time.Sleep(time.Until(deadline) - 20*time.Millisecond)
		if ctx.Err() != nil {
			t.Fatal("the context ended before the racing return")
		}
		return err
	}
}

func returnAfterBound(err error) func(context.Context) error {
	return func(ctx context.Context) error { <-ctx.Done(); return err }
}

func returnAtOnce(err error) func(context.Context) error {
	return func(context.Context) error { return err }
}

func noAnswerInspection() error {
	cause := dnspeertransport.Unknown{Code: dnspeertransport.CodeNoAnswer}
	return pendingDNSPeerCause(bindInspectionPendingCode(cause), cause)
}

func refusedInspection(reason string) error {
	cause := dnspeertransport.Unknown{Code: dnspeertransport.CodeUnavailable, Reason: reason}
	return pendingDNSPeerCause(bindInspectionPendingCode(cause), cause)
}

// runW9Attempt is one completion wave of a paired deletion whose native proof
// stops at the exchange with exchange(ctx).
func runW9Attempt(t *testing.T, exchange func(context.Context) error) error {
	t.Helper()
	budget := scaledPeerProofBudget()
	plan, soa, localAXFR, peerCatalog, peerZone := refusedDeletionWaveProbes()
	native := func(ctx context.Context, _ dnsPeerAXFRAuthority, _ dnsV3PrimaryPropagationPlan) (result error) {
		run := newDNSPeerProofRun(ctx, budget, "w9.test", "BIND")
		defer func() { run.finish(result) }()
		if err := run.step(dnsPeerProofStepPrepare, func(context.Context) error { return nil }); err != nil {
			return err
		}
		if err := run.stepWithin(dnsPeerProofStepExchange, budget.exchange, exchange); err != nil {
			return err
		}
		t.Fatal("the proof went past an exchange that failed")
		return nil
	}
	return completeDNSV3PrimaryPropagationWithNativeAt(context.Background(), plan, soa, localAXFR,
		peerCatalog, peerZone, native)
}

// Every shape of "the backup gave no answer" is the exchange's timeout, an
// automatic wait, at whichever instant it comes back; the Agent's line says
// the Panel checks again.
func TestW9BackupWithoutAnswerIsAWaitWhateverTheTransportReturned(t *testing.T) {
	logs := captureAgentLog(t)
	withScaledWave(t)
	want := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	for _, tc := range []struct {
		name     string
		exchange func(context.Context) error
	}{
		{"dial timer before the step's timer (the w9 race)", returnAtBound(t, noAnswerInspection())},
		{"the step's timer first", returnAfterBound(noAnswerInspection())},
		{"connection refused at once", returnAtOnce(noAnswerInspection())},
		// The alpha.133 transport's shape for the same race: an inspection
		// code with no answer behind it, at the bound.
		{"an inspection code at the bound with nothing answered", returnAtBound(t,
			pendingDNSPeerCause(transport.DNSPeerPendingInspectionUnknown, errors.New("i/o timeout")))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			err := runW9Attempt(t, tc.exchange)
			if code := pendingDNSPeerCode(err); code != want {
				t.Fatalf("code %q, want %q (err %v)", code, want, err)
			}
			text := logs.String() + err.Error()
			if !strings.Contains(text, "stopped at step exchange as "+want) ||
				!strings.Contains(text, "checks the same operation again automatically until it is confirmed") {
				t.Fatalf("lines:\n%s", text)
			}
		})
	}
}

// A backup that answered and refused keeps its own code, at once and at the
// bound: the code proves something about the answer.
func TestW9BackupThatAnswersAndRefusesKeepsItsCode(t *testing.T) {
	captureAgentLog(t)
	withScaledWave(t)
	named := transport.DNSPeerPendingInspectionUnknownWithDetail(transport.DNSPeerInspectorReasonNamedUnavailable)
	for _, tc := range []struct {
		name     string
		exchange func(context.Context) error
		want     string
	}{
		{"refused at once", returnAtOnce(refusedInspection(transport.DNSPeerInspectorReasonNamedUnavailable)), named},
		{"refused at the bound", returnAtBound(t, refusedInspection(transport.DNSPeerInspectorReasonNamedUnavailable)), named},
		{"another host key at the bound", returnAtBound(t, refusedInspection("")), transport.DNSPeerPendingInspectionUnknown},
		{"a malformed answer at the bound", returnAtBound(t, pendingDNSPeerCause(transport.DNSPeerPendingInspectionUnknown,
			dnspeertransport.Unknown{Code: dnspeertransport.CodeMalformed})), transport.DNSPeerPendingInspectionUnknown},
		{"refused transfer at the bound", returnAtBound(t, refusedInspection(transport.DNSPeerInspectorReasonCatalogTransferRefused)),
			transport.DNSPeerPendingCatalogTransferRefused},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := pendingDNSPeerCode(runW9Attempt(t, tc.exchange)); code != tc.want {
				t.Fatalf("code %q, want %q", code, tc.want)
			}
		})
	}
}

// The Agent's line for the one reason the Panel does not re-check says so.
func TestAwaitingPeerLineFollowsTheCode(t *testing.T) {
	for _, code := range []string{transport.DNSPeerPendingJournalUnresolved, transport.DNSPeerPendingOwnerEditUnknown,
		transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckProducerCatalog),
		transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckAuthority)} {
		line := pairedDeletionAwaitingPeer(dnsV3ProofNativePeer, code).Error()
		if strings.Contains(line, "again automatically until") || !strings.Contains(line, "does not check this operation again automatically") {
			t.Fatalf("%s line: %s", code, line)
		}
	}
	for _, code := range []string{"", transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingInspectionUnknown,
		transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange), transport.DNSPeerPendingProofInternal,
		transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckZoneAnswered)} {
		line := pairedDeletionAwaitingPeer(dnsV3ProofNativePeer, code).Error()
		if !strings.Contains(line, "checks the same operation again automatically until it is confirmed") {
			t.Fatalf("%q line: %s", code, line)
		}
	}
}

// The bound is read from the deadline, not only from ctx.Err().
func TestStepDeadlineReachedReadsTheBound(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	for _, tc := range []struct {
		now  time.Time
		want bool
	}{
		{deadline.Add(-time.Second), false},
		{deadline.Add(-dnsPeerStepDeadlineMargin - time.Millisecond), false},
		{deadline.Add(-dnsPeerStepDeadlineMargin), true},
		{deadline.Add(-time.Millisecond), true},
		{deadline.Add(time.Millisecond), true},
	} {
		if got := dnsPeerStepDeadlineReached(ctx, tc.now); got != tc.want {
			t.Fatalf("at %s before the deadline: %t, want %t", deadline.Sub(tc.now), got, tc.want)
		}
	}
	if dnsPeerStepDeadlineReached(context.Background(), time.Now()) {
		t.Fatal("a context without a deadline ran out of time")
	}
}
