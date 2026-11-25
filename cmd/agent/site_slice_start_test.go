package main

import (
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

	// Stopped: the Start writes it into the site's slice first.
	host.mu.Lock()
	host.show = "LoadState=loaded\nActiveState=inactive\nSubState=dead\n"
	host.mu.Unlock()
	start()
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
