package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, fourth fix round (finding 7): a close-again that could not run
// keeps the way it failed apart from the server's own message, so the page
// says its own sentence and quotes the server's words after it. Does not
// build on 336c1da28 (the reason was an English sentence around the
// server's words).
func TestACloseAgainThatCouldNotRunKeepsItsKindApartFromTheServersWords(t *testing.T) {
	restartPanelProcess(t)
	t.Cleanup(func() { restartPanelProcess(t) })
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	saved := closeSideDoorsThroughAgent
	t.Cleanup(func() { closeSideDoorsThroughAgent = saved })
	closeSideDoorsThroughAgent = func(*Panel, context.Context, string, string) (transport.SideDoorsResponse, error) {
		return transport.SideDoorsResponse{}, errors.New("the server is busy with another change")
	}
	recorder := httptest.NewRecorder()
	panel.handleSideDoorsReClose(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/metrics/side-doors/reclose", `{"target":"at"}`, userID))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for sideDoorsRecloseStatus().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	status := sideDoorsRecloseStatus()
	if status.Closed || status.ErrorKind != sideDoorsRecloseNotRun || !strings.Contains(status.Error, "busy with another change") || strings.Contains(status.Error, "side doors step") {
		t.Fatalf("status %+v", status)
	}
}
