package sitesandbox

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The site's slice (S4 R2a, docs/SITE-SANDBOX.md §8). Every program
// CelikPanel starts for one site as a systemd unit - its uploaded programs
// and Python apps, its Node.js application, its service apps, and their
// install, build and deploy steps - runs in one slice whose MemoryMax=,
// TasksMax= and CPUQuota= bound them together. A slice nests: systemd applies
// the most stringent limit of a unit and its parent slices
// ("EffectiveMemoryMax= ... is the most stringent limit of the unit and parent
// slices", systemd.resource-control(5) of systemd 257 and 255).
//
// The name has no "-": systemd reads a dash in a slice's name as nesting
// (celiksite-7.slice would be a child of celiksite.slice), so the slice sits
// directly below the root and a unit in it is /celiksite_<id>.slice/<unit>.
// That is depth two, as /system.slice/<unit> is, so the SMTP block's set
// (typeof socket cgroupv2 level 2) matches a unit in either place without any
// change to the table.
//
// The slice is a native unit file under the D-031 rule: an owner can read
// it, override it with a drop-in (<slice>.d/*.conf), or edit it (an edit is
// kept). It keeps working without CelikPanel (D-022).
//
// Sitenin dilimi: CelikPanel'in bir site için başlattığı her program tek bir
// systemd diliminde çalışır; dilimin bellek, işlem ve işlemci sınırları
// hepsini birlikte sınırlar. Dosya sahibin okuyup değiştirebileceği yerel bir
// birimdir.

// SystemSlice is where a unit not moved into its site's slice runs.
const SystemSlice = "system.slice"

// maxSiteID bounds a site identity in a unit name.
const maxSiteID = 1<<31 - 1

var slicePattern = regexp.MustCompile(`^celiksite_([1-9][0-9]{0,9})\.slice$`)

// SliceName is a site's slice: celiksite_<site id>.slice.
func SliceName(siteID int) string { return "celiksite_" + strconv.Itoa(siteID) + ".slice" }

// ParseSliceName reads a site's slice name back to its site identity.
func ParseSliceName(name string) (int, bool) {
	m := slicePattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	id, err := strconv.Atoi(m[1])
	if err != nil || id <= 0 || id > maxSiteID {
		return 0, false
	}
	return id, true
}

// SliceLimits are a site's limits: what all of its programs may use
// together. MemoryMB becomes MemoryMax=, Tasks TasksMax= (processes and
// threads), CPUPercent CPUQuota= (100 is one processor core).
type SliceLimits struct {
	MemoryMB   int `json:"memory_mb"`
	Tasks      int `json:"tasks"`
	CPUPercent int `json:"cpu_percent"`
}

// Validate keeps the limits in the sandbox's range (the same bounds a unit's
// own limits have).
func (l SliceLimits) Validate() error {
	if l.MemoryMB < 64 || l.MemoryMB > 65536 || l.CPUPercent < 10 || l.CPUPercent > 1600 || l.Tasks < 16 || l.Tasks > 8192 {
		return errors.New("the site's limits must be 64 to 65536 MB of memory, 16 to 8192 processes and 10 to 1600 percent of a processor")
	}
	return nil
}

var sliceDomain = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

// SliceDropInDir is the owner's place for drop-ins of a site's slice.
func SliceDropInDir(unitDir string, siteID int) string {
	return strings.TrimSuffix(unitDir, "/") + "/" + SliceName(siteID) + ".d"
}

// SliceUnitText is the slice's unit text (CelikPanel's header is added when
// the file is written). The domain names the site for a person reading the
// file; it is left out when it is not a plain host name.
func SliceUnitText(siteID int, domain, unitDir string, l SliceLimits) (string, error) {
	if siteID <= 0 || siteID > maxSiteID {
		return "", errors.New("a site identity is required")
	}
	if err := l.Validate(); err != nil {
		return "", err
	}
	name := SliceName(siteID)
	label := fmt.Sprintf("site %d", siteID)
	if sliceDomain.MatchString(domain) && strings.Contains(domain, ".") {
		label = fmt.Sprintf("%s (site %d)", domain, siteID)
	}
	dropIns := SliceDropInDir(unitDir, siteID)
	var b strings.Builder
	fmt.Fprintf(&b, "# The limits of %s, written by CelikPanel (docs/SITE-SANDBOX.md section 8).\n", label)
	b.WriteString("# Every program CelikPanel starts for this site runs in this slice: its programs, Python apps,\n")
	b.WriteString("# Node.js application, service apps and their install, build and deploy steps. Together they\n")
	b.WriteString("# may use no more than the limits below; when they reach the memory limit the kernel stops one\n")
	b.WriteString("# of them, when they reach the process limit no new process starts, and the processor limit slows\n")
	b.WriteString("# them down. Other sites and the server keep what is theirs.\n")
	fmt.Fprintf(&b, "# It works without CelikPanel: systemctl status %s ; systemd-cgls /%s\n", name, name)
	fmt.Fprintf(&b, "# Your own settings go in %s/*.conf (systemd drop-ins); CelikPanel never writes or removes them.\n", dropIns)
	b.WriteString("# An edit to this file is kept: CelikPanel then reports it and does not write the file again.\n")
	b.WriteString("# Bu sitenin bütün programları bu dilimde çalışır ve aşağıdaki sınırları birlikte aşamaz.\n")
	fmt.Fprintf(&b, "# Kendi ayarlarınız %s/*.conf içine; CelikPanel onlara dokunmaz.\n", dropIns)
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=CelikPanel site limits: %s\n", label)
	b.WriteString("Before=slices.target\n")
	b.WriteString("\n[Slice]\n")
	fmt.Fprintf(&b, "MemoryMax=%dM\n", l.MemoryMB)
	b.WriteString("MemorySwapMax=0\n")
	fmt.Fprintf(&b, "TasksMax=%d\n", l.Tasks)
	fmt.Fprintf(&b, "CPUQuota=%d%%\n", l.CPUPercent)
	return b.String(), nil
}

// AllowedControlGroups are the cgroups a unit of the site may run in: its
// site's slice, or the system slice where it ran before it was moved. Both
// are depth two.
func AllowedControlGroups(siteID int, unit string) []string {
	out := []string{"/" + SystemSlice + "/" + unit}
	if siteID > 0 && siteID <= maxSiteID {
		out = append(out, "/"+SliceName(siteID)+"/"+unit)
	}
	return out
}

// UnitControlGroupAllowed reports whether cgroup is /system.slice/<unit> or
// /celiksite_<id>.slice/<unit> for a sandboxed unit name, and returns the
// site identity of the slice (0 for the system slice).
func UnitControlGroupAllowed(cgroup, unit string) (int, bool) {
	if !ValidUnitName(unit) {
		return 0, false
	}
	if cgroup == "/"+SystemSlice+"/"+unit {
		return 0, true
	}
	rest, ok := strings.CutPrefix(cgroup, "/")
	if !ok {
		return 0, false
	}
	slice, leaf, ok := strings.Cut(rest, "/")
	if !ok || leaf != unit {
		return 0, false
	}
	return ParseSliceName(slice)
}

// UnitInSiteSlice reports whether cgroup (a unit's ControlGroup= as systemd
// shows it) is the unit's place in site siteID's slice. The Agent's reading
// of where a program runs goes through UnitControlGroupAllowed, the one
// definition of a unit's two places that the SMTP helper uses too.
func UnitInSiteSlice(cgroup, unit string, siteID int) bool {
	id, ok := UnitControlGroupAllowed(cgroup, unit)
	return ok && siteID > 0 && id == siteID
}

// UnitInSystemSlice reports whether cgroup is the unit's place in the system
// slice, where every unit ran before it was moved into its site's slice.
func UnitInSystemSlice(cgroup, unit string) bool {
	id, ok := UnitControlGroupAllowed(cgroup, unit)
	return ok && id == 0
}

// CgroupElementFor is the SMTP set's element for a unit running in cgroup
// (the unit's ControlGroup= as systemd shows it): '{ "<slice>/<unit>" }'. A
// cgroup that is not the system slice or a site's slice is refused: the set
// matches depth two only, so a unit anywhere else would not be blocked.
func CgroupElementFor(cgroup, unit string) (string, error) {
	if _, ok := UnitControlGroupAllowed(cgroup, unit); !ok {
		return "", fmt.Errorf("%s runs in %q, not in the system slice or a site's slice", unit, cgroup)
	}
	return `{ "` + strings.TrimPrefix(cgroup, "/") + `" }`, nil
}

// SliceCounters are the slice cgroup's event counters since the slice's
// cgroup was made (they start at zero again after every program of the site
// stopped and the slice was removed). Which counter proves which limit was
// measured on real kernels (21 Nov 2026, Debian 13 kernel 6.12.105 with
// systemd 257 and Ubuntu 24.04 kernel 6.8.0 with systemd 255; the raw outputs
// are quoted in docs/SITE-SANDBOX.md section 8):
//
//   - The slice's OWN memory limit: the slice's memory.events.local "oom"
//     rises (both kernels). A program's own smaller MemoryMax= raises only the
//     slice's hierarchical memory.events "oom", not the local one; a host-wide
//     shortage raises only the hierarchical "oom_kill" (no "oom" anywhere).
//   - The slice's OWN process limit: where the slice has pids.events.local
//     (Debian's 6.12), its "max" rises only for the slice's limit. Where it
//     has none (6.8), the kernel counts the refusal in the forking unit's own
//     pids.events whichever level refused it; the slice's limit is then the
//     one reached when that unit's pids.peak stayed below its own pids.max
//     (the slice is the only other level with a limit).
//
// Anything else is counted as "other": a program of the site was stopped or
// refused, but not proved to be at the site's limit; the screens then never
// blame the plan.
type SliceCounters struct {
	// MemoryLimitHits: the slice's memory.events.local oom, times the site's
	// programs together reached the site's memory limit.
	MemoryLimitHits int64 `json:"memory_limit_hits"`
	// MemoryOOMAll: the slice's memory.events oom (hierarchical): a limit was
	// reached in the slice or in one of its programs.
	MemoryOOMAll int64 `json:"memory_oom_all"`
	// OOMKills: the slice's memory.events oom_kill (hierarchical), programs of
	// the site the kernel stopped for any lack of memory.
	OOMKills int64 `json:"oom_kills"`
	// MemoryMaxHits: the slice's memory.events.local max, times the site's
	// memory reached the limit and had to be reclaimed.
	MemoryMaxHits int64 `json:"memory_max_hits"`
	// TasksLimitHits: new processes refused at the site's own process limit.
	TasksLimitHits int64 `json:"tasks_limit_hits"`
	// TasksOtherHits: new processes refused at a program's own limit (or where
	// the kernel does not say which level refused them).
	TasksOtherHits int64 `json:"tasks_other_hits"`
	// TasksRule names how the process counters were read: "local" (the
	// slice's pids.events.local) or "units" (each unit's pids.events and
	// pids.peak).
	TasksRule string `json:"tasks_rule,omitempty"`
	// CPUThrottled: cpu.stat nr_throttled, periods the site was slowed down;
	// CPUThrottledUSec the time it waited; CPUPeriods the periods counted.
	// cpu.stat throttling is the slice's own quota (non-hierarchical).
	CPUThrottled     int64 `json:"cpu_throttled"`
	CPUThrottledUSec int64 `json:"cpu_throttled_usec"`
	CPUPeriods       int64 `json:"cpu_periods"`
	// SlicePidsPeak and SlicePidsMax (alpha.149, lab D3; fix round F7):
	// under the "units" rule, the slice's own pids.peak and the pids.max it
	// was read with (both 0: not read, or no limit). A unit's counters go
	// with its cgroup when the program restarts (lab, Ubuntu 24.04 kernel
	// 6.8: the refusals were counted in a unit cgroup that was gone before
	// any reading); the slice's peak stays while the slice exists. pids.peak
	// is a high-water mark that writing pids.max does not reset, so only
	// readings compared under an unchanged limit can say that the peak
	// reached it since the last one (the Panel's ledger does that). Whether a
	// new process was refused is not known from them.
	SlicePidsPeak int64 `json:"slice_pids_peak,omitempty"`
	SlicePidsMax  int64 `json:"slice_pids_max,omitempty"`
}

// SliceCgroupFiles are the texts of the slice cgroup's files ("" for a file
// that could not be read) and of each unit cgroup in it.
type SliceCgroupFiles struct {
	MemoryEvents      string
	MemoryEventsLocal string
	PidsEventsLocal   string
	// HasPidsEventsLocal: the kernel has pids.events.local (6.12 on Debian 13,
	// 6.13 and later; not 6.8).
	HasPidsEventsLocal bool
	CPUStat            string
	Units              []UnitPidsFiles
	// SlicePidsPeak and SlicePidsMax: the slice's own pids.peak and pids.max
	// (read where it has no pids.events.local).
	SlicePidsPeak string
	SlicePidsMax  string
}

// UnitPidsFiles are one unit cgroup's process files.
type UnitPidsFiles struct {
	Events      string
	EventsLocal string
	Peak        string
	Max         string
}

// cgroupValue reads "key value" lines (memory.events, pids.events, cpu.stat).
func cgroupValue(text, key string) (int64, bool) {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == key {
			n, err := strconv.ParseInt(fields[1], 10, 64)
			if err == nil && n >= 0 {
				return n, true
			}
		}
	}
	return 0, false
}

// cgroupLimit reads a one-number file (pids.max, pids.peak; "max" or
// unreadable: -1).
func cgroupLimit(text string) int64 {
	text = strings.TrimSpace(text)
	if text == "" || text == "max" {
		return -1
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// ParseSliceCounters reads the slice's counters from its files (the rules
// above SliceCounters).
func ParseSliceCounters(f SliceCgroupFiles) SliceCounters {
	var c SliceCounters
	c.MemoryLimitHits, _ = cgroupValue(f.MemoryEventsLocal, "oom")
	c.MemoryMaxHits, _ = cgroupValue(f.MemoryEventsLocal, "max")
	c.MemoryOOMAll, _ = cgroupValue(f.MemoryEvents, "oom")
	c.OOMKills, _ = cgroupValue(f.MemoryEvents, "oom_kill")
	if f.HasPidsEventsLocal {
		c.TasksRule = "local"
		c.TasksLimitHits, _ = cgroupValue(f.PidsEventsLocal, "max")
		for _, u := range f.Units {
			n, _ := cgroupValue(u.EventsLocal, "max")
			c.TasksOtherHits += n
		}
	} else {
		c.TasksRule = "units"
		if limit, peak := cgroupLimit(f.SlicePidsMax), cgroupLimit(f.SlicePidsPeak); limit > 0 && peak >= 0 {
			c.SlicePidsPeak, c.SlicePidsMax = peak, limit
		}
		for _, u := range f.Units {
			n, _ := cgroupValue(u.Events, "max")
			if n == 0 {
				continue
			}
			peak := cgroupLimit(u.Peak)
			if limit := cgroupLimit(u.Max); peak >= 0 && (limit < 0 || peak < limit) {
				c.TasksLimitHits += n
			} else {
				c.TasksOtherHits += n
			}
		}
	}
	c.CPUThrottled, _ = cgroupValue(f.CPUStat, "nr_throttled")
	c.CPUThrottledUSec, _ = cgroupValue(f.CPUStat, "throttled_usec")
	c.CPUPeriods, _ = cgroupValue(f.CPUStat, "nr_periods")
	return c
}

// UnitFileSlice reads the slice a unit file's text names (its last Slice=
// line, as systemd reads it; "" when it names none).
func UnitFileSlice(text string) string {
	slice := ""
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Slice="); ok {
			slice = strings.TrimSpace(rest)
		}
	}
	return slice
}
