package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The setup link outlives the main DNS server's own setup (Beta-1 re-run P7,
// 2026-10-29).
//
// In the run, the main DNS server finished first and ended the link at its
// own completion. The backup's panel certificate step then failed, and its
// retry could never finish: the link was gone (server_setup_peer_link_revoked)
// and a new code was refused (server_setup_join_finished).
//
// Now the main DNS server keeps its side of the link after its own setup is
// ready, until the backup reports over the link that its setup is ready
// (POST /api/v1/setup/peer/ready), or until the owner explicitly stops
// waiting for it on the main (POST /api/v1/setup/peer/retire). Only then is
// the main's side revoked. Every other restriction stays: the report is
// authenticated with the link credential in its role's namespace (HMAC kept
// on the first server), comes from the peer's own address, over the pinned
// key, and names the same join: a report or a retry for another binding is
// refused. The backup reports before it marks itself ready; when the main
// cannot be reached it waits with that reason and reports again by itself.
// Meanwhile the main is ready for its own services, and its peer view says
// the second server has not finished yet (peer_setup "pending").
//
// Ana DNS sunucusu kendi kurulumu bitince bağlantıyı yedek sunucu hazır
// olduğunu bildirene ya da sahip beklemeyi bırakana kadar açık tutar.

const (
	serverSetupPeerReadyPath  = serverSetupPath + "/peer/ready"
	serverSetupPeerRetirePath = serverSetupPath + "/peer/retire"
)

type serverSetupPeerReadyRequest struct {
	Protocol int `json:"protocol"`
	// JoinRequestID is the join both servers share: the report counts only
	// for the binding it names.
	JoinRequestID string `json:"join_request_id"`
}

// serverSetupPeerReadyState is the main DNS server's view of the second
// server's setup once its own setup is ready: "pending", "ready" or
// "retired". Empty when this server is not the main of a pair or its own
// setup is not ready yet.
func serverSetupPeerReadyState(ready, retired string) string {
	switch {
	case retired != "":
		return "retired"
	case ready != "":
		return "ready"
	}
	return "pending"
}


// serverSetupDraftIsMainDNS / Backup: this server's role in its own pair.
func serverSetupDraftIsMainDNS(d serverSetupDraft) bool {
	return d.DNSMode == setupDNSModeLocal && d.DNSRole == transport.DNSPairRolePrimary
}

func serverSetupDraftIsBackupDNS(d serverSetupDraft) bool {
	return d.DNSMode == setupDNSModeLocal && d.DNSRole == transport.DNSPairRoleSecondary
}

// serverSetupOwnLinkRole is this server's side of the link in its own
// setup: first when its latest run joins a second server (peer_join),
// second when it joined a first one (peer_names). A run with neither step (an
// extension) falls back to the one record that exists; both records with no
// run to tell them apart is refused, never guessed (review fix, item 3).
func (p *Panel) serverSetupOwnLinkRole(ctx context.Context) (string, error) {
	execution, err := p.latestServerSetupExecution(ctx)
	if err != nil {
		return "", err
	}
	if execution != nil {
		plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
		if err != nil {
			return "", err
		}
		switch {
		case serverSetupPlanHasStep(plan, "peer_join"):
			return serverSetupLinkRoleFirst, nil
		case serverSetupPlanHasStep(plan, "peer_names"):
			return serverSetupLinkRoleSecond, nil
		}
	}
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return "", err
	}
	var join serverSetupJoinRecord
	joinFound, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil {
		return "", err
	}
	switch {
	case link != nil && joinFound:
		return "", errors.New("this server keeps both sides of setup links and its setup names neither")
	case link != nil:
		return serverSetupLinkRoleSecond, nil
	case joinFound:
		return serverSetupLinkRoleFirst, nil
	}
	return "", nil
}

// serverSetupPeerRecord is one role's view of its link: whether it exists,
// whether its credential is live, and the other server's report or the
// owner's choice.
type serverSetupPeerRecord struct {
	Found, Live          bool
	ReadyAt, RetiredAt   string
	JoinRequestID        string
	PeerSource           string
	checkSourceOnReports bool
}

func (p *Panel) readServerSetupPeerRecord(ctx context.Context, role string) (serverSetupPeerRecord, error) {
	switch role {
	case serverSetupLinkRoleFirst:
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil || !found {
			return serverSetupPeerRecord{}, err
		}
		return serverSetupPeerRecord{Found: true, Live: join.LinkCredentialHash != "" || join.PeerCredential != "", ReadyAt: join.PeerReadyAt, RetiredAt: join.PeerRetiredAt,
			JoinRequestID: join.RequestID, PeerSource: join.Peer.IPv4, checkSourceOnReports: true}, nil
	case serverSetupLinkRoleSecond:
		link, err := p.readServerSetupLink(ctx)
		if err != nil || link == nil {
			return serverSetupPeerRecord{}, err
		}
		return serverSetupPeerRecord{Found: true, Live: link.LinkCredential != "", ReadyAt: link.PeerReadyAt, RetiredAt: link.PeerRetiredAt, JoinRequestID: link.RequestID}, nil
	}
	return serverSetupPeerRecord{}, nil
}

// markServerSetupPeerRecord sets the report or the owner's choice on one
// role's record (the caller holds the join lock).
func (p *Panel) markServerSetupPeerRecord(ctx context.Context, role string, ready bool, at string) error {
	switch role {
	case serverSetupLinkRoleFirst:
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil {
			return err
		}
		if !found {
			return errServerSetupPeerRecordMissing
		}
		if ready {
			join.PeerReadyAt = at
		} else {
			join.PeerRetiredAt = at
		}
		return writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join)
	case serverSetupLinkRoleSecond:
		link, err := p.readServerSetupLink(ctx)
		if err != nil {
			return err
		}
		if link == nil {
			return errServerSetupPeerRecordMissing
		}
		if ready {
			link.PeerReadyAt = at
		} else {
			link.PeerRetiredAt = at
		}
		return p.writeServerSetupLink(ctx, p.db.GetDB(), *link)
	}
	return errServerSetupPeerRecordMissing
}

var errServerSetupPeerRecordMissing = errors.New("this server keeps no setup link for that role")

// settleServerSetupLink runs after this server's setup was marked ready. The
// join code always ends with the setup. On the backup its own side ends (it
// has reported). On the main it ends once the backup has reported or the
// owner stopped waiting, and then whenever settle runs again (idempotent).
func (p *Panel) settleServerSetupLink(ctx context.Context, d serverSetupDraft) error {
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupJoinCodeKey); err != nil {
		return err
	}
	role, err := p.serverSetupOwnLinkRole(ctx)
	if err != nil || role == "" {
		return err
	}
	if serverSetupDraftIsMainDNS(d) {
		record, err := p.readServerSetupPeerRecord(ctx, role)
		if err != nil || (record.ReadyAt == "" && record.RetiredAt == "") {
			return err
		}
	}
	return p.revokeServerSetupLinkSide(ctx, role)
}

// serverSetupPeerReadyOutcome is what the backup does with the main's answer
// to its report.
const (
	serverSetupPeerReadyRefusedMessage  = "The main DNS server did not accept this server's report that its setup is ready, although this server is ready. You, the owner: on the main DNS server open setup; if it still waits for this server, choose \"Stop waiting for the second server\" there. This server reports again by itself."
	serverSetupPeerReadyIdentityMessage = "The main DNS server answered with another identity (TLS key) than the one this server joined, so nothing was sent. This is a verified state, not a network problem. You, the owner: on the main DNS server create a new join code and join this server again."
)

// reportServerSetupReadyToMain: the backup tells the main DNS server, over
// the link, that its setup is ready. Only an acknowledgement or the main's
// authenticated "this link ended" counts as told; a bare refusal is a wait
// for the owner, never taken for the end of the link (review fix, item 1).
func (p *Panel) reportServerSetupReadyToMain(ctx context.Context, d serverSetupDraft) error {
	if !serverSetupDraftIsBackupDNS(d) {
		return nil
	}
	role, err := p.serverSetupOwnLinkRole(ctx)
	if err != nil {
		return err
	}
	var target serverSetupLinkTarget
	var bearer, joinRequestID string
	switch role {
	case serverSetupLinkRoleSecond:
		link, err := p.readServerSetupLink(ctx)
		if err != nil || link == nil || link.LinkCredential == "" {
			return err
		}
		target, bearer, joinRequestID = link.target(), link.bearer(), link.RequestID
	case serverSetupLinkRoleFirst:
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil || !found || join.PeerCredential == "" {
			return err
		}
		execution, err := p.latestServerSetupExecution(ctx)
		if err != nil || execution == nil {
			return err
		}
		plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
		if err != nil {
			return err
		}
		target, bearer, err = p.serverSetupPeerTarget(ctx, plan, execution.ID)
		if errors.Is(err, errServerSetupPeerLinkRevoked) {
			return nil
		}
		if err != nil {
			return err
		}
		joinRequestID = join.RequestID
	default:
		return nil
	}
	var answer struct {
		State string `json:"state"`
	}
	callErr := p.serverSetupLinkJSON(ctx, target, serverSetupPeerReadyPath, bearer, serverSetupPeerReadyRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: joinRequestID}, &answer)
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil:
		return nil
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_link_ended":
		return nil
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_version_incompatible":
		return serverSetupPeerVersionWait()
	case errors.As(callErr, &refusal):
		return &serverSetupPeerWait{Code: "server_setup_peer_ready_refused", Message: serverSetupPeerReadyRefusedMessage}
	case errors.Is(callErr, errServerSetupLinkPinMismatch):
		return &serverSetupPeerWait{Code: "server_setup_peer_ready_identity_changed", Message: serverSetupPeerReadyIdentityMessage}
	default:
		return &serverSetupPeerWait{Code: "server_setup_peer_ready_unreachable", Message: "This server is ready, but the main DNS server could not be told so yet (it could not be reached, or its answer was lost). Nothing to do if it is only briefly offline; otherwise check that the main DNS server's panel is running. This server reports again by itself."}
	}
}

// handleServerSetupPeerReadyLink authenticates the report in whichever
// namespace its credential is live, and answers an ended link with its own
// code for exactly the credential that link accepted, from its joined
// address. Anything else is the ordinary refusal.
func (p *Panel) handleServerSetupPeerReadyLink(w http.ResponseWriter, r *http.Request) {
	id, credential, ok := serverSetupLinkBearer(r)
	if !ok {
		writeServerSetupLinkUnauthorized(w)
		return
	}
	role := p.serverSetupLinkAuthenticatedRole(r.Context(), id, credential)
	if role == "" {
		if p.serverSetupLinkEnded(r.Context(), r, id, credential) {
			writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_link_ended", "This server already ended the setup link (the second server reported ready, or the owner stopped waiting for it).", nil)
			return
		}
		writeServerSetupLinkUnauthorized(w)
		return
	}
	p.handleServerSetupPeerReady(w, r, role)
}

// handleServerSetupPeerReady (main DNS server): the backup's report, on the
// record of the role its credential authenticated in. The same report again
// is answered the same way.
func (p *Panel) handleServerSetupPeerReady(w http.ResponseWriter, r *http.Request, role string) {
	var request serverSetupPeerReadyRequest
	if !decodeServerSetupLinkBody(w, r, &request) || refuseServerSetupLinkProtocol(w, request.Protocol) {
		return
	}
	if !validServiceOperationID(request.JoinRequestID) {
		writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_link_invalid", "The setup link request is not valid.", nil)
		return
	}
	ctx := r.Context()
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	record, err := p.readServerSetupPeerRecord(ctx, role)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !record.Found || request.JoinRequestID != record.JoinRequestID {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_ready_other_join", "This report names another join than the one this server keeps; it is not recorded.", nil)
		return
	}
	// The second server's address was checked at the join; its report comes
	// from that address. (The first server, calling the second, routes from
	// wherever it does; like its DKIM call it is bound by credential and join.)
	if record.checkSourceOnReports {
		if source, known := p.serverSetupJoinSourceIP(r); !known || source.String() != record.PeerSource {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_peer_ready_source", "This report does not come from the second server's joined address; it is not recorded.", nil)
			return
		}
	}
	if record.ReadyAt == "" {
		if err := p.markServerSetupPeerRecord(ctx, role, true, time.Now().UTC().Format(time.RFC3339)); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "server.setup.peer_ready", "", 0)
	}
	// Both are ready once this server's own setup is: its side ends now. A
	// state that cannot be read is answered as an error, so the backup
	// reports again; the link is never left open behind a "recorded".
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.Status == "ready" {
		if err := p.revokeServerSetupLinkSide(ctx, role); err != nil {
			writeServerError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"protocol": serverSetupLinkProtocol, "state": "recorded"})
}

// handleServerSetupPeerRetire (main DNS server, administrator): stop waiting
// for the second server. Decided under the join lock on this server's own
// role's record: a missing record is refused, an already reported or retired
// link only ends what is left of it.
func (p *Panel) handleServerSetupPeerRetire(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !serverSetupAdmin(w, r, http.MethodPost) {
		return
	}
	var request struct {
		Confirmed bool `json:"confirmed"`
	}
	if decodeServiceOperationJSON(w, r, &request) != nil || !request.Confirmed {
		writeClientError(w, http.StatusBadRequest, "confirm that this server stops waiting for the second server")
		return
	}
	ctx := r.Context()
	notWaiting := func() {
		writeCodedError(w, http.StatusConflict, "server_setup_peer_not_waiting", "This server is not waiting for a second server's setup.", "/setup")
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	retired, err := func() (bool, error) {
		state, err := p.loadServerSetup(ctx)
		if err != nil {
			return false, err
		}
		role, err := p.serverSetupOwnLinkRole(ctx)
		if err != nil {
			return false, err
		}
		record, err := p.readServerSetupPeerRecord(ctx, role)
		if err != nil {
			return false, err
		}
		if role == "" || !record.Found || state.Status != "ready" || !serverSetupDraftIsMainDNS(state.Draft) {
			return false, errServerSetupPeerRecordMissing
		}
		if record.ReadyAt != "" || record.RetiredAt != "" {
			return false, p.revokeServerSetupLinkSide(ctx, role)
		}
		if !record.Live {
			return false, errServerSetupPeerRecordMissing
		}
		if err := p.markServerSetupPeerRecord(ctx, role, false, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return false, err
		}
		return true, p.revokeServerSetupLinkSide(ctx, role)
	}()
	joinLock.Unlock()
	if errors.Is(err, errServerSetupPeerRecordMissing) {
		notWaiting()
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if retired {
		p.audit(r, "server.setup.peer_retire", "", 0)
	}
	view, err := p.serverSetupPeerState(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

// serverSetupPeerSetupView fills the main DNS server's view of the second
// server's setup, from this server's own role's record, once its own setup
// is ready. A role that cannot be told leaves it empty.
func (p *Panel) serverSetupPeerSetupView(ctx context.Context, view *serverSetupPeerView) error {
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		return err
	}
	if state.Status != "ready" {
		return nil
	}
	role, err := p.serverSetupOwnLinkRole(ctx)
	if err != nil || role == "" {
		return nil
	}
	if serverSetupDraftIsBackupDNS(state.Draft) {
		// Ready here while the main was not told yet: its side is still live.
		if record, err := p.readServerSetupPeerRecord(ctx, role); err == nil && record.Found && record.Live {
			view.PeerSetup = "reporting"
		}
		return nil
	}
	if !serverSetupDraftIsMainDNS(state.Draft) {
		return nil
	}
	record, err := p.readServerSetupPeerRecord(ctx, role)
	if err != nil {
		return err
	}
	if record.Found && (record.Live || record.ReadyAt != "" || record.RetiredAt != "") {
		view.PeerSetup = serverSetupPeerReadyState(record.ReadyAt, record.RetiredAt)
	}
	return nil
}
