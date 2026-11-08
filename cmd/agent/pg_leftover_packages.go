package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// The package state a removed PostgreSQL major left (alpha.130 lab, Ubuntu
// 24.04, Boston-like VM). The automatic cleanup had removed 16/main's
// files, but dpkg still showed "rc postgresql-16" (removed, its
// configuration kept) and "ii postgresql-client-16" (nothing named it when
// 16 was removed). The owner's rule is no leftovers, so the same cleanup job
// purges them, through the alpha.104 removal path: the package manager's
// own simulation decides, under the package lock, with the exact list it
// simulated, and anything else it would take refuses the purge (nothing
// changes; the answer says why). It runs only for a major that has no
// cluster left and whose server program is gone, so it is idempotent: a
// second run finds nothing.
//
// Kaldırılmış PostgreSQL sürümünün dpkg durumundaki kalıntıları (rc paket,
// artık kullanılmayan istemci) aynı temizlik işiyle, alpha.104'ün benzetim
// yoluyla temizlenir; benzetim başka bir şey alacaksa hiçbir şey değişmez.

var pgLeftoverPackageRe = regexp.MustCompile(`^postgresql-(?:client-|doc-)?([0-9]{1,3})(?:-[a-z0-9.+-]+)?$`)

// dpkgPackageState is one package's dpkg status: "ii" (installed) or "rc"
// (removed, configuration files kept).
type dpkgPackageState struct {
	Name  string
	State string
}

// parseDpkgStates reads dpkg's status file for the installed and the
// config-files-only packages.
func parseDpkgStates(r io.Reader) ([]dpkgPackageState, error) {
	var out []dpkgPackageState
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var name, status string
	flush := func() {
		switch {
		case name == "":
		case strings.HasSuffix(status, " ok installed"):
			out = append(out, dpkgPackageState{Name: name, State: "ii"})
		case strings.HasSuffix(status, " ok config-files"):
			out = append(out, dpkgPackageState{Name: name, State: "rc"})
		}
		name, status = "", ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "Package: "):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimSpace(strings.TrimPrefix(line, "Status: "))
		}
	}
	flush()
	return out, scanner.Err()
}

// readDpkgStates reads this host's dpkg status (a variable for tests).
var readDpkgStates = func() ([]dpkgPackageState, error) {
	file, err := os.Open(dpkgStatusPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseDpkgStates(io.LimitReader(file, repoListsReadCap))
}

// pgLeftoverPackageSets are, per removed major, the packages its removal
// left: every package of that major in state rc, and the major's client or
// documentation still installed. A major counts only when its server
// package is not installed, its program is not on disk and no cluster of it
// is left.
func pgLeftoverPackageSets(states []dpkgPackageState, clusters []transport.PostgreSQLCluster) map[string][]dpkgPackageState {
	withCluster := map[string]bool{}
	for _, cluster := range clusters {
		withCluster[cluster.Version] = true
	}
	serverInstalled := map[string]bool{}
	for _, state := range states {
		if m := pgVersionPackageRe.FindStringSubmatch(state.Name); m != nil && state.State == "ii" {
			serverInstalled[m[1]] = true
		}
	}
	sets := map[string][]dpkgPackageState{}
	for _, state := range states {
		m := pgLeftoverPackageRe.FindStringSubmatch(state.Name)
		if m == nil {
			continue
		}
		major := m[1]
		if serverInstalled[major] || withCluster[major] {
			continue
		}
		if _, err := os.Stat(pgServerBinary(major)); err == nil {
			continue
		}
		client := state.Name == "postgresql-client-"+major || state.Name == "postgresql-doc-"+major
		if state.State == "rc" || (state.State == "ii" && client) {
			sets[major] = append(sets[major], state)
		}
	}
	for major := range sets {
		sort.Slice(sets[major], func(i, j int) bool { return sets[major][i].Name < sets[major][j].Name })
	}
	return sets
}

// pgLeftoverPackageNames are the leftover packages, "name (rc)" for the
// clusters card and the panel's decision to run the cleanup.
func pgLeftoverPackageNames(sets map[string][]dpkgPackageState) []string {
	var names []string
	for _, set := range sets {
		for _, state := range set {
			names = append(names, state.Name)
		}
	}
	sort.Strings(names)
	return names
}

// pgLeftoverPurge is the guarded purge (a variable for tests).
var pgLeftoverPurge = func(ctx context.Context, request packageRemovalRequest) (packageRemovalPlan, error) {
	plan, _, err := guardedRemovePackagesContext(ctx, "apt", request, removalHooks{})
	return plan, err
}

// purgePGLeftoverPackages purges the leftover packages of every removed
// major. A purge the simulation refuses is retried with the rc packages
// only (they hold nothing anything depends on); a refusal of that too is
// reported and nothing changes.
func purgePGLeftoverPackages(ctx context.Context, sets map[string][]dpkgPackageState) (purged []string, notes []string) {
	majors := make([]string, 0, len(sets))
	for major := range sets {
		majors = append(majors, major)
	}
	sort.Strings(majors)
	for _, major := range majors {
		var all, rcOnly []string
		for _, state := range sets[major] {
			all = append(all, state.Name)
			if state.State == "rc" {
				rcOnly = append(rcOnly, state.Name)
			}
		}
		attempts := [][]string{all}
		if len(rcOnly) > 0 && len(rcOnly) < len(all) {
			attempts = append(attempts, rcOnly)
		}
		var last error
		for _, packages := range attempts {
			request := packageRemovalRequest{
				Packages:           packages,
				VersionPrefixes:    []string{"postgresql-" + major},
				ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet(packages)),
			}
			plan, err := pgLeftoverPurge(ctx, request)
			if err == nil {
				names := plan.Remove
				if len(names) == 0 {
					names = packages
				}
				purged = append(purged, names...)
				last = nil
				break
			}
			last = err
			var refusal *packageRemovalRefusal
			if !errors.As(err, &refusal) {
				break // a failed command is not retried with less
			}
		}
		if last != nil {
			notes = append(notes, fmt.Sprintf("PostgreSQL %s's packages (%s) were not purged: %s", major, strings.Join(all, ", "), boundedText(last.Error(), 300)))
		}
	}
	sort.Strings(purged)
	return purged, notes
}

var (
	pgWholeRemovalShared = map[string]bool{"postgresql-common": true, "postgresql-client-common": true}
	pgAnyPackageRe       = regexp.MustCompile(`^postgresql(-[a-z0-9.+-]+)?$`)
	pgClientOrDocRe      = regexp.MustCompile(`^postgresql-(client|doc)-([0-9]{1,3})$`)
)

// pgWholeRemovalExtras (alpha.130 lab): a removal of the whole PostgreSQL
// component also names what earlier removals and installs left: every
// PostgreSQL package in dpkg state rc, every major's client and
// documentation, and the shared postgresql-common / postgresql-client-common
// (a PGDG install names them exactly, which marks them as manually
// installed, so APT never took them as unused). libpq5 is not named: other
// programs (PHP's pgsql module) use it. The package manager's simulation
// still decides: anything else it would take refuses the removal.
func pgWholeRemovalExtras(states []dpkgPackageState) (packages, prefixes []string) {
	for _, state := range states {
		switch {
		case state.State == "rc" && pgAnyPackageRe.MatchString(state.Name):
			packages = append(packages, state.Name)
			if m := pgVersionPackageRe.FindStringSubmatch(state.Name); m != nil {
				prefixes = append(prefixes, state.Name)
			}
		case state.State == "ii" && (pgClientOrDocRe.MatchString(state.Name) || pgWholeRemovalShared[state.Name]):
			packages = append(packages, state.Name)
		}
	}
	sort.Strings(packages)
	sort.Strings(prefixes)
	return packages, prefixes
}

// pgVendorLibraryPackage is the client library a PGDG install names exactly.
const pgVendorLibraryPackage = "libpq5"

// pgWholeRemovalFinish runs after the whole PostgreSQL component was purged
// (alpha.130 lab): the empty /etc/postgresql, /var/lib/postgresql and
// /var/log/postgresql go (a folder with anything left in it stays, named);
// the PGDG client library goes when nothing else needs it (the guarded
// purge refuses otherwise); and when no package from PGDG is left, the
// PGDG source, key and pin CelikPanel wrote go too. It returns sentences
// for the removal's answer (a variable for tests).
var pgWholeRemovalFinish = func(ctx context.Context) []string {
	var notes []string
	for _, dir := range []string{pgConfRoot, pgDataRoot, pgLogRoot} {
		if !realDirectory(dir) {
			continue
		}
		if removeEmptyDir(dir) {
			continue
		}
		if entries, err := os.ReadDir(dir); err == nil {
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			notes = append(notes, fmt.Sprintf("%s was kept: it still holds %s", dir, boundedText(strings.Join(names, ", "), 200)))
		}
	}
	installed, err := readDpkgStatusVersions(dpkgStatusPath)
	if err != nil {
		return append(notes, "the PGDG repository was kept: the installed packages could not be read")
	}
	fromPGDG := func() []string {
		var names []string
		for name, versions := range installed {
			for _, version := range versions {
				if strings.Contains(version, "pgdg") {
					names = append(names, name)
					break
				}
			}
		}
		sort.Strings(names)
		return names
	}
	if versions := installed[pgVendorLibraryPackage]; len(versions) > 0 && strings.Contains(strings.Join(versions, " "), "pgdg") {
		request := packageRemovalRequest{Packages: []string{pgVendorLibraryPackage}, ProtectedComponent: catalogueProtectedComponent("apt", packageNameSet([]string{pgVendorLibraryPackage}))}
		if _, err := pgLeftoverPurge(ctx, request); err != nil {
			notes = append(notes, "libpq5 from PGDG was kept: "+boundedText(err.Error(), 200))
		} else if installed, err = readDpkgStatusVersions(dpkgStatusPath); err != nil {
			return append(notes, "the PGDG repository was kept: the installed packages could not be read")
		}
	}
	if left := fromPGDG(); len(left) > 0 {
		return append(notes, "the PGDG repository was kept: "+strings.Join(left, ", ")+" still come from it")
	}
	if !pathExists(repoSourcePath("pgdg")) {
		return notes
	}
	var resp RepoStatusResponse
	disableManagedRepo(ctx, "pgdg", &resp)
	if resp.Error != "" {
		return append(notes, "the PGDG repository could not be removed: "+boundedText(resp.Error, 200))
	}
	note := "CelikPanel's PGDG repository (source, key and pin) was removed"
	if resp.Confinement != nil {
		note += "; its pin " + resp.Confinement.PinPath + " was changed outside CelikPanel and was kept"
	}
	return append(notes, note)
}
