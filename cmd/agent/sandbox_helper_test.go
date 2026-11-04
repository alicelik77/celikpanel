package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/sandboxbind"
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
