package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// D-032: the panel looks by itself; it never asks the owner to press a button
// to learn a state it can read. Two server sentences still asked for a rescan
// (alpha.85 release notes, "Templated text"): the refusal of a stored component
// record that cannot be verified, and the Store's three component-record
// states. They now say what CelikPanel does (2026-10-15).
//
// D-032: panel kendisi bakar; okuyabileceği bir durumu öğrenmek için sahibe
// düğmeye bastırmaz. Yeniden tarama isteyen iki sunucu cümlesi artık
// CelikPanel'in ne yaptığını söyler.

var askForRescan = regexp.MustCompile(`(?i)\brescan\b|run a (fresh )?scan|yeniden tara|taramas[ıi]n[ıi] çalıştır`)

func TestUnverifiedComponentRecordRefusalSaysThePanelChecksByItself(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	if _, err := panel.db.GetDB().Exec(
		`INSERT INTO service_scan_cache (id, data, scanned_at) VALUES (1, ?, ?)`,
		`{"observations":[`, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	panel.handleManagedServices(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/managed-services", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", recorder.Code, recorder.Body.String())
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != errCodeServiceStateUnverified {
		t.Fatalf("code = %q, want %q", body.Code, errCodeServiceStateUnverified)
	}
	if askForRescan.MatchString(body.Error) {
		t.Fatalf("the refusal asks the owner to rescan: %q", body.Error)
	}
	for _, part := range []string{"nothing was changed", "checks the components again by itself", "while the Dashboard, the Components page or a component's page is open"} {
		if !strings.Contains(body.Error, part) {
			t.Fatalf("the refusal lacks %q: %q", part, body.Error)
		}
	}
}

func TestStoreComponentRecordStatesSayWhatThePanelDoes(t *testing.T) {
	for _, code := range []string{"component_state_unavailable", "component_state_stale", "component_state_invalid"} {
		for _, locale := range []string{"en", "tr"} {
			text := localizedStoreReason(code, locale)
			if text == "" {
				t.Fatalf("%s/%s has no sentence", code, locale)
			}
			if askForRescan.MatchString(text) {
				t.Fatalf("%s/%s asks the owner to rescan: %q", code, locale, text)
			}
			if !strings.Contains(text, "CelikPanel") {
				t.Fatalf("%s/%s does not say what CelikPanel does: %q", code, locale, text)
			}
		}
		if localizedStoreReason(code, "en") == localizedStoreReason(code, "tr") {
			t.Fatalf("%s is not translated", code)
		}
	}
}
