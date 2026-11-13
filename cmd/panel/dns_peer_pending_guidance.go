package main

import (
	"errors"

	"github.com/alicelik/celikpanel/internal/transport"
)

// dnsPeerCatchingUpEnglish: the change is in effect here and only the backup
// has not confirmed it; nobody needs to act and the Panel completes the same
// publication by itself (dns_pending_publication.go).
const dnsPeerCatchingUpEnglish = "The DNS change is saved and in effect on this server, but the backup DNS server has not caught up with it yet: it did not confirm the change within its check time (it may be restarting, offline or briefly unreachable). Nobody needs to do anything: CelikPanel re-checks this same publication by itself, first after about 15 seconds and then less often (at most every 5 minutes), and completes it as soon as the backup has caught up; Server → DNS and peer servers shows the next check. Until then other DNS changes wait for it. If this lasts, the server administrator checks that the backup DNS server is running and reachable from this server on TCP port 53."

// dnsPeerJournalUnknownEnglish: the backup's confirmation of a deletion is not
// complete because this server could not complete its own check record this
// time. Since alpha.133 the Agent finishes an interrupted change of that
// record by itself in the next attempt and a state it cannot prove is
// dns_peer_journal_unresolved, so a re-check can finish this one and nobody
// needs to act. One sentence each: why, who, how it continues (D-024); the
// same words as the screen (web dnsPublication part, deletionGuide).
const dnsPeerJournalUnknownEnglish = "This domain is removed from DNS on this server; that the backup DNS server removed it too is not confirmed yet, because this server could not complete its last check. Nobody needs to do anything. CelikPanel re-checks this same publication by itself, at most every 5 minutes."

// dnsPeerJournalUnresolvedEnglish (alpha.133): the record this server keeps
// for that check is in a state the Agent cannot attribute to an interrupted
// change of its own; it keeps it and does not retry by itself. Plain words
// only: this text can reach any role through an API error, so the commands
// stay in the administrator's closed Details on Domains.
const dnsPeerJournalUnresolvedEnglish = "This domain is removed from DNS on this server, but the record this server keeps to confirm that the backup DNS server removed it too is in a state CelikPanel cannot sort out by itself. Who acts: this server's administrator; if that is not you, tell them. CelikPanel no longer checks this every few minutes; once the administrator has sorted it out, deleting the domain again continues this same publication from where it stopped."

// dnsPeerProofTimeoutEnglish: a step of the proof of the backup ran out of
// time. The zone is already removed on this server and the backup most
// likely followed (its zone list comes from this server); only the
// confirmation is open. Never "nothing was changed": the removal is done
// here. The Panel re-checks by itself (a press meanwhile starts nothing);
// after dnsPendingPublicationStuckAfter such checks the administrator gets a
// notification with the next step (dns_pending_publication.go).
const dnsPeerProofTimeoutEnglish = "The domain is removed from DNS on this server; that the backup DNS server removed it too is not confirmed yet, because the check did not finish in time. Nobody needs to do anything now: CelikPanel re-checks this same publication by itself (at most every 5 minutes) and then finishes the deletion by itself. If it stays like this for about a quarter of an hour, the server's administrator gets a notification saying what to do."

// dnsPeerProofTimeoutDetailEnglish names which side ran out of time.
func dnsPeerProofTimeoutDetailEnglish(detail string) string {
	switch detail {
	case transport.DNSPeerProofTimeoutLocalCheck:
		return "It was this server's own part of the check that ran out of time, not the backup DNS server."
	case transport.DNSPeerProofTimeoutPeerExchange:
		return "It was the backup DNS server's answer that did not come in time: it may be switched off, restarting or not reachable from this server."
	default:
		return ""
	}
}

// dnsPeerPendingRetriesAutomatically reports whether the Panel re-checks a
// pending publication with this code by itself.
//
// The rule (owner's lead, alpha.133 round 5): only a reason for which another
// check provably cannot change the result stops the automatic re-checks.
// Today that is dns_peer_journal_unresolved alone: the Agent keeps a check
// record it cannot prove and never changes it by itself, so a re-check finds
// the same files. Every other reason can clear by itself or because somebody
// repairs either server (the backup switched off or unreachable, refusing the
// check, an inspection access that was changed, a refused catalog transfer,
// local evidence that changed back, an Agent that was updated), so it is
// re-checked at the same slow cadence (15 s doubling to 5 min) and the
// deletion finishes by itself, with no press, once the cause is gone. Such a
// reason still becomes "stuck" after dnsPendingPublicationStuckAfter checks
// in a row and the administrators are notified; it never becomes a state in
// which nothing checks again. A re-check is the exact recovery of the same
// accepted operation, the one a press of More → Delete… runs, one at a time
// under the host mutation lock, and never a second deletion: making it
// automatic changes when it runs, not what runs.
//
// Until this round only the backup catching up, a proof step that ran out of
// time and dns_peer_journal_unknown were re-checked: a backup that did not
// answer for one attempt and came back as dns_peer_inspection_unknown (lab
// w9) left the deletion waiting until a person pressed Delete again.
func dnsPeerPendingRetriesAutomatically(code string) bool {
	if code == "" {
		return true
	}
	reason, _, ok := transport.SplitDNSPeerPendingCode(code)
	if !ok {
		return true
	}
	return reason != transport.DNSPeerPendingJournalUnresolved
}

// These fallbacks are used by older clients without the translated reason key.
// Each sentence identifies the current uncertainty, actor, action and how the
// same accepted publication can resume. No peer output or credential is shown.
func dnsPeerPendingEnglish(code string) string {
	switch code {
	case transport.DNSPeerPendingEnrollmentRequired:
		return "The DNS change is saved on this server, but the secondary has not been shown to have removed the zone: no inspection access is set up between the two servers. On two CelikPanel servers joined with a setup join code, setup sets it up by itself and either server's setup page says why it has not; otherwise the administrators of both servers set it up once with the dns-peer-enroll owner tool (add --engine pdns to each command when the secondary runs PowerDNS): primary-prepare here, secondary-install and secondary-host-key on the secondary, then primary-activate here. CelikPanel checks this same publication again by itself every few minutes and continues it from where it stopped once the access is set up."
	case transport.DNSPeerPendingEnrollmentChanged:
		return "The DNS change is saved, but peer inspection enrollment could not be trusted. This server's administrator must check its peer identity and credential enrollment; CelikPanel checks this same publication again by itself every few minutes and continues it once that is correct."
	case transport.DNSPeerPendingInspectionUnknown:
		return "The DNS change is saved on this server, but the backup DNS server answered this server's check without confirming it. This server's administrator and the backup server's owner check that the backup still lets this server run its check; CelikPanel checks this same publication again by itself every few minutes and continues it once the backup confirms."
	case transport.DNSPeerPendingNativeUnknown:
		return "The DNS change is saved, but the secondary's native secondary DNS state did not prove the deleted zone absent. The secondary owner must inspect its loaded zones and transfer state; CelikPanel checks this same publication again by itself every few minutes and continues it once the secondary confirms."
	case transport.DNSPeerPendingJournalUnknown:
		// A waiting state first (D-024; final alpha.113 acceptance run 4,
		// D7): the Panel re-checks it by itself, so the administrator step
		// is only for a result that lasts.
		return dnsPeerJournalUnknownEnglish
	case transport.DNSPeerPendingJournalUnresolved:
		return dnsPeerJournalUnresolvedEnglish
	case transport.DNSPeerPendingOwnerEditUnknown:
		return "The DNS change is saved, but local or peer evidence changed during verification. This server's administrator must reconcile the accepted operation with native DNS configuration: compare the catalog zone and zone serials on both servers; CelikPanel checks this same publication again by itself every few minutes and continues it once the evidence matches."
	case transport.DNSPeerPendingCatalogTransferRefused:
		return "The DNS change is saved, but the secondary has not been shown to have removed the zone: the secondary's named refused the inspector's local transfer of the catalog zone (allow-transfer). The secondary's owner allows that transfer from loopback on the secondary. A secondary whose DNS CelikPanel set up with this release already allows it; on one set up by an earlier release it arrives when CelikPanel next writes that server's DNS configuration. On a secondary you run without CelikPanel, add 127.0.0.1 and ::1 to the catalog zone's allow-transfer in named's configuration and reload named. CelikPanel checks this same publication again by itself every few minutes and continues it from where it stopped once the transfer is allowed."
	case transport.DNSPeerPendingProofInternal:
		return "The DNS change is saved on this server, but the deletion is not verified yet: this server could not run its own check of the secondary. No change by either server's owner was found, and nothing on either server needs to be undone. The server owner checks the CelikPanel Agent log on this server with sudo journalctl -u celikpanel-agent | grep peer and reports those lines to CelikPanel. Checking again does not help until the CelikPanel Agent is updated with a fix; CelikPanel checks this same publication again by itself every few minutes, so after that update it continues from where it stopped with nothing to press. Until then the deletion stays pending and DNS answers are unaffected."
	case transport.DNSPeerPendingProofTimeout:
		// alpha.133 (incident of 12 Nov 2026): the deletion is already in
		// effect on this server; only the backup's confirmation is open.
		// The side that ran out of time is the detail sentence.
		return dnsPeerProofTimeoutEnglish
	case transport.DNSPeerPendingCatchingUp:
		return dnsPeerCatchingUpEnglish
	default:
		return "The DNS change is saved, but paired deletion is unverified. The server administrator must inspect the exact DNS operation and secondary state; CelikPanel checks this same publication again by itself every few minutes."
	}
}

// dnsPeerInspectorDetailEnglish is the product sentence for one reviewed
// reason the secondary's inspector reported. It is shown after the reason's
// own text; it never repeats inspector, SSH or native command output.
func dnsPeerInspectorDetailEnglish(detail string) string {
	switch detail {
	case transport.DNSPeerInspectorReasonPolicy:
		return "The secondary's inspector reported that its owner policy is missing or does not match this request; the secondary's owner checks it with dns-peer-enroll secondary-status."
	case transport.DNSPeerInspectorReasonNamedUnavailable:
		return "The secondary's inspector reported that named is not running under its standard service and configuration, or that its configuration could not be read."
	case transport.DNSPeerInspectorReasonListenersUnverified:
		// 6 Nov 2026: usually an older check tool; the backup's own Agent
		// puts the current one in place when that server is updated.
		return "The backup DNS server's check tool could not confirm named's listeners. This is usually an older check tool: CelikPanel puts its current one in place when the backup DNS server is updated, and this deletion is checked again by itself. Nobody needs to do anything. Only if the check tool was changed by hand does the backup DNS server's DNS page show the one command to run."
	case transport.DNSPeerInspectorReasonCatalogUnverified:
		return "The secondary's inspector reported that named does not hold the primary's catalog zone as a subscribed secondary zone."
	case transport.DNSPeerInspectorReasonCatalogTransferFailed:
		return "The secondary's inspector reported that the local transfer of the catalog zone did not complete."
	case transport.DNSPeerInspectorReasonCatalogMalformed:
		return "The secondary's inspector reported that the transferred catalog zone does not have the expected content."
	case transport.DNSPeerInspectorReasonObservationExpired:
		return "The secondary's inspector reported that the request expired before it finished; check that both servers' clocks are correct."
	case transport.DNSPeerInspectorReasonConfigUnreviewed:
		return "The secondary's inspector reported that PowerDNS is not running with a configuration it recognises (the panel's own or the documented panel-free one)."
	default:
		return ""
	}
}

// dnsPeerOwnerEditDetailEnglish names, for dns_peer_owner_edit_unknown, the
// one check of the Agent's proof that found different evidence. It is shown
// after the reason's own text and never carries an observed value; the
// Agent log has the recorded and observed values.
func dnsPeerOwnerEditDetailEnglish(check string) string {
	switch check {
	case transport.DNSPeerOwnerEditCheckOperationAttempt:
		return "What differed: this server's record of which attempt is running this deletion changed during the check."
	case transport.DNSPeerOwnerEditCheckEngineState:
		return "What differed: this server's saved DNS engine state changed during the check."
	case transport.DNSPeerOwnerEditCheckActiveEngine:
		return "What differed: which DNS service runs on this server changed during the check; only the panel's DNS server may be running."
	case transport.DNSPeerOwnerEditCheckNativeBinding:
		return "What differed: the DNS server process on this server, its database or its configuration files changed during the check (for example the service was restarted or its configuration was edited)."
	case transport.DNSPeerOwnerEditCheckDeletionReceipt:
		return "What differed: this server's saved record of this deletion no longer matches the accepted change."
	case transport.DNSPeerOwnerEditCheckProducerCatalog:
		return "What differed: this server's catalog zone changed in a way the DNS server does not make by itself (its member zones, their serials or the catalog's identity)."
	case transport.DNSPeerOwnerEditCheckCatalogProbe:
		return "What differed: after the secondary's inspection, the catalog zone served by this server or by the secondary no longer matched this server's saved catalog."
	case transport.DNSPeerOwnerEditCheckAuthority:
		return "What differed: after the secondary's inspection, the catalog zone answered for a different pair of servers or a different catalog."
	case transport.DNSPeerOwnerEditCheckTransferObserved:
		return "What differed: after the secondary's inspection, the secondary did not refuse a transfer of the deleted zone, or that check did not complete."
	case transport.DNSPeerOwnerEditCheckZoneAnswered:
		return "What differed: after the secondary's inspection, the secondary's answer for the deleted zone was no longer the plain refusal seen before, or the query did not complete."
	default:
		return ""
	}
}

// dnsPeerPendingDetailEnglish selects the detail sentence by the reason the
// detail refines: an inspector reason, an owner-edit check or the side of a
// proof timeout.
func dnsPeerPendingDetailEnglish(reason, detail string) string {
	switch reason {
	case transport.DNSPeerPendingInspectionUnknown:
		return dnsPeerInspectorDetailEnglish(detail)
	case transport.DNSPeerPendingOwnerEditUnknown:
		return dnsPeerOwnerEditDetailEnglish(detail)
	case transport.DNSPeerPendingProofTimeout:
		return dnsPeerProofTimeoutDetailEnglish(detail)
	default:
		return ""
	}
}

// dnsPeerPendingGuidance splits a verified pending code into the reviewed
// reason, its optional reviewed detail (inspector reason or owner-edit check)
// and the English fallback.
func dnsPeerPendingGuidance(code string) (reason, detail, message string, ok bool) {
	reason, detail, ok = transport.SplitDNSPeerPendingCode(code)
	if !ok {
		return "", "", "", false
	}
	message = dnsPeerPendingEnglish(reason)
	if sentence := dnsPeerPendingDetailEnglish(reason, detail); sentence != "" {
		message += " " + sentence
	} else {
		detail = ""
	}
	return reason, detail, message, true
}

// dnsPeerPendingAPIError recognizes only a reviewed code carried by the
// verified pending operation. Unknown data must use the generic fallback.
func dnsPeerPendingAPIError(err error) (apiErrorBody, bool) {
	var pending *dnsZoneV3PropagationPendingError
	if !errors.As(err, &pending) || !pending.Exact {
		return apiErrorBody{}, false
	}
	reason, detail, message, ok := dnsPeerPendingGuidance(pending.Code)
	if !ok {
		return apiErrorBody{}, false
	}
	return apiErrorBody{
		Code: errCodeDNSPublicationFailed, Reason: reason, Detail: detail,
		Error: message,
	}, true
}

// dnsPublicationFailureReasonError is a verified local publication failure
// with a reviewed cause from the Agent (transport.ValidDNSPublicationFailure-
// Reason). The change was not applied (or was rolled back); retrying the same
// accepted publication continues it. A lost RPC response loses the reason and
// falls back to the generic failure text.
type dnsPublicationFailureReasonError struct {
	Reason string
}

func (e *dnsPublicationFailureReasonError) Error() string {
	return "agent reported a verified DNS publication failure: " + e.Reason
}

// dnsPublicationFailureEnglish is the fallback for clients without the
// translated reason key. Reason, actor, action and resumption come first; no
// key material or tool output is shown.
func dnsPublicationFailureEnglish(reason string) string {
	switch reason {
	case transport.DNSPublicationFailureBINDRNDCUnavailable:
		return "The DNS change is saved, but this server could not prove the zone was removed: named cannot be asked about zone state because no usable rndc key is present. The server owner fixes this on this server: run sudo rndc-confgen -a, then sudo systemctl restart named. The restart briefly interrupts DNS answers from this server; on Debian the unit is also named and the key is /etc/bind/rndc.key. Then use “Retry this deletion” to retry the same publication. Until then the deletion stays pending, nothing else is changed and DNS answers are unaffected."
	default:
		return "The DNS change is saved, but it could not be published or verified. The server administrator must inspect the exact DNS operation, then retry this same publication."
	}
}

// dnsPublicationAgentHeldReason names an exact DNS publication that the Agent
// cannot move because it refuses every durable change (its hold). It is a
// panel-side reason derived from the Agent's read-only status, not an Agent
// failure reason.
const dnsPublicationAgentHeldReason = "agent_mutations_held"

// dnsPublicationAgentHeldEnglish: reason, actor, next action and resumption.
const dnsPublicationAgentHeldEnglish = "The DNS change is saved, but the CelikPanel Agent on this server is holding every change: it could not verify the result of an earlier operation, and the Agent log names that operation. Websites, mail and DNS answers keep running. Next step: the server owner restarts the CelikPanel Agent on this server (sudo systemctl restart celikpanel-agent); at start it checks that same operation again and releases the hold once it is proven. Then retry this same change (for a domain deletion use “Retry this deletion”); it continues from where it stopped and nothing is done twice. If the hold returns after the restart, leave the server as it is and send the output of sudo journalctl -u celikpanel-agent -n 200 to CelikPanel support."

// dnsPublicationGuidanceAPIError recognizes a reviewed peer pending code, a
// held Agent or a reviewed local failure reason; anything else keeps the
// caller's generic text.
func dnsPublicationGuidanceAPIError(err error) (apiErrorBody, bool) {
	if guidance, ok := dnsPeerPendingAPIError(err); ok {
		return guidance, true
	}
	var held *agentMutationHeldError
	if errors.As(err, &held) {
		return apiErrorBody{
			Code: errCodeDNSPublicationFailed, Reason: dnsPublicationAgentHeldReason,
			Error: dnsPublicationAgentHeldEnglish,
		}, true
	}
	var failure *dnsPublicationFailureReasonError
	if !errors.As(err, &failure) || !transport.ValidDNSPublicationFailureReason(failure.Reason) {
		return apiErrorBody{}, false
	}
	return apiErrorBody{
		Code: errCodeDNSPublicationFailed, Reason: failure.Reason,
		Error: dnsPublicationFailureEnglish(failure.Reason),
	}, true
}
