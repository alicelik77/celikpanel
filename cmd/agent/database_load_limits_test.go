package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// S4 R2a: a database user's time and temporary-file limits never cut the
// panel's own loads (restore, copy, import as the database's own user).

// TestAPostgreSQLLoadLiftsTheUsersLimitsForItself: the load's session sets
// the two user-settable timeouts to 0 before its transaction, and the
// administrator lifts temp_file_limit for that one database while it runs
// and removes the lift afterwards, on the database's own cluster.
func TestAPostgreSQLLoadLiftsTheUsersLimitsForItself(t *testing.T) {
	conn := &pgFakeLoadConn{superuser: "off", rows: [][]string{{"shop_user", "shop_user"}}}
	usePGLoadConn(t, conn, nil)
	old := postgreSQLExecStatementOnPort
	t.Cleanup(func() { postgreSQLExecStatementOnPort = old })
	var admin []string
	current := conn
	postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
		if port != 5433 {
			t.Errorf("administrator statement on port %d", port)
		}
		admin = append(admin, statement)
		// The lift happens before the load signs in.
		if len(admin) == 1 && len(current.sent) != 0 {
			t.Errorf("the lift came after the load started: %q", current.sent)
		}
		return nil, nil
	}
	target := databaseLoadTarget{
		DatabaseIdentity: backupspec.DatabaseIdentity{ID: 2, Name: "shop", Type: "postgresql", ClusterPort: 5433},
		Account:          databaseLoadAccount{User: "shop_user", Password: "pw"},
	}
	if err := loadDatabaseSQL(context.Background(), target, strings.NewReader("CREATE TABLE public.t (id int);\n"), true); err != nil {
		t.Fatal(err)
	}
	want := []string{"SET statement_timeout = 0", "SET idle_in_transaction_session_timeout = 0", "BEGIN", "DROP OWNED BY CURRENT_USER",
		"CREATE TABLE public.t (id int);", "COMMIT"}
	if !reflect.DeepEqual(conn.sent, want) {
		t.Fatalf("the load sent %q, want %q", conn.sent, want)
	}
	if len(admin) != 2 || !strings.Contains(admin[0], `ALTER ROLE "shop_user" IN DATABASE "shop" SET temp_file_limit = -1`) ||
		!strings.Contains(admin[0], "s.setdatabase OPERATOR(pg_catalog.=) 0") ||
		admin[1] != `ALTER ROLE "shop_user" IN DATABASE "shop" RESET temp_file_limit;` {
		t.Fatalf("administrator statements: %q", admin)
	}
	// A refused load still removes the lift.
	admin = nil
	failing := &pgFakeLoadConn{superuser: "off", pgRecordingExecutor: pgRecordingExecutor{failOn: "CREATE TABLE"}}
	usePGLoadConn(t, failing, nil)
	current = failing
	if err := loadDatabaseSQL(context.Background(), target, strings.NewReader("CREATE TABLE public.t (id int);\n"), true); err == nil {
		t.Fatal("a refused statement was not an error")
	}
	if len(admin) != 2 || !strings.Contains(admin[1], "RESET temp_file_limit") {
		t.Fatalf("after a failed load: %q", admin)
	}
	// Names are quoted, never pasted.
	if _, err := pgLoadTempFileLiftSQL(`x"; DROP ROLE postgres; --`, "shop"); err == nil {
		t.Fatal("an unsafe role name reached the administrator's statement")
	}
}

// TestAMariaDBLoadLiftsTheStatementTime: the client's first statement sets
// the session's statement time to 0; the sandbox and no option file stay.
func TestAMariaDBLoadLiftsTheStatementTime(t *testing.T) {
	argv := mariadbLoadArgv("/usr/bin/mariadb", "shop", "shop_user")
	if argv[1] != "--no-defaults" || argv[2] != "--sandbox" || !reflectContains(argv, "--init-command=SET SESSION max_statement_time=0") {
		t.Fatalf("argv = %q", argv)
	}
}
