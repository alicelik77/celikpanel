package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 security fix (review of PR #370, finding 1): no Agent database
// call changes a privileged account on the panel's word. Per call and per
// engine: an administrator account is refused with nothing changed, an
// ordinary site account is still served, and each write carries the guard
// in its own transaction or compound statement.

// ordinaryAccountGuard answers the account rule's read for test engines
// that serve an ordinary site account.
func ordinaryAccountGuard(statement string) ([]byte, bool) {
	if isAccountGuardRead(statement) {
		return []byte("guard=ok\n"), true
	}
	return nil, false
}

func isAccountGuardRead(statement string) bool {
	return strings.HasPrefix(statement, "SELECT 'guard=' || ") || strings.HasPrefix(statement, "SELECT CONCAT('guard=', ")
}

// guardEngine answers the rule's read with state ("" = unreadable) and every
// other statement with other (nil: success, no output).
type guardEngine struct {
	state      string
	statements []string
	other      func(statement string) ([]byte, error)
}

func (e *guardEngine) run(statement string) ([]byte, error) {
	e.statements = append(e.statements, statement)
	if isAccountGuardRead(statement) {
		if e.state == "" {
			return []byte("ERROR 1146 (42S02): Table 'mysql.global_priv' doesn't exist"), errors.New("exit status 1")
		}
		return []byte("guard=" + e.state + "\n"), nil
	}
	if e.other != nil {
		return e.other(statement)
	}
	return nil, nil
}

func (e *guardEngine) writes() []string {
	var out []string
	for _, s := range e.statements {
		if !isAccountGuardRead(s) {
			out = append(out, s)
		}
	}
	return out
}

func installGuardEngine(t *testing.T, e *guardEngine) {
	t.Helper()
	oldMySQL, oldPG, oldPort := mysqlExecStatement, postgreSQLExecStatement, postgreSQLExecStatementOnPort
	mysqlExecStatement, postgreSQLExecStatement = e.run, e.run
	postgreSQLExecStatementOnPort = func(_ int, statement string) ([]byte, error) { return e.run(statement) }
	t.Cleanup(func() {
		mysqlExecStatement, postgreSQLExecStatement, postgreSQLExecStatementOnPort = oldMySQL, oldPG, oldPort
	})
}

func TestTheAccountRuleCoversEveryPrivilege(t *testing.T) {
	pg := pgAccountPrivilegedExpr("r")
	for _, want := range []string{"r.rolsuper", "r.rolcreaterole", "r.rolcreatedb", "r.rolreplication", "r.rolbypassrls",
		"celikpanel_held.rolcreatedb", "pg_catalog.pg_has_role(r.oid, celikpanel_held.oid, 'MEMBER')",
		"pg_catalog.starts_with(celikpanel_held.rolname::pg_catalog.text, 'pg_')", "'pg_database_owner'"} {
		if !strings.Contains(pg, want) {
			t.Errorf("the PostgreSQL rule lacks %q: %s", want, pg)
		}
	}
	if strings.Contains(pgAccountStateExpr("'u'", pgAccountOrdinary), "pg_auth_members") ||
		!strings.Contains(pgAccountStateExpr("'u'", pgAccountNoMembership), "pg_catalog.pg_auth_members celikpanel_m WHERE celikpanel_m.member OPERATOR(pg_catalog.=) celikpanel_r.oid) THEN 'member'") {
		t.Error("the limits call's no-membership rule is not where it belongs")
	}
	my := mysqlAccountPrivilegedExpr("'u'")
	for _, want := range []string{"mysql.global_priv WHERE User = 'u' AND IFNULL(JSON_VALUE(Priv, '$.access'), 'unknown') <> '0'",
		"mysql.roles_mapping WHERE User = 'u'", "mysql.proxies_priv WHERE User = 'u'", "mysql.db WHERE User = 'u' AND 'mysql' LIKE Db",
		"mysql.tables_priv WHERE User = 'u' AND Db = 'mysql'"} {
		if !strings.Contains(my, want) {
			t.Errorf("the MariaDB rule lacks %q: %s", want, my)
		}
	}
	block, err := pgAccountGuardBlock("shop_user", pgAccountOrdinary)
	if err != nil || !strings.Contains(block, "RAISE EXCEPTION 'celikpanel-refused:%', state") {
		t.Fatalf("guard block = %q, %v", block, err)
	}
	compound, err := mysqlGuarded("shop_user", false, "ALTER USER 'shop_user'@'localhost' ACCOUNT LOCK;")
	if err != nil || !strings.HasPrefix(compound, "DELIMITER ;;\nBEGIN NOT ATOMIC\n") || !strings.HasSuffix(compound, "END;;\nDELIMITER ;") ||
		strings.Index(compound, "celikpanel-refused:privileged") > strings.Index(compound, "ACCOUNT LOCK") {
		t.Fatalf("compound = %q, %v", compound, err)
	}
	for _, hostile := range []string{"x'; DROP USER root; --", `a"b`, ""} {
		if _, err := pgGuarded(hostile, pgAccountOrdinary, "SELECT 1;"); err == nil {
			t.Fatalf("pg guard accepted %q", hostile)
		}
		if _, err := mysqlGuarded(hostile, false, "SELECT 1;"); err == nil {
			t.Fatalf("mysql guard accepted %q", hostile)
		}
	}
}

func TestAnUnreadableAccountStateRefuses(t *testing.T) {
	for _, engine := range []string{"mysql", "postgresql"} {
		e := &guardEngine{}
		err := readDatabaseAccountState(engine, e.run, "shop_user", pgAccountOrdinary, false)
		if databaseAccountRefusalOf(err) != transport.DatabaseAccountRefusedUnreadable {
			t.Fatalf("%s: unreadable = %v", engine, err)
		}
		e = &guardEngine{state: "maybe"}
		if err := readDatabaseAccountState(engine, e.run, "shop_user", pgAccountOrdinary, false); databaseAccountRefusalOf(err) != transport.DatabaseAccountRefusedUnreadable {
			t.Fatalf("%s: an unknown answer = %v", engine, err)
		}
	}
}

func TestPasswordRotationRefusesAPrivilegedAccount(t *testing.T) {
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			engine := &rotationEngine{present: true, privileged: true}
			statements := installRotationEngine(t, engineName, engine)
			response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{
				User: transport.SuspensionDatabaseUser{Engine: engineName, Username: "shop_user"}, NewPassword: "Zq7xN2kLp9Vt4RmW",
			})
			if response.AccountRefused != transport.DatabaseAccountRefusedPrivileged || response.PasswordChanged || response.Applied ||
				!strings.Contains(response.Error, "nothing was changed") || len(engine.statements) != 1 || engine.statements[0] != statements.inspect {
				t.Fatalf("response %+v statements %q", response, engine.statements)
			}
			// The account became privileged after the inspection: the guard
			// in the password's own transaction refuses.
			engine = &rotationEngine{present: true, setRefused: true}
			installRotationEngine(t, engineName, engine)
			response = rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{
				User: transport.SuspensionDatabaseUser{Engine: engineName, Username: "shop_user"}, NewPassword: "Zq7xN2kLp9Vt4RmW",
			})
			if response.AccountRefused != transport.DatabaseAccountRefusedPrivileged || response.PasswordChanged || response.SessionsEnded {
				t.Fatalf("write-time refusal %+v", response)
			}
		})
	}
}

func TestSuspensionRefusesAPrivilegedAccount(t *testing.T) {
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			before := transport.DatabaseUserState{Present: true}
			engine := &fakeDatabaseEngine{present: true, privileged: true}
			statements := installFakeDatabaseEngine(t, engineName, engine)
			for _, suspend := range []bool{true, false} {
				engine.statements = nil
				response := setDatabaseSuspension(t, engineName, suspend, before)
				if response.AccountRefused != transport.DatabaseAccountRefusedPrivileged || response.Applied || response.Changed ||
					len(engine.statements) != 1 || engine.statements[0] != statements.inspect {
					t.Fatalf("suspend=%v: %+v %q", suspend, response, engine.statements)
				}
			}
			// The guard in the lock's own statement refuses.
			engine = &fakeDatabaseEngine{present: true, writeRefused: true}
			installFakeDatabaseEngine(t, engineName, engine)
			response := setDatabaseSuspension(t, engineName, true, before)
			if response.AccountRefused != transport.DatabaseAccountRefusedPrivileged || response.Applied || response.Changed || engine.locked {
				t.Fatalf("write-time refusal %+v", response)
			}
		})
	}
}

func TestDeletingADatabaseNeverDropsAPrivilegedUser(t *testing.T) {
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			for _, state := range []string{"privileged", ""} {
				e := &guardEngine{state: state}
				installGuardEngine(t, e)
				var resp transport.DeleteDatabaseResponse
				if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: engineName, Name: "shop", User: "boss"}, &resp); err != nil {
					t.Fatal(err)
				}
				want := transport.DatabaseAccountRefusedPrivileged
				if state == "" {
					want = transport.DatabaseAccountRefusedUnreadable
				}
				if resp.Success || resp.AccountRefused != want || len(e.writes()) != 0 {
					t.Fatalf("state %q: %+v writes %q", state, resp, e.writes())
				}
			}
			// An ordinary user is dropped behind the guard; an absent one is
			// not dropped and the database still goes.
			e := &guardEngine{state: "ok"}
			installGuardEngine(t, e)
			var resp transport.DeleteDatabaseResponse
			if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: engineName, Name: "shop", User: "shop_user"}, &resp); err != nil {
				t.Fatal(err)
			}
			all := strings.Join(e.writes(), "\n")
			if !resp.Success || !strings.Contains(all, "celikpanel-refused") || !strings.Contains(all, "DROP USER IF EXISTS") || !strings.Contains(all, "DROP DATABASE IF EXISTS") {
				t.Fatalf("ordinary: %+v %q", resp, e.writes())
			}
			e = &guardEngine{state: "absent"}
			installGuardEngine(t, e)
			resp = transport.DeleteDatabaseResponse{}
			if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: engineName, Name: "shop", User: "shop_user"}, &resp); err != nil {
				t.Fatal(err)
			}
			all = strings.Join(e.writes(), "\n")
			if !resp.Success || strings.Contains(all, "DROP USER") || !strings.Contains(all, "DROP DATABASE IF EXISTS") {
				t.Fatalf("absent: %+v %q", resp, e.writes())
			}
		})
	}
}

func TestRemovingAnAddedUserNeverTouchesAPrivilegedAccount(t *testing.T) {
	answer := func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return []byte("host=localhost\n"), nil
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=1\nowner=shop_owner\n"), nil
		case strings.HasPrefix(statement, "SELECT 'deps='"):
			return []byte("deps=0\n"), nil
		case strings.Contains(statement, "TABLE_NAME = 'roles_mapping'"):
			return []byte("n=1\n"), nil
		}
		return []byte("n=0\n"), nil
	}
	withOwnHosts(t)
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			req := &transport.DatabaseAccessUserRequest{Engine: engineName, Database: "shop", Username: "boss"}
			e := &guardEngine{state: "privileged", other: answer}
			installGuardEngine(t, e)
			var resp transport.DatabaseAccessUserResponse
			if err := (&Agent{}).RemoveDatabaseAccessUserV1(req, &resp); err != nil {
				t.Fatal(err)
			}
			all := strings.Join(e.writes(), "\n")
			if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged ||
				strings.Contains(all, "DROP") || strings.Contains(all, "REVOKE") || strings.Contains(all, "REASSIGN") {
				t.Fatalf("privileged: %+v %q", resp, e.writes())
			}
			e = &guardEngine{state: "ok", other: answer}
			installGuardEngine(t, e)
			resp = transport.DatabaseAccessUserResponse{}
			req.Username = "shop_reader"
			if err := (&Agent{}).RemoveDatabaseAccessUserV1(req, &resp); err != nil {
				t.Fatal(err)
			}
			if !resp.Applied || resp.AccountRefused != "" {
				t.Fatalf("ordinary: %+v %q", resp, e.writes())
			}
			for _, statement := range e.writes() {
				if (strings.Contains(statement, "DROP ") || strings.Contains(statement, "REASSIGN")) && !strings.Contains(statement, "celikpanel-refused") {
					t.Fatalf("a write without the guard: %q", statement)
				}
			}
		})
	}
}

func TestAddingAUserNeverUsesASystemDatabaseOrAPrivilegedWriter(t *testing.T) {
	for engine, database := range map[string]string{"mysql": "mysql", "postgresql": "postgres"} {
		e := &guardEngine{state: "ok"}
		installGuardEngine(t, e)
		var resp transport.DatabaseAccessUserResponse
		req := &transport.DatabaseAccessUserRequest{Engine: engine, Database: database, Username: "shop_reader", Password: "Zq7xN2kLp9Vt4RmW", Access: transport.DatabaseAccessWrite}
		if err := (&Agent{}).CreateDatabaseAccessUserV1(req, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedSystemDatabase || len(e.statements) != 0 {
			t.Fatalf("%s: %+v %q", engine, resp, e.statements)
		}
	}
	// A writer the panel names whose role is privileged: nothing is made.
	answer := func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=0\nowner=shop_owner\n"), nil
		case strings.HasPrefix(statement, "SELECT 'role='"):
			return []byte("role=boss\n"), nil
		}
		return nil, nil
	}
	e := &guardEngine{state: "privileged", other: answer}
	installGuardEngine(t, e)
	var resp transport.DatabaseAccessUserResponse
	req := &transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "shop", Username: "shop_reader", Password: "Zq7xN2kLp9Vt4RmW",
		Access: transport.DatabaseAccessWrite, Writers: []string{"boss"}}
	if err := (&Agent{}).CreateDatabaseAccessUserV1(req, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Applied || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || strings.Contains(strings.Join(e.statements, "\n"), "CREATE ROLE") {
		t.Fatalf("privileged writer: %+v %q", resp, e.statements)
	}
	// An ordinary writer: the user is made and the grants carry its guard.
	e = &guardEngine{state: "ok", other: answer}
	installGuardEngine(t, e)
	resp = transport.DatabaseAccessUserResponse{}
	if err := (&Agent{}).CreateDatabaseAccessUserV1(req, &resp); err != nil {
		t.Fatal(err)
	}
	grants := e.statements[len(e.statements)-1]
	if !resp.Applied || !strings.Contains(grants, "\nBEGIN;\nDO $celikpanel_account_guard$") || !strings.Contains(grants, "WHERE celikpanel_r.rolname OPERATOR(pg_catalog.=) 'boss'") {
		t.Fatalf("ordinary writer: %+v %q", resp, grants)
	}
}

func TestAResumedCreateNeverHandsADatabaseToAPrivilegedRole(t *testing.T) {
	const operation, token = "0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	marker := "celikpanel-operation:" + wordpressDatabaseOwnershipHash(operation, token)
	e := &guardEngine{state: "privileged", other: func(statement string) ([]byte, error) {
		if strings.HasPrefix(statement, "SELECT 'roles='") {
			return []byte("roles=1\nmark=" + marker + "\nowner=\nver=170000\n"), nil
		}
		return nil, nil
	}}
	installGuardEngine(t, e)
	var resp transport.CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(transport.CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "boss", Password: "Zq7xN2kLp9Vt4RmW",
		OperationID: operation, CleanupToken: token}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || resp.AccountRefused != transport.DatabaseAccountRefusedPrivileged || strings.Contains(strings.Join(e.statements, "\n"), "CREATE DATABASE") {
		t.Fatalf("resumed: %+v %q", resp, e.statements)
	}
}

func TestDroppedRightsAndGrantFormsRefusePrivilegedAccounts(t *testing.T) {
	e := &guardEngine{state: "privileged"}
	installGuardEngine(t, e)
	var dropped transport.DroppedDatabaseRightsResponse
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "shop", Owners: []string{"boss"}}, &dropped); err != nil {
		t.Fatal(err)
	}
	if dropped.Applied || dropped.AccountRefused != transport.DatabaseAccountRefusedPrivileged || len(e.writes()) != 0 {
		t.Fatalf("dropped rights: %+v %q", dropped, e.writes())
	}
	var converted transport.DatabaseGrantFormConvertResponse
	req := &transport.DatabaseGrantFormConvertRequest{ConvertRequest: mariadbgrant.ConvertRequest{User: "boss", Host: "localhost", Database: "shop_a", Flags: "Y"}}
	if err := (&Agent{}).DatabaseGrantFormConvertV1(req, &converted); err != nil {
		t.Fatal(err)
	}
	if converted.Outcome != "" || converted.AccountRefused != transport.DatabaseAccountRefusedPrivileged || len(e.writes()) != 0 {
		t.Fatalf("grant form: %+v %q", converted, e.writes())
	}
	// An owner already gone (its user was removed first) is no refusal.
	e = &guardEngine{state: "absent", other: func(string) ([]byte, error) { return nil, errors.New("stop here") }}
	installGuardEngine(t, e)
	dropped = transport.DroppedDatabaseRightsResponse{}
	if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "shop", Owners: []string{"shop_user"}}, &dropped); err != nil {
		t.Fatal(err)
	}
	if dropped.AccountRefused != "" || len(e.writes()) == 0 {
		t.Fatalf("an absent owner was refused: %+v", dropped)
	}
}

func TestRemoteAccessFollowsTheSharedRule(t *testing.T) {
	// PostgreSQL: CREATEDB and memberships count as server-wide rights.
	restore := databaseRemotePGQuery
	t.Cleanup(func() { databaseRemotePGQuery = restore })
	var asked string
	databaseRemotePGQuery = func(_ int, statement string) (string, error) {
		asked = statement
		return "boss\x1ftrue\x1ftrue\x1ftrue\ndb\x1fshop\n", nil
	}
	ok, problems, err := pgRemoteRoles(5432, []dbremote.Rule{{Database: "shop", User: "boss", Address: "203.0.113.7/32"}})
	if err != nil || len(ok) != 0 || len(problems) != 1 || problems[0].Code != transport.DatabaseRemotePrivilegedUser ||
		!strings.Contains(asked, pgAccountPrivilegedExpr("celikpanel_r")) {
		t.Fatalf("pg remote: ok %v problems %+v err %v query %s", ok, problems, err, asked)
	}
	// MariaDB: an account SHOW GRANTS calls ordinary but which the rule finds
	// privileged (a role, say) is never copied.
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}, guardStates: map[string]string{"shop_u": "privileged"}}
	withFakeMariaDB(t, f)
	target, err := dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7"}}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	var ts dbremote.TargetState
	result := mariadbRemoteInstance{}.apply(target, dbRemoteFacts{running: true}, &ts, "db.example.com")
	refused := false
	for _, p := range result.Problems {
		refused = refused || p.Code == transport.DatabaseRemotePrivilegedUser
	}
	if !refused || strings.Contains(strings.Join(f.executed, "\n"), "CREATE USER 'shop_u'@'203.0.113.7'") || len(ts.Accounts) != 0 {
		t.Fatalf("mariadb remote: problems %+v executed %q", result.Problems, f.executed)
	}
}

func TestACpanelDumpIsNeverLoadedIntoASystemDatabase(t *testing.T) {
	var resp CpmoveImportDBResponse
	if err := importCpmoveDatabase(&CpmoveImportDBRequest{Path: "/nonexistent.tar.gz", TargetDB: "mysql", DumpName: "shop"}, &resp, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Error, "system database") {
		t.Fatalf("resp = %+v", resp)
	}
}
