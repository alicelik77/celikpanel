package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/auth"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Security emergency endpoints (alpha.117, D-043), administrator only
// (isAdminOnlyPath):
//
//	GET  /api/v1/security-emergencies                      list
//	GET  /api/v1/security-emergencies/preview?component=&version=
//	POST /api/v1/security-emergencies                      step 1: withdraw
//	GET  /api/v1/security-emergencies/{id}                 detail, guidance, progress
//	POST /api/v1/security-emergencies/{id}/stop            step 2 (confirmation)
//	POST /api/v1/security-emergencies/{id}/move            step 3 (confirmation)
//	POST /api/v1/security-emergencies/{id}/remove          step 4 (confirmation)
//	POST /api/v1/security-emergencies/{id}/reenable        password
//	POST /api/v1/security-emergencies/{id}/cancel          undo step 1
//	POST /api/v1/security-emergencies/{id}/continue        a failed step again
//
// A GET only reads; it never starts or continues an operation. A repeated
// POST of a step that runs or was reached answers with that operation.

const securityEmergenciesPath = "/api/v1/security-emergencies"

const (
	errCodeEmergencyExists        = "EMERGENCY_EXISTS"
	errCodeEmergencyBusy          = "EMERGENCY_BUSY"
	errCodeEmergencyWrongStage    = "EMERGENCY_WRONG_STAGE"
	errCodeEmergencyBadTarget     = "EMERGENCY_BAD_TARGET"
	errCodeEmergencyBadMoveTarget = "EMERGENCY_BAD_MOVE_TARGET"
	errCodeEmergencyReason        = "EMERGENCY_REASON_REQUIRED"
	errCodeEmergencyNameMismatch  = "EMERGENCY_NAME_MISMATCH"
	errCodeEmergencyWrongPassword = "EMERGENCY_WRONG_PASSWORD"
	errCodeEmergencyImpersonation = "EMERGENCY_IMPERSONATION_REFUSED"
	errCodeEmergencyTooMany       = "EMERGENCY_TOO_MANY_ATTEMPTS"
	errCodeEmergencyMaintenance   = "SECURITY_EMERGENCY_MAINTENANCE"
	errCodeVersionWithdrawn       = "VERSION_WITHDRAWN"
)

// emergencyGuidance is the D-024 summary shown before the step list.
type emergencyGuidance struct {
	Result     string `json:"result"`
	Reason     string `json:"reason"`
	WhoActs    string `json:"who_acts"`
	NextAction string `json:"next_action"`
	Resumes    string `json:"resumes"`
}

type emergencyEffectView struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type emergencyOperationView struct {
	ID            string                `json:"id"`
	Kind          string                `json:"kind"`
	State         string                `json:"state"`
	TargetVersion string                `json:"target_version,omitempty"`
	Reason        string                `json:"reason"`
	Attempts      int                   `json:"attempts"`
	ErrorKind     string                `json:"error_kind,omitempty"`
	LastError     string                `json:"last_error,omitempty"`
	CreatedAt     string                `json:"created_at"`
	FinishedAt    string                `json:"finished_at,omitempty"`
	Done          int                   `json:"done"`
	Total         int                   `json:"total"`
	Effects       []emergencyEffectView `json:"effects"`
}

type emergencyDetailView struct {
	Emergency    securityEmergency       `json:"emergency"`
	Guidance     emergencyGuidance       `json:"guidance"`
	Operation    *emergencyOperationView `json:"operation"`
	Preview      emergencyPreview        `json:"preview"`
	Notices      emergencyNoticeSummary  `json:"notices"`
	Mail         accountMailStatus       `json:"mail"`
	ConfirmName  string                  `json:"confirm_name"`
	Alternatives []string                `json:"alternatives"`
	Can          map[string]bool         `json:"can"`
	// Dependents (alpha.123, D-045): what requires the component or version
	// by the dependency rule, shown before the removal step. The emergency
	// path is not blocked by it; the administrator acts knowingly.
	Dependents []removalDependent `json:"dependents"`
}

func (p *Panel) handleSecurityEmergencies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, securityEmergenciesPath), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		list, err := listEmergencies(r.Context(), p.db.GetDB())
		if err != nil {
			writeServerError(w, err)
			return
		}
		views := []emergencyDetailView{}
		for _, e := range list {
			view, err := p.emergencyDetail(r.Context(), e, false)
			if err != nil {
				writeServerError(w, err)
				return
			}
			views = append(views, view)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"emergencies": views})
	case rest == "" && r.Method == http.MethodPost:
		p.handleEmergencyWithdraw(w, r, caller)
	case rest == "preview" && r.Method == http.MethodGet:
		component, version := r.URL.Query().Get("component"), r.URL.Query().Get("version")
		if !validEmergencyTarget(component, version) {
			writeCodedError(w, http.StatusBadRequest, errCodeEmergencyBadTarget, "Name a component (php, node, mariadb or postgresql) and, for PHP and Node.js, a version.", "")
			return
		}
		sites, err := emergencyAffectedSites(r.Context(), p.db.GetDB(), component, version)
		if err != nil {
			writeServerError(w, err)
			return
		}
		preview, err := buildEmergencyPreview(r.Context(), p.db.GetDB(), component, version, sites)
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(preview)
	default:
		id, action, _ := strings.Cut(rest, "/")
		e, err := loadEmergency(r.Context(), p.db.GetDB(), id)
		if errors.Is(err, errEmergencyNotFound) {
			writeClientError(w, http.StatusNotFound, "security emergency not found")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		if action == "" && r.Method == http.MethodGet {
			view, err := p.emergencyDetail(r.Context(), e, true)
			if err != nil {
				writeServerError(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(view)
			return
		}
		if r.Method != http.MethodPost {
			writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		p.handleEmergencyStep(w, r, caller, e, action)
	}
}

func (p *Panel) handleEmergencyWithdraw(w http.ResponseWriter, r *http.Request, caller *Caller) {
	if caller.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusForbidden, errCodeEmergencyImpersonation, "A security emergency cannot be declared while signed in as another account. Return to your own account first.", "")
		return
	}
	var req struct {
		Component   string `json:"component"`
		Version     string `json:"version"`
		Reason      string `json:"reason"`
		Recommended string `json:"recommended"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyReason, "Write the reason (for example the CVE number); customers are told it.", "")
		return
	}
	e, err := p.createEmergency(r.Context(), req.Component, req.Version, req.Reason, req.Recommended, caller.ID)
	switch {
	case errors.Is(err, errEmergencyExists):
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "An emergency for " + e.Name + " is already open; continue it.", "code": errCodeEmergencyExists, "emergency": e})
		return
	case errors.Is(err, errEmergencyBadTarget):
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyBadTarget, "Name a component (php, node, mariadb or postgresql) and, for PHP and Node.js, a version.", "")
		return
	case errors.Is(err, errEmergencyBadMoveGoal):
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyBadMoveTarget, "The recommended alternative must be another version of the same component.", "")
		return
	case err != nil:
		writeServerError(w, err)
		return
	}
	sites, _ := emergencyAffectedSites(r.Context(), p.db.GetDB(), e.Component, e.Version)
	p.auditDetailed(r, "security.emergency.withdraw:"+e.Name, "service", 0, map[string]any{
		"emergency": e.ID, "component": e.Component, "version": e.Version, "reason": e.Reason,
		"recommended": e.Recommended, "sites_keep_running": len(sites)})
	view, err := p.emergencyDetail(r.Context(), e, true)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(view)
}

// verifyEmergencyConfirmation checks the serious confirmation of steps 2-4
// (and the password of re-enable). It writes the refusal and says false.
// The password is checked against the account and never logged.
func (p *Panel) verifyEmergencyConfirmation(w http.ResponseWriter, r *http.Request, caller *Caller, e securityEmergency, c emergencyConfirmation, needName bool) bool {
	if caller.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusForbidden, errCodeEmergencyImpersonation, "This step cannot be confirmed while signed in as another account. Return to your own account first.", "")
		return false
	}
	if strings.TrimSpace(c.Reason) == "" {
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyReason, "Write the reason for this step (for example the CVE number).", "")
		return false
	}
	if needName && strings.TrimSpace(c.ConfirmName) != e.Name {
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyNameMismatch, "Type the name exactly as shown: "+e.Name+". Nothing was changed.", "")
		return false
	}
	if c.Password == "" {
		writeCodedError(w, http.StatusForbidden, errCodeEmergencyWrongPassword, "Enter your password again to confirm. Nothing was changed.", "")
		return false
	}
	if !p.allowSensitiveAuthAttempt(r, caller.ID, reauthAttemptAction) {
		writeCodedError(w, http.StatusTooManyRequests, errCodeEmergencyTooMany, "Too many password attempts; wait a few minutes and try again. Nothing was changed.", "")
		return false
	}
	user, err := p.users.GetByID(r.Context(), caller.ID)
	if err != nil {
		writeServerError(w, err)
		return false
	}
	ok, err := auth.VerifyPassword(c.Password, user.PasswordHash)
	if err != nil || !ok {
		p.auditDetailed(r, "security.emergency.confirm_refused:"+e.Name, "service", 0, map[string]any{"emergency": e.ID, "cause": "password"})
		writeCodedError(w, http.StatusForbidden, errCodeEmergencyWrongPassword, "The password is not correct. Nothing was changed.", "")
		return false
	}
	return true
}

func (p *Panel) handleEmergencyStep(w http.ResponseWriter, r *http.Request, caller *Caller, e securityEmergency, action string) {
	var c emergencyConfirmation
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid request")
		return
	}
	ctx := r.Context()
	switch action {
	case "cancel":
		if caller.ImpersonatorID > 0 {
			writeCodedError(w, http.StatusForbidden, errCodeEmergencyImpersonation, "Return to your own account first.", "")
			return
		}
		updated, err := p.cancelWithdrawal(ctx, e.ID, caller.ID)
		if errors.Is(err, errEmergencyWrongStage) {
			writeCodedError(w, http.StatusConflict, errCodeEmergencyWrongStage, "Only a withdrawal that was not followed by a stop can be undone here; re-enable instead.", "")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "security.emergency.withdraw_cancel:"+e.Name, "service", 0, map[string]any{"emergency": e.ID, "reason": strings.TrimSpace(c.Reason)})
		p.writeEmergencyDetail(w, ctx, updated, http.StatusOK)
		return
	case "continue":
		if caller.ImpersonatorID > 0 {
			writeCodedError(w, http.StatusForbidden, errCodeEmergencyImpersonation, "Return to your own account first.", "")
			return
		}
		result, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE security_emergency_operations SET state = 'running', attempts = 0, finished_at = NULL, updated_at = datetime('now')
			WHERE id = (SELECT id FROM security_emergency_operations WHERE emergency_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1)
			  AND state = 'failed'`, e.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if n, _ := result.RowsAffected(); n == 1 {
			p.auditDetailed(r, "security.emergency.continue:"+e.Name, "service", 0, map[string]any{"emergency": e.ID})
			p.kickEmergencyWorker()
		}
		p.writeEmergencyDetail(w, ctx, e, http.StatusAccepted)
		return
	case emergencyOpStop, emergencyOpMove, emergencyOpRemove:
		if !p.verifyEmergencyConfirmation(w, r, caller, e, c, true) {
			return
		}
	case emergencyOpReenable:
		if !p.verifyEmergencyConfirmation(w, r, caller, e, c, false) {
			return
		}
		if e.Stage == emergencyStageWithdrawn {
			updated, err := p.cancelWithdrawal(ctx, e.ID, caller.ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			p.auditDetailed(r, "security.emergency.reenable:"+e.Name, "service", 0, map[string]any{"emergency": e.ID, "reason": strings.TrimSpace(c.Reason), "withdrawn": false})
			p.writeEmergencyDetail(w, ctx, updated, http.StatusOK)
			return
		}
	default:
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	accepted, err := p.acceptEmergencyOperation(ctx, e.ID, action, c, caller.ID)
	switch {
	case errors.Is(err, errEmergencyBusy):
		writeCodedError(w, http.StatusConflict, errCodeEmergencyBusy, "Another step of this emergency is running; wait for it to finish (this screen follows it), then try again.", "")
		return
	case errors.Is(err, errEmergencyWrongStage):
		writeCodedError(w, http.StatusConflict, errCodeEmergencyWrongStage, emergencyWrongStageText(e, action), "")
		return
	case errors.Is(err, errEmergencyBadMoveGoal):
		writeCodedError(w, http.StatusBadRequest, errCodeEmergencyBadMoveTarget, "Choose another installed version of "+emergencyComponents[e.Component].Name+" that is not withdrawn.", "")
		return
	case err != nil:
		writeServerError(w, err)
		return
	}
	if accepted.Started {
		details := map[string]any{"emergency": e.ID, "operation": accepted.OperationID, "reason": boundedEmergencyReason(c.Reason), "target": e.Name}
		if action == emergencyOpMove {
			details["to"] = strings.TrimSpace(c.TargetVersion)
		}
		if action == emergencyOpReenable {
			details["keep_withdrawn"] = c.KeepWithdrawn
		}
		if action == emergencyOpRemove {
			details["data_deleted"] = false
		}
		if sites, err := emergencyRecordedSites(ctx, p.db.GetDB(), e.ID); err == nil {
			names := []string{}
			for _, site := range sites {
				if site.Maintenance {
					names = append(names, site.Domain)
				}
			}
			details["sites"] = names
		}
		p.auditDetailed(r, "security.emergency."+action+":"+e.Name, "service", 0, details)
	}
	updated, err := loadEmergency(ctx, p.db.GetDB(), e.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.writeEmergencyDetail(w, ctx, updated, http.StatusAccepted)
}

func emergencyWrongStageText(e securityEmergency, action string) string {
	switch action {
	case emergencyOpStop:
		return e.Name + " is already stopped; follow the step on this screen."
	case emergencyOpMove:
		return "Sites are moved after " + e.Name + " is stopped (step 2)."
	case emergencyOpRemove:
		return e.Name + " is removed only after it is stopped (step 2)."
	}
	return "This step does not apply to " + e.Name + " now."
}

func (p *Panel) writeEmergencyDetail(w http.ResponseWriter, ctx context.Context, e securityEmergency, status int) {
	view, err := p.emergencyDetail(ctx, e, true)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(view)
}

// emergencyDetail reads everything the screen shows. full adds the installed
// alternatives (an Agent read; unknown is an empty list).
func (p *Panel) emergencyDetail(ctx context.Context, e securityEmergency, full bool) (emergencyDetailView, error) {
	db := p.db.GetDB()
	view := emergencyDetailView{Emergency: e, ConfirmName: e.Name, Alternatives: []string{}, Can: map[string]bool{}, Dependents: []removalDependent{}}
	if full {
		view.Dependents = p.emergencyDependents(ctx, e.Component, e.Version)
	}
	var sites []emergencySite
	var err error
	if e.Stage == emergencyStageWithdrawn {
		sites, err = emergencyAffectedSites(ctx, db, e.Component, e.Version)
	} else {
		sites, err = emergencyRecordedSites(ctx, db, e.ID)
	}
	if err != nil {
		return view, err
	}
	if view.Preview, err = buildEmergencyPreview(ctx, db, e.Component, e.Version, sites); err != nil {
		return view, err
	}
	if view.Notices, err = loadEmergencyNoticeSummary(ctx, db, e.ID); err != nil {
		return view, err
	}
	opID, err := latestEmergencyOperation(ctx, db, e.ID)
	if err != nil {
		return view, err
	}
	var op *emergencyOperationRow
	if opID != "" {
		row, err := loadEmergencyOperation(ctx, db, opID)
		if err != nil {
			return view, err
		}
		op = &row
		effects, err := loadEmergencyEffects(ctx, db, opID)
		if err != nil {
			return view, err
		}
		ov := &emergencyOperationView{ID: row.ID, Kind: row.Kind, State: row.State, TargetVersion: row.TargetVersion, Reason: row.Reason,
			Attempts: row.Attempts, ErrorKind: row.ErrorKind, LastError: row.LastError, CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt,
			Total: len(effects), Effects: []emergencyEffectView{}}
		for _, effect := range effects {
			if effectFinished(effect.State) {
				ov.Done++
			}
			ov.Effects = append(ov.Effects, emergencyEffectView{Kind: effect.Kind, Target: effect.Target, State: effect.State, Detail: effect.Detail})
		}
		view.Operation = ov
	}
	running := op != nil && op.State == "running"
	open := e.Stage != emergencyStageResolved
	view.Can["stop"] = open && !running && e.Stage == emergencyStageWithdrawn
	view.Can["move"] = open && !running && e.Version != "" && (e.Stage == emergencyStageStopped || e.Stage == emergencyStageRemoved)
	view.Can["remove"] = open && !running && e.Stage == emergencyStageStopped
	view.Can["reenable"] = open && !running
	view.Can["cancel"] = open && !running && e.Stage == emergencyStageWithdrawn
	view.Can["continue"] = op != nil && op.State == "failed"
	if full {
		view.Mail = p.accountMailSetup(ctx).accountMailStatus
		view.Alternatives = p.emergencyAlternatives(ctx, e)
	}
	view.Guidance = emergencyGuidanceFor(e, op, view.Preview, view.Notices)
	return view, nil
}

// emergencyAlternatives: the other installed versions, not withdrawn.
func (p *Panel) emergencyAlternatives(ctx context.Context, e securityEmergency) []string {
	out := []string{}
	var installed []string
	switch e.Component {
	case "php":
		versions, err := p.phpVersionsFromAgent(ctx)
		if err != nil {
			return out
		}
		installed = versions
	case "node":
		var resp transport.NodeVersionsResponse
		if err := p.callAgentContext(ctx, "Agent.ListNodeVersions", &transport.Empty{}, &resp); err != nil {
			return out
		}
		installed = resp.Installed
	default:
		return out
	}
	withdrawn, err := withdrawnVersions(ctx, p.db.GetDB())
	if err != nil {
		return out
	}
	for _, version := range installed {
		if version != e.Version && validEmergencyTarget(e.Component, version) && !versionWithdrawn(withdrawn, e.Component, version) {
			out = append(out, version)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func emergencyGuidanceFor(e securityEmergency, op *emergencyOperationRow, preview emergencyPreview, notices emergencyNoticeSummary) emergencyGuidance {
	g := emergencyGuidance{Reason: e.Reason, WhoActs: "administrator"}
	sites := 0
	for _, site := range preview.Sites {
		if site.Maintenance {
			sites++
		}
	}
	stepName := map[string]string{emergencyOpStop: "Emergency stop", emergencyOpMove: "Moving the sites", emergencyOpRemove: "Emergency removal", emergencyOpReenable: "Re-enabling"}
	if op != nil {
		switch op.State {
		case "running":
			g.Result = stepName[op.Kind] + " is running."
			g.WhoActs = "nobody"
			g.NextAction = "Nothing needs to be done; this screen follows it."
			g.Resumes = "CelikPanel continues it by itself, also after a restart."
			if op.ErrorKind == "unknown" {
				g.Result = stepName[op.Kind] + " is waiting to try a step again: " + op.LastError
			}
			return g
		case "failed":
			switch op.ErrorKind {
			case "prerequisite":
				g.Result = stepName[op.Kind] + " is waiting for something only the administrator can do."
			case "unknown":
				g.Result = stepName[op.Kind] + " stopped: the result of a step is not known."
			default:
				g.Result = stepName[op.Kind] + " failed."
			}
			g.NextAction = op.LastError + " — then press Continue; finished steps are not repeated."
			g.Resumes = "Continue picks up at the step that failed."
			return g
		case "partial":
			if e.Stage != emergencyStageResolved {
				g.Result = fmt.Sprintf("Some sites could not be moved; %d site(s) stay on the maintenance page.", sites)
				g.NextAction = "Read each site's reason below, fix it, then move the sites again."
				g.Resumes = "Moving again takes only the sites still on the maintenance page."
				return g
			}
		}
	}
	switch e.Stage {
	case emergencyStageWithdrawn:
		g.Result = e.Name + " is withdrawn for security: nobody can choose it for anything new; " + fmt.Sprint(len(preview.Sites)) + " site(s) keep running on it."
		g.NextAction = "If the risk remains while it runs, stop it (step 2). When a patch is installed, re-enable it."
		g.Resumes = "Nothing waits; each step starts only when you confirm it."
	case emergencyStageStopped:
		g.Result = fmt.Sprintf("%s is stopped and locked; %d site(s) show the maintenance page.", e.Name, sites)
		g.NextAction = "Move the sites to another version (step 3); if stopping is not enough, remove it (step 4); when a patch is installed, re-enable it."
		g.Resumes = "Customers can also switch their own sites from the notice they see."
	case emergencyStageRemoved:
		g.Result = fmt.Sprintf("%s is removed; %d site(s) stay on the maintenance page. No customer data was deleted.", e.Name, sites)
		g.NextAction = "Move the sites to another version, or install the patched version and re-enable it."
		g.Resumes = "Re-enable unlocks and starts it again once it is installed."
	case emergencyStageResolved:
		g.Result = e.Name + " is re-enabled; the emergency is closed."
		g.WhoActs = "nobody"
		g.NextAction = "Nothing needs to be done."
		g.Resumes = ""
	}
	if notices.NotAvailable > 0 {
		g.Result += " Notices are shown in the panel only: " + emergencyMailReasonText(notices.MailReason) + "."
	}
	return g
}

// refuseWithdrawnVersion refuses choosing a version withdrawn for security,
// for everyone, the administrator too (D-043 step 1). A site keeping the
// version it runs is never refused.
func (p *Panel) refuseWithdrawnVersion(w http.ResponseWriter, r *http.Request, component, current, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.TrimSpace(current) == requested {
		return false
	}
	withdrawn, err := withdrawnVersions(r.Context(), p.db.GetDB())
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !versionWithdrawn(withdrawn, component, requested) {
		return false
	}
	// "*" asks about a whole component (a database engine).
	name := emergencyComponents[component].Name
	if requested != "*" {
		name += " " + boundedAgentDiagnostic(requested)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{
		Error: "Nothing was changed: " + name + " is withdrawn for security on this server. Choose another one.",
		Code:  errCodeVersionWithdrawn,
		Vars:  map[string]string{"name": name},
	})
	return true
}

// refuseAppStartUnderEmergency refuses starting a Node.js application that
// a security emergency stopped.
func (p *Panel) refuseAppStartUnderEmergency(w http.ResponseWriter, r *http.Request, domainID int, action string) bool {
	if action != "start" && action != "restart" {
		return false
	}
	under, err := siteUnderEmergencyMaintenance(r.Context(), p.db.GetDB(), domainID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if !under {
		return false
	}
	writeCodedError(w, http.StatusConflict, errCodeEmergencyMaintenance,
		"This application was stopped because its Node.js version has a security problem. Switch the site to another version; it starts again there.", "")
	return true
}
