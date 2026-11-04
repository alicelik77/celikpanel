package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Transient sandboxed units (the Python build celikrtbuild-<id>, a service
// app's install celiksvcinst-<id>) are made by systemd-run under a fixed
// name. systemd-run refuses that name while systemd still has a unit of it
// loaded ("Unit … was already loaded or has a fragment file"), and an
// inactive transient unit stays loaded while its bind unit refers to it
// (alpha.119 lab round 3: every redeploy failed after the first build was
// stopped). So before a new attempt the old unit and its bind unit are
// stopped, reset, and the unit must be gone.
//
// Geçici korumalı birimler sabit adla kurulur; yeni denemeden önce eski
// birim ve bağlama birimi durdurulur, sıfırlanır ve birimin kalkması beklenir.

// transientUnloadWait bounds the wait for systemd to unload the unit.
var transientUnloadWait = 5 * time.Second

// clearTransientUnit stops a transient unit and its bind unit, resets both
// and waits until systemd no longer has the unit loaded. Nothing else is
// changed. It is called only where no attempt of the unit may run any more
// (a new operation, or the end of one).
func clearTransientUnit(unit string) error {
	bind := sitesandbox.BindUnit(unit)
	_, _ = appUnitSystemctl(systemctlArgs("stop", unit)...)
	_, _ = appUnitSystemctl(systemctlArgs("stop", bind)...)
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", unit)...)
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", bind)...)
	deadline := time.Now().Add(transientUnloadWait)
	last := map[string]string{}
	for {
		out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=LoadState,ActiveState,Job")...)
		if err == nil {
			last = parseShow(string(out))
			if last["LoadState"] == "" || last["LoadState"] == "not-found" {
				return nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s of an earlier attempt is still loaded (%s %s) after it was stopped, so a new one cannot be made under its name. "+
		"Nothing was started. The administrator runs: systemctl stop %s %s; systemctl reset-failed %s %s. Then deploy again",
		unit, last["LoadState"], last["ActiveState"], unit, bind, unit, bind)
}

var helperRefusal = regexp.MustCompile(`was not started \(([a-z_]+)\)`)

// buildBindFailure explains a build whose start job ended without running
// it: its bind unit did not start. The helper's journal names a refused app
// folder (a prerequisite the customer or administrator fixes); anything
// else is an unknown result with the bind unit's own lines.
func buildBindFailure(ctx context.Context, appID int) (class, reason, detail string, lines []string) {
	bind := sitesandbox.BindUnit(siteruntime.BuildUnitName(appID))
	state := "unknown"
	if out, err := appUnitSystemctl(systemctlArgs("show", bind, "--property=ActiveState,Result")...); err == nil {
		p := parseShow(string(out))
		state = strings.TrimSpace(p["ActiveState"] + " " + p["Result"])
	}
	args := []string{"-u", bind, "-n", "30", "--no-pager", "-o", "cat"}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	var journal []string
	if out, err := serviceAppRun(ctx, "journalctl", args...); err == nil {
		journal = strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	}
	lines = siteruntime.BuildTail(journal, 8, 1200)
	for i := len(journal) - 1; i >= 0; i-- {
		if m := helperRefusal.FindStringSubmatch(journal[i]); m != nil {
			code := m[1]
			if code == sandboxbind.CodeNotReal || code == sandboxbind.CodeNotOwned {
				return transport.RuntimeAppPrerequisite, code,
					"the build did not start: the sandbox helper refused the app folder (" + code + "); the running app was not changed. " +
						"Make app a real folder of the site's own account, then deploy again. Helper: " + strings.Join(lines, " | "), lines
			}
		}
	}
	return transport.RuntimeAppUnknown, siteruntime.BuildFailed,
		"the build did not start: its sandbox unit " + bind + " did not start (" + state + "); the running app was not changed. " +
			"Read journalctl -u " + bind + ", then deploy again. Helper: " + strings.Join(lines, " | "), lines
}
