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

// 2026-10-11 corrections of D-031's second step at the Panel: a service
// refusal says whether the previous file is back (restore: confirmed |
// unconfirmed, the Agent's line for an administrator only); a file the Agent
// did not read outranks CelikPanel's unchanged text on the page's overall
// state and on the Domains row; the unit's RPCs carry the Panel's build.

const restoreNotConfirmed = "whether it is in place is not confirmed"

type restoreCase struct {
	name       string
	restore    string
	agentError string
	want       string
}

func restoreCases() []restoreCase {
	return []restoreCase{
		{"confirmed", transport.SiteFileRestoreConfirmed, "", transport.SiteFileRestoreConfirmed},
		{"unconfirmed", transport.SiteFileRestoreUnconfirmed,
			"php-fpm reload: exit status 1; rollback incomplete: restore /etc/php/8.3/fpm/pool.d/site1.conf: read-only file system",
			transport.SiteFileRestoreUnconfirmed},
		{"no restore fact (an older Agent)", "", "", transport.SiteFileRestoreUnconfirmed},
		{"confirmed but the Agent says rollback incomplete", transport.SiteFileRestoreConfirmed,
			"reload failed; rollback incomplete: rollback reload: exit status 1", transport.SiteFileRestoreUnconfirmed},
	}
}

func decodeRestoreErrorBody(t *testing.T, recorder *httptest.ResponseRecorder) apiErrorBody {
	t.Helper()
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %v %s", err, recorder.Body.String())
	}
	return body
}

func checkRestoreAnswer(t *testing.T, c restoreCase, body apiErrorBody, admin bool, putBack string) {
	t.Helper()
	if body.Restore != c.want {
		t.Fatalf("restore = %q, want %q (%+v)", body.Restore, c.want, body)
	}
	unconfirmed := c.want == transport.SiteFileRestoreUnconfirmed
	if strings.Contains(body.Error, restoreNotConfirmed) != unconfirmed || strings.Contains(body.Error, putBack) == unconfirmed {
		t.Fatalf("sentence for restore %s: %q", c.want, body.Error)
	}
	if unconfirmed && !strings.Contains(body.Error, "the server administrator checks the file on the server") {
		t.Fatalf("unconfirmed sentence names no one: %q", body.Error)
	}
	hasLine := false
	for _, line := range body.Details {
		if c.agentError != "" && strings.Contains(line, "rollback incomplete") {
			hasLine = true
		}
	}
	if admin && c.agentError != "" && strings.Contains(c.agentError, "rollback incomplete") && !hasLine {
		t.Fatalf("administrator did not get the Agent's line: %+v", body.Details)
	}
	if !admin && len(body.Details) != 0 {
		t.Fatalf("details reached a caller who is not an administrator: %+v", body.Details)
	}
}

func TestPoolServiceRefusalSaysWhetherThePreviousFileIsBack(t *testing.T) {
	f := newOtherSiteFileFixture(t, "restore.example", "php")
	ctx := context.Background()
	site, err := f.panel.loadSiteFileSite(ctx, f.domainID)
	if err != nil {
		t.Fatal(err)
	}
	customer := &Caller{ID: f.admin.ID + 1000, Role: "customer"}
	for _, c := range restoreCases() {
		t.Run(c.name, func(t *testing.T) {
			f.agent.poolApply = func(_ *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
				file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeFailed)
				file.Reason, file.Detail, file.Restore = transport.SiteFileReasonReloadFailed, "php-fpm reload: exit status 1", c.restore
				response.File, response.Error = &file, c.agentError
			}
			request := f.panel.sitePoolRequest(ctx, site, transport.SitePoolOperationSave)
			request.Values = &transport.SitePoolValues{PM: "static", PMMaxChildren: 4}
			_, applyErr := f.panel.applySitePool(ctx, site, request)
			if applyErr == nil {
				t.Fatal("a failed save was not an error")
			}
			for _, caller := range []*Caller{f.admin, customer} {
				recorder := httptest.NewRecorder()
				writeServerErrorFor(recorder, siteConfigRequest(http.MethodPost, "/api/v1/domains/1/php/pool", nil, caller), applyErr)
				body := decodeRestoreErrorBody(t, recorder)
				if recorder.Code != http.StatusBadGateway || body.Code != errCodeSiteConfigServiceRefused ||
					body.Reason != transport.SiteFileKindPHPPool || body.Detail != transport.SiteFileReasonReloadFailed {
					t.Fatalf("answer: %d %+v", recorder.Code, body)
				}
				checkRestoreAnswer(t, c, body, caller == f.admin, "so the previous pool file was put back")
			}
			// The panel log line carries the restore and the Agent's line.
			if c.agentError != "" && !strings.Contains(applyErr.Error(), "rollback incomplete") {
				t.Fatalf("log text lost the Agent's line: %v", applyErr)
			}
			if !strings.Contains(applyErr.Error(), "restore "+c.want) {
				t.Fatalf("log text lacks the restore: %v", applyErr)
			}
		})
	}
}

func TestUnitServiceRefusalSaysWhetherThePreviousFileIsBack(t *testing.T) {
	f := newOtherSiteFileFixture(t, "unit-restore.example", "node")
	ctx := context.Background()
	for _, c := range restoreCases() {
		t.Run(c.name, func(t *testing.T) {
			f.agent.unitApply = func(_ *transport.AppApplyRequest, response *transport.AppApplyResponse) {
				file := otherFile(transport.SiteFileKindAppUnit, unitTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeFailed)
				file.Reason, file.Restore = transport.SiteFileReasonReloadFailed, c.restore
				response.File = &file
				response.Error = c.agentError
				if response.Error == "" && c.restore == transport.SiteFileRestoreConfirmed {
					response.Error = "application unit not activated, the previous file was put back: systemctl daemon-reload: exit status 1"
				}
			}
			_, err := f.panel.transitionHosting(ctx, f.domainID, hostingSettings{
				ProjectType: "node", StartCommand: "node server.js", RuntimeVersion: "22.11.0", AppPort: 3101,
			})
			recorder := httptest.NewRecorder()
			writeServerErrorFor(recorder, siteConfigRequest(http.MethodPut, "/api/v1/domains/1/hosting", nil, f.admin), err)
			body := decodeRestoreErrorBody(t, recorder)
			if recorder.Code != http.StatusBadGateway || body.Code != errCodeSiteConfigServiceRefused || body.Reason != transport.SiteFileKindAppUnit {
				t.Fatalf("answer: %d %+v (%v)", recorder.Code, body, err)
			}
			checkRestoreAnswer(t, c, body, true, "so the previous unit file was put back")
		})
	}
	// The unit's calls carry the Panel's build (the Agent's gate).
	for _, request := range f.agent.unitRequests {
		if request.ExpectedBuildCommit != strings.TrimSpace(buildCommit) {
			t.Fatalf("unit request without the Panel's build: %+v", request)
		}
	}
}

func TestNginxRefusalSaysWhetherThePreviousFileIsBack(t *testing.T) {
	f := newOtherSiteFileFixture(t, "nginx-restore.example", "php")
	for _, c := range restoreCases() {
		t.Run(c.name, func(t *testing.T) {
			f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
				file := fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeFailed)
				file.Reason, file.Detail, file.Restore = transport.SiteFileReasonNginxRefused, "nginx: [emerg] unknown directive", c.restore
				response.File = &file
				response.Error = c.agentError
				if response.Error == "" {
					response.Error = "nginx validation failed: exit status 1; previous vhost configuration restored"
				}
			}
			recorder := f.post("recreate", siteConfigActionRequest{})
			body := decodeRestoreErrorBody(t, recorder)
			if recorder.Code != http.StatusBadGateway || body.Code != errCodeSiteConfigNginxRefused || body.Reason != transport.SiteFileReasonNginxRefused {
				t.Fatalf("answer: %d %+v", recorder.Code, body)
			}
			checkRestoreAnswer(t, c, body, true, "so your file was put back")
			if len(body.Details) == 0 || body.Details[0] != "nginx: [emerg] unknown directive" {
				t.Fatalf("nginx's line: %+v", body.Details)
			}
		})
	}
}

func TestAFileTheAgentDidNotReadOutranksUnchangedText(t *testing.T) {
	f := newOtherSiteFileFixture(t, "notread.example", "php")
	f.agent.poolInspect = func(_ *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.Error = "the pool could not be read just now"
	}
	view := f.read(t)
	pool := fileByKind(view, transport.SiteFileKindPHPPool)
	if pool == nil || pool.State != transport.SiteFileStateUnknown || pool.Reason != siteConfigReasonNotRead ||
		strings.Join(pool.Actions, ",") != siteConfigActionRecheck {
		t.Fatalf("pool: %+v", view.Files)
	}
	if view.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("the vhost's own state changed: %q", view.State)
	}
	if view.OverallState != transport.SiteFileStateUnknown || view.OverallKind != transport.SiteFileKindPHPPool {
		t.Fatalf("overall = %q %q, want unknown php_pool", view.OverallState, view.OverallKind)
	}

	// The Domains row: an unknown pool row outranks the vhost's unchanged
	// row and names the pool; an observed problem still outranks unknown.
	ctx := context.Background()
	siteID, _, _ := f.panel.siteIDForDomain(ctx, f.domainID)
	vhost := transport.SiteFileResult{Kind: transport.SiteFileKindNginxVhost, Path: siteFilePath("notread.example"),
		State: transport.SiteFileManagedUnchanged, Outcome: transport.SiteFileOutcomeUnchanged, FileSHA256: otherFileSHA}
	if err := services.RecordSiteFileResult(ctx, f.panel.db.GetDB(), siteID, f.domainID, vhost, "test", 0); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{siteConfigReasonNotRead, siteConfigReasonAgentDoesNotReport} {
		if err := services.RecordSiteFileUnknownKind(ctx, f.panel.db.GetDB(), siteID, f.domainID, transport.SiteFileKindPHPPool, poolTestPath, reason, 0); err != nil {
			t.Fatal(err)
		}
		summaries, err := f.panel.domainSiteConfigSummaries(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if summary := summaries[f.domainID]; summary.State != transport.SiteFileStateUnknown || summary.Kind != transport.SiteFileKindPHPPool {
			t.Fatalf("%s: row = %+v, want unknown php_pool", reason, summary)
		}
	}
	vhost.State, vhost.Outcome = transport.SiteFileOwnerEdited, transport.SiteFileOutcomeKept
	if err := services.RecordSiteFileResult(ctx, f.panel.db.GetDB(), siteID, f.domainID, vhost, "test", 0); err != nil {
		t.Fatal(err)
	}
	summaries, _ := f.panel.domainSiteConfigSummaries(ctx)
	if summary := summaries[f.domainID]; summary.State != transport.SiteFileOwnerEdited || summary.Kind != "" {
		t.Fatalf("an observed problem did not outrank unknown: %+v", summary)
	}
}
