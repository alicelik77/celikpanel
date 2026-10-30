package main

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Moving a certificate of an earlier release to the operating system's
// renewal (D-040 amendment, D-022, D-024).
//
// A website certificate issued before D-040 lives in CelikPanel's isolated
// certbot store and is renewed only by the running Panel, so it stops
// renewing when the Panel is stopped or removed. At its next renewal (the
// scheduler's 30-day window) the Panel replaces it with a native lineage in
// /etc/letsencrypt instead of renewing it in place: the same names, a fresh
// certificate, certbot's own timer and deploy hook from then on. This is the
// Reissue the certificate card offers, run by the scheduler:
//
//   - no downtime: the earlier certificate is served until the new one is in
//     the ledger and the site's vhost names it; applyVhostForDomain renders
//     and tests (nginx -t) before the reload, and a failure keeps the vhost
//     that serves the earlier certificate and records activation_pending
//     with its Retry action (the existing renewal outbox);
//   - idempotent: a moved certificate is native and never selected again; a
//     move that did not finish leaves the legacy row active and is simply
//     tried again at the next run;
//   - recoverable: a native lineage issued but never recorded (the Panel
//     stopped in between) is removed by the startup lineage reconcile, which
//     keeps only lineages the ledger names; one recorded but not yet served
//     is activation_pending and resumes like any renewal;
//   - bounded: a failed move is retried every scheduler run (12 hours); from
//     legacyMoveFallbackWindow before expiry the earlier certificate is
//     renewed the earlier way instead, so a site never goes without one.
//
// Certificates from a certificate authority that requires External Account
// Binding (ZeroSSL, Google Trust Services) are not moved by themselves: a
// new account in the standard store needs those credentials, which
// CelikPanel never keeps. Their card says that one Reissue with the
// credentials moves them; until then the running Panel renews them.
//
// Önceki sürümlerin sertifikası bir sonraki yenilemesinde işletim sisteminin
// certbot'una taşınır; eski sertifika yenisi etkinleşene kadar sunulur.

// legacyMoveFallbackWindow: from this long before expiry a certificate whose
// move keeps failing is renewed the earlier way (while the Panel runs).
const legacyMoveFallbackWindow = 14 * 24 * time.Hour

// renewal_status of a legacy certificate whose move failed:
// "move_failed_<kind>", kind one of the transport.CertificateFailure* kinds
// or "other". Additive values; the row stays active and keeps serving.
const legacyMoveFailedPrefix = "move_failed_"

// legacy_move on the certificate card (additive JSON).
const (
	legacyMoveAutomatic = "automatic"
	legacyMoveReissue   = "reissue"
)

type legacyMoveOutcome int

const (
	// legacyMoveNotEligible: this certificate is not moved by itself (its
	// authority needs EAB credentials); the earlier renewal applies.
	legacyMoveNotEligible legacyMoveOutcome = iota
	// legacyMoveDone: a native certificate is now the active ledger row
	// (served, or pending a Retry the existing outbox owns).
	legacyMoveDone
	// legacyMoveFailed: nothing changed; the earlier certificate serves.
	legacyMoveFailed
	// legacyMoveNotStarted: not attempted this run (unknown validation
	// state, Agent mismatch); nothing changed and nothing was concluded.
	legacyMoveNotStarted
)

// legacyMoveMode says whether a legacy ACME certificate of this provider is
// moved automatically.
func legacyMoveMode(providerID, issuer string) string {
	id := strings.TrimSpace(providerID)
	if id == "" {
		id = acmeProviderIDForIssuer(issuer)
	}
	if id == "" {
		// Rows of the first releases carry neither: they were Let's
		// Encrypt, the only authority those releases used.
		id = "letsencrypt"
	}
	provider := core.ACMEProviderByID(id)
	if provider == nil || provider.NeedsEAB {
		return legacyMoveReissue
	}
	return legacyMoveAutomatic
}

func legacyMoveFailedStatus(kind string) string {
	switch kind {
	case transport.CertificateFailureAuthorityUnreachable, transport.CertificateFailureValidation,
		transport.CertificateFailureRateLimited, transport.CertificateFailureTimeout,
		transport.CertificateFailureTool:
		return legacyMoveFailedPrefix + kind
	}
	return legacyMoveFailedPrefix + "other"
}

// legacyRenewalFallbackDue is true once a certificate whose move failed is
// close enough to expiry to be renewed the earlier way.
func legacyRenewalFallbackDue(expiresAt string, now time.Time) bool {
	expiry, ok := parseCertTime(expiresAt)
	return !ok || !expiry.After(now.Add(legacyMoveFallbackWindow))
}

// legacyMoveContactEmail is the ACME account contact for a move that no
// person started: the first administrator's address when it is valid, else
// none (certbot then registers without one, or uses the account the
// standard store already has).
func (p *Panel) legacyMoveContactEmail(ctx context.Context) string {
	var stored string
	if err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT COALESCE(email, '') FROM users WHERE role = 'admin' ORDER BY id LIMIT 1`).Scan(&stored); err != nil {
		return ""
	}
	email, err := mutationpayload.CanonicalContactEmail(strings.TrimSpace(stored))
	if err != nil {
		return ""
	}
	return email
}

// moveLegacySiteCertificate replaces one due legacy certificate with a
// native lineage. See the file comment for the guarantees.
func (p *Panel) moveLegacySiteCertificate(ctx context.Context, certID, domainID int, domainName string) legacyMoveOutcome {
	unlock := lockDomainSSLOperation(domainID)
	defer unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	if err := p.requireMatchingAgentBuild(ctx); err != nil {
		log.Printf("cert move %s: not started: paired agent upgrade is incomplete: %v", domainName, err)
		return legacyMoveNotStarted
	}
	var current struct {
		CertPath, KeyPath, LineageName string
		ProviderID, Issuer             string
		AutoRenew, SecureMail          bool
		SubscriptionID                 int
	}
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT c.cert_path, c.key_path, COALESCE(c.lineage_name, ''),
		       COALESCE(c.acme_provider_id, ''), COALESCE(c.issuer, ''),
		       COALESCE(c.auto_renew, true), COALESCE(c.secure_mail, false),
		       d.subscription_id
		FROM ssl_certificates c JOIN domains d ON d.id = c.domain_id
		WHERE c.id = ? AND c.domain_id = ? AND c.status = 'active' AND c.type = 'letsencrypt'`,
		certID, domainID,
	).Scan(&current.CertPath, &current.KeyPath, &current.LineageName,
		&current.ProviderID, &current.Issuer, &current.AutoRenew, &current.SecureMail,
		&current.SubscriptionID); err != nil {
		log.Printf("cert move %s: active certificate changed before the move: %v", domainName, err)
		return legacyMoveNotStarted
	}
	if _, native := nativeSiteLineageOfRow(current.CertPath, current.LineageName); native {
		return legacyMoveDone
	}
	if legacyMoveMode(current.ProviderID, current.Issuer) != legacyMoveAutomatic {
		return legacyMoveNotEligible
	}
	providerID := strings.TrimSpace(current.ProviderID)
	if providerID == "" {
		providerID = acmeProviderIDForIssuer(current.Issuer)
	}
	if providerID == "" {
		providerID = "letsencrypt"
	}
	provider := core.ACMEProviderByID(providerID)

	failed := func(kind, detail string) legacyMoveOutcome {
		status := legacyMoveFailedStatus(kind)
		log.Printf("cert move %s: the certificate stays as it is and keeps serving; moving it to the operating system's renewal failed (%s): %s",
			domainName, status, boundedAgentDiagnostic(detail))
		writeCtx, cancel := sslCompensationContext()
		defer cancel()
		if _, err := p.db.GetDB().ExecContext(writeCtx, `
			UPDATE ssl_certificates
			SET last_renewal_attempt = ?, renewal_status = ?, updated_at = datetime('now')
			WHERE id = ? AND status = 'active'`, now, status, certID); err != nil {
			log.Printf("cert move %s: record the failure: %v", domainName, err)
		}
		return legacyMoveFailed
	}

	currentInfo, err := p.inspectInstalledCertificate(ctx, current.CertPath, current.KeyPath)
	if err != nil {
		return failed("", "inspect the current certificate: "+err.Error())
	}
	mailName, err := mailCertificateHostname(domainName)
	if err != nil {
		return failed("", "derive the mail certificate hostname: "+err.Error())
	}
	managedNames, err := p.managedSiteHostnames(ctx, domainID)
	if err != nil {
		return failed("", err.Error())
	}
	// The same names as the certificate it replaces: the site's names and,
	// when it covered it, the mail name.
	preserveMail := certificateCoversHostname(currentInfo.DNSNames, mailName)
	aliases := append([]string(nil), managedNames[1:]...)
	var challengeNames []string
	if preserveMail {
		aliases = append(aliases, mailName)
		challengeNames = []string{mailName}
	}

	keptSiteConfig, err := p.prepareCertificateValidation(ctx, domainID, challengeNames)
	if err != nil {
		var held *siteFileHeldError
		var validation *certificateValidationHeldError
		var unknown *certificateValidationUnknownError
		if errors.As(err, &unknown) {
			log.Printf("cert move %s: not started: %v", domainName, err)
			return legacyMoveNotStarted
		}
		if errors.As(err, &validation) || errors.As(err, &held) {
			// The owner's file stops validation: the same wait the earlier
			// renewal would record; that renewal records it.
			log.Printf("cert move %s: not started: the site's configuration file stops the validation: %v", domainName, err)
			return legacyMoveNotStarted
		}
		return failed("", "prepare the validation vhost: "+err.Error())
	}
	validationVhostPrepared := !keptSiteConfig
	defer func() {
		if !validationVhostPrepared {
			return
		}
		restoreCtx, restoreCancel := sslCompensationContext()
		defer restoreCancel()
		if err := p.applyVhostForDomain(restoreCtx, domainID); err != nil {
			log.Printf("cert move %s: restore the validation vhost: %v", domainName, err)
		}
	}()

	var resp transport.IssueLetsEncryptResponse
	err = p.callAgentContext(ctx, "Agent.IssueLetsEncryptCertificate", transport.IssueLetsEncryptRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Domain:              domainName,
		Aliases:             aliases,
		Email:               p.legacyMoveContactEmail(ctx),
		SubscriptionID:      current.SubscriptionID,
		DomainID:            domainID,
		AutoRenew:           current.AutoRenew,
		ForceRenewal:        true,
		StageLineage:        true,
		FreshLineage:        true,
		CurrentCertPath:     current.CertPath,
		CurrentLineageName:  current.LineageName,
		ACMEServer:          provider.Directory,
	}, &resp)
	cleanup := func() {
		cleanupCtx, cleanupCancel := sslCompensationContext()
		defer cleanupCancel()
		p.cleanupUncommittedCertificate(cleanupCtx, certificateCleanupTarget{
			Domain: domainName, LineageName: resp.LineageName,
			CertPath: resp.CertPath, KeyPath: resp.KeyPath, ChainPath: resp.ChainPath,
		})
	}
	if err != nil || !resp.Success {
		// Only a lineage the Agent named as this request's own is removed.
		if transport.IsNativeSiteLineage(strings.TrimSpace(resp.LineageName)) {
			cleanup()
		}
		if err == nil && resp.Error == transport.SiteCertificateBusy {
			log.Printf("cert move %s: not started: another certificate operation is running", domainName)
			return legacyMoveNotStarted
		}
		if err != nil {
			// No answer of its own: whether certbot issued is not known
			// (D-024). Nothing is recorded; a lineage it may have left is
			// named by no ledger row and the startup reconcile removes it.
			log.Printf("cert move %s: result not known (%v); the certificate stays as it is, the next run tries again", domainName, err)
			return legacyMoveNotStarted
		}
		detail := resp.Error
		if resp.FailureDetail != "" {
			detail = resp.FailureDetail
		}
		return failed(resp.Failure, detail)
	}
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	lineage := strings.TrimSpace(resp.LineageName)
	if !transport.IsNativeSiteLineage(lineage) {
		return failed("", "the Agent issued a certificate outside the operating system's store")
	}
	if err := validateIssuedCertificateLineage(domainName, domainID, true, lineage); err != nil {
		return failed("", err.Error())
	}
	info, err := p.inspectIssuedCertificate(ctx, domainName, resp.CertPath, resp.KeyPath, resp.ChainPath)
	if err != nil {
		return failed("", "read the new certificate: "+err.Error())
	}
	if !info.Valid || !info.TrustChecked || !info.Trusted {
		detail := strings.TrimSpace(info.TrustError)
		if detail == "" {
			detail = strings.TrimSpace(info.Error)
		}
		if detail == "" {
			detail = "the new certificate could not be verified"
		}
		return failed("", detail)
	}
	expectedNames := append([]string(nil), managedNames...)
	if preserveMail {
		expectedNames = append(expectedNames, mailName)
	}
	if err := exactCertificateDNSNames(info.DNSNames, expectedNames); err != nil {
		return failed("", "the new certificate's names: "+err.Error())
	}
	expiresAt := info.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = resp.ExpiresAt
	}
	if expiresAt.IsZero() {
		return failed("", "the new certificate has no expiry")
	}
	subject := strings.TrimSpace(info.Subject)
	if subject == "" {
		subject = domainName
	}
	activation, err := p.activateCertificate(ctx, domainID, certificateInstall{
		DomainName:     domainName,
		Type:           "letsencrypt",
		CertPath:       resp.CertPath,
		KeyPath:        resp.KeyPath,
		ChainPath:      resp.ChainPath,
		LineageName:    lineage,
		ACMEProviderID: provider.ID,
		Issuer:         provider.Name,
		Subject:        subject,
		IssuedAt:       info.IssuedAt,
		ExpiresAt:      expiresAt,
		AutoRenew:      current.AutoRenew,
		SecureMail:     current.SecureMail && preserveMail,
	})
	if err != nil {
		return failed("", "record the new certificate: "+err.Error())
	}
	// From here the ledger names the native certificate; the earlier row is
	// its rollback record. Every later failure is the existing outbox's.
	keep = true
	validationVhostPrepared = false
	if warning := strings.TrimSpace(resp.RenewalWarning); warning != "" {
		log.Printf("cert move %s: automatic renewal needs the server administrator: %s", domainName, warning)
	}
	if err := p.applyVhostForDomain(ctx, domainID); err != nil {
		if _, kept := keptSiteFile(err); !kept {
			markErr := p.persistCertificateRenewalPending(domainID, activation.NewCertID, now, sslPendingActivation, true)
			log.Printf("cert move %s: the web server did not take the new certificate (%v); it is recorded for Retry activation: %v", domainName, err, markErr)
			return legacyMoveDone
		}
		if holdErr := p.holdCertificateForOwner(ctx, domainID, now); holdErr != nil {
			log.Printf("cert move %s: record the new certificate waiting for the owner: %v", domainName, holdErr)
		}
		log.Printf("cert move %s: moved; the site's configuration file is the owner's and does not use the new certificate yet", domainName)
		return legacyMoveDone
	}
	if err := p.syncCertificateDependents(ctx, domainID); err != nil {
		markErr := p.persistCertificateRenewalPending(domainID, activation.NewCertID, now, sslPendingDependents, false)
		log.Printf("cert move %s: mail TLS did not take the new certificate (%v); Retry activation publishes it: %v", domainName, err, markErr)
		return legacyMoveDone
	}
	if err := p.completeCertificateRenewal(ctx, activation.NewCertID, now); err != nil {
		log.Printf("cert move %s: record success: %v", domainName, err)
	}
	log.Printf("cert move %s: moved to the operating system's certbot (lineage %s) until %s; CelikPanel no longer renews it",
		domainName, lineage, expiresAt.UTC().Format("2006-01-02"))
	return legacyMoveDone
}

// keepLegacyMoveFailure re-records a move failure after the earlier renewal
// that followed it succeeded, so the card keeps saying the certificate is
// still renewed only while CelikPanel runs and why (D-024: a known failure
// is not erased by a later, different success).
func (p *Panel) keepLegacyMoveFailure(domainID int, status string) {
	ctx, cancel := sslCompensationContext()
	defer cancel()
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE ssl_certificates SET renewal_status = ?, updated_at = datetime('now')
		WHERE domain_id = ? AND status = 'active' AND type = 'letsencrypt'
		  AND COALESCE(renewal_status, '') IN ('renewed', 'current')`, status, domainID); err != nil {
		log.Printf("cert move: keep the move failure of domain %d: %v", domainID, err)
	}
}

func (p *Panel) activeRenewalStatus(ctx context.Context, certID int) string {
	var status string
	_ = p.db.GetDB().QueryRowContext(ctx,
		`SELECT COALESCE(renewal_status, '') FROM ssl_certificates WHERE id = ?`, certID).Scan(&status)
	return status
}
