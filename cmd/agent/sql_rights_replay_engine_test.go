//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// What a restore with the database's own rights replays from a backup the
// panel's own backup code made (S1, point 2), on the private engines.

func s1Port(e s1Engines) int {
	port := 0
	for _, c := range e.pgPort {
		port = port*10 + int(c-'0')
	}
	return port
}

// MariaDB: tables and rows (a newline and binary data among them), views, a
// view made by an added user, a trigger, a function, a procedure, a
// sequence. The added user's view comes back as the database user's own.
func TestS1MariaDBPanelBackupRestoresWithTheOwnersRights(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS s1r; DROP USER IF EXISTS 's1r_user'@'localhost', 's1r_editor'@'localhost';"
	mustRun(t, "clean", e.maria("", "", cleanup))
	t.Cleanup(func() { _ = e.maria("", "", cleanup) })
	mustRun(t, "setup", e.maria("", "", "CREATE DATABASE s1r; CREATE USER 's1r_user'@'localhost' IDENTIFIED BY '"+engineOwnerPassword+"';"+
		" CREATE USER 's1r_editor'@'localhost' IDENTIFIED BY '"+engineWritePassword+"';"+
		" GRANT ALL PRIVILEGES ON `s1r`.* TO 's1r_user'@'localhost'; GRANT ALL PRIVILEGES ON `s1r`.* TO 's1r_editor'@'localhost';"))
	mustRun(t, "objects", e.maria("s1r_user", engineOwnerPassword, strings.Join([]string{
		"USE s1r;",
		"CREATE TABLE t (id INT PRIMARY KEY, note TEXT, data BLOB);",
		"INSERT INTO t VALUES (1, 'line one\\nline two', 0x00FF), (2, 'DEFINER=`x`@`y` in data', NULL);",
		"CREATE VIEW v AS SELECT id FROM t;",
		"CREATE TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.note = CONCAT(NEW.note, '!');",
		"CREATE FUNCTION f() RETURNS INT DETERMINISTIC RETURN 7;",
		"CREATE PROCEDURE p() SELECT 1;",
		"CREATE SEQUENCE s START WITH 10;",
	}, "\n")))
	mustRun(t, "editor view", e.maria("s1r_editor", engineWritePassword, "USE s1r; CREATE VIEW ve AS SELECT note FROM t;"))
	database := backupspec.DatabaseIdentity{ID: 44, Name: "s1r", Type: "mysql"}
	agent := &Agent{}
	create := testCreateRequest(backupspec.TypeDatabase)
	create.Database = database
	info, err := agent.createBackup(create)
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, "change", e.maria("", "", "USE s1r; INSERT INTO t VALUES (3, 'later', NULL); DROP VIEW v; DROP FUNCTION f;"))
	restore := &backupspec.RestoreRequest{ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: 7, DomainID: 9,
		DomainName: "example.test", BackupName: info.Name, Database: database}
	s1WithCredentials(restore, database.ID, "s1r_user", engineOwnerPassword)
	var resp backupspec.RestoreResponse
	if err := agent.restoreBackup(restore, &resp); err != nil || !resp.Success {
		t.Fatalf("restore = %v %+v", err, resp)
	}
	got := mustRun(t, "after", e.maria("", "", "USE s1r; SELECT CONCAT((SELECT COUNT(*) FROM t), ',', (SELECT COUNT(*) FROM v), ',', f(), ',', "+
		"(SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema = 's1r'), ',', "+
		"(SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = 's1r'), ',', (SELECT HEX(data) FROM t WHERE id = 1), ',', "+
		"(SELECT definer FROM information_schema.views WHERE table_schema = 's1r' AND table_name = 've'));"))
	t.Logf("after the restore: %s", got)
	if !strings.HasPrefix(got, "2,2,7,1,2,00FF,") {
		t.Fatalf("after the restore: %s", got)
	}
}

// PostgreSQL: a schema, tables with a sequence, a view, a function and its
// trigger, a comment, a large object, an extension the owner may create
// (pg_trgm) and one only the administrator may (pageinspect, made by the
// administrator before), in a database closed to PUBLIC with a reader added
// through the panel's code. A copy into a new database cannot create the
// administrator's extension: it stops and the copy is left empty.
func TestS1PostgreSQLPanelBackupRestoresWithTheOwnersRights(t *testing.T) {
	e := s1InstallEngineClients(t)
	installBackupTestPaths(t)
	cleanup := "DROP DATABASE IF EXISTS s1q; DROP DATABASE IF EXISTS s1q_copy; DROP ROLE IF EXISTS s1q_reader; DROP ROLE IF EXISTS s1q_user; DROP ROLE IF EXISTS s1q_copy_user;"
	mustRun(t, "clean", e.pg("", "", "", cleanup))
	t.Cleanup(func() { _ = e.pg("", "", "", cleanup) })
	mustRun(t, "role", e.pg("", "", "", "CREATE ROLE s1q_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "database", e.pg("", "", "", "CREATE DATABASE s1q OWNER s1q_user;"))
	mustRun(t, "public closed", e.pg("", "", "", "REVOKE ALL ON DATABASE s1q FROM PUBLIC;"))
	mustRun(t, "administrator extension", e.pg("", "", "s1q", "CREATE EXTENSION pageinspect;"))
	mustRun(t, "objects", e.pg("s1q_user", engineOwnerPassword, "s1q", strings.Join([]string{
		"CREATE EXTENSION pg_trgm;",
		"CREATE SCHEMA app;",
		"CREATE TABLE app.items (id serial PRIMARY KEY, name text NOT NULL, meta jsonb);",
		"INSERT INTO app.items (name, meta) VALUES ('one', '{\"a\":1}'), ('two', NULL);",
		"CREATE VIEW app.names AS SELECT name FROM app.items;",
		"CREATE FUNCTION app.stamp() RETURNS trigger LANGUAGE plpgsql AS $f$ BEGIN NEW.name := NEW.name || '!'; RETURN NEW; END $f$;",
		"CREATE TRIGGER stamp BEFORE INSERT ON app.items FOR EACH ROW EXECUTE FUNCTION app.stamp();",
		"COMMENT ON TABLE app.items IS 'the items';",
		"SELECT lo_from_bytea(424242, 'large');",
	}, "\n")))
	read := transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "s1q",
		Username: "s1q_reader", Password: engineReadPassword, Access: transport.DatabaseAccessRead}
	if resp := accessUser(t, true, read); !resp.Applied {
		t.Fatalf("adding a reader: %+v", resp)
	}
	database := backupspec.DatabaseIdentity{ID: 45, Name: "s1q", Type: "postgresql", ClusterPort: s1Port(e)}
	agent := &Agent{}
	create := testCreateRequest(backupspec.TypeDatabase)
	create.Database = database
	info, err := agent.createBackup(create)
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, "change", e.pg("s1q_user", engineOwnerPassword, "s1q", "INSERT INTO app.items (name) VALUES ('later'); DROP VIEW app.names;"))
	restore := &backupspec.RestoreRequest{ProtocolVersion: backupspec.ProtocolVersion, SubscriptionID: 7, DomainID: 9,
		DomainName: "example.test", BackupName: info.Name, Database: database}
	s1WithCredentials(restore, database.ID, "s1q_user", engineOwnerPassword)
	var resp backupspec.RestoreResponse
	if err := agent.restoreBackup(restore, &resp); err != nil || !resp.Success {
		t.Fatalf("restore = %v %+v", err, resp)
	}
	mustRun(t, "trigger", e.pg("s1q_user", engineOwnerPassword, "s1q", "INSERT INTO app.items (name) VALUES ('new');"))
	got := mustRun(t, "after", e.pg("s1q_user", engineOwnerPassword, "s1q",
		"SELECT (SELECT count(*) FROM app.names) || ',' || (SELECT name FROM app.items ORDER BY id DESC LIMIT 1) || ',' || "+
			"(similarity('shop', 'shops') > 0)::text || ',' || obj_description('app.items'::regclass) || ',' || "+
			"(SELECT count(*) FROM pg_largeobject_metadata WHERE oid = 424242) || ',' || (SELECT extowner::regrole::text FROM pg_extension WHERE extname = 'pageinspect');"))
	if got != "3,new!,true,the items,1,postgres" {
		t.Fatalf("after the restore: %s", got)
	}
	// The reader's rights: established, not asserted (the restore recreates
	// the objects, as it did before this lane).
	reader := e.pg("s1q_reader", engineReadPassword, "s1q", "SELECT count(*) FROM app.items;")
	t.Logf("after a restore the added reader reads app.items: %v (%s)", reader.err == nil, strings.TrimSpace(string(reader.out)))

	// A copy into a new database: the administrator's extension cannot be
	// made by the new database's owner; the copy stops and nothing is left.
	mustRun(t, "copy role", e.pg("", "", "", "CREATE ROLE s1q_copy_user LOGIN PASSWORD '"+engineOwnerPassword+"';"))
	mustRun(t, "copy database", e.pg("", "", "", "CREATE DATABASE s1q_copy OWNER s1q_copy_user;"))
	var copied transport.DBToolsCopyResponse
	source := transport.DBToolsTarget{SubscriptionID: 7, DomainID: 9, Domain: "example.test", Database: database}
	err = agent.DatabaseCopyV1(s1CopyRequest(source, backupspec.DatabaseIdentity{ID: 46, Name: "s1q_copy", Type: "postgresql", ClusterPort: s1Port(e)}, "s1q_copy_user", engineOwnerPassword), &copied)
	t.Logf("copy with an administrator's extension: success=%v error=%q", copied.Success, copied.Error)
	if err != nil || copied.Success || !strings.Contains(copied.Error, "pageinspect") {
		t.Fatalf("copy = %v %+v, want a refused extension", err, copied)
	}
	if tables := mustRun(t, "copy tables", e.pg("", "", "s1q_copy", "SELECT count(*) FROM pg_tables WHERE schemaname NOT IN ('pg_catalog', 'information_schema');")); tables != "0" {
		t.Fatalf("the stopped copy left %s tables", tables)
	}
}
