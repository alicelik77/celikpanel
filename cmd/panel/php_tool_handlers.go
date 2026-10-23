package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Which PHP version webmail (Roundcube), phpMyAdmin and phpPgAdmin run on
// (alpha.104, owner decision of 2026-10-23 after the Frankfurt removal of
// PHP-FPM 8.4). Each tool is bound to one installed PHP-FPM version; the
// administrator chooses it in the tool's Manage window, the PHP-FPM versions
// list says who uses each version, a version a tool runs on is not removed
// unless the tools are moved first (and verified there), and a tool whose
// PHP is missing is not shown as plainly installed.
//
// Webmail, phpMyAdmin ve phpPgAdmin'in koştuğu PHP sürümü: araç başına bir
// kurulu PHP-FPM sürümü; aracın Yönet penceresinde seçilir, taşıma doğrulanır.

const (
	phpToolsPath    = "/api/v1/service/php-tools"
	phpToolBindPath = "/api/v1/service/php-tools/bind"
	// serviceUninstallPath is the removal route (request-identity guarded
	// since alpha.104).
	serviceUninstallPath = "/api/v1/service/uninstall"
	phpToolReadBound     = 10 * time.Second

	errCodePHPToolMoveFailed = "PHP_TOOL_MOVE_FAILED"
)

var phpToolVersionPattern = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// phpToolServiceIDs maps an Agent tool name to its catalogue component.
var phpToolServiceIDs = map[string]string{
	"webmail":    "roundcube",
	"phpmyadmin": "phpmyadmin",
	"phppgadmin": "phppgadmin",
}

func phpToolForService(serviceID string) string {
	for tool, id := range phpToolServiceIDs {
		if id == serviceID {
			return tool
		}
	}
	return ""
}

// readPHPToolBindings asks the Agent, read-only and bounded.
var readPHPToolBindingsFromAgent = func(p *Panel, ctx context.Context) (transport.PHPToolBindingsResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, phpToolReadBound)
	defer cancel()
	var response transport.PHPToolBindingsResponse
	if err := p.callAgentContext(callCtx, "Agent.PHPToolBindingsV1", &transport.Empty{}, &response); err != nil {
		return transport.PHPToolBindingsResponse{}, err
	}
	if response.Error != "" {
		return transport.PHPToolBindingsResponse{}, errors.New(response.Error)
	}
	return response, nil
}

func (p *Panel) readPHPToolBindings(ctx context.Context) (transport.PHPToolBindingsResponse, error) {
	return readPHPToolBindingsFromAgent(p, ctx)
}

// attachPHPToolBindings puts each installed tool's binding on its scan row.
func attachPHPToolBindings(observations []serviceObservation, bindings transport.PHPToolBindingsResponse) {
	for i := range observations {
		tool := phpToolForService(observations[i].ID)
		if tool == "" || !observations[i].IsInstalled {
			continue
		}
		for _, binding := range bindings.Tools {
			if binding.Tool == tool && binding.Installed && binding.Version != "" {
				copied := binding
				observations[i].PHPBinding = &copied
			}
		}
	}
}

func phpBindingForResponse(installed *bool, binding *transport.PHPToolBinding) *transport.PHPToolBinding {
	if installed == nil || !*installed || binding == nil {
		return nil
	}
	return binding
}

type phpToolView struct {
	Tool             string `json:"tool"`
	ServiceID        string `json:"service_id"`
	Installed        bool   `json:"installed"`
	Version          string `json:"version"`
	Ready            bool   `json:"ready"`
	VersionInstalled bool   `json:"version_installed"`
	// ExtensionsPresent: the tool's extension packages are installed for
	// its version (alpha.104 review; package presence, not proof of load).
	ExtensionsPresent bool `json:"extensions_present"`
}

// phpVersionUsage says who uses one installed PHP-FPM version.
type phpVersionUsage struct {
	Version   string   `json:"version"`
	Sites     int      `json:"sites"`
	SiteNames []string `json:"site_names"`
	Tools     []string `json:"tools"`
}

type phpToolsResponse struct {
	// State: "read" (the Agent answered now) or "unknown" (it could not be
	// read; nothing is claimed about the tools).
	State    string            `json:"state"`
	Tools    []phpToolView     `json:"tools"`
	Versions []phpVersionUsage `json:"versions"`
}

func phpToolsView(ctx context.Context, p *Panel, bindings transport.PHPToolBindingsResponse) (phpToolsResponse, error) {
	response := phpToolsResponse{State: "read", Tools: []phpToolView{}, Versions: []phpVersionUsage{}}
	for _, binding := range bindings.Tools {
		response.Tools = append(response.Tools, phpToolView{
			Tool:              binding.Tool,
			ServiceID:         phpToolServiceIDs[binding.Tool],
			Installed:         binding.Installed,
			Version:           binding.Version,
			Ready:             binding.SocketReady,
			VersionInstalled:  binding.VersionInstalled,
			ExtensionsPresent: binding.ExtensionsPresent,
		})
	}
	for _, version := range bindings.Versions {
		usage := phpVersionUsage{Version: version, SiteNames: []string{}, Tools: []string{}}
		count, lines, err := runtimeVersionBlockers(ctx, p.db.GetDB(), "php-fpm", version)
		if err != nil {
			return phpToolsResponse{}, err
		}
		usage.Sites = count
		for _, line := range lines {
			if !strings.HasPrefix(line, "+") {
				usage.SiteNames = append(usage.SiteNames, line)
			}
		}
		for _, binding := range bindings.Tools {
			if binding.Installed && binding.Version == version {
				usage.Tools = append(usage.Tools, binding.Tool)
			}
		}
		response.Versions = append(response.Versions, usage)
	}
	return response, nil
}

// handlePHPTools: GET, admin-only through the /api/v1/service/ prefix,
// read-only.
func (p *Panel) handlePHPTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bindings, err := p.readPHPToolBindings(r.Context())
	if err != nil {
		log.Printf("php tools: bindings could not be read: %v", err)
		json.NewEncoder(w).Encode(phpToolsResponse{State: "unknown", Tools: []phpToolView{}, Versions: []phpVersionUsage{}})
		return
	}
	response, err := phpToolsView(r.Context(), p, bindings)
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(response)
}

// phpToolBindFailure is the answer of a move that did not complete.
type phpToolBindFailure struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
	// Cause is the structured, translatable reason (alpha.104 review).
	Cause           *transport.PHPToolCause `json:"cause,omitempty"`
	Tool            string                  `json:"tool,omitempty"`
	Version         string                  `json:"version,omitempty"`
	Previous        string                  `json:"previous,omitempty"`
	Restored        bool                    `json:"restored,omitempty"`
	MutationApplied bool                    `json:"mutation_applied,omitempty"`
	Moved           []string                `json:"moved,omitempty"`
	NotAttempted    []string                `json:"not_attempted,omitempty"`
}

// bindPHPTool runs one verified move in its own php_tool_bind job.
var bindPHPToolThroughAgent = func(p *Panel, ctx context.Context, tool, version string) (transport.BindPHPToolResponse, error) {
	var response transport.BindPHPToolResponse
	err := p.withStandaloneAgentMutation(ctx, "php_tool_bind", tool, version, func(callCtx context.Context, binding agentMutationBinding) error {
		request := transport.BindPHPToolRequest{ServiceMutationBinding: binding, Tool: tool, Version: version}
		if err := p.callAgentContext(callCtx, "Agent.BindPHPToolV1", &request, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	if err != nil && response.Error == "" {
		return response, err
	}
	return response, nil
}

func validPHPToolRequest(tool, version string) bool {
	_, ok := phpToolServiceIDs[tool]
	return ok && phpToolVersionPattern.MatchString(version)
}

// handlePHPToolBind: POST {tool, version}. Admin-only, one service change
// at a time, request identity guarded (a replay is answered, not re-run).
func (p *Panel) handlePHPToolBind(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Tool    string `json:"tool"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validPHPToolRequest(req.Tool, req.Version) {
		writeClientError(w, http.StatusBadRequest, "tool and version are required")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()

	response, err := bindPHPToolThroughAgent(p, r.Context(), req.Tool, req.Version)
	if _, scanErr := p.scanManagedServices(r.Context()); scanErr != nil {
		log.Printf("rescan after PHP tool move: %v", scanErr)
	}
	if err != nil {
		p.audit(r, "service.php_tool_bind.failed:"+req.Tool+":"+req.Version+" — "+auditReason(err.Error()), "service", 0)
		writeAgentError(w, err, "PHP version change")
		return
	}
	if !response.Bound {
		p.audit(r, "service.php_tool_bind.failed:"+req.Tool+":"+req.Version+" — "+auditReason(response.Error), "service", 0)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(phpToolBindFailure{
			Error: response.Error, Code: phpToolBindCode(response.ErrorCode), Detail: response.Detail, Cause: response.Cause, Tool: req.Tool, Version: req.Version,
			Previous: response.Previous, Restored: response.Restored, MutationApplied: response.MutationApplied,
		})
		return
	}
	p.audit(r, "service.php_tool_bind:"+req.Tool+":"+response.Previous+"->"+req.Version, "service", 0)
	json.NewEncoder(w).Encode(map[string]any{
		"bound": true, "tool": req.Tool, "version": req.Version, "previous": response.Previous,
		"installed_packages": response.InstalledPackages, "warning": response.Warning,
	})
}

func phpToolBindCode(code string) string {
	if code == "" {
		return "PHP_TOOL_BIND_FAILED"
	}
	return code
}

// movePHPToolsBeforeVersionRemoval moves every installed tool bound to the
// PHP version being removed to target, one verified move at a time (C,
// alpha.104). It stops at the first failure: tools already moved stay on
// target (each was verified there), the failed one was put back by the
// Agent when it says so, the rest were not touched.
func (p *Panel) movePHPToolsBeforeVersionRemoval(ctx context.Context, removed, target string) ([]string, *phpToolBindFailure, error) {
	bindings, err := p.readPHPToolBindings(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read which tools run on PHP %s: %w", removed, err)
	}
	if !slices.Contains(bindings.Versions, target) || target == removed {
		return nil, &phpToolBindFailure{
			Error: fmt.Sprintf("PHP %s is not an installed version the tools can move to; nothing was changed", target),
			Code:  errCodePHPVersionNotInstalled, Version: target,
		}, nil
	}
	var tools []string
	for _, binding := range bindings.Tools {
		if binding.Installed && binding.Version == removed {
			tools = append(tools, binding.Tool)
		}
	}
	slices.Sort(tools)
	moved := []string{}
	for i, tool := range tools {
		response, err := bindPHPToolThroughAgent(p, ctx, tool, target)
		if err != nil || !response.Bound {
			message := response.Error
			if err != nil {
				message = err.Error()
			}
			return moved, &phpToolBindFailure{
				Error:           message,
				Code:            errCodePHPToolMoveFailed,
				Detail:          response.Detail,
				Cause:           response.Cause,
				Tool:            tool,
				Version:         target,
				Previous:        response.Previous,
				Restored:        response.Restored,
				MutationApplied: response.MutationApplied || len(moved) > 0,
				Moved:           moved,
				NotAttempted:    append([]string{}, tools[i+1:]...),
			}, nil
		}
		moved = append(moved, tool)
	}
	return moved, nil, nil
}
