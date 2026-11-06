package main

import (
	"os"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
)

// alpha.127: a PostgreSQL backup is made from the database's cluster, which
// it records; a restore goes only to a running cluster of the same major.

func TestABackupIsMadeFromTheRecordsClusterAndRecordsIt(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	var dumped []backupspec.DatabaseIdentity
	previous := dumpDatabaseToFile
	dumpDatabaseToFile = func(database backupspec.DatabaseIdentity, destination string) error {
		dumped = append(dumped, database)
		return os.WriteFile(destination, []byte("dump"), 0o600)
	}
	t.Cleanup(func() { dumpDatabaseToFile = previous })

	var manifest backupManifest
	identity := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterPort: 5433}
	if err := buildDatabasePayload(t.TempDir(), identity, &manifest); err != nil {
		t.Fatal(err)
	}
	want := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterVersion: "18", ClusterPort: 5433}
	if len(dumped) != 1 || dumped[0] != want || manifest.Databases[0].Identity != want {
		t.Fatalf("dumped=%+v manifest=%+v", dumped, manifest.Databases)
	}
	if command, err := databaseDumpCommand(want); err != nil || !strings.HasSuffix(strings.Join(command.Args, " "), "--port 5433") {
		t.Fatalf("dump command = %v %v", command.Args, err)
	}

	// The record still says 5432: the removed version's cluster is refused,
	// and nothing is dumped.
	dumped = nil
	stale := identity
	stale.ClusterPort = 5432
	err := buildDatabasePayload(t.TempDir(), stale, &backupManifest{})
	if refusal := backupClusterRefusalOf(err); refusal == nil || refusal.Reason != "removed" || refusal.Version != "16" || len(dumped) != 0 {
		t.Fatalf("stale record: err=%v refusal=%+v dumped=%v", err, refusal, dumped)
	}

	// MariaDB names no cluster.
	mysql := backupspec.DatabaseIdentity{ID: 8, Name: "blog", Type: "mysql", ClusterPort: 3306}
	if _, err := validateDatabaseIdentity(mysql); err == nil {
		t.Fatal("a MariaDB identity with a cluster was accepted")
	}
}

func TestARestoreGoesOnlyToARunningClusterOfTheBackupsMajor(t *testing.T) {
	usePGStatementClusters(t, bostonLsclusters)
	request := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterPort: 5433}

	same := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterVersion: "18", ClusterPort: 5440}
	if target, err := restoreDatabaseTarget(same, request); err != nil || target.ClusterPort != 5433 || target.ClusterVersion != "18" {
		t.Fatalf("same major, its port moved: %+v %v", target, err)
	}
	if command, _ := databaseRestoreCommand(backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterVersion: "18", ClusterPort: 5433}); !strings.Contains(strings.Join(command.Args, " "), "--port 5433 --command ") {
		t.Fatalf("restore command = %v", command.Args)
	}

	older := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterVersion: "16", ClusterPort: 5432}
	_, err := restoreDatabaseTarget(older, request)
	refusal := backupClusterRefusalOf(err)
	if refusal == nil || refusal.Reason != "major_mismatch" || refusal.BackupVersion != "16" || refusal.Version != "18" || refusal.Port != 5433 {
		t.Fatalf("a 16 dump into 18: err=%v refusal=%+v", err, refusal)
	}

	// Made before alpha.127 (no major): the database's cluster now, as before.
	legacy := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql"}
	if target, err := restoreDatabaseTarget(legacy, request); err != nil || target.ClusterPort != 5433 {
		t.Fatalf("legacy: %+v %v", target, err)
	}
	if target, err := restoreDatabaseTarget(legacy, legacy); err != nil || target.ClusterPort != 5433 {
		t.Fatalf("legacy request without a port: %+v %v", target, err)
	}

	// The database's record names the removed cluster: refused, nothing runs.
	onRemoved := request
	onRemoved.ClusterPort = 5432
	if refusal := backupClusterRefusalOf(func() error { _, err := restoreDatabaseTarget(same, onRemoved); return err }()); refusal == nil || refusal.Reason != "removed" {
		t.Fatalf("removed target: %+v", refusal)
	}
	// A port no cluster is on, with a major to keep: refused.
	nowhere := request
	nowhere.ClusterPort = 6000
	if refusal := backupClusterRefusalOf(func() error { _, err := restoreDatabaseTarget(same, nowhere); return err }()); refusal == nil || refusal.Reason != "unplaced" || refusal.BackupVersion != "18" {
		t.Fatalf("unplaced target: %+v", refusal)
	}

	// The same database whichever cluster each side names; another one is not.
	if _, err := validateRestoreDatabase(older, request); err != nil {
		t.Fatalf("same database refused: %v", err)
	}
	other := request
	other.ID = 9
	if _, err := validateRestoreDatabase(older, other); err == nil {
		t.Fatal("another database was accepted")
	}
}

// Without postgresql-common the backup identity is not touched: one
// PostgreSQL, psql's own default, as before.
func TestWithoutClustersABackupIdentityIsAsBefore(t *testing.T) {
	identity := backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql"}
	if got, err := resolveBackupCluster(identity); err != nil || got != identity {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := restoreDatabaseTarget(backupspec.DatabaseIdentity{ID: 7, Name: "shop", Type: "postgresql", ClusterVersion: "16"}, identity); err != nil || got.ClusterPort != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}
