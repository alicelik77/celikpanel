package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Connection limits of database users (alpha.128, gap G5,
// docs/SITE-LIMITS.md). The limit is the effective db.max_user_connections
// of the user's subscription (server ⊇ reseller ⊇ plan or Custom). It is
// written on the engine when a user is created, and again for every user of
// the subscriptions a changed limit reaches (a plan, a reseller plan, a
// Custom set, the server's policy), through the same function. No limit
// writes nothing for a new user and removes an earlier one for an existing
// user.
//
// Veritabanı kullanıcılarının bağlantı sınırı; kullanıcı oluşturulurken ve
// sınır değişince aynı yoldan motorun kendisine yazılır.

// dbConnectionLimit is the limit of a subscription's database users (0:
// none).
func (p *Panel) dbConnectionLimit(ctx context.Context, subscriptionID int) (int, error) {
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subscriptionID)
	if err != nil {
		return 0, err
	}
	return limits.ceilingValue(settingDBConnections), nil
}

// applyNewUserConnectionLimit writes the limit for a user just created on
// server through driver; a subscription without a limit writes nothing. A
// PostgreSQL limit goes to the role's own running cluster
// (connectionLimiterFor), never to whichever cluster answers.
func (p *Panel) applyNewUserConnectionLimit(ctx context.Context, server *core.DatabaseServer, driver services.DatabaseDriver, subscriptionID int, username string) error {
	limit, err := p.dbConnectionLimit(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("read the connection limit: %w", err)
	}
	if limit == 0 {
		return nil
	}
	limiter, ok, err := p.connectionLimiterFor(ctx, server, driver, p.pgClustersOnce(ctx))
	if err != nil {
		return err
	}
	if !ok {
		return nil // an engine without per-user connection limits
	}
	return limiter.SetUserConnectionLimit(username, limit)
}

// pgClustersOnce reads this server's PostgreSQL clusters at most once, when
// a PostgreSQL limit is first written.
func (p *Panel) pgClustersOnce(ctx context.Context) func() transport.PostgreSQLClustersResponse {
	var (
		read     bool
		clusters transport.PostgreSQLClustersResponse
	)
	return func() transport.PostgreSQLClustersResponse {
		if !read {
			clusters, read = p.readPGClustersForPlacement(ctx), true
		}
		return clusters
	}
}

// connectionLimiterFor is the driver that writes a user's limit on the
// user's own server. MariaDB: driver (or, nil, the record's own). PostgreSQL
// on this machine follows the alpha.127 placement rule: the cluster the
// record names (by name; by port only for a record made before schema 46)
// when it runs, reached at that cluster's own port. A record whose cluster
// does not run, or is not on this server, is a typed refusal and nothing is
// written; the limit is written at the next change. Where the clusters
// cannot be read (no postgresql-common, one PostgreSQL) or the engine is on
// another machine, the record as it is, as every other statement.
//
// PostgreSQL sınırı rolün kendi çalışan kümesine gider; o küme çalışmıyorsa
// hiçbir şey yazılmaz.
func (p *Panel) connectionLimiterFor(ctx context.Context, server *core.DatabaseServer, driver services.DatabaseDriver, clusters func() transport.PostgreSQLClustersResponse) (services.ConnectionLimiter, bool, error) {
	if dbDriverTypeFor(server) == "postgresql" && databaseEngineIsOnThisMachine(server.Host) {
		if read := clusters(); read.Supported {
			port, err := p.pgRoleClusterPort(ctx, server, read.Clusters)
			if err != nil {
				return nil, false, err
			}
			if port != server.Port || driver == nil {
				placed := *server
				placed.Port = port
				placedDriver, err := p.dbDriverFor(&placed)
				if err != nil {
					return nil, false, err
				}
				driver = placedDriver
			}
		}
	}
	if driver == nil {
		recordDriver, err := p.dbDriverFor(server)
		if err != nil {
			return nil, false, err
		}
		driver = recordDriver
	}
	limiter, ok := driver.(services.ConnectionLimiter)
	return limiter, ok, nil
}

// pgRoleClusterPort is the port of the running cluster a PostgreSQL record
// names, or the refusal that says why its roles cannot be reached.
func (p *Panel) pgRoleClusterPort(ctx context.Context, server *core.DatabaseServer, clusters []transport.PostgreSQLCluster) (int, error) {
	record := pgServerRecord{ID: server.ID, SubscriptionID: server.SubscriptionID, Port: server.Port, Host: server.Host}
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COALESCE(cluster, '') FROM database_servers WHERE id = ?`, server.ID).Scan(&record.Cluster); err != nil {
		return 0, fmt.Errorf("read the cluster of PostgreSQL record %d: %w", server.ID, err)
	}
	cluster, ok := pgClusterOfRecord(record, clusters)
	if !ok {
		return 0, &pgClusterRefusalError{refusal: transport.DatabaseClusterRefusal{Reason: "none", Cluster: record.Cluster, Port: record.Port}}
	}
	if !cluster.Online {
		refusal := pgClusterRefusal([]pgSubscriptionRecord{{pgServerRecord: record}}, clusters)
		return 0, &pgClusterRefusalError{refusal: *refusal}
	}
	return cluster.Port, nil
}

// reapplyDBConnectionLimits writes the current limit for every database
// user of the given subscriptions, in the background. A user whose server
// cannot be reached keeps its limit until the next change; it is logged.
func (p *Panel) reapplyDBConnectionLimits(subscriptionIDs []int) {
	if len(subscriptionIDs) == 0 {
		return
	}
	ids := append([]int(nil), subscriptionIDs...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := p.reapplyDBConnectionLimitsNow(ctx, ids); err != nil {
			log.Printf("database connection limits: %v", err)
		}
	}()
}

// reapplyDBConnectionLimitsNow is the work of reapplyDBConnectionLimits.
func (p *Panel) reapplyDBConnectionLimitsNow(ctx context.Context, subscriptionIDs []int) error {
	if p.db == nil || p.secrets == nil {
		return nil // no engine credentials to reach a server with
	}
	db := p.db.GetDB()
	serverRepo := repositories.NewPostgresDatabaseServerRepository(db)
	clusters := p.pgClustersOnce(ctx)
	for _, subID := range subscriptionIDs {
		limit, err := p.dbConnectionLimit(ctx, subID)
		if err != nil {
			log.Printf("database connection limits of subscription %d: %v", subID, err)
			continue
		}
		rows, err := db.QueryContext(ctx, `SELECT server_id, username FROM database_users WHERE subscription_id = ? ORDER BY id`, subID)
		if err != nil {
			return err
		}
		type user struct {
			server   int
			username string
		}
		users := []user{}
		for rows.Next() {
			var u user
			if err := rows.Scan(&u.server, &u.username); err != nil {
				rows.Close()
				return err
			}
			users = append(users, u)
		}
		rows.Close()
		for _, u := range users {
			server, err := serverRepo.GetByID(ctx, u.server)
			if err != nil {
				log.Printf("database connection limit of %s: server %d: %v", u.username, u.server, err)
				continue
			}
			limiter, ok, err := p.connectionLimiterFor(ctx, server, nil, clusters)
			if err != nil {
				log.Printf("database connection limit of %s stays as it was until the next change: %v", u.username, err)
				continue
			}
			if !ok {
				continue
			}
			if err := limiter.SetUserConnectionLimit(u.username, limit); err != nil {
				log.Printf("database connection limit of %s stays as it was until the next change: %v", u.username, err)
			}
		}
	}
	return nil
}

// subscriptionsReachedBy lists the subscriptions whose limits follow an
// owner's ceilings: a plan's bound subscriptions, a reseller plan's or a
// reseller's tree, one Custom subscription, or every subscription (server).
func (p *Panel) subscriptionsReachedBy(ctx context.Context, ownerKind string, ownerID int) ([]int, error) {
	db := p.db.GetDB()
	switch ownerKind {
	case rightsOwnerPlan:
		return subscriptionsOnPlan(ctx, db, ownerID)
	case rightsOwnerSubscription:
		return []int{ownerID}, nil
	case rightsOwnerReseller:
		return treeSubscriptionIDs(ctx, db, ownerID)
	case rightsOwnerResellerPlan:
		rows, err := db.QueryContext(ctx, `SELECT reseller_id FROM reseller_plan_bindings WHERE reseller_plan_id = ?`, ownerID)
		if err != nil {
			return nil, err
		}
		resellers := []int{}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			resellers = append(resellers, id)
		}
		rows.Close()
		out := []int{}
		for _, id := range resellers {
			tree, err := treeSubscriptionIDs(ctx, db, id)
			if err != nil {
				return nil, err
			}
			out = append(out, tree...)
		}
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id FROM subscriptions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// dbConnectionLimitChanged: whether an edit changed the key.
func dbConnectionLimitChanged(before, after map[string]int) bool {
	b, hadB := before[settingDBConnections]
	a, hadA := after[settingDBConnections]
	return hadB != hadA || b != a
}

// reapplyDBConnectionLimitsAfter starts the rewrite for an owner whose
// ceilings changed the key.
func (p *Panel) reapplyDBConnectionLimitsAfter(ctx context.Context, ownerKind string, ownerID int, before, after map[string]int) {
	if !dbConnectionLimitChanged(before, after) {
		return
	}
	subs, err := p.subscriptionsReachedBy(ctx, ownerKind, ownerID)
	if err != nil {
		log.Printf("database connection limits after a change of %s %d: %v", ownerKind, ownerID, err)
		return
	}
	p.reapplyDBConnectionLimits(subs)
}
