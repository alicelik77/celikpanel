package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/jackc/pgx/v5/pgconn"
)

// S1 (20 Nov 2026): SQL from a file runs with the database's own rights.

func TestMariaDBDefinerClausesBecomeTheLoadingAccount(t *testing.T) {
	cases := map[string]string{
		// mariadb-dump/mysqldump: views, triggers, routines, events.
		"/*!50013 DEFINER=`shop_editor`@`localhost` SQL SECURITY DEFINER */\n":                                                             "/*!50013 DEFINER=CURRENT_USER SQL SECURITY DEFINER */\n",
		"/*!50003 CREATE*/ /*!50017 DEFINER=`root`@`localhost`*/ /*!50003 TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1 */;;\n": "/*!50003 CREATE*/ /*!50017 DEFINER=CURRENT_USER*/ /*!50003 TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1 */;;\n",
		"CREATE DEFINER=`a``b`@`%` PROCEDURE `p`()\n":                                                                                      "CREATE DEFINER=CURRENT_USER PROCEDURE `p`()\n",
		"CREATE DEFINER=`cp_user`@`localhost` FUNCTION `f`() RETURNS int(11)\n":                                                            "CREATE DEFINER=CURRENT_USER FUNCTION `f`() RETURNS int(11)\n",
		"/*!50106 CREATE*/ /*!50117 DEFINER=`root`@`localhost`*/ /*!50106 EVENT `e` ON SCHEDULE */ ;;\n":                                   "/*!50106 CREATE*/ /*!50117 DEFINER=CURRENT_USER*/ /*!50106 EVENT `e` ON SCHEDULE */ ;;\n",
		// phpMyAdmin's plain form.
		"CREATE ALGORITHM=UNDEFINED DEFINER=`root`@`localhost` SQL SECURITY DEFINER VIEW `v` AS select 1;\n": "CREATE ALGORITHM=UNDEFINED DEFINER=CURRENT_USER SQL SECURITY DEFINER VIEW `v` AS select 1;\n",
		"CREATE DEFINER='root'@'localhost' PROCEDURE p() SELECT 1;\n":                                        "CREATE DEFINER=CURRENT_USER PROCEDURE p() SELECT 1;\n",
		// Data and anything not at a line's start pass unchanged.
		"INSERT INTO t VALUES (2,'DEFINER=`x`@`y` in data');\n":       "INSERT INTO t VALUES (2,'DEFINER=`x`@`y` in data');\n",
		"CREATE TABLE `definer` (`DEFINER` int);\n":                   "CREATE TABLE `definer` (`DEFINER` int);\n",
		"/*!50013 DEFINER=CURRENT_USER SQL SECURITY INVOKER */\n":     "/*!50013 DEFINER=CURRENT_USER SQL SECURITY INVOKER */\n",
		"SELECT 1; /*!50013 DEFINER=`root`@`localhost` */\n":          "SELECT 1; /*!50013 DEFINER=`root`@`localhost` */\n",
		"no newline at the end: CREATE DEFINER=`x`@`y` PROCEDURE p()": "no newline at the end: CREATE DEFINER=`x`@`y` PROCEDURE p()",
	}
	for in, want := range cases {
		got, err := io.ReadAll(newMariaDBDefinerFilter(strings.NewReader(in)))
		if err != nil || string(got) != want {
			t.Errorf("filter(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	// A line longer than the reader's buffer: only its start is looked at.
	long := "INSERT INTO t VALUES ('" + strings.Repeat("x", 200<<10) + "');\nCREATE DEFINER=`u`@`h` PROCEDURE p()\n"
	got, err := io.ReadAll(newMariaDBDefinerFilter(strings.NewReader(long)))
	if err != nil || string(got) != strings.Replace(long, "DEFINER=`u`@`h`", "DEFINER=CURRENT_USER", 1) {
		t.Fatalf("a long line changed the result (%v)", err)
	}
}

func TestMariaDBRoutineLinesAreFound(t *testing.T) {
	yes := []string{
		"CREATE DEFINER=`u`@`localhost` FUNCTION `f`() RETURNS int(11)\n",
		"/*!50003 CREATE*/ /*!50017 DEFINER=`u`@`localhost`*/ /*!50003 TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1 */;;\n",
		"CREATE TRIGGER tr BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1;\n",
		"create function f() returns int return 1;\n",
	}
	no := []string{
		"CREATE DEFINER=`u`@`localhost` PROCEDURE `p`()\n",
		"CREATE TABLE `function` (id int);\n",
		"INSERT INTO t VALUES ('CREATE FUNCTION f');\n",
	}
	for _, line := range yes {
		if found, err := sqlReaderCreatesRoutines(strings.NewReader("SELECT 1;\n" + line)); err != nil || !found {
			t.Errorf("not found: %q", line)
		}
	}
	for _, line := range no {
		if found, err := sqlReaderCreatesRoutines(strings.NewReader(line)); err != nil || found {
			t.Errorf("found: %q", line)
		}
	}
}

// pgRecordingExecutor records what a load sends.
type pgRecordingExecutor struct {
	sent   []string
	failOn string
}

func (e *pgRecordingExecutor) Exec(ctx context.Context, sql string) error {
	e.sent = append(e.sent, strings.TrimSpace(sql))
	if e.failOn != "" && strings.Contains(sql, e.failOn) {
		return errors.New("refused")
	}
	return nil
}

func (e *pgRecordingExecutor) CopyFrom(ctx context.Context, sql string, data io.Reader) error {
	_, _ = io.Copy(io.Discard, data)
	e.sent = append(e.sent, "COPY:"+sql)
	return nil
}

// A restore's load: one transaction, the account's objects dropped first
// (what the old prelude did), the file's own SET ROLE/RESET ROLE and COMMENT
// ON EXTENSION left out; a statement the server refuses rolls everything
// back.
func TestAPostgreSQLLoadIsOneTransactionOfTheAccountsOwnStatements(t *testing.T) {
	dump := "SET statement_timeout = 0;\nRESET ROLE;\nCREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;\n" +
		"COMMENT ON EXTENSION pg_trgm IS 'text similarity';\nCREATE TABLE public.t (id int);\n"
	exec := &pgRecordingExecutor{}
	if err := pgLoadTransaction(context.Background(), exec, strings.NewReader(dump), true); err != nil {
		t.Fatal(err)
	}
	want := []string{"BEGIN", "DROP OWNED BY CURRENT_USER",
		"SET statement_timeout = 0;\nCREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;\nCREATE TABLE public.t (id int);", "COMMIT"}
	if !reflect.DeepEqual(exec.sent, want) {
		t.Fatalf("sent %q, want %q", exec.sent, want)
	}
	failing := &pgRecordingExecutor{failOn: "CREATE TABLE"}
	if err := pgLoadTransaction(context.Background(), failing, strings.NewReader(dump), true); err == nil {
		t.Fatal("a refused statement was not an error")
	}
	if last := failing.sent[len(failing.sent)-1]; last != "ROLLBACK" {
		t.Fatalf("after a refused statement the load sent %q, want ROLLBACK", last)
	}
	meta := &pgRecordingExecutor{}
	err := pgLoadTransaction(context.Background(), meta, strings.NewReader("CREATE TABLE public.t (id int);\n\\! touch /tmp/x\n"), true)
	var refusal *pgLoadRefusal
	if !errors.As(err, &refusal) || refusal.Code != "client_command" || meta.sent[len(meta.sent)-1] != "ROLLBACK" {
		t.Fatalf("a psql command: %v, sent %q", err, meta.sent)
	}
}

// pgFakeLoadConn is a session for the account checks.
type pgFakeLoadConn struct {
	pgRecordingExecutor
	superuser string
	rows      [][]string
	closed    bool
}

func (c *pgFakeLoadConn) ParameterStatus(key string) string {
	if key == "is_superuser" {
		return c.superuser
	}
	return ""
}

func (c *pgFakeLoadConn) QueryValues(ctx context.Context, sql string) ([][]string, error) {
	return c.rows, nil
}

func (c *pgFakeLoadConn) Close(ctx context.Context) error {
	c.closed = true
	return nil
}

func usePGLoadConn(t *testing.T, conn *pgFakeLoadConn, err error) {
	t.Helper()
	old := connectPGForLoad
	connectPGForLoad = func(ctx context.Context, port int, database, user, password string) (pgLoadConn, error) {
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	t.Cleanup(func() { connectPGForLoad = old })
}

func refusalCode(err error) string {
	var refusal *databaseLoadRefusal
	if errors.As(err, &refusal) {
		return refusal.Code
	}
	return ""
}

// Every reason a load cannot keep to the database's own rights is said
// before anything is written, with who acts.
func TestALoadIsRefusedBeforeAnythingWithoutTheDatabasesOwnAccount(t *testing.T) {
	pg := backupspec.DatabaseIdentity{ID: 2, Name: "shop", Type: "postgresql", ClusterPort: 5433}
	maria := backupspec.DatabaseIdentity{ID: 3, Name: "shop", Type: "mysql"}
	account := databaseLoadAccount{User: "shop_user", Password: "pw"}

	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: maria})); code != backupspec.RestoreCodeDatabaseAccount {
		t.Fatalf("no account: %q", code)
	}

	usePGLoadConn(t, &pgFakeLoadConn{superuser: "on"}, nil)
	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: pg, Account: account})); code != backupspec.RestoreCodeNeedsAdministrator {
		t.Fatalf("an administrator role: %q", code)
	}
	usePGLoadConn(t, &pgFakeLoadConn{superuser: "off", rows: [][]string{{"shop_owner", "shop_user"}}}, nil)
	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: pg, Account: account})); code != backupspec.RestoreCodeDatabaseAccount {
		t.Fatalf("not the owner: %q", code)
	}
	usePGLoadConn(t, nil, &pgconn.PgError{Code: "28P01", Message: "password authentication failed"})
	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: pg, Account: account})); code != backupspec.RestoreCodeDatabaseAccount {
		t.Fatalf("a refused password: %q", code)
	}
	owner := &pgFakeLoadConn{superuser: "off", rows: [][]string{{"shop_user", "shop_user"}}}
	usePGLoadConn(t, owner, nil)
	if err := prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: pg, Account: account}); err != nil || !owner.closed {
		t.Fatalf("the owner: %v (closed %v)", err, owner.closed)
	}

	oldClient, oldRun := mariadbClient, runDatabaseClientAsSiteUser
	t.Cleanup(func() { mariadbClient, runDatabaseClientAsSiteUser = oldClient, oldRun })
	mariadbClient = func() (string, bool) { return "/usr/bin/mysql", false }
	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: maria, Account: account, SiteUser: "shop_test"})); code != backupspec.RestoreCodeClientUnsafe {
		t.Fatalf("no sandbox mode: %q", code)
	}
	mariadbClient = func() (string, bool) { return "/usr/bin/mariadb", true }
	var argv []string
	var user string
	runDatabaseClientAsSiteUser = func(ctx context.Context, username string, args []string, env []string, stdin io.Reader) (string, error) {
		argv, user = args, username
		return "ERROR 1045 (28000): Access denied for user 'shop_user'@'localhost'", errors.New("exit status 1")
	}
	if code := refusalCode(prepareDatabaseLoad(databaseLoadTarget{DatabaseIdentity: maria, Account: account, SiteUser: "shop_test"})); code != backupspec.RestoreCodeDatabaseAccount {
		t.Fatalf("a refused MariaDB password: %q", code)
	}
	if user != "shop_test" || len(argv) < 3 || argv[1] != "--no-defaults" || argv[2] != "--sandbox" || !reflectContains(argv, "--user=shop_user") {
		t.Fatalf("sign-in ran %q as %q", argv, user)
	}
}

func reflectContains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// A server whose binary log lets only the administrator create functions
// and triggers: a dump holding one is refused before anything is loaded.
func TestARoutineTheAccountCannotCreateIsRefusedBeforeTheLoad(t *testing.T) {
	old := mysqlExecStatement
	t.Cleanup(func() { mysqlExecStatement = old })
	answer := "gate=1:0\n"
	mysqlExecStatement = func(statement string) ([]byte, error) { return []byte(answer), nil }
	gated, err := mariadbBinlogGate()
	if err != nil || !gated {
		t.Fatalf("gate = %v %v", gated, err)
	}
	answer = "gate=1:1\n"
	if gated, _ := mariadbBinlogGate(); gated {
		t.Fatal("log_bin_trust_function_creators=1 was taken for a gate")
	}
	answer = "gate=0:0\n"
	if gated, _ := mariadbBinlogGate(); gated {
		t.Fatal("no binary log was taken for a gate")
	}
	if err := checkLoadableByAccount(backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql"}, "/nonexistent"); err != nil {
		t.Fatalf("PostgreSQL has no such gate: %v", err)
	}
}
