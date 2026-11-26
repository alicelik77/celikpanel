package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 fix round of PR #371 (the independent review's F1, F3, F4, F5, F6,
// F10, W1, W2): the Agent's side.

// keptRecordOf writes a kept move record of site 7 whose units are at step
// "restarting" with the move's text on disk and running in the slice (the
// Agent stopped after their restarts), as an interrupted move leaves it.
func keptRecordOf(t *testing.T, host *sliceTestHost, units []string) {
	t.Helper()
	journal := &siteMoveJournal{Version: 2, SiteID: 7, Slice: "celiksite_7.slice", RequestID: "r-fix", StartedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, unit := range units {
		before := sealedTestUnit(unit, sliceTestHome+"/app")
		body, _ := siteUnitSealedBody(unit, before)
		moved, _, err := siteUnitMovedBody(body, 7)
		if err != nil {
			t.Fatal(err)
		}
		after := siteUnitSeal(unit, moved)
		writeTestUnit(t, host, unit, after)
		host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/celiksite_7.slice/" + unit}
		journal.Units = append(journal.Units, siteMoveJournalUnit{Unit: unit, File: filepath.Join(host.dir, unit), Mode: 0o644,
			Before: before, After: after, Running: true, Step: siteMoveStepRestarting, WaitSeconds: 90})
	}
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
}

// TestTheAgentsStartIsBoundedByItsReadsAndOneReload (F1): 64 programs in a
// kept record and every systemd call slow. The start's added time is its
// calls and nothing else: ONE reading of the programs, ONE daemon-reload, one
// reset-failed and a restart requested with --no-block per program; no
// restart that waits, no polling, no port reading. Every outcome is
// "unverified" and the record stays.
func TestTheAgentsStartIsBoundedByItsReadsAndOneReload(t *testing.T) {
	host := useSliceTestHost(t)
	var units []string
	for i := 0; i < siteSliceMaxUnits; i++ {
		units = append(units, fmt.Sprintf("celikrt-%d.service", 100+i))
	}
	keptRecordOf(t, host, units)
	listened := 0
	siteSliceListening = func(string) map[int]bool { listened++; return nil }
	const delay = 15 * time.Millisecond
	host.mu.Lock()
	host.delay, host.calls, host.reloads = delay, nil, 0
	host.mu.Unlock()

	started := time.Now()
	recoverSiteSlicesAtStartup()
	took := time.Since(started)

	host.mu.Lock()
	calls, reloads := append([]string(nil), host.calls...), host.reloads
	host.mu.Unlock()
	shows, resets, requested := 0, 0, 0
	for _, c := range calls {
		switch {
		case strings.HasPrefix(c, "show"):
			shows++
		case strings.HasPrefix(c, "reset-failed"):
			resets++
		case strings.HasPrefix(c, "restart --no-block "):
			requested++
		case strings.HasPrefix(c, "restart"):
			t.Fatalf("a restart that waits at the Agent's start: %s", c)
		}
	}
	if shows != 1 || reloads != 1 || resets != 1 || requested != len(units) || listened != 0 {
		t.Fatalf("shows=%d reloads=%d resets=%d requested=%d ports read=%d", shows, reloads, resets, requested, listened)
	}
	if bound := time.Duration(len(calls))*delay + 750*time.Millisecond; took > bound {
		t.Fatalf("the start took %v for %d calls of %v (bound %v)", took, len(calls), delay, bound)
	}
	outcomes := readSiteMoveOutcomes()["7"]
	if !outcomes.Kept {
		t.Fatal("the record is not kept")
	}
	for _, u := range units {
		if outcomes.Units[u] != transport.SiteMoveUnverified {
			t.Fatalf("%s: %q", u, outcomes.Units[u])
		}
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed at the start")
	}
}

// TestAnAlreadyPutBackProgramIsLeftAsItIsAtTheStart (F1): a program whose
// earlier file is already back is never started or restarted at the Agent's
// start: one the put-back left not running keeps its outcome, one its owner
// stopped after a verified put-back stays stopped, and nothing is read or
// reloaded for them.
func TestAnAlreadyPutBackProgramIsLeftAsItIsAtTheStart(t *testing.T) {
	host := useSliceTestHost(t)
	failed, ownerStopped := "celikrt-5.service", "celikrt-8.service"
	keptRecordOf(t, host, []string{failed, ownerStopped})
	journal, _ := readSiteMoveJournal()
	for i := range journal.Units {
		u := &journal.Units[i]
		writeTestUnit(t, host, u.Unit, u.Before)
		host.units[u.Unit] = &sliceTestUnit{active: "failed"}
		if u.Unit == ownerStopped {
			u.Step = siteMoveStepPutBack
			host.units[u.Unit].active = "inactive"
		}
	}
	_ = journal.save()
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		all["7"] = transport.SiteSliceMoveRecovery{At: journal.StartedAt, Kept: true,
			Units: map[string]string{failed: transport.SiteMoveFailed, ownerStopped: transport.SiteMovePutBack}}
		return true
	})
	host.mu.Lock()
	host.calls = nil
	host.mu.Unlock()
	recoverSiteSlicesAtStartup()
	host.mu.Lock()
	calls := append([]string(nil), host.calls...)
	host.mu.Unlock()
	if len(calls) != 0 {
		t.Fatalf("the start called systemd for programs already put back: %v", calls)
	}
	outcomes := readSiteMoveOutcomes()["7"]
	if !outcomes.Kept || outcomes.Units[failed] != transport.SiteMoveFailed || outcomes.Units[ownerStopped] != transport.SiteMovePutBack {
		t.Fatalf("outcomes: %+v", outcomes)
	}
	if host.units[ownerStopped].active != "inactive" {
		t.Fatal("a program its owner stopped was started")
	}
}

// TestNotKnownYetIsResolvedByReadingOnly (F3): a kept "unverified" program is
// checked by the status read: still starting stays "unverified"; running
// where it ran closes its entry and, with nothing else left, the record.
// No call of the read starts or restarts anything.
func TestNotKnownYetIsResolvedByReadingOnly(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	keptRecordOf(t, host, []string{unit})
	recoverSiteSliceMoveAtStart()
	host.mu.Lock()
	host.units[unit].active, host.units[unit].cgroup = "activating", ""
	host.calls = nil
	host.mu.Unlock()
	if s := round3Status(t, 7); s.MoveInterrupted == nil || !s.MoveInterrupted.Kept || s.MoveInterrupted.Units[unit] != transport.SiteMoveUnverified {
		t.Fatalf("still starting: %+v", s.MoveInterrupted)
	}
	host.mu.Lock()
	host.units[unit].active, host.units[unit].cgroup = "active", "/system.slice/"+unit
	host.mu.Unlock()
	if s := round3Status(t, 7); s.MoveInterrupted == nil || s.MoveInterrupted.Kept || s.MoveInterrupted.Units[unit] != transport.SiteMovePutBack {
		t.Fatalf("running where it ran: %+v", s.MoveInterrupted)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	for _, c := range host.calls {
		if !strings.HasPrefix(c, "show") && c != "daemon-reload" {
			t.Fatalf("a status read did more than read (and put the slice file back): %v", host.calls)
		}
	}
}

// TestAKeptRecordBoundsTryAgainAndRefusesAnotherSite (F4): a move of another
// site is refused at once with the kept record's reason, before anything of
// the first site is restarted; Try again restarts only what fits the move's
// own budget and leaves the rest "waiting".
func TestAKeptRecordBoundsTryAgainAndRefusesAnotherSite(t *testing.T) {
	host := useSliceTestHost(t)
	var units []string
	for i := 0; i < 6; i++ {
		units = append(units, fmt.Sprintf("celikrt-%d.service", 20+i))
	}
	keptRecordOf(t, host, units)
	for _, u := range units {
		host.units[u].failEverywhere = true
	}
	host.mu.Lock()
	host.calls = nil
	host.mu.Unlock()
	other := moveRequest("celikrt-90.service")
	other.Site.SiteID = 9
	var refused transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(other, &refused)
	if !refused.Kept || refused.Reason == nil || refused.Reason.Code != "kept_earlier" || !strings.Contains(refused.Error, "site 7") {
		t.Fatalf("another site's move: %+v", refused)
	}
	host.mu.Lock()
	for _, c := range host.calls {
		if strings.HasPrefix(c, "restart") || c == "daemon-reload" {
			host.mu.Unlock()
			t.Fatalf("the first site was touched by another site's move: %v", host.calls)
		}
	}
	host.mu.Unlock()
	// Try again within a budget that holds about two of the start waits.
	oldMargin := siteMoveMargin
	t.Cleanup(func() { siteMoveMargin = oldMargin })
	siteMoveBudget, siteMoveMargin, siteSliceSettleDefault = 160*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond
	journal, _ := readSiteMoveJournal()
	for i := range journal.Units {
		journal.Units[i].WaitSeconds = 0 // the default start wait (50 ms here)
	}
	_ = journal.save()
	started := time.Now()
	var again transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r-fix-again", Recover: true}, &again)
	if took := time.Since(started); took > siteMoveBudget+400*time.Millisecond {
		t.Fatalf("Try again took %v (budget %v)", took, siteMoveBudget)
	}
	waiting := 0
	for _, u := range units {
		if again.Recovery == nil {
			t.Fatalf("try again: %+v", again)
		}
		if again.Recovery.Units[u] == transport.SiteMoveWaiting {
			waiting++
		}
	}
	if !again.Kept || waiting == 0 {
		t.Fatalf("Try again was not bounded: %+v", again.Recovery)
	}
}

// TestStatusReadsNeverRefuseTheUpdateOrAMove (F5): a status read holds the
// record's lock for a moment; the update is not refused for it, and a move
// waits for it instead of answering "another move".
func TestStatusReadsNeverRefuseTheUpdateOrAMove(t *testing.T) {
	host := useSliceTestHost(t)
	// An update service that refuses at once (no host is asked).
	previousService, previousErr := globalSystemUpdateService, globalSystemUpdateErr
	globalSystemUpdateService, globalSystemUpdateErr = newSystemUpdateService(nil, nil, "linux", "amd64"), nil
	t.Cleanup(func() { globalSystemUpdateService, globalSystemUpdateErr = previousService, previousErr })
	siteMoveRunning.Lock() // a status read in progress
	var reply transport.SystemUpdateStartResponse
	_ = (&Agent{}).StartSystemUpdate(&transport.SystemUpdateStartRequest{RequestID: "a149-fix-read"}, &reply)
	if reply.Reason == transport.HostMutationReasonSiteMove {
		siteMoveRunning.Unlock()
		t.Fatalf("a status read made the update refuse: %+v", reply)
	}
	unit := "celikrt-5.service"
	writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	done := make(chan transport.MoveSiteSliceResponse, 1)
	go func() {
		var resp transport.MoveSiteSliceResponse
		_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
		done <- resp
	}()
	time.Sleep(50 * time.Millisecond)
	siteMoveRunning.Unlock() // the status read ends
	resp := <-done
	if !resp.Done || (resp.Reason != nil && resp.Reason.Code == "another_move") {
		t.Fatalf("a status read made the move refuse: %+v", resp)
	}
}

// TestAMoveIsRefusedWhileTheUpdateRuns (F6): while the panel's own update is
// queued or running, a move and a Try again are refused with their code and
// nothing is written; the update's records are read only.
func TestAMoveIsRefusedWhileTheUpdateRuns(t *testing.T) {
	host := useSliceTestHost(t)
	unit := "celikrt-5.service"
	before := sealedTestUnit(unit, sliceTestHome+"/app")
	writeTestUnit(t, host, unit, before)
	host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
	siteMoveUpdateActive = func() bool { return true }
	for _, req := range []*transport.MoveSiteSliceRequest{moveRequest(unit), {RequestID: "r-fix-upd", Recover: true}} {
		var resp transport.MoveSiteSliceResponse
		_ = (&Agent{}).MoveSiteSliceV1(req, &resp)
		if resp.Done || resp.Reason == nil || resp.Reason.Code != "update_running" || !strings.Contains(resp.Error, "update is in progress") {
			t.Fatalf("during the update: %+v", resp)
		}
	}
	if unitText(t, filepath.Join(host.dir, unit)) != before || host.called("daemon-reload") {
		t.Fatal("something was written during the update")
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("a move record was written during the update")
	}
	// The reader of the update's records.
	root := t.TempDir()
	if systemUpdateRecordsActive(root) || systemUpdateRecordsActive(filepath.Join(root, "missing")) {
		t.Fatal("no record is an update in progress")
	}
	write := func(id, status string) {
		raw := fmt.Sprintf(`{"version":%d,"request_id":%q,"status":%q}`, systemUpdateStateVersion, id, status)
		if err := os.WriteFile(filepath.Join(root, id+".json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(strings.Repeat("a", 32), systemUpdateSucceeded)
	if systemUpdateRecordsActive(root) {
		t.Fatal("a finished update is not in progress")
	}
	write(strings.Repeat("b", 32), systemUpdateRunning)
	if !systemUpdateRecordsActive(root) {
		t.Fatal("a running update is not seen")
	}
}

// TestAPutBackThatRunsOnAChangedPortRuns (F10): a put-back program active
// where it ran and listening on a port is running, whatever the port: no
// record. One that had ports and listens on none is "not answering"; Try
// again uses the same check (a changed port then settles it).
func TestAPutBackThatRunsOnAChangedPortRuns(t *testing.T) {
	for _, c := range []struct {
		name  string
		ports map[int]bool
		kept  bool
	}{
		{"a changed port", map[int]bool{30099: true}, false},
		{"no port at all", map[int]bool{}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			host := useSliceTestHost(t)
			unit := "celikrt-5.service"
			writeTestUnit(t, host, unit, sealedTestUnit(unit, sliceTestHome+"/app"))
			host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit, failStart: true}
			host.ports[unit] = map[int]bool{30010: true}
			listening, restarted := c.ports, false
			host.onRestart = func(string) { restarted = true }
			siteSliceListening = func(u string) map[int]bool {
				if !restarted {
					return map[int]bool{30010: true} // before the move
				}
				return listening
			}
			var resp transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
			if !resp.RolledBack || resp.Kept != c.kept {
				t.Fatalf("move: %+v", resp)
			}
			if !c.kept {
				if !resp.Units[0].PutBack {
					t.Fatalf("a program running on a changed port is not put back: %+v", resp.Units[0])
				}
				return
			}
			if got := resp.Units[0].Back; got == nil || got.Code != siteRunVerdictNoPort || len(got.Ports) != 1 {
				t.Fatalf("the put-back's reason: %+v", got)
			}
			if outcomes := readSiteMoveOutcomes()["7"]; outcomes.Units[unit] != transport.SiteMoveNotAnswering {
				t.Fatalf("outcome: %+v", outcomes)
			}
			// It answers on another port now: Try again settles it without a restart.
			listening = map[int]bool{30111: true}
			host.mu.Lock()
			host.calls = nil
			host.mu.Unlock()
			var again transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r-fix-port", Recover: true}, &again)
			if !again.Done || host.called("restart "+unit) {
				t.Fatalf("try again: %+v %v", again, host.calls)
			}
		})
	}
}

// TestPutBackWordsSayWhatIsTrue (W1, W2): the move's answer says which
// put-back programs ran before (put back and running again) and which did not
// (put back as they were); a put-back whose place could not be told after its
// restart is "unverified" (it was restarted), never "unknown, not restarted".
func TestPutBackWordsSayWhatIsTrue(t *testing.T) {
	host := useSliceTestHost(t)
	stopped, running := "celiksvc-6.service", "celikrt-5.service"
	writeTestUnit(t, host, stopped, sealedTestUnit(stopped, sliceTestHome+"/apps/n8n"))
	writeTestUnit(t, host, running, sealedTestUnit(running, sliceTestHome+"/app"))
	host.units[stopped] = &sliceTestUnit{active: "inactive"}
	host.units[running] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + running, failStart: true}
	var resp transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(stopped, running), &resp)
	ran := map[string]bool{}
	for _, u := range resp.Units {
		if !u.PutBack {
			t.Fatalf("not put back: %+v", resp.Units)
		}
		ran[u.Unit] = u.Ran
	}
	if ran[stopped] || !ran[running] {
		t.Fatalf("ran: %v", ran)
	}
	state, _, _ := siteUnitPutBackFailure(running, siteRunVerdictNotInPlace, map[string]string{"ActiveState": "active", "ControlGroup": "/x.slice/" + running}, time.Second, nil)
	if state != transport.SiteMoveUnverified {
		t.Fatalf("a restarted program whose place is not told: %q", state)
	}
	if state, _, _ := siteUnitPutBackFailure(running, siteRunVerdictUnread, map[string]string{}, time.Second, nil); state != transport.SiteMoveUnverified {
		t.Fatalf("a restarted program that could not be read: %q", state)
	}
}
