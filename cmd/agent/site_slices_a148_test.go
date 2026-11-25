package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.148 fix round (the independent review of the merged branch).

// placeReading answers systemd's reading of where a unit runs as one of the
// cases a writer must survive: "fail" (systemd does not answer), "empty" (a
// running state with an empty ControlGroup), "odd" (a running state with a
// ControlGroup that is neither slice), "slice:<id>" and "system" (it runs
// there), "stopped".
func placeReading(mode, unit string) ([]byte, error) {
	switch {
	case mode == "fail":
		return []byte("Failed to connect to bus: Connection refused"), errors.New("exit status 1")
	case mode == "empty":
		return []byte("ActiveState=activating\nControlGroup=\n"), nil
	case mode == "odd":
		return []byte("ActiveState=active\nControlGroup=/user.slice/" + unit + "\n"), nil
	case mode == "system":
		return []byte("ActiveState=active\nControlGroup=/system.slice/" + unit + "\n"), nil
	case strings.HasPrefix(mode, "slice:"):
		return []byte("ActiveState=active\nControlGroup=/celiksite_" + strings.TrimPrefix(mode, "slice:") + ".slice/" + unit + "\n"), nil
	}
	return []byte("ActiveState=inactive\nControlGroup=\n"), nil
}

// withPlaceReading routes the writers' place reading through placeReading
// with *mode; every other systemctl call goes to the test host's own fake.
func withPlaceReading(t *testing.T, mode *string) {
	t.Helper()
	inner := appUnitSystemctl
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		if len(args) == 3 && args[0] == "show" && args[2] == "--property=ActiveState,ControlGroup" {
			return placeReading(*mode, args[1])
		}
		return inner(args...)
	}
}

func sliceLineOf(t *testing.T, path string) string {
	t.Helper()
	for _, line := range strings.Split(unitText(t, path), "\n") {
		if strings.HasPrefix(line, "Slice=") {
			return line
		}
	}
	return ""
}

// unknownPlaceReadings are the readings that cannot say where a running
// program runs.
var unknownPlaceReadings = []string{"fail", "empty", "odd"}

// TestARuntimeAppSavedWhileItsPlaceIsUnknownKeepsItsLimits: an uploaded
// program or Python app inside its site's slice, saved with the site's
// limits in the request while systemd does not answer or shows no usable
// place, keeps Slice=celiksite_<id>.slice (review finding 1: it was written
// Slice=system.slice and left its limits at its next restart). A program
// whose file names the system slice is never moved in by such a reading.
func TestARuntimeAppSavedWhileItsPlaceIsUnknownKeepsItsLimits(t *testing.T) {
	useRuntimeAppHost(t)
	units, _ := sandboxUnitDir()
	appUnitDirOverride = units // the unit and the slice file in one folder, as on a server
	mode := "stopped"
	withPlaceReading(t, &mode)
	target := runtimeTarget
	target.Slice = &transport.SiteSlice{SiteID: 4, Domain: "shop.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}
	spec := runtimeSpec()
	path := filepath.Join(units, "celikrt-12.service")
	render := func() {
		t.Helper()
		var resp transport.RuntimeAppRenderResponse
		if err := (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "op-runtime-a148", Target: target, Spec: spec}, &resp); err != nil || !resp.Done {
			t.Fatalf("render (%s): %v %+v", mode, err, resp)
		}
	}
	render()
	if got := sliceLineOf(t, path); got != "Slice=celiksite_4.slice" {
		t.Fatalf("a program written while stopped: %q", got)
	}
	for i, reading := range unknownPlaceReadings {
		mode = reading
		spec.Limits.MemoryMB = 600 + i
		render()
		if got := sliceLineOf(t, path); got != "Slice=celiksite_4.slice" || !strings.Contains(unitText(t, path), "MemoryMax="+strconv.Itoa(600+i)+"M\n") {
			t.Fatalf("saved while the place reading is %q: %q\n%s", reading, got, unitText(t, path))
		}
	}
	// The system slice: a running program there stays there, and an unknown
	// reading never moves it in.
	if err := os.Remove(filepath.Join(units, "celiksite_4.slice")); err != nil {
		t.Fatal(err)
	}
	mode, spec.Limits.MemoryMB = "system", 700
	render()
	if got := sliceLineOf(t, path); got != "Slice=system.slice" {
		t.Fatalf("a running program in the system slice: %q", got)
	}
	writeSliceFile(t, units, "celiksite_4.slice")
	for i, reading := range unknownPlaceReadings {
		mode = reading
		spec.Limits.MemoryMB = 710 + i
		render()
		if got := sliceLineOf(t, path); got != "Slice=system.slice" {
			t.Fatalf("an unknown reading (%s) moved a program in: %q", reading, got)
		}
	}
}

// TestANodeSiteSavedWhileItsPlaceIsUnknownKeepsItsLimits: the same for the
// Node.js application's unit.
func TestANodeSiteSavedWhileItsPlaceIsUnknownKeepsItsLimits(t *testing.T) {
	host := useNodeTestHost(t)
	units, _ := sandboxUnitDir()
	host.dir, appUnitDirOverride = units, units
	mode := "stopped"
	withPlaceReading(t, &mode)
	req := withNode(t, nodeRequest())
	req.Trigger = transport.SiteFileTriggerCreate
	req.Slice = &transport.SiteSlice{SiteID: 7, Domain: "example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}
	apply := func() {
		t.Helper()
		var resp AppApplyResponse
		if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil || resp.Error != "" {
			t.Fatalf("apply (%s): %v %+v", mode, err, resp)
		}
	}
	apply()
	if got := sliceLineOf(t, host.path()); got != "Slice=celiksite_7.slice" {
		t.Fatalf("a new application: %q", got)
	}
	req.Trigger = transport.SiteFileTriggerChange
	for i, reading := range unknownPlaceReadings {
		mode = reading
		req.Command = "node a148-" + strconv.Itoa(i) + ".js"
		apply()
		if got := sliceLineOf(t, host.path()); got != "Slice=celiksite_7.slice" || !strings.Contains(unitText(t, host.path()), `"node a148-`+strconv.Itoa(i)+`.js"`) {
			t.Fatalf("saved while the place reading is %q: %q\n%s", reading, got, unitText(t, host.path()))
		}
	}
	if err := os.Remove(filepath.Join(units, "celiksite_7.slice")); err != nil {
		t.Fatal(err)
	}
	mode, req.Command = "system", "node system.js"
	apply()
	if got := sliceLineOf(t, host.path()); got != "Slice=system.slice" {
		t.Fatalf("a running application in the system slice: %q", got)
	}
	writeSliceFile(t, units, "celiksite_7.slice")
	for i, reading := range unknownPlaceReadings {
		mode = reading
		req.Command = "node b148-" + strconv.Itoa(i) + ".js"
		apply()
		if got := sliceLineOf(t, host.path()); got != "Slice=system.slice" {
			t.Fatalf("an unknown reading (%s) moved the application in: %q", reading, got)
		}
	}
}

// TestAServiceAppSavedWhileItsPlaceIsUnknownKeepsItsLimits: the same for a
// service app's unit (ServiceAppActivateV1 decides with
// serviceAppUnitInSlice).
func TestAServiceAppSavedWhileItsPlaceIsUnknownKeepsItsLimits(t *testing.T) {
	old := appUnitSystemctl
	t.Cleanup(func() { appUnitSystemctl = old })
	appUnitSystemctl = func(...string) ([]byte, error) { return nil, nil }
	mode := "stopped"
	withPlaceReading(t, &mode)
	dir := t.TempDir()
	path := filepath.Join(dir, "celiksvc-5.service")
	writeSliceFile(t, dir, "celiksite_9.slice")
	sc := serviceAppContext{installID: 5, siteID: 9,
		slice: siteSliceFor(&transport.SiteSlice{SiteID: 9, Domain: "flows.example.com", MemoryMB: 1024, Tasks: 256, CPUPercent: 150}, 9)}
	if sc.slice == nil {
		t.Fatal("the site's limits were not accepted")
	}
	if !serviceAppUnitInSlice(sc, 5, path) {
		t.Fatal("a new service app is written outside its site's limits")
	}
	if err := os.WriteFile(path, []byte("[Service]\nSlice=celiksite_9.slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, reading := range append([]string{"slice:9"}, unknownPlaceReadings...) {
		mode = reading
		if !serviceAppUnitInSlice(sc, 5, path) {
			t.Fatalf("a service app inside its limits, place reading %q: written outside them", reading)
		}
	}
	if err := os.WriteFile(path, []byte("[Service]\nSlice=system.slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, reading := range append([]string{"system"}, unknownPlaceReadings...) {
		mode = reading
		if serviceAppUnitInSlice(sc, 5, path) {
			t.Fatalf("a service app outside its limits, place reading %q: moved in without the administrator", reading)
		}
	}
	// The file names the slice but the slice's file is gone: systemd makes
	// that slice without limits, so nothing is kept (as without the limits).
	if err := os.WriteFile(path, []byte("[Service]\nSlice=celiksite_9.slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "celiksite_9.slice")); err != nil {
		t.Fatal(err)
	}
	mode = "fail"
	if serviceAppUnitInSlice(sc, 5, path) {
		t.Fatal("a slice without its file was kept")
	}
}
