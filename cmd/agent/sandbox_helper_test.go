package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

func withSandboxHelperFiles(t *testing.T, program string) (dst string) {
	t.Helper()
	oldSource, oldDir, oldLedger, oldSum := sandboxHelperSource, sandboxEgressDir, sandboxLedgerPath, sandboxHelperSelfSum
	t.Cleanup(func() {
		sandboxHelperSource, sandboxEgressDir, sandboxLedgerPath, sandboxHelperSelfSum = oldSource, oldDir, oldLedger, oldSum
	})
	base := t.TempDir()
	sandboxHelperSource = filepath.Join(base, "agent")
	if err := os.WriteFile(sandboxHelperSource, []byte(program), 0o755); err != nil {
		t.Fatal(err)
	}
	sandboxEgressDir = filepath.Join(base, "celikpanel-sandbox")
	sandboxLedgerPath = filepath.Join(base, "state", "native-files.json")
	sandboxHelperSelfSum = ""
	return filepath.Join(sandboxEgressDir, sitesandbox.HelperName)
}

func readString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// alpha144HelperEnsure is release alpha.144's decision about the helper
// (a0da48bcb:cmd/agent/sandbox_helper.go ensureSandboxHelperBinary), copied
// here as data: the one file it manages, and what it does with it. It is
// not imported; it is what an Agent of that release does on a server this
// release ran on, after the owner went back to it.
func alpha144HelperEnsure(t *testing.T, dir, self string) {
	t.Helper()
	dst := filepath.Join(dir, "sandbox-helper") // the only name alpha.144 knows
	ledger := readSandboxLedger()
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	selfSum := sum([]byte(self))
	if raw, err := os.ReadFile(dst); err == nil {
		current := sum(raw)
		if current == selfSum {
			return
		}
		if recorded := ledger[dst].RenderSHA256; recorded == "" || recorded != current {
			return // the owner's, kept
		}
	}
	if err := os.WriteFile(dst, []byte(self), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := setSandboxLedgerEntry(dst, "sandbox_helper", selfSum); err != nil {
		t.Fatal(err)
	}
}

// TestTaskUnitsKeepTheirHelperWhenAnEarlierReleaseReturns (S4 R2b second
// fix round, review N3): the task units call their own copy of the helper,
// which this release installs and repairs like the programs' copy; an
// alpha.144 Agent (its decision as data above) overwrites the programs'
// copy with its own, which refuses celikcron-* names, and leaves the
// tasks' copy alone - so going back does not stop every moved site's tasks.
// On 6db8cd368 the units named the programs' copy, which the old Agent
// replaced.
func TestTaskUnitsKeepTheirHelperWhenAnEarlierReleaseReturns(t *testing.T) {
	dst := withSandboxHelperFiles(t, "agent r2b")
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	task := filepath.Join(sandboxEgressDir, sitesandbox.TaskHelperName)
	if readString(t, task) != "agent r2b" || readString(t, dst) != "agent r2b" {
		t.Fatal("both copies are not installed")
	}
	if fi, _ := os.Stat(task); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if readSandboxLedger()[task].Kind != "sandbox_task_helper" {
		t.Fatalf("the tasks' copy has no record of its own: %+v", readSandboxLedger()[task])
	}
	// The owner goes back to alpha.144; its Agent touches a program.
	alpha144HelperEnsure(t, sandboxEgressDir, "agent alpha.144")
	if readString(t, dst) != "agent alpha.144" {
		t.Fatal("the stand-in of alpha.144 did not replace the programs' copy (the test is wrong)")
	}
	if readString(t, task) != "agent r2b" {
		t.Fatal("the earlier release replaced the tasks' copy")
	}
	// A rendered task unit names the tasks' copy, never the programs' one.
	u := sitecron.Unit{SiteID: 7, N: 1, TaskID: "0123456789abcdef", Domain: "shop.example.test", Account: "shop_example_test",
		Group: "shop_example_test", Home: taskTestHome(), Shell: "/bin/sh", Spec: sitecron.Spec{Calendars: []string{"*-*-* *:00:00"}},
		Egress: true, SliceFile: "/etc/systemd/system/celiksite_7.slice"}
	service, err := sitecron.ServiceText(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(service, "ExecStartPre=+"+sitesandbox.TaskHelperPath+" egress-add ") {
		t.Fatalf("the task unit does not name its own helper:\n%s", service)
	}
	if strings.Contains(service, sitesandbox.HelperPath+" ") {
		t.Fatalf("the task unit names the programs' helper:\n%s", service)
	}
	// Back on this release (or a later one): the tasks' copy is CelikPanel's
	// and is replaced by the new Agent; the programs' copy too.
	if err := os.WriteFile(sandboxHelperSource, []byte("agent r2b+1"), 0o755); err != nil {
		t.Fatal(err)
	}
	sandboxHelperSelfSum = ""
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	if readString(t, task) != "agent r2b+1" || readString(t, dst) != "agent r2b+1" {
		t.Fatalf("after the update: tasks %q, programs %q", readString(t, task), readString(t, dst))
	}
	// The helper answers under its second name too.
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{sitesandbox.TaskHelperPath, "egress-add"}
	if !sandboxHelperMode() {
		t.Fatal("the program does not act as the helper under the tasks' name")
	}
}

// TestSandboxHelperCopyFollowsTheOwnerRule: the Agent installs a copy of
// itself, replaces its own earlier copy after an update, and keeps a file
// the owner put there instead.
func TestSandboxHelperCopyFollowsTheOwnerRule(t *testing.T) {
	dst := withSandboxHelperFiles(t, "agent v1")
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	if readString(t, dst) != "agent v1" {
		t.Fatal("the helper is not a copy of the Agent")
	}
	if fi, _ := os.Stat(dst); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if _, err := os.Stat(dst + ".partial"); !os.IsNotExist(err) {
		t.Fatal("a partial copy was left behind")
	}
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	// An Agent update: the earlier copy is CelikPanel's, so it is replaced.
	if err := os.WriteFile(sandboxHelperSource, []byte("agent v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	sandboxHelperSelfSum = ""
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	if readString(t, dst) != "agent v2" {
		t.Fatal("the Agent's own earlier copy was not replaced")
	}
	// The owner's own helper is kept.
	if err := os.WriteFile(dst, []byte("owner's helper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sandboxHelperSource, []byte("agent v3"), 0o755); err != nil {
		t.Fatal(err)
	}
	sandboxHelperSelfSum = ""
	if err := ensureSandboxHelper(); err != nil {
		t.Fatal(err)
	}
	if readString(t, dst) != "owner's helper" {
		t.Fatal("the owner's helper was overwritten")
	}
	// The bind unit's template is written beside the units.
	units, _ := sandboxUnitDir()
	if tmpl := readString(t, filepath.Join(units, sitesandbox.BindTemplate)); !strings.Contains(tmpl, "ExecStart="+sitesandbox.HelperPath+" bind --unit=%i.service") {
		t.Fatalf("bind template:\n%s", tmpl)
	}
	// Something that is not a file is refused, not replaced.
	if err := os.Remove(dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureSandboxHelper(); err == nil {
		t.Fatal("a folder at the helper's place was accepted")
	}
}

func TestSandboxHelperModeIsTheProgramName(t *testing.T) {
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"/usr/lib/celikpanel-sandbox/sandbox-helper", "start"}
	if !sandboxHelperMode() {
		t.Fatal("the helper's name was not recognised")
	}
	os.Args = []string{"/opt/celikpanel/bin/agent", "sandbox-helper"}
	if sandboxHelperMode() {
		t.Fatal("an argument made the Agent act as the helper")
	}
}

// TestAFolderTheHelperWouldRefuseIsAPrerequisite: the Agent answers with the
// helper's verdict before it writes or starts a unit, as a prerequisite the
// screens word (an unknown result stays unknown).
func TestAFolderTheHelperWouldRefuseIsAPrerequisite(t *testing.T) {
	old := sandboxFolderRefusal
	t.Cleanup(func() { sandboxFolderRefusal = old })
	sc := serviceAppContext{account: serviceAppAccount{UID: 1005}}
	for code, class := range map[string]string{
		sandboxbind.CodeNotReal:    transport.ServiceAppPrerequisite,
		sandboxbind.CodeNotOwned:   transport.ServiceAppPrerequisite,
		sandboxbind.CodeUnreadable: transport.ServiceAppUnknown,
	} {
		sandboxFolderRefusal = func(string, int) (string, string) { return code, "the reason" }
		var resp transport.ServiceAppStepResponse
		if !serviceAppFolderRefused(sc, &resp) || resp.Reason != code || resp.Class != class || resp.Done {
			t.Errorf("%s: %+v", code, resp)
		}
	}
	sandboxFolderRefusal = func(string, int) (string, string) { return "", "" }
	var resp transport.ServiceAppStepResponse
	if serviceAppFolderRefused(sc, &resp) || resp.Reason != "" {
		t.Fatalf("a good folder refused: %+v", resp)
	}
}
