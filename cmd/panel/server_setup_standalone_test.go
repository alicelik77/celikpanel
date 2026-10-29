package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// One server keeping its own DNS (alpha.114, D-021 Q7) is standalone: no
// peer, both nameserver names on this server.
func standaloneSetupDraftForTest() serverSetupDraft {
	d := setupDNSTestDraft()
	d.MainDomain, d.ServerCount, d.PeerIP, d.PeerNS = "example.test", 1, "", ""
	d.PanelDomain = "panel.example.test"
	return d
}

func TestServerSetupStandaloneDNSIdentityHasNoPeer(t *testing.T) {
	typed := standaloneSetupDraftForTest()
	typed.PeerIP, typed.PeerNS = "192.0.2.2", "ns2.example.test"
	d, err := canonicalServerSetupDraft(typed)
	if err != nil {
		t.Fatal(err)
	}
	if d.PeerIP != "" || d.PeerNS != "" || d.DNSRole != "primary" || d.MainDNSServer != "this" || d.InfrastructureDNS == nil || d.InfrastructureDNS.Zone != "example.test" {
		t.Fatalf("one server with its own DNS keeps no peer: %+v", d)
	}
	request, local, err := setupDNSIdentity(d)
	if err != nil || request.Role != transport.DNSTopologyStandalone || request.PeerIP != "" || local != "192.0.2.1" {
		t.Fatalf("standalone identity: %+v %q %v", request, local, err)
	}
	standalone := dnsEngineDBState{ActiveEngine: "bind", EngineEpoch: 1, Topology: transport.DNSTopologyStandalone}
	if !setupDNSDraftMatchesState(d, request, local, standalone) {
		t.Fatal("a standalone engine matches the standalone draft")
	}
	paired := standalone
	paired.Topology, paired.PairRole, paired.PeerIP = transport.DNSTopologyPaired, "primary", "192.0.2.2"
	if setupDNSDraftMatchesState(d, request, local, paired) {
		t.Fatal("a paired engine never matches a standalone draft")
	}
	withPeer := d
	withPeer.PeerIP = "192.0.2.2"
	if _, _, err := setupDNSIdentity(withPeer); err == nil {
		t.Fatal("a standalone identity with a peer address was accepted")
	}
	// Records: both nameservers inside the zone point to this server.
	records, err := serverSetupInfrastructureDNSDesired(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Type == "A" && strings.HasPrefix(record.Name, "ns") && record.Content != d.LocalIP {
			t.Fatalf("a standalone nameserver must point here: %+v", record)
		}
	}
}

func TestServerSetupStandaloneDNSInstallsAndIsReadyWithoutAPair(t *testing.T) {
	p := newDNSPanelForTest(t)
	seedSetupDNSOwner(t, p)
	agent := newDNSEngineTestAgent()
	attachDNSEngineTestAgent(t, p, agent)
	t.Setenv("CELIKPANEL_SERVER_IP", "192.0.2.1")
	d, err := canonicalServerSetupDraft(standaloneSetupDraftForTest())
	if err != nil {
		t.Fatal(err)
	}
	requestID := strings.Repeat("e", 32)
	if err := p.startServerSetupDNS(context.Background(), d, requestID, serviceOperationActor{UserID: 1}); err != nil {
		t.Fatal(err)
	}
	// A retry reconciles the same child; it never installs twice.
	if err := p.startServerSetupDNS(context.Background(), d, requestID, serviceOperationActor{UserID: 1}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	state, err := readDNSEngineDBState(context.Background(), p.db.GetDB())
	if err != nil || state.Topology != transport.DNSTopologyStandalone || state.ActiveEngine != "bind" || state.PeerIP != "" {
		t.Fatalf("state: %+v %v", state, err)
	}
	for _, test := range []struct {
		name     string
		ns1, ns2 []string
		ready    bool
	}{
		{"both here", []string{"192.0.2.1"}, []string{"192.0.2.1"}, true},
		{"ns2 elsewhere", []string{"192.0.2.1"}, []string{"192.0.2.9"}, false},
		{"unpublished", nil, nil, false},
	} {
		resolver := setupNameserverResolver{d.NS1: test.ns1, d.NS2: test.ns2}
		ready, err := p.setupDNSNameserverReadinessWithResolvers(context.Background(), []hostResolver{resolver})
		if ready != test.ready || (!test.ready && err == nil) {
			t.Fatalf("%s: ready=%v err=%v", test.name, ready, err)
		}
	}
}

func TestServerSetupStandaloneRegistrarCardAsksThisServerOnce(t *testing.T) {
	d, err := canonicalServerSetupDraft(standaloneSetupDraftForTest())
	if err != nil {
		t.Fatal(err)
	}
	expected := serverSetupDelegationExpected(d, "2001:db8::1")
	if expected[0].IPv4 != d.LocalIP || expected[1].IPv4 != d.LocalIP || expected[1].IPv6 != "2001:db8::1" || expected[1].Server != "main" {
		t.Fatalf("both nameservers are this server: %+v", expected)
	}
	options, _ := registrarFixture(t, true)
	result := (&Panel{}).checkServerSetupDelegation(context.Background(), d, "", options)
	count := 0
	for _, item := range result.Items {
		if strings.HasPrefix(item.ID, "answers_") {
			count++
			if item.ID != "answers_main" {
				t.Fatalf("no backup server to wait for: %+v", item)
			}
		}
	}
	if count != 1 || len(result.Servers) != 1 {
		t.Fatalf("this server is asked once: %+v", result.Servers)
	}
}

// One server keeping its own DNS publishes its infrastructure zone once and
// is done: the plan and the zone sync decide the zone kind with the same rule,
// so the reviewed digest is the one the sync leaves (Beta-1 VM cell d looped
// on infrastructure_dns_changed: plan MASTER, sync NATIVE). A paired primary
// keeps MASTER on both sides.
func TestServerSetupStandaloneInfrastructureDNSZoneKindMatchesSync(t *testing.T) {
	for _, test := range []struct {
		name       string
		standalone bool
		want       string
	}{{"standalone bind", true, "NATIVE"}, {"paired bind", false, "MASTER"}} {
		t.Run(test.name, func(t *testing.T) {
			p := newDNSPanelForTest(t)
			p.license = testPanelLicense(t, "active")
			seedSetupDNSOwner(t, p)
			d := infrastructureDNSTestDraft()
			if test.standalone {
				d = standaloneSetupDraftForTest()
				var err error
				if d, err = canonicalServerSetupDraft(d); err != nil {
					t.Fatal(err)
				}
			}
			attachDNSEngineTestAgent(t, p, newDNSEngineTestAgent())
			t.Setenv("CELIKPANEL_SERVER_IP", d.LocalIP)
			if err := p.startServerSetupDNS(context.Background(), d, strings.Repeat("a", 32), serviceOperationActor{UserID: 1}); err != nil {
				t.Fatal(err)
			}
			agent := newDNSZoneV3TestAgent()
			agent.pairReady = !test.standalone
			agent.publisher = transport.DNSEngineBIND
			attachInfrastructureDNSAgent(t, p, agent)
			infra, err := p.buildServerSetupInfrastructureDNSPlan(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			if infra.ZoneType != test.want || infra.ZoneType != p.dnsZoneType(context.Background()) {
				t.Fatalf("plan zone kind %q, sync rule %q, want %q", infra.ZoneType, p.dnsZoneType(context.Background()), test.want)
			}
			plan := serverSetupPlan{Version: serverSetupPlanVersion, BuildCommit: strings.TrimSpace(buildCommit), Draft: d, InfrastructureDNS: infra, Steps: []serverSetupPlanStep{{ID: "03-infrastructure_dns", Kind: "infrastructure_dns", Target: infra.Zone}}}
			plan.ID = serverSetupPlanIdentity(plan)
			step := serverSetupExecutionStep{serverSetupPlanStep: plan.Steps[0], RequestID: strings.Repeat("b", 32), OwnerID: strings.Repeat("c", 32)}
			for i := 0; i < 2; i++ {
				if done, err := p.runServerSetupInfrastructureDNS(context.Background(), plan, step); err != nil || !done {
					t.Fatalf("run %d done=%v err=%v", i, done, err)
				}
			}
			var kind string
			if err := p.db.GetDB().QueryRow(`SELECT type FROM pdns_domains WHERE name=?`, infra.Zone).Scan(&kind); err != nil || kind != test.want {
				t.Fatalf("stored zone kind %q %v, want %q", kind, err, test.want)
			}
			agent.mu.Lock()
			defer agent.mu.Unlock()
			if len(agent.requests) != 1 {
				t.Fatalf("published %d times", len(agent.requests))
			}
		})
	}
}
