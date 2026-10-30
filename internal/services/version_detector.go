package services

import (
	"os/exec"
	"regexp"
	"strings"
)

// VersionDetector provides generic version detection for any service
type VersionDetector struct{}

func NewVersionDetector() *VersionDetector {
	return &VersionDetector{}
}

// DetectVersion returns the version of the few services whose version can be
// read without starting anything (special cases below), or "unknown". It never
// runs a service's own program with guessed flags: a daemon that does not know
// "--version" or "version" starts a second copy instead (Debian's cron logged
// "DEATH (can't lock /var/run/crond.pid)" on every service list). The Agent
// fills "unknown" from the package database (UpstreamPackageVersion).
// DetectVersion hizmet programını tahmini bayraklarla asla çalıştırmaz; cron
// gibi bir program ikinci kopyasını başlatırdı. Bilinmeyen sürüm paket
// veritabanından doldurulur.
func (vd *VersionDetector) DetectVersion(serviceName string) string {
	if version := vd.detectSpecialCases(serviceName); version != "" {
		return version
	}
	return "unknown"
}

var upstreamVersionPrefix = regexp.MustCompile(`^\d+(?:\.\d+)*`)

// UpstreamPackageVersion turns a package database version into the version a
// screen shows: the epoch ("1:") and the distribution revision ("-0+deb13u1",
// "-1") are dropped, then the leading numeric part is kept
// ("1:10.11.13-0+deb13u1" -> "10.11.13", "3.0pl1-197" -> "3.0",
// "5.1.5-1" -> "5.1.5"). "" when nothing numeric remains.
// Paket sürümünden ekranda gösterilen sürüm.
func UpstreamPackageVersion(packageVersion string) string {
	value := strings.TrimSpace(packageVersion)
	if _, rest, ok := strings.Cut(value, ":"); ok {
		value = rest
	}
	if i := strings.LastIndex(value, "-"); i > 0 {
		value = value[:i]
	}
	return upstreamVersionPrefix.FindString(value)
}

// detectSpecialCases handles services with non-standard version commands
func (vd *VersionDetector) detectSpecialCases(serviceName string) string {
	switch {
	case strings.Contains(serviceName, "php") && strings.Contains(serviceName, "fpm"):
		// php8.3-fpm -> 8.3
		parts := strings.Split(serviceName, "-")
		if len(parts) > 0 && strings.HasPrefix(parts[0], "php") {
			return strings.TrimPrefix(parts[0], "php")
		}

	case serviceName == "phpsessionclean":
		return "-"

	case strings.Contains(serviceName, "postgres") && strings.Contains(serviceName, "@"):
		// postgresql@16-main -> 16
		parts := strings.Split(serviceName, "@")
		if len(parts) > 1 {
			versionParts := strings.Split(parts[1], "-")
			if len(versionParts) > 0 {
				return versionParts[0]
			}
		}

	case strings.Contains(serviceName, "mariadb") || strings.Contains(serviceName, "mysql"):
		// The server program is asked, never the client (11 Oct 2026).
		// `mariadb --version` is the client's line: on Ubuntu 24.04 it reads
		// "mariadb  Ver 15.1 Distrib 10.11.14-MariaDB", and "15.1", the
		// client's own version, was shown as the server's. What the server
		// program does not answer is "unknown" here; the Panel then asks the
		// running server (MariaDBDriver.ServerVersion).
		// Sunucu programina sorulur, istemciye asla.
		for _, program := range []string{"mariadbd", "/usr/sbin/mariadbd", "mysqld", "/usr/sbin/mysqld"} {
			output, err := exec.Command(program, "--version").CombinedOutput()
			if err != nil {
				continue
			}
			if version := mariaDBServerProgramVersion(string(output)); version != "" {
				return version
			}
		}
		return "unknown"

	case strings.Contains(serviceName, "postfix"):
		cmd := exec.Command("postconf", "-d", "mail_version")
		if output, err := cmd.CombinedOutput(); err == nil {
			parts := strings.Split(string(output), "=")
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

var mariaDBServerProgramLine = regexp.MustCompile(`(?m)^\S*(?:mariadbd|mysqld)\s+Ver\s+(\d+\.\d+\.\d+)`)

// mariaDBServerProgramVersion reads the version from what `mariadbd --version`
// (or `mysqld --version`) prints: "mariadbd  Ver 10.11.14-MariaDB-... for
// debian-linux-gnu on x86_64 (...)". Only a line that starts with the server
// program's name counts, so a client's "Ver 15.1 Distrib ..." never does.
func mariaDBServerProgramVersion(output string) string {
	if match := mariaDBServerProgramLine.FindStringSubmatch(output); match != nil {
		return match[1]
	}
	return ""
}
