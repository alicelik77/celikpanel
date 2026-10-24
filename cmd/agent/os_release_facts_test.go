package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheReleaseFactsAreReadWithoutChangingAnything(t *testing.T) {
	dir := t.TempDir()
	restoreRelease, restorePG, restoreStatus := osReleaseFactsPath, postgresqlConfDir, dpkgStatusPath
	t.Cleanup(func() {
		osReleaseFactsPath, postgresqlConfDir, dpkgStatusPath = restoreRelease, restorePG, restoreStatus
	})
	osReleaseFactsPath = filepath.Join(dir, "os-release")
	postgresqlConfDir = filepath.Join(dir, "postgresql")
	dpkgStatusPath = filepath.Join(dir, "status")
	os.WriteFile(osReleaseFactsPath, []byte("PRETTY_NAME=\"Debian GNU/Linux 14 (forky)\"\nID=debian\nVERSION_ID=\"14\"\nVERSION_CODENAME=forky\n"), 0o644)
	for _, cluster := range []string{"17/main", "18/main"} {
		os.MkdirAll(filepath.Join(postgresqlConfDir, cluster), 0o755)
		os.WriteFile(filepath.Join(postgresqlConfDir, cluster, "postgresql.conf"), nil, 0o644)
	}
	os.MkdirAll(filepath.Join(postgresqlConfDir, "18", "empty"), 0o755)
	os.WriteFile(dpkgStatusPath, []byte(strings.Join([]string{
		"Package: php8.4-fpm\nStatus: install ok installed\nVersion: 8.4.11-1\n",
		"Package: php8.5-fpm\nStatus: install ok installed\nVersion: 8.5.0-1\n",
		"Package: mariadb-server\nStatus: install ok installed\nVersion: 1:11.8.3-0+deb14u1\n",
		"Package: php8.2-fpm\nStatus: deinstall ok config-files\nVersion: 8.2.29-1\n",
	}, "\n")), 0o644)
	facts := osReleaseFacts()
	if facts.ID != "debian" || facts.VersionID != "14" || facts.Codename != "forky" || facts.PrettyName != "Debian GNU/Linux 14 (forky)" {
		t.Fatalf("release = %+v", facts)
	}
	if strings.Join(facts.PostgreSQLClusters, ",") != "17/main,18/main" {
		t.Fatalf("clusters = %v", facts.PostgreSQLClusters)
	}
	if strings.Join(facts.PHPVersions, ",") != "8.4,8.5" || facts.MariaDBVersion != "11.8" {
		t.Fatalf("php = %v mariadb = %q", facts.PHPVersions, facts.MariaDBVersion)
	}
}
