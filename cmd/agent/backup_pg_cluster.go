package main

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/alicelik/celikpanel/internal/backupspec"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Which PostgreSQL cluster a backup reads and a restore writes (alpha.127).
// A backup names no cluster before alpha.127, so pg_dump without a port went
// to whatever psql chose: on Boston the removed PostgreSQL 16's kept cluster,
// and on a server with majors side by side the default cluster even for a
// database on another one. Now the panel sends its record's port, the cluster
// on that port is resolved when the backup is made (it must run), and its
// major and port go into the manifest. A restore goes to the database's
// cluster now, only when that cluster runs and is the backup's major: a dump
// of one major is never fed into another one without the owner choosing it.
// A backup made before alpha.127 names no major and keeps the earlier rule:
// the database's cluster now, or the running cluster when none is named.
//
// Yedek, kaydın portundaki kümeden alınır ve küme bilgisi manifeste yazılır;
// geri yükleme yalnız aynı ana sürümün çalışan kümesine gider.

var pgClusterVersionRe = regexp.MustCompile(`^[0-9]{1,3}$`)

func validDatabaseCluster(database backupspec.DatabaseIdentity) error {
	if database.ClusterPort < 0 || database.ClusterPort > 65535 {
		return fmt.Errorf("invalid database cluster port %d", database.ClusterPort)
	}
	if database.ClusterVersion != "" && !pgClusterVersionRe.MatchString(database.ClusterVersion) {
		return fmt.Errorf("invalid database cluster version")
	}
	if database.Type != "postgresql" && (database.ClusterVersion != "" || database.ClusterPort != 0) {
		return fmt.Errorf("only a PostgreSQL database names a cluster")
	}
	return nil
}

// pgBackupRefusalError: a restore that would cross majors.
type pgBackupRefusalError struct {
	refusal backupspec.ClusterRefusal
}

func (e *pgBackupRefusalError) Error() string {
	r := e.refusal
	return fmt.Sprintf("this backup was made from PostgreSQL %s; the database's cluster %s (port %d) is PostgreSQL %s, so nothing was restored", r.BackupVersion, r.Cluster, r.Port, r.Version)
}

// backupClusterRefusalOf is the refusal an error carries, in backup terms.
func backupClusterRefusalOf(err error) *backupspec.ClusterRefusal {
	if refusal := pgClusterRefusalOf(err); refusal != nil {
		value := backupspec.ClusterRefusal(*refusal)
		return &value
	}
	var crossing *pgBackupRefusalError
	if errors.As(err, &crossing) {
		value := crossing.refusal
		return &value
	}
	return nil
}

func pgClusterOnPort(clusters []transport.PostgreSQLCluster, port int) (transport.PostgreSQLCluster, bool) {
	for _, cluster := range clusters {
		if cluster.Port == port {
			return cluster, true
		}
	}
	return transport.PostgreSQLCluster{}, false
}

// pgDatabaseCluster is the cluster a database identity names now: the one on
// its port, else (no port) the running one. found is false where no
// postgresql-common cluster answers for it and the port is left to psql.
func pgDatabaseCluster(database backupspec.DatabaseIdentity) (transport.PostgreSQLCluster, bool, error) {
	clusters, ok := pgStatementClusters()
	if !ok || len(clusters) == 0 {
		return transport.PostgreSQLCluster{}, false, nil
	}
	if database.ClusterPort > 0 {
		cluster, found := pgClusterOnPort(clusters, database.ClusterPort)
		if !found {
			return transport.PostgreSQLCluster{}, false, nil
		}
		if !cluster.Online {
			return cluster, true, pgClusterRefusalFor(cluster, clusters)
		}
		return cluster, true, nil
	}
	port, err := resolvePGStatementPort(0)
	if err != nil {
		return transport.PostgreSQLCluster{}, true, err
	}
	cluster, found := pgClusterOnPort(clusters, port)
	return cluster, found, nil
}

// resolveBackupCluster is the identity a backup is made and recorded with:
// a PostgreSQL database's running cluster, its major and port.
func resolveBackupCluster(database backupspec.DatabaseIdentity) (backupspec.DatabaseIdentity, error) {
	if database.Type != "postgresql" {
		return database, nil
	}
	cluster, found, err := pgDatabaseCluster(database)
	if err != nil {
		return database, err
	}
	if found {
		database.ClusterVersion, database.ClusterPort = cluster.Version, cluster.Port
	}
	return database, nil
}

// restoreDatabaseTarget is where a database of a backup is restored: the
// database's cluster now (requested), when it runs and is the major the
// backup was made from. A backup without a major (before alpha.127) goes to
// that cluster as before.
func restoreDatabaseTarget(stored, requested backupspec.DatabaseIdentity) (backupspec.DatabaseIdentity, error) {
	if requested.Type != "postgresql" {
		return requested, nil
	}
	target := requested
	target.ClusterVersion = ""
	cluster, found, err := pgDatabaseCluster(target)
	if err != nil {
		return target, err
	}
	if !found {
		if stored.ClusterVersion != "" {
			// The backup's major cannot be proved to be the target's.
			clusters, _ := pgStatementClusters()
			refusal := backupspec.ClusterRefusal{Reason: "unplaced", Port: target.ClusterPort, BackupVersion: stored.ClusterVersion}
			if running, ok := pgRunningCluster(clusters); ok {
				refusal.RunningVersion, refusal.RunningPort = running.Version, running.Port
			}
			if len(clusters) > 0 {
				return target, &pgBackupRefusalError{refusal: refusal}
			}
			// No postgresql-common at all: one PostgreSQL, as before.
		}
		return target, nil
	}
	if stored.ClusterVersion != "" && stored.ClusterVersion != cluster.Version {
		return target, &pgBackupRefusalError{refusal: backupspec.ClusterRefusal{
			Reason: "major_mismatch", Version: cluster.Version, Cluster: pgClusterRef(cluster.Version, cluster.Name),
			Port: cluster.Port, DataDirectory: cluster.DataDirectory, BackupVersion: stored.ClusterVersion,
		}}
	}
	target.ClusterVersion, target.ClusterPort = cluster.Version, cluster.Port
	return target, nil
}
