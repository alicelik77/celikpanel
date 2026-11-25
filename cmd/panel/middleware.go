package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
)

type contextKey string

const callerKey contextKey = "caller"

// requireAuth wraps the whole mux. Everything under /api requires a valid
// session except the login endpoint; static assets and SPA routes are
// public so the login screen itself can load.
//
// requireAuth tüm mux'ı sarar. /api altındaki her şey, giriş uç noktası
// dışında geçerli bir oturum gerektirir; statik dosyalar ve SPA rotaları
// herkese açıktır, böylece giriş ekranının kendisi yüklenebilir.
func (p *Panel) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if remoteDNSMachineVerified(r) {
			if p.allowLicensedPanel(w, r) {
				next.ServeHTTP(w, r)
			}
			return
		}
		// API preflights never need application data. Terminate them here so an
		// unauthenticated OPTIONS request cannot reach a handler that happens to
		// omit its own method check and disclose the protected GET response.
		if r.Method == http.MethodOptions && strings.HasPrefix(r.URL.Path, "/api/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// alpha.128 (G15): a request that carries an API token is decided by
		// the token layer alone, before public paths and sessions; it never
		// falls through to the cookie path below (api_token_auth.go).
		if credential, present := apiTokenCredential(r); present {
			p.serveAPITokenRequest(w, r, credential, next)
			return
		}

		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}

		// The sign-in cookies are read through one rule (signin_cookies.go):
		// a name sent more than once is refused and removed.
		cookie, present, err := p.readSessionCookie(r)
		if err == nil {
			_, err = p.impersonatorCookieValue(r)
			if errors.Is(err, http.ErrNoCookie) {
				err = nil
			}
		}
		if err != nil {
			p.refuseDuplicateSigninCookies(w)
			return
		}
		// Every refusal of the session below also removes a "Log in as" return
		// cookie the request carried: it holds the operator's own session, and
		// a browser whose session is refused must not keep it (second look at
		// PR #368, A). A refusal of one request that leaves the session
		// usable (another site's request, an administrator-only path, a team
		// member's scope, a suspended subscription's screen, the licence, the
		// administrator address list) keeps it, so "Return" still works.
		if !present {
			p.expirePlainSigninCookiesSent(w, r)
			p.expireImpersonatorCookieSent(w, r)
			writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
			return
		}
		// The transition carries a plain-named session over only on an /api/
		// request (the update page's own polling is one), never on a response
		// another component makes and may let a cache keep (/dbtool/). Such a
		// request is answered 401 with the plain cookie left in place, so the
		// next /api/ call still carries the session over.
		if cookie.legacy && !strings.HasPrefix(r.URL.Path, "/api/") {
			p.expireReturnCookieSent(w, r)
			w.Header().Set("Cache-Control", "no-store")
			writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
			return
		}
		// A signed-in browser's /api/ call from another site or another
		// origin of the same site is refused (fetch metadata).
		if refuseCrossSiteAPIRequest(w, r) {
			return
		}

		if p.sessions == nil || p.users == nil {
			writeAuthStatusUnavailable(w)
			return
		}
		session, err := p.sessions.ValidateIdentity(r.Context(), cookie.token)
		userID := session.UserID
		if err != nil {
			if errors.Is(err, auth.ErrSessionInvalid) {
				// Ended elsewhere (revoked, expired) or never valid. Nothing on
				// the server says any more whether it was a "Log in as" session
				// or which session the return cookie kept, so only the cookie
				// is removed here (endKeptOperatorSession needs the operator).
				p.expirePlainSigninCookiesSent(w, r)
				p.expireImpersonatorCookieSent(w, r)
				writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
			} else {
				writeAuthStatusUnavailable(w)
			}
			return
		}
		// The transition: a session from before the new names, presented
		// under the plain name (legacySigninAccepted).
		if cookie.legacy && !p.legacySigninAccepted(session, time.Now()) {
			p.refuseLegacySignin(w, r)
			return
		}
		// alpha.128 (G4): a session unused for the idle timeout ends here;
		// otherwise its use is recorded (at most once a minute).
		if !p.sessionIdleCheck(w, r, cookie.token, session) {
			return
		}

		c, u, refusal := p.readAccountIdentity(r.Context(), userID)
		if c == nil {
			// A still-valid "Log in as" session whose account may no longer be
			// used (suspended, deleted, inactive) ends here with the
			// operator's kept session (endRefusedLogInAs).
			if session.ImpersonatorID > 0 && refusal.refusesIdentity() {
				p.endRefusedLogInAs(w, r, cookie.token, session)
				return
			}
			if refusal.refusesIdentity() {
				p.expireReturnCookieSent(w, r)
			}
			refusal.write(w)
			return
		}
		if cookie.legacy && !c.hasAccountRole(roleAdmin) {
			p.refuseLegacySignin(w, r)
			return
		}
		// A "Log in as" session lives only while its operator is active and
		// may still manage this account (alpha.114).
		// "Farklı giriş yap" oturumu, operatörü etkin olduğu ve bu hesabı hâlâ
		// yönetebildiği sürece yaşar.
		if session.ImpersonatorID > 0 {
			allowed, checkErr := p.impersonationStillAllowed(r.Context(), session.ImpersonatorID, u)
			if checkErr != nil {
				writeAuthStatusUnavailable(w)
				return
			}
			if !allowed {
				p.endRefusedLogInAs(w, r, cookie.token, session)
				return
			}
			c.ImpersonatorID = session.ImpersonatorID
		}
		// alpha.128 (G4): administrator sessions, and "Log in as" sessions
		// an administrator opened, only from the listed addresses.
		if !p.adminSessionAddressAllowed(r, c) {
			writePanelAccessRefused(w)
			return
		}

		r, ok := p.admitCallerGates(w, r, c)
		if !ok {
			return
		}
		if cookie.legacy {
			// Carried over: the same session under the name in use from now
			// on; the plain names are removed. The answer carries the session
			// token in Set-Cookie, so no cache may keep it.
			w.Header().Set("Cache-Control", "no-store")
			p.setSessionCookie(w, r, cookie.token, session.ExpiresAt)
		} else {
			// A plain-named cookie beside the name in use is never read.
			p.expirePlainSigninCookiesSent(w, r)
		}
		next.ServeHTTP(w, withSessionToken(r, cookie.token))
	})
}

// admitAccountIdentity reads the account a credential names and builds its
// caller: the account must exist and be active, its stored identity must be
// consistent, and a team member's parent customer must be live. It writes the
// refusal and answers false otherwise. Sessions and API tokens (alpha.128)
// share it, so a token never admits an account a session would refuse.
//
// admitAccountIdentity, kimlik bilgisinin adlandırdığı hesabı okur ve
// çağıranı kurar; oturum ve API jetonu aynı denetimi paylaşır.
func (p *Panel) admitAccountIdentity(w http.ResponseWriter, r *http.Request, userID int) (*Caller, *core.User, bool) {
	c, u, refusal := p.readAccountIdentity(r.Context(), userID)
	if c == nil {
		refusal.write(w)
		return nil, nil, false
	}
	return c, u, true
}

// accountRefusal is why readAccountIdentity admitted no caller: a refusal of
// the account (401, 403) or an account that could not be read (503).
type accountRefusal struct {
	status  int
	code    string
	message string
}

var (
	accountRefusedAuthRequired = accountRefusal{http.StatusUnauthorized, errCodeAuthRequired, "authentication required"}
	accountRefusedSuspended    = accountRefusal{http.StatusForbidden, errCodeAccountSuspended, "account suspended"}
	accountReadUnavailable     = accountRefusal{status: http.StatusServiceUnavailable}
)

// refusesIdentity: the account was read and may not be used (not a failed read).
func (refusal accountRefusal) refusesIdentity() bool {
	return refusal.status == http.StatusUnauthorized || refusal.status == http.StatusForbidden
}

func (refusal accountRefusal) write(w http.ResponseWriter) {
	if !refusal.refusesIdentity() {
		writeAuthStatusUnavailable(w)
		return
	}
	writeCodedError(w, refusal.status, refusal.code, refusal.message, "")
}

// readAccountIdentity is admitAccountIdentity without writing: the caller, or
// (nil and) why there is none.
func (p *Panel) readAccountIdentity(ctx context.Context, userID int) (*Caller, *core.User, accountRefusal) {
	// Attach the caller (id + role) so handlers can enforce ownership.
	// Suspended accounts are cut off immediately, whatever session they
	// still hold.
	// Çağıranı (kimlik + rol) iliştir; böylece işleyiciler sahipliği
	// uygulayabilir. Askıya alınmış hesaplar, ellerinde hangi oturum
	// olursa olsun anında kesilir.
	// A failed read blocks admission without inventing an invalid session.
	// Okuma hatası erişimi durdurur; oturumu geçersiz saymaz.
	u, err := p.users.GetByID(ctx, userID)
	if err != nil || u == nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return nil, nil, accountRefusedAuthRequired
		}
		return nil, nil, accountReadUnavailable
	}
	if u.Status == "suspended" {
		return nil, nil, accountRefusedSuspended
	}
	if u.Status != "active" {
		return nil, nil, accountRefusedAuthRequired
	}

	identity, ok := u.EffectiveIdentity()
	if !ok || identity.UserID != userID {
		return nil, nil, accountRefusedAuthRequired
	}

	accountType := u.AccountType
	if accountType == "" {
		accountType = core.AccountTypeAccount
	}

	// An additional-user marker is only meaningful while it points to a
	// live, real customer account. Validate the parent through the
	// repository on every authenticated request so a deleted, suspended or
	// retyped parent immediately revokes the member's effective identity.
	if accountType == core.AccountTypeAdditionalUser {
		parent, parentErr := p.users.GetByID(ctx, identity.CustomerID)
		if parentErr != nil || parent == nil {
			if errors.Is(parentErr, repositories.ErrUserNotFound) {
				return nil, nil, accountRefusedAuthRequired
			}
			return nil, nil, accountReadUnavailable
		}
		if parent.Status == "suspended" {
			return nil, nil, accountRefusedSuspended
		}
		parentIdentity, parentOK := parent.EffectiveIdentity()
		parentType := parent.AccountType
		if parentType == "" {
			parentType = core.AccountTypeAccount
		}
		if parent.Status != "active" || !parentOK ||
			parentType != core.AccountTypeAccount ||
			parentIdentity.UserID != identity.CustomerID ||
			parentIdentity.CustomerID != identity.CustomerID ||
			parentIdentity.Role != roleCustomer {
			return nil, nil, accountRefusedAuthRequired
		}
	}

	c := &Caller{
		ID:          identity.UserID,
		Role:        identity.Role,
		AccountType: accountType,
		CustomerID:  identity.CustomerID,
	}
	if !c.validAuthorizationIdentity() {
		return nil, nil, accountRefusedAuthRequired
	}
	return c, u, accountRefusal{}
}

// admitCallerGates applies the gates every authenticated request passes once
// its identity is known: team-member scope, a suspended subscription, the
// administrator-only paths and the licence. It returns the request with the
// caller attached. Sessions and API tokens (alpha.128) share it.
//
// admitCallerGates, kimliği bilinen her isteğin geçtiği kapıları uygular;
// oturum ve API jetonu aynı kapılardan geçer.
func (p *Panel) admitCallerGates(w http.ResponseWriter, r *http.Request, c *Caller) (*http.Request, bool) {
	// Additional users are tenant-scoped team members. Server-global and
	// account-management surfaces are not inherited from a parent customer.
	if c.isAdditionalUser() && isAdditionalUserRestrictedPath(r.URL.Path) {
		writeCodedError(w, http.StatusForbidden, errCodeAdditionalUserScope, "resource is unavailable to additional users", "")
		return r, false
	}

	// A suspended subscription's owner (and its team) or a suspended
	// reseller sees only the suspended screen (alpha.117); every other
	// call is refused with who to contact in /auth/me.
	// Askıya alınmış hesap yalnız askı ekranını görür.
	if !suspendedAccountAllowedPath(r.URL.Path) && !c.hasAccountRole(roleAdmin) {
		suspension, err := p.accountServiceSuspension(r.Context(), c)
		if err != nil {
			writeAuthStatusUnavailable(w)
			return r, false
		}
		if suspension != nil {
			writeCodedError(w, http.StatusForbidden, errCodeSubscriptionSuspended,
				"this account's subscription is suspended; contact the reseller or administrator named on the screen", "")
			return r, false
		}
	}

	// Server/OS-layer endpoints are administrator-only (ROLES.md: only the
	// admin touches services, config files, and infrastructure). Tenant
	// data (domains) is instead ownership-filtered inside its handlers.
	// Sunucu/OS-katmanı uç noktaları yalnızca yöneticiye açıktır (ROLES.md:
	// servisler, config dosyaları ve altyapıya yalnızca yönetici dokunur).
	// Kiracı verisi (domain'ler) ise kendi işleyicilerinde sahiplik
	// süzgecinden geçer.
	if isAdminOnlyPath(r.URL.Path) && !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return r, false
	}

	// The license exception uses only this freshly validated session identity.
	r = r.WithContext(context.WithValue(r.Context(), callerKey, c))
	if !p.allowLicensedPanel(w, r) {
		return r, false
	}
	return r, true
}

// isPublicPath decides which requests skip authentication.
// isPublicPath, hangi isteklerin kimlik doğrulamayı atlayacağına karar verir.
func isPublicPath(r *http.Request) bool {
	// The database-tool proxy is NOT public: it fronts loopback-only web
	// apps and demands an authenticated, authorized session.
	// Veritabanı-aracı vekili herkese açık DEĞİLDİR: yalnız-loopback web
	// uygulamalarının önündedir ve kimliği doğrulanmış, yetkili oturum ister.
	if r.URL.Path == "/dbtool" || strings.HasPrefix(r.URL.Path, "/dbtool/") {
		return false
	}
	// Non-API paths are static files or SPA routes and must stay public.
	// API dışı yollar statik dosyalar ya da SPA rotalarıdır ve herkese
	// açık kalmalıdır.
	if !strings.HasPrefix(r.URL.Path, "/api") {
		return true
	}
	// The login and demo-credentials endpoints are public. Demo returns
	// nothing unless the server was started with --demo, so this is safe.
	// Giriş ve demo kimlik bilgileri uç noktaları herkese açıktır. Demo,
	// sunucu --demo ile başlatılmadıkça hiçbir şey döndürmez; bu yüzden
	// güvenlidir.
	// login/totp is the second step of an unauthenticated sign-in — it carries
	// the pending token, not a session, so it too must be public.
	// login/totp, kimlik-doğrulamasız girişin ikinci adımıdır — oturum değil
	// bekleme jetonu taşır; o da herkese açık olmalıdır.
	if r.URL.Path == panelAccessAddressPath && r.Method == http.MethodGet {
		return true
	}
	if r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/demo" || r.URL.Path == "/api/v1/auth/login/totp" {
		return true
	}
	// Forgot password, a reset or invitation link, an address confirmation:
	// the link's token is the credential (account_recovery_handlers.go).
	// Parolamı unuttum ve hesap bağlantıları: kimlik bilgisi bağlantının jetonudur.
	return isAccountRecoveryPublicPath(r)
}

// isAdminOnlyPath matches the server/OS-layer and infrastructure endpoints
// that only administrators may call. Read-only dashboard health, auth, and
// the ownership-filtered domain routes are intentionally not listed here.
// Additional-user restrictions for server-global read-only data are enforced
// separately so reseller and customer account behavior remains unchanged.
// isAdminOnlyPath, yalnızca yöneticilerin çağırabileceği sunucu/OS-katmanı ve
// altyapı uç noktalarını eşler. Salt-okunur panel sağlığı
// (/api/v1/system/stats), kimlik doğrulama ve sahiplik-süzgeçli domain
// rotaları bilerek listelenmemiştir.
func isAdminOnlyPath(path string) bool {
	if path == panelAvailabilityPath {
		return false
	}
	if path == panelRecoveryStatusPath {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/dns/remote/") {
		return true
	}
	if path == panelLicensePath {
		return true
	}
	if path == "/api/v1/system-databases" || strings.HasPrefix(path, "/api/v1/system-databases/") {
		return true
	}
	if path == storeCatalogAdminPath || strings.HasPrefix(path, storeCatalogAdminPath+"/") {
		return true
	}
	// alpha.118: offering AI and automation apps is the administrator's.
	if path == serviceAppsAdminPath || strings.HasPrefix(path, serviceAppsAdminPath+"/") {
		return true
	}
	// alpha.119: offering runtime apps is the administrator's.
	if path == runtimeAppsAdminPath || strings.HasPrefix(path, runtimeAppsAdminPath+"/") {
		return true
	}
	// alpha.134: the rewrite of database rights to exact names.
	if path == databaseGrantFormsPath {
		return true
	}
	// 20 Nov 2026 (S1b): closing old PostgreSQL databases to other accounts.
	if path == pgPublicPath {
		return true
	}
	// alpha.117 (D-043): a security emergency is the administrator's alone.
	if path == securityEmergenciesPath || strings.HasPrefix(path, securityEmergenciesPath+"/") {
		return true
	}
	// /api/v2/ is deliberately NOT here anymore (B1 role split, Jul 17):
	// its DB/user/grant endpoints are tenant-scoped inside their handlers
	// (callerSubscriptionID + canAccessDBServer); only SERVER REGISTRATION
	// stays admin, enforced in its own handlers — registering an arbitrary
	// host/port/root-password is infrastructure, not tenant self-service.
	// /dbtool/ likewise: any authenticated session may reach the proxy; the
	// tools' own database-credential login is the real authorization layer.
	// /api/v2/ artık bilerek burada DEĞİL (B1 rol ayrımı, 17 Tem): DB/
	// kullanıcı/grant uçları kendi handler'larında kiracı-kapsamlı
	// (callerSubscriptionID + canAccessDBServer); yalnız SUNUCU KAYDI admin
	// kalır ve kendi handler'ında uygulanır — keyfi host/port/root-parola
	// kaydı kiracı self-servisi değil altyapıdır. /dbtool/ de öyle: vekile
	// kimlikli her oturum ulaşabilir; gerçek yetki katmanı araçların kendi
	// veritabanı-kimlik girişidir.
	if path == serverSetupPath || strings.HasPrefix(path, serverSetupPath+"/") {
		return true
	}
	adminPrefixes := []string{
		"/api/v1/config",
		"/api/v1/dovecot/",
		"/api/v1/dns/",
		"/api/v1/fail2ban/",
		"/api/v1/firewall",
		"/api/v1/import/",
		"/api/v1/mail/",
		"/api/v1/audit-logs",
		"/api/v1/dashboard",
		"/api/v1/managed-services",
		"/api/v1/metrics/",
		"/api/v1/nginx/",
		// alpha.128: the server's IP addresses.
		"/api/v1/network/",
		"/api/v1/panel/",
		"/api/v1/pdns/",
		"/api/v1/php/",
		"/api/v1/postfix/",
		"/api/v1/repo",
		"/api/v1/service/",
		"/api/v1/security/",
		"/api/v1/system/check",
	}
	for _, prefix := range adminPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// isAdditionalUserRestrictedPath centralizes routes that tenant-scoped team
// members must never inherit from their parent customer account. It covers
// server-global inventory and account, billing, VPN, and database-tool
// management surfaces. Real admin, reseller, and customer accounts bypass
// this additional-user-only guard and retain their existing handler rules.
func isAdditionalUserRestrictedPath(path string) bool {
	switch path {
	case "/api/v1/system/stats",
		"/api/v1/hosting/capabilities",
		"/api/v1/products",
		"/api/v1/domains/create":
		return true
	}

	for _, root := range []string{
		"/api/v1/store",
		"/api/v1/users",
		"/api/v1/team-members",
		"/api/v1/invitations",
		"/api/v1/plans",
		"/api/v1/reseller-plans",
		"/api/v1/rights",
		"/api/v1/remote-backup",
		"/api/v1/subscriptions",
		"/api/v1/transfer-requests",
		"/api/v1/my",
		"/api/v1/suspensions",
		"/api/v1/vpn",
		"/api/v1/runtimes",
		// alpha.128 (G4): a reseller's customers' sessions.
		"/api/v1/reseller",
		"/dbtool",
		// alpha.128 (G15): a team member never holds an API token.
		apiTokensPath,
		// alpha.129: remote access to databases is the account's decision.
		"/api/v1/database-remote",
	} {
		if path == root || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

// currentCaller returns the authenticated caller, or nil if none.
// currentCaller, kimliği doğrulanmış çağıranı, yoksa nil döndürür.
func currentCaller(r *http.Request) *Caller {
	if c, ok := r.Context().Value(callerKey).(*Caller); ok {
		return c
	}
	return nil
}

// currentUserID returns the authenticated user's ID, or 0 if none.
// currentUserID, kimliği doğrulanmış kullanıcının kimliğini, yoksa 0
// döndürür.
func currentUserID(r *http.Request) int {
	if c := currentCaller(r); c != nil {
		return c.ID
	}
	return 0
}

// Preserve the cookie and deny this request until authentication can be observed.
// Çerezi koru; kimlik durumu okunabilene kadar bu isteği reddet.
func writeAuthStatusUnavailable(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeCodedError(w, http.StatusServiceUnavailable, errCodeAuthStatusUnavailable,
		"Authentication status could not be checked. Check again; this request did not proceed.", "")
}
