package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Leftover PostgreSQL clusters (alpha.130; the owner's rule of 8 Nov 2026:
// "no leftovers, ever"). A cluster whose PostgreSQL program was removed and
// whose files stayed (Boston: 16/main, "down,binaries_missing", still
// claiming port 5432 while PostgreSQL 18 ran on 5433) is read by the Agent
// from its own files (pg_leftovers.go):
//
//   - holding only what initdb made, it is removed by the Agent once after
//     the panel starts, in its own pg_leftover_cleanup job, and each removal
//     is written to the audit log (postgresql.cluster.leftover.removed);
//   - otherwise it may hold the owner's data and is never removed silently:
//     the PostgreSQL page names it, and the administrator's explicit
//     "Archive and remove" (POST /api/v1/service/postgresql/clusters/leftover)
//     keeps its files in /var/backups/celikpanel first, in its own
//     pg_leftover_remove job, with its steps shown.
//
// A cluster CelikPanel still records databases on is never removed by
// either path: those records are resolved first.
//
// Kaldırılmış bir PostgreSQL'in kümesi: yalnız initdb'nin yaptıklarını
// tutuyorsa Agent onu kendisi kaldırır ve denetim günlüğüne yazılır; aksi
// hâlde sahibi "Arşivle ve kaldır" ile açıkça seçer.

const (
	pgLeftoverAPIPath          = pgClustersAPIPath + "/leftover"
	pgLeftoverCleanupFirstWait = 2 * time.Minute
	pgLeftoverCleanupRetry     = 5 * time.Minute
	pgLeftoverCleanupAttempts  = 12
)

var requestIdentityRoutePGLeftover = operationStepRoute(pgLeftoverAPIPath)

var cleanPGLeftoversThroughAgent = func(p *Panel, ctx context.Context) (transport.PostgreSQLLeftoverCleanupResponse, error) {
	var response transport.PostgreSQLLeftoverCleanupResponse
	err := p.withStandaloneAgentMutation(ctx, "pg_leftover_cleanup", "pg-leftovers", "", func(callCtx context.Context, binding agentMutationBinding) error {
		req := transport.PostgreSQLLeftoverCleanupRequest{ServiceMutationBinding: binding}
		if err := p.callAgentContext(callCtx, "Agent.PostgreSQLLeftoverCleanupV1", &req, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	return response, err
}

var removePGLeftoverThroughAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLLeftoverRemoveRequest) (transport.PostgreSQLLeftoverRemoveResponse, error) {
	var response transport.PostgreSQLLeftoverRemoveResponse
	err := p.withStandaloneAgentMutation(ctx, "pg_leftover_remove", pgClusterJobTarget(req.Version, req.Name), "", func(callCtx context.Context, binding agentMutationBinding) error {
		req.ServiceMutationBinding = binding
		if err := p.callAgentContext(callCtx, "Agent.RemovePostgreSQLLeftoverV1", &req, &response); err != nil {
			return err
		}
		if response.Error != "" {
			return errors.New(response.Error)
		}
		return nil
	})
	if err != nil && response.Error == "" {
		return response, err
	}
	return response, nil
}

// pgLeftoverAuditDetails is what the audit row keeps of one removal.
func pgLeftoverAuditDetails(removal transport.PostgreSQLLeftoverRemoval, automatic bool) map[string]any {
	details := map[string]any{
		"component": "postgresql", "cluster": pgRef(removal.Version, removal.Name), "port": removal.Port,
		"data_directory": removal.DataDirectory, "data_bytes": removal.DataBytes, "removed": removal.Removed,
		"automatic": automatic,
	}
	if removal.Archive != "" {
		details["archive"] = removal.Archive
	}
	return details
}

// pgLeftoversHeldByRecords are the leftover clusters CelikPanel still
// records databases on: never removed by the automatic cleanup.
func (p *Panel) pgLeftoversHeldByRecords(ctx context.Context, clusters transport.PostgreSQLClustersResponse) (map[string]bool, error) {
	view, err := p.pgClustersViewFrom(ctx, clusters)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for ref, records := range view.Records {
		if len(records) > 0 {
			held[ref] = true
		}
	}
	return held, nil
}

// tryBackgroundServiceMutation takes the panel's mutation lock for work the
// panel starts itself, only when no change runs (never waits for one).
func (p *Panel) tryBackgroundServiceMutation(ctx context.Context) (func(), bool) {
	if active, err := p.serverSetupExecutionMutating(ctx); err != nil || active {
		return nil, false
	}
	if !p.serviceMutationMu.TryLock() {
		return nil, false
	}
	release := p.serviceMutationMu.Unlock
	if op, err := p.activeServiceOperation(ctx); err != nil || op != nil {
		release()
		return nil, false
	}
	statusCtx, cancel := context.WithTimeout(ctx, panelMutationFinishTimeout)
	job, err := p.statusAgentMutation(statusCtx, "")
	cancel()
	if err != nil || (job != nil && agentMutationActive(job.Status)) {
		release()
		return nil, false
	}
	return release, true
}

// cleanPGLeftoversOnce is one attempt: done is true when there is nothing
// (more) to do or the outcome is final; false asks for a later attempt.
func (p *Panel) cleanPGLeftoversOnce(ctx context.Context) (done bool) {
	clusters, err := readPGClustersFromAgent(p, ctx)
	if err != nil {
		return false
	}
	if !clusters.Supported {
		return true
	}
	fresh := []string{}
	for _, cluster := range clusters.Clusters {
		if cluster.Leftover != nil && cluster.Leftover.Fresh {
			fresh = append(fresh, pgRef(cluster.Version, cluster.Name))
		}
	}
	if len(fresh) == 0 {
		return true
	}
	release, ok := p.tryBackgroundServiceMutation(ctx)
	if !ok {
		return false
	}
	defer release()
	held, err := p.pgLeftoversHeldByRecords(ctx, clusters)
	if err != nil {
		return false
	}
	for _, ref := range fresh {
		if held[ref] {
			// CelikPanel records databases there: the page shows it, the
			// administrator decides.
			log.Printf("postgresql leftovers: %s holds only what initdb made, but CelikPanel records databases on it; it is left for the administrator", ref)
			return true
		}
	}
	response, err := cleanPGLeftoversThroughAgent(p, ctx)
	for _, removal := range response.Removed {
		ref := pgRef(removal.Version, removal.Name)
		p.recordAudit(auditRecord{
			Action:       "postgresql.cluster.leftover.removed:" + ref,
			ResourceType: "service",
			Details:      auditDetailsText(pgLeftoverAuditDetails(removal, true)),
		})
	}
	if err != nil {
		log.Printf("postgresql leftovers: the automatic removal did not finish: %v", err)
		p.recordAudit(auditRecord{
			Action:       "postgresql.cluster.leftover.failed:" + strings.Join(fresh, ",") + " — " + auditReason(err.Error()),
			ResourceType: "service",
			Details:      auditDetailsText(map[string]any{"component": "postgresql", "clusters": fresh, "automatic": true}),
		})
		return true
	}
	return true
}

// startPGLeftoverCleanup runs the automatic cleanup once after the panel
// starts: first after a short wait (the Agent and the start-up work go
// first), then again a few times while another change holds the server.
func (p *Panel) startPGLeftoverCleanup() {
	go func() {
		time.Sleep(pgLeftoverCleanupFirstWait)
		for attempt := 0; attempt < pgLeftoverCleanupAttempts; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), agentRPCDatabaseTimeout+2*time.Minute)
			done := p.cleanPGLeftoversOnce(ctx)
			cancel()
			if done {
				return
			}
			time.Sleep(pgLeftoverCleanupRetry)
		}
		log.Printf("postgresql leftovers: the automatic check could not run (the server stayed busy or the Agent did not answer); it runs again at the next start")
	}()
}

// handlePGLeftoverRemove is the administrator's explicit "Archive and
// remove" of one leftover cluster.
func (p *Panel) handlePGLeftoverRemove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "archive", "records")
	reportOperationStep(ctx, "check")
	var req struct {
		Version string `json:"version"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !pgClusterRefText.MatchString(pgRef(req.Version, req.Name)) {
		writeClientError(w, http.StatusBadRequest, "a cluster is required")
		return
	}
	ref := pgRef(req.Version, req.Name)
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	clusters, err := readPGClustersFromAgent(p, ctx)
	if err != nil {
		writeCodedError(w, http.StatusBadGateway, "PG_CLUSTERS_UNKNOWN",
			"The PostgreSQL clusters could not be read, so nothing was changed. Check that the CelikPanel Agent runs (sudo systemctl status celikpanel-agent), then try again.", "")
		return
	}
	var leftover *transport.PostgreSQLCluster
	for i := range clusters.Clusters {
		cluster := &clusters.Clusters[i]
		if cluster.Version == req.Version && cluster.Name == req.Name && cluster.Leftover != nil {
			leftover = cluster
		}
	}
	if leftover == nil {
		writeCodedError(w, http.StatusConflict, "PG_NOT_A_LEFTOVER",
			fmt.Sprintf("Cluster %s is not a removed PostgreSQL's cluster on this server any more (it runs, or it is gone). Nothing was changed; the page shows the clusters as they are now.", ref), "")
		return
	}
	held, err := p.pgLeftoversHeldByRecords(ctx, clusters)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if held[ref] {
		writeCodedError(w, http.StatusConflict, "PG_LEFTOVER_HAS_RECORDS",
			fmt.Sprintf("CelikPanel still records databases on cluster %s. Nothing was changed: remove those database records (Databases) or move them first, then archive and remove the cluster.", ref), "")
		return
	}
	reportOperationStep(ctx, "archive")
	response, err := removePGLeftoverThroughAgent(p, ctx, transport.PostgreSQLLeftoverRemoveRequest{
		PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: req.Version, Name: req.Name},
	})
	details := pgLeftoverAuditDetails(response.Removal, false)
	details["code"] = response.ErrorCode
	if err != nil {
		p.auditDetailed(r, "postgresql.cluster.leftover.failed:"+ref+" — "+auditReason(err.Error()), "service", 0, details)
		writeAgentError(w, err, "PostgreSQL leftover cluster")
		return
	}
	if !response.Removed {
		p.auditDetailed(r, "postgresql.cluster.leftover.failed:"+ref+" — "+auditReason(response.Error), "service", 0, details)
		status := http.StatusConflict
		if response.Changed {
			status = http.StatusInternalServerError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": response.Error, "code": "PG_LEFTOVER_" + strings.TrimPrefix(response.ErrorCode, "PG_LEFTOVER_"),
			"changed": response.Changed, "archive": response.Removal.Archive,
		})
		return
	}
	reportOperationStep(ctx, "records")
	p.auditDetailed(r, "postgresql.cluster.leftover.removed:"+ref, "service", 0, details)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"removed": true, "cluster": ref, "archive": response.Removal.Archive, "removed_paths": response.Removal.Removed,
	})
}
