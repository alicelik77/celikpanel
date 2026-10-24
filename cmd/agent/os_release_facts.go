package main

// The operating-system release check (alpha.107, D-038 §17). An OS release
// upgrade (Debian 13 to 14) is the owner's step: CelikPanel never starts it.
// After one, the Panel compares what the Agent reads here with what it
// recorded before: the release, the suite each enabled vendor repository
// names, the PostgreSQL clusters, the PHP-FPM lines and MariaDB's version.
// Everything here is read; nothing is changed.
//
// İşletim sistemi sürüm denetimi: yalnız okunur. Sürüm yükseltmesini sahibi
// yapar; CelikPanel başlatmaz.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostplatform"
	"github.com/alicelik/celikpanel/internal/transport"
)

var (
	osReleaseFactsPath = "/etc/os-release"
	postgresqlConfDir  = "/etc/postgresql"
	phpFPMPackageRe    = regexp.MustCompile(`^php(\d+\.\d+)-fpm$`)
	debVersionMajorRe  = regexp.MustCompile(`^(?:\d+:)?(\d+\.\d+)`)
	pgClusterPartRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

func osReleaseFacts() transport.OSReleaseFactsResponse {
	var resp transport.OSReleaseFactsResponse
	data, err := os.ReadFile(osReleaseFactsPath)
	if err != nil {
		resp.Error = "the operating-system release could not be read: " + err.Error()
		return resp
	}
	release, err := hostplatform.ParseOSRelease(data)
	if err != nil {
		resp.Error = "the operating-system release could not be read: " + err.Error()
		return resp
	}
	resp.ID, resp.VersionID, resp.Codename = release.ID, release.Version, release.Codename
	resp.PrettyName = prettyNameOf(data)
	for id, loc := range enabledRepoLocations() {
		if resp.RepoSuites == nil {
			resp.RepoSuites = map[string]string{}
		}
		resp.RepoSuites[id] = loc.Suite
	}
	resp.PostgreSQLClusters = postgresqlClusters(postgresqlConfDir)
	if installed, err := readDpkgStatusVersions(dpkgStatusPath); err == nil {
		resp.PHPVersions, resp.MariaDBVersion = phpAndMariaDBFrom(installed)
	}
	return resp
}

// prettyNameOf is PRETTY_NAME, bounded and plain.
func prettyNameOf(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			value = strings.Trim(value, `"'`)
			if len(value) > 120 {
				value = value[:120]
			}
			return strings.Map(func(r rune) rune {
				if r < 0x20 || r == 0x7f {
					return -1
				}
				return r
			}, value)
		}
	}
	return ""
}

// postgresqlClusters lists "version/name" for every cluster directory with a
// postgresql.conf, as Debian's postgresql-common lays them out.
func postgresqlClusters(root string) []string {
	versions, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, version := range versions {
		if !version.IsDir() || !pgClusterPartRe.MatchString(version.Name()) {
			continue
		}
		clusters, err := os.ReadDir(filepath.Join(root, version.Name()))
		if err != nil {
			continue
		}
		for _, cluster := range clusters {
			if !cluster.IsDir() || !pgClusterPartRe.MatchString(cluster.Name()) {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, version.Name(), cluster.Name(), "postgresql.conf")); err == nil {
				out = append(out, version.Name()+"/"+cluster.Name())
			}
		}
		if len(out) > 32 {
			break
		}
	}
	sort.Strings(out)
	return out
}

func phpAndMariaDBFrom(installed map[string][]string) (php []string, mariadb string) {
	for name := range installed {
		if match := phpFPMPackageRe.FindStringSubmatch(name); match != nil {
			php = append(php, match[1])
		}
	}
	sort.Strings(php)
	if versions := installed["mariadb-server"]; len(versions) > 0 {
		if match := debVersionMajorRe.FindStringSubmatch(versions[0]); match != nil {
			mariadb = match[1]
		}
	}
	return php, mariadb
}

// OSReleaseFactsV1 answers the operating-system release check. Read only.
func (a *Agent) OSReleaseFactsV1(_ *transport.Empty, resp *transport.OSReleaseFactsResponse) error {
	*resp = osReleaseFacts()
	return nil
}
