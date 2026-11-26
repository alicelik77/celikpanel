package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 fix round A: what the alpha.148 lab found in the per-site limits
// (D6 a put-back that did not bring the program back, D7 the agent's English
// inside the page, O3 the unapplied-limits line, O2 the Start of a stopped
// program, D5 the agent's start and the update).

// TestAPutBackThatDoesNotRunIsNotPutBack (D6, decision 1): the move's program
// does not run in the slice, and after its put-back it does not run where it
// ran either (its program exits at once). It is not "put back": the record is
// kept with the program "failed" (not running) for the card and Problems now,
// and every reason is a code with its values (D7, decision 2). Once the
// program can start again, Try again settles it.
func TestAPutBackThatDoesNotRunIsNotPutBack(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	before := sealedTestUnit(unit, sliceTestHome+"/app")
	writeTestUnit(t, host, unit, before)
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit, failEverywhere: true}

	var resp transport.MoveSiteSliceResponse
	if err := (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.RolledBack || !resp.Kept || resp.Done || len(resp.Units) != 1 {
		t.Fatalf("a program that does not run after its put-back was reported settled: %+v", resp)
	}
	got := resp.Units[0]
	if got.PutBack || got.Outcome != transport.SiteMoveFailed {
		t.Fatalf("the program is said to be put back: %+v", got)
	}
	if got.Reason == nil || got.Reason.Code != "not_running" || got.Reason.Seconds <= 0 && got.Reason.Code == "" {
		t.Fatalf("the move's reason is not a code: %+v", got.Reason)
	}
	if got.Back == nil || got.Back.Code != "not_running" {
		t.Fatalf("the put-back's reason is not a code: %+v", got.Back)
	}
	if resp.Reason == nil || resp.Reason.Code != "not_running" {
		t.Fatalf("the answer's reason: %+v", resp.Reason)
	}
	if unitText(t, filepath.Join(host.dir, unit)) != before {
		t.Fatal("the earlier file is not back")
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed although the program does not run")
	}
	status := round3Status(t, 7)
	if status.MoveInterrupted == nil || !status.MoveInterrupted.Kept || status.MoveInterrupted.Units[unit] != transport.SiteMoveFailed {
		t.Fatalf("the card is not told that the program does not run: %+v", status.MoveInterrupted)
	}
	// The program can start again: Try again settles it.
	host.mu.Lock()
	host.units[unit].failEverywhere = false
	host.mu.Unlock()
	var again transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r-a149", Recover: true}, &again)
	if !again.Done || again.Kept {
		t.Fatalf("try again: %+v", again)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed after the program ran again")
	}
}

// TestAStartPutBackIsCheckedLikeTheMove (D6, decision 1; fix round F1, F3):
// an interrupted move's put-back is checked as the move checks. At the
// Agent's start its restart is only requested: "unverified" (not known:
// neither a failure nor a success), the record kept; the status read then
// sees that it does not run where it ran and says "failed" (F3, read only).
// Try again (the full recovery) checks it within its start wait: "failed".
func TestAStartPutBackIsCheckedLikeTheMove(t *testing.T) {
	for _, c := range []struct {
		name    string
		recover func() *siteMoveRecovered
		want    string
	}{
		{"the Agent's start", recoverSiteSliceMoveAtStart, transport.SiteMoveUnverified},
		{"Try again", recoverSiteSliceMove, transport.SiteMoveFailed},
	} {
		t.Run(c.name, func(t *testing.T) {
			host, running, before := round3Interrupted(t)
			host.mu.Lock()
			host.units[running].failEverywhere = true
			host.mu.Unlock()
			recovered := c.recover()
			if recovered == nil || !recovered.record.Kept || recovered.record.Units[running] != c.want {
				t.Fatalf("want %q kept, got %+v", c.want, recovered)
			}
			if unitText(t, filepath.Join(host.dir, running)) != before {
				t.Fatal("the earlier file is not back")
			}
			if _, err := os.Stat(siteMoveJournalPath); err != nil {
				t.Fatal("the record was removed although the program was not seen running")
			}
			// The status read: not running where it ran is "failed" (read only).
			host.mu.Lock()
			host.calls = nil
			host.mu.Unlock()
			status := round3Status(t, 7)
			if status.MoveInterrupted == nil || !status.MoveInterrupted.Kept || status.MoveInterrupted.Units[running] != transport.SiteMoveFailed {
				t.Fatalf("the status read: %+v", status.MoveInterrupted)
			}
			for _, call := range host.calls {
				if strings.HasPrefix(call, "restart") || strings.HasPrefix(call, "start") || strings.HasPrefix(call, "reset-failed") {
					t.Fatalf("a status read changed a program: %v", host.calls)
				}
			}
		})
	}
}

// TestAProgramInSystemdsRestartCycleIsNotTheOwnersPlace (D6, decision 1): a
// unit systemd is starting again by itself (activating, auto-restart, no
// cgroup) does not run; it is never "where the server owner placed it".
func TestAProgramInSystemdsRestartCycleIsNotTheOwnersPlace(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	for _, props := range []map[string]string{
		{"Id": unit, "LoadState": "loaded", "ActiveState": "activating", "SubState": "auto-restart", "ControlGroup": "", "Slice": "system.slice"},
		{"Id": unit, "LoadState": "loaded", "ActiveState": "activating", "SubState": "auto-restart-queued", "ControlGroup": "", "Slice": "system.slice"},
		{"Id": unit, "LoadState": "loaded", "ActiveState": "activating", "SubState": "start", "ControlGroup": "", "Slice": "system.slice"},
	} {
		if got := siteSliceUnitStatus(7, unit, props); got.Placement != transport.SiteUnitRestarting {
			t.Fatalf("%v: %q", props, got.Placement)
		}
	}
	// A unit that runs elsewhere is still the owner's place.
	elsewhere := map[string]string{"Id": unit, "LoadState": "loaded", "ActiveState": "active", "SubState": "running", "ControlGroup": "/owner.slice/" + unit}
	if got := siteSliceUnitStatus(7, unit, elsewhere); got.Placement != transport.SiteUnitElsewhere {
		t.Fatalf("an owner's place: %q", got.Placement)
	}
}

// TestAMoveThatDoesNotListenSaysSoInCodes (D7, decision 2): the reason of a
// program that runs in the slice but does not answer on its port is a code
// with the ports and seconds, never only English.
func TestAMoveThatDoesNotListenSaysSoInCodes(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	host.ports[unit] = map[int]bool{30010: true}
	inner := siteSliceListening
	siteSliceListening = func(u string) map[int]bool {
		host.mu.Lock()
		inSlice := strings.HasPrefix(host.units[u].cgroup, "/celiksite_")
		host.mu.Unlock()
		if inSlice {
			return map[int]bool{} // it runs in the slice but no longer answers
		}
		return inner(u)
	}
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
	if !resp.RolledBack || len(resp.Units) != 1 {
		t.Fatalf("move: %+v", resp)
	}
	r := resp.Units[0].Reason
	if r == nil || r.Code != "not_listening" || len(r.Ports) != 1 || r.Ports[0] != 30010 {
		t.Fatalf("reason: %+v", r)
	}
	if !resp.Units[0].PutBack || resp.Kept {
		t.Fatalf("a program that runs and answers where it ran is put back: %+v", resp.Units[0])
	}
}

// TestARefusedMoveAnswersACode (D7, decision 2): a move that does not run at
// all answers a code too.
func TestARefusedMoveAnswersACode(t *testing.T) {
	useSliceTestHost(t)
	siteMoveInFlight.Store(true) // another move runs
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest("celikrt-5.service"), &resp)
	siteMoveInFlight.Store(false)
	if resp.Error == "" || resp.Reason == nil || resp.Reason.Code != "another_move" {
		t.Fatalf("refusal: %+v", resp)
	}
}

// slicePushFiles writes a running slice's kernel files.
func slicePushFiles(t *testing.T, name, memory string) string {
	t.Helper()
	dir := filepath.Join(siteSliceCgroupRoot, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for file, value := range map[string]string{"memory.max": memory, "memory.swap.max": "0", "pids.max": "256", "cpu.max": "150000 100000"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(value+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestAPushAnswersWhatTheKernelHolds (O3, decision 3): on every failure path
// of the push the kernel is read before the answer. The lab's case: the
// push's record cannot be written, but systemd applied the file meanwhile, so
// the slice has its limits and nothing failed. A kernel that still differs
// stays a failure; systemd that cannot be read is "not checked", never "does
// not have the limits".
func TestAPushAnswersWhatTheKernelHolds(t *testing.T) {
	host := useSliceTestHost(t)
	name := sitesandbox.SliceName(7)
	host.slices[name] = "MemoryMax=1073741824\nMemorySwapMax=0\nTasksMax=256\nCPUQuotaPerSecUSec=1.5s\n"
	dir := slicePushFiles(t, name, "536870912")
	old := siteSliceRuntimeJournalWrite
	t.Cleanup(func() { siteSliceRuntimeJournalWrite = old })
	siteSliceRuntimeJournalWrite = func(string, []byte) error {
		// systemd applies the written file meanwhile (lab: the write and the
		// reload had); the push's own record cannot be written.
		_ = os.WriteFile(filepath.Join(dir, "memory.max"), []byte("1073741824\n"), 0o644)
		return errors.New("open site-slice-runtime.json.tmp: read-only file system")
	}
	if failed := pushSiteSlicesToKernel([]int{7}); len(failed) != 0 {
		t.Fatalf("a slice that holds its limits is answered as failed: %v", failed)
	}
	slicePushFiles(t, name, "536870912")
	siteSliceRuntimeJournalWrite = func(string, []byte) error { return errors.New("read-only file system") }
	failed := pushSiteSlicesToKernel([]int{7})
	if failed[7] == "" || strings.HasPrefix(failed[7], siteSliceUncheckedPrefix) {
		t.Fatalf("a kernel that still differs: %v", failed)
	}
	failing(func(args []string) bool {
		return args[0] == "show" && strings.HasPrefix(args[1], "--property=Id,MemoryMax")
	})
	failed = pushSiteSlicesToKernel([]int{7})
	if !strings.HasPrefix(failed[7], siteSliceUncheckedPrefix) {
		t.Fatalf("systemd that cannot be read: %v", failed)
	}
}

// TestAppliedLimitsAnswerReasonCodes (O3, decisions 2 and 3): the apply's
// failed outcome carries the code the page words: kernel_differs when the
// kernel was read and differs, unchecked when it could not be read.
func TestAppliedLimitsAnswerReasonCodes(t *testing.T) {
	host := useSliceTestHost(t)
	name := sitesandbox.SliceName(7)
	host.slices[name] = "MemoryMax=1073741824\nMemorySwapMax=0\nTasksMax=256\nCPUQuotaPerSecUSec=1.5s\n"
	slicePushFiles(t, name, "536870912")
	failing(func(args []string) bool { return args[0] == "set-property" })
	apply := func() transport.SiteSliceApplied {
		t.Helper()
		var resp transport.ApplySiteSlicesResponse
		if err := (&Agent{}).ApplySiteSlicesV1(&transport.ApplySiteSlicesRequest{Sites: []transport.SiteSlice{*testSlice()}}, &resp); err != nil || resp.Error != "" || len(resp.Sites) != 1 {
			t.Fatalf("apply: %v %+v", err, resp)
		}
		return resp.Sites[0]
	}
	if got := apply(); got.Outcome != transport.SiteFileOutcomeFailed || got.Reason != "kernel_differs" {
		t.Fatalf("a kernel that differs: %+v", got)
	}
	failing(func(args []string) bool {
		return args[0] == "show" && strings.HasPrefix(args[1], "--property=Id,MemoryMax")
	})
	if got := apply(); got.Outcome != transport.SiteFileOutcomeFailed || got.Reason != "unchecked" {
		t.Fatalf("a kernel that could not be compared: %+v", got)
	}
}

// TestAnUpdateDoesNotStartDuringAMove (D5, decision 8b): the agent refuses
// its own update while a site's programs are being moved into their limits.
func TestAnUpdateDoesNotStartDuringAMove(t *testing.T) {
	siteMoveInFlight.Store(true)
	var reply transport.SystemUpdateStartResponse
	err := (&Agent{}).StartSystemUpdate(&transport.SystemUpdateStartRequest{RequestID: "a149-move"}, &reply)
	siteMoveInFlight.Store(false)
	if err != nil || reply.Accepted || reply.Reason != transport.HostMutationReasonSiteMove || reply.Error == "" {
		t.Fatalf("an update during a move: %v %+v", err, reply)
	}
}

// TestTheAgentsStartLogsEachStep (D5, decision 8a): one journal line per
// startup step with its duration and the time since the start began, and the
// daemon's start names each step before it listens.
func TestTheAgentsStartLogsEachStep(t *testing.T) {
	var lines []string
	at := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	clock := &agentStartClock{began: at, last: at, now: func() time.Time { return at }, logf: func(format string, args ...any) {
		lines = append(lines, args[0].(string))
	}}
	at = at.Add(250 * time.Millisecond)
	clock.step("instance lock")
	at = at.Add(time.Second)
	clock.step("site limits move recovery")
	want := []string{
		"agent start: instance lock took 250ms (250ms since the start began)",
		"agent start: site limits move recovery took 1s (1.25s since the start began)",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines:\n%s", strings.Join(lines, "\n"))
	}
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	last := -1
	for _, step := range []string{"instance lock", "mutation ledger", "system update state", "site limits move recovery",
		"webmail addresses and local server", "nginx generator", "PHP-FPM manager", "SQLite manager", "RPC service and socket"} {
		i := strings.Index(src, `startClock.step("`+step+`")`)
		if i < 0 || i < last {
			t.Fatalf("the start does not time %q in order", step)
		}
		last = i
	}
	if strings.Index(src, `startClock.step("RPC service and socket")`) > strings.Index(src, `log.Printf("Agent listening on unix socket %s", socketPath)`) {
		t.Fatal("the last step is not timed before the agent listens")
	}
}
