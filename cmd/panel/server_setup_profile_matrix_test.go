package main

import (
	"context"
	"slices"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestServerSetupProfileDNSMatrixPreservesIdentityAndServiceScope(t *testing.T) {
	for _, purpose := range []string{"web", "web_mail", "dns"} {
		for _, mode := range []string{"local", "external"} {
			for _, engine := range []string{"bind", "pdns"} {
				for _, role := range []string{"primary", "secondary"} {
					// The paired PowerDNS primary cells run with the product
					// gate closed (until 2026-10-01) and open (D-028);
					// every other cell runs with it closed and is unaffected.
					gates := []bool{false}
					if engine == "pdns" && role == "primary" {
						gates = []bool{false, true}
					}
					for _, gateOpen := range gates {
						name := purpose + "/" + mode + "/" + engine + "/" + role
						if len(gates) > 1 {
							name += map[bool]string{false: "/gate-closed", true: "/gate-open"}[gateOpen]
						}
						t.Run(name, func(t *testing.T) {
							setPDNSPairedPrimaryGateForTest(t, gateOpen)
							f, state := setupOperationFixture(t)
							setServerSetupTestHost(f.panel, "debian", "13")
							caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
							f.agent.versionCapabilities = &caps
							state.Draft.Purpose, state.Draft.DNSMode = purpose, mode
							state.Draft.DNSEngine, state.Draft.DNSRole = engine, role
							state.Draft.NS1, state.Draft.NS2 = "ns1.example.test", "ns2.example.test"
							state.Draft.LocalIP, state.Draft.PeerIP = "72.62.38.15", "2.25.80.4"
							state.Draft.PeerNS = state.Draft.NS2
							if role == "secondary" {
								state.Draft.PeerNS = state.Draft.NS1
								state.Draft.LocalIP, state.Draft.PeerIP = state.Draft.PeerIP, state.Draft.LocalIP
							}
							// A stale hidden mail field must not add mail to web or DNS profiles.
							state.Draft.MailHostname = "mail.frankfurt.celikhost.com"
							plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
							if err != nil {
								t.Fatal(err)
							}
							// The paired PowerDNS primary is refused at review while
							// its product gate is closed (pdnsPairedPrimaryBlocker).
							pdnsPrimaryPaused := mode == "local" && engine == "pdns" && role == "primary" && !pdnsPairedPrimaryGateOpen
							wantBlocked := (purpose == "dns" && mode != "local") || (purpose != "dns" && mode == "local" && role == "secondary") || pdnsPrimaryPaused
							if plan.CanStart == wantBlocked {
								t.Fatalf("unexpected plan decision: %v", plan.Blockers)
							}
							if slices.Contains(plan.Blockers, "pdns_primary_switch_paused") != pdnsPrimaryPaused {
								t.Fatalf("paired PowerDNS primary review blocker=%v", plan.Blockers)
							}
							if plan.HostnameChange != "" {
								t.Fatal("setup planned an OS rename")
							}
							mail := false
							for _, step := range plan.Steps {
								mail = mail || step.Kind == "mail_profile" || step.Kind == "mail_certificate"
								if purpose == "dns" && step.Kind == "service" && !slices.Contains([]string{"nftables", "certbot"}, step.Target) {
									t.Fatalf("DNS-only plan installs unrelated service: %+v", step)
								}
							}
							if mail != (purpose == "web_mail") {
								t.Fatal("mail scope does not follow purpose")
							}
							for _, port := range []int{25, 110, 143, 465, 587, 993, 995} {
								if purpose != "web_mail" && slices.Contains(plan.TCPPorts, port) {
									t.Fatalf("unrelated mail port %d", port)
								}
							}
							if (slices.Contains(plan.TCPPorts, 53) && slices.Contains(plan.UDPPorts, 53)) != (mode == "local") {
								t.Fatal("DNS ports do not follow ownership mode")
							}
							if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
								t.Fatal("review mutated host")
							}
						})
					}
				}
			}
		}
	}
}

// setServerSetupTestHost publishes a verified host identity for a test panel.
func setServerSetupTestHost(p *Panel, distro, version string) {
	identity := debianPolicyTestIdentity()
	identity.DistroID, identity.VersionID = distro, version
	p.pkgFamilyMu.Lock()
	p.pkgFamilyVal = "apt"
	p.hostPlatformVal = identity
	p.hostPlatformKnown = true
	p.pkgFamilyMu.Unlock()
}

// PowerDNS as the main DNS server is refused at review on a system where it
// was not measured, with the same reason the wizard shows (alpha.111 review).
func TestServerSetupBuildRefusesPowerDNSMainOnUntestedSystem(t *testing.T) {
	setPDNSPairedPrimaryGateForTest(t, true)
	f, state := setupOperationFixture(t)
	setServerSetupTestHost(f.panel, "ubuntu", "24.04")
	caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
	f.agent.versionCapabilities = &caps
	state.Draft.Purpose, state.Draft.DNSMode, state.Draft.DNSEngine, state.Draft.DNSRole = "web", "local", "pdns", "primary"
	state.Draft.NS1, state.Draft.NS2, state.Draft.PeerNS = "ns1.example.test", "ns2.example.test", "ns2.example.test"
	state.Draft.LocalIP, state.Draft.PeerIP = "72.62.38.15", "2.25.80.4"
	plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanStart || !slices.Contains(plan.Blockers, "server_setup_pdns_main_untested_system") {
		t.Fatalf("PowerDNS main on Ubuntu was not refused at review: %v", plan.Blockers)
	}
}

// PowerDNS as the main DNS server is offered and accepted by one rule: a
// release of the tested series (Beta-1 VM cell b: Debian 13 offered 4.9.18,
// the DNS step accepted only 4.9.17 and rolled back). A candidate outside the
// series is refused at review with the wizard's reason, never mid-run.
func TestServerSetupBuildPowerDNSMainFollowsTheTestedSeries(t *testing.T) {
	for _, test := range []struct {
		candidate string
		refused   bool
	}{{"4.9.18-0+deb13u1", false}, {"4.9.17-0+deb13u1", false}, {"5.0.1-1", true}, {"", false}} {
		t.Run(test.candidate, func(t *testing.T) {
			setPDNSPairedPrimaryGateForTest(t, true)
			f, state := setupOperationFixture(t)
			setServerSetupTestHost(f.panel, "debian", "13")
			caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
			f.agent.versionCapabilities = &caps
			if test.candidate != "" {
				f.agent.candidateVersions = map[string]string{"pdns": test.candidate}
			}
			state.Draft.Purpose, state.Draft.DNSMode, state.Draft.DNSEngine, state.Draft.DNSRole = "web", "local", "pdns", "primary"
			state.Draft.NS1, state.Draft.NS2, state.Draft.PeerNS = "ns1.example.test", "ns2.example.test", "ns2.example.test"
			state.Draft.LocalIP, state.Draft.PeerIP = "72.62.38.15", "2.25.80.4"
			plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(plan.Blockers, "server_setup_pdns_main_untested_version"); got != test.refused {
				t.Fatalf("candidate %q refused=%v: %v", test.candidate, got, plan.Blockers)
			}
		})
	}
	debian := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
	for candidate, offered := range map[string]bool{"4.9.18": true, "4.9.17": true, "5.0.1": false, "4.10.0": false, "": true} {
		catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true, DNSState: &dnsEngineDBState{}, CandidateVersions: map[string]string{"pdns": candidate}})
		for _, choice := range catalog.DNSEngines {
			if choice.ID != "pdns" {
				continue
			}
			want := offered || choice.Role == "backup"
			if choice.Offered != want || (!want && choice.Reason != "server_setup_pdns_main_untested_version") {
				t.Fatalf("candidate %q %s: %+v", candidate, choice.Role, choice)
			}
		}
	}
}
