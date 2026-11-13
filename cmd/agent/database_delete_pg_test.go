package main

import (
	"strings"
	"testing"
)

// alpha.133 round 5 (W1): deleting a PostgreSQL database through the Agent
// ends the sessions open on it first, in the same psql run, as the panel's
// driver always did; a plain DROP failed while a site held a connection. The
// name is compared as a value and dropped as an identifier. No user is
// dropped when the request names none (the panel removes a database's own
// users itself, one by one).
func TestAgentPostgreSQLDeleteEndsOpenSessionsBeforeTheDrop(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	got := recordPGStatements(t)
	var resp DeleteDatabaseResponse
	if err := (&Agent{}).DeleteDatabase(DeleteDatabaseRequest{Type: "postgresql", Name: "shop", Port: 5433}, &resp); err != nil || !resp.Success {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if len(*got) != 1 {
		t.Fatalf("statements = %q", *got)
	}
	script := (*got)[0]
	terminate := strings.Index(script, "count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE datname = 'shop' AND pid <> pg_backend_pid();")
	drop := strings.Index(script, `DROP DATABASE IF EXISTS "shop";`)
	if !strings.HasPrefix(script, "5433 ") || terminate < 0 || drop < 0 || terminate > drop {
		t.Fatalf("script = %q", script)
	}
	if strings.Contains(script, "BEGIN") {
		t.Fatal("DROP DATABASE cannot run in a transaction block")
	}
}
