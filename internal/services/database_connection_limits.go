package services

import (
	"fmt"
)

// Connection limits of a database user (alpha.128, gap G5,
// docs/SITE-LIMITS.md): one site's user cannot take every connection of the
// database server. The limit is the engine's own, so it keeps working
// without the panel:
//
//	MariaDB     ALTER USER 'u'@'localhost' WITH MAX_USER_CONNECTIONS n   (0: no limit)
//	PostgreSQL  ALTER ROLE "u" CONNECTION LIMIT n                         (-1: no limit)
//
// These are separate functions beside the drivers' other ones; the panel
// calls them through ConnectionLimiter on the driver of the user's own
// server (for PostgreSQL the cluster that runs, as every statement).
//
// Veritabanı kullanıcısının bağlantı sınırı; motorun kendi ayarıdır, panel
// olmadan da çalışır.

// ConnectionLimiter is a driver that caps one user's connections. limit 0
// is "no limit".
type ConnectionLimiter interface {
	SetUserConnectionLimit(username string, limit int) error
}

// MaxUserConnections is the largest limit the panel writes.
const MaxUserConnections = 10000

func validConnectionLimit(limit int) error {
	if limit < 0 || limit > MaxUserConnections {
		return fmt.Errorf("connection limit must be between 0 and %d", MaxUserConnections)
	}
	return nil
}

// mariaDBConnectionLimitSQL is the statement for one user.
func mariaDBConnectionLimitSQL(username string, limit int) (string, error) {
	if err := validConnectionLimit(limit); err != nil {
		return "", err
	}
	if err := ValidateSQLIdentifier(username); err != nil {
		return "", fmt.Errorf("invalid username: %w", err)
	}
	userLiteral, err := QuoteMySQLStringLiteral(username)
	if err != nil {
		return "", fmt.Errorf("invalid username: %w", err)
	}
	return fmt.Sprintf("ALTER USER %s@'localhost' WITH MAX_USER_CONNECTIONS %d;", userLiteral, limit), nil
}

// SetUserConnectionLimit sets a MariaDB user's MAX_USER_CONNECTIONS.
func (d *MariaDBDriver) SetUserConnectionLimit(username string, limit int) error {
	statement, err := mariaDBConnectionLimitSQL(username, limit)
	if err != nil {
		return err
	}
	if err := d.executeSQL(statement); err != nil {
		return fmt.Errorf("set the connection limit of %s: %w", username, err)
	}
	return nil
}

// postgreSQLConnectionLimitSQL is the statement for one role.
func postgreSQLConnectionLimitSQL(username string, limit int) (string, error) {
	if err := validConnectionLimit(limit); err != nil {
		return "", err
	}
	ident, err := QuotePGIdentifier(username)
	if err != nil {
		return "", fmt.Errorf("invalid username: %w", err)
	}
	if limit == 0 {
		limit = -1
	}
	return fmt.Sprintf("ALTER ROLE %s CONNECTION LIMIT %d;", ident, limit), nil
}

// SetUserConnectionLimit sets a PostgreSQL role's CONNECTION LIMIT.
func (d *PostgreSQLDriver) SetUserConnectionLimit(username string, limit int) error {
	statement, err := postgreSQLConnectionLimitSQL(username, limit)
	if err != nil {
		return err
	}
	db, err := d.getDB("postgres")
	if err != nil {
		return fmt.Errorf("open PostgreSQL control database for the connection limit: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		return fmt.Errorf("set the connection limit of %s: %w", username, err)
	}
	return nil
}
