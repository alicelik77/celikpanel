package configmatrix

// TestVersionGatedDirectivesHaveASwitch is the minimum-version rule of the
// distro config matrix. deploy/test-distro-config-matrix/version-gates.json
// names configuration syntax that only some program versions accept; the
// checked-in versions/*.json tables say which version every supported release
// ships. A generator or template that emits gated syntax without a version
// switch, while a supported release is outside the gate, fails here — before
// a release reaches a server (nginx `http2 on;` on Ubuntu 24.04's 1.24 was
// the case that motivated it). No network, no host access.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type versionGate struct {
	ID             string   `json:"id"`
	Program        string   `json:"program"`
	Min            string   `json:"min"`
	MaxExclusive   string   `json:"max_exclusive"`
	Pattern        string   `json:"pattern"`
	Files          []string `json:"files"`
	Guard          string   `json:"guard"`
	GuardWindow    int      `json:"guard_window"`
	GuardElsewhere *struct {
		File    string `json:"file"`
		Pattern string `json:"pattern"`
	} `json:"guard_elsewhere"`
	Accepted        string   `json:"accepted"`
	KnownViolations []string `json:"known_violations"`
}

type releaseVersions struct {
	Release  string `json:"release"`
	Packages map[string]struct {
		Upstream *string `json:"upstream"`
	} `json:"packages"`
}

const matrixDir = "../../deploy/test-distro-config-matrix"

// compareVersions compares dotted upstream versions numerically
// (1.24.0 < 1.25.1; 9.18.39 < 9.20.30; 2.4.1 >= 2.4.0).
func compareVersions(a, b string) int {
	split := func(v string) []int {
		var out []int
		for _, part := range regexp.MustCompile(`[0-9]+`).FindAllString(v, -1) {
			n, _ := strconv.Atoi(part)
			out = append(out, n)
		}
		return out
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			if p < q {
				return -1
			}
			return 1
		}
	}
	return 0
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"1.24.0", "1.25.1", -1}, {"1.26.3", "1.25.1", 1}, {"1.25.1", "1.25.1", 0}, {"254", "252", 1}, {"2.3.21", "2.4.0", -1}, {"3.10.13", "3.6", 1}} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func loadReleaseVersions(t *testing.T) []releaseVersions {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(matrixDir, "versions", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no versions/*.json tables (run deploy/test-distro-config-matrix/regenerate-versions.sh): %v", err)
	}
	var out []releaseVersions
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var r releaseVersions
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out = append(out, r)
	}
	return out
}

func TestVersionGatedDirectivesHaveASwitch(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(matrixDir, "version-gates.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Gates []versionGate `json:"gates"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	releases := loadReleaseVersions(t)
	for _, gate := range doc.Gates {
		gate := gate
		t.Run(gate.ID, func(t *testing.T) {
			// Releases whose packaged version is outside the gate.
			var outside []string
			for _, r := range releases {
				pkg, ok := r.Packages[gate.Program]
				if !ok {
					t.Fatalf("program %q is not in versions/%s.json", gate.Program, r.Release)
				}
				if pkg.Upstream == nil {
					continue // not packaged on that release
				}
				v := *pkg.Upstream
				if (gate.Min != "" && compareVersions(v, gate.Min) < 0) ||
					(gate.MaxExclusive != "" && compareVersions(v, gate.MaxExclusive) >= 0) {
					outside = append(outside, r.Release+" ("+gate.Program+" "+v+")")
				}
			}
			sort.Strings(outside)
			pattern := regexp.MustCompile("(?m)" + gate.Pattern)
			var guard *regexp.Regexp
			if gate.Guard != "" {
				guard = regexp.MustCompile(gate.Guard)
			}
			window := gate.GuardWindow
			if window == 0 {
				window = 15
			}
			guardedElsewhere := false
			if g := gate.GuardElsewhere; g != nil {
				data, err := os.ReadFile(filepath.Join("../..", g.File))
				if err != nil {
					t.Fatalf("guard_elsewhere %s: %v", g.File, err)
				}
				guardedElsewhere = regexp.MustCompile(g.Pattern).Match(data)
			}
			violations := map[string][]string{}
			uses := 0
			for _, glob := range gate.Files {
				matches, err := filepath.Glob(filepath.Join("../..", glob))
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range matches {
					if strings.HasSuffix(file, "_test.go") {
						continue
					}
					data, err := os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
					lines := strings.Split(string(data), "\n")
					rel := filepath.ToSlash(strings.TrimPrefix(file, filepath.Clean("../..")+string(filepath.Separator)))
					for i, line := range lines {
						if !pattern.MatchString(line) {
							continue
						}
						uses++
						guarded := guardedElsewhere
						for j := i; j >= 0 && j >= i-window && guard != nil && !guarded; j-- {
							guarded = guard.MatchString(lines[j])
						}
						if !guarded {
							violations[rel] = append(violations[rel], rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
						}
					}
				}
			}
			known := map[string]bool{}
			for _, k := range gate.KnownViolations {
				known[k] = true
			}
			for file, list := range violations {
				if len(outside) == 0 {
					continue
				}
				switch {
				case gate.Accepted != "":
					t.Logf("accepted (%s): %s on %s", gate.Accepted, strings.Join(list, "; "), strings.Join(outside, ", "))
				case known[file]:
					t.Logf("KNOWN VIOLATION (reported, not yet fixed): %s is not accepted by %s", strings.Join(list, "; "), strings.Join(outside, ", "))
				default:
					t.Errorf("%s needs a version switch: it is not accepted by %s (gate %s, %s)",
						strings.Join(list, "; "), strings.Join(outside, ", "), gate.ID, gateRange(gate))
				}
			}
			for file := range known {
				if len(violations[file]) == 0 || len(outside) == 0 {
					t.Errorf("known_violations lists %s for gate %s, but it no longer violates the gate: remove the entry from version-gates.json", file, gate.ID)
				}
			}
			t.Logf("%d use(s); releases outside the gate: %v", uses, outside)
		})
	}
}

func gateRange(g versionGate) string {
	switch {
	case g.Min != "" && g.MaxExclusive != "":
		return g.Program + " >= " + g.Min + " and < " + g.MaxExclusive
	case g.Min != "":
		return g.Program + " >= " + g.Min
	default:
		return g.Program + " < " + g.MaxExclusive
	}
}
