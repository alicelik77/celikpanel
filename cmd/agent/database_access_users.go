package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// More users for one database (alpha.129). The statements run as the
// engine's own administrator (mysql as root over the socket, psql as
// postgres on the database's cluster), on stdin; a statement that carries a
// password is never logged and the engine's answer to it is never forwarded.
//
// Read only:
//   - MariaDB: SELECT on the database.
//   - PostgreSQL: CONNECT; in every schema of the database that is not the
//     system's (pg_*, information_schema): USAGE, SELECT on its tables and
//     sequences. Later: SELECT on the tables and sequences, and USAGE on the
//     schemas, that the database's owner or any read-and-write user of the
//     database makes (ALTER DEFAULT PRIVILEGES FOR ROLE each of them, in the
//     whole database, so new schemas are covered too).
//
// Read and write:
//   - MariaDB: ALL PRIVILEGES on the database, the set the user made with the
//     database has (without GRANT OPTION).
//   - PostgreSQL: CONNECT and TEMPORARY; in every such schema USAGE and
//     CREATE, all rights on its tables and sequences, EXECUTE on its
//     functions. Later, both ways between this user and the owner and the
//     other read-and-write users: all rights on what each of them makes; and
//     SELECT (USAGE on schemas) on what this user makes for every read-only
//     user of the database.
//
// The other users of the database come with the request (Readers, Writers)
// and only those that exist on the engine are used. Never SUPERUSER,
// CREATEDB, CREATEROLE, REPLICATION or BYPASSRLS. docs/DATABASE-USERS.md
// says exactly what is covered.
//
// Removal: MariaDB DROP USER on every host of the name; PostgreSQL REASSIGN
// OWNED to the database's owner (a table the user made stays, owned by the
// owner), DROP OWNED (its rights in that database) and DROP ROLE. The
// database and its data are never touched.
//
// Bir veritabanına ek kullanıcılar: yalnız okuma ya da okuma ve yazma;
// kaldırma veritabanına ve verisine dokunmaz.

func validDatabaseAccessUser(req *transport.DatabaseAccessUserRequest, creating bool) error {
	if req.Engine != "mysql" && req.Engine != "postgresql" {
		return fmt.Errorf("unknown database engine %q; expected mysql or postgresql", req.Engine)
	}
	if err := services.ValidateSQLIdentifier(req.Database); err != nil {
		return fmt.Errorf("invalid database name: %w", err)
	}
	if err := services.ValidateSQLIdentifier(req.Username); err != nil {
		return fmt.Errorf("invalid database user name: %w", err)
	}
	if suspensionReservedDatabaseUser(req.Username) {
		return fmt.Errorf("%s is an engine or CelikPanel account", req.Username)
	}
	if creating {
		if req.Access != transport.DatabaseAccessRead && req.Access != transport.DatabaseAccessWrite {
			return fmt.Errorf("unknown access %q; expected read or write", req.Access)
		}
		if err := rotationPasswordValid(req.Password); err != nil {
			return err
		}
	}
	return nil
}

// markerValue reads the one "<name>=" line of an inspection's output.
func markerValue(output []byte, name string) (string, bool) {
	value, found := "", 0
	for _, line := range strings.Split(string(output), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), name+"="); ok {
			value = rest
			found++
		}
	}
	return value, found == 1
}

// CreateDatabaseAccessUserV1 makes a user that reads, or reads and writes,
// one existing database.
func (a *Agent) CreateDatabaseAccessUserV1(req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) error {
	if req == nil || resp == nil {
		return errors.New("database user request and response are required")
	}
	*resp = transport.DatabaseAccessUserResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "adding a database user"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := validDatabaseAccessUser(req, true); err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()
	if req.Engine == "mysql" {
		createMySQLAccessUser(req, resp)
		return nil
	}
	run, err := postgreSQLStatementFor(req.Port)
	if err != nil {
		resp.Error = err.Error()
		resp.Refusal = pgClusterRefusalOf(err)
		return nil
	}
	createPostgreSQLAccessUser(run, req, resp)
	return nil
}

// RemoveDatabaseAccessUserV1 removes such a user and its rights; the
// database stays as it is.
func (a *Agent) RemoveDatabaseAccessUserV1(req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) error {
	if req == nil || resp == nil {
		return errors.New("database user request and response are required")
	}
	*resp = transport.DatabaseAccessUserResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "removing a database user"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := validDatabaseAccessUser(req, false); err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return nil
	}
	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()
	if req.Engine == "mysql" {
		removeMySQLAccessUser(req, resp)
		return nil
	}
	run, err := postgreSQLStatementFor(req.Port)
	if err != nil {
		resp.Error = err.Error()
		resp.Refusal = pgClusterRefusalOf(err)
		return nil
	}
	removePostgreSQLAccessUser(run, req, resp)
	return nil
}

// ---- MariaDB ----

func createMySQLAccessUser(req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) {
	user, _ := services.QuoteMySQLStringLiteral(req.Username)
	dbLiteral, _ := services.QuoteMySQLStringLiteral(req.Database)
	dbIdent, err := services.QuoteMySQLIdentifier(req.Database)
	if err != nil {
		resp.Error = "invalid database name; nothing was changed"
		return
	}
	secret, err := services.QuoteMySQLStringLiteral(req.Password)
	if err != nil {
		resp.Error = "the password cannot be written as a MariaDB string; nothing was changed"
		return
	}
	output, err := mysqlExecStatement("SELECT CONCAT('users=', COUNT(*)) FROM mysql.user WHERE User = " + user + "; " +
		"SELECT CONCAT('databases=', COUNT(*)) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = " + dbLiteral + ";")
	if err != nil {
		resp.Error = hostcmd.Fail("the database server could not be asked about the name; nothing was changed", output, err, databaseClientMeaning).Error()
		return
	}
	users, okUsers := markerValue(output, "users")
	databases, okDatabases := markerValue(output, "databases")
	switch {
	case !okUsers || !okDatabases:
		resp.Error = "the database server's answer about the name could not be read; nothing was changed"
		return
	case users != "0":
		resp.Exists = true
		resp.Error = "a database user named " + req.Username + " already exists on this server; nothing was changed"
		return
	case databases != "1":
		resp.Error = "database " + req.Database + " does not exist on this server; nothing was changed"
		return
	}
	if _, err := mysqlExecStatement("CREATE USER " + user + "@'localhost' IDENTIFIED BY " + secret + ";"); err != nil {
		// The engine's words may quote the statement: never forwarded.
		resp.Error = "the database server did not create the user " + req.Username + "; nothing was changed"
		return
	}
	rights := "SELECT"
	if req.Access == transport.DatabaseAccessWrite {
		rights = "ALL PRIVILEGES"
	}
	if output, err := mysqlExecStatement("GRANT " + rights + " ON " + dbIdent + ".* TO " + user + "@'localhost'; FLUSH PRIVILEGES;"); err != nil {
		failure := hostcmd.Fail("the database server did not give the new user its rights", output, err, databaseClientMeaning).Error()
		if _, dropErr := mysqlExecStatement("DROP USER IF EXISTS " + user + "@'localhost';"); dropErr != nil {
			resp.CleanupIncomplete = true
			resp.Error = failure + "; the user it had just made could not be taken back either"
			return
		}
		resp.Error = failure + "; the user it had just made was taken back, so nothing was changed"
		return
	}
	resp.Applied = true
}

func removeMySQLAccessUser(req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) {
	list, _ := rotationMySQLHostsStatement(req.Username)
	output, err := mysqlExecStatement(list)
	if err != nil {
		resp.Error = hostcmd.Fail("the database server could not be asked about the user; nothing was changed", output, err, databaseClientMeaning).Error()
		return
	}
	hosts := parseRotationMySQLHosts(output)
	if len(hosts) == 0 {
		resp.Absent, resp.Applied = true, true
		return
	}
	user, _ := services.QuoteMySQLStringLiteral(req.Username)
	accounts := make([]string, 0, len(hosts))
	for _, host := range hosts {
		literal, err := services.QuoteMySQLStringLiteral(host)
		if err != nil {
			resp.Error = fmt.Sprintf("the account's host %q cannot be written as a MariaDB string; nothing was changed", host)
			return
		}
		accounts = append(accounts, user+"@"+literal)
	}
	if output, err := mysqlExecStatement("DROP USER " + strings.Join(accounts, ", ") + "; FLUSH PRIVILEGES;"); err != nil {
		resp.Error = hostcmd.Fail("the database server did not remove the user", output, err, databaseClientMeaning).Error()
		return
	}
	resp.Applied = true
}

// ---- PostgreSQL ----

// postgreSQLAccessFacts reads whether the role exists and who owns the
// database ("" when the database does not exist).
func postgreSQLAccessFacts(run func(string) ([]byte, error), username, database string) (roles string, owner string, err error) {
	user, _ := services.QuotePGStringLiteral(username)
	db, _ := services.QuotePGStringLiteral(database)
	output, runErr := run("SELECT 'roles=' || count(*) FROM pg_catalog.pg_roles WHERE rolname = " + user + ";\n" +
		"SELECT 'owner=' || COALESCE((SELECT pg_catalog.pg_get_userbyid(datdba) FROM pg_catalog.pg_database WHERE datname = " + db + "), '');")
	if runErr != nil {
		return "", "", hostcmd.Fail("the database server could not be asked about the name", output, runErr, databaseClientMeaning)
	}
	roles, okRoles := markerValue(output, "roles")
	owner, okOwner := markerValue(output, "owner")
	if !okRoles || !okOwner {
		return "", "", errors.New("the database server's answer about the name could not be read")
	}
	return roles, owner, nil
}

// postgreSQLAccessGrants is the script that gives the rights inside the
// database, in one transaction. readers and writers are the database's
// other access users that exist on the engine (never the owner or username).
func postgreSQLAccessGrants(database, username, owner, access string, readers, writers []string) (string, error) {
	db, err := services.QuotePGIdentifier(database)
	if err != nil {
		return "", err
	}
	user, err := services.QuotePGIdentifier(username)
	if err != nil {
		return "", err
	}
	userLiteral, err := services.QuotePGStringLiteral(username)
	if err != nil {
		return "", err
	}
	own, err := services.QuotePGIdentifier(owner)
	if err != nil {
		return "", fmt.Errorf("the database's owner %q is not a name CelikPanel manages", owner)
	}
	quoteAll := func(names []string) ([]string, error) {
		out := make([]string, 0, len(names))
		for _, name := range names {
			quoted, err := services.QuotePGIdentifier(name)
			if err != nil {
				return nil, fmt.Errorf("invalid database user name %q", name)
			}
			out = append(out, quoted)
		}
		return out, nil
	}
	others, err := quoteAll(writers)
	if err != nil {
		return "", err
	}
	reading, err := quoteAll(readers)
	if err != nil {
		return "", err
	}
	// The roles whose new objects this user must reach: the owner and every
	// read-and-write user.
	creators := append([]string{own}, others...)

	// Every non-system schema, existing objects. The role's name is a
	// validated identifier, passed as a literal to format().
	schemaGrants := []string{
		"EXECUTE pg_catalog.format('GRANT USAGE ON SCHEMA %I TO %I', s, " + userLiteral + ");",
		"EXECUTE pg_catalog.format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO %I', s, " + userLiteral + ");",
		"EXECUTE pg_catalog.format('GRANT SELECT ON ALL SEQUENCES IN SCHEMA %I TO %I', s, " + userLiteral + ");",
	}
	databaseRights := "CONNECT"
	if access == transport.DatabaseAccessWrite {
		databaseRights = "CONNECT, TEMPORARY"
		schemaGrants = []string{
			"EXECUTE pg_catalog.format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', s, " + userLiteral + ");",
			"EXECUTE pg_catalog.format('GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA %I TO %I', s, " + userLiteral + ");",
			"EXECUTE pg_catalog.format('GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA %I TO %I', s, " + userLiteral + ");",
			"EXECUTE pg_catalog.format('GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA %I TO %I', s, " + userLiteral + ");",
		}
	}
	lines := []string{`\connect ` + db, "BEGIN;",
		"GRANT " + databaseRights + " ON DATABASE " + db + " TO " + user + ";",
		"DO $celikpanel$ DECLARE s text; BEGIN FOR s IN SELECT nspname FROM pg_catalog.pg_namespace " +
			"WHERE nspname NOT LIKE 'pg\\_%' AND nspname <> 'information_schema' LOOP " + strings.Join(schemaGrants, " ") + " END LOOP; END $celikpanel$;",
	}
	defaults := func(creator, rights, grantee string) {
		lines = append(lines, "ALTER DEFAULT PRIVILEGES FOR ROLE "+creator+" "+rights+" TO "+grantee+";")
	}
	if access == transport.DatabaseAccessRead {
		for _, creator := range creators {
			defaults(creator, "GRANT SELECT ON TABLES", user)
			defaults(creator, "GRANT SELECT ON SEQUENCES", user)
			defaults(creator, "GRANT USAGE ON SCHEMAS", user)
		}
	} else {
		for _, creator := range creators {
			defaults(creator, "GRANT ALL ON TABLES", user)
			defaults(creator, "GRANT ALL ON SEQUENCES", user)
			defaults(creator, "GRANT USAGE, CREATE ON SCHEMAS", user)
			// And what this user makes, for each of them.
			defaults(user, "GRANT ALL ON TABLES", creator)
			defaults(user, "GRANT ALL ON SEQUENCES", creator)
			defaults(user, "GRANT USAGE, CREATE ON SCHEMAS", creator)
		}
		for _, reader := range reading {
			defaults(user, "GRANT SELECT ON TABLES", reader)
			defaults(user, "GRANT SELECT ON SEQUENCES", reader)
			defaults(user, "GRANT USAGE ON SCHEMAS", reader)
		}
	}
	lines = append(lines, "COMMIT;")
	return strings.Join(lines, "\n"), nil
}

// postgreSQLExistingRoles keeps the names that exist as roles on the engine,
// leaving out the owner and the user itself.
func postgreSQLExistingRoles(run func(string) ([]byte, error), names []string, skip ...string) ([]string, error) {
	var asked []string
	for _, name := range names {
		if services.ValidateSQLIdentifier(name) != nil || slices.Contains(skip, name) || suspensionReservedDatabaseUser(name) {
			continue
		}
		literal, _ := services.QuotePGStringLiteral(name)
		asked = append(asked, literal)
	}
	if len(asked) == 0 {
		return nil, nil
	}
	output, err := run("SELECT 'role=' || rolname FROM pg_catalog.pg_roles WHERE rolname IN (" + strings.Join(asked, ", ") + ") ORDER BY rolname;")
	if err != nil {
		return nil, hostcmd.Fail("the database server could not be asked about the database's other users", output, err, databaseClientMeaning)
	}
	var found []string
	for _, line := range strings.Split(string(output), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "role="); ok && name != "" {
			found = append(found, name)
		}
	}
	return found, nil
}

func createPostgreSQLAccessUser(run func(string) ([]byte, error), req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) {
	roles, owner, err := postgreSQLAccessFacts(run, req.Username, req.Database)
	switch {
	case err != nil:
		resp.Error = err.Error() + "; nothing was changed"
		return
	case roles != "0":
		resp.Exists = true
		resp.Error = "a database user named " + req.Username + " already exists on this server; nothing was changed"
		return
	case owner == "":
		resp.Error = "database " + req.Database + " does not exist on this server; nothing was changed"
		return
	}
	readers, err := postgreSQLExistingRoles(run, req.Readers, owner, req.Username)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return
	}
	writers, err := postgreSQLExistingRoles(run, req.Writers, owner, req.Username)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return
	}
	grants, err := postgreSQLAccessGrants(req.Database, req.Username, owner, req.Access, readers, writers)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return
	}
	ident, _ := services.QuotePGIdentifier(req.Username)
	secret, err := services.QuotePGStringLiteral(req.Password)
	if err != nil {
		resp.Error = "the password cannot be written as a PostgreSQL string; nothing was changed"
		return
	}
	if _, err := run("SET password_encryption = 'scram-sha-256';\nCREATE ROLE " + ident +
		" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD " + secret + ";"); err != nil {
		// The engine's words may quote the statement: never forwarded.
		resp.Error = "the database server did not create the user " + req.Username + "; nothing was changed"
		return
	}
	if output, err := run(grants); err != nil {
		failure := hostcmd.Fail("the database server did not give the new user its rights", output, err, databaseClientMeaning).Error()
		if _, dropErr := run("DROP ROLE IF EXISTS " + ident + ";"); dropErr != nil {
			resp.CleanupIncomplete = true
			resp.Error = failure + "; the user it had just made could not be taken back either"
			return
		}
		resp.Error = failure + "; the user it had just made was taken back, so nothing was changed"
		return
	}
	resp.Applied = true
}

func removePostgreSQLAccessUser(run func(string) ([]byte, error), req *transport.DatabaseAccessUserRequest, resp *transport.DatabaseAccessUserResponse) {
	roles, owner, err := postgreSQLAccessFacts(run, req.Username, req.Database)
	if err != nil {
		resp.Error = err.Error() + "; nothing was changed"
		return
	}
	if roles == "0" {
		resp.Absent, resp.Applied = true, true
		return
	}
	ident, _ := services.QuotePGIdentifier(req.Username)
	if owner != "" {
		db, _ := services.QuotePGIdentifier(req.Database)
		own, err := services.QuotePGIdentifier(owner)
		if err != nil {
			resp.Error = fmt.Sprintf("the database's owner %q is not a name CelikPanel manages; nothing was changed", owner)
			return
		}
		// What the user made in the database becomes the owner's; then its
		// rights there go. No table or row is removed.
		script := strings.Join([]string{`\connect ` + db, "BEGIN;",
			"REASSIGN OWNED BY " + ident + " TO " + own + ";",
			"DROP OWNED BY " + ident + ";", "COMMIT;"}, "\n")
		if output, err := run(script); err != nil {
			resp.Error = hostcmd.Fail("the database server did not take the user's rights in "+req.Database+" away; nothing was changed", output, err, databaseClientMeaning).Error()
			return
		}
	}
	if output, err := run("DROP ROLE " + ident + ";"); err != nil {
		resp.Error = hostcmd.Fail("the user's rights in "+req.Database+" were taken away, but the database server did not remove the user (it may still have rights in another database of this server)", output, err, databaseClientMeaning).Error()
		return
	}
	resp.Applied = true
}
