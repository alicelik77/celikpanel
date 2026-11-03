package serviceapps

import (
	"fmt"
	"strconv"
	"strings"
)

// Availability reasons (svcapps.unavailable.<code> on screen). Each one says
// who acts: the administrator (runtime_missing, venv_missing), nobody on this
// operating system (os_unsupported), or nobody in this release (deferred).
const (
	ReasonDeferred      = "deferred"
	ReasonRuntimeAbsent = "runtime_missing"
	ReasonOSUnsupported = "os_unsupported"
	ReasonVenvMissing   = "venv_missing"
	ReasonProbeFailed   = "probe_failed"
)

// NodeCandidate is one Node.js found on the server: the Panel's verified
// runtime under /opt/celikpanel/runtimes/node/<version> (official nodejs.org
// tarballs checked against SHASUMS256.txt, runtime_rpc.go) or the
// distribution's own /usr/bin/node. NPMCLI is npm's entry script beside it.
type NodeCandidate struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	NPMCLI  string `json:"npm_cli"`
	Source  string `json:"source"` // "panel" or "distribution"
}

// PythonCandidate is one distribution Python 3 found on the server.
type PythonCandidate struct {
	Path    string `json:"path"`
	Version string `json:"version"` // "3.11"
	Venv    bool   `json:"venv"`    // the venv and ensurepip modules import
}

// RuntimeChoice is the runtime an install uses, or why there is none.
type RuntimeChoice struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	// Detail names what was found ("Python 3.13", "Node.js 20.19.2"), so
	// the reason is concrete.
	Detail  string `json:"detail,omitempty"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	NPMCLI  string `json:"npm_cli,omitempty"`
	Source  string `json:"source,omitempty"`
}

func majorOf(version string) int {
	head, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	n, err := strconv.Atoi(head)
	if err != nil {
		return -1
	}
	return n
}

func minorOf(version string) (int, int) {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 3)
	if len(parts) < 2 {
		return -1, -1
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return -1, -1
	}
	return major, minor
}

func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// ChooseRuntime decides which runtime an app uses on this server. It never
// adds a package source: a distribution whose Node.js or Python is outside the
// app's supported versions makes the app unavailable here, with the reason.
// The Panel's own Node.js runtime is preferred to the distribution's because
// it is the one the administrator installs and offers for Node.js sites.
func ChooseRuntime(app App, nodes []NodeCandidate, pythons []PythonCandidate) RuntimeChoice {
	if app.Deferred != "" {
		return RuntimeChoice{Reason: ReasonDeferred, Detail: app.Deferred}
	}
	switch app.Runtime {
	case RuntimeNode:
		var best *NodeCandidate
		found := []string{}
		for i := range nodes {
			candidate := nodes[i]
			found = append(found, "Node.js "+candidate.Version)
			ok := false
			for _, major := range app.NodeMajors {
				if majorOf(candidate.Version) == major {
					ok = true
				}
			}
			if !ok || candidate.NPMCLI == "" {
				continue
			}
			if best == nil ||
				(candidate.Source == "panel" && best.Source != "panel") ||
				(candidate.Source == best.Source && compareVersions(candidate.Version, best.Version) > 0) {
				best = &nodes[i]
			}
		}
		if best == nil {
			detail := "none"
			if len(found) > 0 {
				detail = strings.Join(found, ", ")
			}
			return RuntimeChoice{Reason: ReasonRuntimeAbsent, Detail: fmt.Sprintf("needs Node.js %s; found %s", joinInts(app.NodeMajors), detail)}
		}
		return RuntimeChoice{Available: true, Path: best.Path, Version: best.Version, NPMCLI: best.NPMCLI, Source: best.Source}
	case RuntimePython:
		var best *PythonCandidate
		found := []string{}
		venvMissing := false
		for i := range pythons {
			candidate := pythons[i]
			found = append(found, "Python "+candidate.Version)
			major, minor := minorOf(candidate.Version)
			ok := false
			for _, want := range app.PythonMinors {
				if major == 3 && minor == want {
					ok = true
				}
			}
			if !ok {
				continue
			}
			if !candidate.Venv {
				venvMissing = true
				continue
			}
			if best == nil || compareVersions(candidate.Version, best.Version) > 0 {
				best = &pythons[i]
			}
		}
		if best != nil {
			return RuntimeChoice{Available: true, Path: best.Path, Version: best.Version, Source: "distribution"}
		}
		want := make([]string, 0, len(app.PythonMinors))
		for _, minor := range app.PythonMinors {
			want = append(want, fmt.Sprintf("3.%d", minor))
		}
		if venvMissing {
			return RuntimeChoice{Reason: ReasonVenvMissing, Detail: "the distribution's Python venv module is not installed"}
		}
		detail := "none"
		if len(found) > 0 {
			detail = strings.Join(found, ", ")
		}
		return RuntimeChoice{Reason: ReasonOSUnsupported, Detail: fmt.Sprintf("needs Python %s; this system has %s", strings.Join(want, " or "), detail)}
	}
	return RuntimeChoice{Reason: ReasonOSUnsupported, Detail: "unknown runtime"}
}

func joinInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, " or ")
}
