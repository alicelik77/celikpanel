package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.129 integration: removing a read-only or read-write user also takes
// its remote copies out of the remote access record, so no copy of a removed
// user is listed (or recreated) afterwards; another user's copy stays.
func TestRemoveMySQLAccessUserTakesItsRemoteCopiesOutOfTheRecord(t *testing.T) {
	f := &fakeMariaDB{accounts: map[string]bool{"reader@203.0.113.7": true, "other@203.0.113.7": true}}
	withFakeMariaDB(t, f)
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"mysql:3306": {Engine: "mysql", Port: 3306, Listen: true, Accounts: []string{"reader@203.0.113.7", "other@203.0.113.7"}},
	}}); err != nil {
		t.Skipf("the record cannot be written here: %v", err)
	}
	previous := mysqlExecStatement
	t.Cleanup(func() { mysqlExecStatement = previous })
	var engine []string
	mysqlExecStatement = func(statement string) ([]byte, error) {
		engine = append(engine, statement)
		if strings.HasPrefix(statement, "SELECT CONCAT('host=', Host)") {
			return []byte("host=203.0.113.7\nhost=localhost\n"), nil
		}
		return nil, nil
	}
	var resp transport.DatabaseAccessUserResponse
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "reader"}, &resp)
	if !resp.Applied || resp.Error != "" {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.Contains(strings.Join(engine, "\n"), "DROP USER 'reader'@'203.0.113.7', 'reader'@'localhost'") {
		t.Fatalf("the remote copy was not dropped with the user: %v", engine)
	}
	state, err := loadDatabaseRemoteState()
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Targets["mysql:3306"].Accounts; len(got) != 1 || got[0] != "other@203.0.113.7" {
		t.Fatalf("record = %v", got)
	}
	if f.accounts["reader@203.0.113.7"] || !f.accounts["other@203.0.113.7"] {
		t.Fatalf("accounts = %v", f.accounts)
	}

	// A user already gone from the engine still leaves the record.
	if err := saveDatabaseRemoteState(dbremote.State{Targets: map[string]dbremote.TargetState{
		"mysql:3306": {Engine: "mysql", Port: 3306, Listen: true, Accounts: []string{"gone@203.0.113.7"}},
	}}); err != nil {
		t.Fatal(err)
	}
	mysqlExecStatement = func(string) ([]byte, error) { return nil, nil }
	resp = transport.DatabaseAccessUserResponse{}
	removeMySQLAccessUser(&transport.DatabaseAccessUserRequest{Engine: "mysql", Database: "shop", Username: "gone"}, &resp)
	if !resp.Absent || !resp.Applied {
		t.Fatalf("resp = %+v", resp)
	}
	if state, err = loadDatabaseRemoteState(); err != nil {
		t.Fatal(err)
	}
	if got := state.Targets["mysql:3306"].Accounts; len(got) != 0 {
		t.Fatalf("record = %v", got)
	}
}
