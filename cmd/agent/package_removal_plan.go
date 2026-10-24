package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A removal never removes anything it did not name (alpha.104, after the
// owner's Frankfurt incident of 2026-10-23). Removing PHP-FPM 8.4 ran
// `apt-get purge -y --auto-remove php8.4-fpm php8.4-cli php8.4-common …` and
// APT, without asking, also purged phpmyadmin, the distribution's php-*
// meta packages, dbconfig-common and about 35 libraries: everything that
// depended on the purged packages. The dialog had promised only that sites
// would be checked.
//
// The package manager's own simulation now decides what a removal takes,
// before anything changes. The expected set is:
//
//   - the requested packages;
//   - packages of the same runtime version that cannot work without it
//     (named "<version>-…", e.g. php8.4-readline next to php8.4-common, or
//     postgresql-17-… next to postgresql-17), even when APT removes them as
//     reverse dependencies;
//   - automatically installed packages that become unused only because of
//     this removal (APT's own autoremove answer after the removal, minus what
//     was already autoremovable before it, which is left alone).
//
// Anything else the simulation would remove refuses the removal with the
// names and nothing changes: a package of another catalogue component (or,
// for one version, the component's own distribution package such as the
// php-fpm meta package, which means this version is the system default), any
// other reverse dependency (a package that depends on what is removed, such
// as phpmyadmin or php-mysql), or anything the simulation would install.
// The real removal names the exact final list: it runs with the same
// arguments that were simulated last, under the same package lock, after a
// second simulation immediately before it confirms the same set.
//
// Bir kaldırma adını söylemediği hiçbir şeyi kaldırmaz. Paket yöneticisinin
// kendi benzetimi karar verir; beklenen küme dışındaki her şey kaldırmayı
// değişiklik yapmadan reddeder.

const (
	removalReasonComponent         = "component"
	removalReasonReverseDependency = "reverse_dependency"
	removalReasonWouldInstall      = "would_install"

	errCodeRemovalWouldRemoveOthers = "REMOVAL_WOULD_REMOVE_OTHERS"
	errCodeRemovalSimulationFailed  = "REMOVAL_SIMULATION_FAILED"
	errCodeRemovalPlanChanged       = "REMOVAL_PLAN_CHANGED"
	errCodeRemovalToolsBound        = "REMOVAL_TOOLS_BOUND"
)

// packageRemovalSimulationBound bounds every single simulation command.
const packageRemovalSimulationBound = 2 * time.Minute

// packageRemovalCollateralCap bounds the names a refusal carries.
const packageRemovalCollateralCap = 64

type packageRemovalRequest struct {
	// Packages are the packages the caller names for removal.
	Packages []string
	// VersionPrefixes name runtime versions being removed ("php8.4",
	// "postgresql-17"): packages named "<prefix>-…" belong to them.
	VersionPrefixes []string
	// ProtectedComponent returns the catalogue component a package belongs
	// to when it must never be removed as a side effect, or "".
	ProtectedComponent func(string) string
}

type packageRemovalPlan struct {
	Family    string
	Requested []string
	// Remove is the exact final list passed to the package manager.
	Remove []string
	// Collateral is what the simulation would remove (or install) outside
	// the expected set; a non-empty list refuses the removal.
	Collateral []transport.PackageRemovalCollateral
	// LeftAlone are packages that were already autoremovable before this
	// removal; they are not touched.
	LeftAlone []string
}

// packageRemovalRefusal is a typed refusal before (or, for a changed plan,
// right before) the package mutation.
type packageRemovalRefusal struct {
	Code       string
	Message    string
	Collateral []transport.PackageRemovalCollateral
	BoundTools []string
	// AfterStop: the answer changed after the units were stopped; they were
	// started again unless RestartFailed says why not.
	AfterStop     bool
	RestartFailed string
}

// removalHooks are the caller's steps around the real removal: stop the
// units first, and start them again when the removal is refused after they
// were stopped.
// removalHooks, gerçek kaldırmanın çevresindeki adımlardır.
type removalHooks struct {
	stop    func() error
	restart func() error
}

func (h removalHooks) runStop() error {
	if h.stop == nil {
		return nil
	}
	return h.stop()
}

func (h removalHooks) runRestart() error {
	if h.restart == nil {
		return nil
	}
	return h.restart()
}

func (r *packageRemovalRefusal) Error() string { return r.Message }

type packageSimulationRunner func(ctx context.Context, env []string, name string, args ...string) ([]byte, error)

var runPackageSimulationCommand packageSimulationRunner = runServiceMutationCombinedOutputEnv

func packageCommandEnv() []string {
	return append(os.Environ(), "LC_ALL=C", "LANG=C", "DEBIAN_FRONTEND=noninteractive")
}

// aptSimulation is what one `apt-get -s` run says it would do.
type aptSimulation struct {
	Removed   []string
	Installed []string
	// Upgrades (alpha.107) are the Inst lines of packages already installed:
	// "Inst name [old] (new …)".
	Upgrades map[string]aptUpgrade
}

// aptUpgrade is one installed package's version now and after.
type aptUpgrade struct {
	From, To string
}

// parseAptSimulation reads APT's simulation lines: "Purg name [version]",
// "Remv name [version]" and "Inst name …". Everything else (notes, the
// summary, "Conf" lines that follow an Inst) is ignored.
func parseAptSimulation(out []byte) aptSimulation {
	var sim aptSimulation
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "Purg", "Remv":
			if !seen["r:"+fields[1]] {
				seen["r:"+fields[1]] = true
				sim.Removed = append(sim.Removed, fields[1])
			}
		case "Inst":
			if !seen["i:"+fields[1]] {
				seen["i:"+fields[1]] = true
				sim.Installed = append(sim.Installed, fields[1])
				if len(fields) >= 4 && strings.HasPrefix(fields[2], "[") && strings.HasSuffix(fields[2], "]") && strings.HasPrefix(fields[3], "(") {
					if sim.Upgrades == nil {
						sim.Upgrades = map[string]aptUpgrade{}
					}
					sim.Upgrades[fields[1]] = aptUpgrade{
						From: strings.TrimSuffix(strings.TrimPrefix(fields[2], "["), "]"),
						To:   strings.TrimPrefix(fields[3], "("),
					}
				}
			}
		}
	}
	return sim
}

// validRemovalTarget accepts a package name, optionally qualified with an
// architecture as APT prints foreign-architecture packages ("libc6:i386").
func validRemovalTarget(name string) bool {
	base, arch, qualified := strings.Cut(name, ":")
	if !validPackageName(base) {
		return false
	}
	if !qualified {
		return true
	}
	if arch == "" || len(arch) > 32 {
		return false
	}
	for _, r := range arch {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func packageBaseName(name string) string {
	base, _, _ := strings.Cut(name, ":")
	return base
}

func aptSimulateArgs(args []string) []string {
	return append([]string{"-s"}, args...)
}

// aptFinalRemovalArgs is the one argument list of the real removal; its
// simulation is the same list behind "-s".
func aptFinalRemovalArgs(packages []string) []string {
	return append([]string{"purge", "-y", "-o", "APT::Get::AutomaticRemove=false"}, packages...)
}

func runAptSimulation(ctx context.Context, run packageSimulationRunner, aptGet string, args []string) (aptSimulation, error) {
	simCtx, cancel := context.WithTimeout(ctx, packageRemovalSimulationBound)
	defer cancel()
	out, err := run(simCtx, packageCommandEnv(), aptGet, aptSimulateArgs(args)...)
	if err != nil {
		return aptSimulation{}, fmt.Errorf("APT simulation failed: %v: %s", err, firstLine(lastLines(string(out), 3)))
	}
	return parseAptSimulation(out), nil
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" && (strings.HasPrefix(lines[i], "E:") || strings.HasPrefix(lines[i], "error")) {
			return lines[i]
		}
	}
	return strings.Join(lines, " ")
}

func (r packageRemovalRequest) belongsToRemovedVersion(name string) bool {
	base := packageBaseName(name)
	for _, prefix := range r.VersionPrefixes {
		if prefix != "" && strings.HasPrefix(base, prefix+"-") {
			return true
		}
	}
	return false
}

func (r packageRemovalRequest) protectedComponent(name string) string {
	if r.ProtectedComponent == nil {
		return ""
	}
	return r.ProtectedComponent(packageBaseName(name))
}

func validateRemovalRequest(request packageRemovalRequest) error {
	if len(request.Packages) == 0 {
		return fmt.Errorf("no packages to remove")
	}
	for _, p := range request.Packages {
		if !validPackageName(p) {
			return fmt.Errorf("invalid package name: %q", p)
		}
	}
	return nil
}

// planAptRemoval asks APT what removing request.Packages would do and
// classifies every package against the expected set.
func planAptRemoval(ctx context.Context, run packageSimulationRunner, aptGet string, request packageRemovalRequest) (packageRemovalPlan, error) {
	if err := validateRemovalRequest(request); err != nil {
		return packageRemovalPlan{}, err
	}
	requested := uniquePackageNames(request.Packages)
	plan := packageRemovalPlan{Family: "apt", Requested: requested}
	isRequested := map[string]bool{}
	for _, p := range requested {
		isRequested[p] = true
	}

	// 1. Without autoremove: the requested packages and their reverse
	// dependencies (everything that depends on them).
	plain, err := runAptSimulation(ctx, run, aptGet, append([]string{"purge", "-o", "APT::Get::AutomaticRemove=false"}, requested...))
	if err != nil {
		return plan, err
	}
	// 2. With autoremove: plus every automatically installed package that is
	// unused afterwards.
	full, err := runAptSimulation(ctx, run, aptGet, append([]string{"purge", "--auto-remove"}, requested...))
	if err != nil {
		return plan, err
	}
	// 3. What was already autoremovable before: never this removal's to take.
	before, err := runAptSimulation(ctx, run, aptGet, []string{"autoremove", "--purge"})
	if err != nil {
		return plan, err
	}

	inPlain := map[string]bool{}
	for _, p := range plain.Removed {
		inPlain[p] = true
	}
	alreadyOrphan := map[string]bool{}
	for _, p := range before.Removed {
		alreadyOrphan[p] = true
	}

	// A requested package APT does not remove (not installed) is left out of
	// the final command: naming it changes nothing.
	removedByAPT := map[string]bool{}
	for _, name := range append(append([]string{}, plain.Removed...), full.Removed...) {
		removedByAPT[packageBaseName(name)] = true
	}
	remove := []string{}
	for _, p := range requested {
		if removedByAPT[p] {
			remove = append(remove, p)
		}
	}
	extra := []string{}
	addCollateral := func(name, reason, component string) {
		plan.Collateral = append(plan.Collateral, transport.PackageRemovalCollateral{Package: name, Reason: reason, Component: component})
	}
	seen := map[string]bool{}
	for _, name := range append(append([]string{}, plain.Removed...), full.Removed...) {
		if seen[name] || isRequested[packageBaseName(name)] {
			continue
		}
		seen[name] = true
		if !validRemovalTarget(name) {
			return plan, fmt.Errorf("APT simulation named an unexpected package %q", name)
		}
		if inPlain[name] {
			// Removed because it depends on what is removed: only a package
			// of the removed version may go with it.
			if component := request.protectedComponent(name); component != "" {
				addCollateral(name, removalReasonComponent, component)
			} else if request.belongsToRemovedVersion(name) {
				extra = append(extra, name)
			} else {
				addCollateral(name, removalReasonReverseDependency, "")
			}
			continue
		}
		// An orphan is never a reason to refuse: it is removed only when it
		// became unused because of this removal and is not something the
		// server may still want — an orphan that was already unused, a
		// catalogue component's package, or a PHP extension package of
		// another PHP version (a site may load it: removing phpMyAdmin must
		// not take php8.4-mysql) is left alone.
		// Öksüz paket asla ret nedeni değildir; şüpheli olan yerinde bırakılır.
		if alreadyOrphan[name] || request.protectedComponent(name) != "" ||
			(phpVersionedPackage.MatchString(packageBaseName(name)) && !request.belongsToRemovedVersion(name)) {
			plan.LeftAlone = append(plan.LeftAlone, name)
			continue
		}
		extra = append(extra, name)
	}
	for _, sim := range []aptSimulation{plain, full} {
		for _, name := range sim.Installed {
			if !seen["inst:"+name] {
				seen["inst:"+name] = true
				addCollateral(name, removalReasonWouldInstall, "")
			}
		}
	}
	sort.Strings(extra)
	plan.Remove = append(remove, extra...)
	sortCollateral(plan.Collateral)
	if len(plan.Collateral) > 0 {
		return plan, nil
	}
	// 4. The exact final command, simulated: it must remove exactly the
	// planned list and install nothing.
	if err := verifyAptFinalSimulation(ctx, run, aptGet, plan.Remove); err != nil {
		return plan, err
	}
	return plan, nil
}

func sortCollateral(items []transport.PackageRemovalCollateral) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Reason != items[j].Reason {
			return collateralReasonRank(items[i].Reason) < collateralReasonRank(items[j].Reason)
		}
		return items[i].Package < items[j].Package
	})
}

func collateralReasonRank(reason string) int {
	switch reason {
	case removalReasonComponent:
		return 0
	case removalReasonReverseDependency:
		return 1
	default:
		return 2
	}
}

func sameNameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, name := range a {
		set[packageBaseName(name)] = true
	}
	for _, name := range b {
		if !set[packageBaseName(name)] {
			return false
		}
	}
	return true
}

func verifyAptFinalSimulation(ctx context.Context, run packageSimulationRunner, aptGet string, remove []string) error {
	if len(remove) == 0 {
		return nil
	}
	final, err := runAptSimulation(ctx, run, aptGet, aptFinalRemovalArgs(remove))
	if err != nil {
		return err
	}
	if len(final.Installed) > 0 || !sameNameSet(final.Removed, remove) {
		return &packageRemovalRefusal{
			Code: errCodeRemovalPlanChanged,
			Message: fmt.Sprintf(
				"the package manager's answer changed while the removal was being checked (it would now remove %s); nothing was removed — check again",
				strings.Join(capNames(final.Removed, 12), ", "),
			),
		}
	}
	return nil
}

// phpVersionedPackage matches a package of one PHP version ("php8.4-mysql").
var phpVersionedPackage = regexp.MustCompile(`^php[0-9]+\.[0-9]+-`)

// pacmanBreaksDependency matches pacman's refusal line
// ":: removing php breaks dependency 'php' required by phpmyadmin".
var pacmanBreaksDependency = regexp.MustCompile(`^:: removing (\S+) breaks dependency '[^']*' required by (\S+)$`)

func pacmanPrintRemovalArgs(packages []string) []string {
	return append([]string{"-Rns", "--print", "--print-format", "%n"}, packages...)
}

func pacmanRemovalArgs(packages []string) []string {
	return append([]string{"-Rns", "--noconfirm"}, packages...)
}

// planPacmanRemoval: pacman -Rns never removes a package that something else
// still requires (it refuses and names it) nor an explicitly installed
// dependency; its print mode lists the exact targets.
func planPacmanRemoval(ctx context.Context, run packageSimulationRunner, pacman string, request packageRemovalRequest) (packageRemovalPlan, error) {
	if err := validateRemovalRequest(request); err != nil {
		return packageRemovalPlan{}, err
	}
	requested := uniquePackageNames(request.Packages)
	plan := packageRemovalPlan{Family: "pacman", Requested: requested}
	targets, collateral, err := runPacmanPrint(ctx, run, pacman, requested)
	if err != nil {
		return plan, err
	}
	plan.Collateral = collateral
	isRequested := map[string]bool{}
	for _, p := range requested {
		isRequested[p] = true
	}
	for _, name := range targets {
		if isRequested[name] {
			continue
		}
		if component := request.protectedComponent(name); component != "" {
			plan.Collateral = append(plan.Collateral, transport.PackageRemovalCollateral{Package: name, Reason: removalReasonComponent, Component: component})
		}
	}
	sortCollateral(plan.Collateral)
	// pacman's own command names the requested packages; -s adds exactly the
	// printed dependencies.
	plan.Remove = append([]string{}, requested...)
	for _, name := range targets {
		if !isRequested[name] {
			plan.Remove = append(plan.Remove, name)
		}
	}
	return plan, nil
}

func runPacmanPrint(ctx context.Context, run packageSimulationRunner, pacman string, requested []string) ([]string, []transport.PackageRemovalCollateral, error) {
	simCtx, cancel := context.WithTimeout(ctx, packageRemovalSimulationBound)
	defer cancel()
	out, err := run(simCtx, packageCommandEnv(), pacman, pacmanPrintRemovalArgs(requested)...)
	var collateral []transport.PackageRemovalCollateral
	var targets []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if match := pacmanBreaksDependency.FindStringSubmatch(line); match != nil {
			if !seen["c:"+match[2]] && validPackageName(match[2]) {
				seen["c:"+match[2]] = true
				collateral = append(collateral, transport.PackageRemovalCollateral{Package: match[2], Reason: removalReasonReverseDependency})
			}
			continue
		}
		if err == nil && validPackageName(line) && !seen[line] {
			seen[line] = true
			targets = append(targets, line)
		}
	}
	if err != nil {
		if len(collateral) > 0 {
			return nil, collateral, nil
		}
		return nil, nil, fmt.Errorf("pacman simulation failed: %v: %s", err, firstLine(lastLines(string(out), 3)))
	}
	return targets, nil, nil
}

func capNames(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	out := append([]string{}, names[:n]...)
	return append(out, fmt.Sprintf("+%d more", len(names)-n))
}

// collateralRefusal turns a plan with collateral into the typed refusal.
func collateralRefusal(plan packageRemovalPlan) *packageRemovalRefusal {
	names := make([]string, 0, len(plan.Collateral))
	for _, item := range plan.Collateral {
		names = append(names, item.Package)
	}
	collateral := plan.Collateral
	if len(collateral) > packageRemovalCollateralCap {
		collateral = collateral[:packageRemovalCollateralCap]
	}
	return &packageRemovalRefusal{
		Code: errCodeRemovalWouldRemoveOthers,
		Message: fmt.Sprintf(
			"removing %s would also remove packages it did not name: %s; nothing was changed",
			strings.Join(plan.Requested, ", "),
			strings.Join(capNames(names, 12), ", "),
		),
		Collateral: collateral,
	}
}

// packageRemovalExecutor is the narrow seam the removal uses; production
// fills it from the verified host profile.
type packageRemovalExecutor struct {
	family string
	tool   string // verified apt-get or pacman path
	run    packageSimulationRunner
	mutate packageSimulationRunner
}

func (e packageRemovalExecutor) plan(ctx context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
	switch e.family {
	case "apt":
		return planAptRemoval(ctx, e.run, e.tool, request)
	case "pacman":
		return planPacmanRemoval(ctx, e.run, e.tool, request)
	case "dnf":
		return packageRemovalPlan{}, errDNFMutationCertificationPending
	default:
		return packageRemovalPlan{}, fmt.Errorf("unrecognised distribution; remove %s manually", strings.Join(request.Packages, ", "))
	}
}

// reverify repeats the final simulation right before the real removal.
func (e packageRemovalExecutor) reverify(ctx context.Context, plan packageRemovalPlan) error {
	switch e.family {
	case "apt":
		return verifyAptFinalSimulation(ctx, e.run, e.tool, plan.Remove)
	case "pacman":
		targets, collateral, err := runPacmanPrint(ctx, e.run, e.tool, plan.Requested)
		if err != nil {
			return err
		}
		if len(collateral) > 0 || !sameNameSet(uniquePackageNames(append(append([]string{}, plan.Requested...), targets...)), plan.Remove) {
			return &packageRemovalRefusal{
				Code:    errCodeRemovalPlanChanged,
				Message: "the package manager's answer changed while the removal was being checked; nothing was removed — check again",
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported package manager %q", e.family)
	}
}

func (e packageRemovalExecutor) remove(ctx context.Context, plan packageRemovalPlan) (string, error) {
	switch e.family {
	case "apt":
		if len(plan.Remove) == 0 {
			return "", nil
		}
		out, err := e.mutate(ctx, packageCommandEnv(), e.tool, aptFinalRemovalArgs(plan.Remove)...)
		if err != nil {
			return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	case "pacman":
		out, err := e.mutate(ctx, packageCommandEnv(), e.tool, pacmanRemovalArgs(plan.Requested)...)
		if err != nil {
			return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	default:
		return "", fmt.Errorf("unsupported package manager %q", e.family)
	}
}

func hostPackageRemovalExecutor(family string) (packageRemovalExecutor, error) {
	profile, err := verifiedHostProfile(family)
	if err != nil {
		return packageRemovalExecutor{}, err
	}
	executor := packageRemovalExecutor{family: family, run: runPackageSimulationCommand, mutate: runServiceMutationCombinedOutputEnv}
	switch family {
	case "apt":
		executor.tool, err = executableForProfile(profile, family, "apt-get")
	case "pacman":
		executor.tool, err = executableForProfile(profile, family, "pacman")
	case "dnf":
		return packageRemovalExecutor{}, errDNFMutationCertificationPending
	default:
		return packageRemovalExecutor{}, fmt.Errorf("unrecognised distribution")
	}
	return executor, err
}

// guardedRemovePackagesWith holds the package lock across the plan, the
// caller's pre-removal step (stopping units), the second simulation and the
// real removal. A plan with collateral refuses before beforeRemoval runs.
func guardedRemovePackagesWith(
	ctx context.Context,
	executor packageRemovalExecutor,
	request packageRemovalRequest,
	hooks removalHooks,
) (packageRemovalPlan, string, error) {
	packageOperationMu.Lock()
	defer packageOperationMu.Unlock()
	return guardedRemovePackagesLocked(ctx, executor, request, hooks)
}

func guardedRemovePackagesLocked(
	ctx context.Context,
	executor packageRemovalExecutor,
	request packageRemovalRequest,
	hooks removalHooks,
) (packageRemovalPlan, string, error) {
	plan, err := executor.plan(ctx, request)
	if err != nil {
		var refusal *packageRemovalRefusal
		if errors.As(err, &refusal) {
			return plan, "", err
		}
		if errors.Is(err, errDNFMutationCertificationPending) {
			return plan, "", err
		}
		return plan, "", &packageRemovalRefusal{
			Code:    errCodeRemovalSimulationFailed,
			Message: fmt.Sprintf("the package manager could not show what removing %s would take, so nothing was removed: %v", strings.Join(request.Packages, ", "), err),
		}
	}
	if len(plan.Collateral) > 0 {
		return plan, "", collateralRefusal(plan)
	}
	// The same answer once more before anything changes: a refusal here
	// means nothing was stopped or removed.
	// Hiçbir şey değişmeden önce aynı yanıt bir kez daha.
	if err := executor.reverify(ctx, plan); err != nil {
		return plan, "", err
	}
	if err := hooks.runStop(); err != nil {
		return plan, "", err
	}
	// And again right before the real removal, now that the units are
	// stopped; a changed answer starts them again and says so.
	// Ve unit'ler durduktan sonra, gerçek kaldırmadan hemen önce bir kez daha.
	if err := executor.reverify(ctx, plan); err != nil {
		var refusal *packageRemovalRefusal
		if !errors.As(err, &refusal) {
			refusal = &packageRemovalRefusal{Code: errCodeRemovalSimulationFailed, Message: err.Error()}
		}
		refusal.AfterStop = true
		if restartErr := hooks.runRestart(); restartErr != nil {
			refusal.RestartFailed = restartErr.Error()
			refusal.Message += fmt.Sprintf("; the services stopped for it could not be started again (%v): start them in Components", restartErr)
		} else {
			refusal.Message += "; the services stopped for it were started again"
		}
		return plan, "", refusal
	}
	out, err := executor.remove(ctx, plan)
	return plan, out, err
}

// guardedRemovePackagesContext is the production removal used by every
// package removal path.
func guardedRemovePackagesContext(ctx context.Context, family string, request packageRemovalRequest, hooks removalHooks) (packageRemovalPlan, string, error) {
	if err := validateRemovalRequest(request); err != nil {
		return packageRemovalPlan{}, "", err
	}
	executor, err := hostPackageRemovalExecutor(family)
	if err != nil {
		return packageRemovalPlan{}, "", err
	}
	return guardedRemovePackagesWith(ctx, executor, request, hooks)
}

// catalogueProtectedComponent returns the catalogue component that owns a
// package on this family, other than the packages being removed: every
// component's distribution packages and every versioned package matching a
// component's repository pattern. For a single-version removal the removed
// component's own distribution package (php-fpm, postgresql) is protected
// too: losing it means this version is the system default.
func catalogueProtectedComponent(family string, removing map[string]bool) func(string) string {
	type pattern struct {
		id string
		re *regexp.Regexp
	}
	owners := map[string]string{}
	var patterns []pattern
	for i := range core.ManagedServices {
		svc := &core.ManagedServices[i]
		for _, pkg := range svc.Packages[family] {
			if _, ok := owners[pkg]; !ok {
				owners[pkg] = svc.ID
			}
		}
		if family == "apt" && svc.Repo != nil && svc.Repo.PackagePattern != "" {
			if re, err := regexp.Compile(svc.Repo.PackagePattern); err == nil {
				patterns = append(patterns, pattern{svc.ID, re})
			}
		}
	}
	return func(pkg string) string {
		if removing[pkg] {
			return ""
		}
		if id, ok := owners[pkg]; ok {
			return id
		}
		for _, p := range patterns {
			if p.re.MatchString(pkg) {
				return p.id
			}
		}
		return ""
	}
}
