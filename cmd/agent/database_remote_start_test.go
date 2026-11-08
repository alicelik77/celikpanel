package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 lab (release recovery): a clean apply records the digest of the
// wanted set and the rules in force; the status reads them back, so a
// restarted panel can send nothing. A narrowing (lease-free) clears it.
func TestDatabaseRemoteRecordsTheAppliedSetForTheStatus(t *testing.T) {
	fakeFilterHost(t)
	f := &fakeMariaDB{accounts: map[string]bool{"shop_u@localhost": true}}
	withFakeMariaDB(t, f)
	databaseRemoteFail2ban.jailActive = func(name string) bool { return name == dbremote.MariaDBJailName }
	target := dbremote.Target{Engine: "mysql", Port: 3306, Rules: []dbremote.Rule{{Database: "shop", User: "shop_u", Address: "203.0.113.7/32"}}}
	var resp transport.DatabaseRemoteResponse
	applyDatabaseRemote([]dbremote.Target{target}, "", nil, &resp)
	if resp.Error != "" {
		t.Fatalf("apply: %s", resp.Error)
	}
	state, err := loadDatabaseRemoteState()
	if err != nil {
		t.Fatal(err)
	}
	want := dbremote.RequestDigest([]dbremote.Target{target}, false)
	if state.Applied != want || len(state.Targets["mysql:3306"].Active) != len(resp.Targets[0].Active) {
		t.Fatalf("record: applied %q (want %q), active %+v, answer %+v", state.Applied, want, state.Targets["mysql:3306"].Active, resp.Targets)
	}
	var status transport.DatabaseRemoteResponse
	if err := (&Agent{}).DatabaseRemoteStatusV1(&transport.DatabaseRemoteStatusRequest{Targets: []transport.DatabaseRemoteTarget{{Engine: "mysql", Port: 3306}}}, &status); err != nil {
		t.Fatal(err)
	}
	if status.AppliedDigest != want || len(status.Targets) != 1 || len(status.Targets[0].Active) != len(state.Targets["mysql:3306"].Active) {
		t.Fatalf("status: %+v", status)
	}
	// The agreement on MariaDB's name lookup is part of the set.
	if dbremote.RequestDigest([]dbremote.Target{target}, true) == want {
		t.Fatal("the agreement does not change the digest")
	}
	// A narrowing clears it (the record no longer is a full apply).
	if _, err := os.Stat(databaseRemoteFilterFiles.nftPath); err != nil {
		t.Fatalf("test setup: no filter file: %v", err)
	}
	var narrowed transport.DatabaseRemoteResponse
	narrowDatabaseRemoteFilter(nil, &narrowed)
	state, _ = loadDatabaseRemoteState()
	if state.Applied != "" || len(state.Targets["mysql:3306"].Active) != 0 || narrowed.Error != "" {
		t.Fatalf("after narrowing: %+v, %s", state, narrowed.Error)
	}
}

// The narrowing takes no service-mutation lease.
func TestNarrowingTakesNoServiceMutationLease(t *testing.T) {
	source, err := os.ReadFile("database_remote_narrow.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "requiredServiceMutationStep(") {
		t.Fatal("the narrowing takes a service-mutation lease; a release recovery's idle check would wait on it")
	}
}
