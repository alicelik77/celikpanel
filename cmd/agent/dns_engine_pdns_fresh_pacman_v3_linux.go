//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/dnsengineartifact"
	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/pacmanlocal"
	"github.com/alicelik/celikpanel/internal/transport"
	"golang.org/x/sys/unix"
)

// Arch Linux adapter of the fresh paired PowerDNS primary (D-028 amendment,
// 2026-10-30). The tested build is extra/powerdns 5.1 (one package, daemon in
// /usr/bin, service account powerdns). Arch supports only full-system
// upgrades, so the install refreshes the package databases and upgrades the
// system in one pacman transaction with the package, exactly like every other
// pacman install of CelikPanel; the candidate is read again from the
// refreshed databases, under the package lock, immediately before that
// transaction, and nothing is installed unless it is extra's build of the
// tested series. The post-install identity check stays.
//
// Arch bağdaştırıcısı: denenmiş yapı extra/powerdns 5.1'dir. Aday, kurulumdan
// hemen önce tazelenmiş veritabanından yeniden okunur; denenmiş seri ve resmi
// extra deposu değilse hiçbir şey kurulmaz.

// pacmanCandidate is one "%r %n %v" line of pacman's print output.
type pacmanCandidate struct {
	Repository, Name, Version string
}

// pacmanCandidateFromPrint finds the package's line in pacman's
// --print-format "%r %n %v" output; ok is false when pacman names it not
// exactly once.
func pacmanCandidateFromPrint(output []byte, name string) (pacmanCandidate, bool) {
	found := pacmanCandidate{}
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != name {
			continue
		}
		found = pacmanCandidate{Repository: fields[0], Name: fields[1], Version: fields[2]}
		count++
	}
	return found, count == 1 && pacmanlocal.ValidVersion(found.Version)
}

func pacmanPDNSPrintArgs(refresh bool, name string) []string {
	flags := "-Sp"
	if refresh {
		flags = "-Syup"
	}
	return []string{flags, "--needed", "--print-format", "%r %n %v", dnsengineartifact.PDNSOfficialArchRepository + "/" + name}
}

// freshPDNSPacmanCandidateRefusal returns the owner sentence for a candidate
// outside the tested build, or nil.
func freshPDNSPacmanCandidateRefusal(build dnsengineartifact.PDNSFreshPrimaryBuild, candidate pacmanCandidate) error {
	if candidate.Repository != dnsengineartifact.PDNSOfficialArchRepository {
		return &hostOperatorRefusal{sentence: fmt.Sprintf(
			"PowerDNS was not installed and nothing was changed: %s %s would come from the %q repository, "+
				"but CelikPanel installs a fresh paired PowerDNS primary only from Arch Linux's official extra repository. "+
				"Choose BIND as the main DNS server, or the server owner removes that repository from /etc/pacman.conf so pacman takes extra's build, then starts the same change again.",
			candidate.Name, freshPDNSVersionText(candidate.Version), freshPDNSVersionText(candidate.Repository))}
	}
	if !build.VersionInTestedSeries(candidate.Version) {
		return &hostOperatorRefusal{sentence: fmt.Sprintf(
			"PowerDNS was not installed and nothing was changed: Arch Linux now offers %s %s, "+
				"but CelikPanel installs a fresh paired PowerDNS primary only from the tested PowerDNS %s series. "+
				"Choose BIND as the main DNS server, or wait for a CelikPanel release that names %s as tested, then start the same change again.",
			candidate.Name, freshPDNSVersionText(candidate.Version), build.Series, freshPDNSVersionText(candidate.Version))}
	}
	return nil
}

// preflightFreshPDNSPacmanVersionV3 reads, before any receipt, mask, package
// or journal effect, the installed or offered powerdns build (from the
// current package databases, without refreshing them) and refuses one outside
// the tested build.
func preflightFreshPDNSPacmanVersionV3(ctx context.Context, profile hostplatform.Profile, build dnsengineartifact.PDNSFreshPrimaryBuild, missing []string) (string, error) {
	if ctx == nil || build.PackageManager != "pacman" || len(build.Packages) != 1 {
		return "", errors.New("v3 PowerDNS pacman preflight requires the Arch build")
	}
	name := build.Packages[0]
	if len(missing) == 0 {
		pkg, err := pacmanlocal.Installed(pacmanlocal.DBDir, name)
		if err != nil {
			return "", &hostOperatorRefusal{
				sentence: fmt.Sprintf("PowerDNS was not installed and nothing was changed: CelikPanel could not read the installed %s version. "+
					"Start the same change again; if this repeats, the server owner checks it with `pacman -Qi %s`.", name, name),
				cause: err,
			}
		}
		if !build.VersionInTestedSeries(pkg.Version) {
			return "", &hostOperatorRefusal{sentence: fmt.Sprintf(
				"PowerDNS was not installed and nothing was changed: %s %s is already installed on this server, "+
					"but CelikPanel runs a fresh paired PowerDNS primary only from the tested PowerDNS %s series. "+
					"Choose BIND as the main DNS server, or wait for a CelikPanel release that names %s as tested, then start the same change again.",
				name, freshPDNSVersionText(pkg.Version), build.Series, freshPDNSVersionText(pkg.Version))}
		}
		return pkg.Version, nil
	}
	if len(missing) != 1 || missing[0] != name {
		return "", errors.New("v3 PowerDNS pacman preflight names an unexpected package")
	}
	pacman, err := executableForProfile(profile, "pacman", "pacman")
	if err != nil {
		return "", err
	}
	packageOperationMu.Lock()
	output, runErr := runServiceMutationCombinedOutputEnv(ctx, packageCommandEnv(), pacman, pacmanPDNSPrintArgs(false, name)...)
	packageOperationMu.Unlock()
	candidate, ok := pacmanCandidateFromPrint(output, name)
	if runErr != nil || !ok {
		return "", &hostOperatorRefusal{
			sentence: fmt.Sprintf("PowerDNS was not installed and nothing was changed: CelikPanel could not read which %s version pacman would install from Arch Linux's extra repository. "+
				"The server owner updates the system with `pacman -Syu` (CelikPanel never upgrades Arch on its own), then starts the same change again.", name),
			cause: errors.Join(runErr, errors.New(firstLine(lastLines(string(output), 3)))),
		}
	}
	if err := freshPDNSPacmanCandidateRefusal(build, candidate); err != nil {
		return "", err
	}
	return candidate.Version, nil
}

// installFreshPDNSPacmanV3 refreshes the package databases and reads the
// candidate again (pacman -Syup prints, installs nothing), then upgrades the
// system with extra/powerdns in one transaction (pacman -Su, no second
// refresh, so no partial upgrade). A candidate outside the tested build
// installs nothing.
func installFreshPDNSPacmanV3(ctx context.Context, build dnsengineartifact.PDNSFreshPrimaryBuild, missing []string) (string, error) {
	if ctx == nil || build.PackageManager != "pacman" || len(build.Packages) != 1 ||
		len(missing) != 1 || missing[0] != build.Packages[0] {
		return "", errors.New("v3 PowerDNS pacman install requires the Arch build")
	}
	name := build.Packages[0]
	profile, err := verifiedHostProfile("pacman")
	if err != nil {
		return "", err
	}
	pacman, err := executableForProfile(profile, "pacman", "pacman")
	if err != nil {
		return "", err
	}
	packageOperationMu.Lock()
	defer packageOperationMu.Unlock()
	printed, err := runServiceMutationCombinedOutputEnv(ctx, packageCommandEnv(), pacman, pacmanPDNSPrintArgs(true, name)...)
	candidate, ok := pacmanCandidateFromPrint(printed, name)
	if err != nil || !ok {
		return "", newPackageManagerCommandError([]string{name}, printed, errors.Join(err, errors.New("pacman did not name the powerdns candidate")))
	}
	if err := freshPDNSPacmanCandidateRefusal(build, candidate); err != nil {
		return "", err
	}
	args := []string{"-Su", "--noconfirm", "--needed", dnsengineartifact.PDNSOfficialArchRepository + "/" + name}
	out, err := runServiceMutationCombinedOutputEnv(ctx, packageCommandEnv(), pacman, args...)
	if err != nil {
		return "", newPackageManagerCommandError([]string{name}, out, err)
	}
	return string(out), nil
}

// pdnsFreshCandidatePacmanResponse reads, read-only and without refreshing,
// which powerdns build pacman would install, for the setup wizard.
func pdnsFreshCandidatePacmanResponse(ctx context.Context, profile hostplatform.Profile) transport.PDNSFreshPrimaryCandidateResponse {
	resp := transport.PDNSFreshPrimaryCandidateResponse{State: "unknown"}
	build, ok := dnsengineartifact.PDNSFreshPrimaryBuildFor("pacman")
	if !ok {
		return resp
	}
	if _, err := pacmanlocal.Installed(pacmanlocal.DBDir, build.Packages[0]); !errors.Is(err, os.ErrNotExist) {
		return resp
	}
	pacman, err := executableForProfile(profile, "pacman", "pacman")
	if err != nil {
		return resp
	}
	if !packageOperationMu.TryLock() {
		return resp
	}
	defer packageOperationMu.Unlock()
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	output, err := runServiceMutationCombinedOutputEnv(readCtx, packageCommandEnv(), pacman, pacmanPDNSPrintArgs(false, build.Packages[0])...)
	candidate, ok := pacmanCandidateFromPrint(output, build.Packages[0])
	if err != nil || !ok {
		return resp
	}
	resp.State, resp.Version = "known", candidate.Version
	resp.Official = candidate.Repository == dnsengineartifact.PDNSOfficialArchRepository
	return resp
}

// preparePacmanPDNSDirectoriesV3 creates, on Arch only, the two directories
// the Debian package ships and Arch's does not, before any config capture or
// journal: /etc/powerdns/pdns.d (root:root 0755, the managed include
// directory) and /var/lib/powerdns (powerdns:powerdns 0750, where the daemon
// writes its SQLite journal files). An existing directory is left exactly as
// it is for the later proofs to judge. They are native, panel-independent
// paths that stay with PowerDNS if CelikPanel is removed (D-022).
func preparePacmanPDNSDirectoriesV3(profile hostplatform.Profile) error {
	if certifyPacmanPDNSCapabilities(profile) != nil {
		return nil
	}
	account, err := user.Lookup("powerdns")
	if err != nil {
		return fmt.Errorf("PowerDNS service account is missing after the powerdns package install: %w", err)
	}
	serviceGroup, err := user.LookupGroup("powerdns")
	if err != nil {
		return fmt.Errorf("PowerDNS service group is missing after the powerdns package install: %w", err)
	}
	uid, uidErr := strconv.Atoi(account.Uid)
	group, gidErr := strconv.Atoi(serviceGroup.Gid)
	if uidErr != nil || gidErr != nil || uid <= 0 || group <= 0 || account.Gid != serviceGroup.Gid {
		return errors.New("PowerDNS service account is not the powerdns package's own user and group")
	}
	for _, dir := range []struct {
		path      string
		uid, gid  int
		mode      uint32
		parentDir string
	}{
		{path: "/etc/powerdns/pdns.d", uid: 0, gid: 0, mode: 0o755, parentDir: "/etc/powerdns"},
		{path: "/var/lib/powerdns", uid: uid, gid: group, mode: 0o750, parentDir: "/var/lib"},
	} {
		if _, err := os.Lstat(dir.path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent, err := os.Lstat(dir.parentDir)
		if err != nil || !parent.IsDir() || parent.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s is not a safe parent for %s", dir.parentDir, dir.path)
		}
		if err := unix.Mkdir(dir.path, dir.mode); err != nil && !errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("create %s: %w", dir.path, err)
		}
		if err := unix.Lchown(dir.path, dir.uid, dir.gid); err != nil {
			return fmt.Errorf("own %s: %w", dir.path, err)
		}
		if err := unix.Chmod(dir.path, dir.mode); err != nil {
			return fmt.Errorf("set the mode of %s: %w", dir.path, err)
		}
		if err := syncAtomicParentDirectory(dir.parentDir); err != nil {
			return err
		}
	}
	return nil
}
