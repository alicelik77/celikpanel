package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The fix round after the independent review of the automatic move
// (alpha.150, D-048): G2 no site stays outside without a line, G3 the panel's
// update stays possible during a long run, G4 a failing site is not restarted
// at every update, G5 the words say only what is known.

// TestTheWorkerHoldsForAnUpdateTheAdministratorAskedFor (G3): an update
// refused because the worker is moving a site is answered with words that
// say when that move ends, and records "an update is wanted"; the worker then
// starts no further site for 30 minutes, or until an update is admitted (then
// the update's own gate holds it). The move's claim is released before the
// reading that follows the move.
func TestTheWorkerHoldsForAnUpdateTheAdministratorAskedFor(t *testing.T) {
	t.Run("thirty_minutes", func(t *testing.T) {
		x := newAutoMoveFake(t)
		x.site(41, 9901, true)
		x.site(42, 9902, true)
		x.tasksMoved[41], x.tasksMoved[42] = true, true
		var wantedAt, movedAt time.Time
		x.programsAnswer = func(id int) (transport.MoveSiteSliceResponse, error) {
			switch id {
			case 41:
				// The administrator presses the update while 41 moves.
				if !siteMoveBusyByWorker() {
					t.Fatal("the worker's move is not known as the worker's")
				}
				wantedAt = x.clock
				noteSiteAutoUpdateWanted()
			case 42:
				movedAt = x.clock
			}
			return transport.MoveSiteSliceResponse{}, nil
		}
		read := readSiteSlicesCall
		readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
			if len(req.Sites) == 1 && len(x.calls) > 0 && siteMoveBusy() {
				t.Error("the move's claim was still held at the reading after the move")
			}
			return read(p, ctx, req)
		}
		held := 0
		x.onSleep = func(time.Duration) {
			if p := siteAutoMoveProgressNow(); p != nil && p.Waiting == "wanted" {
				held++
			}
		}
		x.run()
		if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41,tasks:42,programs:42" {
			t.Fatalf("calls %s", got)
		}
		if movedAt.Sub(wantedAt) < siteAutoMoveUpdateHold || held == 0 {
			t.Fatalf("the next site moved %v after the update was asked for (held %d times)", movedAt.Sub(wantedAt), held)
		}
	})
	t.Run("until_admitted", func(t *testing.T) {
		x := newAutoMoveFake(t)
		x.site(41, 9901, true)
		x.site(42, 9902, true)
		x.tasksMoved[41], x.tasksMoved[42] = true, true
		var wantedAt, movedAt time.Time
		x.programsAnswer = func(id int) (transport.MoveSiteSliceResponse, error) {
			if id == 41 {
				wantedAt = x.clock
				noteSiteAutoUpdateWanted()
			} else {
				movedAt = x.clock
				if x.updating {
					t.Fatal("moved while the admitted update ran")
				}
			}
			return transport.MoveSiteSliceResponse{}, nil
		}
		updateSleeps := 0
		x.onSleep = func(time.Duration) {
			p := siteAutoMoveProgressNow()
			switch {
			case p != nil && p.Waiting == "wanted" && !x.updating && updateSleeps == 0:
				noteSiteAutoUpdateAdmitted() // the administrator's update is admitted
				x.updating = true
			case x.updating:
				if p == nil || p.Waiting != "update" {
					t.Fatalf("the calm line while the admitted update runs: %+v", p)
				}
				if updateSleeps++; updateSleeps == 3 {
					x.updating = false // it finished
				}
			}
		}
		x.run()
		if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41,tasks:42,programs:42" || updateSleeps != 3 {
			t.Fatalf("calls %s, %d waits for the update", got, updateSleeps)
		}
		if movedAt.Sub(wantedAt) >= siteAutoMoveUpdateHold {
			t.Fatalf("the hold did not end with the admission: %v", movedAt.Sub(wantedAt))
		}
	})
}

// TestTheUpdatesRefusalDuringTheWorkersMove (G3): the update's admission
// refused while the worker moves a site says when that move ends and records
// the wish; an administrator's own press refuses as before and records
// nothing; an admitted update clears the wish; a page read or a status poll
// never sets it.
func TestTheUpdatesRefusalDuringTheWorkersMove(t *testing.T) {
	withSystemUpdateBuild(t)
	fixture := newSystemUpdateTestFixture(t)
	t.Cleanup(func() {
		siteMoveMu.Lock()
		siteMoveRunning, siteMoveByWorker = false, false
		siteMoveMu.Unlock()
		siteAutoMove = siteAutoMover{}
	})
	wanted := func() bool {
		siteAutoMove.mu.Lock()
		defer siteAutoMove.mu.Unlock()
		return !siteAutoMove.wantedAt.IsZero()
	}
	press := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		fixture.panel.handlePanelUpdateStart(recorder, systemUpdateRequest(http.MethodPost, panelUpdateStartPath, systemUpdateStartBody(updateTestTargetVersion), roleAdmin))
		return recorder
	}
	moving := func(running, worker bool) {
		siteMoveMu.Lock()
		siteMoveRunning, siteMoveByWorker = running, worker
		siteMoveMu.Unlock()
	}
	// The administrator's own press runs: refused as before, nothing recorded.
	moving(true, false)
	if r := press(); r.Code != http.StatusConflict || !strings.Contains(r.Body.String(), "PANEL_UPDATE_SITE_MOVE_RUNNING") || wanted() {
		t.Fatalf("during a press: %d %s wanted=%v", r.Code, r.Body.String(), wanted())
	}
	// The worker moves a site: its own words, and the wish recorded.
	moving(true, true)
	r := press()
	if r.Code != http.StatusConflict || !strings.Contains(r.Body.String(), "PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING") || !strings.Contains(r.Body.String(), "within about 12 minutes") || !wanted() {
		t.Fatalf("during the worker's move: %d %s wanted=%v", r.Code, r.Body.String(), wanted())
	}
	if hostingMutationTimeout != 12*time.Minute {
		t.Fatalf("the words name 12 minutes, the move's bound is %v", hostingMutationTimeout)
	}
	fixture.agent.mu.Lock()
	starts := fixture.agent.startCalls
	fixture.agent.mu.Unlock()
	if starts != 0 {
		t.Fatalf("the update was started %d times during a move", starts)
	}
	// The move ended; the administrator presses again: admitted, the wish goes.
	moving(false, false)
	if r := press(); r.Code != http.StatusAccepted || wanted() {
		t.Fatalf("after the move: %d %s wanted=%v", r.Code, r.Body.String(), wanted())
	}
	// A status poll and a page read never record the wish, even while the worker moves.
	moving(true, true)
	poll := httptest.NewRecorder()
	fixture.panel.handlePanelUpdateStatus(poll, systemUpdateRequest(http.MethodGet, panelUpdateStatusPath+"?request_id="+updateTestRequestID, "", roleAdmin))
	_ = fixture.panel.collectMonitoringProblems(context.Background(), time.Now())
	if wanted() {
		t.Fatal("a status poll or a page read recorded an update wish")
	}
}

// TestAFailingSiteIsNotRestartedAtEveryUpdate (G4): the marks of a move that
// was attempted and did not bring the site inside are carried to the next
// version (nothing is sent for them there); "moved", "left" and a failure
// that only records waiting stay per version (the next version tries them);
// a record written before this round reads the same way.
func TestAFailingSiteIsNotRestartedAtEveryUpdate(t *testing.T) {
	for _, c := range []struct {
		name  string
		mark  siteAutoMark
		moves int // moves sent in the next version
	}{
		{"failed_put_back", siteAutoMark{State: siteAutoFailed, Reason: "not_listening", PutBack: true}, 0},
		{"failed_not_finished", siteAutoMark{State: siteAutoFailed, Reason: "prepare_failed"}, 0},
		{"time_up", siteAutoMark{State: siteAutoTimeUp, Reason: "time_up"}, 0},
		{"unknown", siteAutoMark{State: siteAutoUnknown, Reason: "agent_unreachable"}, 0},
		{"kept", siteAutoMark{State: siteAutoKept, Reason: "not_running"}, 0},
		{"started", siteAutoMark{State: siteAutoStarted}, 0},
		{"failed_waited", siteAutoMark{State: siteAutoFailed, Reason: "site_busy", Waited: true}, 1},
		{"failed_waited_before_this_round", siteAutoMark{State: siteAutoFailed, Reason: "host_busy"}, 1},
		{"failed_record", siteAutoMark{State: siteAutoFailed, Reason: "marks_unreadable", Waited: true}, 1},
		{"failed_limits_unread", siteAutoMark{State: siteAutoFailed, Reason: "limits_unread"}, 1},
		{"moved", siteAutoMark{State: siteAutoMoved}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			x := newAutoMoveFake(t)
			x.site(41, 9901, true)
			x.tasksMoved[41] = true
			c.mark.At = "2026-11-28T09:00:00Z"
			if err := x.f.panel.setSiteAutoMark(context.Background(), 41, c.mark); err != nil {
				t.Fatal(err)
			}
			siteAutoMoveVersion = func() string { return "v0.1.0-alpha.151+test" }
			x.run()
			if got := strings.Count(strings.Join(x.calls, ","), "programs:41"); got != c.moves {
				t.Fatalf("%d moves sent in the next version: %v", got, x.calls)
			}
			m := x.marks()["41"]
			if c.moves == 0 && m.State != c.mark.State && !(c.mark.State == siteAutoStarted && m.State == siteAutoUnknown && m.Reason == "unsettled") {
				t.Fatalf("the carried mark: %+v", m)
			}
			if c.moves == 1 && m.State != siteAutoMoved {
				t.Fatalf("after the next version's move: %+v", m)
			}
			// A carried mark keeps its line and Try again.
			if c.moves == 0 {
				if lines := x.lines(); len(lines) != 1 || lines[0].Action.Kind != "retry_move" {
					t.Fatalf("lines %+v", lines)
				}
			}
		})
	}
	// "left" is per version: the next version asks the tasks again.
	x := newAutoMoveFake(t)
	x.site(42, 9902, true)
	x.inside[42] = true
	_ = x.f.panel.setSiteAutoMark(context.Background(), 42, siteAutoMark{State: siteAutoLeft, Reason: transport.SiteTasksRefusalCronAllow, At: "2026-11-28T09:00:00Z"})
	siteAutoMoveVersion = func() string { return "v0.1.0-alpha.151+test" }
	x.run()
	if got := strings.Join(x.calls, ","); !strings.HasPrefix(got, "tasks:42") {
		t.Fatalf("left, next version: %s", got)
	}
	// A record as this code wrote it before the round reads both ways.
	raw := `{"version":"v0.1.0-alpha.150+old","sites":{"42":{"state":"time_up","reason":"time_up","at":"2026-11-28T09:00:00Z","request_id":"auto-1"},"43":{"state":"failed","reason":"site_busy","at":"2026-11-28T09:00:00Z"}}}`
	if err := x.f.panel.setSetting(context.Background(), settingSiteAutoMove, raw); err != nil {
		t.Fatal(err)
	}
	marks, err := x.f.panel.readSiteAutoMarks(context.Background())
	if err != nil || marks.Sites["42"].State != siteAutoTimeUp || marks.Sites["42"].RequestID != "auto-1" {
		t.Fatalf("an earlier record: %+v %v", marks, err)
	}
	if _, ok := marks.Sites["43"]; ok {
		t.Fatalf("a failure that only waited was carried: %+v", marks.Sites["43"])
	}
}

// TestNoSiteStaysOutsideWithoutALine (G2b): a site outside its limits while
// no run is scheduled, running or waiting has a line and Try again on
// Problems now and its card, whatever its mark ("moved": inside once,
// outside again; none; a "started" nobody settled); it is not moved again by
// itself in this version. A site left for the owner's reason and a site put
// back on purpose have no line; while a run is scheduled, a site it will
// take has none either.
func TestNoSiteStaysOutsideWithoutALine(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.site(42, 9902, true)
	x.site(43, 9903, true)
	x.inside[41], x.tasksMoved[41] = true, true
	x.inside[42], x.tasksMoved[42] = true, true
	x.inside[43], x.tasksMoved[43] = true, true
	x.run()
	if len(x.calls) != 0 || len(x.lines()) != 0 {
		t.Fatalf("calls %v lines %+v", x.calls, x.lines())
	}
	ctx := context.Background()
	// 41: the run moved it inside earlier; 42: no mark; 43: started, never settled.
	_ = x.f.panel.setSiteAutoMark(ctx, 41, siteAutoMark{State: siteAutoMoved, Reason: siteAutoBroughtIn, At: "2026-11-28T09:00:00Z"})
	_ = x.f.panel.setSiteAutoMark(ctx, 43, siteAutoMark{State: siteAutoStarted, At: "2026-11-28T09:00:00Z"})
	x.inside[41], x.inside[42], x.inside[43] = false, false, false
	if !x.f.panel.readAllSiteSlices(ctx) {
		t.Fatal("reading")
	}
	lines := x.lines()
	// 41 is reseller.matrix.test, 42 customer.matrix.test, 43 outsider.matrix.test.
	want := map[string][2]string{
		"reseller.matrix.test": {"outside", "outside_again"}, "customer.matrix.test": {"outside", ""}, "outsider.matrix.test": {siteAutoUnknown, "unsettled"},
	}
	got := map[string][2]string{}
	for _, l := range lines {
		got[l.Subject] = [2]string{l.Name, l.Reason}
		if l.Action.Kind != "retry_move" || l.DomainID == 0 || l.Actor != "administrator" {
			t.Fatalf("line %+v", l)
		}
	}
	if len(lines) != 3 || got["reseller.matrix.test"] != want["reseller.matrix.test"] || got["customer.matrix.test"] != want["customer.matrix.test"] || got["outsider.matrix.test"] != want["outsider.matrix.test"] {
		t.Fatalf("lines %+v", lines)
	}
	card := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9901/site-limits", "")
	var view siteLimitsView
	if err := json.Unmarshal(card.Body.Bytes(), &view); err != nil || view.AutoMove == nil || view.AutoMove.Offer != "retry" || view.AutoMove.State != "outside" || view.AutoMove.Reason != "outside_again" {
		t.Fatalf("card %s", card.Body.String())
	}
	// While a run is scheduled, the sites it will take have no line.
	siteAutoMove.mu.Lock()
	siteAutoMove.scheduled = true
	siteAutoMove.mu.Unlock()
	if lines := x.lines(); len(lines) != 0 {
		t.Fatalf("while a run is scheduled: %+v", lines)
	}
	siteAutoMove.mu.Lock()
	siteAutoMove.scheduled = false
	siteAutoMove.mu.Unlock()
	// Not moved again by itself in this version.
	x.calls = nil
	x.run()
	if strings.Contains(strings.Join(x.calls, ","), "programs:41") {
		t.Fatalf("a site outside again was moved by itself: %v", x.calls)
	}
	// The owner's reason and a put-back site have no line.
	_ = x.f.panel.setSiteAutoMark(ctx, 41, siteAutoMark{State: siteAutoLeft, Reason: transport.SiteTasksRefusalCronAllow, At: "2026-11-28T09:00:00Z"})
	x.f.panel.noteSitePutBack(ctx, 42, true)
	for _, l := range x.lines() {
		if l.Subject == "reseller.matrix.test" || l.Subject == "customer.matrix.test" {
			t.Fatalf("a line for the owner's reason or a site put back: %+v", l)
		}
	}
}

// TestTheReadingThatFailsDoesNotGiveUp (G2a): a run whose reading fails
// keeps reading (never closer than ten minutes apart); after an hour without
// a reading Problems now has ONE line that says why and who acts; it goes by
// itself once a reading succeeds, and the run then moves the site.
func TestTheReadingThatFailsDoesNotGiveUp(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	read := readSiteSlicesCall
	fails := 8
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		if fails > 0 {
			fails--
			return transport.SiteSlicesStatusResponse{}, errors.New("the agent did not answer")
		}
		return read(p, ctx, req)
	}
	started := x.clock
	var seenLine, lineBeforeHour bool
	x.onSleep = func(d time.Duration) {
		if d == siteAutoMoveFirstWait {
			return
		}
		if d < siteAutoMoveRetryMax {
			t.Fatalf("read again after %v", d)
		}
		lines := siteAutoMoveUnreadProblem(x.clock)
		failingFor := x.clock.Sub(started.Add(siteAutoMoveFirstWait))
		switch {
		case len(lines) == 1 && failingFor < siteAutoMoveUnreadLine:
			lineBeforeHour = true
		case len(lines) == 1:
			seenLine = true
			l := lines[0]
			if l.Kind != "site_auto_move_unread" || l.Reason != "agent" || l.Actor != "administrator" {
				t.Fatalf("line %+v", l)
			}
			// The line is part of Problems now.
			found := false
			for _, p := range x.f.panel.siteLimitProblems(context.Background(), x.clock) {
				found = found || p.Kind == "site_auto_move_unread"
			}
			if !found {
				t.Fatal("the line is not in Problems now")
			}
		}
	}
	x.run()
	if !seenLine || lineBeforeHour {
		t.Fatalf("line after the hour %v, before it %v", seenLine, lineBeforeHour)
	}
	if len(siteAutoMoveUnreadProblem(x.clock)) != 0 {
		t.Fatal("the line stays after a reading succeeded")
	}
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41" {
		t.Fatalf("calls %s", got)
	}
}

// TestTheEndingSaysWhatIsKnown (G5): the mark carries which part ended the
// move and whether that part put the site back, so that "put back as it
// was" is said only where it was; a refusal for the site's own kept record
// gives up after the same six hours, ending in a kept mark, never in a
// second move.
func TestTheEndingSaysWhatIsKnown(t *testing.T) {
	moved := &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}
	already := &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
	for i, c := range []struct {
		resp    transport.MoveSiteSliceResponse
		tasks   *transport.SiteTasksMoveResponse
		part    string
		putBack bool
	}{
		{transport.MoveSiteSliceResponse{RolledBack: true, Reason: &transport.SiteMoveReason{Code: "not_listening"}}, already, "", true},
		{transport.MoveSiteSliceResponse{RolledBack: true, Reason: &transport.SiteMoveReason{Code: "not_listening"}}, moved, "tasks_moved", true},
		{transport.MoveSiteSliceResponse{Reason: &transport.SiteMoveReason{Code: "prepare_failed"}}, already, "", false},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused}, "tasks", false},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksPutBack}, "tasks", true},
	} {
		if m := classifySiteMove(c.resp, c.tasks, siteMovePressRan); m.Part != c.part || m.PutBack != c.putBack || m.State != siteAutoFailed {
			t.Errorf("%d: %+v", i, m)
		}
	}
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.tasksMoved[41] = true
	x.programsAnswer = func(int) (transport.MoveSiteSliceResponse, error) {
		return transport.MoveSiteSliceResponse{Kept: true, Error: "an earlier move of a site could not be put back completely", Reason: &transport.SiteMoveReason{Code: transport.SiteMoveRefusalKept}}, nil
	}
	hidden := true
	x.onSleep = func(d time.Duration) {
		if d != siteAutoMoveFirstWait && siteAutoMoveProgressNow() != nil {
			hidden = false
		}
	}
	x.run()
	m := x.marks()["41"]
	if m.State != siteAutoKept || m.Reason != transport.SiteMoveRefusalKept || !hidden {
		t.Fatalf("the give-up of the own kept record: %+v (calm line hidden %v)", m, hidden)
	}
	attempts := strings.Count(strings.Join(x.calls, ","), "programs:41")
	x.calls = nil
	x.run()
	if len(x.calls) != 0 || attempts < 2 {
		t.Fatalf("after the give-up: %v (%d attempts before)", x.calls, attempts)
	}
	if x.clock.Sub(time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)) < siteAutoMoveGiveUp {
		t.Fatal("gave up before six hours")
	}
}
