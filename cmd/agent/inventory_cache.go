package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/systemd"
)

// Read-only host inventory, batched and cached (v0.1.0-alpha.125, "faster
// panel" P1).
//
// One Components scan used to start 33 processes, among them two full
// `systemctl list-unit-files` (each about 0.4 s on a 2-vCPU host, because
// systemd computes the enablement state of every unit file), five full
// `dpkg-query` runs and seven host-platform detections. The same native facts
// are now read once and shared by every read of the scan:
//
//   - the host platform is detected once per agent lifetime and again after a
//     failure, a reboot (boot_id) or a change of /etc/os-release;
//   - the package database is read with one `dpkg-query`/`pacman -Q` whose
//     rows give every consumer its own historic view;
//   - the unit files are listed by name pattern (catalogue names plus the
//     literal head of each catalogue regex), never the whole host, and the
//     canonical name of every present catalogue unit is read with one batched
//     `systemctl show`;
//   - what was read is kept while the package database and the systemd unit
//     directories are unchanged (mtime, ctime, size and inode), at most
//     inventoryTTL, and is dropped whenever a panel mutation begins or ends.
//
// What is cached is only what is *installed*. Whether a unit is running is
// always read live. Changes the owner makes outside the panel (apt, pacman,
// systemctl enable/mask, a unit file dropped in /etc) change the package
// database or a unit directory, so the next read sees them (owner
// independence, D-022). Strict, firewall-safe discovery keeps failing closed:
// any doubt about the narrowed listing falls back to the full listing, and a
// failed command is an error, never an empty set.
//
// Salt-okur envanter, toplu ve önbellekli: bir tarama artık her yerel bilgiyi
// bir kez okur. Yalnız "kurulu" bilgisi saklanır; çalışıyor mu her zaman canlı
// okunur. Sahibin panel dışı değişiklikleri paket veritabanını ya da birim
// dizinlerini değiştirir, bir sonraki okuma onları görür. Güvenlik duvarı için
// katı keşif kapalı kalmaya devam eder.

// inventoryCacheEnabled is set by the production agent's main. Tests leave it
// off, so every read is live unless a test turns it on.
var inventoryCacheEnabled atomic.Bool

// inventoryGeneration moves on every invalidation; an entry filled under an
// older generation is never served.
var inventoryGeneration atomic.Uint64

// inventoryTTL bounds how long a reading is served even with an unchanged
// fingerprint. Variables for tests.
var (
	inventoryTTL   = 30 * time.Second
	inventoryClock = time.Now
)

// invalidateHostInventory drops every cached reading. Called when a panel
// mutation begins or ends (serviceMutationManager).
func invalidateHostInventory() {
	inventoryGeneration.Add(1)
}

// inventoryCommand runs one read-only systemctl query. Variable for tests.
var inventoryCommand = func(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return serviceMutationCommand(ctx, executable, args...).Output()
}

// inventoryEntry is one cached reading.
type inventoryEntry[T any] struct {
	mu          sync.Mutex
	valid       bool
	key         string
	fingerprint string
	generation  uint64
	at          time.Time
	value       T
}

// get returns the cached value when it was read under the same key,
// fingerprint and generation within the TTL; otherwise it runs read and keeps
// its answer (an error is never kept). Readers of the same entry wait for one
// another, so concurrent reads start one command, not several.
func (e *inventoryEntry[T]) get(key, fingerprint string, read func() (T, error)) (T, error) {
	if !inventoryCacheEnabled.Load() || fingerprint == "" {
		return read()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	generation := inventoryGeneration.Load()
	now := inventoryClock()
	if e.valid && e.key == key && e.fingerprint == fingerprint && e.generation == generation &&
		now.Sub(e.at) >= 0 && now.Sub(e.at) < inventoryTTL {
		return e.value, nil
	}
	value, err := read()
	if err != nil {
		e.valid = false
		var zero T
		return zero, err
	}
	e.valid, e.key, e.fingerprint, e.generation, e.at, e.value = true, key, fingerprint, generation, now, value
	return value, nil
}

func (e *inventoryEntry[T]) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.valid = false
}

// ---------------------------------------------------------------------------
// Host platform

var hostProfileCache inventoryEntry[hostplatform.Profile]

// readHostProfile is the host platform for read-only inventory RPCs. Host
// mutations keep detecting live (detectHostPlatform), so readiness still
// gates every change.
// Salt-okur envanter için makine platformu; değişiklikler canlı algılar.
func readHostProfile() (hostplatform.Profile, error) {
	profile, err := hostProfileCache.get("profile", hostProfileFingerprint(), verifiedHostProfileForAnyFamily)
	if err != nil {
		return hostplatform.Profile{}, err
	}
	// The caller may not change the shared reading.
	profile.Executables = maps.Clone(profile.Executables)
	return profile, nil
}

// ---------------------------------------------------------------------------
// Package database

// dpkgInventoryRow is one row of the shared dpkg query: every field any
// consumer used to ask for with its own query.
type dpkgInventoryRow struct {
	binary  string // ${binary:Package}: name, with :arch for a multi-arch "same" package
	pkg     string // ${Package}
	abbrev  string // ${db:Status-Abbrev}
	version string // ${Version}
	status  string // ${Status}: "install ok installed"
}

// packageInventory is one reading of the package database.
type packageInventory struct {
	family string
	dpkg   []dpkgInventoryRow
	pacman string // `pacman -Q` output, "name version" rows
}

func dpkgInventoryArgs() []string {
	return []string{"-W", "-f=${binary:Package}\t${Package}\t${db:Status-Abbrev}\t${Version}\t${Status}\n"}
}

// parseDpkgInventory keeps the rows with exactly five fields; anything else is
// skipped, as each consumer's own parser used to skip it.
func parseDpkgInventory(output string) []dpkgInventoryRow {
	rows := []dpkgInventoryRow{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) != 5 {
			continue
		}
		rows = append(rows, dpkgInventoryRow{binary: fields[0], pkg: fields[1], abbrev: fields[2], version: fields[3], status: fields[4]})
	}
	return rows
}

// The historic views. Each consumer keeps its own parser and therefore its
// own rule for what counts as installed; only the query is shared.

// versionsText is the `${binary:Package}\t${db:Status-Abbrev}\t${Version}` view
// (parseDpkgVersions).
func (p packageInventory) versionsText() string {
	var b strings.Builder
	for _, row := range p.dpkg {
		b.WriteString(row.binary + "\t" + row.abbrev + "\t" + row.version + "\n")
	}
	return b.String()
}

// statusText is the `${binary:Package}\t${Status}` view (strict discovery).
func (p packageInventory) statusText() string {
	var b strings.Builder
	for _, row := range p.dpkg {
		b.WriteString(row.binary + "\t" + row.status + "\n")
	}
	return b.String()
}

// abbrevText is the `${Package}\t${db:Status-Abbrev}` view (repository
// package identity).
func (p packageInventory) abbrevText() string {
	var b strings.Builder
	for _, row := range p.dpkg {
		b.WriteString(row.pkg + "\t" + row.abbrev + "\n")
	}
	return b.String()
}

var packageInventoryCache inventoryEntry[packageInventory]

var errPackageInventoryTimeout = errors.New("the package database did not answer in time")

var errPackageInventoryUnsupported = errors.New("the package database of this family is not read by the shared inventory")

// readPackageInventory returns the package database of an apt or pacman host,
// from the cache while the database is unchanged.
func readPackageInventory(ctx context.Context, profile hostplatform.Profile) (packageInventory, error) {
	family := string(profile.PackageManager)
	var tool string
	var args []string
	switch family {
	case "apt":
		tool, args = "dpkg-query", dpkgInventoryArgs()
	case "pacman":
		tool, args = "pacman", pacmanVersionQueryArgs()
	default:
		return packageInventory{}, errPackageInventoryUnsupported
	}
	executable, err := executableForProfile(profile, family, tool)
	if err != nil {
		return packageInventory{}, err
	}
	return packageInventoryCache.get(family+"\x00"+executable, packageDatabaseFingerprint(family), func() (packageInventory, error) {
		queryCtx, cancel := context.WithTimeout(ctx, packageVersionQueryTimeout)
		defer cancel()
		// packageVersionQuery: a fixed environment, its own process group, a
		// timeout and an output ceiling (package_versions_runner.go).
		output, err := packageVersionQuery(queryCtx, executable, args...)
		if err != nil {
			switch {
			case errors.Is(err, errPackageVersionOutputTooLarge):
				return packageInventory{}, err
			case errors.Is(queryCtx.Err(), context.DeadlineExceeded):
				return packageInventory{}, errPackageInventoryTimeout
			}
			return packageInventory{}, fmt.Errorf("query installed packages with %s: %w", family, err)
		}
		inventory := packageInventory{family: family}
		if family == "apt" {
			inventory.dpkg = parseDpkgInventory(string(output))
		} else {
			inventory.pacman = string(output)
		}
		return inventory, nil
	})
}

// ---------------------------------------------------------------------------
// Unit files

// inventorySentinelUnit is shipped by systemd itself on every supported
// host. A narrowed listing that does not show it is not trusted: the full
// listing is read instead.
const inventorySentinelUnit = "systemd-journald.service"

// unitGlobPrefix returns the fixed head every name a `^…` catalogue regex can
// match starts with, or ok=false when the regex has no such head (no ^
// anchor, alternation, or nothing literal). A quantifier that can drop the
// last literal character (? * {) drops it from the head as well.
func unitGlobPrefix(pattern string) (string, bool) {
	if !strings.HasPrefix(pattern, "^") || strings.Contains(pattern, "|") {
		return "", false
	}
	rest := pattern[1:]
	var head []byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '@':
			head = append(head, c)
			continue
		case c == '\\' && i+1 < len(rest) && rest[i+1] == '.':
			head = append(head, '.')
			i++
			continue
		}
		if (c == '?' || c == '*' || c == '{') && len(head) > 0 {
			head = head[:len(head)-1]
		}
		break
	}
	if len(head) == 0 {
		return "", false
	}
	return string(head), true
}

// inventoryUnitFilePatterns lists the unit-file name patterns that cover
// every name strict discovery and the runtime instance read can ask about:
// each catalogue unit name (a template instance by its template) and the
// literal head of each catalogue regex. ok=false when some regex has no
// literal head: the caller then lists every unit file, as before.
func inventoryUnitFilePatterns() ([]string, bool) {
	seen := map[string]bool{}
	patterns := []string{}
	add := func(pattern string) {
		if !seen[pattern] {
			seen[pattern] = true
			patterns = append(patterns, pattern)
		}
	}
	for i := range core.ManagedServices {
		svc := &core.ManagedServices[i]
		for _, unit := range svc.SystemNames {
			lookup := unit
			if at := strings.IndexByte(unit, '@'); at >= 0 {
				lookup = unit[:at+1]
			}
			add(lookup + ".service")
		}
		if svc.SystemNamePattern != "" {
			head, ok := unitGlobPrefix(svc.SystemNamePattern)
			if !ok {
				return nil, false
			}
			add(head + "*")
		}
	}
	for _, pattern := range inventoryRuntimeUnitPatterns {
		head, ok := unitGlobPrefix(pattern)
		if !ok {
			return nil, false
		}
		add(head + "*")
	}
	sort.Strings(patterns)
	return patterns, true
}

// inventoryRuntimeUnitPatterns are the regexes the runtime instance read
// matches unit files with (phpInstances).
var inventoryRuntimeUnitPatterns = []string{phpFPMUnitPattern}

const phpFPMUnitPattern = `^php[0-9]+\.[0-9]+-fpm$`

// patternCoveredByInventory reports whether every name `pattern` can match is
// in the narrowed unit-file listing.
func patternCoveredByInventory(pattern string, patterns []string) bool {
	head, ok := unitGlobPrefix(pattern)
	if !ok {
		return false
	}
	for _, glob := range patterns {
		if prefix, isGlob := strings.CutSuffix(glob, "*"); isGlob && strings.HasPrefix(head, prefix) {
			return true
		}
	}
	return false
}

// unitFileInventory is one reading of the unit files: the names (without
// ".service") and whether the listing is narrowed to the catalogue.
type unitFileInventory struct {
	names    map[string]struct{}
	patterns []string // nil when every unit file was listed
}

var unitFileInventoryCache inventoryEntry[unitFileInventory]

func parseUnitFileListing(output string) map[string]struct{} {
	units := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		units[strings.TrimSuffix(fields[0], ".service")] = struct{}{}
	}
	return units
}

// readUnitFiles lists the unit files strict discovery reads, narrowed to the
// catalogue's names. A narrowed listing that failed or did not show systemd's
// own journald unit is replaced by the full listing; a failed full listing is
// an error.
func readUnitFiles(ctx context.Context, profile hostplatform.Profile) (unitFileInventory, error) {
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return unitFileInventory{}, err
	}
	return unitFileInventoryCache.get(systemctl, systemdUnitFingerprint(), func() (unitFileInventory, error) {
		base := []string{"list-unit-files", "--type=service", "--no-legend", "--no-pager"}
		if patterns, ok := inventoryUnitFilePatterns(); ok {
			args := append(append(append([]string(nil), base...), patterns...), inventorySentinelUnit)
			if out, err := inventoryCommand(ctx, systemctl, args...); err == nil {
				names := parseUnitFileListing(string(out))
				if _, ok := names[strings.TrimSuffix(inventorySentinelUnit, ".service")]; ok {
					return unitFileInventory{names: names, patterns: patterns}, nil
				}
			}
		}
		out, err := inventoryCommand(ctx, systemctl, base...)
		if err != nil {
			return unitFileInventory{}, fmt.Errorf("list systemd unit files: %w", err)
		}
		return unitFileInventory{names: parseUnitFileListing(string(out))}, nil
	})
}

// ---------------------------------------------------------------------------
// Batched systemctl show

// showUnitsBatched reads the given properties (Id is always added) of every
// unit with one systemctl call. ok=false when the answer cannot be split per
// unit or the call failed.
func showUnitsBatched(ctx context.Context, systemctl string, units []string, properties ...string) ([]map[string][]string, bool) {
	if len(units) == 0 {
		return nil, true
	}
	props := append([]string{"Id"}, properties...)
	args := []string{"show", "-p", strings.Join(props, ",")}
	for _, unit := range units {
		args = append(args, unit+".service")
	}
	out, err := inventoryCommand(ctx, systemctl, args...)
	if err != nil {
		return nil, false
	}
	return systemd.ParseShowBlocks(string(out), len(units))
}

type canonicalUnitReading struct {
	byUnit map[string]string
}

var canonicalUnitCache inventoryEntry[canonicalUnitReading]

// readCanonicalUnits resolves each unit's canonical name (systemd follows an
// alias to the unit it names). One batched call; unit by unit when the batch
// cannot be read, and then any failure is an error.
func readCanonicalUnits(ctx context.Context, systemctl string, units []string) (map[string]string, error) {
	sorted := append([]string(nil), units...)
	sort.Strings(sorted)
	reading, err := canonicalUnitCache.get(systemctl+"\x00"+strings.Join(sorted, " "), systemdUnitFingerprint(), func() (canonicalUnitReading, error) {
		resolved := make(map[string]string, len(sorted))
		if blocks, ok := showUnitsBatched(ctx, systemctl, sorted); ok {
			for i, unit := range sorted {
				resolved[unit] = strings.TrimSuffix(strings.TrimSpace(blocks[i]["Id"][0]), ".service")
			}
			return canonicalUnitReading{byUnit: resolved}, nil
		}
		for _, unit := range sorted {
			out, err := inventoryCommand(ctx, systemctl, "show", unit+".service", "-p", "Id", "--value")
			if err != nil {
				return canonicalUnitReading{}, fmt.Errorf("resolve systemd unit %s: %w", unit, err)
			}
			canonical := strings.TrimSuffix(strings.TrimSpace(string(out)), ".service")
			if canonical == "" {
				return canonicalUnitReading{}, fmt.Errorf("resolve systemd unit %s: empty canonical name", unit)
			}
			resolved[unit] = canonical
		}
		return canonicalUnitReading{byUnit: resolved}, nil
	})
	if err != nil {
		return nil, err
	}
	return reading.byUnit, nil
}

// ---------------------------------------------------------------------------
// Strict discovery through the shared inventory

// inventoryStrictServiceStateProbe answers strict discovery from the shared
// inventory. A dnf host keeps the live probe (its rpm query is not shared).
type inventoryStrictServiceStateProbe struct {
	ctx     context.Context
	profile hostplatform.Profile
	units   *unitFileInventory
	resolve map[string]string
}

func newStrictServiceStateProbe(profile hostplatform.Profile) strictServiceStateProbe {
	if profile.PackageManager == hostplatform.PackageManagerDNF {
		return hostStrictServiceStateProbe{profile: profile}
	}
	return &inventoryStrictServiceStateProbe{ctx: context.Background(), profile: profile}
}

func (p *inventoryStrictServiceStateProbe) UnitFiles() (map[string]struct{}, error) {
	inventory, err := readUnitFiles(p.ctx, p.profile)
	if err != nil {
		return nil, err
	}
	p.units = &inventory
	return inventory.names, nil
}

// CanonicalUnit resolves, on its first call, every catalogue unit whose name
// is present, with one batched systemctl show; later calls read that answer.
func (p *inventoryStrictServiceStateProbe) CanonicalUnit(unit string) (string, error) {
	systemctl, err := executableForProfile(p.profile, string(p.profile.PackageManager), "systemctl")
	if err != nil {
		return "", err
	}
	if p.resolve == nil && p.units != nil {
		resolved, err := readCanonicalUnits(p.ctx, systemctl, presentCatalogueUnits(p.units.names))
		if err != nil {
			return "", err
		}
		p.resolve = resolved
	}
	if canonical, ok := p.resolve[unit]; ok {
		return canonical, nil
	}
	resolved, err := readCanonicalUnits(p.ctx, systemctl, []string{unit})
	if err != nil {
		return "", err
	}
	return resolved[unit], nil
}

// presentCatalogueUnits lists every catalogue unit name strict discovery may
// resolve: the ones whose unit file (or template) is present.
func presentCatalogueUnits(units map[string]struct{}) []string {
	seen := map[string]bool{}
	present := []string{}
	for i := range core.ManagedServices {
		for _, unit := range core.ManagedServices[i].SystemNames {
			lookup := unit
			if at := strings.IndexByte(unit, '@'); at >= 0 {
				lookup = unit[:at+1]
			}
			if _, ok := units[lookup]; ok && !seen[unit] {
				seen[unit] = true
				present = append(present, unit)
			}
		}
	}
	return present
}

func (p *inventoryStrictServiceStateProbe) InstalledPackages(family string) (map[string]struct{}, error) {
	inventory, err := readPackageInventory(p.ctx, p.profile)
	if err != nil {
		return nil, fmt.Errorf("query installed packages with %s: %w", family, err)
	}
	if inventory.family != family {
		return nil, fmt.Errorf("query installed packages: host package family is %s, want %s", inventory.family, family)
	}
	packages := make(map[string]struct{})
	switch family {
	case "apt":
		for _, line := range strings.Split(inventory.statusText(), "\n") {
			line = strings.TrimSpace(line)
			fields := strings.Fields(line)
			if len(fields) < 4 || strings.Join(fields[len(fields)-3:], " ") != "install ok installed" {
				continue
			}
			name := fields[0]
			if colon := strings.IndexByte(name, ':'); colon > 0 {
				name = name[:colon]
			}
			packages[name] = struct{}{}
		}
	case "pacman":
		// `pacman -Qq` was the names of `pacman -Q`.
		for _, line := range strings.Split(inventory.pacman, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			packages[fields[0]] = struct{}{}
		}
	}
	return packages, nil
}

func (p *inventoryStrictServiceStateProbe) RoundcubeInstalled() (bool, error) {
	return roundcubeInstallState()
}

// ---------------------------------------------------------------------------
// Runtime instances

// inventoryUnitsMatching is unitsMatching for read-only RPCs: the unit files
// from the shared inventory when the narrowed listing covers the pattern, and
// the loaded units (template instances appear only there) read live. Any
// doubt falls back to unitsMatching.
func inventoryUnitsMatching(ctx context.Context, pattern string) []string {
	profile, err := readHostProfile()
	if err != nil {
		return nil
	}
	inventory, err := readUnitFiles(ctx, profile)
	if err != nil || (inventory.patterns != nil && !patternCoveredByInventory(pattern, inventory.patterns)) {
		return unitsMatching(pattern)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return nil
	}
	head, _ := unitGlobPrefix(pattern)
	loadedArgs := []string{"list-units", "--all", "--type=service", "--no-legend", "--no-pager", "--plain"}
	if head != "" {
		loadedArgs = append(loadedArgs, head+"*")
	}
	loaded, _ := inventoryCommand(ctx, systemctl, loadedArgs...)
	var files strings.Builder
	for name := range inventory.names {
		files.WriteString(name + ".service\n")
	}
	units := unitNamesMatching(re, files.String(), string(loaded))
	sort.Slice(units, func(i, j int) bool {
		ai, an := versionOf(units[i])
		bi, bn := versionOf(units[j])
		if ai != bi {
			return ai > bi
		}
		return an > bn
	})
	return units
}

// inventoryUnitFileExists reports whether a unit file of exactly this name is
// in the shared listing; it asks systemd when the listing cannot say.
func inventoryUnitFileExists(ctx context.Context, name string) bool {
	if templateInstanceUnit(name) {
		return unitExists(name)
	}
	profile, err := readHostProfile()
	if err != nil {
		return false
	}
	inventory, err := readUnitFiles(ctx, profile)
	if err != nil {
		return unitExists(name)
	}
	if _, ok := inventory.names[name]; ok {
		return true
	}
	if inventory.patterns == nil {
		return false
	}
	for _, pattern := range inventory.patterns {
		if pattern == name+".service" {
			return false
		}
		if prefix, isGlob := strings.CutSuffix(pattern, "*"); isGlob && strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return unitExists(name)
}

// unitStatusLines reads the state of every unit with one batched systemctl
// show, in the scan's dialect ("active (running)"); unit by unit when the
// batch cannot be read. Always live: nothing here is cached.
func unitStatusLines(ctx context.Context, units []string) map[string]string {
	lines := make(map[string]string, len(units))
	if len(units) == 0 {
		return lines
	}
	profile, err := readHostProfile()
	if err == nil {
		if systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl"); err == nil {
			if blocks, ok := showUnitsBatched(ctx, systemctl, units, "ActiveState", "SubState"); ok {
				for i, unit := range units {
					lines[unit] = composeUnitStatus(firstValue(blocks[i], "ActiveState"), firstValue(blocks[i], "SubState"))
				}
				return lines
			}
		}
	}
	for _, unit := range units {
		lines[unit] = unitStatusLine(unit)
	}
	return lines
}

func firstValue(block map[string][]string, key string) string {
	if values := block[key]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
