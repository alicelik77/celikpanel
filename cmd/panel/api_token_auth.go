package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// Remote API tokens (alpha.128, G15; docs/REMOTE-API.md). A token lets the
// owner's own program call a short, fixed list of routes for the account that
// made it. It is checked here, in requireAuth, before public paths and before
// the session path, and it fails closed at every step:
//
//  1. A request carrying both a session cookie and a token is refused; a
//     token request never falls through to the cookie path.
//  2. The token must parse (cpk_<id>_<secret>), its id must exist and the
//     SHA-256 of its secret must match (constant time). A token that does
//     not verify is refused as invalid; after ten such failures a minute
//     from one address, that address gets 429 for the rest of the minute.
//     A token that verifies is never refused by that limit.
//  3. It must not be revoked or expired, and the owner's credential epoch
//     and e-mail address must be the ones it was made under.
//  4. The server switch (Settings, off by default) must be on; whether it
//     is on is told only to the holder of a working token.
//  5. 120 requests a minute per token.
//  6. The owner must pass the same account checks as a session (active, not
//     suspended, consistent identity); a team member never holds a token.
//  7. The route must be in apiTokenRoutes; a read token reaches only GET.
//     Everything else - token management, password, two-factor, e-mail,
//     every reveal, VPN, licence, setup, server routes - answers 403
//     API_TOKEN_ROUTE_NOT_ALLOWED. "Never able to read a secret" holds by
//     construction: no listed route answers one.
//  8. The permission remote_api (D-042 layers); an administrator always has
//     it. A customer needs it in every subscription the account holds.
//  9. The ordinary gates every session passes (suspended subscription,
//     administrator-only paths, licence), then the handler's own ownership
//     and rights checks.
//
// Every changing call is audited (the token's id, never its secret); the
// last use is written at most once a minute. CSRF checks keep applying to
// every cookie session (security.go skips the origin check only for a request
// that carries a token and no session cookie, which is authenticated here or
// refused).
//
// Uzak API jetonları: jeton, hesabın kendi programının kısa ve sabit bir rota
// listesini çağırmasını sağlar. Burada, oturumdan önce denetlenir ve her
// adımda kapalı başarısız olur. Listede olmayan her rota reddedilir.

const (
	apiTokenPrefix       = "cpk_"
	apiTokenIDLength     = 12
	apiTokenSecretBytes  = 32
	apiTokenSecretLength = 43 // base64url of 32 bytes, no padding
	apiTokenLength       = len(apiTokenPrefix) + apiTokenIDLength + 1 + apiTokenSecretLength
	apiTokenIDAlphabet   = "abcdefghijklmnopqrstuvwxyz234567"

	apiTokenScopeRead   = "read"
	apiTokenScopeManage = "manage"

	apiTokenCallsPerMinute    = 120
	apiTokenFailuresPerMinute = 10
	apiTokenLastUsedEvery     = time.Minute

	// settingRemoteAPIEnabled: the server switch; "1" is on, anything else
	// (and no row) is off.
	settingRemoteAPIEnabled = "remote_api_enabled"

	errCodeAPITokenWithSession  = "API_TOKEN_WITH_SESSION"
	errCodeAPIDisabled          = "API_DISABLED"
	errCodeAPITokenInvalid      = "API_TOKEN_INVALID"
	errCodeAPITokenRevoked      = "API_TOKEN_REVOKED"
	errCodeAPITokenExpired      = "API_TOKEN_EXPIRED"
	errCodeAPITokenEnded        = "API_TOKEN_ENDED"
	errCodeAPITokenRateLimited  = "API_TOKEN_RATE_LIMITED"
	errCodeAPITokenSignInLimit  = "API_TOKEN_SIGN_IN_LIMITED"
	errCodeAPITokenRouteRefused = "API_TOKEN_ROUTE_NOT_ALLOWED"
	errCodeAPITokenScope        = "API_TOKEN_SCOPE"
	errCodeAPITokenNotPermitted = "API_TOKEN_NOT_PERMITTED"
	errCodeAPITokenTeamMember   = "API_TOKEN_TEAM_MEMBER"
)

// apiTokenNow is the clock of expiry and last use (a test moves it).
var apiTokenNow = time.Now

// apiTokenUse is the token a request was authenticated with; it rides on the
// Caller so handlers and the audit trail can tell (Caller.APIToken).
type apiTokenUse struct {
	ID      int64
	TokenID string
	Scope   string
}

// label is how a token is named in the audit trail and on screens: its
// public prefix, never its secret.
func (u *apiTokenUse) label() string {
	if u == nil {
		return ""
	}
	return apiTokenPrefix + u.TokenID
}

// ----- the credential -----

// apiTokenCredential reports whether the request presents a CelikPanel API
// token and returns it. Only an "Authorization: Bearer cpk_..." header is
// one; any other Authorization header (a proxy's Basic sign-in, an identity
// proxy's bearer ID token, a remote DNS machine's credential) is not a token
// and is ignored here exactly as before the remote API existed. Only the
// Authorization header is read, never a query string or another header. A
// second Authorization header beside a token makes it ambiguous: presented,
// but empty (refused as invalid).
func apiTokenCredential(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	present := false
	for _, value := range values {
		if _, ok := bearerAPIToken(value); ok {
			present = true
		}
	}
	if !present {
		return "", false
	}
	if len(values) != 1 {
		return "", true
	}
	credential, _ := bearerAPIToken(values[0])
	return credential, true
}

// bearerAPIToken: the credential of one "Bearer cpk_..." header value.
func bearerAPIToken(value string) (string, bool) {
	scheme, credential, _ := strings.Cut(strings.TrimSpace(value), " ")
	credential = strings.TrimSpace(credential)
	if !strings.EqualFold(scheme, "Bearer") || !strings.HasPrefix(credential, apiTokenPrefix) {
		return "", false
	}
	return credential, true
}

// apiTokenRequestWithoutSession: the request presents an API token and no
// session cookie. Such a request is authenticated by the token or refused, so
// the origin check that protects cookie sessions has nothing to protect
// (security.go). Any other bearer credential keeps the origin check.
func apiTokenRequestWithoutSession(r *http.Request) bool {
	if _, present := apiTokenCredential(r); !present {
		return false
	}
	_, err := r.Cookie(sessionCookieName)
	return err != nil
}

// parseAPIToken splits cpk_<id>_<secret> by position (the secret's alphabet
// holds '_'), and checks both parts' alphabets and the secret's length.
func parseAPIToken(credential string) (tokenID, secret string, ok bool) {
	if len(credential) != apiTokenLength || !strings.HasPrefix(credential, apiTokenPrefix) {
		return "", "", false
	}
	idEnd := len(apiTokenPrefix) + apiTokenIDLength
	if credential[idEnd] != '_' {
		return "", "", false
	}
	tokenID, secret = credential[len(apiTokenPrefix):idEnd], credential[idEnd+1:]
	for i := 0; i < len(tokenID); i++ {
		if !strings.ContainsRune(apiTokenIDAlphabet, rune(tokenID[i])) {
			return "", "", false
		}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(secret)
	if err != nil || len(decoded) != apiTokenSecretBytes {
		return "", "", false
	}
	return tokenID, secret, true
}

// hashAPITokenSecret is what the database keeps of a secret.
func hashAPITokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// apiTokenMissingHash is compared against when no row matches, so a missing
// id and a wrong secret take the same work.
var apiTokenMissingHash = strings.Repeat("0", sha256.Size*2)

// newAPIToken makes a token: its public id and the full text shown once.
func newAPIToken() (tokenID, secret, full string, err error) {
	raw := make([]byte, apiTokenIDLength+apiTokenSecretBytes)
	if _, err = rand.Read(raw); err != nil {
		return "", "", "", err
	}
	id := make([]byte, apiTokenIDLength)
	for i := range id {
		// 256 is a multiple of 32: every letter is equally likely.
		id[i] = apiTokenIDAlphabet[int(raw[i])%len(apiTokenIDAlphabet)]
	}
	tokenID = string(id)
	secret = base64.RawURLEncoding.EncodeToString(raw[apiTokenIDLength:])
	return tokenID, secret, apiTokenPrefix + tokenID + "_" + secret, nil
}

// ----- the limits -----

type apiTokenGuards struct {
	calls    *rateLimiter // per token id
	failures *rateLimiter // failed token sign-ins per client address

	mu       sync.Mutex
	lastUsed map[int64]time.Time // last write of last_used_at per token row
	// failureRows: when the last failed sign-in row was written per
	// "token|address", and how many failures followed it unwritten.
	failureRows    map[string]time.Time
	failureSkipped map[string]int
}

// apiTokenGuardsByPanel keeps each Panel's limits (one in the product; a
// test's own Panel gets its own, so tests never share a count).
var apiTokenGuardsByPanel sync.Map

func newAPITokenGuards() *apiTokenGuards {
	return &apiTokenGuards{
		calls:    newRateLimiter(apiTokenCallsPerMinute, time.Minute),
		failures: newRateLimiter(apiTokenFailuresPerMinute, time.Minute),
		lastUsed: map[int64]time.Time{},
	}
}

// apiTokenFailureAuditEvery: at most one audit row per token (or "not a
// token") and address in this time; the row after it says how many were
// folded into the one before.
const apiTokenFailureAuditEvery = time.Minute

// auditAPITokenSignInFailure records a failed token sign-in, aggregated per
// token id and address per minute. It names the token's public id (or
// "malformed"), never its secret. rec is nil when no token row matched.
func (p *Panel) auditAPITokenSignInFailure(r *http.Request, guards *apiTokenGuards, tokenID string, rec *apiTokenRecord, reason string) {
	label := "malformed"
	if tokenID != "" {
		label = apiTokenPrefix + tokenID
	}
	key := label + "|" + clientIP(r)
	now := apiTokenNow()
	guards.mu.Lock()
	if guards.failureRows == nil {
		guards.failureRows, guards.failureSkipped = map[string]time.Time{}, map[string]int{}
	}
	if last, ok := guards.failureRows[key]; ok && now.Sub(last) < apiTokenFailureAuditEvery {
		guards.failureSkipped[key]++
		guards.mu.Unlock()
		return
	}
	folded := guards.failureSkipped[key]
	if len(guards.failureRows) > 4096 {
		for k, at := range guards.failureRows {
			if now.Sub(at) >= apiTokenFailureAuditEvery {
				delete(guards.failureRows, k)
				delete(guards.failureSkipped, k)
			}
		}
	}
	guards.failureRows[key] = now
	guards.failureSkipped[key] = 0
	guards.mu.Unlock()
	userID, rowID := 0, 0
	if rec != nil {
		userID, rowID = rec.UserID, int(rec.ID)
	}
	details := map[string]any{"token": label, "reason": reason}
	if folded > 0 {
		details["more_failures_before"] = folded
	}
	p.auditAsDetailed(r, userID, "api_token.sign_in_failed", "api_token", rowID, details)
}

func (p *Panel) apiTokenGuards() *apiTokenGuards {
	if guards, ok := apiTokenGuardsByPanel.Load(p); ok {
		return guards.(*apiTokenGuards)
	}
	guards, _ := apiTokenGuardsByPanel.LoadOrStore(p, newAPITokenGuards())
	return guards.(*apiTokenGuards)
}

// ----- the routes a token reaches -----

// apiTokenRoute is one listed route: its family and whether it changes
// something (a manage token only).
type apiTokenRoute struct {
	family string
	write  bool
}

// apiTokenExactRoutes are the listed routes without an id ("METHOD path").
var apiTokenExactRoutes = map[string]apiTokenRoute{
	"GET /api/v1/domains":         {family: "domains"},
	"POST /api/v1/domains/create": {family: "domains", write: true},
}

// apiTokenDomainRoutes are the listed routes of one domain ("METHOD tail",
// the tail after /api/v1/domains/{id}/; "" is the domain itself and
// databases/{id} one database).
var apiTokenDomainRoutes = map[string]apiTokenRoute{
	"DELETE ":                    {family: "domains", write: true},
	"GET hosting":                {family: "domains"},
	"GET usage":                  {family: "domains"},
	"GET stats/visitors":         {family: "domains"},
	"GET dns/records":            {family: "dns"},
	"POST dns/records":           {family: "dns", write: true},
	"PUT dns/records":            {family: "dns", write: true},
	"DELETE dns/records":         {family: "dns", write: true},
	"GET mail/accounts":          {family: "mail"},
	"POST mail/accounts":         {family: "mail", write: true},
	"PUT mail/accounts":          {family: "mail", write: true},
	"DELETE mail/accounts":       {family: "mail", write: true},
	"PUT mail/accounts/password": {family: "mail", write: true},
	"GET databases":              {family: "databases"},
	"POST databases":             {family: "databases", write: true},
	"DELETE databases/{id}":      {family: "databases", write: true},
	"GET backups":                {family: "backups"},
	"POST backups":               {family: "backups", write: true},
	"DELETE backups":             {family: "backups", write: true},
}

// apiTokenRouteFor names the listed route a request reaches, or false. A
// path is compared in its escaped form, and a domain route goes through the
// dispatcher's own strict matcher, so a path this list names is a path the
// dispatcher serves the same way; anything unusual is simply not listed.
func apiTokenRouteFor(r *http.Request) (apiTokenRoute, bool) {
	if r == nil || r.URL == nil {
		return apiTokenRoute{}, false
	}
	escaped := r.URL.EscapedPath()
	if route, ok := apiTokenExactRoutes[r.Method+" "+escaped]; ok && r.URL.Path == escaped {
		return route, true
	}
	if !strings.HasPrefix(escaped, "/api/v1/domains/") {
		return apiTokenRoute{}, false
	}
	segments, ok := strictRouteSegments(r, "/api/v1/domains/")
	if !ok {
		return apiTokenRoute{}, false
	}
	if _, ok := strictPositiveID(segments[0]); !ok {
		return apiTokenRoute{}, false
	}
	match, ok := matchDomainSubroute(r)
	if !ok || !routeAllows(r.Method, match.methods) {
		return apiTokenRoute{}, false
	}
	tail := strings.Join(segments[1:], "/")
	if len(segments) == 3 && segments[1] == "databases" {
		if _, ok := strictPositiveID(segments[2]); !ok || match.kind != "database-delete" {
			return apiTokenRoute{}, false
		}
		tail = "databases/{id}"
	}
	route, ok := apiTokenDomainRoutes[r.Method+" "+tail]
	return route, ok
}

// ----- the server switch and the permission -----

// remoteAPIEnabled reads the server switch. A failed read is an error, never
// "on".
func (p *Panel) remoteAPIEnabled(ctx context.Context) (bool, error) {
	var value string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingRemoteAPIEnabled).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return value == "1", nil
}

// apiGrantor is who can open the remote API for an account: the
// administrator, or the reseller ("your provider") of a reseller's customer.
type apiGrantor struct {
	Kind string // "administrator" or "provider"
	Name string // the provider's user name
}

// text is the grantor in an English sentence.
func (g apiGrantor) text() string {
	if g.Kind == "provider" && g.Name != "" {
		return "your provider (" + g.Name + ")"
	}
	return "the administrator"
}

var apiGrantorAdministrator = apiGrantor{Kind: "administrator"}

// remoteAPIRefusal: "" when the account may use the remote API; else the
// layer that refuses (administrator, reseller, subscription, add_on, or
// "no_subscription"/"role") and who can open it. An administrator always
// may. A reseller needs it offered by the administrator and in the reseller's
// own rights. A customer needs it in every subscription the account holds: a
// token acts for the whole account, so one subscription without the
// permission keeps the account's tokens closed.
func (p *Panel) remoteAPIRefusal(ctx context.Context, c *Caller) (layer string, grantor apiGrantor, err error) {
	switch callerAccountRole(c) {
	case roleAdmin:
		return "", apiGrantor{}, nil
	case roleReseller:
		admin, err := p.loadAdminOfferLayer(ctx)
		if err != nil {
			return "", apiGrantor{}, err
		}
		if !admin.permits(rightRemoteAPI) {
			return rightsLayerAdministrator, apiGrantorAdministrator, nil
		}
		state, err := resellerRightsState(ctx, p.db.GetDB(), c.ID)
		if err != nil {
			return "", apiGrantor{}, err
		}
		if !state.Set.permits(rightRemoteAPI) {
			return rightsLayerReseller, apiGrantorAdministrator, nil
		}
		return "", apiGrantor{}, nil
	case roleCustomer:
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id`, c.ID)
		if err != nil {
			return "", apiGrantor{}, err
		}
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return "", apiGrantor{}, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", apiGrantor{}, err
		}
		rows.Close()
		if len(ids) == 0 {
			return "no_subscription", apiGrantorAdministrator, nil
		}
		for _, id := range ids {
			e, err := p.effectiveSubscriptionRights(ctx, id)
			if err != nil {
				return "", apiGrantor{}, err
			}
			if layer := e.permissionRefusal(rightRemoteAPI); layer != "" {
				grantor := apiGrantorAdministrator
				if layer == rightsLayerSubscription && e.ResellerID != 0 && e.OwnerID != e.ResellerID && e.ResellerName != "" {
					grantor = apiGrantor{Kind: "provider", Name: e.ResellerName}
				}
				return layer, grantor, nil
			}
		}
		return "", apiGrantor{}, nil
	}
	return "role", apiGrantorAdministrator, nil
}

// ----- one token request -----

type apiTokenRecord struct {
	ID           int64
	UserID       int
	TokenID      string
	SecretSHA256 string
	Scope        string
	// Current: the owner's credential epoch and e-mail address are the ones
	// the token was made under.
	Current    bool
	ExpiresAt  int64
	LastUsedAt sql.NullInt64
	RevokedAt  sql.NullInt64
}

func (p *Panel) lookupAPIToken(ctx context.Context, tokenID string) (apiTokenRecord, error) {
	var rec apiTokenRecord
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT t.id, t.user_id, t.token_id, t.secret_sha256, t.scope,
		       (t.auth_epoch = u.auth_epoch AND t.owner_email = COALESCE(u.email, '')),
		       t.expires_at, t.last_used_at, t.revoked_at
		FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_id = ?`, tokenID).Scan(&rec.ID, &rec.UserID, &rec.TokenID, &rec.SecretSHA256, &rec.Scope,
		&rec.Current, &rec.ExpiresAt, &rec.LastUsedAt, &rec.RevokedAt)
	return rec, err
}

// The sentences an API client reads (D-024: what happened, who acts, what
// next). The screen has its own words for the codes it shows.
const (
	apiTokenWithSessionMessage = "This request carries both a signed-in session and an API token, so it was refused and nothing was changed. " +
		"Send the token alone (Authorization: Bearer ...), without the panel's session cookie."
	apiDisabledMessage = "The remote API is turned off on this server, so the token was not accepted and nothing was changed. " +
		"The administrator turns it on in Settings, Panel; the token works again once it is on."
	apiTokenInvalidMessage = "The API token is not valid, so the request was refused and nothing was changed. " +
		"Copy the whole token (cpk_...) from where you saved it, or make a new one on My account."
	apiTokenRevokedMessage = "This API token was removed, so it no longer works and nothing was changed. " +
		"Make a new token on My account."
	apiTokenExpiredMessage = "This API token has expired, so it no longer works and nothing was changed. " +
		"Make a new token on My account."
	apiTokenEndedMessage = "This API token ended because, after it was made, the account's password, e-mail address or two-factor sign-in changed, or an administrator changed the account. " +
		"Nothing was changed. Make a new token on My account."
	apiTokenRateLimitedMessage = "This API token sent more than 120 requests in a minute, so this one was refused and nothing was changed. " +
		"Wait a minute, then send it again more slowly."
	apiTokenSignInLimitMessage = "Too many requests from this address carried a wrong API token in the last minute, so this one was refused and nothing was changed. " +
		"Wait a minute and check the token before trying again."
	apiTokenRouteRefusedMessage = "API tokens cannot use this address, so the request was refused and nothing was changed. " +
		"Use the panel itself for this, or one of the addresses listed in the Remote API guide."
	apiTokenScopeMessage = "This API token can only read, so this change was refused and nothing was changed. " +
		"Make a token with the Manage scope on My account for changes."
	apiTokenTeamMemberMessage = "Additional users cannot use API tokens, so the request was refused and nothing was changed. " +
		"The account's owner can make a token on My account."
)

// apiTokenNotPermittedMessage names who can open the permission.
func apiTokenNotPermittedMessage(grantor apiGrantor) string {
	return "This account's plan does not include the remote API, so the request was refused and nothing was changed. " +
		"Ask " + grantor.text() + " to include \"Remote API\" in your plan; the token works again once it is."
}

// serveAPITokenRequest authenticates one request by its token and passes it
// on, or writes the refusal. See the top of this file for the order.
func (p *Panel) serveAPITokenRequest(w http.ResponseWriter, r *http.Request, credential string, next http.Handler) {
	w.Header().Set("Cache-Control", "no-store")
	if _, err := r.Cookie(sessionCookieName); err == nil {
		writeCodedError(w, http.StatusBadRequest, errCodeAPITokenWithSession, apiTokenWithSessionMessage, "")
		return
	}
	if _, err := r.Cookie(impersonatorCookieName); err == nil {
		writeCodedError(w, http.StatusBadRequest, errCodeAPITokenWithSession, apiTokenWithSessionMessage, "")
		return
	}
	if p.db == nil || p.users == nil {
		writeAuthStatusUnavailable(w)
		return
	}
	guards := p.apiTokenGuards()
	address := clientIP(r)
	// The presented token is verified first. Its secret is 256 bits, so a
	// right token is never refused for its neighbours' mistakes: the
	// per-address limit answers only requests whose token does not verify
	// (one address can be a whole office behind NAT or a proxy).
	ctx := r.Context()
	tokenID, secret, parsed := parseAPIToken(credential)
	var rec apiTokenRecord
	verified, found := false, false
	if parsed {
		var err error
		rec, err = p.lookupAPIToken(ctx, tokenID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeAuthStatusUnavailable(w)
			return
		}
		found = err == nil
		stored := apiTokenMissingHash
		if found {
			stored = rec.SecretSHA256
		}
		verified = subtle.ConstantTimeCompare([]byte(hashAPITokenSecret(secret)), []byte(stored)) == 1 && found
	}
	if !verified {
		if guards.failures.blocked(address) {
			w.Header().Set("Retry-After", apiTokenRetryAfter)
			writeCodedError(w, http.StatusTooManyRequests, errCodeAPITokenSignInLimit, apiTokenSignInLimitMessage, "")
			return
		}
		guards.failures.allow(address)
		// The owner of a real token id sees the wrong secret in their
		// record; an unknown id or a malformed token has no owner.
		switch {
		case !parsed:
			p.auditAPITokenSignInFailure(r, guards, "", nil, "malformed")
		case found:
			p.auditAPITokenSignInFailure(r, guards, tokenID, &rec, "wrong_secret")
		default:
			p.auditAPITokenSignInFailure(r, guards, tokenID, nil, "unknown")
		}
		writeCodedError(w, http.StatusUnauthorized, errCodeAPITokenInvalid, apiTokenInvalidMessage, "")
		return
	}
	// The token verified: its holder learns why it does not work now. These
	// count as failed sign-ins, but never lock the address out.
	ended := func(reason, code, message string) {
		guards.failures.allow(address)
		p.auditAPITokenSignInFailure(r, guards, rec.TokenID, &rec, reason)
		writeCodedError(w, http.StatusUnauthorized, code, message, "")
	}
	if rec.RevokedAt.Valid {
		ended("revoked", errCodeAPITokenRevoked, apiTokenRevokedMessage)
		return
	}
	now := apiTokenNow().Unix()
	if now >= rec.ExpiresAt {
		ended("expired", errCodeAPITokenExpired, apiTokenExpiredMessage)
		return
	}
	if !rec.Current {
		ended("ended", errCodeAPITokenEnded, apiTokenEndedMessage)
		return
	}
	// Whether the remote API is on is told only to the holder of a working
	// token; anyone else got the invalid-token answer above.
	enabled, err := p.remoteAPIEnabled(ctx)
	if err != nil {
		writeAuthStatusUnavailable(w)
		return
	}
	if !enabled {
		writeCodedError(w, http.StatusForbidden, errCodeAPIDisabled, apiDisabledMessage, "")
		return
	}
	if !guards.calls.allow(rec.TokenID) {
		w.Header().Set("Retry-After", apiTokenRetryAfter)
		writeCodedError(w, http.StatusTooManyRequests, errCodeAPITokenRateLimited, apiTokenRateLimitedMessage, "")
		return
	}

	c, _, ok := p.admitAccountIdentity(w, r, rec.UserID)
	if !ok {
		return
	}
	if c.normalizedAccountType() != core.AccountTypeAccount {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenTeamMember, apiTokenTeamMemberMessage, "")
		return
	}
	route, listed := apiTokenRouteFor(r)
	if !listed {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenRouteRefused, apiTokenRouteRefusedMessage, "")
		return
	}
	// Any scope but manage reads only: an unknown stored value never
	// widens a token.
	if route.write && rec.Scope != apiTokenScopeManage {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenScope, apiTokenScopeMessage, "")
		return
	}
	layer, grantor, err := p.remoteAPIRefusal(ctx, c)
	if err != nil {
		writeAuthStatusUnavailable(w)
		return
	}
	if layer != "" {
		writeCodedError(w, http.StatusForbidden, errCodeAPITokenNotPermitted, apiTokenNotPermittedMessage(grantor), "")
		return
	}

	c.APIToken = &apiTokenUse{ID: rec.ID, TokenID: rec.TokenID, Scope: rec.Scope}
	r, ok = p.admitCallerGates(w, r, c)
	if !ok {
		return
	}
	p.noteAPITokenUse(r.Context(), guards, rec, address)
	if route.write {
		p.auditDetailed(r, "api_token.call", "api_token", int(rec.ID), map[string]any{
			"method": r.Method, "path": r.URL.Path, "family": route.family,
		})
	}
	next.ServeHTTP(w, r)
}

// noteAPITokenUse writes the last use at most once a minute per token. A
// failed write is logged; it never fails the request it describes.
func (p *Panel) noteAPITokenUse(ctx context.Context, guards *apiTokenGuards, rec apiTokenRecord, address string) {
	now := apiTokenNow()
	guards.mu.Lock()
	last, seen := guards.lastUsed[rec.ID]
	if seen && now.Sub(last) < apiTokenLastUsedEvery {
		guards.mu.Unlock()
		return
	}
	guards.lastUsed[rec.ID] = now
	guards.mu.Unlock()
	cutoff := now.Add(-apiTokenLastUsedEvery).Unix()
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE api_tokens SET last_used_at = ?, last_used_ip = ?
		WHERE id = ? AND (last_used_at IS NULL OR last_used_at <= ?)`,
		now.Unix(), address, rec.ID, cutoff); err != nil {
		log.Printf("[api-token] last use of token %d not recorded: %v", rec.ID, err)
	}
}

// apiTokenRetryAfter is the Retry-After of a limit, in seconds.
var apiTokenRetryAfter = strconv.Itoa(int(time.Minute / time.Second))
