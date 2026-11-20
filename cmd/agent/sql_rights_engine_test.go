//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Security lane S1 (20 November 2026) on real engines: the attacks the
// tenant-isolation audit names (findings 1, 8 and 9), run through the Agent's
// own code against the private MariaDB and PostgreSQL of the test
// environment (CELIKPANEL_TEST_ENGINES; skipped without them). Every payload
// only writes a marker file or records a name. These tests fail on the code
// before the lane (the marker appears, the engine's administrator is
// recorded) and pass after it.
//
// The Agent's own clients are pointed at the private engines through small
// wrappers placed first on PATH (sudo, psql, pg_dump, mariadb, mysql,
// mysqldump): the command lines the Agent builds are run as they are, only
// the socket, host and port are added.

// s1Engines is the private engines with the Agent's clients pointed at them.
type s1Engines struct {
	testEngines
	marker string
}

func s1InstallEngineClients(t *testing.T) s1Engines {
	t.Helper()
	e := requireTestEngines(t)
	if e.pgPort == "" {
		t.Skip("CELIKPANEL_TEST_PG_PORT is not set")
	}
	if os.Geteuid() != 0 {
		t.Skip("the engine tests run the clients as other accounts: root only")
	}
	dir := t.TempDir()
	// The MariaDB client runs as another account after the lane: it must
	// reach the wrappers.
	for path := dir; path != "/" && path != os.TempDir(); path = filepath.Dir(path) {
		_ = os.Chmod(path, 0o755)
	}
	root := filepath.Join(e.dir, "root")
	lib := filepath.Join(root, "usr/lib/x86_64-linux-gnu")
	run := filepath.Join(e.dir, "run")
	pgBin := filepath.Join(root, "usr/lib/postgresql/17/bin")
	wrappers := map[string]string{
		// sudo -u postgres <command...>: the command, as the test's account.
		"sudo":    "#!/bin/sh\n[ \"$1\" = \"-u\" ] && shift 2\nexec \"$@\"\n",
		"psql":    "#!/bin/sh\nexport LD_LIBRARY_PATH=" + lib + "\nexec " + pgBin + "/psql -h " + run + " -p " + e.pgPort + " -U postgres \"$@\"\n",
		"pg_dump": "#!/bin/sh\nexport LD_LIBRARY_PATH=" + lib + "\nexec " + pgBin + "/pg_dump -h " + run + " -p " + e.pgPort + " -U postgres \"$@\"\n",
	}
	maria := func(program string) string {
		return "#!/bin/sh\nexport LD_LIBRARY_PATH=" + lib + "\n[ \"$1\" = \"--no-defaults\" ] && shift\nexec " +
			filepath.Join(root, "usr/bin", program) + " --no-defaults --socket=" + filepath.Join(run, "maria.sock") + " \"$@\"\n"
	}
	wrappers["mariadb"] = maria("mariadb")
	wrappers["mysql"] = maria("mariadb")
	wrappers["mysqldump"] = maria("mariadb-dump")
	wrappers["mariadb-dump"] = maria("mariadb-dump")
	for name, body := range wrappers {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	oldClusters := pgStatementClusters
	pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) { return nil, false }
	t.Cleanup(func() { pgStatementClusters = oldClusters })
	markers := t.TempDir()
	for path := markers; path != "/" && path != os.TempDir(); path = filepath.Dir(path) {
		_ = os.Chmod(path, 0o777)
	}
	return s1Engines{testEngines: e, marker: filepath.Join(markers, "client-ran-a-shell-command")}
}

func (e s1Engines) noMarker(t *testing.T, what string) {
	t.Helper()
	if _, err := os.Stat(e.marker); err == nil {
		_ = os.Remove(e.marker)
		t.Errorf("%s: the file's shell command ran (marker %s was written)", what, e.marker)
	}
}

// s1RestorePackage makes a database backup of this site whose payload is
// the given SQL (what a customer puts into a package at their own remote
// destination) and restores it through the Agent, as the remote restore
// does. The safety backup taken by the restore is a real dump.
func s1RestorePackage(t *testing.T, database backupspec.DatabaseIdentity, account databaseTestAccount, payload string) backupspec.RestoreResponse {
	t.Helper()
	original := dumpDatabaseToFile
	made := false
	dumpDatabaseToFile = func(identity backupspec.DatabaseIdentity, destination string) error {
		if !made {
			made = true
			return writeGzipText(destination, payload)
		}
		return original(identity, destination)
	}
	t.Cleanup(func() { dumpDatabaseToFile = original })
	agent := &Agent{}
	create := testCreateRequest(backupspec.TypeDatabase)
	create.Database = database
	info, err := agent.createBackup(create)
	if err != nil {
		t.Fatal(err)
	}
	restore := &backupspec.RestoreRequest{
		ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: 7, DomainID: 9, DomainName: "example.test",
		BackupName: info.Name, Database: database,
	}
	s1WithCredentials(restore, database.ID, account.user, account.password)
	var resp backupspec.RestoreResponse
	if err := agent.restoreBackup(restore, &resp); err != nil {
		code, detail := restoreFailureOf(err)
		resp.Code, resp.Detail, resp.Error = code, detail, err.Error()
	}
	return resp
}

type databaseTestAccount struct{ user, password string }

// A (finding 1), MariaDB: a package whose payload holds a client shell
// command and a statement only the engine's administrator may run.
func TestS1MariaDBRestoreOfAnEditedPackageStaysInsideTheDatabase(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS s1a; DROP USER IF EXISTS 's1a_user'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE s1a; CREATE USER 's1a_user'@'localhost' IDENTIFIED BY '"+engineOwnerPassword+"';"+
		" GRANT ALL PRIVILEGES ON `s1a`.* TO 's1a_user'@'localhost';"))
	mustRun(t, "owner table", e.maria("s1a_user", engineOwnerPassword, "USE s1a; CREATE TABLE seen (who VARCHAR(200)); CREATE TABLE t (id INT);"))
	database := backupspec.DatabaseIdentity{ID: 41, Name: "s1a", Type: "mysql"}
	account := databaseTestAccount{"s1a_user", engineOwnerPassword}

	// 1. The edited package: a right on every database, then a shell command.
	payload := "GRANT ALL PRIVILEGES ON *.* TO 's1a_user'@'localhost';\n\\! touch " + e.marker + "\nsystem touch " + e.marker + "\n"
	resp := s1RestorePackage(t, database, account, payload)
	e.noMarker(t, "restore of an edited package")
	if grants := mustRun(t, "grants", e.maria("", "", "SHOW GRANTS FOR 's1a_user'@'localhost';")); strings.Contains(grants, "ON *.*") && strings.Contains(grants, "ALL PRIVILEGES ON *.*") {
		t.Errorf("the package gave its database user every right on the server: %s", grants)
	}
	if resp.Success {
		t.Errorf("a package with a client command was restored: %+v", resp)
	}

	// 2. Whose rights the payload has: it records its own account.
	resp = s1RestorePackage(t, database, account, "CREATE TABLE IF NOT EXISTS seen (who VARCHAR(200));\nINSERT INTO seen VALUES (CURRENT_USER());\n")
	if !resp.Success {
		t.Fatalf("an ordinary payload was not restored: %+v", resp)
	}
	if who := mustRun(t, "seen", e.maria("", "", "SELECT who FROM s1a.seen;")); who != "s1a_user@localhost" {
		t.Errorf("the payload ran as %q, want the database's own user", who)
	}
}

// A (finding 1), PostgreSQL: a client shell command; RESET ROLE before
// recording the session's role; RESET ROLE before making the user a
// superuser.
func TestS1PostgreSQLRestoreOfAnEditedPackageStaysInsideTheDatabase(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS s1p; DROP ROLE IF EXISTS s1p_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "role", e.pg("", "", "", "CREATE ROLE s1p_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE s1p OWNER s1p_user;"))
	mustRun(t, "public closed", e.pg("", "", "", "REVOKE ALL ON DATABASE s1p FROM PUBLIC;"))
	mustRun(t, "owner table", e.pg("s1p_user", engineOwnerPassword, "s1p", "CREATE TABLE public.items (id int); INSERT INTO public.items VALUES (1);"))
	port := 0
	for _, c := range e.pgPort {
		port = port*10 + int(c-'0')
	}
	database := backupspec.DatabaseIdentity{ID: 42, Name: "s1p", Type: "postgresql", ClusterPort: port}
	account := databaseTestAccount{"s1p_user", engineOwnerPassword}

	resp := s1RestorePackage(t, database, account, "CREATE TABLE public.items (id int);\n\\! touch "+e.marker+"\n")
	e.noMarker(t, "restore of a package with a psql command")
	if resp.Success {
		t.Errorf("a package with a psql command was restored: %+v", resp)
	}

	resp = s1RestorePackage(t, database, account, "CREATE TABLE public.items (id int);\nRESET ROLE;\nALTER ROLE s1p_user SUPERUSER;\n")
	if super := mustRun(t, "rolsuper", e.pg("", "", "", "SELECT rolsuper FROM pg_roles WHERE rolname = 's1p_user';")); super != "f" {
		_ = e.pg("", "", "", "ALTER ROLE s1p_user NOSUPERUSER;")
		t.Errorf("the package made the database's user a superuser (%+v)", resp)
	}

	resp = s1RestorePackage(t, database, account, "CREATE TABLE public.seen (who text);\nRESET ROLE;\nINSERT INTO public.seen SELECT current_user;\n")
	if !resp.Success {
		t.Fatalf("an ordinary payload was not restored: %+v", resp)
	}
	if who := mustRun(t, "seen", e.pg("", "", "s1p", "SELECT who FROM public.seen;")); who != "s1p_user" {
		t.Errorf("the payload ran as %q, want the database's own role", who)
	}
}

// B (finding 9): a cPanel archive's dump with a client shell command and a
// statement only the administrator may run.
func TestS1CPanelImportDumpStaysInsideTheNewDatabase(t *testing.T) {
	e := s1InstallEngineClients(t)
	cleanup := "DROP DATABASE IF EXISTS s1cp; DROP USER IF EXISTS 's1cp_victim'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	dump := "CREATE TABLE t (id INT);\nCREATE USER 's1cp_victim'@'localhost';\n\\! touch " + e.marker + "\n"
	archive := writeCpmoveDatabaseArchive(t, "s1cp", dump)
	resp := &CpmoveImportDBResponse{}
	if err := importCpmoveDatabase(&CpmoveImportDBRequest{Path: archive, DumpName: "s1cp", TargetDB: "s1cp"}, resp, cpmoveExecDatabaseCommandRunner{}); err != nil {
		t.Fatal(err)
	}
	e.noMarker(t, "cPanel import")
	if users := mustRun(t, "users", e.maria("", "", "SELECT COUNT(*) FROM mysql.user WHERE User = 's1cp_victim';")); users != "0" {
		t.Errorf("the archive's dump created an account on the server")
	}
	if left := mustRun(t, "load accounts", e.maria("", "", "SELECT COUNT(*) FROM mysql.user WHERE User LIKE 'celikpanel\\_load\\_%';")); left != "0" {
		t.Errorf("the temporary load account was left behind (%s)", left)
	}
	if resp.Imported {
		t.Errorf("a dump with a client command was imported: %+v", resp)
	}

	// An ordinary dump with a view and a trigger of a cPanel account loads.
	_ = e.maria("", "", cleanup)
	dump = "CREATE TABLE t (id INT, note VARCHAR(20));\nINSERT INTO t VALUES (1,'a');\n" +
		"/*!50001 CREATE ALGORITHM=UNDEFINED */\n/*!50013 DEFINER=`cpuser_wp`@`localhost` SQL SECURITY DEFINER */\n/*!50001 VIEW `v` AS select `t`.`id` AS `id` from `t` */;\n" +
		"/*!50003 CREATE*/ /*!50017 DEFINER=`cpuser_wp`@`localhost`*/ /*!50003 TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.note = 'x' */;\n"
	archive = writeCpmoveDatabaseArchive(t, "s1cp", dump)
	resp = &CpmoveImportDBResponse{}
	if err := importCpmoveDatabase(&CpmoveImportDBRequest{Path: archive, DumpName: "s1cp", TargetDB: "s1cp"}, resp, cpmoveExecDatabaseCommandRunner{}); err != nil {
		t.Fatal(err)
	}
	if !resp.Imported {
		t.Fatalf("an ordinary cPanel dump was not imported: %+v", resp)
	}
	// The objects are there (their definer, a cPanel account, is not
	// migrated, as before: docs/DATABASE-USERS.md).
	if objects := mustRun(t, "objects", e.maria("", "", "SELECT (SELECT COUNT(*) FROM information_schema.views WHERE table_schema = 's1cp') + (SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema = 's1cp') + (SELECT COUNT(*) FROM s1cp.t);")); objects != "3" {
		t.Errorf("the imported view, trigger and row count %q, want 3", objects)
	}
}

// C (finding 8): the database's owner plants an operator its database's
// search_path finds before pg_catalog's; the Agent's administrator sessions
// in that database (database tools, adding a user) must not run it.
func TestS1PostgreSQLAdministratorSessionsDoNotRunTheOwnersFunctions(t *testing.T) {
	e := s1InstallEngineClients(t)
	cleanup := "DROP DATABASE IF EXISTS s1c; DROP ROLE IF EXISTS s1c_reader; DROP ROLE IF EXISTS s1c_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "role", e.pg("", "", "", "CREATE ROLE s1c_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE s1c OWNER s1c_user;"))
	mustRun(t, "plant", e.pg("s1c_user", engineOwnerPassword, "s1c", strings.Join([]string{
		"CREATE TABLE public.seen (who text);",
		"GRANT INSERT ON public.seen TO PUBLIC;",
		"CREATE TABLE public.items (id int);",
		"CREATE FUNCTION public.trap_ne(name, name) RETURNS boolean LANGUAGE plpgsql AS $f$ BEGIN INSERT INTO public.seen VALUES (current_user); RETURN $1::text OPERATOR(pg_catalog.<>) $2::text; END $f$;",
		"CREATE OPERATOR public.<> (LEFTARG = name, RIGHTARG = name, FUNCTION = public.trap_ne);",
		"CREATE FUNCTION public.trap_nlike(name, text) RETURNS boolean LANGUAGE plpgsql AS $f$ BEGIN INSERT INTO public.seen VALUES (current_user); RETURN $1::text OPERATOR(pg_catalog.!~~) $2; END $f$;",
		"CREATE OPERATOR public.!~~ (LEFTARG = name, RIGHTARG = text, FUNCTION = public.trap_nlike);",
		"ALTER DATABASE s1c SET search_path = public, pg_catalog;",
	}, "\n")))
	database := backupspec.DatabaseIdentity{ID: 43, Name: "s1c", Type: "postgresql"}
	target := transport.DBToolsTarget{SubscriptionID: 7, DomainID: 9, Domain: "example.test", Database: database}
	var info transport.DBToolsInfoResponse
	if err := (&Agent{}).DatabaseToolsInfoV1(target, &info); err != nil || info.Error != "" {
		t.Fatalf("database tools: %v %+v", err, info)
	}
	var check transport.DBToolsCheckResponse
	if err := (&Agent{}).DatabaseCheckV1(transport.DBToolsCheckRequest{Target: target}, &check); err != nil || check.Error != "" {
		t.Fatalf("database check: %v %+v", err, check)
	}
	read := transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "s1c",
		Username: "s1c_reader", Password: engineReadPassword, Access: transport.DatabaseAccessRead}
	if resp := accessUser(t, true, read); !resp.Applied {
		t.Fatalf("adding a user: %+v", resp)
	}
	if who := mustRun(t, "seen", e.pg("", "", "s1c", "SELECT coalesce(string_agg(DISTINCT who, ','), '') FROM public.seen;")); who != "" {
		t.Errorf("the owner's planted operators ran in the Agent's sessions as: %s", who)
	}
	if info.Tables != 2 {
		t.Errorf("tables = %d, want 2", info.Tables)
	}
}
