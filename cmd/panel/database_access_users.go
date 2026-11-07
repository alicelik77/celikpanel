package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
)

// The users of one domain database (alpha.129), under the domain like the
// database tools, so ownership, team capability, suspension and the plan's
// databases permission are decided by the domain dispatcher first:
//
//	POST   /api/v1/domains/{id}/databases/{db}/users                 {name, password, access}
//	DELETE /api/v1/domains/{id}/databases/{db}/users/{user}
//	POST   /api/v1/domains/{id}/databases/{db}/users/{user}/password {password}
//
// A new user reads the database ("read") or reads and writes it ("write");
// it is named <database>_<name>, recorded as a database user with one grant
// (privileges SELECT or ALL, the model the account page uses), and made on
// the engine by the Agent (cmd/agent/database_access_users.go). Removing it
// removes the user and its rights, never the database. The user made with
// the database (the first grant) goes only with the database. There is no
// database-user quota in the plans (docs/PLANS-AND-RIGHTS.md), so none is
// counted. A password change needs a grant on that database of that domain;
// the change itself is the account page's (applyDatabaseUserPassword).
//
// Bir alan adı veritabanının kullanıcıları; yetki alan adı yönlendiricisinde.

// matchDatabaseUsersRoute matches databases/{db}/users/... under a domain.
func matchDatabaseUsersRoute(tail []string) (string, []string, bool) {
	if len(tail) < 3 || tail[0] != "databases" || tail[2] != "users" {
		return "", nil, false
	}
	if _, ok := strictPositiveID(tail[1]); !ok {
		return "", nil, false
	}
	switch {
	case len(tail) == 3:
		return "database-users", []string{http.MethodPost}, true
	case len(tail) == 4:
		if _, ok := strictPositiveID(tail[3]); !ok {
			return "", nil, false
		}
		return "database-user", []string{http.MethodDelete}, true
	case len(tail) == 5 && tail[4] == "password":
		if _, ok := strictPositiveID(tail[3]); !ok {
			return "", nil, false
		}
		return "database-user-password", []string{http.MethodPost}, true
	}
	return "", nil, false
}

func init() {
	// D-042: changing a user is use of databases; team members manage.
	// A new user is new use; removing one stays open (D-042: removal is
	// never gated).
	domainSubrouteRights["database-user-password"] = domainSubrouteRight{right: rightDatabases}
	domainSubrouteRights["database-users"] = domainSubrouteRight{right: rightDatabases}
	domainSubrouteRights["database-user"] = domainSubrouteRight{right: rightDatabases}
	manage := teamMemberDomainRequirement{capability: core.TeamCapabilityDatabases, mode: core.TeamPermissionManage}
	teamMemberDomainRequirements[teamMemberRouteKey{kind: "database-user-password", method: http.MethodPost}] = manage
	teamMemberDomainRequirements[teamMemberRouteKey{kind: "database-users", method: http.MethodPost}] = manage
	teamMemberDomainRequirements[teamMemberRouteKey{kind: "database-user", method: http.MethodDelete}] = manage
}

// The password route's answer may carry a password CelikPanel made up, so
// only its status is kept, and the stored digest leaves the password out.
var requestIdentityRouteDomainDBUserPassword = requestIdentityRoute{
	pattern: "/api/v1/domains/{id}/databases/{id}/users/{id}/password", timeout: agentRPCMutationTimeout + time.Minute,
	optional: true, secret: true, digest: requestIdentityDatabaseUserPasswordDigest,
}

// A new user: the stored digest leaves the password out. Removal follows
// the steps too (operationStepRouteFor).
var (
	requestIdentityRouteDomainDBUserAdd = requestIdentityRoute{
		pattern: "/api/v1/domains/{id}/databases/{id}/users", timeout: agentRPCMutationTimeout + time.Minute,
		optional: true, digest: requestIdentityDatabaseAccessUserDigest,
	}
	requestIdentityRouteDomainDBUserRemove = operationStepRoute("/api/v1/domains/{id}/databases/{id}/users/{id}")
)

func requestIdentityDatabaseAccessUserDigest(body []byte) []byte {
	var req struct {
		Name   string `json:"name"`
		Access string `json:"access"`
	}
	_ = json.Unmarshal(body, &req)
	return []byte(req.Name + ":" + req.Access)
}

func databaseUsersIdentityRoute(kind string) (requestIdentityRoute, bool) {
	switch kind {
	case "database-user-password":
		return requestIdentityRouteDomainDBUserPassword, true
	case "database-users":
		return requestIdentityRouteDomainDBUserAdd, true
	}
	return requestIdentityRoute{}, false
}

func (p *Panel) handleDatabaseUsersRoute(w http.ResponseWriter, r *http.Request, domainID int, kind string) {
	w.Header().Set("Content-Type", "application/json")
	switch kind {
	case "database-user-password":
		p.handleDomainDatabaseUserPassword(w, r, domainID)
	case "database-users":
		p.handleAddDatabaseAccessUser(w, r, domainID)
	case "database-user":
		p.handleRemoveDatabaseAccessUser(w, r, domainID)
	default:
		http.NotFound(w, r)
	}
}

// databaseRouteUserID is the {user} segment of a users route.
func databaseRouteUserID(r *http.Request) (int, bool) {
	segments, ok := strictRouteSegments(r, "/api/v1/domains/")
	if !ok || len(segments) < 5 {
		return 0, false
	}
	return strictPositiveID(segments[4])
}

// loadDatabaseGrantUser is a user that holds a grant on the database, on the
// database's own server record.
func (p *Panel) loadDatabaseGrantUser(ctx context.Context, databaseID, serverID, userID int) (*core.DatabaseUser, error) {
	var id int
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT u.id FROM database_users u JOIN database_user_grants g ON g.user_id = u.id
		WHERE g.database_id = ? AND u.id = ? AND u.server_id = ?`, databaseID, userID, serverID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return repositories.NewPostgresDatabaseUserRepository(p.db.GetDB()).GetByID(ctx, id)
}

func (p *Panel) handleDomainDatabaseUserPassword(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "password", "records")
	reportOperationStep(ctx, "check")
	password, ok := readDatabaseUserPasswordRequest(w, r)
	if !ok {
		return
	}
	target, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	userID, ok := databaseRouteUserID(r)
	if !ok {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	user, err := p.loadDatabaseGrantUser(ctx, target.identity.ID, target.serverID, userID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && user.SubscriptionID != target.domain.SubscriptionID) {
		writeClientError(w, http.StatusNotFound, "database user not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	p.applyDatabaseUserPassword(w, r, user, password)
}

// domainDatabaseUser is one user of a domain database in the list: the
// first grant is the user made with the database ("owner"); a later one is
// read only ("read", SELECT) or read and write ("write").
type domainDatabaseUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Access   string `json:"access"`
}

// domainDatabaseUsers lists the users of the domain's databases, by
// database id, in grant order.
func (p *Panel) domainDatabaseUsers(ctx context.Context, domainID int) (map[int][]domainDatabaseUser, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT g.database_id, u.id, u.username, COALESCE(g.privileges, 'ALL')
		FROM database_user_grants g
		JOIN database_users u ON u.id = g.user_id
		JOIN databases_v2 d ON d.id = g.database_id
		WHERE d.domain_id = ? AND u.server_id = d.server_id
		ORDER BY g.database_id, g.id`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]domainDatabaseUser{}
	for rows.Next() {
		var databaseID int
		var user domainDatabaseUser
		var privileges string
		if err := rows.Scan(&databaseID, &user.ID, &user.Username, &privileges); err != nil {
			return nil, err
		}
		user.Access = databaseAccessOf(privileges)
		if len(out[databaseID]) == 0 {
			user.Access = "owner"
		}
		out[databaseID] = append(out[databaseID], user)
	}
	return out, rows.Err()
}

// databaseAccessOf names a grant's privileges: SELECT alone is read only.
func databaseAccessOf(privileges string) string {
	if privileges == databaseReadOnlyPrivileges {
		return "read"
	}
	return "write"
}

// databaseReadOnlyPrivileges is the grant row of a read-only user.
const databaseReadOnlyPrivileges = "SELECT"
