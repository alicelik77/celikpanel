package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.148 fix round: the Agent's start with a kept move record.

// TestAKeptMoveNeverHoldsUpTheAgentsStart (review finding 5; alpha.149 fix
// round F1): an interrupted move whose two programs were restarted in the
// slice is recovered at the Agent's start without waiting for either: their
// earlier files come back, systemd is reloaded once, and each restart is only
// requested (--no-block): no restart that waits, no reading after them. Both
// are "unverified" and the record stays; the status read then sees both
// running where they ran and settles the record (F3).
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
		u.WaitSeconds = 300 // the longest start wait: the start must not take any of it
	}
	if err := journal.save(); err != nil {
		t.Fatal(err)
	}
	var waited []string
	host.onRestart = func(unit string) { waited = append(waited, unit) } // only a restart that waits calls it
	host.mu.Lock()
	host.calls, host.reloads = nil, 0
	host.mu.Unlock()

	started := time.Now()
	recoverSiteSlicesAtStartup()
	if took := time.Since(started); took > 500*time.Millisecond {
		t.Fatalf("the Agent's start took %v with a kept record", took)
	}
	if len(waited) != 0 {
		t.Fatalf("the start waited for a restart: %v", waited)
	}
	host.mu.Lock()
	calls, reloads := append([]string(nil), host.calls...), host.reloads
	host.mu.Unlock()
	if reloads != 1 {
		t.Fatalf("%d daemon-reloads at the start, want one for all: %v", reloads, calls)
	}
	requests, requested := 0, 0
	for i, c := range calls {
		if strings.HasPrefix(c, "restart --no-block ") {
			requests++
			requested += len(strings.Fields(c)) - 2 // second fix round (R5): one request for all
			for _, later := range calls[i:] {
				if strings.HasPrefix(later, "show") {
					t.Fatalf("the start read a program after requesting its restart (polling): %v", calls)
				}
			}
		}
	}
	if requests != 1 || requested != 2 {
		t.Fatalf("restarts requested: %v", calls)
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		t.Fatal("the record was removed before anyone saw the programs run")
	}
	for _, u := range []string{first, second} {
		if !strings.Contains(unitText(t, filepath.Join(host.dir, u)), "Slice=system.slice") {
			t.Fatalf("%s: the pre-move file is not back", u)
		}
		if got := readSiteMoveOutcomes()["7"].Units[u]; got != transport.SiteMoveUnverified {
			t.Fatalf("%s after the start: %q", u, got)
		}
	}
	// Right after the restarts, running where they ran is not enough (second
	// fix round, R1): a program in systemd's restart cycle reads so too.
	if status := round3Status(t, 7); status.MoveInterrupted == nil || !status.MoveInterrupted.Kept ||
		status.MoveInterrupted.Units[first] != transport.SiteMoveUnverified {
		t.Fatalf("a status read closed the record before the programs ran for their start wait: %+v", status.MoveInterrupted)
	}
	// The status read sees both running where they ran for their start wait:
	// the record goes.
	host.advance(300 * time.Second)
	status := round3Status(t, 7)
	if status.MoveInterrupted == nil || status.MoveInterrupted.Kept ||
		status.MoveInterrupted.Units[first] != transport.SiteMovePutBack || status.MoveInterrupted.Units[second] != transport.SiteMovePutBack {
		t.Fatalf("the status read did not settle it: %+v", status.MoveInterrupted)
	}
	if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
		t.Fatal("the record stayed after both programs were seen running where they ran")
	}
}
