package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func usePanelListenTestFiles(t *testing.T, env, unit string) string {
	t.Helper()
	dir := t.TempDir()
	restoreEnv, restoreUnit, restoreRoot := panelListenEnvPath, panelListenUnitPath, panelListenRequireRoot
	t.Cleanup(func() {
		panelListenEnvPath, panelListenUnitPath, panelListenRequireRoot = restoreEnv, restoreUnit, restoreRoot
	})
	panelListenEnvPath = filepath.Join(dir, "panel.env")
	panelListenUnitPath = filepath.Join(dir, "celikpanel-panel.service")
	panelListenRequireRoot = func(os.FileInfo, string) error { return nil }
	if env != "" {
		if err := os.WriteFile(panelListenEnvPath, []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if unit != "" {
		if err := os.WriteFile(panelListenUnitPath, []byte(unit), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The Agent reaches the panel at the configured port, never a constant.
func TestPanelCertificateLoopbackFollowsTheConfiguredPort(t *testing.T) {
	usePanelListenTestFiles(t, "CELIKPANEL_LISTEN=:2083\nCELIKPANEL_TLS=1\n", "Environment=CELIKPANEL_LISTEN=:1923\n")
	if got := panelCertificateLoopbackAddress(); got != "127.0.0.1:2083" {
		t.Fatalf("an installation kept on 2083: %q", got)
	}
	usePanelListenTestFiles(t, "CELIKPANEL_LISTEN=[::]:1923\n", "")
	if got := panelCertificateLoopbackAddress(); got != "[::1]:1923" {
		t.Fatalf("IPv6 wildcard: %q", got)
	}
	usePanelListenTestFiles(t, "", "Environment=CELIKPANEL_LISTEN=:2083\n")
	if got := panelCertificateLoopbackAddress(); got != "127.0.0.1:2083" {
		t.Fatalf("unit fallback: %q", got)
	}
	usePanelListenTestFiles(t, "", "")
	if got := panelCertificateLoopbackAddress(); got != "127.0.0.1:1923" {
		t.Fatalf("default: %q", got)
	}
}

func TestSetPanelListenChangesOnlyTheListenLine(t *testing.T) {
	original := "# kept\nCELIKPANEL_LISTEN=:2083\nCELIKPANEL_TLS=1\nCELIKPANEL_PANEL_INSECURE_COOKIES_FLAG=\nCELIKPANEL_PANEL_DEMO_FLAG=\n"
	dir := usePanelListenTestFiles(t, original, "")
	config := readPanelListenConfig()
	if !config.Present || config.Listen != ":2083" || len(config.SHA256) != 64 || config.Error != "" {
		t.Fatalf("config: %+v", config)
	}
	// A file changed after the panel looked is never overwritten.
	var resp transport.PanelListenSetResponse
	setPanelListen(&transport.PanelListenSetRequest{From: ":2083", To: ":1923", ExpectedSHA256: strings.Repeat("0", 64)}, &resp)
	if resp.Changed || resp.ErrorCode != "CONFIG_CHANGED" {
		t.Fatalf("stale digest: %+v", resp)
	}
	resp = transport.PanelListenSetResponse{}
	setPanelListen(&transport.PanelListenSetRequest{From: ":2083", To: ":1923", ExpectedSHA256: config.SHA256}, &resp)
	if !resp.Changed {
		t.Fatalf("set: %+v", resp)
	}
	raw, _ := os.ReadFile(panelListenEnvPath)
	if string(raw) != strings.Replace(original, ":2083", ":1923", 1) {
		t.Fatalf("file: %q", raw)
	}
	info, _ := os.Stat(panelListenEnvPath)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".panel.env.port-") {
			t.Fatal("temporary file left behind")
		}
	}
	// Repeating the same request after a lost answer is the same success.
	resp = transport.PanelListenSetResponse{}
	setPanelListen(&transport.PanelListenSetRequest{From: ":2083", To: ":1923", ExpectedSHA256: config.SHA256}, &resp)
	if !resp.Changed {
		t.Fatalf("repeat: %+v", resp)
	}
	// A file that is not root-only is refused.
	if err := os.Chmod(panelListenEnvPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if config := readPanelListenConfig(); config.Error == "" {
		t.Fatal("a world-readable panel.env must be reported")
	}
}

func TestPanelListenStepIsBoundToItsJob(t *testing.T) {
	job := mutationPolicyJob("panel_port_change", panelListenMutationTarget, "1923")
	if !serviceMutationStepAllowed(job, mutationPolicyClaim(serviceMutationStepSetPanelListen, panelListenMutationTarget, "1923", "port")) {
		t.Fatal("the panel port job authorizes its step")
	}
	if serviceMutationStepAllowed(job, mutationPolicyClaim(serviceMutationStepSetPanelListen, panelListenMutationTarget, "4000", "port")) {
		t.Fatal("another port is not authorized")
	}
	if serviceMutationStepAllowed(mutationPolicyJob("pg_cluster_port", panelListenMutationTarget, "1923"), mutationPolicyClaim(serviceMutationStepSetPanelListen, panelListenMutationTarget, "1923", "port")) {
		t.Fatal("another job kind is not authorized")
	}
}
