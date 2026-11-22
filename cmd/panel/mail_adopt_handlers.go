package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/passwordpolicy"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A mailbox on the server without a row in the Panel (alpha.144, alpha.143
// lab item 6). A creation whose record could not be written (the Panel
// stopped between the Agent's work and its own row) left the mailbox in the
// server's mail files; creating it again was answered "internal error".
// Now it is said plainly: the administrator may adopt it into its domain
// and then keep it or delete it like any mailbox; anybody else is told to
// ask the administrator. The sentence names only the address asked for,
// inside the person's own domain: nothing of another account.
//
// Adoption never hands over mail or a password it cannot account for
// (review of PR #355, finding 3): it is allowed only when the mailbox's
// folder holds no message (no file in any cur/ or new/ folder of it), and
// it always sets the new password the administrator types, through the same
// Agent step as a password change, so no earlier password survives. Then
// the mailbox follows the domain like a new one (signing key, first-day
// sending limit, size limit).
//
// Panelde satırı olmadan sunucuda bulunan posta kutusu: yönetici onu alan
// adına sahiplenebilir (sunucuyu okur, satırı yazar; postaya dokunmaz);
// başkasına yöneticiye sorması söylenir.

const (
	errCodeMailAccountOnServerOnly = "MAIL_ACCOUNT_ON_SERVER_ONLY"
	// errCodeMailAccountAdoptRefused: the administrator's adoption found
	// something it does not record by itself; the reason says what.
	errCodeMailAccountAdoptRefused = "MAIL_ACCOUNT_ADOPT_REFUSED"
	// errCodeMailAccountAdoptRecordNotWritten: the password was replaced,
	// the row could not be written; adopting again is safe.
	errCodeMailAccountAdoptRecordNotWritten = "MAIL_ACCOUNT_ADOPT_RECORD_NOT_WRITTEN"
)

// Reasons of MAIL_ACCOUNT_ADOPT_REFUSED.
const (
	mailAdoptGone       = "gone"           // the server holds no line for it now
	mailAdoptIncomplete = "incomplete"     // only one of the two lines exists
	mailAdoptElsewhere  = "elsewhere"      // its lines are not CelikPanel's form
	mailAdoptMailFound  = "mail_found"     // its folder holds messages
	mailAdoptUnknown    = "folder_unknown" // its folder could not be counted
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
				"Adopt it into this domain (allowed while it holds no mail; you give it a new password), or choose another name.",
			Code: errCodeMailAccountOnServerOnly, Reason: "admin", Vars: vars,
		})
		return
	}
	writeCodedErrorVars(w, http.StatusConflict, errCodeMailAccountOnServerOnly,
		"a mailbox named "+address+" already exists on the server without a record in the panel, so nothing was changed. "+
			"Ask the server's administrator to look at it, or choose another name.", "", nil, vars)
}

// handleAdoptEmailAccount: POST /api/v1/domains/{id}/mail/accounts/adopt
// {address, password}. Administrator only. It reads what the server holds
// for the address; only an empty mailbox in CelikPanel's own form is
// adopted: the row is written with the size limit found there and the
// mailbox gets the new password (the earlier one is replaced).
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
		Address  string `json:"address"`
		Password string `json:"password"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if refusal := passwordpolicy.CheckField(request.Password, "mailbox"); refusal != nil {
		writePasswordRefusal(w, refusal)
		return
	}
	if err := transport.ValidateMailboxPassword(request.Password); err != nil {
		writeClientError(w, http.StatusBadRequest, err.Error())
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
	case found.MessagesUnknown:
		refuse(mailAdoptUnknown, "whether the mailbox "+address+" on the server holds mail could not be checked, so nothing was recorded: the server owner decides on the server what becomes of it.")
		return
	case found.Messages > 0:
		refuse(mailAdoptMailFound, "mail was found in the mailbox "+address+" on the server, so it was not adopted and nothing was changed: "+
			"whose mail it is is not known to CelikPanel. The server owner decides on the server what becomes of it.")
		return
	}
	// The earlier password is replaced first, through the password
	// change's own Agent step, with no panel database write open (review
	// of PR #355 fixes, finding 3).
	var replaced transport.MailMutationResponse
	if err := p.callAgentContext(r.Context(), "Agent.UpdateMailPassword", &transport.UpdateMailPasswordRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: address, NewPassword: request.Password,
	}, &replaced); err != nil || !replaced.Applied {
		if err != nil && agentAnsweredExactly(err, transport.MailConfigurationBusy) {
			writeServerError(w, agentMutationBusy())
			return
		}
		writeServerError(w, errors.New("the mailbox's new password was not confirmed by the Agent, so it was not adopted"))
		return
	}
	// Then the row, in a short write of its own. When it cannot be written
	// the answer says so exactly: the password was replaced, the record was
	// not; pressing Adopt again is safe (the same checks, the same password
	// step, then the row).
	result, err := p.db.GetDB().ExecContext(r.Context(),
		"INSERT INTO email_accounts (domain_id, address, password_hash, quota_mb) VALUES (?, ?, ?, ?)",
		domainID, address, "managed-by-agent", found.QuotaMB)
	var newID int64
	if err == nil {
		newID, err = result.LastInsertId()
	}
	if err != nil {
		if isEmailAccountAddressTaken(err) {
			writeMailAccountExists(w, address)
			return
		}
		log.Printf("[mail] %s: its password was replaced, but its record could not be written: %v", address, err)
		writeCodedErrorVars(w, http.StatusInternalServerError, errCodeMailAccountAdoptRecordNotWritten,
			"the mailbox "+address+"'s password was replaced, but its record could not be written in the panel. Press Adopt again: it is safe, and it uses the password you type then.",
			"", nil, map[string]string{"address": address})
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
