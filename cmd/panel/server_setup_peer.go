package main

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostname"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The second server's side of the join (alpha.114): connecting with the
// first server's address and code, the private link record, the draft taken
// from the join document, the names step, and the read-only link state.
// Ikinci sunucu tarafi: baglanma, ozel baglanti kaydi, taslak, adlar adimi.

const (
	serverSetupLinkKey        = "server_setup_link"
	serverSetupLinkPendingKey = "server_setup_link_pending"
)

// serverSetupLinkRecord is the second server's private record of its link
// to the first server. The document is kept without its secrets.
type serverSetupLinkRecord struct {
	RequestID      string                  `json:"request_id"`
	Address        string                  `json:"address"`
	Pin            string                  `json:"pin"`
	Document       serverSetupJoinDocument `json:"document"`
	LinkID         string                  `json:"link_id"`
	LinkCredential string                  `json:"link_credential"`
	// CodeMAC is a keyed hash of the code this join was made with: the same
	// code again is the same join; another code is a new join.
	CodeMAC        string `json:"code_mac"`
	EnrollmentCode string `json:"enrollment_code,omitempty"`
	// IssuedEnrollment is the code this server issued for the first server
	// when this server is the main DNS server (sent with its names).
	IssuedEnrollment string `json:"issued_enrollment,omitempty"`
	JoinedAt         string `json:"joined_at"`
}

// serverSetupLinkPending keeps a join whose answer was not received, so the
// next try is the same request (same request_id), never a second join.
type serverSetupLinkPending struct {
	RequestID string `json:"request_id"`
	Address   string `json:"address"`
	Code      string `json:"code"`
	CreatedAt string `json:"created_at"`
}

func (p *Panel) readServerSetupLink(ctx context.Context) (*serverSetupLinkRecord, error) {
	var link serverSetupLinkRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupLinkKey, &link)
	if err != nil || !found {
		return nil, err
	}
	return &link, nil
}

func (l serverSetupLinkRecord) target() serverSetupLinkTarget {
	pin, _ := hex.DecodeString(l.Pin)
	return serverSetupLinkTarget{Address: l.Address, Pin: pin}
}

func (l serverSetupLinkRecord) bearer() string { return l.LinkID + "." + l.LinkCredential }

// serverSetupJoinedDraft is the draft the join document gives the second
// server. The owner's own answers (roles, components, names, mail choice)
// stay; the shared answers come from the first server.
func serverSetupJoinedDraft(d serverSetupDraft, document serverSetupJoinDocument) serverSetupDraft {
	d.MainDomain, d.ServerCount, d.DNSMode = document.MainDomain, 2, setupDNSModeLocal
	d.NS1, d.NS2, d.PeerIP = document.NS1, document.NS2, document.First.IPv4
	if local, ok := canonicalIPv4(serverPrimaryIP()); ok && d.LocalIP == "" {
		d.LocalIP = local
	}
	d.DNSHostingManagement, d.RemoteDNSConnectionID = "", ""
	if document.MainDNSServer == "first" {
		d.DNSRole, d.MainDNSServer, d.PeerNS = transport.DNSPairRoleSecondary, "peer", document.NS1
		d.DNSPublisherEndpoint, d.InfrastructureDNS, d.ImportedRecords, d.PeerMail = document.PublisherEndpoint, nil, nil, nil
		return d
	}
	d.DNSRole, d.MainDNSServer, d.PeerNS, d.DNSPublisherEndpoint = transport.DNSPairRolePrimary, "this", document.NS2, ""
	d.InfrastructureDNS = &serverSetupInfrastructureDNSDraft{Zone: document.MainDomain, PeerPanelDomain: document.First.PanelDomain, PeerMailHostname: document.First.MailHostname}
	d.ImportedRecords, d.PeerMail = nil, nil
	if document.Prepared != nil {
		d.ImportedRecords = document.Prepared.ImportedRecords
	}
	if document.First.MainDomainMail {
		mail := serverSetupPeerMail{Hostname: document.First.MailHostname}
		if document.First.MailDNS != nil {
			mail.MX, mail.DMARC = document.First.MailDNS.MX, document.First.MailDNS.DMARC
		}
		d.PeerMail = &mail
	}
	return d
}

// serverSetupJoinedDraftMatches: the shared answers of a joined second
// server's draft are still the join document's.
func serverSetupJoinedDraftMatches(d serverSetupDraft, link serverSetupLinkRecord) bool {
	want, err := canonicalServerSetupDraft(serverSetupJoinedDraft(d, link.Document))
	if err != nil {
		return false
	}
	return reflect.DeepEqual(want, d)
}

// validServerSetupJoinDocument checks a document from the first server
// before anything of it is kept.
func validServerSetupJoinDocument(document serverSetupJoinDocument, address, requestID string) bool {
	u := strings.TrimPrefix(address, "https://")
	host, _, err := net.SplitHostPort(u)
	if err != nil || document.Version != serverSetupJoinDocumentVersion || document.RequestID != requestID || document.ServerCount != 2 || document.DNSMode != setupDNSModeLocal ||
		!stringIn(document.MainDNSServer, "first", "second") || document.First.IPv4 != host || !validServiceOperationID(document.LinkID) || !remoteDNSValidSecret(document.LinkCredential) {
		return false
	}
	for _, name := range []string{document.MainDomain, document.NS1, document.NS2, document.First.PanelDomain} {
		if canonical, err := hostname.CanonicalFQDN(name); err != nil || canonical != name {
			return false
		}
	}
	if spki, err := hex.DecodeString(document.FirstSPKI); err != nil || len(spki) != 32 {
		return false
	}
	if document.NS1 == document.NS2 || !transport.ValidDNSEngine(transport.DNSEngine(document.First.DNSEngine)) {
		return false
	}
	if document.MainDNSServer == "first" {
		endpoint, err := canonicalRemoteDNSEndpoint(document.PublisherEndpoint)
		codeOK := remoteDNSValidSecret(document.EnrollmentCode) || (document.EnrollmentUsed && document.EnrollmentCode == "")
		if err != nil || endpoint != document.PublisherEndpoint || !codeOK || document.Prepared != nil {
			return false
		}
	} else if document.PublisherEndpoint != "" || document.EnrollmentCode != "" {
		return false
	}
	if document.First.MailHostname != "" {
		if canonical, err := hostname.CanonicalFQDN(document.First.MailHostname); err != nil || canonical != document.First.MailHostname {
			return false
		}
	}
	probe := serverSetupDraft{MainDomain: document.MainDomain, ImportedRecords: nil}
	if document.Prepared != nil {
		probe.ImportedRecords = document.Prepared.ImportedRecords
	}
	return canonicalServerSetupMailAnswers(&probe) == nil
}

type serverSetupPeerConnectAnswer struct {
	State        string            `json:"state"`
	Code         string            `json:"code,omitempty"`
	AttemptsLeft *int              `json:"attempts_left,omitempty"`
	Setup        *serverSetupState `json:"setup,omitempty"`
}

// handleServerSetupPeerConnect joins this server to the first server: the
// address and the code typed on this server. States (D-024): ok, refused
// (a verified answer with its reason), wrong_server (the address answered
// with another key; nothing sent), unreachable (nothing sent; the code is
// unused), unknown (the answer was lost; the next try repeats the same
// request and never joins twice).
func (p *Panel) handleServerSetupPeerConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	var request struct {
		Address string `json:"address"`
		Code    string `json:"code"`
		// PublicIPv4 is the owner's answer when this server's own address is
		// not its public one (1:1 NAT): the join declares it instead.
		PublicIPv4 string `json:"public_ipv4,omitempty"`
	}
	if decodeServiceOperationJSON(w, r, &request) != nil {
		writeClientError(w, http.StatusBadRequest, "invalid join request")
		return
	}
	address, addressOK := canonicalServerSetupPeerAddress(request.Address)
	pin, proof, codeOK := parseServerSetupJoinCode(request.Code)
	if !addressOK || !codeOK {
		writeCodedError(w, http.StatusBadRequest, "server_setup_peer_connect_invalid", "Enter the first server's address (as shown on its setup screen) and the 24-character code.", "")
		return
	}
	ctx := r.Context()
	if active, err := p.serverSetupExecutionActive(ctx); err != nil || active {
		writeCodedError(w, http.StatusConflict, "server_setup_peer_connect_running", "This server's setup is running; join before starting it.", "/setup")
		return
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	canonicalCode := formatServerSetupJoinCode(pin, mustDecodeServerSetupProof(proof))
	// Already joined with this code (a draft that could not be saved, or the
	// same form sent twice): the join is not repeated; the answers it gave
	// are applied again, which changes nothing when they already are. A new
	// code for the same first server (it asked to join again) is a new join
	// that replaces this one.
	if link, err := p.readServerSetupLink(ctx); err != nil {
		writeServerError(w, err)
		return
	} else if link != nil && link.Address == address && serverSetupSameSecret(link.CodeMAC, p.serverSetupLinkCodeMAC(canonicalCode)) {
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey)
		_ = json.NewEncoder(w).Encode(p.applyServerSetupJoinedDraft(ctx, *link))
		return
	}
	var pending serverSetupLinkPending
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupLinkPendingKey, &pending)
	if err != nil {
		writeServerError(w, err)
		return
	}
	pendingCode, openErr := p.openServerSetupSecret(pending.Code)
	if !found || openErr != nil || pending.Address != address || pendingCode != canonicalCode {
		id, err := newServiceOperationID()
		if err != nil {
			writeServerError(w, err)
			return
		}
		sealedCode, err := p.sealServerSetupSecret(canonicalCode)
		if err != nil {
			writeServerError(w, err)
			return
		}
		pending = serverSetupLinkPending{RequestID: id, Address: address, Code: sealedCode, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupLinkPendingKey, pending); err != nil {
			writeServerError(w, err)
			return
		}
	}
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	join := serverSetupJoinRequest{Protocol: serverSetupLinkProtocol, Proof: proof, RequestID: pending.RequestID, ShortName: state.Draft.ShortName, OS: serverSetupHostOS(p)}
	join.DeclaredIPv4, _ = canonicalIPv4(serverPrimaryIP())
	if request.PublicIPv4 != "" {
		typed, err := netip.ParseAddr(strings.TrimSpace(request.PublicIPv4))
		if err != nil || !typed.Is4() || !p.serverSetupPeerAddressAllowed(typed) {
			writeCodedError(w, http.StatusBadRequest, "server_setup_peer_connect_public_ipv4", "Enter this server's public IPv4 address, as the provider shows it.", "")
			return
		}
		join.DeclaredIPv4 = typed.String()
	}
	join.DeclaredIPv6 = serverSetupPublicIPv6()
	var document serverSetupJoinDocument
	var observed []byte
	callErr := p.serverSetupLinkJSON(context.WithoutCancel(ctx), serverSetupLinkTarget{Address: address, Pin: pin, Observed: &observed}, serverSetupJoinPath, "", join, &document)
	if callErr == nil && document.Protocol == serverSetupLinkProtocol && (len(observed) != 32 || document.FirstSPKI != hex.EncodeToString(observed)) {
		// The full fingerprint sent over the pinned channel must be the key
		// that answered (whose prefix the code carries).
		callErr = errServerSetupLinkPinMismatch
	}
	answer := serverSetupPeerConnectAnswer{}
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil && validServerSetupJoinDocument(document, address, pending.RequestID):
		link := serverSetupLinkRecord{RequestID: pending.RequestID, Address: address, Pin: document.FirstSPKI, LinkID: document.LinkID, LinkCredential: document.LinkCredential,
			CodeMAC: p.serverSetupLinkCodeMAC(canonicalCode), EnrollmentCode: document.EnrollmentCode, JoinedAt: time.Now().UTC().Format(time.RFC3339)}
		link.Document = document
		link.Document.LinkCredential, link.Document.EnrollmentCode = "", ""
		tx, err := p.db.GetDB().BeginTx(ctx, nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer tx.Rollback()
		// A new join (another first server, or the same one with a new code)
		// replaces the old link and withdraws the enrollment this server
		// issued for the old join.
		var old serverSetupLinkRecord
		replaced, err := readServerSetupJSONSetting(ctx, tx, serverSetupLinkKey, &old)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if replaced && old.IssuedEnrollment != "" {
			if err := retireServerSetupEnrollment(ctx, tx, remoteDNSHash(old.IssuedEnrollment)); err != nil {
				writeServerError(w, err)
				return
			}
		}
		if err := writeServerSetupJSONSetting(ctx, tx, serverSetupLinkKey, link); err != nil {
			writeServerError(w, err)
			return
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey); err != nil {
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
		p.audit(r, "server.setup.peer_connected", "", 0)
		answer = p.applyServerSetupJoinedDraft(ctx, link)
	case callErr == nil && document.Protocol != serverSetupLinkProtocol:
		// A first server of another version: a known state, not a lost answer.
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey)
		answer.State, answer.Code = "refused", "server_setup_peer_version_incompatible"
	case callErr == nil:
		// An answer that is not a join document for this request.
		answer.State, answer.Code = "unknown", "server_setup_join_unknown"
	case errors.As(callErr, &refusal):
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey)
		answer.State, answer.Code, answer.AttemptsLeft = "refused", refusal.Code, refusal.AttemptsLeft
	case errors.Is(callErr, errServerSetupLinkPinMismatch):
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey)
		answer.State, answer.Code = "wrong_server", "server_setup_join_pin_mismatch"
	case errors.Is(callErr, errServerSetupLinkUnreachable):
		_, _ = p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupLinkPendingKey)
		answer.State, answer.Code = "unreachable", "server_setup_join_unreachable"
	default:
		answer.State, answer.Code = "unknown", "server_setup_join_unknown"
	}
	_ = json.NewEncoder(w).Encode(answer)
}

// serverSetupPeerSaveDraft saves the joined draft; a test makes it fail once.
func (p *Panel) serverSetupPeerSaveDraft(ctx context.Context, revision int, d serverSetupDraft) (serverSetupState, error) {
	if p.setupLink.saveDraft != nil {
		return p.setupLink.saveDraft(ctx, revision, d)
	}
	return p.saveServerSetupDraft(ctx, revision, d)
}

// applyServerSetupJoinedDraft takes the shared answers of the link into the
// draft. When that cannot be saved (D-024: a known join, answers not yet
// applied) the answer says so; sending the form again applies them without
// joining again.
func (p *Panel) applyServerSetupJoinedDraft(ctx context.Context, link serverSetupLinkRecord) serverSetupPeerConnectAnswer {
	state, err := p.loadServerSetup(ctx)
	if err == nil {
		var saved serverSetupState
		if saved, err = p.serverSetupPeerSaveDraft(ctx, state.Revision, serverSetupJoinedDraft(state.Draft, link.Document)); err == nil {
			return serverSetupPeerConnectAnswer{State: "ok", Setup: &saved}
		}
	}
	return serverSetupPeerConnectAnswer{State: "ok", Code: "server_setup_peer_draft_retry"}
}

// serverSetupLinkCodeMAC: the keyed hash of a join code kept with its link.
func (p *Panel) serverSetupLinkCodeMAC(code string) string {
	return p.serverSetupMAC("link-code", code)
}

// serverSetupSameSecret compares two keyed hashes in constant time; an empty
// one never matches.
func serverSetupSameSecret(stored, computed string) bool {
	return stored != "" && subtle.ConstantTimeCompare([]byte(stored), []byte(computed)) == 1
}

func mustDecodeServerSetupProof(proof string) []byte {
	// The proof is the code's last 8 characters: 5 bytes once decoded.
	decoded, err := serverSetupJoinEncoding.DecodeString(proof)
	if err != nil || len(decoded) != 5 {
		return make([]byte, 5)
	}
	return decoded
}

func serverSetupHostOS(p *Panel) string {
	profile := p.managedServiceHostProfile()
	value := strings.TrimSpace(profile.DistroID + " " + profile.VersionID)
	if !serverSetupOSPattern.MatchString(value) {
		return ""
	}
	return value
}

// --- read-only state (both servers) -----------------------------------------

type serverSetupPeerView struct {
	Role    string                   `json:"role"`
	Code    *serverSetupJoinCodeView `json:"code,omitempty"`
	Joined  *serverSetupPeerJoined   `json:"joined,omitempty"`
	Link    *serverSetupPeerLinkView `json:"link,omitempty"`
	Pending bool                     `json:"pending,omitempty"`
}

type serverSetupPeerJoined struct {
	ShortName      string `json:"short_name,omitempty"`
	IPv4           string `json:"ipv4"`
	JoinedAt       string `json:"joined_at"`
	PanelDomain    string `json:"panel_domain,omitempty"`
	MailHostname   string `json:"mail_hostname,omitempty"`
	MainDomainMail bool   `json:"main_domain_mail,omitempty"`
}

type serverSetupPeerLinkView struct {
	Address       string               `json:"address"`
	MainDomain    string               `json:"main_domain"`
	MainDNSServer string               `json:"main_dns_server"`
	NS1           string               `json:"ns1"`
	NS2           string               `json:"ns2"`
	First         serverSetupJoinFirst `json:"first"`
	Imported      int                  `json:"imported"`
	JoinedAt      string               `json:"joined_at"`
}

func (p *Panel) handleServerSetupPeer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	view, err := p.serverSetupPeerState(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}

func (p *Panel) serverSetupPeerState(ctx context.Context) (serverSetupPeerView, error) {
	view := serverSetupPeerView{Role: "none"}
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return view, err
	}
	if link != nil {
		view.Role = "second"
		view.Link = &serverSetupPeerLinkView{Address: link.Address, MainDomain: link.Document.MainDomain, MainDNSServer: link.Document.MainDNSServer, NS1: link.Document.NS1, NS2: link.Document.NS2, First: link.Document.First, JoinedAt: link.JoinedAt}
		view.Link.First.MailDNS = nil
		if link.Document.Prepared != nil {
			view.Link.Imported = len(serverSetupImported(serverSetupDraft{ImportedRecords: link.Document.Prepared.ImportedRecords}))
		}
		return view, nil
	}
	var pending serverSetupLinkPending
	if found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupLinkPendingKey, &pending); err != nil {
		return view, err
	} else if found {
		view.Pending = true
	}
	var code serverSetupJoinCodeRecord
	codeFound, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinCodeKey, &code)
	if err != nil {
		return view, err
	}
	var join serverSetupJoinRecord
	joinFound, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		return view, err
	}
	if codeFound || joinFound {
		view.Role = "first"
	}
	if codeFound {
		state, err := p.loadServerSetup(ctx)
		if err != nil {
			return view, err
		}
		view.Code = &serverSetupJoinCodeView{Address: serverSetupJoinAddress(state.Draft), ExpiresAt: time.Unix(code.ExpiresAt, 0).UTC().Format(time.RFC3339), AttemptsLeft: max(0, serverSetupJoinCodeFailures-code.Attempts), Used: code.ConsumedBy != ""}
	}
	if joinFound {
		view.Joined = &serverSetupPeerJoined{ShortName: join.Peer.ShortName, IPv4: join.Peer.IPv4, JoinedAt: join.JoinedAt}
		if join.Names != nil {
			view.Joined.PanelDomain, view.Joined.MailHostname, view.Joined.MainDomainMail = join.Names.PanelDomain, join.Names.MailHostname, join.Names.Mail != nil
			if join.Names.ShortName != "" {
				view.Joined.ShortName = join.Names.ShortName
			}
		}
	}
	return view, nil
}

// --- the names step (second server) -----------------------------------------

// serverSetupPeerWait is a wait with its own reason (D-024): the step stays
// running and is asked again; nothing is repeated under a new identity.
type serverSetupPeerWait struct{ Code, Message string }

// serverSetupPeerVersionWait: the other panel speaks another link version.
// A known state: the owner updates the older panel from its own update
// screen; setup asks again by itself.
func serverSetupPeerVersionWait() error {
	return &serverSetupPeerWait{Code: "server_setup_peer_version_incompatible", Message: "The two panels run versions that cannot work together. Update the older panel from its own update screen; setup asks again by itself."}
}

func (e *serverSetupPeerWait) Error() string { return e.Code + ": " + e.Message }

// runServerSetupPeerNames sends this server's names to the first server at
// the start of its setup. The request carries the step's own identity, so a
// retry after a lost answer is the same request; the first server accepts
// the same names again.
func (p *Panel) runServerSetupPeerNames(ctx context.Context, plan serverSetupPlan, step serverSetupExecutionStep) (bool, error) {
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return false, err
	}
	if link == nil || step.Target != link.Address {
		return false, &serverSetupChildFailure{Code: "server_setup_peer_link_missing", Message: "This server is no longer joined to the first server. Join it again from the first step of setup, then review the plan."}
	}
	names, err := p.serverSetupOwnNames(ctx, plan.Draft, link, step.RequestID)
	if err != nil {
		return false, err
	}
	var answer struct {
		Protocol int  `json:"protocol"`
		Accepted bool `json:"accepted"`
	}
	callErr := p.serverSetupLinkJSON(ctx, link.target(), serverSetupPeerNamesPath, link.bearer(), names, &answer)
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil && answer.Accepted && answer.Protocol == serverSetupLinkProtocol:
		return true, nil
	case (callErr == nil && answer.Protocol != serverSetupLinkProtocol) || (errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_version_incompatible"):
		return false, serverSetupPeerVersionWait()
	case errors.As(callErr, &refusal):
		return false, &serverSetupChildFailure{Code: refusal.Code, Message: "The first server refused this server's names (" + refusal.Code + "). Change the names in this server's plan, or check the first server's setup."}
	case errors.Is(callErr, errServerSetupLinkUnreachable), errors.Is(callErr, errServerSetupLinkPinMismatch):
		return false, &serverSetupPeerWait{Code: "server_setup_peer_names_unreachable", Message: "The first server cannot be reached to send this server's names. Nothing to do here if it is starting; check that it is running. Setup asks again by itself."}
	default:
		return false, &serverSetupPeerWait{Code: "server_setup_peer_names_unknown", Message: "The first server's answer was not received. Setup sends the same names again; nothing is done twice."}
	}
}

// serverSetupOwnNames: this server's names for the first server, and, when
// this server is the main DNS server, the publishing endpoint and an
// enrollment code for the first server's connection (issued once and kept).
func (p *Panel) serverSetupOwnNames(ctx context.Context, d serverSetupDraft, link *serverSetupLinkRecord, requestID string) (serverSetupPeerNames, error) {
	names := serverSetupPeerNames{Protocol: serverSetupLinkProtocol, ReverseCredential: serverSetupReverseCredential(link.LinkCredential), RequestID: requestID, ShortName: d.ShortName, PanelDomain: d.PanelDomain, DNSEngine: d.DNSEngine}
	local, _ := canonicalIPv4(d.LocalIP)
	names.Address = "https://" + net.JoinHostPort(local, strconv.Itoa(panelPort()))
	cert, err := p.serverSetupPanelCertificate()
	if err != nil {
		return names, &serverSetupPeerWait{Code: "server_setup_join_certificate_unavailable", Message: "This panel's TLS certificate could not be read. Check Settings → Panel certificate; setup asks again by itself."}
	}
	names.Pin = hex.EncodeToString(serverSetupSPKI(cert))
	if serverSetupHasMail(d) {
		names.MailHostname = d.MailHostname
		if !link.Document.First.MainDomainMail && d.DNSRole == transport.DNSPairRoleSecondary {
			mail := serverSetupPeerMail{Hostname: d.MailHostname}
			if d.MailDNS != nil {
				mail.MX, mail.DMARC = d.MailDNS.MX, d.MailDNS.DMARC
			}
			names.Mail = &mail
		}
	}
	if link.Document.MainDNSServer == "second" {
		names.PublisherEndpoint = serverSetupPanelEndpoint(d.PanelDomain)
		if link.IssuedEnrollment == "" {
			code, err := remoteDNSSecret()
			if err != nil {
				return names, err
			}
			tx, err := p.db.GetDB().BeginTx(ctx, nil)
			if err != nil {
				return names, err
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(ctx, `INSERT INTO remote_dns_enrollments(code_hash,expires_at) VALUES(?,?)`, remoteDNSHash(code), time.Now().UTC().Add(serverSetupJoinEnrollmentLifetime).Unix()); err != nil {
				return names, err
			}
			link.IssuedEnrollment = code
			if err := writeServerSetupJSONSetting(ctx, tx, serverSetupLinkKey, link); err != nil {
				return names, err
			}
			if err := tx.Commit(); err != nil {
				return names, err
			}
		}
		names.EnrollmentCode = link.IssuedEnrollment
	}
	return names, nil
}
