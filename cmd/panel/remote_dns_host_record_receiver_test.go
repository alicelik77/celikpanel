package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// alpha.139: the paired main DNS server answers a host record request
// truthfully and in time. In the alpha.135 lab a secondary's request was
// answered CONFLICT for a record that did not exist (a customer zone under
// the infrastructure zone refused the whole zone) and, behind a domain
// deletion holding the DNS locks for ~29 s, "unknown" with nothing in the
// primary's journal.

func seedHostRecordChildZone(t *testing.T, p *Panel, name string) {
	t.Helper()
	if _, err := p.db.GetDB().Exec(`INSERT INTO pdns_domains(name,type) VALUES(?,'MASTER')`, name); err != nil {
		t.Fatal(err)
	}
}

func hostRecordLedgerCount(t *testing.T, p *Panel) int {
	t.Helper()
	var n int
	if err := p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM panel_settings WHERE key LIKE 'remote_dns_host_record:%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// hostRecordRefusalAudits counts the receiver's audit rows that name code.
func hostRecordRefusalAudits(t *testing.T, p *Panel, code string) int {
	t.Helper()
	rows, err := p.db.GetDB().Query(`SELECT COALESCE(details,'') FROM audit_logs WHERE action='dns.remote.host_record.refused'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var raw string
		rows.Scan(&raw)
		var details map[string]any
		if json.Unmarshal([]byte(raw), &details) == nil && details["code"] == code {
			n++
		}
	}
	return n
}

func (f *hostRecordFixture) secret(t *testing.T) string {
	t.Helper()
	c, err := f.origin.readRemoteDNSConnection(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	return c.ID + "." + c.credential
}

// direct sends one host record request to the receiver's production handler
// and keeps the whole answer (status, headers, body).
func (f *hostRecordFixture) direct(ctx context.Context, secret, name, typ, content string) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(remoteDNSHostRecordRequest{Name: name, Type: typ, Content: content})
	r := httptest.NewRequest(http.MethodPost, "https://dns.remote.test:2083"+remoteDNSHostRecordPath, bytes.NewReader(raw)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	f.receiver.handleRemoteDNSMachine(w, r)
	return w
}

func shortenRemoteDNSReceiverLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	old := remoteDNSReceiverLockWait
	remoteDNSReceiverLockWait = wait
	t.Cleanup(func() { remoteDNSReceiverLockWait = old })
}

func TestRemoteDNSHostRecordBesideACustomerZoneUnderTheInfrastructureZone(t *testing.T) {
	f := newHostRecordFixture(t)
	// The lab primary hosted customer zones (h4.dr.test) beside its
	// infrastructure zone dr.test.
	seedHostRecordChildZone(t, f.receiver, "shop.example.test")
	answer, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	if err != nil || !answer.Applied || answer.Result != "created" || answer.Zone != "example.test" {
		t.Fatalf("a sibling customer zone refused the record: answer=%+v err=%v", answer, err)
	}
	// A name the customer zone answers for is not the infrastructure zone's.
	for _, name := range []string{"www.shop.example.test", "shop.example.test", "a.b.shop.example.test"} {
		_, err := f.ask(t, name, "A", hostRecordPeerIP)
		if refusalCode(err) != errCodeRemoteDNSHostRecordOutsideZone {
			t.Fatalf("%s: err=%v code=%q", name, err, refusalCode(err))
		}
		if got := f.addresses(t, name); len(got) != 0 {
			t.Fatalf("%s written: %v", name, got)
		}
	}
	if n := hostRecordLedgerCount(t, f.receiver); n != 1 {
		t.Fatalf("ledger entries=%d, want only the created record's", n)
	}
	if n := hostRecordRefusalAudits(t, f.receiver, errCodeRemoteDNSHostRecordOutsideZone); n != 3 {
		t.Fatalf("refusal audits=%d", n)
	}
	// Control: the setup review keeps its own zone-wide rule in this release.
	_, err = readServerSetupInfrastructureDNSSnapshot(context.Background(), f.receiver.db.GetDB(), "example.test")
	var review *serverSetupInfrastructureDNSPlanError
	if !errors.As(err, &review) || review.Code != "infrastructure_dns_conflict" {
		t.Fatalf("setup review rule changed: %v", err)
	}
}

func TestRemoteDNSHostRecordZoneStateRefusalsHaveTheirOwnCode(t *testing.T) {
	for name, change := range map[string]string{
		"dnssec signed": `INSERT INTO pdns_domainmetadata(domain_id,kind,content) SELECT id,'PRESIGNED','1' FROM pdns_domains WHERE name='example.test'`,
		"over its bound": `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<2050)
			INSERT INTO pdns_records(domain_id,name,type,content,ttl) SELECT d.id,'r'||n.i||'.example.test','TXT','"x"',300 FROM n, pdns_domains d WHERE d.name='example.test'`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newHostRecordFixture(t)
			if _, err := f.receiver.db.GetDB().Exec(change); err != nil {
				t.Fatal(err)
			}
			_, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
			var httpErr *remoteDNSHTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusConflict || httpErr.Code != errCodeRemoteDNSHostRecordZoneReview {
				t.Fatalf("err=%v", err)
			}
			if len(f.addresses(t, "mail.boston.example.test")) != 0 || hostRecordLedgerCount(t, f.receiver) != 0 || f.publishes != 0 {
				t.Fatal("a zone-state refusal wrote or published something")
			}
			if hostRecordRefusalAudits(t, f.receiver, errCodeRemoteDNSHostRecordZoneReview) != 1 {
				t.Fatal("the refusal left no audit row")
			}
		})
	}
	// Control: a record clash at the name stays CONFLICT.
	f := newHostRecordFixture(t)
	if _, err := f.ask(t, "boston.example.test", "A", hostRecordPeerIP); err != nil {
		t.Fatalf("the peer's existing equal record: %v", err)
	}
	if _, err := f.ask(t, "frankfurt.example.test", "A", hostRecordPeerIP); refusalCode(err) != errCodeRemoteDNSHostRecordConflict {
		t.Fatalf("clash: %v", err)
	}
}

// Over the real TLS transport: with the receiver's DNS locks held, the
// sender reads 503 within the bound (well under its 10 s header limit),
// nothing is written and no request result is stored; the same request after
// the locks are free creates the record.
func TestRemoteDNSHostRecordAnswersBusyWithinTheBoundOverTLS(t *testing.T) {
	f := newHostRecordFixture(t)
	var publishes atomic.Int32
	remoteDNSPublishReceived = func(*Panel, context.Context, string, bool) error {
		publishes.Add(1)
		return nil
	}
	stack := f.receiver.requireRemoteDNSMachineAuth(csrfProtect(f.receiver.requireAuth(http.HandlerFunc(f.receiver.handleRemoteDNSMachine))))
	endpoint, deps, _ := remoteDNSTestTransport(t, stack)
	remoteDNSExchange = func(ctx context.Context, _, path, secret string, request, response any) error {
		return remoteDNSHTTPJSONWith(ctx, endpoint, path, secret, request, response, deps)
	}
	f.receiver.serviceMutationMu.Lock()
	started := time.Now()
	_, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	elapsed := time.Since(started)
	f.receiver.serviceMutationMu.Unlock()
	var httpErr *remoteDNSHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable || httpErr.Code != "" {
		t.Fatalf("busy answer: %v", err)
	}
	if elapsed < remoteDNSReceiverLockWait || elapsed > remoteDNSReceiverLockWait+3*time.Second {
		t.Fatalf("answered after %s; the bound is %s and the sender waits 10 s", elapsed, remoteDNSReceiverLockWait)
	}
	if len(f.addresses(t, "mail.boston.example.test")) != 0 || hostRecordLedgerCount(t, f.receiver) != 0 || publishes.Load() != 0 {
		t.Fatal("a busy answer applied something")
	}
	if hostRecordRefusalAudits(t, f.receiver, errCodeRemoteDNSBusy) != 1 {
		t.Fatal("the busy answer left no audit row")
	}
	answer, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	if err != nil || answer.Result != "created" || publishes.Load() != 1 {
		t.Fatalf("resend after release: answer=%+v err=%v publishes=%d", answer, err, publishes.Load())
	}
}

func TestRemoteDNSHostRecordBusyReleasesWhatItTookAndSaysWhenToAskAgain(t *testing.T) {
	f := newHostRecordFixture(t)
	shortenRemoteDNSReceiverLockWait(t, 150*time.Millisecond)
	secret := f.secret(t)
	// Only the second lock is held: the first must be given back.
	dnsPublicationMu.Lock()
	w := f.direct(context.Background(), secret, "mail.boston.example.test", "A", hostRecordPeerIP)
	dnsPublicationMu.Unlock()
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" || remoteDNSRefusalCode(w.Body) != "" {
		t.Fatalf("status=%d retry-after=%q", w.Code, w.Header().Get("Retry-After"))
	}
	if !f.receiver.serviceMutationMu.TryLock() {
		t.Fatal("a busy answer kept the host mutation lease")
	}
	f.receiver.serviceMutationMu.Unlock()
	// The publish receiver waits within the same bound and names the busy code
	// (every sending version reads a 503 as "no verdict" and sends again).
	f.receiver.serviceMutationMu.Lock()
	publication, _ := canonicalRemoteDNSPublication(remoteDNSPublication{Domain: "busy.example", Generation: 1, Records: []DNSRecord{{Name: "busy.example", Type: "A", Content: "192.0.2.42", TTL: 300}}})
	raw, _ := json.Marshal(publication)
	r := httptest.NewRequest(http.MethodPost, "https://dns.remote.test:2083/api/v1/dns/remote/receiver/publish", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+secret)
	pw := httptest.NewRecorder()
	f.receiver.handleRemoteDNSMachine(pw, r)
	f.receiver.serviceMutationMu.Unlock()
	if pw.Code != http.StatusServiceUnavailable || pw.Header().Get("Retry-After") == "" || remoteDNSRefusalCode(pw.Body) != errCodeRemoteDNSBusy {
		t.Fatalf("publish receiver busy: status=%d", pw.Code)
	}
	var owned int
	f.receiver.db.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_zone_ownership WHERE zone_name='busy.example'`).Scan(&owned)
	if owned != 0 || f.publishes != 0 {
		t.Fatal("a busy publish receiver applied something")
	}
}

// The path the lab met: the sender gives up while the request waits for the
// locks. Nothing is applied, and the journal and audit trail say so (before,
// it was answered 401 through a path that wrote neither).
func TestRemoteDNSHostRecordSenderThatLeftIsRecordedAndNothingApplied(t *testing.T) {
	f := newHostRecordFixture(t)
	secret := f.secret(t)
	f.receiver.serviceMutationMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- f.direct(ctx, secret, "mail.boston.example.test", "A", hostRecordPeerIP) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	var w *httptest.ResponseRecorder
	select {
	case w = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the receiver kept waiting for a sender that left")
	}
	f.receiver.serviceMutationMu.Unlock()
	if w.Code == http.StatusUnauthorized {
		t.Fatal("a sender that left was answered as revoked")
	}
	if len(f.addresses(t, "mail.boston.example.test")) != 0 || hostRecordLedgerCount(t, f.receiver) != 0 {
		t.Fatal("applied for a sender that left")
	}
	if hostRecordRefusalAudits(t, f.receiver, "sender_left") != 1 {
		t.Fatal("no audit row for the sender that left")
	}
}

// A sender whose answer was lost while the publication ran (it may take up
// to 30 s, past the sender's 10 s) asks again: while the first request still
// publishes the repeat is "busy" with nothing applied, afterwards "present"
// with no second write. Polling never starts a second mutation (D-024).
func TestRemoteDNSHostRecordRepeatDuringAndAfterASlowPublicationIsReplaySafe(t *testing.T) {
	f := newHostRecordFixture(t)
	shortenRemoteDNSReceiverLockWait(t, 150*time.Millisecond)
	secret := f.secret(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var publishes atomic.Int32
	remoteDNSPublishReceived = func(*Panel, context.Context, string, bool) error {
		if publishes.Add(1) == 1 {
			close(entered)
			<-release
		}
		return nil
	}
	first := make(chan *httptest.ResponseRecorder)
	go func() {
		first <- f.direct(context.Background(), secret, "mail.boston.example.test", "A", hostRecordPeerIP)
	}()
	<-entered
	repeat := f.direct(context.Background(), secret, "mail.boston.example.test", "A", hostRecordPeerIP)
	if repeat.Code != http.StatusServiceUnavailable {
		t.Fatalf("repeat during the publication: %d %s", repeat.Code, repeat.Body.String())
	}
	close(release)
	if w := <-first; w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"result":"created"`) {
		t.Fatalf("first: %d %s", w.Code, w.Body.String())
	}
	again := f.direct(context.Background(), secret, "mail.boston.example.test", "A", hostRecordPeerIP)
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), `"result":"present"`) {
		t.Fatalf("repeat after: %d %s", again.Code, again.Body.String())
	}
	// One record and one ledger entry. (The repeat publishes the zone again
	// only because this fixture's zone never reaches an applied sync state;
	// a publication writes no record.)
	if got := f.addresses(t, "mail.boston.example.test"); len(got) != 1 || hostRecordLedgerCount(t, f.receiver) != 1 {
		t.Fatalf("records=%v ledger=%d", got, hostRecordLedgerCount(t, f.receiver))
	}
}

// --- The secondary's setup side with the new answers ---

func TestSetupOnTheSecondaryWithAChildZoneOnThePrimaryGetsItsRecord(t *testing.T) {
	f := newHostRecordFixture(t)
	seedHostRecordChildZone(t, f.receiver, "shop.example.test")
	plan, step := hostRecordSecondaryPlan(f)
	ctx := context.Background()
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, err := f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if err != nil || outcome == nil || outcome.Status != "applied" || outcome.Result != "created" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	// A zone review on the primary is a definite refusal with its own code,
	// never sent again for the same step.
	f2 := newHostRecordFixture(t)
	if _, err := f2.receiver.db.GetDB().Exec(`INSERT INTO pdns_domainmetadata(domain_id,kind,content) SELECT id,'PRESIGNED','1' FROM pdns_domains WHERE name='example.test'`); err != nil {
		t.Fatal(err)
	}
	plan, step = hostRecordSecondaryPlan(f2)
	f2.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	f2.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, _ = f2.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if outcome == nil || outcome.Status != "refused" || outcome.Code != errCodeRemoteDNSHostRecordZoneReview || f2.exchanges != 1 {
		t.Fatalf("outcome=%+v exchanges=%d", outcome, f2.exchanges)
	}
}

func TestSetupOnTheSecondaryAsksABusyPrimaryAgainLikeAnUnknownResult(t *testing.T) {
	f := newHostRecordFixture(t)
	shortenRemoteDNSReceiverLockWait(t, 100*time.Millisecond)
	plan, step := hostRecordSecondaryPlan(f)
	ctx := context.Background()
	f.receiver.serviceMutationMu.Lock()
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	f.receiver.serviceMutationMu.Unlock()
	outcome, _ := f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if outcome == nil || outcome.Status != "unknown" || outcome.Code != errCodeRemoteDNSBusy || f.exchanges != 1 {
		t.Fatalf("busy read as %+v after %d exchanges", outcome, f.exchanges)
	}
	// Two minutes later it is asked again and the record is created.
	outcome.At = time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(outcome)
	f.origin.db.GetDB().Exec(`UPDATE panel_settings SET value=? WHERE key=?`, string(raw), serverSetupPeerHostRecordKey(step.RequestID))
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, _ = f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if outcome == nil || outcome.Status != "applied" || outcome.Result != "created" || f.exchanges != 2 {
		t.Fatalf("after release: %+v exchanges=%d", outcome, f.exchanges)
	}
	// Control: a 503 a primary put a code on is still not a refusal.
	remoteDNSExchange = func(context.Context, string, string, string, any, any) error {
		return &remoteDNSHTTPError{StatusCode: http.StatusServiceUnavailable, Code: "SOME_CODE"}
	}
	step.RequestID = strings.Repeat("e", 32)
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	if outcome, _ = f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID); outcome == nil || outcome.Status != "unknown" {
		t.Fatalf("coded 503: %+v", outcome)
	}
}
