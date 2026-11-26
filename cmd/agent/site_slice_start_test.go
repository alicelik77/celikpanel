package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// TestAStoppedProgramStartedFromThePanelStartsInsideItsLimits (alpha.149, the
// alpha.148 lab's O2, decision 4): an uploaded program whose unit still names
// the system slice and that does not run is written into its site's slice by
// the Start (like any write while it does not run), the slice file first; a
// running one is never moved by a Start; an owner's changed file is kept.
func TestAStoppedProgramStartedFromThePanelStartsInsideItsLimits(t *testing.T) {
	host := useRuntimeAppHost(t)
	units, _ := sandboxUnitDir()
	appUnitDirOverride = units // the unit and the slice file in one folder, as on a server
	target := runtimeTarget
	var rendered transport.RuntimeAppRenderResponse
	if err := (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "op-runtime-a149", Target: target, Spec: runtimeSpec()}, &rendered); err != nil || !rendered.Done {
		t.Fatalf("render: %v %+v", err, rendered)
	}
	unit := "celikrt-12.service"
	path := filepath.Join(units, unit)
	if got := sliceLineOf(t, path); got != "Slice=system.slice" {
		t.Fatalf("a program written before the limits: %q", got)
	}
	start := func() {
		t.Helper()
		var step transport.RuntimeAppStepResponse
		_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: target, Action: "start"}, &step)
	}
	target.Slice = &transport.SiteSlice{SiteID: 4, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}

	// Running: only the administrator's move takes it inside.
	start()
	if got := sliceLineOf(t, path); got != "Slice=system.slice" {
		t.Fatalf("a running program was moved by a Start: %q", got)
	}

	// Stopped: the Start writes it into the site's slice first, after this
	// release's sandbox helper and SMTP block are in place (fix round F2).
	host.mu.Lock()
	host.show = "LoadState=loaded\nActiveState=inactive\nSubState=dead\n"
	host.mu.Unlock()
	oldPrepare := siteMovePrepare
	t.Cleanup(func() { siteMovePrepare = oldPrepare })
	prepared := 0
	siteMovePrepare = func() error {
		if sliceLineOf(t, path) != "Slice=system.slice" {
			t.Error("the unit was written before the helper was put in place")
		}
		prepared++
		return nil
	}
	start()
	if prepared != 1 {
		t.Fatalf("the helper and SMTP block were prepared %d times", prepared)
	}
	text := unitText(t, path)
	if got := sliceLineOf(t, path); got != "Slice=celiksite_4.slice" {
		t.Fatalf("a stopped program started from the panel: %q\n%s", got, text)
	}
	if _, ours := siteUnitSealedBody(unit, text); !ours {
		t.Fatal("the unit is no longer CelikPanel's own text")
	}
	if _, err := os.Stat(filepath.Join(units, "celiksite_4.slice")); err != nil {
		t.Fatalf("the site's slice file was not put in place first: %v", err)
	}

	// An owner's changed file is kept as it is.
	changed := text + "# the owner's line\n"
	if err := os.WriteFile(path, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	target.Slice = &transport.SiteSlice{SiteID: 5, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}
	start()
	if unitText(t, path) != changed {
		t.Fatal("an owner's changed unit was rewritten by a Start")
	}
}

// TestAStartWithoutThisReleasesHelperWritesNothing (fix round F2): before
// alpha.147 the sandbox helper added only system-slice cgroups to the SMTP
// block, so a unit in its site's slice was refused at its ExecStartPre. When
// this release's helper and SMTP block cannot be put in place, a Start of a
// stopped program writes nothing (no unit change, no slice file) and the
// program starts where it started before.
func TestAStartWithoutThisReleasesHelperWritesNothing(t *testing.T) {
	host := useRuntimeAppHost(t)
	units, _ := sandboxUnitDir()
	appUnitDirOverride = units
	target := runtimeTarget
	var rendered transport.RuntimeAppRenderResponse
	if err := (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "op-runtime-a149-f2", Target: target, Spec: runtimeSpec()}, &rendered); err != nil || !rendered.Done {
		t.Fatalf("render: %v %+v", err, rendered)
	}
	path := filepath.Join(units, "celikrt-12.service")
	written := unitText(t, path)
	host.mu.Lock()
	host.show = "LoadState=loaded\nActiveState=inactive\nSubState=dead\n"
	host.mu.Unlock()
	oldPrepare := siteMovePrepare
	t.Cleanup(func() { siteMovePrepare = oldPrepare })
	siteMovePrepare = func() error { return errors.New("the helper could not be copied") }
	target.Slice = &transport.SiteSlice{SiteID: 4, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}
	var step transport.RuntimeAppStepResponse
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: target, Action: "start"}, &step)
	if unitText(t, path) != written {
		t.Fatal("the unit was written although the helper could not be put in place")
	}
	if _, err := os.Stat(filepath.Join(units, "celiksite_4.slice")); !os.IsNotExist(err) {
		t.Fatalf("the slice file was written: %v", err)
	}
}
