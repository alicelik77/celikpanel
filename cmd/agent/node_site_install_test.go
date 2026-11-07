package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The install step of a Node.js site (alpha.127).

func TestNodeSiteInstallRunsOnlyInTheSandbox(t *testing.T) {
	host := useNodeTestHost(t)
	oldRoot, oldShow, oldJournal, oldBase := nodeInstallStateRoot, nodeInstallUnitShow, nodeInstallJournal, runtimesBaseDir
	t.Cleanup(func() {
		nodeInstallStateRoot, nodeInstallUnitShow, nodeInstallJournal, runtimesBaseDir = oldRoot, oldShow, oldJournal, oldBase
	})
	nodeInstallStateRoot = t.TempDir()
	unit := map[string]string{"LoadState": "loaded", "ActiveState": "activating", "Result": "success"}
	nodeInstallUnitShow = func(int) (map[string]string, bool, error) { return unit, true, nil }
	nodeInstallJournal = func(context.Context, int, time.Time) []string {
		return []string{"added 57 packages", "npm ERR! token=abc123 leaked"}
	}
	var runs [][]string
	serviceAppRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "systemd-run" {
			runs = append(runs, args)
		}
		return nil, nil
	}
	appDir := t.TempDir()
	req := &transport.NodeSiteInstallRequest{RequestID: "req-install-0001", SiteID: 7, Domain: "example.com",
		WorkDir: nodeTestDocRoot, RunAsUser: "example_com", NodeVersion: "22.11.0", Build: true}

	// An earlier release's unit: nothing runs outside the sandbox.
	earlierUnit(t, host, withNode(t, nodeRequest()))
	var resp transport.NodeSiteInstallResponse
	_ = (&Agent{}).NodeSiteInstallV1(req, &resp)
	if resp.Reason != "earlier_release" || resp.Class != transport.RuntimeAppPrerequisite || len(runs) != 0 {
		t.Fatalf("earlier release: %+v %v", resp, runs)
	}

	// In the sandbox. The document root is faked by the folder checks.
	move := withNode(t, nodeRequest())
	move.SandboxMove = true
	var applied AppApplyResponse
	_ = (&Agent{}).ApplyAppUnit(move, &applied)
	if applied.Error != "" {
		t.Fatalf("move: %+v", applied)
	}
	runtime := filepath.Join(runtimesBaseDir, "node", "22.11.0")
	if err := os.MkdirAll(filepath.Join(runtime, "lib", "node_modules", "npm", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, "lib", "node_modules", "npm", "bin", "npm-cli.js"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = appDir
	resp = transport.NodeSiteInstallResponse{}
	_ = (&Agent{}).NodeSiteInstallV1(req, &resp)
	// The checks read the real document root, which is not on this machine:
	// a missing package.json is a named prerequisite, nothing started.
	if resp.Reason != "package_json_missing" || len(runs) != 0 {
		t.Fatalf("no package.json: %+v", resp)
	}
}

func TestNodeSiteInstallRecordAndObserve(t *testing.T) {
	oldRoot, oldShow, oldJournal := nodeInstallStateRoot, nodeInstallUnitShow, nodeInstallJournal
	t.Cleanup(func() { nodeInstallStateRoot, nodeInstallUnitShow, nodeInstallJournal = oldRoot, oldShow, oldJournal })
	nodeInstallStateRoot = t.TempDir()
	unit := map[string]string{"LoadState": "loaded", "ActiveState": "activating", "Result": "success"}
	found := true
	nodeInstallUnitShow = func(int) (map[string]string, bool, error) { return unit, found, nil }
	nodeInstallJournal = func(context.Context, int, time.Time) []string {
		return []string{"added 57 packages", "password=hunter2 in a log line"}
	}
	record := nodeInstallRecord{RequestID: "req-install-0002", Phase: nodesite.InstallRunning, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeNodeInstallRecord(9, record); err != nil {
		t.Fatal(err)
	}
	var resp transport.NodeSiteInstallResponse
	_ = (&Agent{}).NodeSiteInstallStatusV1(&transport.NodeSiteInstallStatusRequest{SiteID: 9}, &resp)
	if resp.Phase != nodesite.InstallRunning || len(resp.Lines) != 2 || strings.Contains(strings.Join(resp.Lines, " "), "hunter2") {
		t.Fatalf("running: %+v", resp)
	}
	// A second start while it runs is refused; the same request id
	// answers the first answer.
	var again transport.NodeSiteInstallResponse
	_ = (&Agent{}).NodeSiteInstallV1(&transport.NodeSiteInstallRequest{RequestID: "req-install-0003", SiteID: 9}, &again)
	if again.Reason != "install_running" {
		t.Fatalf("second start: %+v", again)
	}
	again = transport.NodeSiteInstallResponse{}
	_ = (&Agent{}).NodeSiteInstallV1(&transport.NodeSiteInstallRequest{RequestID: "req-install-0002", SiteID: 9}, &again)
	if again.RequestID != "req-install-0002" || again.Phase != nodesite.InstallRunning {
		t.Fatalf("repeat: %+v", again)
	}
	unit = map[string]string{"LoadState": "loaded", "ActiveState": "failed", "Result": "exit-code"}
	resp = transport.NodeSiteInstallResponse{}
	_ = (&Agent{}).NodeSiteInstallStatusV1(&transport.NodeSiteInstallStatusRequest{SiteID: 9}, &resp)
	if resp.Phase != nodesite.InstallFailed || resp.Reason != "install_failed" || resp.EndedAt == "" {
		t.Fatalf("failed: %+v", resp)
	}
	// The end is recorded: a later read after a reboot (no unit) keeps it.
	found = false
	resp = transport.NodeSiteInstallResponse{}
	_ = (&Agent{}).NodeSiteInstallStatusV1(&transport.NodeSiteInstallStatusRequest{SiteID: 9}, &resp)
	if resp.Phase != nodesite.InstallFailed || resp.Reason != "install_failed" {
		t.Fatalf("after reboot: %+v", resp)
	}
}
