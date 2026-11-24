package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
)

// The panel's sign-in cookies (security lane A1, 24 November 2026).
//
// Over HTTPS the session and "Log in as" cookies carry the __Host- prefix. A
// browser keeps such a cookie only when it is Secure, has Path=/ and no
// Domain, and it never sends one that another host set: a website on another
// address under the panel's registrable domain (shop.example.com beside
// panel.example.com) can no longer plant a cookie under the panel's name with
// Domain=example.com or shadow it with a longer path. Cookies do not separate
// ports, so this does not separate services on the panel's own host name: a
// service there, on any port (also a website the administrator places on
// exactly that name), receives these cookies and can set them. That is the
// next lane's subject (a sign-in binding, so that the cookie alone is not
// enough). The plain names are kept only for development
// over plain HTTP (--insecure-cookies), and the plain session name is read in
// a bounded transition so that an update the administrator follows on the
// update page ends there without a sign-in (signin_cookies.go,
// legacySigninAccepted).

const (
	// sessionCookieName and impersonatorCookieName are the plain names: the
	// names in use over plain HTTP, and the names releases before this one set.
	sessionCookieName      = "celikpanel_session"
	impersonatorCookieName = "celikpanel_impersonator"
	// The names in use over HTTPS.
	hostSessionCookieName      = "__Host-celikpanel_session"
	hostImpersonatorCookieName = "__Host-celikpanel_impersonator"

	// settingSigninCookieNamesSince is when this installation first ran a
	// panel that sets the __Host- names (panel_settings, RFC 3339 UTC). Written
	// once, never moved; the transition below is counted from it.
	settingSigninCookieNamesSince = "auth.signin_cookie_names_since"

	errCodeSigninCookiesDuplicate = "SIGNIN_COOKIES_DUPLICATE"
	errCodeCrossSiteRequest       = "CROSS_SITE_REQUEST_REFUSED"
	// errCodeSigninFromBeforeUpdate refuses a plain-named session the
	// transition does not carry. Like SIGNIN_COOKIES_DUPLICATE it is a 401
	// whose reason the sign-in page shows (web/src/auth/signinRefusal.ts).
	errCodeSigninFromBeforeUpdate = "SIGNIN_FROM_BEFORE_UPDATE"
)

const (
	signinCookiesDuplicateMessage = "This browser sent the panel's sign-in cookie more than once, so none of them was used and they were removed. " +
		"Sign in again. If this happens again, a website under the same domain name is setting cookies with the panel's name: " +
		"tell the server administrator, and open the panel only at its own address."
	crossSiteRequestMessage = "This request came from another website, not from the panel's own pages, so it was refused and nothing was changed. " +
		"Open the panel at its own address and do it there."
	legacySigninRefusedMessage = "This sign-in is from before the panel's last update. Sign in again; nothing on the server was changed."
)

// panelCookieNames are every name the panel has used for its own cookies,
// in either form. The database tools' proxy never hands them to a tool.
var panelCookieNames = map[string]bool{
	sessionCookieName:          true,
	impersonatorCookieName:     true,
	hostSessionCookieName:      true,
	hostImpersonatorCookieName: true,
}

var errSigninCookieDuplicate = errors.New("the sign-in cookie arrived more than once")

// sessionCookieNameInUse is the session cookie's name this panel sets.
func (p *Panel) sessionCookieNameInUse() string {
	if p != nil && p.secureCookies {
		return hostSessionCookieName
	}
	return sessionCookieName
}

// impersonatorCookieNameInUse is the "Log in as" cookie's name this panel sets.
func (p *Panel) impersonatorCookieNameInUse() string {
	if p != nil && p.secureCookies {
		return hostImpersonatorCookieName
	}
	return impersonatorCookieName
}

// cookieValues are the values the request carries under exactly this name,
// in the order the browser sent them.
func cookieValues(r *http.Request, name string) []string {
	var values []string
	for _, cookie := range r.Cookies() {
		if cookie.Name == name {
			values = append(values, cookie.Value)
		}
	}
	return values
}

// requestCarriesSigninCookie reports whether the request carries any of the
// panel's sign-in cookies under any name it has used. A request with one is a
// browser session for the origin check and the API-token rules.
func requestCarriesSigninCookie(r *http.Request) bool {
	for _, cookie := range r.Cookies() {
		if panelCookieNames[cookie.Name] {
			return true
		}
	}
	return false
}

// signinCookie is the session token a request presents and whether it came
// under the plain name over HTTPS (the transition, legacySigninAccepted).
type signinCookie struct {
	token  string
	legacy bool
}

// readSessionCookie reads the request's session cookie through the one rule:
// the name in use, exactly once; over HTTPS, when that name is absent, the
// plain name, exactly once. Present is false when there is none. A name sent
// more than once is errSigninCookieDuplicate: the panel cannot know which one
// the browser's own sign-in set.
func (p *Panel) readSessionCookie(r *http.Request) (signinCookie, bool, error) {
	current := nonEmpty(cookieValues(r, p.sessionCookieNameInUse()))
	switch len(current) {
	case 0:
	case 1:
		return signinCookie{token: current[0]}, true, nil
	default:
		return signinCookie{}, false, errSigninCookieDuplicate
	}
	if !p.secureCookies {
		return signinCookie{}, false, nil
	}
	legacy := nonEmpty(cookieValues(r, sessionCookieName))
	switch len(legacy) {
	case 0:
		return signinCookie{}, false, nil
	case 1:
		return signinCookie{token: legacy[0], legacy: true}, true, nil
	default:
		return signinCookie{}, false, errSigninCookieDuplicate
	}
}

// impersonatorCookieValue is the "Log in as" cookie under the name in use,
// exactly once. The plain name is never read over HTTPS: a "Log in as"
// session is not carried across the update (legacySigninAccepted).
func (p *Panel) impersonatorCookieValue(r *http.Request) (string, error) {
	values := nonEmpty(cookieValues(r, p.impersonatorCookieNameInUse()))
	switch len(values) {
	case 0:
		return "", http.ErrNoCookie
	case 1:
		return values[0], nil
	default:
		return "", errSigninCookieDuplicate
	}
}

func nonEmpty(values []string) []string {
	out := values[:0:0]
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// sessionCookie builds the session cookie under the name in use, with the
// security attributes. Secure is on unless the panel runs with
// --insecure-cookies; over HTTPS the __Host- name requires Secure, Path=/ and
// no Domain, which this cookie always has.
// sessionCookie, oturum çerezini kullanılan adla ve güvenlik nitelikleriyle oluşturur.
func (p *Panel) sessionCookie(value string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     p.sessionCookieNameInUse(),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   p.secureCookies,
		Expires:  expires,
	}
}

// cookie builds a session-style cookie consistent with the login handler's
// flags (HttpOnly, SameSite=Lax, Secure per deployment, Path=/, no Domain).
// cookie, giriş işleyicisinin bayraklarıyla tutarlı bir çerez üretir.
func (p *Panel) cookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   p.secureCookies,
		MaxAge:   maxAge,
	}
}

// setSessionCookie gives the browser its session token under the name in use,
// removes a "Log in as" return cookie the request carried (a new sign-in is
// never a "Log in as" session, so a return cookie beside it is left over from
// someone else's "Log in as" and must not lead anywhere) and, over HTTPS,
// removes the plain names a release before this one set when the request
// carried one. Sign-in, its second step and the transition's carry-over use it.
func (p *Panel) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, p.sessionCookie(token, expires))
	p.expireImpersonatorCookieSent(w, r)
	p.expirePlainSigninCookiesSent(w, r)
}

// expireImpersonatorCookieSent removes the "Log in as" cookie under the name
// in use when the request carried it (the plain names are
// expirePlainSigninCookiesSent's).
func (p *Panel) expireImpersonatorCookieSent(w http.ResponseWriter, r *http.Request) {
	name := p.impersonatorCookieNameInUse()
	if len(cookieValues(r, name)) > 0 {
		http.SetCookie(w, p.cookie(name, "", -1))
	}
}

// clearSessionCookie removes the session cookie under every name it had.
func (p *Panel) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, p.sessionCookie("", time.Unix(0, 0)))
	if p.secureCookies {
		http.SetCookie(w, p.plainCookie(sessionCookieName))
	}
}

// setImpersonatorCookie keeps the operator's own session for the way back.
func (p *Panel) setImpersonatorCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, p.cookie(p.impersonatorCookieNameInUse(), token, 0))
	if p.secureCookies {
		http.SetCookie(w, p.plainCookie(impersonatorCookieName))
	}
}

// clearImpersonatorCookie removes the "Log in as" cookie under every name.
func (p *Panel) clearImpersonatorCookie(w http.ResponseWriter) {
	http.SetCookie(w, p.cookie(p.impersonatorCookieNameInUse(), "", -1))
	if p.secureCookies {
		http.SetCookie(w, p.plainCookie(impersonatorCookieName))
	}
}

// expirePlainSigninCookies removes, over HTTPS, the plain names of both
// cookies. Only a cookie this host set with Path=/ is removed this way; one
// another host set for the parent domain stays in that browser, but the panel
// never reads the plain names again once the name in use is present.
func (p *Panel) expirePlainSigninCookies(w http.ResponseWriter) {
	if !p.secureCookies {
		return
	}
	http.SetCookie(w, p.plainCookie(sessionCookieName))
	http.SetCookie(w, p.plainCookie(impersonatorCookieName))
}

// expirePlainSigninCookiesSent removes the plain names when the request
// carried one of them (over HTTPS only).
func (p *Panel) expirePlainSigninCookiesSent(w http.ResponseWriter, r *http.Request) {
	if !p.secureCookies {
		return
	}
	if len(cookieValues(r, sessionCookieName)) > 0 || len(cookieValues(r, impersonatorCookieName)) > 0 {
		p.expirePlainSigninCookies(w)
	}
}

// plainCookie is the deletion of a plain-named cookie set with Path=/.
func (p *Panel) plainCookie(name string) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   p.secureCookies,
		MaxAge:   -1,
	}
}

// refuseDuplicateSigninCookies answers a request that carried a sign-in
// cookie more than once: every name is removed and the person is asked to
// sign in again.
func (p *Panel) refuseDuplicateSigninCookies(w http.ResponseWriter) {
	p.clearSessionCookie(w)
	p.clearImpersonatorCookie(w)
	w.Header().Set("Cache-Control", "no-store")
	writeCodedError(w, http.StatusUnauthorized, errCodeSigninCookiesDuplicate, signinCookiesDuplicateMessage, "")
}

// refuseCrossSiteAPIRequest is the fetch-metadata gate: a cookie-signed-in
// request to /api/ that the browser marks as coming from another site, or
// from another origin of the same site (another host under the same domain,
// another port of the same host), is refused before it reaches a handler.
// The panel's own pages call /api/ from their own origin ("same-origin"); an
// address typed or a bookmark is "none"; a browser without the header (older
// ones, command-line clients) is not affected. The exemptions are by
// position in requireAuth: the machine-authenticated DNS requests, API-token
// requests and the public paths (sign-in, its second step, the demo list,
// the access address and account recovery) are decided before this gate, and
// a request without a sign-in cookie never reaches it.
func refuseCrossSiteAPIRequest(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "same-site", "cross-site":
		w.Header().Set("Cache-Control", "no-store")
		writeCodedError(w, http.StatusForbidden, errCodeCrossSiteRequest, crossSiteRequestMessage, "")
		return true
	}
	return false
}

// loadSigninCookieNamesSince records, once per installation, when a panel
// that sets the __Host- names first started, and keeps that moment. A failure
// leaves it zero: no plain-named session is then accepted, and people who
// were signed in sign in once.
func (p *Panel) loadSigninCookieNamesSince(ctx context.Context) {
	if p == nil || !p.secureCookies || p.db == nil {
		return
	}
	db := p.db.GetDB()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		settingSigninCookieNamesSince, now); err != nil {
		log.Printf("[signin-cookies] the first start of the new cookie names could not be recorded (%v); sessions from before it are not carried over", err)
		return
	}
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingSigninCookieNamesSince).Scan(&raw); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("[signin-cookies] the first start of the new cookie names could not be read (%v); sessions from before it are not carried over", err)
		}
		return
	}
	since, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		log.Printf("[signin-cookies] the recorded first start %q is not a time; sessions from before it are not carried over", raw)
		return
	}
	p.signinCookieNamesSince = since.UTC()
}

// legacySigninAccepted is the transition. Over HTTPS a session presented
// under the plain name, with no cookie under the name in use, is accepted only
// when all of these hold:
//   - the installation recorded when it first ran this code (since);
//   - the session was opened before that moment, so it was issued under the
//     plain name by the release before (a session opened since is only ever
//     accepted under the __Host- name);
//   - now is less than auth.SessionDuration after that moment; every session
//     opened before it has expired by then, so the transition ends for good;
//   - it is not a "Log in as" session, and its account is an administrator
//     (checked after the account is read, requireAuth): the administrator
//     follows the update on the update page; resellers, customers and team
//     members who were signed in sign in once.
//
// The accepted request is answered with the same token under the __Host-
// name and the plain names removed, so that browser never sends the plain
// name again. A plain-named cookie beside one under the name in use is never
// read and is removed.
func (p *Panel) legacySigninAccepted(session auth.SessionIdentity, now time.Time) bool {
	if p == nil || !p.secureCookies {
		return false
	}
	since := p.signinCookieNamesSince
	if since.IsZero() || session.CreatedAt.IsZero() {
		return false
	}
	if !session.CreatedAt.Before(since) || !now.Before(since.Add(auth.SessionDuration)) {
		return false
	}
	return session.ImpersonatorID == 0
}

// refuseLegacySignin answers a plain-named session the transition does not
// carry: the plain names are removed and the person signs in again.
func (p *Panel) refuseLegacySignin(w http.ResponseWriter) {
	p.expirePlainSigninCookies(w)
	w.Header().Set("Cache-Control", "no-store")
	writeCodedError(w, http.StatusUnauthorized, errCodeSigninFromBeforeUpdate, legacySigninRefusedMessage, "")
}

type sessionTokenKey struct{}

// currentSessionToken is the session token requireAuth admitted for this
// request ("" when the request was not admitted by a session). Handlers use
// it instead of reading the cookie again.
func currentSessionToken(r *http.Request) string {
	token, _ := r.Context().Value(sessionTokenKey{}).(string)
	return token
}

func withSessionToken(r *http.Request, token string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), sessionTokenKey{}, token))
}
