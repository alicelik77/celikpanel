package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Password reset by email, invitation acceptance and address confirmation
// (v0.1.0-alpha.114, P6). See docs/ACCOUNT-ACCESS.md.
//
// Public routes (no session; isPublicPath lists them exactly):
//
//	GET  /api/v1/auth/recovery/mail-status      can this server send account mail, and if not why
//	POST /api/v1/auth/recovery/request          {identifier, lang} -> always 202 {"accepted":true}
//	POST /api/v1/auth/recovery/link             {token, purpose} -> {username} or 410
//	POST /api/v1/auth/recovery/reset            {token, new_password}
//	POST /api/v1/auth/recovery/accept-invitation {token, new_password}
//	POST /api/v1/auth/recovery/verify-email     {token}
//
// Signed in:
//
//	GET/POST /api/v1/auth/email/verification    the own address's state / send a confirmation link
//
// Rules (OWASP Forgot Password Cheat Sheet):
//   - The request answers the same for an unknown name, a suspended account,
//     an unconfirmed address and a known one; the message is handed to the
//     mail system after the answer, so the time does not tell either.
//   - Limits: per client address on every public route; per account on the
//     messages one account can receive. A per-account limit never answers
//     differently: the message is simply not sent.
//   - A reset is sent only to a confirmed address (ROADMAP v0.3), for every
//     role; the administrator's CLI recovery (celikpanel admin) stays.
//   - Setting the password drops every session of that account and its
//     pending two-step sign-ins; it never signs the person in.
//   - Two-step verification stays on after a reset.
//
// Parola sıfırlama, davet kabulü ve adres onayı. İstek bilinmeyen ad, askıdaki
// hesap, onaysız adres ve bilinen hesap için aynı yanıtı verir; ileti yanıttan
// sonra posta sistemine verilir. Sıfırlama yalnız onaylı adrese gider. Parola
// belirlenince hesabın bütün oturumları düşer; kişi kendiliğinden oturum açmaz.

const (
	recoveryMailStatusPath      = "/api/v1/auth/recovery/mail-status"
	recoveryRequestPath         = "/api/v1/auth/recovery/request"
	recoveryLinkPath            = "/api/v1/auth/recovery/link"
	recoveryResetPath           = "/api/v1/auth/recovery/reset"
	recoveryAcceptInvitePath    = "/api/v1/auth/recovery/accept-invitation"
	recoveryVerifyEmailPath     = "/api/v1/auth/recovery/verify-email"
	ownEmailVerificationPath    = "/api/v1/auth/email/verification"
	accountRecoveryBodyMaxBytes = 8 << 10
	accountPasswordMaxBytes     = 1024

	errCodeAccountRecoveryInvalid  = "account_recovery_request_invalid"
	errCodeAccountRecoveryTooMany  = "account_recovery_too_many"
	errCodeAccountMailUnavailable  = "account_mail_unavailable"
	errCodeAccountMailSendFailed   = "account_mail_send_failed"
	errCodeAccountLinkInvalid      = "account_link_invalid"
	errCodeAccountPasswordShort    = "account_password_too_short"
	errCodeAccountPasswordLong     = "account_password_too_long"
	errCodeAccountEmailMissing     = "account_email_missing"
	errCodeAccountEmailVerified    = "account_email_already_verified"
	errCodeAccountVerifyTooMany    = "account_email_verification_too_many"
	errCodeAccountRecoveryConflict = "account_link_changed_elsewhere"
)

// English originals of the coded answers (the screens use web/src/i18n).
// D-024 order: what happened, what was not changed, who acts and how.
const (
	accountRecoveryInvalidMessage  = "The request could not be read, so nothing was sent. Reload the page and try again."
	accountRecoveryTooManyMessage  = "Too many requests from this address, so nothing was sent. Wait 15 minutes, then try again."
	accountMailUnavailableMessage  = "This server cannot send account mail, so nothing was sent. Ask your administrator or provider to set a temporary password for you."
	accountMailSendFailedMessage   = "The server's mail system did not accept the message, so nothing was sent. Try again in a few minutes; if it keeps failing, ask your administrator to check the mail service."
	accountLinkInvalidMessage      = "This link is not valid any more: it was used, replaced by a newer one, or it expired. Nothing was changed. Ask for a new link."
	accountPasswordShortMessage    = "The password must be at least 8 characters, so nothing was changed. Enter a longer password."
	accountPasswordLongMessage     = "The password is too long, so nothing was changed. Use at most 1024 characters."
	accountEmailMissingMessage     = "Your account has no email address, so nothing was sent. Add an address first."
	accountEmailVerifiedMessage    = "Your email address is already confirmed, so nothing was sent."
	accountVerifyTooManyMessage    = "Too many confirmation messages for this account, so nothing was sent. Wait an hour, then try again."
	accountRecoveryConflictMessage = "Your account was changed somewhere else while this was being saved, so nothing was changed. Ask for a new link."
)

// The limiters. Package-level so the Panel struct is unchanged; tests reset
// them with resetAccountRecoveryLimiters.
var (
	recoveryRequestLimiter = newRateLimiter(5, 15*time.Minute)  // per client address: reset requests
	recoveryAccountLimiter = newRateLimiter(3, time.Hour)       // per account: reset mails
	recoveryLinkLimiter    = newRateLimiter(30, 15*time.Minute) // per client address: link reads and uses
	verificationLimiter    = newRateLimiter(3, time.Hour)       // per account: confirmation mails
	invitationMailLimiter  = newRateLimiter(10, time.Hour)      // per invited account: invitation mails
)

func resetAccountRecoveryLimiters() {
	recoveryRequestLimiter = newRateLimiter(5, 15*time.Minute)
	recoveryAccountLimiter = newRateLimiter(3, time.Hour)
	recoveryLinkLimiter = newRateLimiter(30, 15*time.Minute)
	verificationLimiter = newRateLimiter(3, time.Hour)
	invitationMailLimiter = newRateLimiter(10, time.Hour)
}

// accountRecoveryPublicPaths: exact paths, each with its one method.
var accountRecoveryPublicPaths = map[string]string{
	recoveryMailStatusPath:   http.MethodGet,
	recoveryRequestPath:      http.MethodPost,
	recoveryLinkPath:         http.MethodPost,
	recoveryResetPath:        http.MethodPost,
	recoveryAcceptInvitePath: http.MethodPost,
	recoveryVerifyEmailPath:  http.MethodPost,
}

func isAccountRecoveryPublicPath(r *http.Request) bool {
	_, ok := accountRecoveryPublicPaths[r.URL.Path]
	return ok
}

func (p *Panel) registerAccountRecoveryRoutes(mux *http.ServeMux) {
	mux.HandleFunc(recoveryMailStatusPath, p.handleAccountMailStatus)
	mux.HandleFunc(recoveryRequestPath, p.handleRecoveryRequest)
	mux.HandleFunc(recoveryLinkPath, p.handleRecoveryLink)
	mux.HandleFunc(recoveryResetPath, p.handleRecoveryReset)
	mux.HandleFunc(recoveryAcceptInvitePath, p.handleRecoveryAcceptInvitation)
	mux.HandleFunc(recoveryVerifyEmailPath, p.handleRecoveryVerifyEmail)
	mux.HandleFunc(ownEmailVerificationPath, p.handleOwnEmailVerification)
	mux.HandleFunc(invitationsPath, p.handleInvitations)
	mux.HandleFunc(invitationsPath+"/", p.handleInvitationByID)
}

// decodeAccountRecoveryBody reads exactly one JSON object with known keys.
func decodeAccountRecoveryBody(r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, accountRecoveryBodyMaxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// writeAccountMailUnavailable: an unmet server-wide prerequisite; the reason
// (no_mail_server, no_panel_address, no_sender) selects the screen's words.
func writeAccountMailUnavailable(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: accountMailUnavailableMessage, Code: errCodeAccountMailUnavailable, Reason: reason,
	})
}

// handleAccountMailStatus tells the sign-in page, before anyone types, whether
// a reset by email is possible on this server. It names no address.
func (p *Panel) handleAccountMailStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !recoveryLinkLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.accountMailSetup(r.Context()).accountMailStatus)
}

// handleRecoveryRequest: "Forgot password". The answer is the same whatever
// the name; only a server-wide fact (no mail system) or the client address's
// own limit answers differently.
func (p *Panel) handleRecoveryRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !recoveryRequestLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
		Lang       string `json:"lang"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || len(identifier) > 254 {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	setup := p.accountMailSetup(r.Context())
	if !setup.Available {
		writeAccountMailUnavailable(w, setup.Reason)
		return
	}
	target, ok, err := p.recoveryCandidate(r.Context(), identifier)
	if err != nil {
		// The same answer as for any name: a reader of the answer learns
		// nothing, and the failure is in the server's log.
		log.Printf("[account-recovery] candidate lookup failed: %v", err)
	}
	if ok && recoveryAccountLimiter.allow(target.key()) {
		token, _, _, issueErr := issueAccountLink(r.Context(), p.db.GetDB(), accountLinkPasswordReset,
			target.id, target.email, target.authEpoch, accountLinkDeliveryEmail, 0, accountLinkResetTTL)
		if issueErr != nil {
			log.Printf("[account-recovery] reset link for user %d not stored: %v", target.id, issueErr)
		} else {
			p.auditAs(r, target.id, "account.password_reset.requested", "user", target.id)
			content := accountMailContent{
				kind: accountMailReset, lang: accountMailLang(req.Lang), username: target.username,
				link: accountLinkURL(setup.baseURL, accountLinkPasswordReset, token),
			}
			background := r.Clone(context.WithoutCancel(r.Context()))
			go func() {
				if sendErr := p.sendAccountMail(background.Context(), setup, target.email, content); sendErr != nil {
					log.Printf("[account-recovery] reset mail for user %d not handed over: %v", target.id, sendErr)
					p.auditAs(background, target.id, "account.password_reset.mail_failed", "user", target.id)
				}
			}()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
}

type recoveryTarget struct {
	id        int
	username  string
	email     string
	authEpoch int64
}

func (t recoveryTarget) key() string { return "user:" + strconv.Itoa(t.id) }

// recoveryCandidate finds the one account a reset may be sent for: matched by
// user name, or by address when the identifier holds '@'; active (and, for an
// additional user, an active owner); a confirmed address; no open invitation.
func (p *Panel) recoveryCandidate(ctx context.Context, identifier string) (recoveryTarget, bool, error) {
	var target recoveryTarget
	column := "u.username = ?"
	if strings.Contains(identifier, "@") {
		column = "lower(trim(u.email)) = lower(?)"
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT u.id, u.username, u.email, u.auth_epoch
		FROM users u
		LEFT JOIN users owner ON owner.id = u.parent_id
		WHERE `+column+`
		  AND COALESCE(NULLIF(u.status, ''), 'active') = 'active'
		  AND u.email_verified_at IS NOT NULL
		  AND trim(u.email) <> ''
		  AND (COALESCE(NULLIF(u.account_type, ''), 'account') <> 'additional_user'
		       OR COALESCE(NULLIF(owner.status, ''), 'active') = 'active')
		  AND NOT EXISTS (
		      SELECT 1 FROM account_links l
		      WHERE l.user_id = u.id AND l.purpose = 'invitation'
		        AND l.consumed_at IS NULL AND l.revoked_at IS NULL)`, identifier)
	if err != nil {
		return target, false, err
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		if err := rows.Scan(&target.id, &target.username, &target.email, &target.authEpoch); err != nil {
			return recoveryTarget{}, false, err
		}
		found++
	}
	if err := rows.Err(); err != nil {
		return recoveryTarget{}, false, err
	}
	if found != 1 {
		return recoveryTarget{}, false, nil
	}
	if _, err := mutationpayload.CanonicalContactEmail(target.email); err != nil {
		return recoveryTarget{}, false, nil
	}
	return target, true, nil
}

// handleRecoveryLink tells the page whether a link can still be used and for
// which account, before the person types a password.
func (p *Panel) handleRecoveryLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !recoveryLinkLimiter.allow(clientIP(r)) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAccountRecoveryTooMany, accountRecoveryTooManyMessage, "")
		return
	}
	var req struct {
		Token   string `json:"token"`
		Purpose string `json:"purpose"`
	}
	if !decodeAccountRecoveryBody(r, &req) {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	if req.Purpose != accountLinkPasswordReset && req.Purpose != accountLinkInvitation {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
		return
	}
	row, err := lookupAccountLink(r.Context(), p.db.GetDB(), req.Purpose, req.Token)
	if err == nil {
		err = p.accountLinkOwnerUsable(r.Context(), p.db.GetDB(), row)
	}
	if err != nil {
		writeAccountLinkError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"username": row.username, "purpose": row.purpose})
}

func writeAccountLinkError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errAccountLinkInvalid):
		writeCodedError(w, http.StatusGone, errCodeAccountLinkInvalid, accountLinkInvalidMessage, "")
	case errors.Is(err, errAccountLinkConflict):
		writeCodedError(w, http.StatusConflict, errCodeAccountRecoveryConflict, accountRecoveryConflictMessage, "")
	default:
		writeServerError(w, err)
	}
}

var errAccountLinkConflict = errors.New("account changed while the link was used")

// accountLinkOwnerUsable: an additional user's link needs an active owner.
func (p *Panel) accountLinkOwnerUsable(ctx context.Context, q accountLinkQueryer, row accountLinkRow) error {
	if row.accountType != "additional_user" {
		return nil
	}
	if !row.parentID.Valid {
		return errAccountLinkInvalid
	}
	var status string
	err := q.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(status, ''), 'active') FROM users WHERE id = ?`, row.parentID.Int64).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "active") {
		return errAccountLinkInvalid
	}
	return err
}

func (p *Panel) handleRecoveryReset(w http.ResponseWriter, r *http.Request) {
	p.handleSetPasswordByLink(w, r, accountLinkPasswordReset)
}

func (p *Panel) handleRecoveryAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	p.handleSetPasswordByLink(w, r, accountLinkInvitation)
}

// handleSetPasswordByLink sets the password a reset or invitation link allows.
func (p *Panel) handleSetPasswordByLink(w http.ResponseWriter, r *http.Request, purpose string) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !recoveryLinkLimiter.allow(clientIP(r)) {
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
	if len(req.NewPassword) < minPasswordLen {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountPasswordShort, accountPasswordShortMessage, "")
		return
	}
	if len(req.NewPassword) > accountPasswordMaxBytes {
		writeCodedError(w, http.StatusBadRequest, errCodeAccountPasswordLong, accountPasswordLongMessage, "")
		return
	}
	// A malformed token is refused before the (slow) password hash.
	if !wellFormedAccountLinkToken(req.Token) {
		writeAccountLinkError(w, errAccountLinkInvalid)
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeServerError(w, err)
		return
	}
	userID, err := p.setPasswordByLink(r.Context(), purpose, req.Token, hash)
	if err != nil {
		writeAccountLinkError(w, err)
		return
	}
	revokePendingLogins(userID)
	action := "account.password_reset.completed"
	if purpose == accountLinkInvitation {
		action = "account.invitation.accepted"
	}
	p.auditAs(r, userID, action, "user", userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// setPasswordByLink is one transaction: the link is found and consumed, the
// password is written with a new credential epoch while the epoch and the
// address are still the ones the link was made for, every session of the
// account is removed and the account's other open reset and invitation links
// are revoked. A link sent by email also confirms the address.
func (p *Panel) setPasswordByLink(ctx context.Context, purpose, token, hash string) (int, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := lookupAccountLink(ctx, tx, purpose, token)
	if err != nil {
		return 0, err
	}
	if err := p.accountLinkOwnerUsable(ctx, tx, row); err != nil {
		return 0, err
	}
	consumed, err := consumeAccountLink(ctx, tx, row.id)
	if err != nil {
		return 0, err
	}
	if !consumed {
		return 0, errAccountLinkInvalid
	}
	now := accountLinkTime(accountLinkNow())
	verifiedAt := any(nil)
	if row.delivery == accountLinkDeliveryEmail {
		verifiedAt = now
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, auth_epoch = auth_epoch + 1,
		    email_verified_at = COALESCE(email_verified_at, ?), updated_at = datetime('now')
		WHERE id = ? AND auth_epoch = ? AND email = ? AND COALESCE(NULLIF(status, ''), 'active') = 'active'`,
		hash, verifiedAt, row.userID, row.userEpoch, row.userEmail)
	if err != nil {
		return 0, err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return 0, err
	} else if changed != 1 {
		return 0, errAccountLinkConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, row.userID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE account_links SET revoked_at = ?
		WHERE user_id = ? AND purpose IN ('password_reset', 'invitation')
		  AND consumed_at IS NULL AND revoked_at IS NULL`, now, row.userID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return row.userID, nil
}

// handleRecoveryVerifyEmail confirms the address a confirmation link was sent
// to, while it is still the account's address.
func (p *Panel) handleRecoveryVerifyEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !recoveryLinkLimiter.allow(clientIP(r)) {
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
	userID, err := p.verifyEmailByLink(r.Context(), req.Token)
	if err != nil {
		writeAccountLinkError(w, err)
		return
	}
	p.auditAs(r, userID, "account.email.verified", "user", userID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (p *Panel) verifyEmailByLink(ctx context.Context, token string) (int, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := lookupAccountLink(ctx, tx, accountLinkEmailVerify, token)
	if err != nil {
		return 0, err
	}
	consumed, err := consumeAccountLink(ctx, tx, row.id)
	if err != nil {
		return 0, err
	}
	if !consumed {
		return 0, errAccountLinkInvalid
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users SET email_verified_at = ?, updated_at = datetime('now')
		WHERE id = ? AND email = ?`, accountLinkTime(accountLinkNow()), row.userID, row.userEmail)
	if err != nil {
		return 0, err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return 0, err
	} else if changed != 1 {
		return 0, errAccountLinkConflict
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return row.userID, nil
}

// ownEmailVerificationState is what the own-account card shows.
type ownEmailVerificationState struct {
	EmailSet      bool              `json:"email_set"`
	EmailVerified bool              `json:"email_verified"`
	Mail          accountMailStatus `json:"mail"`
}

// handleOwnEmailVerification: GET reads the own address's state; POST sends
// a confirmation link to it.
func (p *Panel) handleOwnEmailVerification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	var email string
	var verified sql.NullString
	var username string
	var epoch int64
	if err := p.db.GetDB().QueryRowContext(r.Context(),
		`SELECT username, email, email_verified_at, auth_epoch FROM users WHERE id = ?`, caller.ID).
		Scan(&username, &email, &verified, &epoch); err != nil {
		writeServerError(w, err)
		return
	}
	email = strings.TrimSpace(email)
	setup := p.accountMailSetup(r.Context())
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ownEmailVerificationState{
			EmailSet: email != "", EmailVerified: verified.Valid, Mail: setup.accountMailStatus,
		})
	case http.MethodPost:
		var req struct {
			Lang string `json:"lang"`
		}
		if !decodeAccountRecoveryBody(r, &req) {
			writeCodedError(w, http.StatusBadRequest, errCodeAccountRecoveryInvalid, accountRecoveryInvalidMessage, "")
			return
		}
		if _, err := mutationpayload.CanonicalContactEmail(email); err != nil {
			writeCodedError(w, http.StatusConflict, errCodeAccountEmailMissing, accountEmailMissingMessage, ownEmailAccountAction)
			return
		}
		if verified.Valid {
			writeCodedError(w, http.StatusConflict, errCodeAccountEmailVerified, accountEmailVerifiedMessage, "")
			return
		}
		if !setup.Available {
			writeAccountMailUnavailable(w, setup.Reason)
			return
		}
		if !verificationLimiter.allow("user:" + strconv.Itoa(caller.ID)) {
			writeCodedError(w, http.StatusTooManyRequests, errCodeAccountVerifyTooMany, accountVerifyTooManyMessage, "")
			return
		}
		if err := p.sendEmailVerification(r.Context(), setup, caller.ID, username, email, epoch, req.Lang); err != nil {
			if errors.Is(err, errAccountMailHandover) {
				writeCodedError(w, http.StatusBadGateway, errCodeAccountMailSendFailed, accountMailSendFailedMessage, "")
				return
			}
			writeServerError(w, err)
			return
		}
		p.audit(r, "account.email.verification_sent", "user", caller.ID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"delivery": "sent"})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

var errAccountMailHandover = errors.New("the mail system did not accept the message")

// sendEmailVerification makes a confirmation link and hands it over; a link
// whose message was refused is revoked at once.
func (p *Panel) sendEmailVerification(ctx context.Context, setup accountMailSetup, userID int,
	username, email string, epoch int64, lang string) error {
	token, linkID, _, err := issueAccountLink(ctx, p.db.GetDB(), accountLinkEmailVerify, userID, email, epoch,
		accountLinkDeliveryEmail, userID, accountLinkVerifyTTL)
	if err != nil {
		return err
	}
	content := accountMailContent{
		kind: accountMailVerify, lang: accountMailLang(lang), username: username,
		link: accountLinkURL(setup.baseURL, accountLinkEmailVerify, token),
	}
	if err := p.sendAccountMail(ctx, setup, email, content); err != nil {
		log.Printf("[account-recovery] confirmation mail for user %d not handed over: %v", userID, err)
		_, _ = p.db.GetDB().ExecContext(context.WithoutCancel(ctx),
			`UPDATE account_links SET revoked_at = ? WHERE id = ? AND consumed_at IS NULL`,
			accountLinkTime(accountLinkNow()), linkID)
		return errAccountMailHandover
	}
	return nil
}

// afterOwnEmailChanged runs after the own-email card changed the address: a
// confirmation link goes to the new address and a notice (without the new
// address) to the old one. Best effort, after the answer; a server without a
// mail system sends nothing and the card says the address is unconfirmed.
func (p *Panel) afterOwnEmailChanged(r *http.Request, userID int, oldEmail, lang string) {
	setup := p.accountMailSetup(r.Context())
	if !setup.Available {
		return
	}
	background := r.Clone(context.WithoutCancel(r.Context()))
	go func() {
		ctx := background.Context()
		var username, email string
		var epoch int64
		if err := p.db.GetDB().QueryRowContext(ctx,
			`SELECT username, email, auth_epoch FROM users WHERE id = ?`, userID).Scan(&username, &email, &epoch); err != nil {
			return
		}
		if _, err := mutationpayload.CanonicalContactEmail(email); err == nil &&
			verificationLimiter.allow("user:"+strconv.Itoa(userID)) {
			if err := p.sendEmailVerification(ctx, setup, userID, username, email, epoch, lang); err == nil {
				p.auditAs(background, userID, "account.email.verification_sent", "user", userID)
			}
		}
		if _, err := mutationpayload.CanonicalContactEmail(oldEmail); err == nil {
			notice := accountMailContent{kind: accountMailEmailChanged, lang: accountMailLang(lang), username: username}
			if err := p.sendAccountMail(ctx, setup, strings.TrimSpace(oldEmail), notice); err != nil {
				log.Printf("[account-recovery] address-change notice for user %d not handed over: %v", userID, err)
			}
		}
	}()
}
