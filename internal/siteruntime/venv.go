package siteruntime

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// Environment generations. Every build makes a new venv-<n> beside the
// running one, so a failed build never touches what runs:
//
//	start:    remove leftover venv-*.partial; n = highest generation + 1;
//	          build into venv-<n>.partial (as the site's account, sandboxed)
//	success:  rename venv-<n>.partial to venv-<n> (one rename: a venv either
//	          is complete or is not there); keep venv-<n> and the running
//	          venv-<a>, remove every other generation
//	failure:  remove venv-<n>.partial; the running app is untouched
//	deploy:   the unit is rendered with venv-<n> and restarted
//	rollback: when the restarted app does not start or answer, the unit is
//	          rendered again with venv-<a> (the generation that ran before)
//
// So at most two complete environments and one partial exist, and one step
// back is always possible after a successful build.
//
// Ortam kuşakları: her derleme çalışanın yanında yeni bir venv-<n> yapar;
// başarısız derleme çalışana dokunmaz. Başarıda yeni ve çalışan kalır, diğerleri
// silinir; yeni sürüm açılmazsa birim bir önceki ortama döner.

// MaxVenvGeneration bounds the generation number (venv-1 .. venv-999999).
const MaxVenvGeneration = 999999

var venvNamePattern = regexp.MustCompile(`^venv-([1-9][0-9]{0,5})(\.partial)?$`)

// VenvName is the folder of generation n.
func VenvName(n int) string { return "venv-" + strconv.Itoa(n) }

// PartialVenvName is the folder a build of generation n writes into.
func PartialVenvName(n int) string { return VenvName(n) + ".partial" }

// ParseVenvName reads a folder name of the app folder.
func ParseVenvName(name string) (n int, partial bool, ok bool) {
	m := venvNamePattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 || n > MaxVenvGeneration {
		return 0, false, false
	}
	return n, m[2] != "", true
}

// VenvSet is what the app folder holds: the complete generations (a
// venv-<n> folder with its pyvenv.cfg), the partial ones, and the generation
// the unit runs (0: none).
type VenvSet struct {
	Complete []int
	Partial  []int
	Active   int
}

// NewVenvSet reads the set from the app folder's entry names (only real
// folders are passed: the Agent drops links) and the unit's generation.
func NewVenvSet(dirs []string, active int) VenvSet {
	s := VenvSet{Active: active}
	for _, name := range dirs {
		n, partial, ok := ParseVenvName(name)
		if !ok {
			continue
		}
		if partial {
			s.Partial = append(s.Partial, n)
		} else {
			s.Complete = append(s.Complete, n)
		}
	}
	sort.Ints(s.Complete)
	sort.Ints(s.Partial)
	return s
}

// Next is the generation a new build writes: one above every generation the
// folder or the unit names, so a build never reuses a name in use.
func (s VenvSet) Next() (int, error) {
	high := s.Active
	for _, list := range [][]int{s.Complete, s.Partial} {
		for _, n := range list {
			if n > high {
				high = n
			}
		}
	}
	if high >= MaxVenvGeneration {
		return 0, fmt.Errorf("the environment numbers are used up; remove the app's venv-* folders and deploy again")
	}
	return high + 1, nil
}

// Has reports whether generation n is complete.
func (s VenvSet) Has(n int) bool {
	for _, c := range s.Complete {
		if c == n {
			return true
		}
	}
	return false
}

// Stale lists the partial folders a new build removes first.
func (s VenvSet) Stale() []string {
	out := []string{}
	for _, n := range s.Partial {
		out = append(out, PartialVenvName(n))
	}
	return out
}

// AfterBuild lists the folders removed once generation built is complete:
// every complete generation except built and the one running now (kept for
// the one-step rollback), and every partial except built's own (already
// renamed).
func (s VenvSet) AfterBuild(built int) []string {
	out := []string{}
	for _, n := range s.Complete {
		if n != built && n != s.Active {
			out = append(out, VenvName(n))
		}
	}
	for _, n := range s.Partial {
		if n != built {
			out = append(out, PartialVenvName(n))
		}
	}
	return out
}

// RollbackTarget is the generation a failed deploy of built returns to: the
// one that ran before the build, when it is still complete and is not built
// itself. ok is false when there is nothing to return to (a first deploy).
func RollbackTarget(before, built int, s VenvSet) (int, bool) {
	if before <= 0 || before == built || !s.Has(before) {
		return 0, false
	}
	return before, true
}

// Build phases of the Agent's record of one build.
const (
	BuildPhaseRunning   = "running"
	BuildPhaseSucceeded = "succeeded"
	BuildPhaseFailed    = "failed"
)

// BuildUnit is systemd's view of a build unit (`systemctl show`).
type BuildUnit struct {
	// Found is false when systemd does not know the unit.
	Found        bool
	ActiveState  string
	Result       string
	Job          string // a queued or running job's id ("" when none)
	InvocationID string // "" until the unit has started once
}

// Why a build that did not succeed ended (BuildOutcome).
const (
	BuildEndedNormally = ""
	// BuildLost: systemd does not know the unit, or it ran and someone
	// stopped it; there is no result.
	BuildLost = "lost"
	// BuildNotStarted: the start job ended without running the unit: its
	// bind unit (the sandbox helper) did not start, or the start was
	// cancelled.
	BuildNotStarted = "not_started"
)

// BuildOutcome decides a build unit's phase from systemd's view of it (the
// transient unit is Type=oneshot with RemainAfterExit=yes: "active" with
// result success means every step exited 0).
//
// A unit whose start job is still queued is "inactive" with a job: it waits
// for its bind unit (Requires=, After=). That is a running build, never a
// lost one (alpha.119 lab round 3: the first read right after the start
// took the queued build for a stopped one, stopped it and with it the
// bind unit's helper, so no build ever ran).
func BuildOutcome(u BuildUnit) (phase, why string) {
	switch {
	case !u.Found:
		return BuildPhaseFailed, BuildLost
	case u.Job != "":
		return BuildPhaseRunning, BuildEndedNormally
	case u.ActiveState == "active" && (u.Result == "" || u.Result == "success"):
		return BuildPhaseSucceeded, BuildEndedNormally
	case u.ActiveState == "activating" || u.ActiveState == "reloading":
		return BuildPhaseRunning, BuildEndedNormally
	case u.ActiveState == "failed" || (u.Result != "" && u.Result != "success"):
		return BuildPhaseFailed, BuildEndedNormally
	case u.ActiveState == "inactive" && u.InvocationID == "":
		return BuildPhaseFailed, BuildNotStarted
	case u.ActiveState == "inactive":
		// A oneshot with RemainAfterExit never goes back to inactive by
		// itself after success: someone stopped it.
		return BuildPhaseFailed, BuildLost
	}
	return BuildPhaseRunning, BuildEndedNormally
}
