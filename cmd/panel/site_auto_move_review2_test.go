package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The automatic move after its second review (N1-N5): the update's last step
// has words of its own and is waited for; the mail says what the screen says;
// an unreadable record of the moves is one line, not silence; a refusal
// before anything changed is not audited as a move each time; "outside again"
// only where the site is known to have been inside.

func autoMoveAudits(t *testing.T, x *autoMoveFake) int {
	t.Helper()
	var n int
	if err := x.f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'site.limits_move' AND details LIKE '%"automatic":true%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestTheUpdatesLastStepIsWaitedForWithItsOwnWords (N1, N4): the Agent
// refuses with update_finishing (only the update's scheduler marker is left):
// the worker sends nothing of the programs, marks nothing, says "finishing"
// on the calm line and tries the site again later; once the step is done the
// site is moved once. The refusals are not audited (N4); the real move is.
// After six hours of it the site is marked failed with that reason, as a wait
// (not carried to a later version), and its line names that reason.
func TestTheUpdatesLastStepIsWaitedForWithItsOwnWords(t *testing.T) {
	if !siteAutoClearsByItself("update_finishing") || !siteMoveClaimRefusal("update_finishing") || siteAutoWaitingOf("update_finishing") != "finishing" {
		t.Fatal("update_finishing is not a refusal that clears by itself")
	}
	if siteAutoWaitingOf("build") != "build" || siteAutoWaitingOf("update_running") != "update" || siteAutoWaitingOf("update_unreadable") != "update" {
		t.Fatal("the other update refusals changed their words")
	}
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	refusals := 0
	x.onSleep = func(d time.Duration) {
		if d == siteAutoMoveFirstWait || refusals == 0 || refusals > 3 {
			return
		}
		if strings.Contains(strings.Join(x.calls, ","), "programs") || len(x.marks()) != 0 {
			t.Fatalf("refused, yet something was sent or marked: %v %v", x.calls, x.marks())
		}
		if p := siteAutoMoveProgressNow(); p == nil || p.Waiting != "finishing" {
			t.Fatalf("the calm line while the update's last step is pending: %+v", p)
		}
	}
	x.tasksAnswer = func(int) (transport.SiteTasksMoveResponse, error) {
		if refusals < 3 {
			refusals++
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: "update_finishing", Error: "the panel's update is installed, but its last step has not finished; nothing was moved"}, nil
		}
		refusals = 4
		return transport.SiteTasksMoveResponse{}, nil
	}
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,tasks:41,tasks:41,tasks:41,programs:41" {
		t.Fatalf("calls %s", got)
	}
	if m := x.marks()["41"]; m.State != siteAutoMoved {
		t.Fatalf("mark %+v", m)
	}
	if n := autoMoveAudits(t, x); n != 1 {
		t.Fatalf("%d audit rows for three refusals and one move", n)
	}

	// Six hours of it: failed with that reason, as a wait.
	y := newAutoMoveFake(t)
	y.site(42, 9902, true)
	y.tasksAnswer = func(int) (transport.SiteTasksMoveResponse, error) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: "update_finishing"}, nil
	}
	y.run()
	m := y.marks()["42"]
	if m.State != siteAutoFailed || m.Reason != "update_finishing" || !m.Waited || siteAutoMarkCarries(m) {
		t.Fatalf("the give-up: %+v", m)
	}
	if strings.Contains(strings.Join(y.calls, ","), "programs") || autoMoveAudits(t, y) != 0 {
		t.Fatalf("calls %v, %d audit rows", y.calls, autoMoveAudits(t, y))
	}
	if lines := y.lines(); len(lines) != 1 || lines[0].Reason != "update_finishing" || lines[0].Name != siteAutoFailed {
		t.Fatalf("lines %+v", lines)
	}
}

// TestARealAttemptStaysAudited (N4): an attempt that changed something is
// audited each time: the tasks moved before the programs' part was refused
// (a wait), and a refusal for a kept record (settling it may restart).
func TestARealAttemptStaysAudited(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	refused := 0
	x.programsAnswer = func(int) (transport.MoveSiteSliceResponse, error) {
		if refused == 0 {
			refused++
			return transport.MoveSiteSliceResponse{Error: "build", Reason: &transport.SiteMoveReason{Code: "build"}}, nil
		}
		return transport.MoveSiteSliceResponse{}, nil
	}
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41,tasks:41,programs:41" {
		t.Fatalf("calls %s", got)
	}
	if n := autoMoveAudits(t, x); n != 2 {
		t.Fatalf("%d audit rows: the wait after the tasks moved is a real attempt", n)
	}
}

// TestAnUnreadableRecordIsOneLine (N3): while the panel's own record of the
// moves, or of the sites put back on purpose, cannot be read, nothing is
// moved and Problems now has ONE line for the administrator (the panel's
// record, the panel's journal), instead of nothing; with no site outside
// there is no line; once the record reads, the sites' own lines come back.
func TestAnUnreadableRecordIsOneLine(t *testing.T) {
	for _, setting := range []string{settingSiteAutoMove, settingSiteTasksPutBack} {
		t.Run(setting, func(t *testing.T) {
			x := newAutoMoveFake(t)
			x.site(41, 9901, true)
			x.site(42, 9902, true)
			ctx := context.Background()
			if err := x.f.panel.setSetting(ctx, setting, "{not json"); err != nil {
				t.Fatal(err)
			}
			x.run()
			if len(x.calls) != 0 {
				t.Fatalf("moved without a readable record: %v", x.calls)
			}
			record := func() []monitoringProblem {
				siteLimitProblemReading = siteLimitReading{at: time.Now()}
				var out []monitoringProblem
				for _, p := range x.f.panel.siteLimitProblems(ctx, time.Now()) {
					if strings.HasPrefix(p.Kind, "site_auto_move") {
						out = append(out, p)
					}
				}
				return out
			}
			lines := record()
			if len(lines) != 1 || lines[0].Kind != "site_auto_move_unread" || lines[0].Reason != "record" || lines[0].Name != "record" || lines[0].Actor != "administrator" {
				t.Fatalf("lines %+v", lines)
			}
			en, tr := alertLine(pendingAlertMail{kind: lines[0].Kind, name: lines[0].Name})
			if !strings.Contains(en, "own record") || !strings.Contains(en, "sudo journalctl -u celikpanel-panel") || !strings.Contains(tr, "sudo journalctl -u celikpanel-panel") || strings.Contains(en, "celikpanel-agent") {
				t.Fatalf("mail %q / %q", en, tr)
			}
			// Readable again: each site outside has its own line.
			if err := x.f.panel.setSetting(ctx, setting, ""); err != nil {
				t.Fatal(err)
			}
			if lines := record(); len(lines) != 2 || lines[0].Kind != "site_auto_move_failed" {
				t.Fatalf("after the record reads: %+v", lines)
			}
			// No site outside: no line, even with the record unreadable.
			_ = x.f.panel.setSetting(ctx, setting, "{not json")
			x.inside[41], x.inside[42], x.tasksMoved[41], x.tasksMoved[42] = true, true, true, true
			if !x.f.panel.readAllSiteSlices(ctx) {
				t.Fatal("reading")
			}
			if lines := record(); len(lines) != 0 {
				t.Fatalf("no site outside: %+v", lines)
			}
		})
	}
}

// TestTheMailSaysWhatTheScreenSays (N2): the alert of the sites that could
// not be read chooses its words by the reason, as Problems now does.
func TestTheMailSaysWhatTheScreenSays(t *testing.T) {
	agentEN, agentTR := alertLine(pendingAlertMail{kind: "site_auto_move_unread", name: "agent"})
	if !strings.Contains(agentEN, "systemctl status celikpanel-agent") || !strings.Contains(agentTR, "systemctl status celikpanel-agent") {
		t.Fatalf("agent: %q / %q", agentEN, agentTR)
	}
	sitesEN, sitesTR := alertLine(pendingAlertMail{kind: "site_auto_move_unread", name: "sites"})
	if !strings.Contains(sitesEN, "own list of sites") || !strings.Contains(sitesEN, "journalctl -u celikpanel-panel") || strings.Contains(sitesEN, "celikpanel-agent") ||
		!strings.Contains(sitesTR, "kendi site listesini") || !strings.Contains(sitesTR, "journalctl -u celikpanel-panel") || strings.Contains(sitesTR, "celikpanel-agent") {
		t.Fatalf("sites: %q / %q", sitesEN, sitesTR)
	}
}

// TestOutsideAgainOnlyWhereKnown (N5): "it was inside its limits and runs
// outside them again" only for a site that a move really brought inside; a
// "moved" mark of a press that took nothing, or one settled by reading after
// a restart, is shown with the neutral words.
func TestOutsideAgainOnlyWhereKnown(t *testing.T) {
	already := &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
	moved := &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}
	for i, c := range []struct {
		resp   transport.MoveSiteSliceResponse
		tasks  *transport.SiteTasksMoveResponse
		reason string
	}{
		{transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{{Outcome: transport.SiteMoveMoved}}}, already, siteAutoBroughtIn},
		{transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{{Outcome: transport.SiteMoveRewritten}, {Outcome: transport.SiteMoveAlready}}}, already, siteAutoBroughtIn},
		{transport.MoveSiteSliceResponse{Done: true}, moved, siteAutoBroughtIn},
		// Nothing was taken.
		{transport.MoveSiteSliceResponse{Done: true}, already, ""},
		{transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{{Outcome: transport.SiteMoveAlready}}}, already, ""},
		// Something stayed outside (the owner's unit).
		{transport.MoveSiteSliceResponse{Done: true, Units: []transport.SiteSliceMoved{{Outcome: transport.SiteMoveMoved}, {Outcome: transport.SiteMoveKept}}}, already, ""},
	} {
		if m := classifySiteMove(c.resp, c.tasks, siteMovePressRan); m.State != siteAutoMoved || m.Reason != c.reason {
			t.Errorf("%d: %+v, want reason %q", i, m, c.reason)
		}
	}
	for _, c := range []struct {
		mark   siteAutoMark
		reason string
	}{
		{siteAutoMark{State: siteAutoMoved, Reason: siteAutoBroughtIn}, "outside_again"},
		{siteAutoMark{State: siteAutoMoved}, ""},
		{siteAutoMark{State: siteAutoMoved, Reason: "panel_restarted"}, ""},
	} {
		shown, line, card := siteAutoOutsideLine(c.mark, true, true)
		if shown.State != "outside" || shown.Reason != c.reason || !line || !card {
			t.Errorf("%+v: %+v %v %v", c.mark, shown, line, card)
		}
	}
	// Settled by reading after a restart: inside then, outside now: neutral.
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.inside[41], x.tasksMoved[41] = true, true
	ctx := context.Background()
	_ = x.f.panel.setSiteAutoMark(ctx, 41, siteAutoMark{State: siteAutoStarted, At: "2026-11-28T09:00:00Z", RequestID: "auto-1"})
	x.run()
	if m := x.marks()["41"]; m.State != siteAutoMoved || m.Reason != "panel_restarted" {
		t.Fatalf("settled %+v", m)
	}
	x.inside[41] = false
	if !x.f.panel.readAllSiteSlices(ctx) {
		t.Fatal("reading")
	}
	if lines := x.lines(); len(lines) != 1 || lines[0].Name != "outside" || lines[0].Reason != "" {
		t.Fatalf("lines %+v", lines)
	}
}
