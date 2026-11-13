//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnspeerjournal"
	"github.com/alicelik/celikpanel/internal/dnspeerproof"
	"github.com/alicelik/celikpanel/internal/pdnspeerjournal"
	"github.com/alicelik/celikpanel/internal/pdnspeerproof"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 release blocker (lab of 12 Nov 2026): the production prepare
// sequence (nextBINDPeerChallengeAttempt with the real reconciliation,
// Recover, Read and Retire) and the production proof tail (real Publish and
// ConsumeOnce) on the real agent-private journal. Only the network exchange,
// the DNS probes and the ledger file are stand-ins. These tests run only
// where that directory is an empty, private test directory (the isolated
// WSL run bind-mounts one and sets CELIKPANEL_TEST_REAL_PEER_JOURNAL=1);
// never against a live host's journal.

func realPeerJournalDirectory(t *testing.T) string {
	t.Helper()
	if os.Getenv("CELIKPANEL_TEST_REAL_PEER_JOURNAL") != "1" || os.Geteuid() != 0 {
		t.Skip("needs root and an isolated, empty agent-private directory (CELIKPANEL_TEST_REAL_PEER_JOURNAL=1)")
	}
	dir := filepath.Dir(dnspeerjournal.Path)
	if filepath.Dir(pdnspeerjournal.Path) != dir {
		t.Fatal("the two journals moved apart")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Skipf("the agent-private directory is not an empty test directory: %d entries, %v", len(entries), err)
	}
	t.Cleanup(func() {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	})
	return dir
}

func putRootFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(path, 0, 0); err != nil {
		t.Fatal(err)
	}
}

func directoryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func heldOnlyChecks() *nativePeerCurrentChecks {
	return &nativePeerCurrentChecks{
		attempt:    func() error { return nil },
		enrollment: func() error { return nil },
		local:      func(context.Context) error { return nil },
	}
}

// runRealBINDPeerProof runs one attempt of the current deletion (plan) on
// the real journal: prepare's journal part, then the minted challenge
// through the proof tail with an accepted answer.
func runRealBINDPeerProof(t *testing.T, previous dnspeerjournal.RecordV1, ledger serviceMutationLedger,
	plan dnsV3PrimaryPropagationPlan, checks *nativePeerCurrentChecks, budget dnsPeerProofBudget,
) error {
	t.Helper()
	return runRealBINDPeerProofAt(t, previous, ledger, plan, 1, checks, budget)
}

// runRealBINDPeerProofAt is runRealBINDPeerProof for the deletion's ledger
// attempt ledgerAttempt.
func runRealBINDPeerProofAt(t *testing.T, previous dnspeerjournal.RecordV1, ledger serviceMutationLedger,
	plan dnsV3PrimaryPropagationPlan, ledgerAttempt uint64, checks *nativePeerCurrentChecks, budget dnsPeerProofBudget,
) error {
	t.Helper()
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	run := newDNSPeerProofRun(context.Background(), budget, plan.Changed.Domain, "BIND")
	var next uint64
	if err := run.step(dnsPeerProofStepPrepare, func(ctx context.Context) error {
		var err error
		next, err = nextBINDPeerChallengeAttempt(ctx, checks, func(verify func() error) error {
			return reconcileHistoricalBINDPeerChallengeAt(plan, verify, load,
				dnspeerjournal.Recover, dnspeerjournal.Read, dnspeerjournal.Retire)
		}, dnspeerjournal.Read)
		return err
	}); err != nil {
		return err
	}
	request := previous.Request
	request.MutationRequestID, request.MutationOwnerID = plan.Operation.RequestID, plan.Operation.OwnerID
	request.DeletedZone, request.DeletionQualifier = plan.Changed.Domain, plan.Operation.Qualifier
	request.Attempt = next
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	request.Nonce = hex.EncodeToString(nonce[:])
	request.IssuedAtUnix, request.ExpiresAtUnix = time.Now().Add(-time.Second).Unix(), time.Now().Add(30*time.Second).Unix()
	digest, err := dnspeerproof.RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	enrollment := previous.EnrollmentSHA256
	return runNativePeerProofTail(run, checks, nativePeerProofTail{
		publish: func(verify func() error) error {
			return dnspeerjournal.Publish(request, nil, enrollment, ledgerAttempt, verify)
		},
		recheckEnrollment: func() error { return nil },
		inspect:           func(context.Context) error { return nil },
		afterInspection:   func(context.Context, func() error) error { return nil },
		verifyAnswer:      func(consume func(string) error) error { return consume(digest) },
		consume: func(digest string, verify func() error) error {
			return dnspeerjournal.ConsumeOnce(request, enrollment, digest, ledgerAttempt, verify)
		},
	})
}

// finishRealBINDDeletion is what follows a verified proof: the terminal
// ledger result, then the retirement of this operation's consumed challenge
// (retireTerminalBINDPeerChallenge; its locks and ledger are stand-ins).
func finishRealBINDDeletion(t *testing.T, dir string, plan dnsV3PrimaryPropagationPlan) {
	t.Helper()
	stored, err := dnspeerjournal.Read()
	if err != nil || stored.State != dnspeerjournal.StateConsumed ||
		stored.Request.MutationRequestID != plan.Operation.RequestID || stored.Request.DeletedZone != plan.Changed.Domain {
		t.Fatalf("after the proof the journal holds %+v (%v)", stored, err)
	}
	if err := dnspeerjournal.Retire(stored.Request, stored.EnrollmentSHA256, stored.RequestSHA256,
		stored.LedgerAttempt, func() error { return nil }); err != nil {
		t.Fatalf("terminal retirement: %v", err)
	}
	if names := directoryNames(t, dir); len(names) != 0 {
		t.Fatalf("left behind in the agent-private directory: %v", names)
	}
}

func encodedRecord(t *testing.T, record dnspeerjournal.RecordV1) []byte {
	t.Helper()
	raw, err := dnspeerjournal.Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The lab state: alpha.132 stopped retiring the consumed challenge of the
// finished deletion of another zone after its exchange. 455b31173 refused it
// on every attempt (dns_peer_journal_unknown, 2-5 ms). One attempt now
// finishes that retirement, mints attempt 1 of this deletion, completes the
// proof, and the deletion's own retirement leaves the directory empty.
func TestLabTornRetirementIsRepairedAndTheDeletionProofCompletes(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	logs := captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	putRootFile(t, dnspeerjournal.Path, nil)
	putRootFile(t, filepath.Join(dir, ".dns-peer-retired-1d291fcb0d73e582d8c9ae595505fb98"), encodedRecord(t, previous))

	// What 455b31173 did with it: no recovery step, unreadable for ever.
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	err := heldOnlyChecks().prepareJournal(context.Background(), func(verify func() error) error {
		return reconcileHistoricalBINDPeerChallengeAt(plan, verify, load, alpha132NoRecover,
			dnspeerjournal.Read, dnspeerjournal.Retire)
	})
	if pendingDNSPeerCode(err) != transport.DNSPeerPendingJournalUnknown || len(directoryNames(t, dir)) != 2 {
		t.Fatalf("the 455b31173 sequence: code=%q files=%v", pendingDNSPeerCode(err), directoryNames(t, dir))
	}

	if err := runRealBINDPeerProof(t, previous, ledger, plan, heldOnlyChecks(), dnsPeerProofSteps); err != nil {
		t.Fatalf("the attempt after the update: %v (%q)", err, pendingDNSPeerCode(err))
	}
	if stored, err := dnspeerjournal.Read(); err != nil || stored.Request.Attempt != 1 {
		t.Fatalf("the deletion's own challenge: %+v %v", stored, err)
	}
	if !strings.Contains(logs.String(), "finished an interrupted change of its check record (retirement_completed)") {
		t.Fatalf("repair not logged:\n%s", logs)
	}
	finishRealBINDDeletion(t, dir, plan)
}

// Frankfurt (13 Nov 2026, read-only facts): the consumed challenge of a
// deletion finished on 6 Nov is intact. The first attempt retires it and the
// deletion completes.
func TestIntactEarlierChallengeIsRetiredAndTheDeletionProofCompletes(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	logs := captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	putRootFile(t, dnspeerjournal.Path, encodedRecord(t, previous))
	if err := runRealBINDPeerProof(t, previous, ledger, plan, heldOnlyChecks(), dnsPeerProofSteps); err != nil {
		t.Fatalf("attempt: %v (%q)", err, pendingDNSPeerCode(err))
	}
	if !strings.Contains(logs.String(), "retired the challenge of an earlier finished deletion (old.example.test, ledger attempt 1)") {
		t.Fatalf("retirement not logged:\n%s", logs)
	}
	finishRealBINDDeletion(t, dir, plan)
}

// Every state an alpha.132 retirement leaves when it stops after one of its
// file operations: the next attempt completes the deletion's proof.
func TestEveryStateOfAnInterruptedAlpha132RetirementCompletes(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	raw := encodedRecord(t, previous)
	retired := filepath.Join(dir, ".dns-peer-retired-0123456789abcdef0123456789abcdef")
	states := map[string]func(){
		"tombstone created": func() { putRootFile(t, dnspeerjournal.Path, raw); putRootFile(t, retired, nil) },
		"exchanged":         func() { putRootFile(t, dnspeerjournal.Path, nil); putRootFile(t, retired, raw) },
		"journal unlinked":  func() { putRootFile(t, retired, raw) },
		"retired unlinked":  func() {},
	}
	for name, setup := range states {
		t.Run(name, func(t *testing.T) {
			setup()
			if err := runRealBINDPeerProof(t, previous, ledger, plan, heldOnlyChecks(), dnsPeerProofSteps); err != nil {
				t.Fatalf("attempt: %v (%q)", err, pendingDNSPeerCode(err))
			}
			finishRealBINDDeletion(t, dir, plan)
		})
	}
}

// The repair is journal work under the held checks: a prepare whose full
// local recheck then runs out of time keeps it, and the next attempt
// completes without repeating it.
func TestRepairSurvivesAPrepareThatRunsOutOfTime(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	logs := captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	putRootFile(t, dnspeerjournal.Path, nil)
	putRootFile(t, filepath.Join(dir, ".dns-peer-retired-00"), encodedRecord(t, previous))
	budget := scaledPeerProofBudget()
	slow := heldOnlyChecks()
	slow.local = func(ctx context.Context) error {
		if err := sleepUnder(ctx, budget.prepare*2); err != nil {
			return dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckActiveEngine, "systemctl show failed: signal: killed")
		}
		return nil
	}
	err := runRealBINDPeerProof(t, previous, ledger, plan, slow, budget)
	if code := pendingDNSPeerCode(err); code != "dns_peer_proof_timeout:local_check" {
		t.Fatalf("slow attempt: %q (%v)", code, err)
	}
	if names := directoryNames(t, dir); len(names) != 0 {
		t.Fatalf("the repair did not survive the timed-out attempt: %v", names)
	}
	if err := runRealBINDPeerProof(t, previous, ledger, plan, heldOnlyChecks(), dnsPeerProofSteps); err != nil {
		t.Fatalf("next attempt: %v", err)
	}
	if strings.Count(logs.String(), "finished an interrupted change") != 1 {
		t.Fatalf("repair ran other than once:\n%s", logs)
	}
	finishRealBINDDeletion(t, dir, plan)
}

// What the Agent cannot prove keeps refusing, untouched, with its own code;
// no challenge is minted.
func TestUnprovableJournalStatesStopAsUnresolved(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	raw := encodedRecord(t, previous)
	notFinished := cloneServiceMutationLedger(ledger)
	old := notFinished.Jobs[previous.Request.MutationRequestID]
	old.Status = serviceMutationStatusPending
	old.Phase, _ = formatDNSZoneSyncV3Phase(dnsZoneSyncV3PropagationPending, old.RequestID, old.Target, old.PackageName)
	old.FinishedAt = time.Time{}
	cases := []struct {
		name   string
		ledger serviceMutationLedger
		files  map[string][]byte
	}{
		{"two retired stages", ledger, map[string][]byte{"dns-peer-challenge-v1.json": nil,
			".dns-peer-retired-01": raw, ".dns-peer-retired-02": raw}},
		{"retired stage that is not a record", ledger, map[string][]byte{"dns-peer-challenge-v1.json": nil,
			".dns-peer-retired-01": []byte("owner text")}},
		{"empty journal alone", ledger, map[string][]byte{"dns-peer-challenge-v1.json": nil}},
		{"torn retirement of an unfinished operation", notFinished, map[string][]byte{"dns-peer-challenge-v1.json": nil,
			".dns-peer-retired-01": raw}},
		{"another operation's unfinished challenge", notFinished, map[string][]byte{"dns-peer-challenge-v1.json": raw}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for name, content := range tc.files {
				putRootFile(t, filepath.Join(dir, name), content)
			}
			before := strings.Join(directoryNames(t, dir), " ")
			err := runRealBINDPeerProof(t, previous, tc.ledger, plan, heldOnlyChecks(), dnsPeerProofSteps)
			if code := pendingDNSPeerCode(err); code != transport.DNSPeerPendingJournalUnresolved {
				t.Fatalf("code %q (%v)", code, err)
			}
			if after := strings.Join(directoryNames(t, dir), " "); after != before {
				t.Fatalf("changed: %s -> %s", before, after)
			}
			for name := range tc.files {
				_ = os.Remove(filepath.Join(dir, name))
			}
		})
	}
}

// The PowerDNS secondary's journal: the same lab state, through its own
// prepare sequence and reconciliation.
func TestLabTornRetirementIsRepairedInThePowerDNSJournal(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	captureAgentLog(t)
	base, ledger, plan := historicalBINDPeerChallengeFixture(t)
	request := pdnspeerproof.RequestV1(base.Request)
	request.Schema = pdnspeerproof.RequestSchemaV1
	digest, err := pdnspeerproof.RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	previous := pdnspeerjournal.RecordV1{
		Schema: pdnspeerjournal.SchemaV1, State: pdnspeerjournal.StateConsumed,
		LedgerAttempt: base.LedgerAttempt, EnrollmentSHA256: base.EnrollmentSHA256,
		RequestSHA256: digest, Request: request,
	}
	raw, err := pdnspeerjournal.Encode(previous)
	if err != nil {
		t.Fatal(err)
	}
	putRootFile(t, pdnspeerjournal.Path, nil)
	putRootFile(t, filepath.Join(dir, ".pdns-peer-retired-00"), raw)
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	next, err := nextPDNSPeerChallengeAttempt(context.Background(), heldOnlyChecks(), func(verify func() error) error {
		return reconcileHistoricalPDNSPeerChallengeAt(plan, verify, load,
			pdnspeerjournal.Recover, pdnspeerjournal.Read, pdnspeerjournal.Retire)
	}, pdnspeerjournal.Read)
	if err != nil || next != 1 {
		t.Fatalf("next=%d err=%v (%q)", next, err, pendingDNSPeerCode(err))
	}
	if names := directoryNames(t, dir); len(names) != 0 {
		t.Fatalf("left behind: %v", names)
	}
	// Two stages stay unresolved here too.
	putRootFile(t, pdnspeerjournal.Path, nil)
	putRootFile(t, filepath.Join(dir, ".pdns-peer-retired-01"), raw)
	putRootFile(t, filepath.Join(dir, ".pdns-peer-retired-02"), raw)
	_, err = nextPDNSPeerChallengeAttempt(context.Background(), heldOnlyChecks(), func(verify func() error) error {
		return reconcileHistoricalPDNSPeerChallengeAt(plan, verify, load,
			pdnspeerjournal.Recover, pdnspeerjournal.Read, pdnspeerjournal.Retire)
	}, pdnspeerjournal.Read)
	if pendingDNSPeerCode(err) != transport.DNSPeerPendingJournalUnresolved || len(directoryNames(t, dir)) != 3 {
		t.Fatalf("code %q files %v", pendingDNSPeerCode(err), directoryNames(t, dir))
	}
}

// The journal operations take no context: nothing in them can be cut by a
// step's deadline. Their only verifier in the prepare step is the held one,
// which runs no command (a canceled context does not reach it).
func TestPrepareJournalWorkIgnoresAnExpiredStepContext(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	captureAgentLog(t)
	previous, ledger, plan := historicalBINDPeerChallengeFixture(t)
	putRootFile(t, dnspeerjournal.Path, nil)
	putRootFile(t, filepath.Join(dir, ".dns-peer-retired-00"), encodedRecord(t, previous))
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	load := func() (serviceMutationLedger, error) { return ledger, nil }
	checks := heldOnlyChecks()
	checks.local = func(ctx context.Context) error { return ctx.Err() }
	_, err := nextBINDPeerChallengeAttempt(expired, checks, func(verify func() error) error {
		return reconcileHistoricalBINDPeerChallengeAt(plan, verify, load,
			dnspeerjournal.Recover, dnspeerjournal.Read, dnspeerjournal.Retire)
	}, dnspeerjournal.Read)
	if !isDNSPeerProofDeadline(err) {
		t.Fatalf("expected the local recheck's deadline, got %v", err)
	}
	if names := directoryNames(t, dir); len(names) != 0 {
		t.Fatalf("journal work was cut short: %v", names)
	}
}

// ownerServerShapedFixture is the owner's PowerDNS primary as read (read-only)
// on 13 Nov 2026: the journal holds the consumed record of ANOTHER zone's
// finished deletion with request attempt 4 and ledger attempt 183; the
// ledger keeps that request as a succeeded dns_zone_sync published at
// attempt 183; this deletion's own job is pending (propagation-pending,
// attempt 162, error code dns_peer_proof_timeout).
func ownerServerShapedFixture(t *testing.T) (dnspeerjournal.RecordV1, serviceMutationLedger, dnsV3PrimaryPropagationPlan, uint64) {
	t.Helper()
	base, ledger, plan := historicalBINDPeerChallengeFixture(t)
	request := base.Request
	request.Attempt = 4
	request.DeletedZone = "earlier-shop.example.test"
	digest, err := dnspeerproof.RequestSHA256(request)
	if err != nil {
		t.Fatal(err)
	}
	previous := dnspeerjournal.RecordV1{
		Schema: dnspeerjournal.SchemaV1, State: dnspeerjournal.StateConsumed,
		LedgerAttempt: 183, EnrollmentSHA256: base.EnrollmentSHA256,
		RequestSHA256: digest, Request: request,
	}
	if err := previous.Validate(); err != nil {
		t.Fatal(err)
	}
	ledger = cloneServiceMutationLedger(ledger)
	old := ledger.Jobs[request.MutationRequestID]
	old.Target, old.Attempt = request.DeletedZone, 183
	if old.Phase, err = formatDNSZoneSyncV3PublishedPhase(old.RequestID, old.Target, old.PackageName); err != nil {
		t.Fatal(err)
	}
	own := ledger.Jobs[plan.Operation.RequestID]
	own.Status, own.Attempt, own.ErrorCode = serviceMutationStatusPending, 162, transport.DNSPeerPendingProofTimeout
	if own.Phase, err = formatDNSZoneSyncV3Phase(dnsZoneSyncV3PropagationPending, own.RequestID, own.Target, own.PackageName); err != nil {
		t.Fatal(err)
	}
	own.LeaseExpiresAt = time.Time{}
	own.FinishedAt = own.UpdatedAt
	ledger.ActiveRequestID = ""
	if err := validateServiceMutationLedger(&ledger); err != nil {
		t.Fatalf("owner-shaped ledger: %v", err)
	}
	return previous, ledger, plan, 162
}

// The owner's server in both states it can be in when he updates: the
// earlier deletion's record intact, or torn by an alpha.132 retirement. The
// first attempt retires or repairs it and the deletion's proof completes,
// with its own challenge at attempt 1 under ledger attempt 162.
func TestOwnerServerShapedStatesCompleteOnTheFirstAttempt(t *testing.T) {
	dir := realPeerJournalDirectory(t)
	captureAgentLog(t)
	previous, ledger, plan, attempt := ownerServerShapedFixture(t)
	raw := encodedRecord(t, previous)
	for name, setup := range map[string]func(){
		"record intact": func() { putRootFile(t, dnspeerjournal.Path, raw) },
		"record torn by alpha.132": func() {
			putRootFile(t, dnspeerjournal.Path, nil)
			putRootFile(t, filepath.Join(dir, ".dns-peer-retired-1d291fcb0d73e582d8c9ae595505fb98"), raw)
		},
	} {
		t.Run(name, func(t *testing.T) {
			setup()
			if err := runRealBINDPeerProofAt(t, previous, ledger, plan, attempt, heldOnlyChecks(), dnsPeerProofSteps); err != nil {
				t.Fatalf("first attempt: %v (%q)", err, pendingDNSPeerCode(err))
			}
			stored, err := dnspeerjournal.Read()
			if err != nil || stored.Request.Attempt != 1 || stored.LedgerAttempt != attempt {
				t.Fatalf("the deletion's own challenge: %+v %v", stored, err)
			}
			finishRealBINDDeletion(t, dir, plan)
		})
	}
}
