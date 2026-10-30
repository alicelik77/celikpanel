package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Website certificates renewed by the operating system (D-022, D-039).
//
// A certificate issued by this release is a certbot lineage in the standard
// store; certbot's own timer renews it and its deploy hook reloads nginx,
// with no CelikPanel program involved. The Panel never renews it: its
// scheduler only reads what the operating system did and records it, so the
// certificate page and the Dashboard tell the administrator when renewal
// needs them (D-024). Certificates of earlier releases (immutable snapshots)
// stay renewed by cert_renewal.go until they are reissued.
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

// retireSupersededNativeLineages switches certbot's renewal off for native
// lineages that only replaced (revoked or expired) ledger rows name. Their
// rows stay as rollback records, so the lineage stays too, but the
// operating system must not keep renewing a certificate nothing serves.
// Idempotent: an already-off or already-removed lineage changes nothing.
func (p *Panel) retireSupersededNativeLineages(ctx context.Context) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT DISTINCT old.domain_id, old.lineage_name
		FROM ssl_certificates old
		WHERE old.status <> 'active' AND old.type = 'letsencrypt' AND old.lineage_name LIKE ?
		  AND NOT EXISTS (
			SELECT 1 FROM ssl_certificates cur
			WHERE cur.status = 'active' AND cur.lineage_name = old.lineage_name)`,
		transport.NativeSiteLineagePrefix+"%")
	if err != nil {
		log.Printf("cert renewal: read replaced OS-renewed certificates: %v", err)
		return
	}
	type retired struct {
		domainID int
		lineage  string
	}
	var list []retired
	for rows.Next() {
		var item retired
		if err := rows.Scan(&item.domainID, &item.lineage); err != nil {
			rows.Close()
			log.Printf("cert renewal: read replaced OS-renewed certificates: %v", err)
			return
		}
		if transport.NativeSiteLineageBelongsTo(item.lineage, item.domainID) {
			list = append(list, item)
		}
	}
	rows.Close()
	for _, item := range list {
		if err := p.setNativeSiteAutoRenew(ctx, item.domainID, item.lineage, false); err != nil {
			log.Printf("cert renewal: stop certbot renewing the replaced lineage %s: %v", item.lineage, err)
		}
	}
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
		// The mail SNI map names immutable copies; a renewed certificate is
		// copied and published to Postfix and Dovecot here. While the Panel
		// is stopped mail keeps the previous copy (D-039, open item).
		if err := p.syncCertificateDependents(ctx, row.DomainID); err != nil {
			if markErr := p.markCertificatePendingDetached(ctx, row.DomainID, sslPendingDependents, false); markErr != nil {
				log.Printf("cert renewal %s: mail TLS after the operating system's renewal: %v; pending state: %v", row.Domain, err, markErr)
				return
			}
			log.Printf("cert renewal %s: mail TLS after the operating system's renewal failed (%v); Retry activation publishes it", row.Domain, err)
		}
	}
}

// snapshotNativeSiteCertificateForMail returns the immutable copy of the
// certificate a native lineage serves now, for the mail SNI map.
func (p *Panel) snapshotNativeSiteCertificateForMail(ctx context.Context, domainID int, domain, lineage string) (string, string, error) {
	var resp transport.SnapshotSiteCertificateResponse
	if err := p.callAgentContext(ctx, "Agent.SnapshotSiteCertificateV1", &transport.SnapshotSiteCertificateRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Domain:              domain, DomainID: domainID, LineageName: lineage,
	}, &resp); err != nil {
		return "", "", err
	}
	if resp.Error != "" {
		return "", "", errors.New(resp.Error)
	}
	if resp.CertPath == "" || resp.KeyPath == "" {
		return "", "", fmt.Errorf("the Agent returned no copy of the certificate for mail")
	}
	return resp.CertPath, resp.KeyPath, nil
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
