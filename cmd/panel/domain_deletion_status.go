package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

type domainDeletionStatusSnapshot struct {
	domain       string
	marked       bool
	inconsistent bool
	candidate    bool
	lease        dnsZoneEngineLease
	state        dnsZoneSyncState
	engine       dnsEngineDBState
	// failure is the last verified stage failure kept with the marker.
	failure domainDeletionFailureRecord
	failed  bool
	// dnsRemoved: the zone's removal was published and verified (its
	// deletion marker retired, no zone, no publication pending), for example
	// by the automatic re-check after the backup caught up; the rest of the
	// deletion is finished by reconcileLocalDNSDeletionsOnce (alpha.128).
	dnsRemoved bool
	// ahead (alpha.129): the zone's exact pending publication is an earlier
	// change of the same zone (for example its creation) that the saved
	// removal waits for; lease, state and engine are then that publication's.
	ahead bool
	// dnsWaiting: the removal is saved and held behind another zone's
	// exact publication; dnsFailed: the removal ended with an error that is
	// not a hold (domain_deletion_pending_dns.go).
	dnsWaiting bool
	dnsFailed  bool
	// dnsHeldAlone: the removal is saved and held with no other publication
	// pending (the backup's zone-list wait; domainDeletionPairWaitStatus).
	dnsHeldAlone bool
	// remote: a joined server's deletion (the zone is on the main DNS
	// server); remoteDNS is what its saved records say.
	remote    bool
	remoteDNS remoteDomainDeletionDNS
}

// domainDeletionDNSStageDone is the stage of a deletion whose DNS removal is
// complete while its later stages have not run yet.
const domainDeletionDNSStageDone = "dns_removed"

// domainDeletionDNSRemovalConfirmedReason names that state for the screen.
const domainDeletionDNSRemovalConfirmedReason = "dns_removal_confirmed"

const domainDeletionDNSRemovedEnglish = "The DNS removal of this domain is complete and verified (on the backup DNS server too, when this server has one). This server finishes the rest of the deletion by itself within a minute; nobody needs to do anything. If this message stays for more than ten minutes, use “Retry this deletion”."

// domainDeletionDNSRemovalConfirmed reports a zone whose deletion was applied
// exactly: no zone row, no publication lease of either version, and an
// applied deletion in its sync state (a domain that never had a zone is not
// reported as removed). Read only.
func domainDeletionDNSRemovalConfirmed(ctx context.Context, tx *sql.Tx, zone string) (bool, error) {
	var zones, leases int
	if err := tx.QueryRowContext(ctx, `
		SELECT (SELECT count(*) FROM pdns_domains WHERE name = ?),
		       (SELECT count(*) FROM dns_zone_engine_leases WHERE zone_name = ?)`,
		zone, zone).Scan(&zones, &leases); err != nil {
		return false, err
	}
	if zones != 0 || leases != 0 {
		return false, nil
	}
	state, err := readDNSZoneSyncState(ctx, tx, zone)
	if errors.Is(err, sql.ErrNoRows) {
		// Retiring the deletion marker also removes the applied tombstone
		// (migration 032); the active engine's application row keeps the
		// applied deletion.
		var applied int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM dns_zone_engine_applications a
			JOIN dns_engine_state e ON e.singleton_id = 1
			WHERE a.zone_name = ? AND a.applied_action = 'delete'
			  AND a.engine = e.active_engine`, zone).Scan(&applied); err != nil {
			return false, err
		}
		return applied == 1, nil
	}
	if err != nil {
		return false, err
	}
	return !state.hasLease() && state.DesiredAction == "delete" && state.Status == "applied" &&
		state.AppliedGeneration == state.DesiredGeneration, nil
}

func (p *Panel) readDomainDeletionStatusSnapshot(ctx context.Context, domainID int) (domainDeletionStatusSnapshot, error) {
	var snapshot domainDeletionStatusSnapshot
	tx, err := p.db.GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return snapshot, err
	}
	defer tx.Rollback()

	var status, mode string
	var parent sql.NullInt64
	err = tx.QueryRowContext(ctx, `
        SELECT d.name, d.status, d.dns_management, d.parent_domain_id
        FROM domains d
        JOIN domain_deletion_operations op ON op.domain_id = d.id
        WHERE d.id = ?`, domainID).Scan(&snapshot.domain, &status, &mode, &parent)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	snapshot.marked = true
	if status != domainDeletionLedgerStatus {
		snapshot.inconsistent = true
		return snapshot, nil
	}
	snapshot.failure, snapshot.failed, err = readDomainDeletionFailure(ctx, tx, domainID, snapshot.domain)
	if err != nil {
		return snapshot, err
	}
	if snapshot.failed {
		// The latest attempt stopped at a recorded stage; the DNS stage was
		// not reached in that attempt, so no DNS evidence supersedes it.
		return snapshot, nil
	}
	if mode == setupDNSModeExisting {
		snapshot.remote = true
		snapshot.remoteDNS, err = readRemoteDomainDeletionDNS(ctx, tx, domainID, snapshot.domain)
		return snapshot, err
	}
	if mode != setupDNSModeLocal || parent.Valid {
		return snapshot, nil
	}

	var markerType string
	err = tx.QueryRowContext(ctx, `SELECT zone_type FROM dns_zone_deletion_markers WHERE zone_name = ?`, snapshot.domain).Scan(&markerType)
	if errors.Is(err, sql.ErrNoRows) {
		snapshot.dnsRemoved, err = domainDeletionDNSRemovalConfirmed(ctx, tx, snapshot.domain)
		return snapshot, err
	}
	if err != nil {
		return snapshot, err
	}
	state, err := readDNSZoneSyncState(ctx, tx, snapshot.domain)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	lease, err := readDNSZoneEngineLease(ctx, tx, snapshot.domain)
	if errors.Is(err, sql.ErrNoRows) {
		snapshot.dnsWaiting, snapshot.dnsHeldAlone, snapshot.dnsFailed, err = domainDeletionDNSWithoutLease(ctx, tx, state)
		return snapshot, err
	}
	if err != nil {
		return snapshot, err
	}
	engineState, err := readDNSEngineDBState(ctx, tx)
	if err != nil {
		return snapshot, err
	}
	if state.ZoneName == snapshot.domain && state.Status == "pending" &&
		engineState.CurrentSwitchID == "" &&
		engineState.ActiveEngine == lease.Engine && engineState.EngineEpoch == lease.EngineEpoch &&
		state.DesiredAction == "delete" && !state.SourceDomainID.Valid &&
		state.DesiredGeneration > state.AppliedGeneration &&
		state.DesiredZoneType == markerType &&
		lease.valid() && lease.ZoneName == snapshot.domain &&
		// Either managed primary engine can hold a zone-sync V3 deletion
		// pending on its peer proof (a PowerDNS primary uses the same
		// typed codes); the lease must still name the active engine.
		(lease.Engine == transport.DNSEngineBIND || lease.Engine == transport.DNSEnginePowerDNS) &&
		lease.DesiredAction == "delete" &&
		lease.DesiredGeneration == state.DesiredGeneration &&
		lease.DesiredZoneType == markerType {
		snapshot.candidate = true
		snapshot.lease = lease
		snapshot.state = state
		snapshot.engine = engineState
	} else if domainDeletionAheadLease(snapshot.domain, state, lease, engineState) {
		snapshot.ahead = true
		snapshot.lease = lease
		snapshot.state = state
		snapshot.engine = engineState
	}
	return snapshot, nil
}

func sameDomainDeletionStatusSnapshot(a, b domainDeletionStatusSnapshot) bool {
	return a.marked && b.marked && a.candidate == b.candidate && a.ahead == b.ahead &&
		(a.candidate || a.ahead) &&
		a.domain == b.domain && a.lease == b.lease &&
		a.state == b.state && a.engine == b.engine
}

type domainDeletionStatusResponse struct {
	Status string `json:"status"`
	Stage  string `json:"stage"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Step: while this server's request runs (stage "running"), the stage
	// it is at now, for the progress window's live step. Read only.
	Step string `json:"step,omitempty"`
	// ErrorLine is the bounded first line of a verified stage failure.
	ErrorLine string `json:"error_line,omitempty"`
	Message   string `json:"message"`
}

// handleDomainDeletionStatus only observes the saved domain marker, V3 lease,
// and exact Agent job. It never reconciles a lease or starts deletion work.
func (p *Panel) handleDomainDeletionStatus(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	snapshot, err := p.readDomainDeletionStatusSnapshot(r.Context(), domainID)
	if err != nil {
		log.Printf("read domain deletion status: %v", err)
		writeServerError(w, errors.New("domain deletion status could not be read"))
		return
	}
	if !snapshot.marked {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if snapshot.inconsistent {
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionMarkerCheckStage,
			Reason:  domainDeletionMarkerInconsistentReason,
			Message: domainDeletionMarkerInconsistentEnglish,
		})
		return
	}
	if p.deletionsRunning.isRunning(domainID) {
		// The request is still working and re-checks by itself; what it
		// saved so far is not its outcome (D-024).
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionRunningStage,
			Reason:  domainDeletionRunningReason,
			Step:    p.deletionsRunning.step(domainID),
			Message: domainDeletionRunningEnglish,
		})
		return
	}
	if snapshot.failed {
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionStatusFailed, Stage: snapshot.failure.Stage,
			Reason: snapshot.failure.Reason, ErrorLine: snapshot.failure.ErrorLine,
			Message: domainMailCleanupFailedEnglish,
		})
		return
	}
	if snapshot.remote {
		_ = json.NewEncoder(w).Encode(p.remoteDomainDeletionStatus(domainID, snapshot.remoteDNS))
		return
	}
	if snapshot.dnsRemoved {
		// A failing finish is said, not hidden; from the third failure with
		// its stage, error line and the person's retry (D-024).
		_ = json.NewEncoder(w).Encode(domainDeletionFinishStatus(
			readDomainDeletionFinishRecord(r.Context(), p.db.GetDB(), domainID, snapshot.domain)))
		return
	}
	if snapshot.dnsFailed {
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
			Reason: domainDeletionDNSRemovalFailedReason, Message: domainDeletionDNSRemovalFailedEnglish,
		})
		return
	}
	if snapshot.dnsWaiting {
		_ = json.NewEncoder(w).Encode(p.domainDeletionDNSWaitingStatus())
		return
	}
	if snapshot.dnsHeldAlone {
		if result, waiting := p.domainDeletionPairWaitStatus(r.Context()); waiting {
			_ = json.NewEncoder(w).Encode(result)
			return
		}
	}
	// No running request, no recorded failure and no DNS step waiting on
	// its own: the deletion stopped before it finished (D-024: never an
	// untyped answer; the next action is the same deletion again).
	result := domainDeletionIncompleteStatus()
	if snapshot.candidate {
		statusCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		job, hold, statusErr := p.statusAgentMutationWithHold(statusCtx, snapshot.lease.RequestID)
		cancel()
		if statusErr == nil && hold != "" && job != nil && agentMutationActive(job.Status) &&
			validateAgentMutationIdentity(job, snapshot.lease.identity()) == nil {
			// The exact DNS removal is frozen by the Agent's hold: name it and
			// the owner's next step instead of "unknown" (D-024).
			result.Status = domainDeletionPendingStatus
			result.Stage = "dns_cleanup"
			result.Reason = dnsPublicationAgentHeldReason
			result.Message = dnsPublicationAgentHeldEnglish
			after, rereadErr := p.readDomainDeletionStatusSnapshot(r.Context(), domainID)
			if rereadErr != nil || !sameDomainDeletionStatusSnapshot(snapshot, after) {
				result = domainDeletionChangingStatus()
			}
		} else if statusErr == nil && validateDNSZoneSyncV3PendingJob(job, snapshot.lease.identity()) == nil {
			// The exact DNS removal is pending; only a reviewed code of its
			// job names a reason (a raw job error is never shown).
			result = domainDeletionStatusResponse{
				Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
				Message: "The DNS removal of this deletion is still pending. The server administrator should inspect the saved operation before retrying this same deletion.",
			}
			after, rereadErr := p.readDomainDeletionStatusSnapshot(r.Context(), domainID)
			if rereadErr != nil || !sameDomainDeletionStatusSnapshot(snapshot, after) {
				result = domainDeletionChangingStatus()
			} else if reason, detail, message, ok := dnsPeerPendingGuidance(
				dnsZoneV3PendingCodeFromJob(job),
			); ok {
				result.Reason = reason
				result.Detail = detail
				result.Message = message
				if dnsPeerPendingRetriesAutomatically(dnsZoneV3PendingCodeFromJob(job)) {
					result.Message += " " + domainDeletionFinishesByItselfEnglish
				}
			}
		}
	}
	if snapshot.ahead {
		result = p.domainDeletionAheadStatus(r.Context(), domainID, snapshot)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// domainDeletionAheadStatus answers for a removal that waits for an earlier
// exact publication of the same zone (alpha.129; alpha.128 lab: a domain
// deleted while its creation still waited for the backup DNS server). Read
// only: one status read of that publication's exact Agent job.
func (p *Panel) domainDeletionAheadStatus(ctx context.Context, domainID int, snapshot domainDeletionStatusSnapshot) domainDeletionStatusResponse {
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	job, statusErr := p.statusAgentMutation(statusCtx, snapshot.lease.RequestID)
	cancel()
	if statusErr != nil || job == nil || validateAgentMutationIdentity(job, snapshot.lease.identity()) != nil {
		return domainDeletionIncompleteStatus()
	}
	// Its recovery running, or a terminal result the reconciler records
	// next: being worked on now.
	result := domainDeletionChangingStatus()
	if validateDNSZoneSyncV3PendingJob(job, snapshot.lease.identity()) == nil {
		code := dnsZoneV3PendingCodeFromJob(job)
		if reason, detail, message, ok := dnsPeerPendingGuidance(code); ok {
			result = domainDeletionStatusResponse{
				Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
				Reason: reason, Detail: detail,
				Message: "An earlier DNS change of this domain is still waiting, and its removal from DNS is published right after it. " + message,
			}
			if dnsPeerPendingRetriesAutomatically(code) {
				result.Message += " " + domainDeletionFinishesByItselfEnglish
			}
		}
	}
	after, err := p.readDomainDeletionStatusSnapshot(ctx, domainID)
	if err != nil || !sameDomainDeletionStatusSnapshot(snapshot, after) {
		return domainDeletionChangingStatus()
	}
	return result
}

// Typed states that replace the former "unknown"/"unknown" answer (alpha.114
// lab run 2, E-new-4; D-024): each names who acts and what happens next.
const (
	domainDeletionIncompleteStage   = "incomplete"
	domainDeletionIncompleteReason  = "deletion_incomplete"
	domainDeletionIncompleteEnglish = "This deletion stopped before it finished, for example because the panel restarted during it. Nothing is lost and nothing more happens by itself. Delete the domain again to finish it; the steps already done are skipped."

	domainDeletionChangingReason  = "deletion_state_changing"
	domainDeletionChangingEnglish = "This deletion's saved state changed while it was being read: it is being worked on right now. Nobody needs to do anything; this page checks again by itself."

	domainDeletionMarkerCheckStage          = "marker_check"
	domainDeletionMarkerInconsistentReason  = "deletion_marker_inconsistent"
	domainDeletionMarkerInconsistentEnglish = "The saved deletion of this domain and the domain's own saved state disagree, so CelikPanel does not continue it by itself. Nothing was removed by this check and the domain's services keep running. The server administrator contacts CelikPanel support with this domain's name before deleting it again."
)

func domainDeletionIncompleteStatus() domainDeletionStatusResponse {
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: domainDeletionIncompleteStage,
		Reason: domainDeletionIncompleteReason, Message: domainDeletionIncompleteEnglish,
	}
}

func domainDeletionChangingStatus() domainDeletionStatusResponse {
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
		Reason: domainDeletionChangingReason, Message: domainDeletionChangingEnglish,
	}
}

// remoteDomainDeletionStatus answers for a joined server's deletion from its
// saved records and this process's last attempt. Read only.
func (p *Panel) remoteDomainDeletionStatus(domainID int, state remoteDomainDeletionDNS) domainDeletionStatusResponse {
	if !state.Reached {
		return domainDeletionIncompleteStatus()
	}
	if state.Done {
		return domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionDNSStageDone,
			Reason: remoteDNSDeletionFinishingReason, Message: remoteDNSDeletionFinishingEnglish,
		}
	}
	reason := p.remoteDNSDeletions().lastReason(domainID)
	switch reason {
	case remoteDNSDeletionUnreachableReason, remoteDNSDeletionRefusedReason:
	default:
		// Before this process's first attempt, or while the main server
		// confirms: the joined server asks again by itself.
		reason = remoteDNSDeletionConfirmingReason
	}
	return domainDeletionStatusResponse{
		Status: domainDeletionPendingStatus, Stage: "dns_cleanup",
		Reason: reason, Message: remoteDNSDeletionEnglish(reason),
	}
}
