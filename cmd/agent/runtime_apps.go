package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sandboxbind"
	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Runtime apps (alpha.119, docs/RUNTIME-APPS.md): a customer's own program,
// this release an uploaded executable, run as celikrt-<app id>.service, the
// site's own account, in the site sandbox, on one loopback port, reached only
// through the site's nginx over HTTPS.
//
// Root does only what needs root and never runs the customer's program or a
// shell:
//   - the entry is read without following any link (openat2 RESOLVE_BENEATH
//     | RESOLVE_NO_SYMLINKS), never executed;
//   - folders in the app tree are made, and the tree removed, by the site's
//     own account (mkdir and rm executed directly, without a shell);
//   - the environment file is root's (/etc/celikpanel-runtime, 0600):
//     systemd reads it as root before the sandbox starts, so a file in the
//     site's folder could have been a link to a root-only file;
//   - the unit follows the D-031 render rule: an owner's change is kept, the
//     owner's drop-ins are read and shown, and the proof refuses to publish a
//     unit whose loaded sandbox differs from its profile.
//
// Çalışma ortamı uygulamaları: root yalnız root gerektiren işi yapar; müşterinin
// programını ya da bir kabuğu asla çalıştırmaz. Ortam dosyası root'undur.

var (
	runtimeAppStateRoot = filepath.Join(hostingpath.ServiceMutationStateRoot(), "runtime-apps")
	runtimeAppEnvDir    = siteruntime.EnvDir
	runtimeAppOperation = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,63}$`)
	runtimeAppLocks     [64]sync.Mutex
	// runtimeAppHostArch is this server's ELF machine.
	runtimeAppHostArch = siteruntime.MachineFor(runtime.GOARCH)
	// runtimeAppCgroupRoot is where a unit's cgroup files are read.
	runtimeAppCgroupRoot = "/sys/fs/cgroup/system.slice"
)

// Seams (Linux: runtime_apps_linux.go; tests replace them).
var (
	runtimeAppEntryFacts = readRuntimeAppEntryFacts
	runtimeAppAsUser     = runRuntimeAppAsUser
	runtimeAppHealthGet  = runtimeAppHTTPHealth
)

func lockRuntimeApp(appID int) func() {
	lock := &runtimeAppLocks[uint(appID)%uint(len(runtimeAppLocks))]
	lock.Lock()
	return lock.Unlock
}

func runtimeStep(resp *transport.RuntimeAppStepResponse, class, reason, detail string) {
	resp.Done = false
	resp.Class, resp.Reason, resp.Error = class, reason, detail
}

// runtimeAppContext is everything an action resolved from a target.
type runtimeAppContext struct {
	kind    siteruntime.Kind
	account serviceAppAccount
	layout  siteruntime.Layout
}

func resolveRuntimeApp(target transport.RuntimeAppTarget, resp *transport.RuntimeAppStepResponse) (runtimeAppContext, bool) {
	kind := siteruntime.Kind(target.Kind)
	if !siteruntime.Implemented(kind) || target.AppID <= 0 || target.SiteID <= 0 || target.SubscriptionID <= 0 || target.DomainID <= 0 {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "an app of an implemented kind, its site, subscription and domain are required")
		return runtimeAppContext{}, false
	}
	layout, err := siteruntime.LayoutFor(target.SubscriptionID, target.DomainID)
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", err.Error())
		return runtimeAppContext{}, false
	}
	account, err := serviceAppLookupAccount(transport.ServiceAppTarget{SubscriptionID: target.SubscriptionID, DomainID: target.DomainID, Domain: target.Domain})
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "site_account", err.Error())
		return runtimeAppContext{}, false
	}
	return runtimeAppContext{kind: kind, account: account, layout: layout}, true
}

// runtimeAppEntryCheck reads the entry without following a link and decides.
func runtimeAppEntryCheck(rc runtimeAppContext, entry string) transport.RuntimeAppEntryCheck {
	if _, err := siteruntime.ValidEntry(rc.layout, entry); err != nil {
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryInvalid, Detail: err.Error()}
	}
	// The folder as the sandbox helper walks it before every start: no link
	// anywhere from the hosting tree down, the right owners. A missing
	// folder is the entry's state (upload the program), not a verdict here.
	if _, err := os.Lstat(rc.layout.Root); err == nil {
		if code, detail := sandboxFolderRefusal(rc.layout.Root, rc.account.UID); code != "" {
			if code == sandboxbind.CodeUnreadable {
				code = siteruntime.EntryUnreadable
			}
			return transport.RuntimeAppEntryCheck{Code: code, Detail: detail}
		}
	}
	facts, err := runtimeAppEntryFacts(rc.layout.Root, siteruntime.EntryRelative(entry))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryMissing,
			Detail: "app/bin/" + entry + " is not on the server; upload it with the file manager or SFTP"}
	case err != nil:
		return transport.RuntimeAppEntryCheck{Code: siteruntime.EntryUnreadable, Detail: err.Error()}
	}
	facts.HostArch = runtimeAppHostArch
	code, detail := siteruntime.CheckEntry(facts, rc.account.UID, rc.layout.Root)
	if code == siteruntime.EntryOK {
		return transport.RuntimeAppEntryCheck{Code: code, Format: detail}
	}
	return transport.RuntimeAppEntryCheck{Code: code, Detail: detail}
}

// runtimeAppLevel decides whether this host may run the spec: a reduced host
// only when its every gap was accepted with the offering.
func runtimeAppLevel(caps sitesandbox.Caps, spec transport.RuntimeAppSpec) (sitesandbox.Assessment, string) {
	a := sitesandbox.Assess(caps)
	gaps := make([]string, len(a.Gaps))
	for i, g := range a.Gaps {
		gaps[i] = string(g)
	}
	if caps.SystemdVersion == 0 {
		return a, "the systemd version of this server could not be read, so the sandbox cannot be proved"
	}
	if a.Level == sitesandbox.LevelReduced && (spec.Level != string(sitesandbox.LevelReduced) || !siteruntime.GapsCovered(a.Gaps, spec.AcceptedGaps)) {
		return a, "this server does not enforce the full sandbox (" + strings.Join(gaps, ", ") + ") and the administrator has not accepted these gaps for this offering"
	}
	return a, ""
}

func gapNames(a sitesandbox.Assessment) []string {
	out := make([]string, len(a.Gaps))
	for i, g := range a.Gaps {
		out[i] = string(g)
	}
	return out
}

// RuntimeAppCheckV1 checks an app before a deploy. It changes nothing.
func (a *Agent) RuntimeAppCheckV1(req *transport.RuntimeAppCheckRequest, resp *transport.RuntimeAppCheckResponse) error {
	if req == nil {
		runtimeStep(&resp.RuntimeAppStepResponse, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	rc, ok := resolveRuntimeApp(req.Target, &resp.RuntimeAppStepResponse)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	assessment, refusal := runtimeAppLevel(sandboxCaps(ctx), req.Spec)
	resp.Level, resp.Gaps = string(assessment.Level), gapNames(assessment)
	resp.Entry = runtimeAppEntryCheck(rc, req.Spec.Entry)
	switch {
	case refusal != "":
		runtimeStep(&resp.RuntimeAppStepResponse, transport.RuntimeAppPrerequisite, "sandbox_reduced", refusal)
	case resp.Entry.Code == siteruntime.EntryUnreadable:
		runtimeStep(&resp.RuntimeAppStepResponse, transport.RuntimeAppUnknown, "entry_unreadable", resp.Entry.Detail)
	case resp.Entry.Code != siteruntime.EntryOK:
		runtimeStep(&resp.RuntimeAppStepResponse, transport.RuntimeAppPrerequisite, resp.Entry.Code, resp.Entry.Detail)
	default:
		resp.Done = true
	}
	return nil
}

func runtimeAppUnitPath(appID int) (string, error) {
	dir, err := unitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, siteruntime.UnitName(appID)), nil
}

func runtimeAppStateDir(appID int) string {
	return filepath.Join(runtimeAppStateRoot, strconv.Itoa(appID))
}

type runtimeAppLedger struct {
	RenderSHA256 string `json:"render_sha256"`
	WrittenAt    string `json:"written_at"`
}

func readRuntimeAppLedger(appID int) runtimeAppLedger {
	var l runtimeAppLedger
	if raw, err := os.ReadFile(filepath.Join(runtimeAppStateDir(appID), "unit.json")); err == nil {
		_ = json.Unmarshal(raw, &l)
	}
	return l
}

func writeRuntimeAppLedger(appID int, l runtimeAppLedger) error {
	dir := runtimeAppStateDir(appID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "unit.json")
	if err := os.WriteFile(path+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func runtimeAppManagedUnit(appID int, body string) (services.ManagedSiteFile, error) {
	path, err := runtimeAppUnitPath(appID)
	if err != nil {
		return services.ManagedSiteFile{}, err
	}
	return services.ManagedSiteFile{
		Kind: "runtime_unit", Path: path, HeaderPrefix: services.ManagedHeaderPrefixHash, Body: body,
		Trigger: transport.SiteFileTriggerCreate, RecordedSHA256: readRuntimeAppLedger(appID).RenderSHA256,
		IncludeDir: path + ".d",
	}, nil
}

// runtimeAppUnitFileState reads the D-031 state of the unit (read-only).
func runtimeAppUnitFileState(appID int) string {
	path, err := runtimeAppUnitPath(appID)
	if err != nil {
		return transport.SiteFileUnreadable
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return transport.SiteFileAbsent
	}
	f := services.ManagedSiteFile{Kind: "runtime_unit", Path: path, HeaderPrefix: services.ManagedHeaderPrefixHash,
		RecordedSHA256: readRuntimeAppLedger(appID).RenderSHA256}
	result, _, _ := services.InspectManagedSiteFile(f)
	return result.State
}

// runtimeAppDropIns lists the owner's drop-in files of the unit.
func runtimeAppDropIns(appID int) []string {
	path, err := runtimeAppUnitPath(appID)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(path + ".d")
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".conf") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func runtimeAppUnitParams(rc runtimeAppContext, target transport.RuntimeAppTarget, spec transport.RuntimeAppSpec, caps sitesandbox.Caps) (siteruntime.UnitParams, error) {
	path, err := runtimeAppUnitPath(target.AppID)
	if err != nil {
		return siteruntime.UnitParams{}, err
	}
	level := sitesandbox.LevelFull
	if spec.Level == string(sitesandbox.LevelReduced) {
		level = sitesandbox.LevelReduced
	}
	return siteruntime.UnitParams{
		AppID: target.AppID, Kind: rc.kind, Domain: target.Domain, User: rc.account.Name, Group: rc.account.Group,
		Layout: rc.layout, Entry: spec.Entry, Args: spec.Args, Port: spec.Port,
		Limits: siteruntime.Limits{MemoryMB: spec.Limits.MemoryMB, CPUPercent: spec.Limits.CPUPercent, Tasks: spec.Limits.TasksMax},
		Level:  level, Caps: caps, DropInDir: path + ".d",
	}, nil
}

// RuntimeAppRenderV1 makes the app's folders as the site's account, writes
// the unit under the D-031 rule and reloads systemd. It starts nothing.
func (a *Agent) RuntimeAppRenderV1(req *transport.RuntimeAppRenderRequest, resp *transport.RuntimeAppRenderResponse) error {
	step := &resp.RuntimeAppStepResponse
	if req == nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "writing a runtime app"); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	caps := sandboxCaps(ctx)
	if _, refusal := runtimeAppLevel(caps, req.Spec); refusal != "" {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "sandbox_reduced", refusal)
		return nil
	}
	if check := runtimeAppEntryCheck(rc, req.Spec.Entry); check.Code != siteruntime.EntryOK {
		runtimeStep(step, transport.RuntimeAppPrerequisite, check.Code, check.Detail)
		return nil
	}
	if _, err := runtimeAppAsUser(rc.account, "/bin/mkdir", "-p", "--", rc.layout.DataDir(), rc.layout.ConfigDir()); err != nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "site_folder", "the app's data and config folders could not be made as the site's account: "+err.Error())
		return nil
	}
	params, err := runtimeAppUnitParams(rc, req.Target, req.Spec, caps)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "unit_dir", err.Error())
		return nil
	}
	body, err := siteruntime.RenderUnit(params)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "unit_render", err.Error())
		return nil
	}
	if err := ensureSandboxHelper(); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "sandbox_helper", err.Error())
		return nil
	}
	if err := ensureSandboxEgress(caps); err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "sandbox_egress", err.Error())
		return nil
	}
	managed, err := runtimeAppManagedUnit(req.Target.AppID, body)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "unit_dir", err.Error())
		return nil
	}
	result, err := services.ApplyManagedSiteFile(managed, nil, func() error {
		if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	})
	resp.UnitFile, resp.PendingPath = result.Outcome, result.PendingPath
	if err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "unit_write", err.Error())
		return nil
	}
	switch result.Outcome {
	case transport.SiteFileOutcomeWritten, transport.SiteFileOutcomeRecreated, transport.SiteFileOutcomeTaken, transport.SiteFileOutcomeUnchanged:
		if err := writeRuntimeAppLedger(req.Target.AppID, runtimeAppLedger{RenderSHA256: result.RenderSHA256, WrittenAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
			log.Printf("RuntimeAppRenderV1 app %d: record the unit: %v", req.Target.AppID, err)
		}
		resp.Done = true
	case transport.SiteFileOutcomeKept:
		runtimeStep(step, transport.RuntimeAppPrerequisite, "unit_edited",
			managed.Path+" was changed on the server, so CelikPanel kept it; CelikPanel's text is in "+result.PendingPath+
				". Move your own settings to "+managed.IncludeDir+"/*.conf, or put CelikPanel's text back, then deploy again")
	default:
		runtimeStep(step, transport.RuntimeAppUnknown, "unit_write", fmt.Sprintf("the unit could not be written (%s %s): %s", result.Outcome, result.Reason, result.Detail))
	}
	return nil
}

// RuntimeAppControlV1 starts, stops or restarts an app's unit.
func (a *Agent) RuntimeAppControlV1(req *transport.RuntimeAppControlRequest, resp *transport.RuntimeAppStepResponse) error {
	if req == nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "controlling a runtime app"); err != nil {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.Target.AppID <= 0 {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "an app is required")
		return nil
	}
	name := siteruntime.UnitName(req.Target.AppID)
	var steps [][]string
	switch req.Action {
	case "start", "restart":
		// reset-failed clears systemd's start limit, so a start after a
		// restart loop is a real new attempt.
		steps = [][]string{{"reset-failed", name}, {"enable", name}, {"restart", name}}
	case "stop":
		steps = [][]string{{"stop", name}, {"disable", name}}
	default:
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "action must be start, stop or restart")
		return nil
	}
	unlock := lockRuntimeApp(req.Target.AppID)
	defer unlock()
	if req.Action != "stop" && runtimeAppUnitFileState(req.Target.AppID) == transport.SiteFileAbsent {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "unit_missing", "the app's unit is not on the server; deploy the app again")
		return nil
	}
	for _, s := range steps {
		if out, err := appUnitSystemctl(systemctlArgs(s...)...); err != nil {
			if s[0] == "reset-failed" {
				continue
			}
			// A start the sandbox helper refused: name the app folder's
			// state instead of systemd's exit status.
			if s[0] == "restart" {
				var probe transport.RuntimeAppStepResponse
				if rc, ok := resolveRuntimeApp(req.Target, &probe); ok {
					if code, detail := sandboxFolderRefusal(rc.layout.Root, rc.account.UID); code != "" && code != sandboxbind.CodeUnreadable {
						runtimeStep(resp, transport.RuntimeAppPrerequisite, code, detail)
						return nil
					}
				}
			}
			runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "systemctl",
				fmt.Sprintf("systemctl %s: %v: %s", strings.Join(s, " "), err, strings.TrimSpace(string(out))))
			return nil
		}
	}
	resp.Done = true
	return nil
}

func parseCgroupCount(text, key string) int64 {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == key {
			n, _ := strconv.ParseInt(fields[1], 10, 64)
			return n
		}
	}
	return 0
}

// runtimeAppHTTPHealth is one GET on the app's loopback port with the site's
// name, no redirects followed, a small body read and a short time limit.
func runtimeAppHTTPHealth(ctx context.Context, port int, host, path string) transport.RuntimeAppHealth {
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+path, nil)
	if err != nil {
		return transport.RuntimeAppHealth{Checked: true, Error: err.Error()}
	}
	request.Host = host
	request.Header.Set("User-Agent", "CelikPanel-health/1")
	request.Header.Set("X-Forwarded-Proto", "https")
	response, err := client.Do(request)
	if err != nil {
		return transport.RuntimeAppHealth{Checked: true, Error: "no answer on 127.0.0.1:" + strconv.Itoa(port)}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	response.Body.Close()
	return transport.RuntimeAppHealth{Checked: true, Status: response.StatusCode, OK: response.StatusCode < 500}
}

// RuntimeAppStatusV1 reads an app (read-only): the unit file and the owner's
// drop-ins, systemd's view, the cgroup's events, the listen proof and, when
// asked, the sandbox proof, one health GET and the entry check.
func (a *Agent) RuntimeAppStatusV1(req *transport.RuntimeAppStatusRequest, resp *transport.RuntimeAppStatusResponse) error {
	if req == nil || req.Target.AppID <= 0 {
		resp.Error = "an app is required"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := siteruntime.UnitName(req.Target.AppID)
	resp.UnitFile = runtimeAppUnitFileState(req.Target.AppID)
	resp.DropIns = runtimeAppDropIns(req.Target.AppID)
	if path, err := runtimeAppUnitPath(req.Target.AppID); err == nil {
		if raw, err := os.ReadFile(path); err == nil {
			resp.Generation = serviceapps.UnitGeneration(string(raw))
		}
	}
	if raw, err := os.ReadFile(runtimeAppEnvPath(req.Target.AppID)); err == nil {
		for key := range siteruntime.ParseEnv(string(raw)) {
			resp.EnvKeys = append(resp.EnvKeys, key)
		}
		sort.Strings(resp.EnvKeys)
	}
	caps := sandboxCaps(ctx)
	assessment := sitesandbox.Assess(caps)
	resp.Level, resp.Gaps = string(assessment.Level), gapNames(assessment)
	out, err := appUnitSystemctl(systemctlArgs("show", name, "--property=LoadState,ActiveState,SubState,UnitFileState,MemoryCurrent,Result,NRestarts")...)
	if err != nil {
		resp.Error = "systemd could not be read: " + strings.TrimSpace(string(out))
		return nil
	}
	props := parseShow(string(out))
	if state, err := appUnitStateFromShow(string(out)); err == nil {
		resp.Unit = state
	}
	resp.ActiveState, resp.SubState = props["ActiveState"], props["SubState"]
	if props["Result"] != "" && props["Result"] != "success" {
		resp.Result = props["Result"]
	}
	resp.StartLimitHit = props["Result"] == "start-limit-hit"
	resp.Restarts, _ = strconv.Atoi(props["NRestarts"])
	if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil && mem > 0 {
		resp.MemoryMB = mem / (1 << 20)
	}
	if raw, err := os.ReadFile(filepath.Join(runtimeAppCgroupRoot, name, "memory.events")); err == nil {
		resp.OOMKills = parseCgroupCount(string(raw), "oom_kill")
	}
	if raw, err := os.ReadFile(filepath.Join(runtimeAppCgroupRoot, name, "pids.events")); err == nil {
		resp.PidsMax = parseCgroupCount(string(raw), "max")
	}
	var rc runtimeAppContext
	var resolved bool
	if req.Entry || req.Proof {
		var step transport.RuntimeAppStepResponse
		rc, resolved = resolveRuntimeApp(req.Target, &step)
		if req.Entry {
			if resolved {
				check := runtimeAppEntryCheck(rc, req.Spec.Entry)
				resp.Entry = &check
			} else {
				resp.Entry = &transport.RuntimeAppEntryCheck{Code: step.Reason, Detail: step.Error}
			}
		}
	}
	if !resp.Unit.Active || !siteruntime.ValidPort(req.Spec.Port) {
		return nil
	}
	proof, err := serviceAppListenProof(name, req.Spec.Port)
	if err != nil {
		resp.Error = "the listening sockets could not be read: " + err.Error()
		return nil
	}
	resp.Serving, resp.Exposed, resp.Squatted = proof.Serving, proof.Exposed, proof.Squatted
	if req.Proof {
		if !resolved {
			resp.Proof = &transport.SandboxProof{Code: sitesandbox.ProofUnreadable, Detail: "the site account could not be resolved"}
		} else if params, err := runtimeAppUnitParams(rc, req.Target, req.Spec, caps); err != nil {
			resp.Proof = &transport.SandboxProof{Code: sitesandbox.ProofUnreadable, Detail: err.Error()}
		} else {
			result := sandboxProof(ctx, name, params.Profile(), caps, &proof)
			resp.Proof = &transport.SandboxProof{OK: result.OK, Code: result.Code, Detail: result.Detail}
		}
	}
	if req.HealthPath != "" && siteruntime.ValidHealthPath(req.HealthPath) && proof.Serving {
		resp.Health = runtimeAppHealthGet(ctx, req.Spec.Port, req.Target.Domain, req.HealthPath)
	}
	return nil
}

// RuntimeAppEnvV1 replaces the environment file (root's, 0600) and, when
// asked, restarts a running app so it reads it.
func (a *Agent) RuntimeAppEnvV1(req *transport.RuntimeAppEnvRequest, resp *transport.RuntimeAppStepResponse) error {
	if req == nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a runtime app's environment"); err != nil {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if req.Target.AppID <= 0 {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "invalid_request", "an app is required")
		return nil
	}
	unlock := lockRuntimeApp(req.Target.AppID)
	defer unlock()
	entries := make([]siteruntime.EnvEntry, len(req.Entries))
	for i, e := range req.Entries {
		entries[i] = siteruntime.EnvEntry{Key: e.Key, Value: e.Value, Secret: e.Secret}
	}
	existing := map[string]string{}
	if raw, err := os.ReadFile(runtimeAppEnvPath(req.Target.AppID)); err == nil {
		existing = siteruntime.ParseEnv(string(raw))
	} else if !os.IsNotExist(err) {
		runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
		return nil
	}
	// D-022: a key the owner added to the file by hand (not one CelikPanel
	// wrote, not one in this request) is never dropped silently.
	if added := siteruntime.EnvOwnerKeys(existing, req.PreviousKeys, entries); len(added) > 0 {
		runtimeStep(resp, transport.RuntimeAppPrerequisite, "env_edited",
			"the environment file "+runtimeAppEnvPath(req.Target.AppID)+" has keys CelikPanel did not write ("+strings.Join(added, ", ")+
				"); nothing was changed. Add them here to keep them, or remove them from the file, then save again")
		return nil
	}
	merged, err := siteruntime.MergeEnv(entries, existing)
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	text, err := siteruntime.RenderEnv(merged)
	if err != nil {
		runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	if err := writeRuntimeAppEnvFile(req.Target.AppID, text); err != nil {
		runtimeStep(resp, transport.RuntimeAppUnknown, "env_file", err.Error())
		return nil
	}
	if req.Restart {
		if out, err := appUnitSystemctl(systemctlArgs("try-restart", siteruntime.UnitName(req.Target.AppID))...); err != nil {
			runtimeStep(resp, transport.RuntimeAppVerifiedFailure, "systemctl",
				fmt.Sprintf("the values are saved, but the app could not be restarted: %v: %s", err, strings.TrimSpace(string(out))))
			return nil
		}
	}
	resp.Done = true
	return nil
}

// writeRuntimeAppEnvFile writes the file root's and 0600 in a root folder
// (0700): no link the site's account controls is ever on the path.
func writeRuntimeAppEnvFile(appID int, text string) error {
	if err := os.MkdirAll(runtimeAppEnvDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeAppEnvDir, 0o700); err != nil {
		return err
	}
	path := runtimeAppEnvPath(appID)
	tmp := path + ".partial"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(text)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	return os.Rename(tmp, path)
}

// RuntimeAppLogsV1 returns the unit's last journal lines with every secret
// value of the environment file and every key-like value hidden.
func (a *Agent) RuntimeAppLogsV1(req *transport.RuntimeAppLogsRequest, resp *transport.RuntimeAppLogsResponse) error {
	if req == nil || req.Target.AppID <= 0 {
		resp.Error = "an app is required"
		return nil
	}
	lines := req.Lines
	if lines <= 0 || lines > 500 {
		lines = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"-u", siteruntime.UnitName(req.Target.AppID), "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso"}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	out, err := serviceAppRun(ctx, "journalctl", args...)
	if err != nil {
		resp.Error = "the journal could not be read"
		return nil
	}
	resp.Lines = redactRuntimeAppLines(string(out), runtimeAppSecretValues(req.Target.AppID, req.SecretKeys))
	return nil
}

func runtimeAppSecretValues(appID int, keys []string) []string {
	raw, err := os.ReadFile(runtimeAppEnvPath(appID))
	if err != nil {
		return nil
	}
	env := siteruntime.ParseEnv(string(raw))
	out := []string{}
	for _, key := range keys {
		if v := env[key]; v != "" {
			out = append(out, v)
		}
	}
	return out
}

func redactRuntimeAppLines(text string, secrets []string) []string {
	out := []string{}
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "-- ") {
			continue
		}
		out = append(out, serviceapps.Redact(line, secrets))
	}
	return out
}

// RuntimeAppRemoveV1 removes the unit (a copy is kept when the owner had
// changed it) and the environment file; the app folder only when asked.
func (a *Agent) RuntimeAppRemoveV1(req *transport.RuntimeAppRemoveRequest, resp *transport.RuntimeAppRemoveResponse) error {
	step := &resp.RuntimeAppStepResponse
	if req == nil {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing a runtime app"); err != nil {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	unlock := lockRuntimeApp(req.Target.AppID)
	defer unlock()
	if req.Target.AppID <= 0 {
		runtimeStep(step, transport.RuntimeAppVerifiedFailure, "invalid_request", "an app is required")
		return nil
	}
	// Removal is never refused for want of the site account: the unit and
	// the environment file go; only the folder needs the account.
	var resolveStep transport.RuntimeAppStepResponse
	rc, ok := resolveRuntimeApp(req.Target, &resolveStep)
	name := siteruntime.UnitName(req.Target.AppID)
	for _, s := range []string{"stop", "disable", "reset-failed"} {
		_, _ = appUnitSystemctl(systemctlArgs(s, name)...)
	}
	path, err := runtimeAppUnitPath(req.Target.AppID)
	if err != nil {
		runtimeStep(step, transport.RuntimeAppUnknown, "unit_dir", err.Error())
		return nil
	}
	if raw, err := os.ReadFile(path); err == nil {
		if state := runtimeAppUnitFileState(req.Target.AppID); state != transport.SiteFileManagedUnchanged {
			copyPath := path + ".celikpanel-removed-" + time.Now().UTC().Format("20060102T150405Z")
			if err := os.WriteFile(copyPath, raw, 0o600); err != nil {
				runtimeStep(step, transport.RuntimeAppUnknown, "unit_copy", "a copy of the changed unit could not be kept, so it was not removed: "+err.Error())
				return nil
			}
			resp.KeptCopy = copyPath
		}
		if err := os.Remove(path); err != nil {
			runtimeStep(step, transport.RuntimeAppUnknown, "unit_remove", err.Error())
			return nil
		}
	}
	_ = os.Remove(path + ".celikpanel-pending")
	if err := os.Remove(runtimeAppEnvPath(req.Target.AppID)); err != nil && !os.IsNotExist(err) {
		runtimeStep(step, transport.RuntimeAppUnknown, "env_remove", err.Error())
		return nil
	}
	if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		log.Printf("RuntimeAppRemoveV1 app %d: daemon-reload: %v: %s", req.Target.AppID, err, bytes.TrimSpace(out))
	}
	if req.RemoveFiles && !ok {
		runtimeStep(step, transport.RuntimeAppPrerequisite, "files", "the unit is removed, but the app folder was left: "+resolveStep.Error)
		return nil
	}
	if req.RemoveFiles {
		if _, err := runtimeAppAsUser(rc.account, "/bin/rm", "-rf", "--", rc.layout.Root); err != nil {
			runtimeStep(step, transport.RuntimeAppUnknown, "files", "the unit is removed, but the app folder could not be removed: "+err.Error())
			return nil
		}
	}
	_ = os.RemoveAll(runtimeAppStateDir(req.Target.AppID))
	state, err := inspectUnitSuspensionState(name)
	if err != nil || state.Active {
		runtimeStep(step, transport.RuntimeAppUnknown, "verify", "the unit's final state could not be confirmed")
		return nil
	}
	resp.Done = true
	return nil
}

// RuntimeAppUnitStateV1 reads one app's unit (suspension before-image).
func (a *Agent) RuntimeAppUnitStateV1(req *transport.RuntimeAppUnitStateRequest, resp *transport.ServiceAppUnitStateResponse) error {
	if req == nil || req.AppID <= 0 {
		resp.Error = "an app is required"
		return nil
	}
	state, err := inspectUnitSuspensionState(siteruntime.UnitName(req.AppID))
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.State = state
	return nil
}

// SetRuntimeAppSuspensionV1 stops and disables an app's unit for a
// suspension or a security hold, or restores the recorded state
// compare-and-set (D-022).
func (a *Agent) SetRuntimeAppSuspensionV1(req *transport.SetRuntimeAppSuspensionRequest, resp *transport.SetAppUnitSuspensionResponse) error {
	if req == nil || resp == nil {
		return errors.New("runtime app suspension request and response are required")
	}
	*resp = transport.SetAppUnitSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a runtime app"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.AppID <= 0 {
		resp.Error = "a positive app identity is required; nothing was changed"
		return nil
	}
	unlock := lockRuntimeApp(req.AppID)
	defer unlock()
	applyUnitSuspension(siteruntime.UnitName(req.AppID), req.Suspend, req.Before, resp)
	return nil
}

// runtimeAppEnvPath is the environment file of an app's unit.
func runtimeAppEnvPath(appID int) string {
	return filepath.Join(runtimeAppEnvDir, siteruntime.UnitName(appID)+".env")
}
