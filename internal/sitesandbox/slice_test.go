package sitesandbox

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// S4 R2a: the site's slice (docs/SITE-SANDBOX.md section 8).

// TestSiteSliceNameIsDepthOne: the slice's name has no dash, so systemd puts
// it directly below the root and a unit in it is at depth two, where the SMTP
// set matches (a dash would nest it under "celiksite.slice").
func TestSiteSliceNameIsDepthOne(t *testing.T) {
	if got := SliceName(7); got != "celiksite_7.slice" || strings.Contains(strings.TrimSuffix(got, ".slice"), "-") {
		t.Fatalf("slice name = %q", got)
	}
	for name, want := range map[string]int{"celiksite_7.slice": 7, "celiksite_1234567890.slice": 1234567890} {
		if id, ok := ParseSliceName(name); !ok || id != want {
			t.Errorf("%s = %d %v", name, id, ok)
		}
	}
	for _, bad := range []string{"celiksite-7.slice", "celiksite_0.slice", "celiksite_07.slice", "celiksite_7", "system.slice", "celiksite_7.slice/x", "celiksite_.slice",
		"celiksite_7.slice.d", "celiksite_12345678901.slice", "celiksite_9999999999.slice", "x/celiksite_7.slice"} {
		if _, ok := ParseSliceName(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

// TestUnitInSiteSliceRendersItsSlice: a unit placed in its site's slice says
// so; a unit not placed keeps the system slice, byte for byte as before.
func TestUnitInSiteSliceRendersItsSlice(t *testing.T) {
	p := programProfile()
	lines, err := Directives(p, fullCaps(257))
	if err != nil || !has(lines, "Slice=system.slice") {
		t.Fatalf("not placed: %v %v", err, lines)
	}
	before := strings.Join(lines, "\n")
	p.SiteID = 12
	if lines, err := Directives(p, fullCaps(257)); err != nil || strings.Join(lines, "\n") != before {
		t.Fatalf("a site identity alone changed the unit: %v\n%v", err, lines)
	}
	p.InSlice = true
	lines, err = Directives(p, fullCaps(257))
	if err != nil || !has(lines, "Slice=celiksite_12.slice") || has(lines, "Slice=system.slice") {
		t.Fatalf("placed: %v %v", err, lines)
	}
	if got := strings.Replace(strings.Join(lines, "\n"), "Slice=celiksite_12.slice", "Slice=system.slice", 1); got != before {
		t.Fatalf("placing the unit changed another line:\n%s", got)
	}
	transient, err := TransientDirectives(p, fullCaps(255))
	if err != nil || !has(transient, "Slice=celiksite_12.slice") {
		t.Fatalf("transient: %v %v", err, transient)
	}
	for _, line := range transient {
		if refusal := TransientPropertyRefusal(line, 249); refusal != "" {
			t.Errorf("systemd-run 249 would refuse %q: %s", line, refusal)
		}
	}
	// A unit's own limits stay its own: the slice bounds them in the kernel.
	if !has(lines, "MemoryMax=512M") || !has(lines, "TasksMax=128") {
		t.Fatalf("the unit's own limits changed: %v", lines)
	}
	p.SiteID = 0
	if _, err := Directives(p, fullCaps(257)); err == nil {
		t.Fatal("a slice without the site's identity was rendered")
	}
}

// TestSiteSliceUnitText: the native slice carries the three limits, the
// owner's drop-in place and a way to read it without CelikPanel.
func TestSiteSliceUnitText(t *testing.T) {
	text, err := SliceUnitText(12, "shop.example.com", "/etc/systemd/system", SliceLimits{MemoryMB: 1024, Tasks: 256, CPUPercent: 150})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[Slice]\n", "MemoryMax=1024M\n", "MemorySwapMax=0\n", "TasksMax=256\n", "CPUQuota=150%\n",
		"Description=CelikPanel site limits: shop.example.com (site 12)\n",
		"/etc/systemd/system/celiksite_12.slice.d/*.conf", "systemctl status celiksite_12.slice",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("slice text lacks %q:\n%s", want, text)
		}
	}
	// No line systemd would expand or a person could inject.
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		if strings.ContainsAny(line, "$\\\"'") || (strings.Contains(line, "%") && !strings.HasPrefix(line, "CPUQuota=")) {
			t.Errorf("unsafe line %q", line)
		}
	}
	odd, err := SliceUnitText(12, "bad name\nMemoryMax=infinity", "/etc/systemd/system", SliceLimits{MemoryMB: 1024, Tasks: 256, CPUPercent: 150})
	if err != nil || strings.Contains(odd, "infinity") || !strings.Contains(odd, "Description=CelikPanel site limits: site 12\n") {
		t.Fatalf("an odd domain reached the text: %v\n%s", err, odd)
	}
	for _, bad := range []SliceLimits{{MemoryMB: 32, Tasks: 256, CPUPercent: 100}, {MemoryMB: 512, Tasks: 8, CPUPercent: 100}, {MemoryMB: 512, Tasks: 256, CPUPercent: 5}, {MemoryMB: 70000, Tasks: 256, CPUPercent: 100}} {
		if _, err := SliceUnitText(12, "", "/etc/systemd/system", bad); err == nil {
			t.Errorf("limits %+v accepted", bad)
		}
	}
	if _, err := SliceUnitText(0, "", "/etc/systemd/system", SliceLimits{MemoryMB: 512, Tasks: 256, CPUPercent: 100}); err == nil {
		t.Fatal("a slice without a site was rendered")
	}
}

// TestEgressElementFollowsTheUnitsCgroup: the SMTP set's element is the
// cgroup the unit really runs in, the system slice or its site's slice; any
// other place is refused (the unit then does not start).
func TestEgressElementFollowsTheUnitsCgroup(t *testing.T) {
	unit := "celikapp-12.service"
	for cgroup, want := range map[string]string{
		"/system.slice/celikapp-12.service":       `{ "system.slice/celikapp-12.service" }`,
		"/celiksite_12.slice/celikapp-12.service": `{ "celiksite_12.slice/celikapp-12.service" }`,
	} {
		if got, err := CgroupElementFor(cgroup, unit); err != nil || got != want {
			t.Errorf("%s = %q %v", cgroup, got, err)
		}
	}
	for _, bad := range []string{
		"", "/my.slice/celikapp-12.service", "/celiksite.slice/celiksite-12.slice/celikapp-12.service",
		"/celiksite_12.slice/celikapp-13.service", "/celiksite_012.slice/celikapp-12.service", "/system.slice/x/celikapp-12.service",
		"system.slice/celikapp-12.service",
	} {
		if _, err := CgroupElementFor(bad, unit); err == nil {
			t.Errorf("cgroup %q accepted", bad)
		}
	}
	if _, err := CgroupElementFor("/system.slice/nginx.service", "nginx.service"); err == nil {
		t.Fatal("a unit that is not sandboxed got an element")
	}
	// The system slice's element is exactly the one earlier releases added.
	element, _ := CgroupElement("celikapp-12.service")
	if got, _ := CgroupElementFor("/system.slice/celikapp-12.service", "celikapp-12.service"); got != element {
		t.Fatalf("system slice element %q, earlier %q", got, element)
	}
	elements := []string{"celiksite_12.slice/celikapp-12.service", "system.slice/celikrt-3.service"}
	if !InSetCgroup(elements, "/celiksite_12.slice/celikapp-12.service") || !InSetCgroup(elements, "/system.slice/celikrt-3.service") ||
		InSetCgroup(elements, "/system.slice/celikapp-12.service") || InSetCgroup(elements, "") {
		t.Fatal("set membership by cgroup")
	}
}

// TestGuardReaddsUnitsInTheirSitesSlice: the guard re-adds a unit running in
// its site's slice with that cgroup, and still refuses any other place.
func TestGuardReaddsUnitsInTheirSitesSlice(t *testing.T) {
	guard := EgressGuardScriptText()
	for _, want := range []string{`"/system.slice/$unit") allowed=yes`, `/celiksite_*.slice/"$unit")`,
		`[ "$cgroup" = "/celiksite_$site.slice/$unit" ] && allowed=yes`, `"{ \"${cgroup#/}\" }"`} {
		if !strings.Contains(guard, want) {
			t.Errorf("guard lacks %q:\n%s", want, guard)
		}
	}
	// The guard's place check, run by the shell itself on sample cgroups.
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh here")
	}
	start := strings.Index(guard, "\tallowed=no\n")
	end := strings.Index(guard, "\tif [ \"$allowed\" != yes ]")
	if start < 0 || end < start {
		t.Fatalf("the place check is not where it was:\n%s", guard)
	}
	check := "unit=$1; cgroup=$2\n" + guard[start:end] + "echo $allowed\n"
	for cgroup, want := range map[string]string{
		"/system.slice/celikrt-5.service":                      "yes",
		"/celiksite_7.slice/celikrt-5.service":                 "yes",
		"/celiksite_07.slice/celikrt-5.service":                "no",
		"/celiksite_7x.slice/celikrt-5.service":                "no",
		"/celiksite_1/../x.slice/celikrt-5.service":            "no",
		"/celiksite.slice/celiksite-7.slice/celikrt-5.service": "no",
		"/owner.slice/celikrt-5.service":                       "no",
	} {
		out, err := exec.Command(sh, "-c", check, "guard", "celikrt-5.service", cgroup).CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("%s: %q %v, want %s", cgroup, out, err, want)
		}
	}
}

// TestProofAcceptsTheSitesSlice: a unit moved into its site's slice proves;
// one in another site's slice or anywhere else does not, and the set element
// is checked for the cgroup it really runs in.
func TestProofAcceptsTheSitesSlice(t *testing.T) {
	p := programProfile()
	p.SiteID, p.InSlice = 7, true
	c := fullCaps(257)
	unit := "celikrt-5.service"
	chain := "table inet celikpanel_egress {\n\tchain output {\n\t\ttcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n\t}\n}\n"
	in := ProofInput{Unit: unit, Profile: p, Caps: c, Show: showFor(p, unit, c),
		SetElements: []string{"celiksite_7.slice/celikrt-5.service"}, OutputChain: chain, Listen: &ListenProof{Serving: true}}
	if r := Prove(in); !r.OK {
		t.Fatalf("moved unit: %+v", r)
	}
	// The caller rendered it in the slice; a status read whose request does
	// not know yet still proves the unit where it runs.
	notYet := in
	notYet.Profile.InSlice = false
	if r := Prove(notYet); !r.OK {
		t.Fatalf("placement not known to the caller: %+v", r)
	}
	other := in
	other.Show = strings.Replace(in.Show, "/celiksite_7.slice/", "/celiksite_8.slice/", 1)
	if r := Prove(other); r.Code != ProofCgroup {
		t.Fatalf("another site's slice: %+v", r)
	}
	stale := in
	stale.SetElements = []string{"system.slice/celikrt-5.service"}
	if r := Prove(stale); r.Code != ProofNotInSet {
		t.Fatalf("set element of the old place: %+v", r)
	}
	// Without the site's identity only the system slice proves, as before.
	unknown := in
	unknown.Profile.SiteID, unknown.Profile.InSlice = 0, false
	if r := Prove(unknown); r.Code != ProofCgroup {
		t.Fatalf("a slice proved without the site's identity: %+v", r)
	}
}

// TestUnitFileSlice: the Slice= line a unit file names (the last one, as
// systemd reads it).
func TestUnitFileSlice(t *testing.T) {
	for text, want := range map[string]string{
		"[Unit]\nDescription=x\n\n[Service]\nSlice=celiksite_7.slice\nUser=a\n": "celiksite_7.slice",
		"[Service]\nSlice=system.slice\n":                                       "system.slice",
		"[Service]\nSlice=system.slice\nSlice=celiksite_7.slice\n":              "celiksite_7.slice",
		"[Service]\n  Slice = celiksite_7.slice \n":                             "",
		"[Service]\nUser=a\n":                                                   "",
		"":                                                                      "",
	} {
		if got := UnitFileSlice(text); got != want {
			t.Errorf("%q = %q, want %q", text, got, want)
		}
	}
}

// TestTheAgentsPlaceReadingIsTheHelpers: UnitInSiteSlice and
// UnitInSystemSlice are UnitControlGroupAllowed (the one definition of a
// unit's two places, the SMTP helper's too): exactly the unit's own leaf in
// its site's slice or the system slice, nothing nested, no other unit.
func TestTheAgentsPlaceReadingIsTheHelpers(t *testing.T) {
	unit := "celikrt-5.service"
	for _, tc := range []struct {
		cgroup         string
		site, inSystem bool
	}{
		{"/celiksite_7.slice/celikrt-5.service", true, false},
		{"/system.slice/celikrt-5.service", false, true},
		{"/celiksite_8.slice/celikrt-5.service", false, false},
		{"/celiksite_07.slice/celikrt-5.service", false, false},
		{"/celiksite_7.slice/celikrt-5.service/.control", false, false},
		{"/celiksite_7.slice/celikrt-6.service", false, false},
		{"/system.slice/celikrt-6.service", false, false},
		{"/", false, false},
		{"", false, false},
	} {
		if got := UnitInSiteSlice(tc.cgroup, unit, 7); got != tc.site {
			t.Errorf("site slice %q = %v", tc.cgroup, got)
		}
		if got := UnitInSystemSlice(tc.cgroup, unit); got != tc.inSystem {
			t.Errorf("system slice %q = %v", tc.cgroup, got)
		}
	}
	if UnitInSiteSlice("/celiksite_7.slice/nginx.service", "nginx.service", 7) || UnitInSiteSlice("/celiksite_7.slice/celikrt-5.service", unit, 0) {
		t.Fatal("a unit that is not sandboxed, or no site, was placed")
	}
}

// TestSliceCountersAreRead: the slice cgroup's counters, as the kernel
// writes them.
func TestSliceCountersAreRead(t *testing.T) {
	c := ParseSliceCounters(SliceCgroupFiles{
		MemoryEvents:      "low 0\nhigh 0\nmax 41\noom 3\noom_kill 2\noom_group_kill 0\n",
		MemoryEventsLocal: "low 0\nhigh 0\nmax 41\noom 3\noom_kill 0\noom_group_kill 0\n",
		PidsEventsLocal:   "max 17\n", HasPidsEventsLocal: true,
		CPUStat: "usage_usec 900\nuser_usec 600\nsystem_usec 300\nnr_periods 100\nnr_throttled 12\nthrottled_usec 345000\n"})
	if c.OOMKills != 2 || c.MemoryLimitHits != 3 || c.MemoryMaxHits != 41 || c.TasksLimitHits != 17 || c.CPUThrottled != 12 ||
		c.CPUThrottledUSec != 345000 || c.CPUPeriods != 100 {
		t.Fatalf("counters = %+v", c)
	}
	if z := ParseSliceCounters(SliceCgroupFiles{}); z != (SliceCounters{TasksRule: "units"}) {
		t.Fatalf("unreadable files = %+v", z)
	}
}

// TestSliceCountersTellTheSitesLimitApart: the files a real kernel wrote
// (21 Nov 2026, Debian 13 kernel 6.12.105 and Ubuntu 24.04 kernel 6.8.0;
// scratch facts F2 and F3, quoted in docs/SITE-SANDBOX.md section 8). Only
// the site's own limit counts as the site's limit; a program's own smaller
// limit and a host-wide shortage never do.
func TestSliceCountersTellTheSitesLimitApart(t *testing.T) {
	mem := func(max, oom, kill int) string {
		return fmt.Sprintf("low 0\nhigh 0\nmax %d\noom %d\noom_kill %d\noom_group_kill 0\n", max, oom, kill)
	}
	cases := []struct {
		name               string
		files              SliceCgroupFiles
		memLimit, memAll   int64
		kills, tasks, othr int64
	}{
		// F3a, both kernels: slice MemoryMax=128M reached.
		{"slice memory limit", SliceCgroupFiles{MemoryEvents: mem(25, 1, 1), MemoryEventsLocal: mem(25, 1, 0)}, 1, 1, 1, 0, 0},
		// F3b: the unit's own MemoryMax=96M inside a 512M slice.
		{"program's own memory limit", SliceCgroupFiles{MemoryEvents: mem(36, 1, 1), MemoryEventsLocal: mem(0, 0, 0)}, 0, 1, 1, 0, 0},
		// F3d: global OOM (slice limit 8G on a 2G VM).
		{"host-wide shortage", SliceCgroupFiles{MemoryEvents: mem(0, 0, 1), MemoryEventsLocal: mem(0, 0, 0)}, 0, 0, 1, 0, 0},
		// F2a, Debian 6.12: slice TasksMax=30 reached; the slice's local file counts it.
		{"6.12 slice process limit", SliceCgroupFiles{HasPidsEventsLocal: true, PidsEventsLocal: "max 1\n",
			Units: []UnitPidsFiles{{Events: "max 0\n", EventsLocal: "max 0\n", Peak: "31\n", Max: "2338\n"}}}, 0, 0, 0, 1, 0},
		// F2b, Debian 6.12: the unit's own TasksMax=10.
		{"6.12 program's own process limit", SliceCgroupFiles{HasPidsEventsLocal: true, PidsEventsLocal: "max 0\n",
			Units: []UnitPidsFiles{{Events: "max 1\n", EventsLocal: "max 1\n", Peak: "10\n", Max: "10\n"}}}, 0, 0, 0, 0, 1},
		// F2a, Ubuntu 6.8: no local file; the forking unit counts it, its peak below its own max.
		{"6.8 slice process limit", SliceCgroupFiles{
			Units: []UnitPidsFiles{{Events: "max 1\n", Peak: "31\n", Max: "2314\n"}}}, 0, 0, 0, 1, 0},
		// F2c, Ubuntu 6.8: two units share the slice; the second one forks into its limit.
		{"6.8 slice limit shared by two units", SliceCgroupFiles{Units: []UnitPidsFiles{
			{Events: "max 0\n", Peak: "20\n", Max: "2314\n"}, {Events: "max 1\n", Peak: "11\n", Max: "2314\n"}}}, 0, 0, 0, 1, 0},
		// F2b, Ubuntu 6.8: the unit's own TasksMax=10 (peak equals its own max).
		{"6.8 program's own process limit", SliceCgroupFiles{
			Units: []UnitPidsFiles{{Events: "max 1\n", Peak: "10\n", Max: "10\n"}}}, 0, 0, 0, 0, 1},
	}
	for _, tc := range cases {
		c := ParseSliceCounters(tc.files)
		if c.MemoryLimitHits != tc.memLimit || c.MemoryOOMAll != tc.memAll || c.OOMKills != tc.kills || c.TasksLimitHits != tc.tasks || c.TasksOtherHits != tc.othr {
			t.Errorf("%s: %+v", tc.name, c)
		}
	}
}
