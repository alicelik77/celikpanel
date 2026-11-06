package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Which PostgreSQL cluster a statement goes to (alpha.127). Boston, 6 Nov
// 2026: PostgreSQL 16 had been removed with its data kept (its cluster
// 16/main still configured for port 5432, "down,binaries_missing"), and
// PostgreSQL 18 had been installed after it, on the next free port, 5433.
// The panel's record said 5432, so every database statement went to a
// cluster that had no program left to answer, and the owner saw "internal
// error". A statement now goes only to a cluster that runs: the one named by
// port when it runs, else a typed refusal that says which cluster it is and
// why it cannot answer; with no port, the running cluster on the default
// port, else the newest running one. Where postgresql-common is not there
// (one PostgreSQL, no pg_lsclusters) nothing changes: psql's own default.
//
// Bir ifade yalnız çalışan bir kümeye gider: port adıyla verilen küme
// çalışıyorsa o, değilse kümenin hangisi olduğunu ve neden yanıt veremediğini
// söyleyen türlü bir ret; port yoksa varsayılan porttaki çalışan küme, o da
// yoksa en yeni çalışan küme. postgresql-common yoksa hiçbir şey değişmez.

// pgStatementClusters reads the clusters a statement could go to; ok is
// false where they cannot be read (a variable for tests).
var pgStatementClusters = func() ([]transport.PostgreSQLCluster, bool) {
	host := pgClusterHostActive()
	if host.family() != "apt" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgReadBound)
	defer cancel()
	out, err := host.lsclusters(ctx)
	if err != nil {
		return nil, false
	}
	return parsePGLsClusters(out), true
}

// pgClusterRefusalError is a verified prerequisite: the cluster a statement
// would go to cannot take it, and nothing ran.
type pgClusterRefusalError struct {
	refusal transport.DatabaseClusterRefusal
}

func (e *pgClusterRefusalError) Error() string {
	r := e.refusal
	running := ""
	if r.RunningVersion != "" {
		running = fmt.Sprintf("; PostgreSQL %s runs on port %d", r.RunningVersion, r.RunningPort)
	}
	switch r.Reason {
	case "removed":
		return fmt.Sprintf("PostgreSQL %s (cluster %s, port %d) cannot answer: its program was removed and its data was kept in %s%s", r.Version, r.Cluster, r.Port, r.DataDirectory, running)
	case "stopped":
		return fmt.Sprintf("PostgreSQL %s (cluster %s, port %d) is installed but not running%s", r.Version, r.Cluster, r.Port, running)
	}
	return "no PostgreSQL cluster runs on this server"
}

// pgClusterRefusalOf returns the refusal an error carries, if it is one.
func pgClusterRefusalOf(err error) *transport.DatabaseClusterRefusal {
	var refusal *pgClusterRefusalError
	if errors.As(err, &refusal) {
		value := refusal.refusal
		return &value
	}
	return nil
}

func pgClusterRemoved(cluster transport.PostgreSQLCluster) bool {
	return strings.Contains(cluster.Status, "binaries_missing")
}

func pgClusterNewer(a, b transport.PostgreSQLCluster) bool {
	va, _ := strconv.Atoi(a.Version)
	vb, _ := strconv.Atoi(b.Version)
	if va != vb {
		return va > vb
	}
	return a.Port < b.Port
}

// pgRunningCluster is the cluster a statement without a port goes to: the
// running one on the default port, else the newest running one.
func pgRunningCluster(clusters []transport.PostgreSQLCluster) (transport.PostgreSQLCluster, bool) {
	var best transport.PostgreSQLCluster
	found := false
	for _, cluster := range clusters {
		if !cluster.Online {
			continue
		}
		if cluster.Port == pgDefaultPort {
			return cluster, true
		}
		if !found || pgClusterNewer(cluster, best) {
			best, found = cluster, true
		}
	}
	return best, found
}

// pgClusterRefusalFor describes why cluster cannot take a statement.
func pgClusterRefusalFor(cluster transport.PostgreSQLCluster, clusters []transport.PostgreSQLCluster) *pgClusterRefusalError {
	refusal := transport.DatabaseClusterRefusal{
		Reason: "stopped", Version: cluster.Version, Cluster: pgClusterRef(cluster.Version, cluster.Name),
		Port: cluster.Port, DataDirectory: cluster.DataDirectory,
	}
	if pgClusterRemoved(cluster) {
		refusal.Reason = "removed"
	}
	if running, ok := pgRunningCluster(clusters); ok {
		refusal.RunningVersion, refusal.RunningPort = running.Version, running.Port
	}
	return &pgClusterRefusalError{refusal: refusal}
}

// resolvePGStatementPort is the port a statement for port goes to, or a
// refusal. Port 0 resolves to the running cluster when clusters are read.
func resolvePGStatementPort(port int) (int, error) {
	clusters, ok := pgStatementClusters()
	if !ok || len(clusters) == 0 {
		return port, nil
	}
	if port > 0 {
		for _, cluster := range clusters {
			if cluster.Port == port {
				if cluster.Online {
					return port, nil
				}
				return 0, pgClusterRefusalFor(cluster, clusters)
			}
		}
		// Not a postgresql-common cluster: psql says what is there.
		return port, nil
	}
	if running, ok := pgRunningCluster(clusters); ok {
		return running.Port, nil
	}
	// Nothing runs: the newest cluster says why.
	newest := clusters[0]
	for _, cluster := range clusters[1:] {
		if pgClusterNewer(cluster, newest) {
			newest = cluster
		}
	}
	return 0, pgClusterRefusalFor(newest, clusters)
}
