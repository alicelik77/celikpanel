package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 round 5 (W3, AGENTS.md: the owner's own changes are detected,
// never silently overwritten). Removing a database's user takes the account
// away only when every right it has is one CelikPanel gave. A right the
// owner gave by hand - on another database, or on a host CelikPanel did not
// create - keeps the account; only CelikPanel's grant on this database goes.

func withOwnHosts(t *testing.T, hosts ...string) {
	t.Helper()
	previous := mysqlAccessUserOwnHosts
	mysqlAccessUserOwnHosts = func(string) (map[string]bool, error) {
		set := map[string]bool{"localhost": true}
		for _, host := range hosts {
			set[host] = true
		}
		return set, nil
	}
	t.Cleanup(func() { mysqlAccessUserOwnHosts = previous })
}

func recordMySQL(t *testing.T, answer func(string) string) *[]string {
	t.Helper()
	previous := mysqlExecStatement
	var got []string
	mysqlExecStatement = func(statement string) ([]byte, error) {
		got = append(got, statement)
		// MariaDB has its role table unless a test says otherwise.
		if strings.Contains(statement, "TABLE_NAME = 'roles_mapping'") {
			if out := answer(statement); out != "" {
				return []byte(out), nil
			}
			return []byte("n=1\n"), nil
		}
		return []byte(answer(statement)), nil
	}
	t.Cleanup(func() { mysqlExecStatement = previous })
	return &got
}

func TestMySQLUserWithARightOnAnotherDatabaseIsKeptAndLosesOnlyThisOne(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=localhost\n"
		case strings.Contains(statement, "TABLE_SCHEMA <> 'shop'"):
			// The owner granted SELECT on another schema by hand.
			return "n=1\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('f=', HEX(Db))"):
			// alpha.134: the host holds CelikPanel's grant on shop.
			return "f=73686F70\n"
		}
		return ""
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	if !resp.Applied || !resp.Kept || resp.KeptReason != transport.DatabaseAccessKeptOtherRights || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	all := strings.Join(*got, "\n")
	if strings.Contains(all, "DROP USER") {
		t.Fatalf("an account with the owner's rights was dropped: %v", *got)
	}
	if !strings.Contains(all, "REVOKE ALL PRIVILEGES ON `shop`.* FROM 'shop_user'@'localhost';") {
		t.Fatalf("CelikPanel's grant on this database stayed: %v", *got)
	}
}

func TestMySQLUserWithAHostCelikPanelDidNotCreateIsKept(t *testing.T) {
	withOwnHosts(t, "203.0.113.7")
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.0.0.%\nhost=203.0.113.7\nhost=localhost\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('f=', HEX(Db))"):
			// alpha.134: each of CelikPanel's hosts holds its grant on shop.
			return "f=73686F70\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	if !resp.Applied || !resp.Kept {
		t.Fatalf("resp = %+v", resp)
	}
	all := strings.Join(*got, "\n")
	if strings.Contains(all, "DROP USER") || strings.Contains(all, "FROM 'shop_user'@'10.0.0.%'") {
		t.Fatalf("the owner's own host was touched: %v", *got)
	}
	if !strings.Contains(all, "FROM 'shop_user'@'localhost'") || !strings.Contains(all, "FROM 'shop_user'@'203.0.113.7'") {
		t.Fatalf("CelikPanel's grants were not taken away: %v", *got)
	}
}

func TestMySQLUserWhoseRemoteAccessRecordCannotBeReadIsKept(t *testing.T) {
	previous := mysqlAccessUserOwnHosts
	mysqlAccessUserOwnHosts = func(string) (map[string]bool, error) {
		return map[string]bool{"localhost": true}, errors.New("record unreadable")
	}
	t.Cleanup(func() { mysqlAccessUserOwnHosts = previous })
	got := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return "host=localhost\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	if !resp.Kept || strings.Contains(strings.Join(*got, "\n"), "DROP USER") {
		t.Fatalf("resp = %+v statements %v", resp, *got)
	}
}

func TestMySQLUserWithOnlyCelikPanelsRightsIsDropped(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return "host=localhost\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	if !resp.Applied || resp.Kept || !strings.Contains(strings.Join(*got, "\n"), "DROP USER 'shop_user'@'localhost'") {
		t.Fatalf("resp = %+v statements %v", resp, *got)
	}
}

func pgRemoval(t *testing.T, answer func(string) ([]byte, error)) (transport.DatabaseAccessUserResponse, []string) {
	t.Helper()
	var got []string
	run := func(statement string) ([]byte, error) {
		got = append(got, statement)
		return answer(statement)
	}
	var resp transport.DatabaseAccessUserResponse
	removePostgreSQLAccessUser(run, &transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "shop", Username: "shop_user"}, &resp)
	return resp, got
}

func TestPostgreSQLRoleWithRightsElsewhereIsKeptNotRetriedForever(t *testing.T) {
	// The database is gone (deleted first): no owner; the role still owns
	// a table in another database.
	resp, got := pgRemoval(t, func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=1\nowner=\n"), nil
		case strings.HasPrefix(statement, "SELECT 'deps='"):
			return []byte("deps=2\n"), nil
		}
		return nil, nil
	})
	if !resp.Applied || !resp.Kept || resp.KeptReason != transport.DatabaseAccessKeptOtherRights || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	for _, statement := range got {
		if strings.HasPrefix(statement, "DROP ROLE") {
			t.Fatalf("a role with rights elsewhere was dropped: %v", got)
		}
	}

	// The engine refuses DROP ROLE for dependents anyway: kept, not an error.
	resp, _ = pgRemoval(t, func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=1\nowner=\n"), nil
		case strings.HasPrefix(statement, "SELECT 'deps='"):
			return []byte("deps=0\n"), nil
		case strings.HasPrefix(statement, "DROP ROLE"):
			return []byte(`ERROR:  role "shop_user" cannot be dropped because some objects depend on it`), errors.New("exit status 3")
		}
		return nil, nil
	})
	if !resp.Applied || !resp.Kept {
		t.Fatalf("resp = %+v", resp)
	}

	// Nothing else: the role goes.
	resp, got = pgRemoval(t, func(statement string) ([]byte, error) {
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=1\nowner=\n"), nil
		case strings.HasPrefix(statement, "SELECT 'deps='"):
			return []byte("deps=0\n"), nil
		}
		return nil, nil
	})
	if !resp.Applied || resp.Kept || got[len(got)-1] != `DROP ROLE "shop_user";` {
		t.Fatalf("resp = %+v statements %v", resp, got)
	}
}

// W-B (review): from PostgreSQL 16 a role created by a non-superuser with
// CREATEROLE is granted back to its creator (ADMIN TRUE, SET FALSE, INHERIT
// FALSE, grantor the bootstrap superuser). CelikPanel's own account creates
// roles that way through a record's credential; that row is not a right the
// owner gave, so it must not keep the role. The statement counts the role's
// own memberships and other roles' memberships in it except that one (and
// the bootstrap superuser's). This is the statement's shape; the real
// PostgreSQL 17 behaviour is a lab check.
func TestPostgreSQLRoleCheckIgnoresTheAutomaticCreatorMembership(t *testing.T) {
	statement := postgreSQLRoleElsewhereStatement("'shop_user'")
	for _, want := range []string{
		"JOIN pg_catalog.pg_roles r ON r.oid = m.member WHERE r.rolname = 'shop_user'",
		"JOIN pg_catalog.pg_roles r ON r.oid = m.roleid",
		"m.member <> 10",
		"who.rolname <> '" + databaseAdminAccountName + "'",
		"pg_catalog.pg_shdepend",
	} {
		if !strings.Contains(statement, want) {
			t.Fatalf("missing %q in %s", want, statement)
		}
	}
	if strings.Contains(statement, "IN (m.member, m.roleid)") {
		t.Fatal("every membership row still counts")
	}
}

// N-D (review): a routine the user made in the database being deleted is not
// a right elsewhere; a server without MariaDB's role table (MySQL 8) is not
// asked about it; and a check that fails keeps the account as "unchecked"
// instead of failing on every press.
func TestMySQLRightsCheckCountsRoutinesElsewhereOnlyAndSurvivesMySQL(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=localhost\n"
		case strings.Contains(statement, "TABLE_NAME = 'roles_mapping'"):
			return "n=0\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	all := strings.Join(*got, "\n")
	if !resp.Applied || resp.Kept || strings.Contains(all, "mysql.roles_mapping WHERE") {
		t.Fatalf("resp = %+v statements %v", resp, *got)
	}
	if !strings.Contains(all, "FROM mysql.procs_priv WHERE User = 'shop_user' AND Host IN ('localhost') AND Db <> 'shop'") {
		t.Fatalf("routine grants are not limited to other databases: %v", *got)
	}

	failing := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return "host=localhost\n"
		}
		return "ERROR 1146 (42S02): Table 'mysql.something' doesn't exist"
	})
	resp = transport.DatabaseAccessUserResponse{}
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	if !resp.Applied || !resp.Kept || resp.KeptReason != transport.DatabaseAccessKeptUnchecked || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	if strings.Contains(strings.Join(*failing, "\n"), "DROP USER") {
		t.Fatal("an account whose rights could not be checked was dropped")
	}
}
