package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 fix round of PR #371 (the independent review's F6, F8, F9): the
// Panel's side.

// TestAnEventBeforeAChangeKeepsTheLimitThatHeld (F8; lab CHECK 2 Q2): the
// limit is lowered to 128 MB, a program hits it, and the limit is put back to
// the default before any reading. The change reads the site's counters first,
// so the event is recorded with 128 MB, never with today's value. When that
// read cannot be made, the event the next reading finds carries no limit.
func TestAnEventBeforeAChangeKeepsTheLimitThatHeld(t *testing.T) {
	for _, c := range []struct {
		name      string
		readFails bool
		want      int
	}{
		{"read before the change", false, 128},
		{"the read cannot be made", true, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newRightsFixture(t)
			siteID := round2Site(t, f)
			ctx := context.Background()
			oldRead, oldApply, oldNeeds := readSiteSlicesCall, applySiteSlicesCall, siteLimitReadNeedsAgent
			t.Cleanup(func() { readSiteSlicesCall, applySiteSlicesCall, siteLimitReadNeedsAgent = oldRead, oldApply, oldNeeds })
			siteLimitReadNeedsAgent = false
			status := siteLimitsStatus(siteID, 0, 0).Sites[0]
			lowered := transport.SiteSliceLimitsRead{MemoryMB: 128, Tasks: 512, CPUPercent: 200}
			status.Live, status.Kernel, status.KernelState = lowered, lowered, transport.SiteSliceInForce
			readFails := false
			readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
				if readFails {
					return transport.SiteSlicesStatusResponse{}, errors.New("timeout")
				}
				return transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{status}}, nil
			}
			applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
				restored := transport.SiteSliceLimitsRead{MemoryMB: 3072, Tasks: 512, CPUPercent: 200}
				status.Live, status.Kernel = restored, restored // the push: the default holds now
				return transport.ApplySiteSlicesResponse{Sites: []transport.SiteSliceApplied{{SiteID: siteID, Outcome: transport.SiteFileOutcomeWritten}}}, nil
			}
			now := time.Now()
			f.panel.noteSiteSlices(ctx, []transport.SiteSliceStatus{status}, false, now) // the baseline
			// The hog at 128 MB; no reading before the limit is put back.
			status.Counters.MemoryLimitHits, status.Counters.MemoryOOMAll, status.Counters.OOMKills = 1, 1, 1
			readFails = c.readFails
			if err := f.panel.reapplySiteSlicesNow(ctx, "Lean", []int{9802}); err != nil {
				t.Fatal(err)
			}
			readFails = false
			f.panel.noteSiteSlices(ctx, []transport.SiteSliceStatus{status}, false, now.Add(time.Minute)) // the card, later
			event := f.panel.loadSiteLimitLedger(ctx)[itoa(siteID)].Events["memory"]
			if event == nil || event.Count != 1 || event.Limit != c.want {
				t.Fatalf("the event: %+v (want the limit %d)", event, c.want)
			}
		})
	}
}

// TestTheUnappliedLineWordsEachSiteByItsReason (F9): a batch in which one
// site's kernel still holds the earlier values and another could not be read
// is ONE line that counts each reason: only the first is said to lack the
// limits; the mail says the same.
func TestTheUnappliedLineWordsEachSiteByItsReason(t *testing.T) {
	f := newRightsFixture(t)
	siteID := round2Site(t, f)
	f.exec(t, `INSERT INTO domains (id, subscription_id, name, status) VALUES (9904, 9802, 'second.matrix.test', 'active')`)
	f.exec(t, `INSERT INTO sites (domain_id, document_root, project_type) VALUES (9904, '/srv/node2', 'node')`)
	var second int
	if err := f.panel.db.GetDB().QueryRow(`SELECT id FROM sites WHERE domain_id = 9904`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	oldApply, oldNeeds := applySiteSlicesCall, siteLimitReadNeedsAgent
	t.Cleanup(func() { applySiteSlicesCall, siteLimitReadNeedsAgent = oldApply, oldNeeds })
	siteLimitReadNeedsAgent = false
	applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
		return transport.ApplySiteSlicesResponse{Sites: []transport.SiteSliceApplied{
			{SiteID: siteID, Outcome: transport.SiteFileOutcomeFailed, Reason: "kernel_differs"},
			{SiteID: second, Outcome: transport.SiteFileOutcomeFailed, Reason: "unchecked"},
		}}, nil
	}
	if err := f.panel.reapplySiteSlicesNow(ctx, "Lean", []int{9802}); err != nil {
		t.Fatal(err)
	}
	lines := siteLimitApplyProblem(f.panel.setting(ctx, settingSiteLimitApply))
	if len(lines) != 1 || lines[0].Name != "kernel_differs:1,unchecked:1" || lines[0].Value != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if lacking, unknown := siteLimitApplyCounts(lines[0].Name, 2); lacking != 1 || unknown != 1 {
		t.Fatalf("lacking %d, not known %d", lacking, unknown)
	}
	en, tr := alertLine(pendingAlertMail{kind: "site_limits_unapplied", subject: "Lean", name: lines[0].Name, value: 2})
	if !strings.Contains(en, "the earlier limits still hold for 1 site, and for 1 site it is not known") || !strings.Contains(tr, "1 site için önceki sınırlar sürüyor") {
		t.Fatalf("the mail: %s / %s", en, tr)
	}
}

// TestTheUnappliedCountsReadEarlierRecords (F9): a record written before the
// per-reason counts (one code for every site) is counted under that code.
func TestTheUnappliedCountsReadEarlierRecords(t *testing.T) {
	lines := siteLimitApplyProblem(`{"change":"Lean","pending":{"9802":3},"reason":"unchecked","at":"2026-11-26T08:00:00Z"}`)
	if len(lines) != 1 || lines[0].Name != "unchecked:3" {
		t.Fatalf("lines = %+v", lines)
	}
	if lacking, unknown := siteLimitApplyCounts("kernel_differs", 4); lacking != 4 || unknown != 0 {
		t.Fatalf("an earlier single code: %d %d", lacking, unknown)
	}
	if lacking, unknown := siteLimitApplyCounts("agent_unreachable:2,write_failed:1", 3); lacking != 1 || unknown != 2 {
		t.Fatalf("counts: %d %d", lacking, unknown)
	}
}

// TestAMoveIsRefusedWhileTheUpdateIsInProgress (F6): while an update the
// panel admitted is queued or running, the move and Try again are refused
// with their code and the plain sentence, and the agent is not asked.
func TestAMoveIsRefusedWhileTheUpdateIsInProgress(t *testing.T) {
	f := newRightsFixture(t)
	round2Site(t, f)
	oldUpdate, oldMove := siteMoveUpdateInProgress, moveSiteSliceCall
	t.Cleanup(func() { siteMoveUpdateInProgress, moveSiteSliceCall = oldUpdate, oldMove })
	siteMoveUpdateInProgress = func(*Panel, context.Context) bool { return true }
	asked := 0
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		asked++
		return transport.MoveSiteSliceResponse{Done: true}, nil
	}
	handler := siteLimitsMux(f)
	for i, body := range []string{`{"request_id":"a149fixupdate01"}`, `{"request_id":"a149fixupdate02","recover":true}`} {
		w := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", body)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "site_limits_update_running") || !strings.Contains(w.Body.String(), "update is in progress") {
			t.Fatalf("%d: status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
	if asked != 0 {
		t.Fatalf("the agent was asked to move %d times during the update", asked)
	}
	// After the update the move goes ahead.
	siteMoveUpdateInProgress = func(*Panel, context.Context) bool { return false }
	w := siteLimitsCall(t, f, handler, authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"a149fixupdate03","recover":true}`)
	if w.Code != http.StatusOK || asked != 1 {
		t.Fatalf("after the update: status=%d asked=%d body=%s", w.Code, asked, w.Body.String())
	}
}
