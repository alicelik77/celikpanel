package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// alpha.130 lab (release/alpha130 36e09fb38): during a long "starting" boot
// POST /api/v1/domains/create answered a bare 500 INTERNAL. A change refused
// before dispatch because systemd still reports the host as starting is an
// unmet prerequisite that clears by itself: 503 HOST_STARTING with the
// reason, who acts and how it resumes.

func TestHostStillStartingIsACodedGuidedAnswer(t *testing.T) {
	refusal := errors.New(`detect host platform: systemd is not ready: systemd state "starting"`)
	cases := []error{
		// As the Agent RPC policy wraps it.
		fmt.Errorf("%w: Agent.CreateSite: %w", errAgentRPCPlatformIdentityUnavailable, refusal),
		// As a handler wraps it again on its way out.
		fmt.Errorf("create site: %w", fmt.Errorf("%w: Agent.CreateSite: %w", errAgentRPCPlatformIdentityUnavailable, refusal)),
		// Only the text kept.
		fmt.Errorf("create site: %v", fmt.Errorf("%w: %w", errAgentRPCPlatformIdentityUnavailable, refusal)),
		errors.New(`agent RPC platform identity is unavailable: systemd is not ready: systemd state "initializing"`),
	}
	for _, err := range cases {
		for name, write := range map[string]func(http.ResponseWriter, error){
			"writeServerError": writeServerError,
			"writeAgentError":  func(w http.ResponseWriter, err error) { writeAgentError(w, err, "create site") },
		} {
			recorder := httptest.NewRecorder()
			write(recorder, err)
			var body map[string]any
			_ = json.Unmarshal(recorder.Body.Bytes(), &body)
			if recorder.Code != http.StatusServiceUnavailable || body["code"] != errCodeHostStarting ||
				recorder.Header().Get("Retry-After") == "" {
				t.Fatalf("%s(%v) = %d %s", name, err, recorder.Code, recorder.Body.String())
			}
			message, _ := body["error"].(string)
			for _, part := range []string{"still starting", "nothing was changed", "Nobody needs to act", "systemctl is-system-running"} {
				if !strings.Contains(message, part) {
					t.Fatalf("%s: message lacks %q: %s", name, part, message)
				}
			}
		}
	}
}

func TestOtherSystemdStatesAreNotHostStarting(t *testing.T) {
	for _, err := range []error{
		errors.New(`systemd is not ready: systemd state "stopping"`),
		errors.New(`systemd is not ready: systemd state "maintenance"`),
		errors.New("a database row could not be read"),
		nil,
	} {
		if hostStillStartingError(err) {
			t.Fatalf("%v read as still starting", err)
		}
	}
	recorder := httptest.NewRecorder()
	writeServerError(recorder, errors.New(`systemd is not ready: systemd state "stopping"`))
	if recorder.Code == http.StatusServiceUnavailable && strings.Contains(recorder.Body.String(), errCodeHostStarting) {
		t.Fatalf("stopping answered as starting: %s", recorder.Body.String())
	}
}
