package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a second fix round: the Agent's side of what the second review of PR
// #349 found (review B1, B2, B4, B6, B8, B9, B10).

func moveTestUnits(t *testing.T, host *sliceTestHost) (running, stopped string, before map[string]string) {
	t.Helper()
	running, stopped = "celikrt-5.service", "celiksvc-6.service"
	before = map[string]string{running: sealedTestUnit(running, sliceTestHome+"/app"), stopped: sealedTestUnit(stopped, sliceTestHome+"/apps/n8n")}
	writeTestUnit(t, host, running, before[running])
	writeTestUnit(t, host, stopped, before[stopped])
	host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running}
	host.units[stopped] = &sliceTestUnit{active: "inactive"}
	return running, stopped, before
}

// TestTheMoveIsRecordedBeforeEveryWrite (B1): the Agent is killed between
// every pair of writes of a move. At its next start every unit file is either
// its pre-move bytes or the move's text recorded as moved (or rewritten, for
// a stopped unit): never "not tried" while it names the slice. A running
// program ends where it ran, or inside the slice when it moved. The slice
// file the move made goes again when nothing names it and it holds what the
// move wrote.
func TestTheMoveIsRecordedBeforeEveryWrite(t *testing.T) {
	type point struct{ unit, step string }
	running, stopped := "celikrt-5.service", "celiksvc-6.service"
	var points []point
	for _, step := range []string{"journal", "slice", "slice_recorded"} {
		points = append(points, point{"", step})
	}
	for _, step := range []string{"writing", "wrote", "written", "restarting", "restarted", "moved"} {
		points = append(points, point{running, step})
	}
	for _, step := range []string{"writing", "wrote", "written"} {
		points = append(points, point{stopped, step})
	}
	for _, p := range points {
		t.Run(p.step+"_"+p.unit, func(t *testing.T) {
			host := useSliceTestHost(t)
			_, _, before := moveTestUnits(t, host)
			siteMoveKilled = func(unit, step string) bool { return unit == p.unit && step == p.step }
			var resp transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running, stopped), &resp)
			if _, err := os.Stat(siteMoveJournalPath); err != nil {
				t.Fatalf("the killed Agent left no record: %v (%+v)", err, resp)
			}
			siteMoveKilled = func(string, string) bool { return false }
			recoverSiteSlicesAtStartup()
			if _, err := os.Stat(siteMoveJournalPath); err == nil && p.step == "restarted" {
				// alpha.149 fix round (F1, F3): the start only requested the
				// put-back's restart ("unverified"); the next status read sees
				// it running where it ran and settles the record (second fix
				// round, R1: once it has run for its start wait).
				if got := readSiteMoveOutcomes()["7"].Units[running]; got != transport.SiteMoveUnverified {
					t.Fatalf("after the start: %q", got)
				}
				host.advance(time.Minute)
				round3Status(t, 7)
			}
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
				t.Fatal("the record stayed after a complete recovery")
			}
			outcomes := readSiteMoveOutcomes()["7"].Units
			for _, unit := range []string{running, stopped} {
				got := unitText(t, filepath.Join(host.dir, unit))
				names := strings.Contains(got, "\nSlice=celiksite_7.slice\n")
				switch {
				case got == before[unit]:
				case names && (outcomes[unit] == transport.SiteMoveMoved || outcomes[unit] == transport.SiteMoveRewritten):
				default:
					t.Errorf("%s after the recovery (outcome %q):\n%s", unit, outcomes[unit], got)
				}
				if names && outcomes[unit] == transport.SiteMoveNotTried {
					t.Errorf("%s names the slice but is recorded as not tried", unit)
				}
			}
			cg := host.units[running].cgroup
			moved := outcomes[running] == transport.SiteMoveMoved
			if (moved && cg != "/celiksite_7.slice/"+running) || (!moved && (cg != "/system.slice/"+running || host.units[running].active != "active")) {
				t.Errorf("the running program is %s in %s (outcome %q)", host.units[running].active, cg, outcomes[running])
			}
			named := siteSliceNamedByUnitFile(7)
			if _, err := os.Stat(filepath.Join(host.dir, "celiksite_7.slice")); !named && p.step != "slice" && err == nil {
				t.Error("the slice file the move made stayed although nothing names it")
			}
		})
	}
}

// failJournalFrom makes every record of the move from the n-th one fail, as a
// full disk would.
func failJournalFrom(t *testing.T, n int) *int {
	t.Helper()
	calls := 0
	old := siteMoveJournalWrite
	t.Cleanup(func() { siteMoveJournalWrite = old })
	siteMoveJournalWrite = func(path string, raw []byte) error {
		calls++
		if calls >= n {
			return errors.New("no space left on device")
		}
		return old(path, raw)
	}
	return &calls
}

// TestAFullDiskAtAnyMoveRecordStopsTheMove (B1): a record that cannot be
// written is a failed step: nothing further is written, what was written is
// put back to its pre-move bytes, the running program runs where it ran, and
// the answer says the move failed.
func TestAFullDiskAtAnyMoveRecordStopsTheMove(t *testing.T) {
	for n := 1; n <= 8; n++ {
		t.Run(fmt.Sprint("record_", n), func(t *testing.T) {
			host := useSliceTestHost(t)
			running, stopped, before := moveTestUnits(t, host)
			calls := failJournalFrom(t, n)
			var resp transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running, stopped), &resp)
			if *calls < n {
				// The move finished before the n-th record: it must be whole.
				if !resp.Done {
					t.Fatalf("a move with every record written: %+v", resp)
				}
				return
			}
			if resp.Done || resp.Error == "" {
				t.Fatalf("a move whose record failed answered done: %+v", resp)
			}
			for _, unit := range []string{running, stopped} {
				if got := unitText(t, filepath.Join(host.dir, unit)); got != before[unit] {
					t.Errorf("%s is not its pre-move bytes after a failed record:\n%s", unit, got)
				}
			}
			if u := host.units[running]; u.active != "active" || u.cgroup != "/system.slice/"+running {
				t.Errorf("the running program is %s in %s", u.active, u.cgroup)
			}
		})
	}
}

// TestALedgerThatCannotBeReadWritesNothing (B4): a ledger that cannot be read
// is an error, never an empty ledger: no slice is written, nothing is removed
// or moved aside, the ledger itself is not written over, and each caller says
// so. A write for an existing site never moves the owner's drop-ins aside,
// whatever the ledger knows.
func TestALedgerThatCannotBeReadWritesNothing(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, strings.Replace(sealedTestUnit(unit, sliceTestHome+"/app"), "Slice=system.slice", "Slice=celiksite_7.slice", 1))
	file := filepath.Join(host.dir, "celiksite_7.slice")
	dropIn := filepath.Join(file+".d", "owner.conf")
	_ = os.MkdirAll(filepath.Dir(dropIn), 0o755)
	_ = os.WriteFile(dropIn, []byte("[Slice]\nMemoryMax=infinity\n"), 0o644)
	// A readable ledger that does not know the file: the write keeps the
	// owner's drop-in where it is.
	ensureSiteSlice(testSlice())
	if _, err := os.Stat(dropIn); err != nil {
		t.Fatal("a write for an existing site moved the owner's drop-in aside")
	}
	_ = os.Remove(file)
	if err := os.MkdirAll(filepath.Dir(sandboxLedgerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandboxLedgerPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var apply transport.ApplySiteSlicesResponse
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &apply)
	if apply.Error == "" || !strings.Contains(apply.Error, "could not be read") {
		t.Fatalf("apply with an unreadable ledger: %+v", apply)
	}
	ensureSiteSlice(testSlice())
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a slice was written on an unreadable ledger")
	}
	if aside, err := setAsideEarlierSiteSlice(7); err == nil || len(aside) != 0 {
		t.Fatalf("set aside on an unreadable ledger: %v %v", aside, err)
	}
	if _, err := removeSiteSlice(7); err == nil {
		t.Fatal("a removal on an unreadable ledger answered done")
	}
	if _, err := os.Stat(dropIn); err != nil {
		t.Fatal("the owner's drop-in was moved on an unreadable ledger")
	}
	if err := setSandboxLedgerEntry("/x", "k", "d"); err == nil {
		t.Fatal("an unreadable ledger was written over")
	}
	if raw, _ := os.ReadFile(sandboxLedgerPath); string(raw) != "{not json" {
		t.Fatalf("the ledger was written over: %s", raw)
	}
	var move transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &move)
	if move.Error == "" || move.Done {
		t.Fatalf("a move on an unreadable ledger: %+v", move)
	}
}

// TestTheStatusSaysWhetherTheSliceIsUsed (B2): a site whose programs were
// never moved (no unit names the slice, no file) is not "named"; one whose
// unit names the slice is. A file CelikPanel wrote and the owner deleted is
// written again by the next change even when no unit names it, so the card's
// sentences about what writes it are true.
func TestTheStatusSaysWhetherTheSliceIsUsed(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	read := func() transport.SiteSliceStatus {
		var status transport.SiteSlicesStatusResponse
		_ = (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: 7, Units: []string{unit}}}}, &status)
		return status.Sites[0]
	}
	if s := read(); s.Named || s.File != transport.SiteFileAbsent {
		t.Fatalf("a site never moved: %+v", s)
	}
	writeTestUnit(t, host, unit, strings.Replace(sealedTestUnit(unit, sliceTestHome+"/app"), "Slice=system.slice", "Slice=celiksite_7.slice", 1))
	if s := read(); !s.Named {
		t.Fatalf("a unit names the slice: %+v", s)
	}
	// Written, then deleted by the owner while no unit names it.
	ensureSiteSlice(testSlice())
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	file := filepath.Join(host.dir, "celiksite_7.slice")
	_ = os.Remove(file)
	var apply transport.ApplySiteSlicesResponse
	_ = (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &apply)
	if apply.Sites[0].Outcome != transport.SiteFileOutcomeWritten {
		t.Fatalf("a file CelikPanel wrote and the owner deleted: %+v", apply.Sites)
	}
}

// TestAMoveFitsTheLongestStartWait (B8): a program that may take 300 seconds
// to start fits a move from the start; a program that does not fit what is
// left is not started, the ones moved stay moved and the answer says the time
// ran out.
func TestAMoveFitsTheLongestStartWait(t *testing.T) {
	slow := siteMoveUnit{running: true, wait: 300 * time.Second}
	if !siteMoveFits(siteMoveBudget, slow, nil) {
		t.Fatal("a 300 second program can never be moved")
	}
	if siteMoveFits(siteMoveBudget-2*slow.wait, slow, []siteMoveUnit{slow}) {
		t.Fatal("the put-back of the program moved before was not counted")
	}
	if siteMoveBudget+30*time.Second >= 12*time.Minute+30*time.Second {
		t.Fatal("the move's budget and its put-backs pass the Panel's bound")
	}
	host := useSliceTestHost(t)
	first, second := "celikrt-5.service", "celikrt-8.service"
	for _, unit := range []string{first, second} {
		writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
		host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	}
	siteSliceSettleMin, siteSliceSettleDefault, siteSliceSettleMax = 300*time.Millisecond, 300*time.Millisecond, 300*time.Millisecond
	oldMargin := siteMoveMargin
	t.Cleanup(func() { siteMoveMargin = oldMargin })
	siteMoveBudget, siteMoveMargin = 900*time.Millisecond, 5*time.Millisecond
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(first, second), &resp)
	outcomes := map[string]string{}
	for _, u := range resp.Units {
		outcomes[u.Unit] = u.Outcome
	}
	if !resp.TimeUp || resp.Done || outcomes[first] != transport.SiteMoveMoved || outcomes[second] != transport.SiteMoveNotTried {
		t.Fatalf("a move out of time: %+v", resp)
	}
	if !strings.Contains(resp.Error, first) || host.units[first].cgroup != "/celiksite_7.slice/"+first {
		t.Fatalf("the moved program: %q in %s", resp.Error, host.units[first].cgroup)
	}
}

// TestASiteWithManyProgramsIsRead (B9): a site with more programs than one
// move takes is read whole; the read never fails for it.
func TestASiteWithManyProgramsIsRead(t *testing.T) {
	useSliceTestHost(t)
	var units []string
	for i := 1; i <= 150; i++ {
		units = append(units, fmt.Sprintf("celikrt-%d.service", i))
	}
	var resp transport.SiteSlicesStatusResponse
	_ = (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: 7, Units: units}, {SiteID: 8}}}, &resp)
	if resp.Error != "" || len(resp.Sites) != 2 || len(resp.Sites[0].Units) != 150 || resp.Sites[0].UnitsUnread != 0 {
		t.Fatalf("error %q, %d sites", resp.Error, len(resp.Sites))
	}
}

// TestProgramLocksAreTheirOwn (B10): a program's lock and a site's lock are
// never shared with another id.
func TestProgramLocksAreTheirOwn(t *testing.T) {
	pairs := []struct {
		name string
		a, b func() bool
	}{
		{"runtime apps", func() bool { return runtimeAppLocks.of(5).TryLock() }, func() bool { return runtimeAppLocks.of(5 + 64).TryLock() }},
		{"service apps", func() bool { return serviceAppLocks.of(6).TryLock() }, func() bool { return serviceAppLocks.of(6 + 64).TryLock() }},
		{"Node.js apps", func() bool { return appUnitMutationLocks.of(7).TryLock() }, func() bool { return appUnitMutationLocks.of(7 + 256).TryLock() }},
		{"sites", func() bool { return siteMutationMutex(7).TryLock() }, func() bool { return siteMutationMutex(7 + 256).TryLock() }},
	}
	for _, p := range pairs {
		if !p.a() || !p.b() {
			t.Errorf("%s: two ids share a lock", p.name)
		}
	}
	runtimeAppLocks.of(5).Unlock()
	runtimeAppLocks.of(69).Unlock()
	serviceAppLocks.of(6).Unlock()
	serviceAppLocks.of(70).Unlock()
	appUnitMutationLocks.of(7).Unlock()
	appUnitMutationLocks.of(263).Unlock()
	siteMutationMutex(7).Unlock()
	siteMutationMutex(263).Unlock()
}

// TestTheAgentRemovesPushedLimitsFirst (B6): the runtime drop-ins a stop left
// are removed right after the instance lock, before anything else of the
// Agent's start reads or writes limits.
func TestTheAgentRemovesPushedLimitsFirst(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	lock, cleanup, manager := strings.Index(src, "acquireAgentInstanceLock("), strings.Index(src, "\tcleanupSiteSliceRuntime()\n"), strings.Index(src, "agentServiceMutationManager()")
	if lock < 0 || cleanup < 0 || manager < 0 || !(lock < cleanup && cleanup < manager) {
		t.Fatalf("instance lock at %d, runtime cleanup at %d, mutation manager at %d", lock, cleanup, manager)
	}
}
