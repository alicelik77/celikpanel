package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/auth"
)

// G4 (alpha.128): idle timeout, administrator access by address, sessions.

func sessionsTestPanel(t *testing.T) (*Panel, string) {
	t.Helper()
	panel, token := newAuthHandlerTestPanel(t, true)
	panelAccessRecoveryFile = filepath.Join(t.TempDir(), "panel-access-open")
	invalidatePanelAccessCache()
	t.Cleanup(invalidatePanelAccessCache)
	return panel, token
}

func throughAuth(panel *Panel, method, path, remote, token string) *httptest.ResponseRecorder {
	reached := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = remote
	if token != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	panel.requireAuth(reached).ServeHTTP(recorder, request)
	return recorder
}

func lastSeen(t *testing.T, panel *Panel, token string) string {
	t.Helper()
	var value string
	if err := panel.db.GetDB().QueryRow(`SELECT COALESCE(last_seen_at, '') FROM sessions WHERE substr(token_hash, 1, 16) = ?`,
		auth.SessionHandle(token)).Scan(&value); err != nil {
		return "gone"
	}
	return value
}

func setLastSeen(t *testing.T, panel *Panel, token string, at time.Time) {
	t.Helper()
	if _, err := panel.db.GetDB().Exec(`UPDATE sessions SET last_seen_at = ? WHERE substr(token_hash, 1, 16) = ?`,
		at.UTC().Format(time.RFC3339), auth.SessionHandle(token)); err != nil {
		t.Fatal(err)
	}
}

func TestIdleSessionEndsAndBackgroundReadsDoNotKeepItAlive(t *testing.T) {
	panel, token := sessionsTestPanel(t)

	// A session from before this release (NULL) is never idle: it is used and stamped.
	if got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:4000", token); got.Code != http.StatusTeapot {
		t.Fatalf("first use: %d %s", got.Code, got.Body)
	}
	if lastSeen(t, panel, token) == "" {
		t.Fatal("first use was not recorded")
	}
	var ip, agent string
	_ = panel.db.GetDB().QueryRow(`SELECT COALESCE(client_ip, ''), COALESCE(user_agent, '') FROM sessions`).Scan(&ip, &agent)
	if ip != "198.51.100.7" {
		t.Fatalf("address not recorded: %q", ip)
	}

	// A read 30 minutes later passes but does not count as use.
	stamp := time.Now().Add(-30 * time.Minute)
	setLastSeen(t, panel, token, stamp)
	before := lastSeen(t, panel, token)
	if got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:4000", token); got.Code != http.StatusTeapot {
		t.Fatalf("read: %d", got.Code)
	}
	if lastSeen(t, panel, token) != before {
		t.Fatal("a background read kept the session alive")
	}
	// The activity signal (and any change) counts. It is recorded before the
	// licence gate (which refuses changes in this unlicensed test panel).
	if got := throughAuth(panel, http.MethodPost, sessionActivityPath, "198.51.100.7:4000", token); got.Code == http.StatusUnauthorized || got.Code == http.StatusForbidden {
		t.Fatalf("activity: %d", got.Code)
	}
	if lastSeen(t, panel, token) == before {
		t.Fatal("the activity signal was not recorded")
	}

	// Unused for longer than the default 60 minutes: ended at the next request.
	setLastSeen(t, panel, token, time.Now().Add(-61*time.Minute))
	got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:4000", token)
	if got.Code != http.StatusUnauthorized || !strings.Contains(got.Body.String(), errCodeSessionIdle) {
		t.Fatalf("idle session: %d %s", got.Code, got.Body)
	}
	if lastSeen(t, panel, token) != "gone" {
		t.Fatal("the idle session row was kept")
	}
}

func TestIdleTimeoutFollowsTheAdministratorsChoice(t *testing.T) {
	panel, token := sessionsTestPanel(t)
	if err := panel.setSetting(context.Background(), settingSessionIdleMinutes, "240"); err != nil {
		t.Fatal(err)
	}
	invalidatePanelAccessCache()
	setLastSeen(t, panel, token, time.Now().Add(-90*time.Minute))
	if got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:4000", token); got.Code != http.StatusTeapot {
		t.Fatalf("a 90-minute pause under a 240-minute timeout: %d", got.Code)
	}
	// A value outside the choices falls back to the default.
	_ = panel.setSetting(context.Background(), settingSessionIdleMinutes, "5")
	invalidatePanelAccessCache()
	if got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:4000", token); got.Code != http.StatusUnauthorized {
		t.Fatalf("an invalid stored value did not fall back to 60 minutes: %d", got.Code)
	}
}

func TestAdministratorAddressListLoopbackAndRecoveryFile(t *testing.T) {
	panel, token := sessionsTestPanel(t)
	if err := panel.setSetting(context.Background(), settingPanelAccessList, `["203.0.113.0/24","2001:db8::1"]`); err != nil {
		t.Fatal(err)
	}
	invalidatePanelAccessCache()
	for _, testCase := range []struct {
		remote string
		want   int
	}{
		{"203.0.113.9:5000", http.StatusTeapot},
		{"[2001:db8::1]:5000", http.StatusTeapot},
		{"127.0.0.1:5000", http.StatusTeapot},
		{"[::1]:5000", http.StatusTeapot},
		{"198.51.100.7:5000", http.StatusForbidden},
	} {
		got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", testCase.remote, token)
		if got.Code != testCase.want {
			t.Fatalf("%s: %d %s", testCase.remote, got.Code, got.Body)
		}
		if got.Code == http.StatusForbidden && !strings.Contains(got.Body.String(), errCodePanelAccessNotAllowed) {
			t.Fatalf("refusal without its code: %s", got.Body)
		}
	}
	// The owner's local recovery file suspends the list.
	if err := os.WriteFile(panelAccessRecoveryFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	invalidatePanelAccessCache()
	if got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:5000", token); got.Code != http.StatusTeapot {
		t.Fatalf("recovery file did not open access: %d", got.Code)
	}
}

func TestAddressListNeverRefusesCustomers(t *testing.T) {
	panel, _ := sessionsTestPanel(t)
	db := panel.db.GetDB()
	if _, err := db.Exec(`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (4242, 'cust', 'x', 'c@example.test', 'customer', 'active')`); err != nil {
		t.Fatal(err)
	}
	token, err := panel.sessions.Create(context.Background(), 4242)
	if err != nil {
		t.Fatal(err)
	}
	_ = panel.setSetting(context.Background(), settingPanelAccessList, `["203.0.113.0/24"]`)
	invalidatePanelAccessCache()
	got := throughAuth(panel, http.MethodGet, "/api/v1/auth/me", "198.51.100.7:5000", token)
	if got.Code == http.StatusForbidden && strings.Contains(got.Body.String(), errCodePanelAccessNotAllowed) {
		t.Fatal("a customer was refused by the administrator address list")
	}
}

func TestPanelAccessListParsing(t *testing.T) {
	good, err := parsePanelAccessList([]string{" 203.0.113.7 ", "203.0.113.0/24", "::ffff:198.51.100.1", "2001:db8::/32", "203.0.113.7"})
	if err != nil || len(good) != 4 {
		t.Fatalf("%v %v", good, err)
	}
	if good[2].String() != "198.51.100.1/32" {
		t.Fatalf("mapped address not unmapped: %s", good[2])
	}
	for _, bad := range [][]string{{"0.0.0.0/0"}, {"::/0"}, {"example.com"}, {"203.0.113.300"}, {"fe80::1%eth0"}} {
		if _, err := parsePanelAccessList(bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	many := make([]string, panelAccessListMax+1)
	for i := range many {
		many[i] = "203.0.113.1"
	}
	if _, err := parsePanelAccessList(many); err == nil {
		t.Fatal("accepted too many entries")
	}
}

func putPanelAccess(panel *Panel, remote, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, panelAccessSettingPath, strings.NewReader(body))
	request.RemoteAddr = remote
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: authHandlerTestUserID, Role: roleAdmin}))
	recorder := httptest.NewRecorder()
	panel.handlePanelAccess(recorder, request)
	return recorder
}

func TestSavingTheAddressListCannotLockOutTheSaver(t *testing.T) {
	panel, _ := sessionsTestPanel(t)
	got := putPanelAccess(panel, "198.51.100.7:5000", `{"idle_minutes": 30, "allowlist": ["203.0.113.0/24"]}`)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), errCodePanelAccessLockout) {
		t.Fatalf("lock-out was saved: %d %s", got.Code, got.Body)
	}
	if panel.setting(context.Background(), settingPanelAccessList) != "" {
		t.Fatal("a refused list was stored")
	}
	got = putPanelAccess(panel, "203.0.113.9:5000", `{"idle_minutes": 30, "allowlist": ["203.0.113.0/24", "198.51.100.7"]}`)
	if got.Code != http.StatusOK {
		t.Fatalf("save: %d %s", got.Code, got.Body)
	}
	var view panelAccessView
	if err := json.Unmarshal(got.Body.Bytes(), &view); err != nil || view.IdleMinutes != 30 || len(view.Allowlist) != 2 || !view.YourAddressIn {
		t.Fatalf("%+v %v", view, err)
	}
	// From the server itself (an SSH tunnel) any list may be saved, and an empty list is "everyone".
	if got := putPanelAccess(panel, "127.0.0.1:5000", `{"idle_minutes": 60, "allowlist": ["192.0.2.1"]}`); got.Code != http.StatusOK {
		t.Fatalf("loopback save: %d", got.Code)
	}
	if got := putPanelAccess(panel, "198.51.100.9:5000", `{"idle_minutes": 60, "allowlist": []}`); got.Code != http.StatusOK {
		t.Fatalf("empty list (everyone): %d", got.Code)
	}
	for _, body := range []string{`{"idle_minutes": 7, "allowlist": []}`, `{"idle_minutes": 60}`, `{"idle_minutes": 60, "allowlist": [], "x": 1}`} {
		if got := putPanelAccess(panel, "127.0.0.1:5000", body); got.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, got.Code)
		}
	}
}

func TestOwnSessionsListEndAndEndOthers(t *testing.T) {
	panel, token := sessionsTestPanel(t)
	ctx := context.Background()
	second, _ := panel.sessions.Create(ctx, authHandlerTestUserID)
	third, _ := panel.sessions.Create(ctx, authHandlerTestUserID)
	call := func(handler http.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: authHandlerTestUserID, Role: roleAdmin}))
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		return recorder
	}
	got := call(panel.handleOwnSessions, http.MethodGet, ownSessionsPath, "")
	var report sessionsReport
	if err := json.Unmarshal(got.Body.Bytes(), &report); err != nil || len(report.Sessions) != 3 {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	currentSeen := 0
	for _, session := range report.Sessions {
		if session.Current {
			currentSeen++
		}
		if strings.Contains(got.Body.String(), token) {
			t.Fatal("a session token was sent")
		}
	}
	if currentSeen != 1 {
		t.Fatalf("current marked %d times", currentSeen)
	}
	// The current one is ended with Sign out, not here.
	if got := call(panel.handleOwnSessionEnd, http.MethodPost, ownSessionEndPath, `{"handle":"`+auth.SessionHandle(token)+`"}`); got.Code != http.StatusConflict {
		t.Fatalf("ending the current session: %d", got.Code)
	}
	if got := call(panel.handleOwnSessionEnd, http.MethodPost, ownSessionEndPath, `{"handle":"`+auth.SessionHandle(second)+`"}`); got.Code != http.StatusOK {
		t.Fatalf("end: %d %s", got.Code, got.Body)
	}
	if lastSeen(t, panel, second) != "gone" {
		t.Fatal("the ended session is still there")
	}
	if got := call(panel.handleOwnSessionsEndOthers, http.MethodPost, ownSessionsEndOthers, ""); got.Code != http.StatusOK {
		t.Fatalf("end others: %d", got.Code)
	}
	if lastSeen(t, panel, third) != "gone" || lastSeen(t, panel, token) == "gone" {
		t.Fatal("end-others removed the wrong sessions")
	}
	// Another person's session cannot be ended through the own route.
	_, _ = panel.db.GetDB().Exec(`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (4243, 'other', 'x', 'o@example.test', 'customer', 'active')`)
	other, _ := panel.sessions.Create(ctx, 4243)
	_ = call(panel.handleOwnSessionEnd, http.MethodPost, ownSessionEndPath, `{"handle":"`+auth.SessionHandle(other)+`"}`)
	if lastSeen(t, panel, other) == "gone" {
		t.Fatal("another account's session was ended through the own route")
	}
}

func TestResellerSeesAndEndsOnlyTheirCustomersSessions(t *testing.T) {
	panel, adminToken := sessionsTestPanel(t)
	db := panel.db.GetDB()
	ctx := context.Background()
	for _, statement := range []string{
		`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (5001, 'res1', 'x', 'r1@example.test', 'reseller', 'active')`,
		`INSERT INTO users (id, username, password_hash, email, role, status) VALUES (5002, 'res2', 'x', 'r2@example.test', 'reseller', 'active')`,
		`INSERT INTO users (id, username, password_hash, email, role, status, parent_id) VALUES (5011, 'cust1', 'x', 'c1@example.test', 'customer', 'active', 5001)`,
		`INSERT INTO users (id, username, password_hash, email, role, status, parent_id) VALUES (5021, 'cust2', 'x', 'c2@example.test', 'customer', 'active', 5002)`,
		`INSERT INTO users (id, username, password_hash, email, role, account_type, parent_id, status) VALUES (5012, 'member1', 'x', 'm1@example.test', 'customer', 'additional_user', 5011, 'active')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	resellerToken, _ := panel.sessions.Create(ctx, 5001)
	own, _ := panel.sessions.Create(ctx, 5011)
	member, _ := panel.sessions.Create(ctx, 5012)
	foreign, _ := panel.sessions.Create(ctx, 5021)
	byAdmin, err := panel.sessions.CreateImpersonation(ctx, 5011, authHandlerTestUserID)
	if err != nil {
		t.Fatal(err)
	}
	call := func(handler http.HandlerFunc, method, path, body string, caller *Caller) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: resellerToken})
		request = request.WithContext(context.WithValue(request.Context(), callerKey, caller))
		recorder := httptest.NewRecorder()
		handler(recorder, request)
		return recorder
	}
	reseller := &Caller{ID: 5001, Role: roleReseller}
	got := call(panel.handleResellerSessions, http.MethodGet, resellerSessionsPath, "", reseller)
	var report sessionsReport
	if err := json.Unmarshal(got.Body.Bytes(), &report); err != nil {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	seen := map[string]bool{}
	for _, session := range report.Sessions {
		seen[session.Handle] = true
	}
	if len(report.Sessions) != 2 || !seen[auth.SessionHandle(own)] || !seen[auth.SessionHandle(member)] {
		t.Fatalf("reseller scope: %s", got.Body)
	}
	if seen[auth.SessionHandle(foreign)] || seen[auth.SessionHandle(byAdmin)] || seen[auth.SessionHandle(adminToken)] || seen[auth.SessionHandle(resellerToken)] {
		t.Fatal("the reseller saw a session outside their customers")
	}
	// Another reseller's customer cannot be ended.
	_ = call(panel.handleResellerSessionEnd, http.MethodPost, resellerSessionEndPath, `{"handle":"`+auth.SessionHandle(foreign)+`"}`, reseller)
	if lastSeen(t, panel, foreign) == "gone" {
		t.Fatal("a reseller ended another reseller's customer's session")
	}
	_ = call(panel.handleResellerSessionEnd, http.MethodPost, resellerSessionEndPath, `{"handle":"`+auth.SessionHandle(byAdmin)+`"}`, reseller)
	if lastSeen(t, panel, byAdmin) == "gone" {
		t.Fatal("a reseller ended the administrator's Log in as session")
	}
	if got := call(panel.handleResellerSessionEnd, http.MethodPost, resellerSessionEndPath, `{"handle":"`+auth.SessionHandle(member)+`"}`, reseller); got.Code != http.StatusOK {
		t.Fatalf("end: %d %s", got.Code, got.Body)
	}
	if lastSeen(t, panel, member) != "gone" {
		t.Fatal("the customer's team member's session was not ended")
	}
	// A customer is refused; under "Log in as" ending is refused.
	if got := call(panel.handleResellerSessions, http.MethodGet, resellerSessionsPath, "", &Caller{ID: 5011, Role: roleCustomer}); got.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", got.Code)
	}
	if got := call(panel.handleResellerSessionEnd, http.MethodPost, resellerSessionEndPath, `{"handle":"`+auth.SessionHandle(own)+`"}`,
		&Caller{ID: 5001, Role: roleReseller, ImpersonatorID: authHandlerTestUserID}); got.Code != http.StatusConflict {
		t.Fatalf("impersonating: %d", got.Code)
	}
}
