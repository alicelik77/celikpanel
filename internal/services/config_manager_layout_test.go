package services

import (
	"os"
	"path/filepath"
	"testing"
)

// The MariaDB settings editor uses the distribution's own server file:
// Debian/Ubuntu's 50-server.cnf, else Arch Linux's /etc/my.cnf.d/server.cnf.
func TestMySQLServerConfigFileFollowsTheDistributionLayout(t *testing.T) {
	dir := t.TempDir()
	debian := filepath.Join(dir, "mysql", "mariadb.conf.d", "50-server.cnf")
	arch := filepath.Join(dir, "my.cnf.d", "server.cnf")
	oldPath, oldCandidates := mysqlServerConfigPath, mysqlServerConfigCandidates
	t.Cleanup(func() { mysqlServerConfigPath, mysqlServerConfigCandidates = oldPath, oldCandidates })
	mysqlServerConfigPath = ""
	mysqlServerConfigCandidates = []string{debian, arch}

	if got := mysqlServerConfigFile(); got != debian {
		t.Fatalf("nothing present: %s", got)
	}
	if err := os.MkdirAll(filepath.Dir(arch), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(arch, []byte("[server]\n[mariadb-13.0]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mysqlServerConfigFile(); got != arch {
		t.Fatalf("Arch layout: %s", got)
	}
	if err := os.MkdirAll(filepath.Dir(debian), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(debian, []byte("[mysqld]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mysqlServerConfigFile(); got != debian {
		t.Fatalf("Debian layout first: %s", got)
	}
	mysqlServerConfigPath = "/explicit"
	if got := mysqlServerConfigFile(); got != "/explicit" {
		t.Fatalf("explicit path: %s", got)
	}
}
