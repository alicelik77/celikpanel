// Package sitesandbox is the one systemd sandbox every customer program runs
// in (alpha.119, docs/SITE-SANDBOX.md): the AI and automation service apps,
// Python apps and uploaded programs alike. Every program is treated as
// arbitrary code. The boundary is this sandbox, not which languages are
// installed on the server: an uploaded Go binary may start Python, so the
// sandbox, not the absence of an interpreter, is what has to hold.
//
// The package renders only native systemd and nftables text. The unit, the
// egress table and its guard keep working without CelikPanel (D-022): they
// are ordinary files an owner can read, change or remove.
//
// Paket sitesandbox, müşterinin her programının çalıştığı tek systemd
// korumasıdır. Her program rastgele kod sayılır; sınır bu korumadır, sunucuda
// hangi dillerin kurulu olduğu değildir.
package sitesandbox

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
)

// Generation is the sandbox text this release renders. A unit written by an
// earlier release carries no marker (generation 1: alpha.118's service apps)
// and is shown as "needs the new sandbox"; it is never rewritten silently.
const Generation = 2

// GenerationMarker is the comment line a rendered unit carries.
const GenerationMarker = "# celikpanel-sandbox-generation: "

// Kind is what runs in the sandbox.
type Kind string

const (
	// KindServiceApp: a catalog AI or automation app (n8n, Open WebUI).
	KindServiceApp Kind = "service_app"
	// KindPython: a customer's Python web app.
	KindPython Kind = "python"
	// KindProgram: an uploaded program (Go, Rust, anything executable).
	KindProgram Kind = "program"
)

// Level is how much of the sandbox this host enforces.
type Level string

const (
	// LevelFull: every protection of the base set is enforced.
	LevelFull Level = "full"
	// LevelReduced: at least one named protection is not enforced on this
	// host (Assessment.Gaps). A runtime is offered at this level only when
	// the administrator accepted the named gaps.
	LevelReduced Level = "reduced"
)

// Limits are the unit's resource limits.
type Limits struct {
	MemoryMB   int
	CPUPercent int
	Tasks      int
	NOFILE     int
}

// Profile is everything one sandboxed unit is made of.
type Profile struct {
	Kind  Kind
	User  string
	Group string
	// AppDir is the only writable path; it is bound into an otherwise empty,
	// read-only hosting tree.
	AppDir string
	// ReadOnlyPaths are further paths the program may read (for example the
	// distribution's Python); never inside the hosting tree.
	ReadOnlyPaths []string
	// Port is the loopback port the program listens on (0: it listens on
	// nothing, such as an install step). ExtraPorts are further loopback
	// ports (n8n's task broker).
	Port       int
	ExtraPorts []int
	Limits     Limits
	// Level is the level the administrator accepted for this unit. A host
	// that assesses below it is refused (Permit).
	Level Level
}

var (
	userPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	// valuePattern is the only text a sandbox value may contain. systemd
	// expands "%" specifiers in unit files (but not in transient
	// properties) and "$" in command lines; quotes, backslashes and spaces
	// split or unquote words. No escape is right for both a unit file and
	// systemd-run, so a value with any of them is refused, never escaped
	// (alpha.118's lesson in serviceapps.InstallCommand).
	valuePattern = regexp.MustCompile(`^[A-Za-z0-9._@:/+=-]+$`)
)

// CheckValue refuses a value that systemd would expand, unquote or split.
func CheckValue(value string) error {
	if value == "" {
		return errors.New("an empty sandbox value")
	}
	if !valuePattern.MatchString(value) {
		return fmt.Errorf("the sandbox value %q has characters systemd would expand or split", value)
	}
	return nil
}

// Port range a sandboxed program may listen on: the service apps' range
// (21000–29999) and the runtime's (30000–34999).
const (
	PortMin = 21000
	PortMax = 34999
)

// Validate checks a profile before anything is rendered.
func (p Profile) Validate() error {
	switch p.Kind {
	case KindServiceApp, KindPython, KindProgram:
	default:
		return errors.New("unknown sandbox kind")
	}
	if !userPattern.MatchString(p.User) || !userPattern.MatchString(p.Group) || p.User == "root" || p.Group == "root" {
		return errors.New("the sandbox needs the site's own account")
	}
	if err := CheckValue(p.AppDir); err != nil {
		return err
	}
	clean := path.Clean(p.AppDir)
	if clean != p.AppDir || !path.IsAbs(clean) || !hostingpath.IsSiteHome(clean) {
		return errors.New("the application folder is not inside a site home")
	}
	for _, ro := range p.ReadOnlyPaths {
		if err := CheckValue(ro); err != nil {
			return err
		}
		if !path.IsAbs(ro) || path.Clean(ro) != ro || ro == "/" || strings.HasPrefix(ro+"/", hostingpath.HostingBase()+"/") {
			return fmt.Errorf("the read-only path %s is not allowed", ro)
		}
	}
	ports := append([]int{p.Port}, p.ExtraPorts...)
	seen := map[int]bool{}
	for i, port := range ports {
		if i == 0 && port == 0 {
			continue
		}
		if port < PortMin || port > PortMax || seen[port] {
			return errors.New("a sandbox port is outside the sandbox range or repeated")
		}
		seen[port] = true
	}
	if p.Port == 0 && len(p.ExtraPorts) > 0 {
		return errors.New("extra ports need a main port")
	}
	l := p.Limits
	if l.MemoryMB < 64 || l.MemoryMB > 65536 || l.CPUPercent < 10 || l.CPUPercent > 1600 || l.Tasks < 16 || l.Tasks > 8192 {
		return errors.New("the resource limits are outside the sandbox range")
	}
	if l.NOFILE != 0 && (l.NOFILE < 256 || l.NOFILE > 65536) {
		return errors.New("the open-file limit is outside the sandbox range")
	}
	switch p.Level {
	case LevelFull, LevelReduced:
	default:
		return errors.New("the accepted sandbox level is missing")
	}
	return nil
}

// ValidUnitName is a sandboxed unit's name: a service app's long-running or
// install unit, or a runtime app's unit.
var unitNamePattern = regexp.MustCompile(`^(celiksvc|celiksvcinst|celikrt)-[1-9][0-9]{0,9}\.service$`)

// ValidUnitName reports whether name (with ".service") is a sandboxed unit.
func ValidUnitName(name string) bool { return unitNamePattern.MatchString(name) }
