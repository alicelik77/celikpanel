package main

import (
	"encoding/json"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 third fix round (lab D6): the earlier rounds' tests fed the
// panel and the page answers shaped by hand, while the pair showed what the
// real path does with them. These tests run the Agent's real handlers through
// the real transport (net/rpc, gob, over a pipe) and keep the answers the
// panel receives in web/tests/fixtures/real-path/. The panel's own tests
// (cmd/panel/real_path_refusals_test.go) send exactly those answers through
// the same transport into its real handlers and keep the JSON the page
// receives beside them; the web tests feed the page exactly that JSON.
//
// gob does not carry an empty list: the Agent's "units": [] arrives as no
// list, which the page read as "no answer" (D6). The fixture keeps that.
//
// CELIKPANEL_WRITE_REAL_PATH=1 writes the fixtures instead of comparing.

// realPathFixtureDir is web/tests/fixtures/real-path from this package.
const realPathFixtureDir = "../../web/tests/fixtures/real-path"

// realPathAgentClient serves the real Agent on one end of a pipe.
func realPathAgentClient(t *testing.T) *rpc.Client {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", &Agent{}); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	go server.ServeConn(serverConn)
	client := rpc.NewClient(clientConn)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// realPathFixture compares answers (name -> value, as JSON) with the kept
// fixture, or writes it.
func realPathFixture(t *testing.T, file string, answers map[string]any) {
	t.Helper()
	path := filepath.Join(realPathFixtureDir, file)
	got, err := json.MarshalIndent(answers, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CELIKPANEL_WRITE_REAL_PATH") == "1" {
		if err := os.MkdirAll(realPathFixtureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the real path's fixture %s: %v", path, err)
	}
	var want, have any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &have); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("the Agent's answers on the real path are not the kept ones (%s):\n%s", path, got)
	}
}

// TestTheMovesRefusalsOnTheRealPath: each refusal a move gives before
// anything changes, as the panel receives it. Every one carries its code and
// "nothing was moved" (or changed), and no list of programs (D6).
func TestTheMovesRefusalsOnTheRealPath(t *testing.T) {
	const update = "version=1\ntoken=t\noperation=update\nsnapshot=s\n"
	unit := "celikrt-5.service"
	cases := []struct {
		code  string
		setup func(t *testing.T, host *sliceTestHost, root string, req *transport.MoveSiteSliceRequest) func()
	}{
		{"update_running", func(t *testing.T, _ *sliceTestHost, root string, _ *transport.MoveSiteSliceRequest) func() {
			writeRealPathFile(t, filepath.Join(root, "active"), update)
			return nil
		}},
		{"update_finishing", func(t *testing.T, _ *sliceTestHost, root string, _ *transport.MoveSiteSliceRequest) func() {
			writeRealPathFile(t, filepath.Join(root, "scheduler-restore.pending"), update)
			return nil
		}},
		{"update_unreadable", func(t *testing.T, _ *sliceTestHost, root string, _ *transport.MoveSiteSliceRequest) func() {
			if err := os.Remove(root); err != nil {
				t.Fatal(err)
			}
			writeRealPathFile(t, root, "x")
			return nil
		}},
		{"another_move", func(*testing.T, *sliceTestHost, string, *transport.MoveSiteSliceRequest) func() {
			siteMoveInFlight.Store(true)
			return func() { siteMoveInFlight.Store(false) }
		}},
		{"site_busy", func(_ *testing.T, _ *sliceTestHost, _ string, req *transport.MoveSiteSliceRequest) func() {
			mu := siteMutationMutex(req.Site.SiteID)
			mu.Lock()
			return mu.Unlock
		}},
		{"kept_earlier", func(t *testing.T, host *sliceTestHost, _ string, req *transport.MoveSiteSliceRequest) func() {
			keptRecordOf(t, host, []string{"celikrt-20.service"})
			req.Site.SiteID = 9
			return nil
		}},
		{"build", func(_ *testing.T, _ *sliceTestHost, _ string, req *transport.MoveSiteSliceRequest) func() {
			req.ExpectedBuildCommit = strings.Repeat("a", 40)
			return nil
		}},
	}
	answers := map[string]any{}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			host := useSliceTestHost(t)
			root := useUpdateGateRoots(t)
			before := sealedTestUnit(unit, sliceTestHome+"/app")
			writeTestUnit(t, host, unit, before)
			host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
			req := moveRequest(unit)
			if undo := c.setup(t, host, root, req); undo != nil {
				defer undo()
			}
			var resp transport.MoveSiteSliceResponse
			if err := realPathAgentClient(t).Call("Agent.MoveSiteSliceV1", req, &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Done || resp.Reason == nil || resp.Reason.Code != c.code || resp.Error == "" {
				t.Fatalf("%+v", resp)
			}
			if !strings.Contains(resp.Error, "nothing was moved") && !strings.Contains(resp.Error, "nothing was changed") && !strings.Contains(resp.Error, "Nothing new was moved") && c.code != "build" {
				t.Fatalf("the refusal does not say that nothing was moved: %q", resp.Error)
			}
			// The transport's own shape: the Agent's empty list is no list.
			if resp.Units != nil {
				t.Fatalf("gob carried an empty list: %#v", resp.Units)
			}
			if unitText(t, filepath.Join(host.dir, unit)) != before {
				t.Fatal("the refused move changed the unit")
			}
			answers[c.code] = resp
		})
	}
	if !t.Failed() {
		realPathFixture(t, "agent-move-refusals.json", answers)
	}
}

// TestTheTasksRefusalsOnTheRealPath: the tasks part's refusals the worker
// must take as waits: the update's last step, and an Agent of another build
// (lab D6: that one had no code, so the worker marked it a failure).
func TestTheTasksRefusalsOnTheRealPath(t *testing.T) {
	const update = "version=1\ntoken=t\noperation=update\nsnapshot=s\n"
	answers := map[string]any{}
	t.Run("update_finishing", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		root := useUpdateGateRoots(t)
		writeRealPathFile(t, filepath.Join(root, "scheduler-restore.pending"), update)
		var resp transport.SiteTasksMoveResponse
		req := &transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}
		if err := realPathAgentClient(t).Call("Agent.MoveSiteTasksV1", req, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Outcome != transport.SiteTasksRefused || resp.Refusal != "update_finishing" {
			t.Fatalf("%+v", resp)
		}
		if crontab, _ := host.crontab(taskTestAccount); crontab != moveTaskCrontab {
			t.Fatal("the refused tasks move changed the crontab")
		}
		answers["update_finishing"] = resp
	})
	t.Run("build", func(t *testing.T) {
		var resp transport.SiteTasksMoveResponse
		req := &transport.SiteTasksMoveRequest{ExpectedBuildCommit: strings.Repeat("a", 40), CronTenant: transport.CronTenant{SiteID: taskTestSite}}
		if err := realPathAgentClient(t).Call("Agent.MoveSiteTasksV1", req, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Outcome != transport.SiteTasksRefused || resp.Refusal != "build" || !strings.Contains(resp.Error, "build mismatch") {
			t.Fatalf("an Agent of another build: %+v", resp)
		}
		answers["build"] = resp
	})
	if !t.Failed() {
		realPathFixture(t, "agent-tasks-refusals.json", answers)
	}
}

func writeRealPathFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
