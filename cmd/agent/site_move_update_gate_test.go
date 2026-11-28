package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The update's own gate of every move (alpha.150 automatic move fix round,
// G1): while a marker of the update's transaction exists (the update has not
// reached its completion mark), every move, Try again, tasks move, undo and
// rebuild is refused with update_running before anything changes, whether or
// not the update's scripts hold the server's change lock at that moment; a
// transaction folder that cannot be inspected refuses with update_unreadable;
// without a marker the moves run as before.

// useUpdateGateRoots makes the claim read the real gate over a transaction
// folder and an empty records folder of the test's own.
func useUpdateGateRoots(t *testing.T) (transactionRoot string) {
	t.Helper()
	transactionRoot = filepath.Join(t.TempDir(), "celikpanel-release-transaction")
	if err := os.Mkdir(transactionRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	records := t.TempDir()
	oldUpdate := siteMoveUpdateState
	t.Cleanup(func() { siteMoveUpdateState = oldUpdate })
	siteMoveUpdateState = func() string { return siteMoveUpdateStateOf(transactionRoot, records, time.Now()) }
	return transactionRoot
}

func TestTheUpdatesPendingMarkerRefusesEveryMove(t *testing.T) {
	for _, marker := range agentReleaseTransactionMarkers {
		t.Run(marker, func(t *testing.T) {
			host := useSliceTestHost(t)
			root := useUpdateGateRoots(t)
			unit := "celikrt-5.service"
			before := sealedTestUnit(unit, sliceTestHome+"/app")
			writeTestUnit(t, host, unit, before)
			host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
			if err := os.WriteFile(filepath.Join(root, marker), []byte("version=1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, req := range []*transport.MoveSiteSliceRequest{moveRequest(unit), {RequestID: "r-gate-rec", Recover: true}} {
				var resp transport.MoveSiteSliceResponse
				_ = (&Agent{}).MoveSiteSliceV1(req, &resp)
				if resp.Done || resp.Reason == nil || resp.Reason.Code != "update_running" || resp.Error != siteMoveUpdateRefusal {
					t.Fatalf("with %s: %+v", marker, resp)
				}
			}
			if unitText(t, filepath.Join(host.dir, unit)) != before || host.called("daemon-reload") {
				t.Fatal("something was written while the update was not finished")
			}
			if _, err := os.Stat(siteMoveJournalPath); !os.IsNotExist(err) {
				t.Fatal("a move record was written while the update was not finished")
			}
			// The marker goes (the update's completion mark): the same move runs.
			if err := os.Remove(filepath.Join(root, marker)); err != nil {
				t.Fatal(err)
			}
			var resp transport.MoveSiteSliceResponse
			_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
			if !resp.Done {
				t.Fatalf("without a marker the move did not run: %+v", resp)
			}
		})
	}
	t.Run("unreadable", func(t *testing.T) {
		host := useSliceTestHost(t)
		root := useUpdateGateRoots(t)
		unit := "celikrt-5.service"
		before := sealedTestUnit(unit, sliceTestHome+"/app")
		writeTestUnit(t, host, unit, before)
		host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
		// The transaction folder is not a folder: it cannot be inspected.
		if err := os.Remove(root); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		var resp transport.MoveSiteSliceResponse
		_ = (&Agent{}).MoveSiteSliceV1(moveRequest(unit), &resp)
		if resp.Done || resp.Reason == nil || resp.Reason.Code != "update_unreadable" || !strings.Contains(resp.Error, "nothing was moved") {
			t.Fatalf("an unreadable transaction folder: %+v", resp)
		}
		if unitText(t, filepath.Join(host.dir, unit)) != before || host.called("daemon-reload") {
			t.Fatal("something was written while the update's folder could not be read")
		}
	})
}

func TestTheUpdatesPendingMarkerRefusesTheTasksMoves(t *testing.T) {
	for _, c := range []struct{ name, want string }{{"completion.pending", "update_running"}, {"", "update_unreadable"}} {
		t.Run(c.want, func(t *testing.T) {
			host := useTaskTestHost(t)
			host.startWith(t, moveTaskCrontab)
			root := useUpdateGateRoots(t)
			if c.name != "" {
				if err := os.WriteFile(filepath.Join(root, c.name), []byte("version=1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Remove(root); err != nil {
				t.Fatal(err)
			} else if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			move := moveTasks(t)
			undo := undoTasks(t)
			var rebuild transport.SiteTaskSourceResponse
			_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &rebuild)
			for name, got := range map[string][2]string{"move": {move.Refusal, move.Error}, "undo": {undo.Refusal, undo.Error}, "rebuild": {rebuild.Refusal, rebuild.Error}} {
				if got[0] != c.want || got[1] != siteMoveRefusalText(c.want) {
					t.Fatalf("%s: %q %q", name, got[0], got[1])
				}
			}
			if crontab, _ := host.crontab(taskTestAccount); crontab != moveTaskCrontab || len(host.unitFiles(t)) != 0 || len(host.systemdCalls()) != 0 {
				t.Fatalf("something changed: %v", host.systemdCalls())
			}
		})
	}
}

// TestTheUpdateGateReadsOnly: the gate never writes, moves or removes a
// marker, and a missing transaction folder (no update ever ran) holds none.
func TestTheUpdateGateReadsOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "txn")
	if got := siteMoveUpdateStateOf(root, t.TempDir(), time.Now()); got != "" {
		t.Fatalf("no transaction folder: %q", got)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "completion.pending")
	if err := os.WriteFile(marker, []byte("version=1\noperation=rollback\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := siteMoveUpdateStateOf(root, t.TempDir(), time.Now()); got != "update_running" {
		t.Fatalf("a return's completion marker: %q", got)
	}
	entries, _ := os.ReadDir(root)
	if raw, _ := os.ReadFile(marker); len(entries) != 1 || string(raw) != "version=1\noperation=rollback\n" {
		t.Fatalf("the gate changed the transaction folder: %v", entries)
	}
}

// TestTheUpdatesLastStepHasItsOwnRefusal (second review, N1): when the only
// marker left is an update's scheduler-restore.pending (the update is
// installed; turning Certbot's renewal back on has not finished), every move
// is still refused before anything changes, with update_finishing and its own
// words; with any other marker beside it, or a return's scheduler marker, the
// refusal stays update_running.
func TestTheUpdatesLastStepHasItsOwnRefusal(t *testing.T) {
	const update = "version=1\ntoken=t\noperation=update\nsnapshot=s\n"
	const rollback = "version=1\ntoken=t\noperation=rollback\nsnapshot=s\n"
	for _, c := range []struct {
		name    string
		markers map[string]string
		want    string
	}{
		{"scheduler only", map[string]string{"scheduler-restore.pending": update}, "update_finishing"},
		{"with completion", map[string]string{"scheduler-restore.pending": update, "completion.pending": update}, "update_running"},
		{"a return's", map[string]string{"scheduler-restore.pending": rollback}, "update_running"},
		{"no operation", map[string]string{"scheduler-restore.pending": "version=1\n"}, "update_running"},
	} {
		t.Run(c.name, func(t *testing.T) {
			host := useSliceTestHost(t)
			root := useUpdateGateRoots(t)
			unit := "celikrt-5.service"
			before := sealedTestUnit(unit, sliceTestHome+"/app")
			writeTestUnit(t, host, unit, before)
			host.units[unit] = &sliceTestUnit{active: "active", cgroup: "/system.slice/" + unit}
			for name, text := range c.markers {
				if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, req := range []*transport.MoveSiteSliceRequest{moveRequest(unit), {RequestID: "r-gate-rec", Recover: true}} {
				var resp transport.MoveSiteSliceResponse
				_ = (&Agent{}).MoveSiteSliceV1(req, &resp)
				if resp.Done || resp.Reason == nil || resp.Reason.Code != c.want || resp.Error != siteMoveRefusalText(c.want) {
					t.Fatalf("%+v", resp)
				}
			}
			if unitText(t, filepath.Join(host.dir, unit)) != before || host.called("daemon-reload") {
				t.Fatal("something was written before the update's last step finished")
			}
		})
	}
	if text := siteMoveRefusalText("update_finishing"); !strings.Contains(text, "nothing was moved") || !strings.Contains(text, "celikpanel-release-recovery.service") {
		t.Fatalf("the words: %q", text)
	}
	t.Run("tasks", func(t *testing.T) {
		host := useTaskTestHost(t)
		host.startWith(t, moveTaskCrontab)
		root := useUpdateGateRoots(t)
		if err := os.WriteFile(filepath.Join(root, "scheduler-restore.pending"), []byte(update), 0o600); err != nil {
			t.Fatal(err)
		}
		move := moveTasks(t)
		undo := undoTasks(t)
		var rebuild transport.SiteTaskSourceResponse
		_ = (&Agent{}).RebuildSiteTaskSourceV1(&transport.SiteTasksMoveRequest{CronTenant: transport.CronTenant{SiteID: taskTestSite}}, &rebuild)
		for name, got := range map[string][2]string{"move": {move.Refusal, move.Error}, "undo": {undo.Refusal, undo.Error}, "rebuild": {rebuild.Refusal, rebuild.Error}} {
			if got[0] != "update_finishing" || got[1] != siteMoveUpdateFinishing {
				t.Fatalf("%s: %q %q", name, got[0], got[1])
			}
		}
		if crontab, _ := host.crontab(taskTestAccount); crontab != moveTaskCrontab || len(host.unitFiles(t)) != 0 || len(host.systemdCalls()) != 0 {
			t.Fatalf("something changed: %v", host.systemdCalls())
		}
	})
}
