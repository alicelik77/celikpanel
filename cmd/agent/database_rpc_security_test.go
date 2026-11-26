package main

import (
	"errors"
	"strings"
	"testing"
)

const (
	testWordPressOperationID  = "00112233445566778899aabbccddeeff"
	testWordPressCleanupToken = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
)

func captureMySQLStatements(
	t *testing.T,
	executor func(int, string) ([]byte, error),
) *[]string {
	t.Helper()
	previous := mysqlExecStatement
	statements := &[]string{}
	mysqlExecStatement = func(statement string) ([]byte, error) {
		// alpha.134 review: a grant asks the server's version (for
		// NO_AUTO_CREATE_USER) and reads its row back; answered here, not
		// counted among the statements under test.
		switch {
		case strings.HasPrefix(statement, "SELECT CONCAT('v=', VERSION())"):
			return []byte("v=11.8.6-MariaDB-0+deb13u1\n"), nil
		case strings.HasPrefix(statement, "SELECT CONCAT('n=', COUNT(*)) FROM mysql.db WHERE User = "):
			return []byte("n=1\n"), nil
		case isAccountGuardRead(statement):
			// alpha.149: the account guard's read; an ordinary site account.
			return []byte("guard=ok\n"), nil
		case strings.HasPrefix(statement, "SELECT CONCAT('accounts=', COUNT(*)) FROM mysql.user WHERE User = "):
			// alpha.149 (second review, finding 5): no account of the name yet.
			return []byte("accounts=0\nothers=0\n"), nil
		}
		*statements = append(*statements, statement)
		return executor(len(*statements)-1, statement)
	}
	t.Cleanup(func() { mysqlExecStatement = previous })
	return statements
}

func wordpressCreateDatabaseRequest() CreateDatabaseRequest {
	return CreateDatabaseRequest{
		Type:         "mysql",
		Name:         "tenant_wordpress",
		User:         "tenant_wordpress",
		Password:     "secret-password",
		OperationID:  testWordPressOperationID,
		CleanupToken: testWordPressCleanupToken,
	}
}

func TestMySQLStatementIsSentOnStdinNotCommandArguments(t *testing.T) {
	statement := "CREATE USER 'tenant'@'localhost' IDENTIFIED BY 'super-secret';"
	command := newMySQLStatementCommand(statement)
	joined := strings.Join(command.Args, " ")
	if strings.Contains(joined, statement) || strings.Contains(joined, "super-secret") {
		t.Fatalf("SQL secret leaked into process arguments: %q", joined)
	}
	if command.Stdin == nil {
		t.Fatal("SQL statement was not attached to stdin")
	}
}

func TestWordPressDatabaseCreateIsStrictExclusiveAndMarked(t *testing.T) {
	statements := captureMySQLStatements(t, func(_ int, _ string) ([]byte, error) {
		return nil, nil
	})
	var response CreateDatabaseResponse
	if err := (&Agent{}).createMySQLDatabase(wordpressCreateDatabaseRequest(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || !response.OwnedByOperation || response.Error != "" {
		t.Fatalf("response=%+v", response)
	}
	if len(*statements) != 4 {
		t.Fatalf("statements=%d, want 4", len(*statements))
	}
	if !strings.HasPrefix((*statements)[0], "CREATE DATABASE ") ||
		strings.Contains((*statements)[0], "IF NOT EXISTS") {
		t.Fatalf("database create is not strict-exclusive: %s", (*statements)[0])
	}
	if !strings.HasPrefix((*statements)[1], "CREATE USER ") ||
		strings.Contains((*statements)[1], "IF NOT EXISTS") {
		t.Fatalf("user create is not strict-exclusive: %s", (*statements)[1])
	}
	if !strings.Contains((*statements)[2], wordpressDatabaseOwnershipTable) ||
		strings.Contains((*statements)[2], testWordPressCleanupToken) {
		t.Fatalf("ownership marker is missing or stores the raw cleanup token: %s", (*statements)[2])
	}
}

func TestWordPressDatabaseCollisionNeverTakesOverOrDeletes(t *testing.T) {
	statements := captureMySQLStatements(t, func(index int, _ string) ([]byte, error) {
		if index == 0 {
			return nil, errors.New("database exists")
		}
		return []byte("0\n"), nil
	})
	var response CreateDatabaseResponse
	_ = (&Agent{}).createMySQLDatabase(wordpressCreateDatabaseRequest(), &response)
	if response.Success || response.Error == "" {
		t.Fatalf("collision response=%+v", response)
	}
	for _, statement := range *statements {
		if strings.Contains(statement, "DROP ") || strings.Contains(statement, "GRANT ") {
			t.Fatalf("foreign database was mutated after collision: %s", statement)
		}
	}
}

func TestWordPressUserCollisionCleansOnlyDatabaseCreatedByThisCall(t *testing.T) {
	statements := captureMySQLStatements(t, func(index int, _ string) ([]byte, error) {
		if index == 1 {
			return nil, errors.New("user exists")
		}
		return nil, nil
	})
	var response CreateDatabaseResponse
	_ = (&Agent{}).createMySQLDatabase(wordpressCreateDatabaseRequest(), &response)
	// alpha.129: a fourth, read-only statement asks whether the name is taken,
	// so the refusal can say so.
	if response.Success || response.CleanupIncomplete || len(*statements) != 4 ||
		!strings.HasPrefix((*statements)[3], "SELECT CONCAT('n=', COUNT(*)) FROM mysql.user") {
		t.Fatalf("response=%+v statements=%v", response, *statements)
	}
	if !strings.HasPrefix((*statements)[2], "DROP DATABASE IF EXISTS ") {
		t.Fatalf("owned database was not cleaned after user collision: %s", (*statements)[2])
	}
	if strings.Contains((*statements)[2], "DROP USER") {
		t.Fatal("pre-existing user was targeted for deletion")
	}
}

// alpha.149 (second review of PR #370, finding 5): an account of the name on
// any host ('u'@'%', the server owner's own perhaps) or a role of the name:
// nothing is written, and the answer names the taken user.
func TestAMariaDBCreateNeverMakesAnAccountBesideOneOfTheSameName(t *testing.T) {
	for _, existing := range []string{"accounts=1\nothers=1\n", "accounts=2\nothers=1\n"} {
		var statements []string
		previous := mysqlExecStatement
		mysqlExecStatement = func(statement string) ([]byte, error) {
			statements = append(statements, statement)
			if strings.HasPrefix(statement, "SELECT CONCAT('accounts=', COUNT(*)) FROM mysql.user WHERE User = 'tenant_shop';") &&
				strings.Contains(statement, "AND Host <> 'localhost'") {
				return []byte(existing), nil
			}
			return nil, nil
		}
		for _, req := range []CreateDatabaseRequest{
			{Type: "mysql", Name: "tenant_shop", User: "tenant_shop", Password: "secret-password"},
			wordpressCreateDatabaseRequest(), // a resumed operation too
		} {
			statements = nil
			req.User = "tenant_shop"
			var response CreateDatabaseResponse
			if err := (&Agent{}).CreateDatabase(req, &response); err != nil {
				t.Fatal(err)
			}
			if response.Success || response.NameTaken != "user" || len(statements) != 1 || !strings.Contains(response.Error, "nothing was changed") {
				t.Fatalf("existing %q: %+v %q", existing, response, statements)
			}
		}
		mysqlExecStatement = previous
	}
	// The name only on localhost, without an operation: refused before
	// anything is written as well (no database made and dropped again).
	statements := captureMySQLStatements(t, func(int, string) ([]byte, error) { return nil, nil })
	previous := mysqlExecStatement
	mysqlExecStatement = func(statement string) ([]byte, error) {
		if strings.HasPrefix(statement, "SELECT CONCAT('accounts=', COUNT(*))") {
			return []byte("accounts=1\nothers=0\n"), nil
		}
		return previous(statement)
	}
	var response CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "mysql", Name: "tenant_shop", User: "tenant_shop", Password: "secret-password"}, &response); err != nil {
		t.Fatal(err)
	}
	if response.Success || response.NameTaken != "user" || len(*statements) != 0 {
		t.Fatalf("localhost twin: %+v %q", response, *statements)
	}
}

func TestWordPressCreateReportsIncompleteSelfCleanup(t *testing.T) {
	captureMySQLStatements(t, func(index int, _ string) ([]byte, error) {
		switch index {
		case 1:
			return nil, errors.New("user exists")
		case 2:
			return nil, errors.New("drop failed")
		default:
			return nil, nil
		}
	})
	var response CreateDatabaseResponse
	_ = (&Agent{}).createMySQLDatabase(wordpressCreateDatabaseRequest(), &response)
	if !response.CleanupIncomplete {
		t.Fatalf("response=%+v, want cleanup_incomplete", response)
	}
}

func TestWordPressLostCreateResponseCanResumeOnlyWithMatchingMarker(t *testing.T) {
	statements := captureMySQLStatements(t, func(index int, _ string) ([]byte, error) {
		switch index {
		case 0:
			return nil, errors.New("database exists")
		case 1:
			return []byte("1\n"), nil
		default:
			return nil, nil
		}
	})
	var response CreateDatabaseResponse
	_ = (&Agent{}).createMySQLDatabase(wordpressCreateDatabaseRequest(), &response)
	if !response.Success || !response.OwnedByOperation || len(*statements) != 3 {
		t.Fatalf("response=%+v statements=%v", response, *statements)
	}
	if !strings.Contains((*statements)[2], "NO_AUTO_CREATE_USER');\nGRANT ALL PRIVILEGES ON `tenant\\_wordpress`.*") {
		t.Fatalf("matching retry did not resume at grant: %s", (*statements)[2])
	}
}

func TestWordPressDeleteRequiresMatchingOwnershipToken(t *testing.T) {
	statements := captureMySQLStatements(t, func(_ int, _ string) ([]byte, error) {
		return []byte("0\n"), nil
	})
	request := DeleteDatabaseRequest{
		Type:                  "mysql",
		Name:                  "tenant_wordpress",
		User:                  "tenant_wordpress",
		RequireUserCleanup:    true,
		RequireOwnershipProof: true,
		OperationID:           testWordPressOperationID,
		CleanupToken:          testWordPressCleanupToken,
	}
	var response DeleteDatabaseResponse
	_ = (&Agent{}).deleteMySQLDatabase(request, &response)
	if response.Success || response.Error == "" || len(*statements) != 1 {
		t.Fatalf("response=%+v statements=%v", response, *statements)
	}
	if strings.Contains((*statements)[0], "DROP ") {
		t.Fatal("destructive cleanup ran without ownership proof")
	}
}

func TestWordPressDeleteWithMatchingOwnershipDropsUserThenDatabase(t *testing.T) {
	statements := captureMySQLStatements(t, func(index int, _ string) ([]byte, error) {
		if index == 0 {
			return []byte("1\n"), nil
		}
		return nil, nil
	})
	request := DeleteDatabaseRequest{
		Type:                  "mysql",
		Name:                  "tenant_wordpress",
		User:                  "tenant_wordpress",
		RequireUserCleanup:    true,
		RequireOwnershipProof: true,
		OperationID:           testWordPressOperationID,
		CleanupToken:          testWordPressCleanupToken,
	}
	var response DeleteDatabaseResponse
	_ = (&Agent{}).deleteMySQLDatabase(request, &response)
	if !response.Success || len(*statements) != 3 {
		t.Fatalf("response=%+v statements=%v", response, *statements)
	}
	// alpha.149 (security): the user is dropped behind the account guard.
	if !strings.HasPrefix((*statements)[1], "DELIMITER ;;\nBEGIN NOT ATOMIC\n") || !strings.Contains((*statements)[1], "\n  DROP USER IF EXISTS ") ||
		!strings.HasPrefix((*statements)[2], "DROP DATABASE IF EXISTS ") {
		t.Fatalf("cleanup order is unsafe: %v", *statements)
	}
}
