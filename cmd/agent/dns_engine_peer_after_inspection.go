package main

import (
	"context"
	"fmt"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// nativePeerAfterInspectionProbes are the DNS observations the native peer
// proof repeats after the authenticated inspection returned.
type nativePeerAfterInspectionProbes struct {
	soa          dnsZoneSOAProbe
	localCatalog dnsCatalogAXFRProbe
	peerCatalog  dnsBoundCatalogAXFRProbe
	peerZone     dnsBoundZoneAXFRProbe
}

// nativePeerAfterInspectionRetries bounds how long the post-inspection pair
// check waits for the secondary to transfer a catalog the daemon re-stamped
// after the challenge was minted (measured: about 0.1 s after the re-stamp).
const nativePeerAfterInspectionRetries = 20

const nativePeerAfterInspectionRetryDelay = 250 * time.Millisecond

func (state dnsZoneAXFRState) String() string {
	switch state {
	case dnsZoneAXFRPresent:
		return "transfer_present"
	case dnsZoneAXFRNoTransfer:
		return "no_transfer"
	default:
		return "indeterminate"
	}
}

func (state dnsDeletedZoneSOAState) String() string {
	switch state {
	case dnsDeletedZoneParentNegative:
		return "parent_negative"
	case dnsDeletedZoneEmptyRefused:
		return "empty_refused"
	default:
		return "unknown"
	}
}

// verifyNativePeerAfterInspectionAt is the post-inspection re-verification
// both native verifiers share (BIND and PowerDNS secondaries). The source-bound
// catalog pair, the peer's refused zone transfer and its empty REFUSED
// answer must still hold after the network round trip; the catalog pair is
// judged against the attempt's recorded evidence (record), which may have
// followed an admitted PowerDNS daemon re-stamp since the challenge was
// minted at authority.catalogSerial. verifyCurrent is the verifier's full
// current-evidence recheck (it admits such a re-stamp). Every difference is
// dns_peer_owner_edit_unknown with the reviewed check that found it; a
// contradictory positive DNS answer stays fatal.
func verifyNativePeerAfterInspectionAt(
	ctx context.Context,
	record *dnsRecordedProducerCatalog,
	authority dnsPeerAXFRAuthority,
	domain string,
	probes nativePeerAfterInspectionProbes,
	verifyCurrent func() error,
) error {
	if record == nil || verifyCurrent == nil {
		return pendingDNSPeerProofInternal("post-inspection check for "+domain,
			fmt.Errorf("recorded catalog or current-evidence recheck is missing"))
	}
	var fresh dnsPeerAXFRAuthority
	for try := 0; ; try++ {
		evidence := record.Evidence()
		var err error
		fresh, err = verifyDNSPrimaryPairReadyAuthorityAt(ctx, evidence,
			probes.soa, probes.localCatalog, probes.peerCatalog)
		if err == nil {
			break
		}
		retry := try < nativePeerAfterInspectionRetries && ctx.Err() == nil
		if retry && !record.RestampedSince(authority.catalogSerial) {
			// Look for a re-stamp the daemon made since the last recheck;
			// a changed operation or owner edit ends the proof here. Once
			// the record moved, only the secondary's transfer is awaited.
			if currentErr := verifyCurrent(); currentErr != nil {
				return currentErr
			}
		}
		retry = retry && record.RestampedSince(authority.catalogSerial)
		if !retry && ctx.Err() != nil {
			// The step's time ran out: nothing was observed to differ. While
			// the record moved past the challenge, the check was waiting for
			// the secondary to transfer the re-stamped catalog.
			deadlineErr := fmt.Errorf("post-inspection catalog pair check (recorded serial %d): %w",
				evidence.Serial, err)
			if record.RestampedSince(authority.catalogSerial) {
				return dnsPeerProofDeadlineAt(transport.DNSPeerProofTimeoutPeerExchange, deadlineErr)
			}
			return dnsPeerProofDeadline(deadlineErr)
		}
		if !retry && isDNSPeerBehind(err) {
			// This server's catalog matched its record; only the backup did
			// not serve it (it stopped answering after the exchange, is
			// restarting, or is behind). That is the backup catching up, the
			// same reading as the wave's for the same observation, and is
			// re-checked by itself; it is not an edit of this server's DNS
			// (alpha.133 round 5 re-review: a backup reboot right after it
			// answered was told "finish your change to this server's DNS").
			return pendingDNSPeerCause(transport.DNSPeerPendingCatchingUp,
				fmt.Errorf("post-inspection catalog pair check (recorded serial %d): %w", evidence.Serial, err))
		}
		if !retry {
			// This server's own catalog or member answers differ from the
			// record: a local edit.
			return pendingDNSPeerOwnerEdit(dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckCatalogProbe,
				"after the inspection the catalog pair did not match the recorded evidence (recorded catalog %s serial %d, %d member(s); challenge serial %d; %s): %v",
				evidence.Domain, evidence.Serial, len(evidence.Members), authority.catalogSerial,
				describeDNSCatalogProbeObservation(ctx, evidence, probes), err))
		}
		select {
		case <-ctx.Done():
		case <-time.After(nativePeerAfterInspectionRetryDelay):
		}
	}
	sameIdentity := fresh.sourceIP == authority.sourceIP && fresh.peerIP == authority.peerIP &&
		fresh.catalog == authority.catalog
	sameSerial := fresh.catalogSerial == authority.catalogSerial
	if !sameIdentity || (!sameSerial && !record.RestampedSince(authority.catalogSerial)) ||
		fresh.catalogSerial != record.Evidence().Serial {
		return pendingDNSPeerOwnerEdit(dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckAuthority,
			"recorded pair source=%s peer=%s catalog=%s serial=%d; observed source=%s peer=%s catalog=%s serial=%d",
			authority.sourceIP, authority.peerIP, authority.catalog, authority.catalogSerial,
			fresh.sourceIP, fresh.peerIP, fresh.catalog, fresh.catalogSerial))
	}
	state, err := observePeerZoneTransferAt(ctx, fresh, domain, probes.peerZone)
	if err != nil && ctx.Err() != nil {
		// A transfer request to the secondary that did not answer in time.
		return dnsPeerProofDeadlineAt(transport.DNSPeerProofTimeoutPeerExchange,
			fmt.Errorf("post-inspection no-transfer probe of %s: %w", domain, err))
	}
	if err != nil || state != dnsZoneAXFRNoTransfer {
		return pendingDNSPeerOwnerEdit(dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckTransferObserved,
			"expected no transfer of %s from peer %s; observed %s (%v)",
			domain, fresh.peerIP, state, errorTextOrNone(err)))
	}
	observation, err := observeDeletedDNSZoneAt(ctx, fresh.sourceIP, fresh.peerIP, domain, probes.soa)
	if err != nil && ctx.Err() != nil {
		return dnsPeerProofDeadline(fmt.Errorf("post-inspection SOA probe of %s: %w", domain, err))
	}
	if err != nil || observation != dnsDeletedZoneEmptyRefused {
		return pendingDNSPeerOwnerEdit(dnsPeerOwnerEditf(transport.DNSPeerOwnerEditCheckZoneAnswered,
			"expected the empty REFUSED answer for %s from peer %s over UDP and TCP seen before the inspection; observed %s (%v)",
			domain, fresh.peerIP, observation, errorTextOrNone(err)))
	}
	return nil
}

func errorTextOrNone(err error) string {
	if err == nil {
		return "no error"
	}
	return err.Error()
}

// describeDNSCatalogProbeObservation reads the local and peer catalogs once
// more, only for the Agent log of a failed post-inspection pair check.
func describeDNSCatalogProbeObservation(
	ctx context.Context,
	evidence dnsPrimaryCatalogEvidence,
	probes nativePeerAfterInspectionProbes,
) string {
	local, peer := "unavailable", "unavailable"
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dnsProbeTimeout)
	defer cancel()
	if probes.localCatalog != nil {
		if live, err := probes.localCatalog(probeCtx, evidence.LocalIP, evidence.Domain); err == nil {
			local = fmt.Sprintf("serial %d, %d member(s)", live.Serial, len(live.Members))
		}
	}
	if probes.peerCatalog != nil {
		if live, err := probes.peerCatalog(probeCtx, evidence.LocalIP, evidence.PeerIP, evidence.Domain); err == nil {
			peer = fmt.Sprintf("serial %d, %d member(s)", live.Serial, len(live.Members))
		}
	}
	return "observed local catalog " + local + ", peer catalog " + peer
}
