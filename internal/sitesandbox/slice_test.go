package sitesandbox

import (
	"os/exec"
	"strings"
	"testing"
)

// Forward compatibility with the site's slice of S4 R2a (slice.go): this
// release accepts and keeps a unit a later release placed in
// celiksite_<id>.slice. The tests are R2a's own, for the parts ported here.

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
	for _, bad := range []string{"celiksite_0.slice", "celiksite_07.slice", "celiksite-7.slice", "celiksite_7.slice.d", "celiksite_.slice",
		"system.slice", "celiksite_12345678901.slice", "celiksite_9999999999.slice", "x/celiksite_7.slice"} {
		if id, ok := ParseSliceName(bad); ok {
			t.Errorf("%s accepted as %d", bad, id)
		}
	}
}

// TestUnitInSiteSliceRendersItsSlice: a unit kept in its site's slice
// renders that slice; every other unit renders the system slice, exactly as
// before.
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
		t.Fatalf("keeping the slice changed another line:\n%s", got)
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
	// The system slice's element is exactly the one this release always added.
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

// TestProofAcceptsTheSitesSlice: a unit in its site's slice proves; one in
// another site's slice or anywhere else does not, and the set element is
// checked for the cgroup it really runs in.
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
	// The caller rendered it in the system slice; a status read still proves
	// the unit where it runs.
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
