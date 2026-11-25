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
		port := server.Port
		if read := clusters(); read.Supported {
			placed, err := p.pgRoleClusterPort(ctx, server, read.Clusters)
			if err != nil {
				return nil, false, err
			}
			port = placed
		}
		// alpha.149 (lab D10): this machine's PostgreSQL through the Agent,
		// as the postgres superuser, by one narrow call; never through the
		// panel's own account, which gains no right for it.
		return &pgAgentRoleLimiter{p: p, ctx: ctx, port: port}, true, nil
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

// reapplyDBConnectionLimits writes the current limits for every database
// user of the given subscriptions, in the background. A user they do not
// reach is recorded and named in Problems now (database_user_limits.go).
func (p *Panel) reapplyDBConnectionLimits(subscriptionIDs []int) {
	if len(subscriptionIDs) == 0 {
		return
	}
	ids := append([]int(nil), subscriptionIDs...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if _, err := p.reapplyDBConnectionLimitsNow(ctx, ids, nil); err != nil {
			log.Printf("database connection limits: %v", err)
		}
	}()
}

// dbLimitUser is one recorded database user.
type dbLimitUser struct {
	subscription int
	server       int
	username     string
}

// dbLimitUsersOf lists the recorded users of the subscriptions.
func (p *Panel) dbLimitUsersOf(ctx context.Context, subscriptionIDs []int) ([]dbLimitUser, error) {
	users := []dbLimitUser{}
	for _, subID := range subscriptionIDs {
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT server_id, username FROM database_users WHERE subscription_id = ? ORDER BY id`, subID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			u := dbLimitUser{subscription: subID}
			if err := rows.Scan(&u.server, &u.username); err != nil {
				rows.Close()
				return nil, err
			}
			users = append(users, u)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return users, nil
}

// reapplyDBConnectionLimitsNow is the work of reapplyDBConnectionLimits:
// every user through the one function, each outcome sent to report (may be
// nil) as it comes. It answers the outcomes.
func (p *Panel) reapplyDBConnectionLimitsNow(ctx context.Context, subscriptionIDs []int, report func(dbLimitOutcome)) ([]dbLimitOutcome, error) {
	if p.db == nil || p.secrets == nil {
		return nil, nil // no engine credentials to reach a server with
	}
	users, err := p.dbLimitUsersOf(ctx, subscriptionIDs)
	if err != nil {
		return nil, err
	}
	return p.applyDBLimitsToUsers(ctx, users, report), nil
}

// applyDBLimitsToUsers applies the limits of each user (not fresh: values
// the panel wrote may be changed or removed).
func (p *Panel) applyDBLimitsToUsers(ctx context.Context, users []dbLimitUser, report func(dbLimitOutcome)) []dbLimitOutcome {
	serverRepo := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB())
	clusters := p.pgClustersOnce(ctx)
	outcomes := make([]dbLimitOutcome, 0, len(users))
	for _, u := range users {
		var outcome dbLimitOutcome
		server, err := serverRepo.GetByID(ctx, u.server)
		if err != nil {
			outcome = dbLimitOutcome{ServerID: u.server, SubscriptionID: u.subscription, User: u.username, State: dbLimitFailed,
				Reason: boundedAgentDiagnostic("the database server record could not be read: " + err.Error())}
			log.Printf("database user limits of %s: %s", u.username, outcome.Reason)
			p.noteDBLimitOutcome(ctx, outcome)
		} else {
			outcome = p.applyDatabaseUserLimits(ctx, u.subscription, server, nil, u.username, false, clusters)
		}
		outcomes = append(outcomes, outcome)
		if report != nil {
			report(outcome)
		}
	}
	return outcomes
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

// dbConnectionLimitChanged: whether an edit changed a key the database
// users carry (the connection limit and, S4 R2a, the time and
// temporary-file limits).
func dbConnectionLimitChanged(before, after map[string]int) bool {
	for _, key := range append([]string{settingDBConnections}, dbRoleLimitKeys...) {
		b, hadB := before[key]
		a, hadA := after[key]
		if hadB != hadA || b != a {
			return true
		}
	}
	return false
}

// reapplyDBConnectionLimitsAfter starts the rewrite for an owner whose
// ceilings changed the database users' keys and (S4 R2a) the sites' slices.
func (p *Panel) reapplyDBConnectionLimitsAfter(ctx context.Context, ownerKind string, ownerID int, before, after map[string]int) {
	dbChanged, siteChanged := dbConnectionLimitChanged(before, after), siteLimitsChanged(before, after)
	if !dbChanged && !siteChanged {
		return
	}
	subs, err := p.subscriptionsReachedBy(ctx, ownerKind, ownerID)
	if err != nil {
		log.Printf("database connection limits after a change of %s %d: %v", ownerKind, ownerID, err)
		return
	}
	if dbChanged {
		p.reapplyDBConnectionLimits(subs)
	}
	if siteChanged {
		p.reapplySiteSlices(p.siteLimitChangeLabel(ctx, ownerKind, ownerID), subs)
	}
}
