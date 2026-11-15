package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

var errDomainDeletionPending = errors.New("domain deletion is pending")

const (
	// The domains table deliberately has a small, schema-enforced lifecycle
	// vocabulary. Pending is the ledger state shared with import work; the
	// domain_deletion_operations row is the durable deletion marker that
	// distinguishes the two and preserves the previous status for rollback.
	domainDeletionLedgerStatus  = "pending"
	domainDeletionPendingStatus = "deletion_pending"
)

// markDomainDeletionPending leaves a durable retry handle before any external
// resource is removed. A crash or partial agent failure therefore cannot make
// the remaining work disappear behind an apparently active/deleted domain.
func (p *Panel) markDomainDeletionPending(ctx context.Context, domainID int) (bool, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin domain deletion marker: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO domain_deletion_operations (domain_id, previous_status)
		SELECT id, status FROM domains WHERE id = ?
		ON CONFLICT(domain_id) DO NOTHING`, domainID)
	if err != nil {
		return false, fmt.Errorf("create domain deletion marker: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("confirm domain deletion marker creation: %w", err)
	}
	firstAttempt := created == 1
	if !firstAttempt {
		var status string
		err := tx.QueryRowContext(ctx, `
			SELECT d.status
			FROM domains d
			JOIN domain_deletion_operations op ON op.domain_id = d.id
			WHERE d.id = ?`, domainID).Scan(&status)
		if err != nil {
			return false, fmt.Errorf("read existing domain deletion marker: %w", err)
		}
		if status != domainDeletionLedgerStatus {
			return false, fmt.Errorf("existing domain deletion marker has unexpected domain status %q", status)
		}
	} else {
		result, err = tx.ExecContext(ctx, `
			UPDATE domains
			SET status = ?, updated_at = datetime('now')
			WHERE id = ?`, domainDeletionLedgerStatus, domainID)
		if err != nil {
			return false, fmt.Errorf("mark domain deletion pending: %w", err)
		}
		changed, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, fmt.Errorf("confirm domain deletion status: %w", rowsErr)
		}
		if changed != 1 {
			return false, fmt.Errorf("mark domain deletion pending: expected one domain, changed %d", changed)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE domain_deletion_operations
		SET updated_at = datetime('now')
		WHERE domain_id = ?`, domainID); err != nil {
		return false, fmt.Errorf("touch domain deletion marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit domain deletion marker: %w", err)
	}
	return firstAttempt, nil
}

func (p *Panel) restoreDomainDeletionStart(ctx context.Context, domainID int) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin domain deletion restoration: %w", err)
	}
	defer tx.Rollback()
	var previousStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT op.previous_status
		FROM domain_deletion_operations op
		JOIN domains d ON d.id = op.domain_id
		WHERE op.domain_id = ? AND d.status = ?`,
		domainID, domainDeletionLedgerStatus,
	).Scan(&previousStatus)
	if err != nil {
		return fmt.Errorf("read domain deletion restoration state: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE domains
		SET status = ?, updated_at = datetime('now')
		WHERE id = ? AND status = ?`,
		previousStatus, domainID, domainDeletionLedgerStatus)
	if err != nil {
		return fmt.Errorf("restore domain status: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm domain status restoration: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("restore domain status: expected one pending domain, changed %d", changed)
	}
	result, err = tx.ExecContext(ctx,
		`DELETE FROM domain_deletion_operations WHERE domain_id = ?`, domainID)
	if err != nil {
		return fmt.Errorf("remove domain deletion marker: %w", err)
	}
	changed, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm domain deletion marker removal: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("remove domain deletion marker: expected one marker, changed %d", changed)
	}
	if err := clearDomainDeletionPlan(ctx, tx, domainID); err != nil {
		return fmt.Errorf("remove domain deletion plan: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit domain deletion restoration: %w", err)
	}
	return nil
}

func (p *Panel) ensureMailDomainMutable(ctx context.Context, domainID int) error {
	var marked int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM domain_deletion_operations WHERE domain_id = ?
		)`, domainID).Scan(&marked)
	if err != nil {
		return fmt.Errorf("read domain deletion marker: %w", err)
	}
	if marked != 0 {
		return errDomainDeletionPending
	}
	return nil
}

func writeMailDomainMutationError(w http.ResponseWriter, err error) {
	if errors.Is(err, errDomainDeletionPending) {
		writeCodedError(
			w,
			http.StatusConflict,
			errCodeDomainDeletionPending,
			"domain deletion is pending; mail settings cannot be changed",
			"",
		)
		return
	}
	writeServerError(w, err)
}

// removeDomainMailRuntimeLocked requires p.mailMutationMu. Keeping marker
// publication, mail-TLS reconciliation and runtime cleanup under that one
// lock prevents another global forwarding snapshot from observing a marker
// that is later rolled back.
func (p *Panel) removeDomainMailRuntimeLocked(
	ctx context.Context,
	domainID int,
	domain string,
) (*domainDeletionKeptMail, error) {
	var storedDomain string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.name
		FROM domains d
		JOIN domain_deletion_operations op ON op.domain_id = d.id
		WHERE d.id = ? AND d.status = ?`,
		domainID, domainDeletionLedgerStatus,
	).Scan(&storedDomain)
	if err != nil {
		return nil, fmt.Errorf("verify domain deletion marker before mail cleanup: %w", err)
	}
	storedDomain = strings.ToLower(strings.TrimSpace(storedDomain))
	if !strings.EqualFold(storedDomain, strings.TrimSpace(domain)) {
		return nil, fmt.Errorf("mail cleanup domain identity changed")
	}

	var response transport.DeleteMailDomainResponse
	err = p.callAgentContext(ctx, "Agent.DeleteMailDomain", &transport.DeleteMailDomainRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		DomainID:            domainID,
		Domain:              storedDomain,
	}, &response)
	if err != nil {
		return nil, fmt.Errorf("remove mail domain runtime: %w", err)
	}
	if !response.Applied {
		return nil, errDomainMailCleanupNotConfirmed
	}
	// alpha.134/135: where the stored mail was moved aside, for the holding list.
	if response.Quarantined && response.QuarantineName != "" {
		return &domainDeletionKeptMail{Name: response.QuarantineName, Path: response.QuarantinePath, Bytes: response.QuarantineBytes}, nil
	}
	return nil, nil
}

func (p *Panel) writeDomainDeletionPending(
	w http.ResponseWriter,
	r *http.Request,
	domainID int,
	domain string,
	stage string,
	cause error,
) {
	p.writeDomainDeletionPendingKept(w, r, domainID, domain, stage, cause, nil)
}

// writeDomainDeletionPendingKept also names, when known, what the deletion
// keeps and which databases it deleted so far (alpha.134).
func (p *Panel) writeDomainDeletionPendingKept(
	w http.ResponseWriter,
	r *http.Request,
	domainID int,
	domain string,
	stage string,
	cause error,
	kept *domainDeletionKept,
) {
	log.Printf("domain deletion pending for %s at %s: %v", domain, stage, cause)
	p.audit(r, "domain.delete.pending:"+domain+":"+stage, "domain", domainID)
	response := map[string]string{
		"status":  domainDeletionPendingStatus,
		"domain":  domain,
		"stage":   stage,
		"message": "Deletion is incomplete but retryable. Retry this deletion.",
		// alpha.129: the read-only status of this deletion (a token reads it too).
		"status_url": domainDeletionStatusURL(domainID),
	}
	if stage == "dns_cleanup" {
		if reason, ok := remoteDNSDeletionReason(cause); ok {
			// A joined server: the main DNS server's answer, typed, and the
			// joined server's own next check (E-new-4).
			response["reason"] = reason
			response["message"] = remoteDNSDeletionEnglish(reason)
			p.remoteDNSDeletions().pendingAnswered(domainID, reason)
		} else {
			// This server's own DNS step: its guidance and, when CelikPanel
			// completes it by itself, that the deletion then finishes by
			// itself too (domain_deletion_pending_dns.go).
			domainDeletionDNSPendingAnswer(response, cause)
			// What happens next, as the status read says it (alpha.133
			// review: the deletion window must not assume CelikPanel
			// re-checks by itself). Read only.
			var pending *dnsZoneV3PropagationPendingError
			if errors.As(cause, &pending) && pending.Exact && response["reason"] != "" {
				if lease, err := readDNSZoneEngineLease(r.Context(), p.db.GetDB(), domain); err == nil {
					followUp, next := p.domainDeletionDNSFollowUp(lease, pending.Code)
					response["follow_up"] = followUp
					if next != "" {
						response["next_check_at"] = next
					}
				}
			}
		}
	}
	var stageFailure *domainDeletionStageFailure
	if errors.As(cause, &stageFailure) && stageFailure.Stage == stage {
		// A verified stage failure: the deletion stays pending and retryable,
		// and the reason names what failed on this server.
		response["reason"] = stageFailure.Reason
		response["error_line"] = stageFailure.ErrorLine
		response["message"] = domainDeletionStageFailureEnglish(stage)
	}
	w.WriteHeader(http.StatusAccepted)
	if kept != nil && !kept.empty() {
		answer := make(map[string]any, len(response)+1)
		for key, value := range response {
			answer[key] = value
		}
		answer["kept"] = kept
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}

// removeDomainDNSForDeletion first commits the local PowerDNS-zone removal.
// Migration 032 turns that ledger delete into a durable tombstone/generation;
// only the exact V2 receipt may retire it. The tenant domain and deletion
// operation rows remain intact until the later saga finalize stage.
func (p *Panel) removeDomainDNSForDeletion(
	ctx context.Context,
	domain string,
	parentDomain string,
) error {
	mode, err := p.domainDNSManagementMode(ctx, domain)
	if err != nil {
		return err
	}
	if mode == setupDNSModeExternal {
		// Provider records belong to the operator. Local site removal must not
		// publish a DNS deletion or claim that external records were removed.
		return nil
	}
	if mode == setupDNSModeExisting {
		return p.syncRemoteDomainDNS(ctx, domain, true)
	}
	if parentDomain != "" {
		_, err := p.removeSubdomainFromParentZone(ctx, parentDomain, domain)
		return err
	}
	// A website added later for the infrastructure zone's own name is removed
	// without its zone, which holds the panels' own address records
	// (2026-10-16). The zone returns to Domains as "DNS only".
	// Altyapi bolgesiyle ayni adli site kaldirilir; bolge kalir.
	if infrastructure, err := p.isInfrastructureDNSZone(ctx, domain); err != nil {
		return fmt.Errorf("check infrastructure DNS zone: %w", err)
	} else if infrastructure {
		return nil
	}
	p.serviceMutationMu.Lock()
	defer p.serviceMutationMu.Unlock()
	dnsPublicationMu.Lock()
	defer dnsPublicationMu.Unlock()
	if err := p.requireDNSZoneSyncV2Agent(ctx); err != nil {
		return fmt.Errorf("verify DNS deletion publisher: %w", err)
	}

	var zoneID int
	err = p.db.GetDB().QueryRowContext(ctx,
		`SELECT id FROM pdns_domains WHERE name = ?`, domain,
	).Scan(&zoneID)
	zoneExists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find DNS zone: %w", err)
	}

	if zoneExists {
		tx, err := p.db.GetDB().BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin DNS ledger cleanup: %w", err)
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(ctx,
			`DELETE FROM pdns_domains WHERE id = ? AND name = ?`, zoneID, domain,
		)
		if err != nil {
			return fmt.Errorf("remove DNS zone from ledger: %w", err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("confirm DNS zone ledger cleanup: %w", err)
		}
		if changed != 1 {
			return fmt.Errorf("remove DNS zone from ledger: expected one zone, changed %d", changed)
		}
		var markerType, desiredAction string
		if err := tx.QueryRowContext(ctx, `
			SELECT marker.zone_type, state.desired_action
			FROM dns_zone_deletion_markers marker
			JOIN dns_zone_sync_state state ON state.zone_name = marker.zone_name
			WHERE marker.zone_name = ?`, domain).Scan(&markerType, &desiredAction); err != nil {
			return fmt.Errorf("verify durable DNS deletion tombstone: %w", err)
		}
		if desiredAction != "delete" || (markerType != "NATIVE" && markerType != "MASTER") {
			return errors.New("durable DNS deletion tombstone has invalid semantics")
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit DNS ledger cleanup: %w", err)
		}
	}

	// A crash at any point after the ledger commit resumes from the persisted
	// lease/tombstone. If the exact delete was already CAS-applied, the state is
	// absent and syncZoneToDNSLocked returns idempotent success.
	if err := p.syncZoneToDNSLocked(ctx, domain, true); err != nil {
		return fmt.Errorf("publish DNS zone deletion: %w", err)
	}
	return nil
}

func (p *Panel) removeManagedDomainCertificates(
	ctx context.Context,
	domain string,
	deleteCanonical bool,
	stagedLineages []string,
) error {
	if !deleteCanonical && len(stagedLineages) == 0 {
		return nil
	}
	var response transport.DeleteCertLineageResponse
	err := p.callAgentContext(ctx, "Agent.DeleteCertLineage", &transport.DeleteCertLineageRequest{
		Domain:              domain,
		DeleteCanonical:     deleteCanonical,
		LineageNames:        stagedLineages,
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
	}, &response)
	if err != nil {
		return fmt.Errorf("remove managed certificate lineages: %w", err)
	}
	if response.Error != "" {
		return fmt.Errorf("remove managed certificate lineages: %s", response.Error)
	}
	return nil
}

// finalizeDomainDeletion removes the domain row. In the same transaction
// (alpha.134) every database still bound to the domain is unbound, so the
// row's ON DELETE CASCADE never removes the panel's record of a database that
// still exists in its engine (a deletion an older release started, or one
// finished by CelikPanel by itself), and the deletion's plan record goes.
func (p *Panel) finalizeDomainDeletion(ctx context.Context, domainID int) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin domain ledger deletion: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE databases_v2 SET domain_id = NULL, updated_at = datetime('now')
		WHERE domain_id = ?
		  AND EXISTS (SELECT 1 FROM domain_deletion_operations op WHERE op.domain_id = ?)`,
		domainID, domainID); err != nil {
		return fmt.Errorf("unbind the domain's databases: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM domains
		WHERE id = ? AND status = ?
		  AND EXISTS (
			SELECT 1
			FROM domain_deletion_operations op
			WHERE op.domain_id = domains.id
		  )`,
		domainID,
		domainDeletionLedgerStatus,
	)
	if err != nil {
		return fmt.Errorf("delete domain ledger row: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm domain ledger deletion: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("delete domain ledger row: expected one pending domain, changed %d", changed)
	}
	if err := clearDomainDeletionPlan(ctx, tx, domainID); err != nil {
		return fmt.Errorf("remove domain deletion plan: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit domain ledger deletion: %w", err)
	}
	// The sender-limit lists follow the removed mailboxes (they already left
	// at the mail step; this is the routine re-check).
	p.requestOutgoingApply()
	return nil
}
