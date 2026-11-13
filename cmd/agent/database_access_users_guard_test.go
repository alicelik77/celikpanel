package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Re-review of alpha.133's database deletion: the guards every engine keeps.

func pgDroppedRemoval(t *testing.T, req transport.DatabaseAccessUserRequest, answer func(string) []byte) (transport.DatabaseAccessUserResponse, []string) {
	t.Helper()
	var got []string
	run := func(statement string) ([]byte, error) {
		got = append(got, statement)
		return answer(statement), nil
	}
	var resp transport.DatabaseAccessUserResponse
	removePostgreSQLAccessUser(run, &req, &resp)
	return resp, got
}

// PostgreSQL: a user another database uses stays (RightsOnly): dropping the
// database already took every right in it, so nothing is run after the
// facts - above all no DROP ROLE.
func TestPostgreSQLSharedUserOfADroppedDatabaseIsLeftAlone(t *testing.T) {
	resp, got := pgDroppedRemoval(t, transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "shop", Username: "shop_user",
		DatabaseDropped: true, RightsOnly: true}, func(statement string) []byte {
		if strings.HasPrefix(statement, "SELECT 'roles='") {
			return []byte("roles=1\nowner=\n")
		}
		return nil
	})
	if !resp.Applied || resp.Kept || resp.Error != "" || len(got) != 1 {
		t.Fatalf("resp = %+v statements %v", resp, got)
	}
	for _, statement := range got {
		if strings.Contains(statement, "DROP ROLE") || strings.Contains(statement, "DROP OWNED") {
			t.Fatalf("a staying role was touched: %v", got)
		}
	}
}

// PostgreSQL: the dropped database is not taken on the panel's word.
func TestPostgreSQLDroppedDatabaseThatStillExistsChangesNothing(t *testing.T) {
	for _, rightsOnly := range []bool{false, true} {
		resp, got := pgDroppedRemoval(t, transport.DatabaseAccessUserRequest{Engine: "postgresql", Database: "shop", Username: "shop_user",
			DatabaseDropped: true, RightsOnly: rightsOnly}, func(statement string) []byte {
			if strings.HasPrefix(statement, "SELECT 'roles='") {
				return []byte("roles=1\nowner=shop_user\n")
			}
			return nil
		})
		if resp.Applied || !resp.DatabaseExists || len(got) != 1 {
			t.Fatalf("rightsOnly=%v: resp = %+v statements %v", rightsOnly, resp, got)
		}
	}
}

// Both engines: a rights-only removal without a dropped database is refused
// before anything runs; it must never fall through to removing the user.
func TestRightsOnlyRemovalWithoutADroppedDatabaseIsRefused(t *testing.T) {
	previous := mysqlExecStatement
	mysqlExecStatement = func(statement string) ([]byte, error) {
		t.Fatalf("a statement ran: %s", statement)
		return nil, nil
	}
	t.Cleanup(func() { mysqlExecStatement = previous })
	for _, engine := range []string{"mysql", "postgresql"} {
		var resp transport.DatabaseAccessUserResponse
		req := &transport.DatabaseAccessUserRequest{ExpectedBuildCommit: buildCommit, Engine: engine, Database: "shop", Username: "shop_user", RightsOnly: true}
		if err := (&Agent{}).RemoveDatabaseAccessUserV1(req, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Applied || !strings.Contains(resp.Error, "rights-only removal is for a deleted database only") {
			t.Fatalf("%s: resp = %+v", engine, resp)
		}
	}
}

// MariaDB: a database made again between the first existence check and the
// DELETE (whose NOT EXISTS then skips every row) is "exists", not "rights".
func TestDroppedDatabaseRecreatedDuringTheRemovalIsSaidAsExists(t *testing.T) {
	withOwnHosts(t)
	checks := 0
	resp, _ := droppedRemoval(t, true, func(statement string) string {
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA"):
			checks++
			if checks == 1 {
				return "n=0\n"
			}
			return "n=1\n"
		case strings.HasPrefix(statement, "SELECT CONCAT('n=', (SELECT COUNT(*) FROM information_schema.SCHEMA_PRIVILEGES"):
			return "n=1\n"
		}
		return ""
	})
	if resp.Applied || resp.RightsLeft || !resp.DatabaseExists || checks != 2 {
		t.Fatalf("resp = %+v, existence checks %d", resp, checks)
	}
}
