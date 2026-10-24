package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.105 (D-034 amendment): unexpected installs are shown before
// anything changes, a chosen PHP version that cannot serve its tool is
// refused, and a PHP tool is proved on its first install.

func withPHPVersionsPresent(t *testing.T, versions ...string) {
	t.Helper()
	previous := phpVersionPresent
	present := map[string]bool{}
	for _, version := range versions {
		present[version] = true
	}
	phpVersionPresent = func(version string) bool { return present[version] }
	t.Cleanup(func() { phpVersionPresent = previous })
}

func notableNames(items []transport.NotableInstall) []string {
	names := []string{}
	for _, item := range items {
		names = append(names, item.Reason+":"+item.Package)
	}
	return names
}

func TestNotableInstallsAreClassifiedByReason(t *testing.T) {
	withPHPVersionsPresent(t, "8.3")
	c := newInstallClassification("apt", []string{"phpmyadmin", "php8.3-mysql"}, []string{"phpmyadmin"}, []string{"8.3"})
	got := notableNames(c.classify([]string{
		"phpmyadmin", "php8.3-mysql", "php8.3-common", "libzip5", "dbconfig-common",
		"php8.5-mbstring", "php8.5-common", "apache2", "apache2-bin", "libapache2-mod-php8.5",
		"php-fpm", "mariadb-server", "php-mbstring",
	}))
	want := []string{
		"database_server:mariadb-server",
		"apache:apache2", "apache:apache2-bin", "apache:libapache2-mod-php8.5",
		"new_php_version:php8.5-common", "new_php_version:php8.5-mbstring",
		"component:php-fpm",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("notable = %v, want %v", got, want)
	}
}

func TestTheVersionTheDistributionPHPBringsIsExpected(t *testing.T) {
	withPHPVersionsPresent(t)
	// A bundle on a clean server: PHP-FPM's distribution package and
	// phpMyAdmin together; php8.4-* is the version php-fpm installs.
	c := newInstallClassification("apt", []string{"php-fpm", "phpmyadmin"}, []string{"php-fpm", "phpmyadmin", "nginx", "mariadb"}, nil)
	got := c.classify([]string{"php-fpm", "php8.4-fpm", "php8.4-common", "php8.4-mbstring", "phpmyadmin", "php-mbstring"})
	if len(got) != 0 {
		t.Fatalf("notable = %v, want none", notableNames(got))
	}
	// The same install without php-fpm named: php8.4 is a new version.
	c = newInstallClassification("apt", []string{"phpmyadmin"}, []string{"phpmyadmin"}, nil)
	got = c.classify([]string{"php8.4-fpm", "php8.4-common", "phpmyadmin"})
	if names := notableNames(got); !reflect.DeepEqual(names, []string{"new_php_version:php8.4-common", "new_php_version:php8.4-fpm"}) {
		t.Fatalf("notable = %v", names)
	}
}

func TestAnUnconfirmedNotableInstallIsRefusedUnderTheLock(t *testing.T) {
	withPHPVersionsPresent(t, "8.3")
	run := func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		if args[0] != "-s" || args[1] != "install" {
			t.Fatalf("args = %v", args)
		}
		return []byte("Inst phpmyadmin (4:5.2.2 Debian:13 [all])\nInst apache2 (2.4.66-1 Debian:13 [amd64])\nInst libzip5 (1.11 Debian:13 [amd64])\n"), nil
	}
	c := newInstallClassification("apt", []string{"phpmyadmin"}, []string{"phpmyadmin"}, []string{"8.3"})
	err := checkAptInstallSimulation(context.Background(), run, "/usr/bin/apt-get", []string{"phpmyadmin"}, aptInstallArgs([]string{"phpmyadmin"}),
		notableInstallGuard(c, []string{"phpmyadmin"}, nil))
	var refusal *packageInstallNotableRefusal
	if !errors.As(err, &refusal) || len(refusal.Notable) != 1 || refusal.Notable[0].Package != "apache2" ||
		!strings.Contains(err.Error(), "apache2") || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("err = %v", err)
	}
	// Confirmed by name: the install goes ahead.
	if err := checkAptInstallSimulation(context.Background(), run, "/usr/bin/apt-get", []string{"phpmyadmin"}, aptInstallArgs([]string{"phpmyadmin"}),
		notableInstallGuard(c, []string{"phpmyadmin"}, []string{"apache2"})); err != nil {
		t.Fatal(err)
	}
	// A removal still refuses first, whatever was confirmed.
	removing := func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte("Remv nginx [1.26]\nInst apache2 (2.4)\n"), nil
	}
	err = checkAptInstallSimulation(context.Background(), removing, "/usr/bin/apt-get", []string{"phpmyadmin"}, aptInstallArgs([]string{"phpmyadmin"}),
		notableInstallGuard(c, []string{"phpmyadmin"}, []string{"apache2"}))
	var removes *packageInstallRefusal
	if !errors.As(err, &removes) {
		t.Fatalf("err = %v", err)
	}
}

type fakePreviewHost struct {
	installed map[string]bool
	available map[string]bool
	sim       aptSimulation
	simErr    error
	busy      bool
	simulated []string
}

func (h *fakePreviewHost) ops() installPreviewOps {
	return installPreviewOps{
		family:    "apt",
		installed: func(pkg string) bool { return h.installed[pkg] },
		available: func(pkg string) bool { return h.available[pkg] },
		simulate: func(_ context.Context, packages []string) (aptSimulation, error) {
			h.simulated = append([]string{}, packages...)
			return h.sim, h.simErr
		},
		lock:   func() bool { return !h.busy },
		unlock: func() {},
	}
}

func TestABundlePreviewSimulatesItsStepsTogether(t *testing.T) {
	withPHPVersionsPresent(t)
	host := &fakePreviewHost{
		installed: map[string]bool{"nginx": true, "mariadb-server": true},
		available: map[string]bool{"php8.3-cli": true, "php8.3-common": true, "php8.3-mysql": true, "php8.3-mbstring": true, "php8.3-xml": true, "php8.3-fpm": true},
		sim:       aptSimulation{Installed: []string{"php8.3-fpm", "php8.3-cli", "php8.3-mysql", "phpmyadmin", "php8.5-mbstring", "libapache2-mod-php8.5", "apache2"}},
	}
	req := &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{
		{ID: "php-fpm", Package: "php8.3-fpm"},
		{ID: "phpmyadmin", PHPVersion: "8.3"},
	}}
	resp := emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, host.ops())
	if resp.State != "known" || len(resp.Refusals) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
	for _, want := range []string{"php8.3-fpm", "php8.3-mysql", "phpmyadmin"} {
		found := false
		for _, pkg := range host.simulated {
			found = found || pkg == want
		}
		if !found {
			t.Fatalf("simulated %v lacks %s", host.simulated, want)
		}
	}
	if names := notableNames(resp.Notable); !reflect.DeepEqual(names, []string{"apache:apache2", "apache:libapache2-mod-php8.5", "new_php_version:php8.5-mbstring"}) {
		t.Fatalf("notable = %v", names)
	}
}

func TestAChosenPHPVersionWithoutTheToolsExtensionsIsRefused(t *testing.T) {
	withPHPVersionsPresent(t, "8.6")
	host := &fakePreviewHost{
		installed: map[string]bool{"php8.6-fpm": true, "php8.6-xml": true},
		available: map[string]bool{"php8.6-mbstring": true},
	}
	req := &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "phpmyadmin", PHPVersion: "8.6"}}}
	resp := emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, host.ops())
	if len(resp.Refusals) != 1 || resp.Refusals[0].Code != "php_extensions_unavailable" || !reflect.DeepEqual(resp.Refusals[0].Packages, []string{"php8.6-mysql"}) {
		t.Fatalf("refusals = %+v", resp.Refusals)
	}
	// A version with no FPM package at all.
	req.Steps[0].PHPVersion = "9.1"
	resp = emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, host.ops())
	if len(resp.Refusals) != 1 || resp.Refusals[0].Code != "php_version_unavailable" {
		t.Fatalf("refusals = %+v", resp.Refusals)
	}
}

func TestAPreviewNeverWaitsForThePackageLock(t *testing.T) {
	host := &fakePreviewHost{busy: true, installed: map[string]bool{}, available: map[string]bool{}}
	req := &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "nginx"}}}
	resp := emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, host.ops())
	if resp.State != "busy" || host.simulated != nil {
		t.Fatalf("resp = %+v simulated = %v", resp, host.simulated)
	}
	host = &fakePreviewHost{installed: map[string]bool{}, available: map[string]bool{}, simErr: errors.New("APT simulation failed")}
	resp = emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, host.ops())
	if resp.State != "failed" || resp.Error == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestPacmanPrintNamesTheTargets(t *testing.T) {
	got := parsePacmanPrint([]byte(":: Synchronizing\nphp\nphp-fpm\nwarning: something here\nphpmyadmin\n"))
	if !reflect.DeepEqual(got, []string{"php", "php-fpm", "phpmyadmin"}) {
		t.Fatalf("got %v", got)
	}
}

// proveFake builds prove ops over the bind fake with a probe that fails on
// the listed versions.
func proveFake(host *fakeBindHost, failing map[string]error, candidates []string, recorded map[string]*transport.PHPToolCause) phpToolProveOps {
	bind := host.ops()
	bind.httpProbe = func(_ context.Context, _ string, version string) error {
		host.steps = append(host.steps, "probe:"+version)
		return failing[version]
	}
	return phpToolProveOps{
		bind:       bind,
		candidates: func(string, string) []string { return candidates },
		record: func(tool string, cause *transport.PHPToolCause) error {
			recorded[tool] = cause
			return nil
		},
	}
}

func TestAToolJustInstalledIsProvedOnItsVersion(t *testing.T) {
	host := newFakeBindHost()
	recorded := map[string]*transport.PHPToolCause{"phpmyadmin": {Kind: "other"}}
	resp := provePHPToolWith(context.Background(), proveFake(host, nil, []string{"8.5"}, recorded), phpToolPHPMyAdmin, true)
	if !resp.Proved || resp.Moved || resp.Version != "8.4" || recorded["phpmyadmin"] != nil || host.recorded["phpmyadmin"] != "8.4" {
		t.Fatalf("resp = %+v recorded = %v", resp, recorded)
	}
}

func TestAFailedFirstProofTriesTheNextVersion(t *testing.T) {
	host := newFakeBindHost()
	host.packages = map[string]bool{"php8.5-mysql": true, "php8.5-mbstring": true, "php8.5-xml": true}
	recorded := map[string]*transport.PHPToolCause{}
	failing := map[string]error{"8.4": &phpToolProbeError{Kind: "missing_extensions", Message: "PHP-FPM 8.4 lacks mysqli"}}
	resp := provePHPToolWith(context.Background(), proveFake(host, failing, []string{"8.5"}, recorded), phpToolPHPMyAdmin, true)
	if !resp.Proved || !resp.Moved || resp.Version != "8.5" || len(resp.Attempts) != 1 || resp.Attempts[0].Version != "8.4" ||
		resp.Attempts[0].Cause == nil || resp.Attempts[0].Cause.Kind != "missing_extensions" {
		t.Fatalf("resp = %+v", resp)
	}
	if _, failed := recorded["phpmyadmin"]; !failed || recorded["phpmyadmin"] != nil {
		t.Fatalf("a passing proof clears the record: %v", recorded)
	}
}

func TestAToolProvedNowhereStaysInstalledAndNotReady(t *testing.T) {
	host := newFakeBindHost()
	host.packages = map[string]bool{"php8.5-mysql": true, "php8.5-mbstring": true, "php8.5-xml": true}
	recorded := map[string]*transport.PHPToolCause{}
	failing := map[string]error{
		"8.4": &phpToolProbeError{Kind: "no_answer", Message: "phpMyAdmin did not answer"},
		"8.5": &phpToolProbeError{Kind: "http_status", Status: 502, Message: "502"},
	}
	resp := provePHPToolWith(context.Background(), proveFake(host, failing, []string{"8.5"}, recorded), phpToolPHPMyAdmin, true)
	if resp.Proved || len(resp.Attempts) != 2 || recorded["phpmyadmin"] == nil || recorded["phpmyadmin"].Kind != "no_answer" {
		t.Fatalf("resp = %+v recorded = %v", resp, recorded["phpmyadmin"])
	}
	// The failed move put the previous configuration back.
	if string(host.conf["/etc/nginx/conf.d/celikpanel-dbtools.conf"]) != "old" {
		t.Fatalf("conf = %q", host.conf["/etc/nginx/conf.d/celikpanel-dbtools.conf"])
	}
}

func TestAToolAlreadyBoundIsNeverMovedByAProof(t *testing.T) {
	host := newFakeBindHost()
	recorded := map[string]*transport.PHPToolCause{}
	failing := map[string]error{"8.4": &phpToolProbeError{Kind: "no_answer", Message: "no"}}
	resp := provePHPToolWith(context.Background(), proveFake(host, failing, []string{"8.5"}, recorded), phpToolPHPMyAdmin, false)
	if resp.Proved || resp.Moved || len(resp.Attempts) != 1 || recorded["phpmyadmin"] == nil {
		t.Fatalf("resp = %+v", resp)
	}
	for _, step := range host.steps {
		if strings.HasPrefix(step, "write:") {
			t.Fatalf("a proof without fallback wrote a configuration: %v", host.steps)
		}
	}
}
