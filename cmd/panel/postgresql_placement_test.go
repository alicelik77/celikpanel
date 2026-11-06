package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.127, Boston 6 Nov 2026: the subscription's PostgreSQL record said
// 5432 (the engine type's default), where the cluster of the removed
// PostgreSQL 16 is kept without its program; PostgreSQL 18 runs on 5433.

func bostonClusters() transport.PostgreSQLClustersResponse {
	return transport.PostgreSQLClustersResponse{Supported: true, Clusters: []transport.PostgreSQLCluster{
		{Version: "16", Name: "main", Port: 5432, Status: "down,binaries_missing", Online: false, Owner: "postgres", DataDirectory: "/var/lib/postgresql/16/main"},
		{Version: "18", Name: "main", Port: 5433, Status: "online", Online: true, Owner: "postgres", DataDirectory: "/var/lib/postgresql/18/main"},
	}}
}

// bostonRecord makes a subscription with Boston's record: PostgreSQL, port
// 5432, cluster NULL, version from the service scan, no account.
func bostonRecord(t *testing.T, db *sql.DB, userID int) (subscription, server int) {
	t.Helper()
	exec := func(query string, args ...any) int {
		result, err := db.Exec(query, args...)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		id, _ := result.LastInsertId()
		return int(id)
	}
	subscription = exec(`INSERT INTO subscriptions (owner_id, name) VALUES (?, 'boston')`, userID)
	server = exec(`INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, is_default, status)
		SELECT ?, id, 'PostgreSQL', '18.6', 'localhost', 5432, 0, 'active' FROM database_server_types WHERE name = 'postgresql'`, subscription)
	return subscription, server
}

func pgRecordState(t *testing.T, db *sql.DB, id int) (port int, cluster, status string) {
	t.Helper()
	if err := db.QueryRow(`SELECT port, COALESCE(cluster, ''), COALESCE(status, 'active') FROM database_servers WHERE id = ?`, id).Scan(&port, &cluster, &status); err != nil {
		t.Fatal(err)
	}
	return port, cluster, status
}

func TestANewPostgreSQLDatabaseGoesToTheRunningClusterNotTheRemovedOne(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	db := database.GetDB()
	subscription, server := bostonRecord(t, db, userID)
	provisioned := 0
	previous := provisionPGTargetAccount
	provisionPGTargetAccount = func(*Panel, context.Context, int) error { provisioned++; return nil }
	t.Cleanup(func() { provisionPGTargetAccount = previous })

	id, port, refusal, err := panel.pgPlaceNewDatabase(context.Background(), subscription, bostonClusters())
	if err != nil || refusal != nil || id != server || port != 5433 {
		t.Fatalf("id=%d port=%d refusal=%+v err=%v", id, port, refusal, err)
	}
	if port, cluster, _ := pgRecordState(t, db, server); port != 5433 || cluster != "18/main" {
		t.Fatalf("record = %d %q", port, cluster)
	}
	if provisioned != 1 {
		t.Fatalf("the panel's own account was asked for %d times on the running cluster, want once", provisioned)
	}
	// Placed once: the next create changes nothing.
	if id, port, _, err := panel.pgPlaceNewDatabase(context.Background(), subscription, bostonClusters()); err != nil || id != server || port != 5433 || provisioned != 1 {
		t.Fatalf("second placement id=%d port=%d provisioned=%d err=%v", id, port, provisioned, err)
	}
}

// A record that holds a database lives where that database lives: it is
// never moved; the new database gets a record of the running cluster.
func TestARecordHoldingADatabaseOnTheKeptClusterIsNeverMoved(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	db := database.GetDB()
	subscription, server := bostonRecord(t, db, userID)
	if _, err := db.Exec(`INSERT INTO databases_v2 (server_id, subscription_id, name) VALUES (?, ?, 'old_shop')`, server, subscription); err != nil {
		t.Fatal(err)
	}
	previous := provisionPGTargetAccount
	provisionPGTargetAccount = func(*Panel, context.Context, int) error { return nil }
	t.Cleanup(func() { provisionPGTargetAccount = previous })

	id, port, refusal, err := panel.pgPlaceNewDatabase(context.Background(), subscription, bostonClusters())
	if err != nil || refusal != nil || id == server || port != 5433 {
		t.Fatalf("id=%d port=%d refusal=%+v err=%v", id, port, refusal, err)
	}
	if port, cluster, status := pgRecordState(t, db, server); port != 5432 || cluster != "" || status != "active" {
		t.Fatalf("the record holding old_shop changed: %d %q %s", port, cluster, status)
	}
	if port, cluster, _ := pgRecordState(t, db, id); port != 5433 || cluster != "18/main" {
		t.Fatalf("new record = %d %q", port, cluster)
	}
}

func TestWithNoRunningClusterTheCreateIsRefusedWithTheFacts(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	db := database.GetDB()
	subscription, server := bostonRecord(t, db, userID)
	onlyRemoved := bostonClusters()
	onlyRemoved.Clusters = onlyRemoved.Clusters[:1]

	_, _, refusal, err := panel.pgPlaceNewDatabase(context.Background(), subscription, onlyRemoved)
	want := transport.DatabaseClusterRefusal{Reason: "removed", Version: "16", Cluster: "16/main", Port: 5432, DataDirectory: "/var/lib/postgresql/16/main"}
	if err != nil || refusal == nil || *refusal != want {
		t.Fatalf("refusal=%+v err=%v", refusal, err)
	}
	if port, cluster, _ := pgRecordState(t, db, server); port != 5432 || cluster != "" {
		t.Fatalf("record changed: %d %q", port, cluster)
	}

	stopped := bostonClusters()
	stopped.Clusters[1].Online, stopped.Clusters[1].Status = false, "down"
	stopped.Clusters = stopped.Clusters[1:]
	if _, _, refusal, _ := panel.pgPlaceNewDatabase(context.Background(), subscription, stopped); refusal == nil || refusal.Reason != "stopped" || refusal.Version != "18" {
		t.Fatalf("stopped refusal = %+v", refusal)
	}

	recorder := httptest.NewRecorder()
	writeDatabaseClusterRefusal(recorder, serviceStateAtomicRequest(http.MethodPost, "/api/v1/domains/1/databases", "", userID), refusal)
	var body apiErrorBody
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || body.Code != errCodePGClusterUnavailable || body.Reason != "removed" ||
		body.Vars["version"] != "16" || body.Vars["port"] != "5432" || body.Vars["data"] != "/var/lib/postgresql/16/main" ||
		body.Vars["unit"] != "postgresql@16-main" || body.Action != "/components/postgresql" {
		t.Fatalf("answer = %d %+v", recorder.Code, body)
	}
}

// Without postgresql-common (one PostgreSQL) the record is used as it is.
func TestWithoutClustersTheRecordIsUsedAsItIs(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	subscription, server := bostonRecord(t, database.GetDB(), userID)
	id, port, refusal, err := panel.pgPlaceNewDatabase(context.Background(), subscription, transport.PostgreSQLClustersResponse{})
	if err != nil || refusal != nil || id != server || port != 5432 {
		t.Fatalf("id=%d port=%d refusal=%+v err=%v", id, port, refusal, err)
	}
}

// A PostgreSQL database's backup names the cluster its record says; MariaDB
// names none.
func TestABackupIdentityNamesThePostgreSQLRecordsPort(t *testing.T) {
	pg := withBackupCluster(backupspec.DatabaseIdentity{ID: 1, Name: "shop", Type: "postgresql"}, 5433)
	if pg.ClusterPort != 5433 || pg.ClusterVersion != "" {
		t.Fatalf("postgresql = %+v", pg)
	}
	if maria := withBackupCluster(backupspec.DatabaseIdentity{ID: 2, Name: "blog", Type: "mariadb"}, 3306); maria.ClusterPort != 0 {
		t.Fatalf("mariadb = %+v", maria)
	}
	// The backup refusal is the same refusal the database page says.
	refusal := transport.DatabaseClusterRefusal(backupspec.ClusterRefusal{Reason: "major_mismatch", Version: "18", Cluster: "18/main", Port: 5433, BackupVersion: "16"})
	recorder := httptest.NewRecorder()
	writeDatabaseClusterRefusal(recorder, httptest.NewRequest(http.MethodPost, "/", nil), &refusal)
	var body apiErrorBody
	_ = json.NewDecoder(recorder.Body).Decode(&body)
	if recorder.Code != http.StatusConflict || body.Reason != "major_mismatch" || body.Vars["backup_version"] != "16" || body.Vars["version"] != "18" || body.Action != "" {
		t.Fatalf("answer = %d %+v", recorder.Code, body)
	}
}

// Remote-backup and WordPress-tools restores name the PostgreSQL
// prerequisite (alpha.127): each reason has its own code, the administrator
// acts, and the next step is said.
func TestARestoreRefusalHasItsOwnGuidanceCode(t *testing.T) {
	for reason, code := range map[string]string{"removed": "PG_CLUSTER_REMOVED", "stopped": "PG_CLUSTER_STOPPED", "none": "PG_CLUSTER_NONE",
		"major_mismatch": "PG_MAJOR_MISMATCH", "unplaced": "PG_CLUSTER_UNPLACED"} {
		if got := pgClusterRefusalCodeSuffix(reason); got != code {
			t.Fatalf("%s -> %s", reason, got)
		}
		if remoteFailureWho(code, false) != "administrator" || remoteNextAction(code) == remoteNextAction("SOMETHING_ELSE") {
			t.Fatalf("%s: who=%s next=%q", code, remoteFailureWho(code, false), remoteNextAction(code))
		}
	}
	err := error(&pgClusterRefusalError{refusal: transport.DatabaseClusterRefusal{Reason: "major_mismatch", Version: "18", Cluster: "18/main", Port: 5433, BackupVersion: "16"}})
	if !strings.Contains(err.Error(), "PostgreSQL 16") || !strings.Contains(err.Error(), "port 5433") {
		t.Fatalf("message = %q", err.Error())
	}
}
