package dnsengineartifact

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"strings"
)

// PDNSFreshPrimaryTestedSeries is the one source of truth for which PowerDNS
// builds a fresh paired PowerDNS primary may use (D-028, amended 2026-10-29).
// The wizard offers PowerDNS as the main DNS server only when the
// distribution's candidate is in this series, and the Agent's preflight, its
// post-install identity check and the independent recovery attestation accept
// exactly the same series. Patch releases inside the series are the
// distribution's security fixes; a new major or minor release is not offered
// until it is measured.
//
// PDNSFreshPrimaryTestedSeries, eşli PowerDNS birincilinin ilk kurulumunda
// kabul edilen tek PowerDNS serisidir (major.minor). Sihirbazın sunduğu ve DNS
// adımının kabul ettiği sürüm aynı kuraldan gelir; seri içi yama güncellemesi
// dağıtımın güvenlik düzeltmesidir.
const PDNSFreshPrimaryTestedSeries = "4.9"

// PDNSFreshPrimaryPackages are the distribution packages of that build. They
// come from one source package and must carry one version.
var PDNSFreshPrimaryPackages = []string{"pdns-server", "pdns-backend-sqlite3"}

// PDNSFreshPrimaryBinary is the package-owned daemon whose identity is checked.
const PDNSFreshPrimaryBinary = "/usr/sbin/pdns_server"

// PDNSUpstreamVersion returns the upstream part of a distribution package
// version ("4.9.18" for "4.9.18-0+deb13u1", an epoch removed), or the value
// itself when it already is one.
func PDNSUpstreamVersion(version string) string {
	value := strings.TrimSpace(version)
	if colon := strings.IndexByte(value, ':'); colon > 0 && allDigits(value[:colon]) {
		value = value[colon+1:]
	}
	if dash := strings.LastIndexByte(value, '-'); dash > 0 {
		value = value[:dash]
	}
	return value
}

// PDNSFreshPrimaryVersionInTestedSeries reports whether a package version (a
// full distribution version or its upstream part) is a release of the tested
// series: exactly major.minor.patch with a numeric patch. Pre-releases and
// other series are outside it.
func PDNSFreshPrimaryVersionInTestedSeries(version string) bool {
	upstream := PDNSUpstreamVersion(version)
	patch, ok := strings.CutPrefix(upstream, PDNSFreshPrimaryTestedSeries+".")
	return ok && allDigits(patch)
}

// PDNSPackageFileMD5 returns the digest dpkg recorded for one file of an
// installed package (its md5sums control file; paths there have no leading
// slash). It proves the file on disk is the package's own, unmodified; it is
// an identity check, not a defence against a root user who can change both.
func PDNSPackageFileMD5(md5sums []byte, path string) (string, bool) {
	want := strings.TrimPrefix(path, "/")
	scanner := bufio.NewScanner(bytes.NewReader(md5sums))
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	found := ""
	for scanner.Scan() {
		sum, name, ok := strings.Cut(scanner.Text(), "  ")
		if !ok || name != want {
			continue
		}
		if decoded, err := hex.DecodeString(sum); err != nil || len(decoded) != 16 || found != "" {
			return "", false
		}
		found = strings.ToLower(sum)
	}
	if scanner.Err() != nil || found == "" {
		return "", false
	}
	return found, true
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
