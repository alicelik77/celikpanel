package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
// The rule now: the version a tool is bound to is what its generated
// configuration names (fastcgi_pass). A regeneration keeps a tool on that
// version while the version is installed. Only a tool with nothing bound
// yet gets a version: the distribution's default PHP when it can be read
// (the php-fpm.sock alternative, else the php command's alternative), else
// the numerically newest installed version. The choice is always written as
// the versioned socket, never the floating alias. When the bound version is
// no longer installed the tool's configuration is left as it is and the
// caller reports it; moving a tool to another version is a separate,
// explicit action (not in alpha.104; bindPHPToolSocket is its seam).
// Removing a PHP version a tool is bound to is refused (REMOVAL_TOOLS_BOUND).
//
// Bir aracın PHP sürümü, onun için üretilen yapılandırmanın adını verdiği
// sürümdür; yeniden üretim aracı o sürüm kurulu kaldıkça orada tutar. Bağlı
// olduğu sürümü kaldırmak reddedilir.

const (
	phpToolWebmail    = "webmail"
	phpToolPHPMyAdmin = "phpmyadmin"
	phpToolPHPPgAdmin = "phppgadmin"
)

// Paths are variables so tests can point them at a temporary tree.
var (
	phpFPMRunDir            = "/run/php"
	phpEtcDir               = "/etc/php"
	phpCLIAlternative       = "/usr/bin/php"
	phpToolWebmailConfPath  = webmailConfPath
	phpToolDBToolsConfPath  = dbToolsConfPath
	phpFallbackSocketGlobs  = []string{"/run/php-fpm/php-fpm.sock", "/var/run/php-fpm/php-fpm.sock"}
	versionedFPMSocketName  = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})-fpm\.sock$`)
	phpCLIVersionedName     = regexp.MustCompile(`^php([0-9]{1,2}\.[0-9]{1,2})$`)
	fastcgiPassUnix         = regexp.MustCompile(`fastcgi_pass\s+unix:([^;\s]+)\s*;`)
	dbToolLocationStartLine = regexp.MustCompile(`^\s*location\s+/dbtool/([a-z]+)/\s*\{`)
)

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
		"%s runs on PHP %s, which is no longer installed; install PHP-FPM %s again from Components (PHP-FPM versions) and try again — moving it to another PHP version is not available yet",
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
	if version == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(phpEtcDir, version, "fpm"))
	return err == nil && info.IsDir()
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

// currentPHPToolSockets reads what each tool is bound to today.
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

// resolvedSocketVersion follows a floating alias (php-fpm.sock) to the
// version it points at today.
func resolvedSocketVersion(socket string) string {
	if version := fpmSocketVersion(socket); version != "" {
		return version
	}
	if resolved, err := filepath.EvalSymlinks(socket); err == nil {
		return fpmSocketVersion(resolved)
	}
	return ""
}

// phpToolsBoundToVersion lists the tools bound to a PHP version ("8.4"); an
// empty version lists every tool bound to any PHP-FPM socket.
func phpToolsBoundToVersion(version string) []string {
	bound := []string{}
	for tool, socket := range currentPHPToolSockets() {
		if version == "" || resolvedSocketVersion(socket) == version {
			bound = append(bound, tool)
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

// newestVersionedFPMSocket: the numerically newest versioned socket (8.10
// is newer than 8.9; a lexical sort said otherwise).
func newestVersionedFPMSocket() string {
	matches, _ := filepath.Glob(filepath.Join(phpFPMRunDir, "php*-fpm.sock"))
	best, bestVersion := "", ""
	for _, match := range matches {
		version := fpmSocketVersion(match)
		if version == "" {
			continue
		}
		if best == "" || comparePHPVersions(version, bestVersion) > 0 {
			best, bestVersion = match, version
		}
	}
	return best
}

// defaultFPMSocket is the socket for a tool with nothing bound yet.
func defaultFPMSocket() string {
	// 1. The php-fpm.sock alternative points at the distribution default.
	if version := resolvedSocketVersion(filepath.Join(phpFPMRunDir, "php-fpm.sock")); version != "" {
		if socket := versionedFPMSocketPath(version); pathExists(socket) {
			return socket
		}
	}
	// 2. The php command's alternative (/usr/bin/php → php8.4).
	if resolved, err := filepath.EvalSymlinks(phpCLIAlternative); err == nil {
		if match := phpCLIVersionedName.FindStringSubmatch(filepath.Base(resolved)); match != nil {
			if socket := versionedFPMSocketPath(match[1]); pathExists(socket) {
				return socket
			}
		}
	}
	// 3. The numerically newest versioned socket.
	if socket := newestVersionedFPMSocket(); socket != "" {
		return socket
	}
	// 4. Unversioned layouts (Arch: /run/php-fpm/php-fpm.sock).
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
	socket := defaultFPMSocket()
	if socket == "" {
		return "", fmt.Errorf("PHP-FPM is not installed")
	}
	return socket, nil
}

// bindPHPToolSocket is the seam for the explicit "run this tool on PHP X"
// action the owner is shaping (not offered in alpha.104): it validates the
// target version and returns its socket; the caller ensures the tool's
// extensions for that version, probes them and regenerates the tool's
// configuration with this socket. Nothing calls it with a version other than
// the bound one yet.
func bindPHPToolSocket(tool, version string) (string, error) {
	if !phpCLIVersionedName.MatchString("php" + version) {
		return "", fmt.Errorf("invalid PHP version %q", version)
	}
	if !phpVersionInstalled(version) {
		return "", fmt.Errorf("PHP %s is not installed", version)
	}
	switch tool {
	case phpToolWebmail, phpToolPHPMyAdmin, phpToolPHPPgAdmin:
	default:
		return "", fmt.Errorf("unknown PHP tool %q", tool)
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
