package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.114: the owner's automatic/manual choice and the Dashboard summary.
// The choice is written only with the digest of the file that was shown; the
// summary answers from the last reading and never starts a change.

func TestTheUpdateChoiceIsValidatedPassedOnAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restore := setOSUpdatesModeThroughAgent
	t.Cleanup(func() { setOSUpdatesModeThroughAgent = restore })
	var calls atomic.Int32
	var gotMode, gotSHA string
	var gotConfirm bool
	setOSUpdatesModeThroughAgent = func(_ *Panel, _ context.Context, mode, sha string, confirm bool) (transport.SetOSUpdatesModeResponse, error) {
		calls.Add(1)
		gotMode, gotSHA, gotConfirm = mode, sha, confirm
		return transport.SetOSUpdatesModeResponse{Written: true, Effective: true, Mode: mode, Path: "/etc/apt/apt.conf.d/20auto-upgrades", PreviousState: "edited"}, nil
	}
	for _, body := range []string{`{"mode":"sometimes"}`, `{"mode":"auto","expected_current_sha256":"nope"}`, `not json`, `{}`} {
		recorder := httptest.NewRecorder()
		panel.handleOSUpdatesMode(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesModePath, body, userID))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, recorder.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("the Agent was asked about an invalid request")
	}
	// Not by a plain read.
	recorder := httptest.NewRecorder()
	panel.handleOSUpdatesMode(recorder, serviceStateAtomicRequest(http.MethodGet, osUpdatesModePath, "", userID))
	if recorder.Code != http.StatusMethodNotAllowed || calls.Load() != 0 {
		t.Fatalf("GET: %d", recorder.Code)
	}
	digest := strings.Repeat("cd", 32)
	recorder = httptest.NewRecorder()
	panel.handleOSUpdatesMode(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesModePath, `{"mode":"manual","expected_current_sha256":"`+digest+`","confirm_owner_edit":true}`, userID))
	if recorder.Code != http.StatusOK || gotMode != "manual" || gotSHA != digest || !gotConfirm || !strings.Contains(recorder.Body.String(), `"effective":true`) {
		t.Fatalf("write: %d %s", recorder.Code, recorder.Body.String())
	}
	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "security.os_updates_mode:manual" || !strings.Contains(details, `"owner_edit_confirmed":true`) || !strings.Contains(details, `"seen_sha256":"`+digest+`"`) {
		t.Fatalf("audit = %q %s", action, details)
	}
	// Not for a customer.
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, osUpdatesModePath, strings.NewReader(`{"mode":"auto"}`))
	panel.handleOSUpdatesMode(recorder, request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 9, Role: roleCustomer})))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", recorder.Code)
	}
}

func TestARefusedChoiceKeepsTheAgentsReasonAndCode(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restore := setOSUpdatesModeThroughAgent
	t.Cleanup(func() { setOSUpdatesModeThroughAgent = restore })
	setOSUpdatesModeThroughAgent = func(*Panel, context.Context, string, string, bool) (transport.SetOSUpdatesModeResponse, error) {
		return transport.SetOSUpdatesModeResponse{Error: "the file was changed by the owner", ErrorCode: "OWNER_EDITED"}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleOSUpdatesMode(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesModePath, `{"mode":"manual"}`, userID))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "OS_UPDATES_MODE_OWNER_EDITED") || !strings.Contains(recorder.Body.String(), "changed by the owner") {
		t.Fatalf("refused: %d %s", recorder.Code, recorder.Body.String())
	}
	var action string
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(action, "security.os_updates_mode.failed") {
		t.Fatalf("audit = %q", action)
	}
	// An Agent that cannot be reached is an error, not "written".
	setOSUpdatesModeThroughAgent = func(*Panel, context.Context, string, string, bool) (transport.SetOSUpdatesModeResponse, error) {
		return transport.SetOSUpdatesModeResponse{}, errors.New("agent unreachable")
	}
	recorder = httptest.NewRecorder()
	panel.handleOSUpdatesMode(recorder, serviceStateAtomicRequest(http.MethodPost, osUpdatesModePath, `{"mode":"auto"}`, userID))
	if recorder.Code == http.StatusOK {
		t.Fatalf("unreachable Agent answered OK: %s", recorder.Body.String())
	}
}

func TestTheDashboardSummaryNeverStartsAChangeAndReadsOnceAtATime(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restoreRead, restoreStart, restoreMode := readOSUpdatesFromAgent, startOSUpdatesRefresh, setOSUpdatesModeThroughAgent
	t.Cleanup(func() {
		readOSUpdatesFromAgent, startOSUpdatesRefresh, setOSUpdatesModeThroughAgent = restoreRead, restoreStart, restoreMode
		osUpdatesSummary.forget()
	})
	osUpdatesSummary.forget()
	var reads, changes atomic.Int32
	setOSUpdatesModeThroughAgent = func(*Panel, context.Context, string, string, bool) (transport.SetOSUpdatesModeResponse, error) {
		changes.Add(1)
		return transport.SetOSUpdatesModeResponse{}, nil
	}
	readOSUpdatesFromAgent = func(*Panel, context.Context) (transport.OSUpdatesStatusResponse, error) {
		reads.Add(1)
		return transport.OSUpdatesStatusResponse{
			PackageFamily: "apt",
			Automatic:     transport.OSUpdatesAutomatic{State: "on", Tool: "unattended-upgrades"},
			Pending:       transport.OSUpdatesPending{State: "known", Total: 7, Security: 2, SecurityNames: []string{"openssl", "libssl3"}},
		}, nil
	}
	var queued []func()
	startOSUpdatesRefresh = func(run func()) { queued = append(queued, run) }
	get := func() string {
		recorder := httptest.NewRecorder()
		panel.handleOSUpdatesSummary(recorder, serviceStateAtomicRequest(http.MethodGet, osUpdatesSummaryPath, "", userID))
		if recorder.Code != http.StatusOK {
			t.Fatalf("summary: %d %s", recorder.Code, recorder.Body.String())
		}
		return recorder.Body.String()
	}
	if body := get(); !strings.Contains(body, `"state":"checking"`) {
		t.Fatalf("first: %s", body)
	}
	// Asking again while the reading is under way starts no second reading.
	get()
	get()
	if len(queued) != 1 || reads.Load() != 0 {
		t.Fatalf("queued %d reads %d", len(queued), reads.Load())
	}
	queued[0]()
	body := get()
	if !strings.Contains(body, `"state":"ready"`) || !strings.Contains(body, `"total":7`) || !strings.Contains(body, `"security":2`) || strings.Contains(body, "openssl") {
		t.Fatalf("ready: %s", body)
	}
	// A fresh reading is reused: no new reading, never a change.
	get()
	get()
	if len(queued) != 1 || reads.Load() != 1 || changes.Load() != 0 {
		t.Fatalf("queued %d reads %d changes %d", len(queued), reads.Load(), changes.Load())
	}
	// A failed reading keeps what is known and does not retry at once.
	osUpdatesSummary.mu.Lock()
	osUpdatesSummary.at = osUpdatesSummary.at.Add(-2 * osUpdatesSummaryFresh)
	osUpdatesSummary.mu.Unlock()
	readOSUpdatesFromAgent = func(*Panel, context.Context) (transport.OSUpdatesStatusResponse, error) {
		return transport.OSUpdatesStatusResponse{}, errors.New("agent unreachable")
	}
	if body := get(); !strings.Contains(body, `"stale":true`) || !strings.Contains(body, `"total":7`) {
		t.Fatalf("stale: %s", body)
	}
	queued[1]()
	get()
	get()
	if len(queued) != 2 {
		t.Fatalf("retried at once: %d", len(queued))
	}
	// Nothing known and the reading failed: unknown, never "no updates".
	osUpdatesSummary.forget()
	if body := get(); !strings.Contains(body, `"state":"unknown"`) {
		t.Fatalf("unknown: %s", body)
	}
	// Only for administrators.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, osUpdatesSummaryPath, nil)
	panel.handleOSUpdatesSummary(recorder, request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 9, Role: roleCustomer})))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", recorder.Code)
	}
}
