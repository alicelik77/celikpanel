package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/alicelik/celikpanel/internal/services"
)

// The database user's time and temporary-file limits never cut the panel's
// own loads (S4 R2a, docs/SITE-LIMITS.md section 4). A restore, a copy and
// an import load a file as the database's own user since alpha.141, so the
// user's limits would apply to them:
//
//   - PostgreSQL statement_timeout and idle_in_transaction_session_timeout
//     are user-settable: the load's session sets both to 0 for itself.
//   - MariaDB MAX_STATEMENT_TIME becomes the session's max_statement_time at
//     sign-in, a session variable: the client sets it to 0 first
//     (--init-command).
//   - PostgreSQL temp_file_limit only a superuser changes: before the load,
//     the engine's administrator gives the role a value for that one
//     database (ALTER ROLE ... IN DATABASE ... SET temp_file_limit = -1,
//     which outranks the role's own value at sign-in), and removes it again
//     after the load. Only when the role has a temp_file_limit of its own.
//     A load that never got to remove it (the Agent stopped) leaves it until
//     the next write of the user's limits, which removes every such value
//     (services.PostgreSQLResetLoadOverridesSQL).
//
// Panelin kendi geri yükleme, kopya ve içe aktarmaları kullanıcının süre ve
// geçici dosya sınırlarıyla kesilmez.

// mariadbLoadInitCommand is the client's first statement of a load.
const mariadbLoadInitCommand = "--init-command=SET SESSION max_statement_time=0"

// pgLoadSessionDefaults are the session settings a load sends after sign-in.
var pgLoadSessionDefaults = []string{"SET statement_timeout = 0", "SET idle_in_transaction_session_timeout = 0"}

// pgLoadLiftSessionLimits sends them.
func pgLoadLiftSessionLimits(ctx context.Context, exec pgLoadExecutor) error {
	for _, statement := range pgLoadSessionDefaults {
		if err := exec.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// pgLoadTempFileLiftSQL is the administrator's statement before a load: a
// role-in-database value only when the role has its own temp_file_limit.
func pgLoadTempFileLiftSQL(role, database string) (string, error) {
	roleIdent, err := services.QuotePGIdentifier(role)
	if err != nil {
		return "", err
	}
	databaseIdent, err := services.QuotePGIdentifier(database)
	if err != nil {
		return "", err
	}
	roleLiteral, err := services.QuotePGStringLiteral(role)
	if err != nil {
		return "", err
	}
	return "DO $celikpanel$ BEGIN IF EXISTS (SELECT 1 FROM pg_catalog.pg_db_role_setting s " +
		"JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) s.setrole " +
		"WHERE r.rolname OPERATOR(pg_catalog.=) " + roleLiteral + " AND s.setdatabase OPERATOR(pg_catalog.=) 0 " +
		"AND pg_catalog.array_to_string(s.setconfig, ',') OPERATOR(pg_catalog.~~) '%temp_file_limit=%') THEN " +
		"ALTER ROLE " + roleIdent + " IN DATABASE " + databaseIdent + " SET temp_file_limit = -1; END IF; END $celikpanel$;", nil
}

// pgLoadTempFileRestoreSQL removes the value again.
func pgLoadTempFileRestoreSQL(role, database string) (string, error) {
	roleIdent, err := services.QuotePGIdentifier(role)
	if err != nil {
		return "", err
	}
	databaseIdent, err := services.QuotePGIdentifier(database)
	if err != nil {
		return "", err
	}
	return "ALTER ROLE " + roleIdent + " IN DATABASE " + databaseIdent + " RESET temp_file_limit;", nil
}

// pgLiftLoadTempFileLimit lifts the role's temporary-file limit for one
// database while a load runs and returns what puts it back. A lift that
// fails is logged: the load then runs under the role's own limit and, if
// the limit stops it, says so (nothing is half-loaded: one transaction).
func pgLiftLoadTempFileLimit(port int, role, database string) func() {
	lift, err := pgLoadTempFileLiftSQL(role, database)
	if err != nil {
		return func() {}
	}
	if out, err := postgreSQLExecStatementOnPort(port, lift); err != nil {
		log.Printf("database load: the temporary-file limit of %s could not be lifted for %s: %v: %s", role, database, err, strings.TrimSpace(string(out)))
		return func() {}
	}
	return func() {
		restore, err := pgLoadTempFileRestoreSQL(role, database)
		if err != nil {
			return
		}
		if out, err := postgreSQLExecStatementOnPort(port, restore); err != nil {
			log.Printf("database load: the temporary-file limit of %s for %s stays lifted until the next change of its limits: %v: %s",
				role, database, err, strings.TrimSpace(string(out)))
		}
	}
}

// mariadbLoadArgvWithLimits is the load's client command line with the
// statement time lifted for the load's own session.
func mariadbLoadArgvWithLimits(argv []string) []string {
	if len(argv) < 2 {
		return argv
	}
	// --no-defaults stays the first option; the rest have no order.
	return append(append([]string{}, argv...), mariadbLoadInitCommand)
}

func pgLoadSessionDefaultsError(err error) error {
	return fmt.Errorf("the load's session settings could not be made: %w", err)
}
