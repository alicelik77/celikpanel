//go:build linux && realsystem

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// Real-system evidence for alpha.104, run only inside a disposable Debian 13
// virtual machine (build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A104=1):
// the package manager's own simulation refuses the Frankfurt removal, a
// clean removal runs exactly the simulated list, a PHP tool is moved to
// another PHP version, proved through nginx, and put back when the proof
// fails, removing a PHP version the tools run on is refused by the real
// uninstall, and webmail (Roundcube, installed for real) is moved the same
// way (review of alpha.104). Never run it on an installed server.
//
// Two phases: before PHP 8.5 exists (FrankfurtRemoval, CleanRemoval,
// WebmailInstalled), then after it is installed (ToolMove, WebmailMove).
//
// alpha.104 gerçek sistem kanıtı; yalnız atılabilir bir Debian 13 sanal
// makinesinde koşar. Kurulu bir sunucuda asla koşturulmaz.

func requireRealSystem(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A104") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.104 VM")
	}
}

func dpkgInstalledSet(t *testing.T) map[string]bool {
	t.Helper()
	out, err := exec.Command("dpkg-query", "-W", "-f", "${Package}\t${Status}\n").Output()
	if err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		name, status, ok := strings.Cut(line, "\t")
		if ok && strings.HasSuffix(status, "install ok installed") {
			set[name] = true
		}
	}
	return set
}

func logJSON(t *testing.T, label string, value any) {
	t.Helper()
	data, _ := json.MarshalIndent(value, "", "  ")
	t.Logf("%s: %s", label, data)
}

var frankfurtRealPackages = []string{
	"php8.4-fpm", "php8.4-cli", "php8.4-common", "php8.4-opcache", "php8.4-mysql",
	"php8.4-pgsql", "php8.4-sqlite3", "php8.4-mbstring", "php8.4-xml", "php8.4-zip", "php8.4-intl",
}

func TestRealSystemA104FrankfurtRemovalIsRefused(t *testing.T) {
	requireRealSystem(t)
	before := dpkgInstalledSet(t)
	if !before["phpmyadmin"] || !before["php8.4-fpm"] {
		t.Fatalf("the VM is not prepared: phpmyadmin=%v php8.4-fpm=%v", before["phpmyadmin"], before["php8.4-fpm"])
	}
	request := packageRemovalRequest{
		Packages:           frankfurtRealPackages,
		VersionPrefixes:    []string{"php8.4"},
		ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet(frankfurtRealPackages)),
	}
	beforeCalled := false
	plan, _, err := guardedRemovePackagesContext(context.Background(), "apt", request, removalHooks{stop: func() error {
		beforeCalled = true
		return nil
	}})
	logJSON(t, "plan", plan)
	var refusal *packageRemovalRefusal
	if !errors.As(err, &refusal) || refusal.Code != errCodeRemovalWouldRemoveOthers {
		t.Fatalf("err = %v", err)
	}
	t.Logf("refusal: %s", refusal.Message)
	found := false
	for _, item := range refusal.Collateral {
		if item.Package == "phpmyadmin" && item.Component == "phpmyadmin" {
			found = true
		}
	}
	if !found || beforeCalled {
		t.Fatalf("phpmyadmin named=%v beforeRemoval ran=%v", found, beforeCalled)
	}
	after := dpkgInstalledSet(t)
	if len(after) != len(before) || !after["phpmyadmin"] || !after["php8.4-fpm"] {
		t.Fatalf("the refusal changed installed packages: %d -> %d", len(before), len(after))
	}
}

func TestRealSystemA104CleanRemovalRunsTheSimulatedList(t *testing.T) {
	requireRealSystem(t)
	before := dpkgInstalledSet(t)
	if !before["redis-server"] {
		t.Fatal("the VM is not prepared: redis-server is missing")
	}
	request := packageRemovalRequest{
		Packages:           []string{"redis-server"},
		ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet([]string{"redis-server"})),
	}
	plan, _, err := guardedRemovePackagesContext(context.Background(), "apt", request, removalHooks{})
	logJSON(t, "plan", plan)
	if err != nil {
		t.Fatal(err)
	}
	after := dpkgInstalledSet(t)
	var gone []string
	for name := range before {
		if !after[name] {
			gone = append(gone, name)
		}
	}
	sort.Strings(gone)
	want := append([]string{}, plan.Remove...)
	for i := range want {
		want[i] = packageBaseName(want[i])
	}
	sort.Strings(want)
	t.Logf("removed by the real command: %v", gone)
	if strings.Join(gone, " ") != strings.Join(want, " ") {
		t.Fatalf("removed %v, simulated %v", gone, want)
	}
}

func writeRealDBToolsConf(t *testing.T, sockets map[string]string) {
	t.Helper()
	if err := ensurePHPToolCheckScript(); err != nil {
		t.Fatal(err)
	}
	tools, roots := installedDBToolRoots()
	if len(tools) != 2 {
		t.Fatalf("db tools installed = %v", tools)
	}
	if err := os.WriteFile(dbToolsConfPath, []byte(renderDBToolsConfig(tools, roots, sockets)), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		t.Fatalf("nginx -t: %s", out)
	}
	if out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput(); err != nil {
		t.Fatalf("reload: %s", out)
	}
}

func TestRealSystemA104ToolMoveIsProvedAndRolledBack(t *testing.T) {
	requireRealSystem(t)
	if !phpVersionInstalled("8.5") {
		t.Skip("PHP 8.5 is not installed in this VM; the move evidence needs a second PHP version")
	}
	saved := phpToolBindingRecordPath
	phpToolBindingRecordPath = filepath.Join(t.TempDir(), "php-tool-bindings.json")
	t.Cleanup(func() { phpToolBindingRecordPath = saved })
	ctx := context.Background()
	sock84, sock85 := versionedFPMSocketPath("8.4"), versionedFPMSocketPath("8.5")
	writeRealDBToolsConf(t, map[string]string{phpToolPHPMyAdmin: sock84, phpToolPHPPgAdmin: sock84})
	for _, tool := range []string{phpToolPHPMyAdmin, phpToolPHPPgAdmin} {
		if err := retryPHPToolProbe(ctx, tool, "8.4", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
			t.Fatalf("%s on 8.4 before the move: %v", tool, err)
		}
	}
	if got := phpToolsBoundToVersion("8.4"); !slices.Contains(got, phpToolPHPMyAdmin) || !slices.Contains(got, phpToolPHPPgAdmin) {
		t.Fatalf("bound to 8.4 = %v", got)
	}

	// 1. A verified move of phpPgAdmin to 8.5.
	result, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolPHPPgAdmin, "8.5")
	logJSON(t, "move to 8.5", result)
	if err != nil {
		t.Fatalf("move to 8.5: %v", err)
	}
	conf := parseDBToolsFPMSockets(readFileString(dbToolsConfPath))
	if conf[phpToolPHPPgAdmin] != sock85 || conf[phpToolPHPMyAdmin] != sock84 {
		t.Fatalf("configuration after the move = %v", conf)
	}
	if err := retryPHPToolProbe(ctx, phpToolPHPPgAdmin, "8.5", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
		t.Fatalf("phpPgAdmin on 8.5: %v", err)
	}

	// 2. Back to 8.4, then a move whose proof fails: PHP 8.5's FPM loses
	// pgsql while its command line keeps it, so only the page can tell.
	if _, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolPHPPgAdmin, "8.4"); err != nil {
		t.Fatalf("move back to 8.4: %v", err)
	}
	if out, err := exec.Command("phpdismod", "-v", "8.5", "-s", "fpm", "pgsql").CombinedOutput(); err != nil {
		t.Fatalf("phpdismod: %s", out)
	}
	if out, err := exec.Command("systemctl", "restart", "php8.5-fpm").CombinedOutput(); err != nil {
		t.Fatalf("restart php8.5-fpm: %s", out)
	}
	t.Cleanup(func() {
		_ = exec.Command("phpenmod", "-v", "8.5", "-s", "fpm", "pgsql").Run()
		_ = exec.Command("systemctl", "restart", "php8.5-fpm").Run()
	})
	before := readFileString(dbToolsConfPath)
	result, err = bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolPHPPgAdmin, "8.5")
	logJSON(t, "failing move", result)
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolProbeFailed || !result.Restored {
		t.Fatalf("failing move: err=%v result=%+v", err, result)
	}
	t.Logf("refusal: %v", err)
	if after := readFileString(dbToolsConfPath); after != before {
		t.Fatal("the previous configuration was not restored byte for byte")
	}
	if err := retryPHPToolProbe(ctx, phpToolPHPPgAdmin, "8.4", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
		t.Fatalf("phpPgAdmin after the rollback: %v", err)
	}

	// 3. Removing PHP 8.4 while the tools run on it is refused by the real
	// uninstall before anything changes (alpha.104 review: an actual
	// refusal, not only the binding count).
	ops, err := defaultServiceUninstallOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	installedBefore := dpkgInstalledSet(t)
	var removal UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "php-fpm", Package: "php8.4-fpm"}, &removal, ops); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "removal of PHP-FPM 8.4", removal)
	if removal.ErrorCode != errCodeRemovalToolsBound || removal.Removed || removal.MutationApplied ||
		!slices.Contains(removal.BoundTools, phpToolPHPMyAdmin) || !slices.Contains(removal.BoundTools, phpToolPHPPgAdmin) {
		t.Fatalf("removal of PHP-FPM 8.4 = %+v", removal)
	}
	installedAfter := dpkgInstalledSet(t)
	if len(installedAfter) != len(installedBefore) || !installedAfter["php8.4-fpm"] {
		t.Fatalf("the refusal changed installed packages: %d -> %d", len(installedBefore), len(installedAfter))
	}
	if out, _ := exec.Command("systemctl", "is-active", "php8.4-fpm").Output(); strings.TrimSpace(string(out)) != "active" {
		t.Fatalf("php8.4-fpm after the refusal: %s", out)
	}
}

// Webmail (Roundcube) installed for real on the only PHP (8.4), then
// configured: it is bound to 8.4 and answers as itself through its socket.
func TestRealSystemA104WebmailInstalledOnItsPHP(t *testing.T) {
	requireRealSystem(t)
	if phpVersionInstalled("8.5") {
		t.Skip("phase one runs before PHP 8.5 is installed")
	}
	ctx := context.Background()
	agent := &Agent{}
	var installed InstallRoundcubeResponse
	agent.installRoundcubeWith(ctx, &installed)
	logJSON(t, "install Roundcube", installed)
	if installed.Error != "" || !installed.Installed {
		t.Fatalf("install = %+v", installed)
	}
	var configured ConfigureWebmailResponse
	agent.configureWebmailWith(ctx, &configured)
	logJSON(t, "configure webmail", configured)
	if configured.Error != "" || !configured.Configured || !configured.Present {
		t.Fatalf("configure = %+v", configured)
	}
	if socket := parseWebmailFPMSocket(readFileString(phpToolWebmailConfPath)); socket != versionedFPMSocketPath("8.4") {
		t.Fatalf("webmail socket = %q", socket)
	}
	if err := retryPHPToolProbe(ctx, phpToolWebmail, "8.4", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
		t.Fatalf("webmail on 8.4: %v", err)
	}
}

// Webmail moved to PHP 8.5 (its extensions installed by the move), proved,
// moved back; then a move whose proof fails (intl off for 8.5's FPM only)
// is refused with a structured cause and the configuration is restored
// byte for byte.
func TestRealSystemA104WebmailMoveIsProvedAndRolledBack(t *testing.T) {
	requireRealSystem(t)
	if !phpVersionInstalled("8.5") {
		t.Skip("PHP 8.5 is not installed in this VM; the move evidence needs a second PHP version")
	}
	if present, err := roundcubeInstallState(); err != nil || !present {
		t.Fatalf("Roundcube is not installed (phase one): %v", err)
	}
	saved := phpToolBindingRecordPath
	phpToolBindingRecordPath = filepath.Join(t.TempDir(), "php-tool-bindings.json")
	t.Cleanup(func() { phpToolBindingRecordPath = saved })
	ctx := context.Background()
	sock84, sock85 := versionedFPMSocketPath("8.4"), versionedFPMSocketPath("8.5")
	if socket := parseWebmailFPMSocket(readFileString(phpToolWebmailConfPath)); socket != sock84 {
		t.Fatalf("webmail socket before the move = %q", socket)
	}
	result, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolWebmail, "8.5")
	logJSON(t, "webmail to 8.5", result)
	if err != nil {
		t.Fatalf("webmail to 8.5: %v", err)
	}
	if socket := parseWebmailFPMSocket(readFileString(phpToolWebmailConfPath)); socket != sock85 {
		t.Fatalf("webmail socket after the move = %q", socket)
	}
	if err := retryPHPToolProbe(ctx, phpToolWebmail, "8.5", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
		t.Fatalf("webmail on 8.5: %v", err)
	}
	if _, err := bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolWebmail, "8.4"); err != nil {
		t.Fatalf("webmail back to 8.4: %v", err)
	}
	if out, err := exec.Command("phpdismod", "-v", "8.5", "-s", "fpm", "intl").CombinedOutput(); err != nil {
		t.Fatalf("phpdismod: %s", out)
	}
	if out, err := exec.Command("systemctl", "restart", "php8.5-fpm").CombinedOutput(); err != nil {
		t.Fatalf("restart php8.5-fpm: %s", out)
	}
	t.Cleanup(func() {
		_ = exec.Command("phpenmod", "-v", "8.5", "-s", "fpm", "intl").Run()
		_ = exec.Command("systemctl", "restart", "php8.5-fpm").Run()
	})
	before := readFileString(phpToolWebmailConfPath)
	result, err = bindPHPToolWith(ctx, hostPHPToolBindOps(), phpToolWebmail, "8.5")
	logJSON(t, "failing webmail move", result)
	var typed *phpToolBindError
	if !errors.As(err, &typed) || typed.Code != errCodePHPToolProbeFailed || !result.Restored {
		t.Fatalf("failing move: err=%v result=%+v", err, result)
	}
	logJSON(t, "cause", typed.Cause)
	if typed.Cause.Kind != "missing_extensions" || !slices.Contains(typed.Cause.Extensions, "intl") {
		t.Fatalf("cause = %+v", typed.Cause)
	}
	t.Logf("refusal: %v", err)
	if after := readFileString(phpToolWebmailConfPath); after != before {
		t.Fatal("the previous webmail configuration was not restored byte for byte")
	}
	if err := retryPHPToolProbe(ctx, phpToolWebmail, "8.4", probePHPToolHTTP, phpToolProbeSettle, time.Second); err != nil {
		t.Fatalf("webmail after the rollback: %v", err)
	}
}
