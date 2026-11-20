package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.141 carried item 12: the Agent drops a database only while it is the
// one the deletion identified before its first drop.

// fakeOnceMariaDB answers the identity read and records the drop.
type fakeOnceMariaDB struct {
	exists     bool
	dataDir    string
	lower      string
	statements []string
}

func useFakeOnceMariaDB(t *testing.T, e *fakeOnceMariaDB, directories map[string]string) {
	t.Helper()
	savedExec, savedDir := mysqlExecStatement, mariadbDatabaseDirectory
	mysqlExecStatement = func(statement string) ([]byte, error) {
		e.statements = append(e.statements, statement)
		if strings.HasPrefix(statement, "SELECT CONCAT('n='") {
			n := "0"
			if e.exists {
				n = "1"
			}
			return []byte("n=" + n + "\nd=" + e.dataDir + "\nl=" + e.lower + "\n"), nil
		}
		if strings.HasPrefix(statement, "DROP DATABASE") {
			e.exists = false
		}
		return nil, nil
	}
	mariadbDatabaseDirectory = func(path string) string { return directories[path] }
	t.Cleanup(func() { mysqlExecStatement, mariadbDatabaseDirectory = savedExec, savedDir })
}

func dropOnce(t *testing.T, req transport.DropDatabaseOnceRequest) transport.DropDatabaseOnceResponse {
	t.Helper()
	var resp transport.DropDatabaseOnceResponse
	if err := (&Agent{}).DropDatabaseOnceV1(&req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestDropDatabaseOnceDropsOnlyTheIdentifiedMariaDBDatabase(t *testing.T) {
	e := &fakeOnceMariaDB{exists: true, dataDir: "/var/lib/mysql/", lower: "0"}
	directories := map[string]string{"/var/lib/mysql/shop_db": "mariadb-dir:8:1:42:1700000000.000000001"}
	useFakeOnceMariaDB(t, e, directories)

	var read transport.DatabaseIdentityResponse
	if err := (&Agent{}).DatabaseIdentityV1(&transport.DatabaseIdentityRequest{Type: "mysql", Name: "shop_db"}, &read); err != nil ||
		!read.Exists || read.Identity != "mariadb-dir:8:1:42:1700000000.000000001" || read.Error != "" {
		t.Fatalf("identity %+v %v", read, err)
	}
	// The name now holds another database (made again: same inode number,
	// another birth time): nothing is dropped.
	directories["/var/lib/mysql/shop_db"] = "mariadb-dir:8:1:42:1700000999.000000001"
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db", Identity: read.Identity}); !resp.Replaced || resp.Dropped || resp.Error != "" {
		t.Fatalf("replaced: %+v", resp)
	}
	// Without an identity a later press never drops what is there.
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db"}); resp.Dropped || resp.Error == "" {
		t.Fatalf("no identity: %+v", resp)
	}
	for _, statement := range e.statements {
		if strings.HasPrefix(statement, "DROP") {
			t.Fatalf("dropped: %q", statement)
		}
	}
	// The identified database: dropped, once.
	directories["/var/lib/mysql/shop_db"] = read.Identity
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db", Identity: read.Identity}); !resp.Dropped {
		t.Fatalf("identified: %+v", resp)
	}
	if last := e.statements[len(e.statements)-1]; last != "DROP DATABASE IF EXISTS `shop_db`;" {
		t.Fatalf("drop statement %q", last)
	}
	// Gone: absent, nothing sent.
	before := len(e.statements)
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db", Identity: read.Identity}); !resp.Absent || resp.Dropped {
		t.Fatalf("absent: %+v", resp)
	}
	if len(e.statements) != before+1 {
		t.Fatalf("statements after absent: %q", e.statements[before:])
	}
}

func TestDropDatabaseOnceByNameOnlyOnTheFirstPressWithoutAnIdentity(t *testing.T) {
	e := &fakeOnceMariaDB{exists: true, dataDir: "/var/lib/mysql/", lower: "0"}
	useFakeOnceMariaDB(t, e, map[string]string{})
	var read transport.DatabaseIdentityResponse
	_ = (&Agent{}).DatabaseIdentityV1(&transport.DatabaseIdentityRequest{Type: "mysql", Name: "shop_db"}, &read)
	if !read.Exists || read.Identity != "" {
		t.Fatalf("identity %+v", read)
	}
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db", ByName: true}); !resp.Dropped {
		t.Fatalf("by name: %+v", resp)
	}
}

func TestMariaDBDatabaseIdentityFollowsTheEnginesNaming(t *testing.T) {
	e := &fakeOnceMariaDB{exists: true, dataDir: "/data/", lower: "1"}
	asked := ""
	useFakeOnceMariaDB(t, e, nil)
	mariadbDatabaseDirectory = func(path string) string { asked = path; return "id" }
	if exists, identity, err := mysqlDatabaseIdentity("Shop_DB"); err != nil || !exists || identity != "id" || asked != "/data/shop_db" {
		t.Fatalf("lower_case_table_names=1: %v %q %v asked %q", exists, identity, err, asked)
	}
	e.dataDir = "relative/"
	if _, identity, _ := mysqlDatabaseIdentity("shop_db"); identity != "" {
		t.Fatalf("a data directory that is not absolute gave identity %q", identity)
	}
	e.exists = false
	if exists, _, err := mysqlDatabaseIdentity("shop_db"); exists || err != nil {
		t.Fatalf("absent: %v %v", exists, err)
	}
}

func TestDropDatabaseOncePostgreSQLIsBoundToTheOid(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	oid := "16390"
	var got []string
	savedPort := postgreSQLExecStatementOnPort
	postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
		got = append(got, statement)
		if strings.HasPrefix(statement, "SELECT 'oid='") {
			return []byte("oid=" + oid + "\n"), nil
		}
		if strings.Contains(statement, "DROP DATABASE") {
			oid = ""
		}
		return nil, nil
	}
	t.Cleanup(func() { postgreSQLExecStatementOnPort = savedPort })

	var read transport.DatabaseIdentityResponse
	_ = (&Agent{}).DatabaseIdentityV1(&transport.DatabaseIdentityRequest{Type: "postgresql", Name: "shop", Port: 5433}, &read)
	if !read.Exists || read.Identity != "postgresql-oid:16390" {
		t.Fatalf("identity %+v", read)
	}
	oid = "16502" // made again
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "postgresql", Name: "shop", Port: 5433, Identity: read.Identity}); !resp.Replaced {
		t.Fatalf("replaced: %+v", resp)
	}
	for _, statement := range got {
		if strings.Contains(statement, "DROP") || strings.Contains(statement, "pg_terminate_backend") {
			t.Fatalf("a replaced database was touched: %q", statement)
		}
	}
	oid = "16390"
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "postgresql", Name: "shop", Port: 5433, Identity: read.Identity}); !resp.Dropped {
		t.Fatalf("identified: %+v", resp)
	}
	if last := got[len(got)-1]; !strings.Contains(last, "pg_terminate_backend") || !strings.Contains(last, `DROP DATABASE IF EXISTS "shop";`) {
		t.Fatalf("drop script %q", last)
	}
	// The removed major's cluster on 5432 answers as a refusal, nothing run.
	before := len(got)
	if resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "postgresql", Name: "shop", Port: 5432, Identity: read.Identity}); resp.Refusal == nil || resp.Dropped {
		t.Fatalf("refusal: %+v", resp)
	}
	if len(got) != before {
		t.Fatal("a statement ran on a refused cluster")
	}
}

// The directory identity itself, on this machine's filesystem: a directory
// made again under the same name is another identity, also when the
// filesystem hands it the freed inode number.
func TestDatabaseDirectoryIdentityChangesWhenTheDirectoryIsMadeAgain(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("birth times are read on Linux only")
	}
	dir := filepath.Join(t.TempDir(), "shop_db")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	first := databaseDirectoryIdentity(dir)
	if first == "" {
		t.Skip("this filesystem records no birth time")
	}
	if again := databaseDirectoryIdentity(dir); again != first {
		t.Fatalf("one directory read twice: %q then %q", first, again)
	}
	if err := os.WriteFile(filepath.Join(dir, "t.frm"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := databaseDirectoryIdentity(dir); after != first {
		t.Fatalf("a table added changed the identity: %q then %q", first, after)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if second := databaseDirectoryIdentity(dir); second == "" || second == first {
		t.Fatalf("made again: %q, first %q", second, first)
	}
	link := dir + "_link"
	if err := os.Symlink(dir, link); err == nil {
		if got := databaseDirectoryIdentity(link); got != "" {
			t.Fatalf("a link was followed: %q", got)
		}
	}
}

// A client that lost its connection during the drop: the database is looked
// for again; gone is dropped, still there is the verified failure.
func TestDropDatabaseOnceReadsTheResultOfADropWhoseClientFailed(t *testing.T) {
	for _, gone := range []bool{true, false} {
		e := &fakeOnceMariaDB{exists: true, dataDir: "/var/lib/mysql/", lower: "0"}
		useFakeOnceMariaDB(t, e, map[string]string{"/var/lib/mysql/shop_db": "id-1"})
		inner := mysqlExecStatement
		mysqlExecStatement = func(statement string) ([]byte, error) {
			if strings.HasPrefix(statement, "DROP DATABASE") {
				e.statements = append(e.statements, statement)
				e.exists = !gone
				return []byte("ERROR 2013 (HY000): Lost connection to server during query"), errors.New("exit status 1")
			}
			return inner(statement)
		}
		resp := dropOnce(t, transport.DropDatabaseOnceRequest{Type: "mysql", Name: "shop_db", Identity: "id-1"})
		if resp.Dropped != gone || (resp.Error == "") != gone {
			t.Fatalf("gone=%v: %+v", gone, resp)
		}
	}
}
