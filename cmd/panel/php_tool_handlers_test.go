package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// PHP tools and their PHP versions (alpha.104, owner decision of 2026-10-23).

// frankfurtBindings: PHP 8.4, 8.5 and 8.6 installed; webmail and phpPgAdmin
// on 8.4, phpMyAdmin not installed (its old line still names 8.4).
func frankfurtBindings() transport.PHPToolBindingsResponse {
	return transport.PHPToolBindingsResponse{
		Versions: []string{"8.6", "8.5", "8.4"},
		Tools: []transport.PHPToolBinding{
			{Tool: "phpmyadmin", Installed: false, Version: "8.4", SocketReady: true, VersionInstalled: true, Source: "config"},
			{Tool: "phppgadmin", Installed: true, Version: "8.4", SocketReady: true, VersionInstalled: true, Source: "config"},
			{Tool: "webmail", Installed: true, Version: "8.4", SocketReady: true, VersionInstalled: true, Source: "config"},
		},
	}
}

func stubPHPToolBindings(t *testing.T, answer transport.PHPToolBindingsResponse, err error) {
	t.Helper()
	restore := readPHPToolBindingsFromAgent
	t.Cleanup(func() { readPHPToolBindingsFromAgent = restore })
	readPHPToolBindingsFromAgent = func(*Panel, context.Context) (transport.PHPToolBindingsResponse, error) {
		return answer, err
	}
}

func stubPHPToolBind(t *testing.T, answer func(tool, version string) (transport.BindPHPToolResponse, error)) *[]string {
	t.Helper()
	restore := bindPHPToolThroughAgent
	t.Cleanup(func() { bindPHPToolThroughAgent = restore })
	var calls []string
	bindPHPToolThroughAgent = func(_ *Panel, _ context.Context, tool, version string) (transport.BindPHPToolResponse, error) {
		calls = append(calls, tool+"->"+version)
		return answer(tool, version)
	}
	return &calls
}

func TestPHPToolsSaysWhoUsesEachVersion(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubPHPToolBindings(t, frankfurtBindings(), nil)
	recorder := httptest.NewRecorder()
	panel.handlePHPTools(recorder, httptest.NewRequest(http.MethodGet, phpToolsPath, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response phpToolsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.State != "read" || len(response.Versions) != 3 || len(response.Tools) != 3 {
		t.Fatalf("response = %+v", response)
	}
	byVersion := map[string]phpVersionUsage{}
	for _, usage := range response.Versions {
		byVersion[usage.Version] = usage
	}
	if !slices.Equal(byVersion["8.4"].Tools, []string{"phppgadmin", "webmail"}) || len(byVersion["8.6"].Tools) != 0 {
		t.Fatalf("usage = %+v", response.Versions)
	}
	for _, tool := range response.Tools {
		if tool.Tool == "webmail" && tool.ServiceID != "roundcube" {
			t.Fatalf("webmail service id = %q", tool.ServiceID)
		}
	}

	stubPHPToolBindings(t, transport.PHPToolBindingsResponse{}, errors.New("agent unreachable"))
	recorder = httptest.NewRecorder()
	panel.handlePHPTools(recorder, httptest.NewRequest(http.MethodGet, phpToolsPath, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"state":"unknown"`) {
		t.Fatalf("unknown read: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPHPToolBindAnswersTheMoveAndItsFailure(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	phpToolBindRequest := func(panel *Panel, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		panel.handlePHPToolBind(recorder, serviceStateAtomicRequest(http.MethodPost, phpToolBindPath, body, userID))
		return recorder
	}
	stubPHPToolBindings(t, frankfurtBindings(), nil)
	calls := stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		if version == "8.6" {
			return transport.BindPHPToolResponse{
				Tool: tool, Version: version, Previous: "8.4", Restored: true, MutationApplied: true,
				Error: "phpPgAdmin did not work on PHP 8.6 (phpPgAdmin answered HTTP 502); it was put back on PHP 8.4", ErrorCode: "PHP_TOOL_PROBE_FAILED",
			}, nil
		}
		return transport.BindPHPToolResponse{Bound: true, Tool: tool, Version: version, Previous: "8.4", InstalledPackages: []string{"php8.5-pgsql"}}, nil
	})

	recorder := phpToolBindRequest(panel, `{"tool":"phppgadmin","version":"8.5"}`)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"bound":true`) || !strings.Contains(recorder.Body.String(), `"previous":"8.4"`) {
		t.Fatalf("move: %d %s", recorder.Code, recorder.Body.String())
	}

	recorder = phpToolBindRequest(panel, `{"tool":"phppgadmin","version":"8.6"}`)
	var failure phpToolBindFailure
	if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || failure.Code != "PHP_TOOL_PROBE_FAILED" || !failure.Restored || failure.Previous != "8.4" {
		t.Fatalf("failed move: %d %+v", recorder.Code, failure)
	}

	for _, body := range []string{`{"tool":"wordpress","version":"8.5"}`, `{"tool":"webmail","version":"8"}`, `{"tool":"webmail"}`, `nope`} {
		if recorder := phpToolBindRequest(panel, body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, recorder.Code)
		}
	}
	if len(*calls) != 2 {
		t.Fatalf("agent calls = %v", *calls)
	}
	var audited int
	if err := database.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'service.php_tool_bind%'`).Scan(&audited); err != nil || audited != 2 {
		t.Fatalf("audit rows = %d err=%v", audited, err)
	}
}

func TestMovingToolsBeforeAVersionRemovalStopsAtTheFirstFailure(t *testing.T) {
	panel := newManagedServiceCachePanel(t)
	stubPHPToolBindings(t, frankfurtBindings(), nil)

	calls := stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		return transport.BindPHPToolResponse{Bound: true, Tool: tool, Version: version, Previous: "8.4"}, nil
	})
	moved, failure, err := panel.movePHPToolsBeforeVersionRemoval(context.Background(), "8.4", "8.5")
	if err != nil || failure != nil || !slices.Equal(moved, []string{"phppgadmin", "webmail"}) {
		t.Fatalf("moved=%v failure=%+v err=%v", moved, failure, err)
	}
	// The uninstalled phpMyAdmin's old line is not moved.
	if !slices.Equal(*calls, []string{"phppgadmin->8.5", "webmail->8.5"}) {
		t.Fatalf("calls = %v", *calls)
	}

	calls = stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		if tool == "phppgadmin" {
			return transport.BindPHPToolResponse{Tool: tool, Version: version, Previous: "8.4", Restored: true, MutationApplied: true, Error: "phpPgAdmin did not work on PHP 8.5; it was put back on PHP 8.4"}, nil
		}
		return transport.BindPHPToolResponse{Bound: true}, nil
	})
	moved, failure, err = panel.movePHPToolsBeforeVersionRemoval(context.Background(), "8.4", "8.5")
	if err != nil || failure == nil || len(moved) != 0 {
		t.Fatalf("moved=%v failure=%+v err=%v", moved, failure, err)
	}
	if failure.Code != errCodePHPToolMoveFailed || failure.Tool != "phppgadmin" || !failure.Restored || !slices.Equal(failure.NotAttempted, []string{"webmail"}) {
		t.Fatalf("failure = %+v", failure)
	}
	if !slices.Equal(*calls, []string{"phppgadmin->8.5"}) {
		t.Fatalf("calls after a failure = %v", *calls)
	}

	// The target must be another installed version.
	for _, target := range []string{"8.4", "8.3"} {
		_, failure, err := panel.movePHPToolsBeforeVersionRemoval(context.Background(), "8.4", target)
		if err != nil || failure == nil || failure.Code != errCodePHPVersionNotInstalled {
			t.Fatalf("target %s: failure=%+v err=%v", target, failure, err)
		}
	}
}

func TestScanRowsCarryTheToolsPHPBinding(t *testing.T) {
	observations := []serviceObservation{
		{ID: "phppgadmin", IsInstalled: true},
		{ID: "phpmyadmin", IsInstalled: false},
		{ID: "roundcube", IsInstalled: true},
		{ID: "nginx", IsInstalled: true},
	}
	bindings := frankfurtBindings()
	bindings.Tools[1].SocketReady = false // phpPgAdmin's PHP is gone
	attachPHPToolBindings(observations, bindings)
	if observations[0].PHPBinding == nil || observations[0].PHPBinding.SocketReady || observations[0].PHPBinding.Version != "8.4" {
		t.Fatalf("phppgadmin = %+v", observations[0].PHPBinding)
	}
	if observations[1].PHPBinding != nil || observations[3].PHPBinding != nil {
		t.Fatal("a binding was attached to a row that is not an installed PHP tool")
	}
	if observations[2].PHPBinding == nil || !observations[2].PHPBinding.SocketReady {
		t.Fatalf("roundcube = %+v", observations[2].PHPBinding)
	}
	installed, notInstalled := true, false
	if phpBindingForResponse(&notInstalled, observations[0].PHPBinding) != nil || phpBindingForResponse(&installed, observations[0].PHPBinding) == nil {
		t.Fatal("binding visibility")
	}
	// Stored and read back with the scan.
	data, err := json.Marshal(scanCacheDoc{Observations: observations})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeScanCache(string(data))
	if err != nil || decoded[0].PHPBinding == nil || decoded[0].PHPBinding.Version != "8.4" {
		t.Fatalf("decoded = %+v err=%v", decoded, err)
	}
}

func TestPHPToolBindIsARequestIdentityRoute(t *testing.T) {
	route, ok := requestIdentityRouteFor(httptest.NewRequest(http.MethodPost, phpToolBindPath, nil))
	if !ok || route.pattern != phpToolBindPath || route.timeout <= agentRPCDeploymentTimeout {
		t.Fatalf("route = %+v ok=%v", route, ok)
	}
	if _, ok := requestIdentityRouteFor(httptest.NewRequest(http.MethodGet, phpToolsPath, nil)); ok {
		t.Fatal("the read is guarded")
	}
	// A removal may first move three tools, then remove (alpha.104 review).
	route, ok = requestIdentityRouteFor(httptest.NewRequest(http.MethodPost, serviceUninstallPath, nil))
	if !ok || route.pattern != serviceUninstallPath || route.timeout < 4*agentRPCDeploymentTimeout {
		t.Fatalf("uninstall route = %+v ok=%v", route, ok)
	}
}

func TestVersionRemovalWithAMoveRefusesBeforeRemovingWhenAMoveFails(t *testing.T) {
	panel, _, _, userID := newServiceStateAtomicPanel(t)
	uninstall := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		panel.handleServiceUninstall(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/service/uninstall", body, userID))
		return recorder
	}
	for _, body := range []string{
		`{"service_id":"nginx","move_tools_to":"8.5"}`,
		`{"service_id":"php-fpm","move_tools_to":"8.5"}`,
		`{"service_id":"php-fpm","package":"php8.4-fpm","move_tools_to":"latest"}`,
		`{"service_id":"php-fpm","package":"php8.4-fpm","move_tools_to":"8.4"}`,
	} {
		if recorder := uninstall(body); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, recorder.Code, recorder.Body.String())
		}
	}

	stubPHPToolBindings(t, frankfurtBindings(), nil)
	stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{State: "checked", PackageFamily: "apt", Requested: []string{"php8.4-fpm"}, Remove: []string{"php8.4-fpm"}}, nil
	})
	calls := stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		if tool == "webmail" {
			return transport.BindPHPToolResponse{Tool: tool, Version: version, Previous: "8.4", Restored: true, MutationApplied: true, Error: "Webmail (Roundcube) did not work on PHP 8.5; it was put back on PHP 8.4"}, nil
		}
		return transport.BindPHPToolResponse{Bound: true, Tool: tool, Version: version, Previous: "8.4"}, nil
	})
	recorder := uninstall(`{"service_id":"php-fpm","package":"php8.4-fpm","move_tools_to":"8.5"}`)
	var failure phpToolBindFailure
	if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || failure.Code != errCodePHPToolMoveFailed || failure.Tool != "webmail" || !failure.Restored ||
		!slices.Equal(failure.Moved, []string{"phppgadmin"}) || len(failure.NotAttempted) != 0 {
		t.Fatalf("failure: %d %+v", recorder.Code, failure)
	}
	if !slices.Equal(*calls, []string{"phppgadmin->8.5", "webmail->8.5"}) {
		t.Fatalf("calls = %v", *calls)
	}
}

// The package manager is asked before any tool moves; a removal the Agent
// then refuses still names the tools that moved (alpha.104 review).
func TestVersionRemovalWithAMoveAsksThePackageManagerFirstAndKeepsTheMovesOnARefusal(t *testing.T) {
	panel, agent, _, userID := newServiceStateAtomicPanel(t)
	uninstall := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		panel.handleServiceUninstall(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/service/uninstall",
			`{"service_id":"php-fpm","package":"php8.4-fpm","move_tools_to":"8.5"}`, userID))
		return recorder
	}
	stubPHPToolBindings(t, frankfurtBindings(), nil)
	calls := stubPHPToolBind(t, func(tool, version string) (transport.BindPHPToolResponse, error) {
		return transport.BindPHPToolResponse{Bound: true, Tool: tool, Version: version, Previous: "8.4", MutationApplied: true}, nil
	})
	type answer struct {
		Error   string   `json:"error"`
		Code    string   `json:"code"`
		Details []string `json:"details"`
		Moved   []string `json:"moved"`
		MovedTo string   `json:"moved_to"`
	}
	decode := func(recorder *httptest.ResponseRecorder) answer {
		t.Helper()
		var got answer
		if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
			t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
		}
		return got
	}

	// The package manager cannot answer (busy, unknown, no answer): nothing moves.
	for _, preview := range []func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error){
		func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
			return transport.ServiceRemovalPreviewResponse{State: "busy"}, nil
		},
		func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
			return transport.ServiceRemovalPreviewResponse{State: "unknown"}, nil
		},
		func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
			return transport.ServiceRemovalPreviewResponse{}, errors.New("agent did not answer")
		},
	} {
		stubRemovalPreview(t, preview)
		recorder := uninstall()
		if got := decode(recorder); recorder.Code != http.StatusConflict || (got.Code != errCodeRemovalCheckUnavailable && got.Code != errCodeRemovalCheckFailed) || len(got.Moved) != 0 {
			t.Fatalf("%d %+v", recorder.Code, got)
		}
	}
	// Not installed and not checkable here: the truthful reason, not "try again".
	for state, code := range map[string]string{"not_installed": errCodeRemovalVersionNotInstalled, "unsupported": errCodeRemovalCheckUnsupported, "busy": errCodeRemovalCheckUnavailable} {
		stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
			return transport.ServiceRemovalPreviewResponse{State: state}, nil
		})
		recorder := uninstall()
		got := decode(recorder)
		if got.Code != code || len(got.Moved) != 0 || (state != "busy" && strings.Contains(got.Error, "in a moment")) {
			t.Fatalf("%s: %d %+v", state, recorder.Code, got)
		}
	}
	// It would take other packages: nothing moves, and they are named.
	asked := stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{State: "checked", Collateral: []transport.PackageRemovalCollateral{{Package: "phpmyadmin", Reason: "component", Component: "phpmyadmin"}}}, nil
	})
	recorder := uninstall()
	if got := decode(recorder); recorder.Code != http.StatusConflict || got.Code != errCodeRemovalWouldRemoveOthers || !slices.Equal(got.Details, []string{"phpmyadmin"}) {
		t.Fatalf("%d %+v", recorder.Code, got)
	}
	if len(*calls) != 0 || len(*asked) != 1 || (*asked)[0].Package != "php8.4-fpm" || (*asked)[0].ID != "php-fpm" {
		t.Fatalf("calls=%v asked=%+v", *calls, *asked)
	}
	if agent.uninstallCalls.Load() != 0 {
		t.Fatal("the Agent was asked to remove after a refused check")
	}

	// Moves ok, then the Agent refuses: the answer carries the moves.
	stubRemovalPreview(t, func(transport.ServiceRemovalPreviewRequest) (transport.ServiceRemovalPreviewResponse, error) {
		return transport.ServiceRemovalPreviewResponse{State: "checked", Requested: []string{"php8.4-fpm"}, Remove: []string{"php8.4-fpm"}}, nil
	})
	agent.uninstallReplyError = "the package manager's answer changed between the check and the removal; nothing was removed"
	recorder = uninstall()
	got := decode(recorder)
	if recorder.Code != http.StatusConflict || got.Code == "" || got.MovedTo != "8.5" || !slices.Equal(got.Moved, []string{"phppgadmin", "webmail"}) || !strings.Contains(got.Error, "nothing was removed") {
		t.Fatalf("%d %+v", recorder.Code, got)
	}
	if agent.uninstallCalls.Load() != 1 || !slices.Equal(*calls, []string{"phppgadmin->8.5", "webmail->8.5"}) {
		t.Fatalf("uninstall calls=%d binds=%v", agent.uninstallCalls.Load(), *calls)
	}
}
