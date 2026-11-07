package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129 lab (Arch): the certificate root was made 0750 root:celikpanel
// under the Agent's umask 0027, so the postgres user could not reach its own
// folder and PostgreSQL did not start after the administrator's restart.
// The root is 0755 every time, also when an earlier Agent left it 0750.
func TestDatabaseRemoteCertRootIsTraversableForTheEngines(t *testing.T) {
	saved := databaseRemoteCertRoot
	t.Cleanup(func() { databaseRemoteCertRoot = saved })
	databaseRemoteCertRoot = filepath.Join(t.TempDir(), "celikpanel-database")
	engineDir := filepath.Join(databaseRemoteCertRoot, "postgresql")
	if err := os.MkdirAll(engineDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(databaseRemoteCertRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(engineDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := ensureDatabaseRemoteCertificate("postgresql", "nogroup-needed", "host1.example.test"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(databaseRemoteCertRoot); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("certificate root mode = %v, %v; want 0755", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(engineDir); err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("the engine's own folder keeps 0750: %v %v", info.Mode().Perm(), err)
	}
}

func TestDatabaseRemoteCertRootMadeUnderTheAgentUmaskIsTraversable(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("making the pair changes ownership (root)")
	}
	saved := databaseRemoteCertRoot
	t.Cleanup(func() { databaseRemoteCertRoot = saved })
	databaseRemoteCertRoot = filepath.Join(t.TempDir(), "celikpanel-database")
	old := syscall.Umask(0o027)
	t.Cleanup(func() { syscall.Umask(old) })
	g, err := user.LookupGroupId(strconv.Itoa(os.Getgid()))
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := ensureDatabaseRemoteCertificate("mariadb", g.Name, "host1.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(databaseRemoteCertRoot); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("certificate root mode = %v, %v; want 0755", info.Mode().Perm(), err)
	}
	for path, want := range map[string]os.FileMode{cert: 0o644, key: 0o640} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode = %v, %v; want %v", path, info.Mode().Perm(), err, want)
		}
	}
}

// alpha.129 lab (Arch, 129 -> 131): with the root left 0750 by the earlier
// Agent, PostgreSQL could not start, so no apply reached the certificate
// step and the fix never ran. Every apply heals an existing root first, also
// while the engine is down; a missing root is not made.
func TestDatabaseRemoteApplyHealsTheCertRootWhileTheEngineIsDown(t *testing.T) {
	saved := databaseRemoteCertRoot
	t.Cleanup(func() { databaseRemoteCertRoot = saved })
	// The fakes swap the root too; this test sets its own after them.
	fakeFilterHost(t)
	f := &fakePGCluster{listen: "*", ssl: true, preload: "auth_delay", hbaText: "local all postgres peer\n"}
	withFakePG(t, f)
	databaseRemotePGQuery = func(int, string) (string, error) { return "", os.ErrDeadlineExceeded }
	databaseRemoteCertRoot = filepath.Join(t.TempDir(), "celikpanel-database")
	if err := healDatabaseRemoteCertRoot(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(databaseRemoteCertRoot); !os.IsNotExist(err) {
		t.Fatalf("a missing root was made: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(databaseRemoteCertRoot, "postgresql"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(databaseRemoteCertRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	var resp transport.DatabaseRemoteResponse
	applyDatabaseRemote([]dbremote.Target{shopTarget()}, "db.example.com", nil, &resp)
	if info, err := os.Stat(databaseRemoteCertRoot); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("certificate root mode = %v, %v; want 0755", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(filepath.Join(databaseRemoteCertRoot, "postgresql")); err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("the engine's own folder keeps 0750: %v %v", info.Mode().Perm(), err)
	}
}
