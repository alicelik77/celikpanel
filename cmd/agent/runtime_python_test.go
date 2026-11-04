package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Python apps (alpha.119, W5): the distribution Python, the requirements
// refusal, the build unit started once per operation, its completion (one
// rename, older generations removed as the site's account) and its failure
// (the package named, the partial environment removed), and the unit of a
// built environment.

type pythonHost struct {
	*runtimeAppHost
	mu      sync.Mutex
	files   map[string]string
	dirs    []string
	venvs   map[string]bool
	runs    [][]string
	written map[string]string
}

var pythonTarget = transport.RuntimeAppTarget{AppID: 12, Kind: "python", SiteID: 4, SubscriptionID: 3, DomainID: 7, Domain: "shop.example.com"}

func pythonSpec() transport.RuntimeAppSpec {
	return transport.RuntimeAppSpec{Entry: "mysite.wsgi:application", Port: 30010, Level: "full",
		Limits: transport.RuntimeAppLimits{MemoryMB: 512, CPUPercent: 100, TasksMax: 256},
		Python: &transport.RuntimeAppPython{Server: "wsgi", Workers: 2}}
}

func usePythonHost(t *testing.T) *pythonHost {
	t.Helper()
	base := useRuntimeAppHost(t)
	base.facts = siteruntime.FileFacts{Dir: true, UID: 1500, Mode: 0o750, FolderUID: 1500}
	h := &pythonHost{runtimeAppHost: base, files: map[string]string{"src/requirements.txt": "Django==4.2.16\n"},
		venvs: map[string]bool{}, written: map[string]string{}}
	oldProbe, oldSource, oldDirs, oldComplete, oldBuildDir, oldChown, oldRun :=
		runtimePythonProbe, runtimeAppSourceFile, runtimeAppDirNames, runtimeAppVenvComplete, runtimeAppBuildDir, runtimeAppBuildChown, serviceAppRun
	t.Cleanup(func() {
		runtimePythonProbe, runtimeAppSourceFile, runtimeAppDirNames, runtimeAppVenvComplete, runtimeAppBuildDir, runtimeAppBuildChown, serviceAppRun =
			oldProbe, oldSource, oldDirs, oldComplete, oldBuildDir, oldChown, oldRun
	})
	runtimePythonProbe = func() transport.RuntimePythonResponse {
		return transport.RuntimePythonResponse{Minor: "3.13", Expected: "3.13", Ready: true}
	}
	runtimeAppSourceFile = func(_ string, rel string, _ int64) ([]byte, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		text, ok := h.files[rel]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(text), nil
	}
	runtimeAppDirNames = func(string) ([]string, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		return append([]string{}, h.dirs...), nil
	}
	runtimeAppVenvComplete = func(_ string, name string) bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.venvs[name]
	}
	runtimeAppBuildDir = filepath.Join(t.TempDir(), "rtbuild")
	runtimeAppBuildChown = func(string, int, int) error { return nil }
	serviceAppRun = func(_ context.Context, name string, args ...string) ([]byte, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if name == "journalctl" {
			return []byte(base.journal), nil
		}
		h.runs = append(h.runs, append([]string{name}, args...))
		if name == "systemd-run" {
			// What the build will read, as the unit would see it.
			for _, f := range []string{"server.txt", "requirements.txt", "constraints.txt"} {
				if raw, err := os.ReadFile(filepath.Join(runtimeAppBuildDir, "12", f)); err == nil {
					h.written[f] = string(raw)
				}
			}
		}
		return nil, nil
	}
	return h
}

func TestDistributionPythonIsTheSystemsOwn(t *testing.T) {
	oldLink, oldExists := runtimePythonReadlink, runtimePythonFileExists
	t.Cleanup(func() { runtimePythonReadlink, runtimePythonFileExists = oldLink, oldExists })
	present := map[string]bool{}
	link := "python3.11"
	runtimePythonReadlink = func(string) (string, error) {
		if link == "" {
			return "", os.ErrNotExist
		}
		return link, nil
	}
	runtimePythonFileExists = func(p string) bool { return present[p] }
	present["/usr/bin/python3.11"] = true
	if py := pythonFromHost("debian", "12", "apt"); py.Ready || !strings.Contains(py.Detail, "python3-venv") || py.Reason != siteruntime.PythonRuntimeMissing {
		t.Fatalf("without venv: %+v", py)
	}
	present["/usr/lib/python3.11/ensurepip/__init__.py"] = true
	if py := pythonFromHost("debian", "12", "apt"); !py.Ready || py.Minor != "3.11" || strings.Join(py.Packages, " ") != "python3 python3-venv" {
		t.Fatalf("debian 12: %+v", py)
	}
	// Another python3 than the distribution's is never used.
	link = "python3.12"
	if py := pythonFromHost("debian", "12", "apt"); py.Ready || !strings.Contains(py.Detail, "own Python is 3.11") {
		t.Fatalf("a foreign python3: %+v", py)
	}
	// Arch rolls: its python3 is the distribution's whatever the minor,
	// when this release pins an app server for it.
	link = "python3.14"
	present["/usr/bin/python3.14"], present["/usr/lib/python3.14/ensurepip/__init__.py"] = true, true
	if py := pythonFromHost("arch", "", "pacman"); !py.Ready || py.Minor != "3.14" || strings.Join(py.Packages, " ") != "python" {
		t.Fatalf("arch: %+v", py)
	}
	link = "python3.15"
	present["/usr/bin/python3.15"], present["/usr/lib/python3.15/ensurepip/__init__.py"] = true, true
	if py := pythonFromHost("arch", "", "pacman"); py.Ready || !strings.Contains(py.Detail, "no app-server pins") {
		t.Fatalf("arch 3.15: %+v", py)
	}
	link = ""
	if py := pythonFromHost("ubuntu", "24.04", "apt"); py.Ready || !strings.Contains(py.Detail, "not installed") {
		t.Fatalf("no python: %+v", py)
	}
	if py := pythonFromHost("fedora", "40", "dnf"); py.Ready {
		t.Fatalf("unsupported system: %+v", py)
	}
}

func TestPythonCheckRefusesTheRequirementsLine(t *testing.T) {
	h := usePythonHost(t)
	var resp transport.RuntimeAppCheckResponse
	if err := (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: pythonTarget, Spec: pythonSpec()}, &resp); err != nil || !resp.Done {
		t.Fatalf("good app: %+v %v", resp, err)
	}
	h.files["src/requirements.txt"] = "Django==4.2.16\n--extra-index-url https://evil.example.com/simple\n"
	resp = transport.RuntimeAppCheckResponse{}
	_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: pythonTarget, Spec: pythonSpec()}, &resp)
	if resp.Done || resp.Reason != siteruntime.RequirementsRefused || resp.Class != transport.RuntimeAppPrerequisite ||
		!strings.HasPrefix(resp.Error, "requirements.txt:2 extra_index_url: ") {
		t.Fatalf("refusal %+v", resp)
	}
	h.facts.Dir = false
	resp = transport.RuntimeAppCheckResponse{}
	_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: pythonTarget, Spec: pythonSpec()}, &resp)
	if resp.Reason != siteruntime.PythonSrcMissing {
		t.Fatalf("src a file: %+v", resp)
	}
	runtimePythonProbe = func() transport.RuntimePythonResponse {
		return transport.RuntimePythonResponse{Reason: siteruntime.PythonRuntimeMissing, Detail: "Python 3 is not installed"}
	}
	resp = transport.RuntimeAppCheckResponse{}
	_ = (&Agent{}).RuntimeAppCheckV1(&transport.RuntimeAppCheckRequest{Target: pythonTarget, Spec: pythonSpec()}, &resp)
	if resp.Reason != siteruntime.PythonRuntimeMissing || resp.Class != transport.RuntimeAppPrerequisite {
		t.Fatalf("no python: %+v", resp)
	}
}

func TestPythonBuildStartsOnceAndCompletesAsTheSiteAccount(t *testing.T) {
	h := usePythonHost(t)
	h.dirs = []string{"src", "venv-1", "venv-2", "venv-3.partial"}
	h.venvs["venv-1"], h.venvs["venv-2"] = true, true
	// The unit runs venv-2.
	unitDir := appUnitDirOverride
	if err := os.WriteFile(filepath.Join(unitDir, "celikrt-12.service"), []byte("ExecStart=/var/www/celikpanel/subscriptions/3/sites/7/app/venv-2/bin/python -m gunicorn a:b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := &transport.RuntimeAppBuildRequest{OperationID: "op-python-1", Target: pythonTarget, Spec: pythonSpec()}
	var resp transport.RuntimeAppBuildResponse
	if err := (&Agent{}).RuntimeAppBuildV1(req, &resp); err != nil || !resp.Done || resp.Venv != 4 || resp.Previous != 2 || resp.Phase != siteruntime.BuildPhaseRunning {
		t.Fatalf("build: %+v %v", resp, err)
	}
	if len(h.runs) != 1 || h.runs[0][0] != "systemd-run" {
		t.Fatalf("runs %v", h.runs)
	}
	run := strings.Join(h.runs[0], "\n")
	for _, want := range []string{"--unit=celikrtbuild-12.service", "--uid=shop_example_com",
		"--property=ExecStartPre=/usr/bin/python3.13 -m venv /var/www/celikpanel/subscriptions/3/sites/7/app/venv-4.partial"} {
		if !strings.Contains(run, want) {
			t.Fatalf("missing %q\n%s", want, run)
		}
	}
	if !strings.Contains(h.written["server.txt"], "gunicorn==26.2.0 --hash=sha256:") || h.written["requirements.txt"] != "Django==4.2.16\n" {
		t.Fatalf("build files %v", h.written)
	}
	// The leftover partial was removed and the folders made as the site's account.
	if !containsArgs(h.asUser, "/bin/rm", "/var/www/celikpanel/subscriptions/3/sites/7/app/venv-3.partial") || !containsArgs(h.asUser, "/bin/mkdir", "/var/www/celikpanel/subscriptions/3/sites/7/app/.pip-cache") {
		t.Fatalf("as user %v", h.asUser)
	}
	// A repeat of the same operation starts nothing.
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildV1(req, &resp)
	if len(h.runs) != 1 || resp.Venv != 4 {
		t.Fatalf("repeat started a second build: %v", h.runs)
	}
	// Still running: nothing changes.
	h.show = "LoadState=loaded\nActiveState=activating\nResult=success\nInvocationID=0123456789abcdef0123456789abcdef\n"
	adv := &transport.RuntimeAppBuildAdvanceRequest{OperationID: "op-python-1", Target: pythonTarget}
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(adv, &resp)
	if !resp.Done || resp.Phase != siteruntime.BuildPhaseRunning {
		t.Fatalf("running: %+v", resp)
	}
	before := len(h.asUser)
	// Finished: one rename into place, then everything but the new and the
	// running generation goes.
	h.show = "LoadState=loaded\nActiveState=active\nResult=success\nInvocationID=0123456789abcdef0123456789abcdef\n"
	h.venvs["venv-4.partial"] = true
	h.dirs = []string{"src", "venv-1", "venv-2", "venv-4"}
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(adv, &resp)
	if !resp.Done || resp.Phase != siteruntime.BuildPhaseSucceeded || resp.Venv != 4 {
		t.Fatalf("finished: %+v", resp)
	}
	done := h.asUser[before:]
	if len(done) == 0 || strings.Join(done[0], " ") != "/bin/mv -T -- /var/www/celikpanel/subscriptions/3/sites/7/app/venv-4.partial /var/www/celikpanel/subscriptions/3/sites/7/app/venv-4" {
		t.Fatalf("rename %v", done)
	}
	if !containsArgs(done, "/bin/rm", "/var/www/celikpanel/subscriptions/3/sites/7/app/venv-1") || containsArgs(done, "/bin/rm", "/var/www/celikpanel/subscriptions/3/sites/7/app/venv-2") {
		t.Fatalf("cleanup %v", done)
	}
	if _, err := os.Stat(filepath.Join(runtimeAppBuildDir, "12")); !os.IsNotExist(err) {
		t.Fatal("the build files stayed")
	}
	// Completed: repeating answers the record and changes nothing more.
	n := len(h.asUser)
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(adv, &resp)
	if len(h.asUser) != n || resp.Phase != siteruntime.BuildPhaseSucceeded {
		t.Fatal("a repeat changed something")
	}
}

func TestPythonBuildFailureNamesThePackageAndKeepsTheRunningApp(t *testing.T) {
	h := usePythonHost(t)
	req := &transport.RuntimeAppBuildRequest{OperationID: "op-python-2", Target: pythonTarget, Spec: pythonSpec()}
	var resp transport.RuntimeAppBuildResponse
	_ = (&Agent{}).RuntimeAppBuildV1(req, &resp)
	if !resp.Done || resp.Venv != 1 || resp.Previous != 0 {
		t.Fatalf("build %+v", resp)
	}
	h.show = "LoadState=loaded\nActiveState=failed\nResult=exit-code\nInvocationID=0123456789abcdef0123456789abcdef\n"
	h.journal = "Collecting numpy==1.21.0\nERROR: Could not find a version that satisfies the requirement numpy==1.21.0 (from versions: 2.1.0)\nERROR: No matching distribution found for numpy==1.21.0\n"
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{OperationID: "op-python-2", Target: pythonTarget}, &resp)
	if resp.Done || resp.Reason != siteruntime.WheelUnavailable || resp.Package != "numpy" || resp.Class != transport.RuntimeAppVerifiedFailure ||
		!strings.Contains(resp.Error, "numpy has no ready-made package") || !strings.Contains(resp.Error, "running app was not changed") {
		t.Fatalf("failure %+v", resp)
	}
	if !containsArgs(h.asUser, "/bin/rm", "/var/www/celikpanel/subscriptions/3/sites/7/app/venv-1.partial") || containsArgs(h.asUser, "/bin/mv") {
		t.Fatalf("as user %v", h.asUser)
	}
	// A lost build is unknown, never a success.
	_ = os.Remove(filepath.Join(runtimeAppStateDir(12), "build.json"))
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{OperationID: "op-python-3", Target: pythonTarget, Spec: pythonSpec()}, &resp)
	h.show = "LoadState=not-found\nActiveState=inactive\n"
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{OperationID: "op-python-3", Target: pythonTarget}, &resp)
	if resp.Done || resp.Class != transport.RuntimeAppUnknown || resp.Reason != siteruntime.BuildFailed {
		t.Fatalf("lost %+v", resp)
	}
	// Another operation's id is not answered with this record.
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildAdvanceV1(&transport.RuntimeAppBuildAdvanceRequest{OperationID: "op-python-9", Target: pythonTarget}, &resp)
	if resp.Done || resp.Class != transport.RuntimeAppUnknown {
		t.Fatalf("foreign %+v", resp)
	}
	// A refused requirements line starts nothing.
	h.files["src/requirements.txt"] = "-e git+https://example.com/x.git#egg=x\n"
	runs := len(h.runs)
	resp = transport.RuntimeAppBuildResponse{}
	_ = (&Agent{}).RuntimeAppBuildV1(&transport.RuntimeAppBuildRequest{OperationID: "op-python-4", Target: pythonTarget, Spec: pythonSpec()}, &resp)
	if resp.Done || resp.Reason != siteruntime.RequirementsRefused || !strings.HasPrefix(resp.Error, "requirements.txt:1 editable: ") || len(h.runs) != runs {
		t.Fatalf("refused %+v", resp)
	}
}

func TestPythonRenderNeedsTheBuiltEnvironment(t *testing.T) {
	h := usePythonHost(t)
	spec := pythonSpec()
	spec.Python.Venv = 4
	req := &transport.RuntimeAppRenderRequest{OperationID: "op-python-5", Target: pythonTarget, Spec: spec}
	var resp transport.RuntimeAppRenderResponse
	_ = (&Agent{}).RuntimeAppRenderV1(req, &resp)
	if resp.Done || resp.Reason != siteruntime.PythonVenvMissing {
		t.Fatalf("missing venv: %+v", resp)
	}
	h.venvs["venv-4"] = true
	resp = transport.RuntimeAppRenderResponse{}
	if err := (&Agent{}).RuntimeAppRenderV1(req, &resp); err != nil || !resp.Done {
		t.Fatalf("render %+v %v", resp, err)
	}
	raw, _ := os.ReadFile(filepath.Join(appUnitDirOverride, "celikrt-12.service"))
	if !strings.Contains(string(raw), "ExecStart=/var/www/celikpanel/subscriptions/3/sites/7/app/venv-4/bin/python -m gunicorn --bind 127.0.0.1:30010 --workers 2 mysite.wsgi:application\n") ||
		strings.Contains(string(raw), "NoExecPaths") || strings.Contains(string(raw), "MemoryDenyWriteExecute") {
		t.Fatalf("unit\n%s", raw)
	}
	if runtimeAppActiveVenv(12) != 4 {
		t.Fatal("active venv")
	}
	// The screen's read names a gone environment.
	delete(h.venvs, "venv-4")
	var status transport.RuntimeAppStatusResponse
	_ = (&Agent{}).RuntimeAppStatusV1(&transport.RuntimeAppStatusRequest{Target: pythonTarget, Spec: spec, Entry: true}, &status)
	if status.Entry == nil || status.Entry.Code != siteruntime.PythonVenvMissing {
		t.Fatalf("status %+v", status.Entry)
	}
}

func containsArgs(calls [][]string, program string, args ...string) bool {
	for _, call := range calls {
		if call[0] != program {
			continue
		}
		joined := strings.Join(call, " ")
		ok := true
		for _, a := range args {
			if !strings.Contains(joined, a) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}
