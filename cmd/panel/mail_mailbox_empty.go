package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Emptying a mailbox (alpha.128, gap G20).
//
//	POST /api/v1/domains/{id}/mail/accounts/empty   {"id": N}
//
// Dovecot removes every message of every folder; the mailbox, its password,
// folders, aliases and settings stay. The screen asks first (the panel's own
// dialogue) and follows the server's steps; a replay with the same request
// identity is answered from the first run, so a reload never empties twice.
//
// Posta kutusunu boşaltma: Dovecot bütün iletileri siler; kutu ve ayarları
// kalır. Aynı istek kimliğiyle tekrar gelen istek ilk çalışmanın yanıtını alır.

const mailboxEmptyMessage = "The mailbox could not be emptied"

// mailboxEmptyAgent is replaced by tests.
var mailboxEmptyAgent = func(p *Panel, ctx context.Context, email string) (transport.EmptyMailboxResponse, error) {
	var resp transport.EmptyMailboxResponse
	err := p.callAgentContext(ctx, "Agent.EmptyMailboxV1", &transport.EmptyMailboxRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: email,
	}, &resp)
	return resp, err
}

func (p *Panel) handleEmptyMailbox(w http.ResponseWriter, r *http.Request, domainID int) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		ID int `json:"id"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || request.ID <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "empty")
	reportOperationStep(ctx, "check")
	p.mailMutationMu.Lock()
	defer p.mailMutationMu.Unlock()
	if err := p.ensureMailDomainMutable(ctx, domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	var stored, domain string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT a.address, d.name FROM email_accounts a JOIN domains d ON d.id = a.domain_id
		WHERE a.id = ? AND a.domain_id = ?`, request.ID, domainID).Scan(&stored, &domain)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	address, err := transport.CanonicalMailboxForDomain(stored, domain)
	if err != nil {
		writeServerError(w, errors.New("stored mailbox identity is invalid"))
		return
	}
	reportOperationStep(ctx, "empty")
	resp, err := mailboxEmptyAgent(p, ctx, address)
	if err != nil {
		writeServerError(w, errors.New(mailboxEmptyMessage+": the server could not be asked; whether messages were removed is not known"))
		return
	}
	if !resp.Applied {
		writeClientError(w, http.StatusConflict, mailboxEmptyMessage+": "+boundedAgentDiagnostic(resp.Error))
		return
	}
	p.audit(r, "mail.account.empty", "email_account", request.ID)
	p.requestMailQuotaReconcile()
	_ = json.NewEncoder(w).Encode(map[string]any{"emptied": true, "messages": resp.Messages, "quota_recalculated": resp.QuotaRecalculated})
}
