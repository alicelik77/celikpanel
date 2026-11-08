package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/alicelik/celikpanel/internal/transport"
)

// alpha.130 (the owner's rule of 8 Nov 2026: "no leftovers, ever"): a
// removed major's cluster that holds only what initdb made is removed by the
// Agent once after the start and audited; any other waits for the
// administrator's "Archive and remove"; a cluster CelikPanel records
// databases on is never removed by either path.

func pgLeftoverClusters(leftoverPort int, fresh bool) transport.PostgreSQLClustersResponse {
	clusters := pgTestClusters(false)
	clusters.Clusters = append(clusters.Clusters, transport.PostgreSQLCluster{
		Version: "16", Name: "main", Port: leftoverPort, Status: "down,binaries_missing", Owner: "postgres",
		DataDirectory: "/var/lib/postgresql/16/main", Databases: []string{},
		Leftover: &transport.PostgreSQLLeftover{Fresh: fresh, DataBytes: 40 << 20},
	})
	return clusters
}

func stubPGLeftoverAgent(t *testing.T) (cleanups *int, removes *[]transport.PostgreSQLLeftoverRemoveRequest) {
	t.Helper()
	restoreClean, restoreRemove := cleanPGLeftoversThroughAgent, removePGLeftoverThroughAgent
	t.Cleanup(func() { cleanPGLeftoversThroughAgent, removePGLeftoverThroughAgent = restoreClean, restoreRemove })
	count := 0
	var requests []transport.PostgreSQLLeftoverRemoveRequest
	cleanPGLeftoversThroughAgent = func(*Panel, context.Context) (transport.PostgreSQLLeftoverCleanupResponse, error) {
		count++
		return transport.PostgreSQLLeftoverCleanupResponse{Removed: []transport.PostgreSQLLeftoverRemoval{{
			Version: "16", Name: "main", Port: 5434, DataDirectory: "/var/lib/postgresql/16/main", DataBytes: 40 << 20,
			Removed: []string{"/etc/postgresql/16/main", "/var/lib/postgresql/16/main"},
		}}}, nil
	}
	removePGLeftoverThroughAgent = func(_ *Panel, _ context.Context, req transport.PostgreSQLLeftoverRemoveRequest) (transport.PostgreSQLLeftoverRemoveResponse, error) {
		requests = append(requests, req)
		return transport.PostgreSQLLeftoverRemoveResponse{Removed: true, Changed: true, Removal: transport.PostgreSQLLeftoverRemoval{
			Version: req.Version, Name: req.Name, Port: 5434, Archive: "/var/backups/celikpanel/postgresql-16-main-20261108T120000Z.tar.gz",
			Removed: []string{"/etc/postgresql/16/main"},
		}}, nil
	}
	return &count, &requests
}

func lastAudit(t *testing.T, panel *Panel) (action, details string) {
	t.Helper()
	panel.db.GetDB().QueryRow(`SELECT action, COALESCE(details, '') FROM audit_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &details)
	return action, details
}

func TestPGFreshLeftoverIsRemovedOnceAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	pgTestRecords(t, database.GetDB(), userID) // a record on 17/main (port 5432)
	clusters := pgLeftoverClusters(5434, true)
	pgStubAgent(t, &clusters)
	cleanups, _ := stubPGLeftoverAgent(t)
	if done := panel.cleanPGLeftoversOnce(context.Background()); !done || *cleanups != 1 {
		t.Fatalf("done=%v cleanups=%d", done, *cleanups)
	}
	action, details := lastAudit(t, panel)
	if action != "postgresql.cluster.leftover.removed:16/main" || !strings.Contains(details, `"automatic":true`) || !strings.Contains(details, `/var/lib/postgresql/16/main`) {
		t.Fatalf("audit = %q %s", action, details)
	}
	// Nothing left: no call.
	clusters = pgTestClusters(false)
	if done := panel.cleanPGLeftoversOnce(context.Background()); !done || *cleanups != 1 {
		t.Fatalf("second: done=%v cleanups=%d", done, *cleanups)
	}
	// A leftover that may hold data is never removed automatically.
	clusters = pgLeftoverClusters(5434, false)
	if done := panel.cleanPGLeftoversOnce(context.Background()); !done || *cleanups != 1 {
		t.Fatalf("kept: done=%v cleanups=%d", done, *cleanups)
	}
}

func TestPGFreshLeftoverWithPanelRecordsIsLeftToTheAdministrator(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	pgTestRecords(t, database.GetDB(), userID) // record port 5432: the leftover's port
	clusters := pgLeftoverClusters(5432, true)
	clusters.Clusters[0].Port = 5435
	pgStubAgent(t, &clusters)
	cleanups, _ := stubPGLeftoverAgent(t)
	if done := panel.cleanPGLeftoversOnce(context.Background()); !done || *cleanups != 0 {
		t.Fatalf("done=%v cleanups=%d", done, *cleanups)
	}
	recorder := pgCall(t, panel, http.MethodPost, pgLeftoverAPIPath, `{"version":"16","name":"main"}`, userID)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "PG_LEFTOVER_HAS_RECORDS") {
		t.Fatalf("archive with records: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestPGLeftoverArchiveAndRemoveIsExplicitAndAudited(t *testing.T) {
	panel, _, database, userID := newServiceStateAtomicPanel(t)
	pgTestRecords(t, database.GetDB(), userID)
	clusters := pgLeftoverClusters(5434, false)
	pgStubAgent(t, &clusters)
	_, removes := stubPGLeftoverAgent(t)
	if recorder := pgCall(t, panel, http.MethodPost, pgLeftoverAPIPath, `{"version":"17","name":"main"}`, userID); recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "PG_NOT_A_LEFTOVER") {
		t.Fatalf("a running cluster: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder := pgCall(t, panel, http.MethodPost, pgLeftoverAPIPath, `{"version":"16","name":"main"}`, userID)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "postgresql-16-main-20261108T120000Z.tar.gz") || len(*removes) != 1 {
		t.Fatalf("archive and remove: %d %s", recorder.Code, recorder.Body.String())
	}
	action, details := lastAudit(t, panel)
	if action != "postgresql.cluster.leftover.removed:16/main" || !strings.Contains(details, `"archive":"/var/backups/celikpanel/`) || !strings.Contains(details, `"automatic":false`) {
		t.Fatalf("audit = %q %s", action, details)
	}
}
