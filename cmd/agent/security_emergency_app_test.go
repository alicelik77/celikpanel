package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

func setEmergencyAppLock(t *testing.T, lock bool, version string) transport.SetEmergencyAppLockResponse {
	t.Helper()
	var resp transport.SetEmergencyAppLockResponse
	if err := (&Agent{}).SetEmergencyAppLockV1(&transport.SetEmergencyAppLockRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: 7, NodeVersion: version, Lock: lock,
	}, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func useEmergencyAppLockDirs(t *testing.T) (string, *fakeAppUnitHost) {
	t.Helper()
	dir := t.TempDir()
	oldDir, oldBase := appUnitDirOverride, runtimesBaseDir
	appUnitDirOverride, runtimesBaseDir = dir, "/opt/celikpanel/runtimes"
	t.Cleanup(func() { appUnitDirOverride, runtimesBaseDir = oldDir, oldBase })
	host := &fakeAppUnitHost{present: true}
	useFakeAppUnitHost(t, host)
	return filepath.Join(dir, "celikapp-7.service.d", emergencyAppLockFile), host
}

func TestEmergencyAppLockIsWrittenAndRemovedExactly(t *testing.T) {
	path, host := useEmergencyAppLockDirs(t)
	r := setEmergencyAppLock(t, true, "22.11.0")
	if !r.Applied || !r.Changed || r.Path != path {
		t.Fatalf("lock = %+v", r)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != emergencyAppLockText("22.11.0") {
		t.Fatalf("lock file = %q, %v", data, err)
	}
	if !strings.Contains(string(data), "Restart=no") || !strings.Contains(string(data), ":/opt/celikpanel/runtimes/node/22.11.0/bin:") {
		t.Fatalf("lock text: %s", data)
	}
	if strings.Join(host.calls, ";") != "daemon-reload" {
		t.Fatalf("calls = %v", host.calls)
	}
	// Repeated: nothing changes.
	if r := setEmergencyAppLock(t, true, "22.11.0"); !r.Applied || r.Changed {
		t.Fatalf("second lock = %+v", r)
	}
	// Unlock removes the file and the directory it alone was in.
	if r := setEmergencyAppLock(t, false, "22.11.0"); !r.Applied || !r.Changed {
		t.Fatalf("unlock = %+v", r)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("drop-in directory left: %v", err)
	}
	if r := setEmergencyAppLock(t, false, "22.11.0"); !r.Applied || r.Changed {
		t.Fatalf("second unlock = %+v", r)
	}
}

func TestEmergencyAppLockKeepsOwnerFiles(t *testing.T) {
	path, host := useEmergencyAppLockDirs(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(filepath.Dir(path), "owner.conf")
	if err := os.WriteFile(owner, []byte("[Service]\nEnvironment=A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := setEmergencyAppLock(t, true, "22.11.0"); !r.Applied {
		t.Fatalf("lock = %+v", r)
	}
	// The owner edits the lock: neither a lock nor an unlock overwrites it.
	edited := emergencyAppLockText("22.11.0") + "# mine\n"
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	host.calls = nil
	for _, lock := range []bool{true, false} {
		r := setEmergencyAppLock(t, lock, "22.11.0")
		if r.Applied || !r.Conflict || !strings.Contains(r.Error, "changed on the server") {
			t.Fatalf("lock %v over an edited file = %+v", lock, r)
		}
	}
	// A lock for another version is not this one: it is kept too.
	if r := setEmergencyAppLock(t, false, "22.12.0"); !r.Conflict {
		t.Fatalf("unlock of another version = %+v", r)
	}
	if data, _ := os.ReadFile(path); string(data) != edited {
		t.Fatalf("edited lock was changed: %q", data)
	}
	if len(host.calls) != 0 {
		t.Fatalf("systemctl ran: %v", host.calls)
	}
	// The owner's own drop-in survives an unlock of ours.
	if err := os.WriteFile(path, []byte(emergencyAppLockText("22.11.0")), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := setEmergencyAppLock(t, false, "22.11.0"); !r.Applied || !r.Changed {
		t.Fatalf("unlock = %+v", r)
	}
	if _, err := os.Stat(owner); err != nil {
		t.Fatalf("owner's drop-in removed: %v", err)
	}
}

// A failed emergency move writes the unit back without enabling or
// starting it (release decision c).
func TestAppUnitKeepStoppedWritesWithoutStarting(t *testing.T) {
	host := newUnitTestHost(t)
	applyUnit(t, unitTestRequest(transport.SiteFileTriggerCreate))
	host.calls = nil
	request := unitTestRequest(transport.SiteFileTriggerChange)
	request.Port = 3002
	request.KeepStopped = true
	response := applyUnit(t, request)
	if response.Error != "" || response.File.Outcome != transport.SiteFileOutcomeWritten || !strings.Contains(unitText(t, host.path()), "PORT=3002") {
		t.Fatalf("keep stopped: %+v %+v", response, response.File)
	}
	calls := strings.Join(host.calls, "|")
	if strings.Contains(calls, "restart") || strings.Contains(calls, "enable") || strings.Contains(calls, "start celikapp") ||
		!strings.Contains(calls, "stop celikapp-7|disable celikapp-7") {
		t.Fatalf("systemctl calls = %v", host.calls)
	}
}

func TestEmergencyAppLockRefusesBadRequests(t *testing.T) {
	useEmergencyAppLockDirs(t)
	var resp transport.SetEmergencyAppLockResponse
	_ = (&Agent{}).SetEmergencyAppLockV1(&transport.SetEmergencyAppLockRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), SiteID: 7, NodeVersion: "22; rm", Lock: true,
	}, &resp)
	if resp.Applied || !strings.Contains(resp.Error, "nothing was changed") {
		t.Fatalf("bad version = %+v", resp)
	}
}

// The lock's start check, run by a real shell: it refuses the withdrawn
// version's PATH and lets another version start.
func TestEmergencyAppLockStartCheckRefusesOnlyTheWithdrawnVersion(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	old := runtimesBaseDir
	runtimesBaseDir = "/opt/celikpanel/runtimes"
	defer func() { runtimesBaseDir = old }()
	text := emergencyAppLockText("22.11.0")
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "ExecStartPre=/bin/sh -c '") {
			line = l
		}
	}
	if line == "" || strings.Contains(line, "%") {
		t.Fatalf("no usable ExecStartPre line: %s", text)
	}
	script := strings.TrimSuffix(strings.TrimPrefix(line, "ExecStartPre=/bin/sh -c '"), "'")
	script = strings.ReplaceAll(script, "$$", "$")
	run := func(path string) (int, string) {
		cmd := exec.Command("/bin/sh", "-c", script)
		cmd.Env = []string{"PATH=" + path}
		out, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), string(out)
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, string(out)
	}
	system := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	if code, out := run("/opt/celikpanel/runtimes/node/22.11.0/bin:" + system); code != 1 || !strings.Contains(out, "withdrawn for security") {
		t.Fatalf("withdrawn version started: %d %q", code, out)
	}
	for _, path := range []string{"/opt/celikpanel/runtimes/node/22.12.0/bin:" + system, "/opt/celikpanel/runtimes/node/22.11.01/bin:" + system, system} {
		if code, out := run(path); code != 0 {
			t.Fatalf("PATH %s refused: %d %q", path, code, out)
		}
	}
}
