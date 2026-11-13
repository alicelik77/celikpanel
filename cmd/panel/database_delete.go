package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
// (acquireDatabaseEngineGate), so a user is never given to another database
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

// databaseDroppedRightsAgent is the Agent call that takes every right naming
// a deleted database from every grantee on this machine's MariaDB
// (alpha.134); tests replace it.
var databaseDroppedRightsAgent = func(p *Panel, ctx context.Context, req transport.DroppedDatabaseRightsRequest, resp *transport.DroppedDatabaseRightsResponse) error {
	return p.callAgentContext(ctx, "Agent.RemoveDroppedDatabaseRightsV1", &req, resp)
}

// databaseDroppedRightsElsewhere takes the same rights away on a MariaDB
// reached with the record's own credential; tests replace it.
var databaseDroppedRightsElsewhere = func(driver services.DatabaseDriver, name string) ([]string, error) {
	sweeper, ok := driver.(interface {
		RemoveRightsOnDroppedDatabase(string) ([]string, error)
	})
	if !ok {
		return nil, nil
	}
	return sweeper.RemoveRightsOnDroppedDatabase(name)
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
	// databaseUserServer: the database's server record is reached with its
	// own account (another machine, or an administrator's own record on this
	// one); CelikPanel does not remove users there (review W-A).
	databaseUserServer = "server"
	// databaseUserUnknown: what deleting would leave could not be read (lists
	// only; the screens then offer no deletion).
	databaseUserUnknown = "unknown"
	// databaseUserServerShared: another database uses it AND the database's
	// server record is reached with its own account; the user stays on that
	// server untouched (no rights are taken; review). Its record stays, as
	// for every shared user.
	databaseUserServerShared = "server_shared"
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
	// UsersKeptUnchecked: the Agent kept the accounts because what else they
	// have could not be checked (fail closed).
	UsersKeptUnchecked []string
	// UsersKeptOnServer: an engine reached with the record's own account;
	// CelikPanel does not remove users there.
	UsersKeptOnServer []string
	// UsersLeft: users a deletion that did not finish still has to do
	// something about; UsersRightsLeft: those of them that stay on the engine
	// but still hold a right on the dropped database (for the audit row).
	UsersLeft       []string
	UsersRightsLeft []string
	// RightsRemovedFrom (alpha.134): every grantee (user@host, a role's name
	// alone) that held a right naming the deleted database and lost it -
	// accounts of any name and roles, which stay. Names only; the answer
	// gives them to administrators, everybody else gets the count.
	RightsRemovedFrom []string
	// RightsMayRemain (alpha.134): on a MariaDB reached with the record's own
	// credential that account could not take those rights away; the
	// administrator of that server removes them.
	RightsMayRemain bool
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
	// rightsLeft: of usersLeft, the accounts that stay but still hold a
	// right naming the dropped database (the Agent's RightsLeft).
	rightsLeft []string
	// databaseExists: the engine has a database of that name again, so the
	// Agent removed nothing from the users.
	databaseExists bool
	// sweepLeft (alpha.134): rights naming the dropped database could not all
	// be taken from every grantee (sweepUnknown: no answer came back).
	sweepLeft, sweepUnknown bool
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

// The engine-account gate (alpha.133 round 5, narrowed after review W-C).
// A deletion holds it for one engine from choosing which accounts it removes
// until its records are written; the few steps that give an EXISTING user
// record to a database (a grant, a new database for an existing user, a
// domain database reusing a record of its name) hold it only around that
// step. Nobody waits silently or long: such a step waits at most
// databaseEngineLinkWait for a deletion in progress and is otherwise refused
// with DATABASE_ENGINE_BUSY ("another database change on this server is
// running; try again shortly"); a deletion waits at most
// databaseEngineDeleteWait for such a step. Creating a database with a new
// user never takes it.
// Bir silme, bir motorda kullanıcıları seçtiği andan kayıtlarını yazana dek
// kapıyı tutar; var olan bir kullanıcıyı bağlayan adım yalnız o adımda tutar
// ve en çok birkaç saniye bekler, sonra "biraz sonra yeniden deneyin" der.
var databaseEngineGates = struct {
	sync.Mutex
	gates map[string]chan struct{}
}{gates: map[string]chan struct{}{}}

// The longest anything waits at the gate (tests shorten them).
var (
	databaseEngineLinkWait   = 10 * time.Second
	databaseEngineDeleteWait = 30 * time.Second
)

const (
	// errCodeDatabaseEngineBusy: another database change on this engine is
	// running; nothing was changed.
	errCodeDatabaseEngineBusy = "DATABASE_ENGINE_BUSY"
)

// errDatabaseEngineBusy: the gate was not free within the bound.
var errDatabaseEngineBusy = errors.New("another database change on this database server is running")

// acquireDatabaseEngineGate waits at most `wait` for the gate of a record's
// engine and returns its release.
func acquireDatabaseEngineGate(ctx context.Context, server *core.DatabaseServer, wait time.Duration) (func(), error) {
	key := databaseServerEngineIdentity(server)
	databaseEngineGates.Lock()
	gate := databaseEngineGates.gates[key]
	if gate == nil {
		gate = make(chan struct{}, 1)
		databaseEngineGates.gates[key] = gate
	}
	databaseEngineGates.Unlock()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-timer.C:
		return nil, errDatabaseEngineBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// writeDatabaseEngineBusy answers a step refused by the gate.
func writeDatabaseEngineBusy(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodeDatabaseEngineBusy,
		"Another database change on this database server is running, so nothing was changed. Try again in a moment.", "")
}

// databaseQueryer is a pool or a transaction.
type databaseQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// databaseUserFatesOf reads what deleting each of the databases would do to
// each of its users, for any number of databases in five queries (review
// N-F: the lists ask this on every read). A user stays when:
//   - it is recorded for another server record or subscription (shared);
//   - it is an engine's or CelikPanel's own account (account);
//   - another database uses the same engine account: a grant of the record on
//     another database, or another record of the same name on the same
//     engine, any server record, any subscription (shared);
//   - the database's record is not CelikPanel's own record of this machine's
//     engine: such an engine is reached with the record's own account, and
//     CelikPanel does not remove users there (server; review W-A).
//
// Bir veritabanı silinirse her kullanıcısına ne olacağı; kaç veritabanı
// olursa olsun beş sorgu.
func databaseUserFatesOf(ctx context.Context, q databaseQueryer, databaseIDs []int) (map[int][]databaseDeleteUser, error) {
	out := map[int][]databaseDeleteUser{}
	if len(databaseIDs) == 0 {
		return out, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(databaseIDs)), ",")
	ids := make([]any, len(databaseIDs))
	for i, id := range databaseIDs {
		ids[i] = id
	}

	servers := map[int]*core.DatabaseServer{}
	rows, err := q.QueryContext(ctx, `
		SELECT ds.id, dst.name, COALESCE(ds.host, ''), ds.port, COALESCE(ds.admin_username, ''), COALESCE(ds.root_password_encrypted, '')
		FROM database_servers ds JOIN database_server_types dst ON dst.id = ds.type_id`)
	if err != nil {
		return nil, fmt.Errorf("read the database servers: %w", err)
	}
	for rows.Next() {
		server := &core.DatabaseServer{}
		if err := rows.Scan(&server.ID, &server.TypeName, &server.Host, &server.Port, &server.AdminUsername, &server.AdminPasswordEncrypted); err != nil {
			rows.Close()
			return nil, err
		}
		servers[server.ID] = server
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	type place struct{ server, subscription int }
	databases := map[int]place{}
	rows, err = q.QueryContext(ctx, `SELECT id, server_id, subscription_id FROM databases_v2 WHERE id IN (`+marks+`)`, ids...)
	if err != nil {
		return nil, fmt.Errorf("read the databases: %w", err)
	}
	for rows.Next() {
		var id int
		var at place
		if err := rows.Scan(&id, &at.server, &at.subscription); err != nil {
			rows.Close()
			return nil, err
		}
		databases[id] = at
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	type granted struct {
		database int
		user     databaseDeleteUser
		at       place
	}
	var grants []granted
	rows, err = q.QueryContext(ctx, `
		SELECT g.database_id, u.id, u.username, u.server_id, u.subscription_id
		FROM database_user_grants g JOIN database_users u ON u.id = g.user_id
		WHERE g.database_id IN (`+marks+`)
		ORDER BY g.database_id, g.id`, ids...)
	if err != nil {
		return nil, fmt.Errorf("read the users of the databases: %w", err)
	}
	for rows.Next() {
		var g granted
		if err := rows.Scan(&g.database, &g.user.ID, &g.user.Username, &g.at.server, &g.at.subscription); err != nil {
			rows.Close()
			return nil, err
		}
		grants = append(grants, g)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Every grant of these users, on any database.
	otherGrants := map[int][]int{}
	rows, err = q.QueryContext(ctx, `
		SELECT user_id, database_id FROM database_user_grants
		WHERE user_id IN (SELECT user_id FROM database_user_grants WHERE database_id IN (`+marks+`))`, ids...)
	if err != nil {
		return nil, fmt.Errorf("read the other grants of the users: %w", err)
	}
	for rows.Next() {
		var user, database int
		if err := rows.Scan(&user, &database); err != nil {
			rows.Close()
			return nil, err
		}
		otherGrants[user] = append(otherGrants[user], database)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Every record of the same names, on any server record.
	sameName := map[string][]databaseUserRecordRef{}
	rows, err = q.QueryContext(ctx, `
		SELECT id, username, server_id FROM database_users
		WHERE username IN (SELECT u.username FROM database_user_grants g JOIN database_users u ON u.id = g.user_id WHERE g.database_id IN (`+marks+`))`, ids...)
	if err != nil {
		return nil, fmt.Errorf("read the records of the same names: %w", err)
	}
	for rows.Next() {
		var n databaseUserRecordRef
		var name string
		if err := rows.Scan(&n.id, &name, &n.server); err != nil {
			rows.Close()
			return nil, err
		}
		sameName[name] = append(sameName[name], n)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	seen := map[[2]int]bool{}
	for _, g := range grants {
		if seen[[2]int{g.database, g.user.ID}] {
			continue
		}
		seen[[2]int{g.database, g.user.ID}] = true
		at, ok := databases[g.database]
		server := servers[at.server]
		if !ok || server == nil {
			return nil, fmt.Errorf("database %d has no server record", g.database)
		}
		user := g.user
		identity := databaseServerEngineIdentity(server)
		switch {
		case g.at != at:
			user.Stays = databaseUserShared
		case databaseNeverRemovedUser(user.Username, server):
			user.Stays = databaseUserAccount
		case databaseAccountElsewhere(g.database, user, otherGrants[user.ID], sameName[user.Username], servers, identity):
			user.Stays = databaseUserShared
		case !databaseRecordIsOurLocalEngine(server):
			user.Stays = databaseUserServer
		}
		// On a record reached with its own account nothing is taken from a
		// shared user: said as such, never "its rights go". Which records are
		// forgotten does not change (only the "server" case is).
		if user.Stays == databaseUserShared && !databaseRecordIsOurLocalEngine(server) {
			user.Stays = databaseUserServerShared
		}
		out[g.database] = append(out[g.database], user)
	}
	return out, nil
}

// databaseUserRecordRef is one user record: its id and server record.
type databaseUserRecordRef struct{ id, server int }

// databaseAccountElsewhere: the engine account behind a user record is used
// by something other than this database.
func databaseAccountElsewhere(database int, user databaseDeleteUser, grants []int, records []databaseUserRecordRef, servers map[int]*core.DatabaseServer, identity string) bool {
	for _, other := range grants {
		if other != database {
			return true
		}
	}
	for _, record := range records {
		if record.id == user.ID {
			continue
		}
		if server := servers[record.server]; server != nil && databaseServerEngineIdentity(server) == identity {
			return true
		}
	}
	return false
}

// databaseUserFates is what becomes of each user of each listed database if
// it is deleted (user id -> Stays). A failure to tell is "unknown" for every
// user, logged; the list itself still loads, and the screens then offer no
// deletion they cannot describe.
func (p *Panel) databaseUserFates(ctx context.Context, databaseIDs []int) map[int]map[int]string {
	out := map[int]map[int]string{}
	fates, err := databaseUserFatesRead(ctx, p.db.GetDB(), databaseIDs)
	if err != nil {
		log.Printf("database lists: what deleting would leave of each user could not be read: %v", err)
	}
	for _, id := range databaseIDs {
		out[id] = map[int]string{}
		for _, user := range fates[id] {
			out[id][user.ID] = user.Stays
		}
		if err != nil {
			out[id][0] = databaseUserUnknown
		}
	}
	return out
}

// databaseUserFatesRead is databaseUserFatesOf for the lists; tests replace it.
var databaseUserFatesRead = databaseUserFatesOf

// databaseUserStays is the Stays a list answers for one user.
func databaseUserStays(fates map[int]map[int]string, databaseID, userID int) string {
	if fates[databaseID][0] == databaseUserUnknown {
		return databaseUserUnknown
	}
	return fates[databaseID][userID]
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
	release, err := acquireDatabaseEngineGate(ctx, server, databaseEngineDeleteWait)
	if err != nil {
		return nil, err
	}
	defer release()
	fates, err := databaseUserFatesOf(ctx, p.db.GetDB(), []int{databaseID})
	if err != nil {
		return nil, err
	}
	// remove: CelikPanel removes them from the engine. forget: they stay on
	// an engine CelikPanel reaches with the record's own account, and their
	// records go with the database (nothing else uses them).
	// shared: they stay because another database uses them, but their rights
	// on this database go with it (review: CelikPanel's own GRANT on it would
	// otherwise stay in mysql.db).
	var remove, forget, shared []databaseDeleteUser
	for _, user := range fates[databaseID] {
		switch user.Stays {
		case databaseUserRemoved:
			remove = append(remove, user)
		case databaseUserServer:
			forget = append(forget, user)
			deletion.UsersKeptOnServer = append(deletion.UsersKeptOnServer, user.Username)
		case databaseUserShared:
			shared = append(shared, user)
			deletion.UsersKept = append(deletion.UsersKept, user.Username)
		case databaseUserServerShared:
			deletion.UsersKept = append(deletion.UsersKept, user.Username)
		default:
			deletion.UsersKept = append(deletion.UsersKept, user.Username)
		}
	}
	if databaseDeleteAfterClassify != nil {
		databaseDeleteAfterClassify()
	}

	reportOperationStep(ctx, "delete")
	var keptOnEngine map[string]string
	// Only CelikPanel's own record of the machine's engine goes through the
	// Agent's door; a record an administrator registered with their own
	// credential (127.0.0.1:3307, a container) is reached with it.
	if databaseRecordIsOurLocalEngine(server) {
		keptOnEngine, err = p.deleteDatabaseOnThisMachine(ctx, server, deletion, remove, shared)
	} else {
		err = p.deleteDatabaseElsewhere(server, deletion)
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
		case keptOnEngine[user.Username] == transport.DatabaseAccessKeptUnchecked:
			deletion.UsersKeptUnchecked = append(deletion.UsersKeptUnchecked, user.Username)
		case keptOnEngine[user.Username] != "":
			deletion.UsersKeptOnEngine = append(deletion.UsersKeptOnEngine, user.Username)
		case err == nil || (failed != nil && failed.databaseGone && !left[user.Username]):
			deletion.UsersRemoved = append(deletion.UsersRemoved, user.Username)
		}
	}
	if err != nil {
		if failed != nil {
			deletion.UsersLeft, deletion.UsersRightsLeft = failed.usersLeft, failed.rightsLeft
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
	for _, user := range append(remove, forget...) {
		// Removed from the engine, or kept there (the owner's own rights, or
		// an engine reached with the record's account): either way no longer
		// CelikPanel's.
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
func (p *Panel) deleteDatabaseOnThisMachine(ctx context.Context, server *core.DatabaseServer, deletion *databaseDeletion, remove, shared []databaseDeleteUser) (map[string]string, error) {
	name := deletion.Name
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
	kept := map[string]string{}
	var left, rightsLeft []string
	unknown, exists := false, false
	// remove: the user goes unless the Agent finds rights its owner gave by
	// hand. shared: the user stays; only its rights on the dropped database go
	// (RightsOnly). Both tell the Agent the database is dropped; it checks
	// that itself and touches nothing while a database of that name exists.
	type step struct {
		user       databaseDeleteUser
		rightsOnly bool
	}
	var steps []step
	for _, user := range remove {
		steps = append(steps, step{user: user})
	}
	for _, user := range shared {
		steps = append(steps, step{user: user, rightsOnly: true})
	}
	for _, s := range steps {
		answer, err := p.databaseAccessNativeOps().remove(ctx, transport.DatabaseAccessUserRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), Engine: engine, Port: port,
			Database: name, Username: s.user.Username, DatabaseDropped: true, RightsOnly: s.rightsOnly,
		})
		switch {
		case err != nil:
			unknown = true
			left = append(left, s.user.Username)
			if s.rightsOnly {
				// It stays by design; what is not known is whether its
				// rights on the database went.
				rightsLeft = append(rightsLeft, s.user.Username)
			}
			log.Printf("database deletion %s: user %s: %v", name, s.user.Username, err)
		case answer.Refusal != nil || !answer.Applied:
			left = append(left, s.user.Username)
			if answer.DatabaseExists {
				exists = true
			}
			if answer.RightsLeft || (s.rightsOnly && !answer.DatabaseExists) {
				rightsLeft = append(rightsLeft, s.user.Username)
			}
			log.Printf("[409][database] %s %s: user %s: %s", errCodeDatabaseDeleteUsersLeft, name, s.user.Username, boundedAgentDiagnostic(answer.Error))
		case answer.Kept:
			kept[s.user.Username] = answer.KeptReason
			if kept[s.user.Username] == "" {
				kept[s.user.Username] = transport.DatabaseAccessKeptOtherRights
			}
		}
	}
	// alpha.134: every right that names the dropped database goes from every
	// grantee - accounts made by hand under any name and roles, which stay -
	// once its own users were handled (MariaDB only: PostgreSQL's DROP
	// DATABASE leaves nothing that names it). Not while a database of that
	// name is there again.
	// Silinen veritabanını adlandıran haklar her hesaptan ve rolden alınır.
	sweepLeft, sweepUnknown := false, false
	if engine == "mysql" && !exists {
		var swept transport.DroppedDatabaseRightsResponse
		err := databaseDroppedRightsAgent(p, ctx, transport.DroppedDatabaseRightsRequest{
			ExpectedBuildCommit: strings.TrimSpace(buildCommit), Database: name}, &swept)
		switch {
		case err != nil:
			sweepLeft, sweepUnknown = true, true
			log.Printf("database deletion %s: rights naming it on other accounts: %v", name, err)
		case swept.DatabaseExists:
			exists = true
			log.Printf("[409][database] %s %s: rights naming it on other accounts: %s", errCodeDatabaseDeleteUsersLeft, name, boundedAgentDiagnostic(swept.Error))
		case !swept.Applied:
			sweepLeft = true
			log.Printf("[409][database] %s %s: rights naming it on other accounts: %s", errCodeDatabaseDeleteUsersLeft, name, boundedAgentDiagnostic(swept.Error))
		default:
			deletion.RightsRemovedFrom = append([]string(nil), swept.Grantees...)
		}
	}
	if len(left) > 0 || sweepLeft || exists {
		return kept, &databaseDeleteError{unknown: unknown, databaseGone: true, usersLeft: left, rightsLeft: rightsLeft, databaseExists: exists,
			sweepLeft: sweepLeft, sweepUnknown: sweepUnknown,
			cause: fmt.Errorf("database %s was dropped; for its users %s or the rights naming it not everything was done", name, strings.Join(left, ", "))}
	}
	return kept, nil
}

// deleteDatabaseElsewhere drops the database on an engine reached with the
// record's own credential (another machine, or an administrator's own record
// on this one). Its users stay (review W-A): removing them safely needs a
// look at the rights the owner gave there, which the record's account may
// not be able to read; the answer says they stay and why.
// Kendi hesabıyla bağlanılan sunucuda yalnız veritabanı silinir; kullanıcılar
// kalır.
//
// alpha.134: on MariaDB it then tries, with the same credential, to take
// every right that names the dropped database from every grantee. When that
// account cannot, the deletion still finishes (the database is gone) and the
// answer says rights may remain there and that the administrator of that
// server removes them.
func (p *Panel) deleteDatabaseElsewhere(server *core.DatabaseServer, deletion *databaseDeletion) error {
	name := deletion.Name
	driver, err := p.dbDriverFor(server)
	if err != nil {
		return fmt.Errorf("open the credential of database server %d: %w", server.ID, err)
	}
	if err := driver.DeleteDatabase(name); err != nil {
		return &databaseDeleteError{engine: err, server: server, cause: fmt.Errorf("drop database %s: %w", name, err)}
	}
	if dbDriverTypeFor(server) == "mariadb" {
		grantees, err := databaseDroppedRightsElsewhere(driver, name)
		if err != nil {
			deletion.RightsMayRemain = true
			log.Printf("database deletion %s on server %d: rights naming it were not removed with the record's account: %v", name, server.ID, err)
		} else {
			deletion.RightsRemovedFrom = grantees
		}
	}
	return nil
}

// databaseDeletionAnswer is the success body of both delete routes.
func databaseDeletionAnswer(status string, deletion *databaseDeletion) map[string]any {
	return map[string]any{"status": status,
		"users_removed":        nonNilStrings(deletion.UsersRemoved),
		"users_kept":           nonNilStrings(deletion.UsersKept),
		"users_kept_on_engine": nonNilStrings(deletion.UsersKeptOnEngine),
		"users_kept_unchecked": nonNilStrings(deletion.UsersKeptUnchecked),
		"users_kept_on_server": nonNilStrings(deletion.UsersKeptOnServer),
		// alpha.134: how many grantees lost a right naming the database; the
		// names are added for administrators only (databaseDeletionAnswerFor).
		"rights_removed_count": len(deletion.RightsRemovedFrom),
		"rights_may_remain":    deletion.RightsMayRemain}
}

// databaseDeletionAnswerFor is databaseDeletionAnswer for the caller: an
// administrator also gets the names of the grantees whose rights went.
func databaseDeletionAnswerFor(r *http.Request, status string, deletion *databaseDeletion) map[string]any {
	answer := databaseDeletionAnswer(status, deletion)
	if caller := currentCaller(r); caller != nil && caller.hasAccountRole(roleAdmin) {
		answer["rights_removed_from"] = nonNilStrings(deletion.RightsRemovedFrom)
	}
	return answer
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
		"users_kept_on_engine": nonNilStrings(deletion.UsersKeptOnEngine),
		"users_kept_unchecked": nonNilStrings(deletion.UsersKeptUnchecked),
		"users_kept_on_server": nonNilStrings(deletion.UsersKeptOnServer),
		"rights_removed_count": len(deletion.RightsRemovedFrom),
		"rights_removed_from":  nonNilStrings(deletion.RightsRemovedFrom)}
	if deletion.RightsMayRemain {
		details["rights_may_remain"] = true
	}
	if len(deletion.UsersLeft) > 0 {
		details["users_left"] = deletion.UsersLeft
		details["users_rights_left"] = nonNilStrings(deletion.UsersRightsLeft)
	}
	if owner, ok := p.databaseSubscriptionOwner(r.Context(), deletion.SubscriptionID); ok {
		details["owner"], details["owner_id"] = owner.Username, owner.ID
	}
	switch outcome {
	case "deleted":
		p.auditDetailed(r, "database.delete:"+deletion.Name, "database", deletion.ID, details)
	case "unknown":
		// No answer came back: an attempt whose result is not known.
		p.auditDetailed(r, "database.delete.unknown:"+deletion.Name, "database", deletion.ID, details)
	default:
		p.auditDetailed(r, "database.delete.partial:"+deletion.Name, "database", deletion.ID, details)
	}
}

// writeDatabaseDeleteError answers a deletion that did not finish, in the
// product's error shape: a code the screen has words for, the facts as vars,
// and the in-panel fix only for an administrator. A deletion that changed
// the engine is recorded in the audit log first.
func (p *Panel) writeDatabaseDeleteError(w http.ResponseWriter, r *http.Request, name string, err error) {
	var failed *databaseDeleteError
	if !errors.As(err, &failed) {
		if errors.Is(err, errDatabaseEngineBusy) {
			writeDatabaseEngineBusy(w)
			return
		}
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
	case failed.databaseGone && (len(failed.usersLeft) > 0 || failed.sweepLeft || failed.databaseExists):
		p.auditDatabaseDeletion(r, failed.deletion, "users_left")
		log.Printf("[409][database] %s: %v", errCodeDatabaseDeleteUsersLeft, failed.cause)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		// Per group (review): users still to be removed, and users that stay
		// but still hold a right on the dropped database; one "press Delete
		// again" line.
		rights := map[string]bool{}
		for _, user := range failed.rightsLeft {
			rights[user] = true
		}
		var pending []string
		for _, user := range failed.usersLeft {
			if !rights[user] {
				pending = append(pending, user)
			}
		}
		body := apiErrorBody{
			Error: "The database " + name + " was deleted, but not everything was done for its users (" + strings.Join(failed.usersLeft, ", ") +
				"). It stays in the list until it is; press Delete again to finish.",
			Code: errCodeDatabaseDeleteUsersLeft, PartialSuccess: true, MutationApplied: true,
			Vars: map[string]string{"name": name, "users": strings.Join(failed.usersLeft, ", "),
				"pending": strings.Join(pending, ", "), "rights": strings.Join(failed.rightsLeft, ", ")},
		}
		switch {
		case failed.databaseExists:
			body.Reason = "exists"
		case len(failed.usersLeft) == 0 && failed.sweepUnknown:
			// alpha.134: only the rights naming it on other accounts are left.
			body.Reason = "sweep_unknown"
		case len(failed.usersLeft) == 0:
			body.Reason = "sweep"
		case failed.unknown && len(failed.rightsLeft) == 0:
			body.Reason = "unknown"
		case failed.unknown && len(pending) == 0:
			body.Reason = "unknown_rights"
		case failed.unknown:
			body.Reason = "unknown_mixed"
		case len(pending) == 0:
			body.Reason = "rights"
		case len(failed.rightsLeft) > 0:
			body.Reason = "mixed"
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
		// No answer: whether it ran is not known; the list is read again. The
		// attempt is recorded with its unknown result.
		p.auditDatabaseDeletion(r, failed.deletion, "unknown")
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

// writeDatabaseEngineErrorFor is the one writer of a database engine's
// refusal (R-053), for every handler: an administrator is the one who acts, so their answer carries the reason
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
