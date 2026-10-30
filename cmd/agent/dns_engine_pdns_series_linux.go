//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/pacmanlocal"
	"github.com/alicelik/celikpanel/internal/transport"
)

// After an install, the PowerDNS package belongs to the operating system's
// own update path. On Arch Linux (rolling) the owner's `pacman -Syu` can move
// powerdns to a release outside the tested series (5.1 → 5.2). PowerDNS keeps
// running from its native configuration and keeps answering (D-022: the panel
// never stops hosted DNS); CelikPanel only stops publishing changes into a
// daemon it has not measured, says so on the DNS screens (engine detail code
// engine_series_untested, D-024) and resumes by itself once the installed
// release is in a tested series again, for example after a CelikPanel update
// that names it. A pacman update inside the series is not this case.
//
// Kurulumdan sonra PowerDNS paketi işletim sisteminin güncelleme yoluna aittir.
// Arch'ta sahibin `pacman -Syu`'su powerdns'i denenmiş serinin dışına
// taşıyabilir. PowerDNS yanıt vermeye devam eder; CelikPanel yalnız ölçmediği
// bir daemon'a değişiklik yayımlamayı durdurur, DNS ekranlarında söyler ve
// kurulu sürüm yeniden denenmiş bir seride olunca kendiliğinden sürer.

// pdnsSeriesBuildForState is the tested build that governs an active
// PowerDNS state: the native catalog tenure's own build, or the Arch build
// for any PowerDNS state on a pacman host. Debian/Ubuntu states without a
// native tenure keep their earlier, version-independent behaviour.
func pdnsSeriesBuildForState(profile hostplatform.Profile, state dnsEngineStateReceipt) (dnsengineartifact.PDNSFreshPrimaryBuild, bool) {
	if state.Engine != transport.DNSEnginePowerDNS {
		return dnsengineartifact.PDNSFreshPrimaryBuild{}, false
	}
	if build, ok := dnsengineartifact.PDNSFreshPrimaryBuildForKind(state.NativeCatalogV3); ok {
		return build, build.PackageManager == string(profile.PackageManager)
	}
	if profile.PackageManager == hostplatform.PackageManagerPacman {
		return dnsengineartifact.PDNSFreshPrimaryBuildFor("pacman")
	}
	return dnsengineartifact.PDNSFreshPrimaryBuild{}, false
}

// installedPDNSBuildVersion reads the installed version of the build's
// daemon package from the package database.
func installedPDNSBuildVersion(ctx context.Context, profile hostplatform.Profile, build dnsengineartifact.PDNSFreshPrimaryBuild) (string, error) {
	if len(build.Packages) == 0 {
		return "", errors.New("PowerDNS build names no package")
	}
	switch build.PackageManager {
	case "pacman":
		pkg, err := pacmanlocal.Installed(pacmanlocal.DBDir, build.Packages[0])
		if err != nil {
			return "", err
		}
		return pkg.Version, nil
	case "apt":
		dpkg, err := executableForProfile(profile, string(profile.PackageManager), "dpkg-query")
		if err != nil {
			return "", err
		}
		command := serviceMutationCommand(ctx, dpkg, "-W", "-f", "${Status}\t${Version}", "--", build.Packages[0])
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
	}
	return "", errors.New("PowerDNS package family is unsupported")
}

// pdnsUntestedInstalledVersion returns the installed PowerDNS version when an
// active PowerDNS state's package is outside its tested series, "" when it is
// inside (or the rule does not apply). An unreadable version is an error.
func pdnsUntestedInstalledVersion(ctx context.Context, profile hostplatform.Profile, state dnsEngineStateReceipt) (string, error) {
	build, applies := pdnsSeriesBuildForState(profile, state)
	if !applies {
		return "", nil
	}
	version, err := installedPDNSBuildVersion(ctx, profile, build)
	if err != nil {
		return "", err
	}
	if build.VersionInTestedSeries(version) {
		return "", nil
	}
	return version, nil
}

// pdnsSeriesUntestedRefusal is the owner sentence of a held DNS change.
func pdnsSeriesUntestedRefusal(build dnsengineartifact.PDNSFreshPrimaryBuild, version string) error {
	return &hostOperatorRefusal{sentence: fmt.Sprintf(
		"This DNS change was not published and nothing was changed: a system update moved PowerDNS on this server to %s, "+
			"outside the PowerDNS %s series CelikPanel has tested. PowerDNS keeps answering with the records it has. "+
			"Nobody needs to undo the update: update CelikPanel from its own update screen when a release names this PowerDNS version as tested, "+
			"and the held change can then be made again.",
		freshPDNSVersionText(version), build.Series)}
}

// requirePDNSNativeSeriesForManagement refuses a DNS change into a PowerDNS
// whose installed release is outside its tested series.
func requirePDNSNativeSeriesForManagement(ctx context.Context, profile hostplatform.Profile, state dnsEngineStateReceipt) error {
	build, applies := pdnsSeriesBuildForState(profile, state)
	if !applies {
		return nil
	}
	version, err := installedPDNSBuildVersion(ctx, profile, build)
	if err != nil {
		return fmt.Errorf("read the installed PowerDNS version: %w", err)
	}
	if !build.VersionInTestedSeries(version) {
		return pdnsSeriesUntestedRefusal(build, version)
	}
	return nil
}
