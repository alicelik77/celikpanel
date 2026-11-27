package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 fix round 4 after the fourth review of R2b (N1-N9).

// TestAKeptMoveWhoseTimersWereSwitchedOffSwitchesThemOnAgain (N1): a move
// kept at enabled meets a crontab (the server owner's, or one being
// compared), so its timers are switched off and the record stays. Once the
// crontab is removed again (the owner keeps the timers), the next pass - the
// Agent's start or Try again - switches every timer on again and answers
// "moved"; it never answers "moved" with the timers left off.
func TestAKeptMoveWhoseTimersWereSwitchedOffSwitchesThemOnAgain(t *testing.T) {
	for _, atStart := range []bool{true, false} {
		name := "try_again"
		if atStart {
			name = "start"
		}
		t.Run(name, func(t *testing.T) {
			host := useTaskTestHost(t)
			stoppedMove(t, host, rebootTaskCrontab, siteTasksStepEnabled)
			want := host.timersOn()
			if len(want) == 0 {
				t.Fatal("the stopped move left no timer on")
			}
			if err := os.WriteFile(spoolPath(taskTestAccount), []byte(cronHeader+"* * * * * echo other\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			recoverSiteSlicesAtStartup()
			checkNothingRunsTwice(t, host)
			if len(host.timersOn()) != 0 {
				t.Fatalf("timers on beside the crontab: %v", host.timersOn())
			}
			journal, err := readSiteMoveJournal()
			if err != nil || journal.Tasks == nil || journal.Tasks.Step != siteTasksStepCrontabEmptied {
				t.Errorf("the kept record's step was not saved before the switch-off: %v %+v", err, journal)
			}
			// The owner keeps the timers: the crontab goes.
			if err := os.Remove(spoolPath(taskTestAccount)); err != nil {
				t.Fatal(err)
			}
			host.resetCalls()
			if atStart {
				recoverSiteSlicesAtStartup()
				if calls := host.systemdCalls(); len(calls) > 4 {
					t.Fatalf("the start made %d systemd calls: %v", len(calls), calls)
				}
			} else {
				tryAgainTasks(t)
			}
			checkNothingRunsTwice(t, host)
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) || readSiteMoveOutcomes()["7"].Tasks != transport.SiteTasksMoved {
				t.Fatalf("record %v, outcome %+v", err, readSiteMoveOutcomes()["7"])
			}
			on := host.timersOn()
			for timer := range want {
				if !on[timer] {
					t.Fatalf("answered moved with %s off (on: %v, want %v)", timer, on, want)
				}
			}
		})
	}
}

// TestACrontabProcessIsKnownByItsFileNotItsPath (N2): a process of the
// site's account is compared with the crontab program by the file its
// /proc/<pid>/exe leads to, not by the link's text. A program started from a
// folder deeper than a path can be long (the kernel answers ENAMETOOLONG for
// the text) no longer refuses the move; a crontab reached that way is still
// ended, and so is one replaced since it started; a process whose file
// cannot be read refuses the move with its own code (the page words it,
// naming who acts), before anything changes and at cron_closed.
func TestACrontabProcessIsKnownByItsFileNotItsPath(t *testing.T) {
	type proc struct {
		runs    string // the file /proc/<pid>/exe leads to ("crontab", "other", "old")
		statErr error
		text    string // the link's text; "" = ENAMETOOLONG
	}
	setup := func(t *testing.T, procs map[string]proc) *[]int {
		t.Helper()
		root := t.TempDir()
		files := map[string]string{"crontab": filepath.Join(root, "bin", "crontab"), "other": filepath.Join(root, "deep", "sleep"), "old": filepath.Join(root, "old", "crontab")}
		for _, path := range files {
			writeTaskFile(t, path, "")
		}
		oldRoot, oldLook, oldKill, oldStat, oldRead := procRoot, cronLookPath, siteTaskKill, siteTaskStatExe, siteTaskReadExe
		t.Cleanup(func() {
			procRoot, cronLookPath, siteTaskKill, siteTaskStatExe, siteTaskReadExe = oldRoot, oldLook, oldKill, oldStat, oldRead
		})
		procRoot = filepath.Join(root, "proc")
		cronLookPath = func(string) (string, error) { return files["crontab"], nil }
		for pid := range procs {
			writeTaskFile(t, filepath.Join(procRoot, pid, "status"), "Name:\tx\nUid:\t1500\t1500\t0\t0\n")
			writeTaskFile(t, filepath.Join(procRoot, pid, "exe"), "")
		}
		siteTaskStatExe = func(path string) (os.FileInfo, error) {
			p := procs[filepath.Base(filepath.Dir(path))]
			if p.statErr != nil {
				return nil, p.statErr
			}
			return os.Stat(files[p.runs])
		}
		siteTaskReadExe = func(path string) (string, error) {
			p := procs[filepath.Base(filepath.Dir(path))]
			if p.text == "" {
				return "", &os.PathError{Op: "readlink", Path: path, Err: syscall.ENAMETOOLONG}
			}
			return p.text, nil
		}
		var killed []int
		siteTaskKill = func(pid int) error { killed = append(killed, pid); return nil }
		return &killed
	}
	t.Run("a_deep_path_does_not_refuse", func(t *testing.T) {
		killed := setup(t, map[string]proc{"4001": {runs: "other"}})
		if n, err := endCrontabProcesses(1500); err != nil || n != 0 || len(*killed) != 0 {
			t.Fatalf("%d %v %v", n, err, *killed)
		}
		if err := siteTaskCheckCrontabs(1500); err != nil {
			t.Fatalf("the listing refused: %v", err)
		}
	})
	t.Run("crontab_by_its_file_is_ended", func(t *testing.T) {
		killed := setup(t, map[string]proc{"4001": {runs: "other"}, "4002": {runs: "crontab"}})
		if n, err := endCrontabProcesses(1500); err != nil || n != 1 || len(*killed) != 1 || (*killed)[0] != 4002 {
			t.Fatalf("%d %v %v", n, err, *killed)
		}
	})
	t.Run("a_crontab_replaced_since_it_started_is_ended", func(t *testing.T) {
		killed := setup(t, map[string]proc{"4001": {runs: "old"}})
		program, _ := cronLookPath("crontab")
		if resolved, err := filepath.EvalSymlinks(program); err == nil {
			program = resolved
		}
		siteTaskReadExe = func(string) (string, error) { return program + " (deleted)", nil }
		if n, err := endCrontabProcesses(1500); err != nil || n != 1 || len(*killed) != 1 {
			t.Fatalf("%d %v %v", n, err, *killed)
		}
	})
	t.Run("a_file_that_cannot_be_read_refuses", func(t *testing.T) {
		killed := setup(t, map[string]proc{"4003": {statErr: syscall.EACCES}})
		if _, err := endCrontabProcesses(1500); err == nil || !errors.Is(err, errCrontabProcessUnknown) || len(*killed) != 0 {
			t.Fatalf("%v %v", err, *killed)
		}
		if err := siteTaskCheckCrontabs(1500); !errors.Is(err, errCrontabProcessUnknown) {
			t.Fatalf("the listing: %v", err)
		}
	})
	t.Run("a_text_that_cannot_be_read_otherwise_refuses", func(t *testing.T) {
		setup(t, map[string]proc{"4004": {runs: "other"}})
		siteTaskReadExe = func(string) (string, error) { return "", syscall.EIO }
		if _, err := endCrontabProcesses(1500); !errors.Is(err, errCrontabProcessUnknown) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("gone_is_passed_over", func(t *testing.T) {
		setup(t, map[string]proc{"4004": {statErr: os.ErrNotExist}, "4005": {statErr: syscall.ESRCH}})
		if _, err := endCrontabProcesses(1500); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the_moves_refusal_has_its_code", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		old := siteTaskCheckCrontabs
		t.Cleanup(func() { siteTaskCheckCrontabs = old })
		siteTaskCheckCrontabs = func(int) error { return errCrontabProcessUnknown }
		resp := moveTasks(t)
		if resp.Outcome != transport.SiteTasksRefused || resp.Refusal != transport.SiteTasksRefusalProcesses {
			t.Fatalf("move %+v", resp)
		}
		if got, _ := host.crontab(taskTestAccount); got != moveTaskCrontab || len(host.unitFiles(t)) != 0 {
			t.Fatalf("something changed: %q %v", got, host.unitFiles(t))
		}
	})
	t.Run("at_cron_closed_the_put_back_has_its_code", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		old := siteTaskEndCrontabs
		t.Cleanup(func() { siteTaskEndCrontabs = old })
		siteTaskEndCrontabs = func(int) (int, error) { return 0, errCrontabProcessUnknown }
		resp := moveTasks(t)
		if resp.Outcome != transport.SiteTasksPutBack || resp.Refusal != transport.SiteTasksRefusalProcesses {
			t.Fatalf("move %+v", resp)
		}
		checkNothingRunsTwice(t, host)
		if got, _ := host.crontab(taskTestAccount); got != moveTaskCrontab {
			t.Fatalf("the crontab is not as it was: %q", got)
		}
	})
}

// TestTheUndosPausedNoteIsNeverADescription (N7): the note the undo writes
// above a task whose timer was switched off on the server is never a task's
// description on the Scheduled tasks screen, never pushes out the one the
// customer wrote, goes when the task is resumed there, and does not pile up
// over repeated move and undo rounds.
func TestTheUndosPausedNoteIsNeverADescription(t *testing.T) {
	const job = "17 3 * * * php report.php"
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	switchOff := func(t *testing.T) {
		t.Helper()
		units, _ := siteTaskUnits(taskTestSite)
		for n, u := range units {
			if u.TaskKey == generateCronID(job) {
				timer := sitecron.UnitBase(taskTestSite, n) + ".timer"
				host.mu.Lock()
				delete(host.enabled, timer)
				delete(host.active, timer)
				host.mu.Unlock()
				return
			}
		}
		t.Fatal("no timer for the task")
	}
	shown := func(t *testing.T, content string) CronJob {
		t.Helper()
		var found *CronJob
		for _, j := range parseCrontab(content) {
			if strings.Contains(j.Comment, "CelikPanel") || strings.Contains(j.Comment, "switched off") {
				t.Fatalf("the note is shown as a description: %+v", j)
			}
			if j.ID == generateCronID(job) {
				j := j
				found = &j
			}
		}
		if found == nil {
			t.Fatalf("the task is not listed:\n%s", content)
		}
		return *found
	}
	for round := 1; round <= 2; round++ {
		if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
			t.Fatalf("round %d: move %+v", round, resp)
		}
		switchOff(t)
		if resp := undoTasks(t); resp.Outcome != transport.SiteTasksUndone || len(resp.Paused) != 1 {
			t.Fatalf("round %d: undo %+v", round, resp)
		}
		got, _ := host.crontab(taskTestAccount)
		if strings.Count(got, cronPausedNote) != 1 || !strings.Contains(got, "# nightly report\n"+cronPausedNote+"\n# DISABLED: "+job+"\n") {
			t.Fatalf("round %d: crontab\n%s", round, got)
		}
		if j := shown(t, got); j.Enabled || j.Comment != "nightly report" {
			t.Fatalf("round %d: shown %+v", round, j)
		}
		// Moved and put back again without resuming: the note stays once.
		if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
			t.Fatalf("round %d: second move %+v", round, resp)
		}
		if resp := undoTasks(t); resp.Outcome != transport.SiteTasksUndone || len(resp.Paused) != 0 {
			t.Fatalf("round %d: second undo %+v", round, resp)
		}
		if again, _ := host.crontab(taskTestAccount); again != got {
			t.Fatalf("round %d: the crontab changed over a move and undo:\n%s\nwas:\n%s", round, again, got)
		}
		// Resumed on the screen: the note goes, the description stays.
		existing, err := readCrontab(taskTestAccount)
		if err != nil {
			t.Fatal(err)
		}
		if err := updateCronJobFor(taskTestAccount, &UpdateCronJobRequest{ID: generateCronID(job), Schedule: "17 3 * * *", Command: "php report.php",
			Enabled: true, Comment: "nightly report", Version: cronVersion(existing)}); err != nil {
			t.Fatalf("round %d: resume: %v", round, err)
		}
		resumed, _ := host.crontab(taskTestAccount)
		if strings.Contains(resumed, cronPausedNote) || !strings.Contains(resumed, "# nightly report\n"+job+"\n") || strings.Count(resumed, "nightly report") != 1 {
			t.Fatalf("round %d: after resume\n%s", round, resumed)
		}
		if j := shown(t, resumed); !j.Enabled || j.Comment != "nightly report" {
			t.Fatalf("round %d: shown after resume %+v", round, j)
		}
	}
}

// TestTheRebuildsRefusalForAnUnreadableRecordSaysAMoveOnThisServer (N9): a
// move record that cannot be read may be any site's; the rebuild's refusal
// says "a move on this server", not "of this site", and changes nothing.
func TestTheRebuildsRefusalForAnUnreadableRecordSaysAMoveOnThisServer(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %+v", resp)
	}
	if err := os.Remove(sitecron.SourcePath(taskTestSite)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(siteMoveJournalPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	host.resetCalls()
	for _, preview := range []bool{true, false} {
		var resp transport.SiteTaskSourceResponse
		_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}, Preview: preview}, &resp)
		if resp.Refusal != transport.SiteMoveRefusalKept || !strings.Contains(resp.Error, "a move on this server") || strings.Contains(resp.Error, "of this site") {
			t.Fatalf("preview %v: %+v", preview, resp)
		}
	}
	if _, err := os.Stat(sitecron.SourcePath(taskTestSite)); !os.IsNotExist(err) || len(host.systemdCalls()) != 0 {
		t.Fatalf("something changed: %v %v", err, host.systemdCalls())
	}
}

// TestTheScreensSavesLeaveThePausedNoteOut (N7): adding or deleting a task
// on the Scheduled tasks screen does not write the undo's note back either,
// and the screen never lists it.
func TestTheScreensSavesLeaveThePausedNoteOut(t *testing.T) {
	host := useTaskTestHost(t)
	crontab := "# nightly report\n" + cronPausedNote + "\n# DISABLED: 17 3 * * * php report.php\n" + cronPausedNote + "\n# DISABLED: 5 4 * * * php other.php\n*/10 * * * * php ten.php\n"
	host.startWith(t, crontab)
	if jobs := parseCrontab(crontab); len(jobs) != 3 || jobs[0].Comment != "nightly report" || jobs[1].Comment != "" || jobs[2].Comment != "" {
		t.Fatalf("listed %+v", jobs)
	}
	existing, err := readCrontab(taskTestAccount)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteCronJobFor(taskTestAccount, &DeleteCronJobRequest{ID: generateCronID("*/10 * * * * php ten.php"), Version: cronVersion(existing)}); err != nil {
		t.Fatal(err)
	}
	got, _ := host.crontab(taskTestAccount)
	if want := "# nightly report\n# DISABLED: 17 3 * * * php report.php\n# DISABLED: 5 4 * * * php other.php\n"; got != want {
		t.Fatalf("after delete:\n%q\nwant:\n%q", got, want)
	}
}
