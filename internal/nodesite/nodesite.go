// Package nodesite renders a Node.js site's systemd units in the site
// sandbox (alpha.127, docs/SITE-SANDBOX.md): the long-running application
// celikapp-<site id> and its install step celikappinst-<site id> (npm ci or
// npm install, then npm run build when the project has one). Both run as the
// site's own account, with the site's document root as their only writable
// folder, the Node.js version the site chose as the only runtime they can
// see, HOME inside the document root and the one port the site was given.
//
// Every customer program is arbitrary code: the boundary is the sandbox
// (internal/sitesandbox), the same one service apps, Python apps and
// uploaded programs run in. The start command stays the customer's shell
// command line, as it was before alpha.127; it runs under /bin/sh as the
// site's account inside the sandbox, never as root.
//
// The package renders only native systemd text; it never touches the
// server. A unit an earlier release wrote (no sandbox marker) is not
// rewritten by this package's mere existence: the Agent writes the sandbox
// text only for a new site, for a unit that already carries it, or when the
// administrator moves the site (docs/SITE-SANDBOX.md §6).
//
// Paket nodesite, Node.js sitesinin systemd birimlerini site korumasında
// üretir: uzun süre çalışan uygulama ve kurulum adımı (npm). İkisi de sitenin
// kendi hesabıyla, yalnız belge kökünü yazabilen, yalnız seçilen Node.js
// sürümünü görebilen ve yalnız kendi portunu açabilen birimlerdir.
package nodesite

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Marker is the comment line that says a unit is a Node.js site's sandboxed
// unit; its value is the Node.js version the unit runs.
const Marker = "# celikpanel-node-site: "

// UnitName is a site's application unit (with ".service"). It is the same
// name the units of earlier releases had, so status, logs, start and stop
// keep working across the move.
func UnitName(siteID int) string { return fmt.Sprintf("celikapp-%d.service", siteID) }

// InstallUnitName is a site's transient install step (with ".service").
func InstallUnitName(siteID int) string { return fmt.Sprintf("celikappinst-%d.service", siteID) }

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// RuntimeDir is the folder of a Node.js version CelikPanel installed.
func RuntimeDir(version string) (string, error) {
	if !versionPattern.MatchString(version) {
		return "", fmt.Errorf("%q is not a Node.js version", version)
	}
	return path.Join(sitesandbox.RuntimesBase, "node", version), nil
}

// NodeBinary and NPMCLI are the version's own node and npm.
func NodeBinary(runtimeDir string) string { return path.Join(runtimeDir, "bin", "node") }
func NPMCLI(runtimeDir string) string {
	return path.Join(runtimeDir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
}

// HomeName is HOME's folder inside the document root: npm's cache and
// whatever a program keeps under ~ stay in the site's own folder.
const HomeName = ".home"

// HomeDir is HOME of a site's units.
func HomeDir(appDir string) string { return path.Join(appDir, HomeName) }

// ValidAppDir accepts only a site's document root in the hosting tree
// (<subscriptions>/<s>/sites/<d>/public_html), the folder the sandbox
// helper binds (sandboxbind.PlanFor). A Node.js site whose folder is
// anywhere else stays in its earlier protection and is listed.
func ValidAppDir(dir string) error {
	root := hostingpath.SubscriptionsRoot()
	clean := path.Clean(dir)
	if clean != dir || !strings.HasPrefix(clean, root+"/") {
		return fmt.Errorf("%s is not a site folder in the hosting tree", dir)
	}
	parts := strings.Split(strings.TrimPrefix(clean, root+"/"), "/")
	if len(parts) != 4 || !idPart.MatchString(parts[0]) || parts[1] != "sites" || !idPart.MatchString(parts[2]) || parts[3] != "public_html" {
		return fmt.Errorf("%s is not a site's document root", dir)
	}
	return sitesandbox.CheckValue(dir)
}

var idPart = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// RunLimits and InstallLimits are the units' resource limits in this
// release (a Node.js site had none before alpha.127). The install step gets
// more memory: a production build (Next.js, Vite) takes more than the
// running server. They are fixed here; per-plan limits are open.
var (
	RunLimits     = sitesandbox.Limits{MemoryMB: 1536, CPUPercent: 200, Tasks: 512, NOFILE: 8192}
	InstallLimits = sitesandbox.Limits{MemoryMB: 2048, CPUPercent: 200, Tasks: 512, NOFILE: 8192}
)

// InstallTimeoutSec bounds one install step.
const InstallTimeoutSec = 1800

var domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}\.)+[a-z0-9-]{1,63}$`)

// Params is everything a site's application unit is made of.
type Params struct {
	SiteID int
	// Domain names the unit (Description=, comments).
	Domain string
	User   string
	Group  string
	// AppDir is the site's document root (ValidAppDir), the only writable
	// folder and the working directory.
	AppDir      string
	Port        int
	NodeVersion string
	// Command is the customer's start command: a shell command line run
	// with /bin/sh -c as the site's account inside the sandbox.
	Command string
	Caps    sitesandbox.Caps
	// DropInDir is the owner's place (<unit>.d); CelikPanel never writes
	// or removes it.
	DropInDir string
}

func profile(kind sitesandbox.Kind, unit, user, group, appDir, version string, port int, limits sitesandbox.Limits, caps sitesandbox.Caps) sitesandbox.Profile {
	runtime, _ := RuntimeDir(version)
	return sitesandbox.Profile{
		Kind: kind, Unit: unit, User: user, Group: group, AppDir: appDir, RuntimeDir: runtime, Port: port, Limits: limits,
		// Node.js sites existed before levels: like the service apps they
		// run at the level this host enforces, and the Panel shows it
		// with its named gaps (Ubuntu 22.04 and 24.04 alike).
		Level: sitesandbox.Assess(caps).Level,
	}
}

// Profile is the application unit's sandbox profile.
func (p Params) Profile() sitesandbox.Profile {
	return profile(sitesandbox.KindNode, UnitName(p.SiteID), p.User, p.Group, p.AppDir, p.NodeVersion, p.Port, RunLimits, p.Caps)
}

// MaxCommand bounds the start command.
const MaxCommand = 4096

// ExecStartArg is the start command as one double-quoted word of a unit
// file's ExecStart=, so that /bin/sh receives exactly the text the customer
// typed. systemd resolves "%" specifiers on the whole line ("%%" is a "%"),
// unquotes and C-unescapes the word ("\\" and "\"" inside the quotes) and
// replaces "$NAME" with the unit's environment when the command runs ("$$"
// is a "$"). A line break or another control character is refused: it would
// end the directive.
func ExecStartArg(command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("the start command is empty")
	}
	if len(command) > MaxCommand {
		return "", fmt.Errorf("the start command is longer than %d characters", MaxCommand)
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range command {
		switch {
		case r < 0x20 || r == 0x7f:
			return "", errors.New("the start command has a line break or a control character")
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '$':
			b.WriteString("$$")
		case r == '%':
			b.WriteString("%%")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

func (p Params) validate() error {
	if p.SiteID <= 0 {
		return errors.New("a site identity is required")
	}
	if !domainPattern.MatchString(p.Domain) {
		return errors.New("invalid domain")
	}
	if err := ValidAppDir(p.AppDir); err != nil {
		return err
	}
	if p.Port < sitesandbox.NodePortMin || p.Port > sitesandbox.NodePortMax {
		return errors.New("the port must be between 1024 and 65535")
	}
	if _, err := RuntimeDir(p.NodeVersion); err != nil {
		return err
	}
	return sitesandbox.CheckValue(p.DropInDir)
}

// RenderUnit is the application unit's text (the D-031 header is added when
// the file is written). It refuses a host whose systemd version could not
// be read: the sandbox could not be proved there.
func RenderUnit(p Params) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	start, err := ExecStartArg(p.Command)
	if err != nil {
		return "", err
	}
	prof := p.Profile()
	if err := sitesandbox.Permit(prof, p.Caps); err != nil {
		return "", err
	}
	sandbox, err := sitesandbox.Directives(prof, p.Caps)
	if err != nil {
		return "", err
	}
	unitLines, pre, post, err := sitesandbox.HelperLines(prof, p.Caps)
	if err != nil {
		return "", err
	}
	runtime, _ := RuntimeDir(p.NodeVersion)
	unit := UnitName(p.SiteID)
	var b strings.Builder
	fmt.Fprintf(&b, "# The Node.js application of %s, written by CelikPanel (docs/SITE-SANDBOX.md).\n", p.Domain)
	fmt.Fprintf(&b, "# It runs in the site sandbox and without CelikPanel: systemctl status|stop|start %s ; journalctl -u %s\n", unit, unit)
	fmt.Fprintf(&b, "# Your own settings go in %s/*.conf (systemd drop-ins); CelikPanel never writes or removes them.\n", p.DropInDir)
	b.WriteString("# An edit to this file is kept: CelikPanel then shows the difference and asks before writing it again.\n")
	fmt.Fprintf(&b, "# Kendi ayarlarınız %s/*.conf içine; CelikPanel onlara dokunmaz. Bu dosyadaki bir değişiklik korunur.\n", p.DropInDir)
	fmt.Fprintf(&b, "%s%d\n", sitesandbox.GenerationMarker, sitesandbox.Generation)
	fmt.Fprintf(&b, "%s%s\n", Marker, p.NodeVersion)
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=CelikPanel app: %s\n", p.Domain)
	b.WriteString("After=network-online.target\nWants=network-online.target\n")
	for _, line := range append(sitesandbox.UnitDirectives(), unitLines...) {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Service]\nType=simple\n")
	fmt.Fprintf(&b, "User=%s\nGroup=%s\n", p.User, p.Group)
	// Resolved inside the sandbox, where the document root is the helper's
	// bind of the folder itself (never the site home's entry on the host).
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", p.AppDir)
	fmt.Fprintf(&b, "Environment=HOME=%s\n", HomeDir(p.AppDir))
	fmt.Fprintf(&b, "Environment=PATH=%s:/usr/bin:/bin\n", path.Join(runtime, "bin"))
	fmt.Fprintf(&b, "Environment=PORT=%d\n", p.Port)
	b.WriteString("Environment=HOST=127.0.0.1\nEnvironment=NODE_ENV=production\n")
	for _, line := range pre {
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "ExecStart=/bin/sh -c %s\n", start)
	for _, line := range post {
		b.WriteString(line + "\n")
	}
	b.WriteString("Restart=on-failure\nRestartSec=3\nTimeoutStopSec=30\n")
	for _, line := range sandbox {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return b.String(), nil
}

// UnitGeneration is the sandbox generation a Node.js unit's text declares:
// 0 for a unit without the marker (written before alpha.127, outside the
// sandbox). It is read, never trusted for safety: the proof checks what
// systemd loaded.
func UnitGeneration(text string) int {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, sitesandbox.GenerationMarker); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && n > 0 {
				return n
			}
			return 0
		}
	}
	return 0
}

// Sandboxed reports whether a unit text is this release's sandbox (or a
// later one).
func Sandboxed(text string) bool { return UnitGeneration(text) >= sitesandbox.Generation }
