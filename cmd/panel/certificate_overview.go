package main

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Server certificates overview and certificate requests (alpha.128, G31).
//
//   - GET  /api/v1/maintenance/certificates  one read-only list of every
//     certificate this server uses, with what uses it, who issued it, how
//     many days are left and whether it renews by itself: the panel's own
//     (read from its file), the default HTTPS server and the mail host
//     (Agent reads kept for ten minutes, so a page load never waits on
//     nginx -T), and every site's (the stored ledger). The certificate
//     requests that wait for their issued certificate come with it.
//   - POST /api/v1/maintenance/certificate-requests  makes a request (CSR)
//     for a certificate bought elsewhere: the Agent makes the key, keeps it
//     root-only and answers only the CSR (Agent.CertificateRequestCreateV1).
//   - POST /api/v1/maintenance/certificate-requests/{id}/discard  removes
//     the key and marks the request discarded.
//
// Renewal of Let's Encrypt certificates stays automatic and has no switch
// here. Installing an issued certificate with a request's key is not in this
// release (docs/CERTIFICATE-REQUESTS.md); the key file is the owner's and is
// usable with the operating system's own tools (D-022).
//
// Sunucu sertifikaları: tek salt-okunur liste ve ücretli sertifika için CSR.
// Özel anahtar Agent'ta kalır; tarayıcıya ve panele hiç gelmez.

const (
	certificatesOverviewPath   = "/api/v1/maintenance/certificates"
	certificateRequestsPath    = "/api/v1/maintenance/certificate-requests"
	certificateAgentReadBound  = 15 * time.Second
	certificateAgentCacheFor   = 10 * time.Minute
	certificateRequestsWaiting = 50

	errCodeCertificateRequestInvalid  = "CERTIFICATE_REQUEST_INVALID"
	errCodeCertificateRequestFailed   = "CERTIFICATE_REQUEST_FAILED"
	errCodeCertificateRequestUnknown  = "CERTIFICATE_REQUEST_UNKNOWN"
	errCodeCertificateRequestNotFound = "CERTIFICATE_REQUEST_NOT_FOUND"
	errCodeCertificateRequestTooMany  = "CERTIFICATE_REQUEST_TOO_MANY"
)

// certificateView is one certificate in the overview.
type certificateView struct {
	// Kind: panel, default_https, mail_host or site.
	Kind  string   `json:"kind"`
	Name  string   `json:"name"`
	Names []string `json:"names,omitempty"`
	// Source: letsencrypt, uploaded, self_signed, external (configured
	// outside CelikPanel), none (no certificate is used) or unknown.
	Source string `json:"source"`
	Issuer string `json:"issuer,omitempty"`
	// State: in_use, not_needed, pending, failed, owner (the owner's own
	// configuration) or unknown.
	State     string `json:"state"`
	ExpiresAt string `json:"expires_at,omitempty"`
	DaysLeft  *int   `json:"days_left,omitempty"`
	// Renewal: automatic, manual, off (automatic renewal turned off) or none.
	Renewal string `json:"renewal"`
	// UsedBy: panel, default_https, mail_host, website, mail.
	UsedBy   []string `json:"used_by"`
	DomainID int      `json:"domain_id,omitempty"`
}

type certificateRequestView struct {
	ID              int      `json:"id"`
	CommonName      string   `json:"common_name"`
	Names           []string `json:"names"`
	KeyType         string   `json:"key_type"`
	CSRPEM          string   `json:"csr_pem"`
	PublicKeySHA256 string   `json:"public_key_sha256"`
	KeyFile         string   `json:"key_file"`
	CreatedAt       string   `json:"created_at"`
}

type certificatesOverview struct {
	Certificates []certificateView        `json:"certificates"`
	Requests     []certificateRequestView `json:"requests"`
	ReadAt       string                   `json:"read_at"`
}

// The two Agent reads, kept for a while (memory only).
type certificateAgentReads struct {
	mu        sync.Mutex
	at        time.Time
	defaultCV certificateView
	mailCV    *certificateView
}

var certificateReadsCache certificateAgentReads

func daysLeft(expires, now time.Time) *int {
	if expires.IsZero() {
		return nil
	}
	days := int(math.Floor(expires.Sub(now).Hours() / 24))
	return &days
}

func certificateSourceOf(cert *x509.Certificate) string {
	if cert.Issuer.String() == cert.Subject.String() {
		return "self_signed"
	}
	for _, org := range cert.Issuer.Organization {
		if strings.Contains(strings.ToLower(org), "let's encrypt") {
			return "letsencrypt"
		}
	}
	return "uploaded"
}

func panelCertificateView(now time.Time) certificateView {
	view := certificateView{Kind: "panel", Name: "CelikPanel", UsedBy: []string{"panel"}, State: "in_use"}
	cert, ok := currentPanelLeaf()
	if !ok {
		view.Source, view.State, view.Renewal = "unknown", "unknown", "none"
		return view
	}
	view.Names = cert.DNSNames
	if len(view.Names) > 0 {
		view.Name = view.Names[0]
	}
	view.Issuer = cert.Issuer.CommonName
	view.ExpiresAt = cert.NotAfter.UTC().Format(time.RFC3339)
	view.DaysLeft = daysLeft(cert.NotAfter, now)
	view.Source = certificateSourceOf(cert)
	switch {
	case strings.TrimSpace(os.Getenv("CELIKPANEL_TLS_CERT")) != "":
		view.Source, view.Renewal = "external", "manual"
	case view.Source == "letsencrypt":
		view.Renewal = "automatic"
	case view.Source == "self_signed":
		view.Renewal = "none"
	default:
		view.Renewal = "manual"
	}
	return view
}

var readDefaultHTTPSForOverview = func(p *Panel, ctx context.Context) (core.NginxDefaultHTTPS, error) {
	callCtx, cancel := context.WithTimeout(ctx, certificateAgentReadBound)
	defer cancel()
	var result core.NginxInspectResult
	err := p.callAgentContext(callCtx, "Agent.NginxInspect", &transport.Empty{}, &result)
	return result.DefaultHTTPS, err
}

var readMailHostCertificateForOverview = func(p *Panel, ctx context.Context, name string) (transport.MailHostCertificateStatusResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, certificateAgentReadBound)
	defer cancel()
	var result transport.MailHostCertificateStatusResponse
	err := p.callAgentContext(callCtx, "Agent.MailHostCertificateStatus", &transport.MailHostCertificateStatusRequest{Domain: name}, &result)
	return result, err
}

func defaultHTTPSView(status core.NginxDefaultHTTPS, err error) certificateView {
	view := certificateView{Kind: "default_https", Name: "default", UsedBy: []string{"default_https"}, Renewal: "none", Source: "none"}
	if err != nil {
		view.State, view.Source = "unknown", "unknown"
		return view
	}
	switch status.State {
	case core.NginxDefaultHTTPSActive:
		view.State = "in_use"
		if status.Mode == "placeholder" {
			view.Source = "self_signed"
		}
	case core.NginxDefaultHTTPSOwnerDefault, core.NginxDefaultHTTPSOwnerFile:
		view.State, view.Source = "owner", "external"
	case core.NginxDefaultHTTPSNotNeeded:
		view.State = "not_needed"
	case core.NginxDefaultHTTPSPending:
		view.State = "pending"
	case core.NginxDefaultHTTPSFailed:
		view.State = "failed"
	default:
		view.State, view.Source = "unknown", "unknown"
	}
	return view
}

func mailHostView(name string, status transport.MailHostCertificateStatusResponse, err error, now time.Time) certificateView {
	view := certificateView{Kind: "mail_host", Name: name, Names: []string{name}, UsedBy: []string{"mail_host"}}
	switch {
	case err != nil:
		view.State, view.Source, view.Renewal = "unknown", "unknown", "none"
	case !status.Ready:
		view.State, view.Source, view.Renewal = "pending", "none", "none"
	default:
		view.State, view.Source = "in_use", "letsencrypt"
		view.ExpiresAt = status.ExpiresAt.UTC().Format(time.RFC3339)
		view.DaysLeft = daysLeft(status.ExpiresAt, now)
		view.Renewal = "manual"
		if status.RenewalReady {
			view.Renewal = "automatic"
		}
	}
	return view
}

// agentCertificateViews: the default HTTPS server and the mail host, read
// at most once per ten minutes.
func (p *Panel) agentCertificateViews(ctx context.Context, now time.Time) []certificateView {
	cache := &certificateReadsCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.at.IsZero() || now.Sub(cache.at) >= certificateAgentCacheFor {
		status, err := readDefaultHTTPSForOverview(p, ctx)
		cache.defaultCV = defaultHTTPSView(status, err)
		cache.mailCV = nil
		if name := strings.TrimSpace(p.setting(ctx, settingMailHostname)); name != "" {
			if canonical, nameErr := hostname.CanonicalFQDN(name); nameErr == nil {
				mail, mailErr := readMailHostCertificateForOverview(p, ctx, canonical)
				view := mailHostView(canonical, mail, mailErr, now)
				cache.mailCV = &view
			}
		}
		cache.at = now
	}
	views := []certificateView{cache.defaultCV}
	if cache.mailCV != nil {
		mail := *cache.mailCV
		if mail.ExpiresAt != "" {
			if expires, err := time.Parse(time.RFC3339, mail.ExpiresAt); err == nil {
				mail.DaysLeft = daysLeft(expires, now)
			}
		}
		views = append(views, mail)
	}
	return views
}

func (p *Panel) siteCertificateViews(ctx context.Context, now time.Time) ([]certificateView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name, sc.type, COALESCE(sc.issuer, ''), COALESCE(sc.subject, ''), CAST(sc.expires_at AS TEXT),
		       CASE WHEN COALESCE(sc.auto_renew, 1) = 1 THEN 1 ELSE 0 END,
		       CASE WHEN sc.secure_mail = 1 THEN 1 ELSE 0 END
		FROM ssl_certificates sc JOIN domains d ON d.id = sc.domain_id
		WHERE sc.status = 'active'
		ORDER BY sc.expires_at, d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []certificateView{}
	for rows.Next() {
		var (
			domainID                                   int
			name, kind, issuer, subject, expiresAtText string
			autoRenew, secureMail                      int
		)
		if err := rows.Scan(&domainID, &name, &kind, &issuer, &subject, &expiresAtText, &autoRenew, &secureMail); err != nil {
			return nil, err
		}
		view := certificateView{Kind: "site", Name: name, Issuer: issuer, State: "in_use", DomainID: domainID, UsedBy: []string{"website"}}
		if subject != "" && subject != name {
			view.Names = []string{subject}
		}
		if secureMail == 1 {
			view.UsedBy = append(view.UsedBy, "mail")
		}
		switch {
		case kind == "letsencrypt" && autoRenew == 1:
			view.Source, view.Renewal = "letsencrypt", "automatic"
		case kind == "letsencrypt":
			view.Source, view.Renewal = "letsencrypt", "off"
		default:
			view.Source, view.Renewal = "uploaded", "manual"
		}
		if expires, ok := parseStoredCertificateTime(expiresAtText); ok {
			view.ExpiresAt = expires.UTC().Format(time.RFC3339)
			view.DaysLeft = daysLeft(expires, now)
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func parseStoredCertificateTime(text string) (time.Time, bool) {
	text = strings.TrimSpace(text)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05Z07:00"} {
		if value, err := time.Parse(layout, text); err == nil {
			return value, true
		}
	}
	return time.Time{}, false
}

func (p *Panel) waitingCertificateRequests(ctx context.Context) ([]certificateRequestView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT id, common_name, alt_names, key_type, csr_pem, public_key_sha256, key_file, created_at
		FROM certificate_requests WHERE state = 'waiting' ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []certificateRequestView{}
	for rows.Next() {
		var view certificateRequestView
		var names string
		if err := rows.Scan(&view.ID, &view.CommonName, &names, &view.KeyType, &view.CSRPEM, &view.PublicKeySHA256, &view.KeyFile, &view.CreatedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(names), &view.Names) != nil || len(view.Names) == 0 {
			view.Names = []string{view.CommonName}
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (p *Panel) handleCertificatesOverview(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	now := time.Now()
	overview := certificatesOverview{ReadAt: now.UTC().Format(time.RFC3339)}
	overview.Certificates = append(overview.Certificates, panelCertificateView(now))
	overview.Certificates = append(overview.Certificates, p.agentCertificateViews(r.Context(), now)...)
	sites, err := p.siteCertificateViews(r.Context(), now)
	if err != nil {
		writeServerError(w, err)
		return
	}
	overview.Certificates = append(overview.Certificates, sites...)
	requests, err := p.waitingCertificateRequests(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	overview.Requests = requests
	_ = json.NewEncoder(w).Encode(overview)
}

// --- Requests -------------------------------------------------------------

var callCertificateRequestCreate = func(p *Panel, ctx context.Context, request *transport.CertificateRequestCreateRequest) (transport.CertificateRequestCreateResponse, error) {
	var response transport.CertificateRequestCreateResponse
	err := p.callAgentContext(ctx, "Agent.CertificateRequestCreateV1", request, &response)
	return response, err
}

var callCertificateRequestDiscard = func(p *Panel, ctx context.Context, request *transport.CertificateRequestDiscardRequest) (transport.CertificateRequestDiscardResponse, error) {
	var response transport.CertificateRequestDiscardResponse
	err := p.callAgentContext(ctx, "Agent.CertificateRequestDiscardV1", request, &response)
	return response, err
}

// canonicalRequestNames: each name canonical (a leading "*." kept), the
// first one the common name, none twice, 1..50.
func canonicalRequestNames(raw []string) ([]string, error) {
	names := []string{}
	seen := map[string]bool{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		wildcard := strings.HasPrefix(item, "*.")
		canonical, err := hostname.CanonicalFQDN(strings.TrimPrefix(item, "*."))
		if err != nil || strings.Contains(canonical, "*") {
			return nil, errors.New(item)
		}
		if wildcard {
			canonical = "*." + canonical
		}
		if !seen[canonical] {
			seen[canonical] = true
			names = append(names, canonical)
		}
	}
	if len(names) == 0 || len(names) > 50 {
		return nil, errors.New("count")
	}
	return names, nil
}

func newCertificateRequestToken() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

// certificateRequestTokenOf: the token is the key file's name.
func certificateRequestTokenOf(keyFile string) string {
	return strings.TrimSuffix(filepath.Base(keyFile), ".key")
}

func (p *Panel) handleCertificateRequests(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var req struct {
		Names   []string `json:"names"`
		KeyType string   `json:"key_type"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid, "The request was not understood, so nothing was made. Reload the page and try again.", "")
		return
	}
	if req.KeyType == "" {
		req.KeyType = transport.CertificateKeyECDSAP256
	}
	if !transport.ValidCertificateRequestKeyType(req.KeyType) {
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid, "Choose one of the key types offered (ECDSA P-256, RSA 2048, 3072 or 4096). Nothing was made.", "")
		return
	}
	names, err := canonicalRequestNames(req.Names)
	if err != nil {
		message := "Give 1 to 50 host names, one per line (for example shop.example.com or *.example.com). Nothing was made."
		if err.Error() != "count" {
			message = "\"" + truncateForMessage(err.Error()) + "\" is not a host name. Give full names such as shop.example.com or *.example.com, one per line. Nothing was made."
		}
		writeCodedError(w, http.StatusBadRequest, errCodeCertificateRequestInvalid, message, "")
		return
	}
	var waiting int
	if err := p.db.GetDB().QueryRowContext(r.Context(), `SELECT count(*) FROM certificate_requests WHERE state = 'waiting'`).Scan(&waiting); err != nil {
		writeServerError(w, err)
		return
	}
	if waiting >= certificateRequestsWaiting {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestTooMany,
			"There are already "+strconv.Itoa(waiting)+" requests waiting for their certificate. Nothing was made. Discard the ones you no longer need, then make a new one.", "")
		return
	}
	token, err := newCertificateRequestToken()
	if err != nil {
		writeServerError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
	defer cancel()
	response, err := callCertificateRequestCreate(p, ctx, &transport.CertificateRequestCreateRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Token: token,
		CommonName: names[0], Names: names, KeyType: req.KeyType,
	})
	if err != nil {
		log.Printf("certificate request: the Agent did not answer: %v", err)
		writeCodedError(w, http.StatusBadGateway, errCodeCertificateRequestUnknown,
			"The CelikPanel Agent did not answer, so whether a key was made is not known; no request is listed. Nobody needs to act: make the request again. A key left behind cannot be used by anyone and is removed with its directory.", "")
		return
	}
	if response.Error != "" || response.CSRPEM == "" || len(response.PublicKeySHA256) != 64 ||
		!filepath.IsAbs(response.KeyFile) || certificateRequestTokenOf(response.KeyFile) != token || len(response.KeyFile) > 300 {
		log.Printf("certificate request: refused by the Agent: %s", response.Error)
		writeCodedError(w, http.StatusBadGateway, errCodeCertificateRequestFailed,
			"The server could not make the key and request ("+truncateForMessage(response.Error)+"). Nothing was saved. Try again; if it repeats, check `sudo journalctl -u celikpanel-agent -n 50`.", "")
		return
	}
	alt, _ := json.Marshal(names)
	result, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO certificate_requests (common_name, alt_names, key_type, csr_pem, public_key_sha256, key_file, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, names[0], string(alt), req.KeyType, response.CSRPEM, response.PublicKeySHA256, response.KeyFile, currentCaller(r).ID)
	var id int64
	if err == nil {
		id, err = result.LastInsertId()
	}
	if err != nil {
		// Not listed, so the key goes too.
		if _, discardErr := callCertificateRequestDiscard(p, ctx, &transport.CertificateRequestDiscardRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Token: token}); discardErr != nil {
			log.Printf("certificate request: an unlisted key could not be removed: %v", discardErr)
		}
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "certificate.request.create", "certificate_request", int(id), map[string]any{
		"common_name": names[0], "names": len(names), "key_type": req.KeyType, "public_key_sha256": response.PublicKeySHA256,
	})
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(certificateRequestView{
		ID: int(id), CommonName: names[0], Names: names, KeyType: req.KeyType, CSRPEM: response.CSRPEM,
		PublicKeySHA256: response.PublicKeySHA256, KeyFile: response.KeyFile, CreatedAt: time.Now().UTC().Format("2006-01-02 15:04:05"),
	})
}

func truncateForMessage(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 120 {
		text = text[:120] + "…"
	}
	return text
}

// handleCertificateRequestItem: POST /api/v1/maintenance/certificate-requests/{id}/discard.
func (p *Panel) handleCertificateRequestItem(w http.ResponseWriter, r *http.Request) {
	if !requireMaintenanceAdmin(w, r) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, certificateRequestsPath+"/")
	idText, action, found := strings.Cut(rest, "/")
	id, valid := strictPositiveID(idText)
	if !found || action != "discard" || !valid {
		writeCodedError(w, http.StatusNotFound, errCodeCertificateRequestNotFound, "No such certificate request. Reload the page to see the current list.", "")
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	ctx := r.Context()
	var keyFile, state string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT key_file, state FROM certificate_requests WHERE id = ?`, id).Scan(&keyFile, &state)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state == "discarded") {
		writeCodedError(w, http.StatusNotFound, errCodeCertificateRequestNotFound, "This certificate request is not waiting any more (it was discarded). Reload the page to see the current list.", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state != "waiting" {
		writeCodedError(w, http.StatusConflict, errCodeCertificateRequestInvalid, "This request's certificate is installed, so its key is in use and is kept. Nothing was changed.", "")
		return
	}
	response, err := callCertificateRequestDiscard(p, ctx, &transport.CertificateRequestDiscardRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Token: certificateRequestTokenOf(keyFile),
	})
	if err != nil || response.Error != "" {
		log.Printf("certificate request %d: the key could not be removed: %v %s", id, err, response.Error)
		writeCodedError(w, http.StatusBadGateway, errCodeCertificateRequestFailed,
			"The key could not be removed, so the request is still listed. Try again; if it repeats, the owner can remove the file `"+keyFile+"` from a terminal with `sudo rm`.", "")
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE certificate_requests SET state = 'discarded' WHERE id = ? AND state = 'waiting'`, id); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "certificate.request.discard", "certificate_request", id, map[string]any{"key_removed": response.Removed})
	_ = json.NewEncoder(w).Encode(map[string]any{"state": "discarded", "key_removed": response.Removed})
}
