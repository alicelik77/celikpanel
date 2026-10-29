package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Distribution updates confirmed during a run (Beta-1 re-run P5, 2026-10-29).
//
// A component step waits, installing nothing, when its install would bring a
// distribution point update the review did not list
// (server_setup_distribution_updates_changed) or when that list could not be
// read (server_setup_distribution_updates_unread). The owner acts from that
// wait: "review" reads the list again for the run's remaining component
// installs (read only; nothing is installed), and "confirm" records the exact
// list the owner saw for this run. The same execution then continues from the
// waiting step: nothing already done is repeated, and no new plan or restart
// is needed. A running step or a known failure is never touched: both actions
// are refused unless the run waits on exactly these codes, before its child
// operation exists.
//
// The confirmed list is kept in panel_settings under the execution's id (no
// schema change), written only by "confirm". The runner reads it into the
// effective plan; it never writes it, so a runner pass cannot drop it.
//
// Kurulum sırasında onaylanan dağıtım güncellemeleri: bekleyen adımdan sahip
// listeyi yeniden okur ve gördüğünü onaylar; aynı çalışma o adımdan sürer.

const serverSetupRunPackageUpdatesKeyPrefix = "server_setup_run_package_updates:"

// serverSetupRunPackageUpdatesMax bounds a confirmation.
const serverSetupRunPackageUpdatesMax = 256

func serverSetupRunPackageUpdatesKey(executionID string) string {
	return serverSetupRunPackageUpdatesKeyPrefix + executionID
}

// serverSetupPackageUpdatesWaitingStep is the index of the component step a
// run waits on for its distribution updates, or -1: the run waits on one of
// the two codes, every earlier step succeeded, and the waiting step is a
// component install still running without a child operation.
func serverSetupPackageUpdatesWaitingStep(execution serverSetupExecution) int {
	if execution.Status != "waiting" || execution.Error == nil ||
		!stringIn(execution.Error.Code, "server_setup_distribution_updates_changed", serverSetupPackageUpdatesUnreadCode) {
		return -1
	}
	for index, step := range execution.Steps {
		if step.Status == "succeeded" {
			continue
		}
		if step.Kind == "service" && step.Status == "running" && step.OperationID == "" && execution.Phase == step.Kind {
			return index
		}
		return -1
	}
	return -1
}

// serverSetupRemainingComponentSteps are the run's component installs from
// the waiting step on: one confirmation covers what is left, so a later
// component does not stop again for the same list.
func serverSetupRemainingComponentSteps(execution serverSetupExecution, from int) []serverSetupPlanStep {
	var steps []serverSetupPlanStep
	for _, step := range execution.Steps[from:] {
		if step.Kind == "service" && step.Status != "succeeded" && step.OperationID == "" {
			steps = append(steps, step.serverSetupPlanStep)
		}
	}
	return steps
}

func (p *Panel) readServerSetupRunPackageUpdates(ctx context.Context, executionID string) ([]transport.NotableInstall, error) {
	var updates []transport.NotableInstall
	if _, err := readServerSetupJSONSetting(ctx, p.db.GetDB(), serverSetupRunPackageUpdatesKey(executionID), &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// withServerSetupRunPackageUpdates adds the updates the owner confirmed
// during this run to the plan the steps use. The reviewed plan itself is
// never changed; its identity does not include the list.
func (p *Panel) withServerSetupRunPackageUpdates(ctx context.Context, plan serverSetupPlan, executionID string) (serverSetupPlan, error) {
	confirmed, err := p.readServerSetupRunPackageUpdates(ctx, executionID)
	if err != nil || len(confirmed) == 0 {
		return plan, err
	}
	merged := slices.Clone(plan.PackageUpdates)
	if plan.PackageUpdatesState != "known" {
		merged = nil
	}
	for _, item := range confirmed {
		if !slices.ContainsFunc(merged, func(listed transport.NotableInstall) bool {
			return listed.Package == item.Package && listed.To == item.To
		}) {
			merged = append(merged, item)
		}
	}
	slices.SortFunc(merged, func(a, b transport.NotableInstall) int { return strings.Compare(a.Package, b.Package) })
	plan.PackageUpdates, plan.PackageUpdatesState = merged, "known"
	return plan, nil
}

type serverSetupPackageUpdatesRequest struct {
	ExecutionID string                     `json:"execution_id"`
	Action      string                     `json:"action"`
	Updates     []transport.NotableInstall `json:"updates,omitempty"`
}

type serverSetupPackageUpdatesResponse struct {
	State     string                     `json:"state"`
	Updates   []transport.NotableInstall `json:"updates"`
	Execution *serverSetupExecution      `json:"execution,omitempty"`
}

// waitingServerSetupPackageUpdates loads the run and its plan, and the
// remaining component steps, only while the run waits on its updates.
func (p *Panel) waitingServerSetupPackageUpdates(ctx context.Context, executionID string) (serverSetupExecution, string, serverSetupPlan, []serverSetupPlanStep, error) {
	var raw string
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT execution_json FROM server_setup_executions WHERE id=? AND status='waiting'`, executionID).Scan(&raw); err != nil {
		return serverSetupExecution{}, "", serverSetupPlan{}, nil, err
	}
	var execution serverSetupExecution
	if json.Unmarshal([]byte(raw), &execution) != nil || execution.ID != executionID {
		return serverSetupExecution{}, "", serverSetupPlan{}, nil, errors.New("saved setup operation is invalid")
	}
	index := serverSetupPackageUpdatesWaitingStep(execution)
	if index < 0 {
		return serverSetupExecution{}, "", serverSetupPlan{}, nil, sql.ErrNoRows
	}
	// The waiting step has started nothing: no operation under its own
	// request identity (review fix, item 8).
	if !p.serverSetupStepChildMissing(ctx, execution.Steps[index]) {
		return serverSetupExecution{}, "", serverSetupPlan{}, nil, sql.ErrNoRows
	}
	plan, err := p.loadServerSetupPlan(ctx, execution.PlanID)
	if err != nil || validateServerSetupExecution(plan, execution) != nil {
		return serverSetupExecution{}, "", serverSetupPlan{}, nil, sql.ErrNoRows
	}
	return execution, raw, plan, serverSetupRemainingComponentSteps(execution, index), nil
}

// handleServerSetupPackageUpdates: POST {execution_id, action: "review"} reads
// the list (read only); POST {execution_id, action: "confirm", updates} records
// what the owner saw and lets the same run continue.
func (p *Panel) handleServerSetupPackageUpdates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !serverSetupAdmin(w, r, http.MethodPost) {
		return
	}
	var request serverSetupPackageUpdatesRequest
	if decodeServiceOperationJSON(w, r, &request) != nil || !validServiceOperationID(request.ExecutionID) ||
		!stringIn(request.Action, "review", "confirm") || len(request.Updates) > serverSetupRunPackageUpdatesMax {
		writeClientError(w, http.StatusBadRequest, "invalid setup package update request")
		return
	}
	notWaiting := func() {
		writeCodedError(w, http.StatusConflict, "setup_conflict", "This setup run is not waiting for its distribution updates. Read the current setup state.", "/setup")
	}
	if request.Action == "review" {
		_, _, _, steps, err := p.waitingServerSetupPackageUpdates(r.Context(), request.ExecutionID)
		if err != nil {
			notWaiting()
			return
		}
		updates, state := p.serverSetupPackageUpdates(r.Context(), steps)
		if updates == nil {
			updates = []transport.NotableInstall{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serverSetupPackageUpdatesResponse{State: stateOrKnown(state), Updates: updates})
		return
	}
	p.serverSetupMu.Lock()
	defer p.serverSetupMu.Unlock()
	execution, raw, plan, steps, err := p.waitingServerSetupPackageUpdates(r.Context(), request.ExecutionID)
	if err != nil {
		notWaiting()
		return
	}
	// The package manager is asked again now; what it lists must be what the
	// owner confirmed (or already confirmed), as at the start of setup.
	fresh, state := p.serverSetupPackageUpdates(r.Context(), steps)
	if stateOrKnown(state) != "known" {
		writeCodedError(w, http.StatusConflict, serverSetupPackageUpdatesUnreadCode, "CelikPanel could not read the distribution updates again, so nothing was confirmed. Nobody needs to fix anything: wait a moment and review the updates again; setup continues once you confirm them.", "")
		return
	}
	known, err := p.withServerSetupRunPackageUpdates(r.Context(), plan, execution.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	covered := func(item transport.NotableInstall) bool {
		match := func(other transport.NotableInstall) bool { return other.Package == item.Package && other.To == item.To }
		return slices.ContainsFunc(request.Updates, match) || slices.ContainsFunc(serverSetupKnownPackageUpdates(known), match)
	}
	confirmed, _ := p.readServerSetupRunPackageUpdates(r.Context(), execution.ID)
	for _, item := range fresh {
		if !covered(item) {
			writeCodedError(w, http.StatusConflict, "server_setup_review_stale", "The distribution updates changed again since you reviewed them, so nothing was confirmed. Review the updates again and confirm the new list.", "")
			return
		}
		if !slices.ContainsFunc(confirmed, func(other transport.NotableInstall) bool { return other.Package == item.Package && other.To == item.To }) {
			confirmed = append(confirmed, transport.NotableInstall{Package: item.Package, Reason: item.Reason, From: item.From, To: item.To, PointUpdate: true})
		}
	}
	if err := writeServerSetupJSONSetting(r.Context(), p.db.GetDB(), serverSetupRunPackageUpdatesKey(execution.ID), confirmed); err != nil {
		writeServerError(w, err)
		return
	}
	// The wait ends with the owner's confirmation; the runner's next pass
	// (woken now) installs the waiting component. If a runner pass is in
	// flight it may write its older wait once more; the confirmation stays.
	execution.Status, execution.Error = "running", nil
	if encoded, err := json.Marshal(execution); err == nil {
		_, _ = p.db.GetDB().ExecContext(r.Context(), `UPDATE server_setup_executions SET status='running',execution_json=?,updated_at=? WHERE id=? AND status='waiting' AND execution_json=?`, string(encoded), time.Now().UTC().Format(time.RFC3339Nano), execution.ID, raw)
	}
	p.audit(r, "server.setup.package_updates.confirm", "", 0)
	p.wakeServerSetupRunner()
	p.launchServerSetupExecution()
	latest, err := p.latestServerSetupExecution(r.Context())
	if err != nil {
		writeServerError(w, err)
		return
	}
	if fresh == nil {
		fresh = []transport.NotableInstall{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(serverSetupPackageUpdatesResponse{State: "known", Updates: fresh, Execution: latest})
}

func stateOrKnown(state string) string {
	if state == "" {
		return "known"
	}
	return state
}

func serverSetupKnownPackageUpdates(plan serverSetupPlan) []transport.NotableInstall {
	if plan.PackageUpdatesState != "known" {
		return nil
	}
	return plan.PackageUpdates
}
