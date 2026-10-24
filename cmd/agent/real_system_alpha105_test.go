//go:build linux && realsystem

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Real-system evidence for alpha.105, run only inside a disposable Debian 13
// virtual machine booted from the base image (nginx, the vendor PHP
// repository configured, no PHP; build tag realsystem, plus
// CELIKPANEL_REAL_SYSTEM_A105=1). It walks the PostgreSQL + phpPgAdmin bundle
// with PHP 8.3 chosen from the vendor repository: the bundle's simulation
// names the notable packages before anything changes, an unconfirmed install
// is refused with nothing installed, the confirmed install runs through the
// Agent's own install code, phpPgAdmin is configured and proved on 8.3; then,
// with PHP 8.4 installed and pgsql disabled for 8.3's FPM, a proof without
// fallback leaves it Not ready, and a first-install proof moves it to 8.4.
// Never run it on an installed server.
//
// alpha.105 gerçek sistem kanıtı; yalnız atılabilir bir Debian 13 sanal
// makinesinde koşar. Kurulu bir sunucuda asla koşturulmaz.

func requireRealSystemA105(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_REAL_SYSTEM_A105") != "1" {
		t.Skip("real-system evidence runs only in the disposable alpha.105 VM")
	}
}

func a105DpkgSet(t *testing.T) map[string]bool {
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

func a105Run(t *testing.T, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, lastLines(string(out), 20))
	}
}

func a105Install(t *testing.T, a *Agent, id, pkg, phpVersion string, confirmed []string) *InstallServiceResponse {
	t.Helper()
	svc := core.GetManagedServiceByID(id)
	var match []string
	if pkg != "" {
		var err error
		if match, err = validateRepoPackageSelection(svc, pkg); err != nil {
			t.Fatal(err)
		}
	}
	req := &InstallServiceRequest{ID: id, Package: pkg, PHPVersion: phpVersion, CheckInstalls: true, ConfirmedInstalls: confirmed}
	resp := &InstallServiceResponse{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := a.installServiceWith(ctx, req, svc, match, phpToolForInstall(id), phpVersion, resp); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "install "+id, resp)
	return resp
}

func TestRealSystemA105BundleWithAChosenPHPIsCheckedConfirmedAndProved(t *testing.T) {
	requireRealSystemA105(t)
	a := &Agent{}
	steps := []transport.ServiceInstallPreviewStep{{ID: "postgresql"}, {ID: "php-fpm", Package: "php8.3-fpm"}, {ID: "phppgadmin", PHPVersion: "8.3"}}

	// 1. The bundle's simulation, before anything changes.
	before := a105DpkgSet(t)
	var preview transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: steps}, &preview); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "bundle preview", preview)
	if preview.State != "known" || len(preview.Refusals) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	notable := []string{}
	for _, item := range preview.Notable {
		notable = append(notable, item.Package)
		if item.Reason != notableReasonNewPHPVersion || item.Version != "8.4" {
			t.Fatalf("unexpected notable %+v", item)
		}
	}
	if !slices.Contains(notable, "php8.4-pgsql") {
		t.Fatalf("the bundle did not name PHP 8.4's packages: %v", notable)
	}
	if after := a105DpkgSet(t); len(after) != len(before) {
		t.Fatalf("the preview changed the installed set: %d -> %d", len(before), len(after))
	}

	// 2. A chosen version that cannot serve the tool is refused.
	var refused transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "phppgadmin", PHPVersion: "5.1"}}}, &refused); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "preview for PHP 5.1", refused)
	if len(refused.Refusals) != 1 || refused.Refusals[0].Code != "php_version_unavailable" {
		t.Fatalf("refusals = %+v", refused.Refusals)
	}

	// 3. The bundle's steps through the Agent's own install code.
	if resp := a105Install(t, a, "postgresql", "", "", notable); resp.Error != "" {
		t.Fatal(resp.Error)
	}
	if resp := a105Install(t, a, "php-fpm", "php8.3-fpm", "", notable); resp.Error != "" {
		t.Fatal(resp.Error)
	}
	// Unconfirmed: refused, nothing installed.
	beforeTool := a105DpkgSet(t)
	unconfirmed := a105Install(t, a, "phppgadmin", "", "8.3", nil)
	if unconfirmed.ErrorCode != errCodeInstallNeedsConfirmation || len(unconfirmed.Notable) == 0 || !strings.Contains(unconfirmed.Error, "nothing was installed") {
		t.Fatalf("unconfirmed install = %+v", unconfirmed)
	}
	if after := a105DpkgSet(t); len(after) != len(beforeTool) || after["phppgadmin"] {
		t.Fatalf("an unconfirmed install changed the installed set")
	}
	confirmed := a105Install(t, a, "phppgadmin", "", "8.3", notable)
	if confirmed.Error != "" || !confirmed.FirstInstall {
		t.Fatalf("confirmed install = %+v", confirmed)
	}
	for _, pkg := range []string{"phppgadmin", "php8.3-pgsql", "php8.3-mbstring", "php8.3-xml"} {
		if !a105DpkgSet(t)[pkg] {
			t.Fatalf("%s is not installed", pkg)
		}
	}

	// 4. Configured on its chosen version and proved there.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var conf ConfigureDBToolsResponse
	configureDBToolsWith(ctx, &conf)
	logJSON(t, "configure", conf)
	if conf.Error != "" || !conf.Configured {
		t.Fatalf("configure = %+v", conf)
	}
	proof := provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, confirmed.FirstInstall)
	logJSON(t, "proof on the chosen version", proof)
	if !proof.Proved || proof.Moved || proof.Version != "8.3" {
		t.Fatalf("proof = %+v", proof)
	}
}

func TestRealSystemA105AFailingVersionTriesTheNext(t *testing.T) {
	requireRealSystemA105(t)
	if !phpVersionInstalled("8.3") || phpToolStateFor(phpToolPHPPgAdmin).version() != "8.3" {
		t.Skip("runs after the bundle test")
	}
	// PHP 8.4 is the default the tool's packages brought (php8.4-pgsql, -mbstring);
	// its FPM and php8.4-xml are installed as an owner would, so 8.4 has the
	// tool's extensions.
	a105Run(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "php8.4-fpm", "php8.4-xml")
	a105Run(t, "phpdismod", "-v", "8.3", "-s", "fpm", "pgsql")
	a105Run(t, "systemctl", "restart", "php8.3-fpm")
	t.Cleanup(func() {
		_ = exec.Command("phpenmod", "-v", "8.3", "-s", "fpm", "pgsql").Run()
		_ = exec.Command("systemctl", "restart", "php8.3-fpm").Run()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Without fallback (an already bound tool): never moved, Not ready.
	stay := provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, false)
	logJSON(t, "proof without fallback", stay)
	if stay.Proved || stay.Moved || len(stay.Attempts) != 1 || stay.Attempts[0].Cause == nil || stay.Attempts[0].Cause.Kind != "missing_extensions" {
		t.Fatalf("proof = %+v", stay)
	}
	view := phpToolBindingsView()
	logJSON(t, "bindings after a failed proof", view)
	for _, binding := range view.Tools {
		if binding.Tool == phpToolPHPPgAdmin && (!binding.ProofFailed || binding.Version != "8.3") {
			t.Fatalf("binding = %+v", binding)
		}
	}

	// A first install's proof: 8.3 fails, 8.4 (the default) is tried and proved.
	moved := provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, true)
	logJSON(t, "proof with fallback", moved)
	if !moved.Proved || !moved.Moved || moved.Version != "8.4" {
		t.Fatalf("proof = %+v", moved)
	}
	if got := phpToolStateFor(phpToolPHPPgAdmin).version(); got != "8.4" {
		t.Fatalf("phpPgAdmin runs on %q", got)
	}
	for _, binding := range phpToolBindingsView().Tools {
		if binding.Tool == phpToolPHPPgAdmin && binding.ProofFailed {
			t.Fatalf("the failed proof still stands: %+v", binding)
		}
	}
	if err := probePHPToolHTTP(ctx, phpToolPHPPgAdmin, "8.4"); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("phpPgAdmin does not answer on 8.4: %v", err)
	}
}
