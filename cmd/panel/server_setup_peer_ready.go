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

// settleServerSetupLink runs after this server's setup was marked ready. The
// join code always ends with the setup. The link's credentials end now on
// the backup (it reported first), and on the main only once the backup has
// reported or the owner stopped waiting for it.
func (p *Panel) settleServerSetupLink(ctx context.Context, d serverSetupDraft) error {
	if !serverSetupDraftIsMainDNS(d) {
		return p.revokeServerSetupLink(ctx)
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	if _, err := p.db.GetDB().ExecContext(ctx, `DELETE FROM panel_settings WHERE key=?`, serverSetupJoinCodeKey); err != nil {
		return err
	}
	finished, err := p.serverSetupPeerFinished(ctx)
	if err != nil || !finished {
		return err
	}
	return p.revokeServerSetupLink(ctx)
}

// serverSetupPeerFinished: the other server reported ready or was retired,
// on whichever side of the link this server is.
func (p *Panel) serverSetupPeerFinished(ctx context.Context) (bool, error) {
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return false, err
	}
	if link != nil {
		return link.PeerReadyAt != "" || link.PeerRetiredAt != "", nil
	}
	var join serverSetupJoinRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
	if err != nil || !found {
		return !found, err
	}
	return join.PeerReadyAt != "" || join.PeerRetiredAt != "", nil
}

// reportServerSetupReadyToMain: the backup tells the main DNS server, over
// the link, that its setup is ready. A refused credential means the main
// already ended the link (it recorded the report, or its owner stopped
// waiting): nothing more to tell. An unreachable main is a wait (D-024).
func (p *Panel) reportServerSetupReadyToMain(ctx context.Context, d serverSetupDraft) error {
	if !serverSetupDraftIsBackupDNS(d) {
		return nil
	}
	var target serverSetupLinkTarget
	var bearer, joinRequestID string
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		return err
	}
	if link != nil {
		if link.LinkCredential == "" {
			return nil
		}
		target, bearer, joinRequestID = link.target(), link.bearer(), link.RequestID
	} else {
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil {
			return err
		}
		if !found || (join.PeerCredential == "" && join.LinkCredentialHash == "") {
			return nil
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
	}
	var answer struct {
		State string `json:"state"`
	}
	callErr := p.serverSetupLinkJSON(ctx, target, serverSetupPeerReadyPath, bearer, serverSetupPeerReadyRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: joinRequestID}, &answer)
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil:
		return nil
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_link_unauthorized":
		return nil
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_version_incompatible":
		return serverSetupPeerVersionWait()
	case errors.As(callErr, &refusal):
		return &serverSetupPeerWait{Code: "server_setup_peer_ready_refused", Message: "The main DNS server refused this server's report that its setup is ready (" + refusal.Code + "). The owner checks the setup link on the main DNS server (its setup page names the second server); this server reports again by itself."}
	default:
		return &serverSetupPeerWait{Code: "server_setup_peer_ready_unreachable", Message: "Everything on this server is ready, but the main DNS server could not be told so yet (it could not be reached, or its answer was lost). Nothing to do if it is only briefly offline; otherwise check that the main DNS server's panel is running. This server reports again by itself and then finishes."}
	}
}

// handleServerSetupPeerReady (main DNS server, link machine endpoint): the
// backup's report. The same report again is answered the same way.
func (p *Panel) handleServerSetupPeerReady(w http.ResponseWriter, r *http.Request) {
	var request serverSetupPeerReadyRequest
	if !decodeServerSetupLinkBody(w, r, &request) || refuseServerSetupLinkProtocol(w, request.Protocol) {
		return
	}
	if !validServiceOperationID(request.JoinRequestID) {
		writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_link_invalid", "The setup link request is not valid.", nil)
		return
	}
	ctx := r.Context()
	source, sourceKnown := p.serverSetupJoinSourceIP(r)
	otherJoin := func() {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_ready_other_join", "This report names another join than the one this server keeps; it is not recorded.", nil)
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	defer joinLock.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	link, err := p.readServerSetupLink(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if link != nil {
		// The first server calls from wherever it routes; like its DKIM
		// call, the report is bound by the link credential and the join.
		if request.JoinRequestID != link.RequestID {
			otherJoin()
			return
		}
		if link.PeerReadyAt == "" {
			link.PeerReadyAt = now
			if err := p.writeServerSetupLink(ctx, p.db.GetDB(), *link); err != nil {
				writeServerError(w, err)
				return
			}
			p.audit(r, "server.setup.peer_ready", "", 0)
		}
	} else {
		var join serverSetupJoinRecord
		found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join)
		if err != nil {
			writeServerError(w, err)
			return
		}
		// The second server's address was checked at the join; its report
		// comes from that address.
		if !found || request.JoinRequestID != join.RequestID || !sourceKnown || source.String() != join.Peer.IPv4 {
			otherJoin()
			return
		}
		if join.PeerReadyAt == "" {
			join.PeerReadyAt = now
			if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join); err != nil {
				writeServerError(w, err)
				return
			}
			p.audit(r, "server.setup.peer_ready", "", 0)
		}
	}
	// Both are ready once this server's own setup is: its side ends now.
	if state, err := p.loadServerSetup(ctx); err == nil && state.Status == "ready" {
		if err := p.revokeServerSetupLink(ctx); err != nil {
			writeServerError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"protocol": serverSetupLinkProtocol, "state": "recorded"})
}

// handleServerSetupPeerRetire (main DNS server, administrator): stop waiting
// for the second server. Allowed only once this server's setup is ready and
// the second server has not reported; it ends the link at once.
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
	view, err := p.serverSetupPeerState(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if view.PeerSetup != "pending" {
		writeCodedError(w, http.StatusConflict, "server_setup_peer_not_waiting", "This server is not waiting for a second server's setup.", "/setup")
		return
	}
	joinLock := p.serverSetupJoinLock()
	joinLock.Lock()
	now := time.Now().UTC().Format(time.RFC3339)
	err = func() error {
		link, err := p.readServerSetupLink(ctx)
		if err != nil {
			return err
		}
		if link != nil {
			link.PeerRetiredAt = now
			if err := p.writeServerSetupLink(ctx, p.db.GetDB(), *link); err != nil {
				return err
			}
		} else {
			var join serverSetupJoinRecord
			if _, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, &join); err != nil {
				return err
			}
			join.PeerRetiredAt = now
			if err := writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupJoinKey, join); err != nil {
				return err
			}
		}
		return p.revokeServerSetupLink(ctx)
	}()
	joinLock.Unlock()
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "server.setup.peer_retire", "", 0)
	view, err = p.serverSetupPeerState(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

// serverSetupPeerSetupView fills the main DNS server's view of the second
// server's setup: only once this server's own setup is ready.
func (p *Panel) serverSetupPeerSetupView(ctx context.Context, view *serverSetupPeerView, ready, retired string) error {
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		return err
	}
	if state.Status == "ready" && serverSetupDraftIsMainDNS(state.Draft) {
		view.PeerSetup = serverSetupPeerReadyState(ready, retired)
	}
	return nil
}
