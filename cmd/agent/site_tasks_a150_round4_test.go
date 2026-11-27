package main

import (
	"os"
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
