package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The domain certificate page dates the end of an HSTS retirement as the
// moment it received the answer plus hsts_retire_remaining_seconds, measured on
// the server's own clock (D-032 clock rule, 2026-10-14). Before, it compared
// hsts_retire_after with the browser's clock, so a browser clock ahead of or
// behind the server's moved the end and could unlock controls the server
// still refuses, or keep them locked after the server allows them.
// Sayfa, HSTS geri çekilmesinin sonunu yanıtı aldığı an artı sunucunun kendi
// saatiyle ölçülen kalan süre olarak tarihler.

func TestStampHSTSRetireRemaining(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	ahead := now.Add(90*time.Second + 250*time.Millisecond)
	past := now.Add(-time.Minute)
	for _, tc := range []struct {
		name   string
		retire *time.Time
		want   *float64
	}{
		{"no retirement", nil, nil},
		{"running", &ahead, ptrFloat(90.25)},
		{"passed", &past, ptrFloat(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := SSLSettings{HSTSRetireAfter: tc.retire, HSTSRetireRemainingSeconds: ptrFloat(-1)}
			settings.stampHSTSRetireRemaining(now)
			switch {
			case tc.want == nil && settings.HSTSRetireRemainingSeconds != nil:
				t.Fatalf("remaining = %v, want none", *settings.HSTSRetireRemainingSeconds)
			case tc.want != nil && (settings.HSTSRetireRemainingSeconds == nil || *settings.HSTSRetireRemainingSeconds != *tc.want):
				t.Fatalf("remaining = %v, want %v", settings.HSTSRetireRemainingSeconds, *tc.want)
			}
			body, err := json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(body, &wire); err != nil {
				t.Fatal(err)
			}
			if _, present := wire["hsts_retire_remaining_seconds"]; present != (tc.want != nil) {
				t.Fatalf("hsts_retire_remaining_seconds present = %v in %s", present, body)
			}
		})
	}
}

func ptrFloat(value float64) *float64 { return &value }

func TestDomainSSLAnswerCarriesHSTSRetireRemaining(t *testing.T) {
	p, domainID, _ := newSSLStateFixture(t)
	read := func() map[string]json.RawMessage {
		t.Helper()
		recorder := httptest.NewRecorder()
		p.handleDomainSSL(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/domains/%d/ssl", domainID), nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var answer struct {
			Settings map[string]json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		return answer.Settings
	}

	if _, present := read()["hsts_retire_remaining_seconds"]; present {
		t.Fatal("hsts_retire_remaining_seconds present without a retirement")
	}

	// RFC3339 keeps whole seconds, so the stored end is up to one second
	// earlier than this; the remaining time is measured from the stored end.
	retireAfter := time.Now().UTC().Add(120 * time.Second)
	if _, err := p.db.GetDB().Exec(`
		UPDATE sites SET hsts_enabled = false, hsts_retire_after = ? WHERE domain_id = ?`,
		retireAfter.Format(time.RFC3339), domainID,
	); err != nil {
		t.Fatalf("prepare HSTS retirement: %v", err)
	}
	settings := read()
	if _, present := settings["hsts_retire_after"]; !present {
		t.Fatal("hsts_retire_after is kept for older pages")
	}
	var remaining float64
	if err := json.Unmarshal(settings["hsts_retire_remaining_seconds"], &remaining); err != nil {
		t.Fatalf("hsts_retire_remaining_seconds = %s, want a number", settings["hsts_retire_remaining_seconds"])
	}
	if remaining <= 117 || remaining > 120 {
		t.Fatalf("hsts_retire_remaining_seconds = %v, want in (117, 120]", remaining)
	}
}
