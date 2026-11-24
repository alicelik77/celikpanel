package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/repositories"
)

// Security lane A1 (24 November 2026): the sign-in cookies' names, the
// duplicate refusal, the fetch-metadata gate, the transition from the plain
// names and the database tools' proxy stripping.

const (
	signinAdminID     = 9711
	signinAdmin2ID    = 9712
	signinCustomerID  = 9713
	signinTestMeRoute = "/api/v1/auth/me"
)

type signinFixture struct {
	panel   *Panel
	handler http.Handler
	tokens  map[int]string
	// seen is what the handler behind requireAuth saw last.
	seenCaller int
	seenToken  string
}

func newSigninFixture(t *testing.T, since time.Time) *signinFixture {
	t.Helper()
	database, err := openTemplatePanelDatabase(filepath.Join(t.TempDir(), "panel.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(database.Close)
	sqlDB := database.GetDB()
	if _, err := sqlDB.Exec(`
		INSERT INTO users (id, username, password_hash, email, role, parent_id, status)
		VALUES
			(9711, 'signin-admin', 'x', 'signin-admin@example.test', 'admin', NULL, 'active'),
			(9712, 'signin-admin2', 'x', 'signin-admin2@example.test', 'admin', NULL, 'active'),
			(9713, 'signin-customer', 'x', 'signin-customer@example.test', 'customer', NULL, 'active');
		INSERT INTO subscriptions (id, owner_id, name, status) VALUES (9811, 9713, 'signin subscription', 'active');
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	sessions := auth.NewSessionStore(sqlDB)
	fixture := &signinFixture{tokens: map[int]string{}}
	fixture.panel = &Panel{
		db:                     database,
		license:                testPanelLicense(t, "active"),
		sessions:               sessions,
		users:                  repositories.NewPostgresUserRepository(sqlDB),
		secureCookies:          true,
		signinCookieNamesSince: since,
	}
	for _, id := range []int{signinAdminID, signinAdmin2ID, signinCustomerID} {
		token, err := sessions.Create(context.Background(), id)
		if err != nil {
			t.Fatalf("session %d: %v", id, err)
		}
		fixture.tokens[id] = token
	}
	pass := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture.seenCaller, fixture.seenToken = 0, currentSessionToken(r)
		if caller := currentCaller(r); caller != nil {
			fixture.seenCaller = caller.ID
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux := http.NewServeMux()
	for _, path := range []string{signinTestMeRoute, "/api/v1/auth/login", "/dbtool/phpmyadmin/"} {
		mux.Handle(path, pass)
	}
	fixture.handler = fixture.panel.requireAuth(mux)
	return fixture
}

// openedAt moves a session's created_at, as a release before this one would
// have left it.
func (f *signinFixture) openedAt(t *testing.T, userID int, at time.Time) {
	t.Helper()
	sum := sha256.Sum256([]byte(f.tokens[userID]))
	if _, err := f.panel.db.GetDB().Exec(`UPDATE sessions SET created_at = ? WHERE token_hash = ?`,
		at.UTC().Format("2006-01-02 15:04:05"), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("move created_at: %v", err)
	}
}

func (f *signinFixture) do(t *testing.T, method, path string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	f.seenCaller, f.seenToken = -1, ""
	f.handler.ServeHTTP(recorder, request)
	return recorder
}

func cookieNamed(recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			found = cookie
		}
	}
	return found
}

func signinErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

// hostPrefixAccepted is what a browser requires of a cookie whose name starts
// with __Host- before it stores it (RFC 6265bis "__Host-" prefix): Secure,
// Path=/ and no Domain. A cookie another host sets for the parent domain
// always carries a Domain, so it can never be stored under such a name.
func hostPrefixAccepted(cookie *http.Cookie) bool {
	return cookie.Secure && cookie.Path == "/" && cookie.Domain == ""
}

func TestSigninCookiesUseHostPrefixOverHTTPS(t *testing.T) {
	panel := &Panel{secureCookies: true}
	recorder := httptest.NewRecorder()
	panel.setSessionCookie(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil), "token-1", time.Now().Add(time.Hour))
	panel.setImpersonatorCookie(recorder, "token-0")
	session := cookieNamed(recorder, hostSessionCookieName)
	impersonator := cookieNamed(recorder, hostImpersonatorCookieName)
	if session == nil || impersonator == nil {
		t.Fatalf("cookies = %v, want both __Host- names", recorder.Result().Cookies())
	}
	for _, cookie := range []*http.Cookie{session, impersonator} {
		if !hostPrefixAccepted(cookie) || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("%s attributes = Secure:%v Path:%q Domain:%q HttpOnly:%v SameSite:%v", cookie.Name, cookie.Secure, cookie.Path, cookie.Domain, cookie.HttpOnly, cookie.SameSite)
		}
	}
	// What the panel itself sends, header by header: the attributes a
	// browser requires before it keeps a __Host- cookie (Secure, Path=/, no
	// Domain). A cookie another host sets for the parent domain must carry a
	// Domain, so a browser never keeps one under these names.
	headers := recorder.Header().Values("Set-Cookie")
	for _, prefix := range []string{hostSessionCookieName + "=token-1;", hostImpersonatorCookieName + "=token-0;"} {
		var header string
		for _, value := range headers {
			if strings.HasPrefix(value, prefix) {
				header = value
			}
		}
		attributes := map[string]bool{}
		for _, part := range strings.Split(header, ";")[1:] {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			attributes[strings.ToLower(name)] = true
		}
		if header == "" || !attributes["secure"] || !attributes["httponly"] || attributes["domain"] ||
			!strings.Contains(header, "; Path=/;") || !strings.Contains(header, "; SameSite=Lax") {
			t.Fatalf("Set-Cookie %q (all: %q): want Secure, HttpOnly, Path=/, SameSite=Lax and no Domain", header, headers)
		}
	}

	plain := &Panel{secureCookies: false}
	recorder = httptest.NewRecorder()
	plain.setSessionCookie(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil), "token-1", time.Now().Add(time.Hour))
	if cookieNamed(recorder, sessionCookieName) == nil || len(recorder.Result().Cookies()) != 1 {
		t.Fatalf("development over HTTP keeps the plain name: %v", recorder.Result().Cookies())
	}
}

func TestSigninDuplicateSessionCookiesAreRefusedAndCleared(t *testing.T) {
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	admin, admin2 := fixture.tokens[signinAdminID], fixture.tokens[signinAdmin2ID]
	for name, cookies := range map[string][]*http.Cookie{
		"new name twice": {{Name: hostSessionCookieName, Value: admin}, {Name: hostSessionCookieName, Value: admin2}},
		"plain twice":    {{Name: sessionCookieName, Value: admin}, {Name: sessionCookieName, Value: admin2}},
		"log in as twice": {{Name: hostSessionCookieName, Value: admin},
			{Name: hostImpersonatorCookieName, Value: admin}, {Name: hostImpersonatorCookieName, Value: admin2}},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, cookies, nil)
			if recorder.Code != http.StatusUnauthorized || signinErrorCode(t, recorder) != errCodeSigninCookiesDuplicate {
				t.Fatalf("status = %d code = %q body = %s", recorder.Code, signinErrorCode(t, recorder), recorder.Body.String())
			}
			if fixture.seenCaller != -1 {
				t.Fatal("the handler was reached")
			}
			for _, name := range []string{hostSessionCookieName, sessionCookieName, hostImpersonatorCookieName, impersonatorCookieName} {
				cleared := cookieNamed(recorder, name)
				if cleared == nil || cleared.Value != "" || (cleared.MaxAge >= 0 && !cleared.Expires.Before(time.Now())) {
					t.Fatalf("%s not cleared: %+v", name, cleared)
				}
			}
			if !strings.Contains(recorder.Body.String(), "Sign in again") {
				t.Fatalf("body does not say what to do: %s", recorder.Body.String())
			}
		})
	}

	t.Run("plain name twice over HTTP", func(t *testing.T) {
		fixture.panel.secureCookies = false
		defer func() { fixture.panel.secureCookies = true }()
		recorder := fixture.do(t, http.MethodGet, signinTestMeRoute,
			[]*http.Cookie{{Name: sessionCookieName, Value: admin}, {Name: sessionCookieName, Value: admin2}}, nil)
		if recorder.Code != http.StatusUnauthorized || signinErrorCode(t, recorder) != errCodeSigninCookiesDuplicate {
			t.Fatalf("status = %d code = %q", recorder.Code, signinErrorCode(t, recorder))
		}
	})
}

func TestSigninFetchMetadataGate(t *testing.T) {
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	signedIn := []*http.Cookie{{Name: hostSessionCookieName, Value: fixture.tokens[signinAdminID]}}
	for _, site := range []string{"same-site", "cross-site", "Cross-Site"} {
		recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, signedIn, map[string]string{"Sec-Fetch-Site": site})
		if recorder.Code != http.StatusForbidden || signinErrorCode(t, recorder) != errCodeCrossSiteRequest || fixture.seenCaller != -1 {
			t.Fatalf("Sec-Fetch-Site %s: status = %d code = %q", site, recorder.Code, signinErrorCode(t, recorder))
		}
	}
	for _, site := range []string{"same-origin", "none", ""} {
		headers := map[string]string{}
		if site != "" {
			headers["Sec-Fetch-Site"] = site
		}
		recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, signedIn, headers)
		if recorder.Code != http.StatusNoContent || fixture.seenCaller != signinAdminID {
			t.Fatalf("Sec-Fetch-Site %q: status = %d body = %s", site, recorder.Code, recorder.Body.String())
		}
	}
	// Exempt by position: a public path, a path outside /api/, and a request
	// without a sign-in cookie never meet the gate.
	if recorder := fixture.do(t, http.MethodPost, "/api/v1/auth/login", signedIn, map[string]string{"Sec-Fetch-Site": "cross-site"}); recorder.Code != http.StatusNoContent {
		t.Fatalf("public sign-in path: status = %d", recorder.Code)
	}
	if recorder := fixture.do(t, http.MethodGet, "/dbtool/phpmyadmin/", signedIn, map[string]string{"Sec-Fetch-Site": "same-site"}); recorder.Code != http.StatusNoContent {
		t.Fatalf("non-API path: status = %d", recorder.Code)
	}
	if recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); recorder.Code != http.StatusUnauthorized || signinErrorCode(t, recorder) == errCodeCrossSiteRequest {
		t.Fatalf("no cookie: status = %d code = %q", recorder.Code, signinErrorCode(t, recorder))
	}
	// An API token request is decided by the token layer before the gate.
	recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, nil, map[string]string{
		"Sec-Fetch-Site": "cross-site", "Authorization": "Bearer cpk_notreal",
	})
	if signinErrorCode(t, recorder) == errCodeCrossSiteRequest {
		t.Fatal("an API token request met the cookie gate")
	}
}

func TestCSRFBlocksOriginThatDiffersOnlyByPort(t *testing.T) {
	for _, testCase := range []struct {
		origin string
		want   int
	}{
		{"https://panel.example.com:1924", http.StatusForbidden},
		{"https://panel.example.com", http.StatusForbidden},
		{"https://panel.example.com:1923", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/domains/create", nil)
		request.Host = "panel.example.com:1923"
		request.Header.Set("Origin", testCase.origin)
		recorder := httptest.NewRecorder()
		csrfProtect(okHandler()).ServeHTTP(recorder, request)
		if recorder.Code != testCase.want {
			t.Fatalf("Origin %s: got %d, want %d", testCase.origin, recorder.Code, testCase.want)
		}
	}
}

func TestSigninTransitionCarriesAPreUpdateAdministratorSessionOnce(t *testing.T) {
	since := time.Now().Add(-10 * time.Minute)
	fixture := newSigninFixture(t, since)
	fixture.openedAt(t, signinAdminID, since.Add(-2*time.Hour))
	token := fixture.tokens[signinAdminID]

	recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, []*http.Cookie{{Name: sessionCookieName, Value: token}}, nil)
	if recorder.Code != http.StatusNoContent || fixture.seenCaller != signinAdminID || fixture.seenToken != token {
		t.Fatalf("pre-update administrator session: status = %d caller = %d body = %s", recorder.Code, fixture.seenCaller, recorder.Body.String())
	}
	carried := cookieNamed(recorder, hostSessionCookieName)
	if carried == nil || carried.Value != token || !hostPrefixAccepted(carried) || !carried.HttpOnly {
		t.Fatalf("not carried over under the new name: %+v", carried)
	}
	// The carried cookie ends when the session's stored expiry does (the
	// fixture created it a moment ago; only created_at was moved).
	if wantEnd := time.Now().Add(auth.SessionDuration); carried.Expires.After(wantEnd.Add(time.Second)) || carried.Expires.Before(wantEnd.Add(-time.Minute)) {
		t.Fatalf("carried cookie expiry %v, want the session's own end (about %v)", carried.Expires, wantEnd)
	}
	if plain := cookieNamed(recorder, sessionCookieName); plain == nil || plain.MaxAge >= 0 || plain.Path != "/" {
		t.Fatalf("plain name not removed: %+v", plain)
	}
	// The browser now sends the new name: admitted with no further change.
	recorder = fixture.do(t, http.MethodGet, signinTestMeRoute, []*http.Cookie{carried}, nil)
	if recorder.Code != http.StatusNoContent || len(recorder.Result().Cookies()) != 0 {
		t.Fatalf("after the carry-over: status = %d cookies = %v", recorder.Code, recorder.Result().Cookies())
	}
}

// Review of PR #368, finding 5: the carry-over's Set-Cookie holds the session
// token, so it happens only on an /api/ answer marked no-store, never on a
// tool's cacheable response.
func TestSigninTransitionCarriesOverOnlyOnAPIAndNeverCached(t *testing.T) {
	since := time.Now().Add(-10 * time.Minute)
	fixture := newSigninFixture(t, since)
	fixture.openedAt(t, signinAdminID, since.Add(-2*time.Hour))
	token := fixture.tokens[signinAdminID]
	reached := false
	mux := http.NewServeMux()
	mux.HandleFunc("/dbtool/phpmyadmin/app.js", func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Header().Set("Cache-Control", "public, max-age=31536000")
		_, _ = io.WriteString(w, "tool script")
	})
	mux.HandleFunc(signinTestMeRoute, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := fixture.panel.requireAuth(mux)
	send := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	tool := send("/dbtool/phpmyadmin/app.js")
	if tool.Code != http.StatusUnauthorized || reached || tool.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("a tool's response under the plain name: status %d, reached %v, Cache-Control %q", tool.Code, reached, tool.Header().Get("Cache-Control"))
	}
	if cookies := tool.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("a tool's response changed the sign-in cookies: %v", cookies)
	}
	// The plain cookie was left in place: the next /api/ call carries it over.
	api := send(signinTestMeRoute)
	carried := cookieNamed(api, hostSessionCookieName)
	if api.Code != http.StatusNoContent || carried == nil || carried.Value != token {
		t.Fatalf("the next /api/ call: status %d, carried %+v", api.Code, carried)
	}
	if value := api.Header().Get("Cache-Control"); value != "no-store" {
		t.Fatalf("the carry-over answer has Cache-Control %q", value)
	}
}

func TestSigninTransitionRefusals(t *testing.T) {
	now := time.Now()
	for _, testCase := range []struct {
		name    string
		since   time.Time
		user    int
		opened  time.Duration // relative to since
		cookies func(f *signinFixture) []*http.Cookie
	}{
		{name: "session opened after the new names", since: now.Add(-time.Hour), user: signinAdminID, opened: time.Minute},
		{name: "transition over", since: now.Add(-auth.SessionDuration - time.Minute), user: signinAdminID, opened: -time.Hour},
		{name: "first start not recorded", since: time.Time{}, user: signinAdminID, opened: 0},
		{name: "not an administrator", since: now.Add(-time.Hour), user: signinCustomerID, opened: -time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newSigninFixture(t, testCase.since)
			base := testCase.since
			if base.IsZero() {
				base = now.Add(-time.Hour)
			}
			fixture.openedAt(t, testCase.user, base.Add(testCase.opened))
			recorder := fixture.do(t, http.MethodGet, signinTestMeRoute,
				[]*http.Cookie{{Name: sessionCookieName, Value: fixture.tokens[testCase.user]}}, nil)
			if recorder.Code != http.StatusUnauthorized || signinErrorCode(t, recorder) != errCodeSigninFromBeforeUpdate || fixture.seenCaller != -1 {
				t.Fatalf("status = %d code = %q body = %s", recorder.Code, signinErrorCode(t, recorder), recorder.Body.String())
			}
			if cookieNamed(recorder, hostSessionCookieName) != nil {
				t.Fatal("a refused session was given the new name")
			}
			if plain := cookieNamed(recorder, sessionCookieName); plain == nil || plain.MaxAge >= 0 {
				t.Fatalf("plain name not removed: %+v", plain)
			}
		})
	}

	t.Run("log in as session", func(t *testing.T) {
		since := now.Add(-time.Hour)
		fixture := newSigninFixture(t, since)
		token, err := fixture.panel.sessions.CreateImpersonation(context.Background(), signinAdmin2ID, signinAdminID)
		if err != nil {
			t.Fatal(err)
		}
		fixture.tokens[0] = token
		fixture.openedAt(t, 0, since.Add(-time.Hour))
		recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, []*http.Cookie{{Name: sessionCookieName, Value: token}}, nil)
		if recorder.Code != http.StatusUnauthorized || fixture.seenCaller != -1 {
			t.Fatalf("a pre-update Log in as session was carried: %d", recorder.Code)
		}
		// Refused by the transition itself (its message, the plain name
		// removed), not only by the later "Log in as" checks, which refuse
		// administrator → administrator anyway.
		if signinErrorCode(t, recorder) != errCodeSigninFromBeforeUpdate || !strings.Contains(recorder.Body.String(), legacySigninRefusedMessage) {
			t.Fatalf("not the transition's refusal: %s", recorder.Body.String())
		}
		if plain := cookieNamed(recorder, sessionCookieName); plain == nil || plain.MaxAge >= 0 {
			t.Fatalf("plain name not removed: %+v", plain)
		}
	})

	// The rule alone: the same pre-update administrator session is carried
	// as itself and refused as a "Log in as" session.
	panel := &Panel{secureCookies: true, signinCookieNamesSince: now.Add(-time.Hour)}
	session := auth.SessionIdentity{UserID: signinAdminID, CreatedAt: now.Add(-2 * time.Hour)}
	if !panel.legacySigninAccepted(session, now) {
		t.Fatal("a pre-update administrator session is not carried")
	}
	session.ImpersonatorID = signinAdmin2ID
	if panel.legacySigninAccepted(session, now) {
		t.Fatal("a pre-update Log in as session is carried")
	}
	var absent *Panel
	if absent.legacySigninAccepted(session, now) {
		t.Fatal("no panel carries nothing")
	}
}

func TestSigninPlainCookieBesideTheNewNameIsIgnoredAndCleared(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	fixture := newSigninFixture(t, since)
	// The planted cookie names another administrator's pre-update session,
	// the strongest case: it would be carried over if it were read at all.
	fixture.openedAt(t, signinAdmin2ID, since.Add(-time.Hour))
	recorder := fixture.do(t, http.MethodGet, signinTestMeRoute, []*http.Cookie{
		{Name: sessionCookieName, Value: fixture.tokens[signinAdmin2ID]},
		{Name: hostSessionCookieName, Value: fixture.tokens[signinAdminID]},
	}, nil)
	if recorder.Code != http.StatusNoContent || fixture.seenCaller != signinAdminID {
		t.Fatalf("status = %d caller = %d", recorder.Code, fixture.seenCaller)
	}
	if plain := cookieNamed(recorder, sessionCookieName); plain == nil || plain.MaxAge >= 0 {
		t.Fatalf("plain cookie not removed: %+v", plain)
	}
	if cookieNamed(recorder, hostSessionCookieName) != nil {
		t.Fatal("the session cookie was rewritten")
	}
}

func TestSigninCookieNamesSinceIsRecordedOnceAndKeptAcrossRestarts(t *testing.T) {
	fixture := newSigninFixture(t, time.Time{})
	fixture.panel.loadSigninCookieNamesSince(context.Background())
	first := fixture.panel.signinCookieNamesSince
	if first.IsZero() || time.Since(first) > time.Minute {
		t.Fatalf("first start = %v", first)
	}
	earlier := first.Add(-3 * time.Hour).Format(time.RFC3339)
	if _, err := fixture.panel.db.GetDB().Exec(`UPDATE panel_settings SET value = ? WHERE key = ?`, earlier, settingSigninCookieNamesSince); err != nil {
		t.Fatal(err)
	}
	restarted := &Panel{db: fixture.panel.db, secureCookies: true}
	restarted.loadSigninCookieNamesSince(context.Background())
	if got := restarted.signinCookieNamesSince.Format(time.RFC3339); got != earlier {
		t.Fatalf("a restart moved the first start: %s, want %s", got, earlier)
	}
	insecure := &Panel{db: fixture.panel.db}
	insecure.loadSigninCookieNamesSince(context.Background())
	if !insecure.signinCookieNamesSince.IsZero() {
		t.Fatal("development over HTTP has no transition")
	}
}

func TestSigninLogInAsAndReturnUseTheNewNames(t *testing.T) {
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	operator := fixture.tokens[signinAdminID]
	impersonated, err := fixture.panel.sessions.CreateImpersonation(context.Background(), signinCustomerID, signinAdminID)
	if err != nil {
		t.Fatal(err)
	}
	// The way back: the impersonated session and the operator's own token
	// under the __Host- names, through requireAuth as in production.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/unimpersonate", fixture.panel.handleUnimpersonate)
	handler := fixture.panel.requireAuth(mux)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/unimpersonate", nil)
	request.AddCookie(&http.Cookie{Name: hostSessionCookieName, Value: impersonated})
	request.AddCookie(&http.Cookie{Name: hostImpersonatorCookieName, Value: operator})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("return: status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	restored := cookieNamed(recorder, hostSessionCookieName)
	if restored == nil || restored.Value != operator || !hostPrefixAccepted(restored) {
		t.Fatalf("operator session not restored under the new name: %+v", restored)
	}
	if marker := cookieNamed(recorder, hostImpersonatorCookieName); marker == nil || marker.Value != "" || marker.MaxAge >= 0 {
		t.Fatalf("return marker not cleared: %+v", marker)
	}
	if _, err := fixture.panel.sessions.Validate(context.Background(), impersonated); err != auth.ErrSessionInvalid {
		t.Fatalf("impersonated session survives the return: %v", err)
	}

	// The plain "Log in as" name is never read over HTTPS.
	impersonated, err = fixture.panel.sessions.CreateImpersonation(context.Background(), signinCustomerID, signinAdminID)
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/unimpersonate", nil)
	request.AddCookie(&http.Cookie{Name: hostSessionCookieName, Value: impersonated})
	request.AddCookie(&http.Cookie{Name: impersonatorCookieName, Value: operator})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("plain Log in as cookie was read: status = %d", recorder.Code)
	}
}

// signinBrowser is one browser against the real sign-in handlers behind
// requireAuth over HTTPS: its cookie jar keeps what the panel sets.
type signinBrowser struct {
	t      *testing.T
	server *httptest.Server
	client *http.Client
}

func newSigninBrowser(t *testing.T, fixture *signinFixture) *signinBrowser {
	t.Helper()
	panel := fixture.panel
	panel.loginLimiter = newRateLimiter(100, time.Minute)
	for _, id := range []int{signinAdminID, signinCustomerID} {
		hash, err := auth.HashPassword(signinTestPassword)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := panel.db.GetDB().Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", panel.handleLogin)
	mux.HandleFunc("/api/v1/auth/logout", panel.handleLogout)
	mux.HandleFunc("/api/v1/auth/me", panel.handleMe)
	mux.HandleFunc("/api/v1/auth/unimpersonate", panel.handleUnimpersonate)
	// "Log in as" the customer, through the real handler (the route's own
	// permission check is not what is tested here).
	mux.HandleFunc("/api/v1/test/log-in-as-customer", func(w http.ResponseWriter, r *http.Request) {
		target, err := panel.users.GetByID(r.Context(), signinCustomerID)
		if err != nil {
			t.Errorf("read the customer: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		panel.handleImpersonate(w, r, currentCaller(r), target)
	})
	server := httptest.NewTLSServer(panel.requireAuth(mux))
	t.Cleanup(server.Close)
	client := server.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	return &signinBrowser{t: t, server: server, client: client}
}

const signinTestPassword = "Sign-in cookies test password 368"

func (b *signinBrowser) send(method, path, body string) (*http.Response, string) {
	b.t.Helper()
	request, err := http.NewRequest(method, b.server.URL+path, strings.NewReader(body))
	if err != nil {
		b.t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		b.t.Fatal(err)
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(response.Body)
	return response, string(text)
}

func (b *signinBrowser) signIn(username string) {
	b.t.Helper()
	response, body := b.send(http.MethodPost, "/api/v1/auth/login", `{"username":"`+username+`","password":"`+signinTestPassword+`"}`)
	if response.StatusCode != http.StatusOK {
		b.t.Fatalf("sign in as %s: %d %s", username, response.StatusCode, body)
	}
}

// cookie is the value the browser keeps under name ("" when it keeps none).
func (b *signinBrowser) cookie(name string) string {
	target, _ := url.Parse(b.server.URL)
	for _, cookie := range b.client.Jar.Cookies(target) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func (b *signinBrowser) plant(name, value string) {
	target, _ := url.Parse(b.server.URL)
	b.client.Jar.SetCookies(target, []*http.Cookie{{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true}})
}

func (b *signinBrowser) me() (int, authMeResponse) {
	b.t.Helper()
	response, body := b.send(http.MethodGet, signinTestMeRoute, "")
	var identity authMeResponse
	_ = json.Unmarshal([]byte(body), &identity)
	return response.StatusCode, identity
}

// Review of PR #368, finding 1 (older than the PR): after "Log in as", "Sign
// out" left the operator's own session in the return cookie, and the next
// person who signed in on that browser and pressed "Return to my account"
// became the operator.
func TestSigninLogInAsThenSignOutLeavesNoWayToTheOperator(t *testing.T) {
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	browser := newSigninBrowser(t, fixture)
	sessions := fixture.panel.sessions

	// The administrator signs in and uses "Log in as" on the customer.
	browser.signIn("signin-admin")
	operator := browser.cookie(hostSessionCookieName)
	if response, body := browser.send(http.MethodPost, "/api/v1/test/log-in-as-customer", ""); response.StatusCode != http.StatusOK {
		t.Fatalf("log in as: %d %s", response.StatusCode, body)
	}
	impersonated := browser.cookie(hostSessionCookieName)
	if operator == "" || browser.cookie(hostImpersonatorCookieName) != operator || impersonated == operator {
		t.Fatal("the browser is not in Log in as with the operator's session kept")
	}
	if status, identity := browser.me(); status != http.StatusOK || identity.Username != "signin-customer" || !identity.Impersonating {
		t.Fatalf("/me in Log in as: %d %+v", status, identity)
	}

	// "Sign out" (the SPA calls only /auth/logout): no usable token is left,
	// in the browser or on the server.
	if response, body := browser.send(http.MethodPost, "/api/v1/auth/logout", ""); response.StatusCode != http.StatusNoContent {
		t.Fatalf("sign out: %d %s", response.StatusCode, body)
	}
	if browser.cookie(hostImpersonatorCookieName) != "" || browser.cookie(hostSessionCookieName) != "" {
		t.Fatal("sign out left a sign-in cookie in the browser")
	}
	for name, token := range map[string]string{"the operator's kept session": operator, "the Log in as session": impersonated} {
		if _, err := sessions.Validate(context.Background(), token); err != auth.ErrSessionInvalid {
			t.Fatalf("%s is still valid after sign out: %v", name, err)
		}
	}

	// The next person signs in on that browser: an ordinary session, no way back.
	browser.signIn("signin-customer")
	if status, identity := browser.me(); status != http.StatusOK || identity.Username != "signin-customer" || identity.Impersonating {
		t.Fatalf("/me after the next sign-in: %d %+v", status, identity)
	}
	response, _ := browser.send(http.MethodPost, "/api/v1/auth/unimpersonate", "")
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("return after someone else's sign-out: status %d", response.StatusCode)
	}
	if status, identity := browser.me(); status != http.StatusOK || identity.Username != "signin-customer" {
		t.Fatalf("after the refused return: %d %+v", status, identity)
	}
}

// The same browser with a return cookie still holding a valid operator
// session beside an ordinary one (a browser from before this fix, or a
// sign-out that never reached the panel): every step refuses to use it.
func TestSigninReturnCookieBesideAnOrdinarySessionLeadsNowhere(t *testing.T) {
	fixture := newSigninFixture(t, time.Now().Add(-time.Hour))
	browser := newSigninBrowser(t, fixture)
	operator := fixture.tokens[signinAdminID]

	// Signing in removes a return cookie the browser still holds.
	browser.plant(hostImpersonatorCookieName, operator)
	browser.signIn("signin-customer")
	if browser.cookie(hostImpersonatorCookieName) != "" {
		t.Fatal("sign-in kept a return cookie")
	}

	// /me neither offers it as a way back nor keeps it.
	browser.plant(hostImpersonatorCookieName, operator)
	if status, identity := browser.me(); status != http.StatusOK || identity.Impersonating {
		t.Fatalf("/me with a left-over return cookie: %d %+v", status, identity)
	}
	if browser.cookie(hostImpersonatorCookieName) != "" {
		t.Fatal("/me kept a left-over return cookie")
	}

	// "Return" refuses it, removes it and keeps the person's own session.
	customer := browser.cookie(hostSessionCookieName)
	browser.plant(hostImpersonatorCookieName, operator)
	response, _ := browser.send(http.MethodPost, "/api/v1/auth/unimpersonate", "")
	if response.StatusCode != http.StatusBadRequest || browser.cookie(hostImpersonatorCookieName) != "" ||
		browser.cookie(hostSessionCookieName) != customer || customer == operator {
		t.Fatalf("return beside an ordinary session: status %d", response.StatusCode)
	}
	if _, err := fixture.panel.sessions.Validate(context.Background(), customer); err != nil {
		t.Fatalf("the person's own session was ended: %v", err)
	}

	// Inside a real Log in as, a return cookie naming another person's
	// session is refused too, and that Log in as session ends.
	impersonated, err := fixture.panel.sessions.CreateImpersonation(context.Background(), signinCustomerID, signinAdminID)
	if err != nil {
		t.Fatal(err)
	}
	browser.plant(hostSessionCookieName, impersonated)
	browser.plant(hostImpersonatorCookieName, fixture.tokens[signinAdmin2ID])
	response, _ = browser.send(http.MethodPost, "/api/v1/auth/unimpersonate", "")
	if response.StatusCode != http.StatusUnauthorized || browser.cookie(hostSessionCookieName) != "" || browser.cookie(hostImpersonatorCookieName) != "" {
		t.Fatalf("return to another person's session: status %d", response.StatusCode)
	}

	// The ordinary way still works: Log in as, then Return.
	browser.signIn("signin-admin")
	own := browser.cookie(hostSessionCookieName)
	if response, body := browser.send(http.MethodPost, "/api/v1/test/log-in-as-customer", ""); response.StatusCode != http.StatusOK {
		t.Fatalf("log in as: %d %s", response.StatusCode, body)
	}
	if response, body := browser.send(http.MethodPost, "/api/v1/auth/unimpersonate", ""); response.StatusCode != http.StatusOK {
		t.Fatalf("return: %d %s", response.StatusCode, body)
	}
	if browser.cookie(hostSessionCookieName) != own || browser.cookie(hostImpersonatorCookieName) != "" {
		t.Fatal("return did not restore the operator's own session")
	}
	if status, identity := browser.me(); status != http.StatusOK || identity.Username != "signin-admin" || identity.Impersonating {
		t.Fatalf("/me after return: %d %+v", status, identity)
	}
}

func TestDBToolProxyStripsPanelCredentialsBothWays(t *testing.T) {
	var seenCookie, seenAuthorization string
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCookie, seenAuthorization = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		// An informational response first: the reverse proxy relays it
		// without ModifyResponse.
		w.Header().Set("Link", "</dbtool/phpmyadmin/app.css>; rel=preload")
		w.Header().Add("Set-Cookie", "celikpanel_session=early; Path=/")
		w.Header().Add("Set-Cookie", "=celikpanel_session=early; Path=/")
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		w.WriteHeader(http.StatusEarlyHints)
		for _, name := range []string{"Link", "Set-Cookie", "Clear-Site-Data"} {
			w.Header().Del(name)
		}
		w.Header().Set("Clear-Site-Data", `"cookies"`)
		for _, value := range []string{
			// Nameless: a browser sends these back as celikpanel_session=planted.
			"=celikpanel_session=planted; Path=/",
			" =__Host-celikpanel_session=planted; Path=/; Secure",
			"=other; Path=/",
			"celikpanel_session=planted; Path=/",
			"__Host-celikpanel_session=planted; Path=/; Secure",
			"__host-anything=1; Path=/; Secure",
			"__Secure-x=1; Path=/; Secure",
			"CelikPanel_Impersonator=planted; Path=/",
			"phpMyAdmin=tool-session; path=/dbtool/phpmyadmin/; HttpOnly",
			"pma_lang=tr; path=/dbtool/phpmyadmin/",
		} {
			w.Header().Add("Set-Cookie", value)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "tool page")
	}))
	defer tool.Close()
	target, err := url.Parse(tool.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newDBToolProxy(target)
	front := httptest.NewServer(proxy)
	defer front.Close()

	request, err := http.NewRequest(http.MethodGet, front.URL+"/dbtool/phpmyadmin/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "celikpanel_session=a; __Host-celikpanel_session=b; phpMyAdmin=tool-session; "+
		"celikpanel_impersonator=c; __Host-celikpanel_impersonator=d; pma_lang=tr")
	request.Header.Set("Authorization", "Bearer cpk_secret")
	var early []textproto.MIMEHeader
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			early = append(early, header)
			return nil
		},
	}))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if len(early) != 1 || early[0].Get("Link") == "" {
		t.Fatalf("the informational response was not relayed: %v", early)
	}
	if values := early[0].Values("Set-Cookie"); len(values) != 0 || early[0].Get("Clear-Site-Data") != "" {
		t.Fatalf("the informational response carried Set-Cookie %q, Clear-Site-Data %q", values, early[0].Get("Clear-Site-Data"))
	}
	// What the browser receives, header by header (a nameless cookie is not
	// in response.Cookies()).
	if values := response.Header.Values("Set-Cookie"); len(values) != 2 ||
		values[0] != "phpMyAdmin=tool-session; path=/dbtool/phpmyadmin/; HttpOnly" || values[1] != "pma_lang=tr; path=/dbtool/phpmyadmin/" {
		t.Fatalf("Set-Cookie the tool could send = %q", values)
	}
	if value := response.Header.Get("Clear-Site-Data"); value != "" {
		t.Fatalf("the tool could send Clear-Site-Data %q", value)
	}
	if seenCookie != "phpMyAdmin=tool-session; pma_lang=tr" {
		t.Fatalf("the tool received Cookie %q", seenCookie)
	}
	if seenAuthorization != "" {
		t.Fatalf("the tool received Authorization %q", seenAuthorization)
	}
	var names []string
	for _, cookie := range response.Cookies() {
		names = append(names, cookie.Name)
	}
	if strings.Join(names, ",") != "phpMyAdmin,pma_lang" {
		t.Fatalf("cookies the tool could set = %v", names)
	}
	if body, _ := io.ReadAll(response.Body); string(body) != "tool page" {
		t.Fatalf("body = %q", body)
	}
}

// requestWithHostToken is requestWithToken for a panel served over HTTPS: the
// session cookie under its __Host- name.
func requestWithHostToken(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: hostSessionCookieName, Value: token})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
