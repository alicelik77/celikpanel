package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Boston, 6 Nov 2026, pg_lsclusters word for word: PostgreSQL 16 removed with
// its data kept (still configured for 5432), PostgreSQL 18 installed after it
// on the next free port.
const bostonLsclusters = `Ver Cluster Port Status                Owner    Data directory              Log file
16  main    5432 down,binaries_missing postgres /var/lib/postgresql/16/main /var/log/postgresql/postgresql-16-main.log
18  main    5433 online                postgres /var/lib/postgresql/18/main /var/log/postgresql/postgresql-18-main.log
`

func usePGStatementClusters(t *testing.T, lsclusters string) {
	t.Helper()
	previous := pgStatementClusters
	pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) {
		return parsePGLsClusters(lsclusters), true
	}
	t.Cleanup(func() { pgStatementClusters = previous })
}

func recordPGStatements(t *testing.T) *[]string {
	t.Helper()
	var got []string
	previousPort, previousDefault := postgreSQLExecStatementOnPort, postgreSQLExecStatement
	postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
		got = append(got, strconv.Itoa(port)+" "+statement)
		return nil, nil
	}
	postgreSQLExecStatement = func(statement string) ([]byte, error) {
		got = append(got, "default "+statement)
		return nil, nil
	}
	t.Cleanup(func() { postgreSQLExecStatementOnPort, postgreSQLExecStatement = previousPort, previousDefault })
	return &got
}

// The record said 5432: nothing runs there, and the create is refused with
// the cluster's facts before a single statement, not "internal error".
func TestAStatementForARemovedVersionsClusterIsRefusedWithItsFacts(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	got := recordPGStatements(t)
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5432}, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Success || len(*got) != 0 {
		t.Fatalf("resp=%+v statements=%v", resp, *got)
	}
	want := transport.DatabaseClusterRefusal{Reason: "removed", Version: "16", Cluster: "16/main", Port: 5432,
		DataDirectory: "/var/lib/postgresql/16/main", RunningVersion: "18", RunningPort: 5433}
	if resp.Refusal == nil || *resp.Refusal != want {
		t.Fatalf("refusal = %+v", resp.Refusal)
	}
	if strings.Contains(resp.Error, "pw") || !strings.Contains(resp.Error, "program was removed") {
		t.Fatalf("error = %q", resp.Error)
	}
	var deleted DeleteDatabaseResponse
	if err := (&Agent{}).DeleteDatabase(DeleteDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Port: 5432}, &deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.Success || deleted.Refusal == nil || deleted.Refusal.Reason != "removed" || len(*got) != 0 {
		t.Fatalf("delete = %+v statements=%v", deleted, *got)
	}
}

func TestAStatementGoesToTheRunningClusterItNames(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	got := recordPGStatements(t)
	var resp CreateDatabaseResponse
	if err := (&Agent{}).CreateDatabase(CreateDatabaseRequest{Type: "postgresql", Name: "shop", User: "shop_user", Password: "pw", Port: 5433}, &resp); err != nil || !resp.Success {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if len(*got) != 2 || !strings.HasPrefix((*got)[0], "5433 CREATE USER") || !strings.HasPrefix((*got)[1], "5433 CREATE DATABASE") {
		t.Fatalf("statements = %v", *got)
	}
}

// No port: psql's own default would pick 16/main (it is configured for
// 5432); the running cluster is chosen instead.
func TestAStatementWithoutAPortGoesToTheRunningCluster(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	got := recordPGStatements(t)
	var resp RemoveDatabaseAdminAccountResponse
	if err := (&Agent{}).RemoveDatabaseAdminAccount(RemoveDatabaseAdminAccountRequest{Engine: "postgresql"}, &resp); err != nil || !resp.Removed {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if len(*got) != 1 || !strings.HasPrefix((*got)[0], "5433 DROP ROLE") {
		t.Fatalf("statements = %v", *got)
	}
	// The running cluster on the default port is psql's own choice, kept.
	usePGStatementClusters(t, "17 main 5432 online postgres /var/lib/postgresql/17/main log\n18 main 5433 online postgres /var/lib/postgresql/18/main log\n")
	if port, err := resolvePGStatementPort(0); err != nil || port != 5432 {
		t.Fatalf("port=%d err=%v", port, err)
	}
}

func TestNothingRunningIsRefusedAndAStoppedClusterSaysStopped(t *testing.T) {
	usePGStatementClusters(t, "18 main 5433 down postgres /var/lib/postgresql/18/main log\n")
	_, err := resolvePGStatementPort(0)
	refusal := pgClusterRefusalOf(err)
	if refusal == nil || refusal.Reason != "stopped" || refusal.Port != 5433 || refusal.RunningVersion != "" {
		t.Fatalf("refusal = %+v err=%v", refusal, err)
	}
	usePGStatementClusters(t, bostonLsclusters[strings.Index(bostonLsclusters, "\n")+1:strings.Index(bostonLsclusters, "\n18")+1])
	if refusal := pgClusterRefusalOf(func() error { _, err := resolvePGStatementPort(0); return err }()); refusal == nil || refusal.Reason != "removed" {
		t.Fatalf("only the removed cluster: %+v", refusal)
	}
}

// Where the clusters cannot be read (no postgresql-common), nothing changes:
// the port as asked, 0 for psql's own default.
func TestWithoutClustersTheStatementGoesWhereItWasAsked(t *testing.T) {
	previous := pgStatementClusters
	pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) { return nil, false }
	t.Cleanup(func() { pgStatementClusters = previous })
	for _, port := range []int{0, 5432, 5433} {
		if got, err := resolvePGStatementPort(port); err != nil || got != port {
			t.Fatalf("port %d -> %d %v", port, got, err)
		}
	}
	// A port no postgresql-common cluster has is left to psql.
	usePGStatementClusters(t, bostonLsclusters)
	if got, err := resolvePGStatementPort(6000); err != nil || got != 6000 {
		t.Fatalf("6000 -> %d %v", got, err)
	}
}

// A removal looks at the ports the clusters run on, not only the
// catalogue's 5432; one major's removal at that major's port.
func TestPostgreSQLRemovalTracesLookAtTheClustersPorts(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	svc := &core.ManagedService{ID: "postgresql"}
	if got := newRemovalTraceScope(svc, "", nil).facts.Ports; !intsEqual(got, []int{5432, 5433}) {
		t.Fatalf("whole = %v", got)
	}
	if got := newRemovalTraceScope(svc, "18", nil).facts.Ports; !intsEqual(got, []int{5433}) {
		t.Fatalf("18 = %v", got)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A PostgreSQL backup names no cluster: it goes to the running one, not to
// the removed version's kept cluster psql would pick without a port.
func TestAPostgreSQLBackupGoesToTheRunningCluster(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	identity := backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql"}
	dump, err := databaseDumpCommand(identity)
	if err != nil || !strings.HasSuffix(strings.Join(dump.Args, " "), "--dbname shop --port 5433") {
		t.Fatalf("dump = %v %v", dump.Args, err)
	}
	restore, err := databaseRestoreCommand(identity)
	if err != nil || !strings.Contains(strings.Join(restore.Args, " "), "--dbname shop --port 5433 --single-transaction --command ") {
		t.Fatalf("restore = %v %v", restore.Args, err)
	}
	usePGStatementClusters(t, bostonLsclusters[:strings.Index(bostonLsclusters, "\n18")+1])
	if _, err := databaseDumpCommand(identity); pgClusterRefusalOf(err) == nil {
		t.Fatalf("a dump with nothing running = %v", err)
	}
}
