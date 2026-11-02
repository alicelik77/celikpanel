package main

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Security emergency, Agent side (alpha.117, D-043). The Panel names a
// component (php-fpm with a version, mariadb or postgresql); the Agent finds
// that component's systemd units itself and locks them through systemd: stop,
// then mask, so they stay stopped after a reboot and without the Panel
// (D-022). An unlock is compare-and-set against the before-image the Panel
// stored first: unmask, and start only a unit that ran before.
//
// Güvenlik acil durumu, Agent tarafı: bileşenin birimleri systemd ile
// durdurulur ve maskelenir; geri açma ön görüntüye göre karşılaştır-ve-yaz.

var emergencyPHPVersionRe = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)

// emergencySystemctl runs systemctl (a test seam).
var emergencySystemctl = func(args ...string) ([]byte, error) {
	return exec.Command("systemctl", args...).CombinedOutput()
}

// emergencyUnitCandidates names the component's units (a test seam).
var emergencyUnitCandidates = func(service, version string) ([]string, error) {
	switch service {
	case "php-fpm":
		units := []string{}
		for _, instance := range phpInstances() {
			if instance.Version == version && instance.Unit != "" {
				units = append(units, instance.Unit)
			}
		}
		if len(units) == 0 {
			// A version whose package was removed keeps its mask.
			units = append(units, "php"+version+"-fpm")
		}
		return units, nil
	case "mariadb":
		return []string{"mariadb", "mysql"}, nil
	case "postgresql":
		return append([]string{"postgresql"}, unitsMatching(`^postgresql@.+$`)...), nil
	}
	return nil, fmt.Errorf("a security emergency stop is not supported for %q", service)
}

// emergencyLockMu serializes emergency lock calls.
var emergencyLockMu sync.Mutex

func validEmergencyTarget(service, version string) error {
	switch service {
	case "php-fpm":
		if !emergencyPHPVersionRe.MatchString(version) {
			return errors.New("a PHP version such as 8.4 is required")
		}
	case "mariadb", "postgresql":
		if version != "" {
			return errors.New("a database engine is stopped as a whole; no version is named")
		}
	default:
		return fmt.Errorf("a security emergency stop is not supported for %q", service)
	}
	return nil
}

type emergencyUnitShow struct {
	id    string
	state transport.EmergencyUnitState
}

func readEmergencyUnit(unit string) (emergencyUnitShow, error) {
	out, err := emergencySystemctl("show", unit+".service", "--property=Id,LoadState,ActiveState,UnitFileState")
	if err != nil {
		return emergencyUnitShow{}, fmt.Errorf("systemctl show %s: %v: %s", unit, err, strings.TrimSpace(string(out)))
	}
	props := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	load, active := props["LoadState"], props["ActiveState"]
	if load == "" || active == "" {
		return emergencyUnitShow{}, fmt.Errorf("systemctl returned an incomplete state for %s", unit)
	}
	show := emergencyUnitShow{id: strings.TrimSuffix(props["Id"], ".service")}
	if show.id == "" {
		show.id = unit
	}
	fileState := props["UnitFileState"]
	show.state.Masked = load == "masked" || fileState == "masked" || fileState == "masked-runtime"
	show.state.Present = load == "loaded" || show.state.Masked
	switch active {
	case "active", "activating", "reloading":
		show.state.Active = true
	}
	return show, nil
}

// inspectEmergencyUnits reads the component's present units, one per real
// unit (an alias such as mysql -> mariadb is read once), sorted by name.
func inspectEmergencyUnits(service, version string) ([]transport.EmergencyUnitObservation, error) {
	if err := validEmergencyTarget(service, version); err != nil {
		return nil, err
	}
	candidates, err := emergencyUnitCandidates(service, version)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	units := []transport.EmergencyUnitObservation{}
	for _, candidate := range candidates {
		show, err := readEmergencyUnit(candidate)
		if err != nil {
			return nil, err
		}
		if !show.state.Present || seen[show.id] {
			continue
		}
		seen[show.id] = true
		units = append(units, transport.EmergencyUnitObservation{Unit: show.id, State: show.state})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })
	return units, nil
}

// EmergencyUnitsInspectV1 reports the component's units. Read-only.
func (a *Agent) EmergencyUnitsInspectV1(req *transport.EmergencyUnitsInspectRequest, resp *transport.EmergencyUnitsInspectResponse) error {
	if resp == nil {
		return nil
	}
	*resp = transport.EmergencyUnitsInspectResponse{Units: []transport.EmergencyUnitObservation{}}
	if req == nil {
		resp.Error = "an inspection request is required"
		return nil
	}
	units, err := inspectEmergencyUnits(req.Service, req.Version)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Units = units
	return nil
}

var emergencyLockedState = transport.EmergencyUnitState{Present: true, Active: false, Masked: true}

// SetEmergencyUnitLockV1 locks or unlocks the component's units. Locking
// always converges (stopping and masking is never wrong in an emergency); an
// unlock changes a unit only while it is still exactly as the lock left it.
func (a *Agent) SetEmergencyUnitLockV1(req *transport.SetEmergencyUnitLockRequest, resp *transport.SetEmergencyUnitLockResponse) error {
	if req == nil || resp == nil {
		return fmt.Errorf("emergency unit lock request and response are required")
	}
	*resp = transport.SetEmergencyUnitLockResponse{Units: []transport.EmergencyUnitObservation{}}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "locking or unlocking a component in a security emergency"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := validEmergencyTarget(req.Service, req.Version); err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	emergencyLockMu.Lock()
	defer emergencyLockMu.Unlock()

	current, err := inspectEmergencyUnits(req.Service, req.Version)
	if err != nil {
		resp.Error = "the units could not be read, so nothing was changed: " + err.Error()
		return nil
	}
	resp.Units = current
	var steps [][]string
	var conflicts []string
	if req.Lock {
		for _, unit := range current {
			if unit.State.Active {
				steps = append(steps, []string{"stop", unit.Unit + ".service"})
			}
			if !unit.State.Masked {
				steps = append(steps, []string{"mask", unit.Unit + ".service"})
			}
		}
	} else {
		for _, unit := range current {
			before, known := req.Before[unit.Unit]
			if !known {
				// Not part of the lock (it appeared later): left alone.
				continue
			}
			target := before
			target.Present = true
			switch suspensionDecide(unit.State, target, emergencyLockedState) {
			case suspensionConflict:
				conflicts = append(conflicts, fmt.Sprintf("%s (running %v, masked %v)", unit.Unit, unit.State.Active, unit.State.Masked))
			case suspensionChange:
				if unit.State.Masked && !target.Masked {
					steps = append(steps, []string{"unmask", unit.Unit + ".service"})
				}
				if target.Active {
					steps = append(steps, []string{"start", unit.Unit + ".service"})
				}
			}
		}
	}
	for _, step := range steps {
		if out, err := emergencySystemctl(step...); err != nil {
			resp.Error = fmt.Sprintf("systemctl %s %s failed: %v: %s", step[0], step[1], err, strings.TrimSpace(string(out)))
			if after, readErr := inspectEmergencyUnits(req.Service, req.Version); readErr == nil {
				resp.Units = after
			}
			return nil
		}
		resp.Changed = true
	}
	after, err := inspectEmergencyUnits(req.Service, req.Version)
	if err != nil {
		resp.Error = "the change finished but the units could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	resp.Units = after
	for _, unit := range after {
		wanted := !unit.State.Active && unit.State.Masked
		if !req.Lock {
			before, known := req.Before[unit.Unit]
			if !known || containsPrefix(conflicts, unit.Unit+" ") {
				continue
			}
			wanted = unit.State.Active == before.Active && unit.State.Masked == before.Masked
		}
		if !wanted {
			resp.Error = fmt.Sprintf("%s does not read as wanted after the change (running %v, masked %v)", unit.Unit, unit.State.Active, unit.State.Masked)
			return nil
		}
	}
	if len(conflicts) > 0 {
		resp.Conflict = true
		resp.Error = "changed on the server while locked, so left as it is: " + strings.Join(conflicts, ", ")
		return nil
	}
	resp.Applied = true
	return nil
}

func containsPrefix(list []string, prefix string) bool {
	for _, item := range list {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}
