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

// alpha.107 (D-038 §9): an installed Node.js line says its newer patch and
// the sites still on the old one; a site moves only on request, with proof,
// and is put back when it does not answer.

func TestNodePatchesNameTheSitesStillOnTheOldPatch(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	seedNodeSite(t, panel, userID, nil)
	restore := readNodePatchesFromAgent
	t.Cleanup(func() { readNodePatchesFromAgent = restore })
	readNodePatchesFromAgent = func(*Panel, context.Context) (transport.NodePatchesResponse, error) {
		return transport.NodePatchesResponse{Patches: []transport.NodePatch{{Installed: "22.14.0", Line: "22", Newest: "22.23.4", Newer: true}}}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleNodePatches(recorder, serviceStateAtomicRequest(http.MethodGet, "/api/v1/runtimes/node/patches", "", userID))
	body := recorder.Body.String()
	if !strings.Contains(body, `"newest":"22.23.4"`) || !strings.Contains(body, `"name":"app.example"`) {
		t.Fatalf("patches = %s", body)
	}
}

func TestASiteMovesToANewerPatchWithProofOrIsPutBack(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	seedNodeSite(t, panel, userID, nil)
	restoreMove, restoreProve := transitionHostingForMove, proveAppAnswers
	t.Cleanup(func() { transitionHostingForMove, proveAppAnswers = restoreMove, restoreProve })
	var moves []string
	transitionHostingForMove = func(_ *Panel, _ context.Context, _ int, settings hostingSettings) error {
		moves = append(moves, settings.RuntimeVersion)
		return nil
	}
	proveAppAnswers = func(context.Context, int) error { return errors.New("connection refused") }
	recorder := httptest.NewRecorder()
	panel.handleDomainAppNodeVersion(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9102/app/node-version", `{"version":"22.23.4"}`, userID), 9102)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"rolled_back":true`) || strings.Join(moves, ",") != "22.23.4,22.14.0" {
		t.Fatalf("put back: %d %s moves=%v", recorder.Code, recorder.Body.String(), moves)
	}
	moves = nil
	proveAppAnswers = func(context.Context, int) error { return nil }
	recorder = httptest.NewRecorder()
	panel.handleDomainAppNodeVersion(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9102/app/node-version", `{"version":"22.23.4"}`, userID), 9102)
	if recorder.Code != http.StatusOK || strings.Join(moves, ",") != "22.23.4" {
		t.Fatalf("move: %d %s", recorder.Code, recorder.Body.String())
	}
	var action string
	database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action)
	if action != "site.node_version:app.example:22.14.0->22.23.4" {
		t.Fatalf("audit = %q", action)
	}
	recorder = httptest.NewRecorder()
	panel.handleDomainAppNodeVersion(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/9102/app/node-version", `{"version":"latest"}`, userID), 9102)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad version: %d", recorder.Code)
	}
}
