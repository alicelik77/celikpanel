package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Automatic DKIM (30 Oct 2026): a domain that can send mail from this server
// gets its key, signing and (where this server keeps the zone) its record by
// itself; an existing key or a different record is kept; a failure never
// blocks the creation and is shown with its reason and retry; deletion
// removes only what CelikPanel made.

type dkimAutoAgent struct {
	*strictDNSRPCAgent

	dmu          sync.Mutex
	keys         map[string]string // domain -> public key
	ownerKeys    map[string]bool   // keys CelikPanel did not make
	signed       map[string]bool
	ensureCalls  int
	configCalls  int
	removeCalls  int
	ensureError  string
	signError    string
	busy         bool
	configLosses int // ConfigureDKIMSigning answers lost (transport error)
}

func newDKIMAutoAgent() *dkimAutoAgent {
	return &dkimAutoAgent{
		strictDNSRPCAgent: &strictDNSRPCAgent{},
		keys:              map[string]string{},
		ownerKeys:         map[string]bool{},
		signed:            map[string]bool{},
	}
}

func (a *dkimAutoAgent) BeginServiceMutation(req *ServiceOperationMutationBeginRequest, resp *ServiceOperationMutationResponse) error {
	a.dmu.Lock()
	busy := a.busy
	a.dmu.Unlock()
	if busy {
		resp.ErrorCode = transport.HostMutationBusy
		resp.Reason = transport.HostMutationReasonAgentMutation
		resp.Error = "another service mutation owns the host lease"
		return nil
	}
	return a.strictDNSRPCAgent.BeginServiceMutation(req, resp)
}

func (a *dkimAutoAgent) EnsureDKIMKey(req *transport.DKIMEnsureRequest, resp *transport.DKIMEnsureResponse) error {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	a.ensureCalls++
	if a.ensureError != "" {
		resp.Error = a.ensureError
		return nil
	}
	if pub, ok := a.keys[req.Domain]; ok {
		resp.PublicKeyB64 = pub
		return nil
	}
	a.keys[req.Domain] = "PUB" + strings.ReplaceAll(req.Domain, ".", "")
	resp.PublicKeyB64 = a.keys[req.Domain]
	resp.Created = true
	return nil
}

func (a *dkimAutoAgent) GetDKIMStatus(req *transport.DKIMStatusRequest, resp *transport.DKIMStatusResponse) error {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	resp.SigningInstalled = true
	if pub, ok := a.keys[req.Domain]; ok {
		resp.HasKey, resp.PublicKeyB64, resp.Signed = true, pub, a.signed[req.Domain]
	}
	return nil
}

func (a *dkimAutoAgent) ConfigureDKIMSigning(_ *transport.ServiceMutationRequest, resp *transport.ConfigureDKIMSigningResponse) error {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	a.configCalls++
	if a.configLosses > 0 {
		a.configLosses--
		return errors.New("connection reset")
	}
	if a.signError != "" {
		resp.Error = a.signError
		return nil
	}
	for domain := range a.keys {
		a.signed[domain] = true
	}
	for domain := range a.signed {
		if _, ok := a.keys[domain]; !ok {
			delete(a.signed, domain)
		}
	}
	resp.Configured, resp.Domains = true, len(a.keys)
	return nil
}

func (a *dkimAutoAgent) RemoveDKIMKey(req *transport.DKIMRemoveRequest, resp *transport.DKIMRemoveResponse) error {
	a.dmu.Lock()
	defer a.dmu.Unlock()
	a.removeCalls++
	resp.SigningConfigured = true
	switch {
	case a.ownerKeys[req.Domain]:
		resp.KeptKey = "/var/lib/celikpanel-dkim/keys/" + req.Domain + "/celik.private"
	case a.keys[req.Domain] != "":
		delete(a.keys, req.Domain)
		resp.Removed = true
	default:
		resp.Absent = true
	}
	return nil
}

type dkimAutoFixture struct {
	p      *Panel
	agent  *dkimAutoAgent
	kicks  atomic.Int32
	nextID int
}

func newDKIMAutoFixture(t *testing.T) *dkimAutoFixture {
	t.Helper()
	p := newDNSPanelForTest(t)
	setDNSIdentityForTest(t, p, "standalone")
	f := &dkimAutoFixture{p: p, agent: newDKIMAutoAgent(), nextID: 7100}
	p.automaticDKIMKicked = func() { f.kicks.Add(1) }
	ensureActiveDNSEngineForTest(t, p, transport.DNSEnginePowerDNS)
	f.agent.publisher = transport.DNSEnginePowerDNS
	p.pkgFamilyVal = "apt"
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", f.agent); err != nil {
		t.Fatalf("register DKIM agent: %v", err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	raw, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(raw, connector)
	t.Cleanup(func() { _ = raw.Close() })
	if _, err := p.db.GetDB().Exec(`
		INSERT INTO users (id, username, password_hash, email, role) VALUES (7001, 'dkim-admin', 'x', 'dkim@example.test', 'admin');
		INSERT INTO subscriptions (id, owner_id, name, max_email_accounts, status) VALUES (7002, 7001, 'DKIM', 50, 'active');
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return f
}

// domain adds a domain; mail=true gives it a mailbox (the product's own
// "this domain has mail here" record).
func (f *dkimAutoFixture) domain(t *testing.T, name, dnsMode string, parent int, mail bool) int {
	t.Helper()
	f.nextID++
	id := f.nextID
	var parentID any
	if parent > 0 {
		parentID = parent
	}
	if _, err := f.p.db.GetDB().Exec(
		`INSERT INTO domains (id, subscription_id, name, status, dns_management, parent_domain_id) VALUES (?, 7002, ?, 'active', ?, ?)`,
		id, name, dnsMode, parentID); err != nil {
		t.Fatalf("seed domain %s: %v", name, err)
	}
	if mail {
		if _, err := f.p.db.GetDB().Exec(
			`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, ?, 'managed-by-agent', 100)`,
			id, "info@"+name); err != nil {
			t.Fatalf("seed mailbox %s: %v", name, err)
		}
	}
	return id
}

func (f *dkimAutoFixture) txt(t *testing.T, name string) []string {
	t.Helper()
	rows, err := f.p.db.GetDB().Query(`SELECT content FROM pdns_records WHERE type = 'TXT' AND name = ? ORDER BY id`, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			t.Fatal(err)
		}
		out = append(out, joinTXTSegments(content))
	}
	return out
}

func (f *dkimAutoFixture) record(t *testing.T, id int, name string) (dkimAutomaticRecord, bool) {
	t.Helper()
	record, ok, err := f.p.readDKIMAutomaticRecord(context.Background(), id, name)
	if err != nil {
		t.Fatal(err)
	}
	return record, ok
}

func (f *dkimAutoFixture) queue(t *testing.T, id int, name, trigger string) {
	t.Helper()
	if _, err := f.p.requestAutomaticDKIM(context.Background(), id, name, trigger, false); err != nil {
		t.Fatal(err)
	}
}

func (f *dkimAutoFixture) pass(t *testing.T) bool {
	t.Helper()
	return f.p.runAutomaticDKIMPass(context.Background(), false)
}

func TestAutomaticDKIMMakesKeySigningAndRecordInALocalZone(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "site.example")
	id := f.domain(t, "site.example", "local", 0, true)

	f.queue(t, id, "site.example", dkimTriggerDomainCreated)
	if retry := f.pass(t); retry {
		t.Fatal("a successful pass asked to be repeated")
	}
	if f.agent.ensureCalls != 1 || f.agent.configCalls != 1 {
		t.Fatalf("ensure=%d configure=%d, want 1/1", f.agent.ensureCalls, f.agent.configCalls)
	}
	want := dkimRecommended(f.agent.keys["site.example"])
	if got := f.txt(t, "celik._domainkey.site.example"); len(got) != 1 || got[0] != want {
		t.Fatalf("DKIM TXT = %q, want [%q]", got, want)
	}
	if calls := f.agent.syncCalls; len(calls) == 0 || calls[len(calls)-1] != "site.example" {
		t.Fatalf("zone not published through the ordinary publication: %v", calls)
	}
	if _, ok := f.record(t, id, "site.example"); ok {
		t.Fatal("a finished automatic record was kept")
	}

	// Idempotent: the same trigger again changes nothing.
	syncs := len(f.agent.syncCalls)
	f.queue(t, id, "site.example", dkimTriggerFirstMailbox)
	f.pass(t)
	if f.agent.configCalls != 1 {
		t.Fatalf("signing reconfigured for a key already signed: %d", f.agent.configCalls)
	}
	if got := f.txt(t, "celik._domainkey.site.example"); len(got) != 1 {
		t.Fatalf("record duplicated: %q", got)
	}
	if len(f.agent.syncCalls) != syncs {
		t.Fatalf("an unchanged, published zone was published again")
	}
}

func TestAutomaticDKIMKeepsAnExistingKeyAndAnOwnerRecord(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "owner.example")
	id := f.domain(t, "owner.example", "local", 0, true)
	f.agent.keys["owner.example"] = "OWNERKEY" // placed by the owner, not signed yet
	zoneID, _, _, err := f.p.mailAuthZone(context.Background(), "owner.example")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'celik._domainkey.owner.example', 'TXT', ?, 3600)`,
		zoneID, splitTXTContent("v=DKIM1; k=rsa; p=SOMETHINGELSE")); err != nil {
		t.Fatal(err)
	}

	f.queue(t, id, "owner.example", dkimTriggerDomainCreated)
	f.pass(t)
	if f.agent.keys["owner.example"] != "OWNERKEY" {
		t.Fatal("the existing key was replaced")
	}
	if f.agent.configCalls != 1 || !f.agent.signed["owner.example"] {
		t.Fatalf("an unsigned existing key was not put into the signing table (configure=%d)", f.agent.configCalls)
	}
	if got := f.txt(t, "celik._domainkey.owner.example"); len(got) != 1 || got[0] != "v=DKIM1; k=rsa; p=SOMETHINGELSE" {
		t.Fatalf("the owner's record was changed: %q", got)
	}
	if _, ok := f.record(t, id, "owner.example"); ok {
		t.Fatal("record kept although signing works and the owner's record is reported on the card")
	}
}

func TestAutomaticDKIMWithExternalDNSSignsAndLeavesTheRecordToTheOwner(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "ext.example", "external", 0, true)
	f.queue(t, id, "ext.example", dkimTriggerDomainCreated)
	f.pass(t)
	if f.agent.configCalls != 1 || !f.agent.signed["ext.example"] {
		t.Fatal("signing was not configured for an external-DNS domain")
	}
	if got := f.txt(t, "celik._domainkey.ext.example"); len(got) != 0 {
		t.Fatalf("a record was written for external DNS: %q", got)
	}
	if len(f.agent.syncCalls) != 0 {
		t.Fatalf("external DNS was published: %v", f.agent.syncCalls)
	}
	if _, ok := f.record(t, id, "ext.example"); ok {
		t.Fatal("external-DNS record kept as unfinished work")
	}
}

func TestAutomaticDKIMSubdomainUsesOnlyItsRecordedParentZone(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "parent.example")
	parent := f.domain(t, "parent.example", "local", 0, false)
	child := f.domain(t, "shop.parent.example", "local", parent, true)
	// Another tenant's top-level domain that merely ends in parent.example.
	lookalike := f.domain(t, "evil.parent.example", "local", 0, true)

	if _, name, ok, err := f.p.mailAuthZone(context.Background(), "evil.parent.example"); err != nil || ok {
		t.Fatalf("a lookalike name reached another zone: %q %v %v", name, ok, err)
	}
	f.queue(t, child, "shop.parent.example", dkimTriggerFirstMailbox)
	f.queue(t, lookalike, "evil.parent.example", dkimTriggerFirstMailbox)
	f.pass(t)
	if got := f.txt(t, "celik._domainkey.shop.parent.example"); len(got) != 1 {
		t.Fatalf("subdomain record not in its parent's zone: %q", got)
	}
	if got := f.txt(t, "celik._domainkey.evil.parent.example"); len(got) != 0 {
		t.Fatalf("a lookalike wrote into another tenant's zone: %q", got)
	}
	if f.agent.configCalls != 1 {
		t.Fatalf("signing configured %d times in one pass, want 1", f.agent.configCalls)
	}

	// The subdomain's DKIM record goes with it.
	if _, err := f.p.removeSubdomainFromParentZone(context.Background(), "parent.example", "shop.parent.example"); err != nil {
		t.Fatal(err)
	}
	if got := f.txt(t, "celik._domainkey.shop.parent.example"); len(got) != 0 {
		t.Fatalf("subdomain DKIM record left behind: %q", got)
	}
}

func TestAutomaticDKIMFailureIsKeptWithItsReasonAndRetriedOnce(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "fail.example")
	id := f.domain(t, "fail.example", "local", 0, true)
	f.agent.signError = "opendkim restart: Job for opendkim.service failed"

	f.queue(t, id, "fail.example", dkimTriggerDomainCreated)
	if retry := f.pass(t); retry {
		t.Fatal("a verified failure was treated as a busy host")
	}
	record, ok := f.record(t, id, "fail.example")
	if !ok || record.State != dkimAutoFailed || record.Reason != dkimReasonSigningFailed ||
		!strings.Contains(record.ErrorLine, "opendkim restart") {
		t.Fatalf("failure record = %+v %v", record, ok)
	}
	if got := f.txt(t, "celik._domainkey.fail.example"); len(got) != 0 {
		t.Fatalf("a record was published while mail is unsigned: %q", got)
	}
	// A failed record is not taken again by a pass (nothing repeats by itself).
	f.pass(t)
	if f.agent.configCalls != 1 {
		t.Fatalf("a failed record was retried without the owner: %d", f.agent.configCalls)
	}

	// The card shows it; reading never starts work.
	ensures := f.agent.ensureCalls
	view := f.status(t, id)
	if view.Automatic == nil || view.Automatic.State != dkimAutoFailed || view.Automatic.Reason != dkimReasonSigningFailed {
		t.Fatalf("card automatic = %+v", view.Automatic)
	}
	if f.agent.ensureCalls != ensures || f.kicks.Load() != 0 {
		t.Fatal("a status read started work")
	}

	// "Try again" queues once; a second press while pending queues nothing.
	f.agent.signError = ""
	first := f.retry(t, id)
	second := f.retry(t, id)
	if first.Code != http.StatusAccepted || !strings.Contains(first.Body.String(), `"queued":true`) ||
		!strings.Contains(second.Body.String(), `"queued":false`) {
		t.Fatalf("retry answers: %d %s / %s", first.Code, first.Body.String(), second.Body.String())
	}
	f.pass(t)
	if f.agent.configCalls != 2 {
		t.Fatalf("retry did not configure signing exactly once more: %d", f.agent.configCalls)
	}
	if _, ok := f.record(t, id, "fail.example"); ok {
		t.Fatal("retried work did not finish")
	}
	if got := f.txt(t, "celik._domainkey.fail.example"); len(got) != 1 {
		t.Fatalf("record after retry: %q", got)
	}
}

func TestAutomaticDKIMKeyFailureAndLostAnswerAreDistinguished(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "key.example", "external", 0, true)
	f.agent.ensureError = "cannot create key directory: read-only file system"
	f.queue(t, id, "key.example", dkimTriggerDomainCreated)
	f.pass(t)
	if record, _ := f.record(t, id, "key.example"); record.State != dkimAutoFailed || record.Reason != dkimReasonKeyFailed {
		t.Fatalf("key failure record = %+v", record)
	}

	f.agent.ensureError = ""
	f.agent.configLosses = 1
	f.queue(t, id, "key.example", dkimTriggerRetry)
	f.pass(t)
	if record, _ := f.record(t, id, "key.example"); record.State != dkimAutoUnknown || record.Reason != dkimReasonSigningUnknown {
		t.Fatalf("lost signing answer record = %+v", record)
	}
}

func TestAutomaticDKIMWaitsForABusyHostAndResumes(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "busy.example", "external", 0, true)
	f.agent.busy = true
	f.queue(t, id, "busy.example", dkimTriggerDomainCreated)
	if retry := f.pass(t); !retry {
		t.Fatal("a busy host was not reported for a repeat")
	}
	if record, _ := f.record(t, id, "busy.example"); record.State != dkimAutoWaiting || record.Reason != dkimReasonHostBusy {
		t.Fatalf("busy record = %+v", record)
	}
	f.agent.busy = false
	if retry := f.pass(t); retry {
		t.Fatal("pass after the host freed still waits")
	}
	if _, ok := f.record(t, id, "busy.example"); ok || !f.agent.signed["busy.example"] {
		t.Fatal("waiting work did not resume")
	}
}

func TestAutomaticDKIMSkipsDomainsWithoutMailOrBeingDeleted(t *testing.T) {
	f := newDKIMAutoFixture(t)
	noMail := f.domain(t, "nomail.example", "external", 0, false)
	deleting := f.domain(t, "gone.example", "external", 0, true)
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO domain_deletion_operations (domain_id, previous_status) VALUES (?, 'active')`, deleting); err != nil {
		t.Fatal(err)
	}
	f.queue(t, noMail, "nomail.example", dkimTriggerDomainCreated)
	f.queue(t, deleting, "gone.example", dkimTriggerDomainCreated)
	f.pass(t)
	if f.agent.ensureCalls != 0 {
		t.Fatalf("a key was made for a domain without mail or being deleted: %d", f.agent.ensureCalls)
	}
	for id, name := range map[int]string{noMail: "nomail.example", deleting: "gone.example"} {
		if _, ok := f.record(t, id, name); ok {
			t.Fatalf("record of %s kept", name)
		}
	}
	// The owner's explicit "Try again" applies even without mail records.
	f.queue(t, noMail, "nomail.example", dkimTriggerRetry)
	f.pass(t)
	if f.agent.keys["nomail.example"] == "" {
		t.Fatal("the owner's retry was ignored")
	}
}

func TestAutomaticDKIMNewDomainIsHeldUntilTheRequestEnds(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "held.example", "external", 0, true)
	release := f.p.queueAutomaticDKIMForNewDomain(context.Background(), id, "held.example")
	f.pass(t)
	if f.agent.ensureCalls != 0 {
		t.Fatal("a held record was taken before its request finished")
	}
	release()
	if f.kicks.Load() != 1 {
		t.Fatalf("release did not start the runner: %d", f.kicks.Load())
	}
	f.pass(t)
	if f.agent.ensureCalls != 1 {
		t.Fatal("released record not handled")
	}

	// Panel start takes a held record whose request is gone.
	other := f.domain(t, "crash.example", "external", 0, true)
	f.p.queueAutomaticDKIMForNewDomain(context.Background(), other, "crash.example")
	f.p.runAutomaticDKIMPass(context.Background(), true)
	if f.agent.keys["crash.example"] == "" {
		t.Fatal("a held record was not resumed at start")
	}

	// Without mail on this server nothing is queued.
	plain := f.domain(t, "plain.example", "external", 0, false)
	f.p.queueAutomaticDKIMForNewDomain(context.Background(), plain, "plain.example")()
	if _, ok := f.record(t, plain, "plain.example"); ok {
		t.Fatal("a domain without mail was queued")
	}
}

func TestAutomaticDKIMFirstMailboxTriggersOnce(t *testing.T) {
	f := newDKIMAutoFixture(t)
	id := f.domain(t, "box.example", "external", 0, true) // its first mailbox
	f.p.queueAutomaticDKIMForMailbox(context.Background(), id, "box.example")
	if _, ok := f.record(t, id, "box.example"); !ok || f.kicks.Load() != 1 {
		t.Fatal("the first mailbox did not queue the domain's DKIM")
	}
	if err := f.p.clearDKIMAutomaticRecord(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, 'two@box.example', 'managed-by-agent', 100)`, id); err != nil {
		t.Fatal(err)
	}
	f.p.queueAutomaticDKIMForMailbox(context.Background(), id, "box.example")
	if _, ok := f.record(t, id, "box.example"); ok {
		t.Fatal("a second mailbox queued the work again")
	}
}

func TestDomainDeletionRemovesOnlyTheKeyCelikPanelMade(t *testing.T) {
	f := newDKIMAutoFixture(t)
	ours := f.domain(t, "ours.example", "external", 0, true)
	theirs := f.domain(t, "theirs.example", "external", 0, true)
	f.agent.keys["ours.example"] = "K1"
	f.agent.keys["theirs.example"] = "K2"
	f.agent.ownerKeys["theirs.example"] = true
	f.queue(t, ours, "ours.example", dkimTriggerFirstMailbox)

	cleanup := f.p.removeDomainDKIMLocked(context.Background(), ours, "ours.example")
	if !cleanup.Removed || cleanup.worthReporting() {
		t.Fatalf("our key: %+v", cleanup)
	}
	if _, ok := f.record(t, ours, "ours.example"); ok {
		t.Fatal("the deleted domain's automatic record was kept")
	}
	if f.p.setting(context.Background(), dkimSigningRefreshKey) == "" {
		t.Fatal("no signing refresh was requested after a removal")
	}
	f.pass(t)
	if f.agent.configCalls != 1 || f.agent.signed["ours.example"] {
		t.Fatalf("signing tables not refreshed (configure=%d)", f.agent.configCalls)
	}
	if f.p.setting(context.Background(), dkimSigningRefreshKey) != "" {
		t.Fatal("refresh request kept after it ran")
	}

	kept := f.p.removeDomainDKIMLocked(context.Background(), theirs, "theirs.example")
	if kept.Removed || !strings.HasSuffix(kept.KeptKey, "/theirs.example/celik.private") || !kept.worthReporting() {
		t.Fatalf("owner key: %+v", kept)
	}
	if f.agent.keys["theirs.example"] != "K2" {
		t.Fatal("the owner's key was removed")
	}
}

type dkimAutoStatusView struct {
	Automatic       *dkimAutomaticView `json:"automatic"`
	DKIMSigned      bool               `json:"dkim_signed"`
	DKIMOwnerRecord bool               `json:"dkim_owner_record"`
}

func (f *dkimAutoFixture) status(t *testing.T, id int) dkimAutoStatusView {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/mail/auth", nil)
	f.p.handleMailAuth(recorder, request, id)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: %d %s", recorder.Code, recorder.Body.String())
	}
	var view dkimAutoStatusView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func (f *dkimAutoFixture) retry(t *testing.T, id int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/1/mail/auth/automatic", nil)
	f.p.handleMailAuth(recorder, request, id)
	return recorder
}

func TestMailAuthStatusReportsTheOwnersDKIMRecordAndSigning(t *testing.T) {
	f := newDKIMAutoFixture(t)
	seedStrictDNSZone(t, f.p, "card.example")
	id := f.domain(t, "card.example", "local", 0, true)
	f.agent.keys["card.example"] = "CARDKEY"
	zoneID, _, _, _ := f.p.mailAuthZone(context.Background(), "card.example")
	if _, err := f.p.db.GetDB().Exec(`INSERT INTO pdns_records (domain_id, name, type, content, ttl) VALUES (?, 'celik._domainkey.card.example', 'TXT', ?, 3600)`,
		zoneID, splitTXTContent("v=DKIM1; k=rsa; p=OTHER")); err != nil {
		t.Fatal(err)
	}
	view := f.status(t, id)
	if !view.DKIMOwnerRecord || view.DKIMSigned || view.Automatic != nil {
		t.Fatalf("card = %+v", view)
	}
}
