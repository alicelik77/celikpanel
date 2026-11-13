package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/transport"
)

// GET /api/v1/databases/visible (alpha.133): every database the caller may
// see, by the one visibility rule (visibleOwnerIDs): the administrator every
// database, a reseller its own and its customers', a customer its own. A team
// member sees what it saw on this page before, which is nothing: team-member
// grants are not wired to the account-wide list (fail closed, as
// visibleOwnerIDs does).
//
// The owner of a database is the owner of the database record's own
// subscription. The answer carries names only: no password, no connection
// secret, no content. Two reads in all, whatever the number of
// subscriptions: the databases with their owners and domains, then the users
// of those databases.
//
// size_bytes is null in this release: the Agent measures one database at a
// time (DatabaseToolsInfoV1) and the list does not ask it once per row.
//
// engines is what the server has installed and running, read from the Agent
// once per request; it is null when the Agent could not be asked (not known
// right now), and empty for a team member (server inventory is not theirs).
//
// GET /api/v1/databases/visible: çağıranın görebildiği her veritabanı, tek
// görünürlük kuralıyla. Parola, bağlantı sırrı ya da içerik yoktur.
const databasesVisiblePath = "/api/v1/databases/visible"

type visibleDatabaseOwner struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	// Parent is the reseller's username when the owner is a reseller's
	// customer; null otherwise.
	Parent *string `json:"parent"`
	Self   bool    `json:"self"`
}

type visibleDatabaseDomain struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type visibleDatabaseUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	// Stays (alpha.133 round 5): what deleting the database leaves of the
	// user: "" (removed with it), "shared" (another database uses the same
	// account) or "account" (an engine's or CelikPanel's own account).
	Stays string `json:"stays,omitempty"`
}

type visibleDatabase struct {
	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	Engine    string                 `json:"engine"`
	Owner     visibleDatabaseOwner   `json:"owner"`
	Domain    *visibleDatabaseDomain `json:"domain"`
	SizeBytes *int64                 `json:"size_bytes"`
	CreatedAt string                 `json:"created_at"`
	Users     []visibleDatabaseUser  `json:"users"`
}

type visibleDatabaseEngine struct {
	Engine    string `json:"engine"`
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
}

type visibleDatabasesAnswer struct {
	Databases []visibleDatabase       `json:"databases"`
	Engines   []visibleDatabaseEngine `json:"engines"`
	MayCreate bool                    `json:"may_create"`
}

// readDatabaseEngineServices reads the server's services from the Agent. A
// test replaces it.
var readDatabaseEngineServices = func(p *Panel, ctx context.Context) ([]core.Service, error) {
	var services []core.Service
	if err := p.callAgentContext(ctx, "Agent.GetServices", &transport.Empty{}, &services); err != nil {
		return nil, err
	}
	return services, nil
}

// databaseEngineOfUnit names the database engine a service unit belongs to
// ("mariadb" for MariaDB and MySQL, "postgresql"), or "" for any other unit.
// Autodiscovery registers engines by the same rule.
func databaseEngineOfUnit(unit string) string {
	name := strings.ToLower(strings.TrimSpace(unit))
	switch {
	case strings.HasPrefix(name, "postgresql"):
		return "postgresql"
	case strings.HasPrefix(name, "mariadb"), strings.HasPrefix(name, "mysql"):
		return "mariadb"
	}
	return ""
}

// databaseEnginesFromServices answers both engines, in a fixed order: an
// engine is installed when one of its units is present and running when one
// of its units runs (a PostgreSQL cluster unit, not only the wrapper unit
// that stays "active (exited)").
func databaseEnginesFromServices(services []core.Service) []visibleDatabaseEngine {
	engines := []visibleDatabaseEngine{{Engine: "mariadb"}, {Engine: "postgresql"}}
	for _, service := range services {
		engine := databaseEngineOfUnit(service.Name)
		if engine == "" {
			continue
		}
		for i := range engines {
			if engines[i].Engine != engine {
				continue
			}
			engines[i].Installed = true
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(service.Status)), "active (running)") {
				engines[i].Running = true
			}
		}
	}
	return engines
}

func (p *Panel) handleVisibleDatabases(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	ctx := r.Context()
	caller := currentCaller(r)
	answer := visibleDatabasesAnswer{Databases: []visibleDatabase{}, Engines: []visibleDatabaseEngine{}}
	if caller == nil || !caller.validAuthorizationIdentity() || caller.isAdditionalUser() {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	ids, all, err := p.visibleOwnerIDs(ctx, caller)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if answer.Databases, err = p.visibleDatabaseRows(ctx, caller, ids, all); err != nil {
		writeServerError(w, err)
		return
	}
	if answer.MayCreate, err = p.callerMayCreateDatabase(ctx, r, ids, all); err != nil {
		writeServerError(w, err)
		return
	}
	if services, err := readDatabaseEngineServices(p, ctx); err != nil {
		log.Printf("databases list: the Agent could not be asked for the installed engines: %v", err)
		answer.Engines = nil
	} else {
		answer.Engines = databaseEnginesFromServices(services)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(answer)
}

// visibleOwnerFilter is the WHERE part limiting subscriptions s to the
// visible owners, with its arguments; ok is false when nobody is visible.
func visibleOwnerFilter(ids map[int]bool, all bool) (string, []any, bool) {
	if all {
		return "", nil, true
	}
	if len(ids) == 0 {
		return "", nil, false
	}
	marks := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for id := range ids {
		marks = append(marks, "?")
		args = append(args, id)
	}
	return " WHERE s.owner_id IN (" + strings.Join(marks, ",") + ")", args, true
}

// visibleDatabaseRows reads the visible databases and their users: two
// queries, each read to the end before the next starts.
func (p *Panel) visibleDatabaseRows(ctx context.Context, caller *Caller, ids map[int]bool, all bool) ([]visibleDatabase, error) {
	out := []visibleDatabase{}
	filter, args, ok := visibleOwnerFilter(ids, all)
	if !ok {
		return out, nil
	}
	pool := p.db.GetDB()
	rows, err := pool.QueryContext(ctx, `
		SELECT d.id, d.name, dst.name, COALESCE(d.created_at, ''),
		       dom.id, COALESCE(dom.name, ''),
		       u.id, u.username, u.role,
		       CASE WHEN pu.role = 'reseller' THEN pu.username END
		FROM databases_v2 d
		JOIN database_servers ds ON ds.id = d.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		JOIN subscriptions s ON s.id = d.subscription_id
		JOIN users u ON u.id = s.owner_id
		LEFT JOIN users pu ON pu.id = u.parent_id
		LEFT JOIN domains dom ON dom.id = d.domain_id AND dom.subscription_id = d.subscription_id`+filter+`
		ORDER BY u.username, d.name, d.id`, args...)
	if err != nil {
		return nil, err
	}
	index := map[int]int{}
	for rows.Next() {
		var db visibleDatabase
		var serverType, createdAt, domainName string
		var domainID sql.NullInt64
		var parent sql.NullString
		if err := rows.Scan(&db.ID, &db.Name, &serverType, &createdAt, &domainID, &domainName,
			&db.Owner.ID, &db.Owner.Username, &db.Owner.Role, &parent); err != nil {
			rows.Close()
			return nil, err
		}
		db.Engine = "mariadb"
		if serverType == "postgresql" {
			db.Engine = "postgresql"
		}
		if domainID.Valid {
			db.Domain = &visibleDatabaseDomain{ID: int(domainID.Int64), Name: domainName}
		}
		if parent.Valid && parent.String != "" {
			name := parent.String
			db.Owner.Parent = &name
		}
		db.Owner.Self = db.Owner.ID == caller.ID
		if at := parseSQLiteTime(createdAt); !at.IsZero() {
			db.CreatedAt = at.UTC().Format(time.RFC3339)
		}
		db.Users = []visibleDatabaseUser{}
		index[db.ID] = len(out)
		out = append(out, db)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) == 0 {
		return out, nil
	}

	// A user is named only when it belongs to the database's own
	// subscription, so a stray grant never names another account's user.
	users, err := pool.QueryContext(ctx, `
		SELECT g.database_id, du.id, du.username
		FROM database_user_grants g
		JOIN databases_v2 d ON d.id = g.database_id
		JOIN database_users du ON du.id = g.user_id AND du.subscription_id = d.subscription_id
		JOIN subscriptions s ON s.id = d.subscription_id`+filter+`
		ORDER BY g.database_id, g.id`, args...)
	if err != nil {
		return nil, err
	}
	defer users.Close()
	for users.Next() {
		var databaseID int
		var user visibleDatabaseUser
		if err := users.Scan(&databaseID, &user.ID, &user.Username); err != nil {
			return nil, err
		}
		if i, ok := index[databaseID]; ok {
			out[i].Users = append(out[i].Users, user)
		}
	}
	if err := users.Err(); err != nil {
		return nil, err
	}
	// The delete question says which users go and which stay, decided as the
	// deletion decides it (database_delete.go).
	listed := make([]int, 0, len(out))
	for _, db := range out {
		listed = append(listed, db.ID)
	}
	// A failure to tell is "unknown" per user; the list still loads.
	fates := p.databaseUserFates(ctx, listed)
	for i := range out {
		for j := range out[i].Users {
			out[i].Users[j].Stays = databaseUserStays(fates, out[i].ID, out[i].Users[j].ID)
		}
	}
	return out, nil
}

// callerMayCreateDatabase: true when one of the caller's visible domains
// belongs to a subscription whose plan takes another database: the databases
// permission and at least one database engine open to it (the administrator
// acting as itself is never refused these, as on the create paths), and the
// database limit, the disk, the reseller's pool and suspension allow one more
// (checkSubscriptionQuota).
func (p *Panel) callerMayCreateDatabase(ctx context.Context, r *http.Request, ids map[int]bool, all bool) (bool, error) {
	filter, args, ok := visibleOwnerFilter(ids, all)
	if !ok {
		return false, nil
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT DISTINCT d.subscription_id
		FROM domains d
		JOIN subscriptions s ON s.id = d.subscription_id`+filter+`
		ORDER BY d.subscription_id`, args...)
	if err != nil {
		return false, err
	}
	var subscriptions []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		subscriptions = append(subscriptions, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()

	administrator := callerIsAdministrator(r)
	for _, subscriptionID := range subscriptions {
		if !administrator {
			rights, err := p.effectiveSubscriptionRights(ctx, subscriptionID)
			if err != nil {
				return false, err
			}
			if !rights.hasPermission(rightDatabases) {
				continue
			}
			if rights.anyVersionRefusal(databaseEngineOffering("mariadb")) != "" &&
				rights.anyVersionRefusal(databaseEngineOffering("postgresql")) != "" {
				continue
			}
		}
		err := p.checkSubscriptionQuota(ctx, subscriptionID, quotaDBs)
		if err == nil {
			return true, nil
		}
		var exceeded *quotaExceededError
		var pool *resellerPoolExceededError
		if errors.As(err, &exceeded) || errors.As(err, &pool) || errors.Is(err, errSubscriptionSuspendedCreate) ||
			errors.Is(err, errQuotaSubscriptionMissing) {
			continue
		}
		return false, err
	}
	return false, nil
}
