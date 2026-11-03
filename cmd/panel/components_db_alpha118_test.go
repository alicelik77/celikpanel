package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
)

// alpha.118, the owner's Boston screens (Ubuntu 24.04, alpha.117): the
// Components page showed only the "PostgreSQL + phpPgAdmin" bundle, so the
// owner installed the bundle, then removed PostgreSQL, and phpPgAdmin stayed
// "Installed" without the database it administers.
// Sahibin Boston ekranları: yalnız paket görünüyordu; PostgreSQL kaldırılınca
// phpPgAdmin sahipsiz kaldı.

// PostgreSQL that is not installed is an installable row of its own, on every
// family that installs it, with its own vendor repository for the version
// choice the install dialog offers (/api/v1/repo, PGDG on apt). A bundle never
// takes its place.
// Kurulu olmayan PostgreSQL kendi satırında kurulabilir; sürüm seçimi deposu vardır.
func TestPostgreSQLNotInstalledIsAnInstallableRowWithItsVersions(t *testing.T) {
	for _, family := range []string{"apt", "pacman"} {
		view := catalogView([]serviceObservation{
			{ID: "nginx", IsInstalled: true, Status: "active (running)"},
			{ID: "php-fpm", IsInstalled: true, Status: "active (running)"},
			{ID: "mariadb", IsInstalled: true, Status: "active (running)"},
			{ID: "postgresql", IsInstalled: false, Status: "not_installed"},
			{ID: "phppgadmin", IsInstalled: false, Status: "not_installed"},
		}, family)
		index := slices.IndexFunc(view, func(row ManagedServiceResponse) bool { return row.ID == "postgresql" })
		if index < 0 {
			t.Fatalf("%s: no PostgreSQL row", family)
		}
		row := view[index]
		if row.IsInstalled == nil || *row.IsInstalled || row.Status != "not_installed" {
			t.Fatalf("%s: PostgreSQL row = (%v, %q), want an observed not-installed row", family, row.IsInstalled, row.Status)
		}
		if row.NotOffered || row.ConflictWith != "" || len(row.RequiresMissing) != 0 || row.Kind != core.KindService || row.Category != "database" {
			t.Fatalf("%s: PostgreSQL row cannot be installed on its own: %+v", family, row)
		}
		if len(row.Packages) == 0 {
			t.Fatalf("%s: PostgreSQL row names no package", family)
		}
	}
	pg := core.GetManagedServiceByID("postgresql")
	if pg.Repo == nil || pg.Repo.ID != "pgdg" || pg.Repo.PackagePattern == "" {
		t.Fatalf("PostgreSQL lost its version choice: repo = %+v", pg.Repo)
	}
}

// An installed tool whose own component is gone says so (needs_missing); one
// whose requirements are all there says nothing; a requirement nobody has
// observed is not claimed missing; a service that is not a tool never carries
// it.
// Kurulu araç, gereksinimi gittiyse bunu söyler; gözlenmemiş gereksinim iddia edilmez.
func TestInstalledToolWithoutItsDatabaseSaysWhatItNeeds(t *testing.T) {
	view := catalogView([]serviceObservation{
		{ID: "nginx", IsInstalled: true, Status: "active (running)"},
		{ID: "apache", IsInstalled: false, Status: "not_installed"},
		{ID: "php-fpm", IsInstalled: true, Status: "active (running)"},
		{ID: "mariadb", IsInstalled: true, Status: "active (running)"},
		{ID: "phpmyadmin", IsInstalled: true, Status: "installed"},
		{ID: "postgresql", IsInstalled: false, Status: "not_installed"},
		{ID: "phppgadmin", IsInstalled: true, Status: "installed"},
		{ID: "rspamd", IsInstalled: true, Status: "active (running)"},
	}, "apt")
	byID := map[string]ManagedServiceResponse{}
	for _, row := range view {
		byID[row.ID] = row
	}
	if got := byID["phppgadmin"].NeedsMissing; !slices.Equal(got, []string{"postgresql"}) {
		t.Fatalf("phpPgAdmin without PostgreSQL needs %v, want [postgresql]", got)
	}
	if got := byID["phpmyadmin"].NeedsMissing; len(got) != 0 {
		t.Fatalf("phpMyAdmin beside MariaDB needs %v", got)
	}
	// Rspamd needs an SMTP server nobody has observed here: not a tool, and
	// not observed, so nothing is claimed.
	if got := byID["rspamd"].NeedsMissing; len(got) != 0 {
		t.Fatalf("rspamd carries needs_missing %v", got)
	}
	raw, err := json.Marshal(byID["phpmyadmin"])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "needs_missing") {
		t.Fatalf("a tool with everything it needs sends needs_missing: %s", raw)
	}

	// The scan never looked at PostgreSQL: unknown, not missing.
	unobserved := catalogView([]serviceObservation{
		{ID: "nginx", IsInstalled: true, Status: "active (running)"},
		{ID: "php-fpm", IsInstalled: true, Status: "active (running)"},
		{ID: "phppgadmin", IsInstalled: true, Status: "installed"},
	}, "apt")
	for _, row := range unobserved {
		if row.ID == "phppgadmin" && len(row.NeedsMissing) != 0 {
			t.Fatalf("an unobserved PostgreSQL was claimed missing: %v", row.NeedsMissing)
		}
	}
}

// Removing a database engine names its own web admin tool as a companion the
// dialog offers to remove after it; a tool that only needs a seat (Roundcube
// needs "an SMTP server") is not one, nor is anything for one version.
// Motor kaldırılırken kendi web yönetim aracı birlikte kaldırmaya sunulur.
func TestRemovalImpactOffersTheEnginesOwnToolAsCompanion(t *testing.T) {
	pg := removalImpactFor(t, "postgresql", "apt", removalFullServer)
	if !slices.Equal(pg.CompanionTools, []string{"phppgadmin"}) {
		t.Fatalf("postgresql companions = %v", pg.CompanionTools)
	}
	maria := removalImpactFor(t, "mariadb", "apt", removalFullServer)
	if !slices.Equal(maria.CompanionTools, []string{"phpmyadmin"}) {
		t.Fatalf("mariadb companions = %v", maria.CompanionTools)
	}
	postfix := removalImpactFor(t, "postfix", "apt", removalFullServer)
	if len(postfix.CompanionTools) != 0 {
		t.Fatalf("postfix companions = %v (Roundcube needs a seat, not Postfix)", postfix.CompanionTools)
	}
	nginx := removalImpactFor(t, "nginx", "apt", removalFullServer)
	if len(nginx.CompanionTools) != 0 {
		t.Fatalf("nginx companions = %v", nginx.CompanionTools)
	}
	// PostgreSQL alone: no tool installed, nothing offered, and the wire
	// carries an empty list.
	alone := removalImpactFor(t, "postgresql", "apt", removalInstalled("postgresql"))
	raw, err := json.Marshal(alone)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"companion_tools":[]`) {
		t.Fatalf("%s lacks an empty companion_tools", raw)
	}
}
