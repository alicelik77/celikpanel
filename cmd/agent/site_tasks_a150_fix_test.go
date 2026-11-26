package main

import (
	"errors"
	"os"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.150 fix round after the third review of R2b (F1-F7): a put-back
// whose crontab was written but not read back the same never switches the
// timers on beside it.

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
