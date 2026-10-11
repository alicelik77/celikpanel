package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// D-031 second step (2026-10-11): the application unit file.

type unitTestHost struct {
	dir   string
	calls []string
	// active is the unit's `systemctl is-active`; verify the answer of
	// systemd-analyze verify (counted in verified); failDaemonReload makes
	// the first daemon-reload fail.
	active           string
	verify           error
	verified         int
	failDaemonReload bool
}

func newUnitTestHost(t *testing.T) *unitTestHost {
	t.Helper()
	host := &unitTestHost{dir: t.TempDir()}
	oldDir, oldSystemctl, oldExists := appUnitDirOverride, appUnitSystemctl, appUnitAccountExists
	appUnitDirOverride = host.dir
	appUnitSystemctl = func(args ...string) ([]byte, error) {
		host.calls = append(host.calls, strings.Join(args, " "))
		if len(args) > 0 && args[0] == "show" {
			return []byte("LoadState=not-found\nActiveState=inactive\n"), nil
		}
		if len(args) > 0 && args[0] == "daemon-reload" && host.failDaemonReload {
			host.failDaemonReload = false
			return []byte("Failed to reload daemon"), errors.New("exit status 1")
		}
		return nil, nil
	}
	appUnitAccountExists = func(name string) bool { return name == "www-data" }
	oldActive, oldVerify := appUnitActive, appUnitVerify
	appUnitActive = func(string) string { return host.active }
	appUnitVerify = func(string) error { host.verified++; return host.verify }
	host.active = transport.SiteFileServiceActiveTrue
	t.Cleanup(func() {
		appUnitDirOverride, appUnitSystemctl, appUnitAccountExists = oldDir, oldSystemctl, oldExists
		appUnitActive, appUnitVerify = oldActive, oldVerify
	})
	return host
}

func (host *unitTestHost) path() string { return filepath.Join(host.dir, "celikapp-7.service") }

func unitTestRequest(trigger string) *AppApplyRequest {
	return &AppApplyRequest{SiteID: 7, Description: "example.com", WorkDir: "/srv/example", Command: "node app.js",
		Port: 3001, RunAsUser: "www-data", Trigger: trigger}
}

func applyUnit(t *testing.T, request *AppApplyRequest) AppApplyResponse {
	t.Helper()
	var response AppApplyResponse
	if err := (&Agent{}).ApplyAppUnit(request, &response); err != nil {
		t.Fatal(err)
	}
	if response.File == nil {
		t.Fatalf("no file result: %+v", response)
	}
	return response
}

func unitText(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestAppUnitIsSealedAndWrittenThroughARename(t *testing.T) {
	host := newUnitTestHost(t)
	created := applyUnit(t, unitTestRequest(transport.SiteFileTriggerCreate))
	if created.Error != "" || created.File.Outcome != transport.SiteFileOutcomeWritten || created.File.Kind != transport.SiteFileKindAppUnit {
		t.Fatalf("create: %+v %+v", created, created.File)
	}
	content := unitText(t, host.path())
	if !strings.HasPrefix(content, "# celikpanel-render v2 sha256=") || !strings.Contains(content, "celikapp-7.service.d/*.conf") {
		t.Fatalf("unit:\n%s", content)
	}
	if strings.Join(host.calls, "|") != "daemon-reload|enable celikapp-7|restart celikapp-7" {
		t.Fatalf("systemctl calls = %v", host.calls)
	}
	if info, _ := os.Stat(host.path()); info.Mode().Perm() != 0o644 {
		t.Fatalf("new unit mode = %o", info.Mode().Perm())
	}

	// Identical text: nothing written, systemd not reloaded.
	host.calls = nil
	same := applyUnit(t, unitTestRequest(""))
	if same.File.Outcome != transport.SiteFileOutcomeUnchanged || strings.Contains(strings.Join(host.calls, "|"), "daemon-reload") {
		t.Fatalf("identical: %+v calls=%v", same.File, host.calls)
	}

	// A change replaces the file by a rename (a hard link to the old file
	// keeps the old bytes) and keeps the file's mode.
	link := filepath.Join(host.dir, "old-link")
	if err := os.Link(host.path(), link); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(host.path(), 0o600); err != nil {
		t.Fatal(err)
	}
	before := unitText(t, link)
	changed := unitTestRequest("")
	changed.Port = 3002
	response := applyUnit(t, changed)
	if response.File.Outcome != transport.SiteFileOutcomeWritten || !strings.Contains(unitText(t, host.path()), "PORT=3002") {
		t.Fatalf("change: %+v", response.File)
	}
	if unitText(t, link) != before {
		t.Fatal("the unit was written in place, not replaced by a rename")
	}
	if info, _ := os.Stat(host.path()); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want the file's 0600", info.Mode().Perm())
	}
}

func TestAppUnitOwnerEditIsKeptAndTheDropInIsNeverTouched(t *testing.T) {
	host := newUnitTestHost(t)
	applyUnit(t, unitTestRequest(transport.SiteFileTriggerCreate))
	dropIn := filepath.Join(host.dir, "celikapp-7.service.d")
	if err := os.MkdirAll(dropIn, 0o755); err != nil {
		t.Fatal(err)
	}
	ownerSetting := filepath.Join(dropIn, "memory.conf")
	if err := os.WriteFile(ownerSetting, []byte("[Service]\nMemoryMax=512M\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	edited := unitText(t, host.path()) + "Environment=FEATURE=1\n"
	if err := os.WriteFile(host.path(), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	host.calls = nil
	changed := unitTestRequest("")
	changed.Command = "node server.js"
	kept := applyUnit(t, changed)
	if kept.File.Outcome != transport.SiteFileOutcomeKept || kept.File.State != transport.SiteFileOwnerEdited || kept.Error == "" {
		t.Fatalf("change on an edited unit: %+v %q", kept.File, kept.Error)
	}
	if unitText(t, host.path()) != edited || len(host.calls) != 0 {
		t.Fatalf("edited unit changed or systemd called: %v", host.calls)
	}
	if !strings.Contains(unitText(t, host.path()+services.SiteFilePendingSuffix), "node server.js") {
		t.Fatal("CelikPanel's text is not held beside the unit")
	}

	var inspected transport.InspectSiteFileResponse
	if err := (&Agent{}).InspectAppUnit(changed, &inspected); err != nil || inspected.File.State != transport.SiteFileOwnerEdited {
		t.Fatalf("inspect: %+v %v", inspected.File, err)
	}
	if !strings.Contains(inspected.Diff, "-Environment=FEATURE=1") || inspected.File.IncludeDir != dropIn {
		t.Fatalf("diff/include dir: %q %q", inspected.Diff, inspected.File.IncludeDir)
	}

	take := unitTestRequest(transport.SiteFileTriggerTake)
	take.Command = "node server.js"
	take.ExpectedFileSHA256, take.ExpectedRenderSHA256 = inspected.File.FileSHA256, inspected.File.RenderSHA256
	host.calls = nil
	taken := applyUnit(t, take)
	if taken.File.Outcome != transport.SiteFileOutcomeTaken || taken.File.BackupPath == "" || taken.Error != "" {
		t.Fatalf("take: %+v %q", taken.File, taken.Error)
	}
	if strings.Join(host.calls, "|") != "daemon-reload|try-restart celikapp-7" {
		t.Fatalf("take systemctl calls = %v", host.calls)
	}
	if unitText(t, taken.File.BackupPath) != edited {
		t.Fatal("dated copy is not the owner's unit")
	}
	if unitText(t, ownerSetting) != "[Service]\nMemoryMax=512M\n" {
		t.Fatal("the drop-in was touched")
	}

	// Removing the unit (the site is no longer a Node site) keeps a copy of
	// an edited unit and leaves the drop-in directory.
	if err := os.WriteFile(host.path(), []byte(unitText(t, host.path())+"Nice=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var removed AppApplyResponse
	if err := (&Agent{}).RemoveAppUnit(&AppControlRequest{SiteID: 7}, &removed); err != nil || removed.Error != "" {
		t.Fatalf("remove: %+v %v", removed, err)
	}
	if _, err := os.Lstat(host.path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unit remains: %v", err)
	}
	copies, _ := filepath.Glob(host.path() + services.SiteFileBackupMarker + "*")
	if len(copies) != 2 {
		t.Fatalf("copies = %v, want the take's and the removal's", copies)
	}
	if unitText(t, ownerSetting) != "[Service]\nMemoryMax=512M\n" {
		t.Fatal("removal touched the drop-in")
	}
}

func TestAppUnitSymlinkAndMissingFileAreNotWritten(t *testing.T) {
	host := newUnitTestHost(t)
	missing := applyUnit(t, unitTestRequest(""))
	if missing.File.Outcome != transport.SiteFileOutcomeMissing || missing.Error == "" {
		t.Fatalf("change on an absent unit: %+v", missing.File)
	}
	if _, err := os.Lstat(host.path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("an absent unit was recreated by a change")
	}
	target := filepath.Join(t.TempDir(), "elsewhere.service")
	if err := os.WriteFile(target, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, host.path()); err != nil {
		t.Fatal(err)
	}
	refused := applyUnit(t, unitTestRequest(transport.SiteFileTriggerCreate))
	if refused.File.Outcome != transport.SiteFileOutcomeRefused || refused.File.Reason != transport.SiteFileReasonSymlink {
		t.Fatalf("link: %+v", refused.File)
	}
	if unitText(t, target) != "[Unit]\n" {
		t.Fatal("the link's target was written")
	}
	recreated := applyUnit(t, func() *AppApplyRequest {
		_ = os.Remove(host.path())
		return unitTestRequest(transport.SiteFileTriggerRecreate)
	}())
	if recreated.File.Outcome != transport.SiteFileOutcomeRecreated {
		t.Fatalf("recreate: %+v", recreated.File)
	}
}

func TestAppUnitLegacyTextIsAdopted(t *testing.T) {
	host := newUnitTestHost(t)
	request := unitTestRequest("")
	renders, _ := appUnitFile(request, host.dir).Legacy()
	// On this host www-data exists: alpha.81 and alpha.82 wrote the same text,
	// and the newest release is named.
	if renders[0].Text != renders[1].Text || renders[0].Release != "v0.1.0-alpha.82" {
		t.Fatalf("renders = %+v", renders)
	}
	inspectState := func() transport.SiteFileResult {
		var inspected transport.InspectSiteFileResponse
		if err := (&Agent{}).InspectAppUnit(request, &inspected); err != nil {
			t.Fatal(err)
		}
		return inspected.File
	}
	if err := os.WriteFile(host.path(), []byte(renders[1].Text), 0o644); err != nil {
		t.Fatal(err)
	}
	if file := inspectState(); file.State != transport.SiteFileManagedUnchanged || file.AdoptedFrom != "v0.1.0-alpha.82" {
		t.Fatalf("same text: %+v", file)
	}
	// Where the web server's account is http (Arch), alpha.81 wrote
	// User=www-data as asked and alpha.82 User=http: each is named.
	appUnitAccountExists = func(name string) bool { return name == "http" }
	arch, _ := appUnitFile(request, host.dir).Legacy()
	for _, render := range arch {
		if err := os.WriteFile(host.path(), []byte(render.Text), 0o644); err != nil {
			t.Fatal(err)
		}
		if file := inspectState(); file.State != transport.SiteFileManagedUnchanged || file.AdoptedFrom != render.Release {
			t.Fatalf("%s: %+v", render.Release, file)
		}
	}
	appUnitAccountExists = func(name string) bool { return name == "www-data" }
	if err := os.WriteFile(host.path(), []byte(renders[0].Text), 0o644); err != nil {
		t.Fatal(err)
	}
	written := applyUnit(t, request)
	if written.File.Outcome != transport.SiteFileOutcomeWritten || written.File.AdoptedFrom == "" {
		t.Fatalf("adopted unit's next change: %+v", written.File)
	}
	if !strings.HasPrefix(unitText(t, host.path()), "# celikpanel-render v2 sha256=") {
		t.Fatal("adopted unit not sealed")
	}
	// A headerless unit with a line no release wrote is unknown origin.
	if err := os.WriteFile(host.path(), []byte(renders[0].Text+"Nice=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var inspected transport.InspectSiteFileResponse
	_ = (&Agent{}).InspectAppUnit(request, &inspected)
	if inspected.File.State != transport.SiteFileUnknownOrigin {
		t.Fatalf("headerless edited unit: %+v", inspected.File)
	}
}

// The unit's start command does not depend on a login shell's PATH (set12
// observation, Debian 13: a bare `node` exited 127 under `/bin/sh -lc`): the
// PATH line names the Panel-installed Node directory and the system
// directories, and the command runs under `/bin/sh -c`. The texts alpha.82 and
// alpha.81 wrote stay as they were written.
func TestAppUnitPathDoesNotDependOnALoginShell(t *testing.T) {
	host := newUnitTestHost(t)
	request := unitTestRequest(transport.SiteFileTriggerCreate)
	request.NodeVersion = "22.20.0"
	file := appUnitFile(request, host.dir)
	nodeDir := filepath.Dir(nodeBinPath("22.20.0"))
	wantPath := "Environment=PATH=" + nodeDir + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n"
	if !strings.Contains(file.Body, wantPath) || !strings.Contains(file.Body, "ExecStart=/bin/sh -c 'node app.js'\n") ||
		strings.Contains(file.Body, "-lc") {
		t.Fatalf("unit body:\n%s", file.Body)
	}
	legacy, _ := file.Legacy()
	for _, render := range legacy {
		if !strings.Contains(render.Text, "ExecStart=/bin/sh -lc 'node app.js'\n") ||
			!strings.Contains(render.Text, "Environment=PATH="+nodeDir+":"+os.Getenv("PATH")+"\n") {
			t.Fatalf("%s legacy text changed:\n%s", render.Release, render.Text)
		}
	}
}

// After a take that systemd did not take (its daemon-reload failed), the
// owner's unit is put back, systemd reads it again (daemon-reload is
// harmless), and the running application is restarted only when
// systemd-analyze verify accepts the put-back unit (set12 cell RP1's rule).
func TestAppUnitRestoreRestartsOnlyAVerifiedUnit(t *testing.T) {
	for _, c := range []struct {
		name         string
		active       string
		verify       error
		wantService  string
		wantReason   string
		wantCalls    string
		wantVerified int
	}{
		{"verified", transport.SiteFileServiceActiveTrue, nil, transport.SiteFileServiceReloaded, "",
			"daemon-reload|daemon-reload|try-restart celikapp-7", 1},
		{"refused", transport.SiteFileServiceActiveTrue, errors.New("systemd-analyze verify: celikapp-7.service: Unknown key name 'Bogus' in section 'Service'"),
			transport.SiteFileServiceNotReloaded, transport.SiteFileServiceReasonConfigRefusedElsewhere, "daemon-reload|daemon-reload", 1},
		{"no systemd-analyze", transport.SiteFileServiceActiveTrue, services.ErrServiceCheckUnavailable,
			transport.SiteFileServiceNotReloaded, transport.SiteFileServiceReasonCheckUnavailable, "daemon-reload|daemon-reload", 1},
		{"not running before", transport.SiteFileServiceActiveFalse, nil, transport.SiteFileServiceNotRunning, "",
			"daemon-reload|daemon-reload", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			host := newUnitTestHost(t)
			applyUnit(t, unitTestRequest(transport.SiteFileTriggerCreate))
			edited := unitText(t, host.path()) + "Environment=FEATURE=1\n"
			if err := os.WriteFile(host.path(), []byte(edited), 0o644); err != nil {
				t.Fatal(err)
			}
			var inspected transport.InspectSiteFileResponse
			if err := (&Agent{}).InspectAppUnit(unitTestRequest(""), &inspected); err != nil {
				t.Fatal(err)
			}
			host.active, host.verify, host.failDaemonReload, host.calls = c.active, c.verify, true, nil
			take := unitTestRequest(transport.SiteFileTriggerTake)
			take.ExpectedFileSHA256, take.ExpectedRenderSHA256 = inspected.File.FileSHA256, inspected.File.RenderSHA256
			response := applyUnit(t, take)
			file := response.File
			if file.Outcome != transport.SiteFileOutcomeFailed || file.Restore != transport.SiteFileRestoreConfirmed ||
				file.Service != c.wantService || file.ServiceReason != c.wantReason || file.ServiceActive != c.active {
				t.Fatalf("result: %+v %q", file, response.Error)
			}
			if strings.Join(host.calls, "|") != c.wantCalls || host.verified != c.wantVerified {
				t.Fatalf("systemctl calls = %v, verified %d", host.calls, host.verified)
			}
			if unitText(t, host.path()) != edited {
				t.Fatal("the owner's unit is not back")
			}
			if c.wantReason == transport.SiteFileServiceReasonConfigRefusedElsewhere &&
				(!strings.Contains(file.ServiceCheck, "Unknown key name") || !strings.Contains(response.Error, "not restarted")) {
				t.Fatalf("check line / error: %q %q", file.ServiceCheck, response.Error)
			}
			if strings.Contains(response.Error, "rollback incomplete") {
				t.Fatalf("error: %q", response.Error)
			}
		})
	}
}
