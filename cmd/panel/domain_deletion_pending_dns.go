package main

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

// A deletion whose DNS step waits on this server (alpha.129).
//
// alpha.128 lab, round 1: on a DNS pair's main server a domain made with an
// API token answered 409 DNS_PUBLICATION_PENDING (its creation waited for the
// backup DNS server); the token's deletion then answered 202 at dns_cleanup
// with "Retry this deletion", which a token cannot press, and the status
// said the deletion had stopped. The removal itself was saved and was
// published by the pending-publication reconciler once the earlier change
// completed (dns_pending_publication.go), and the deletion reconciler
// finishes a deletion whose removal is confirmed (ddbd6975b,
// reconcileLocalDNSDeletionsOnce). What was missing is the answer:
//
//   - the 202 now says who acts and that the deletion finishes by itself
//     when its DNS step completes by itself (and names status_url, which a
//     token may read: GET /api/v1/domains/{id}/deletion-status);
//   - the status names an earlier publication of the same zone that the
//     removal waits for, a removal held behind another zone's publication,
//     and a removal that failed (a plain reason, nothing more by itself,
//     "Retry this deletion"), instead of "the deletion stopped";
//   - a second press while the deletion continues by itself starts nothing.
//
// Every read here is read only; no status read starts work (D-024).
//
// Ana DNS sunucusunda DNS adımı bekleyen silme: yanıt ve durum, silmenin DNS
// adımı kendiliğinden tamamlanınca kendiliğinden biteceğini, başarısız olursa
// düz nedeni ve "Bu silmeyi yeniden dene" eylemini söyler.

// domainDeletionStatusURL is the read-only status of one deletion.
func domainDeletionStatusURL(domainID int) string {
	return "/api/v1/domains/" + strconv.Itoa(domainID) + "/deletion-status"
}

// domainDeletionFinishesByItselfEnglish follows a DNS reason whose next step
// CelikPanel takes by itself.
const domainDeletionFinishesByItselfEnglish = "The rest of this deletion waits only for this DNS step: once it completes, this server finishes the deletion by itself. Nobody needs to press anything; the deletion status (status_url) and the domain list show when it is done."

// Reasons of a local deletion's DNS step that are not a peer code.
const (
	// The removal is saved and waits for an earlier DNS change (of another
	// domain, or the backup's confirmation of the zone list) to complete;
	// CelikPanel publishes it by itself afterwards.
	domainDeletionDNSRemovalWaitingReason = "dns_removal_waiting"
	// The removal ended failed on this server: nothing more by itself.
	domainDeletionDNSRemovalFailedReason = "dns_removal_failed"
)

const domainDeletionDNSRemovalWaitingEnglish = "This domain's removal from DNS is saved and waits for an earlier DNS change to complete: DNS changes are published one at a time, and the earlier one is waiting for the backup DNS server. Nobody needs to do anything: CelikPanel publishes the removal by itself once the earlier change completes, then finishes the deletion by itself. Server → DNS and peer servers shows the next check."

const domainDeletionDNSRemovalFailedEnglish = "Removing this domain from DNS did not succeed, so the deletion stopped at this step. Its website and mail settings were removed before it; the domain stays in the list until its DNS removal succeeds, and nothing more happens by itself. The server administrator checks Server → DNS and peer servers (it shows whether this server's DNS service and the backup DNS server answer), then uses “Retry this deletion”."

// domainDeletionDNSFinishRetryEnglish: the removal is confirmed, but the
// last attempt to finish the rest of the deletion did not succeed.
const domainDeletionDNSFinishRetryEnglish = "The DNS removal of this domain is complete and verified, but the last attempt to finish the rest of the deletion (its certificates and its entry in the panel) did not succeed. This server tries again by itself after a growing pause (5 minutes, then 15 minutes, then an hour); if it still fails, this page names the error and who acts. “Retry this deletion” tries again now."

// domainDeletionDNSWithoutLease classifies a parentless local deletion whose
// zone has a deletion marker and a saved removal but no publication lease of
// its own: waiting (held behind another zone's exact publication, which the
// reconciler owns and after which it publishes held zones), held alone (held
// with no other publication pending: the caller decides whether the backup's
// zone-list wait publishes it by itself), or failed (the removal ended with
// an error that is not a hold). Read only.
func domainDeletionDNSWithoutLease(ctx context.Context, tx *sql.Tx, state dnsZoneSyncState) (waiting, heldAlone, failed bool, err error) {
	if state.DesiredAction != "delete" || state.SourceDomainID.Valid || state.DesiredGeneration <= state.AppliedGeneration {
		return false, false, false, nil
	}
	held := state.Status == "pending"
	if state.Status == "error" {
		lastError := strings.TrimSpace(state.LastError.String)
		held = strings.Contains(lastError, dnsAgentPendingRecoveryRefusal) || lastError == errHostMutationBusy.Error()
		if !held {
			return false, false, true, nil
		}
	}
	if !held {
		return false, false, false, nil
	}
	var leases int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM dns_zone_engine_leases WHERE zone_name <> ?`, state.ZoneName).Scan(&leases); err != nil {
		return false, false, false, err
	}
	return leases > 0, leases == 0, false, nil
}

// domainDeletionStatusPublisher is the DNS publisher as screens read it
// (cached display check, never a mutation); a variable for tests.
var domainDeletionStatusPublisher = func(p *Panel, ctx context.Context) (dnsPublisherIdentity, bool, error) {
	return p.activeDNSPublisherForDisplay(ctx)
}

// domainDeletionPairWaitStatus answers for a removal held with no other
// publication pending, when the hold is the backup DNS server's confirmation
// of the zone list and the reconciler is armed to publish held zones
// (dns_sync.go arms it with the hold). alpha.129 lab, case 6: the 202 said
// the deletion finishes by itself while the status said "stopped … delete
// again"; it then finished 31 s after the backup answered. Without the armed
// reconciler (for example after a panel restart) nothing publishes it by
// itself and the answer stays "incomplete" (delete again). Read only: memory
// and the cached display check of the DNS publisher.
func (p *Panel) domainDeletionPairWaitStatus(ctx context.Context) (domainDeletionStatusResponse, bool) {
	scheduler := p.dnsPendingPublications()
	scheduler.mu.Lock()
	armed := scheduler.republishArmed
	scheduler.mu.Unlock()
	if !armed {
		return domainDeletionStatusResponse{}, false
	}
	publisher, ready, err := domainDeletionStatusPublisher(p, ctx)
	if err != nil || !dnsHeldStatusIsPairWait(publisher, ready) {
		return domainDeletionStatusResponse{}, false
	}
	held := &dnsPublicationHeldError{Automatic: true}
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
		Reason:  held.heldReason(),
		Message: held.englishMessage() + " " + domainDeletionFinishesByItselfEnglish,
	}, true
}

// domainDeletionAheadLease: the zone's exact publication lease is an
// earlier change of the same zone (its creation or a record change) that the
// saved removal waits for.
func domainDeletionAheadLease(domain string, state dnsZoneSyncState, lease dnsZoneEngineLease, engine dnsEngineDBState) bool {
	return state.ZoneName == domain && state.Status == "pending" && state.DesiredAction == "delete" &&
		!state.SourceDomainID.Valid && state.DesiredGeneration > state.AppliedGeneration &&
		lease.valid() && lease.ZoneName == domain && lease.DesiredGeneration < state.DesiredGeneration &&
		engine.CurrentSwitchID == "" && engine.ActiveEngine == lease.Engine && engine.EngineEpoch == lease.EngineEpoch
}

// domainDeletionDNSPendingAnswer adds the deletion's own next step to the DNS
// guidance of a 202 at dns_cleanup on this server: a reviewed pending code or
// a hold that CelikPanel resolves by itself. It reports whether it answered.
func domainDeletionDNSPendingAnswer(response map[string]string, cause error) bool {
	if guidance, ok := dnsPublicationGuidanceAPIError(cause); ok {
		response["reason"] = guidance.Reason
		response["message"] = guidance.Error
		if guidance.Detail != "" {
			response["detail"] = guidance.Detail
		}
		var pending *dnsZoneV3PropagationPendingError
		if errors.As(cause, &pending) && pending.Exact && dnsPeerPendingRetriesAutomatically(pending.Code) {
			response["message"] += " " + domainDeletionFinishesByItselfEnglish
		}
		return true
	}
	var held *dnsPublicationHeldError
	if errors.As(cause, &held) {
		if reason := held.heldReason(); reason != "" && reason != dnsPublicationHeldPendingVerification {
			response["reason"] = reason
		}
		response["message"] = held.englishMessage()
		if held.Automatic {
			response["message"] += " " + domainDeletionFinishesByItselfEnglish
		}
		return true
	}
	return false
}

// localDomainDeletionContinuesByItself: a parentless local deletion whose DNS
// step is in CelikPanel's own hands: its removal is confirmed and the rest
// has not failed yet, its exact publication (or the earlier one it waits
// for) is pending with a cause re-checked automatically, or it is held
// behind another zone's publication. Read only (one status read of the
// exact Agent job). The caller holds the domain's operation lock.
func (p *Panel) localDomainDeletionContinuesByItself(ctx context.Context, domainID int) (bool, error) {
	snapshot, err := p.readDomainDeletionStatusSnapshot(ctx, domainID)
	if err != nil || !snapshot.marked || snapshot.inconsistent || snapshot.failed || snapshot.remote {
		return false, err
	}
	switch {
	case snapshot.dnsRemoved:
		// Continues by itself until finishing has failed once; then a press
		// is the person's retry (it starts the backoff again).
		_, failed := readDomainDeletionFinishRecord(ctx, p.db.GetDB(), domainID, snapshot.domain)
		return !failed, nil
	case snapshot.dnsWaiting:
		return p.dnsPendingPublicationsAutomatic(), nil
	case snapshot.dnsHeldAlone:
		_, waiting := p.domainDeletionPairWaitStatus(ctx)
		return waiting, nil
	case snapshot.candidate || snapshot.ahead:
		code, pending, active, err := p.readPendingDNSPublicationStatus(ctx, snapshot.lease)
		if err != nil {
			return false, nil
		}
		if active {
			return true, nil
		}
		if !pending || !dnsPeerPendingRetriesAutomatically(code) {
			return false, nil
		}
		// Stuck (the same cause for a quarter of an hour, administrators
		// notified): a press is a person's retry again.
		if entry, ok := p.dnsPendingPublications().entry(snapshot.lease.RequestID); ok && (entry.Stopped || entry.Stuck) {
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

// dnsPendingPublicationsAutomatic: every exact publication the reconciler
// knows is re-checked automatically (none needs a person, stopped after
// repeated failures or is stuck on the same cause). Memory only; read only.
func (p *Panel) dnsPendingPublicationsAutomatic() bool {
	scheduler := p.dnsPendingPublications()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	for _, entry := range scheduler.entries {
		if !entry.Automatic || entry.Stopped || entry.Stuck {
			return false
		}
	}
	return true
}

// domainDeletionDNSWaitingStatus: a removal held behind another zone's
// publication; who acts depends on that publication's cause.
func (p *Panel) domainDeletionDNSWaitingStatus() domainDeletionStatusResponse {
	if !p.dnsPendingPublicationsAutomatic() {
		return domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
			Message: dnsPublicationHeldMessage + " The deletion of this domain then finishes by itself.",
		}
	}
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
		Reason: domainDeletionDNSRemovalWaitingReason, Message: domainDeletionDNSRemovalWaitingEnglish,
	}
}
