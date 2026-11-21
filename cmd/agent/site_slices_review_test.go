package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a fix round: the Agent's side of the tests the independent review of
// PR #349 required (site_slices.go, site_slices_move.go,
// site_slices_runtime.go).

// TestAMoveSurvivesAnAgentStopAtEveryStep (review 6): the Agent stops right
// after each step of a move; at its next start the journal finishes or puts
// back every unit by the step it reached, the pre-move bytes are what comes
// back, the outcome is kept for the card, and a later failed move still puts
// back the ORIGINAL bytes.
func TestAMoveSurvivesAnAgentStopAtEveryStep(t *testing.T) {
	running, stopped := "celikrt-5.service", "celiksvc-6.service"
	cases := []struct {
		unit, step   string
		wantRunning  string // the running unit's file afterwards: "before" or "after"
		wantStopped  string
		wantInSlice  bool
		wantOutcome  string
		wantRestarts int // restarts of the running unit in all
	}{
		{"", "journal", "before", "before", false, transport.SiteMoveNotTried, 0},
		{"", "slice", "before", "before", false, transport.SiteMoveNotTried, 0},
		{running, siteMoveStepWritten, "before", "before", false, transport.SiteMovePutBack, 0},
		{running, siteMoveStepRestarting, "before", "before", false, transport.SiteMovePutBack, 1},
		{running, siteMoveStepMoved, "after", "before", true, transport.SiteMoveMoved, 1},
		{stopped, siteMoveStepWritten, "after", "after", true, transport.SiteMoveRewritten, 1},
	}
	for _, tc := range cases {
		t.Run(tc.step+"_"+tc.unit, func(t *testing.T) {
			host := useSliceTestHost(t)
			before := map[string]string{running: sealedTestUnit(running, sliceTestHome+"/app"), stopped: sealedTestUnit(stopped, sliceTestHome+"/apps/n8n")}
			writeTestUnit(t, host, running, before[running])
			writeTestUnit(t, host, stopped, before[stopped])
			host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running}
			host.units[stopped] = &sliceTestUnit{active: "inactive"}
			siteMoveKilled = func(unit, step string) bool { return unit == tc.unit && step == tc.step }
			var resp transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running, stopped), &resp)
			if _, err := os.Stat(siteMoveJournalPath); err != nil {
				t.Fatalf("the stopped Agent left no journal: %v", err)
			}
			// The Agent starts again.
			siteMoveKilled = func(string, string) bool { return false }
			recoverSiteSlicesAtStartup()
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
				t.Fatal("the journal stayed after the recovery")
			}
			check := func(unit, want string) {
				got := unitText(t, filepath.Join(host.dir, unit))
				if want == "before" && got != before[unit] {
					t.Errorf("%s is not its pre-move bytes:\n%s", unit, got)
				}
				if want == "after" && !strings.Contains(got, "\nSlice=celiksite_7.slice\n") {
					t.Errorf("%s does not name the slice:\n%s", unit, got)
				}
			}
			check(running, tc.wantRunning)
			if tc.unit == stopped {
				check(stopped, tc.wantStopped)
			}
			if inSlice := strings.HasPrefix(host.units[running].cgroup, "/celiksite_7.slice/"); tc.unit == running && inSlice != tc.wantInSlice {
				t.Errorf("running unit in %s", host.units[running].cgroup)
			}
			restarts := 0
			for _, c := range host.calls {
				if c == "restart "+running {
					restarts++
				}
			}
			if tc.unit == running && restarts != tc.wantRestarts {
				t.Errorf("%d restarts, want %d: %v", restarts, tc.wantRestarts, host.calls)
			}
			if tc.unit != "" {
				outcomes := readSiteMoveOutcomes()["7"]
				if outcomes.Units[tc.unit] != tc.wantOutcome {
					t.Errorf("recorded outcome %+v, want %s for %s", outcomes, tc.wantOutcome, tc.unit)
				}
				var status transport.SiteSlicesStatusResponse
				_ = (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: 7}}}, &status)
				if status.Sites[0].MoveInterrupted == nil {
					t.Error("the card is not told about the interrupted move")
				}
			}
		})
	}

	// After an interrupted move that was put back, a later move that fails
	// puts back the ORIGINAL bytes, never the moved text.
	host := useSliceTestHost(t)
	original := sealedTestUnit(running, sliceTestHome+"/app")
	writeTestUnit(t, host, running, original)
	host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running}
	siteMoveKilled = func(unit, step string) bool { return unit == running && step == siteMoveStepWritten }
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running), &transport.MoveSiteSliceResponse{})
	siteMoveKilled = func(string, string) bool { return false }
	recoverSiteSlicesAtStartup()
	host.units[running].failStart = true
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running), &resp)
	if !resp.RolledBack || unitText(t, filepath.Join(host.dir, running)) != original || host.units[running].cgroup != "/system.slice/"+running {
		t.Fatalf("later failed move: %+v\n%s", resp, unitText(t, filepath.Join(host.dir, running)))
	}
}

// TestAMoveTakesEachProgramsLockAndLosesNothing (review 7): each unit's step
// holds that program's own mutation lock; a change made to a program while
// the move runs is kept (never written over, never put back over); a unit
// removed meanwhile is never re-created; a site being changed is not moved.
func TestAMoveTakesEachProgramsLockAndLosesNothing(t *testing.T) {
	host := useSliceTestHost(t)
	first, second, third := "celikrt-5.service", "celikrt-8.service", "celiksvc-6.service"
	for _, unit := range []string{first, second, third} {
		dir := sliceTestHome + "/app"
		if unit == third {
			dir = sliceTestHome + "/apps/n8n"
		}
		writeTestUnit(t, host, unit, sealedTestUnit(unit, dir))
		host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	}
	customer := strings.Replace(sealedTestUnit(second, sliceTestHome+"/app"), "MemoryMax=1536M", "MemoryMax=1024M", 1)
	lockHeld := false
	host.onRestart = func(unit string) {
		switch unit {
		case first:
			// While the first program restarts, its lock is held by the move;
			// a customer's change of the second program completes meanwhile.
			lockHeld = !runtimeAppLocks[5%len(runtimeAppLocks)].TryLock()
			if !lockHeld {
				runtimeAppLocks[5%len(runtimeAppLocks)].Unlock()
			}
			writeTestUnit(t, host, second, customer)
		case third:
			// The first program is deleted while the third one restarts.
			_ = os.Remove(filepath.Join(host.dir, first))
		}
	}
	host.units[third].failStart = true
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(first, second, third), &resp)
	if !lockHeld {
		t.Fatal("the move restarted a program without holding its lock")
	}
	outcomes := map[string]string{}
	for _, u := range resp.Units {
		outcomes[u.Unit] = u.Outcome
	}
	if outcomes[second] != transport.SiteMoveChanged || unitText(t, filepath.Join(host.dir, second)) != customer {
		t.Fatalf("the customer's change was lost: %+v\n%s", resp.Units, unitText(t, filepath.Join(host.dir, second)))
	}
	if _, err := os.Stat(filepath.Join(host.dir, first)); !os.IsNotExist(err) {
		t.Fatalf("a deleted program's unit was re-created by the put-back: %+v", resp.Units)
	}
	if outcomes[first] != transport.SiteMoveChanged || !resp.RolledBack {
		t.Fatalf("outcomes: %+v", resp.Units)
	}
	// A site being changed (for example deleted) is not moved at all.
	siteMutationMutex(7).Lock()
	resp = transport.MoveSiteSliceResponse{}
	host.calls = nil
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(third), &resp)
	siteMutationMutex(7).Unlock()
	if resp.Error == "" || !strings.Contains(resp.Error, "being changed") || len(host.calls) != 0 {
		t.Fatalf("a site being changed was moved: %+v %v", resp, host.calls)
	}
}

// TestAMoveReadsDropInsAndWaitsAsTheUnitDoes (review 10): a drop-in that sets
// Slice= is read before anything restarts and leaves the unit to the owner;
// a slow program gets its own start wait (systemd's 90 seconds for these
// units, at least 20, at most 300).
func TestAMoveReadsDropInsAndWaitsAsTheUnitDoes(t *testing.T) {
	for value, want := range map[string]time.Duration{"1min 30s": 90 * time.Second, "infinity": 90 * time.Second, "": 90 * time.Second,
		"5s": 20 * time.Second, "10min": 300 * time.Second, "2min": 120 * time.Second} {
		if got := siteSliceStartWait(value); got != want {
			t.Errorf("TimeoutStartUSec %q: %s, want %s", value, got, want)
		}
	}
	host := useSliceTestHost(t)
	owner, slow := "celikrt-5.service", "celiksvc-6.service"
	writeTestUnit(t, host, owner, sealedTestUnit(owner, sliceTestHome+"/app"))
	writeTestUnit(t, host, slow, sealedTestUnit(slow, sliceTestHome+"/apps/n8n"))
	dropIn := filepath.Join(host.dir, owner+".d", "owner.conf")
	_ = os.MkdirAll(filepath.Dir(dropIn), 0o755)
	_ = os.WriteFile(dropIn, []byte("[Service]\nSlice=system.slice\n"), 0o644)
	host.units[owner] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + owner, dropIns: dropIn}
	host.units[slow] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + slow, slowShows: 30}
	siteSliceSettleMin, siteSliceSettleDefault, siteSliceSettleMax = 2*time.Second, 2*time.Second, 2*time.Second
	siteSlicePoll = 10 * time.Millisecond
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(owner, slow), &resp)
	outcomes := map[string]string{}
	for _, u := range resp.Units {
		outcomes[u.Unit] = u.Outcome
	}
	if outcomes[owner] != transport.SiteMoveOwnerSlice || host.called("restart "+owner) {
		t.Fatalf("the owner's drop-in: %+v", resp.Units)
	}
	if outcomes[slow] != transport.SiteMoveMoved || !resp.Done || resp.WaitSeconds != 2 {
		t.Fatalf("the slow program: %+v", resp)
	}
}

// TestAnAbsentSliceIsWrittenAgainWhenUsed (review 8): the owner deleted the
// slice file of a site whose unit names it; the next change of limits
// writes it again. A site nothing of which uses the slice gets none.
func TestAnAbsentSliceIsWrittenAgainWhenUsed(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, strings.Replace(sealedTestUnit(unit, sliceTestHome+"/app"), "Slice=system.slice", "Slice=celiksite_7.slice", 1))
	var resp transport.ApplySiteSlicesResponse
	other := transport.SiteSlice{SiteID: 8, MemoryMB: 512, Tasks: 128, CPUPercent: 100}
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice(), other}}, &resp)
	if resp.Sites[0].Outcome != transport.SiteFileOutcomeWritten || resp.Sites[1].Outcome != transport.SiteFileAbsent {
		t.Fatalf("outcomes: %+v", resp.Sites)
	}
	if _, err := os.Stat(filepath.Join(host.dir, "celiksite_7.slice")); err != nil {
		t.Fatal("the slice a unit names was not written again")
	}
	// A large change is refused above one batch, never half-applied.
	resp = transport.ApplySiteSlicesResponse{}
	var many []transport.SiteSlice
	for i := 1; i <= siteSliceApplyBatch+1; i++ {
		many = append(many, transport.SiteSlice{SiteID: i, MemoryMB: 512, Tasks: 128, CPUPercent: 100})
	}
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: many}, &resp)
	if resp.Error == "" || len(resp.Sites) != 0 {
		t.Fatalf("an oversized batch: %+v", resp.Error)
	}
}

// TestARunningSliceTakesTheNewLimitAtOnce (stage 1, F4): where the reload
// did not reach the kernel (systemd 257), only the keys that differ are set
// at runtime with systemd's EFFECTIVE value, the control drop-ins made are
// removed again, an owner's own control drop-in is never touched, and where
// the kernel already holds them (systemd 255) nothing is set. A stop between
// the set and the removal is cleaned up at the next start.
func TestARunningSliceTakesTheNewLimitAtOnce(t *testing.T) {
	host := useSliceTestHost(t)
	slice := "celiksite_7.slice"
	cg := filepath.Join(siteSliceCgroupRoot, slice)
	_ = os.MkdirAll(cg, 0o755)
	kernel := func(memory, pids, cpu string) {
		_ = os.WriteFile(filepath.Join(cg, "memory.max"), []byte(memory+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(cg, "memory.swap.max"), []byte("0\n"), 0o644)
		_ = os.WriteFile(filepath.Join(cg, "pids.max"), []byte(pids+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(cg, "cpu.max"), []byte(cpu+"\n"), 0o644)
	}
	kernel("536870912", "64", "100000 100000")
	// systemd's effective values: the slice file (1024M, 256, 150 %) with an
	// owner's drop-in that sets the memory to 768M.
	host.slices[slice] = "MemoryMax=805306368\nMemorySwapMax=0\nTasksMax=256\nCPUQuotaPerSecUSec=1.500000s\n"
	var set [][]string
	var madeBeforeRemoval []string
	host.setProperty = func(args []string) {
		set = append(set, args)
		dir := filepath.Join(siteSliceControlDir, args[2]+".d")
		_ = os.MkdirAll(dir, 0o755)
		for _, a := range args[3:] {
			key, _, _ := strings.Cut(a, "=")
			file := filepath.Join(dir, "50-"+key+".conf")
			_ = os.WriteFile(file, []byte("[Slice]\n"+a+"\n"), 0o644)
			madeBeforeRemoval = append(madeBeforeRemoval, file)
		}
		kernel("805306368", "256", "150000 100000")
	}
	ensureSiteSlice(testSlice())
	if len(set) != 1 || strings.Join(set[0], " ") != "set-property --runtime celiksite_7.slice MemoryMax=805306368 TasksMax=256 CPUQuota=150%" {
		t.Fatalf("set-property calls: %q", set)
	}
	for _, file := range madeBeforeRemoval {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("%s was left", file)
		}
	}
	if _, err := os.Stat(siteSliceRuntimeJournalPath); !os.IsNotExist(err) {
		t.Error("the record of pushed limits stayed")
	}
	// systemd 255: the kernel already holds the values: nothing is set.
	set = nil
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{{SiteID: 7, Domain: "shop.example.com", MemoryMB: 2048, Tasks: 256, CPUPercent: 150}}}, &transport.ApplySiteSlicesResponse{})
	if len(set) != 0 {
		t.Fatalf("set-property where the kernel agreed: %q", set)
	}
	// The owner's own runtime setting of a key is never touched.
	ownerFile := filepath.Join(siteSliceControlDir, slice+".d", "50-TasksMax.conf")
	_ = os.MkdirAll(filepath.Dir(ownerFile), 0o755)
	_ = os.WriteFile(ownerFile, []byte("[Slice]\nTasksMax=99\n"), 0o644)
	kernel("536870912", "99", "150000 100000")
	host.setProperty = func(args []string) { set = append(set, args) }
	pushSiteSlicesToKernel([]int{7})
	if len(set) != 1 || strings.Contains(strings.Join(set[0], " "), "TasksMax") || !strings.Contains(strings.Join(set[0], " "), "MemoryMax=805306368") {
		t.Fatalf("set-property with an owner's runtime setting: %q", set)
	}
	if _, err := os.Stat(ownerFile); err != nil {
		t.Fatal("the owner's runtime setting was removed")
	}
	// An Agent stop between set-property and the removal: the next start
	// removes the recorded files (and only those).
	leftover := filepath.Join(siteSliceControlDir, slice+".d", "50-MemoryMax.conf")
	_ = os.WriteFile(leftover, []byte("[Slice]\nMemoryMax=805306368\n"), 0o644)
	if err := saveSiteSliceRuntimeJournal([]string{leftover, "/etc/passwd"}); err != nil {
		t.Fatal(err)
	}
	recoverSiteSlicesAtStartup()
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("a pushed control drop-in survived the Agent's start")
	}
	if _, err := os.Stat(ownerFile); err != nil {
		t.Fatal("the owner's runtime setting was removed at the start")
	}
}

// TestADeletedSitesSliceGoes (review 13): CelikPanel's unchanged slice file
// goes with the site; an owner's edit and drop-ins are moved aside under
// names systemd does not load; a file the ledger does not know, found for a
// new site's id, is moved aside before the new one is written.
func TestADeletedSitesSliceGoes(t *testing.T) {
	host := useSliceTestHost(t)
	ensureSiteSlice(testSlice())
	file := filepath.Join(host.dir, "celiksite_7.slice")
	if aside, err := removeSiteSlice(7); err != nil || len(aside) != 0 {
		t.Fatalf("remove: %v %v", aside, err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("the unchanged slice file stayed")
	}
	if _, known := readSandboxLedger()[file]; known {
		t.Fatal("the ledger kept the deleted site's slice")
	}
	ensureSiteSlice(testSlice())
	raw, _ := os.ReadFile(file)
	_ = os.WriteFile(file, []byte(strings.Replace(string(raw), "MemoryMax=1024M", "MemoryMax=4096M", 1)), 0o644)
	_ = os.MkdirAll(file+".d", 0o755)
	_ = os.WriteFile(filepath.Join(file+".d", "owner.conf"), []byte("[Slice]\nTasksMax=9\n"), 0o644)
	aside, err := removeSiteSlice(7)
	if err != nil || len(aside) != 2 {
		t.Fatalf("aside: %v %v", aside, err)
	}
	for _, name := range aside {
		if strings.HasSuffix(name, ".slice") || strings.HasSuffix(name, ".slice.d") || !strings.Contains(name, ".celikpanel-aside-") {
			t.Errorf("%s is a name systemd may load", name)
		}
		if _, err := os.Stat(name); err != nil {
			t.Errorf("%s is not kept: %v", name, err)
		}
	}
	// A leftover of an earlier installation (unknown to the ledger, edited).
	_ = os.WriteFile(file, []byte(strings.Replace(string(raw), "MemoryMax=1024M", "MemoryMax=8192M", 1)), 0o644)
	var resp transport.ApplySiteSlicesResponse
	writeTestUnit(t, host, "celikrt-5.service", strings.Replace(sealedTestUnit("celikrt-5.service", sliceTestHome+"/app"), "Slice=system.slice", "Slice=celiksite_7.slice", 1))
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &resp)
	now, _ := os.ReadFile(file)
	if resp.Sites[0].MovedAside == "" || !strings.Contains(string(now), "MemoryMax=1024M") {
		t.Fatalf("leftover: %+v\n%s", resp.Sites[0], now)
	}
}

// TestNothingIsWrittenOnAHostWithoutLimits (review "not confirmed"): without
// the unified cgroup hierarchy or its controllers no slice is written, no
// unit is placed in one, the move is refused and the status says why.
func TestNothingIsWrittenOnAHostWithoutLimits(t *testing.T) {
	root := t.TempDir()
	old := siteSliceCgroupRoot
	t.Cleanup(func() { siteSliceCgroupRoot = old })
	siteSliceCgroupRoot = root
	if got := realSiteSliceHostSupport(); got != "cgroup v2" {
		t.Fatalf("no cgroup.controllers: %q", got)
	}
	_ = os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset cpu io memory\n"), 0o644)
	if got := realSiteSliceHostSupport(); got != "pids" {
		t.Fatalf("no pids controller: %q", got)
	}
	_ = os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset cpu io memory hugetlb pids rdma misc\n"), 0o644)
	if got := realSiteSliceHostSupport(); got != "" {
		t.Fatalf("a full host: %q", got)
	}
	host := useSliceTestHost(t)
	siteSliceHostSupport = func() string { return "cgroup v2" }
	if siteSliceFor(testSlice(), 7) != nil || siteUnitEntersSlice("celikrt-5.service", siteSliceFor(testSlice(), 7), false) {
		t.Fatal("a unit was placed in a slice the host cannot enforce")
	}
	var apply transport.ApplySiteSlicesResponse
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &apply)
	var move transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest("celikrt-5.service"), &move)
	var status transport.SiteSlicesStatusResponse
	_ = (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: 7}}}, &status)
	if apply.Unsupported != "cgroup v2" || move.Error == "" || status.Unsupported != "cgroup v2" {
		t.Fatalf("apply %+v move %+v status %+v", apply, move, status.Unsupported)
	}
	if _, err := os.Stat(filepath.Join(host.dir, sitesandbox.SliceName(7))); !os.IsNotExist(err) {
		t.Fatal("a slice file was written")
	}
}

// TestTheStatusReadTakesFiveThousandSites (review 12): 5000 sites in one read
// complete; a larger request is read up to the bound and says so, never
// refused as a whole; the ledger is read once.
func TestTheStatusReadTakesFiveThousandSites(t *testing.T) {
	useSliceTestHost(t)
	var sites []transport.SiteSliceQuery
	for i := 1; i <= siteSliceMaxSites+1; i++ {
		sites = append(sites, transport.SiteSliceQuery{SiteID: i})
	}
	started := time.Now()
	var resp transport.SiteSlicesStatusResponse
	_ = (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: sites}, &resp)
	if resp.Error != "" || len(resp.Sites) != siteSliceMaxSites || resp.Partial == "" {
		t.Fatalf("error %q, %d sites, partial %q", resp.Error, len(resp.Sites), resp.Partial)
	}
	if elapsed := time.Since(started); elapsed > time.Minute {
		t.Fatalf("5000 sites took %s", elapsed)
	}
}
