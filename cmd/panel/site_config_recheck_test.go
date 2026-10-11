package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// After a refused replacement (set10 scenario g, corrected 2026-10-11): the
// domain screen's "check again" records what it found, so the page and the
// Domains row agree; CelikPanel's unchanged text whose protection was removed
// offers "apply"; the owner's file whose "take" was refused shows both facts.

type recheckFixture struct {
	t        *testing.T
	panel    *Panel
	agent    *siteFileTestAgent
	domainID int
	admin    *Caller
	base     string
	// What the Agent's read answers.
	state, fileSHA, protection, systemError string
}

func newRecheckFixture(t *testing.T, domain, state string) *recheckFixture {
	t.Helper()
	panel, subscriptionID := newStartupVhostBatchFixture(t)
	domainID := addStartupHostedDomain(t, panel, subscriptionID, domain, "static")
	var adminID int
	if err := panel.db.GetDB().QueryRow(`SELECT id FROM users WHERE username = 'startup-vhost-owner'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	f := &recheckFixture{
		t: t, panel: panel, domainID: domainID, admin: &Caller{ID: adminID, Role: roleAdmin},
		base:  "/api/v1/domains/" + itoaForTest(domainID) + "/site-config",
		state: state, fileSHA: strings.Repeat("c", 64),
		protection: transport.SiteFileProtectionImmutable, systemError: "EPERM",
	}
	f.agent = &siteFileTestAgent{}
	f.agent.inspect = func(request *transport.ApplyVhostRequest, response *transport.InspectSiteFileResponse) {
		response.File = fileResult(request.Domain, f.state, transport.SiteFileOutcomeInspected)
		response.File.FileSHA256 = f.fileSHA
		response.File.Protection, response.File.SystemError = f.protection, f.systemError
		response.Diff = "--- " + siteFilePath(request.Domain) + " (on this server)\n+++ CelikPanel's text\n@@ -1 +1 @@\n-old\n+new\n"
	}
	attachStartupVhostBatchAgent(t, panel, f.agent)
	return f
}

func (f *recheckFixture) read() siteConfigView {
	f.t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleDomainSiteConfig(recorder, siteConfigRequest(http.MethodGet, f.base, nil, f.admin), f.domainID, "")
	var view siteConfigView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || recorder.Code != http.StatusOK {
		f.t.Fatalf("read: %d %s", recorder.Code, recorder.Body.String())
	}
	return view
}

func (f *recheckFixture) post(action string, body siteConfigActionRequest) *httptest.ResponseRecorder {
	f.t.Helper()
	recorder := httptest.NewRecorder()
	f.panel.handleDomainSiteConfig(recorder, siteConfigRequest(http.MethodPost, f.base+"/"+action, body, f.admin), f.domainID, action)
	return recorder
}

func (f *recheckFixture) listState() string {
	f.t.Helper()
	summaries, err := f.panel.domainSiteConfigSummaries(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return summaries[f.domainID].State
}

// refusedRender records a settings render the server refused on CelikPanel's
// unchanged text.
func (f *recheckFixture) refusedRender() {
	f.t.Helper()
	f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
		file := fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeRefused)
		file.FileSHA256 = f.fileSHA
		file.Reason = transport.SiteFileReasonWriteRefused
		file.SystemError, file.Protection = "EPERM", transport.SiteFileProtectionImmutable
		response.File = &file
	}
	if err := f.panel.applyVhostForDomain(context.Background(), f.domainID); err == nil {
		f.t.Fatal("a refused render was reported as applied")
	}
}

func TestRemovedProtectionOffersApplyWhichWritesTheHeldText(t *testing.T) {
	f := newRecheckFixture(t, "unlocked.example", transport.SiteFileManagedUnchanged)
	f.refusedRender()

	// Still protected: unwritable stays, and only the time it looked is stored.
	before := ledgerRow(t, f.panel, f.domainID)
	if view := f.read(); view.State != transport.SiteFileUnwritable || strings.Join(view.Actions, ",") != siteConfigActionRecheck {
		t.Fatalf("still protected: %+v", view)
	}
	if row := ledgerRow(t, f.panel, f.domainID); row.State != transport.SiteFileUnreadable ||
		row.StateReason != transport.SiteFileReasonWriteRefused || row.ObservedAt < before.ObservedAt {
		t.Fatalf("still protected, ledger: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileUnwritable {
		t.Fatalf("still protected, list: %q", state)
	}
	// Apply is not offered while the refusal stands.
	if recorder := f.post("apply", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: strings.Repeat("b", 64)}); recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), errCodeSiteConfigNotApplicable) {
		t.Fatalf("apply while protected: %d %s", recorder.Code, recorder.Body.String())
	}

	// The owner removed the protection (chattr -i): the same bytes, none.
	f.protection, f.systemError = transport.SiteFileProtectionNone, ""
	view := f.read()
	if view.State != transport.SiteFileManagedUnchanged || view.Reason != services.SiteFileReasonProtectionRemoved ||
		strings.Join(view.Actions, ",") != siteConfigActionApply || view.Diff == "" {
		t.Fatalf("protection removed: %+v", view)
	}
	row := ledgerRow(t, f.panel, f.domainID)
	if row.State != transport.SiteFileManagedUnchanged || row.StateReason != services.SiteFileReasonProtectionRemoved || row.FileSHA256 != f.fileSHA {
		t.Fatalf("protection removed, ledger: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileManagedUnchanged {
		t.Fatalf("protection removed, list: %q", state)
	}

	// Apply is bound to what the page showed.
	if recorder := f.post("apply", siteConfigActionRequest{FileSHA256: strings.Repeat("e", 64), RenderSHA256: view.RenderSHA256}); recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), errCodeSiteConfigChanged) {
		t.Fatalf("stale apply: %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := f.post("apply", siteConfigActionRequest{}); recorder.Code != http.StatusBadRequest {
		t.Fatalf("apply without digests: %d", recorder.Code)
	}

	// Apply: the render of a settings change for this site only, no copy.
	var sent *transport.ApplyVhostRequest
	f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
		copied := *request
		sent = &copied
		file := fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeWritten)
		file.WrittenSHA256 = file.RenderSHA256
		file.FileSHA256 = strings.Repeat("f", 64)
		file.Reloaded = true
		response.File = &file
	}
	recorder := f.post("apply", siteConfigActionRequest{FileSHA256: view.FileSHA256, RenderSHA256: view.RenderSHA256})
	if recorder.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", recorder.Code, recorder.Body.String())
	}
	var answer siteConfigView
	_ = json.Unmarshal(recorder.Body.Bytes(), &answer)
	if answer.State != transport.SiteFileManagedUnchanged || answer.Outcome != transport.SiteFileOutcomeWritten ||
		answer.BackupPath != "" || len(answer.Actions) != 0 {
		t.Fatalf("apply answer: %+v", answer)
	}
	if sent == nil || sent.FileTrigger != transport.SiteFileTriggerChange ||
		sent.ExpectedFileSHA256 != view.FileSHA256 || sent.ExpectedRenderSHA256 != view.RenderSHA256 {
		t.Fatalf("apply request: %+v", sent)
	}
	row = ledgerRow(t, f.panel, f.domainID)
	if row.State != transport.SiteFileManagedUnchanged || row.StateReason != "" || row.BodySHA256 != strings.Repeat("b", 64) ||
		row.BackupPath != "" || row.Decision != "" {
		t.Fatalf("apply, ledger: %+v", row)
	}
	var audits int
	_ = f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'domain.site_config.apply'`).Scan(&audits)
	if audits != 1 {
		t.Fatalf("apply audit rows: %d", audits)
	}
	// Written: nothing is offered any more.
	f.fileSHA = strings.Repeat("f", 64)
	if view := f.read(); view.State != transport.SiteFileManagedUnchanged || len(view.Actions) != 0 {
		t.Fatalf("after apply: %+v", view)
	}
}

func TestRemovedProtectionOnAChangedFileIsTheFileAsItIsNeverAWrite(t *testing.T) {
	f := newRecheckFixture(t, "changed.example", transport.SiteFileManagedUnchanged)
	f.refusedRender()
	renders := len(f.agent.requests)

	// The owner removed the protection and edited the file meanwhile.
	f.state, f.fileSHA = transport.SiteFileOwnerEdited, strings.Repeat("d", 64)
	f.protection, f.systemError = transport.SiteFileProtectionNone, ""
	view := f.read()
	if view.State != transport.SiteFileOwnerEdited || view.FileState != "" || strings.Join(view.Actions, ",") != "keep,take,merge" {
		t.Fatalf("changed file: %+v", view)
	}
	row := ledgerRow(t, f.panel, f.domainID)
	if row.State != transport.SiteFileOwnerEdited || row.StateReason != "" || row.FileSHA256 != f.fileSHA {
		t.Fatalf("changed file, ledger: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileOwnerEdited {
		t.Fatalf("changed file, list: %q", state)
	}
	if recorder := f.post("apply", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: view.RenderSHA256}); recorder.Code != http.StatusConflict ||
		!strings.Contains(recorder.Body.String(), errCodeSiteConfigNotApplicable) {
		t.Fatalf("apply on a changed file: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, request := range f.agent.requests[renders:] {
		if request.ExpectedFileSHA256 != "" {
			t.Fatalf("a render was sent for a changed file: %+v", request)
		}
	}
}

func TestARefusedTakeOnAnOwnerEditedFileShowsBothFactsEverywhere(t *testing.T) {
	f := newRecheckFixture(t, "edited-locked.example", transport.SiteFileOwnerEdited)
	renderSHA := strings.Repeat("b", 64)
	// The read does not measure the protection (another platform): take goes
	// to the Agent, whose replacement is refused.
	f.protection, f.systemError = "", ""
	f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
		file := fileResult(request.Domain, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeRefused)
		file.FileSHA256 = f.fileSHA
		file.Reason = transport.SiteFileReasonWriteRefused
		file.SystemError, file.Protection = "EPERM", transport.SiteFileProtectionImmutable
		response.File = &file
	}
	recorder := f.post("take", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: renderSHA})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), errCodeSiteConfigUnwritable) {
		t.Fatalf("refused take: %d %s", recorder.Code, recorder.Body.String())
	}
	takes := 0
	for _, request := range f.agent.requests {
		if request.FileTrigger == transport.SiteFileTriggerTake && request.ExpectedFileSHA256 != "" {
			takes++
		}
	}
	if takes != 1 {
		t.Fatalf("takes sent: %d", takes)
	}

	// The page, the row and the badge: one state by one precedence.
	f.protection, f.systemError = transport.SiteFileProtectionImmutable, "EPERM"
	view := f.read()
	if view.State != transport.SiteFileUnwritable || view.FileState != transport.SiteFileOwnerEdited ||
		view.Reason != transport.SiteFileReasonWriteRefused || view.SystemError != "EPERM" ||
		view.Protection != transport.SiteFileProtectionImmutable ||
		strings.Join(view.Actions, ",") != "keep,take,merge,recheck" {
		t.Fatalf("combined page: %+v", view)
	}
	if state := f.listState(); state != view.State {
		t.Fatalf("list %q, page %q", state, view.State)
	}

	// Take stays offered, and is answered unwritable while the protection
	// stands, before anything is written (no dated copy).
	recorder = f.post("take", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: renderSHA})
	var refusal apiErrorBody
	_ = json.Unmarshal(recorder.Body.Bytes(), &refusal)
	if recorder.Code != http.StatusConflict || refusal.Code != errCodeSiteConfigUnwritable || refusal.Reason != transport.SiteFileReasonWriteRefused {
		t.Fatalf("take while protected: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, request := range f.agent.requests {
		if request.FileTrigger == transport.SiteFileTriggerTake && request.ExpectedFileSHA256 != "" {
			takes--
		}
	}
	if takes != 0 {
		t.Fatal("a take was sent to the Agent while the protection stands")
	}
	if state := f.listState(); state != transport.SiteFileUnwritable {
		t.Fatalf("list after the refused take: %q", state)
	}

	// The protection removed: the owner's file, its choices open again.
	f.protection, f.systemError = transport.SiteFileProtectionNone, ""
	view = f.read()
	if view.State != transport.SiteFileOwnerEdited || view.FileState != "" || strings.Join(view.Actions, ",") != "keep,take,merge" {
		t.Fatalf("unprotected owner file: %+v", view)
	}
	if state := f.listState(); state != transport.SiteFileOwnerEdited {
		t.Fatalf("list after the protection was removed: %q", state)
	}
}

func TestSiteFileStatePrecedence(t *testing.T) {
	for _, tc := range []struct{ first, second, want string }{
		{transport.SiteFileOwnerEdited, transport.SiteFileUnwritable, transport.SiteFileUnwritable},
		{transport.SiteFileManagedUnchanged, transport.SiteFileUnwritable, transport.SiteFileUnwritable},
		{transport.SiteFileUnreadable, transport.SiteFileUnwritable, transport.SiteFileUnreadable},
		{transport.SiteFileForeign, transport.SiteFileMissingState, transport.SiteFileMissingState},
		{transport.SiteFileUnknownOrigin, transport.SiteFileManagedUnchanged, transport.SiteFileUnknownOrigin},
		{transport.SiteFileOwnerEdited, transport.SiteFileForeign, transport.SiteFileOwnerEdited},
	} {
		if got := services.SiteFileShownState(tc.first, tc.second); got != tc.want {
			t.Fatalf("%s + %s: %q, want %q", tc.first, tc.second, got, tc.want)
		}
	}
}
