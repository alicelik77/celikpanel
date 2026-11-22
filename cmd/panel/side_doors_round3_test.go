package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, third fix round (23 Nov 2026): the third independent review of PR
// #348. These do not build on b6d3c832e.

// restartPanelProcess forgets what this process knew of the last close-again,
// as a Panel restart does.
func restartPanelProcess(t *testing.T) {
	t.Helper()
	sideDoorsRecloseMu.Lock()
	sideDoorsRecloseLast, sideDoorsRecloseLoaded = sideDoorsReclose{}, false
	sideDoorsRecloseMu.Unlock()
}

// Finding 5: the close-again result survives a Panel restart, with its time;
// one that was running when the Panel stopped is said as not known.
func TestTheCloseAgainResultSurvivesAPanelRestart(t *testing.T) {
	restartPanelProcess(t)
	t.Cleanup(func() { restartPanelProcess(t) })
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	saved := closeSideDoorsThroughAgent
	t.Cleanup(func() { closeSideDoorsThroughAgent = saved })
	closeSideDoorsThroughAgent = func(*Panel, context.Context, string, string) (transport.SideDoorsResponse, error) {
		return transport.SideDoorsResponse{State: &transport.SideDoorsState{CheckedAt: "2026-11-23T09:00:00Z", Scope: transport.SideDoorsScopeReClose,
			StockPools: []transport.SideDoorsStockPool{{Version: "8.4", State: transport.SideDoorsStockTurnedOff}}}}, nil
	}
	recorder := httptest.NewRecorder()
	panel.handleSideDoorsReClose(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/metrics/side-doors/reclose", `{"target":"stock_pool","version":"8.4"}`, userID))
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for sideDoorsRecloseStatus().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	get := func() sideDoorsReclose {
		t.Helper()
		recorder := httptest.NewRecorder()
		panel.handleSideDoorsReClose(recorder, serviceStateAtomicRequest(http.MethodGet, "/api/v1/metrics/side-doors/reclose", "", userID))
		var status sideDoorsReclose
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &status) != nil {
			t.Fatalf("read: %d %s", recorder.Code, recorder.Body.String())
		}
		return status
	}
	restartPanelProcess(t)
	if status := get(); !status.Closed || status.Running || status.Unknown || status.Target != "stock_pool" || status.Version != "8.4" || status.At == "" {
		t.Fatalf("after a restart: %+v", status)
	}

	// A run the Panel's restart cut: not known, never "closed" or "still open".
	if started, _ := panel.beginSideDoorsReclose(context.Background(), "at", ""); !started {
		t.Fatal("not started")
	}
	restartPanelProcess(t)
	status := get()
	if status.Running || status.Closed || !status.Unknown || status.Target != "at" || status.At == "" {
		t.Fatalf("a cut run after a restart: %+v", status)
	}
	// A new press is a new run, not refused as one that still runs.
	if started, _ := panel.beginSideDoorsReclose(context.Background(), "at", ""); !started {
		t.Fatal("a cut run still blocks a new one")
	}
	panel.endSideDoorsReclose(context.Background(), true, transport.SideDoorsAtWritten, "")
}
