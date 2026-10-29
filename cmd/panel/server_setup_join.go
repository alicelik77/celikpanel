package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/secrets"
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

	serverSetupJoinCodeKey = "server_setup_join_code"
	serverSetupJoinKey     = "server_setup_join"
	// Wrong codes: 5 per source address (per /64 for IPv6), 50 per code in
	// all; a correct code from a wrong address uses up nothing. What a screen
	// shows as "wrong tries left" is the code's own count (50 at creation, the
	// same on every later read and refusal); the per-source limit is not shown.
	serverSetupJoinAttempts      = 5
	serverSetupJoinCodeFailures  = 50
	serverSetupJoinRatePerSource = 10
	serverSetupJoinRateTotal     = 600
	serverSetupJoinLifetime      = 24 * time.Hour
	// The enrollment the join hands over is accepted only once the main DNS
	// server is ready (remote DNS accept rules); it lives long enough for the
	// registrar change and both setups.
	serverSetupJoinEnrollmentLifetime = 14 * 24 * time.Hour
	serverSetupJoinDocumentVersion    = 1
)

var serverSetupJoinEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)
var serverSetupJoinProofPattern = regexp.MustCompile(`^[A-Z2-7]{8}$`)

// The setup link's identity is the certificate the listener serves on the
// panel's IP address (no name in the handshake), which is where peers dial:
// configurePanelHTTPTLS puts the first self-signed identity there while
// panelIPCertificate finds it valid and paired with its key (CELIKPANEL_TLS
// managed certificates), otherwise the served certificate itself. It is
// captured once when the listener starts, never read again from the files,
// so the pin a code carries is the key that actually answers.
var (
	errServerSetupIdentityMissing = errors.New("this panel serves no TLS identity on its address")
	errServerSetupIdentityInvalid = errors.New("this panel's address identity is not a valid self-signed certificate for its IP address")
)

// captureServerSetupIdentity records the certificate a handshake without a
// name gets from this listener configuration.
func (p *Panel) captureServerSetupIdentity(config *tls.Config) {
	if config == nil || len(config.Certificates) == 0 || len(config.Certificates[0].Certificate) == 0 {
		return
	}
	leaf := config.Certificates[0].Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(config.Certificates[0].Certificate[0])
		if err != nil {
			return
		}
		leaf = parsed
	}
	p.setupLink.identity.Store(leaf)
}

// serverSetupPanelCertificate is the captured identity when it can be the
// link's trust: self-signed, for an IP address, valid now. A missing or
// unusable one is a verified state (server_setup_join_certificate_unavailable).
func (p *Panel) serverSetupPanelCertificate() (*x509.Certificate, error) {
	leaf := p.setupLink.identity.Load()
	if leaf == nil {
		return nil, errServerSetupIdentityMissing
	}
	now := time.Now()
	if len(leaf.IPAddresses) == 0 || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) ||
		leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) != nil {
		return nil, errServerSetupIdentityInvalid
	}
	return leaf, nil
}

// serverSetupIdentityGuidance: who acts and what to do when the identity
// cannot be used (D-024).
const serverSetupIdentityGuidance = "This panel does not serve a valid certificate of its own on its IP address (its self-signed panel certificate is missing, damaged or expired), and the other server checks exactly that key, so it cannot be paired by code. The server owner restores the panel's own certificate (panel.crt and panel.key in the panel's tls folder, for example from a backup) and restarts the panel; then try again."

// serverSetupSPKI is the full SHA-256 of a certificate's public key; the
// join code carries its first 10 bytes.
func serverSetupSPKI(cert *x509.Certificate) []byte {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

func serverSetupSPKIPin(cert *x509.Certificate) []byte { return serverSetupSPKI(cert)[:10] }

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
	// Dashes and spaces are dropped; digits that look like base32 letters
	// (0, 1, 8) are read as O, I and B.
	cleaned := strings.NewReplacer("-", "", " ", "", "0", "O", "1", "I", "8", "B").Replace(strings.ToUpper(strings.TrimSpace(raw)))
	if len(cleaned) != 24 {
		return nil, "", false
	}
	decoded, err := serverSetupJoinEncoding.DecodeString(cleaned)
	if err != nil || len(decoded) != 15 {
		return nil, "", false
	}
	return decoded[:10], cleaned[16:], true
}

// The join's secrets at rest (alpha.114): what the first server only checks
// is kept as a keyed hash, what a server must use again is sealed, both
// under the panel's key file (secret.key, outside the database), so a copy
// of either database alone yields no bearer, code or enrollment. Both
// formats carry the key generation they were made with
// (serverSetupSecretGeneration + ":"), so a later key rotation can tell old
// values from new ones; there is no rotation yet. A panel without a key
// (only tests) uses a key of the process.
const serverSetupSecretGeneration = "k1"

var serverSetupProcessBox = func() *secrets.Box {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, err := secrets.New(key)
	if err != nil {
		panic(err)
	}
	return box
}()

func (p *Panel) serverSetupBox() *secrets.Box {
	if p.secrets != nil {
		return p.secrets
	}
	return serverSetupProcessBox
}

// serverSetupMAC is the keyed hash of a join value under one label.
func (p *Panel) serverSetupMAC(label, value string) string {
	return serverSetupSecretGeneration + ":" + p.serverSetupBox().MAC("server-setup/"+label, value)
}

func (p *Panel) serverSetupJoinProofHash(salt, proof string) string {
	return p.serverSetupMAC("join-proof", salt+":"+proof)
}

// serverSetupReverseCredential is the credential the first server uses on
// the second: derived from the link credential by the second server, which
// therefore keeps no copy of it, and sent with its names.
func serverSetupReverseCredential(linkCredential string) string {
	mac := hmac.New(sha256.New, []byte(linkCredential))
	mac.Write([]byte("celikpanel-setup-link-reverse/v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// sealServerSetupSecret / openServerSetupSecret keep a secret this server
// must use again sealed under the key file. The empty value stays empty (a
// removed credential); anything else must be a sealed value of this key
// generation: a plain value is never read as a secret.
func (p *Panel) sealServerSetupSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	sealed, err := p.serverSetupBox().Encrypt(value)
	if err != nil {
		return "", err
	}
	return serverSetupSecretGeneration + ":" + sealed, nil
}

var errServerSetupSecretUnreadable = errors.New("a saved setup secret cannot be opened with this panel's key")

func (p *Panel) openServerSetupSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	sealed, ok := strings.CutPrefix(value, serverSetupSecretGeneration+":")
	if !ok || !secrets.IsEncrypted(sealed) {
		return "", errServerSetupSecretUnreadable
	}
	opened, err := p.serverSetupBox().Decrypt(sealed)
	if err != nil {
		return "", errServerSetupSecretUnreadable
	}
	return opened, nil
}

// openServerSetupPeerNames is a copy of received names with the enrollment
// code (Option A) opened: never written back.
func (p *Panel) openServerSetupPeerNames(names serverSetupPeerNames) (serverSetupPeerNames, error) {
	code, err := p.openServerSetupSecret(names.EnrollmentCode)
	names.EnrollmentCode = code
	return names, err
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
	// FailuresBy counts wrong codes per source (/32, or /64 for IPv6);
	// Attempts is their total for the code.
	FailuresBy map[string]int `json:"failures_by,omitempty"`
}

// serverSetupJoinSourceKey groups callers: one IPv4 address, or one IPv6
// /64 (a host usually holds a whole /64).
func serverSetupJoinSourceKey(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is6() {
		prefix, _ := ip.Prefix(64)
		return prefix.String()
	}
	return ip.String()
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
	Protocol          int                  `json:"protocol"`
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
	// ReverseCredential is what this server accepts from the first server.
	ReverseCredential string `json:"reverse_credential"`
}

// serverSetupJoinRecord is the first server's private record of the join:
// never returned as stored (the link credential and the handed-over
// enrollment code are secrets).
type serverSetupJoinRecord struct {
	RequestID string              `json:"request_id"`
	PlanID    string              `json:"plan_id"`
	Digest    string              `json:"digest"`
	Peer      serverSetupJoinPeer `json:"peer"`
	JoinedAt  string              `json:"joined_at"`
	LinkID    string              `json:"link_id"`
	// Only a keyed hash of the credential the second server uses here is
	// kept; PeerCredential is the one this server uses on the second (sent
	// with its names), sealed under the key file.
	LinkCredentialHash string                `json:"link_credential_hash"`
	PeerCredential     string                `json:"peer_credential,omitempty"`
	EnrollmentHash     string                `json:"enrollment_hash,omitempty"`
	Names              *serverSetupPeerNames `json:"names,omitempty"`
	NamesAt            string                `json:"names_at,omitempty"`
	// NamesChanged (Beta-1 fix): the second server sent other names after
	// this join was bound to a setup run that has not succeeded. The binding
	// is never rewritten with them: the run waits for a new join
	// (server_setup_peer_join_changed), and a new code may be created.
	NamesChanged bool `json:"names_changed,omitempty"`
	// PeerReadyAt / PeerRetiredAt (Beta-1 re-run P7): when this server is
	// the main DNS server, the link outlives its own setup until the second
	// server reports ready over it, or the owner stops waiting for it.
	PeerReadyAt   string `json:"peer_ready_at,omitempty"`
	PeerRetiredAt string `json:"peer_retired_at,omitempty"`
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
	Protocol          int                      `json:"protocol"`
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
	// EnrollmentUsed: a replayed join whose enrollment the second server
	// already used; there is no new code to hand over.
	EnrollmentUsed bool   `json:"enrollment_used,omitempty"`
	LinkID         string `json:"link_id"`
	LinkCredential string `json:"link_credential"`
	// FirstSPKI is the first server's full key fingerprint; it must match
	// the code's prefix and the key the join was answered with.
	FirstSPKI string `json:"first_spki"`
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
	document := serverSetupJoinDocument{Protocol: serverSetupLinkProtocol, Version: serverSetupJoinDocumentVersion, MainDomain: d.MainDomain, ServerCount: 2, DNSMode: d.DNSMode, MainDNSServer: "first", NS1: d.NS1, NS2: d.NS2,
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
	document.RequestID, document.EnrollmentCode, document.LinkID, document.LinkCredential, document.FirstSPKI = "", "", "", "", ""
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
		// A binding belongs to one run. While that run has not succeeded and
		// its binding went stale (the second server's answers changed, or a
		// newer join replaced it), a new code is the recovery (Beta-1 P4).
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil {
			return serverSetupPlan{}, nil, err
		}
		if !found || !serverSetupBindingStale(join, *binding) {
			return serverSetupPlan{}, nil, errServerSetupJoinBound
		}
		unused, err := p.serverSetupBindingUnused(ctx, *execution)
		if err != nil {
			return serverSetupPlan{}, nil, err
		}
		if !unused {
			return serverSetupPlan{}, nil, errServerSetupJoinReopen
		}
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
var errServerSetupJoinReopen = errors.New("the second server's earlier answers were already used by this setup run")

// --- rate limit ------------------------------------------------------------

// serverSetupJoinLimiter bounds join requests per source (an IPv4 address or
// an IPv6 /64) per minute, with a much larger ceiling in all, so one caller
// cannot hold every other one out.
type serverSetupJoinLimiter struct {
	mu     sync.Mutex
	window time.Time
	total  int
	byIP   map[string]int
}

func (l *serverSetupJoinLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.window) >= time.Minute || now.Before(l.window) {
		l.window, l.total, l.byIP = now, 0, map[string]int{}
	}
	if l.total >= serverSetupJoinRateTotal || l.byIP[ip] >= serverSetupJoinRatePerSource {
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
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	plan, _, err := p.serverSetupJoinWaitingPlan(r.Context())
	if errors.Is(err, errServerSetupJoinNotWaiting) {
		if finished, findErr := p.serverSetupSucceededPeerBinding(r.Context()); findErr == nil && finished != nil {
			writeCodedError(w, http.StatusConflict, "server_setup_join_finished", "This server's setup finished with its second server joined. Joining a second server again after a finished setup is not available in this release.", "/setup")
			return
		}
		writeCodedError(w, http.StatusConflict, "server_setup_join_not_waiting", "Create the code once this server's setup has started and waits for the second server.", "/setup")
		return
	}
	if errors.Is(err, errServerSetupJoinBound) {
		writeCodedError(w, http.StatusConflict, "server_setup_join_bound", "The second server is already part of this setup; no new code is needed.", "/setup")
		return
	}
	if errors.Is(err, errServerSetupJoinReopen) {
		writeCodedError(w, http.StatusConflict, "server_setup_join_reopen_required", serverSetupPeerJoinReopenMessage, "/setup")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	cert, err := p.serverSetupPanelCertificate()
	if err != nil {
		writeCodedError(w, http.StatusConflict, "server_setup_join_certificate_unavailable", serverSetupIdentityGuidance, "")
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
	record.Hash = p.serverSetupJoinProofHash(record.Salt, proof)
	if err := writeServerSetupJSONSetting(r.Context(), p.db.GetDB(), serverSetupJoinCodeKey, record); err != nil {
		writeServerError(w, err)
		return
	}
	// The code itself is shown once, here; the audit names the act only.
	p.audit(r, "server.setup.join_code", "", 0)
	_ = json.NewEncoder(w).Encode(serverSetupJoinCodeView{Code: code, Address: serverSetupJoinAddress(plan.Draft), ExpiresAt: time.Unix(record.ExpiresAt, 0).UTC().Format(time.RFC3339), AttemptsLeft: serverSetupJoinCodeFailures})
}

// serverSetupJoinCodeAttemptsLeft is the one meaning of "wrong tries left":
// the code's remaining wrong attempts in all.
func serverSetupJoinCodeAttemptsLeft(record serverSetupJoinCodeRecord) int {
	return max(0, serverSetupJoinCodeFailures-record.Attempts)
}

// serverSetupJoinLocks serializes one panel's join, names and binding.
var serverSetupJoinLocks sync.Map

func (p *Panel) serverSetupJoinLock() *sync.Mutex {
	lock, _ := serverSetupJoinLocks.LoadOrStore(p, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func serverSetupJoinAddress(d serverSetupDraft) string {
	ip := d.LocalIP
	if ip == "" {
		ip = serverPrimaryIP()
	}
	return "https://" + net.JoinHostPort(ip, strconv.Itoa(panelPort()))
}

// --- machine: the join -----------------------------------------------------

type serverSetupJoinRequest struct {
	Protocol     int    `json:"protocol"`
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

var serverSetupOSPattern = regexp.MustCompile(`^[A-Za-z0-9 ._()+-]{0,64}$`)

func (p *Panel) handleServerSetupJoin(w http.ResponseWriter, r *http.Request) {
	source, ok := p.serverSetupJoinSourceIP(r)
	if !ok || !p.setupLink.limiter.allow(serverSetupJoinSourceKey(source), time.Now()) {
		writeServerSetupJoinRefusal(w, http.StatusTooManyRequests, "server_setup_join_rate_limited", "Too many join attempts; wait a minute and try again.", nil)
		return
	}
	var request serverSetupJoinRequest
	if !decodeServerSetupLinkBody(w, r, &request) || refuseServerSetupLinkProtocol(w, request.Protocol) {
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
		// Informational only (the second server's address record): a public
		// global unicast address, never used to authorize the join.
		v6, err := netip.ParseAddr(request.DeclaredIPv6)
		if err != nil || !v6.Is6() || v6.Is4In6() || v6.String() != request.DeclaredIPv6 || !v6.IsGlobalUnicast() || v6.IsPrivate() || !p.serverSetupPeerAddressAllowed(v6) {
			writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_join_invalid", "The join request is not valid.", nil)
			return
		}
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
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
	sourceKey := serverSetupJoinSourceKey(source)
	left := func() *int { value := serverSetupJoinCodeAttemptsLeft(record); return &value }
	proofOK := subtle.ConstantTimeCompare([]byte(p.serverSetupJoinProofHash(record.Salt, request.Proof)), []byte(record.Hash)) == 1
	// The second server always dials the first server's IPv4 address, so the
	// caller's address must be exactly the IPv4 address it declares.
	sourceOK := source.Is4() && source == declared
	if record.ConsumedBy != "" {
		// The same request after a lost answer gets the same join while the
		// code is still valid; any other request finds the code used.
		if record.ConsumedBy == request.RequestID && proofOK && sourceOK {
			if time.Now().Unix() >= record.ExpiresAt {
				writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_expired", "This code has expired. Create a new code on the first server.", nil)
				return
			}
			p.answerServerSetupJoinReplay(w, r, request)
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_used", "This code was already used. Create a new code on the first server if the second server must join again.", nil)
		return
	}
	if record.Attempts >= serverSetupJoinCodeFailures || record.FailuresBy[sourceKey] >= serverSetupJoinAttempts {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_exhausted", "This code was entered wrongly too often. Create a new code on the first server.", left())
		return
	}
	if time.Now().Unix() >= record.ExpiresAt {
		writeServerSetupJoinRefusal(w, http.StatusGone, "server_setup_join_code_expired", "This code has expired. Create a new code on the first server.", nil)
		return
	}
	if !proofOK {
		record.Attempts++
		if record.FailuresBy == nil {
			record.FailuresBy = map[string]int{}
		}
		record.FailuresBy[sourceKey]++
		if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinCodeKey, record); err != nil {
			writeServerError(w, err)
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusUnauthorized, "server_setup_join_code_wrong", "The code is not correct. Check it on the first server's setup screen.", left())
		return
	}
	// A correct code from a wrong or private address uses up nothing: the
	// owner corrects the address (for example behind 1:1 NAT) and tries again.
	if !sourceOK || !p.serverSetupPeerAddressAllowed(declared) {
		if !sourceOK {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_join_source_mismatch", "The request did not come from the address the second server gave. Join from the second server itself, without a proxy.", left())
			return
		}
		writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_join_address_not_public", "The second server's address is not a public server address.", left())
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
	if _, err := p.serverSetupPanelCertificate(); err != nil {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_join_certificate_unavailable", serverSetupIdentityGuidance, nil)
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
	// A join that replaces an earlier one withdraws what the earlier one
	// handed over: its unused enrollment code, or the publishing client it
	// already made (revoked).
	var previous serverSetupJoinRecord
	replaced, err := readServerSetupJSONSetting(ctx, tx, serverSetupJoinKey, &previous)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if replaced {
		if err := retireServerSetupEnrollment(ctx, tx, previous.EnrollmentHash); err != nil {
			writeServerError(w, err)
			return
		}
	}
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
	if replaced {
		p.audit(r, "server.setup.join_replaced", "", 0)
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
	if cert, err := p.serverSetupPanelCertificate(); err == nil {
		document.FirstSPKI = hex.EncodeToString(serverSetupSPKI(cert))
	}
	join := serverSetupJoinRecord{RequestID: request.RequestID, PlanID: plan.ID, Digest: serverSetupJoinDocumentDigest(document),
		Peer:     serverSetupJoinPeer{IPv4: request.DeclaredIPv4, IPv6: request.DeclaredIPv6, ShortName: request.ShortName, OS: strings.TrimSpace(request.OS)},
		JoinedAt: time.Now().UTC().Format(time.RFC3339), LinkID: linkID, LinkCredentialHash: p.serverSetupMAC("link", credential)}
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

// retireServerSetupEnrollment withdraws an enrollment code an earlier join
// handed over: unused, it is deleted; used, it is deleted and the remote DNS
// publishing client it made is revoked, so nothing the earlier join granted
// stays live.
// Onceki katilmanin verdigi kayit kodu geri alinir; kullanildiysa istemci iptal edilir.
func retireServerSetupEnrollment(ctx context.Context, tx *sql.Tx, hash string) error {
	if hash == "" {
		return nil
	}
	var client string
	err := tx.QueryRowContext(ctx, `SELECT client_id FROM remote_dns_enrollments WHERE code_hash=?`, hash).Scan(&client)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if client != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE remote_dns_clients SET revoked=1 WHERE id=?`, client); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM remote_dns_enrollments WHERE code_hash=?`, hash)
	return err
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
// Only while the join is live: its link not ended (revoked at completion)
// and this server's setup for that plan still running or waiting, so an
// ended join never hands out a credential or an enrollment again.
func (p *Panel) answerServerSetupJoinReplay(w http.ResponseWriter, r *http.Request, request serverSetupJoinRequest) {
	ctx := r.Context()
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		writeServerError(w, err)
		return
	}
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	live := found && join.LinkCredentialHash != "" && execution != nil && execution.PlanID == join.PlanID && stringIn(execution.Status, "running", "waiting")
	plan, err := p.loadServerSetupPlan(ctx, join.PlanID)
	if !live || err != nil || join.RequestID != request.RequestID || join.Peer.IPv4 != request.DeclaredIPv4 {
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
	join.LinkID, join.LinkCredentialHash = fresh.LinkID, fresh.LinkCredentialHash
	if enrollmentUsed {
		// The second server already enrolled with the earlier code.
		document.EnrollmentCode, document.EnrollmentUsed = "", true
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
	return path == serverSetupJoinPath || path == serverSetupPeerNamesPath || path == serverSetupPeerDKIMPath || path == serverSetupPeerReadyPath
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
			// Only a first server receives names.
			if p.authenticateServerSetupLink(w, r, serverSetupLinkRoleFirst) {
				p.handleServerSetupPeerNames(w, r)
			}
		case serverSetupPeerDKIMPath:
			// The main DNS server is either one: the role of this server's
			// own setup says which credentials it accepts.
			if p.authenticateServerSetupLink(w, r, p.serverSetupLinkRole(r.Context())) {
				p.handleServerSetupPeerDKIM(w, r)
			}
		case serverSetupPeerReadyPath:
			// The backup reports to the main DNS server, which is either one.
			if p.authenticateServerSetupLink(w, r, p.serverSetupLinkRole(r.Context())) {
				p.handleServerSetupPeerReady(w, r)
			}
		}
	})
}

// The two credential namespaces of the link. A server can be the second of
// one pair and the first of another; each endpoint accepts only its role's
// credentials and never falls through to the other's.
const (
	// serverSetupLinkRoleFirst: credentials this server issued with a join
	// (the second server calls it).
	serverSetupLinkRoleFirst = "first"
	// serverSetupLinkRoleSecond: the credential derived from this server's
	// own link record (the first server calls it).
	serverSetupLinkRoleSecond = "second"
)

// serverSetupLinkRole is this server's role in its own latest setup: first
// when its plan waits for a second server, second when it joined one, none
// otherwise.
func (p *Panel) serverSetupLinkRole(ctx context.Context) string {
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil || execution == nil {
		return ""
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	switch {
	case err != nil:
		return ""
	case serverSetupPlanHasStep(plan, "peer_join"):
		return serverSetupLinkRoleFirst
	case serverSetupPlanHasStep(plan, "peer_names"):
		return serverSetupLinkRoleSecond
	}
	return ""
}

// serverSetupLinkAuthorized checks a link credential in one role's
// namespace: as first server against the keyed hash it keeps of the join's
// credential, as second against the credential derived from its own link
// credential. Both compares are constant-time; a removed credential (an
// ended link) matches nothing.
func (p *Panel) serverSetupLinkAuthorized(ctx context.Context, role, id, credential string) bool {
	switch role {
	case serverSetupLinkRoleFirst:
		var join serverSetupJoinRecord
		if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil || !found || join.LinkCredentialHash == "" {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(id), []byte(join.LinkID)) == 1 &&
			subtle.ConstantTimeCompare([]byte(p.serverSetupMAC("link", credential)), []byte(join.LinkCredentialHash)) == 1
	case serverSetupLinkRoleSecond:
		link, err := p.readServerSetupLink(ctx)
		if err != nil || link == nil || link.LinkCredential == "" {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(id), []byte(link.LinkID)) == 1 &&
			subtle.ConstantTimeCompare([]byte(credential), []byte(serverSetupReverseCredential(link.LinkCredential))) == 1
	}
	return false
}

func (p *Panel) authenticateServerSetupLink(w http.ResponseWriter, r *http.Request, role string) bool {
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
	if !p.serverSetupLinkAuthorized(r.Context(), role, id, credential) {
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
	if !decodeServerSetupLinkBody(w, r, &names) || refuseServerSetupLinkProtocol(w, names.Protocol) {
		return
	}
	ctx := r.Context()
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
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
	var stored *serverSetupPeerNames
	if join.Names != nil {
		opened, err := p.openServerSetupPeerNames(*join.Names)
		if err != nil {
			writeServerError(w, err)
			return
		}
		stored = &opened
	}
	if stored != nil && !serverSetupPeerNamesEqual(*stored, names) {
		// After a run with this join succeeded there is no path to other
		// names in this release; before, the join is marked changed and the
		// first server's run waits for a new join (Beta-1 P4).
		if finished, err := p.serverSetupSucceededPeerBinding(ctx); err != nil {
			writeServerError(w, err)
			return
		} else if finished != nil && finished.JoinRequestID == join.RequestID {
			writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_names_final", "The first server's setup finished with this server's earlier answers. Changing them after a finished setup is not available in this release; keep the earlier answers in this server's plan.", nil)
			return
		}
		if execution, err := p.latestServerSetupExecution(ctx); err == nil && execution != nil {
			if latest, err := p.loadServerSetupPlan(ctx, execution.PlanID); err == nil {
				if binding, err := p.readServerSetupPeerBinding(ctx, latest, execution.ID); err == nil && binding != nil && binding.JoinRequestID == join.RequestID {
					// A run that already went on with the earlier answers,
					// or one that is not running, keeps them: the earlier
					// refusal, and the join is left as it is.
					unused, unusedErr := p.serverSetupBindingUnused(ctx, *execution)
					if unusedErr != nil {
						writeServerError(w, unusedErr)
						return
					}
					if !unused || !stringIn(execution.Status, "running", "waiting") {
						writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_names_bound", "The first server's setup already uses the second server's earlier names. Keep those names, or reopen the first server's plan.", nil)
						return
					}
					if !join.NamesChanged {
						join.NamesChanged = true
						if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join); err != nil {
							writeServerError(w, err)
							return
						}
						p.audit(r, "server.setup.peer_names_changed", "", 0)
						p.wakeServerSetupRunner()
					}
					writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_names_changed", "The first server already uses this server's earlier answers. Create a new join code on the first server and enter it here; setup then continues with these answers.", nil)
					return
				}
			}
		}
	}
	same := stored != nil && serverSetupPeerNamesEqual(*stored, names)
	if same {
		// The earlier names again: the binding is consistent once more.
		join.NamesChanged = false
	}
	sealed, err := p.sealServerSetupSecret(names.ReverseCredential)
	if err != nil {
		writeServerError(w, err)
		return
	}
	sealedEnrollment, err := p.sealServerSetupSecret(names.EnrollmentCode)
	if err != nil {
		writeServerError(w, err)
		return
	}
	join.PeerCredential = sealed
	join.Names, join.NamesAt = &names, time.Now().UTC().Format(time.RFC3339)
	join.Names.ReverseCredential, join.Names.EnrollmentCode = "", sealedEnrollment
	if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join); err != nil {
		writeServerError(w, err)
		return
	}
	if !same {
		// The same names again (a retry) change nothing and are not audited again.
		p.audit(r, "server.setup.peer_names", "", 0)
	}
	p.wakeServerSetupRunner()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"protocol": serverSetupLinkProtocol, "accepted": true})
}

func serverSetupPeerNamesEqual(a, b serverSetupPeerNames) bool {
	a.RequestID, b.RequestID = "", ""
	a.ReverseCredential, b.ReverseCredential = "", ""
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
	if names.PanelDomain == "" || (names.ShortName != "" && !serverSetupShortNamePattern.MatchString(names.ShortName)) || (names.ReverseCredential != "" && !remoteDNSValidSecret(names.ReverseCredential)) {
		return invalid()
	}
	if names.Mail != nil && (names.Mail.Hostname != names.MailHostname || names.MailHostname == "" || !stringIn(names.Mail.MX, "", "now", "later") || !stringIn(names.Mail.DMARC, "", "keep", "add")) {
		return invalid()
	}
	if _, err := hex.DecodeString(names.Pin); err != nil || len(names.Pin) != 64 {
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
			conflict := "infrastructure_dns_conflict"
			if errors.As(err, &planErr) && planErr.Code != "" {
				conflict = planErr.Code
			}
			return "server_setup_peer_names_clash", "The second server's names cannot be added to the main domain's zone (" + conflict + "). Change them in the second server's plan, or the conflicting record in Domains."
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
	ip, err := netip.ParseAddr(host)
	number, portErr := strconv.Atoi(port)
	// IPv4 only: the join's source check is the declared IPv4 address.
	if err != nil || !ip.Is4() || portErr != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return "", false
	}
	return "https://" + net.JoinHostPort(ip.Unmap().String(), port), true
}
