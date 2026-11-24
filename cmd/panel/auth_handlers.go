package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/repositories"
)

// loginRequest is the JSON body of a login attempt.
// loginRequest, bir giriş denemesinin JSON gövdesidir.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin verifies credentials and, on success, sets the session
// cookie. Failures return an identical generic message and status so the
// endpoint does not reveal whether a username exists.
//
// handleLogin, kimlik bilgilerini doğrular ve başarılı olursa oturum
// çerezini ayarlar. Başarısızlıklar aynı genel mesaj ve durumu döndürür;
// böylece uç nokta bir kullanıcı adının var olup olmadığını açığa vurmaz.
func (p *Panel) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Throttle credential guessing before doing any work.
	// Herhangi bir iş yapmadan önce kimlik bilgisi tahminini kısıtla.
	if !p.loginLimiter.allow(clientIP(r)) {
		writeClientError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if p.users == nil {
		_, _ = auth.VerifyPassword(req.Password, dummyHash)
		writeSignInStatusUnavailable(w)
		return
	}
	user, err := p.users.GetByUsername(r.Context(), req.Username)
	if err != nil || user == nil {
		// Run a verify against a dummy hash anyway to keep timing uniform,
		// then fail. Kullanıcı yoksa da zamanlamayı eşit tutmak için sahte
		// bir özete karşı doğrulama çalıştır, sonra başarısız ol.
		_, _ = auth.VerifyPassword(req.Password, dummyHash)
		if errors.Is(err, repositories.ErrUserNotFound) {
			writeClientError(w, http.StatusUnauthorized, "invalid username or password")
		} else {
			writeSignInStatusUnavailable(w)
		}
		return
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !ok {
		writeClientError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	// Second factor: if the account has 2FA on, the password alone does not
	// grant a session. Hand back a short-lived pending token; the session is
	// issued by /auth/login/totp once a valid code arrives.
	// İkinci faktör: hesabın 2FA'sı açıksa parola tek başına oturum vermez.
	// Kısa ömürlü bir bekleme jetonu döndür; oturum, geçerli bir kod gelince
	// /auth/login/totp tarafından verilir.
	state, err := p.userAuthState(r.Context(), user.ID)
	if err != nil {
		writeSignInStateError(w, err)
		return
	}
	if state.passwordHash != user.PasswordHash {
		writeClientError(w, http.StatusUnauthorized, "sign-in expired, start again")
		return
	}
	if state.status == "suspended" {
		writeClientError(w, http.StatusForbidden, "account suspended")
		return
	}
	identity, err := p.canonicalAuthIdentity(r.Context(), user.ID)
	if err != nil {
		writeSignInStateError(w, err)
		return
	}
	if !state.matchesCanonical(identity) {
		writeClientError(w, http.StatusUnauthorized, "sign-in expired, start again")
		return
	}
	// alpha.128 (G4): administrators only from the listed addresses.
	if p.refuseAdministratorFromUnlistedAddress(w, r, user.ID) {
		return
	}
	if state.totpEnabled {
		pendingToken, err := newPendingToken(state)
		if err != nil {
			writeServerError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totp_required": true,
			"pending_token": pendingToken,
		})
		return
	}

	if p.sessions == nil {
		writeSignInStatusUnavailable(w)
		return
	}
	token, err := p.sessions.CreateForAuthEpoch(r.Context(), user.ID, state.authEpoch, false)
	if err != nil {
		writeSignInStateError(w, err)
		return
	}
	p.describeNewSession(r, token)

	p.setSessionCookie(w, r, token, time.Now().Add(auth.SessionDuration))

	// Record the sign-in against the user (the session is fresh, so the
	// caller-based audit cannot resolve them yet).
	// Girişi kullanıcıya kaydet (oturum yeni; çağıran-tabanlı denetim onu
	// henüz çözemez).
	p.auditAs(r, user.ID, "auth.login", "", 0)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p.identityResponse(r.Context(), identity, false))
}

// handleLogout deletes the current session and clears the cookie.
// handleLogout, mevcut oturumu siler ve çerezi temizler.
func (p *Panel) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var deleteErr error
	// The token requireAuth admitted; the cookie is not read a second time.
	if token := currentSessionToken(r); token != "" {
		deleteErr = p.sessions.Delete(r.Context(), token)
	}
	// Signing out of a "Log in as" session also ends the operator's own
	// session kept in the return cookie, once it is proved to be that
	// operator's: no usable token of the operator stays in this browser for
	// the next person who signs in on it.
	// "Farklı giriş yap" içinden çıkış, dönüş çerezindeki operatör oturumunu da
	// (operatörün olduğu doğrulanınca) sonlandırır.
	if c := currentCaller(r); deleteErr == nil && c != nil && c.ImpersonatorID > 0 {
		deleteErr = p.endKeptOperatorSession(r, c.ImpersonatorID)
	}
	// An already-expired cookie tells the browser to drop it, under every
	// name the session cookie and the "Log in as" return cookie had.
	// Süresi geçmiş bir çerez, tarayıcıya onu bırakmasını söyler.
	p.clearSessionCookie(w)
	p.clearImpersonatorCookie(w)
	if deleteErr != nil {
		// The browser token is still cleared, but do not claim a complete
		// logout when the server-side credential could not be revoked.
		// Tarayıcı jetonu yine temizlenir; fakat sunucu tarafındaki kimlik
		// bilgisi iptal edilemediyse çıkış tamamlandı diye davranma.
		writeServerError(w, deleteErr)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// endKeptOperatorSession ends the session the "Log in as" return cookie holds
// when it is the given operator's own session. A cookie naming anything else
// is not ended (it is only removed from the browser by the caller): it is not
// this operator's to end. A return cookie that is absent or no longer valid
// needs nothing.
func (p *Panel) endKeptOperatorSession(r *http.Request, operatorID int) error {
	kept, err := p.impersonatorCookieValue(r)
	if err != nil {
		return nil
	}
	original, err := p.sessions.ValidateIdentity(r.Context(), kept)
	if errors.Is(err, auth.ErrSessionInvalid) {
		return nil
	}
	if err != nil {
		return err
	}
	if original.UserID != operatorID || original.ImpersonatorID != 0 {
		return nil
	}
	return p.sessions.Delete(r.Context(), kept)
}

// handleMe returns the current user, used by the SPA to decide whether to
// show the login screen.
// handleMe, mevcut kullanıcıyı döndürür; SPA'nın giriş ekranını gösterip
// göstermeyeceğine karar vermek için kullanılır.
func (p *Panel) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	caller := currentCaller(r)
	if caller == nil || !caller.validAuthorizationIdentity() {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	identity, err := p.canonicalAuthIdentity(r.Context(), caller.ID)
	if err != nil {
		if errors.Is(err, errInvalidCanonicalAuthIdentity) {
			writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		} else {
			writeAuthStatusUnavailable(w)
		}
		return
	}
	if identity.identity.UserID != caller.ID || identity.identity.Role != caller.Role ||
		identity.accountType != caller.normalizedAccountType() ||
		identity.identity.CustomerID != caller.CustomerID {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	// Tell the SPA when this session is an impersonation so it can show the
	// "return to my account" banner. Only the session itself says so: a
	// return cookie beside an ordinary session is left over from someone
	// else's "Log in as" and is removed, never offered as a way back.
	// Bu oturum bir taklit oturumuysa SPA'ya söyle; "hesabıma dön" şeridini
	// gösterebilsin. Bunu yalnız oturumun kendisi söyler.
	if caller.ImpersonatorID == 0 {
		p.expireImpersonatorCookieSent(w, r)
	}
	response := p.identityResponse(r.Context(), identity, caller.ImpersonatorID > 0)
	// The banner names the real person too (alpha.114): "Viewing as X —
	// signed in as Y". A failed read leaves only the account's own name.
	// Şerit gerçek kişiyi de adlandırır; okunamazsa yalnız hesabın adı kalır.
	if caller.ImpersonatorID > 0 {
		response.Impersonating = true
		if operator, err := p.users.GetByID(r.Context(), caller.ImpersonatorID); err == nil && operator != nil {
			response.Impersonator = operator.Username
		}
	}

	suspension, err := p.accountServiceSuspension(r.Context(), caller)
	if err != nil {
		writeAuthStatusUnavailable(w)
		return
	}
	response.Suspended = suspension

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// dummyHash is a valid argon2id hash of a random password nobody knows.
// Verifying against it when the username does not exist keeps login timing
// uniform, so the endpoint does not leak account existence via response
// time. It is computed once at startup.
//
// dummyHash, kimsenin bilmediği rastgele bir parolanın geçerli bir
// argon2id özetidir. Kullanıcı adı yokken buna karşı doğrulama, giriş
// zamanlamasını eşit tutar; böylece uç nokta yanıt süresiyle hesabın
// varlığını sızdırmaz. Başlangıçta bir kez hesaplanır.
var dummyHash = mustDummyHash()

func mustDummyHash() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		panic("failed to generate dummy password: " + err.Error())
	}
	h, err := auth.HashPassword(hex.EncodeToString(raw))
	if err != nil {
		panic("failed to compute dummy password hash: " + err.Error())
	}
	return h
}

// A pending TOTP token is still single-use after an unavailable backend result.
// Arka uç okunamasa da TOTP bekleme jetonu tek kullanımlık kalır.
func writeSignInStatusUnavailable(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeCodedError(w, http.StatusServiceUnavailable, errCodeAuthStatusUnavailable,
		"Sign-in could not be completed. Check again, then start sign-in again.", "")
}

func writeSignInStateError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrAuthStateChanged) || errors.Is(err, errInvalidCanonicalAuthIdentity) ||
		errors.Is(err, repositories.ErrUserNotFound) || errors.Is(err, sql.ErrNoRows) {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "sign-in expired, start again", "")
		return
	}
	writeSignInStatusUnavailable(w)
}
