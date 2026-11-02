package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// fakeEmergencySystemd is a tiny systemd: units with running and mask state,
// and an alias (mysql -> mariadb).
type fakeEmergencySystemd struct {
	units map[string]*transport.EmergencyUnitState
	alias map[string]string
	calls []string
	fail  string
}

func (f *fakeEmergencySystemd) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	name := strings.TrimSuffix(args[1], ".service")
	if real, ok := f.alias[name]; ok {
		name = real
	}
	unit := f.units[name]
	if args[0] == "show" {
		if unit == nil {
			return []byte("Id=" + name + ".service\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n"), nil
		}
		load, active, file := "loaded", "inactive", "enabled"
		if unit.Masked {
			load, file = "masked", "masked"
		}
		if unit.Active {
			active = "active"
		}
		return []byte("Id=" + name + ".service\nLoadState=" + load + "\nActiveState=" + active + "\nUnitFileState=" + file + "\n"), nil
	}
	if f.fail == args[0] {
		return []byte("Failed to " + args[0]), errors.New("exit status 1")
	}
	switch args[0] {
	case "stop":
		unit.Active = false
	case "mask":
		unit.Masked = true
	case "unmask":
		unit.Masked = false
	case "start":
		if unit.Masked {
			return []byte("Unit is masked."), errors.New("exit status 1")
		}
		unit.Active = true
	}
	return nil, nil
}

func withFakeEmergencySystemd(t *testing.T, f *fakeEmergencySystemd, candidates []string) {
	t.Helper()
	oldRun, oldCandidates := emergencySystemctl, emergencyUnitCandidates
	emergencySystemctl = f.run
	emergencyUnitCandidates = func(string, string) ([]string, error) { return candidates, nil }
	t.Cleanup(func() { emergencySystemctl, emergencyUnitCandidates = oldRun, oldCandidates })
}

func emergencyLock(t *testing.T, service, version string, lock bool, before map[string]transport.EmergencyUnitState) transport.SetEmergencyUnitLockResponse {
	t.Helper()
	var resp transport.SetEmergencyUnitLockResponse
	if err := (&Agent{}).SetEmergencyUnitLockV1(&transport.SetEmergencyUnitLockRequest{Service: service, Version: version, Lock: lock, Before: before}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestEmergencyLockStopsMasksAndUnlockRestoresExactly(t *testing.T) {
	f := &fakeEmergencySystemd{units: map[string]*transport.EmergencyUnitState{
		"php8.5-fpm": {Present: true, Active: true},
	}}
	withFakeEmergencySystemd(t, f, []string{"php8.5-fpm"})

	var inspect transport.EmergencyUnitsInspectResponse
	if err := (&Agent{}).EmergencyUnitsInspectV1(&transport.EmergencyUnitsInspectRequest{Service: "php-fpm", Version: "8.5"}, &inspect); err != nil || inspect.Error != "" {
		t.Fatalf("inspect: %v %+v", err, inspect)
	}
	before := map[string]transport.EmergencyUnitState{}
	for _, unit := range inspect.Units {
		before[unit.Unit] = unit.State
	}
	resp := emergencyLock(t, "php-fpm", "8.5", true, before)
	if !resp.Applied || !resp.Changed || f.units["php8.5-fpm"].Active || !f.units["php8.5-fpm"].Masked {
		t.Fatalf("lock: %+v %+v", resp, f.units["php8.5-fpm"])
	}
	// Again: nothing to do, still applied.
	resp = emergencyLock(t, "php-fpm", "8.5", true, before)
	if !resp.Applied || resp.Changed {
		t.Fatalf("second lock: %+v", resp)
	}
	resp = emergencyLock(t, "php-fpm", "8.5", false, before)
	if !resp.Applied || !resp.Changed || !f.units["php8.5-fpm"].Active || f.units["php8.5-fpm"].Masked {
		t.Fatalf("unlock: %+v %+v", resp, f.units["php8.5-fpm"])
	}
}

func TestEmergencyUnlockLeavesAUnitTheOwnerChanged(t *testing.T) {
	f := &fakeEmergencySystemd{units: map[string]*transport.EmergencyUnitState{
		"mariadb": {Present: true, Active: true},
	}, alias: map[string]string{"mysql": "mariadb"}}
	withFakeEmergencySystemd(t, f, []string{"mariadb", "mysql"})
	before := map[string]transport.EmergencyUnitState{"mariadb": {Present: true, Active: true}}
	resp := emergencyLock(t, "mariadb", "", true, before)
	if !resp.Applied || len(resp.Units) != 1 {
		t.Fatalf("lock reads the alias once: %+v", resp)
	}
	// The owner unmasked it by hand (but did not start it).
	f.units["mariadb"].Masked = false
	resp = emergencyLock(t, "mariadb", "", false, before)
	if !resp.Conflict || resp.Applied || f.units["mariadb"].Active {
		t.Fatalf("unlock over the owner's change: %+v %+v", resp, f.units["mariadb"])
	}
}

func TestEmergencyLockRefusesAnythingButItsComponents(t *testing.T) {
	f := &fakeEmergencySystemd{units: map[string]*transport.EmergencyUnitState{}}
	withFakeEmergencySystemd(t, f, []string{"nginx"})
	for _, c := range []struct{ service, version string }{
		{"nginx", ""}, {"php-fpm", ""}, {"php-fpm", "8.5; rm"}, {"mariadb", "10.11"}, {"celikpanel-agent", ""},
	} {
		resp := emergencyLock(t, c.service, c.version, true, nil)
		if resp.Applied || !strings.Contains(resp.Error, "nothing was changed") {
			t.Fatalf("%+v: %+v", c, resp)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("systemctl was called: %v", f.calls)
	}
}

func TestEmergencyLockReportsAFailedStep(t *testing.T) {
	f := &fakeEmergencySystemd{units: map[string]*transport.EmergencyUnitState{
		"php8.5-fpm": {Present: true, Active: true},
	}, fail: "mask"}
	withFakeEmergencySystemd(t, f, []string{"php8.5-fpm"})
	resp := emergencyLock(t, "php-fpm", "8.5", true, nil)
	if resp.Applied || !strings.Contains(resp.Error, "systemctl mask") {
		t.Fatalf("failed mask: %+v", resp)
	}
}
