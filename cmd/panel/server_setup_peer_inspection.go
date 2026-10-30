package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Automatic deletion-check setup on a pair joined by code (2026-10-30).
//
// When a domain is deleted on a pair, the main DNS server proves that the
// backup removed it too through a read-only SSH inspector on the backup (the
// deliberate native proof of docs/DNS-RECOVERY-ACCEPTANCE.md row 17). Until
// now that channel existed only after the server owners ran the dns-peer-enroll
// owner tool by hand on both servers (four steps), and without it the first
// deletion stayed pending and held every later DNS change.
//
// The backup now runs those four steps as part of its setup completion,
// before it reports ready to the main, over the private setup link the join
// already has: the main's Agent prepares a key, its public key comes back,
// the backup's Agent installs the channel for it and returns its SSH host-key
// digest, and the main's Agent activates the enrollment pinned to that digest
// and checks once that it can reach the channel. Each Agent writes exactly the
// owner tool's native files, so the owner can revoke them without the panel.
// Only public material crosses the link. An owner's own enrollment or SSH
// configuration is reported and kept; a check the owner revoked is never set
// up again.
//
// A wait (the main is unreachable, its DNS pair is not ready yet, SSH is not
// on port 22, the main cannot reach the backup's SSH) keeps the link open and
// resumes by itself; a final outcome is shown on both setup pages.
//
// Silme denetimi kurulumu: sahip aracinin dort adimi, kurulum baglantisi
// uzerinden otomatik; sahibin kendi kaydi ve SSH ayarlari korunur.

const (
	serverSetupPeerInspectionPath = serverSetupPath + "/peer/inspection"
	serverSetupPeerInspectionKey  = "server_setup_peer_inspection"
	// A refused native step is tried again at most this often while the
	// owner may be fixing it (it never runs once per runner pass).
	serverSetupPeerInspectionRetryInterval = 2 * time.Minute
)

// Final outcomes (the record's Outcome; the view's code adds a prefix).
const (
	peerInspectionOutcomeReady         = "ready"
	peerInspectionOutcomeExisting      = "existing"
	peerInspectionOutcomeOwnerConflict = "owner_conflict"
	peerInspectionOutcomeRevoked       = "revoked"
	peerInspectionOutcomeInspector     = "inspector_unavailable"
	peerInspectionOutcomeUnsupported   = "unsupported"
	peerInspectionOutcomeLinkEnded     = "link_ended"
	peerInspectionOutcomeNotApplicable = "not_applicable"
)

// serverSetupPeerInspectionRecord is each server's own record of the
// automation for its current join. Only public values are kept: a credential
// identifier (a file name stem), public keys and digests.
type serverSetupPeerInspectionRecord struct {
	JoinRequestID string `json:"join_request_id"`
	Engine        string `json:"engine,omitempty"`
	CredentialID  string `json:"credential_id,omitempty"`
	PublicKey     string `json:"public_key,omitempty"`
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
	MainIP        string `json:"main_ip,omitempty"`
	BackupIP      string `json:"backup_ip,omitempty"`
	// Outcome is final; empty while the steps run or wait.
	Outcome string `json:"outcome,omitempty"`
	// Side is the server an owner_conflict or failure was found on.
	Side   string `json:"side,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Wait is the current wait code while not final.
	Wait       string `json:"wait,omitempty"`
	Probe      string `json:"probe,omitempty"`
	SSHPorts   []int  `json:"ssh_ports,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

func (p *Panel) readServerSetupPeerInspection(ctx context.Context) (serverSetupPeerInspectionRecord, bool, error) {
	var record serverSetupPeerInspectionRecord
	found, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupPeerInspectionKey, &record)
	return record, found, err
}

func (p *Panel) writeServerSetupPeerInspection(ctx context.Context, record serverSetupPeerInspectionRecord) error {
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return writeServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupPeerInspectionKey, record)
}

// serverSetupPeerInspectionAgent is the Agent call the automation makes; a
// test sets serverSetupPeerInspectionCall on its own panel.
func (p *Panel) serverSetupPeerInspectionAgent(ctx context.Context, method string, args, reply any) error {
	if p.serverSetupPeerInspectionCall != nil {
		return p.serverSetupPeerInspectionCall(ctx, method, args, reply)
	}
	return p.callAgentContext(ctx, method, args, reply)
}

// serverSetupPeerInspectionAgentLacks: this server's Agent does not offer the
// automation (an older Agent, or a platform without the native channel). A
// known state, not a wait.
func serverSetupPeerInspectionAgentLacks(err error, method string) bool {
	return agentRPCMethodUnavailable(err, method) || errors.Is(err, errAgentRPCPlatformCapabilityDenied)
}

// One step at a time on each side. The two sides have their own locks: in a
// process that serves both (tests), the backup's held lock must never block
// the main's handler it is waiting for.
var serverSetupPeerInspectionMainMu, serverSetupPeerInspectionBackupMu sync.Mutex

// --- the main DNS server's side --------------------------------------------

type serverSetupPeerInspectionRequest struct {
	Protocol      int    `json:"protocol"`
	JoinRequestID string `json:"join_request_id"`
	Action        string `json:"action"`
	Engine        string `json:"engine"`
	CredentialID  string `json:"credential_id,omitempty"`
	HostKeySHA256 string `json:"host_key_sha256,omitempty"`
}

type serverSetupPeerInspectionAnswer struct {
	Protocol     int    `json:"protocol"`
	State        string `json:"state"`
	CredentialID string `json:"credential_id,omitempty"`
	PublicKey    string `json:"public_key,omitempty"`
	Probe        string `json:"probe,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

const serverSetupPeerInspectionMainAgentMessage = "The main DNS server's CelikPanel Agent did not answer the deletion-check setup."

// handleServerSetupPeerInspection (main DNS server): one of the owner tool's
// primary steps for the backup that joined this server, on the record of the
// role its credential authenticated in. The same request again is answered
// the same way (the Agent's steps are idempotent for this pair).
func (p *Panel) handleServerSetupPeerInspection(w http.ResponseWriter, r *http.Request, role string) {
	var request serverSetupPeerInspectionRequest
	if !decodeServerSetupLinkBody(w, r, &request) || refuseServerSetupLinkProtocol(w, request.Protocol) {
		return
	}
	if !validServiceOperationID(request.JoinRequestID) ||
		(request.Action != transport.DNSPeerInspectionActionPrepare && request.Action != transport.DNSPeerInspectionActionActivate) ||
		!transport.ValidDNSEngine(transport.DNSEngine(request.Engine)) ||
		len(request.CredentialID) > 64 || len(request.HostKeySHA256) > 64 {
		writeServerSetupJoinRefusal(w, http.StatusBadRequest, "server_setup_link_invalid", "The setup link request is not valid.", nil)
		return
	}
	ctx := r.Context()
	record, err := p.readServerSetupPeerRecord(ctx, role)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !record.Found || request.JoinRequestID != record.JoinRequestID {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_ready_other_join", "This request names another join than the one this server keeps.", nil)
		return
	}
	if record.checkSourceOnReports {
		if source, known := p.serverSetupJoinSourceIP(r); !known || source.String() != record.PeerSource {
			writeServerSetupJoinRefusal(w, http.StatusForbidden, "server_setup_peer_ready_source", "This request does not come from the second server's joined address.", nil)
			return
		}
	}
	state, err := p.loadServerSetup(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !serverSetupDraftIsMainDNS(state.Draft) {
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_inspection_not_main", "This server is not the main DNS server of this pair.", nil)
		return
	}
	serverSetupPeerInspectionMainMu.Lock()
	defer serverSetupPeerInspectionMainMu.Unlock()
	method := "Agent.DNSPeerInspectionPrimaryV1"
	var reply transport.DNSPeerInspectionPrimaryResponse
	callErr := p.serverSetupPeerInspectionAgent(ctx, method, &transport.DNSPeerInspectionPrimaryRequest{
		Action: request.Action, Engine: request.Engine, CredentialID: request.CredentialID, HostKeySHA256: request.HostKeySHA256,
	}, &reply)
	switch {
	case callErr != nil && serverSetupPeerInspectionAgentLacks(callErr, method):
		reply = transport.DNSPeerInspectionPrimaryResponse{State: peerInspectionOutcomeUnsupported}
	case callErr != nil:
		log.Printf("server setup peer inspection: the Agent did not answer %s: %v", request.Action, callErr)
		writeServerSetupJoinRefusal(w, http.StatusConflict, "server_setup_peer_inspection_main_agent", serverSetupPeerInspectionMainAgentMessage, nil)
		return
	}
	// The main keeps its own record for its setup page.
	own, _, err := p.readServerSetupPeerInspection(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if own.JoinRequestID != request.JoinRequestID {
		own = serverSetupPeerInspectionRecord{JoinRequestID: request.JoinRequestID}
	}
	own.Engine, own.MainIP, own.BackupIP = request.Engine, reply.LocalIP, reply.PeerIP
	if reply.CredentialID != "" {
		own.CredentialID = reply.CredentialID
	}
	own.Outcome, own.Side, own.Reason, own.Wait, own.Probe = "", "", "", "", reply.Probe
	switch reply.State {
	case transport.DNSPeerInspectionActive:
		if reply.Probe == "reachable" {
			own.Outcome = peerInspectionOutcomeReady
		} else {
			own.Wait = "server_setup_peer_inspection_ssh_unreachable"
		}
	case transport.DNSPeerInspectionExisting:
		own.Outcome = peerInspectionOutcomeExisting
	case transport.DNSPeerInspectionOwnerConflict:
		own.Outcome, own.Side, own.Reason = peerInspectionOutcomeOwnerConflict, "main", reply.Reason
	case transport.DNSPeerInspectionFailed:
		own.Wait, own.Side, own.Reason = "server_setup_peer_inspection_failed", "main", reply.Reason
	case transport.DNSPeerInspectionPairNotReady:
		own.Wait = "server_setup_peer_inspection_main_pair"
	case peerInspectionOutcomeUnsupported, transport.DNSPeerInspectionNotApplicable:
		own.Outcome = peerInspectionOutcomeUnsupported
	}
	if err := p.writeServerSetupPeerInspection(ctx, own); err != nil {
		writeServerError(w, err)
		return
	}
	if request.Action == transport.DNSPeerInspectionActionPrepare {
		p.audit(r, "server.setup.peer_inspection_prepare", "", 0)
	} else {
		p.audit(r, "server.setup.peer_inspection_activate", "", 0)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(serverSetupPeerInspectionAnswer{Protocol: serverSetupLinkProtocol, State: reply.State,
		CredentialID: reply.CredentialID, PublicKey: reply.PublicKey, Probe: reply.Probe, Reason: reply.Reason})
}

// --- the backup DNS server's side ------------------------------------------

// enrollServerSetupPeerInspection runs on the backup after its own setup is
// ready and before it reports ready to the main (the link is still open). It
// returns a wait while the steps cannot finish yet; a final outcome (set up,
// kept owner state, unsupported) returns nil so the report goes on.
func (p *Panel) enrollServerSetupPeerInspection(ctx context.Context, d serverSetupDraft) error {
	if !serverSetupDraftIsBackupDNS(d) {
		return nil
	}
	target, bearer, joinRequestID, live, err := p.serverSetupLinkToMain(ctx)
	if err != nil || !live {
		return err
	}
	serverSetupPeerInspectionBackupMu.Lock()
	defer serverSetupPeerInspectionBackupMu.Unlock()
	record, _, err := p.readServerSetupPeerInspection(ctx)
	if err != nil {
		return err
	}
	if record.JoinRequestID != joinRequestID {
		record = serverSetupPeerInspectionRecord{JoinRequestID: joinRequestID}
	}
	if record.Outcome != "" {
		return nil
	}
	if record.RetryAfter != "" {
		if after, parseErr := time.Parse(time.RFC3339, record.RetryAfter); parseErr == nil && time.Now().Before(after) {
			return serverSetupPeerInspectionWait(record)
		}
	}
	wait := p.runServerSetupPeerInspection(ctx, &record, target, bearer)
	record.RetryAfter = ""
	if wait != nil && record.Wait == "server_setup_peer_inspection_failed" {
		record.RetryAfter = time.Now().Add(serverSetupPeerInspectionRetryInterval).UTC().Format(time.RFC3339)
	}
	if wait == nil {
		record.Wait = ""
	}
	if err := p.writeServerSetupPeerInspection(ctx, record); err != nil {
		return err
	}
	if wait != nil {
		return wait
	}
	return nil
}

// runServerSetupPeerInspection performs the next steps and leaves either a
// final outcome or a wait in the record.
func (p *Panel) runServerSetupPeerInspection(ctx context.Context, record *serverSetupPeerInspectionRecord, target serverSetupLinkTarget, bearer string) error {
	final := func(outcome, side, reason string) error {
		record.Outcome, record.Side, record.Reason, record.Wait = outcome, side, reason, ""
		return nil
	}
	wait := func(code, side, reason string) error {
		record.Wait, record.Side, record.Reason = code, side, reason
		return serverSetupPeerInspectionWait(*record)
	}
	// 1. This server can host the channel (read only).
	check, err := p.serverSetupPeerInspectionSecondary(ctx, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionCheck})
	if err != nil {
		return wait("server_setup_peer_inspection_agent", "backup", "")
	}
	record.Engine = check.Engine
	if check.PrimaryIP != "" {
		record.MainIP, record.BackupIP = check.PrimaryIP, check.LocalIP
	}
	switch check.State {
	case transport.DNSPeerInspectionReady, transport.DNSPeerInspectionInstalled, transport.DNSPeerInspectionExisting:
	case transport.DNSPeerInspectionPairNotReady:
		return wait("server_setup_peer_inspection_backup_pair", "backup", "")
	case transport.DNSPeerInspectionSSHPort:
		record.SSHPorts = check.SSHPorts
		return wait("server_setup_peer_inspection_ssh_port", "backup", "")
	case transport.DNSPeerInspectionFailed:
		return wait("server_setup_peer_inspection_failed", "backup", check.Reason)
	case transport.DNSPeerInspectionOwnerConflict:
		return final(peerInspectionOutcomeOwnerConflict, "backup", check.Reason)
	case transport.DNSPeerInspectionRevoked:
		return final(peerInspectionOutcomeRevoked, "backup", "")
	case transport.DNSPeerInspectionInspectorUnavailable:
		return final(peerInspectionOutcomeInspector, "backup", "")
	case peerInspectionOutcomeUnsupported:
		return final(peerInspectionOutcomeUnsupported, "backup", "")
	default:
		// This server's Agent does not see itself as the backup of a managed
		// pair (for example a receipt older than pair roles): said, never silent.
		return final(peerInspectionOutcomeUnsupported, "backup", "")
	}
	// 2. The main prepares (or returns its earlier prepared key).
	prepared, done, waitErr := p.serverSetupPeerInspectionMain(ctx, record, target, bearer, transport.DNSPeerInspectionActionPrepare)
	if waitErr != nil || done {
		return waitErr
	}
	if prepared.State != transport.DNSPeerInspectionPrepared || prepared.PublicKey == "" || prepared.CredentialID == "" {
		return final(peerInspectionOutcomeUnsupported, "main", "")
	}
	record.CredentialID, record.PublicKey = prepared.CredentialID, prepared.PublicKey
	// 3. This server installs the channel for that key.
	installed, err := p.serverSetupPeerInspectionSecondary(ctx, transport.DNSPeerInspectionSecondaryRequest{Action: transport.DNSPeerInspectionActionInstall, PublicKey: prepared.PublicKey})
	if err != nil {
		return wait("server_setup_peer_inspection_agent", "backup", "")
	}
	switch installed.State {
	case transport.DNSPeerInspectionInstalled:
		record.HostKeySHA256 = installed.HostKeySHA256
	case transport.DNSPeerInspectionSSHPort:
		record.SSHPorts = installed.SSHPorts
		return wait("server_setup_peer_inspection_ssh_port", "backup", "")
	case transport.DNSPeerInspectionFailed:
		return wait("server_setup_peer_inspection_failed", "backup", installed.Reason)
	case transport.DNSPeerInspectionOwnerConflict:
		return final(peerInspectionOutcomeOwnerConflict, "backup", installed.Reason)
	case transport.DNSPeerInspectionRevoked:
		return final(peerInspectionOutcomeRevoked, "backup", "")
	case transport.DNSPeerInspectionInspectorUnavailable:
		return final(peerInspectionOutcomeInspector, "backup", "")
	default:
		// This server's Agent does not see itself as the backup of a managed
		// pair (for example a receipt older than pair roles): said, never silent.
		return final(peerInspectionOutcomeUnsupported, "backup", "")
	}
	// 4. The main activates, pinned to this server's host key.
	if _, done, waitErr := p.serverSetupPeerInspectionMain(ctx, record, target, bearer, transport.DNSPeerInspectionActionActivate); waitErr != nil || done {
		return waitErr
	}
	return final(peerInspectionOutcomeUnsupported, "main", "")
}

// serverSetupPeerInspectionSecondary asks this server's own Agent. An Agent
// without the automation answers "unsupported".
func (p *Panel) serverSetupPeerInspectionSecondary(ctx context.Context, request transport.DNSPeerInspectionSecondaryRequest) (transport.DNSPeerInspectionSecondaryResponse, error) {
	method := "Agent.DNSPeerInspectionSecondaryV1"
	var reply transport.DNSPeerInspectionSecondaryResponse
	if err := p.serverSetupPeerInspectionAgent(ctx, method, &request, &reply); err != nil {
		if serverSetupPeerInspectionAgentLacks(err, method) {
			return transport.DNSPeerInspectionSecondaryResponse{State: peerInspectionOutcomeUnsupported}, nil
		}
		log.Printf("server setup peer inspection: this server's Agent did not answer %s: %v", request.Action, err)
		return reply, err
	}
	return reply, nil
}

// serverSetupPeerInspectionMain sends one step to the main. It returns the
// answer to go on with, or a wait, or done when the record has a final
// outcome (or a probe wait) from this answer.
func (p *Panel) serverSetupPeerInspectionMain(ctx context.Context, record *serverSetupPeerInspectionRecord, target serverSetupLinkTarget, bearer, action string) (serverSetupPeerInspectionAnswer, bool, error) {
	var answer serverSetupPeerInspectionAnswer
	request := serverSetupPeerInspectionRequest{Protocol: serverSetupLinkProtocol, JoinRequestID: record.JoinRequestID, Action: action,
		Engine: record.Engine, CredentialID: record.CredentialID}
	if action == transport.DNSPeerInspectionActionActivate {
		request.HostKeySHA256 = record.HostKeySHA256
	}
	finalize := func(outcome, side, reason string) (serverSetupPeerInspectionAnswer, bool, error) {
		record.Outcome, record.Side, record.Reason, record.Wait = outcome, side, reason, ""
		return answer, true, nil
	}
	waitFor := func(code, side, reason string) (serverSetupPeerInspectionAnswer, bool, error) {
		record.Wait, record.Side, record.Reason = code, side, reason
		return answer, true, serverSetupPeerInspectionWait(*record)
	}
	callErr := p.serverSetupLinkJSON(ctx, target, serverSetupPeerInspectionPath, bearer, request, &answer)
	var refusal *serverSetupLinkRefusal
	switch {
	case callErr == nil && answer.Protocol != serverSetupLinkProtocol:
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	case callErr == nil:
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_version_incompatible":
		// An older main without this step: the owner tool remains.
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_link_ended":
		return finalize(peerInspectionOutcomeLinkEnded, "main", "")
	case errors.As(callErr, &refusal) && refusal.Code == "server_setup_peer_inspection_main_agent":
		return waitFor("server_setup_peer_inspection_main_agent", "main", "")
	case errors.As(callErr, &refusal):
		// A verified refusal is the ready report's to explain (it follows).
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	case errors.Is(callErr, errServerSetupLinkPinMismatch):
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	default:
		return waitFor("server_setup_peer_inspection_unreachable", "main", "")
	}
	if answer.CredentialID != "" {
		record.CredentialID = answer.CredentialID
	}
	record.Probe = answer.Probe
	switch answer.State {
	case transport.DNSPeerInspectionPrepared:
		if action == transport.DNSPeerInspectionActionPrepare {
			return answer, false, nil
		}
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	case transport.DNSPeerInspectionActive:
		if answer.Probe == "reachable" {
			return finalize(peerInspectionOutcomeReady, "", "")
		}
		return waitFor("server_setup_peer_inspection_ssh_unreachable", "main", answer.Probe)
	case transport.DNSPeerInspectionExisting:
		return finalize(peerInspectionOutcomeExisting, "main", "")
	case transport.DNSPeerInspectionOwnerConflict:
		return finalize(peerInspectionOutcomeOwnerConflict, "main", answer.Reason)
	case transport.DNSPeerInspectionPairNotReady:
		return waitFor("server_setup_peer_inspection_main_pair", "main", "")
	case transport.DNSPeerInspectionFailed:
		if answer.Reason == transport.DNSPeerInspectionReasonCredentialStale {
			// The prepared key is gone on the main: prepare again next time.
			record.CredentialID, record.PublicKey, record.HostKeySHA256 = "", "", ""
		}
		return waitFor("server_setup_peer_inspection_failed", "main", answer.Reason)
	default:
		return finalize(peerInspectionOutcomeUnsupported, "main", "")
	}
}

// --- guidance ---------------------------------------------------------------

func serverSetupPeerInspectionPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return strings.Join(parts, ", ")
}

func serverSetupPeerInspectionAddress(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// serverSetupPeerInspectionWait is the wait for the record's current code
// (D-024: reason, who acts, next action, how it resumes). This server is the
// backup; every wait resumes by itself.
func serverSetupPeerInspectionWait(record serverSetupPeerInspectionRecord) error {
	main := serverSetupPeerInspectionAddress(record.MainIP, "the main DNS server")
	backup := serverSetupPeerInspectionAddress(record.BackupIP, "this server")
	lead := "This server's setup is ready. It is now setting up, together with the main DNS server, the read-only check the main uses to confirm that a deleted domain is gone from this backup too. "
	escape := " If you do not want this check, choose \"Stop waiting for the other server\" on the main DNS server's setup page; DNS keeps working without it, but a deleted domain then stays pending until the check is set up with the dns-peer-enroll owner tool."
	var message string
	switch record.Wait {
	case "server_setup_peer_inspection_unreachable":
		message = lead + "The main DNS server could not be reached, or its answer was lost. Nothing to do if it is only briefly offline; otherwise check that the main DNS server's panel is running. This server tries again by itself."
	case "server_setup_peer_inspection_main_agent":
		message = lead + "The main DNS server's CelikPanel Agent did not answer. Nothing to do if it is restarting; if it lasts, the main DNS server's owner checks it there with sudo systemctl status celikpanel-agent. This server tries again by itself."
	case "server_setup_peer_inspection_agent":
		message = lead + "This server's CelikPanel Agent did not answer. Nothing to do if it is restarting; if it lasts, check it here with sudo systemctl status celikpanel-agent. This server tries again by itself."
	case "server_setup_peer_inspection_main_pair":
		message = lead + "The main DNS server has not finished setting up its side of the DNS pair yet. Nothing to do; this server asks again by itself."
	case "server_setup_peer_inspection_backup_pair":
		message = lead + "This server's DNS pair is not recorded as set up yet. Nothing to do; this server asks again by itself."
	case "server_setup_peer_inspection_ssh_port":
		message = lead + "The check connects to this server's SSH on port 22, but SSH here listens on port " + serverSetupPeerInspectionPorts(record.SSHPorts) + ". You, the owner: let OpenSSH also listen on port 22 (add Port 22 next to the current Port line in /etc/ssh/sshd_config, run sudo sshd -t, then reload OpenSSH); CelikPanel's firewall opens the SSH ports OpenSSH listens on. This server checks again by itself." + escape
	case "server_setup_peer_inspection_failed":
		where := "on this server"
		if record.Side == "main" {
			where = "on the main DNS server (" + main + ")"
		}
		message = lead + "A step " + where + " was refused (" + serverSetupPeerInspectionReasonText(record.Reason) + "); nothing was overwritten and staged files are kept. The server owner reads the reason there with sudo journalctl -u celikpanel-agent | grep \"peer inspection\", fixes it, and this server tries again by itself within a few minutes." + escape
	case "server_setup_peer_inspection_ssh_unreachable":
		message = lead + "The check is set up on both servers, but the main DNS server (" + main + ") " + serverSetupPeerInspectionProbeText(record.Probe, backup) + " This server checks again by itself." + escape
	default:
		message = lead + "Nothing to do; this server continues by itself."
	}
	return &serverSetupPeerWait{Code: record.Wait, Message: message}
}

func serverSetupPeerInspectionProbeText(probe, backup string) string {
	switch probe {
	case "host_key_mismatch":
		return "reached SSH on port 22 at " + backup + " but it answered with another server key than this server's. Check that " + backup + " is really this server's address and that nothing in between answers for it."
	case "auth_refused":
		return "reached this server's SSH on port 22 at " + backup + ", but the check's key was refused. The server owner checks here with sudo /opt/celikpanel/dns-owner-tools/dns-peer-enroll secondary-status (add --engine pdns for PowerDNS) and the SSH log (sudo journalctl -u ssh -u sshd)."
	default:
		return "could not connect to this server's SSH on port 22 at " + backup + ". Allow incoming TCP port 22 from the main DNS server's address in every firewall in front of this server (for example the hosting provider's firewall)."
	}
}

func serverSetupPeerInspectionReasonText(reason string) string {
	switch reason {
	case transport.DNSPeerInspectionReasonSSHDRejected:
		return "OpenSSH did not accept the added configuration"
	case transport.DNSPeerInspectionReasonSSHInactive:
		return "OpenSSH is not running as ssh.service or sshd.service, or could not be reloaded"
	case transport.DNSPeerInspectionReasonDNSInactive:
		return "the DNS server is not running"
	case transport.DNSPeerInspectionReasonHostKey:
		return "the server's SSH Ed25519 host key could not be read"
	case transport.DNSPeerInspectionReasonCredentialStale:
		return "the key prepared earlier is no longer there; a new one is prepared"
	case transport.DNSPeerInspectionReasonStaged:
		return "files from an earlier attempt differ from what this step writes"
	default:
		return "a native step did not complete"
	}
}

// serverSetupPeerInspectionView is the outcome or current wait shown on the
// setup page of either server. Code is the translation key's last part.
type serverSetupPeerInspectionView struct {
	State   string            `json:"state"` // ready, attention, working
	Code    string            `json:"code"`
	Side    string            `json:"side,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
	Message string            `json:"message"`
}

// serverSetupPeerInspectionViewFor reads this server's record (main or
// backup). None while there is no record or nothing applies.
func serverSetupPeerInspectionViewFor(record serverSetupPeerInspectionRecord, backup bool) *serverSetupPeerInspectionView {
	vars := map[string]string{}
	if record.MainIP != "" {
		vars["main"] = record.MainIP
	}
	if record.BackupIP != "" {
		vars["backup"] = record.BackupIP
	}
	view := &serverSetupPeerInspectionView{State: "attention", Side: record.Side, Vars: vars}
	engine := ""
	if record.Engine == string(transport.DNSEnginePowerDNS) {
		engine = " --engine pdns"
	}
	tool := "sudo /opt/celikpanel/dns-owner-tools/dns-peer-enroll"
	switch record.Outcome {
	case peerInspectionOutcomeReady:
		view.State, view.Code = "ready", "ready"
		view.Message = "The deletion check between the two servers is set up: when a domain is deleted, the main DNS server confirms by itself that the backup removed it too. Nothing to do. It is native configuration on both servers; either owner can remove it with " + tool + " secondary-revoke" + engine + " (backup) and primary-revoke" + engine + " (main)."
	case peerInspectionOutcomeExisting:
		view.State, view.Code = "ready", "existing"
		view.Message = "A deletion check the server owner set up earlier was found and kept exactly as it is; setup changed nothing. If deleted domains stay pending, the owner compares it on both servers with " + tool + " primary-status" + engine + " (main) and secondary-status" + engine + " (backup)."
	case peerInspectionOutcomeOwnerConflict:
		view.Code = "owner_conflict"
		where, status := "the backup DNS server", "secondary-status"+engine
		if record.Side == "main" {
			where, status = "the main DNS server", "primary-status"+engine
		}
		view.Message = "The deletion check was not set up automatically: " + where + " already has inspection settings, files or SSH configuration that the owner made or changed, and setup never overwrites them. Nothing was changed. Until this is resolved a deleted domain stays pending and later DNS changes wait behind it. The server owner reviews them there with " + tool + " " + status + ", then either keeps them and completes the check with the owner tool's steps, or removes them with its revoke command and sets the check up with the owner tool."
	case peerInspectionOutcomeRevoked:
		view.Code = "revoked"
		view.Message = "The server owner turned off the deletion check on the backup DNS server, and setup does not turn it back on. A deleted domain stays pending until the check is set up again with the dns-peer-enroll owner tool on both servers; DNS keeps answering meanwhile."
	case peerInspectionOutcomeInspector:
		view.Code = "inspector_unavailable"
		view.Message = "The deletion check was not set up automatically: the backup DNS server has no packaged inspector program in /opt/celikpanel/dns-owner-tools (an installation updated from an earlier release). A deleted domain stays pending until the check is set up with the dns-peer-enroll owner tool from the verified release archive, as its README describes."
	case peerInspectionOutcomeUnsupported:
		view.Code = "unsupported"
		view.Message = "The deletion check was not set up automatically because one of the two panels or Agents does not offer this yet. Update the older panel from its own update screen; until then, a deleted domain stays pending unless the check is set up with the dns-peer-enroll owner tool."
	case peerInspectionOutcomeLinkEnded:
		view.Code = "link_ended"
		view.Message = "The deletion check was not set up: the main DNS server stopped waiting for this server before it finished. A deleted domain stays pending until the check is set up with the dns-peer-enroll owner tool on both servers."
	case peerInspectionOutcomeNotApplicable:
		return nil
	case "":
		if record.Wait == "" {
			view.State, view.Code = "working", "working"
			view.Message = "The deletion check between the two servers is being set up. Nothing to do."
			break
		}
		view.State, view.Code = "working", strings.TrimPrefix(record.Wait, "server_setup_peer_inspection_")
		if record.Wait == "server_setup_peer_inspection_ssh_port" {
			vars["ports"] = serverSetupPeerInspectionPorts(record.SSHPorts)
		}
		if record.Wait == "server_setup_peer_inspection_ssh_unreachable" && record.Probe != "" {
			view.Code = "ssh_" + record.Probe
		}
		if wait, ok := serverSetupPeerInspectionWait(record).(*serverSetupPeerWait); ok {
			view.Message = wait.Message
		}
		if !backup {
			view.Message = "The backup DNS server is setting up the deletion check with this server and waits: " + view.Message
		}
	default:
		return nil
	}
	if record.Reason != "" {
		vars["reason"] = record.Reason
	}
	return view
}

// serverSetupIsBackupDNS: this server's saved setup answers make it the
// backup DNS server of its pair.
func (p *Panel) serverSetupIsBackupDNS(ctx context.Context) bool {
	state, err := p.loadServerSetup(ctx)
	return err == nil && serverSetupDraftIsBackupDNS(state.Draft)
}

// serverSetupPeerInspectionViewInto adds this server's record to its peer
// view. A record of another join than the current one is not shown.
func (p *Panel) serverSetupPeerInspectionViewInto(ctx context.Context, view *serverSetupPeerView, joinRequestID string, backup bool) {
	record, found, err := p.readServerSetupPeerInspection(ctx)
	if err != nil || !found || record.JoinRequestID != joinRequestID {
		return
	}
	view.Inspection = serverSetupPeerInspectionViewFor(record, backup)
}
