package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Gap 6 of the second server as a full peer (2026-10-16): the secondary asks
// the primary for its host record in the infrastructure zone over the
// authorized remote DNS connection. The fixture is the remote DNS HTTP fixture
// (origin = the secondary, receiver = the primary) with the receiver's
// infrastructure zone seeded as setup leaves it.

const hostRecordPeerIP = "192.0.2.2"

type hostRecordFixture struct {
	*remoteDNSHTTPFixture
	zone      serverSetupDraft
	sourceIP  string
	exchanges int
	down      bool
}

func newHostRecordFixture(t *testing.T) *hostRecordFixture {
	t.Helper()
	f := &hostRecordFixture{remoteDNSHTTPFixture: newRemoteDNSHTTPFixture(t)}
	f.zone = seedSetupInfrastructureZone(t, f.receiver)
	f.connect(t)
	remoteDNSReadLocalAuthority = func(*Panel, context.Context) (remoteDNSAuthority, error) {
		return remoteDNSAuthority{Ready: true, Engine: "bind", Epoch: 1, Nameservers: []string{"ns1.example.test", "ns2.example.test"}, PrimaryIP: "192.0.2.1", SecondaryIP: hostRecordPeerIP}, nil
	}
	// The same exchange as the fixture's, keeping the typed refusal code the
	// production transport reads from the answer's body.
	remoteDNSExchange = func(ctx context.Context, endpoint, path, secret string, request, response any) error {
		f.exchanges++
		if f.down {
			return errors.New("remote DNS HTTPS exchange failed")
		}
		raw, err := json.Marshal(request)
		if err != nil {
			return err
		}
		r := httptest.NewRequest(http.MethodPost, endpoint+path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+secret)
		if f.sourceIP != "" {
			r.RemoteAddr = net.JoinHostPort(f.sourceIP, "40000")
		}
		w := httptest.NewRecorder()
		f.receiver.handleRemoteDNSMachine(w, r)
		if w.Code != http.StatusOK {
			return &remoteDNSHTTPError{StatusCode: w.Code, Code: remoteDNSRefusalCode(w.Body)}
		}
		return json.Unmarshal(w.Body.Bytes(), response)
	}
	return f
}

func (f *hostRecordFixture) ask(t *testing.T, name, typ, content string) (remoteDNSHostRecordAnswer, error) {
	t.Helper()
	c, err := f.origin.readRemoteDNSConnection(context.Background(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	var answer remoteDNSHostRecordAnswer
	err = remoteDNSExchange(context.Background(), c.Endpoint, remoteDNSHostRecordPath, c.ID+"."+c.credential, remoteDNSHostRecordRequest{Name: name, Type: typ, Content: content}, &answer)
	return answer, err
}

func refusalCode(err error) string {
	var httpErr *remoteDNSHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Code
	}
	return ""
}

func (f *hostRecordFixture) addresses(t *testing.T, name string) []string {
	t.Helper()
	rows, err := f.receiver.db.GetDB().Query(`SELECT content FROM pdns_records WHERE name=? AND type='A' ORDER BY id`, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var content string
		rows.Scan(&content)
		list = append(list, content)
	}
	return list
}

func TestRemoteDNSHostRecordIsCreatedOnceAndAnsweredAgainIdempotently(t *testing.T) {
	f := newHostRecordFixture(t)
	answer, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	if err != nil || !answer.Applied || answer.Result != "created" || answer.Zone != "example.test" {
		t.Fatalf("answer=%+v err=%v", answer, err)
	}
	publishes := f.publishes
	if publishes != 1 {
		t.Fatalf("publications=%d", publishes)
	}
	// The same request again: the same record, nothing written twice.
	answer, err = f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	if err != nil || answer.Result != "present" {
		t.Fatalf("replay=%+v err=%v", answer, err)
	}
	if got := f.addresses(t, "mail.boston.example.test"); len(got) != 1 || got[0] != hostRecordPeerIP {
		t.Fatalf("records=%v", got)
	}
	// The editor on the primary names it as the peer's record.
	managed, err := f.receiver.infrastructureDNSManagedRecords(context.Background(), "example.test")
	if err != nil || managed[infrastructureDNSRecordKey("mail.boston.example.test", "A", hostRecordPeerIP)] == "" {
		t.Fatalf("managed=%v err=%v", managed, err)
	}
	// Audited on the primary.
	var audits int
	f.receiver.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'dns.remote.host_record.%'`).Scan(&audits)
	if audits != 2 {
		t.Fatalf("audits=%d", audits)
	}
}

func TestRemoteDNSHostRecordRefusesWhatIsNotThePeersOwnHost(t *testing.T) {
	for name, c := range map[string]struct {
		host, typ, content, code string
	}{
		"other address":  {"mail.boston.example.test", "A", "198.51.100.40", errCodeRemoteDNSHostRecordAddress},
		"outside zone":   {"mail.boston.other.test", "A", hostRecordPeerIP, errCodeRemoteDNSHostRecordOutsideZone},
		"zone apex":      {"example.test", "A", hostRecordPeerIP, errCodeRemoteDNSHostRecordOutsideZone},
		"setup's own":    {"frankfurt.example.test", "A", hostRecordPeerIP, errCodeRemoteDNSHostRecordConflict},
		"not a host":     {"mail.boston.example.test", "TXT", "x", errCodeRemoteDNSHostRecordInvalid},
		"wildcard":       {"*.example.test", "A", hostRecordPeerIP, errCodeRemoteDNSHostRecordInvalid},
		"ipv6 not yours": {"mail.boston.example.test", "AAAA", "2001:4860::1", errCodeRemoteDNSHostRecordAddress},
	} {
		t.Run(name, func(t *testing.T) {
			f := newHostRecordFixture(t)
			_, err := f.ask(t, c.host, c.typ, c.content)
			if refusalCode(err) != c.code {
				t.Fatalf("err=%v code=%q want %s", err, refusalCode(err), c.code)
			}
			if f.publishes != 0 {
				t.Fatal("a refused request published the zone")
			}
		})
	}
}

func TestRemoteDNSHostRecordAcceptsTheRequestsOwnIPv6Address(t *testing.T) {
	f := newHostRecordFixture(t)
	f.sourceIP = "2001:4860::5"
	answer, err := f.ask(t, "mail.boston.example.test", "AAAA", "2001:4860::5")
	if err != nil || answer.Result != "created" {
		t.Fatalf("answer=%+v err=%v", answer, err)
	}
}

func TestRemoteDNSHostRecordNeverOverwritesTheOwner(t *testing.T) {
	f := newHostRecordFixture(t)
	// The owner's own record for the name with another address is kept.
	if _, err := f.receiver.db.GetDB().Exec(`INSERT INTO pdns_records(domain_id,name,type,content,ttl) SELECT id,'webmail.example.test','A','198.51.100.8',300 FROM pdns_domains WHERE name='example.test'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ask(t, "webmail.example.test", "A", hostRecordPeerIP); refusalCode(err) != errCodeRemoteDNSHostRecordConflict {
		t.Fatalf("owner record overwritten? err=%v", err)
	}
	if got := f.addresses(t, "webmail.example.test"); len(got) != 1 || got[0] != "198.51.100.8" {
		t.Fatalf("owner record changed: %v", got)
	}
	// A record this channel created and the owner then changed is refused too.
	if _, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP); err != nil {
		t.Fatal(err)
	}
	if _, err := f.receiver.db.GetDB().Exec(`UPDATE pdns_records SET content='198.51.100.9' WHERE name='mail.boston.example.test'`); err != nil {
		t.Fatal(err)
	}
	remoteDNSReadLocalAuthority = func(*Panel, context.Context) (remoteDNSAuthority, error) {
		return remoteDNSAuthority{Ready: true, Engine: "bind", Epoch: 1, Nameservers: []string{"ns1.example.test", "ns2.example.test"}, SecondaryIP: "192.0.2.3"}, nil
	}
	if _, err := f.ask(t, "mail.boston.example.test", "A", "192.0.2.3"); refusalCode(err) != errCodeRemoteDNSHostRecordConflict {
		t.Fatalf("owner change overwritten? err=%v", err)
	}
	// Control: the channel's own unchanged record follows a new peer address.
	if _, err := f.receiver.db.GetDB().Exec(`UPDATE pdns_records SET content=? WHERE name='mail.boston.example.test'`, hostRecordPeerIP); err != nil {
		t.Fatal(err)
	}
	answer, err := f.ask(t, "mail.boston.example.test", "A", "192.0.2.3")
	if err != nil || answer.Result != "updated" {
		t.Fatalf("own record not updated: %+v %v", answer, err)
	}
	// A removed channel record is not created again behind the owner's back.
	if _, err := f.receiver.db.GetDB().Exec(`DELETE FROM pdns_records WHERE name='mail.boston.example.test'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ask(t, "mail.boston.example.test", "A", "192.0.2.3"); refusalCode(err) != errCodeRemoteDNSHostRecordConflict {
		t.Fatalf("removed record recreated: %v", err)
	}
}

func TestRemoteDNSHostRecordNeedsTheAuthorizedConnection(t *testing.T) {
	f := newHostRecordFixture(t)
	if _, err := f.receiver.db.GetDB().Exec(`UPDATE remote_dns_clients SET revoked=1`); err != nil {
		t.Fatal(err)
	}
	_, err := f.ask(t, "mail.boston.example.test", "A", hostRecordPeerIP)
	var httpErr *remoteDNSHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked connection accepted: %v", err)
	}
	if got := f.addresses(t, "mail.boston.example.test"); len(got) != 0 {
		t.Fatal("a revoked connection created a record")
	}
	// A request without the machine credential is not a machine request.
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "https://dns.remote.test:2083"+remoteDNSHostRecordPath, strings.NewReader(`{}`))
	f.receiver.handleRemoteDNSMachine(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous=%d", w.Code)
	}
}

// Replication independence: the primary's infrastructure zone and its
// records exist and are listed without any remote DNS connection; the channel
// only adds a record to that zone.
func TestInfrastructureZoneWorksWithoutTheRemoteConnection(t *testing.T) {
	p := newDNSPanelForTest(t)
	d := seedSetupInfrastructureZone(t, p)
	var connections, clients int
	p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_connections`).Scan(&connections)
	p.db.GetDB().QueryRow(`SELECT COUNT(*) FROM remote_dns_clients`).Scan(&clients)
	if connections+clients != 0 {
		t.Fatal("fixture has a connection")
	}
	zones, err := p.infrastructureDNSZones(context.Background())
	if err != nil || len(zones) != 1 || zones[0].Zone != d.InfrastructureDNS.Zone || zones[0].PeerRecords != 0 {
		t.Fatalf("zones=%+v err=%v", zones, err)
	}
	// The receiver's guard keeps the replication path free of the channel:
	// zone transfer code never reads the host record ledger.
	for _, file := range []string{"dns_sync.go"} {
		if strings.Contains(readSourceForTest(t, file), "remote_dns_host_record") {
			t.Fatalf("%s reads the host record ledger", file)
		}
	}
}

// --- The secondary's setup side ---

func hostRecordSecondaryPlan(f *hostRecordFixture) (serverSetupPlan, serverSetupExecutionStep) {
	plan := serverSetupPlan{ServerIP: hostRecordPeerIP}
	plan.Draft = setupDNSTestDraft()
	plan.Draft.DNSRole = "secondary"
	plan.Draft.LocalIP, plan.Draft.PeerIP = hostRecordPeerIP, "192.0.2.1"
	plan.Draft.PeerNS = "ns1.example.test"
	plan.Draft.DNSPublisherEndpoint = "https://dns.remote.test:2083"
	step := serverSetupExecutionStep{serverSetupPlanStep: serverSetupPlanStep{Kind: "access_dns", Target: "mail.boston.example.test", Qualifier: hostRecordPeerIP}, RequestID: strings.Repeat("b", 32), Status: "running"}
	return plan, step
}

func TestSetupOnTheSecondaryAsksThePrimaryOnceAndSaysWhatHappened(t *testing.T) {
	f := newHostRecordFixture(t)
	plan, step := hostRecordSecondaryPlan(f)
	ctx := context.Background()
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, err := f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if err != nil || outcome == nil || outcome.Status != "applied" || outcome.Result != "created" || outcome.Zone != "example.test" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	// The runner's next passes (every 20 seconds) send nothing more.
	for i := 0; i < 3; i++ {
		f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	}
	if f.exchanges != 1 {
		t.Fatalf("exchanges=%d", f.exchanges)
	}
	peerContext := f.origin.serverSetupPeerHostRecordContextFor(ctx, plan, step)
	if peerContext == nil || !peerContext.Channel || peerContext.Outcome == nil || peerContext.Outcome.Status != "applied" || peerContext.Outcome.Connection != "" {
		t.Fatalf("context=%+v", peerContext)
	}
	var audits int
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action='server.setup.peer_host_record.applied'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("secondary audits=%d", audits)
	}
}

func TestSetupOnTheSecondaryKeepsTheRefusalAndRetriesOnlyAnUnknownResult(t *testing.T) {
	f := newHostRecordFixture(t)
	plan, step := hostRecordSecondaryPlan(f)
	step.Target = "mail.boston.other.test"
	ctx := context.Background()
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, _ := f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if outcome == nil || outcome.Status != "refused" || outcome.Code != errCodeRemoteDNSHostRecordOutsideZone {
		t.Fatalf("outcome=%+v", outcome)
	}
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	if f.exchanges != 1 {
		t.Fatal("a refusal was sent again")
	}
	// An unknown result: kept, and sent again only after two minutes.
	step.RequestID = strings.Repeat("c", 32)
	f.down = true
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	outcome, _ = f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID)
	if outcome == nil || outcome.Status != "unknown" || f.exchanges != 2 {
		t.Fatalf("outcome=%+v exchanges=%d", outcome, f.exchanges)
	}
	outcome.At = time.Now().Add(-3 * time.Minute).UTC().Format(time.RFC3339)
	raw, _ := json.Marshal(outcome)
	f.origin.db.GetDB().Exec(`UPDATE panel_settings SET value=? WHERE key=?`, string(raw), serverSetupPeerHostRecordKey(step.RequestID))
	f.down = false
	step.Target = "mail.boston.example.test"
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	if f.exchanges != 3 {
		t.Fatalf("unknown result not retried: %d", f.exchanges)
	}
}

func TestSetupOnTheSecondaryWithoutAConnectionShowsTheManualRecord(t *testing.T) {
	f := newHostRecordFixture(t)
	plan, step := hostRecordSecondaryPlan(f)
	if _, err := f.origin.db.GetDB().Exec(`DELETE FROM remote_dns_connections`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	if f.exchanges != 0 {
		t.Fatal("sent without a connection")
	}
	peerContext := f.origin.serverSetupPeerHostRecordContextFor(ctx, plan, step)
	if peerContext == nil || peerContext.Channel || peerContext.Outcome == nil || peerContext.Outcome.Status != "none" {
		t.Fatalf("context=%+v", peerContext)
	}
	// Control: a primary (not a secondary) never asks anyone.
	plan.Draft.DNSRole = "primary"
	step.RequestID = strings.Repeat("d", 32)
	f.origin.requestServerSetupPeerHostRecord(ctx, plan, step)
	if outcome, _ := f.origin.readServerSetupPeerHostRecordOutcome(ctx, step.RequestID); outcome != nil {
		t.Fatalf("a primary recorded %+v", outcome)
	}
	if f.origin.serverSetupPeerHostRecordContextFor(ctx, plan, step) != nil {
		t.Fatal("a primary's context has a peer record")
	}
}

func TestRemoteDNSRefusalCodeReadsOnlyATypedCode(t *testing.T) {
	for body, want := range map[string]string{
		`{"error":"x","code":"REMOTE_DNS_HOST_RECORD_CONFLICT"}`: "REMOTE_DNS_HOST_RECORD_CONFLICT",
		`{"code":"has space"}`: "",
		`not json`:             "",
		`{"code":"` + strings.Repeat("A", 65) + `"}`: "",
	} {
		if got := remoteDNSRefusalCode(strings.NewReader(body)); got != want {
			t.Fatalf("%s -> %q", body, got)
		}
	}
}

func readSourceForTest(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
