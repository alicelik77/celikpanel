package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 fix round after the third review of R2b (F1-F7): a crontab put
// back by hand never gets timers beside it, at the Agent's start nor at Try
// again; a put-back whose crontab was written but not read back the same
// never switches the timers on beside it.

// putBackByHand is the card's way back by hand: the timers off (when off),
// then the task file installed whole as the account's crontab
// (`crontab -u <account> /etc/celikpanel-cron/<id>.crontab`, which Debian's
// crontab stores under its three header lines).
func (h *taskTestHost) putBackByHand(t *testing.T, off bool) {
	t.Helper()
	if off {
		h.mu.Lock()
		for unit := range h.enabled {
			if strings.HasPrefix(unit, "celikcron-") {
				delete(h.enabled, unit)
				delete(h.active, unit)
			}
		}
		h.mu.Unlock()
	}
	source := readTaskFile(t, sitecron.SourcePath(taskTestSite))
	if err := os.WriteFile(spoolPath(taskTestAccount), []byte(cronHeader+source), 0o600); err != nil {
		t.Fatal(err)
	}
}

// unreadableSpool puts a link where the account's spool file is (read as
// "cannot be read"; it is never followed), to a crontab with a job.
func unreadableSpool(t *testing.T) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("* * * * * date\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(spoolPath(taskTestAccount))
	if err := os.Symlink(target, spoolPath(taskTestAccount)); err != nil {
		t.Fatal(err)
	}
}

func tryAgainTasks(t *testing.T) {
	t.Helper()
	claimed, release := claimSiteMove()
	if claimed != "" {
		t.Fatal(claimed)
	}
	defer release()
	recoverSiteSliceMove(time.Now().Add(time.Minute), true)
}

// TestACrontabPutBackByHandNeverGetsTimersBeside (F1): a move kept at
// crontab_emptied or enabled whose crontab was put back by hand - the
// card's steps, with or without the timers switched off first - is kept at
// the Agent's start with its timers off and "runs from cron"; Try again
// finishes the put-back (the crontab as it is, the timers and files gone,
// cron open for the account). A crontab that is not the recorded one, or a
// spool that cannot be read, keeps the record at Try again too and never
// gets a timer switched on.
func TestACrontabPutBackByHandNeverGetsTimersBeside(t *testing.T) {
	for _, c := range []struct {
		name  string
		point string
		off   bool
	}{
		{"emptied_timers_off", siteTasksStepCrontabEmptied, true},
		{"emptied_enable_ran_timers_left_on", "act:" + siteTasksStepEnabled, false},
		{"enabled_timers_left_on", siteTasksStepEnabled, false},
		{"enabled_timers_off", siteTasksStepEnabled, true},
	} {
		t.Run(strings.ReplaceAll(c.name, ":", "_"), func(t *testing.T) {
			host := useTaskTestHost(t)
			stoppedMove(t, host, rebootTaskCrontab, c.point)
			host.putBackByHand(t, c.off)
			before, _ := os.ReadFile(spoolPath(taskTestAccount))
			recoverSiteSlicesAtStartup()
			checkNothingRunsTwice(t, host)
			if len(host.timersOn()) != 0 {
				t.Fatalf("timers on beside the crontab: %v", host.timersOn())
			}
			record := readSiteMoveOutcomes()["7"]
			if _, err := os.Stat(siteMoveJournalPath); err != nil || !record.Kept || record.TasksRunning != transport.SiteTasksRunCron {
				t.Fatalf("at the start: record %v, outcome %+v", err, record)
			}
			// Try again: the put-back is finished.
			tryAgainTasks(t)
			checkNothingRunsTwice(t, host)
			after, _ := os.ReadFile(spoolPath(taskTestAccount))
			if string(after) != string(before) {
				t.Fatalf("the crontab put back by hand was changed:\n%s", after)
			}
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) || readSiteMoveOutcomes()["7"].Tasks != transport.SiteTasksPutBack {
				t.Fatalf("after Try again: record %v, outcome %+v", err, readSiteMoveOutcomes()["7"])
			}
			if files := host.unitFiles(t); len(files) != 0 || len(host.timersOn()) != 0 {
				t.Fatalf("after Try again: units %v, timers %v", files, host.timersOn())
			}
			if _, err := os.Stat(sitecron.SourcePath(taskTestSite)); !os.IsNotExist(err) {
				t.Fatal("the task file stayed after the put-back")
			}
			if host.refused(taskTestAccount) {
				t.Fatal("cron still keeps the account out after the put-back")
			}
		})
	}
	t.Run("another_crontab_stays_kept", func(t *testing.T) {
		host := useTaskTestHost(t)
		stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
		if err := os.WriteFile(spoolPath(taskTestAccount), []byte(cronHeader+"* * * * * echo other\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, start := range []bool{true, false} {
			if start {
				recoverSiteSlicesAtStartup()
			} else {
				tryAgainTasks(t)
			}
			checkNothingRunsTwice(t, host)
			record := readSiteMoveOutcomes()["7"]
			if _, err := os.Stat(siteMoveJournalPath); err != nil || record.TasksRunning != transport.SiteTasksRunCron || len(host.timersOn()) != 0 {
				t.Fatalf("start %v: record %v, outcome %+v, timers %v", start, err, record, host.timersOn())
			}
			if got, _ := host.crontab(taskTestAccount); got != "* * * * * echo other\n" || len(host.unitFiles(t)) == 0 {
				t.Fatalf("start %v: crontab %q, units %v", start, got, host.unitFiles(t))
			}
		}
	})
	t.Run("unreadable_spool_stays_kept", func(t *testing.T) {
		host := useTaskTestHost(t)
		stoppedMove(t, host, rebootTaskCrontab, siteTasksStepEnabled)
		unreadableSpool(t)
		tryAgainTasks(t)
		checkNothingRunsTwice(t, host)
		record := readSiteMoveOutcomes()["7"]
		if _, err := os.Stat(siteMoveJournalPath); err != nil || !record.Kept || record.TasksRunning != "" || len(host.timersOn()) != 0 {
			t.Fatalf("record %v, outcome %+v, timers %v", err, record, host.timersOn())
		}
	})
	t.Run("no_crontab_still_finishes", func(t *testing.T) {
		host := useTaskTestHost(t)
		stoppedMove(t, host, rebootTaskCrontab, siteTasksStepCrontabEmptied)
		recoverSiteSlicesAtStartup()
		if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) || len(host.timersOn()) == 0 {
			t.Fatalf("record %v, timers %v", err, host.timersOn())
		}
	})
}

// TestTheRebuildsRefusalGivesACommandThatWorks (F1): `systemctl disable`
// takes no pattern, so the command the rebuild's refusal gives lists the
// site's timers by name, and every later step runs only if it worked.
func TestTheRebuildsRefusalGivesACommandThatWorks(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %+v", resp)
	}
	if err := os.Remove(sitecron.SourcePath(taskTestSite)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spoolPath(taskTestAccount), []byte(cronHeader+moveTaskCrontab), 0o600); err != nil {
		t.Fatal(err)
	}
	var resp transport.SiteTaskSourceResponse
	_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}, Preview: true}, &resp)
	want := "sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-7-*.timer' | awk '{print $1}') && sudo rm -f "
	if resp.Refusal != transport.SiteTaskSourceRefusalCrontab || !strings.Contains(resp.Error, want) || !strings.Contains(resp.Error, " && sudo systemctl daemon-reload") ||
		strings.Contains(resp.Error, "disable --now '") || len(resp.LeftUnits) == 0 || !strings.Contains(resp.Error, strings.Join(resp.LeftUnits, " ")) {
		t.Fatalf("refusal %q: %s (%v)", resp.Refusal, resp.Error, resp.LeftUnits)
	}
}

// TestAPutBackWhoseCrontabIsNotReadBackNeverSwitchesTimersOn (F2): the
// move's batched enable fails half-way, its put-back writes the crontab and
// then cannot read it back (or reads another one): the timers are never
// switched on again beside it. The recorded crontab in the spool is back
// (the put-back finishes); anything else keeps the record, timers off.
func TestAPutBackWhoseCrontabIsNotReadBackNeverSwitchesTimersOn(t *testing.T) {
	const crontab = moveTaskCrontab + "*/10 * * * * php ten.php\n*/20 * * * * php twenty.php\n"
	for _, c := range []struct {
		name     string
		differs  bool
		want     string
		keptWith string
	}{
		{"read_back_fails", false, transport.SiteTasksPutBack, ""},
		{"read_back_differs", true, transport.SiteTasksUnknown, transport.SiteTasksRunCron},
	} {
		t.Run(c.name, func(t *testing.T) {
			host := useTaskTestHost(t)
			host.startWith(t, crontab)
			times := 1
			halfEnable(t, &times)
			written := false
			install, list := cronInstallCrontab, cronListCrontab
			t.Cleanup(func() { cronInstallCrontab, cronListCrontab = install, list })
			cronInstallCrontab = func(username, content string) error {
				if c.differs {
					content += "# changed on the way\n"
				}
				err := install(username, content)
				written = err == nil
				return err
			}
			cronListCrontab = func(username string) ([]byte, string, int, error) {
				if written && !c.differs {
					return nil, "crontab: cannot read", 1, errors.New("exit status 1")
				}
				return list(username)
			}
			resp := moveTasks(t)
			checkNothingRunsTwice(t, host)
			if resp.Outcome != c.want || len(host.timersOn()) != 0 {
				t.Fatalf("move %+v, timers %v", resp, host.timersOn())
			}
			if _, has := host.crontab(taskTestAccount); !has {
				t.Fatal("the crontab written back is gone")
			}
			_, err := os.Stat(siteMoveJournalPath)
			if kept := err == nil; kept != (c.keptWith != "") {
				t.Fatalf("record kept %v", kept)
			}
			if c.keptWith != "" && (resp.Recovery == nil || resp.Recovery.TasksRunning != c.keptWith) {
				t.Fatalf("recovery %+v", resp.Recovery)
			}
		})
	}
}

// TestAScheduleSettingIsRefusedNotSilentlyChanged (F5): CRON_TZ=,
// RANDOM_DELAY= and START_HOURS_RANGE= change when jobs run under cronie and
// a timer would keep only the setting: the screen's save of a moved site's
// tasks refuses a task file holding one (the move's refusal is in
// TestTasksMoveKeepsTheWholeCrontabForOneLine), and such a line never
// reaches the settings file.
func TestAScheduleSettingIsRefusedNotSilentlyChanged(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %+v", resp)
	}
	before := readTaskFile(t, sitecron.SourcePath(taskTestSite))
	for _, line := range []string{"CRON_TZ=Europe/Berlin", "RANDOM_DELAY=30", "START_HOURS_RANGE=3-22"} {
		err := siteTaskStore(taskTestTarget()).write(moveTaskCrontab, line+"\n"+moveTaskCrontab, nil)
		if err == nil || err.Error() != transport.SiteTasksCronSetting {
			t.Fatalf("%s: %v", line, err)
		}
		if got := readTaskFile(t, sitecron.SourcePath(taskTestSite)); got != before {
			t.Fatalf("%s: the task file changed", line)
		}
		if strings.Contains(sitecron.Parse(line+"\n").EnvFileBody(), line[:5]) {
			t.Fatalf("%s reaches the settings file", line)
		}
	}
}
