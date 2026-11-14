package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.135: "Add" beside "Copy" for the webmail address record. The sender
// (this server, the pair's DNS secondary) runs against the real receiver
// handler of the main DNS server in-process (the host record fixture): the
// receiver is unchanged, so a primary on an older release answers the same.

const dnsAddHost = "mail.boston.example.test"

type dnsAddFixture struct {
	*hostRecordFixture
	fromExchanges int
	fromSeen      string // the address the receiver sees for an IPv6 request ("" = the source)
	fromErr       error
	nsAsked       []string
}

func newDNSAddFixture(t *testing.T, ipv6 string) *dnsAddFixture {
	t.Helper()
	f := &dnsAddFixture{hostRecordFixture: newHostRecordFixture(t)}
	stubWebmailAddressWorld(t, webmailAddressTestResolver{}, FirewallStatusResp{})
	t.Setenv("CELIKPANEL_SERVER_IP", hostRecordPeerIP)
	if ipv6 == "" {
		ipv6 = "none"
	}
	t.Setenv("CELIKPANEL_SERVER_IPV6", ipv6)
	restorePair, restoreFrom, restoreNS := webmailAddressPairState, remoteDNSExchangeFrom, webmailAddressNSLookup
	t.Cleanup(func() {
		webmailAddressPairState, remoteDNSExchangeFrom, webmailAddressNSLookup = restorePair, restoreFrom, restoreNS
	})
	webmailAddressPairState = func(*Panel, context.Context) (dnsEngineDBState, error) {
		return dnsEngineDBState{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRoleSecondary, LocalIP: hostRecordPeerIP, PeerIP: "192.0.2.1"}, nil
	}
	webmailAddressNSLookup = func(_ context.Context, name string) ([]string, error) {
		f.nsAsked = append(f.nsAsked, name)
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	// The main DNS server receives its own request, never this server's
	// signed-in caller (the fixture hands the context through).
	fixtureExchange := remoteDNSExchange
	remoteDNSExchange = func(ctx context.Context, endpoint, path, secret string, request, response any) error {
		var own context.Context
		var cancel context.CancelFunc
		if deadline, ok := ctx.Deadline(); ok {
			own, cancel = context.WithDeadline(context.Background(), deadline)
		} else {
			own, cancel = context.WithCancel(context.Background())
		}
		defer cancel()
		return fixtureExchange(own, endpoint, path, secret, request, response)
	}
	remoteDNSExchangeFrom = func(ctx context.Context, source netip.Addr, endpoint, path, secret string, request, response any) error {
		f.fromExchanges++
		if f.fromErr != nil {
			f.exchanges++
			return f.fromErr
		}
		previous := f.sourceIP
		f.sourceIP = source.String()
		if f.fromSeen != "" {
			f.sourceIP = f.fromSeen
		}
		defer func() { f.sourceIP = previous }()
		return remoteDNSExchange(ctx, endpoint, path, secret, request, response)
	}
	f.secondaryDraft(t, "example.test")
	return f
}

// secondaryDraft saves the joined secondary's setup draft (the main domain is
// the main DNS server's infrastructure zone; "" for an older setup).
func (f *dnsAddFixture) secondaryDraft(t *testing.T, mainDomain string) {
	t.Helper()
	draft := setupDNSTestDraft()
	draft.DNSRole, draft.LocalIP, draft.PeerIP, draft.PeerNS = transport.DNSPairRoleSecondary, hostRecordPeerIP, "192.0.2.1", "ns1.example.test"
	draft.DNSPublisherEndpoint = "https://dns.remote.test:2083"
	if mainDomain != "" {
		draft.MainDomain, draft.ServerCount, draft.MainDNSServer = mainDomain, 2, "peer"
	}
	if _, err := canonicalServerSetupDraft(draft); err != nil {
		t.Fatalf("the secondary's draft is not canonical: %v", err)
	}
	raw, _ := json.Marshal(draft)
	if _, err := f.origin.db.GetDB().Exec(`UPDATE server_setup_state SET draft_json=? WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
}

func (f *dnsAddFixture) check(t *testing.T, host string) webmailAddressCheck {
	t.Helper()
	return checkWebmailAddressForTest(t, f.origin, 1, host)
}

type dnsAddAnswer struct {
	status int
	record webmailAddressDNSRecordRequest
	code   string
}

func (f *dnsAddFixture) add(t *testing.T, host, requestID string) dnsAddAnswer {
	t.Helper()
	w := httptest.NewRecorder()
	f.origin.handleWebmailAddressDNSRecord(w, serviceStateAtomicRequest(http.MethodPost, webmailAddressDNSRecordPath, `{"host":"`+host+`","request_id":"`+requestID+`"}`, 1))
	var body struct {
		Record webmailAddressDNSRecordRequest `json:"record"`
		Code   string                         `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("answer %d %s", w.Code, w.Body.String())
	}
	return dnsAddAnswer{status: w.Code, record: body.Record, code: body.Code}
}

func outcomes(record webmailAddressDNSRecordRequest) string {
	parts := []string{}
	for _, r := range record.Records {
		parts = append(parts, strings.TrimSuffix(r.Type+"="+r.Outcome+":"+r.Code, ":"))
	}
	return strings.Join(parts, " ")
}

func (f *dnsAddFixture) receiverAddresses(t *testing.T, name, typ string) []string {
	t.Helper()
	rows, err := f.receiver.db.GetDB().Query(`SELECT content FROM pdns_records WHERE name=? AND type=? ORDER BY id`, name, typ)
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

func TestWebmailAddressCheckOffersTheMainDNSServerOnlyWhenThisServerCanAsk(t *testing.T) {
	f := newDNSAddFixture(t, "")
	check := f.check(t, dnsAddHost)
	if check.State != webmailAddressNotFound || check.PeerZone == nil || check.PeerZone.Server != "dns.remote.test" || check.PeerZone.Zone != "example.test" || check.Zone != nil || check.ManagedBy != nil {
		t.Fatalf("case 2: %+v", check)
	}
	// The record shown is exactly what the main DNS server writes.
	if len(check.Records) != 1 || check.Records[0].TTL != remoteDNSHostRecordTTL || check.Records[0].Name != "mail.boston" || check.Records[0].Text != "mail.boston.example.test. 300 IN A 192.0.2.2" {
		t.Fatalf("records: %+v", check.Records)
	}
	if f.exchanges != 0 || len(f.nsAsked) != 0 {
		t.Fatalf("the check sent %d requests and asked public DNS %v", f.exchanges, f.nsAsked)
	}
	// Outside the joined main domain: elsewhere (the name servers are read).
	other := f.check(t, "webmail.other.test")
	if other.PeerZone != nil || len(f.nsAsked) == 0 || other.Records[0].TTL != 3600 {
		t.Fatalf("outside the zone: %+v asked=%v", other, f.nsAsked)
	}
	// The zone apex is not a host record.
	if apex := f.check(t, "example.test"); apex.PeerZone != nil {
		t.Fatalf("apex: %+v", apex.PeerZone)
	}
	// An older setup without the joined main domain: offered, the other server decides.
	f.secondaryDraft(t, "")
	if old := f.check(t, dnsAddHost); old.PeerZone == nil || old.PeerZone.Zone != "" || old.Records[0].Name != dnsAddHost+"." {
		t.Fatalf("older setup: %+v", old.PeerZone)
	}
	f.secondaryDraft(t, "example.test")
	// Not the pair's secondary, or its verified address is not this server's: no offer.
	webmailAddressPairState = func(*Panel, context.Context) (dnsEngineDBState, error) {
		return dnsEngineDBState{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRolePrimary, LocalIP: hostRecordPeerIP}, nil
	}
	if primary := f.check(t, dnsAddHost); primary.PeerZone != nil {
		t.Fatal("a primary offered to ask")
	}
	webmailAddressPairState = func(*Panel, context.Context) (dnsEngineDBState, error) {
		return dnsEngineDBState{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRoleSecondary, LocalIP: "192.0.2.9"}, nil
	}
	if moved := f.check(t, dnsAddHost); moved.PeerZone != nil {
		t.Fatal("offered with an address the main DNS server would refuse")
	}
	webmailAddressPairState = func(*Panel, context.Context) (dnsEngineDBState, error) {
		return dnsEngineDBState{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRoleSecondary, LocalIP: hostRecordPeerIP}, nil
	}
	// No connection: no offer, the record to copy stays.
	if _, err := f.origin.db.GetDB().Exec(`DELETE FROM remote_dns_connections`); err != nil {
		t.Fatal(err)
	}
	none := f.check(t, dnsAddHost)
	if none.PeerZone != nil || len(none.Records) != 1 || none.Records[0].TTL != 3600 {
		t.Fatalf("no connection: %+v", none)
	}
	if f.exchanges != 0 {
		t.Fatalf("checks sent %d requests", f.exchanges)
	}
}

func TestWebmailAddressManagedByNamesTheClosestZoneOrNothing(t *testing.T) {
	restore := webmailAddressNSLookup
	t.Cleanup(func() { webmailAddressNSLookup = restore })
	var asked []string
	webmailAddressNSLookup = func(_ context.Context, name string) ([]string, error) {
		asked = append(asked, name)
		switch name {
		case "example.co":
			return []string{"NS2.Provider.test.", "ns1.provider.test", "ns1.provider.test"}, nil
		case "sub.example.co":
			return []string{}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	got := webmailAddressManagedByLookup(context.Background(), "mail.sub.example.co")
	if got == nil || got.Zone != "example.co" || strings.Join(got.Nameservers, ",") != "ns1.provider.test,ns2.provider.test" {
		t.Fatalf("managed by: %+v", got)
	}
	if strings.Join(asked, ",") != "mail.sub.example.co,sub.example.co,example.co" {
		t.Fatalf("asked %v", asked)
	}
	// A top-level label is never asked; nothing found says nothing.
	asked = nil
	if got := webmailAddressManagedByLookup(context.Background(), "mail.nowhere"); got != nil || strings.Join(asked, ",") != "mail.nowhere" {
		t.Fatalf("nowhere: %+v asked=%v", got, asked)
	}
	// A lookup that fails is not guessed past.
	webmailAddressNSLookup = func(context.Context, string) ([]string, error) { return nil, errors.New("i/o timeout") }
	if got := webmailAddressManagedByLookup(context.Background(), "mail.example.co"); got != nil {
		t.Fatalf("failed lookup: %+v", got)
	}
}

func TestWebmailAddressDNSRecordIsAskedOnceAndItsAnswerKept(t *testing.T) {
	f := newDNSAddFixture(t, "")
	id := strings.Repeat("a", 32)
	answer := f.add(t, dnsAddHost, id)
	if answer.status != http.StatusOK || outcomes(answer.record) != "A=added" || answer.record.Server != "dns.remote.test" || answer.record.Zone != "example.test" || answer.record.State != "done" {
		t.Fatalf("created: %+v", answer)
	}
	if r := answer.record.Records[0]; r.TTL != remoteDNSHostRecordTTL || r.Text != "mail.boston.example.test. 300 IN A 192.0.2.2" {
		t.Fatalf("record: %+v", r)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "A"); len(got) != 1 || got[0] != hostRecordPeerIP {
		t.Fatalf("receiver records: %v", got)
	}
	// The same request identity again: the kept answer, nothing sent.
	if again := f.add(t, dnsAddHost, id); again.status != http.StatusOK || outcomes(again.record) != "A=added" || f.exchanges != 1 {
		t.Fatalf("replay: %+v exchanges=%d", again, f.exchanges)
	}
	if conflict := f.add(t, "webmail.boston.example.test", id); conflict.status != http.StatusConflict || conflict.code != errCodeWebmailDNSRecordRequestConflict || f.exchanges != 1 {
		t.Fatalf("identity reused: %+v", conflict)
	}
	// A new press for the same name: the record is already there.
	if present := f.add(t, dnsAddHost, strings.Repeat("b", 32)); outcomes(present.record) != "A=present" {
		t.Fatalf("present: %+v", present)
	}
	// Both servers audit it; no credential in any row.
	var requested, added int
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, "webmail.address.dns_record.requested:"+dnsAddHost).Scan(&requested)
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, "webmail.address.dns_record.added:"+dnsAddHost).Scan(&added)
	if requested != 2 || added != 2 {
		t.Fatalf("origin audits requested=%d added=%d", requested, added)
	}
	connection, _ := f.origin.readRemoteDNSConnection(context.Background(), f.id)
	var leaked int
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE COALESCE(details,'') LIKE ?`, "%"+connection.credential+"%").Scan(&leaked)
	if raw := f.origin.setting(context.Background(), settingWebmailAddressDNSRecord); leaked != 0 || strings.Contains(raw, connection.credential) || strings.Contains(raw, f.id) {
		t.Fatalf("credential stored: audits=%d setting=%s", leaked, raw)
	}
	var receiverAudits int
	f.receiver.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'dns.remote.host_record.%'`).Scan(&receiverAudits)
	if receiverAudits != 2 {
		t.Fatalf("receiver audits=%d", receiverAudits)
	}
}

func TestWebmailAddressDNSRecordAddsTheIPv6RecordOnlyWhenTheMainServerAcceptsIt(t *testing.T) {
	f := newDNSAddFixture(t, "2001:4860::5")
	if check := f.check(t, dnsAddHost); len(check.Records) != 2 || check.Records[1].Text != "mail.boston.example.test. 300 IN AAAA 2001:4860::5" {
		t.Fatalf("check records: %+v", check.Records)
	}
	answer := f.add(t, dnsAddHost, strings.Repeat("a", 32))
	if outcomes(answer.record) != "A=added AAAA=added" || f.fromExchanges != 1 {
		t.Fatalf("both: %+v from=%d", answer.record, f.fromExchanges)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "AAAA"); len(got) != 1 || got[0] != "2001:4860::5" {
		t.Fatalf("receiver AAAA: %v", got)
	}
	// The request reached the main server from another address: AAAA refused, A kept.
	g := newDNSAddFixture(t, "2001:4860::5")
	g.fromSeen = "2001:4860::77"
	answer = g.add(t, "webmail.boston.example.test", strings.Repeat("c", 32))
	if outcomes(answer.record) != "A=added AAAA=refused:address" || len(g.receiverAddresses(t, "webmail.boston.example.test", "AAAA")) != 0 {
		t.Fatalf("not acceptable: %+v", answer.record)
	}
	// The main server cannot be reached over IPv6 from this address: not sent.
	h := newDNSAddFixture(t, "2001:4860::5")
	h.fromErr = &remoteDNSExchangeError{Delivered: false}
	if answer = h.add(t, dnsAddHost, strings.Repeat("d", 32)); outcomes(answer.record) != "A=added AAAA=not_sent:unreachable" {
		t.Fatalf("no IPv6 route: %+v", answer.record)
	}
}

func TestWebmailAddressDNSRecordSaysEachRefusalAndWritesNothing(t *testing.T) {
	cases := []struct {
		name, host, want string
		setup            func(*dnsAddFixture)
	}{
		{"outside the zone", "mail.boston.other.test", "outside_zone", func(f *dnsAddFixture) { f.secondaryDraft(t, "") }},
		{"name taken by another record", "webmail.example.test", "taken", func(f *dnsAddFixture) {
			f.receiver.db.GetDB().Exec(`INSERT INTO pdns_records(domain_id,name,type,content,ttl) SELECT id,'webmail.example.test','CNAME','elsewhere.test',300 FROM pdns_domains WHERE name='example.test'`)
		}},
		{"address not acceptable", dnsAddHost, "address", func(f *dnsAddFixture) {
			remoteDNSReadLocalAuthority = func(*Panel, context.Context) (remoteDNSAuthority, error) {
				return remoteDNSAuthority{Ready: true, Engine: "bind", Epoch: 1, Nameservers: []string{"ns1.example.test", "ns2.example.test"}, SecondaryIP: "192.0.2.3"}, nil
			}
		}},
		{"main server not ready", dnsAddHost, "not_ready", func(f *dnsAddFixture) {
			remoteDNSReadLocalAuthority = func(*Panel, context.Context) (remoteDNSAuthority, error) {
				return remoteDNSAuthority{}, errRemoteDNSNotReady
			}
		}},
		{"connection revoked", dnsAddHost, "unauthorized", func(f *dnsAddFixture) {
			f.receiver.db.GetDB().Exec(`UPDATE remote_dns_clients SET revoked=1`)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newDNSAddFixture(t, "2001:4860::5")
			c.setup(f)
			answer := f.add(t, c.host, strings.Repeat("e", 32))
			if answer.status != http.StatusOK || outcomes(answer.record) != "A=refused:"+c.want+" AAAA=not_sent:skipped" || f.fromExchanges != 0 {
				t.Fatalf("%+v from=%d", answer, f.fromExchanges)
			}
			if c.want != "taken" && len(f.receiverAddresses(t, c.host, "A")) != 0 {
				t.Fatal("a refused request wrote a record")
			}
			if f.publishes != 0 {
				t.Fatal("a refused request published the zone")
			}
			var refused int
			f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, "webmail.address.dns_record.refused:"+c.host).Scan(&refused)
			if refused != 1 {
				t.Fatalf("refused audits=%d", refused)
			}
		})
	}
}

func TestWebmailAddressDNSRecordNeverOverwritesAnOwnersChange(t *testing.T) {
	f := newDNSAddFixture(t, "")
	if answer := f.add(t, dnsAddHost, strings.Repeat("a", 32)); outcomes(answer.record) != "A=added" {
		t.Fatalf("first: %+v", answer.record)
	}
	// The owner points the name elsewhere on the main server.
	if _, err := f.receiver.db.GetDB().Exec(`UPDATE pdns_records SET content='198.51.100.9' WHERE name=?`, dnsAddHost); err != nil {
		t.Fatal(err)
	}
	if answer := f.add(t, dnsAddHost, strings.Repeat("b", 32)); outcomes(answer.record) != "A=refused:taken" {
		t.Fatalf("owner change: %+v", answer.record)
	}
	if got := f.receiverAddresses(t, dnsAddHost, "A"); len(got) != 1 || got[0] != "198.51.100.9" {
		t.Fatalf("owner's record changed: %v", got)
	}
	// The owner removes it: not created again behind the owner's back.
	f.receiver.db.GetDB().Exec(`DELETE FROM pdns_records WHERE name=?`, dnsAddHost)
	if answer := f.add(t, dnsAddHost, strings.Repeat("c", 32)); outcomes(answer.record) != "A=refused:taken" || len(f.receiverAddresses(t, dnsAddHost, "A")) != 0 {
		t.Fatalf("removed record: %+v", answer.record)
	}
}

func TestWebmailAddressDNSRecordWithoutAnAnswerIsUnknownAndNotSentAgain(t *testing.T) {
	f := newDNSAddFixture(t, "2001:4860::5")
	f.down = true
	id := strings.Repeat("a", 32)
	answer := f.add(t, dnsAddHost, id)
	if outcomes(answer.record) != "A=unknown AAAA=not_sent:skipped" || f.exchanges != 1 {
		t.Fatalf("no answer: %+v exchanges=%d", answer.record, f.exchanges)
	}
	if again := f.add(t, dnsAddHost, id); outcomes(again.record) != "A=unknown AAAA=not_sent:skipped" || f.exchanges != 1 {
		t.Fatalf("replay sent again: %+v exchanges=%d", again.record, f.exchanges)
	}
	var unknown int
	f.origin.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action=?`, "webmail.address.dns_record.unknown:"+dnsAddHost).Scan(&unknown)
	if unknown != 1 {
		t.Fatalf("unknown audits=%d", unknown)
	}
	// Interrupted by a restart while it was sent: unknown, never sent by itself.
	stored, _ := f.origin.readWebmailAddressDNSRecordRequest(context.Background())
	stored.RequestID, stored.State = strings.Repeat("f", 32), "sending"
	stored.Records[0].Outcome = ""
	if err := f.origin.saveWebmailAddressDNSRecordRequest(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	f.down = false
	if interrupted := f.add(t, dnsAddHost, stored.RequestID); outcomes(interrupted.record) != "A=unknown AAAA=unknown" || f.exchanges != 1 {
		t.Fatalf("interrupted: %+v exchanges=%d", interrupted.record, f.exchanges)
	}
}

func TestWebmailAddressDNSRecordIsRefusedHereWhenThisServerCannotAsk(t *testing.T) {
	f := newDNSAddFixture(t, "")
	if _, err := f.origin.db.GetDB().Exec(`DELETE FROM remote_dns_connections`); err != nil {
		t.Fatal(err)
	}
	if answer := f.add(t, dnsAddHost, strings.Repeat("a", 32)); answer.status != http.StatusConflict || answer.code != errCodeWebmailDNSRecordNotOffered {
		t.Fatalf("no connection: %+v", answer)
	}
	// A zone hosted here is added through its own editor, never asked of the other server.
	g := newDNSAddFixture(t, "")
	seedInfrastructureDNSZone(t, g.origin, "boston.example.test")
	if _, err := g.origin.db.GetDB().Exec(`INSERT INTO panel_settings(key,value) SELECT 'server_setup_infrastructure_zone:boston.example.test', '{"zone":"boston.example.test","zone_id":' || id || '}' FROM pdns_domains WHERE name='boston.example.test'`); err != nil {
		t.Fatal(err)
	}
	if check := g.check(t, dnsAddHost); check.Zone == nil || check.PeerZone != nil {
		t.Fatalf("a zone hosted here is case 1: %+v", check)
	}
	if answer := g.add(t, dnsAddHost, strings.Repeat("b", 32)); answer.status != http.StatusConflict || answer.code != errCodeWebmailDNSRecordNotOffered {
		t.Fatalf("local zone: %+v", answer)
	}
	if f.exchanges+g.exchanges != 0 {
		t.Fatal("a refused request was sent")
	}
	if !isAdminOnlyPath(webmailAddressDNSRecordPath) {
		t.Fatal("the request is not administrator only")
	}
}

func TestWebmailAddressDNSRecordOutcomeReadsEveryTypedAnswer(t *testing.T) {
	want := remoteDNSHostRecordRequest{Name: dnsAddHost, Type: "A", Content: hostRecordPeerIP}
	good := remoteDNSHostRecordAnswer{ClientID: "c", Name: want.Name, Type: "A", Content: want.Content, Applied: true, Result: "created"}
	for name, c := range map[string]struct {
		err           error
		answer        remoteDNSHostRecordAnswer
		outcome, code string
	}{
		"created":         {nil, good, "added", ""},
		"other client":    {nil, remoteDNSHostRecordAnswer{ClientID: "x", Name: want.Name, Type: "A", Content: want.Content, Applied: true, Result: "created"}, "unknown", ""},
		"pending":         {&remoteDNSHTTPError{StatusCode: 409, Code: errCodeRemoteDNSHostRecordPending}, remoteDNSHostRecordAnswer{}, "saved", ""},
		"invalid":         {&remoteDNSHTTPError{StatusCode: 400, Code: errCodeRemoteDNSHostRecordInvalid}, remoteDNSHostRecordAnswer{}, "refused", "invalid"},
		"licence":         {&remoteDNSHTTPError{StatusCode: 403, Code: "license_required"}, remoteDNSHostRecordAnswer{}, "refused", "license"},
		"licence unknown": {&remoteDNSHTTPError{StatusCode: 503, Code: errCodeLicenseStatusUnavailable}, remoteDNSHostRecordAnswer{}, "refused", "license"},
		"older primary":   {&remoteDNSHTTPError{StatusCode: 404}, remoteDNSHostRecordAnswer{}, "refused", "unsupported"},
		"server error":    {&remoteDNSHTTPError{StatusCode: 502}, remoteDNSHostRecordAnswer{}, "unknown", ""},
		"delivered":       {&remoteDNSExchangeError{Delivered: true}, remoteDNSHostRecordAnswer{}, "unknown", ""},
		"not delivered":   {&remoteDNSExchangeError{Delivered: false}, remoteDNSHostRecordAnswer{}, "not_sent", "unreachable"},
		"after answer":    {errors.New("remote DNS response does not match its contract"), remoteDNSHostRecordAnswer{}, "unknown", ""},
	} {
		if outcome, code := webmailAddressDNSRecordOutcome(c.err, c.answer, "c", want); outcome != c.outcome || code != c.code {
			t.Errorf("%s: %s/%s want %s/%s", name, outcome, code, c.outcome, c.code)
		}
	}
}

func TestRemoteDNSSourceDialNeverDialsTheOtherFamily(t *testing.T) {
	dialled := []string{}
	dial := remoteDNSSourceDial(netip.MustParseAddr("2001:4860::5"), func(_ context.Context, _, address string) (net.Conn, error) {
		dialled = append(dialled, address)
		return nil, errors.New("test dial")
	})
	if _, err := dial(context.Background(), "tcp", "192.0.2.1:2083"); err == nil || len(dialled) != 0 {
		t.Fatalf("an IPv4 target was dialled from an IPv6 source: %v", dialled)
	}
	dial(context.Background(), "tcp", "[2001:4860::1]:2083")
	if len(dialled) != 1 {
		t.Fatalf("the IPv6 target was not dialled: %v", dialled)
	}
}
