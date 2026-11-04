package siteruntime

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// The build of a Python app (deploy step "build"): a transient unit
// celikrtbuild-<app id>, the site's own account, the same site sandbox as the
// app (no listening port; outbound network open for PyPI, the mail ports
// still blocked), Type=oneshot. Its steps are separate command lines, never
// a shell:
//
//	ExecStart     /usr/bin/python3.<m> -m venv <app>/venv-<n>.partial
//	ExecStartPost <venv>/bin/python -m pip install ... --require-hashes --no-deps -r <build dir>/server.txt
//	ExecStartPost <venv>/bin/python -m pip install ... -r <build dir>/requirements.txt [-c <build dir>/constraints.txt]
//	ExecStartPost <venv>/bin/python -m pip check
//
// The unit's ExecStart is the system's own interpreter: systemd-run looks
// up the executable of its command on the host before the unit exists
// ("Failed to find executable"), so the command can never be a path the
// build itself creates. The steps that run the new environment's python
// are ExecStartPost lines, which systemd runs in order after ExecStart
// succeeded; the first one that fails ends the build (Result=exit-code)
// and the rest do not run. The root egress ExecStartPre=+ still comes
// before every step (alpha.119 lab round 1: every build failed with the
// environment's python as the command).
//
// pip runs isolated (--isolated, PIP_CONFIG_FILE=/dev/null: no pip.conf of
// the customer's and no PIP_* variable changes the index), from
// https://pypi.org/simple only, wheels only unless the offering allows source
// builds, with its cache and temporary files inside the app folder. The
// files pip reads are written by the Agent as root into BuildDir/<app id>
// (root:<site group>, 0640): the checked, flattened requirements, never the
// customer's own file.
//
// Python derlemesi: geçici celikrtbuild-<id> birimi, sitenin kendi hesabı ve
// aynı site koruması; adımlar ayrı komut satırlarıdır, kabuk yoktur. Birimin
// ana komutu sistemin kendi Python'udur (ortamı kurar); yeni ortamın
// python'u ile çalışan adımlar ExecStartPost satırlarıdır. pip yalnız
// pypi.org'dan, açık izin yoksa yalnız hazır paketle kurar.

// PyPIIndex is the only index a build installs from.
const PyPIIndex = "https://pypi.org/simple"

// BuildDir holds each build's root-owned input files (tmpfs: a reboot
// removes them with the transient unit).
const BuildDir = "/run/celikpanel-rtbuild"

// BuildTimeoutSec bounds one build.
const BuildTimeoutSec = 1800

// BuildUnitName is an app's build unit (with ".service").
func BuildUnitName(appID int) string { return fmt.Sprintf("celikrtbuild-%d.service", appID) }

// BuildFilesDir is the folder of an app's build input files.
func BuildFilesDir(appID int) string { return path.Join(BuildDir, strconv.Itoa(appID)) }

// Build input file names inside BuildFilesDir.
const (
	BuildServerFile      = "server.txt"
	BuildRequirementFile = "requirements.txt"
	BuildConstraintFile  = "constraints.txt"
)

// BuildParams is everything a build unit is made of.
type BuildParams struct {
	AppID  int
	Domain string
	User   string
	Group  string
	Layout Layout
	Minor  string
	Venv   int
	Limits Limits
	Level  sitesandbox.Level
	Caps   sitesandbox.Caps
	// HasRequirements and HasConstraints: the checked files are written.
	HasRequirements bool
	HasConstraints  bool
	// Hashed: the customer's files carry hashes (--require-hashes).
	Hashed bool
	// SourceBuilds: the administrator allowed source builds for the
	// offering (off by default, named in the risk summary).
	SourceBuilds bool
}

// Profile is the build's sandbox profile: the app's, without a port.
func (p BuildParams) Profile() sitesandbox.Profile {
	return sitesandbox.Profile{
		Kind: sitesandbox.KindPython, Unit: BuildUnitName(p.AppID), User: p.User, Group: p.Group, AppDir: p.Layout.Root,
		Limits: sitesandbox.Limits{MemoryMB: p.Limits.MemoryMB, CPUPercent: p.Limits.CPUPercent, Tasks: p.Limits.Tasks, NOFILE: 8192},
		Level:  p.Level,
	}
}

func (p BuildParams) pipInstall() []string {
	out := []string{"-m", "pip", "install", "--isolated", "--no-input", "--disable-pip-version-check", "--no-warn-script-location",
		"--progress-bar=off", "--index-url=" + PyPIIndex, "--cache-dir=" + p.Layout.PipCacheDir()}
	if !p.SourceBuilds {
		out = append(out, "--only-binary=:all:")
	}
	return out
}

// BuildSteps are the build's command lines in order. The first one creates
// the environment with the system's interpreter and is the unit's command
// (ExecStart); the others run the new environment's python and are its
// ExecStartPost lines (see the package comment of this file).
func BuildSteps(p BuildParams) ([][]string, error) {
	if !ValidPythonMinor(p.Minor) || !PinnedMinor(p.Minor) {
		return nil, fmt.Errorf("Python %s has no app-server pins in this release", p.Minor)
	}
	if p.Venv <= 0 || p.Venv > MaxVenvGeneration {
		return nil, errors.New("invalid environment generation")
	}
	files := BuildFilesDir(p.AppID)
	venv := path.Join(p.Layout.Root, PartialVenvName(p.Venv))
	python := path.Join(venv, "bin", "python")
	steps := [][]string{{PythonBinary(p.Minor), "-m", "venv", venv}}
	server := append([]string{python}, p.pipInstall()...)
	// The pins are wheels: --only-binary always, whatever the offering says.
	if p.SourceBuilds {
		server = append(server, "--only-binary=:all:")
	}
	server = append(server, "--require-hashes", "--no-deps", "-r", path.Join(files, BuildServerFile))
	steps = append(steps, server)
	if p.HasRequirements {
		own := append([]string{python}, p.pipInstall()...)
		if p.Hashed {
			own = append(own, "--require-hashes")
		}
		own = append(own, "-r", path.Join(files, BuildRequirementFile))
		if p.HasConstraints {
			own = append(own, "-c", path.Join(files, BuildConstraintFile))
		}
		steps = append(steps, own)
	}
	steps = append(steps, []string{python, "-m", "pip", "check", "--disable-pip-version-check"})
	for _, step := range steps {
		for _, word := range step {
			if err := sitesandbox.CheckValue(word); err != nil {
				return nil, err
			}
		}
	}
	return steps, nil
}

// BuildRunArgs are systemd-run's arguments for the build (after the program
// name): the unit, the account, the environment, the sandbox, the SMTP
// egress lines, the later steps as ExecStartPost lines, then "--" and the
// environment's creation with the system's interpreter as the command. Nothing here holds "$" or "%" (CheckValue).
func BuildRunArgs(p BuildParams) ([]string, error) {
	if p.AppID <= 0 || !domainPattern.MatchString(p.Domain) {
		return nil, errors.New("an app identity and its domain are required")
	}
	if err := p.Limits.Validate(); err != nil {
		return nil, err
	}
	steps, err := BuildSteps(p)
	if err != nil {
		return nil, err
	}
	profile := p.Profile()
	if err := sitesandbox.Permit(profile, p.Caps); err != nil {
		return nil, err
	}
	// A transient unit: the systemd-run form of the sandbox (systemd 257's
	// systemd-run refuses ProtectControlGroups=strict).
	sandbox, err := sitesandbox.TransientDirectives(profile, p.Caps)
	if err != nil {
		return nil, err
	}
	unit := BuildUnitName(p.AppID)
	for _, value := range []string{p.Layout.Root, p.Layout.DataDir(), p.Layout.BuildTmpDir()} {
		if err := sitesandbox.CheckValue(value); err != nil {
			return nil, err
		}
	}
	args := []string{
		"--unit=" + unit, "--description=CelikPanel Python build of " + p.Domain,
		"--uid=" + p.User, "--gid=" + p.Group, "--working-directory=" + p.Layout.Root,
		"--setenv=HOME=" + p.Layout.DataDir(), "--setenv=PATH=/usr/bin:/bin", "--setenv=LC_ALL=C.UTF-8",
		"--setenv=TMPDIR=" + p.Layout.BuildTmpDir(), "--setenv=PIP_CONFIG_FILE=/dev/null",
		"--setenv=PYTHONNOUSERSITE=1", "--setenv=PYTHONDONTWRITEBYTECODE=1",
		"--property=Type=oneshot", "--property=RemainAfterExit=yes",
		"--property=TimeoutStartSec=" + strconv.Itoa(BuildTimeoutSec), "--no-block", "--quiet",
	}
	// Sandbox generation 3 (docs/SITE-SANDBOX.md §1a): the build binds the
	// app folder from its bind unit's root-owned mount point (Requires=,
	// After=), and its only root commands are the static helper's SMTP
	// egress calls, which start nft with an empty environment. The egress
	// ExecStartPre=+ runs before every step (ExecStart and ExecStartPost).
	unitLines, pre, post, err := sitesandbox.HelperLines(profile, p.Caps)
	if err != nil {
		return nil, err
	}
	for _, line := range append(append(append(unitLines, sandbox...), pre...), post...) {
		args = append(args, "--property="+line)
	}
	// Only the command is looked up by systemd-run before the unit starts:
	// it must already exist on the host, never inside the app folder the
	// build writes to.
	if strings.HasPrefix(steps[0][0], p.Layout.Root+"/") || !path.IsAbs(steps[0][0]) {
		return nil, errors.New("the build's command must be an existing system program")
	}
	for _, step := range steps[1:] {
		args = append(args, "--property=ExecStartPost="+strings.Join(step, " "))
	}
	args = append(args, "--")
	args = append(args, steps[0]...)
	return args, nil
}

// Build failure classification. pip says "Could not find a version that
// satisfies the requirement X" and "No matching distribution found for X"
// when, with --only-binary=:all:, X has no wheel for this Python (or no
// such package exists on PyPI); and it cannot reach the index when the
// network or PyPI fails.
var (
	pipNoVersion   = regexp.MustCompile(`(?:Could not find a version that satisfies the requirement|No matching distribution found for) ([A-Za-z0-9][A-Za-z0-9._\-\[\],<>=!~*]*)`)
	pipUnreachable = regexp.MustCompile(`(?i)(Temporary failure in name resolution|Failed to establish a new connection|Network is unreachable|Connection refused|Max retries exceeded with url|Read timed out|SSLError|CERTIFICATE_VERIFY_FAILED|Name or service not known)`)
	secretLike     = regexp.MustCompile(`(?i)(password|passwd|token|secret|api[_-]?key|access[_-]?key)(\s*[=:]\s*)\S+`)
)

// ClassifyBuildFailure reads a failed build's output: wheel_unavailable with
// the package, pypi_unreachable, or build_failed.
func ClassifyBuildFailure(lines []string) (reason, pkg string) {
	for _, line := range lines {
		if m := pipNoVersion.FindStringSubmatch(line); m != nil {
			return WheelUnavailable, RequirementName(m[1])
		}
	}
	for _, line := range lines {
		if pipUnreachable.MatchString(line) {
			return PyPIUnreachable, ""
		}
	}
	return BuildFailed, ""
}

// RedactBuildLine hides credentials in URLs and values that look secret.
func RedactBuildLine(line string) string {
	line = hideCredentials(line)
	return secretLike.ReplaceAllString(line, "${1}${2}***")
}

// BuildTail is the last lines of a build's output, redacted and bounded,
// for the operation's message.
func BuildTail(lines []string, max int, maxBytes int) []string {
	out := []string{}
	total := 0
	for i := len(lines) - 1; i >= 0 && len(out) < max; i-- {
		line := strings.TrimSpace(RedactBuildLine(lines[i]))
		if line == "" {
			continue
		}
		if len(line) > 300 {
			line = line[:300] + "…"
		}
		if total+len(line) > maxBytes {
			break
		}
		total += len(line) + 1
		out = append([]string{line}, out...)
	}
	return out
}
