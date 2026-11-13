package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 review (W2): a step that found a state of its own (an
// unresolved check record, a changed enrollment, ...) keeps that code even
// when it answered after the step's time ran out; only a check that could
// not complete is relabelled as the step's timeout.
func TestStepKeepsAReviewedStateFoundAfterItsDeadline(t *testing.T) {
	captureAgentLog(t)
	budget := scaledPeerProofBudget()
	for _, code := range []string{
		transport.DNSPeerPendingJournalUnresolved,
		transport.DNSPeerPendingJournalUnknown,
		transport.DNSPeerPendingEnrollmentChanged,
		transport.DNSPeerPendingProofInternal,
	} {
		run := newDNSPeerProofRun(context.Background(), budget, "gone.example.test", "BIND")
		err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
			<-ctx.Done()
			return pendingDNSPeerCause(code, errors.New("found after a slow fsync"))
		})
		if got := pendingDNSPeerCode(err); got != code {
			t.Fatalf("%s came back as %q", code, got)
		}
	}
	// What the transport or a probe reports for a check that could not
	// complete is still the deadline's.
	for step, code := range map[dnsPeerProofStep]string{
		dnsPeerProofStepExchange: transport.DNSPeerPendingInspectionUnknown,
		dnsPeerProofStepPrepare:  transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckActiveEngine),
	} {
		run := newDNSPeerProofRun(context.Background(), budget, "gone.example.test", "BIND")
		err := run.step(step, func(ctx context.Context) error {
			<-ctx.Done()
			return pendingDNSPeerCause(code, ctx.Err())
		})
		reason, _, _ := transport.SplitDNSPeerPendingCode(pendingDNSPeerCode(err))
		if reason != transport.DNSPeerPendingProofTimeout {
			t.Fatalf("%s %s after its deadline: %q", step, code, pendingDNSPeerCode(err))
		}
	}
	// Before the deadline every reviewed code passes unchanged.
	run := newDNSPeerProofRun(context.Background(), budget, "gone.example.test", "BIND")
	inspection := transport.DNSPeerPendingInspectionUnknownWithDetail(transport.DNSPeerInspectorReasonPolicy)
	if err := run.step(dnsPeerProofStepExchange, func(context.Context) error {
		return pendingDNSPeerCause(inspection, errors.New("policy"))
	}); pendingDNSPeerCode(err) != inspection {
		t.Fatalf("inspection reason: %q", pendingDNSPeerCode(err))
	}
}
