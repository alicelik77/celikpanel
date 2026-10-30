package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestFreshPDNSV3LocalDeletionRequiresBoundNegativeOnBothTransports(t *testing.T) {
	const address = "192.0.2.10"
	const domain = "gone.example.test"
	negative := dnsSOAProbeResult{
		LocalIP: address, ExactDeletedZone: true, Authoritative: true,
		RCode: dnsRCodeNameError, AuthoritySOAOwners: []string{"example.test"},
	}
	nodata := dnsSOAProbeResult{
		LocalIP: address, ExactDeletedZone: true, Authoritative: true,
		RCode: dnsRCodeNoError, AuthoritySOAOwners: []string{"example.test"},
	}
	refused := dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeRefused}
	positive := dnsSOAProbeResult{
		LocalIP: address, Authoritative: true, RCode: dnsRCodeNoError,
		AnswerCount: 1, SOASerials: []uint32{42},
		AnswerSOAOwners: []string{domain},
	}
	const dnsRCodeServFail = 2
	for _, tc := range []struct {
		name     string
		udp, tcp dnsSOAProbeResult
		probeErr error
		wantErr  bool
	}{
		{"parent-negative", negative, negative, nil, false},
		{"parent-nodata", nodata, nodata, nil, false},
		{"parentless-refused", refused, refused, nil, false},
		{"mixed-negative-refused", negative, refused, nil, false},
		{"positive-udp", positive, refused, nil, true},
		{"positive-tcp", refused, positive, nil, true},
		{"refused-with-answer", dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeRefused, AnswerCount: 1}, refused, nil, true},
		{"refused-authoritative", dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeRefused, Authoritative: true}, refused, nil, true},
		{"refused-with-authority-soa", dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeRefused, AuthoritySOAOwners: []string{"example.test"}}, refused, nil, true},
		{"refused-with-zone-soa-answer", refused, dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeRefused, SOASerials: []uint32{7}, AnswerSOAOwners: []string{domain}}, nil, true},
		{"servfail", dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeServFail}, refused, nil, true},
		{"servfail-tcp", refused, dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeServFail}, nil, true},
		{"nxdomain-with-own-zone-soa", dnsSOAProbeResult{LocalIP: address, Authoritative: true, RCode: dnsRCodeNameError, AuthoritySOAOwners: []string{domain}}, refused, nil, true},
		{"nxdomain-without-authority", dnsSOAProbeResult{LocalIP: address, Authoritative: true, RCode: dnsRCodeNameError}, refused, nil, true},
		{"notauth", dnsSOAProbeResult{LocalIP: address, RCode: dnsRCodeNotAuth}, refused, nil, true},
		{"wrong-source", dnsSOAProbeResult{LocalIP: "192.0.2.11", RCode: dnsRCodeRefused}, refused, nil, true},
		{"unbound-source", dnsSOAProbeResult{RCode: dnsRCodeRefused}, refused, nil, true},
		{"probe-error", refused, refused, errors.New("private DNS detail"), true},
		{"timeout", refused, refused, context.DeadlineExceeded, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			probe := func(_ context.Context, network, gotAddress, gotDomain string) (dnsSOAProbeResult, error) {
				if gotAddress != address || gotDomain != domain {
					t.Fatalf("unexpected proof identity %s %s", gotAddress, gotDomain)
				}
				calls++
				if network == "udp" {
					if tc.probeErr != nil {
						return dnsSOAProbeResult{}, tc.probeErr
					}
					return tc.udp, nil
				}
				if network != "tcp" {
					t.Fatalf("unexpected network %s", network)
				}
				return tc.tcp, nil
			}
			err := verifyFreshPDNSV3DeletedZoneAt(context.Background(), address, domain, probe)
			if (err != nil) != tc.wantErr {
				t.Fatalf("proof err=%v wantErr=%v", err, tc.wantErr)
			}
			if calls == 0 {
				t.Fatal("DNS proof was skipped")
			}
			if !tc.wantErr && calls != 2 {
				t.Fatalf("proof used %d transports, want UDP and TCP", calls)
			}
			if tc.probeErr != nil && err != nil && strings.Contains(err.Error(), tc.probeErr.Error()) {
				t.Fatal("private probe detail leaked")
			}
		})
	}
}

func TestFreshPDNSV3LocalDeletionAcceptsCanonicalIPv6Binding(t *testing.T) {
	const address = "2001:db8::10"
	const domain = "gone.test"
	probe := func(_ context.Context, _ string, gotAddress, _ string) (dnsSOAProbeResult, error) {
		return dnsSOAProbeResult{LocalIP: gotAddress, RCode: dnsRCodeRefused}, nil
	}
	if err := verifyFreshPDNSV3DeletedZoneAt(context.Background(), address, domain, probe); err != nil {
		t.Fatalf("IPv6-bound empty REFUSED rejected: %v", err)
	}
	for _, bad := range []string{"2001:DB8::10", "2001:db8:0::10", "::1", "fe80::1", "127.0.0.1", "0.0.0.0", ""} {
		if err := verifyFreshPDNSV3DeletedZoneAt(context.Background(), bad, domain, probe); err == nil {
			t.Fatalf("unsafe or noncanonical proof address %q accepted", bad)
		}
	}
}

// The strict DNS-only rule (used by every non-tombstone path and by the pair
// proof's parent-negative observation) still refuses an empty REFUSED.
func TestStrictDeletedZoneRuleStillRefusesEmptyRefused(t *testing.T) {
	const domain = "gone.test"
	err := verifyDNSZoneAuthoritiesAt(context.Background(), "192.0.2.10",
		[]expectedDNSZoneAuthority{{Domain: domain, Delete: true}},
		func(context.Context, string, string, string) (dnsSOAProbeResult, error) {
			return dnsSOAProbeResult{LocalIP: "192.0.2.10", RCode: dnsRCodeRefused}, nil
		})
	if err == nil {
		t.Fatal("strict DNS-only deletion rule accepted an empty REFUSED")
	}
}

func TestPDNSV3DeletionProofSelectsTombstonePathOnlyForStandaloneAndNativePrimary(t *testing.T) {
	previousAddress := standalonePDNSDeletionProofAddress
	previousPairing := pdnsLegacyClusterPairingConfigured
	previousOwned := dnsPairHostAddressOwned
	t.Cleanup(func() {
		standalonePDNSDeletionProofAddress = previousAddress
		pdnsLegacyClusterPairingConfigured = previousPairing
		dnsPairHostAddressOwned = previousOwned
	})
	standalonePDNSDeletionProofAddress = func(context.Context) (string, error) {
		return "198.51.100.7", nil
	}
	legacyPaired := false
	pdnsLegacyClusterPairingConfigured = func(context.Context) (bool, error) {
		return legacyPaired, nil
	}
	dnsPairHostAddressOwned = func(address string) (bool, error) {
		return address == "192.0.2.10", nil
	}
	standalone := dnsEngineStateReceipt{
		Mode: transport.DNSEngineSwitchModeSwitch, Engine: transport.DNSEnginePowerDNS,
		EngineEpoch: 1,
	}
	adopted := standalone
	adopted.Mode = transport.DNSEngineSwitchModeAdopt
	nativePrimary := standalone
	nativePrimary.PairRole = transport.DNSPairRolePrimary
	nativePrimary.PairLocalIP, nativePrimary.PairPeerIP = "192.0.2.10", "192.0.2.11"
	nativePrimary.NativeCatalogV3 = dnsengineartifact.NativeCatalogDebian49V3
	legacyPrimary := nativePrimary
	legacyPrimary.NativeCatalogV3 = ""
	secondary := nativePrimary
	secondary.PairRole = transport.DNSPairRoleSecondary
	bind := standalone
	bind.Engine = transport.DNSEngineBIND
	deleted := transport.DNSEngineSwitchZoneSnapshot{Domain: "gone.test", Delete: true}
	kept := transport.DNSEngineSwitchZoneSnapshot{Domain: "kept.test"}

	for _, tc := range []struct {
		name         string
		state        dnsEngineStateReceipt
		zone         transport.DNSEngineSwitchZoneSnapshot
		legacyPaired bool
		wantAddress  string
		wantNative   bool
	}{
		{"standalone fresh switch", standalone, deleted, false, "198.51.100.7", true},
		{"standalone adopted", adopted, deleted, false, "198.51.100.7", true},
		{"standalone publication is not a deletion", standalone, kept, false, "", false},
		{"legacy cluster pairing keeps strict rule", standalone, deleted, true, "", false},
		{"native V3 primary", nativePrimary, deleted, false, "192.0.2.10", true},
		{"legacy directional primary keeps strict rule", legacyPrimary, deleted, false, "", false},
		{"secondary keeps strict rule", secondary, deleted, false, "", false},
		{"BIND uses its own control-channel proof", bind, deleted, false, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacyPaired = tc.legacyPaired
			address, native, err := pdnsV3DeletionProofAddress(context.Background(), tc.state, tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			if address != tc.wantAddress || native != tc.wantNative {
				t.Fatalf("address=%q native=%v want %q %v", address, native, tc.wantAddress, tc.wantNative)
			}
		})
	}

	pdnsLegacyClusterPairingConfigured = func(context.Context) (bool, error) {
		return false, errors.New("cluster configuration unreadable")
	}
	if _, _, err := pdnsV3DeletionProofAddress(context.Background(), standalone, deleted); err == nil {
		t.Fatal("unreadable pairing configuration did not fail closed")
	}
}
