package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Setup wizard v2, phase 4 (alpha.114): the second server joins the first by
// one code at the start (D-021 Q4/Q5). The first server's administrator
// creates the code while its setup waits at the join step; the second
// server's administrator enters the first server's address and the code.
//
// The code is 24 characters (RFC 4648 base32): the first 16 carry 80 bits of
// the first panel's TLS key fingerprint (SHA-256 of its SubjectPublicKeyInfo),
// the last 8 a single-use secret of 40 bits. The second server checks the
// fingerprint during the TLS handshake, before anything is sent, so a wrong
// address or an interception never sees the secret. The first server keeps
// only a salted hash of the secret, for 24 hours and 5 attempts, checks the
// caller's source address against the address it declares, and consumes the
// code once: the same request (same request_id) after a lost answer gets the
// same join with fresh credentials, never a second join. The answer (the
// join document) carries what the second server's setup needs from the
// first: the main domain, the nameserver names, which server is the main DNS
// server, the first server's names and, when the second is the main DNS
// server, the list the first prepared. Nothing secret is logged, audited or
// returned by an administrator API.
// Ikinci sunucu ilk sunucuya baslangicta tek kodla katilir.

const (
	serverSetupJoinCodesPath   = serverSetupPath + "/join-codes"
	serverSetupJoinPath        = serverSetupPath + "/join"
	serverSetupPeerPath        = serverSetupPath + "/peer"
	serverSetupPeerConnectPath = serverSetupPath + "/peer/connect"
	serverSetupPeerNamesPath   = serverSetupPath + "/peer/names"
	serverSetupPeerDKIMPath    = serverSetupPath + "/peer/dkim"

	serverSetupJoinCodeKey  = "server_setup_join_code"
	serverSetupJoinKey      = "server_setup_join"
	serverSetupJoinAttempts = 5
	serverSetupJoinLifetime = 24 * time.Hour
	// The enrollment the join hands over is accepted only once the main DNS
	// server is ready (remote DNS accept rules); it lives long enough for the
	// registrar change and both setups.
	serverSetupJoinEnrollmentLifetime = 14 * 24 * time.Hour
	serverSetupJoinDocumentVersion    = 1
)

var serverSetupJoinEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)
var serverSetupJoinProofPattern = regexp.MustCompile(`^[A-Z2-7]{8}$`)

// serverSetupPeerAddressAllowed is the address policy for a peer server: a
// public unicast address. Tests replace it for loopback servers only.
var serverSetupPeerAddressAllowed = func(ip netip.Addr) bool { return remoteDNSPublicAddress(ip) }

// serverSetupPanelCertificate reads the certificate this panel serves, the
// same file the TLS listener and the certificate page use.
var serverSetupPanelCertificate = func() (*x509.Certificate, error) {
	path := os.Getenv("CELIKPANEL_TLS_CERT")
	if path == "" {
		active, _, found, err := managedPanelCertificatePaths(tlsDir())
		if err != nil {
			return nil, err
		}
		path = filepath.Join(tlsDir(), "panel.crt")
		if found {
			path = active
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("panel certificate is not PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func serverSetupSPKIPin(cert *x509.Certificate) []byte {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:10]
}

func formatServerSetupJoinCode(pin, secret []byte) string {
	raw := serverSetupJoinEncoding.EncodeToString(append(append([]byte{}, pin...), secret...))
	groups := []string{}
	for index := 0; index < len(raw); index += 4 {
		groups = append(groups, raw[index:index+4])
	}
	return strings.Join(groups, "-")
}

// parseServerSetupJoinCode returns the 10-byte fingerprint and the 8-character
// secret part of a code typed with or without dashes and spaces.
func parseServerSetupJoinCode(raw string) ([]byte, string, bool) {
	cleaned := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(raw)))
	if len(cleaned) != 24 {
		return nil, "", false
	}
	decoded, err := serverSetupJoinEncoding.DecodeString(cleaned)
	if err != nil || len(decoded) != 15 {
		return nil, "", false
	}
	return decoded[:10], cleaned[16:], true
}

func serverSetupJoinProofHash(salt, proof string) string {
	sum := sha256.Sum256([]byte(salt + ":" + proof))
	return hex.EncodeToString(sum[:])
}

type serverSetupJoinCodeRecord struct {
	Salt       string `json:"salt"`
	Hash       string `json:"hash"`
	Pin        string `json:"pin"`
	PlanID     string `json:"plan_id"`
	ExpiresAt  int64  `json:"expires_at"`
	Attempts   int    `json:"attempts"`
	CreatedAt  string `json:"created_at"`
	ConsumedBy string `json:"consumed_by,omitempty"`
}

type serverSetupJoinPeer struct {
	IPv4      string `json:"ipv4"`
	IPv6      string `json:"ipv6,omitempty"`
	ShortName string `json:"short_name,omitempty"`
	OS        string `json:"os,omitempty"`
}

// serverSetupPeerMail is the main domain's mail choice of the server that
// receives the main domain's mail when that server is not the main DNS
// server: its mail host and the owner's MX and DMARC answers.
type serverSetupPeerMail struct {
	Hostname string `json:"hostname"`
	MX       string `json:"mx,omitempty"`
	DMARC    string `json:"dmarc,omitempty"`
}

// serverSetupPeerNames is what the second server tells the first once its
// own setup starts: its names and, when it is the main DNS server, how the
// first server reaches it for publishing.
type serverSetupPeerNames struct {
	RequestID         string               `json:"request_id"`
	ShortName         string               `json:"short_name,omitempty"`
	PanelDomain       string               `json:"panel_domain"`
	MailHostname      string               `json:"mail_hostname,omitempty"`
	Mail              *serverSetupPeerMail `json:"mail,omitempty"`
	Address           string               `json:"address"`
	Pin               string               `json:"pin"`
	DNSEngine         string               `json:"dns_engine"`
	PublisherEndpoint string               `json:"publisher_endpoint,omitempty"`
	EnrollmentCode    string               `json:"enrollment_code,omitempty"`
}

// serverSetupJoinRecord is the first server's private record of the join:
// never returned as stored (the link credential and the handed-over
// enrollment code are secrets).
type serverSetupJoinRecord struct {
	RequestID      string                `json:"request_id"`
	PlanID         string                `json:"plan_id"`
	Digest         string                `json:"digest"`
	Peer           serverSetupJoinPeer   `json:"peer"`
	JoinedAt       string                `json:"joined_at"`
	LinkID         string                `json:"link_id"`
	LinkCredential string                `json:"link_credential"`
	EnrollmentHash string                `json:"enrollment_hash,omitempty"`
	Names          *serverSetupPeerNames `json:"names,omitempty"`
	NamesAt        string                `json:"names_at,omitempty"`
}

type serverSetupJoinFirst struct {
	IPv4           string              `json:"ipv4"`
	IPv6           string              `json:"ipv6,omitempty"`
	ShortName      string              `json:"short_name,omitempty"`
	PanelDomain    string              `json:"panel_domain"`
	MailHostname   string              `json:"mail_hostname,omitempty"`
	MainDomainMail bool                `json:"main_domain_mail,omitempty"`
	MailDNS        *serverSetupMailDNS `json:"mail_dns,omitempty"`
	DNSEngine      string              `json:"dns_engine"`
}

type serverSetupJoinPrepared struct {
	ImportedRecords []serverSetupImportedRecord `json:"imported_records,omitempty"`
}

// serverSetupJoinDocument is the answer to a join: everything the second
// server's setup takes from the first. MainDNSServer is "first" or "second".
type serverSetupJoinDocument struct {
	Version           int                      `json:"version"`
	RequestID         string                   `json:"request_id"`
	MainDomain        string                   `json:"main_domain"`
	ServerCount       int                      `json:"server_count"`
	DNSMode           string                   `json:"dns_mode"`
	MainDNSServer     string                   `json:"main_dns_server"`
	NS1               string                   `json:"ns1"`
	NS2               string                   `json:"ns2"`
	First             serverSetupJoinFirst     `json:"first"`
	Prepared          *serverSetupJoinPrepared `json:"prepared,omitempty"`
	PublisherEndpoint string                   `json:"publisher_endpoint,omitempty"`
	EnrollmentCode    string                   `json:"enrollment_code,omitempty"`
	LinkID            string                   `json:"link_id"`
	LinkCredential    string                   `json:"link_credential"`
}

// serverSetupJoinPending: a first server with two servers and its own DNS
// whose second server has not joined yet (the new flow: the peer is never
// typed, it joins by code).
func serverSetupJoinPending(d serverSetupDraft) bool {
	return d.DNSMode == setupDNSModeLocal && d.ServerCount == 2 && d.MainDomain != "" && d.PeerIP == ""
}

func serverSetupPanelEndpoint(name string) string {
	endpoint, _ := canonicalRemoteDNSEndpoint("https://" + net.JoinHostPort(name, strconv.Itoa(panelPort())))
	return endpoint
}

// serverSetupJoinDocumentFor is the document for a plan, without its
// per-join identity and secrets. Its digest tells whether a later plan still
// matches what the second server received.
func serverSetupJoinDocumentFor(d serverSetupDraft) serverSetupJoinDocument {
	document := serverSetupJoinDocument{Version: serverSetupJoinDocumentVersion, MainDomain: d.MainDomain, ServerCount: 2, DNSMode: d.DNSMode, MainDNSServer: "first", NS1: d.NS1, NS2: d.NS2,
		First: serverSetupJoinFirst{IPv4: d.LocalIP, ShortName: d.ShortName, PanelDomain: d.PanelDomain, DNSEngine: d.DNSEngine}}
	if serverSetupHasMail(d) {
		document.First.MailHostname, document.First.MainDomainMail = d.MailHostname, true
		document.First.MailDNS = d.MailDNS
	}
	if d.DNSRole == transport.DNSPairRoleSecondary {
		// Option A: the second server is the main DNS server. The list this
		// server prepared goes with the join; this server installs as backup.
		document.MainDNSServer = "second"
		if imported := serverSetupImported(d); len(imported) > 0 {
			document.Prepared = &serverSetupJoinPrepared{ImportedRecords: d.ImportedRecords}
		}
	} else {
		document.PublisherEndpoint = serverSetupPanelEndpoint(d.PanelDomain)
	}
	return document
}

func serverSetupJoinDocumentDigest(document serverSetupJoinDocument) string {
	document.RequestID, document.EnrollmentCode, document.LinkID, document.LinkCredential = "", "", "", ""
	document.First.IPv6 = ""
	raw, _ := json.Marshal(document)
	return serverSetupID(string(raw))
}

// --- storage ---------------------------------------------------------------

func readServerSetupJSONSetting(ctx context.Context, q serverSetupInfrastructureDNSQuery, key string, dst any) (bool, error) {
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return false, errors.New("saved setup join state is invalid")
	}
	return true, nil
}

type serverSetupExec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func writeServerSetupJSONSetting(ctx context.Context, q serverSetupExec, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO panel_settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=CURRENT_TIMESTAMP`, key, string(raw))
	return err
}

// serverSetupJoinWaitingPlan is the latest execution's plan when it has a
// join step that is not bound yet: the only time a code may be created or
// used.
func (p *Panel) serverSetupJoinWaitingPlan(ctx context.Context) (serverSetupPlan, *serverSetupExecution, error) {
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil {
		return serverSetupPlan{}, nil, err
	}
	if execution == nil || !stringIn(execution.Status, "running", "waiting") {
		return serverSetupPlan{}, nil, errServerSetupJoinNotWaiting
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	if err != nil {
		return serverSetupPlan{}, nil, err
	}
	if !serverSetupPlanHasStep(plan, "peer_join") {
		return serverSetupPlan{}, nil, errServerSetupJoinNotWaiting
	}
	binding, err := p.readServerSetupPeerBinding(ctx, plan, execution.ID)
	if err != nil {
		return serverSetupPlan{}, nil, err
	}
	if binding != nil {
		return serverSetupPlan{}, nil, errServerSetupJoinBound
	}
	return plan, execution, nil
}

func serverSetupPlanHasStep(plan serverSetupPlan, kind string) bool {
	for _, step := range plan.Steps {
		if step.Kind == kind {
			return true
		}
	}
	return false
}

var errServerSetupJoinNotWaiting = errors.New("this server's setup is not waiting for a second server")
var errServerSetupJoinBound = errors.New("the second server is already bound to this setup")

// --- rate limit ------------------------------------------------------------

// serverSetupJoinLimiter bounds join attempts on top of the code's own five:
// at most 20 a minute in all and 5 a minute from one address.
type serverSetupJoinLimiter struct {
	mu     sync.Mutex
	window time.Time
	total  int
	byIP   map[string]int
}

var setupJoinLimiter = &serverSetupJoinLimiter{}

func (l *serverSetupJoinLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.window) >= time.Minute || now.Before(l.window) {
		l.window, l.total, l.byIP = now, 0, map[string]int{}
	}
	if l.total >= 20 || l.byIP[ip] >= 5 {
		return false
	}
	l.total++
	l.byIP[ip]++
	return true
}

// --- administrator: the code -----------------------------------------------

type serverSetupJoinCodeView struct {
	Code         string `json:"code,omitempty"`
	Address      string `json:"address"`
	ExpiresAt    string `json:"expires_at"`
	AttemptsLeft int    `json:"attempts_left"`
	Used         bool   `json:"used"`
}

func (p *Panel) handleServerSetupJoinCodes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var request struct{}
	if decodeServiceOperationJSON(w, r, &request) != nil {
		writeClientError(w, http.StatusBadRequest, "invalid join code request")
		return
	}
	serverSetupJoinMu.Lock()
	defer serverSetupJoinMu.Unlock()
	plan, _, err := p.serverSetupJoinWaitingPlan(r.Context())
	if errors.Is(err, errServerSetupJoinNotWaiting) {
		writeCodedError(w, http.StatusConflict, "server_setup_join_not_waiting", "Create the code once this server's setup has started and waits for the second server.", "/setup")
		return
	}
	if errors.Is(err, errServerSetupJoinBound) {
		writeCodedError(w, http.StatusConflict, "server_setup_join_bound", "The second server is already part of this setup; no new code is needed.", "/setup")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	cert, err := serverSetupPanelCertificate()
	if err != nil {
		writeCodedError(w, http.StatusConflict, "server_setup_join_certificate_unavailable", "This panel's TLS certificate could not be read, so no code can be made. Check Settings → Panel certificate, then try again.", "")
		return
	}
	pin := serverSetupSPKIPin(cert)
	secret := make([]byte, 5)
	salt := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		writeServerError(w, err)
		return
	}
	if _, err := rand.Read(salt); err != nil {
		writeServerError(w, err)
		return
	}
	code := formatServerSetupJoinCode(pin, secret)
	_, proof, _ := parseServerSetupJoinCode(code)
	now := time.Now().UTC()
	record := serverSetupJoinCodeRecord{Salt: hex.EncodeToString(salt), Pin: hex.EncodeToString(pin), PlanID: plan.ID, ExpiresAt: now.Add(serverSetupJoinLifetime).Unix(), CreatedAt: now.Format(time.RFC3339)}
	record.Hash = serverSetupJoinProofHash(record.Salt, proof)
	if err := writeServerSetupJSONSetting(r.Context(), p.db.GetDB(), serverSetupJoinCodeKey, record); err != nil {
		writeServerError(w, err)
		return
	}
	// The code itself is shown once, here; the audit names the act only.
	p.audit(r, "server.setup.join_code", "", 0)
	_ = json.NewEncoder(w).Encode(serverSetupJoinCodeView{Code: code, Address: serverSetupJoinAddress(plan.Draft), ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC().Format(time.RFC3339), AttemptsLeft: serverSetupJoinAttempts})
}

var serverSetupJoinMu sync.Mutex

func serverSetupJoinAddress(d serverSetupDraft) string {
	ip := d.LocalIP
	if ip == "" {
		ip = serverPrimaryIP()
	}
	return "https://" + net.JoinHostPort(ip, strconv.Itoa(panelPort()))
}

// --- machine: the join -----------------------------------------------------

type serverSetupJoinRequest struct {
	Proof        string `json:"proof"`
	RequestID    string `json:"request_id"`
	DeclaredIPv4 string `json:"declared_ipv4"`
	DeclaredIPv6 string `json:"declared_ipv6,omitempty"`
	ShortName    string `json:"short_name,omitempty"`
	OS           string `json:"os,omitempty"`
}

type serverSetupJoinRefusal struct {
	Code         string `json:"code"`
	Error        string `json:"error"`
	AttemptsLeft *int   `json:"attempts_left,omitempty"`
}

func writeServerSetupJoinRefusal(w http.ResponseWriter, status int, code, message string, attemptsLeft *int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(serverSetupJoinRefusal{Code: code, Error: message, AttemptsLeft: attemptsLeft})
}

func serverSetupRequestSourceIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// serverSetupJoinSourceIP is the caller's address as the listener saw it
// (no proxy header is trusted). Tests map their loopback caller.
var serverSetupJoinSourceIP = serverSetupRequestSourceIP

var serverSetupOSPattern = regexp.MustCompile(`^[A-Za-z0-9 ._()+-]{0,64}$`)

func (p *Panel) handleServerSetupJoin(w http.ResponseWriter, r *http.Request) {
	source, ok := serverSetupJoinSourceIP(r)
	if !ok || !setupJoinLimiter.allow(source.String(), time.Now()) {
		writeServerSetupJoinRefusal(w, http.StatusTooManyRequests, "server_setup_join_rate_limited", "Too many join attempts; wait a minute and try again.", nil)
		return
	}
	var request serverSetupJoinRequest
	if !decodeRemoteDNSBody(w, r, &request) {
		return
	}
	declared, declaredErr := netip.ParseAddr(request.DeclaredIPv4)
	request.ShortName = strings.ToLower(strings.TrimSpace(request.ShortName))
	if !serverSetupJoinProofPattern.MatchString(request.Proof) || !validServiceOperationID(request.RequestID) || declaredErr != nil || !declared.Is4() || declared.String() != request.DeclaredIPv4 ||
		(request.ShortName != "" && !serverSetupShortNamePattern.MatchString(request.ShortName)) || !serverSetupOSPattern.MatchString(request.OS) {
		writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_join_invalid", "The join request is not valid.", nil)
		return
	}
	if request.DeclaredIPv6 != "" {
		v6, err := netip.ParseAddr(request.DeclaredIPv6)
		if err != nil || !v6.Is6() || v6.Is4In6() || v6.String() != request.DeclaredIPv6 {
			writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_join_invalid", "The join request is not valid.", nil)
			return
		}
	}
	serverSetupJoinMu.Lock()
	defer serverSetupJoinMu.Unlock()
	ctx := r.Context()
	var record serverSetupJoinCodeRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinCodeKey, &record)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_unavailable", "This server has no join code. Create one on this server's setup screen.", nil)
		return
	}
	left := func() *int { value := max(0, serverSetupJoinAttempts-record.Attempts); return &value }
	proofOK := subtle.ConstantTimeCompare([]byte(serverSetupJoinProofHash(record.Salt, request.Proof)), []byte(record.Hash)) == 1
	sourceOK := source == declared || (request.DeclaredIPv6 != "" && source.String() == request.DeclaredIPv6)
	if record.ConsumedBy != "" {
		// The same request after a lost answer gets the same join; any other
		// request finds the code used.
		if record.ConsumedBy == request.RequestID && proofOK && sourceOK {
			p.answerServerSetupJoinReplay(w, r, request)
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_used", "This code was already used. Create a new code on the first server if the second server must join again.", nil)
		return
	}
	if record.Attempts >= serverSetupJoinAttempts {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_exhausted", "This code was entered wrongly too often. Create a new code on the first server.", left())
		return
	}
	if time.Now().Unix() >= record.ExpiresAt {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_expired", "This code has expired. Create a new code on the first server.", nil)
		return
	}
	if !proofOK || !sourceOK || !serverSetupPeerAddressAllowed(declared) {
		record.Attempts++
		if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinCodeKey, record); err != nil {
			writeServerError(w, err)
			return
		}
		if proofOK && !sourceOK {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_join_source_mismatch", "The request did not come from the address the second server gave. Join from the second server itself, without a proxy.", left())
			return
		}
		if proofOK {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_join_address_not_public", "The second server's address is not a public server address.", left())
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusUnauthorized, "server_setup_join_code_wrong", "The code is not correct. Check it on the first server's setup screen.", left())
		return
	}
	plan, _, err := p.serverSetupJoinWaitingPlan(ctx)
	if err != nil || plan.ID != record.PlanID {
		if err != nil && !errors.Is(err, errServerSetupJoinNotWaiting) && !errors.Is(err, errServerSetupJoinBound) {
			writeServerError(w, err)
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_join_not_waiting", "The first server's setup changed since this code was made. Create a new code on the first server.", nil)
		return
	}
	if request.DeclaredIPv4 == plan.Draft.LocalIP {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_join_same_server", "The second server must be a different server from the first.", nil)
		return
	}
	document, join, err := p.newServerSetupJoin(ctx, plan, request)
	if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	if err := p.replaceServerSetupJoinEnrollment(ctx, tx, document.EnrollmentCode, ""); err != nil {
		writeServerError(w, err)
		return
	}
	record.ConsumedBy = request.RequestID
	if err := writeServerSetupJSONSetting(ctx, tx, serverSetupJoinCodeKey, record); err != nil {
		writeServerError(w, err)
		return
	}
	if err := writeServerSetupJSONSetting(ctx, tx, serverSetupJoinKey, join); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "server.setup.peer_joined", "", 0)
	p.wakeServerSetupRunner()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(document)
}

// newServerSetupJoin makes the document and the private record of a join,
// with a fresh link credential and, when this server is the main DNS server,
// a fresh enrollment code for the second server's publishing connection.
func (p *Panel) newServerSetupJoin(ctx context.Context, plan serverSetupPlan, request serverSetupJoinRequest) (serverSetupJoinDocument, serverSetupJoinRecord, error) {
	document := serverSetupJoinDocumentFor(plan.Draft)
	document.First.IPv6 = serverSetupPublicIPv6()
	document.RequestID = request.RequestID
	linkID, err := newServiceOperationID()
	if err != nil {
		return document, serverSetupJoinRecord{}, err
	}
	credential, err := remoteDNSSecret()
	if err != nil {
		return document, serverSetupJoinRecord{}, err
	}
	document.LinkID, document.LinkCredential = linkID, credential
	join := serverSetupJoinRecord{RequestID: request.RequestID, PlanID: plan.ID, Digest: serverSetupJoinDocumentDigest(document),
		Peer:     serverSetupJoinPeer{IPv4: request.DeclaredIPv4, IPv6: request.DeclaredIPv6, ShortName: request.ShortName, OS: strings.TrimSpace(request.OS)},
		JoinedAt: time.Now().UTC().Format(time.RFC3339), LinkID: linkID, LinkCredential: credential}
	if document.MainDNSServer == "first" {
		code, err := remoteDNSSecret()
		if err != nil {
			return document, join, err
		}
		document.EnrollmentCode = code
		join.EnrollmentHash = remoteDNSHash(code)
	}
	return document, join, nil
}

// replaceServerSetupJoinEnrollment issues the handed-over enrollment code and
// withdraws a previous one that was never used.
func (p *Panel) replaceServerSetupJoinEnrollment(ctx context.Context, tx *sql.Tx, code, previousHash string) error {
	if previousHash != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM remote_dns_enrollments WHERE code_hash=? AND client_id=''`, previousHash); err != nil {
			return err
		}
	}
	if code == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO remote_dns_enrollments(code_hash,expires_at) VALUES(?,?)`, remoteDNSHash(code), time.Now().UTC().Add(serverSetupJoinEnrollmentLifetime).Unix())
	return err
}

// answerServerSetupJoinReplay answers the same join again after a lost
// answer: the same peer and plan, fresh secrets (the lost ones were never
// seen), the unused old enrollment withdrawn. Names already received stay.
func (p *Panel) answerServerSetupJoinReplay(w http.ResponseWriter, r *http.Request, request serverSetupJoinRequest) {
	ctx := r.Context()
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		writeServerError(w, err)
		return
	}
	plan, err := p.loadServerSetupPlan(ctx, join.PlanID)
	if !found || err != nil || join.RequestID != request.RequestID || join.Peer.IPv4 != request.DeclaredIPv4 {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_used", "This code was already used. Create a new code on the first server if the second server must join again.", nil)
		return
	}
	document, fresh, err := p.newServerSetupJoin(ctx, plan, request)
	if err != nil {
		writeServerError(w, err)
		return
	}
	enrollmentUsed := false
	if join.EnrollmentHash != "" {
		var client string
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT client_id FROM remote_dns_enrollments WHERE code_hash=?`, join.EnrollmentHash).Scan(&client); err == nil && client != "" {
			enrollmentUsed = true
		}
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	join.LinkID, join.LinkCredential = fresh.LinkID, fresh.LinkCredential
	if enrollmentUsed {
		// The second server already enrolled with the earlier code.
		document.EnrollmentCode = ""
	} else {
		if err := p.replaceServerSetupJoinEnrollment(ctx, tx, document.EnrollmentCode, join.EnrollmentHash); err != nil {
			writeServerError(w, err)
			return
		}
		join.EnrollmentHash = fresh.EnrollmentHash
	}
	if err := writeServerSetupJSONSetting(ctx, tx, serverSetupJoinKey, join); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(document)
}

func serverSetupPublicIPv6() string {
	if parsed := net.ParseIP(serverPrimaryIPv6()); parsed != nil && parsed.To4() == nil && parsed.IsGlobalUnicast() && !parsed.IsPrivate() {
		return parsed.String()
	}
	return ""
}

// --- machine: the link ------------------------------------------------------

func serverSetupLinkMachinePath(path string) bool {
	return path == serverSetupJoinPath || path == serverSetupPeerNamesPath || path == serverSetupPeerDKIMPath
}

// serverSetupLinkMachineRequest has the remote DNS machine request's shape:
// TLS, POST, no query, no browser headers.
func serverSetupLinkMachineRequest(r *http.Request) bool {
	if !serverSetupLinkMachinePath(r.URL.Path) || r.Method != http.MethodPost || r.TLS == nil || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		return false
	}
	for name := range r.Header {
		lower := strings.ToLower(name)
		if lower == "origin" || lower == "referer" || lower == "cookie" || strings.HasPrefix(lower, "sec-fetch-") {
			return false
		}
	}
	return true
}

// requireServerSetupLinkMachine answers the join and link machine endpoints
// before CSRF and session middleware: they are authenticated by the code or
// the link credential alone, never by a browser session.
func (p *Panel) requireServerSetupLinkMachine(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverSetupLinkMachinePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !serverSetupLinkMachineRequest(r) {
			writeServerSetupJoinRefusal(w, http.StatusUnauthorized, "server_setup_link_unauthorized", "This request is not a server setup link request.", nil)
			return
		}
		switch r.URL.Path {
		case serverSetupJoinPath:
			p.handleServerSetupJoin(w, r)
		case serverSetupPeerNamesPath:
			if p.authenticateServerSetupLink(w, r) {
				p.handleServerSetupPeerNames(w, r)
			}
		case serverSetupPeerDKIMPath:
			if p.authenticateServerSetupLink(w, r) {
				p.handleServerSetupPeerDKIM(w, r)
			}
		}
	})
}

// serverSetupLinkSecret reads this server's side of the link: on the first
// server the join record, on the second the link record.
func (p *Panel) serverSetupLinkSecret(ctx context.Context) (string, string, error) {
	var join serverSetupJoinRecord
	if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil || found {
		return join.LinkID, join.LinkCredential, err
	}
	var link serverSetupLinkRecord
	if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupLinkKey, &link); err != nil || found {
		return link.LinkID, link.LinkCredential, err
	}
	return "", "", nil
}

func (p *Panel) authenticateServerSetupLink(w http.ResponseWriter, r *http.Request) bool {
	deny := func() bool {
		writeServerSetupJoinRefusal(w, http.StatusUnauthorized, "server_setup_link_unauthorized", "The server setup link is not valid; join the second server again.", nil)
		return false
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(r.Header.Values("Authorization")) != 1 {
		return deny()
	}
	id, credential, ok := strings.Cut(strings.TrimPrefix(auth, "Bearer "), ".")
	if !ok || !validServiceOperationID(id) || !remoteDNSValidSecret(credential) {
		return deny()
	}
	storedID, storedCredential, err := p.serverSetupLinkSecret(r.Context())
	if err != nil || storedID == "" || subtle.ConstantTimeCompare([]byte(id), []byte(storedID)) != 1 || !remoteDNSHashEqual(remoteDNSHash(credential), remoteDNSHash(storedCredential)) {
		return deny()
	}
	return true
}

// handleServerSetupPeerNames: the second server's names reach the first
// server, which checks them against its own names and records at once
// (clashes are refused before anything is written), keeps them and wakes
// its waiting setup. The same names again are accepted; other names are
// accepted until the first server's setup bound the second server.
func (p *Panel) handleServerSetupPeerNames(w http.ResponseWriter, r *http.Request) {
	var names serverSetupPeerNames
	if !decodeRemoteDNSBody(w, r, &names) {
		return
	}
	ctx := r.Context()
	serverSetupJoinMu.Lock()
	defer serverSetupJoinMu.Unlock()
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_not_joined", "This server has no joined second server.", nil)
		return
	}
	plan, err := p.loadServerSetupPlan(ctx, join.PlanID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if code, message := canonicalServerSetupPeerNames(&names, plan.Draft, join); code != "" {
		writeServerSetupJoinRefusal(w, http.StatusUnprocessableEntity, code, message, nil)
		return
	}
	if join.Names != nil && !serverSetupPeerNamesEqual(*join.Names, names) {
		if execution, err := p.latestServerSetupExecution(ctx); err == nil && execution != nil {
			if latest, err := p.loadServerSetupPlan(ctx, execution.PlanID); err == nil {
				if binding, err := p.readServerSetupPeerBinding(ctx, latest, execution.ID); err == nil && binding != nil {
					writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_names_bound", "The first server's setup already uses the second server's earlier names. Keep those names, or reopen the first server's plan.", nil)
					return
				}
			}
		}
	}
	join.Names, join.NamesAt = &names, time.Now().UTC().Format(time.RFC3339)
	if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join); err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "server.setup.peer_names", "", 0)
	p.wakeServerSetupRunner()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
}

func serverSetupPeerNamesEqual(a, b serverSetupPeerNames) bool {
	a.RequestID, b.RequestID = "", ""
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

// canonicalServerSetupPeerNames checks the second server's names: full names
// inside the main domain, never one of this server's names, and, when this
// server keeps the zone, publishable beside every record it prepared.
func canonicalServerSetupPeerNames(names *serverSetupPeerNames, d serverSetupDraft, join serverSetupJoinRecord) (string, string) {
	invalid := func() (string, string) {
		return "server_setup_peer_names_invalid", "The second server's names are not valid."
	}
	if !validServiceOperationID(names.RequestID) || !transport.ValidDNSEngine(transport.DNSEngine(names.DNSEngine)) {
		return invalid()
	}
	for _, value := range []*string{&names.PanelDomain, &names.MailHostname} {
		if *value == "" {
			continue
		}
		canonical, err := hostname.CanonicalFQDN(*value)
		if err != nil || canonical != *value || !serverSetupDNSNameWithin(canonical, d.MainDomain) {
			return "server_setup_peer_names_outside_domain", "The second server's names must be names under " + d.MainDomain + "."
		}
	}
	if names.PanelDomain == "" || (names.ShortName != "" && !serverSetupShortNamePattern.MatchString(names.ShortName)) {
		return invalid()
	}
	if names.Mail != nil && (names.Mail.Hostname != names.MailHostname || names.MailHostname == "" || !stringIn(names.Mail.MX, "", "now", "later") || !stringIn(names.Mail.DMARC, "", "keep", "add")) {
		return invalid()
	}
	if _, err := hex.DecodeString(names.Pin); err != nil || len(names.Pin) != 20 {
		return invalid()
	}
	address, ok := canonicalServerSetupPeerAddress(names.Address)
	if !ok || address != names.Address || !strings.Contains(address, "//"+join.Peer.IPv4+":") {
		return invalid()
	}
	if d.DNSRole == transport.DNSPairRoleSecondary {
		// Option A: the second server is the main DNS server; its
		// publishing endpoint and enrollment code are needed here.
		endpoint, err := canonicalRemoteDNSEndpoint(names.PublisherEndpoint)
		if err != nil || endpoint != names.PublisherEndpoint || !remoteDNSValidSecret(names.EnrollmentCode) || names.PublisherEndpoint != serverSetupPanelEndpoint(names.PanelDomain) {
			return invalid()
		}
	} else if names.PublisherEndpoint != "" || names.EnrollmentCode != "" {
		return invalid()
	}
	for _, own := range []string{d.PanelDomain, d.MailHostname, d.NS1, d.NS2, d.MainDomain} {
		if own != "" && (names.PanelDomain == own || names.MailHostname == own) {
			return "server_setup_peer_names_clash", "The second server's name " + own + " is already one of the first server's names. Choose another name on the second server."
		}
	}
	if names.MailHostname != "" && names.MailHostname == names.PanelDomain {
		return invalid()
	}
	if names.Mail != nil && serverSetupHasMail(d) {
		return "server_setup_peer_mail_both", "The first server already receives the main domain's mail; the second server cannot also take the main domain's mail records."
	}
	if d.DNSRole == transport.DNSPairRolePrimary && d.InfrastructureDNS != nil {
		bound := serverSetupEffectiveDraft(d, serverSetupPeerBindingFromJoin(join, *names))
		if _, err := serverSetupInfrastructureDNSDesired(bound); err != nil {
			var planErr *serverSetupInfrastructureDNSPlanError
			if errors.As(err, &planErr) {
				return "server_setup_peer_names_clash", planErr.Message
			}
			return "server_setup_peer_names_clash", err.Error()
		}
	}
	return "", ""
}

// canonicalServerSetupPeerAddress: https://<IP literal>:<port>, the form a
// join uses for a server not yet reachable by name.
func canonicalServerSetupPeerAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	if !strings.HasPrefix(raw, "https://") {
		return "", false
	}
	rest := strings.TrimSuffix(strings.TrimPrefix(raw, "https://"), "/")
	host, port, err := net.SplitHostPort(rest)
	if err != nil {
		host, port = rest, strconv.Itoa(panelPort())
	}
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	number, portErr := strconv.Atoi(port)
	if err != nil || ip.Zone() != "" || portErr != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", false
	}
	return "https://" + net.JoinHostPort(ip.Unmap().String(), port), true
}
