package serviceapps

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
)

// Layout is where one install lives: inside the site's own home, beside (not
// inside) public_html, so its files are the site's, its size is counted in the
// subscription's disk usage and nginx never serves them.
//
//	<site home>/apps/<app>/
//	  dist/          the verified upstream file
//	  app-<version>/ the application (npm prefix or Python venv)
//	  data/          the application's own data
//
// The environment file is not in the site's folder: systemd (PID 1) reads an
// EnvironmentFile= as root and follows links, so a file the site's user can
// replace could have been a link to a root-only file whose values would then
// reach the app's process. It lives in EnvDir, root's (alpha.118 security fix).
type Layout struct {
	Root string
}

// LayoutFor derives the layout from database identities only.
func LayoutFor(subscriptionID, domainID int, appID string) (Layout, error) {
	if !Known(appID) {
		return Layout{}, errors.New("unknown application")
	}
	home, err := hostingpath.SiteHome(subscriptionID, domainID)
	if err != nil {
		return Layout{}, err
	}
	return Layout{Root: path.Join(home, "apps", appID)}, nil
}

func (l Layout) DistDir() string   { return path.Join(l.Root, "dist") }
func (l Layout) DataDir() string   { return path.Join(l.Root, "data") }
func (l Layout) ConfigDir() string { return path.Join(l.Root, "config") }

// LegacyEnvFile is where alpha.118 builds before the security fix kept the
// environment file; it is read only to move it (as the site's user, never
// through a link) and then removed.
func (l Layout) LegacyEnvFile() string { return path.Join(l.Root, "config", "app.env") }
func (l Layout) CacheDir() string      { return path.Join(l.Root, ".cache") }
func (l Layout) InstallDir(version string) string {
	return path.Join(l.Root, "app-"+version)
}

var unitNamePattern = regexp.MustCompile(`^celiksvc-[1-9][0-9]{0,9}$`)

// UnitName is the long-running unit of an install.
func UnitName(installID int) string { return fmt.Sprintf("celiksvc-%d", installID) }

// InstallUnitName is the transient unit that runs npm or pip for an install.
func InstallUnitName(installID int) string { return fmt.Sprintf("celiksvcinst-%d", installID) }

// EnvDir holds the service apps' environment files: root's, 0700; each file
// root's, 0600. It is native configuration and stays without CelikPanel.
// The runtime apps of alpha.119 use /etc/celikpanel-runtime the same way.
const EnvDir = "/etc/celikpanel-svcapps"

// EnvFile is the environment file of an install's unit.
func EnvFile(installID int) string { return path.Join(EnvDir, UnitName(installID)+".env") }

// ValidUnitName reports whether name is a service-app unit name.
func ValidUnitName(name string) bool { return unitNamePattern.MatchString(name) }

// Ports: every install listens on 127.0.0.1 only, on a port the Panel
// allocates from this range (n8n also takes the next one for its task broker).
const (
	PortMin = 21000
	PortMax = 29998
)

// UnitParams is everything the unit text is made of.
type UnitParams struct {
	InstallID int
	App       App
	Version   string
	Domain    string
	User      string
	Group     string
	Layout    Layout
	Port      int
	AuxPort   int
	Runtime   RuntimeChoice
	Limits    Resources
	// Caps is what this host enforces (the Agent's sandbox probe).
	Caps sitesandbox.Caps
	// DropInDir is the owner's place (<unit>.service.d); CelikPanel never
	// writes or removes it.
	DropInDir string
}

var (
	safeValue    = regexp.MustCompile(`^[A-Za-z0-9._@:/+=-]+$`)
	userPattern  = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	versionShape = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]{0,40}$`)
)

func (p UnitParams) validate() error {
	if p.InstallID <= 0 || !p.App.Installable() {
		return errors.New("an installable application and a positive install identity are required")
	}
	if !versionShape.MatchString(p.Version) {
		return errors.New("invalid version")
	}
	if !userPattern.MatchString(p.User) || !userPattern.MatchString(p.Group) {
		return errors.New("invalid site account")
	}
	if !hostingpath.IsSiteHome(p.Layout.Root) || !safeValue.MatchString(p.Layout.Root) {
		return errors.New("the application folder is not inside a site home")
	}
	if p.Port < PortMin || p.Port > PortMax || (p.App.AuxPort && (p.AuxPort < PortMin || p.AuxPort > PortMax+1 || p.AuxPort == p.Port)) {
		return errors.New("the application port is outside the service-app range")
	}
	if !p.Runtime.Available || !safeValue.MatchString(p.Runtime.Path) || !path.IsAbs(p.Runtime.Path) {
		return errors.New("no usable runtime")
	}
	if p.Limits.MemoryMB < 128 || p.Limits.CPUPercent < 10 || p.Limits.TasksMax < 16 {
		return errors.New("resource limits are too small")
	}
	if p.Domain == "" || !safeValue.MatchString(p.Domain) {
		return errors.New("invalid domain")
	}
	return nil
}

// SandboxProfile is a service app's sandbox profile (alpha.119: every
// service-app process, the long-running unit and the transient install unit
// alike, runs in the shared site sandbox, internal/sitesandbox). The process
// is the site's own user; it sees the hosting tree only through its own
// application folder, cannot gain privileges, sees no other user's
// processes, cannot reach the Agent's or any PHP-FPM socket, the metadata
// address or a mail port, binds only its own loopback ports and writes only
// to its own folder (and its private /tmp). port is 0 for the install unit.
func SandboxProfile(layout Layout, user, group string, limits Resources, memoryMB, port, aux int, caps sitesandbox.Caps) sitesandbox.Profile {
	profile := sitesandbox.Profile{
		Kind: sitesandbox.KindServiceApp, User: user, Group: group, AppDir: layout.Root, Port: port,
		Limits: sitesandbox.Limits{MemoryMB: memoryMB, CPUPercent: limits.CPUPercent, Tasks: limits.TasksMax, NOFILE: 8192},
		// The service apps were offered before levels existed: they run at
		// what this host enforces, and the Panel shows the level.
		Level: sitesandbox.Assess(caps).Level,
	}
	if port != 0 && aux != 0 {
		profile.ExtraPorts = []int{aux}
	}
	return profile
}

// SandboxDirectives are the systemd settings of a service-app process on a
// host with caps.
func SandboxDirectives(profile sitesandbox.Profile, caps sitesandbox.Caps) ([]string, error) {
	return sitesandbox.Directives(profile, caps)
}

// InstallUnitProperties are the --property values of the transient install
// unit: the sandbox and, where the host can hold it, the SMTP block (npm and
// pip run third-party install scripts).
func InstallUnitProperties(installID int, layout Layout, user, group string, limits Resources, memoryMB int, caps sitesandbox.Caps) ([]string, error) {
	directives, err := SandboxDirectives(SandboxProfile(layout, user, group, limits, memoryMB, 0, 0, caps), caps)
	if err != nil {
		return nil, err
	}
	if caps.EgressEnforced() {
		pre, post, err := sitesandbox.EgressExecLines(InstallUnitName(installID) + ".service")
		if err != nil {
			return nil, err
		}
		directives = append(directives, pre...)
		directives = append(directives, post...)
	}
	return directives, nil
}

// StartCommand is the ExecStart argument vector.
func StartCommand(p UnitParams) []string {
	dir := p.Layout.InstallDir(p.Version)
	switch p.App.ID {
	case "n8n":
		return []string{p.Runtime.Path, path.Join(dir, "node_modules", "n8n", "bin", "n8n"), "start"}
	case "open-webui":
		return []string{path.Join(dir, "bin", "open-webui"), "serve", "--host", "127.0.0.1", "--port", strconv.Itoa(p.Port)}
	}
	return nil
}

// RenderHeader is the first line of a unit CelikPanel wrote: the hash of the
// text under it. A unit whose text no longer matches was changed by the owner
// and is kept (D-022, D-031): CelikPanel then refuses to overwrite it.
const RenderHeader = "# celikpanel-render-sha256: "

// RenderUnit is the unit file text, header included.
func RenderUnit(p UnitParams) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	command := StartCommand(p)
	if len(command) == 0 {
		return "", errors.New("the application has no start command")
	}
	for _, part := range command {
		if !safeValue.MatchString(part) {
			return "", errors.New("the start command has unsafe characters")
		}
	}
	name := UnitName(p.InstallID)
	sandbox, err := SandboxDirectives(SandboxProfile(p.Layout, p.User, p.Group, p.Limits, p.Limits.MemoryMB, p.Port, p.AuxPort, p.Caps), p.Caps)
	if err != nil {
		return "", err
	}
	if p.Caps.EgressEnforced() {
		pre, post, err := sitesandbox.EgressExecLines(name + ".service")
		if err != nil {
			return "", err
		}
		sandbox = append(append(pre, sandbox...), post...)
	}
	pathEnv := path.Dir(p.Runtime.Path) + ":/usr/local/bin:/usr/bin:/bin"
	if p.App.Runtime == RuntimePython {
		pathEnv = path.Join(p.Layout.InstallDir(p.Version), "bin") + ":/usr/local/bin:/usr/bin:/bin"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s %s for %s, written by CelikPanel (docs/AI-APPS.md).\n", p.App.Name, p.Version, p.Domain)
	fmt.Fprintf(&b, "# It runs without CelikPanel: systemctl status|stop|start %s ; journalctl -u %s\n", name, name)
	fmt.Fprintf(&b, "# Your own settings go in %s/*.conf; CelikPanel never writes or removes them.\n", p.DropInDir)
	fmt.Fprintf(&b, "# Kendi ayarlarınız %s/*.conf içine; CelikPanel onlara dokunmaz. Bu dosyadaki bir değişiklik korunur.\n", p.DropInDir)
	fmt.Fprintf(&b, "%s%d\n", sitesandbox.GenerationMarker, sitesandbox.Generation)
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=CelikPanel service app %s for %s\n", p.App.Name, p.Domain)
	b.WriteString("After=network-online.target\nWants=network-online.target\n")
	for _, line := range sitesandbox.UnitDirectives() {
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString("[Service]\nType=simple\n")
	fmt.Fprintf(&b, "User=%s\nGroup=%s\n", p.User, p.Group)
	fmt.Fprintf(&b, "WorkingDirectory=%s\n", p.Layout.Root)
	fmt.Fprintf(&b, "EnvironmentFile=%s\n", EnvFile(p.InstallID))
	fmt.Fprintf(&b, "Environment=HOME=%s\n", p.Layout.DataDir())
	fmt.Fprintf(&b, "Environment=PATH=%s\n", pathEnv)
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(command, " "))
	b.WriteString("Restart=on-failure\nRestartSec=10\nTimeoutStopSec=30\n")
	for _, directive := range sandbox {
		b.WriteString(directive)
		b.WriteByte('\n')
	}
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return Seal(b.String()), nil
}

// Seal puts the render header (the hash of body) above body. The unit and
// the environment file both carry it, so an owner's edit of either is
// detected and kept (D-022, D-031).
func Seal(body string) string { return RenderHeader + BodyHash(body) + "\n" + body }

// BodyHash is the hex SHA-256 of a unit's text under its header.
func BodyHash(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// UnitGeneration is the sandbox generation a unit text declares: 1 for a
// unit without the marker (alpha.118), the marker's number otherwise. It is
// read, never trusted for safety: Prove checks what systemd loaded.
func UnitGeneration(text string) int {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(line, sitesandbox.GenerationMarker); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && n > 0 {
				return n
			}
			return 1
		}
	}
	return 1
}

// UnitTextState classifies a unit file CelikPanel finds: "ours" (header
// matches its body), "edited" (header present, body changed) or "foreign"
// (no header: not CelikPanel's).
func UnitTextState(text string) string {
	first, body, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, RenderHeader) {
		return "foreign"
	}
	if strings.TrimPrefix(first, RenderHeader) == BodyHash(body) {
		return "ours"
	}
	return "edited"
}

// EnvParams is what the environment file is made of. Fixed values are
// CelikPanel's (listen address, disabled features); Generated are made once
// on the server and kept; Secrets are the customer's.
type EnvParams struct {
	App     App
	Domain  string
	Layout  Layout
	Port    int
	AuxPort int
	// Owner is the app's first administrator. For n8n OwnerSecret is a
	// bcrypt hash (n8n's own owner-by-environment setting); for Open WebUI
	// it is the first password, removed again after the first start.
	OwnerEmail  string
	OwnerSecret string
	Generated   map[string]string
	Secrets     map[string]string
}

// n8nExcludedNodes are the nodes that act on the host itself: Execute
// Command runs a shell command, Local File Trigger watches the server's
// files. n8n excludes both by default since 2.0; the list is written
// explicitly so an upstream change of the default does not reopen them.
var n8nExcludedNodes = `["n8n-nodes-base.executeCommand","n8n-nodes-base.localFileTrigger"]`

// FixedEnv is CelikPanel's part of the environment.
func FixedEnv(p EnvParams) map[string]string {
	base := "https://" + p.Domain
	switch p.App.ID {
	case "n8n":
		return map[string]string{
			"NODE_ENV":                              "production",
			"N8N_LISTEN_ADDRESS":                    "127.0.0.1",
			"N8N_PORT":                              strconv.Itoa(p.Port),
			"N8N_HOST":                              p.Domain,
			"N8N_PROTOCOL":                          "https",
			"N8N_EDITOR_BASE_URL":                   base + "/",
			"N8N_WEBHOOK_URL":                       base + "/",
			"N8N_PROXY_HOPS":                        "1",
			"N8N_SECURE_COOKIE":                     "true",
			"N8N_USER_FOLDER":                       p.Layout.DataDir(),
			"N8N_RUNNERS_BROKER_LISTEN_ADDRESS":     "127.0.0.1",
			"N8N_RUNNERS_BROKER_PORT":               strconv.Itoa(p.AuxPort),
			"NODES_EXCLUDE":                         n8nExcludedNodes,
			"N8N_BLOCK_ENV_ACCESS_IN_NODE":          "true",
			"N8N_BLOCK_FILE_ACCESS_TO_N8N_FILES":    "true",
			"N8N_RESTRICT_FILE_ACCESS_TO":           path.Join(p.Layout.DataDir(), "files"),
			"N8N_ENFORCE_SETTINGS_FILE_PERMISSIONS": "true",
			"N8N_COMMUNITY_PACKAGES_ENABLED":        "false",
			"N8N_PYTHON_ENABLED":                    "false",
			"N8N_GIT_NODE_ENABLE_HOOKS":             "false",
			"N8N_GIT_NODE_DISABLE_BARE_REPOS":       "true",
			"N8N_DIAGNOSTICS_ENABLED":               "false",
			"N8N_VERSION_NOTIFICATIONS_ENABLED":     "false",
			"N8N_HIRING_BANNER_ENABLED":             "false",
			"N8N_INSTANCE_OWNER_MANAGED_BY_ENV":     "true",
		}
	case "open-webui":
		return map[string]string{
			"ENV":                         "prod",
			"DATA_DIR":                    p.Layout.DataDir(),
			"HF_HOME":                     path.Join(p.Layout.DataDir(), "cache", "huggingface"),
			"WEBUI_URL":                   base,
			"CORS_ALLOW_ORIGIN":           base,
			"FORWARDED_ALLOW_IPS":         "127.0.0.1",
			"ENABLE_SIGNUP":               "False",
			"DEFAULT_USER_ROLE":           "pending",
			"ENABLE_CODE_EXECUTION":       "False",
			"ENABLE_CODE_INTERPRETER":     "False",
			"SAFE_MODE":                   "True",
			"ENABLE_COMMUNITY_SHARING":    "False",
			"ENABLE_OLLAMA_API":           "False",
			"ENABLE_VERSION_UPDATE_CHECK": "False",
			"ENABLE_DIRECT_CONNECTIONS":   "False",
			"ANONYMIZED_TELEMETRY":        "False",
			"SCARF_NO_ANALYTICS":          "true",
			"DO_NOT_TRACK":                "true",
		}
	}
	return map[string]string{}
}

// GeneratedKeys are the values made once on the server (random, 32 bytes
// hex) and kept across rewrites: changing them would lock the owner out of
// encrypted credentials or sessions.
func GeneratedKeys(app App) []string {
	switch app.ID {
	case "n8n":
		return []string{"N8N_ENCRYPTION_KEY"}
	case "open-webui":
		return []string{"WEBUI_SECRET_KEY"}
	}
	return nil
}

// OwnerKeys are the environment keys of the first administrator.
func OwnerKeys(app App) (email, secret string) {
	switch app.ID {
	case "n8n":
		return "N8N_INSTANCE_OWNER_EMAIL", "N8N_INSTANCE_OWNER_PASSWORD_HASH"
	case "open-webui":
		return "WEBUI_ADMIN_EMAIL", "WEBUI_ADMIN_PASSWORD"
	}
	return "", ""
}

// FirstStartOnlyKeys are removed from the environment file once the app has
// started: Open WebUI reads its first administrator only into an empty
// database, so its plain password must not stay on disk.
func FirstStartOnlyKeys(app App) []string {
	if app.ID == "open-webui" {
		return []string{"WEBUI_ADMIN_PASSWORD"}
	}
	return nil
}

var (
	envKeyPattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	generatedPattern = regexp.MustCompile(`^[0-9a-f]{32,128}$`)
	emailPattern     = regexp.MustCompile(`^[^\s@"\\]{1,64}@[A-Za-z0-9.-]{1,190}\.[A-Za-z]{2,}$`)
)

// ValidEmail is the owner e-mail shape accepted in the environment file.
func ValidEmail(email string) bool { return emailPattern.MatchString(email) && len(email) <= 254 }

// ValidSecretValue: one line of printable text, 1..4096 bytes.
func ValidSecretValue(value string) bool {
	if value == "" || len(value) > 4096 {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// quoteEnv writes a value for a systemd EnvironmentFile: double quotes with
// backslash and quote escaped. systemd expands nothing in an
// EnvironmentFile, so "$" in a bcrypt hash is kept as it is.
func quoteEnv(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

// RenderEnv is the environment file text. Fixed values win over everything
// else, so a customer value can never reopen a disabled feature or move the
// listen address.
func RenderEnv(p EnvParams) (string, error) {
	values := map[string]string{}
	allowed := p.App.AllowedSecretKeys()
	for key, value := range p.Secrets {
		if !allowed[key] {
			return "", fmt.Errorf("%s is not a setting of %s", key, p.App.Name)
		}
		if !ValidSecretValue(value) {
			return "", fmt.Errorf("the value of %s is not one line of text", key)
		}
		values[key] = value
	}
	for _, key := range GeneratedKeys(p.App) {
		value := p.Generated[key]
		if !generatedPattern.MatchString(value) {
			return "", fmt.Errorf("generated %s is missing", key)
		}
		values[key] = value
	}
	emailKey, secretKey := OwnerKeys(p.App)
	if p.OwnerEmail != "" {
		if !ValidEmail(p.OwnerEmail) {
			return "", errors.New("the owner e-mail address is not valid")
		}
		values[emailKey] = p.OwnerEmail
	}
	if p.OwnerSecret != "" {
		if !ValidSecretValue(p.OwnerSecret) {
			return "", errors.New("the owner password is not one line of text")
		}
		values[secretKey] = p.OwnerSecret
	}
	for key, value := range FixedEnv(p) {
		values[key] = value
	}
	return renderEnvText(p.App.Name, values)
}

// renderEnvText is the sealed environment file text of values (keys sorted).
func renderEnvText(appName string, values map[string]string) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !envKeyPattern.MatchString(key) {
			return "", fmt.Errorf("invalid key %q", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Environment of " + appName + ", written by CelikPanel. Root's (0600): systemd reads it before the app starts.\n")
	b.WriteString("# Lines CelikPanel owns (listen address, disabled features) are written again on every change.\n")
	b.WriteString("# A change made here is kept: CelikPanel then refuses to write this file again and says so.\n")
	for _, key := range keys {
		b.WriteString(key + "=" + quoteEnv(values[key]) + "\n")
	}
	return Seal(b.String()), nil
}

// RenderLegacyEnv re-seals the values of an environment file read from the
// site's folder (alpha.118 before the security fix) for the root location.
// Lines that are not a valid key with one line of text are dropped; the
// next change in the panel renders the file from its parts again.
func RenderLegacyEnv(app App, values map[string]string) (string, error) {
	kept := map[string]string{}
	for key, value := range values {
		if envKeyPattern.MatchString(key) && (value == "" || ValidSecretValue(value)) {
			kept[key] = value
		}
	}
	return renderEnvText(app.Name, kept)
}

// MigrateUnitEnvLine rewrites an unchanged ("ours") unit of an earlier
// alpha.118 build whose EnvironmentFile= names a file in the site's folder:
// only that line changes, to EnvFile(installID), and the unit is sealed
// again. It returns the legacy path the unit named. ok is false when the
// unit is not CelikPanel's unchanged text or already names the root file.
func MigrateUnitEnvLine(text string, installID int) (string, string, bool) {
	if UnitTextState(text) != "ours" {
		return "", "", false
	}
	_, body, _ := strings.Cut(text, "\n")
	want := EnvFile(installID)
	lines := strings.Split(body, "\n")
	legacy := ""
	for i, line := range lines {
		value, found := strings.CutPrefix(line, "EnvironmentFile=")
		if !found {
			continue
		}
		if value == want || legacy != "" {
			return "", "", false
		}
		legacy = value
		lines[i] = "EnvironmentFile=" + want
	}
	if legacy == "" {
		return "", "", false
	}
	return Seal(strings.Join(lines, "\n")), legacy, true
}

var legacyEnvPattern = regexp.MustCompile(`^/([1-9][0-9]{0,9})/sites/([1-9][0-9]{0,9})/apps/([a-z0-9-]{1,40})/config/app\.env$`)

// LegacyEnvTarget reads the subscription, domain and app from a legacy
// environment file path; ok is false for any other path.
func LegacyEnvTarget(legacy string) (subscriptionID, domainID int, appID string, ok bool) {
	root := hostingpath.SubscriptionsRoot()
	rest, found := strings.CutPrefix(legacy, root)
	if !found || path.Clean(legacy) != legacy {
		return 0, 0, "", false
	}
	m := legacyEnvPattern.FindStringSubmatch(rest)
	if m == nil || !Known(m[3]) {
		return 0, 0, "", false
	}
	sub, err1 := strconv.Atoi(m[1])
	dom, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return 0, 0, "", false
	}
	return sub, dom, m[3], true
}

// UnitUser is the User= of a unit text ("" when absent).
func UnitUser(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if value, found := strings.CutPrefix(line, "User="); found {
			return value
		}
	}
	return ""
}

// ParseEnv reads an environment file written by RenderEnv (or edited in the
// same format). Unknown lines are ignored.
func ParseEnv(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !envKeyPattern.MatchString(key) {
			continue
		}
		if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			inner := value[1 : len(value)-1]
			var b strings.Builder
			for i := 0; i < len(inner); i++ {
				if inner[i] == '\\' && i+1 < len(inner) {
					i++
				}
				b.WriteByte(inner[i])
			}
			value = b.String()
		}
		out[key] = value
	}
	return out
}

// InstallScript is the fixed shell text the transient install unit runs as
// the site's user. Every value arrives as a positional argument, never in the
// text. The upstream file was already checked by the Agent; it is checked
// again here against the same pin before anything reads it.
func InstallScript(app App) string {
	common := `set -eu
umask 077
root=$1; file=$2; algo=$3; digest=$4; target=$5; runtime=$6
cd "$root"
mkdir -p data config .cache
echo "$digest  $root/dist/$file" | "${algo}sum" -c --status -
`
	switch app.Runtime {
	case RuntimeNode:
		return common + `npmcli=$7
export npm_config_cache="$root/.cache/npm" npm_config_update_notifier=false npm_config_fund=false npm_config_audit=false
rm -rf "$target.partial"
mkdir -p "$target.partial"
"$runtime" "$npmcli" install --prefix "$target.partial" --omit=dev --no-save` + npmPlatformFlag(app) + ` "$root/dist/$file"
"$runtime" "$npmcli" audit signatures --prefix "$target.partial"
rm -rf "$target"
mv "$target.partial" "$target"
rm -rf "$root/.cache/npm"
echo "CelikPanel: installed $file"
`
	case RuntimePython:
		return common + `export PIP_CACHE_DIR="$root/.cache/pip" PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_NO_INPUT=1
rm -rf "$target"
"$runtime" -m venv "$target"
"$target/bin/pip" install --prefer-binary "$root/dist/$file"
rm -rf "$root/.cache/pip"
echo "CelikPanel: installed $file"
`
	}
	return ""
}

// npmPlatformFlag is " --force" for an app whose own package requires other
// platforms' binaries (App.NPMPlatformDepsRequired), "" otherwise: npm
// skips other platforms' OPTIONAL dependencies by itself, and refuses
// required ones unless forced.
func npmPlatformFlag(app App) string {
	if app.NPMPlatformDepsRequired {
		return " --force"
	}
	return ""
}

// SystemdExecArg escapes one argument of a systemd command line (ExecStart=
// or systemd-run's command): systemd replaces "${NAME}" (and a lone "$NAME"
// word) with environment values before the program runs, so every "$" the
// program itself must see is written "$$". A "%" is a unit-file specifier but
// is passed literally to a transient unit, so no escape is right for both:
// SystemdExecArg refuses one and the install text is written without any.
func SystemdExecArg(arg string) (string, error) {
	if strings.ContainsRune(arg, '%') {
		return "", errors.New("a systemd command argument contains a specifier character (%)")
	}
	return strings.ReplaceAll(arg, "$", "$$"), nil
}

// InstallCommand is the command vector (after systemd-run's "--") of the
// transient install unit: the shell, InstallScript and its positional
// arguments, each escaped for systemd. Without the escape systemd expanded
// "${algo}" to nothing and the checksum step ran "sum -c" (alpha.118 lab).
func InstallCommand(app App, args []string) ([]string, error) {
	script := InstallScript(app)
	if script == "" {
		return nil, errors.New("the application has no install script")
	}
	out := append([]string{"/bin/sh", "-c", script, "sh"}, args...)
	for i, arg := range out {
		escaped, err := SystemdExecArg(arg)
		if err != nil {
			return nil, err
		}
		out[i] = escaped
	}
	return out, nil
}

// InstallArgs are the positional arguments of InstallScript.
func InstallArgs(app App, layout Layout, version string, runtime RuntimeChoice) ([]string, error) {
	algorithm, digest, err := app.ArtifactHex()
	if err != nil {
		return nil, err
	}
	args := []string{layout.Root, app.ArtifactFile, algorithm, digest, layout.InstallDir(version), runtime.Path}
	if app.Runtime == RuntimeNode {
		args = append(args, runtime.NPMCLI)
	}
	for _, arg := range args {
		if !safeValue.MatchString(arg) {
			return nil, errors.New("an install argument has unsafe characters")
		}
	}
	return args, nil
}
