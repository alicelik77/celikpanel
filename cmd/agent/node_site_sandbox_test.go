package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js sites in the site sandbox (alpha.127): which text ApplyAppUnit
// writes, the move of an earlier release's unit, its proof and put-back.

const nodeTestDocRoot = "/var/www/celikpanel/subscriptions/3/sites/7/public_html"

type nodeTestHost struct {
	mu    sync.Mutex
	dir   string
	calls []string
	// active is the unit's ActiveState; listening whether its process
	// listens on the port (on every address, as Node.js does); proofBroken
	// makes systemd's view differ from the profile.
	active      string
	listening   bool
	proofBroken bool
	caps        sitesandbox.Caps
	refusal     string
}

func useNodeTestHost(t *testing.T) *nodeTestHost {
	t.Helper()
	host := &nodeTestHost{active: "active", listening: true,
		caps: sitesandbox.Caps{SystemdVersion: 257, BPFFramework: true, CgroupV2: true, NFTSocketCgroup: true}}
	runner := useSandboxHost(t, true)
	runner.set = "set smtp_blocked {\n\telements = { \"system.slice/celikapp-7.service\" }\n}\n"
	runner.chain = "chain output {\n\ttcp dport { 25, 465, 587 } socket cgroupv2 level 2 @smtp_blocked counter name \"smtp_rejects\" reject with tcp reset\n}\n"
	host.dir = t.TempDir()
	helperSource := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(helperSource, []byte("agent binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldDir, oldCtl, oldExists, oldActive, oldVerify := appUnitDirOverride, appUnitSystemctl, appUnitAccountExists, appUnitActive, appUnitVerify
	oldLookup, oldCaps, oldRefusal, oldPIDs, oldRun, oldSettle := nodeSiteAccountLookup, nodeSiteCaps, sandboxFolderRefusal, serviceAppUnitPIDs, serviceAppRun, nodeSiteSettle
	oldSource, oldSum := sandboxHelperSource, sandboxHelperSelfSum
	t.Cleanup(func() {
		appUnitDirOverride, appUnitSystemctl, appUnitAccountExists, appUnitActive, appUnitVerify = oldDir, oldCtl, oldExists, oldActive, oldVerify
		nodeSiteAccountLookup, nodeSiteCaps, sandboxFolderRefusal, serviceAppUnitPIDs, serviceAppRun, nodeSiteSettle = oldLookup, oldCaps, oldRefusal, oldPIDs, oldRun, oldSettle
		sandboxHelperSource, sandboxHelperSelfSum = oldSource, oldSum
	})
	sandboxHelperSource, sandboxHelperSelfSum = helperSource, ""
	appUnitDirOverride = host.dir
	appUnitAccountExists = func(name string) bool { return true }
	appUnitActive = func(string) string { return transport.SiteFileServiceActiveTrue }
	appUnitVerify = func(string) error { return nil }
	nodeSiteSettle = 0
	nodeSiteAccountLookup = func(name, appDir string) (serviceAppAccount, error) {
		return serviceAppAccount{Name: name, Group: name, UID: 1500, GID: 1500, Home: filepath.Dir(appDir)}, nil
	}
	nodeSiteCaps = func(context.Context) sitesandbox.Caps { return host.caps }
	sandboxFolderRefusal = func(string, int) (string, string) {
		if host.refusal != "" {
			return host.refusal, "the reason"
		}
		return "", ""
	}
	serviceAppUnitPIDs = func(string) (map[int]bool, error) { return map[int]bool{4242: true}, nil }
	serviceAppRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		if name == "ss" && host.listening {
			return []byte("LISTEN 0 511 *:3001 *:* users:((\"node\",pid=4242,fd=20))\n"), nil
		}
		return nil, nil
	}
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.calls = append(host.calls, strings.Join(args, " "))
		if len(args) > 2 && args[0] == "show" && args[2] == "--property=ActiveState" {
			return []byte(host.active + "\n"), nil
		}
		if len(args) > 2 && args[0] == "show" && strings.HasPrefix(args[2], "--property=User,") {
			return []byte(host.show(args[1])), nil
		}
		return nil, nil
	}
	return host
}

// show is systemd's view of a sandboxed unit rendered from its profile.
func (host *nodeTestHost) show(unit string) string {
	p := nodesite.Params{SiteID: 7, User: "example_com", Group: "example_com", AppDir: nodeTestDocRoot, Port: 3001, NodeVersion: "22.11.0", Caps: host.caps}
	var b strings.Builder
	for name, e := range sitesandbox.Expected(p.Profile(), host.caps, unit) {
		value := e.Value
		if e.Contains {
			value = "{ argv[]=" + e.Value + " ; }"
		}
		if host.proofBroken && name == "NoNewPrivileges" {
			value = "no"
		}
		b.WriteString(name + "=" + value + "\n")
	}
	return b.String()
}

func (host *nodeTestHost) path() string { return filepath.Join(host.dir, "celikapp-7.service") }

func (host *nodeTestHost) called(prefix string) bool {
	host.mu.Lock()
	defer host.mu.Unlock()
	for _, c := range host.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func nodeRequest() *AppApplyRequest {
	return &AppApplyRequest{SiteID: 7, Description: "example.com", WorkDir: nodeTestDocRoot, Command: "npm start",
		Port: 3001, NodeVersion: "", RunAsUser: "example_com", Trigger: transport.SiteFileTriggerChange}
}

// earlierUnit writes the text the release before alpha.127 wrote for the
// same request: CelikPanel's own sealed text, outside the sandbox.
func earlierUnit(t *testing.T, host *nodeTestHost, req *AppApplyRequest) string {
	t.Helper()
	file := appUnitFile(req, host.dir)
	file.Trigger = transport.SiteFileTriggerCreate
	result, err := services.ApplyManagedSiteFile(file, nil, nil)
	if err != nil || result.Outcome != transport.SiteFileOutcomeWritten {
		t.Fatalf("earlier unit: %+v %v", result, err)
	}
	return unitText(t, host.path())
}

func TestNodeSiteNewSiteGetsTheSandbox(t *testing.T) {
	host := useNodeTestHost(t)
	req := nodeRequest()
	req.Trigger = transport.SiteFileTriggerCreate
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(withNode(t, req), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || resp.Sandbox == nil || resp.Sandbox.Generation != sitesandbox.Generation || resp.Sandbox.Proof == nil || !resp.Sandbox.Proof.OK {
		t.Fatalf("new site: %+v %+v", resp, resp.Sandbox)
	}
	text := unitText(t, host.path())
	if !nodesite.Sandboxed(text) || !strings.Contains(text, "BindPaths=/run/celikpanel-sandbox/celikapp-7.service/app:"+nodeTestDocRoot) {
		t.Fatalf("unit:\n%s", text)
	}
	if !host.called("reset-failed celikapp-7") || !host.called("restart celikapp-7") {
		t.Fatalf("calls %v", host.calls)
	}
}

// withNode gives the request an installed Node.js version: the Agent
// refuses a version that is not on disk.
func withNode(t *testing.T, req *AppApplyRequest) *AppApplyRequest {
	t.Helper()
	old, oldSandbox := runtimesBaseDir, nodeSiteRuntimesBase
	base := t.TempDir()
	runtimesBaseDir, nodeSiteRuntimesBase = base, base
	t.Cleanup(func() { runtimesBaseDir, nodeSiteRuntimesBase = old, oldSandbox })
	bin := filepath.Join(base, "node", "22.11.0", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o755); err != nil {
		t.Fatal(err)
	}
	req.NodeVersion = "22.11.0"
	return req
}

func TestNodeSiteEarlierUnitIsNotRewrittenSilently(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	before := earlierUnit(t, host, req)
	// A settings change of the site (same inputs) keeps the earlier text.
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || resp.Sandbox == nil || resp.Sandbox.Reason != "earlier_release" || resp.Sandbox.Generation != 0 {
		t.Fatalf("change: %+v %+v", resp, resp.Sandbox)
	}
	if after := unitText(t, host.path()); after != before {
		t.Fatalf("the earlier unit was rewritten:\n%s", after)
	}
	// The site configuration page's difference is from the earlier text too.
	var inspected transport.InspectSiteFileResponse
	if err := (&Agent{}).InspectAppUnit(req, &inspected); err != nil || inspected.File.State != transport.SiteFileManagedUnchanged || inspected.Diff != "" {
		t.Fatalf("inspect: %+v %v", inspected, err)
	}
}

func TestNodeSiteMoveIsProved(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	earlierUnit(t, host, req)
	req.SandboxMove = true
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || resp.Sandbox.PutBack || resp.Sandbox.Generation != sitesandbox.Generation || !resp.Sandbox.Proof.OK {
		t.Fatalf("move: %+v %+v", resp, resp.Sandbox)
	}
	if !nodesite.Sandboxed(unitText(t, host.path())) {
		t.Fatal("not moved")
	}
	// Once moved, an ordinary change keeps the sandbox.
	req.SandboxMove = false
	req.Command = "node server.js"
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	text := unitText(t, host.path())
	if resp.Error != "" || !nodesite.Sandboxed(text) || !strings.Contains(text, `ExecStart=/bin/sh -c "node server.js"`) {
		t.Fatalf("change after the move: %+v\n%s", resp, text)
	}
}

func TestNodeSiteRefutedMoveIsPutBack(t *testing.T) {
	for _, name := range []string{"proof", "stops answering"} {
		t.Run(name, func(t *testing.T) {
			host := useNodeTestHost(t)
			req := withNode(t, nodeRequest())
			before := earlierUnit(t, host, req)
			req.SandboxMove = true
			if name == "proof" {
				host.proofBroken = true
			} else {
				// It answered before the move; the sandboxed start fails.
				prev := appUnitSystemctl
				appUnitSystemctl = func(args ...string) ([]byte, error) {
					if len(args) > 1 && args[0] == "restart" {
						host.mu.Lock()
						host.active, host.listening = "failed", false
						host.mu.Unlock()
					}
					return prev(args...)
				}
			}
			var resp AppApplyResponse
			if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil {
				t.Fatal(err)
			}
			if !resp.Sandbox.PutBack || resp.Error == "" || resp.Sandbox.Generation != 0 {
				t.Fatalf("%s: %+v %+v", name, resp, resp.Sandbox)
			}
			if after := unitText(t, host.path()); after != before {
				t.Fatalf("%s: the earlier unit is not back:\n%s", name, after)
			}
			if resp.File == nil || resp.File.Outcome != transport.SiteFileOutcomeWritten {
				t.Fatalf("%s: the ledger must record the put-back text: %+v", name, resp.File)
			}
		})
	}
}

func TestNodeSiteStoppedApplicationStaysStopped(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	earlierUnit(t, host, req)
	req.SandboxMove, req.NoStart = true, true
	var resp AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(req, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" || !nodesite.Sandboxed(unitText(t, host.path())) {
		t.Fatalf("%+v", resp)
	}
	if host.called("restart celikapp") || host.called("enable celikapp") || host.called("start celikapp") {
		t.Fatalf("a stopped application was started: %v", host.calls)
	}
}

func TestNodeSiteOwnerEditedUnitIsKept(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	earlierUnit(t, host, req)
	edited := unitText(t, host.path()) + "Nice=5\n"
	if err := os.WriteFile(host.path(), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	req.SandboxMove = true
	var resp AppApplyResponse
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if resp.Error == "" || resp.Sandbox == nil || resp.Sandbox.Reason != "unit_edited" {
		t.Fatalf("%+v %+v", resp, resp.Sandbox)
	}
	if unitText(t, host.path()) != edited {
		t.Fatal("the owner's unit was changed")
	}
}

func TestNodeSiteSharedAccountAndFolderRefusals(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	req.RunAsUser, req.Trigger = "www-data", transport.SiteFileTriggerCreate
	var resp AppApplyResponse
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if resp.Error != "" || resp.Sandbox.Reason != "shared_account" || nodesite.Sandboxed(unitText(t, host.path())) {
		t.Fatalf("shared account: %+v %+v", resp, resp.Sandbox)
	}
	// The move of a shared-account application is refused, nothing changes.
	before := unitText(t, host.path())
	req.SandboxMove = true
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if !strings.Contains(resp.Error, "shared_account") || unitText(t, host.path()) != before {
		t.Fatalf("shared account move: %+v", resp)
	}
	// A document root the helper refuses: nothing is written.
	if err := os.Remove(host.path()); err != nil {
		t.Fatal(err)
	}
	req = withNode(t, nodeRequest())
	host.refusal = "app_folder_not_real"
	resp = AppApplyResponse{}
	_ = (&Agent{}).ApplyAppUnit(req, &resp)
	if resp.Sandbox == nil || resp.Sandbox.Reason != "app_folder_not_real" {
		t.Fatalf("folder: %+v %+v", resp, resp.Sandbox)
	}
	if _, err := os.Stat(host.path()); !os.IsNotExist(err) {
		t.Fatal("a unit was written for a refused folder")
	}
}

func TestNodeSiteUnitsAreListed(t *testing.T) {
	host := useNodeTestHost(t)
	req := withNode(t, nodeRequest())
	earlierUnit(t, host, req)
	units := nodeSiteUnits()
	if len(units) != 1 || units[0].Kind != "node" || units[0].SiteID != 7 || units[0].File != "ours" || units[0].Generation != 0 {
		t.Fatalf("%+v", units)
	}
	if generation, read := nodeSiteUnitGeneration(7); !read || generation != 0 {
		t.Fatalf("generation %d %v", generation, read)
	}
}
