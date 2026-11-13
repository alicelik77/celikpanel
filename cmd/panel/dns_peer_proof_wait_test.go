package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5 (lab w9, 13 Nov 2026): a backup DNS server that was
// unreachable for about 45 seconds turned one of nine re-checks into
// dns_peer_inspection_unknown, which the Panel did not re-check, so the
// deletion stayed waiting after the backup answered again until a person
// pressed Delete. Every reason a re-check can change is now re-checked; only
// dns_peer_journal_unresolved stops the re-checks.

// runPendingSequence lets the reconciler run, with no press, while the
// scripted Agent answers each re-check with the next code; "" completes it.
// It returns the follow-up the deletion status reports after each re-check.
func runPendingSequence(t *testing.T, zone string, codes []string) (followUps []string, recovers int) {
	t.Helper()
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, zone)
	ctx := context.Background()
	configurePendingWithCode(t, agent, codes[0])
	step := 0
	current := func() string { return codes[step] }
	recoverOutcome(t, agent, func() bool { return current() != "" }, current)
	base := agent.recoverHook
	agent.recoverHook = func(request transport.RecoverDNSZoneV3Request, response *transport.RecoverDNSZoneV3Response) error {
		recovers++
		err := base(request, response)
		if step < len(codes)-1 {
			step++
		}
		return err
	}
	if err := panel.syncZoneToDNS(ctx, zone, false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), zone)
	if err != nil {
		t.Fatal(err)
	}
	// The publication's own request resumed its operation at most once.
	recovers, step = 0, 1
	panel.reconcilePendingDNSPublicationsOnce(ctx) // discovery
	for i := 0; i < 3*len(codes) && leaseCount(t, panel) != 0; i++ {
		entry, ok := scheduler.entry(lease.RequestID)
		if !ok {
			break
		}
		clock.Advance(entry.NextAt.Sub(clock.Now()))
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		if entry, ok := scheduler.entry(lease.RequestID); ok {
			followUp, _ := panel.domainDeletionDNSFollowUp(lease, entry.Code)
			followUps = append(followUps, followUp)
		}
	}
	if leaseCount(t, panel) != 0 {
		t.Fatalf("%s did not finish by itself; follow-ups %v, re-checks %d", zone, followUps, recovers)
	}
	return followUps, recovers
}

// The w9 sequence: the backup does not answer for three attempts (each
// ending at the exchange's bound; the Agent now reports every shape of that
// as dns_peer_proof_timeout:peer_exchange, and the alpha.133 race's
// dns_peer_inspection_unknown is re-checked too), then it answers and the
// deletion finishes with no press.
func TestBackupThatDoesNotAnswerIsRecheckedUntilTheDeletionFinishes(t *testing.T) {
	noAnswer := transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange)
	followUps, recovers := runPendingSequence(t, "w9.example", []string{
		noAnswer, noAnswer, transport.DNSPeerPendingInspectionUnknown, noAnswer, "",
	})
	if recovers != 4 {
		t.Fatalf("re-checks %d, want 4 (three waits, then the one that finishes)", recovers)
	}
	for _, followUp := range followUps {
		if followUp != domainDeletionFollowUpAutomatic {
			t.Fatalf("follow-ups %v: every wait must be automatic", followUps)
		}
	}
}

// A backup that answers and refuses keeps its own code, is still re-checked
// (and becomes "stuck" after the same number in a row, with one
// notification), and the deletion finishes once it stops refusing.
func TestBackupThatRefusesIsRecheckedAndFinishesOnceItStops(t *testing.T) {
	refused := transport.DNSPeerPendingInspectionUnknownWithDetail(transport.DNSPeerInspectorReasonNamedUnavailable)
	codes := []string{}
	for i := 0; i < dnsPendingPublicationStuckAfter+2; i++ {
		codes = append(codes, refused)
	}
	codes = append(codes, "")
	followUps, recovers := runPendingSequence(t, "refusing.example", codes)
	if recovers != len(codes)-1 {
		t.Fatalf("re-checks %d, want %d", recovers, len(codes)-1)
	}
	if followUps[0] != domainDeletionFollowUpAutomatic ||
		followUps[dnsPendingPublicationStuckAfter-1] != domainDeletionFollowUpStuck {
		t.Fatalf("follow-ups %v", followUps)
	}
	for _, followUp := range followUps {
		if followUp == domainDeletionFollowUpPerson || followUp == domainDeletionFollowUpStopped {
			t.Fatalf("follow-ups %v: a refusing backup never stops the checks", followUps)
		}
	}
}

// Every reviewed DNS proof reason is re-checked except an unresolved check
// record and an owner edit of this server; its deletion status says so.
func TestOnlyUnresolvedRecordsAndOwnerEditsStopTheRechecks(t *testing.T) {
	panel := &Panel{}
	lease := dnsZoneEngineLease{RequestID: strings.Repeat("d", 32), ZoneName: "table.example"}
	for _, code := range []string{
		transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingProofTimeout,
		transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutLocalCheck),
		transport.DNSPeerPendingProofTimeoutWithDetail(transport.DNSPeerProofTimeoutPeerExchange),
		transport.DNSPeerPendingJournalUnknown, transport.DNSPeerPendingEnrollmentRequired,
		transport.DNSPeerPendingEnrollmentChanged, transport.DNSPeerPendingInspectionUnknown,
		transport.DNSPeerPendingInspectionUnknownWithDetail(transport.DNSPeerInspectorReasonPolicy),
		transport.DNSPeerPendingNativeUnknown,
		transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckTransferObserved),
		transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckZoneAnswered),
		transport.DNSPeerPendingCatalogTransferRefused, transport.DNSPeerPendingProofInternal,
	} {
		if !dnsPeerPendingRetriesAutomatically(code) {
			t.Fatalf("%s is not re-checked", code)
		}
		if followUp, _ := panel.domainDeletionDNSFollowUp(lease, code); followUp != domainDeletionFollowUpAutomatic {
			t.Fatalf("%s follow-up %q", code, followUp)
		}
	}
	for _, code := range localOwnerEditCodes() {
		if followUp, next := panel.domainDeletionDNSFollowUp(lease, code); followUp != domainDeletionFollowUpPerson || next != "" {
			t.Fatalf("%s follow-up %q %q", code, followUp, next)
		}
	}
}

// localOwnerEditCodes are the owner-edit codes found on this server or in the
// pair's identity, plus the plain code and an unresolved check record: the
// states that stop the automatic re-checks (alpha.133 round 5 review, S1).
func localOwnerEditCodes() []string {
	codes := []string{transport.DNSPeerPendingJournalUnresolved, transport.DNSPeerPendingOwnerEditUnknown}
	for _, check := range []string{
		transport.DNSPeerOwnerEditCheckOperationAttempt, transport.DNSPeerOwnerEditCheckEngineState,
		transport.DNSPeerOwnerEditCheckActiveEngine, transport.DNSPeerOwnerEditCheckNativeBinding,
		transport.DNSPeerOwnerEditCheckDeletionReceipt, transport.DNSPeerOwnerEditCheckProducerCatalog,
		transport.DNSPeerOwnerEditCheckCatalogProbe, transport.DNSPeerOwnerEditCheckAuthority,
	} {
		codes = append(codes, transport.DNSPeerPendingOwnerEditUnknownWithDetail(check))
	}
	return codes
}

// S1 (review of round 5): a deletion waiting because this server's DNS may be
// changed by hand is never re-checked by the Panel's own timer (its recovery
// pre-checks can hold every Agent change), also on the first scan after a
// restart with the code already stored; the administrators get exactly one
// notification, the status says a person acts, and a press continues it.
func TestOwnerEditOfThisServerIsNotRecheckedAndNotifiesOnce(t *testing.T) {
	for _, code := range localOwnerEditCodes()[1:] {
		t.Run(code, func(t *testing.T) {
			panel, agent, clock, scheduler := newPendingPublicationPanel(t, "edited.example")
			ctx := context.Background()
			if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
				('edit-admin', 'x', 'admin@example.test', 'admin')`); err != nil {
				t.Fatal(err)
			}
			admins, err := administratorIDs(ctx, panel.db.GetDB())
			if err != nil || len(admins) == 0 {
				t.Fatalf("administrators=%v err=%v", admins, err)
			}
			configurePendingWithCode(t, agent, code)
			pending := true
			recoverOutcome(t, agent, func() bool { return pending }, func() string { return code })
			if err := panel.syncZoneToDNS(ctx, "edited.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
				t.Fatalf("initial err=%v", err)
			}
			lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "edited.example")
			if err != nil {
				t.Fatal(err)
			}
			_, before := pendingAgentCalls(agent)
			scan := func(n int) {
				for i := 0; i < n; i++ {
					panel.reconcilePendingDNSPublicationsOnce(ctx)
					clock.Advance(dnsPendingPublicationMaxDelay + time.Second)
				}
			}
			scan(6)
			// A panel restart: the code is read from the stored job at once.
			scheduler = useTestPendingScheduler(panel, clock)
			scan(6)
			if _, after := pendingAgentCalls(agent); len(after) != len(before) {
				t.Fatalf("re-checked %d time(s) by the Panel's own timer", len(after)-len(before))
			}
			if entry, _ := scheduler.entry(lease.RequestID); entry.Automatic || entry.Code != code {
				t.Fatalf("entry=%+v", entry)
			}
			if followUp, next := panel.domainDeletionDNSFollowUp(lease, code); followUp != domainDeletionFollowUpPerson || next != "" {
				t.Fatalf("follow-up %q %q", followUp, next)
			}
			unseen, total := stuckNoticeRows(t, panel)
			if unseen != len(admins) || total != len(admins) {
				t.Fatalf("notices unseen=%d total=%d, want %d", unseen, total, len(admins))
			}
			// The administrator finished the change and pressed again (the
			// same publication's own request): the same operation continues
			// and completes.
			pending = false
			if err := panel.syncZoneToDNS(ctx, "edited.example", false); err != nil {
				t.Fatalf("press: %v", err)
			}
			if leaseCount(t, panel) != 0 {
				t.Fatal("the press did not continue the deletion")
			}
			panel.reconcilePendingDNSPublicationsOnce(ctx)
			if unseen, _ := stuckNoticeRows(t, panel); unseen != 0 {
				t.Fatalf("finished: %d notices still unseen", unseen)
			}
		})
	}
}

// The notification's button opens the page its text names: a deletion's
// waiting notice is on Domains; another DNS change's state on the DNS page.
func TestDeletionNotificationOpensDomains(t *testing.T) {
	panel := newDNSPanelForTest(t)
	ctx := context.Background()
	if _, err := panel.db.GetDB().Exec(`INSERT INTO users (username, password_hash, email, role) VALUES
		('link-admin', 'x', 'admin@example.test', 'admin')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		action, reason, want string
	}{
		{"delete", transport.DNSPeerPendingJournalUnresolved, "/domains"},
		{"delete", transport.DNSPeerPendingInspectionUnknown, "/domains"},
		{"upsert", transport.DNSPeerPendingCatchingUp, "/dns-infrastructure"},
	} {
		lease := dnsZoneEngineLease{RequestID: strings.Repeat(tc.action[:1], 32), ZoneName: tc.action + ".example", DesiredAction: tc.action}
		panel.notifyDNSPublicationAdministrators(ctx, lease, dnsPendingPublicationEntry{Lease: lease, Code: tc.reason, Repeats: 6}, true)
		var fix, body string
		if err := panel.db.GetDB().QueryRow(`SELECT fix_path, body FROM account_notices WHERE kind = ? AND period = ?`,
			noticeDNSPublicationStuck, lease.RequestID).Scan(&fix, &body); err != nil {
			t.Fatal(err)
		}
		if fix != tc.want {
			t.Fatalf("%s %s: button opens %q, want %q", tc.action, tc.reason, fix, tc.want)
		}
		words := dnsPublicationStuckNoticeWords(audienceAdministrator, map[string]any{
			"zone": lease.ZoneName, "reason": tc.reason, "delete": tc.action == "delete",
		})
		if tc.want == "/domains" && (!strings.Contains(words.TextEN, "On Domains") || strings.Contains(words.TextEN, "Server → DNS and peer servers")) {
			t.Fatalf("%s: text does not send the reader to Domains: %s", tc.reason, words.TextEN)
		}
		if tc.want == "/dns-infrastructure" && strings.Contains(words.TextEN, "On Domains, the waiting notice") {
			t.Fatalf("%s: a DNS change's text sends the reader to Domains: %s", tc.reason, words.TextEN)
		}
	}
}

// Re-review of round 5 (W3): when the reconciler finds the exact job still
// running (for example the Agent's own startup recovery), it waits for it;
// if it ends pending with a code that stops the automatic re-checks, the
// reconciler does not run the recovery itself. A person's press still does.
func TestReconcilerDoesNotRecoverAStopCodeAfterWaitingForTheJob(t *testing.T) {
	code := transport.DNSPeerPendingOwnerEditUnknownWithDetail(transport.DNSPeerOwnerEditCheckProducerCatalog)
	panel, agent, clock, scheduler := newPendingPublicationPanel(t, "running.example")
	ctx := context.Background()
	configurePendingWithCode(t, agent, code)
	recoverOutcome(t, agent, func() bool { return true }, func() string { return code })
	if err := panel.syncZoneToDNS(ctx, "running.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("initial err=%v", err)
	}
	lease, err := readDNSZoneEngineLease(ctx, panel.db.GetDB(), "running.example")
	if err != nil {
		t.Fatal(err)
	}
	_, before := pendingAgentCalls(agent)
	// A panel restart while the Agent's own recovery of the job runs: the
	// first status reads see it running, then it ends pending.
	scheduler = useTestPendingScheduler(panel, clock)
	reads := 0
	agent.statusHook = func(_ string, job *ServiceOperationMutationJob) {
		reads++
		if job == nil {
			return
		}
		if reads <= 3 {
			job.Status = agentMutationRunning
		} else {
			job.Status = agentMutationPending
		}
	}
	for i := 0; i < 3; i++ {
		panel.reconcilePendingDNSPublicationsOnce(ctx)
		clock.Advance(dnsPendingPublicationMaxDelay + time.Second)
	}
	agent.statusHook = nil
	if reads <= 3 {
		t.Fatalf("the reconciler never waited for the running job (%d status reads)", reads)
	}
	if _, after := pendingAgentCalls(agent); len(after) != len(before) {
		t.Fatalf("the reconciler ran the recovery of %s %d time(s)", code, len(after)-len(before))
	}
	if entry, _ := scheduler.entry(lease.RequestID); entry.Automatic || entry.Code != code {
		t.Fatalf("entry=%+v", entry)
	}
	// A person's press runs it.
	if err := panel.syncZoneToDNS(ctx, "running.example", false); !errors.Is(err, errDNSZoneV3PropagationDeferred) {
		t.Fatalf("press err=%v", err)
	}
	if _, after := pendingAgentCalls(agent); len(after) != len(before)+1 {
		t.Fatalf("the press ran the recovery %d time(s)", len(after)-len(before))
	}
}

// Re-review of round 5 (W1): the backup that stopped answering right after
// the exchange is dns_peer_catching_up (Agent test
// TestNativePeerAfterInspectionNamesTheCheck); the Panel re-checks it and the
// deletion finishes once the backup is back.
func TestBackupGoneAfterTheExchangeIsRecheckedUntilItReturns(t *testing.T) {
	if _, recovers := runPendingSequence(t, "gone-after.example", []string{
		transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingCatchingUp, transport.DNSPeerPendingCatchingUp, "",
	}); recovers != 3 {
		t.Fatalf("re-checks %d, want 3", recovers)
	}
}
