package siteruntime

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// UnitParams is everything an app's unit is made of.
type UnitParams struct {
	AppID  int
	Kind   Kind
	Domain string
	User   string
	Group  string
	Layout Layout
	// Entry is the customer's entry name under bin/ (ValidEntry).
	Entry string
	Args  []string
	Port  int
	// Limits are the offering's.
	Limits Limits
	// Level is the level the administrator accepted for the offering
	// (sitesandbox.Permit refuses a host below it).
	Level sitesandbox.Level
	Caps  sitesandbox.Caps
	// DropInDir is the owner's place (<unit>.d); CelikPanel never writes
	// or removes it.
	DropInDir string
	// Python is the Python kind's part (nil for an uploaded program).
	Python *PythonUnit
}

var domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}\.)+[a-z0-9-]{1,63}$`)

// Profile is the app's sandbox profile.
func (p UnitParams) Profile() sitesandbox.Profile {
	return sitesandbox.Profile{
		Kind: SandboxKind(p.Kind), User: p.User, Group: p.Group, AppDir: p.Layout.Root, Port: p.Port,
		Limits: sitesandbox.Limits{MemoryMB: p.Limits.MemoryMB, CPUPercent: p.Limits.CPUPercent, Tasks: p.Limits.Tasks, NOFILE: 8192},
		Level:  p.Level,
	}
}

// RuntimeMarker is the comment line that names a runtime unit's kind.
const RuntimeMarker = "# celikpanel-runtime-kind: "

// StartCommand is the ExecStart vector: the entry and the expanded
// arguments. There is no shell: systemd executes the entry itself.
func StartCommand(p UnitParams) ([]string, error) {
	if p.Kind == KindPython {
		if p.Python == nil {
			return nil, errors.New("the Python settings are missing")
		}
		command, err := PythonStartCommand(p.Layout, *p.Python, p.Port)
		if err != nil {
			return nil, err
		}
		for _, word := range command {
			if err := sitesandbox.CheckValue(word); err != nil {
				return nil, err
			}
		}
		return command, nil
	}
	if p.Kind != KindProgram {
		return nil, errors.New("unknown runtime kind")
	}
	entry, err := ValidEntry(p.Layout, p.Entry)
	if err != nil {
		return nil, err
	}
	args, err := ExpandArgs(p.Args, p.Port)
	if err != nil {
		return nil, err
	}
	command := append([]string{entry}, args...)
	for _, word := range command {
		if err := sitesandbox.CheckValue(word); err != nil {
			return nil, err
		}
	}
	return command, nil
}

// RenderUnit is the unit's text under CelikPanel's render header (the D-031
// header is added when the file is written). It refuses anything systemd
// would expand: a "$" or a "%" never reaches the text except CPUQuota's unit.
func RenderUnit(p UnitParams) (string, error) {
	if p.AppID <= 0 || !Implemented(p.Kind) {
		return "", errors.New("an app identity and an implemented kind are required")
	}
	if !domainPattern.MatchString(p.Domain) {
		return "", errors.New("invalid domain")
	}
	if !ValidPort(p.Port) {
		return "", errors.New("the port is outside the runtime range")
	}
	if err := p.Limits.Validate(); err != nil {
		return "", err
	}
	command, err := StartCommand(p)
	if err != nil {
		return "", err
	}
	if err := sitesandbox.Permit(p.Profile(), p.Caps); err != nil {
		return "", err
	}
	sandbox, err := sitesandbox.Directives(p.Profile(), p.Caps)
	if err != nil {
		return "", err
	}
	unit := UnitName(p.AppID)
	var pre, post []string
	if p.Caps.EgressEnforced() {
		if pre, post, err = sitesandbox.EgressExecLines(unit); err != nil {
			return "", err
		}
	}
	if err := sitesandbox.CheckValue(p.DropInDir); err != nil {
		return "", err
	}
	what, workdir, pathEnv := "Uploaded program", p.Layout.Root, p.Layout.BinDir()
	if p.Kind == KindPython {
		if err := p.Python.Settings.Validate(p.Limits); err != nil {
			return "", err
		}
		// The code's folder is the working folder, so "module:callable"
		// imports from app/src; the venv's bin comes first on PATH.
		what, workdir = "Python web app", p.Layout.SrcDir()
		pathEnv = path.Join(p.Layout.Root, VenvName(p.Python.Venv), "bin") + ":/usr/bin:/bin"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s of %s, written by CelikPanel (docs/RUNTIME-APPS.md).\n", what, p.Domain)
	fmt.Fprintf(&b, "# It runs without CelikPanel: systemctl status|stop|start %s ; journalctl -u %s\n", unit, unit)
	fmt.Fprintf(&b, "# Your own settings go in %s/*.conf; CelikPanel never writes or removes them,\n", p.DropInDir)
	b.WriteString("# but the Panel lists them, and its proof stops a deploy when the protections it reads differ.\n")
	fmt.Fprintf(&b, "# Kendi ayarlarınız %s/*.conf içine; CelikPanel onlara dokunmaz. Bu dosyadaki bir değişiklik korunur.\n", p.DropInDir)
	fmt.Fprintf(&b, "%s%d\n", sitesandbox.GenerationMarker, sitesandbox.Generation)
	fmt.Fprintf(&b, "%s%s\n", RuntimeMarker, p.Kind)
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=CelikPanel %s of %s\n", strings.ToLower(what), p.Domain)
	b.WriteString("After=network-online.target\nWants=network-online.target\n")
	for _, line := range sitesandbox.UnitDirectives() {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Service]\nType=simple\n")
	fmt.Fprintf(&b, "User=%s\nGroup=%s\n", p.User, p.Group)
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", workdir)
	fmt.Fprintf(&b, "EnvironmentFile=-%s\n", EnvFile(p.AppID))
	fmt.Fprintf(&b, "Environment=HOME=%s\n", p.Layout.DataDir())
	fmt.Fprintf(&b, "Environment=PATH=%s\n", pathEnv)
	if p.Kind == KindPython {
		b.WriteString("Environment=PYTHONUNBUFFERED=1\nEnvironment=PYTHONNOUSERSITE=1\n")
	}
	fmt.Fprintf(&b, "Environment=PORT=%d\n", p.Port)
	b.WriteString("Environment=HOST=127.0.0.1\n")
	for _, line := range pre {
		b.WriteString(line + "\n")
	}
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(command, " "))
	for _, line := range post {
		b.WriteString(line + "\n")
	}
	b.WriteString("Restart=on-failure\nRestartSec=5\nTimeoutStopSec=30\n")
	for _, line := range sandbox {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	text := b.String()
	if err := checkUnitText(text); err != nil {
		return "", err
	}
	return text, nil
}

// checkUnitText is the last guard: no "$" anywhere, no shell, and the only
// "%" is CPUQuota's (comments excepted, systemd does not expand them).
func checkUnitText(text string) error {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "$") {
			return fmt.Errorf("the unit line %q would be expanded by systemd", line)
		}
		if strings.Contains(line, "%") && !(strings.HasPrefix(line, "CPUQuota=") && strings.Count(line, "%") == 1) {
			return fmt.Errorf("the unit line %q has a specifier", line)
		}
		if rest, ok := strings.CutPrefix(line, "ExecStart="); ok {
			first, _, _ := strings.Cut(rest, " ")
			if first == "/bin/sh" || first == "/usr/bin/sh" || first == "/bin/bash" || first == "/usr/bin/bash" || strings.HasPrefix(first, "-") || strings.HasPrefix(first, "+") || strings.HasPrefix(first, "!") || strings.HasPrefix(first, "@") {
				return errors.New("a runtime unit starts the program itself, never a shell and never with a privilege prefix")
			}
		}
	}
	return nil
}

// UnitKind is the runtime kind a unit text declares ("" when none).
func UnitKind(text string) Kind {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, RuntimeMarker); ok {
			return Kind(strings.TrimSpace(rest))
		}
	}
	return ""
}

// HealthPathPattern is the shape of the health path (no query, no "%").
var healthPathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,200}$`)

// ValidHealthPath reports whether p is an acceptable health path.
func ValidHealthPath(p string) bool {
	return healthPathPattern.MatchString(p) && !strings.Contains(p, "..")
}
