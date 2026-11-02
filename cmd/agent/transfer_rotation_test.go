package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

const installedWPConfig = "<?php\n" +
	"define( 'DB_NAME', 'shop_wp' );\n" +
	"define( 'DB_USER', 'shop_user' );\n" +
	"define( 'DB_PASSWORD', 'old\\'pass' );\n" +
	"define( 'DB_HOST', 'localhost' );\n" +
	"$table_prefix = 'wp_';\n"

func TestWordPressConfigDefinesAndReplace(t *testing.T) {
	if reason := wordpressConfigMatches([]byte(installedWPConfig), "shop_wp", "shop_user"); reason != "" {
		t.Fatalf("installed file not recognised: %s", reason)
	}
	if reason := wordpressConfigMatches([]byte(installedWPConfig), "shop_wp", "other_user"); !strings.Contains(reason, "no longer uses") {
		t.Fatalf("other user = %q", reason)
	}
	twice := installedWPConfig + "define( 'DB_PASSWORD', 'x' );\n"
	if reason := wordpressConfigMatches([]byte(twice), "shop_wp", "shop_user"); !strings.Contains(reason, "exactly one DB_PASSWORD") {
		t.Fatalf("two passwords = %q", reason)
	}
	next, err := replaceWordPressDBPassword([]byte(installedWPConfig), `n3w'pa\ss`)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(installedWPConfig, `'old\'pass'`, `'n3w\'pa\\ss'`, 1)
	if string(next) != want {
		t.Fatalf("replaced:\n%s", next)
	}
	// The value reads back as the new password; nothing else moved.
	_, _, start, end, err := wordpressDBDefines(next)
	if err != nil || phpUnquote(string(next[start+1:end-1])) != `n3w'pa\ss` {
		t.Fatalf("read back %q, %v", next[start:end], err)
	}
}

// rotationEngine answers the inspect, password and disconnect statements.
type rotationEngine struct {
	present    bool
	statements []string
	failSet    bool
}

func installRotationEngine(t *testing.T, engineName string, engine *rotationEngine) suspensionDatabaseStatements {
	t.Helper()
	statements, err := suspensionDatabaseSQL(engineName, "shop_user")
	if err != nil {
		t.Fatal(err)
	}
	run := func(statement string) ([]byte, error) {
		engine.statements = append(engine.statements, statement)
		switch {
		case statement == statements.inspect:
			if !engine.present {
				return []byte("state=absent\n"), nil
			}
			return []byte("state=open\n"), nil
		case statement == statements.disconnect:
			return nil, nil
		case strings.Contains(statement, "PASSWORD") || strings.Contains(statement, "IDENTIFIED BY"):
			if engine.failSet {
				return []byte("ERROR 1064: near '" + statement + "'"), errors.New("exit status 1")
			}
			return nil, nil
		}
		return nil, errors.New("unexpected statement")
	}
	oldMySQL, oldPG := mysqlExecStatement, postgreSQLExecStatement
	mysqlExecStatement, postgreSQLExecStatement = run, run
	t.Cleanup(func() { mysqlExecStatement, postgreSQLExecStatement = oldMySQL, oldPG })
	return statements
}

func rotateDatabaseUser(t *testing.T, request transport.RotateDatabaseUserPasswordRequest) transport.RotateDatabaseUserPasswordResponse {
	t.Helper()
	var response transport.RotateDatabaseUserPasswordResponse
	if err := (&Agent{}).RotateDatabaseUserPasswordV1(&request, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func TestRotateDatabaseUserPasswordSetsPasswordAndEndsSessions(t *testing.T) {
	for _, engineName := range []string{"mysql", "postgresql"} {
		t.Run(engineName, func(t *testing.T) {
			engine := &rotationEngine{present: true}
			statements := installRotationEngine(t, engineName, engine)
			secret := "Zq7xN2kLp9Vt4RmW"
			response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{
				User: transport.SuspensionDatabaseUser{Engine: engineName, Username: "shop_user"}, NewPassword: secret,
			})
			if !response.Applied || !response.PasswordChanged || !response.SessionsEnded || response.Error != "" {
				t.Fatalf("response %+v", response)
			}
			if len(engine.statements) != 3 || engine.statements[0] != statements.inspect || engine.statements[2] != statements.disconnect ||
				!strings.Contains(engine.statements[1], "'"+secret+"'") {
				t.Fatalf("statements %q", engine.statements)
			}
		})
	}
}

func TestRotateDatabaseUserPasswordNeverForwardsTheEngineAnswer(t *testing.T) {
	engine := &rotationEngine{present: true, failSet: true}
	installRotationEngine(t, "mysql", engine)
	secret := "Zq7xN2kLp9Vt4RmW"
	response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{
		User: transport.SuspensionDatabaseUser{Engine: "mysql", Username: "shop_user"}, NewPassword: secret,
	})
	if response.Applied || response.PasswordChanged || strings.Contains(response.Error, secret) || response.Error == "" {
		t.Fatalf("response %+v", response)
	}
}

func TestRotateDatabaseUserPasswordAbsentAndRefusals(t *testing.T) {
	engine := &rotationEngine{}
	installRotationEngine(t, "mysql", engine)
	user := transport.SuspensionDatabaseUser{Engine: "mysql", Username: "shop_user"}
	if response := rotateDatabaseUser(t, transport.RotateDatabaseUserPasswordRequest{User: user, NewPassword: "Zq7xN2kLp9Vt4RmW"}); !response.Absent || response.PasswordChanged {
		t.Fatalf("absent %+v", response)
	}
	for _, bad := range []transport.RotateDatabaseUserPasswordRequest{
		{User: user, NewPassword: "short"},
		{User: user, NewPassword: "line\nbreak-long-enough"},
		{User: transport.SuspensionDatabaseUser{Engine: "mysql", Username: "root"}, NewPassword: "Zq7xN2kLp9Vt4RmW"},
		{User: user, NewPassword: "Zq7xN2kLp9Vt4RmW", App: &transport.DatabaseUserApp{Kind: "drupal", SubscriptionID: 1, DomainID: 1, SiteUsername: "s", DBName: "d"}},
	} {
		engine.statements = nil
		response := rotateDatabaseUser(t, bad)
		if response.Applied || response.PasswordChanged || len(engine.statements) != 0 || !strings.Contains(response.Error, "nothing was changed") {
			t.Fatalf("accepted %+v: %+v %q", bad.User, response, engine.statements)
		}
	}
}
