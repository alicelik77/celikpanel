package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// The database user's time limits do not cut the panel's own loads, and its
// temporary-file limit is never lifted (S4 R2a, docs/SITE-LIMITS.md section
// 4). A restore, a copy and an import load a file as the database's own user
// since alpha.141, so the user's settings apply to them:
//
//   - PostgreSQL statement_timeout and idle_in_transaction_session_timeout
//     are user-settable defaults: the load's own session sets both to 0 for
//     itself (as the user could), on every load path - restore, database
//     copy, site copy, WordPress copy, cPanel import and the customer's own
//     import.
//   - MariaDB MAX_STATEMENT_TIME becomes the session's max_statement_time at
//     sign-in, a session variable: the load's client sets it to 0 first
//     (--init-command).
//   - PostgreSQL temp_file_limit is a real limit (only a superuser changes
//     it) and stays the user's limit during every load: nothing ever raises
//     it for a role the customer signs in with. A load that needs more
//     temporary space stops, is rolled back whole (one transaction) and says
//     which limit stopped it and who can raise it (pgLoadTempFileLimitError).
//     So an Agent stopped in the middle of a load leaves nothing lifted:
//     there is nothing to lift.
//
// Panelin kendi geri yükleme, kopya ve içe aktarmaları kullanıcının süre
// varsayılanlarıyla kesilmez; geçici dosya sınırı hiçbir zaman kaldırılmaz,
// yetmezse yükleme bütünüyle geri alınır ve hangi sınırın durdurduğunu söyler.

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

// pgLoadTempFileLimit reads the session's temporary-file limit as the server
// shows it ("-1" or "" when there is none or it cannot be read).
func pgLoadTempFileLimit(ctx context.Context, conn pgLoadConn) string {
	rows, err := conn.QueryValues(ctx, "SHOW temp_file_limit")
	if err != nil || len(rows) != 1 || len(rows[0]) != 1 {
		return ""
	}
	return strings.TrimSpace(rows[0][0])
}

// pgTempFileLimitReached: the error is PostgreSQL's refusal at
// temp_file_limit (SQLSTATE 53400, "temporary file size exceeds
// temp_file_limit").
func pgTempFileLimitReached(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "53400" && strings.Contains(pgErr.Message, "temp_file_limit")
	}
	return err != nil && strings.Contains(err.Error(), "exceeds temp_file_limit")
}

// pgLoadTempFileLimitText is the sentence for a load the user's
// temporary-file limit stopped: what happened, that nothing changed, who can
// raise the limit and how the load resumes.
func pgLoadTempFileLimitText(user, limit string) string {
	value := ""
	if limit != "" && limit != "-1" {
		value = " (" + limit + ")"
	}
	return fmt.Sprintf("the load needed more temporary space than the database user %s may use%s: the hosting plan's "+
		"\"Temporary files of one database session\" limit. Nothing was changed. The administrator, or the reseller within "+
		"their own limits, raises it in the plan; then start the load again", user, value)
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
