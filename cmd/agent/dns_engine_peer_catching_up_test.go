package main

import (
	"context"
	"errors"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// 2026-10-30: a publication wave that ends because the backup has not served
// this server's current catalog carries the reviewed, transient
// dns_peer_catching_up reason (the Panel re-checks it by itself). A local
// mismatch, and the deletion-specific checks after the catalog, stay
// unclassified. Component test with fake probes; no native DNS.
func TestDNSV3PublicationWaveNamesBackupCatchingUp(t *testing.T) {
	const member = "a.example.test"
	evidence := testPDNSPrimaryPropagationEvidence(9, []string{member}, []uint32{2026103001})
	soa := func(_ context.Context, _, address, domain string) (dnsSOAProbeResult, error) {
		serial := evidence.Serial
		if domain == member {
			serial = evidence.MemberSerials[0]
		}
		return dnsSOAProbeResult{Authoritative: true, RCode: dnsRCodeNoError, SOASerials: []uint32{serial}}, nil
	}
	localExact := func(context.Context, string, string) (dnsCatalogAXFRResult, error) {
		return dnsCatalogAXFRResult{Serial: evidence.Serial, Members: []string{member}}, nil
	}
	localDiffers := func(context.Context, string, string) (dnsCatalogAXFRResult, error) {
		return dnsCatalogAXFRResult{Serial: evidence.Serial + 1, Members: []string{member}}, nil
	}
	peerDown := func(context.Context, string, string, string) (dnsCatalogAXFRResult, error) {
		return dnsCatalogAXFRResult{}, errors.New("connection timed out")
	}
	peerBehindCatalog := func(context.Context, string, string, string) (dnsCatalogAXFRResult, error) {
		return dnsCatalogAXFRResult{Serial: evidence.Serial - 1, Members: []string{member}}, nil
	}
	peerZone := absentTestPeerZoneAXFR(evidence)
	ended := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	change := dnsV3PrimaryPropagationPlan{
		Evidence: evidence,
		Changed:  expectedDNSZoneAuthority{Domain: member, Serial: evidence.MemberSerials[0]},
	}
	for name, peer := range map[string]dnsBoundCatalogAXFRProbe{"backup offline": peerDown, "backup behind": peerBehindCatalog} {
		err := completeDNSV3PrimaryPropagationAt(ended(), change, soa, localExact, peer, peerZone)
		if got := pendingDNSPeerCode(err); got != transport.DNSPeerPendingCatchingUp {
			t.Fatalf("%s: record change code=%q err=%v", name, got, err)
		}
		if pending := dnsZoneV3RecoveryPending(err); pendingDNSPeerCode(pending) != transport.DNSPeerPendingCatchingUp {
			t.Fatalf("%s: recovery pending lost the code: %v", name, pending)
		}
	}
	// This server's own catalog differs: not "backup catching up".
	if got := pendingDNSPeerCode(completeDNSV3PrimaryPropagationAt(ended(), change, soa, localDiffers, peerDown, peerZone)); got != "" {
		t.Fatalf("local mismatch was classified as %q", got)
	}
	// A converged pair publishes without any pending code.
	if err := completeDNSV3PrimaryPropagationAt(context.Background(), change, soa, localExact, exactTestPeerCatalogAXFR(evidence), peerZone); err != nil {
		t.Fatalf("converged pair: %v", err)
	}

	deletionEvidence := testPDNSPrimaryPropagationEvidence(8, nil, nil)
	deletion := dnsV3PrimaryPropagationPlan{
		Evidence: deletionEvidence,
		Changed:  expectedDNSZoneAuthority{Domain: "gone.example.test", Delete: true},
	}
	localEmpty := func(context.Context, string, string) (dnsCatalogAXFRResult, error) {
		return dnsCatalogAXFRResult{Serial: deletionEvidence.Serial}, nil
	}
	if got := pendingDNSPeerCode(completeDNSV3PrimaryPropagationAt(ended(), deletion, soa, localEmpty, peerDown, peerZone)); got != transport.DNSPeerPendingCatchingUp {
		t.Fatalf("deletion with backup offline code=%q", got)
	}
	stillServes := func(context.Context, string, string, string) (dnsZoneAXFRState, error) {
		return dnsZoneAXFRPresent, nil
	}
	deletionSOA := func(_ context.Context, _, _, domain string) (dnsSOAProbeResult, error) {
		return dnsSOAProbeResult{Authoritative: true, RCode: dnsRCodeNoError, SOASerials: []uint32{deletionEvidence.Serial}}, nil
	}
	if got := pendingDNSPeerCode(completeDNSV3PrimaryPropagationAt(ended(), deletion, deletionSOA, localEmpty,
		exactTestPeerCatalogAXFR(deletionEvidence), stillServes)); got != "" {
		t.Fatalf("deletion check after the catalog was classified as %q", got)
	}
	if !isDNSPeerBehind(dnsPeerBehind("x")) || isDNSPeerBehind(errors.New("x")) {
		t.Fatal("peer-behind marker")
	}
}
