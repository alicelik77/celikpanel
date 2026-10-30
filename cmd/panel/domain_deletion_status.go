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
	// deletion still waits for "Retry this deletion".
	dnsRemoved bool
}

// domainDeletionDNSStageDone is the stage of a deletion whose DNS removal is
// complete while its later stages have not run yet.
const domainDeletionDNSStageDone = "dns_removed"

// domainDeletionDNSRemovalConfirmedReason names that state for the screen.
const domainDeletionDNSRemovalConfirmedReason = "dns_removal_confirmed"

const domainDeletionDNSRemovedEnglish = "The DNS removal of this domain is complete and verified, including on the backup DNS server. The rest of the deletion has not run yet: use “Retry this deletion” to finish removing the domain from this server. Nothing else is waiting."

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
		return snapshot, nil
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
	}
	return snapshot, nil
}

func sameDomainDeletionStatusSnapshot(a, b domainDeletionStatusSnapshot) bool {
	return a.marked && b.marked && a.candidate && b.candidate &&
		a.domain == b.domain && a.lease == b.lease &&
		a.state == b.state && a.engine == b.engine
}

type domainDeletionStatusResponse struct {
	Status string `json:"status"`
	Stage  string `json:"stage"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
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
			Status: "unknown", Stage: "unknown",
			Message: "The saved deletion marker and domain state disagree. The server administrator must inspect the operation before retrying.",
		})
		return
	}
	if p.deletionsRunning.isRunning(domainID) {
		// The request is still working and re-checks by itself; what it
		// saved so far is not its outcome (D-024).
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionRunningStage,
			Reason:  domainDeletionRunningReason,
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
	if snapshot.dnsRemoved {
		_ = json.NewEncoder(w).Encode(domainDeletionStatusResponse{
			Status: domainDeletionPendingStatus, Stage: domainDeletionDNSStageDone,
			Reason:  domainDeletionDNSRemovalConfirmedReason,
			Message: domainDeletionDNSRemovedEnglish,
		})
		return
	}
	result := domainDeletionStatusResponse{
		Status:  "unknown",
		Stage:   "unknown",
		Message: "Deletion is pending. The server administrator should inspect the saved operation before retrying this same deletion.",
	}
	if snapshot.candidate {
		statusCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		job, statusErr := p.statusAgentMutation(statusCtx, snapshot.lease.RequestID)
		cancel()
		if statusErr == nil && validateDNSZoneSyncV3PendingJob(job, snapshot.lease.identity()) == nil {
			result.Status = domainDeletionPendingStatus
			result.Stage = "dns_cleanup"
			after, rereadErr := p.readDomainDeletionStatusSnapshot(r.Context(), domainID)
			if rereadErr != nil || !sameDomainDeletionStatusSnapshot(snapshot, after) {
				result.Status = "unknown"
				result.Stage = "unknown"
				result.Message = "The saved deletion state changed during the read. The server administrator must inspect the operation before retrying."
			} else if reason, detail, message, ok := dnsPeerPendingGuidance(
				dnsZoneV3PendingCodeFromJob(job),
			); ok {
				result.Reason = reason
				result.Detail = detail
				result.Message = message
			}
		}
	}
	_ = json.NewEncoder(w).Encode(result)
}
