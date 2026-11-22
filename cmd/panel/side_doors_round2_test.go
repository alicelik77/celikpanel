package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R1, second fix round (22 Nov 2026): the second independent review of
// PR #348. These do not build on 193b87001.

// --- B4: "close it again" is that one door, apart from the browser's request ---

func TestClosingAgainRunsApartFromTheRequestOnceAndOnlyForItsDoor(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	// What the step said at the last start: a tool line and the owner's undo.
	start := transport.SideDoorsState{CheckedAt: "2026-11-22T08:00:00Z",
		Tools:      []transport.SideDoorsTool{{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolFailed}},
		StockPools: []transport.SideDoorsStockPool{{Version: "8.4", Path: "/etc/php/8.4/fpm/pool.d/www.conf", State: transport.SideDoorsStockOwnerRestored}, {Version: "8.3", State: transport.SideDoorsStockInUse}}}
	panel.recordSideDoorsState(context.Background(), &start)

	release := make(chan struct{})
	var mu sync.Mutex
	var calls []string
	saved := closeSideDoorsThroughAgent
	t.Cleanup(func() { closeSideDoorsThroughAgent = saved })
	closeSideDoorsThroughAgent = func(_ *Panel, ctx context.Context, reclose, version string) (transport.SideDoorsResponse, error) {
		mu.Lock()
		calls = append(calls, reclose+":"+version)
		mu.Unlock()
		<-release
		if ctx.Err() != nil {
			t.Error("the run was cancelled with the browser's request")
		}
		return transport.SideDoorsResponse{State: &transport.SideDoorsState{CheckedAt: "2026-11-22T09:00:00Z", Scope: transport.SideDoorsScopeReClose,
			StockPools: []transport.SideDoorsStockPool{{Version: "8.4", Path: "/etc/php/8.4/fpm/pool.d/www.conf", State: transport.SideDoorsStockTurnedOff}}}}, nil
	}
	post := func() *httptest.ResponseRecorder {
		ctx, cancel := context.WithCancel(context.Background())
		request := serviceStateAtomicRequest(http.MethodPost, "/api/v1/metrics/side-doors/reclose", `{"target":"stock_pool","version":"8.4"}`, userID)
		recorder := httptest.NewRecorder()
		panel.handleSideDoorsReClose(recorder, request.WithContext(context.WithValue(ctx, callerKey, currentCaller(request))))
		cancel() // the browser leaves
		return recorder
	}
	if first := post(); first.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", first.Code, first.Body.String())
	}
	if second := post(); second.Code != http.StatusAccepted {
		t.Fatalf("second press: %d %s", second.Code, second.Body.String())
	}
	if status := sideDoorsRecloseStatus(); !status.Running {
		t.Fatalf("status %+v: it runs on", status)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for sideDoorsRecloseStatus().Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	status := sideDoorsRecloseStatus()
	if status.Running || !status.Closed || status.Target != "stock_pool" || status.Version != "8.4" {
		t.Fatalf("status %+v", status)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 || calls[0] != "stock_pool:8.4" {
		t.Fatalf("calls %v: one run, for that door only", calls)
	}
	var kept transport.SideDoorsState
	if err := json.Unmarshal([]byte(panel.setting(context.Background(), settingSideDoorsState)), &kept); err != nil {
		t.Fatal(err)
	}
	if len(kept.Tools) != 1 || len(kept.StockPools) != 2 || kept.StockPools[0].State != transport.SideDoorsStockTurnedOff || kept.StockPools[1].State != transport.SideDoorsStockInUse {
		t.Fatalf("kept %+v: only that door's line changes", kept)
	}
	recorder := httptest.NewRecorder()
	panel.handleSideDoorsReClose(recorder, serviceStateAtomicRequest(http.MethodGet, "/api/v1/metrics/side-doors/reclose", "", userID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status read: %d", recorder.Code)
	}
}

// --- B7: the states are read again without a Panel restart ---

func TestAReadAgainShowsAToolDownAndLetsALineGoByItself(t *testing.T) {
	step := transport.SideDoorsState{CheckedAt: "2026-11-22T08:00:00Z",
		Tools:      []transport.SideDoorsTool{{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolMoved}},
		StockPools: []transport.SideDoorsStockPool{{Version: "8.4", State: transport.SideDoorsStockInUse, UsedBy: []string{"/etc/nginx/sites-enabled/a.conf"}}, {Version: "8.3", State: transport.SideDoorsStockTurnedOff}},
		At:         &transport.SideDoorsAt{State: transport.SideDoorsAtWritten}}
	read := transport.SideDoorsState{CheckedAt: "2026-11-22T09:00:00Z", Scope: transport.SideDoorsScopeRead,
		Tools:      []transport.SideDoorsTool{{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolDown}},
		StockPools: []transport.SideDoorsStockPool{{Version: "8.4"}, {Version: "8.3", State: transport.SideDoorsStockOwnerRestored}},
		At:         &transport.SideDoorsAt{State: transport.SideDoorsAtOwnerRemoved}}
	merged := mergeSideDoorsState(step, read)
	raw, _ := json.Marshal(merged)
	kinds := map[string]bool{}
	for _, problem := range sideDoorsProblems(string(raw)) {
		kinds[problem.Kind] = true
	}
	for _, problem := range siteAtProblems(merged) {
		kinds[problem.Kind] = true
	}
	for _, want := range []string{"tool_down", "stock_pool_in_use", "stock_pool_owner_restored", "at_owner_removed"} {
		if !kinds[want] {
			t.Fatalf("kinds %v: %s missing after a read again", kinds, want)
		}
	}
	// The administrator fixed the tool in its Manage window: the line goes.
	fixed := read
	fixed.Tools = []transport.SideDoorsTool{{Tool: "webmail", Version: "8.4", State: transport.SideDoorsToolOwnAccount}}
	raw, _ = json.Marshal(mergeSideDoorsState(merged, fixed))
	for _, problem := range sideDoorsProblems(string(raw)) {
		if problem.Kind == "tool_down" {
			t.Fatal("the tool answers again but its line stayed")
		}
	}
}

func TestAMoveInAToolsManageWindowReadsTheStatesAgain(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	stubPHPToolBindings(t, frankfurtBindings(), nil)
	stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		return transport.BindPHPToolResponse{Tool: tool, Version: version, Bound: true}, nil
	})
	reads := 0
	readSideDoorsAfterMove = func(*Panel) { reads++ }
	recorder := httptest.NewRecorder()
	panel.handlePHPToolBind(recorder, serviceStateAtomicRequest(http.MethodPost, phpToolBindPath, `{"tool":"phppgadmin","version":"8.3"}`, userID))
	if reads != 1 {
		t.Fatalf("reads %d after a move (answer %d)", reads, recorder.Code)
	}
}
