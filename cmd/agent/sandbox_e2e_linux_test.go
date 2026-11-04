//go:build linux

package main

// End-to-end check of the sandboxed units on a REAL systemd (alpha.119 lab
// round 3: the Python build never ran on Debian 13). It drives the Agent's
// own code paths (RuntimeAppBuildV1 and RuntimeAppBuildAdvanceV1 polled
// as the Panel polls them, RuntimeAppRenderV1, RuntimeAppControlV1) with the
// real bind template, the real helper (the Agent binary) and a real site
// account under the hosting tree.
//
// It changes the host: a site account, folders under
// /var/www/celikpanel/subscriptions/99090x, units, the bind template and the
// helper. Run it only on a throwaway system, as root:
//
//	CGO_ENABLED=0 go build -o /tmp/celikpanel-agent ./cmd/agent
//	CELIKPANEL_E2E_SYSTEMD=1 CELIKPANEL_E2E_AGENT=/tmp/celikpanel-agent \
//	  go test -count=1 -v -run TestE2E ./cmd/agent/
//
// It removes the accounts, folders and units it made. The bind template
// (/etc/systemd/system/celikpanel-sandbox@.service) and the helper
// (/usr/lib/celikpanel-sandbox/) stay, as on a CelikPanel host; on a system
// that had none, remove both and run systemctl daemon-reload afterwards.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

type e2eSite struct {
	sub, dom int
	domain   string
	user     string
	home     string
}

func e2eHost(t *testing.T) {
	t.Helper()
	if os.Getenv("CELIKPANEL_E2E_SYSTEMD") != "1" {
		t.Skip("set CELIKPANEL_E2E_SYSTEMD=1 on a throwaway system with systemd (root)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	agent := os.Getenv("CELIKPANEL_E2E_AGENT")
	if agent == "" {
		t.Fatal("CELIKPANEL_E2E_AGENT names the Agent binary built from this tree (the helper)")
	}
	// TestMain points the sandbox's native files at a throwaway folder; this
	// test needs the host's own: the helper (the Agent started under its own
	// name, so never the test binary), the bind template in
	// /etc/systemd/system, a real daemon-reload and the real folder walk.
	old := sandboxHelperSource
	oldSum := sandboxHelperSelfSum
	oldEgress, oldUnits, oldReload, oldRefusal := sandboxEgressDir, sandboxUnitDir, sandboxDaemonReload, sandboxFolderRefusal
	oldState, oldLedger, oldWait := runtimeAppStateRoot, sandboxLedgerPath, transientUnloadWait
	transientUnloadWait = 5 * time.Second
	sandboxHelperSource, sandboxHelperSelfSum = agent, ""
	sandboxEgressDir, sandboxUnitDir, sandboxDaemonReload, sandboxFolderRefusal = sitesandbox.EgressDir, unitDir, hostDaemonReload, hostSandboxFolderRefusal
	runtimeAppStateRoot = t.TempDir()
	sandboxLedgerPath = filepath.Join(t.TempDir(), "native-files.json")
	t.Cleanup(func() {
		sandboxHelperSource, sandboxHelperSelfSum = old, oldSum
		sandboxEgressDir, sandboxUnitDir, sandboxDaemonReload, sandboxFolderRefusal = oldEgress, oldUnits, oldReload, oldRefusal
		runtimeAppStateRoot, sandboxLedgerPath, transientUnloadWait = oldState, oldLedger, oldWait
	})
}

func e2eRun(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func e2eTry(name string, args ...string) string {
	out, _ := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

// newE2ESite makes a site as CelikPanel lays one out: root's subscription
// and "sites" folders, the account's home (its own, not writable by others).
func newE2ESite(t *testing.T, sub, dom int, domain string) e2eSite {
	t.Helper()
	home, err := hostingpath.SiteHome(sub, dom)
	if err != nil {
		t.Fatal(err)
	}
	s := e2eSite{sub: sub, dom: dom, domain: domain, user: services.SiteUsername(domain), home: home}
	subDir := filepath.Join(hostingpath.SubscriptionsRoot(), strconv.Itoa(sub))
	for _, dir := range []string{hostingpath.SubscriptionsRoot(), subDir, filepath.Join(subDir, "sites")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(dir, 0o755)
	}
	_ = exec.Command("userdel", s.user).Run()
	e2eRun(t, "useradd", "--home-dir", home, "--no-create-home", "--shell", "/usr/sbin/nologin", "--user-group", s.user)
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	e2eRun(t, "chown", s.user+":"+s.user, home)
	t.Cleanup(func() {
		// runuser's PAM session may have started a user manager, which
		// userdel refuses to remove an account under.
		_ = exec.Command("loginctl", "terminate-user", s.user).Run()
		for i := 0; i < 20; i++ {
			if exec.Command("userdel", s.user).Run() == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		_ = os.RemoveAll(subDir)
	})
	return s
}

// asUser writes a file as the site's account.
func (s e2eSite) write(t *testing.T, rel, text string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(s.home, rel)
	e2eRun(t, "runuser", "-u", s.user, "--", "mkdir", "-p", filepath.Dir(p))
	if err := os.WriteFile(p, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
	e2eRun(t, "chown", s.user+":"+s.user, p)
	_ = os.Chmod(p, mode)
}

func (s e2eSite) target(appID int, kind siteruntime.Kind) transport.RuntimeAppTarget {
	return transport.RuntimeAppTarget{AppID: appID, Kind: string(kind), SiteID: s.dom, SubscriptionID: s.sub, DomainID: s.dom, Domain: s.domain}
}

func e2eShow(unit string) map[string]string {
	return parseShow(e2eTry("systemctl", "show", unit, "--property=LoadState,ActiveState,SubState,Result,InvocationID,Job"))
}

// e2eDiagnose logs both units' state and journals.
func e2eDiagnose(t *testing.T, unit string) {
	t.Helper()
	bind := sitesandbox.BindUnit(unit)
	for _, u := range []string{unit, bind} {
		t.Logf("%s: %v", u, e2eShow(u))
		t.Logf("journal %s:\n%s", u, e2eTry("journalctl", "-u", u, "-n", "40", "--no-pager", "-o", "short-precise"))
	}
}

func e2eSpec(kind siteruntime.Kind) transport.RuntimeAppSpec {
	caps := sandboxCaps(context.Background())
	a := sitesandbox.Assess(caps)
	spec := transport.RuntimeAppSpec{Port: 30990, Level: string(a.Level), AcceptedGaps: gapNames(a),
		Limits: transport.RuntimeAppLimits{MemoryMB: 512, CPUPercent: 100, TasksMax: 256}}
	if kind == siteruntime.KindPython {
		spec.Entry = "app:app"
		spec.Python = &transport.RuntimeAppPython{Server: "wsgi", Workers: 1}
	}
	return spec
}

const e2eWSGI = `def app(environ, start_response):
    start_response("200 OK", [("Content-Type", "text/plain")])
    return [b"celikpanel e2e\n"]
`

// e2eBuild runs one build as the Panel does (runRuntimePythonBuild): start
// it, then read it at once and every 3 seconds until it ends.
func e2eBuild(t *testing.T, s e2eSite, appID int, op string) transport.RuntimeAppBuildResponse {
	t.Helper()
	target := s.target(appID, siteruntime.KindPython)
	var started transport.RuntimeAppBuildResponse
	if err := (&Agent{}).RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{OperationID: op, Target: target, Spec: e2eSpec(siteruntime.KindPython)}, &started); err != nil {
		t.Fatal(err)
	}
	if !started.Done {
		t.Fatalf("the build did not start: %s/%s: %s", started.Class, started.Reason, started.Error)
	}
	deadline := time.Now().Add(15 * time.Minute)
	for {
		var state transport.RuntimeAppBuildResponse
		_ = (&Agent{}).RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{OperationID: op, Target: target}, &state)
		if !state.Done || state.Phase != siteruntime.BuildPhaseRunning {
			t.Logf("build %s: done=%v phase=%s class=%s reason=%s: %s %s", op, state.Done, state.Phase, state.Class, state.Reason, state.Error, strings.Join(state.Lines, " | "))
			return state
		}
		if time.Now().After(deadline) {
			t.Fatal("the build did not end in 15 minutes")
		}
		time.Sleep(3 * time.Second)
	}
}

// TestE2EPythonBuildAndUnitsOnRealSystemd: a Python app is built, rendered,
// started and stopped; a second deploy builds again; a program starts and
// stops. Every unit uses the real bind template and helper.
func TestE2EPythonBuildAndUnitsOnRealSystemd(t *testing.T) {
	e2eHost(t)
	py := runtimePythonProbe()
	if !py.Ready {
		t.Skipf("the distribution Python is not ready here: %s %s", py.Detail, py.Error)
	}
	s := newE2ESite(t, 990901, 990901, "e2e-py.test")
	s.write(t, "app/src/app.py", e2eWSGI, 0o644)
	s.write(t, "app/src/requirements.txt", "", 0o644)
	const appID = 990901
	unit := siteruntime.UnitName(appID)
	build := siteruntime.BuildUnitName(appID)
	t.Cleanup(func() {
		for _, u := range []string{unit, build} {
			_ = exec.Command("systemctl", "stop", u, sitesandbox.BindUnit(u)).Run()
			_ = exec.Command("systemctl", "disable", u).Run()
			_ = exec.Command("systemctl", "reset-failed", u, sitesandbox.BindUnit(u)).Run()
		}
		_ = os.Remove(filepath.Join("/etc/systemd/system", unit))
		_ = os.RemoveAll(filepath.Join(siteruntime.BuildDir, strconv.Itoa(appID)))
		_ = exec.Command("systemctl", "daemon-reload").Run()
	})

	for round, op := range []string{"e2e-build-1", "e2e-build-2"} {
		state := e2eBuild(t, s, appID, op)
		if state.Phase != siteruntime.BuildPhaseSucceeded {
			e2eDiagnose(t, build)
			// Without PyPI the build must at least have run: a pip
			// failure with its reason, never a build that did not start.
			if state.Reason == siteruntime.PyPIUnreachable {
				t.Skipf("round %d reached pip, which could not reach PyPI: %s", round+1, state.Error)
			}
			t.Fatalf("round %d: the build did not succeed: %s/%s: %s", round+1, state.Class, state.Reason, state.Error)
		}
		t.Logf("round %d: venv-%d built (previous %d); %s: %v", round+1, state.Venv, state.Previous, build, e2eShow(build))
		if bind := e2eShow(sitesandbox.BindUnit(build)); bind["ActiveState"] == "active" {
			t.Errorf("round %d: the build's bind unit still runs after the build ended: %v", round+1, bind)
		}
		// The app on the new environment: render, start, serve, stop.
		spec := e2eSpec(siteruntime.KindPython)
		spec.Python.Venv = state.Venv
		var rendered transport.RuntimeAppRenderResponse
		_ = (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: op, Target: s.target(appID, siteruntime.KindPython), Spec: spec}, &rendered)
		if !rendered.Done {
			t.Fatalf("render: %s/%s: %s", rendered.Class, rendered.Reason, rendered.Error)
		}
		e2eStartStop(t, s.target(appID, siteruntime.KindPython), unit, spec.Port)
	}

	// A build someone stops while it runs ends without a result, and the
	// next deploy's build still starts under the same unit name.
	target := s.target(appID, siteruntime.KindPython)
	var started transport.RuntimeAppBuildResponse
	_ = (&Agent{}).RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{OperationID: "e2e-build-3", Target: target, Spec: e2eSpec(siteruntime.KindPython)}, &started)
	if !started.Done {
		t.Fatalf("build 3: %s/%s: %s", started.Class, started.Reason, started.Error)
	}
	// The same operation again (a lost answer) starts nothing new.
	first := ""
	for i := 0; i < 50 && first == ""; i++ {
		first = e2eShow(build)["InvocationID"]
		time.Sleep(100 * time.Millisecond)
	}
	var again transport.RuntimeAppBuildResponse
	_ = (&Agent{}).RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{OperationID: "e2e-build-3", Target: target, Spec: e2eSpec(siteruntime.KindPython)}, &again)
	if now := e2eShow(build)["InvocationID"]; !again.Done || first == "" || now != first {
		t.Fatalf("a repeat of the running build's operation started another build: %q then %q (%+v)", first, now, again)
	}
	e2eRun(t, "systemctl", "stop", build)
	var stopped transport.RuntimeAppBuildResponse
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{OperationID: "e2e-build-3", Target: target}, &stopped)
	if stopped.Phase != siteruntime.BuildPhaseFailed {
		t.Fatalf("a stopped build: %+v", stopped)
	}
	t.Logf("build 3 stopped from outside: %s/%s: %s", stopped.Class, stopped.Reason, stopped.Error)
	if state := e2eBuild(t, s, appID, "e2e-build-4"); state.Phase != siteruntime.BuildPhaseSucceeded {
		e2eDiagnose(t, build)
		t.Fatalf("the build after a stopped one: %s/%s: %s", state.Class, state.Reason, state.Error)
	}
	t.Logf("build 4 after the stopped one: succeeded")
}

// e2eStartStop starts a rendered runtime unit through the Agent, checks it
// and its bind unit run, then stops it and checks nothing is left mounted.
func e2eStartStop(t *testing.T, target transport.RuntimeAppTarget, unit string, port int) {
	t.Helper()
	var step transport.RuntimeAppStepResponse
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: target, Action: "start"}, &step)
	if !step.Done {
		e2eDiagnose(t, unit)
		t.Fatalf("start %s: %s/%s: %s", unit, step.Class, step.Reason, step.Error)
	}
	var active map[string]string
	for i := 0; i < 30; i++ {
		active = e2eShow(unit)
		if active["ActiveState"] == "active" && (port == 0 || e2eTry("ss", "-Htln", "sport = :"+strconv.Itoa(port)) != "") {
			break
		}
		time.Sleep(time.Second)
	}
	bind := e2eShow(sitesandbox.BindUnit(unit))
	if active["ActiveState"] != "active" || bind["ActiveState"] != "active" {
		e2eDiagnose(t, unit)
		t.Fatalf("%s does not run: %v; bind %v", unit, active, bind)
	}
	if port > 0 {
		if out := e2eTry("curl", "-sS", "--max-time", "5", "http://127.0.0.1:"+strconv.Itoa(port)+"/"); !strings.Contains(out, "celikpanel e2e") {
			t.Errorf("%s did not answer on %d: %q", unit, port, out)
		}
	}
	mount := filepath.Join(sitesandbox.MountRoot, unit, "app")
	if !strings.Contains(e2eTry("findmnt", "-rn", "-o", "TARGET", mount), mount) {
		t.Errorf("the bind unit of %s mounted nothing on %s", unit, mount)
	}
	t.Logf("%s runs (%v), bind unit %v", unit, active["SubState"], bind["SubState"])
	step = transport.RuntimeAppStepResponse{}
	_ = (&Agent{}).RuntimeAppControlV1(&transport.RuntimeAppControlRequest{Target: target, Action: "stop"}, &step)
	if !step.Done {
		t.Fatalf("stop %s: %s/%s: %s", unit, step.Class, step.Reason, step.Error)
	}
	e2eWaitBindStopped(unit, mount)
	if b := e2eShow(sitesandbox.BindUnit(unit)); !e2eStopped(b) || e2eTry("findmnt", "-rn", "-o", "TARGET", mount) != "" {
		t.Errorf("after the stop of %s: bind unit %v, mount %q", unit, b, e2eTry("findmnt", "-rn", "-o", "TARGET", mount))
	}
	t.Logf("%s stopped: %v; bind unit %v", unit, e2eShow(unit)["ActiveState"], e2eShow(sitesandbox.BindUnit(unit))["ActiveState"])
}

func e2eStopped(u map[string]string) bool {
	return u["ActiveState"] == "inactive" || u["ActiveState"] == "failed"
}

// e2eWaitBindStopped waits until the bind unit of unit has stopped and its
// mount is gone (a bind unit stops just after its unit).
func e2eWaitBindStopped(unit, mount string) {
	for i := 0; i < 30; i++ {
		if e2eStopped(e2eShow(sitesandbox.BindUnit(unit))) && e2eTry("findmnt", "-rn", "-o", "TARGET", mount) == "" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// TestE2EProgramUnitOnRealSystemd: an uploaded program (a copy of sleep)
// starts and stops under the real bind template.
func TestE2EProgramUnitOnRealSystemd(t *testing.T) {
	e2eHost(t)
	s := newE2ESite(t, 990902, 990902, "e2e-prog.test")
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep program")
	}
	raw, err := os.ReadFile(sleep)
	if err != nil {
		t.Fatal(err)
	}
	s.write(t, "app/bin/server", string(raw), 0o750)
	const appID = 990902
	unit := siteruntime.UnitName(appID)
	t.Cleanup(func() {
		_ = exec.Command("systemctl", "stop", unit, sitesandbox.BindUnit(unit)).Run()
		_ = exec.Command("systemctl", "disable", unit).Run()
		_ = exec.Command("systemctl", "reset-failed", unit, sitesandbox.BindUnit(unit)).Run()
		_ = os.Remove(filepath.Join("/etc/systemd/system", unit))
		_ = exec.Command("systemctl", "daemon-reload").Run()
	})
	spec := e2eSpec(siteruntime.KindProgram)
	spec.Entry, spec.Args = "server", []string{"infinity"}
	target := s.target(appID, siteruntime.KindProgram)
	var rendered transport.RuntimeAppRenderResponse
	_ = (&Agent{}).RuntimeAppRenderV1(&transport.RuntimeAppRenderRequest{OperationID: "e2e-prog-1", Target: target, Spec: spec}, &rendered)
	if !rendered.Done {
		t.Fatalf("render: %s/%s: %s", rendered.Class, rendered.Reason, rendered.Error)
	}
	e2eStartStop(t, target, unit, 0)
	fmt.Fprintln(os.Stderr, "program unit: started and stopped")
}
