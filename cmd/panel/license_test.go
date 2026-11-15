package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
)

func TestLicenseEndpointRequiresServerAdministrator(t *testing.T) {
	if !isAdminOnlyPath(panelLicensePath) {
		t.Fatal("license endpoint not admin only")
	}
	for _, role := range []string{"customer", "reseller", ""} {
		request := httptest.NewRequest(http.MethodPost, panelLicensePath, strings.NewReader(`{"action":"refresh"}`))
		request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 1, Role: role}))
		w := httptest.NewRecorder()
		(&Panel{}).handleLicense(w, request)
		if w.Code != 403 {
			t.Fatal(role, w.Code)
		}
	}
}

// Real signed local receipts keep authorization fixtures independent of the network.
func testPanelLicense(t *testing.T, state string) *licensing.Manager {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "license.json")
	now := time.Now().Unix()
	c := licensing.Claims{Format: "celikpanel-license-v1", Product: "celikpanel", LicenseID: strings.Repeat("a", 32), ServerID: strings.Repeat("b", 64), ActivatedAt: now - 86400, IssuedAt: now - 10, ExpiresAt: now + 86400, RefreshAfter: now + 3600, OfflineUntil: now + 7200}
	if state == "expired" {
		c.ExpiresAt = now - 1
		c.OfflineUntil = now - 1
	}
	if state == "active_year" {
		c.ExpiresAt = now + 365*86400
	}
	if state == "verification_unavailable" {
		c.OfflineUntil = now - 1
	}
	if state != "missing" {
		payload, _ := json.Marshal(c)
		e := licensing.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload)), ActivationToken: strings.Repeat("c", 64)}
		if state == "rejected" {
			e.Rejected = true
		}
		if state == "invalid" {
			e.Signature = "invalid"
		}
		data, _ := json.Marshal(e)
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := licensing.New(file, pub, c.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLicenseAccessResponseIsMinimalAndTracksState(t *testing.T) {
	p := &Panel{}
	for _, state := range []string{"missing", "expired", "invalid", "verification_unavailable", "rejected", "active"} {
		p.license = testPanelLicense(t, state)
		w := httptest.NewRecorder()
		p.handleLicenseAccess(w, httptest.NewRequest("GET", panelLicenseAccessPath, nil))
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		// The fixture's signed offline window ends two hours from now.
		if state == "active" && result["valid_until"].(float64) > float64(time.Now().Add(2*time.Hour).Unix()) {
			t.Fatal("browser deadline exceeds the signed offline window", result)
		}
		// The time left is the same deadline, stated on the server's clock
		// (a refused licence may still carry its unexpired deadline; the page
		// reads it only with can_use_panel).
		// Kalan süre aynı son andır; sunucunun saatiyle söylenir (reddedilen
		// lisans dolmamış son anını taşıyabilir; sayfa onu yalnız can_use_panel ile okur).
		if left, ok := result["valid_for_seconds"].(float64); !ok || left < 0 || left > 7200 || (state == "active" && left <= 0) {
			t.Fatal("time left does not follow the deadline", state, result)
		}
		wantState, wantObservation, wantFields := state, "known", 6
		if state == "rejected" {
			wantState = "invalid"
		}
		if state == "verification_unavailable" {
			// The lapse names its cause, so the page can say who acts.
			wantObservation, wantFields = "unavailable", 7
			if result["reason"] != licensing.ReasonOfflineEnded {
				t.Fatal("lapse without its reason", result)
			}
		}
		if wantLevel := map[bool]string{true: "full", false: "limited"}[state == "active"]; result["level"] != wantLevel {
			t.Fatal("level", state, result)
		}
		if len(result) != wantFields || result["state"] != wantState || result["observation"] != wantObservation || result["can_use_panel"] != (state == "active") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(state, result)
		}
	}
}

// The access answer states the time left on the server's own clock, so a page
// whose clock differs from the server's still follows the server's deadline
// (set14b D2). Rule: valid_until minus the moment of the answer, millisecond
// precision, never below 0; 0 without a deadline.
// Erişim yanıtı kalan süreyi sunucunun kendi saatiyle söyler; saati farklı bir
// sayfa da sunucunun son anını izler. Kural: valid_until eksi yanıt anı,
// milisaniye hassasiyetinde, 0'ın altına inmez; son an yoksa 0.
func TestLicenseAccessAnswerStatesTimeLeftOnTheServerClock(t *testing.T) {
	now := time.Unix(1_800_000_000, 250_000_000)
	for _, tc := range []struct {
		name          string
		status        licensing.Status
		until         int64
		validFor      float64
		canUsePanel   bool
		state, observ string
	}{
		{"active, offline window first", licensing.Status{State: "active", Observation: "known", ExpiresAt: 1_800_003_600, OfflineUntil: 1_800_000_060, CanProvision: true}, 1_800_000_060, 59.75, true, "active", "known"},
		{"active, expiry first", licensing.Status{State: "active", Observation: "known", ExpiresAt: 1_800_000_010, OfflineUntil: 1_800_000_060, CanProvision: true}, 1_800_000_010, 9.75, true, "active", "known"},
		{"deadline passed", licensing.Status{State: "verification_unavailable", Observation: "unavailable", ExpiresAt: 1_800_003_600, OfflineUntil: 1_799_999_000}, 1_799_999_000, 0, false, "verification_unavailable", "unavailable"},
		{"no deadline", licensing.Status{State: "missing", Observation: "known"}, 0, 0, false, "missing", "known"},
	} {
		got := newLicenseAccessAnswer(tc.status, now)
		want := licenseAccessAnswer{CanUsePanel: tc.canUsePanel, ValidUntil: tc.until, ValidForSeconds: tc.validFor, State: tc.state, Observation: tc.observ,
			Level: map[bool]string{true: licenceLevelFull, false: licenceLevelLimited}[tc.canUsePanel]}
		if got != want {
			t.Fatalf("%s: got %+v want %+v", tc.name, got, want)
		}
	}
}

// Panel updates and readiness are the administrator's in every licence state:
// the licence gate never refuses them (D-047), the role gate does for others.
func TestLicenseUpdateRoutesAreOpenInEveryStateAndAdministratorOnly(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	paths := []string{panelUpdateCheckPath, panelUpdateStatusPath, panelUpdateStartPath, panelUpdateAbandonPath,
		panelUpdateDiscoveryPath, panelUpdateDiscoveryRefreshPath, "/api/v1/panel/version"}
	for _, state := range []string{"missing", "expired", "invalid", "verification_unavailable", "rejected", "unconfigured"} {
		fixture.panel.license = nil
		if state != "unconfigured" {
			fixture.panel.license = testPanelLicense(t, state)
		}
		reached := 0
		handler := fixture.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(204) }))
		for _, path := range paths {
			if !isAdminOnlyPath(path) {
				t.Fatalf("%s is not administrator only", path)
			}
			for _, id := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, authzMatrixAdditionalUserID} {
				for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"} {
					before := reached
					w := requestWithToken(handler, method, path, fixture.tokens[id])
					admin := id == authzMatrixAdminID
					if (reached != before) != admin || (admin && w.Code != 204) || (!admin && w.Code != http.StatusForbidden) {
						t.Fatalf("%s %s %s role=%d status=%d", state, method, path, id, w.Code)
					}
				}
			}
			before := reached
			if w := requestWithToken(handler, http.MethodPost, path, ""); reached != before || w.Code != 401 {
				t.Fatalf("unauthenticated update: %s %d", path, w.Code)
			}
		}
		w := httptest.NewRecorder()
		fixture.panel.handleLicenseAccess(w, httptest.NewRequest("GET", panelLicenseAccessPath, nil))
		if !strings.Contains(w.Body.String(), `"can_use_panel":false`) || !strings.Contains(w.Body.String(), `"level":"limited"`) {
			t.Fatal("update access changed the licence", w.Body.String())
		}
	}
}

func TestLicenseLockedSignedUpdatePreservesLicenseAndTargetChecks(t *testing.T) {
	withSystemUpdateBuild(t)
	for _, state := range []string{"missing", "expired"} {
		t.Run(state, func(t *testing.T) {
			fixture := newSystemUpdateTestFixture(t)
			fixture.panel.license = testPanelLicense(t, state)
			before := fixture.panel.license.Status()
			invoke := func(path, body string) *httptest.ResponseRecorder {
				r := systemUpdateRequest("POST", path, body, roleAdmin)
				w := httptest.NewRecorder()
				if !fixture.panel.allowLicensedPanel(w, r) {
					t.Fatal("signed update blocked", w.Body.String())
				}
				csrfProtect(http.HandlerFunc(fixture.panel.handlePanelUpdateStart)).ServeHTTP(w, r)
				return w
			}
			bad := strings.Replace(systemUpdateStartBody(updateTestTargetVersion), updateTestTargetSHA, strings.Repeat("f", 64), 1)
			if w := invoke(panelUpdateStartPath, bad); w.Code < 400 {
				t.Fatal("unverified target accepted", w.Body.String())
			}
			if fixture.agent.startCalls != 0 {
				t.Fatal("bad target reached start")
			}
			w := invoke(panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion))
			if w.Code != http.StatusAccepted {
				t.Fatalf("valid update: %d %s", w.Code, w.Body.String())
			}
			if fixture.agent.startCalls != 1 {
				t.Fatal("valid target not started once")
			}
			if after := fixture.panel.license.Status(); after != before {
				t.Fatal("update changed license", before, after)
			}
			// The firewall works while the licence is not active (D-047);
			// the update does not open growth.
			w = httptest.NewRecorder()
			if !fixture.panel.allowLicensedPanel(w, systemUpdateRequest("POST", "/api/v1/firewall", "{}", roleAdmin)) {
				t.Fatal("the firewall was refused for a licence reason", w.Body.String())
			}
			w = httptest.NewRecorder()
			if fixture.panel.allowLicensedPanel(w, systemUpdateRequest("POST", "/api/v1/domains/create", "{}", roleAdmin)) || w.Code != http.StatusForbidden {
				t.Fatal("update unlocked growth", w.Code)
			}
		})
	}
}
