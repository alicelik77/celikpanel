package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

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
