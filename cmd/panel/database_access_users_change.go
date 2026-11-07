package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Adding and removing a user of one domain database (alpha.129); the routes
// and the list are in database_access_users.go.
//
// Bir alan adı veritabanına kullanıcı ekleme ve kaldırma.

// databaseAccessUserNameRe is the part of a new user's name the person
// chooses: <database>_<name>.
var databaseAccessUserNameRe = regexp.MustCompile(`^[a-z0-9_]{1,16}$`)

const (
	errCodeDatabaseUserExists    = "DATABASE_USER_EXISTS"
	errCodeDatabaseUserName      = "DATABASE_USER_NAME_INVALID"
	errCodeDatabaseUserEngine    = "DATABASE_USER_ENGINE_FAILED"
	errCodeDatabaseUserIsOwner   = "DATABASE_USER_MADE_WITH_DATABASE"
	errCodeDatabaseUserElsewhere = "DATABASE_USER_USED_ELSEWHERE"
)

// databaseAccessEngine is the Agent's engine name and cluster port of a
// tools target.
func databaseAccessEngine(target databaseToolsTarget) (string, int) {
	if target.identity.Type == "postgresql" {
		return "postgresql", target.serverPort
	}
	return "mysql", 0
}

// writeDatabaseUserEngineFailure: the engine did not do it. The Agent's
// words stay in the log; the person is told what happened and what to do.
func writeDatabaseUserEngineFailure(w http.ResponseWriter, resp transport.DatabaseAccessUserResponse, message string) {
	log.Printf("[409][database] %s: %s", errCodeDatabaseUserEngine, boundedAgentDiagnostic(resp.Error))
	if resp.CleanupIncomplete {
		message += " The user it had started to make may still be on the database server; the administrator removes it there (it is not in CelikPanel's list)."
	}
	writeCodedError(w, http.StatusConflict, errCodeDatabaseUserEngine, message, "")
}

// handleAddDatabaseAccessUser makes a read-only or read-and-write user for
// one database of the domain. Steps: check, create (the engine), records.
func (p *Panel) handleAddDatabaseAccessUser(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "create", "records")
	reportOperationStep(ctx, "check")
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Access   string `json:"access"`
	}
	if err := decodeStrictJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Access != transport.DatabaseAccessRead && req.Access != transport.DatabaseAccessWrite {
		writeClientError(w, http.StatusBadRequest, "access must be read or write")
		return
	}
	if !databaseAccessUserNameRe.MatchString(req.Name) || req.Name == "user" {
		writeCodedError(w, http.StatusBadRequest, errCodeDatabaseUserName,
			`The name must be 1 to 16 lowercase letters, digits or underscores, and not "user" (the user made with the database has that name). Nothing was changed.`, "")
		return
	}
	if refusal := checkDatabaseUserPassword(req.Password); refusal != nil {
		writePasswordRefusal(w, refusal)
		return
	}
	target, err := p.loadDatabaseToolsTarget(ctx, r, domainID)
	if err != nil {
		p.writeDatabaseToolsLoadError(w, err)
		return
	}
	// alpha.117 (D-042): the engine must be open to the subscription.
	if !p.requireSubscriptionOffering(w, r, target.domain.SubscriptionID, databaseEngineOffering(target.identity.Type), target.identity.Type, true) {
		return
	}
	username := target.identity.Name + "_" + req.Name
	if len(username) > 63 {
		writeCodedError(w, http.StatusBadRequest, errCodeDatabaseUserName,
			"The database name and this name together are longer than 63 characters, the longest user name the database server takes. Choose a shorter name; nothing was changed.", "")
		return
	}
	// The name is free on this engine as far as CelikPanel knows (every
	// subscription's record of the same engine, database_names.go); the Agent
	// checks the engine itself too.
	if what, err := p.databaseNameTakenOnEngine(ctx, target.serverID, "", username, false); err != nil {
		writeServerError(w, err)
		return
	} else if what != "" {
		writeCodedError(w, http.StatusConflict, errCodeDatabaseUserExists,
			"A database user named "+username+" already exists on this database server. Choose another name; nothing was changed.", "")
		return
	}
	if p.secrets == nil {
		writeServerError(w, errors.New("secret storage is unavailable"))
		return
	}
	sealed, err := p.secrets.Encrypt(req.Password)
	if err != nil {
		writeServerError(w, err)
		return
	}

	readers, writers, err := p.databaseAddedUsers(ctx, target.identity.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}

	reportOperationStep(ctx, "create")
	engine, port := databaseAccessEngine(target)
	agentReq := transport.DatabaseAccessUserRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Engine: engine, Port: port,
		Database: target.identity.Name, Username: username, Password: req.Password, Access: req.Access,
		Readers: readers, Writers: writers,
	}
	resp, err := p.databaseAccessNativeOps().create(ctx, agentReq)
	if err != nil {
		writeAgentError(w, err, "")
		return
	}
	switch {
	case resp.Refusal != nil:
		writeDatabaseClusterRefusal(w, r, resp.Refusal)
		return
	case resp.Exists:
		writeCodedError(w, http.StatusConflict, errCodeDatabaseUserExists,
			"A database user named "+username+" already exists on this database server (made outside CelikPanel). Choose another name; nothing was changed.", "")
		return
	case !resp.Applied:
		writeDatabaseUserEngineFailure(w, resp, "The database server did not make the user "+username+"; nothing was changed. Try again; if it fails again, the administrator finds the reason in the panel's log.")
		return
	}

	reportOperationStep(ctx, "records")
	privileges := "ALL"
	if req.Access == transport.DatabaseAccessRead {
		privileges = databaseReadOnlyPrivileges
	}
	userID, err := p.recordDatabaseAccessUser(ctx, target, username, sealed, privileges)
	if err != nil {
		// The engine has the user and the panel could not record it: it is
		// taken back so no unlisted account stays.
		undoReq := agentReq
		undoReq.Password, undoReq.Access = "", ""
		undo, undoErr := p.databaseAccessNativeOps().remove(ctx, undoReq)
		if undoErr == nil && !undo.Applied {
			undoErr = errors.New("the user could not be taken back from the database server")
		}
		writeServerError(w, errors.Join(err, undoErr))
		return
	}
	// alpha.128 (G5): the subscription's connection limit applies to it too.
	if limit, err := p.dbConnectionLimit(ctx, target.domain.SubscriptionID); err != nil {
		log.Printf("database connection limit of %s: %v", username, err)
	} else if limit > 0 {
		p.reapplyDBConnectionLimits([]int{target.domain.SubscriptionID})
	}
	p.auditDetailed(r, "database.user.add:"+username, "database_user", userID,
		map[string]any{"database": target.identity.Name, "access": req.Access})
	_ = json.NewEncoder(w).Encode(map[string]any{"id": userID, "username": username, "access": req.Access})
}

// databaseAddedUsers names the database's users added after the one made
// with it: read only (SELECT) and read and write.
func (p *Panel) databaseAddedUsers(ctx context.Context, databaseID int) (readers, writers []string, err error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT u.username, COALESCE(g.privileges, 'ALL') FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		WHERE g.database_id = ? AND g.id > (SELECT MIN(id) FROM database_user_grants WHERE database_id = ?)
		ORDER BY g.id`, databaseID, databaseID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, privileges string
		if err := rows.Scan(&name, &privileges); err != nil {
			return nil, nil, err
		}
		if databaseAccessOf(privileges) == "read" {
			readers = append(readers, name)
		} else {
			writers = append(writers, name)
		}
	}
	return readers, writers, rows.Err()
}

// recordDatabaseAccessUser writes the user and its one grant together.
func (p *Panel) recordDatabaseAccessUser(ctx context.Context, target databaseToolsTarget, username, sealed, privileges string) (int, error) {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO database_users (server_id, subscription_id, username, password) VALUES (?, ?, ?, ?)`,
		target.serverID, target.domain.SubscriptionID, username, sealed)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO database_user_grants (database_id, user_id, privileges) VALUES (?, ?, ?)`,
		target.identity.ID, id, privileges); err != nil {
		return 0, err
	}
	return int(id), tx.Commit()
}

// handleRemoveDatabaseAccessUser removes a user added for one database:
// the user and its rights go, the database stays. Steps: check, delete,
// records.
func (p *Panel) handleRemoveDatabaseAccessUser(w http.ResponseWriter, r *http.Request, domainID int) {
	ctx := r.Context()
	reportOperationPlan(ctx, "check", "delete", "records")
	reportOperationStep(ctx, "check")
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
	var firstUser, grants int
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE((SELECT user_id FROM database_user_grants WHERE database_id = ? ORDER BY id LIMIT 1), 0),
		       (SELECT COUNT(*) FROM database_user_grants WHERE user_id = ?)`, target.identity.ID, user.ID).Scan(&firstUser, &grants); err != nil {
		writeServerError(w, err)
		return
	}
	if firstUser == user.ID {
		writeCodedError(w, http.StatusConflict, errCodeDatabaseUserIsOwner,
			user.Username+" was made with the database and goes only with it: deleting the database removes it. Nothing was changed.", "")
		return
	}
	if grants > 1 {
		writeCodedError(w, http.StatusConflict, errCodeDatabaseUserElsewhere,
			user.Username+" also has access to another database, so it is not removed from here. Nothing was changed; take its access away on the Databases page first.", "")
		return
	}

	reportOperationStep(ctx, "delete")
	engine, port := databaseAccessEngine(target)
	resp, err := p.databaseAccessNativeOps().remove(ctx, transport.DatabaseAccessUserRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Engine: engine, Port: port,
		Database: target.identity.Name, Username: user.Username,
	})
	if err != nil {
		writeAgentError(w, err, "")
		return
	}
	if resp.Refusal != nil {
		writeDatabaseClusterRefusal(w, r, resp.Refusal)
		return
	}
	if !resp.Applied {
		writeDatabaseUserEngineFailure(w, resp, "The database server did not remove the user "+user.Username+"; it is still listed and the database was not changed. Try again; if it fails again, the administrator finds the reason in the panel's log.")
		return
	}

	reportOperationStep(ctx, "records")
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_user_grants WHERE user_id = ?`, user.ID); err != nil {
		writeServerError(w, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_users WHERE id = ?`, user.ID); err != nil {
		writeServerError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeServerError(w, err)
		return
	}
	// alpha.129: its remote access rules went with the user's row; the next
	// apply closes their addresses (the Agent already dropped the copies).
	kickDatabaseRemoteSync()
	p.auditDetailed(r, "database.user.remove:"+user.Username, "database_user", user.ID,
		map[string]any{"database": target.identity.Name, "was_absent": resp.Absent})
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "removed"})
}

// databaseAccessNative is the Agent's side, replaceable in tests.
type databaseAccessNative interface {
	create(context.Context, transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error)
	remove(context.Context, transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error)
}

type panelDatabaseAccessNative struct{ p *Panel }

func (n panelDatabaseAccessNative) create(ctx context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	var resp transport.DatabaseAccessUserResponse
	err := n.p.callAgentContext(ctx, "Agent.CreateDatabaseAccessUserV1", &req, &resp)
	return resp, err
}

func (n panelDatabaseAccessNative) remove(ctx context.Context, req transport.DatabaseAccessUserRequest) (transport.DatabaseAccessUserResponse, error) {
	var resp transport.DatabaseAccessUserResponse
	err := n.p.callAgentContext(ctx, "Agent.RemoveDatabaseAccessUserV1", &req, &resp)
	return resp, err
}

func (p *Panel) databaseAccessNativeOps() databaseAccessNative {
	if p.databaseAccessNative != nil {
		return p.databaseAccessNative
	}
	return panelDatabaseAccessNative{p: p}
}
