package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 third fix round (lab D5): the Agent's real answer to the update's
// start while a site is being moved (kept by cmd/agent's
// real_path_update_refusal_test.go in web/tests/fixtures/real-path) goes
// through the real transport (net/rpc, gob, over a pipe) into the panel's
// real admission; the JSON the page receives for every refusal the admission
// gives before anything starts is kept in panel-update-start-refusals.json,
// and the web test feeds the page exactly that JSON.
// CELIKPANEL_WRITE_REAL_PATH=1 writes the fixture instead of comparing it.

const updateRealPathFixtureDir = "../../web/tests/fixtures/real-path"

// TestTheUpdatesRefusalsReachThePageOnTheRealPath (lab D5): every refusal the
// update's admission gives before anything starts, as the page receives it:
// at once, 409, its code and words; the update was not started.
func TestTheUpdatesRefusalsReachThePageOnTheRealPath(t *testing.T) {
	withSystemUpdateBuild(t)
	var agentRefusals map[string]transport.SystemUpdateStartResponse
	raw, err := os.ReadFile(filepath.Join(updateRealPathFixtureDir, "agent-update-start-refusals.json"))
	if err != nil || json.Unmarshal(raw, &agentRefusals) != nil {
		t.Fatalf("the Agent's real-path fixture: %v", err)
	}
	t.Cleanup(func() {
		siteMoveMu.Lock()
		siteMoveRunning, siteMoveByWorker = false, false
		siteMoveMu.Unlock()
		siteAutoMove = siteAutoMover{}
	})
	moving := func(running, worker bool) {
		siteMoveMu.Lock()
		siteMoveRunning, siteMoveByWorker = running, worker
		siteMoveMu.Unlock()
	}
	cases := []struct {
		name, code string
		setup      func(f systemUpdateTestFixture) func()
		agentStart bool // the Agent's admission was asked (and refused)
	}{
		{"site_auto_move_running", "PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING", func(systemUpdateTestFixture) func() {
			moving(true, true)
			return func() { moving(false, false) }
		}, false},
		{"site_move_running", "PANEL_UPDATE_SITE_MOVE_RUNNING", func(systemUpdateTestFixture) func() {
			moving(true, false)
			return func() { moving(false, false) }
		}, false},
		{"agent_site_move_running", "PANEL_UPDATE_SITE_MOVE_RUNNING", func(f systemUpdateTestFixture) func() {
			f.agent.start = agentRefusals["site_move_running"]
			return nil
		}, true},
		{"busy", "PANEL_UPDATE_BUSY", func(f systemUpdateTestFixture) func() {
			f.panel.serviceMutationMu.Lock()
			return f.panel.serviceMutationMu.Unlock
		}, false},
		{"target_changed", "PANEL_UPDATE_TARGET_CHANGED", func(f systemUpdateTestFixture) func() {
			f.agent.check.TargetCommit = strings.Repeat("e", 40)
			return nil
		}, false},
		{"package_manager_busy", "HOST_MUTATION_BUSY", func(f systemUpdateTestFixture) func() {
			f.agent.start = transport.SystemUpdateStartResponse{Error: "the package manager is active", Reason: transport.HostMutationReasonPackageManager}
			return nil
		}, true},
		{"start_refused", "PANEL_UPDATE_START_REFUSED", func(f systemUpdateTestFixture) func() {
			f.agent.start = transport.SystemUpdateStartResponse{Error: "refused by the update service"}
			return nil
		}, true},
	}
	pages := map[string]any{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fixture := newSystemUpdateTestFixture(t)
			if undo := c.setup(fixture); undo != nil {
				defer undo()
			}
			recorder := httptest.NewRecorder()
			fixture.panel.handlePanelUpdateStart(recorder, systemUpdateRequest(http.MethodPost, panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion), roleAdmin))
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusConflict || body["code"] != c.code || body["error"] == "" {
				t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
			}
			fixture.agent.mu.Lock()
			starts := fixture.agent.startCalls
			fixture.agent.mu.Unlock()
			if (starts == 1) != c.agentStart {
				t.Fatalf("the Agent's admission was asked %d times", starts)
			}
			pages[c.name] = map[string]any{"status": recorder.Code, "body": body}
		})
	}
	if t.Failed() {
		return
	}
	path := filepath.Join(updateRealPathFixtureDir, "panel-update-start-refusals.json")
	got, err := json.MarshalIndent(pages, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("CELIKPANEL_WRITE_REAL_PATH") == "1" {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	kept, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want, have any
	if json.Unmarshal(kept, &want) != nil || json.Unmarshal(got, &have) != nil || !reflect.DeepEqual(want, have) {
		t.Fatalf("the JSON the page receives is not the kept one (%s):\n%s", path, got)
	}
}
