package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a: the site's slice on this server (site_slices.go,
// docs/SITE-SANDBOX.md section 8).

// sliceTestUnit is one unit as the fake systemd sees it.
type sliceTestUnit struct {
	active string
	cgroup string
	// failStart: a restart in the slice leaves it failed.
	failStart bool
}

type sliceTestHost struct {
	mu      sync.Mutex
	dir     string
	units   map[string]*sliceTestUnit
	calls   []string
	slices  map[string]string // slice name -> show block
	ports   map[string]map[int]bool
	reloads int
}

func useSliceTestHost(t *testing.T) *sliceTestHost {
	t.Helper()
	useSandboxHost(t, true)
	dir, _ := sandboxUnitDir()
	host := &sliceTestHost{dir: dir, units: map[string]*sliceTestUnit{}, slices: map[string]string{}, ports: map[string]map[int]bool{}}
	oldCtl, oldVerify, oldRoot, oldListen, oldSettle, oldPoll, oldPrepare := appUnitSystemctl, appUnitVerify, siteSliceCgroupRoot, siteSliceListening, siteSliceSettle, siteSlicePoll, siteMovePrepare
	t.Cleanup(func() {
		appUnitSystemctl, appUnitVerify, siteSliceCgroupRoot, siteSliceListening, siteSliceSettle, siteSlicePoll, siteMovePrepare = oldCtl, oldVerify, oldRoot, oldListen, oldSettle, oldPoll, oldPrepare
	})
	siteMovePrepare = func() error { return nil }
	appUnitVerify = func(string) error { return nil }
	siteSliceCgroupRoot = t.TempDir()
	siteSliceSettle, siteSlicePoll = 50*time.Millisecond, time.Millisecond
	siteSliceListening = func(unit string) map[int]bool {
		host.mu.Lock()
		defer host.mu.Unlock()
		out := map[int]bool{}
		if u := host.units[unit]; u != nil && u.active == "active" {
			for p := range host.ports[unit] {
				out[p] = true
			}
		}
		return out
	}
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.calls = append(host.calls, strings.Join(args, " "))
		switch {
		case args[0] == "daemon-reload":
			host.reloads++
		case args[0] == "restart" && len(args) == 2:
			u := host.units[args[1]]
			if u == nil {
				return []byte("Unit not found."), errors.New("exit status 5")
			}
			raw, _ := os.ReadFile(filepath.Join(host.dir, args[1]))
			slice := sitesandbox.SystemSlice
			for _, line := range strings.Split(string(raw), "\n") {
				if rest, ok := strings.CutPrefix(line, "Slice="); ok {
					slice = rest
				}
			}
			u.cgroup = "/" + slice + "/" + args[1]
			u.active = "active"
			if u.failStart && slice != sitesandbox.SystemSlice {
				u.active, u.cgroup = "failed", ""
			}
		case args[0] == "show" && len(args) == 3 && args[2] == "--property=ActiveState,ControlGroup":
			u := host.units[args[1]]
			if u == nil {
				return []byte("ActiveState=inactive\nControlGroup=\n"), nil
			}
			return []byte("ActiveState=" + u.active + "\nControlGroup=" + u.cgroup + "\n"), nil
		case args[0] == "show" && strings.HasPrefix(args[1], "--property=Id,"):
			var blocks []string
			for _, name := range args[2:] {
				if block, ok := host.slices[name]; ok {
					blocks = append(blocks, "Id="+name+"\n"+block)
					continue
				}
				if u := host.units[name]; u != nil {
					blocks = append(blocks, "Id="+name+"\nLoadState=loaded\nActiveState="+u.active+"\nControlGroup="+u.cgroup+
						"\nSlice=system.slice\nMemoryMax=1610612736\nTasksMax=512\nCPUQuotaPerSecUSec=1s\n")
					continue
				}
				blocks = append(blocks, "Id="+name+"\nLoadState=not-found\nActiveState=inactive\n")
			}
			return []byte(strings.Join(blocks, "\n\n") + "\n"), nil
		}
		return nil, nil
	}
	return host
}

func (host *sliceTestHost) called(call string) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	for _, c := range host.calls {
		if c == call {
			return true
		}
	}
	return false
}

func testSlice() *transport.SiteSlice {
	return &transport.SiteSlice{SiteID: 7, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}
}

// TestSiteSliceFileFollowsTheOwnerRule: the slice is written with the
// header and the site's limits and reloaded once; a change of limits
// replaces CelikPanel's own text; an owner's edit is kept, with
// CelikPanel's text beside it; a slice that does not exist yet is not made
// by a change of limits.
func TestSiteSliceFileFollowsTheOwnerRule(t *testing.T) {
	host := useSliceTestHost(t)
	s := testSlice()
	ensureSiteSlice(s)
	file := filepath.Join(host.dir, "celiksite_7.slice")
	raw, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(string(raw), services.ManagedHeaderPrefixHash) || !strings.Contains(string(raw), "MemoryMax=1024M\n") ||
		!strings.Contains(string(raw), "TasksMax=256\n") || !strings.Contains(string(raw), "CPUQuota=150%\n") || host.reloads != 1 {
		t.Fatalf("slice file: %v reloads=%d\n%s", err, host.reloads, raw)
	}
	ensureSiteSlice(s)
	if host.reloads != 1 {
		t.Fatal("an unchanged slice reloaded systemd")
	}
	var resp transport.ApplySiteSlicesResponse
	changed := *s
	changed.MemoryMB = 2048
	other := transport.SiteSlice{SiteID: 8, MemoryMB: 512, Tasks: 128, CPUPercent: 100}
	if err := (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{changed, other}}, &resp); err != nil || resp.Error != "" {
		t.Fatalf("apply: %v %+v", err, resp)
	}
	raw, _ = os.ReadFile(file)
	if resp.Sites[0].Outcome != transport.SiteFileOutcomeWritten || !strings.Contains(string(raw), "MemoryMax=2048M\n") || host.reloads != 2 {
		t.Fatalf("changed limits: %+v reloads=%d", resp.Sites, host.reloads)
	}
	if resp.Sites[1].Outcome != transport.SiteFileAbsent {
		t.Fatalf("a site without a slice: %+v", resp.Sites[1])
	}
	if _, err := os.Stat(filepath.Join(host.dir, "celiksite_8.slice")); !os.IsNotExist(err) {
		t.Fatal("a change of limits made a slice nobody runs in")
	}
	// The owner raises the memory by hand: kept, reported, not overwritten.
	edited := strings.Replace(string(raw), "MemoryMax=2048M", "MemoryMax=4096M", 1)
	if err := os.WriteFile(file, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = transport.ApplySiteSlicesResponse{}
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*s}}, &resp)
	after, _ := os.ReadFile(file)
	if resp.Sites[0].Outcome != transport.SiteFileOutcomeKept || string(after) != edited || resp.Sites[0].PendingPath == "" {
		t.Fatalf("owner's edit: %+v", resp.Sites[0])
	}
	if state, _ := siteSliceFileState(7); state != transport.SiteFileOwnerEdited {
		t.Fatalf("state = %s", state)
	}
	// Out of range limits never reach a file.
	resp = transport.ApplySiteSlicesResponse{}
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{{SiteID: 9, MemoryMB: 1, Tasks: 1, CPUPercent: 1}}}, &resp)
	if resp.Sites[0].Outcome != transport.SiteFileOutcomeFailed {
		t.Fatalf("bad limits: %+v", resp.Sites[0])
	}
}

// TestSiteUnitEntersSliceOnlyWhenItDoesNotRun: a unit that does not run is
// written into its site's slice; a running one stays where it runs (systemd
// does not move a running unit), unless it already runs there or the
// administrator's move restarts it.
func TestSiteUnitEntersSliceOnlyWhenItDoesNotRun(t *testing.T) {
	host := useSliceTestHost(t)
	s := testSlice()
	unit := "celikrt-5.service"
	if !siteUnitEntersSlice(unit, s, false) {
		t.Fatal("a unit that does not exist yet stays outside")
	}
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	if siteUnitEntersSlice(unit, s, false) {
		t.Fatal("a running unit in the system slice was written into the site's slice")
	}
	if !siteUnitEntersSlice(unit, s, true) {
		t.Fatal("the administrator's move did not place the unit")
	}
	host.units[unit].cgroup = "/celiksite_7.slice/" + unit
	if !siteUnitEntersSlice(unit, s, false) {
		t.Fatal("a unit that runs in its slice was written back to the system slice")
	}
	host.units[unit] = &sliceTestUnit{active: "failed"}
	if !siteUnitEntersSlice(unit, s, false) {
		t.Fatal("a failed unit stays outside")
	}
	if siteUnitEntersSlice(unit, nil, false) || siteUnitEntersSlice("nginx.service", s, false) {
		t.Fatal("no slice, or not a site's unit")
	}
	if siteSliceFor(s, 8) != nil || siteSliceFor(&transport.SiteSlice{SiteID: 7, MemoryMB: 10, Tasks: 256, CPUPercent: 100}, 7) != nil {
		t.Fatal("a slice of another site or out of range was accepted")
	}
}

// sealedTestUnit is CelikPanel's unchanged text of a sandboxed unit.
func sealedTestUnit(unit, appDir string) string {
	body := "# a unit\n" + sitesandbox.GenerationMarker + "3\n[Unit]\nRequires=" + sitesandbox.BindUnit(unit) + "\n\n[Service]\nSlice=system.slice\n" +
		"BindPaths=" + sitesandbox.BindSource(unit) + ":" + appDir + "\nMemoryMax=1536M\n"
	if strings.HasPrefix(unit, "celiksvc-") {
		return serviceapps.Seal(body)
	}
	return services.SealManagedTextWith(services.ManagedHeaderPrefixHash, body)
}

func writeTestUnit(t *testing.T, host *sliceTestHost, unit, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(host.dir, unit), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func moveRequest(units ...string) *transport.MoveSiteSliceRequest {
	return &transport.MoveSiteSliceRequest{RequestID: "r1", Site: *testSlice(), SubscriptionID: 3, DomainID: 70, Units: units}
}

const sliceTestHome = "/var/www/celikpanel/subscriptions/3/sites/70"

// TestMoveSiteSliceMovesRunningUnitsOnce: a running unit restarts once in the
// slice and listens again; a stopped one is only rewritten; an owner's edit,
// another site's folder and a missing unit are listed, never changed.
func TestMoveSiteSliceMovesRunningUnitsOnce(t *testing.T) {
	host := useSliceTestHost(t)
	rt, svc, app, foreign := "celikrt-5.service", "celiksvc-6.service", "celikapp-7.service", "celikrt-9.service"
	writeTestUnit(t, host, rt, sealedTestUnit(rt, sliceTestHome+"/app"))
	writeTestUnit(t, host, svc, sealedTestUnit(svc, sliceTestHome+"/apps/n8n"))
	edited := strings.Replace(sealedTestUnit(app, sliceTestHome+"/public_html"), "MemoryMax=1536M", "MemoryMax=4G", 1)
	writeTestUnit(t, host, app, edited)
	writeTestUnit(t, host, foreign, sealedTestUnit(foreign, "/var/www/celikpanel/subscriptions/4/sites/80/app"))
	host.units[rt] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + rt}
	host.units[svc] = &sliceTestUnit{active: "inactive"}
	host.units[app] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + app}
	host.ports[rt] = map[int]bool{30010: true}

	var resp transport.MoveSiteSliceResponse
	if err := (&Agent{}).MoveSiteSliceV1(moveRequest(rt, svc, app, foreign, "celikrt-11.service"), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Done || resp.Error != "" || resp.SliceFile != transport.SiteFileOutcomeWritten {
		t.Fatalf("move: %+v", resp)
	}
	outcomes := map[string]string{}
	for _, u := range resp.Units {
		outcomes[u.Unit] = u.Outcome
	}
	want := map[string]string{rt: transport.SiteMoveMoved, svc: transport.SiteMoveRewritten, app: transport.SiteMoveKept,
		foreign: transport.SiteMoveRefused, "celikrt-11.service": transport.SiteMoveAbsent}
	for unit, outcome := range want {
		if outcomes[unit] != outcome {
			t.Errorf("%s = %q, want %q (%+v)", unit, outcomes[unit], outcome, resp.Units)
		}
	}
	if host.units[rt].cgroup != "/celiksite_7.slice/"+rt || !host.called("restart "+rt) {
		t.Fatalf("the running unit did not restart in the slice: %+v", host.units[rt])
	}
	if host.called("restart "+svc) || host.units[svc].active != "inactive" {
		t.Fatal("a stopped unit was started")
	}
	if host.called("restart "+app) || unitText(t, filepath.Join(host.dir, app)) != edited {
		t.Fatal("the owner's unit was touched")
	}
	for _, unit := range []string{rt, svc} {
		text := unitText(t, filepath.Join(host.dir, unit))
		if _, ours := siteUnitSealedBody(unit, text); !ours || !strings.Contains(text, "\nSlice=celiksite_7.slice\n") || strings.Contains(text, "Slice=system.slice") {
			t.Errorf("%s after the move:\n%s", unit, text)
		}
	}
	// A second press changes nothing.
	host.calls = nil
	resp = transport.MoveSiteSliceResponse{}
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(rt, svc), &resp)
	for _, u := range resp.Units {
		if u.Outcome != transport.SiteMoveAlready {
			t.Errorf("second move of %s = %s", u.Unit, u.Outcome)
		}
	}
	if host.called("restart " + rt) {
		t.Fatal("a unit already in its slice was restarted again")
	}
}

// TestMoveSiteSlicePutsTheSiteBack: a unit that does not come up in the
// slice is put back byte for byte, and so is every unit of the site moved
// before it; the ones after it are not touched.
func TestMoveSiteSlicePutsTheSiteBack(t *testing.T) {
	host := useSliceTestHost(t)
	first, second, third := "celikrt-5.service", "celiksvc-6.service", "celikrt-8.service"
	texts := map[string]string{
		first:  sealedTestUnit(first, sliceTestHome+"/app"),
		second: sealedTestUnit(second, sliceTestHome+"/apps/n8n"),
		third:  sealedTestUnit(third, sliceTestHome+"/app"),
	}
	for unit, text := range texts {
		writeTestUnit(t, host, unit, text)
		host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	}
	host.units[second].failStart = true
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(first, second, third), &resp)
	if resp.Done || !resp.RolledBack || !strings.Contains(resp.Error, second) {
		t.Fatalf("move: %+v", resp)
	}
	outcomes := map[string]transport.SiteSliceMoved{}
	for _, u := range resp.Units {
		outcomes[u.Unit] = u
	}
	if outcomes[first].Outcome != transport.SiteMovePutBack || !outcomes[first].PutBack ||
		outcomes[second].Outcome != transport.SiteMoveFailed || !outcomes[second].PutBack || outcomes[second].Step != "restart" ||
		outcomes[third].Outcome != transport.SiteMoveNotTried {
		t.Fatalf("outcomes: %+v", resp.Units)
	}
	for unit, text := range texts {
		if got := unitText(t, filepath.Join(host.dir, unit)); got != text {
			t.Errorf("%s was not put back byte for byte:\n%s", unit, got)
		}
	}
	for _, unit := range []string{first, second} {
		if host.units[unit].cgroup != "/system.slice/"+unit || host.units[unit].active != "active" {
			t.Errorf("%s does not run where it ran: %+v", unit, host.units[unit])
		}
	}
	if host.called("restart " + third) {
		t.Fatal("a unit after the failure was touched")
	}
}

// TestMoveSiteSliceAnswerCheck: a unit that runs in the slice but no longer
// listens on the port it listened on before is put back.
func TestMoveSiteSliceAnswerCheck(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	text := sealedTestUnit(unit, sliceTestHome+"/app")
	writeTestUnit(t, host, unit, text)
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	host.ports[unit] = map[int]bool{30010: true}
	siteSliceListening = func(u string) map[int]bool {
		host.mu.Lock()
		defer host.mu.Unlock()
		if strings.HasPrefix(host.units[u].cgroup, "/celiksite_") {
			return map[int]bool{}
		}
		return map[int]bool{30010: true}
	}
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
	if resp.Done || len(resp.Units) != 1 || resp.Units[0].Step != "answer" || !resp.Units[0].PutBack || unitText(t, filepath.Join(host.dir, unit)) != text {
		t.Fatalf("move: %+v", resp)
	}
}

// TestSiteSlicesStatusReadsCountersAndPlacement: one systemctl call for
// every slice and unit; the slice cgroup's counters; where each unit runs.
func TestSiteSlicesStatusReadsCountersAndPlacement(t *testing.T) {
	host := useSliceTestHost(t)
	ensureSiteSlice(testSlice())
	host.slices["celiksite_7.slice"] = "LoadState=loaded\nActiveState=active\nMemoryMax=1073741824\nTasksMax=256\nCPUQuotaPerSecUSec=1.500000s\nMemoryCurrent=314572800\nTasksCurrent=40\n"
	host.slices["celiksite_8.slice"] = "LoadState=not-found\nActiveState=inactive\nMemoryMax=infinity\nTasksMax=infinity\nCPUQuotaPerSecUSec=infinity\n"
	dir := filepath.Join(siteSliceCgroupRoot, "celiksite_7.slice")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "memory.events"), []byte("low 0\nhigh 0\nmax 9\noom 1\noom_kill 1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "pids.events"), []byte("max 4\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "cpu.stat"), []byte("nr_periods 50\nnr_throttled 5\nthrottled_usec 7000\n"), 0o644)
	in, out := "celikrt-5.service", "celiksvc-6.service"
	writeTestUnit(t, host, in, sealedTestUnit(in, sliceTestHome+"/app"))
	writeTestUnit(t, host, out, sealedTestUnit(out, sliceTestHome+"/apps/n8n"))
	host.units[in] = &sliceTestUnit{active: "active", cgroup: "/celiksite_7.slice/" + in}
	host.units[out] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + out}
	host.calls = nil
	var resp transport.SiteSlicesStatusResponse
	req := &transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: 7, Units: []string{in, out, "celikapp-7.service"}}, {SiteID: 8}}}
	if err := (&Agent{}).SiteSlicesStatusV1(req, &resp); err != nil || resp.Error != "" || len(resp.Sites) != 2 {
		t.Fatalf("status: %v %+v", err, resp)
	}
	if len(host.calls) != 1 {
		t.Fatalf("systemd was asked %d times: %v", len(host.calls), host.calls)
	}
	site := resp.Sites[0]
	if site.File != transport.SiteFileManagedUnchanged || !site.Active || site.Live != (transport.SiteSliceLimitsRead{MemoryMB: 1024, Tasks: 256, CPUPercent: 150}) ||
		site.MemoryCurrentMB != 300 || site.TasksCurrent != 40 || !site.CountersRead ||
		site.Counters != (transport.SiteSliceCounters{OOMKills: 1, MemoryMaxHits: 9, TasksMaxHits: 4, CPUThrottled: 5, CPUThrottledUSec: 7000, CPUPeriods: 50}) {
		t.Fatalf("site 7: %+v", site)
	}
	placements := map[string]string{}
	for _, u := range site.Units {
		placements[u.Unit] = u.Placement
	}
	if placements[in] != transport.SiteUnitInSlice || placements[out] != transport.SiteUnitOutside || placements["celikapp-7.service"] != transport.SiteUnitAbsent {
		t.Fatalf("placements: %+v", site.Units)
	}
	if site.Units[1].Own.MemoryMB != 1536 || site.Units[1].Own.CPUPercent != 100 {
		t.Fatalf("own limits: %+v", site.Units[1])
	}
	empty := resp.Sites[1]
	if empty.File != transport.SiteFileAbsent || empty.Active || empty.CountersRead || empty.Live != (transport.SiteSliceLimitsRead{}) {
		t.Fatalf("site 8: %+v", empty)
	}
	for _, bad := range []*transport.SiteSlicesStatusRequest{{Sites: []transport.SiteSliceQuery{{SiteID: 0}}}, {Sites: []transport.SiteSliceQuery{{SiteID: 7, Units: []string{"nginx.service"}}}}} {
		resp = transport.SiteSlicesStatusResponse{}
		_ = (&Agent{}).SiteSlicesStatusV1(bad, &resp)
		if resp.Error == "" {
			t.Errorf("request %+v accepted", bad)
		}
	}
}

// TestNodeSiteUnitGoesIntoItsSitesSlice: a new Node.js site's unit names its
// site's slice and the slice is written first; a running application in the
// system slice keeps it on an ordinary change (it moves with its site); the
// request of an older Panel (no slice) changes nothing.
func TestNodeSiteUnitGoesIntoItsSitesSlice(t *testing.T) {
	host := useNodeTestHost(t)
	running := false
	inner := appUnitSystemctl
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		if len(args) == 3 && args[0] == "show" && args[2] == "--property=ActiveState,ControlGroup" {
			if running {
				return []byte("ActiveState=active\nControlGroup=/system.slice/" + args[1] + "\n"), nil
			}
			return []byte("ActiveState=inactive\nControlGroup=\n"), nil
		}
		return inner(args...)
	}
	req := withNode(t, nodeRequest())
	req.Trigger = transport.SiteFileTriggerCreate
	req.Slice = &transport.SiteSlice{SiteID: 7, Domain: "example.com", MemoryMB: 3072, Tasks: 512, CPUPercent: 200}
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil || resp.Error != "" {
		t.Fatalf("new site: %v %+v", err, resp)
	}
	text := unitText(t, host.path())
	if !strings.Contains(text, "\nSlice=celiksite_7.slice\n") {
		t.Fatalf("unit:\n%s", text)
	}
	units, _ := sandboxUnitDir()
	if slice := unitText(t, filepath.Join(units, "celiksite_7.slice")); !strings.Contains(slice, "MemoryMax=3072M\n") {
		t.Fatalf("slice:\n%s", slice)
	}
	// The application now runs in the system slice (an earlier start): an
	// ordinary change keeps it there.
	running = true
	req.Trigger, req.Command = transport.SiteFileTriggerChange, "node server.js"
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	text = unitText(t, host.path())
	if resp.Error != "" || !strings.Contains(text, "\nSlice=system.slice\n") || !strings.Contains(text, `"node server.js"`) {
		t.Fatalf("change of a running application: %+v\n%s", resp, text)
	}
	// An older Panel's request names no slice.
	req.Slice = nil
	running = false
	req.Command = "node app.js"
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if text = unitText(t, host.path()); !strings.Contains(text, "\nSlice=system.slice\n") {
		t.Fatalf("older Panel:\n%s", text)
	}
}

func TestSystemdValuesRead(t *testing.T) {
	for value, want := range map[string]int{"2s": 200, "1.500000s": 150, "500ms": 50, "1s 500ms": 150, "infinity": 0, "": 0, "x": 0} {
		if got := systemdCPUPercent(value); got != want {
			t.Errorf("CPUQuotaPerSecUSec %q = %d, want %d", value, got, want)
		}
	}
	if systemdBytesMB("536870912") != 512 || systemdBytesMB("infinity") != 0 || systemdBytesMB("18446744073709551615") != 0 {
		t.Fatal("bytes")
	}
	if systemdCount("512") != 512 || systemdCount("infinity") != 0 {
		t.Fatal("count")
	}
}
