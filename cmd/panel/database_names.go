package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/services"
)

// Database and user names are unique per engine, not per account
// (alpha.129). Every subscription has its own record of the same local
// engine (database_servers is unique per subscription, host and port), and
// the names two schemes make can meet on that engine: a domain database is
// <domain with . and - as _>_<name> (a.b and a-b give the same prefix; the
// domain s5.com gives s5_com_...), an account-level one s<subscription>_<name>.
// Before this, the account-level create took over a database or user that
// was already there ("IF NOT EXISTS", or an existing PostgreSQL role
// returned as success) and granted its new user on it; the domain create
// checked only its own record and stopped at the Agent with an unexplained
// error. Now both look at every record of the same engine first and refuse
// with the reason, and the drivers create exclusively. Two databases of
// different engines may share a name (owner, 7 Nov 2026): the engine is part
// of the comparison.
//
// Ad tekliği motor başınadır, hesap başına değil: aynı motorun her kaydına
// bakılır; farklı motorlar aynı adı taşıyabilir.

const errCodeDatabaseNameTaken = "DATABASE_NAME_TAKEN"

// databaseNameTakenOnEngine answers which of the names is already recorded on
// the engine of serverID, under any account: "database", "user" or "".
// userMayBeOwn: a user recorded on serverID itself is not "taken" (the domain
// create reuses its own subscription's user of that name).
func (p *Panel) databaseNameTakenOnEngine(ctx context.Context, serverID int, database, user string, userMayBeOwn bool) (string, error) {
	const sameEngine = `
		JOIN database_servers mine ON mine.id = ? AND mine.type_id = ds.type_id AND mine.port = ds.port
		 AND (CASE WHEN mine.host IN ('localhost', '127.0.0.1', '::1') THEN '' ELSE mine.host END)
		   = (CASE WHEN ds.host IN ('localhost', '127.0.0.1', '::1') THEN '' ELSE ds.host END)`
	ownUser := ""
	if userMayBeOwn {
		ownUser = ` AND u.server_id <> mine.id`
	}
	checks := []struct{ what, name, query string }{
		{"database", database, `SELECT EXISTS(SELECT 1 FROM databases_v2 d JOIN database_servers ds ON ds.id = d.server_id` + sameEngine + ` WHERE d.name = ?)`},
		{"user", user, `SELECT EXISTS(SELECT 1 FROM database_users u JOIN database_servers ds ON ds.id = u.server_id` + sameEngine + ` WHERE u.username = ?` + ownUser + `)`},
	}
	for _, check := range checks {
		if check.name == "" {
			continue
		}
		var taken bool
		if err := p.db.GetDB().QueryRowContext(ctx, check.query, serverID, check.name).Scan(&taken); err != nil {
			return "", err
		}
		if taken {
			return check.what, nil
		}
	}
	return "", nil
}

// writeDatabaseNameTaken is the refusal: what is taken, that nothing was
// changed, and what to do.
func writeDatabaseNameTaken(w http.ResponseWriter, what, name string) {
	message := "A database named " + name + " already exists on this database server (it may belong to another account, or have been made outside CelikPanel). Choose another name; nothing was changed."
	if what == "user" {
		message = "A database user named " + name + " already exists on this database server (it may belong to another account, or have been made outside CelikPanel). Choose another name; nothing was changed."
	}
	writeCodedError(w, http.StatusConflict, errCodeDatabaseNameTaken, message, "")
}

// errCodeDatabaseNameTooLong (alpha.134): on MariaDB a right is written by the
// database's exact name, each `_` with a backslash before it; for this name
// that is longer than the 64 characters MariaDB stores for a right, which
// would keep it cut short - a right on another name. Nothing was changed.
const errCodeDatabaseNameTooLong = "DATABASE_NAME_TOO_LONG"

// writeDatabaseNameTooLong is that refusal: what to do (a shorter name) and
// by how much, counted as the limit counts (the name's length plus one for
// each underscore; mariadbgrant.OverLimit).
func writeDatabaseNameTooLong(w http.ResponseWriter, name string) {
	over := strconv.Itoa(mariadbgrant.OverLimit(name))
	writeCodedErrorVars(w, http.StatusConflict, errCodeDatabaseNameTooLong,
		"The database name "+name+" is too long. Choose a name at least "+over+" characters shorter, or with fewer underscores (each underscore counts as two characters). Nothing was changed.",
		"", nil, map[string]string{"name": name, "over": over})
}

// databaseNameTakenByEngine: the driver found the name on the engine itself.
func databaseNameTakenByEngine(err error) bool {
	return errors.Is(err, services.ErrDatabaseNameTaken)
}
