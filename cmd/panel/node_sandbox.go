package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js sites in the site sandbox (alpha.127, docs/SITE-SANDBOX.md §6),
// on Components → Node.js. The page reads, never changes: the host's level
// and gaps and which Node.js sites still run in an earlier release's
// protection. Those units are never rewritten silently; the administrator
// moves them with one action. It runs one at a time across the Panel, a
// repeated request id returns the first answer, the number the page showed
// must still hold, and each site is moved one after the other: a running
// application restarts once in the sandbox and is proved (the Agent puts
// the earlier unit back and starts it again when the proof refutes the move
// or the application stops answering), a stopped one only gets its new unit.
// An owner-changed unit is kept and listed. It is audited.
//
// Node.js siteleri site korumasında: sayfa yalnız okur; eski korumadaki
// siteler sessizce değiştirilmez, yönetici tek işlemle taşır.

// nodeSandboxSite is one Node.js site as the page lists it.
type nodeSandboxSite struct {
	DomainID int    `json:"domain_id"`
	Name     string `json:"name"`
}

// nodeSandboxView is GET /api/v1/runtimes/node/sandbox.
type nodeSandboxView struct {
	Known          bool     `json:"known"`
	Level          string   `json:"level,omitempty"`
	Gaps           []string `json:"gaps"`
	SystemdVersion int      `json:"systemd_version,omitempty"`
	EgressLoaded   bool     `json:"egress_loaded"`
	// Sandboxed counts the sites whose unit is in the sandbox.
	Sandboxed int `json:"sandboxed"`
	// NeedsMove are the sites the action moves, Running how many of them
	// run now.
	NeedsMove []nodeSandboxSite `json:"needs_move"`
	Running   int               `json:"running"`
	// Edited: an owner changed the unit (kept, not moved). SharedAccount:
	// the application runs as the shared web account; "Run under the
	// site's own user" on the site's page comes first.
	Edited        []nodeSandboxSite `json:"edited"`
	SharedAccount []nodeSandboxSite `json:"shared_account"`
	Busy          bool              `json:"busy"`
	Error         string            `json:"error,omitempty"`
}

// nodeSandboxCandidate is one site the action would move.
type nodeSandboxCandidate struct {
	site    nodeSandboxSite
	running bool
}

// Seams (tests replace them).
var (
	nodeSandboxCaps = func(p *Panel, ctx context.Context) (transport.SandboxCapsResponse, error) {
		return p.serviceAppNativeOps().sandboxCaps(ctx)
	}
	nodeSandboxApply = func(p *Panel, ctx context.Context, req *transport.AppApplyRequest) (transport.AppApplyResponse, error) {
		var resp transport.AppApplyResponse
		err := p.callAgentContext(ctx, "Agent.ApplyAppUnit", req, &resp)
		return resp, err
	}
)

// nodeSandboxPlan reads the server and the records.
func (p *Panel) nodeSandboxPlan(ctx context.Context) (transport.SandboxCapsResponse, nodeSandboxView, []nodeSandboxCandidate, error) {
	view := nodeSandboxView{Gaps: []string{}, NeedsMove: []nodeSandboxSite{}, Edited: []nodeSandboxSite{}, SharedAccount: []nodeSandboxSite{}}
	caps, err := nodeSandboxCaps(p, ctx)
	if err != nil {
		return caps, view, nil, err
	}
	units := map[int]transport.SandboxUnit{}
	for _, u := range caps.Units {
		if u.Kind == "node" {
			units[u.SiteID] = u
		}
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT s.id, d.id, d.name, COALESCE(s.app_run_as, '')
		FROM sites s JOIN domains d ON d.id = s.domain_id
		WHERE COALESCE(s.project_type, 'php') = 'node' ORDER BY d.name`)
	if err != nil {
		return caps, view, nil, err
	}
	defer rows.Close()
	var candidates []nodeSandboxCandidate
	for rows.Next() {
		var siteID int
		var site nodeSandboxSite
		var runAs string
		if err := rows.Scan(&siteID, &site.DomainID, &site.Name, &runAs); err != nil {
			return caps, view, nil, err
		}
		u, ok := units[siteID]
		switch {
		case !ok:
			// No unit on the server: the site's next write makes one in
			// the sandbox.
		case u.Generation >= caps.Generation:
			view.Sandboxed++
		case u.File != "ours":
			view.Edited = append(view.Edited, site)
		case runAs != "site":
			view.SharedAccount = append(view.SharedAccount, site)
		default:
			candidates = append(candidates, nodeSandboxCandidate{site: site, running: u.Active == "active" || u.Active == "activating" || u.Active == "reloading"})
		}
	}
	if err := rows.Err(); err != nil {
		return caps, view, nil, err
	}
	view.Known = true
	view.Level, view.Gaps, view.SystemdVersion, view.EgressLoaded = caps.Level, caps.Gaps, caps.Caps.SystemdVersion, caps.EgressLoaded
	if view.Gaps == nil {
		view.Gaps = []string{}
	}
	for _, c := range candidates {
		view.NeedsMove = append(view.NeedsMove, c.site)
		if c.running {
			view.Running++
		}
	}
	return caps, view, candidates, nil
}

// The move is single-flight across the Panel; a repeated request id
// returns the first answer.
var (
	nodeSandboxMu      sync.Mutex
	nodeSandboxRunning bool
	nodeSandboxAnswers = map[string]nodeSandboxResult{}
	nodeSandboxOrder   []string
)

func nodeSandboxBusy() bool {
	nodeSandboxMu.Lock()
	defer nodeSandboxMu.Unlock()
	return nodeSandboxRunning
}

type nodeSandboxOutcome struct {
	Domain string `json:"domain"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

type nodeSandboxResult struct {
	RequestID string `json:"request_id"`
	// Moved: running applications that restarted in the sandbox and were
	// proved. Rewritten: stopped applications that got the new unit and
	// stay stopped. PutBack: the proof refuted the move and the earlier
	// unit runs again. Skipped: nothing changed for that site.
	Moved     []string             `json:"moved"`
	Rewritten []string             `json:"rewritten"`
	PutBack   []nodeSandboxOutcome `json:"put_back"`
	Skipped   []nodeSandboxOutcome `json:"skipped"`
}

func (p *Panel) handleNodeSandboxView(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	_, view, _, err := p.nodeSandboxPlan(ctx)
	view.Busy = nodeSandboxBusy()
	if err != nil {
		view.Known = false
		view.Error = "the server did not answer"
	}
	_ = json.NewEncoder(w).Encode(view)
}

// handleNodeSandboxMove is POST /api/v1/runtimes/node/sandbox-refresh.
func (p *Panel) handleNodeSandboxMove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var req struct {
		RequestID string `json:"request_id"`
		Expected  int    `json:"expected"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || !refreshRequestIDPattern.MatchString(req.RequestID) {
		writeClientError(w, http.StatusBadRequest, "a request id is required")
		return
	}
	nodeSandboxMu.Lock()
	if answer, ok := nodeSandboxAnswers[req.RequestID]; ok {
		nodeSandboxMu.Unlock()
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	if nodeSandboxRunning {
		nodeSandboxMu.Unlock()
		writeServiceAppRefusal(w, http.StatusConflict, "sandbox_refresh_running",
			"Nothing new was started: Node.js sites are already being moved into the site sandbox. This page updates by itself when it is done.", nil)
		return
	}
	nodeSandboxRunning = true
	nodeSandboxMu.Unlock()
	defer func() {
		nodeSandboxMu.Lock()
		nodeSandboxRunning = false
		nodeSandboxMu.Unlock()
	}()

	ctx, cancel := hostingDurableContext(r.Context())
	defer cancel()
	_, _, candidates, err := p.nodeSandboxPlan(ctx)
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"Nothing was changed: the server did not answer. Try again when it is reachable.", nil)
		return
	}
	if len(candidates) != req.Expected {
		writeServiceAppRefusal(w, http.StatusConflict, "sandbox_refresh_changed",
			"Nothing was changed: the number of Node.js sites that need the site sandbox changed since this page was opened. Read the page again, then move them.", nil)
		return
	}
	result := nodeSandboxResult{RequestID: req.RequestID, Moved: []string{}, Rewritten: []string{}, PutBack: []nodeSandboxOutcome{}, Skipped: []nodeSandboxOutcome{}}
	for _, c := range candidates {
		result.add(c, p.moveNodeSiteIntoSandbox(ctx, c))
	}
	sort.Strings(result.Moved)
	p.auditDetailed(r, "site.node_sandbox_refresh", "domain", 0, map[string]any{
		"request_id": req.RequestID, "generation": sitesandbox.Generation,
		"moved": result.Moved, "rewritten": result.Rewritten, "put_back": result.PutBack, "skipped": result.Skipped,
	})
	nodeSandboxMu.Lock()
	nodeSandboxAnswers[req.RequestID] = result
	nodeSandboxOrder = append(nodeSandboxOrder, req.RequestID)
	if len(nodeSandboxOrder) > 32 {
		delete(nodeSandboxAnswers, nodeSandboxOrder[0])
		nodeSandboxOrder = nodeSandboxOrder[1:]
	}
	nodeSandboxMu.Unlock()
	_ = json.NewEncoder(w).Encode(result)
}

// nodeSiteMove is what happened to one site.
type nodeSiteMove struct {
	kind   string // moved, rewritten, put_back, skipped
	reason string
	detail string
}

func (result *nodeSandboxResult) add(c nodeSandboxCandidate, m nodeSiteMove) {
	switch m.kind {
	case "moved":
		result.Moved = append(result.Moved, c.site.Name)
	case "rewritten":
		result.Rewritten = append(result.Rewritten, c.site.Name)
	case "put_back":
		result.PutBack = append(result.PutBack, nodeSandboxOutcome{Domain: c.site.Name, Reason: m.reason, Detail: m.detail})
	default:
		result.Skipped = append(result.Skipped, nodeSandboxOutcome{Domain: c.site.Name, Reason: m.reason, Detail: m.detail})
	}
}

// moveNodeSiteIntoSandbox moves one site: the Agent writes the sandbox
// unit, restarts a running application once and proves it, or puts the
// earlier unit back. The unit's ledger record follows what is on disk.
func (p *Panel) moveNodeSiteIntoSandbox(ctx context.Context, c nodeSandboxCandidate) nodeSiteMove {
	request, _, err := p.appUnitRequest(ctx, c.site.DomainID)
	if err != nil {
		return nodeSiteMove{kind: "skipped", reason: "incomplete", detail: err.Error()}
	}
	request.SandboxMove = true
	request.NoStart = !c.running
	resp, err := nodeSandboxApply(p, ctx, request)
	if err != nil {
		return nodeSiteMove{kind: "skipped", reason: "agent_unreachable", detail: err.Error()}
	}
	if site, err := p.loadSiteFileSite(ctx, c.site.DomainID); err == nil {
		p.recordOtherSiteFile(ctx, site, transport.SiteFileKindAppUnit, "sandbox_move", resp.File, "no answer from the Agent")
	}
	reason, detail := "agent_error", resp.Error
	if resp.Sandbox != nil {
		if resp.Sandbox.Reason != "" {
			reason = resp.Sandbox.Reason
		}
		if resp.Sandbox.PutBack {
			return nodeSiteMove{kind: "put_back", reason: reason, detail: resp.Error}
		}
	}
	switch {
	case resp.Error != "":
		return nodeSiteMove{kind: "skipped", reason: reason, detail: detail}
	case resp.Sandbox == nil || resp.Sandbox.Generation < sitesandbox.Generation:
		// An older Agent, or a unit it kept: nothing moved.
		return nodeSiteMove{kind: "skipped", reason: "not_moved"}
	case c.running:
		return nodeSiteMove{kind: "moved"}
	}
	return nodeSiteMove{kind: "rewritten"}
}

// Node.js install step (alpha.127): POST /api/v1/domains/{id}/app/packages
// starts npm ci (or npm install) and, when asked, npm run build in the site
// sandbox; GET reads it. Polling only reads.
var (
	nodeSiteInstallCall = func(p *Panel, ctx context.Context, req *transport.NodeSiteInstallRequest) (transport.NodeSiteInstallResponse, error) {
		var resp transport.NodeSiteInstallResponse
		err := p.callAgentContext(ctx, "Agent.NodeSiteInstallV1", req, &resp)
		return resp, err
	}
	nodeSiteInstallStatusCall = func(p *Panel, ctx context.Context, req *transport.NodeSiteInstallStatusRequest) (transport.NodeSiteInstallResponse, error) {
		var resp transport.NodeSiteInstallResponse
		err := p.callAgentContext(ctx, "Agent.NodeSiteInstallStatusV1", req, &resp)
		return resp, err
	}
)

func (p *Panel) handleDomainAppInstall(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	request, state, err := p.appUnitRequest(r.Context(), domainID)
	if err != nil {
		writeClientError(w, http.StatusConflict, "this domain is not a Node.js site with a start command and a Node.js version")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	if r.Method == http.MethodGet {
		resp, err := nodeSiteInstallStatusCall(p, ctx, &transport.NodeSiteInstallStatusRequest{SiteID: state.SiteID, WorkDir: request.WorkDir})
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	var body struct {
		RequestID string `json:"request_id"`
		Build     bool   `json:"build"`
		// Script (alpha.128, G19): run only this package.json script; the
		// Agent checks that package.json names it.
		Script string `json:"script"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || !refreshRequestIDPattern.MatchString(body.RequestID) {
		writeClientError(w, http.StatusBadRequest, "a request id is required")
		return
	}
	if body.Script != "" && !nodesite.ValidScriptName(body.Script) {
		writeClientError(w, http.StatusBadRequest, "the script name is not a package.json script name")
		return
	}
	// D-043: npm does not run on a Node.js version a security emergency
	// withdrew, as the application does not start there.
	if p.refuseAppStartUnderEmergency(w, r, domainID, "start") {
		return
	}
	resp, err := nodeSiteInstallCall(p, ctx, &transport.NodeSiteInstallRequest{
		ExpectedBuildCommit: request.ExpectedBuildCommit, RequestID: body.RequestID, SiteID: state.SiteID,
		Domain: state.DomainName, WorkDir: request.WorkDir, RunAsUser: request.RunAsUser, NodeVersion: request.NodeVersion, Build: body.Build,
		Script: body.Script,
	})
	if err != nil {
		writeServerError(w, err)
		return
	}
	if resp.RequestID == body.RequestID && resp.Phase != "" {
		p.auditDetailed(r, "site.node_install", "domain", domainID, map[string]any{
			"site": state.DomainName, "request_id": body.RequestID, "build": body.Build, "script": body.Script, "phase": resp.Phase,
		})
	}
	_ = json.NewEncoder(w).Encode(resp)
}
