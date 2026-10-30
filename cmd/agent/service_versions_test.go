package main

import (
	"testing"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestServiceVersionsComeFromThePackageDatabase(t *testing.T) {
	list := []core.Service{
		{ID: "cron", Version: "unknown"},
		{ID: "nginx", Version: "unknown"},
		{ID: "php8.3-fpm", Version: "8.3"},
	}
	fillServiceVersionsFromPackages(list, transport.InstalledPackageVersionsResponse{
		State: transport.PackageVersionsRead,
		Components: []transport.ComponentPackageVersions{
			{ServiceID: "cron", Packages: []transport.InstalledPackageVersion{{Package: "cron", Version: "3.0pl1-197"}}},
			{ServiceID: "nginx", Packages: []transport.InstalledPackageVersion{{Package: "nginx", Version: "1.24.0-2ubuntu7.5"}}},
			{ServiceID: "php-fpm", Packages: []transport.InstalledPackageVersion{{Package: "php8.3-fpm", Version: "8.3.6-0ubuntu0.24.04.5"}}},
		},
	})
	want := map[string]string{"cron": "3.0", "nginx": "1.24.0", "php8.3-fpm": "8.3"}
	for _, s := range list {
		if s.Version != want[s.ID] {
			t.Errorf("%s version = %q, want %q", s.ID, s.Version, want[s.ID])
		}
	}
}

func TestServiceVersionsStayUnknownWhenThePackageDatabaseIsNotRead(t *testing.T) {
	for _, state := range []string{transport.PackageVersionsBusy, transport.PackageVersionsFailed, transport.PackageVersionsUnsupported} {
		list := []core.Service{{ID: "cron", Version: "unknown"}}
		fillServiceVersionsFromPackages(list, transport.InstalledPackageVersionsResponse{
			State:      state,
			Components: []transport.ComponentPackageVersions{{ServiceID: "cron", Packages: []transport.InstalledPackageVersion{{Package: "cron", Version: "3.0pl1-197"}}}},
		})
		if list[0].Version != "unknown" {
			t.Errorf("%s: version = %q, want unknown", state, list[0].Version)
		}
	}
}
