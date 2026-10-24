package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Which PHP a tool runs on (alpha.104). Webmail (Roundcube), phpMyAdmin and
// phpPgAdmin are served through a PHP-FPM socket named in the nginx
// configuration CelikPanel generates for them. Until alpha.103 every
// regeneration picked the lexically last socket under /run/php, so
// installing a newer PHP and regenerating silently moved the tools to a
// version that might not have their extensions, and removing the version
// they ran on broke them without a word (Frankfurt, 2026-10-23: all three
// still pointed at php8.4-fpm.sock after PHP 8.4 was removed).
//
// The rule now:
//   - The version a tool is bound to is what its generated configuration
//     names (fastcgi_pass); an owner's own edit of that line is respected.
//     The Agent also keeps the binding in its record
//     (php-tool-bindings.json) so a tool that is removed and installed again
//     returns to the same version. A regeneration keeps a tool on its bound
//     version while that version is installed, and makes sure the tool's
//     PHP extensions exist for it.
//   - A tool with nothing bound yet gets the distribution's default PHP
//     (read from the installed php-* meta packages' dependencies, else the
//     php-fpm.sock and php alternatives), else the newest version that has
//     the tool's extensions, else the newest; always written as the
//     versioned socket, never the floating alias.
//   - When the bound version is no longer installed the tool's line is left
//     as it is and the caller reports it.
//   - Moving a tool to another version is the explicit, verified action
//     Agent.BindPHPToolV1 (extensions ensured, nginx validated, the tool
//     probed through its own socket, the previous binding restored when the
//     probe fails).
//   - Removing a PHP version an installed tool is bound to is refused
//     (REMOVAL_TOOLS_BOUND).
//
// Bir aracın PHP sürümü, onun için üretilen yapılandırmanın adını verdiği
// sürümdür; yeniden üretim aracı o sürüm kurulu kaldıkça orada tutar.
// Taşıma açık ve doğrulanan bir işlemdir; bağlı sürümü kaldırmak reddedilir.

const (
	phpToolWebmail    = "webmail"
	phpToolPHPMyAdmin = "phpmyadmin"
	phpToolPHPPgAdmin = "phppgadmin"

	errCodePHPToolNotInstalled      = "PHP_TOOL_NOT_INSTALLED"
	errCodePHPVersionNotInstalled   = "PHP_VERSION_NOT_INSTALLED"
	errCodePHPVersionNotRunning     = "PHP_VERSION_NOT_RUNNING"
	errCodePHPToolExtensionsFailed  = "PHP_TOOL_EXTENSIONS_FAILED"
	errCodePHPToolConfigRejected    = "PHP_TOOL_CONFIG_REJECTED"
	errCodePHPToolProbeFailed       = "PHP_TOOL_PROBE_FAILED"
	errCodePHPToolUnsupported       = "PHP_TOOL_UNSUPPORTED"
	errCodePHPToolConfigOwnerEdited = "PHP_TOOL_CONFIG_OWNER_EDITED"
)

var phpTools = []string{phpToolPHPMyAdmin, phpToolPHPPgAdmin, phpToolWebmail}

// Paths are variables so tests can point them at a temporary tree.
var (
	phpFPMRunDir               = "/run/php"
	phpEtcDir                  = "/etc/php"
	phpCLIAlternative          = "/usr/bin/php"
	phpToolWebmailConfPath     = webmailConfPath
	phpToolDBToolsConfPath     = dbToolsConfPath
	phpToolBindingRecordPath   = "/var/lib/celikpanel-agent-private/php-tool-bindings.json"
	phpToolCheckScriptPath     = "/var/lib/celikpanel-php-check/check.php"
	phpFallbackSocketGlobs     = []string{"/run/php-fpm/php-fpm.sock", "/var/run/php-fpm/php-fpm.sock"}
	versionedFPMSocketName     = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})-fpm\.sock$`)
	phpCLIVersionedName        = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})$`)
	phpVersionText             = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}$`)
	fastcgiPassUnix            = regexp.MustCompile(`fastcgi_pass\s+unix:([^;\s]+)\s*;`)
	dbToolLocationStartLine    = regexp.MustCompile(`^\s*location\s+/dbtool/([a-z]+)/\s*\{`)
	phpMetaDependsVersion      = regexp.MustCompile(`\bphp([0-9]{1,2}\.[0-9]{1,2})-`)
	phpDefaultVersionFromMetas = defaultPHPVersionFromInstalledMetas
	phpToolInstalledCheck      = phpToolInstalledOnHost
	phpToolPackageInstalled    = func(pkg string) bool { return packageInstalledForFamily("apt", pkg) }
)

// phpToolExtensionSuffixes are the Debian/Ubuntu package suffixes each tool
// needs for its PHP version: Roundcube's capability list (webmail_rpc.go),
// phpMyAdmin's Depends (php-mysql, php-mbstring, php-xml) and phpPgAdmin's
// (php-pgsql, php-mbstring; xml for its exports).
var phpToolExtensionSuffixes = map[string][]string{
	phpToolWebmail:    {"mbstring", "xml", "intl", "sqlite3", "zip"},
	phpToolPHPMyAdmin: {"mysql", "mbstring", "xml"},
	phpToolPHPPgAdmin: {"pgsql", "mbstring", "xml"},
}

// phpToolExtensions are the PHP extension names the CLI probe asks for.
var phpToolExtensions = map[string][]string{
	phpToolWebmail:    {"mbstring", "dom", "intl", "pdo_sqlite", "zip"},
	phpToolPHPMyAdmin: {"mysqli", "mbstring", "xml"},
	phpToolPHPPgAdmin: {"pgsql", "mbstring", "xml"},
}

func validPHPTool(tool string) bool {
	_, ok := phpToolExtensions[tool]
	return ok
}

// errPHPToolVersionGone: the version a tool is bound to is not installed.
type errPHPToolVersionGone struct {
	Tool    string
	Socket  string
	Version string
}

func (e *errPHPToolVersionGone) Error() string {
	version := e.Version
	if version == "" {
		version = e.Socket
	}
	return fmt.Sprintf(
		"%s runs on PHP %s, which is no longer installed; choose another PHP version for it in its Manage window, or install PHP-FPM %s again from Components",
		phpToolDisplayName(e.Tool), version, version,
	)
}

func phpToolDisplayName(tool string) string {
	switch tool {
	case phpToolWebmail:
		return "Webmail (Roundcube)"
	case phpToolPHPMyAdmin:
		return "phpMyAdmin"
	case phpToolPHPPgAdmin:
		return "phpPgAdmin"
	default:
		return tool
	}
}

// fpmSocketVersion returns the PHP version a versioned socket path names
// ("/run/php/php8.4-fpm.sock" → "8.4"), or "".
func fpmSocketVersion(path string) string {
	if match := versionedFPMSocketName.FindStringSubmatch(filepath.Base(path)); match != nil {
		return match[1]
	}
	return ""
}

func versionedFPMSocketPath(version string) string {
	return filepath.Join(phpFPMRunDir, "php"+version+"-fpm.sock")
}

// phpVersionInstalled: the version's FPM tree exists (a stopped pool has no
// socket but is still installed).
func phpVersionInstalled(version string) bool {
	if !phpVersionText.MatchString(version) {
		return false
	}
	info, err := os.Stat(filepath.Join(phpEtcDir, version, "fpm"))
	return err == nil && info.IsDir()
}

// installedPHPFPMVersions lists the installed PHP-FPM versions, newest first.
func installedPHPFPMVersions() []string {
	entries, err := os.ReadDir(phpEtcDir)
	if err != nil {
		return []string{}
	}
	versions := []string{}
	for _, entry := range entries {
		if entry.IsDir() && phpVersionInstalled(entry.Name()) {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool { return comparePHPVersions(versions[i], versions[j]) > 0 })
	return versions
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// boundSocketUsable reports whether a bound socket's PHP is still there.
func boundSocketUsable(socket string) bool {
	if version := fpmSocketVersion(socket); version != "" {
		return phpVersionInstalled(version) || pathExists(socket)
	}
	return pathExists(socket)
}

// parseWebmailFPMSocket returns the socket the webmail server hands PHP to.
func parseWebmailFPMSocket(conf string) string {
	if match := fastcgiPassUnix.FindStringSubmatch(conf); match != nil {
		return match[1]
	}
	return ""
}

// parseDBToolsFPMSockets returns, per tool location, the socket it hands
// PHP to.
func parseDBToolsFPMSockets(conf string) map[string]string {
	out := map[string]string{}
	current := ""
	for _, line := range strings.Split(conf, "\n") {
		if match := dbToolLocationStartLine.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		if current == "" {
			continue
		}
		if match := fastcgiPassUnix.FindStringSubmatch(line); match != nil {
			if _, seen := out[current]; !seen {
				out[current] = match[1]
			}
			current = ""
		}
	}
	return out
}

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// currentPHPToolSockets reads what each tool's generated configuration
// names today.
func currentPHPToolSockets() map[string]string {
	out := map[string]string{}
	if socket := parseWebmailFPMSocket(readFileString(phpToolWebmailConfPath)); socket != "" {
		out[phpToolWebmail] = socket
	}
	for tool, socket := range parseDBToolsFPMSockets(readFileString(phpToolDBToolsConfPath)) {
		if tool == phpToolPHPMyAdmin || tool == phpToolPHPPgAdmin {
			out[tool] = socket
		}
	}
	return out
}

// phpToolBindingRecord is the Agent's own memory of each tool's version.
type phpToolBindingRecord struct {
	Version int               `json:"version"`
	Tools   map[string]string `json:"tools"`
}

func readPHPToolBindingRecord() map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(phpToolBindingRecordPath)
	if err != nil || len(data) > 16*1024 {
		return out
	}
	var record phpToolBindingRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 {
		return out
	}
	for tool, version := range record.Tools {
		if validPHPTool(tool) && phpVersionText.MatchString(version) {
			out[tool] = version
		}
	}
	return out
}

// writePHPToolBinding records one tool's version (atomic replace).
func writePHPToolBinding(tool, version string) error {
	if !validPHPTool(tool) || (version != "" && !phpVersionText.MatchString(version)) {
		return fmt.Errorf("invalid PHP tool binding %s=%s", tool, version)
	}
	tools := readPHPToolBindingRecord()
	if version == "" {
		delete(tools, tool)
	} else {
		tools[tool] = version
	}
	data, err := json.Marshal(phpToolBindingRecord{Version: 1, Tools: tools})
	if err != nil {
		return err
	}
	dir := filepath.Dir(phpToolBindingRecordPath)
	tmp, err := os.CreateTemp(dir, ".php-tool-bindings-*")
	if err != nil {
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	if err := os.Rename(tmpName, phpToolBindingRecordPath); err != nil {
		return fmt.Errorf("record PHP tool binding: %w", err)
	}
	return nil
}

func phpToolInstalledOnHost(tool string) bool {
	switch tool {
	case phpToolWebmail:
		return roundcubeInstalled()
	case phpToolPHPMyAdmin, phpToolPHPPgAdmin:
		return firstInstalledDBToolRoot(dbToolRoots[tool]) != ""
	}
	return false
}

// phpToolState is one tool's binding as the host shows it.
type phpToolState struct {
	Tool      string
	Installed bool
	// Socket is the bound socket ("" = nothing bound); Source says where it
	// was read: "config" (the generated nginx configuration) or "record"
	// (the Agent's record, for a tool whose configuration has no line).
	Socket string
	Source string
}

func (s phpToolState) version() string { return resolvedSocketVersion(s.Socket) }

// phpToolStates reads every tool's binding: the configuration's line first
// (it is what serves, and an owner's edit wins), else the record.
func phpToolStates() []phpToolState {
	conf := currentPHPToolSockets()
	record := readPHPToolBindingRecord()
	states := make([]phpToolState, 0, len(phpTools))
	for _, tool := range phpTools {
		state := phpToolState{Tool: tool, Installed: phpToolInstalledCheck(tool)}
		if socket := conf[tool]; socket != "" {
			state.Socket, state.Source = socket, "config"
		} else if version := record[tool]; version != "" && phpVersionInstalled(version) {
			// A remembered version that is gone is no binding: the tool,
			// installed again, gets the default instead of a refusal.
			state.Socket, state.Source = versionedFPMSocketPath(version), "record"
		}
		states = append(states, state)
	}
	return states
}

func phpToolStateFor(tool string) phpToolState {
	for _, state := range phpToolStates() {
		if state.Tool == tool {
			return state
		}
	}
	return phpToolState{Tool: tool}
}

// resolvedSocketVersion follows a floating alias (php-fpm.sock) to the
// version it points at today.
func resolvedSocketVersion(socket string) string {
	if socket == "" {
		return ""
	}
	if version := fpmSocketVersion(socket); version != "" {
		return version
	}
	if resolved, err := filepath.EvalSymlinks(socket); err == nil {
		return fpmSocketVersion(resolved)
	}
	return ""
}

// phpToolsBoundToVersion lists the installed tools bound to a PHP version
// ("8.4"); an empty version lists every installed tool bound to any PHP-FPM
// socket.
func phpToolsBoundToVersion(version string) []string {
	bound := []string{}
	for _, state := range phpToolStates() {
		if !state.Installed || state.Socket == "" {
			continue
		}
		if version == "" || state.version() == version {
			bound = append(bound, state.Tool)
		}
	}
	sort.Strings(bound)
	return bound
}

func comparePHPVersions(a, b string) int {
	aMajor, aMinor, _ := strings.Cut(a, ".")
	bMajor, bMinor, _ := strings.Cut(b, ".")
	for _, pair := range [][2]string{{aMajor, bMajor}, {aMinor, bMinor}} {
		x, _ := strconv.Atoi(pair[0])
		y, _ := strconv.Atoi(pair[1])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// versionedFPMSockets lists the existing versioned sockets, numerically
// newest first (8.10 is newer than 8.9; a lexical sort said otherwise).
func versionedFPMSockets() []string {
	matches, _ := filepath.Glob(filepath.Join(phpFPMRunDir, "php*-fpm.sock"))
	sockets := []string{}
	for _, match := range matches {
		if fpmSocketVersion(match) != "" {
			sockets = append(sockets, match)
		}
	}
	sort.Slice(sockets, func(i, j int) bool {
		return comparePHPVersions(fpmSocketVersion(sockets[i]), fpmSocketVersion(sockets[j])) > 0
	})
	return sockets
}

func newestVersionedFPMSocket() string {
	if sockets := versionedFPMSockets(); len(sockets) > 0 {
		return sockets[0]
	}
	return ""
}

// defaultPHPVersionFromInstalledMetas reads the distribution's default PHP
// from what its installed php-* meta packages depend on (php-fpm →
// php8.4-fpm on Debian 13). The tools' distribution packages pull their PHP
// extensions through these meta packages, so this is the version their
// extensions were installed for.
func defaultPHPVersionFromInstalledMetas() string {
	dpkgQuery, err := exec.LookPath("dpkg-query")
	if err != nil {
		return ""
	}
	for _, meta := range []string{"php-fpm", "php-mysql", "php-pgsql", "php-mbstring", "php-xml", "php-cli"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := serviceMutationCommand(ctx, dpkgQuery, "-W", "-f", "${Status}\t${Depends}\n", meta).Output()
		cancel()
		if err != nil {
			continue
		}
		status, depends, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")
		if !strings.HasSuffix(status, "install ok installed") {
			continue
		}
		if match := phpMetaDependsVersion.FindStringSubmatch(depends); match != nil && phpVersionInstalled(match[1]) {
			return match[1]
		}
	}
	return ""
}

func phpToolHasExtensionPackages(tool, version string) bool {
	for _, suffix := range phpToolExtensionSuffixes[tool] {
		if !phpToolPackageInstalled("php" + version + "-" + suffix) {
			return false
		}
	}
	return true
}

// defaultFPMSocket is the socket for a tool with nothing bound yet.
func defaultFPMSocket(tool string) string {
	// 1. The distribution's default PHP, from its meta packages.
	if version := phpDefaultVersionFromMetas(); version != "" {
		if socket := versionedFPMSocketPath(version); pathExists(socket) {
			return socket
		}
	}
	// 2. The php-fpm.sock alternative.
	if version := resolvedSocketVersion(filepath.Join(phpFPMRunDir, "php-fpm.sock")); version != "" {
		if socket := versionedFPMSocketPath(version); pathExists(socket) {
			return socket
		}
	}
	// 3. The php command's alternative (/usr/bin/php → php8.4).
	if resolved, err := filepath.EvalSymlinks(phpCLIAlternative); err == nil {
		if match := phpCLIVersionedName.FindStringSubmatch(filepath.Base(resolved)); match != nil {
			if socket := versionedFPMSocketPath(match[1]); pathExists(socket) {
				return socket
			}
		}
	}
	// 4. The newest version that has the tool's extensions, else the newest.
	sockets := versionedFPMSockets()
	for _, socket := range sockets {
		if phpToolHasExtensionPackages(tool, fpmSocketVersion(socket)) {
			return socket
		}
	}
	if len(sockets) > 0 {
		return sockets[0]
	}
	// 5. Unversioned layouts (Arch: /run/php-fpm/php-fpm.sock).
	for _, candidate := range append([]string{filepath.Join(phpFPMRunDir, "php-fpm.sock")}, phpFallbackSocketGlobs...) {
		if pathExists(candidate) {
			return candidate
		}
	}
	return ""
}

// phpToolSocket returns the socket a tool must be served from: its bound
// socket while that PHP is installed; for a tool with nothing bound, the
// default. A bound version that is gone returns errPHPToolVersionGone and
// the bound socket, so the caller can leave the tool's configuration as it is.
func phpToolSocket(tool, bound string) (string, error) {
	if bound != "" {
		if boundSocketUsable(bound) {
			return bound, nil
		}
		return bound, &errPHPToolVersionGone{Tool: tool, Socket: bound, Version: fpmSocketVersion(bound)}
	}
	socket := defaultFPMSocket(tool)
	if socket == "" {
		return "", fmt.Errorf("PHP-FPM is not installed")
	}
	return socket, nil
}

// bindPHPToolSocket validates an explicit target version for a tool and
// returns its socket.
func bindPHPToolSocket(tool, version string) (string, error) {
	if !phpVersionText.MatchString(version) {
		return "", fmt.Errorf("invalid PHP version %q", version)
	}
	if !validPHPTool(tool) {
		return "", fmt.Errorf("unknown PHP tool %q", tool)
	}
	if !phpVersionInstalled(version) {
		return "", fmt.Errorf("PHP %s is not installed", version)
	}
	return versionedFPMSocketPath(version), nil
}

// phpCLIForVersion is the PHP command of one version when it is installed
// (/usr/bin/php8.4), else the default "php".
func phpCLIForVersion(version string) string {
	if version != "" {
		candidate := filepath.Join(filepath.Dir(phpCLIAlternative), "php"+version)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "php"
}

// missingPHPToolExtensionPackages names the apt packages a tool still needs
// on a version.
func missingPHPToolExtensionPackages(tool, version string, installed func(string) bool) []string {
	missing := []string{}
	if version == "" {
		return missing
	}
	for _, suffix := range phpToolExtensionSuffixes[tool] {
		pkg := "php" + version + "-" + suffix
		if !installed(pkg) {
			missing = append(missing, pkg)
		}
	}
	return missing
}

// phpToolCLIProbeCode asks one PHP for the tool's extensions.
func phpToolCLIProbeCode(tool string) string {
	quoted := make([]string, 0, len(phpToolExtensions[tool]))
	for _, ext := range phpToolExtensions[tool] {
		quoted = append(quoted, "'"+ext+"'")
	}
	return "$m=[];foreach([" + strings.Join(quoted, ",") + "] as $e){if(!extension_loaded($e)){$m[]=$e;}}echo $m?'missing:'.implode(',',$m):'ok';"
}

// phpToolProbeError is a cause a screen can say in its own language
// (alpha.104): Kind is one of the transport.PHPToolCause kinds.
// phpToolProbeError, ekranın kendi dilinde söyleyebileceği bir nedendir.
type phpToolProbeError struct {
	Kind       string
	Extensions []string
	Status     int
	Running    string
	Message    string
}

func (e *phpToolProbeError) Error() string { return e.Message }

// phpToolCauseOf turns an error into a cause: a typed probe error keeps its
// kind; anything else is the fallback kind with the error as its detail.
func phpToolCauseOf(err error, fallback, version string) transport.PHPToolCause {
	cause := transport.PHPToolCause{Kind: fallback, Version: version}
	if err == nil {
		return cause
	}
	cause.Detail = err.Error()
	var typed *phpToolProbeError
	if errors.As(err, &typed) {
		cause.Kind = typed.Kind
		cause.Extensions = typed.Extensions
		cause.Status = typed.Status
		cause.Running = typed.Running
	}
	return cause
}

func splitExtensions(list string) []string {
	out := []string{}
	for _, item := range strings.Split(list, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func probePHPToolCLI(ctx context.Context, tool, version string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := serviceMutationCommand(probeCtx, phpCLIForVersion(version), "-r", phpToolCLIProbeCode(tool)).Output()
	if err != nil {
		return &phpToolProbeError{Kind: "no_answer", Message: fmt.Sprintf("PHP %s could not be asked for its extensions: %v", version, err)}
	}
	result := strings.TrimSpace(string(out))
	if result == "ok" {
		return nil
	}
	if strings.HasPrefix(result, "missing:") {
		missing := splitExtensions(strings.TrimPrefix(result, "missing:"))
		return &phpToolProbeError{Kind: "missing_extensions", Extensions: missing,
			Message: fmt.Sprintf("PHP %s lacks %s for %s", version, strings.Join(missing, ","), phpToolDisplayName(tool))}
	}
	return &phpToolProbeError{Kind: "no_answer", Message: fmt.Sprintf("PHP %s answered the extension check unexpectedly", version)}
}

// checkPHPToolResponse decides whether a page is the tool itself, rendered
// by PHP: a 200 from nginx alone is not enough (a raw PHP file, a bad
// gateway page or the tool's own "extension missing" page are not ready).
func checkPHPToolResponse(tool string, status int, body []byte) error {
	text := string(body)
	if status != http.StatusOK {
		return &phpToolProbeError{Kind: "http_status", Status: status, Message: fmt.Sprintf("%s answered HTTP %d", phpToolDisplayName(tool), status)}
	}
	if strings.Contains(text, "<?php") {
		return &phpToolProbeError{Kind: "served_as_file", Message: fmt.Sprintf("%s was served as a file, not run by PHP", phpToolDisplayName(tool))}
	}
	var marker string
	var failures []string
	switch tool {
	case phpToolWebmail:
		marker = "rcmloginuser"
	case phpToolPHPMyAdmin:
		marker = "phpMyAdmin"
		failures = []string{"extension is missing", "Fatal error"}
	case phpToolPHPPgAdmin:
		marker = "phpPgAdmin"
		failures = []string{"does not support PostgreSQL", "Fatal error"}
	default:
		return fmt.Errorf("unknown PHP tool %q", tool)
	}
	for _, failure := range failures {
		if strings.Contains(text, failure) {
			return &phpToolProbeError{Kind: "tool_error", Message: fmt.Sprintf("%s reported: %s", phpToolDisplayName(tool), failure)}
		}
	}
	if !strings.Contains(text, marker) {
		return &phpToolProbeError{Kind: "not_tool_page", Message: fmt.Sprintf("%s did not answer with its own page", phpToolDisplayName(tool))}
	}
	return nil
}

const phpToolProbeBodyLimit = 512 * 1024

// The PHP check (alpha.104): a CelikPanel-owned script served through the
// tool's own PHP-FPM socket at a path outside /dbtool/ and /webmail/, so the
// Panel's proxies never forward it; only the Agent asks it, on the loopback
// address or the webmail socket. It answers which PHP version runs it and
// whether the tool's extensions are loaded in that PHP-FPM (the command-line
// PHP can differ from FPM).
// CelikPanel'in kendi PHP denetimi: aracın kendi soketinden, Panel'in
// vekillerinin asla iletmediği bir yolda sunulur.
const phpToolCheckPrefix = "/__celikpanel_php_check/"

const phpToolCheckScript = `<?php
// Managed by CelikPanel: answers which PHP runs here and whether the
// extensions named by the web server are loaded. Served only on the
// loopback tools server and the webmail socket, never through the panel.
header('Content-Type: text/plain');
$need = array_filter(explode(',', (string) ($_SERVER['CELIKPANEL_PHP_EXTENSIONS'] ?? '')));
$missing = [];
foreach ($need as $extension) {
    if (!extension_loaded($extension)) {
        $missing[] = $extension;
    }
}
echo $missing ? 'celikpanel-php-check:missing:' . implode(',', $missing) . ':' . PHP_VERSION : 'celikpanel-php-check:ok:' . PHP_VERSION;
`

// ensurePHPToolCheckScript writes the check script (root-owned, readable by
// the PHP-FPM pool) when it is missing or different.
func ensurePHPToolCheckScript() error {
	if current, err := os.ReadFile(phpToolCheckScriptPath); err == nil && string(current) == phpToolCheckScript {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(phpToolCheckScriptPath), 0o755); err != nil {
		return fmt.Errorf("prepare the PHP check: %w", err)
	}
	if err := os.WriteFile(phpToolCheckScriptPath, []byte(phpToolCheckScript), 0o644); err != nil {
		return fmt.Errorf("prepare the PHP check: %w", err)
	}
	return nil
}

// phpToolCheckBlock is the nginx location of a tool's PHP check.
func phpToolCheckBlock(tool, socket string) string {
	return fmt.Sprintf(`
    # CelikPanel's PHP check for %s (asked only by the Agent; never forwarded by the panel).
    location = %s%s {
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME %s;
        fastcgi_param CELIKPANEL_PHP_EXTENSIONS "%s";
        fastcgi_pass unix:%s;
    }
`, tool, phpToolCheckPrefix, tool, phpToolCheckScriptPath, strings.Join(phpToolExtensions[tool], ","), socket)
}

// checkPHPToolCheckResponse decides whether the check proves the tool's PHP:
// the tool's extensions loaded in the PHP-FPM behind its socket, and, when a
// version is expected, that version.
func checkPHPToolCheckResponse(tool, version string, status int, body []byte) error {
	text := strings.TrimSpace(string(body))
	name := phpToolDisplayName(tool)
	if status != http.StatusOK || !strings.HasPrefix(text, "celikpanel-php-check:") {
		return &phpToolProbeError{Kind: "no_answer", Status: status, Message: fmt.Sprintf("PHP did not answer for %s on its socket (HTTP %d)", name, status)}
	}
	rest := strings.TrimPrefix(text, "celikpanel-php-check:")
	if strings.HasPrefix(rest, "missing:") {
		parts := strings.Split(strings.TrimPrefix(rest, "missing:"), ":")
		missing := splitExtensions(parts[0])
		return &phpToolProbeError{Kind: "missing_extensions", Extensions: missing,
			Message: fmt.Sprintf("PHP-FPM %s lacks %s for %s", version, strings.Join(missing, ","), name)}
	}
	running := strings.TrimPrefix(rest, "ok:")
	if version != "" && !strings.HasPrefix(running, version+".") {
		return &phpToolProbeError{Kind: "wrong_version", Running: running, Message: fmt.Sprintf("%s answered on PHP %s, not on PHP %s", name, running, version)}
	}
	return nil
}

// probePHPToolHTTP asks the tool through the same local server the Panel
// proxies to: webmail over its Unix socket, the database tools on the
// loopback address.
func probePHPToolHTTP(ctx context.Context, tool, version string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 20 * time.Second}
	base := "http://" + dbToolsAddr
	url := base + "/dbtool/" + tool + "/"
	if tool == phpToolWebmail {
		socket, err := webmailSocketPathForNginx()
		if err != nil {
			return err
		}
		client.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		}}
		base = "http://webmail.internal"
		url = base + "/webmail/"
	}
	// 1. PHP itself, through the tool's socket: the version and extensions.
	checkRequest, err := http.NewRequestWithContext(probeCtx, http.MethodGet, base+phpToolCheckPrefix+tool, nil)
	if err != nil {
		return err
	}
	checkResponse, err := client.Do(checkRequest)
	if err != nil {
		return &phpToolProbeError{Kind: "no_answer", Message: fmt.Sprintf("%s did not answer: %v", phpToolDisplayName(tool), err)}
	}
	checkBody, err := io.ReadAll(io.LimitReader(checkResponse.Body, phpToolProbeBodyLimit))
	checkResponse.Body.Close()
	if err != nil {
		return fmt.Errorf("%s answer could not be read: %v", phpToolDisplayName(tool), err)
	}
	if err := checkPHPToolCheckResponse(tool, version, checkResponse.StatusCode, checkBody); err != nil {
		return err
	}
	// 2. The tool's own page.
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/html")
	response, err := client.Do(request)
	if err != nil {
		return &phpToolProbeError{Kind: "no_answer", Message: fmt.Sprintf("%s did not answer: %v", phpToolDisplayName(tool), err)}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, phpToolProbeBodyLimit))
	if err != nil {
		return fmt.Errorf("%s answer could not be read: %v", phpToolDisplayName(tool), err)
	}
	return checkPHPToolResponse(tool, response.StatusCode, body)
}

// phpToolProbeSettle bounds how long a proof waits for nginx's reloaded
// configuration: right after "systemctl reload nginx" the old workers can
// still answer with the previous socket (a real Debian 13 run saw PHP 8.4
// answer for a tool just bound to 8.5).
// Kanıt, yeniden yüklenen nginx yapılandırmasını en çok bu kadar bekler.
var phpToolProbeSettle = 20 * time.Second

// retryPHPToolProbe repeats a proof until it passes or the bound runs out,
// and returns the last failure.
func retryPHPToolProbe(ctx context.Context, tool, version string, probe func(context.Context, string, string) error, bound, every time.Duration) error {
	deadline := time.Now().Add(bound)
	for {
		err := probe(ctx, tool, version)
		if err == nil || time.Now().Add(every).After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(every):
		}
	}
}

// renderDBToolsConfig writes the loopback server for the installed tools,
// each location handing PHP to its own socket.
func renderDBToolsConfig(tools []string, roots, sockets map[string]string) string {
	var b strings.Builder
	b.WriteString("# Managed by CelikPanel — database web tools, loopback only.\n")
	b.WriteString("# The panel reverse-proxies /dbtool/ here behind an admin session.\n")
	b.WriteString("# Each tool runs on the PHP version its fastcgi_pass names; choose it in the tool's Manage window.\n")
	b.WriteString("server {\n")
	fmt.Fprintf(&b, "    listen %s;\n", dbToolsAddr)
	b.WriteString("    server_name _;\n")
	for _, id := range tools {
		root := roots[id]
		// Path-preserving locations: the browser path and this server's path
		// are identical, so the tools' absolute URLs survive the panel proxy
		// without any rewriting.
		// Yol-koruyan location'lar: tarayıcı yolu ile bu sunucunun yolu aynı.
		fmt.Fprintf(&b, `
    location /dbtool/%s/ {
        alias %s/;
        index index.php;
        location ~ ^/dbtool/%s/(.+\.php)$ {
            alias %s/$1;
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $request_filename;
            fastcgi_pass unix:%s;
        }
    }
`, id, root, id, root, sockets[id])
		b.WriteString(phpToolCheckBlock(id, sockets[id]))
	}
	b.WriteString("}\n")
	return b.String()
}

func installedDBToolRoots() ([]string, map[string]string) {
	var tools []string
	roots := map[string]string{}
	for id, candidates := range dbToolRoots {
		if root := firstInstalledDBToolRoot(candidates); root != "" {
			tools = append(tools, id)
			roots[id] = root
		}
	}
	sort.Strings(tools)
	return tools, roots
}

// phpToolBindOps is the narrow seam of a verified rebind; production fills
// it with host operations, tests with recorders.
type phpToolBindOps struct {
	family           func() string
	toolInstalled    func(string) bool
	state            func(string) phpToolState
	targetSocket     func(tool, version string) (string, error)
	socketReady      func(string) bool
	packageInstalled func(string) bool
	installPackages  func(context.Context, []string) error
	reloadFPM        func(context.Context, string) error
	cliProbe         func(context.Context, string, string) error
	render           func(tool, socket string) (string, []byte, error)
	snapshotConf     func(string) (mailFileSnapshot, error)
	writeConf        func(string, []byte) error
	restoreConf      func(mailFileSnapshot) error
	nginxTest        func(context.Context) error
	nginxReload      func(context.Context) error
	httpProbe        func(context.Context, string, string) error
	writeRecord      func(tool, version string) error
}

// phpToolBindError is a typed failure of a rebind.
type phpToolBindError struct {
	Code    string
	Message string
	// Detail is the cause alone ("PHP-FPM 8.6 lacks intl for Webmail
	// (Roundcube)"), or "" when the message is the whole story.
	Detail string
	// Cause is the same, structured, for a screen that says it in its own
	// language.
	Cause transport.PHPToolCause
}

func (e *phpToolBindError) Error() string { return e.Message }

type phpToolBindResult struct {
	Previous          string
	InstalledPackages []string
	MutationApplied   bool
	Restored          bool
	RecordWarning     string
}

// bindPHPToolWith moves a tool to a PHP version and proves it answers
// there; when the proof fails, the previous configuration comes back.
func bindPHPToolWith(ctx context.Context, ops phpToolBindOps, tool, version string) (phpToolBindResult, error) {
	var result phpToolBindResult
	// The first error among the arguments is the cause, said on its own as
	// Detail so a screen can quote it without repeating the sentence; Cause
	// says the same in a form a screen translates.
	fail := func(code string, cause transport.PHPToolCause, format string, args ...any) (phpToolBindResult, error) {
		detail := ""
		for _, arg := range args {
			if err, ok := arg.(error); ok && err != nil {
				detail = err.Error()
				break
			}
		}
		if cause.Detail == "" {
			cause.Detail = detail
		}
		if cause.Version == "" {
			cause.Version = version
		}
		return result, &phpToolBindError{Code: code, Message: fmt.Sprintf(format, args...), Detail: detail, Cause: cause}
	}
	kind := func(k string) transport.PHPToolCause { return transport.PHPToolCause{Kind: k} }
	name := phpToolDisplayName(tool)
	if !validPHPTool(tool) {
		return fail(errCodePHPToolUnsupported, kind("unsupported"), "unknown PHP tool %q", tool)
	}
	if ops.family() != "apt" {
		return fail(errCodePHPToolUnsupported, kind("unsupported"), "choosing a PHP version for a tool is available on Debian and Ubuntu; this system runs one PHP")
	}
	if !ops.toolInstalled(tool) {
		return fail(errCodePHPToolNotInstalled, kind("tool_not_installed"), "%s is not installed", name)
	}
	socket, err := ops.targetSocket(tool, version)
	if err != nil {
		return fail(errCodePHPVersionNotInstalled, kind("version_not_installed"), "%v; install it from Components (PHP-FPM versions) first", err)
	}
	result.Previous = ops.state(tool).version()
	if !ops.socketReady(socket) {
		return fail(errCodePHPVersionNotRunning, kind("not_running"), "PHP-FPM %s is installed but not running; start it in Components, then choose it again. Nothing was changed", version)
	}

	// 1. The tool's extensions for this version.
	missing := missingPHPToolExtensionPackages(tool, version, ops.packageInstalled)
	if len(missing) > 0 {
		if err := ops.installPackages(ctx, missing); err != nil {
			return fail(errCodePHPToolExtensionsFailed, transport.PHPToolCause{Kind: "extensions_install_failed", Extensions: missing},
				"the PHP %s extensions %s needs could not be installed (%v); %s still runs on PHP %s", version, name, err, name, orUnknown(result.Previous))
		}
		result.InstalledPackages = missing
		result.MutationApplied = true
		if err := ops.reloadFPM(ctx, version); err != nil {
			return fail(errCodePHPToolExtensionsFailed, kind("fpm_reload_failed"),
				"PHP-FPM %s did not reload after its extensions were installed (%v); %s still runs on PHP %s", version, err, name, orUnknown(result.Previous))
		}
	}
	if err := ops.cliProbe(ctx, tool, version); err != nil {
		return fail(errCodePHPToolExtensionsFailed, phpToolCauseOf(err, "other", version), "%v; %s still runs on PHP %s", err, name, orUnknown(result.Previous))
	}

	// 2. The new configuration, validated before nginx reloads it. A
	// configuration changed outside CelikPanel is not overwritten.
	path, content, err := ops.render(tool, socket)
	if err != nil {
		var edited *errPHPToolConfigOwnerEdited
		if errors.As(err, &edited) {
			return fail(errCodePHPToolConfigOwnerEdited, transport.PHPToolCause{Kind: "config_owner_edited", Detail: edited.Path}, "%v", err)
		}
		return fail(errCodePHPToolConfigRejected, kind("config_prepare_failed"), "the configuration for %s could not be prepared: %v; nothing else was changed", name, err)
	}
	previous, err := ops.snapshotConf(path)
	if err != nil {
		return fail(errCodePHPToolConfigRejected, kind("config_read_failed"), "the current configuration of %s could not be read: %v; nothing else was changed", name, err)
	}
	// restore puts the previous file back (mode and owner kept) and, when
	// nginx already loaded the new one, checks and reloads; it says which
	// step failed, so the message never claims a restore that happened.
	restore := func(reload bool) (string, error) {
		if err := ops.restoreConf(previous); err != nil {
			return "file", err
		}
		if reload {
			if err := ops.nginxTest(ctx); err != nil {
				return "test", err
			}
			if err := ops.nginxReload(ctx); err != nil {
				return "reload", err
			}
		}
		return "", nil
	}
	restoreSentence := func(step string, restoreErr error) string {
		switch step {
		case "file":
			return fmt.Sprintf("the previous configuration could not be put back (%v); the server administrator checks %s", restoreErr, path)
		case "test":
			return fmt.Sprintf("the previous configuration was put back, but nginx rejected it (%v); the server administrator checks the web server configuration", restoreErr)
		default:
			return fmt.Sprintf("the previous configuration was put back, but nginx did not reload it (%v); the server administrator reloads nginx", restoreErr)
		}
	}
	if err := ops.writeConf(path, content); err != nil {
		if step, restoreErr := restore(false); restoreErr != nil {
			return fail(errCodePHPToolConfigRejected, kind("config_write_failed"), "the configuration for %s could not be written: %v; %s", name, err, restoreSentence(step, restoreErr))
		}
		return fail(errCodePHPToolConfigRejected, kind("config_write_failed"), "the configuration for %s could not be written: %v; the previous one is kept", name, err)
	}
	if err := ops.nginxTest(ctx); err != nil {
		if step, restoreErr := restore(false); restoreErr != nil {
			return fail(errCodePHPToolConfigRejected, kind("nginx_rejected"), "nginx rejected the new configuration (%v); %s", err, restoreSentence(step, restoreErr))
		}
		return fail(errCodePHPToolConfigRejected, kind("nginx_rejected"), "nginx rejected the new configuration (%v); the previous one was put back and %s still runs on PHP %s", err, name, orUnknown(result.Previous))
	}
	if err := ops.nginxReload(ctx); err != nil {
		step, restoreErr := restore(true)
		result.MutationApplied = true
		result.Restored = restoreErr == nil
		if restoreErr != nil {
			return fail(errCodePHPToolConfigRejected, kind("nginx_reload_failed"), "nginx did not reload (%v); %s", err, restoreSentence(step, restoreErr))
		}
		return fail(errCodePHPToolConfigRejected, kind("nginx_reload_failed"), "nginx did not reload (%v); the previous configuration was restored", err)
	}
	result.MutationApplied = true

	// 3. Proof: the tool answers as itself on the new version.
	if err := ops.httpProbe(ctx, tool, version); err != nil {
		cause := phpToolCauseOf(err, "other", version)
		if step, restoreErr := restore(true); restoreErr != nil {
			return fail(errCodePHPToolProbeFailed, cause, "%s did not work on PHP %s (%v); %s", name, version, err, restoreSentence(step, restoreErr))
		}
		result.Restored = true
		return fail(errCodePHPToolProbeFailed, cause, "%s did not work on PHP %s (%v); it was put back on PHP %s", name, version, err, orUnknown(result.Previous))
	}
	if err := ops.writeRecord(tool, version); err != nil {
		result.RecordWarning = err.Error()
	}
	return result, nil
}

func orUnknown(version string) string {
	if version == "" {
		return "its previous version"
	}
	return version
}

// renderPHPToolConfig renders one tool's configuration with a new socket,
// keeping every other tool's line as it is.
func renderPHPToolConfig(tool, socket string) (string, []byte, error) {
	if err := ensurePHPToolCheckScript(); err != nil {
		return "", nil, err
	}
	path := phpToolDBToolsConfPath
	if tool == phpToolWebmail {
		path = phpToolWebmailConfPath
	}
	if err := refuseOwnerEditedConfig(path); err != nil {
		return "", nil, err
	}
	if tool == phpToolWebmail {
		webmailSocket, err := webmailSocketPathForNginx()
		if err != nil {
			return "", nil, err
		}
		return phpToolWebmailConfPath, []byte(renderWebmailNginxConfig(webmailSocket, roundcubeRootDir(), socket)), nil
	}
	tools, roots := installedDBToolRoots()
	sockets := map[string]string{}
	for _, state := range phpToolStates() {
		if state.Tool == tool || roots[state.Tool] == "" {
			continue
		}
		current, err := phpToolSocket(state.Tool, state.Socket)
		if err != nil && current == "" {
			return "", nil, err
		}
		sockets[state.Tool] = current
	}
	sockets[tool] = socket
	return phpToolDBToolsConfPath, []byte(renderDBToolsConfig(tools, roots, sockets)), nil
}

// writePHPToolConfFile publishes a tool's configuration atomically (a
// temporary file renamed into place): webmail's root-only 0600 as before,
// the database tools' 0644 owned by root.
// Bir aracın yapılandırması atomik yayımlanır.
func writePHPToolConfFile(path string, content []byte) error {
	if path == phpToolWebmailConfPath {
		if err := secureWriteConfig(path, content, 0o600); err != nil {
			return err
		}
		uid, gid := webmailConfigMetadataIdentity(path)
		return webmailSetConfigMetadata(path, 0o600, uid, gid)
	}
	return writeDBToolsConfFile(path, content)
}

// writeDBToolsConfFile is the atomic write of the database tools'
// configuration; a variable so unprivileged tests can use a plain write.
var writeDBToolsConfFile = func(path string, content []byte) error {
	if path != dbToolsConfPath {
		return secureWriteConfig(path, content, 0o644)
	}
	return secureWriteConfigOwnedBy(path, content, 0o644, 0, 0)
}

// errPHPToolConfigOwnerEdited: the tool's configuration is not the form
// CelikPanel writes, so it was changed outside CelikPanel (D-022).
type errPHPToolConfigOwnerEdited struct{ Path string }

func (e *errPHPToolConfigOwnerEdited) Error() string {
	return fmt.Sprintf("%s was changed outside CelikPanel, so CelikPanel does not overwrite it; move your changes to another file in /etc/nginx/conf.d (or delete this one to let CelikPanel write it again), then try again", e.Path)
}

var (
	dbToolLocationBlock = regexp.MustCompile(`(?m)^    location /dbtool/([a-z]+)/ \{\n        alias (\S+)/;`)
	webmailListenLine   = regexp.MustCompile(`listen unix:([^;\s]+);`)
	webmailAliasLine    = regexp.MustCompile(`location /webmail/ \{\n        alias (\S+)/;`)
	// alpha.1 to alpha.12 served webmail on a loopback address.
	webmailLoopbackListenLine = regexp.MustCompile(`(?m)^    listen (127\.0\.0\.1:[0-9]+);$`)
)

// loopbackRenderWebmailNginxConfig is the form alpha.1 to alpha.12 wrote
// (webmail on a loopback address), still CelikPanel's own.
func loopbackRenderWebmailNginxConfig(listenAddr, roundcubeRoot, fpmSocket string) string {
	return fmt.Sprintf(`# Managed by CelikPanel — Roundcube webmail, loopback only.
# The panel reverse-proxies /webmail/ here; Roundcube's own login is the auth.
server {
    listen %s;
    server_name _;
    client_max_body_size 25m;

    location /webmail/ {
        alias %s/;
        index index.php;
        try_files $uri $uri/ /webmail/index.php$is_args$args;

        location ~ ^/webmail/(.+\.php)$ {
            alias %s/$1;
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $request_filename;
            fastcgi_pass unix:%s;
        }
    }
}
`, listenAddr, roundcubeRoot, roundcubeRoot, fpmSocket)
}

// legacyRenderDBToolsConfig is the form alpha.103 and earlier wrote (since
// 2026-08-11), still accepted as CelikPanel's own.
func legacyRenderDBToolsConfig(tools []string, roots, sockets map[string]string) string {
	var b strings.Builder
	b.WriteString("# Managed by CelikPanel — database web tools, loopback only.\n")
	b.WriteString("# The panel reverse-proxies /dbtool/ here behind an admin session.\n")
	b.WriteString("server {\n")
	fmt.Fprintf(&b, "    listen %s;\n", dbToolsAddr)
	b.WriteString("    server_name _;\n")
	for _, id := range tools {
		root := roots[id]
		fmt.Fprintf(&b, `
    location /dbtool/%s/ {
        alias %s/;
        index index.php;
        location ~ ^/dbtool/%s/(.+\.php)$ {
            alias %s/$1;
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $request_filename;
            fastcgi_pass unix:%s;
        }
    }
`, id, root, id, root, sockets[id])
	}
	b.WriteString("}\n")
	return b.String()
}

// managedDBToolsConfig: the content is one of the forms CelikPanel writes,
// for the tools, roots and sockets it names (an owner's edit of a
// fastcgi_pass line alone is still CelikPanel's form with that socket).
func managedDBToolsConfig(content string) bool {
	var tools []string
	roots := map[string]string{}
	for _, match := range dbToolLocationBlock.FindAllStringSubmatch(content, -1) {
		tools = append(tools, match[1])
		roots[match[1]] = match[2]
	}
	sockets := parseDBToolsFPMSockets(content)
	return content == renderDBToolsConfig(tools, roots, sockets) || content == legacyRenderDBToolsConfig(tools, roots, sockets)
}

func managedWebmailConfig(content string) bool {
	alias := webmailAliasLine.FindStringSubmatch(content)
	if alias == nil {
		return false
	}
	fpm := parseWebmailFPMSocket(content)
	if loopback := webmailLoopbackListenLine.FindStringSubmatch(content); loopback != nil {
		return content == loopbackRenderWebmailNginxConfig(loopback[1], alias[1], fpm)
	}
	listen := webmailListenLine.FindStringSubmatch(content)
	if listen == nil {
		return false
	}
	return content == renderWebmailNginxConfig(listen[1], alias[1], fpm) || content == legacyRenderWebmailNginxConfig(listen[1], alias[1], fpm)
}

// refuseOwnerEditedConfig returns errPHPToolConfigOwnerEdited when the file
// exists and is not CelikPanel's own form.
func refuseOwnerEditedConfig(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// An empty file is CelikPanel's own: an older non-atomic write could
	// leave one; it holds nothing of the owner's.
	// Boş dosya CelikPanel'indir; sahibin hiçbir şeyini tutmaz.
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	managed := managedDBToolsConfig(string(data))
	if path == phpToolWebmailConfPath {
		managed = managedWebmailConfig(string(data))
	}
	if !managed {
		return &errPHPToolConfigOwnerEdited{Path: path}
	}
	return nil
}

func hostPHPToolBindOps() phpToolBindOps {
	return phpToolBindOps{
		family:           detectPkgFamily,
		toolInstalled:    phpToolInstalledCheck,
		state:            phpToolStateFor,
		targetSocket:     bindPHPToolSocket,
		socketReady:      pathExists,
		packageInstalled: phpToolPackageInstalled,
		installPackages: func(ctx context.Context, packages []string) error {
			_, err := installPackagesContext(ctx, "apt", packages)
			return err
		},
		reloadFPM: func(ctx context.Context, version string) error {
			out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "php"+version+"-fpm")
			if err != nil {
				return errors.New(commandFailureDetail("systemctl reload", out, err))
			}
			return nil
		},
		cliProbe:     probePHPToolCLI,
		render:       renderPHPToolConfig,
		snapshotConf: snapshotMailFile,
		writeConf:    writePHPToolConfFile,
		restoreConf:  restoreMailFile,
		nginxTest: func(ctx context.Context) error {
			out, err := serviceMutationCommand(ctx, "nginx", "-t").CombinedOutput()
			if err != nil {
				return errors.New(firstLine(string(out)))
			}
			return nil
		},
		nginxReload: func(ctx context.Context) error {
			out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "nginx")
			if err != nil {
				return errors.New(commandFailureDetail("nginx reload", out, err))
			}
			return nil
		},
		httpProbe: func(ctx context.Context, tool, version string) error {
			return retryPHPToolProbe(ctx, tool, version, probePHPToolHTTP, phpToolProbeSettle, time.Second)
		},
		writeRecord: writePHPToolBinding,
	}
}

// ensurePHPToolExtensions installs a tool's missing extension packages for
// its version during a regeneration (apt only; best effort, reported).
func ensurePHPToolExtensions(ctx context.Context, tool, version string) error {
	if version == "" || detectPkgFamily() != "apt" {
		return nil
	}
	missing := missingPHPToolExtensionPackages(tool, version, phpToolPackageInstalled)
	if len(missing) == 0 {
		return nil
	}
	if _, err := installPackagesContext(ctx, "apt", missing); err != nil {
		return fmt.Errorf("the PHP %s extensions %s needs (%s) could not be installed: %v", version, phpToolDisplayName(tool), strings.Join(missing, ", "), err)
	}
	if out, err := runServiceMutationCombinedOutput(ctx, "systemctl", "reload", "php"+version+"-fpm"); err != nil {
		return fmt.Errorf("PHP-FPM %s did not reload after its extensions were installed: %s", version, commandFailureDetail("systemctl reload", out, err))
	}
	return nil
}

// phpToolBindingsView answers the read-only binding RPC.
func phpToolBindingsView() transport.PHPToolBindingsResponse {
	response := transport.PHPToolBindingsResponse{Tools: []transport.PHPToolBinding{}, Versions: installedPHPFPMVersions()}
	proofFailures := readPHPToolProofFailures()
	for _, state := range phpToolStates() {
		binding := transport.PHPToolBinding{Tool: state.Tool, Installed: state.Installed, Source: state.Source}
		// alpha.105: a tool whose proof after its install failed on every
		// version tried reads "Not ready" with the cause.
		if failure, failed := proofFailures[state.Tool]; failed && state.Installed {
			copied := failure.Cause
			binding.ProofFailed, binding.ProofCause, binding.ProofTriedOthers = true, &copied, failure.TriedOthers
		}
		if state.Socket != "" {
			binding.Version = state.version()
			binding.SocketReady = pathExists(state.Socket)
			binding.VersionInstalled = binding.Version != "" && phpVersionInstalled(binding.Version)
			// Package presence only (a package can be installed and its
			// extension disabled; a move's proof checks what PHP loads).
			binding.ExtensionsPresent = binding.Version == "" || detectPkgFamily() != "apt" ||
				phpToolHasExtensionPackages(state.Tool, binding.Version)
		}
		response.Tools = append(response.Tools, binding)
	}
	return response
}
