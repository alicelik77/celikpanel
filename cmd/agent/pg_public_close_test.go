package main

import (
	"errors"
	"strings"
	"testing"
)

// 20 Nov 2026 (security): a database the Agent creates is closed to every
// other role on the cluster - PUBLIC's CONNECT and TEMPORARY revoked - and
// the creation is complete only when that is read back. One customer's
// database user could otherwise sign in to another customer's database.

func closeAnswers(t *testing.T, roles, mark, owner string, close func() ([]byte, error)) *[]string {
	t.Helper()
	usePGStatementClusters(t, bostonLsclusters)
	var got []string
	previous := postgreSQLExecStatementOnPort
	postgreSQLExecStatementOnPort = func(_ int, statement string) ([]byte, error) {
		got = append(got, statement)
		switch {
		case strings.HasPrefix(statement, "SELECT 'roles='"):
			return []byte("roles=" + roles + "\nmark=" + mark + "\nowner=" + owner + "\nver=180001\n"), nil
		case strings.HasPrefix(statement, "REVOKE ALL ON DATABASE"):
			return close()
		}
		return nil, nil
	}
	t.Cleanup(func() { postgreSQLExecStatementOnPort = previous })
	return &got
}

func TestANewPostgreSQLDatabaseIsClosedToPublicAndReadBack(t *testing.T) {
	got := closeAnswers(t, "0", "", "", func() ([]byte, error) { return []byte("public=false\nowner=true\n"), nil })
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil || !resp.Success {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	last := (*got)[len(*got)-1]
	for _, want := range []string{`REVOKE ALL ON DATABASE "shop" FROM PUBLIC;`,
		"has_database_privilege('public', 'shop', 'CONNECT') OR pg_catalog.has_database_privilege('public', 'shop', 'TEMPORARY')",
		"has_database_privilege('shop_user', 'shop', 'CONNECT')"} {
		if !strings.Contains(last, want) {
			t.Fatalf("close statement %q lacks %q", last, want)
		}
	}
}

// Not closed (the revoke refused, or the read-back still open, or not
// readable): the database and the role this call made go again, and the
// answer says so; nothing is reported as made.
func TestADatabaseThatCannotBeClosedIsRemovedAgain(t *testing.T) {
	for name, close := range map[string]func() ([]byte, error){
		"revoke refused": func() ([]byte, error) {
			return []byte("ERROR:  permission denied for database shop"), errors.New("exit status 3")
		},
		"still open":   func() ([]byte, error) { return []byte("public=true\nowner=true\n"), nil },
		"owner shut":   func() ([]byte, error) { return []byte("public=false\nowner=false\n"), nil },
		"not readable": func() ([]byte, error) { return []byte("REVOKE\n"), nil },
	} {
		got := closeAnswers(t, "0", "", "", close)
		var resp CreateDatabaseResponse
		if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(*got, "\n")
		if resp.Success || resp.CleanupIncomplete || !strings.Contains(resp.Error, "removed again") ||
			!strings.Contains(joined, `DROP DATABASE IF EXISTS "shop";`) || !strings.Contains(joined, `DROP ROLE IF EXISTS "shop_user";`) {
			t.Fatalf("%s: resp=%+v statements=%q", name, resp, *got)
		}
	}
}

// A replay of the same operation resumes on its own database: it is closed
// too, and a failure to close it never drops what an earlier attempt made.
func TestAResumedOperationsDatabaseIsClosedAndNeverDropped(t *testing.T) {
	req := CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433,
		OperationID: strings.Repeat("ab", 16), CleanupToken: strings.Repeat("cd", 32)}
	mark := "celikpanel-operation:" + wordpressDatabaseOwnershipHash(req.OperationID, req.CleanupToken)
	got := closeAnswers(t, "1", mark, "shop_user", func() ([]byte, error) { return []byte("public=false\nowner=true\n"), nil })
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(req, &resp); err != nil || !resp.Success {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if joined := strings.Join(*got, "\n"); strings.Contains(joined, "CREATE DATABASE") || !strings.Contains(joined, "REVOKE ALL ON DATABASE") {
		t.Fatalf("statements = %q", *got)
	}
	got = closeAnswers(t, "1", mark, "shop_user", func() ([]byte, error) { return []byte("public=true\nowner=true\n"), nil })
	resp = CreateDatabaseResponse{}
	if err := (&Agent{}).CreateDatabase(req, &resp); err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(*got, "\n"); resp.Success || strings.Contains(joined, "DROP DATABASE") || strings.Contains(joined, "DROP ROLE") {
		t.Fatalf("resp=%+v statements=%q", resp, *got)
	}
}
