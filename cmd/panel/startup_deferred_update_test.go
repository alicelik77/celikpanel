package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// a124 lab (5 Nov 2026): Update pressed right after a rollback was admitted on
// an idle host, then the Panel's deferred startup mail work began a server
// change one second later and the updater's read-only preflight refused. The
// deferred work now yields while the update this Panel admitted is queued or
// running, by the Agent's read-only status, and runs again once it ended.
func TestStartupDeferredMailWorkYieldsToAnAdmittedUpdate(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	ctx := context.Background()

	// No update admitted: the readiness read alone decides, the update
	// status is not asked.
	if !fixture.panel.startupDeferredHostIdle(ctx) {
		t.Fatal("idle host without an admitted update was not idle")
	}
	fixture.agent.mu.Lock()
	statusCalls := fixture.agent.statusCalls
	fixture.agent.mu.Unlock()
	if statusCalls != 0 {
		t.Fatalf("update status read without an admitted update: %d", statusCalls)
	}

	// The real start handler remembers the admitted request.
	recorder := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateStart(recorder, systemUpdateRequest(
		http.MethodPost, panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion), roleAdmin,
	))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if id := fixture.panel.updateAdmittedRequest.Load(); id == nil || *id != updateTestRequestID {
		t.Fatalf("admitted request not remembered: %v", id)
	}

	setStatus := func(status transport.SystemUpdateStatusResponse, err error) {
		fixture.agent.mu.Lock()
		fixture.agent.status, fixture.agent.statusErr = status, err
		fixture.agent.mu.Unlock()
	}
	found := func(status string) transport.SystemUpdateStatusResponse {
		return transport.SystemUpdateStatusResponse{Found: true, RequestID: updateTestRequestID, Status: status}
	}
	cases := []struct {
		name   string
		status transport.SystemUpdateStatusResponse
		err    error
		idle   bool
	}{
		{"queued", found("queued"), nil, false},
		{"running", found("running"), nil, false},
		{"status unreadable", transport.SystemUpdateStatusResponse{}, errors.New("agent read failed"), false},
		{"failed, nothing changed", found("failed"), nil, true},
		{"succeeded", found("succeeded"), nil, true},
		{"not found", transport.SystemUpdateStatusResponse{Found: false}, nil, true},
	}
	for _, test := range cases {
		setStatus(test.status, test.err)
		if got := fixture.panel.startupDeferredHostIdle(ctx); got != test.idle {
			t.Fatalf("%s: idle=%v, want %v", test.name, got, test.idle)
		}
	}
	fixture.agent.mu.Lock()
	lastID, starts := fixture.agent.lastStatusID, fixture.agent.startCalls
	fixture.agent.mu.Unlock()
	if lastID != updateTestRequestID || starts != 1 {
		t.Fatalf("status asked for %q, starts=%d: the probe must only read", lastID, starts)
	}

	// The same retry the Panel runs: a yielded attempt changes nothing and
	// the work completes at the next attempt once the update has ended.
	setStatus(found("running"), nil)
	runs := 0
	attempt := 0
	logs := &deferredRetryLog{}
	retry := startupDeferredRetry{
		interval: time.Millisecond, attempts: 3,
		idle: fixture.panel.startupDeferredHostIdle,
		wait: func(context.Context, time.Duration) bool {
			attempt++
			if attempt == 2 {
				setStatus(found("failed"), nil)
			}
			return true
		},
		logf: logs.logf,
	}
	remaining := retry.run(ctx, []startupDeferredTask{{name: "mail filter wiring",
		run: func(context.Context) (string, error) { runs++; return "", nil }}})
	if len(remaining) != 0 || runs != 1 {
		t.Fatalf("remaining=%v runs=%d lines=%q", remaining, runs, logs.snapshot())
	}

	// An admission older than the active window is not asked about.
	fixture.panel.markPanelUpdateAdmitted(panelUpdateNow().Add(-2 * panelUpdateActiveWindow))
	setStatus(found("running"), nil)
	if !fixture.panel.startupDeferredHostIdle(ctx) {
		t.Fatal("an admission outside the active window still held the startup work")
	}
}
