package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// docs/SITE-BACKUP-CONTENTS-DESIGN.md: a restore of a backup that holds the
// site's program stops a running program first, starts it again after with
// the Programs tab's gates, and says what happened; nothing of this runs for
// a backup without the program.

// programBackupAgent is the backup Agent of these tests; it records whether
// the program ran while the restore ran.
type programBackupAgent struct {
	*panelBackupV2Agent
	rt              *fakeRuntimeNative
	activeAtRestore []bool
	restoreResp     backupspec.RestoreResponse
	contents        backupspec.ContentsResponse
}

func (a *programBackupAgent) RestoreBackup(req *backupspec.RestoreRequest, resp *backupspec.RestoreResponse) error {
	a.rt.mu.Lock()
	active := false
	for _, unit := range a.rt.units {
		active = active || unit.Active
	}
	a.rt.mu.Unlock()
	a.activeAtRestore = append(a.activeAtRestore, active)
	if err := a.panelBackupV2Agent.RestoreBackup(req, resp); err != nil {
		return err
	}
	if a.restoreResp.Code != "" || a.restoreResp.Error != "" || len(a.restoreResp.Restored) > 0 {
		*resp = a.restoreResp
	}
	return nil
}

func (a *programBackupAgent) BackupContentsV1(_ *backupspec.ListRequest, resp *backupspec.ContentsResponse) error {
	*resp = a.contents
	return nil
}

func attachProgramBackupAgent(t *testing.T, p *Panel, agent *programBackupAgent) {
	t.Helper()
	server := rpc.NewServer()
	if err := server.RegisterName("Agent", agent); err != nil {
		t.Fatalf("register backup agent: %v", err)
	}
	connector := func(ctx context.Context) (*rpc.Client, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		serverConn, clientConn := net.Pipe()
		go server.ServeConn(serverConn)
		return rpc.NewClient(clientConn), nil
	}
	client, err := connector(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p.agentClient = transport.NewReconnectingClientWithContextConnector(client, connector)
	t.Cleanup(func() { _ = client.Close() })
}

type programRestoreFixture struct {
	runtimeFixture
	agent *programBackupAgent
}

// newProgramRestoreFixture: a deployed, running program on domain 9902 (when
// deploy), and a backup Agent whose inspected backup holds areas.
func newProgramRestoreFixture(t *testing.T, deploy bool, areas ...string) programRestoreFixture {
	t.Helper()
	f := newRuntimeFixture(t)
	oldStart := backupProgramStartTimeout
	backupProgramStartTimeout = 300 * time.Millisecond
	t.Cleanup(func() { backupProgramStartTimeout = oldStart })
	if deploy {
		f.offerRuntime(t, "")
		f.create(t)
		wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/deploy", `{"operation_id":"op-backup-01"}`), http.StatusAccepted, "")
		if op := f.waitRuntimeOperation(t, "op-backup-01"); op.State != "succeeded" {
			t.Fatalf("deploy: %+v", op)
		}
	}
	info := backupspec.Info{Name: "files-x.cpbak", Type: backupspec.TypeFiles, Restorable: true, Format: 2}
	for _, area := range areas {
		info.Format = 3
		info.Areas = append(info.Areas, backupspec.Area{Name: area})
	}
	agent := &programBackupAgent{
		panelBackupV2Agent: &panelBackupV2Agent{createOK: true, restoreOK: true, deleteOK: true,
			inspectResp: backupspec.InspectResponse{Success: true, Backup: info}},
		rt: f.rt,
	}
	attachProgramBackupAgent(t, f.panel, agent)
	f.rt.mu.Lock()
	f.rt.calls = nil
	f.rt.mu.Unlock()
	return programRestoreFixture{runtimeFixture: f, agent: agent}
}

func (f programRestoreFixture) restore(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains/9902/backups/restore", strings.NewReader(`{"backup_name":"files-x.cpbak"}`))
	rec := httptest.NewRecorder()
	f.panel.handleRestoreBackup(rec, req)
	return rec
}

func (f programRestoreFixture) controls() []string {
	f.rt.mu.Lock()
	defer f.rt.mu.Unlock()
	var out []string
	for _, call := range f.rt.calls {
		if strings.HasPrefix(call, "control:") {
			out = append(out, call)
		}
	}
	return out
}

func decodeRestoreAnswer(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("answer %q: %v", rec.Body.String(), err)
	}
	return body
}

func programState(body map[string]any) string {
	program, _ := body["program"].(map[string]any)
	state, _ := program["state"].(string)
	return state
}

func errorVars(body map[string]any) map[string]any {
	vars, _ := body["vars"].(map[string]any)
	return vars
}

func TestRestoreStopsTheRunningProgramAndStartsItAgain(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.restoreResp = backupspec.RestoreResponse{Success: true, Restored: []string{"app"}}
	rec := f.restore(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.controls(); strings.Join(got, ",") != "control:stop,control:start" {
		t.Fatalf("controls = %v", got)
	}
	if len(f.agent.activeAtRestore) != 1 || f.agent.activeAtRestore[0] {
		t.Fatalf("the program ran while its folder was replaced: %v", f.agent.activeAtRestore)
	}
	body := decodeRestoreAnswer(t, rec)
	if programState(body) != backupProgramStarted {
		t.Fatalf("answer = %s", rec.Body.String())
	}
	var state string
	if err := f.panel.db.GetDB().QueryRow(`SELECT state FROM runtime_apps WHERE domain_id = 9902`).Scan(&state); err != nil || state != runtimeAppRunning {
		t.Fatalf("record state %q, %v", state, err)
	}
}

func TestRestoreLeavesAProgramTheOwnerStoppedStopped(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	wantStatus(t, f.as(t, 9703, http.MethodPost, runtimePath+"/control", `{"action":"stop"}`), http.StatusOK, "")
	f.rt.mu.Lock()
	f.rt.calls = nil
	f.rt.mu.Unlock()
	rec := f.restore(t)
	if rec.Code != http.StatusOK || programState(decodeRestoreAnswer(t, rec)) != backupProgramNotRunning {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.controls(); len(got) != 0 {
		t.Fatalf("a stopped program was touched: %v", got)
	}
}

func TestRestoreOfABackupWithoutTheProgramLeavesItAlone(t *testing.T) {
	f := newProgramRestoreFixture(t, true)
	rec := f.restore(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.controls(); len(got) != 0 {
		t.Fatalf("the program was touched for a backup without it: %v", got)
	}
	if _, ok := decodeRestoreAnswer(t, rec)["program"]; ok {
		t.Fatalf("an answer without the program names it: %s", rec.Body.String())
	}
}

func TestRestoreIsRefusedWhileAProgramOperationRuns(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.exec(t, `UPDATE runtime_app_operations SET state = 'running' WHERE id = 'op-backup-01'`)
	rec := f.restore(t)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodeBackupProgramBusy) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(f.agent.restoreReqs) != 0 || len(f.controls()) != 0 {
		t.Fatalf("something changed: restores %d, controls %v", len(f.agent.restoreReqs), f.controls())
	}
}

func TestFailedRestoreStartsTheProgramAgainAndSaysSo(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.restoreResp = backupspec.RestoreResponse{Code: backupspec.RestoreCodeLoadReverted, Error: "restore failed"}
	rec := f.restore(t)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeRestoreAnswer(t, rec)
	if body["code"] != errCodeRestoreLoadReverted || errorVars(body)["program"] != backupProgramStarted {
		t.Fatalf("answer = %s", rec.Body.String())
	}
	if got := f.controls(); strings.Join(got, ",") != "control:stop,control:start" {
		t.Fatalf("controls = %v", got)
	}
}

func TestRestoreWhoseRollbackFailedLeavesTheProgramStopped(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.restoreResp = backupspec.RestoreResponse{Code: backupspec.RestoreCodeRollbackFailed, Error: "restore failed"}
	rec := f.restore(t)
	body := decodeRestoreAnswer(t, rec)
	vars := errorVars(body)
	if body["code"] != errCodeRestoreRollbackFailed || vars["program"] != backupProgramLeftStopped || vars["program_reason"] != "files_unknown" {
		t.Fatalf("answer = %s", rec.Body.String())
	}
	if got := f.controls(); strings.Join(got, ",") != "control:stop" {
		t.Fatalf("a program over files in an unknown state was started: %v", got)
	}
}

func TestRestoreWithoutAnAnswerLeavesTheProgramStopped(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.agent.restoreErr = errors.New("connection reset")
	rec := f.restore(t)
	body := decodeRestoreAnswer(t, rec)
	if rec.Code != http.StatusBadGateway || body["code"] != errCodeBackupProgramUnknown || errorVars(body)["program"] != backupProgramLeftStopped {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.controls(); strings.Join(got, ",") != "control:stop" {
		t.Fatalf("controls = %v", got)
	}
}

func TestRestoreLeavesAHeldProgramStoppedAndSaysWhy(t *testing.T) {
	f := newProgramRestoreFixture(t, true, "app")
	f.exec(t, `UPDATE runtime_offerings SET held = 1, held_reason = 'review' WHERE offering_id = 'runtime:program'`)
	rec := f.restore(t)
	body := decodeRestoreAnswer(t, rec)
	program, _ := body["program"].(map[string]any)
	if rec.Code != http.StatusOK || program["state"] != backupProgramLeftStopped || program["reason"] != "held" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got := f.controls(); strings.Join(got, ",") != "control:stop" {
		t.Fatalf("controls = %v", got)
	}
}

func TestRestoreOfAProgramWithoutItsRecordSaysToSetItUp(t *testing.T) {
	f := newProgramRestoreFixture(t, false, "app")
	rec := f.restore(t)
	if rec.Code != http.StatusOK || programState(decodeRestoreAnswer(t, rec)) != backupProgramNotSetUp {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(f.controls()) != 0 {
		t.Fatalf("controls = %v", f.controls())
	}
}

func TestBackupContentsNamesTheSiteProgram(t *testing.T) {
	f := newProgramRestoreFixture(t, true)
	f.agent.contents = backupspec.ContentsResponse{SiteBytes: 12, Areas: []backupspec.Area{{Name: "app", Bytes: 1 << 30, Files: 3}},
		LeftOut: []backupspec.LeftOut{}, BackupFreeBytes: 5 << 30, Complete: true}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/9902/backups/contents", nil)
	rec := httptest.NewRecorder()
	f.panel.handleBackupContents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		SiteBytes int64             `json:"site_bytes"`
		Areas     []backupspec.Area `json:"areas"`
		Program   *struct {
			Kind     string `json:"kind"`
			Deployed bool   `json:"deployed"`
		} `json:"program"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SiteBytes != 12 || len(body.Areas) != 1 || body.Program == nil || body.Program.Kind != "program" || !body.Program.Deployed {
		t.Fatalf("contents = %s", rec.Body.String())
	}
}
