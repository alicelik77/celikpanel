package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §4): Security → Security updates. A failed read is
// unknown, never off; the drop-in is written only with the digest the
// administrator saw, and the write is audited with its details.

func TestSecurityUpdatesReadIsUnknownWhenTheAgentCannotAnswer(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restore := readOSUpdatesFromAgent
	t.Cleanup(func() { readOSUpdatesFromAgent = restore })
	readOSUpdatesFromAgent = func(*Panel, context.Context) (transport.OSUpdatesStatusResponse, error) {
		return transport.OSUpdatesStatusResponse{}, errors.New("agent unreachable")
	}
	recorder := httptest.NewRecorder()
	panel.handleOSUpdates(recorder, serviceStateAtomicRequest(http.MethodGet, osUpdatesPath, "", userID))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unknown"`) {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body.String())
	}
	readOSUpdatesFromAgent = func(*Panel, context.Context) (transport.OSUpdatesStatusResponse, error) {
		return transport.OSUpdatesStatusResponse{PackageFamily: "apt", Automatic: transport.OSUpdatesAutomatic{State: "off", Reason: "config_off"}}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleOSUpdates(recorder, serviceStateAtomicRequest(http.MethodGet, osUpdatesPath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"state":"read"`) || !strings.Contains(recorder.Body.String(), `"vendors":[]`) {
		t.Fatalf("read: %s", recorder.Body.String())
	}
	// Not for a customer.
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, osUpdatesPath, nil)
	panel.handleOSUpdates(recorder, request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 9, Role: roleCustomer})))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", recorder.Code)
	}
}

func TestTheDropInIsWrittenOnlyWithTheConfirmedDigestAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restore := writeVendorOriginsThroughAgent
	t.Cleanup(func() { writeVendorOriginsThroughAgent = restore })
	var sent string
	writeVendorOriginsThroughAgent = func(_ *Panel, _ context.Context, expected string) (transport.WriteVendorUpdateOriginsResponse, error) {
		sent = expected
		return transport.WriteVendorUpdateOriginsResponse{Written: true, Effective: true, Path: "/etc/apt/apt.conf.d/52celikpanel-vendor-origins", Origins: []string{"deb.sury.org"}}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleOSUpdatesOrigins(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesOriginsPath, `{"expected_sha256":"not-a-digest"}`, userID))
	if recorder.Code != http.StatusBadRequest || sent != "" {
		t.Fatalf("bad digest: %d sent=%q", recorder.Code, sent)
	}
	digest := strings.Repeat("ab", 32)
	recorder = httptest.NewRecorder()
	panel.handleOSUpdatesOrigins(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesOriginsPath, `{"expected_sha256":"`+digest+`"}`, userID))
	if recorder.Code != http.StatusOK || sent != digest || !strings.Contains(recorder.Body.String(), `"effective":true`) {
		t.Fatalf("write: %d %s", recorder.Code, recorder.Body.String())
	}
	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "security.update_origins:deb.sury.org" || !strings.Contains(details, `"confirmed_sha256":"`+digest+`"`) || !strings.Contains(details, `"origins":["deb.sury.org"]`) {
		t.Fatalf("audit = %q %s", action, details)
	}
}
