package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Service apps (alpha.118, docs/AI-APPS.md): AI and automation applications
// that run as a long-running unit of the site's own user, in a systemd
// sandbox, listening on 127.0.0.1 only and reached through the site's nginx
// over HTTPS. Root does only what needs root: it downloads and checks the
// pinned upstream file, writes the unit and drives systemd. Every write
// inside the site's folder (folders, the verified file, the environment file,
// npm or pip, removal) runs as the site's user, so a link planted in that
// folder can never make root write elsewhere. The unit and its environment
// file are native systemd configuration and keep working without CelikPanel.
//
// Hizmet uygulamaları: sitenin kendi kullanıcısıyla, systemd korumasında,
// yalnız 127.0.0.1'de çalışan ve sitenin nginx'i üzerinden HTTPS ile açılan
// yapay zekâ ve otomasyon uygulamaları. Root yalnız root gerektiren işi yapar;
// site klasörünün içindeki her yazma sitenin kullanıcısıyla yapılır.

var (
	serviceAppStateRoot = filepath.Join(hostingpath.ServiceMutationStateRoot(), "service-apps")
	serviceAppOperation = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{7,63}$`)
	serviceAppLocks     [64]sync.Mutex
	// serviceAppGeneratedValue is a key made on the server (randomHex).
	serviceAppGeneratedValue = regexp.MustCompile(`^[0-9a-f]{32,128}$`)
)

func lockServiceApp(installID int) func() {
	lock := &serviceAppLocks[uint(installID)%uint(len(serviceAppLocks))]
	lock.Lock()
	return lock.Unlock
}

// serviceAppAccount is the site's own system account.
type serviceAppAccount struct {
	Name  string
	Group string
	UID   int
	GID   int
	Home  string
}

// serviceAppLookupAccount resolves and proves the site account: its name is
// derived from the domain, and its home must be exactly the site's home
// (SiteUsername is not injective, so the name alone proves nothing).
var serviceAppLookupAccount = func(target transport.ServiceAppTarget) (serviceAppAccount, error) {
	home, err := hostingpath.SiteHome(target.SubscriptionID, target.DomainID)
	if err != nil {
		return serviceAppAccount{}, err
	}
	name := services.SiteUsername(target.Domain)
	account, err := user.Lookup(name)
	if err != nil {
		return serviceAppAccount{}, fmt.Errorf("the site's system account %s is missing", name)
	}
	if filepath.Clean(account.HomeDir) != home {
		return serviceAppAccount{}, fmt.Errorf("the account %s does not belong to this site", name)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid < 1000 {
		return serviceAppAccount{}, errors.New("the site account is a system account")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid < 1000 {
		return serviceAppAccount{}, errors.New("the site account's group is a system group")
	}
	group, err := user.LookupGroupId(account.Gid)
	if err != nil {
		return serviceAppAccount{}, fmt.Errorf("the site account's group: %v", err)
	}
	return serviceAppAccount{Name: name, Group: group.Name, UID: uid, GID: gid, Home: home}, nil
}

// serviceAppRun runs a root command (systemctl is appUnitSystemctl).
var serviceAppRun = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// serviceAppAsUser runs a fixed shell text as the site's user (Linux:
// service_apps_linux.go).
var serviceAppAsUser = runServiceAppAsUser

// serviceAppUnitPIDs lists the processes of a unit (cgroup v2).
var serviceAppUnitPIDs = unitCgroupPIDs

// serviceAppDownload fetches the pinned file into dest, checking its hash
// while it is written; a file that does not match is removed.
var serviceAppDownload = func(url, dest, algorithm, digest string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	tmp := dest + ".partial"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	var h hash.Hash = sha256.New()
	if algorithm == "sha512" {
		h = sha512.New()
	}
	const limit = int64(512 << 20)
	written, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(response.Body, limit+1))
	if copyErr == nil && written > limit {
		copyErr = errors.New("the download is larger than expected")
	}
	if copyErr == nil {
		copyErr = f.Sync()
	}
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && hex.EncodeToString(h.Sum(nil)) != digest {
		copyErr = errors.New("the downloaded file does not match the pinned hash; it was discarded")
	}
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	return os.Rename(tmp, dest)
}

func serviceAppFileDigest(path, algorithm string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var h hash.Hash = sha256.New()
	if algorithm == "sha512" {
		h = sha512.New()
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// serviceAppProbe lists the runtimes on this server. Node.js: the Panel's
// verified runtimes and the distribution's /usr/bin/node; Python: the
// distribution's python3 and its versioned names. Nothing is installed.
var serviceAppProbe = func(ctx context.Context) ([]serviceapps.NodeCandidate, []serviceapps.PythonCandidate) {
	var nodes []serviceapps.NodeCandidate
	if entries, err := os.ReadDir(filepath.Join(runtimesBaseDir, "node")); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || !nodeVersionRe.MatchString(entry.Name()) {
				continue
			}
			dir := filepath.Join(runtimesBaseDir, "node", entry.Name())
			cli := filepath.Join(dir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
			if _, err := os.Stat(filepath.Join(dir, "bin", "node")); err != nil {
				continue
			}
			if _, err := os.Stat(cli); err != nil {
				cli = ""
			}
			nodes = append(nodes, serviceapps.NodeCandidate{Path: filepath.Join(dir, "bin", "node"), Version: entry.Name(), NPMCLI: cli, Source: "panel"})
		}
	}
	if out, err := serviceAppRun(ctx, "/usr/bin/node", "--version"); err == nil {
		version := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
		cli := ""
		for _, candidate := range []string{"/usr/lib/node_modules/npm/bin/npm-cli.js", "/usr/share/nodejs/npm/bin/npm-cli.js"} {
			if _, err := os.Stat(candidate); err == nil {
				cli = candidate
				break
			}
		}
		if nodeVersionRe.MatchString(version) {
			nodes = append(nodes, serviceapps.NodeCandidate{Path: "/usr/bin/node", Version: version, NPMCLI: cli, Source: "distribution"})
		}
	}
	var pythons []serviceapps.PythonCandidate
	seen := map[string]bool{}
	const probe = "import sys\nv='%d.%d'%sys.version_info[:2]\ntry:\n import venv, ensurepip\n print(v, 1)\nexcept Exception:\n print(v, 0)\n"
	for _, path := range []string{"/usr/bin/python3", "/usr/bin/python3.11", "/usr/bin/python3.12"} {
		out, err := serviceAppRun(ctx, path, "-I", "-c", probe)
		if err != nil {
			continue
		}
		fields := strings.Fields(string(out))
		if len(fields) != 2 || seen[fields[0]] {
			continue
		}
		seen[fields[0]] = true
		pythons = append(pythons, serviceapps.PythonCandidate{Path: path, Version: fields[0], Venv: fields[1] == "1"})
	}
	return nodes, pythons
}

// ServiceAppProbeV1 says, for every catalog app, whether it can run here.
func (a *Agent) ServiceAppProbeV1(_ *transport.Empty, resp *transport.ServiceAppProbeResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	nodes, pythons := serviceAppProbe(ctx)
	resp.Apps = []transport.ServiceAppAvailability{}
	for _, app := range serviceapps.Catalog() {
		choice := serviceapps.ChooseRuntime(app, nodes, pythons)
		resp.Apps = append(resp.Apps, transport.ServiceAppAvailability{
			AppID: app.ID, Available: choice.Available, Reason: choice.Reason, Detail: choice.Detail,
			RuntimeVersion: choice.Version, RuntimeSource: choice.Source,
		})
	}
	return nil
}

func serviceAppStep(resp *transport.ServiceAppStepResponse, class, reason, detail string) {
	resp.Done = false
	resp.Class, resp.Reason, resp.Error = class, reason, detail
}

// serviceAppContext validates a target and resolves everything an action
// needs. A failure fills resp and returns ok false.
type serviceAppContext struct {
	app     serviceapps.App
	account serviceAppAccount
	layout  serviceapps.Layout
}

func resolveServiceApp(target transport.ServiceAppTarget, resp *transport.ServiceAppStepResponse) (serviceAppContext, bool) {
	app, ok := serviceapps.Lookup(target.AppID)
	if !ok || target.InstallID <= 0 || target.SiteID <= 0 || target.SubscriptionID <= 0 || target.DomainID <= 0 {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "an install, site, subscription, domain and catalog app are required")
		return serviceAppContext{}, false
	}
	layout, err := serviceapps.LayoutFor(target.SubscriptionID, target.DomainID, app.ID)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", err.Error())
		return serviceAppContext{}, false
	}
	account, err := serviceAppLookupAccount(target)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "site_account", err.Error())
		return serviceAppContext{}, false
	}
	return serviceAppContext{app: app, account: account, layout: layout}, true
}

func validServiceAppPorts(app serviceapps.App, port, aux int) bool {
	if port < serviceapps.PortMin || port > serviceapps.PortMax {
		return false
	}
	if app.AuxPort {
		return aux >= serviceapps.PortMin && aux <= serviceapps.PortMax+1 && aux != port
	}
	return true
}

func serviceAppLimits(app serviceapps.App, limits transport.ServiceAppLimits) serviceapps.Resources {
	out := app.Defaults
	if limits.MemoryMB >= 256 && limits.MemoryMB <= 65536 {
		out.MemoryMB = limits.MemoryMB
	}
	if limits.CPUPercent >= 10 && limits.CPUPercent <= 1600 {
		out.CPUPercent = limits.CPUPercent
	}
	if limits.TasksMax >= 32 && limits.TasksMax <= 8192 {
		out.TasksMax = limits.TasksMax
	}
	return out
}

func serviceAppRuntime(ctx context.Context, app serviceapps.App) serviceapps.RuntimeChoice {
	nodes, pythons := serviceAppProbe(ctx)
	return serviceapps.ChooseRuntime(app, nodes, pythons)
}

type serviceAppMarker struct {
	OperationID string `json:"operation_id"`
	Version     string `json:"version"`
	StartedAt   string `json:"started_at"`
}

func serviceAppStateDir(installID int) string {
	return filepath.Join(serviceAppStateRoot, strconv.Itoa(installID))
}

func readServiceAppMarker(installID int) serviceAppMarker {
	var marker serviceAppMarker
	raw, err := os.ReadFile(filepath.Join(serviceAppStateDir(installID), "operation.json"))
	if err == nil {
		_ = json.Unmarshal(raw, &marker)
	}
	return marker
}

func writeServiceAppMarker(installID int, marker serviceAppMarker) error {
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	path := filepath.Join(serviceAppStateDir(installID), "operation.json")
	if err := os.WriteFile(path+".tmp", raw, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// installUnitState reads the transient install unit: none, running,
// succeeded (active/exited, RemainAfterExit) or failed.
func installUnitState(installID int) (string, string) {
	out, err := appUnitSystemctl(systemctlArgs("show", serviceapps.InstallUnitName(installID), "--property=LoadState,ActiveState,SubState,Result")...)
	if err != nil {
		return "unknown", strings.TrimSpace(string(out))
	}
	props := parseShow(string(out))
	switch {
	case props["LoadState"] == "not-found" || props["ActiveState"] == "inactive" && props["Result"] == "success":
		return "none", ""
	case props["ActiveState"] == "failed":
		return "failed", props["Result"]
	case props["ActiveState"] == "active" && props["SubState"] == "exited":
		return "succeeded", ""
	case props["ActiveState"] == "active" || props["ActiveState"] == "activating" || props["ActiveState"] == "deactivating":
		return "running", ""
	}
	return "none", props["ActiveState"]
}

func parseShow(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return props
}

// readServiceAppEnv reads the environment file as the site's user ("" when
// it does not exist yet).
func readServiceAppEnv(sc serviceAppContext) (map[string]string, error) {
	out, err := serviceAppAsUser(sc.account, nil, `f="$1/config/app.env"; if [ -f "$f" ] && [ ! -L "$f" ]; then cat "$f"; fi`, sc.layout.Root)
	if err != nil {
		return nil, fmt.Errorf("read the environment file: %v", err)
	}
	return serviceapps.ParseEnv(string(out)), nil
}

func writeServiceAppEnv(sc serviceAppContext, text string) error {
	_, err := serviceAppAsUser(sc.account, strings.NewReader(text),
		`umask 077; mkdir -p "$1/config" && cat > "$1/config/app.env.partial" && mv -f "$1/config/app.env.partial" "$1/config/app.env"`, sc.layout.Root)
	if err != nil {
		return fmt.Errorf("write the environment file: %v", err)
	}
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// renderServiceAppEnv merges what is on disk (generated keys, the customer's
// values, the owner) with this request's values and CelikPanel's fixed lines.
func renderServiceAppEnv(sc serviceAppContext, existing map[string]string, domain string, port, aux int,
	ownerEmail, ownerSecret string, secrets map[string]string, clear []string) (string, error) {
	params := serviceapps.EnvParams{App: sc.app, Domain: domain, Layout: sc.layout, Port: port, AuxPort: aux,
		Generated: map[string]string{}, Secrets: map[string]string{}}
	for _, key := range serviceapps.GeneratedKeys(sc.app) {
		if value := existing[key]; serviceAppGeneratedValue.MatchString(value) {
			params.Generated[key] = value
			continue
		}
		value, err := randomHex(32)
		if err != nil {
			return "", err
		}
		params.Generated[key] = value
	}
	allowed := sc.app.AllowedSecretKeys()
	for key := range allowed {
		if value := existing[key]; value != "" {
			params.Secrets[key] = value
		}
	}
	emailKey, secretKey := serviceapps.OwnerKeys(sc.app)
	params.OwnerEmail, params.OwnerSecret = existing[emailKey], existing[secretKey]
	for key, value := range secrets {
		if value == "" {
			continue
		}
		params.Secrets[key] = value
	}
	if ownerEmail != "" {
		params.OwnerEmail = ownerEmail
	}
	if ownerSecret != "" {
		params.OwnerSecret = ownerSecret
	}
	for _, key := range clear {
		switch {
		case allowed[key]:
			delete(params.Secrets, key)
		case key == secretKey:
			params.OwnerSecret = ""
		}
	}
	return serviceapps.RenderEnv(params)
}

// ServiceAppPrepareV1 prepares an install or update and starts npm or pip in
// a transient sandboxed unit as the site's user. It returns once that unit
// has started; ServiceAppStatusV1 follows it.
func (a *Agent) ServiceAppPrepareV1(req *transport.ServiceAppPrepareRequest, resp *transport.ServiceAppStepResponse) error {
	if req == nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "installing a service app"); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	if !serviceAppOperation.MatchString(req.OperationID) {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "an operation identity is required")
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	sc, ok := resolveServiceApp(req.Target, resp)
	if !ok {
		return nil
	}
	if !sc.app.Installable() || req.Version != sc.app.Version {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "version_not_pinned",
			fmt.Sprintf("%s can be installed only at the version this release pins (%s)", sc.app.Name, sc.app.Version))
		return nil
	}
	if !validServiceAppPorts(sc.app, req.Port, req.AuxPort) {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "the ports are outside the service-app range")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Minute)
	defer cancel()
	runtime := serviceAppRuntime(ctx, sc.app)
	if !runtime.Available {
		class := transport.ServiceAppPrerequisite
		if runtime.Reason == serviceapps.ReasonOSUnsupported || runtime.Reason == serviceapps.ReasonDeferred {
			class = transport.ServiceAppVerifiedFailure
		}
		serviceAppStep(resp, class, runtime.Reason, runtime.Detail)
		return nil
	}

	// The same operation again (the Panel restarted, a lost answer): the
	// install unit it started is followed, never started twice.
	marker := readServiceAppMarker(req.Target.InstallID)
	state, _ := installUnitState(req.Target.InstallID)
	if marker.OperationID == req.OperationID {
		switch state {
		case "running", "succeeded":
			resp.Done = true
			return nil
		case "failed":
			serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "install_failed", "the install step of this operation failed; read its log and start a new attempt")
			return nil
		}
	} else if state == "running" {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "busy", "another install of this application is still running")
		return nil
	}

	algorithm, digest, err := sc.app.ArtifactHex()
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "pin_invalid", err.Error())
		return nil
	}
	stateDir := serviceAppStateDir(req.Target.InstallID)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "state_dir", err.Error())
		return nil
	}
	staged := filepath.Join(stateDir, sc.app.ArtifactFile)
	if got, err := serviceAppFileDigest(staged, algorithm); err != nil || got != digest {
		_ = os.Remove(staged)
		if err := serviceAppDownload(sc.app.ArtifactURL, staged, algorithm, digest); err != nil {
			serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "download_failed",
				fmt.Sprintf("%s %s could not be downloaded and verified from %s: %v", sc.app.Name, sc.app.Version, sc.app.ArtifactURL, err))
			return nil
		}
	}

	// Folders and the verified file, written by the site's user.
	artifact, err := os.Open(staged)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "staged_file", err.Error())
		return nil
	}
	_, err = serviceAppAsUser(sc.account, artifact,
		`umask 077; mkdir -p "$1/dist" "$1/data/files" "$1/config" && cat > "$1/dist/$2.partial" && mv -f "$1/dist/$2.partial" "$1/dist/$2"`,
		sc.layout.Root, sc.app.ArtifactFile)
	artifact.Close()
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "site_folder", "the application folder could not be prepared as the site's user: "+err.Error())
		return nil
	}

	existing, err := readServiceAppEnv(sc)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "env_file", err.Error())
		return nil
	}
	text, err := renderServiceAppEnv(sc, existing, req.Target.Domain, req.Port, req.AuxPort, req.OwnerEmail, req.OwnerSecret, req.Secrets, nil)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	if err := writeServiceAppEnv(sc, text); err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "env_file", err.Error())
		return nil
	}

	args, err := serviceapps.InstallArgs(sc.app, sc.layout, req.Version, runtime)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", err.Error())
		return nil
	}
	unit := serviceapps.InstallUnitName(req.Target.InstallID)
	_, _ = appUnitSystemctl(systemctlArgs("stop", unit)...)
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", unit)...)
	limits := serviceAppLimits(sc.app, req.Limits)
	pathEnv := filepath.Dir(runtime.Path) + ":/usr/local/bin:/usr/bin:/bin"
	run := []string{"--unit=" + unit, "--description=CelikPanel install of " + sc.app.Name + " " + req.Version,
		"--uid=" + sc.account.Name, "--gid=" + sc.account.Group, "--working-directory=" + sc.layout.Root,
		"--setenv=HOME=" + sc.layout.DataDir(), "--setenv=PATH=" + pathEnv,
		"--property=RemainAfterExit=yes", "--property=RuntimeMaxSec=7200", "--no-block", "--quiet"}
	caps := sandboxCaps(ctx)
	if err := ensureSandboxEgress(caps); err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "sandbox_egress", err.Error())
		return nil
	}
	properties, err := serviceapps.InstallUnitProperties(req.Target.InstallID, sc.layout, sc.account.Name, sc.account.Group, limits, sc.app.Defaults.InstallMemoryMB, caps)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "sandbox_render", err.Error())
		return nil
	}
	for _, directive := range properties {
		run = append(run, "--property="+directive)
	}
	command, err := serviceapps.InstallCommand(sc.app, args)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", err.Error())
		return nil
	}
	run = append(run, "--")
	run = append(run, command...)
	if out, err := serviceAppRun(ctx, "systemd-run", run...); err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "install_unit",
			"the install step could not be started: "+strings.TrimSpace(string(out)))
		return nil
	}
	if err := writeServiceAppMarker(req.Target.InstallID, serviceAppMarker{OperationID: req.OperationID, Version: req.Version, StartedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		log.Printf("ServiceAppPrepareV1 install %d: record the operation: %v", req.Target.InstallID, err)
	}
	resp.Done = true
	return nil
}

func serviceAppInstalledEntry(app serviceapps.App, layout serviceapps.Layout, version string) string {
	dir := layout.InstallDir(version)
	if app.Runtime == serviceapps.RuntimeNode {
		return filepath.Join(dir, "node_modules", app.Package, "package.json")
	}
	return filepath.Join(dir, "bin", app.Package)
}

func serviceAppUnitPath(installID int) (string, error) {
	dir, err := unitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, serviceapps.UnitName(installID)+".service"), nil
}

func serviceAppUnitFileState(installID int) string {
	path, err := serviceAppUnitPath(installID)
	if err != nil {
		return "unknown"
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "absent"
	}
	if err != nil {
		return "unknown"
	}
	return serviceapps.UnitTextState(string(raw))
}

// serviceAppUnitGeneration is the sandbox generation of the unit file (0:
// no readable file).
func serviceAppUnitGeneration(installID int) int {
	path, err := serviceAppUnitPath(installID)
	if err != nil {
		return 0
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return serviceapps.UnitGeneration(string(raw))
}

// ServiceAppStatusV1 reads an install (read-only).
func (a *Agent) ServiceAppStatusV1(req *transport.ServiceAppStatusRequest, resp *transport.ServiceAppStatusResponse) error {
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	app, ok := serviceapps.Lookup(req.Target.AppID)
	layout, err := serviceapps.LayoutFor(req.Target.SubscriptionID, req.Target.DomainID, req.Target.AppID)
	if !ok || err != nil || req.Target.InstallID <= 0 {
		resp.Error = "an install and a catalog app are required"
		return nil
	}
	resp.Install, resp.Result = installUnitState(req.Target.InstallID)
	resp.InstallOperation = readServiceAppMarker(req.Target.InstallID).OperationID
	if req.Version != "" {
		_, statErr := os.Stat(serviceAppInstalledEntry(app, layout, req.Version))
		resp.Installed = statErr == nil
	}
	resp.UnitFile = serviceAppUnitFileState(req.Target.InstallID)
	resp.SandboxGeneration = serviceAppUnitGeneration(req.Target.InstallID)
	name := serviceapps.UnitName(req.Target.InstallID)
	out, err := appUnitSystemctl(systemctlArgs("show", name, "--property=LoadState,ActiveState,UnitFileState,MemoryCurrent,Result")...)
	if err != nil {
		resp.Error = "systemd could not be read: " + strings.TrimSpace(string(out))
		return nil
	}
	props := parseShow(string(out))
	if state, err := appUnitStateFromShow(string(out)); err == nil {
		resp.Unit = state
	}
	resp.ActiveState = props["ActiveState"]
	if props["Result"] != "" && props["Result"] != "success" {
		resp.Result = props["Result"]
	}
	if mem, err := strconv.ParseInt(props["MemoryCurrent"], 10, 64); err == nil && mem > 0 {
		resp.MemoryMB = mem / (1 << 20)
	}
	if resp.Unit.Active && req.Port > 0 {
		proof, err := serviceAppListenProof(name, req.Port)
		if err != nil {
			resp.Error = "the listening sockets could not be read: " + err.Error()
			return nil
		}
		resp.Serving, resp.Exposed, resp.Squatted = proof.Serving, proof.Exposed, proof.Squatted
		if req.Proof {
			var step transport.ServiceAppStepResponse
			if sc, ok := resolveServiceApp(req.Target, &step); ok {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				resp.SandboxProof = serviceAppSandboxProof(ctx, sc, req, proof)
				cancel()
			} else {
				resp.SandboxProof = &transport.SandboxProof{Code: "unreadable", Detail: step.Error}
			}
		}
	}
	return nil
}

func serviceAppListenProof(unit string, port int) (serviceapps.ListenProof, error) {
	pids, err := serviceAppUnitPIDs(unit)
	if err != nil {
		return serviceapps.ListenProof{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := serviceAppRun(ctx, "ss", "-Htlnp")
	if err != nil {
		return serviceapps.ListenProof{}, fmt.Errorf("ss: %v", err)
	}
	return serviceapps.ProveListening(serviceapps.ParseSS(string(out)), pids, port), nil
}

// ServiceAppActivateV1 writes the long-running unit of an installed version
// and starts it. A unit the owner changed is kept and not overwritten
// (D-022); the owner's own settings belong in the drop-in directory.
func (a *Agent) ServiceAppActivateV1(req *transport.ServiceAppActivateRequest, resp *transport.ServiceAppStepResponse) error {
	if req == nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "starting a service app"); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	sc, ok := resolveServiceApp(req.Target, resp)
	if !ok {
		return nil
	}
	if !validServiceAppPorts(sc.app, req.Port, req.AuxPort) || req.Version != sc.app.Version {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "the version or ports are not this release's")
		return nil
	}
	if _, err := os.Stat(serviceAppInstalledEntry(sc.app, sc.layout, req.Version)); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "install_missing", "the application files of this version are not in place; run the install again")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	runtime := serviceAppRuntime(ctx, sc.app)
	if !runtime.Available {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, runtime.Reason, runtime.Detail)
		return nil
	}
	path, err := serviceAppUnitPath(req.Target.InstallID)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "unit_dir", err.Error())
		return nil
	}
	caps := sandboxCaps(ctx)
	if err := ensureSandboxEgress(caps); err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "sandbox_egress", err.Error())
		return nil
	}
	text, err := serviceapps.RenderUnit(serviceapps.UnitParams{
		InstallID: req.Target.InstallID, App: sc.app, Version: req.Version, Domain: req.Target.Domain,
		User: sc.account.Name, Group: sc.account.Group, Layout: sc.layout, Port: req.Port, AuxPort: req.AuxPort,
		Runtime: runtime, Limits: serviceAppLimits(sc.app, req.Limits), Caps: caps, DropInDir: path + ".d",
	})
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "unit_render", err.Error())
		return nil
	}
	current, readErr := os.ReadFile(path)
	switch {
	case readErr == nil && string(current) == text:
	case readErr == nil && serviceapps.UnitTextState(string(current)) != "ours":
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "unit_edited",
			path+" was changed on the server, so CelikPanel kept it and did not write it again; move your changes to "+path+".d/*.conf, or remove the file, then try again")
		return nil
	case readErr != nil && !os.IsNotExist(readErr):
		serviceAppStep(resp, transport.ServiceAppUnknown, "unit_read", readErr.Error())
		return nil
	default:
		if err := os.WriteFile(path+".partial", []byte(text), 0o644); err != nil {
			serviceAppStep(resp, transport.ServiceAppUnknown, "unit_write", err.Error())
			return nil
		}
		if err := os.Rename(path+".partial", path); err != nil {
			serviceAppStep(resp, transport.ServiceAppUnknown, "unit_write", err.Error())
			return nil
		}
	}
	name := serviceapps.UnitName(req.Target.InstallID)
	steps := [][]string{{"daemon-reload"}, {"enable", name}, {"restart", name}}
	if req.KeepStopped {
		steps = [][]string{{"daemon-reload"}, {"stop", name}, {"disable", name}}
	}
	for _, step := range steps {
		if out, err := appUnitSystemctl(systemctlArgs(step...)...); err != nil {
			serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "systemctl",
				fmt.Sprintf("systemctl %s: %v: %s", strings.Join(step, " "), err, strings.TrimSpace(string(out))))
			return nil
		}
	}
	// The install unit has served its purpose.
	_, _ = appUnitSystemctl(systemctlArgs("stop", serviceapps.InstallUnitName(req.Target.InstallID))...)
	resp.Done = true
	return nil
}

// ServiceAppFinalizeV1 runs after the Panel proved the new version serves:
// earlier versions' folders and files are removed, and first-start-only
// values (Open WebUI's first password) leave the environment file.
func (a *Agent) ServiceAppFinalizeV1(req *transport.ServiceAppActivateRequest, resp *transport.ServiceAppStepResponse) error {
	if req == nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "finishing a service app install"); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	sc, ok := resolveServiceApp(req.Target, resp)
	if !ok {
		return nil
	}
	if req.Version != sc.app.Version {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "not this release's version")
		return nil
	}
	if _, err := serviceAppAsUser(sc.account, nil,
		`cd "$1" || exit 0; for d in app-*; do [ "$d" = "app-$2" ] || [ ! -e "$d" ] || rm -rf -- "$d"; done; for f in dist/*; do [ "$f" = "dist/$3" ] || [ ! -e "$f" ] || rm -f -- "$f"; done; rm -rf .cache`,
		sc.layout.Root, req.Version, sc.app.ArtifactFile); err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "prune", err.Error())
		return nil
	}
	if keys := serviceapps.FirstStartOnlyKeys(sc.app); len(keys) > 0 {
		existing, err := readServiceAppEnv(sc)
		if err != nil {
			serviceAppStep(resp, transport.ServiceAppUnknown, "env_file", err.Error())
			return nil
		}
		text, err := renderServiceAppEnv(sc, existing, req.Target.Domain, req.Port, req.AuxPort, "", "", nil, keys)
		if err == nil {
			err = writeServiceAppEnv(sc, text)
		}
		if err != nil {
			serviceAppStep(resp, transport.ServiceAppUnknown, "env_file", err.Error())
			return nil
		}
	}
	_ = os.Remove(filepath.Join(serviceAppStateDir(req.Target.InstallID), sc.app.ArtifactFile))
	resp.Done = true
	return nil
}

// ServiceAppControlV1 starts, stops or restarts an install's unit.
func (a *Agent) ServiceAppControlV1(req *transport.ServiceAppControlRequest, resp *transport.ServiceAppStepResponse) error {
	if req == nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "controlling a service app"); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	var steps [][]string
	name := serviceapps.UnitName(req.Target.InstallID)
	switch req.Action {
	case "start":
		steps = [][]string{{"enable", name}, {"start", name}}
	case "stop":
		steps = [][]string{{"stop", name}, {"disable", name}}
	case "restart":
		steps = [][]string{{"restart", name}}
	default:
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "action must be start, stop or restart")
		return nil
	}
	if req.Target.InstallID <= 0 {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "an install is required")
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	if req.Action != "stop" && serviceAppUnitFileState(req.Target.InstallID) == "absent" {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "unit_missing", "the unit file is missing; install the application again")
		return nil
	}
	for _, step := range steps {
		if out, err := appUnitSystemctl(systemctlArgs(step...)...); err != nil {
			serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "systemctl",
				fmt.Sprintf("systemctl %s: %v: %s", strings.Join(step, " "), err, strings.TrimSpace(string(out))))
			return nil
		}
	}
	resp.Done = true
	return nil
}

// ServiceAppLogsV1 returns the unit's (or the install step's) last journal
// lines with every secret of the environment file and every key-like value
// hidden.
func (a *Agent) ServiceAppLogsV1(req *transport.ServiceAppLogsRequest, resp *transport.ServiceAppLogsResponse) error {
	if req == nil || req.Target.InstallID <= 0 {
		resp.Error = "an install is required"
		return nil
	}
	lines := req.Lines
	if lines <= 0 || lines > 500 {
		lines = 100
	}
	name := serviceapps.UnitName(req.Target.InstallID)
	if req.Install {
		name = serviceapps.InstallUnitName(req.Target.InstallID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{"-u", name + ".service", "-n", strconv.Itoa(lines), "--no-pager", "-o", "short-iso"}
	if systemdUserMode {
		args = append([]string{"--user"}, args...)
	}
	out, err := serviceAppRun(ctx, "journalctl", args...)
	if err != nil {
		resp.Error = "the journal could not be read"
		return nil
	}
	var secrets []string
	var stepResp transport.ServiceAppStepResponse
	if sc, ok := resolveServiceApp(req.Target, &stepResp); ok {
		if env, err := readServiceAppEnv(sc); err == nil {
			secrets = serviceapps.SecretValues(sc.app, env)
		}
	}
	resp.Lines = []string{}
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "-- ") {
			continue
		}
		resp.Lines = append(resp.Lines, serviceapps.Redact(line, secrets))
	}
	return nil
}

// ServiceAppSecretsV1 replaces the customer's values or the owner's login in
// the environment file and restarts a running app so it reads them.
func (a *Agent) ServiceAppSecretsV1(req *transport.ServiceAppSecretsRequest, resp *transport.ServiceAppStepResponse) error {
	if req == nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a service app's keys"); err != nil {
		serviceAppStep(resp, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	sc, ok := resolveServiceApp(req.Target, resp)
	if !ok {
		return nil
	}
	if !validServiceAppPorts(sc.app, req.Port, req.AuxPort) {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "invalid_request", "the ports are outside the service-app range")
		return nil
	}
	existing, err := readServiceAppEnv(sc)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppUnknown, "env_file", err.Error())
		return nil
	}
	text, err := renderServiceAppEnv(sc, existing, req.Target.Domain, req.Port, req.AuxPort, req.OwnerEmail, req.OwnerSecret, req.Secrets, req.Clear)
	if err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "env_values", err.Error())
		return nil
	}
	if err := writeServiceAppEnv(sc, text); err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "env_file", err.Error())
		return nil
	}
	name := serviceapps.UnitName(req.Target.InstallID)
	if out, err := appUnitSystemctl(systemctlArgs("try-restart", name)...); err != nil {
		serviceAppStep(resp, transport.ServiceAppVerifiedFailure, "systemctl",
			fmt.Sprintf("the new values are saved, but the app could not be restarted: %v: %s", err, strings.TrimSpace(string(out))))
		return nil
	}
	resp.Done = true
	return nil
}

// ServiceAppRemoveV1 removes an install: unit (a copy is kept when the owner
// had changed it), application files and, only when asked, its data.
func (a *Agent) ServiceAppRemoveV1(req *transport.ServiceAppRemoveRequest, resp *transport.ServiceAppRemoveResponse) error {
	if req == nil {
		serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppVerifiedFailure, "invalid_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing a service app"); err != nil {
		serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppPrerequisite, "build_mismatch", err.Error())
		return nil
	}
	unlock := lockServiceApp(req.Target.InstallID)
	defer unlock()
	sc, ok := resolveServiceApp(req.Target, &resp.ServiceAppStepResponse)
	if !ok {
		return nil
	}
	name := serviceapps.UnitName(req.Target.InstallID)
	for _, unit := range []string{name, serviceapps.InstallUnitName(req.Target.InstallID)} {
		_, _ = appUnitSystemctl(systemctlArgs("stop", unit)...)
		_, _ = appUnitSystemctl(systemctlArgs("disable", unit)...)
		_, _ = appUnitSystemctl(systemctlArgs("reset-failed", unit)...)
	}
	path, err := serviceAppUnitPath(req.Target.InstallID)
	if err != nil {
		serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppUnknown, "unit_dir", err.Error())
		return nil
	}
	if raw, err := os.ReadFile(path); err == nil {
		if serviceapps.UnitTextState(string(raw)) != "ours" {
			copyPath := path + ".celikpanel-removed-" + time.Now().UTC().Format("20060102T150405Z")
			if err := os.WriteFile(copyPath, raw, 0o600); err != nil {
				serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppUnknown, "unit_copy", "a copy of the changed unit could not be kept, so it was not removed: "+err.Error())
				return nil
			}
			resp.KeptCopy = copyPath
		}
		if err := os.Remove(path); err != nil {
			serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppUnknown, "unit_remove", err.Error())
			return nil
		}
	}
	if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
		log.Printf("ServiceAppRemoveV1 install %d: daemon-reload: %v: %s", req.Target.InstallID, err, bytes.TrimSpace(out))
	}
	script := `cd "$1" 2>/dev/null || exit 0; rm -rf -- app-* dist config .cache`
	if req.RemoveData {
		script += `; rm -rf -- data; cd .. && rmdir -- "$2" 2>/dev/null; true`
	}
	if _, err := serviceAppAsUser(sc.account, nil, script, sc.layout.Root, sc.app.ID); err != nil {
		serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppUnknown, "files", "the unit is removed, but the application's files could not be removed: "+err.Error())
		return nil
	}
	_ = os.RemoveAll(serviceAppStateDir(req.Target.InstallID))
	state, err := inspectUnitSuspensionState(name)
	if err != nil || state.Active {
		serviceAppStep(&resp.ServiceAppStepResponse, transport.ServiceAppUnknown, "verify", "the unit's final state could not be confirmed")
		return nil
	}
	resp.Done = true
	return nil
}

// ServiceAppUnitStateV1 reads one install's unit (suspension before-image).
func (a *Agent) ServiceAppUnitStateV1(req *transport.ServiceAppUnitStateRequest, resp *transport.ServiceAppUnitStateResponse) error {
	if req == nil || req.InstallID <= 0 {
		resp.Error = "an install is required"
		return nil
	}
	state, err := inspectUnitSuspensionState(serviceapps.UnitName(req.InstallID))
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.State = state
	return nil
}

// SetServiceAppSuspensionV1 stops and disables an install's unit for a
// suspension, or restores the recorded state compare-and-set (D-022).
func (a *Agent) SetServiceAppSuspensionV1(req *transport.SetServiceAppSuspensionRequest, resp *transport.SetAppUnitSuspensionResponse) error {
	if req == nil || resp == nil {
		return errors.New("service app suspension request and response are required")
	}
	*resp = transport.SetAppUnitSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a service app"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if req.InstallID <= 0 {
		resp.Error = "a positive install identity is required; nothing was changed"
		return nil
	}
	unlock := lockServiceApp(req.InstallID)
	defer unlock()
	applyUnitSuspension(serviceapps.UnitName(req.InstallID), req.Suspend, req.Before, resp)
	return nil
}
