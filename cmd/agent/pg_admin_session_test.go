package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// S1 (20 Nov 2026), finding 8: every session the engine's administrator
// opens starts with the fixed search path. This guard fails when a
// PostgreSQL client is started some other way: a new superuser session must
// go through pgAdminSudoArgs/pgAdminPSQLArgs (as postgres, through sudo) or
// pgCommandAs (as a cluster's owner), which both carry it.
func TestEveryPostgreSQLAdministratorSessionHasTheFixedSearchPath(t *testing.T) {
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(self), "..", "..")
	client := regexp.MustCompile(`"(psql|pg_dump|pg_dumpall|pg_restore)"`)
	asPostgres := regexp.MustCompile(`"-u",\s*"postgres"`)
	allowed := []string{"pgAdminSudoArgs(", "pgAdminPSQLArgs(", "pgAdminPSQLArgsFor(", "pgClientBinary(", "host.stream(", `exec.LookPath("psql")`, `exec.Command("which", "psql")`}
	for _, dir := range []string{"cmd/agent", "cmd/panel", "cmd/recovery", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			name := filepath.Base(path)
			for number, line := range strings.Split(string(data), "\n") {
				where := filepath.ToSlash(path[len(root)+1:]) + ":" + strconv.Itoa(number+1)
				if asPostgres.MatchString(line) && name != "pg_admin_session.go" && !strings.Contains(line, `"runuser", "-u", "postgres", "--",`) {
					t.Errorf("%s starts a program as postgres outside pg_admin_session.go: %s", where, strings.TrimSpace(line))
				}
				if !client.MatchString(line) || name == "pg_admin_session.go" {
					continue
				}
				ok := false
				for _, form := range allowed {
					ok = ok || strings.Contains(line, form)
				}
				if !ok {
					t.Errorf("%s starts a PostgreSQL client without the fixed search path: %s", where, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// pgCommandAs runs every client as a cluster's owner: it must carry it.
	data, err := os.ReadFile(filepath.Join(root, "cmd/agent/pg_exec_linux.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	start := strings.Index(body, "func pgCommandAs(")
	if start < 0 || !strings.Contains(body[start:start+min(len(body)-start, 2000)], "pgAdminSessionEnv") {
		t.Error("pgCommandAs does not give its client the fixed search path")
	}
}

// The command lines themselves: the statement runner, the backup's dump and
// the database tools' query.
func TestAdministratorCommandLinesStartWithTheFixedSearchPath(t *testing.T) {
	want := pgAdminSessionEnv
	statement := newPostgreSQLStatementCommandOn(5433, "SELECT 1;")
	args := strings.Join(statement.Args, " ")
	if !strings.Contains(args, "-u postgres env "+want+" psql --no-psqlrc --set ON_ERROR_STOP=on --port 5433") {
		t.Fatalf("statement command = %v", statement.Args)
	}
	dump, err := databaseDumpCommand(backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql", ClusterPort: 5433})
	if err != nil || !strings.HasPrefix(strings.Join(dump.Args, " "), "-u postgres env "+want+" pg_dump ") {
		t.Fatalf("dump = %v %v", dump.Args, err)
	}
	cluster := pgAdminPSQLArgs("18/main", "--quiet")
	if strings.Join(cluster, " ") != "-u postgres env "+want+" psql --cluster 18/main --no-psqlrc --set ON_ERROR_STOP=on --quiet" {
		t.Fatalf("cluster query = %v", cluster)
	}
}

// Second review of alpha.143, B5: the administrator session pins what a
// database's owner could otherwise set for it (ALTER DATABASE ... SET).
func TestAdministratorSessionPinsWhatADatabaseOwnerCouldSet(t *testing.T) {
	for _, pin := range []string{"search_path=pg_catalog,pg_temp", "statement_timeout=0", "lock_timeout=0", "idle_in_transaction_session_timeout=0",
		"default_transaction_read_only=off", "role=none", "exit_on_error=off"} {
		if !strings.Contains(pgAdminSessionEnv, "-c "+pin) {
			t.Errorf("the administrator session does not pin %s: %s", pin, pgAdminSessionEnv)
		}
	}
	named := strings.Join(pgAdminPSQLArgsFor("celikpanel-x", "--port", "5433"), " ")
	if !strings.HasPrefix(named, "-u postgres env "+pgAdminSessionEnv+" PGAPPNAME=celikpanel-x PGCONNECT_TIMEOUT=10 psql --no-psqlrc --set ON_ERROR_STOP=on --port 5433") {
		t.Fatalf("named client = %s", named)
	}
}
