package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type cpmoveRecordedCommand struct {
	executable string
	args       []string
	stdin      []byte
	hasStdin   bool
	hasTimeout bool
	tempPath   string
	fileMode   os.FileMode
	dirMode    os.FileMode
	pathExists bool
}

type cpmoveRecordingRunner struct {
	calls      []cpmoveRecordedCommand
	failCall   int
	failErr    error
	failures   map[int]error
	inspectErr error
}

func (r *cpmoveRecordingRunner) Run(ctx context.Context, executable string, args []string, stdin io.Reader) error {
	_, hasTimeout := ctx.Deadline()
	call := cpmoveRecordedCommand{
		executable: executable,
		args:       append([]string(nil), args...),
		hasTimeout: hasTimeout,
	}
	if stdin != nil {
		call.hasStdin = true
		file, ok := stdin.(*os.File)
		if !ok {
			r.inspectErr = fmt.Errorf("stdin type = %T, want *os.File", stdin)
		} else {
			call.tempPath = file.Name()
			if info, err := file.Stat(); err != nil {
				r.inspectErr = fmt.Errorf("stat stdin file: %w", err)
			} else {
				call.fileMode = info.Mode().Perm()
			}
			if info, err := os.Stat(filepath.Dir(file.Name())); err != nil {
				r.inspectErr = fmt.Errorf("stat stdin directory: %w", err)
			} else {
				call.dirMode = info.Mode().Perm()
			}
			if _, err := os.Stat(file.Name()); err == nil {
				call.pathExists = true
			} else if !os.IsNotExist(err) {
				r.inspectErr = fmt.Errorf("stat stdin path: %w", err)
			}
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			r.inspectErr = fmt.Errorf("read stdin: %w", err)
		} else {
			call.stdin = data
		}
	}
	r.calls = append(r.calls, call)
	if err, ok := r.failures[len(r.calls)]; ok {
		return err
	}
	if len(r.calls) == r.failCall {
		return r.failErr
	}
	return nil
}

func writeCpmoveDatabaseArchive(t *testing.T, dumpName, dump string) string {
	t.Helper()

	archiveRoot := t.TempDir()
	archivePath := filepath.Join(archiveRoot, "backup.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	header := &tar.Header{
		Name: "cpmove-user/mysql/" + dumpName + ".sql",
		Mode: 0o600,
		Size: int64(len(dump)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(tarWriter, dump); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(archivePath, 0o600); err != nil {
		t.Fatal(err)
	}
	previousRoot := cpmoveArchiveRoot
	cpmoveArchiveRoot = archiveRoot
	t.Cleanup(func() { cpmoveArchiveRoot = previousRoot })
	if runtime.GOOS != "windows" {
		previousOwner := cpmoveArchiveOwnerUID
		cpmoveArchiveOwnerUID = uint32(os.Geteuid())
		t.Cleanup(func() { cpmoveArchiveOwnerUID = previousOwner })
	}
	return archivePath
}

// cpmoveTestLoad stands in for the load (S1): a sandboxed client and the
// dump it was given.
type cpmoveTestLoad struct {
	calls    int
	req      CpmoveImportDBRequest
	dump     []byte
	tempPath string
	fileMode os.FileMode
	dirMode  os.FileMode
	visible  bool
	timedOut bool
	err      error
}

func installCpmoveTestLoad(t *testing.T, load *cpmoveTestLoad) {
	t.Helper()
	oldLoad, oldClient := cpmoveLoadDump, mariadbClient
	mariadbClient = func() (string, bool) { return "/usr/bin/mariadb", true }
	cpmoveLoadDump = func(req *CpmoveImportDBRequest, dump io.Reader) (bool, error) {
		load.calls++
		load.req = *req
		if file, ok := dump.(*os.File); ok {
			load.tempPath = file.Name()
			if info, err := file.Stat(); err == nil {
				load.fileMode = info.Mode().Perm()
			}
			if info, err := os.Stat(filepath.Dir(file.Name())); err == nil {
				load.dirMode = info.Mode().Perm()
			}
			_, err := os.Stat(file.Name())
			load.visible = err == nil
		}
		load.dump, _ = io.ReadAll(dump)
		return load.timedOut, load.err
	}
	t.Cleanup(func() { cpmoveLoadDump, mariadbClient = oldLoad, oldClient })
}

// S1 (20 Nov 2026): the archive's dump is never fed to the administrator's
// client. The administrator only creates the database (and drops it after
// a failed load); the dump goes to the sandboxed load.
func TestImportCpmoveDatabaseLoadsTheDumpOnlyThroughTheSandboxedLoad(t *testing.T) {
	const dump = "INSERT INTO secrets VALUES ('opaque $(touch /tmp/pwn); `whoami`');\n"
	archivePath := writeCpmoveDatabaseArchive(t, "source_db", dump)
	runner := &cpmoveRecordingRunner{}
	load := &cpmoveTestLoad{}
	installCpmoveTestLoad(t, load)
	resp := &CpmoveImportDBResponse{}

	if err := importCpmoveDatabase(&CpmoveImportDBRequest{
		Path:           archivePath,
		DumpName:       "source_db",
		TargetDB:       "target_db",
		SubscriptionID: 3, DomainID: 4, Domain: "imported.test",
	}, resp, runner); err != nil {
		t.Fatal(err)
	}
	if !resp.Imported || resp.Error != "" {
		t.Fatalf("response = %+v, want successful import", resp)
	}
	if runner.inspectErr != nil {
		t.Fatal(runner.inspectErr)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("administrator commands = %d, want only the CREATE DATABASE", len(runner.calls))
	}
	call := runner.calls[0]
	if call.executable != "mysql" || !call.hasTimeout || call.hasStdin ||
		!reflect.DeepEqual(call.args, []string{"--execute", "CREATE DATABASE `target_db`;"}) {
		t.Fatalf("administrator command = %+v, want only a deadline-bound CREATE DATABASE", call)
	}
	if load.calls != 1 || string(load.dump) != dump {
		t.Fatalf("load got %d call(s) with %q, want the exact dump once", load.calls, load.dump)
	}
	if load.req.TargetDB != "target_db" || load.req.SubscriptionID != 3 || load.req.DomainID != 4 || load.req.Domain != "imported.test" {
		t.Fatalf("load request = %+v", load.req)
	}
	if load.fileMode != 0o600 || load.dirMode != 0o700 {
		t.Fatalf("dump file mode = %#o, directory %#o; want 0600 and 0700", load.fileMode, load.dirMode)
	}
	if runtime.GOOS != "windows" && load.visible {
		t.Fatal("temporary dump pathname remained visible while the client was running")
	}
	if _, err := os.Stat(load.tempPath); !os.IsNotExist(err) {
		t.Fatalf("temporary dump still exists after import: %v", err)
	}
}

// The load itself: a temporary account with rights on the new database
// only, the sandboxed client signed in as it, under the site's account, and
// the account removed again.
func TestCpmoveLoadDumpUsesATemporaryDatabaseScopedAccount(t *testing.T) {
	oldAccount, oldRun, oldClient, oldSite := cpmoveCreateLoadAccount, runDatabaseClientAsSiteUser, mariadbClient, databaseLoadSiteUser
	t.Cleanup(func() {
		cpmoveCreateLoadAccount, runDatabaseClientAsSiteUser, mariadbClient, databaseLoadSiteUser = oldAccount, oldRun, oldClient, oldSite
	})
	mariadbClient = func() (string, bool) { return "/usr/bin/mariadb", true }
	databaseLoadSiteUser = func(subscriptionID, domainID int, domain string) string {
		if subscriptionID == 3 && domainID == 4 && domain == "imported.test" {
			return "imported_test"
		}
		return "nobody"
	}
	dropped := 0
	cpmoveCreateLoadAccount = func(database string) (databaseLoadAccount, func(), error) {
		if database != "target_db" {
			t.Fatalf("account for %q", database)
		}
		return databaseLoadAccount{User: "celikpanel_load_0123456789ab", Password: "secret-pw"}, func() { dropped++ }, nil
	}
	var gotUser string
	var gotArgv, gotEnv []string
	var gotStdin []byte
	runDatabaseClientAsSiteUser = func(ctx context.Context, username string, argv []string, env []string, stdin io.Reader) (string, error) {
		gotUser, gotArgv, gotEnv = username, argv, env
		gotStdin, _ = io.ReadAll(stdin)
		return "ERROR 1227 (42000) at line 2: Access denied; you need the SUPER privilege near 'secret text'", errors.New("exit status 1")
	}
	timedOut, err := cpmoveLoadDump(&CpmoveImportDBRequest{TargetDB: "target_db", SubscriptionID: 3, DomainID: 4, Domain: "imported.test"},
		strings.NewReader("CREATE DEFINER=`cpuser_x`@`localhost` PROCEDURE p() SELECT 1;\n"))
	if timedOut || err == nil {
		t.Fatalf("load = %v %v, want a refused load", timedOut, err)
	}
	if text := cpmoveDatabaseCommandError("import", false, err); text != "mysql import failed (ERROR 1227 at line 2)" || strings.Contains(text, "secret") {
		t.Fatalf("error = %q", text)
	}
	want := []string{"/usr/bin/mariadb", "--no-defaults", "--sandbox", "--batch", "--protocol=socket",
		"--user=celikpanel_load_0123456789ab", "--database=target_db", "--default-character-set=utf8mb4", "--max-allowed-packet=1073741824"}
	if gotUser != "imported_test" || !reflect.DeepEqual(gotArgv, want) || !reflect.DeepEqual(gotEnv, []string{"MYSQL_PWD=secret-pw"}) {
		t.Fatalf("client = %q %v %v", gotUser, gotArgv, gotEnv)
	}
	if string(gotStdin) != "CREATE DEFINER=CURRENT_USER PROCEDURE p() SELECT 1;\n" {
		t.Fatalf("stdin = %q, want the definer written as CURRENT_USER", gotStdin)
	}
	if dropped != 1 {
		t.Fatalf("temporary account dropped %d times, want 1", dropped)
	}
}

func TestImportCpmoveDatabaseRejectsMetacharactersBeforeExecution(t *testing.T) {
	tests := []struct {
		name     string
		dumpName string
		targetDB string
		wantErr  string
	}{
		{name: "target database", dumpName: "source_db", targetDB: "target;touch_pwn", wantErr: "invalid target database name"},
		{name: "dump name", dumpName: "source;touch_pwn", targetDB: "target_db", wantErr: "invalid dump name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &cpmoveRecordingRunner{}
			resp := &CpmoveImportDBResponse{}
			err := importCpmoveDatabase(&CpmoveImportDBRequest{
				Path:     filepath.Join(t.TempDir(), "does-not-matter.tar.gz"),
				DumpName: test.dumpName,
				TargetDB: test.targetDB,
			}, resp, runner)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Error != test.wantErr {
				t.Fatalf("error = %q, want %q", resp.Error, test.wantErr)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("executed %d command(s) for rejected input", len(runner.calls))
			}
		})
	}
}

func TestImportCpmoveDatabaseDoesNotLeakDumpSecretsInErrors(t *testing.T) {
	const secret = "dump-password-NEVER-RETURN"
	archivePath := writeCpmoveDatabaseArchive(t, "source_db", "SELECT '"+secret+"';\n")
	tests := []struct {
		name     string
		failErr  error
		timedOut bool
		wantErr  string
	}{
		{name: "client failure", failErr: errors.New("mysql echoed " + secret), wantErr: "mysql import failed"},
		{name: "timeout", failErr: context.DeadlineExceeded, timedOut: true, wantErr: "mysql import timed out"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &cpmoveRecordingRunner{}
			installCpmoveTestLoad(t, &cpmoveTestLoad{err: test.failErr, timedOut: test.timedOut})
			resp := &CpmoveImportDBResponse{}
			err := importCpmoveDatabase(&CpmoveImportDBRequest{
				Path:     archivePath,
				DumpName: "source_db",
				TargetDB: "target_db",
			}, resp, runner)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Imported {
				t.Fatal("failed import reported success")
			}
			if resp.Error != test.wantErr {
				t.Fatalf("error = %q, want %q", resp.Error, test.wantErr)
			}
			if strings.Contains(resp.Error, secret) {
				t.Fatalf("dump secret leaked in response: %q", resp.Error)
			}
			if len(runner.calls) != 2 {
				t.Fatalf("create and cleanup commands were not executed: %+v", runner.calls)
			}
			cleanup := runner.calls[1]
			wantCleanup := []string{"--execute", "DROP DATABASE IF EXISTS `target_db`;"}
			if cleanup.executable != "mysql" || !cleanup.hasTimeout ||
				cleanup.hasStdin || !reflect.DeepEqual(cleanup.args, wantCleanup) {
				t.Fatalf("cleanup command = %+v, want deadline-bound direct DROP", cleanup)
			}
		})
	}
}

func TestImportCpmoveDatabaseDoesNotDropWhenExclusiveCreateFails(t *testing.T) {
	archivePath := writeCpmoveDatabaseArchive(t, "source_db", "SELECT 1;\n")
	runner := &cpmoveRecordingRunner{failCall: 1, failErr: errors.New("database exists")}
	load := &cpmoveTestLoad{}
	installCpmoveTestLoad(t, load)
	resp := &CpmoveImportDBResponse{}

	if err := importCpmoveDatabase(&CpmoveImportDBRequest{
		Path:     archivePath,
		DumpName: "source_db",
		TargetDB: "target_db",
	}, resp, runner); err != nil {
		t.Fatal(err)
	}
	if resp.Imported || resp.Error != "mysql create failed" {
		t.Fatalf("response = %+v, want exclusive create failure", resp)
	}
	if len(runner.calls) != 1 || load.calls != 0 {
		t.Fatalf("commands = %d, loads = %d; want no load and no DROP after a failed CREATE", len(runner.calls), load.calls)
	}
}

func TestImportCpmoveDatabaseReportsFailedCompensation(t *testing.T) {
	archivePath := writeCpmoveDatabaseArchive(t, "source_db", "SELECT 1;\n")
	runner := &cpmoveRecordingRunner{failures: map[int]error{
		2: errors.New("drop failed"),
	}}
	installCpmoveTestLoad(t, &cpmoveTestLoad{err: errors.New("import failed")})
	resp := &CpmoveImportDBResponse{}

	if err := importCpmoveDatabase(&CpmoveImportDBRequest{
		Path:     archivePath,
		DumpName: "source_db",
		TargetDB: "target_db",
	}, resp, runner); err != nil {
		t.Fatal(err)
	}
	if resp.Imported {
		t.Fatal("failed import and cleanup reported success")
	}
	want := "mysql import failed; database cleanup failed; manual reconciliation is required"
	if resp.Error != want {
		t.Fatalf("error = %q, want %q", resp.Error, want)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("command count = %d, want CREATE and DROP", len(runner.calls))
	}
}

// S1: a server whose MariaDB client has no sandbox mode creates nothing.
func TestImportCpmoveDatabaseCreatesNothingWithoutASandboxedClient(t *testing.T) {
	archivePath := writeCpmoveDatabaseArchive(t, "source_db", "SELECT 1;\n")
	runner := &cpmoveRecordingRunner{}
	load := &cpmoveTestLoad{}
	installCpmoveTestLoad(t, load)
	mariadbClient = func() (string, bool) { return "/usr/bin/mysql", false }
	resp := &CpmoveImportDBResponse{}
	if err := importCpmoveDatabase(&CpmoveImportDBRequest{Path: archivePath, DumpName: "source_db", TargetDB: "target_db"}, resp, runner); err != nil {
		t.Fatal(err)
	}
	if resp.Imported || !strings.Contains(resp.Error, "sandbox mode") || len(runner.calls) != 0 || load.calls != 0 {
		t.Fatalf("response = %+v, commands %d, loads %d; want a refusal before anything", resp, len(runner.calls), load.calls)
	}
}
