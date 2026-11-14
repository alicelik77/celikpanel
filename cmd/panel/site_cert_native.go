package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Website certificates renewed by the operating system (D-022, D-040).
//
// A certificate issued by this release is a certbot lineage in the standard
// store; certbot's own timer renews it and its deploy hook reloads nginx,
// with no CelikPanel program involved. The Panel never renews it: its
// scheduler only reads what the operating system did and records it, so the
// certificate page and the Dashboard tell the administrator when renewal
// needs them (D-024). Certificates of earlier releases (immutable snapshots)
// are moved to a native lineage at their next renewal
// (site_cert_legacy_move.go) and renewed by cert_renewal.go until then.
//
// İşletim sisteminin yenilediği web sitesi sertifikaları: Panel yenilemez,
// yalnız işletim sisteminin yaptığını okur ve kaydeder.

// renewal_status values of an OS-renewed certificate are "os_" + the
// transport.SiteRenewal* state.
const nativeRenewalStatusPrefix = "os_"

func nativeRenewalStatus(state string) string {
	return nativeRenewalStatusPrefix + state
}

// nativeSiteLineageOfRow is the native lineage a ledger row names, when its
// certificate path is that lineage's live full chain.
func nativeSiteLineageOfRow(certPath, lineageName string) (string, bool) {
	lineage, ok := transport.NativeSiteLineageFromPath(strings.TrimSpace(certPath))
	if !ok || lineage != strings.TrimSpace(lineageName) {
		return "", false
	}
	return lineage, true
}

// renewalStatusIsOutbox reports a renewal_status that an activation or the
// owner's file decision owns; observation never overwrites it.
func renewalStatusIsOutbox(status string) bool {
	switch status {
	case sslPendingActivation, sslPendingDependents, sslWaitingForOwner:
		return true
	}
	return false
}

type nativeSiteRow struct {
	CertID, DomainID, SubscriptionID int
	Domain, CertPath, Lineage        string
	ProviderID, RenewalStatus        string
	ExpiresAt                        string
	SecureMail                       bool
}

// readNativeSiteRenewal asks the Agent what the operating system did for one
// native lineage. Read only.
func (p *Panel) readNativeSiteRenewal(ctx context.Context, row nativeSiteRow) (transport.SiteCertificateRenewalStateResponse, error) {
	server := ""
	if provider := core.ACMEProviderByID(strings.TrimSpace(row.ProviderID)); provider != nil {
		server = provider.Directory
	}
	var resp transport.SiteCertificateRenewalStateResponse
	err := p.callAgentContext(ctx, "Agent.SiteCertificateRenewalStateV1", &transport.SiteCertificateRenewalStateRequest{
		Domain: row.Domain, DomainID: row.DomainID, SubscriptionID: row.SubscriptionID,
		LineageName: row.Lineage, ACMEServer: server,
	}, &resp)
	if err != nil {
		return resp, err
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func (p *Panel) nativeSiteRows(ctx context.Context) ([]nativeSiteRow, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT sc.id, sc.domain_id, d.subscription_id, d.name, sc.cert_path,
		       COALESCE(sc.lineage_name, ''), COALESCE(sc.acme_provider_id, ''),
		       COALESCE(sc.renewal_status, ''), sc.expires_at, COALESCE(sc.secure_mail, false)
		FROM ssl_certificates sc JOIN domains d ON d.id = sc.domain_id
		WHERE sc.status = 'active' AND sc.type = 'letsencrypt'
		ORDER BY sc.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []nativeSiteRow
	for rows.Next() {
		var row nativeSiteRow
		if err := rows.Scan(&row.CertID, &row.DomainID, &row.SubscriptionID, &row.Domain, &row.CertPath,
			&row.Lineage, &row.ProviderID, &row.RenewalStatus, &row.ExpiresAt, &row.SecureMail); err != nil {
			return nil, err
		}
		if _, ok := nativeSiteLineageOfRow(row.CertPath, row.Lineage); ok {
			out = append(out, row)
		}
	}
	return out, rows.Err()
}

// observeNativeSiteCertificates records the operating system's renewal of
// every active native certificate. It never runs certbot.
func (p *Panel) observeNativeSiteCertificates(ctx context.Context) {
	rows, err := p.nativeSiteRows(ctx)
	if err != nil {
		log.Printf("cert renewal: read OS-renewed certificates: %v", err)
		return
	}
	for _, row := range rows {
		jobCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		p.observeNativeSiteCertificate(jobCtx, row)
		cancel()
	}
	p.retireSupersededNativeLineages(ctx)
}

// replacedNativeLineagesQuery selects the native lineages that only replaced
// (revoked or expired) ledger rows name.
const replacedNativeLineagesQuery = `
	SELECT DISTINCT old.domain_id, old.lineage_name, COALESCE(old.acme_provider_id, '')
	FROM ssl_certificates old
	WHERE old.status <> 'active' AND old.type = 'letsencrypt' AND old.lineage_name LIKE ?
	  AND NOT EXISTS (
		SELECT 1 FROM ssl_certificates cur
		WHERE cur.status = 'active' AND cur.lineage_name = old.lineage_name)
	  AND NOT EXISTS (
		SELECT 1 FROM sites s
		WHERE instr(COALESCE(s.ssl_cert_path, '') || ' ' || COALESCE(s.ssl_key_path, ''), '/' || old.lineage_name || '/') > 0)`

// retireSupersededNativeLineages is the periodic pass of
// retireReplacedNativeLineages: every domain with a replaced native lineage,
// each under its own certificate lock. What could not be removed is asked
// again at the next pass (alpha.134).
func (p *Panel) retireSupersededNativeLineages(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, replacedNativeLineagesQuery, transport.NativeSiteLineagePrefix+"%")
	if err != nil {
		log.Printf("cert renewal: read replaced OS-renewed certificates: %v", err)
		return
	}
	var domains []int
	seen := map[int]bool{}
	for rows.Next() {
		var domainID int
		var lineage, provider string
		if err := rows.Scan(&domainID, &lineage, &provider); err != nil {
			rows.Close()
			log.Printf("cert renewal: read replaced OS-renewed certificates: %v", err)
			return
		}
		if !seen[domainID] && transport.NativeSiteLineageBelongsTo(lineage, domainID) {
			seen[domainID] = true
			domains = append(domains, domainID)
		}
	}
	rows.Close()
	for _, domainID := range domains {
		unlock := lockDomainSSLOperation(domainID)
		_, _ = p.retireReplacedNativeLineages(ctx, domainID)
		unlock()
	}
}

// retireReplacedNativeLineages asks the Agent to remove the native lineages
// of one domain that a newer certificate replaced (or a detach left), with
// certbot's own `certbot delete`, once nothing names them
// (RetireSiteCertificateLineagesV1 checks the files on the server). The
// caller holds the domain's certificate lock. Nothing is asked while the
// active certificate waits for its activation, its mail step or the owner's
// file: until it is served the previous one may still be. Their ledger rows
// stay as the record they are; no row is ever made active again.
//
// It answers each lineage's outcome (nil: nothing to do). Every outcome other
// than retired or absent is logged and asked again at the next pass.
func (p *Panel) retireReplacedNativeLineages(ctx context.Context, domainID int) ([]transport.RetireSiteLineageResult, error) {
	pool := p.db.GetDB()
	var subscriptionID int
	var domainName string
	if err := pool.QueryRowContext(ctx, `SELECT subscription_id, name FROM domains WHERE id = ?`, domainID).Scan(&subscriptionID, &domainName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var activeLineage, activePath, activeStatus string
	var activeCount int
	if err := pool.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(lineage_name), ''), COALESCE(MAX(cert_path), ''), COALESCE(MAX(renewal_status), '')
		FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).
		Scan(&activeCount, &activeLineage, &activePath, &activeStatus); err != nil {
		return nil, err
	}
	if activeCount > 1 || renewalStatusIsOutbox(activeStatus) {
		return nil, nil
	}
	request := transport.RetireSiteCertificateLineagesRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		DomainID:            domainID,
		SubscriptionID:      subscriptionID,
	}
	if lineage, ok := nativeSiteLineageOfRow(activePath, activeLineage); ok {
		request.ActiveLineage = lineage
	}
	rows, err := pool.QueryContext(ctx, replacedNativeLineagesQuery+` AND old.domain_id = ?`,
		transport.NativeSiteLineagePrefix+"%", domainID)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var id int
		var lineage, providerID string
		if err := rows.Scan(&id, &lineage, &providerID); err != nil {
			rows.Close()
			return nil, err
		}
		if seen[lineage] || lineage == request.ActiveLineage || !transport.NativeSiteLineageBelongsTo(lineage, domainID) {
			continue
		}
		seen[lineage] = true
		server := ""
		if provider := core.ACMEProviderByID(strings.TrimSpace(providerID)); provider != nil {
			server = provider.Directory
		}
		request.Lineages = append(request.Lineages, transport.RetireSiteLineage{LineageName: lineage, ACMEServer: server})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(request.Lineages) == 0 {
		return nil, nil
	}
	if len(request.Lineages) > 100 {
		request.Lineages = request.Lineages[:100]
	}
	// Removing is a change of the server that cannot be taken back: it
	// waits, as the database-rights rewrite does, until the latest update to
	// this build is verified, the start-up quiet time has passed and the
	// server is not busy (the one helper, databaseGrantFormsHold); an Agent
	// of another build refuses it itself.
	if hold := p.databaseGrantFormsHold(ctx, p.processStartedAt()); hold != "" {
		log.Printf("cert renewal domain %d: replaced certificate lineages wait (%s); asked again in the hourly round once that gate opens", domainID, hold)
		nativeLineageRetireWaiting.Store(true)
		return nil, &lineageRetireHeld{reason: hold}
	}
	var resp transport.RetireSiteCertificateLineagesResponse
	if err := p.callAgentContext(ctx, "Agent.RetireSiteCertificateLineagesV1", &request, &resp); err != nil {
		log.Printf("cert renewal domain %d: replaced certificate lineages not removed now (the Agent was not asked or did not answer: %v); asked again at the next pass", domainID, err)
		return nil, err
	}
	if resp.Error != "" {
		log.Printf("cert renewal domain %d: replaced certificate lineages not removed now (%s); asked again at the next pass", domainID, resp.Error)
		return nil, &nativeSiteAgentRefusal{detail: resp.Error}
	}
	for _, result := range resp.Results {
		p.auditLineageOutcome(ctx, domainID, domainName, result)
		switch result.Outcome {
		case transport.SiteLineageAbsent:
		case transport.SiteLineageRetired:
			note := ""
			if result.Detail != "" {
				note = " (" + result.Detail + ")"
			}
			log.Printf("cert renewal domain %d: removed the replaced certificate lineage %s with certbot; nothing renews it any more%s", domainID, result.LineageName, note)
		default:
			log.Printf("cert renewal domain %d: kept the replaced certificate lineage %s (%s): %s; asked again at the next pass", domainID, result.LineageName, result.Outcome, result.Detail)
		}
	}
	return resp.Results, nil
}

// nativeObservation is what one reading changes in the ledger.
type nativeObservation struct {
	Renewed   bool
	Status    string // "" keeps the recorded renewal_status
	ExpiresAt time.Time
	IssuedAt  time.Time
	AttemptAt time.Time
}

// decideNativeObservation is the pure rule: a later expiry than the ledger's
// is a renewal the operating system made; unknown records nothing; an outbox
// status (activation, mail, the owner's file) is never overwritten; a
// renewal stays "renewed" while the schedule is healthy.
func decideNativeObservation(row nativeSiteRow, state transport.SiteCertificateRenewalStateResponse) nativeObservation {
	var out nativeObservation
	recorded, ok := parseCertTime(row.ExpiresAt)
	if !state.NotAfter.IsZero() && (!ok || state.NotAfter.After(recorded.Add(time.Minute))) {
		out.Renewed = true
		out.ExpiresAt = state.NotAfter
		out.IssuedAt = state.NotBefore
		out.AttemptAt = state.NotBefore
		if state.LastDeploy != nil && state.LastDeploy.CompletedAt.After(state.NotBefore) {
			out.AttemptAt = state.LastDeploy.CompletedAt
		}
	}
	if state.State == transport.SiteRenewalUnknown || renewalStatusIsOutbox(row.RenewalStatus) {
		return out
	}
	status := nativeRenewalStatus(state.State)
	if state.State == transport.SiteRenewalScheduled &&
		(out.Renewed || row.RenewalStatus == nativeRenewalStatus(transport.SiteRenewalRenewed)) {
		status = nativeRenewalStatus(transport.SiteRenewalRenewed)
	}
	if state.State == transport.SiteRenewalFailed && state.LastFailed != nil {
		out.AttemptAt = state.LastFailed.At
	}
	if status != row.RenewalStatus {
		out.Status = status
	}
	return out
}

func (p *Panel) observeNativeSiteCertificate(ctx context.Context, row nativeSiteRow) {
	unlock := lockDomainSSLOperation(row.DomainID)
	defer unlock()
	state, err := p.readNativeSiteRenewal(ctx, row)
	if err != nil {
		// Unknown is not failed: nothing is recorded, the next run asks again.
		log.Printf("cert renewal %s: the operating system's renewal state could not be read: %v", row.Domain, err)
		return
	}
	decision := decideNativeObservation(row, state)
	if decision.Renewed {
		if _, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE ssl_certificates
			SET expires_at = ?, issued_at = ?, last_renewal_attempt = ?, updated_at = datetime('now')
			WHERE id = ? AND status = 'active'`,
			decision.ExpiresAt.UTC().Format(time.RFC3339), decision.IssuedAt.UTC().Format(time.RFC3339),
			decision.AttemptAt.UTC().Format(time.RFC3339), row.CertID,
		); err != nil {
			log.Printf("cert renewal %s: record the operating system's renewal: %v", row.Domain, err)
			return
		}
		log.Printf("cert renewal %s: renewed by the operating system's certbot until %s", row.Domain,
			decision.ExpiresAt.UTC().Format("2006-01-02"))
	}
	if decision.Status != "" {
		attempt := any(nil)
		if !decision.AttemptAt.IsZero() {
			attempt = decision.AttemptAt.UTC().Format(time.RFC3339)
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE ssl_certificates
			SET renewal_status = ?, last_renewal_attempt = COALESCE(?, last_renewal_attempt), updated_at = datetime('now')
			WHERE id = ? AND status = 'active'`, decision.Status, attempt, row.CertID); err != nil {
			log.Printf("cert renewal %s: record renewal state: %v", row.Domain, err)
		}
		if transport.SiteRenewalStateIsProblem(state.State) {
			log.Printf("cert renewal %s: needs the server administrator (%s): %s", row.Domain, state.State, state.Detail)
		}
	}
	if decision.Renewed && row.SecureMail {
		// The native deploy hook already rebuilt the SNI map and reloaded
		// Postfix and Dovecot; this only reconciles the accepted mail TLS
		// plan (an unchanged plan is a no-op) and the TLSA dependent.
		if err := p.syncCertificateDependents(ctx, row.DomainID); err != nil {
			if markErr := p.markCertificatePendingDetached(ctx, row.DomainID, sslPendingDependents, false); markErr != nil {
				log.Printf("cert renewal %s: mail TLS after the operating system's renewal: %v; pending state: %v", row.Domain, err, markErr)
				return
			}
			log.Printf("cert renewal %s: mail TLS after the operating system's renewal failed (%v); Retry activation publishes it", row.Domain, err)
		}
	}
}

// nativeLineageRetireWaiting: a removal was held by the update gate. The
// hourly round of the database-rights rewrite, which waits behind the same
// gate, asks the pass again once the gate opens (retireWaitingNativeLineages),
// so the replaced certificates go within the hour after an update is
// verified instead of at the next 12-hour pass (alpha.134 lab run 6).
var nativeLineageRetireWaiting atomic.Bool

// retireWaitingNativeLineages runs the replaced-lineage pass once when a
// held one waits and the gate is open now. Bounded: at most once per hourly
// round, only while something waits; a pass held again sets the flag again.
func (p *Panel) retireWaitingNativeLineages(ctx context.Context, startedAt time.Time) {
	if !nativeLineageRetireWaiting.Load() || p.databaseGrantFormsHold(ctx, startedAt) != "" {
		return
	}
	if !nativeLineageRetireWaiting.CompareAndSwap(true, false) {
		return
	}
	p.retireSupersededNativeLineages(ctx)
}

// lineageRetireHeld: the removal waits for a verified update, the start-up
// quiet time or a quiet server; nothing was asked.
type lineageRetireHeld struct{ reason string }

func (e *lineageRetireHeld) Error() string { return "waiting: " + e.reason }

// lineageOutcomeAudited is the last outcome written to the audit log per
// lineage in this process, so a kept lineage is recorded once per change of
// outcome, not at every pass.
var lineageOutcomeAudited sync.Map

// auditLineageOutcome writes one audit row (Denetim günlüğü) when a replaced
// lineage was removed or its kept outcome changed: the lineage, the domain
// and the reason in the action's words; the file that names it for in_use.
// After a restart the latest row of the lineage is read back first.
func (p *Panel) auditLineageOutcome(ctx context.Context, domainID int, domainName string, result transport.RetireSiteLineageResult) {
	var action string
	details := map[string]any{"lineage": result.LineageName}
	switch result.Outcome {
	case transport.SiteLineageRetired:
		action = "ssl.lineage.removed:" + domainName
	case transport.SiteLineageInUse:
		action = "ssl.lineage.kept_in_use:" + domainName
		details["file"] = result.Detail
	case transport.SiteLineageOwnerChanged:
		action = "ssl.lineage.kept_owner_changed:" + domainName
	case transport.SiteLineageHeldBySnapshot:
		action = "ssl.lineage.kept_for_rollback:" + domainName
	case transport.SiteLineageFailed, transport.SiteLineageNotChecked:
		action = "ssl.lineage.not_removed:" + domainName
	default:
		return
	}
	file, _ := details["file"].(string)
	signature := action + "\x00" + file
	if last, ok := lineageOutcomeAudited.Load(result.LineageName); ok && last == signature {
		return
	}
	var lastAction, lastDetails string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT action, COALESCE(details, '') FROM audit_logs
		WHERE resource_type = 'domain' AND resource_id = ? AND action LIKE 'ssl.lineage.%'
		  AND instr(COALESCE(details, ''), ?) > 0
		ORDER BY id DESC LIMIT 1`, domainID, `"lineage":"`+result.LineageName+`"`).Scan(&lastAction, &lastDetails)
	if err == nil {
		var recorded map[string]any
		_ = json.Unmarshal([]byte(lastDetails), &recorded)
		recordedFile, _ := recorded["file"].(string)
		if lastAction+"\x00"+recordedFile == signature {
			lineageOutcomeAudited.Store(result.LineageName, signature)
			return
		}
	}
	p.recordAudit(auditRecord{Action: action, ResourceType: "domain", ResourceID: domainID, Details: auditDetailsText(details)})
	lineageOutcomeAudited.Store(result.LineageName, signature)
}

// retireAfterIssue removes the lineages the certificate just served replaced,
// right after it is served (the periodic pass repeats what is left). The
// answer names what was kept where an administrator can act or should know
// (a file names it, its renewal file is the owner's, it could not be removed
// or checked), for an administrator only. nil when everything went, nothing
// was there, it waits for an update's rollback copy or the update gate, or
// the reader is not an administrator.
func (p *Panel) retireAfterIssue(ctx context.Context, caller *Caller, domainID int) map[string]string {
	results, err := p.retireReplacedNativeLineages(ctx, domainID)
	if caller == nil || caller.Role != roleAdmin {
		return nil
	}
	var held *lineageRetireHeld
	if errors.As(err, &held) {
		// Waiting for a verified update, the start-up quiet time or a quiet
		// server is ordinary and ends by itself: nothing to say.
		return nil
	}
	if err != nil {
		return map[string]string{"outcome": transport.SiteLineageNotChecked}
	}
	for _, result := range results {
		switch result.Outcome {
		case transport.SiteLineageRetired, transport.SiteLineageAbsent, transport.SiteLineageHeldBySnapshot:
			// Kept for an update's rollback copy is ordinary operation and
			// ends by itself when the copy is pruned: the audit row and the
			// journal say it, nobody is asked to act.
			continue
		}
		return map[string]string{"outcome": result.Outcome, "lineage": result.LineageName, "detail": result.Detail}
	}
	return nil
}

// setNativeSiteAutoRenew writes the page's "renew automatically" into
// certbot's own configuration of the lineage.
func (p *Panel) setNativeSiteAutoRenew(ctx context.Context, domainID int, lineage string, enabled bool) error {
	var resp transport.SetSiteCertificateAutoRenewResponse
	if err := p.callAgentContext(ctx, "Agent.SetSiteCertificateAutoRenewV1", &transport.SetSiteCertificateAutoRenewRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		DomainID:            domainID, LineageName: lineage, AutoRenew: enabled,
	}, &resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return &nativeSiteAgentRefusal{detail: resp.Error}
	}
	return nil
}

// nativeSiteAgentRefusal is the Agent's own answer (it was reached and
// refused), as distinct from a call that did not reach it.
type nativeSiteAgentRefusal struct{ detail string }

func (e *nativeSiteAgentRefusal) Error() string { return e.detail }
