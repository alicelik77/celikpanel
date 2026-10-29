//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/hostplatform"
)

// A fresh paired PowerDNS primary is admitted only on a release of the tested
// PowerDNS series (dnsengineartifact.PDNSFreshPrimaryTestedSeries, D-028 as
// amended): the same rule the setup wizard uses to offer PowerDNS as the main
// DNS server, so what is offered is what this step accepts. The version the
// package manager WOULD install is read before any receipt, mask, package or
// journal effect, and the install itself passes "name=version" for that one
// version. The post-install identity check stays: both packages at that one
// version and the daemon the package's own file.
//
// Eşli PowerDNS birincilinin ilk kurulumu yalnız denenmiş PowerDNS serisinin
// (major.minor) bir sürümüyle kabul edilir; sihirbaz da aynı kuralla sunar.
// Seri içi yama sürümü dağıtımın güvenlik düzeltmesidir. Kuracağı sürüm her
// etkiden önce okunur; kurulum "ad=sürüm" ile yapılır.

type freshPDNSVersionOpsV3 struct {
	// installed returns the installed version of a package that is present.
	installed func(string) (string, error)
	// candidate returns the version apt would install ("" when none).
	candidate func(string) (string, error)
	// archives returns the archives APT names for name=version (read-only
	// simulation); none when it names none.
	archives func(name, version string) ([]dnsengineartifact.PDNSArchive, error)
}

// pdnsArchivesText names the archives for a refusal sentence.
func pdnsArchivesText(archives []dnsengineartifact.PDNSArchive) string {
	if len(archives) == 0 {
		return "an archive APT could not name"
	}
	names := make([]string, 0, len(archives))
	for _, archive := range archives {
		names = append(names, freshPDNSVersionText(archive.Origin+" "+archive.Suite))
	}
	return strings.Join(names, ", ")
}

func freshPDNSVersionText(value string) string {
	return boundedStatOverrideText(value)
}

// preflightFreshPDNSPackageVersionsWithOpsV3 refuses, before any mutation,
// unless every package of the set is (or would be installed) at one release
// of the tested series. It returns that version for the pinned install.
func preflightFreshPDNSPackageVersionsWithOpsV3(packages, missing []string, ops freshPDNSVersionOpsV3) (string, error) {
	if ops.installed == nil || ops.candidate == nil || ops.archives == nil || len(packages) == 0 {
		return "", errors.New("v3 PowerDNS version preflight is incomplete")
	}
	absent := map[string]bool{}
	for _, name := range missing {
		absent[name] = true
	}
	series := dnsengineartifact.PDNSFreshPrimaryTestedSeries
	chosen, chosenFrom := "", ""
	for _, name := range packages {
		var version string
		if absent[name] {
			offered, err := ops.candidate(name)
			if err != nil {
				return "", &hostOperatorRefusal{
					sentence: fmt.Sprintf(
						"PowerDNS was not installed and nothing was changed: CelikPanel could not read which %s version the package manager on this server would install. "+
							"Start the same change again; if this repeats, the server owner checks the package sources with `apt-cache policy %s`.",
						name, name),
					cause: err,
				}
			}
			if offered == "" {
				return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
					"PowerDNS was not installed and nothing was changed: the package manager on this server offers no installable %s, "+
						"and CelikPanel installs a fresh paired PowerDNS primary only from the tested PowerDNS %s series. "+
						"The server owner makes %s installable for APT on this server (the Debian 13 repository that carries it), then starts the same change again.",
					name, series, name)}
			}
			if !validDebianVersion(offered) || !dnsengineartifact.PDNSFreshPrimaryVersionInTestedSeries(offered) {
				return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
					"PowerDNS was not installed and nothing was changed: the package manager on this server would install %s %s, "+
						"but CelikPanel installs a fresh paired PowerDNS primary only from the tested PowerDNS %s series. "+
						"Choose BIND as the main DNS server, or wait for a CelikPanel release that names %s as tested, then start the same change again.",
					name, freshPDNSVersionText(offered), series, freshPDNSVersionText(offered))}
			}
			// Provenance: the build must come from Debian's own archive.
			archives, err := ops.archives(name, offered)
			if err != nil {
				return "", &hostOperatorRefusal{
					sentence: fmt.Sprintf(
						"PowerDNS was not installed and nothing was changed: CelikPanel could not read which archive %s %s would come from. "+
							"Start the same change again; if this repeats, the server owner checks the package sources with `apt-cache policy %s`.",
						name, freshPDNSVersionText(offered), name),
					cause: err,
				}
			}
			if !dnsengineartifact.PDNSOfficialDebianArchive(archives) {
				return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
					"PowerDNS was not installed and nothing was changed: %s %s would come from %s, "+
						"but CelikPanel installs a fresh paired PowerDNS primary only from Debian's own archive. "+
						"Choose BIND as the main DNS server, or the server owner removes or confines that repository (for example repo.powerdns.com or backports) so APT takes Debian's build, then starts the same change again.",
					name, freshPDNSVersionText(offered), pdnsArchivesText(archives))}
			}
			version = offered
		} else {
			installed, err := ops.installed(name)
			if err != nil {
				return "", &hostOperatorRefusal{
					sentence: fmt.Sprintf(
						"PowerDNS was not installed and nothing was changed: CelikPanel could not read the installed %s version. "+
							"Start the same change again; if this repeats, the server owner checks it with `dpkg -s %s`.",
						name, name),
					cause: err,
				}
			}
			if !validDebianVersion(installed) || !dnsengineartifact.PDNSFreshPrimaryVersionInTestedSeries(installed) {
				return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
					"PowerDNS was not installed and nothing was changed: %s %s is already installed on this server, "+
						"but CelikPanel runs a fresh paired PowerDNS primary only from the tested PowerDNS %s series. "+
						"The server owner replaces it with the distribution's PowerDNS %s release, or waits for a CelikPanel release that names %s as tested, then starts the same change again.",
					name, freshPDNSVersionText(installed), series, series, freshPDNSVersionText(installed))}
			}
			version = installed
		}
		if chosen == "" {
			chosen, chosenFrom = version, name
			continue
		}
		if version != chosen {
			return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
				"PowerDNS was not installed and nothing was changed: %s %s and %s %s are not one PowerDNS build. "+
					"The server owner brings both to the same version with `apt-get install --only-upgrade %s %s`, then starts the same change again.",
				chosenFrom, freshPDNSVersionText(chosen), name, freshPDNSVersionText(version), chosenFrom, name)}
		}
	}
	return chosen, nil
}

func preflightFreshPDNSPackageVersionsV3(ctx context.Context, profile hostplatform.Profile, packages, missing []string) (string, error) {
	if ctx == nil || profile.PackageManager != hostplatform.PackageManagerAPT {
		return "", errors.New("v3 PowerDNS version preflight requires an APT host")
	}
	dpkg, err := executableForProfile(profile, string(profile.PackageManager), "dpkg-query")
	if err != nil {
		return "", err
	}
	aptGet, err := executableForProfile(profile, string(profile.PackageManager), "apt-get")
	if err != nil {
		return "", err
	}
	aptCache, err := executableForProfile(profile, string(profile.PackageManager), "apt-cache")
	if err != nil {
		return "", err
	}
	refreshed := false
	return preflightFreshPDNSPackageVersionsWithOpsV3(packages, missing, freshPDNSVersionOpsV3{
		installed: func(name string) (string, error) {
			command := serviceMutationCommand(ctx, dpkg, "-W", "-f", "${Status}\t${Version}", "--", name)
			command.Env = bindSafeAPTCommandEnvironment()
			output, err := command.CombinedOutputLimited(64 << 10)
			if err != nil {
				return "", err
			}
			version, ok := strings.CutPrefix(string(output), "install ok installed\t")
			if !ok || !validDebianVersion(version) {
				return "", errors.New("dpkg-query returned a non-canonical installed package record")
			}
			return version, nil
		},
		candidate: func(name string) (string, error) {
			// The same lock and freshness rule the install uses; the install
			// re-checks the candidate under the lock and passes name=version.
			packageOperationMu.Lock()
			defer packageOperationMu.Unlock()
			if !refreshed {
				refreshAptListsIfStaleWithExecutable(ctx, time.Hour, aptGet)
				refreshed = true
			}
			return aptInstallCandidateWithExecutable(ctx, aptCache, name)
		},
		archives: func(name, version string) ([]dnsengineartifact.PDNSArchive, error) {
			packageOperationMu.Lock()
			defer packageOperationMu.Unlock()
			sim, err := runAptSimulation(ctx, runPackageSimulationCommand, aptGet, aptInstallArgs([]string{name + "=" + version}))
			if err != nil {
				return nil, err
			}
			return pdnsArchivesOf(sim, name, version), nil
		},
	})
}

// pdnsArchivesOf is the archives a simulation names for name at version.
func pdnsArchivesOf(sim aptSimulation, name, version string) []dnsengineartifact.PDNSArchive {
	line, ok := sim.Versions[name]
	if !ok || line.To != version {
		return nil
	}
	archives := make([]dnsengineartifact.PDNSArchive, 0, len(line.Origins))
	for _, origin := range line.Origins {
		archives = append(archives, dnsengineartifact.PDNSArchive{Origin: origin.Origin, Suite: origin.Suite})
	}
	return archives
}

// installFreshPDNSPackagesV3 installs the missing packages at the one tested
// version the preflight read.
func installFreshPDNSPackagesV3(ctx context.Context, missing []string, version string) (string, error) {
	if !dnsengineartifact.PDNSFreshPrimaryVersionInTestedSeries(version) {
		return "", errors.New("v3 PowerDNS install version is outside the tested series")
	}
	return installPackagesAtVersionContext(ctx, missing, version)
}
