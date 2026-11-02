package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"

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
	switch engine {
	case "mysql":
		literal, err := services.QuoteMySQLStringLiteral(username)
		if err != nil {
			return suspensionDatabaseStatements{}, fmt.Errorf("invalid database user name: %w", err)
		}
		// Panel-created MariaDB accounts are 'name'@'localhost'. MariaDB
		// (10.4+) keeps the lock in mysql.global_priv as account_locked.
		// KILL ... USER 'name' (host %) ends every session of that name.
		return suspensionDatabaseStatements{
			inspect: "SELECT CONCAT('state=', CASE WHEN COUNT(*) = 0 THEN 'absent' " +
				"WHEN MAX(JSON_VALUE(Priv, '$.account_locked')) = 'true' THEN 'locked' ELSE 'open' END) " +
				"FROM mysql.global_priv WHERE User = " + literal + " AND Host = 'localhost';",
			lock:       "ALTER USER " + literal + "@'localhost' ACCOUNT LOCK;",
			disconnect: "KILL CONNECTION USER " + literal + ";",
			unlock:     "ALTER USER " + literal + "@'localhost' ACCOUNT UNLOCK;",
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
		return suspensionDatabaseStatements{
			inspect: "SELECT 'state=' || COALESCE((SELECT CASE WHEN rolcanlogin THEN 'open' ELSE 'locked' END " +
				"FROM pg_catalog.pg_roles WHERE rolname = " + literal + "), 'absent') AS celikpanel_state;",
			lock: "ALTER ROLE " + ident + " NOLOGIN;",
			disconnect: "SELECT count(pg_catalog.pg_terminate_backend(pid)) FROM pg_catalog.pg_stat_activity " +
				"WHERE usename = " + literal + ";",
			unlock: "ALTER ROLE " + ident + " LOGIN;",
		}, nil
	}
	return suspensionDatabaseStatements{}, fmt.Errorf("unknown database engine %q; expected mysql or postgresql", engine)
}

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

	// A suspended account's sessions are always ended, also on a retry after
	// the lock was already in place: a session opened before the lock keeps
	// working until it is ended. It is idempotent.
	var disconnectErr error
	if req.Suspend {
		if output, runErr := run(statements.disconnect); runErr != nil {
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
