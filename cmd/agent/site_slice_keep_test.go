package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Going back one release from the release that places a site's programs in
// celiksite_<id>.slice (S4 R2a): this release keeps the unit's Slice= line
// when it writes the unit again (site_slice_keep.go).

// laterReleaseMovesUnit writes what the later release writes for a program it
// moved into slice: CelikPanel's own sealed text, identical but for the Slice=
// line. It returns the new text and its digest.
func laterReleaseMovesUnit(t *testing.T, path, slice string) (string, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(raw), "\n")
	if !ok || !strings.Contains(body, "\nSlice=system.slice\n") {
		t.Fatalf("not a unit of this release:\n%s", raw)
	}
	moved := strings.Replace(body, "\nSlice=system.slice\n", "\nSlice="+slice+"\n", 1)
	text := services.SealManagedText(moved)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(moved))
	return text, hex.EncodeToString(sum[:])
}

func writeSliceFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("[Slice]\nMemoryMax=256M\nTasksMax=64\nCPUQuota=50%\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUnitKeepsOnlyItsOwnSitesSliceWithItsFile(t *testing.T) {
	dir := t.TempDir()
	unit := "[Unit]\nDescription=x\n\n[Service]\nSlice=celiksite_7.slice\nUser=a\n"
	if unitKeepsSiteSlice(unit, 7, dir) {
		t.Fatal("kept without the slice's file")
	}
	writeSliceFile(t, dir, "celiksite_7.slice")
	if !unitKeepsSiteSlice(unit, 7, dir) {
		t.Fatal("the site's own slice with its file was not kept")
	}
	for name, keep := range map[string]bool{
		"another site":   unitKeepsSiteSlice(unit, 8, dir),
		"no site":        unitKeepsSiteSlice(unit, 0, dir),
		"system slice":   unitKeepsSiteSlice(strings.Replace(unit, "celiksite_7", "system", 1), 7, dir),
		"no Slice= line": unitKeepsSiteSlice("[Service]\nUser=a\n", 7, dir),
		"nested name":    unitKeepsSiteSlice(strings.Replace(unit, "celiksite_7.slice", "celiksite-7.slice", 1), 7, dir),
		"leading zero":   unitKeepsSiteSlice(strings.Replace(unit, "celiksite_7.slice", "celiksite_07.slice", 1), 7, dir),
	} {
		if keep {
			t.Errorf("%s: kept", name)
		}
	}
	// The unit file itself: a regular file keeps the line, a missing one or a
	// symbolic link to the same text keeps nothing.
	unitPath := filepath.Join(dir, "celikrt-1.service")
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	if !unitFileKeepsSiteSlice(unitPath, 7) {
		t.Fatal("a regular unit file did not keep its site's slice")
	}
	if unitFileKeepsSiteSlice(filepath.Join(dir, "celikrt-2.service"), 7) {
		t.Fatal("a missing unit file kept a slice")
	}
	if err := os.Symlink(unitPath, filepath.Join(dir, "celikrt-3.service")); err != nil {
		t.Fatal(err)
	}
	if unitFileKeepsSiteSlice(filepath.Join(dir, "celikrt-3.service"), 7) {
		t.Fatal("a unit file that is a link kept a slice")
	}
	// A slice "file" that is a symbolic link or a folder is not the slice's file.
	slice := filepath.Join(dir, "celiksite_7.slice")
	real := filepath.Join(t.TempDir(), "real.slice")
	if err := os.Rename(slice, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, slice); err != nil {
		t.Fatal(err)
	}
	if unitKeepsSiteSlice(unit, 7, dir) {
		t.Fatal("a link was taken for the slice's file")
	}
	if err := os.Remove(slice); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(slice, 0o755); err != nil {
		t.Fatal(err)
	}
	if unitKeepsSiteSlice(unit, 7, dir) {
		t.Fatal("a folder was taken for the slice's file")
	}
}

// TestRuntimeAppRenderKeepsTheSitesSlice: a program the later release moved
// into its site's slice keeps that line when this release writes its unit
// again (the same text: nothing is rewritten; a changed setting: written with
// the line). Without the slice's file, or for another site's slice, the unit
// gets the system slice as before.
func TestRuntimeAppRenderKeepsTheSitesSlice(t *testing.T) {
	useRuntimeAppHost(t)
	spec := runtimeSpec()
	render := func() transport.RuntimeAppRenderResponse {
		var resp transport.RuntimeAppRenderResponse
		if err := (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "op-runtime-slice", Target: runtimeTarget, Spec: spec}, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := render(); !resp.Done {
		t.Fatalf("first render: %+v", resp)
	}
	path := filepath.Join(appUnitDirOverride, "celikrt-12.service")
	moved, digest := laterReleaseMovesUnit(t, path, "celiksite_4.slice")
	if err := writeRuntimeAppLedger(12, runtimeAppLedger{RenderSHA256: digest}); err != nil {
		t.Fatal(err)
	}
	slice := writeSliceFile(t, appUnitDirOverride, "celiksite_4.slice")
	if resp := render(); !resp.Done || resp.UnitFile != transport.SiteFileOutcomeUnchanged {
		t.Fatalf("the moved unit was rewritten: %+v\n%s", resp, unitText(t, path))
	}
	if got := unitText(t, path); got != moved {
		t.Fatalf("the moved unit changed:\n%s", got)
	}
	spec.Limits.MemoryMB = 640
	if resp := render(); !resp.Done || resp.UnitFile != transport.SiteFileOutcomeWritten {
		t.Fatalf("a changed setting: %+v", resp)
	}
	if got := unitText(t, path); !strings.Contains(got, "\nSlice=celiksite_4.slice\n") || strings.Contains(got, "Slice=system.slice") ||
		!strings.Contains(got, "\nMemoryMax=640M\n") {
		t.Fatalf("a changed setting lost the site's slice:\n%s", got)
	}
	// The slice's file is gone: the unit goes back to the system slice.
	if err := os.Remove(slice); err != nil {
		t.Fatal(err)
	}
	if resp := render(); !resp.Done {
		t.Fatalf("render without the slice's file: %+v", resp)
	}
	if got := unitText(t, path); !strings.Contains(got, "\nSlice=system.slice\n") {
		t.Fatalf("a slice without its file was kept:\n%s", got)
	}
	// Another site's slice is never kept.
	_, digest = laterReleaseMovesUnit(t, path, "celiksite_5.slice")
	if err := writeRuntimeAppLedger(12, runtimeAppLedger{RenderSHA256: digest}); err != nil {
		t.Fatal(err)
	}
	writeSliceFile(t, appUnitDirOverride, "celiksite_5.slice")
	if resp := render(); !resp.Done {
		t.Fatalf("render over another site's slice: %+v", resp)
	}
	if got := unitText(t, path); !strings.Contains(got, "\nSlice=system.slice\n") {
		t.Fatalf("another site's slice was kept:\n%s", got)
	}
}

// TestRuntimeAppStatusReadsTheCountersInTheSitesSlice: the program's own
// cgroup files are read where it runs, also in its site's slice.
func TestRuntimeAppStatusReadsTheCountersInTheSitesSlice(t *testing.T) {
	host := useRuntimeAppHost(t)
	runtimeAppCgroupRoot = filepath.Join(t.TempDir(), "system.slice")
	host.show = "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nResult=success\nNRestarts=0\n" +
		"ControlGroup=/celiksite_4.slice/celikrt-12.service\n"
	cg := filepath.Join(filepath.Dir(runtimeAppCgroupRoot), "celiksite_4.slice", "celikrt-12.service")
	if err := os.MkdirAll(cg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cg, "memory.events"), []byte("low 0\nhigh 0\nmax 4\noom 3\noom_kill 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var resp transport.RuntimeAppStatusResponse
	if err := (&Agent{}).RuntimeAppStatusV1(&transport.RuntimeAppStatusRequest{Target: runtimeTarget, Spec: runtimeSpec()}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.OOMKills != 3 {
		t.Fatalf("counters of a program in its site's slice: %+v", resp)
	}
}

// TestNodeSiteKeepsTheSitesSlice: the same for a Node.js site's unit.
func TestNodeSiteKeepsTheSitesSlice(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	req.Trigger = transport.SiteFileTriggerCreate
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil || resp.Error != "" {
		t.Fatalf("new site: %v %+v", err, resp)
	}
	moved, _ := laterReleaseMovesUnit(t, host.path(), "celiksite_7.slice")
	writeSliceFile(t, host.dir, "celiksite_7.slice")
	req.Trigger = transport.SiteFileTriggerChange
	resp = AppApplyResponse{}
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil || resp.Error != "" {
		t.Fatalf("write again: %v %+v", err, resp)
	}
	if got := unitText(t, host.path()); got != moved {
		t.Fatalf("the moved unit was rewritten:\n%s", got)
	}
	req.Port = 3002
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if got := unitText(t, host.path()); !strings.Contains(got, "\nSlice=celiksite_7.slice\n") || strings.Contains(got, "Slice=system.slice") ||
		!strings.Contains(got, "SocketBindAllow=tcp:3002") {
		t.Fatalf("a changed port lost the site's slice (%+v):\n%s", resp, got)
	}
}
