package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// legacyMoveAgent is the validation-restore fake whose issuance answers as
// the D-040 Agent does: a fresh native lineage of the domain.
type legacyMoveAgent struct {
	*validationRestoreAgent
	moveFailure string // "" issues; otherwise the certbot failure kind
	moveCalls   []ValidationRestoreIssueRequest
}

func (a *legacyMoveAgent) IssueLetsEncryptCertificate(req *ValidationRestoreIssueRequest, resp *transport.IssueLetsEncryptResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.moveCalls = append(a.moveCalls, *req)
	lineage := fmt.Sprintf("celikpanel-site-%d-%024x", req.DomainID, 7)
	if a.moveFailure != "" {
		*resp = transport.IssueLetsEncryptResponse{LineageName: lineage, Failure: a.moveFailure,
			FailureDetail: "Some challenges have failed.", Error: "certbot failed"}
		return nil
	}
	full, key, chain := transport.NativeSiteLineagePaths(lineage)
	*resp = transport.IssueLetsEncryptResponse{Success: true, CertPath: full, KeyPath: key, ChainPath: chain,
		ExpiresAt: time.Date(2027, time.March, 1, 0, 0, 0, 0, time.UTC), LineageName: lineage}
	return nil
}

func newLegacyMoveFixture(t *testing.T, provider, issuer string, expires time.Time, moveFailure string) (*Panel, int, int, *legacyMoveAgent) {
	t.Helper()
	p, domainID, certificateID64 := newSSLStateFixture(t)
	if _, err := p.db.GetDB().Exec(`
		UPDATE ssl_certificates
		SET type = 'letsencrypt', lineage_name = 'ssl-state.example', acme_provider_id = ?,
		    issuer = ?, auto_renew = true, secure_mail = false, expires_at = ?
		WHERE id = ?`, provider, issuer, expires.UTC().Format(time.RFC3339), certificateID64); err != nil {
		t.Fatal(err)
	}
	if _, err := p.db.GetDB().Exec(`
		UPDATE sites SET ssl_type = 'letsencrypt', force_https = false, hsts_enabled = false, hsts_retire_after = NULL
		WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	agent := &legacyMoveAgent{
		validationRestoreAgent: &validationRestoreAgent{domain: "ssl-state.example", renewMode: "exact"},
		moveFailure:            moveFailure,
	}
	attachValidationRestoreAgentAs(t, p, agent)
	return p, domainID, int(certificateID64), agent
}

type legacyMoveRow struct {
	id                        int
	certPath, lineage, status string
	renewalStatus, provider   string
}

func activeLegacyMoveRow(t *testing.T, p *Panel, domainID int) legacyMoveRow {
	t.Helper()
	var row legacyMoveRow
	if err := p.db.GetDB().QueryRow(`
		SELECT id, cert_path, COALESCE(lineage_name, ''), status, COALESCE(renewal_status, ''), COALESCE(acme_provider_id, '')
		FROM ssl_certificates WHERE domain_id = ? AND status = 'active'`, domainID).
		Scan(&row.id, &row.certPath, &row.lineage, &row.status, &row.renewalStatus, &row.provider); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestADueLegacyCertificateMovesToTheOperatingSystemAtItsRenewal(t *testing.T) {
	p, domainID, oldID, agent := newLegacyMoveFixture(t, "letsencrypt", "Let's Encrypt", time.Now().Add(20*24*time.Hour), "")
	p.runDueCertRenewals()

	agent.mu.Lock()
	moves, renews := append([]ValidationRestoreIssueRequest(nil), agent.moveCalls...), agent.renewCalls
	applies := append([]ValidationRestoreApplyRequest(nil), agent.applyCalls...)
	agent.mu.Unlock()
	if len(moves) != 1 || !moves[0].FreshLineage || !moves[0].StageLineage || !moves[0].ForceRenewal ||
		moves[0].CurrentLineageName != "ssl-state.example" || renews != 0 {
		t.Fatalf("moves=%+v renews=%d", moves, renews)
	}
	row := activeLegacyMoveRow(t, p, domainID)
	lineage := fmt.Sprintf("celikpanel-site-%d-%024x", domainID, 7)
	full, _, _ := transport.NativeSiteLineagePaths(lineage)
	if row.id == oldID || row.lineage != lineage || row.certPath != full || row.renewalStatus != "renewed" || row.provider != "letsencrypt" {
		t.Fatalf("active row after the move = %+v", row)
	}
	assertCertificateStatus(t, p, int64(oldID), "revoked")
	if len(applies) == 0 || applies[len(applies)-1].SSLCert != full {
		t.Fatalf("the vhost was not rendered with the native live files last: %+v", applies)
	}
	// Idempotent: the moved certificate is native and never moved again.
	p.runDueCertRenewals()
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.moveCalls) != 1 || agent.renewCalls != 0 {
		t.Fatalf("second run moved or renewed again: moves=%d renews=%d", len(agent.moveCalls), agent.renewCalls)
	}
}

func TestAFailedMoveKeepsTheCertificateAndRetriesUntilTheFallbackWindow(t *testing.T) {
	p, domainID, oldID, agent := newLegacyMoveFixture(t, "letsencrypt", "Let's Encrypt",
		time.Now().Add(25*24*time.Hour), transport.CertificateFailureValidation)
	p.runDueCertRenewals()
	row := activeLegacyMoveRow(t, p, domainID)
	agent.mu.Lock()
	moves, renews, deletes := len(agent.moveCalls), agent.renewCalls, append([]ValidationRestoreDeleteRequest(nil), agent.deleteCalls...)
	agent.mu.Unlock()
	if row.id != oldID || row.renewalStatus != "move_failed_validation" || moves != 1 || renews != 0 {
		t.Fatalf("row=%+v moves=%d renews=%d", row, moves, renews)
	}
	lineage := fmt.Sprintf("celikpanel-site-%d-%024x", domainID, 7)
	if len(deletes) != 1 || len(deletes[0].LineageNames) != 1 || deletes[0].LineageNames[0] != lineage || deletes[0].DeleteCanonical {
		t.Fatalf("the failed move's own lineage was not the only one removed: %+v", deletes)
	}
}

func TestAMoveThatKeepsFailingNearExpiryFallsBackToTheEarlierRenewal(t *testing.T) {
	p, domainID, _, agent := newLegacyMoveFixture(t, "letsencrypt", "Let's Encrypt",
		time.Now().Add(10*24*time.Hour), transport.CertificateFailureRateLimited)
	p.runDueCertRenewals()
	row := activeLegacyMoveRow(t, p, domainID)
	agent.mu.Lock()
	moves, renews := len(agent.moveCalls), agent.renewCalls
	agent.mu.Unlock()
	if moves != 1 || renews != 1 {
		t.Fatalf("moves=%d renews=%d", moves, renews)
	}
	// Renewed the earlier way, and the card still says why it is not moved.
	if row.certPath != "/certs/renewed/fullchain.pem" || row.renewalStatus != "move_failed_rate_limited" {
		t.Fatalf("row = %+v", row)
	}
}

func TestACertificateFromAnEABAuthorityIsRenewedTheEarlierWay(t *testing.T) {
	p, domainID, _, agent := newLegacyMoveFixture(t, "zerossl", "ZeroSSL", time.Now().Add(20*24*time.Hour), "")
	p.runDueCertRenewals()
	agent.mu.Lock()
	moves, renews := len(agent.moveCalls), agent.renewCalls
	agent.mu.Unlock()
	if moves != 0 || renews != 1 {
		t.Fatalf("moves=%d renews=%d", moves, renews)
	}
	if row := activeLegacyMoveRow(t, p, domainID); row.renewalStatus != "renewed" {
		t.Fatalf("row = %+v", row)
	}
}

func TestLegacyMoveModeAndFallbackWindow(t *testing.T) {
	for _, c := range []struct{ provider, issuer, want string }{
		{"letsencrypt", "", legacyMoveAutomatic},
		{"", "Let's Encrypt", legacyMoveAutomatic},
		{"", "", legacyMoveAutomatic},
		{"zerossl", "ZeroSSL", legacyMoveReissue},
		{"google", "", legacyMoveReissue},
		{"unknown-ca", "", legacyMoveReissue},
	} {
		if got := legacyMoveMode(c.provider, c.issuer); got != c.want {
			t.Fatalf("legacyMoveMode(%q, %q) = %q, want %q", c.provider, c.issuer, got, c.want)
		}
	}
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	if legacyRenewalFallbackDue(now.Add(15*24*time.Hour).Format(time.RFC3339), now) ||
		!legacyRenewalFallbackDue(now.Add(14*24*time.Hour).Format(time.RFC3339), now) ||
		!legacyRenewalFallbackDue("not a time", now) {
		t.Fatal("fallback window")
	}
	if legacyMoveFailedStatus("validation") != "move_failed_validation" || legacyMoveFailedStatus("x") != "move_failed_other" {
		t.Fatal("failed status")
	}
}
