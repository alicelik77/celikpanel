package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// runtimeAppHost fakes what a runtime-app RPC touches: the site account,
// systemctl, the entry facts and the programs run as the site's account.
type runtimeAppHost struct {
	mu        sync.Mutex
	facts     siteruntime.FileFacts
	factsErr  error
	asUser    [][]string
	systemctl []string
	show      string
	journal   string
}

func elfHeader() []byte {
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	h[18] = byte(runtimeAppHostArch)
	h[19] = byte(runtimeAppHostArch >> 8)
	return h
}

func useRuntimeAppHost(t *testing.T) *runtimeAppHost {
	t.Helper()
	host := &runtimeAppHost{
		facts: siteruntime.FileFacts{Regular: true, UID: 1500, Mode: 0o750, Header: elfHeader(), FolderUID: 1500},
		show:  "LoadState=loaded\nActiveState=active\nSubState=running\nUnitFileState=enabled\nResult=success\nNRestarts=0\n",
	}
	oldAccount, oldCtl, oldDir, oldRoot, oldEnv, oldFacts, oldUser, oldRun, oldHealth, oldCgroup :=
		serviceAppLookupAccount, appUnitSystemctl, appUnitDirOverride, runtimeAppStateRoot, runtimeAppEnvDir,
		runtimeAppEntryFacts, runtimeAppAsUser, serviceAppRun, runtimeAppHealthGet, runtimeAppCgroupRoot
	t.Cleanup(func() {
		serviceAppLookupAccount, appUnitSystemctl, appUnitDirOverride, runtimeAppStateRoot, runtimeAppEnvDir,
			runtimeAppEntryFacts, runtimeAppAsUser, serviceAppRun, runtimeAppHealthGet, runtimeAppCgroupRoot =
			oldAccount, oldCtl, oldDir, oldRoot, oldEnv, oldFacts, oldUser, oldRun, oldHealth, oldCgroup
	})
	useSandboxHost(t, true)
	appUnitDirOverride = t.TempDir()
	runtimeAppStateRoot = t.TempDir()
	runtimeAppEnvDir = filepath.Join(t.TempDir(), "celikpanel-runtime")
	runtimeAppCgroupRoot = t.TempDir()
	serviceAppLookupAccount = func(target transport.ServiceAppTarget) (serviceAppAccount, error) {
		return serviceAppAccount{Name: "shop_example_com", Group: "shop_example_com", UID: 1500, GID: 1500, Home: "/var/www/celikpanel/subscriptions/3/sites/7"}, nil
	}
	runtimeAppEntryFacts = func(root, relative string) (siteruntime.FileFacts, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.facts, host.factsErr
	}
	runtimeAppAsUser = func(account serviceAppAccount, program string, args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.asUser = append(host.asUser, append([]string{program}, args...))
		return nil, nil
	}
	serviceAppRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "journalctl" {
			return []byte(host.journal), nil
		}
		return nil, nil
	}
	runtimeAppHealthGet = func(context.Context, int, string, string) transport.RuntimeAppHealth {
		return transport.RuntimeAppHealth{Checked: true, Status: 200, OK: true}
	}
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.systemctl = append(host.systemctl, strings.Join(args, " "))
		if len(args) > 0 && args[0] == "show" {
			return []byte(host.show), nil
		}
		// systemd unloads a stopped transient unit (a build): the next
		// read finds none (clearTransientUnit waits for that).
		if len(args) > 1 && args[0] == "stop" && strings.HasPrefix(args[1], "celikrtbuild-") {
			host.show = "LoadState=not-found\nActiveState=inactive\n"
		}
		return nil, nil
	}
	return host
}

var runtimeTarget = transport.RuntimeAppTarget{AppID: 12, Kind: "program", SiteID: 4, SubscriptionID: 3, DomainID: 7, Domain: "shop.example.com"}

func runtimeSpec() transport.RuntimeAppSpec {
	return transport.RuntimeAppSpec{Entry: "server", Args: []string{"-port", "{port}"}, Port: 30010,
		Limits: transport.RuntimeAppLimits{MemoryMB: 512, CPUPercent: 100, TasksMax: 256}, Level: "full"}
}

func TestRuntimeAppCheckRefusesBadEntriesWithTheirCodes(t *testing.T) {
	host := useRuntimeAppHost(t)
	var resp transport.RuntimeAppCheckResponse
	if err := (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: runtimeTarget, Spec: runtimeSpec()}, &resp); err != nil || !resp.Done || resp.Entry.Format != "elf" {
		t.Fatalf("good entry: %+v %v", resp, err)
	}
	cases := []struct {
		edit func()
		want string
	}{
		{func() { host.factsErr = os.ErrNotExist }, siteruntime.EntryMissing},
		{func() { host.factsErr = nil; host.facts.Link = true }, siteruntime.EntryLink},
		{func() { host.facts.Link = false; host.facts.Mode = 0o640 }, siteruntime.EntryNotExecutable},
		{func() { host.facts.Mode = 0o750; host.facts.UID = 0 }, siteruntime.EntryNotOwned},
		{func() { host.facts.UID = 1500; host.facts.Header = []byte("#!/bin/sh\n") }, siteruntime.EntryInterpreter},
	}
	for _, c := range cases {
		c.edit()
		resp = transport.RuntimeAppCheckResponse{}
		_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: runtimeTarget, Spec: runtimeSpec()}, &resp)
		if resp.Done || resp.Reason != c.want || resp.Class != transport.RuntimeAppPrerequisite || resp.Error == "" {
			t.Fatalf("want %s, got %+v", c.want, resp)
		}
	}
	host.factsErr = errors.New("permission denied")
	resp = transport.RuntimeAppCheckResponse{}
	_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: runtimeTarget, Spec: runtimeSpec()}, &resp)
	if resp.Class != transport.RuntimeAppUnknown {
		t.Fatalf("an unreadable entry must be an unknown result: %+v", resp)
	}
	bad := runtimeSpec()
	bad.Entry = "../../../../bin/sh"
	resp = transport.RuntimeAppCheckResponse{}
	_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: runtimeTarget, Spec: bad}, &resp)
	if resp.Reason != siteruntime.EntryInvalid {
		t.Fatalf("escaping entry: %+v", resp)
	}
}

func TestRuntimeAppRenderWritesTheUnitWithoutStartingIt(t *testing.T) {
	host := useRuntimeAppHost(t)
	var resp transport.RuntimeAppRenderResponse
	req := &transport.RuntimeAppRenderRequest{OperationID: "op-runtime-1", Target: runtimeTarget, Spec: runtimeSpec()}
	if err := (&Agent{}).RuntimeAppRenderV1(req, &resp); err != nil || !resp.Done {
		t.Fatalf("render: %+v %v", resp, err)
	}
	path := filepath.Join(appUnitDirOverride, "celikrt-12.service")
	raw, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(raw), "# celikpanel-render v2 sha256=") ||
		!strings.Contains(string(raw), "ExecStart=/var/www/celikpanel/subscriptions/3/sites/7/app/bin/server -port 30010\n") {
		t.Fatalf("unit: %v\n%s", err, raw)
	}
	for _, c := range host.systemctl {
		if strings.HasPrefix(c, "restart") || strings.HasPrefix(c, "start") {
			t.Fatalf("render started the unit: %v", host.systemctl)
		}
	}
	if len(host.asUser) != 1 || host.asUser[0][0] != "/bin/mkdir" {
		t.Fatalf("folders not made as the site's account: %v", host.asUser)
	}
	if state := runtimeAppUnitFileState(12); state != transport.SiteFileManagedUnchanged {
		t.Fatalf("state %s", state)
	}
	// The same render again changes nothing.
	resp = transport.RuntimeAppRenderResponse{}
	if err := (&Agent{}).RuntimeAppRenderV1(req, &resp); err != nil || !resp.Done || resp.UnitFile != transport.SiteFileOutcomeUnchanged {
		t.Fatalf("second render: %+v", resp)
	}
	// The owner's change is kept; the deploy stops with the reason.
	if err := os.WriteFile(path, append(raw, []byte("Environment=EXTRA=1\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = transport.RuntimeAppRenderResponse{}
	_ = (&Agent{}).RuntimeAppRenderV1(req, &resp)
	if resp.Done || resp.Reason != "unit_edited" || resp.PendingPath == "" {
		t.Fatalf("owner edit: %+v", resp)
	}
	now, _ := os.ReadFile(path)
	if !strings.Contains(string(now), "EXTRA=1") {
		t.Fatal("the owner's unit was overwritten")
	}
}

func TestRuntimeAppRenderRefusesAReducedHostWithoutAcceptedGaps(t *testing.T) {
	host := useRuntimeAppHost(t)
	_ = host
	runner := sandboxRunner.(*fakeSandboxRunner)
	runner.mu.Lock()
	runner.nftOK = false
	runner.mu.Unlock()
	resetSandboxCapsCache()
	req := &transport.RuntimeAppRenderRequest{OperationID: "op-runtime-2", Target: runtimeTarget, Spec: runtimeSpec()}
	var resp transport.RuntimeAppRenderResponse
	_ = (&Agent{}).RuntimeAppRenderV1(req, &resp)
	if resp.Done || resp.Reason != "sandbox_reduced" || !strings.Contains(resp.Error, "nft_cgroup_missing") {
		t.Fatalf("reduced host passed: %+v", resp)
	}
	// Reduced, but a different gap accepted: still refused.
	req.Spec.Level, req.Spec.AcceptedGaps = "reduced", []string{"bpf_missing"}
	resp = transport.RuntimeAppRenderResponse{}
	_ = (&Agent{}).RuntimeAppRenderV1(req, &resp)
	if resp.Done || resp.Reason != "sandbox_reduced" {
		t.Fatalf("an unaccepted gap passed: %+v", resp)
	}
	req.Spec.AcceptedGaps = []string{"nft_cgroup_missing"}
	resp = transport.RuntimeAppRenderResponse{}
	_ = (&Agent{}).RuntimeAppRenderV1(req, &resp)
	if !resp.Done {
		t.Fatalf("accepted gap refused: %+v", resp)
	}
	raw, _ := os.ReadFile(filepath.Join(appUnitDirOverride, "celikrt-12.service"))
	if strings.Contains(string(raw), "nft add element") {
		t.Fatal("the mail block lines were written on a host that cannot load them")
	}
}

func TestRuntimeAppEnvIsRootsAndSecretsStayOnTheServer(t *testing.T) {
	host := useRuntimeAppHost(t)
	previous := []string{}
	set := func(entries []transport.RuntimeAppEnvEntry) transport.RuntimeAppStepResponse {
		var resp transport.RuntimeAppStepResponse
		_ = (&Agent{}).RuntimeAppEnvV1(&transport.RuntimeAppEnvRequest{Target: runtimeTarget, Entries: entries, Restart: true, PreviousKeys: previous}, &resp)
		if resp.Done {
			previous = previous[:0]
			for _, e := range entries {
				previous = append(previous, e.Key)
			}
		}
		return resp
	}
	if resp := set([]transport.RuntimeAppEnvEntry{{Key: "LOG_LEVEL", Value: "info"}, {Key: "API_TOKEN", Value: "tok-123456789", Secret: true}}); !resp.Done {
		t.Fatalf("%+v", resp)
	}
	path := runtimeAppEnvPath(12)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if dir, _ := os.Stat(runtimeAppEnvDir); dir.Mode().Perm() != 0o700 {
		t.Fatalf("folder mode %v", dir.Mode())
	}
	// The secret is kept when only its key comes back.
	if resp := set([]transport.RuntimeAppEnvEntry{{Key: "LOG_LEVEL", Value: "debug"}, {Key: "API_TOKEN", Secret: true}}); !resp.Done {
		t.Fatalf("%+v", resp)
	}
	raw, _ := os.ReadFile(path)
	env := siteruntime.ParseEnv(string(raw))
	if env["API_TOKEN"] != "tok-123456789" || env["LOG_LEVEL"] != "debug" {
		t.Fatalf("env %v", env)
	}
	if !strings.Contains(strings.Join(host.systemctl, "\n"), "try-restart celikrt-12.service") {
		t.Fatalf("not restarted: %v", host.systemctl)
	}
	for _, refused := range []string{"LD_PRELOAD", "PATH", "PORT", "HOST", "HOME", "PYTHONPATH"} {
		if resp := set([]transport.RuntimeAppEnvEntry{{Key: refused, Value: "x"}}); resp.Done || resp.Reason != "env_values" {
			t.Fatalf("%s accepted: %+v", refused, resp)
		}
	}
	// A key the owner added to the file by hand is never dropped silently.
	if err := os.WriteFile(path, append(raw, []byte("OWNER_FLAG=\"1\"\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if resp := set([]transport.RuntimeAppEnvEntry{{Key: "LOG_LEVEL", Value: "warn"}, {Key: "API_TOKEN", Secret: true}}); resp.Done || resp.Reason != "env_edited" || !strings.Contains(resp.Error, "OWNER_FLAG") {
		t.Fatalf("owner edit overwritten: %+v", resp)
	}
	if resp := set([]transport.RuntimeAppEnvEntry{{Key: "LOG_LEVEL", Value: "warn"}, {Key: "API_TOKEN", Secret: true}, {Key: "OWNER_FLAG", Value: "1"}}); !resp.Done {
		t.Fatalf("naming the owner's key: %+v", resp)
	}
	// A secret too short to be hidden in a log is refused.
	if resp := set([]transport.RuntimeAppEnvEntry{{Key: "PIN", Value: "1234", Secret: true}}); resp.Done || resp.Reason != "env_values" {
		t.Fatalf("short secret: %+v", resp)
	}
	// Logs hide the secret.
	host.journal = "2026-11-03T10:00:00 app[1]: using token tok-123456789 now\n"
	var logs transport.RuntimeAppLogsResponse
	_ = (&Agent{}).RuntimeAppLogsV1(&transport.RuntimeAppLogsRequest{Target: runtimeTarget, Lines: 10, SecretKeys: []string{"API_TOKEN"}}, &logs)
	if len(logs.Lines) != 1 || strings.Contains(logs.Lines[0], "tok-123456789") {
		t.Fatalf("logs %v", logs.Lines)
	}
}

func TestRuntimeAppStatusReadsWithoutChangingAnything(t *testing.T) {
	host := useRuntimeAppHost(t)
	host.show = "LoadState=loaded\nActiveState=failed\nSubState=failed\nUnitFileState=enabled\nResult=start-limit-hit\nNRestarts=5\n"
	cg := filepath.Join(runtimeAppCgroupRoot, "celikrt-12.service")
	_ = os.MkdirAll(cg, 0o755)
	_ = os.WriteFile(filepath.Join(cg, "memory.events"), []byte("low 0\nhigh 0\nmax 4\noom 2\noom_kill 2\n"), 0o644)
	var resp transport.RuntimeAppStatusResponse
	if err := (&Agent{}).RuntimeAppStatusV1(&transport.RuntimeAppStatusRequest{Target: runtimeTarget, Spec: runtimeSpec(), Entry: true}, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.StartLimitHit || resp.Restarts != 5 || resp.OOMKills != 2 || resp.UnitFile != transport.SiteFileAbsent || resp.Entry == nil || resp.Entry.Code != "ok" {
		t.Fatalf("%+v", resp)
	}
	for _, c := range host.systemctl {
		if !strings.HasPrefix(c, "show") {
			t.Fatalf("a status read ran %q", c)
		}
	}
	if len(host.asUser) != 0 {
		t.Fatal("a status read ran a program as the site's account")
	}
}

func TestRuntimeAppControlAndRemove(t *testing.T) {
	host := useRuntimeAppHost(t)
	var step transport.RuntimeAppStepResponse
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: runtimeTarget, Action: "start"}, &step)
	if step.Done || step.Reason != "unit_missing" {
		t.Fatalf("start without a unit: %+v", step)
	}
	// A stop of an app that was never deployed: systemd does not know the
	// unit, nothing runs, the stop is done without "systemctl stop" (whose
	// "Unit … not loaded" never reaches the screen).
	host.show = "LoadState=not-found\nActiveState=inactive\n"
	host.systemctl = nil
	step = transport.RuntimeAppStepResponse{}
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: runtimeTarget, Action: "stop"}, &step)
	if !step.Done || step.Reason != "" || step.Error != "" || strings.Contains(strings.Join(host.systemctl, "\n"), "stop celikrt-12") {
		t.Fatalf("stop of an unknown unit: %+v %v", step, host.systemctl)
	}
	host.show = ""
	var render transport.RuntimeAppRenderResponse
	_ = (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "op-runtime-3", Target: runtimeTarget, Spec: runtimeSpec()}, &render)
	step = transport.RuntimeAppStepResponse{}
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: runtimeTarget, Action: "start"}, &step)
	if !step.Done || !strings.Contains(strings.Join(host.systemctl, "\n"), "restart celikrt-12.service") {
		t.Fatalf("start: %+v %v", step, host.systemctl)
	}
	var env transport.RuntimeAppStepResponse
	_ = (&Agent{}).RuntimeAppEnvV1(&transport.RuntimeAppEnvRequest{Target: runtimeTarget, Entries: []transport.RuntimeAppEnvEntry{{Key: "A", Value: "b"}}}, &env)
	host.show = "LoadState=not-found\nActiveState=inactive\nUnitFileState=\n"
	var removed transport.RuntimeAppRemoveResponse
	_ = (&Agent{}).RuntimeAppRemoveV1(&transport.RuntimeAppRemoveRequest{OperationID: "op-runtime-4", Target: runtimeTarget}, &removed)
	if !removed.Done {
		t.Fatalf("remove: %+v", removed)
	}
	if _, err := os.Stat(filepath.Join(appUnitDirOverride, "celikrt-12.service")); !os.IsNotExist(err) {
		t.Fatal("unit left")
	}
	if _, err := os.Stat(runtimeAppEnvPath(12)); !os.IsNotExist(err) {
		t.Fatal("environment file left")
	}
	for _, call := range host.asUser {
		if call[0] == "/bin/rm" {
			t.Fatal("the app folder was removed without being asked")
		}
	}
	removed = transport.RuntimeAppRemoveResponse{}
	_ = (&Agent{}).RuntimeAppRemoveV1(&transport.RuntimeAppRemoveRequest{OperationID: "op-runtime-5", Target: runtimeTarget, RemoveFiles: true}, &removed)
	last := host.asUser[len(host.asUser)-1]
	if !removed.Done || strings.Join(last, " ") != "/bin/rm -rf -- /var/www/celikpanel/subscriptions/3/sites/7/app" {
		t.Fatalf("remove files: %+v %v", removed, last)
	}
}
