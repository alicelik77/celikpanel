package siteruntime

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Python web apps (alpha.119, docs/RUNTIME-APPS.md "Python apps"): Django,
// Flask, FastAPI or any WSGI or ASGI application, run by the distribution's
// own Python in a virtual environment inside the app folder, served by a
// gunicorn (WSGI) or uvicorn (ASGI) that CelikPanel pins by hash.
//
// The uploaded code is arbitrary code, and so is every package pip installs.
// The boundary is the site sandbox, never the language: a Python app gets the
// same base sandbox as every customer program. It does not get
// MemoryDenyWriteExecute or NoExecPaths, because an interpreter executes its
// own extension modules and some of them generate code at run time; that is
// written in docs/SITE-SANDBOX.md and in the risk summary.
//
//	<site home>/app/
//	  src/          the uploaded code and its requirements.txt
//	  venv-<n>/     the environment of build n (the running one and the one before)
//	  venv-<n>.partial  a build in progress (removed when it fails)
//	  .pip-cache/   pip's download cache (counts toward the site's disk)
//	  .build-tmp/   pip's temporary folder during a build
//	  data/ config/ as for every runtime app
//
// Python uygulamaları: dağıtımın kendi Python'u, uygulama klasöründe bir sanal
// ortam, CelikPanel'in özetle sabitlediği gunicorn ya da uvicorn. Yüklenen kod
// ve pip'in kurduğu her paket rastgele koddur; sınır site korumasıdır.

// Python state and failure codes the screen words in English and Turkish
// (runtime.state.<code>). PythonStateCodes is the list the web test reads.
const (
	// PythonRuntimeMissing: the distribution's Python or its venv module is
	// not installed (the administrator installs Python in Components).
	PythonRuntimeMissing = "runtime_missing"
	// PythonSrcMissing: app/src is not on the server yet.
	PythonSrcMissing = "src_missing"
	// PythonVenvMissing: the environment the unit names is gone.
	PythonVenvMissing = "venv_missing"
	// RequirementsRefused: a requirements line CelikPanel does not install.
	RequirementsRefused = "requirements_refused"
	// WheelUnavailable: a package has no wheel for this Python on PyPI.
	WheelUnavailable = "wheel_unavailable"
	// BuildFailed: pip (or the venv step) stopped with an error.
	BuildFailed = "build_failed"
	// PyPIUnreachable: the build could not reach pypi.org (an external
	// provider's state, not a verdict on the app).
	PyPIUnreachable = "pypi_unreachable"
)

// PythonStateCodes are the Python states an app or a deploy can show.
var PythonStateCodes = []string{
	PythonRuntimeMissing, PythonSrcMissing, PythonVenvMissing, RequirementsRefused, WheelUnavailable, BuildFailed, PyPIUnreachable,
}

// App servers.
const (
	PythonWSGI = "wsgi"
	PythonASGI = "asgi"
	// MaxPythonWorkers is the most worker processes an app may run.
	MaxPythonWorkers = 8
	// PythonWorkerMemoryMB is the memory one worker is counted with when
	// the workers are capped by the memory limit.
	PythonWorkerMemoryMB = 128
)

// callablePattern is "module:callable": a dotted module path of identifiers
// and one identifier, nothing else (no factory call "app()", no expression).
var callablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}(\.[A-Za-z_][A-Za-z0-9_]{0,63}){0,7}:[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// ValidCallable reports whether s is "module:callable" (for example
// "mysite.wsgi:application" or "main:app").
func ValidCallable(s string) bool { return callablePattern.MatchString(s) }

// MaxWorkers is the workers an app may run under a memory limit: one per
// PythonWorkerMemoryMB, at least one, at most MaxPythonWorkers.
func MaxWorkers(memoryMB int) int {
	n := memoryMB / PythonWorkerMemoryMB
	if n < 1 {
		return 1
	}
	if n > MaxPythonWorkers {
		return MaxPythonWorkers
	}
	return n
}

// PythonSettings are a Python app's settings. The record keeps the callable
// in runtime_apps.entry and the server and workers as two argument tokens
// ("server=wsgi", "workers=2") in runtime_apps.args, so the Python kind fits
// migration 063's columns without a schema change.
type PythonSettings struct {
	Server   string `json:"server"`
	Callable string `json:"callable"`
	Workers  int    `json:"workers"`
}

// Validate checks the settings against the offering's memory limit.
func (s PythonSettings) Validate(limits Limits) error {
	if s.Server != PythonWSGI && s.Server != PythonASGI {
		return errors.New("the app server is wsgi (gunicorn: Django, Flask) or asgi (uvicorn: FastAPI, Starlette)")
	}
	if !ValidCallable(s.Callable) {
		return errors.New(`the app is "module:callable", for example mysite.wsgi:application or main:app (letters, digits, _ and dots in the module)`)
	}
	most := MaxWorkers(limits.MemoryMB)
	if s.Workers < 1 || s.Workers > most {
		return fmt.Errorf("workers must be 1 to %d (one per %d MB of the app's %d MB memory limit, at most %d)", most, PythonWorkerMemoryMB, limits.MemoryMB, MaxPythonWorkers)
	}
	return nil
}

// Args is the settings' argument tokens.
func (s PythonSettings) Args() []string {
	return []string{"server=" + s.Server, "workers=" + strconv.Itoa(s.Workers)}
}

// PythonSettingsFrom reads the settings back from the record.
func PythonSettingsFrom(entry string, args []string) (PythonSettings, error) {
	s := PythonSettings{Callable: entry}
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			return s, fmt.Errorf("unknown Python setting %q", arg)
		}
		switch key {
		case "server":
			s.Server = value
		case "workers":
			n, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("unknown worker count %q", value)
			}
			s.Workers = n
		default:
			return s, fmt.Errorf("unknown Python setting %q", arg)
		}
	}
	if s.Server == "" || s.Workers == 0 {
		return s, errors.New("the Python settings are incomplete")
	}
	return s, nil
}

// DistroPython is the Python one supported system ships.
type DistroPython struct {
	OSID      string // /etc/os-release ID
	VersionID string // /etc/os-release VERSION_ID ("" = rolling)
	Minor     string // "3.13"
}

// DistroPythons is the distribution Python of every supported system,
// verified against the distributions' package indexes on 2026-11-04
// (python3-defaults: bookworm 3.11.2, trixie 3.13.5; Ubuntu jammy 3.10,
// noble 3.12; Arch core/python 3.14.9). Only these are used: no
// deadsnakes, no pyenv, no python.org builds (security before freedom).
var DistroPythons = []DistroPython{
	{OSID: "debian", VersionID: "12", Minor: "3.11"},
	{OSID: "debian", VersionID: "13", Minor: "3.13"},
	{OSID: "ubuntu", VersionID: "22.04", Minor: "3.10"},
	{OSID: "ubuntu", VersionID: "24.04", Minor: "3.12"},
	{OSID: "arch", VersionID: "", Minor: "3.14"},
}

// ExpectedPython is the minor a system's own python3 is. A rolling system
// (Arch) answers rolling=true: its python3 moves with the distribution, and
// whatever minor /usr/bin/python3 names is the distribution's, provided this
// release pins an app server for it.
func ExpectedPython(osID, versionID string) (minor string, rolling bool, ok bool) {
	for _, d := range DistroPythons {
		if d.OSID != osID {
			continue
		}
		if d.VersionID == "" {
			return d.Minor, true, true
		}
		if d.VersionID == versionID {
			return d.Minor, false, true
		}
	}
	return "", false, false
}

// PythonPackages are the distribution packages of the Python component:
// apt's python3-venv pulls python3.<minor>-venv (ensurepip and the bundled
// pip wheel); Arch's python carries ensurepip itself.
func PythonPackages(family string) []string {
	switch family {
	case "apt":
		return []string{"python3", "python3-venv"}
	case "pacman":
		return []string{"python"}
	}
	return nil
}

var minorPattern = regexp.MustCompile(`^3\.(1[0-9]|[0-9])$`)

// ValidPythonMinor reports whether m is "3.<minor>".
func ValidPythonMinor(m string) bool { return minorPattern.MatchString(m) }

// PythonBinary is the distribution interpreter of a minor.
func PythonBinary(minor string) string { return "/usr/bin/python" + minor }

// PythonEnsurepip is the file that tells the venv module can install pip
// (on Debian and Ubuntu it is in python3.<minor>-venv).
func PythonEnsurepip(minor string) string {
	return "/usr/lib/python" + minor + "/ensurepip/__init__.py"
}

// PythonMinorFromLink reads the minor from /usr/bin/python3's link target
// ("python3.13" or "/usr/bin/python3.13").
func PythonMinorFromLink(target string) (string, bool) {
	base := path.Base(target)
	minor, ok := strings.CutPrefix(base, "python")
	if !ok || !ValidPythonMinor(minor) {
		return "", false
	}
	return minor, true
}

// PythonOfferingPrefix is the offering id prefix of the Python kind: it is
// offered per distribution Python, "component:python:<minor>", closed until
// the administrator offers it (like the other runtimes' version lines).
const PythonOfferingPrefix = "component:python:"

// PythonOffering is the offering id of a minor.
func PythonOffering(minor string) string { return PythonOfferingPrefix + minor }

// PythonMinorOfOffering reads the minor of an offering id.
func PythonMinorOfOffering(id string) (string, bool) {
	minor, ok := strings.CutPrefix(id, PythonOfferingPrefix)
	return minor, ok && ValidPythonMinor(minor)
}

// Layout parts of a Python app.
func (l Layout) SrcDir() string      { return path.Join(l.Root, "src") }
func (l Layout) PipCacheDir() string { return path.Join(l.Root, ".pip-cache") }
func (l Layout) BuildTmpDir() string { return path.Join(l.Root, ".build-tmp") }

// PythonUnit is what a Python app's unit adds to UnitParams.
type PythonUnit struct {
	Settings PythonSettings
	// Venv is the environment generation the unit runs (venv-<n>).
	Venv int
}

// PythonStartCommand is the ExecStart vector of a Python app: the venv's own
// interpreter running the pinned app server as a module. "python -m" and not
// venv-<n>/bin/gunicorn: the environment is built as venv-<n>.partial and
// renamed when complete, and a console script keeps the folder name it was
// installed under in its "#!" line; the interpreter link and pyvenv.cfg do
// not. No shell anywhere.
func PythonStartCommand(layout Layout, py PythonUnit, port int) ([]string, error) {
	if py.Venv <= 0 || py.Venv > MaxVenvGeneration {
		return nil, errors.New("the app has no built environment yet")
	}
	interpreter := path.Join(layout.Root, VenvName(py.Venv), "bin", "python")
	bind := strconv.Itoa(port)
	var command []string
	switch py.Settings.Server {
	case PythonWSGI:
		command = []string{interpreter, "-m", "gunicorn", "--bind", "127.0.0.1:" + bind, "--workers", strconv.Itoa(py.Settings.Workers), py.Settings.Callable}
	case PythonASGI:
		command = []string{interpreter, "-m", "uvicorn", "--host", "127.0.0.1", "--port", bind, "--workers", strconv.Itoa(py.Settings.Workers), py.Settings.Callable}
	default:
		return nil, errors.New("unknown app server")
	}
	if !ValidCallable(py.Settings.Callable) || py.Settings.Workers < 1 || py.Settings.Workers > MaxPythonWorkers {
		return nil, errors.New("invalid Python settings")
	}
	return command, nil
}

var execStartVenv = regexp.MustCompile(`^ExecStart=\S*/(venv-[0-9]{1,6})/bin/python `)

// ActiveVenv reads the environment generation a unit text runs (0: none).
func ActiveVenv(unitText string) int {
	for _, line := range strings.Split(unitText, "\n") {
		if m := execStartVenv.FindStringSubmatch(line); m != nil {
			if n, partial, ok := ParseVenvName(m[1]); ok && !partial {
				return n
			}
		}
	}
	return 0
}
