package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2b: the Panel's side of a site's scheduled tasks as timers
// (site_tasks.go).

// TestScheduledTasksCarryTheSite: every Scheduled tasks request names the
// site and carries its slice's limits; a domain without a site keeps 0
// (cron).
func TestScheduledTasksCarryTheSite(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/php', 'php')`)
	var siteID int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID); err != nil {
		t.Fatal(err)
	}
	tenant := f.panel.cronTenantFor(context.Background(), 9802, 9902, "customer.test")
	if tenant.SiteID != siteID || tenant.Site == nil || tenant.Site.SiteID != siteID || tenant.Site.MemoryMB != 3072 {
		t.Fatalf("tenant %+v site %+v", tenant, tenant.Site)
	}
	if none := f.panel.cronTenantFor(context.Background(), 9802, 9999, "nosite.test"); none.SiteID != 0 || none.Site != nil {
		t.Fatalf("a domain without a site: %+v", none)
	}
}

// TestSiteTasksMoveIsPartOfTheMoveAndUndoIsTheAdministrators: a site without
// programs moves its tasks with the same action; the preview and the undo
// are the administrator's alone; both are audited; the "outside" record
// follows.
func TestSiteTasksMoveIsPartOfTheMoveAndUndoIsTheAdministrators(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/php', 'php')`)
	var siteID int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID)
	oldMove, oldSlice := moveSiteTasksCall, moveSiteSliceCall
	t.Cleanup(func() { moveSiteTasksCall, moveSiteSliceCall = oldMove, oldSlice })
	var calls []string
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		calls = append(calls, method+map[bool]string{true: " preview", false: ""}[req.Preview])
		if req.SiteID != siteID || req.Site == nil {
			t.Errorf("request %+v", req)
		}
		switch {
		case req.Preview:
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksPreview, Rows: []transport.SiteTaskMoveRow{{Line: 1, Text: "0 3 * * * php x.php", Unit: "celikcron-1-1"}}}, nil
		case method == "Agent.UndoSiteTasksMoveV1":
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUndone}, nil
		}
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}, nil
	}
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		t.Error("a site without programs asked the programs' move")
		return transport.MoveSiteSliceResponse{}, nil
	}
	handler := siteLimitsMux(f)
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/site-limits/preview", ""), http.StatusForbidden, "")
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/site-limits/tasks-undo", `{"request_id":"undo-0001"}`), http.StatusForbidden, "")
	if len(calls) != 0 {
		t.Fatalf("a customer reached the Agent: %v", calls)
	}
	// The "Move all" list is the administrator's alone.
	outside := http.NewServeMux()
	outside.HandleFunc("/api/v1/site-limits/outside", f.panel.handleSiteLimitsOutside)
	outsideHandler := f.panel.requireAuth(outside)
	wantStatus(t, siteLimitsCall(t, f, outsideHandler, authzMatrixCustomerID, http.MethodGet, "/api/v1/site-limits/outside", ""), http.StatusForbidden, "")
	wantStatus(t, siteLimitsCall(t, f, outsideHandler, authzMatrixAdminID, http.MethodGet, "/api/v1/site-limits/outside", ""), http.StatusOK, "")
	preview := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits/preview", "")
	wantStatus(t, preview, http.StatusOK, "")
	if !strings.Contains(preview.Body.String(), `"celikcron-1-1"`) {
		t.Fatalf("preview %s", preview.Body.String())
	}
	f.panel.noteSiteTasksOutside(context.Background(), map[int]bool{siteID: true}, nil, time.Now())
	move := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"move-0002"}`)
	wantStatus(t, move, http.StatusOK, "")
	var resp transport.MoveSiteSliceResponse
	if err := json.Unmarshal(move.Body.Bytes(), &resp); err != nil || resp.Tasks == nil || resp.Tasks.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %s", move.Body.String())
	}
	var state siteLimitOutside
	_ = json.Unmarshal([]byte(f.panel.setting(context.Background(), settingSiteLimitOutside)), &state)
	if state.Tasks[itoa(siteID)] {
		t.Fatalf("the moved site is still outside: %+v", state)
	}
	undo := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/tasks-undo", `{"request_id":"undo-0002"}`)
	wantStatus(t, undo, http.StatusOK, "")
	_ = json.Unmarshal([]byte(f.panel.setting(context.Background(), settingSiteLimitOutside)), &state)
	if !state.Tasks[itoa(siteID)] {
		t.Fatalf("the site put back into cron is not outside: %+v", state)
	}
	var audited int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action IN ('site.limits_move', 'site.tasks_undo')`).Scan(&audited)
	if audited != 2 {
		t.Fatalf("%d audit rows", audited)
	}
	if strings.Join(calls, ",") != "Agent.MoveSiteTasksV1 preview,Agent.MoveSiteTasksV1,Agent.UndoSiteTasksMoveV1" {
		t.Fatalf("calls %v", calls)
	}
}

// TestAnUnresolvedTasksMoveStartsNoProgramsMove (S4 R2b second fix round,
// review N1; alpha.150: main's one kept record): a tasks part whose result
// is unknown, or that was refused before it began (a kept record, the
// panel's update, another move), does not start the programs part of the
// same press; the answer carries main's code. A kept tasks record is main's
// ONE site_move_kept line (from the status read), and Try again goes to the
// Agent's one Try again (Recover), which settles a tasks record too.
func TestAnUnresolvedTasksMoveStartsNoProgramsMove(t *testing.T) {
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/node', 'node')`)
	var siteID int
	_ = f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9902`).Scan(&siteID)
	oldMove, oldSlice := moveSiteTasksCall, moveSiteSliceCall
	t.Cleanup(func() { moveSiteTasksCall, moveSiteSliceCall = oldMove, oldSlice })
	kept := &transport.SiteSliceMoveRecovery{At: "2026-11-23T10:00:00Z", Units: map[string]string{}, Kept: true, Tasks: transport.SiteTasksUnknown,
		TasksStep: "crontab_emptied", TasksSite: siteID, TasksAccount: "customer_test"}
	answer := transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown, Error: "putting them back did not complete", Recovery: kept}
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		return answer, nil
	}
	programs, recovers := 0, 0
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		if req.Recover {
			recovers++
			return transport.MoveSiteSliceResponse{Done: true, Recovery: &transport.SiteSliceMoveRecovery{Tasks: transport.SiteTasksPutBack}}, nil
		}
		programs++
		return transport.MoveSiteSliceResponse{Done: true}, nil
	}
	handler := siteLimitsMux(f)
	for i, tc := range []struct {
		tasks    transport.SiteTasksMoveResponse
		code     string
		wantKept bool
	}{
		{answer, "not_finished", true},
		{transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: transport.SiteMoveRefusalKept, Error: "an earlier move of site 7 could not be put back completely"}, transport.SiteMoveRefusalKept, true},
		{transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: "update_running", Error: "the panel's update is in progress"}, "update_running", false},
		{transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: "another_move", Error: "another move into a site's limits is running"}, "another_move", false},
	} {
		answer = tc.tasks
		move := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"move-001`+itoa(i)+`"}`)
		wantStatus(t, move, http.StatusOK, "")
		var resp transport.MoveSiteSliceResponse
		_ = json.Unmarshal(move.Body.Bytes(), &resp)
		if resp.Done || resp.Error == "" || resp.Tasks == nil || resp.Reason == nil || resp.Reason.Code != tc.code || resp.Kept != tc.wantKept {
			t.Fatalf("answer %d %s", i, move.Body.String())
		}
	}
	if programs != 0 {
		t.Fatalf("the programs part ran %d times after a tasks part that did not begin or whose result is unknown", programs)
	}
	// Try again: the one Recover call; no tasks call.
	answer = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}
	again := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"move-0019","recover":true}`)
	wantStatus(t, again, http.StatusOK, "")
	if recovers != 1 || programs != 0 {
		t.Fatalf("Try again: recovers %d programs %d", recovers, programs)
	}
	// The kept tasks record is main's one line, opening that site.
	f.panel.noteSiteMoveKept(context.Background(), []transport.SiteSliceStatus{{SiteID: siteID, MoveInterrupted: kept}}, true)
	lines := siteMoveKeptProblem(f.panel.setting(context.Background(), settingSiteMoveKept), f.panel.setting(context.Background(), settingSiteMoveStopped),
		map[string]string{itoa(siteID): "customer.test"})
	if len(lines) != 1 || lines[0].Kind != "site_move_kept" || lines[0].Subject != "customer.test" || lines[0].Actor != "administrator" {
		t.Fatalf("problem %+v", lines)
	}
	if _, ordered := problemKindOrder["site_move_unresolved"]; ordered {
		t.Error("site_move_unresolved is gone: one record, one line (site_move_kept)")
	}
	for _, kind := range []string{"site_move_kept", "site_user_manager_open"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Errorf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, subject: "shop.example.com", name: "x", value: 2}); strings.HasPrefix(en, "A monitored problem") || tr == "" {
			t.Errorf("%s has no alert line", kind)
		}
	}
	// Problems-now kinds keep unique order numbers.
	seen := map[int]string{}
	for kind, n := range problemKindOrder {
		if other, dup := seen[n]; dup {
			t.Errorf("%s and %s share order %d", kind, other, n)
		}
		seen[n] = kind
	}
}

// TestUserManagerProblem (review N11): one line names the site accounts whose
// own service manager can still start (the owner's file, the owner's removal,
// not written) with the units to mask; none while every one is closed.
func TestUserManagerProblem(t *testing.T) {
	state := transport.SideDoorsState{CheckedAt: "2026-11-23T10:00:00Z", UserManagers: &transport.SideDoorsUserManagers{Masked: 3,
		Owner: []string{"b_test"}, Removed: []string{"a_test"}, OpenUnits: []string{"user@1502.service", "user@1501.service"}}}
	lines := siteUserManagerProblems(state)
	if len(lines) != 1 || lines[0].Subject != "a_test, b_test" || lines[0].Name != "user@1501.service user@1502.service" || lines[0].Value != 2 {
		t.Fatalf("%+v", lines)
	}
	state.UserManagers = &transport.SideDoorsUserManagers{Masked: 3, OwnerMasked: 1}
	if lines := siteUserManagerProblems(state); len(lines) != 0 {
		t.Fatalf("a line while every manager is closed: %+v", lines)
	}
}

// TestTheOutsideLineCountsProgramsAndTasks: ONE line for the sites whose
// programs or tasks run outside their limits, opening "Move all"; a reading
// of the programs keeps the tasks part.
func TestTheOutsideLineCountsProgramsAndTasks(t *testing.T) {
	names := map[string]string{"1": "a.test", "2": "b.test", "3": "c.test"}
	raw, _ := json.Marshal(siteLimitOutside{CheckedAt: "2026-11-22T10:00:00Z", Sites: map[string]int{"1": 2}, Tasks: map[string]bool{"1": true, "2": true, "9": true}})
	lines := siteLimitsOutsideProblem(string(raw), names)
	if len(lines) != 1 || lines[0].Value != 2 || lines[0].Subject != "a.test, b.test" || lines[0].Action.Kind != "move_all" || lines[0].Kind != "site_limits_outside" {
		t.Fatalf("%+v", lines)
	}
	f := newRightsFixture(t)
	f.panel.noteSiteTasksOutside(context.Background(), map[int]bool{5: true}, nil, time.Now())
	f.panel.noteSiteLimitOutside(context.Background(), nil, true, time.Now())
	var state siteLimitOutside
	_ = json.Unmarshal([]byte(f.panel.setting(context.Background(), settingSiteLimitOutside)), &state)
	if !state.Tasks["5"] {
		t.Fatalf("a programs reading dropped the tasks part: %+v", state)
	}
	for _, kind := range []string{"site_limits_outside", "cron_allow_lists_sites", "cron_not_closed", "cron_owner_removed", "site_crontab_returned", "site_lingering", "linger_owner_removed", "linger_not_closed"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Errorf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, subject: "shop.example.com", name: "/etc/cron.allow", value: 2}); strings.HasPrefix(en, "A monitored problem") || tr == "" {
			t.Errorf("%s has no alert line", kind)
		}
	}
}

// TestFixRoundProblemsAndRefusals (S4 R2b fix round): a crontab the site's
// own code can have made is its own line (never called the owner's); held
// tasks are one administrator line naming the sites; the Agent's new exact
// refusals of a task change become coded answers with words; the task file
// rebuild is the administrator's alone and audited.
func TestFixRoundProblemsAndRefusals(t *testing.T) {
	state := transport.SideDoorsState{CheckedAt: "2026-11-22T10:00:00Z",
		Cron: &transport.SideDoorsCron{State: transport.SideDoorsAtCurrent, Reappeared: map[string]int{"a_test": 1, "b_test": 3},
			ReappearedBySite: map[string]bool{"b_test": true}},
		TasksHeld: []transport.SideDoorsTasksHeld{{SiteID: 4, Domain: "shop.test", Detail: "the slice file could not be written"}}}
	kinds := map[string]monitoringProblem{}
	for _, p := range append(siteCronProblems(state), siteTasksHeldProblems(state)...) {
		kinds[p.Kind] = p
	}
	if p := kinds["site_crontab_returned"]; p.Subject != "a_test" || p.Value != 1 {
		t.Fatalf("owner's: %+v", p)
	}
	if p := kinds["site_crontab_by_site"]; p.Subject != "b_test" || p.Value != 3 {
		t.Fatalf("by site: %+v", p)
	}
	if p := kinds["site_tasks_held"]; p.Subject != "shop.test" || p.Actor != "administrator" || p.Name == "" {
		t.Fatalf("held: %+v", p)
	}
	for _, kind := range []string{"site_crontab_by_site", "site_tasks_held"} {
		if _, ordered := problemKindOrder[kind]; !ordered {
			t.Errorf("%s has no place in the problem order", kind)
		}
		if en, tr := alertLine(pendingAlertMail{kind: kind, subject: "shop.example.com", name: "x", value: 2}); strings.HasPrefix(en, "A monitored problem") || tr == "" {
			t.Errorf("%s has no alert line", kind)
		}
	}
	for code, err := range map[string]error{
		errCodeSiteTasksSourceMissing: rpc.ServerError(transport.SiteTasksSourceGone),
		errCodeCronCommandTooLong:     rpc.ServerError(transport.CronCommandTooLong),
		errCodeSiteTasksEnvValue:      rpc.ServerError(transport.SiteTasksEnvValue),
		errCodeSiteTasksLimitsMissing: rpc.ServerError(transport.SiteTasksLimitsMissing + ": /etc/systemd/system/celiksite_4.slice is not there"),
	} {
		recorder := httptest.NewRecorder()
		writeCronAgentError(recorder, err, cronReasonWrite)
		var body apiErrorBody
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		if body.Code != code || recorder.Code < 400 || body.Error == "" {
			t.Errorf("%s: %d %+v", code, recorder.Code, body)
		}
	}

	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9902, '/srv/php', 'php')`)
	old := rebuildSiteTaskSourceCall
	t.Cleanup(func() { rebuildSiteTaskSourceCall = old })
	var calls []bool
	rebuildSiteTaskSourceCall = func(p *Panel, ctx context.Context, req *transport.SiteTasksMoveRequest) (transport.SiteTaskSourceResponse, error) {
		calls = append(calls, req.Preview)
		if req.Preview {
			return transport.SiteTaskSourceResponse{Outcome: transport.SiteTasksPreview, Crontab: "0 3 * * * php x.php\n"}, nil
		}
		return transport.SiteTaskSourceResponse{Outcome: transport.SiteTaskSourceRebuilt}, nil
	}
	handler := siteLimitsMux(f)
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodGet, "/api/v1/domains/9902/site-limits/tasks-source", ""), http.StatusForbidden, "")
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixCustomerID, http.MethodPost, "/api/v1/domains/9902/site-limits/tasks-source", `{"request_id":"move-0003"}`), http.StatusForbidden, "")
	if len(calls) != 0 {
		t.Fatalf("a customer reached the Agent: %v", calls)
	}
	preview := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits/tasks-source", "")
	wantStatus(t, preview, http.StatusOK, "")
	if !strings.Contains(preview.Body.String(), `php x.php`) {
		t.Fatalf("preview %s", preview.Body.String())
	}
	wantStatus(t, siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/tasks-source", `{"request_id":"move-0004"}`), http.StatusOK, "")
	var audited int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'site.tasks_source_rebuild'`).Scan(&audited)
	if audited != 1 || len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatalf("audited %d calls %v", audited, calls)
	}
}

// TestCronAndLingerProblems: one line per kind, administrator lines only
// where the administrator acts (the explicit re-close), the server owner's
// otherwise; the lingering line carries the command that ends it.
func TestCronAndLingerProblems(t *testing.T) {
	state := transport.SideDoorsState{CheckedAt: "2026-11-22T10:00:00Z",
		Cron: &transport.SideDoorsCron{State: transport.SideDoorsAtOwnerFile, Path: "/etc/cron.allow", SiteAccounts: []string{"shop_test"},
			Reappeared: map[string]int{"shop_test": 2}},
		Linger: &transport.SideDoorsLinger{State: transport.SideDoorsAtOwnerRemoved, Path: "/etc/polkit-1/rules.d/49-celikpanel-linger.rules", Accounts: []string{"a_test", "b_test"}}}
	kinds := map[string]monitoringProblem{}
	for _, p := range append(siteCronProblems(state), siteLingerProblems(state)...) {
		kinds[p.Kind] = p
	}
	if p := kinds["cron_allow_lists_sites"]; p.Subject != "shop_test" || p.Actor != "server_owner" {
		t.Fatalf("%+v", p)
	}
	if p := kinds["site_crontab_returned"]; p.Value != 2 {
		t.Fatalf("%+v", p)
	}
	if p := kinds["site_lingering"]; p.Name != "loginctl disable-linger a_test b_test" || p.Value != 2 {
		t.Fatalf("%+v", p)
	}
	if p := kinds["linger_owner_removed"]; p.Action.Kind != "reclose" || p.Action.To != transport.SideDoorsReCloseLinger || p.Actor != "administrator" {
		t.Fatalf("%+v", p)
	}
	state.Cron = &transport.SideDoorsCron{State: transport.SideDoorsAtOwnerRemoved, Path: "/etc/cron.allow"}
	if lines := siteCronProblems(state); len(lines) != 1 || lines[0].Action.To != transport.SideDoorsReCloseCron {
		t.Fatalf("%+v", lines)
	}
	state.Cron = &transport.SideDoorsCron{State: transport.SideDoorsAtCurrent}
	state.Linger = &transport.SideDoorsLinger{State: transport.SideDoorsAtCurrent}
	if lines := append(siteCronProblems(state), siteLingerProblems(state)...); len(lines) != 0 {
		t.Fatalf("lines while closed: %+v", lines)
	}
	for _, target := range []string{transport.SideDoorsReCloseCron, transport.SideDoorsReCloseLinger} {
		closed := sideDoorClosed(&transport.SideDoorsState{Cron: &transport.SideDoorsCron{State: transport.SideDoorsAtWritten},
			Linger: &transport.SideDoorsLinger{State: transport.SideDoorsAtWritten}}, target, "")
		if !closed {
			t.Errorf("%s is not closed after a write", target)
		}
	}
}
