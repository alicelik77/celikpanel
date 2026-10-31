package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/licensing"
)

// D-039: while the license service cannot be reached, a receipt inside its
// signed offline window keeps management open for every role, and the license
// page says so (check, time left, last successful check) without a secret.
func TestLicenseOutageInsideOfflineWindowKeepsPanelAndExplainsIt(t *testing.T) {
	fixture := newAuthzMatrixFixture(t)
	seedAdditionalUserSession(t, &fixture)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().Unix()
	issued := now - 2*86400
	c := licensing.Claims{Format: "celikpanel-license-v1", Product: "celikpanel", LicenseID: strings.Repeat("a", 32), ServerID: strings.Repeat("b", 64),
		ActivatedAt: issued - 86400, IssuedAt: issued, ExpiresAt: now + 365*86400, RefreshAfter: issued + 3600, OfflineUntil: issued + 7*86400}
	payload, _ := json.Marshal(c)
	token := strings.Repeat("c", 64)
	body, _ := json.Marshal(licensing.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, payload)), ActivationToken: token})
	file := filepath.Join(t.TempDir(), "license.json")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	previous := http.DefaultTransport
	http.DefaultTransport = licenseTestTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("license service unreachable")
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	m, err := licensing.New(file, pub, c.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.panel.license = m
	management := fixture.panel.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for _, id := range []int{authzMatrixAdminID, authzMatrixResellerID, authzMatrixCustomerID, authzMatrixAdditionalUserID} {
		if w := requestWithToken(management, http.MethodGet, "/api/v1/domains", fixture.tokens[id]); w.Code != http.StatusNoContent {
			t.Fatalf("outage inside the offline window closed management for %d: %d %s", id, w.Code, w.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatal("outage was retried per request instead of backing off", calls.Load())
	}
	page := fixture.panel.requireAuth(http.HandlerFunc(fixture.panel.handleLicense))
	w := requestWithToken(page, http.MethodGet, panelLicensePath, fixture.tokens[authzMatrixAdminID])
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	left, _ := status["offline_for_seconds"].(float64)
	ago, _ := status["verified_seconds_ago"].(float64)
	if status["state"] != "active" || status["can_provision"] != true || status["check"] != licensing.CheckUnreachable ||
		left < 5*86400-120 || left > 5*86400 || ago < 2*86400 || ago > 2*86400+120 {
		t.Fatal("license page does not explain the outage", status)
	}
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "activation_token") {
		t.Fatal("license page exposed the refresh credential")
	}
	access := fixture.panel.requireAuth(http.HandlerFunc(fixture.panel.handleLicenseAccess))
	w = requestWithToken(access, http.MethodGet, panelLicenseAccessPath, fixture.tokens[authzMatrixCustomerID])
	var answer map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &answer)
	if answer["can_use_panel"] != true || answer["valid_until"] != float64(c.OfflineUntil) || len(answer) != 5 {
		t.Fatal("access answer", answer)
	}
}
