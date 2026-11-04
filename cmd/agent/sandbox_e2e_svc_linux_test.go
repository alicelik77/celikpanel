//go:build linux

package main

// The service apps' units on a REAL systemd (see sandbox_e2e_linux_test.go
// for how to run it): the install unit celiksvcinst-<id> made by systemd-run
// with the Agent's own argv (serviceAppInstallRunArgs), polled as the Panel
// polls it, and the long-running celiksvc-<id> rendered by RenderUnit, both
// under the real bind template and helper. The install command and the app
// are stand-ins (a shell line, a sleeping script): downloading n8n or Open
// WebUI is not what is checked here.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

func TestE2EServiceAppUnitsOnRealSystemd(t *testing.T) {
	e2eHost(t)
	s := newE2ESite(t, 990903, 990903, "e2e-svc.test")
	const installID = 990903
	app, _ := serviceapps.Lookup("open-webui")
	layout, err := serviceapps.LayoutFor(s.sub, s.dom, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	account, err := serviceAppLookupAccount(transport.ServiceAppTarget{SubscriptionID: s.sub, DomainID: s.dom, Domain: s.domain})
	if err != nil {
		t.Fatal(err)
	}
	sc := serviceAppContext{installID: installID, app: app, account: account, layout: layout}
	rel := strings.TrimPrefix(layout.Root, s.home+"/")
	s.write(t, filepath.Join(rel, "data", ".keep"), "", 0o600)
	install := serviceapps.InstallUnitName(installID) + ".service"
	unit := serviceapps.UnitName(installID) + ".service"
	envFile := serviceapps.EnvFile(installID)
	madeEnvDir := false
	if _, err := os.Stat(serviceapps.EnvDir); os.IsNotExist(err) {
		madeEnvDir = true
	}
	t.Cleanup(func() {
		for _, u := range []string{install, unit} {
			_ = exec.Command("systemctl", "stop", u, sitesandbox.BindUnit(u)).Run()
			_ = exec.Command("systemctl", "reset-failed", u, sitesandbox.BindUnit(u)).Run()
		}
		_ = os.Remove(filepath.Join("/etc/systemd/system", unit))
		_ = os.Remove(envFile)
		if madeEnvDir {
			_ = os.Remove(serviceapps.EnvDir)
		}
		_ = exec.Command("systemctl", "daemon-reload").Run()
	})

	// The install unit, twice: a second attempt must find the name free.
	caps := sandboxCaps(context.Background())
	for round := 1; round <= 2; round++ {
		var step transport.ServiceAppStepResponse
		if !serviceAppSandboxReady(sc, caps, &step) {
			t.Fatalf("sandbox: %s/%s: %s", step.Class, step.Reason, step.Error)
		}
		if err := clearTransientUnit(install); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		run, err := serviceAppInstallRunArgs(sc, app.Version, serviceapps.RuntimeChoice{Available: true, Path: "/usr/bin/python3"},
			serviceAppLimits(app, transport.ServiceAppLimits{}), caps, []string{"/bin/sh", "-c", "touch e2e-install-ran && echo install-ok"})
		if err != nil {
			t.Fatal(err)
		}
		if out, err := serviceAppRun(context.Background(), "systemd-run", run...); err != nil {
			t.Fatalf("round %d: systemd-run: %v: %s", round, err, out)
		}
		seen := []string{}
		deadline := time.Now().Add(2 * time.Minute)
		state := ""
		for {
			state, _ = installUnitState(installID)
			if len(seen) == 0 || seen[len(seen)-1] != state {
				seen = append(seen, state)
			}
			if state == "succeeded" || state == "failed" || time.Now().After(deadline) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if state != "succeeded" {
			e2eDiagnose(t, install)
			t.Fatalf("round %d: the install unit ended %q (states %v)", round, state, seen)
		}
		if fi, err := os.Stat(filepath.Join(layout.Root, "e2e-install-ran")); err != nil {
			t.Errorf("round %d: the install did not run in the app folder: %v", round, err)
		} else if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != account.UID {
			t.Errorf("round %d: the install ran as uid %d, not the site's account", round, st.Uid)
		}
		t.Logf("round %d: %s states %v; journal: %s", round, install, seen, e2eTry("journalctl", "-u", install, "-n", "3", "--no-pager", "-o", "cat"))
		_ = os.Remove(filepath.Join(layout.Root, "e2e-install-ran"))
	}
	if err := clearTransientUnit(install); err != nil {
		t.Fatal(err)
	}
	if b := e2eShow(sitesandbox.BindUnit(install)); !e2eStopped(b) {
		t.Errorf("the install's bind unit runs after the install was cleared: %v", b)
	}

	// The long-running unit, with a sleeping stand-in for the app.
	s.write(t, filepath.Join(rel, "app-"+app.Version, "bin", "open-webui"), "#!/bin/sh\nexec sleep infinity\n", 0o755)
	if err := os.MkdirAll(serviceapps.EnvDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte("E2E=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := serviceapps.RenderUnit(serviceapps.UnitParams{InstallID: installID, App: app, Version: app.Version, Domain: s.domain,
		User: account.Name, Group: account.Group, Layout: layout, Port: 21990, AuxPort: 21991,
		Runtime: serviceapps.RuntimeChoice{Available: true, Path: "/usr/bin/python3"}, Limits: app.Defaults, Caps: caps,
		DropInDir: "/etc/systemd/system/" + unit + ".d"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/etc/systemd/system", unit), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	e2eRun(t, "systemctl", "daemon-reload")
	e2eRun(t, "systemctl", "start", unit)
	mount := filepath.Join(sitesandbox.MountRoot, unit, "app")
	if a, b := e2eShow(unit), e2eShow(sitesandbox.BindUnit(unit)); a["ActiveState"] != "active" || b["ActiveState"] != "active" ||
		!strings.Contains(e2eTry("findmnt", "-rn", "-o", "TARGET", mount), mount) {
		e2eDiagnose(t, unit)
		t.Fatalf("%s does not run: %v; bind %v", unit, a, b)
	}
	t.Logf("%s runs; bind unit active, %s mounted", unit, mount)
	e2eRun(t, "systemctl", "stop", unit)
	e2eWaitBindStopped(unit, mount)
	if b := e2eShow(sitesandbox.BindUnit(unit)); !e2eStopped(b) || e2eTry("findmnt", "-rn", "-o", "TARGET", mount) != "" {
		t.Errorf("after the stop of %s: bind %v, mount %q", unit, b, e2eTry("findmnt", "-rn", "-o", "TARGET", mount))
	}
	t.Logf("%s stopped; bind unit %s", unit, e2eShow(sitesandbox.BindUnit(unit))["ActiveState"])
}
