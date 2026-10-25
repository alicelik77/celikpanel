package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §9): webmail updates. A failed check is unknown; an
// update names a release, runs in its own job and is audited with from, to,
// the backup and the outcome; a put-back is said.
// alpha.109: the update is a durable operation: POST answers 202 at once,
// the work runs detached from the request, the same request id never starts
// a second update, and GET carries the last update's outcome.

func postWebmailUpdate(t *testing.T, panel *Panel, userID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodPost, roundcubeUpdatePath, body, userID))
	return recorder
}

func TestWebmailUpdateIsADurableOperationThatSaysItsOutcome(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restoreRead, restoreUpdate := readRoundcubeUpdateFromAgent, updateRoundcubeThroughAgent
	t.Cleanup(func() { readRoundcubeUpdateFromAgent, updateRoundcubeThroughAgent = restoreRead, restoreUpdate })
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{}, errors.New("agent unreachable")
	}
	recorder := httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodGet, roundcubeUpdatePath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"state":"unknown"`) || !strings.Contains(recorder.Body.String(), `"last_update":null`) {
		t.Fatalf("unknown: %s", recorder.Body.String())
	}
	if code := postWebmailUpdate(t, panel, userID, `{"version":"latest; rm -rf /","request_id":"`+mustServiceOperationRequestID(t)+`"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("bad version: %d", code)
	}
	if code := postWebmailUpdate(t, panel, userID, `{"version":"1.6.20"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("no request id: %d", code)
	}

	// Frankfurt's failure: put back, the previous tree answers again.
	calls := 0
	updateRoundcubeThroughAgent = func(_ *Panel, ctx context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
		calls++
		if _, err := panelMutationBinding(ctx); err != nil {
			t.Errorf("the update runs without the operation's lease: %v", err)
		}
		return transport.RoundcubeUpdateResponse{To: version, From: "1.6.15", RolledBack: true, PreviousAnswers: true, ErrorCode: "PROOF_FAILED",
			Error: "webmail did not answer on 1.6.20 (PHP-FPM 8.4 could not open CelikPanel's PHP check for Webmail (Roundcube) (HTTP 404: 404 Not Found)); the previous 1.6.15 tree was put back byte for byte and answers again"}, nil
	}
	requestID := mustServiceOperationRequestID(t)
	body := `{"version":"1.6.20","request_id":"` + requestID + `"}`
	recorder = postWebmailUpdate(t, panel, userID, body)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	accepted := decodeServiceOperationEnvelope(t, recorder)
	if accepted == nil || accepted.Kind != serviceOperationKindWebmailUpdate || accepted.ServiceID != "roundcube" || accepted.PackageName != "1.6.20" {
		t.Fatalf("operation: %+v", accepted)
	}
	failed, _ := waitForServiceOperation(t, panel, userID, accepted.ID, serviceOperationFailed)
	var result map[string]any
	if err := json.Unmarshal(failed.Result, &result); err != nil {
		t.Fatal(err)
	}
	if failed.Error == nil || failed.Error.Code != "webmail_update_failed" || result["outcome"] != webmailUpdateOutcomeRolledBack ||
		result["previous_answers"] != true || result["from"] != "1.6.15" || !strings.Contains(result["reason"].(string), "HTTP 404") {
		t.Fatalf("failed row: %+v %s", failed.Error, failed.Result)
	}
	// The same request again (a reload, a lost answer) is the same operation.
	recorder = postWebmailUpdate(t, panel, userID, body)
	if again := decodeServiceOperationEnvelope(t, recorder); recorder.Code != http.StatusAccepted || again.ID != accepted.ID || calls != 1 {
		t.Fatalf("replay: %d %+v calls=%d", recorder.Code, again, calls)
	}
	var action string
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(action, "webmail.update.failed:1.6.20 — webmail did not answer") {
		t.Fatalf("audit = %q", action)
	}
	// The row reads the outcome back with the check.
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{Installed: true, Version: "1.6.15", Line: "1.6", LatestOnLine: "1.6.20", UpdateAvailable: true}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodGet, roundcubeUpdatePath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"state":"read"`) || !strings.Contains(recorder.Body.String(), `"id":"`+accepted.ID+`"`) {
		t.Fatalf("last update: %s", recorder.Body.String())
	}

	// A proved update.
	updateRoundcubeThroughAgent = func(_ *Panel, _ context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
		return transport.RoundcubeUpdateResponse{Updated: true, Proved: true, To: version, From: "1.6.15", Backup: "/var/lib/celikpanel-agent-private/roundcube-backups/roundcube-1.6.15-x.tar.gz"}, nil
	}
	recorder = postWebmailUpdate(t, panel, userID, `{"version":"1.6.20","request_id":"`+mustServiceOperationRequestID(t)+`"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("accepted: %d %s", recorder.Code, recorder.Body.String())
	}
	succeeded, _ := waitForServiceOperation(t, panel, userID, decodeServiceOperationEnvelope(t, recorder).ID, serviceOperationSucceeded)
	if !strings.Contains(string(succeeded.Result), `"outcome":"updated"`) {
		t.Fatalf("succeeded row: %s", succeeded.Result)
	}
	var details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "webmail.update:1.6.15->1.6.20" || !strings.Contains(details, `"from":"1.6.15"`) || !strings.Contains(details, `"to":"1.6.20"`) || !strings.Contains(details, "roundcube-backups") {
		t.Fatalf("audit = %q %s", action, details)
	}
}

func TestAnInterruptedWebmailUpdateIsReconciledReadOnly(t *testing.T) {
	panel, _, _, _ := newServiceStateAtomicPanel(t)
	restoreRead := readRoundcubeUpdateFromAgent
	t.Cleanup(func() { readRoundcubeUpdateFromAgent = restoreRead })
	version := "1.6.15"
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{Installed: true, Version: version}, nil
	}
	result, failure := panel.reconcileInterruptedWebmailUpdate(context.Background(), "1.6.20")
	if failure == nil || failure.Code != "webmail_update_interrupted" || result["outcome"] != webmailUpdateOutcomeInterrupted {
		t.Fatalf("not updated: %+v %+v", result, failure)
	}
	version = "1.6.20"
	if result, failure := panel.reconcileInterruptedWebmailUpdate(context.Background(), "1.6.20"); failure != nil || result["outcome"] != webmailUpdateOutcomeUpdated {
		t.Fatalf("updated: %+v %+v", result, failure)
	}
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{}, errors.New("agent unreachable")
	}
	if result, failure := panel.reconcileInterruptedWebmailUpdate(context.Background(), "1.6.20"); failure == nil || failure.Code != "webmail_update_unknown" || result["outcome"] != webmailUpdateOutcomeUnknown {
		t.Fatalf("unknown: %+v %+v", result, failure)
	}
}
