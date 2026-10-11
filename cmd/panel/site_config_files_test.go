package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/rpc"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// D-031 second step (2026-10-11) at the Panel: the site's PHP-FPM pool and
// application unit on the Configuration file page, their typed refusals, the
// owner's choices per file, and the Domains row's precedence across files.

type otherSiteFileAgent struct {
	siteFileTestAgent

	poolInspect  func(*transport.SitePoolRequest, *transport.InspectSiteFileResponse)
	poolApply    func(*transport.SitePoolRequest, *transport.ApplySiteFileResponse)
	unitInspect  func(*transport.AppApplyRequest, *transport.InspectSiteFileResponse)
	unitApply    func(*transport.AppApplyRequest, *transport.AppApplyResponse)
	poolRequests []transport.SitePoolRequest
	unitRequests []transport.AppApplyRequest
	vhostApplied int
	unitsRemoved int
}

func (a *otherSiteFileAgent) InspectSitePool(request *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.poolRequests = append(a.poolRequests, *request)
	a.poolInspect(request, response)
	return nil
}

func (a *otherSiteFileAgent) ApplySitePool(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.poolRequests = append(a.poolRequests, *request)
	a.poolApply(request, response)
	return nil
}

func (a *otherSiteFileAgent) InspectAppUnit(request *transport.AppApplyRequest, response *transport.InspectSiteFileResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unitRequests = append(a.unitRequests, *request)
	a.unitInspect(request, response)
	return nil
}

func (a *otherSiteFileAgent) ApplyAppUnit(request *transport.AppApplyRequest, response *transport.AppApplyResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unitRequests = append(a.unitRequests, *request)
	a.unitApply(request, response)
	return nil
}

func (a *otherSiteFileAgent) RemoveAppUnit(_ *transport.AppControlRequest, _ *transport.AppApplyResponse) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unitsRemoved++
	return nil
}

const (
	otherFileSHA   = "1111111111111111111111111111111111111111111111111111111111111111"
	otherRenderSHA = "2222222222222222222222222222222222222222222222222222222222222222"
)

func otherFile(kind, path, state, outcome string) transport.SiteFileResult {
	return transport.SiteFileResult{Kind: kind, Path: path, State: state, Outcome: outcome,
		FileSHA256: otherFileSHA, RenderSHA256: otherRenderSHA}
}

type otherSiteFileFixture struct {
	panel    *Panel
	agent    *otherSiteFileAgent
	domainID int
	admin    *Caller
	base     string
}

func newOtherSiteFileFixture(t *testing.T, domain, projectType string) *otherSiteFileFixture {
	t.Helper()
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, domain, projectType)
	if _, err := panel.db.GetDB().Exec(`UPDATE sites SET php_version = '8.3', app_port = 3101,
		start_command = 'node app.js', runtime_version = '22.11.0' WHERE domain_id = ?`, domainID); err != nil {
		t.Fatal(err)
	}
	agent := &otherSiteFileAgent{}
	agent.inspect = func(request *transport.ApplyVhostRequest, response *transport.InspectSiteFileResponse) {
		response.File = fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeInspected)
		response.File.WrittenSHA256 = response.File.RenderSHA256
	}
	agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
		agent.vhostApplied++
		file := fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeUnchanged)
		response.File = &file
	}
	attachStartupVhostBatchAgent(t, panel, agent)
	var adminID int
	if err := panel.db.GetDB().QueryRow(`SELECT id FROM users WHERE username = 'startup-vhost-owner'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	return &otherSiteFileFixture{panel: panel, agent: agent, domainID: domainID, admin: &Caller{ID: adminID, Role: roleAdmin},
		base: "/api/v1/domains/" + itoaForTest(domainID) + "/site-config"}
}

func (f *otherSiteFileFixture) read(t *testing.T) siteConfigView {
	t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleDomainSiteConfig(recorder, siteConfigRequest(http.MethodGet, f.base, nil, f.admin), f.domainID, "")
	var view siteConfigView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || recorder.Code != http.StatusOK {
		t.Fatalf("read: %d %s", recorder.Code, recorder.Body.String())
	}
	return view
}

func (f *otherSiteFileFixture) post(action string, body siteConfigActionRequest) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	f.panel.handleDomainSiteConfig(recorder, siteConfigRequest(http.MethodPost, f.base+"/"+action, body, f.admin), f.domainID, action)
	return recorder
}

func (f *otherSiteFileFixture) row(t *testing.T, kind string) services.SiteFileRecord {
	t.Helper()
	siteID, _, err := f.panel.siteIDForDomain(context.Background(), f.domainID)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := services.LoadSiteFileRecord(context.Background(), f.panel.db.GetDB(), siteID, kind)
	if err != nil || !found {
		t.Fatalf("%s row: found=%v err=%v", kind, found, err)
	}
	return record
}

func fileByKind(view siteConfigView, kind string) *siteConfigView {
	for index := range view.Files {
		if view.Files[index].Kind == kind {
			return &view.Files[index]
		}
	}
	return nil
}

const poolTestPath = "/etc/php/8.3/fpm/pool.d/site1.conf"

func TestSiteConfigListsThePoolAndItsChoices(t *testing.T) {
	f := newOtherSiteFileFixture(t, "pool.example", "php")
	f.agent.poolInspect = func(request *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeInspected)
		response.File.PendingPath = poolTestPath + ".celikpanel-pending"
		response.Diff = "--- a\n+++ b\n@@ -1 +1 @@\n-php_admin_value[memory_limit] = 512M\n"
	}
	f.agent.poolApply = func(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
		if request.Operation != transport.SitePoolOperationTake || request.ExpectedFileSHA256 != otherFileSHA ||
			request.ExpectedRenderSHA256 != otherRenderSHA || request.Username != services.SiteUsername("pool.example") {
			file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeRefused)
			file.Reason = transport.SiteFileReasonChanged
			response.File = &file
			return
		}
		file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeTaken)
		file.BackupPath = poolTestPath + ".celikpanel-backup-20261011T010000Z"
		file.WrittenSHA256 = otherRenderSHA
		response.File = &file
	}

	view := f.read(t)
	if len(view.Files) != 2 || view.Files[0].Kind != transport.SiteFileKindNginxVhost || view.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("files: %+v", view.Files)
	}
	pool := fileByKind(view, transport.SiteFileKindPHPPool)
	if pool == nil || pool.State != transport.SiteFileOwnerEdited || strings.Join(pool.Actions, ",") != "keep,take,merge" ||
		pool.Diff == "" || pool.PendingPath == "" {
		t.Fatalf("pool view: %+v", pool)
	}
	if view.OverallState != transport.SiteFileOwnerEdited || view.OverallKind != transport.SiteFileKindPHPPool {
		t.Fatalf("overall = %q %q", view.OverallState, view.OverallKind)
	}
	if request := f.agent.poolRequests[0]; request.Operation != transport.SitePoolOperationInspect || request.PHPVersion != "8.3" ||
		request.ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
		t.Fatalf("pool inspect request: %+v", request)
	}
	// The page's read is recorded for the pool, so the Domains row agrees.
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileOwnerEdited || row.FileSHA256 != otherFileSHA {
		t.Fatalf("pool row: %+v", row)
	}
	summaries, err := f.panel.domainSiteConfigSummaries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary := summaries[f.domainID]; summary.State != transport.SiteFileOwnerEdited || summary.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("list summary: %+v", summary)
	}

	// Keep mine, on the pool.
	recorder := f.post("keep", siteConfigActionRequest{Kind: transport.SiteFileKindPHPPool, FileSHA256: strings.Repeat("9", 64)})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeSiteConfigChanged) {
		t.Fatalf("stale keep: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = f.post("keep", siteConfigActionRequest{Kind: transport.SiteFileKindPHPPool, FileSHA256: otherFileSHA})
	if recorder.Code != http.StatusOK {
		t.Fatalf("keep: %d %s", recorder.Code, recorder.Body.String())
	}
	if row := f.row(t, transport.SiteFileKindPHPPool); row.Decision != services.SiteFileDecisionKeepMine || row.DecisionFileSHA256 != otherFileSHA {
		t.Fatalf("keep row: %+v", row)
	}
	siteID, _, _ := f.panel.siteIDForDomain(context.Background(), f.domainID)
	if row, found, _ := services.LoadSiteFileRecord(context.Background(), f.panel.db.GetDB(), siteID, transport.SiteFileKindNginxVhost); found && row.Decision != "" {
		t.Fatalf("the vhost row got the pool's decision: %+v", row)
	}
	summaries, _ = f.panel.domainSiteConfigSummaries(context.Background())
	if summary := summaries[f.domainID]; !summary.KeptByChoice {
		t.Fatalf("summary after keep: %+v", summary)
	}

	// Take CelikPanel's, on the pool.
	recorder = f.post("take", siteConfigActionRequest{Kind: transport.SiteFileKindPHPPool, FileSHA256: otherFileSHA, RenderSHA256: otherRenderSHA})
	var taken siteConfigView
	if err := json.Unmarshal(recorder.Body.Bytes(), &taken); err != nil || recorder.Code != http.StatusOK ||
		taken.Kind != transport.SiteFileKindPHPPool || !strings.Contains(taken.BackupPath, ".celikpanel-backup-") {
		t.Fatalf("take: %d %s", recorder.Code, recorder.Body.String())
	}
	if row := f.row(t, transport.SiteFileKindPHPPool); row.Decision != services.SiteFileDecisionTake || row.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("take row: %+v", row)
	}

	// An unknown kind is refused before anything is asked.
	recorder = f.post("keep", siteConfigActionRequest{Kind: "crontab", FileSHA256: otherFileSHA})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind: %d", recorder.Code)
	}
	// The unit is not a file of a PHP site.
	recorder = f.post("keep", siteConfigActionRequest{Kind: transport.SiteFileKindAppUnit, FileSHA256: otherFileSHA})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeSiteConfigNotApplicable) {
		t.Fatalf("unit on a PHP site: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPoolSaveAndAPHPSwitchOnAKeptPoolAreTypedRefusals(t *testing.T) {
	f := newOtherSiteFileFixture(t, "save.example", "php")
	f.agent.poolApply = func(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
		file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeKept)
		file.PendingPath = poolTestPath + ".celikpanel-pending"
		response.File = &file
	}
	site, err := f.panel.loadSiteFileSite(context.Background(), f.domainID)
	if err != nil {
		t.Fatal(err)
	}
	request := f.panel.sitePoolRequest(context.Background(), site, transport.SitePoolOperationSave)
	request.Values = &transport.SitePoolValues{PM: "static", PMMaxChildren: 4}
	_, applyErr := f.panel.applySitePool(context.Background(), site, request)
	recorder := httptest.NewRecorder()
	writeServerError(recorder, applyErr)
	var body apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	if recorder.Code != http.StatusConflict || body.Code != errCodeSiteConfigOwnerEdited || body.Reason != transport.SiteFileKindPHPPool ||
		body.Detail != transport.SiteFileOwnerEdited || !strings.Contains(body.Error, "PHP-FPM pool") {
		t.Fatalf("save refusal: %d %+v", recorder.Code, body)
	}
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileOwnerEdited || row.PendingPath == "" {
		t.Fatalf("save row: %+v", row)
	}

	// The switch: the same refusal; the target version is the request's.
	request = f.panel.sitePoolRequest(context.Background(), site, transport.SitePoolOperationSwitch)
	request.NewPHPVersion = "8.4"
	_, applyErr = f.panel.applySitePool(context.Background(), site, request)
	classification, ok := classifyStableAgentError(applyErr)
	if !ok || classification.Code != errCodeSiteConfigOwnerEdited || classification.Reason != transport.SiteFileKindPHPPool {
		t.Fatalf("switch refusal: %+v %v", classification, ok)
	}
	if last := f.agent.poolRequests[len(f.agent.poolRequests)-1]; last.Operation != transport.SitePoolOperationSwitch || last.NewPHPVersion != "8.4" {
		t.Fatalf("switch request: %+v", last)
	}

	// The older pool RPCs answer only an error's text; its tokens are read.
	held := &services.SiteFileHeldError{File: otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeKept)}
	classification, ok = classifyStableAgentError(rpc.ServerError(held.Error()))
	if !ok || classification.Code != errCodeSiteConfigOwnerEdited || classification.Reason != transport.SiteFileKindPHPPool {
		t.Fatalf("token refusal: %+v %v", classification, ok)
	}
	// The pool's other refusals.
	for _, c := range []struct {
		file transport.SiteFileResult
		code string
	}{
		{otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileAbsent, transport.SiteFileOutcomeMissing), errCodeSiteConfigMissing},
		{func() transport.SiteFileResult {
			file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeRefused)
			file.Reason = transport.SiteFileReasonNotApplicable
			return file
		}(), errCodeSiteConfigExists},
		{func() transport.SiteFileResult {
			file := otherFile(transport.SiteFileKindAppUnit, "/etc/systemd/system/celikapp-1.service", transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeFailed)
			file.Reason = transport.SiteFileReasonReloadFailed
			return file
		}(), errCodeSiteConfigServiceRefused},
		{func() transport.SiteFileResult {
			file := otherFile(transport.SiteFileKindAppUnit, "/etc/systemd/system/celikapp-1.service", transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeRefused)
			file.Reason = transport.SiteFileReasonWriteRefused
			return file
		}(), errCodeSiteConfigUnwritable},
	} {
		classification := classifyOtherSiteFileHeld(c.file, "")
		if classification.Code != c.code || classification.Reason != c.file.Kind || classification.Message == "" {
			t.Errorf("%s/%s: %+v", c.file.Outcome, c.file.Reason, classification)
		}
	}
}

const unitTestPath = "/etc/systemd/system/celikapp-1.service"

func TestSiteConfigListsTheUnitAndAHostingChangeOnAKeptUnitIsRefused(t *testing.T) {
	f := newOtherSiteFileFixture(t, "app.example", "node")
	f.agent.unitInspect = func(request *transport.AppApplyRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeInspected)
		response.File.AdoptedFrom = "v0.1.0-alpha.82"
		response.File.IncludeDir = unitTestPath + ".d"
	}
	f.agent.unitApply = func(request *transport.AppApplyRequest, response *transport.AppApplyResponse) {
		file := otherFile(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeKept)
		response.File = &file
		response.Error = "kept"
	}
	view := f.read(t)
	unit := fileByKind(view, transport.SiteFileKindAppUnit)
	if unit == nil || unit.State != transport.SiteFileManagedUnchanged || unit.AdoptedFrom != "v0.1.0-alpha.82" || unit.IncludeDir == "" {
		t.Fatalf("unit view: %+v", view.Files)
	}
	if request := f.agent.unitRequests[0]; request.Port != 3101 || request.Command != "node app.js" || request.RunAsUser != "www-data" {
		t.Fatalf("unit inspect request: %+v", request)
	}
	if view.OverallState != transport.SiteFileManagedUnchanged || fileByKind(view, transport.SiteFileKindPHPPool) != nil {
		t.Fatalf("node site's files: %+v", view)
	}

	// A hosting change (Node to Node) on the kept unit: the typed refusal,
	// no compensation (nothing was changed): no vhost render, no removal.
	_, err := f.panel.transitionHosting(context.Background(), f.domainID, hostingSettings{
		ProjectType: "node", StartCommand: "node server.js", RuntimeVersion: "22.11.0", AppPort: 3101,
	})
	classification, ok := classifyStableAgentError(err)
	if !ok || classification.Code != errCodeSiteConfigOwnerEdited || classification.Reason != transport.SiteFileKindAppUnit {
		t.Fatalf("hosting change: %v %+v", err, classification)
	}
	last := f.agent.unitRequests[len(f.agent.unitRequests)-1]
	if last.Trigger != transport.SiteFileTriggerChange || last.Command != "node server.js" {
		t.Fatalf("unit request: %+v", last)
	}
	if f.agent.vhostApplied != 0 || f.agent.unitsRemoved != 0 {
		t.Fatalf("compensation ran: vhost %d removed %d", f.agent.vhostApplied, f.agent.unitsRemoved)
	}
	var command string
	_ = f.panel.db.GetDB().QueryRow(`SELECT start_command FROM sites WHERE domain_id = ?`, f.domainID).Scan(&command)
	if command != "node app.js" {
		t.Fatalf("the database took the refused change: %q", command)
	}

	// Take CelikPanel's on the unit names the unit's trigger.
	f.agent.unitApply = func(request *transport.AppApplyRequest, response *transport.AppApplyResponse) {
		file := otherFile(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeTaken)
		if request.Trigger != transport.SiteFileTriggerTake || request.ExpectedFileSHA256 != otherFileSHA {
			file.Outcome, file.Reason = transport.SiteFileOutcomeRefused, transport.SiteFileReasonChanged
		}
		file.BackupPath = unitTestPath + ".celikpanel-backup-20261011T010000Z"
		response.File = &file
		response.Unit = "celikapp-1"
	}
	f.agent.unitInspect = func(request *transport.AppApplyRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeInspected)
	}
	recorder := f.post("take", siteConfigActionRequest{Kind: transport.SiteFileKindAppUnit, FileSHA256: otherFileSHA, RenderSHA256: otherRenderSHA})
	if recorder.Code != http.StatusOK {
		t.Fatalf("unit take: %d %s", recorder.Code, recorder.Body.String())
	}
	if row := f.row(t, transport.SiteFileKindAppUnit); row.Decision != services.SiteFileDecisionTake {
		t.Fatalf("unit row: %+v", row)
	}
}

func TestDomainsRowShowsTheWorstFileVhostFirstOnATie(t *testing.T) {
	f := newOtherSiteFileFixture(t, "rows.example", "php")
	siteID, _, _ := f.panel.siteIDForDomain(context.Background(), f.domainID)
	ctx := context.Background()
	record := func(kind, path, state, outcome, reason string) {
		file := transport.SiteFileResult{Kind: kind, Path: path, State: state, Outcome: outcome, Reason: reason, FileSHA256: otherFileSHA}
		if err := services.RecordSiteFileResult(ctx, f.panel.db.GetDB(), siteID, f.domainID, file, "test", 0); err != nil {
			t.Fatal(err)
		}
	}
	record(transport.SiteFileKindNginxVhost, siteFilePath("rows.example"), transport.SiteFileOwnerEdited, transport.SiteFileOutcomeKept, "")
	record(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileForeign, transport.SiteFileOutcomeKept, "")
	summaries, _ := f.panel.domainSiteConfigSummaries(ctx)
	if summary := summaries[f.domainID]; summary.State != transport.SiteFileOwnerEdited || summary.Kind != "" {
		t.Fatalf("tie: %+v", summary)
	}
	record(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeRefused, transport.SiteFileReasonWriteRefused)
	summaries, _ = f.panel.domainSiteConfigSummaries(ctx)
	if summary := summaries[f.domainID]; summary.State != transport.SiteFileUnwritable || summary.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("pool unwritable: %+v", summary)
	}
	// A unit row of a site that is no longer a Node site is not shown.
	record(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileUnreadable, transport.SiteFileOutcomeRefused, transport.SiteFileReasonSymlink)
	summaries, _ = f.panel.domainSiteConfigSummaries(ctx)
	if summary := summaries[f.domainID]; summary.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("stale unit row decided: %+v", summary)
	}
}
