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
)

// startSideDoorsClose runs the step once after the start, in the background.
func (p *Panel) startSideDoorsClose() {
	go func() {
		time.Sleep(sideDoorsFirstWait)
		for attempt := 0; attempt < sideDoorsAttempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), agentRPCDeploymentTimeout+2*time.Minute)
			done := p.closeSideDoorsOnce(ctx)
			cancel()
			if done {
				return
			}
			time.Sleep(sideDoorsRetry)
		}
		log.Printf("side doors: the step could not run (the server stayed busy or the Agent did not answer); it runs again at the next start")
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

// recordSideDoorsState keeps the Agent's latest answer.
func (p *Panel) recordSideDoorsState(ctx context.Context, state *transport.SideDoorsState) {
	if state == nil {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingSideDoorsState, string(raw)); err != nil {
		log.Printf("side doors: state not recorded: %v", err)
	}
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
// for what the server owner undid (S4 R1 fix round, 21 Nov 2026): POST
// {"target":"at"} or {"target":"stock_pool","version":"8.4"}, administrator
// only (the /api/v1/metrics/ prefix). It runs the side doors step once with
// the owner's undo forgotten for that target, keeps the answer, audits it and
// says whether the door is closed now. The screen asks first in the panel's
// own window and shows the work while it runs.
// Sahibin geri aldığını yönetici açıkça yeniden kapatır.
func (p *Panel) handleSideDoorsReClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
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
	case transport.SideDoorsReCloseAt, transport.SideDoorsReCloseCron, transport.SideDoorsReCloseLinger:
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
	ctx, cancel := context.WithTimeout(r.Context(), agentRPCDeploymentTimeout+time.Minute)
	defer cancel()
	details := map[string]any{"target": request.Target, "php": request.Version}
	response, err := closeSideDoorsThroughAgent(p, ctx, request.Target, request.Version)
	if err != nil {
		p.auditDetailed(r, "side_doors.reclose.failed:"+request.Target, "service", 0, details)
		writeClientError(w, http.StatusConflict, "the side doors step could not run now: "+auditReason(err.Error()))
		return
	}
	p.applySideDoorsAnswer(ctx, response.State)
	closed := sideDoorClosed(response.State, request.Target, request.Version)
	action := "side_doors.reclose:" + request.Target
	if !closed {
		action = "side_doors.reclose.failed:" + request.Target
	}
	p.auditDetailed(r, action, "service", 0, details)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"closed": closed, "state": sideDoorState(response.State, request.Target, request.Version)})
}

var sideDoorsPHPVersion = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// sideDoorState is the target's state in the step's answer ("" when absent).
func sideDoorState(state *transport.SideDoorsState, target, version string) string {
	if state == nil {
		return ""
	}
	switch target {
	case transport.SideDoorsReCloseAt:
		if state.At == nil {
			return ""
		}
		return state.At.State
	case transport.SideDoorsReCloseCron:
		if state.Cron == nil {
			return ""
		}
		return state.Cron.State
	case transport.SideDoorsReCloseLinger:
		if state.Linger == nil {
			return ""
		}
		return state.Linger.State
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
		return target == transport.SideDoorsReCloseAt || target == transport.SideDoorsReCloseCron || target == transport.SideDoorsReCloseLinger
	case transport.SideDoorsStockTurnedOff, transport.SideDoorsStockOff:
		return target == transport.SideDoorsReCloseStockPool
	}
	return false
}
