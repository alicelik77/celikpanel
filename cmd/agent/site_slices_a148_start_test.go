package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.148 fix round: the Agent's start with a kept move record.

// TestAKeptMoveNeverHoldsUpTheAgentsStart (review finding 5): an interrupted
// move whose put-back restarts take long is recovered at the Agent's start
// within siteMoveStartBudget: the first restart waits only what is left of
// it ("starting"), the second is not sent ("waiting"), the record stays, so
// the site's card and the Problems now line (site_move_kept) name both; the
// administrator's Try again then finishes it without that bound.
func TestAKeptMoveNeverHoldsUpTheAgentsStart(t *testing.T) {
	host := useSliceTestHost(t)
	first, second := "celikrt-5.service", "celiksvc-6.service"
	writeTestUnit(t, host, first, sealedTestUnit(first, sliceTestHome+"/app"))
	writeTestUnit(t, host, second, sealedTestUnit(second, sliceTestHome+"/apps/n8n"))
	for _, u := range []string{first, second} {
		host.units[u] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + u}
	}
	// The Agent stopped right after the first program's restart was sent;
	// the second is at the same point (its file the move's text, restarted
	// in the slice, not yet seen there).
	siteMoveKilled = func(unit, step string) bool { return unit == first && step == "restarted" }
	_ = (&Agent{}).MoveSiteSliceV1(moveRequest(first, second), &transport.MoveSiteSliceResponse{})
	siteMoveKilled = func(string, string) bool { return false }
	journal, err := readSiteMoveJournal()
	if err != nil || len(journal.Units) != 2 {
		t.Fatalf("journal: %v %+v", err, journal)
	}
	for i := range journal.Units {
		u := &journal.Units[i]
		if u.Unit == second {
			writeTestUnit(t, host, second, u.After)
			host.mu.Lock()
			host.units[second].cgroup = "/celiksite_7.slice/" + second
			host.mu.Unlock()
			u.Step = siteMoveStepRestarting
		}
		u.WaitSeconds = 0 // systemd's default start wait, set below
	}
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	// Each restart takes far longer than the start's budget.
	siteSliceSettleDefault = 30 * time.Second
	siteMoveStartBudget, siteMoveStartMinWait = 400*time.Millisecond, 100*time.Millisecond
	release := make(chan struct{})
	var once sync.Once
	letThemStart := func() { once.Do(func() { close(release) }) }
	t.Cleanup(letThemStart)
	host.onRestart = func(string) { <-release }

	started := time.Now()
	recoverSiteSlicesAtStartup()
	if took := time.Since(started); took > 1500*time.Millisecond {
		t.Fatalf("the Agent's start waited %v for the put-back restarts (budget %v)", took, siteMoveStartBudget)
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed although the put-back did not finish")
	}
	for _, u := range []string{first, second} {
		if !strings.Contains(unitText(t, filepath.Join(host.dir, u)), "Slice=system.slice") {
			t.Fatalf("%s: the pre-move file is not back", u)
		}
	}
	status := round3Status(t, 7)
	if status.MoveInterrupted == nil || !status.MoveInterrupted.Kept ||
		status.MoveInterrupted.Units[first] != transport.SiteMoveStarting || status.MoveInterrupted.Units[second] != transport.SiteMoveWaiting {
		t.Fatalf("the card is not told what is left: %+v", status.MoveInterrupted)
	}
	if host.called("restart " + second) {
		t.Fatal("a restart that did not fit the start's budget was sent")
	}
	// The administrator's Try again, once the programs start: done, record gone.
	letThemStart()
	var done transport.MoveSiteSliceResponse
	_ = (&Agent{}).MoveSiteSliceV1(&transport.MoveSiteSliceRequest{RequestID: "r-a148", Recover: true}, &done)
	if !done.Done || done.Kept {
		t.Fatalf("try again: %+v", done)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed after the put-back finished")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	for _, u := range []string{first, second} {
		if host.units[u].cgroup != "/system.slice/"+u {
			t.Fatalf("%s runs in %s", u, host.units[u].cgroup)
		}
	}
}
