package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site's slice on this server (S4 R2a, docs/SITE-SANDBOX.md section 8).
//
//   - ensureSiteSlice writes celiksite_<site id>.slice under the D-031 rule
//     (an absent file is written, CelikPanel's unchanged text replaced, an
//     owner's edit kept with CelikPanel's text beside it) before a unit is
//     written into the slice. It runs only inside a mutation.
//   - siteUnitEntersSlice decides, read-only, where a unit written now goes:
//     a unit that does not run goes into its site's slice; a running unit
//     stays in the slice it runs in. A changed Slice= reaches a running unit
//     only at its next start (measured 21 Nov 2026 on systemd 257 and 255:
//     after daemon-reload the unit shows the new Slice= but its process stays
//     in /system.slice until it restarts). A running unit therefore moves
//     only by the administrator's action (MoveSiteSliceV1,
//     site_slices_move.go), which restarts it once.
//   - ApplySiteSlicesV1 writes changed limits into the slices, and makes the
//     running slices take them at once (site_slices_runtime.go: systemd 257
//     does not apply a changed slice file to a running slice at
//     daemon-reload; systemd 255 does).
//   - SiteSlicesStatusV1 reads the slices, their counters and where each unit
//     runs. It changes nothing.
//   - A host without the unified cgroup hierarchy and its memory, pids and
//     cpu controllers gets no slice at all: nothing is written that would
//     pretend to limit (siteSliceHostSupport).
//
// Sitenin dilimi: dilim dosyası D-031 kuralıyla yazılır; çalışan bir birim
// yalnız yöneticinin taşıma işlemiyle (bir kez yeniden başlatılarak) dilime
// girer; durum okuması hiçbir şey değiştirmez; sınır uygulayamayan bir
// sunucuya dilim yazılmaz.

// siteSliceCgroupRoot is the cgroup v2 hierarchy (tests replace it).
var siteSliceCgroupRoot = "/sys/fs/cgroup"

var siteSliceMu sync.Mutex

// siteSliceControllers are the controllers a site's slice needs.
var siteSliceControllers = []string{"memory", "pids", "cpu"}

// siteSliceHostSupport says what this host lacks to hold a site's limits
// ("": nothing): the unified cgroup hierarchy ("cgroup v2":
// cgroup.controllers at the root of /sys/fs/cgroup; a cgroup v1 or hybrid
// host has none there) and its memory, pids and cpu controllers (their
// names). A seam for tests.
var siteSliceHostSupport = func() string {
	raw, err := os.ReadFile(filepath.Join(siteSliceCgroupRoot, "cgroup.controllers"))
	if err != nil {
		return "cgroup v2"
	}
	have := map[string]bool{}
	for _, c := range strings.Fields(string(raw)) {
		have[c] = true
	}
	var missing []string
	for _, c := range siteSliceControllers {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return strings.Join(missing, ", ")
}

// siteSliceFor accepts a request's slice for the site it names: nil when the
// request carries none (an older Panel), names another site, has limits out
// of range, or this host cannot hold a site's limits. A unit then stays
// where it runs.
func siteSliceFor(s *transport.SiteSlice, siteID int) *transport.SiteSlice {
	if s == nil || siteID <= 0 || s.SiteID != siteID {
		return nil
	}
	if siteSliceLimits(s).Validate() != nil {
		return nil
	}
	if siteSliceHostSupport() != "" {
		return nil
	}
	return s
}

func siteSliceLimits(s *transport.SiteSlice) sitesandbox.SliceLimits {
	return sitesandbox.SliceLimits{MemoryMB: s.MemoryMB, Tasks: s.Tasks, CPUPercent: s.CPUPercent}
}

// siteSliceFile is a site's slice file.
func siteSliceFile(siteID int) (string, error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sitesandbox.SliceName(siteID)), nil
}

// siteSliceManaged is the slice's native file under the D-031 rule.
func siteSliceManaged(s *transport.SiteSlice, trigger, recorded string) (services.ManagedSiteFile, error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return services.ManagedSiteFile{}, err
	}
	body, err := sitesandbox.SliceUnitText(s.SiteID, s.Domain, dir, siteSliceLimits(s))
	if err != nil {
		return services.ManagedSiteFile{}, err
	}
	file := filepath.Join(dir, sitesandbox.SliceName(s.SiteID))
	return services.ManagedSiteFile{
		Kind: "site_slice", Path: file, HeaderPrefix: services.ManagedHeaderPrefixHash, Body: body,
		Trigger: trigger, RecordedSHA256: recorded, IncludeDir: file + ".d",
	}, nil
}

// siteSliceVerify is systemd's own check of the slice text; a host that
// cannot run it does not refuse the file (the slice has no command).
var siteSliceVerify = func(path string) error {
	if err := appUnitVerify(path); err != nil && !errors.Is(err, services.ErrServiceCheckUnavailable) {
		return err
	}
	return nil
}

// siteSliceAsideSuffix names a slice file or drop-in folder CelikPanel moved
// aside: systemd loads neither "<name>.slice.<suffix>" nor
// "<name>.slice.d.<suffix>".
func siteSliceAsideName(path string, now time.Time) string {
	return path + ".celikpanel-aside-" + now.UTC().Format("20060102T150405Z")
}

// moveSliceFileAside renames an owner's slice file (or drop-in folder) to a
// name systemd does not load. It returns the new name.
func moveSliceFileAside(path string) (string, error) {
	base := siteSliceAsideName(path, time.Now())
	aside := base
	for n := 2; ; n++ {
		if _, err := os.Lstat(aside); errors.Is(err, os.ErrNotExist) {
			break
		}
		aside = base + "-" + strconv.Itoa(n) // never over an earlier copy
	}
	if err := os.Rename(path, aside); err != nil {
		return "", err
	}
	return aside, nil
}

// writeSiteSlice applies the slice file and records CelikPanel's digest. The
// caller holds siteSliceMu, read the ledger with loadSandboxLedger (never
// one that could not be read) and reloads systemd when written is true. An
// earlier site's leftover file of the same id is not this function's: it is
// moved aside when the new site is created (setAsideEarlierSiteSlice), so a
// write for an existing site never moves an owner's file or drop-in aside.
func writeSiteSlice(s *transport.SiteSlice, trigger string, ledger map[string]sandboxLedgerEntry) (transport.SiteFileResult, bool, error) {
	f, err := siteSliceManaged(s, trigger, "")
	if err != nil {
		return transport.SiteFileResult{}, false, err
	}
	f.RecordedSHA256 = ledger[f.Path].RenderSHA256
	result, err := services.ApplyManagedSiteFile(f, func() error { return siteSliceVerify(f.Path) }, nil)
	if err != nil {
		return result, false, err
	}
	written := false
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken:
		written = true
		fallthrough
	case transport.SiteFileOutcomeUnchanged:
		if ledger[f.Path].RenderSHA256 != result.RenderSHA256 {
			ledger[f.Path] = sandboxLedgerEntry{Kind: f.Kind, RenderSHA256: result.RenderSHA256}
			if err := setSandboxLedgerEntry(f.Path, f.Kind, result.RenderSHA256); err != nil {
				log.Printf("site slice: record %s: %v", f.Path, err)
			}
		}
	case transport.SiteFileOutcomeKept:
		log.Printf("site slice: %s was changed on this server and is kept (%s); CelikPanel's text is in %s", f.Path, result.State, result.PendingPath)
	}
	return result, written, nil
}

// setAsideEarlierSiteSlice runs when a site is created (CreateSite; S4 R2a
// second fix round, review B4): a slice file of the new site's id that the
// ledger does not know belongs to an earlier site of an earlier installation
// with the same id. The file AND its drop-in folder are moved aside together
// under names systemd does not load, so the new customer's site inherits
// neither; their new names are returned (and logged). A file the ledger
// knows, and a site that has no such file, are left alone. Third fix round
// (the review's B4 residue): where such a file or folder is there and could
// not be classified (a ledger that cannot be read) or moved aside, the error
// is returned and CreateSite refuses the site with it, before anything of
// the site is made; a site with neither proceeds whatever the ledger.
func setAsideEarlierSiteSlice(siteID int) ([]string, error) {
	file, err := siteSliceFile(siteID)
	if err != nil {
		return nil, err
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	present := false
	for _, p := range []string{file, file + ".d"} {
		if _, err := os.Lstat(p); err == nil {
			present = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s could not be read: %v", p, err)
		}
	}
	if !present {
		return nil, nil
	}
	ledger, err := loadSandboxLedger()
	if err != nil {
		return nil, fmt.Errorf("%s is on the server and CelikPanel's record of its own files could not be read (%v), so it is not known whether an earlier site left it", file, err)
	}
	if _, known := ledger[file]; known {
		return nil, nil
	}
	var aside []string
	for _, p := range []string{file, file + ".d"} {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		moved, err := moveSliceFileAside(p)
		if err != nil {
			return aside, fmt.Errorf("an earlier site's %s could not be moved aside: %v", p, err)
		}
		aside = append(aside, moved)
		log.Printf("site slice: %s belonged to an earlier site with the id %d; it is kept as %s (systemd does not load it)", p, siteID, moved)
	}
	if len(aside) > 0 {
		_ = os.Remove(file + ".celikpanel-pending")
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
		}
	}
	return aside, nil
}

func siteSliceDaemonReload() error {
	if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ensureSiteSlice puts the site's slice in place before a unit is written
// into it. An owner's edited slice is kept and used. A slice that could not
// be written does not stop the operation: systemd makes a slice that has no
// file without limits and the unit still runs; the site's card then shows
// the slice file as absent and that no limit applies, and the next change of
// limits (or the move) writes it again (logged here).
func ensureSiteSlice(s *transport.SiteSlice) {
	if s == nil {
		return
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	ledger, err := loadSandboxLedger()
	if err != nil {
		log.Printf("site slice: %s was not written: %v", sitesandbox.SliceName(s.SiteID), err)
		return
	}
	result, written, err := writeSiteSlice(s, transport.SiteFileTriggerCreate, ledger)
	if err != nil || result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused {
		log.Printf("site slice: %s could not be written (%s %s %s): %v", sitesandbox.SliceName(s.SiteID), result.Outcome, result.Reason, result.Detail, err)
		return
	}
	if written {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
			return
		}
		logSiteSlicePushFailures(pushSiteSlicesToKernel([]int{s.SiteID}))
	}
}

// siteUnitRunningStates are the states of a unit whose cgroup holds
// processes.
var siteUnitRunningStates = map[string]bool{"active": true, "activating": true, "reloading": true, "deactivating": true}

// siteUnitEntersSlice decides where a unit written now runs (read-only): in
// its site's slice when it does not run, or when it already runs there; a
// running unit elsewhere stays there until the site is moved. move: the
// caller restarts the unit right after the write (an administrator's move),
// so it goes into the slice whatever it runs in now.
func siteUnitEntersSlice(unit string, s *transport.SiteSlice, move bool) bool {
	if s == nil || !sitesandbox.ValidUnitName(unit) {
		return false
	}
	if move {
		return true
	}
	out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,ControlGroup")...)
	if err != nil {
		// Unknown: where it ran before, as every unit did before R2a.
		return false
	}
	props := parseShow(string(out))
	if !siteUnitRunningStates[props["ActiveState"]] {
		return true
	}
	return sitesandbox.UnitInSiteSlice(props["ControlGroup"], unit, s.SiteID)
}

// siteSliceTaskUnitPattern: a scheduled task's service of the next release
// (R2b: celikcron-<site id>-<n>.service in its site's slice). This release
// makes none; a server that comes back to it from that release has them, and
// such a unit file naming its own site's slice counts as naming it (S4 R2a
// third fix round), so the slice file and its limits stay managed and a
// recovery never removes a slice the tasks run in.
var siteSliceTaskUnitPattern = regexp.MustCompile(`^celikcron-([1-9][0-9]{0,9})-[1-9][0-9]{0,3}\.service$`)

// siteSliceNamedBy reads which site's slice a unit file in the unit folder
// names (0: none, or not a unit that may name one): a site's program, or a
// task's service naming its own site's slice. The file's slice is read as
// systemd reads it (its last Slice= line, sitesandbox.UnitFileSlice).
func siteSliceNamedBy(dir, name string) int {
	task := siteSliceTaskUnitPattern.FindStringSubmatch(name)
	if task == nil && !siteSliceUnitPattern.MatchString(name) {
		return 0
	}
	id, ok := sitesandbox.ParseSliceName(sitesandbox.UnitFileSlice(readSmallFile(filepath.Join(dir, name))))
	if !ok || (task != nil && task[1] != strconv.Itoa(id)) {
		return 0
	}
	return id
}

// siteSlicesNamed reads the unit folder once: the sites whose slice a unit
// file names (Slice=celiksite_<id>.slice; siteSliceNamedBy) or whose slice
// cgroup exists (a unit runs in it).
func siteSlicesNamed() map[int]bool {
	out := map[int]bool{}
	if dir, err := sandboxUnitDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if !e.Type().IsRegular() {
					continue
				}
				if id := siteSliceNamedBy(dir, e.Name()); id > 0 {
					out[id] = true
				}
			}
		}
	}
	if entries, err := os.ReadDir(siteSliceCgroupRoot); err == nil {
		for _, e := range entries {
			if id, ok := sitesandbox.ParseSliceName(e.Name()); ok && e.IsDir() {
				out[id] = true
			}
		}
	}
	return out
}

// --- Status -----------------------------------------------------------------

// siteSliceUnitPattern: the long-running units a site's slice holds and the
// status read and the move accept.
var siteSliceUnitPattern = regexp.MustCompile(`^(celikapp|celikrt|celiksvc)-[1-9][0-9]{0,9}\.service$`)

const (
	// siteSliceMaxSites bounds one request. The Panel asks in pages well
	// below it; a larger request is read up to the bound and says so
	// (Partial), never refused as a whole.
	siteSliceMaxSites = 5000
	// siteSliceMaxUnits bounds one move (its units each restart in turn).
	siteSliceMaxUnits = 64
	// siteSliceMaxReadUnits bounds the units of one status request (review
	// B9): a site may have any number of programs; the Panel pages its
	// reads by units too, and a request above the bound is read up to it,
	// the rest counted per site as unread, never refused as a whole.
	siteSliceMaxReadUnits = 20000
	siteSliceShowList     = 400
)

var siteSliceShowProperties = []string{"Id", "LoadState", "ActiveState", "SubState", "ControlGroup", "Slice", "MemoryMax", "TasksMax",
	"CPUQuotaPerSecUSec", "MemoryCurrent", "TasksCurrent", "DropInPaths", "FragmentPath", "NeedDaemonReload"}

// siteSliceKernelState reads what a running slice's cgroup enforces
// (memory.max, pids.max, cpu.max) and compares it with what systemd applies
// (S4 R2a third fix round, review N3): "in_force" only when all three equal
// systemd's values and systemd has read the file that is on disk; "pending"
// when they differ (on systemd 257 a changed file reaches a running slice
// only by CelikPanel's own push, a program's start, or, after an owner's
// edit and daemon-reload, up to about two minutes later) or systemd has not
// read the file (FragmentPath empty, NeedDaemonReload=yes); "unknown" when a
// file cannot be read; "" when the slice does not run (systemd applies the
// file when a program starts). Anything that cannot be read is never "in
// force".
func siteSliceKernelState(dir string, props map[string]string, file string) (transport.SiteSliceLimitsRead, string) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return transport.SiteSliceLimitsRead{}, ""
	}
	memory := kernelBytes(readSmallFile(filepath.Join(dir, "memory.max")))
	tasks := kernelBytes(readSmallFile(filepath.Join(dir, "pids.max")))
	cpu := kernelCPUPercent(readSmallFile(filepath.Join(dir, "cpu.max")))
	if memory == -2 || tasks == -2 || cpu == -2 {
		return transport.SiteSliceLimitsRead{}, transport.SiteSliceKernelUnknown
	}
	read := transport.SiteSliceLimitsRead{}
	if memory >= 0 {
		read.MemoryMB = int(memory >> 20)
	}
	if tasks >= 0 && tasks <= math.MaxInt32 {
		read.Tasks = int(tasks)
	}
	if cpu >= 0 {
		read.CPUPercent = int(cpu)
	}
	if props == nil {
		return read, transport.SiteSliceKernelUnknown
	}
	if fragment, shown := props["FragmentPath"]; file != transport.SiteFileAbsent && ((shown && fragment == "") || props["NeedDaemonReload"] == "yes") {
		return read, transport.SiteSlicePending // systemd has not read the file on disk
	}
	wantMemory, wantTasks := systemdBytes(props["MemoryMax"]), systemdBytes(props["TasksMax"])
	wantCPU := int64(-2)
	if usec, limited := systemdTimespanUSec(props["CPUQuotaPerSecUSec"]); limited {
		wantCPU = int64(math.Round(usec / 1e4))
	} else if strings.TrimSpace(props["CPUQuotaPerSecUSec"]) == "infinity" {
		wantCPU = -1
	}
	if wantMemory == -2 || wantTasks == -2 || wantCPU == -2 {
		return read, transport.SiteSliceKernelUnknown
	}
	if !siteSliceSameBytes(wantMemory, memory) || wantTasks != tasks || wantCPU != cpu {
		return read, transport.SiteSlicePending
	}
	return read, transport.SiteSliceInForce
}

// showUnits asks systemd about many units in as few calls as possible
// (systemctl show prints one block per unit, separated by a blank line).
func showUnits(names []string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for start := 0; start < len(names); start += siteSliceShowList {
		end := start + siteSliceShowList
		if end > len(names) {
			end = len(names)
		}
		args := append([]string{"show", "--property=" + strings.Join(siteSliceShowProperties, ",")}, names[start:end]...)
		raw, err := appUnitSystemctl(systemctlArgs(args...)...)
		if err != nil {
			return nil, fmt.Errorf("systemctl show: %v: %s", err, strings.TrimSpace(string(raw)))
		}
		for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
			props := parseShow(block)
			if id := props["Id"]; id != "" {
				out[id] = props
			}
		}
	}
	return out, nil
}

// systemdBytesMB reads a byte value systemd shows ("infinity": 0).
func systemdBytesMB(value string) int {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || n == math.MaxUint64 {
		return 0
	}
	return int(n >> 20)
}

func systemdCount(value string) int {
	n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || n > math.MaxInt32 {
		return 0
	}
	return int(n)
}

var timespanPart = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(us|ms|s|min|h)$`)

// systemdTimespanUSec reads a systemd time span ("1min 30s"); ok false for
// "infinity" or text that is not one.
func systemdTimespanUSec(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "infinity" {
		return 0, false
	}
	var usec float64
	for _, part := range strings.Fields(value) {
		m := timespanPart.FindStringSubmatch(part)
		if m == nil {
			return 0, false
		}
		n, _ := strconv.ParseFloat(m[1], 64)
		switch m[2] {
		case "us":
			usec += n
		case "ms":
			usec += n * 1e3
		case "s":
			usec += n * 1e6
		case "min":
			usec += n * 60e6
		case "h":
			usec += n * 3600e6
		}
	}
	return usec, true
}

// systemdCPUPercent reads CPUQuotaPerSecUSec ("2s" is 200 %, "infinity" no
// quota: 0).
func systemdCPUPercent(value string) int {
	usec, ok := systemdTimespanUSec(value)
	if !ok {
		return 0
	}
	return int(math.Round(usec / 1e4))
}

func readSmallFile(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

// siteSliceFileState reads the slice file's D-031 state (read-only) with the
// ledger the caller read once for the whole request.
func siteSliceFileState(siteID int, ledger map[string]sandboxLedgerEntry) (string, []string) {
	file, err := siteSliceFile(siteID)
	if err != nil {
		return transport.SiteFileUnreadable, nil
	}
	result, _, _ := services.InspectManagedSiteFile(services.ManagedSiteFile{Kind: "site_slice", Path: file,
		HeaderPrefix: services.ManagedHeaderPrefixHash, RecordedSHA256: ledger[file].RenderSHA256})
	var dropIns []string
	if entries, err := os.ReadDir(file + ".d"); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".conf") {
				dropIns = append(dropIns, e.Name())
			}
		}
		sort.Strings(dropIns)
	}
	return result.State, dropIns
}

// siteUnitFileState classifies a unit's file: ours, edited, older (no
// sandbox generation 3 yet), absent, unreadable.
func siteUnitFileState(unit string) string {
	text, _, _, err := siteUnitText(unit)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "absent"
	case err != nil:
		return transport.SiteFileUnreadable
	}
	if _, ok := siteUnitSealedBody(unit, text); !ok {
		return "edited"
	}
	if serviceapps.UnitGeneration(text) < sitesandbox.Generation {
		return "older"
	}
	return "ours"
}

// siteUnitDropInSlice reads the unit's drop-ins (systemd's DropInPaths) for a
// Slice= line: the slice an owner's drop-in places the unit in ("": none).
// The move never restarts such a unit; it is the owner's choice.
func siteUnitDropInSlice(dropInPaths string) string {
	slice := ""
	for _, p := range strings.Fields(dropInPaths) {
		if !strings.HasSuffix(p, ".conf") {
			continue
		}
		if named := sitesandbox.UnitFileSlice(readSmallFile(p)); named != "" {
			slice = named
		}
	}
	return slice
}

// SiteSlicesStatusV1 reads the slices of the asked sites: the file, what
// systemd applies, the slice cgroup's counters and where each unit runs. The
// ledger is read once for the whole request.
func (a *Agent) SiteSlicesStatusV1(req *transport.SiteSlicesStatusRequest, resp *transport.SiteSlicesStatusResponse) error {
	resp.Sites = []transport.SiteSliceStatus{}
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	resp.Unsupported = siteSliceHostSupport()
	sites := req.Sites
	if len(sites) > siteSliceMaxSites {
		sites = sites[:siteSliceMaxSites]
		resp.Partial = fmt.Sprintf("only the first %d of %d sites were read in this request", siteSliceMaxSites, len(req.Sites))
	}
	var names []string
	unread := map[int]int{}
	taken := 0
	for i, q := range sites {
		if q.SiteID <= 0 {
			resp.Error = "a site identity is required"
			return nil
		}
		names = append(names, sitesandbox.SliceName(q.SiteID))
		for _, u := range q.Units {
			if !siteSliceUnitPattern.MatchString(u) {
				resp.Error = "not a site's unit: " + u
				return nil
			}
		}
		if room := siteSliceMaxReadUnits - taken; len(q.Units) > room {
			if room < 0 {
				room = 0
			}
			unread[i] = len(q.Units) - room
			sites[i].Units = q.Units[:room]
			resp.Partial = fmt.Sprintf("only the first %d programs were read in this request", siteSliceMaxReadUnits)
		}
		taken += len(sites[i].Units)
		names = append(names, sites[i].Units...)
	}
	show := map[string]map[string]string{}
	if len(names) > 0 {
		var err error
		if show, err = showUnits(names); err != nil {
			resp.Error = "systemd could not be read: " + err.Error()
			return nil
		}
	}
	ledger := readSandboxLedger() // read-only here: nothing is written
	moves := siteMoveOutcomesNow()
	named := siteSlicesNamed()
	for i, q := range sites {
		slice := sitesandbox.SliceName(q.SiteID)
		props := show[slice]
		status := transport.SiteSliceStatus{SiteID: q.SiteID, Slice: slice, Units: []transport.SiteSliceUnit{}, Named: named[q.SiteID], UnitsUnread: unread[i]}
		status.File, status.DropIns = siteSliceFileState(q.SiteID, ledger)
		status.Active = siteUnitRunningStates[props["ActiveState"]]
		status.Live = transport.SiteSliceLimitsRead{MemoryMB: systemdBytesMB(props["MemoryMax"]), Tasks: systemdCount(props["TasksMax"]),
			CPUPercent: systemdCPUPercent(props["CPUQuotaPerSecUSec"])}
		if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil && mem > 0 {
			status.MemoryCurrentMB = mem >> 20
		}
		if tasks, err := strconv.ParseInt(props["TasksCurrent"], 10, 64); err == nil && tasks > 0 {
			status.TasksCurrent = tasks
		}
		if outcome, ok := moves[strconv.Itoa(q.SiteID)]; ok {
			o := outcome
			status.MoveInterrupted = &o
		}
		for _, u := range q.Units {
			status.Units = append(status.Units, siteSliceUnitStatus(q.SiteID, u, show[u]))
		}
		dir := filepath.Join(siteSliceCgroupRoot, slice)
		status.Kernel, status.KernelState = siteSliceKernelState(dir, props, status.File)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			files := sitesandbox.SliceCgroupFiles{
				MemoryEvents: readSmallFile(filepath.Join(dir, "memory.events")), MemoryEventsLocal: readSmallFile(filepath.Join(dir, "memory.events.local")),
				CPUStat: readSmallFile(filepath.Join(dir, "cpu.stat")),
			}
			if _, err := os.Stat(filepath.Join(dir, "pids.events.local")); err == nil {
				files.HasPidsEventsLocal, files.PidsEventsLocal = true, readSmallFile(filepath.Join(dir, "pids.events.local"))
			} else {
				// alpha.149 (lab D3): the slice's own peak against its limit.
				files.SlicePidsPeak, files.SlicePidsMax = readSmallFile(filepath.Join(dir, "pids.peak")), readSmallFile(filepath.Join(dir, "pids.max"))
			}
			for _, u := range status.Units {
				if u.Placement != transport.SiteUnitInSlice {
					continue
				}
				ud := filepath.Join(dir, u.Unit)
				files.Units = append(files.Units, sitesandbox.UnitPidsFiles{Events: readSmallFile(filepath.Join(ud, "pids.events")),
					EventsLocal: readSmallFile(filepath.Join(ud, "pids.events.local")), Peak: readSmallFile(filepath.Join(ud, "pids.peak")),
					Max: readSmallFile(filepath.Join(ud, "pids.max"))})
			}
			c := sitesandbox.ParseSliceCounters(files)
			status.CountersRead = true
			status.Counters = transport.SiteSliceCounters{MemoryLimitHits: c.MemoryLimitHits, MemoryOOMAll: c.MemoryOOMAll, OOMKills: c.OOMKills,
				MemoryMaxHits: c.MemoryMaxHits, TasksLimitHits: c.TasksLimitHits, TasksOtherHits: c.TasksOtherHits, TasksRule: c.TasksRule,
				CPUThrottled: c.CPUThrottled, CPUThrottledUSec: c.CPUThrottledUSec, CPUPeriods: c.CPUPeriods, TasksPeakAtLimit: c.TasksPeakAtLimit}
		}
		resp.Sites = append(resp.Sites, status)
	}
	return nil
}

// siteSliceUnitStatus says where one unit runs, and what the move can do for
// it: its file's state (edited and older units are not moved) and a slice an
// owner's drop-in names.
func siteSliceUnitStatus(siteID int, unit string, props map[string]string) transport.SiteSliceUnit {
	out := transport.SiteSliceUnit{Unit: unit, File: siteUnitFileState(unit), Slice: props["Slice"],
		DropInSlice: siteUnitDropInSlice(props["DropInPaths"]),
		Own: transport.SiteSliceLimitsRead{MemoryMB: systemdBytesMB(props["MemoryMax"]), Tasks: systemdCount(props["TasksMax"]),
			CPUPercent: systemdCPUPercent(props["CPUQuotaPerSecUSec"])}}
	switch {
	case props == nil || props["LoadState"] == "not-found" || out.File == "absent":
		out.Placement = transport.SiteUnitAbsent
	case !siteUnitRunningStates[props["ActiveState"]]:
		out.Placement = transport.SiteUnitStopped
	case props["ActiveState"] == "activating" && (siteUnitAutoRestart(props) || strings.TrimSpace(props["ControlGroup"]) == ""):
		// alpha.149 (lab D6): systemd is starting it again by itself (its
		// program exited; no process, no cgroup): not running, never "where
		// the server owner placed it".
		out.Placement = transport.SiteUnitRestarting
	case sitesandbox.UnitInSiteSlice(props["ControlGroup"], unit, siteID):
		out.Placement, out.Slice = transport.SiteUnitInSlice, sitesandbox.SliceName(siteID)
	case sitesandbox.UnitInSystemSlice(props["ControlGroup"], unit):
		out.Placement, out.Slice = transport.SiteUnitOutside, sitesandbox.SystemSlice
	default:
		out.Placement = transport.SiteUnitElsewhere
	}
	return out
}

// --- Changed limits ---------------------------------------------------------

// siteSliceApplyBatch bounds one ApplySiteSlicesV1 request: the Panel sends a
// large change in batches of this size, so siteSliceMu (which a customer's
// unit write waits for) is held for one batch only.
const siteSliceApplyBatch = 100

// ApplySiteSlicesV1 writes changed limits into the slices. A site without a
// slice file gets one when one of its units names the slice or runs in it,
// or CelikPanel wrote the file before (the owner deleted it: no limit would
// apply otherwise); a site nothing of which uses the slice gets one when its
// first unit goes into it. A ledger that cannot be read writes nothing and
// answers why (Error). The running slices take the new limits at once (one daemon-reload per
// request, then pushSiteSlicesToKernel); nothing is restarted.
func (a *Agent) ApplySiteSlicesV1(req *transport.ApplySiteSlicesRequest, resp *transport.ApplySiteSlicesResponse) error {
	resp.Sites = []transport.SiteSliceApplied{}
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing site limits"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if len(req.Sites) > siteSliceApplyBatch {
		resp.Error = fmt.Sprintf("at most %d sites can be changed in one request; send the rest in the next one", siteSliceApplyBatch)
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		resp.Unsupported = reason
		return nil
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	ledger, err := loadSandboxLedger()
	if err != nil {
		resp.Error = err.Error() // nothing was written
		return nil
	}
	named := siteSlicesNamed()
	var written, accepted []int
	for i := range req.Sites {
		s := &req.Sites[i]
		applied := transport.SiteSliceApplied{SiteID: s.SiteID}
		if siteSliceFor(s, s.SiteID) == nil {
			applied.Outcome, applied.Detail, applied.Reason = transport.SiteFileOutcomeFailed, "the site's limits are out of range", "out_of_range"
			resp.Sites = append(resp.Sites, applied)
			continue
		}
		accepted = append(accepted, s.SiteID)
		trigger := transport.SiteFileTriggerChange
		path, _ := siteSliceFile(s.SiteID)
		if _, wrote := ledger[path]; named[s.SiteID] || wrote {
			// A unit names the slice or runs in it, or CelikPanel wrote the
			// file before (the owner deleted it to give it back): an absent
			// file is written again (systemd would otherwise run the slice
			// without limits).
			trigger = transport.SiteFileTriggerCreate
		}
		result, didWrite, err := writeSiteSlice(s, trigger, ledger)
		applied.Outcome, applied.PendingPath = result.Outcome, result.PendingPath
		switch {
		case err != nil:
			applied.Outcome, applied.Detail, applied.Reason = transport.SiteFileOutcomeFailed, err.Error(), "write_failed"
		case result.Outcome == transport.SiteFileOutcomeMissing:
			applied.Outcome = transport.SiteFileAbsent
		case result.Outcome == transport.SiteFileOutcomeFailed:
			applied.Detail, applied.Reason = strings.TrimSpace(result.Reason+" "+result.Detail), "write_failed"
		case result.Outcome == transport.SiteFileOutcomeRefused:
			applied.Detail, applied.Reason = strings.TrimSpace(result.Reason+" "+result.Detail), "refused"
		}
		if didWrite {
			written = append(written, s.SiteID)
		}
		resp.Sites = append(resp.Sites, applied)
	}
	if len(written) > 0 {
		if err := siteSliceDaemonReload(); err != nil {
			resp.Error = err.Error()
			return nil
		}
	}
	// Review N3: every running slice of the request is compared with what
	// systemd applies, also one whose file did not change now (an earlier
	// push failed: the hourly round sends it again), and a site whose
	// running slice does not hold its limits afterwards is a failed outcome,
	// never "written".
	failed := pushSiteSlicesToKernel(accepted)
	for i := range resp.Sites {
		why := failed[resp.Sites[i].SiteID]
		if why == "" || resp.Sites[i].Outcome == transport.SiteFileOutcomeFailed || resp.Sites[i].Outcome == transport.SiteFileOutcomeRefused {
			continue
		}
		// alpha.149 (lab O3): the push read the kernel again before this
		// answer; only a slice whose kernel values differ is "does not have
		// the limits"; one that could not be read is "unchecked".
		reason, text := "kernel_differs", "the limits file is written, but the running programs do not have the limits yet: "
		if strings.HasPrefix(why, siteSliceUncheckedPrefix) {
			reason, text = "unchecked", "the limits file is written; whether the running programs have the limits could not be read: "
		}
		resp.Sites[i].Outcome, resp.Sites[i].Detail, resp.Sites[i].Reason = transport.SiteFileOutcomeFailed, text+why, reason
		log.Printf("site slice: %s: %s", sitesandbox.SliceName(resp.Sites[i].SiteID), resp.Sites[i].Detail)
	}
	return nil
}

// --- A deleted site's slice --------------------------------------------------

// removeSiteSlice removes a deleted site's slice: CelikPanel's unchanged file
// goes; a file the owner changed, and the owner's drop-in folder, are moved
// aside under names systemd does not load and returned (the Panel names them
// in the audit log). The ledger forgets the file. Missing files are fine.
func removeSiteSlice(siteID int) ([]string, error) {
	file, err := siteSliceFile(siteID)
	if err != nil {
		return nil, err
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	ledger, err := loadSandboxLedger()
	if err != nil {
		return nil, err // nothing is removed or moved aside on that reading
	}
	var aside []string
	state, _ := siteSliceFileState(siteID, ledger)
	switch state {
	case transport.SiteFileAbsent:
	case transport.SiteFileManagedUnchanged:
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return aside, err
		}
	case transport.SiteFileUnreadable:
		return aside, fmt.Errorf("%s could not be read; it is left as it is", file)
	default:
		moved, err := moveSliceFileAside(file)
		if err != nil {
			return aside, err
		}
		aside = append(aside, moved)
	}
	_ = os.Remove(file + ".celikpanel-pending")
	if info, err := os.Lstat(file + ".d"); err == nil && info.IsDir() {
		moved, err := moveSliceFileAside(file + ".d")
		if err != nil {
			return aside, err
		}
		aside = append(aside, moved)
	}
	if err := updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if _, ok := l[file]; !ok {
			return false
		}
		delete(l, file)
		return true
	}); err != nil {
		log.Printf("site slice: forget %s: %v", file, err)
	}
	forgetSiteMoveOutcome(siteID)
	if state != transport.SiteFileAbsent || len(aside) > 0 {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
		}
	}
	for _, name := range aside {
		log.Printf("site slice: the deleted site %d's %s was changed on this server; it is kept as %s (systemd does not load it)", siteID, filepath.Base(file), name)
	}
	return aside, nil
}

// --- Unit files ----------------------------------------------------------------

// siteUnitText reads a unit's file (no link followed).
func siteUnitText(unit string) (text, file string, mode os.FileMode, err error) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return "", "", 0, err
	}
	file = filepath.Join(dir, unit)
	info, err := os.Lstat(file)
	if err != nil {
		return "", file, 0, err
	}
	if !info.Mode().IsRegular() {
		return "", file, 0, fmt.Errorf("%s is not a regular file", file)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", file, 0, err
	}
	return string(raw), file, info.Mode().Perm(), nil
}

func sha256Text(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// siteUnitSealedBody returns the body of CelikPanel's unchanged unit text:
// a service app's unit carries serviceapps.RenderHeader, the others the
// D-031 header. A text an owner changed (or without a header) is not ours.
func siteUnitSealedBody(unit, text string) (string, bool) {
	prefix := services.ManagedHeaderPrefixHash
	if strings.HasPrefix(unit, "celiksvc-") {
		prefix = serviceapps.RenderHeader
	}
	first, body, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, prefix) {
		return "", false
	}
	declared := strings.TrimPrefix(first, prefix)
	if strings.HasPrefix(unit, "celiksvc-") {
		return body, declared == serviceapps.BodyHash(body)
	}
	return body, declared == sha256Text(body)
}

// siteUnitSeal puts the header of the unit's kind above a new body.
func siteUnitSeal(unit, body string) string {
	if strings.HasPrefix(unit, "celiksvc-") {
		return serviceapps.Seal(body)
	}
	return services.SealManagedTextWith(services.ManagedHeaderPrefixHash, body)
}

var bindPathsLine = regexp.MustCompile(`(?m)^BindPaths=/run/celikpanel-sandbox/([^/\s]+)/app:(\S+)$`)

// siteUnitInHome reports whether the unit binds an app folder inside home.
func siteUnitInHome(unit, body, home string) bool {
	m := bindPathsLine.FindStringSubmatch(body)
	if m == nil || m[1] != unit {
		return false
	}
	dir := path.Clean(m[2])
	return dir == home || strings.HasPrefix(dir, home+"/")
}

// siteUnitMovedBody is the body with its Slice= line naming the site's
// slice. Exactly one Slice= line, of the system slice or of this site's
// slice, is accepted.
func siteUnitMovedBody(body string, siteID int) (string, bool, error) {
	slice := sitesandbox.SliceName(siteID)
	lines := strings.Split(body, "\n")
	found, already := -1, false
	for i, line := range lines {
		if !strings.HasPrefix(line, "Slice=") {
			continue
		}
		if found >= 0 {
			return "", false, errors.New("the unit names more than one slice")
		}
		found = i
		switch line {
		case "Slice=" + sitesandbox.SystemSlice:
		case "Slice=" + slice:
			already = true
		default:
			return "", false, fmt.Errorf("the unit names another slice (%s)", strings.TrimPrefix(line, "Slice="))
		}
	}
	if found < 0 {
		return "", false, errors.New("the unit names no slice")
	}
	lines[found] = "Slice=" + slice
	return strings.Join(lines, "\n"), already, nil
}

// writeUnitBytes replaces a unit file (or a slice file the move puts back)
// with text through the writer every other managed site file uses
// (services.WriteFileDurably: a new file beside it, synced before the
// rename, its folder synced after it; S4 R2a third fix round, review N4), so
// a power loss right after the move's write never leaves an empty unit.
func writeUnitBytes(file, text string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	return services.WriteFileDurably(file, []byte(text), mode)
}
