package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
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
	if err := siteTaskOwnership(t.SiteID, username); err != nil {
		return siteTaskTarget{}, err
	}
	if tenant.Site != nil && tenant.Site.SiteID == tenant.SiteID {
		t.Slice = tenant.Site
	}
	return t, nil
}

// siteTaskOwnership re-derives, from the Agent's own files, that the site id
// the Panel sent belongs to the proved account (S4 R2b fix round; the Agent
// re-derives every fact it acts on): the task file's intro and every task
// unit of that id must name this account. An id with no task file and no
// unit holds nothing of another site.
func siteTaskOwnership(siteID int, account string) error {
	if raw, err := os.ReadFile(sitecron.SourcePath(siteID)); err == nil && len(raw) <= 1<<20 {
		if _, named := splitSiteTaskSource(raw); named != "" && named != account {
			return fmt.Errorf("the task file of site %d belongs to another account; nothing was changed", siteID)
		}
	}
	dir, err := sandboxUnitDir()
	if err != nil {
		return nil
	}
	units, _ := siteTaskUnits(siteID)
	for n := range units {
		path := filepath.Join(dir, sitecron.UnitBase(siteID, n)+".service")
		for _, line := range strings.Split(readSmallFile(path), "\n") {
			if user, ok := strings.CutPrefix(line, "User="); ok && strings.TrimSpace(user) != account {
				return fmt.Errorf("the tasks of site %d run as another account; nothing was changed", siteID)
			}
		}
	}
	return nil
}

// --- The site's limits ----------------------------------------------------------

// siteSliceLoaded says whether systemd loaded the site's slice from its file
// (FragmentPath is the file): a slice systemd made up for a missing file has
// no limits. A test seam.
var siteSliceLoaded = func(siteID int, path string) bool {
	out, err := appUnitSystemctl(systemctlArgs("show", sitesandbox.SliceName(siteID), "--property=FragmentPath", "--value")...)
	return err == nil && strings.TrimSpace(string(out)) == path
}

// siteSlicePathOf is the site's slice file.
func siteSlicePathOf(siteID int) (string, error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sitesandbox.SliceName(siteID)), nil
}

// ensureSiteSliceStrict is ensureSiteSlice for a site's tasks: a slice file
// that could not be written is an error (with what failed), not a log line.
// An owner's edited slice file is kept and used: it is in place.
func ensureSiteSliceStrict(s *transport.SiteSlice) error {
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	result, written, _, err := writeSiteSlice(s, transport.SiteFileTriggerCreate, readSandboxLedger())
	if err != nil {
		return err
	}
	switch result.Outcome {
	case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
		return fmt.Errorf("%s %s %s", result.Outcome, result.Reason, result.Detail)
	}
	if written {
		if err := siteSliceDaemonReload(); err != nil {
			return err
		}
		pushSiteSlicesToKernel([]int{s.SiteID})
	}
	return nil
}

// requireTaskSlice puts the site's slice file in place when the Panel sent
// its limits, then checks that it is there and loaded. Its error is
// transport.SiteTasksLimitsMissing with what is wrong, who acts and what to
// do: no timer of the site is switched on without it.
func requireTaskSlice(t siteTaskTarget) error {
	path, err := siteSlicePathOf(t.SiteID)
	if err != nil {
		return fmt.Errorf("%s (%v)", transport.SiteTasksLimitsMissing, err)
	}
	var writeErr error
	if t.Slice != nil {
		writeErr = ensureSiteSliceStrict(t.Slice)
	}
	info, statErr := os.Lstat(path)
	switch {
	case statErr == nil && info.Mode().IsRegular() && siteSliceLoaded(t.SiteID, path):
		return nil
	case writeErr != nil:
		return fmt.Errorf("%s: %s could not be written (%v). The server owner checks the folder (disk full, a file in its way); saving the task again tries again", transport.SiteTasksLimitsMissing, path, writeErr)
	case t.Slice == nil:
		return fmt.Errorf("%s: %s is not there and the subscription's limits could not be read. The administrator opens the subscription's limits and saves them; then the task is saved again", transport.SiteTasksLimitsMissing, path)
	case statErr != nil || !info.Mode().IsRegular():
		return fmt.Errorf("%s: %s is not a file. The server owner removes what is in its place; saving the task again writes it", transport.SiteTasksLimitsMissing, path)
	default:
		return fmt.Errorf("%s: systemd has not loaded %s. The server owner runs systemctl daemon-reload; saving the task again checks it again", transport.SiteTasksLimitsMissing, path)
	}
}

// siteTaskRenderedPath is the copy of the crontab a site's timers were last
// made from, in the Agent's private folder.
func siteTaskRenderedPath(siteID int) string {
	return filepath.Join(siteTaskRenderedDir, strconv.Itoa(siteID)+".crontab")
}

// siteTaskRenderedDir holds those copies (a variable for tests).
var siteTaskRenderedDir = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-task-rendered")

// --- The source ---------------------------------------------------------------

// siteTaskIntroMark starts the lines CelikPanel puts above a site's tasks in
// its source; they are not part of the crontab the screen shows.
const siteTaskIntroMark = "#| "

// siteTaskIntroLines is how many lines siteTaskIntro writes.
const siteTaskIntroLines = 6

func siteTaskIntro(t siteTaskTarget) string {
	id := strconv.Itoa(t.SiteID)
	lines := []string{
		"CelikPanel: the scheduled tasks of " + t.Domain + " (site " + id + ", account " + t.Account + "), in crontab form.",
		"They run as systemd timers celikcron-" + id + "-<n> in the site's own limits, not by cron (docs/SITE-SANDBOX.md section 9).",
		"Change them on the site's Scheduled tasks screen. A change made here by hand is kept and reaches the timers at",
		"the panel's next start or next save (later panel changes wait beside it as .celikpanel-pending).",
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
	// Only CelikPanel's own intro lines (six of them) are taken off: a
	// crontab whose own first lines start with "#|" keeps them (fix round:
	// the undo gives the crontab back byte for byte).
	for i := 0; i < siteTaskIntroLines && strings.HasPrefix(text, siteTaskIntroMark); i++ {
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
//
// Fix round: it also puts a missing limits file back from the slices the
// Panel sent (no task runs without the site's limits). A site whose limits
// file is not in place and cannot be written has its running timers held
// (stopped, and remembered), and is returned with why; held timers start
// again at the first start that finds the file in place.
func reconcileSiteTimers(slices map[int]*transport.SiteSlice) (rendered int, held []transport.SideDoorsTasksHeld) {
	ids, _ := movedSiteTaskSites()
	ledger := readSandboxLedger()
	wasHeld := readHeldSiteTimers()
	for _, id := range ids {
		path := sitecron.SourcePath(id)
		raw, err := os.ReadFile(path)
		if err != nil || len(raw) > 1<<20 {
			continue
		}
		crontab, _ := splitSiteTaskSource(raw)
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
		t := siteTaskTarget{SiteID: id, Domain: domain, Account: account, Group: group, UID: uid, GID: gid, Home: home, Slice: slices[id]}
		jobs := len(sitecron.Parse(crontab).Jobs())
		cronMu.Lock()
		if jobs > 0 || len(wasHeld[strconv.Itoa(id)]) > 0 {
			if err := requireTaskSlice(t); err != nil {
				holdSiteTimers(id)
				held = append(held, transport.SideDoorsTasksHeld{SiteID: id, Domain: domain,
					Detail: strings.TrimPrefix(strings.TrimPrefix(err.Error(), transport.SiteTasksLimitsMissing), ": ")})
				cronMu.Unlock()
				continue
			}
			resumeSiteTimers(id)
		}
		if ledger[siteTaskAppliedKey(id)].RenderSHA256 != taskDigest(crontab) {
			if _, err := applySiteTimers(t, crontab); err != nil {
				log.Printf("site tasks: the timers of site %d could not all be written: %v", id, err)
			} else {
				rendered++
			}
		}
		cronMu.Unlock()
	}
	return rendered, held
}

// siteTimersHeldPath records, per site, the timers the start held because
// the site's limits file was missing.
var siteTimersHeldPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-timers-held.json")

func readHeldSiteTimers() map[string][]string {
	out := map[string][]string{}
	if raw, err := os.ReadFile(siteTimersHeldPath); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func writeHeldSiteTimers(all map[string][]string) {
	raw, err := json.Marshal(all)
	if err == nil {
		err = writePrivateFile(siteTimersHeldPath, raw)
	}
	if err != nil {
		log.Printf("site tasks: record the held timers: %v", err)
	}
}

// holdSiteTimers stops the site's running timers (a running task is not
// stopped) and remembers them (the caller holds cronMu).
func holdSiteTimers(siteID int) {
	units, err := siteTaskUnits(siteID)
	if err != nil {
		return
	}
	var timers []string
	for n, u := range units {
		if u.Timer {
			timers = append(timers, sitecron.UnitBase(siteID, n)+".timer")
		}
	}
	sort.Strings(timers)
	all := readHeldSiteTimers()
	key := strconv.Itoa(siteID)
	known := map[string]bool{}
	for _, timer := range all[key] {
		known[timer] = true
	}
	props := siteTaskShow(timers, siteTaskShowTimerProps)
	for _, timer := range timers {
		if props[timer]["ActiveState"] != "active" {
			continue
		}
		if out, err := siteTaskSystemctl("stop", timer); err != nil {
			log.Printf("site tasks: %s could not be held: %v: %s", timer, err, strings.TrimSpace(string(out)))
			continue
		}
		if !known[timer] {
			all[key] = append(all[key], timer)
			known[timer] = true
		}
	}
	if len(all[key]) > 0 {
		writeHeldSiteTimers(all)
	}
}

// resumeSiteTimers starts the timers the start held for the site (its limits
// file is back).
func resumeSiteTimers(siteID int) {
	all := readHeldSiteTimers()
	key := strconv.Itoa(siteID)
	timers := all[key]
	if len(timers) == 0 {
		return
	}
	var left []string
	for _, timer := range timers {
		if out, err := siteTaskSystemctl("start", timer); err != nil {
			log.Printf("site tasks: held %s could not be started again: %v: %s", timer, err, strings.TrimSpace(string(out)))
			left = append(left, timer)
		}
	}
	if len(left) == 0 {
		delete(all, key)
	} else {
		all[key] = left
	}
	writeHeldSiteTimers(all)
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
	// No task runs without the site's limits (fix round): nothing of the
	// timers is written (and so none is switched on) until the site's slice
	// file is in place and loaded. The task file itself is saved; the next
	// save, or the next start, writes the timers once the limits are there.
	if len(jobs) > 0 {
		if err := requireTaskSlice(t); err != nil {
			return result, err
		}
	}
	if len(jobs) > 0 || len(c.Env) > 0 {
		if err := ensureSiteTaskDirs(t); err != nil {
			return result, err
		}
	}
	egress, bpf := siteTaskCaps()
	// A task's ExecStartPre runs the sandbox helper, which loads the SMTP
	// block's file: both must be in place before a unit names them, or the
	// task would never start (a server where no program ran yet).
	if egress && len(jobs) > 0 {
		if err := siteMovePrepare(); err != nil {
			return result, fmt.Errorf("the site sandbox's helper and SMTP block could not be put in place: %w", err)
		}
	}
	dir, err := sandboxUnitDir()
	if err != nil {
		return result, err
	}
	sliceFile := filepath.Join(dir, sitesandbox.SliceName(t.SiteID))
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
			Shell: c.Shell, HasEnv: hasEnv, HasInput: task.Entry.HasInput, Spec: task.Entry.Spec, Line: task.Entry.Line, Egress: egress, BPF: bpf,
			SliceFile: sliceFile}
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
	// A copy of the crontab these timers were made from, in the Agent's
	// private folder: a task file that goes missing is rebuilt from it by the
	// administrator (fix round). The units are written by now.
	if err := writePrivateFile(siteTaskRenderedPath(t.SiteID), []byte(crontab)); err != nil {
		log.Printf("site tasks: keep the copy of what site %d's timers were made from: %v", t.SiteID, err)
	}
	// New timers are switched on; one CelikPanel could not switch on before
	// (a save or a start whose enable failed) is switched on now (second fix
	// round: it is remembered until it is on, and never confused with a timer
	// the server owner turned off).
	toEnable := map[string]bool{}
	for _, task := range result.Tasks {
		timer := task.Unit + ".timer"
		if newTimers[timer] && !siteTimersHoldEnable {
			toEnable[timer] = task.Entry.Spec.Reboot
		}
	}
	for timer, atBoot := range readSiteTimersToEnable()[strconv.Itoa(t.SiteID)] {
		for _, task := range result.Tasks {
			if task.Unit+".timer" == timer {
				toEnable[timer] = atBoot
			}
		}
	}
	enableErr := enableSiteTimersToEnable(t.SiteID, toEnable)
	for i := range result.Tasks {
		task := &result.Tasks[i]
		timer := task.Unit + ".timer"
		if changedTimers[timer] && !newTimers[timer] {
			// Only a running timer is restarted: one the owner stopped or
			// disabled stays so.
			if out, err := siteTaskSystemctl("try-restart", timer); err != nil {
				log.Printf("site tasks: %s could not be restarted with its new time: %v: %s", timer, err, strings.TrimSpace(string(out)))
			}
		}
	}
	if enableErr != nil {
		// Not recorded as applied: the reconcile at the next start renders
		// the site again and switches the remembered timers on.
		return result, enableErr
	}
	// The crontab these timers were made from, so the reconcile at each start
	// re-renders only a site whose task file changed since; recorded only once
	// every new timer is on (second fix round).
	if err := setSandboxLedgerEntry(siteTaskAppliedKey(t.SiteID), siteTaskAppliedKind, taskDigest(crontab)); err != nil {
		log.Printf("site tasks: record what site %d's timers were made from: %v", t.SiteID, err)
	}
	return result, nil
}

// siteTimersToEnablePath records, per site, the timers CelikPanel wrote but
// could not switch on yet (timer -> an @reboot one, enabled only), so a
// failed `systemctl enable` is tried again at the next save or start and the
// screen says "could not be switched on yet" instead of "turned off".
var siteTimersToEnablePath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-timers-to-enable.json")

var siteTimersToEnableMu sync.Mutex

func readSiteTimersToEnable() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if raw, err := os.ReadFile(siteTimersToEnablePath); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

// setSiteTimersToEnable replaces one site's list (empty: none left).
func setSiteTimersToEnable(siteID int, timers map[string]bool) {
	siteTimersToEnableMu.Lock()
	defer siteTimersToEnableMu.Unlock()
	all := readSiteTimersToEnable()
	key := strconv.Itoa(siteID)
	if len(timers) == 0 {
		if _, ok := all[key]; !ok {
			return
		}
		delete(all, key)
	} else {
		all[key] = timers
	}
	raw, err := json.Marshal(all)
	if err == nil {
		err = writePrivateFile(siteTimersToEnablePath, raw)
	}
	if err != nil {
		log.Printf("site tasks: record the timers still to switch on: %v", err)
	}
}

// enableSiteTimersToEnable records the timers as to be switched on, switches
// each on (an @reboot one is only enabled) and keeps the ones that failed.
func enableSiteTimersToEnable(siteID int, timers map[string]bool) error {
	if len(timers) == 0 {
		setSiteTimersToEnable(siteID, nil)
		return nil
	}
	setSiteTimersToEnable(siteID, timers)
	names := make([]string, 0, len(timers))
	for timer := range timers {
		names = append(names, timer)
	}
	sort.Strings(names)
	left := map[string]bool{}
	var failures []string
	for _, timer := range names {
		args := []string{"enable", "--now", timer}
		if timers[timer] {
			args = []string{"enable", timer}
		}
		if out, err := siteTaskSystemctl(args...); err != nil {
			left[timer] = timers[timer]
			failures = append(failures, fmt.Sprintf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out))))
		}
	}
	setSiteTimersToEnable(siteID, left)
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
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
	siteTaskShowServiceProps = []string{"Id", "ActiveState", "Result", "ExecMainStartTimestamp", "ExecMainExitTimestamp", "ExecMainStatus", "ExecMainCode", "InvocationID",
		"ConditionResult", "ConditionTimestamp"}
	siteTaskShowTimerProps = []string{"Id", "LoadState", "ActiveState", "UnitFileState", "NextElapseUSecRealtime"}
)

// siteTaskOutputLines and siteTaskOutputBytes bound the output a run shows;
// siteTaskOutputReads bounds the journal readers one list starts (fix round:
// a finished run's lines never change, so they are kept in a bounded cache
// and read once; a list shows at most this many runs it has not read yet,
// the others on the next read).
const (
	siteTaskOutputLines = 10
	siteTaskOutputBytes = 300
	siteTaskOutputReads = 8
	siteTaskOutputCache = 4096
)

var (
	siteTaskOutputMu   sync.Mutex
	siteTaskOutputSeen = map[string][]string{}
)

// cachedTaskOutput returns a finished run's lines, reading the journal only
// when it has not yet and reads are left for this list.
func cachedTaskOutput(invocation string, finished bool, reads *int) ([]string, bool) {
	siteTaskOutputMu.Lock()
	lines, ok := siteTaskOutputSeen[invocation]
	siteTaskOutputMu.Unlock()
	if ok {
		return lines, true
	}
	if *reads >= siteTaskOutputReads {
		return nil, false
	}
	*reads++
	lines, err := siteTaskJournal(invocation, siteTaskOutputLines)
	if err != nil {
		return nil, false
	}
	if finished {
		siteTaskOutputMu.Lock()
		if len(siteTaskOutputSeen) >= siteTaskOutputCache {
			for k := range siteTaskOutputSeen {
				delete(siteTaskOutputSeen, k)
				if len(siteTaskOutputSeen) < siteTaskOutputCache/2 {
					break
				}
			}
		}
		siteTaskOutputSeen[invocation] = lines
		siteTaskOutputMu.Unlock()
	}
	return lines, true
}

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

// siteTaskRuns fills each task's state and last run; toEnable are the
// site's timers CelikPanel could not switch on yet.
func siteTaskRuns(tasks []siteTimerTask, kept []string, toEnable map[string]bool) map[string]*transport.SiteTaskRun {
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
	reads := 0
	for i := range tasks {
		task := &tasks[i]
		tp := timerProps[task.Unit+".timer"]
		_, pending := toEnable[task.Unit+".timer"]
		switch {
		case keptSet[task.Unit+".service"] || keptSet[task.Unit+".timer"]:
			task.State = transport.SiteTaskKept
		case tp == nil || tp["LoadState"] != "loaded":
			task.State = transport.SiteTaskWaiting
		case pending && (tp["UnitFileState"] != "enabled" || (!task.Entry.Spec.Reboot && tp["ActiveState"] != "active")):
			// CelikPanel could not switch it on (second fix round): not the
			// server owner's "turned off"; tried again at the next save or start.
			task.State = transport.SiteTaskNotOnYet
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
			if inv := sp["InvocationID"]; inv != "" {
				if lines, ok := cachedTaskOutput(inv, run.State != "running" && run.State != "unknown", &reads); ok {
					for _, line := range lines {
						run.Output = append(run.Output, boundedLine(line, siteTaskOutputBytes))
					}
				}
			}
		}
		// Second fix round: the last start was skipped because the site's
		// limits file was missing (ConditionPathExists=). systemd marks it
		// as a success with an unmet condition, never as failed; the screen
		// says the run was skipped and why.
		if sp != nil && sp["ConditionResult"] == "no" && run.State != "running" {
			skipped, _ := unixStamp(sp["ConditionTimestamp"])
			*run = transport.SiteTaskRun{State: transport.SiteTaskRunSkipped, StartedAt: skipped, NextAt: run.NextAt}
		}
		runs[task.Unit] = run
	}
	return runs
}

// --- Reading a moved site's tasks and a crontab that came back -------------------

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
			// Fix round: a missing task file while timers of the site remain
			// is a state to report, never one to rebuild from a single line
			// (that would remove every other task). The administrator
			// rebuilds it from the copy of what CelikPanel last rendered.
			if !source.Exists && siteTaskUnitsExist(t.SiteID) {
				return "", errSiteTasksSourceGone
			}
			return source.Crontab, nil
		},
		write: func(before, after string, _ []string) error {
			// A setting systemd cannot read would fail every task of the site:
			// such a file is never written by a save.
			for _, bad := range sitecron.Parse(after).Untranslatable() {
				if bad.Reason == sitecron.ReasonEnvValue {
					return errSiteTasksEnvValue
				}
			}
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
				if strings.HasPrefix(err.Error(), transport.SiteTasksLimitsMissing) {
					return err // saved; its timers wait for the site's limits (said with who acts)
				}
				return fmt.Errorf("the task file was saved, but its timers could not all be written (%v); saving again tries again", err)
			}
			return nil
		},
	}
}

// errSiteTasksSourceOwner: the source was changed on the server; the
// panel's change waits beside it.
var errSiteTasksSourceOwner = errors.New(transport.SiteTasksSourceHeld)

// errSiteTasksSourceGone, errSiteTasksEnvValue: the exact answers the Panel
// classifies (fix round).
var (
	errSiteTasksSourceGone = errors.New(transport.SiteTasksSourceGone)
	errSiteTasksEnvValue   = errors.New(transport.SiteTasksEnvValue)
)

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
		// Second fix round: with a crontab back for the account, the tasks
		// run from it (a put-back that left a unit file the owner changed,
		// or an undo by hand): not "missing"; the owner removes these files.
		if files, err := accountSpoolFiles(t.Account); err == nil && len(files) > 0 {
			resp.Source = transport.SiteTasksSourceCrontab
			resp.LeftUnits = siteTaskUnitPaths(t.SiteID)
		}
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
	if len(resp.Reappeared) > 0 {
		files, _ := accountSpoolFiles(t.Account)
		resp.ReappearedBySite = reappearedBySite(t.Account, files)
	}

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
	runs := siteTaskRuns(tasks, kept, readSiteTimersToEnable()[strconv.Itoa(t.SiteID)])
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
func createSiteTasks(siteID int, domain, username string, slice *transport.SiteSlice) error {
	if siteID < 1 {
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		log.Printf("site tasks: site %d keeps cron: this server cannot hold a site's limits (%s)", siteID, reason)
		return nil
	}
	// Fix round: a site whose account cron's access files would still let
	// install its own jobs keeps cron (the administrator sees it outside its
	// limits and why when moving it), as an existing site's move is refused.
	if why := cronMoveRefusal(username); why != "" {
		log.Printf("site tasks: site %d keeps cron: %s", siteID, why)
		return nil
	}
	uid, gid, group, home, err := siteTaskLookup(username)
	if err != nil {
		return err
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	t := siteTaskTarget{SiteID: siteID, Domain: domain, Account: username, Group: group, UID: uid, GID: gid, Home: home}
	// Its limits file comes with the site (no task of it may run without it).
	if slice != nil && slice.SiteID == siteID {
		if err := ensureSiteSliceStrict(slice); err != nil {
			log.Printf("site tasks: the limits file of new site %d could not be written (%v); its first task waits for it", siteID, err)
		}
	}
	result, err := writeSiteTaskSource(t, "", transport.SiteFileTriggerCreate)
	if err != nil {
		return err
	}
	if result.Outcome == transport.SiteFileOutcomeKept {
		log.Printf("site tasks: %s was already there and changed on this server (an earlier site with the same id); it is kept", sitecron.SourcePath(siteID))
	}
	refreshCronAllow()
	// A new account never had a crontab: from now on one found for it was
	// made by root, while CelikPanel's own cron.allow keeps it out.
	if cronKeptOutByUs(username) {
		if files, _ := accountSpoolFiles(username); len(files) == 0 {
			noteCronClosed(username, time.Now())
		}
	}
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
	account := ""
	if raw, err := os.ReadFile(sitecron.SourcePath(siteID)); err == nil && len(raw) <= 1<<20 {
		_, account = splitSiteTaskSource(raw)
	}
	aside, err := removeSiteTasksLocked(siteID, true)
	forgetSiteTasksUndo(siteID)
	_ = os.Remove(siteTaskRenderedPath(siteID))
	if account != "" {
		forgetCronClosed(map[string]bool{account: true})
	}
	return aside, err
}
