package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Deleting a database (alpha.133 round 5). One function does it for both
// places a database is deleted from: the left-menu Databases page
// (DELETE /api/v1/databases/{id}) and the domain's Databases section
// (DELETE /api/v1/domains/{d}/databases/{id}).
//
// What goes: the database, and every user CelikPanel recorded for it that no
// other database uses (the owner's rule: no leftovers). What stays: a user
// that also has access to another database, and the engine's and CelikPanel's
// own accounts, whatever the records say.
//
// How the engine is reached: an engine on this machine through the Agent's
// own door (mysql as root over the socket, psql as postgres on the database's
// cluster), as creating a database and giving a user a new password already
// do; never through the credential stored on a subscription's server record.
// That record is per subscription while CelikPanel's account on the engine is
// one, so a record's copy can be older than the account's password (the lab
// refusal DATABASE_ENGINE_CREDENTIAL_REFUSED of 13 Nov 2026). An engine on
// another machine has no such door and is reached with its record's
// credential, as before.
//
// Order, so that an interruption leaves a state the same button finishes:
// the database is dropped first (IF EXISTS), then each of its own users
// (absent counts as removed), and only then are the records removed, in one
// transaction. Until the records go the database stays listed, and pressing
// Delete again runs the same steps over what is left.
//
// Veritabanını silmek: veritabanı ve başka hiçbir veritabanının kullanmadığı
// kullanıcıları gider. Bu makinedeki motora Agent'ın kendi kapısından
// ulaşılır; kayıtlar en son, tek işlemde silinir. Yarıda kalan silme aynı
// düğmeyle tamamlanır.

const (
	// errCodeDatabaseDeleteFailed: the engine did not drop the database;
	// nothing was changed.
	errCodeDatabaseDeleteFailed = "DATABASE_DELETE_FAILED"
	// errCodeDatabaseDeleteUnconfirmed: no answer came back; whether the
	// database was dropped is not known. The list is read again.
	errCodeDatabaseDeleteUnconfirmed = "DATABASE_DELETE_UNCONFIRMED"
	// errCodeDatabaseDeleteUsersLeft: the database is gone from the engine,
	// one or more of its own users is not yet; it stays listed until they are.
	errCodeDatabaseDeleteUsersLeft = "DATABASE_DELETE_USERS_LEFT"
)

// databaseDeleteAgent is the Agent call for dropping a database; tests
// replace it.
var databaseDeleteAgent = func(p *Panel, ctx context.Context, req transport.DeleteDatabaseRequest, resp *transport.DeleteDatabaseResponse) error {
	return p.callAgentContext(ctx, "Agent.DeleteDatabase", &req, resp)
}

// databaseDeleteUser is one user recorded for the database being deleted.
type databaseDeleteUser struct {
	ID       int
	Username string
	// Kept: another database uses it too, or it is an account that is never
	// removed by deleting a database.
	Kept bool
}

// databaseDeletion is the outcome a handler answers with.
type databaseDeletion struct {
	Name         string
	UsersRemoved []string
	UsersKept    []string
}

// databaseDeleteError says how far a deletion got (D-024: a verified failure,
// a verified prerequisite, or an unknown result).
type databaseDeleteError struct {
	// unknown: no answer came back; whether the step ran is not known.
	unknown bool
	// databaseGone: the engine confirmed the database is dropped.
	databaseGone bool
	// usersLeft: the database's own users the engine has not removed.
	usersLeft []string
	refusal   *transport.DatabaseClusterRefusal
	// engine: an error from the record's credential path (another machine).
	engine error
	server *core.DatabaseServer
	cause  error
}

func (e *databaseDeleteError) Error() string { return e.cause.Error() }
func (e *databaseDeleteError) Unwrap() error { return e.cause }

// databaseNeverRemovedUser: accounts a database deletion never removes, even
// when a record names them: the engines' own and CelikPanel's.
func databaseNeverRemovedUser(name string, server *core.DatabaseServer) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "root", "mysql", "mariadb.sys", "mysql.sys", "mysql.session", "mysql.infoschema", "postgres",
		strings.ToLower(transport.DatabaseAdminAccountName):
		return true
	}
	return server != nil && strings.EqualFold(strings.TrimSpace(server.AdminUsername), strings.TrimSpace(name))
}

// databaseDeleteUsers reads the users recorded for the database and whether
// each is used by another database too.
func (p *Panel) databaseDeleteUsers(ctx context.Context, databaseID int, server *core.DatabaseServer) ([]databaseDeleteUser, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT u.id, u.username,
		       EXISTS(SELECT 1 FROM database_user_grants o WHERE o.user_id = u.id AND o.database_id <> ?1)
		FROM database_user_grants g
		JOIN database_users u ON u.id = g.user_id
		WHERE g.database_id = ?1
		ORDER BY g.id`, databaseID)
	if err != nil {
		return nil, fmt.Errorf("read the users of database %d: %w", databaseID, err)
	}
	defer rows.Close()
	var users []databaseDeleteUser
	seen := map[int]bool{}
	for rows.Next() {
		var user databaseDeleteUser
		var shared bool
		if err := rows.Scan(&user.ID, &user.Username, &shared); err != nil {
			return nil, fmt.Errorf("read a user of database %d: %w", databaseID, err)
		}
		if seen[user.ID] {
			continue
		}
		seen[user.ID] = true
		user.Kept = shared || databaseNeverRemovedUser(user.Username, server)
		users = append(users, user)
	}
	return users, rows.Err()
}

// deleteDatabaseRecord deletes the database with this id: on the engine, its
// own users, then its records. The caller has authorized it.
func (p *Panel) deleteDatabaseRecord(ctx context.Context, databaseID int) (*databaseDeletion, error) {
	var name string
	var serverID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT name, server_id FROM databases_v2 WHERE id = ?`, databaseID).Scan(&name, &serverID); err != nil {
		return nil, fmt.Errorf("read database %d: %w", databaseID, err)
	}
	server, err := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB()).GetByID(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("read the server of database %d: %w", databaseID, err)
	}
	users, err := p.databaseDeleteUsers(ctx, databaseID, server)
	if err != nil {
		return nil, err
	}
	result := &databaseDeletion{Name: name}
	var remove []databaseDeleteUser
	for _, user := range users {
		if user.Kept {
			result.UsersKept = append(result.UsersKept, user.Username)
		} else {
			remove = append(remove, user)
			result.UsersRemoved = append(result.UsersRemoved, user.Username)
		}
	}

	reportOperationStep(ctx, "delete")
	if localDatabaseHost(server.Host) {
		err = p.deleteDatabaseOnThisMachine(ctx, server, name, remove)
	} else {
		err = p.deleteDatabaseElsewhere(server, name, remove)
	}
	if err != nil {
		return nil, err
	}

	reportOperationStep(ctx, "records")
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, &databaseDeleteError{databaseGone: true, cause: fmt.Errorf("begin the records of the deleted database %s: %w", name, err)}
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_user_grants WHERE database_id = ?`, databaseID); err != nil {
		return nil, &databaseDeleteError{databaseGone: true, cause: fmt.Errorf("remove the grants of database %s: %w", name, err)}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM databases_v2 WHERE id = ?`, databaseID); err != nil {
		return nil, &databaseDeleteError{databaseGone: true, cause: fmt.Errorf("remove the record of database %s: %w", name, err)}
	}
	for _, user := range remove {
		// Only a user no grant names any more: the engine removed it, and a
		// grant made meanwhile keeps the record for the next deletion to see.
		if _, err := tx.ExecContext(ctx, `DELETE FROM database_users WHERE id = ? AND NOT EXISTS (SELECT 1 FROM database_user_grants WHERE user_id = ?)`, user.ID, user.ID); err != nil {
			return nil, &databaseDeleteError{databaseGone: true, cause: fmt.Errorf("remove the record of user %s: %w", user.Username, err)}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, &databaseDeleteError{databaseGone: true, cause: fmt.Errorf("commit the removal of database %s: %w", name, err)}
	}
	kickDatabaseRemoteSync() // alpha.129: after the committed change only (remote database access follows)
	return result, nil
}

// deleteDatabaseOnThisMachine drops the database and then its own users
// through the Agent.
func (p *Panel) deleteDatabaseOnThisMachine(ctx context.Context, server *core.DatabaseServer, name string, remove []databaseDeleteUser) error {
	engine, port := "mysql", 0
	if dbDriverTypeFor(server) == "postgresql" {
		engine, port = "postgresql", server.Port
	}
	var resp transport.DeleteDatabaseResponse
	// No user here: the database's own users go below, each only when no
	// other database uses it (the Agent's old user field dropped one blindly).
	req := transport.DeleteDatabaseRequest{Type: engine, Name: name, Port: port}
	if err := databaseDeleteAgent(p, ctx, req, &resp); err != nil {
		return &databaseDeleteError{unknown: true, cause: fmt.Errorf("drop database %s: %w", name, err)}
	}
	if resp.Refusal != nil {
		return &databaseDeleteError{refusal: resp.Refusal, cause: fmt.Errorf("drop database %s: its PostgreSQL cluster cannot take it", name)}
	}
	if !resp.Success {
		log.Printf("[409][database] %s %s: %s", errCodeDatabaseDeleteFailed, name, boundedAgentDiagnostic(resp.Error))
		return &databaseDeleteError{cause: fmt.Errorf("drop database %s: the engine did not drop it", name)}
	}
	var left []string
	unknown := false
	for _, user := range remove {
		answer, err := p.databaseAccessNativeOps().remove(ctx, transport.DatabaseAccessUserRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), Engine: engine, Port: port,
			Database: name, Username: user.Username,
		})
		switch {
		case err != nil:
			unknown = true
			left = append(left, user.Username)
			log.Printf("database deletion %s: remove user %s: %v", name, user.Username, err)
		case answer.Refusal != nil || !answer.Applied:
			left = append(left, user.Username)
			log.Printf("[409][database] %s %s: user %s: %s", errCodeDatabaseDeleteUsersLeft, name, user.Username, boundedAgentDiagnostic(answer.Error))
		}
	}
	if len(left) > 0 {
		return &databaseDeleteError{unknown: unknown, databaseGone: true, usersLeft: left,
			cause: fmt.Errorf("database %s was dropped; its users %s were not all removed", name, strings.Join(left, ", "))}
	}
	return nil
}

// deleteDatabaseElsewhere drops the database and its own users on an engine
// on another machine, with the record's credential.
func (p *Panel) deleteDatabaseElsewhere(server *core.DatabaseServer, name string, remove []databaseDeleteUser) error {
	driver, err := p.dbDriverFor(server)
	if err != nil {
		return fmt.Errorf("open the credential of database server %d: %w", server.ID, err)
	}
	if err := driver.DeleteDatabase(name); err != nil {
		return &databaseDeleteError{engine: err, server: server, cause: fmt.Errorf("drop database %s: %w", name, err)}
	}
	var left []string
	var first error
	for _, user := range remove {
		if err := driver.DeleteUser(user.Username); err != nil {
			left = append(left, user.Username)
			if first == nil {
				first = err
			}
			log.Printf("database deletion %s: remove user %s: %v", name, user.Username, err)
		}
	}
	if len(left) > 0 {
		return &databaseDeleteError{databaseGone: true, usersLeft: left, engine: first, server: server,
			cause: fmt.Errorf("database %s was dropped; its users %s were not all removed: %w", name, strings.Join(left, ", "), first)}
	}
	return nil
}

// writeDatabaseDeleteError answers a deletion that did not finish, in the
// product's error shape: a code the screen has words for, the facts as vars,
// and the in-panel fix only for an administrator.
func writeDatabaseDeleteError(w http.ResponseWriter, r *http.Request, name string, err error) {
	var failed *databaseDeleteError
	if !errors.As(err, &failed) {
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "invalid request")
			return
		}
		writeServerErrorFor(w, r, err)
		return
	}
	switch {
	case failed.refusal != nil:
		writeDatabaseClusterRefusal(w, r, failed.refusal)
	case failed.databaseGone && len(failed.usersLeft) > 0:
		log.Printf("[409][database] %s: %v", errCodeDatabaseDeleteUsersLeft, failed.cause)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		body := apiErrorBody{
			Error: "The database " + name + " was deleted, but its users " + strings.Join(failed.usersLeft, ", ") +
				" were not removed yet. It stays in the list until they are; press Delete again to finish.",
			Code: errCodeDatabaseDeleteUsersLeft, PartialSuccess: true, MutationApplied: true,
			Vars: map[string]string{"name": name, "users": strings.Join(failed.usersLeft, ", ")},
		}
		if failed.unknown {
			body.Reason = "unknown"
		}
		_ = json.NewEncoder(w).Encode(body)
	case failed.databaseGone:
		// The engine is done; the records could not be removed.
		writeServerErrorFor(w, r, failed.cause)
	case failed.unknown && agentUnreachableAlone(errors.Unwrap(failed.cause)):
		// The Agent could not be reached: nothing ran.
		writeAgentError(w, errors.Unwrap(failed.cause), "")
	case failed.unknown:
		// No answer: whether it ran is not known; the list is read again.
		log.Printf("[502][database] %s: %v", errCodeDatabaseDeleteUnconfirmed, failed.cause)
		writeCodedErrorVars(w, http.StatusBadGateway, errCodeDatabaseDeleteUnconfirmed,
			"No answer came back, so it is not known whether "+name+" was deleted. The list is read again: if "+name+" is still there, press Delete again; it continues from where it stopped.",
			"", nil, map[string]string{"name": name})
	case failed.engine != nil:
		writeDatabaseEngineErrorFor(w, r, failed.server, failed.engine)
	default:
		writeCodedErrorVars(w, http.StatusConflict, errCodeDatabaseDeleteFailed,
			"The database server did not delete "+name+"; nothing was changed. Try again; if it fails again, the administrator finds the reason in the panel's log.",
			"", nil, map[string]string{"name": name})
	}
}

// writeDatabaseEngineErrorFor is writeDatabaseEngineError for a request: an
// administrator is the one who acts, so their answer carries the reason
// "administrator" (their sentence names the page) and the page itself;
// anybody else is told to ask the server administrator. The engine's own
// words stay in the log.
// writeDatabaseEngineErrorFor: yönetici kendisi yapar (sayfa ve yolu);
// başkasına sunucu yöneticisine başvurması söylenir.
func writeDatabaseEngineErrorFor(w http.ResponseWriter, r *http.Request, server *core.DatabaseServer, err error) {
	refusal := services.ClassifyDatabaseEngineRefusal(err)
	driverType := ""
	if server != nil {
		driverType = dbDriverTypeFor(server)
	}
	message := services.DatabaseEngineRefusalMessage(driverType, refusal)
	if message == "" {
		writeServerErrorFor(w, r, err)
		return
	}
	code := errCodeDatabaseEngineUnreachable
	if refusal == services.DatabaseEngineRefusalCredential {
		code = errCodeDatabaseEngineCredentialRefused
	}
	log.Printf("[409][database] %s: %v", code, err)
	body := apiErrorBody{Error: message, Code: code, Vars: map[string]string{"engine": databaseEngineDisplayName(driverType)}}
	if caller := currentCaller(r); caller != nil && caller.hasAccountRole(roleAdmin) {
		body.Reason = "administrator"
		if driverType == "mariadb" || driverType == "postgresql" {
			body.Action = "/components/" + driverType
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(body)
}

// databaseEngineDisplayName is the engine's product name for a sentence.
func databaseEngineDisplayName(driverType string) string {
	switch driverType {
	case "mariadb":
		return "MariaDB"
	case "postgresql":
		return "PostgreSQL"
	}
	return "the database server"
}
