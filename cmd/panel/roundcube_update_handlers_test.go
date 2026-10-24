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

// alpha.107 (D-038 §9): webmail updates. A failed check is unknown; an
// update names a release, runs in its own job and is audited with from, to,
// the backup and the outcome; a put-back is said.

func TestWebmailUpdateIsCheckedUpdatedAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restoreRead, restoreUpdate := readRoundcubeUpdateFromAgent, updateRoundcubeThroughAgent
	t.Cleanup(func() { readRoundcubeUpdateFromAgent, updateRoundcubeThroughAgent = restoreRead, restoreUpdate })
	readRoundcubeUpdateFromAgent = func(*Panel, context.Context) (transport.RoundcubeUpdateCheckResponse, error) {
		return transport.RoundcubeUpdateCheckResponse{}, errors.New("agent unreachable")
	}
	recorder := httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodGet, roundcubeUpdatePath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"state":"unknown"`) {
		t.Fatalf("unknown: %s", recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodPost, roundcubeUpdatePath, `{"version":"latest; rm -rf /"}`, userID))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad version: %d", recorder.Code)
	}
	updateRoundcubeThroughAgent = func(_ *Panel, _ context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
		return transport.RoundcubeUpdateResponse{To: version, From: "1.6.15", RolledBack: true, ErrorCode: "PROOF_FAILED", Error: "webmail did not answer on 1.6.20; the previous 1.6.15 tree was put back byte for byte and answers again"}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodPost, roundcubeUpdatePath, `{"version":"1.6.20"}`, userID))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"rolled_back":true`) {
		t.Fatalf("put back: %d %s", recorder.Code, recorder.Body.String())
	}
	updateRoundcubeThroughAgent = func(_ *Panel, _ context.Context, version string) (transport.RoundcubeUpdateResponse, error) {
		return transport.RoundcubeUpdateResponse{Updated: true, Proved: true, To: version, From: "1.6.15", Backup: "/var/lib/celikpanel-agent-private/roundcube-backups/roundcube-1.6.15-x.tar.gz"}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleRoundcubeUpdate(recorder, serviceStateAtomicRequest(http.MethodPost, roundcubeUpdatePath, `{"version":"1.6.20"}`, userID))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"updated":true`) {
		t.Fatalf("update: %d %s", recorder.Code, recorder.Body.String())
	}
	var action, details string
	if err := database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details); err != nil {
		t.Fatal(err)
	}
	if action != "webmail.update:1.6.15->1.6.20" || !strings.Contains(details, `"from":"1.6.15"`) || !strings.Contains(details, `"to":"1.6.20"`) || !strings.Contains(details, "roundcube-backups") {
		t.Fatalf("audit = %q %s", action, details)
	}
}
