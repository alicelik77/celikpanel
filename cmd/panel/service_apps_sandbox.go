package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site sandbox on the administrator's service-app page (alpha.119,
// docs/SITE-SANDBOX.md). The page reads, never changes: the host's level and
// gaps, the SMTP block's state and how many apps still run in an earlier
// release's sandbox. Those apps are never rewritten silently; the
// administrator applies the new sandbox with one action, which restarts
// each running app once through the durable service-app operation (D-025)
// and rewrites a stopped app's unit without starting it.
//
// Yöneticinin hizmet uygulaması sayfasında site koruması: sayfa yalnız okur.
// Eski korumadaki uygulamalar sessizce değiştirilmez; yönetici tek işlemle
// yeni korumayı uygular.

// serviceAppSandboxView is the "sandbox" part of the administrator's page.
type serviceAppSandboxView struct {
	// Known: the server answered; the rest is empty otherwise.
	Known          bool     `json:"known"`
	Level          string   `json:"level,omitempty"`
	Gaps           []string `json:"gaps"`
	SystemdVersion int      `json:"systemd_version,omitempty"`
	EgressFile     string   `json:"egress_file,omitempty"`
	EgressLoaded   bool     `json:"egress_loaded"`
	GuardTimer     string   `json:"guard_timer,omitempty"`
	SMTPRejects    int64    `json:"smtp_rejects"`
	// NeedsRefresh counts the installs whose unit is CelikPanel's unchanged
	// text of an earlier sandbox; Edited those an owner changed (kept, not
	// refreshable); Running how many of NeedsRefresh run now.
	NeedsRefresh int    `json:"needs_refresh"`
	Running      int    `json:"running"`
	Edited       int    `json:"edited"`
	Busy         bool   `json:"busy"`
	Error        string `json:"error,omitempty"`
}

// serviceAppSandboxCandidate is one install the refresh would touch.
type serviceAppSandboxCandidate struct {
	install serviceAppInstall
	running bool
}

// serviceAppSandboxPlan reads the server and the records: the installs that
// need the new sandbox, and those whose unit the owner changed.
func (p *Panel) serviceAppSandboxPlan(ctx context.Context) (transport.SandboxCapsResponse, []serviceAppSandboxCandidate, []serviceAppInstall, error) {
	caps, err := p.serviceAppNativeOps().sandboxCaps(ctx)
	if err != nil {
		return caps, nil, nil, err
	}
	units := map[int]transport.SandboxUnit{}
	for _, u := range caps.Units {
		// Node.js sites' units (alpha.127) have their own page.
		if u.Kind != "" {
			continue
		}
		units[u.InstallID] = u
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT `+serviceAppInstallColumns+` FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		WHERE i.version <> '' AND i.state IN ('running', 'stopped') ORDER BY i.id`)
	if err != nil {
		return caps, nil, nil, err
	}
	defer rows.Close()
	var candidates []serviceAppSandboxCandidate
	var edited []serviceAppInstall
	for rows.Next() {
		in, err := scanServiceAppInstall(rows)
		if err != nil {
			return caps, nil, nil, err
		}
		u, ok := units[in.ID]
		if !ok || u.Generation >= caps.Generation {
			continue
		}
		if u.File != "ours" {
			edited = append(edited, in)
			continue
		}
		candidates = append(candidates, serviceAppSandboxCandidate{install: in, running: in.State == serviceAppRunning})
	}
	return caps, candidates, edited, rows.Err()
}

func (p *Panel) serviceAppSandboxView(ctx context.Context) serviceAppSandboxView {
	view := serviceAppSandboxView{Gaps: []string{}, SMTPRejects: -1, Busy: serviceAppSandboxRefreshBusy()}
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	caps, candidates, edited, err := p.serviceAppSandboxPlan(callCtx)
	if err != nil {
		view.Error = "the server did not answer"
		return view
	}
	view.Known = true
	view.Level, view.Gaps, view.SystemdVersion = caps.Level, caps.Gaps, caps.Caps.SystemdVersion
	view.EgressFile, view.EgressLoaded, view.GuardTimer, view.SMTPRejects = caps.EgressFile, caps.EgressLoaded, caps.GuardTimer, caps.SMTPRejects
	if view.Gaps == nil {
		view.Gaps = []string{}
	}
	view.NeedsRefresh, view.Edited = len(candidates), len(edited)
	for _, c := range candidates {
		if c.running {
			view.Running++
		}
	}
	return view
}

// The refresh is single-flight across the Panel, and a repeated request
// (the answer was lost) returns the first answer instead of starting again.
var (
	serviceAppSandboxMu      sync.Mutex
	serviceAppSandboxRunning bool
	serviceAppSandboxAnswers = map[string]serviceAppSandboxRefreshResult{}
	serviceAppSandboxOrder   []string
	refreshRequestIDPattern  = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)
)

func serviceAppSandboxRefreshBusy() bool {
	serviceAppSandboxMu.Lock()
	defer serviceAppSandboxMu.Unlock()
	return serviceAppSandboxRunning
}

type serviceAppSandboxSkip struct {
	Domain string `json:"domain"`
	Reason string `json:"reason"`
}

type serviceAppSandboxRefreshResult struct {
	RequestID string `json:"request_id"`
	// Started: running apps that restart once into the new sandbox (each a
	// durable operation the domain's page follows).
	Started []string `json:"started"`
	// Rewritten: stopped apps whose unit now carries the new sandbox; they
	// stay stopped.
	Rewritten []string                `json:"rewritten"`
	Skipped   []serviceAppSandboxSkip `json:"skipped"`
}

// refreshServiceAppSandbox is POST .../service-apps/sandbox-refresh.
func (p *Panel) refreshServiceAppSandbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RequestID string `json:"request_id"`
		Expected  int    `json:"expected"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if !refreshRequestIDPattern.MatchString(req.RequestID) {
		writeClientError(w, http.StatusBadRequest, "a request id is required")
		return
	}
	serviceAppSandboxMu.Lock()
	if answer, ok := serviceAppSandboxAnswers[req.RequestID]; ok {
		serviceAppSandboxMu.Unlock()
		writeServiceAppSandboxAnswer(w, answer)
		return
	}
	if serviceAppSandboxRunning {
		serviceAppSandboxMu.Unlock()
		writeServiceAppRefusal(w, http.StatusConflict, "sandbox_refresh_running",
			"Nothing new was started: the new sandbox is already being applied. This page updates by itself when it is done.", nil)
		return
	}
	serviceAppSandboxRunning = true
	serviceAppSandboxMu.Unlock()
	defer func() {
		serviceAppSandboxMu.Lock()
		serviceAppSandboxRunning = false
		serviceAppSandboxMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	_, candidates, edited, err := p.serviceAppSandboxPlan(ctx)
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"Nothing was changed: the server did not answer. Try again when it is reachable.", nil)
		return
	}
	if len(candidates) != req.Expected {
		writeServiceAppRefusal(w, http.StatusConflict, "sandbox_refresh_changed",
			"Nothing was changed: the number of apps that need the new sandbox changed since this page was opened. Read the page again, then apply.", nil)
		return
	}
	result := serviceAppSandboxRefreshResult{RequestID: req.RequestID, Started: []string{}, Rewritten: []string{}, Skipped: []serviceAppSandboxSkip{}}
	for _, in := range edited {
		result.Skipped = append(result.Skipped, serviceAppSandboxSkip{Domain: in.Domain, Reason: "unit_edited"})
	}
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	native := p.serviceAppNativeOps()
	actor := nullablePositiveInt(callerID(currentCaller(r)))
	var started []string
	for _, c := range candidates {
		in := c.install
		app, ok := serviceapps.Lookup(in.AppID)
		if !ok || in.Version != app.Version {
			// Another release's version: its update carries the sandbox.
			result.Skipped = append(result.Skipped, serviceAppSandboxSkip{Domain: in.Domain, Reason: "update_first"})
			continue
		}
		if !c.running || offerings[in.AppID].Held {
			resp, err := native.activate(ctx, transport.ServiceAppActivateRequest{Target: in.target(), Version: in.Version,
				Port: in.Port, AuxPort: in.AuxPort, Limits: serviceAppLimitsFor(app, offerings), KeepStopped: true})
			switch {
			case err != nil:
				result.Skipped = append(result.Skipped, serviceAppSandboxSkip{Domain: in.Domain, Reason: "agent_unreachable"})
			case !resp.Done:
				result.Skipped = append(result.Skipped, serviceAppSandboxSkip{Domain: in.Domain, Reason: resp.Reason})
			default:
				result.Rewritten = append(result.Rewritten, in.Domain)
			}
			continue
		}
		opID, err := newServiceOperationID()
		if err != nil {
			writeServerError(w, err)
			return
		}
		if err := p.createServiceAppSandboxOperation(ctx, in, opID, actor); err != nil {
			result.Skipped = append(result.Skipped, serviceAppSandboxSkip{Domain: in.Domain, Reason: "busy"})
			continue
		}
		result.Started = append(result.Started, in.Domain)
		started = append(started, opID)
	}
	for _, opID := range started {
		p.startServiceAppOperation(opID, nil)
	}
	p.auditDetailed(r, "service_app.sandbox_refresh", "service_app", 0, map[string]any{
		"request_id": req.RequestID, "generation": sitesandbox.Generation,
		"started": result.Started, "rewritten": result.Rewritten, "skipped": result.Skipped,
	})
	serviceAppSandboxMu.Lock()
	serviceAppSandboxAnswers[req.RequestID] = result
	serviceAppSandboxOrder = append(serviceAppSandboxOrder, req.RequestID)
	if len(serviceAppSandboxOrder) > 32 {
		delete(serviceAppSandboxAnswers, serviceAppSandboxOrder[0])
		serviceAppSandboxOrder = serviceAppSandboxOrder[1:]
	}
	serviceAppSandboxMu.Unlock()
	writeServiceAppSandboxAnswer(w, result)
}

// createServiceAppSandboxOperation records the refresh of one running app
// as an update to its own version that starts at "activate": the unit is
// written with the new sandbox and restarted, then proved (systemd's view,
// the SMTP set, loopback only) before the site is published again. The
// unique running-operation index refuses a second operation.
func (p *Panel) createServiceAppSandboxOperation(ctx context.Context, in serviceAppInstall, opID string, actor any) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO service_app_operations (id, install_id, kind, step, actor_id, message)
		VALUES (?, ?, 'update', 'activate', ?, 'sandbox_refresh')`, opID, in.ID, actor); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE service_app_installs SET target_version = version, updated_at = datetime('now')
		WHERE id = ? AND version = ? AND state = 'running'`, in.ID, in.Version); err != nil {
		return err
	}
	return tx.Commit()
}

func writeServiceAppSandboxAnswer(w http.ResponseWriter, result serviceAppSandboxRefreshResult) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// serviceAppSandboxProofFailure is the operation's refusal text for a unit
// whose proof failed.
func serviceAppSandboxProofFailure(proof *transport.SandboxProof) string {
	return "the application started, but its sandbox could not be proved (" + proof.Code + ": " + proof.Detail + "), so it was stopped and not published"
}
