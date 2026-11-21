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

	"github.com/alicelik/celikpanel/internal/hostingpath"
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
//     stays in the slice it runs in. systemd does not move a running unit:
//     unit_set_slice() refuses a unit that is not inactive (-EBUSY,
//     src/core/unit.c of systemd 255 and 257), and a file naming another
//     slice than the one the unit runs in would leave systemd's view and the
//     cgroup apart after a daemon-reload. A running unit therefore moves only
//     by the administrator's action (MoveSiteSliceV1), which restarts it once.
//   - ApplySiteSlicesV1 writes changed limits into existing slices; systemd
//     applies a changed slice to its running cgroup at the daemon-reload
//     (unit_deserialize_state() invalidates every realized cgroup "so that
//     new cgroup settings get applied", src/core/unit-serialize.c of 255 and
//     257): nothing is restarted.
//   - SiteSlicesStatusV1 reads the slices, their counters and where each unit
//     runs, with one systemctl call for all of them. It changes nothing.
//
// Sitenin dilimi: dilim dosyası D-031 kuralıyla yazılır; çalışan bir birim
// yalnız yöneticinin taşıma işlemiyle (bir kez yeniden başlatılarak) dilime
// girer; durum okuması hiçbir şey değiştirmez.

// siteSliceCgroupRoot is the cgroup v2 hierarchy (tests replace it).
var siteSliceCgroupRoot = "/sys/fs/cgroup"

var siteSliceMu sync.Mutex

// siteSliceFor accepts a request's slice for the site it names: nil when the
// request carries none (an older Panel), names another site or limits out
// of range. A unit then stays where it runs.
func siteSliceFor(s *transport.SiteSlice, siteID int) *transport.SiteSlice {
	if s == nil || siteID <= 0 || s.SiteID != siteID {
		return nil
	}
	if siteSliceLimits(s).Validate() != nil {
		return nil
	}
	return s
}

func siteSliceLimits(s *transport.SiteSlice) sitesandbox.SliceLimits {
	return sitesandbox.SliceLimits{MemoryMB: s.MemoryMB, Tasks: s.Tasks, CPUPercent: s.CPUPercent}
}

// siteSliceManaged is the slice's native file under the D-031 rule.
func siteSliceManaged(s *transport.SiteSlice, trigger string) (services.ManagedSiteFile, error) {
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
		Trigger: trigger, RecordedSHA256: readSandboxLedger()[file].RenderSHA256, IncludeDir: file + ".d",
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

// writeSiteSlice applies the slice file and records CelikPanel's digest. The
// caller holds siteSliceMu and reloads systemd when written is true.
func writeSiteSlice(s *transport.SiteSlice, trigger string) (transport.SiteFileResult, bool, error) {
	f, err := siteSliceManaged(s, trigger)
	if err != nil {
		return transport.SiteFileResult{}, false, err
	}
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
		ledger := readSandboxLedger()
		if ledger[f.Path].RenderSHA256 != result.RenderSHA256 {
			ledger[f.Path] = sandboxLedgerEntry{Kind: f.Kind, RenderSHA256: result.RenderSHA256, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
			if err := writeSandboxLedger(ledger); err != nil {
				log.Printf("site slice: record %s: %v", f.Path, err)
			}
		}
	case transport.SiteFileOutcomeKept:
		log.Printf("site slice: %s was changed on this server and is kept (%s); CelikPanel's text is in %s", f.Path, result.State, result.PendingPath)
	}
	return result, written, nil
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
// file without limits, the unit still runs, and the site's card shows the
// slice file as missing (logged here).
func ensureSiteSlice(s *transport.SiteSlice) {
	if s == nil {
		return
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	result, written, err := writeSiteSlice(s, transport.SiteFileTriggerCreate)
	if err != nil || result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused {
		log.Printf("site slice: %s could not be written (%s %s %s): %v", sitesandbox.SliceName(s.SiteID), result.Outcome, result.Reason, result.Detail, err)
		return
	}
	if written {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
		}
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
	return props["ControlGroup"] == "/"+sitesandbox.SliceName(s.SiteID)+"/"+unit
}

// --- Status -----------------------------------------------------------------

// siteSliceUnitPattern: the long-running units a site's slice holds and the
// status read and the move accept.
var siteSliceUnitPattern = regexp.MustCompile(`^(celikapp|celikrt|celiksvc)-[1-9][0-9]{0,9}\.service$`)

const (
	siteSliceMaxSites = 5000
	siteSliceMaxUnits = 64
	siteSliceShowList = 400
)

var siteSliceShowProperties = []string{"Id", "LoadState", "ActiveState", "ControlGroup", "Slice", "MemoryMax", "TasksMax",
	"CPUQuotaPerSecUSec", "MemoryCurrent", "TasksCurrent"}

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

// systemdCPUPercent reads CPUQuotaPerSecUSec ("2s" is 200 %, "infinity" no
// quota: 0).
func systemdCPUPercent(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "infinity" {
		return 0
	}
	var usec float64
	for _, part := range strings.Fields(value) {
		m := timespanPart.FindStringSubmatch(part)
		if m == nil {
			return 0
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

// siteSliceFileState reads the slice file's D-031 state (read-only).
func siteSliceFileState(siteID int) (string, []string) {
	dir, err := sandboxUnitDir()
	if err != nil {
		return transport.SiteFileUnreadable, nil
	}
	file := filepath.Join(dir, sitesandbox.SliceName(siteID))
	result, _, _ := services.InspectManagedSiteFile(services.ManagedSiteFile{Kind: "site_slice", Path: file,
		HeaderPrefix: services.ManagedHeaderPrefixHash, RecordedSHA256: readSandboxLedger()[file].RenderSHA256})
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

// SiteSlicesStatusV1 reads the slices of the asked sites: the file, what
// systemd applies, the slice cgroup's counters and where each unit runs.
func (a *Agent) SiteSlicesStatusV1(req *transport.SiteSlicesStatusRequest, resp *transport.SiteSlicesStatusResponse) error {
	resp.Sites = []transport.SiteSliceStatus{}
	if req == nil || len(req.Sites) > siteSliceMaxSites {
		resp.Error = "between 0 and 5000 sites can be read at once"
		return nil
	}
	var names []string
	for _, q := range req.Sites {
		if q.SiteID <= 0 || len(q.Units) > siteSliceMaxUnits {
			resp.Error = "a site identity and at most 64 units per site are required"
			return nil
		}
		names = append(names, sitesandbox.SliceName(q.SiteID))
		for _, u := range q.Units {
			if !siteSliceUnitPattern.MatchString(u) {
				resp.Error = "not a site's unit: " + u
				return nil
			}
			names = append(names, u)
		}
	}
	show := map[string]map[string]string{}
	if len(names) > 0 {
		var err error
		if show, err = showUnits(names); err != nil {
			resp.Error = "systemd could not be read: " + err.Error()
			return nil
		}
	}
	for _, q := range req.Sites {
		slice := sitesandbox.SliceName(q.SiteID)
		props := show[slice]
		status := transport.SiteSliceStatus{SiteID: q.SiteID, Slice: slice, Units: []transport.SiteSliceUnit{}}
		status.File, status.DropIns = siteSliceFileState(q.SiteID)
		status.Active = siteUnitRunningStates[props["ActiveState"]]
		status.Live = transport.SiteSliceLimitsRead{MemoryMB: systemdBytesMB(props["MemoryMax"]), Tasks: systemdCount(props["TasksMax"]),
			CPUPercent: systemdCPUPercent(props["CPUQuotaPerSecUSec"])}
		if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil && mem > 0 {
			status.MemoryCurrentMB = mem >> 20
		}
		if tasks, err := strconv.ParseInt(props["TasksCurrent"], 10, 64); err == nil && tasks > 0 {
			status.TasksCurrent = tasks
		}
		dir := filepath.Join(siteSliceCgroupRoot, slice)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			c := sitesandbox.ParseSliceCounters(readSmallFile(filepath.Join(dir, "memory.events")),
				readSmallFile(filepath.Join(dir, "pids.events")), readSmallFile(filepath.Join(dir, "cpu.stat")))
			status.CountersRead = true
			status.Counters = transport.SiteSliceCounters{OOMKills: c.OOMKills, MemoryMaxHits: c.MemoryMaxHits, TasksMaxHits: c.TasksMaxHits,
				CPUThrottled: c.CPUThrottled, CPUThrottledUSec: c.CPUThrottledUSec, CPUPeriods: c.CPUPeriods}
		}
		for _, u := range q.Units {
			status.Units = append(status.Units, siteSliceUnitStatus(q.SiteID, u, show[u]))
		}
		resp.Sites = append(resp.Sites, status)
	}
	return nil
}

// siteSliceUnitStatus says where one unit runs.
func siteSliceUnitStatus(siteID int, unit string, props map[string]string) transport.SiteSliceUnit {
	out := transport.SiteSliceUnit{Unit: unit, File: siteUnitFileState(unit), Slice: props["Slice"],
		Own: transport.SiteSliceLimitsRead{MemoryMB: systemdBytesMB(props["MemoryMax"]), Tasks: systemdCount(props["TasksMax"]),
			CPUPercent: systemdCPUPercent(props["CPUQuotaPerSecUSec"])}}
	switch {
	case props == nil || props["LoadState"] == "not-found" || out.File == "absent":
		out.Placement = transport.SiteUnitAbsent
	case !siteUnitRunningStates[props["ActiveState"]]:
		out.Placement = transport.SiteUnitStopped
	case props["ControlGroup"] == "/"+sitesandbox.SliceName(siteID)+"/"+unit:
		out.Placement, out.Slice = transport.SiteUnitInSlice, sitesandbox.SliceName(siteID)
	case props["ControlGroup"] == "/"+sitesandbox.SystemSlice+"/"+unit:
		out.Placement, out.Slice = transport.SiteUnitOutside, sitesandbox.SystemSlice
	default:
		out.Placement = transport.SiteUnitElsewhere
	}
	return out
}

// --- Changed limits ---------------------------------------------------------

// ApplySiteSlicesV1 writes changed limits into the slices that exist. A
// site without a slice file gets one when its first unit goes into it. The
// running slices take the new limits at the one daemon-reload; nothing is
// restarted.
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
	if len(req.Sites) > siteSliceMaxSites {
		resp.Error = "at most 5000 sites can be changed at once"
		return nil
	}
	siteSliceMu.Lock()
	defer siteSliceMu.Unlock()
	reload := false
	for i := range req.Sites {
		s := &req.Sites[i]
		applied := transport.SiteSliceApplied{SiteID: s.SiteID}
		if siteSliceFor(s, s.SiteID) == nil {
			applied.Outcome, applied.Detail = transport.SiteFileOutcomeFailed, "the site's limits are out of range"
			resp.Sites = append(resp.Sites, applied)
			continue
		}
		result, written, err := writeSiteSlice(s, transport.SiteFileTriggerChange)
		applied.Outcome, applied.PendingPath = result.Outcome, result.PendingPath
		switch {
		case err != nil:
			applied.Outcome, applied.Detail = transport.SiteFileOutcomeFailed, err.Error()
		case result.Outcome == transport.SiteFileOutcomeMissing:
			applied.Outcome = transport.SiteFileAbsent
		case result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused:
			applied.Detail = strings.TrimSpace(result.Reason + " " + result.Detail)
		}
		reload = reload || written
		resp.Sites = append(resp.Sites, applied)
	}
	if reload {
		if err := siteSliceDaemonReload(); err != nil {
			resp.Error = err.Error()
		}
	}
	return nil
}

// --- The administrator's move -----------------------------------------------

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

// writeUnitBytes replaces a unit file with text (a new file beside it,
// renamed into place).
func writeUnitBytes(file, text string, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	tmp := file + ".celikpanel-partial"
	if err := os.WriteFile(tmp, []byte(text), mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Seams of the move (tests replace them).
var (
	siteSliceSettle    = 20 * time.Second
	siteSlicePoll      = 500 * time.Millisecond
	siteSliceListening = func(unit string) map[int]bool {
		listeners, pids, err := nodeSiteListeners(unit)
		if err != nil {
			return nil
		}
		ports := map[int]bool{}
		for _, l := range listeners {
			for _, pid := range l.PIDs {
				if pids[pid] {
					ports[l.Port] = true
				}
			}
		}
		return ports
	}
)

// siteMoveUnit is one unit the move changes.
type siteMoveUnit struct {
	unit    string
	file    string
	mode    os.FileMode
	before  string
	after   string
	running bool
	ports   map[int]bool
	already bool
}

var siteMoveRunning sync.Mutex

// MoveSiteSliceV1 moves a site's units into its slice: the slice is written,
// then each unit in turn: its file is rewritten to name the slice and, if it
// was running, it restarts once and must run in the slice (and listen again
// on every port it listened on before) within siteSliceSettle. A failure
// puts that unit and every unit of the site moved before it back as they
// were (the earlier text, a restart where it ran); other sites are never
// touched. A unit an owner changed is kept and listed.
func (a *Agent) MoveSiteSliceV1(req *transport.MoveSiteSliceRequest, resp *transport.MoveSiteSliceResponse) error {
	resp.Units = []transport.SiteSliceMoved{}
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "moving a site into its limits"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	s := siteSliceFor(&req.Site, req.Site.SiteID)
	home, homeErr := hostingpath.SiteHome(req.SubscriptionID, req.DomainID)
	if s == nil || homeErr != nil || len(req.Units) == 0 || len(req.Units) > siteSliceMaxUnits {
		resp.Error = "the site, its home and up to 64 of its units are required"
		return nil
	}
	for _, u := range req.Units {
		if !siteSliceUnitPattern.MatchString(u) {
			resp.Error = "not a site's unit: " + u
			return nil
		}
	}
	if !siteMoveRunning.TryLock() {
		resp.Error = "another move into a site's limits is running; nothing was changed"
		return nil
	}
	defer siteMoveRunning.Unlock()

	siteSliceMu.Lock()
	result, written, err := writeSiteSlice(s, transport.SiteFileTriggerCreate)
	siteSliceMu.Unlock()
	resp.SliceFile = result.Outcome
	if err != nil || result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused {
		resp.Error = fmt.Sprintf("the site's limits could not be written (%s %s): nothing was moved", result.Outcome, strings.TrimSpace(result.Reason+" "+result.Detail))
		if err != nil {
			resp.Error = "the site's limits could not be written: " + err.Error() + "; nothing was moved"
		}
		return nil
	}
	if written {
		if err := siteSliceDaemonReload(); err != nil {
			resp.Error = err.Error() + "; nothing was moved"
			return nil
		}
	}

	// Classify every unit before anything changes.
	var plan []siteMoveUnit
	for _, u := range req.Units {
		text, file, mode, err := siteUnitText(u)
		if errors.Is(err, os.ErrNotExist) {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveAbsent})
			continue
		}
		if err != nil {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: err.Error()})
			continue
		}
		body, ours := siteUnitSealedBody(u, text)
		switch {
		case !ours:
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept,
				Detail: file + " was changed on this server; it is kept and runs where it runs. Your own settings belong in " + file + ".d/*.conf"})
			continue
		case serviceapps.UnitGeneration(text) < sitesandbox.Generation:
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveOlder})
			continue
		case !siteUnitInHome(u, body, home):
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveRefused, Detail: "its app folder is not in this site's home"})
			continue
		}
		moved, already, err := siteUnitMovedBody(body, s.SiteID)
		if err != nil {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: err.Error()})
			continue
		}
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", u, "--property=ActiveState,ControlGroup")...); err == nil {
			props = parseShow(string(out))
		} else {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: "systemd could not be read: " + strings.TrimSpace(string(out))})
			continue
		}
		running := siteUnitRunningStates[props["ActiveState"]]
		if already && (!running || props["ControlGroup"] == "/"+sitesandbox.SliceName(s.SiteID)+"/"+u) {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveAlready})
			continue
		}
		m := siteMoveUnit{unit: u, file: file, mode: mode, before: text, after: siteUnitSeal(u, moved), running: running, already: already}
		if running {
			m.ports = siteSliceListening(u)
		}
		plan = append(plan, m)
	}

	var done []siteMoveUnit
	for i, m := range plan {
		step, err := moveOneSiteUnit(m, s.SiteID)
		if err == nil {
			done = append(done, m)
			outcome := transport.SiteMoveRewritten
			if m.running {
				outcome = transport.SiteMoveMoved
			}
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: m.unit, Outcome: outcome})
			continue
		}
		// Put this unit back, then every unit of the site moved before it.
		failed := transport.SiteSliceMoved{Unit: m.unit, Outcome: transport.SiteMoveFailed, Step: step, Detail: err.Error()}
		if backErr := putBackSiteUnit(m); backErr != nil {
			failed.Detail += "; putting it back did not complete: " + backErr.Error()
		} else {
			failed.PutBack = true
		}
		resp.Units = append(resp.Units, failed)
		for j := len(done) - 1; j >= 0; j-- {
			back := transport.SiteSliceMoved{Unit: done[j].unit, Outcome: transport.SiteMovePutBack, PutBack: true}
			if backErr := putBackSiteUnit(done[j]); backErr != nil {
				back.PutBack, back.Detail = false, "putting it back did not complete: "+backErr.Error()
			}
			for k := range resp.Units {
				if resp.Units[k].Unit == back.Unit {
					resp.Units[k] = back
				}
			}
		}
		for _, rest := range plan[i+1:] {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: rest.unit, Outcome: transport.SiteMoveNotTried})
		}
		resp.RolledBack = true
		resp.Error = fmt.Sprintf("%s could not be moved into the site's limits (%s): %s; the site was put back as it was", m.unit, step, err.Error())
		return nil
	}
	resp.Done = true
	return nil
}

// moveOneSiteUnit rewrites one unit and, if it ran, restarts it in the slice
// and waits for it to run there and listen again. It returns the failed step.
func moveOneSiteUnit(m siteMoveUnit, siteID int) (string, error) {
	if err := writeUnitBytes(m.file, m.after, m.mode); err != nil {
		return "write", err
	}
	if err := siteSliceDaemonReload(); err != nil {
		return "write", err
	}
	if !m.running {
		return "", nil
	}
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
	if out, err := appUnitSystemctl(systemctlArgs("restart", m.unit)...); err != nil {
		return "restart", fmt.Errorf("systemctl restart: %v: %s", err, strings.TrimSpace(string(out)))
	}
	want := "/" + sitesandbox.SliceName(siteID) + "/" + m.unit
	deadline := time.Now().Add(siteSliceSettle)
	for {
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", m.unit, "--property=ActiveState,ControlGroup")...); err == nil {
			props = parseShow(string(out))
		}
		inSlice := props["ActiveState"] == "active" && props["ControlGroup"] == want
		listening := true
		if inSlice && len(m.ports) > 0 {
			now := siteSliceListening(m.unit)
			for port := range m.ports {
				listening = listening && now[port]
			}
		}
		if inSlice && listening {
			return "", nil
		}
		if time.Now().After(deadline) {
			switch {
			case props["ActiveState"] != "active":
				return "restart", fmt.Errorf("it is %s after the restart (journalctl -u %s says why)", strings.TrimSpace(props["ActiveState"]+" "), m.unit)
			case props["ControlGroup"] != want:
				return "cgroup", fmt.Errorf("it runs in %s, not %s", props["ControlGroup"], want)
			}
			return "answer", fmt.Errorf("it no longer listens on every port it listened on before (%s)", siteSlicePorts(m.ports))
		}
		time.Sleep(siteSlicePoll)
	}
}

func siteSlicePorts(ports map[int]bool) string {
	list := make([]int, 0, len(ports))
	for p := range ports {
		list = append(list, p)
	}
	sort.Ints(list)
	words := make([]string, len(list))
	for i, p := range list {
		words[i] = strconv.Itoa(p)
	}
	return strings.Join(words, ", ")
}

// putBackSiteUnit writes the unit's earlier text and, if it ran, restarts it
// where it ran before.
func putBackSiteUnit(m siteMoveUnit) error {
	if err := writeUnitBytes(m.file, m.before, m.mode); err != nil {
		return err
	}
	if err := siteSliceDaemonReload(); err != nil {
		return err
	}
	if !m.running {
		return nil
	}
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
	if out, err := appUnitSystemctl(systemctlArgs("restart", m.unit)...); err != nil {
		return fmt.Errorf("systemctl restart: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
