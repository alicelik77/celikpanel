package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// fakeStartGuardHost is systemd plus a package manager: a unit is not-found
// until its package is unpacked; a masked unit cannot be started; the
// package's own start (deb-systemd-invoke) starts every unmasked unit.
type fakeStartGuardHost struct {
	installed map[string]bool // unit file present on disk
	masked    map[string]bool
	active    map[string]bool
	calls     []string
	marker    []byte
	events    []string // marker and mask order
	failMask  string
	failUnmsk string
}

func newFakeStartGuardHost() *fakeStartGuardHost {
	return &fakeStartGuardHost{installed: map[string]bool{}, masked: map[string]bool{}, active: map[string]bool{}}
}

func (h *fakeStartGuardHost) run(_ context.Context, name string, args ...string) ([]byte, error) {
	h.calls = append(h.calls, strings.Join(args, " "))
	unit := ""
	if len(args) > 1 {
		unit = args[1]
	}
	switch args[0] {
	case "show":
		load, fileState := "not-found", ""
		switch {
		case h.masked[unit]:
			load, fileState = "masked", "masked"
		case h.installed[unit]:
			load, fileState = "loaded", "enabled"
			if strings.Contains(unit, "@") {
				fileState = "indirect"
			}
		}
		active := "inactive"
		if h.active[unit] {
			active = "active"
		}
		return []byte(fmt.Sprintf("LoadState=%s\nActiveState=%s\nUnitFileState=%s\n", load, active, fileState)), nil
	case "mask":
		if unit == h.failMask {
			return []byte("refused"), errors.New("exit 1")
		}
		h.masked[unit] = true
		h.events = append(h.events, "mask "+unit)
	case "unmask":
		if unit == h.failUnmsk {
			return []byte("refused"), errors.New("exit 1")
		}
		delete(h.masked, unit)
		h.events = append(h.events, "unmask "+unit)
	case "stop", "reset-failed":
		delete(h.active, unit)
	default:
		return nil, fmt.Errorf("unexpected systemctl %v", args)
	}
	return nil, nil
}

// packageInstall unpacks units and starts every one that is not masked,
// as Debian's maintainer scripts do.
func (h *fakeStartGuardHost) packageInstall(units ...string) {
	for _, unit := range units {
		h.installed[unit] = true
		if !h.masked[unit] {
			h.active[unit] = true
		}
	}
}

func (h *fakeStartGuardHost) ops() packageStartGuardOps {
	return packageStartGuardOps{
		systemctl:        "/usr/bin/systemctl",
		runSystemd:       h.run,
		verifyMaskParent: func() error { return nil },
		readMarker: func(string) ([]byte, bool, error) {
			return h.marker, h.marker != nil, nil
		},
		writeMarker: func(_ string, raw []byte) error {
			h.marker = append([]byte(nil), raw...)
			h.events = append(h.events, "marker written")
			return nil
		},
		removeMarker: func(string) error {
			h.marker = nil
			h.events = append(h.events, "marker removed")
			return nil
		},
	}
}

func TestPackageStartGuardApplies(t *testing.T) {
	for _, tc := range []struct {
		service, family string
		want            bool
	}{
		{"postfix", "apt", true},
		{"dovecot", "apt", true},
		{"vsftpd", "apt", true},
		{"postfix", "pacman", false}, // pacman starts nothing
		{"vsftpd", "pacman", false},
		{"nginx", "apt", false},
		{"bind", "apt", false}, // BIND keeps its own guard
	} {
		if got := packageStartGuardApplies(tc.service, tc.family); got != tc.want {
			t.Errorf("%s/%s: %v, want %v", tc.service, tc.family, got, tc.want)
		}
	}
}

func TestPackageStartGuardSelectsOnlyUnitsThePackageBrings(t *testing.T) {
	state := func(name, load, active string) bindInstallUnitState {
		return bindInstallUnitState{name: name, loadState: load, activeState: active}
	}
	for name, tc := range map[string]struct {
		states []bindInstallUnitState
		want   []string
	}{
		"fresh host": {
			[]bindInstallUnitState{state("postfix.service", "not-found", "inactive"), state("postfix@-.service", "not-found", "inactive")},
			[]string{"postfix.service", "postfix@-.service"},
		},
		"already installed and running": {
			[]bindInstallUnitState{state("vsftpd.service", "loaded", "active")},
			nil,
		},
		"masked by the owner": {
			[]bindInstallUnitState{state("dovecot.service", "masked", "inactive"), state("dovecot.socket", "not-found", "inactive")},
			[]string{"dovecot.socket"},
		},
		"installed but stopped": {
			[]bindInstallUnitState{state("vsftpd.service", "loaded", "inactive")},
			nil,
		},
	} {
		if got := packageStartGuardSelect(tc.states); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", name, got, tc.want)
		}
	}
}

// The package's own start is blocked; after release every unit is unmasked
// and still stopped, and the marker was written before the first mask and
// removed only after the units were proven.
func TestPackageStartGuardHoldsThePackageStartAndReleases(t *testing.T) {
	for _, service := range []string{"postfix", "dovecot", "vsftpd"} {
		t.Run(service, func(t *testing.T) {
			host := newFakeStartGuardHost()
			ctx := context.Background()
			guard, err := beginPackageStartGuard(ctx, host.ops(), service)
			if err != nil || !guard.held() {
				t.Fatalf("begin: held=%v err=%v", guard.held(), err)
			}
			units := packageStartGuardUnits[service]
			host.packageInstall(units...)
			for _, unit := range units {
				if host.active[unit] {
					t.Fatalf("%s started during the package install", unit)
				}
			}
			if err := guard.release(ctx); err != nil {
				t.Fatalf("release: %v", err)
			}
			for _, unit := range units {
				if host.masked[unit] || host.active[unit] {
					t.Fatalf("%s after release: masked=%v active=%v", unit, host.masked[unit], host.active[unit])
				}
			}
			if host.events[0] != "marker written" || host.events[len(host.events)-1] != "marker removed" || host.marker != nil {
				t.Fatalf("order: %v", host.events)
			}
			if guard.held() || guard.heldNotice() != "" {
				t.Fatal("guard still held after release")
			}
		})
	}
}

// A unit already on the host is never masked; with nothing to hold, no
// marker is written.
func TestPackageStartGuardLeavesExistingUnitsAlone(t *testing.T) {
	host := newFakeStartGuardHost()
	host.installed["vsftpd.service"] = true
	host.active["vsftpd.service"] = true
	guard, err := beginPackageStartGuard(context.Background(), host.ops(), "vsftpd")
	if err != nil || guard.held() || host.marker != nil || len(host.events) != 0 {
		t.Fatalf("held=%v marker=%s events=%v err=%v", guard.held(), host.marker, host.events, err)
	}
	if err := guard.release(context.Background()); err != nil || !host.active["vsftpd.service"] {
		t.Fatalf("release touched a running unit: %v", err)
	}
}

// A failed mask releases what was masked and keeps no marker.
func TestPackageStartGuardFailedMaskReleases(t *testing.T) {
	host := newFakeStartGuardHost()
	host.failMask = "postfix@-.service"
	guard, err := beginPackageStartGuard(context.Background(), host.ops(), "postfix")
	if err == nil {
		t.Fatal("a failed mask was accepted")
	}
	if host.masked["postfix.service"] || host.marker != nil || guard.held() {
		t.Fatalf("left behind: masked=%v marker=%s", host.masked, host.marker)
	}
}

// A release that cannot unmask keeps the marker, so the next attempt or the
// removal continues it; the notice names the units and the next action.
func TestPackageStartGuardFailedReleaseKeepsMarker(t *testing.T) {
	host := newFakeStartGuardHost()
	ctx := context.Background()
	guard, err := beginPackageStartGuard(ctx, host.ops(), "vsftpd")
	if err != nil {
		t.Fatal(err)
	}
	host.packageInstall("vsftpd.service")
	host.failUnmsk = "vsftpd.service"
	if err := guard.release(ctx); err == nil {
		t.Fatal("a failed unmask was accepted")
	}
	if host.marker == nil || !guard.held() || !strings.Contains(guard.heldNotice(), "vsftpd.service stay masked") {
		t.Fatalf("marker=%s notice=%q", host.marker, guard.heldNotice())
	}
	// The next attempt adopts the marker and finishes.
	host.failUnmsk = ""
	resumed, err := resumePackageStartGuard(host.ops(), "vsftpd")
	if err != nil || !resumed.wasResumed() || !resumed.held() {
		t.Fatalf("resume: %+v %v", resumed, err)
	}
	if err := resumed.release(ctx); err != nil || host.masked["vsftpd.service"] || host.active["vsftpd.service"] || host.marker != nil {
		t.Fatalf("resumed release: err=%v masked=%v active=%v marker=%s", err, host.masked, host.active, host.marker)
	}
}

// Removing the service releases masks an unfinished install left.
func TestPackageStartGuardRemovalReleases(t *testing.T) {
	host := newFakeStartGuardHost()
	ctx := context.Background()
	if _, err := beginPackageStartGuard(ctx, host.ops(), "dovecot"); err != nil {
		t.Fatal(err)
	}
	host.packageInstall("dovecot.service", "dovecot.socket")
	if err := releaseHeldPackageStartGuard(ctx, host.ops(), "dovecot"); err != nil {
		t.Fatal(err)
	}
	if len(host.masked) != 0 || host.marker != nil {
		t.Fatalf("masked=%v marker=%s", host.masked, host.marker)
	}
	// Nothing held: nothing to do.
	if err := releaseHeldPackageStartGuard(ctx, host.ops(), "dovecot"); err != nil {
		t.Fatal(err)
	}
}

func TestPackageStartGuardMarkerIsValidated(t *testing.T) {
	for name, raw := range map[string]string{
		"other service":  `{"version":1,"service":"postfix","units":["postfix.service"]}`,
		"foreign unit":   `{"version":1,"service":"vsftpd","units":["sshd.service"]}`,
		"duplicate unit": `{"version":1,"service":"vsftpd","units":["vsftpd.service","vsftpd.service"]}`,
		"no units":       `{"version":1,"service":"vsftpd","units":[]}`,
		"other version":  `{"version":2,"service":"vsftpd","units":["vsftpd.service"]}`,
		"unknown field":  `{"version":1,"service":"vsftpd","units":["vsftpd.service"],"x":1}`,
		"not json":       `masked`,
	} {
		if _, err := decodePackageStartGuardMarker([]byte(raw), "vsftpd"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	units, err := decodePackageStartGuardMarker([]byte(`{"version":1,"service":"vsftpd","units":["vsftpd.service"]}`), "vsftpd")
	if err != nil || !slices.Equal(units, []string{"vsftpd.service"}) {
		t.Fatalf("valid marker: %v %v", units, err)
	}
}

// A failed package transaction releases under the bounded recovery context,
// even when the lease context is already cancelled.
func TestPackageStartGuardFailureReleaseUsesRecoveryContext(t *testing.T) {
	host := newFakeStartGuardHost()
	ops := host.ops()
	recoveries := 0
	ops.recoveryContext = func(context.Context) (context.Context, context.CancelFunc, error) {
		recoveries++
		recovery, cancel := context.WithCancel(context.Background())
		return recovery, cancel, nil
	}
	var seen []error
	ops.runSystemd = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		seen = append(seen, ctx.Err())
		return host.run(ctx, name, args...)
	}
	lease, cancel := context.WithCancel(context.Background())
	guard, err := beginPackageStartGuard(lease, ops, "vsftpd")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	seen = nil
	if err := guard.releaseAfterFailure(lease); err != nil || recoveries != 1 || host.marker != nil || len(host.masked) != 0 {
		t.Fatalf("err=%v recoveries=%d marker=%s masked=%v", err, recoveries, host.marker, host.masked)
	}
	for _, ctxErr := range seen {
		if ctxErr != nil {
			t.Fatal("a recovery command ran on the cancelled lease context")
		}
	}
}

// A nil guard (no guard applies) is safe to use on every path.
func TestPackageStartGuardNilIsInert(t *testing.T) {
	var guard *packageStartGuard
	if guard.held() || guard.wasResumed() || guard.heldNotice() != "" || guard.release(context.Background()) != nil {
		t.Fatal("nil guard is not inert")
	}
}
