package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

func TestDebianVersionsOrderAsDpkgDoes(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2:101+0~20261021.78+debian13~1.gbp53f25f", "2:96", 1},
		{"1:11.8.3-0+deb13u1", "1:11.8.3-0+deb13u2", -1},
		{"1.0~rc1", "1.0", -1},
		{"18.0-1.pgdg13+1", "17.6-0+deb13u1", 1},
		{"5.40.1-6", "5.40.1-6+deb13u1", -1},
		{"1.2", "1.2", 0},
	} {
		if got := debVersionCompare(c.a, c.b); got != c.want {
			t.Fatalf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestAVendorOnlyVersionNamesTheVendorsSharedPackages(t *testing.T) {
	repo := &core.ManagedRepo{ID: "sury", SharedPackages: []string{"php-common"}}
	index := map[string]map[string]bool{"php-common": {"2:100+0~1": true, "2:101+0~1": true}}
	got := vendorSharedPackageArgsFrom(repo, map[string][]string{"php-common": {"2:96"}}, index)
	if strings.Join(got, ",") != "php-common=2:101+0~1" {
		t.Fatalf("args = %v", got)
	}
	if got := vendorSharedPackageArgsFrom(repo, map[string][]string{"php-common": {"2:100+0~1"}}, index); len(got) != 0 {
		t.Fatalf("already the vendor's: %v", got)
	}
	if !validInstallArg("php-common=2:101+0~1") || validInstallArg("php-common=--force") || validInstallArg("-o=x") || installArgName("libpq5=18.0-1") != "libpq5" {
		t.Fatal("install argument validation")
	}
}

func TestTheVendorsSharedPackageIsNamedAndConfirmedNeverSilent(t *testing.T) {
	withPHPVersionsPresent(t, "8.4")
	args := []string{"php8.3-fpm", "php8.3-cli", "php-common=2:101+sury"}
	c := newInstallClassification("apt", args, []string{"php-fpm"}, nil, nil)
	c.vendorOf = func(name, version string) string {
		if strings.Contains(version, "sury") {
			return "sury"
		}
		return ""
	}
	sim := aptSimulation{
		Installed: []string{"php8.3-fpm", "php8.3-cli", "php8.3-common", "php-common"},
		Upgrades:  map[string]aptUpgrade{"php-common": {From: "2:96", To: "2:101+sury"}},
	}
	notable := c.classifySimulation(sim)
	if len(notable) != 1 || notable[0].Reason != notableReasonVendorShared || notable[0].From != "2:96" || notable[0].To != "2:101+sury" || notable[0].Component != "sury" {
		t.Fatalf("notable = %+v", notable)
	}
	guard := notableInstallGuard(c, args, nil)
	var refusal *packageInstallNotableRefusal
	if err := guard(sim); !errors.As(err, &refusal) {
		t.Fatalf("unconfirmed = %v", err)
	}
	if err := notableInstallGuard(c, args, []string{"php-common"})(sim); err != nil {
		t.Fatalf("confirmed = %v", err)
	}
}
