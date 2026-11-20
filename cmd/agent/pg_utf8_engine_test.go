package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// 20 Nov 2026, on a real PostgreSQL (the private engines of
// database_access_users_engine_test.go) whose cluster was initialised the way
// the owner's server 1 was: locale C, so template0, template1 and postgres
// are SQL_ASCII. Skipped where the engines' templates are something else.
//
// Gerçek bir PostgreSQL'de, şablonları SQL_ASCII olan bir kümede.

const turkishText = "Çağrı Işık İstanbulda şöyle yazdı: ğüöçış ÇĞİÖŞÜ"

func requireSQLASCIITemplates(t *testing.T, e testEngines) {
	t.Helper()
	if got := mustRun(t, "template1", e.pg("", "", "", "SELECT pg_encoding_to_char(encoding) || ' ' || datcollate FROM pg_database WHERE datname = 'template1';")); got != "SQL_ASCII C" {
		t.Skipf("these engines' template1 is %q; this test needs a cluster initialised with locale C (SQL_ASCII templates)", got)
	}
}

func TestPGDatabaseIsUTF8OnAClusterWithSQLASCIITemplatesOnRealPostgreSQL(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	requireSQLASCIITemplates(t, e)
	cleanup := "DROP DATABASE IF EXISTS a140enc; DROP ROLE IF EXISTS a140enc_user;"
	for _, form := range []string{"f17", "f16", "f14", "fc16", "fc14"} {
		cleanup += " DROP DATABASE IF EXISTS a140" + form + ";"
	}
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })

	// The Agent's own create (every panel path that makes a PostgreSQL
	// database goes through it).
	if resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: "a140enc", User: "a140enc_user", Password: engineOwnerPassword, Port: 5432}); !resp.Success {
		t.Fatalf("create: %+v", resp)
	}
	version := mustRun(t, "version", e.pg("", "", "", "SHOW server_version_num;"))
	want := "UTF8 b C.UTF-8 C C"
	if services.PostgreSQLMajor(services.ParsePostgreSQLVersionNum(version)) < 17 {
		t.Fatalf("these engines are PostgreSQL %s; this test reads PostgreSQL 17's catalogue", version)
	}
	if got := mustRun(t, "encoding", e.pg("", "", "", "SELECT pg_encoding_to_char(encoding) || ' ' || datlocprovider::text || ' ' || COALESCE(datlocale, '') || ' ' || datcollate || ' ' || datctype FROM pg_database WHERE datname = 'a140enc';")); got != want {
		t.Fatalf("a140enc = %q, want %q", got, want)
	}

	// Turkish text as the database's own user: stored and read back as it
	// was, counted in characters, cased by Unicode; bytes that are not UTF-8
	// are refused (a SQL_ASCII database took them).
	mustRun(t, "table", e.pg("a140enc_user", engineOwnerPassword, "a140enc", "CREATE TABLE notes (body text); INSERT INTO notes VALUES ('"+turkishText+"');"))
	if got := mustRun(t, "read back", e.pg("a140enc_user", engineOwnerPassword, "a140enc", "SELECT body FROM notes;")); got != turkishText {
		t.Fatalf("read back %q", got)
	}
	if got := mustRun(t, "length", e.pg("a140enc_user", engineOwnerPassword, "a140enc", "SELECT length('ğüşıöçİ') || ' ' || octet_length('ğüşıöçİ') || ' ' || upper('çğıöşü') || ' ' || lower('ÇĞÖŞÜ');")); got != "7 14 ÇĞIÖŞÜ çğöşü" {
		t.Fatalf("characters and case: %q", got)
	}
	res := e.pg("a140enc_user", engineOwnerPassword, "a140enc", `SELECT E'caf\xe9'::text;`)
	if res.err == nil || !strings.Contains(string(res.out), `invalid byte sequence for encoding "UTF8"`) {
		t.Fatalf("a Latin-1 byte was accepted: %v %s", res.err, res.out)
	}

	// Every major's form runs on this cluster and gives UTF8 (the forms for
	// 15/16 and 14 and earlier are accepted by 17 as well; their syntax on
	// those majors is the official CREATE DATABASE page of each).
	for form, versionNum := range map[string]int{"f17": 170000, "f16": 160000, "f14": 140000} {
		mustRun(t, form, e.pg("", "", "", services.PostgreSQLCreateDatabaseStatement("a140"+form, "", versionNum, false)))
	}
	for form, versionNum := range map[string]int{"fc16": 160000, "fc14": 140000} {
		mustRun(t, form, e.pg("", "", "", services.PostgreSQLCreateDatabaseStatement("a140"+form, "", versionNum, true)))
	}
	if got := mustRun(t, "forms", e.pg("", "", "", "SELECT string_agg(datname || '=' || pg_encoding_to_char(encoding) || '/' || datlocprovider::text || '/' || datcollate, ' ' ORDER BY datname) FROM pg_database WHERE datname LIKE 'a140f%';")); got !=
		"a140f14=UTF8/c/C.UTF-8 a140f16=UTF8/c/C.UTF-8 a140f17=UTF8/b/C a140fc14=UTF8/c/C a140fc16=UTF8/c/C" {
		t.Fatalf("forms = %q", got)
	}
}

// 20 Nov 2026 (security), on the same real engine, with the distribution's
// pg_hba (scram-sha-256 on 127.0.0.1): a database the Agent makes is closed
// to every other role. Another customer's user is refused at the door; the
// owner, a read-only user (CONNECT) and a read-and-write user (CONNECT,
// TEMPORARY) the panel adds still connect; the export (as the engine's
// superuser) still reads it.
func TestANewDatabaseIsClosedToOtherCustomersOnRealPostgreSQL(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	useEngineClients(t, e)
	cleanup := "DROP DATABASE IF EXISTS a140a; DROP DATABASE IF EXISTS a140b; DROP ROLE IF EXISTS a140a_user; DROP ROLE IF EXISTS a140b_user;" +
		" DROP ROLE IF EXISTS a140a_reader; DROP ROLE IF EXISTS a140a_editor;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	for _, name := range []string{"a140a", "a140b"} {
		if resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: name, User: name + "_user", Password: engineOwnerPassword, Port: 5432}); !resp.Success {
			t.Fatalf("create %s: %+v", name, resp)
		}
	}
	if got := mustRun(t, "public", e.pg("", "", "", "SELECT has_database_privilege('public', 'a140a', 'CONNECT') || ' ' || has_database_privilege('public', 'a140a', 'TEMPORARY') || ' ' || has_database_privilege('a140b_user', 'a140a', 'CONNECT');")); got != "false false false" {
		t.Fatalf("public/other = %q", got)
	}
	res := e.pg("a140b_user", engineOwnerPassword, "a140a", "SELECT 1;")
	if res.err == nil || !strings.Contains(string(res.out), "User does not have CONNECT privilege") {
		t.Fatalf("another customer's user got in: %v %s", res.err, res.out)
	}
	t.Logf("another customer's user at the door: %s", strings.TrimSpace(string(res.out)))
	mustRun(t, "owner", e.pg("a140a_user", engineOwnerPassword, "a140a", "CREATE TABLE notes (s text); CREATE TEMP TABLE scratch (s text);"))

	base := transport.DatabaseAccessUserRequest{Engine: "postgresql", Port: 5432, Database: "a140a"}
	read, write := base, base
	read.Username, read.Password, read.Access = "a140a_reader", engineReadPassword, transport.DatabaseAccessRead
	write.Username, write.Password, write.Access = "a140a_editor", engineWritePassword, transport.DatabaseAccessWrite
	for _, req := range []transport.DatabaseAccessUserRequest{read, write} {
		if resp := accessUser(t, true, req); !resp.Applied {
			t.Fatalf("%s: %+v", req.Username, resp)
		}
	}
	mustRun(t, "reader connects and reads", e.pg("a140a_reader", engineReadPassword, "a140a", "SELECT count(*) FROM notes;"))
	mustFail(t, "reader makes a temporary table", e.pg("a140a_reader", engineReadPassword, "a140a", "CREATE TEMP TABLE r (s text);"))
	mustRun(t, "editor writes and makes a temporary table", e.pg("a140a_editor", engineWritePassword, "a140a", "INSERT INTO notes VALUES ('x'); CREATE TEMP TABLE w (s text);"))
	mustFail(t, "the reader is still a stranger to a140b", e.pg("a140a_reader", engineReadPassword, "a140b", "SELECT 1;"))
	if err := dumpDatabaseToFile(backupspec.DatabaseIdentity{ID: 1, Name: "a140a", Type: "postgresql", ClusterPort: atoiOrZero(e.pgPort)}, filepath.Join(t.TempDir(), "a140a.sql.gz")); err != nil {
		t.Fatalf("export of a closed database: %v", err)
	}
}

func atoiOrZero(text string) int {
	n, _ := strconv.Atoi(text)
	return n
}

// useEngineClients points the backup commands (sudo -u postgres pg_dump/psql)
// at the private engine's own clients.
func useEngineClients(t *testing.T, e testEngines) {
	t.Helper()
	previous := backupCommandContext
	backupCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// S1: sudo -u postgres env PGOPTIONS=... <client> ...
		if name == "sudo" && len(args) > 5 && args[0] == "-u" && args[2] == "env" && (args[4] == "pg_dump" || args[4] == "psql") {
			client := filepath.Join(e.dir, "root/usr/lib/postgresql/17/bin", args[4])
			cmd := exec.CommandContext(ctx, client, append([]string{"-h", filepath.Join(e.dir, "run"), "-U", "postgres"}, args[5:]...)...)
			cmd.Env = append(e.env(), args[3])
			return cmd
		}
		t.Fatalf("unexpected backup command %s %v", name, args)
		return nil
	}
	t.Cleanup(func() { backupCommandContext = previous })
}

// The way out the card names for a SQL_ASCII database with data, through the
// panel's own code: export (the backup's dump), a new database (now UTF8),
// import (as the database's own user) - and the copy's restore step. Text
// that is valid UTF-8 moves as it is; a byte that is not stops the import, the
// new database is put back empty, and the reason names the byte.
func TestSQLASCIIDatabaseMovesToUTF8ThroughExportAndImportOnRealPostgreSQL(t *testing.T) {
	e := requireTestEngines(t)
	installTestEngines(t, e)
	requireSQLASCIITemplates(t, e)
	useEngineClients(t, e)
	port, err := strconv.Atoi(e.pgPort)
	if err != nil {
		t.Fatalf("CELIKPANEL_TEST_PG_PORT = %q", e.pgPort)
	}
	names := []string{"a140legacy", "a140latin", "a140moved", "a140copy", "a140stuck", "a140stuckcopy"}
	cleanup := ""
	for _, name := range names {
		cleanup += "DROP DATABASE IF EXISTS " + name + "; DROP ROLE IF EXISTS " + name + "_user; "
	}
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })

	// Two databases as the panel made them before this release: no options,
	// so SQL_ASCII from template1. One holds valid UTF-8 (what a site writes
	// through a UTF-8 connection), the other a Latin-1 byte.
	for _, name := range []string{"a140legacy", "a140latin"} {
		mustRun(t, name, e.pg("", "", "", "CREATE ROLE "+name+"_user LOGIN PASSWORD '"+engineOwnerPassword+"'; CREATE DATABASE "+name+" OWNER "+name+"_user;"))
	}
	if got := mustRun(t, "legacy encoding", e.pg("", "", "", "SELECT pg_encoding_to_char(encoding) FROM pg_database WHERE datname = 'a140legacy';")); got != "SQL_ASCII" {
		t.Fatalf("legacy = %q", got)
	}
	mustRun(t, "legacy data", e.pg("a140legacy_user", engineOwnerPassword, "a140legacy",
		"CREATE TABLE notes (id int PRIMARY KEY, body text NOT NULL); CREATE INDEX notes_body ON notes (body);"+
			" INSERT INTO notes VALUES (1, '"+turkishText+"'), (2, 'plain ascii');"+
			" CREATE FUNCTION greet() RETURNS text LANGUAGE sql AS $$ SELECT 'Günaydın' $$;"))
	mustRun(t, "latin data", e.pg("a140latin_user", engineOwnerPassword, "a140latin", `CREATE TABLE notes (body text); INSERT INTO notes VALUES (E'caf\xe9 au lait');`))

	dir := t.TempDir()
	identity := func(id int, name string) backupspec.DatabaseIdentity {
		return backupspec.DatabaseIdentity{ID: id, Name: name, Type: "postgresql", ClusterPort: port}
	}
	exportOf := func(id int, name string) string {
		path := filepath.Join(dir, name+".sql.gz")
		if err := dumpDatabaseToFile(identity(id, name), path); err != nil {
			t.Fatalf("export %s: %v", name, err)
		}
		return path
	}
	importInto := func(id int, name, file string) transport.DBToolsImportResponse {
		var resp transport.DBToolsImportResponse
		importPostgreSQL(context.Background(), identity(id, name), transport.DBToolsImportRequest{User: name + "_user", Password: engineOwnerPassword}, file, &resp)
		return resp
	}
	newDatabase := func(name string) {
		if resp := createDatabaseOn(t, CreateDatabaseRequest{Type: "postgresql", Name: name, User: name + "_user", Password: engineOwnerPassword, Port: port}); !resp.Success {
			t.Fatalf("create %s: %+v", name, resp)
		}
	}

	// Valid UTF-8: export -> new database -> import.
	legacyFile := exportOf(1, "a140legacy")
	newDatabase("a140moved")
	if resp := importInto(3, "a140moved", legacyFile); !resp.Success {
		t.Fatalf("import: %+v", resp)
	}
	if got := mustRun(t, "moved", e.pg("a140moved_user", engineOwnerPassword, "a140moved",
		"SELECT pg_encoding_to_char(encoding) FROM pg_database WHERE datname = current_database();"+
			" SELECT body FROM notes WHERE id = 1; SELECT length(body) FROM notes WHERE id = 1; SELECT greet();")); got != "UTF8\n"+turkishText+"\n"+strconv.Itoa(len([]rune(turkishText)))+"\nGünaydın" {
		t.Fatalf("moved = %q", got)
	}
	// The copy's step (Copy on the card): restore the same dump into a new database.
	newDatabase("a140copy")
	if err := restoreDatabaseFromFile(databaseLoadTarget{DatabaseIdentity: identity(4, "a140copy"), Account: databaseLoadAccount{User: "a140copy_user", Password: engineOwnerPassword}}, legacyFile); err != nil {
		t.Fatalf("copy restore: %v", err)
	}
	if got := mustRun(t, "copy", e.pg("a140copy_user", engineOwnerPassword, "a140copy", "SELECT body FROM notes WHERE id = 1;")); got != turkishText {
		t.Fatalf("copy = %q", got)
	}

	// Not UTF-8: the import stops, the new database is put back as it was.
	latinFile := exportOf(2, "a140latin")
	newDatabase("a140stuck")
	resp := importInto(5, "a140stuck", latinFile)
	if resp.Success || !resp.RolledBack || resp.Code != transport.DBImportCodeLoadFailed ||
		!strings.Contains(resp.Error, `invalid byte sequence for encoding "UTF8": 0xe9`) {
		t.Fatalf("latin import: %+v", resp)
	}
	t.Logf("what the import answers for a byte that is not UTF-8: %s", resp.Error)
	if got := mustRun(t, "stuck is empty", e.pg("", "", "a140stuck", "SELECT count(*) FROM pg_tables WHERE schemaname = 'public';")); got != "0" {
		t.Fatalf("the failed import left %s tables", got)
	}
	newDatabase("a140stuckcopy")
	err = restoreDatabaseFromFile(databaseLoadTarget{DatabaseIdentity: identity(6, "a140stuckcopy"), Account: databaseLoadAccount{User: "a140stuckcopy_user", Password: engineOwnerPassword}}, latinFile)
	if err == nil || !strings.Contains(err.Error(), `invalid byte sequence for encoding "UTF8": 0xe9`) {
		t.Fatalf("latin copy restore: %v", err)
	}
	t.Logf("what the copy's restore answers for a byte that is not UTF-8: %s", boundedText(err.Error(), 300))
	// The source is untouched either way.
	if got := mustRun(t, "source kept", e.pg("", "", "a140latin", "SELECT count(*) FROM notes;")); got != "1" {
		t.Fatalf("source = %q", got)
	}
	// A SQL_ASCII database's own backup still restores into it as before,
	// whatever bytes it holds (the backup restore uses the same step).
	if err := restoreDatabaseFromFile(databaseLoadTarget{DatabaseIdentity: identity(2, "a140latin"), Account: databaseLoadAccount{User: "a140latin_user", Password: engineOwnerPassword}}, latinFile); err != nil {
		t.Fatalf("restore into the SQL_ASCII source: %v", err)
	}
	if got := mustRun(t, "source bytes", e.pg("", "", "a140latin", "SELECT encode(body::bytea, 'hex') FROM notes;")); got != "636166e9206175206c616974" {
		t.Fatalf("source bytes = %q", got)
	}
}
