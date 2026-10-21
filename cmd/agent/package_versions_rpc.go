package main

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Installed package versions (v0.1.0-alpha.97, D-036).
//
// The Components page used to show a version only for the multi-version
// runtimes (PHP-FPM, Node.js); every other component had none, because the
// only other source, a per-unit binary probe, is not a package version and is
// unreliable. This read asks the system package database itself, once per
// component scan, for the installed version of each catalogue package:
//
//   - read-only: `dpkg-query -W` (Debian/Ubuntu) or `pacman -Q` (Arch), fixed
//     argv, no shell, no network, no package-manager lock taken;
//   - bounded: one query per scan for the whole catalogue, a timeout and an
//     output ceiling;
//   - never while a package operation runs: the process table and the
//     package-manager lock table are read first (both read-only) and a busy
//     host answers "busy", so the Panel keeps its previous answer, marked;
//   - unknown stays unknown: a component with no installed catalogue package
//     is not listed, and a version text that is overlong or carries characters
//     a package database does not print is dropped, never shortened.
//
// Kurulu paket sürümleri: bileşen taraması başına bir kez, sistemin paket
// veritabanına salt-okur, sınırlı ve kilitsiz sorulur. Paket işlemi sürerken
// sorulmaz ("busy"); bilinmeyen bilinmez kalır.

const (
	packageVersionOutputLimit = 8 << 20
	packageVersionMaxLength   = 64
)

// packageVersionQueryTimeout bounds the one query; a variable for tests.
var packageVersionQueryTimeout = 10 * time.Second

// packageVersionText is the character set dpkg and pacman print in a version:
// digits, letters, and . + ~ : _ - (epoch and revision included).
var packageVersionText = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+~:_-]*$`)

// Injectable for tests: the busy probe and the query runner.
var (
	packageVersionBusyProbe = realPackageDatabaseReadBusy
	packageVersionQuery     = func(ctx context.Context, executable string, args ...string) ([]byte, error) {
		return serviceMutationCommand(ctx, executable, args...).OutputLimited(packageVersionOutputLimit)
	}
	packageVersionHostProfile = verifiedHostProfileForAnyFamily
)

var errPackageVersionOutputTooLarge = errors.New("package database answer is larger than the read ceiling")

// InstalledPackageVersionsV1 reports the installed package versions of the
// catalogue components. It takes no argument: which packages are asked about
// is the agent's own compiled catalogue, never caller data.
func (a *Agent) InstalledPackageVersionsV1(_ *transport.Empty, resp *transport.InstalledPackageVersionsResponse) error {
	*resp = readInstalledPackageVersions(context.Background())
	return nil
}

func readInstalledPackageVersions(parent context.Context) transport.InstalledPackageVersionsResponse {
	answer := transport.InstalledPackageVersionsResponse{Components: []transport.ComponentPackageVersions{}}
	profile, err := packageVersionHostProfile()
	if err != nil {
		answer.State = transport.PackageVersionsFailed
		answer.Error = "the host platform is not verified"
		return answer
	}
	family := string(profile.PackageManager)
	answer.Family = family
	var executable string
	var args []string
	switch family {
	case "apt":
		executable, err = executableForProfile(profile, family, "dpkg-query")
		args = dpkgVersionQueryArgs()
	case "pacman":
		executable, err = executableForProfile(profile, family, "pacman")
		args = pacmanVersionQueryArgs()
	default:
		answer.State = transport.PackageVersionsUnsupported
		return answer
	}
	if err != nil {
		answer.State = transport.PackageVersionsFailed
		answer.Error = "the package database tool is not verified"
		return answer
	}
	busy, err := packageVersionBusyProbe()
	if err != nil {
		// A probe that cannot say "idle" is treated as busy: nothing is read.
		answer.State = transport.PackageVersionsBusy
		return answer
	}
	if busy {
		answer.State = transport.PackageVersionsBusy
		return answer
	}
	ctx, cancel := context.WithTimeout(parent, packageVersionQueryTimeout)
	defer cancel()
	output, err := packageVersionQuery(ctx, executable, args...)
	if err == nil && len(output) > packageVersionOutputLimit {
		err = errPackageVersionOutputTooLarge
	}
	if err != nil {
		answer.State = transport.PackageVersionsFailed
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			answer.Error = "the package database did not answer in time"
		case errors.Is(err, errPackageVersionOutputTooLarge):
			answer.Error = err.Error()
		default:
			answer.Error = "the package database query failed"
		}
		return answer
	}
	var installed map[string][]string
	if family == "apt" {
		installed = parseDpkgVersions(string(output))
	} else {
		installed = parsePacmanVersions(string(output))
	}
	answer.State = transport.PackageVersionsRead
	answer.Components = componentPackageVersions(installed, family)
	return answer
}

// dpkgVersionQueryArgs lists every package with its state and version. One
// query for the whole catalogue: naming packages would make dpkg-query fail
// for each absent one.
func dpkgVersionQueryArgs() []string {
	return []string{"-W", "-f=${binary:Package}\t${db:Status-Abbrev}\t${Version}\n"}
}

// pacmanVersionQueryArgs lists every installed package and its version from
// the local database; -Q takes no lock.
func pacmanVersionQueryArgs() []string {
	return []string{"-Q"}
}

// cleanPackageVersion returns the version as printed, or "" when it is not one
// a package database prints (odd characters, or longer than the ceiling). It is
// never shortened: a cut version would be a guess.
func cleanPackageVersion(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > packageVersionMaxLength || !packageVersionText.MatchString(value) {
		return ""
	}
	return value
}

// parseDpkgVersions reads "name[:arch]\tstatus\tversion" rows. Only a package
// whose state is installed ("ii") counts; a half-installed or removed one has
// no version to show. A name seen for two architectures keeps both versions.
func parseDpkgVersions(output string) map[string][]string {
	installed := map[string][]string{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) != 3 || strings.TrimSpace(fields[1]) != "ii" {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if colon := strings.IndexByte(name, ':'); colon > 0 {
			name = name[:colon]
		}
		if !validPackageName(name) {
			continue
		}
		installed[name] = append(installed[name], cleanPackageVersion(fields[2]))
	}
	return installed
}

// parsePacmanVersions reads "name version" rows of `pacman -Q`.
func parsePacmanVersions(output string) map[string][]string {
	installed := map[string][]string{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !validPackageName(fields[0]) {
			continue
		}
		installed[fields[0]] = append(installed[fields[0]], cleanPackageVersion(fields[1]))
	}
	return installed
}

// componentPackageVersions maps the installed packages onto the catalogue. A
// component with a versioned repository pattern (PostgreSQL's postgresql-17)
// reports its versioned packages when any is installed, because the catalogue
// name is then a meta-package whose version is not the server's; otherwise
// the catalogue's own package names for this family. A component with no
// installed package is left out (unknown). A version that was dropped as
// unreadable stays as an empty string, so the Panel says nothing rather than
// a version of the other packages.
func componentPackageVersions(installed map[string][]string, family string) []transport.ComponentPackageVersions {
	components := []transport.ComponentPackageVersions{}
	for i := range core.ManagedServices {
		service := &core.ManagedServices[i]
		names := []string{}
		if service.Repo != nil && service.Repo.PackagePattern != "" {
			if pattern, err := regexp.Compile(service.Repo.PackagePattern); err == nil {
				for name := range installed {
					if pattern.FindString(name) == name {
						names = append(names, name)
					}
				}
				sort.Strings(names)
			}
		}
		if len(names) == 0 {
			names = append(names, service.Packages[family]...)
		}
		packages := []transport.InstalledPackageVersion{}
		for _, name := range names {
			for _, version := range installed[name] {
				packages = append(packages, transport.InstalledPackageVersion{Package: name, Version: version})
			}
		}
		if len(packages) == 0 {
			continue
		}
		components = append(components, transport.ComponentPackageVersions{ServiceID: service.ID, Packages: packages})
	}
	return components
}
