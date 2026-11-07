package main

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgconnLoadExecutor sends a file's statements over one connection.
type pgconnLoadExecutor struct{ conn *pgconn.PgConn }

func (e pgconnLoadExecutor) Exec(ctx context.Context, sql string) error {
	results, err := e.conn.Exec(ctx, sql).ReadAll()
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.Err != nil {
			return result.Err
		}
	}
	return nil
}

func (e pgconnLoadExecutor) CopyFrom(ctx context.Context, sql string, data io.Reader) error {
	_, err := e.conn.CopyFrom(ctx, data, sql)
	return err
}

// connectPGAsDatabaseUser signs in to the cluster on port as the database's
// own role, with its password, over loopback TCP (the distributions' default
// pg_hba.conf accepts a password there; peer authentication on the socket
// would need the role's operating-system account).
func connectPGAsDatabaseUser(ctx context.Context, port int, database, user, password string) (*pgconn.PgConn, error) {
	if port <= 0 {
		port = 5432
	}
	cfg, err := pgconn.ParseConfig("host=127.0.0.1 port=" + strconv.Itoa(port) + " sslmode=disable")
	if err != nil {
		return nil, err
	}
	cfg.User, cfg.Password, cfg.Database = user, password, database
	cfg.ConnectTimeout = 15 * time.Second
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{"application_name": "celikpanel-import"}
	return pgconn.ConnectConfig(ctx, cfg)
}

// pgAuthRefusal tells a refused sign-in apart from other failures.
func pgAuthRefusal(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01":
			return "PostgreSQL refused the password CelikPanel has stored for this database's user; it was probably changed outside the panel", true
		case "28000":
			return "PostgreSQL's access rules (pg_hba.conf) do not let this database's user sign in with a password from 127.0.0.1", true
		}
	}
	return "", false
}

func pgErrorText(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		text := pgErr.Message
		if pgErr.Detail != "" {
			text += " (" + pgErr.Detail + ")"
		}
		return text
	}
	return err.Error()
}
