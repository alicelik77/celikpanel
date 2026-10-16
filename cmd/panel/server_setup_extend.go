package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostname"
)

// Extending a completed setup (2026-10-16, second server as a full peer).
//
// A completed setup can be extended: the wizard reopens with the completed
// draft, the owner adds profiles, components or host names, reviews the plan
// and starts it with the existing runner. Only the state row and one setting
// change here; no host is touched before the reviewed start.
//
// What an extension keeps: the completion record (completed_at), so the
// server stays prepared and nothing routes the owner back into setup
// (serverSetupState.Extension, Required=false); every installed component,
// because a plan never removes one (D-021 customization invariants); the
// completed draft as the extension's base, so the review can show only what
// changes and stopping the extension restores it.
//
// Refusals: an active setup run (`server_setup_busy`), a setup that has not
// finished (`server_setup_extend_not_ready`), a stale revision
// (`setup_conflict`), stopping what is not an extension
// (`server_setup_extend_not_active`).
//
// Tamamlanmis kurulumu genisletme: yalniz durum satiri ve bir ayar degisir;
// incelenip baslatilmadan sunucuya dokunulmaz. Yuklu hicbir sey kaldirilmaz.

const settingServerSetupExtensionBase = "server_setup_extension_base"

const (
	errCodeServerSetupExtendNotReady  = "server_setup_extend_not_ready"
	errCodeServerSetupExtendNotActive = "server_setup_extend_not_active"
	serverSetupExtendNotReadyMessage  = "This server's setup has not finished yet, so there is nothing to extend. Continue the current setup instead."
	serverSetupExtendNotActiveMessage = "This server's setup is not being extended, so there is nothing to stop."
	serverSetupExtendBusyMessage      = "A setup run is in progress on this server. Its page shows the progress; extend or stop the setup after it has finished."
)

// The base is stored as the completed draft_json, unchanged.
func saveServerSetupExtensionBase(ctx context.Context, tx *sql.Tx, raw string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO panel_settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingServerSetupExtensionBase, raw)
	return err
}

// serverSetupExtensionBase reads the completed draft an extension started
// from. An unreadable or invalid base is absent: the review then shows the
// whole plan, and stopping keeps the current draft.
func (p *Panel) serverSetupExtensionBase(ctx context.Context) *serverSetupDraft {
	var raw string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key=?`, settingServerSetupExtensionBase).Scan(&raw); err != nil {
		return nil
	}
	base := defaultServerSetupDraft()
	if json.Unmarshal([]byte(raw), &base) != nil {
		return nil
	}
	canonical, err := canonicalServerSetupDraft(base)
	if err != nil {
		return nil
	}
	return &canonical
}

// serverSetupMailProfileComponents are the setup components that select a
// mail profile (serverSetupMailProfileIDs reads them back).
var serverSetupMailProfileComponents = map[string][]string{
	core.MailProfileCore:      {"postfix", "dovecot"},
	core.MailProfileWebmail:   {"postfix", "dovecot", "roundcube"},
	core.MailProfileProtected: {"postfix", "dovecot", "rspamd"},
}

var errServerSetupMailProfileUnknown = errors.New("unknown mail profile")

// serverSetupDraftWithMailProfile adds one mail profile to a setup draft. A
// draft whose mail profiles already cover every service of the profile is
// returned unchanged (the run repairs that profile). Otherwise the draft's
// components are kept and the profile's components are added; a DNS-only
// purpose becomes a custom selection, because it then hosts mail.
func serverSetupDraftWithMailProfile(draft serverSetupDraft, profileID string) (serverSetupDraft, error) {
	profile, ok := mailProfileByID(profileID)
	added, known := serverSetupMailProfileComponents[profileID]
	if !ok || !known {
		return draft, errServerSetupMailProfileUnknown
	}
	covered := map[string]bool{}
	for _, id := range serverSetupMailProfileIDs(draft) {
		if existing, found := mailProfileByID(id); found {
			for _, service := range existing.Services {
				covered[service] = true
			}
		}
	}
	missing := false
	for _, service := range profile.Services {
		missing = missing || !covered[service]
	}
	if !missing {
		return draft, nil
	}
	components := serverSetupPresetComponents(draft)
	if draft.Customization != nil {
		components = append([]string{}, draft.Customization.Components...)
	}
	components = append(components, added...)
	slices.Sort(components)
	draft.Customization = &serverSetupCustomization{Components: slices.Compact(components)}
	if draft.Purpose == "dns" {
		draft.Purpose = "custom"
	}
	return canonicalServerSetupDraft(draft)
}

func (p *Panel) handleServerSetupExtend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if !requireServerSetupAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Revision     int    `json:"revision"`
		Action       string `json:"action"`
		MailProfile  string `json:"mail_profile,omitempty"`
		MailHostname string `json:"mail_hostname,omitempty"`
	}
	if decodeServiceOperationJSON(w, r, &request) != nil || !stringIn(request.Action, "begin", "cancel") {
		writeClientError(w, http.StatusBadRequest, "invalid setup extension request")
		return
	}
	request.MailProfile = strings.TrimSpace(request.MailProfile)
	request.MailHostname = strings.TrimSpace(request.MailHostname)
	if request.Action == "cancel" && (request.MailProfile != "" || request.MailHostname != "") {
		writeClientError(w, http.StatusBadRequest, "invalid setup extension request")
		return
	}
	if request.MailProfile != "" {
		if _, ok := mailProfileByID(request.MailProfile); !ok {
			writeClientError(w, http.StatusBadRequest, "unknown mail profile")
			return
		}
	}
	if request.MailHostname != "" {
		canonical, err := hostname.CanonicalFQDN(request.MailHostname)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeMailProfileServerHostnameInvalid, mailProfileServerHostnameMessage, "")
			return
		}
		request.MailHostname = canonical
	}

	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state.Revision != request.Revision {
		writeCodedError(w, http.StatusConflict, "setup_conflict", errServerSetupConflict.Error(), "/setup")
		return
	}
	active, err := p.serverSetupExecutionActive(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if active || stringIn(state.Status, "running", "waiting") {
		writeCodedError(w, http.StatusConflict, "server_setup_busy", serverSetupExtendBusyMessage, "/setup")
		return
	}
	extending := state.Extension && stringIn(state.Status, "draft", "failed")
	if request.Action == "cancel" {
		if !extending {
			writeCodedError(w, http.StatusConflict, errCodeServerSetupExtendNotActive, serverSetupExtendNotActiveMessage, "/setup")
			return
		}
		p.cancelServerSetupExtension(w, r, state)
		return
	}
	if state.Status != "ready" && !extending {
		writeCodedError(w, http.StatusConflict, errCodeServerSetupExtendNotReady, serverSetupExtendNotReadyMessage, "/setup")
		return
	}
	draft := state.Draft
	if request.MailProfile != "" {
		draft, err = serverSetupDraftWithMailProfile(draft, request.MailProfile)
		if err != nil {
			writeClientError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if request.MailHostname != "" {
		draft.MailHostname = request.MailHostname
	}
	if request.MailProfile != "" && draft.MailHostname == "" {
		// The name saved for the mail services, when one is; otherwise the
		// dialog asks for one (Components, mail profile dialog).
		if saved, err := hostname.CanonicalFQDN(strings.TrimSpace(p.setting(r.Context(), settingMailHostname))); err == nil {
			draft.MailHostname = saved
		} else {
			writeCodedError(w, http.StatusBadRequest, errCodeMailProfileServerHostnameRequired, mailProfileServerHostnameRequired, "")
			return
		}
	}
	draft, err = canonicalServerSetupDraft(draft)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	if extending && serverSetupDraftsEqual(draft, state.Draft) {
		json.NewEncoder(w).Encode(state)
		return
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	if state.Status == "ready" {
		var completed string
		if err := tx.QueryRowContext(r.Context(), `SELECT draft_json FROM server_setup_state WHERE id=1 AND revision=?`, request.Revision).Scan(&completed); err != nil {
			writeCodedError(w, http.StatusConflict, "setup_conflict", errServerSetupConflict.Error(), "/setup")
			return
		}
		if err := saveServerSetupExtensionBase(r.Context(), tx, completed); err != nil {
			writeServerError(w, err)
			return
		}
	}
	// A ready row always carries completed_at; one that does not gets the
	// time of this extension, so the extension is never taken for a first setup.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(r.Context(), `UPDATE server_setup_state SET draft_json=?,status='draft',revision=revision+1,updated_at=?,
		completed_at=CASE WHEN completed_at='' THEN ? ELSE completed_at END
		WHERE id=1 AND revision=? AND (status='ready' OR (completed_at!='' AND status IN ('draft','failed')))
		AND NOT EXISTS (SELECT 1 FROM server_setup_executions WHERE status IN ('running','waiting'))`, string(raw), now, now, request.Revision)
	if p.commitServerSetupExtension(w, tx, result, err) {
		p.audit(r, "server.setup.extend", "", 0)
		p.writeServerSetupState(w, r)
	}
}

// Stopping an extension restores the completed draft and the ready status.
// Nothing installed by a partly run extension is removed: Components keeps
// managing it (D-021 customization invariants).
// Genisletmeyi durdurmak tamamlanmis taslagi geri yukler; kurulan kaldirilmaz.
func (p *Panel) cancelServerSetupExtension(w http.ResponseWriter, r *http.Request, state serverSetupState) {
	restored := state.Draft
	if state.ExtensionBase != nil {
		restored = *state.ExtensionBase
	}
	raw, err := json.Marshal(restored)
	if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM panel_settings WHERE key=?`, settingServerSetupExtensionBase); err != nil {
		writeServerError(w, err)
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE server_setup_state SET draft_json=?,status='ready',revision=revision+1,updated_at=?
		WHERE id=1 AND revision=? AND completed_at!='' AND status IN ('draft','failed')
		AND NOT EXISTS (SELECT 1 FROM server_setup_executions WHERE status IN ('running','waiting'))`, string(raw), time.Now().UTC().Format(time.RFC3339Nano), state.Revision)
	if p.commitServerSetupExtension(w, tx, result, err) {
		p.audit(r, "server.setup.extend.cancel", "", 0)
		p.writeServerSetupState(w, r)
	}
}

func (p *Panel) commitServerSetupExtension(w http.ResponseWriter, tx *sql.Tx, result sql.Result, err error) bool {
	if err != nil {
		writeServerError(w, err)
		return false
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeServerError(w, err)
		return false
	}
	if count != 1 {
		writeCodedError(w, http.StatusConflict, "setup_conflict", errServerSetupConflict.Error(), "/setup")
		return false
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return false
	}
	return true
}

func (p *Panel) writeServerSetupState(w http.ResponseWriter, r *http.Request) {
	state, err := p.loadServerSetup(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(state)
}

func serverSetupDraftsEqual(a, b serverSetupDraft) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	return errLeft == nil && errRight == nil && string(left) == string(right)
}
