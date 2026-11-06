package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/systemd"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Service health (alpha.127, monitoring stage 2). One batched, read-only
// `systemctl show` for every managed unit the panel names: state, since when
// it is active, how often systemd restarted it, and its cgroup's memory and
// CPU time. Nothing is started, stopped or cached here; the panel calls it at
// most once a minute from its sampler.
//
// Hizmet sağlığı: panelin adlandırdığı her yönetilen birim için tek, salt
// okur, toplu `systemctl show`. Hiçbir şey başlatılmaz ya da durdurulmaz.

var serviceHealthProperties = []string{
	"LoadState", "ActiveState", "SubState", "ActiveEnterTimestamp", "NRestarts", "MemoryCurrent", "CPUUsageNSec",
}

// serviceHealthSystemctl finds the verified systemctl. Variable for tests.
var serviceHealthSystemctl = func() (string, error) {
	profile, err := readHostProfile()
	if err != nil {
		return "", err
	}
	return executableForProfile(profile, string(profile.PackageManager), "systemctl")
}

// ServiceHealthV1 answers the units in the order asked.
func (a *Agent) ServiceHealthV1(req *transport.ServiceHealthRequest, resp *transport.ServiceHealthResponse) error {
	*resp = transport.ServiceHealthResponse{Units: []transport.ServiceHealthUnit{}}
	if req == nil || len(req.Units) == 0 {
		resp.Batched = true
		return nil
	}
	if len(req.Units) > transport.ServiceHealthMaxUnits {
		return fmt.Errorf("at most %d units may be read at once", transport.ServiceHealthMaxUnits)
	}
	seen := map[string]bool{}
	for _, unit := range req.Units {
		if !validUnitName(unit) || strings.HasSuffix(unit, ".service") || seen[unit] {
			return errors.New("a unit name is not valid")
		}
		seen[unit] = true
	}
	systemctl, err := serviceHealthSystemctl()
	if err != nil {
		return fmt.Errorf("systemctl is not available: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	units, ok := readServiceHealth(ctx, systemctl, req.Units)
	if !ok {
		return nil
	}
	resp.Batched, resp.Units = true, units
	return nil
}

// readServiceHealth runs the one batched call. Timestamps are asked as Unix
// seconds (systemd 248 and later); an older systemd that refuses the option is
// asked once more without it and its local text form is read.
func readServiceHealth(ctx context.Context, systemctl string, units []string) ([]transport.ServiceHealthUnit, bool) {
	props := strings.Join(append([]string{"Id"}, serviceHealthProperties...), ",")
	args := []string{"show", "--timestamp=unix", "-p", props}
	for _, unit := range units {
		args = append(args, unit+".service")
	}
	out, err := inventoryCommand(ctx, systemctl, args...)
	if err != nil {
		args = append([]string{"show"}, args[2:]...)
		if out, err = inventoryCommand(ctx, systemctl, args...); err != nil {
			return nil, false
		}
	}
	blocks, ok := systemd.ParseShowBlocks(string(out), len(units))
	if !ok {
		return nil, false
	}
	result := make([]transport.ServiceHealthUnit, 0, len(units))
	for i, unit := range units {
		block := blocks[i]
		row := transport.ServiceHealthUnit{
			Unit:        unit,
			LoadState:   firstValue(block, "LoadState"),
			ActiveState: firstValue(block, "ActiveState"),
			SubState:    firstValue(block, "SubState"),
		}
		if row.ActiveState == "active" || row.ActiveState == "reloading" {
			if since, ok := parseSystemdTimestamp(firstValue(block, "ActiveEnterTimestamp")); ok {
				row.ActiveSince = since.UTC().Format(time.RFC3339)
			}
		}
		if n, ok := systemdUint(firstValue(block, "NRestarts")); ok && n <= math.MaxInt64 {
			value := int64(n)
			row.Restarts = &value
		}
		if n, ok := systemdUint(firstValue(block, "MemoryCurrent")); ok {
			row.MemoryBytes = &n
		}
		if n, ok := systemdUint(firstValue(block, "CPUUsageNSec")); ok {
			row.CPUNanos = &n
		}
		result = append(result, row)
	}
	return result, true
}

// systemdUint reads a counter; "[not set]", empty and UINT64_MAX (systemd's
// "no value") are not a number.
func systemdUint(raw string) (uint64, bool) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || value == math.MaxUint64 {
		return 0, false
	}
	return value, true
}

// parseSystemdTimestamp reads "@1730800000" (--timestamp=unix) or systemd's
// default "Tue 2026-11-05 10:00:00 UTC". Empty or "n/a" is not a time.
func parseSystemdTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "n/a" {
		return time.Time{}, false
	}
	if seconds, ok := strings.CutPrefix(raw, "@"); ok {
		n, err := strconv.ParseInt(seconds, 10, 64)
		if err != nil || n <= 0 {
			return time.Time{}, false
		}
		return time.Unix(n, 0), true
	}
	for _, layout := range []string{"Mon 2006-01-02 15:04:05 MST", "Mon 2006-01-02 15:04:05 -0700"} {
		if at, err := time.Parse(layout, raw); err == nil {
			return at, true
		}
	}
	return time.Time{}, false
}
