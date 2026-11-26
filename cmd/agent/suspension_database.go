package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alicelik/celikpanel/internal/dbremote"
	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A suspended database account is locked by the engine itself: MariaDB
// ACCOUNT LOCK, PostgreSQL NOLOGIN. New logins are refused and the account's
// open sessions are ended. Nothing is dropped; restore puts back the lock
// state recorded before suspension, so an account the owner had locked stays
// locked.
//
// Askıya alınmış veritabanı hesabını motor kendisi kilitler (MariaDB ACCOUNT
// LOCK, PostgreSQL NOLOGIN); açık oturumlar kapatılır, hiçbir şey silinmez.

// suspensionDatabaseMutex keeps one database-account compare-and-set at a time.
var suspensionDatabaseMutex sync.Mutex

var errSuspensionDatabaseUserAbsent = errors.New("database user is absent")

// suspensionDatabaseStatements are the exact statements for one account.
type suspensionDatabaseStatements struct {
	inspect    string
	lock       string
	disconnect string
	unlock     string
	// copies lists the hosts of the user's other accounts (MariaDB): the
	// remote copies of remote database access, locked and unlocked with the
	// local account (alpha.129 review M1). Empty for PostgreSQL, whose one
	// role is the remote access too.
	copies string
}

// suspensionReservedDatabaseUsers are engine and CelikPanel accounts that
// suspension never locks, whatever the request says.
func suspensionReservedDatabaseUser(username string) bool {
	lowered := strings.ToLower(username)
	switch lowered {
	case "root", "mysql", "postgres", strings.ToLower(databaseAdminAccountName):
		return true
	}
	return strings.HasPrefix(lowered, "pg_")
}

// suspensionDatabaseSQL composes the statements for one account. The name is
// validated as a plain identifier first, then quoted by the engine's helper.
func suspensionDatabaseSQL(engine, username string) (suspensionDatabaseStatements, error) {
	if err := services.ValidateSQLIdentifier(username); err != nil {
		return suspensionDatabaseStatements{}, fmt.Errorf("invalid database user name: %w", err)
	}
	if suspensionReservedDatabaseUser(username) {
		return suspensionDatabaseStatements{}, fmt.Errorf("database user %s is an engine or CelikPanel account and is never suspended", username)
	}
	// alpha.149 (security, review of PR #370 finding 1): the inspection also
	// reads whether the account is privileged ("state=privileged",
	// database_account_guard.go), and every write runs behind the same test
	// in the same transaction or compound statement, so a lock, an unlock or
	// the end of the sessions never reaches an administrator account.
	// Yönetici hesabına kilit, kilit açma ya da oturum kapatma uygulanmaz.
	switch engine {
	case "mysql":
		literal, err := services.QuoteMySQLStringLiteral(username)
		if err != nil {
			return suspensionDatabaseStatements{}, fmt.Errorf("invalid database user name: %w", err)
		}
		guarded := func(body string) (string, error) { return mysqlGuarded(username, false, body) }
		lock, err := guarded("ALTER USER " + literal + "@'localhost' ACCOUNT LOCK;")
		if err != nil {
			return suspensionDatabaseStatements{}, err
		}
		unlock, _ := guarded("ALTER USER " + literal + "@'localhost' ACCOUNT UNLOCK;")
		disconnect, _ := guarded("KILL CONNECTION USER " + literal + ";")
		// Panel-created MariaDB accounts are 'name'@'localhost'. MariaDB
		// (10.4+) keeps the lock in mysql.global_priv as account_locked;
		// JSON_VALUE answers 'true' on some versions and '1' on others
		// (11.8 on Debian 13, lab run 1 of alpha.117), so both mean locked.
		// KILL ... USER 'name' (host %) ends every session of that name.
		return suspensionDatabaseStatements{
			inspect: "SELECT CONCAT('state=', CASE WHEN " + mysqlAccountAbsentExpr(literal, false) + " THEN 'absent' " +
				"WHEN " + mysqlAccountPrivilegedExpr(literal) + " THEN 'privileged' " +
				"WHEN (SELECT MAX(JSON_VALUE(Priv, '$.account_locked')) FROM mysql.global_priv WHERE User = " + literal + " AND Host = 'localhost') IN ('true', '1') THEN 'locked' ELSE 'open' END);",
			lock:       lock,
			disconnect: disconnect,
			unlock:     unlock,
			copies:     "SELECT Host FROM mysql.user WHERE User = " + literal + " AND Host <> 'localhost';",
		}, nil
	case "postgresql":
		ident, err := services.QuotePGIdentifier(username)
		if err != nil {
			return suspensionDatabaseStatements{}, fmt.Errorf("invalid database user name: %w", err)
		}
		literal, err := services.QuotePGStringLiteral(username)
		if err != nil {
			return suspensionDatabaseStatements{}, fmt.Errorf("invalid database user name: %w", err)
		}
		guarded := func(body string) string {
			statement, _ := pgGuarded(username, pgAccountOrdinary, body)
			return statement
		}
		return suspensionDatabaseStatements{
			inspect: "SELECT 'state=' || COALESCE((SELECT CASE WHEN " + pgAccountPrivilegedExpr("celikpanel_role") + " THEN 'privileged' " +
				"WHEN celikpanel_role.rolcanlogin THEN 'open' ELSE 'locked' END " +
				"FROM pg_catalog.pg_roles celikpanel_role WHERE celikpanel_role.rolname = " + literal + "), 'absent') AS celikpanel_state;",
			lock: guarded("ALTER ROLE " + ident + " NOLOGIN;"),
			disconnect: guarded("SELECT count(pg_catalog.pg_terminate_backend(pid)) FROM pg_catalog.pg_stat_activity " +
				"WHERE usename = " + literal + ";"),
			unlock: guarded("ALTER ROLE " + ident + " LOGIN;"),
		}, nil
	}
	return suspensionDatabaseStatements{}, fmt.Errorf("unknown database engine %q; expected mysql or postgresql", engine)
}

// errSuspensionDatabaseUserPrivileged: the inspection found an administrator
// account; nothing is changed.
var errSuspensionDatabaseUserPrivileged = errors.New("database user is privileged")

// suspensionDatabaseRunner is the existing superuser statement runner of the
// engine: mysql as root over the socket, psql as postgres; both on stdin.
func suspensionDatabaseRunner(target transport.SuspensionDatabaseUser) (func(string) ([]byte, error), error) {
	switch target.Engine {
	case "mysql":
		return mysqlExecStatement, nil
	case "postgresql":
		return postgreSQLStatementFor(target.Port)
	}
	return nil, fmt.Errorf("unknown database engine %q; expected mysql or postgresql", target.Engine)
}

// parseSuspensionDatabaseState reads the one "state=" marker line of the
// inspection statement's output (MariaDB batch output or psql's aligned table).
func parseSuspensionDatabaseState(output []byte) (transport.DatabaseUserState, error) {
	value := ""
	markers := 0
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "state=") {
			continue
		}
		markers++
		value = strings.TrimPrefix(trimmed, "state=")
	}
	if markers != 1 {
		return transport.DatabaseUserState{}, errors.New("the database engine's answer about the account could not be read")
	}
	switch value {
	case "absent":
		return transport.DatabaseUserState{}, nil
	case "open":
		return transport.DatabaseUserState{Present: true}, nil
	case "locked":
		return transport.DatabaseUserState{Present: true, Locked: true}, nil
	case transport.DatabaseAccountRefusedPrivileged:
		return transport.DatabaseUserState{}, errSuspensionDatabaseUserPrivileged
	}
	return transport.DatabaseUserState{}, errors.New("the database engine's answer about the account could not be read")
}

func readSuspensionDatabaseUserState(
	run func(string) ([]byte, error),
	statements suspensionDatabaseStatements,
) (transport.DatabaseUserState, error) {
	output, err := run(statements.inspect)
	if err != nil {
		return transport.DatabaseUserState{}, hostcmd.Fail("the database account could not be inspected", output, err, databaseClientMeaning)
	}
	return parseSuspensionDatabaseState(output)
}

// databaseAccountWriteRefusal reads a guard's refusal from a write's output
// (the account changed between the inspection and the write): the code, ""
// when the write failed for another reason.
func databaseAccountWriteRefusal(output []byte) string {
	return databaseAccountRefusalIn(output)
}

// suspensionAccountRefused fills a refusal of the account into the answer's
// code and sentence; true when err is one.
func suspensionAccountRefused(err error, username string, code *string, message *string) bool {
	switch {
	case errors.Is(err, errSuspensionDatabaseUserPrivileged):
		*code = transport.DatabaseAccountRefusedPrivileged
	case databaseAccountRefusalOf(err) != "":
		*code = databaseAccountRefusalOf(err)
	default:
		return false
	}
	*message = databaseAccountRefusalText(username, *code)
	return true
}

func prepareSuspensionDatabaseUser(target transport.SuspensionDatabaseUser) (func(string) ([]byte, error), suspensionDatabaseStatements, error) {
	statements, err := suspensionDatabaseSQL(target.Engine, target.Username)
	if err != nil {
		return nil, suspensionDatabaseStatements{}, err
	}
	run, err := suspensionDatabaseRunner(target)
	if err != nil {
		return nil, suspensionDatabaseStatements{}, err
	}
	return run, statements, nil
}

func inspectSuspensionDatabaseUser(target transport.SuspensionDatabaseUser) (transport.DatabaseUserState, error) {
	run, statements, err := prepareSuspensionDatabaseUser(target)
	if err != nil {
		return transport.DatabaseUserState{}, err
	}
	return readSuspensionDatabaseUserState(run, statements)
}

func describeDatabaseLock(state transport.DatabaseUserState) string {
	if state.Locked {
		return "locked"
	}
	return "open"
}

// SetDatabaseUserSuspensionV1 locks (and disconnects) or restores one
// database account on the local engine.
//
// SetDatabaseUserSuspensionV1 yerel motordaki bir veritabanı hesabını
// kilitler (ve oturumlarını kapatır) ya da eski durumuna getirir.
func (a *Agent) SetDatabaseUserSuspensionV1(
	req *transport.SetDatabaseUserSuspensionRequest,
	resp *transport.SetDatabaseUserSuspensionResponse,
) error {
	if req == nil || resp == nil {
		return fmt.Errorf("database user suspension request and response are required")
	}
	*resp = transport.SetDatabaseUserSuspensionResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "suspending or restoring a database user"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	run, statements, err := prepareSuspensionDatabaseUser(req.User)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}

	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()

	current, err := readSuspensionDatabaseUserState(run, statements)
	if suspensionAccountRefused(err, req.User.Username, &resp.AccountRefused, &resp.Error) {
		return nil
	}
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	resp.State = current
	if !current.Present {
		resp.Error = fmt.Sprintf("database user is absent: %s does not exist on this server's %s engine, so nothing was changed", req.User.Username, req.User.Engine)
		return nil
	}
	if !req.Before.Present {
		resp.Conflict = true
		resp.Error = fmt.Sprintf(
			"database user %s exists, but the state recorded before suspension says it did not; nothing was changed. "+
				"CelikPanel inspects the account again before retrying",
			req.User.Username,
		)
		return nil
	}

	suspended := transport.DatabaseUserState{Present: true, Locked: true}
	target, from := suspended, req.Before
	if !req.Suspend {
		target, from = req.Before, suspended
	}
	step := suspensionDecide(current, target, from)
	if step == suspensionConflict {
		resp.Conflict = true
		resp.Error = fmt.Sprintf(
			"database user %s is %s, which is neither the state recorded before suspension (%s) nor the suspended state (locked); "+
				"it was changed outside CelikPanel, so nothing was changed. The server owner decides which state is right, "+
				"then the operation can be retried",
			req.User.Username, describeDatabaseLock(current), describeDatabaseLock(req.Before),
		)
		return nil
	}

	if step == suspensionChange {
		statement := statements.lock
		if !target.Locked {
			statement = statements.unlock
		}
		output, runErr := run(statement)
		if code := databaseAccountWriteRefusal(output); runErr != nil && code != "" {
			// The account changed after the inspection: the guard in the
			// same statement refused, so nothing was changed.
			resp.AccountRefused, resp.Error = code, databaseAccountRefusalText(req.User.Username, code)
			return nil
		}
		if runErr != nil {
			resp.Error = hostcmd.Fail("the database account's lock could not be changed", output, runErr, databaseClientMeaning).Error()
			if after, readErr := readSuspensionDatabaseUserState(run, statements); readErr == nil {
				resp.State = after
				resp.Changed = after != current
			}
			return nil
		}
		resp.Changed = true
	}

	// The remote copies follow the local account's lock, also on a retry,
	// before the sessions are ended so none comes back through a copy.
	if err := lockSuspensionRemoteCopies(run, statements, req.User.Username, target.Locked); err != nil {
		resp.AccountRefused = databaseAccountRefusalOf(err)
		resp.Error = err.Error()
		return nil
	}

	// A suspended account's sessions are always ended, also on a retry after
	// the lock was already in place: a session opened before the lock keeps
	// working until it is ended. It is idempotent.
	var disconnectErr error
	if req.Suspend {
		if output, runErr := run(statements.disconnect); runErr != nil {
			if code := databaseAccountWriteRefusal(output); code != "" {
				resp.AccountRefused, resp.Error = code, databaseAccountRefusalText(req.User.Username, code)
				return nil
			}
			disconnectErr = hostcmd.Fail("the database account is locked but its open sessions could not be ended", output, runErr, databaseClientMeaning)
		}
	}

	after, err := readSuspensionDatabaseUserState(run, statements)
	if err != nil {
		resp.Error = "the change finished but the database account could not be read back, so the result is unknown: " + err.Error()
		return nil
	}
	resp.State = after
	if after != target {
		resp.Error = fmt.Sprintf(
			"database user %s reads %s after the change instead of %s; the result is not the wanted state",
			req.User.Username, describeDatabaseLock(after), describeDatabaseLock(target),
		)
		return nil
	}
	if disconnectErr != nil {
		resp.Error = disconnectErr.Error() + "; retrying ends them"
		return nil
	}
	resp.Applied = true
	return nil
}

// lockSuspensionRemoteCopies locks or unlocks every remote copy of a MariaDB
// user (an account of the same name whose host is an address or an IPv4
// network, the forms remote database access writes); other hosts are never
// touched.
func lockSuspensionRemoteCopies(run func(string) ([]byte, error), statements suspensionDatabaseStatements, username string, locked bool) error {
	if statements.copies == "" {
		return nil
	}
	output, err := run(statements.copies)
	if err != nil {
		return hostcmd.Fail("the remote copies of the database account could not be listed", output, err, databaseClientMeaning)
	}
	literal, err := services.QuoteMySQLStringLiteral(username)
	if err != nil {
		return err
	}
	action := "ACCOUNT UNLOCK"
	if locked {
		action = "ACCOUNT LOCK"
	}
	var batch []string
	for _, line := range strings.Split(string(output), "\n") {
		host := strings.TrimSpace(line)
		if host == "" || !dbremote.IsRemoteAccountHost(host) {
			continue
		}
		hostLiteral, err := services.QuoteMySQLStringLiteral(host)
		if err != nil {
			continue
		}
		batch = append(batch, "ALTER USER "+literal+"@"+hostLiteral+" "+action+";")
	}
	if len(batch) == 0 {
		return nil
	}
	// alpha.149 (security): the copies share the account's name, so they
	// change only behind the same guard as the local account.
	statement, err := mysqlGuarded(username, false, strings.Join(batch, "\n"))
	if err != nil {
		return err
	}
	if output, err := run(statement); err != nil {
		if code := databaseAccountWriteRefusal(output); code != "" {
			return &errDatabaseAccountRefused{code: code, username: username}
		}
		verb := "unlocked"
		if locked {
			verb = "locked"
		}
		return hostcmd.Fail("the database account's remote copies could not be "+verb, output, err, databaseClientMeaning)
	}
	return nil
}
