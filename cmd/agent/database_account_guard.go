package main

import (
	"errors"
	"fmt"
	"regexp"
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
		return systemDatabaseRefusal
	case transport.DatabaseAccountRefusedUnsupportedEngine:
		return "this server runs MySQL in MariaDB's place, and CelikPanel manages database accounts on MariaDB only, so it did not change the database user " +
			username + "; nothing was changed. The server's administrator makes the change on the server itself"
	case transport.DatabaseAccountRefusedOwnsObjects:
		return "the database user " + username + " owns tables or other objects in this database, so it was not removed; nothing was changed. " +
			"The server's administrator first hands them to the database's own user on the server (the user guide's Databases chapter gives the command), " +
			"then removes the user again from the panel"
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
	return pgGuardBlockOf(pgAccountStateExpr(literal, rule), literal), nil
}

// pgGuardBlockOf is the DO block that raises celikpanel-refused:<state>:<name>
// unless the state expression answers 'ok'. The name (a validated
// identifier, as a literal) tells which of several guarded roles in one
// transaction refused (second review of PR #370, finding 9).
func pgGuardBlockOf(stateExpr, literal string) string {
	return "DO $celikpanel_account_guard$\nDECLARE state pg_catalog.text;\nBEGIN\n" +
		"  state := " + stateExpr + ";\n" +
		"  IF state OPERATOR(pg_catalog.<>) 'ok' THEN RAISE EXCEPTION 'celikpanel-refused:%:%', state, " + literal + "; END IF;\n" +
		"END\n$celikpanel_account_guard$;"
}

// A guard's refusal is read only from the client's own error line (third
// review of PR #370, I1): the first line of the output that is an error
// line, and only when the whole line is the refusal - psql's
// "ERROR:  celikpanel-refused:<code>[:<name>]" (with psql's
// "psql:<file>:<line>: " locus when it reads a file), or MariaDB's SIGNAL
// "ERROR 1644 (45000) at line <n>: celikpanel-refused:<code>". Before, the
// marker was looked for anywhere in the output, so a site user could name
// an object "celikpanel-refused:privileged:postgres" and, when a later
// statement's error quoted it, turn the answer into a refusal naming
// another account. Both clients stop at the first error (ON_ERROR_STOP,
// MariaDB without --force), so the first error line is the one that ended
// the run; an identifier with a line break in it can only follow it.
var (
	databaseErrorLineRe   = regexp.MustCompile(`^(?:psql:[^ ]*:[0-9]+: )?ERROR[: ]`)
	databaseRefusalLineRe = regexp.MustCompile(`^(?:psql:[^ ]*:[0-9]+: )?(?:ERROR:[ \t]+|ERROR 1644 \(45000\)(?: at line [0-9]+)?: )` +
		`celikpanel-refused:([a-z_]+)(?::([A-Za-z_][A-Za-z0-9_]*))?[ \t]*$`)
)

// databaseAccountRefusalLine answers the code and the name (or "") of the
// refusal on the output's first error line; "" and "" when that line is no
// refusal or there is none.
func databaseAccountRefusalLine(output []byte) (code, name string) {
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimRight(line, "\r")
		if !databaseErrorLineRe.MatchString(line) {
			continue
		}
		if match := databaseRefusalLineRe.FindStringSubmatch(line); match != nil {
			return match[1], match[2]
		}
		return "", ""
	}
	return "", ""
}

// databaseAccountRefusedNameIn answers the role a write-time refusal names,
// "" when it names none.
func databaseAccountRefusedNameIn(output []byte) string {
	if code, name := databaseAccountRefusalLine(output); databaseAccountGuardCode(code) {
		return name
	}
	return ""
}

// databaseAccountGuardCode: a code the write-time guards raise.
func databaseAccountGuardCode(code string) bool {
	switch code {
	case transport.DatabaseAccountRefusedAbsent, transport.DatabaseAccountRefusedPrivileged, transport.DatabaseAccountRefusedMember,
		transport.DatabaseAccountRefusedOwnsObjects:
		return true
	}
	return false
}

// databaseAccountRefusedName answers the account a refusal error is about.
func databaseAccountRefusedName(err error) string {
	var refused *errDatabaseAccountRefused
	if errors.As(err, &refused) {
		return refused.username
	}
	return ""
}

// The owner of a database a call adds a user to or removes one from
// (alpha.149, second review of PR #370 finding 1). The added-user calls write
// ALTER DEFAULT PRIVILEGES FOR ROLE <owner> and REASSIGN OWNED BY <user> TO
// <owner>; with the owner a superuser, a compromised panel could add a user
// to the server owner's own database, let it create a SECURITY DEFINER
// function, and have the removal hand that function to the superuser. So the
// owner read from the engine must pass the shared rule too. The one exception
// is exactly CelikPanel's own account (celikpanel_admin, which owns the
// databases made through it and holds CREATEDB and CREATEROLE by design), and
// only while it is not a superuser, has neither REPLICATION nor BYPASSRLS, and
// is a member - through any chain - of no predefined pg_* role and of no
// superuser role. Even then a removal never hands it anything (third review
// of PR #370, H1; pgOwnsObjectsGuardBlock).
//
// Veritabanının sahibi de sıradan bir rol olmalıdır; tek istisna, süper
// kullanıcı olmadığı ve hiçbir pg_* rolünün üyesi olmadığı sürece
// CelikPanel'in kendi celikpanel_admin hesabıdır.

// pgDatabaseOwnerStateExpr is the owner's state expression: the shared rule,
// or for celikpanel_admin the narrower test above.
func pgDatabaseOwnerStateExpr(literal string, admin bool) string {
	if !admin {
		return pgAccountStateExpr(literal, pgAccountOrdinary)
	}
	// Third review of PR #370, I2: its own REPLICATION or BYPASSRLS fails it
	// too (an administrator may have given it either by hand).
	return "COALESCE((SELECT CASE WHEN celikpanel_r.rolsuper OR celikpanel_r.rolreplication OR celikpanel_r.rolbypassrls" +
		" OR EXISTS (SELECT 1 FROM pg_catalog.pg_roles celikpanel_held WHERE celikpanel_held.oid OPERATOR(pg_catalog.<>) celikpanel_r.oid" +
		" AND (celikpanel_held.rolsuper OR (pg_catalog.starts_with(celikpanel_held.rolname::pg_catalog.text, 'pg_')" +
		" AND celikpanel_held.rolname OPERATOR(pg_catalog.<>) 'pg_database_owner'))" +
		" AND pg_catalog.pg_has_role(celikpanel_r.oid, celikpanel_held.oid, 'MEMBER'))" +
		" THEN 'privileged' ELSE 'ok' END FROM pg_catalog.pg_roles celikpanel_r WHERE celikpanel_r.rolname OPERATOR(pg_catalog.=) " + literal + "), 'absent')"
}

// pgDatabaseOwnerExpr answers the owner's state expression, or an error for a
// name CelikPanel cannot test.
func pgDatabaseOwnerExpr(owner string) (string, error) {
	literal, err := pgAccountLiteral(owner)
	if err != nil {
		return "", fmt.Errorf("the database's owner %q is not a name CelikPanel manages", owner)
	}
	return pgDatabaseOwnerStateExpr(literal, owner == databaseAdminAccountName), nil
}

// readDatabaseOwnerState reads the owner's state as readDatabaseAccountState
// does: nil for an owner that passes, else the refusal.
func readDatabaseOwnerState(run func(string) ([]byte, error), owner string) error {
	expr, err := pgDatabaseOwnerExpr(owner)
	if err != nil {
		return err
	}
	return readGuardState(run, "SELECT 'guard=' || "+expr+";", owner)
}

// pgDatabaseOwnerGuardBlock is the owner's test in the write's transaction.
func pgDatabaseOwnerGuardBlock(owner string) (string, error) {
	expr, err := pgDatabaseOwnerExpr(owner)
	if err != nil {
		return "", err
	}
	literal, _ := pgAccountLiteral(owner)
	return pgGuardBlockOf(expr, literal), nil
}

// pgOwnsObjectsGuardBlock (third review of PR #370, H1) raises
// celikpanel-refused:owns_objects:<user> when the role owns anything that
// REASSIGN OWNED, run in the current database, would hand to another role.
// Removing a user added to a database owned by CelikPanel's own account
// (celikpanel_admin) runs it, in the same transaction, instead of REASSIGN
// OWNED ... TO celikpanel_admin: a site user's function, SECURITY DEFINER or
// called from an index expression, given to celikpanel_admin would run as
// celikpanel_admin, which holds CREATEROLE (and on PostgreSQL 16 and later
// ADMIN on every role it made).
//
// What it reads is exactly the set REASSIGN OWNED walks (shdepReassignOwned):
// pg_shdepend rows of the role with deptype 'o' (owner) whose dbid is the
// current database or 0 (shared objects: a database or tablespace the role
// owns). That covers relations of every kind in every schema (tables, views,
// sequences, materialized views, foreign tables; indexes and TOAST tables
// follow their table), functions and procedures, types and domains,
// operators, operator classes and families, collations, conversions, text
// search objects, extended statistics, schemas, large objects, languages,
// extensions, publications, subscriptions, foreign data wrappers and servers,
// event triggers. Two classes are left out because REASSIGN OWNED skips them
// and only DROP OWNED removes them: default-ACL entries (pg_default_acl; every
// read-and-write user has its own, ALTER DEFAULT PRIVILEGES FOR ROLE <user>)
// and user mappings (pg_user_mapping).
//
// Kaldırılan kullanıcının bu veritabanında nesnesi varsa celikpanel_admin'e
// devredilmez; kaldırma hiçbir şey değiştirmeden reddedilir.
func pgOwnsObjectsGuardBlock(username string) (string, error) {
	literal, err := pgAccountLiteral(username)
	if err != nil {
		return "", err
	}
	return "DO $celikpanel_owned_guard$\nBEGIN\n" +
		"  IF EXISTS (SELECT 1 FROM pg_catalog.pg_shdepend celikpanel_d" +
		" WHERE celikpanel_d.refclassid OPERATOR(pg_catalog.=) 'pg_catalog.pg_authid'::pg_catalog.regclass::pg_catalog.oid" +
		" AND celikpanel_d.refobjid OPERATOR(pg_catalog.=) (SELECT celikpanel_r.oid FROM pg_catalog.pg_roles celikpanel_r" +
		" WHERE celikpanel_r.rolname OPERATOR(pg_catalog.=) " + literal + ")" +
		" AND celikpanel_d.deptype OPERATOR(pg_catalog.=) 'o'" +
		" AND (celikpanel_d.dbid OPERATOR(pg_catalog.=) 0::pg_catalog.oid" +
		" OR celikpanel_d.dbid OPERATOR(pg_catalog.=) (SELECT celikpanel_db.oid FROM pg_catalog.pg_database celikpanel_db" +
		" WHERE celikpanel_db.datname OPERATOR(pg_catalog.=) pg_catalog.current_database()))" +
		" AND celikpanel_d.classid OPERATOR(pg_catalog.<>) 'pg_catalog.pg_default_acl'::pg_catalog.regclass::pg_catalog.oid" +
		" AND celikpanel_d.classid OPERATOR(pg_catalog.<>) 'pg_catalog.pg_user_mapping'::pg_catalog.regclass::pg_catalog.oid)" +
		" THEN RAISE EXCEPTION 'celikpanel-refused:" + transport.DatabaseAccountRefusedOwnsObjects + ":%', " + literal + "; END IF;\n" +
		"END\n$celikpanel_owned_guard$;", nil
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

// databaseAccountRefusalIn reads a guard's refusal from a client's output:
// the code on its first error line (databaseAccountRefusalLine), "" for
// none or a code no guard raises.
func databaseAccountRefusalIn(output []byte) string {
	if code, _ := databaseAccountRefusalLine(output); databaseAccountGuardCode(code) {
		return code
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
	err = readGuardState(run, query, username)
	if engine == "mysql" && databaseAccountRefusalOf(err) == transport.DatabaseAccountRefusedUnreadable {
		if refused := mysqlEngineRefusal(run, username, anyHost); refused != nil {
			return refused
		}
	}
	return err
}

// mysqlEngineRefusal tells, after the MariaDB rule could not be read, a
// MySQL server in MariaDB's place (second review of PR #370, finding 7: MySQL
// has no mysql.global_priv, so the rule fails there on every press and the
// answer used to say "could not be read ... check that the database server
// answers", which it does). It asks the engine's name and, as mysql.user is
// there on both, whether the account exists: an account already gone is
// absent (so a deletion the administrator prepared by removing it by hand
// goes through), any other is unsupported_engine. It answers nil on MariaDB
// or when the engine cannot be told (the caller keeps "unreadable").
func mysqlEngineRefusal(run func(string) ([]byte, error), username string, anyHost bool) error {
	literal, err := mysqlAccountLiteral(username)
	if err != nil {
		return nil
	}
	where := "User = " + literal
	if !anyHost {
		where += " AND Host = 'localhost'"
	}
	output, runErr := run("SELECT CONCAT('engine=', IF(VERSION() LIKE '%MariaDB%', 'mariadb', 'other')); " +
		"SELECT CONCAT('present=', COUNT(*)) FROM mysql.user WHERE " + where + ";")
	if engine, ok := markerValue(output, "engine"); runErr != nil || !ok || engine != "other" {
		return nil
	}
	if present, ok := markerValue(output, "present"); ok && present == "0" {
		return &errDatabaseAccountRefused{code: transport.DatabaseAccountRefusedAbsent, username: username}
	}
	return &errDatabaseAccountRefused{code: transport.DatabaseAccountRefusedUnsupportedEngine, username: username}
}

// readGuardState runs one "guard=" read and answers nil for 'ok' or the
// refusal it names (unreadable for anything else).
func readGuardState(run func(string) ([]byte, error), query, username string) error {
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

// systemDatabaseName: the engine's own system databases. No Agent call gives
// a database user rights on one, drops, empties or loads into one, or
// changes who may enter one (alpha.149; second review of PR #370, finding
// 2: dropping MariaDB's mysql removes every account, dropping PostgreSQL's
// postgres breaks the Agent's own psql). Every caller acts on a site's
// database, which never has such a name. engine: "mysql" (also "mariadb")
// or "postgresql".
//
// Motorun kendi sistem veritabanı hiçbir Agent çağrısıyla silinmez,
// boşaltılmaz, içine yüklenmez ve erişimi değiştirilmez.
func systemDatabaseName(engine, name string) bool {
	lowered := strings.ToLower(name)
	switch engine {
	case "mysql", "mariadb":
		switch lowered {
		case "mysql", "information_schema", "performance_schema", "sys":
			return true
		}
	case "postgresql", "postgres":
		switch lowered {
		case "postgres", "template0", "template1":
			return true
		}
	}
	return false
}

// systemDatabaseRefusal is the answer's sentence when a call names a system
// database.
const systemDatabaseRefusal = "CelikPanel never changes the database server's own system database " +
	"(it is never given to a database user, dropped, emptied, loaded into, or opened or closed to other accounts); nothing was changed"

// errSystemDatabase is that refusal as an error.
var errSystemDatabase = errors.New(systemDatabaseRefusal)
