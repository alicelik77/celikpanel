package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The administrator's offering of runtime apps (D-042: third-party code is
// gated at the "offer" step). The page shows what an uploaded program can
// do, what the sandbox blocks, what remains a risk, the limits and the
// host's sandbox level. Offering requires the risk digest the administrator
// saw and, on a host that enforces less than the full sandbox, the named
// gaps accepted one by one; a reduced level never passes silently. A
// security hold stops every program at once.
//
// Yöneticinin çalışma ortamı sunumu: risk özeti görülür ve onaylanır; tam
// korumayı sağlamayan sunucuda açıklar tek tek kabul edilir; güvenlik
// beklemesi bütün programları bir anda durdurur.

type runtimeAdminEntry struct {
	Kind          string             `json:"kind"`
	OfferingID    string             `json:"offering_id"`
	Implemented   bool               `json:"implemented"`
	Risk          siteruntime.Risk   `json:"risk"`
	RiskDigest    string             `json:"risk_digest"`
	Offered       bool               `json:"offered"`
	OfferStale    bool               `json:"offer_stale"`
	Held          bool               `json:"held"`
	HeldReason    string             `json:"held_reason,omitempty"`
	Limits        siteruntime.Limits `json:"limits"`
	AcceptedLevel string             `json:"accepted_level"`
	AcceptedGaps  []string           `json:"accepted_gaps"`
	// LevelGap: the host is reduced now and the offering does not cover
	// its gaps (the programs cannot deploy or start until accepted).
	LevelGap bool `json:"level_gap"`
	Apps     int  `json:"apps"`
	Running  int  `json:"running"`
}

func (p *Panel) handleRuntimeAppsAdmin(w http.ResponseWriter, r *http.Request) {
	if !callerIsAdministrator(r) {
		writeClientError(w, http.StatusForbidden, "administrator only")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, runtimeAppsAdminPath), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.writeRuntimeAppsAdmin(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	kind := siteruntime.Kind(parts[0])
	if !siteruntime.Implemented(kind) || siteruntime.OfferingID(kind) == "" {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "offer":
		p.offerRuntime(w, r, kind)
	case "withdraw":
		p.withdrawRuntime(w, r, kind)
	case "hold":
		p.holdRuntime(w, r, kind, true)
	case "release":
		p.holdRuntime(w, r, kind, false)
	default:
		http.NotFound(w, r)
	}
}

func (p *Panel) writeRuntimeAppsAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	offerings, err := p.loadRuntimeOfferings(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	counts := map[string][2]int{}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT offering_id, COUNT(*), SUM(state = 'running') FROM runtime_apps GROUP BY offering_id`)
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
	caps, capsErr := p.runtimeHostLevel(ctx)
	sandbox := serviceAppSandboxLvl{Gaps: []string{}}
	if capsErr == nil {
		sandbox = serviceAppSandboxLvl{Known: true, Level: caps.Level, Gaps: caps.Gaps}
		if sandbox.Gaps == nil {
			sandbox.Gaps = []string{}
		}
	}
	entries := []runtimeAdminEntry{}
	for _, kind := range siteruntime.Kinds {
		id := siteruntime.OfferingID(kind)
		if id == "" {
			continue
		}
		o, has := offerings[id]
		entry := runtimeAdminEntry{Kind: string(kind), OfferingID: id, Implemented: siteruntime.Implemented(kind),
			Risk: siteruntime.RiskOf(kind), RiskDigest: siteruntime.RiskDigest(kind), Offered: o.Offered, Held: o.Held,
			HeldReason: o.HeldReason, Limits: runtimeLimitsFor(kind, o, has), AcceptedLevel: "full", AcceptedGaps: []string{},
			Apps: counts[id][0], Running: counts[id][1]}
		if has && o.AcceptedLevel != "" {
			entry.AcceptedLevel, entry.AcceptedGaps = o.AcceptedLevel, o.AcceptedGaps
		}
		entry.OfferStale = o.Offered && o.RiskDigest != entry.RiskDigest
		entry.LevelGap = o.Offered && capsErr == nil && runtimeLevelRefusal(caps, o) != ""
		entries = append(entries, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"kinds": entries, "sandbox": sandbox,
		"port_range": [2]int{siteruntime.PortMin, siteruntime.PortMax}})
}

// runtimeOfferLevel decides the level an offer is stored with: full on a
// full host; on a reduced host, reduced only when the administrator accepted
// exactly every gap the host has now (refusal "" = acceptable).
func runtimeOfferLevel(caps transport.SandboxCapsResponse, accepted []string) (level string, gaps []string, refusal string) {
	normalized, err := siteruntime.NormalizeGaps(accepted)
	if err != nil {
		return "", nil, err.Error()
	}
	if caps.Level != string(sitesandbox.LevelReduced) {
		return string(sitesandbox.LevelFull), []string{}, ""
	}
	current := make([]sitesandbox.Gap, len(caps.Gaps))
	for i, g := range caps.Gaps {
		current[i] = sitesandbox.Gap(g)
	}
	if !siteruntime.GapsCovered(current, normalized) {
		return "", nil, "this server does not enforce the full sandbox (" + strings.Join(caps.Gaps, ", ") + "); accept each named gap to offer at the reduced level"
	}
	return string(sitesandbox.LevelReduced), normalized, ""
}

func (p *Panel) offerRuntime(w http.ResponseWriter, r *http.Request, kind siteruntime.Kind) {
	var req struct {
		RiskDigest   string   `json:"risk_digest"`
		MemoryMB     int      `json:"memory_mb"`
		CPUPercent   int      `json:"cpu_percent"`
		TasksMax     int      `json:"tasks_max"`
		AcceptedGaps []string `json:"accepted_gaps"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	ctx := r.Context()
	if req.RiskDigest != siteruntime.RiskDigest(kind) {
		writeRuntimeAppRefusal(w, http.StatusConflict, "risk_changed",
			"Nothing was changed: the risk summary of uploaded programs changed since this page was opened. Read it again, then offer.", nil)
		return
	}
	limits := siteruntime.DefaultLimits(kind)
	if req.MemoryMB != 0 {
		limits.MemoryMB = req.MemoryMB
	}
	if req.CPUPercent != 0 {
		limits.CPUPercent = req.CPUPercent
	}
	if req.TasksMax != 0 {
		limits.Tasks = req.TasksMax
	}
	if err := limits.Validate(); err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "limits", "Nothing was changed: "+err.Error()+".", nil)
		return
	}
	caps, err := p.runtimeHostLevel(ctx)
	if err != nil {
		writeRuntimeAppRefusal(w, http.StatusBadGateway, "agent_unreachable",
			"Nothing was changed: the server did not say which sandbox level it enforces, so it cannot be offered yet. Try again in a minute.", nil)
		return
	}
	level, gaps, refusal := runtimeOfferLevel(caps, req.AcceptedGaps)
	if refusal != "" {
		writeRuntimeAppRefusal(w, http.StatusConflict, "sandbox_reduced", "Nothing was changed: "+refusal+".",
			map[string]string{"gaps": strings.Join(caps.Gaps, ", ")})
		return
	}
	id := siteruntime.OfferingID(kind)
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO runtime_offerings (offering_id, kind, offered, risk_digest, memory_mb, cpu_percent, tasks_max, accepted_level, accepted_gaps, changed_by)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(offering_id) DO UPDATE SET offered = 1, risk_digest = excluded.risk_digest, memory_mb = excluded.memory_mb,
		  cpu_percent = excluded.cpu_percent, tasks_max = excluded.tasks_max, accepted_level = excluded.accepted_level,
		  accepted_gaps = excluded.accepted_gaps, changed_by = excluded.changed_by, updated_at = datetime('now')`,
		id, string(kind), req.RiskDigest, limits.MemoryMB, limits.CPUPercent, limits.Tasks, level, strings.Join(gaps, ","),
		nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "runtime.offer", "runtime", 0, map[string]any{"offering": id, "risk_digest": req.RiskDigest,
		"memory_mb": limits.MemoryMB, "cpu_percent": limits.CPUPercent, "tasks_max": limits.Tasks, "level": level, "accepted_gaps": gaps})
	p.writeRuntimeAppsAdmin(w, r)
}

// withdrawRuntime closes the offering: nobody creates or deploys any more;
// what runs keeps running (D-038, D-042 P5).
func (p *Panel) withdrawRuntime(w http.ResponseWriter, r *http.Request, kind siteruntime.Kind) {
	id := siteruntime.OfferingID(kind)
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		UPDATE runtime_offerings SET offered = 0, changed_by = ?, updated_at = datetime('now') WHERE offering_id = ?`,
		nullablePositiveInt(callerID(currentCaller(r))), id); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "runtime.withdraw", "runtime", 0, map[string]any{"offering": id})
	p.writeRuntimeAppsAdmin(w, r)
}

// holdRuntime is the security hold: every app of the offering is stopped and
// kept from starting at boot, and nobody can deploy or start one until the
// hold is lifted. Lifting starts nothing; each owner starts their program
// again. Files are never removed.
func (p *Panel) holdRuntime(w http.ResponseWriter, r *http.Request, kind siteruntime.Kind, hold bool) {
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeServiceAppBody(w, r, &req) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if hold && (req.Reason == "" || len(req.Reason) > 500) {
		writeRuntimeAppRefusal(w, http.StatusBadRequest, "hold_reason", "Write why the programs are held (for example the advisory), up to 500 characters.", nil)
		return
	}
	ctx := r.Context()
	id := siteruntime.OfferingID(kind)
	limits := siteruntime.DefaultLimits(kind)
	held := 0
	if hold {
		held = 1
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO runtime_offerings (offering_id, kind, offered, memory_mb, cpu_percent, tasks_max, held, held_reason, changed_by)
		VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(offering_id) DO UPDATE SET held = excluded.held, held_reason = excluded.held_reason,
		  changed_by = excluded.changed_by, updated_at = datetime('now')`,
		id, string(kind), limits.MemoryMB, limits.CPUPercent, limits.Tasks, held, req.Reason, nullablePositiveInt(callerID(currentCaller(r)))); err != nil {
		writeServerError(w, err)
		return
	}
	stopped, failed := []string{}, []string{}
	if hold {
		stopped, failed = p.stopRuntimeApps(ctx, id)
	}
	details := map[string]any{"offering": id, "reason": req.Reason, "stopped": stopped, "not_stopped": failed}
	if hold {
		p.auditDetailed(r, "runtime.hold", "runtime", 0, details)
	} else {
		p.auditDetailed(r, "runtime.release", "runtime", 0, details)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"stopped": stopped, "not_stopped": failed})
}

// stopRuntimeApps stops every deployed app of an offering (the hold). Each
// is reported: stopped, or not stopped with the server's reason.
func (p *Panel) stopRuntimeApps(ctx context.Context, offeringID string) ([]string, []string) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT `+runtimeAppColumns+` FROM runtime_apps a JOIN domains d ON d.id = a.domain_id
		WHERE a.offering_id = ? AND (a.deployed = 1 OR a.state IN ('deploying', 'running'))`, offeringID)
	if err != nil {
		return nil, []string{"the programs could not be listed: " + err.Error()}
	}
	var apps []runtimeApp
	for rows.Next() {
		if a, err := scanRuntimeApp(rows); err == nil {
			apps = append(apps, a)
		}
	}
	rows.Close()
	stopped, failed := []string{}, []string{}
	native := p.runtimeAppNativeOps()
	for _, a := range apps {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		resp, err := native.control(callCtx, transport.RuntimeAppControlRequest{Target: a.target(), Action: "stop"})
		cancel()
		switch {
		case err != nil:
			failed = append(failed, a.Domain+": the server did not answer")
		case !resp.Done:
			failed = append(failed, a.Domain+": "+resp.Error)
		default:
			stopped = append(stopped, a.Domain)
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE runtime_apps SET state = 'stopped', updated_at = datetime('now') WHERE id = ?`, a.ID)
		}
	}
	return stopped, failed
}
