package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alicelik/celikpanel/internal/auth"
)

// The screens of the remote API (alpha.128, G15): the signed-in account's own
// tokens on My account, and the administrator's server switch in Settings.
// Every route here is for a session only: a token never reaches them (they
// are not in apiTokenRoutes, and each handler refuses a token caller too).
//
//	GET    /api/v1/account/api-tokens       the account's tokens and whether it may make one
//	POST   /api/v1/account/api-tokens       make a token; the answer carries it once
//	DELETE /api/v1/account/api-tokens/{id}  remove a token (it never works again)
//	GET    /api/v1/panel/remote-api         (administrator) is the remote API on?
//	PUT    /api/v1/panel/remote-api         (administrator) turn it on or off
//
// Uzak API ekranları: hesabın kendi jetonları ve yöneticinin sunucu anahtarı.
// Buradaki rotalara yalnız oturum ulaşır; jeton asla.

const (
	apiTokensPath        = "/api/v1/account/api-tokens"
	remoteAPISettingPath = "/api/v1/panel/remote-api"

	apiTokenMaxPerAccount = 20
	apiTokenNameMaxRunes  = 64
	apiTokenBodyMaxBytes  = 4 << 10

	errCodeAPITokenInvalidRequest = "API_TOKEN_REQUEST_INVALID"
	errCodeAPITokenLimit          = "API_TOKEN_LIMIT"
	errCodeAPITokenImpersonation  = "API_TOKEN_IMPERSONATION"
	errCodeAPITokenNotFound       = "API_TOKEN_NOT_FOUND"
	errCodeAPITokenReauth         = "API_TOKEN_REAUTH"
	errCodeAPITokenReauthLimited  = "API_TOKEN_REAUTH_LIMITED"
)

// apiTokenLifetimes are the expiries a token can be made with, in days. A
// token always ends; the longest is a year.
var apiTokenLifetimes = map[int]bool{7: true, 30: true, 90: true, 365: true}

// requestIdentityRouteAPITokenCreate: the answer carries the token once, so
// only its status is kept; a replay is told the result is not kept and never
// makes a second token (D-029).
var requestIdentityRouteAPITokenCreate = requestIdentityRoute{
	pattern: apiTokensPath, timeout: 30 * time.Second, secret: true, digest: requestIdentityAPITokenCreateDigest,
}

// requestIdentityAPITokenCreateDigest: what of a create body identifies the
// request; the password and the two-factor code are left out.
func requestIdentityAPITokenCreateDigest(body []byte) []byte {
	var req apiTokenCreateRequest
	if json.Unmarshal(body, &req) != nil {
		return []byte("unreadable")
	}
	canonical, err := json.Marshal(struct {
		Name        string `json:"name"`
		Scope       string `json:"scope"`
		ExpiresDays int    `json:"expires_days"`
	}{req.Name, req.Scope, req.ExpiresDays})
	if err != nil {
		return []byte("unreadable")
	}
	return canonical
}

// apiTokenView is one token as a screen shows it: never the secret.
type apiTokenView struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Scope      string `json:"scope"`
	State      string `json:"state"` // active, expired, ended
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  int64  `json:"expires_at"`
	LastUsedAt *int64 `json:"last_used_at,omitempty"`
	LastUsedIP string `json:"last_used_ip,omitempty"`
}

type apiTokensAnswer struct {
	ServerEnabled bool   `json:"server_enabled"`
	CanCreate     bool   `json:"can_create"`
	RefusedBy     string `json:"refused_by,omitempty"`
	// GrantorKind (administrator, provider) and GrantorName: who can open
	// the remote API for this account when it is refused.
	GrantorKind string `json:"grantor_kind,omitempty"`
	GrantorName string `json:"grantor_name,omitempty"`
	// TwoFactor: making a token asks for the two-factor code as well.
	TwoFactor bool  `json:"two_factor"`
	MaxTokens int   `json:"max_tokens"`
	Lifetimes []int `json:"lifetimes"`
	// Tokens: the account's tokens that are not removed, newest first.
	Tokens []apiTokenView `json:"tokens"`
}

// sessionAccountCaller: the signed-in account (not a token, not a team
// member), or a written refusal.
func sessionAccountCaller(w http.ResponseWriter, r *http.Request) (*Caller, bool) {
	c := currentCaller(r)
	if c == nil {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return nil, false
	}
	if c.APIToken != nil {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenRouteRefused, apiTokenRouteRefusedMessage, "")
		return nil, false
	}
	if callerAccountRole(c) == "" {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenTeamMember, apiTokenTeamMemberMessage, "")
		return nil, false
	}
	return c, true
}

func (p *Panel) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	c, ok := sessionAccountCaller(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		p.listAPITokens(w, r, c)
	case http.MethodPost:
		p.createAPIToken(w, r, c)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// apiTokenCreateRefusal: why this account may not make a token now ("" when
// it may), and who can change that.
func (p *Panel) apiTokenCreateRefusal(ctx context.Context, c *Caller) (enabled bool, refusedBy string, grantor apiGrantor, err error) {
	enabled, err = p.remoteAPIEnabled(ctx)
	if err != nil {
		return false, "", apiGrantor{}, err
	}
	if !enabled {
		return false, "server", apiGrantorAdministrator, nil
	}
	layer, grantor, err := p.remoteAPIRefusal(ctx, c)
	if err != nil {
		return true, "", apiGrantor{}, err
	}
	return true, layer, grantor, nil
}

func (p *Panel) listAPITokens(w http.ResponseWriter, r *http.Request, c *Caller) {
	ctx := r.Context()
	enabled, refusedBy, grantor, err := p.apiTokenCreateRefusal(ctx, c)
	if err != nil {
		writeServerError(w, err)
		return
	}
	tokens, err := p.accountAPITokens(ctx, c.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if refusedBy == "" && c.ImpersonatorID > 0 {
		refusedBy = "impersonation"
	}
	var totp int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(totp_enabled, 0) FROM users WHERE id = ?`, c.ID).Scan(&totp); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(apiTokensAnswer{
		ServerEnabled: enabled, CanCreate: refusedBy == "", RefusedBy: refusedBy,
		GrantorKind: grantor.Kind, GrantorName: grantor.Name, TwoFactor: totp == 1,
		MaxTokens: apiTokenMaxPerAccount, Lifetimes: []int{7, 30, 90, 365}, Tokens: tokens,
	})
}

// accountAPITokens lists the account's tokens that are not removed.
func (p *Panel) accountAPITokens(ctx context.Context, userID int) ([]apiTokenView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT t.id, t.name, t.token_id, t.scope, (t.auth_epoch = u.auth_epoch AND t.owner_email = COALESCE(u.email, '')), t.created_at, t.expires_at,
		       t.last_used_at, COALESCE(t.last_used_ip, '')
		FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.user_id = ? AND t.revoked_at IS NULL
		ORDER BY t.created_at DESC, t.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := apiTokenNow().Unix()
	out := []apiTokenView{}
	for rows.Next() {
		var v apiTokenView
		var tokenID string
		var current bool
		var lastUsed sql.NullInt64
		if err := rows.Scan(&v.ID, &v.Name, &tokenID, &v.Scope, &current, &v.CreatedAt, &v.ExpiresAt, &lastUsed, &v.LastUsedIP); err != nil {
			return nil, err
		}
		v.Prefix = apiTokenPrefix + tokenID
		if v.Scope != apiTokenScopeManage {
			v.Scope = apiTokenScopeRead
		}
		switch {
		case !current:
			v.State = "ended"
		case now >= v.ExpiresAt:
			v.State = "expired"
		default:
			v.State = "active"
		}
		if lastUsed.Valid {
			used := lastUsed.Int64
			v.LastUsedAt = &used
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type apiTokenCreateRequest struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	ExpiresDays int    `json:"expires_days"`
	// Password and Code: the account's password, and its two-factor code
	// when two-factor sign-in is on (a fresh proof, as for the e-mail and
	// password changes).
	Password string `json:"password"`
	Code     string `json:"code"`
}

// validAPITokenName: 1 to 64 printable characters, no control character.
func validAPITokenName(name string) bool {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > apiTokenNameMaxRunes {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func (p *Panel) createAPIToken(w http.ResponseWriter, r *http.Request, c *Caller) {
	ctx := r.Context()
	// A "Log in as" session acts for the account but is not its owner: it
	// does not leave a lasting credential behind.
	if c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenImpersonation,
			"An API token cannot be made while signed in with \"Log in as\", so none was made. The account's owner makes tokens from their own sign-in.", "")
		return
	}
	var req apiTokenCreateRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, apiTokenBodyMaxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeAPITokenInvalidRequest, "The request could not be read, so no token was made.", "")
		return
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		writeCodedError(w, http.StatusBadRequest, errCodeAPITokenInvalidRequest, "The request could not be read, so no token was made.", "")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !validAPITokenName(req.Name) || (req.Scope != apiTokenScopeRead && req.Scope != apiTokenScopeManage) || !apiTokenLifetimes[req.ExpiresDays] {
		writeCodedError(w, http.StatusBadRequest, errCodeAPITokenInvalidRequest,
			"A token needs a name (1 to 64 characters), a scope (read or manage) and an expiry of 7, 30, 90 or 365 days; no token was made.", "")
		return
	}
	enabled, refusedBy, grantor, err := p.apiTokenCreateRefusal(ctx, c)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !enabled {
		writeCodedError(w, http.StatusForbidden, errCodeAPIDisabled, apiDisabledMessage, "")
		return
	}
	if refusedBy != "" {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenNotPermitted, apiTokenNotPermittedMessage(grantor), "")
		return
	}
	// Step-up: a lasting credential is made only after the person proves
	// the password again (and the two-factor code when it is on). Every
	// attempt counts in the same limiter as the password and e-mail changes.
	if req.Password == "" {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenReauth, apiTokenReauthMessage, "")
		return
	}
	if !p.allowSensitiveAuthAttempt(r, c.ID, reauthAttemptAction) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeAPITokenReauthLimited,
			"Too many password attempts; wait a few minutes and try again. No token was made.", "")
		return
	}
	state, err := p.userAuthState(ctx, c.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	passwordOK, err := auth.VerifyPassword(req.Password, state.passwordHash)
	if err != nil || !passwordOK || (state.totpEnabled && !auth.ValidateTOTP(state.totpSecret, strings.TrimSpace(req.Code))) {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenReauth, apiTokenReauthMessage, "")
		return
	}

	tokenID, secret, full, err := newAPIToken()
	if err != nil {
		writeServerError(w, err)
		return
	}
	now := apiTokenNow().Unix()
	expires := now + int64(req.ExpiresDays)*24*60*60
	db := p.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM api_tokens WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?`,
		c.ID, now).Scan(&active); err != nil {
		writeServerError(w, err)
		return
	}
	if active >= apiTokenMaxPerAccount {
		writeCodedError(w, http.StatusConflict, errCodeAPITokenLimit,
			"This account already has 20 working API tokens, so no new one was made. Remove one you no longer use, then make the new one.", "")
		return
	}
	// The token is bound to the owner's credential epoch and e-mail address
	// read in the same transaction: a password, e-mail or two-factor change
	// after this ends it.
	result, err := tx.ExecContext(ctx, `
		INSERT INTO api_tokens (user_id, name, token_id, secret_sha256, scope, auth_epoch, owner_email, created_at, expires_at)
		SELECT id, ?, ?, ?, ?, auth_epoch, COALESCE(email, ''), ?, ?
		FROM users WHERE id = ? AND auth_epoch = ? AND COALESCE(NULLIF(status, ''), 'active') = 'active'`,
		req.Name, tokenID, hashAPITokenSecret(secret), req.Scope, now, expires, c.ID, state.authEpoch)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		writeServerError(w, errors.New("the account changed while the token was being made"))
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "api_token.create", "api_token", int(id), map[string]any{
		"token": apiTokenPrefix + tokenID, "name": req.Name, "scope": req.Scope, "expires_days": req.ExpiresDays,
	})
	doNotRetainRequestAnswer(ctx)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token": full,
		"item": apiTokenView{
			ID: id, Name: req.Name, Prefix: apiTokenPrefix + tokenID, Scope: req.Scope, State: "active",
			CreatedAt: now, ExpiresAt: expires,
		},
	})
}

// apiTokenReauthMessage: the step-up refusal (D-024: what, nothing changed,
// what next).
const apiTokenReauthMessage = "The password (or the two-factor code) is not correct, so no token was made. " +
	"Enter your current password, and the code from your authenticator app if two-factor sign-in is on, then try again."

// handleAPITokenItem removes one of the account's tokens.
func (p *Panel) handleAPITokenItem(w http.ResponseWriter, r *http.Request) {
	c, ok := sessionAccountCaller(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	segments, ok := strictRouteSegments(r, apiTokensPath+"/")
	if !ok || len(segments) != 1 {
		writeCodedError(w, http.StatusNotFound, errCodeAPITokenNotFound, "This token was not found on your account; nothing was changed.", "")
		return
	}
	id, ok := strictPositiveID(segments[0])
	if !ok {
		writeCodedError(w, http.StatusNotFound, errCodeAPITokenNotFound, "This token was not found on your account; nothing was changed.", "")
		return
	}
	ctx := r.Context()
	var tokenID string
	var revoked sql.NullInt64
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT token_id, revoked_at FROM api_tokens WHERE id = ? AND user_id = ?`, id, c.ID).Scan(&tokenID, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusNotFound, errCodeAPITokenNotFound, "This token was not found on your account; nothing was changed.", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !revoked.Valid {
		revoker, _ := auditPerson(r)
		if _, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE api_tokens SET revoked_at = ?, revoked_by = ?
			WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, apiTokenNow().Unix(), revoker, id, c.ID); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "api_token.revoke", "api_token", id, map[string]any{"token": apiTokenPrefix + tokenID})
	}
	// Removing a removed token answers the same: it does not work.
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoteAPISetting is the administrator's server switch (the
// /api/v1/panel/ prefix is administrator-only in requireAuth).
func (p *Panel) handleRemoteAPISetting(w http.ResponseWriter, r *http.Request) {
	c := currentCaller(r)
	if c == nil || c.APIToken != nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, apiTokenBodyMaxBytes))
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&body); err != nil || body.Enabled == nil || !errors.Is(decoder.Decode(&trailing), io.EOF) {
			writeClientError(w, http.StatusBadRequest, "send {\"enabled\": true} or {\"enabled\": false}; nothing was changed")
			return
		}
		value, action := "0", "remote_api.disable"
		if *body.Enabled {
			value, action = "1", "remote_api.enable"
		}
		if err := p.setSetting(ctx, settingRemoteAPIEnabled, value); err != nil {
			writeServerError(w, err)
			return
		}
		p.audit(r, action, "panel_settings", 0)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	enabled, err := p.remoteAPIEnabled(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var working int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.revoked_at IS NULL AND t.expires_at > ? AND t.auth_epoch = u.auth_epoch AND t.owner_email = COALESCE(u.email, '')`,
		apiTokenNow().Unix()).Scan(&working); err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"enabled": enabled, "working_tokens": working})
}
