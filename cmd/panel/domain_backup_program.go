package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A site backup's program (docs/SITE-BACKUP-CONTENTS-DESIGN.md). A backup
// that holds the site's "app" area replaces the program's folder; a running
// program keeps the old folder open through its sandbox bind, so the restore
// stops it first with the Programs tab's own stop and starts it again after,
// with the Programs tab's own start and gates, and says what happened. A
// program the owner had stopped stays stopped (D-022). Nothing of this runs
// for a backup without the program.
//
// Programı içeren yedeğin geri yüklemesi programı önce durdurur, sonra
// Programlar sekmesinin kendi başlatmasıyla başlatır ve sonucu söyler.

const (
	errCodeBackupProgramBusy    = "BACKUP_RESTORE_PROGRAM_BUSY"
	errCodeBackupProgramUnknown = "BACKUP_RESTORE_PROGRAM_UNKNOWN"
	errCodeBackupProgramStop    = "BACKUP_RESTORE_PROGRAM_STOP_FAILED"
)

// Program outcomes of a restore.
const (
	backupProgramStarted     = "started"      // started and listening on its port
	backupProgramNotStarted  = "not_started"  // the start failed or it did not listen in time
	backupProgramLeftStopped = "left_stopped" // a gate refused the start, or the files are not in a known state
	backupProgramNotRunning  = "not_running"  // it was not running before; left as it was
	backupProgramNotSetUp    = "not_set_up"   // no program record on the site; only its files came back
	backupProgramUnknown     = "unknown"      // the server did not answer the start
)

// backupProgramOutcome is the program part of a restore's answer.
type backupProgramOutcome struct {
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// backupProgramRestore is what the restore does with the site's program.
type backupProgramRestore struct {
	app     runtimeApp
	has     bool
	spec    transport.RuntimeAppSpec
	running bool
	stopped bool
}

// backupStartTimeout bounds the wait for a restarted program to listen.
var backupProgramStartTimeout = 2 * time.Minute

// backupHoldsArea reports whether a backup holds a site-home area.
func backupHoldsArea(info backupspec.Info, area string) bool {
	for _, held := range info.Areas {
		if held.Name == area {
			return true
		}
	}
	return false
}

// prepareProgramRestore reads the site's program before anything changes. A
// refusal is written to w and ok is false: a program operation still runs,
// or the program's state cannot be read (the restore must not guess whether
// to start it again).
func (p *Panel) prepareProgramRestore(w http.ResponseWriter, ctx context.Context, d backupDomain) (backupProgramRestore, bool) {
	a, has, err := p.runtimeAppForDomain(ctx, d.ID)
	if err != nil {
		writeServerError(w, err)
		return backupProgramRestore{}, false
	}
	plan := backupProgramRestore{app: a, has: has}
	if !has {
		return plan, true
	}
	var running int
	if err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM runtime_app_operations WHERE app_id = ? AND state = 'running'`, a.ID).Scan(&running); err != nil {
		writeServerError(w, err)
		return backupProgramRestore{}, false
	}
	if running > 0 {
		writeCodedError(w, http.StatusConflict, errCodeBackupProgramBusy,
			"The site's program is being deployed or removed right now, so the restore was not started and nothing changed. Wait until that ends on the Programs tab, then restore again.", "")
		return backupProgramRestore{}, false
	}
	if !a.Deployed {
		return plan, true
	}
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return backupProgramRestore{}, false
	}
	o, offered := offerings[a.OfferingID]
	plan.spec = a.spec(o, offered)
	status, err := p.runtimeAppNativeOps().status(ctx, transport.RuntimeAppStatusRequest{Target: a.target(), Spec: plan.spec})
	if err != nil || status.Error != "" {
		writeCodedError(w, http.StatusBadGateway, errCodeBackupProgramUnknown,
			"Whether the site's program runs could not be read from the server, so the restore was not started and nothing changed. Try again in a moment; if it keeps failing, open the Programs tab.", "")
		return backupProgramRestore{}, false
	}
	switch status.ActiveState {
	case "active", "activating", "reloading":
		plan.running = true
	}
	return plan, true
}

// stop stops a running program before its folder is replaced. A refusal is
// written to w and ok is false; nothing else has changed then.
func (plan *backupProgramRestore) stop(w http.ResponseWriter, ctx context.Context, p *Panel) bool {
	if !plan.running {
		return true
	}
	resp, err := p.runtimeAppNativeOps().control(ctx, transport.RuntimeAppControlRequest{Target: plan.app.target(), Action: "stop"})
	if err != nil {
		writeCodedError(w, http.StatusBadGateway, errCodeBackupProgramUnknown,
			"The server did not answer when the site's program was being stopped, so the restore was not started. Whether the program still runs is not known: check the Programs tab and start it there if it is stopped.", "")
		return false
	}
	if !resp.Done {
		writeCodedErrorDetails(w, http.StatusConflict, errCodeBackupProgramStop,
			"The site's program could not be stopped, so the restore was not started and nothing changed. The server's reason is below; check the program on the Programs tab.", "",
			[]string{boundedAgentDiagnostic(resp.Error)})
		return false
	}
	plan.stopped = true
	_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET state = ?, updated_at = datetime('now') WHERE id = ?`, runtimeAppStopped, plan.app.ID)
	return true
}

// finish starts the program again when this restore stopped it, and says
// what happened. filesUnknown is a restore whose putting back failed: the
// program's files are not in a known state and it is left stopped.
func (plan *backupProgramRestore) finish(ctx context.Context, p *Panel, filesUnknown bool) *backupProgramOutcome {
	if !plan.has {
		return &backupProgramOutcome{State: backupProgramNotSetUp}
	}
	if !plan.stopped {
		return &backupProgramOutcome{State: backupProgramNotRunning}
	}
	if filesUnknown {
		return &backupProgramOutcome{State: backupProgramLeftStopped, Reason: "files_unknown"}
	}
	a := plan.app
	if reason, message, _, err := p.runtimeStartRefusal(ctx, a); err != nil {
		return &backupProgramOutcome{State: backupProgramLeftStopped, Reason: "gate_unread", Message: err.Error()}
	} else if reason != "" {
		return &backupProgramOutcome{State: backupProgramLeftStopped, Reason: reason, Message: message}
	}
	native := p.runtimeAppNativeOps()
	resp, err := native.control(ctx, transport.RuntimeAppControlRequest{Target: a.target(), Action: "start"})
	if err != nil {
		return &backupProgramOutcome{State: backupProgramUnknown, Reason: "agent_unreachable"}
	}
	if !resp.Done {
		return &backupProgramOutcome{State: backupProgramNotStarted, Reason: resp.Reason, Message: resp.Error}
	}
	_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET state = ?, updated_at = datetime('now') WHERE id = ?`, runtimeAppRunning, a.ID)
	deadline := time.Now().Add(backupProgramStartTimeout)
	for {
		status, err := native.status(ctx, transport.RuntimeAppStatusRequest{Target: a.target(), Spec: plan.spec})
		if err == nil && status.Error == "" {
			if reason, message := runtimeStatusFailure(status, a.Port); reason != "" {
				return &backupProgramOutcome{State: backupProgramNotStarted, Reason: reason, Message: message}
			}
			if status.Serving {
				return &backupProgramOutcome{State: backupProgramStarted}
			}
			if status.ActiveState == "failed" {
				return &backupProgramOutcome{State: backupProgramNotStarted, Reason: "start_failed",
					Message: "the program stopped while starting (" + status.Result + "); its log on the Programs tab shows why"}
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return &backupProgramOutcome{State: backupProgramUnknown, Reason: "agent_unreachable"}
			}
			return &backupProgramOutcome{State: backupProgramNotStarted, Reason: "not_listening",
				Message: "the program runs but did not listen on its port in time"}
		}
		if !sleepContext(ctx, runtimeAppPollInterval) {
			return &backupProgramOutcome{State: backupProgramUnknown, Reason: "agent_unreachable"}
		}
	}
}

// writeWithProgramOutcome writes the refusal write makes, with the program's
// outcome added to its vars (program, program_reason), so a failed restore
// also says whether the program it stopped runs again.
func writeWithProgramOutcome(w http.ResponseWriter, outcome *backupProgramOutcome, write func(http.ResponseWriter)) {
	if outcome == nil {
		write(w)
		return
	}
	recorder := httptest.NewRecorder()
	write(recorder)
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body == nil {
		for key, values := range recorder.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
		return
	}
	vars, _ := body["vars"].(map[string]any)
	if vars == nil {
		vars = map[string]any{}
	}
	vars["program"] = outcome.State
	if outcome.Reason != "" {
		vars["program_reason"] = outcome.Reason
	}
	body["vars"] = vars
	for key, values := range recorder.Header() {
		w.Header()[key] = values
	}
	w.WriteHeader(recorder.Code)
	_ = json.NewEncoder(w).Encode(body)
}

// restoreAnswer is a successful restore's answer: the Agent's, the program's
// outcome and what the Panel did not restore.
type restoreAnswer struct {
	backupspec.RestoreResponse
	Program *backupProgramOutcome `json:"program,omitempty"`
}

// handleBackupContents: GET /api/v1/domains/{id}/backups/contents, what a
// backup of this site would hold now (reads only), with the site's program.
func (p *Panel) handleBackupContents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	domainID, err := backupDomainIDFromPath(r.URL.Path)
	if err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid domain ID")
		return
	}
	d, err := p.loadBackupDomain(r.Context(), domainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "domain not found")
		} else {
			writeServerError(w, err)
		}
		return
	}
	req := backupspec.ListRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: d.SubscriptionID,
		DomainID: d.ID, DomainName: d.Name,
	}
	var contents backupspec.ContentsResponse
	if err := p.callAgentContext(r.Context(), "Agent.BackupContentsV1", &req, &contents); err != nil {
		writeServerError(w, err)
		return
	}
	answer := struct {
		backupspec.ContentsResponse
		// Program is the site's program record, when it has one: its kind
		// and whether it was deployed.
		Program *struct {
			Kind     siteruntime.Kind `json:"kind"`
			Deployed bool             `json:"deployed"`
		} `json:"program"`
	}{ContentsResponse: contents}
	if a, has, err := p.runtimeAppForDomain(r.Context(), d.ID); err != nil {
		writeServerError(w, err)
		return
	} else if has {
		answer.Program = &struct {
			Kind     siteruntime.Kind `json:"kind"`
			Deployed bool             `json:"deployed"`
		}{Kind: a.Kind, Deployed: a.Deployed}
	}
	_ = json.NewEncoder(w).Encode(answer)
}
