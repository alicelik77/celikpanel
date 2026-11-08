package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/passwordpolicy"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Routes of mailbox recovery (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md).
//
// Signed in, with the domain's mail rights (the same gate as a password
// change: kind "mail", manage for PUT and DELETE):
//
//	GET    /api/v1/domains/{id}/mail/accounts/recovery?id=  the mailbox's address and state
//	PUT    /api/v1/domains/{id}/mail/accounts/recovery      {id, address, lang}: set, mail the confirmation
//	DELETE /api/v1/domains/{id}/mail/accounts/recovery?id=  remove
//
// Public (the token, or nothing at all, is the credential):
//
//	POST /api/v1/auth/mailbox-recovery/request  {address, lang} -> always 202 {"accepted":true}
//	POST /api/v1/auth/mailbox-recovery/confirm  {token}          -> {mailbox}
//	POST /api/v1/auth/mailbox-recovery/link     {token}          -> {mailbox}
//	POST /api/v1/auth/mailbox-recovery/reset    {token, new_password}
//
// Kurtarma adresi rotaları: oturumlu yönetim ve herkese açık istek, onay,
// bağlantı ve sıfırlama.

const (
	mailboxRecoveryRequestPath = "/api/v1/auth/mailbox-recovery/request"
	mailboxRecoveryConfirmPath = "/api/v1/auth/mailbox-recovery/confirm"
	mailboxRecoveryLinkPath    = "/api/v1/auth/mailbox-recovery/link"
	mailboxRecoveryResetPath   = "/api/v1/auth/mailbox-recovery/reset"

	errCodeMailboxRecoveryInvalidAddress = "mailbox_recovery_address_invalid"
	errCodeMailboxRecoverySelf           = "mailbox_recovery_address_self"
	errCodeMailboxRecoveryTooMany        = "mailbox_recovery_too_many"
	errCodeMailboxRecoveryLinkInvalid    = "mailbox_recovery_link_invalid"
	errCodeMailboxRecoveryLoginDisabled  = "mailbox_recovery_login_disabled"
	errCodeMailboxRecoveryUnknown        = "mailbox_recovery_result_unknown"
	errCodeMailboxRecoveryBusy           = "mailbox_recovery_busy"
	errCodeMailboxRecoveryNotSent        = "mailbox_recovery_mail_not_sent"
)

// English originals (the screens use their own words for each code).
const (
	mailboxRecoveryInvalidAddressMessage = "The recovery address is not a valid email address, so nothing was changed. Type a complete address such as name@example.com."
	mailboxRecoverySelfMessage           = "A mailbox cannot be its own recovery address, so nothing was changed. Use an address at another mailbox the person can read."
	mailboxRecoveryTooManyMessage        = "Too many confirmation messages for this mailbox or from your account, so nothing was sent. Wait an hour, then try again."
	mailboxRecoveryLinkInvalidMessage    = "This link is not valid any more: it was used, replaced by a newer one, expired, or the mailbox changed. Nothing was changed. Ask for a new link."
	mailboxRecoveryLoginDisabledMessage  = "This mailbox cannot sign in at the moment (it is suspended or turned off), so its password was not changed. Ask your provider."
	mailboxRecoveryUnknownMessage        = "It is not known whether the new password was set: the server did not answer in time. Try signing in with the new password; if it does not work, ask for a new link."
	mailboxRecoveryBusyMessage           = "Another mail change is running on the server, so the password was not changed and the link still works. Try again in a minute."
	mailboxRecoveryNotSentMessage        = "The recovery address was saved, but the server's mail system did not accept the confirmation message, so it was not sent. Press Send again in a few minutes; if it keeps failing, the administrator checks the mail service."
)

var (
	mailboxRecoveryRequestLimiter = newRateLimiter(5, 15*time.Minute)  // per client address: forgot-password requests
	mailboxRecoveryMailboxLimiter = newRateLimiter(3, time.Hour)       // per mailbox: reset mails
	mailboxRecoveryLinkLimiter    = newRateLimiter(30, 15*time.Minute) // per client address: link reads and uses
	mailboxRecoveryConfirmLimiter = newRateLimiter(3, time.Hour)       // per mailbox: confirmation mails
	mailboxRecoverySetterLimiter  = newRateLimiter(20, time.Hour)      // per signed-in person: confirmation mails
)

// resetMailboxRecoveryLimiters clears the limiters in place; the variables
// are never reassigned, because the background sends read them.
func resetMailboxRecoveryLimiters() {
	for _, limiter := range []*rateLimiter{mailboxRecoveryRequestLimiter, mailboxRecoveryMailboxLimiter,
		mailboxRecoveryLinkLimiter, mailboxRecoveryConfirmLimiter, mailboxRecoverySetterLimiter} {
		limiter.reset()
	}
}

// goMailboxRecovery runs a send that happens after the public answer, tracked
// so the panel (and its tests) can wait for every one to finish.
func (p *Panel) goMailboxRecovery(send func()) {
	p.mailboxRecoverySends.Add(1)
	go func() {
		defer p.mailboxRecoverySends.Done()
		send()
	}()
}

// waitMailboxRecoverySends waits until no send started by goMailboxRecovery
// is still running.
func (p *Panel) waitMailboxRecoverySends() { p.mailboxRecoverySends.Wait() }

func init() {
	// The public routes join the account routes' exact list (path and its
	// one method), which isPublicPath reads.
	accountRecoveryPublicPaths[mailboxRecoveryRequestPath] = http.MethodPost
	accountRecoveryPublicPaths[mailboxRecoveryConfirmPath] = http.MethodPost
	accountRecoveryPublicPaths[mailboxRecoveryLinkPath] = http.MethodPost
	accountRecoveryPublicPaths[mailboxRecoveryResetPath] = http.MethodPost
}

func writeMailboxRecoveryLinkInvalid(w http.ResponseWriter) {
	writeCodedError(w, http.StatusGone, errCodeMailboxRecoveryLinkInvalid, mailboxRecoveryLinkInvalidMessage, "")
}

// ---- Signed in: the manager's dialog ---------------------------------------

type mailboxRecoveryView struct {
	Mailbox     string            `json:"mailbox"`
	State       string            `json:"state"`
	VoidReason  string            `json:"void_reason,omitempty"`
	Address     string            `json:"address,omitempty"`
	SetAt       string            `json:"set_at,omitempty"`
	ConfirmedAt string            `json:"confirmed_at,omitempty"`
	Mail        accountMailStatus `json:"mail"`
	RequestPage string            `json:"request_page,omitempty"`
}

func (p *Panel) mailboxRecoveryView(ctx context.Context, row mailboxRecoveryRow) (mailboxRecoveryView, error) {
	state, reason, err := row.state(ctx, p.db.GetDB())
	if err != nil {
		return mailboxRecoveryView{}, err
	}
	setup := p.accountMailSetup(ctx)
	view := mailboxRecoveryView{Mailbox: row.mailbox, State: state, VoidReason: reason, Mail: setup.accountMailStatus}
	if row.present {
		view.Address, view.SetAt = row.address, row.setAt
		if row.confirmedAt.Valid {
			view.ConfirmedAt = row.confirmedAt.String
		}
	}
	if setup.baseURL != "" {
		view.RequestPage = setup.baseURL + mailboxRecoveryRequestPage
	}
	return view, nil
}

func (p *Panel) handleMailboxRecovery(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		p.handleGetMailboxRecovery(w, r, domainID)
	case http.MethodPut:
		p.handleSetMailboxRecovery(w, r, domainID)
	case http.MethodDelete:
		p.handleRemoveMailboxRecovery(w, r, domainID)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func mailboxRecoveryAccountID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	return id, err == nil && id > 0
}

func (p *Panel) handleGetMailboxRecovery(w http.ResponseWriter, r *http.Request, domainID int) {
	accountID, ok := mailboxRecoveryAccountID(r)
	if !ok {
		writeClientError(w, http.StatusBadRequest, "invalid mailbox id")
		return
	}
	row, err := loadMailboxRecovery(r.Context(), p.db.GetDB(), domainID, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	view, err := p.mailboxRecoveryView(r.Context(), row)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}

// handleSetMailboxRecovery stores the address unconfirmed and mails the
// confirmation link to it. The same, already confirmed address changes
// nothing and says so.
func (p *Panel) handleSetMailboxRecovery(w http.ResponseWriter, r *http.Request, domainID int) {
	var request struct {
		ID      int    `json:"id"`
		Address string `json:"address"`
		Lang    string `json:"lang"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || request.ID <= 0 {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	address, ok := canonicalRecoveryAddress(request.Address)
	if !ok {
		writeCodedError(w, http.StatusBadRequest, errCodeMailboxRecoveryInvalidAddress, mailboxRecoveryInvalidAddressMessage, "")
		return
	}
	if err := p.ensureMailDomainMutable(r.Context(), domainID); err != nil {
		writeMailDomainMutationError(w, err)
		return
	}
	db := p.db.GetDB()
	row, err := loadMailboxRecovery(r.Context(), db, domainID, request.ID)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if address == row.mailbox {
		writeCodedError(w, http.StatusBadRequest, errCodeMailboxRecoverySelf, mailboxRecoverySelfMessage, "")
		return
	}
	state, _, err := row.state(r.Context(), db)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if state == mailboxRecoveryStateConfirmed && row.address == address {
		view, err := p.mailboxRecoveryView(r.Context(), row)
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"unchanged": true, "recovery": view})
		return
	}
	setup := p.accountMailSetup(r.Context())
	if !setup.Available {
		writeAccountMailUnavailable(w, setup.Reason)
		return
	}
	setterID, _ := auditPerson(r)
	if !mailboxRecoveryConfirmLimiter.allow("mailbox:"+strconv.Itoa(row.accountID)) ||
		!mailboxRecoverySetterLimiter.allow("user:"+strconv.Itoa(setterID)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeMailboxRecoveryTooMany, mailboxRecoveryTooManyMessage, "")
		return
	}
	lang := accountMailLang(request.Lang)
	var setter any
	if setterID > 0 {
		setter = setterID
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO mailbox_recovery_addresses (email_account_id, address, subscription_id, owner_user_id, local_domain_id, lang, set_by, set_at, confirmed_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?, ?, NULL)
		ON CONFLICT(email_account_id) DO UPDATE SET
		    address = excluded.address, subscription_id = excluded.subscription_id,
		    owner_user_id = excluded.owner_user_id, local_domain_id = NULL, lang = excluded.lang,
		    set_by = excluded.set_by, set_at = excluded.set_at, confirmed_at = NULL`,
		row.accountID, address, row.subscriptionID, row.ownerID, lang, setter,
		accountLinkTime(accountLinkNow())); err != nil {
		writeServerError(w, err)
		return
	}
	if err := revokeMailboxRecoveryLinks(r.Context(), tx, row.accountID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "mail.account.recovery.set", "email_account", row.accountID, map[string]any{"address": address})
	token, err := issueMailboxRecoveryLink(r.Context(), db, mailboxRecoveryPurposeConfirm, row.accountID, address, "", mailboxRecoveryConfirmTTL)
	if err != nil {
		writeServerError(w, err)
		return
	}
	body := mailboxRecoveryMail{
		kind: mailboxRecoveryMailConfirm, lang: lang, host: setup.host, mailbox: row.mailbox,
		link: setup.baseURL + mailboxRecoveryConfirmPage + "#t=" + token,
	}
	subject, text := mailboxRecoveryMailText(body)
	if err := p.sendAccountMail(r.Context(), setup, address, accountMailContent{
		kind: accountMailMailboxRecovery, lang: lang, subject: subject, body: text,
	}); err != nil {
		log.Printf("[mailbox-recovery] confirmation for mailbox %d not handed over: %v", row.accountID, err)
		writeCodedError(w, http.StatusBadGateway, errCodeMailboxRecoveryNotSent, mailboxRecoveryNotSentMessage, "")
		return
	}
	saved, err := loadMailboxRecovery(r.Context(), db, domainID, row.accountID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	view, err := p.mailboxRecoveryView(r.Context(), saved)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sent": true, "recovery": view})
}

func (p *Panel) handleRemoveMailboxRecovery(w http.ResponseWriter, r *http.Request, domainID int) {
	accountID, ok := mailboxRecoveryAccountID(r)
	if !ok {
		writeClientError(w, http.StatusBadRequest, "invalid mailbox id")
		return
	}
	db := p.db.GetDB()
	if _, err := loadMailboxRecovery(r.Context(), db, domainID, accountID); errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "account not found")
		return
	} else if err != nil {
		writeServerError(w, err)
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(r.Context(), `DELETE FROM mailbox_recovery_addresses WHERE email_account_id = ?`, accountID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	removed, _ := result.RowsAffected()
	if err := revokeMailboxRecoveryLinks(r.Context(), tx, accountID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	if removed > 0 {
		p.audit(r, "mail.account.recovery.removed", "email_account", accountID)
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"removed": removed > 0})
}

// ---- Public: forgot the mailbox password -----------------------------------

func (p *Panel) handleMailboxRecoveryRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !mailboxRecoveryRequestLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Address string `json:"address"`
		Lang    string `json:"lang"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	mailbox, err := transport.CanonicalMailAddress(strings.TrimSpace(req.Address))
	if err != nil || len(mailbox) > 254 {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	setup := p.accountMailSetup(r.Context())
	if !setup.Available {
		writeAccountMailUnavailable(w, setup.Reason)
		return
	}
	// Everything about the mailbox happens after the answer, so neither the
	// answer nor its time tells whether the mailbox or an address exists.
	background := r.Clone(context.WithoutCancel(r.Context()))
	lang := req.Lang
	p.goMailboxRecovery(func() { p.sendMailboxResetLink(background, setup, strings.ToLower(mailbox), lang) })
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
}

// mailboxRecoveryUsable: the mailbox may be reset now (not suspended, not
// being deleted).
func (p *Panel) mailboxRecoveryUsable(ctx context.Context, row mailboxRecoveryRow) (bool, error) {
	if err := p.ensureMailDomainMutable(ctx, row.domainID); err != nil {
		if errors.Is(err, errDomainDeletionPending) {
			return false, nil
		}
		return false, err
	}
	suspended, err := p.subscriptionSuspended(ctx, row.subscriptionID)
	if errors.Is(err, errSuspensionNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !suspended, nil
}

// sendMailboxResetLink runs after the public answer. Every "no" is silent.
func (p *Panel) sendMailboxResetLink(r *http.Request, setup accountMailSetup, mailbox, lang string) {
	ctx := r.Context()
	db := p.db.GetDB()
	row, err := loadMailboxRecoveryByAddress(ctx, db, mailbox)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[mailbox-recovery] lookup failed: %v", err)
		}
		return
	}
	state, _, err := row.state(ctx, db)
	if err != nil || state != mailboxRecoveryStateConfirmed {
		return
	}
	if usable, err := p.mailboxRecoveryUsable(ctx, row); err != nil || !usable {
		return
	}
	if !mailboxRecoveryMailboxLimiter.allow("mailbox:" + strconv.Itoa(row.accountID)) {
		return
	}
	var fingerprint transport.MailPasswordFingerprintResponse
	if err := p.callAgentContext(ctx, "Agent.MailPasswordFingerprintV1",
		&transport.MailPasswordFingerprintRequest{Email: row.mailbox}, &fingerprint); err != nil {
		log.Printf("[mailbox-recovery] fingerprint of mailbox %d not read", row.accountID)
		return
	}
	if !fingerprint.Found || fingerprint.LoginDisabled || !strings.HasPrefix(fingerprint.Fingerprint, "sha256:") {
		return
	}
	token, err := issueMailboxRecoveryLink(ctx, db, mailboxRecoveryPurposeReset, row.accountID, row.address,
		fingerprint.Fingerprint, mailboxRecoveryResetTTL)
	if err != nil {
		log.Printf("[mailbox-recovery] reset link for mailbox %d not stored: %v", row.accountID, err)
		return
	}
	p.auditAs(r, 0, "mail.account.recovery.reset_requested", "email_account", row.accountID)
	if lang == "" {
		lang = row.lang
	}
	subject, text := mailboxRecoveryMailText(mailboxRecoveryMail{
		kind: mailboxRecoveryMailReset, lang: accountMailLang(lang), host: setup.host, mailbox: row.mailbox,
		link:        setup.baseURL + mailboxRecoveryResetPage + "#t=" + token,
		requestPage: setup.baseURL + mailboxRecoveryRequestPage,
	})
	if err := p.sendAccountMail(ctx, setup, row.address, accountMailContent{
		kind: accountMailMailboxRecovery, subject: subject, body: text,
	}); err != nil {
		log.Printf("[mailbox-recovery] reset mail for mailbox %d not handed over: %v", row.accountID, err)
		p.auditAs(r, 0, "mail.account.recovery.mail_failed", "email_account", row.accountID)
	}
}

// ---- Public: the confirmation link ------------------------------------------

func (p *Panel) handleMailboxRecoveryConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !mailboxRecoveryLinkLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	link, err := lookupMailboxRecoveryLink(ctx, tx, mailboxRecoveryPurposeConfirm, req.Token)
	if errors.Is(err, errMailboxRecoveryLinkInvalid) {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	localDomain, err := mailboxRecoveryDomainOf(ctx, tx, link.address)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var local any
	if localDomain > 0 {
		local = localDomain
	}
	now := accountLinkTime(accountLinkNow())
	consumed, err := tx.ExecContext(ctx, `
		UPDATE mailbox_recovery_links SET consumed_at = ?
		WHERE id = ? AND consumed_at IS NULL AND revoked_at IS NULL`, now, link.id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := consumed.RowsAffected(); n != 1 {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}
	updated, err := tx.ExecContext(ctx, `
		UPDATE mailbox_recovery_addresses SET confirmed_at = ?, local_domain_id = ?
		WHERE email_account_id = ? AND address = ? AND confirmed_at IS NULL`,
		now, local, link.accountID, link.address)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := updated.RowsAffected(); n != 1 {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditAsDetailed(r, 0, "mail.account.recovery.confirmed", "email_account", link.accountID, map[string]any{"address": link.address})
	// The mailbox is told its recovery address is active (review T3).
	if setup := p.accountMailSetup(ctx); setup.Available {
		mail := mailboxRecoveryMail{
			kind: mailboxRecoveryMailActivated, lang: link.mailbox.lang, host: setup.host,
			mailbox: link.mailbox.mailbox, masked: maskedRecoveryAddress(link.address),
			requestPage: setup.baseURL + mailboxRecoveryRequestPage,
		}
		background := r.Clone(context.WithoutCancel(ctx))
		p.goMailboxRecovery(func() { p.sendMailboxNotice(background, setup, link.mailbox.mailbox, link.accountID, mail) })
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"mailbox": link.mailbox.mailbox})
}

func (p *Panel) sendMailboxNotice(r *http.Request, setup accountMailSetup, mailbox string, accountID int, mail mailboxRecoveryMail) {
	subject, text := mailboxRecoveryMailText(mail)
	if err := p.sendAccountMail(r.Context(), setup, mailbox, accountMailContent{
		kind: accountMailMailboxRecovery, subject: subject, body: text,
	}); err != nil {
		log.Printf("[mailbox-recovery] notice to mailbox %d not handed over: %v", accountID, err)
		p.auditAs(r, 0, "mail.account.recovery.notice_failed", "email_account", accountID)
	}
}

// ---- Public: the reset link -------------------------------------------------

// handleMailboxRecoveryLink tells the page whether a reset link can still be
// used and for which mailbox, before anyone types a password.
func (p *Panel) handleMailboxRecoveryLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !mailboxRecoveryLinkLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	link, err := lookupMailboxRecoveryLink(r.Context(), p.db.GetDB(), mailboxRecoveryPurposeReset, req.Token)
	if err == nil {
		var usable bool
		usable, err = p.mailboxRecoveryUsable(r.Context(), link.mailbox)
		if err == nil && !usable {
			err = errMailboxRecoveryLinkInvalid
		}
	}
	if errors.Is(err, errMailboxRecoveryLinkInvalid) {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"mailbox": link.mailbox.mailbox})
}

// handleMailboxRecoveryReset sets the new mailbox password a reset link
// allows. The link is consumed first (one transaction); the Agent then sets
// the password only while the field is the one the link was made for. A
// refusal that changed nothing (busy, a file problem) gives the link back.
func (p *Panel) handleMailboxRecoveryReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !mailboxRecoveryLinkLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	if refusal := passwordpolicy.CheckField(req.NewPassword, "mailbox"); refusal != nil {
		writePasswordRefusal(w, refusal)
		return
	}
	if err := transport.ValidateMailboxPassword(req.NewPassword); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	if !wellFormedAccountLinkToken(req.Token) {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}

	p.mailMutationMu.Lock()
	defer p.mailMutationMu.Unlock()
	ctx := r.Context()
	db := p.db.GetDB()
	link, consumedAt, err := p.claimMailboxResetLink(ctx, db, req.Token)
	if errors.Is(err, errMailboxRecoveryLinkInvalid) {
		writeMailboxRecoveryLinkInvalid(w)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	giveBack := func() {
		if _, err := db.ExecContext(context.WithoutCancel(ctx), `
			UPDATE mailbox_recovery_links SET consumed_at = NULL
			WHERE id = ? AND consumed_at = ? AND revoked_at IS NULL`, link.id, consumedAt); err != nil {
			log.Printf("[mailbox-recovery] link %d could not be given back: %v", link.id, err)
		}
	}

	var response transport.SetMailPasswordIfUnchangedResponse
	if err := p.callAgentContext(ctx, "Agent.SetMailPasswordIfUnchangedV1", &transport.SetMailPasswordIfUnchangedRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit),
		Email:               link.mailbox.mailbox,
		NewPassword:         req.NewPassword,
		ExpectedFingerprint: link.fingerprint,
	}, &response); err != nil {
		// Unknown: the Agent may or may not have written the new hash. The
		// link stays used; the fingerprint makes a second use harmless.
		p.auditAs(r, 0, "mail.account.recovery.reset_unknown", "email_account", link.accountID)
		writeCodedError(w, http.StatusGatewayTimeout, errCodeMailboxRecoveryUnknown, mailboxRecoveryUnknownMessage, "")
		return
	}
	switch {
	case response.Applied:
	case response.Changed:
		writeMailboxRecoveryLinkInvalid(w)
		return
	case response.LoginDisabled:
		writeCodedError(w, http.StatusConflict, errCodeMailboxRecoveryLoginDisabled, mailboxRecoveryLoginDisabledMessage, "")
		return
	default:
		giveBack()
		if response.Error == transport.MailConfigurationBusy {
			writeCodedError(w, http.StatusServiceUnavailable, errCodeMailboxRecoveryBusy, mailboxRecoveryBusyMessage, "")
			return
		}
		log.Printf("[mailbox-recovery] reset of mailbox %d refused by the Agent", link.accountID)
		writeServerError(w, errors.New("the mailbox password was not changed"))
		return
	}

	answer := map[string]any{"success": true}
	var ended transport.EndMailSessionsResponse
	if err := p.callAgentContext(ctx, "Agent.EndMailSessionsV1", &transport.EndMailSessionsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Email: link.mailbox.mailbox,
	}, &ended); err != nil || !ended.Applied {
		answer["sessions_not_ended"] = true
	}
	// A security pause ends with a new password (owner decision, 7 Nov
	// 2026); an administrator's manual pause stays.
	if resumed, err := p.resumeOutgoingAfterPassword(context.WithoutCancel(ctx), link.accountID); resumed {
		answer["sending_resumed"] = err == nil
		if err != nil {
			answer["sending_resume_pending"] = true
		}
	} else if p.mailboxPauseReason(ctx, link.accountID) == "manual" {
		answer["sending_paused"] = true
	}
	p.auditAs(r, 0, "mail.account.recovery.reset_completed", "email_account", link.accountID)
	if setup := p.accountMailSetup(ctx); setup.Available {
		mail := mailboxRecoveryMail{
			kind: mailboxRecoveryMailWasReset, lang: link.mailbox.lang, host: setup.host,
			mailbox: link.mailbox.mailbox, masked: maskedRecoveryAddress(link.address),
		}
		background := r.Clone(context.WithoutCancel(ctx))
		p.goMailboxRecovery(func() { p.sendMailboxNotice(background, setup, link.mailbox.mailbox, link.accountID, mail) })
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

// claimMailboxResetLink finds and consumes the link in one transaction and
// revokes the mailbox's other open reset links.
func (p *Panel) claimMailboxResetLink(ctx context.Context, db *sql.DB, token string) (mailboxRecoveryLinkRow, string, error) {
	// Suspension and deletion are read before the transaction (they are
	// other features' rows); the link itself is read again inside it.
	first, err := lookupMailboxRecoveryLink(ctx, db, mailboxRecoveryPurposeReset, token)
	if err != nil {
		return first, "", err
	}
	usable, err := p.mailboxRecoveryUsable(ctx, first.mailbox)
	if err != nil {
		return first, "", err
	}
	if !usable {
		return first, "", errMailboxRecoveryLinkInvalid
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return mailboxRecoveryLinkRow{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	link, err := lookupMailboxRecoveryLink(ctx, tx, mailboxRecoveryPurposeReset, token)
	if err != nil {
		return link, "", err
	}
	if link.id != first.id {
		return link, "", errMailboxRecoveryLinkInvalid
	}
	now := accountLinkTime(accountLinkNow())
	result, err := tx.ExecContext(ctx, `
		UPDATE mailbox_recovery_links SET consumed_at = ?
		WHERE id = ? AND consumed_at IS NULL AND revoked_at IS NULL`, now, link.id)
	if err != nil {
		return link, "", err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return link, "", errMailboxRecoveryLinkInvalid
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE mailbox_recovery_links SET revoked_at = ?
		WHERE email_account_id = ? AND purpose = ? AND id <> ? AND consumed_at IS NULL AND revoked_at IS NULL`,
		now, link.accountID, mailboxRecoveryPurposeReset, link.id); err != nil {
		return link, "", err
	}
	if err := tx.Commit(); err != nil {
		return link, "", err
	}
	return link, now, nil
}

// mailboxPauseReason is the mailbox's outgoing pause reason, "" when it is
// not paused (or the outgoing limits never saw it).
func (p *Panel) mailboxPauseReason(ctx context.Context, accountID int) string {
	var reason sql.NullString
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(paused_reason, 'spike') FROM mailbox_outgoing
		WHERE email_account_id = ? AND paused_at IS NOT NULL`, accountID).Scan(&reason)
	if err != nil {
		return ""
	}
	return reason.String
}
