package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Side doors (S4 R1, 20 Nov 2026, tenant isolation; the Agent's
// cmd/agent/side_doors.go). Once after each start, when the server is quiet,
// the Panel asks the Agent in its own job (side_doors_close) to move the
// database tools and webmail to their own accounts and to turn off the
// distribution's untouched, unused stock PHP-FPM pool. The answer is kept in
// panel_settings; what needs someone (a move that failed, a configuration or
// pool the owner changed, a pool something still uses) is shown in Problems
// now with who acts and what to do (D-024). Reading it never starts a change.
//
// The states are read again without changing anything (S4 R1, second fix
// round, B7): after every move in a tool's Manage window and in an hourly
// round (Agent.ReadSideDoorsV1), so a tool that stopped answering has its
// Problems line without a Panel restart, and a line whose cause is gone goes
// by itself.
//
// Yan kapılar: her başlangıçtan sonra bir kez araçlar kendi hesaplarına
// taşınır ve dokunulmamış, kullanılmayan varsayılan havuz kapatılır; birinin
// yapması gereken şey Şu anki sorunlar'da gösterilir.

const (
	settingSideDoorsState = "side_doors_state_v1"
	sideDoorsJobKind      = "side_doors_close"
	sideDoorsJobTarget    = "side-doors"
)

var (
	sideDoorsFirstWait = 3 * time.Minute
	sideDoorsRetry     = 5 * time.Minute
	sideDoorsAttempts  = 12
	// sideDoorsReadEvery: the hourly round that reads the states again.
	sideDoorsReadEvery = time.Hour
	// After a Manage-window move: read again now, and again while the Agent
	// does not answer (it may be restarting).
	sideDoorsAfterMoveRetry    = 20 * time.Second
	sideDoorsAfterMoveAttempts = 6
)

// startSideDoorsClose runs the step once after the start, then reads the
// states again every hour, in the background.
func (p *Panel) startSideDoorsClose() {
	go func() {
		time.Sleep(sideDoorsFirstWait)
		closed := false
		for attempt := 0; attempt < sideDoorsAttempts && !closed; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), agentRPCDeploymentTimeout+2*time.Minute)
			closed = p.closeSideDoorsOnce(ctx)
			cancel()
			if !closed {
				time.Sleep(sideDoorsRetry)
			}
		}
		if !closed {
			log.Printf("side doors: the step could not run (the server stayed busy or the Agent did not answer); it runs again at the next start")
		}
		for {
			time.Sleep(sideDoorsReadEvery)
			if err := p.readSideDoorsOnce(); err != nil {
				log.Printf("side doors: the hourly read did not answer: %v", err)
			}
		}
	}()
}

// readSideDoorsOnce reads the states again through the Agent (nothing is
// changed) and keeps them; an Agent older than this Panel is not an error.
func (p *Panel) readSideDoorsOnce() error {
	ctx, cancel := context.WithTimeout(context.Background(), agentRPCNetworkReadTimeout+30*time.Second)
	defer cancel()
	var response transport.SideDoorsResponse
	if err := p.callAgentContext(ctx, "Agent.ReadSideDoorsV1", &transport.SideDoorsReadRequest{}, &response); err != nil {
		if strings.Contains(err.Error(), "can't find method") {
			return nil
		}
		return err
	}
	if response.Error != "" {
		return errors.New(response.Error)
	}
	if response.State == nil {
		return nil
	}
	p.recordSideDoorsState(ctx, response.State)
	p.recordSiteCrontabsOutside(ctx, siteJobsCheckedAt(response.State), response.State.SiteCrontabs)
	return nil
}

// readSideDoorsAfterMove reads the states again after a move in a tool's
// Manage window, whatever came of it; while the Agent does not answer (an
// Agent restarted in the middle of the move) it tries again a few times.
var readSideDoorsAfterMove = func(p *Panel) {
	go func() {
		for attempt := 0; attempt < sideDoorsAfterMoveAttempts; attempt++ {
			if err := p.readSideDoorsOnce(); err == nil {
				return
			}
			time.Sleep(sideDoorsAfterMoveRetry)
		}
	}()
}

// closeSideDoorsThroughAgent runs the step; reclose/version is the
// administrator's explicit "close it again" (empty at each start).
var closeSideDoorsThroughAgent = func(p *Panel, ctx context.Context, reclose, version string) (transport.SideDoorsResponse, error) {
	var response transport.SideDoorsResponse
	err := p.withStandaloneAgentMutation(ctx, sideDoorsJobKind, sideDoorsJobTarget, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.SideDoorsRequest{ServiceMutationBinding: binding, ReClose: reclose, Version: version}
		if err := p.callAgentContext(callCtx, "Agent.CloseSideDoorsV1", &request, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	return response, err
}

// closeSideDoorsOnce is one attempt: done is true when the outcome is final.
func (p *Panel) closeSideDoorsOnce(ctx context.Context) bool {
	release, ok := p.tryBackgroundServiceMutation(ctx)
	if !ok {
		return false
	}
	defer release()
	response, err := closeSideDoorsThroughAgent(p, ctx, "", "")
	if err != nil {
		if strings.Contains(err.Error(), "can't find method") {
			// An Agent older than this Panel: nothing to do until it is updated.
			log.Printf("side doors: the Agent does not have the step yet: %v", err)
			return true
		}
		log.Printf("side doors: %v", err)
		return false
	}
	p.applySideDoorsAnswer(ctx, response.State)
	return true
}

// applySideDoorsAnswer keeps the step's answer and audits what it did.
func (p *Panel) applySideDoorsAnswer(ctx context.Context, state *transport.SideDoorsState) {
	response := transport.SideDoorsResponse{State: state}
	p.recordSideDoorsState(ctx, response.State)
	if response.State != nil {
		for _, tool := range response.State.Tools {
			details := auditDetailsText(map[string]any{"tool": tool.Tool, "php": tool.Version, "account": tool.Account, "detail": tool.Detail, "automatic": true})
			switch tool.State {
			case transport.SideDoorsToolMoved:
				p.recordAudit(auditRecord{Action: "php_tool.account.moved:" + tool.Tool, ResourceType: "service", Details: details})
			case transport.SideDoorsToolFailed:
				p.recordAudit(auditRecord{Action: "php_tool.account.failed:" + tool.Tool, ResourceType: "service", Details: details})
			}
		}
		for _, pool := range response.State.StockPools {
			details := auditDetailsText(map[string]any{"path": pool.Path, "detail": pool.Detail, "automatic": true})
			switch pool.State {
			case transport.SideDoorsStockTurnedOff:
				p.recordAudit(auditRecord{Action: "php_fpm.stock_pool.turned_off:" + pool.Version, ResourceType: "service", Details: details})
			case transport.SideDoorsStockFailed, transport.SideDoorsStockNotPutBack:
				p.recordAudit(auditRecord{Action: "php_fpm.stock_pool.failed:" + pool.Version, ResourceType: "service", Details: details})
			}
		}
		// 21 Nov 2026 (side_doors_jobs.go): at and the site crontabs.
		p.auditSiteAt(response.State.At)
		p.recordSiteCrontabsOutside(ctx, siteJobsCheckedAt(response.State), response.State.SiteCrontabs)
	}
}

var sideDoorsStateMu sync.Mutex

// recordSideDoorsState keeps the Agent's latest answer, merged with what is
// kept by the answer's scope (mergeSideDoorsState).
func (p *Panel) recordSideDoorsState(ctx context.Context, state *transport.SideDoorsState) {
	if state == nil {
		return
	}
	sideDoorsStateMu.Lock()
	defer sideDoorsStateMu.Unlock()
	var previous transport.SideDoorsState
	if raw := p.setting(ctx, settingSideDoorsState); raw != "" {
		_ = json.Unmarshal([]byte(raw), &previous)
	}
	raw, err := json.Marshal(mergeSideDoorsState(previous, *state))
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingSideDoorsState, string(raw)); err != nil {
		log.Printf("side doors: state not recorded: %v", err)
	}
}

// mergeSideDoorsState: the whole step's answer replaces what is kept; the
// administrator's "close it again" replaces only its door; a read again
// replaces the tools and every stock pool or at state it could read, and
// keeps the step's word for the others.
func mergeSideDoorsState(previous, fresh transport.SideDoorsState) transport.SideDoorsState {
	switch fresh.Scope {
	case "":
		return fresh
	case transport.SideDoorsScopeReClose:
		out := previous
		if fresh.At != nil {
			out.At = fresh.At
		}
		for _, row := range fresh.StockPools {
			replaced := false
			pools := make([]transport.SideDoorsStockPool, 0, len(out.StockPools)+1)
			for _, kept := range out.StockPools {
				if kept.Version == row.Version {
					kept, replaced = row, true
				}
				pools = append(pools, kept)
			}
			if !replaced {
				pools = append(pools, row)
			}
			out.StockPools = pools
		}
		if out.CheckedAt == "" {
			out.CheckedAt = fresh.CheckedAt
		}
		return out
	case transport.SideDoorsScopeRead:
		out := previous
		out.CheckedAt = fresh.CheckedAt
		out.Tools = fresh.Tools
		pools := []transport.SideDoorsStockPool{}
		for _, row := range fresh.StockPools {
			if row.State == "" {
				for _, kept := range previous.StockPools {
					if kept.Version == row.Version {
						pools = append(pools, kept)
					}
				}
				continue
			}
			pools = append(pools, row)
		}
		out.StockPools = pools
		switch {
		case fresh.At == nil:
		case fresh.At.State != "":
			out.At = fresh.At
		case out.At != nil:
			at := *out.At
			at.Jobs = fresh.At.Jobs
			out.At = &at
		}
		out.SiteCrontabs = fresh.SiteCrontabs
		return out
	}
	return previous
}

// sideDoorsToolComponent is the component page of a tool.
func sideDoorsToolComponent(tool string) string {
	if tool == "webmail" {
		return "roundcube"
	}
	return tool
}

var sideDoorsToolNames = map[string]string{
	"webmail":    "Webmail (Roundcube)",
	"phpmyadmin": "phpMyAdmin",
	"phppgadmin": "phpPgAdmin",
}

// sideDoorsProblems turns the recorded answer into Problems now.
func sideDoorsProblems(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state transport.SideDoorsState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	problems := []monitoringProblem{}
	for _, tool := range state.Tools {
		name, known := sideDoorsToolNames[tool.Tool]
		if !known {
			continue
		}
		action := monitoringProblemAction{Kind: "open", To: "/components/" + url.PathEscape(sideDoorsToolComponent(tool.Tool))}
		switch tool.State {
		case transport.SideDoorsToolFailed:
			problems = append(problems, monitoringProblem{
				Key: "tool_account_failed:" + tool.Tool, Kind: "tool_account_failed", Severity: "warning",
				Actor: "administrator", Name: name, Subject: tool.Version, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsToolOwnerConfig:
			// S4 R1 fix round: an owner's own pool that runs as the web
			// server's account names that pool.
			subject := sideDoorsToolConfPath(tool.Tool)
			if tool.Path != "" {
				subject = tool.Path
			}
			problems = append(problems, monitoringProblem{
				Key: "tool_account_owner_config:" + tool.Tool, Kind: "tool_account_owner_config", Severity: "warning",
				Actor: "server_owner", Name: name, Subject: subject, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsToolDown:
			// S4 R1 fix round: the tool does not answer as it is configured.
			problems = append(problems, monitoringProblem{
				Key: "tool_down:" + tool.Tool, Kind: "tool_down", Severity: "critical",
				Actor: "administrator", Name: name, Subject: tool.Version, ObservedAt: state.CheckedAt, Action: action,
			})
		}
	}
	for _, pool := range state.StockPools {
		action := monitoringProblemAction{Kind: "open", To: "/components/php-fpm"}
		switch pool.State {
		case transport.SideDoorsStockOwnerChanged:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_owner_changed:" + pool.Version, Kind: "stock_pool_owner_changed", Severity: "warning",
				Actor: "server_owner", Name: pool.Version, Subject: pool.Path, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsStockInUse:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_in_use:" + pool.Version, Kind: "stock_pool_in_use", Severity: "warning",
				Actor: "server_owner", Name: pool.Version, Subject: strings.Join(pool.UsedBy, ", "), ObservedAt: state.CheckedAt, Action: action,
			})
		// S4 R1 fix round: a turn-off that failed, one whose put-back failed
		// too, a pool whose state could not be read, and the owner's undo.
		case transport.SideDoorsStockFailed:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_failed:" + pool.Version, Kind: "stock_pool_failed", Severity: "warning",
				Actor: "server_owner", Name: pool.Version, Subject: pool.Path, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsStockNotPutBack:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_not_put_back:" + pool.Version, Kind: "stock_pool_not_put_back", Severity: "critical",
				Actor: "server_owner", Name: pool.Version, Subject: pool.Path, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsStockUnknown:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_unknown:" + pool.Version, Kind: "stock_pool_unknown", Severity: "warning",
				Actor: "server_owner", Name: pool.Version, Subject: pool.Path, ObservedAt: state.CheckedAt, Action: action,
			})
		case transport.SideDoorsStockOwnerRestored:
			problems = append(problems, monitoringProblem{
				Key: "stock_pool_owner_restored:" + pool.Version, Kind: "stock_pool_owner_restored", Severity: "warning",
				Actor: "administrator", Name: pool.Version, Subject: pool.Path, ObservedAt: state.CheckedAt,
				Action: monitoringProblemAction{Kind: "reclose", To: transport.SideDoorsReCloseStockPool},
			})
		}
	}
	return problems
}

// sideDoorsToolConfPath is the web server configuration CelikPanel writes
// for a tool (cmd/agent: webmailConfPath, dbToolsConfPath).
func sideDoorsToolConfPath(tool string) string {
	if tool == "webmail" {
		return "/etc/nginx/conf.d/celikpanel-webmail.conf"
	}
	return "/etc/nginx/conf.d/celikpanel-dbtools.conf"
}

// handleSideDoorsReClose is the administrator's explicit "close it again"
// for what the server owner undid (S4 R1 fix round, 21 Nov 2026), on
// /api/v1/metrics/side-doors/reclose, administrator only. POST
// {"target":"at"} or {"target":"stock_pool","version":"8.4"} starts it and
// answers 202 at once; GET says how the last one went. Since the second fix
// round (B4) it does exactly what its confirmation says: only that door, in
// the Agent's own job, run apart from the browser's request, so leaving the
// page neither cancels nor half-does it, and the result is on the screen
// when the page is opened again (the Problems now line goes, or stays with
// Monitoring saying why). A second press while one runs is the same run, not
// a second one. The owner's undo stays recorded until the door is closed
// again, so a cut or refused attempt never arms a later automatic close.
// Sahibin geri aldığını yönetici açıkça yeniden kapatır.
func (p *Panel) handleSideDoorsReClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sideDoorsRecloseStatus())
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var request struct {
		Target  string `json:"target"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request")
		return
	}
	switch request.Target {
	case transport.SideDoorsReCloseAt:
		request.Version = ""
	case transport.SideDoorsReCloseStockPool:
		if !sideDoorsPHPVersion.MatchString(request.Version) {
			writeClientError(w, http.StatusBadRequest, "invalid PHP version")
			return
		}
	default:
		writeClientError(w, http.StatusBadRequest, "unknown side door")
		return
	}
	// The audit rows are written when it ends; who asked is read now.
	uid, actingAs := auditPerson(r)
	ip, agent := clientIP(r), r.UserAgent()
	if len(agent) > 300 {
		agent = agent[:300]
	}
	details := map[string]any{"target": request.Target, "php": request.Version}
	if c := currentCaller(r); c != nil && c.APIToken != nil {
		details["api_token"] = c.APIToken.label()
	}
	auditDone := auditRecord{UserID: uid, ActingAs: actingAs, ResourceType: "service", IP: ip, UserAgent: agent, Details: auditDetailsText(details)}
	started, running := beginSideDoorsReclose(request.Target, request.Version)
	if !started {
		if running.Target == request.Target && running.Version == request.Version {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(running)
			return
		}
		writeClientError(w, http.StatusConflict, "another close-again is running; wait for it to finish")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), agentRPCDeploymentTimeout+time.Minute)
		defer cancel()
		response, err := closeSideDoorsThroughAgent(p, ctx, request.Target, request.Version)
		if err != nil {
			p.recordAudit(auditRecord{UserID: auditDone.UserID, ActingAs: auditDone.ActingAs, Action: "side_doors.reclose.failed:" + request.Target, ResourceType: auditDone.ResourceType, IP: auditDone.IP, UserAgent: auditDone.UserAgent, Details: auditDone.Details})
			endSideDoorsReclose(false, "", "the side doors step could not run now: "+auditReason(err.Error()))
			return
		}
		p.applySideDoorsAnswer(ctx, response.State)
		closed := sideDoorClosed(response.State, request.Target, request.Version)
		if closed {
			p.recordAudit(auditRecord{UserID: auditDone.UserID, ActingAs: auditDone.ActingAs, Action: "side_doors.reclose:" + request.Target, ResourceType: auditDone.ResourceType, IP: auditDone.IP, UserAgent: auditDone.UserAgent, Details: auditDone.Details})
		} else {
			p.recordAudit(auditRecord{UserID: auditDone.UserID, ActingAs: auditDone.ActingAs, Action: "side_doors.reclose.failed:" + request.Target, ResourceType: auditDone.ResourceType, IP: auditDone.IP, UserAgent: auditDone.UserAgent, Details: auditDone.Details})
		}
		endSideDoorsReclose(closed, sideDoorState(response.State, request.Target, request.Version), "")
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(sideDoorsRecloseStatus())
}

// sideDoorsReclose is the last "close it again" (one at a time).
type sideDoorsReclose struct {
	Target  string `json:"target,omitempty"`
	Version string `json:"version,omitempty"`
	Running bool   `json:"running"`
	Closed  bool   `json:"closed"`
	State   string `json:"state,omitempty"`
	Error   string `json:"error,omitempty"`
	At      string `json:"at,omitempty"`
}

var (
	sideDoorsRecloseMu   sync.Mutex
	sideDoorsRecloseLast sideDoorsReclose
)

func sideDoorsRecloseStatus() sideDoorsReclose {
	sideDoorsRecloseMu.Lock()
	defer sideDoorsRecloseMu.Unlock()
	return sideDoorsRecloseLast
}

// beginSideDoorsReclose starts one unless one runs (then it returns that one).
func beginSideDoorsReclose(target, version string) (bool, sideDoorsReclose) {
	sideDoorsRecloseMu.Lock()
	defer sideDoorsRecloseMu.Unlock()
	if sideDoorsRecloseLast.Running {
		return false, sideDoorsRecloseLast
	}
	sideDoorsRecloseLast = sideDoorsReclose{Target: target, Version: version, Running: true, At: time.Now().UTC().Format(time.RFC3339)}
	return true, sideDoorsRecloseLast
}

func endSideDoorsReclose(closed bool, state, failure string) {
	sideDoorsRecloseMu.Lock()
	defer sideDoorsRecloseMu.Unlock()
	sideDoorsRecloseLast.Running, sideDoorsRecloseLast.Closed = false, closed
	sideDoorsRecloseLast.State, sideDoorsRecloseLast.Error = state, failure
	sideDoorsRecloseLast.At = time.Now().UTC().Format(time.RFC3339)
}

var sideDoorsPHPVersion = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// sideDoorState is the target's state in the step's answer ("" when absent).
func sideDoorState(state *transport.SideDoorsState, target, version string) string {
	if state == nil {
		return ""
	}
	if target == transport.SideDoorsReCloseAt {
		if state.At == nil {
			return ""
		}
		return state.At.State
	}
	for _, pool := range state.StockPools {
		if pool.Version == version {
			return pool.State
		}
	}
	return ""
}

// sideDoorClosed: the target is closed after the step.
func sideDoorClosed(state *transport.SideDoorsState, target, version string) bool {
	switch sideDoorState(state, target, version) {
	case transport.SideDoorsAtWritten, transport.SideDoorsAtCurrent:
		return target == transport.SideDoorsReCloseAt
	case transport.SideDoorsStockTurnedOff, transport.SideDoorsStockOff:
		return target == transport.SideDoorsReCloseStockPool
	}
	return false
}
