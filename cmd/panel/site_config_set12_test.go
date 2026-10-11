package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Corrections from the twelfth native record (set12, 2026-10-11) at the
// Panel: an unread file reaches the Domains row (cell U), and a confirmed
// put-back says whether the service was reloaded (cell RP1).

// set12 cell U: a read of the pool that got no answer (the Agent's pool lock
// held it past the read's time) reaches the Domains row: the ledger records
// the observation (unknown, not_read, the error class) without a digest, the
// row and its badge read it, and the next successful read replaces it.
func TestAnUnreadPoolReachesTheDomainsRowAndASuccessfulReadEndsIt(t *testing.T) {
	f := newOtherSiteFileFixture(t, "unread.example", "php")
	managed := func(_ *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeInspected)
		response.File.WrittenSHA256 = otherRenderSHA
	}
	f.agent.setPoolInspect(managed)
	f.read(t)
	before := f.row(t, transport.SiteFileKindPHPPool)
	if before.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("pool row before: %+v", before)
	}
	summary := func() domainSiteConfigSummary {
		summaries, err := f.panel.domainSiteConfigSummaries(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return summaries[f.domainID]
	}

	// The Agent does not answer within the read's time.
	release := make(chan struct{})
	f.agent.setPoolInspect(func(request *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		<-release
		managed(request, response)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	view := f.panel.withOtherSiteFiles(ctx, f.domainID, siteConfigView{DomainID: f.domainID, Kind: transport.SiteFileKindNginxVhost,
		State: transport.SiteFileManagedUnchanged, Actions: []string{}})
	cancel()
	close(release)
	pool := fileByKind(view, transport.SiteFileKindPHPPool)
	if pool == nil || pool.State != transport.SiteFileStateUnknown || pool.Reason != siteConfigReasonNotRead || pool.ReadError != "timeout" ||
		view.OverallState != transport.SiteFileStateUnknown || view.OverallKind != transport.SiteFileKindPHPPool {
		t.Fatalf("page: %+v %q %q", pool, view.OverallState, view.OverallKind)
	}
	row := f.row(t, transport.SiteFileKindPHPPool)
	if row.State != transport.SiteFileStateUnknown || row.StateReason != siteConfigReasonNotRead+":timeout" ||
		row.FileSHA256 != before.FileSHA256 || row.BodySHA256 != before.BodySHA256 || row.Path != before.Path {
		t.Fatalf("pool row after the unread read: %+v (before %+v)", row, before)
	}
	if got := summary(); got.State != transport.SiteFileStateUnknown || got.Kind != transport.SiteFileKindPHPPool ||
		got.Reason != siteConfigReasonNotRead || got.ReadError != "timeout" {
		t.Fatalf("Domains row: %+v", got)
	}

	// The next read answers: the row is back to what the file is.
	f.agent.setPoolInspect(managed)
	f.read(t)
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileManagedUnchanged || row.StateReason != "" {
		t.Fatalf("pool row after a successful read: %+v", row)
	}
	if got := summary(); got.State != transport.SiteFileManagedUnchanged || got.Reason != "" || got.ReadError != "" {
		t.Fatalf("Domains row after a successful read: %+v", got)
	}

	// An Agent that answers with an error: agent_error.
	f.agent.setPoolInspect(func(_ *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.Error = "the pool could not be read just now"
	})
	f.read(t)
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileStateUnknown || row.StateReason != siteConfigReasonNotRead+":agent_error" {
		t.Fatalf("pool row after an Agent error: %+v", row)
	}

	// A known problem is not hidden by a read that failed.
	f.agent.setPoolInspect(func(_ *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.File = otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeInspected)
	})
	f.read(t)
	f.agent.setPoolInspect(func(_ *transport.SitePoolRequest, response *transport.InspectSiteFileResponse) {
		response.Error = "the pool could not be read just now"
	})
	f.read(t)
	if row := f.row(t, transport.SiteFileKindPHPPool); row.State != transport.SiteFileOwnerEdited {
		t.Fatalf("an owner's file was replaced by an unread observation: %+v", row)
	}
	if got := summary(); got.State != transport.SiteFileOwnerEdited || got.Kind != transport.SiteFileKindPHPPool {
		t.Fatalf("Domains row: %+v", got)
	}
}

// set12 cell RP1 at the Panel: a confirmed put-back after which the service
// was deliberately not reloaded (its check refused the configuration
// elsewhere), or was not running before, has its own sentence; the check's
// line reaches an administrator only; service_active is in the answer.
func TestServiceRefusalSaysWhetherTheServiceWasReloaded(t *testing.T) {
	const line = "php-fpm8.3 -t: ERROR: [/etc/php/8.3/fpm/pool.d/zz-owner.conf:1] unknown entry 'x'"
	f := newOtherSiteFileFixture(t, "service.example", "php")
	ctx := context.Background()
	site, err := f.panel.loadSiteFileSite(ctx, f.domainID)
	if err != nil {
		t.Fatal(err)
	}
	customer := &Caller{ID: f.admin.ID + 1000, Role: "customer"}
	for _, c := range []struct {
		name, service, reason, active string
		wantAdmin, wantOther          string
	}{
		{"not reloaded", transport.SiteFileServiceNotReloaded, transport.SiteFileServiceReasonConfigRefusedElsewhere, transport.SiteFileServiceActiveTrue,
			"The previous file is back. PHP-FPM was not reloaded, because its configuration is refused by another file — PHP-FPM keeps running with the configuration it loaded before. Fix what the check names (" + line + "), then reload PHP-FPM yourself.",
			"The server administrator fixes what PHP-FPM's check names, then reloads PHP-FPM."},
		{"not running", transport.SiteFileServiceNotRunning, "", transport.SiteFileServiceActiveFalse,
			"PHP-FPM was not running before this change, so CelikPanel neither reloaded nor started it.",
			"PHP-FPM was not running before this change, so CelikPanel neither reloaded nor started it."},
		{"reloaded", transport.SiteFileServiceReloaded, "", "",
			"so the previous pool file was put back and PHP-FPM keeps running with it", "so the previous pool file was put back"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.agent.setPoolApply(func(_ *transport.SitePoolRequest, response *transport.ApplySiteFileResponse) {
				file := otherFile(transport.SiteFileKindPHPPool, poolTestPath, transport.SiteFileOwnerEdited, transport.SiteFileOutcomeFailed)
				file.Reason, file.Detail, file.Restore = transport.SiteFileReasonConfigRefused, "php-fpm8.3 -t: exit status 78", transport.SiteFileRestoreConfirmed
				file.Service, file.ServiceReason, file.ServiceActive = c.service, c.reason, c.active
				if c.reason != "" {
					file.ServiceCheck = line
				}
				response.File = &file
			})
			request := f.panel.sitePoolRequest(ctx, site, transport.SitePoolOperationTake)
			_, applyErr := f.panel.applySitePool(ctx, site, request)
			for _, caller := range []*Caller{f.admin, customer} {
				recorder := httptest.NewRecorder()
				writeServerErrorFor(recorder, siteConfigRequest(http.MethodPost, "/x", nil, caller), applyErr)
				body := decodeRestoreErrorBody(t, recorder)
				wantActive := c.active
				if wantActive == "" {
					wantActive = transport.SiteFileServiceActiveUnknown
				}
				if recorder.Code != http.StatusBadGateway || body.Code != errCodeSiteConfigServiceRefused || body.Restore != transport.SiteFileRestoreConfirmed ||
					body.Service != c.service || body.ServiceReason != c.reason || body.ServiceActive != wantActive {
					t.Fatalf("answer: %d %+v", recorder.Code, body)
				}
				admin := caller == f.admin
				want := c.wantOther
				if admin {
					want = c.wantAdmin
				}
				if !strings.Contains(body.Error, want) {
					t.Fatalf("sentence for %s: %q", caller.Role, body.Error)
				}
				if c.reason != "" && admin && (body.Vars["line"] != line || len(body.Details) == 0 || body.Details[0] != line) {
					t.Fatalf("administrator's line: %+v %+v", body.Vars, body.Details)
				}
				if !admin && (strings.Contains(body.Error, "zz-owner") || len(body.Vars) != 0 || len(body.Details) != 0) {
					t.Fatalf("the check's line reached a caller who is not an administrator: %+v", body)
				}
			}
		})
	}
}

func TestNginxRefusalSaysWhetherNginxWasReloaded(t *testing.T) {
	const line = `nginx: [emerg] unknown directive "bogus" in /etc/nginx/sites-enabled/owner.conf:3`
	f := newOtherSiteFileFixture(t, "nginx-service.example", "php")
	for _, c := range []struct {
		name, service, reason, active, want string
	}{
		{"not reloaded", transport.SiteFileServiceNotReloaded, transport.SiteFileServiceReasonConfigRefusedElsewhere, transport.SiteFileServiceActiveTrue,
			"nginx was not reloaded, because its configuration is refused by another file — nginx keeps running with the configuration it loaded before. Fix what the check names (" + line + "), then reload nginx yourself."},
		{"not running", transport.SiteFileServiceNotRunning, "", transport.SiteFileServiceActiveFalse,
			"nginx was not running before this change, so CelikPanel neither reloaded nor started it."},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.agent.single = func(request *transport.ApplyVhostRequest, response *transport.ApplyVhostResponse) {
				file := fileResult(request.Domain, transport.SiteFileManagedUnchanged, transport.SiteFileOutcomeFailed)
				file.Reason, file.Detail, file.Restore = transport.SiteFileReasonNginxRefused, "nginx: [emerg] refused", transport.SiteFileRestoreConfirmed
				file.Service, file.ServiceReason, file.ServiceActive = c.service, c.reason, c.active
				if c.reason != "" {
					file.ServiceCheck = line
				}
				response.File = &file
				response.Error = "nginx validation failed: exit status 1; rollback restored the previous vhost"
			}
			recorder := f.post("recreate", siteConfigActionRequest{})
			body := decodeRestoreErrorBody(t, recorder)
			if recorder.Code != http.StatusBadGateway || body.Code != errCodeSiteConfigNginxRefused || body.Restore != transport.SiteFileRestoreConfirmed ||
				body.Service != c.service || body.ServiceReason != c.reason || body.ServiceActive != c.active || !strings.Contains(body.Error, c.want) {
				t.Fatalf("answer: %d %+v", recorder.Code, body)
			}
			if c.reason != "" && (body.Vars["line"] != line || !containsString(body.Details, line)) {
				t.Fatalf("nginx -t's line: %+v %+v", body.Vars, body.Details)
			}
		})
	}
}
