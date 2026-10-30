package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Arch Linux PowerDNS (D-028 amendment, 2026-10-30): offered as the main and
// the backup DNS server on a fresh Arch host, from extra's 5.1 build only.
func TestServerSetupCatalogOffersArchPowerDNS(t *testing.T) {
	arch := core.ManagedServiceHostProfile{PackageFamily: "pacman", DistroFamily: "arch", DistroID: "arch", Architecture: "amd64"}
	engine := func(catalog serverSetupComponentCatalog, id, role string) serverSetupDNSEngineChoice {
		for _, choice := range catalog.DNSEngines {
			if choice.ID == id && choice.Role == role {
				return choice
			}
		}
		t.Fatalf("missing %s/%s", id, role)
		return serverSetupDNSEngineChoice{}
	}
	fresh := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: arch, InventoryReady: true, DNSState: &dnsEngineDBState{},
		PDNSCandidate: &transport.PDNSFreshPrimaryCandidateResponse{State: "known", Version: "5.1.5-1", Official: true}})
	for _, role := range []string{"main", "backup"} {
		if choice := engine(fresh, "pdns", role); !choice.Offered {
			t.Fatalf("PowerDNS %s refused on a fresh Arch host: %+v", role, choice)
		}
	}
	moved := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: arch, InventoryReady: true, DNSState: &dnsEngineDBState{},
		PDNSCandidate: &transport.PDNSFreshPrimaryCandidateResponse{State: "known", Version: "5.2.0-1", Official: true}})
	if choice := engine(moved, "pdns", "main"); choice.Offered || choice.Reason != "server_setup_pdns_main_untested_version" {
		t.Fatalf("Arch moved to 5.2: the main role must name the untested version: %+v", choice)
	}
	if !engine(moved, "pdns", "backup").Offered {
		t.Fatal("the backup role follows the Agent's own check, not the main-role candidate rule")
	}
	foreign := buildServerSetupComponentCatalogFrom(serverSetupCatalogInputs{Host: arch, InventoryReady: true, DNSState: &dnsEngineDBState{},
		PDNSCandidate: &transport.PDNSFreshPrimaryCandidateResponse{State: "known", Version: "5.1.5-1", Official: false}})
	if choice := engine(foreign, "pdns", "main"); choice.Offered || choice.Reason != "server_setup_pdns_main_not_official_repository" {
		t.Fatalf("a third-party repository build: %+v", choice)
	}
	arm := arch
	arm.Architecture = "arm64"
	if serverSetupPDNSMainSystemTested(arm) {
		t.Fatal("Arch on arm64 is not measured")
	}
	if code := serverSetupPDNSMainCandidateBlockerFor("apt", &transport.PDNSFreshPrimaryCandidateResponse{State: "known", Version: "5.1.5-1", Official: true}); code != "server_setup_pdns_main_untested_version" {
		t.Fatalf("the Arch series leaked into Debian's rule: %q", code)
	}
}

func TestPDNSArchSetupOnlyBlocker(t *testing.T) {
	arch := core.ManagedServiceHostProfile{PackageFamily: "pacman", DistroID: "arch", Architecture: "amd64"}
	pdns := transport.DNSEnginePowerDNS
	freshPrimary := dnsEngineSnapshot{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRolePrimary}
	freshSecondary := dnsEngineSnapshot{Topology: transport.DNSTopologyPaired, PairRole: transport.DNSPairRoleSecondary}
	for name, snapshot := range map[string]dnsEngineSnapshot{"primary": freshPrimary, "secondary": freshSecondary} {
		if code := pdnsArchSetupOnlyBlocker(arch, snapshot, pdns); code != "" {
			t.Fatalf("fresh %s refused: %s", name, code)
		}
	}
	standalone := dnsEngineSnapshot{Topology: transport.DNSTopologyStandalone}
	if code := pdnsArchSetupOnlyBlocker(arch, standalone, pdns); code != "" {
		t.Fatalf("a first standalone PowerDNS on Arch: %q", code)
	}
	active := pdns
	reinstall := freshSecondary
	reinstall.ActiveEngine, reinstall.EngineEpoch = &active, 2
	if code := pdnsArchSetupOnlyBlocker(arch, reinstall, pdns); code != "pdns_arch_setup_only" {
		t.Fatalf("PowerDNS reinstall on Arch: %q", code)
	}
	if code := pdnsArchSetupOnlyBlocker(arch, standalone, transport.DNSEngineBIND); code != "" {
		t.Fatalf("BIND on Arch blocked: %q", code)
	}
	debian := core.ManagedServiceHostProfile{PackageFamily: "apt"}
	if code := pdnsArchSetupOnlyBlocker(debian, standalone, pdns); code != "" {
		t.Fatalf("Debian blocked: %q", code)
	}
}

func TestDNSEngineSeriesUntestedDetail(t *testing.T) {
	state := dnsEngineDBState{ActiveEngine: transport.DNSEnginePowerDNS, EngineEpoch: 1}
	runtimes := map[transport.DNSEngine]transport.DNSBackendRuntimeState{
		transport.DNSEngineBIND:     {Engine: transport.DNSEngineBIND},
		transport.DNSEnginePowerDNS: {Engine: transport.DNSEnginePowerDNS, Installed: true, Running: true, Managed: true, UntestedVersion: "5.2.0-1"},
	}
	status, entries := deriveDNSEnginePresentation(state, runtimes, nil, "")
	found := false
	for _, entry := range entries {
		if entry.ID == transport.DNSEnginePowerDNS {
			found = entry.Status == "active" && entry.DetailCode == dnsEngineSeriesUntestedCode
		}
	}
	if !found {
		t.Fatalf("untested series not shown on the active engine (%s): %+v", status, entries)
	}
	tested := runtimes[transport.DNSEnginePowerDNS]
	tested.UntestedVersion = ""
	runtimes[transport.DNSEnginePowerDNS] = tested
	_, entries = deriveDNSEnginePresentation(state, runtimes, nil, "")
	for _, entry := range entries {
		if entry.DetailCode == dnsEngineSeriesUntestedCode {
			t.Fatal("a tested release shows the notice")
		}
	}
	sentence := dnsOverviewSentenceFor(dnsOverviewInput{Ready: true, EngineSeriesUntested: true,
		Draft: serverSetupDraft{MainDomain: "example.test", DNSMode: "local"}}, dnsOverview{})
	if sentence.Code != "dns_overview_engine_series_untested" || sentence.State != "prerequisite" ||
		sentence.Who == "" || sentence.Next == "" || sentence.Then == "" {
		t.Fatalf("overview guidance: %+v", sentence)
	}
}
