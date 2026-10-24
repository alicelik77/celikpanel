package supportdata

import (
	"testing"
	"time"
)

// alpha.107 (D-038 §11): "unsupported" means whoever provides the package no
// longer ships security fixes for it; a distribution keeps patching the
// version it ships; unknown stays unknown.

func at(t *testing.T, day string) time.Time {
	t.Helper()
	value, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTheTableIsReadAndEveryDateIsADate(t *testing.T) {
	table, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if table.Checked == "" || len(table.Sources) == 0 {
		t.Fatal("the table names no check date or sources")
	}
	for id, component := range table.Components {
		if table.Sources[component.Source] == "" {
			t.Fatalf("%s names no source", id)
		}
		for line, version := range component.Versions {
			if _, err := time.Parse("2006-01-02", version.UpstreamUntil); err != nil {
				t.Fatalf("%s %s: %v", id, line, err)
			}
		}
		for distro, shipped := range component.Distro {
			if _, ok := table.Distros[distro]; !ok {
				t.Fatalf("%s ships on an unknown release %s", id, distro)
			}
			for line := range shipped {
				if _, ok := component.Versions[line]; !ok {
					t.Fatalf("%s %s on %s has no upstream date", id, line, distro)
				}
			}
		}
	}
	for key, distro := range table.Distros {
		if _, err := time.Parse("2006-01-02", distro.PatchedUntil); err != nil || table.Sources[distro.Source] == "" {
			t.Fatalf("%s: %v", key, err)
		}
	}
}

func TestADistributionKeepsPatchingTheVersionItShips(t *testing.T) {
	table, _ := Load()
	now := at(t, "2026-10-24")
	// PHP 8.1 is past its upstream end, but Ubuntu 22.04 ships and patches it.
	if got := table.Status("php", "8.1", "ubuntu-22.04", now); got.Status != StatusSupported || got.Provider != "distribution" || got.Until != "2027-05-31" {
		t.Fatalf("ubuntu 22.04 php 8.1 = %+v", got)
	}
	// The same 8.1 from a vendor repository on Debian 13 follows upstream.
	if got := table.Status("php", "8.1.34", "debian-13", now); got.Status != StatusUnsupported || got.Provider != "upstream" {
		t.Fatalf("debian 13 php 8.1 = %+v", got)
	}
	if got := table.Status("php", "8.4", "debian-13", now); got.Status != StatusSupported || got.Provider != "distribution" {
		t.Fatalf("debian 13 php 8.4 = %+v", got)
	}
	// Within 90 days of its end: ends soon.
	if got := table.Status("php", "8.2", "debian-13", now); got.Status != StatusEndsSoon || got.Until != "2026-12-31" {
		t.Fatalf("php 8.2 = %+v", got)
	}
	if got := table.Status("postgresql", "14", "debian-13", now); got.Status != StatusEndsSoon {
		t.Fatalf("postgresql 14 = %+v", got)
	}
	if got := table.Status("node", "22.14.0", "debian-13", now); got.Status != StatusSupported || got.Until != "2027-04-30" {
		t.Fatalf("node 22 = %+v", got)
	}
	if got := table.Status("node", "20.19.2", "debian-13", now); got.Status != StatusUnsupported {
		t.Fatalf("node 20 = %+v", got)
	}
}

func TestWhatIsNotKnownStaysUnknown(t *testing.T) {
	table, _ := Load()
	now := at(t, "2026-10-24")
	for _, c := range []struct{ component, version, distro string }{
		{"php", "8.6", "debian-13"},          // not released when the table was checked
		{"mariadb", "10.11", "ubuntu-24.04"}, // universe: not standard security maintenance
		{"redis", "7.2", "debian-13"},        // not in the table
		{"php", "", "debian-13"},
	} {
		if got := table.Status(c.component, c.version, c.distro, now); got.Status != StatusUnknown {
			t.Fatalf("%+v = %+v", c, got)
		}
	}
	if DistroKey("debian", "13") != "debian-13" || DistroKey("ubuntu", "24.04") != "ubuntu-24.04" || DistroKey("", "1") != "" {
		t.Fatal("distro keys")
	}
}
