package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A database deletion bound to the database it was asked to delete
// (alpha.141 carried item 12; internal/transport/database_drop_once.go).
// The panel reads the identity first and keeps it with the deletion; the
// drop takes the database only while the engine still shows that identity,
// so pressing Delete again after a lost answer, a stop or a step that did
// not finish never drops a database of the same name made in the meantime.
//
// Silme, silinmesi istenen veritabanının kimliğine bağlıdır: ikinci basış,
// bu arada aynı adla açılmış veritabanını silmez.

// mariadbDatabaseDirectory reads one database directory's identity; tests
// replace it.
var mariadbDatabaseDirectory = databaseDirectoryIdentity

// mysqlPlainDirectoryName: MariaDB keeps a database of this name in a
// directory of exactly this name (letters, digits and underscores are not
// encoded); any other name gets no identity here.
func mysqlPlainDirectoryName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// mysqlDatabaseIdentity answers whether this machine's MariaDB has the
// database and, when the filesystem tells, its directory's identity. Read
// only.
func mysqlDatabaseIdentity(name string) (exists bool, identity string, err error) {
	literal, err := services.QuoteMySQLStringLiteral(name)
	if err != nil {
		return false, "", fmt.Errorf("invalid database name: %w", err)
	}
	output, err := mysqlExecStatement("SELECT CONCAT('n=', COUNT(*)) FROM information_schema.SCHEMATA WHERE BINARY SCHEMA_NAME = " + literal + ";\n" +
		"SELECT CONCAT('d=', @@datadir);\nSELECT CONCAT('l=', @@lower_case_table_names);")
	if err != nil {
		return false, "", hostcmd.Fail("the database server could not be asked about the database; nothing was changed", output, err, databaseClientMeaning)
	}
	count, ok := markerValue(output, "n")
	if !ok {
		return false, "", errors.New("the database server's answer about the database could not be read; nothing was changed")
	}
	if count == "0" {
		return false, "", nil
	}
	dataDir, okDir := markerValue(output, "d")
	lower, okLower := markerValue(output, "l")
	entry := name
	if okLower && lower != "0" {
		entry = strings.ToLower(name)
	}
	if !okDir || !okLower || !filepath.IsAbs(dataDir) || !mysqlPlainDirectoryName(entry) {
		return true, "", nil
	}
	return true, mariadbDatabaseDirectory(filepath.Join(dataDir, entry)), nil
}

// postgreSQLDatabaseIdentity answers whether the cluster on port has the
// database and its oid. Read only.
func postgreSQLDatabaseIdentity(port int, name string) (exists bool, identity string, refusal *transport.DatabaseClusterRefusal, err error) {
	literal, err := services.QuotePGStringLiteral(name)
	if err != nil {
		return false, "", nil, fmt.Errorf("invalid database name: %w", err)
	}
	run, err := postgreSQLStatementFor(port)
	if err != nil {
		return false, "", pgClusterRefusalOf(err), err
	}
	output, err := run("SELECT 'oid=' || COALESCE((SELECT oid::text FROM pg_catalog.pg_database WHERE datname = " + literal + "), '');")
	if err != nil {
		return false, "", nil, hostcmd.Fail("the database server could not be asked about the database; nothing was changed", output, err, databaseClientMeaning)
	}
	oid, ok := markerValue(output, "oid")
	if !ok {
		return false, "", nil, errors.New("the database server's answer about the database could not be read; nothing was changed")
	}
	if oid == "" {
		return false, "", nil, nil
	}
	return true, "postgresql-oid:" + oid, nil, nil
}

// databaseIdentityOf reads one database's identity on the engine of type.
func databaseIdentityOf(engine, name string, port int) (exists bool, identity string, refusal *transport.DatabaseClusterRefusal, err error) {
	switch engine {
	case "mysql":
		exists, identity, err = mysqlDatabaseIdentity(name)
		return exists, identity, nil, err
	case "postgresql":
		return postgreSQLDatabaseIdentity(port, name)
	}
	return false, "", nil, errors.New("invalid database type")
}

// DatabaseIdentityV1 reads whether a database exists on this machine's
// engine and its identity. Read only: it changes nothing.
func (a *Agent) DatabaseIdentityV1(req *transport.DatabaseIdentityRequest, resp *transport.DatabaseIdentityResponse) error {
	if req == nil || resp == nil {
		return errors.New("database identity request and response are required")
	}
	*resp = transport.DatabaseIdentityResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "reading a database's identity"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := services.ValidateSQLIdentifier(req.Name); err != nil {
		resp.Error = "invalid database name; nothing was read"
		return nil
	}
	exists, identity, refusal, err := databaseIdentityOf(req.Type, req.Name, req.Port)
	if refusal != nil {
		resp.Refusal, resp.Error = refusal, err.Error()
		return nil
	}
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	resp.Exists, resp.Identity = exists, identity
	return nil
}

// DropDatabaseOnceV1 drops a database only while it is the one identified
// (or, with ByName and no identity, the one of that name now). It reads the
// identity and drops in one call, under the lock the Agent's other database
// steps take, so nothing of this Agent comes between the two.
func (a *Agent) DropDatabaseOnceV1(req *transport.DropDatabaseOnceRequest, resp *transport.DropDatabaseOnceResponse) error {
	if req == nil || resp == nil {
		return errors.New("database drop request and response are required")
	}
	*resp = transport.DropDatabaseOnceResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "deleting a database"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if err := services.ValidateSQLIdentifier(req.Name); err != nil {
		resp.Error = "invalid database name; nothing was changed"
		return nil
	}
	if req.Identity == "" && !req.ByName {
		// A later press without an identity: what is there now cannot be
		// told from what the deletion was asked for.
		resp.Error = "no identity of the database to delete was given; nothing was changed"
		return nil
	}
	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()
	exists, identity, refusal, err := databaseIdentityOf(req.Type, req.Name, req.Port)
	switch {
	case refusal != nil:
		resp.Refusal, resp.Error = refusal, err.Error()
		return nil
	case err != nil:
		resp.Error = err.Error()
		return nil
	case !exists:
		resp.Absent = true
		return nil
	case req.Identity != "" && identity != req.Identity:
		// Another database of that name (or one whose identity cannot be
		// read now): never dropped by this deletion.
		resp.Replaced = true
		return nil
	}
	switch req.Type {
	case "mysql":
		ident, err := services.QuoteMySQLIdentifier(req.Name)
		if err != nil {
			resp.Error = "invalid database name; nothing was changed"
			return nil
		}
		if output, err := mysqlExecStatement("DROP DATABASE IF EXISTS " + ident + ";"); err != nil {
			dropFailed(req, resp, hostcmd.Fail("failed to drop database", output, err, databaseClientMeaning))
			return nil
		}
	case "postgresql":
		ident, err := services.QuotePGIdentifier(req.Name)
		if err != nil {
			resp.Error = "invalid database name; nothing was changed"
			return nil
		}
		literal, _ := services.QuotePGStringLiteral(req.Name)
		run, err := postgreSQLStatementFor(req.Port)
		if err != nil {
			resp.Refusal, resp.Error = pgClusterRefusalOf(err), err.Error()
			return nil
		}
		if output, err := run(postgreSQLDropDatabaseScript(literal, ident)); err != nil {
			dropFailed(req, resp, hostcmd.Fail("failed to drop database", output, err, databaseClientMeaning))
			return nil
		}
	default:
		resp.Error = "invalid database type"
		return nil
	}
	resp.Dropped = true
	return nil
}

// dropFailed answers a drop statement that failed. A client that lost its
// connection during the statement cannot say whether the engine finished it:
// the database is looked for again, and gone means dropped (D-024: the result
// is read, not guessed). Still there, it is a verified failure.
func dropFailed(req *transport.DropDatabaseOnceRequest, resp *transport.DropDatabaseOnceResponse, cause error) {
	exists, _, _, err := databaseIdentityOf(req.Type, req.Name, req.Port)
	if err == nil && !exists {
		resp.Dropped = true
		return
	}
	resp.Error = cause.Error()
}
