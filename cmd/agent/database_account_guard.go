package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// No Agent database call acts on a privileged account on the panel's word
// (alpha.149 security fix; review of PR #370, finding 1).
//
// The Agent runs as root and, on the engines, as their superuser (mysql as
// root over the socket, psql as postgres); the panel is the less trusted
// side. Before this fix a call that changes an account the panel names
// (a password, a lock, a drop, a revoke, limits) checked only the name's
// form and a short list of engine names, so a compromised panel could give
// the server owner's own administrator account (an "admin" superuser, say) a
// password it chose and take the engine over. Now every such call asks the
// engine itself, as the superuser, what the named account holds, and refuses
// - nothing changed, the answer naming the reason as a code
// (transport.DatabaseAccountRefused*) - unless the account is an ordinary
// one. Where the engine allows it the same test runs again in the same
// transaction (PostgreSQL) or the same compound statement (MariaDB) as the
// write, so the write cannot land on an account that changed in between. A
// state that cannot be read refuses.
//
// The rule:
//
//   - PostgreSQL: the role exists; it is not a superuser and has none of
//     CREATEROLE, CREATEDB, REPLICATION or BYPASSRLS; and it is not a member,
//     directly or through any chain (pg_has_role MEMBER, whatever INHERIT
//     and SET say), of a role that has one of them or of a predefined pg_*
//     role. pg_database_owner is left out: its only member is implicit (the
//     current database's owner) and it cannot be granted. The limits call
//     adds: a member of no role at all (pgAccountNoMembership).
//   - MariaDB: the account (every host of the name) holds no global
//     privilege - mysql.global_priv's access bits are 0, which leaves only
//     USAGE (the bits include GRANT OPTION on *.*); it holds no role and no
//     PROXY right; and it holds no right on the mysql system database (a
//     database-level grant whose pattern matches "mysql", or a table-level
//     grant there), which would let it write the grant tables.
//
// Every account CelikPanel makes passes: site and added users are plain
// LOGIN roles (PostgreSQL) or 'name'@'localhost' with rights on their own
// databases only (MariaDB); no path grants a role, a membership or a global
// privilege to them (the PR names each creating path).
//
// Panelin adını verdiği ayrıcalıklı veritabanı hesabı hiçbir Agent çağrısıyla
// değiştirilmez: Agent motora hesabın neye sahip olduğunu kendisi sorar ve
// sıradan bir hesap değilse hiçbir şeyi değiştirmeden reddeder.

// pgAccountRule is the PostgreSQL rule a call applies.
type pgAccountRule int

const (
	// pgAccountOrdinary: the rule above.
	pgAccountOrdinary pgAccountRule = iota
	// pgAccountNoMembership: the rule above, and a member of no role at all
	// (the limits call; review of PR #370, finding 2).
	pgAccountNoMembership
)

// errDatabaseAccountRefused carries a refusal code.
type errDatabaseAccountRefused struct {
	code     string
	username string
}

func (e *errDatabaseAccountRefused) Error() string {
	return databaseAccountRefusalText(e.username, e.code)
}

// databaseAccountRefusalOf answers the refusal code an error carries, "" for
// none.
func databaseAccountRefusalOf(err error) string {
	var refused *errDatabaseAccountRefused
	if errors.As(err, &refused) {
		return refused.code
	}
	return ""
}

// databaseAccountRefusalText is the answer's plain sentence for a refusal.
func databaseAccountRefusalText(username, code string) string {
	switch code {
	case transport.DatabaseAccountRefusedAbsent:
		return "the database user " + username + " does not exist on this server's database engine; nothing was changed"
	case transport.DatabaseAccountRefusedPrivileged:
		return "the database user " + username + " holds administrator rights on this server's database engine (directly or through a role), " +
			"so CelikPanel does not change it; nothing was changed. The server's administrator decides about such accounts"
	case transport.DatabaseAccountRefusedMember:
		return "the database user " + username + " is a member of another PostgreSQL role, which no site database user CelikPanel makes is, " +
			"so CelikPanel does not change its limits; nothing was changed. The server's administrator decides"
	case transport.DatabaseAccountRefusedSystemDatabase:
		return "the database server's own system database is never given to a database user by CelikPanel; nothing was changed"
	}
	return "what the database user " + username + " holds on this server's database engine could not be read, so CelikPanel did not change it; nothing was changed"
}

// --- PostgreSQL ------------------------------------------------------------

// pgPrivilegedFlags is the attribute test on one pg_roles row.
func pgPrivilegedFlags(alias string) string {
	return alias + ".rolsuper OR " + alias + ".rolcreaterole OR " + alias + ".rolcreatedb OR " +
		alias + ".rolreplication OR " + alias + ".rolbypassrls"
}

// pgAccountPrivilegedExpr is true when the pg_roles row alias is privileged
// by the rule (its own attributes, or a membership through any chain of a
// role that has one of them or is a predefined pg_* role).
func pgAccountPrivilegedExpr(alias string) string {
	return "(" + pgPrivilegedFlags(alias) +
		" OR EXISTS (SELECT 1 FROM pg_catalog.pg_roles celikpanel_held WHERE celikpanel_held.oid OPERATOR(pg_catalog.<>) " + alias + ".oid" +
		" AND (" + pgPrivilegedFlags("celikpanel_held") +
		" OR (pg_catalog.starts_with(celikpanel_held.rolname::pg_catalog.text, 'pg_')" +
		" AND celikpanel_held.rolname OPERATOR(pg_catalog.<>) 'pg_database_owner'))" +
		" AND pg_catalog.pg_has_role(" + alias + ".oid, celikpanel_held.oid, 'MEMBER')))"
}

// pgAccountStateExpr is a text expression: 'ok', 'absent', 'privileged' or
// (pgAccountNoMembership) 'member', for the role named by literal.
func pgAccountStateExpr(literal string, rule pgAccountRule) string {
	member := ""
	if rule == pgAccountNoMembership {
		member = " WHEN EXISTS (SELECT 1 FROM pg_catalog.pg_auth_members celikpanel_m" +
			" WHERE celikpanel_m.member OPERATOR(pg_catalog.=) celikpanel_r.oid) THEN 'member'"
	}
	return "COALESCE((SELECT CASE WHEN " + pgAccountPrivilegedExpr("celikpanel_r") + " THEN 'privileged'" + member +
		" ELSE 'ok' END FROM pg_catalog.pg_roles celikpanel_r WHERE celikpanel_r.rolname OPERATOR(pg_catalog.=) " + literal + "), 'absent')"
}

// pgAccountStateQuery reads the state as one "guard=" marker line.
func pgAccountStateQuery(username string, rule pgAccountRule) (string, error) {
	literal, err := pgAccountLiteral(username)
	if err != nil {
		return "", err
	}
	return "SELECT 'guard=' || " + pgAccountStateExpr(literal, rule) + ";", nil
}

// pgAccountGuardBlock raises celikpanel-refused:<code> unless the role passes
// the rule; it is put in the same transaction as the write.
func pgAccountGuardBlock(username string, rule pgAccountRule) (string, error) {
	literal, err := pgAccountLiteral(username)
	if err != nil {
		return "", err
	}
	return "DO $celikpanel_account_guard$\nDECLARE state pg_catalog.text;\nBEGIN\n" +
		"  state := " + pgAccountStateExpr(literal, rule) + ";\n" +
		"  IF state OPERATOR(pg_catalog.<>) 'ok' THEN RAISE EXCEPTION 'celikpanel-refused:%', state; END IF;\n" +
		"END\n$celikpanel_account_guard$;", nil
}

// pgGuarded runs body in one transaction after the guard of username.
func pgGuarded(username string, rule pgAccountRule, body string) (string, error) {
	guard, err := pgAccountGuardBlock(username, rule)
	if err != nil {
		return "", err
	}
	return "BEGIN;\n" + guard + "\n" + body + "\nCOMMIT;", nil
}

func pgAccountLiteral(username string) (string, error) {
	if err := services.ValidateSQLIdentifier(username); err != nil {
		return "", fmt.Errorf("invalid database user name: %w", err)
	}
	return services.QuotePGStringLiteral(username)
}

// --- MariaDB ---------------------------------------------------------------

// mysqlAccountPrivilegedExpr is true when any account of the name (every
// host; a role of that name too) is privileged by the rule.
func mysqlAccountPrivilegedExpr(literal string) string {
	return "(EXISTS (SELECT 1 FROM mysql.global_priv WHERE User = " + literal + " AND IFNULL(JSON_VALUE(Priv, '$.access'), 'unknown') <> '0')" +
		" OR EXISTS (SELECT 1 FROM mysql.roles_mapping WHERE User = " + literal + ")" +
		" OR EXISTS (SELECT 1 FROM mysql.proxies_priv WHERE User = " + literal + ")" +
		" OR EXISTS (SELECT 1 FROM mysql.db WHERE User = " + literal + " AND 'mysql' LIKE Db)" +
		" OR EXISTS (SELECT 1 FROM mysql.tables_priv WHERE User = " + literal + " AND Db = 'mysql'))"
}

// mysqlAccountAbsentExpr: no account of the name on localhost (anyHost:
// on any host).
func mysqlAccountAbsentExpr(literal string, anyHost bool) string {
	where := "User = " + literal
	if !anyHost {
		where += " AND Host = 'localhost'"
	}
	return "NOT EXISTS (SELECT 1 FROM mysql.global_priv WHERE " + where + ")"
}

// mysqlAccountStateQuery reads the state as one "guard=" marker line: ok,
// absent or privileged.
func mysqlAccountStateQuery(username string, anyHost bool) (string, error) {
	literal, err := mysqlAccountLiteral(username)
	if err != nil {
		return "", err
	}
	return "SELECT CONCAT('guard=', CASE WHEN " + mysqlAccountAbsentExpr(literal, anyHost) + " THEN 'absent' WHEN " +
		mysqlAccountPrivilegedExpr(literal) + " THEN 'privileged' ELSE 'ok' END);", nil
}

// mysqlGuarded is one compound statement (MariaDB 10.1 and later): the guard
// of username, then body (statements, each ending in ";"). A refusal is a
// SIGNAL "celikpanel-refused:<code>" before body runs.
func mysqlGuarded(username string, anyHost bool, body string) (string, error) {
	literal, err := mysqlAccountLiteral(username)
	if err != nil {
		return "", err
	}
	return "DELIMITER ;;\nBEGIN NOT ATOMIC\n" +
		"  IF " + mysqlAccountAbsentExpr(literal, anyHost) + " THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'celikpanel-refused:absent'; END IF;\n" +
		"  IF " + mysqlAccountPrivilegedExpr(literal) + " THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'celikpanel-refused:privileged'; END IF;\n" +
		"  " + body + "\n" +
		"END;;\nDELIMITER ;", nil
}

func mysqlAccountLiteral(username string) (string, error) {
	if err := services.ValidateSQLIdentifier(username); err != nil {
		return "", fmt.Errorf("invalid database user name: %w", err)
	}
	return services.QuoteMySQLStringLiteral(username)
}

// --- Both ------------------------------------------------------------------

// databaseAccountRefusalIn reads a guard's refusal from a client's output.
func databaseAccountRefusalIn(output []byte) string {
	text := string(output)
	for _, code := range []string{transport.DatabaseAccountRefusedAbsent, transport.DatabaseAccountRefusedPrivileged, transport.DatabaseAccountRefusedMember} {
		if strings.Contains(text, "celikpanel-refused:"+code) {
			return code
		}
	}
	return ""
}

// readDatabaseAccountState asks the engine ("mysql" or "postgresql") through
// run what username holds. It answers "" for an ordinary account, or the
// refusal code (absent, privileged, member, unreadable) as an
// *errDatabaseAccountRefused.
func readDatabaseAccountState(engine string, run func(string) ([]byte, error), username string, rule pgAccountRule, anyHost bool) error {
	var query string
	var err error
	switch engine {
	case "mysql":
		query, err = mysqlAccountStateQuery(username, anyHost)
	case "postgresql":
		query, err = pgAccountStateQuery(username, rule)
	default:
		err = fmt.Errorf("unknown database engine %q; expected mysql or postgresql", engine)
	}
	if err != nil {
		return err
	}
	output, runErr := run(query)
	state, ok := markerValue(output, "guard")
	if runErr != nil || !ok {
		return &errDatabaseAccountRefused{code: transport.DatabaseAccountRefusedUnreadable, username: username}
	}
	switch state {
	case "ok":
		return nil
	case transport.DatabaseAccountRefusedAbsent, transport.DatabaseAccountRefusedPrivileged, transport.DatabaseAccountRefusedMember:
		return &errDatabaseAccountRefused{code: state, username: username}
	}
	return &errDatabaseAccountRefused{code: transport.DatabaseAccountRefusedUnreadable, username: username}
}

// systemDatabaseName: the engine's own system databases, never given to a
// database user by CelikPanel.
func systemDatabaseName(engine, name string) bool {
	lowered := strings.ToLower(name)
	switch engine {
	case "mysql":
		switch lowered {
		case "mysql", "information_schema", "performance_schema", "sys":
			return true
		}
	case "postgresql":
		switch lowered {
		case "postgres", "template0", "template1":
			return true
		}
	}
	return false
}
