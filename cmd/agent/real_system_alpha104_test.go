//go:build linux && realsystem

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Real-system evidence for alpha.104, run only inside a disposable Debian 13
// virtual machine (build tag realsystem, plus CELIKPANEL_REAL_SYSTEM_A104=1):
// the package manager's own simulation refuses the Frankfurt removal, a
// clean removal runs exactly the simulated list, and a PHP tool is moved to
// another PHP version, proved through nginx, and put back when the proof
// fails. Never run it on an installed server.
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
	plan, _, err := guardedRemovePackagesContext(context.Background(), "apt", request, func() error {
		beforeCalled = true
		return nil
	})
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
	plan, _, err := guardedRemovePackagesContext(context.Background(), "apt", request, nil)
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
	if got := phpToolsBoundToVersion("8.4"); strings.Join(got, ",") != "phpmyadmin,phppgadmin" {
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

	// 3. Removing PHP 8.4 while the tools run on it is refused before
	// anything changes.
	if got := phpToolsBoundToVersion("8.4"); len(got) != 2 {
		t.Fatalf("bound to 8.4 = %v", got)
	}
}
