package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Unexpected installs are shown before anything changes (alpha.105, D-034
// amendment). The package manager's own simulation of an install names
// every package it would bring. What the install named, the packages of
// the PHP version it installs and ordinary libraries are expected; another
// PHP version's packages, Apache, another catalogue component's package and
// a database server are notable: the owner sees them by name and confirms
// them before anything is installed, and the real install is checked again
// against what he confirmed.
//
// Beklenmeyen kurulumlar, hiçbir şey değişmeden önce gösterilir. Paket
// yöneticisinin benzetimi kurulumun getireceği her paketi adlandırır;
// dikkat çekenleri sahip adıyla görür ve onaylar, gerçek kurulum onaylanana
// göre yeniden denetlenir.

const (
	notableReasonNewPHPVersion  = "new_php_version"
	notableReasonApache         = "apache"
	notableReasonComponent      = "component"
	notableReasonDatabaseServer = "database_server"
	// alpha.107 (D-038 §12): an installed package the install would
	// upgrade although it was not asked for, and a distribution package a
	// vendor repository would replace (never confirmable).
	notableReasonUpgrade            = "upgrade"
	notableReasonVendorReplacesDist = "vendor_replaces_distribution"

	errCodeInstallNeedsConfirmation = "INSTALL_NEEDS_CONFIRMATION"
	errCodeInstallPHPVersionRefused = "INSTALL_PHP_VERSION_REFUSED"
	errCodeInstallVendorReplacement = "INSTALL_VENDOR_REPLACEMENT_REFUSED"

	serviceInstallPreviewBound = 25 * time.Second
	notableInstallCap          = 48
)

var (
	phpInstallVersionPackage = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})-`)
	phpFPMVersionPackage     = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})-fpm$`)
	apachePackage            = regexp.MustCompile(`^(apache2|apache2-bin|libapache2-mod-php.*)$`)
	databaseServerPackages   = []struct {
		re        *regexp.Regexp
		component string
	}{
		{regexp.MustCompile(`^(mariadb-server|mariadb-server-[0-9.]+|mariadb-server-core|mysql-server|mysql-server-[0-9.]+)$`), "mariadb"},
		{regexp.MustCompile(`^postgresql(-[0-9]+)?$`), "postgresql"},
	}
	// phpVersionPresent: any package of the version is installed (its
	// /etc/php/<version> tree exists).
	phpVersionPresent = func(version string) bool {
		if !phpVersionText.MatchString(version) {
			return false
		}
		info, err := os.Stat(filepath.Join(phpEtcDir, version))
		return err == nil && info.IsDir()
	}
)

// installClassification is what a simulation is judged against.
type installClassification struct {
	requested   map[string]bool
	components  map[string]bool
	phpVersions map[string]bool
	owner       func(string) string
	present     func(string) bool
	// vendorOf names the enabled vendor repository that offers this version
	// of this package ("" for none); nil reads none (alpha.107).
	vendorOf func(name, version string) string
}

// installed are the catalogue components already on the host: their own
// packages and roles are not "another component" (a bundle's earlier step
// installed them, or the owner did).
func newInstallClassification(family string, requested []string, components []string, phpVersions []string, installed map[string]bool) installClassification {
	c := installClassification{
		requested:   map[string]bool{},
		components:  map[string]bool{},
		phpVersions: map[string]bool{},
		present:     phpVersionPresent,
	}
	for _, pkg := range requested {
		pkg = installArgName(pkg)
		c.requested[pkg] = true
		if match := phpInstallVersionPackage.FindStringSubmatch(pkg); match != nil {
			c.phpVersions[match[1]] = true
		}
	}
	for _, id := range components {
		c.components[id] = true
	}
	for id, ok := range installed {
		if ok {
			c.components[id] = true
		}
	}
	for _, version := range phpVersions {
		if version != "" {
			c.phpVersions[version] = true
		}
	}
	c.owner = catalogueProtectedComponent(family, c.requested)
	return c
}

// classify returns the notable packages of a simulation's installs, sorted
// by reason and name; every other package is expected. The PHP version the
// distribution's php-fpm brings, when the install names it, is the
// version it installs.
func (c installClassification) classify(installs []string) []transport.NotableInstall {
	notable := []transport.NotableInstall{}
	if c.requested["php-fpm"] {
		versions := map[string]bool{}
		for version := range c.phpVersions {
			versions[version] = true
		}
		for _, name := range installs {
			if match := phpFPMVersionPackage.FindStringSubmatch(packageBaseName(name)); match != nil {
				versions[match[1]] = true
			}
		}
		c.phpVersions = versions
	}
	for _, name := range uniquePackageNames(installs) {
		pkg := packageBaseName(name)
		if c.requested[pkg] {
			continue
		}
		if item, ok := c.notableOne(pkg); ok {
			notable = append(notable, item)
		}
	}
	sort.SliceStable(notable, func(i, j int) bool {
		if ri, rj := notableReasonRank(notable[i].Reason), notableReasonRank(notable[j].Reason); ri != rj {
			return ri < rj
		}
		return notable[i].Package < notable[j].Package
	})
	return notable
}

func (c installClassification) notableOne(pkg string) (transport.NotableInstall, bool) {
	for _, db := range databaseServerPackages {
		if db.re.MatchString(pkg) && !c.components[db.component] {
			return transport.NotableInstall{Package: pkg, Reason: notableReasonDatabaseServer, Component: db.component}, true
		}
	}
	if apachePackage.MatchString(pkg) && !c.components["apache"] {
		return transport.NotableInstall{Package: pkg, Reason: notableReasonApache, Component: "apache"}, true
	}
	if match := phpInstallVersionPackage.FindStringSubmatch(pkg); match != nil {
		version := match[1]
		if !c.phpVersions[version] && !c.present(version) {
			return transport.NotableInstall{Package: pkg, Reason: notableReasonNewPHPVersion, Version: version}, true
		}
		return transport.NotableInstall{}, false
	}
	if id := c.owner(pkg); id != "" && !c.components[id] {
		return transport.NotableInstall{Package: pkg, Reason: notableReasonComponent, Component: id}, true
	}
	return transport.NotableInstall{}, false
}

// classifySimulation is classify over the packages the simulation newly
// installs, then every installed package it would upgrade without being
// asked: a distribution package replaced by a vendor repository's build is
// vendor_replaces_distribution, any other upgrade is upgrade (alpha.107).
func (c installClassification) classifySimulation(sim aptSimulation) []transport.NotableInstall {
	fresh := make([]string, 0, len(sim.Installed))
	for _, name := range sim.Installed {
		if _, upgrade := sim.Upgrades[name]; !upgrade {
			fresh = append(fresh, name)
		}
	}
	notable := c.classify(fresh)
	names := make([]string, 0, len(sim.Upgrades))
	for name := range sim.Upgrades {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pkg := packageBaseName(name)
		if c.requested[pkg] {
			continue
		}
		upgrade := sim.Upgrades[name]
		item := transport.NotableInstall{Package: pkg, Reason: notableReasonUpgrade, From: upgrade.From, To: upgrade.To}
		if c.vendorOf != nil {
			if repo := c.vendorOf(pkg, upgrade.To); repo != "" && c.vendorOf(pkg, upgrade.From) == "" {
				item.Reason, item.Component = notableReasonVendorReplacesDist, repo
			}
		}
		notable = append(notable, item)
	}
	sort.SliceStable(notable, func(i, j int) bool {
		if ri, rj := notableReasonRank(notable[i].Reason), notableReasonRank(notable[j].Reason); ri != rj {
			return ri < rj
		}
		return notable[i].Package < notable[j].Package
	})
	return notable
}

func notableReasonRank(reason string) int {
	switch reason {
	case notableReasonVendorReplacesDist:
		return -1
	case notableReasonDatabaseServer:
		return 0
	case notableReasonApache:
		return 1
	case notableReasonNewPHPVersion:
		return 2
	case notableReasonUpgrade:
		return 4
	default:
		return 3
	}
}

// unconfirmedNotable is what the owner has not confirmed.
func unconfirmedNotable(notable []transport.NotableInstall, confirmed []string) []transport.NotableInstall {
	ok := map[string]bool{}
	for _, name := range confirmed {
		ok[strings.TrimSpace(name)] = true
	}
	out := []transport.NotableInstall{}
	for _, item := range notable {
		// A vendor replacement of a distribution package is never
		// confirmable (alpha.107).
		if !ok[item.Package] || item.Reason == notableReasonVendorReplacesDist {
			out = append(out, item)
		}
	}
	return out
}

// packageInstallNotableRefusal: the install would bring packages the owner
// has not confirmed (alpha.105). Nothing was installed.
type packageInstallNotableRefusal struct {
	Packages []string
	Notable  []transport.NotableInstall
}

func (r *packageInstallNotableRefusal) Error() string {
	names := make([]string, 0, len(r.Notable))
	for _, item := range r.Notable {
		names = append(names, item.Package)
	}
	return fmt.Sprintf("installing %s would also install %s, which was not confirmed; nothing was installed. Install it from Components, which lists these packages for you to confirm, or choose another version",
		strings.Join(r.Packages, ", "), strings.Join(capNames(names, 12), ", "))
}

// packageInstallVendorRefusal: the install would replace distribution
// packages with a vendor repository's builds (alpha.107, D-038 §12). Never
// confirmable; nothing was installed.
type packageInstallVendorRefusal struct {
	Packages []string
	Notable  []transport.NotableInstall
}

func (r *packageInstallVendorRefusal) Error() string {
	names := make([]string, 0, len(r.Notable))
	repo := ""
	for _, item := range r.Notable {
		if item.Reason == notableReasonVendorReplacesDist {
			names = append(names, item.Package)
			repo = item.Component
		}
	}
	return fmt.Sprintf("installing %s would replace the distribution's %s with the %s repository's builds; nothing was installed. The repository is not confined to its component on this server: see its card under Components",
		strings.Join(r.Packages, ", "), strings.Join(capNames(names, 12), ", "), repo)
}

// notableInstallGuard refuses a simulation whose notable installs are not
// all confirmed, and one that would replace a distribution package with a
// vendor repository's build whatever was confirmed.
func notableInstallGuard(c installClassification, packages, confirmed []string) func(aptSimulation) error {
	return func(sim aptSimulation) error {
		missing := unconfirmedNotable(c.classifySimulation(sim), confirmed)
		for _, item := range missing {
			if item.Reason == notableReasonVendorReplacesDist {
				return &packageInstallVendorRefusal{Packages: append([]string{}, packages...), Notable: missing}
			}
		}
		if len(missing) > 0 {
			return &packageInstallNotableRefusal{Packages: append([]string{}, packages...), Notable: missing}
		}
		return nil
	}
}

// versionCompanionPackages are the companions of a picked version that the
// package manager offers for it, and the ones it does not.
func versionCompanionPackages(svc *core.ManagedService, versionPrefix string, available func(string) bool) (offered, skipped []string) {
	if svc == nil || svc.Repo == nil || versionPrefix == "" {
		return nil, nil
	}
	for _, tpl := range svc.Repo.VersionCompanions {
		name := strings.ReplaceAll(tpl, "{v}", versionPrefix)
		if available(name) {
			offered = append(offered, name)
		} else {
			skipped = append(skipped, name)
		}
	}
	return offered, skipped
}

// phpToolForService maps a catalogue id to its PHP tool (the database
// tools only: webmail's PHP version is not chosen at install).
func phpToolForInstall(serviceID string) string {
	switch serviceID {
	case phpToolPHPMyAdmin, phpToolPHPPgAdmin:
		return serviceID
	}
	return ""
}

// phpToolVersionPackages are a tool's extension packages for one PHP
// version, and a refusal when the version cannot serve the tool here:
// its FPM package or an extension package has no installation candidate
// (and is not installed, and is not installed by another step).
func phpToolVersionPackages(tool, version, family string, installed, available func(string) bool, coming map[string]bool) ([]string, *transport.InstallPreviewRefusal) {
	if family != "apt" {
		return nil, &transport.InstallPreviewRefusal{ID: tool, Code: "php_version_unsupported", Version: version}
	}
	fpm := "php" + version + "-fpm"
	if !installed(fpm) && !coming[fpm] && !available(fpm) {
		return nil, &transport.InstallPreviewRefusal{ID: tool, Code: "php_version_unavailable", Version: version, Packages: []string{fpm}}
	}
	var packages, unavailable []string
	for _, suffix := range phpToolExtensionSuffixes[tool] {
		name := "php" + version + "-" + suffix
		if installed(name) {
			continue
		}
		if coming[name] || available(name) {
			packages = append(packages, name)
			continue
		}
		unavailable = append(unavailable, name)
	}
	if len(unavailable) > 0 {
		return nil, &transport.InstallPreviewRefusal{ID: tool, Code: "php_extensions_unavailable", Version: version, Packages: unavailable}
	}
	return packages, nil
}

// installPreviewOps is the narrow seam of a preview.
type installPreviewOps struct {
	family              string
	installedComponents func() map[string]bool
	installed           func(string) bool
	available           func(string) bool
	simulate            func(ctx context.Context, packages []string) (aptSimulation, error)
	vendorOf            func(name, version string) string
	// exactDefault (alpha.107): the exact package a plain install of the
	// component installs with its vendor repository enabled ("" for none).
	exactDefault func(ctx context.Context, svc *core.ManagedService) (string, error)
	// existingData (alpha.107): an engine's data a previous install left.
	existingData func(svc *core.ManagedService, pkg string) []transport.ExistingData
	// vendorShared (alpha.107): the vendor's shared packages a vendor-only
	// version needs, as name=version.
	vendorShared func(repo *core.ManagedRepo) []string
	lock         func() bool
	unlock       func()
}

// serviceInstallPreviewWithOps answers one preview.
func serviceInstallPreviewWithOps(ctx context.Context, req *transport.ServiceInstallPreviewRequest, resp *transport.ServiceInstallPreviewResponse, ops installPreviewOps) {
	resp.PackageFamily = ops.family
	if len(req.Steps) == 0 || len(req.Steps) > 8 {
		resp.Error = "a preview names one to eight components"
		return
	}
	if ops.family != "apt" && ops.family != "pacman" {
		resp.State = "unsupported"
		return
	}
	var requested, components, phpVersions []string
	coming := map[string]bool{}
	type toolStep struct{ tool, version string }
	var tools []toolStep
	for _, step := range req.Steps {
		svc := core.GetManagedServiceByID(strings.TrimSpace(step.ID))
		if svc == nil {
			resp.Error = "unknown service"
			return
		}
		if refusal := genericDNSEngineMutationRefusal(svc); refusal != "" {
			resp.State, resp.Error = "unsupported", refusal
			return
		}
		components = append(components, svc.ID)
		if ops.existingData != nil {
			resp.ExistingData = append(resp.ExistingData, ops.existingData(svc, step.Package)...)
		}
		if svc.ID == "roundcube" {
			// Webmail is a verified tarball, not a package; its PHP
			// extensions are prepared for the version it is bound to.
			continue
		}
		pkgs := svc.Packages[ops.family]
		if step.Package == "" && ops.exactDefault != nil {
			exact, err := ops.exactDefault(ctx, svc)
			if err != nil {
				resp.Error = err.Error()
				return
			}
			if exact != "" {
				pkgs = []string{exact}
			}
		}
		if step.Package != "" {
			match, err := validateRepoPackageSelection(svc, step.Package)
			if err != nil {
				resp.Error = err.Error()
				return
			}
			if ops.family != "apt" {
				resp.Error = "choosing a version needs a managed repository, which currently requires a verified APT package ecosystem"
				return
			}
			pkgs = []string{step.Package}
			if len(match) > 1 {
				offered, _ := versionCompanionPackages(svc, match[1], ops.available)
				pkgs = append(pkgs, offered...)
			}
			if ops.vendorShared != nil {
				pkgs = append(pkgs, ops.vendorShared(svc.Repo)...)
			}
		}
		for _, pkg := range pkgs {
			if !ops.installed(pkg) {
				requested = append(requested, pkg)
				coming[pkg] = true
			}
		}
		if version := strings.TrimSpace(step.PHPVersion); version != "" {
			tool := phpToolForInstall(svc.ID)
			if tool == "" || !phpVersionText.MatchString(version) {
				resp.Error = "a PHP version is chosen only for phpMyAdmin and phpPgAdmin"
				return
			}
			phpVersions = append(phpVersions, version)
			tools = append(tools, toolStep{tool, version})
		}
	}
	resp.Refusals = []transport.InstallPreviewRefusal{}
	for _, item := range tools {
		pkgs, refusal := phpToolVersionPackages(item.tool, item.version, ops.family, ops.installed, ops.available, coming)
		if refusal != nil {
			resp.Refusals = append(resp.Refusals, *refusal)
			continue
		}
		for _, pkg := range pkgs {
			if !coming[pkg] {
				requested = append(requested, pkg)
				coming[pkg] = true
			}
		}
	}
	requested = uniquePackageNames(requested)
	resp.Requested = append([]string{}, requested...)
	if len(requested) == 0 {
		resp.State = "known"
		return
	}
	if !ops.lock() {
		resp.State = "busy"
		return
	}
	sim, err := ops.simulate(ctx, requested)
	ops.unlock()
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.State = "known"
	resp.Installs = append([]string{}, sim.Installed...)
	resp.Removes = append([]string{}, sim.Removed...)
	classification := newInstallClassification(ops.family, requested, components, phpVersions, ops.installedComponents())
	classification.vendorOf = ops.vendorOf
	notable := classification.classifySimulation(sim)
	if len(notable) > notableInstallCap {
		notable = notable[:notableInstallCap]
	}
	resp.Notable = notable
}

func emptyServiceInstallPreview() transport.ServiceInstallPreviewResponse {
	return transport.ServiceInstallPreviewResponse{
		State:     "failed",
		Requested: []string{},
		Installs:  []string{},
		Notable:   []transport.NotableInstall{},
		Removes:   []string{},
		Refusals:  []transport.InstallPreviewRefusal{},
	}
}

// pacmanPrintInstallArgs lists what an install would bring without
// changing anything (the targets not installed yet, dependencies included).
func pacmanPrintInstallArgs(packages []string) []string {
	return append([]string{"-S", "--print", "--print-format", "%n", "--needed"}, packages...)
}

func parsePacmanPrint(out []byte) []string {
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "::") || strings.Contains(line, " ") || !validPackageName(line) {
			continue
		}
		names = append(names, line)
	}
	return uniquePackageNames(names)
}

func runPacmanInstallPrint(ctx context.Context, run packageSimulationRunner, pacman string, packages []string) (aptSimulation, error) {
	simCtx, cancel := context.WithTimeout(ctx, packageRemovalSimulationBound)
	defer cancel()
	out, err := run(simCtx, packageCommandEnv(), pacman, pacmanPrintInstallArgs(packages)...)
	if err != nil {
		return aptSimulation{}, fmt.Errorf("pacman could not show what installing would do: %v: %s", err, firstLine(lastLines(string(out), 3)))
	}
	return aptSimulation{Installed: parsePacmanPrint(out)}, nil
}

// ServiceInstallPreviewV1 answers, read-only, what installing these
// components together would bring (alpha.105): the package manager's own
// simulation, the notable packages among them, and any step whose chosen
// PHP version cannot serve its tool. It starts no mutation, never waits
// for the package lock (another package operation → "busy") and is bounded.
//
// ServiceInstallPreviewV1, bu bileşenleri birlikte kurmanın neyi
// getireceğini salt-okur yanıtlar. Değişiklik başlatmaz, kilidi beklemez.
func (a *Agent) ServiceInstallPreviewV1(req *transport.ServiceInstallPreviewRequest, resp *transport.ServiceInstallPreviewResponse) error {
	*resp = emptyServiceInstallPreview()
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), serviceInstallPreviewBound)
	defer cancel()
	profile, err := detectHostPlatform()
	if err != nil {
		resp.Error = "detect host platform: " + err.Error()
		return nil
	}
	family := string(profile.PackageManager)
	ops := installPreviewOps{
		family:              family,
		installedComponents: func() map[string]bool { return a.installedServiceSetWithProfile(profile) },
		installed:           func(pkg string) bool { return packageInstalledForProfile(profile, pkg) },
		available:           func(string) bool { return false },
		existingData: func(svc *core.ManagedService, pkg string) []transport.ExistingData {
			if a.serviceInstalledWithProfile(svc, profile) && pkg == "" {
				return nil
			}
			return existingEngineData(svc, family, pkg)
		},
		lock:   packageOperationMu.TryLock,
		unlock: packageOperationMu.Unlock,
	}
	switch family {
	case "apt":
		aptGet, err := executableForProfile(profile, family, "apt-get")
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		aptCache, err := executableForProfile(profile, family, "apt-cache")
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		ops.available = func(name string) bool { return packageAvailableWithExecutable(name, aptCache) }
		ops.vendorOf = hostVendorVersionLookup()
		ops.vendorShared = hostVendorSharedPackageArgs
		defaults := hostExactDefaultOps(aptCache)
		ops.exactDefault = func(ctx context.Context, svc *core.ManagedService) (string, error) {
			return exactDefaultPackage(ctx, svc, family, defaults)
		}
		ops.simulate = func(ctx context.Context, packages []string) (aptSimulation, error) {
			return runAptSimulation(ctx, runPackageSimulationCommand, aptGet, aptInstallArgs(packages))
		}
	case "pacman":
		pacman, err := executableForProfile(profile, family, "pacman")
		if err != nil {
			resp.Error = err.Error()
			return nil
		}
		ops.simulate = func(ctx context.Context, packages []string) (aptSimulation, error) {
			return runPacmanInstallPrint(ctx, runPackageSimulationCommand, pacman, packages)
		}
	}
	serviceInstallPreviewWithOps(ctx, req, resp, ops)
	return nil
}

// aptInstallArgs is the one argument list of a real install; its
// simulation is the same list behind "-s".
func aptInstallArgs(packages []string) []string {
	return append([]string{"install", "-y", "--no-install-recommends"}, packages...)
}

// ---- proof of a PHP tool just installed ----

// phpToolProofRecordPath keeps the tools whose proof after install failed.
var phpToolProofRecordPath = "/var/lib/celikpanel-agent-private/php-tool-proofs.json"

type phpToolProofRecord struct {
	Version int                            `json:"version"`
	Failed  map[string]phpToolProofFailure `json:"failed"`
}

// phpToolProofFailure is one tool's failed proof: the cause on the version
// it runs on, and whether other installed versions were tried too.
type phpToolProofFailure struct {
	Cause       transport.PHPToolCause `json:"cause"`
	TriedOthers bool                   `json:"tried_others,omitempty"`
}

func readPHPToolProofFailures() map[string]phpToolProofFailure {
	out := map[string]phpToolProofFailure{}
	data, err := os.ReadFile(phpToolProofRecordPath)
	if err != nil || len(data) > 32*1024 {
		return out
	}
	var record phpToolProofRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return out
	}
	for tool, failure := range record.Failed {
		if validPHPTool(tool) {
			out[tool] = failure
		}
	}
	return out
}

// writePHPToolProof records (cause != nil) or clears one tool's failed proof;
// triedOthers says whether other installed versions were tried as well.
func writePHPToolProof(tool string, cause *transport.PHPToolCause, triedOthers bool) error {
	if !validPHPTool(tool) {
		return fmt.Errorf("unknown PHP tool %q", tool)
	}
	failed := readPHPToolProofFailures()
	if cause == nil {
		if _, ok := failed[tool]; !ok {
			return nil
		}
		delete(failed, tool)
	} else {
		bounded := *cause
		if len(bounded.Detail) > 512 {
			bounded.Detail = bounded.Detail[:512]
		}
		failed[tool] = phpToolProofFailure{Cause: bounded, TriedOthers: triedOthers}
	}
	data, err := json.Marshal(phpToolProofRecord{Version: 1, Failed: failed})
	if err != nil {
		return err
	}
	return writePrivateFileAtomic(phpToolProofRecordPath, data, ".php-tool-proofs-*")
}

// writePrivateFileAtomic replaces a small private file (0600) atomically.
func writePrivateFileAtomic(path string, data []byte, pattern string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return fmt.Errorf("record %s: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		return fmt.Errorf("record %s: %w", filepath.Base(path), err)
	}
	return nil
}

// phpToolProveOps adds to the bind seam what a proof needs.
type phpToolProveOps struct {
	bind       phpToolBindOps
	candidates func(tool, current string) []string
	record     func(tool string, cause *transport.PHPToolCause, triedOthers bool) error
}

// phpToolProofCandidates are the installed versions, other than current,
// whose FPM socket exists and that have the tool's extension packages: the
// distribution's default first, then the newest first.
func phpToolProofCandidates(tool, current string) []string {
	var out []string
	seen := map[string]bool{current: true}
	add := func(version string) {
		if version == "" || seen[version] {
			return
		}
		seen[version] = true
		if pathExists(versionedFPMSocketPath(version)) && phpToolHasExtensionPackages(tool, version) {
			out = append(out, version)
		}
	}
	add(phpDefaultVersionFromMetas())
	for _, version := range installedPHPFPMVersions() {
		add(version)
	}
	return out
}

func hostPHPToolProveOps() phpToolProveOps {
	return phpToolProveOps{bind: hostPHPToolBindOps(), candidates: phpToolProofCandidates, record: writePHPToolProof}
}

func causeOfBindError(err error, version string) *transport.PHPToolCause {
	var typed *phpToolBindError
	if errors.As(err, &typed) {
		cause := typed.Cause
		if cause.Version == "" {
			cause.Version = version
		}
		return &cause
	}
	cause := phpToolCauseOf(err, "other", version)
	return &cause
}

// provePHPToolWith proves a tool just installed: it must answer as itself
// on the version it was configured on; with allowFallback (a first
// install, apt) every other installed version that has its extensions is
// tried through the verified move, which puts the previous configuration
// back when it fails. A tool proved nowhere stays installed, and its first
// cause is recorded so Components reads "Not ready" with it.
func provePHPToolWith(ctx context.Context, ops phpToolProveOps, tool string, allowFallback bool) transport.ProvePHPToolResponse {
	resp := transport.ProvePHPToolResponse{Tool: tool}
	if !validPHPTool(tool) {
		resp.Error = fmt.Sprintf("unknown PHP tool %q", tool)
		return resp
	}
	if !ops.bind.toolInstalled(tool) {
		resp.Error = fmt.Sprintf("%s is not installed", phpToolDisplayName(tool))
		return resp
	}
	state := ops.bind.state(tool)
	current := state.version()
	var first *transport.PHPToolCause
	if state.Socket != "" {
		err := ops.bind.httpProbe(ctx, tool, current)
		if err == nil {
			resp.Proved, resp.Version = true, current
			_ = ops.record(tool, nil, false)
			if current != "" {
				_ = ops.bind.writeRecord(tool, current)
			}
			return resp
		}
		cause := phpToolCauseOf(err, "other", current)
		first = &cause
		resp.Attempts = append(resp.Attempts, transport.PHPToolAttempt{Version: current, Cause: &cause})
	} else {
		first = &transport.PHPToolCause{Kind: "version_not_installed"}
	}
	triedOthers := false
	if allowFallback && ops.bind.family() == "apt" {
		for _, version := range ops.candidates(tool, current) {
			triedOthers = true
			if ctx.Err() != nil {
				break
			}
			if _, err := bindPHPToolWith(ctx, ops.bind, tool, version); err != nil {
				resp.Attempts = append(resp.Attempts, transport.PHPToolAttempt{Version: version, Cause: causeOfBindError(err, version)})
				continue
			}
			resp.Proved, resp.Moved, resp.Version = true, true, version
			_ = ops.record(tool, nil, false)
			return resp
		}
	}
	resp.Version = current
	if err := ops.record(tool, first, triedOthers); err != nil {
		resp.Error = "the failed proof could not be recorded: " + err.Error()
	}
	return resp
}

// ProvePHPToolV1 proves a PHP tool right after its install (alpha.105).
// ProvePHPToolV1, bir PHP aracını kurulumundan hemen sonra kanıtlar.
func (a *Agent) ProvePHPToolV1(req *transport.ProvePHPToolRequest, resp *transport.ProvePHPToolResponse) error {
	*resp = transport.ProvePHPToolResponse{}
	if req == nil {
		resp.Error = "missing request"
		return nil
	}
	tool := strings.TrimSpace(req.Tool)
	resp.Tool = tool
	if !validPHPTool(tool) {
		resp.Error = "invalid PHP tool"
		return nil
	}
	ctx, finishStep, err := a.requiredServiceMutationStep(
		req.ServiceMutationBinding,
		newServiceMutationStepClaim(serviceMutationStepProvePHPTool, tool, "", "prove"),
	)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	defer finishStep()
	*resp = provePHPToolWith(ctx, hostPHPToolProveOps(), tool, req.AllowFallback)
	return nil
}
