package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Beta-1 VM cell a: after the owner's native rndc edit the DNS software card
// said "unmanaged DNS detected" (a foreign server) under a generic
// "degraded" while the overview said "Nothing to do". A detected owner change
// of the active engine is its own notice, with one code for both screens.
func TestDNSEngineOwnerChangeIsANoticeNotAForeignServer(t *testing.T) {
	state := dnsEngineDBState{ActiveEngine: transport.DNSEngineBIND, EngineEpoch: 1}
	runtimes := map[transport.DNSEngine]transport.DNSBackendRuntimeState{
		transport.DNSEngineBIND:     {Engine: transport.DNSEngineBIND, Installed: true, Running: true, OwnerChanged: true},
		transport.DNSEnginePowerDNS: {Engine: transport.DNSEnginePowerDNS},
	}
	_, entries := deriveDNSEnginePresentation(state, runtimes, nil, "")
	if code := detailCodeFor(t, entries, transport.DNSEngineBIND); code != dnsEngineOwnerChangedCode {
		t.Fatalf("owner change read as %q", code)
	}
	// A foreign engine (not the active one) stays foreign; a hold stays a hold.
	foreign := dnsEngineDBState{}
	if _, entries := deriveDNSEnginePresentation(foreign, runtimes, nil, ""); detailCodeFor(t, entries, transport.DNSEngineBIND) != "unmanaged_dns_detected" {
		t.Fatal("an engine the panel does not run cannot be the owner's change")
	}
	if _, entries := deriveDNSEnginePresentation(state, runtimes, nil, transport.MutationHoldLedgerAmbiguous); detailCodeFor(t, entries, transport.DNSEngineBIND) != "mutations_held" {
		t.Fatal("a held change system is said first")
	}
}

func TestDNSOverviewSaysTheEngineOwnerChangeNotNothingToDo(t *testing.T) {
	f, _ := overviewPairedFirst(t)
	activateDNSEngineForTest(t, f.panel, "bind")
	f.agent.mu.Lock()
	f.agent.bindOwnerChanged = true
	f.agent.mu.Unlock()
	value := readDNSOverview(t, f.panel)
	if value.Sentence.Code != "dns_overview_"+dnsEngineOwnerChangedCode || value.Sentence.State != "prerequisite" ||
		value.Sentence.Reason != dnsEngineOwnerChangedReason || value.Sentence.Next == "Nothing to do." {
		t.Fatalf("sentence: %+v", value.Sentence)
	}
	if f.agent.installCalls.Load() != 0 || len(f.agent.capturedMutationEvents()) != 0 {
		t.Fatal("the overview mutated the host")
	}
}
