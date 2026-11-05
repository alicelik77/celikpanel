//go:build linux

package hostplatform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type fixedExecutableContract struct {
	path                 string
	allowedSymlinkTarget string
}

var fixedExecutableContracts = map[string]fixedExecutableContract{
	"apt-get":      {path: "/usr/bin/apt-get"},
	"apt-cache":    {path: "/usr/bin/apt-cache"},
	"dpkg-query":   {path: "/usr/bin/dpkg-query"},
	"pacman":       {path: "/usr/bin/pacman"},
	"dnf":          {path: "/usr/bin/dnf", allowedSymlinkTarget: "/usr/bin/dnf-3"},
	"rpm":          {path: "/usr/bin/rpm"},
	"systemctl":    {path: "/usr/bin/systemctl"},
	"timeout":      {path: "/usr/bin/timeout"},
	"restorecon":   {path: "/usr/sbin/restorecon"},
	"matchpathcon": {path: "/usr/sbin/matchpathcon"},
	"getenforce":   {path: "/usr/sbin/getenforce"},
}

const (
	systemdReadinessTimeout = 3 * time.Second
	dnfSecurityProbeTimeout = 3 * time.Second
)

func Detect() (Profile, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return Profile{}, fmt.Errorf("read /etc/os-release: %w", err)
	}
	return DetectWith(data, Probe{
		ExecutablePresent:   fixedExecutablePresent,
		LookPath:            fixedExecutablePath,
		ValidateExecutable:  validateFixedExecutable,
		SecurityPolicyState: InspectLiveSecurityPolicy,
		DNFSecurityReady:    verifyDNFSecurityPolicy,
		SystemdReady: func(systemctl string) error {
			if err := validateRootOwnedPathChain("/run/systemd"); err != nil {
				// A machine that is still creating its runtime tree has not
				// refused anything yet. The same absence on a host that never
				// had systemd is caught a moment later, when is-system-running
				// reports offline, and that answer is durable.
				// Calisma zamani agacini henuz olusturan bir makine hicbir sey
				// reddetmis degildir.
				return earlyBootAbsence(err)
			}
			info, err := os.Lstat("/run/systemd/private")
			if err != nil {
				return earlyBootAbsence(err)
			}
			if info.Mode()&os.ModeSocket == 0 {
				return fmt.Errorf("/run/systemd/private is not a socket")
			}
			if err := validateSystemdPrivateSocket("/run/systemd/private", info); err != nil {
				return err
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Nlink != 1 {
				return fmt.Errorf("/run/systemd/private must have exactly one hard link")
			}
			ctx, cancel := context.WithTimeout(context.Background(), systemdReadinessTimeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, systemctl, "is-system-running").Output()
			if ctx.Err() != nil {
				// A boot busy enough to starve this probe is the same "not yet"
				// the starting state reports; a manager that never answers is
				// caught by the caller's own bounded window rather than by
				// declaring the host unmanageable on one slow read.
				// Bu yoklamayi ac birakacak kadar mesgul bir acilis, starting
				// durumunun bildirdigi "henuz degil" ile aynidir.
				return stillStarting(
					fmt.Errorf("systemd readiness probe timed out: %w", ctx.Err()),
				)
			}
			status := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					return fmt.Errorf("systemd readiness probe failed: %w", err)
				}
				status = exitErr.ExitCode()
			}
			return validateSystemdReadinessResult(out, status)
		},
		Architecture: func() (string, error) {
			return runtime.GOARCH, nil
		},
	})
}

func validateSystemdReadinessResult(out []byte, status int) error {
	state := strings.TrimRight(string(out), "\n")
	switch {
	case status == 0 && (state == "running" || state == "degraded"):
		return nil
	case status == 1 && state == "degraded":
		return nil
	case systemdStateIsATransition(state):
		// R-048: at boot the agent can start before the boot transaction ends,
		// and this refusal is what stopped the startup recovery from running at
		// all. The states below are ones systemd is passing through, not states
		// it settled in: what they mean is "ask again", never "this host cannot
		// be managed". Nothing here is accepted as ready - only classified.
		//
		// R-048: acilista agent, acilis islemi bitmeden baslayabilir ve bu ret,
		// baslangic kurtarmasinin hic calismamasinin sebebiydi. Asagidaki
		// durumlar systemd'nin icinden gectigi durumlardir; anlamlari "tekrar
		// sor"dur, "bu makine yonetilemez" degil. Burada hicbir sey hazir
		// sayilmaz, yalnizca siniflandirilir.
		return stillStarting(
			fmt.Errorf("systemd state %q exited with status %d", state, status),
		)
	default:
		return fmt.Errorf("systemd state %q exited with status %d", state, status)
	}
}

// systemdStateIsATransition names the is-system-running states the manager
// leaves on its own. initializing and starting are the boot transaction still
// running; stopping is the machine going down, where the honest answer is also
// "not now" - a shutdown must never be read as a broken host and must never
// make a recovery abandon a plan the next boot can still finish.
//
// Every other state is settled: maintenance, offline and unknown describe a
// host that will not become manageable by being asked again.
//
// systemdStateIsATransition, yoneticinin kendiliginden terk ettigi durumlari
// adlandirir; digerleri yerlesmis durumlardir.
func systemdStateIsATransition(state string) bool {
	return state == "initializing" || state == "starting" || state == "stopping"
}

// earlyBootAbsence classifies a missing systemd runtime path. Only absence is
// the boot condition; a path that exists but fails its ownership or type
// contract is a host property and stays durable.
// earlyBootAbsence yalnizca yoklugu acilis durumu sayar.
func earlyBootAbsence(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return stillStarting(err)
	}
	return err
}

func verifyDNFSecurityPolicy(getenforce string) error {
	if err := validateFixedExecutable("getenforce", getenforce); err != nil {
		return fmt.Errorf("revalidate getenforce: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnfSecurityProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, getenforce).Output()
	if ctx.Err() != nil {
		return fmt.Errorf("getenforce probe timed out: %w", ctx.Err())
	}
	if err != nil {
		return fmt.Errorf("run getenforce: %w", err)
	}
	return validateGetenforceOutput(out)
}

func validateGetenforceOutput(out []byte) error {
	state := strings.TrimRight(string(out), "\n")
	if state != "Enforcing" {
		return fmt.Errorf("getenforce reported %q, want Enforcing", state)
	}
	return nil
}

func fixedExecutablePresent(name string) (bool, error) {
	contract, ok := fixedExecutableContracts[name]
	if !ok {
		return false, fmt.Errorf("unknown fixed executable role %q", name)
	}
	_, err := os.Lstat(contract.path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("inspect fixed executable %s: %w", contract.path, err)
}

func fixedExecutablePath(name string) (string, error) {
	contract, ok := fixedExecutableContracts[name]
	if !ok {
		return "", fmt.Errorf("unknown fixed executable role %q", name)
	}
	present, err := fixedExecutablePresent(name)
	if err != nil {
		return "", err
	}
	if !present {
		return "", fmt.Errorf("fixed executable is absent: %s", contract.path)
	}
	return contract.path, nil
}

func validateFixedExecutable(name, path string) error {
	contract, ok := fixedExecutableContracts[name]
	if !ok {
		return fmt.Errorf("unknown fixed executable role %q", name)
	}
	if path != contract.path {
		return fmt.Errorf("%s must use fixed executable path %s, got %s", name, contract.path, path)
	}
	if err := validateRootOwnedPathChain(filepath.Dir(contract.path)); err != nil {
		return fmt.Errorf("validate %s directory chain: %w", name, err)
	}

	entry, err := os.Lstat(contract.path)
	if err != nil {
		return err
	}
	canonical := contract.path
	if entry.Mode()&os.ModeSymlink != 0 {
		if contract.allowedSymlinkTarget == "" {
			return fmt.Errorf("%s fixed executable must not be symbolic: %s", name, contract.path)
		}
		if err := validateRootOwner(contract.path, entry); err != nil {
			return err
		}
		canonical, err = filepath.EvalSymlinks(contract.path)
		if err != nil {
			return fmt.Errorf("resolve %s fixed executable: %w", name, err)
		}
	} else {
		canonical, err = filepath.EvalSymlinks(contract.path)
		if err != nil {
			return fmt.Errorf("canonicalize %s fixed executable: %w", name, err)
		}
	}
	if err := validateFixedExecutableResolution(
		name, path, contract, entry.Mode()&os.ModeSymlink != 0, canonical,
	); err != nil {
		return err
	}
	if err := validateRootOwnedPathChain(filepath.Dir(canonical)); err != nil {
		return fmt.Errorf("validate %s target directory chain: %w", name, err)
	}
	target, err := os.Lstat(canonical)
	if err != nil {
		return err
	}
	if target.Mode()&os.ModeSymlink != 0 || !target.Mode().IsRegular() ||
		target.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s target is not a direct regular executable: %s", name, canonical)
	}
	if err := validateRootOwnedNotWritable(canonical, target); err != nil {
		return err
	}
	stat, ok := target.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return fmt.Errorf("%s target must have exactly one hard link: %s", name, canonical)
	}
	return nil
}

func validateFixedExecutableResolution(
	name, path string,
	contract fixedExecutableContract,
	symbolic bool,
	canonical string,
) error {
	if path != contract.path {
		return fmt.Errorf("%s must use fixed executable path %s, got %s", name, contract.path, path)
	}
	if !symbolic {
		if canonical != contract.path {
			return fmt.Errorf("%s fixed executable path is not canonical: %s", name, contract.path)
		}
		return nil
	}
	if contract.allowedSymlinkTarget == "" {
		return fmt.Errorf("%s fixed executable must not be symbolic: %s", name, contract.path)
	}
	if canonical != contract.allowedSymlinkTarget {
		return fmt.Errorf(
			"%s fixed executable symlink resolves to %s, want %s",
			name, canonical, contract.allowedSymlinkTarget,
		)
	}
	return nil
}

func validateRootOwnedPathChain(path string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path %q is not absolute", path)
	}
	var chain []string
	for current := path; ; current = filepath.Dir(current) {
		chain = append(chain, current)
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	for i := len(chain) - 1; i >= 0; i-- {
		info, err := os.Lstat(chain[i])
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("path %q is not a direct directory", chain[i])
		}
		if err := validateRootOwnedNotWritable(chain[i], info); err != nil {
			return err
		}
	}
	return nil
}

func validateRootOwner(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || stat.Gid != 0 {
		return fmt.Errorf("path %q is not owned by root:root", path)
	}
	return nil
}

// validateSystemdPrivateSocket mirrors validate_systemd_runtime in install.sh
// and rollback.sh. The socket must be root:root; its mode may be owner-only
// writable or exactly the plain 0777 that systemd before v252 creates.
//
// systemd authenticates every peer of /run/systemd/private itself:
// bus_on_connection() calls bus_check_peercred(), which drops any SO_PEERCRED
// uid other than 0 (src/core/dbus.c, src/shared/bus-util.c, unchanged from
// v249 to v257). The mode decides only who may attempt a connection, never who
// is served. systemd before v252 binds the socket under PID 1's umask 0, so
// Ubuntu 22.04 (v249) shows 0777; v252 and later bind it under umask 0077
// (0700). Replacing the socket needs write access to /run/systemd, which the
// caller already requires to be root-owned and not group/other writable. Any
// other writable shape (0770, 01777, ...) is not one systemd creates and stays
// refused.
//
// systemd, ozel soketin her istemcisini kendisi dogrular (yalniz uid 0).
// v252 oncesi 0777, sonrasi 0700 olusturur; ikisi kabul edilir, baskasi edilmez.
func validateSystemdPrivateSocket(path string, info os.FileInfo) error {
	if err := validateRootOwner(path, info); err != nil {
		return err
	}
	if !systemdPrivateSocketModeAllowed(info.Mode()) {
		return fmt.Errorf(
			"path %q is group/world-writable (mode %04o) and is not systemd's pre-v252 0777",
			path, info.Mode().Perm(),
		)
	}
	return nil
}

func systemdPrivateSocketModeAllowed(mode os.FileMode) bool {
	if mode.Perm()&0o022 == 0 {
		return true
	}
	special := mode & (os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	return mode.Perm() == 0o777 && special == 0
}

func validateRootOwnedNotWritable(path string, info os.FileInfo) error {
	if err := validateRootOwner(path, info); err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("path %q is group/world-writable", path)
	}
	return nil
}
