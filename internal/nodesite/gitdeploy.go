package nodesite

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Git deploy of a site (alpha.128, PLESK-GAP G32): the site's document root
// follows one branch of a repository. One deploy is a transient unit
// celikgit-<site id>, the site's own account, the same sandbox as the Node.js
// install step (outbound network open, mail ports blocked, only the document
// root writable), Type=oneshot, its steps separate command lines and never a
// shell of CelikPanel's:
//
//	ExecStart      /usr/bin/git -C <root> init -q
//	ExecStartPost  /usr/bin/git -C <root> fetch -q --depth 1 -- <url> <branch>
//	ExecStartPost  /usr/bin/git -C <root> reset -q --hard FETCH_HEAD
//	ExecStartPost  (Node.js, when asked) npm ci|install, npm run build --if-present
//
// Only https:// and SSH repositories (GIT_ALLOW_PROTOCOL=https:ssh), never a
// password in the address. An SSH repository is read with the site's deploy
// key: root's file, handed to the unit as a systemd credential
// (LoadCredential=), so the site's account never reads it from disk. Files
// the repository does not track (uploads) stay; tracked files are replaced.
//
// Git ile yayın: sitenin belge kökü bir dalı izler; korumalı geçici birim,
// ayrı komut satırları, kabuk yok.

// GitUnitName is a site's deploy unit.
func GitUnitName(siteID int) string { return fmt.Sprintf("celikgit-%d.service", siteID) }

// GitBinary is the distribution's git (an official package).
const GitBinary = "/usr/bin/git"

// GitKeyCredential is the credential name of the deploy key in the unit.
const GitKeyCredential = "deploykey"

// GitCredentialMinSystemd: the first systemd whose transient units take
// LoadCredential= (lab-verify on every supported system).
const GitCredentialMinSystemd = 250

// GitDeployTimeoutSec bounds one deploy.
const GitDeployTimeoutSec = 1800

var (
	gitBranchPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)
	gitHostPattern    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,252}[A-Za-z0-9])?$`)
	gitPathPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,250}$`)
	gitSCPLikePattern = regexp.MustCompile(`^([a-z_][a-z0-9_-]{0,31})@([A-Za-z0-9.-]+):([A-Za-z0-9][A-Za-z0-9._/-]{0,250})$`)
)

// ValidGitBranch: a plain branch name (no option, no "..", no reflog syntax).
func ValidGitBranch(branch string) bool {
	return gitBranchPattern.MatchString(branch) && !strings.Contains(branch, "..") &&
		!strings.HasSuffix(branch, "/") && !strings.HasSuffix(branch, ".lock") && !strings.Contains(branch, "//")
}

// ParseGitURL checks a repository address and says whether it is SSH. Only
// https://host[:port]/path and SSH (user@host:path or ssh://user@host[:port]/path);
// never a password, a local path or another protocol.
func ParseGitURL(raw string) (ssh bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 300 || strings.HasPrefix(raw, "-") {
		return false, errors.New("a repository address is required")
	}
	if m := gitSCPLikePattern.FindStringSubmatch(raw); m != nil {
		if !gitHostPattern.MatchString(m[2]) || strings.Contains(m[3], "..") {
			return false, errors.New("the repository address is not a plain user@host:path")
		}
		return true, sitesandbox.CheckValue(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false, errors.New("the repository address cannot be read")
	}
	switch u.Scheme {
	case "https":
		if u.User != nil {
			return false, errors.New("an https address cannot hold a user or password; use SSH with the site's deploy key for a private repository")
		}
	case "ssh":
		if u.User == nil {
			return false, errors.New("an ssh:// address names its user (ssh://git@host/path)")
		}
		if _, hasPassword := u.User.Password(); hasPassword {
			return false, errors.New("an ssh address cannot hold a password; the site's deploy key is used")
		}
		ssh = true
	default:
		return false, errors.New("only https:// and SSH repositories can be used")
	}
	host := u.Hostname()
	if !gitHostPattern.MatchString(host) || u.RawQuery != "" || u.Fragment != "" {
		return false, errors.New("the repository address is not a plain host and path")
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return false, errors.New("the repository port is not a port")
		}
	}
	p := strings.TrimPrefix(u.Path, "/")
	if !gitPathPattern.MatchString(p) || strings.Contains(p, "..") {
		return false, errors.New("the repository path is not a plain path")
	}
	return ssh, sitesandbox.CheckValue(raw)
}

// GitDeployParams is everything a deploy is made of.
type GitDeployParams struct {
	SiteID  int
	Domain  string
	User    string
	Group   string
	AppDir  string
	RepoURL string
	Branch  string
	// KeyFile is the deploy key (root's, 0600) for an SSH repository.
	KeyFile string
	// NodeVersion and Build: a Node.js site's install and build after the
	// files are in place (Clean: the project has a lock file).
	NodeVersion string
	Build       bool
	Clean       bool
	Caps        sitesandbox.Caps
}

// Profile is the deploy's sandbox profile: the install step's, without a
// port; the Node.js version only when it builds.
func (p GitDeployParams) Profile() sitesandbox.Profile {
	version := ""
	if p.Build {
		version = p.NodeVersion
	}
	return profile(sitesandbox.KindNode, GitUnitName(p.SiteID), p.User, p.Group, p.AppDir, version, 0, InstallLimits, p.Caps)
}

// GitDeploySteps are the command lines in order.
func GitDeploySteps(p GitDeployParams) ([][]string, error) {
	ssh, err := ParseGitURL(p.RepoURL)
	if err != nil {
		return nil, err
	}
	if ssh && p.KeyFile == "" {
		return nil, errors.New("an SSH repository needs the site's deploy key; make one first")
	}
	if !ValidGitBranch(p.Branch) {
		return nil, errors.New("the branch is not a plain branch name")
	}
	steps := [][]string{
		{GitBinary, "-C", p.AppDir, "init", "-q"},
		{GitBinary, "-C", p.AppDir, "fetch", "-q", "--depth", "1", "--", strings.TrimSpace(p.RepoURL), p.Branch},
		{GitBinary, "-C", p.AppDir, "reset", "-q", "--hard", "FETCH_HEAD"},
	}
	if p.Build {
		runtime, err := RuntimeDir(p.NodeVersion)
		if err != nil {
			return nil, err
		}
		node, npm := NodeBinary(runtime), NPMCLI(runtime)
		verb := "install"
		if p.Clean {
			verb = "ci"
		}
		steps = append(steps,
			[]string{node, npm, verb, "--no-audit", "--no-fund", "--loglevel=warn", "--foreground-scripts"},
			[]string{node, npm, "run", "build", "--if-present"})
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

// GitDeployRunArgs are systemd-run's arguments for one deploy.
func GitDeployRunArgs(p GitDeployParams) ([]string, error) {
	if p.SiteID <= 0 || !domainPattern.MatchString(p.Domain) {
		return nil, errors.New("a site identity and its domain are required")
	}
	if err := ValidAppDir(p.AppDir); err != nil {
		return nil, err
	}
	steps, err := GitDeploySteps(p)
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
	home := HomeDir(p.AppDir)
	pathEnv := "/usr/bin:/bin"
	if p.Build {
		runtime, _ := RuntimeDir(p.NodeVersion)
		pathEnv = runtime + "/bin:" + pathEnv
	}
	unit := GitUnitName(p.SiteID)
	knownHosts := p.AppDir + "/.git/celikpanel_known_hosts"
	sshCommand := "ssh -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=" + knownHosts
	args := []string{
		"--unit=" + unit, "--description=CelikPanel Git deploy of " + p.Domain,
		"--uid=" + p.User, "--gid=" + p.Group, "--working-directory=" + p.AppDir,
		"--setenv=HOME=" + home, "--setenv=PATH=" + pathEnv, "--setenv=LC_ALL=C.UTF-8",
		"--setenv=GIT_TERMINAL_PROMPT=0", "--setenv=GIT_ALLOW_PROTOCOL=https:ssh",
		"--setenv=GIT_CONFIG_NOSYSTEM=1",
		"--setenv=npm_config_cache=" + home + "/.npm", "--setenv=npm_config_update_notifier=false",
		"--property=Type=oneshot", "--property=RemainAfterExit=yes",
		"--property=TimeoutStartSec=" + strconv.Itoa(GitDeployTimeoutSec), "--no-block", "--quiet",
	}
	if p.KeyFile != "" {
		// A transient unit takes LoadCredential= from systemd 250 on
		// (Ubuntu 22.04 has 249): there an SSH repository is refused
		// plainly instead of handing the key over any other way.
		if p.Caps.SystemdVersion < GitCredentialMinSystemd {
			return nil, fmt.Errorf("an SSH repository needs systemd %d or newer to hand the deploy key to the sandbox (this server has %d); use an https:// repository", GitCredentialMinSystemd, p.Caps.SystemdVersion)
		}
		if err := sitesandbox.CheckValue(p.KeyFile); err != nil {
			return nil, err
		}
		// systemd hands the key to the unit under /run/credentials/<unit>/,
		// readable by the unit's account only; the file on disk stays root's.
		sshCommand += " -i /run/credentials/" + unit + "/" + GitKeyCredential
		args = append(args, "--property=LoadCredential="+GitKeyCredential+":"+p.KeyFile)
	}
	args = append(args, "--setenv=GIT_SSH_COMMAND="+sshCommand)
	for _, line := range append(append(append(unitLines, sandbox...), pre...), post...) {
		args = append(args, "--property="+line)
	}
	for _, step := range steps[1:] {
		args = append(args, "--property=ExecStartPost="+strings.Join(step, " "))
	}
	args = append(args, "--")
	args = append(args, steps[0]...)
	return args, nil
}
