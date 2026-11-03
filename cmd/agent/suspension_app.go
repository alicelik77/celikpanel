package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// A suspended site's Node.js app unit is stopped and disabled through
// systemd itself, so it stays stopped after a reboot and without the Panel
// (D-022). A resume enables and starts it again only as it was recorded
// before: an app that was stopped stays stopped.
//
// Askıdaki sitenin Node.js uygulama birimi systemd ile durdurulur ve
// açılışta başlamaz; geri açılışta yalnız önceden çalışan başlatılır.

func appUnitStateFromShow(out string) (transport.AppUnitState, error) {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	load, active := props["LoadState"], props["ActiveState"]
	if load == "" || active == "" {
		return transport.AppUnitState{}, errors.New("systemctl returned an incomplete unit state")
	}
	state := transport.AppUnitState{Present: load == "loaded"}
	if !state.Present {
		return state, nil
	}
	switch active {
	case "active", "activating", "reloading":
		state.Active = true
	}
	switch props["UnitFileState"] {
	case "enabled", "enabled-runtime":
		state.Enabled = true
	}
	return state, nil
}

func inspectSuspensionAppUnit(siteID int) (transport.AppUnitState, error) {
	if siteID <= 0 {
		return transport.AppUnitState{}, errors.New("a positive site identity is required")
	}
	return inspectUnitSuspensionState(appUnitName(siteID))
}

// inspectUnitSuspensionState reads one unit's presence, running and boot
// state (Node.js app units and, since alpha.118, service-app units).
func inspectUnitSuspensionState(name string) (transport.AppUnitState, error) {
	out, err := appUnitSystemctl(systemctlArgs("show", name, "--property=LoadState,ActiveState,UnitFileState")...)
	if err != nil {
		return transport.AppUnitState{}, fmt.Errorf("systemctl show %s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return appUnitStateFromShow(string(out))
}

// SetAppUnitSuspensionV1 stops and disables one app unit, or restores its
// recorded running and boot state. Suspending always converges (stopping is
// never wrong for a suspended site); a restore is compare-and-set, so an app
// the owner started or enabled meanwhile is left as they made it.
func (a *Agent) SetAppUnitSuspensionV1(
	req *transport.SetAppUnitSuspensionRequest,
	resp *transport.SetAppUnitSuspensionResponse,
) error {
	if req == nil || resp == nil {
		return fmt.Errorf("app unit suspension request and response are required")
	}
	*resp = transport.SetAppUnitSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a Node.js app"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.SiteID <= 0 {
		resp.Error = "a positive site identity is required; nothing was changed"
		return nil
	}
	unlock := lockAppUnitMutation(req.SiteID)
	defer unlock()
	applyUnitSuspension(appUnitName(req.SiteID), req.Suspend, req.Before, resp)
	return nil
}

// applyUnitSuspension stops and disables one unit, or restores its recorded
// running and boot state compare-and-set. The caller holds the unit's
// mutation lock. Shared by Node.js app units and service-app units.
func applyUnitSuspension(name string, suspend bool, before transport.AppUnitState, resp *transport.SetAppUnitSuspensionResponse) {
	current, err := inspectUnitSuspensionState(name)
	if err != nil {
		resp.Error = "the app unit could not be read, so nothing was changed: " + err.Error()
		return
	}
	resp.State = current
	if !current.Present {
		resp.Error = "app unit is absent: " + name + " is not installed"
		return
	}

	var steps [][]string
	if suspend {
		if current.Active {
			steps = append(steps, []string{"stop", name})
		}
		if current.Enabled {
			steps = append(steps, []string{"disable", name})
		}
	} else {
		target := before
		target.Present = true
		switch suspensionDecide(current, target, transport.AppUnitState{Present: true}) {
		case suspensionConflict:
			resp.Conflict = true
			resp.Error = fmt.Sprintf("%s was changed while suspended (running %v, starts at boot %v); it is left as it is", name, current.Active, current.Enabled)
			return
		case suspensionChange:
			if target.Enabled {
				steps = append(steps, []string{"enable", name})
			}
			if target.Active {
				steps = append(steps, []string{"start", name})
			}
		}
	}
	for _, step := range steps {
		if out, err := appUnitSystemctl(systemctlArgs(step...)...); err != nil {
			resp.Error = fmt.Sprintf("systemctl %s %s failed: %v: %s", step[0], name, err, strings.TrimSpace(string(out)))
			if after, readErr := inspectUnitSuspensionState(name); readErr == nil {
				resp.State = after
			}
			return
		}
		resp.Changed = true
	}

	after, err := inspectUnitSuspensionState(name)
	if err != nil {
		resp.Error = "the change finished but the app unit could not be read back, so the result is unknown: " + err.Error()
		return
	}
	resp.State = after
	wanted := after.Present && !after.Active && !after.Enabled
	if !suspend {
		wanted = after.Active == before.Active && after.Enabled == before.Enabled
	}
	if !wanted {
		resp.Error = fmt.Sprintf("%s does not read as the wanted state after the change (running %v, starts at boot %v)", name, after.Active, after.Enabled)
		return
	}
	resp.Applied = true
}
