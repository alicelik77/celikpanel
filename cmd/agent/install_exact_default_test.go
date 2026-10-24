package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.107 (D-038 §1/§7): with a vendor repository enabled, a plain install
// names the exact version the distribution's default package brings.

const phpFPMShow = `Package: php-fpm
Version: 2:96+sury1
Depends: php8.5-fpm

Package: php-fpm
Version: 2:96
Depends: php8.4-fpm
`

func exactOps(enabled bool, show string, showErr error) exactDefaultOps {
	return exactDefaultOps{
		repoEnabled: func(string) bool { return enabled },
		show:        func(context.Context, string) (string, error) { return show, showErr },
		vendorOf: func(name, version string) string {
			if strings.Contains(version, "sury") {
				return "sury"
			}
			return ""
		},
	}
}

func TestWithTheVendorRepositoryAPlainInstallNamesTheDistributionsExactVersion(t *testing.T) {
	php := core.GetManagedServiceByID("php-fpm")
	got, err := exactDefaultPackage(context.Background(), php, "apt", exactOps(true, phpFPMShow, nil))
	if err != nil || got != "php8.4-fpm" {
		t.Fatalf("got %q, %v", got, err)
	}
	// Without the repository nothing changes.
	if got, err := exactDefaultPackage(context.Background(), php, "apt", exactOps(false, phpFPMShow, nil)); err != nil || got != "" {
		t.Fatalf("disabled: %q, %v", got, err)
	}
	// pacman, and a component whose repository is required, are untouched.
	if got, _ := exactDefaultPackage(context.Background(), php, "pacman", exactOps(true, phpFPMShow, nil)); got != "" {
		t.Fatalf("pacman: %q", got)
	}
	if got, _ := exactDefaultPackage(context.Background(), core.GetManagedServiceByID("netdata"), "apt", exactOps(true, phpFPMShow, nil)); got != "" {
		t.Fatalf("netdata: %q", got)
	}
	pg := core.GetManagedServiceByID("postgresql")
	got, err = exactDefaultPackage(context.Background(), pg, "apt", exactOps(true, "Package: postgresql\nVersion: 17+278\nDepends: postgresql-17, postgresql-common (>= 252~)\n", nil))
	if err != nil || got != "postgresql-17" {
		t.Fatalf("postgresql: %q, %v", got, err)
	}
}

func TestAnUnresolvableDefaultIsRefusedNotInstalledAsTheMovingPackage(t *testing.T) {
	php := core.GetManagedServiceByID("php-fpm")
	if _, err := exactDefaultPackage(context.Background(), php, "apt", exactOps(true, "Package: php-fpm\nVersion: 2:96+sury1\nDepends: php8.5-fpm\n", nil)); err == nil ||
		!strings.Contains(err.Error(), "choose the version") {
		t.Fatalf("err = %v", err)
	}
	if _, err := exactDefaultPackage(context.Background(), php, "apt", exactOps(true, "", errors.New("boom"))); err == nil || !strings.Contains(err.Error(), "never the moving default package") {
		t.Fatalf("err = %v", err)
	}
}

func TestAPreviewOfAPlainInstallSimulatesTheExactPackage(t *testing.T) {
	host := &fakePreviewHost{installed: map[string]bool{}, sim: aptSimulation{Installed: []string{"php8.4-fpm", "php8.4-common"}}}
	ops := host.ops()
	ops.exactDefault = func(context.Context, *core.ManagedService) (string, error) { return "php8.4-fpm", nil }
	var resp transport.ServiceInstallPreviewResponse
	serviceInstallPreviewWithOps(context.Background(), &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "php-fpm"}}}, &resp, ops)
	if resp.State != "known" || strings.Join(host.simulated, ",") != "php8.4-fpm" {
		t.Fatalf("resp = %+v simulated = %v", resp, host.simulated)
	}
}
