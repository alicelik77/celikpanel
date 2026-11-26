package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 (S4 R2b on top of alpha.149's one move record): the Agent's
// start with a tasks record, by COUNT of systemd calls on every path; a
// batched enable that fails half-way; the update gate of the tasks move, its
// undo and the rebuild of a task file; one press, one deadline.

// rebootTaskCrontab has a task that starts now and one at boot, so that a
// recovery that switches timers on makes both of its enable calls.
const rebootTaskCrontab = moveTaskCrontab + "@reboot php warm.php\n"

// systemdCalls are the systemctl calls the fake saw (not the other seams'
// notes).
func (h *taskTestHost) systemdCalls() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, c := range h.calls {
		if strings.HasPrefix(c, "prepare") || strings.HasPrefix(c, "end-crontabs") {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (h *taskTestHost) resetCalls() {
	h.mu.Lock()
	h.calls = nil
	h.mu.Unlock()
}

// timersOn are the site's timers enabled or active now.
func (h *taskTestHost) timersOn() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	on := map[string]bool{}
	for unit, v := range h.enabled {
		if v && strings.HasPrefix(unit, "celikcron-") {
			on[unit] = true
		}
	}
	for unit, v := range h.active {
		if v && strings.HasPrefix(unit, "celikcron-") {
			on[unit] = true
		}
	}
	return on
}

// halfEnable makes the next `systemctl enable --now a b ...` switch on the
// first timer only and then fail (a batched enable that fails half-way);
// times says how many such calls fail.
func halfEnable(t *testing.T, times *int) {
	t.Helper()
	inner := siteTaskSystemctl
	t.Cleanup(func() { siteTaskSystemctl = inner })
	siteTaskSystemctl = func(args ...string) ([]byte, error) {
		if *times > 0 && len(args) >= 4 && args[0] == "enable" && args[1] == "--now" {
			*times--
			_, _ = inner("enable", "--now", args[2])
			return []byte("Failed to start " + args[3]), errors.New("exit status 1")
		}
		return inner(args...)
	}
}

// failCalls makes every call whose first word is verb fail while *on.
func failCalls(t *testing.T, verb string, on *bool) {
	t.Helper()
	inner := siteTaskSystemctl
	t.Cleanup(func() { siteTaskSystemctl = inner })
	siteTaskSystemctl = func(args ...string) ([]byte, error) {
		if *on && len(args) > 0 && args[0] == verb {
			return []byte("Failed to " + verb + " unit"), errors.New("exit status 1")
		}
		return inner(args...)
	}
}

// checkNothingRunsTwice: the crontab and a timer are never both on, and
// while a record is kept a timer that is on is one the record knows
// (without a record the timers that are on are a finished move's).
func checkNothingRunsTwice(t *testing.T, host *taskTestHost) {
	t.Helper()
	on := host.timersOn()
	if _, has := host.crontab(taskTestAccount); has && len(on) > 0 {
		t.Fatalf("the crontab and timers %v are both on", on)
	}
	journal, err := readSiteMoveJournal()
	if err != nil {
		return
	}
	known := map[string]bool{}
	if journal.Tasks != nil {
		for _, timer := range journal.Tasks.Timers {
			known[timer] = true
		}
	}
	for timer := range on {
		if !known[timer] {
			t.Fatalf("%s is on and the record does not know it (record: %v)", timer, known)
		}
	}
}

// stoppedMove leaves the record a move stopped at point leaves.
func stoppedMove(t *testing.T, host *taskTestHost, crontab, point string) {
	t.Helper()
	host.startWith(t, crontab)
	siteTasksMoveKilled = func(p string) bool { return p == point }
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksUnknown {
		t.Fatalf("the stopped move answered %+v", resp)
	}
	siteTasksMoveKilled = func(string) bool { return false }
}

// stoppedUndo leaves the record an undo stopped at point leaves.
func stoppedUndo(t *testing.T, host *taskTestHost, crontab, point string) {
	t.Helper()
	host.startWith(t, crontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %+v", resp)
	}
	siteTasksMoveKilled = func(p string) bool { return p == point }
	if resp := undoTasks(t); resp.Outcome != transport.SiteTasksUnknown {
		t.Fatalf("the stopped undo answered %+v", resp)
	}
	siteTasksMoveKilled = func(string) bool { return false }
}

// TestTheAgentsStartWithATasksRecordMakesAtMostFourSystemdCalls: whatever
// point a tasks move or undo stopped at, and whatever fails at the start
// (an enable, a batched enable half-way, the crontab's way back), the
// Agent's start makes at most FOUR systemd calls (counted, the slice's
// reading included), never waits for a task, and leaves nothing running
// twice nor a timer on that the record does not know. A record it cannot
// settle stays with how the tasks run now.
func TestTheAgentsStartWithATasksRecordMakesAtMostFourSystemdCalls(t *testing.T) {
	type tc struct {
		name    string
		prepare func(t *testing.T, host *taskTestHost)
		kept    bool
		running string
	}
	var cases []tc
	for _, point := range []string{siteTasksStepPlanned, "act:" + siteTasksStepSourceWritten, siteTasksStepSourceWritten,
		"act:" + siteTasksStepUnitsWritten, siteTasksStepUnitsWritten, "act:" + siteTasksStepCronClosed, siteTasksStepCronClosed,
		"act:" + siteTasksStepCrontabEmptied, siteTasksStepCrontabEmptied, "act:" + siteTasksStepEnabled, siteTasksStepEnabled} {
		point := point
		cases = append(cases, tc{name: "move_" + point, prepare: func(t *testing.T, host *taskTestHost) { stoppedMove(t, host, rebootTaskCrontab, point) }})
	}
	for _, point := range []string{siteTasksStepUndoPlanned, "act:" + siteTasksStepTimersDisabled, siteTasksStepTimersDisabled,
		"act:" + siteTasksStepCronOpened, siteTasksStepCronOpened, "act:" + siteTasksStepCrontabWritten, siteTasksStepCrontabWritten} {
		point := point
		cases = append(cases, tc{name: "undo_" + point, prepare: func(t *testing.T, host *taskTestHost) { stoppedUndo(t, host, rebootTaskCrontab, point) }})
	}
	cases = append(cases,
		tc{name: "emptied_enable_fails", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
			on := true
			failCalls(t, "enable", &on)
		}},
		tc{name: "emptied_enable_half_way", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab+"*/10 * * * * php ten.php\n", siteTasksStepCrontabEmptied)
			times := 1
			halfEnable(t, &times)
		}},
		tc{name: "emptied_boot_enable_fails", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
			inner := siteTaskSystemctl
			t.Cleanup(func() { siteTaskSystemctl = inner })
			siteTaskSystemctl = func(args ...string) ([]byte, error) {
				if len(args) > 1 && args[0] == "enable" && args[1] != "--now" {
					return []byte("Failed"), errors.New("exit status 1")
				}
				return inner(args...)
			}
		}},
		tc{name: "emptied_limits_missing", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
			slice, _ := siteSlicePathOf(taskTestSite)
			_ = os.Remove(slice)
		}},
		// The crontab went (the site's own code removed it) before the record
		// said so, and it cannot be written back: the put-back keeps the
		// record and switches the timers on again (both enables).
		tc{name: "put_back_not_back", kept: true, running: transport.SiteTasksRunTimers, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCronClosed)
			_ = os.Remove(spoolPath(taskTestAccount))
			host.installFails = true
		}},
		tc{name: "put_back_not_back_enable_fails", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCronClosed)
			_ = os.Remove(spoolPath(taskTestAccount))
			host.installFails = true
			on := true
			failCalls(t, "enable", &on)
		}},
		tc{name: "undo_rollback_enable_fails", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedUndo(t, host, rebootTaskCrontab, siteTasksStepTimersDisabled)
			on := true
			failCalls(t, "enable", &on)
		}},
		tc{name: "undo_finish_reload_fails", kept: true, running: transport.SiteTasksRunCron, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedUndo(t, host, rebootTaskCrontab, siteTasksStepCrontabWritten)
			on := true
			failCalls(t, "daemon-reload", &on)
		}},
		// Fix round F3: the move's own enable ran before the Agent stopped
		// (the record still says crontab_emptied) and the limits file is gone
		// at the start: the timers are switched off anyway.
		tc{name: "enabled_before_the_stop_limits_missing", kept: true, running: transport.SiteTasksRunNone, prepare: func(t *testing.T, host *taskTestHost) {
			stoppedMove(t, host, rebootTaskCrontab, "act:"+siteTasksStepEnabled)
			slice, _ := siteSlicePathOf(taskTestSite)
			_ = os.Remove(slice)
		}},
	)
	for _, c := range cases {
		t.Run(strings.ReplaceAll(c.name, ":", "_"), func(t *testing.T) {
			host := useTaskTestHost(t)
			c.prepare(t, host)
			host.resetCalls()
			recoverSiteSlicesAtStartup()
			calls := host.systemdCalls()
			if len(calls) > 4 {
				t.Fatalf("the start made %d systemd calls: %v", len(calls), calls)
			}
			for _, call := range calls {
				if strings.HasPrefix(call, "start ") || strings.HasPrefix(call, "restart") || strings.HasPrefix(call, "stop ") {
					t.Fatalf("the start started or waited for a task: %s", call)
				}
			}
			checkNothingRunsTwice(t, host)
			_, err := os.Stat(siteMoveJournalPath)
			if kept := err == nil; kept != c.kept {
				t.Fatalf("record kept %v, want %v (calls %v)", kept, c.kept, calls)
			}
			if c.kept {
				record := readSiteMoveOutcomes()["7"]
				if !record.Kept || record.Tasks != transport.SiteTasksUnknown || record.TasksRunning != c.running {
					t.Fatalf("outcome %+v, want running %q", record, c.running)
				}
				if c.running == transport.SiteTasksRunNone && len(host.timersOn()) > 0 {
					t.Fatalf("said not running, but timers are on: %v", host.timersOn())
				}
			}
		})
	}
}

// TestTheAgentsStartWithATasksRecordStopsAtItsBound: every systemd call
// hangs; the start sends one, gives it up at its bound, sends nothing more
// and keeps the record. Its time is the bound, not a call's.
func TestTheAgentsStartWithATasksRecordStopsAtItsBound(t *testing.T) {
	host := useTaskTestHost(t)
	stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
	oldBound := siteMoveStartBound
	t.Cleanup(func() { siteMoveStartBound = oldBound })
	siteMoveStartBound = 200 * time.Millisecond
	const hang = 1500 * time.Millisecond
	host.resetCalls()
	host.site = func() { time.Sleep(hang) }
	started := time.Now()
	recoverSiteSlicesAtStartup()
	took := time.Since(started)
	time.Sleep(hang + 200*time.Millisecond) // the call given up answers now
	if took >= hang {
		t.Fatalf("the start waited %v for a call that hangs (bound %v)", took, siteMoveStartBound)
	}
	if calls := host.systemdCalls(); len(calls) != 1 {
		t.Fatalf("calls sent after the bound: %v", calls)
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record went although nothing was settled")
	}
	if _, has := host.crontab(taskTestAccount); has || len(host.timersOn()) > 0 {
		t.Fatalf("something was switched on: crontab %v timers %v", has, host.timersOn())
	}
}

// TestABatchedEnableThatFailsHalfWayIsPutBackWithoutRunningTwice: the move's
// `systemctl enable --now a b c` switches some timers on and fails; the
// put-back switches them off and only then writes the crontab back. When
// switching them off is not confirmed, the crontab is not written back
// beside them: the record stays, the timers it knows are on again, and
// Try again settles it.
func TestABatchedEnableThatFailsHalfWayIsPutBackWithoutRunningTwice(t *testing.T) {
	const crontab = moveTaskCrontab + "*/10 * * * * php ten.php\n*/20 * * * * php twenty.php\n"
	t.Run("switched_off_then_back", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, crontab)
		times := 1
		halfEnable(t, &times)
		resp := moveTasks(t)
		if resp.Outcome != transport.SiteTasksPutBack {
			t.Fatalf("move %+v", resp)
		}
		checkNothingRunsTwice(t, host)
		if got, has := host.crontab(taskTestAccount); !has || got != crontab || len(host.timersOn()) != 0 || len(host.unitFiles(t)) != 0 {
			t.Fatalf("crontab %v %q, timers %v, files %v", has, got, host.timersOn(), host.unitFiles(t))
		}
		if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
			t.Fatal("the record stayed")
		}
	})
	t.Run("off_not_confirmed_keeps_the_record", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, crontab)
		times := 1
		halfEnable(t, &times)
		disableFails := true
		failCalls(t, "disable", &disableFails)
		resp := moveTasks(t)
		if resp.Outcome != transport.SiteTasksUnknown || resp.Recovery == nil || resp.Recovery.TasksRunning != transport.SiteTasksRunTimers {
			t.Fatalf("move %+v", resp)
		}
		checkNothingRunsTwice(t, host)
		if _, has := host.crontab(taskTestAccount); has {
			t.Fatal("the crontab was written back beside timers that may be on")
		}
		journal, err := readSiteMoveJournal()
		if err != nil || journal.Tasks == nil || journal.Tasks.Crontab != crontab {
			t.Fatalf("the record does not keep the crontab: %+v %v", journal, err)
		}
		// Try again, with systemd answering again: the record is at
		// crontab_emptied, so it finishes the move (every timer the record
		// knows on, no crontab) and the record goes.
		disableFails = false
		claimed, release := claimSiteMove()
		if claimed != "" {
			t.Fatal(claimed)
		}
		recoverSiteSliceMove(time.Now().Add(time.Minute), true)
		release()
		checkNothingRunsTwice(t, host)
		if _, has := host.crontab(taskTestAccount); has || len(host.timersOn()) != len(journal.Tasks.Timers) {
			t.Fatalf("after Try again: crontab %v, timers %v of %v", has, host.timersOn(), journal.Tasks.Timers)
		}
		if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) || readSiteMoveOutcomes()["7"].Tasks != transport.SiteTasksMoved {
			t.Fatalf("after Try again: record %v, outcome %+v", err, readSiteMoveOutcomes()["7"])
		}
	})
	t.Run("at_the_start_then_try_again", func(t *testing.T) {
		host := useTaskTestHost(t)
		stoppedMove(t, host, crontab, siteTasksStepCrontabEmptied)
		times := 2
		halfEnable(t, &times)
		recoverSiteSlicesAtStartup()
		checkNothingRunsTwice(t, host)
		if len(host.timersOn()) != 0 {
			t.Fatalf("the start left timers on after a half-way enable: %v", host.timersOn())
		}
		claimed, release := claimSiteMove()
		if claimed != "" {
			t.Fatal(claimed)
		}
		recoverSiteSliceMove(time.Now().Add(time.Minute), true)
		release()
		checkNothingRunsTwice(t, host)
		if got, has := host.crontab(taskTestAccount); !has || got != crontab || len(host.timersOn()) != 0 {
			t.Fatalf("after Try again: crontab %v %q, timers %v", has, got, host.timersOn())
		}
	})
}

// TestTasksMoveUndoAndRebuildKeepTheUpdateGate: while the panel's update is
// in progress (or its record cannot be read) the tasks move, the undo and
// the rebuild of a task file are refused with main's codes and words before
// anything changes; while each of them runs, the update is refused.
func TestTasksMoveUndoAndRebuildKeepTheUpdateGate(t *testing.T) {
	oldUpdate, oldClaimed := siteMoveUpdateState, siteMoveClaimed
	t.Cleanup(func() { siteMoveUpdateState, siteMoveClaimed = oldUpdate, oldClaimed })
	for _, state := range []string{"update_running", "update_unreadable"} {
		t.Run(state, func(t *testing.T) {
			host := useTaskTestHost(t)
			host.startWith(t, moveTaskCrontab)
			siteMoveUpdateState = func() string { return state }
			move := moveTasks(t)
			undo := undoTasks(t)
			var rebuild transport.SiteTaskSourceResponse
			_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &rebuild)
			for name, got := range map[string][2]string{"move": {move.Refusal, move.Error}, "undo": {undo.Refusal, undo.Error}, "rebuild": {rebuild.Refusal, rebuild.Error}} {
				if got[0] != state || got[1] != siteMoveRefusalText(state) {
					t.Fatalf("%s: %q %q", name, got[0], got[1])
				}
			}
			if c, _ := host.crontab(taskTestAccount); c != moveTaskCrontab || len(host.unitFiles(t)) != 0 || len(host.systemdCalls()) != 0 {
				t.Fatalf("something changed: %v", host.systemdCalls())
			}
		})
	}
	t.Run("the_update_waits", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		siteMoveUpdateState = func() string { return "" }
		var refused []string
		siteMoveClaimed = func() {
			var reply transport.SystemUpdateStartResponse
			_ = (&Agent{}).StartSystemUpdate(&transport.SystemUpdateStartRequest{}, &reply)
			refused = append(refused, reply.Reason)
		}
		if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
			t.Fatalf("move %+v", resp)
		}
		if err := os.Remove(sitecron.SourcePath(taskTestSite)); err != nil {
			t.Fatal(err)
		}
		var rebuild transport.SiteTaskSourceResponse
		_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &rebuild)
		if rebuild.Outcome != transport.SiteTaskSourceRebuilt {
			t.Fatalf("rebuild %+v", rebuild)
		}
		if resp := undoTasks(t); resp.Outcome != transport.SiteTasksUndone {
			t.Fatalf("undo %+v", resp)
		}
		if len(refused) != 3 {
			t.Fatalf("the claim was taken %d times, want 3 (move, rebuild, undo)", len(refused))
		}
		for _, reason := range refused {
			if reason != transport.HostMutationReasonSiteMove {
				t.Fatalf("the update was not refused while a tasks change ran: %v", refused)
			}
		}
		// A preview takes no claim.
		refused = nil
		var preview transport.SiteTaskSourceResponse
		_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}, Preview: true}, &preview)
		if len(refused) != 0 {
			t.Fatal("a preview took the move's claim")
		}
	})
}

// TestOnePressOneDeadline: the programs part of a press gets what the Panel
// says is left of it, never more than the Agent's own budget; absent or 0
// is that budget; a negative value (never sent) means nothing is left; an
// absurd one cannot overflow into more.
func TestOnePressOneDeadline(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	own := now.Add(siteMoveBudget)
	for _, c := range []struct {
		seconds int
		want    time.Time
	}{
		{0, own},
		{-5, now},
		{1, now.Add(time.Second)},
		{300, now.Add(5 * time.Minute)},
		{int(siteMoveBudget / time.Second), own},
		{int(siteMoveBudget/time.Second) + 1, own},
		{1 << 40, own},
		{int(^uint(0) >> 1), own},
	} {
		if got := siteMovePressDeadline(now, c.seconds); !got.Equal(c.want) {
			t.Fatalf("%d seconds: %v, want %v", c.seconds, got, c.want)
		}
		if got := siteMovePressDeadline(now, c.seconds); got.After(own) {
			t.Fatalf("%d seconds gave more than the Agent's budget", c.seconds)
		}
	}
}
