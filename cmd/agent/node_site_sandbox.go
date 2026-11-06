package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/nodesite"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Node.js sites in the site sandbox (alpha.127, docs/SITE-SANDBOX.md). The
// unit keeps its name, celikapp-<site id>. Which text ApplyAppUnit writes is
// decided here, from the request and the unit file on the server:
//
//   - an application that runs as the shared web account stays outside
//     the sandbox (the sandbox needs the site's own account; the account
//     move of D-038 §6 comes first);
//   - a new site (no unit file yet) and a unit that already carries the
//     sandbox get the sandbox;
//   - a unit an earlier release wrote keeps that release's text until the
//     administrator moves it (SandboxMove): it is never rewritten silently.
//
// A sandboxed unit is proved after every start (systemd's view, the SMTP
// set, the listening sockets). A move that the proof refutes, or after which
// an application that answered before no longer answers, is put back: the
// earlier unit is written and started again (resilience contract §4, a
// mutation with its recovery). A unit already in the sandbox that fails its
// proof is stopped and not published.
//
// Node.js siteleri site korumasında: birim adı aynı kalır. Önceki sürümün
// birimi yalnız yönetici taşıyınca değişir; taşıma kanıtlanır, kanıtlanamazsa
// eski birim geri yazılır ve başlatılır.

// nodeSitePlan is the decision for one ApplyAppUnit or InspectAppUnit.
type nodeSitePlan struct {
	sandboxed bool
	// move: the earlier release's unit is replaced by this request.
	move bool
	// reason: why the unit stays outside the sandbox ("" when sandboxed).
	reason  string
	account serviceAppAccount
	caps    sitesandbox.Caps
	params  nodesite.Params
	body    string
}

// nodeSiteAccountLookup proves that name is the site account whose home
// holds appDir (<home>/public_html); tests replace it.
var nodeSiteAccountLookup = func(name, appDir string) (serviceAppAccount, error) {
	return serviceAppLookupAccountByName(name, path.Dir(appDir))
}

// nodeSiteCaps reads the host's sandbox capabilities (a seam).
var nodeSiteCaps = func(ctx context.Context) sitesandbox.Caps { return sandboxCaps(ctx) }

// nodeSiteSettle bounds the wait for a started application to answer.
var nodeSiteSettle = 20 * time.Second

// nodeSiteReadUnit reads the unit file ("", false when there is none).
func nodeSiteReadUnit(unitPath string) (string, bool, error) {
	info, err := os.Lstat(unitPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !info.Mode().IsRegular() {
		return "", true, errors.New(unitPath + " is not a regular file")
	}
	raw, err := os.ReadFile(unitPath)
	if err != nil {
		return "", true, err
	}
	return string(raw), true, nil
}

// planNodeSite decides which text the request's unit gets. An error is a
// refusal: nothing was written.
func planNodeSite(ctx context.Context, req *AppApplyRequest, unitPath string) (nodeSitePlan, error) {
	if systemdUserMode {
		return nodeSitePlan{reason: "development"}, nil
	}
	text, exists, err := nodeSiteReadUnit(unitPath)
	if err != nil {
		return nodeSitePlan{}, fmt.Errorf("the application unit could not be read: %v", err)
	}
	current := exists && nodesite.Sandboxed(text)
	switch {
	case req.RunAsUser == "" || req.RunAsUser == "www-data":
		if current || req.SandboxMove {
			return nodeSitePlan{}, errors.New("shared_account: the application runs as the shared web account; the site sandbox needs the site's own account. Use \"Run under the site's own user\" on the site's page first")
		}
		return nodeSitePlan{reason: "shared_account"}, nil
	case exists && !current && !req.SandboxMove:
		return nodeSitePlan{reason: "earlier_release"}, nil
	}
	if err := nodesite.ValidAppDir(req.WorkDir); err != nil {
		if exists && !current && req.SandboxMove {
			return nodeSitePlan{}, fmt.Errorf("app_folder: %v; the site sandbox runs a Node.js site only from its document root", err)
		}
		return nodeSitePlan{}, fmt.Errorf("app_folder: %v", err)
	}
	account, err := nodeSiteAccountLookup(req.RunAsUser, req.WorkDir)
	if err != nil {
		return nodeSitePlan{}, fmt.Errorf("site_account: %v", err)
	}
	caps := nodeSiteCaps(ctx)
	params := nodesite.Params{
		SiteID: req.SiteID, Domain: strings.ToLower(strings.TrimSpace(req.Description)), User: account.Name, Group: account.Group,
		AppDir: req.WorkDir, Port: req.Port, NodeVersion: req.NodeVersion, Command: req.Command, Caps: caps,
		DropInDir: unitPath + ".d",
	}
	body, err := nodesite.RenderUnit(params)
	if err != nil {
		return nodeSitePlan{}, fmt.Errorf("unit_render: %v", err)
	}
	return nodeSitePlan{sandboxed: true, move: exists && !current, account: account, caps: caps, params: params, body: body}, nil
}

// nodeSiteListeners reads which sockets the unit's processes listen on.
func nodeSiteListeners(unit string) ([]sitesandbox.Listener, map[int]bool, error) {
	pids, err := serviceAppUnitPIDs(unit)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := serviceAppRun(ctx, "ss", "-Htlnp")
	if err != nil {
		return nil, nil, fmt.Errorf("ss: %v", err)
	}
	return sitesandbox.ParseSS(string(out)), pids, nil
}

// nodeSiteServing reports whether a process of the unit listens on port,
// on any address (an earlier release's unit may listen on every address).
func nodeSiteServing(unit string, port int) bool {
	listeners, pids, err := nodeSiteListeners(unit)
	if err != nil {
		return false
	}
	for _, l := range listeners {
		if l.Port != port {
			continue
		}
		for _, pid := range l.PIDs {
			if pids[pid] {
				return true
			}
		}
	}
	return false
}

// nodeSiteActive is the unit's ActiveState.
func nodeSiteActive(unit string) string {
	out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState", "--value")...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// nodeSiteVerdict is what the proof after a start found.
type nodeSiteVerdict struct {
	result transport.NodeSandboxResult
	// failed: the sandbox is refuted (a proof code other than not
	// answering); the unit must not stay published.
	failed bool
	// answering: the application runs and listens on its port.
	answering bool
}

// proveNodeSite waits up to nodeSiteSettle for the application to run and
// listen, then proves the unit. Not running or not listening is reported,
// not a refutation of the sandbox: a new site has no code yet.
func proveNodeSite(ctx context.Context, plan nodeSitePlan) nodeSiteVerdict {
	unit := nodesite.UnitName(plan.params.SiteID)
	verdict := nodeSiteVerdict{result: transport.NodeSandboxResult{Generation: sitesandbox.Generation, Level: string(sitesandbox.Assess(plan.caps).Level)}}
	deadline := time.Now().Add(nodeSiteSettle)
	var listen sitesandbox.ListenProof
	active := false
	for {
		if nodeSiteActive(unit) == "active" {
			active = true
			if listeners, pids, err := nodeSiteListeners(unit); err == nil {
				listen = sitesandbox.ProveNodeListening(listeners, pids, plan.params.Port, plan.caps.EgressEnforced())
				if listen.Serving || len(listen.Exposed) > 0 || listen.Squatted {
					break
				}
			}
		} else {
			active = false
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !active {
		verdict.result.Reason = "not_running"
		verdict.result.Detail = "the application is not running in the sandbox (journalctl -u " + unit + " says why)"
		return verdict
	}
	var listenArg *sitesandbox.ListenProof
	if listen.Serving || len(listen.Exposed) > 0 || listen.Squatted {
		listenArg = &listen
	}
	proof := sandboxProof(ctx, unit, plan.params.Profile(), plan.caps, listenArg)
	verdict.result.Proof = &transport.SandboxProof{OK: proof.OK, Code: proof.Code, Detail: proof.Detail}
	switch {
	case !proof.OK:
		verdict.failed = true
		verdict.result.Reason, verdict.result.Detail = proof.Code, proof.Detail
	case !listen.Serving:
		verdict.result.Reason = sitesandbox.ProofNotListening
		verdict.result.Detail = "the application runs but does not listen on port " + strconv.Itoa(plan.params.Port) + " yet"
	default:
		verdict.answering = true
	}
	return verdict
}

// nodeSitePrepare puts the helper, its template and the SMTP block in place
// and checks the document root as the helper will. A refusal is returned
// with its code.
func nodeSitePrepare(plan nodeSitePlan) error {
	if err := ensureSandboxHelper(); err != nil {
		return fmt.Errorf("sandbox_helper: %v", err)
	}
	if err := ensureSandboxEgress(plan.caps); err != nil {
		return fmt.Errorf("sandbox_egress: %v", err)
	}
	if code, detail := sandboxFolderRefusal(plan.params.AppDir, plan.account.UID); code != "" {
		return fmt.Errorf("%s: %s", code, detail)
	}
	return nil
}

// nodeSitePutBack writes the earlier release's unit text again (the D-031
// rule: it replaces CelikPanel's own sandbox text just written) and starts
// the application on it.
func nodeSitePutBack(req *AppApplyRequest, dir string, started bool) (transport.SiteFileResult, error) {
	legacy := appUnitFile(req, dir)
	result, err := services.ApplyManagedSiteFile(legacy, nil, func() error {
		if out, err := appUnitSystemctl(systemctlArgs("daemon-reload")...); err != nil {
			return fmt.Errorf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if !services.SiteFileApplied(result.Outcome) {
		return result, errors.New(appUnitHeldMessage(result))
	}
	if started {
		name := appUnitName(req.SiteID)
		for _, args := range [][]string{systemctlArgs("reset-failed", name), systemctlArgs("restart", name)} {
			if out, err := appUnitSystemctl(args...); err != nil && args[len(args)-2] != "reset-failed" {
				return result, fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
		}
	}
	return result, nil
}

// nodeSiteUnitGeneration reads the generation of a site's unit file
// (read-only): 0 for an earlier release's unit.
func nodeSiteUnitGeneration(siteID int) (int, bool) {
	dir, err := unitDir()
	if err != nil {
		return 0, false
	}
	text, exists, err := nodeSiteReadUnit(filepath.Join(dir, appUnitName(siteID)+".service"))
	if err != nil || !exists {
		return 0, false
	}
	return nodesite.UnitGeneration(text), true
}

// nodeSiteUnits lists the Node.js sites' unit files with their sandbox
// generation and whether they are CelikPanel's unchanged text (read-only).
func nodeSiteUnits() []transport.SandboxUnit {
	out := []transport.SandboxUnit{}
	dir, err := unitDir()
	if err != nil {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		name := entry.Name()
		rest, ok := strings.CutPrefix(name, "celikapp-")
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		idText, ok := strings.CutSuffix(rest, ".service")
		id, err := strconv.Atoi(idText)
		if !ok || err != nil || id <= 0 || strconv.Itoa(id) != idText {
			continue
		}
		full := filepath.Join(dir, name)
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		result, _, _ := services.InspectManagedSiteFile(services.ManagedSiteFile{
			Kind: transport.SiteFileKindAppUnit, Path: full, HeaderPrefix: services.ManagedHeaderPrefixHash,
		})
		file := "foreign"
		switch result.State {
		case transport.SiteFileManagedUnchanged:
			file = "ours"
		case transport.SiteFileOwnerEdited:
			file = "edited"
		}
		out = append(out, transport.SandboxUnit{Kind: "node", SiteID: id, Unit: name, File: file,
			Generation: nodesite.UnitGeneration(string(raw)), Active: nodeSiteActive(name)})
	}
	return out
}

// nodeSiteRefusalCode is the code at the front of a plan or prepare error.
func nodeSiteRefusalCode(err error) string {
	code, _, ok := strings.Cut(err.Error(), ":")
	if !ok || strings.ContainsAny(code, " ") {
		return "sandbox"
	}
	return code
}
