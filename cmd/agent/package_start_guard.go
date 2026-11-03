package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Debian and Ubuntu packages start their daemon as soon as the package is
// configured, with the distribution's own configuration: Postfix and Dovecot
// on self-signed certificates, vsftpd as plain FTP. The package start guard
// closes that window for the network daemons CelikPanel configures before
// their first start. It is the BIND guard's mask pattern, narrowed:
//
//   - only units the package brings now (not-found before) are masked; a
//     unit already on the host is never touched;
//   - the masked units are listed in a marker file before the first mask,
//     so a crash or a failed preparation is continued by the next install
//     of the same service, and removing the service releases them;
//   - a failed package transaction releases the masks again (the host is
//     as before); a failed preparation keeps them, so the distribution's
//     configuration cannot serve until the install is repeated;
//   - release unmasks only the listed units and proves them stopped. The
//     install path then starts each daemon with CelikPanel's configuration.
//
// A policy-rc.d file is not used: it is one global file for every package,
// an owner or image may already have one, and a copy left by a crash would
// keep security updates from restarting any service. pacman starts nothing,
// so Arch needs no guard.
//
// Debian/Ubuntu paketleri daemon'u dağıtımın kendi yapılandırmasıyla hemen
// başlatır. Bu koruma yalnız paketin şimdi getirdiği unit'leri maskeler,
// önce bir işaret dosyasına yazar; hazırlık bitince maskeleri kaldırır ve
// unit'lerin durduğunu kanıtlar. Var olan unit'e dokunulmaz.

const packageStartGuardMarkerVersion = 1

var packageStartGuardUnits = map[string][]string{
	"postfix": {"postfix.service", "postfix@-.service"},
	"dovecot": {"dovecot.service", "dovecot.socket"},
	"vsftpd":  {"vsftpd.service"},
}

func packageStartGuardApplies(serviceID, family string) bool {
	return family == "apt" && len(packageStartGuardUnits[serviceID]) > 0
}

type packageStartGuardMarker struct {
	Version int      `json:"version"`
	Service string   `json:"service"`
	Units   []string `json:"units"`
}

type packageStartGuardOps struct {
	systemctl        string
	runSystemd       bindInstallSystemdRunner
	verifyMaskParent func() error
	readMarker       func(service string) ([]byte, bool, error)
	writeMarker      func(service string, raw []byte) error
	removeMarker     func(service string) error
	// recoveryContext bounds a release after a failure, also when the
	// mutation lease is already cancelling. Nil: the caller's context.
	recoveryContext bindInstallRecoveryContextFactory
}

type packageStartGuard struct {
	ops     packageStartGuardOps
	service string
	// units were masked by CelikPanel and are listed in the marker.
	units []string
	// resumed: adopted from the marker of an earlier, unfinished attempt.
	resumed bool
}

func defaultPackageStartGuardOps(systemctl string) packageStartGuardOps {
	return packageStartGuardOps{
		systemctl:        systemctl,
		runSystemd:       runServiceMutationCombinedOutput,
		verifyMaskParent: verifyBINDMaskParentMetadata,
		readMarker: func(service string) ([]byte, bool, error) {
			return readSecureServiceMutationLedger(packageStartGuardMarkerPath(service), 4096)
		},
		writeMarker: func(service string, raw []byte) error {
			path := packageStartGuardMarkerPath(service)
			if err := ensureSecureServiceMutationStateDirectory(filepath.Dir(path)); err != nil {
				return err
			}
			return secureWriteConfigOwnedBy(path, raw, 0o600,
				serviceMutationRequiredOwnerUID, serviceMutationRequiredOwnerGID)
		},
		removeMarker: func(service string) error {
			err := os.Remove(packageStartGuardMarkerPath(service))
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		},
		recoveryContext: func(parent context.Context) (context.Context, context.CancelFunc, error) {
			return serviceMutationCancellingRecoveryContext(parent, bindInstallRollbackTimeout)
		},
	}
}

func packageStartGuardMarkerPath(service string) string {
	return filepath.Join(serviceMutationStateDirectory(), "package-start-guard-"+service+".json")
}

func (o packageStartGuardOps) valid() bool {
	return strings.TrimSpace(o.systemctl) != "" && o.runSystemd != nil &&
		o.verifyMaskParent != nil && o.readMarker != nil &&
		o.writeMarker != nil && o.removeMarker != nil
}

// resumePackageStartGuard adopts the units an earlier attempt masked and
// did not release. No marker: nil guard, nothing held.
func resumePackageStartGuard(ops packageStartGuardOps, service string) (*packageStartGuard, error) {
	if !ops.valid() || len(packageStartGuardUnits[service]) == 0 {
		return nil, errors.New("invalid package start guard")
	}
	raw, found, err := ops.readMarker(service)
	if err != nil {
		return nil, fmt.Errorf("read the package start guard of %s: %w", service, err)
	}
	if !found {
		return nil, nil
	}
	units, err := decodePackageStartGuardMarker(raw, service)
	if err != nil {
		return nil, err
	}
	return &packageStartGuard{ops: ops, service: service, units: units, resumed: true}, nil
}

func decodePackageStartGuardMarker(raw []byte, service string) ([]string, error) {
	var marker packageStartGuardMarker
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&marker); err != nil {
		return nil, fmt.Errorf("the package start guard of %s is unreadable: %w", service, err)
	}
	allowed := packageStartGuardUnits[service]
	if marker.Version != packageStartGuardMarkerVersion || marker.Service != service || len(marker.Units) == 0 {
		return nil, fmt.Errorf("the package start guard of %s does not belong to this service", service)
	}
	for i, unit := range marker.Units {
		if !slices.Contains(allowed, unit) || slices.Contains(marker.Units[:i], unit) {
			return nil, fmt.Errorf("the package start guard of %s names an unexpected unit", service)
		}
	}
	return marker.Units, nil
}

// packageStartGuardSelect: the units the package brings now. A unit the
// host already knows (loaded, masked by someone, running) is left alone.
func packageStartGuardSelect(states []bindInstallUnitState) []string {
	var units []string
	for _, state := range states {
		if state.loadState == "not-found" && state.activeState == "inactive" {
			units = append(units, state.name)
		}
	}
	return units
}

// beginPackageStartGuard masks the units the package would start. The
// marker is written before the first mask; a failed mask releases again.
func beginPackageStartGuard(ctx context.Context, ops packageStartGuardOps, service string) (*packageStartGuard, error) {
	if ctx == nil || !ops.valid() || len(packageStartGuardUnits[service]) == 0 {
		return nil, errors.New("invalid package start guard")
	}
	guard := &packageStartGuard{ops: ops, service: service}
	states := make([]bindInstallUnitState, 0, len(packageStartGuardUnits[service]))
	for _, unit := range packageStartGuardUnits[service] {
		state, err := guard.inspect(ctx, unit)
		if err != nil {
			return nil, fmt.Errorf("inspect %s before the package install: %w", unit, err)
		}
		states = append(states, state)
	}
	units := packageStartGuardSelect(states)
	if len(units) == 0 {
		return guard, nil
	}
	raw, err := json.Marshal(packageStartGuardMarker{
		Version: packageStartGuardMarkerVersion, Service: service, Units: units,
	})
	if err != nil {
		return nil, err
	}
	if err := ops.writeMarker(service, raw); err != nil {
		return nil, fmt.Errorf("record the package start guard of %s: %w", service, err)
	}
	guard.units = units
	for _, unit := range units {
		if err := guard.mask(ctx, unit); err != nil {
			return guard, errors.Join(err, guard.releaseAfterFailure(ctx))
		}
	}
	return guard, nil
}

// releaseAfterFailure is release under the bounded recovery context, so a
// cancelled lease still restores the host; if it cannot, the marker stays.
func (g *packageStartGuard) releaseAfterFailure(ctx context.Context) error {
	if !g.held() {
		return nil
	}
	if g.ops.recoveryContext == nil {
		return g.release(ctx)
	}
	recoveryCtx, cancel, err := g.ops.recoveryContext(ctx)
	if err != nil {
		return fmt.Errorf("create a bounded recovery context: %w", err)
	}
	defer cancel()
	return g.release(recoveryCtx)
}

// held: units are masked by this guard right now.
func (g *packageStartGuard) held() bool {
	return g != nil && len(g.units) > 0
}

func (g *packageStartGuard) wasResumed() bool {
	return g != nil && g.resumed
}

// release unmasks the listed units, proves them stopped and removes the
// marker. The marker stays when anything could not be proven.
func (g *packageStartGuard) release(ctx context.Context) error {
	if !g.held() {
		return nil
	}
	var errs []error
	for _, unit := range g.units {
		if err := g.unmask(ctx, unit); err != nil {
			errs = append(errs, err)
		}
	}
	for _, unit := range g.units {
		if err := g.ensureStopped(ctx, unit); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := g.ops.removeMarker(g.service); err != nil {
		return fmt.Errorf("remove the package start guard of %s: %w", g.service, err)
	}
	g.units = nil
	return nil
}

// heldNotice is the operator line when a failure keeps the units masked.
func (g *packageStartGuard) heldNotice() string {
	if !g.held() {
		return ""
	}
	return fmt.Sprintf("%s stay masked so the distribution's default configuration cannot serve; "+
		"install it again from Components to finish, or remove it to release them", strings.Join(g.units, ", "))
}

func (g *packageStartGuard) mask(ctx context.Context, unit string) error {
	if err := g.ops.verifyMaskParent(); err != nil {
		return fmt.Errorf("verify the systemd unit directory before masking %s: %w", unit, err)
	}
	output, commandErr := g.ops.runSystemd(ctx, g.ops.systemctl, "mask", unit)
	after, inspectErr := g.inspect(ctx, unit)
	if inspectErr == nil && after.loadState == "masked" {
		return nil
	}
	return reconciledBINDSystemdError([]string{"mask", unit}, output, commandErr, inspectErr, after)
}

func (g *packageStartGuard) unmask(ctx context.Context, unit string) error {
	if err := g.ops.verifyMaskParent(); err != nil {
		return fmt.Errorf("verify the systemd unit directory before unmasking %s: %w", unit, err)
	}
	output, commandErr := g.ops.runSystemd(ctx, g.ops.systemctl, "unmask", unit)
	after, inspectErr := g.inspect(ctx, unit)
	if inspectErr == nil && after.loadState != "masked" {
		return nil
	}
	return reconciledBINDSystemdError([]string{"unmask", unit}, output, commandErr, inspectErr, after)
}

func (g *packageStartGuard) ensureStopped(ctx context.Context, unit string) error {
	before, err := g.inspect(ctx, unit)
	if err != nil {
		return fmt.Errorf("prove %s stopped: %w", unit, err)
	}
	if before.activeState == "inactive" {
		return nil
	}
	action := "stop"
	if before.activeState == "failed" {
		action = "reset-failed"
	}
	output, commandErr := g.ops.runSystemd(ctx, g.ops.systemctl, action, unit)
	after, inspectErr := g.inspect(ctx, unit)
	if inspectErr == nil && after.activeState == "inactive" {
		return nil
	}
	return reconciledBINDSystemdError([]string{action, unit}, output, commandErr, inspectErr, after)
}

// inspect reads the three unit properties. Unlike the BIND guard it accepts
// any unit-file state: a template instance such as postfix@-.service reads
// back as static, indirect or generated once its package is installed.
func (g *packageStartGuard) inspect(ctx context.Context, unit string) (bindInstallUnitState, error) {
	output, err := g.ops.runSystemd(ctx, g.ops.systemctl,
		"show", unit, "--property=LoadState,ActiveState,UnitFileState", "--no-pager")
	if err != nil {
		return bindInstallUnitState{}, fmt.Errorf("systemctl show failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	state := bindInstallUnitState{name: unit}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || seen[key] {
			return bindInstallUnitState{}, errors.New("systemctl show returned a malformed unit state")
		}
		seen[key] = true
		switch key {
		case "LoadState":
			state.loadState = value
		case "ActiveState":
			state.activeState = value
		case "UnitFileState":
			state.unitFileState = value
		default:
			return bindInstallUnitState{}, errors.New("systemctl show returned an unexpected unit property")
		}
	}
	if !seen["LoadState"] || !seen["ActiveState"] || !seen["UnitFileState"] {
		return bindInstallUnitState{}, errors.New("systemctl show returned incomplete unit state")
	}
	if !validBINDInstallLoadState(state.loadState) || state.activeState == "" {
		return bindInstallUnitState{}, fmt.Errorf("unsupported unit state load=%q active=%q",
			state.loadState, state.activeState)
	}
	return state, nil
}

func releasePackageStartGuardForRemoval(ctx context.Context, service string) error {
	if len(packageStartGuardUnits[service]) == 0 {
		return nil
	}
	profile, err := detectHostPlatform()
	if err != nil {
		return err
	}
	systemctl, err := executableForProfile(profile, string(profile.PackageManager), "systemctl")
	if err != nil {
		return err
	}
	return releaseHeldPackageStartGuard(ctx, defaultPackageStartGuardOps(systemctl), service)
}

// releaseHeldPackageStartGuard: removing a service releases the masks an
// unfinished install left, before the package removal stops its units.
func releaseHeldPackageStartGuard(ctx context.Context, ops packageStartGuardOps, service string) error {
	guard, err := resumePackageStartGuard(ops, service)
	if err != nil || guard == nil {
		return err
	}
	return guard.release(ctx)
}
