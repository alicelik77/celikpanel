package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Invitations (v0.1.0-alpha.114, P6). An administrator or reseller creates an
// account (POST /api/v1/users with "invite": true), or a customer an
// additional user (POST /api/v1/team-members with "invite": true), without a
// password. The account gets a random password nobody knows and a single-use
// link, valid for 7 days, by which the invited person chooses their own.
//
// The link goes by email when this server can send account mail. When it
// cannot (no mail system, no panel address, an address the mail system
// refused), the answer gives the link's path to the inviter alone, once, to
// copy and pass on; the page builds the full address from the address the
// inviter is using. The raw link is never stored and is never shown again:
// "Send again" makes a new one and voids the old one.
//
//	GET    /api/v1/invitations              open invitations the caller may manage
//	POST   /api/v1/invitations/{id}/resend  {delivery: "email"|"link", lang}
//	DELETE /api/v1/invitations/{id}         void the open invitation link
//
// Who manages an invitation: an administrator any non-administrator account;
// a reseller or customer the accounts whose parent they are (a reseller's
// customers, a customer's additional users). Additional users manage none.
//
// Davetler: hesap parolasız açılır; davet edilen kişi tek kullanımlık, 7 gün
// geçerli bağlantıyla kendi parolasını belirler. Sunucu posta gönderemiyorsa
// bağlantının yolu yalnız davet edene, bir kez, kopyalaması için verilir.
// Ham bağlantı saklanmaz; "Yeniden gönder" yenisini yapar, eskisini geçersiz kılar.

const (
	invitationsPath = "/api/v1/invitations"

	errCodeInvitationNotFound   = "account_invitation_not_found"
	errCodeInvitationSuperseded = "account_invitation_superseded"
	errCodeInvitationRequest    = "account_invitation_request_invalid"
	errCodeInvitationPassword   = "account_invitation_with_password"

	invitationNotFoundMessage   = "This invitation was not found, so nothing was changed. Reload the list."
	invitationSupersededMessage = "This account already has a password, so no invitation was sent. The person signs in with it, or uses Forgot password."
	invitationRequestMessage    = "The request could not be read, so nothing was changed. Reload the page and try again."
	invitationPasswordMessage   = "An invitation lets the person choose their own password, so a password cannot be given with it. Leave the password empty."
)

// invitationOutcome is what the inviter's screen shows after a send.
//   - delivery "sent": handed to this server's mail system (not "delivered").
//   - delivery "link": not sent by mail; link_path is the one copy, and
//     reason says why (a mail status reason, "send_failed", "invalid_address",
//     "too_many" or "requested").
//   - delivery "none": no invitation could be prepared; reason "error".
type invitationOutcome struct {
	ID        int64  `json:"id,omitempty"`
	Delivery  string `json:"delivery"`
	Reason    string `json:"reason,omitempty"`
	LinkPath  string `json:"link_path,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// unusableInvitationPassword is the password an invited account is created
// with: 32 random bytes nobody sees, so the account cannot be signed in to
// before the invitation is accepted.
func unusableInvitationPassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "invite-" + base64.RawURLEncoding.EncodeToString(raw), nil
}

// issueInvitation makes the invitation link for an account just created (or
// for a resend) and sends it when it may. wantEmail false copies the link.
func (p *Panel) issueInvitation(r *http.Request, inviterID, userID int, lang string, wantEmail bool) invitationOutcome {
	ctx := r.Context()
	var username, email, inviter string
	var epoch int64
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT u.username, u.email, u.auth_epoch, COALESCE(i.username, '')
		FROM users u LEFT JOIN users i ON i.id = ?
		WHERE u.id = ?`, inviterID, userID).Scan(&username, &email, &epoch, &inviter)
	if err != nil {
		log.Printf("[invitation] account %d not read: %v", userID, err)
		return invitationOutcome{Delivery: "none", Reason: "error"}
	}
	email = strings.TrimSpace(email)
	setup := p.accountMailSetup(ctx)
	reason := ""
	switch {
	case !wantEmail:
		reason = "requested"
	case !setup.Available:
		reason = setup.Reason
	default:
		if _, err := mutationpayload.CanonicalContactEmail(email); err != nil {
			reason = "invalid_address"
		} else if !invitationMailLimiter.allow("user:" + strconv.Itoa(userID)) {
			reason = "too_many"
		}
	}
	delivery := accountLinkDeliveryEmail
	if reason != "" {
		delivery = accountLinkDeliveryLink
	}
	token, linkID, expires, err := issueAccountLink(ctx, p.db.GetDB(), accountLinkInvitation, userID, email, epoch,
		delivery, inviterID, accountLinkInvitationTTL)
	if err != nil {
		log.Printf("[invitation] link for account %d not stored: %v", userID, err)
		return invitationOutcome{Delivery: "none", Reason: "error"}
	}
	outcome := invitationOutcome{ID: linkID, ExpiresAt: accountLinkTime(expires)}
	if delivery == accountLinkDeliveryEmail {
		content := accountMailContent{
			kind: accountMailInvitation, lang: accountMailLang(lang), username: username, inviter: inviter,
			link: accountLinkURL(setup.baseURL, accountLinkInvitation, token),
		}
		if err := p.sendAccountMail(ctx, setup, email, content); err == nil {
			outcome.Delivery = "sent"
			p.auditAsDetailed(r, inviterID, "account.invitation.sent", "user", userID, map[string]any{"delivery": "email"})
			return outcome
		} else {
			log.Printf("[invitation] mail for account %d not handed over: %v", userID, err)
			p.auditAs(r, inviterID, "account.invitation.mail_failed", "user", userID)
		}
		// The message was refused: the same link becomes the copy.
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE account_links SET delivery = ? WHERE id = ?`,
			accountLinkDeliveryLink, linkID); err != nil {
			log.Printf("[invitation] link %d not marked as copied: %v", linkID, err)
		}
		reason = "send_failed"
	}
	outcome.Delivery = "link"
	outcome.Reason = reason
	outcome.LinkPath = accountLinkPaths[accountLinkInvitation] + "#t=" + token
	p.auditAsDetailed(r, inviterID, "account.invitation.sent", "user", userID, map[string]any{"delivery": "link"})
	return outcome
}

// invitationPassword reads the optional invite flag of a create request and
// swaps an empty password for an unusable one. It writes the refusal itself.
func invitationPassword(w http.ResponseWriter, invite bool, password *string) bool {
	if !invite {
		return true
	}
	if *password != "" {
		writeCodedError(w, http.StatusBadRequest, errCodeInvitationPassword, invitationPasswordMessage, "")
		return false
	}
	generated, err := unusableInvitationPassword()
	if err != nil {
		writeServerError(w, err)
		return false
	}
	*password = generated
	return true
}

// canManageInvitee is the one rule for listing, resending and voiding.
func canManageInvitee(c *Caller, role string, parentID sql.NullInt64) bool {
	if c == nil || !c.validAuthorizationIdentity() || c.isAdditionalUser() || role == roleAdmin {
		return false
	}
	if c.hasAccountRole(roleAdmin) {
		return true
	}
	if !c.hasAccountRole(roleReseller) && !c.hasAccountRole(roleCustomer) {
		return false
	}
	return parentID.Valid && int(parentID.Int64) == c.ID
}

type invitationListItem struct {
	ID          int64  `json:"id"`
	UserID      int    `json:"user_id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	AccountType string `json:"account_type"`
	Role        string `json:"role"`
	// State: pending (usable), expired, or superseded (the account got a
	// password, a new address or a suspension another way).
	State     string `json:"state"`
	Delivery  string `json:"delivery"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

func (p *Panel) handleInvitations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != invitationsPath {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	c := currentCaller(r)
	if c == nil || !c.validAuthorizationIdentity() || c.isAdditionalUser() {
		writeClientError(w, http.StatusForbidden, "account management access required")
		return
	}
	rows, err := p.db.GetDB().QueryContext(r.Context(), `
		SELECT l.id, l.user_id, l.email, l.auth_epoch, l.delivery, l.created_at, l.expires_at,
		       u.username, u.email, u.auth_epoch, u.role, COALESCE(NULLIF(u.account_type, ''), 'account'),
		       COALESCE(NULLIF(u.status, ''), 'active'), u.parent_id
		FROM account_links l JOIN users u ON u.id = l.user_id
		WHERE l.purpose = 'invitation' AND l.consumed_at IS NULL AND l.revoked_at IS NULL
		ORDER BY l.created_at DESC, l.id DESC`)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	now := accountLinkTime(accountLinkNow())
	items := []invitationListItem{}
	for rows.Next() {
		var item invitationListItem
		var linkEmail, userEmail, status string
		var linkEpoch, userEpoch int64
		var parentID sql.NullInt64
		if err := rows.Scan(&item.ID, &item.UserID, &linkEmail, &linkEpoch, &item.Delivery, &item.CreatedAt,
			&item.ExpiresAt, &item.Username, &userEmail, &userEpoch, &item.Role, &item.AccountType, &status, &parentID); err != nil {
			writeServerError(w, err)
			return
		}
		if !canManageInvitee(c, item.Role, parentID) {
			continue
		}
		item.Email = userEmail
		switch {
		case linkEpoch != userEpoch || !strings.EqualFold(strings.TrimSpace(linkEmail), strings.TrimSpace(userEmail)) || status != "active":
			item.State = "superseded"
		case item.ExpiresAt <= now:
			item.State = "expired"
		default:
			item.State = "pending"
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"invitations": items})
}

// handleInvitationByID routes resend and void.
func (p *Panel) handleInvitationByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	rest := strings.TrimPrefix(r.URL.Path, invitationsPath+"/")
	idText, action, _ := strings.Cut(rest, "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 || strings.Contains(action, "/") {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	c := currentCaller(r)
	if c == nil || !c.validAuthorizationIdentity() || c.isAdditionalUser() {
		writeClientError(w, http.StatusForbidden, "account management access required")
		return
	}
	var userID int
	var linkEpoch, userEpoch int64
	var linkEmail, userEmail, role, status string
	var parentID sql.NullInt64
	err = p.db.GetDB().QueryRowContext(r.Context(), `
		SELECT l.user_id, l.auth_epoch, l.email, u.auth_epoch, u.email, u.role,
		       COALESCE(NULLIF(u.status, ''), 'active'), u.parent_id
		FROM account_links l JOIN users u ON u.id = l.user_id
		WHERE l.id = ? AND l.purpose = 'invitation' AND l.consumed_at IS NULL AND l.revoked_at IS NULL`, id).
		Scan(&userID, &linkEpoch, &linkEmail, &userEpoch, &userEmail, &role, &status, &parentID)
	// "Missing" and "not yours" are the same answer.
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !canManageInvitee(c, role, parentID)) {
		writeCodedError(w, http.StatusNotFound, errCodeInvitationNotFound, invitationNotFoundMessage, "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	switch {
	case action == "resend" && r.Method == http.MethodPost:
		var req struct {
			Delivery string `json:"delivery"`
			Lang     string `json:"lang"`
		}
		if !decodeAccountRecoveryBody(r, &req) || (req.Delivery != "email" && req.Delivery != "link") {
			writeCodedError(w, http.StatusBadRequest, errCodeInvitationRequest, invitationRequestMessage, "")
			return
		}
		// An account that got a password another way is not re-invited: that
		// would be a password reset by the inviter.
		if linkEpoch != userEpoch || status != "active" ||
			!strings.EqualFold(strings.TrimSpace(linkEmail), strings.TrimSpace(userEmail)) {
			writeCodedError(w, http.StatusConflict, errCodeInvitationSuperseded, invitationSupersededMessage, "")
			return
		}
		outcome := p.issueInvitation(r, c.ID, userID, req.Lang, req.Delivery == "email")
		if outcome.Delivery == "none" {
			writeServerError(w, errors.New("invitation not prepared"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"invitation": outcome})
	case action == "" && r.Method == http.MethodDelete:
		if _, err := p.db.GetDB().ExecContext(r.Context(), `
			UPDATE account_links SET revoked_at = ?
			WHERE user_id = ? AND purpose = 'invitation' AND consumed_at IS NULL AND revoked_at IS NULL`,
			accountLinkTime(accountLinkNow()), userID); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, "account.invitation.revoked", "user", userID)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
