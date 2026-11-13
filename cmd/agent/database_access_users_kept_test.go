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
		case strings.Contains(statement, "TABLE_SCHEMA = 'shop'"):
			return "n=1\n"
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
		case strings.Contains(statement, "TABLE_SCHEMA = 'shop'"):
			return "n=1\n"
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
