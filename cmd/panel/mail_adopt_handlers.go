package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A mailbox on the server without a row in the Panel (alpha.144, alpha.143
// lab item 6). A creation whose record could not be written (the Panel
// stopped between the Agent's work and its own row) left the mailbox in the
// server's mail files; creating it again was answered "internal error".
// Now it is said plainly: the administrator may adopt it into its domain
// (one explicit action that reads the server and writes the row; the mail,
// its folder and its password are not touched) and then keep it or delete
// it like any mailbox; anybody else is told to ask the administrator. The
// sentence names only the address asked for, inside the person's own domain:
// nothing of another account.
//
// Panelde satırı olmadan sunucuda bulunan posta kutusu: yönetici onu alan
// adına sahiplenebilir (sunucuyu okur, satırı yazar; postaya dokunmaz);
// başkasına yöneticiye sorması söylenir.

const (
	errCodeMailAccountOnServerOnly = "MAIL_ACCOUNT_ON_SERVER_ONLY"
	// errCodeMailAccountAdoptRefused: the administrator's adoption found
	// something it does not record by itself; the reason says what.
	errCodeMailAccountAdoptRefused = "MAIL_ACCOUNT_ADOPT_REFUSED"
)

// Reasons of MAIL_ACCOUNT_ADOPT_REFUSED.
const (
	mailAdoptGone       = "gone"       // the server holds no line for it now
	mailAdoptIncomplete = "incomplete" // only one of the two lines exists
	mailAdoptElsewhere  = "elsewhere"  // its lines are not CelikPanel's form
)

// writeMailAccountOnServerOnly answers a creation the server refused because
// it already holds the mailbox. The administrator's sentence offers the
// adoption; the reason "admin" lets the page show its button.
func writeMailAccountOnServerOnly(w http.ResponseWriter, r *http.Request, address string) {
	vars := map[string]string{"address": address}
	if callerIsAdmin(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(apiErrorBody{
			Error: "a mailbox named " + address + " already exists on the server without a record in the panel, so nothing was changed. " +
				"Adopt it into this domain (CelikPanel records it as it is; its mail and password stay as they are), or choose another name.",
			Code: errCodeMailAccountOnServerOnly, Reason: "admin", Vars: vars,
		})
		return
	}
	writeCodedErrorVars(w, http.StatusConflict, errCodeMailAccountOnServerOnly,
		"a mailbox named "+address+" already exists on the server without a record in the panel, so nothing was changed. "+
			"Ask the server's administrator to look at it, or choose another name.", "", nil, vars)
}

// handleAdoptEmailAccount: POST /api/v1/domains/{id}/mail/accounts/adopt
// {address}. Administrator only. It reads what the server holds for the
// address and writes the Panel's row with the size limit found there; it
// changes nothing on the server.
func (p *Panel) handleAdoptEmailAccount(w http.ResponseWriter, r *http.Request, domainID int) {
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	caller := currentCaller(r)
	if caller == nil || caller.APIToken != nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	var request struct {
		Address string `json:"address"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	domain, err := p.domainNameByIDStrict(r.Context(), domainID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	address, err := transport.CanonicalMailboxForDomain(request.Address, domain)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, err.Error())
		return
	}
	exists, err := p.emailAccountExists(r.Context(), address)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if exists {
		writeMailAccountExists(w, address)
		return
	}
	if !p.enforceDomainQuota(w, r, domainID, quotaMail) {
		return
	}

	p.mailMutationMu.Lock()
	defer p.mailMutationMu.Unlock()
	if err := p.ensureMailDomainMutable(r.Context(), domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	var found transport.MailboxOnServerResponse
	if err := p.callAgentContext(r.Context(), "Agent.MailboxOnServerV1", &transport.MailboxOnServerRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: address,
	}, &found); err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	refuse := func(reason, message string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodeMailAccountAdoptRefused, Reason: reason,
			Vars: map[string]string{"address": address}})
	}
	switch {
	case !found.InUsers && !found.InMap:
		refuse(mailAdoptGone, "the server no longer holds a mailbox named "+address+", so nothing was recorded. Create it again.")
		return
	case !found.InUsers || !found.InMap:
		refuse(mailAdoptIncomplete, "the server holds only part of the mailbox "+address+" (one of its two lines, in /etc/dovecot/users and /etc/postfix/vmailbox), so nothing was recorded. "+
			"The server owner removes the line that is left; then create the mailbox again.")
		return
	case !found.OwnFolder || found.QuotaMB <= 0:
		refuse(mailAdoptElsewhere, "the mailbox "+address+" on the server is not in the form CelikPanel writes (its folder, account or size limit is set differently), so nothing was recorded: "+
			"it is left to the server owner. Choose another name, or the server owner removes it first.")
		return
	}
	result, err := p.db.GetDB().ExecContext(r.Context(),
		"INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, ?, ?, ?)",
		domainID, address, "managed-by-agent", found.QuotaMB)
	if err != nil {
		if isEmailAccountAddressTaken(err) {
			writeMailAccountExists(w, address)
			return
		}
		writeServerError(w, err)
		return
	}
	newID, err := result.LastInsertId()
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.audit(r, "mail.account.adopt", "email_account", int(newID))
	// The same follow-ups as a mailbox created here: it is an ordinary
	// mailbox from now on (its size limit is the one found on the server).
	detached := context.WithoutCancel(r.Context())
	p.reconcileAutoresponderAddressesLocked(detached, "mailbox_added")
	p.queueAutomaticDKIMForMailbox(detached, domainID, domain)
	p.requestMailQuotaReconcile()
	p.requestOutgoingApply()
	p.requestDomainMailOptionsApply(detached)
	log.Printf("[mail] %s: adopted into domain %d by an administrator (size limit %d MB, read from the server)", address, domainID, found.QuotaMB)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": newID, "quota_mb": found.QuotaMB})
}
