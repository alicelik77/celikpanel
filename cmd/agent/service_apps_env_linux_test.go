//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The alpha.118 security fix: systemd reads a unit's EnvironmentFile= as
// root and follows links, so the file must not be one the site's user can
// replace. These tests prove root's file is used, links are refused, an
// owner's edit is kept and an earlier build's site-folder file is moved.

func prepareN8N(t *testing.T) *serviceAppHost {
	t.Helper()
	host := useServiceAppHost(t)
	var resp transport.ServiceAppStepResponse
	if err := (&Agent{}).ServiceAppPrepareV1(n8nPrepareRequest("op-env-0001"), &resp); err != nil || !resp.Done {
		t.Fatalf("prepare: %v %+v", err, resp)
	}
	host.installUnit = "succeeded"
	return host
}

func n8nSecrets(t *testing.T) transport.ServiceAppStepResponse {
	t.Helper()
	var resp transport.ServiceAppStepResponse
	_ = (&Agent{}).ServiceAppSecretsV1(&transport.ServiceAppSecretsRequest{Target: n8nPrepareRequest("x").Target, Port: 21010, AuxPort: 21011,
		OwnerSecret: "$2a$12$zyxwvutsrqponmlkjihgfeJ0123456789abcdefghijklmnopqrstu"}, &resp)
	return resp
}

func TestServiceAppEnvFileIsRootsOutsideTheSite(t *testing.T) {
	host := prepareN8N(t)
	path := filepath.Join(host.envDir, "celiksvc-5.env")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("root's environment file: %v %v", info, err)
	}
	dir, err := os.Lstat(host.envDir)
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("environment folder: %v %v", dir, err)
	}
	if serviceapps.UnitTextState(host.rootEnv(5)) != "ours" {
		t.Fatal("the environment file must carry CelikPanel's header")
	}
	// Nothing secret went to a process of the site's user.
	for _, stdin := range host.userStdin {
		if strings.Contains(stdin, "N8N_ENCRYPTION_KEY") || strings.Contains(stdin, "$2a$12$") {
			t.Fatalf("the environment reached the site's user: %q", stdin)
		}
	}
}

func TestServiceAppEnvWriteRefusesLinks(t *testing.T) {
	host := prepareN8N(t)
	path := filepath.Join(host.envDir, "celiksvc-5.env")
	secret := filepath.Join(t.TempDir(), "root-only.cnf")
	if err := os.WriteFile(secret, []byte("PANEL_SECRET=do-not-leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The file itself is a link: never read, never written through.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, path); err != nil {
		t.Fatal(err)
	}
	if resp := n8nSecrets(t); resp.Done {
		t.Fatalf("a write through a link must be refused: %+v", resp)
	}
	if raw, _ := os.ReadFile(secret); string(raw) != "PANEL_SECRET=do-not-leak\n" {
		t.Fatalf("the link's target was changed: %q", raw)
	}
	var logs transport.ServiceAppLogsResponse
	host.journal = "2026-11-04T10:00:00 n8n[1]: started\n"
	_ = (&Agent{}).ServiceAppLogsV1(&transport.ServiceAppLogsRequest{Target: n8nPrepareRequest("x").Target}, &logs)
	if strings.Contains(strings.Join(logs.Lines, "\n"), "do-not-leak") {
		t.Fatal("a link's target was read")
	}
	// The folder is a link: refused too.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	moved := host.envDir + ".real"
	if err := os.Rename(host.envDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, host.envDir); err != nil {
		t.Fatal(err)
	}
	if resp := n8nSecrets(t); resp.Done {
		t.Fatalf("a linked environment folder must be refused: %+v", resp)
	}
	if entries, _ := os.ReadDir(moved); len(entries) != 0 {
		t.Fatalf("something was written through the linked folder: %v", entries)
	}
}

func TestServiceAppEnvOwnerEditIsKept(t *testing.T) {
	host := prepareN8N(t)
	path := filepath.Join(host.envDir, "celiksvc-5.env")
	edited := host.rootEnv(5) + "OWNER_SETTING=\"mine\"\n"
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := n8nSecrets(t)
	if resp.Done || resp.Reason != "env_edited" || resp.Class != transport.ServiceAppPrerequisite || !strings.Contains(resp.Error, ".service.d") {
		t.Fatalf("owner's edit: %+v", resp)
	}
	if host.rootEnv(5) != edited {
		t.Fatal("the owner's edit was overwritten")
	}
	// Removal keeps a copy of the edited file and deletes the file.
	var removed transport.ServiceAppRemoveResponse
	_ = (&Agent{}).ServiceAppRemoveV1(&transport.ServiceAppRemoveRequest{Target: n8nPrepareRequest("x").Target}, &removed)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("the environment file must be removed: %v", err)
	}
	if removed.KeptEnvCopy == "" {
		t.Fatalf("no copy of the edited file: %+v", removed)
	}
	if raw, err := os.ReadFile(removed.KeptEnvCopy); err != nil || string(raw) != edited {
		t.Fatalf("copy: %q %v", raw, err)
	}
}

func TestServiceAppRemoveDeletesTheEnvFile(t *testing.T) {
	host := prepareN8N(t)
	var removed transport.ServiceAppRemoveResponse
	_ = (&Agent{}).ServiceAppRemoveV1(&transport.ServiceAppRemoveRequest{Target: n8nPrepareRequest("x").Target}, &removed)
	if host.rootEnv(5) != "" || removed.KeptEnvCopy != "" {
		t.Fatalf("remove: %+v", removed)
	}
}

func TestServiceAppLegacyEnvReadScriptNamesALink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("ROOT_ONLY=leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "config", "app.env")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", serviceAppLegacyEnvReadScript, "sh", root).Output()
	if err != nil || strings.Contains(string(out), "leak") || strings.TrimSpace(string(out)) != "celikpanel-env-link" {
		t.Fatalf("a link must be named, not read: %q %v", out, err)
	}
	if err := os.Remove(filepath.Join(root, "config", "app.env")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "app.env"), []byte("A=\"1\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/bin/sh", "-c", serviceAppLegacyEnvReadScript, "sh", root).Output(); err != nil || string(out) != "A=\"1\"\n" {
		t.Fatalf("a regular file is read: %q %v", out, err)
	}
}

const legacyEnvLine = "EnvironmentFile=/var/www/celikpanel/subscriptions/3/sites/7/apps/n8n/config/app.env\n"

func writeLegacyUnit(t *testing.T, sealed bool) (string, string) {
	t.Helper()
	body := "[Service]\nType=simple\nUser=flows_example_com\nGroup=flows_example_com\n" + legacyEnvLine + "ExecStart=/bin/true\n"
	text := serviceapps.Seal(body)
	if !sealed {
		text = serviceapps.Seal(body) + "# the owner's line\n"
	}
	path := filepath.Join(appUnitDirOverride, "celiksvc-5.service")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, text
}

func TestServiceAppMigrationMovesTheSiteFolderEnvFile(t *testing.T) {
	host := useServiceAppHost(t)
	key := strings.Repeat("ab", 32)
	host.env = "N8N_ENCRYPTION_KEY=\"" + key + "\"\nN8N_PORT=\"21010\"\n"
	host.appActive = true
	path, _ := writeLegacyUnit(t, true)
	migrateServiceAppEnvFiles()

	raw, _ := os.ReadFile(path)
	unit := string(raw)
	if !strings.Contains(unit, "EnvironmentFile="+serviceapps.EnvFile(5)+"\n") || strings.Contains(unit, "/config/app.env") ||
		serviceapps.UnitTextState(unit) != "ours" {
		t.Fatalf("unit after the move:\n%s", unit)
	}
	if serviceapps.EnvFile(5) != "/etc/celikpanel-svcapps/celiksvc-5.env" {
		t.Fatalf("root path: %s", serviceapps.EnvFile(5))
	}
	if env := serviceapps.ParseEnv(host.rootEnv(5)); env["N8N_ENCRYPTION_KEY"] != key {
		t.Fatalf("values not moved: %v", env)
	}
	if host.env != "" {
		t.Fatal("the site-folder file must be removed after the move")
	}
	calls := strings.Join(host.systemctl, ",")
	if !strings.Contains(calls, "daemon-reload") || !strings.Contains(calls, "restart celiksvc-5") {
		t.Fatalf("a running app must be restarted on root's file: %s", calls)
	}
	// Again: nothing to do, nothing restarted twice.
	host.systemctl = nil
	migrateServiceAppEnvFiles()
	if len(host.systemctl) != 0 {
		t.Fatalf("a second start changed something: %v", host.systemctl)
	}
}

func TestServiceAppMigrationKeepsAStoppedAppStopped(t *testing.T) {
	host := useServiceAppHost(t)
	host.env = "N8N_ENCRYPTION_KEY=\"" + strings.Repeat("cd", 32) + "\"\n"
	writeLegacyUnit(t, true)
	migrateServiceAppEnvFiles()
	calls := strings.Join(host.systemctl, ",")
	if strings.Contains(calls, "restart") || strings.Contains(calls, "start celiksvc-5") {
		t.Fatalf("a stopped app was started: %s", calls)
	}
	if host.rootEnv(5) == "" {
		t.Fatal("values not moved")
	}
}

func TestServiceAppMigrationRefusesALinkedSiteFolderFile(t *testing.T) {
	host := useServiceAppHost(t)
	host.legacyLink = true
	host.appActive = true
	path, _ := writeLegacyUnit(t, true)
	migrateServiceAppEnvFiles()
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "/config/app.env") || !strings.Contains(string(raw), serviceapps.EnvFile(5)) {
		t.Fatalf("the unit must stop naming the site-folder file:\n%s", raw)
	}
	if host.rootEnv(5) != "" {
		t.Fatal("nothing may be taken over from a link")
	}
	calls := strings.Join(host.systemctl, ",")
	if !strings.Contains(calls, "stop celiksvc-5") || strings.Contains(calls, "restart") {
		t.Fatalf("the app must be stopped: %s", calls)
	}
}

func TestServiceAppMigrationKeepsAnOwnerEditedUnit(t *testing.T) {
	host := useServiceAppHost(t)
	host.env = "N8N_PORT=\"21010\"\n"
	path, text := writeLegacyUnit(t, false)
	migrateServiceAppEnvFiles()
	if raw, _ := os.ReadFile(path); string(raw) != text {
		t.Fatal("an owner-edited unit was rewritten")
	}
	if len(host.systemctl) != 0 {
		t.Fatalf("an owner-edited unit's app was touched: %v", host.systemctl)
	}
}
