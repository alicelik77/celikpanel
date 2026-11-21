package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// S4 R2a: a database user's time defaults never cut the panel's own loads,
// and its temporary-file limit is never lifted (review findings 1 and 3).

// pgTempLimitConn is a load session whose statement containing failOn stops
// at the user's temporary-file limit, as PostgreSQL answers it.
type pgTempLimitConn struct {
	*pgFakeLoadConn
	failOn string
}

func (c *pgTempLimitConn) Exec(ctx context.Context, sql string) error {
	c.pgFakeLoadConn.sent = append(c.pgFakeLoadConn.sent, strings.TrimSpace(sql))
	if c.failOn != "" && strings.Contains(sql, c.failOn) {
		return &pgconn.PgError{Severity: "ERROR", Code: "53400", Message: "temporary file size exceeds temp_file_limit (1024kB)"}
	}
	return nil
}

func useTempLimitConn(t *testing.T, conn pgLoadConn) {
	t.Helper()
	old := connectPGForLoad
	connectPGForLoad = func(ctx context.Context, port int, database, user, password string) (pgLoadConn, error) {
		return conn, nil
	}
	t.Cleanup(func() { connectPGForLoad = old })
}

// TestAPostgreSQLLoadNeverLiftsTheTemporaryFileLimit: every load path (a
// restore, the copies and the cPanel import go through loadDatabaseSQL) sets
// the two user-settable timeouts to 0 for its own session and sends nothing
// as the engine's administrator: the role's temp_file_limit holds for the
// load and for every other session the role opens meanwhile. A load the
// limit stops is rolled back and names the limit and who raises it.
func TestAPostgreSQLLoadNeverLiftsTheTemporaryFileLimit(t *testing.T) {
	old := postgreSQLExecStatementOnPort
	t.Cleanup(func() { postgreSQLExecStatementOnPort = old })
	var admin []string
	postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
		admin = append(admin, statement)
		return nil, nil
	}
	target := databaseLoadTarget{
		DatabaseIdentity: backupspec.DatabaseIdentity{ID: 2, Name: "shop", Type: "postgresql", ClusterPort: 5433},
		Account:          databaseLoadAccount{User: "shop_user", Password: "pw"},
	}
	ok := &pgTempLimitConn{pgFakeLoadConn: &pgFakeLoadConn{superuser: "off", rows: [][]string{{"1MB"}}}}
	useTempLimitConn(t, ok)
	if err := loadDatabaseSQL(context.Background(), target, strings.NewReader("CREATE TABLE public.t (id int);\n"), true); err != nil {
		t.Fatal(err)
	}
	want := []string{"SET statement_timeout = 0", "SET idle_in_transaction_session_timeout = 0", "BEGIN", "DROP OWNED BY CURRENT_USER",
		"CREATE TABLE public.t (id int);", "COMMIT"}
	if !reflect.DeepEqual(ok.sent, want) {
		t.Fatalf("the load sent %q, want %q", ok.sent, want)
	}
	stopped := &pgTempLimitConn{pgFakeLoadConn: &pgFakeLoadConn{superuser: "off", rows: [][]string{{"1MB"}}}, failOn: "CREATE INDEX"}
	useTempLimitConn(t, stopped)
	err := loadDatabaseSQL(context.Background(), target, strings.NewReader("CREATE TABLE public.t (id int);\nCREATE INDEX i ON public.t (id);\n"), true)
	if err == nil || !strings.Contains(err.Error(), "Temporary files of one database session") || !strings.Contains(err.Error(), "(1MB)") ||
		!strings.Contains(err.Error(), "Nothing was changed") || !strings.Contains(err.Error(), "reseller") {
		t.Fatalf("a load stopped at the limit: %v", err)
	}
	if last := stopped.sent[len(stopped.sent)-1]; last != "ROLLBACK" {
		t.Fatalf("the stopped load was not rolled back: %q", stopped.sent)
	}
	if len(admin) != 0 {
		t.Fatalf("a load sent statements as the engine's administrator: %q", admin)
	}
}

// TestTheCustomersPostgreSQLImportFollowsTheSameRule: the customer's own
// import (database tools) lifts the two timeouts for its session as every
// other load does, keeps the temporary-file limit, and says which limit
// stopped it.
func TestTheCustomersPostgreSQLImportFollowsTheSameRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(path, []byte("CREATE TABLE public.t (id int);\nCREATE INDEX i ON public.t (id);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database := backupspec.DatabaseIdentity{ID: 2, Name: "shop", Type: "postgresql", ClusterPort: 5433}
	req := transport.DBToolsImportRequest{User: "shop_user", Password: "pw"}

	ok := &pgTempLimitConn{pgFakeLoadConn: &pgFakeLoadConn{superuser: "off", rows: [][]string{{"256MB"}}}}
	useTempLimitConn(t, ok)
	var resp transport.DBToolsImportResponse
	importPostgreSQL(context.Background(), database, req, path, &resp)
	if !resp.Success || len(ok.sent) < 3 || ok.sent[0] != "SET statement_timeout = 0" || ok.sent[1] != "SET idle_in_transaction_session_timeout = 0" {
		t.Fatalf("import: %+v sent %q", resp, ok.sent)
	}
	stopped := &pgTempLimitConn{pgFakeLoadConn: &pgFakeLoadConn{superuser: "off", rows: [][]string{{"256MB"}}}, failOn: "CREATE INDEX"}
	useTempLimitConn(t, stopped)
	resp = transport.DBToolsImportResponse{}
	importPostgreSQL(context.Background(), database, req, path, &resp)
	if resp.Success || !resp.RolledBack || !strings.Contains(resp.Error, "Temporary files of one database session") || !strings.Contains(resp.Error, "(256MB)") {
		t.Fatalf("import stopped at the limit: %+v", resp)
	}
}

// TestNoCodeLiftsATemporaryFileLimit: no statement in the Agent, the Panel or
// the engine drivers ever raises temp_file_limit for a role (as a whole or
// in one database) or resets role-in-database settings in bulk, so neither a
// finished nor a killed Agent can leave one lifted.
func TestNoCodeLiftsATemporaryFileLimit(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)temp_file_limit\s*=\s*-1|IN DATABASE[^"\n]*temp_file_limit|ResetLoadOverrides`)
	for _, dir := range []string{".", "../panel", "../../internal/services"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if m := forbidden.Find(raw); m != nil {
				t.Errorf("%s/%s: %q", dir, e.Name(), m)
			}
		}
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
