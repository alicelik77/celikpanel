package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// Installing the certificate issued for a certificate request (alpha.128,
// G31 follow-up).
//
// The administrator pastes or uploads the certificate the authority issued
// (and its chain) and picks the site it is for. Before anything changes the
// panel proves that the certificate was issued for this request's key (the
// SHA-256 of its public key is the one recorded when the request was made),
// that it is valid now and that it covers every name the request asked
// for. The site's ordinary custom-certificate path then runs with real
// steps (validate, install, reload, mail): the Agent reads the request's
// private key from its own key file (Agent.ValidateCertificate and
// Agent.InstallCustomCertificate with RequestKeyToken), so the key never
// passes through the panel or the browser. Mail for the site's domain
// follows the site's certificate as for an uploaded one.
//
// The panel's own certificate and the mail server's host certificate are
// renewed by Let's Encrypt with their own lineages; a bought certificate is
// not installed there (it would be overwritten at renewal or stop renewals
// silently). The answer says so and names the site path instead.
//
// İstek için verilen sertifikanın kurulması: önce sertifikanın bu isteğin
// anahtarı için verildiği, şu an geçerli olduğu ve istenen bütün adları
// kapsadığı kanıtlanır; sonra sitenin olağan özel sertifika yolu adım adım
// çalışır. Özel anahtarı Agent kendi dosyasından okur; panelden geçmez.

const (
	certificateRequestInstallMaxBody   = 256 << 10
	errCodeCertificateRequestMismatch  = "CERTIFICATE_REQUEST_KEY_MISMATCH"
	errCodeCertificateRequestExpired   = "CERTIFICATE_REQUEST_CERT_NOT_VALID_NOW"
	errCodeCertificateRequestNames     = "CERTIFICATE_REQUEST_NAMES_MISSING"
	errCodeCertificateRequestTarget    = "CERTIFICATE_REQUEST_TARGET_UNSUPPORTED"
	errCodeCertificateRequestInstalled = "CERTIFICATE_REQUEST_ALREADY_INSTALLED"
)

// requestIdentityRouteCertificateRequestInstall: the install shows its real
// steps when the page sends an identity (operation_step_routes.go).
var requestIdentityRouteCertificateRequestInstall = requestIdentityRoute{
	pattern: certificateRequestsPath + "/{id}/install", timeout: operationStepRouteTimeout, optional: true,
	maxBody: certificateRequestInstallMaxBody,
}

var certificateRequestInstallNow = time.Now

type certificateRequestInstallBody struct {
	Target      string `json:"target"`
	DomainID    int    `json:"domain_id"`
	Certificate string `json:"certificate"`
	Chain       string `json:"chain"`
}

// splitIssuedCertificate takes the issued certificate and any chain the
// authority sent in the same file: the first CERTIFICATE block is the
// certificate, the others join the chain. Anything but certificates is
// refused (a private key is never accepted here).
func splitIssuedCertificate(certificateText, chainText string) (*x509.Certificate, string, string, error) {
	var blocks []*pem.Block
	for _, text := range []string{certificateText, chainText} {
		rest := []byte(strings.TrimSpace(text))
		for len(bytes.TrimSpace(rest)) > 0 {
			block, next := pem.Decode(rest)
			if block == nil {
				return nil, "", "", errors.New("not PEM")
			}
			if block.Type != "CERTIFICATE" {
				return nil, "", "", errors.New("not a certificate: " + block.Type)
			}
			blocks = append(blocks, block)
			rest = next
		}
	}
	if len(blocks) == 0 {
		return nil, "", "", errors.New("no certificate")
	}
	leaf, err := x509.ParseCertificate(blocks[0].Bytes)
	if err != nil {
		return nil, "", "", err
	}
	var chain bytes.Buffer
	for _, block := range blocks[1:] {
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, "", "", err
		}
		_ = pem.Encode(&chain, block)
	}
	return leaf, string(pem.EncodeToMemory(blocks[0])), chain.String(), nil
}

func issuedPublicKeySHA256(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

// missingRequestedNames: requested names the certificate does not cover.
func missingRequestedNames(cert *x509.Certificate, requested []string) []string {
	var missing []string
	for _, name := range requested {
		covered := false
		for _, have := range cert.DNSNames {
			if strings.EqualFold(have, name) {
				covered = true
				break
			}
		}
		if !covered && !strings.HasPrefix(name, "*.") {
			covered = certificateCoversHostname(cert.DNSNames, name)
		}
		if !covered {
			missing = append(missing, name)
		}
	}
	return missing
}

// handleCertificateRequestInstall: POST
// /api/v1/maintenance/certificate-requests/{id}/install
// {"target": "site", "domain_id", "certificate", "chain"}.
func (p *Panel) handleCertificateRequestInstall(w http.ResponseWriter, r *http.Request, id int) {
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	if caller := currentCaller(r); caller != nil && caller.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeSessionImpersonating,
			"A certificate cannot be installed while you are logged in as another account. Return to your own account first. Nothing was changed.", "")
		return
	}
	var body certificateRequestInstallBody
	decoder := json.NewDecoder(io.LimitReader(r.Body, certificateRequestInstallMaxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid,
			"The certificate could not be read from the request, so nothing was changed. Paste the certificate (PEM, beginning with -----BEGIN CERTIFICATE-----) and try again.", "")
		return
	}
	switch body.Target {
	case "site":
	case "panel", "mail":
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestTarget,
			"The panel's own certificate and the mail server's host certificate are renewed automatically by Let's Encrypt, so a bought certificate is not installed there: it would be replaced at the next renewal. Nothing was changed. Install it on the site it was bought for; mail for that domain then uses it too.", "")
		return
	default:
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid, "Choose the site the certificate is for. Nothing was changed.", "")
		return
	}
	ctx := r.Context()
	var keyFile, state, fingerprint, commonName, altNamesJSON string
	var installedFor sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT key_file, state, public_key_sha256, common_name, alt_names, installed_for
		FROM certificate_requests WHERE id = ?`, id).Scan(&keyFile, &state, &fingerprint, &commonName, &altNamesJSON, &installedFor)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state == "discarded") {
		writeCodedError(w, http.StatusNotFound, errCodeCertificateRequestNotFound,
			"This certificate request is not waiting any more (it was discarded, and its key with it). Make a new request; nothing was changed.", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state == "installed" {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestInstalled,
			"This request's certificate is already installed ("+installedFor.String+"). To renew a bought certificate, make a new request. Nothing was changed.", "")
		return
	}
	leaf, leafPEM, chainPEM, err := splitIssuedCertificate(body.Certificate, body.Chain)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid,
			"This is not a certificate the panel can read (only PEM certificates are accepted here, never a private key). Nothing was changed. Paste the certificate the authority sent, beginning with -----BEGIN CERTIFICATE-----.", "")
		return
	}
	if !strings.EqualFold(issuedPublicKeySHA256(leaf), fingerprint) {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestMismatch,
			"This certificate was not issued for this request: its public key is not the one this server made for "+commonName+". Nothing was changed. Check that you pasted the certificate of this order, or ask the authority to reissue it with this request's CSR.", "")
		return
	}
	now := certificateRequestInstallNow()
	if now.Before(leaf.NotBefore) {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestExpired,
			"This certificate is valid only from "+leaf.NotBefore.UTC().Format("2006-01-02 15:04 MST")+". Nothing was changed. Install it after that time (check the server clock under Maintenance if that time has passed).", "")
		return
	}
	if !now.Before(leaf.NotAfter) {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestExpired,
			"This certificate expired on "+leaf.NotAfter.UTC().Format("2006-01-02")+", so visitors would see a warning. Nothing was changed. Ask the authority for a new one (make a new request).", "")
		return
	}
	var requested []string
	if err := json.Unmarshal([]byte(altNamesJSON), &requested); err != nil {
		writeServerError(w, err)
		return
	}
	if len(requested) == 0 {
		requested = []string{commonName}
	}
	if missing := missingRequestedNames(leaf, requested); len(missing) > 0 {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestNames,
			"This certificate does not cover every name the request asked for (missing: "+strings.Join(missing, ", ")+"). Nothing was changed. Ask the authority to issue it for all the names of the request.", "")
		return
	}
	if body.DomainID <= 0 {
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid, "Choose the site the certificate is for. Nothing was changed.", "")
		return
	}
	var domainName string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT name FROM domains WHERE id = ?`, body.DomainID).Scan(&domainName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeCodedError(w, http.StatusNotFound, errCodeCertificateRequestInvalid, "That site does not exist any more. Reload the page and choose again; nothing was changed.", "")
			return
		}
		writeServerError(w, err)
		return
	}
	// The site's own path: validation with the Agent (chain, dates, names,
	// key), the managed host names, the replacement guard, installation,
	// activation, the web server and mail, with their steps and audit.
	activated := p.installCustomCertificateForDomain(w, r, body.DomainID, customCertificateMaterial{
		cert: leafPEM, chain: chainPEM,
		requestToken: certificateRequestTokenOf(keyFile), requestKeySHA256: fingerprint,
	})
	if !activated {
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE certificate_requests SET state = 'installed', installed_at = ?, installed_for = ?
		WHERE id = ? AND state = 'waiting'`,
		now.UTC().Format("2006-01-02 15:04:05"), "site:"+domainName, id); err != nil {
		// The certificate is active (the answer said so); only the request's
		// record lags. The site's installed files hold their own copy of the
		// key, so even a later Discard of this request cannot break the site.
		return
	}
	p.auditDetailed(r, "certificate.request.install", "certificate_request", id, map[string]any{
		"site": domainName, "expires": leaf.NotAfter.UTC().Format("2006-01-02"),
	})
}
