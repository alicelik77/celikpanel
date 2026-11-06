package main

import (
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// A PostgreSQL restore into a database that still holds its tables must not
// stop at "relation already exists" (alpha.127 lab): the session becomes the
// database's owner and drops that owner's objects before the dump runs, and
// the dump is read from stdin after that prelude in the same session.
func TestPostgreSQLRestoreClearsTheOwnersObjectsFirst(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	restore, err := databaseRestoreCommand(backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql"})
	if err != nil {
		t.Fatal(err)
	}
	args := restore.Args
	if len(args) < 4 || args[len(args)-4] != "--command" || args[len(args)-3] != pgRestorePrelude || args[len(args)-2] != "--file" || args[len(args)-1] != "-" {
		t.Fatalf("restore argv = %v", args)
	}
	for _, want := range []string{"pg_get_userbyid(datdba)", "rolsuper", "SET ROLE %I", "DROP OWNED BY CURRENT_USER"} {
		if !strings.Contains(pgRestorePrelude, want) {
			t.Fatalf("prelude lacks %q", want)
		}
	}
	if strings.Contains(pgRestorePrelude, "DROP DATABASE") || strings.Contains(pgRestorePrelude, "DROP SCHEMA") {
		t.Fatal("the prelude must drop only the owner's objects")
	}
}
