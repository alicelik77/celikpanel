package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2b second fix round: the tests of the second review's findings
// (handover r2b/REVIEW-353-FIXES.md, N1-N13). Each fails on 6db8cd368.

// failEnables makes `systemctl enable` fail while on is true.
func failEnables(t *testing.T, on *bool) {
	t.Helper()
	inner := siteTaskSystemctl
	t.Cleanup(func() { siteTaskSystemctl = inner })
	siteTaskSystemctl = func(args ...string) ([]byte, error) {
		if *on && len(args) > 0 && args[0] == "enable" {
			return []byte("Failed to enable unit"), errors.New("exit status 1")
		}
		return inner(args...)
	}
}

// TestUnresolvedMoveBlocksEveryMoveAndUndo (review N1): a move whose
// put-back fails keeps its journal (the only copy of the crontab). Every new
// move and undo - the same site's programs, another site's tasks, an undo -
// is then refused before anything changes, the journal stays byte for byte,
// and the Agent's next start resolves it. On 6db8cd368 the programs part of
// the same request resolved nothing, wrote its own journal and deleted it.
func TestUnresolvedMoveBlocksEveryMoveAndUndo(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	enableFails := true
	failEnables(t, &enableFails)
	host.installFails = true
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksUnknown {
		t.Fatalf("move %+v", resp)
	}
	kept, err := os.ReadFile(siteMoveJournalPath)
	if err != nil {
		t.Fatal("the journal is gone")
	}
	var moved transport.SiteTasksMovedResponse
	_ = (&Agent{}).SiteTasksMovedV1(&transport.SiteTasksMovedRequest{SiteIDs: []int{99}}, &moved)
	if u := moved.Unresolved; u == nil || u.SiteID != taskTestSite || u.Kind != transport.SiteMoveUnresolvedTasksMove || u.Account != taskTestAccount {
		t.Fatalf("the unresolved move is not reported: %+v", moved.Unresolved)
	}
	// The same site's programs, in the same request.
	var programs transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{
		Site:           transport.SiteSlice{SiteID: taskTestSite, Domain: "shop.example.test", MemoryMB: 512, Tasks: 128, CPUPercent: 100},
		SubscriptionID: 2, DomainID: 9, Units: []string{"celikrt-12.service"},
	}, &programs)
	if programs.Refusal != transport.SiteMoveRefusalUnresolved || !strings.Contains(programs.Error, "site 7") ||
		!strings.Contains(programs.Error, "systemctl disable --now 'celikcron-7-*.timer'") || programs.Done {
		t.Fatalf("programs %+v", programs)
	}
	// Another site's tasks.
	other := taskTestTarget()
	other.SiteID, other.Account, other.Group = 8, "blog_example_test", "blog_example_test"
	siteTasksTenantTarget = func(*transport.SiteTasksMoveRequest) (siteTaskTarget, string, error) {
		return other, other.Account, nil
	}
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksRefused || resp.Refusal != transport.SiteMoveRefusalUnresolved {
		t.Fatalf("another site's move %+v", resp)
	}
	if resp := undoTasks(t); resp.Outcome != transport.SiteTasksRefused || resp.Refusal != transport.SiteMoveRefusalUnresolved {
		t.Fatalf("an undo %+v", resp)
	}
	if now, _ := os.ReadFile(siteMoveJournalPath); string(now) != string(kept) {
		t.Fatalf("the journal changed:\n%s\n%s", kept, now)
	}
	if _, err := os.Stat(sitecron.SourcePath(8)); !os.IsNotExist(err) {
		t.Fatal("the refused move wrote something")
	}
	// The next start, with crontab and systemctl working again, resolves it
	// (the record says the crontab was emptied: the move is finished).
	enableFails, host.installFails = false, false
	recoverSiteSlicesAtStartup()
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the journal stayed after the next start")
	}
	if _, has := host.crontab(taskTestAccount); has || !host.enabled["celikcron-7-1.timer"] {
		t.Fatalf("crontab %v, timers %v", has, host.enabled)
	}
	moved = transport.SiteTasksMovedResponse{}
	_ = (&Agent{}).SiteTasksMovedV1(&transport.SiteTasksMovedRequest{}, &moved)
	if moved.Unresolved != nil {
		t.Fatalf("still unresolved: %+v", moved.Unresolved)
	}
}

// TestRebuildIsRefusedWhileTheAccountHasACrontab (review N2): scenario A,
// the owner's partial undo by hand (crontab back, task file deleted, timers
// left): the rebuild and its preview are refused with why and who acts, and
// no timer is touched. Scenario B, a put-back that left a unit file the
// owner changed: the screen says the tasks run from the crontab again and
// names the file to remove, not "task file missing". On 6db8cd368 the
// rebuild switched the timers on next to the crontab.
func TestRebuildIsRefusedWhileTheAccountHasACrontab(t *testing.T) {
	t.Run("A_undo_by_hand", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
			t.Fatalf("%+v", resp)
		}
		writeTaskFile(t, spoolPath(taskTestAccount), cronHeader+moveTaskCrontab) // crontab -u acc /etc/celikpanel-cron/7.crontab
		_ = os.Remove(sitecron.SourcePath(7))
		enables := host.count("enable")
		for _, preview := range []bool{true, false} {
			var resp transport.SiteTaskSourceResponse
			_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}, Preview: preview}, &resp)
			if resp.Outcome != transport.SiteTasksRefused || resp.Refusal != transport.SiteTaskSourceRefusalCrontab ||
				!strings.Contains(resp.Error, "twice") || len(resp.LeftUnits) != 2 {
				t.Fatalf("preview=%v: %+v", preview, resp)
			}
		}
		if _, err := os.Stat(sitecron.SourcePath(7)); !os.IsNotExist(err) || host.count("enable") != enables {
			t.Fatal("the refused rebuild changed something")
		}
	})
	t.Run("B_put_back_left_an_owner_unit", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
			t.Fatalf("%+v", resp)
		}
		service := filepath.Join(host.unitDir, "celikcron-7-1.service")
		writeTaskFile(t, service, readTaskFile(t, service)+"# the owner's line\n")
		writeTaskFile(t, spoolPath(taskTestAccount), cronHeader+moveTaskCrontab)
		_ = os.Remove(sitecron.SourcePath(7))
		var list ListCronJobsResponse
		if err := listSiteTasks(taskTestTarget(), &list); err != nil {
			t.Fatal(err)
		}
		if list.Source != transport.SiteTasksSourceCrontab || len(list.LeftUnits) == 0 || !strings.HasSuffix(list.LeftUnits[0], "celikcron-7-1.service") {
			t.Fatalf("list %+v", list)
		}
	})
}

// TestHeldTasksAreSkippedAndComeBackWithTheirLimits (review N4): the task
// service runs only while the site's slice file exists (rendered
// ConditionPathExists=, holding across a reboot and without the panel); a
// run systemd skipped for it shows as skipped, never as failed; and
// CelikPanel's own way of putting the file back writes it, reloads systemd
// and pushes the limits to the running slice before the held timers start
// again (measured: without the reload the next run has no limits).
func TestHeldTasksAreSkippedAndComeBackWithTheirLimits(t *testing.T) {
	host := useTaskTestHost(t)
	slicePath := filepath.Join(host.unitDir, sitesandbox.SliceName(taskTestSite))
	if _, err := writeSiteTaskSource(taskTestTarget(), "0 5 * * * php five.php\n", transport.SiteFileTriggerCreate); err != nil {
		t.Fatal(err)
	}
	if _, err := applySiteTimers(taskTestTarget(), "0 5 * * * php five.php\n"); err != nil {
		t.Fatal(err)
	}
	if service := readTaskFile(t, filepath.Join(host.unitDir, "celikcron-7-1.service")); !strings.Contains(service, "\nConditionPathExists="+slicePath+"\n") {
		t.Fatalf("the service does not wait for its limits file:\n%s", service)
	}
	// A skipped start: success with an unmet condition.
	inner := siteTaskSystemctl
	siteTaskSystemctl = func(args ...string) ([]byte, error) {
		if len(args) > 3 && args[0] == "show" && strings.HasSuffix(args[3], ".service") {
			return []byte("Id=" + args[3] + "\nActiveState=inactive\nResult=success\nExecMainStartTimestamp=@1795287180\nExecMainExitTimestamp=@1795287182\nExecMainStatus=0\n" +
				"InvocationID=abc\nConditionResult=no\nConditionTimestamp=@1795290000\n\n"), nil
		}
		return inner(args...)
	}
	var list ListCronJobsResponse
	if err := listSiteTasks(taskTestTarget(), &list); err != nil || len(list.Jobs) != 1 || list.Jobs[0].Run == nil {
		t.Fatalf("list %+v %v", list, err)
	}
	if run := list.Jobs[0].Run; run.State != transport.SiteTaskRunSkipped || run.StartedAt == "" || run.Result != "" {
		t.Fatalf("a skipped run reads as %+v", run)
	}
	siteTaskSystemctl = inner
	// The file goes; the start holds the timer; the Panel's limits bring it
	// back: written, reloaded, pushed, then the timer starts.
	_ = os.Remove(slicePath)
	if _, held := reconcileSiteTimers(nil); len(held) != 1 {
		t.Fatalf("held %+v", held)
	}
	oldVerify, oldRoot := siteSliceVerify, siteSliceCgroupRoot
	t.Cleanup(func() { siteSliceVerify, siteSliceCgroupRoot = oldVerify, oldRoot })
	siteSliceVerify = func(string) error { return nil }
	siteSliceCgroupRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(siteSliceCgroupRoot, sitesandbox.SliceName(taskTestSite)), 0o755); err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	host.calls = nil
	host.mu.Unlock()
	slice := &transport.SiteSlice{SiteID: taskTestSite, Domain: "shop.example.test", MemoryMB: 512, Tasks: 128, CPUPercent: 100}
	if _, held := reconcileSiteTimers(map[int]*transport.SiteSlice{taskTestSite: slice}); len(held) != 0 {
		t.Fatalf("still held %+v", held)
	}
	order := map[string]int{}
	host.mu.Lock()
	for i, c := range host.calls {
		for _, key := range []string{"daemon-reload", "show --property=Id,MemoryMax", "start celikcron-7-1.timer"} {
			if _, seen := order[key]; !seen && strings.HasPrefix(c, key) {
				order[key] = i + 1
			}
		}
	}
	calls := append([]string{}, host.calls...)
	host.mu.Unlock()
	if order["daemon-reload"] == 0 || order["show --property=Id,MemoryMax"] <= order["daemon-reload"] || order["start celikcron-7-1.timer"] <= order["show --property=Id,MemoryMax"] {
		t.Fatalf("not written, reloaded and pushed before the timer started: %v", calls)
	}
}

// TestMovedSiteWithoutTaskFileStaysOutOfCron (review N6): a moved site whose
// task file is missing still counts as moved: its account is taken from the
// units' User= and stays out of CelikPanel's cron.allow; when it can be read
// from neither, the file is left as it is. On 6db8cd368 the next render let
// the account back in.
func TestMovedSiteWithoutTaskFileStaysOutOfCron(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("%+v", resp)
	}
	_ = os.Remove(sitecron.SourcePath(7))
	// Another account appears, so CelikPanel's file is rendered again.
	raw := readTaskFile(t, sideDoorsPasswd)
	writeTaskFile(t, sideDoorsPasswd, raw+"bob:x:1001:1001::/home/bob:/bin/bash\n")
	refreshCronAllow()
	if allow := readTaskFile(t, cronAllowPath); strings.Contains(allow, taskTestAccount) || !strings.Contains(allow, "\nbob\n") {
		t.Fatalf("cron.allow after the task file went:\n%s", allow)
	}
	// Neither the file nor a readable unit: cron.allow is left as it is.
	service := filepath.Join(host.unitDir, "celikcron-7-1.service")
	writeTaskFile(t, service, strings.ReplaceAll(readTaskFile(t, service), "User="+taskTestAccount, "User=root"))
	before := readTaskFile(t, cronAllowPath)
	writeTaskFile(t, sideDoorsPasswd, raw+"bob:x:1001:1001::/home/bob:/bin/bash\ncarol:x:1002:1002::/home/carol:/bin/bash\n")
	out := closeSiteCronWith(hostSiteCronOps(), 0)
	if readTaskFile(t, cronAllowPath) != before || !strings.Contains(out.Detail, "left as it is") {
		t.Fatalf("cron.allow was rendered without the moved site's account: %+v\n%s", out, readTaskFile(t, cronAllowPath))
	}
}

// TestTimerThatCouldNotBeSwitchedOnIsRetried (review N7): a save whose
// `systemctl enable` fails is not recorded as applied; the task reads "could
// not be switched on yet" (never "turned off on the server"); the next start
// (or save) switches it on. On 6db8cd368 the digest was recorded, the
// reconcile skipped the site and a second save never enabled the timer.
func TestTimerThatCouldNotBeSwitchedOnIsRetried(t *testing.T) {
	host := useTaskTestHost(t)
	if _, err := writeSiteTaskSource(taskTestTarget(), "0 5 * * * php five.php\n", transport.SiteFileTriggerCreate); err != nil {
		t.Fatal(err)
	}
	enableFails := true
	failEnables(t, &enableFails)
	if _, err := applySiteTimers(taskTestTarget(), "0 5 * * * php five.php\n"); err == nil {
		t.Fatal("the failed enable was not answered")
	}
	if readSandboxLedger()[siteTaskAppliedKey(taskTestSite)].RenderSHA256 != "" {
		t.Fatal("recorded as applied with its timer off")
	}
	var list ListCronJobsResponse
	if err := listSiteTasks(taskTestTarget(), &list); err != nil || len(list.Jobs) != 1 || list.Jobs[0].TaskState != transport.SiteTaskNotOnYet {
		t.Fatalf("state %+v %v", list.Jobs, err)
	}
	// The next start: enable works again.
	enableFails = false
	if n, held := reconcileSiteTimers(nil); n != 1 || len(held) != 0 {
		t.Fatalf("reconcile %d %+v", n, held)
	}
	if !host.enabled["celikcron-7-1.timer"] || !host.active["celikcron-7-1.timer"] {
		t.Fatalf("not switched on: %v", host.enabled)
	}
	list = ListCronJobsResponse{}
	if err := listSiteTasks(taskTestTarget(), &list); err != nil || list.Jobs[0].TaskState != transport.SiteTaskScheduled {
		t.Fatalf("state after the retry %+v %v", list.Jobs, err)
	}
	if len(readSiteTimersToEnable()) != 0 {
		t.Fatalf("still remembered: %v", readSiteTimersToEnable())
	}
	// The owner's own disable stays "turned off", and is never retried.
	host.mu.Lock()
	delete(host.enabled, "celikcron-7-1.timer")
	delete(host.active, "celikcron-7-1.timer")
	host.mu.Unlock()
	list = ListCronJobsResponse{}
	if err := listSiteTasks(taskTestTarget(), &list); err != nil || list.Jobs[0].TaskState != transport.SiteTaskOff {
		t.Fatalf("the owner's disable reads %+v %v", list.Jobs, err)
	}
}

// TestUndoRemovesOnlyTheSpoolFileItWrote (review N9): when the crontab the
// undo wrote cannot be confirmed, only a spool file holding exactly what it
// wrote is taken away; another one (the site's own code wrote it while cron
// was open) is left and named. On 6db8cd368 every spool file was removed.
func TestUndoRemovesOnlyTheSpoolFileItWrote(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	if resp := moveTasks(t); resp.Outcome != transport.SiteTasksMoved {
		t.Fatalf("%+v", resp)
	}
	other := filepath.Join(t.TempDir(), taskTestAccount)
	siteCronSpoolDirs = append(siteCronSpoolDirs, filepath.Dir(other))
	// The undo's crontab lands in the first spool; while cron is open the
	// site's own code writes another one in the second, and reading the
	// crontab back fails once.
	inner, innerList := cronInstallCrontab, cronListCrontab
	t.Cleanup(func() { cronInstallCrontab, cronListCrontab = inner, innerList })
	failRead := false
	cronInstallCrontab = func(username, content string) error {
		if err := inner(username, content); err != nil {
			return err
		}
		failRead = true
		return os.WriteFile(other, []byte("* * * * * ~/mine\n"), 0o600)
	}
	cronListCrontab = func(username string) ([]byte, string, int, error) {
		if failRead {
			failRead = false
			return nil, "crontab: read error", 1, errors.New("exit status 1")
		}
		return innerList(username)
	}
	resp := undoTasks(t)
	if resp.Outcome != transport.SiteTasksRefused || !strings.Contains(resp.Error, other) {
		t.Fatalf("undo %+v", resp)
	}
	if raw, err := os.ReadFile(other); err != nil || string(raw) != "* * * * * ~/mine\n" {
		t.Fatalf("the site's own crontab was removed: %v", err)
	}
	if _, err := os.Stat(spoolPath(taskTestAccount)); !os.IsNotExist(err) {
		t.Fatal("the crontab the undo wrote was not taken away again")
	}
	if host.enabledTimers() == 0 {
		t.Fatal("the timers were not switched on again")
	}
}

// TestMoveFailsWhenCrontabProcessesCannotBeListed (review N10): when the
// scan that ends the account's crontab processes fails, the move fails at
// that step: cron opens again, nothing was removed. On 6db8cd368 the failure
// was only logged and the move went on.
func TestMoveFailsWhenCrontabProcessesCannotBeListed(t *testing.T) {
	host := useTaskTestHost(t)
	host.startWith(t, moveTaskCrontab)
	siteTaskEndCrontabs = func(int) (int, error) { return 0, errors.New("open /proc: permission denied") }
	resp := moveTasks(t)
	if resp.Outcome != transport.SiteTasksPutBack || !strings.Contains(resp.Error, "cron_closed") || !strings.Contains(resp.Error, "/proc") {
		t.Fatalf("move %+v", resp)
	}
	if got, _ := host.crontab(taskTestAccount); got != moveTaskCrontab || host.refused(taskTestAccount) || len(host.unitFiles(t)) != 0 {
		t.Fatalf("crontab %q refused=%v units %v", got, host.refused(taskTestAccount), host.unitFiles(t))
	}
}

// TestSiteAccountsServiceManagerIsMasked (review N11): every site account's
// user@<uid>.service is masked (CelikPanel's own link, recorded); a file the
// owner put there is kept and said; the owner's removal of CelikPanel's
// link stands; a deleted site's mask goes; an administrator's account (or
// a uid one shares) is never masked. On 6db8cd368 nothing was masked.
func TestSiteAccountsServiceManagerIsMasked(t *testing.T) {
	useTaskTestHost(t)
	units, _ := sandboxUnitDir()
	reloads := 0
	oldReload, oldGroup := sandboxDaemonReload, sideDoorsGroup
	t.Cleanup(func() { sandboxDaemonReload, sideDoorsGroup = oldReload, oldGroup })
	sandboxDaemonReload = func() error { reloads++; return nil }
	sideDoorsGroup = filepath.Join(t.TempDir(), "group")
	writeTaskFile(t, sideDoorsGroup, "root:x:0:\nsudo:x:27:anna,adm2\n")
	site := func(name string, uid string) string {
		return name + ":x:" + uid + ":" + uid + "::" + hostingpath.SubscriptionsRoot() + "/3/sites/" + uid + ":/usr/sbin/nologin\n"
	}
	writeTaskFile(t, sideDoorsPasswd, "root:x:0:0:root:/root:/bin/bash\nanna:x:1000:1000::/home/anna:/bin/bash\n"+
		site("shop_a", "1501")+site("shop_b", "1502")+site("shop_c", "1503")+
		// An administrator who shares a site's uid: never masked.
		site("shop_d", "1504")+"adm2:x:1504:1504::/home/adm2:/bin/bash\n")
	mask := func(uid string) string { return filepath.Join(units, "user@"+uid+".service") }
	// The owner's own file for shop_b.
	writeTaskFile(t, mask("1502"), "[Service]\n# the owner's\n")
	out := closeSiteUserManagers()
	for _, uid := range []string{"1501", "1503"} {
		if target, err := os.Readlink(mask(uid)); err != nil || target != "/dev/null" {
			t.Fatalf("user@%s.service not masked: %v", uid, err)
		}
	}
	if readTaskFile(t, mask("1502")) != "[Service]\n# the owner's\n" || len(out.Owner) != 1 || out.Owner[0] != "shop_b" {
		t.Fatalf("the owner's file: %+v", out)
	}
	for _, uid := range []string{"1000", "1504", "0"} {
		if _, err := os.Lstat(mask(uid)); !os.IsNotExist(err) {
			t.Fatalf("user@%s.service was masked (not a site account, or an administrator's)", uid)
		}
	}
	if out.Masked != 2 || reloads != 1 {
		t.Fatalf("masked %d, reloads %d", out.Masked, reloads)
	}
	// The owner removes CelikPanel's link for shop_c: it stays removed, said.
	_ = os.Remove(mask("1503"))
	out = closeSiteUserManagers()
	if _, err := os.Lstat(mask("1503")); !os.IsNotExist(err) || len(out.Removed) != 1 || out.Removed[0] != "shop_c" {
		t.Fatalf("the owner's removal did not stand: %+v", out)
	}
	// shop_a's site is deleted: its mask goes with it.
	writeTaskFile(t, sideDoorsPasswd, "root:x:0:0:root:/root:/bin/bash\nanna:x:1000:1000::/home/anna:/bin/bash\n"+site("shop_b", "1502")+site("shop_c", "1503"))
	if err := syncSiteUserManagers(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(mask("1501")); !os.IsNotExist(err) {
		t.Fatal("a deleted site's mask stayed")
	}
	if _, ok := readSandboxLedger()[mask("1501")]; ok {
		t.Fatal("a deleted site's mask is still recorded")
	}
}

// TestPrivilegedFlagOnlyInCommandPosition (review N13, A3): a program that
// needs extra rights is flagged only where it is called - the start of the
// command, or after ;, &&, ||, | - by its basename; the server's sendmail and
// the mail commands count (inside a task they run without their rights). On
// 6db8cd368 "echo backup at noon" was flagged and sendmail was not.
func TestPrivilegedFlagOnlyInCommandPosition(t *testing.T) {
	for command, want := range map[string]bool{
		`echo "backup at $(date)" >> log`:    false,
		"echo write log; ls /usr/bin/at":     false,
		"cd /var/www && ./su-report 2>&1":    false,
		"php artisan schedule:run":           false,
		"grep sudo /var/log/x | wc -l":       false,
		"sudo -u x true":                     true,
		"/usr/bin/crontab -l > ~/list":       true,
		"cd x; at now + 1 minute":            true,
		"php r.php | /usr/sbin/sendmail -t":  true,
		"mail -s hi a@example.test < report": true,
		"FOO=1 ping -c1 example.test":        true,
		"nohup sudo x &":                     true,
		"test -f x || mailx -s down a@b":     true,
		`echo "a; sudo x"`:                   false,
	} {
		if got := privilegedCommand(command); got != want {
			t.Errorf("%q: flagged %v, want %v", command, got, want)
		}
	}
}
