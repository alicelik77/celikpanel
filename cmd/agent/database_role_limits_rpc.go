package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/hostcmd"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/systemsqlite"
	"github.com/alicelik/celikpanel/internal/transport"
)

// PostgreSQL role limits of a site's database user (alpha.149 fix round B).
//
// The panel connects to PostgreSQL as celikpanel_admin (LOGIN CREATEDB
// CREATEROLE, deliberately not a superuser). PostgreSQL 16 and later let such
// a role ALTER only the roles it holds ADMIN OPTION on, which it gets only for
// roles it created itself; every site role the Agent creates (Databases,
// apps, copies, added users, cluster moves) was therefore out of its reach,
// and temp_file_limit needs a superuser on every version. The panel's own
// account gains nothing here. Instead this one call runs as the postgres
// superuser and is narrow:
//
//   - the role name must be a plain SQL identifier, never an engine or
//     CelikPanel account (root, mysql, postgres, celikpanel_*, pg_*);
//   - the panel must have recorded it: a row of database_users on a
//     PostgreSQL server record of this machine, at this cluster (its port,
//     or the cluster the record names), read from the panel's own database
//     file, read only;
//   - on the engine, in the same transaction as the writes: the role passes
//     the shared account rule (database_account_guard.go: not a superuser,
//     none of CREATEROLE, CREATEDB, REPLICATION or BYPASSRLS, no membership
//     of such a role or of a pg_* role) and, for this call, is a member of
//     no role at all;
//   - only CONNECTION LIMIT and the role-wide statement_timeout,
//     idle_in_transaction_session_timeout and temp_file_limit, each an
//     integer in a fixed range; the Agent builds every statement, the role's
//     name quoted by the server itself (format %I). No SQL text comes from
//     the panel, and settings for one database are never touched.
//
// Every call is written to the Agent's journal (role, port, keys, outcome).
//
// Site rol sınırları Agent üzerinden, postgres süper kullanıcısıyla, dar bir
// çağrıyla: rol panelin kaydında olmalı, ayrıcalıklı olmamalı; yalnız dört
// ayar, sabit aralıkta tam sayı; ifadeleri Agent kurar.

// pgRoleLimitRange is the range of one key's value.
var pgRoleLimitRange = map[string]struct {
	parameter, unit string
	max             int
}{
	transport.PGRoleLimitConnection:               {"", "", services.MaxUserConnections},
	transport.PGRoleLimitStatementSeconds:         {"statement_timeout", "s", services.MaxRoleLimitSeconds},
	transport.PGRoleLimitIdleInTransactionSeconds: {"idle_in_transaction_session_timeout", "s", services.MaxRoleLimitSeconds},
	transport.PGRoleLimitTempFileMB:               {"temp_file_limit", "MB", services.MaxRoleTempFileMB},
}

// pgRoleSettingKeys are the role settings in their order (the read markers).
var pgRoleSettingKeys = []string{
	transport.PGRoleLimitStatementSeconds, transport.PGRoleLimitIdleInTransactionSeconds, transport.PGRoleLimitTempFileMB,
}

// pgRoleLimitsReserved: an engine or CelikPanel account, never touched.
func pgRoleLimitsReserved(role string) bool {
	return suspensionReservedDatabaseUser(role) || strings.HasPrefix(strings.ToLower(role), "celikpanel")
}

// validPGRoleLimitsRequest checks the name and the writes before anything is
// read; a refusal names the reason.
func validPGRoleLimitsRequest(req *transport.PostgreSQLRoleLimitsRequest) (string, error) {
	if err := services.ValidateSQLIdentifier(req.Role); err != nil {
		return transport.PGRoleLimitRefusedInvalid, fmt.Errorf("invalid database user name: %w", err)
	}
	if pgRoleLimitsReserved(req.Role) {
		return transport.PGRoleLimitRefusedReserved, fmt.Errorf("%s is an engine or CelikPanel account; its settings are never changed here", req.Role)
	}
	if req.Port <= 0 || req.Port > 65535 {
		return transport.PGRoleLimitRefusedInvalid, fmt.Errorf("invalid PostgreSQL port %d", req.Port)
	}
	if len(req.Writes) > len(pgRoleLimitRange) {
		return transport.PGRoleLimitRefusedInvalid, errors.New("too many settings in one request")
	}
	seen := map[string]bool{}
	for _, write := range req.Writes {
		limit, ok := pgRoleLimitRange[write.Key]
		if !ok {
			return transport.PGRoleLimitRefusedInvalid, fmt.Errorf("unknown setting %q", write.Key)
		}
		if seen[write.Key] {
			return transport.PGRoleLimitRefusedInvalid, fmt.Errorf("setting %q named twice", write.Key)
		}
		seen[write.Key] = true
		if write.Value < 0 || write.Value > limit.max {
			return transport.PGRoleLimitRefusedInvalid, fmt.Errorf("%s must be 0 to %d", write.Key, limit.max)
		}
	}
	return "", nil
}

// pgRoleRecordedByPanel: whether the panel's own database records the role
// on a PostgreSQL server record of this machine whose cluster is the one at
// port (a variable for tests). An unreadable file is an error: the role is
// then not touched.
//
// What this check is, and is not (alpha.149, review of PR #370 finding 2):
// the panel writes that file, so a record in it guards against the panel's
// own mistakes (a name it never made, another cluster's role) - not against
// a compromised panel, which can write any record. The boundary against a
// compromised panel is the engine-side account rule in the same transaction
// as the writes (database_account_guard.go). The file is read as root, so it
// is opened through the managed-file checks (internal/systemsqlite: no
// symbolic link, a regular file with one link, never a FIFO, owned by the
// panel's account or root), within a time limit, one read at a time.
//
// Panelin kaydı panelin kendi hatalarına karşıdır, ele geçirilmiş panele
// karşı değil; sınır motor tarafındaki hesap kuralıdır.
var pgRoleRecordedByPanel = func(ctx context.Context, role string, port int) (bool, error) {
	return pgRoleRecordedIn(ctx, panelDatabasePath(), role, port, pgStatementClusters)
}

// panelRecordReadTimeout bounds one read of the panel's database file.
const panelRecordReadTimeout = 5 * time.Second

// panelRecordRead lets one read of the panel's file run at a time, so a read
// that SQLite's own open could hold (a file swapped after the checks) never
// piles up: the next call is refused until it ends.
var panelRecordRead = make(chan struct{}, 1)

// panelDatabaseOwnerUIDs are the accounts that may own the panel's database
// file: the panel's service account and root (a variable for tests).
var panelDatabaseOwnerUIDs = func() []uint32 {
	owners := []uint32{0}
	if account, err := user.Lookup("celikpanel"); err == nil {
		if uid, err := strconv.ParseUint(account.Uid, 10, 32); err == nil {
			owners = append(owners, uint32(uid))
		}
	}
	return owners
}

func pgRoleRecordedIn(ctx context.Context, path, role string, port int, clusters func() ([]transport.PostgreSQLCluster, bool)) (bool, error) {
	select {
	case panelRecordRead <- struct{}{}:
	default:
		return false, errors.New("an earlier read of CelikPanel's own database has not ended")
	}
	type answer struct {
		found bool
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		defer func() { <-panelRecordRead }()
		found, err := pgRoleRecordedInFile(path, role, port, clusters)
		done <- answer{found, err}
	}()
	timer := time.NewTimer(panelRecordReadTimeout)
	defer timer.Stop()
	select {
	case got := <-done:
		return got.found, got.err
	case <-timer.C:
		return false, errors.New("CelikPanel's own database did not answer in time")
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func pgRoleRecordedInFile(path, role string, port int, clusters func() ([]transport.PostgreSQLCluster, bool)) (bool, error) {
	pinned, err := systemsqlite.OpenPinnedReadOnly(path, panelDatabaseOwnerUIDs()...)
	if err != nil {
		return false, err
	}
	defer pinned.Close()
	ctx, cancel := context.WithTimeout(context.Background(), panelRecordReadTimeout)
	defer cancel()
	found, err := pgRoleRecordedInDatabase(ctx, pinned.Path(), role, port, clusters)
	if err != nil {
		return false, err
	}
	// The name still holds the file that passed the checks.
	if err := pinned.Verify(); err != nil {
		return false, err
	}
	return found, nil
}

func pgRoleRecordedInDatabase(ctx context.Context, path, role string, port int, clusters func() ([]transport.PostgreSQLCluster, bool)) (bool, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return false, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.QueryContext(ctx, `
		SELECT s.port, COALESCE(s.cluster, '') FROM database_users u
		JOIN database_servers s ON s.id = u.server_id
		JOIN database_server_types t ON t.id = s.type_id
		WHERE u.username = ? AND t.name = 'postgresql'
		  AND LOWER(TRIM(COALESCE(s.host, ''))) IN ('', 'localhost', '127.0.0.1', '::1', '[::1]')`, role)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var named []string
	for rows.Next() {
		var recordPort int
		var cluster string
		if err := rows.Scan(&recordPort, &cluster); err != nil {
			return false, err
		}
		if recordPort == port {
			return true, nil
		}
		if cluster != "" {
			named = append(named, cluster)
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(named) == 0 {
		return false, nil
	}
	// A record that names its cluster (schema 46) is reached at that
	// cluster's own port, which may differ from the port stored with it.
	list, ok := clusters()
	if !ok {
		return false, nil
	}
	for _, cluster := range list {
		for _, ref := range named {
			if pgClusterRef(cluster.Version, cluster.Name) == ref && cluster.Port == port {
				return true, nil
			}
		}
	}
	return false, nil
}

// The bounds of one limits call (review of PR #370, finding 9): a role lock
// is waited on at most 5 s, one statement runs at most 15 s, and the client
// is ended 30 s after it starts (it gives up connecting after 10 s,
// pgAdminPSQLArgsFor).
const (
	pgRoleLimitsApplication   = "celikpanel-role-limits"
	pgRoleLimitsBound         = 30 * time.Second
	pgRoleLimitsSessionBounds = "SET lock_timeout = '5s';\nSET statement_timeout = '15s';\n"
)

// pgRoleLimitsRun runs the script on the cluster at port, bounded (a
// variable for tests).
var pgRoleLimitsRun = func(port int, script string) ([]byte, error) {
	return pgBoundedRun(port, pgRoleLimitsApplication, time.Now().Add(pgRoleLimitsBound), script)
}

// pgClientDidNotConnect: psql's own report that it made no connection (its
// "psql: error:" line comes before any statement).
func pgClientDidNotConnect(output []byte) bool {
	lowered := strings.ToLower(string(output))
	return strings.Contains(lowered, "psql: error: connection to server") || strings.Contains(lowered, "could not connect to server")
}

// pgServerRunning establishes whether the cluster at port runs, from its
// service state (pg_lsclusters) or, where that cannot be read, from its
// socket; known is false when neither answers (a variable for tests).
var pgServerRunning = func(port int) (running, known bool) {
	if clusters, ok := pgStatementClusters(); ok {
		for _, cluster := range clusters {
			if cluster.Port == port {
				return cluster.Online, true
			}
		}
	}
	info, err := os.Stat(filepath.Join("/var/run/postgresql", ".s.PGSQL."+strconv.Itoa(port)))
	switch {
	case err == nil:
		return info.Mode()&os.ModeSocket != 0, true
	case errors.Is(err, os.ErrNotExist):
		return false, true
	}
	return false, false
}

// pgRoleLimitsScript is the whole statement: the role's check and the writes
// in one transaction (a DO block), then the values read back as markers.
func pgRoleLimitsScript(role string, writes []transport.PostgreSQLRoleLimitWrite) (string, error) {
	literal, err := services.QuotePGStringLiteral(role)
	if err != nil {
		return "", err
	}
	// alpha.149 (security): the shared account rule
	// (database_account_guard.go), and for this call a member of no role at
	// all - no site role CelikPanel makes is one (finding 2 of the review of
	// PR #370: that covers the predefined pg_* roles and any group the owner
	// made), in the same transaction as the writes.
	var b strings.Builder
	b.WriteString("DO $celikpanel_role_limits$\nDECLARE\n  target pg_catalog.pg_roles%ROWTYPE;\n  state pg_catalog.text;\nBEGIN\n")
	b.WriteString("  state := " + pgAccountStateExpr(literal, pgAccountNoMembership) + ";\n")
	b.WriteString("  IF state OPERATOR(pg_catalog.<>) 'ok' THEN RAISE EXCEPTION 'celikpanel-refused:%', state; END IF;\n")
	b.WriteString("  SELECT * INTO target FROM pg_catalog.pg_roles WHERE rolname OPERATOR(pg_catalog.=) " + literal + ";\n")
	b.WriteString("  IF NOT FOUND THEN RAISE EXCEPTION 'celikpanel-refused:absent'; END IF;\n")
	for _, write := range writes {
		limit := pgRoleLimitRange[write.Key]
		switch {
		case write.Key == transport.PGRoleLimitConnection:
			value := write.Value
			if value == 0 {
				value = -1
			}
			b.WriteString("  EXECUTE pg_catalog.format('ALTER ROLE %I CONNECTION LIMIT " + strconv.Itoa(value) + "', target.rolname);\n")
		case write.Value == 0:
			b.WriteString("  EXECUTE pg_catalog.format('ALTER ROLE %I RESET " + limit.parameter + "', target.rolname);\n")
		default:
			b.WriteString("  EXECUTE pg_catalog.format('ALTER ROLE %I SET " + limit.parameter + " = %L', target.rolname, '" +
				strconv.Itoa(write.Value) + limit.unit + "');\n")
		}
	}
	b.WriteString("END\n$celikpanel_role_limits$;\n")
	b.WriteString("SELECT 'conn=' || r.rolconnlimit FROM pg_catalog.pg_roles r WHERE r.rolname OPERATOR(pg_catalog.=) " + literal + ";\n")
	for _, key := range pgRoleSettingKeys {
		parameter := pgRoleLimitRange[key].parameter
		b.WriteString("SELECT '" + key + "=' || COALESCE((SELECT pg_catalog.substr(c, " + strconv.Itoa(len(parameter)+2) + ") " +
			"FROM pg_catalog.pg_db_role_setting s, pg_catalog.unnest(s.setconfig) c " +
			"WHERE s.setrole OPERATOR(pg_catalog.=) (SELECT oid FROM pg_catalog.pg_roles WHERE rolname OPERATOR(pg_catalog.=) " + literal + ") " +
			"AND s.setdatabase OPERATOR(pg_catalog.=) 0 AND pg_catalog.starts_with(c, '" + parameter + "=') LIMIT 1), '');\n")
	}
	return b.String(), nil
}

// pgRoleLimitsRefusalIn reads the DO block's own refusal from psql's output.
func pgRoleLimitsRefusalIn(output []byte) string {
	return databaseAccountRefusalIn(output)
}

// pgRoleLimitsRefusalText is the answer's sentence for a refused role.
func pgRoleLimitsRefusalText(role, code string) string {
	switch code {
	case transport.PGRoleLimitRefusedAbsent:
		return "the database user " + role + " does not exist on this server's PostgreSQL; nothing was changed"
	case transport.PGRoleLimitRefusedPrivileged, transport.PGRoleLimitRefusedMember:
		return databaseAccountRefusalText(role, code)
	case transport.PGRoleLimitRefusedNotRecorded:
		return "the database user " + role + " is not one of CelikPanel's site database users on this PostgreSQL, so its limits are not changed here; nothing was changed"
	}
	return "the database user " + role + " cannot be given limits here; nothing was changed"
}

// SetPostgreSQLRoleLimitsV1 reads, and with Writes changes, the limits of one
// site database role (see the top of this file).
func (a *Agent) SetPostgreSQLRoleLimitsV1(req *transport.PostgreSQLRoleLimitsRequest, resp *transport.PostgreSQLRoleLimitsResponse) error {
	if req == nil || resp == nil {
		return errors.New("PostgreSQL role limits request and response are required")
	}
	*resp = transport.PostgreSQLRoleLimitsResponse{}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "changing a database user's limits"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	keys := make([]string, 0, len(req.Writes))
	for _, write := range req.Writes {
		keys = append(keys, write.Key+"="+strconv.Itoa(write.Value))
	}
	outcome := "applied"
	defer func() {
		log.Printf("database user limits: role %q port %d writes [%s]: %s", req.Role, req.Port, strings.Join(keys, " "), outcome)
	}()
	if code, err := validPGRoleLimitsRequest(req); err != nil {
		resp.Refused, resp.Error = code, err.Error()+"; nothing was changed"
		outcome = "refused (" + code + ")"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	recorded, err := pgRoleRecordedByPanel(ctx, req.Role, req.Port)
	if err != nil {
		resp.Error = "CelikPanel's own record of the database users could not be read, so the user's limits were not changed: " + err.Error()
		outcome = "panel record unreadable"
		return nil
	}
	if !recorded {
		resp.Refused, resp.Error = transport.PGRoleLimitRefusedNotRecorded, pgRoleLimitsRefusalText(req.Role, transport.PGRoleLimitRefusedNotRecorded)
		outcome = "refused (not recorded)"
		return nil
	}
	script, err := pgRoleLimitsScript(req.Role, req.Writes)
	if err != nil {
		resp.Refused, resp.Error = transport.PGRoleLimitRefusedInvalid, err.Error()+"; nothing was changed"
		outcome = "refused (invalid)"
		return nil
	}
	suspensionDatabaseMutex.Lock()
	defer suspensionDatabaseMutex.Unlock()
	port, err := pgStatementPortFor(req.Port)
	if err != nil {
		resp.Error = err.Error()
		// The role's cluster does not run: nothing ran.
		resp.Refusal, resp.Prerequisite = pgClusterRefusalOf(err), true
		outcome = "cluster refused"
		return nil
	}
	// alpha.149 (review of PR #370, finding 9): a role the owner holds locked
	// in an open transaction never holds this call - and with it every
	// suspension and password change, which wait on the same lock - for
	// longer than the bounds: lock_timeout and statement_timeout before the
	// block, and the client itself ended at its deadline.
	output, runErr := pgRoleLimitsRun(port, pgRoleLimitsSessionBounds+script)
	if runErr != nil {
		if code := pgRoleLimitsRefusalIn(output); code != "" {
			resp.Refused, resp.Error = code, pgRoleLimitsRefusalText(req.Role, code)
			outcome = "refused (" + code + ")"
			return nil
		}
		failure := hostcmd.Fail("the database user's limits could not be changed", output, runErr, databaseClientMeaning)
		var ended *pgClientEndedError
		switch {
		case errors.As(runErr, &ended):
			// The client was ended at its deadline: what the engine did
			// with a block it may have received is not known.
			resp.Unknown = true
			outcome = "result unknown (time ran out)"
		case pgClientDidNotConnect(output):
			// alpha.149 (finding 7): psql made no connection, so nothing
			// ran. Whether the server runs is established from its service
			// state or its socket, never from psql's words: not running is
			// an unmet prerequisite; running, it refused the connection
			// (authentication, too many connections, starting up).
			if running, known := pgServerRunning(port); known && !running {
				resp.Prerequisite = true
				outcome = "database server not running"
			} else {
				resp.Connection = true
				outcome = "database server refused the connection"
			}
		case strings.Contains(string(output), "ERROR:"):
			// The server refused a statement (a lock or statement timeout
			// among them): the DO block is one transaction, so nothing of it
			// was kept.
			outcome = "engine refused"
		default:
			// A lost answer: unknown, also for a read (nothing was refused).
			resp.Unknown = true
			outcome = "result unknown"
		}
		resp.Error = failure.Error()
		return nil
	}
	conn, okConn := markerValue(output, "conn")
	limit, convErr := strconv.Atoi(conn)
	settings := map[string]string{}
	okSettings := true
	for _, key := range pgRoleSettingKeys {
		value, ok := markerValue(output, key)
		okSettings = okSettings && ok
		settings[key] = value
	}
	if !okConn || convErr != nil || !okSettings {
		// The writes committed (or, for a read, the answer was lost); what
		// the role holds now was not read.
		resp.Unknown = true
		resp.Error = "the database user's limits were sent, but PostgreSQL's answer about them could not be read"
		outcome = "read back unreadable"
		return nil
	}
	resp.Applied, resp.ConnectionLimit, resp.Settings = true, limit, settings
	return nil
}
