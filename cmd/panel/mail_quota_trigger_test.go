package main

import (
	"context"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.118 lab, D2: a fresh install's first quota check ran before mail
// existed ("no mail") and the next one came an hour later, so a 1 MB mailbox
// took 1.25 MB. A "no mail" or unknown answer is checked again every round;
// configuring mail or creating a mailbox asks for a check at once; an
// enforced answer waits for the hour; requests coalesce into one check.
func TestMailQuotaCheckFollowsMailInsteadOfTheHour(t *testing.T) {
	f := newUsageFixture(t)
	calls := 0
	state := transport.MailQuotaNoMail
	f.panel.mailQuota = func(context.Context) (transport.MailQuotaReconcileResponse, error) {
		calls++
		return transport.MailQuotaReconcileResponse{State: state}, nil
	}
	ctx := context.Background()
	f.panel.runUsageRound(ctx)
	if calls != 1 {
		t.Fatalf("first round: %d checks", calls)
	}
	// Mail is installed five minutes later: the next round enforces it.
	state = transport.MailQuotaEnforced
	f.now = f.now.Add(5 * time.Minute)
	f.panel.runUsageRound(ctx)
	if calls != 2 {
		t.Fatalf("a 'no mail' answer must be checked again next round: %d", calls)
	}
	// Enforced: the next rounds within the hour do not check.
	f.now = f.now.Add(5 * time.Minute)
	f.panel.runUsageRound(ctx)
	if calls != 2 {
		t.Fatalf("an enforced answer waits for the hour: %d", calls)
	}
	// A mailbox is created (twice, quickly): one check in the next round.
	f.panel.requestMailQuotaReconcile()
	f.panel.requestMailQuotaReconcile()
	f.panel.runUsageRound(ctx)
	f.panel.runUsageRound(ctx)
	if calls != 3 {
		t.Fatalf("two requests must coalesce into one check: %d", calls)
	}
	// The hour passes.
	f.now = f.now.Add(time.Hour)
	f.panel.runUsageRound(ctx)
	if calls != 4 {
		t.Fatalf("the hourly check: %d", calls)
	}
}

func TestMailQuotaDueOnUnreachableAgent(t *testing.T) {
	now := time.Date(2026, 11, 3, 14, 0, 0, 0, time.UTC)
	enforced := transport.MailQuotaReconcileResponse{State: transport.MailQuotaEnforced}
	if mailQuotaDue(false, now.Add(-time.Minute), enforced, "", now) {
		t.Fatal("enforced a minute ago is not due")
	}
	if !mailQuotaDue(false, now.Add(-time.Minute), enforced, "agent unreachable", now) {
		t.Fatal("an unreachable Agent is tried again next round")
	}
	if mailQuotaDue(false, now.Add(-time.Minute), transport.MailQuotaReconcileResponse{State: transport.MailQuotaKept}, "", now) {
		t.Fatal("an owner's kept file is not rechecked every round")
	}
}
