package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/dnswire"
	"github.com/alicelik/celikpanel/internal/dnswire/dnswiretest"
)

// The DNS page's overview (alpha.115) against loopback DNS servers and
// controlled public resolvers only; nothing leaves the machine.

// overviewWorld is the internet as the check sees it: the public
// resolvers' answers ("name TYPE" → values; absent = no such record), names
// whose lookup fails, the registry's delegation and how each of our DNS
// servers answers ("ok", "refused"; absent = unreachable).
type overviewWorld struct {
	mu       sync.Mutex
	public   map[string][]string
	failing  map[string]bool
	asked    atomic.Int32
	parentNS []string
	glue     map[string]string
	servers  map[string]string
}

func newOverviewWorld() *overviewWorld {
	return &overviewWorld{public: map[string][]string{}, failing: map[string]bool{}, glue: map[string]string{}, servers: map[string]string{}}
}

func (w *overviewWorld) set(fn func(*overviewWorld)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(w)
}

func (w *overviewWorld) seams(t *testing.T) *dnsOverviewSeams {
	t.Helper()
	typeName := map[uint16]string{}
	for name, value := range serverSetupRecordTypes {
		typeName[value] = name
	}
	lookup := func(_ context.Context, name string, qtype uint16) ([]string, error) {
		w.asked.Add(1)
		w.mu.Lock()
		defer w.mu.Unlock()
		key := name + " " + typeName[qtype]
		if w.failing[key] || w.failing[name] {
			return nil, errors.New("timeout")
		}
		if values, ok := w.public[key]; ok {
			return append([]string(nil), values...), nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	resolver := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		switch {
		case qtype == dnswiretest.TypeSOA && name == "test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: "test", Type: dnswiretest.TypeSOA, Value: "ns.nic.test hostmaster.nic.test 1 3600 600 604800 300"}}}
		case qtype == dnswiretest.TypeNS && name == "test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: "test", Type: dnswiretest.TypeNS, Value: "ns.nic.test"}}}
		case qtype == dnswiretest.TypeA && name == "ns.nic.test":
			return dnswiretest.Reply{Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeA, Value: "127.0.0.1"}}}
		}
		return dnswiretest.Reply{RCode: 3}
	})
	parent := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if recursive || name != "example.test" || qtype != dnswiretest.TypeNS {
			return dnswiretest.Reply{RCode: 5}
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		reply := dnswiretest.Reply{}
		for _, ns := range w.parentNS {
			reply.Authority = append(reply.Authority, dnswiretest.RR{Name: name, Type: dnswiretest.TypeNS, Value: ns})
		}
		for ns, ip := range w.glue {
			reply.Additional = append(reply.Additional, dnswiretest.RR{Name: ns, Type: dnswiretest.TypeA, Value: ip})
		}
		return reply
	})
	authoritative := dnswiretest.Start(t, func(name string, qtype uint16, recursive bool) dnswiretest.Reply {
		if recursive {
			return dnswiretest.Reply{RCode: 5}
		}
		if name == "example.test" && qtype == dnswiretest.TypeSOA {
			return dnswiretest.Reply{Authoritative: true, Answer: []dnswiretest.RR{{Name: name, Type: dnswiretest.TypeSOA, Value: "ns1.example.test hostmaster.example.test 2026102801 3600 600 604800 300"}}}
		}
		return dnswiretest.Reply{Authoritative: true, RCode: 3}
	})
	refusing := dnswiretest.Start(t, func(string, uint16, bool) dnswiretest.Reply { return dnswiretest.Reply{RCode: 5} })
	options := dnswire.DelegationOptions{Resolvers: []string{resolver.Address}, Address: func(ip string) string {
		if ip == "127.0.0.1" {
			return parent.Address
		}
		w.mu.Lock()
		defer w.mu.Unlock()
		switch w.servers[ip] {
		case "ok":
			return authoritative.Address
		case "refused":
			return refusing.Address
		}
		return "127.0.0.1:1"
	}}
	return &dnsOverviewSeams{Lookups: []setupProviderRecordLookup{lookup, lookup}, Delegation: options,
		IPv4: func() string { return "203.0.113.10" }, IPv6: func() string { return "" },
		DKIM: func(context.Context, string) (string, error) { return "", nil }}
}

func readDNSOverview(t *testing.T, p *Panel) dnsOverview {
	t.Helper()
	w := httptest.NewRecorder()
	p.handleDNSOverview(w, infraAdminRequest(http.MethodGet, dnsOverviewPath, ""))
	var value dnsOverview
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(w.Body.Bytes(), &value) != nil {
		t.Fatalf("overview: %d %s", w.Code, w.Body.String())
	}
	return value
}

func overviewName(value dnsOverview, kind, name string) dnsOverviewName {
	for _, row := range value.Names {
		if row.Kind == kind && row.Name == name {
			return row
		}
	}
	return dnsOverviewName{}
}

func overviewServer(value dnsOverview, who string) dnsOverviewServer {
	for _, server := range value.Servers {
		if server.Who == who {
			return server
		}
	}
	return dnsOverviewServer{}
}

// overviewPairedFirst: the first server, main DNS server, with the second
// server ("beta", 198.51.100.20) bound to its finished setup.
func overviewPairedFirst(t *testing.T) (serviceOperationTestFixture, *overviewWorld) {
	t.Helper()
	f, plan, execution := joinFirstFixture(t, joinFirstDraft())
	binding := serverSetupPeerBinding{ExecutionID: execution.ID, PlanID: plan.ID, JoinRequestID: strings.Repeat("8", 32), PeerIPv4: "198.51.100.20", PeerShortName: "beta",
		PeerPanelDomain: "beta.example.test", PeerAddress: "https://beta.example.test:2083", PeerPin: strings.Repeat("ab", 32), PeerDNSEngine: "bind"}
	if err := writeServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupPeerBindingKey(execution.ID), binding); err != nil {
		t.Fatal(err)
	}
	execution.Status = "succeeded"
	raw, _ := json.Marshal(execution)
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_executions SET status='succeeded', execution_json=? WHERE id=?`, string(raw), execution.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='ready', completed_at='2026-10-28T00:00:00Z' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	world := newOverviewWorld()
	world.set(func(w *overviewWorld) {
		w.parentNS = []string{"ns1.example.test", "ns2.example.test"}
		w.glue = map[string]string{"ns1.example.test": "203.0.113.10", "ns2.example.test": "198.51.100.20"}
		w.servers = map[string]string{"203.0.113.10": "ok", "198.51.100.20": "ok"}
		for name, ip := range map[string]string{"panel.example.test": "203.0.113.10", "ns1.example.test": "203.0.113.10", "ns2.example.test": "198.51.100.20", "beta.example.test": "198.51.100.20"} {
			w.public[name+" A"] = []string{ip}
		}
	})
	f.panel.dnsOverviewSeams = world.seams(t)
	return f, world
}

func TestDNSOverviewPairedMainServerAllGood(t *testing.T) {
	f, world := overviewPairedFirst(t)
	seedInfrastructureDNSZone(t, f.panel, "example.test")
	var zoneID int64
	if err := f.database.GetDB().QueryRow(`SELECT id FROM pdns_domains WHERE name='example.test'`).Scan(&zoneID); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(serverSetupInfrastructureDNSOwner{Zone: "example.test", ZoneID: zoneID, PlanID: infrastructureTestPlanID})
	if _, err := f.database.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSOwnerKey("example.test"), string(owner)); err != nil {
		t.Fatal(err)
	}
	value := readDNSOverview(t, f.panel)
	if value.Sentence.State != "ok" || value.Sentence.Code != "dns_overview_ok" || value.Home != "own" || !value.SetupReady {
		t.Fatalf("sentence: %+v", value.Sentence)
	}
	this, peer := overviewServer(value, "this"), overviewServer(value, "peer")
	if this.Role != "main" || this.ShortName != "alpha" || this.Answers != "ok" || this.Serial != 2026102801 || peer.Role != "backup" || peer.ShortName != "beta" || peer.Software != "bind" || peer.Answers != "ok" {
		t.Fatalf("servers: %+v", value.Servers)
	}
	for _, check := range []struct{ kind, name, server string }{{"panel", "panel.example.test", "this"}, {"panel", "beta.example.test", "peer"}, {"ns", "ns1.example.test", "this"}, {"ns", "ns2.example.test", "peer"}} {
		row := overviewName(value, check.kind, check.name)
		if row.State != "ok" || row.Server != check.server || row.Where != "zone" {
			t.Fatalf("%s %s: %+v", check.kind, check.name, row)
		}
	}
	if row := overviewName(value, "nameservers", "example.test"); row.State != "ok" || row.Where != "registrar" {
		t.Fatalf("nameservers: %+v", row)
	}
	if row := overviewName(value, "glue", "ns2.example.test"); row.State != "ok" || row.Expected != "198.51.100.20" {
		t.Fatalf("glue: %+v", row)
	}
	if value.Join.State != "bound" || value.OwnRecordsLink.Where != "here" || value.OwnRecordsLink.Path != "/domains/infrastructure/example.test" || value.OwnRecordsLink.API != infrastructureDNSZonesPath+"/example.test" {
		t.Fatalf("join/own records: %+v %+v", value.Join, value.OwnRecordsLink)
	}
	if value.MailReceiver.Server != "none" || len(value.Mail) != 0 || value.MXMove != nil {
		t.Fatalf("no mail on either server: %+v %+v", value.MailReceiver, value.Mail)
	}
	// A poll within the minute is the shared answer: nothing is asked again.
	asked := world.asked.Load()
	readDNSOverview(t, f.panel)
	if world.asked.Load() != asked {
		t.Fatal("a poll within a minute asked the internet again")
	}
	// Reading changes nothing: no agent work, no setup change.
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("the overview mutated the host")
	}
}

func TestDNSOverviewSentencePutsAVerifiedFailureFirst(t *testing.T) {
	f, world := overviewPairedFirst(t)
	now := time.Now()
	read := func() dnsOverview {
		now = now.Add(2 * time.Minute)
		value, err := f.panel.dnsOverview(context.Background(), now, false)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	// The backup answers, not for the domain, while the registry also points
	// elsewhere: the verified failure is said first.
	world.set(func(w *overviewWorld) {
		w.servers["198.51.100.20"] = "refused"
		w.parentNS = []string{"ns1.parking.example", "ns2.parking.example"}
		w.glue = map[string]string{}
	})
	if value := read(); value.Sentence.State != "failed" || value.Sentence.Code != "dns_overview_backup_not_serving" || overviewServer(value, "peer").Answers != "not_serving" || value.Sentence.Vars["backup"] != "beta" {
		t.Fatalf("backup failure: %+v %+v", value.Sentence, value.Servers)
	}
	// The backup serves again: what the owner must do at the registrar.
	world.set(func(w *overviewWorld) { w.servers["198.51.100.20"] = "ok" })
	value := read()
	if value.Sentence.State != "prerequisite" || value.Sentence.Code != "dns_overview_registrar_not_pointing" || !strings.Contains(value.Sentence.Vars["names"], "ns1.example.test") {
		t.Fatalf("registrar: %+v", value.Sentence)
	}
	if row := overviewName(value, "nameservers", "example.test"); row.State != "not_yet" || !strings.Contains(strings.Join(row.Seen, ","), "parking") {
		t.Fatalf("nameservers row: %+v", row)
	}
	// A backup that cannot be reached is unknown, never "not answering".
	world.set(func(w *overviewWorld) {
		delete(w.servers, "198.51.100.20")
		w.parentNS = []string{"ns1.example.test", "ns2.example.test"}
		w.glue = map[string]string{"ns1.example.test": "203.0.113.10", "ns2.example.test": "198.51.100.20"}
	})
	if value := read(); value.Sentence.State != "unknown" || value.Sentence.Code != "dns_overview_server_unknown" || overviewServer(value, "peer").Answers != "unknown" {
		t.Fatalf("unreachable backup: %+v %+v", value.Sentence, value.Servers)
	}
	// A public lookup that fails is unknown, never missing.
	world.set(func(w *overviewWorld) {
		w.servers["198.51.100.20"] = "ok"
		w.failing["beta.example.test"] = true
	})
	value = read()
	if row := overviewName(value, "panel", "beta.example.test"); row.State != "unknown" || value.Sentence.Code != "dns_overview_some_unknown" {
		t.Fatalf("failed lookup: %+v %+v", row, value.Sentence)
	}
	// A name the internet does not show yet is a spreading change: nobody acts.
	world.set(func(w *overviewWorld) {
		delete(w.failing, "beta.example.test")
		delete(w.public, "beta.example.test A")
	})
	if value := read(); value.Sentence.State != "waiting" || value.Sentence.Code != "dns_overview_spreading" || overviewName(value, "panel", "beta.example.test").State != "missing" {
		t.Fatalf("spreading: %+v", value.Sentence)
	}
}

func TestDNSOverviewWhileSetupWaitsForTheSecondServer(t *testing.T) {
	f, _, _ := joinFirstFixture(t, joinFirstDraft())
	world := newOverviewWorld()
	f.panel.dnsOverviewSeams = world.seams(t)
	value := readDNSOverview(t, f.panel)
	if value.Join.State != "waiting" || value.Sentence.Code != "dns_overview_join_waiting" || value.Sentence.State != "prerequisite" || !strings.Contains(value.Sentence.Next, "join code") {
		t.Fatalf("waiting join: %+v %+v", value.Join, value.Sentence)
	}
	if peer := overviewServer(value, "peer"); peer.Answers != "not_joined" || peer.ShortName != "" || peer.PanelName != "" {
		t.Fatalf("the second server is not named before it joins: %+v", peer)
	}
}

func overviewDraftFixture(t *testing.T, d serverSetupDraft) (serviceOperationTestFixture, *overviewWorld) {
	t.Helper()
	f, _ := setupOperationFixture(t)
	raw, _ := json.Marshal(d)
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET draft_json=?, status='ready', completed_at='2026-10-28T00:00:00Z' WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	world := newOverviewWorld()
	f.panel.dnsOverviewSeams = world.seams(t)
	return f, world
}

func TestDNSOverviewExternalProviderNamesTheRecordsAndNoDNSSoftware(t *testing.T) {
	d := serverSetupDraft{Purpose: "web", PanelDomain: "panel.example.test", DNSMode: "external", MainDomain: "example.test", ShortName: "alpha", ServerCount: 1}
	f, world := overviewDraftFixture(t, d)
	value := readDNSOverview(t, f.panel)
	if value.Home != "provider" || value.Sentence.State != "prerequisite" || value.Sentence.Code != "dns_overview_provider_records" || value.Sentence.Vars["value"] != "203.0.113.10" {
		t.Fatalf("provider sentence: %+v", value.Sentence)
	}
	this := overviewServer(value, "this")
	if this.Role != "none" || this.Software != "" || this.Answers != "not_applicable" || len(value.Servers) != 1 {
		t.Fatalf("no DNS server claims software: %+v", value.Servers)
	}
	row := overviewName(value, "panel", "panel.example.test")
	if row.State != "missing" || row.Where != "provider" || row.Expected != "203.0.113.10" {
		t.Fatalf("provider row: %+v", row)
	}
	for _, name := range value.Names {
		if name.Kind == "nameservers" || name.Kind == "glue" {
			t.Fatalf("no registrar check without own DNS servers: %+v", name)
		}
	}
	if value.OwnRecordsLink.Where != "provider" || value.Join.State != "unavailable" || value.Join.Reason != "external" {
		t.Fatalf("own records/join: %+v %+v", value.OwnRecordsLink, value.Join)
	}
	// Once the provider has it, and a failed lookup stays unknown.
	world.set(func(w *overviewWorld) { w.public["panel.example.test A"] = []string{"203.0.113.10"} })
	later, err := f.panel.dnsOverview(context.Background(), time.Now().Add(2*time.Minute), false)
	if err != nil || later.Sentence.Code != "dns_overview_ok" {
		t.Fatalf("provider done: %+v %v", later.Sentence, err)
	}
	world.set(func(w *overviewWorld) { w.failing["panel.example.test"] = true })
	failed, _ := f.panel.dnsOverview(context.Background(), time.Now().Add(4*time.Minute), false)
	if failed.Sentence.State != "unknown" || overviewName(failed, "panel", "panel.example.test").State != "unknown" {
		t.Fatalf("a failed lookup must be unknown: %+v", failed.Sentence)
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("the overview mutated the host")
	}
}

func TestDNSOverviewKeptOldMailOffersTheMoveWithTheEditor(t *testing.T) {
	d := mailDraftForTest()
	d.ServerCount, d.ShortName, d.PeerIP, d.PeerNS = 1, "alpha", "", ""
	d.MailDNS = &serverSetupMailDNS{MX: "later", DMARC: "keep"}
	f, world := overviewDraftFixture(t, d)
	seedInfrastructureDNSZone(t, f.panel, "example.test")
	var zoneID int64
	if err := f.database.GetDB().QueryRow(`SELECT id FROM pdns_domains WHERE name='example.test'`).Scan(&zoneID); err != nil {
		t.Fatal(err)
	}
	owner, _ := json.Marshal(serverSetupInfrastructureDNSOwner{Zone: "example.test", ZoneID: zoneID, PlanID: infrastructureTestPlanID})
	if _, err := f.database.GetDB().Exec(`INSERT INTO panel_settings(key,value) VALUES(?,?)`, serverSetupInfrastructureDNSOwnerKey("example.test"), string(owner)); err != nil {
		t.Fatal(err)
	}
	key := "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAtest"
	f.panel.dnsOverviewSeams.DKIM = func(context.Context, string) (string, error) { return key, nil }
	world.set(func(w *overviewWorld) {
		w.parentNS = []string{"ns1.example.test", "ns2.example.test"}
		w.glue = map[string]string{"ns1.example.test": "203.0.113.10", "ns2.example.test": "203.0.113.10"}
		w.servers = map[string]string{"203.0.113.10": "ok"}
		for _, name := range []string{"panel.example.test", "mail.example.test", "ns1.example.test", "ns2.example.test"} {
			w.public[name+" A"] = []string{"203.0.113.10"}
		}
		w.public["example.test MX"] = []string{"10 mx.oldhost.example"}
		w.public["example.test TXT"] = []string{"v=spf1 mx a:mail.example.test ~all"}
		w.public["_dmarc.example.test TXT"] = []string{"v=DMARC1; p=none"}
		w.public["celik._domainkey.example.test TXT"] = []string{dkimRecommended(key)}
	})
	value := readDNSOverview(t, f.panel)
	states := map[string]string{}
	for _, mail := range value.Mail {
		states[mail.Kind] = mail.State
	}
	if states["mx"] != "kept" || states["spf"] != "ok" || states["dmarc"] != "ok" || states["dkim"] != "ok" || value.MailReceiver.Server != "this" || value.MailReceiver.Host != "mail.example.test" {
		t.Fatalf("mail: %+v %+v", value.Mail, value.MailReceiver)
	}
	if value.MXMove == nil || value.MXMove.ToHost != "mail.example.test" || value.MXMove.Zone != "example.test" || value.Sentence.Code != "dns_overview_ok_mail_kept" {
		t.Fatalf("move offer: %+v %+v", value.MXMove, value.Sentence)
	}
	if this := overviewServer(value, "this"); this.Role != "standalone" || this.Answers != "ok" || len(value.Servers) != 1 {
		t.Fatalf("one server: %+v", value.Servers)
	}
	if value.Join.State != "unavailable" || value.Join.Reason != "standalone_bind" {
		t.Fatalf("a running BIND cannot gain a backup here: %+v", value.Join)
	}
	if strings.Contains(mustJSON(t, value), "PRIVATE") {
		t.Fatal("a private key reached the answer")
	}
}

func TestDNSOverviewSecondServerIsTheBackupAndKeepsSecretsOut(t *testing.T) {
	d := serverSetupDraft{Purpose: "web", PanelDomain: "beta.example.test", DNSMode: "local", DNSEngine: "bind", DNSRole: "secondary", MainDNSServer: "peer", NS1: "ns1.example.test", NS2: "ns2.example.test",
		LocalIP: "198.51.100.20", PeerIP: "203.0.113.10", PeerNS: "ns1.example.test", MainDomain: "example.test", ServerCount: 2, ShortName: "beta"}
	f, world := overviewDraftFixture(t, d)
	link := serverSetupLinkRecord{RequestID: strings.Repeat("4", 32), Address: "https://panel.example.test:2083", Pin: strings.Repeat("cd", 32), LinkID: "link-1", LinkCredential: "secret-link-credential", EnrollmentCode: "secret-enrollment-code",
		Document: serverSetupJoinDocument{MainDomain: "example.test", ServerCount: 2, DNSMode: "local", MainDNSServer: "first", NS1: "ns1.example.test", NS2: "ns2.example.test", LinkCredential: "secret-link-credential",
			First: serverSetupJoinFirst{IPv4: "203.0.113.10", ShortName: "alpha", PanelDomain: "panel.example.test", DNSEngine: "pdns"}}}
	if err := writeServerSetupJSONSetting(context.Background(), f.database.GetDB(), serverSetupLinkKey, link); err != nil {
		t.Fatal(err)
	}
	world.set(func(w *overviewWorld) {
		w.parentNS = []string{"ns1.example.test", "ns2.example.test"}
		w.glue = map[string]string{"ns1.example.test": "203.0.113.10", "ns2.example.test": "198.51.100.20"}
		w.servers = map[string]string{"203.0.113.10": "ok", "198.51.100.20": "ok"}
		for name, ip := range map[string]string{"panel.example.test": "203.0.113.10", "ns1.example.test": "203.0.113.10", "ns2.example.test": "198.51.100.20", "beta.example.test": "198.51.100.20"} {
			w.public[name+" A"] = []string{ip}
		}
	})
	w := httptest.NewRecorder()
	f.panel.handleDNSOverview(w, infraAdminRequest(http.MethodGet, dnsOverviewPath, ""))
	for _, secret := range []string{"secret-link-credential", "secret-enrollment-code", "link_credential", "enrollment"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("the answer carries %q", secret)
		}
	}
	var value dnsOverview
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	this, peer := overviewServer(value, "this"), overviewServer(value, "peer")
	if this.Role != "backup" || this.Answers != "ok" || peer.Role != "main" || peer.ShortName != "alpha" || peer.Software != "pdns" || peer.Answers != "ok" {
		t.Fatalf("servers: %+v", value.Servers)
	}
	if value.OwnRecordsLink.Where != "main" || value.OwnRecordsLink.PanelName != "panel.example.test" || value.Join.State != "bound" || value.Sentence.Code != "dns_overview_ok" {
		t.Fatalf("backup view: %+v %+v %+v", value.OwnRecordsLink, value.Join, value.Sentence)
	}
	if row := overviewName(value, "ns", "ns1.example.test"); row.Server != "peer" || row.Where != "main" || row.State != "ok" {
		t.Fatalf("ns1 is the main's: %+v", row)
	}
}

func TestDNSOverviewAdministratorReadOnly(t *testing.T) {
	f, _ := overviewDraftFixture(t, serverSetupDraft{Purpose: "web", PanelDomain: "panel.example.test", DNSMode: "external"})
	value := readDNSOverview(t, f.panel)
	if value.Sentence.Code != "dns_overview_setup_not_done" || value.Sentence.State != "prerequisite" || value.Join.Reason != "setup_not_done" {
		t.Fatalf("without a main domain: %+v", value.Sentence)
	}
	r := httptest.NewRequest(http.MethodGet, dnsOverviewPath, nil)
	r = r.WithContext(context.WithValue(r.Context(), callerKey, &Caller{Role: roleCustomer}))
	w := httptest.NewRecorder()
	f.panel.handleDNSOverview(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("customer reached the overview: %d", w.Code)
	}
	w = httptest.NewRecorder()
	f.panel.handleDNSOverview(w, infraAdminRequest(http.MethodPost, dnsOverviewPath, "{}"))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
	if !isAdminOnlyPath(dnsOverviewPath) {
		t.Fatal("the overview must be administrator-only")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDNSOverviewConcurrentReadsShareOneCheckAndForcedChecksAreBounded(t *testing.T) {
	f, world := overviewPairedFirst(t)
	now := time.Now()
	var wg sync.WaitGroup
	results := make([]dnsOverview, 16)
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			value, err := f.panel.dnsOverview(context.Background(), now, index%2 == 0)
			if err != nil {
				t.Error(err)
			}
			results[index] = value
		}(index)
	}
	wg.Wait()
	for _, value := range results {
		if value.Sentence.Code != "dns_overview_ok" {
			t.Fatalf("concurrent read: %+v", value.Sentence)
		}
	}
	asked := world.asked.Load()
	// A forced check inside the 15 s window is the shared answer, with when
	// the next forced check is accepted.
	inside, _ := f.panel.dnsOverview(context.Background(), now.Add(5*time.Second), true)
	if world.asked.Load() != asked || inside.FreshAfter != 10 || inside.NextCheckAfter != 55 {
		t.Fatalf("forced inside the window: asked %d→%d fresh_after=%d next=%d", asked, world.asked.Load(), inside.FreshAfter, inside.NextCheckAfter)
	}
	// After the window a forced check asks the internet again, inside the minute.
	if _, err := f.panel.dnsOverview(context.Background(), now.Add(20*time.Second), true); err != nil || world.asked.Load() == asked {
		t.Fatalf("a forced check after 15 s did not check: %v", err)
	}
	if f.panel.dnsOverviewInFlight() {
		t.Fatal("a check stayed in flight")
	}
}

func TestDNSOverviewACheckThatPanicsWakesItsReaders(t *testing.T) {
	d := mailDraftForTest()
	d.ServerCount, d.ShortName, d.PeerIP, d.PeerNS = 1, "alpha", "", ""
	f, _ := overviewDraftFixture(t, d)
	release := make(chan struct{})
	f.panel.dnsOverviewSeams.DKIM = func(context.Context, string) (string, error) {
		<-release
		panic("the key read panicked")
	}
	first := make(chan any, 1)
	go func() {
		defer func() { first <- recover() }()
		_, _ = f.panel.dnsOverview(context.Background(), time.Now(), false)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !f.panel.dnsOverviewInFlight() {
		if time.Now().After(deadline) {
			t.Fatal("the check never started")
		}
		time.Sleep(time.Millisecond)
	}
	second := make(chan dnsOverview, 1)
	go func() {
		value, _ := f.panel.dnsOverview(context.Background(), time.Now(), false)
		second <- value
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)
	select {
	case value := <-second:
		if value.Sentence.Code != "dns_overview_check_running" || value.Sentence.State != "unknown" {
			t.Fatalf("waiting reader: %+v", value.Sentence)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a reader hung after the check panicked")
	}
	if recovered := <-first; recovered == nil {
		t.Fatal("the panic was swallowed")
	}
	if f.panel.dnsOverviewInFlight() {
		t.Fatal("the in-flight entry stayed")
	}
}

func TestDNSOverviewSetupNotFinishedAndNotRunningAsksNothing(t *testing.T) {
	f, world := overviewDraftFixture(t, joinFirstDraft())
	if _, err := f.database.GetDB().Exec(`UPDATE server_setup_state SET status='draft', completed_at='' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	world.set(func(w *overviewWorld) { w.servers = map[string]string{} })
	value := readDNSOverview(t, f.panel)
	if value.Sentence.State != "prerequisite" || value.Sentence.Code != "dns_overview_setup_not_done" || value.SetupReady {
		t.Fatalf("sentence: %+v", value.Sentence)
	}
	if world.asked.Load() != 0 || len(value.Names) != 0 || len(value.Mail) != 0 {
		t.Fatalf("nothing is asked of the internet: asked=%d names=%+v", world.asked.Load(), value.Names)
	}
	for _, server := range value.Servers {
		if server.Answers == "not_serving" || server.Answers == "unknown" {
			t.Fatalf("an unfinished server is reported as failing: %+v", server)
		}
	}
}
