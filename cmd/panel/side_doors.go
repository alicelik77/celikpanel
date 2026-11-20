package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
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

var closeSideDoorsThroughAgent = func(p *Panel, ctx context.Context) (transport.SideDoorsResponse, error) {
	var response transport.SideDoorsResponse
	err := p.withStandaloneAgentMutation(ctx, sideDoorsJobKind, sideDoorsJobTarget, "", func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.ServiceMutationRequest{ServiceMutationBinding: binding}
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
	response, err := closeSideDoorsThroughAgent(p, ctx)
	if err != nil {
		if strings.Contains(err.Error(), "can't find method") {
			// An Agent older than this Panel: nothing to do until it is updated.
			log.Printf("side doors: the Agent does not have the step yet: %v", err)
			return true
		}
		log.Printf("side doors: %v", err)
		return false
	}
	p.recordSideDoorsState(ctx, response.State)
	if response.State != nil {
		for _, tool := range response.State.Tools {
			action := ""
			switch tool.State {
			case transport.SideDoorsToolMoved:
				action = "php_tool.account.moved:"
			case transport.SideDoorsToolFailed:
				action = "php_tool.account.failed:"
			}
			if action != "" {
				p.recordAudit(auditRecord{
					Action:       action + tool.Tool,
					ResourceType: "service",
					Details:      auditDetailsText(map[string]any{"tool": tool.Tool, "php": tool.Version, "account": tool.Account, "detail": tool.Detail, "automatic": true}),
				})
			}
		}
		for _, pool := range response.State.StockPools {
			action := ""
			switch pool.State {
			case transport.SideDoorsStockTurnedOff:
				action = "php_fpm.stock_pool.turned_off:"
			case transport.SideDoorsStockFailed:
				action = "php_fpm.stock_pool.failed:"
			}
			if action != "" {
				p.recordAudit(auditRecord{
					Action:       action + pool.Version,
					ResourceType: "service",
					Details:      auditDetailsText(map[string]any{"path": pool.Path, "detail": pool.Detail, "automatic": true}),
				})
			}
		}
	}
	return true
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
			problems = append(problems, monitoringProblem{
				Key: "tool_account_owner_config:" + tool.Tool, Kind: "tool_account_owner_config", Severity: "warning",
				Actor: "server_owner", Name: name, Subject: sideDoorsToolConfPath(tool.Tool), ObservedAt: state.CheckedAt, Action: action,
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
