package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 second fix round of PR #371 (the second review's R1-R8): the
// Agent's side.

// keptUnitsAfterStart leaves a kept record of site 7 whose programs were put
// back and restarted at the Agent's start ("unverified"), running where they
// ran from the host's clock now.
func keptUnitsAfterStart(t *testing.T, host *sliceTestHost, units []string, ports map[string][]int) {
	t.Helper()
	keptRecordOf(t, host, units)
	if len(ports) > 0 {
		journal, _ := readSiteMoveJournal()
		for i := range journal.Units {
			journal.Units[i].Ports = ports[journal.Units[i].Unit]
		}
		if err := journal.save(); err != nil {
			t.Fatal(err)
		}
	}
	recoverSiteSliceMoveAtStart()
	for _, u := range units {
		if got := readSiteMoveOutcomes()["7"].Units[u]; got != transport.SiteMoveUnverified {
			t.Fatalf("%s after the start: %q", u, got)
		}
	}
}

func keptState(t *testing.T, unit string) (string, bool) {
	t.Helper()
	s := round3Status(t, 7)
	if s.MoveInterrupted == nil {
		t.Fatal("no outcome")
	}
	return s.MoveInterrupted.Units[unit], s.MoveInterrupted.Kept
}

// TestAReadClosesOnlyAProgramThatReallyRuns (R1, R3, R9): one reading of
// "active" right after a restart, as a program in systemd's restart cycle
// gives, never closes an entry; the restart cycle makes it "not running"; a
// program that then runs in its place without a break for its start wait is
// closed by the next read (also from "not running"). A program that had
// ports is closed only once it listens on one; a listener table that cannot
// be read leaves it as it is.
func TestAReadClosesOnlyAProgramThatReallyRuns(t *testing.T) {
	host := useSliceTestHost(t)
	cycling, ported := "celikrt-5.service", "celikrt-6.service"
	keptUnitsAfterStart(t, host, []string{cycling, ported}, map[string][]int{ported: {30010}})
	wait := 90 * time.Second // the record's start wait

	// Right after the restart: active, running, in its place, for 0 s.
	if state, kept := keptState(t, cycling); state != transport.SiteMoveUnverified || !kept {
		t.Fatalf("one lucky reading closed it: %q kept=%v", state, kept)
	}
	// The program exits and systemd starts it again by itself.
	host.mu.Lock()
	host.units[cycling].active, host.units[cycling].sub, host.units[cycling].cgroup = "activating", "auto-restart", ""
	host.mu.Unlock()
	if state, _ := keptState(t, cycling); state != transport.SiteMoveFailed {
		t.Fatalf("the restart cycle: %q", state)
	}
	// Up again, for less than its start wait: still "not running".
	host.mu.Lock()
	host.units[cycling].active, host.units[cycling].sub, host.units[cycling].cgroup = "active", "", "/system.slice/"+cycling
	host.units[cycling].enter = host.now
	host.mu.Unlock()
	host.advance(wait / 2)
	if state, _ := keptState(t, cycling); state != transport.SiteMoveFailed {
		t.Fatalf("up for half its wait: %q", state)
	}
	// Up for its whole wait: closed. The program with ports listens on none.
	host.advance(wait)
	if state, kept := keptState(t, cycling); state != transport.SiteMovePutBack || !kept {
		t.Fatalf("up for its wait: %q kept=%v", state, kept)
	}
	if state, _ := keptState(t, ported); state != transport.SiteMoveUnverified {
		t.Fatalf("a program that had ports and listens on none: %q", state)
	}
	// Its listener table cannot be read: left as it is.
	inner := siteSliceListening
	siteSliceListening = func(string) (map[int]bool, bool) { return nil, false }
	if state, _ := keptState(t, ported); state != transport.SiteMoveUnverified {
		t.Fatalf("an unreadable listener table: %q", state)
	}
	siteSliceListening = inner
	// It answers on a port (another one than before): closed, and the record
	// with it.
	host.mu.Lock()
	host.ports[ported] = map[int]bool{30011: true}
	host.mu.Unlock()
	if state, kept := keptState(t, ported); state != transport.SiteMovePutBack || kept {
		t.Fatalf("listening again: %q kept=%v", state, kept)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed")
	}
}

// TestStartingItOnItsPageSettlesTheRecord (R3): a program a move left "not
// running" that the administrator starts on its page is closed by the next
// status read once it has run for its start wait; the record goes and moves
// are no longer refused.
func TestStartingItOnItsPageSettlesTheRecord(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit, failEverywhere: true}
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
	if !resp.Kept || readSiteMoveOutcomes()["7"].Units[unit] != transport.SiteMoveFailed {
		t.Fatalf("move: %+v", resp)
	}
	// Started on its page: it runs where it ran.
	host.mu.Lock()
	host.units[unit].failEverywhere = false
	host.units[unit].active, host.units[unit].cgroup, host.units[unit].enter = "active", "/system.slice/"+unit, host.now
	host.mu.Unlock()
	if state, kept := keptState(t, unit); state != transport.SiteMoveFailed || !kept {
		t.Fatalf("just started: %q kept=%v", state, kept)
	}
	host.advance(time.Second) // more than its start wait (50 ms here)
	if state, kept := keptState(t, unit); state != transport.SiteMovePutBack || kept {
		t.Fatalf("running for its wait: %q kept=%v", state, kept)
	}
	other := "celikrt-9.service"
	writeTestUnit(t, host, other, sealedTestUnit(other, sliceTestHome+"/app"))
	host.units[other] = &sliceTestUnit{active: "inactive"}
	var next transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(other), &next)
	if !next.Done {
		t.Fatalf("a move after the record was settled: %+v", next)
	}
}

// TestAReadClosesARecordOnlyUnderTheSliceLock (R6): a read that closes the
// last entry puts the slice file back only while it can take the slice lock
// at once; while a limits change holds it, the closing is left for the next
// read, and nothing of the file is touched.
func TestAReadClosesARecordOnlyUnderTheSliceLock(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	keptRecordOf(t, host, []string{unit})
	slice := filepath.Join(host.dir, "celiksite_7.slice")
	moved := "# the move's slice\n[Slice]\nMemoryMax=1024M\n"
	if err := os.WriteFile(slice, []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	journal, _ := readSiteMoveJournal()
	journal.SliceFile, journal.SliceExisted, journal.SliceAfter = slice, false, moved
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	recoverSiteSliceMoveAtStart()
	host.advance(time.Hour)
	siteSliceMu.Lock() // a limits change in progress
	state, kept := keptState(t, unit)
	_, journalErr := os.Stat(siteMoveJournalPath)
	_, sliceErr := os.Stat(slice)
	siteSliceMu.Unlock()
	if state != transport.SiteMovePutBack || !kept || journalErr != nil || sliceErr != nil {
		t.Fatalf("closed without the slice lock: %q kept=%v journal=%v slice=%v", state, kept, journalErr, sliceErr)
	}
	if state, kept := keptState(t, unit); state != transport.SiteMovePutBack || kept {
		t.Fatalf("the next read: %q kept=%v", state, kept)
	}
	if _, err := os.Stat(slice); !os.IsNotExist(err) {
		t.Fatal("the slice file the move made was not put back")
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed")
	}
}

// TestTheStartsSystemdCallsShareOneBound (R5): a systemd that never answers
// costs the Agent's start no more than the one bound, whatever the number of
// programs; once it runs out nothing further is sent, and the record stays
// with every outcome "unknown".
func TestTheStartsSystemdCallsShareOneBound(t *testing.T) {
	host := useSliceTestHost(t)
	var units []string
	for i := 0; i < siteSliceMaxUnits; i++ {
		units = append(units, fmt.Sprintf("celikrt-%d.service", 200+i))
	}
	keptRecordOf(t, host, units)
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	var mu sync.Mutex
	var sent []string
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		mu.Lock()
		sent = append(sent, strings.Join(args, " "))
		mu.Unlock()
		<-never
		return nil, nil
	}
	siteMoveStartBound = 200 * time.Millisecond
	started := time.Now()
	recoverSiteSlicesAtStartup()
	if took := time.Since(started); took > siteMoveStartBound+400*time.Millisecond {
		t.Fatalf("the start took %v with a systemd that never answers (bound %v)", took, siteMoveStartBound)
	}
	mu.Lock()
	calls := append([]string(nil), sent...)
	mu.Unlock()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "show") {
		t.Fatalf("calls sent after the bound ran out: %v", calls)
	}
	outcomes := readSiteMoveOutcomes()["7"]
	if !outcomes.Kept {
		t.Fatal("the record is not kept")
	}
	for _, u := range units {
		if outcomes.Units[u] != transport.SiteMoveUnknown {
			t.Fatalf("%s: %q", u, outcomes.Units[u])
		}
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed")
	}
}

// TestOnePressHasOneDeadline (R4): a move that first settles its own site's
// kept record and then moves shares ONE deadline with it: what no longer
// fits is not started ("the move's time ran out"), and the whole press stays
// within the move's budget.
func TestOnePressHasOneDeadline(t *testing.T) {
	host := useSliceTestHost(t)
	kept := []string{"celikrt-20.service", "celikrt-21.service"}
	keptRecordOf(t, host, kept)
	journal, _ := readSiteMoveJournal()
	for i := range journal.Units {
		journal.Units[i].WaitSeconds = 0 // the default start wait (400 ms here)
	}
	_ = journal.save()
	unit := "celikrt-50.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	host.onRestart = func(string) { time.Sleep(150 * time.Millisecond) } // each restart that waits takes 150 ms
	oldMargin := siteMoveMargin
	t.Cleanup(func() { siteMoveMargin = oldMargin })
	// Every start wait 400 ms: a put-back restart needs 410 ms left, the
	// move's own program 810 ms (its restart and its put-back). The two
	// put-backs leave about 600 ms of the press's 900 ms: the move's program
	// no longer fits. With a fresh budget after them it would.
	siteSliceSettleMin, siteSliceSettleDefault, siteSliceSettleMax = 400*time.Millisecond, 400*time.Millisecond, 400*time.Millisecond
	siteMoveBudget, siteMoveMargin = 900*time.Millisecond, 10*time.Millisecond

	started := time.Now()
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
	took := time.Since(started)
	if resp.Done || !resp.TimeUp || resp.Kept {
		t.Fatalf("the move went on with a fresh budget after settling the earlier record: %+v", resp)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the earlier record was not settled")
	}
	if took > siteMoveBudget+250*time.Millisecond {
		t.Fatalf("the press took %v (budget %v)", took, siteMoveBudget)
	}
	if host.called("restart " + unit) {
		t.Fatal("a program was restarted past the press's deadline")
	}
}

// TestTheUpdateIsNeverHeldBehindAMovesRecordLock (R7): a move or Try again
// that waits for the record's lock (a status read holds it) does not hold the
// update's gate: the update's admission answers at once, refused because a
// move is in flight; no update can be admitted between the two steps.
func TestTheUpdateIsNeverHeldBehindAMovesRecordLock(t *testing.T) {
	useSliceTestHost(t)
	previousService, previousErr := globalSystemUpdateService, globalSystemUpdateErr
	globalSystemUpdateService, globalSystemUpdateErr = newSystemUpdateService(nil, nil, "linux", "amd64"), nil
	t.Cleanup(func() { globalSystemUpdateService, globalSystemUpdateErr = previousService, previousErr })
	between := make(chan struct{})
	siteMoveClaimed = func() { close(between) }
	siteMoveRunning.Lock() // a status read in progress
	done := make(chan transport.MoveSiteSliceResponse, 1)
	go func() {
		var resp transport.MoveSiteSliceResponse
		_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r-fix2-gate", Recover: true}, &resp)
		done <- resp
	}()
	<-between
	answered := make(chan transport.SystemUpdateStartResponse, 1)
	go func() {
		var reply transport.SystemUpdateStartResponse
		_ = (&Agent{}).StartSystemUpdate(&transport.SystemUpdateStartRequest{RequestID: "a149-fix2-gate"}, &reply)
		answered <- reply
	}()
	select {
	case reply := <-answered:
		if reply.Reason != transport.HostMutationReasonSiteMove {
			siteMoveRunning.Unlock()
			t.Fatalf("an update was admitted while a move was claimed: %+v", reply)
		}
	case <-time.After(2 * time.Second):
		siteMoveRunning.Unlock()
		t.Fatal("the update's admission waited behind the move's record lock")
	}
	siteMoveRunning.Unlock()
	if resp := <-done; !resp.Done {
		t.Fatalf("try again: %+v", resp)
	}
}

// notAnsweringRecord leaves a move's put-back "not answering": the program
// listened on 30010 before the move and listens on no port after it.
func notAnsweringRecord(t *testing.T, host *sliceTestHost, unit string) {
	t.Helper()
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit, failStart: true}
	host.ports[unit] = map[int]bool{30010: true}
	host.onRestart = func(string) {
		host.mu.Lock()
		delete(host.ports, unit)
		host.mu.Unlock()
	}
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
	if !resp.Kept || readSiteMoveOutcomes()["7"].Units[unit] != transport.SiteMoveNotAnswering {
		t.Fatalf("move: %+v %+v", resp, readSiteMoveOutcomes()["7"])
	}
	host.onRestart = nil
}

// TestTryAgainSettlesAProgramThatRunsWithoutAPort (R8): the administrator's
// Try again restarts a "not answering" program once more; if it then runs
// where it ran without a break for its start wait, it is put back although it
// listens on no port, and the answer names it. One that keeps restarting
// within the wait stays "not answering"; a listener table that cannot be read
// is "not known yet", never "listens on no port".
func TestTryAgainSettlesAProgramThatRunsWithoutAPort(t *testing.T) {
	unit := "celikrt-5.service"
	tryAgain := func(id string) transport.MoveSiteSliceResponse {
		var resp transport.MoveSiteSliceResponse
		_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: id, Recover: true}, &resp)
		return resp
	}
	t.Run("runs without a port", func(t *testing.T) {
		host := useSliceTestHost(t)
		notAnsweringRecord(t, host, unit)
		host.mu.Lock()
		host.calls = nil
		host.mu.Unlock()
		resp := tryAgain("r-fix2-noport")
		if !resp.Done || len(resp.RunsNoPort) != 1 || resp.RunsNoPort[0] != unit || !host.called("restart "+unit) {
			t.Fatalf("try again: %+v %v", resp, host.calls)
		}
		if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
			t.Fatal("the record stayed")
		}
	})
	t.Run("keeps restarting", func(t *testing.T) {
		host := useSliceTestHost(t)
		notAnsweringRecord(t, host, unit)
		inner := appUnitSystemctl
		appUnitSystemctl = func(args ...string) ([]byte, error) {
			if args[0] == "show" && len(args) == 3 && args[1] == unit {
				host.mu.Lock()
				host.units[unit].enter++ // it went down and came up again between two readings
				host.mu.Unlock()
			}
			return inner(args...)
		}
		resp := tryAgain("r-fix2-cycle")
		if resp.Done || !resp.Kept || len(resp.RunsNoPort) != 0 || resp.Recovery.Units[unit] != transport.SiteMoveNotAnswering {
			t.Fatalf("try again: %+v", resp)
		}
	})
	t.Run("listeners unread", func(t *testing.T) {
		host := useSliceTestHost(t)
		notAnsweringRecord(t, host, unit)
		siteSliceListening = func(string) (map[int]bool, bool) { return nil, false }
		host.mu.Lock()
		host.calls = nil
		host.mu.Unlock()
		resp := tryAgain("r-fix2-unread")
		if resp.Done || !resp.Kept || len(resp.RunsNoPort) != 0 || resp.Recovery.Units[unit] != transport.SiteMoveUnverified {
			t.Fatalf("try again: %+v", resp)
		}
		if host.called("restart " + unit) {
			t.Fatal("a program whose listeners could not be read was restarted as not answering")
		}
	})
}
