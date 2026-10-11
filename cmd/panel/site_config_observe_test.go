package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The thirteenth record's observation (set13, 2026-10-11): after a refused
// "take" the ledger kept the row foreign / nginx_refused when the owner put
// CelikPanel's text back by hand, so the Domains row kept asking for a choice
// the page no longer offered. A page read records what it sees.

func (f *recheckFixture) refusedTake(shaOfFile string) {
	f.t.Helper()
	f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
		file := fileResult(request.Domain, transport.SiteFileForeign, transport.SiteFileOutcomeFailed)
		file.FileSHA256 = shaOfFile
		file.Reason, file.Detail, file.Restore = transport.SiteFileReasonNginxRefused, "nginx: [emerg] refused", transport.SiteFileRestoreConfirmed
		response.File = &file
		response.Error = "nginx validation failed: exit status 1; rollback restored the previous vhost"
	}
	recorder := f.post("take", siteConfigActionRequest{FileSHA256: shaOfFile, RenderSHA256: strings.Repeat("b", 64)})
	if recorder.Code != http.StatusBadGateway {
		f.t.Fatalf("refused take: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAPageReadRecordsWhatTheOwnerDidAfterARefusedTake(t *testing.T) {
	f := newRecheckFixture(t, "restored-by-hand.example", transport.SiteFileForeign)
	f.protection, f.systemError = "", ""
	foreignSHA := f.fileSHA
	f.refusedTake(foreignSHA)

	// The refusal: the owner's file stays, the row says why.
	if row := ledgerRow(t, f.panel, f.domainID); row.State != transport.SiteFileForeign || row.StateReason != transport.SiteFileReasonNginxRefused {
		t.Fatalf("after the refused take: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileForeign {
		t.Fatalf("list after the refused take: %q", state)
	}
	// Reading the same file again changes nothing: the refusal is about these bytes.
	if view := f.read(); view.State != transport.SiteFileForeign {
		t.Fatalf("read of the same file: %+v", view)
	}
	if row := ledgerRow(t, f.panel, f.domainID); row.State != transport.SiteFileForeign || row.StateReason != transport.SiteFileReasonNginxRefused ||
		row.FileSHA256 != foreignSHA {
		t.Fatalf("same file, ledger: %+v", row)
	}

	// The owner restores CelikPanel's text by hand: the page says so, and so
	// do the row and the Domains list; the reason of the refusal is gone.
	f.state, f.fileSHA = transport.SiteFileManagedUnchanged, strings.Repeat("d", 64)
	view := f.read()
	if view.State != transport.SiteFileManagedUnchanged || len(view.Actions) != 0 {
		t.Fatalf("restored by hand, page: %+v", view)
	}
	row := ledgerRow(t, f.panel, f.domainID)
	if row.State != transport.SiteFileManagedUnchanged || row.StateReason != "" || row.FileSHA256 != f.fileSHA {
		t.Fatalf("restored by hand, ledger: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileManagedUnchanged {
		t.Fatalf("restored by hand, list: %q", state)
	}
	// Nothing about what CelikPanel wrote was touched by the read.
	if row.WrittenAt != 0 || row.BodySHA256 != "" || row.Decision != "" {
		t.Fatalf("the read changed more than the observation: %+v", row)
	}

	// The owner edits again: the row follows.
	f.state, f.fileSHA = transport.SiteFileOwnerEdited, strings.Repeat("e", 64)
	if view := f.read(); view.State != transport.SiteFileOwnerEdited || strings.Join(view.Actions, ",") != "keep,take,merge" {
		t.Fatalf("edited again, page: %+v", view)
	}
	row = ledgerRow(t, f.panel, f.domainID)
	if row.State != transport.SiteFileOwnerEdited || row.StateReason != "" || row.FileSHA256 != f.fileSHA {
		t.Fatalf("edited again, ledger: %+v", row)
	}
	if state := f.listState(); state != transport.SiteFileOwnerEdited {
		t.Fatalf("edited again, list: %q", state)
	}
}

func TestAKeptReadingKeepsTheOwnersDecisionOnlyOnTheDigestItWasMadeOn(t *testing.T) {
	f := newRecheckFixture(t, "decided.example", transport.SiteFileOwnerEdited)
	f.protection, f.systemError = "", ""
	if recorder := f.post("keep", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: strings.Repeat("b", 64)}); recorder.Code != http.StatusOK {
		t.Fatalf("keep: %d %s", recorder.Code, recorder.Body.String())
	}
	decided := f.fileSHA

	// The same file read again as another kept kind: the decision stands.
	f.state = transport.SiteFileForeign
	view := f.read()
	if view.Decision == nil || view.Decision.Kind != services.SiteFileDecisionKeepMine || !view.Decision.Current {
		t.Fatalf("same digest: %+v", view.Decision)
	}
	row := ledgerRow(t, f.panel, f.domainID)
	if row.Decision != services.SiteFileDecisionKeepMine || row.DecisionFileSHA256 != decided || row.State != transport.SiteFileForeign {
		t.Fatalf("same digest, ledger: %+v", row)
	}
	summaries, err := f.panel.domainSiteConfigSummaries(context.Background())
	if err != nil || !summaries[f.domainID].KeptByChoice {
		t.Fatalf("same digest, list: %+v %v", summaries[f.domainID], err)
	}

	// The owner changed the file: the decision is kept in the row, but is not current.
	f.state, f.fileSHA = transport.SiteFileOwnerEdited, strings.Repeat("f", 64)
	view = f.read()
	if view.Decision == nil || view.Decision.Current {
		t.Fatalf("other digest: %+v", view.Decision)
	}
	row = ledgerRow(t, f.panel, f.domainID)
	if row.Decision != services.SiteFileDecisionKeepMine || row.DecisionFileSHA256 != decided || row.FileSHA256 != f.fileSHA {
		t.Fatalf("other digest, ledger: %+v", row)
	}
	summaries, err = f.panel.domainSiteConfigSummaries(context.Background())
	if err != nil || summaries[f.domainID].KeptByChoice {
		t.Fatalf("other digest, list: %+v %v", summaries[f.domainID], err)
	}
}

func TestAReadDoesNotEndTheRemovedProtectionReason(t *testing.T) {
	f := newRecheckFixture(t, "protection-stays.example", transport.SiteFileManagedUnchanged)
	f.refusedRender()
	f.protection, f.systemError = transport.SiteFileProtectionNone, ""
	if view := f.read(); view.State != transport.SiteFileManagedUnchanged || view.Reason != services.SiteFileReasonProtectionRemoved {
		t.Fatalf("protection removed: %+v", view)
	}
	// Read again: the reason that offers "apply" stays.
	f.read()
	if row := ledgerRow(t, f.panel, f.domainID); row.StateReason != services.SiteFileReasonProtectionRemoved {
		t.Fatalf("ledger after another read: %+v", row)
	}
}

func TestAPoolReadEndsTheRefusalOfARestoredPoolToo(t *testing.T) {
	f := newOtherSiteFileFixture(t, "pool-restored.example", "php")
	state, sha := transport.SiteFileForeign, otherFileSHA
	f.agent.setPoolInspect(func(request *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindPHPPool, poolTestPath, state, transport.SiteFileOutcomeInspected)
		response.File.FileSHA256 = sha
	})
	f.agent.setPoolApply(func(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
		file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileForeign, transport.SiteFileOutcomeFailed)
		file.Reason, file.Restore = transport.SiteFileReasonConfigRefused, transport.SiteFileRestoreConfirmed
		response.File = &file
		response.Error = "php-fpm -t refused"
	})
	if recorder := f.post("take", siteConfigActionRequest{Kind: transport.SiteFileKindPHPPool, FileSHA256: otherFileSHA, RenderSHA256: otherRenderSHA}); recorder.Code != http.StatusBadGateway {
		t.Fatalf("refused pool take: %d %s", recorder.Code, recorder.Body.String())
	}
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileForeign || row.StateReason != transport.SiteFileReasonConfigRefused {
		t.Fatalf("pool after the refused take: %+v", row)
	}
	state, sha = transport.SiteFileManagedUnchanged, strings.Repeat("9", 64)
	f.read(t)
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileManagedUnchanged || row.StateReason != "" || row.FileSHA256 != sha {
		t.Fatalf("pool restored by hand: %+v", row)
	}
	// A managed row with a refusal reason on the same bytes (a refused apply
	// that put the file back) is ended by the read as well.
	if _, err := f.panel.db.GetDB().Exec(`UPDATE managed_site_files SET state_reason = ? WHERE kind = ?`,
		transport.SiteFileReasonConfigRefused, transport.SiteFileKindPHPPool); err != nil {
		t.Fatal(err)
	}
	f.read(t)
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileManagedUnchanged || row.StateReason != "" {
		t.Fatalf("pool, managed row with a refusal reason: %+v", row)
	}
}

func TestASuccessfulWriteSaysTheServiceWasReloaded(t *testing.T) {
	f := newRecheckFixture(t, "reload-says.example", transport.SiteFileForeign)
	f.protection, f.systemError = "", ""
	for _, c := range []struct {
		name, active, wantActive string
		reloaded                 bool
	}{
		{"reloaded, no measurement", "", transport.SiteFileServiceActiveTrue, true},
		{"reloaded, measured", transport.SiteFileServiceActiveTrue, transport.SiteFileServiceActiveTrue, true},
		{"not reloaded", "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
				file := fileResult(request.Domain, transport.SiteFileForeign, transport.SiteFileOutcomeTaken)
				file.Reloaded, file.ServiceActive = c.reloaded, c.active
				response.File = &file
			}
			recorder := f.post("take", siteConfigActionRequest{FileSHA256: f.fileSHA, RenderSHA256: strings.Repeat("b", 64)})
			var view siteConfigView
			if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || recorder.Code != http.StatusOK {
				t.Fatalf("take: %d %s", recorder.Code, recorder.Body.String())
			}
			wantService := ""
			if c.reloaded {
				wantService = transport.SiteFileServiceReloaded
			}
			if view.Outcome != transport.SiteFileOutcomeTaken || view.Service != wantService || view.ServiceActive != c.wantActive {
				t.Fatalf("answer: %+v", view)
			}
			if !c.reloaded && (strings.Contains(recorder.Body.String(), `"service"`) || strings.Contains(recorder.Body.String(), "service_active")) {
				t.Fatalf("an unreloaded write names a service: %s", recorder.Body.String())
			}
		})
	}
}

func TestASuccessfulPoolWriteSaysTheServiceWasReloaded(t *testing.T) {
	f := newOtherSiteFileFixture(t, "pool-reload.example", "php")
	f.agent.setPoolInspect(func(request *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeInspected)
	})
	f.agent.setPoolApply(func(request *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
		file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeTaken)
		file.Reloaded, file.WrittenSHA256 = true, otherRenderSHA
		response.File = &file
	})
	recorder := f.post("take", siteConfigActionRequest{Kind: transport.SiteFileKindPHPPool, FileSHA256: otherFileSHA, RenderSHA256: otherRenderSHA})
	var view siteConfigView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || recorder.Code != http.StatusOK ||
		view.Service != transport.SiteFileServiceReloaded || view.ServiceActive != transport.SiteFileServiceActiveTrue {
		t.Fatalf("pool take: %d %s", recorder.Code, recorder.Body.String())
	}
}
