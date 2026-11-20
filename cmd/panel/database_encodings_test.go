package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// 20 Nov 2026: the domain's database list names each PostgreSQL database's
// encoding, read from the Agent once for the whole list.

func listDomainDatabases(t *testing.T, panel *Panel, domainID int) map[string]DatabaseInfo {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/databases", nil)
	request = request.WithContext(context.WithValue(request.Context(), callerKey, &Caller{ID: adminAccountUserID, Role: roleAdmin}))
	recorder := httptest.NewRecorder()
	panel.handleGetDomainDatabases(recorder, request, domainID)
	var list struct {
		Databases []DatabaseInfo `json:"databases"`
	}
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &list) != nil {
		t.Fatalf("list: %d %s", recorder.Code, recorder.Body.String())
	}
	byName := map[string]DatabaseInfo{}
	for _, db := range list.Databases {
		byName[db.Name] = db
	}
	return byName
}

func seedEncodingDatabases(t *testing.T, panel *Panel, domainID int) {
	t.Helper()
	db := panel.db.GetDB()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec(`INSERT INTO database_servers (id, subscription_id, type_id, name, host, port, cluster) VALUES (7701, ?, (SELECT id FROM database_server_types WHERE name = 'postgresql'), 'pg18', '127.0.0.1', 5433, '18/main')`, adminAccountSubscriptionID)
	exec(`INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (7702, ?, (SELECT id FROM database_server_types WHERE name = 'postgresql'), 'remote', 'db.example.net', 5432)`, adminAccountSubscriptionID)
	exec(`INSERT INTO database_servers (id, subscription_id, type_id, name, host, port) VALUES (7703, ?, (SELECT id FROM database_server_types WHERE name = 'mariadb'), 'maria', 'localhost', 3306)`, adminAccountSubscriptionID)
	for _, row := range []struct {
		server int
		name   string
	}{{7701, "shop_old"}, {7701, "shop_new"}, {7702, "shop_far"}, {7703, "shop_maria"}} {
		exec(`INSERT INTO databases_v2 (server_id, subscription_id, domain_id, name) VALUES (?, ?, ?, ?)`, row.server, adminAccountSubscriptionID, domainID, row.name)
	}
}

func TestDomainDatabaseListNamesEachPostgreSQLEncodingInOneRead(t *testing.T) {
	panel, domainID := newDomainCreateFixture(t, "shop.example", &domainCreateAgent{})
	seedEncodingDatabases(t, panel, domainID)
	calls := 0
	previous := readPGEncodingsFromAgent
	readPGEncodingsFromAgent = func(*Panel, context.Context) (transport.PostgreSQLEncodingsResponse, error) {
		calls++
		return transport.PostgreSQLEncodingsResponse{Clusters: []transport.PostgreSQLClusterEncodings{
			{Version: "17", Name: "main", Port: 5432, Online: true, Known: true, Encodings: map[string]string{"shop_old": "UTF8"}},
			{Version: "18", Name: "main", Port: 5433, Online: true, Known: true, Encodings: map[string]string{"shop_old": "SQL_ASCII", "shop_new": "UTF8", "shop_far": "UTF8"}},
		}}, nil
	}
	t.Cleanup(func() { readPGEncodingsFromAgent = previous })

	byName := listDomainDatabases(t, panel, domainID)
	if calls != 1 {
		t.Fatalf("the Agent was asked %d times", calls)
	}
	for name, want := range map[string]string{"shop_old": "SQL_ASCII", "shop_new": "UTF8", "shop_far": "", "shop_maria": ""} {
		if got := byName[name].Encoding; got != want {
			t.Fatalf("%s: encoding %q, want %q (%+v)", name, got, want, byName)
		}
	}

	// The Agent does not answer: the list loads, nothing is said about encodings.
	readPGEncodingsFromAgent = func(*Panel, context.Context) (transport.PostgreSQLEncodingsResponse, error) {
		return transport.PostgreSQLEncodingsResponse{}, errors.New("agent unavailable")
	}
	for name, db := range listDomainDatabases(t, panel, domainID) {
		if db.Encoding != "" {
			t.Fatalf("%s: %q without an answer", name, db.Encoding)
		}
	}
}

// A down cluster's databases are not known, and a server with one PostgreSQL
// (no postgresql-common) answers for the record on its port.
func TestPGEncodingOfIsOnlyWhatTheReadSays(t *testing.T) {
	record := pgServerRecord{Host: "127.0.0.1", Port: 5433, Cluster: "18/main"}
	down := transport.PostgreSQLEncodingsResponse{Clusters: []transport.PostgreSQLClusterEncodings{{Version: "18", Name: "main", Port: 5433, Error: "the cluster is not running", Encodings: map[string]string{}}}}
	if got := pgEncodingOf(down, record, "shop"); got != "" {
		t.Fatalf("down cluster: %q", got)
	}
	single := transport.PostgreSQLEncodingsResponse{Single: true, Clusters: []transport.PostgreSQLClusterEncodings{{Port: 5432, Online: true, Known: true, Encodings: map[string]string{"shop": "SQL_ASCII"}}}}
	if got := pgEncodingOf(single, pgServerRecord{Host: "localhost", Port: 5432}, "shop"); got != "SQL_ASCII" {
		t.Fatalf("single: %q", got)
	}
	if got := pgEncodingOf(single, pgServerRecord{Host: "localhost", Port: 5440}, "shop"); got != "" {
		t.Fatalf("single, another port: %q", got)
	}
}
