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

// alpha.130: SSH password sign-in. The change is validated, passed on once,
// audited, and every Agent outcome is said as what it is.

func TestSSHPasswordLoginChangeIsValidatedPassedOnAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	restore := setSSHPasswordLoginThroughAgent
	t.Cleanup(func() { setSSHPasswordLoginThroughAgent = restore })
	var calls atomic.Int32
	var gotAction, gotSHA string
	answer := transport.SetSSHPasswordLoginResponse{Outcome: transport.SSHPasswordLoginTurnedOff, Reloaded: true, PasswordAuthentication: "no", KbdInteractiveAuthentication: "no"}
	var agentErr error
	setSSHPasswordLoginThroughAgent = func(_ *Panel, _ context.Context, action, sha string) (transport.SetSSHPasswordLoginResponse, error) {
		calls.Add(1)
		gotAction, gotSHA = action, sha
		return answer, agentErr
	}
	for _, body := range []string{`{"action":"maybe"}`, `{"action":"turn_off","expected_drop_in_sha256":"nope"}`, `not json`, `{}`, `{"action":"turn_off","extra":1}`} {
		recorder := httptest.NewRecorder()
		panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodPost, sshPasswordLoginPath, body, userID))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, recorder.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("the Agent was asked about an invalid request")
	}
	recorder := httptest.NewRecorder()
	panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodPost, sshPasswordLoginPath, `{"action":"turn_off"}`, userID))
	if recorder.Code != http.StatusOK || gotAction != "turn_off" || gotSHA != "" || !strings.Contains(recorder.Body.String(), `"outcome":"turned_off"`) {
		t.Fatalf("turn off: %d %s", recorder.Code, recorder.Body.String())
	}
	var action string
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "security.ssh_password.off" {
		t.Fatalf("audit %q", action)
	}

	// A refusal keeps the Agent's reason; a rollback says so.
	answer = transport.SetSSHPasswordLoginResponse{Outcome: transport.SSHPasswordLoginRefused, Code: "no_key_account", Error: "no key"}
	recorder = httptest.NewRecorder()
	panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodPost, sshPasswordLoginPath, `{"action":"turn_off"}`, userID))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"reason":"no_key_account"`) || !strings.Contains(recorder.Body.String(), "SSH_PASSWORD_REFUSED") {
		t.Fatalf("refused: %d %s", recorder.Code, recorder.Body.String())
	}
	answer = transport.SetSSHPasswordLoginResponse{Outcome: transport.SSHPasswordLoginRolledBack, Code: "reload_failed", Detail: "Job for ssh.service failed"}
	recorder = httptest.NewRecorder()
	panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodPost, sshPasswordLoginPath, `{"action":"turn_off"}`, userID))
	if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "SSH_PASSWORD_ROLLED_BACK") || !strings.Contains(recorder.Body.String(), `"outcome":"rolled_back"`) {
		t.Fatalf("rolled back: %d %s", recorder.Code, recorder.Body.String())
	}
	if err := database.GetDB().QueryRow(`SELECT action FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(action, "security.ssh_password.failed") {
		t.Fatalf("audit %q", action)
	}
	// No answer from the Agent: not known, never "done".
	agentErr = errors.New("agent unreachable")
	recorder = httptest.NewRecorder()
	panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodPost, sshPasswordLoginPath, `{"action":"turn_on"}`, userID))
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "SSH_PASSWORD_UNKNOWN") {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body.String())
	}
	// Not for a customer.
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, sshPasswordLoginPath, strings.NewReader(`{"action":"turn_off"}`))
	panel.handleSSHPasswordLogin(recorder, request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: 9, Role: roleCustomer})))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("customer: %d", recorder.Code)
	}
}

func TestSSHPasswordLoginReadAnswersEmptyListsAsArrays(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restore := readSSHPasswordLoginFromAgent
	t.Cleanup(func() { readSSHPasswordLoginFromAgent = restore })
	readSSHPasswordLoginFromAgent = func(*Panel, context.Context) (transport.SSHPasswordLoginStatusResponse, error) {
		return transport.SSHPasswordLoginStatusResponse{State: transport.SSHPasswordLoginStateRead, PasswordAuthentication: "yes"}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleSSHPasswordLogin(recorder, serviceStateAtomicRequest(http.MethodGet, sshPasswordLoginPath, "", userID))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"deciders":[]`) || !strings.Contains(recorder.Body.String(), `"key_accounts":[]`) {
		t.Fatalf("read: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSSHPasswordLoginChangeAlwaysNeedsARequestIdentity(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, sshPasswordLoginPath, strings.NewReader(`{}`))
	route, protected := operationStepRouteFor(request)
	if !protected || route.optional || route.pattern != sshPasswordLoginPath {
		t.Fatalf("route %+v %v", route, protected)
	}
	if _, protected := operationStepRouteFor(httptest.NewRequest(http.MethodGet, sshPasswordLoginPath, nil)); protected {
		t.Fatal("a read needs no identity")
	}
}
