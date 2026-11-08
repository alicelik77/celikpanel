package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 lab (Ubuntu 24.04, Boston-like): after the automatic cleanup
// removed 16/main's files, dpkg still showed "rc postgresql-16" and
// "ii postgresql-client-16". The same job purges them through the alpha.104
// removal path; a simulation that would take anything else refuses it.

const bostonDpkgStatus = `Package: postgresql-16
Status: deinstall ok config-files
Version: 16.10-0ubuntu0.24.04.1

Package: postgresql-client-16
Status: install ok installed
Version: 16.10-0ubuntu0.24.04.1

Package: postgresql-18
Status: install ok installed
Version: 18.1-1.pgdg24.04+2

Package: postgresql-client-18
Status: install ok installed
Version: 18.1-1.pgdg24.04+2

Package: postgresql-client-common
Status: install ok installed
Version: 293.pgdg24.04+1

Package: postgresql-15-postgis-3
Status: deinstall ok config-files
Version: 3.5

Package: postgresql-15
Status: deinstall ok config-files
Version: 15.1
`

func TestPGLeftoverPackagesAreTheRemovedMajorsOnly(t *testing.T) {
	newPGLeftoverFixture(t)
	states, err := parseDpkgStates(strings.NewReader(bostonDpkgStatus))
	if err != nil {
		t.Fatal(err)
	}
	clusters := []transport.PostgreSQLCluster{{Version: "18", Name: "main"}, {Version: "15", Name: "kept"}}
	sets := pgLeftoverPackageSets(states, clusters)
	if len(sets) != 1 || len(sets["16"]) != 2 {
		t.Fatalf("sets = %+v (15 keeps a cluster, 18 runs)", sets)
	}
	if got := pgLeftoverPackageNames(sets); !reflect.DeepEqual(got, []string{"postgresql-16", "postgresql-client-16"}) {
		t.Fatalf("names = %v", got)
	}
	// With 15's cluster gone too, its rc packages count.
	sets = pgLeftoverPackageSets(states, clusters[:1])
	if got := pgLeftoverPackageNames(sets); !reflect.DeepEqual(got, []string{"postgresql-15", "postgresql-15-postgis-3", "postgresql-16", "postgresql-client-16"}) {
		t.Fatalf("names = %v", got)
	}
}

func TestPGLeftoverPackagesArePurgedThroughTheGuardedPath(t *testing.T) {
	newPGLeftoverFixture(t)
	states, _ := parseDpkgStates(strings.NewReader(bostonDpkgStatus))
	sets := pgLeftoverPackageSets(states, []transport.PostgreSQLCluster{{Version: "18", Name: "main"}, {Version: "15", Name: "kept"}})
	var requests [][]string
	pgLeftoverPurge = func(_ context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
		requests = append(requests, request.Packages)
		if request.KeepData || !reflect.DeepEqual(request.VersionPrefixes, []string{"postgresql-16"}) || request.ProtectedComponent == nil {
			t.Fatalf("request = %+v", request)
		}
		return packageRemovalPlan{Remove: request.Packages}, nil
	}
	purged, notes := purgePGLeftoverPackages(context.Background(), sets)
	if !reflect.DeepEqual(purged, []string{"postgresql-16", "postgresql-client-16"}) || len(notes) != 0 || len(requests) != 1 {
		t.Fatalf("purged=%v notes=%v requests=%v", purged, notes, requests)
	}
	// The client is still needed by another package: the simulation refuses,
	// the rc package alone is purged and the client stays, named.
	requests = nil
	pgLeftoverPurge = func(_ context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
		requests = append(requests, request.Packages)
		if len(request.Packages) > 1 {
			return packageRemovalPlan{}, &packageRemovalRefusal{Code: errCodeRemovalWouldRemoveOthers, Message: "it would also remove postgresql-client; nothing was removed"}
		}
		return packageRemovalPlan{Remove: request.Packages}, nil
	}
	purged, notes = purgePGLeftoverPackages(context.Background(), sets)
	if !reflect.DeepEqual(purged, []string{"postgresql-16"}) || len(notes) != 0 || len(requests) != 2 {
		t.Fatalf("retry: purged=%v notes=%v requests=%v", purged, notes, requests)
	}
	// Refused both times: nothing purged, said why.
	pgLeftoverPurge = func(context.Context, packageRemovalRequest) (packageRemovalPlan, error) {
		return packageRemovalPlan{}, &packageRemovalRefusal{Code: errCodeRemovalWouldRemoveOthers, Message: "it would also remove x"}
	}
	purged, notes = purgePGLeftoverPackages(context.Background(), sets)
	if len(purged) != 0 || len(notes) != 1 || !strings.Contains(notes[0], "would also remove x") {
		t.Fatalf("refused: purged=%v notes=%v", purged, notes)
	}
}

func TestPGLeftoverCleanupPurgesAfterTheClusterAndIsIdempotent(t *testing.T) {
	f := newPGLeftoverFixture(t)
	f.installMajor("18")
	fresh := f.cluster("16", "main", 5432, "down,binaries_missing")
	live := f.cluster("18", "main", 5433, "online")
	listed := pgLsLine(fresh) + pgLsLine(live)
	status := bostonDpkgStatus
	readDpkgStates = func() ([]dpkgPackageState, error) { return parseDpkgStates(strings.NewReader(status)) }
	purges := 0
	pgLeftoverPurge = func(_ context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
		purges++
		// The package manager's answer: the packages are gone afterwards.
		status = "Package: postgresql-18\nStatus: install ok installed\nVersion: 18\n"
		return packageRemovalPlan{Remove: request.Packages}, nil
	}
	host := pgClusterHost{
		family:     func() string { return "apt" },
		lsclusters: func(context.Context) (string, error) { return listed, nil },
	}
	var resp transport.PostgreSQLLeftoverCleanupResponse
	// The cluster is listed until its files go; the purge reads the list again.
	host.lsclusters = func(context.Context) (string, error) {
		out := listed
		listed = pgLsLine(live)
		return out, nil
	}
	cleanPGLeftovers(context.Background(), host, &resp)
	// 15 has no cluster either: its rc packages go too, one guarded purge
	// per major.
	if resp.Error != "" || len(resp.Removed) != 1 || purges != 2 ||
		!reflect.DeepEqual(resp.Purged, []string{"postgresql-15", "postgresql-15-postgis-3", "postgresql-16", "postgresql-client-16"}) {
		t.Fatalf("cleanup = %+v purges=%d", resp, purges)
	}
	resp = transport.PostgreSQLLeftoverCleanupResponse{}
	cleanPGLeftovers(context.Background(), host, &resp)
	if len(resp.Removed) != 0 || len(resp.Purged) != 0 || purges != 2 {
		t.Fatalf("second run = %+v purges=%d", resp, purges)
	}
}
