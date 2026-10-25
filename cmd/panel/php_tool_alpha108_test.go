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

// alpha.108 (Boston, Ubuntu 24.04): the Manage card said "PHP-FPM 8.3 is not
// running" for a failed proof, because the view folded the proof into
// "ready". The parts are now carried, and "Check again" proves without
// moving anything.

func bostonBindings() transport.PHPToolBindingsResponse {
	return transport.PHPToolBindingsResponse{
		Versions: []string{"8.3"},
		Tools: []transport.PHPToolBinding{
			{Tool: "phppgadmin", Installed: true, Version: "8.3", SocketReady: true, VersionInstalled: true, ExtensionsPresent: true, Source: "config",
				FPMState: "active", ProofFailed: true, ProofCause: &transport.PHPToolCause{Kind: "check_unreadable", Version: "8.3", Status: 404}},
		},
	}
}

func TestA108PHPToolsViewCarriesTheProofApartFromTheSocket(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubPHPToolBindings(t, bostonBindings(), nil)
	recorder := httptest.NewRecorder()
	panel.handlePHPTools(recorder, httptest.NewRequest(http.MethodGet, phpToolsPath, nil))
	var response phpToolsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Tools) != 1 {
		t.Fatalf("response = %+v", response)
	}
	tool := response.Tools[0]
	if tool.Ready || !tool.SocketReady || tool.FPMState != "active" || !tool.ProofFailed || tool.ProofCause == nil ||
		tool.ProofCause.Kind != "check_unreadable" || tool.ProofCause.Status != 404 {
		t.Fatalf("tool = %+v", tool)
	}
	for _, field := range []string{`"socket_ready":true`, `"fpm_state":"active"`, `"proof_failed":true`, `"kind":"check_unreadable"`} {
		if !strings.Contains(recorder.Body.String(), field) {
			t.Fatalf("%s missing: %s", field, recorder.Body.String())
		}
	}
}

func stubPHPToolProve(t *testing.T, answer func(tool string) (transport.ProvePHPToolResponse, error)) *[]string {
	t.Helper()
	restore := provePHPToolThroughAgent
	t.Cleanup(func() { provePHPToolThroughAgent = restore })
	var calls []string
	provePHPToolThroughAgent = func(_ *Panel, _ context.Context, tool string) (transport.ProvePHPToolResponse, error) {
		calls = append(calls, tool)
		return answer(tool)
	}
	return &calls
}

func TestA108CheckAgainProvesAndSaysTheCause(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	prove := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		panel.handlePHPToolProve(recorder, serviceStateAtomicRequest(http.MethodPost, phpToolProvePath, body, userID))
		return recorder
	}
	failing := true
	calls := stubPHPToolProve(t, func(tool string) (transport.ProvePHPToolResponse, error) {
		if tool == "webmail" {
			return transport.ProvePHPToolResponse{}, errors.New("agent unreachable")
		}
		if failing {
			return transport.ProvePHPToolResponse{Tool: tool, Version: "8.3", Attempts: []transport.PHPToolAttempt{
				{Version: "8.3", Cause: &transport.PHPToolCause{Kind: "not_tool_page", Version: "8.3", Detail: "phpPgAdmin did not answer with its own page (it answered: Welcome to nginx!)"}},
			}}, nil
		}
		return transport.ProvePHPToolResponse{Tool: tool, Version: "8.3", Proved: true}, nil
	})

	recorder := prove(`{"tool":"phppgadmin"}`)
	var answer phpToolProveAnswer
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || answer.Proved || answer.Version != "8.3" || answer.Cause == nil || answer.Cause.Kind != "not_tool_page" {
		t.Fatalf("failed proof: %d %s", recorder.Code, recorder.Body.String())
	}
	failing = false
	recorder = prove(`{"tool":"phppgadmin"}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"proved":true`) || strings.Contains(recorder.Body.String(), `"cause"`) {
		t.Fatalf("proof: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := prove(`{"tool":"webmail"}`); recorder.Code < 500 || !strings.Contains(recorder.Header().Get("Content-Type"), "json") {
		t.Fatalf("agent failure: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, body := range []string{`{"tool":"wordpress"}`, `{}`, `nope`} {
		if recorder := prove(body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, recorder.Code)
		}
	}
	if len(*calls) != 3 {
		t.Fatalf("agent calls = %v", *calls)
	}

	// While another service change runs, nothing is started.
	panel.serviceMutationMu.Lock()
	recorder = prove(`{"tool":"phppgadmin"}`)
	panel.serviceMutationMu.Unlock()
	if recorder.Code != http.StatusConflict || len(*calls) != 3 {
		t.Fatalf("busy: %d %s calls=%v", recorder.Code, recorder.Body.String(), *calls)
	}
}

func TestA108SameVersionBindSaysOnlyTheProofRan(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	stubPHPToolBindings(t, bostonBindings(), nil)
	stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		return transport.BindPHPToolResponse{Tool: tool, Version: version, Previous: "8.3", ProofOnly: true,
			Error: "phpPgAdmin is already set to PHP 8.3 but did not answer as itself there; its web server configuration was not changed", ErrorCode: "PHP_TOOL_PROBE_FAILED",
			Cause: &transport.PHPToolCause{Kind: "check_unreadable", Version: "8.3", Status: 404}}, nil
	})
	recorder := httptest.NewRecorder()
	panel.handlePHPToolBind(recorder, serviceStateAtomicRequest(http.MethodPost, phpToolBindPath, `{"tool":"phppgadmin","version":"8.3"}`, userID))
	var failure phpToolBindFailure
	if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || !failure.ProofOnly || failure.Restored || failure.MutationApplied || failure.Cause == nil || failure.Cause.Kind != "check_unreadable" {
		t.Fatalf("answer: %d %+v", recorder.Code, failure)
	}
}
