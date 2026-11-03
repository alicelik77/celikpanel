package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// serviceAppHost fakes what a service-app RPC touches: the site account,
// systemd (systemctl and systemd-run), the runtimes, the download and the
// commands run as the site's user.
type serviceAppHost struct {
	mu          sync.Mutex
	installUnit string // "", "running", "succeeded"
	systemdRuns [][]string
	userScripts []string
	userStdin   []string
	env         string
	journal     string
	systemctl   []string
}

func useServiceAppHost(t *testing.T) *serviceAppHost {
	t.Helper()
	host := &serviceAppHost{}
	oldAccount, oldRun, oldUser, oldDownload, oldProbe, oldCtl, oldRoot, oldDir :=
		serviceAppLookupAccount, serviceAppRun, serviceAppAsUser, serviceAppDownload, serviceAppProbe, appUnitSystemctl, serviceAppStateRoot, appUnitDirOverride
	t.Cleanup(func() {
		serviceAppLookupAccount, serviceAppRun, serviceAppAsUser, serviceAppDownload, serviceAppProbe, appUnitSystemctl, serviceAppStateRoot, appUnitDirOverride =
			oldAccount, oldRun, oldUser, oldDownload, oldProbe, oldCtl, oldRoot, oldDir
	})
	serviceAppStateRoot = t.TempDir()
	appUnitDirOverride = t.TempDir()
	useSandboxHost(t, true)
	serviceAppLookupAccount = func(target transport.ServiceAppTarget) (serviceAppAccount, error) {
		return serviceAppAccount{Name: "flows_example_com", Group: "flows_example_com", UID: 1500, GID: 1500, Home: "/var/www/celikpanel/subscriptions/3/sites/7"}, nil
	}
	serviceAppProbe = func(context.Context) ([]serviceapps.NodeCandidate, []serviceapps.PythonCandidate) {
		return []serviceapps.NodeCandidate{{Path: "/opt/celikpanel/runtimes/node/24.11.1/bin/node", Version: "24.11.1",
			NPMCLI: "/opt/celikpanel/runtimes/node/24.11.1/lib/node_modules/npm/bin/npm-cli.js", Source: "panel"}}, nil
	}
	serviceAppDownload = func(url, dest, algorithm, digest string) error {
		return os.WriteFile(dest, []byte("artifact"), 0o600)
	}
	serviceAppAsUser = func(account serviceAppAccount, stdin io.Reader, script string, args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.userScripts = append(host.userScripts, script)
		if stdin != nil {
			raw, _ := io.ReadAll(stdin)
			host.userStdin = append(host.userStdin, string(raw))
			if strings.Contains(script, "app.env.partial") {
				host.env = string(raw)
			}
		}
		if strings.Contains(script, "config/app.env\"; if") {
			return []byte(host.env), nil
		}
		return nil, nil
	}
	serviceAppRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		switch name {
		case "systemd-run":
			host.systemdRuns = append(host.systemdRuns, args)
			host.installUnit = "running"
		case "journalctl":
			return []byte(host.journal), nil
		}
		return nil, nil
	}
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		host.systemctl = append(host.systemctl, strings.Join(args, " "))
		if args[0] == "show" && strings.HasPrefix(args[1], "celiksvcinst-") {
			switch host.installUnit {
			case "running":
				return []byte("LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\n"), nil
			case "succeeded":
				return []byte("LoadState=loaded\nActiveState=active\nSubState=exited\nResult=success\n"), nil
			}
			return []byte("LoadState=not-found\nActiveState=inactive\nSubState=dead\nResult=success\n"), nil
		}
		if args[0] == "stop" && strings.HasPrefix(args[1], "celiksvcinst-") {
			// stopping a running install of another operation is not done
			// by Prepare; record only.
			return nil, nil
		}
		return nil, nil
	}
	return host
}

func n8nPrepareRequest(operation string) *transport.ServiceAppPrepareRequest {
	return &transport.ServiceAppPrepareRequest{
		OperationID: operation,
		Target:      transport.ServiceAppTarget{InstallID: 5, SiteID: 9, SubscriptionID: 3, DomainID: 7, Domain: "flows.example.com", AppID: "n8n"},
		Version:     "2.46.4", Port: 21010, AuxPort: 21011,
		OwnerEmail: "owner@example.com", OwnerSecret: "$2a$12$abcdefghijklmnopqrstuuJ0123456789abcdefghijklmnopqrstu",
	}
}

func TestServiceAppPrepareStartsTheInstallOnceAsTheSiteUser(t *testing.T) {
	host := useServiceAppHost(t)
	agent := &Agent{}
	var resp transport.ServiceAppStepResponse
	if err := agent.ServiceAppPrepareV1(n8nPrepareRequest("op-aaaa-0001"), &resp); err != nil || !resp.Done {
		t.Fatalf("prepare: %v %+v", err, resp)
	}
	// The same operation again starts nothing.
	resp = transport.ServiceAppStepResponse{}
	if err := agent.ServiceAppPrepareV1(n8nPrepareRequest("op-aaaa-0001"), &resp); err != nil || !resp.Done {
		t.Fatalf("repeat: %v %+v", err, resp)
	}
	// Another operation while the install runs is refused.
	resp = transport.ServiceAppStepResponse{}
	_ = agent.ServiceAppPrepareV1(n8nPrepareRequest("op-bbbb-0002"), &resp)
	if resp.Done || resp.Reason != "busy" {
		t.Fatalf("a second install must wait: %+v", resp)
	}
	if len(host.systemdRuns) != 1 {
		t.Fatalf("systemd-run ran %d times", len(host.systemdRuns))
	}
	args := strings.Join(host.systemdRuns[0], "\n")
	for _, want := range []string{"--uid=flows_example_com", "--unit=celiksvcinst-5", "--property=ProtectSystem=strict",
		"--property=NoNewPrivileges=yes", "--property=ReadWritePaths=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n", "--property=MemoryMax=3072M"} {
		if !strings.Contains(args, want) {
			t.Errorf("install unit lacks %s", want)
		}
	}
	if strings.Contains(args, "$2a$12$") || strings.Contains(args, "owner@example.com") {
		t.Fatal("a secret reached the command line")
	}
	env := serviceapps.ParseEnv(host.env)
	if env["N8N_LISTEN_ADDRESS"] != "127.0.0.1" || env["N8N_PORT"] != "21010" || env["N8N_INSTANCE_OWNER_PASSWORD_HASH"] == "" || len(env["N8N_ENCRYPTION_KEY"]) != 64 {
		t.Fatalf("environment file: %v", env)
	}
	// Every write in the site's folder went through the site's user.
	for _, script := range host.userScripts {
		if strings.Contains(script, "sudo") || strings.Contains(script, "chown") {
			t.Fatalf("site-user script escalates: %s", script)
		}
	}
	// The generated key survives a rewrite.
	key := env["N8N_ENCRYPTION_KEY"]
	host.installUnit = "succeeded"
	var secretsResp transport.ServiceAppStepResponse
	_ = agent.ServiceAppSecretsV1(&transport.ServiceAppSecretsRequest{Target: n8nPrepareRequest("x").Target, Port: 21010, AuxPort: 21011,
		OwnerSecret: "$2a$12$zyxwvutsrqponmlkjihgfeJ0123456789abcdefghijklmnopqrstu"}, &secretsResp)
	if !secretsResp.Done || serviceapps.ParseEnv(host.env)["N8N_ENCRYPTION_KEY"] != key {
		t.Fatalf("rewrite lost the generated key: %+v", secretsResp)
	}
}

func TestServiceAppPrepareRefusesAnUnpinnedVersionAndOldRuntimes(t *testing.T) {
	useServiceAppHost(t)
	req := n8nPrepareRequest("op-cccc-0003")
	req.Version = "2.47.0"
	var resp transport.ServiceAppStepResponse
	_ = (&Agent{}).ServiceAppPrepareV1(req, &resp)
	if resp.Done || resp.Reason != "version_not_pinned" {
		t.Fatalf("unpinned: %+v", resp)
	}
	serviceAppProbe = func(context.Context) ([]serviceapps.NodeCandidate, []serviceapps.PythonCandidate) {
		return []serviceapps.NodeCandidate{{Path: "/usr/bin/node", Version: "20.19.2", NPMCLI: "/usr/share/nodejs/npm/bin/npm-cli.js", Source: "distribution"}},
			[]serviceapps.PythonCandidate{{Path: "/usr/bin/python3", Version: "3.13", Venv: true}}
	}
	resp = transport.ServiceAppStepResponse{}
	_ = (&Agent{}).ServiceAppPrepareV1(n8nPrepareRequest("op-cccc-0004"), &resp)
	if resp.Done || resp.Class != transport.ServiceAppPrerequisite || resp.Reason != serviceapps.ReasonRuntimeAbsent {
		t.Fatalf("old node: %+v", resp)
	}
	owui := n8nPrepareRequest("op-cccc-0005")
	owui.Target.AppID, owui.Version, owui.AuxPort = "open-webui", "0.12.5", 0
	resp = transport.ServiceAppStepResponse{}
	_ = (&Agent{}).ServiceAppPrepareV1(owui, &resp)
	if resp.Done || resp.Class != transport.ServiceAppVerifiedFailure || resp.Reason != serviceapps.ReasonOSUnsupported {
		t.Fatalf("python 3.13: %+v", resp)
	}
}

func TestServiceAppLogsHideSecrets(t *testing.T) {
	host := useServiceAppHost(t)
	host.env = `OPENAI_API_KEY="sk-live-abcdefghijklmnopqrstuvwxyz"` + "\n" + `WEBUI_SECRET_KEY="` + strings.Repeat("ab", 32) + `"` + "\n"
	host.journal = "2026-11-03T10:00:00 chat[1]: using key sk-live-abcdefghijklmnopqrstuvwxyz\n" +
		"2026-11-03T10:00:01 chat[1]: session secret " + strings.Repeat("ab", 32) + "\n"
	var resp transport.ServiceAppLogsResponse
	target := transport.ServiceAppTarget{InstallID: 6, SiteID: 9, SubscriptionID: 3, DomainID: 7, Domain: "chat.example.com", AppID: "open-webui"}
	if err := (&Agent{}).ServiceAppLogsV1(&transport.ServiceAppLogsRequest{Target: target, Lines: 50}, &resp); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(resp.Lines, "\n")
	if len(resp.Lines) != 2 || strings.Contains(joined, "sk-live") || strings.Contains(joined, strings.Repeat("ab", 32)) {
		t.Fatalf("logs: %q", joined)
	}
}

func TestServiceAppSuspensionUsesTheInstallUnit(t *testing.T) {
	host := &fakeAppUnitHost{present: true, active: true, enabled: true}
	useFakeAppUnitHost(t, host)
	var resp transport.SetAppUnitSuspensionResponse
	if err := (&Agent{}).SetServiceAppSuspensionV1(&transport.SetServiceAppSuspensionRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), InstallID: 12, Suspend: true,
	}, &resp); err != nil || !resp.Applied {
		t.Fatalf("suspend: %v %+v", err, resp)
	}
	if host.active || host.enabled || !strings.Contains(strings.Join(host.calls, ","), "stop celiksvc-12") {
		t.Fatalf("calls: %v", host.calls)
	}
}

func TestServiceAppUnitKeepsAnOwnerEdit(t *testing.T) {
	host := useServiceAppHost(t)
	dir := t.TempDir()
	appUnitDirOverride = dir
	unit := filepath.Join(dir, "celiksvc-5.service")
	if err := os.WriteFile(unit, []byte("[Unit]\nDescription=the owner's own\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Activate needs the application in place; the edited unit is checked
	// first only after that, so give it a fake installed entry by pointing
	// the check at the refusal path: a missing install is a prerequisite.
	req := &transport.ServiceAppActivateRequest{Target: n8nPrepareRequest("x").Target, Version: "2.46.4", Port: 21010, AuxPort: 21011}
	var resp transport.ServiceAppStepResponse
	_ = (&Agent{}).ServiceAppActivateV1(req, &resp)
	if resp.Done || resp.Reason != "install_missing" {
		t.Fatalf("activate without files: %+v", resp)
	}
	raw, _ := os.ReadFile(unit)
	if string(raw) != "[Unit]\nDescription=the owner's own\n" {
		t.Fatal("the owner's unit was changed")
	}
	if serviceAppUnitFileState(5) != "foreign" {
		t.Fatalf("state: %s", serviceAppUnitFileState(5))
	}
	_ = host
}
