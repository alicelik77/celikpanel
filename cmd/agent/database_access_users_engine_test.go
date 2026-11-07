package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Real engines (alpha.129): a private MariaDB and PostgreSQL that the test
// environment started under one directory, never the system's services.
// CELIKPANEL_TEST_ENGINES names it: root/ holds the unpacked official
// packages, run/ the sockets (MariaDB maria.sock, PostgreSQL on port
// CELIKPANEL_TEST_PG_PORT). Without it the tests are skipped.
//
// Gerçek motorlar: test ortamının tek bir dizinde başlattığı özel MariaDB ve
// PostgreSQL; sistemin hizmetleri değil.

type testEngines struct {
	dir    string
	pgPort string
}

func requireTestEngines(t *testing.T) testEngines {
	t.Helper()
	dir := os.Getenv("CELIKPANEL_TEST_ENGINES")
	if dir == "" {
		t.Skip("CELIKPANEL_TEST_ENGINES is not set: no private database engines")
	}
	return testEngines{dir: dir, pgPort: os.Getenv("CELIKPANEL_TEST_PG_PORT")}
}

func (e testEngines) env() []string {
	return append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Join(e.dir, "root/usr/lib/x86_64-linux-gnu"))
}

// engineResult is one client run: its output and how it ended.
type engineResult struct {
	out []byte
	err error
}

// maria runs statements as the engine root over the private socket, or as
// user with password when user is not "".
func (e testEngines) maria(user, password, statement string) engineResult {
	args := []string{"--no-defaults", "-S", filepath.Join(e.dir, "run/maria.sock"), "--batch", "--skip-column-names"}
	if user == "" {
		args = append(args, "-uroot")
	} else {
		args = append(args, "-u"+user, "-p"+password)
	}
	cmd := exec.Command(filepath.Join(e.dir, "root/usr/bin/mariadb"), args...)
	cmd.Env = e.env()
	cmd.Stdin = strings.NewReader(statement + "\n")
	out, err := cmd.CombinedOutput()
	return engineResult{out, err}
}

// pg runs statements as postgres over the socket (trust), or as user with
// password over TCP (scram-sha-256), in database.
func (e testEngines) pg(user, password, database, statement string) engineResult {
	host := filepath.Join(e.dir, "run")
	if user == "" {
		user = "postgres"
	} else {
		host = "127.0.0.1"
	}
	args := []string{"-h", host, "-p", e.pgPort, "-U", user, "--no-psqlrc", "--set", "ON_ERROR_STOP=on", "-At"}
	if database != "" {
		args = append(args, "-d", database)
	}
	cmd := exec.Command(filepath.Join(e.dir, "root/usr/lib/postgresql/17/bin/psql"), args...)
	cmd.Env = append(e.env(), "PGPASSWORD="+password)
	cmd.Stdin = strings.NewReader(statement + "\n")
	out, err := cmd.CombinedOutput()
	return engineResult{out, err}
}

// installTestEngines points the Agent's statement runners at the private
// engines for one test.
func installTestEngines(t *testing.T, e testEngines) {
	t.Helper()
	oldMySQL, oldPG, oldClusters := mysqlExecStatement, postgreSQLExecStatementOnPort, pgStatementClusters
	mysqlExecStatement = func(statement string) ([]byte, error) { r := e.maria("", "", statement); return r.out, r.err }
	postgreSQLExecStatementOnPort = func(_ int, statement string) ([]byte, error) { r := e.pg("", "", "", statement); return r.out, r.err }
	pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) { return nil, false }
	t.Cleanup(func() {
		mysqlExecStatement, postgreSQLExecStatementOnPort, pgStatementClusters = oldMySQL, oldPG, oldClusters
	})
}

func mustRun(t *testing.T, what string, r engineResult) string {
	t.Helper()
	if r.err != nil {
		t.Fatalf("%s: %v: %s", what, r.err, r.out)
	}
	return strings.TrimSpace(string(r.out))
}

func mustFail(t *testing.T, what string, r engineResult) {
	t.Helper()
	if r.err == nil {
		t.Fatalf("%s succeeded but must be refused: %s", what, r.out)
	}
}

func accessUser(t *testing.T, create bool, req transport.DatabaseAccessUserRequest) transport.DatabaseAccessUserResponse {
	t.Helper()
	var resp transport.DatabaseAccessUserResponse
	var err error
	if create {
		err = (&Agent{}).CreateDatabaseAccessUserV1(&req, &resp)
	} else {
		err = (&Agent{}).RemoveDatabaseAccessUserV1(&req, &resp)
	}
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

const (
	engineOwnerPassword = "Owner-Passw0rd!12"
	engineReadPassword  = "Read-Passw0rd!123"
	engineWritePassword = "Write-Passw0rd!12"
)

func TestDatabaseAccessUsersOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	var res engineResult
	cleanup := "DROP DATABASE IF EXISTS a129shop; DROP USER IF EXISTS 'a129shop_user'@'localhost', 'a129shop_user'@'127.0.0.1', 'a129shop_reader'@'localhost', 'a129shop_editor'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	// The database and the user made with it, as CelikPanel makes them.
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE a129shop; CREATE USER 'a129shop_user'@'localhost' IDENTIFIED BY '"+engineOwnerPassword+"';"+
		" GRANT ALL PRIVILEGES ON a129shop.* TO 'a129shop_user'@'localhost';"))
	mustRun(t, "owner table", e.maria("a129shop_user", engineOwnerPassword, "CREATE TABLE a129shop.items (id INT PRIMARY KEY, name TEXT); INSERT INTO a129shop.items VALUES (1, 'one');"))

	base := transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "a129shop"}
	read := base
	read.Username, read.Password, read.Access = "a129shop_reader", engineReadPassword, transport.DatabaseAccessRead
	if resp := accessUser(t, true, read); !resp.Applied {
		t.Fatalf("read user: %+v", resp)
	}
	if got := mustRun(t, "reader selects", e.maria("a129shop_reader", engineReadPassword, "SELECT name FROM a129shop.items;")); got != "one" {
		t.Fatalf("reader read %q", got)
	}
	res = e.maria("a129shop_reader", engineReadPassword, "INSERT INTO a129shop.items VALUES (2, 'two');")
	mustFail(t, "reader inserts", res)
	res = e.maria("a129shop_reader", engineReadPassword, "CREATE TABLE a129shop.other (id INT);")
	mustFail(t, "reader creates a table", res)
	// The same name again changes nothing.
	if resp := accessUser(t, true, read); resp.Applied || !resp.Exists {
		t.Fatalf("second create: %+v", resp)
	}

	write := base
	write.Username, write.Password, write.Access = "a129shop_editor", engineWritePassword, transport.DatabaseAccessWrite
	if resp := accessUser(t, true, write); !resp.Applied {
		t.Fatalf("write user: %+v", resp)
	}
	mustRun(t, "editor writes", e.maria("a129shop_editor", engineWritePassword, "INSERT INTO a129shop.items VALUES (2, 'two'); CREATE TABLE a129shop.notes (id INT);"))
	grants := mustRun(t, "editor grants", e.maria("", "", "SHOW GRANTS FOR 'a129shop_editor'@'localhost';"))
	for _, line := range strings.Split(grants, "\n") {
		if strings.Contains(line, "GRANT OPTION") || (strings.Contains(line, "ON *.*") && !strings.HasPrefix(line, "GRANT USAGE ON *.*")) {
			t.Fatalf("editor has more than its database: %s", grants)
		}
	}

	// Removing both users leaves the database and every row.
	for _, user := range []string{"a129shop_reader", "a129shop_editor"} {
		req := base
		req.Username = user
		if resp := accessUser(t, false, req); !resp.Applied || resp.Absent {
			t.Fatalf("remove %s: %+v", user, resp)
		}
	}
	if got := mustRun(t, "users left", e.maria("", "", "SELECT COUNT(*) FROM mysql.user WHERE User IN ('a129shop_reader', 'a129shop_editor');")); got != "0" {
		t.Fatalf("users left: %s", got)
	}
	if got := mustRun(t, "rows kept", e.maria("a129shop_user", engineOwnerPassword, "SELECT COUNT(*) FROM a129shop.items; SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = 'a129shop';")); got != "2\n2" {
		t.Fatalf("database changed by the removal: %q", got)
	}
	// A user that is gone already is not an error.
	req := base
	req.Username = "a129shop_reader"
	if resp := accessUser(t, false, req); !resp.Applied || !resp.Absent {
		t.Fatalf("remove again: %+v", resp)
	}
	// The engine's accounts are never touched.
	req.Username = "root"
	if resp := accessUser(t, false, req); resp.Applied || resp.Error == "" {
		t.Fatalf("root: %+v", resp)
	}

	// A new password reaches every host of the account (an owner added
	// 127.0.0.1 by hand).
	mustRun(t, "second host", e.maria("", "", "CREATE USER 'a129shop_user'@'127.0.0.1' IDENTIFIED BY 'Other-Passw0rd!1';"))
	const newPassword = "N3w-Passw0rd!abc"
	var rotated transport.RotateDatabaseUserPasswordResponse
	if err := (&Agent{}).RotateDatabaseUserPasswordV1(&transport.RotateDatabaseUserPasswordRequest{
		User: transport.SuspensionDatabaseUser{Engine: "mysql", Username: "a129shop_user"}, NewPassword: newPassword,
	}, &rotated); err != nil || !rotated.Applied {
		t.Fatalf("rotate: %v %+v", err, rotated)
	}
	if got := mustRun(t, "hashes", e.maria("", "", "SELECT COUNT(DISTINCT authentication_string), MAX(authentication_string = PASSWORD('"+newPassword+"')) FROM mysql.user WHERE User = 'a129shop_user';")); got != "1\t1" {
		t.Fatalf("not every host has the new password: %q", got)
	}
	mustRun(t, "new password signs in", e.maria("a129shop_user", newPassword, "SELECT 1;"))
	res = e.maria("a129shop_user", engineOwnerPassword, "SELECT 1;")
	mustFail(t, "old password", res)
}

func TestDatabaseAccessUsersOnRealPostgreSQL(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	var res engineResult
	cleanup := "DROP DATABASE IF EXISTS a129shop; DROP ROLE IF EXISTS a129shop_reader; DROP ROLE IF EXISTS a129shop_editor; DROP ROLE IF EXISTS a129shop_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	// As CelikPanel makes it: the user, then the database it owns.
	mustRun(t, "setup", e.pg("", "", "", "CREATE USER a129shop_user WITH PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE a129shop OWNER a129shop_user;"))
	mustRun(t, "owner table", e.pg("a129shop_user", engineOwnerPassword, "a129shop", "CREATE TABLE items (id serial PRIMARY KEY, name text); INSERT INTO items (name) VALUES ('one');"))

	base := transport.DatabaseAccessUserRequest{Engine: "postgresql", Port: 5432, Database: "a129shop"}
	read := base
	read.Username, read.Password, read.Access = "a129shop_reader", engineReadPassword, transport.DatabaseAccessRead
	if resp := accessUser(t, true, read); !resp.Applied {
		t.Fatalf("read user: %+v", resp)
	}
	if got := mustRun(t, "reader selects", e.pg("a129shop_reader", engineReadPassword, "a129shop", "SELECT name FROM items;")); got != "one" {
		t.Fatalf("reader read %q", got)
	}
	res = e.pg("a129shop_reader", engineReadPassword, "a129shop", "INSERT INTO items (name) VALUES ('two');")
	mustFail(t, "reader inserts", res)
	res = e.pg("a129shop_reader", engineReadPassword, "a129shop", "CREATE TABLE other (id int);")
	mustFail(t, "reader creates a table", res)
	// A table the owner makes later is readable too (default privileges).
	mustRun(t, "owner later table", e.pg("a129shop_user", engineOwnerPassword, "a129shop", "CREATE TABLE later (v int); INSERT INTO later VALUES (7);"))
	if got := mustRun(t, "reader reads the later table", e.pg("a129shop_reader", engineReadPassword, "a129shop", "SELECT v FROM later;")); got != "7" {
		t.Fatalf("later table read %q", got)
	}
	if got := mustRun(t, "role flags", e.pg("", "", "", "SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls, rolpassword LIKE 'SCRAM-SHA-256$%' FROM pg_authid WHERE rolname = 'a129shop_reader';")); got != "f|t" {
		t.Fatalf("reader role flags/verifier %q", got)
	}
	if resp := accessUser(t, true, read); resp.Applied || !resp.Exists {
		t.Fatalf("second create: %+v", resp)
	}

	write := base
	write.Username, write.Password, write.Access = "a129shop_editor", engineWritePassword, transport.DatabaseAccessWrite
	if resp := accessUser(t, true, write); !resp.Applied {
		t.Fatalf("write user: %+v", resp)
	}
	mustRun(t, "editor writes", e.pg("a129shop_editor", engineWritePassword, "a129shop",
		"INSERT INTO items (name) VALUES ('two'); CREATE TABLE notes (v int); INSERT INTO notes VALUES (1);"))
	// The owner keeps all rights on the editor's table.
	if got := mustRun(t, "owner reads the editor's table", e.pg("a129shop_user", engineOwnerPassword, "a129shop", "SELECT count(*) FROM notes;")); got != "1" {
		t.Fatalf("owner read %q", got)
	}

	// Removing both users leaves the database, its rows and the table the
	// editor made (now the owner's).
	for _, user := range []string{"a129shop_reader", "a129shop_editor"} {
		req := base
		req.Username = user
		if resp := accessUser(t, false, req); !resp.Applied || resp.Absent {
			t.Fatalf("remove %s: %+v", user, resp)
		}
	}
	if got := mustRun(t, "roles left", e.pg("", "", "", "SELECT count(*) FROM pg_roles WHERE rolname IN ('a129shop_reader', 'a129shop_editor');")); got != "0" {
		t.Fatalf("roles left: %s", got)
	}
	if got := mustRun(t, "data kept", e.pg("", "", "a129shop", "SELECT (SELECT count(*) FROM items) || ' ' || (SELECT count(*) FROM notes) || ' ' || (SELECT tableowner FROM pg_tables WHERE tablename = 'notes');")); got != "2 1 a129shop_user" {
		t.Fatalf("database changed by the removal: %q", got)
	}
	req := base
	req.Username = "a129shop_reader"
	if resp := accessUser(t, false, req); !resp.Applied || !resp.Absent {
		t.Fatalf("remove again: %+v", resp)
	}
	req.Username = "postgres"
	if resp := accessUser(t, false, req); resp.Applied || resp.Error == "" {
		t.Fatalf("postgres: %+v", resp)
	}

	// A new password is a SCRAM verifier and signs in; the old one does not.
	const newPassword = "N3w-Passw0rd!abc"
	var rotated transport.RotateDatabaseUserPasswordResponse
	if err := (&Agent{}).RotateDatabaseUserPasswordV1(&transport.RotateDatabaseUserPasswordRequest{
		User: transport.SuspensionDatabaseUser{Engine: "postgresql", Port: 5432, Username: "a129shop_user"}, NewPassword: newPassword,
	}, &rotated); err != nil || !rotated.Applied {
		t.Fatalf("rotate: %v %+v", err, rotated)
	}
	if got := mustRun(t, "verifier", e.pg("", "", "", "SELECT rolpassword LIKE 'SCRAM-SHA-256$%' FROM pg_authid WHERE rolname = 'a129shop_user';")); got != "t" {
		t.Fatalf("not SCRAM: %q", got)
	}
	mustRun(t, "new password signs in", e.pg("a129shop_user", newPassword, "a129shop", "SELECT 1;"))
	res = e.pg("a129shop_user", engineOwnerPassword, "a129shop", "SELECT 1;")
	mustFail(t, "old password", res)
}

func createDatabaseOn(t *testing.T, req CreateDatabaseRequest) CreateDatabaseResponse {
	t.Helper()
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

// alpha.129: the Agent's create (every path that makes a database: the
// domain, the catalogue and WordPress installs, a database copy, a site copy)
// never takes over a role or database that is already there; a replay of
// CelikPanel's own operation resumes on its own role only.
func TestCreateDatabaseNeverTakesOverAnExistingRoleOnRealPostgreSQL(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	cleanup := "DROP DATABASE IF EXISTS a129take; DROP DATABASE IF EXISTS a129takedb; DROP DATABASE IF EXISTS a129fresh; DROP DATABASE IF EXISTS a129op; " +
		"DROP ROLE IF EXISTS a129take_user; DROP ROLE IF EXISTS a129takedb_user; DROP ROLE IF EXISTS a129fresh_user; DROP ROLE IF EXISTS a129op_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	const foreignPassword = "Foreign-Passw0rd!1"
	const newPassword = "Panel-Passw0rd!123"

	// A role made outside CelikPanel: refused, nothing changed, its password kept.
	mustRun(t, "foreign role", e.pg("", "", "", "CREATE ROLE a129take_user LOGIN PASSWORD '"+foreignPassword+"';"))
	resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: "a129take", User: "a129take_user", Password: newPassword, Port: 5432})
	if resp.Success || resp.NameTaken != "user" || strings.Contains(resp.Error, newPassword) {
		t.Fatalf("foreign role: %+v", resp)
	}
	if got := mustRun(t, "no database", e.pg("", "", "", "SELECT count(*) FROM pg_database WHERE datname = 'a129take';")); got != "0" {
		t.Fatal("a database was made for a foreign role")
	}
	mustRun(t, "foreign password kept", e.pg("a129take_user", foreignPassword, "postgres", "SELECT 1;"))
	mustFail(t, "the new password does not sign in", e.pg("a129take_user", newPassword, "postgres", "SELECT 1;"))

	// A database made outside CelikPanel: refused, no role made.
	mustRun(t, "foreign database", e.pg("", "", "", "CREATE DATABASE a129takedb;"))
	resp = createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: "a129takedb", User: "a129takedb_user", Password: newPassword, Port: 5432})
	if resp.Success || resp.NameTaken != "database" {
		t.Fatalf("foreign database: %+v", resp)
	}
	if got := mustRun(t, "no role", e.pg("", "", "", "SELECT count(*) FROM pg_roles WHERE rolname = 'a129takedb_user';")); got != "0" {
		t.Fatal("a role was made for a foreign database")
	}

	// A fresh create: the user owns the database and signs in with a SCRAM
	// verifier; the same create again is refused (the panel's request
	// identity answers its own replays before the Agent).
	if resp = createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: "a129fresh", User: "a129fresh_user", Password: newPassword, Port: 5432}); !resp.Success {
		t.Fatalf("fresh: %+v", resp)
	}
	if got := mustRun(t, "owner and verifier", e.pg("", "", "", "SELECT pg_get_userbyid(datdba) || ' ' || (SELECT rolpassword LIKE 'SCRAM-SHA-256$%' FROM pg_authid WHERE rolname = 'a129fresh_user') FROM pg_database WHERE datname = 'a129fresh';")); got != "a129fresh_user true" {
		t.Fatalf("fresh: %q", got)
	}
	mustRun(t, "fresh signs in", e.pg("a129fresh_user", newPassword, "a129fresh", "SELECT 1;"))
	if resp = createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: "a129fresh", User: "a129fresh_user", Password: newPassword, Port: 5432}); resp.Success || resp.NameTaken != "user" {
		t.Fatalf("second plain create: %+v", resp)
	}

	// An operation's create: a replay of the same operation resumes on its
	// own marked role (also after the database was lost in between); another
	// operation never adopts it.
	op := CreateDatabaseRequest{Type: "postgresql", Name: "a129op", User: "a129op_user", Password: newPassword, Port: 5432,
		OperationID: strings.Repeat("ab", 16), CleanupToken: strings.Repeat("cd", 32)}
	if resp = createDatabaseOn(t, op); !resp.Success || !resp.OwnedByOperation {
		t.Fatalf("operation: %+v", resp)
	}
	if resp = createDatabaseOn(t, op); !resp.Success || !resp.OwnedByOperation {
		t.Fatalf("replay of the same operation: %+v", resp)
	}
	mustRun(t, "lose the database", e.pg("", "", "", "DROP DATABASE a129op;"))
	if resp = createDatabaseOn(t, op); !resp.Success {
		t.Fatalf("resume after the database was lost: %+v", resp)
	}
	if got := mustRun(t, "one role, owner", e.pg("", "", "", "SELECT (SELECT count(*) FROM pg_roles WHERE rolname = 'a129op_user') || ' ' || pg_get_userbyid(datdba) FROM pg_database WHERE datname = 'a129op';")); got != "1 a129op_user" {
		t.Fatalf("operation state %q", got)
	}
	other := op
	other.CleanupToken = strings.Repeat("ef", 32)
	if resp = createDatabaseOn(t, other); resp.Success || resp.NameTaken != "user" {
		t.Fatalf("another operation adopted the role: %+v", resp)
	}
}

func TestCreateDatabaseNamesWhatIsTakenOnRealMariaDB(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	cleanup := "DROP DATABASE IF EXISTS a129mtake; DROP USER IF EXISTS 'a129mtake_user'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "foreign database", e.maria("", "", "CREATE DATABASE a129mtake;"))
	resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "mysql", Name: "a129mtake", User: "a129mtake_user", Password: "Panel-Passw0rd!123"})
	if resp.Success || resp.NameTaken != "database" {
		t.Fatalf("foreign database: %+v", resp)
	}
	if got := mustRun(t, "no user", e.maria("", "", "SELECT COUNT(*) FROM mysql.user WHERE User = 'a129mtake_user';")); got != "0" {
		t.Fatal("a user was made for a foreign database")
	}
}
