package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Out-of-office reply per mailbox (alpha.114). The reply is the mailbox's
// Sieve script that Dovecot runs (cmd/agent/mail_autoresponder.go), so it
// works with the Panel stopped. The Panel stores what it last applied
// (email_autoresponders) and shows the Agent's reading of the mailbox: a
// script the mailbox owner made is named, never replaced (D-022, D-024).
//
// Posta kutusu başına otomatik yanıt. Yanıtı Dovecot çalıştırır; Panel son
// uygulananı saklar ve kutu sahibinin kendi betiğini adıyla söyler.

type mailAutoresponderView struct {
	AccountID int                         `json:"account_id"`
	Address   string                      `json:"address"`
	Setting   transport.MailAutoresponder `json:"autoresponder"`
	Stored    bool                        `json:"stored"`
	// State is the Agent's reading; StateError says why it is missing
	// (unknown, not "no reply").
	State      *transport.MailAutoresponderResponse `json:"state,omitempty"`
	StateError string                               `json:"state_error,omitempty"`
}

func (p *Panel) mailAutoresponderAccount(ctx context.Context, domainID, accountID int) (string, error) {
	var address, domain string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT account.address, domain.name
		FROM email_accounts account
		JOIN domains domain ON domain.id = account.domain_id
		WHERE account.id = ? AND account.domain_id = ?`, accountID, domainID).Scan(&address, &domain)
	if err != nil {
		return "", err
	}
	canonical, err := transport.CanonicalMailboxForDomain(address, domain)
	if err != nil {
		return "", errors.New("stored mailbox identity is invalid")
	}
	return canonical, nil
}

func (p *Panel) readStoredAutoresponder(ctx context.Context, accountID int) (transport.MailAutoresponder, bool, error) {
	var setting transport.MailAutoresponder
	var enabled int
	var start, end sql.NullString
	var addresses string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT enabled, subject, message, start_date, end_date, interval_days, addresses
		FROM email_autoresponders WHERE account_id = ?`, accountID).
		Scan(&enabled, &setting.Subject, &setting.Message, &start, &end, &setting.IntervalDays, &addresses)
	if errors.Is(err, sql.ErrNoRows) {
		return transport.MailAutoresponder{Subject: "Out of office", IntervalDays: 1}, false, nil
	}
	if err != nil {
		return setting, false, err
	}
	setting.Enabled = enabled == 1
	setting.StartDate, setting.EndDate = start.String, end.String
	if err := json.Unmarshal([]byte(addresses), &setting.Addresses); err != nil {
		return setting, false, err
	}
	return setting, true, nil
}

func (p *Panel) handleMailAutoresponder(w http.ResponseWriter, r *http.Request, domainID int) {
	switch r.Method {
	case http.MethodGet:
		p.handleGetMailAutoresponder(w, r, domainID)
	case http.MethodPut:
		p.handlePutMailAutoresponder(w, r, domainID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (p *Panel) handleGetMailAutoresponder(w http.ResponseWriter, r *http.Request, domainID int) {
	accountID, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || accountID <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	address, err := p.mailAutoresponderAccount(r.Context(), domainID, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	setting, stored, err := p.readStoredAutoresponder(r.Context(), accountID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view := mailAutoresponderView{AccountID: accountID, Address: address, Setting: setting, Stored: stored}
	var state transport.MailAutoresponderResponse
	if err := p.callAgentContext(r.Context(), "Agent.MailAutoresponderState",
		&transport.MailAutoresponderStateRequest{Email: address}, &state); err != nil {
		// Unknown, not "no reply": the page says the Agent could not be asked.
		view.StateError = "agent_unavailable"
	} else {
		view.State = &state
	}
	_ = json.NewEncoder(w).Encode(view)
}

func (p *Panel) handlePutMailAutoresponder(w http.ResponseWriter, r *http.Request, domainID int) {
	var request struct {
		ID int `json:"id"`
		transport.MailAutoresponder
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || request.ID <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	address, err := p.mailAutoresponderAccount(r.Context(), domainID, request.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	setting, err := transport.NormalizeMailAutoresponder(request.MailAutoresponder, address)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, err.Error())
		return
	}

	p.mailMutationMu.Lock()
	defer p.mailMutationMu.Unlock()
	if err := p.ensureMailDomainMutable(r.Context(), domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	var response transport.MailAutoresponderResponse
	if err := p.callAgentContext(r.Context(), "Agent.SetMailAutoresponder", &transport.SetMailAutoresponderRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Email:               address,
		Autoresponder:       setting,
	}, &response); err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	if response.Refusal != "" {
		// Nothing was changed on the server; the code says who acts.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(apiErrorBody{
			Error: response.Detail, Code: errCodeMailAutoresponderRefused, Reason: response.Refusal,
		})
		return
	}
	if !response.Applied {
		writeServerError(w, errors.New("the out-of-office reply was not confirmed by the Agent"))
		return
	}
	addresses, err := json.Marshal(append([]string{}, setting.Addresses...))
	if err != nil {
		writeServerError(w, err)
		return
	}
	var start, end any
	if setting.StartDate != "" {
		start = setting.StartDate
	}
	if setting.EndDate != "" {
		end = setting.EndDate
	}
	enabled := 0
	if setting.Enabled {
		enabled = 1
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		INSERT INTO email_autoresponders
			(account_id, enabled, subject, message, start_date, end_date, interval_days, addresses, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(account_id) DO UPDATE SET
			enabled = excluded.enabled, subject = excluded.subject, message = excluded.message,
			start_date = excluded.start_date, end_date = excluded.end_date,
			interval_days = excluded.interval_days, addresses = excluded.addresses,
			updated_at = excluded.updated_at`,
		request.ID, enabled, setting.Subject, setting.Message, start, end, setting.IntervalDays, string(addresses)); err != nil {
		// The server already answers mail as requested; only the page's
		// copy is missing. Saving again rewrites the same script.
		writeServerError(w, errors.Join(errors.New("the reply is active on the server, but the Panel could not store it; save it again"), err))
		return
	}
	action := "mail.autoresponder.disable"
	if setting.Enabled {
		action = "mail.autoresponder.enable"
	}
	p.audit(r, action, "email_account", request.ID)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"autoresponder": setting,
		"state":         response,
	})
}
