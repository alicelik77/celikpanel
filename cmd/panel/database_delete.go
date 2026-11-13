package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"strconv"
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
// What goes: the database, and every user CelikPanel recorded for it that
// nothing else uses (the owner's rule: no leftovers). What stays: a user that
// another database uses too - decided by the engine account (the engine and
// the name), across every record of that engine, not by one record - a user
// recorded for another server record or subscription, the engines' own and
// CelikPanel's accounts, and, on this machine, an account the owner gave
// rights of their own (the Agent keeps it and takes only CelikPanel's grant
// away; it is then reported as kept on the engine).
//
// How the engine is reached: CelikPanel's own record of an engine on this
// machine through the Agent's own door (databaseRecordIsOurLocalEngine), as
// creating a database and giving a user a new password do; never through the
// credential stored on a subscription's record, which is a per-subscription
// copy of one account's password (the lab refusal
// DATABASE_ENGINE_CREDENTIAL_REFUSED of 13 Nov 2026). Any other record - on
// another machine, or an administrator's own record on this one - with its
// own credential.
//
// Order, so that an interruption leaves a state the same button finishes:
// the database is dropped first (IF EXISTS), then each of its own users
// (absent counts as removed), and only then are the records removed, in one
// transaction. Until the records go the database stays listed, and pressing
// Delete again runs the same steps over what is left. Grants to an engine
// account wait while a deletion of the same engine runs
// (lockDatabaseEngineUsers), so a user is never given to another database
// between being classified and being dropped.
//
// Veritabanını silmek: veritabanı ve onu kullanan başka hiçbir şey olmayan
// kullanıcıları gider. Kayıtlar en son, tek işlemde silinir; yarıda kalan
// silme aynı düğmeyle tamamlanır.

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
	// errCodeDatabaseDeleteRecordsLeft: the engine is done, CelikPanel's
	// records of the database could not be removed; Delete again finishes.
	errCodeDatabaseDeleteRecordsLeft = "DATABASE_DELETE_RECORDS_LEFT"
)

// databaseDeleteAgent is the Agent call for dropping a database; tests
// replace it.
var databaseDeleteAgent = func(p *Panel, ctx context.Context, req transport.DeleteDatabaseRequest, resp *transport.DeleteDatabaseResponse) error {
	return p.callAgentContext(ctx, "Agent.DeleteDatabase", &req, resp)
}

// databaseDeleteAfterClassify is a test seam: it runs between classifying
// the users and the first engine statement.
var databaseDeleteAfterClassify func()

// What becomes of a database's user when the database is deleted.
const (
	databaseUserRemoved = ""
	// databaseUserShared: another database uses the same engine account, or
	// the user is recorded for another server record or subscription.
	databaseUserShared = "shared"
	// databaseUserAccount: an engine's or CelikPanel's own account, or a
	// name CelikPanel does not manage.
	databaseUserAccount = "account"
)

// databaseDeleteUser is one user recorded for the database being deleted.
type databaseDeleteUser struct {
	ID       int
	Username string
	Stays    string // databaseUserRemoved, databaseUserShared or databaseUserAccount
}

// databaseDeletion is what a deletion did, for the answer and the audit.
type databaseDeletion struct {
	ID             int
	SubscriptionID int
	Name           string
	UsersRemoved   []string
	UsersKept      []string
	// UsersKeptOnEngine: the Agent found rights the owner gave by hand and
	// kept the account (only CelikPanel's grant on this database went).
	UsersKeptOnEngine []string
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
	// recordsLeft: the engine is done; the records were not removed.
	recordsLeft bool
	refusal     *transport.DatabaseClusterRefusal
	// engine: an error from the record's credential path.
	engine   error
	server   *core.DatabaseServer
	deletion *databaseDeletion
	cause    error
}

func (e *databaseDeleteError) Error() string { return e.cause.Error() }
func (e *databaseDeleteError) Unwrap() error { return e.cause }

// databaseNeverRemovedUser: accounts a database deletion never removes, even
// when a record names them: the engines' own (as the Agent refuses them,
// cmd/agent/suspension_database.go), CelikPanel's, the record's own account,
// and a name CelikPanel cannot manage (the Agent would refuse it on every
// press).
func databaseNeverRemovedUser(name string, server *core.DatabaseServer) bool {
	lowered := strings.ToLower(strings.TrimSpace(name))
	switch lowered {
	case "", "root", "mysql", "mariadb.sys", "mysql.sys", "mysql.session", "mysql.infoschema", "postgres",
		"debian-sys-maint", strings.ToLower(transport.DatabaseAdminAccountName):
		return true
	}
	if strings.HasPrefix(lowered, "pg_") || services.ValidateSQLIdentifier(name) != nil {
		return true
	}
	return server != nil && strings.EqualFold(strings.TrimSpace(server.AdminUsername), strings.TrimSpace(name))
}

// databaseEngineIdentity names the engine a record points at: its kind, the
// machine (this one, or the recorded address) and the port (the kind's
// default when none is recorded). Two records with one identity reach the
// same accounts.
func databaseEngineIdentity(kind, host string, port int) string {
	if port == 0 {
		port = mariaDBSocketEnginePort
		if kind == "postgresql" {
			port = 5432
		}
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if databaseEngineIsOnThisMachine(host) {
		host = "this-machine"
	}
	return kind + "@" + host + ":" + strconv.Itoa(port)
}

func databaseServerEngineIdentity(server *core.DatabaseServer) string {
	return databaseEngineIdentity(dbDriverTypeFor(server), server.Host, server.Port)
}

// databaseEngineUserLocks serialise, per engine, deciding which accounts a
// deletion removes with giving an account to a database (alpha.133 round 5).
var databaseEngineUserLocks keyedLocks

// lockDatabaseEngineUsers waits for the engine of a record and returns the
// release.
func lockDatabaseEngineUsers(server *core.DatabaseServer) func() {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(databaseServerEngineIdentity(server)))
	return databaseEngineUserLocks.lock(int(hash.Sum32() & 0x7fffffff))
}

// databaseQueryer is a pool or a transaction.
type databaseQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// databaseDeleteUsers reads the users recorded for the database and what
// becomes of each when it is deleted.
func databaseDeleteUsers(ctx context.Context, q databaseQueryer, databaseID int, server *core.DatabaseServer, subscriptionID int) ([]databaseDeleteUser, error) {
	identity := databaseServerEngineIdentity(server)
	rows, err := q.QueryContext(ctx, `
		SELECT u.id, u.username, u.server_id, u.subscription_id
		FROM database_user_grants g
		JOIN database_users u ON u.id = g.user_id
		WHERE g.database_id = ?
		ORDER BY g.id`, databaseID)
	if err != nil {
		return nil, fmt.Errorf("read the users of database %d: %w", databaseID, err)
	}
	type recorded struct {
		user                   databaseDeleteUser
		serverID, subscription int
	}
	var found []recorded
	seen := map[int]bool{}
	for rows.Next() {
		var r recorded
		if err := rows.Scan(&r.user.ID, &r.user.Username, &r.serverID, &r.subscription); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read a user of database %d: %w", databaseID, err)
		}
		if !seen[r.user.ID] {
			seen[r.user.ID] = true
			found = append(found, r)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	users := make([]databaseDeleteUser, 0, len(found))
	for _, r := range found {
		user := r.user
		switch {
		case r.serverID != server.ID || r.subscription != subscriptionID:
			user.Stays = databaseUserShared
		case databaseNeverRemovedUser(user.Username, server):
			user.Stays = databaseUserAccount
		default:
			shared, err := databaseAccountUsedElsewhere(ctx, q, databaseID, user, identity)
			if err != nil {
				return nil, err
			}
			if shared {
				user.Stays = databaseUserShared
			}
		}
		users = append(users, user)
	}
	return users, nil
}

// databaseAccountUsedElsewhere: whether the engine account behind a user
// record is used by anything but this database: a grant of the record on
// another database, or another record of the same name on the same engine
// (any server record, any subscription).
func databaseAccountUsedElsewhere(ctx context.Context, q databaseQueryer, databaseID int, user databaseDeleteUser, identity string) (bool, error) {
	var grants int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = ? AND database_id <> ?`, user.ID, databaseID).Scan(&grants); err != nil {
		return false, fmt.Errorf("read the other grants of %s: %w", user.Username, err)
	}
	if grants > 0 {
		return true, nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT dst.name, COALESCE(ds.host, ''), ds.port
		FROM database_users ou
		JOIN database_servers ds ON ds.id = ou.server_id
		JOIN database_server_types dst ON dst.id = ds.type_id
		WHERE ou.username = ? AND ou.id <> ?`, user.Username, user.ID)
	if err != nil {
		return false, fmt.Errorf("read the other records of %s: %w", user.Username, err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind, host string
		var port int
		if err := rows.Scan(&kind, &host, &port); err != nil {
			return false, err
		}
		if databaseEngineIdentity(dbDriverTypeFor(&core.DatabaseServer{TypeName: kind}), host, port) == identity {
			return true, nil
		}
	}
	return false, rows.Err()
}

// databaseUserFates is what becomes of each user of each database if it is
// deleted, for the lists that ask before a deletion (user id -> Stays).
func (p *Panel) databaseUserFates(ctx context.Context, databaseIDs []int) (map[int]map[int]string, error) {
	out := map[int]map[int]string{}
	repo := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB())
	servers := map[int]*core.DatabaseServer{}
	for _, id := range databaseIDs {
		var serverID, subscriptionID int
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT server_id, subscription_id FROM databases_v2 WHERE id = ?`, id).Scan(&serverID, &subscriptionID); err != nil {
			return nil, err
		}
		server := servers[serverID]
		if server == nil {
			loaded, err := repo.GetByID(ctx, serverID)
			if err != nil {
				return nil, err
			}
			server, servers[serverID] = loaded, loaded
		}
		users, err := databaseDeleteUsers(ctx, p.db.GetDB(), id, server, subscriptionID)
		if err != nil {
			return nil, err
		}
		out[id] = map[int]string{}
		for _, user := range users {
			out[id][user.ID] = user.Stays
		}
	}
	return out, nil
}

// deleteDatabaseRecord deletes the database with this id: on the engine, its
// own users, then its records. The caller has authorized it.
func (p *Panel) deleteDatabaseRecord(ctx context.Context, databaseID int) (*databaseDeletion, error) {
	deletion := &databaseDeletion{ID: databaseID}
	var serverID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT name, server_id, subscription_id FROM databases_v2 WHERE id = ?`, databaseID).
		Scan(&deletion.Name, &serverID, &deletion.SubscriptionID); err != nil {
		return nil, fmt.Errorf("read database %d: %w", databaseID, err)
	}
	server, err := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB()).GetByID(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("read the server of database %d: %w", databaseID, err)
	}
	unlock := lockDatabaseEngineUsers(server)
	defer unlock()
	users, err := databaseDeleteUsers(ctx, p.db.GetDB(), databaseID, server, deletion.SubscriptionID)
	if err != nil {
		return nil, err
	}
	var remove []databaseDeleteUser
	for _, user := range users {
		if user.Stays == databaseUserRemoved {
			remove = append(remove, user)
		} else {
			deletion.UsersKept = append(deletion.UsersKept, user.Username)
		}
	}
	if databaseDeleteAfterClassify != nil {
		databaseDeleteAfterClassify()
	}

	reportOperationStep(ctx, "delete")
	var keptOnEngine map[string]bool
	// Only CelikPanel's own record of the machine's engine goes through the
	// Agent's door; a record an administrator registered with their own
	// credential (127.0.0.1:3307, a container) is reached with it.
	if databaseRecordIsOurLocalEngine(server) {
		keptOnEngine, err = p.deleteDatabaseOnThisMachine(ctx, server, deletion.Name, remove)
	} else {
		err = p.deleteDatabaseElsewhere(server, deletion.Name, remove)
	}
	var failed *databaseDeleteError
	errors.As(err, &failed)
	left := map[string]bool{}
	if failed != nil {
		for _, name := range failed.usersLeft {
			left[name] = true
		}
	}
	for _, user := range remove {
		switch {
		case keptOnEngine[user.Username]:
			deletion.UsersKeptOnEngine = append(deletion.UsersKeptOnEngine, user.Username)
		case err == nil || (failed != nil && failed.databaseGone && !left[user.Username]):
			deletion.UsersRemoved = append(deletion.UsersRemoved, user.Username)
		}
	}
	if err != nil {
		if failed != nil {
			failed.deletion = deletion
		}
		return nil, err
	}

	reportOperationStep(ctx, "records")
	recordsLeft := func(cause error) error {
		return &databaseDeleteError{databaseGone: true, recordsLeft: true, deletion: deletion, cause: cause}
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, recordsLeft(fmt.Errorf("begin the records of the deleted database %s: %w", deletion.Name, err))
	}
	defer func() { _ = tx.Rollback() }()
	// The lock keeps every grant path of this process out; a record that
	// gained a grant anyway is kept and said in the log.
	for _, user := range remove {
		var grants int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM database_user_grants WHERE user_id = ? AND database_id <> ?`, user.ID, databaseID).Scan(&grants); err != nil {
			return nil, recordsLeft(fmt.Errorf("re-check the grants of %s: %w", user.Username, err))
		}
		if grants > 0 {
			log.Printf("database deletion %s: user %s was given to another database while it was being removed; its record stays", deletion.Name, user.Username)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM database_user_grants WHERE database_id = ?`, databaseID); err != nil {
		return nil, recordsLeft(fmt.Errorf("remove the grants of database %s: %w", deletion.Name, err))
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM databases_v2 WHERE id = ?`, databaseID); err != nil {
		return nil, recordsLeft(fmt.Errorf("remove the record of database %s: %w", deletion.Name, err))
	}
	for _, user := range remove {
		// Removed from the engine, or kept there with the owner's own rights:
		// either way no longer CelikPanel's.
		if _, err := tx.ExecContext(ctx, `DELETE FROM database_users WHERE id = ? AND NOT EXISTS (SELECT 1 FROM database_user_grants WHERE user_id = ?)`, user.ID, user.ID); err != nil {
			return nil, recordsLeft(fmt.Errorf("remove the record of user %s: %w", user.Username, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, recordsLeft(fmt.Errorf("commit the removal of database %s: %w", deletion.Name, err))
	}
	kickDatabaseRemoteSync() // alpha.129: after the committed change only (remote database access follows)
	return deletion, nil
}

// deleteDatabaseOnThisMachine drops the database and then its own users
// through the Agent. It answers the users the Agent kept for rights the
// owner gave by hand.
func (p *Panel) deleteDatabaseOnThisMachine(ctx context.Context, server *core.DatabaseServer, name string, remove []databaseDeleteUser) (map[string]bool, error) {
	engine, port := "mysql", 0
	if dbDriverTypeFor(server) == "postgresql" {
		engine, port = "postgresql", server.Port
	}
	var resp transport.DeleteDatabaseResponse
	// No user here: the database's own users go below, each only when nothing
	// else uses it (the Agent's old user field dropped one blindly).
	req := transport.DeleteDatabaseRequest{Type: engine, Name: name, Port: port}
	if err := databaseDeleteAgent(p, ctx, req, &resp); err != nil {
		return nil, &databaseDeleteError{unknown: true, cause: fmt.Errorf("drop database %s: %w", name, err)}
	}
	if resp.Refusal != nil {
		return nil, &databaseDeleteError{refusal: resp.Refusal, cause: fmt.Errorf("drop database %s: its PostgreSQL cluster cannot take it", name)}
	}
	if !resp.Success {
		log.Printf("[409][database] %s %s: %s", errCodeDatabaseDeleteFailed, name, boundedAgentDiagnostic(resp.Error))
		return nil, &databaseDeleteError{cause: fmt.Errorf("drop database %s: the engine did not drop it", name)}
	}
	kept := map[string]bool{}
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
		case answer.Kept:
			kept[user.Username] = true
		}
	}
	if len(left) > 0 {
		return kept, &databaseDeleteError{unknown: unknown, databaseGone: true, usersLeft: left,
			cause: fmt.Errorf("database %s was dropped; its users %s were not all removed", name, strings.Join(left, ", "))}
	}
	return kept, nil
}

// deleteDatabaseElsewhere drops the database and its own users on an engine
// reached with the record's own credential.
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

// databaseDeletionAnswer is the success body of both delete routes.
func databaseDeletionAnswer(status string, deletion *databaseDeletion) map[string]any {
	return map[string]any{"status": status,
		"users_removed":        nonNilStrings(deletion.UsersRemoved),
		"users_kept":           nonNilStrings(deletion.UsersKept),
		"users_kept_on_engine": nonNilStrings(deletion.UsersKeptOnEngine)}
}

// auditDatabaseDeletion records a deletion that changed the engine, on
// either route: who acted (the session and, during "Log in as", the account
// acted as), whose database it was, which users went and which stayed, and
// how it ended.
func (p *Panel) auditDatabaseDeletion(r *http.Request, deletion *databaseDeletion, outcome string) {
	if deletion == nil {
		return
	}
	details := map[string]any{"outcome": outcome,
		"users_removed": nonNilStrings(deletion.UsersRemoved), "users_kept": nonNilStrings(deletion.UsersKept),
		"users_kept_on_engine": nonNilStrings(deletion.UsersKeptOnEngine)}
	if owner, ok := p.databaseSubscriptionOwner(r.Context(), deletion.SubscriptionID); ok {
		details["owner"], details["owner_id"] = owner.Username, owner.ID
	}
	if outcome == "deleted" {
		p.auditDetailed(r, "database.delete:"+deletion.Name, "database", deletion.ID, details)
		return
	}
	p.auditDetailed(r, "database.delete.partial:"+deletion.Name, "database", deletion.ID, details)
}

// writeDatabaseDeleteError answers a deletion that did not finish, in the
// product's error shape: a code the screen has words for, the facts as vars,
// and the in-panel fix only for an administrator. A deletion that changed
// the engine is recorded in the audit log first.
func (p *Panel) writeDatabaseDeleteError(w http.ResponseWriter, r *http.Request, name string, err error) {
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
		p.auditDatabaseDeletion(r, failed.deletion, "users_left")
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
	case failed.recordsLeft:
		p.auditDatabaseDeletion(r, failed.deletion, "records_left")
		log.Printf("[500][database] %s: %v", errCodeDatabaseDeleteRecordsLeft, failed.cause)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(apiErrorBody{
			Error: "The database " + name + " was deleted on the database server, but CelikPanel's records of it were not removed. " +
				"Press Delete again to finish.",
			Code: errCodeDatabaseDeleteRecordsLeft, PartialSuccess: true, MutationApplied: true,
			Vars: map[string]string{"name": name},
		})
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
// anybody else is told to ask the server administrator. Every answer carries
// the engine's name its sentence needs. The engine's own words stay in the
// log.
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
		if r == nil {
			writeServerError(w, err)
		} else {
			writeServerErrorFor(w, r, err)
		}
		return
	}
	code := errCodeDatabaseEngineUnreachable
	if refusal == services.DatabaseEngineRefusalCredential {
		code = errCodeDatabaseEngineCredentialRefused
	}
	log.Printf("[409][database] %s: %v", code, err)
	body := apiErrorBody{Error: message, Code: code, Vars: map[string]string{"engine": databaseEngineDisplayName(driverType)}}
	// Without a request the general sentence (ask the server administrator).
	if caller := callerOf(r); caller != nil && caller.hasAccountRole(roleAdmin) {
		body.Reason = "administrator"
		if driverType == "mariadb" || driverType == "postgresql" {
			body.Action = "/components/" + driverType
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(body)
}

// callerOf is currentCaller for a request that may be absent.
func callerOf(r *http.Request) *Caller {
	if r == nil {
		return nil
	}
	return currentCaller(r)
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
