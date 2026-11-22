//go:build linux

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S1 (20 Nov 2026) at the unit level: the command lines are the proof.

// A restore (and every other load) never hands a file to a client started
// as the engine's administrator: MariaDB goes to the sandboxed client as the
// database's own user, with every definer written as that user.
func TestARestoreNeverLoadsAFileThroughTheAdministratorsClient(t *testing.T) {
	oldCommand, oldClient, oldRun := backupCommandContext, mariadbClient, runDatabaseClientAsSiteUser
	t.Cleanup(func() {
		backupCommandContext, mariadbClient, runDatabaseClientAsSiteUser = oldCommand, oldClient, oldRun
	})
	backupCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		t.Errorf("a file was given to %s %v", name, args)
		return exec.CommandContext(ctx, "false")
	}
	mariadbClient = func() (string, bool) { return "/usr/bin/mariadb", true }
	var argv, env []string
	var user string
	var stdin []byte
	runDatabaseClientAsSiteUser = func(ctx context.Context, username string, args []string, environment []string, r io.Reader) (string, error) {
		user, argv, env = username, args, environment
		stdin, _ = io.ReadAll(r)
		return "", nil
	}
	payload := filepath.Join(t.TempDir(), "payload.sql.gz")
	if err := writeGzipText(payload, "CREATE DEFINER=`root`@`localhost` PROCEDURE p() SELECT 1;\n\\! touch /tmp/marker\n"); err != nil {
		t.Fatal(err)
	}
	target := databaseLoadTarget{
		DatabaseIdentity: backupspec.DatabaseIdentity{ID: 5, Name: "shop", Type: "mysql"},
		Account:          databaseLoadAccount{User: "shop_user", Password: "pw"},
		SiteUser:         "shop_test",
	}
	if err := restoreDatabaseFromFile(target, payload); err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/mariadb", "--no-defaults", "--sandbox", "--batch", "--protocol=socket",
		"--user=shop_user", "--database=shop", "--default-character-set=utf8mb4", "--max-allowed-packet=1073741824",
		// S4 R2a: the user's statement time does not cut the restore.
		mariadbLoadInitCommand}
	if user != "shop_test" || strings.Join(argv, " ") != strings.Join(want, " ") || strings.Join(env, " ") != "MYSQL_PWD=pw" {
		t.Fatalf("client %q %v %v", user, argv, env)
	}
	if string(stdin) != "CREATE DEFINER=CURRENT_USER PROCEDURE p() SELECT 1;\n\\! touch /tmp/marker\n" {
		t.Fatalf("stdin %q", stdin)
	}
}

// A restore without the database's own account is refused before the
// safety backup and before anything is written.
func TestARestoreWithoutTheDatabasesAccountChangesNothing(t *testing.T) {
	installBackupTestPaths(t)
	state := map[int]string{6: "backup"}
	installDatabaseState(t, state, nil, nil)
	agent := &Agent{}
	database := backupspec.DatabaseIdentity{ID: 6, Name: "shop", Type: "mysql"}
	create := testCreateRequest(backupspec.TypeDatabase)
	create.Database = database
	info, err := agent.createBackup(create)
	if err != nil {
		t.Fatal(err)
	}
	// The real account check, not the test's.
	prepareDatabaseLoadFor = prepareDatabaseLoad
	state[6] = "live"
	before := s1BackupCount(t, agent)
	restore := &backupspec.RestoreRequest{ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: 7, DomainID: 9,
		DomainName: "example.test", BackupName: info.Name, Database: database}
	err = agent.restoreBackup(restore, &backupspec.RestoreResponse{})
	if code, _ := restoreFailureOf(err); code != backupspec.RestoreCodeDatabaseAccount {
		t.Fatalf("restore = %v (code %q), want the database account refusal", err, code)
	}
	after := s1BackupCount(t, agent)
	if state[6] != "live" || after != before {
		t.Fatalf("the refused restore changed something: state %q, backups %d -> %d", state[6], before, after)
	}
}

// A database copy without the new database's account copies nothing.
func TestADatabaseCopyWithoutTheCopysAccountLoadsNothing(t *testing.T) {
	installBackupTestPaths(t)
	state := map[int]string{8: "source"}
	installDatabaseState(t, state, nil, nil)
	prepareDatabaseLoadFor = prepareDatabaseLoad
	var resp transport.DBToolsCopyResponse
	err := (&Agent{}).DatabaseCopyV1(transport.DBToolsCopyRequest{
		Source: transport.DBToolsTarget{SubscriptionID: 7, DomainID: 9, Domain: "example.test", Database: backupspec.DatabaseIdentity{ID: 8, Name: "shop", Type: "mysql"}},
		Target: backupspec.DatabaseIdentity{ID: 10, Name: "shop_copy", Type: "mysql"},
	}, &resp)
	if err != nil || resp.Success || !strings.Contains(resp.Error, "no user with a stored password") {
		t.Fatalf("copy = %v %+v", err, resp)
	}
	if _, loaded := state[10]; loaded {
		t.Fatal("the copy was loaded")
	}
}

// Finding 8: the database tools' query inside a customer's database starts
// with the fixed search path (a sudo standing in for the real one records
// the command line).
func TestTheDatabaseToolsQueryInsideADatabaseHasTheFixedSearchPath(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\necho \"$@\" > "+record+"\necho 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	out, err := dbToolsQuery(backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql", ClusterPort: 5433}, true, "SELECT count(*) FROM pg_catalog.pg_tables;")
	if err != nil || out != "3" {
		t.Fatalf("query = %q %v", out, err)
	}
	args, _ := os.ReadFile(record)
	if got := strings.TrimSpace(string(args)); got != "-u postgres env "+pgAdminSessionEnv+" psql --no-psqlrc --set ON_ERROR_STOP=on --tuples-only --no-align --port 5433 --dbname shop" {
		t.Fatalf("command line %q", got)
	}
}

func s1BackupCount(t *testing.T, agent *Agent) int {
	t.Helper()
	var list backupspec.ListResponse
	if err := agent.ListBackups(&backupspec.ListRequest{ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: 7, DomainID: 9, DomainName: "example.test"}, &list); err != nil {
		t.Fatal(err)
	}
	return len(list.Backups)
}
