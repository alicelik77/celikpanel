package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The Frankfurt removal of 2026-10-23, as APT simulates it on Debian 13 with
// phpMyAdmin installed (format as `apt-get -s` prints it as root; the names
// are the ones the owner's apt history listed).
var frankfurtPHP84Packages = []string{
	"php8.4-fpm", "php8.4-cli", "php8.4-common", "php8.4-opcache", "php8.4-mysql",
	"php8.4-pgsql", "php8.4-sqlite3", "php8.4-mbstring", "php8.4-xml", "php8.4-zip", "php8.4-intl",
}

func purgLines(names ...string) string {
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "Purg %s [1.0-1]\n", name)
	}
	return b.String()
}

const aptSimulationHeader = "Reading package lists...\nBuilding dependency tree...\nReading state information...\nThe following packages will be REMOVED:\n  x*\n0 upgraded, 0 newly installed, 2 to remove and 0 not upgraded.\n"

// fakeAPT answers the four simulations and records every call.
type fakeAPT struct {
	mu        sync.Mutex
	plain     string
	full      string
	before    string
	final     func(args []string) string
	calls     [][]string
	deadlines []bool
	mutations [][]string
	failOn    string
}

func (f *fakeAPT) run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{}, args...))
	_, hasDeadline := ctx.Deadline()
	f.deadlines = append(f.deadlines, hasDeadline)
	joined := strings.Join(args, " ")
	if f.failOn != "" && strings.Contains(joined, f.failOn) {
		return []byte("E: Unable to locate package"), errors.New("exit status 100")
	}
	if !slices.Contains(env, "LC_ALL=C") {
		return nil, errors.New("simulation without LC_ALL=C")
	}
	switch {
	case len(args) > 1 && args[0] == "-s" && args[1] == "autoremove":
		return []byte(aptSimulationHeader + f.before), nil
	case strings.HasPrefix(joined, "-s purge --auto-remove"):
		return []byte(aptSimulationHeader + f.full), nil
	case strings.HasPrefix(joined, "-s purge -o APT::Get::AutomaticRemove=false"):
		return []byte(aptSimulationHeader + f.plain), nil
	case strings.HasPrefix(joined, "-s purge -y -o APT::Get::AutomaticRemove=false"):
		return []byte(aptSimulationHeader + f.final(args[5:])), nil
	}
	return nil, fmt.Errorf("unexpected simulation %q", joined)
}

func (f *fakeAPT) mutate(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mutations = append(f.mutations, append([]string{}, args...))
	return []byte("done"), nil
}

func exactFinal(args []string) string { return purgLines(args...) }

func frankfurtRequest() packageRemovalRequest {
	return packageRemovalRequest{
		Packages:           frankfurtPHP84Packages,
		VersionPrefixes:    []string{"php8.4"},
		ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet(frankfurtPHP84Packages)),
	}
}

func TestParseAptSimulationReadsPurgRemvAndInstLines(t *testing.T) {
	out := []byte(`NOTE: This is only a simulation!
Reading package lists...
The following packages will be REMOVED:
  curl* libcurl4t64*
0 upgraded, 0 newly installed, 2 to remove and 0 not upgraded.
Purg curl [8.14.1-2+deb13u4]
Remv libcurl4t64 [8.14.1-2+deb13u4]
Purg libc6:i386 [2.41-12]
Inst perl-modules-5.40 (5.40.1-6 Debian:13.6/stable [all])
Conf perl-modules-5.40 (5.40.1-6 Debian:13.6/stable [all])
Purg curl [8.14.1-2+deb13u4]
`)
	sim := parseAptSimulation(out)
	if want := []string{"curl", "libcurl4t64", "libc6:i386"}; !reflect.DeepEqual(sim.Removed, want) {
		t.Fatalf("removed = %v, want %v", sim.Removed, want)
	}
	if want := []string{"perl-modules-5.40"}; !reflect.DeepEqual(sim.Installed, want) {
		t.Fatalf("installed = %v, want %v", sim.Installed, want)
	}
}

func TestPlanAptRemovalRefusesTheFrankfurtCollateral(t *testing.T) {
	// Without autoremove APT removes the requested packages, everything that
	// depends on them (phpmyadmin, the php-* meta packages, php8.4-readline).
	plain := purgLines(append(append([]string{}, frankfurtPHP84Packages...),
		"php-fpm", "php-mysql", "php-mbstring", "php-pgsql", "php-xml", "php8.4-readline", "phpmyadmin")...)
	// With autoremove it adds the libraries only phpmyadmin needed.
	full := plain + purgLines("dbconfig-common", "dbconfig-mysql", "php-twig", "libjs-jquery")
	apt := &fakeAPT{plain: plain, full: full, before: "", final: exactFinal}

	plan, err := planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", frankfurtRequest())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]transport.PackageRemovalCollateral{}
	for _, item := range plan.Collateral {
		got[item.Package] = item
	}
	for name, want := range map[string]transport.PackageRemovalCollateral{
		"phpmyadmin":   {Package: "phpmyadmin", Reason: removalReasonComponent, Component: "phpmyadmin"},
		"php-fpm":      {Package: "php-fpm", Reason: removalReasonComponent, Component: "php-fpm"},
		"php-mysql":    {Package: "php-mysql", Reason: removalReasonReverseDependency},
		"php-mbstring": {Package: "php-mbstring", Reason: removalReasonReverseDependency},
		"php-pgsql":    {Package: "php-pgsql", Reason: removalReasonReverseDependency},
		"php-xml":      {Package: "php-xml", Reason: removalReasonReverseDependency},
	} {
		if got[name] != want {
			t.Fatalf("collateral[%s] = %+v, want %+v (all: %+v)", name, got[name], want, plan.Collateral)
		}
	}
	if _, ok := got["php8.4-readline"]; ok {
		t.Fatal("a package of the removed version was named as collateral")
	}
	if len(plan.Collateral) != 6 {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}
	// Components first, then reverse dependencies.
	if plan.Collateral[0].Reason != removalReasonComponent || plan.Collateral[5].Reason != removalReasonReverseDependency {
		t.Fatalf("collateral order = %+v", plan.Collateral)
	}
	refusal := collateralRefusal(plan)
	if refusal.Code != errCodeRemovalWouldRemoveOthers || !strings.Contains(refusal.Message, "phpmyadmin") || !strings.Contains(refusal.Message, "nothing was changed") {
		t.Fatalf("refusal = %+v", refusal)
	}
}

func TestPlanAptRemovalAcceptsVersionPackagesAndNewOrphansAndLeavesOldOrphansAlone(t *testing.T) {
	requested := []string{"php8.5-fpm", "php8.5-cli", "php8.5-common"}
	plain := purgLines("php8.5-fpm", "php8.5-cli", "php8.5-common", "php8.5-readline", "php8.5-xml")
	full := plain + purgLines("libonig5", "linux-image-6.12.38+deb13-amd64")
	before := purgLines("linux-image-6.12.38+deb13-amd64")
	apt := &fakeAPT{plain: plain, full: full, before: before, final: exactFinal}
	request := packageRemovalRequest{
		Packages:           requested,
		VersionPrefixes:    []string{"php8.5"},
		ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet(requested)),
	}
	plan, err := planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collateral) != 0 {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}
	want := []string{"php8.5-fpm", "php8.5-cli", "php8.5-common", "libonig5", "php8.5-readline", "php8.5-xml"}
	if !reflect.DeepEqual(plan.Remove, want) {
		t.Fatalf("remove = %v, want %v", plan.Remove, want)
	}
	if !reflect.DeepEqual(plan.LeftAlone, []string{"linux-image-6.12.38+deb13-amd64"}) {
		t.Fatalf("left alone = %v", plan.LeftAlone)
	}
	last := apt.calls[len(apt.calls)-1]
	if !reflect.DeepEqual(last, aptSimulateArgs(aptFinalRemovalArgs(want))) {
		t.Fatalf("final simulation = %v", last)
	}
	for i, bounded := range apt.deadlines {
		if !bounded {
			t.Fatalf("simulation %d (%v) ran without a deadline", i, apt.calls[i])
		}
	}
}

func TestPlanAptRemovalTreatsAnInstallAsCollateral(t *testing.T) {
	plain := purgLines("php8.4-fpm") + "Inst php8.3-fpm (8.3.1-1 Debian:13/stable [amd64])\nConf php8.3-fpm (8.3.1-1 Debian:13/stable [amd64])\n"
	apt := &fakeAPT{plain: plain, full: plain, final: exactFinal}
	plan, err := planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", packageRemovalRequest{Packages: []string{"php8.4-fpm"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collateral) != 1 || plan.Collateral[0].Reason != removalReasonWouldInstall || plan.Collateral[0].Package != "php8.3-fpm" {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}
}

func TestPlanAptRemovalLeavesNotInstalledRequestedPackagesOut(t *testing.T) {
	plain := purgLines("redis-server")
	apt := &fakeAPT{plain: plain, full: plain, final: exactFinal}
	plan, err := planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", packageRemovalRequest{Packages: []string{"redis-server", "redis"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"redis-server"}) {
		t.Fatalf("remove = %v", plan.Remove)
	}
}

func TestGuardedRemovalRefusesCollateralBeforeAnyMutation(t *testing.T) {
	plain := purgLines(append(append([]string{}, frankfurtPHP84Packages...), "phpmyadmin")...)
	apt := &fakeAPT{plain: plain, full: plain, final: exactFinal}
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	beforeCalls := 0
	_, _, err := guardedRemovePackagesWith(context.Background(), executor, frankfurtRequest(), removalHooks{stop: func() error {
		beforeCalls++
		return nil
	}})
	var refusal *packageRemovalRefusal
	if !errors.As(err, &refusal) || refusal.Code != errCodeRemovalWouldRemoveOthers {
		t.Fatalf("err = %v", err)
	}
	if beforeCalls != 0 || len(apt.mutations) != 0 {
		t.Fatalf("mutation ran: before=%d mutations=%v", beforeCalls, apt.mutations)
	}
	if len(refusal.Collateral) != 1 || refusal.Collateral[0].Package != "phpmyadmin" {
		t.Fatalf("collateral = %+v", refusal.Collateral)
	}
}

func TestGuardedRemovalRunsExactlyTheSimulatedArgumentsUnderTheLock(t *testing.T) {
	requested := []string{"redis-server"}
	plain := purgLines("redis-server")
	full := plain + purgLines("libjemalloc2")
	apt := &fakeAPT{plain: plain, full: full, final: exactFinal}
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	lockedDuringBefore := false
	plan, _, err := guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: requested}, removalHooks{stop: func() error {
		lockedDuringBefore = !packageOperationMu.TryLock()
		if !lockedDuringBefore {
			packageOperationMu.Unlock()
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !lockedDuringBefore {
		t.Fatal("the package lock was not held across the pre-removal step")
	}
	if !packageOperationMu.TryLock() {
		t.Fatal("the package lock was not released")
	}
	packageOperationMu.Unlock()
	if len(apt.mutations) != 1 {
		t.Fatalf("mutations = %v", apt.mutations)
	}
	real := apt.mutations[0]
	if want := aptFinalRemovalArgs([]string{"redis-server", "libjemalloc2"}); !reflect.DeepEqual(real, want) {
		t.Fatalf("real removal = %v, want %v", real, want)
	}
	// The last simulation before the real removal is the same list behind -s.
	lastSim := apt.calls[len(apt.calls)-1]
	if !reflect.DeepEqual(lastSim, aptSimulateArgs(real)) {
		t.Fatalf("last simulation %v is not the real removal %v behind -s", lastSim, real)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"redis-server", "libjemalloc2"}) {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestGuardedRemovalRefusesWhenTheAnswerChangesBeforeTheRealRemoval(t *testing.T) {
	plain := purgLines("redis-server")
	finalCalls := 0
	apt := &fakeAPT{plain: plain, full: plain, final: func(args []string) string {
		finalCalls++
		if finalCalls == 1 {
			return purgLines(args...)
		}
		// The owner installed something in between that now depends on it.
		return purgLines(append(args, "owner-tool")...)
	}}
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	_, _, err := guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: []string{"redis-server"}}, removalHooks{stop: func() error { t.Fatal("units stopped although the answer had changed"); return nil }})
	var refusal *packageRemovalRefusal
	if !errors.As(err, &refusal) || refusal.Code != errCodeRemovalPlanChanged || !strings.Contains(refusal.Message, "owner-tool") {
		t.Fatalf("err = %v", err)
	}
	if len(apt.mutations) != 0 {
		t.Fatalf("real removal ran: %v", apt.mutations)
	}
}

func TestGuardedRemovalSimulationFailureRefusesWithoutMutation(t *testing.T) {
	apt := &fakeAPT{failOn: "--auto-remove", plain: purgLines("x"), final: exactFinal}
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	beforeCalls := 0
	_, _, err := guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: []string{"x"}}, removalHooks{stop: func() error { beforeCalls++; return nil }})
	var refusal *packageRemovalRefusal
	if !errors.As(err, &refusal) || refusal.Code != errCodeRemovalSimulationFailed {
		t.Fatalf("err = %v", err)
	}
	if beforeCalls != 0 || len(apt.mutations) != 0 {
		t.Fatal("mutation after a failed simulation")
	}
}

type fakePacman struct {
	out       string
	err       error
	calls     [][]string
	mutations [][]string
}

func (f *fakePacman) run(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{}, args...))
	return []byte(f.out), f.err
}

func (f *fakePacman) mutate(_ context.Context, _ []string, _ string, args ...string) ([]byte, error) {
	f.mutations = append(f.mutations, append([]string{}, args...))
	return nil, nil
}

func TestPlanPacmanRemovalNamesWhatBreaksAndProtectedDependencies(t *testing.T) {
	broken := &fakePacman{
		out: "checking dependencies...\nerror: failed to prepare transaction (could not satisfy dependencies)\n:: removing php breaks dependency 'php' required by phpmyadmin\n",
		err: errors.New("exit status 1"),
	}
	plan, err := planPacmanRemoval(context.Background(), broken.run, "/usr/bin/pacman", packageRemovalRequest{Packages: []string{"php-fpm", "php"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collateral) != 1 || plan.Collateral[0].Package != "phpmyadmin" || plan.Collateral[0].Reason != removalReasonReverseDependency {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}
	if want := pacmanPrintRemovalArgs([]string{"php-fpm", "php"}); !reflect.DeepEqual(broken.calls[0], want) {
		t.Fatalf("print args = %v", broken.calls[0])
	}

	protected := &fakePacman{out: "redis\nnginx\nlibfoo\n"}
	request := packageRemovalRequest{Packages: []string{"redis"}, ProtectedComponent: catalogueProtectedComponent("pacman", packageNameSet([]string{"redis"}))}
	plan, err = planPacmanRemoval(context.Background(), protected.run, "/usr/bin/pacman", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collateral) != 1 || plan.Collateral[0].Package != "nginx" || plan.Collateral[0].Component != "nginx" {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}

	clean := &fakePacman{out: "redis\nlibfoo\n"}
	executor := packageRemovalExecutor{family: "pacman", tool: "/usr/bin/pacman", run: clean.run, mutate: clean.mutate}
	plan, _, err = guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: []string{"redis"}}, removalHooks{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"redis", "libfoo"}) {
		t.Fatalf("remove = %v", plan.Remove)
	}
	if len(clean.mutations) != 1 || !reflect.DeepEqual(clean.mutations[0], pacmanRemovalArgs([]string{"redis"})) {
		t.Fatalf("mutations = %v", clean.mutations)
	}
	// The plan, once more before the units stop, and again right before the removal.
	if len(clean.calls) != 3 {
		t.Fatalf("pacman was not simulated again before and after the stop: %v", clean.calls)
	}
}

func TestCatalogueProtectedComponent(t *testing.T) {
	protect := catalogueProtectedComponent("apt", packageNameSet([]string{"php8.4-fpm"}))
	for pkg, want := range map[string]string{
		"phpmyadmin":     "phpmyadmin",
		"phppgadmin":     "phppgadmin",
		"php-fpm":        "php-fpm",
		"php8.5-fpm":     "php-fpm",
		"postgresql-17":  "postgresql",
		"php8.4-fpm":     "",
		"php-mysql":      "",
		"dbconfig-mysql": "",
	} {
		if got := protect(pkg); got != want {
			t.Fatalf("protected(%s) = %q, want %q", pkg, got, want)
		}
	}
}

func TestUninstallRefusesCollateralBeforeStoppingUnits(t *testing.T) {
	disabled := 0
	installed := map[string]bool{"php8.4-fpm": true, "php8.4-cli": true, "php8.4-mysql": true}
	var requested packageRemovalRequest
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return installed[pkg] },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(string) error { disabled++; return nil },
		removePackages: func(_ string, request packageRemovalRequest, before removalHooks) (packageRemovalPlan, string, error) {
			requested = request
			return packageRemovalPlan{}, "", &packageRemovalRefusal{
				Code:       errCodeRemovalWouldRemoveOthers,
				Message:    "removing php8.4-fpm would also remove packages it did not name: phpmyadmin; nothing was changed",
				Collateral: []transport.PackageRemovalCollateral{{Package: "phpmyadmin", Reason: removalReasonComponent, Component: "phpmyadmin"}},
			}
		},
		boundPHPTools: func(string) []string { return nil },
	}
	var resp UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "php-fpm", Package: "php8.4-fpm"}, &resp, ops); err != nil {
		t.Fatal(err)
	}
	if disabled != 0 || resp.MutationApplied || resp.PartialSuccess || resp.Removed {
		t.Fatalf("refusal changed the host: disabled=%d resp=%+v", disabled, resp)
	}
	if resp.ErrorCode != errCodeRemovalWouldRemoveOthers || len(resp.Collateral) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	if !reflect.DeepEqual(requested.Packages, []string{"php8.4-fpm", "php8.4-cli", "php8.4-mysql"}) || !reflect.DeepEqual(requested.VersionPrefixes, []string{"php8.4"}) {
		t.Fatalf("request = %+v", requested)
	}
	if requested.ProtectedComponent("php-fpm") != "php-fpm" || requested.ProtectedComponent("phpmyadmin") != "phpmyadmin" {
		t.Fatal("the version removal does not protect the distribution package or other components")
	}
}

func TestUninstallRefusesAPHPVersionAToolRunsOn(t *testing.T) {
	removeCalls := 0
	var askedVersion string
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "php8.4-fpm" },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(string) error { t.Fatal("unit stopped"); return nil },
		removePackages: func(string, packageRemovalRequest, removalHooks) (packageRemovalPlan, string, error) {
			removeCalls++
			return packageRemovalPlan{}, "", nil
		},
		boundPHPTools: func(version string) []string {
			askedVersion = version
			return []string{"phppgadmin", "webmail"}
		},
	}
	var resp UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "php-fpm", Package: "php8.4-fpm"}, &resp, ops); err != nil {
		t.Fatal(err)
	}
	if askedVersion != "8.4" || removeCalls != 0 || resp.MutationApplied {
		t.Fatalf("asked=%q removeCalls=%d resp=%+v", askedVersion, removeCalls, resp)
	}
	if resp.ErrorCode != errCodeRemovalToolsBound || !reflect.DeepEqual(resp.BoundTools, []string{"phppgadmin", "webmail"}) {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.Contains(resp.Error, "phpPgAdmin, Webmail (Roundcube) run on PHP 8.4") || !strings.Contains(resp.Error, "move them to another PHP version first") {
		t.Fatalf("error = %q", resp.Error)
	}
}

func TestServiceRemovalPreviewIsReadOnlyAndDoesNotWaitForTheLock(t *testing.T) {
	plain := purgLines(append(append([]string{}, "php8.4-fpm"), "phpmyadmin")...)
	apt := &fakeAPT{plain: plain, full: plain, final: exactFinal}
	executorFor := func(family string) (packageRemovalExecutor, error) {
		return packageRemovalExecutor{family: family, tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}, nil
	}
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "php8.4-fpm" },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		boundPHPTools:       func(string) []string { return []string{"webmail"} },
	}
	request := &transport.ServiceRemovalPreviewRequest{ID: "php-fpm", Package: "php8.4-fpm"}

	resp := emptyServiceRemovalPreview()
	serviceRemovalPreviewWithOps(context.Background(), request, &resp, ops, executorFor)
	if resp.State != "checked" || !reflect.DeepEqual(resp.BoundTools, []string{"webmail"}) || len(resp.Collateral) != 1 || resp.Collateral[0].Package != "phpmyadmin" {
		t.Fatalf("preview = %+v", resp)
	}
	if len(apt.mutations) != 0 {
		t.Fatal("preview mutated")
	}

	packageOperationMu.Lock()
	busy := emptyServiceRemovalPreview()
	calls := len(apt.calls)
	serviceRemovalPreviewWithOps(context.Background(), request, &busy, ops, executorFor)
	packageOperationMu.Unlock()
	if busy.State != "busy" || len(apt.calls) != calls {
		t.Fatalf("busy preview = %+v (simulations %d → %d)", busy, calls, len(apt.calls))
	}
}

func TestGuardedRemovalChangedAfterTheStopStartsTheUnitsAgain(t *testing.T) {
	plain := purgLines("redis-server")
	finalCalls := 0
	apt := &fakeAPT{plain: plain, full: plain, final: func(args []string) string {
		finalCalls++
		if finalCalls <= 2 {
			return purgLines(args...)
		}
		return purgLines(append(args, "owner-tool")...)
	}}
	executor := packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	stops, restarts := 0, 0
	_, _, err := guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: []string{"redis-server"}}, removalHooks{
		stop:    func() error { stops++; return nil },
		restart: func() error { restarts++; return nil },
	})
	var refusal *packageRemovalRefusal
	if !errors.As(err, &refusal) || refusal.Code != errCodeRemovalPlanChanged || !refusal.AfterStop || refusal.RestartFailed != "" {
		t.Fatalf("err = %v (%+v)", err, refusal)
	}
	if stops != 1 || restarts != 1 || len(apt.mutations) != 0 || !strings.Contains(refusal.Message, "were started again") {
		t.Fatalf("stops=%d restarts=%d mutations=%v message=%q", stops, restarts, apt.mutations, refusal.Message)
	}
	apt = &fakeAPT{plain: plain, full: plain, final: func(args []string) string {
		finalCalls++
		return purgLines(append(args, "owner-tool")...)
	}}
	finalCalls = 0
	apt.final = func(args []string) string {
		finalCalls++
		if finalCalls <= 2 {
			return purgLines(args...)
		}
		return purgLines(append(args, "owner-tool")...)
	}
	executor = packageRemovalExecutor{family: "apt", tool: "/usr/bin/apt-get", run: apt.run, mutate: apt.mutate}
	_, _, err = guardedRemovePackagesWith(context.Background(), executor, packageRemovalRequest{Packages: []string{"redis-server"}}, removalHooks{
		stop:    func() error { return nil },
		restart: func() error { return errors.New("redis-server did not start") },
	})
	if !errors.As(err, &refusal) || refusal.RestartFailed == "" || !strings.Contains(refusal.Message, "could not be started again") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrphansThatSitesOrComponentsMayUseAreLeftAlone(t *testing.T) {
	// Removing phpMyAdmin: its PHP 8.4 extension packages become unused by
	// it, but a site may load them; an old catalogue orphan is no refusal.
	plain := purgLines("phpmyadmin")
	full := plain + purgLines("php8.4-mysql", "php8.4-mbstring", "php8.4-xml", "php-twig", "dbconfig-mysql", "redis-server")
	before := purgLines("redis-server")
	apt := &fakeAPT{plain: plain, full: full, before: before, final: exactFinal}
	request := packageRemovalRequest{
		Packages:           []string{"phpmyadmin"},
		ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet([]string{"phpmyadmin"})),
	}
	plan, err := planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collateral) != 0 {
		t.Fatalf("collateral = %+v", plan.Collateral)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"phpmyadmin", "dbconfig-mysql", "php-twig"}) {
		t.Fatalf("remove = %v", plan.Remove)
	}
	for _, kept := range []string{"php8.4-mysql", "php8.4-mbstring", "php8.4-xml", "redis-server"} {
		if !slices.Contains(plan.LeftAlone, kept) {
			t.Fatalf("%s was not left alone: %v", kept, plan.LeftAlone)
		}
	}
	// A catalogue package that is a new orphan is left alone, not a refusal.
	full = plain + purgLines("nginx")
	apt = &fakeAPT{plain: plain, full: full, final: exactFinal}
	plan, err = planAptRemoval(context.Background(), apt.run, "/usr/bin/apt-get", request)
	if err != nil || len(plan.Collateral) != 0 || !slices.Contains(plan.LeftAlone, "nginx") {
		t.Fatalf("plan = %+v err = %v", plan, err)
	}
}

func TestAnInstallThatWouldRemovePackagesIsRefused(t *testing.T) {
	calls := 0
	run := func(_ context.Context, env []string, _ string, args ...string) ([]byte, error) {
		calls++
		if args[0] != "-s" || args[1] != "install" {
			t.Fatalf("args = %v", args)
		}
		return []byte("Remv apache2 [2.4.62-1]\nInst nginx (1.26.3-3 Debian:13/stable [amd64])\n"), nil
	}
	err := refuseInstallThatRemoves(context.Background(), run, "/usr/bin/apt-get", []string{"nginx"}, []string{"install", "-y", "--no-install-recommends", "nginx"})
	var refusal *packageInstallRefusal
	if !errors.As(err, &refusal) || !reflect.DeepEqual(refusal.Removes, []string{"apache2"}) || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("err = %v", err)
	}
	clean := func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte("Inst php8.5-mysql (8.5.0-1 Sury:trixie [amd64])\n"), nil
	}
	if err := refuseInstallThatRemoves(context.Background(), clean, "/usr/bin/apt-get", []string{"php8.5-mysql"}, []string{"install", "-y", "php8.5-mysql"}); err != nil {
		t.Fatal(err)
	}
	failing := func(context.Context, []string, string, ...string) ([]byte, error) {
		return []byte("E: Unable to locate package x"), errors.New("exit status 100")
	}
	if err := refuseInstallThatRemoves(context.Background(), failing, "/usr/bin/apt-get", []string{"x"}, []string{"install", "-y", "x"}); err == nil || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("err = %v", err)
	}
}

// A refusal after the stop puts back each unit exactly as the owner left it:
// a unit the owner had disabled or stopped is not enabled or started; an
// alias name is put back by its real unit (alpha.104 review).
func TestALateRefusalPutsBackEachUnitAsTheOwnerLeftIt(t *testing.T) {
	recorded := map[string]unitRuntimeState{
		"php8.4-fpm": {ID: "php8.4-fpm.service", Enabled: true, Active: true},
	}
	var restored []unitRuntimeState
	ops := serviceUninstallOps{
		detectPackageFamily: func() string { return "apt" },
		packageInstalled:    func(pkg string) bool { return pkg == "php8.4-fpm" },
		unitExists:          func(string) bool { return true },
		unitsMatching:       func(string) []string { return nil },
		disableUnit:         func(string) error { return nil },
		unitState:           func(unit string) (unitRuntimeState, error) { return recorded[unit], nil },
		restoreUnit:         func(state unitRuntimeState) error { restored = append(restored, state); return nil },
		removePackages: func(_ string, _ packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
			if err := hooks.runStop(); err != nil {
				return packageRemovalPlan{}, "", err
			}
			restartErr := hooks.runRestart()
			refusal := &packageRemovalRefusal{Code: errCodeRemovalPlanChanged, Message: "changed; nothing was removed", AfterStop: true}
			if restartErr != nil {
				refusal.RestartFailed = restartErr.Error()
			}
			return packageRemovalPlan{}, "", refusal
		},
		boundPHPTools: func(string) []string { return nil },
	}
	var resp UninstallServiceResponse
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "php-fpm", Package: "php8.4-fpm"}, &resp, ops); err != nil {
		t.Fatal(err)
	}
	if resp.ErrorCode != errCodeRemovalPlanChanged || resp.MutationApplied || len(restored) != 1 || restored[0] != recorded["php8.4-fpm"] {
		t.Fatalf("resp=%+v restored=%+v", resp, restored)
	}
	// The commands put back exactly the recorded state.
	cases := []struct {
		state unitRuntimeState
		want  [][]string
	}{
		{unitRuntimeState{ID: "redis-server.service", Enabled: true, Active: true}, [][]string{{"enable", "redis-server.service"}, {"start", "redis-server.service"}}},
		{unitRuntimeState{ID: "redis-server.service", Enabled: false, Active: true}, [][]string{{"start", "redis-server.service"}}},
		{unitRuntimeState{ID: "redis-server.service", Enabled: true, Active: false}, [][]string{{"enable", "redis-server.service"}}},
		{unitRuntimeState{ID: "redis-server.service"}, nil},
	}
	for _, c := range cases {
		if got := unitRestoreCommands(c.state); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%+v: %v", c.state, got)
		}
	}
	// An alias reports its real unit; masked or disabled is not "enabled".
	state, err := parseUnitRuntimeState("redis", "Id=redis-server.service\nUnitFileState=enabled\nActiveState=inactive\n")
	if err != nil || state != (unitRuntimeState{ID: "redis-server.service", Enabled: true}) {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	state, _ = parseUnitRuntimeState("x", "Id=x.service\nUnitFileState=masked\nActiveState=active\n")
	if state.Enabled || !state.Active {
		t.Fatalf("masked = %+v", state)
	}
	if _, err := parseUnitRuntimeState("x", "Id=x.service\n"); err == nil {
		t.Fatal("a partial answer was taken as the state")
	}
	// A state that cannot be read refuses before anything stops.
	stopped := 0
	ops.disableUnit = func(string) error { stopped++; return nil }
	ops.unitState = func(string) (unitRuntimeState, error) { return unitRuntimeState{}, errors.New("bus") }
	resp = UninstallServiceResponse{}
	if err := (&Agent{}).uninstallServiceWithOps(&InstallServiceRequest{ID: "php-fpm", Package: "php8.4-fpm"}, &resp, ops); err != nil {
		t.Fatal(err)
	}
	if stopped != 0 || !strings.Contains(resp.Error, "nothing was changed") || resp.MutationApplied {
		t.Fatalf("stopped=%d resp=%+v", stopped, resp)
	}
}
