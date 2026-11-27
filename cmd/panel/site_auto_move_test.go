package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The panel's own worker that moves every site into its limits (alpha.150,
// D-048; site_auto_move.go). The Agent is a fake that keeps where each site's
// programs run, whether its tasks run as timers and whether a move record is
// kept; it answers the same calls the press makes.

type autoMoveFake struct {
	t *testing.T
	f rightsFixture
	// programs inside their site's slice; tasks moved into timers; a kept
	// move record per site.
	inside, tasksMoved, kept map[int]bool
	calls                    []string
	// The answers of the two moves (nil: done, and the site goes inside).
	tasksAnswer    func(siteID int) (transport.SiteTasksMoveResponse, error)
	programsAnswer func(siteID int) (transport.MoveSiteSliceResponse, error)
	// onSleep runs at every pause of the worker (the fake clock moves on).
	onSleep func(d time.Duration)
	clock   time.Time
	sleeps  []time.Duration
	// gates
	updating, busy bool
}

func newAutoMoveFake(t *testing.T) *autoMoveFake {
	t.Helper()
	useHostMemory(t, 8192)
	f := newRightsFixture(t)
	x := &autoMoveFake{t: t, f: f, inside: map[int]bool{}, tasksMoved: map[int]bool{}, kept: map[int]bool{},
		clock: time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)}
	oldRead, oldMoved, oldTasks, oldSlice := readSiteSlicesCall, siteTasksMovedCall, moveSiteTasksCall, moveSiteSliceCall
	oldUpdate, oldQuiet, oldSleep, oldNow, oldStart, oldNeeds := siteMoveUpdateInProgress, backgroundHostQuiet, siteAutoMoveSleep, siteAutoMoveNow, startSiteAutoMoveRun, siteAutoMoveNeedsAgent
	oldVersion, oldPressNow := siteAutoMoveVersion, siteMovePressNow
	t.Cleanup(func() {
		readSiteSlicesCall, siteTasksMovedCall, moveSiteTasksCall, moveSiteSliceCall = oldRead, oldMoved, oldTasks, oldSlice
		siteMoveUpdateInProgress, backgroundHostQuiet, siteAutoMoveSleep, siteAutoMoveNow, startSiteAutoMoveRun, siteAutoMoveNeedsAgent = oldUpdate, oldQuiet, oldSleep, oldNow, oldStart, oldNeeds
		siteAutoMoveVersion, siteMovePressNow = oldVersion, oldPressNow
		siteAutoMove = siteAutoMover{}
		siteMoveMu.Lock()
		siteMoveRunning = false
		siteMoveMu.Unlock()
	})
	siteAutoMove = siteAutoMover{}
	siteAutoMoveVersion = func() string { return "v0.1.0-alpha.150+test" }
	siteAutoMoveNeedsAgent = false
	startSiteAutoMoveRun = func(run func()) { run() }
	siteAutoMoveNow = func() time.Time { return x.clock }
	siteMovePressNow = func() time.Time { return x.clock }
	siteAutoMoveSleep = func(d time.Duration) {
		x.sleeps = append(x.sleeps, d)
		x.clock = x.clock.Add(d)
		if x.onSleep != nil {
			x.onSleep(d)
		}
	}
	siteMoveUpdateInProgress = func(*Panel, context.Context) bool { return x.updating }
	backgroundHostQuiet = func(*Panel, context.Context) bool { return !x.busy }
	readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
		resp := transport.SiteSlicesStatusResponse{}
		for _, q := range req.Sites {
			slice := "celiksite_" + strconv.Itoa(q.SiteID) + ".slice"
			status := transport.SiteSliceStatus{SiteID: q.SiteID, Slice: slice, File: transport.SiteFileManagedUnchanged}
			for _, u := range q.Units {
				if x.inside[q.SiteID] {
					status.Units = append(status.Units, transport.SiteSliceUnit{Unit: u, Placement: transport.SiteUnitInSlice, Slice: slice})
				} else {
					status.Units = append(status.Units, transport.SiteSliceUnit{Unit: u, Placement: transport.SiteUnitOutside, Slice: "system.slice"})
				}
			}
			if x.kept[q.SiteID] {
				status.MoveInterrupted = &transport.SiteSliceMoveRecovery{At: "2026-11-28T09:00:00Z", Units: map[string]string{}, Kept: true}
			}
			resp.Sites = append(resp.Sites, status)
		}
		return resp, nil
	}
	siteTasksMovedCall = func(p *Panel, ctx context.Context, req *transport.SiteTasksMovedRequest) (transport.SiteTasksMovedResponse, error) {
		resp := transport.SiteTasksMovedResponse{Moved: []int{}}
		for id, moved := range x.tasksMoved {
			if moved {
				resp.Moved = append(resp.Moved, id)
			}
		}
		return resp, nil
	}
	moveSiteTasksCall = func(p *Panel, ctx context.Context, method string, req *transport.SiteTasksMoveRequest) (transport.SiteTasksMoveResponse, error) {
		if method != "Agent.MoveSiteTasksV1" || req.Preview {
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUndone}, nil
		}
		x.calls = append(x.calls, "tasks:"+strconv.Itoa(req.SiteID))
		if !strings.HasPrefix(req.RequestID, "auto-") && !strings.HasPrefix(req.RequestID, "press-") {
			t.Errorf("request id %q", req.RequestID)
		}
		if x.tasksAnswer != nil {
			if resp, err := x.tasksAnswer(req.SiteID); err != nil || resp.Outcome != "" {
				if resp.Outcome == transport.SiteTasksMoved {
					x.tasksMoved[req.SiteID] = true
				}
				return resp, err
			}
		}
		if x.tasksMoved[req.SiteID] {
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}, nil
		}
		x.tasksMoved[req.SiteID] = true
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}, nil
	}
	moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
		if req.Recover {
			return transport.MoveSiteSliceResponse{Done: true}, nil
		}
		x.calls = append(x.calls, "programs:"+strconv.Itoa(req.Site.SiteID))
		if x.programsAnswer != nil {
			if resp, err := x.programsAnswer(req.Site.SiteID); err != nil || resp.Done || resp.Error != "" || resp.Kept {
				if resp.Done {
					x.inside[req.Site.SiteID] = true
				}
				return resp, err
			}
		}
		x.inside[req.Site.SiteID] = true
		return transport.MoveSiteSliceResponse{Done: true}, nil
	}
	return x
}

// site adds a site of a domain (node: it runs a program).
func (x *autoMoveFake) site(id, domainID int, node bool) {
	kind := "php"
	if node {
		kind = "node"
	}
	x.f.exec(x.t, `INSERT INTO sites (id, domain_id, document_root, project_type) VALUES (`+strconv.Itoa(id)+`, `+strconv.Itoa(domainID)+`, '/srv/s`+strconv.Itoa(id)+`', '`+kind+`')`)
}

func (x *autoMoveFake) run() {
	x.t.Helper()
	x.f.panel.scheduleSiteAutoMove(siteAutoMoveFirstWait)
}

func (x *autoMoveFake) marks() map[string]siteAutoMark {
	x.t.Helper()
	marks, err := x.f.panel.readSiteAutoMarks(context.Background())
	if err != nil {
		x.t.Fatal(err)
	}
	return marks.Sites
}

func (x *autoMoveFake) lines() []monitoringProblem {
	siteLimitProblemReading = siteLimitReading{at: time.Now()} // no background reading
	var out []monitoringProblem
	for _, p := range x.f.panel.siteLimitProblems(context.Background(), time.Now()) {
		if p.Kind == "site_auto_move_failed" {
			out = append(out, p)
		}
	}
	return out
}

// TestTheWorkerMovesEverySiteOnceInOrder: every site outside its limits is
// moved by itself, one at a time in the order of the sites' ids, the
// scheduled tasks first, with a pause between two sites and the run's
// progress for the calm line; a site already inside is not moved; each site
// is moved once per installed version (a second run moves nothing), and the
// next version tries again only a site that is still outside.
func TestTheWorkerMovesEverySiteOnceInOrder(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(43, 9903, true)
	x.site(41, 9901, true)
	x.site(42, 9902, false)
	x.inside[43], x.tasksMoved[43] = false, false
	var progress []string
	x.onSleep = func(time.Duration) {
		if p := siteAutoMoveProgressNow(); p != nil {
			progress = append(progress, strconv.Itoa(p.Done)+"/"+strconv.Itoa(p.Total))
		}
	}
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41,tasks:42,tasks:43,programs:43" {
		t.Fatalf("calls %s", got)
	}
	if x.sleeps[0] != siteAutoMoveFirstWait {
		t.Fatalf("the run did not wait after the panel's start: %v", x.sleeps)
	}
	pauses := 0
	for _, d := range x.sleeps {
		if d == siteAutoMovePause {
			pauses++
		}
	}
	if pauses != 2 || strings.Join(progress, ",") != "1/3,2/3" {
		t.Fatalf("pauses %d progress %v", pauses, progress)
	}
	if siteAutoMoveProgressNow() != nil {
		t.Fatal("the calm line stays after the run")
	}
	for _, id := range []string{"41", "42", "43"} {
		if m := x.marks()[id]; m.State != siteAutoMoved || !strings.HasPrefix(m.RequestID, "auto-") {
			t.Fatalf("site %s: %+v", id, m)
		}
	}
	var audited int
	_ = x.f.panel.db.GetDB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = 'site.limits_move' AND details LIKE '%"automatic":true%'`).Scan(&audited)
	if audited != 3 {
		t.Fatalf("%d audit rows", audited)
	}
	// Once per version: the server restarts and a program is outside again
	// (a reboot before the move existed); the same version does not move it.
	x.calls, x.inside[41] = nil, false
	x.run()
	if len(x.calls) != 0 {
		t.Fatalf("a second run moved again: %v", x.calls)
	}
	// The next version tries the site still outside, and only that one.
	siteAutoMoveVersion = func() string { return "v0.1.0-alpha.151+test" }
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41" {
		t.Fatalf("next version: %s", got)
	}
}

// TestTheWorkerStartsOnlyAfterTheGate: while the panel's update is admitted
// or running, or the server is busy with another change, or the Agent says
// the update is in progress, nothing is sent and no site is marked; the run
// comes back to the site by itself and moves it once the gate allows. This
// is the proof that the worker never starts anything before the update is
// complete and verified (the Agent's own claim refuses until then too).
func TestTheWorkerStartsOnlyAfterTheGate(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.updating = true
	refused := 0
	x.onSleep = func(d time.Duration) {
		if (x.updating || x.busy) && (len(x.calls) != 0 || len(x.marks()) != 0) {
			t.Fatalf("something was sent or marked behind the gate: %v %v", x.calls, x.marks())
		}
		switch {
		case x.updating && len(x.sleeps) > 3:
			x.updating, x.busy = false, true
		case x.busy && len(x.sleeps) > 6:
			x.busy = false
		}
	}
	x.tasksAnswer = func(int) (transport.SiteTasksMoveResponse, error) {
		if refused < 2 {
			refused++
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: "update_running", Error: "the panel's update is in progress"}, nil
		}
		return transport.SiteTasksMoveResponse{}, nil
	}
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,tasks:41,tasks:41,programs:41" || len(x.sleeps) < 8 {
		t.Fatalf("calls %s after %d pauses", got, len(x.sleeps))
	}
	if m := x.marks()["41"]; m.State != siteAutoMoved {
		t.Fatalf("mark %+v", m)
	}
	if len(x.lines()) != 0 {
		t.Fatal("a refusal that clears by itself made a line")
	}
}

// TestARealFailureIsNotTriedAgain: a move put back, stopped for its time or
// whose end is not known is marked and not tried again by the worker (the
// run goes on with the next site); one Problems now line per such site says
// why, the administrator acts, Try again (its domain); Try again's success
// takes the line away. A refusal that clears by itself (the site being
// changed) is tried again later in the same run.
func TestARealFailureIsNotTriedAgain(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.site(42, 9902, true)
	x.site(43, 9903, true)
	x.tasksMoved[41], x.tasksMoved[42], x.tasksMoved[43] = true, true, true
	busy := 0
	x.programsAnswer = func(id int) (transport.MoveSiteSliceResponse, error) {
		switch id {
		case 41:
			return transport.MoveSiteSliceResponse{RolledBack: true, Error: "celikapp-41.service could not be moved", Reason: &transport.SiteMoveReason{Code: "not_listening"},
				Units: []transport.SiteSliceMoved{{Unit: "celikapp-41.service", Outcome: transport.SiteMoveFailed, PutBack: true}}}, nil
		case 42:
			if busy == 0 {
				busy++
				return transport.MoveSiteSliceResponse{Error: "the site is being changed", Reason: &transport.SiteMoveReason{Code: "site_busy"}}, nil
			}
		case 43:
			return transport.MoveSiteSliceResponse{TimeUp: true, Error: "the move's time ran out", Reason: &transport.SiteMoveReason{Code: "time_up"}}, nil
		}
		return transport.MoveSiteSliceResponse{}, nil
	}
	x.run()
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41,tasks:42,programs:42,tasks:43,programs:43,tasks:42,programs:42" {
		t.Fatalf("calls %s", got)
	}
	marks := x.marks()
	if marks["41"].State != siteAutoFailed || marks["41"].Reason != "not_listening" || marks["42"].State != siteAutoMoved || marks["43"].State != siteAutoTimeUp {
		t.Fatalf("marks %+v", marks)
	}
	lines := x.lines()
	if len(lines) != 2 || lines[0].Subject != "outsider.matrix.test" || lines[1].Subject != "reseller.matrix.test" || lines[1].Reason != "not_listening" {
		t.Fatalf("lines %+v", lines)
	}
	for _, line := range lines {
		if line.Actor != "administrator" || line.Action.Kind != "retry_move" || line.DomainID == 0 || line.Severity != "warning" {
			t.Fatalf("line %+v", line)
		}
	}
	// Not tried again by the worker.
	x.calls = nil
	x.run()
	if len(x.calls) != 0 {
		t.Fatalf("a failure was tried again: %v", x.calls)
	}
	// The administrator's Try again (the same move call) brings 41 inside:
	// its line goes.
	x.programsAnswer = nil
	move := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9901/site-limits/move", `{"request_id":"press-0001"}`)
	wantStatus(t, move, http.StatusOK, "")
	if m := x.marks()["41"]; m.State != siteAutoMoved || m.RequestID != "press-0001" {
		t.Fatalf("after Try again: %+v", m)
	}
	if lines := x.lines(); len(lines) != 1 || lines[0].Subject != "outsider.matrix.test" || lines[0].Name != siteAutoTimeUp {
		t.Fatalf("after Try again: %+v", lines)
	}
}

// TestAKeptRecordStopsTheRun: a move that leaves its record kept stops the
// run (the next site is not moved; no calm line while it waits); once a
// reading finds the record settled the run goes on by itself. A refusal
// because of a record kept from before is not a failure of that site: it is
// moved after the record is settled.
func TestAKeptRecordStopsTheRun(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.site(42, 9902, true)
	x.tasksMoved[41], x.tasksMoved[42] = true, true
	x.programsAnswer = func(id int) (transport.MoveSiteSliceResponse, error) {
		if id == 41 {
			x.kept[41] = true
			return transport.MoveSiteSliceResponse{RolledBack: true, Kept: true, Error: "putting back did not complete", Reason: &transport.SiteMoveReason{Code: "not_running"}}, nil
		}
		return transport.MoveSiteSliceResponse{}, nil
	}
	waits := 0
	x.onSleep = func(d time.Duration) {
		if d != siteAutoMoveKeptEvery {
			return
		}
		waits++
		if strings.Contains(strings.Join(x.calls, ","), "42") {
			t.Fatal("the next site was moved while the record was kept")
		}
		if siteAutoMoveProgressNow() != nil {
			t.Fatal("the calm line shows while a kept record waits for the administrator")
		}
		if waits == 3 {
			x.kept[41] = false // the administrator pressed Try again
		}
	}
	x.run()
	if waits != 3 || strings.Join(x.calls, ",") != "tasks:41,programs:41,tasks:42,programs:42" {
		t.Fatalf("waits %d calls %v", waits, x.calls)
	}
	marks := x.marks()
	if marks["41"].State != siteAutoKept || marks["42"].State != siteAutoMoved {
		t.Fatalf("marks %+v", marks)
	}
	// A record kept from before: the site refused with kept_earlier is not
	// marked; it is moved once the record is settled.
	x2 := newAutoMoveFake(t)
	x2.site(41, 9901, true)
	x2.tasksMoved[41] = true
	first := true
	x2.programsAnswer = func(int) (transport.MoveSiteSliceResponse, error) {
		if first {
			first = false
			x2.kept[41] = true
			return transport.MoveSiteSliceResponse{Kept: true, Error: "an earlier move of a site could not be put back completely", Reason: &transport.SiteMoveReason{Code: transport.SiteMoveRefusalKept}}, nil
		}
		return transport.MoveSiteSliceResponse{}, nil
	}
	x2.onSleep = func(d time.Duration) {
		if d == siteAutoMoveKeptEvery {
			x2.kept[41] = false
		}
	}
	x2.run()
	if m := x2.marks()["41"]; m.State != siteAutoMoved || strings.Count(strings.Join(x2.calls, ","), "programs:41") != 2 {
		t.Fatalf("mark %+v calls %v", m, x2.calls)
	}
}

// TestARestartInTheMiddleNeverMovesTwice: a site whose mark says the move
// was sent but not how it ended (the panel stopped in the middle) is settled
// by reading only: inside, it is moved; outside, its end is not known and a
// line offers Try again. The move is never sent a second time; the Agent's
// own claim refuses a move while the first still runs.
func TestARestartInTheMiddleNeverMovesTwice(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.site(42, 9902, true)
	x.inside[41], x.tasksMoved[41] = true, true
	for _, id := range []int{41, 42} {
		if err := x.f.panel.setSiteAutoMark(context.Background(), id, siteAutoMark{State: siteAutoStarted, At: "2026-11-28T09:00:00Z", RequestID: "auto-earlier"}); err != nil {
			t.Fatal(err)
		}
	}
	x.run()
	if len(x.calls) != 0 {
		t.Fatalf("a site was moved again: %v", x.calls)
	}
	marks := x.marks()
	if marks["41"].State != siteAutoMoved || marks["42"].State != siteAutoUnknown || marks["42"].Reason != "panel_restarted" {
		t.Fatalf("marks %+v", marks)
	}
	if lines := x.lines(); len(lines) != 1 || lines[0].Subject != "customer.matrix.test" || lines[0].Name != siteAutoUnknown {
		t.Fatalf("lines %+v", lines)
	}
	// The Agent still moving the first site (a restarted panel's first move):
	// another_move is a wait, never a failure.
	x3 := newAutoMoveFake(t)
	x3.site(41, 9901, true)
	x3.tasksMoved[41] = true
	running := true
	x3.programsAnswer = func(int) (transport.MoveSiteSliceResponse, error) {
		if running {
			running = false
			return transport.MoveSiteSliceResponse{Error: "another move into a site's limits is running", Reason: &transport.SiteMoveReason{Code: "another_move"}}, nil
		}
		return transport.MoveSiteSliceResponse{}, nil
	}
	x3.run()
	if m := x3.marks()["41"]; m.State != siteAutoMoved {
		t.Fatalf("mark %+v", m)
	}
}

// TestSingleFlight: one worker for the panel; a site created or restored
// while a run waits or runs adds one more pass, never a second worker; a
// press of the administrator in flight makes the worker wait.
func TestSingleFlight(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	var queued []func()
	startSiteAutoMoveRun = func(run func()) { queued = append(queued, run) }
	x.f.panel.scheduleSiteAutoMove(siteAutoMoveFirstWait)
	x.f.panel.pokeSiteAutoMove()
	x.f.panel.pokeSiteAutoMove()
	if len(queued) != 1 {
		t.Fatalf("%d workers", len(queued))
	}
	// A press of the administrator runs: the worker waits for it.
	siteMoveMu.Lock()
	siteMoveRunning = true
	siteMoveMu.Unlock()
	pressDone := false
	x.tasksAnswer = func(int) (transport.SiteTasksMoveResponse, error) {
		if !pressDone {
			t.Error("the worker moved beside a press")
		}
		return transport.SiteTasksMoveResponse{}, nil
	}
	x.onSleep = func(d time.Duration) {
		if d == siteAutoMoveRetryFirst && !pressDone {
			if len(x.calls) != 0 {
				t.Fatal("the worker moved beside a press")
			}
			pressDone = true
			siteMoveMu.Lock()
			siteMoveRunning = false
			siteMoveMu.Unlock()
		}
	}
	queued[0]()
	if !pressDone {
		t.Fatal("the worker never waited for the press")
	}
	if got := strings.Join(x.calls, ","); got != "tasks:41,programs:41" {
		t.Fatalf("calls %s", got)
	}
	if siteAutoMove.scheduled || siteAutoMove.again {
		t.Fatal("the worker did not end")
	}
}

// TestAPageReadStartsNothing (D-024): reading the site's card and Problems
// now never sends a move and never starts the worker.
func TestAPageReadStartsNothing(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	var started int
	startSiteAutoMoveRun = func(run func()) { started++ }
	oldStart, oldNeeds := startSiteLimitRead, siteLimitReadNeedsAgent
	t.Cleanup(func() { startSiteLimitRead, siteLimitReadNeedsAgent = oldStart, oldNeeds })
	startSiteLimitRead, siteLimitReadNeedsAgent = func(run func()) { run() }, false
	handler := siteLimitsMux(x.f)
	for _, user := range []int{authzMatrixAdminID, authzMatrixResellerID} {
		siteLimitsCall(t, x.f, handler, user, http.MethodGet, "/api/v1/domains/9901/site-limits", "")
	}
	_ = x.f.panel.collectMonitoringProblems(context.Background(), time.Now())
	if len(x.calls) != 0 || started != 0 {
		t.Fatalf("a read moved %v or started the worker %d times", x.calls, started)
	}
}

// TestASitePutBackOnPurposeIsLeftAlone: after the administrator put a site's
// tasks back into cron, the worker leaves it alone (also in the next
// version); its card is the one place a move stays ("move"); moving it from
// the card ends that.
func TestASitePutBackOnPurposeIsLeftAlone(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(42, 9902, false)
	x.tasksMoved[42] = true
	undo := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/tasks-undo", `{"request_id":"undo-0001"}`)
	wantStatus(t, undo, http.StatusOK, "")
	x.tasksMoved[42] = false
	siteAutoMoveVersion = func() string { return "v0.1.0-alpha.151+test" }
	x.run()
	if len(x.calls) != 0 {
		t.Fatalf("the worker moved a site put back on purpose: %v", x.calls)
	}
	card := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
	var view siteLimitsView
	if err := json.Unmarshal(card.Body.Bytes(), &view); err != nil || view.AutoMove == nil || view.AutoMove.Offer != "move" {
		t.Fatalf("card %s", card.Body.String())
	}
	if len(x.lines()) != 0 {
		t.Fatal("a site put back on purpose is a line")
	}
	move := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodPost, "/api/v1/domains/9902/site-limits/move", `{"request_id":"press-0002"}`)
	wantStatus(t, move, http.StatusOK, "")
	if put, _ := x.f.panel.sitesPutBack(context.Background()); len(put) != 0 {
		t.Fatalf("still put back: %v", put)
	}
	card = siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
	view = siteLimitsView{}
	_ = json.Unmarshal(card.Body.Bytes(), &view)
	if view.AutoMove != nil {
		t.Fatalf("a healthy site offers a move: %+v", view.AutoMove)
	}
}

// TestCustomersAndResellersReadNothingOfIt: the card's worker part (the calm
// line, Try again, the move) is the administrator's alone; a healthy site's
// card offers no move to anyone; the server owner's reasons (a line the
// timers do not reproduce, the owner's cron.allow) are no line in Problems
// now.
func TestCustomersAndResellersReadNothingOfIt(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(42, 9902, true)
	x.tasksMoved[42] = false
	x.tasksAnswer = func(int) (transport.SiteTasksMoveResponse, error) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: transport.SiteTasksRefusalCronAllow, Error: "the server owner's cron.allow names the account"}, nil
	}
	x.run()
	if m := x.marks()["42"]; m.State != siteAutoLeft || m.Reason != transport.SiteTasksRefusalCronAllow || !x.inside[42] {
		t.Fatalf("mark %+v inside %v", m, x.inside[42])
	}
	if len(x.lines()) != 0 {
		t.Fatal("the server owner's choice is a to-do line")
	}
	siteAutoMove.set(true, 1, 4)
	for _, user := range []int{authzMatrixCustomerID, authzMatrixResellerID} {
		card := siteLimitsCall(t, x.f, siteLimitsMux(x.f), user, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
		if strings.Contains(card.Body.String(), "auto_move") {
			t.Fatalf("user %d reads the worker: %s", user, card.Body.String())
		}
	}
	card := siteLimitsCall(t, x.f, siteLimitsMux(x.f), authzMatrixAdminID, http.MethodGet, "/api/v1/domains/9902/site-limits", "")
	var view siteLimitsView
	_ = json.Unmarshal(card.Body.Bytes(), &view)
	if view.AutoMove == nil || view.AutoMove.Total != 4 || view.AutoMove.Offer != "retry" || view.AutoMove.Reason != transport.SiteTasksRefusalCronAllow {
		t.Fatalf("administrator's card %s", card.Body.String())
	}
	if report := x.f.panel.collectMonitoringProblems(context.Background(), time.Now()); report.SiteMoveProgress == nil || report.SiteMoveProgress.Done != 1 {
		t.Fatalf("progress %+v", report.SiteMoveProgress)
	}
}

// TestClassifySiteMove: each answer of the press as the worker reads it.
func TestClassifySiteMove(t *testing.T) {
	moved := &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksMoved}
	for i, c := range []struct {
		resp          transport.MoveSiteSliceResponse
		tasks         *transport.SiteTasksMoveResponse
		outcome       siteMovePressOutcome
		state, reason string
	}{
		{transport.MoveSiteSliceResponse{Done: true}, moved, siteMovePressRan, siteAutoMoved, ""},
		{transport.MoveSiteSliceResponse{}, nil, siteMovePressUnreachable, siteAutoUnknown, "agent_unreachable"},
		{transport.MoveSiteSliceResponse{}, nil, siteMovePressNoLimits, siteAutoFailed, "limits_unread"},
		{transport.MoveSiteSliceResponse{Reason: &transport.SiteMoveReason{Code: "not_finished"}}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}, siteMovePressRan, siteAutoUnknown, "tasks_unknown"},
		{transport.MoveSiteSliceResponse{Kept: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown, Recovery: &transport.SiteSliceMoveRecovery{Kept: true}}, siteMovePressRan, siteAutoKept, "tasks"},
		{transport.MoveSiteSliceResponse{Kept: true, Reason: &transport.SiteMoveReason{Code: transport.SiteMoveRefusalKept}}, nil, siteMovePressRan, siteAutoWaitKept, transport.SiteMoveRefusalKept},
		{transport.MoveSiteSliceResponse{Reason: &transport.SiteMoveReason{Code: "update_unreadable"}}, nil, siteMovePressRan, siteAutoWait, "update_unreadable"},
		{transport.MoveSiteSliceResponse{Reason: &transport.SiteMoveReason{Code: "build"}}, nil, siteMovePressRan, siteAutoWait, "build"},
		{transport.MoveSiteSliceResponse{Reason: &transport.SiteMoveReason{Code: "prepare_failed"}}, nil, siteMovePressRan, siteAutoFailed, "prepare_failed"},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksStayed}, siteMovePressRan, siteAutoLeft, "stayed"},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: transport.SiteTasksRefusalLimits}, siteMovePressRan, siteAutoFailed, transport.SiteTasksRefusalLimits},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: transport.SiteTasksRefusalProcesses}, siteMovePressRan, siteAutoFailed, transport.SiteTasksRefusalProcesses},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksPutBack, Refusal: transport.SiteTasksRefusalCrontab}, siteMovePressRan, siteAutoFailed, transport.SiteTasksRefusalCrontab},
		{transport.MoveSiteSliceResponse{Done: true}, &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksCrontabBack}, siteMovePressRan, siteAutoMoved, ""},
	} {
		if state, reason := classifySiteMove(c.resp, c.tasks, c.outcome); state != c.state || reason != c.reason {
			t.Errorf("%d: %s %s, want %s %s", i, state, reason, c.state, c.reason)
		}
	}
}

// TestADeletedSiteLeavesNoMark: the marks of a site that no longer exists go
// with the next run (no leftovers); a live site's mark stays.
func TestADeletedSiteLeavesNoMark(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	x.inside[41], x.tasksMoved[41] = true, true
	ctx := context.Background()
	for _, id := range []int{41, 77} {
		if err := x.f.panel.setSiteAutoMark(ctx, id, siteAutoMark{State: siteAutoFailed, At: "2026-11-28T09:00:00Z"}); err != nil {
			t.Fatal(err)
		}
		x.f.panel.noteSitePutBack(ctx, id, true)
	}
	x.run()
	marks := x.marks()
	if _, ok := marks["77"]; ok || marks["41"].State != siteAutoFailed {
		t.Fatalf("marks %+v", marks)
	}
	if put, _ := x.f.panel.sitesPutBack(ctx); len(put) != 1 || put["41"] == "" {
		t.Fatalf("put back %+v", put)
	}
}

// TestAMarkThatCannotBeReadMovesNothing: a record of the marks that cannot
// be decoded is never "no marks" (a site must not be moved twice).
func TestAMarkThatCannotBeReadMovesNothing(t *testing.T) {
	x := newAutoMoveFake(t)
	x.site(41, 9901, true)
	if err := x.f.panel.setSetting(context.Background(), settingSiteAutoMove, "{not json"); err != nil {
		t.Fatal(err)
	}
	x.run()
	if len(x.calls) != 0 {
		t.Fatalf("moved without a readable record: %v", x.calls)
	}
}
