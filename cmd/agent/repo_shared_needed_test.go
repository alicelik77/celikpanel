package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.121 (Boston, Ubuntu 24.04): PostgreSQL 18 from PGDG could not be
// installed where the distribution's PostgreSQL had been before, because
// postgresql-common and postgresql-client-common stayed at Ubuntu's 257
// under the confinement pin. Frankfurt (Debian 13, nothing before) worked:
// Debian's 278 satisfies postgresql-18's dependency.

// pgdgStanzas are PGDG's lists as apt.postgresql.org publishes them (the
// dependency lines that matter; codename suffix pg).
func pgdgStanzas(pg string) string {
	return fmt.Sprintf(`Package: postgresql-18
Version: 18.6-1.%[1]s+2
Depends: postgresql-client-18 (= 18.6-1.%[1]s+2), postgresql-common (>= 275~), libpq5 (>= 17~~), libc6 (>= 2.38), ssl-cert
Description: object-relational SQL database, version 18 server

Package: postgresql-client-18
Version: 18.6-1.%[1]s+2
Depends: libpq5 (>= 18~~), postgresql-client-common (>= 275~), libc6 (>= 2.38) | libc6.1, sensible-utils

Package: postgresql-common
Version: 287.%[1]s+1
Pre-Depends: adduser
Depends: postgresql-client-common (= 287.%[1]s+1), ssl-cert (>= 1.0.11), ucf

Package: postgresql-common
Version: 286.%[1]s+1
Depends: postgresql-client-common (= 286.%[1]s+1), ssl-cert (>= 1.0.11), ucf

Package: postgresql-client-common
Version: 287.%[1]s+1
Depends: netbase

Package: postgresql-client-common
Version: 286.%[1]s+1
Depends: netbase

Package: libpq5
Version: 18.1-1.%[1]s+1
Depends: libc6 (>= 2.38), libgssapi-krb5-2 (>= 1.17), libssl3t64 (>= 3.0.0)

Package: postgresql-17
Version: 17.7-1.%[1]s+1
Depends: postgresql-common (>= 252~), libpq5 (>= 17~~)
`, pg)
}

type pgHost struct {
	root string
	loc  aptSourceLocation
}

// pgdgHost writes a host whose PGDG repository is enabled and confined:
// its source, its lists, the distribution's lists and dpkg's status.
func pgdgHost(t *testing.T, codename, pg, distroFile, distro, status string) pgHost {
	t.Helper()
	root := confinementHost(t)
	sourcesDir := filepath.Join(root, "sources")
	if err := os.MkdirAll(sourcesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	restore := aptSourcesListDir
	aptSourcesListDir = sourcesDir
	t.Cleanup(func() { aptSourcesListDir = restore })
	line := "deb https://apt.postgresql.org/pub/repos/apt " + codename + "-pgdg main"
	writeTestFile(t, repoSourcePath("pgdg"), line+"\n")
	loc, err := parseAptSourceLine(line)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "lists", loc.listsPrefix()+"_main_binary-amd64_Packages"), pgdgStanzas(pg))
	writeTestFile(t, filepath.Join(root, "lists", distroFile), distro)
	writeTestFile(t, filepath.Join(root, "status"), status)
	return pgHost{root: root, loc: loc}
}

const bostonUbuntu = `Package: postgresql-common
Version: 257build1.1
Depends: postgresql-client-common (= 257build1.1), adduser

Package: postgresql-client-common
Version: 257build1.1

Package: libpq5
Version: 16.15-0ubuntu0.24.04.1

Package: postgresql-16
Version: 16.15-0ubuntu0.24.04.1

Package: php8.3-pgsql
Version: 8.3.6-0ubuntu0.24.04.5
Depends: libpq5 (>= 9.1~)

Package: libc6
Version: 2.39-0ubuntu8.6

Package: ssl-cert
Version: 1.1.2ubuntu1
`

// Boston after removing Ubuntu's PostgreSQL 16: postgresql-common,
// postgresql-client-common and libpq5 stayed (libpq5 serves php8.3-pgsql).
const bostonStatus = `Package: postgresql-common
Status: install ok installed
Version: 257build1.1

Package: postgresql-client-common
Status: install ok installed
Version: 257build1.1

Package: libpq5
Status: install ok installed
Version: 16.15-0ubuntu0.24.04.1

Package: php8.3-pgsql
Status: install ok installed
Version: 8.3.6-0ubuntu0.24.04.5

Package: libc6
Status: install ok installed
Version: 2.39-0ubuntu8.6
`

const frankfurtDebian = `Package: postgresql-common
Version: 278
Depends: postgresql-client-common (= 278)

Package: postgresql-client-common
Version: 278

Package: libpq5
Version: 17.6-0+deb13u1

Package: postgresql-17
Version: 17.6-0+deb13u1

Package: libc6
Version: 2.41-12
`

const frankfurtStatus = `Package: libc6
Status: install ok installed
Version: 2.41-12
`

func bostonHost(t *testing.T) pgHost {
	return pgdgHost(t, "noble", "pgdg24.04", "archive.ubuntu.com_ubuntu_dists_noble_main_binary-amd64_Packages", bostonUbuntu, bostonStatus)
}

func frankfurtHost(t *testing.T) pgHost {
	return pgdgHost(t, "trixie", "pgdg13", "deb.debian.org_debian_dists_trixie_main_binary-amd64_Packages", frankfurtDebian, frankfurtStatus)
}

func TestBostonNamesPGDGsCommonPackagesItsVersionNeeds(t *testing.T) {
	bostonHost(t)
	got := hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-18")
	want := []string{"libpq5=18.1-1.pgdg24.04+1", "postgresql-client-common=287.pgdg24.04+1", "postgresql-common=287.pgdg24.04+1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for _, arg := range got {
		if !validInstallArg(arg) {
			t.Fatalf("%q is not a valid install argument", arg)
		}
	}
	// The owner's 17 path on Boston stays exactly as it was: Ubuntu's
	// postgresql-common 257 satisfies postgresql-17 (>= 252~), so only
	// libpq5 is named, as before. 16 is Ubuntu's own: nothing is named.
	if got := hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-17"); !reflect.DeepEqual(got, []string{"libpq5=18.1-1.pgdg24.04+1"}) {
		t.Fatalf("postgresql-17 = %v", got)
	}
	if got := hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-16"); len(got) != 0 {
		t.Fatalf("postgresql-16 = %v", got)
	}
}

func TestFrankfurtKeepsDebiansCommonPackagesAsBefore(t *testing.T) {
	frankfurtHost(t)
	// Exactly what alpha.107 to 120 named: Debian's postgresql-common 278
	// satisfies postgresql-18, so it stays Debian's.
	got := hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-18")
	if !reflect.DeepEqual(got, []string{"libpq5=18.1-1.pgdg13+1"}) {
		t.Fatalf("args = %v", got)
	}
	// A major the distribution ships takes nothing from the vendor.
	if got := hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-17"); len(got) != 0 {
		t.Fatalf("distribution major = %v", got)
	}
}

func TestOnlyAllowedSharedPackagesAreEverNamed(t *testing.T) {
	in := vendorNeedInputs{
		allowed: map[string]bool{"postgresql-common": true},
		depends: map[string]map[string]string{
			"postgresql-18":     {"18.6-1": "postgresql-common (>= 275~), libc6 (>= 2.99)"},
			"postgresql-common": {"287": "", "270": ""},
			"libc6":             {"2.99-1": ""},
		},
		distro:    map[string]map[string]bool{"postgresql-common": {"257": true}, "libc6": {"2.39": true}},
		installed: map[string][]string{},
		named:     map[string]bool{},
	}
	// libc6 is unmet too, but it is not the repository's to replace: never
	// named (the simulation then says why); postgresql-common takes the
	// newest version that satisfies the relation.
	if got := vendorNeededSharedArgs("postgresql-18", in); !reflect.DeepEqual(got, []string{"postgresql-common=287"}) {
		t.Fatalf("args = %v", got)
	}
	// Installed from the vendor already: the confinement keeps it following
	// the vendor, nothing is named.
	in.installed = map[string][]string{"postgresql-common": {"270"}}
	if got := vendorNeededSharedArgs("postgresql-18", in); len(got) != 0 {
		t.Fatalf("vendor-installed = %v", got)
	}
}

func TestAptDependencyLinesAreReadAsAptReadsThem(t *testing.T) {
	groups := parseAptDependencyGroups("libc6 (>= 2.38) | libc6.1, postgresql-common (>= 275~), libpq5:any (<< 19) [amd64], debconf <!nocheck>")
	if len(groups) != 4 || len(groups[0]) != 2 || groups[1][0] != (aptDependency{"postgresql-common", ">=", "275~"}) ||
		groups[2][0] != (aptDependency{"libpq5", "<<", "19"}) || groups[3][0] != (aptDependency{Name: "debconf"}) {
		t.Fatalf("groups = %+v", groups)
	}
	for _, c := range []struct {
		dep     aptDependency
		version string
		want    bool
	}{
		{aptDependency{"x", ">=", "275~"}, "257build1.1", false},
		{aptDependency{"x", ">=", "275~"}, "278", true},
		{aptDependency{"x", ">=", "17~~"}, "16.15-0ubuntu0.24.04.1", false},
		{aptDependency{"x", ">=", "17~~"}, "17.6-0+deb13u1", true},
		{aptDependency{"x", "=", "287.pgdg24.04+1"}, "287.pgdg24.04+1", true},
		{aptDependency{"x", "<<", "19"}, "18.1-1", true},
		{aptDependency{Name: "x"}, "1", true},
	} {
		if got := c.dep.satisfiedBy(c.version); got != c.want {
			t.Fatalf("%+v by %s = %v", c.dep, c.version, got)
		}
	}
}

// bostonApt is a fake apt-get for the Boston host: it applies the
// confinement as apt does (the distribution at 500 wins unless a package is
// named name=version) and answers -s install with apt's own words, as the
// owner's `apt-get -s install postgresql-18` did on Boston.
func bostonApt(calls *[]string) packageSimulationRunner {
	return func(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
		*calls = append(*calls, strings.Join(args, " "))
		named := map[string]bool{}
		for _, arg := range args {
			if name, _, ok := strings.Cut(arg, "="); ok {
				named[name] = true
			}
		}
		var unmet []string
		if !named["libpq5"] {
			unmet = append(unmet, "Depends: libpq5 (>= 17~~) but 16.15-0ubuntu0.24.04.1 is to be installed")
		}
		if !named["postgresql-common"] {
			unmet = append(unmet, "Depends: postgresql-common (>= 275~) but 257build1.1 is to be installed")
		}
		if !named["postgresql-client-common"] {
			unmet = append([]string{"Depends: postgresql-client-18 (= 18.6-1.pgdg24.04+2) but it is not going to be installed"}, unmet...)
		}
		if len(unmet) > 0 {
			out := "Reading package lists...\nBuilding dependency tree...\nReading state information...\nSome packages could not be installed. This may mean that you have\nrequested an impossible situation or if you are using the unstable\ndistribution that some required packages have not yet been created\nor been moved out of Incoming.\nThe following information may help to resolve the situation:\n\nThe following packages have unmet dependencies:\n"
			out += " postgresql-18 : " + strings.Join(unmet, "\n                 ") + "\nE: Unable to correct problems, you have held broken packages.\n"
			return []byte(out), errors.New("exit status 100")
		}
		return []byte(`NOTE: This is only a simulation!
Inst libpq5 [16.15-0ubuntu0.24.04.1] (18.1-1.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64])
Inst postgresql-common [257build1.1] (287.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64]) []
Inst postgresql-client-common [257build1.1] (287.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64])
Inst postgresql-client-18 (18.6-1.pgdg24.04+2 apt.postgresql.org:noble-pgdg [amd64])
Inst postgresql-18 (18.6-1.pgdg24.04+2 apt.postgresql.org:noble-pgdg [amd64])
Conf libpq5 (18.1-1.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64])
Conf postgresql-common (287.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64])
Conf postgresql-client-common (287.pgdg24.04+1 apt.postgresql.org:noble-pgdg [amd64])
Conf postgresql-client-18 (18.6-1.pgdg24.04+2 apt.postgresql.org:noble-pgdg [amd64])
Conf postgresql-18 (18.6-1.pgdg24.04+2 apt.postgresql.org:noble-pgdg [amd64])
`), nil
	}
}

func bostonPreviewOps(calls *[]string) installPreviewOps {
	return installPreviewOps{
		family:              "apt",
		installedComponents: func() map[string]bool { return map[string]bool{} },
		installed:           func(string) bool { return false },
		available:           func(string) bool { return false },
		simulate: func(ctx context.Context, packages []string) (aptSimulation, error) {
			return runAptSimulation(ctx, bostonApt(calls), "/usr/bin/apt-get", aptInstallArgs(packages))
		},
		vendorOf:     hostVendorVersionLookup(),
		vendorShared: hostVendorSharedPackageArgs,
		lock:         func() bool { return true },
		unlock:       func() {},
	}
}

func TestTheOwnersBostonAptAnswerNamesTheUnmetPackages(t *testing.T) {
	bostonHost(t)
	var calls []string
	_, err := runAptSimulation(context.Background(), bostonApt(&calls), "/usr/bin/apt-get", aptInstallArgs([]string{"postgresql-18"}))
	var sim *aptSimulationError
	if !errors.As(err, &sim) || sim.Code != aptFailureUnmetDependencies ||
		!reflect.DeepEqual(sim.Packages, []string{"postgresql-client-18", "libpq5", "postgresql-common"}) {
		t.Fatalf("err = %#v", err)
	}
}

func TestBostonsPreviewNamesTheSharedUpgradesForConfirmation(t *testing.T) {
	bostonHost(t)
	var calls []string
	req := &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "postgresql", Package: "postgresql-18"}}}
	resp := emptyServiceInstallPreview()
	serviceInstallPreviewWithOps(context.Background(), req, &resp, bostonPreviewOps(&calls))
	if resp.State != "known" || resp.Error != "" || len(resp.Removes) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "postgresql-common=287.pgdg24.04+1") || !strings.Contains(calls[0], "postgresql-client-common=287.pgdg24.04+1") || !strings.Contains(calls[0], "libpq5=18.1-1.pgdg24.04+1") {
		t.Fatalf("simulated %v", calls)
	}
	var got []string
	for _, item := range resp.Notable {
		got = append(got, fmt.Sprintf("%s:%s:%s>%s:%s", item.Reason, item.Package, item.From, item.To, item.Component))
	}
	want := []string{
		"vendor_shared:libpq5:16.15-0ubuntu0.24.04.1>18.1-1.pgdg24.04+1:pgdg",
		"vendor_shared:postgresql-client-common:257build1.1>287.pgdg24.04+1:pgdg",
		"vendor_shared:postgresql-common:257build1.1>287.pgdg24.04+1:pgdg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("notable = %v", got)
	}
	// The install checks the same simulation against what was confirmed:
	// nothing confirmed refuses it, the three confirmed pass.
	requested := append([]string{"postgresql-18"}, hostVendorSharedPackageArgs(repoFromCatalogue("pgdg"), "postgresql-18")...)
	c := newInstallClassification("apt", requested, []string{"postgresql"}, nil, nil)
	c.vendorOf = hostVendorVersionLookup()
	sim, err := runAptSimulation(context.Background(), bostonApt(&calls), "/usr/bin/apt-get", aptInstallArgs(requested))
	if err != nil {
		t.Fatal(err)
	}
	var refusal *packageInstallNotableRefusal
	if err := notableInstallGuard(c, requested, nil)(sim); !errors.As(err, &refusal) {
		t.Fatalf("unconfirmed = %v", err)
	}
	if err := notableInstallGuard(c, requested, []string{"libpq5", "postgresql-common", "postgresql-client-common"})(sim); err != nil {
		t.Fatalf("confirmed = %v", err)
	}
}

// A package an enabled repository should offer and apt cannot find makes
// the preview refresh that repository's lists once and simulate again.
func TestAMissingPackageRefreshesItsRepositoryOnceAndSimulatesAgain(t *testing.T) {
	notFound := &aptSimulationError{message: "APT simulation failed", Code: aptFailurePackageNotFound, Packages: []string{"postgresql-18"}}
	run := func(refreshErr error, second error) (transport.ServiceInstallPreviewResponse, int, int) {
		simulations, refreshes := 0, 0
		ops := installPreviewOps{
			family:              "apt",
			installedComponents: func() map[string]bool { return map[string]bool{} },
			installed:           func(string) bool { return false },
			available:           func(string) bool { return false },
			simulate: func(context.Context, []string) (aptSimulation, error) {
				simulations++
				if simulations == 1 {
					return aptSimulation{}, notFound
				}
				return aptSimulation{Installed: []string{"postgresql-18"}}, second
			},
			lock:        func() bool { return true },
			unlock:      func() {},
			repoEnabled: func(id string) bool { return id == "pgdg" },
			refreshRepo: func(context.Context, string) error { refreshes++; return refreshErr },
		}
		req := &transport.ServiceInstallPreviewRequest{Steps: []transport.ServiceInstallPreviewStep{{ID: "postgresql", Package: "postgresql-18"}}}
		resp := emptyServiceInstallPreview()
		serviceInstallPreviewWithOps(context.Background(), req, &resp, ops)
		return resp, simulations, refreshes
	}
	resp, simulations, refreshes := run(nil, nil)
	if resp.State != "known" || simulations != 2 || refreshes != 1 || resp.ListsRefreshed != "pgdg" {
		t.Fatalf("refreshed: %+v %d %d", resp, simulations, refreshes)
	}
	resp, simulations, _ = run(nil, notFound)
	if resp.State != "failed" || resp.ErrorCode != aptFailurePackageNotFound || resp.ErrorRepo != "pgdg" || simulations != 2 {
		t.Fatalf("still missing: %+v", resp)
	}
	resp, simulations, _ = run(errors.New("apt-get update failed: exit status 100"), nil)
	if resp.ErrorCode != errCodeInstallPreviewListsRefreshFailed || resp.ErrorRepo != "pgdg" || simulations != 1 {
		t.Fatalf("refresh failed: %+v", resp)
	}
	resp, simulations, _ = run(errRepoListsRecentlyRefreshed, nil)
	if resp.ErrorCode != aptFailurePackageNotFound || resp.ErrorRepo != "pgdg" || simulations != 1 {
		t.Fatalf("recently refreshed: %+v", resp)
	}
	resp, _, _ = run(errRepoListsBusy, nil)
	if resp.State != "busy" {
		t.Fatalf("busy: %+v", resp)
	}
}

func TestARepositoryListRefreshIsSingleAndSpaced(t *testing.T) {
	bostonHost(t)
	runs := 0
	restoreRun, restoreNow := repoListsRefreshRun, repoListsRefreshNow
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	repoListsRefreshNow = func() time.Time { return now }
	var sources []string
	repoListsRefreshRun = func(_ context.Context, _ string, source string) ([]byte, error) {
		runs++
		sources = append(sources, source)
		return []byte("Hit:1 https://apt.postgresql.org/pub/repos/apt noble-pgdg InRelease\n"), nil
	}
	t.Cleanup(func() {
		repoListsRefreshRun, repoListsRefreshNow = restoreRun, restoreNow
		repoListsRefreshState.Lock()
		repoListsRefreshState.last = map[string]time.Time{}
		repoListsRefreshState.Unlock()
	})
	if err := refreshManagedRepoLists(context.Background(), "/usr/bin/apt-get", "pgdg"); err != nil {
		t.Fatal(err)
	}
	if err := refreshManagedRepoLists(context.Background(), "/usr/bin/apt-get", "pgdg"); !errors.Is(err, errRepoListsRecentlyRefreshed) {
		t.Fatalf("second = %v", err)
	}
	now = now.Add(repoListsRefreshInterval + time.Second)
	packageOperationMu.Lock()
	err := refreshManagedRepoLists(context.Background(), "/usr/bin/apt-get", "pgdg")
	packageOperationMu.Unlock()
	if !errors.Is(err, errRepoListsBusy) {
		t.Fatalf("under another package operation = %v", err)
	}
	if err := refreshManagedRepoLists(context.Background(), "/usr/bin/apt-get", "sury"); !errors.Is(err, errRepoNotEnabled) {
		t.Fatalf("not enabled = %v", err)
	}
	if runs != 1 || sources[0] != repoSourcePath("pgdg") {
		t.Fatalf("runs = %d sources = %v", runs, sources)
	}
}
