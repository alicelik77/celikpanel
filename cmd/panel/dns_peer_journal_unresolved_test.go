package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 release blocker (lab of 12 Nov 2026): a check record the Agent
// cannot prove is dns_peer_journal_unresolved. It is not re-checked by
// itself, the administrators are told once, and the deletion status says a
// person must act; dns_peer_journal_unknown (which the Agent now repairs by
// itself) keeps "nobody needs to act" while its automatic checks run.

func TestJournalUnresolvedNotifiesAdministratorsOnceAndIsNotRetried(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "torn.example")
	ctx := context.Background()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
		('torn-admin', 'x', 'admin@example.test', 'admin'),
		('torn-customer', 'x', 'customer@example.test', 'customer')`); err != nil {
		t.Fatal(err)
	}
	admins, err := administratorIDs(ctx, panel.db.GetDB())
	if err != nil || len(admins) == 0 {
		t.Fatalf("administrators=%v err=%v", admins, err)
	}
	code := transport.DNSPeerPendingJournalUnresolved
	configurePendingWithCode(t, agent, code)
	recoverOutcome(t, agent, func() bool { return true }, func() string { return code })
	recovers := 0
	hook := agent.recoverHook
	agent.recoverHook = func(request transport.RecoverDNSZoneV3Request, response *transport.RecoverDNSZoneV3Response) error {
		recovers++
		return hook(request, response)
	}
	if err := panel.syncZoneToDNS(ctx, "torn.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "torn.example")
	if err != nil {
		t.Fatal(err)
	}
	// The publication's own request may resume its exact operation once;
	// the reconciler then attempts nothing by itself.
	recovers = 0
	for i := 0; i < 5; i++ {
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		clock.Advance(dnsPendingPublicationOwnerRecheck + time.Second)
	}
	entry, _ := scheduler.entry(lease.RequestID)
	if entry.Automatic || entry.Code != code || recovers != 0 {
		t.Fatalf("entry=%+v recovers=%d", entry, recovers)
	}
	unseen, total := stuckNoticeRows(t, panel)
	if unseen != len(admins) || total != len(admins) {
		t.Fatalf("notices unseen=%d total=%d, want %d", unseen, total, len(admins))
	}
	var body string
	if err := panel.db.GetDB().QueryRow(`SELECT n.body FROM account_notices n JOIN users u ON u.id = n.user_id
		WHERE n.kind = ? AND u.username = 'torn-admin'`, noticeDNSPublicationStuck).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"reason":"dns_peer_journal_unresolved"`) {
		t.Fatalf("notice body %s", body)
	}
	if followUp, next := panel.domainDeletionDNSFollowUp(lease, code); followUp != domainDeletionFollowUpPerson || next != "" {
		t.Fatalf("follow-up %q %q", followUp, next)
	}
}

func TestJournalUnresolvedNoticeWords(t *testing.T) {
	words := dnsPublicationStuckNoticeWords(audienceAdministrator, map[string]any{
		"zone": "sil7.test", "reason": transport.DNSPeerPendingJournalUnresolved, "delete": true,
	})
	for _, want := range []string{"sil7.test is removed from DNS on this server", "cannot sort out by itself",
		"Who acts: you", "do not delete those files yourself", "no longer checks this every few minutes",
		"open Details in this deletion's waiting notice", "More → Delete…"} {
		if !strings.Contains(words.TextEN, want) {
			t.Fatalf("EN lacks %q: %s", want, words.TextEN)
		}
	}
	for _, want := range []string{"sil7.test bu sunucuda DNS'ten kaldırıldı", "Kim yapar: siz",
		"kendiliğinden düzeltemeyeceği", "kendiniz silmeyin", "artık birkaç dakikada bir denetlemiyor", "Diğer → Sil…"} {
		if !strings.Contains(words.TextTR, want) {
			t.Fatalf("TR lacks %q: %s", want, words.TextTR)
		}
	}
	if !strings.Contains(words.TitleTR, "sunucu yöneticisini bekliyor") ||
		strings.Contains(words.TextEN, "keeps checking") || strings.Contains(words.TextTR, "denetlemeyi sürdürür") ||
		strings.Contains(words.TextEN, "Retry this deletion") || strings.Contains(words.TextEN, "Nobody needs") {
		t.Fatalf("words=%+v", words)
	}
}

func TestDeletionDNSFollowUpTellsWhetherSomeoneMustAct(t *testing.T) {
	clock := &pendingPublicationClock{now: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)}
	panel := &Panel{}
	scheduler := useTestPendingScheduler(panel, clock)
	lease := dnsZoneEngineLease{RequestID: strings.Repeat("b", 32), ZoneName: "sil7.test"}
	unknown := transport.DNSPeerPendingJournalUnknown
	scheduler.syncLeases([]dnsZoneEngineLease{lease})
	scheduler.recordAttempt(lease, dnsPendingPublicationStillPending, unknown, true)
	followUp, next := panel.domainDeletionDNSFollowUp(lease, unknown)
	entry, _ := scheduler.entry(lease.RequestID)
	if followUp != domainDeletionFollowUpAutomatic || next != entry.NextAt.UTC().Format(time.RFC3339) {
		t.Fatalf("automatic: %q %q", followUp, next)
	}
	for i := 1; i < dnsPendingPublicationStuckAfter; i++ {
		scheduler.recordAttempt(lease, dnsPendingPublicationStillPending, unknown, true)
	}
	if followUp, next := panel.domainDeletionDNSFollowUp(lease, unknown); followUp != domainDeletionFollowUpStuck || next == "" {
		t.Fatalf("stuck: %q %q", followUp, next)
	}
	for i := 0; i < dnsPendingPublicationFailureLimit; i++ {
		scheduler.recordAttempt(lease, dnsPendingPublicationFailed, "", false)
	}
	if followUp, _ := panel.domainDeletionDNSFollowUp(lease, unknown); followUp != domainDeletionFollowUpStopped {
		t.Fatalf("stopped: %q", followUp)
	}
	other := dnsZoneEngineLease{RequestID: strings.Repeat("c", 32), ZoneName: "sil8.test"}
	if followUp, next := panel.domainDeletionDNSFollowUp(other, transport.DNSPeerPendingJournalUnresolved); followUp != domainDeletionFollowUpPerson || next != "" {
		t.Fatalf("person: %q %q", followUp, next)
	}
	// Before the reconciler has read it, a reason it re-checks by itself is
	// still automatic (without a time).
	if followUp, next := panel.domainDeletionDNSFollowUp(other, unknown); followUp != domainDeletionFollowUpAutomatic || next != "" {
		t.Fatalf("unread: %q %q", followUp, next)
	}
}

func stuckNoticeReasons(t *testing.T, p *Panel) (unseen map[string]int, total int) {
	t.Helper()
	unseen = map[string]int{}
	rows, err := p.db.GetDB().Query(`SELECT json_extract(body, '$.reason'), seen_at IS NULL FROM account_notices WHERE kind = ?`,
		noticeDNSPublicationStuck)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var open bool
		if err := rows.Scan(&reason, &open); err != nil {
			t.Fatal(err)
		}
		total++
		if open {
			unseen[reason]++
		}
	}
	return unseen, total
}

// alpha.133 review (S1): a deletion that was stuck (administrators notified
// that CelikPanel keeps checking) and then needs a person tells them so, in
// the same notification, shown again; nothing says "keeps checking" any
// more. When the administrator acted and the cause changed, the "act now"
// text is retired; when the deletion finished, nothing stays unseen.
func TestStuckThenUnresolvedNotificationSaysTheNewStateOnce(t *testing.T) {
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "turn.example")
	ctx := context.Background()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
		('turn-admin', 'x', 'admin@example.test', 'admin')`); err != nil {
		t.Fatal(err)
	}
	admins, err := administratorIDs(ctx, panel.db.GetDB())
	if err != nil || len(admins) == 0 {
		t.Fatalf("administrators=%v err=%v", admins, err)
	}
	code := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck)
	configurePendingWithCode(t, agent, code)
	pending := true
	recoverOutcome(t, agent, func() bool { return pending }, func() string { return code })
	if err := panel.syncZoneToDNS(ctx, "turn.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "turn.example")
	if err != nil {
		t.Fatal(err)
	}
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	for i := 0; i < dnsPendingPublicationStuckAfter; i++ {
		entry, _ := scheduler.entry(lease.RequestID)
		clock.Advance(entry.NextAt.Sub(clock.Now()))
		panel.reconcilePendingDNSPublicationsOnce(ctx)
	}
	if unseen, total := stuckNoticeReasons(t, panel); unseen[transport.DNSPeerPendingProofTimeout] != len(admins) || total != len(admins) {
		t.Fatalf("stuck: unseen=%v total=%d", unseen, total)
	}
	// The next attempt finds a check record nobody can sort out by itself.
	code = transport.DNSPeerPendingJournalUnresolved
	entry, _ := scheduler.entry(lease.RequestID)
	clock.Advance(entry.NextAt.Sub(clock.Now()))
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	clock.Advance(dnsPendingPublicationOwnerRecheck + time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	unseen, total := stuckNoticeReasons(t, panel)
	if unseen[transport.DNSPeerPendingJournalUnresolved] != len(admins) || unseen[transport.DNSPeerPendingProofTimeout] != 0 || total != len(admins) {
		t.Fatalf("unresolved: unseen=%v total=%d", unseen, total)
	}
	var body string
	if err := panel.db.GetDB().QueryRow(`SELECT body FROM account_notices WHERE kind = ?`, noticeDNSPublicationStuck).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	words := dnsPublicationStuckNoticeWords(audienceAdministrator, parsed)
	if strings.Contains(words.TextEN, "keeps checking") || !strings.Contains(words.TitleEN, "waits for the server administrator") {
		t.Fatalf("revised notice words: %+v", words)
	}
	// Another pass and a panel restart do not show it a third time.
	clock.Advance(dnsPendingPublicationOwnerRecheck + time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	scheduler = useTestPendingScheduler(panel, clock)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	clock.Advance(dnsPendingPublicationOwnerRecheck + time.Second)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if unseen, total := stuckNoticeReasons(t, panel); unseen[transport.DNSPeerPendingJournalUnresolved] != len(admins) || total != len(admins) {
		t.Fatalf("after restart: unseen=%v total=%d", unseen, total)
	}
	// The administrator acted: the person's retry finds a cause CelikPanel
	// re-checks by itself. "Act now" is no longer shown.
	code = transport.DNSPeerPendingJournalUnknown
	panel.attemptPendingDNSPublication(ctx, lease)
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if unseen, _ := stuckNoticeReasons(t, panel); len(unseen) != 0 {
		t.Fatalf("acted: unseen=%v", unseen)
	}
	// Finished: nothing stays unseen.
	pending = false
	entry, _ = scheduler.entry(lease.RequestID)
	clock.Advance(entry.NextAt.Sub(clock.Now()))
	panel.reconcilePendingDNSPublicationsOnce(ctx)
	if leaseCount(t, panel) != 0 {
		t.Fatal("publication did not complete")
	}
	if unseen, total := stuckNoticeReasons(t, panel); len(unseen) != 0 || total != len(admins) {
		t.Fatalf("finished: unseen=%v total=%d", unseen, total)
	}
}

// A stuck deletion whose row's Delete is disabled (the backup catching up)
// is never sent to it.
func TestStuckNoticeNamesDeleteOnlyWhereTheRowOffersIt(t *testing.T) {
	for _, reason := range []string{
		transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingProofTimeout,
		transport.DNSPeerPendingJournalUnknown, transport.DNSPeerPendingJournalUnresolved,
		transport.DNSPeerPendingOwnerEditUnknown, transport.DNSPeerPendingNativeUnknown,
	} {
		words := dnsPublicationStuckNoticeWords(audienceAdministrator, map[string]any{
			"zone": "gone.example", "reason": reason, "delete": true,
		})
		names := strings.Contains(words.TextEN, "More → Delete…") || strings.Contains(words.TextTR, "Diğer → Sil…")
		if names != dnsDeletionRowDeleteEnabled(reason) {
			t.Fatalf("%s: names Delete=%t, row offers it=%t: %s", reason, names, dnsDeletionRowDeleteEnabled(reason), words.TextEN)
		}
		if strings.Contains(words.TextEN+words.TextTR, "Retry this deletion") || strings.Contains(words.TextTR, "Bu silme işlemini tekrar dene") {
			t.Fatalf("%s names a control that does not exist", reason)
		}
	}
	if dnsDeletionRowDeleteEnabled(transport.DNSPeerPendingCatchingUp) {
		t.Fatal("catching up keeps the row's Delete")
	}
}
