package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.149 security fix (second review of PR #370, finding 2): no Agent call
// drops, empties, loads into or changes access to the engine's own system
// database on the panel's word (dropping MariaDB's mysql removes every
// account; dropping PostgreSQL's postgres breaks the Agent's own psql).

func TestNoAgentCallDropsOrEmptiesASystemDatabase(t *testing.T) {
	e := &guardEngine{state: "ok"}
	installGuardEngine(t, e)
	systems := map[string][]string{"mysql": {"mysql", "MySQL", "sys", "performance_schema"}, "postgresql": {"postgres", "template1"}}
	for engine, names := range systems {
		for _, name := range names {
			var deleted transport.DeleteDatabaseResponse
			if err := (&Agent{}).DeleteDatabase(transport.DeleteDatabaseRequest{Type: engine, Name: name, User: "shop_user"}, &deleted); err != nil {
				t.Fatal(err)
			}
			if deleted.Success || deleted.AccountRefused != transport.DatabaseAccountRefusedSystemDatabase {
				t.Errorf("DeleteDatabase %s %s: %+v", engine, name, deleted)
			}
			var dropped transport.DropDatabaseOnceResponse
			if err := (&Agent{}).DropDatabaseOnceV1(&transport.DropDatabaseOnceRequest{Type: engine, Name: name, ByName: true}, &dropped); err != nil {
				t.Fatal(err)
			}
			if dropped.Dropped || !strings.Contains(dropped.Error, "system database") {
				t.Errorf("DropDatabaseOnceV1 %s %s: %+v", engine, name, dropped)
			}
			var created transport.CreateDatabaseResponse
			if err := (&Agent{}).CreateDatabase(transport.CreateDatabaseRequest{Type: engine, Name: name, User: "shop_user", Password: "Zq7xN2kLp9Vt4RmW"}, &created); err != nil {
				t.Fatal(err)
			}
			if created.Success || created.AccountRefused != transport.DatabaseAccountRefusedSystemDatabase {
				t.Errorf("CreateDatabase %s %s: %+v", engine, name, created)
			}
			// Backup, restore, import (with "empty first"), export, copy and
			// the database tools all validate their database here.
			if _, err := validateDatabaseIdentity(backupspec.DatabaseIdentity{ID: 1, Name: name, Type: engine}); !errors.Is(err, errSystemDatabase) {
				t.Errorf("validateDatabaseIdentity %s %s: %v", engine, name, err)
			}
		}
	}
	if err := mariadbEmptyDatabase("mysql"); !errors.Is(err, errSystemDatabase) {
		t.Errorf("emptying mysql: %v", err)
	}
	if len(e.statements) != 0 {
		t.Fatalf("a system database reached the engine: %q", e.statements)
	}
	var closed transport.PostgreSQLPublicAccessCloseResponse
	if err := (&Agent{}).PostgreSQLPublicAccessCloseV1(&transport.PostgreSQLPublicAccessCloseRequest{Port: 5432, Name: "postgres"}, &closed); err != nil {
		t.Fatal(err)
	}
	var undone transport.PostgreSQLPublicAccessUndoResponse
	if err := (&Agent{}).PostgreSQLPublicAccessUndoV1(&transport.PostgreSQLPublicAccessUndoRequest{Port: 5432, Name: "template1"}, &undone); err != nil {
		t.Fatal(err)
	}
	if closed.Closed || closed.Unknown || !strings.Contains(closed.Error, "system database") ||
		undone.Undone || undone.Unknown || !strings.Contains(undone.Error, "system database") {
		t.Fatalf("public access: %+v %+v", closed, undone)
	}
	// A site's database still passes.
	if _, err := validateDatabaseIdentity(backupspec.DatabaseIdentity{ID: 1, Name: "shop_db", Type: "mariadb"}); err != nil {
		t.Fatalf("a site database: %v", err)
	}
}
