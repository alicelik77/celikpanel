package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.131: remote database access follows a PostgreSQL cluster's port
// change at once, every apply carries the server switch (so the engine stays
// ready while it is on), and the client settings name the server itself.

func drainDatabaseRemoteKick() bool {
	select {
	case <-databaseRemoteMemory.kick:
		return true
	default:
		return false
	}
}

func TestPGPortChangeAsksRemoteAccessToFollow(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	pgTestRecords(t, database.GetDB(), userID)
	clusters := pgTestClusters(false)
	pgStubAgent(t, &clusters)
	previousMemory := databaseRemoteMemory
	databaseRemoteMemory = &databaseRemoteState{kick: make(chan struct{}, 1)}
	t.Cleanup(func() { databaseRemoteMemory = previousMemory })
	sha := strings.Repeat("a", 64)
	body := `{"version":"17","name":"main","port":5440,"expected_sha256":"` + sha + `"}`
	// Not proved: nothing moved, nothing to follow.
	setPGPortThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLSetPortRequest) (transport.PostgreSQLSetPortResponse, error) {
		return transport.PostgreSQLSetPortResponse{From: 5432, To: req.Port, RolledBack: true, Restored: true, ErrorCode: "PROOF_FAILED", Error: "no"}, nil
	}
	if recorder := pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/port", body, userID); recorder.Code != http.StatusConflict || drainDatabaseRemoteKick() {
		t.Fatalf("refused change: %d, kicked", recorder.Code)
	}
	setPGPortThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLSetPortRequest) (transport.PostgreSQLSetPortResponse, error) {
		return transport.PostgreSQLSetPortResponse{Changed: true, Proved: true, From: 5432, To: req.Port}, nil
	}
	if recorder := pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/port", body, userID); recorder.Code != http.StatusOK || !drainDatabaseRemoteKick() {
		t.Fatalf("proved change: %d, no forced apply asked for", recorder.Code)
	}
	// The records follow a port changed outside CelikPanel: the same.
	clusters.Clusters[0].Port = 5441
	if recorder := pgCall(t, panel, http.MethodPost, pgClustersAPIPath+"/records", `{"version":"17","name":"main"}`, userID); recorder.Code != http.StatusOK || !drainDatabaseRemoteKick() {
		t.Fatalf("records sync: %d, no forced apply asked for", recorder.Code)
	}
}

func TestDatabaseRemoteApplyCarriesTheServerSwitch(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	recorder := &databaseRemoteRecorder{}
	recorder.install(t)
	dbrOffer(t, p, true)
	if _, _, err := p.applyDatabaseRemote(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !recorder.last().ServerOffered {
		t.Fatal("the switch on was not sent: the Agent would take the listen address back with the last rule")
	}
	dbrOffer(t, p, false)
	if _, _, err := p.applyDatabaseRemote(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if recorder.last().ServerOffered {
		t.Fatal("the switch off was sent as on")
	}
}

func TestDatabaseRemoteClientNamesTheServerNotTheMailHost(t *testing.T) {
	p := newDatabaseRemotePanel(t)
	saved := localServerName
	t.Cleanup(func() { localServerName = saved })
	localServerName = func() (string, error) { return "Boston.Example.Test.", nil }
	if host := p.databaseRemoteClientFor(context.Background(), databaseRemoteDatabase{Port: 5432}).Host; host != "boston.example.test" {
		t.Fatalf("host = %q", host)
	}
	localServerName = func() (string, error) { return "localhost", nil }
	if host := p.databaseRemoteServerHost(context.Background()); host == "localhost" {
		t.Fatal("a name that is not a full host name was shown")
	}
}
