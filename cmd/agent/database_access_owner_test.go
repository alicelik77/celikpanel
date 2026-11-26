package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 security fix (second review of PR #370, finding 1): the added-user
// calls act on the database's owner too (ALTER DEFAULT PRIVILEGES FOR ROLE
// <owner>, REASSIGN OWNED ... TO <owner>), so the owner read from the engine
// must pass the shared rule; the one exception is celikpanel_admin while it
// is not a superuser and a member of no pg_* role.

var guardedNameRe = regexp.MustCompile(`(?:celikpanel_r\.rolname OPERATOR\(pg_catalog\.=\)|User =) '([A-Za-z0-9_]+)'`)

// namedGuardEngine answers each guard read by the name it tests (states;
// "ok" for a name not listed) and every other statement with other.
type namedGuardEngine struct {
	states     map[string]string
	statements []string
	other      func(statement string) ([]byte, error)
}

func (e *namedGuardEngine) run(statement string) ([]byte, error) {
	e.statements = append(e.statements, statement)
	if isAccountGuardRead(statement) {
		state := "ok"
		if match := guardedNameRe.FindStringSubmatch(statement); match != nil {
			if s, ok := e.states[match[1]]; ok {
				state = s
			}
		}
		return []byte("guard=" + state + "\n"), nil
	}
	if e.other != nil {
		return e.other(statement)
	}
	return nil, nil
}

func (e *namedGuardEngine) all() string { return strings.Join(e.statements, "\n") }

func installNamedGuardEngine(t *testing.T, e *namedGuardEngine) {
	t.Helper()
	oldMySQL, oldPG, oldPort := mysqlExecStatement, postgreSQLExecStatement, postgreSQLExecStatementOnPort
	mysqlExecStatement, postgreSQLExecStatement = e.run, e.run
	postgreSQLExecStatementOnPort = func(_ int, statement string) ([]byte, error) { return e.run(statement) }
	t.Cleanup(func() {
		mysqlExecStatement, postgreSQLExecStatement, postgreSQLExecStatementOnPort = oldMySQL, oldPG, oldPort
	})
}

// pgAccessAnswers is a PostgreSQL engine with the user (present or not) and a
// database owned by owner.
func pgAccessAnswers(userPresent bool, owner string) func(string) ([]byte, error) {
	roles := "0"
	if userPresent {
		roles = "1"
	}
	return func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=" + roles + "\nowner=" + owner + "\n"), nil
		case strings.HasPrefix(statement, "SELECT 'deps='"):
			return []byte("deps=0\n"), nil
		}
		return nil, nil
	}
}

func TestAddingAUserRefusesADatabaseOwnedByAPrivilegedRole(t *testing.T) {
	create := func(owner string, states map[string]string) (*namedGuardEngine, transport.DatabaseAccessUserResponse) {
		e := &namedGuardEngine{states: states, other: pgAccessAnswers(false, owner)}
		installNamedGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		req := &transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "app", Username: "app_x", Password: "Zq7xN2kLp9Vt4RmW", Access: transport.DatabaseAccessWrite}
		if err := (&Agent{}).CreateDatabaseAccessUserV1(req, &resp); err != nil {
			t.Fatal(err)
		}
		return e, resp
	}
	// A superuser-owned database (the server owner's own `createdb app`):
	// refused, nothing made.
	e, resp := create("postgres", map[string]string{"postgres": "privileged"})
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || strings.Contains(e.all(), "CREATE ROLE") ||
		strings.Contains(e.all(), "ALTER DEFAULT PRIVILEGES") || !strings.Contains(resp.Error, "postgres") {
		t.Fatalf("superuser owner: %+v %q", resp, e.statements)
	}
	// celikpanel_admin made a superuser (or a member of a pg_* role): refused.
	e, resp = create(databaseAdminAccountName, map[string]string{databaseAdminAccountName: "privileged"})
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || strings.Contains(e.all(), "CREATE ROLE") {
		t.Fatalf("privileged celikpanel_admin: %+v %q", resp, e.statements)
	}
	// A site role's database and celikpanel_admin's: accepted, and the owner
	// is tested again in the grants' own transaction.
	for _, owner := range []string{"app_user", databaseAdminAccountName} {
		e, resp = create(owner, nil)
		grants := e.statements[len(e.statements)-1]
		if !resp.Applied || resp.AccountRefused != "" || !strings.Contains(grants, "ALTER DEFAULT PRIVILEGES FOR ROLE \""+owner+"\"") ||
			strings.Index(grants, "celikpanel_r.rolname OPERATOR(pg_catalog.=) '"+owner+"'") < 0 ||
			strings.Index(grants, "celikpanel_r.rolname OPERATOR(pg_catalog.=) '"+owner+"'") > strings.Index(grants, "GRANT CONNECT") {
			t.Fatalf("owner %s: %+v %q", owner, resp, grants)
		}
	}
}

func TestRemovingAUserNeverHandsItsObjectsToAPrivilegedOwner(t *testing.T) {
	remove := func(database, owner string, states map[string]string) (*namedGuardEngine, transport.DatabaseAccessUserResponse) {
		e := &namedGuardEngine{states: states, other: pgAccessAnswers(true, owner)}
		installNamedGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		if err := (&Agent{}).RemoveDatabaseAccessUserV1(&transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: database, Username: "app_x"}, &resp); err != nil {
			t.Fatal(err)
		}
		return e, resp
	}
	e, resp := remove("app", "postgres", map[string]string{"postgres": "privileged"})
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged ||
		strings.Contains(e.all(), "REASSIGN") || strings.Contains(e.all(), "DROP ") {
		t.Fatalf("superuser owner: %+v %q", resp, e.statements)
	}
	e, resp = remove("app", databaseAdminAccountName, map[string]string{databaseAdminAccountName: "privileged"})
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || strings.Contains(e.all(), "REASSIGN") {
		t.Fatalf("privileged celikpanel_admin: %+v %q", resp, e.statements)
	}
	for _, owner := range []string{"app_user", databaseAdminAccountName} {
		e, resp = remove("app", owner, nil)
		var reassign string
		for _, statement := range e.statements {
			if strings.Contains(statement, "REASSIGN OWNED BY") {
				reassign = statement
			}
		}
		at := strings.Index(reassign, "celikpanel_r.rolname OPERATOR(pg_catalog.=) '"+owner+"'")
		if !resp.Applied || at < 0 || at > strings.Index(reassign, "REASSIGN OWNED BY") || !strings.Contains(reassign, "TO \""+owner+"\"") {
			t.Fatalf("owner %s: %+v %q", owner, resp, e.statements)
		}
	}
	// The engine's own system database is refused before anything runs.
	for engine, database := range map[string]string{"postgresql": "postgres", "mysql": "mysql"} {
		e := &namedGuardEngine{other: pgAccessAnswers(true, "app_user")}
		installNamedGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		if err := (&Agent{}).RemoveDatabaseAccessUserV1(&transport.DatabaseAccessUserRequest{Engine: engine, Database: database, Username: "app_x"}, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedSystemDatabase || len(e.statements) != 0 {
			t.Fatalf("%s system database: %+v %q", engine, resp, e.statements)
		}
	}
}

func TestTheOwnerRuleOfCelikPanelsOwnAccount(t *testing.T) {
	admin := pgDatabaseOwnerStateExpr("'celikpanel_admin'", true)
	for _, want := range []string{"celikpanel_r.rolsuper", "celikpanel_held.rolsuper", "pg_catalog.starts_with(celikpanel_held.rolname::pg_catalog.text, 'pg_')",
		"'pg_database_owner'", "pg_catalog.pg_has_role(celikpanel_r.oid, celikpanel_held.oid, 'MEMBER')", "'absent'"} {
		if !strings.Contains(admin, want) {
			t.Errorf("the celikpanel_admin rule lacks %q", want)
		}
	}
	// It holds CREATEDB and CREATEROLE by design: those are not its test.
	if strings.Contains(admin, "rolcreatedb") || strings.Contains(admin, "rolcreaterole") {
		t.Errorf("the celikpanel_admin rule refuses its own attributes: %s", admin)
	}
	// Any other owner, whatever it is called, gets the shared rule.
	if expr, err := pgDatabaseOwnerExpr("celikpanel_admin2"); err != nil || expr != pgAccountStateExpr("'celikpanel_admin2'", pgAccountOrdinary) {
		t.Errorf("another owner: %q %v", expr, err)
	}
	if _, err := pgDatabaseOwnerExpr("Owner-With-Dash"); err == nil {
		t.Error("an owner name CelikPanel cannot test was accepted")
	}
}
