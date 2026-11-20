package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// A move loads each owner's rows with that owner's rights (security lane
// S1b, 20 November 2026).
//
// PostgreSQL runs code the database's users wrote while rows are loaded:
// a table's CHECK constraints and the CHECK constraints of a domain a column
// uses are evaluated for every row COPY writes, and stored generated columns
// are computed (COPY: "COPY FROM will invoke any triggers and check
// constraints on the destination table"; CREATE TABLE: GENERATED ALWAYS AS
// … STORED, CHECK; CREATE DOMAIN: CHECK). That code runs as the session's
// current user. The move used to restore the whole archive as the cluster's
// owner (a superuser), so a function a customer placed in a constraint ran
// with the administrator's rights (measured on PostgreSQL 17.12). The other
// places user code runs in a restore are run by PostgreSQL itself as the
// object's owner inside a security-restricted operation (index expressions
// and predicates of CREATE INDEX and exclusion constraints, REFRESH
// MATERIALIZED VIEW, the foreign-key checks; release notes of 14.3/13.7/
// 12.11/11.16/10.21, CVE-2022-1552), are not run by a restore (triggers are
// created after the data; rules are not fired by COPY; column defaults are
// not evaluated because every column is given), or come from the server's
// own files (CREATE EXTENSION; CVE-2022-2625 fixed in 14.5/13.8/12.12/11.17).
//
// So the restore is split by section:
//   - pre-data (the database, its objects, owners, privileges, comments,
//     settings) and post-data (indexes, constraints that are not inline
//     CHECKs, triggers, materialized views' data) as the cluster's owner, as
//     before: nothing of the users' runs with its rights there;
//   - every table's rows as that table's owner: a role made for the move
//     signs in over 127.0.0.1 with a random password, holds membership in
//     the owner but does not inherit it (NOINHERIT), and the restore runs
//     SET ROLE owner. Code that resets the role falls back to that
//     move role, which holds nothing. The role is removed after the load;
//   - the other data (large objects, sequence values, an extension's own
//     configuration tables) as the cluster's owner: no user code runs there.
//
// Refused before anything is changed: a table, partitioned table or
// materialized view owned by an administrator role (not part of an
// extension), whose code would run with administrator rights; PostgreSQL
// clients older than the August 2025 minors (their dump can carry crafted
// object names into the restore's SQL: CVE-2025-8714, CVE-2025-8715) and a
// target server older than the August 2022 minors; a table or schema name
// with white space (the archive's list cannot be read safely for it); a
// target that does not let the move role sign in with a password on
// 127.0.0.1.
//
// Taşıma, her tablonun satırlarını o tablonun sahibinin haklarıyla yükler;
// yöneticinin haklarıyla kullanıcı kodu çalışmaz.

// pgMoveClientMinimum: the first minor of each major whose pg_dump writes
// object names safely (August 2025).
var pgMoveClientMinimum = map[int]int{13: 22, 14: 19, 15: 14, 16: 10, 17: 6}

// pgMoveServerMinimum: the first minor of each major that runs index and
// materialized-view code as the owner and installs extensions safely
// (August 2022). 15 and later have it from their first release.
var pgMoveServerMinimum = map[int]int{13: 8, 14: 5}

var pgClientVersionRe = regexp.MustCompile(`\(PostgreSQL\) (\d+)\.(\d+)`)

// pgClientVersionSafe reads "pg_dump (PostgreSQL) 17.12 …".
func pgClientVersionSafe(text string) (string, bool) {
	m := pgClientVersionRe.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	version := m[1] + "." + m[2]
	if major >= 18 {
		return version, true
	}
	least, known := pgMoveClientMinimum[major]
	return version, known && minor >= least
}

// pgServerVersionSafe reads server_version_num (170012).
func pgServerVersionSafe(text string) (string, bool) {
	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 100000 {
		return "", false
	}
	major, minor := num/10000, num%10000
	version := strconv.Itoa(major) + "." + strconv.Itoa(minor)
	if major >= 15 {
		return version, true
	}
	least, known := pgMoveServerMinimum[major]
	return version, known && minor >= least
}

// pgAdministratorObjectsStatement lists, in the moved database, tables,
// partitioned tables and materialized views an administrator role owns
// that are not part of an extension.
const pgAdministratorObjectsStatement = `SELECT pg_catalog.quote_ident(n.nspname) OPERATOR(pg_catalog.||) '.' OPERATOR(pg_catalog.||) pg_catalog.quote_ident(c.relname) ` +
	`FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid OPERATOR(pg_catalog.=) c.relnamespace ` +
	`JOIN pg_catalog.pg_roles r ON r.oid OPERATOR(pg_catalog.=) c.relowner ` +
	`WHERE r.rolsuper AND c.relkind OPERATOR(pg_catalog.=) ANY (ARRAY['r','p','m']::"char"[]) ` +
	`AND n.nspname OPERATOR(pg_catalog.<>) ALL (ARRAY['pg_catalog','information_schema']) AND n.nspname NOT LIKE 'pg\_toast%' ` +
	`AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid OPERATOR(pg_catalog.=) 'pg_catalog.pg_class'::pg_catalog.regclass ` +
	`AND d.objid OPERATOR(pg_catalog.=) c.oid AND d.deptype OPERATOR(pg_catalog.=) 'e') ORDER BY 1 LIMIT 5;`

// pgMoveRefusal is a move refused before anything was changed.
type pgMoveRefusal struct{ code, message string }

// pgMoveSafety checks, before anything is changed, that the move can load
// every table's rows with its owner's rights.
func pgMoveSafety(ctx context.Context, host pgClusterHost, source, target transport.PostgreSQLCluster, database string) *pgMoveRefusal {
	version := &pgBuffer{limit: 4 << 10}
	if err := host.stream(ctx, target.Owner, "pg_dump", nil, version, "--version"); err != nil {
		return &pgMoveRefusal{"CLIENT_UNKNOWN", fmt.Sprintf("the version of this server's PostgreSQL client programs could not be read (%s); nothing was changed", boundedText(err.Error(), 160))}
	}
	if found, ok := pgClientVersionSafe(version.String()); !ok {
		return &pgMoveRefusal{"CLIENT_OUTDATED", fmt.Sprintf("this server's PostgreSQL client programs (%s) are older than the August 2025 security updates, which a move needs; the server administrator updates the PostgreSQL packages, then moves again; nothing was changed", boundedText(strings.TrimSpace(found), 40))}
	}
	out, err := host.query(ctx, target.Owner, target.Port, "postgres", "SHOW server_version_num;")
	if err != nil {
		return &pgMoveRefusal{"TARGET_UNREADABLE", fmt.Sprintf("cluster %s did not answer (%s); nothing was changed", pgClusterRef(target.Version, target.Name), boundedText(err.Error(), 160))}
	}
	if found, ok := pgServerVersionSafe(strings.Join(pgRows(out), "")); !ok {
		return &pgMoveRefusal{"TARGET_OUTDATED", fmt.Sprintf("PostgreSQL %s on cluster %s is older than the security updates a move needs; the server administrator updates its packages, then moves again; nothing was changed", found, pgClusterRef(target.Version, target.Name))}
	}
	out, err = host.query(ctx, source.Owner, source.Port, database, pgAdministratorObjectsStatement)
	if err != nil {
		return &pgMoveRefusal{"SOURCE_UNREADABLE", fmt.Sprintf("the owners of %s's tables could not be read (%s); nothing was changed", database, boundedText(err.Error(), 160))}
	}
	if rows := pgRows(out); len(rows) > 0 {
		return &pgMoveRefusal{"ADMINISTRATOR_OBJECTS", fmt.Sprintf("%s holds tables owned by an administrator role (%s); a move loads every table with its owner's rights and never runs a database's code with administrator rights. The server administrator gives them to the database's owner (ALTER TABLE … OWNER TO) or moves the database by hand; nothing was changed", database, boundedText(strings.Join(rows, ", "), 200))}
	}
	return nil
}

// pgTableDataRe is one table's rows in pg_restore --list:
// "123; 0 16390 TABLE DATA public orders shop_user".
var pgTableDataRe = regexp.MustCompile(`^(\d+); \d+ \d+ TABLE DATA (\S+) (\S+) (\S+)$`)

// pgMoveLists splits an archive's list: each owner's table rows, and every
// other entry (restored as the cluster's owner, where the data section holds
// no user code: large objects, sequence values, an extension's own tables).
func pgMoveLists(list string, administrators map[string]bool) (byOwner map[string][]string, rest []string, err error) {
	byOwner = map[string][]string{}
	for _, raw := range strings.Split(list, "\n") {
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if !strings.Contains(line, " TABLE DATA ") {
			rest = append(rest, line)
			continue
		}
		m := pgTableDataRe.FindStringSubmatch(line)
		if m == nil {
			return nil, nil, fmt.Errorf("a table's name holds white space, so the archive's list cannot be read safely for it")
		}
		owner := m[4]
		if administrators[owner] {
			// Only an extension's own tables reach here (pgMoveSafety).
			rest = append(rest, line)
			continue
		}
		byOwner[owner] = append(byOwner[owner], line)
	}
	return byOwner, rest, nil
}

func randomPGMoveSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// pgOwnerRestore restores the archive in the target with each owner's rights
// for the rows (see the top of this file). file is the archive, seekable;
// administrators are the target's administrator roles. On an error the
// caller removes the database; the move role is always removed here.
func pgOwnerRestore(ctx context.Context, host pgClusterHost, target transport.PostgreSQLCluster, database string, file *os.File, administrators map[string]bool) (err error) {
	port := strconv.Itoa(target.Port)
	rewind := func() error {
		_, err := file.Seek(0, io.SeekStart)
		return err
	}
	listing := &pgBuffer{limit: 32 << 20}
	if err := rewind(); err != nil {
		return err
	}
	if err := host.stream(ctx, target.Owner, "pg_restore", file, listing, "--list"); err != nil {
		return fmt.Errorf("the archive's list could not be read: %w", err)
	}
	byOwner, rest, err := pgMoveLists(listing.String(), administrators)
	if err != nil {
		return err
	}
	dir, err := host.privateDir(target.Owner)
	if err != nil {
		return fmt.Errorf("a private directory for the restore could not be made: %w", err)
	}
	defer os.RemoveAll(dir)
	writeList := func(name string, lines []string) (string, error) {
		path := filepath.Join(dir, name)
		// Table names only; the directory itself is the cluster owner's alone.
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}

	// The move role, and its sign-in proved before anything is restored.
	secret, err := randomPGMoveSecret(24)
	if err != nil {
		return err
	}
	role := "celikpanel_move_" + secret[:12]
	password := secret[12:]
	roleIdent := quotePGIdent(role)
	if _, err := host.query(ctx, target.Owner, target.Port, "postgres", "SET password_encryption = 'scram-sha-256';\nCREATE ROLE "+roleIdent+
		" LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 4 PASSWORD '"+password+"';"); err != nil {
		return fmt.Errorf("the role that loads each owner's rows could not be made on the target")
	}
	defer func() {
		dropCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		ident, _ := services.QuotePGIdentifier(database)
		_, _ = host.query(dropCtx, target.Owner, target.Port, "postgres", "REVOKE ALL ON DATABASE "+ident+" FROM "+roleIdent+";")
		// Anything made while the role was reset to itself (it holds no
		// rights, but PUBLIC may create in public before PostgreSQL 15).
		_, _ = host.query(dropCtx, target.Owner, target.Port, database, "DROP OWNED BY "+roleIdent+";")
		if _, dropErr := host.query(dropCtx, target.Owner, target.Port, "postgres", "DROP ROLE IF EXISTS "+roleIdent+";"); dropErr != nil && err == nil {
			err = fmt.Errorf("the role %s made for the move could not be removed from cluster %s; the server administrator removes it", role, pgClusterRef(target.Version, target.Name))
		}
	}()
	probe := &pgBuffer{limit: 4 << 10}
	if err := host.streamLogin(ctx, target.Owner, "psql", password, nil, probe, "--no-psqlrc", "--no-align", "--tuples-only", "--host", "127.0.0.1",
		"--port", port, "--username", role, "--dbname", "postgres", "--command", "SELECT 1"); err != nil {
		return &pgMoveLoginError{cluster: pgClusterRef(target.Version, target.Name)}
	}

	// Pre-data: the database and its objects, as the cluster's owner.
	if err := rewind(); err != nil {
		return err
	}
	if err := host.stream(ctx, target.Owner, "pg_restore", file, io.Discard, "--port", port, "--section", "pre-data", "--create", "--exit-on-error", "--no-password", "--dbname", "postgres"); err != nil {
		return fmt.Errorf("pg_restore (definitions) into cluster %s stopped (%s)", pgClusterRef(target.Version, target.Name), boundedText(err.Error(), 240))
	}

	// Each owner's rows, with that owner's rights.
	owners := make([]string, 0, len(byOwner))
	for owner := range byOwner {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	dbIdent, _ := services.QuotePGIdentifier(database)
	grants := []string{"GRANT CONNECT ON DATABASE " + dbIdent + " TO " + roleIdent + ";"}
	for _, owner := range owners {
		if !pgRoleNameRe.MatchString(owner) {
			return fmt.Errorf("role %q has a name CelikPanel does not carry", boundedText(owner, 64))
		}
		grants = append(grants, "GRANT "+quotePGIdent(owner)+" TO "+roleIdent+";")
	}
	if _, err := host.query(ctx, target.Owner, target.Port, "postgres", strings.Join(grants, "\n")); err != nil {
		return fmt.Errorf("the move role could not be given the owners' membership")
	}
	for i, owner := range owners {
		path, err := writeList("owner-"+strconv.Itoa(i)+".list", byOwner[owner])
		if err != nil {
			return err
		}
		if err := rewind(); err != nil {
			return err
		}
		if err := host.streamLogin(ctx, target.Owner, "pg_restore", password, file, io.Discard, "--host", "127.0.0.1", "--port", port,
			"--username", role, "--role", owner, "--section", "data", "--use-list", path, "--single-transaction", "--exit-on-error", "--no-password", "--dbname", database); err != nil {
			return fmt.Errorf("the rows of the tables %s owns could not be loaded with its rights (%s)", owner, boundedText(err.Error(), 240))
		}
	}

	// The rest of the data, as the cluster's owner (no user code runs).
	if len(rest) > 0 {
		path, err := writeList("rest.list", rest)
		if err != nil {
			return err
		}
		if err := rewind(); err != nil {
			return err
		}
		if err := host.stream(ctx, target.Owner, "pg_restore", file, io.Discard, "--port", port, "--section", "data", "--use-list", path,
			"--single-transaction", "--exit-on-error", "--no-password", "--dbname", database); err != nil {
			return fmt.Errorf("pg_restore (large objects and sequences) stopped (%s)", boundedText(err.Error(), 240))
		}
	}

	// Post-data: indexes, constraints, triggers, materialized views.
	if err := rewind(); err != nil {
		return err
	}
	if err := host.stream(ctx, target.Owner, "pg_restore", file, io.Discard, "--port", port, "--section", "post-data", "--exit-on-error", "--no-password", "--dbname", database); err != nil {
		return fmt.Errorf("pg_restore (indexes and constraints) stopped (%s)", boundedText(err.Error(), 240))
	}
	return nil
}

// pgMoveLoginError: the target does not let the move role sign in.
type pgMoveLoginError struct{ cluster string }

func (e *pgMoveLoginError) Error() string {
	return "cluster " + e.cluster + " does not let a role sign in with a password on 127.0.0.1 (pg_hba.conf), which a move needs to load each owner's rows with that owner's rights; the server administrator allows it (host all all 127.0.0.1/32 scram-sha-256), then moves again; nothing was copied"
}

// pgAdministratorRoles are the target's administrator roles.
func pgAdministratorRoles(ctx context.Context, host pgClusterHost, target transport.PostgreSQLCluster) (map[string]bool, error) {
	out, err := host.query(ctx, target.Owner, target.Port, "postgres", "SELECT rolname FROM pg_catalog.pg_roles WHERE rolsuper ORDER BY 1;")
	if err != nil {
		return nil, err
	}
	roles := map[string]bool{}
	for _, name := range pgRows(out) {
		roles[strings.TrimSpace(name)] = true
	}
	return roles, nil
}
