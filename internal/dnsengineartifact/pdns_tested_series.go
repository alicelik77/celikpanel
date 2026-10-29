package dnsengineartifact

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/debversion"
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

// pdnsDebianArchiveRevision is the revision the Debian archive itself gives a
// PowerDNS release ("1", "0+deb13u1", "2+deb13u3"). A backport ("~bpo13+1"),
// the PowerDNS repository's builds ("1pdns.trixie"), an Ubuntu or vendor
// rebuild carry other revisions.
var pdnsDebianArchiveRevision = regexp.MustCompile(`^[0-9]+(\+deb[0-9]+u[0-9]+)?$`)

// PDNSFreshPrimaryVersionInTestedSeries reports whether a full package
// version is the Debian archive's build of a release of the tested series:
// a valid Debian version with no epoch, a Debian archive revision, and an
// upstream major.minor.patch with a numeric patch. Pre-releases, other
// series, epochs, backports and the PowerDNS repository's builds are not.
// Where the version comes from is checked apart, at the install
// (PDNSOfficialDebianArchive); an installed package can no longer show it.
func PDNSFreshPrimaryVersionInTestedSeries(version string) bool {
	if !debversion.Valid(version) {
		return false
	}
	v, ok := debversion.Parse(version)
	if !ok || v.Epoch != "" || !pdnsDebianArchiveRevision.MatchString(v.Revision) {
		return false
	}
	patch, ok := strings.CutPrefix(v.Upstream, PDNSFreshPrimaryTestedSeries+".")
	return ok && allDigits(patch)
}

// PDNSArchive is one archive APT names for a candidate version: the Release
// file's Origin and the archive (suite).
type PDNSArchive struct {
	Origin, Suite string
}

// PDNSOfficialDebianArchive: every archive APT names for the candidate is
// Debian's own released suite or its security/updates pocket. A vendor or
// third-party repository (repo.powerdns.com), backports, testing/unstable or
// no archive at all (an unreadable index) is not.
func PDNSOfficialDebianArchive(archives []PDNSArchive) bool {
	if len(archives) == 0 {
		return false
	}
	for _, archive := range archives {
		if archive.Origin != "Debian" && archive.Origin != "Debian-Security" {
			return false
		}
		suite := strings.ToLower(archive.Suite)
		for _, refused := range []string{"backports", "proposed", "experimental", "unstable", "testing", "sid"} {
			if strings.Contains(suite, refused) {
				return false
			}
		}
	}
	return true
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
