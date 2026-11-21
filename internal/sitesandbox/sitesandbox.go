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
	"github.com/alicelik/celikpanel/internal/panellisten"
)

// Generation is the sandbox text this release renders. A unit written by an
// earlier release carries no marker (generation 1: alpha.118's service apps)
// and is shown as "needs the new sandbox"; it is never rewritten silently.
// Generation 3 binds the app folder only through the bind unit's root-owned
// mount point (helper.go) and reads a service app's environment file only
// through its copy.
const Generation = 3

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
	// KindNode: a Node.js site (alpha.127): the customer's start command
	// under the Node.js version the site chose, in the site's document
	// root, and its install step (npm install and build).
	KindNode Kind = "node"
)

// RuntimesBase holds the Node.js versions CelikPanel installs
// (/opt/celikpanel/runtimes/node/<version>). A unit with a RuntimeDir sees
// only that one version of it: the tree is an empty read-only tmpfs with
// the version bound back in read-only.
const RuntimesBase = "/opt/celikpanel/runtimes"

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
	Kind Kind
	// Unit is the systemd unit's name (ValidUnitName); the helper's mount
	// point is named after it.
	Unit  string
	User  string
	Group string
	// AppDir is the only writable path; it is bound into an otherwise empty,
	// read-only hosting tree at its own path, from the helper's root-owned
	// mount point (never from the site home on the host).
	AppDir string
	// ReadOnlyPaths are further paths the program may read (for example the
	// distribution's Python); never inside the hosting tree.
	ReadOnlyPaths []string
	// RuntimeDir is the one runtime the program may run (a Node.js
	// version under RuntimesBase); every other version is hidden. "" for a
	// kind that brings its own (a service app's runtime is outside the
	// sandbox's view rule in this release).
	RuntimeDir string
	// Port is the loopback port the program listens on (0: it listens on
	// nothing, such as an install step). ExtraPorts are further loopback
	// ports (n8n's task broker).
	Port       int
	ExtraPorts []int
	Limits     Limits
	// Level is the level the administrator accepted for this unit. A host
	// that assesses below it is refused (Permit).
	Level Level
	// SiteID is the site the unit belongs to (sites.id; 0: the caller did
	// not say). InSlice places the unit in that site's slice (SliceName),
	// whose limits bound everything of the site together; false keeps
	// Slice=system.slice, where every unit ran before S4 R2a. The unit's own
	// Limits are not lowered to the slice's: systemd applies the most
	// stringent of the unit and its slice (EffectiveMemoryMax=,
	// systemd.resource-control(5)), docs/SITE-SANDBOX.md §8.
	SiteID  int
	InSlice bool
}

// SliceUnit is the slice the unit is placed in.
func (p Profile) SliceUnit() string {
	if p.InSlice && p.SiteID > 0 {
		return SliceName(p.SiteID)
	}
	return SystemSlice
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

// Node.js sites keep the port their administrator or customer chose
// (1024–65535, checked by the Panel against every other site's); the
// sandbox still lets the program bind only that one port.
const (
	NodePortMin = 1024
	NodePortMax = 65535
)

// NodeAssignMin and NodeAssignMax bound the port a Node.js site is given or
// may choose (security release, 20 November 2026): the range the panel has
// always allocated from. A site that kept an older port outside it keeps it,
// as long as NodePortRefusal accepts it.
const (
	NodeAssignMin = 3001
	NodeAssignMax = 3999
)

// NodePortRefusal says why port cannot be a Node.js site's port, or "" when
// it can: outside NodePortMin–NodePortMax, a port of this server's own
// services (panellisten.ServerPort), the range the service apps and the
// programs are given (PortMin–PortMax), or one of extra (the configured
// panel port). nginx passes the public to this port, so the Panel and the
// Agent both apply this rule; neither trusts the other's value.
func NodePortRefusal(port int, extra ...int) string {
	if port < NodePortMin || port > NodePortMax {
		return fmt.Sprintf("port %d is outside %d-%d", port, NodePortMin, NodePortMax)
	}
	if name, ok := panellisten.ServerPort(port); ok {
		return fmt.Sprintf("port %d belongs to %s", port, name)
	}
	if port >= PortMin && port <= PortMax {
		return fmt.Sprintf("port %d is in the range of the service apps and programs", port)
	}
	for _, other := range extra {
		if port == other {
			return fmt.Sprintf("port %d belongs to the panel", port)
		}
	}
	return ""
}

// PortRange is the range a kind's ports must lie in.
func PortRange(k Kind) (int, int) {
	if k == KindNode {
		return NodePortMin, NodePortMax
	}
	return PortMin, PortMax
}

// Validate checks a profile before anything is rendered.
func (p Profile) Validate() error {
	switch p.Kind {
	case KindServiceApp, KindPython, KindProgram, KindNode:
	default:
		return errors.New("unknown sandbox kind")
	}
	if !ValidUnitName(p.Unit) {
		return errors.New("the sandbox needs its unit's name")
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
	if p.RuntimeDir != "" {
		if err := CheckValue(p.RuntimeDir); err != nil {
			return err
		}
		if path.Clean(p.RuntimeDir) != p.RuntimeDir || !strings.HasPrefix(p.RuntimeDir, RuntimesBase+"/") {
			return fmt.Errorf("the runtime %s is not one CelikPanel installed", p.RuntimeDir)
		}
	}
	low, high := PortRange(p.Kind)
	ports := append([]int{p.Port}, p.ExtraPorts...)
	seen := map[int]bool{}
	for i, port := range ports {
		if i == 0 && port == 0 {
			continue
		}
		if port < low || port > high || seen[port] {
			return errors.New("a sandbox port is outside the sandbox range or repeated")
		}
		if p.Kind == KindNode && NodePortRefusal(port) != "" {
			return errors.New("a Node.js site's port belongs to another service of this server")
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
	if p.SiteID < 0 || p.SiteID > maxSiteID || (p.InSlice && p.SiteID == 0) {
		return errors.New("the site's slice needs the site's identity")
	}
	return nil
}

// ValidUnitName is a sandboxed unit's name: a service app's long-running or
// install unit, a runtime app's unit or its build (celikrtbuild, a Python
// app's environment build), or a Node.js site's unit or its install step
// (celikapp, celikappinst; alpha.127; celikgit, the Git deploy of alpha.128).
var unitNamePattern = regexp.MustCompile(`^(celiksvc|celiksvcinst|celikrt|celikrtbuild|celikapp|celikappinst|celikgit)-[1-9][0-9]{0,9}\.service$`)

// ValidUnitName reports whether name (with ".service") is a sandboxed unit.
func ValidUnitName(name string) bool { return unitNamePattern.MatchString(name) }
