package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// SSH password sign-in (alpha.130, owner approval of 8 Nov 2026: "add the
// button; the owner presses it"). Security page, administrator only (the
// /api/v1/security/ prefix is behind the central administrator guard).
//
//   - GET /api/v1/security/ssh-password-login: what sshd itself says
//     (`sshd -T`), which file decides it, CelikPanel's drop-in, and which
//     accounts can still sign in with a key. A read only.
//   - POST the same path, {"action": "turn_off" | "turn_on",
//     "expected_drop_in_sha256": "<digest the page showed>"}: always with a
//     request identity, so a replay is answered from the stored row and the
//     progress window follows the server's steps (check, apply, verify). The
//     Agent writes or removes only /etc/ssh/sshd_config.d/00-celikpanel.conf,
//     validates with `sshd -t`, confirms with `sshd -T`, reloads ssh/sshd
//     (never restarts it) and takes its change back on any failure.
//
// SSH parola ile giriş: okuma ve yöneticinin onayladığı tek değişiklik.

const sshPasswordLoginPath = "/api/v1/security/ssh-password-login"

var requestIdentityRouteSSHPasswordLogin = requestIdentityRoute{
	pattern: sshPasswordLoginPath, timeout: agentRPCMutationTimeout + time.Minute,
}

var readSSHPasswordLoginFromAgent = func(p *Panel, ctx context.Context) (transport.SSHPasswordLoginStatusResponse, error) {
	var response transport.SSHPasswordLoginStatusResponse
	err := p.callAgentContext(ctx, "Agent.SSHPasswordLoginStatusV1", &transport.Empty{}, &response)
	return response, err
}

var setSSHPasswordLoginThroughAgent = func(p *Panel, ctx context.Context, action, expected string) (transport.SetSSHPasswordLoginResponse, error) {
	var response transport.SetSSHPasswordLoginResponse
	err := p.withStandaloneAgentMutation(ctx, "ssh_password_login", "openssh", "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.SetSSHPasswordLoginRequest{ServiceMutationBinding: binding, Action: action, ExpectedDropInSHA256: expected}
		return p.callAgentContext(callCtx, "Agent.SetSSHPasswordLoginV1", &request, &response)
	})
	return response, err
}

func (p *Panel) handleSSHPasswordLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "Only the administrator can change how SSH signs in. Nothing was changed.", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, err := readSSHPasswordLoginFromAgent(p, r.Context())
		if err != nil {
			writeAgentError(w, err, "SSH sign-in settings")
			return
		}
		normalizeSSHPasswordLoginStatus(&status)
		_ = json.NewEncoder(w).Encode(status)
	case http.MethodPost:
		p.changeSSHPasswordLogin(w, r)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
	}
}

// normalizeSSHPasswordLoginStatus: gob does not keep empty slices; the page
// reads [] rather than null.
func normalizeSSHPasswordLoginStatus(status *transport.SSHPasswordLoginStatusResponse) {
	status.Deciders = append([]transport.SSHSettingDecider{}, status.Deciders...)
	status.KeyAccounts = append([]transport.SSHKeyAccount{}, status.KeyAccounts...)
}

func (p *Panel) changeSSHPasswordLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action               string `json:"action"`
		ExpectedDropInSHA256 string `json:"expected_drop_in_sha256"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil ||
		(req.Action != transport.SSHPasswordLoginActionOff && req.Action != transport.SSHPasswordLoginActionOn) ||
		(req.ExpectedDropInSHA256 != "" && !sha256Text.MatchString(req.ExpectedDropInSHA256)) {
		writeCodedError(w, http.StatusBadRequest, "SSH_PASSWORD_INVALID", "The request was not understood, so nothing was changed. Reload the page and try again.", "")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "apply", "verify")
	reportOperationStep(ctx, "check")
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()

	reportOperationStep(ctx, "apply")
	response, err := setSSHPasswordLoginThroughAgent(p, ctx, req.Action, req.ExpectedDropInSHA256)
	details := map[string]any{"action": req.Action, "outcome": response.Outcome, "code": response.Code, "reloaded": response.Reloaded}
	if err != nil {
		p.auditDetailed(r, "security.ssh_password.failed — "+auditReason(err.Error()), "security", 0, details)
		log.Printf("ssh password sign-in: the Agent did not answer: %v", err)
		writeCodedError(w, http.StatusBadGateway, "SSH_PASSWORD_UNKNOWN",
			"The CelikPanel Agent did not answer, so whether SSH password sign-in changed is not known. Keep your current SSH session open, wait a minute and reload this page: it reads sshd's own settings again. ("+auditReason(err.Error())+")", "")
		return
	}
	switch response.Outcome {
	case transport.SSHPasswordLoginTurnedOff, transport.SSHPasswordLoginTurnedOn, transport.SSHPasswordLoginUnchanged:
		reportOperationStep(ctx, "verify")
		action := "security.ssh_password.off"
		if req.Action == transport.SSHPasswordLoginActionOn {
			action = "security.ssh_password.on"
		}
		if response.Outcome != transport.SSHPasswordLoginUnchanged {
			p.auditDetailed(r, action, "security", 0, details)
		}
		_ = json.NewEncoder(w).Encode(response)
	case transport.SSHPasswordLoginRefused:
		p.auditDetailed(r, "security.ssh_password.failed — "+auditReason(response.Error), "security", 0, details)
		writeSSHPasswordLoginRefusal(w, http.StatusConflict, response)
	default:
		// rolled_back or rollback_failed: a step failed after the change began.
		p.auditDetailed(r, "security.ssh_password.failed — "+auditReason(response.Error), "security", 0, details)
		writeSSHPasswordLoginRefusal(w, http.StatusUnprocessableEntity, response)
	}
}

// writeSSHPasswordLoginRefusal keeps the Agent's outcome, code and detail on
// the refusal so the page can say exactly what happened.
func writeSSHPasswordLoginRefusal(w http.ResponseWriter, status int, response transport.SetSSHPasswordLoginResponse) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":   response.Error,
		"code":    "SSH_PASSWORD_" + sshPasswordCode(response),
		"outcome": response.Outcome,
		"reason":  response.Code,
		"detail":  response.Detail,
	})
}

func sshPasswordCode(response transport.SetSSHPasswordLoginResponse) string {
	switch response.Outcome {
	case transport.SSHPasswordLoginRolledBack:
		return "ROLLED_BACK"
	case transport.SSHPasswordLoginRollbackFailed:
		return "ROLLBACK_FAILED"
	}
	return "REFUSED"
}
