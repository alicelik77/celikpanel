package main

import (
	"errors"
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
	// A site role's database: what the user made goes to that owner, tested
	// again in the REASSIGN's own transaction. (celikpanel_admin's database:
	// TestRemovingAUserNeverHandsAnythingToCelikPanelsOwnAccount.)
	e, resp = remove("app", "app_user", nil)
	var reassign string
	for _, statement := range e.statements {
		if strings.Contains(statement, "REASSIGN OWNED BY") {
			reassign = statement
		}
	}
	at := strings.Index(reassign, "celikpanel_r.rolname OPERATOR(pg_catalog.=) 'app_user'")
	if !resp.Applied || at < 0 || at > strings.Index(reassign, "REASSIGN OWNED BY") || !strings.Contains(reassign, "TO \"app_user\"") ||
		strings.Contains(reassign, "$celikpanel_owned_guard$") {
		t.Fatalf("owner app_user: %+v %q", resp, e.statements)
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

// Third review of PR #370, H1 (security): removing a user added to a
// database owned by celikpanel_admin never hands anything to celikpanel_admin.
// No REASSIGN OWNED is sent at all; in the same transaction, before DROP
// OWNED, a user that owns anything there is refused (owns_objects, naming the
// user), and one that owns nothing is removed as before.
func TestRemovingAUserNeverHandsAnythingToCelikPanelsOwnAccount(t *testing.T) {
	remove := func(ownsObjects bool) (*namedGuardEngine, transport.DatabaseAccessUserResponse) {
		answers := pgAccessAnswers(true, databaseAdminAccountName)
		e := &namedGuardEngine{other: func(statement string) ([]byte, error) {
			// The engine: the user owns a function in the database, so the
			// owned-objects guard raises (psql's own error line, then stop).
			if ownsObjects && strings.Contains(statement, "$celikpanel_owned_guard$") {
				return []byte("You are now connected to database \"app\" as user \"postgres\".\nBEGIN\nDO\nDO\n" +
					"ERROR:  celikpanel-refused:owns_objects:app_x\nCONTEXT:  PL/pgSQL function inline_code_block line 3 at RAISE\n"), errors.New("exit status 3")
			}
			return answers(statement)
		}}
		installNamedGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		if err := (&Agent{}).RemoveDatabaseAccessUserV1(&transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "app", Username: "app_x"}, &resp); err != nil {
			t.Fatal(err)
		}
		return e, resp
	}
	scriptOf := func(e *namedGuardEngine) string {
		for _, statement := range e.statements {
			if strings.Contains(statement, "DROP OWNED BY") {
				return statement
			}
		}
		return ""
	}

	// The user owns a function: refused, nothing handed over, the role stays.
	e, resp := remove(true)
	script := scriptOf(e)
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedOwnsObjects || resp.AccountRefusedName != "app_x" ||
		!strings.Contains(resp.Error, "app_x owns tables or other objects") || strings.Contains(e.all(), "REASSIGN") || strings.Contains(e.all(), "DROP ROLE") {
		t.Fatalf("owns a function: %+v %q", resp, e.statements)
	}
	// The guard runs inside the transaction, after the user's and the owner's
	// guards and before DROP OWNED.
	guard := strings.Index(script, "$celikpanel_owned_guard$")
	if guard < 0 || guard < strings.Index(script, "BEGIN;") || guard < strings.Index(script, "celikpanel_r.rolname OPERATOR(pg_catalog.=) '"+databaseAdminAccountName+"'") ||
		guard > strings.Index(script, "DROP OWNED BY") || !strings.HasSuffix(script, "COMMIT;") {
		t.Fatalf("the owned-objects guard is not in the removal's transaction before DROP OWNED: %q", script)
	}

	// The user owns nothing: removed, still no REASSIGN sent.
	e, resp = remove(false)
	script = scriptOf(e)
	if !resp.Applied || resp.AccountRefused != "" || strings.Contains(e.all(), "REASSIGN") || !strings.Contains(script, "$celikpanel_owned_guard$") ||
		!strings.Contains(e.all(), "DROP ROLE \"app_x\"") {
		t.Fatalf("owns nothing: %+v %q", resp, e.statements)
	}
}

// The owned-objects guard reads exactly what REASSIGN OWNED would move: the
// user's owner rows in pg_shdepend for this database and for shared objects,
// leaving out default-ACL entries and user mappings (REASSIGN OWNED skips
// them; DROP OWNED removes them).
func TestTheOwnedObjectsGuardReadsWhatReassignWouldMove(t *testing.T) {
	block, err := pgOwnsObjectsGuardBlock("app_x")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"FROM pg_catalog.pg_shdepend celikpanel_d",
		"celikpanel_d.refclassid OPERATOR(pg_catalog.=) 'pg_catalog.pg_authid'::pg_catalog.regclass::pg_catalog.oid",
		"WHERE celikpanel_r.rolname OPERATOR(pg_catalog.=) 'app_x')",
		"celikpanel_d.deptype OPERATOR(pg_catalog.=) 'o'",
		"celikpanel_d.dbid OPERATOR(pg_catalog.=) 0::pg_catalog.oid",
		"celikpanel_db.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()",
		"celikpanel_d.classid OPERATOR(pg_catalog.<>) 'pg_catalog.pg_default_acl'::pg_catalog.regclass::pg_catalog.oid",
		"celikpanel_d.classid OPERATOR(pg_catalog.<>) 'pg_catalog.pg_user_mapping'::pg_catalog.regclass::pg_catalog.oid",
		"RAISE EXCEPTION 'celikpanel-refused:owns_objects:%', 'app_x';",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the owned-objects guard lacks %q: %s", want, block)
		}
	}
	for _, hostile := range []string{"x'; DROP ROLE postgres; --", `a"b`, ""} {
		if _, err := pgOwnsObjectsGuardBlock(hostile); err == nil {
			t.Errorf("the owned-objects guard accepted %q", hostile)
		}
	}
}

func TestTheOwnerRuleOfCelikPanelsOwnAccount(t *testing.T) {
	admin := pgDatabaseOwnerStateExpr("'celikpanel_admin'", true)
	// Third review of PR #370, I2: its own REPLICATION or BYPASSRLS too.
	for _, want := range []string{"celikpanel_r.rolsuper", "celikpanel_r.rolreplication", "celikpanel_r.rolbypassrls", "celikpanel_held.rolsuper", "pg_catalog.starts_with(celikpanel_held.rolname::pg_catalog.text, 'pg_')",
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

// Second review of PR #370, finding 9: a refusal names the account it is
// about - the refused writer or reader, the database's owner, the role a
// write-time guard names - never the new user on a create.
func TestARefusalNamesTheRefusedAccount(t *testing.T) {
	answers := func(grants func(string) ([]byte, error)) func(string) ([]byte, error) {
		return func(statement string) ([]byte, error) {
			switch {
			case strings.HasPrefix(statement, "SELECT 'roles='"):
				return []byte("roles=0\nowner=app_user\n"), nil
			case strings.HasPrefix(statement, "SELECT 'role='"):
				return []byte("role=boss\n"), nil
			case strings.Contains(statement, "ALTER DEFAULT PRIVILEGES") && grants != nil:
				return grants(statement)
			}
			return nil, nil
		}
	}
	create := func(states map[string]string, grants func(string) ([]byte, error)) transport.DatabaseAccessUserResponse {
		e := &namedGuardEngine{states: states, other: answers(grants)}
		installNamedGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		req := &transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "app", Username: "app_x", Password: "Zq7xN2kLp9Vt4RmW",
			Access: transport.DatabaseAccessWrite, Writers: []string{"boss"}}
		if err := (&Agent{}).CreateDatabaseAccessUserV1(req, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	for states, want := range map[string]map[string]string{"boss": {"boss": "privileged"}, "app_user": {"app_user": "privileged"}} {
		resp := create(want, nil)
		if resp.AccountRefusedName != states || !strings.Contains(resp.Error, states) || strings.Contains(resp.Error, "app_x") {
			t.Fatalf("read-time refusal of %s: %+v", states, resp)
		}
	}
	// The writer became privileged after the read: the guard in the grants'
	// transaction names it.
	resp := create(nil, func(string) ([]byte, error) {
		return []byte("ERROR:  celikpanel-refused:privileged:boss\nCONTEXT:  PL/pgSQL function inline_code_block line 5 at RAISE\n"), errors.New("exit status 3")
	})
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || resp.AccountRefusedName != "boss" ||
		!strings.Contains(resp.Error, "boss") || strings.Contains(resp.Error, "app_x holds") {
		t.Fatalf("write-time refusal: %+v", resp)
	}
	// A removal names the user itself.
	e := &namedGuardEngine{states: map[string]string{"app_x": "privileged"}, other: pgAccessAnswers(true, "app_user")}
	installNamedGuardEngine(t, e)
	var removed transport.DatabaseAccessUserResponse
	if err := (&Agent{}).RemoveDatabaseAccessUserV1(&transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "app", Username: "app_x"}, &removed); err != nil {
		t.Fatal(err)
	}
	if removed.AccountRefusedName != "app_x" {
		t.Fatalf("removal: %+v", removed)
	}
	if got := databaseAccountRefusedNameIn([]byte("ERROR:  celikpanel-refused:member:shop_u\n")); got != "shop_u" {
		t.Fatalf("name read from a refusal: %q", got)
	}
}
