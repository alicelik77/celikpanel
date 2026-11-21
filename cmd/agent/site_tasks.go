package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A site's scheduled tasks as systemd timers in the site's own slice (S4
// R2b, 22 Nov 2026; docs/SITE-SANDBOX.md section 9).
//
// A moved site (every new site, and an existing one after the
// administrator's "Move into the limits") keeps its tasks in one source,
// /etc/celikpanel-cron/<site id>.crontab: crontab syntax under CelikPanel's
// render header (D-031: an owner's edit is kept and used, CelikPanel's own
// change then waits beside it as <file>.celikpanel-pending). Every enabled
// task line becomes celikcron-<site id>-<n>.service and .timer (package
// sitecron); the command and its input are files the site's account reads,
// never a unit's settings (systemd shows those to every account). A task
// keeps its number while its text stays the same (the "# celikpanel-task:"
// line of its service names it), so its last run stays its own.
//
// What is written is only what changed; a new timer is enabled and started
// (an @reboot one only enabled: it runs after the next start of the server,
// as cron runs @reboot lines when it starts); a changed timer is restarted
// only when it runs (an owner who stopped or disabled it keeps it so); a
// task removed from the source loses its timer (stopped and disabled), and
// its files go when they are still CelikPanel's; a running task is never
// stopped by a change (cron does not stop one either).
//
// Every change of a site's tasks (the Scheduled tasks screen, the move, its
// undo, the reconcile at each start) holds cronMu, the lock every crontab
// change takes.
//
// Taşınmış bir sitenin görevleri tek kaynak dosyadan systemd
// zamanlayıcılarına çevrilir; komut ve girdisi, birimin ayarlarında değil,
// sitenin hesabının okuyabildiği dosyalardadır.

// File kinds (the ledger and D-031 results).
const (
	siteTaskSourceKind  = "site_task_source"
	siteTaskScriptKind  = "site_task_script"
	siteTaskEnvKind     = "site_task_env"
	siteTaskInputKind   = "site_task_input"
	siteTaskServiceKind = "celikcron_service"
	siteTaskTimerKind   = "celikcron_timer"
)

// Seams for tests.
var (
	// siteTaskSystemctl runs systemctl.
	siteTaskSystemctl = func(args ...string) ([]byte, error) { return appUnitSystemctl(systemctlArgs(args...)...) }
	// siteTaskJournal returns the last lines one run wrote.
	siteTaskJournal = func(invocation string, lines int) ([]string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "journalctl", "_SYSTEMD_INVOCATION_ID="+invocation, "-o", "cat", "-n", strconv.Itoa(lines), "--no-pager", "-q")
		cmd.Env = append(os.Environ(), "LC_ALL=C", "SYSTEMD_COLORS=0")
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		text := strings.TrimRight(string(out), "\n")
		if text == "" {
			return nil, nil
		}
		return strings.Split(text, "\n"), nil
	}
	// siteTaskLookup reads the site account's ids, group and home.
	siteTaskLookup = func(account string) (uid, gid int, group, home string, err error) {
		u, err := user.Lookup(account)
		if err != nil {
			return 0, 0, "", "", err
		}
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
		g, err := user.LookupGroupId(u.Gid)
		if err != nil {
			return 0, 0, "", "", err
		}
		return uid, gid, g.Name, filepath.Clean(u.HomeDir), nil
	}
	// siteTaskChown sets a task file's group to the site's (root owns it).
	siteTaskChown = func(path string, gid int, mode os.FileMode) error {
		if err := os.Lchown(path, 0, gid); err != nil {
			return err
		}
		return os.Chmod(path, mode)
	}
	// siteTaskVerify is systemd's own check of a written unit.
	siteTaskVerify = func(path string) error {
		if err := appUnitVerify(path); err != nil && !errors.Is(err, services.ErrServiceCheckUnavailable) {
			return err
		}
		return nil
	}
	// siteTaskCaps says whether units add themselves to the SMTP block and
	// whether systemd enforces IPAddressDeny=.
	siteTaskCaps = func() (egress, bpf bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c := sandboxCaps(ctx)
		return c.EgressEnforced(), c.BPFFramework && c.CgroupV2
	}
)

// siteTaskTarget is a proved site account with what its timers need.
type siteTaskTarget struct {
	SiteID   int
	Domain   string
	Account  string
	Group    string
	UID, GID int
	Home     string
	// Slice is the site's slice with its limits (nil: the Panel did not send
	// it; a missing slice file is then left to the next limits change).
	Slice *transport.SiteSlice
}

// siteTaskTargetFor proves the tenant's account (cronTenantUser did) and
// reads what the timers need.
func siteTaskTargetFor(tenant transport.CronTenant, username string) (siteTaskTarget, error) {
	if tenant.SiteID < 1 {
		return siteTaskTarget{}, errors.New("the site's identity is required for its task file")
	}
	uid, gid, group, home, err := siteTaskLookup(username)
	if err != nil {
		return siteTaskTarget{}, fmt.Errorf("look up %s: %w", username, err)
	}
	t := siteTaskTarget{SiteID: tenant.SiteID, Domain: strings.ToLower(strings.TrimSpace(tenant.Domain)), Account: username,
		Group: group, UID: uid, GID: gid, Home: home}
	if tenant.Site != nil && tenant.Site.SiteID == tenant.SiteID {
		t.Slice = tenant.Site
	}
	return t, nil
}

// --- The source ---------------------------------------------------------------

// siteTaskIntroMark starts the lines CelikPanel puts above a site's tasks in
// its source; they are not part of the crontab the screen shows.
const siteTaskIntroMark = "#| "

func siteTaskIntro(t siteTaskTarget) string {
	id := strconv.Itoa(t.SiteID)
	lines := []string{
		"CelikPanel: the scheduled tasks of " + t.Domain + " (site " + id + ", account " + t.Account + "), in crontab form.",
		"They run as systemd timers celikcron-" + id + "-<n> in the site's own limits, not by cron (docs/SITE-SANDBOX.md section 9).",
		"Change them on the site's Scheduled tasks screen. A change made here by hand is kept and used; CelikPanel's",
		"own later changes then wait beside this file as .celikpanel-pending, and the screen says so.",
		"Back to cron: the administrator's \"Put the tasks back into cron\" on the site's Overview.",
		"CelikPanel: sitenin zamanlanmis gorevleri; sitenin kendi sinirlari icinde zamanlayici olarak calisir.",
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(siteTaskIntroMark + line + "\n")
	}
	return b.String()
}

// splitSiteTaskSource returns the crontab a source holds (without the render
// header and the intro) and the account its intro names ("" if none).
func splitSiteTaskSource(raw []byte) (string, string) {
	text := string(raw)
	if strings.HasPrefix(text, services.ManagedHeaderPrefixHash) {
		if newline := strings.IndexByte(text, '\n'); newline >= 0 {
			text = text[newline+1:]
		} else {
			text = ""
		}
	}
	account := ""
	for strings.HasPrefix(text, strings.TrimSpace(siteTaskIntroMark)) {
		newline := strings.IndexByte(text, '\n')
		line := text
		if newline >= 0 {
			line, text = text[:newline], text[newline+1:]
		} else {
			text = ""
		}
		if _, rest, ok := strings.Cut(line, ", account "); ok && account == "" {
			account, _, _ = strings.Cut(rest, ")")
		}
	}
	return text, account
}

// parseSiteTaskIntro reads the domain and the account the intro of a source
// names ("" when there is no intro).
func parseSiteTaskIntro(raw []byte) (domain, account string) {
	_, account = splitSiteTaskSource(raw)
	for _, line := range strings.Split(string(raw), "\n") {
		if _, rest, ok := strings.Cut(line, "the scheduled tasks of "); ok && strings.HasPrefix(line, siteTaskIntroMark) {
			domain, _, _ = strings.Cut(rest, " (site ")
			break
		}
	}
	return domain, account
}

// The applied record: per site, the digest of the crontab its timers were
// last made from (a sandbox ledger entry under a key that is not a path).
const siteTaskAppliedKind = "site_task_applied"

func siteTaskAppliedKey(siteID int) string { return "site-task-applied:" + strconv.Itoa(siteID) }

// reconcileSiteTimers re-renders the timers of every moved site whose task
// file changed since its timers were made (a hand edit by the server owner,
// an interrupted render); run by the side doors step after each Panel start.
// It reads small files only, and renders only a changed site.
func reconcileSiteTimers() (rendered int) {
	ids, _ := movedSiteTaskSites()
	ledger := readSandboxLedger()
	for _, id := range ids {
		path := sitecron.SourcePath(id)
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 1<<20 {
			continue
		}
		crontab, _ := splitSiteTaskSource(raw)
		if ledger[siteTaskAppliedKey(id)].RenderSHA256 == taskDigest(crontab) {
			continue
		}
		domain, account := parseSiteTaskIntro(raw)
		if domain == "" || !cronUsernameRe.MatchString(account) || account == "root" {
			log.Printf("site tasks: %s names no site account; its timers are left as they are", path)
			continue
		}
		uid, gid, group, home, err := siteTaskLookup(account)
		if err != nil || uid < 1000 {
			log.Printf("site tasks: the account of %s cannot be used (%v); its timers are left as they are", path, err)
			continue
		}
		t := siteTaskTarget{SiteID: id, Domain: domain, Account: account, Group: group, UID: uid, GID: gid, Home: home}
		cronMu.Lock()
		if _, err := applySiteTimers(t, crontab); err != nil {
			log.Printf("site tasks: the timers of site %d could not all be written: %v", id, err)
		} else {
			rendered++
		}
		cronMu.Unlock()
	}
	return rendered
}

func siteTaskSourceManaged(t siteTaskTarget, crontab, trigger string) services.ManagedSiteFile {
	return services.ManagedSiteFile{Kind: siteTaskSourceKind, Path: sitecron.SourcePath(t.SiteID), HeaderPrefix: services.ManagedHeaderPrefixHash,
		Body: siteTaskIntro(t) + crontab, Trigger: trigger}
}

// siteTaskSource is a site's source as read.
type siteTaskSource struct {
	Exists  bool
	Crontab string
	// State is the D-031 state (managed_unchanged, owner_edited, foreign...).
	State   string
	Account string
	Pending string
}

// readSiteTaskSource reads a site's source; a link or a non-regular file is
// an error (never followed).
func readSiteTaskSource(siteID int) (siteTaskSource, error) {
	path := sitecron.SourcePath(siteID)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return siteTaskSource{}, nil
	}
	if err != nil {
		return siteTaskSource{}, err
	}
	if !info.Mode().IsRegular() {
		return siteTaskSource{Exists: true, State: transport.SiteFileUnreadable}, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > 1<<20 {
		return siteTaskSource{Exists: true, State: transport.SiteFileUnreadable}, fmt.Errorf("%s is larger than 1 MiB", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return siteTaskSource{Exists: true, State: transport.SiteFileUnreadable}, err
	}
	crontab, account := splitSiteTaskSource(raw)
	inspected, _, _ := services.InspectManagedSiteFile(services.ManagedSiteFile{Kind: siteTaskSourceKind, Path: path, HeaderPrefix: services.ManagedHeaderPrefixHash})
	return siteTaskSource{Exists: true, Crontab: crontab, State: inspected.State, Account: account, Pending: inspected.PendingPath}, nil
}

// ensureSiteTaskDirs makes /etc/celikpanel-cron (0711: an account can pass
// it but not list it) and the site's task folder (root with the site's
// group, 0750). A link in their place is refused.
func ensureSiteTaskDirs(t siteTaskTarget) error {
	if err := ensureRealDir(sitecron.SourceDir, 0o711, -1); err != nil {
		return err
	}
	return ensureRealDir(sitecron.TaskDir(t.SiteID), 0o750, t.GID)
}

func ensureRealDir(dir string, mode os.FileMode, gid int) error {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.Mkdir(dir, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s is not a folder", dir)
	}
	if gid >= 0 {
		if err := siteTaskChown(dir, gid, mode); err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		return nil
	}
	return os.Chmod(dir, mode)
}

// writeSiteTaskSource writes the source under the D-031 rule.
func writeSiteTaskSource(t siteTaskTarget, crontab, trigger string) (transport.SiteFileResult, error) {
	if err := ensureSiteTaskDirs(t); err != nil {
		return transport.SiteFileResult{Outcome: transport.SiteFileOutcomeFailed}, err
	}
	if crontab != "" && !strings.HasSuffix(crontab, "\n") {
		crontab += "\n"
	}
	return services.ApplyManagedSiteFile(siteTaskSourceManaged(t, crontab, trigger), nil, nil)
}

// --- The units ------------------------------------------------------------------

// siteTaskUnitFile is one existing task unit pair of a site.
type siteTaskUnitFile struct {
	N       int
	TaskKey string
	Service bool
	Timer   bool
}

// siteTaskUnits lists a site's task units as their files say.
func siteTaskUnits(siteID int) (map[int]*siteTaskUnitFile, error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[int]*siteTaskUnitFile{}
	prefix := sitecron.UnitPrefix + strconv.Itoa(siteID) + "-"
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		site, n, suffix, ok := sitecron.ParseUnit(e.Name())
		if !ok || site != siteID {
			continue
		}
		u := out[n]
		if u == nil {
			u = &siteTaskUnitFile{N: n}
			out[n] = u
		}
		if suffix == "timer" {
			u.Timer = true
			continue
		}
		u.Service = true
		for _, line := range strings.Split(readSmallFile(filepath.Join(dir, e.Name())), "\n") {
			if key, ok := strings.CutPrefix(line, sitecron.TaskMarker); ok {
				u.TaskKey = strings.TrimSpace(key)
				break
			}
		}
	}
	return out, nil
}

// siteTaskUnitsExist: a site has any task unit file.
func siteTaskUnitsExist(siteID int) bool {
	units, err := siteTaskUnits(siteID)
	return err == nil && len(units) > 0
}

// siteTasksMoved: the site runs its tasks as timers (its source is there, or
// timers of it are while the source is gone).
func siteTasksMoved(siteID int) bool {
	if siteID < 1 {
		return false
	}
	if _, err := os.Lstat(sitecron.SourcePath(siteID)); err == nil {
		return true
	}
	return siteTaskUnitsExist(siteID)
}

// siteTaskKeys gives every enabled job its key: the task ID, and "#k" for
// the k-th copy of the same text.
func siteTaskKeys(jobs []sitecron.Entry) []string {
	seen := map[string]int{}
	keys := make([]string, len(jobs))
	for i, job := range jobs {
		id := generateCronID(job.Text)
		seen[id]++
		keys[i] = id
		if seen[id] > 1 {
			keys[i] = id + "#" + strconv.Itoa(seen[id])
		}
	}
	return keys
}

// siteTimerTask is one task as applied.
type siteTimerTask struct {
	Entry sitecron.Entry
	Key   string
	N     int
	Unit  string
	// State is transport.SiteTask*.
	State string
}

// siteTimersResult is what applySiteTimers did.
type siteTimersResult struct {
	Tasks  []siteTimerTask
	NotRun []transport.SiteTaskLine
	// Kept: unit or task files the server owner changed (kept, with
	// CelikPanel's text pending beside them).
	Kept []string
	// Units are the site's unit files written now (the move checks them).
	Written []string
}

// managedTaskFile is one file applySiteTimers puts in place.
type managedTaskFile struct {
	kind, path, body string
	unit             bool
	header           bool
	group            bool
}

// applySiteTimers renders and applies every timer of a site from its
// crontab (the caller holds cronMu). It stops at the first file that could
// not be written or that systemd refuses and returns that error; what was
// written before stays (it is CelikPanel's text for the same source and is
// applied again at the next change or start).
func applySiteTimers(t siteTaskTarget, crontab string) (siteTimersResult, error) {
	var result siteTimersResult
	c := sitecron.Parse(crontab)
	for _, bad := range c.Untranslatable() {
		result.NotRun = append(result.NotRun, transport.SiteTaskLine{Line: bad.Line, Text: bad.Raw, Reason: bad.Reason, Detail: bad.Detail})
	}
	jobs := c.Jobs()
	if len(jobs) > sitecron.MaxTasks {
		return result, fmt.Errorf("a site has at most %d tasks; this one has %d", sitecron.MaxTasks, len(jobs))
	}
	existing, err := siteTaskUnits(t.SiteID)
	if err != nil {
		return result, err
	}
	byKey, next := map[string]int{}, 1
	for n, u := range existing {
		if u.TaskKey != "" {
			if _, taken := byKey[u.TaskKey]; !taken {
				byKey[u.TaskKey] = n
			}
		}
		if n >= next {
			next = n + 1
		}
	}
	keys := siteTaskKeys(jobs)
	wanted := map[int]bool{}
	for i, job := range jobs {
		n, ok := byKey[keys[i]]
		if !ok {
			if next > 9999 {
				return result, errors.New("the site's task numbers are used up; remove its timers and save again")
			}
			n = next
			next++
		}
		wanted[n] = true
		result.Tasks = append(result.Tasks, siteTimerTask{Entry: job, Key: keys[i], N: n, Unit: sitecron.UnitBase(t.SiteID, n)})
	}
	if t.Slice != nil && len(jobs) > 0 {
		ensureSiteSlice(t.Slice)
	}
	if len(jobs) > 0 || len(c.Env) > 0 {
		if err := ensureSiteTaskDirs(t); err != nil {
			return result, err
		}
	}
	egress, bpf := siteTaskCaps()
	dir, err := sandboxUnitDir()
	if err != nil {
		return result, err
	}
	var files []managedTaskFile
	hasEnv := len(c.Env) > 0
	if hasEnv {
		files = append(files, managedTaskFile{kind: siteTaskEnvKind, path: sitecron.EnvPath(t.SiteID), body: c.EnvFileBody(), header: true})
	}
	var units []managedTaskFile
	for _, task := range result.Tasks {
		files = append(files, managedTaskFile{kind: siteTaskScriptKind, path: sitecron.ScriptPath(t.SiteID, task.N),
			body: sitecron.ScriptText(task.N, task.Entry.Line, task.Entry.Command), header: true, group: true})
		if task.Entry.HasInput {
			files = append(files, managedTaskFile{kind: siteTaskInputKind, path: sitecron.InputPath(t.SiteID, task.N), body: task.Entry.Input, group: true})
		}
		u := sitecron.Unit{SiteID: t.SiteID, N: task.N, TaskID: task.Key, Domain: t.Domain, Account: t.Account, Group: t.Group, Home: t.Home,
			Shell: c.Shell, HasEnv: hasEnv, HasInput: task.Entry.HasInput, Spec: task.Entry.Spec, Line: task.Entry.Line, Egress: egress, BPF: bpf}
		service, err := sitecron.ServiceText(u)
		if err != nil {
			return result, fmt.Errorf("task on line %d: %w", task.Entry.Line, err)
		}
		timer, err := sitecron.TimerText(u)
		if err != nil {
			return result, fmt.Errorf("task on line %d: %w", task.Entry.Line, err)
		}
		units = append(units,
			managedTaskFile{kind: siteTaskServiceKind, path: filepath.Join(dir, task.Unit+".service"), body: service, unit: true, header: true},
			managedTaskFile{kind: siteTaskTimerKind, path: filepath.Join(dir, task.Unit+".timer"), body: timer, unit: true, header: true})
	}
	// Files a unit names come first, so systemd's check of the unit sees them.
	files = append(files, units...)
	reload := false
	newTimers, changedTimers := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		outcome, existed, err := applyTaskFile(f, t.GID)
		if err != nil {
			return result, err
		}
		switch outcome {
		case transport.SiteFileOutcomeKept:
			result.Kept = append(result.Kept, f.path)
		case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated:
			if f.unit {
				reload = true
				result.Written = append(result.Written, f.path)
				if f.kind == siteTaskTimerKind {
					if existed {
						changedTimers[filepath.Base(f.path)] = true
					} else {
						newTimers[filepath.Base(f.path)] = true
					}
				}
			}
		}
	}
	if !hasEnv {
		if removed, kept := removeTaskFile(siteTaskEnvKind, sitecron.EnvPath(t.SiteID), true); kept {
			result.Kept = append(result.Kept, sitecron.EnvPath(t.SiteID))
		} else if removed {
			reload = true
		}
	}
	// Tasks no longer in the source.
	var gone []int
	for n := range existing {
		if !wanted[n] {
			gone = append(gone, n)
		}
	}
	sort.Ints(gone)
	for _, n := range gone {
		base := sitecron.UnitBase(t.SiteID, n)
		timerPath, servicePath := filepath.Join(dir, base+".timer"), filepath.Join(dir, base+".service")
		if siteTaskFileKept(siteTaskTimerKind, timerPath) || siteTaskFileKept(siteTaskServiceKind, servicePath) {
			result.Kept = append(result.Kept, base)
			continue
		}
		if out, err := siteTaskSystemctl("disable", "--now", base+".timer"); err != nil && existing[n].Timer {
			log.Printf("site tasks: %s.timer could not be stopped and disabled: %v: %s", base, err, strings.TrimSpace(string(out)))
		}
		for _, f := range []struct{ kind, path string }{
			{siteTaskTimerKind, timerPath}, {siteTaskServiceKind, servicePath},
			{siteTaskScriptKind, sitecron.ScriptPath(t.SiteID, n)}, {siteTaskInputKind, sitecron.InputPath(t.SiteID, n)},
		} {
			if removed, _ := removeTaskFile(f.kind, f.path, f.kind != siteTaskInputKind); removed {
				reload = true
			}
		}
	}
	for _, task := range result.Tasks {
		if !task.Entry.HasInput {
			_, _ = removeTaskFile(siteTaskInputKind, sitecron.InputPath(t.SiteID, task.N), false)
		}
	}
	if reload {
		if err := siteSliceDaemonReload(); err != nil {
			return result, err
		}
	}
	defer func() {
		// The crontab these timers were made from, so the reconcile at each
		// start re-renders only a site whose task file changed since.
		if err := setSandboxLedgerEntry(siteTaskAppliedKey(t.SiteID), siteTaskAppliedKind, taskDigest(crontab)); err != nil {
			log.Printf("site tasks: record what site %d's timers were made from: %v", t.SiteID, err)
		}
	}()
	for i := range result.Tasks {
		task := &result.Tasks[i]
		timer := task.Unit + ".timer"
		switch {
		case newTimers[timer] && siteTimersHoldEnable:
			// The move enables them after the crontab is empty.
		case newTimers[timer] && task.Entry.Spec.Reboot:
			if out, err := siteTaskSystemctl("enable", timer); err != nil {
				return result, fmt.Errorf("systemctl enable %s: %v: %s", timer, err, strings.TrimSpace(string(out)))
			}
		case newTimers[timer]:
			if out, err := siteTaskSystemctl("enable", "--now", timer); err != nil {
				return result, fmt.Errorf("systemctl enable --now %s: %v: %s", timer, err, strings.TrimSpace(string(out)))
			}
		case changedTimers[timer]:
			// Only a running timer is restarted: one the owner stopped or
			// disabled stays so.
			if out, err := siteTaskSystemctl("try-restart", timer); err != nil {
				log.Printf("site tasks: %s could not be restarted with its new time: %v: %s", timer, err, strings.TrimSpace(string(out)))
			}
		}
	}
	return result, nil
}

// applyTaskFile puts one file in place: under the D-031 rule with
// CelikPanel's header, or (the input, whose bytes are the task's standard
// input) by the ledger's digest. existed: a file was there before.
func applyTaskFile(f managedTaskFile, gid int) (outcome string, existed bool, err error) {
	if _, statErr := os.Lstat(f.path); statErr == nil {
		existed = true
	}
	if !f.header {
		return applyDerivedTaskFile(f, gid, existed)
	}
	m := services.ManagedSiteFile{Kind: f.kind, Path: f.path, HeaderPrefix: services.ManagedHeaderPrefixHash, Body: f.body, Trigger: transport.SiteFileTriggerCreate}
	var validate func() error
	if f.unit {
		path := f.path
		validate = func() error { return siteTaskVerify(path) }
	}
	result, err := services.ApplyManagedSiteFile(m, validate, nil)
	if err != nil {
		return result.Outcome, existed, fmt.Errorf("%s: %w", f.path, err)
	}
	switch result.Outcome {
	case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
		return result.Outcome, existed, fmt.Errorf("%s was not written (%s %s %s)", f.path, result.Outcome, result.Reason, result.Detail)
	case transport.SiteFileOutcomeKept:
		log.Printf("site tasks: %s was changed on this server and is kept; CelikPanel's text is in %s", f.path, result.PendingPath)
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated:
		if f.group && !existed {
			if err := siteTaskChown(f.path, gid, 0o640); err != nil {
				return result.Outcome, existed, fmt.Errorf("%s: %w", f.path, err)
			}
		}
	}
	return result.Outcome, existed, nil
}

func taskDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// applyDerivedTaskFile writes a headerless task file (the input). Its digest
// is kept in the sandbox ledger: a file that holds neither what CelikPanel
// last wrote nor the new text was changed on the server and is kept.
func applyDerivedTaskFile(f managedTaskFile, gid int, existed bool) (string, bool, error) {
	want := taskDigest(f.body)
	if existed {
		info, err := os.Lstat(f.path)
		if err != nil || !info.Mode().IsRegular() {
			return transport.SiteFileOutcomeRefused, existed, fmt.Errorf("%s is not a regular file", f.path)
		}
		raw, err := os.ReadFile(f.path)
		if err != nil {
			return transport.SiteFileOutcomeRefused, existed, err
		}
		current := taskDigest(string(raw))
		if current == want {
			return transport.SiteFileOutcomeUnchanged, existed, nil
		}
		if recorded := readSandboxLedger()[f.path].RenderSHA256; recorded != current {
			log.Printf("site tasks: %s was changed on this server and is kept", f.path)
			return transport.SiteFileOutcomeKept, existed, nil
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.path), "."+filepath.Base(f.path)+".celikpanel-")
	if err != nil {
		return transport.SiteFileOutcomeFailed, existed, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = tmp.WriteString(f.body)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = siteTaskChown(name, gid, 0o640)
	}
	if err == nil {
		err = os.Rename(name, f.path)
	}
	if err != nil {
		return transport.SiteFileOutcomeFailed, existed, fmt.Errorf("%s: %w", f.path, err)
	}
	if err := setSandboxLedgerEntry(f.path, f.kind, want); err != nil {
		log.Printf("site tasks: record %s: %v", f.path, err)
	}
	if existed {
		return transport.SiteFileOutcomeWritten, existed, nil
	}
	return transport.SiteFileOutcomeRecreated, existed, nil
}

// siteTaskFileKept: the file is there and is not CelikPanel's unchanged text.
func siteTaskFileKept(kind, path string) bool {
	if _, err := os.Lstat(path); err != nil {
		return false
	}
	inspected, _, _ := services.InspectManagedSiteFile(services.ManagedSiteFile{Kind: kind, Path: path, HeaderPrefix: services.ManagedHeaderPrefixHash})
	return inspected.State != transport.SiteFileManagedUnchanged
}

// removeTaskFile removes a task file that is still CelikPanel's (header: its
// sealed text; otherwise the ledger's digest). kept: it was changed on the
// server and stays.
func removeTaskFile(kind, path string, header bool) (removed, kept bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, false
	}
	if !info.Mode().IsRegular() {
		return false, true
	}
	if header {
		if siteTaskFileKept(kind, path) {
			return false, true
		}
	} else {
		raw, err := os.ReadFile(path)
		if err != nil || readSandboxLedger()[path].RenderSHA256 != taskDigest(string(raw)) {
			return false, true
		}
	}
	if err := os.Remove(path); err != nil {
		log.Printf("site tasks: remove %s: %v", path, err)
		return false, true
	}
	services.RemoveSiteFilePending(path)
	_ = updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if _, ok := l[path]; !ok {
			return false
		}
		delete(l, path)
		return true
	})
	return true, false
}

// --- What the screen shows ----------------------------------------------------

// siteTaskShowServiceProps and siteTaskShowTimerProps are what a task's
// last run and state are read from.
var (
	siteTaskShowServiceProps = []string{"Id", "ActiveState", "Result", "ExecMainStartTimestamp", "ExecMainExitTimestamp", "ExecMainStatus", "ExecMainCode", "InvocationID"}
	siteTaskShowTimerProps   = []string{"Id", "LoadState", "ActiveState", "UnitFileState", "NextElapseUSecRealtime"}
)

// siteTaskOutputLines and siteTaskOutputBytes bound the output a run shows;
// siteTaskOutputTasks bounds the journal reads of one list.
const (
	siteTaskOutputLines = 10
	siteTaskOutputBytes = 300
	siteTaskOutputTasks = 100
)

func siteTaskShow(names []string, props []string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for start := 0; start < len(names); start += 50 {
		end := start + 50
		if end > len(names) {
			end = len(names)
		}
		args := append([]string{"show", "--timestamp=unix", "--property=" + strings.Join(props, ",")}, names[start:end]...)
		raw, err := siteTaskSystemctl(args...)
		if err != nil {
			continue
		}
		for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
			p := parseShow(block)
			if id := p["Id"]; id != "" {
				out[id] = p
			}
		}
	}
	return out
}

// unixStamp reads systemctl's "@<seconds>" (--timestamp=unix) as RFC 3339.
func unixStamp(value string) (string, time.Time) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@"))
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return "", time.Time{}
	}
	at := time.Unix(seconds, 0).UTC()
	return at.Format(time.RFC3339), at
}

// boundedLine cuts a line at max bytes without splitting a character.
func boundedLine(line string, max int) string {
	if len(line) <= max {
		return line
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + "…"
}

// siteTaskRuns fills each task's state and last run.
func siteTaskRuns(tasks []siteTimerTask, kept []string) map[string]*transport.SiteTaskRun {
	var services, timers []string
	for _, task := range tasks {
		services = append(services, task.Unit+".service")
		timers = append(timers, task.Unit+".timer")
	}
	serviceProps := siteTaskShow(services, siteTaskShowServiceProps)
	timerProps := siteTaskShow(timers, siteTaskShowTimerProps)
	keptSet := map[string]bool{}
	for _, k := range kept {
		keptSet[filepath.Base(k)] = true
	}
	runs := map[string]*transport.SiteTaskRun{}
	journals := 0
	for i := range tasks {
		task := &tasks[i]
		tp := timerProps[task.Unit+".timer"]
		switch {
		case keptSet[task.Unit+".service"] || keptSet[task.Unit+".timer"]:
			task.State = transport.SiteTaskKept
		case tp == nil || tp["LoadState"] != "loaded":
			task.State = transport.SiteTaskWaiting
		case task.Entry.Spec.Reboot && tp["UnitFileState"] == "enabled":
			task.State = transport.SiteTaskAtBoot
		case tp["UnitFileState"] != "enabled" || (!task.Entry.Spec.Reboot && tp["ActiveState"] != "active"):
			task.State = transport.SiteTaskOff
		default:
			task.State = transport.SiteTaskScheduled
		}
		run := &transport.SiteTaskRun{State: "never"}
		if next, _ := unixStamp(tp["NextElapseUSecRealtime"]); next != "" {
			run.NextAt = next
		}
		sp := serviceProps[task.Unit+".service"]
		started, startAt := unixStamp(sp["ExecMainStartTimestamp"])
		if sp != nil && started != "" {
			run.StartedAt = started
			ended, endAt := unixStamp(sp["ExecMainExitTimestamp"])
			status, _ := strconv.Atoi(sp["ExecMainStatus"])
			run.ExitStatus, run.Result = status, sp["Result"]
			switch {
			case sp["ActiveState"] == "activating" || sp["ActiveState"] == "active" || sp["ActiveState"] == "deactivating":
				run.State = "running"
			case ended == "":
				run.State = "unknown"
			case sp["Result"] == "success" && status == 0:
				run.State = "succeeded"
			default:
				run.State = "failed"
			}
			if ended != "" && !endAt.Before(startAt) && run.State != "running" {
				run.EndedAt = ended
				run.Seconds = endAt.Sub(startAt).Seconds()
			}
			if inv := sp["InvocationID"]; inv != "" && journals < siteTaskOutputTasks {
				journals++
				if lines, err := siteTaskJournal(inv, siteTaskOutputLines); err == nil {
					for _, line := range lines {
						run.Output = append(run.Output, boundedLine(line, siteTaskOutputBytes))
					}
				}
			}
		}
		runs[task.Unit] = run
	}
	return runs
}

// --- Reading a moved site's tasks and a crontab that came back -------------------

// siteCronSpoolDir is where Debian's and Ubuntu's cron keep crontabs.
var siteCronSpoolDir = "/var/spool/cron/crontabs"

// spoolCrontabLines reads an account's crontab straight from cron's spool
// (root may read it; `crontab -u` refuses an account that /etc/cron.allow
// leaves out, Stage 1 F1) and returns its lines cron acts on.
func spoolCrontabLines(account string) []string {
	if !validSiteCrontabRecordName(account) {
		return nil
	}
	path := filepath.Join(siteCronSpoolDir, account)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range cronEffectiveLines(string(raw)) {
		if !strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

// siteTaskStore is a moved site's tasks as the cron RPCs edit them.
func siteTaskStore(t siteTaskTarget) cronStore {
	return cronStore{
		read: func() (string, error) {
			source, err := readSiteTaskSource(t.SiteID)
			if err != nil {
				return "", err
			}
			if source.Exists && source.Account != "" && source.Account != t.Account {
				return "", fmt.Errorf("the task file of site %d names another account", t.SiteID)
			}
			return source.Crontab, nil
		},
		write: func(before, after string, _ []string) error {
			// An owner's edit is kept and used; this change is written beside
			// it as the pending file (ApplyManagedSiteFile) and refused here.
			result, err := writeSiteTaskSource(t, after, transport.SiteFileTriggerCreate)
			if err != nil {
				return err
			}
			switch result.Outcome {
			case transport.SiteFileOutcomeKept:
				// The owner's text applies: its timers follow it now.
				if source, err := readSiteTaskSource(t.SiteID); err == nil && source.Exists {
					if _, err := applySiteTimers(t, source.Crontab); err != nil {
						log.Printf("site tasks: the timers of site %d could not follow the server owner's task file: %v", t.SiteID, err)
					}
				}
				return errSiteTasksSourceOwner
			case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
				return fmt.Errorf("the task file could not be written (%s %s)", result.Outcome, result.Reason)
			}
			if _, err := applySiteTimers(t, after); err != nil {
				return fmt.Errorf("the task file was saved, but its timers could not all be written (%v); saving again tries again", err)
			}
			return nil
		},
	}
}

// errSiteTasksSourceOwner: the source was changed on the server; the
// panel's change waits beside it.
var errSiteTasksSourceOwner = errors.New(transport.SiteTasksSourceHeld)

// listSiteTasks answers ListCronJobs for a moved site.
func listSiteTasks(t siteTaskTarget, resp *ListCronJobsResponse) error {
	source, err := readSiteTaskSource(t.SiteID)
	if err != nil {
		return err
	}
	resp.Timers = true
	switch {
	case !source.Exists:
		resp.Source = transport.SiteTasksSourceMissing
	case source.State == transport.SiteFileManagedUnchanged:
		resp.Source = transport.SiteTasksSourceOurs
	default:
		resp.Source = transport.SiteTasksSourceOwner
	}
	resp.Jobs = parseCrontab(source.Crontab)
	if resp.Jobs == nil {
		resp.Jobs = []CronJob{}
	}
	resp.Version = cronVersion(source.Crontab)
	resp.Reappeared = spoolCrontabLines(t.Account)

	c := sitecron.Parse(source.Crontab)
	for _, bad := range c.Untranslatable() {
		resp.NotRun = append(resp.NotRun, transport.SiteTaskLine{Line: bad.Line, Text: bad.Raw, Reason: bad.Reason, Detail: bad.Detail})
	}
	jobs := c.Jobs()
	keys := siteTaskKeys(jobs)
	existing, _ := siteTaskUnits(t.SiteID)
	byKey := map[string]int{}
	for n, u := range existing {
		if u.TaskKey != "" {
			byKey[u.TaskKey] = n
		}
	}
	var tasks []siteTimerTask
	for i, job := range jobs {
		if n, ok := byKey[keys[i]]; ok {
			tasks = append(tasks, siteTimerTask{Entry: job, Key: keys[i], N: n, Unit: sitecron.UnitBase(t.SiteID, n)})
		}
	}
	dir, _ := sandboxUnitDir()
	var kept []string
	for _, task := range tasks {
		for _, suffix := range []string{".service", ".timer"} {
			if siteTaskFileKept(siteTaskServiceKind, filepath.Join(dir, task.Unit+suffix)) {
				kept = append(kept, task.Unit+suffix)
			}
		}
	}
	runs := siteTaskRuns(tasks, kept)
	seen := map[string]int{}
	for i := range resp.Jobs {
		job := &resp.Jobs[i]
		if !job.Enabled {
			continue
		}
		k := seen[job.ID]
		seen[job.ID]++
		matched := false
		for _, task := range tasks {
			if generateCronID(task.Entry.Text) != job.ID {
				continue
			}
			if k > 0 {
				k--
				continue
			}
			job.Unit, job.TaskState, job.Run = task.Unit, task.State, runs[task.Unit]
			matched = true
			break
		}
		if !matched {
			job.TaskState = transport.SiteTaskWaiting
			for _, bad := range resp.NotRun {
				if parts := parseCronLine(strings.Trim(bad.Text, " \t")); parts != nil && generateCronID(strings.Trim(bad.Text, " \t")) == job.ID {
					job.TaskState, job.Reason = transport.SiteTaskNotRun, bad.Reason
				}
			}
		}
	}
	return nil
}

// --- A new site and a deleted one ----------------------------------------------

// createSiteTasks gives a new site its empty task file: new sites run their
// tasks as timers from the start (nothing is enabled until a task exists).
// A host that cannot hold a site's limits keeps cron for new sites too.
func createSiteTasks(siteID int, domain, username string) error {
	if siteID < 1 {
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		log.Printf("site tasks: site %d keeps cron: this server cannot hold a site's limits (%s)", siteID, reason)
		return nil
	}
	uid, gid, group, home, err := siteTaskLookup(username)
	if err != nil {
		return err
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	t := siteTaskTarget{SiteID: siteID, Domain: domain, Account: username, Group: group, UID: uid, GID: gid, Home: home}
	result, err := writeSiteTaskSource(t, "", transport.SiteFileTriggerCreate)
	if err != nil {
		return err
	}
	if result.Outcome == transport.SiteFileOutcomeKept {
		log.Printf("site tasks: %s was already there and changed on this server (an earlier site with the same id); it is kept", sitecron.SourcePath(siteID))
	}
	refreshCronAllow()
	return nil
}

// removeSiteTasks takes a deleted site's timers and task files away:
// CelikPanel's unchanged files are removed; a file the server owner changed
// is moved aside under a name systemd does not load, and returned.
func removeSiteTasks(siteID int) ([]string, error) {
	if siteID < 1 {
		return nil, nil
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	aside, err := removeSiteTasksLocked(siteID, true)
	forgetSiteTasksUndo(siteID)
	return aside, err
}
