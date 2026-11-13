package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 (incident of 12 Nov 2026): a deletion's DNS step stayed pending
// with the same cause, re-checked every 5 minutes for hours, and nobody was
// told. After dnsPendingPublicationStuckAfter same results in a row the state
// is stuck: the administrators get one notification naming the reason, who
// acts and the next step; re-checks continue at the slow rate; a press of
// "Retry this deletion" is a person's retry again; a resolved publication
// marks the notification seen.

func stuckNoticeRows(t *testing.T, p *Panel) (unseen, total int) {
	t.Helper()
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM account_notices WHERE kind = ? AND seen_at IS NULL`,
		noticeDNSPublicationStuck).Scan(&unseen); err != nil {
		t.Fatal(err)
	}
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM account_notices WHERE kind = ?`,
		noticeDNSPublicationStuck).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return unseen, total
}

func TestDNSPendingPublicationStuckNotifiesAdministratorsOnce(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "stuck-pending.example")
	ctx := context.Background()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
		('stuck-admin', 'x', 'admin@example.test', 'admin'),
		('stuck-customer', 'x', 'customer@example.test', 'customer')`); err != nil {
		t.Fatal(err)
	}
	admins, err := administratorIDs(ctx, panel.db.GetDB())
	if err != nil || len(admins) == 0 {
		t.Fatalf("administrators=%v err=%v", admins, err)
	}
	code := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck)
	configurePendingWithCode(t, agent, code)
	stuck := true
	recoverOutcome(t, agent, func() bool { return stuck }, func() string { return code })
	if err := panel.syncZoneToDNS(ctx, "stuck-pending.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "stuck-pending.example")
	if err != nil {
		t.Fatal(err)
	}
	panel.reconcilePendingDNSPublicationsOnce(ctx) // discovery
	for attempt := 1; attempt <= dnsPendingPublicationStuckAfter+2; attempt++ {
		entry, _ := scheduler.entry(lease.RequestID)
		clock.Advance(entry.NextAt.Sub(clock.Now()))
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		entry, _ = scheduler.entry(lease.RequestID)
		unseen, total := stuckNoticeRows(t, panel)
		wantStuck := attempt >= dnsPendingPublicationStuckAfter
		if entry.Repeats != attempt || entry.Stuck != wantStuck || !entry.Automatic {
			t.Fatalf("attempt %d: entry=%+v", attempt, entry)
		}
		if wantStuck && (unseen != len(admins) || total != len(admins)) || !wantStuck && total != 0 {
			t.Fatalf("attempt %d: notices unseen=%d total=%d", attempt, unseen, total)
		}
		if wantStuck && entry.NextAt.Sub(clock.Now()) != dnsPendingPublicationMaxDelay {
			t.Fatalf("attempt %d: stuck re-check not at the slow rate: %s", attempt, entry.NextAt.Sub(clock.Now()))
		}
	}
	var audience, body, fix string
	var action int
	if err := panel.db.GetDB().QueryRow(`SELECT n.audience, n.needs_action, n.body, n.fix_path FROM account_notices n
		JOIN users u ON u.id = n.user_id WHERE n.kind = ? AND u.username = 'stuck-admin'`, noticeDNSPublicationStuck).
		Scan(&audience, &action, &body, &fix); err != nil {
		t.Fatalf("administrator notice: %v", err)
	}
	if audience != audienceAdministrator || action != 1 || fix != "/dns-infrastructure" ||
		!strings.Contains(body, `"detail":"local_check"`) {
		t.Fatalf("notice audience=%s action=%d fix=%s body=%s", audience, action, fix, body)
	}
	// A panel restart forgets the memory; the durable row stays one.
	scheduler = useTestPendingScheduler(panel, clock)
	for i := 0; i <= dnsPendingPublicationStuckAfter+1; i++ {
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		entry, _ := scheduler.entry(lease.RequestID)
		clock.Advance(entry.NextAt.Sub(clock.Now()))
	}
	if unseen, total := stuckNoticeRows(t, panel); unseen != len(admins) || total != len(admins) {
		t.Fatalf("after restart notices unseen=%d total=%d", unseen, total)
	}
	// Resolved: the notification no longer describes the server.
	stuck = false
	entry, _ := scheduler.entry(lease.RequestID)
	clock.Advance(entry.NextAt.Sub(clock.Now()))
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if leaseCount(t, panel) != 0 {
		t.Fatal("publication did not complete")
	}
	if unseen, total := stuckNoticeRows(t, panel); unseen != 0 || total != len(admins) {
		t.Fatalf("resolved: notices unseen=%d total=%d", unseen, total)
	}
}

// A changed cause starts the count again; a stuck entry stops counting as
// "continues by itself", so a press is a person's retry.
func TestDNSPendingPublicationStuckResetsOnChangedCause(t *testing.T) {
	clock := &pendingPublicationClock{now: time.Date(2026, 11, 12, 11, 0, 0, 0, time.UTC)}
	panel := &Panel{}
	scheduler := useTestPendingScheduler(panel, clock)
	lease := dnsZoneEngineLease{RequestID: strings.Repeat("a", 32), ZoneName: "gone.example"}
	timeout := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	scheduler.syncLeases([]dnsZoneEngineLease{lease})
	for i := 0; i < dnsPendingPublicationStuckAfter; i++ {
		scheduler.recordAttempt(lease, dnsPendingPublicationStillPending, timeout, true)
	}
	if entry, _ := scheduler.entry(lease.RequestID); !entry.Stuck || !entry.Automatic {
		t.Fatalf("not stuck: %+v", entry)
	}
	if !scheduler.entries[lease.RequestID].Stuck || panel.dnsPendingPublicationsAutomatic() {
		t.Fatal("a stuck publication still counts as continuing by itself")
	}
	if _, due := scheduler.takeStuckNotice(lease.RequestID); !due {
		t.Fatal("first notice not due")
	}
	if _, due := scheduler.takeStuckNotice(lease.RequestID); due {
		t.Fatal("notice due twice")
	}
	scheduler.recordAttempt(lease, dnsPendingPublicationStillPending, transport.DNSPeerPendingCatchingUp, true)
	entry, _ := scheduler.entry(lease.RequestID)
	if entry.Stuck || entry.Noticed || entry.Repeats != 1 {
		t.Fatalf("changed cause kept the stuck state: %+v", entry)
	}
	// An error that is not a pending result breaks the run as well.
	scheduler.recordAttempt(lease, dnsPendingPublicationFailed, "", false)
	if entry, _ := scheduler.entry(lease.RequestID); entry.Repeats != 0 {
		t.Fatalf("failure kept the run: %+v", entry)
	}
}

// The notification says what is already true, why it is stuck, who acts and
// the next step, per reason; English and Turkish; no secret or peer output.
func TestDNSPublicationStuckNoticeWords(t *testing.T) {
	for _, tc := range []struct {
		reason, detail string
		delete         bool
		want           []string
		not            []string
	}{
		{transport.DNSPeerPendingProofTimeout, transport.DNSPeerProofTimeoutLocalCheck, true,
			[]string{"gone.example is removed from DNS on this server", "this server's own part of the check",
				"The backup DNS server is not the cause", "Who acts: you", "journalctl -u celikpanel-agent", "More → Delete…"},
			[]string{"SSH", "port 53"}},
		{transport.DNSPeerPendingProofTimeout, transport.DNSPeerProofTimeoutPeerExchange, true,
			[]string{"is removed from DNS on this server", "has not answered", "SSH (port 22", "whoever runs the backup DNS server"},
			[]string{"journalctl"}},
		{transport.DNSPeerPendingCatchingUp, "", false,
			[]string{"The DNS change of gone.example is in effect on this server", "has not caught up", "TCP port 53"},
			[]string{"Retry this deletion", "Delete…", "removed"}},
		{transport.DNSPeerPendingJournalUnknown, "", true,
			[]string{"is removed from DNS on this server", "Server → DNS and peer servers", "journalctl"}, nil},
	} {
		words := dnsPublicationStuckNoticeWords(audienceAdministrator, map[string]any{
			"zone": "gone.example", "reason": tc.reason, "detail": tc.detail, "delete": tc.delete,
		})
		for _, want := range tc.want {
			if !strings.Contains(words.TitleEN+" "+words.TextEN, want) {
				t.Fatalf("%s:%s lacks %q: %s", tc.reason, tc.detail, want, words.TextEN)
			}
		}
		for _, not := range tc.not {
			if strings.Contains(words.TextEN, not) {
				t.Fatalf("%s:%s says %q: %s", tc.reason, tc.detail, not, words.TextEN)
			}
		}
		if !strings.Contains(words.TitleTR, "takıldı") || !strings.Contains(words.TextTR, "Kim yapar") ||
			strings.Contains(words.TextEN, "Nothing was changed") {
			t.Fatalf("%s:%s words=%+v", tc.reason, tc.detail, words)
		}
	}
}

// alpha.133 review: a re-check the Panel stopped waiting for while the exact
// Agent job was still running has no result yet. It is neither another repeat
// of the earlier cause (six of them raised a false "stuck" notification) nor
// a failure; its result is read again shortly.
func TestDNSPendingPublicationAbandonedRecheckIsNotARepeat(t *testing.T) {
	for _, err := range []error{
		&dnsAgentPublicationError{Err: fmt.Errorf("%w: %w: %v",
			errDNSZoneV3PropagationDeferred, errDNSZoneV3RecoveryStillRunning, errors.New("wait ended"))},
		&dnsAgentPublicationError{Err: errors.Join(errors.New("call ended"), errDNSZoneV3RecoveryStillRunning)},
	} {
		if outcome, _, _ := classifyDNSPendingPublicationAttempt(err); outcome != dnsPendingPublicationInFlight {
			t.Fatalf("%v classified as %d", err, outcome)
		}
	}
	clock := &pendingPublicationClock{now: time.Date(2026, 11, 12, 14, 0, 0, 0, time.UTC)}
	scheduler := useTestPendingScheduler(&Panel{}, clock)
	lease := dnsZoneEngineLease{RequestID: strings.Repeat("b", 32), ZoneName: "sil7.test"}
	code := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck)
	scheduler.syncLeases([]dnsZoneEngineLease{lease})
	for i := 0; i < dnsPendingPublicationStuckAfter-1; i++ {
		scheduler.recordAttempt(lease, dnsPendingPublicationStillPending, code, true)
	}
	before, _ := scheduler.entry(lease.RequestID)
	for i := 0; i < 3; i++ {
		entry := scheduler.recordAttempt(lease, dnsPendingPublicationInFlight, "", false)
		if entry.Repeats != before.Repeats || entry.Failures != 0 || entry.Stuck || entry.Attempts != before.Attempts ||
			entry.NextAt.Sub(clock.Now()) != dnsPendingPublicationFirstDelay {
			t.Fatalf("abandoned re-check %d counted: %+v", i, entry)
		}
	}
	// The re-check waits longer than the Agent's own bound for a recovery:
	// the propagation wave (15 s) and the native proof (60 s).
	if dnsPendingPublicationAttemptLimit != panelMutationRecoveryTimeout ||
		dnsPendingPublicationAttemptLimit < 15*time.Second+60*time.Second+10*time.Second {
		t.Fatalf("re-check limit %s", dnsPendingPublicationAttemptLimit)
	}
}
