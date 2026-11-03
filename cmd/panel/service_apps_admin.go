package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The administrator's catalog of service apps (D-042: third-party software is
// gated at the "offer" step). Each entry shows what the app can do, what
// CelikPanel turns off, its licence, network and resource needs, and whether
// it can run on this server. Offering requires the risk summary's digest
// the administrator saw; a security hold stops every install at once.
//
// Yöneticinin hizmet uygulaması kataloğu: sunmadan önce risk özeti görülür ve
// onaylanır; güvenlik beklemesi bütün kurulumları bir anda durdurur.

type serviceAppAdminEntry struct {
	serviceapps.App
	RiskDigest    string                     `json:"risk_digest"`
	Offered       bool                       `json:"offered"`
	OfferStale    bool                       `json:"offer_stale"`
	Held          bool                       `json:"held"`
	HeldReason    string                     `json:"held_reason,omitempty"`
	Limits        transport.ServiceAppLimits `json:"limits"`
	Available     bool                       `json:"available"`
	Reason        string                     `json:"reason,omitempty"`
	Detail        string                     `json:"detail,omitempty"`
	RuntimeSource string                     `json:"runtime_source,omitempty"`
	Installs      int                        `json:"installs"`
	Running       int                        `json:"running"`
}

func (p *Panel) handleServiceAppsAdmin(w http.ResponseWriter, r *http.Request) {
	if !callerIsAdministrator(r) {
		writeClientError(w, http.StatusForbidden, "administrator only")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, serviceAppsAdminPath), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.writeServiceAppsAdmin(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	app, ok := serviceapps.Lookup(parts[0])
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "offer":
		p.offerServiceApp(w, r, app)
	case "withdraw":
		p.withdrawServiceApp(w, r, app)
	case "hold":
		p.holdServiceApp(w, r, app, true)
	case "release":
		p.holdServiceApp(w, r, app, false)
	default:
		http.NotFound(w, r)
	}
}

func (p *Panel) writeServiceAppsAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	offerings, err := p.loadServiceAppOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	resetServiceAppProbeCache()
	availability, probeErr := p.serviceAppAvailability(ctx)
	counts := map[string][2]int{}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT app_id, COUNT(*), SUM(state = 'running') FROM service_app_installs GROUP BY app_id`)
	if err != nil {
		writeServerError(w, err)
		return
	}
	for rows.Next() {
		var id string
		var total, running int
		if rows.Scan(&id, &total, &running) == nil {
			counts[id] = [2]int{total, running}
		}
	}
	rows.Close()
	entries := []serviceAppAdminEntry{}
	for _, app := range serviceapps.Catalog() {
		o := offerings[app.ID]
		entry := serviceAppAdminEntry{App: app, RiskDigest: app.RiskDigest(), Offered: o.Offered, Held: o.Held,
			HeldReason: o.HeldReason, Limits: serviceAppLimitsFor(app, offerings),
			Installs: counts[app.ID][0], Running: counts[app.ID][1]}
		entry.OfferStale = o.Offered && o.RiskDigest != app.RiskDigest()
		if a, ok := availability[app.ID]; ok {
			entry.Available, entry.Reason, entry.Detail, entry.RuntimeSource = a.Available, a.Reason, a.Detail, a.RuntimeSource
		} else if probeErr != nil {
			entry.Reason = serviceapps.ReasonProbeFailed
		}
		if app.Deferred != "" {
			entry.Available, entry.Reason, entry.Detail = false, serviceapps.ReasonDeferred, app.Deferred
		}
		entries = append(entries, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"apps": entries, "availability_known": probeErr == nil})
}

func (p *Panel) offerServiceApp(w http.ResponseWriter, r *http.Request, app serviceapps.App) {
	var req struct {
		RiskDigest string `json:"risk_digest"`
		MemoryMB   int    `json:"memory_mb"`
		CPUPercent int    `json:"cpu_percent"`
		TasksMax   int    `json:"tasks_max"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	if !app.Installable() {
		writeServiceAppRefusal(w, http.StatusConflict, "deferred",
			"Nothing was changed: "+app.Name+" cannot be offered in this release; docs/AI-APPS.md says what blocks it.", map[string]string{"app": app.Name})
		return
	}
	if req.RiskDigest != app.RiskDigest() {
		writeServiceAppRefusal(w, http.StatusConflict, "risk_changed",
			"Nothing was changed: the risk summary of "+app.Name+" changed since this page was opened. Read it again, then offer.", nil)
		return
	}
	limits := serviceAppLimitsFor(app, nil)
	if req.MemoryMB != 0 {
		limits.MemoryMB = req.MemoryMB
	}
	if req.CPUPercent != 0 {
		limits.CPUPercent = req.CPUPercent
	}
	if req.TasksMax != 0 {
		limits.TasksMax = req.TasksMax
	}
	if limits.MemoryMB < 256 || limits.MemoryMB > 65536 || limits.CPUPercent < 10 || limits.CPUPercent > 1600 || limits.TasksMax < 32 || limits.TasksMax > 8192 {
		writeServiceAppRefusal(w, http.StatusBadRequest, "limits",
			"Memory must be 256 to 65536 MB, CPU 10 to 1600 percent, processes 32 to 8192.", nil)
		return
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		INSERT INTO service_app_offerings (app_id, offered, risk_digest, memory_mb, cpu_percent, tasks_max, changed_by)
		VALUES (?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET offered = 1, risk_digest = excluded.risk_digest, memory_mb = excluded.memory_mb,
		  cpu_percent = excluded.cpu_percent, tasks_max = excluded.tasks_max, changed_by = excluded.changed_by,
		  updated_at = datetime('now')`,
		app.ID, req.RiskDigest, limits.MemoryMB, limits.CPUPercent, limits.TasksMax, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "service_app.offer", "service_app", 0, map[string]any{"app": app.ID, "version": app.Version,
		"risk_digest": req.RiskDigest, "licence": app.Licence, "memory_mb": limits.MemoryMB, "cpu_percent": limits.CPUPercent, "tasks_max": limits.TasksMax})
	p.writeServiceAppsAdmin(w, r)
}

// withdrawServiceApp closes the offering: nobody installs or updates it any
// more; what runs keeps running (D-038, D-042 P5).
func (p *Panel) withdrawServiceApp(w http.ResponseWriter, r *http.Request, app serviceapps.App) {
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		UPDATE service_app_offerings SET offered = 0, changed_by = ?, updated_at = datetime('now') WHERE app_id = ?`,
		nullablePositiveInt(callerID(currentCaller(r))), app.ID); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "service_app.withdraw", "service_app", 0, map[string]any{"app": app.ID})
	p.writeServiceAppsAdmin(w, r)
}

// holdServiceApp is the administrator's security hold: every install of the
// app is stopped and kept from starting at boot, and nobody can install,
// update or start it until the hold is lifted. Lifting starts nothing; each
// owner starts their app again. Data is never removed.
func (p *Panel) holdServiceApp(w http.ResponseWriter, r *http.Request, app serviceapps.App, hold bool) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if hold && (req.Reason == "" || len(req.Reason) > 500) {
		writeServiceAppRefusal(w, http.StatusBadRequest, "hold_reason", "Write why the app is held (for example the CVE), up to 500 characters.", nil)
		return
	}
	ctx := r.Context()
	limits := serviceAppLimitsFor(app, nil)
	held := 0
	if hold {
		held = 1
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO service_app_offerings (app_id, offered, memory_mb, cpu_percent, tasks_max, held, held_reason, changed_by)
		VALUES (?, 0, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET held = excluded.held, held_reason = excluded.held_reason,
		  changed_by = excluded.changed_by, updated_at = datetime('now')`,
		app.ID, limits.MemoryMB, limits.CPUPercent, limits.TasksMax, held, req.Reason, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServerError(w, err)
		return
	}
	stopped, failed := []string{}, []string{}
	if hold {
		stopped, failed = p.stopServiceAppInstalls(ctx, app.ID)
	}
	details := map[string]any{"app": app.ID, "reason": req.Reason, "stopped": stopped, "not_stopped": failed}
	if hold {
		p.auditDetailed(r, "service_app.hold", "service_app", 0, details)
	} else {
		p.auditDetailed(r, "service_app.release", "service_app", 0, details)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"stopped": stopped, "not_stopped": failed})
}

// stopServiceAppInstalls stops every install of an app (the hold). Each one
// is reported: stopped, or not stopped with the server's reason.
func (p *Panel) stopServiceAppInstalls(ctx context.Context, appID string) ([]string, []string) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT `+serviceAppInstallColumns+` FROM service_app_installs i JOIN domains d ON d.id = i.domain_id
		WHERE i.app_id = ? AND i.version <> ''`, appID)
	if err != nil {
		return nil, []string{"the installs could not be listed: " + err.Error()}
	}
	var installs []serviceAppInstall
	for rows.Next() {
		if in, err := scanServiceAppInstall(rows); err == nil {
			installs = append(installs, in)
		}
	}
	rows.Close()
	stopped, failed := []string{}, []string{}
	native := p.serviceAppNativeOps()
	for _, in := range installs {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		resp, err := native.control(callCtx, transport.ServiceAppControlRequest{Target: in.target(), Action: "stop"})
		cancel()
		switch {
		case err != nil:
			failed = append(failed, in.Domain+": the server did not answer")
		case !resp.Done:
			failed = append(failed, in.Domain+": "+resp.Error)
		default:
			stopped = append(stopped, in.Domain)
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE service_app_installs SET state = 'stopped', updated_at = datetime('now') WHERE id = ?`, in.ID)
		}
	}
	return stopped, failed
}
