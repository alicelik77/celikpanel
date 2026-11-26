package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

func syscallMkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }

// alpha.149 fix round B (lab D10): a site role's limits through the Agent, as
// the superuser, by one narrow call.

func withPGRoleLimitsStubs(t *testing.T, recorded bool, run func(port int, statement string) ([]byte, error)) *[]string {
	t.Helper()
	restoreRecorded, restoreRun, restoreDefault, restoreClusters := pgRoleRecordedByPanel, pgRoleLimitsRun, postgreSQLExecStatement, pgStatementClusters
	restoreRunning := pgServerRunning
	t.Cleanup(func() {
		pgRoleRecordedByPanel, pgRoleLimitsRun, postgreSQLExecStatement, pgStatementClusters = restoreRecorded, restoreRun, restoreDefault, restoreClusters
		pgServerRunning = restoreRunning
	})
	pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) { return nil, false }
	pgRoleRecordedByPanel = func(context.Context, string, int) (bool, error) { return recorded, nil }
	// alpha.149: the server runs unless a test says otherwise.
	pgServerRunning = func(int) (bool, bool) { return true, true }
	ran := []string{}
	pgRoleLimitsRun = func(port int, statement string) ([]byte, error) {
		// alpha.149 (finding 9): the session's bounds come first.
		if !strings.HasPrefix(statement, "SET lock_timeout = '5s';\nSET statement_timeout = '15s';\nDO $celikpanel_role_limits$") {
			t.Errorf("the script does not start with its bounds: %q", statement)
		}
		ran = append(ran, statement)
		return run(port, statement)
	}
	postgreSQLExecStatement = func(statement string) ([]byte, error) {
		t.Fatal("a statement went to psql's default cluster, not the role's")
		return nil, nil
	}
	return &ran
}

func roleLimitsCall(t *testing.T, req transport.PostgreSQLRoleLimitsRequest) transport.PostgreSQLRoleLimitsResponse {
	t.Helper()
	var resp transport.PostgreSQLRoleLimitsResponse
	if err := (&Agent{}).SetPostgreSQLRoleLimitsV1(&req, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

const pgRoleLimitsAnswer = " conn=12\n statement_seconds=5s\n idle_in_transaction_seconds=\n temp_file_mb=100MB\n"

// The admission rule before anything reaches the engine: a plain name, never
// an engine or CelikPanel account, only the four settings in their ranges.
func TestPGRoleLimitsRefuseWhatIsNotASiteRoleSetting(t *testing.T) {
	ran := withPGRoleLimitsStubs(t, true, func(int, string) ([]byte, error) { return []byte(pgRoleLimitsAnswer), nil })
	for _, c := range []struct {
		name string
		req  transport.PostgreSQLRoleLimitsRequest
		code string
	}{
		{"postgres", transport.PostgreSQLRoleLimitsRequest{Role: "postgres", Port: 5432}, transport.PGRoleLimitRefusedReserved},
		{"the panel's account", transport.PostgreSQLRoleLimitsRequest{Role: "celikpanel_admin", Port: 5432}, transport.PGRoleLimitRefusedReserved},
		{"a CelikPanel load account", transport.PostgreSQLRoleLimitsRequest{Role: "celikpanel_load_abc", Port: 5432}, transport.PGRoleLimitRefusedReserved},
		{"a predefined role", transport.PostgreSQLRoleLimitsRequest{Role: "pg_read_all_data", Port: 5432}, transport.PGRoleLimitRefusedReserved},
		{"root", transport.PostgreSQLRoleLimitsRequest{Role: "Root", Port: 5432}, transport.PGRoleLimitRefusedReserved},
		{"a quoted name", transport.PostgreSQLRoleLimitsRequest{Role: `a"; DROP ROLE x; --`, Port: 5432}, transport.PGRoleLimitRefusedInvalid},
		{"no port", transport.PostgreSQLRoleLimitsRequest{Role: "shop_u"}, transport.PGRoleLimitRefusedInvalid},
		{"another setting", transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5432, Writes: []transport.PostgreSQLRoleLimitWrite{{Key: "search_path", Value: 1}}}, transport.PGRoleLimitRefusedInvalid},
		{"out of range", transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5432, Writes: []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitStatementSeconds, Value: 86401}}}, transport.PGRoleLimitRefusedInvalid},
		{"negative", transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5432, Writes: []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitConnection, Value: -1}}}, transport.PGRoleLimitRefusedInvalid},
		{"twice", transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5432, Writes: []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitTempFileMB, Value: 1}, {Key: transport.PGRoleLimitTempFileMB, Value: 2}}}, transport.PGRoleLimitRefusedInvalid},
	} {
		if resp := roleLimitsCall(t, c.req); resp.Refused != c.code || resp.Applied {
			t.Errorf("%s: %+v, want refused %s", c.name, resp, c.code)
		}
	}
	if len(*ran) != 0 {
		t.Fatalf("a refused request reached PostgreSQL: %q", *ran)
	}
}

// A role the panel did not record is never touched.
func TestPGRoleLimitsRefuseARoleThePanelDidNotRecord(t *testing.T) {
	ran := withPGRoleLimitsStubs(t, false, func(int, string) ([]byte, error) { return []byte(pgRoleLimitsAnswer), nil })
	resp := roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "owners_app", Port: 5432,
		Writes: []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitTempFileMB, Value: 1}}})
	if resp.Refused != transport.PGRoleLimitRefusedNotRecorded || len(*ran) != 0 {
		t.Fatalf("an unrecorded role: %+v, ran %q", resp, *ran)
	}
	// A record that cannot be read touches nothing either.
	pgRoleRecordedByPanel = func(context.Context, string, int) (bool, error) { return false, errors.New("database is locked") }
	resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5432})
	if resp.Applied || resp.Error == "" || len(*ran) != 0 {
		t.Fatalf("an unreadable record: %+v, ran %q", resp, *ran)
	}
}

// The statement is the Agent's own: the role's name only as a checked
// literal, every identifier quoted by the server, fixed parameters and
// integer values, the checks and the writes in one transaction.
func TestPGRoleLimitsScriptIsBuiltByTheAgent(t *testing.T) {
	script, err := pgRoleLimitsScript("shop_u", []transport.PostgreSQLRoleLimitWrite{
		{Key: transport.PGRoleLimitConnection, Value: 0},
		{Key: transport.PGRoleLimitStatementSeconds, Value: 5},
		{Key: transport.PGRoleLimitIdleInTransactionSeconds, Value: 0},
		{Key: transport.PGRoleLimitTempFileMB, Value: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"WHERE rolname OPERATOR(pg_catalog.=) 'shop_u'",
		// alpha.149 (security): the shared account rule, CREATEDB and the
		// pg_* roles included, and for this call a member of no role.
		"state := " + pgAccountStateExpr("'shop_u'", pgAccountNoMembership) + ";",
		"celikpanel_r.rolcreatedb",
		"pg_catalog.pg_has_role(celikpanel_r.oid, celikpanel_held.oid, 'MEMBER')",
		"pg_catalog.pg_auth_members celikpanel_m",
		"IF state OPERATOR(pg_catalog.<>) 'ok' THEN RAISE EXCEPTION 'celikpanel-refused:%', state; END IF;",
		"EXECUTE pg_catalog.format('ALTER ROLE %I CONNECTION LIMIT -1', target.rolname);",
		"EXECUTE pg_catalog.format('ALTER ROLE %I SET statement_timeout = %L', target.rolname, '5s');",
		"EXECUTE pg_catalog.format('ALTER ROLE %I RESET idle_in_transaction_session_timeout', target.rolname);",
		"EXECUTE pg_catalog.format('ALTER ROLE %I SET temp_file_limit = %L', target.rolname, '100MB');",
		"s.setdatabase OPERATOR(pg_catalog.=) 0",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the script lacks %q:\n%s", want, script)
		}
	}
	if strings.Count(script, "shop_u") != 6 || strings.Contains(script, `"shop_u"`) {
		t.Fatalf("the role's name appears other than as the checked literal:\n%s", script)
	}
	if strings.Index(script, "RAISE EXCEPTION 'celikpanel-refused:%'") > strings.Index(script, "ALTER ROLE") {
		t.Fatal("a write comes before the role's check")
	}
}

// alpha.149 (review of PR #370, finding 2): a role that is a member of any
// role is refused with its own code, nothing changed.
func TestPGRoleLimitsRefuseAMemberOfAnyRole(t *testing.T) {
	withPGRoleLimitsStubs(t, true, func(int, string) ([]byte, error) {
		return []byte("ERROR:  celikpanel-refused:member\nCONTEXT:  PL/pgSQL function inline_code_block line 6 at RAISE\n"), errors.New("exit status 3")
	})
	resp := roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433,
		Writes: []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitStatementSeconds, Value: 5}}})
	if resp.Refused != transport.PGRoleLimitRefusedMember || resp.Applied || resp.Unknown || !strings.Contains(resp.Error, "nothing was changed") {
		t.Fatalf("a member: %+v", resp)
	}
}

// The outcomes the panel tells apart: applied with the values read back, a
// refusal of the engine's own check, an unmet prerequisite, a verified
// failure and an unknown result.
func TestPGRoleLimitsOutcomes(t *testing.T) {
	answer, answerErr := []byte(pgRoleLimitsAnswer), error(nil)
	ran := withPGRoleLimitsStubs(t, true, func(port int, statement string) ([]byte, error) {
		if port != 5433 {
			t.Errorf("the statement went to port %d, not the role's cluster", port)
		}
		return answer, answerErr
	})
	write := []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitStatementSeconds, Value: 5}}
	resp := roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write})
	if !resp.Applied || resp.ConnectionLimit != 12 || resp.Settings[transport.PGRoleLimitStatementSeconds] != "5s" ||
		resp.Settings[transport.PGRoleLimitTempFileMB] != "100MB" || resp.Settings[transport.PGRoleLimitIdleInTransactionSeconds] != "" || len(*ran) != 1 {
		t.Fatalf("applied: %+v", resp)
	}
	answer, answerErr = []byte("ERROR:  celikpanel-refused:privileged\nCONTEXT:  PL/pgSQL function inline_code_block line 7 at RAISE\n"), errors.New("exit status 3")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); resp.Refused != transport.PGRoleLimitRefusedPrivileged || resp.Applied {
		t.Fatalf("a privileged role: %+v", resp)
	}
	answer = []byte("ERROR:  celikpanel-refused:absent\n")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433}); resp.Refused != transport.PGRoleLimitRefusedAbsent {
		t.Fatalf("an absent role: %+v", resp)
	}
	answer = []byte("psql: error: connection to server on socket \"/var/run/postgresql/.s.PGSQL.5433\" failed: No such file or directory\n")
	pgServerRunning = func(int) (bool, bool) { return false, true }
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); !resp.Prerequisite || resp.Connection || resp.Unknown || resp.Applied {
		t.Fatalf("a server that does not run: %+v", resp)
	}
	// alpha.149 (finding 7): a running server that refuses the connection
	// is not "not running", whatever psql's words.
	pgServerRunning = func(int) (bool, bool) { return true, true }
	for _, refused := range []string{
		"psql: error: connection to server on socket \"/var/run/postgresql/.s.PGSQL.5433\" failed: FATAL:  Peer authentication failed for user \"postgres\"\n",
		"psql: error: connection to server on socket \"/var/run/postgresql/.s.PGSQL.5433\" failed: FATAL:  sorry, too many clients already\n",
		"psql: error: connection to server on socket \"/var/run/postgresql/.s.PGSQL.5433\" failed: FATAL:  the database system is starting up\n",
	} {
		answer = []byte(refused)
		if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); !resp.Connection || resp.Prerequisite || resp.Unknown || resp.Applied {
			t.Fatalf("a running server that refused the connection: %+v", resp)
		}
	}
	// A lost answer on a read is unknown, not a refusal or a failure.
	answer = []byte("server closed the connection unexpectedly\n")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433}); !resp.Unknown || resp.Applied || resp.Refused != "" {
		t.Fatalf("a lost answer on a read: %+v", resp)
	}
	// The client ended at its deadline: unknown.
	answer, answerErr = []byte("Cancel request sent\n"), &pgClientEndedError{}
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); !resp.Unknown || resp.Applied {
		t.Fatalf("a client ended at its deadline: %+v", resp)
	}
	answerErr = errors.New("exit status 3")
	// A lock timeout is the engine's refusal of the block: nothing kept.
	answer = []byte("ERROR:  canceling statement due to lock timeout\n")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); resp.Unknown || resp.Applied || resp.Error == "" {
		t.Fatalf("a lock timeout: %+v", resp)
	}
	answer = []byte("ERROR:  permission denied to alter role\n")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); resp.Prerequisite || resp.Unknown || resp.Applied || resp.Error == "" {
		t.Fatalf("a refused statement: %+v", resp)
	}
	answer = []byte("server closed the connection unexpectedly\n")
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); !resp.Unknown || resp.Applied {
		t.Fatalf("a lost answer: %+v", resp)
	}
	answer, answerErr = []byte(" conn=12\n"), nil
	if resp = roleLimitsCall(t, transport.PostgreSQLRoleLimitsRequest{Role: "shop_u", Port: 5433, Writes: write}); !resp.Unknown || resp.Applied {
		t.Fatalf("an answer that cannot be read after a write: %+v", resp)
	}
}

// The panel's record: a PostgreSQL record of this machine with the role, at
// the cluster's port or the cluster it names; never MariaDB's, never another
// machine's.
func TestPGRoleRecordedByThePanelsOwnDatabase(t *testing.T) {
	restoreOwners := panelDatabaseOwnerUIDs
	t.Cleanup(func() { panelDatabaseOwnerUIDs = restoreOwners })
	panelDatabaseOwnerUIDs = func() []uint32 { return []uint32{uint32(os.Geteuid())} }
	path := filepath.Join(t.TempDir(), "celikpanel.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE database_server_types (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE database_servers (id INTEGER PRIMARY KEY, type_id INTEGER, host TEXT, port INTEGER, cluster TEXT)`,
		`CREATE TABLE database_users (id INTEGER PRIMARY KEY, server_id INTEGER, username TEXT)`,
		`INSERT INTO database_server_types VALUES (1, 'mariadb'), (2, 'postgresql')`,
		`INSERT INTO database_servers VALUES (1, 2, 'localhost', 5432, ''), (2, 2, '127.0.0.1', 5432, '17/main'), (3, 1, 'localhost', 5432, ''), (4, 2, '203.0.113.5', 5433, '')`,
		`INSERT INTO database_users VALUES (1, 1, 'shop_u'), (2, 2, 'blog_u'), (3, 3, 'maria_u'), (4, 4, 'far_u')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	clusters := func() ([]transport.PostgreSQLCluster, bool) {
		return []transport.PostgreSQLCluster{{Version: "17", Name: "main", Port: 5434}}, true
	}
	ctx := context.Background()
	for _, c := range []struct {
		role string
		port int
		want bool
	}{
		{"shop_u", 5432, true},
		{"shop_u", 5433, false}, // another cluster's role of the same name
		{"blog_u", 5434, true},  // the cluster the record names, at its own port
		{"blog_u", 5433, false},
		{"maria_u", 5432, false}, // a MariaDB user
		{"far_u", 5433, false},   // another machine's PostgreSQL
		{"owners_app", 5432, false},
	} {
		got, err := pgRoleRecordedIn(ctx, path, c.role, c.port, clusters)
		if err != nil || got != c.want {
			t.Errorf("%s at %d: %v (%v), want %v", c.role, c.port, got, err, c.want)
		}
	}
	if _, err := pgRoleRecordedIn(ctx, filepath.Join(t.TempDir(), "missing.db"), "shop_u", 5432, clusters); err == nil {
		t.Fatal("a missing panel database was read as no record")
	}
	// One read at a time, but a read that has answered never holds the next
	// one (the slot goes before the answer).
	for i := 0; i < 200; i++ {
		if _, err := pgRoleRecordedIn(ctx, path, "shop_u", 5432, clusters); err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
	}

	// alpha.149 (review of PR #370, finding 2): read as root, the file goes
	// through the managed-file checks - a symbolic link, a FIFO or a file of
	// an unexpected owner is an error (nothing is touched), and a FIFO never
	// holds the call.
	dir := t.TempDir()
	link := filepath.Join(dir, "celikpanel.db")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := pgRoleRecordedIn(ctx, link, "shop_u", 5432, clusters); err == nil {
		t.Fatal("a symbolic link was followed")
	}
	fifo := filepath.Join(t.TempDir(), "celikpanel.db")
	if err := syscallMkfifo(fifo); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	started := time.Now()
	if _, err := pgRoleRecordedIn(ctx, fifo, "shop_u", 5432, clusters); err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("a FIFO: %v after %s", err, time.Since(started))
	}
	panelDatabaseOwnerUIDs = func() []uint32 { return []uint32{uint32(os.Geteuid()) + 1} }
	if _, err := pgRoleRecordedIn(ctx, path, "shop_u", 5432, clusters); err == nil || !strings.Contains(err.Error(), "unexpected account") {
		t.Fatalf("a file of another owner: %v", err)
	}
}
