package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Second review of PR #370, finding 7 (decision 6 of the second round): a
// MySQL server in MariaDB's place has no mysql.global_priv, so the MariaDB
// rule cannot be read there. The refusal says so (unsupported_engine), and an
// account the administrator already removed by hand is absent, so deleting
// the database or the site then goes through.

var probedUserRe = regexp.MustCompile(`FROM mysql\.user WHERE User = '([A-Za-z0-9_]+)'`)

// mysqlInPlaceEngine is MySQL 8: the MariaDB tables are missing; present
// names the accounts that exist; other answers everything else.
type mysqlInPlaceEngine struct {
	present    map[string]bool
	statements []string
	other      func(statement string) ([]byte, error)
}

func (e *mysqlInPlaceEngine) run(statement string) ([]byte, error) {
	e.statements = append(e.statements, statement)
	switch {
	case isEngineProbe(statement):
		count := "0"
		if match := probedUserRe.FindStringSubmatch(statement); match != nil && e.present[match[1]] {
			count = "1"
		}
		return []byte("engine=other\npresent=" + count + "\n"), nil
	case strings.Contains(statement, "mysql.global_priv"), strings.Contains(statement, "BEGIN NOT ATOMIC"):
		return []byte("ERROR 1146 (42S02) at line 1: Table 'mysql.global_priv' doesn't exist"), errors.New("exit status 1")
	}
	if e.other != nil {
		return e.other(statement)
	}
	return nil, nil
}

func (e *mysqlInPlaceEngine) writes() []string {
	var out []string
	for _, s := range e.statements {
		if !strings.HasPrefix(s, "SELECT ") {
			out = append(out, s)
		}
	}
	return out
}

func installMySQLInPlace(t *testing.T, e *mysqlInPlaceEngine) {
	t.Helper()
	previous := mysqlExecStatement
	mysqlExecStatement = e.run
	t.Cleanup(func() { mysqlExecStatement = previous })
}

func TestMySQLInMariaDBsPlaceIsNamedNotUnreadable(t *testing.T) {
	e := &mysqlInPlaceEngine{present: map[string]bool{"shop_user": true}}
	installMySQLInPlace(t, e)
	if got := databaseAccountRefusalOf(readDatabaseAccountState("mysql", e.run, "shop_user", pgAccountOrdinary, false)); got != transport.DatabaseAccountRefusedUnsupportedEngine {
		t.Fatalf("a present account: %q", got)
	}
	if got := databaseAccountRefusalOf(readDatabaseAccountState("mysql", e.run, "gone_user", pgAccountOrdinary, true)); got != transport.DatabaseAccountRefusedAbsent {
		t.Fatalf("an account removed by hand: %q", got)
	}
	text := databaseAccountRefusalText("shop_user", transport.DatabaseAccountRefusedUnsupportedEngine)
	for _, want := range []string{"MySQL", "MariaDB only", "nothing was changed", "administrator"} {
		if !strings.Contains(text, want) {
			t.Errorf("the sentence lacks %q: %s", want, text)
		}
	}
	// On MariaDB an unreadable rule stays unreadable (the probe says mariadb).
	maria := func(statement string) ([]byte, error) {
		if isEngineProbe(statement) {
			return []byte("engine=mariadb\npresent=1\n"), nil
		}
		return nil, errors.New("exit status 1")
	}
	if got := databaseAccountRefusalOf(readDatabaseAccountState("mysql", maria, "shop_user", pgAccountOrdinary, false)); got != transport.DatabaseAccountRefusedUnreadable {
		t.Fatalf("MariaDB unreadable: %q", got)
	}
}

func TestOnMySQLInMariaDBsPlaceADeletionGoesThroughOnceTheAccountIsGone(t *testing.T) {
	// The account is still there: the deletion is refused, nothing written.
	e := &mysqlInPlaceEngine{present: map[string]bool{"shop_user": true}}
	installMySQLInPlace(t, e)
	var resp transport.DeleteDatabaseResponse
	if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: "mysql", Name: "shop", User: "shop_user"}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || resp.AccountRefused != transport.DatabaseAccountRefusedUnsupportedEngine || len(e.writes()) != 0 {
		t.Fatalf("present: %+v %q", resp, e.writes())
	}
	// The administrator removed it by hand: the database goes.
	e = &mysqlInPlaceEngine{}
	installMySQLInPlace(t, e)
	resp = transport.DeleteDatabaseResponse{}
	if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: "mysql", Name: "shop", User: "shop_user"}, &resp); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(e.writes(), "\n")
	if !resp.Success || resp.AccountRefused != "" || strings.Contains(all, "DROP USER") || !strings.Contains(all, "DROP DATABASE IF EXISTS") {
		t.Fatalf("removed by hand: %+v %q", resp, e.writes())
	}
	// The panel's deletion of a database (and of a site) then sweeps the
	// rights naming it: an owner removed by hand is no refusal, and one still
	// there (a user another database uses) is not taken on the panel's word
	// but no longer stops the deletion either.
	for _, present := range []bool{false, true} {
		e = &mysqlInPlaceEngine{present: map[string]bool{"shop_user": present}, other: func(string) ([]byte, error) { return nil, errors.New("stop here") }}
		installMySQLInPlace(t, e)
		var swept transport.DroppedDatabaseRightsResponse
		if err := (&Agent{}).RemoveDroppedDatabaseRightsV1(&transport.DroppedDatabaseRightsRequest{Database: "shop", Owners: []string{"shop_user"}}, &swept); err != nil {
			t.Fatal(err)
		}
		if swept.AccountRefused != "" || len(e.statements) < 3 {
			t.Fatalf("present=%v: the sweep was refused: %+v %q", present, swept, e.statements)
		}
	}
	// Removing an added user that is still there: refused with the code.
	e = &mysqlInPlaceEngine{present: map[string]bool{"shop_reader": true}, other: func(statement string) ([]byte, error) {
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return []byte("host=localhost\n"), nil
		}
		return nil, nil
	}}
	installMySQLInPlace(t, e)
	var removed transport.DatabaseAccessUserResponse
	if err := (&Agent{}).RemoveDatabaseAccessUserV1(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "shop_reader"}, &removed); err != nil {
		t.Fatal(err)
	}
	if removed.Applied || removed.AccountRefused != transport.DatabaseAccountRefusedUnsupportedEngine || len(e.writes()) != 0 {
		t.Fatalf("an added user on MySQL: %+v %q", removed, e.writes())
	}
}

func TestOnMySQLInMariaDBsPlaceSuspensionSaysSo(t *testing.T) {
	for _, present := range []bool{true, false} {
		e := &mysqlInPlaceEngine{present: map[string]bool{"shop_user": present}}
		installMySQLInPlace(t, e)
		response := setDatabaseSuspension(t, "mysql", true, transport.DatabaseUserState{Present: true})
		if present && (response.AccountRefused != transport.DatabaseAccountRefusedUnsupportedEngine || response.Applied || len(e.writes()) != 0) {
			t.Fatalf("present: %+v %q", response, e.writes())
		}
		if !present && (response.AccountRefused != "" || response.Applied || !strings.Contains(response.Error, "absent")) {
			t.Fatalf("gone: %+v", response)
		}
	}
}
