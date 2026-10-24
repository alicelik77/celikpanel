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
// with PHP 8.3 chosen from the vendor repository. phpMyAdmin alone on the
// owner's bare PHP 8.3 would bring the default PHP's packages: the simulation
// names them before anything changes and the unconfirmed install is refused
// with nothing installed. The bundle with 8.3 chosen brings no other PHP;
// its steps run through the Agent's own install code, each checked, and
// phpPgAdmin is configured and proved on 8.3; then,
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
	// The vendor PHP repository enabled the way Components enables it (the
	// base image's own source for it is replaced), then the owner's own bare
	// PHP 8.3 from it; the distribution's default PHP is not installed.
	if err := os.Remove("/etc/apt/sources.list.d/php.list"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var repo RepoStatusResponse
	repoCtx, repoCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if err := a.enableRepoWith(repoCtx, &EnableRepoRequest{RepoID: "sury"}, &repo); err != nil || repo.Error != "" || !repo.Enabled {
		repoCancel()
		t.Fatalf("enable the vendor PHP repository: %v %+v", err, repo)
	}
	repoCancel()
	a105Run(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "php8.3-fpm", "php8.3-cli")

	// 1. A notable install is said before anything changes: phpMyAdmin on
	// this server would bring the default PHP's packages.
	before := a105DpkgSet(t)
	var alone transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "phpmyadmin"}}}, &alone); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "phpMyAdmin preview", alone)
	notable := []string{}
	for _, item := range alone.Notable {
		notable = append(notable, item.Package)
	}
	if alone.State != "known" || !slices.Contains(notable, "php8.4-mysql") || !slices.Contains(notable, "php8.4-common") {
		t.Fatalf("the preview did not name PHP 8.4's packages: %+v", alone)
	}
	// Unconfirmed, the real install is refused and nothing changes.
	var pkgOut string
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	guard := notableInstallGuard(newInstallClassification("apt", []string{"phpmyadmin"}, []string{"phpmyadmin"}, nil, a.installedServiceSet()), []string{"phpmyadmin"}, nil)
	pkgOut, err := installPackagesGuardedContext(ctx, "apt", []string{"phpmyadmin"}, "", guard)
	var refusal *packageInstallNotableRefusal
	if !errors.As(err, &refusal) || pkgOut != "" {
		t.Fatalf("unconfirmed phpMyAdmin install: out=%q err=%v", pkgOut, err)
	}
	t.Logf("unconfirmed install refused: %v", err)
	if after := a105DpkgSet(t); len(after) != len(before) || after["phpmyadmin"] {
		t.Fatalf("an unconfirmed install changed the installed set")
	}

	// 2. The bundle with PHP 8.3 chosen: the tool's extensions for 8.3 come
	// with it, so the package manager brings no other PHP.
	steps := []transport.ServiceInstallPreviewStep{{ID: "postgresql"}, {ID: "php-fpm", Package: "php8.3-fpm"}, {ID: "phppgadmin", PHPVersion: "8.3"}}
	var preview transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: steps}, &preview); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "bundle preview", preview)
	if preview.State != "known" || len(preview.Refusals) != 0 || len(preview.Notable) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	var refused transport.ServiceInstallPreviewResponse
	if err := a.ServiceInstallPreviewV1(&transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "phppgadmin", PHPVersion: "5.1"}}}, &refused); err != nil {
		t.Fatal(err)
	}
	logJSON(t, "preview for PHP 5.1", refused)
	if len(refused.Refusals) != 1 || refused.Refusals[0].Code != "php_version_unavailable" {
		t.Fatalf("refusals = %+v", refused.Refusals)
	}

	// 3. The bundle's steps through the Agent's own install code, each
	// checked against what was confirmed (nothing: nothing was notable).
	for _, step := range steps {
		if resp := a105Install(t, a, step.ID, step.Package, step.PHPVersion, nil); resp.Error != "" {
			t.Fatalf("%s: %s", step.ID, resp.Error)
		} else if step.ID == "phppgadmin" && !resp.FirstInstall {
			t.Fatalf("phpPgAdmin was not a first install: %+v", resp)
		}
	}
	installed := a105DpkgSet(t)
	for _, pkg := range []string{"phppgadmin", "php8.3-pgsql", "php8.3-mbstring", "php8.3-xml", "postgresql"} {
		if !installed[pkg] {
			t.Fatalf("%s is not installed", pkg)
		}
	}
	for pkg := range installed {
		if strings.HasPrefix(pkg, "php8.4-") || strings.HasPrefix(pkg, "apache2") {
			t.Fatalf("%s was installed without being confirmed", pkg)
		}
	}

	// 4. Configured on its chosen version and proved there.
	var conf ConfigureDBToolsResponse
	configureDBToolsWith(ctx, &conf)
	logJSON(t, "configure", conf)
	if conf.Error != "" || !conf.Configured {
		t.Fatalf("configure = %+v", conf)
	}
	proof := provePHPToolWith(ctx, hostPHPToolProveOps(), phpToolPHPPgAdmin, true)
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
	// The owner adds PHP 8.4 with the tool's extensions.
	a105Run(t, "apt-get", "install", "-y", "-q", "--no-install-recommends", "php8.4-fpm", "php8.4-pgsql", "php8.4-mbstring", "php8.4-xml")
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
