package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Deleting a domain on a joined server (alpha.114 lab run 2, E-new-4).
//
// A joined server keeps its domains' zones on the main DNS server. Deleting
// such a domain publishes the zone's removal there (syncRemoteDomainDNS). On
// a DNS pair the main server removes the zone at once but confirms the
// removal with its backup DNS server first; when that proof is still running
// it answers 409 REMOTE_DNS_PUBLICATION_PENDING and finishes the proof by
// itself (dns_pending_publication.go). Until now nothing on the joined server
// asked again: the exact saved removal stayed unconfirmed here, the deletion
// stayed at dns_cleanup for good (also across reboots) and its status read
// "unknown" although the zone was gone from both DNS servers.
//
// This file closes that loop. A deletion that reached its DNS stage is
// carried to its end by the joined server itself:
//
//  1. The same exact saved removal is sent again (the same generation and
//     payload hash; the main server answers an applied generation with its
//     receipt and never applies it twice), with a growing pause: 15 s, then
//     30 s, 60 s ... up to 5 minutes.
//  2. Once the main server's receipt confirms the removal, the rest of the
//     deletion runs here exactly as the request would have run it: the
//     domain's managed certificates, then its ledger row (the schema refuses
//     that delete without the receipt, migration 042).
//
// Single flight: one attempt per domain at a time, under the domain's own
// operation lock (the same lock a "Retry this deletion" request holds) and
// the origin's publication lock. A status read never starts this work.
// Every state has a typed reason with who acts and what happens next (D-024).
//
// Ana DNS sunucusu silmeyi yedekle doğrularken "bekliyor" yanıtı verirse,
// katılan sunucu aynı kayıtlı silmeyi kendisi yeniden sorar ve onay gelince
// silmeyi kendisi bitirir. Durum okuması hiçbir işi başlatmaz.

// Reasons of a joined server's deletion (each with its own EN/TR text).
const (
	// The main DNS server holds the removal and is confirming it with the
	// backup DNS server. Nobody acts; this server asks again by itself.
	remoteDNSDeletionConfirmingReason = "remote_dns_removal_confirming"
	// The last attempt got no usable answer from the main DNS server. Nobody
	// acts now; if it lasts, the main server's owner checks its panel.
	remoteDNSDeletionUnreachableReason = "remote_dns_main_unreachable"
	// The main DNS server refused the removal (its authorization for this
	// server was withdrawn, or the zone there is in another state). A person
	// acts: the administrator checks the connection, then retries.
	remoteDNSDeletionRefusedReason = "remote_dns_removal_refused"
	// The removal is confirmed; this server finishes the deletion by itself.
	remoteDNSDeletionFinishingReason = "remote_dns_removed_finishing"
)

const (
	remoteDNSDeletionConfirmingEnglish  = "The main DNS server has removed this domain and is confirming the removal with the backup DNS server. Nobody needs to do anything: this server asks again by itself and then finishes the deletion. This page updates by itself."
	remoteDNSDeletionUnreachableEnglish = "This server could not get an answer from the main DNS server about this domain's removal. Nobody needs to do anything now: this server asks again by itself (at least every 5 minutes) and then finishes the deletion. If this lasts more than an hour, the main DNS server's owner checks that its panel is running and reachable from this server."
	remoteDNSDeletionRefusedEnglish     = "The main DNS server refused to remove this domain: its permission for this server was withdrawn, or the domain there is in a different state. The domain stays on this server and nothing else changes. The server administrator opens Server → DNS and peer servers to check the connection to the main DNS server, then deletes the domain again."
	remoteDNSDeletionFinishingEnglish   = "The main DNS server confirmed that this domain is removed from DNS. This server finishes the rest of the deletion by itself; nobody needs to do anything. If this message stays for more than ten minutes, delete the domain again to finish it."
)

// The record that a joined server's deletion reached its DNS stage (its site
// and mail were removed in that request). It is kept with the deletion marker
// and removed with the domain. Without it, a top-level domain whose saved
// publication is its zone's removal counts as reached too: only a deletion
// writes that publication.
const remoteDNSDeletionStageKeyPrefix = "domain_deletion_remote_dns:"

func remoteDNSDeletionStageKey(domainID int) string {
	return remoteDNSDeletionStageKeyPrefix + strconv.Itoa(domainID)
}

func (p *Panel) recordRemoteDNSDeletionStage(ctx context.Context, domainID int, domain string) error {
	_, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO panel_settings (key, value, updated_at)
		SELECT ?, ?, CURRENT_TIMESTAMP
		WHERE EXISTS (SELECT 1 FROM domain_deletion_operations WHERE domain_id = ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		remoteDNSDeletionStageKey(domainID), strings.ToLower(strings.TrimSpace(domain)), domainID)
	if err != nil {
		return fmt.Errorf("record the deletion's DNS stage: %w", err)
	}
	return nil
}

func (p *Panel) clearRemoteDNSDeletionStage(ctx context.Context, domainID int) {
	if _, err := p.db.GetDB().ExecContext(ctx,
		`DELETE FROM panel_settings WHERE key = ?`, remoteDNSDeletionStageKey(domainID)); err != nil {
		log.Printf("domain deletion %d: clear its DNS stage record: %v", domainID, err)
	}
}

type remoteDNSDeletionQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// remoteDomainDeletionDNS is what the saved records say about a joined
// server's deletion. Read only.
type remoteDomainDeletionDNS struct {
	// Reached: the deletion reached its DNS stage.
	Reached bool
	// Pending: a saved publication of the zone waits for the main server's receipt.
	Pending bool
	// Done: the main server's receipt confirms the removal.
	Done bool
}

func readRemoteDomainDeletionDNS(ctx context.Context, q remoteDNSDeletionQuery, domainID int, domain string) (remoteDomainDeletionDNS, error) {
	var state remoteDomainDeletionDNS
	target, err := resolveRemoteDNSDomain(ctx, q, domain)
	if err != nil {
		// No consistent connection to the main DNS server is recorded for
		// this domain: the DNS stage cannot be continued by itself.
		return state, nil
	}
	var recordedName string
	err = q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, remoteDNSDeletionStageKey(domainID)).Scan(&recordedName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	recorded := err == nil && recordedName == strings.ToLower(strings.TrimSpace(domain))
	var generation, applied int64
	var deleted bool
	err = q.QueryRowContext(ctx, `SELECT generation, applied_generation, deleted FROM remote_dns_zones WHERE domain_id = ?`, target.ID).Scan(&generation, &applied, &deleted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	hasZone := err == nil
	apex := target.ID == domainID
	state.Reached = recorded || (apex && hasZone && deleted)
	if !state.Reached {
		return state, nil
	}
	if hasZone && generation != applied {
		state.Pending = true
		return state, nil
	}
	if apex {
		// Nothing ever published (no zone row) has nothing to remove.
		state.Done = !hasZone || deleted
		return state, nil
	}
	records, err := readRemoteDNSRecords(ctx, q, target.ID)
	if err != nil {
		return state, err
	}
	state.Done = true
	for _, record := range records {
		if remoteDNSNameWithin(record.Name, domain) {
			state.Done = false
			break
		}
	}
	return state, nil
}

// remoteDNSDeletionReason classifies an attempt to publish the removal on the
// main DNS server. ok is false for an error that is not about that exchange.
func remoteDNSDeletionReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var refusal *remoteDNSHTTPError
	if errors.As(err, &refusal) {
		switch {
		case refusal.StatusCode == http.StatusConflict &&
			(refusal.Code == "REMOTE_DNS_PUBLICATION_PENDING" || refusal.Code == "REMOTE_DNS_AUTHORITY_NOT_READY"):
			return remoteDNSDeletionConfirmingReason, true
		case refusal.StatusCode == http.StatusUnauthorized || refusal.StatusCode == http.StatusForbidden ||
			refusal.StatusCode == http.StatusBadRequest ||
			(refusal.StatusCode == http.StatusConflict && refusal.Code == "REMOTE_DNS_PUBLICATION_CONFLICT"):
			return remoteDNSDeletionRefusedReason, true
		default:
			// 5xx, busy, or an answer this server does not know: no verdict.
			return remoteDNSDeletionUnreachableReason, true
		}
	}
	var exchange *remoteDNSExchangeError
	if errors.As(err, &exchange) {
		if exchange.Delivered {
			// The main DNS server got the removal and has not answered in
			// time (its check with its backup can take longer): it is
			// running, so this is a wait, not "unreachable" (lab run 6).
			return remoteDNSDeletionConfirmingReason, true
		}
		return remoteDNSDeletionUnreachableReason, true
	}
	if errors.Is(err, errRemoteDNSPublicationPending) || errors.Is(err, errRemoteDNSNotReady) {
		// No answer (network, timeout), or the connection is not usable now.
		return remoteDNSDeletionUnreachableReason, true
	}
	return "", false
}

func remoteDNSDeletionEnglish(reason string) string {
	switch reason {
	case remoteDNSDeletionUnreachableReason:
		return remoteDNSDeletionUnreachableEnglish
	case remoteDNSDeletionRefusedReason:
		return remoteDNSDeletionRefusedEnglish
	case remoteDNSDeletionFinishingReason:
		return remoteDNSDeletionFinishingEnglish
	default:
		return remoteDNSDeletionConfirmingEnglish
	}
}

// Variables only so tests can shorten them.
var (
	remoteDNSDeletionFirstDelay   = 15 * time.Second
	remoteDNSDeletionMaxDelay     = 5 * time.Minute
	remoteDNSDeletionScanInterval = 10 * time.Second
	remoteDNSDeletionAttemptLimit = 45 * time.Second
)

type remoteDNSDeletionEntry struct {
	Attempts int
	NextAt   time.Time
	// Reason of the last attempt; "" before the first one.
	Reason string
}

type remoteDNSDeletionScheduler struct {
	mu      sync.Mutex
	entries map[int]*remoteDNSDeletionEntry
	now     func() time.Time
	wake    chan struct{}
}

func (p *Panel) remoteDNSDeletions() *remoteDNSDeletionScheduler {
	p.remoteDNSDeletionsOnce.Do(func() {
		if p.remoteDNSDeletionsState == nil {
			p.remoteDNSDeletionsState = &remoteDNSDeletionScheduler{
				entries: map[int]*remoteDNSDeletionEntry{}, now: time.Now, wake: make(chan struct{}, 1),
			}
		}
	})
	return p.remoteDNSDeletionsState
}

// due reports whether the domain's next attempt is due (a domain never tried
// in this process is due at once: a deletion stuck before this start resumes).
func (s *remoteDNSDeletionScheduler) due(domainID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[domainID]
	return entry == nil || !s.now().Before(entry.NextAt)
}

func (s *remoteDNSDeletionScheduler) record(domainID int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[domainID]
	if entry == nil {
		entry = &remoteDNSDeletionEntry{}
		s.entries[domainID] = entry
	}
	delay := remoteDNSDeletionFirstDelay
	for i := 0; i < entry.Attempts && delay < remoteDNSDeletionMaxDelay; i++ {
		delay *= 2
	}
	if delay > remoteDNSDeletionMaxDelay {
		delay = remoteDNSDeletionMaxDelay
	}
	entry.Attempts++
	entry.Reason = reason
	entry.NextAt = s.now().Add(delay)
}

// pendingAnswered: the request itself got this answer; the first re-check
// follows after the first pause.
func (s *remoteDNSDeletionScheduler) pendingAnswered(domainID int, reason string) {
	s.mu.Lock()
	s.entries[domainID] = &remoteDNSDeletionEntry{Reason: reason, NextAt: s.now().Add(remoteDNSDeletionFirstDelay)}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *remoteDNSDeletionScheduler) forget(domainID int) {
	s.mu.Lock()
	delete(s.entries, domainID)
	s.mu.Unlock()
}

// lastReason is a read for the status of a deletion.
func (s *remoteDNSDeletionScheduler) lastReason(domainID int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[domainID]; entry != nil {
		return entry.Reason
	}
	return ""
}

// startRemoteDNSDeletionReconciler runs until ctx ends. It only reads until a
// joined server's deletion that reached its DNS stage is due.
func (p *Panel) startRemoteDNSDeletionReconciler(ctx context.Context) {
	scheduler := p.remoteDNSDeletions()
	go func() {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			case <-scheduler.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			}
			p.reconcileRemoteDNSDeletionsOnce(ctx)
			p.reconcileLocalDNSDeletionsOnce(ctx)
			timer.Reset(remoteDNSDeletionScanInterval)
		}
	}()
}

// reconcileRemoteDNSDeletionsOnce makes one attempt for every due deletion.
func (p *Panel) reconcileRemoteDNSDeletionsOnce(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name
		FROM domains d
		JOIN domain_deletion_operations op ON op.domain_id = d.id
		WHERE d.status = ? AND d.dns_management = ?
		ORDER BY d.id`, domainDeletionLedgerStatus, setupDNSModeExisting)
	if err != nil {
		log.Printf("joined-server deletions: read the waiting deletions: %v", err)
		return
	}
	type candidate struct {
		id   int
		name string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.name); err != nil {
			rows.Close()
			log.Printf("joined-server deletions: read the waiting deletions: %v", err)
			return
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	scheduler := p.remoteDNSDeletions()
	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		if p.deletionsRunning.isRunning(c.id) || !scheduler.due(c.id) {
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, remoteDNSDeletionAttemptLimit)
		p.resumeRemoteDNSDeletion(attemptCtx, c.id, c.name)
		cancel()
	}
}

// reconcileLocalDNSDeletionsOnce finishes the deletions whose own DNS
// removal is complete. On a DNS pair's main server a deletion stops at
// dns_cleanup while the backup DNS server confirms the zone's removal; the
// removal is then confirmed by itself (dns_pending_publication.go), but the
// rest of the deletion waited for "Retry this deletion" (alpha.128 lab: a
// domain deleted with an API token stayed "pending" until someone pressed it
// in the panel, and a token cannot). Such a deletion is finished here, as
// the request would have finished it: certificates, then the ledger row. A
// recorded failure of another stage stays the owner's retry. A status read
// never starts this work.
//
// Ana DNS sunucusunda silmenin DNS adımı yedeğin onayını beklerken durur;
// onay kendiliğinden gelince silmenin kalanı burada bitirilir.
func (p *Panel) reconcileLocalDNSDeletionsOnce(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name
		FROM domains d
		JOIN domain_deletion_operations op ON op.domain_id = d.id
		WHERE d.status = ? AND d.dns_management = ?
		ORDER BY d.id`, domainDeletionLedgerStatus, setupDNSModeLocal)
	if err != nil {
		log.Printf("local DNS deletions: read the waiting deletions: %v", err)
		return
	}
	type candidate struct {
		id   int
		name string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.name); err != nil {
			rows.Close()
			log.Printf("local DNS deletions: read the waiting deletions: %v", err)
			return
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	scheduler := p.remoteDNSDeletions()
	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		if p.deletionsRunning.isRunning(c.id) || !scheduler.due(c.id) {
			continue
		}
		if snapshot, err := p.readDomainDeletionStatusSnapshot(ctx, c.id); err != nil || !snapshot.dnsRemoved {
			continue
		}
		attemptCtx, cancel := context.WithTimeout(ctx, remoteDNSDeletionAttemptLimit)
		p.finishLocalDNSDeletion(attemptCtx, c.id, c.name)
		cancel()
	}
}

// finishLocalDNSDeletion is one attempt, under the domain's operation lock
// (the lock a "Retry this deletion" request holds), re-read under it.
func (p *Panel) finishLocalDNSDeletion(ctx context.Context, domainID int, domain string) {
	unlock := lockDomainSSLOperation(domainID)
	defer unlock()
	scheduler := p.remoteDNSDeletions()
	if p.deletionsRunning.isRunning(domainID) {
		return
	}
	snapshot, err := p.readDomainDeletionStatusSnapshot(ctx, domainID)
	if err != nil || !snapshot.dnsRemoved || snapshot.domain != domain {
		return
	}
	if err := p.finishRemoteDNSDeletion(ctx, domainID, domain); err != nil {
		scheduler.record(domainID, remoteDNSDeletionFinishingReason)
		log.Printf("deletion of %s: the DNS removal is confirmed; finishing the deletion did not succeed yet: %v; tried again by itself", domain, err)
		return
	}
	scheduler.forget(domainID)
	log.Printf("deletion of %s: finished by itself after the backup DNS server confirmed the zone's removal", domain)
}

// resumeRemoteDNSDeletion is one attempt for one deletion: the exact removal
// is confirmed on the main DNS server, then the deletion is finished here.
func (p *Panel) resumeRemoteDNSDeletion(ctx context.Context, domainID int, domain string) {
	unlock := lockDomainSSLOperation(domainID)
	defer unlock()
	scheduler := p.remoteDNSDeletions()
	// Under the lock: a "Retry this deletion" that held it may have ended
	// the deletion, or recorded a failure that now speaks for it.
	var status, mode string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.status, d.dns_management
		FROM domains d
		JOIN domain_deletion_operations op ON op.domain_id = d.id
		WHERE d.id = ? AND d.name = ?`, domainID, domain).Scan(&status, &mode)
	if errors.Is(err, sql.ErrNoRows) {
		scheduler.forget(domainID)
		return
	}
	if err != nil {
		log.Printf("joined-server deletion of %s: read its marker: %v", domain, err)
		return
	}
	if status != domainDeletionLedgerStatus || mode != setupDNSModeExisting {
		scheduler.forget(domainID)
		return
	}
	tx, err := p.db.GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		log.Printf("joined-server deletion of %s: %v", domain, err)
		return
	}
	_, failed, failureErr := readDomainDeletionFailure(ctx, tx, domainID, domain)
	state, stateErr := readRemoteDomainDeletionDNS(ctx, tx, domainID, domain)
	_ = tx.Rollback()
	if failureErr != nil || stateErr != nil {
		log.Printf("joined-server deletion of %s: read its saved state: %v", domain, errors.Join(failureErr, stateErr))
		return
	}
	if failed || !state.Reached {
		// A recorded failure of an earlier stage, or a deletion that never
		// reached its DNS stage: its retry is the owner's (its status says so).
		scheduler.forget(domainID)
		return
	}
	if !state.Done {
		// The same exact saved removal, sent again; committed first when the
		// request stopped before saving it.
		if err := p.syncRemoteDomainDNS(ctx, domain, true); err != nil && !p.remoteDNSDeletionConfirmed(ctx, domainID, domain) {
			// (The exact re-send can get its receipt while the main server's
			// later readiness probe fails: the receipt alone is the proof.)
			reason, ok := remoteDNSDeletionReason(err)
			if !ok {
				reason = remoteDNSDeletionUnreachableReason
			}
			scheduler.record(domainID, reason)
			log.Printf("joined-server deletion of %s: the main DNS server has not confirmed the removal yet (%s): %v; asked again by itself",
				domain, reason, err)
			return
		}
		log.Printf("joined-server deletion of %s: the main DNS server confirmed the removal", domain)
	}
	if err := p.finishRemoteDNSDeletion(ctx, domainID, domain); err != nil {
		scheduler.record(domainID, remoteDNSDeletionFinishingReason)
		log.Printf("joined-server deletion of %s: the DNS removal is confirmed; finishing the deletion did not succeed yet: %v; tried again by itself", domain, err)
		return
	}
	scheduler.forget(domainID)
	log.Printf("joined-server deletion of %s: finished by itself after the main DNS server confirmed the removal", domain)
}

// remoteDNSDeletionConfirmed re-reads whether the main server's receipt now
// confirms the removal. Read only.
func (p *Panel) remoteDNSDeletionConfirmed(ctx context.Context, domainID int, domain string) bool {
	state, err := readRemoteDomainDeletionDNS(ctx, p.db.GetDB(), domainID, domain)
	return err == nil && state.Reached && state.Done
}

// finishRemoteDNSDeletion runs the stages after DNS, as the request does.
// The caller holds the domain's operation lock.
func (p *Panel) finishRemoteDNSDeletion(ctx context.Context, domainID int, domain string) error {
	deleteCanonical, staged, err := domainDeletionCertificateLineages(ctx, p.db.GetDB(), domainID, domain)
	if err != nil {
		return err
	}
	if deleteCanonical || len(staged) > 0 {
		if err := p.requireMatchingAgentBuild(ctx); err != nil {
			return err
		}
	}
	if err := p.removeManagedDomainCertificates(ctx, domain, deleteCanonical, staged); err != nil {
		return err
	}
	if err := p.finalizeDomainDeletion(ctx, domainID); err != nil {
		return err
	}
	if err := p.clearDomainDeletionFailure(ctx, domainID); err != nil {
		log.Printf("domain deletion for %s completed; %v", domain, err)
	}
	p.clearRemoteDNSDeletionStage(ctx, domainID)
	p.recordAudit(auditRecord{Action: "domain.delete:" + domain, ResourceType: "domain", ResourceID: domainID})
	return nil
}

// domainDeletionCertificateLineages names the managed certificate lineages a
// deletion removes: the domain's canonical lineage, and the staged ones the
// Agent made. Any other lineage name is refused.
func domainDeletionCertificateLineages(ctx context.Context, db *sql.DB, domainID int, domain string) (bool, []string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT lineage_name
		FROM ssl_certificates
		WHERE domain_id = ?
		  AND type = 'letsencrypt'
		  AND lineage_name IS NOT NULL`, domainID)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	var deleteCanonical bool
	var staged []string
	canonical := strings.ToLower(strings.TrimSpace(domain))
	for rows.Next() {
		var lineage string
		if err := rows.Scan(&lineage); err != nil {
			return false, nil, err
		}
		lineage = strings.ToLower(strings.TrimSpace(lineage))
		switch {
		case lineage == canonical:
			deleteCanonical = true
		case strings.HasPrefix(lineage, "cp-site-"), transport.IsNativeSiteLineage(lineage):
			staged = append(staged, lineage)
		default:
			return false, nil, fmt.Errorf("refuse unknown managed certificate lineage %q for domain %s", lineage, domain)
		}
	}
	return deleteCanonical, staged, rows.Err()
}
