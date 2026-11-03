package core

import (
	"reflect"
	"testing"
)

// alpha.119: Go and Rust come from the operating system's own packages. The
// mapping was read from the distributions' package lists: Debian 12 and 13 and
// Ubuntu 22.04 and 24.04 share the apt names (golang-go; rustc and cargo), Arch
// ships go and rust (rust carries cargo and rustc).
func TestToolchainPackagesPerFamily(t *testing.T) {
	want := map[string]map[string][]string{
		"go":   {"apt": {"golang-go"}, "pacman": {"go"}},
		"rust": {"apt": {"rustc", "cargo"}, "pacman": {"rust"}},
	}
	for id, families := range want {
		svc := GetManagedServiceByID(id)
		if svc == nil {
			t.Fatalf("%s is not in the catalogue", id)
		}
		if !reflect.DeepEqual(svc.Packages, families) {
			t.Fatalf("%s packages = %v, want %v", id, svc.Packages, families)
		}
		for _, family := range []string{"apt", "pacman"} {
			if kind, reason := ManagedServiceInstallBlock(svc, family); kind != ManagedServiceInstallBlockNone {
				t.Fatalf("%s on %s is closed: %s", id, family, reason)
			}
		}
		// Not part of an unqualified or preview family.
		if kind, _ := ManagedServiceInstallBlock(svc, "dnf"); kind == ManagedServiceInstallBlockNone {
			t.Fatalf("%s is installable on dnf", id)
		}
		if svc.Repo != nil || len(svc.SystemNames) != 0 || len(svc.FirewallPorts) != 0 {
			t.Fatalf("%s must be a plain package tool: no vendor repository, unit or port", id)
		}
		if svc.Kind != KindTool || svc.Category != DeveloperToolsCategory || !svc.AdminOnly {
			t.Fatalf("%s: %+v", id, svc)
		}
	}
}

// The distribution's version is what is installed; no upstream tarball and no
// third-party repository is part of 119.
func TestToolchainsUseOnlyOfficialDistributionPackages(t *testing.T) {
	for _, id := range []string{"go", "rust"} {
		svc := GetManagedServiceByID(id)
		if svc.Repo != nil {
			t.Fatalf("%s carries a vendor repository", id)
		}
		for family, packages := range svc.Packages {
			if family != "apt" && family != "pacman" {
				t.Fatalf("%s: unexpected family %s", id, family)
			}
			for _, name := range packages {
				if name == "" || len(name) > 20 {
					t.Fatalf("%s: package name %q", id, name)
				}
			}
		}
	}
}
