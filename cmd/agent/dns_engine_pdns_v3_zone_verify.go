package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A deleted zone whose parent this server does not serve normally returns an
// empty REFUSED from PowerDNS: a standalone server for example.com does not
// serve com., so no authoritative parent-negative answer can ever exist. An
// empty REFUSED alone never proves removal; it is accepted only together with
// the exact durable PowerDNS tombstone (the delete receipt and no domain row in
// the PowerDNS database), read before and after the DNS observation, on the
// fresh native V3 primary and on a standalone server. Every other engine and
// topology path retains the strict authoritative parent-negative rule.
//
// Ust bolgesini bu sunucu sunmuyorsa silinmis bolge PowerDNS'ten bos REFUSED
// alir; tek basina yeterli degildir, yalniz DNS gozleminden once ve sonra
// okunan kesin PowerDNS silme kaydi (silme makbuzu ve satirsiz alan) ile
// birlikte kabul edilir.
func verifyPDNSV3PublishedZoneAuthority(
	ctx context.Context,
	state dnsEngineStateReceipt,
	zone transport.DNSEngineSwitchZoneSnapshot,
	binding transport.ServiceMutationBinding,
) error {
	address, native, err := pdnsV3DeletionProofAddress(ctx, state, zone)
	if err != nil {
		return err
	}
	if !native {
		return verifyDNSZoneManifestAuthority(ctx, []transport.DNSEngineSwitchZoneSnapshot{zone})
	}
	verifyTombstone := func() error {
		snapshot, exact, err := readPDNSV3ZoneSnapshot(
			ctx, pdnsDBPath(), state, zone.Domain, zone.ZoneQualifier, binding,
		)
		if err != nil {
			return fmt.Errorf("verify PowerDNS V3 deletion receipt: %w", err)
		}
		if !exact || !snapshot.Delete || snapshot.Domain != zone.Domain ||
			snapshot.DesiredGeneration != zone.DesiredGeneration ||
			snapshot.ZoneQualifier != zone.ZoneQualifier ||
			snapshot.ZoneType != zone.ZoneType ||
			!reflect.DeepEqual(snapshot.Records, zone.Records) {
			return errors.New("PowerDNS V3 deletion receipt or database tombstone is not exact")
		}
		return nil
	}
	if err := verifyTombstone(); err != nil {
		return err
	}
	if err := verifyFreshPDNSV3DeletedZoneAt(
		ctx, address, zone.Domain, probeDNSZoneSOA,
	); err != nil {
		return err
	}
	if err := verifyTombstone(); err != nil {
		return err
	}
	after, exists, err := readDNSEngineState()
	if err != nil || !exists || !reflect.DeepEqual(after, state) {
		if err != nil {
			return err
		}
		return errors.New("PowerDNS engine state changed during deletion proof")
	}
	return nil
}

// pdnsV3DeletionProofAddress selects the local address that a PowerDNS V3
// deletion is proved at. native=false keeps the strict DNS-only rule.
func pdnsV3DeletionProofAddress(
	ctx context.Context,
	state dnsEngineStateReceipt,
	zone transport.DNSEngineSwitchZoneSnapshot,
) (address string, native bool, err error) {
	if !zone.Delete || state.Engine != transport.DNSEnginePowerDNS {
		return "", false, nil
	}
	switch {
	case state.Mode == transport.DNSEngineSwitchModeSwitch &&
		state.PairRole == transport.DNSPairRolePrimary &&
		dnsengineartifact.IsNativeCatalogV3(state.NativeCatalogV3):
		if err := requireHostOwnedDNSPairAddress(state.PairLocalIP); err != nil {
			return "", false, err
		}
		return state.PairLocalIP, true, nil
	case state.PairRole == "" && state.PairLocalIP == "" && state.PairPeerIP == "" &&
		(state.Mode == transport.DNSEngineSwitchModeSwitch ||
			state.Mode == transport.DNSEngineSwitchModeAdopt):
		// A legacy pairing recorded only in the cluster configuration is a
		// paired primary, not a standalone server: it keeps the strict rule.
		paired, err := pdnsLegacyClusterPairingConfigured(ctx)
		if err != nil {
			return "", false, err
		}
		if paired {
			return "", false, nil
		}
		address, err := standalonePDNSDeletionProofAddress(ctx)
		if err != nil {
			return "", false, err
		}
		return address, true, nil
	default:
		return "", false, nil
	}
}

// pdnsLegacyClusterPairingConfigured reports a managed legacy PowerDNS pairing
// (cluster configuration present). Replaced only by tests.
var pdnsLegacyClusterPairingConfigured = func(ctx context.Context) (bool, error) {
	_, enabled, err := managedPDNSLegacyCatalogIdentity(ctx, pdnsDBPath())
	return enabled, err
}

// standalonePDNSDeletionProofAddress is the same host address a standalone
// zone's publication is verified at (verifyDNSZoneAuthorities), replaced only
// by tests.
var standalonePDNSDeletionProofAddress = func(ctx context.Context) (string, error) {
	addresses, err := publicListenAddresses(ctx)
	if err != nil {
		return "", fmt.Errorf("discover PowerDNS deletion proof address: %w", err)
	}
	if len(addresses) == 0 {
		return "", errors.New("no PowerDNS deletion proof address is available")
	}
	address := strings.TrimSpace(addresses[0])
	if !canonicalDNSDeletionProofIP(address) {
		return "", errors.New("PowerDNS deletion proof selected an unsafe address")
	}
	return address, nil
}

// canonicalDNSDeletionProofIP accepts one canonical global unicast IPv4 or
// IPv6 literal: the probe's own socket must report the same text.
func canonicalDNSDeletionProofIP(value string) bool {
	parsed := net.ParseIP(value)
	if parsed == nil || !parsed.IsGlobalUnicast() {
		return false
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String() == value
	}
	return parsed.String() == value
}

// verifyFreshPDNSV3DeletedZoneAt accepts, over both UDP and TCP and from the
// exact local address, only an authoritative parent-negative answer or an
// empty REFUSED (no answer, no SOA anywhere, not authoritative). A timeout,
// SERVFAIL, an answer still carrying the zone's SOA or any other shape is
// unverifiable and fails closed.
func verifyFreshPDNSV3DeletedZoneAt(
	ctx context.Context, address, domain string, probe dnsZoneSOAProbe,
) error {
	if probe == nil || !canonicalDNSDeletionProofIP(address) ||
		!serviceMutationCanonicalFQDN(domain) {
		return errors.New("PowerDNS V3 local deletion proof identity is invalid")
	}
	for _, network := range []string{"udp", "tcp"} {
		probeCtx, cancel := context.WithTimeout(ctx, dnsProbeTimeout)
		result, err := probe(probeCtx, network, address, domain)
		cancel()
		if err != nil || result.LocalIP != address {
			return fmt.Errorf("PowerDNS V3 local deletion response is unavailable or unbound over %s", network)
		}
		if validDeletedDNSZoneProof(domain, result) {
			continue
		}
		if emptyRefusedDNSZoneAnswer(result) {
			continue
		}
		return fmt.Errorf("PowerDNS V3 deleted zone returned a contradictory or unverifiable answer over %s", network)
	}
	return nil
}

// emptyRefusedDNSZoneAnswer is a server's "I do not serve this name" answer:
// REFUSED, not authoritative, and no answer or SOA in any section.
func emptyRefusedDNSZoneAnswer(result dnsSOAProbeResult) bool {
	return result.RCode == dnsRCodeRefused && !result.Authoritative &&
		result.AnswerCount == 0 && len(result.SOASerials) == 0 &&
		len(result.AnswerSOAOwners) == 0 && len(result.AuthoritySOAOwners) == 0
}
