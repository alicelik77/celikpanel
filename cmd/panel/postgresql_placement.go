package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Where a new PostgreSQL database goes (alpha.127). Boston, 6 Nov 2026: the
// owner removed PostgreSQL 16 (its data kept, so its cluster 16/main still
// claims port 5432, "down,binaries_missing") and installed PostgreSQL 18,
// which Debian put on the next free port, 5433. Autodiscovery had registered
// the subscription's PostgreSQL record with the engine type's default port,
// 5432, so every database statement went to the cluster that had no program
// left, the panel's own account was never opened, and the owner saw
// "internal error" for a plain database create.
//
// A record now says the cluster that runs: a record that names a cluster
// which does not run and holds nothing of the panel (no database, no user,
// no unfinished move) is set to the running cluster; a record that holds
// something is never moved (its databases live in that cluster's kept data),
// and a new database goes to a record of the running cluster instead. With
// no running cluster the create is refused with what is wrong, who acts and
// what to do next (D-024), before anything runs. Data is never touched.
//
// Yeni bir PostgreSQL veritabanı çalışan kümeye gider. Çalışmayan bir kümeyi
// gösteren ve panelden hiçbir şey taşımayan kayıt çalışan kümeye çevrilir;
// bir şey taşıyan kayıt asla taşınmaz. Çalışan küme yoksa oluşturma, ne
// olduğunu, kimin ne yapacağını söyleyerek hiçbir şey çalışmadan reddedilir.

const (
	errCodePGClusterUnavailable = "PG_CLUSTER_UNAVAILABLE"
	pgPlacementReadBound        = 30 * time.Second
)

// pgSubscriptionRecord is one PostgreSQL record of a subscription with
// whether it holds anything of the panel.
type pgSubscriptionRecord struct {
	pgServerRecord
	Holds bool
}

func (p *Panel) pgSubscriptionRecords(ctx context.Context, subscriptionID int, includeRetired bool) ([]pgSubscriptionRecord, error) {
	query := `
		SELECT ds.id, ds.subscription_id, ds.port, COALESCE(ds.cluster, ''), COALESCE(ds.status, 'active'), COALESCE(ds.host, ''),
		       EXISTS(SELECT 1 FROM databases_v2 d WHERE d.server_id = ds.id)
		       OR EXISTS(SELECT 1 FROM database_users u WHERE u.server_id = ds.id)
		       OR EXISTS(SELECT 1 FROM postgresql_database_moves m
		                 WHERE (m.source_server_id = ds.id OR m.target_server_id = ds.id)
		                   AND m.state IN ('copied', 'switched', 'switched_back'))
		FROM database_servers ds
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE ds.subscription_id = ? AND dst.name = 'postgresql'`
	if !includeRetired {
		query += ` AND COALESCE(ds.status, 'active') <> 'retired'`
	}
	query += ` ORDER BY ds.is_default DESC, ds.id`
	rows, err := p.db.GetDB().QueryContext(ctx, query, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pgSubscriptionRecord
	for rows.Next() {
		var record pgSubscriptionRecord
		if err := rows.Scan(&record.ID, &record.SubscriptionID, &record.Port, &record.Cluster, &record.Status, &record.Host, &record.Holds); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// pgClusterOfRecord is the cluster a record names, when it is on this server.
func pgClusterOfRecord(record pgServerRecord, clusters []transport.PostgreSQLCluster) (transport.PostgreSQLCluster, bool) {
	ref, ok := pgServerCluster(record, clusters)
	if !ok {
		return transport.PostgreSQLCluster{}, false
	}
	for _, cluster := range clusters {
		if pgRef(cluster.Version, cluster.Name) == ref {
			return cluster, true
		}
	}
	return transport.PostgreSQLCluster{}, false
}

// pgNewestRunningCluster is where a new database goes when no record names a
// running cluster: the newest major that runs (the lowest port on a tie).
func pgNewestRunningCluster(clusters []transport.PostgreSQLCluster) (transport.PostgreSQLCluster, bool) {
	var best transport.PostgreSQLCluster
	found := false
	for _, cluster := range clusters {
		if !cluster.Online {
			continue
		}
		if !found || pgMajorOf(cluster) > pgMajorOf(best) || (pgMajorOf(cluster) == pgMajorOf(best) && cluster.Port < best.Port) {
			best, found = cluster, true
		}
	}
	return best, found
}

func pgMajorOf(cluster transport.PostgreSQLCluster) int {
	major, _ := strconv.Atoi(cluster.Version)
	return major
}

// pgClusterRefusal describes why no running cluster can take a new database:
// the cluster the subscription's record names, else the newest one on the
// server, else none at all.
func pgClusterRefusal(records []pgSubscriptionRecord, clusters []transport.PostgreSQLCluster) *transport.DatabaseClusterRefusal {
	var named *transport.PostgreSQLCluster
	for _, record := range records {
		if cluster, ok := pgClusterOfRecord(record.pgServerRecord, clusters); ok {
			named = &cluster
			break
		}
	}
	if named == nil {
		for i := range clusters {
			if named == nil || pgMajorOf(clusters[i]) > pgMajorOf(*named) {
				named = &clusters[i]
			}
		}
	}
	if named == nil {
		return &transport.DatabaseClusterRefusal{Reason: "none"}
	}
	refusal := &transport.DatabaseClusterRefusal{
		Reason: "stopped", Version: named.Version, Cluster: pgRef(named.Version, named.Name),
		Port: named.Port, DataDirectory: named.DataDirectory,
	}
	if strings.Contains(named.Status, "binaries_missing") {
		refusal.Reason = "removed"
	}
	if running, ok := pgNewestRunningCluster(clusters); ok {
		refusal.RunningVersion, refusal.RunningPort = running.Version, running.Port
	}
	return refusal
}

// pgRepointEmptyRecords sets the subscription's records that name a cluster
// which does not run, and hold nothing of the panel, to the running cluster.
// Nothing changes when a record already names the running cluster (even a
// retired one: the placement brings it back), when no cluster runs, or when
// every such record holds something. The panel's own account on the record
// is cleared with it: it was for another cluster. Returns whether a record
// changed.
func (p *Panel) pgRepointEmptyRecords(ctx context.Context, subscriptionID int, clusters []transport.PostgreSQLCluster) (bool, error) {
	target, ok := pgNewestRunningCluster(clusters)
	if !ok {
		return false, nil
	}
	ref := pgRef(target.Version, target.Name)
	records, err := p.pgSubscriptionRecords(ctx, subscriptionID, true)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if !databaseEngineIsOnThisMachine(record.Host) {
			continue
		}
		if cluster, ok := pgClusterOfRecord(record.pgServerRecord, clusters); ok && cluster.Online {
			// A record of a running cluster is already there to use.
			return false, nil
		}
		if record.Port == target.Port || record.Cluster == ref {
			return false, nil
		}
	}
	for _, record := range records {
		if record.Status == "retired" || record.Holds || !databaseEngineIsOnThisMachine(record.Host) {
			continue
		}
		result, err := p.db.GetDB().ExecContext(ctx, `
			UPDATE database_servers
			SET port = ?, cluster = ?, version = ?, admin_username = NULL, root_password_encrypted = NULL, updated_at = datetime('now')
			WHERE id = ?
			  AND NOT EXISTS (SELECT 1 FROM databases_v2 WHERE server_id = ?)
			  AND NOT EXISTS (SELECT 1 FROM database_users WHERE server_id = ?)`,
			target.Port, ref, target.Version, record.ID, record.ID, record.ID)
		if err != nil {
			return false, err
		}
		if changed, _ := result.RowsAffected(); changed == 1 {
			log.Printf("postgresql: record %d of subscription %d named port %d, where no cluster runs; it now names %s on port %d (it held no database)",
				record.ID, subscriptionID, record.Port, ref, target.Port)
			return true, nil
		}
	}
	return false, nil
}

// pgPlaceNewDatabase is the record and port a new PostgreSQL database of the
// subscription goes on, or the refusal that says why none can take it.
// sql.ErrNoRows: the subscription has no PostgreSQL record at all.
func (p *Panel) pgPlaceNewDatabase(ctx context.Context, subscriptionID int, clusters transport.PostgreSQLClustersResponse) (int, int, *transport.DatabaseClusterRefusal, error) {
	if clusters.Supported {
		changed, err := p.pgRepointEmptyRecords(ctx, subscriptionID, clusters.Clusters)
		if err != nil {
			return 0, 0, nil, err
		}
		if changed {
			if err := provisionPGTargetAccount(p, ctx, subscriptionID); err != nil {
				log.Printf("postgresql: CelikPanel's own account on the running cluster: %v", err)
			}
		}
	}
	records, err := p.pgSubscriptionRecords(ctx, subscriptionID, false)
	if err != nil {
		return 0, 0, nil, err
	}
	if len(records) == 0 {
		return 0, 0, nil, sql.ErrNoRows
	}
	if !clusters.Supported {
		// One PostgreSQL without postgresql-common, or the clusters could not
		// be read: the record as it is, as before.
		return records[0].ID, records[0].Port, nil, nil
	}
	for _, record := range records {
		if !databaseEngineIsOnThisMachine(record.Host) {
			return record.ID, record.Port, nil, nil
		}
		if cluster, ok := pgClusterOfRecord(record.pgServerRecord, clusters.Clusters); ok && cluster.Online {
			return record.ID, cluster.Port, nil, nil
		}
	}
	target, ok := pgNewestRunningCluster(clusters.Clusters)
	if !ok {
		return 0, 0, pgClusterRefusal(records, clusters.Clusters), nil
	}
	id, err := p.pgTargetServerRecord(ctx, subscriptionID, pgRef(target.Version, target.Name), target.Version, target.Port)
	if err != nil {
		return 0, 0, nil, err
	}
	return id, target.Port, nil, nil
}

// readPGClustersForPlacement reads the clusters for a placement; an Agent
// that cannot say is "not supported" here, and the record is used as it is.
func (p *Panel) readPGClustersForPlacement(ctx context.Context) transport.PostgreSQLClustersResponse {
	readCtx, cancel := context.WithTimeout(ctx, pgPlacementReadBound)
	defer cancel()
	clusters, err := readPGClustersFromAgent(p, readCtx)
	if err != nil {
		log.Printf("postgresql: clusters could not be read for a placement: %v", err)
		return transport.PostgreSQLClustersResponse{}
	}
	return clusters
}

// writeDatabaseClusterRefusal answers a PostgreSQL change that did not run
// because its cluster cannot take it: a verified prerequisite (409) with the
// facts the page says it with. The fix belongs to the server administrator;
// only an administrator gets the path to it.
func writeDatabaseClusterRefusal(w http.ResponseWriter, r *http.Request, refusal *transport.DatabaseClusterRefusal) {
	vars := map[string]string{
		"version": refusal.Version, "cluster": refusal.Cluster, "data": refusal.DataDirectory,
		"unit": "postgresql@" + strings.Replace(refusal.Cluster, "/", "-", 1),
	}
	if refusal.Port > 0 {
		vars["port"] = strconv.Itoa(refusal.Port)
	}
	if refusal.RunningVersion != "" {
		vars["running_version"], vars["running_port"] = refusal.RunningVersion, strconv.Itoa(refusal.RunningPort)
	}
	if refusal.BackupVersion != "" {
		vars["backup_version"] = refusal.BackupVersion
	}
	action := ""
	if caller := currentCaller(r); caller != nil && caller.hasAccountRole(roleAdmin) {
		action = "/components/postgresql"
	}
	message := "nothing was changed: no running PostgreSQL cluster can take this change"
	switch refusal.Reason {
	case "removed":
		message = "nothing was changed: PostgreSQL " + refusal.Version + " was removed from this server; its cluster " + refusal.Cluster + " keeps its data in " + refusal.DataDirectory + " but cannot answer"
	case "stopped":
		message = "nothing was changed: PostgreSQL " + refusal.Version + " (cluster " + refusal.Cluster + ") is installed but not running"
	case "none":
		message = "nothing was changed: there is no PostgreSQL cluster on this server"
	case "major_mismatch":
		message = "nothing was restored: this backup was made from PostgreSQL " + refusal.BackupVersion + ", and the database's cluster " + refusal.Cluster + " is PostgreSQL " + refusal.Version
	case "unplaced":
		message = "nothing was restored: this backup was made from PostgreSQL " + refusal.BackupVersion + ", and no PostgreSQL cluster is on the port the database's record names"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(apiErrorBody{Error: message, Code: errCodePGClusterUnavailable, Reason: refusal.Reason, Action: action, Vars: vars})
}
