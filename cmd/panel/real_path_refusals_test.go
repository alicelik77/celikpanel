package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 third fix round (lab D6): the answers the Agent's real
// handlers give on the real transport (kept by cmd/agent's
// real_path_refusals_test.go in web/tests/fixtures/real-path) are sent
// through the same transport (net/rpc, gob, over a pipe) into the panel's
// real handlers; the JSON the page receives is kept beside them, and the web
// tests feed the page exactly that JSON.
//
// CELIKPANEL_WRITE_REAL_PATH=1 writes the panel's fixtures instead of
// comparing them.

const panelRealPathFixtureDir = "../../web/tests/fixtures/real-path"

func readRealPathFixture(t *testing.T, file string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(panelRealPathFixtureDir, file))
	if err != nil {
		t.Fatalf("the Agent's real-path fixture: %v", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func panelRealPathFixture(t *testing.T, file string, answers map[string]any) {
	t.Helper()
	path := filepath.Join(panelRealPathFixtureDir, file)
	got, err := json.MarshalIndent(answers, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CELIKPANEL_WRITE_REAL_PATH") == "1" {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the page's real-path fixture %s: %v", path, err)
	}
	var want, have any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &have); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("the JSON the page receives is not the kept one (%s):\n%s", path, got)
	}
}

// realPathMoveAgent answers the two calls of a press with the Agent's kept
// real answers.
type realPathMoveAgent struct {
	mu          sync.Mutex
	slice       transport.MoveSiteSliceResponse
	tasks       transport.SiteTasksMoveResponse
	sliceCalls  int
	tasksCalled int
}

func (a *realPathMoveAgent) MoveSiteSliceV1(_ *transport.MoveSiteSliceRequest, resp *transport.MoveSiteSliceResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sliceCalls++
	*resp = a.slice
	return nil
}

func (a *realPathMoveAgent) MoveSiteTasksV1(_ *transport.SiteTasksMoveRequest, resp *transport.SiteTasksMoveResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tasksCalled++
	*resp = a.tasks
	return nil
}

// useRealPathMoveAgent routes the press's Agent calls over a real rpc pipe.
func useRealPathMoveAgent(t *testing.T) *realPathMoveAgent {
	t.Helper()
	agent := &realPathMoveAgent{}
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	go server.ServeConn(serverConn)
	client := rpc.NewClient(clientConn)
	oldSlice, oldTasks, oldUpdate := moveSiteSliceCall, moveSiteTasksCall, siteMoveUpdateInProgress
	t.Cleanup(func() {
		moveSiteSliceCall, moveSiteTasksCall, siteMoveUpdateInProgress = oldSlice, oldTasks, oldUpdate
		_ = client.Close()
	})
	siteMoveUpdateInProgress = func(*Panel, context.Context) bool { return false }
	moveSiteSliceCall = func(_ *Panel, _ context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		var resp transport.MoveSiteSliceResponse
		err := client.Call("Agent.MoveSiteSliceV1", req, &resp)
		return resp, err
	}
	moveSiteTasksCall = func(_ *Panel, _ context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		var resp transport.SiteTasksMoveResponse
		err := client.Call(method, req, &resp)
		return resp, err
	}
	return agent
}

// TestAMovesRefusalReachesThePageOnTheRealPath (lab D6): each refusal the
// Agent gives before anything changes reaches the page with its code, its
// words and a list of programs (empty), so the page says why; before, the
// list arrived as null and the page said "the result is not known".
func TestAMovesRefusalReachesThePageOnTheRealPath(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var refusals map[string]transport.MoveSiteSliceResponse
	readRealPathFixture(t, "agent-move-refusals.json", &refusals)
	if len(refusals) != 7 {
		t.Fatalf("the Agent's kept refusals: %v", sortedKeys(refusals))
	}
	agent := useRealPathMoveAgent(t)
	handler := siteLimitsMux(f)
	pages := map[string]any{}
	for i, code := range sortedKeys(refusals) {
		agent.slice = refusals[code]
		agent.tasks = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
		w := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move",
			`{"request_id":"realpath-move-`+itoa(i)+`"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", code, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		units, isList := body["units"].([]any)
		reason, _ := body["reason"].(map[string]any)
		if !isList || len(units) != 0 || body["done"] != false || reason["code"] != code {
			t.Fatalf("%s: the page receives %s", code, w.Body.String())
		}
		pages[code] = body
	}
	panelRealPathFixture(t, "panel-move-refusals.json", pages)
}

// TestTheWorkerWaitsForEveryRefusalThatClearsByItselfOnTheRealPath (lab D6):
// the worker classifies the same real answers: a refusal that clears by
// itself is a wait (a kept record: the wait for it), never a failure or an
// unknown result; also when the site's tasks part refuses for the update's
// last step or for an Agent of another build, with or without programs.
func TestTheWorkerWaitsForEveryRefusalThatClearsByItselfOnTheRealPath(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	ctx := context.Background()
	sites, err := f.panel.siteLimitSites(ctx, 9902)
	if err != nil || len(sites) != 1 || len(sites[0].Units) == 0 {
		t.Fatalf("the site: %+v %v", sites, err)
	}
	var refusals map[string]transport.MoveSiteSliceResponse
	readRealPathFixture(t, "agent-move-refusals.json", &refusals)
	var tasksRefusals map[string]transport.SiteTasksMoveResponse
	readRealPathFixture(t, "agent-tasks-refusals.json", &tasksRefusals)
	agent := useRealPathMoveAgent(t)
	want := func(code string) string {
		if code == transport.SiteMoveRefusalKept {
			return siteAutoWaitKept
		}
		return siteAutoWait
	}
	for _, code := range sortedKeys(refusals) {
		agent.slice = refusals[code]
		agent.tasks = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
		resp, tasks, outcome := f.panel.pressSiteMove(ctx, sites[0], "realpath-worker-"+code)
		if mark := classifySiteMove(resp, tasks, outcome); mark.State != want(code) || mark.Reason != code {
			t.Fatalf("the programs' %s: the worker marks %+v", code, mark)
		}
	}
	// The tasks part refuses first; a programs' part that ran would fail.
	notFinished := transport.MoveSiteSliceResponse{Error: "x", Reason: &transport.SiteMoveReason{Code: "not_finished"}}
	for _, code := range sortedKeys(tasksRefusals) {
		for _, programs := range []bool{true, false} {
			site := sites[0]
			if !programs {
				site.Units = nil
			}
			agent.slice, agent.tasks = notFinished, tasksRefusals[code]
			resp, tasks, outcome := f.panel.pressSiteMove(ctx, site, "realpath-tasks-"+code)
			if mark := classifySiteMove(resp, tasks, outcome); mark.State != siteAutoWait || mark.Reason != code {
				t.Fatalf("the tasks' %s (programs %v): the worker marks %+v", code, programs, mark)
			}
		}
	}
	if agent.sliceCalls != len(refusals) {
		t.Fatalf("a programs' part ran after the tasks part refused (%d calls)", agent.sliceCalls)
	}
}
