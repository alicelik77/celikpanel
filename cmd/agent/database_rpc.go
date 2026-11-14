package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

const wordpressDatabaseOwnershipTable = "celikpanel_install_ownership"

var mysqlExecStatement = runMySQLStatement

// mysqlStatementProgram is the MariaDB client the Agent runs statements with.
// Started as "mysql", MariaDB 11 and later print "mysql: Deprecated program
// name ..." on stderr before every answer. runMySQLStatement reads stdout and
// stderr together, so an exact read such as the ownership proof never
// matched: on Arch (MariaDB 13) a refused website copy left its new database
// and user behind while the panel forgot them (alpha.129 lab). "mariadb" is
// the client's name on every supported system; "mysql" stays the fallback.
//
// MariaDB 11 ve sonrası "mysql" adıyla başlatıldığında stderr'e uyarı yazar;
// birleşik çıktıda bu satır sahiplik kanıtını bozuyordu. "mariadb" kullanılır.
var mysqlStatementProgram = func() string {
	if path, err := exec.LookPath("mariadb"); err == nil {
		return path
	}
	return "mysql"
}

func newMySQLStatementCommand(statement string) *exec.Cmd {
	command := exec.Command(mysqlStatementProgram(), "--batch", "--skip-column-names")
	command.Stdin = strings.NewReader(statement + "\n")
	return command
}

func runMySQLStatement(statement string) ([]byte, error) {
	return newMySQLStatementCommand(statement).CombinedOutput()
}

var postgreSQLExecStatement = runPostgreSQLStatement

// R-062. psql took its statement in the argument list, and one of those
// statements is CREATE USER ... WITH PASSWORD. An argument list is public:
// every local account can read /proc/<pid>/cmdline for as long as the command
// runs, and a hosting panel's whole premise is other local accounts. The
// MariaDB side of this file already knew - newMySQLStatementCommand puts its
// statement on stdin, where nothing but the process itself can see it - so
// this is that shape, applied to the client that needed it more.
//
// ON_ERROR_STOP is not decoration here. With -c, psql exits non-zero when the
// statement fails; reading a statement from stdin it does not, and every
// caller below decides success from the exit status. Without this flag the
// move from -c to stdin would turn every failure into a silent success.
//
// --no-psqlrc for the same reason in a smaller way: a start-up file on the
// postgres account could otherwise change what this command does.
//
// R-062. psql ifadesini argüman listesinde alıyordu ve o ifadelerden biri
// CREATE USER ... WITH PASSWORD. Argüman listesi herkese açıktır: komut
// çalıştığı sürece her yerel hesap /proc/<pid>/cmdline dosyasını okuyabilir.
// Bu dosyanın MariaDB tarafı bunu zaten biliyordu; bu, o biçimin daha çok
// ihtiyaç duyan istemciye uygulanmasıdır. ON_ERROR_STOP süs değildir: stdin
// okuyan psql, ifade başarısız olsa da sıfır döner ve aşağıdaki her çağrı
// başarıyı çıkış kodundan okur.
func newPostgreSQLStatementCommand(statement string) *exec.Cmd {
	return newPostgreSQLStatementCommandOn(0, statement)
}

// newPostgreSQLStatementCommandOn is the same command for the cluster on one
// port (alpha.108: every PostgreSQL major runs as its own cluster on its own
// port). Port 0 is psql's own choice, as before.
func newPostgreSQLStatementCommandOn(port int, statement string) *exec.Cmd {
	args := []string{"-u", "postgres", "psql", "--no-psqlrc", "--set", "ON_ERROR_STOP=on"}
	if port > 0 {
		args = append(args, "--port", strconv.Itoa(port))
	}
	command := exec.Command("sudo", args...)
	command.Stdin = strings.NewReader(statement + "\n")
	return command
}

func runPostgreSQLStatement(statement string) ([]byte, error) {
	return newPostgreSQLStatementCommand(statement).CombinedOutput()
}

// postgreSQLExecStatementOnPort runs a statement on the cluster on one port
// (a variable for tests).
var postgreSQLExecStatementOnPort = func(port int, statement string) ([]byte, error) {
	return newPostgreSQLStatementCommandOn(port, statement).CombinedOutput()
}

// postgreSQLStatementFor is the statement runner for a request's port: the
// default runner for 0, the port's otherwise. A port outside 1..65535 is
// refused before anything runs. alpha.127: a cluster that does not run (its
// program removed, or stopped) is refused with its facts
// (pgClusterRefusalError), and 0 resolves to the running cluster where the
// clusters can be read (pg_statement_target.go).
func postgreSQLStatementFor(port int) (func(string) ([]byte, error), error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid PostgreSQL port %d", port)
	}
	port, err := resolvePGStatementPort(port)
	if err != nil {
		return nil, err
	}
	if port == 0 {
		return postgreSQLExecStatement, nil
	}
	return func(statement string) ([]byte, error) { return postgreSQLExecStatementOnPort(port, statement) }, nil
}

// Database Management RPC Methods

// CreateDatabaseRequest represents a request to create a database
type CreateDatabaseRequest = transport.CreateDatabaseRequest

// CreateDatabaseResponse represents the response from creating a database
type CreateDatabaseResponse = transport.CreateDatabaseResponse

// DeleteDatabaseRequest represents a request to delete a database
type DeleteDatabaseRequest = transport.DeleteDatabaseRequest

// DeleteDatabaseResponse represents the response from deleting a database
type DeleteDatabaseResponse = transport.DeleteDatabaseResponse

// CreateDatabase creates a new MySQL or PostgreSQL database
func (a *Agent) CreateDatabase(req CreateDatabaseRequest, resp *CreateDatabaseResponse) error {
	if req.Type == "mysql" {
		return a.createMySQLDatabase(req, resp)
	} else if req.Type == "postgresql" {
		return a.createPostgreSQLDatabase(req, resp)
	}

	resp.Success = false
	resp.Error = "invalid database type"
	return nil
}

// DeleteDatabase deletes a MySQL or PostgreSQL database
func (a *Agent) DeleteDatabase(req DeleteDatabaseRequest, resp *DeleteDatabaseResponse) error {
	if req.Type == "mysql" {
		return a.deleteMySQLDatabase(req, resp)
	} else if req.Type == "postgresql" {
		return a.deletePostgreSQLDatabase(req, resp)
	}

	resp.Success = false
	resp.Error = "invalid database type"
	return nil
}

// createMySQLDatabase creates a MySQL database and user
func (a *Agent) createMySQLDatabase(req CreateDatabaseRequest, resp *CreateDatabaseResponse) error {
	dbIdent, err := services.QuoteMySQLIdentifier(req.Name)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}
	// alpha.134: the grant names the database exactly (`shop\_a`.*); written
	// as `shop_a`.* it is a pattern that also covers shop1a, shopXa and so
	// on (internal/mariadbgrant). A name whose exact form is longer than a
	// grant row holds would be stored cut short - a right on another name -
	// so such a database is not made at all.
	grantTarget, err := mariadbgrant.Target(req.Name)
	if errors.Is(err, mariadbgrant.ErrExactTooLong) {
		resp.NameTooLong = true
		resp.Error = "the database name is too long for a right by its exact name; nothing was created"
		return nil
	}
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}
	if err := services.ValidateSQLIdentifier(req.User); err != nil {
		resp.Error = fmt.Sprintf("invalid username: %v", err)
		return nil
	}
	userLiteral, err := services.QuoteMySQLStringLiteral(req.User)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid username: %v", err)
		return nil
	}
	pwLiteral, err := services.QuoteMySQLStringLiteral(req.Password)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid password: %v", err)
		return nil
	}

	hasOperation := req.OperationID != "" || req.CleanupToken != ""
	if hasOperation &&
		(!validWordPressOperationID(req.OperationID) || !validWordPressCleanupToken(req.CleanupToken)) {
		resp.Error = "invalid database operation ownership proof"
		return nil
	}

	databaseCreated := false
	userCreated := false
	if _, err := mysqlExecStatement(fmt.Sprintf("CREATE DATABASE %s;", dbIdent)); err != nil {
		if !hasOperation || !verifyMySQLOwnership(dbIdent, req.OperationID, req.CleanupToken) {
			resp.Error = "database already exists or could not be created exclusively"
			if mysqlNameExists("SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + mysqlLiteral(req.Name) + ";") {
				resp.NameTaken = "database"
			}
			return nil
		}
		// A matching marker is written only after this operation exclusively
		// created both the database and user. A lost response can therefore
		// safely resume at the idempotent grant step.
		userCreated = true
	} else {
		databaseCreated = true
		if _, err := mysqlExecStatement(fmt.Sprintf(
			"CREATE USER %s@'localhost' IDENTIFIED BY %s;",
			userLiteral, pwLiteral,
		)); err != nil {
			resp.CleanupIncomplete = !cleanupCreatedMySQLResources(dbIdent, "", false, true)
			resp.Error = "database user already exists or could not be created exclusively"
			if mysqlNameExists("SELECT CONCAT('n=', COUNT(*)) FROM mysql.user WHERE User = " + userLiteral + ";") {
				resp.NameTaken = "user"
			}
			return nil
		}
		userCreated = true

		if hasOperation {
			operationLiteral, _ := services.QuoteMySQLStringLiteral(req.OperationID)
			hashLiteral, _ := services.QuoteMySQLStringLiteral(
				wordpressDatabaseOwnershipHash(req.OperationID, req.CleanupToken),
			)
			markerSQL := fmt.Sprintf(
				"CREATE TABLE %s.%s ("+
					"operation_id VARCHAR(32) NOT NULL PRIMARY KEY,"+
					"cleanup_token_hash CHAR(64) NOT NULL"+
					"); INSERT INTO %s.%s (operation_id, cleanup_token_hash) VALUES (%s, %s);",
				dbIdent, wordpressDatabaseOwnershipTable,
				dbIdent, wordpressDatabaseOwnershipTable,
				operationLiteral, hashLiteral,
			)
			if _, err := mysqlExecStatement(markerSQL); err != nil {
				resp.CleanupIncomplete = !cleanupCreatedMySQLResources(
					dbIdent, userLiteral, userCreated, databaseCreated,
				)
				resp.Error = "failed to persist database ownership proof"
				return nil
			}
		}
	}

	if _, err := mysqlGrantExactly("ALL PRIVILEGES", grantTarget, mariadbgrant.Exact(req.Name), userLiteral, req.User); err != nil {
		// A resumed operation already had a durable marker. Preserve it for
		// retry instead of deleting resources after an ambiguous prior result.
		if databaseCreated {
			resp.CleanupIncomplete = !cleanupCreatedMySQLResources(
				dbIdent, userLiteral, userCreated, databaseCreated,
			)
		} else {
			resp.CleanupIncomplete = true
		}
		resp.Error = "failed to grant database privileges"
		return nil
	}

	resp.Success = true
	resp.OwnedByOperation = hasOperation
	return nil
}

// mysqlNameExists answers a "n=<count>" question; an unreadable answer is
// "not known to exist" (the refusal then stays the general one).
func mysqlNameExists(statement string) bool {
	output, err := mysqlExecStatement(statement)
	if err != nil {
		return false
	}
	count, ok := markerValue(output, "n")
	return ok && count != "0"
}

func mysqlLiteral(value string) string {
	literal, _ := services.QuoteMySQLStringLiteral(value)
	return literal
}

func validWordPressCleanupToken(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func wordpressDatabaseOwnershipHash(operationID, cleanupToken string) string {
	digest := sha256.Sum256([]byte(operationID + "\x00" + cleanupToken))
	return hex.EncodeToString(digest[:])
}

func verifyMySQLOwnership(dbIdent, operationID, cleanupToken string) bool {
	operationLiteral, err := services.QuoteMySQLStringLiteral(operationID)
	if err != nil {
		return false
	}
	hashLiteral, err := services.QuoteMySQLStringLiteral(
		wordpressDatabaseOwnershipHash(operationID, cleanupToken),
	)
	if err != nil {
		return false
	}
	output, err := mysqlExecStatement(fmt.Sprintf(
		"SELECT COUNT(*) FROM %s.%s WHERE operation_id = %s AND cleanup_token_hash = %s;",
		dbIdent, wordpressDatabaseOwnershipTable, operationLiteral, hashLiteral,
	))
	return err == nil && strings.TrimSpace(string(output)) == "1"
}

func cleanupCreatedMySQLResources(
	dbIdent, userLiteral string,
	userCreated, databaseCreated bool,
) bool {
	complete := true
	if userCreated && userLiteral != "" {
		if _, err := mysqlExecStatement(fmt.Sprintf(
			"DROP USER IF EXISTS %s@'localhost';", userLiteral,
		)); err != nil {
			complete = false
		}
	}
	if databaseCreated {
		if _, err := mysqlExecStatement(fmt.Sprintf(
			"DROP DATABASE IF EXISTS %s;", dbIdent,
		)); err != nil {
			complete = false
		}
	}
	return complete
}

// deleteMySQLDatabase deletes a MySQL database and user
func (a *Agent) deleteMySQLDatabase(req DeleteDatabaseRequest, resp *DeleteDatabaseResponse) error {
	dbIdent, err := services.QuoteMySQLIdentifier(req.Name)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}

	if req.RequireOwnershipProof {
		if !validWordPressOperationID(req.OperationID) ||
			!validWordPressCleanupToken(req.CleanupToken) ||
			!verifyMySQLOwnership(dbIdent, req.OperationID, req.CleanupToken) {
			resp.Error = "database ownership proof did not match; destructive cleanup was refused"
			return nil
		}
	}

	// Normal database deletion remains tolerant for backward compatibility.
	// Transaction compensations can require proof that the user was dropped.
	if req.User != "" {
		userLiteral, quoteErr := services.QuoteMySQLStringLiteral(req.User)
		validateErr := services.ValidateSQLIdentifier(req.User)
		if quoteErr != nil || validateErr != nil {
			if req.RequireUserCleanup {
				resp.Success = false
				resp.Error = "invalid database user for required cleanup"
				return nil
			}
		} else {
			_, dropErr := mysqlExecStatement(fmt.Sprintf("DROP USER IF EXISTS %s@'localhost';", userLiteral))
			if dropErr == nil {
				// alpha.129: its remote copies go with it.
				if err := databaseRemoteFollowMariaDBUser(req.User, true); err != nil {
					log.Printf("database deletion: %v", err)
				}
			}
			if dropErr != nil && req.RequireUserCleanup {
				resp.Success = false
				resp.Error = "failed to drop database user"
				return nil
			}
		}
	}

	if _, err := mysqlExecStatement(fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbIdent)); err != nil {
		resp.Success = false
		resp.Error = "failed to drop database"
		return nil
	}

	resp.Success = true
	return nil
}

// postgreSQLDropDatabaseScript ends every other session on the database and
// drops it, in one psql run (autocommit: DROP DATABASE cannot run inside a
// transaction block).
func postgreSQLDropDatabaseScript(nameLiteral, ident string) string {
	return "SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE datname = " + nameLiteral +
		" AND pid <> pg_backend_pid();\nDROP DATABASE IF EXISTS " + ident + ";"
}

// R-053 stood the other way round from here. There, a client diagnostic was
// discarded and the panel could not say why an engine refused it. Here the
// diagnostic was forwarded whole - the error plus the command output -
// straight into the RPC response the browser renders, and the statement psql
// was running is
//
//	CREATE USER "name" WITH PASSWORD 'the password'
//
// which psql quotes back when it will not accept it. The output is still
// read, because "already exists" is decided from it and because a failure
// with no reason helps nobody; only what it meant travels.
//
// R-053 buradan bakildiginda tersine duruyordu. Orada bir istemcinin teshis
// metni atilmisti; burada ise tarayicinin gosterdigi RPC yanitina oldugu gibi
// aktariliyordu - ve psql-in calistirdigi ifade parolayi tasir. Cikti hala
// okunur; yalnizca ne anlama geldigi tasinir.
func databaseClientMeaning(text string) string {
	switch services.ClassifyDatabaseEngineRefusal(errors.New(text)) {
	case services.DatabaseEngineRefusalCredential:
		return "this server's database engine refused the credential CelikPanel presented"
	case services.DatabaseEngineRefusalUnreachable:
		return "nothing answered at this server's database engine"
	}
	lowered := strings.ToLower(text)
	switch {
	case strings.Contains(lowered, "already exists"):
		return "the database or the role this change names is already there"
	case strings.Contains(lowered, "does not exist"):
		return "the database or the role this change names is not on this server"
	case strings.Contains(lowered, "permission denied"),
		strings.Contains(lowered, "must be superuser"),
		strings.Contains(lowered, "must be owner"):
		return "the database engine would not let the account CelikPanel uses make this change"
	case strings.Contains(lowered, "syntax error"),
		strings.Contains(lowered, "invalid"):
		return "the database engine rejected the statement CelikPanel composed"
	case strings.Contains(lowered, "is being accessed by other users"):
		return "the database is still in use, so it could not be dropped"
	default:
		return ""
	}
}

// createPostgreSQLDatabase creates a PostgreSQL database and its user,
// exclusively (alpha.129). It used to take "role already exists" as success
// and then made the new database owned by that role: a role of the same name
// made outside CelikPanel (or by another account) became the owner of this
// account's database. Now an existing role or database changes nothing and
// the answer names it (NameTaken). A request that carries an operation's
// ownership proof (OperationID + CleanupToken, as the MySQL path) marks the
// role it makes with the proof's hash (COMMENT ON ROLE), so a replay of that
// same operation resumes on its own role and never adopts another one. A role
// this call made is dropped again when the database cannot be made.
//
// PostgreSQL veritabanı ve kullanıcısı artık yalnız yeni oluşturulur; var
// olan rol ya da veritabanı devralınmaz.
func (a *Agent) createPostgreSQLDatabase(req CreateDatabaseRequest, resp *CreateDatabaseResponse) error {
	userIdent, err := services.QuotePGIdentifier(req.User)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid username: %v", err)
		return nil
	}
	dbIdent, err := services.QuotePGIdentifier(req.Name)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}
	pwLiteral, err := services.QuotePGStringLiteral(req.Password)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid password: %v", err)
		return nil
	}
	userLiteral, _ := services.QuotePGStringLiteral(req.User)
	dbLiteral, _ := services.QuotePGStringLiteral(req.Name)
	hasOperation := req.OperationID != "" || req.CleanupToken != ""
	if hasOperation &&
		(!validWordPressOperationID(req.OperationID) || !validWordPressCleanupToken(req.CleanupToken)) {
		resp.Error = "invalid database operation ownership proof"
		return nil
	}
	marker := ""
	if hasOperation {
		marker = "celikpanel-operation:" + wordpressDatabaseOwnershipHash(req.OperationID, req.CleanupToken)
	}

	run, err := postgreSQLStatementFor(req.Port)
	if err != nil {
		resp.Error = err.Error()
		resp.Refusal = pgClusterRefusalOf(err)
		return nil
	}

	output, err := run("SELECT 'roles=' || count(*) FROM pg_catalog.pg_roles WHERE rolname = " + userLiteral + ";\n" +
		"SELECT 'mark=' || COALESCE((SELECT pg_catalog.shobj_description(oid, 'pg_authid') FROM pg_catalog.pg_roles WHERE rolname = " + userLiteral + "), '');\n" +
		"SELECT 'owner=' || COALESCE((SELECT pg_catalog.pg_get_userbyid(datdba) FROM pg_catalog.pg_database WHERE datname = " + dbLiteral + "), '');")
	if err != nil {
		resp.Error = hostcmd.Fail("the database server could not be asked about the names; nothing was changed", output, err, databaseClientMeaning).Error()
		return nil
	}
	roles, okRoles := markerValue(output, "roles")
	mark, okMark := markerValue(output, "mark")
	owner, okOwner := markerValue(output, "owner")
	if !okRoles || !okMark || !okOwner {
		resp.Error = "the database server's answer about the names could not be read; nothing was changed"
		return nil
	}
	// The role is this operation's own only when it carries the operation's
	// mark; anything else is somebody else's.
	resumed := roles != "0" && marker != "" && mark == marker
	switch {
	case roles != "0" && !resumed:
		resp.NameTaken = "user"
		resp.Error = "a database user named " + req.User + " already exists on this server; nothing was changed"
		return nil
	case owner != "" && !(resumed && owner == req.User):
		resp.NameTaken = "database"
		resp.Error = "a database named " + req.Name + " already exists on this server; nothing was changed"
		return nil
	}

	roleCreated := false
	if !resumed {
		statement := "SET password_encryption = 'scram-sha-256';\nCREATE ROLE " + userIdent + " LOGIN PASSWORD " + pwLiteral + ";"
		if marker != "" {
			markLiteral, _ := services.QuotePGStringLiteral(marker)
			statement = "SET password_encryption = 'scram-sha-256';\nBEGIN;\nCREATE ROLE " + userIdent + " LOGIN PASSWORD " + pwLiteral +
				";\nCOMMENT ON ROLE " + userIdent + " IS " + markLiteral + ";\nCOMMIT;"
		}
		if _, err := run(statement); err != nil {
			// The statement carries the password: its answer is not forwarded.
			resp.Error = "the database server did not create the user " + req.User + "; nothing was changed"
			return nil
		}
		roleCreated = true
	}

	if owner == "" {
		output, err = run(fmt.Sprintf("CREATE DATABASE %s OWNER %s;", dbIdent, userIdent))
		if err != nil {
			resp.Error = hostcmd.Fail("failed to create database", output, err, databaseClientMeaning).Error()
			if roleCreated {
				if _, dropErr := run("DROP ROLE IF EXISTS " + userIdent + ";"); dropErr != nil {
					resp.CleanupIncomplete = true
				}
			}
			return nil
		}
	}

	resp.Success = true
	resp.OwnedByOperation = hasOperation
	return nil
}

// deletePostgreSQLDatabase deletes a PostgreSQL database and user
func (a *Agent) deletePostgreSQLDatabase(req DeleteDatabaseRequest, resp *DeleteDatabaseResponse) error {
	dbIdent, err := services.QuotePGIdentifier(req.Name)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}

	run, err := postgreSQLStatementFor(req.Port)
	if err != nil {
		resp.Error = err.Error()
		resp.Refusal = pgClusterRefusalOf(err)
		return nil
	}

	// Drop database. alpha.133 round 5: the sessions open on it are ended
	// first, in the same psql run, as the panel's own driver always did
	// (internal/services/postgresql_driver.go DeleteDatabase); a plain DROP
	// failed on every press while a site still held a connection. Ending
	// backends works on every PostgreSQL major the systems ship (DROP ...
	// WITH (FORCE) would need 13 or later). The name is a value here.
	// Önce açık oturumlar sonlandırılır, sonra veritabanı silinir.
	nameLiteral, err := services.QuotePGStringLiteral(req.Name)
	if err != nil {
		resp.Error = fmt.Sprintf("invalid database name: %v", err)
		return nil
	}
	output, err := run(postgreSQLDropDatabaseScript(nameLiteral, dbIdent))
	if err != nil {
		resp.Success = false
		resp.Error = hostcmd.Fail("failed to drop database", output, err, databaseClientMeaning).Error()
		return nil
	}

	// Drop user
	if userIdent, err := services.QuotePGIdentifier(req.User); err == nil {
		// Don't fail if user doesn't exist
		_, _ = run(fmt.Sprintf("DROP USER IF EXISTS %s;", userIdent))
	}

	resp.Success = true
	return nil
}
