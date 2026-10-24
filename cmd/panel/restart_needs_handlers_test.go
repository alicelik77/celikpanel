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

// alpha.107 (D-038 §10): a failed read is unknown, never "nothing to restart".
func TestRestartsReadIsUnknownWhenTheAgentCannotAnswer(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	restore := readRestartNeedsFromAgent
	t.Cleanup(func() { readRestartNeedsFromAgent = restore })
	readRestartNeedsFromAgent = func(*Panel, context.Context) (transport.RestartNeedsResponse, error) {
		return transport.RestartNeedsResponse{}, errors.New("agent unreachable")
	}
	recorder := httptest.NewRecorder()
	panel.handleRestartNeeds(recorder, serviceStateAtomicRequest(http.MethodGet, restartNeedsPath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"state":"unknown"`) || strings.Contains(recorder.Body.String(), `"services"`) {
		t.Fatalf("unknown: %s", recorder.Body.String())
	}
	readRestartNeedsFromAgent = func(*Panel, context.Context) (transport.RestartNeedsResponse, error) {
		return transport.RestartNeedsResponse{ServicesKnown: true, RebootKnown: true, RebootRequired: true}, nil
	}
	recorder = httptest.NewRecorder()
	panel.handleRestartNeeds(recorder, serviceStateAtomicRequest(http.MethodGet, restartNeedsPath, "", userID))
	if !strings.Contains(recorder.Body.String(), `"services":[]`) || !strings.Contains(recorder.Body.String(), `"reboot_required":true`) {
		t.Fatalf("read: %s", recorder.Body.String())
	}
}
