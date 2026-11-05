package services

import (
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

		// MariaDB and Postfix are no longer asked (`mariadbd --version`,
		// `postconf -d mail_version`, alpha.125): their package version is the
		// same release and is already read once per scan from the package
		// database, which the Agent fills in for every "unknown" here. The
		// server program's version line was never the client's either way.
		// MariaDB ve Postfix'e artık sorulmaz; sürümleri paket veritabanından
		// doldurulur.
	}

	return ""
}
