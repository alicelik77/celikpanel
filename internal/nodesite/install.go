package nodesite

import (
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// The install step of a Node.js site (alpha.127): a transient unit
// celikappinst-<site id> made by systemd-run, the site's own account, the
// same sandbox as the application (no listening port; outbound network open
// for the npm registry, the mail ports still blocked), Type=oneshot. Its
// steps are separate command lines, never a shell of CelikPanel's:
//
//	ExecStart      <version>/bin/node <version>/lib/node_modules/npm/bin/npm-cli.js ci|install --no-audit --no-fund --foreground-scripts
//	ExecStartPost  <version>/bin/node <version>/lib/node_modules/npm/bin/npm-cli.js run build --if-present   (when asked)
//
// npm itself runs the project's own lifecycle scripts (postinstall, build)
// with /bin/sh as the site's account, inside the sandbox: they are the
// customer's code. npm reads no configuration of CelikPanel's; its cache is
// HOME/.npm inside the document root.
//
// Node.js sitesinin kurulum adımı: geçici celikappinst-<id> birimi, sitenin
// kendi hesabı ve uygulamayla aynı koruma; npm ci/install ve istenirse
// npm run build.

// InstallParams is everything an install step is made of.
type InstallParams struct {
	SiteID      int
	Domain      string
	User        string
	Group       string
	AppDir      string
	NodeVersion string
	// Clean: the project has a package-lock.json; npm ci installs exactly
	// what it names. Otherwise npm install.
	Clean bool
	// Build: run the project's build script afterwards (--if-present: a
	// project without one is not a failure).
	Build bool
	// Script (alpha.128, G19): run only this script of package.json
	// (`npm run <script>`), no install. It must be one of the project's
	// scripts (the Agent reads package.json) and a plain name.
	Script string
	Caps   sitesandbox.Caps
}

// scriptNamePattern is a package.json script name the step may run: no
// shell, no option, nothing systemd would expand.
var scriptNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._-]{0,63}$`)

// ValidScriptName says whether a script name can be run.
func ValidScriptName(name string) bool { return scriptNamePattern.MatchString(name) }

// PackageScripts lists the script names of a package.json text that can be
// run, sorted. A text that is not JSON, or has no scripts, lists none.
func PackageScripts(text []byte) []string {
	var pkg struct {
		Scripts map[string]json.RawMessage `json:"scripts"`
	}
	if len(text) > 1<<20 || json.Unmarshal(text, &pkg) != nil {
		return nil
	}
	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		if ValidScriptName(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Profile is the install step's sandbox profile: the application's,
// without a port, with the install limits.
func (p InstallParams) Profile() sitesandbox.Profile {
	return profile(sitesandbox.KindNode, InstallUnitName(p.SiteID), p.User, p.Group, p.AppDir, p.NodeVersion, 0, InstallLimits, p.Caps)
}

// InstallSteps are the step's command lines in order: the first is the
// unit's ExecStart, the rest its ExecStartPost lines (systemd runs them
// after it succeeded; the first that fails ends the step).
func InstallSteps(p InstallParams) ([][]string, error) {
	runtime, err := RuntimeDir(p.NodeVersion)
	if err != nil {
		return nil, err
	}
	node, npm := NodeBinary(runtime), NPMCLI(runtime)
	install := "install"
	if p.Clean {
		install = "ci"
	}
	// --foreground-scripts: the project's own install scripts write to the
	// step's journal, so the site's owner sees what they did.
	steps := [][]string{{node, npm, install, "--no-audit", "--no-fund", "--loglevel=warn", "--foreground-scripts"}}
	if p.Build {
		steps = append(steps, []string{node, npm, "run", "build", "--if-present"})
	}
	if p.Script != "" {
		// A chosen package script alone: npm runs it with /bin/sh as the
		// site's account inside the same sandbox as the install step.
		if !ValidScriptName(p.Script) {
			return nil, errors.New("the script name is not a plain package.json script name")
		}
		steps = [][]string{{node, npm, "run", p.Script}}
	}
	for _, step := range steps {
		for _, word := range step {
			if err := sitesandbox.CheckValue(word); err != nil {
				return nil, err
			}
		}
	}
	return steps, nil
}

// InstallRunArgs are systemd-run's arguments for the install step (after
// the program name): the unit, the account, the environment, the sandbox
// in systemd-run's form, the helper's lines, the later steps as
// ExecStartPost lines, then "--" and the first step. Nothing here holds "$"
// or "%" (CheckValue).
func InstallRunArgs(p InstallParams) ([]string, error) {
	if p.SiteID <= 0 || !domainPattern.MatchString(p.Domain) {
		return nil, errors.New("a site identity and its domain are required")
	}
	if err := ValidAppDir(p.AppDir); err != nil {
		return nil, err
	}
	steps, err := InstallSteps(p)
	if err != nil {
		return nil, err
	}
	prof := p.Profile()
	if err := sitesandbox.Permit(prof, p.Caps); err != nil {
		return nil, err
	}
	sandbox, err := sitesandbox.TransientDirectives(prof, p.Caps)
	if err != nil {
		return nil, err
	}
	unitLines, pre, post, err := sitesandbox.HelperLines(prof, p.Caps)
	if err != nil {
		return nil, err
	}
	runtime, _ := RuntimeDir(p.NodeVersion)
	home := HomeDir(p.AppDir)
	for _, value := range []string{home, runtime} {
		if err := sitesandbox.CheckValue(value); err != nil {
			return nil, err
		}
	}
	args := []string{
		"--unit=" + InstallUnitName(p.SiteID), "--description=CelikPanel Node.js install of " + p.Domain,
		"--uid=" + p.User, "--gid=" + p.Group, "--working-directory=" + p.AppDir,
		"--setenv=HOME=" + home, "--setenv=PATH=" + runtime + "/bin:/usr/bin:/bin", "--setenv=LC_ALL=C.UTF-8",
		"--setenv=npm_config_cache=" + home + "/.npm", "--setenv=npm_config_update_notifier=false",
		"--setenv=npm_config_fund=false", "--setenv=npm_config_audit=false",
		"--property=Type=oneshot", "--property=RemainAfterExit=yes",
		"--property=TimeoutStartSec=" + strconv.Itoa(InstallTimeoutSec), "--no-block", "--quiet",
	}
	for _, line := range append(append(append(unitLines, sandbox...), pre...), post...) {
		args = append(args, "--property="+line)
	}
	// Only the command is looked up by systemd-run before the unit starts:
	// it is the version's own node, root's, never a file of the site.
	if strings.HasPrefix(steps[0][0], p.AppDir+"/") || !strings.HasPrefix(steps[0][0], sitesandbox.RuntimesBase+"/") {
		return nil, errors.New("the install step's command must be the installed Node.js")
	}
	for _, step := range steps[1:] {
		args = append(args, "--property=ExecStartPost="+strings.Join(step, " "))
	}
	args = append(args, "--")
	args = append(args, steps[0]...)
	return args, nil
}

// Install phases, as the Agent records and reports them.
const (
	InstallRunning   = "running"
	InstallSucceeded = "succeeded"
	InstallFailed    = "failed"
)

// InstallOutcome reads the transient unit's state (systemctl show
// ActiveState, Result, Job) once the step was started: running while
// systemd runs it or still has its start job queued, succeeded or failed
// when it ended, and failed when the unit is gone without a result (a
// reboot removed it).
func InstallOutcome(found bool, activeState, result, job string) string {
	if !found {
		return InstallFailed
	}
	if job != "" && job != "0" {
		return InstallRunning
	}
	switch activeState {
	case "activating", "deactivating", "reloading":
		return InstallRunning
	case "active":
		if result == "" || result == "success" {
			return InstallSucceeded
		}
		return InstallFailed
	case "inactive":
		if result == "success" {
			return InstallSucceeded
		}
		return InstallFailed
	case "failed":
		return InstallFailed
	}
	return InstallRunning
}
