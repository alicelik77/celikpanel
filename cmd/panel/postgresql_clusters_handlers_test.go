package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.108: PostgreSQL majors side by side. The panel's record of where a
// database lives changes only after the Agent's proof; a move's record switch
// is its own step after the copy's proof, reversible until the source is
// dropped; the drop is explicit; every step is audited.

func pgTestClusters(onTarget bool) transport.PostgreSQLClustersResponse {
	target := []string{}
	if onTarget {
		target = []string{"shop"}
	}
	return transport.PostgreSQLClustersResponse{Supported: true, Clusters: []transport.PostgreSQLCluster{
		{Version: "17", Name: "main", Port: 5432, Status: "online", Online: true, Owner: "postgres", Databases: []string{"shop"}, DatabasesKnown: true},
		{Version: "18", Name: "main", Port: 5433, Status: "online", Online: true, Owner: "postgres", Databases: target, DatabasesKnown: true},
	}, Offers: []transport.PostgreSQLMajorOffer{}, UsedPorts: []int{5432, 5433}}
}

// pgTestRecords makes a subscription with one PostgreSQL record (as before
// schema 46: cluster NULL, port 5432) holding database shop with its user.
func pgTestRecords(t *testing.T, db *sql.DB, userID int) (serverID, databaseID int) {
	t.Helper()
	exec := func(query string, args ...any) int {
		result, err := db.Exec(query, args...)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		id, _ := result.LastInsertId()
		return int(id)
	}
	subscription := exec(`INSERT INTO subscriptions (owner_id, name) VALUES (?, 'pg')`, userID)
	domain := exec(`INSERT INTO domains (subscription_id, name, status) VALUES (?, 'shop.example.test', 'active')`, subscription)
	serverID = exec(`INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, is_default, status)
		SELECT ?, id, 'PostgreSQL', '17', 'localhost', 5432, 1, 'active' FROM database_server_types WHERE name = 'postgresql'`, subscription)
	databaseID = exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, 'shop')`, serverID, subscription, domain)
	user := exec(`INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, 'shop_user', 'sealed')`, serverID, subscription)
	exec(`INSERT INTO database_user_grants (database_id, user_id) VALUES (?, ?)`, databaseID, user)
	return serverID, databaseID
}

func pgStubAgent(t *testing.T, clusters *transport.PostgreSQLClustersResponse) {
	t.Helper()
	restoreRead, restorePort, restoreCopy, restoreDrop, restoreAccount := readPGClustersFromAgent, setPGPortThroughAgent, copyPGThroughAgent, dropPGThroughAgent, provisionPGTargetAccount
	t.Cleanup(func() {
		readPGClustersFromAgent, setPGPortThroughAgent, copyPGThroughAgent, dropPGThroughAgent, provisionPGTargetAccount = restoreRead, restorePort, restoreCopy, restoreDrop, restoreAccount
	})
	readPGClustersFromAgent = func(*Panel, context.Context) (transport.PostgreSQLClustersResponse, error) {
		if clusters == nil {
			return transport.PostgreSQLClustersResponse{}, errors.New("agent unreachable")
		}
		return *clusters, nil
	}
	provisionPGTargetAccount = func(*Panel, context.Context, int) error { return nil }
}

func pgCall(t *testing.T, panel *Panel, method, target, body string, userID int) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	panel.handlePostgreSQLService(recorder, serviceStateAtomicRequest(method, target, body, userID))
	return recorder
}

func TestPGClustersViewNamesThePanelsRecordsAndTiesOldRecords(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	serverID, _ := pgTestRecords(t, database.GetDB(), userID)
	pgStubAgent(t, nil)
	if recorder := pgCall(t, panel, http.MethodGet, pgClustersAPIPath, "", userID); !strings.Contains(recorder.Body.String(), `"state":"unknown"`) {
		t.Fatalf("unknown: %s", recorder.Body.String())
	}
	clusters := pgTestClusters(false)
	pgStubAgent(t, &clusters)
	recorder := pgCall(t, panel, http.MethodGet, pgClustersAPIPath, "", userID)
	var view pgClustersView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil || view.State != "read" {
		t.Fatalf("view: %v %s", err, recorder.Body.String())
	}
	if records := view.Records["17/main"]; len(records) != 1 || records[0].Name != "shop" || records[0].Port != 5432 || records[0].Users[0] != "shop_user" || records[0].Domain != "shop.example.test" {
		t.Fatalf("records = %+v", view.Records)
	}
	var cluster sql.NullString
	database.GetDB().QueryRow(`SELECT cluster FROM database_servers WHERE id = ?`, serverID).Scan(&cluster)
	if cluster.String != "17/main" {
		t.Fatalf("the record made before schema 46 was not tied to its cluster: %v", cluster)
	}
	// Changed outside CelikPanel: the cluster now runs on 5440.
	clusters.Clusters[0].Port = 5440
	recorder = pgCall(t, panel, http.MethodGet, pgClustersAPIPath, "", userID)
	json.Unmarshal(recorder.Body.Bytes(), &view)
	if len(view.Mismatches) != 1 || view.Mismatches[0].RecordPort != 5432 || view.Mismatches[0].ClusterPort != 5440 {
		t.Fatalf("mismatch = %+v", view.Mismatches)
	}
	recorder = pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/records", `{"version":"17","name":"main"}`, userID)
	var port int
	database.GetDB().QueryRow(`SELECT port FROM database_servers WHERE id = ?`, serverID).Scan(&port)
	if recorder.Code != http.StatusOK || port != 5440 {
		t.Fatalf("records sync: %d %s port=%d", recorder.Code, recorder.Body.String(), port)
	}
}

func TestPGPortChangeUpdatesTheRecordsOnlyAfterProof(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	serverID, _ := pgTestRecords(t, database.GetDB(), userID)
	clusters := pgTestClusters(false)
	pgStubAgent(t, &clusters)
	sha := strings.Repeat("a", 64)
	setPGPortThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLSetPortRequest) (transport.PostgreSQLSetPortResponse, error) {
		return transport.PostgreSQLSetPortResponse{From: 5432, To: req.Port, RolledBack: true, Restored: true, ErrorCode: "PROOF_FAILED", Error: "cluster 17/main did not answer on port 5440; the previous configuration was written back and it answers on port 5432 again"}, nil
	}
	recorder := pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/port", `{"version":"17","name":"main","port":5440,"expected_sha256":"`+sha+`"}`, userID)
	var port int
	database.GetDB().QueryRow(`SELECT port FROM database_servers WHERE id = ?`, serverID).Scan(&port)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"rolled_back":true`) || port != 5432 {
		t.Fatalf("rolled back: %d %s port=%d", recorder.Code, recorder.Body.String(), port)
	}
	if recorder := pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/port", `{"version":"17","name":"main","port":80,"expected_sha256":"`+sha+`"}`, userID); recorder.Code != http.StatusBadRequest {
		t.Fatalf("port 80: %d", recorder.Code)
	}
	setPGPortThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLSetPortRequest) (transport.PostgreSQLSetPortResponse, error) {
		if req.ExpectedSHA256 != sha || req.Version != "17" || req.Name != "main" {
			t.Fatalf("request = %+v", req)
		}
		return transport.PostgreSQLSetPortResponse{Changed: true, Proved: true, From: 5432, To: req.Port}, nil
	}
	recorder = pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/port", `{"version":"17","name":"main","port":5440,"expected_sha256":"`+sha+`"}`, userID)
	database.GetDB().QueryRow(`SELECT port FROM database_servers WHERE id = ?`, serverID).Scan(&port)
	if recorder.Code != http.StatusOK || port != 5440 || !strings.Contains(recorder.Body.String(), `"records":1`) {
		t.Fatalf("proved: %d %s port=%d", recorder.Code, recorder.Body.String(), port)
	}
	var action, details string
	database.GetDB().QueryRow(`SELECT action, details FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details)
	if action != "postgresql.cluster.port:17/main 5432->5440" || !strings.Contains(details, `"cluster":"17/main"`) {
		t.Fatalf("audit = %q %s", action, details)
	}
}

func TestPGMoveIsCopiedProvedSwitchedAsItsOwnStepAndDroppedExplicitly(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	sourceServer, databaseID := pgTestRecords(t, database.GetDB(), userID)
	clusters := pgTestClusters(false)
	pgStubAgent(t, &clusters)
	proof := transport.PostgreSQLCopyProof{Equal: true, TablesTotal: 2, Tables: []transport.PostgreSQLTableCount{{Schema: "public", Table: "orders", Source: 10, Target: 10}}, RolesCreated: []string{"shop_user"}, RolesKept: []string{}}
	copyPGThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLCopyRequest) (transport.PostgreSQLCopyResponse, error) {
		if req.Database != "shop" || req.SourcePort != 5432 || req.TargetPort != 5433 {
			t.Fatalf("copy request = %+v", req)
		}
		clusters.Clusters[1].Databases = []string{"shop"}
		return transport.PostgreSQLCopyResponse{Copied: true, Proof: proof}, nil
	}
	recorder := pgCall(t, panel, http.MethodPost, pgServiceAPIPrefix+"moves", `{"database":"shop","source_port":5432,"target_port":5433}`, userID)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"recorded":true`) {
		t.Fatalf("copy: %d %s", recorder.Code, recorder.Body.String())
	}
	var moveID, targetServer int
	var state, targetCluster string
	database.GetDB().QueryRow(`SELECT id, state, target_server_id FROM postgresql_database_moves`).Scan(&moveID, &state, &targetServer)
	database.GetDB().QueryRow(`SELECT cluster FROM database_servers WHERE id = ?`, targetServer).Scan(&targetCluster)
	var recordServer int
	database.GetDB().QueryRow(`SELECT server_id FROM databases_v2 WHERE id = ?`, databaseID).Scan(&recordServer)
	if state != "copied" || targetCluster != "18/main" || recordServer != sourceServer {
		t.Fatalf("after the copy: state %s, target %s, record on %d", state, targetCluster, recordServer)
	}
	// A second copy while this one is open is refused; the source cannot be dropped before the switch.
	if recorder := pgCall(t, panel, http.MethodPost, pgServiceAPIPrefix+"moves", `{"database":"shop","source_port":5432,"target_port":5433}`, userID); !strings.Contains(recorder.Body.String(), "PG_MOVE_ACTIVE") {
		t.Fatalf("second copy: %s", recorder.Body.String())
	}
	moves := pgServiceAPIPrefix + "moves/" + itoaTest(moveID)
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/drop-source", "", userID); !strings.Contains(recorder.Body.String(), "PG_MOVE_STATE") {
		t.Fatalf("drop before the switch: %s", recorder.Body.String())
	}
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/switch", "", userID); recorder.Code != http.StatusOK {
		t.Fatalf("switch: %d %s", recorder.Code, recorder.Body.String())
	}
	var grantUserServer int
	database.GetDB().QueryRow(`SELECT server_id FROM databases_v2 WHERE id = ?`, databaseID).Scan(&recordServer)
	database.GetDB().QueryRow(`SELECT u.server_id FROM database_user_grants g JOIN database_users u ON u.id = g.user_id WHERE g.database_id = ?`, databaseID).Scan(&grantUserServer)
	if recordServer != targetServer || grantUserServer != targetServer {
		t.Fatalf("switched: record on %d, user on %d, want %d", recordServer, grantUserServer, targetServer)
	}
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/switch-back", "", userID); recorder.Code != http.StatusOK {
		t.Fatalf("switch back: %d %s", recorder.Code, recorder.Body.String())
	}
	database.GetDB().QueryRow(`SELECT server_id FROM databases_v2 WHERE id = ?`, databaseID).Scan(&recordServer)
	if recordServer != sourceServer {
		t.Fatalf("switched back: record on %d", recordServer)
	}
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/switch", "", userID); recorder.Code != http.StatusOK {
		t.Fatalf("switch again: %d %s", recorder.Code, recorder.Body.String())
	}
	dropPGThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLDropRequest) (transport.PostgreSQLDropResponse, error) {
		if req.Database != "shop" || req.Port != 5432 || req.KeepPort != 5433 {
			t.Fatalf("drop request = %+v", req)
		}
		return transport.PostgreSQLDropResponse{Dropped: true}, nil
	}
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/drop-source", "", userID); recorder.Code != http.StatusOK {
		t.Fatalf("drop source: %d %s", recorder.Code, recorder.Body.String())
	}
	var status string
	database.GetDB().QueryRow(`SELECT state FROM postgresql_database_moves WHERE id = ?`, moveID).Scan(&state)
	database.GetDB().QueryRow(`SELECT status FROM database_servers WHERE id = ?`, sourceServer).Scan(&status)
	if state != "source_dropped" || status != "retired" {
		t.Fatalf("after the drop: move %s, source record %s", state, status)
	}
	if recorder := pgCall(t, panel, http.MethodPost, moves+"/switch-back", "", userID); !strings.Contains(recorder.Body.String(), "PG_MOVE_STATE") {
		t.Fatalf("switch back after the drop: %s", recorder.Body.String())
	}
	rows, _ := database.GetDB().Query(`SELECT action FROM audit_logs WHERE action LIKE 'postgresql.%' ORDER BY id`)
	var actions []string
	for rows.Next() {
		var action string
		rows.Scan(&action)
		actions = append(actions, strings.SplitN(action, ":", 2)[0])
	}
	rows.Close()
	want := "postgresql.database.copy postgresql.database.switch postgresql.database.switch_back postgresql.database.switch postgresql.database.drop_source"
	if strings.Join(actions, " ") != want {
		t.Fatalf("audit = %v", actions)
	}
}

func TestPGRemovalPreviewRefusesWhileAClusterHoldsDatabases(t *testing.T) {
	simulation := removalSimulationFrom("postgresql", transport.ServiceRemovalPreviewResponse{State: "checked", HoldsDatabases: []string{"17/main: shop"}}, nil)
	if !simulation.Refuses || len(simulation.HoldsDatabases) != 1 {
		t.Fatalf("simulation = %+v", simulation)
	}
	simulation = removalSimulationFrom("postgresql", transport.ServiceRemovalPreviewResponse{State: "checked", DatabasesUnknown: []string{"16/main"}}, nil)
	if !simulation.Refuses {
		t.Fatalf("unknown = %+v", simulation)
	}
}

func itoaTest(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}
