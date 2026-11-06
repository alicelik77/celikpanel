package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Python apps (alpha.119, docs/RUNTIME-APPS.md "Python apps"). Root reads
// the customer's files only beneath the app folder without following a link
// (openat2), writes the checked requirements into its own folder under
// /run/celikpanel-rtbuild, and starts the build as a transient sandboxed
// unit of the site's account. Root never runs pip, Python or the customer's
// code; the build and the app run as the site's account in the sandbox.
//
// Python uygulamaları: root müşterinin dosyalarını bağlantı izlemeden okur,
// denetlenmiş gereksinimleri kendi klasörüne yazar ve derlemeyi sitenin
// hesabıyla korumalı geçici bir birimde başlatır. Root pip, Python ya da
// müşteri kodu çalıştırmaz.

// Seams (Linux: runtime_python_linux.go; tests replace them).
var (
	runtimePythonProbe      = probeRuntimePython
	runtimeAppSourceFile    = readRuntimeAppSourceFile
	runtimeAppDirNames      = listRuntimeAppDirNames
	runtimeAppVenvComplete  = runtimeAppVenvCompleteOnDisk
	runtimeAppBuildDir      = siteruntime.BuildDir
	runtimeAppBuildChown    = os.Chown
	runtimePythonReadlink   = os.Readlink
	runtimePythonFileExists = func(path string) bool { _, err := os.Stat(path); return err == nil }
)

// RuntimePythonV1 reports the server's distribution Python (read-only).
func (a *Agent) RuntimePythonV1(_ *transport.Empty, resp *transport.RuntimePythonResponse) error {
	*resp = runtimePythonProbe()
	return nil
}

func probeRuntimePython() transport.RuntimePythonResponse {
	profile, err := readHostProfile() // read-only: the cached platform (alpha.125)
	if err != nil {
		return transport.RuntimePythonResponse{Error: err.Error()}
	}
	return pythonFromHost(profile.ID, profile.Version, string(profile.PackageManager))
}

// pythonFromHost decides the distribution Python from the system's identity
// and what /usr/bin holds.
func pythonFromHost(osID, versionID, family string) transport.RuntimePythonResponse {
	resp := transport.RuntimePythonResponse{Family: family, Packages: siteruntime.PythonPackages(family), Reason: siteruntime.PythonRuntimeMissing}
	expected, rolling, ok := siteruntime.ExpectedPython(osID, versionID)
	if !ok {
		resp.Detail = "this system (" + osID + " " + versionID + ") is not one CelikPanel runs Python apps on"
		return resp
	}
	resp.Expected = expected
	minor := ""
	if target, err := runtimePythonReadlink("/usr/bin/python3"); err == nil {
		minor, _ = siteruntime.PythonMinorFromLink(target)
	} else if !rolling && runtimePythonFileExists(siteruntime.PythonBinary(expected)) {
		minor = expected
	}
	if minor == "" {
		resp.Detail = "Python 3 is not installed; the administrator installs Python in Components"
		return resp
	}
	resp.Minor = minor
	if rolling {
		resp.Expected = minor
	} else if minor != expected {
		resp.Detail = fmt.Sprintf("/usr/bin/python3 is Python %s, but this system's own Python is %s; CelikPanel uses only the distribution's Python", minor, expected)
		return resp
	}
	if !runtimePythonFileExists(siteruntime.PythonBinary(minor)) {
		resp.Detail = siteruntime.PythonBinary(minor) + " is missing; the administrator installs Python in Components"
		return resp
	}
	if !runtimePythonFileExists(siteruntime.PythonEnsurepip(minor)) {
		resp.Detail = "Python's venv module is not installed (" + strings.Join(resp.Packages, " ") + "); the administrator installs Python in Components"
		return resp
	}
	if !siteruntime.PinnedMinor(minor) {
		resp.Detail = "this CelikPanel release has no app-server pins for Python " + minor + "; a CelikPanel update adds them"
		return resp
	}
	resp.Ready, resp.Reason = true, ""
	return resp
}

// pythonSettings reads and checks a spec's Python settings.
func pythonSettings(spec transport.RuntimeAppSpec) (siteruntime.PythonSettings, error) {
	if spec.Python == nil {
		return siteruntime.PythonSettings{}, errors.New("the Python settings are missing")
	}
	s := siteruntime.PythonSettings{Server: spec.Python.Server, Callable: spec.Entry, Workers: spec.Python.Workers}
	limits := siteruntime.Limits{MemoryMB: spec.Limits.MemoryMB, CPUPercent: spec.Limits.CPUPercent, Tasks: spec.Limits.TasksMax}
	return s, s.Validate(limits)
}

// errSourceLink is a file of app/src that is a link.
var errSourceLink = errors.New("is a link")

// readRequirements checks app/src/requirements.txt and what it includes.
func readRequirements(rc runtimeAppContext) (siteruntime.Requirements, bool, error) {
	return siteruntime.CheckRequirements(func(rel string) ([]byte, error) {
		data, err := runtimeAppSourceFile(rc.layout.Root, "src/"+rel, 256<<10)
		if errors.Is(err, errSourceLink) {
			return nil, &siteruntime.ReqRefusal{File: rel, Code: siteruntime.ReqPath, Text: "the file is a link; upload the file itself"}
		}
		return data, err
	})
}

// runtimePythonSourceCheck checks app/src: a real folder of the site's
// account in a real app folder.
func runtimePythonSourceCheck(rc runtimeAppContext) transport.RuntimeAppEntryCheck {
	facts, err := runtimeAppEntryFacts(rc.layout.Root, "src")
	switch {
	case errors.Is(err, os.ErrNotExist):
		return transport.RuntimeAppEntryCheck{Code: siteruntime.PythonSrcMissing,
			Detail: "app/src is not on the server; upload the code and its requirements.txt with the file manager or SFTP"}
	case err != nil:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryUnreadable, Detail: err.Error()}
	case facts.FolderLink || facts.FolderUID != rc.account.UID:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryFolderNotOwned, Detail: "the app folder must be a real folder of the site's own account"}
	case facts.Link || !facts.Dir:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.PythonSrcMissing, Detail: "app/src must be a real folder, not a link or a file"}
	case facts.UID != rc.account.UID:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryFolderNotOwned, Detail: "app/src belongs to another account; upload it again with the site's file manager or SFTP"}
	}
	return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryOK, Format: "python"}
}

// runtimePythonCheck is the Python app's check before a build: the
// distribution Python, the settings, app/src and the requirements. A
// refused requirements line is requirements_refused with the line named.
func runtimePythonCheck(rc runtimeAppContext, spec transport.RuntimeAppSpec) (transport.RuntimeAppEntryCheck, *siteruntime.Requirements, bool) {
	py := runtimePythonProbe()
	if !py.Ready {
		detail := py.Detail
		if py.Error != "" {
			detail = py.Error
		}
		return transport.RuntimeAppEntryCheck{Code: siteruntime.PythonRuntimeMissing, Detail: detail}, nil, false
	}
	if _, err := pythonSettings(spec); err != nil {
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryInvalid, Detail: err.Error()}, nil, false
	}
	if check := runtimePythonSourceCheck(rc); check.Code != siteruntime.EntryOK {
		return check, nil, false
	}
	req, present, err := readRequirements(rc)
	var refusal *siteruntime.ReqRefusal
	switch {
	case errors.As(err, &refusal):
		return transport.RuntimeAppEntryCheck{Code: siteruntime.RequirementsRefused, Detail: refusal.Message()}, nil, false
	case err != nil:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryUnreadable, Detail: "requirements.txt could not be read: " + err.Error()}, nil, false
	}
	return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryOK, Format: "python"}, &req, present
}

// runtimePythonStatusCheck is the read-only check the screen shows: the
// distribution Python, app/src and, once a unit exists, its environment.
func runtimePythonStatusCheck(rc runtimeAppContext, appID int) transport.RuntimeAppEntryCheck {
	py := runtimePythonProbe()
	if !py.Ready {
		return transport.RuntimeAppEntryCheck{Code: siteruntime.PythonRuntimeMissing, Detail: py.Detail}
	}
	if check := runtimePythonSourceCheck(rc); check.Code != siteruntime.EntryOK {
		return check
	}
	if active := runtimeAppActiveVenv(appID); active > 0 && !runtimeAppVenvComplete(rc.layout.Root, siteruntime.VenvName(active)) {
		return transport.RuntimeAppEntryCheck{Code: siteruntime.PythonVenvMissing,
			Detail: "app/" + siteruntime.VenvName(active) + " that the app runs is gone; deploy again to build it"}
	}
	return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryOK, Format: "python"}
}

// runtimeAppActiveVenv is the environment generation the unit file names.
func runtimeAppActiveVenv(appID int) int {
	path, err := runtimeAppUnitPath(appID)
	if err != nil {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return siteruntime.ActiveVenv(string(raw))
}

// runtimeBuildRecord is the Agent's record of the last build of an app
// (runtime-apps/<id>/build.json, root's): which operation, which generation,
// the generation that ran before it, and how it ended.
type runtimeBuildRecord struct {
	OperationID string   `json:"operation_id"`
	Venv        int      `json:"venv"`
	Previous    int      `json:"previous"`
	Phase       string   `json:"phase"`
	Class       string   `json:"class,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	Package     string   `json:"package,omitempty"`
	Lines       []string `json:"lines,omitempty"`
	StartedAt   string   `json:"started_at"`
	EndedAt     string   `json:"ended_at,omitempty"`
}

func readRuntimeBuildRecord(appID int) runtimeBuildRecord {
	var r runtimeBuildRecord
	if raw, err := os.ReadFile(filepath.Join(runtimeAppStateDir(appID), "build.json")); err == nil {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

func writeRuntimeBuildRecord(appID int, r runtimeBuildRecord) error {
	dir := runtimeAppStateDir(appID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	p := filepath.Join(dir, "build.json")
	if err := os.WriteFile(p+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

func (r runtimeBuildRecord) answer(resp *transport.RuntimeAppBuildResponse) {
	resp.Phase, resp.Venv, resp.Previous, resp.Package, resp.Lines = r.Phase, r.Venv, r.Previous, r.Package, r.Lines
	if r.Phase == siteruntime.BuildPhaseFailed {
		resp.Done = false
		resp.Class, resp.Reason, resp.Error = r.Class, r.Reason, r.Detail
		return
	}
	resp.Done = true
}

// writeRuntimeAppBuildFiles writes the build's input files root's, readable
// by the site's group only: <BuildDir> (0711) / <app id> (0750 root:group) /
// file (0640 root:group). Nothing the site's account controls is on the path.
func writeRuntimeAppBuildFiles(appID, gid int, files map[string]string) error {
	if err := os.MkdirAll(runtimeAppBuildDir, 0o711); err != nil {
		return err
	}
	if err := os.Chmod(runtimeAppBuildDir, 0o711); err != nil {
		return err
	}
	dir := filepath.Join(runtimeAppBuildDir, strconv.Itoa(appID))
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o750); err != nil {
		return err
	}
	if err := runtimeAppBuildChown(dir, 0, gid); err != nil {
		return err
	}
	for name, text := range files {
		p := filepath.Join(dir, name)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
		if err != nil {
			return err
		}
		_, werr := f.WriteString(text)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
		if err := runtimeAppBuildChown(p, 0, gid); err != nil {
			return err
		}
	}
	return nil
}

func removeRuntimeAppBuildFiles(appID int) {
	_ = os.RemoveAll(filepath.Join(runtimeAppBuildDir, strconv.Itoa(appID)))
}

// buildUnitShow reads the build unit (found=false: systemd does not know it).
func buildUnitShow(appID int) (props map[string]string, found bool, err error) {
	out, err := appUnitSystemctl(systemctlArgs("show", siteruntime.BuildUnitName(appID), "--property=LoadState,ActiveState,Result,InvocationID,Job")...)
	if err != nil {
		return nil, false, fmt.Errorf("systemd could not be read: %s", strings.TrimSpace(string(out)))
	}
	props = parseShow(string(out))
	return props, props["LoadState"] != "" && props["LoadState"] != "not-found", nil
}

// stopRuntimeAppBuildUnit stops and unloads an app's build unit and its
// bind unit (clearTransientUnit).
func stopRuntimeAppBuildUnit(appID int) error {
	return clearTransientUnit(siteruntime.BuildUnitName(appID))
}

// RuntimeAppBuildV1 starts the build of a Python app's next environment for
// one deploy operation (see siteruntime.BuildRunArgs). The running app is not
// touched. A repeat for the same operation answers its record.
func (a *Agent) RuntimeAppBuildV1(req *transport.RuntimeAppBuildRequest, resp *transport.RuntimeAppBuildResponse) error {
	step := &resp.RuntimeAppStepResponse
	if req == nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "building a Python app"); err != nil {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if !runtimeAppOperation.MatchString(req.OperationID) {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "an operation identity is required")
		return nil
	}
	unlock := lockRuntimeApp(req.Target.AppID)
	defer unlock()
	rc, ok := resolveRuntimeApp(req.Target, step)
	if !ok {
		return nil
	}
	if rc.kind != siteruntime.KindPython {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "only a Python app has a build")
		return nil
	}
	if record := readRuntimeBuildRecord(req.Target.AppID); record.OperationID == req.OperationID {
		record.answer(resp)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	caps := sandboxCaps(ctx)
	if _, refusal := runtimeAppLevel(caps, req.Spec); refusal != "" {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "sandbox_reduced", refusal)
		return nil
	}
	check, requirements, present := runtimePythonCheck(rc, req.Spec)
	if check.Code != siteruntime.EntryOK {
		class := transport.RuntimeAppPrerequisite
		if check.Code == siteruntime.EntryUnreadable {
			class = transport.RuntimeAppUnknown
		}
		runtimeStep(step, class, check.Code, check.Detail)
		return nil
	}
	settingsSpec := req.Spec.Python
	py := runtimePythonProbe()
	pins, err := siteruntime.AppServerRequirements(py.Minor)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppPrerequisite, siteruntime.PythonRuntimeMissing, err.Error())
		return nil
	}
	dirs, err := runtimeAppDirNames(rc.layout.Root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		runtimeStep(step, transport.RuntimeAppUnknown, "site_folder", "the app folder could not be read: "+err.Error())
		return nil
	}
	active := runtimeAppActiveVenv(req.Target.AppID)
	set := siteruntime.NewVenvSet(dirs, active)
	next, err := set.Next()
	if err != nil {
		runtimeStep(step, transport.RuntimeAppPrerequisite, siteruntime.BuildFailed, err.Error())
		return nil
	}
	// A build of an operation that no longer runs (the Panel restarted
	// and failed it) is stopped first; one build per app. Its unit must be
	// gone before systemd-run makes the new one ("already loaded").
	if err := stopRuntimeAppBuildUnit(req.Target.AppID); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, siteruntime.BuildFailed, err.Error())
		return nil
	}
	for _, stale := range set.Stale() {
		if _, err := runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", path.Join(rc.layout.Root, stale)); err != nil {
			runtimeStep(step, transport.RuntimeAppVerifiedFailure, "site_folder", "a leftover build folder could not be removed as the site's account: "+err.Error())
			return nil
		}
	}
	if _, err := runtimeAppAsUser(rc.account, "/bin/mkdir", "-p", "--", rc.layout.DataDir(), rc.layout.ConfigDir(),
		rc.layout.PipCacheDir(), rc.layout.BuildTmpDir()); err != nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "site_folder", "the app's folders could not be made as the site's account: "+err.Error())
		return nil
	}
	files := map[string]string{siteruntime.BuildServerFile: pins}
	params := siteruntime.BuildParams{
		AppID: req.Target.AppID, Domain: req.Target.Domain, User: rc.account.Name, Group: rc.account.Group, Layout: rc.layout,
		Minor: py.Minor, Venv: next, Limits: siteruntime.Limits{MemoryMB: req.Spec.Limits.MemoryMB, CPUPercent: req.Spec.Limits.CPUPercent, Tasks: req.Spec.Limits.TasksMax},
		Level: sitesandbox.LevelFull, Caps: caps, SourceBuilds: settingsSpec.SourceBuilds,
	}
	if req.Spec.Level == string(sitesandbox.LevelReduced) {
		params.Level = sitesandbox.LevelReduced
	}
	if present && requirements != nil && requirements.Text != "" {
		files[siteruntime.BuildRequirementFile] = requirements.Text
		params.HasRequirements, params.Hashed = true, requirements.Hashed
		if requirements.Constraints != "" {
			files[siteruntime.BuildConstraintFile] = requirements.Constraints
			params.HasConstraints = true
		}
	}
	args, err := siteruntime.BuildRunArgs(params)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "unit_render", err.Error())
		return nil
	}
	if err := writeRuntimeAppBuildFiles(req.Target.AppID, rc.account.GID, files); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "build_files", err.Error())
		return nil
	}
	// The build unit requires its bind unit like the app's own unit: the
	// helper and its template are put in place, and the folder is checked
	// as the helper will check it.
	if err := ensureSandboxHelper(); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "sandbox_helper", err.Error())
		return nil
	}
	if err := ensureSandboxEgress(caps); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "sandbox_egress", err.Error())
		return nil
	}
	if code, detail := sandboxFolderRefusal(rc.layout.Root, rc.account.UID); code != "" {
		class := transport.RuntimeAppPrerequisite
		if code == sandboxbind.CodeUnreadable {
			class = transport.RuntimeAppUnknown
		}
		runtimeStep(step, class, code, detail)
		return nil
	}
	record := runtimeBuildRecord{OperationID: req.OperationID, Venv: next, Previous: active, Phase: siteruntime.BuildPhaseRunning,
		StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeRuntimeBuildRecord(req.Target.AppID, record); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "build_record", err.Error())
		return nil
	}
	if out, err := serviceAppRun(ctx, "systemd-run", args...); err != nil {
		record.Phase, record.Class, record.Reason = siteruntime.BuildPhaseFailed, transport.RuntimeAppVerifiedFailure, siteruntime.BuildFailed
		record.Detail = "the build could not be started: " + siteruntime.RedactBuildLine(strings.TrimSpace(string(out)))
		record.EndedAt = time.Now().UTC().Format(time.RFC3339)
		_ = writeRuntimeBuildRecord(req.Target.AppID, record)
		removeRuntimeAppBuildFiles(req.Target.AppID)
		record.answer(resp)
		return nil
	}
	record.answer(resp)
	return nil
}

// buildJournal reads the build's own output (its last invocation).
func buildJournal(ctx context.Context, appID int, invocation string) []string {
	args := []string{"-u", siteruntime.BuildUnitName(appID), "-n", "200", "--no-pager", "-o", "cat"}
	if invocation != "" && runtimeInvocationPattern(invocation) {
		args = []string{"_SYSTEMD_INVOCATION_ID=" + invocation, "-n", "200", "--no-pager", "-o", "cat"}
	}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	out, err := serviceAppRun(ctx, "journalctl", args...)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

func runtimeInvocationPattern(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// RuntimeAppBuildAdvanceV1 reads the build of an operation and completes it
// when the unit has finished (see the transport type). Running: nothing
// changes. Repeating a completed one answers the record.
func (a *Agent) RuntimeAppBuildAdvanceV1(req *transport.RuntimeAppBuildAdvanceRequest, resp *transport.RuntimeAppBuildResponse) error {
	step := &resp.RuntimeAppStepResponse
	if req == nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "completing a Python app's build"); err != nil {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.Target.AppID <= 0 || !runtimeAppOperation.MatchString(req.OperationID) {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "an app and an operation are required")
		return nil
	}
	unlock := lockRuntimeApp(req.Target.AppID)
	defer unlock()
	record := readRuntimeBuildRecord(req.Target.AppID)
	if record.OperationID != req.OperationID {
		runtimeStep(step, transport.RuntimeAppUnknown, siteruntime.BuildFailed, "the server has no build of this deploy; deploy again")
		return nil
	}
	if record.Phase != siteruntime.BuildPhaseRunning {
		record.answer(resp)
		return nil
	}
	props, found, err := buildUnitShow(req.Target.AppID)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "systemctl", err.Error())
		return nil
	}
	phase, why := siteruntime.BuildOutcome(siteruntime.BuildUnit{Found: found, ActiveState: props["ActiveState"], Result: props["Result"],
		Job: props["Job"], InvocationID: props["InvocationID"]})
	if phase == siteruntime.BuildPhaseRunning {
		record.answer(resp)
		return nil
	}
	rc, ok := resolveRuntimeApp(req.Target, step)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	partial := path.Join(rc.layout.Root, siteruntime.PartialVenvName(record.Venv))
	finished := path.Join(rc.layout.Root, siteruntime.VenvName(record.Venv))
	end := func() {
		record.EndedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writeRuntimeBuildRecord(req.Target.AppID, record); err != nil {
			log.Printf("RuntimeAppBuildAdvanceV1 app %d: record: %v", req.Target.AppID, err)
		}
		if err := stopRuntimeAppBuildUnit(req.Target.AppID); err != nil {
			log.Printf("RuntimeAppBuildAdvanceV1 app %d: %v", req.Target.AppID, err)
		}
		removeRuntimeAppBuildFiles(req.Target.AppID)
		record.answer(resp)
	}
	if phase == siteruntime.BuildPhaseSucceeded {
		renamed := runtimeAppVenvComplete(rc.layout.Root, siteruntime.VenvName(record.Venv))
		if !renamed {
			if !runtimeAppVenvComplete(rc.layout.Root, siteruntime.PartialVenvName(record.Venv)) {
				record.Phase, record.Class, record.Reason = siteruntime.BuildPhaseFailed, transport.RuntimeAppVerifiedFailure, siteruntime.BuildFailed
				record.Detail = "the build ended, but app/" + siteruntime.PartialVenvName(record.Venv) + " is not a complete environment (it was changed during the build); deploy again"
				_, _ = runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", partial)
				end()
				return nil
			}
			// One rename: the environment either is complete under its
			// final name or is not there.
			if _, err := runtimeAppAsUser(rc.account, "/bin/mv", "-T", "--", partial, finished); err != nil {
				runtimeStep(step, transport.RuntimeAppUnknown, "site_folder", "the finished environment could not be moved into place: "+err.Error())
				return nil
			}
		}
		dirs, _ := runtimeAppDirNames(rc.layout.Root)
		set := siteruntime.NewVenvSet(dirs, runtimeAppActiveVenv(req.Target.AppID))
		for _, old := range set.AfterBuild(record.Venv) {
			if _, err := runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", path.Join(rc.layout.Root, old)); err != nil {
				log.Printf("RuntimeAppBuildAdvanceV1 app %d: remove %s: %v", req.Target.AppID, old, err)
			}
		}
		_, _ = runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", rc.layout.BuildTmpDir())
		record.Phase = siteruntime.BuildPhaseSucceeded
		end()
		return nil
	}
	if why == siteruntime.BuildNotStarted {
		// The build never ran: its bind unit (the sandbox helper) did not
		// start. The helper's journal says why (a refused app folder).
		record.Phase = siteruntime.BuildPhaseFailed
		record.Class, record.Reason, record.Detail, record.Lines = buildBindFailure(ctx, req.Target.AppID)
		end()
		return nil
	}
	lines := buildJournal(ctx, req.Target.AppID, props["InvocationID"])
	reason, pkg := siteruntime.ClassifyBuildFailure(lines)
	record.Phase, record.Reason, record.Package = siteruntime.BuildPhaseFailed, reason, pkg
	record.Lines = siteruntime.BuildTail(lines, 25, 1400)
	switch {
	case why == siteruntime.BuildLost:
		record.Class, record.Reason = transport.RuntimeAppUnknown, siteruntime.BuildFailed
		record.Detail = "the build stopped without a result (the server restarted or someone stopped it); the running app was not changed. Deploy again"
	case reason == siteruntime.WheelUnavailable:
		record.Class = transport.RuntimeAppVerifiedFailure
		record.Detail = pkg + " has no ready-made package (wheel) for Python " + runtimePythonProbe().Minor + " on PyPI, or no such package exists; the running app was not changed"
	case reason == siteruntime.PyPIUnreachable:
		record.Class = transport.RuntimeAppUnknown
		record.Detail = "the build could not reach pypi.org; the running app was not changed"
	default:
		record.Class = transport.RuntimeAppVerifiedFailure
		record.Detail = "pip stopped with an error (" + props["Result"] + "); the running app was not changed"
	}
	if len(record.Lines) > 0 {
		record.Detail += ". pip: " + strings.Join(record.Lines, " | ")
		if len(record.Detail) > 1800 {
			record.Detail = record.Detail[:1800] + "…"
		}
	}
	if _, err := runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", partial); err != nil {
		log.Printf("RuntimeAppBuildAdvanceV1 app %d: remove the partial environment: %v", req.Target.AppID, err)
	}
	end()
	return nil
}
