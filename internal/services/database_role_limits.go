package services

import (
	"fmt"
)

// Time and temporary-file limits of a database user (S4 R2a,
// docs/SITE-LIMITS.md section 4). Off unless a plan, a reseller or the
// server sets them. The values are the engines' own role settings, so they
// keep working without the panel:
//
//	MariaDB     ALTER USER 'u'@'localhost' WITH MAX_STATEMENT_TIME n     (0: none)
//	            The server copies the account's value into the session's
//	            max_statement_time at sign-in (acl_authenticate, sql_acl.cc);
//	            max_statement_time is a session variable, so a session can
//	            change it: a default, not a limit.
//	PostgreSQL  ALTER ROLE "u" SET statement_timeout = 'n s'            (RESET: none)
//	            ALTER ROLE "u" SET idle_in_transaction_session_timeout   (RESET: none)
//	            Both are user-settable ("the session default", ALTER ROLE):
//	            defaults, not limits.
//	            ALTER ROLE "u" SET temp_file_limit = 'n MB'              (RESET: none)
//	            Only superusers and roles granted SET on it can change it
//	            (runtime-config-resource): a real limit.
//
// The panel's own work is never cut by them: dumps run as the engine's
// administrator, whose sessions do not read a customer role's settings; a
// load of a file as the database's owner (restore, copy, import) sets the
// two session values to 0 for itself, and lifts temp_file_limit for that
// database only while it runs (cmd/agent/database_owner_load.go).
//
// Veritabanı kullanıcısının süre ve geçici dosya sınırları; motorun kendi rol
// ayarlarıdır. Panelin kendi yedek, geri yükleme ve kopyaları kesilmez.

// RoleLimits are one user's limits; 0 is "none".
type RoleLimits struct {
	StatementSeconds         int
	IdleInTransactionSeconds int
	TempFileMB               int
}

// RoleLimiter is a driver that writes a user's time and temporary-file
// limits. Every call writes all three (a 0 removes an earlier one).
type RoleLimiter interface {
	SetUserRoleLimits(username string, limits RoleLimits) error
}

// Bounds of the values the panel writes.
const (
	MaxRoleLimitSeconds = 86400
	MaxRoleTempFileMB   = 1048576
)

func (l RoleLimits) validate() error {
	if l.StatementSeconds < 0 || l.StatementSeconds > MaxRoleLimitSeconds ||
		l.IdleInTransactionSeconds < 0 || l.IdleInTransactionSeconds > MaxRoleLimitSeconds ||
		l.TempFileMB < 0 || l.TempFileMB > MaxRoleTempFileMB {
		return fmt.Errorf("time limits must be 0 to %d seconds and the temporary-file limit 0 to %d MB", MaxRoleLimitSeconds, MaxRoleTempFileMB)
	}
	return nil
}

// mariaDBRoleLimitSQL is the statement for one user (MariaDB has the
// statement time only; the other two are PostgreSQL's).
func mariaDBRoleLimitSQL(account string, limits RoleLimits) (string, error) {
	if err := limits.validate(); err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER USER %s WITH MAX_STATEMENT_TIME %d;", account, limits.StatementSeconds), nil
}

// SetUserRoleLimits sets a MariaDB user's MAX_STATEMENT_TIME, and its
// remote copies'.
func (d *MariaDBDriver) SetUserRoleLimits(username string, limits RoleLimits) error {
	if err := ValidateSQLIdentifier(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	statement, err := mariaDBRoleLimitSQL(userLiteral+"@'localhost'", limits)
	if err != nil {
		return err
	}
	if err := d.executeSQL(statement); err != nil {
		return fmt.Errorf("set the statement time limit of %s: %w", username, err)
	}
	if err := d.forEachRemoteCopy(userLiteral, func(account string) string {
		text, _ := mariaDBRoleLimitSQL(account, limits)
		return text
	}); err != nil {
		return fmt.Errorf("set the statement time limit of %s's remote copies: %w", username, err)
	}
	return nil
}

// postgreSQLRoleLimitSQL is the statements for one role, in order. The
// role-in-database overrides of temp_file_limit a load leaves only while it
// runs (a crashed load could leave one) are removed too, so the role's own
// value holds everywhere after every write.
func postgreSQLRoleLimitSQL(username string, limits RoleLimits) ([]string, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	ident, err := QuotePGIdentifier(username)
	if err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	literal, err := QuotePGStringLiteral(username)
	if err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	set := func(name string, value int, unit string) string {
		if value == 0 {
			return fmt.Sprintf("ALTER ROLE %s RESET %s", ident, name)
		}
		return fmt.Sprintf("ALTER ROLE %s SET %s = '%d%s'", ident, name, value, unit)
	}
	return []string{
		set("statement_timeout", limits.StatementSeconds, "s"),
		set("idle_in_transaction_session_timeout", limits.IdleInTransactionSeconds, "s"),
		set("temp_file_limit", limits.TempFileMB, "MB"),
		PostgreSQLResetLoadOverridesSQL(literal),
	}, nil
}

// PostgreSQLResetLoadOverridesSQL removes every role-in-database
// temp_file_limit of the role named by the quoted literal (what a load lifts
// while it runs).
func PostgreSQLResetLoadOverridesSQL(roleLiteral string) string {
	return "DO $celikpanel$ DECLARE d pg_catalog.text; BEGIN " +
		"FOR d IN SELECT db.datname::pg_catalog.text FROM pg_catalog.pg_db_role_setting s " +
		"JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) s.setrole " +
		"JOIN pg_catalog.pg_database db ON db.oid OPERATOR(pg_catalog.=) s.setdatabase " +
		"WHERE r.rolname OPERATOR(pg_catalog.=) " + roleLiteral +
		" AND pg_catalog.array_to_string(s.setconfig, ',') OPERATOR(pg_catalog.~~) '%temp_file_limit=%' " +
		"LOOP EXECUTE pg_catalog.format('ALTER ROLE %I IN DATABASE %I RESET temp_file_limit', " + roleLiteral + ", d); END LOOP; " +
		"END $celikpanel$"
}

// SetUserRoleLimits sets a PostgreSQL role's time and temporary-file
// limits.
func (d *PostgreSQLDriver) SetUserRoleLimits(username string, limits RoleLimits) error {
	statements, err := postgreSQLRoleLimitSQL(username, limits)
	if err != nil {
		return err
	}
	db, err := d.getDB("postgres")
	if err != nil {
		return fmt.Errorf("open PostgreSQL control database for the time limits: %w", err)
	}
	defer db.Close()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("set the time and temporary-file limits of %s: %w", username, err)
		}
	}
	return nil
}
