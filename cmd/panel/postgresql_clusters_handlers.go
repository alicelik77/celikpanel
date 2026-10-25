package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQL majors side by side (alpha.108; the owner's direction of
// 2026-10-25 replacing D-038 §7's major upgrade flow). Admin only, through
// the /api/v1/service/ prefix:
//
//	GET  /api/v1/service/postgresql/clusters              every cluster, the panel's records on it, the moves
//	GET  /api/v1/service/postgresql/clusters/port-preview who connects and what changes (?version=&name=&port=)
//	POST /api/v1/service/postgresql/clusters/port         change a cluster's port (pg_cluster_port job)
//	POST /api/v1/service/postgresql/clusters/records      set the panel's records to a cluster's current port
//	POST /api/v1/service/postgresql/moves                 copy a database to another cluster (pg_database_copy job)
//	POST /api/v1/service/postgresql/moves/{id}/{action}   switch | switch-back | drop-source | drop-copy
//
// The panel's record of where a database lives is databases_v2.server_id: a
// PostgreSQL server record is one cluster (database_servers.port, and since
// schema 46 .cluster). Records are changed only after the Agent's proof, and
// a move's record switch is its own confirmed step after the copy's proof.
// Every mutation holds the service-mutation lock, so a poll or a second click
// never starts a second one; reads never mutate the host.
//
// PostgreSQL ana sürümleri yan yana: kümeler, portları, taşımalar. Kayıt
// yalnız Agent'ın kanıtından sonra değişir; taşımanın geçişi ayrı bir adımdır.

const (
	pgClustersAPIPath  = "/api/v1/service/postgresql/clusters"
	pgServiceAPIPrefix = "/api/v1/service/postgresql/"
	pgClustersReadBud  = 75 * time.Second
	pgMoveHistoryCap   = 20
)

var (
	pgClusterRefText = regexp.MustCompile(`^([0-9]{1,3})/([A-Za-z0-9_][A-Za-z0-9_.-]{0,62})$`)
	pgDatabaseText   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$-]{0,62}$`)
	pgMovePath       = regexp.MustCompile(`^/api/v1/service/postgresql/moves/([0-9]{1,12})/(switch|switch-back|drop-source|drop-copy)$`)
)

// provisionPGTargetAccount opens CelikPanel's own account on a cluster a new
// server record names (a variable for tests).
var provisionPGTargetAccount = func(p *Panel, ctx context.Context, subscriptionID int) error {
	return p.provisionDatabaseAdminAccountsFor(ctx, subscriptionID, []string{"postgresql"})
}

var readPGClustersFromAgent = func(p *Panel, ctx context.Context) (transport.PostgreSQLClustersResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, pgClustersReadBud)
	defer cancel()
	var response transport.PostgreSQLClustersResponse
	err := p.callAgentContext(callCtx, "Agent.PostgreSQLClustersV1", &transport.Empty{}, &response)
	return response, err
}

var readPGPortPreviewFromAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLPortPreviewRequest) (transport.PostgreSQLPortPreviewResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, pgClustersReadBud)
	defer cancel()
	var response transport.PostgreSQLPortPreviewResponse
	err := p.callAgentContext(callCtx, "Agent.PostgreSQLClusterPortPreviewV1", &req, &response)
	return response, err
}

func pgClusterJobTarget(version, name string) string {
	return "pg-" + version + "-" + strings.ToLower(name)
}

func pgDatabaseJobTarget(name string) string { return "pgdb-" + strings.ToLower(name) }

var setPGPortThroughAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLSetPortRequest) (transport.PostgreSQLSetPortResponse, error) {
	var response transport.PostgreSQLSetPortResponse
	err := p.withStandaloneAgentMutation(ctx, "pg_cluster_port", pgClusterJobTarget(req.Version, req.Name), strconv.Itoa(req.Port), func(callCtx context.Context, binding agentMutationBinding) error {
		req.ServiceMutationBinding = binding
		if err := p.callAgentContext(callCtx, "Agent.SetPostgreSQLClusterPortV1", &req, &response); err != nil {
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

var copyPGThroughAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLCopyRequest) (transport.PostgreSQLCopyResponse, error) {
	var response transport.PostgreSQLCopyResponse
	qualifier := strconv.Itoa(req.SourcePort) + "-" + strconv.Itoa(req.TargetPort)
	err := p.withStandaloneAgentMutation(ctx, "pg_database_copy", pgDatabaseJobTarget(req.Database), qualifier, func(callCtx context.Context, binding agentMutationBinding) error {
		req.ServiceMutationBinding = binding
		if err := p.callAgentContext(callCtx, "Agent.CopyPostgreSQLDatabaseV1", &req, &response); err != nil {
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

// checkPGSourceThroughAgent asks, read only, whether a move's source is still
// as it was at the copy (alpha.108 review).
var checkPGSourceThroughAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLSourceCheckRequest) (transport.PostgreSQLSourceCheckResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, pgClustersReadBud)
	defer cancel()
	var response transport.PostgreSQLSourceCheckResponse
	err := p.callAgentContext(callCtx, "Agent.PostgreSQLSourceCheckV1", &req, &response)
	return response, err
}

// pgMoveExpectation is the source as the stored proof saw it; ok is false
// for a proof without the source's write counters (it cannot prove the source
// unchanged, so nothing that relies on that runs).
func pgMoveExpectation(move *pgMoveView) (transport.PostgreSQLSourceExpectation, bool) {
	var proof transport.PostgreSQLCopyProof
	if err := json.Unmarshal(move.Proof, &proof); err != nil || proof.SourceWrites == nil {
		return transport.PostgreSQLSourceExpectation{}, false
	}
	return transport.PostgreSQLSourceExpectation{Writes: *proof.SourceWrites, Tables: proof.Tables, TablesTotal: proof.TablesTotal}, true
}

var dropPGThroughAgent = func(p *Panel, ctx context.Context, req transport.PostgreSQLDropRequest) (transport.PostgreSQLDropResponse, error) {
	var response transport.PostgreSQLDropResponse
	err := p.withStandaloneAgentMutation(ctx, "pg_database_drop", pgDatabaseJobTarget(req.Database), strconv.Itoa(req.Port), func(callCtx context.Context, binding agentMutationBinding) error {
		req.ServiceMutationBinding = binding
		if err := p.callAgentContext(callCtx, "Agent.DropPostgreSQLDatabaseV1", &req, &response); err != nil {
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

// pgServerRecord is one PostgreSQL server record of the panel.
type pgServerRecord struct {
	ID             int
	SubscriptionID int
	Port           int
	Cluster        string
	Status         string
	Host           string
}

// pgPanelRecord is one database the panel records on a cluster.
type pgPanelRecord struct {
	DatabaseID   int      `json:"database_id"`
	Name         string   `json:"name"`
	Domain       string   `json:"domain,omitempty"`
	Subscription int      `json:"subscription_id"`
	ServerID     int      `json:"server_id"`
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	Users        []string `json:"users"`
	Legacy       bool     `json:"legacy,omitempty"`
}

// pgRecordMismatch: the panel's records of a cluster say another port than
// the cluster runs on (changed outside CelikPanel).
type pgRecordMismatch struct {
	Cluster     string `json:"cluster"`
	RecordPort  int    `json:"record_port"`
	ClusterPort int    `json:"cluster_port"`
	Records     int    `json:"records"`
}

type pgMoveView struct {
	ID             int             `json:"id"`
	DatabaseID     *int            `json:"database_id"`
	Database       string          `json:"database"`
	SourceCluster  string          `json:"source_cluster"`
	TargetCluster  string          `json:"target_cluster"`
	SourcePort     int             `json:"source_port"`
	TargetPort     int             `json:"target_port"`
	State          string          `json:"state"`
	Proof          json.RawMessage `json:"proof"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	SourceServerID *int            `json:"-"`
	TargetServerID *int            `json:"-"`
}

type pgClustersView struct {
	State string `json:"state"`
	transport.PostgreSQLClustersResponse
	Records    map[string][]pgPanelRecord `json:"records"`
	Unplaced   []pgPanelRecord            `json:"unplaced"`
	Mismatches []pgRecordMismatch         `json:"mismatches"`
	Moves      []pgMoveView               `json:"moves"`
}

func pgRef(version, name string) string { return version + "/" + name }

func (p *Panel) pgServerRecords(ctx context.Context) ([]pgServerRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT ds.id, ds.subscription_id, ds.port, COALESCE(ds.cluster, ''), COALESCE(ds.status, 'active'), COALESCE(ds.host, '')
		FROM database_servers ds
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE dst.name = 'postgresql'
		ORDER BY ds.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pgServerRecord
	for rows.Next() {
		var record pgServerRecord
		if err := rows.Scan(&record.ID, &record.SubscriptionID, &record.Port, &record.Cluster, &record.Status, &record.Host); err != nil {
			return nil, err
		}
		if databaseEngineIsOnThisMachine(record.Host) {
			out = append(out, record)
		}
	}
	return out, rows.Err()
}

// pgServerCluster says which cluster a server record is: its cluster name
// (schema 46), else the cluster running on its port.
func pgServerCluster(record pgServerRecord, clusters []transport.PostgreSQLCluster) (string, bool) {
	if record.Cluster != "" {
		return record.Cluster, true
	}
	for _, cluster := range clusters {
		if cluster.Port == record.Port {
			return pgRef(cluster.Version, cluster.Name), true
		}
	}
	return "", false
}

// tiePGServerRecords names the cluster of every PostgreSQL record made before
// schema 46 (cluster NULL) after the cluster on its port: on an existing
// server that is its single cluster. Only a NULL is filled; a named record
// is never renamed.
func (p *Panel) tiePGServerRecords(ctx context.Context, records []pgServerRecord, clusters []transport.PostgreSQLCluster) {
	for i, record := range records {
		if record.Cluster != "" {
			continue
		}
		ref, ok := pgServerCluster(record, clusters)
		if !ok {
			continue
		}
		if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE database_servers SET cluster = ?, updated_at = datetime('now') WHERE id = ? AND cluster IS NULL`, ref, record.ID); err != nil {
			log.Printf("postgresql clusters: tie record %d to %s: %v", record.ID, ref, err)
			continue
		}
		records[i].Cluster = ref
	}
}

func (p *Panel) pgDatabaseRecords(ctx context.Context) ([]pgPanelRecord, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT d.id, d.name, COALESCE(dom.name, ''), d.subscription_id, ds.id, COALESCE(ds.host, ''), ds.port,
		       COALESCE((SELECT group_concat(u.username, ',') FROM database_user_grants g JOIN database_users u ON u.id = g.user_id WHERE g.database_id = d.id), '')
		FROM databases_v2 d
		JOIN database_servers ds ON ds.id = d.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		LEFT JOIN domains dom ON dom.id = d.domain_id
		WHERE dst.name = 'postgresql'
		ORDER BY d.name, d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pgPanelRecord
	for rows.Next() {
		var record pgPanelRecord
		var users string
		if err := rows.Scan(&record.DatabaseID, &record.Name, &record.Domain, &record.Subscription, &record.ServerID, &record.Host, &record.Port, &users); err != nil {
			return nil, err
		}
		record.Users = []string{}
		if users != "" {
			record.Users = strings.Split(users, ",")
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The legacy databases table records host and port per database.
	legacy, err := p.db.GetDB().QueryContext(ctx, `SELECT id, name, subscription_id, COALESCE(host, ''), COALESCE(port, 5432), db_user FROM databases WHERE LOWER(db_type) IN ('postgresql', 'postgres') ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer legacy.Close()
	for legacy.Next() {
		var record pgPanelRecord
		var user string
		if err := legacy.Scan(&record.DatabaseID, &record.Name, &record.Subscription, &record.Host, &record.Port, &user); err != nil {
			return nil, err
		}
		if databaseEngineIsOnThisMachine(record.Host) {
			record.Legacy, record.Users = true, []string{user}
			out = append(out, record)
		}
	}
	return out, legacy.Err()
}

func (p *Panel) pgMoves(ctx context.Context) ([]pgMoveView, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT id, database_id, database_name, source_cluster, target_cluster, source_port, target_port, state, proof, created_at, updated_at, source_server_id, target_server_id
		FROM postgresql_database_moves
		ORDER BY CASE WHEN state IN ('copied', 'switched', 'switched_back') THEN 0 ELSE 1 END, id DESC
		LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	moves := []pgMoveView{}
	history := 0
	for rows.Next() {
		var move pgMoveView
		var databaseID, sourceServer, targetServer sql.NullInt64
		var proof string
		if err := rows.Scan(&move.ID, &databaseID, &move.Database, &move.SourceCluster, &move.TargetCluster, &move.SourcePort, &move.TargetPort, &move.State, &proof, &move.CreatedAt, &move.UpdatedAt, &sourceServer, &targetServer); err != nil {
			return nil, err
		}
		move.DatabaseID = nullableInt(databaseID)
		move.SourceServerID, move.TargetServerID = nullableInt(sourceServer), nullableInt(targetServer)
		move.Proof = json.RawMessage(proof)
		if !pgMoveActive(move.State) {
			if history == pgMoveHistoryCap {
				continue
			}
			history++
		}
		moves = append(moves, move)
	}
	return moves, rows.Err()
}

func nullableInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func pgMoveActive(state string) bool {
	return state == "copied" || state == "switched" || state == "switched_back"
}

// pgClustersViewFrom joins the Agent's clusters with the panel's records.
func (p *Panel) pgClustersViewFrom(ctx context.Context, clusters transport.PostgreSQLClustersResponse) (pgClustersView, error) {
	view := pgClustersView{State: "read", PostgreSQLClustersResponse: clusters, Records: map[string][]pgPanelRecord{}, Unplaced: []pgPanelRecord{}, Mismatches: []pgRecordMismatch{}, Moves: []pgMoveView{}}
	servers, err := p.pgServerRecords(ctx)
	if err != nil {
		return view, err
	}
	p.tiePGServerRecords(ctx, servers, clusters.Clusters)
	byServer := map[int]pgServerRecord{}
	for _, server := range servers {
		byServer[server.ID] = server
	}
	portOf := map[string]int{}
	for _, cluster := range clusters.Clusters {
		ref := pgRef(cluster.Version, cluster.Name)
		portOf[ref] = cluster.Port
		view.Records[ref] = []pgPanelRecord{}
	}
	mismatch := map[string]*pgRecordMismatch{}
	for _, server := range servers {
		ref, ok := pgServerCluster(server, clusters.Clusters)
		if port, known := portOf[ref]; ok && known && port != server.Port {
			if mismatch[ref] == nil {
				mismatch[ref] = &pgRecordMismatch{Cluster: ref, RecordPort: server.Port, ClusterPort: port}
			}
			mismatch[ref].Records++
		}
	}
	for _, ref := range sortedKeys(mismatch) {
		view.Mismatches = append(view.Mismatches, *mismatch[ref])
	}
	records, err := p.pgDatabaseRecords(ctx)
	if err != nil {
		return view, err
	}
	for _, record := range records {
		ref := ""
		if server, ok := byServer[record.ServerID]; ok && !record.Legacy {
			ref, _ = pgServerCluster(server, clusters.Clusters)
		} else {
			for _, cluster := range clusters.Clusters {
				if cluster.Port == record.Port {
					ref = pgRef(cluster.Version, cluster.Name)
				}
			}
		}
		if _, known := view.Records[ref]; ref != "" && known {
			view.Records[ref] = append(view.Records[ref], record)
		} else {
			view.Unplaced = append(view.Unplaced, record)
		}
	}
	view.Moves, err = p.pgMoves(ctx)
	return view, err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (p *Panel) handlePostgreSQLService(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if caller := currentCaller(r); caller == nil || !caller.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, "ADMIN_ONLY", "administrator access is required", "")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	switch {
	case path == pgClustersAPIPath && r.Method == http.MethodGet:
		p.handlePGClusters(w, r)
	case path == pgClustersAPIPath+"/port-preview" && r.Method == http.MethodGet:
		p.handlePGPortPreview(w, r)
	case path == pgClustersAPIPath+"/port" && r.Method == http.MethodPost:
		p.handlePGPortChange(w, r)
	case path == pgClustersAPIPath+"/records" && r.Method == http.MethodPost:
		p.handlePGRecordsSync(w, r)
	case path == pgServiceAPIPrefix+"moves" && r.Method == http.MethodPost:
		p.handlePGCopy(w, r)
	case pgMovePath.MatchString(path) && r.Method == http.MethodPost:
		m := pgMovePath.FindStringSubmatch(path)
		id, _ := strconv.Atoi(m[1])
		p.handlePGMoveAction(w, r, id, m[2])
	default:
		writeClientError(w, http.StatusNotFound, "not found")
	}
}

func (p *Panel) handlePGClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := readPGClustersFromAgent(p, r.Context())
	if err != nil {
		log.Printf("postgresql clusters: could not be read: %v", err)
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	view, err := p.pgClustersViewFrom(r.Context(), clusters)
	if err != nil {
		writeServerError(w, err)
		return
	}
	json.NewEncoder(w).Encode(view)
}

// pgClusterRecordsOn are the panel's databases on one cluster (by its ref).
func (p *Panel) pgClusterRecordsOn(ctx context.Context, clusters transport.PostgreSQLClustersResponse, ref string) ([]pgPanelRecord, error) {
	view, err := p.pgClustersViewFrom(ctx, clusters)
	if err != nil {
		return nil, err
	}
	return view.Records[ref], nil
}

func (p *Panel) handlePGPortPreview(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	version, name := query.Get("version"), query.Get("name")
	port, err := strconv.Atoi(query.Get("port"))
	if !pgClusterRefText.MatchString(pgRef(version, name)) || err != nil {
		writeClientError(w, http.StatusBadRequest, "a cluster and a port are required")
		return
	}
	preview, err := readPGPortPreviewFromAgent(p, r.Context(), transport.PostgreSQLPortPreviewRequest{PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: version, Name: name}, Port: port})
	if err != nil {
		log.Printf("postgresql port preview: %v", err)
		json.NewEncoder(w).Encode(map[string]any{"state": "unknown"})
		return
	}
	records := []pgPanelRecord{}
	if clusters, err := readPGClustersFromAgent(p, r.Context()); err == nil {
		if on, err := p.pgClusterRecordsOn(r.Context(), clusters, pgRef(version, name)); err == nil && on != nil {
			records = on
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"state": "read", "preview": preview, "records": records})
}

// updatePGRecordsPort sets the panel's records of a cluster to its port, in
// one transaction, after the Agent proved the cluster answers there.
func (p *Panel) updatePGRecordsPort(ctx context.Context, ref string, from, to int) (int64, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE database_servers SET port = ?, cluster = ?, updated_at = datetime('now')
		WHERE type_id = (SELECT id FROM database_server_types WHERE name = 'postgresql')
		  AND LOWER(COALESCE(host, 'localhost')) IN ('', 'localhost', '127.0.0.1', '::1', '[::1]')
		  AND (cluster = ? OR (cluster IS NULL AND port = ?))`, to, ref, ref, from)
	if err != nil {
		return 0, err
	}
	servers, _ := result.RowsAffected()
	if _, err := tx.ExecContext(ctx, `
		UPDATE databases SET port = ?, updated_at = datetime('now')
		WHERE LOWER(db_type) IN ('postgresql', 'postgres') AND port = ?
		  AND LOWER(COALESCE(host, 'localhost')) IN ('', 'localhost', '127.0.0.1', '::1', '[::1]')`, to, from); err != nil {
		return 0, err
	}
	return servers, tx.Commit()
}

func (p *Panel) handlePGPortChange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version        string `json:"version"`
		Name           string `json:"name"`
		Port           int    `json:"port"`
		ExpectedSHA256 string `json:"expected_sha256"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !pgClusterRefText.MatchString(pgRef(req.Version, req.Name)) || !sha256Text.MatchString(req.ExpectedSHA256) {
		writeClientError(w, http.StatusBadRequest, "the cluster, the port and the confirmed file are required")
		return
	}
	if req.Port < 1024 || req.Port > 65535 {
		writeCodedError(w, http.StatusBadRequest, "PG_PORT_INVALID", "choose a port from 1024 to 65535; ports below 1024 are for system services", "")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	ref := pgRef(req.Version, req.Name)
	response, err := setPGPortThroughAgent(p, r.Context(), transport.PostgreSQLSetPortRequest{
		PostgreSQLClusterRef: transport.PostgreSQLClusterRef{Version: req.Version, Name: req.Name},
		Port:                 req.Port, ExpectedSHA256: req.ExpectedSHA256,
	})
	details := map[string]any{"component": "postgresql", "cluster": ref, "from": strconv.Itoa(response.From), "to": strconv.Itoa(req.Port), "rolled_back": response.RolledBack, "code": response.ErrorCode}
	if err != nil {
		p.auditDetailed(r, "postgresql.cluster.port.failed:"+ref+" — "+auditReason(err.Error()), "service", 0, details)
		writeAgentError(w, err, "PostgreSQL cluster port")
		return
	}
	if !response.Proved {
		p.auditDetailed(r, "postgresql.cluster.port.failed:"+ref+" — "+auditReason(response.Error), "service", 0, details)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{
			"error": response.Error, "code": "PG_PORT_" + strings.TrimPrefix(response.ErrorCode, "PG_PORT_"),
			"rolled_back": response.RolledBack, "restored": response.Restored, "from": response.From, "to": req.Port,
		})
		return
	}
	updated, recordErr := p.updatePGRecordsPort(r.Context(), ref, response.From, req.Port)
	details["records"] = updated
	p.auditDetailed(r, "postgresql.cluster.port:"+ref+" "+strconv.Itoa(response.From)+"->"+strconv.Itoa(req.Port), "service", 0, details)
	body := map[string]any{"changed": true, "proved": true, "from": response.From, "to": req.Port, "records": updated}
	if recordErr != nil {
		log.Printf("postgresql cluster port: records of %s: %v", ref, recordErr)
		body["records_error"] = "CelikPanel's records could not be updated to the new port; use “Update CelikPanel's records” on this cluster"
	}
	json.NewEncoder(w).Encode(body)
}

func (p *Panel) handlePGRecordsSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !pgClusterRefText.MatchString(pgRef(req.Version, req.Name)) {
		writeClientError(w, http.StatusBadRequest, "a cluster is required")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	clusters, err := readPGClustersFromAgent(p, r.Context())
	if err != nil {
		writeAgentError(w, err, "PostgreSQL clusters")
		return
	}
	ref := pgRef(req.Version, req.Name)
	view, err := p.pgClustersViewFrom(r.Context(), clusters)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var found *pgRecordMismatch
	for i := range view.Mismatches {
		if view.Mismatches[i].Cluster == ref {
			found = &view.Mismatches[i]
		}
	}
	if found == nil {
		json.NewEncoder(w).Encode(map[string]any{"records": 0})
		return
	}
	updated, err := p.updatePGRecordsPort(r.Context(), ref, found.RecordPort, found.ClusterPort)
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "postgresql.records.port:"+ref+" "+strconv.Itoa(found.RecordPort)+"->"+strconv.Itoa(found.ClusterPort), "service", 0, map[string]any{
		"component": "postgresql", "cluster": ref, "from": strconv.Itoa(found.RecordPort), "to": strconv.Itoa(found.ClusterPort), "records": updated,
	})
	json.NewEncoder(w).Encode(map[string]any{"records": updated, "port": found.ClusterPort})
}

func pgFindCluster(clusters []transport.PostgreSQLCluster, port int) (transport.PostgreSQLCluster, bool) {
	for _, cluster := range clusters {
		if cluster.Port == port {
			return cluster, true
		}
	}
	return transport.PostgreSQLCluster{}, false
}

// pgTargetServerRecord finds or makes the subscription's record of the
// target cluster; a new one opens CelikPanel's own account on that cluster.
func (p *Panel) pgTargetServerRecord(ctx context.Context, subscriptionID int, ref, version string, port int) (int, error) {
	db := p.db.GetDB()
	var id int
	var status string
	err := db.QueryRowContext(ctx, `
		SELECT ds.id, COALESCE(ds.status, 'active') FROM database_servers ds
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE ds.subscription_id = ? AND dst.name = 'postgresql'
		  AND (ds.cluster = ? OR (ds.cluster IS NULL AND ds.port = ?))
		ORDER BY ds.id LIMIT 1`, subscriptionID, ref, port).Scan(&id, &status)
	if err == nil {
		if status == "retired" {
			if _, err := db.ExecContext(ctx, `UPDATE database_servers SET status = 'active', updated_at = datetime('now') WHERE id = ?`, id); err != nil {
				return 0, err
			}
		}
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	result, err := db.ExecContext(ctx, `
		INSERT INTO database_servers (subscription_id, type_id, name, version, host, port, is_default, status, cluster)
		SELECT ?, id, ?, ?, 'localhost', ?, 0, 'active', ? FROM database_server_types WHERE name = 'postgresql'`,
		subscriptionID, "PostgreSQL "+version, version, port, ref)
	if err != nil {
		return 0, err
	}
	newID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := provisionPGTargetAccount(p, ctx, subscriptionID); err != nil {
		log.Printf("postgresql move: CelikPanel's own account on %s: %v", ref, err)
	}
	return int(newID), nil
}

func (p *Panel) handlePGCopy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Database   string `json:"database"`
		SourcePort int    `json:"source_port"`
		TargetPort int    `json:"target_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !pgDatabaseText.MatchString(req.Database) ||
		req.SourcePort < 1 || req.SourcePort > 65535 || req.TargetPort < 1 || req.TargetPort > 65535 || req.SourcePort == req.TargetPort {
		writeClientError(w, http.StatusBadRequest, "a database, its cluster's port and another cluster's port are required")
		return
	}
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	clusters, err := readPGClustersFromAgent(p, r.Context())
	if err != nil {
		writeAgentError(w, err, "PostgreSQL clusters")
		return
	}
	source, okSource := pgFindCluster(clusters.Clusters, req.SourcePort)
	target, okTarget := pgFindCluster(clusters.Clusters, req.TargetPort)
	if !okSource || !okTarget {
		writeCodedError(w, http.StatusConflict, "PG_CLUSTER_NOT_FOUND", "a cluster on one of these ports is not on this server; read the clusters again", "")
		return
	}
	sourceRef, targetRef := pgRef(source.Version, source.Name), pgRef(target.Version, target.Name)
	var active int
	if err := p.db.GetDB().QueryRowContext(r.Context(), `SELECT COUNT(*) FROM postgresql_database_moves WHERE database_name = ? AND state IN ('copied', 'switched', 'switched_back')`, req.Database).Scan(&active); err != nil {
		writeServerError(w, err)
		return
	}
	if active > 0 {
		writeCodedError(w, http.StatusConflict, "PG_MOVE_ACTIVE", "this database already has a copy that is not finished; switch to it, or remove that copy, first", "")
		return
	}
	view, err := p.pgClustersViewFrom(r.Context(), clusters)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var record *pgPanelRecord
	for i, candidate := range view.Records[sourceRef] {
		if candidate.Name == req.Database && !candidate.Legacy {
			if record != nil {
				writeCodedError(w, http.StatusConflict, "PG_RECORD_AMBIGUOUS", "CelikPanel holds more than one record of this database on this cluster; nothing was copied", "")
				return
			}
			record = &view.Records[sourceRef][i]
		}
	}
	response, err := copyPGThroughAgent(p, r.Context(), transport.PostgreSQLCopyRequest{Database: req.Database, SourcePort: req.SourcePort, TargetPort: req.TargetPort})
	details := map[string]any{"component": "postgresql", "database": req.Database, "from": sourceRef, "to": targetRef,
		"tables": response.Proof.TablesTotal, "proved": response.Proof.Equal, "code": response.ErrorCode}
	if err != nil {
		p.auditDetailed(r, "postgresql.database.copy.failed:"+req.Database+" — "+auditReason(err.Error()), "service", 0, details)
		writeAgentError(w, err, "PostgreSQL database copy")
		return
	}
	if !response.Copied {
		p.auditDetailed(r, "postgresql.database.copy.failed:"+req.Database+" — "+auditReason(response.Error), "service", 0, details)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": response.Error, "code": "PG_COPY_" + response.ErrorCode, "proof": response.Proof, "cleaned": response.Cleaned})
		return
	}
	var databaseID, sourceServer, targetServer any
	if record != nil {
		databaseID, sourceServer = record.DatabaseID, record.ServerID
		id, err := p.pgTargetServerRecord(r.Context(), record.Subscription, targetRef, target.Version, target.Port)
		if err != nil {
			writeServerError(w, err)
			return
		}
		targetServer = id
	}
	proof := pgBoundedProof(response.Proof)
	result, err := p.db.GetDB().ExecContext(r.Context(), `
		INSERT INTO postgresql_database_moves (database_id, database_name, source_cluster, target_cluster, source_port, target_port, source_server_id, target_server_id, state, proof, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'copied', ?, ?)`,
		databaseID, req.Database, sourceRef, targetRef, source.Port, target.Port, sourceServer, targetServer, proof, currentUserID(r))
	if err != nil {
		writeServerError(w, err)
		return
	}
	moveID, _ := result.LastInsertId()
	p.auditDetailed(r, "postgresql.database.copy:"+req.Database+" "+sourceRef+"->"+targetRef, "service", int(moveID), details)
	json.NewEncoder(w).Encode(map[string]any{"copied": true, "move_id": moveID, "proof": response.Proof, "recorded": record != nil})
}

// pgBoundedProof is the proof as stored: its table list shortened until it
// fits the column.
func pgBoundedProof(proof transport.PostgreSQLCopyProof) string {
	for {
		data, _ := json.Marshal(proof)
		if len(data) <= 60000 || len(proof.Tables) == 0 {
			return string(data)
		}
		proof.Tables = proof.Tables[:len(proof.Tables)/2]
	}
}

func (p *Panel) pgMoveByID(ctx context.Context, id int) (*pgMoveView, error) {
	moves, err := p.pgMovesWhere(ctx, id)
	if err != nil || len(moves) == 0 {
		return nil, err
	}
	return &moves[0], nil
}

func (p *Panel) pgMovesWhere(ctx context.Context, id int) ([]pgMoveView, error) {
	var move pgMoveView
	var databaseID, sourceServer, targetServer sql.NullInt64
	var proof string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT id, database_id, database_name, source_cluster, target_cluster, source_port, target_port, state, proof, created_at, updated_at, source_server_id, target_server_id
		FROM postgresql_database_moves WHERE id = ?`, id).Scan(&move.ID, &databaseID, &move.Database, &move.SourceCluster, &move.TargetCluster, &move.SourcePort, &move.TargetPort, &move.State, &proof, &move.CreatedAt, &move.UpdatedAt, &sourceServer, &targetServer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	move.DatabaseID = nullableInt(databaseID)
	move.SourceServerID, move.TargetServerID = nullableInt(sourceServer), nullableInt(targetServer)
	move.Proof = json.RawMessage(proof)
	return []pgMoveView{move}, nil
}

// pgSwitchRecord points the panel's record of the database (and the grants'
// users) at the server record `to`, from `from`, in one transaction. Users
// are found or recorded on `to` with the same name and sealed password: the
// copy carried the role with the source's own definition.
func (p *Panel) pgSwitchRecord(ctx context.Context, databaseID, from, to, moveID int, fromState, toState string) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current int
	if err := tx.QueryRowContext(ctx, `SELECT server_id FROM databases_v2 WHERE id = ?`, databaseID).Scan(&current); err != nil {
		return fmt.Errorf("the database record is gone: %w", err)
	}
	if current != from {
		return errPGRecordMoved
	}
	rows, err := tx.QueryContext(ctx, `SELECT g.id, u.username, u.password, u.subscription_id FROM database_user_grants g JOIN database_users u ON u.id = g.user_id WHERE g.database_id = ?`, databaseID)
	if err != nil {
		return err
	}
	type grant struct {
		id                 int
		username, password string
		subscription       int
	}
	var grants []grant
	for rows.Next() {
		var g grant
		if err := rows.Scan(&g.id, &g.username, &g.password, &g.subscription); err != nil {
			rows.Close()
			return err
		}
		grants = append(grants, g)
	}
	rows.Close()
	for _, g := range grants {
		var userID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM database_users WHERE server_id = ? AND username = ?`, to, g.username).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			result, insertErr := tx.ExecContext(ctx, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, ?, ?)`, to, g.subscription, g.username, g.password)
			if insertErr != nil {
				return insertErr
			}
			userID, err = result.LastInsertId()
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE database_user_grants SET user_id = ? WHERE id = ?`, userID, g.id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE databases_v2 SET server_id = ?, updated_at = datetime('now') WHERE id = ?`, to, databaseID); err != nil {
		return err
	}
	// The record and the move's state change together.
	result, err := tx.ExecContext(ctx, `UPDATE postgresql_database_moves SET state = ?, updated_at = datetime('now') WHERE id = ? AND state = ?`, toState, moveID, fromState)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errPGRecordMoved
	}
	// A record the database now points at takes new databases again.
	if _, err := tx.ExecContext(ctx, `UPDATE database_servers SET status = 'active', updated_at = datetime('now') WHERE id = ? AND status = 'retired'`, to); err != nil {
		return err
	}
	return tx.Commit()
}

var errPGRecordMoved = errors.New("the database record no longer points where this move expects")

func (p *Panel) pgSetMoveState(ctx context.Context, id int, from, to string) error {
	result, err := p.db.GetDB().ExecContext(ctx, `UPDATE postgresql_database_moves SET state = ?, updated_at = datetime('now') WHERE id = ? AND state = ?`, to, id, from)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("the move changed meanwhile")
	}
	return nil
}

// pgDatabaseOnCluster reads, from the Agent, whether a cluster now has the
// database.
func pgDatabaseOnCluster(clusters transport.PostgreSQLClustersResponse, port int, name string) (known, present bool) {
	cluster, ok := pgFindCluster(clusters.Clusters, port)
	if !ok || !cluster.DatabasesKnown {
		return false, false
	}
	for _, database := range cluster.Databases {
		if database == name {
			return true, true
		}
	}
	return true, false
}

func (p *Panel) handlePGMoveAction(w http.ResponseWriter, r *http.Request, id int, action string) {
	release, busy := p.beginServiceMutation(w, r)
	if busy {
		return
	}
	defer release()
	ctx := r.Context()
	move, err := p.pgMoveByID(ctx, id)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if move == nil {
		writeClientError(w, http.StatusNotFound, "no such move")
		return
	}
	details := map[string]any{"component": "postgresql", "database": move.Database, "from": move.SourceCluster, "to": move.TargetCluster, "state": move.State}
	refuse := func(code, message string) {
		writeCodedError(w, http.StatusConflict, code, message, "")
	}
	switch action {
	case "switch", "switch-back":
		if move.DatabaseID == nil || move.SourceServerID == nil || move.TargetServerID == nil {
			refuse("PG_MOVE_UNRECORDED", "CelikPanel has no record of this database, so there is nothing to switch; point your application at the new port yourself")
			return
		}
		from, to, want, next := *move.SourceServerID, *move.TargetServerID, "copied", "switched"
		checkPort := move.TargetPort
		if action == "switch-back" {
			from, to, want, next, checkPort = *move.TargetServerID, *move.SourceServerID, "switched", "switched_back", move.SourcePort
		} else if move.State == "switched_back" {
			want = "switched_back"
		}
		if move.State != want {
			refuse("PG_MOVE_STATE", "this move is "+move.State+"; that step is not possible now")
			return
		}
		clusters, err := readPGClustersFromAgent(p, ctx)
		if err != nil {
			writeAgentError(w, err, "PostgreSQL clusters")
			return
		}
		if known, present := pgDatabaseOnCluster(clusters, checkPort, move.Database); !known || !present {
			refuse("PG_MOVE_TARGET_MISSING", fmt.Sprintf("database %s is not seen on the cluster on port %d now, so CelikPanel's record was not switched", move.Database, checkPort))
			return
		}
		// alpha.108 review: the copy proves rows at copy time only; a switch
		// to it is refused when the source was written after the copy.
		if action == "switch" {
			expect, ok := pgMoveExpectation(move)
			if !ok {
				refuse("PG_SOURCE_UNKNOWN", "this copy's proof has no record of the source's writes, so whether the source changed since cannot be proved; copy it again; nothing was switched")
				return
			}
			check, err := checkPGSourceThroughAgent(p, ctx, transport.PostgreSQLSourceCheckRequest{Database: move.Database, Port: move.SourcePort, Expect: expect})
			if err != nil {
				writeAgentError(w, err, "PostgreSQL source check")
				return
			}
			if !check.Unchanged {
				details["code"] = check.Code
				p.auditDetailed(r, "postgresql.database.switch.failed:"+move.Database+" — "+check.Code, "service", id, details)
				refuse("PG_"+check.Code, check.Error)
				return
			}
		}
		if err := p.pgSwitchRecord(ctx, *move.DatabaseID, from, to, id, move.State, next); err != nil {
			if errors.Is(err, errPGRecordMoved) {
				refuse("PG_RECORD_MOVED", "CelikPanel's record of this database no longer points at the cluster this move expects; nothing was switched")
				return
			}
			writeServerError(w, err)
			return
		}
		auditName := "postgresql.database.switch"
		if action == "switch-back" {
			auditName = "postgresql.database.switch_back"
		}
		details["state"] = next
		p.auditDetailed(r, auditName+":"+move.Database+" "+move.SourceCluster+"->"+move.TargetCluster, "service", id, details)
		json.NewEncoder(w).Encode(map[string]any{"state": next, "port": checkPort})
	case "drop-source", "drop-copy":
		port, keep, next := move.SourcePort, move.TargetPort, "source_dropped"
		allowed := move.State == "switched" || (move.DatabaseID == nil && move.State == "copied")
		if action == "drop-copy" {
			port, keep, next = move.TargetPort, move.SourcePort, "copy_dropped"
			allowed = move.State == "copied" || move.State == "switched_back"
		}
		if !allowed {
			refuse("PG_MOVE_STATE", "this move is "+move.State+"; that database is still the one CelikPanel's record uses, so it is not dropped")
			return
		}
		drop := transport.PostgreSQLDropRequest{Database: move.Database, Port: port, KeepPort: keep}
		if action == "drop-source" {
			// alpha.108 review: the source goes only while it is as it was at
			// the copy; writes after it would be lost.
			expect, ok := pgMoveExpectation(move)
			if !ok {
				refuse("PG_SOURCE_UNKNOWN", "this copy's proof has no record of the source's writes, so whether the source changed since cannot be proved; copy it again; nothing was removed")
				return
			}
			drop.ExpectSource = &expect
		}
		response, err := dropPGThroughAgent(p, ctx, drop)
		details["code"] = response.ErrorCode
		if err != nil || !response.Dropped {
			reason := response.Error
			if err != nil && reason == "" {
				reason = err.Error()
			}
			p.auditDetailed(r, "postgresql.database.drop.failed:"+move.Database+" — "+auditReason(reason), "service", id, details)
			if err != nil && response.Error == "" {
				writeAgentError(w, err, "PostgreSQL database drop")
				return
			}
			writeCodedError(w, http.StatusConflict, "PG_DROP_"+response.ErrorCode, response.Error, "")
			return
		}
		if err := p.pgSetMoveState(ctx, id, move.State, next); err != nil {
			writeServerError(w, err)
			return
		}
		if next == "source_dropped" && move.SourceServerID != nil {
			p.retirePGServerRecordIfEmpty(ctx, *move.SourceServerID)
		}
		details["state"] = next
		auditName := "postgresql.database.drop_source"
		if next == "copy_dropped" {
			auditName = "postgresql.database.drop_copy"
		}
		p.auditDetailed(r, auditName+":"+move.Database+" "+pgMoveDroppedRef(move, next), "service", id, details)
		json.NewEncoder(w).Encode(map[string]any{"state": next, "dropped": true})
	}
}

func pgMoveDroppedRef(move *pgMoveView, next string) string {
	if next == "copy_dropped" {
		return move.TargetCluster
	}
	return move.SourceCluster
}

// retirePGServerRecordIfEmpty: a subscription's record of a cluster that
// holds none of its databases any more takes no new database (status
// retired); a later move back to that cluster makes it active again.
func (p *Panel) retirePGServerRecordIfEmpty(ctx context.Context, serverID int) {
	if _, err := p.db.GetDB().ExecContext(ctx, `
		UPDATE database_servers SET status = 'retired', updated_at = datetime('now')
		WHERE id = ? AND NOT EXISTS (SELECT 1 FROM databases_v2 WHERE server_id = ?)
		  AND NOT EXISTS (SELECT 1 FROM postgresql_database_moves WHERE (source_server_id = ? OR target_server_id = ?) AND state IN ('copied', 'switched', 'switched_back'))`,
		serverID, serverID, serverID, serverID); err != nil {
		log.Printf("postgresql move: retire record %d: %v", serverID, err)
	}
}

// pgDistributionOffers reports, from the Agent's clusters read, whether the
// distribution itself offers a postgresql-NN package, so its exact install
// needs no vendor repository (alpha.108).
func (p *Panel) pgDistributionOffers(ctx context.Context, pkg string) bool {
	clusters, err := readPGClustersFromAgent(p, ctx)
	if err != nil {
		return false
	}
	for _, offer := range clusters.Offers {
		if offer.Package == pkg && offer.Origin == "distribution" {
			return true
		}
	}
	return false
}
