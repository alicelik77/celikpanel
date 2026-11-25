//go:build linux

package sitesandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRenderedSliceAndUnitPassSystemdAnalyze (S4 R2a): the site's slice and a
// unit placed in it, as rendered, are accepted by the host's own
// `systemd-analyze verify`. Private files only: nothing is installed, loaded
// or started. It runs where CELIKPANEL_SYSTEMD_ANALYZE=1 and the tool exists.
func TestRenderedSliceAndUnitPassSystemdAnalyze(t *testing.T) {
	if os.Getenv("CELIKPANEL_SYSTEMD_ANALYZE") != "1" {
		t.Skip("set CELIKPANEL_SYSTEMD_ANALYZE=1 to run systemd-analyze verify on the rendered texts")
	}
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is not installed")
	}
	dir := t.TempDir()
	slice, err := SliceUnitText(12, "shop.example.com", dir, SliceLimits{MemoryMB: 1024, Tasks: 256, CPUPercent: 150})
	if err != nil {
		t.Fatal(err)
	}
	p := programProfile()
	p.SiteID, p.InSlice = 12, true
	caps := fullCaps(257)
	lines, err := Directives(p, caps)
	if err != nil {
		t.Fatal(err)
	}
	unitLines, pre, post, err := HelperLines(p, caps)
	if err != nil {
		t.Fatal(err)
	}
	unit := "[Unit]\nDescription=verify\n" + strings.Join(append(UnitDirectives(), unitLines...), "\n") + "\n\n[Service]\nType=simple\n" +
		strings.Join(pre, "\n") + "\nExecStart=/bin/true\n" + strings.Join(post, "\n") + "\n" + strings.Join(lines, "\n") + "\n"
	// The bind unit's template, so Requires= resolves.
	files := map[string]string{
		SliceName(12): slice,
		p.Unit:        unit,
		BindTemplate:  strings.ReplaceAll(BindTemplateText(), HelperPath, "/bin/true"),
		"celikpanel-sandbox@" + strings.TrimSuffix(p.Unit, ".service") + ".service": strings.ReplaceAll(strings.ReplaceAll(BindTemplateText(), HelperPath, "/bin/true"), "%i", strings.TrimSuffix(p.Unit, ".service")),
	}
	var paths []string
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		if name == SliceName(12) || name == p.Unit {
			paths = append(paths, path)
		}
	}
	cmd := exec.Command(analyze, append([]string{"verify", "--man=no"}, paths...)...)
	cmd.Env = append(os.Environ(), "SYSTEMD_UNIT_PATH="+dir+":")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	// The helper path does not exist outside a server; that is the only
	// complaint allowed (the egress lines name it).
	var other []string
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.Contains(line, HelperPath) {
			continue
		}
		other = append(other, line)
	}
	if len(other) > 0 {
		t.Fatalf("systemd-analyze verify (%v):\n%s", err, strings.Join(other, "\n"))
	}
	t.Logf("systemd-analyze verify accepted %s and %s (%s)", SliceName(12), p.Unit, text)
}
