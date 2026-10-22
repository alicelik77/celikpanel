package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// The signed-in person changes their OWN email (v0.1.0-alpha.99, D-037
// amendment of 2026-10-22). Until then no screen did: an administrator's
// address was set only when the administrator was created on the server, and
// the panel certificate, which hands that address to the certificate authority
// as its contact, could only say "there is no screen for this yet".
//
// The rules:
//   - Re-authentication: the current password is part of the request, as for
//     the password change, and its wrong guesses count against the same limiter
//     (allowSensitiveAuthAttempt) under their own action name.
//   - One identity per action (D-029): the route is guarded by
//     requestIdentityGuard; the same identity is answered from the first
//     arrival and never changes the address twice. The stored digest of the
//     request leaves the password out (requestIdentityEmailDigest).
//   - One validator: mutationpayload.CanonicalContactEmail, the one the panel
//     certificate request uses, so an accepted address is always one the
//     certificate request accepts.
//   - users.email is UNIQUE in the schema (001_full_schema.sql); an address
//     another account uses is refused with its own code, compared without
//     regard to letter case, and the constraint is the last word.
//   - The write is a compare-and-set on the address and password hash that
//     were verified, so a change made elsewhere in between is never
//     overwritten; it is reported instead.
//   - Sessions stay: the address is not a sign-in credential (sign-in uses the
//     user name). Nothing about a certificate is requested or renewed; the
//     panel certificate request reads the address when it is made.
//   - Audit: account.email.changed on the user, without the address (the
//     trail names accounts, never addresses).
//
// Oturum açan kişi KENDİ e-postasını değiştirir. Mevcut parola istenir (parola
// değişimiyle aynı sınırlayıcı), her eylemin tek kimliği vardır (D-029), adres
// panel sertifikasının doğrulayıcısıyla denetlenir, başka hesabın adresi
// reddedilir, yazım doğrulanan duruma koşullu yapılır, oturumlar sürer,
// sertifika için hiçbir şey kendiliğinden yapılmaz, denetim kaydı adres
// içermez.

const (
	ownEmailPath = "/api/v1/auth/email"

	// ownEmailAccountAction is where the panel certificate's and the setup
	// plan's contact refusal sends the person: the card on My account.
	ownEmailAccountAction = "/account#email"

	errCodeOwnEmailRequestInvalid   = "account_email_request_invalid"
	errCodeOwnEmailInvalid          = "account_email_invalid"
	errCodeOwnEmailUnchanged        = "account_email_unchanged"
	errCodeOwnEmailTaken            = "account_email_taken"
	errCodeOwnEmailWrongPassword    = "account_email_wrong_password"
	errCodeOwnEmailTooManyAttempts  = "account_email_too_many_attempts"
	errCodeOwnEmailChangedElsewhere = "account_email_changed_elsewhere"

	ownEmailRequestMaxBytes = 16 << 10
)

// English originals of the sentences a screen shows from web/src/i18n
// (err.<code>); D-024 order: what happened, what was not changed, what to do.
const (
	ownEmailRequestInvalidMessage   = "The request could not be read, so nothing was changed. Reload the page and try again."
	ownEmailInvalidMessage          = "This is not an email address the panel can use, so nothing was changed. Enter one address such as name@example.com."
	ownEmailUnchangedMessage        = "This is already your email address, so nothing was changed."
	ownEmailTakenMessage            = "Another account already uses this email address, so nothing was changed. Enter a different address."
	ownEmailWrongPasswordMessage    = "Your current password is not correct, so nothing was changed. Enter the password you sign in with."
	ownEmailTooManyAttemptsMessage  = "Too many attempts, so nothing was changed. Wait a few minutes, then try again."
	ownEmailChangedElsewhereMessage = "Your account was changed somewhere else while this was being saved, so nothing was changed. Reload the page and try again."
)

type ownEmailRequest struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
}

// handleChangeOwnEmail lets the signed-in user change their own email after
// proving their current password.
// handleChangeOwnEmail, oturumdaki kullanıcının mevcut parolasını
// kanıtladıktan sonra kendi e-postasını değiştirmesini sağlar.
func (p *Panel) handleChangeOwnEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// The guard is always in front of this route; a request that reaches the
	// handler without an identity did not pass through it and is not run.
	if requestIdentityFromContext(r.Context()) == "" {
		writeRequestIdentityRefusal(w, http.StatusPreconditionRequired, errCodeRequestIDRequired,
			requestIDRequiredMessage, "", "")
		return
	}
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}

	var req ownEmailRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, ownEmailRequestMaxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeOwnEmailRequestInvalid, ownEmailRequestInvalidMessage, "")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeCodedError(w, http.StatusBadRequest, errCodeOwnEmailRequestInvalid, ownEmailRequestInvalidMessage, "")
		return
	}
	address, err := mutationpayload.CanonicalContactEmail(req.Email)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeOwnEmailInvalid, ownEmailInvalidMessage, "")
		return
	}
	if req.CurrentPassword == "" {
		writeCodedError(w, http.StatusForbidden, errCodeOwnEmailWrongPassword, ownEmailWrongPasswordMessage, "")
		return
	}

	// Every attempt that carries a password counts, as on the password change.
	if !p.allowSensitiveAuthAttempt(r, caller.ID, "email-change") {
		writeCodedError(w, http.StatusTooManyRequests, errCodeOwnEmailTooManyAttempts, ownEmailTooManyAttemptsMessage, "")
		return
	}
	user, err := p.users.GetByID(r.Context(), caller.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	ok, err := auth.VerifyPassword(req.CurrentPassword, user.PasswordHash)
	if err != nil || !ok {
		writeCodedError(w, http.StatusForbidden, errCodeOwnEmailWrongPassword, ownEmailWrongPasswordMessage, "")
		return
	}
	// The address and whether another account holds it are told only to a
	// person who proved the password.
	current := strings.TrimSpace(user.Email)
	if address == current {
		writeCodedError(w, http.StatusConflict, errCodeOwnEmailUnchanged, ownEmailUnchangedMessage, "")
		return
	}
	db := p.db.GetDB()
	var holders int
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM users WHERE id <> ? AND lower(trim(email)) = lower(?)`,
		user.ID, address).Scan(&holders); err != nil {
		writeServerError(w, err)
		return
	}
	if holders > 0 {
		writeCodedError(w, http.StatusConflict, errCodeOwnEmailTaken, ownEmailTakenMessage, "")
		return
	}

	result, err := db.ExecContext(r.Context(), `
		UPDATE users SET email = ?, updated_at = datetime('now')
		WHERE id = ? AND email = ? AND password_hash = ?`,
		address, user.ID, user.Email, user.PasswordHash)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			writeCodedError(w, http.StatusConflict, errCodeOwnEmailTaken, ownEmailTakenMessage, "")
			return
		}
		writeServerError(w, err)
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if changed != 1 {
		writeCodedError(w, http.StatusConflict, errCodeOwnEmailChangedElsewhere, ownEmailChangedElsewhereMessage, "")
		return
	}
	p.audit(r, "account.email.changed", "user", user.ID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "email": address})
}

// requestIdentityEmailDigest is what the guard hashes for this route: the
// request without its password, so the stored digest of a request cannot be
// used to test password guesses. Two arrivals with the same identity and the
// same new address are the same request; the first one's answer stands.
// requestIdentityEmailDigest: korumanın bu rota için özetlediği şey; parola
// çıkarılmış istek.
func requestIdentityEmailDigest(body []byte) []byte {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return []byte("unreadable")
	}
	delete(fields, "current_password")
	canonical, err := json.Marshal(fields)
	if err != nil {
		return []byte("unreadable")
	}
	return canonical
}
