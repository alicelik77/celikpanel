package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.133 lab finding (MariaDB 11.8): deleting a database kept an account
// whose owner had made it a host of their own ('10.%') with GRANT ALL ON the
// database; the hand-made host kept that grant, because only CelikPanel's
// own hosts were revoked. MariaDB keeps the rights that name a dropped
// database (KB, DROP DATABASE), and a later database of the same name would
// inherit them. When the database is dropped, every right that names it
// exactly goes from every host of the user name; the account, its hosts and
// its rights elsewhere stay; a wildcard grant names no one database and
// stays.

func droppedRemoval(t *testing.T, hosts string, answer func(string) string) (transport.DatabaseAccessUserResponse, []string) {
	t.Helper()
	got := recordMySQL(t, func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return hosts
		}
		return answer(statement)
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "pastane_dr_test_hand5",
		Username: "pastane_dr_test_hand5_user", DatabaseDropped: true}, &resp)
	return resp, *got
}

const droppedForms = `Db IN ('pastane_dr_test_hand5', 'pastane\\_dr\\_test\\_hand5')`

func TestDroppedDatabaseLeavesNoRightOnAHandMadeHost(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, "host=10.%\nhost=localhost\n", func(statement string) string { return "n=0\n" })
	if !resp.Applied || !resp.Kept || resp.RightsLeft || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	all := strings.Join(got, "\n")
	if strings.Contains(all, "DROP USER") {
		t.Fatal("an account with a host its owner made was dropped")
	}
	for _, table := range []string{"mysql.db", "mysql.tables_priv", "mysql.columns_priv", "mysql.procs_priv"} {
		// Every host of the name (no Host filter), exactly this database.
		if !strings.Contains(all, "DELETE FROM "+table+" WHERE User = 'pastane_dr_test_hand5_user' AND "+droppedForms+";") {
			t.Fatalf("%s rows naming the database are not removed: %v", table, got)
		}
	}
	if !strings.Contains(all, "FLUSH PRIVILEGES;") {
		t.Fatal("the grant tables were changed without FLUSH PRIVILEGES")
	}
}

func TestDroppedDatabaseKeepsARightOnAnotherSchemaAndAWildcardGrant(t *testing.T) {
	withOwnHosts(t)
	resp, got := droppedRemoval(t, "host=localhost\n", func(statement string) string {
		// A hand-made SELECT on another schema (or a wildcard pattern) is a
		// right elsewhere: the account is kept.
		if strings.Contains(statement, "TABLE_SCHEMA NOT IN "+strings.TrimPrefix(droppedForms, "Db IN ")) {
			return "n=1\n"
		}
		return "n=0\n"
	})
	if !resp.Applied || !resp.Kept || resp.KeptReason != transport.DatabaseAccessKeptOtherRights {
		t.Fatalf("resp = %+v", resp)
	}
	for _, statement := range got {
		if !strings.HasPrefix(statement, "DELETE FROM") {
			continue
		}
		// Only the two exact forms of this database's name, never a
		// pattern and never another schema.
		if strings.Contains(statement, "%") || strings.Contains(statement, "kahvedukkani") || strings.Count(statement, droppedForms) != 4 {
			t.Fatalf("a removal reaches beyond this database: %s", statement)
		}
	}
}

func TestDroppedDatabaseRightThatCannotBeRemovedIsSaidAsLeft(t *testing.T) {
	withOwnHosts(t)
	resp, _ := droppedRemoval(t, "host=10.%\nhost=localhost\n", func(statement string) string {
		if strings.HasPrefix(statement, "SELECT CONCAT('n=', (SELECT COUNT(*) FROM mysql.db") {
			return "n=1\n"
		}
		return "n=0\n"
	})
	if resp.Applied || !resp.RightsLeft || resp.Error == "" {
		t.Fatalf("resp = %+v", resp)
	}
}

// The domain page's "remove added user" removes a user from a database that
// still exists: the owner's own hosts keep their rights there (unchanged).
func TestRemovingAUserFromALiveDatabaseTouchesOnlyCelikPanelsHosts(t *testing.T) {
	withOwnHosts(t)
	got := recordMySQL(t, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)"):
			return "host=10.%\nhost=localhost\n"
		case strings.Contains(statement, "TABLE_SCHEMA = 'shop'"):
			return "n=1\n"
		}
		return "n=0\n"
	})
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_user"}, &resp)
	all := strings.Join(*got, "\n")
	if !resp.Applied || !resp.Kept || strings.Contains(all, "DELETE FROM mysql.db") || strings.Contains(all, "FROM 'shop_user'@'10.%'") {
		t.Fatalf("resp = %+v statements %v", resp, *got)
	}
}
