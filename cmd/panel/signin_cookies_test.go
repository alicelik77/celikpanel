package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	// What a website under the same domain would have to send to plant a
	// session under the new name: a browser refuses every one of these.
	for _, planted := range []*http.Cookie{
		{Name: hostSessionCookieName, Value: "x", Path: "/", Domain: "example.com", Secure: true},
		{Name: hostSessionCookieName, Value: "x", Path: "/api", Secure: true},
		{Name: hostSessionCookieName, Value: "x", Path: "/"},
	} {
		if hostPrefixAccepted(planted) {
			t.Fatalf("planted %+v would be stored", planted)
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
			if recorder.Code != http.StatusUnauthorized || signinErrorCode(t, recorder) != errCodeAuthRequired || fixture.seenCaller != -1 {
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
	})
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

func TestDBToolProxyStripsPanelCredentialsBothWays(t *testing.T) {
	var seenCookie, seenAuthorization string
	tool := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCookie, seenAuthorization = r.Header.Get("Cookie"), r.Header.Get("Authorization")
		for _, value := range []string{
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
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
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
