package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Mail server settings one at a time (alpha.130; docs/MAIL-SERVER-SETTINGS-
// DESIGN.md), administrator only (the /api/v1/mail/ prefix):
//
//	GET  /api/v1/mail/server-settings  subaddressing and the sender list:
//	                                   the saved wish, the last apply and what
//	                                   the server holds
//	POST /api/v1/mail/subaddressing    {"enabled": bool} (steps: check, save, apply)
//	POST /api/v1/mail/sender-rules     {"op": "add", "value", "action"} or
//	                                   {"op": "remove", "id"} (steps: check,
//	                                   save, apply); the whole list is applied
//
// Server-wide only. A per-domain list would need one Postfix restriction
// class per domain; that is not built (the design note says why).
//
// Alt adresleme ve gönderen listesi: yalnız yönetici, sunucu geneli.

const (
	mailServerSettingsPath = "/api/v1/mail/server-settings"
	mailSubaddressingPath  = "/api/v1/mail/subaddressing"
	mailSenderRulesPath    = "/api/v1/mail/sender-rules"

	errCodeMailSenderInvalid   = "MAIL_SENDER_INVALID"
	errCodeMailSenderOwnDomain = "MAIL_SENDER_OWN_DOMAIN"
	errCodeMailSenderExists    = "MAIL_SENDER_EXISTS"
	errCodeMailSenderTooMany   = "MAIL_SENDER_TOO_MANY"
)

type mailSettingApply struct {
	State  string `json:"state,omitempty"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type mailSenderRuleView struct {
	ID        int    `json:"id"`
	Value     string `json:"value"`
	Action    string `json:"action"`
	CreatedAt string `json:"created_at"`
}

type mailServerSettingsView struct {
	Subaddressing struct {
		Enabled bool             `json:"enabled"`
		Apply   mailSettingApply `json:"apply"`
	} `json:"subaddressing"`
	Sender struct {
		Rules []mailSenderRuleView `json:"rules"`
		Apply mailSettingApply     `json:"apply"`
	} `json:"sender"`
	Server        *transport.MailServerSettingsStatus `json:"server,omitempty"`
	ServerUnknown string                              `json:"server_unknown,omitempty"`
}

// Swapped by tests.
var (
	mailSettingsAgentStatus = func(p *Panel, ctx context.Context) (transport.MailServerSettingsStatus, error) {
		var status transport.MailServerSettingsStatus
		err := p.callAgentContext(ctx, "Agent.MailServerSettingsStatusV1", &transport.Empty{}, &status)
		return status, err
	}
	mailSubaddressingAgentApply = func(p *Panel, ctx context.Context, req *transport.MailSubaddressingRequest) (transport.MailSettingApplyResponse, error) {
		var resp transport.MailSettingApplyResponse
		err := p.callAgentContext(ctx, "Agent.MailSubaddressingV1", req, &resp)
		return resp, err
	}
	mailSenderAgentApply = func(p *Panel, ctx context.Context, req *transport.MailSenderListRequest) (transport.MailSettingApplyResponse, error) {
		var resp transport.MailSettingApplyResponse
		err := p.callAgentContext(ctx, "Agent.MailSenderListV1", req, &resp)
		return resp, err
	}
)

func (p *Panel) readMailServerSettings(ctx context.Context) (mailServerSettingsView, error) {
	var view mailServerSettingsView
	var enabled int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT subaddressing, subaddressing_apply_state, subaddressing_apply_code, subaddressing_apply_detail,
		       sender_apply_state, sender_apply_code, sender_apply_detail
		FROM mail_server_options WHERE id = 1`).Scan(&enabled,
		&view.Subaddressing.Apply.State, &view.Subaddressing.Apply.Code, &view.Subaddressing.Apply.Detail,
		&view.Sender.Apply.State, &view.Sender.Apply.Code, &view.Sender.Apply.Detail); err != nil {
		return view, err
	}
	view.Subaddressing.Enabled = enabled == 1
	rules, err := p.readMailSenderRules(ctx)
	view.Sender.Rules = rules
	return view, err
}

func (p *Panel) readMailSenderRules(ctx context.Context) ([]mailSenderRuleView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id, value, action, created_at FROM mail_sender_rules ORDER BY action DESC, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []mailSenderRuleView{}
	for rows.Next() {
		var rule mailSenderRuleView
		if err := rows.Scan(&rule.ID, &rule.Value, &rule.Action, &rule.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (p *Panel) handleMailServerSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case mailSubaddressingPath:
		p.saveMailSubaddressing(w, r)
		return
	case mailSenderRulesPath:
		p.changeMailSenderRules(w, r)
		return
	case mailServerSettingsPath:
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	view, err := p.readMailServerSettings(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	status, err := mailSettingsAgentStatus(p, r.Context())
	switch {
	case err != nil:
		view.ServerUnknown = "the server's mail settings could not be read just now"
	case status.Error != "":
		view.Server = &status
		view.ServerUnknown = status.Error
	default:
		view.Server = &status
	}
	_ = json.NewEncoder(w).Encode(view)
}

// recordMailSettingApply stores how the server took one of the settings.
func (p *Panel) recordMailSettingApply(ctx context.Context, prefix string, resp transport.MailSettingApplyResponse, callErr error) string {
	state, code, detail := "applied", "", ""
	switch {
	case callErr != nil:
		state, detail = "unknown", "the server did not answer; whether it took the change is not known"
	case resp.Applied:
	case resp.Code == transport.MailSettingOwnerConfig || resp.Code == transport.MailSettingEditedConfig ||
		resp.Code == transport.MailSettingDovecot || resp.Code == transport.MailSettingInvalid ||
		resp.Code == transport.MailSettingNoPostfix || resp.Code == transport.MailSettingBusy:
		state, code, detail = "refused", resp.Code, resp.Error
	default:
		state, code, detail = "failed", resp.Code, resp.Error
	}
	// prefix is one of two constants, never input.
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE mail_server_options SET `+prefix+`_apply_state = ?, `+prefix+`_apply_code = ?, `+
		prefix+`_apply_detail = ?, updated_at = datetime('now') WHERE id = 1`, state, code, relayBounded(detail, 1000)); err != nil {
		log.Printf("[mail-settings] record the %s apply: %v", prefix, err)
	}
	return state
}

func (p *Panel) saveMailSubaddressing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "save", "apply")
	reportOperationStep(ctx, "check")
	reportOperationStep(ctx, "save")
	enabled := 0
	if request.Enabled {
		enabled = 1
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE mail_server_options SET subaddressing = ?, updated_at = datetime('now') WHERE id = 1`, enabled); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "mail.subaddressing", "mail_server_options", 1, map[string]any{"enabled": request.Enabled})
	reportOperationStep(ctx, "apply")
	resp, err := mailSubaddressingAgentApply(p, ctx, &transport.MailSubaddressingRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Enabled: request.Enabled})
	state := p.recordMailSettingApply(ctx, "subaddressing", resp, err)
	answer := map[string]any{"saved": true, "state": state, "result": resp}
	if err != nil {
		answer["unknown"] = "the server did not answer; whether it took the change is not known"
	}
	_ = json.NewEncoder(w).Encode(answer)
}

// hostedMailDomain reports whether a sender value is, or is under, a domain
// hosted here: mail from those is protected by sign-in, and blocking them
// would also stop this server's own mailboxes.
func (p *Panel) hostedMailDomain(ctx context.Context, value string) (string, bool, error) {
	domain := value
	if at := strings.LastIndexByte(value, '@'); at >= 0 {
		domain = value[at+1:]
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT lower(name) FROM domains`)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", false, err
		}
		if domain == name || strings.HasSuffix(domain, "."+name) || strings.HasSuffix(name, "."+domain) {
			return name, true, nil
		}
	}
	return "", false, rows.Err()
}

func (p *Panel) changeMailSenderRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Op     string `json:"op"`
		Value  string `json:"value"`
		Action string `json:"action"`
		ID     int    `json:"id"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "save", "apply")
	reportOperationStep(ctx, "check")
	db := p.db.GetDB()
	switch request.Op {
	case "add":
		value, err := transport.CanonicalMailSenderValue(request.Value)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeMailSenderInvalid, err.Error(), "")
			return
		}
		if request.Action != transport.MailSenderAllow && request.Action != transport.MailSenderBlock {
			writeCodedError(w, http.StatusBadRequest, errCodeMailSenderInvalid, "choose allow or block", "")
			return
		}
		hosted, ok, err := p.hostedMailDomain(ctx, value)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if ok {
			writeCodedErrorVars(w, http.StatusConflict, errCodeMailSenderOwnDomain,
				value+" belongs to "+hosted+", a domain of this server: its mail is protected by sign-in and a list entry would also stop this server's own mailboxes. Nothing was changed", "", nil, map[string]string{"domain": hosted})
			return
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mail_sender_rules`).Scan(&count); err != nil {
			writeServerError(w, err)
			return
		}
		if count >= transport.MailSenderMaxRules {
			writeCodedError(w, http.StatusConflict, errCodeMailSenderTooMany, "the sender list is full; remove entries first", "")
			return
		}
		reportOperationStep(ctx, "save")
		var created any
		if caller := currentCaller(r); caller != nil {
			created = caller.ID
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO mail_sender_rules (value, action, created_by) VALUES (?, ?, ?)`, value, request.Action, created); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeCodedError(w, http.StatusConflict, errCodeMailSenderExists, value+" is already on the list; nothing was changed", "")
				return
			}
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "mail.sender.add", "mail_server_options", 1, map[string]any{"value": value, "action": request.Action})
	case "remove":
		reportOperationStep(ctx, "save")
		var value string
		err := db.QueryRowContext(ctx, `SELECT value FROM mail_sender_rules WHERE id = ?`, request.ID).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "that entry is no longer on the list; nothing was changed")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM mail_sender_rules WHERE id = ?`, request.ID); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "mail.sender.remove", "mail_server_options", 1, map[string]any{"value": value})
	default:
		writeClientError(w, http.StatusBadRequest, "op must be add or remove")
		return
	}

	reportOperationStep(ctx, "apply")
	rules, err := p.readMailSenderRules(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	agentRules := make([]transport.MailSenderRule, 0, len(rules))
	for _, rule := range rules {
		agentRules = append(agentRules, transport.MailSenderRule{Value: rule.Value, Action: rule.Action})
	}
	resp, err := mailSenderAgentApply(p, ctx, &transport.MailSenderListRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), Rules: agentRules})
	state := p.recordMailSettingApply(ctx, "sender", resp, err)
	answer := map[string]any{"saved": true, "state": state, "result": resp}
	if err != nil {
		answer["unknown"] = "the server did not answer; whether it took the change is not known"
	}
	_ = json.NewEncoder(w).Encode(answer)
}
