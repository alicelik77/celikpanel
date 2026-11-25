package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a third fix round: the Agent's side of what the third review of PR
// #349 found (N2 recovery that could not finish, N3 the kernel's values, N4
// the unit writer, the move's restart bound, the set-aside at creation).

// round3Interrupted leaves an interrupted move of site 7 whose running
// program was restarted in the slice and not yet seen running there (the
// Agent stopped right after the restart was sent).
func round3Interrupted(t *testing.T) (*sliceTestHost, string, string) {
	t.Helper()
	host := useSliceTestHost(t)
	running := "celikrt-5.service"
	before := sealedTestUnit(running, sliceTestHome+"/app")
	writeTestUnit(t, host, running, before)
	host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running}
	siteMoveKilled = func(unit, step string) bool { return unit == running && step == "restarted" }
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running), &transport.MoveSiteSliceResponse{})
	siteMoveKilled = func(string, string) bool { return false }
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatalf("no journal: %v", err)
	}
	return host, running, before
}

// failing wraps the fake systemctl: while fail(args) answers true the call
// fails as systemctl would.
func failing(fail func(args []string) bool) {
	inner := appUnitSystemctl
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		if fail(args) {
			return []byte("Job failed. See journalctl -xeu for details."), errors.New("exit status 1")
		}
		return inner(args...)
	}
}

func round3Status(t *testing.T, siteID int) transport.SiteSliceStatus {
	t.Helper()
	var status transport.SiteSlicesStatusResponse
	if err := (&Agent{}).SiteSlicesStatusV1(&transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{{SiteID: siteID}}}, &status); err != nil || status.Error != "" {
		t.Fatalf("status: %v %s", err, status.Error)
	}
	return status.Sites[0]
}

// TestARecoveryThatCannotRestartKeepsItsRecord (N2): the put-back's restart
// fails at the Agent's start: the record stays, the program is "failed" on
// the card (kept), the next move of any site is refused with that reason;
// "Try again" after the program can start finishes it and the record goes.
func TestARecoveryThatCannotRestartKeepsItsRecord(t *testing.T) {
	host, running, before := round3Interrupted(t)
	broken := true
	failing(func(args []string) bool { return broken && args[0] == "restart" && args[len(args)-1] == running })

	recoverSiteSlicesAtStartup()
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed although the program did not start again")
	}
	if got := unitText(t, filepath.Join(host.dir, running)); got != before {
		t.Fatalf("the pre-move bytes are not back:\n%s", got)
	}
	status := round3Status(t, 7)
	if status.MoveInterrupted == nil || !status.MoveInterrupted.Kept || status.MoveInterrupted.Units[running] != transport.SiteMoveFailed {
		t.Fatalf("the card is not told the program is stopped: %+v", status.MoveInterrupted)
	}
	// The next move of any site is refused while the record stays.
	other := moveRequest(running)
	other.Site.SiteID, other.RequestID = 9, "r2"
	var refused transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(other, &refused)
	if !refused.Kept || refused.Done || !strings.Contains(refused.Error, "site 7") {
		t.Fatalf("a new move went ahead: %+v", refused)
	}
	// "Try again" while it still cannot start: kept, nothing else.
	var again transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r3", Recover: true}, &again)
	if !again.Kept || again.Done || again.Recovery == nil || again.Recovery.Units[running] != transport.SiteMoveFailed {
		t.Fatalf("try again while it cannot start: %+v", again)
	}
	// The program can start now: "Try again" finishes the put-back.
	broken = false
	var done transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r4", Recover: true}, &done)
	if !done.Done || done.Kept {
		t.Fatalf("try again: %+v", done)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed after the put-back finished")
	}
	if host.units[running].cgroup != "/system.slice/"+running {
		t.Fatalf("the program runs in %s", host.units[running].cgroup)
	}
	if status := round3Status(t, 7); status.MoveInterrupted == nil || status.MoveInterrupted.Kept || status.MoveInterrupted.Units[running] != transport.SiteMovePutBack {
		t.Fatalf("the card after the put-back: %+v", status.MoveInterrupted)
	}
}

// TestAnUnknownPlaceOrAFailedReloadRestartsNothing (N2, N4): systemd cannot
// say where the program runs, or its daemon-reload fails: nothing is
// restarted, the record stays and the program is "unknown" on the card.
func TestAnUnknownPlaceOrAFailedReloadRestartsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail func(running string, args []string) bool
	}{
		{"status read fails", func(running string, args []string) bool {
			return args[0] == "show" && len(args) == 3 && args[1] == running
		}},
		{"daemon-reload fails", func(running string, args []string) bool { return args[0] == "daemon-reload" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host, running, _ := round3Interrupted(t)
			failing(func(args []string) bool { return tc.fail(running, args) })
			host.mu.Lock()
			host.calls = nil
			host.mu.Unlock()
			recoverSiteSlicesAtStartup()
			if host.called("restart " + running) {
				t.Fatalf("a program was restarted on an unknown reading: %v", host.calls)
			}
			if _, err := os.Stat(siteMoveJournalPath); err != nil {
				t.Fatal("the record was removed")
			}
			outcomes := readSiteMoveOutcomes()["7"]
			if !outcomes.Kept || outcomes.Units[running] != transport.SiteMoveUnknown {
				t.Fatalf("outcome: %+v", outcomes)
			}
		})
	}
}

// TestTheMovesRestartIsBoundedByItsStartWait (the review's B8 residue): a
// program whose start never answers (TimeoutStartSec=infinity) holds the move
// no longer than its start wait, twice (the move's restart and the
// put-back's); it is reported still starting and the record is kept.
func TestTheMovesRestartIsBoundedByItsStartWait(t *testing.T) {
	host := useSliceTestHost(t)
	running := "celikrt-5.service"
	writeTestUnit(t, host, running, sealedTestUnit(running, sliceTestHome+"/app"))
	host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running}
	release := make(chan struct{})
	host.onRestart = func(string) { <-release }
	defer close(release)
	start := time.Now()
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(running), &resp)
	took := time.Since(start)
	if took > 2*time.Second {
		t.Fatalf("the move waited %v for a start that never answers", took)
	}
	if !resp.RolledBack || !resp.Kept || len(resp.Units) != 1 || resp.Units[0].Step != "restart" || !strings.Contains(resp.Units[0].Detail, "still starting") {
		t.Fatalf("answer: %+v", resp)
	}
	if strings.Contains(resp.Error, "put back as it was") {
		t.Fatalf("the answer says put back for what was not: %s", resp.Error)
	}
	if outcomes := readSiteMoveOutcomes()["7"]; !outcomes.Kept || outcomes.Units[running] != transport.SiteMoveStarting {
		t.Fatalf("outcome: %+v", outcomes)
	}
}

// round3Cgroup writes a running slice's three limit files.
func round3Cgroup(t *testing.T, slice, memory, pids, cpu string) string {
	t.Helper()
	dir := filepath.Join(siteSliceCgroupRoot, slice)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"memory.max": memory, "pids.max": pids, "cpu.max": cpu, "memory.swap.max": "0"} {
		if value == "" {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestTheStatusReadsTheKernel (N3): "in force" is what the slice's cgroup
// holds: equal values are in force (the kernel's page rounding of memory
// too); a differing value, or systemd not having read the file, is pending;
// a file that cannot be read is unknown; a slice that does not run says
// nothing.
func TestTheStatusReadsTheKernel(t *testing.T) {
	host := useSliceTestHost(t)
	slice := "celiksite_7.slice"
	ensureSiteSlice(testSlice()) // the file on disk: 1024M, 256, 150 %
	show := "LoadState=loaded\nActiveState=active\nFragmentPath=" + filepath.Join(host.dir, slice) + "\nNeedDaemonReload=no\n" +
		"MemoryMax=1073741824\nTasksMax=256\nCPUQuotaPerSecUSec=1.500000s\n"
	host.slices[slice] = show
	if s := round3Status(t, 7); s.KernelState != "" {
		t.Fatalf("a slice that does not run: %q", s.KernelState)
	}
	round3Cgroup(t, slice, "1073741824", "256", "150000 100000")
	if s := round3Status(t, 7); s.KernelState != transport.SiteSliceInForce || s.Kernel.MemoryMB != 1024 || s.Kernel.Tasks != 256 || s.Kernel.CPUPercent != 150 {
		t.Fatalf("equal values: %+v %+v", s.KernelState, s.Kernel)
	}
	host.slices[slice] = strings.Replace(show, "MemoryMax=1073741824", "MemoryMax=1073745000", 1)
	if s := round3Status(t, 7); s.KernelState != transport.SiteSliceInForce {
		t.Fatalf("the kernel's page rounding: %q", s.KernelState)
	}
	// The file was lowered and reloaded, the kernel still holds the earlier value.
	host.slices[slice] = strings.Replace(show, "MemoryMax=1073741824", "MemoryMax=268435456", 1)
	if s := round3Status(t, 7); s.KernelState != transport.SiteSlicePending || s.Kernel.MemoryMB != 1024 {
		t.Fatalf("a value the kernel does not hold yet: %q %+v", s.KernelState, s.Kernel)
	}
	// systemd has not read the file on disk (a reload that failed).
	host.slices[slice] = strings.Replace(strings.Replace(show, "MemoryMax=1073741824", "MemoryMax=infinity", 1), "FragmentPath="+filepath.Join(host.dir, slice), "FragmentPath=", 1)
	round3Cgroup(t, slice, "max", "max", "max 100000")
	if s := round3Status(t, 7); s.KernelState != transport.SiteSlicePending {
		t.Fatalf("a file systemd has not read: %q", s.KernelState)
	}
	host.slices[slice] = strings.Replace(show, "NeedDaemonReload=no", "NeedDaemonReload=yes", 1)
	round3Cgroup(t, slice, "1073741824", "256", "150000 100000")
	if s := round3Status(t, 7); s.KernelState != transport.SiteSlicePending {
		t.Fatalf("a file changed since the reload: %q", s.KernelState)
	}
	// A file that cannot be read is never "in force".
	host.slices[slice] = show
	round3Cgroup(t, slice, "1073741824", "", "150000 100000")
	if s := round3Status(t, 7); s.KernelState != transport.SiteSliceKernelUnknown {
		t.Fatalf("an unreadable cgroup: %q", s.KernelState)
	}
}

// TestAPushThatDoesNotTakeIsAFailedOutcome (N3): a limit change whose
// runtime push fails (set-property refused, its record not written, or the
// kernel still holding the earlier value after it) is a failed outcome with
// the reason, never "written"; the same request sent again (the Panel's
// hourly round) pushes a slice whose file did not change, and is written
// when the kernel then holds the values.
func TestAPushThatDoesNotTakeIsAFailedOutcome(t *testing.T) {
	host := useSliceTestHost(t)
	slice := "celiksite_7.slice"
	ensureSiteSlice(testSlice())
	dir := round3Cgroup(t, slice, "1073741824", "256", "150000 100000")
	host.slices[slice] = "MemoryMax=268435456\nMemorySwapMax=0\nTasksMax=256\nCPUQuotaPerSecUSec=1.500000s\n"
	lowered := transport.SiteSlice{SiteID: 7, Domain: "shop.example.com", MemoryMB: 256, Tasks: 256, CPUPercent: 150}
	apply := func() transport.SiteSliceApplied {
		t.Helper()
		var resp transport.ApplySiteSlicesResponse
		if err := (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{lowered}}, &resp); err != nil || resp.Error != "" || len(resp.Sites) != 1 {
			t.Fatalf("apply: %v %+v", err, resp)
		}
		return resp.Sites[0]
	}
	// set-property refused.
	refuse := true
	failing(func(args []string) bool { return refuse && args[0] == "set-property" })
	if got := apply(); got.Outcome != transport.SiteFileOutcomeFailed || !strings.Contains(got.Detail, "set-property") {
		t.Fatalf("a refused push: %+v", got)
	}
	// set-property answers but the kernel keeps the earlier value.
	refuse = false
	if got := apply(); got.Outcome != transport.SiteFileOutcomeFailed || !strings.Contains(got.Detail, "memory.max") {
		t.Fatalf("a push the kernel did not take: %+v", got)
	}
	// The record of the push cannot be written (a full disk).
	siteSliceRuntimeJournalPath = filepath.Join(t.TempDir(), "missing", "dir", "runtime.json")
	_ = os.WriteFile(filepath.Dir(filepath.Dir(siteSliceRuntimeJournalPath)), nil, 0o644) // a file where its folder would be
	if got := apply(); got.Outcome != transport.SiteFileOutcomeFailed || !strings.Contains(got.Detail, "record") {
		t.Fatalf("a push without its record: %+v", got)
	}
	siteSliceRuntimeJournalPath = filepath.Join(t.TempDir(), "runtime.json")
	// The kernel takes it now: the unchanged file's slice is pushed and written.
	host.setProperty = func(args []string) { _ = os.WriteFile(filepath.Join(dir, "memory.max"), []byte("268435456\n"), 0o644) }
	if got := apply(); got.Outcome == transport.SiteFileOutcomeFailed {
		t.Fatalf("a push the kernel took: %+v", got)
	}
}

// TestUnitFilesAreWrittenThroughTheSyncedWriter (N4): the move writes and
// puts back unit files through services.WriteFileDurably (synced before the
// rename, folder synced after it: internal/services
// TestWriteFileDurablySyncsBeforeTheRename), never a plain write and rename.
func TestUnitFilesAreWrittenThroughTheSyncedWriter(t *testing.T) {
	raw, err := os.ReadFile("site_slices.go")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(raw), "func writeUnitBytes(")
	if !ok {
		t.Fatal("writeUnitBytes is gone")
	}
	body, _, _ = strings.Cut(body, "\n}\n")
	if !strings.Contains(body, "services.WriteFileDurably(") || strings.Contains(body, "os.WriteFile") || strings.Contains(body, "os.Rename") {
		t.Fatalf("writeUnitBytes does not use the synced writer:\n%s", body)
	}
	host := useSliceTestHost(t)
	file := filepath.Join(host.dir, "celikrt-5.service")
	if err := writeUnitBytes(file, "unit\n", 0o644); err != nil || unitText(t, file) != "unit\n" {
		t.Fatalf("write: %v", err)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", info.Mode())
	}
}

// TestAnEarlierSitesFileThatCannotBeSetAsideRefusesTheSite (the review's B4
// residue): at site creation, a limits file of the new id that cannot be
// classified (the ledger unreadable) is a refused step with the file and the
// reason; with no such file the creation goes on whatever the ledger.
func TestAnEarlierSitesFileThatCannotBeSetAsideRefusesTheSite(t *testing.T) {
	host := useSliceTestHost(t)
	if err := os.MkdirAll(filepath.Dir(sandboxLedgerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandboxLedgerPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if aside, err := setAsideEarlierSiteSlice(13); err != nil || len(aside) != 0 {
		t.Fatalf("no file of the id: %v %v", aside, err)
	}
	file := filepath.Join(host.dir, "celiksite_13.slice")
	if err := os.WriteFile(file, []byte("[Slice]\nMemoryMax=8G\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := setAsideEarlierSiteSlice(13); err == nil || !strings.Contains(err.Error(), file) {
		t.Fatalf("an unclassified earlier file: %v", err)
	}
	// CreateSite refuses with the file and the reason, before anything of
	// the site is made.
	withLifecycleTestBuild(t)
	req := validLifecycleCreateRequest(t) // site 13
	home, _ := hostingpath.SiteHome(req.SubscriptionID, req.DomainID)
	fake := &fakeSiteLifecycle{home: home, failures: map[string]error{}}
	agent := lifecycleTestAgent(t, fake)
	agent.siteOps.setAsideSiteSlice = setAsideEarlierSiteSlice
	reply := &transport.CreateSiteResponse{}
	if err := agent.CreateSite(req, reply); err != nil {
		t.Fatal(err)
	}
	if reply.Success || reply.ErrorCode != transport.SiteConfigExists || !strings.Contains(reply.ErrorDetail, file) {
		t.Fatalf("the site was not refused: %+v", reply)
	}
	for _, call := range fake.calls {
		if call == "create-user" || call == "mkdir-all" || call == "apply-vhost" {
			t.Fatalf("something of the site was made: %v", fake.calls)
		}
	}
	if raw, _ := os.ReadFile(file); string(raw) != "[Slice]\nMemoryMax=8G\n" {
		t.Fatal("the earlier file was changed")
	}
}

// TestATaskOfTheNextReleaseNamesItsSlice (S4 R2a third fix round): a server
// that comes back to this release from the next one (R2b) has its sites'
// scheduled tasks as celikcron-<site>-<n>.service units naming their site's
// slice. Such a unit file counts as naming the slice: the status says so (an
// absent file is "missing", not "outside"), a change of limits writes an
// absent file again, and a recovery never removes a slice the tasks name. A
// task unit naming another site's slice does not count.
func TestATaskOfTheNextReleaseNamesItsSlice(t *testing.T) {
	host := useSliceTestHost(t)
	task := "[Unit]\nDescription=CelikPanel task 1 of shop.example.com\n\n[Service]\nType=oneshot\nSlice=celiksite_7.slice\nExecStart=/bin/true\n"
	if err := os.WriteFile(filepath.Join(host.dir, "celikcron-9-1.service"), []byte(task), 0o644); err != nil {
		t.Fatal(err)
	}
	if siteSlicesNamed()[7] || siteSliceNamedByUnitFile(7) {
		t.Fatal("a task of site 9 naming site 7's slice counted")
	}
	if err := os.WriteFile(filepath.Join(host.dir, "celikcron-7-1.service"), []byte(task), 0o644); err != nil {
		t.Fatal(err)
	}
	if !siteSlicesNamed()[7] || !siteSliceNamedByUnitFile(7) {
		t.Fatal("a task naming its own site's slice does not count")
	}
	if s := round3Status(t, 7); !s.Named {
		t.Fatalf("the status: %+v", s)
	}
	// A change of limits writes the absent file again.
	var resp transport.ApplySiteSlicesResponse
	if err := (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &resp); err != nil || resp.Error != "" {
		t.Fatalf("apply: %v %+v", err, resp)
	}
	file := filepath.Join(host.dir, "celiksite_7.slice")
	raw, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(raw), "MemoryMax=1024M") {
		t.Fatalf("the slice the tasks run in was not written: %v %+v", err, resp.Sites)
	}
	// An interrupted move that wrote the file where there was none: the
	// recovery keeps it while a task names the slice.
	journal := &siteMoveJournal{Version: 2, SiteID: 7, Slice: "celiksite_7.slice", SliceFile: file, SliceExisted: false, SliceAfter: string(raw)}
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	recoverSiteSlicesAtStartup()
	if _, err := os.Stat(file); err != nil {
		t.Fatal("the recovery removed the slice a task names")
	}
}
