package services

import (
	"fmt"
	"math"
	"strconv"
	"strings"
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
// The panel never resets what it did not set (review finding 2): it reads a
// user's current values (ReadUserRoleSettings), writes ONE key at a time
// (SetUserRoleSetting) and only where the value is still the one it wrote
// itself; the Panel keeps that record (cmd/panel/database_role_limits.go).
// Role settings for one database are never written or reset by the panel.
//
// Veritabanı kullanıcısının süre ve geçici dosya sınırları; motorun kendi rol
// ayarlarıdır. Panel yalnız kendi yazdığı değeri değiştirir; sahibin elle
// koyduğu değere dokunmaz.

// RoleLimits are one user's limits; 0 is "none".
type RoleLimits struct {
	StatementSeconds         int
	IdleInTransactionSeconds int
	TempFileMB               int
}

// The role-limit keys (the Panel's setting keys without "db.").
const (
	RoleKeyStatementSeconds         = "statement_seconds"
	RoleKeyIdleInTransactionSeconds = "idle_in_transaction_seconds"
	RoleKeyTempFileMB               = "temp_file_mb"
)

// RoleKeys are the keys in their order.
var RoleKeys = []string{RoleKeyStatementSeconds, RoleKeyIdleInTransactionSeconds, RoleKeyTempFileMB}

// Value is one key's value.
func (l RoleLimits) Value(key string) int {
	switch key {
	case RoleKeyStatementSeconds:
		return l.StatementSeconds
	case RoleKeyIdleInTransactionSeconds:
		return l.IdleInTransactionSeconds
	case RoleKeyTempFileMB:
		return l.TempFileMB
	}
	return 0
}

// RoleSettings are a user's current values as the engine stores them, per
// key ("" or absent: not set). Only the keys the engine has are present.
type RoleSettings map[string]string

// RoleLimiter is a driver that reads and writes a user's time and
// temporary-file limits, one key at a time.
type RoleLimiter interface {
	// RoleKeysSupported are the keys this engine has.
	RoleKeysSupported() []string
	ReadUserRoleSettings(username string) (RoleSettings, error)
	// SetUserRoleSetting writes one key (0 removes it).
	SetUserRoleSetting(username, key string, value int) error
}

// Bounds of the values the panel writes.
const (
	MaxRoleLimitSeconds = 86400
	MaxRoleTempFileMB   = 1048576
)

func validRoleValue(key string, value int) error {
	switch key {
	case RoleKeyStatementSeconds, RoleKeyIdleInTransactionSeconds:
		if value < 0 || value > MaxRoleLimitSeconds {
			return fmt.Errorf("time limits must be 0 to %d seconds", MaxRoleLimitSeconds)
		}
	case RoleKeyTempFileMB:
		if value < 0 || value > MaxRoleTempFileMB {
			return fmt.Errorf("the temporary-file limit must be 0 to %d MB", MaxRoleTempFileMB)
		}
	default:
		return fmt.Errorf("unknown limit %q", key)
	}
	return nil
}

// --- MariaDB ------------------------------------------------------------------

// mariaDBStatementTimeSQL is the statement for one account.
func mariaDBStatementTimeSQL(account string, seconds int) (string, error) {
	if err := validRoleValue(RoleKeyStatementSeconds, seconds); err != nil {
		return "", err
	}
	return fmt.Sprintf("ALTER USER %s WITH MAX_STATEMENT_TIME %d;", account, seconds), nil
}

// RoleKeysSupported: MariaDB has the statement time only.
func (d *MariaDBDriver) RoleKeysSupported() []string { return []string{RoleKeyStatementSeconds} }

// mariaDBStatementTimeValue reads the client's batch output of
// SELECT max_statement_time (a header line, then the value): "" for none (0).
func mariaDBStatementTimeValue(output string) (string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return "", fmt.Errorf("the account was not found")
	}
	raw := strings.TrimSpace(lines[1])
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return "", fmt.Errorf("max_statement_time %q could not be read", raw)
	}
	if f == 0 {
		return "", nil
	}
	return strconv.FormatFloat(math.Round(f*1000)/1000, 'f', -1, 64) + "s", nil
}

// ReadUserRoleSettings reads the account's MAX_STATEMENT_TIME.
func (d *MariaDBDriver) ReadUserRoleSettings(username string) (RoleSettings, error) {
	if err := ValidateSQLIdentifier(username); err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	out, err := d.runSQL("SELECT max_statement_time FROM mysql.user WHERE User = " + userLiteral + " AND Host = 'localhost';")
	if err != nil {
		return nil, fmt.Errorf("read the statement time of %s: %w", username, err)
	}
	value, err := mariaDBStatementTimeValue(string(out))
	if err != nil {
		return nil, fmt.Errorf("read the statement time of %s: %w", username, err)
	}
	return RoleSettings{RoleKeyStatementSeconds: value}, nil
}

// SetUserRoleSetting sets a MariaDB user's MAX_STATEMENT_TIME, and its
// remote copies'. Other keys do not exist on MariaDB: nothing is written.
func (d *MariaDBDriver) SetUserRoleSetting(username, key string, value int) error {
	if key != RoleKeyStatementSeconds {
		return nil
	}
	if err := ValidateSQLIdentifier(username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}
	statement, err := mariaDBStatementTimeSQL(userLiteral+"@'localhost'", value)
	if err != nil {
		return err
	}
	if err := d.executeSQL(statement); err != nil {
		return fmt.Errorf("set the statement time limit of %s: %w", username, err)
	}
	if err := d.forEachRemoteCopy(userLiteral, func(account string) string {
		text, _ := mariaDBStatementTimeSQL(account, value)
		return text
	}); err != nil {
		return fmt.Errorf("set the statement time limit of %s's remote copies: %w", username, err)
	}
	return nil
}

// --- PostgreSQL ---------------------------------------------------------------

// postgreSQLRoleParameter is the role parameter of a key and its unit.
func postgreSQLRoleParameter(key string) (string, string) {
	switch key {
	case RoleKeyStatementSeconds:
		return "statement_timeout", "s"
	case RoleKeyIdleInTransactionSeconds:
		return "idle_in_transaction_session_timeout", "s"
	case RoleKeyTempFileMB:
		return "temp_file_limit", "MB"
	}
	return "", ""
}

// postgreSQLRoleSettingSQL is the statement for one key of one role.
func postgreSQLRoleSettingSQL(username, key string, value int) (string, error) {
	if err := validRoleValue(key, value); err != nil {
		return "", err
	}
	ident, err := QuotePGIdentifier(username)
	if err != nil {
		return "", fmt.Errorf("invalid username: %w", err)
	}
	name, unit := postgreSQLRoleParameter(key)
	if value == 0 {
		return fmt.Sprintf("ALTER ROLE %s RESET %s", ident, name), nil
	}
	return fmt.Sprintf("ALTER ROLE %s SET %s = '%d%s'", ident, name, value, unit), nil
}

// postgreSQLRoleSettingsFrom reads the role-wide setconfig entries
// ("name=value") into the keys.
func postgreSQLRoleSettingsFrom(entries []string) RoleSettings {
	out := RoleSettings{}
	for _, key := range RoleKeys {
		out[key] = ""
	}
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		for _, key := range RoleKeys {
			if parameter, _ := postgreSQLRoleParameter(key); parameter == name {
				out[key] = value
			}
		}
	}
	return out
}

// RoleKeysSupported: PostgreSQL has all three.
func (d *PostgreSQLDriver) RoleKeysSupported() []string { return RoleKeys }

// ReadUserRoleSettings reads the role's own settings (for every database,
// setdatabase 0); a role's settings for one database are not read or written.
func (d *PostgreSQLDriver) ReadUserRoleSettings(username string) (RoleSettings, error) {
	if _, err := QuotePGIdentifier(username); err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	db, err := d.getDB("postgres")
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL control database for the time limits: %w", err)
	}
	defer db.Close()
	var config string
	err = db.QueryRow(`SELECT COALESCE(pg_catalog.array_to_string(s.setconfig, E'\n'), '') FROM pg_catalog.pg_roles r `+
		`LEFT JOIN pg_catalog.pg_db_role_setting s ON s.setrole OPERATOR(pg_catalog.=) r.oid AND s.setdatabase OPERATOR(pg_catalog.=) 0 `+
		`WHERE r.rolname OPERATOR(pg_catalog.=) $1`, username).Scan(&config)
	if err != nil {
		return nil, fmt.Errorf("read the time and temporary-file limits of %s: %w", username, err)
	}
	return postgreSQLRoleSettingsFrom(strings.Split(config, "\n")), nil
}

// SetUserRoleSetting writes one key of a PostgreSQL role.
func (d *PostgreSQLDriver) SetUserRoleSetting(username, key string, value int) error {
	statement, err := postgreSQLRoleSettingSQL(username, key, value)
	if err != nil {
		return err
	}
	db, err := d.getDB("postgres")
	if err != nil {
		return fmt.Errorf("open PostgreSQL control database for the time limits: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		return fmt.Errorf("set the %s of %s: %w", key, username, err)
	}
	return nil
}
