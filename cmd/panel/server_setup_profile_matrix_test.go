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
	type candidate = transport.PDNSFreshPrimaryCandidateResponse
	for name, test := range map[string]struct {
		candidate *candidate
		code      string
	}{
		"4.9.18 from Debian":       {&candidate{State: "known", Version: "4.9.18-0+deb13u1", Official: true}, ""},
		"4.9.17 from Debian":       {&candidate{State: "known", Version: "4.9.17-0+deb13u1", Official: true}, ""},
		"other series":             {&candidate{State: "known", Version: "5.0.1-1", Official: true}, "server_setup_pdns_main_untested_version"},
		// Beta-1 re-run: the vendor build in the tested series was refused as
		// an untested version; the origin is checked first.
		"repo.powerdns.com build":  {&candidate{State: "known", Version: "4.9.4-1pdns.trixie"}, "server_setup_pdns_main_not_debian_archive"},
		"vendor 4.9.18 build":      {&candidate{State: "known", Version: "4.9.18-1pdns.trixie"}, "server_setup_pdns_main_not_debian_archive"},
		"backport":                 {&candidate{State: "known", Version: "4.9.4-1~bpo13+1"}, "server_setup_pdns_main_not_debian_archive"},
		"epoch":                    {&candidate{State: "known", Version: "1:4.9.18-0+deb13u1", Official: true}, "server_setup_pdns_main_untested_version"},
		"Debian version elsewhere": {&candidate{State: "known", Version: "4.9.18-0+deb13u1", Official: false}, "server_setup_pdns_main_not_debian_archive"},
		"unknown":                  {&candidate{State: "unknown"}, ""},
		"older agent":              {nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			setPDNSPairedPrimaryGateForTest(t, true)
			f, state := setupOperationFixture(t)
			setServerSetupTestHost(f.panel, "debian", "13")
			caps := []string{transport.AgentCapabilityMailHostCertificateV1, transport.AgentCapabilityMailTLSSyncV2, transport.AgentCapabilityFirewallApplyV2, transport.AgentCapabilityPanelCertificateIssueV2}
			f.agent.versionCapabilities = &caps
			f.agent.pdnsCandidate = test.candidate
			state.Draft.Purpose, state.Draft.DNSMode, state.Draft.DNSEngine, state.Draft.DNSRole = "web", "local", "pdns", "primary"
			state.Draft.NS1, state.Draft.NS2, state.Draft.PeerNS = "ns1.example.test", "ns2.example.test", "ns2.example.test"
			state.Draft.LocalIP, state.Draft.PeerIP = "72.62.38.15", "2.25.80.4"
			plan, err := f.panel.buildServerSetupPlan(context.Background(), state, serviceOperationActor{UserID: f.userID})
			if err != nil {
				t.Fatal(err)
			}
			for _, code := range []string{"server_setup_pdns_main_untested_version", "server_setup_pdns_main_not_debian_archive"} {
				if got := slices.Contains(plan.Blockers, code); got != (code == test.code) {
					t.Fatalf("%s refused=%v: %v", code, got, plan.Blockers)
				}
			}
			debian := core.ManagedServiceHostProfile{PackageFamily: "apt", DistroID: "debian", VersionID: "13", Architecture: "amd64"}
			catalog := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: debian, InventoryReady: true, DNSState: &dnsEngineDBState{}, PDNSCandidate: test.candidate})
			for _, choice := range catalog.DNSEngines {
				if choice.ID != "pdns" {
					continue
				}
				want := test.code == "" || choice.Role == "backup"
				if choice.Offered != want || (!want && choice.Reason != test.code) {
					t.Fatalf("%s: %+v", choice.Role, choice)
				}
			}
		})
	}
}
