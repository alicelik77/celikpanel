package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// One function decides and applies a database user's limits (alpha.149 fix
// round B; the alpha.148 lab, D9 and D10). The limits are the subscription's
// effective db.max_user_connections and its time and temporary-file keys
// (database_role_limits.go). applyDatabaseUserLimits is called by every path
// that creates or re-creates a database user, whenever ANY of those keys is
// set, and by the rewrite after a changed limit:
//
//   - the current values are read first; a state that cannot be read writes
//     nothing at all (nothing is removed or raised) and the user is reported;
//   - the connection limit is the panel's own, as since alpha.128 (the plan's
//     value is written; a lifted limit is removed again; a new user without a
//     limit is not touched); the time and temporary-file keys keep the server
//     owner's hand-set values (database_role_limits.go);
//   - local PostgreSQL is reached through the Agent as the postgres
//     superuser (Agent.SetPostgreSQLRoleLimitsV1): the panel's own account
//     (celikpanel_admin) cannot alter the roles the Agent made on PostgreSQL
//     16 and later, nor set temp_file_limit anywhere, and gains no right for
//     it. MariaDB, and a PostgreSQL on another machine, through the record's
//     own account as before;
//   - a user the limits did not reach is recorded (panel_settings
//     db_limits_unapplied_v1; no schema change) with whether it was a
//     verified failure, an unmet prerequisite (the database server is not
//     running) or an unknown result; Problems now names them and offers
//     Apply again (database_limits_problems.go). A user they reach later
//     leaves the record.
//
// Veritabanı kullanıcısının sınırlarını tek işlev belirler ve uygular; her
// oluşturma yolu onu çağırır. Okunamayan durum hiçbir şeyi kaldırmaz ya da
// yükseltmez; ulaşmayan kullanıcı kaydedilir ve Şu anki sorunlar'da görünür.

// The outcomes of one user's apply.
const (
	dbLimitApplied      = "applied"
	dbLimitFailed       = "failed"
	dbLimitPrerequisite = "prerequisite"
	dbLimitUnknown      = "unknown"
)

// dbLimitError carries an outcome class with its cause.
type dbLimitError struct {
	state string
	err   error
}

func (e *dbLimitError) Error() string { return e.err.Error() }
func (e *dbLimitError) Unwrap() error { return e.err }

// dbLimitStateOf classifies an apply's error: verified failure, unmet
// prerequisite or unknown result.
func dbLimitStateOf(err error) string {
	if err == nil {
		return dbLimitApplied
	}
	var typed *dbLimitError
	if errors.As(err, &typed) {
		return typed.state
	}
	var refusal *pgClusterRefusalError
	if errors.As(err, &refusal) || agentUnreachable(err) {
		return dbLimitPrerequisite
	}
	if services.ClassifyDatabaseEngineRefusal(err) == services.DatabaseEngineRefusalCredential {
		return dbLimitFailed
	}
	// A connection that broke or timed out after the statement may have been
	// sent: the result is not known.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return dbLimitUnknown
	}
	text := strings.ToLower(err.Error())
	for _, phrase := range []string{"bad connection", "broken pipe", "connection reset", "i/o timeout", "unexpected eof", "deadline exceeded"} {
		if strings.Contains(text, phrase) {
			return dbLimitUnknown
		}
	}
	// No connection was made: nothing ran (the database server is stopped).
	if databaseEngineFailedBeforeStatement(err) || (strings.Contains(text, "dial ") && strings.Contains(text, "no such file or directory")) {
		return dbLimitPrerequisite
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return dbLimitUnknown
	}
	return dbLimitFailed
}

// dbRoleWrittenText is the engine's text of a value the panel wrote.
func dbRoleWrittenText(key string, value int) string {
	if key == services.RoleKeyTempFileMB {
		return strconv.Itoa(value) + "MB"
	}
	return strconv.Itoa(value) + "s"
}

// --- Local PostgreSQL through the Agent ---------------------------------------

// pgRoleLimitsThroughAgent is the Agent's call (a variable for tests).
var pgRoleLimitsThroughAgent = func(p *Panel, ctx context.Context, req *transport.PostgreSQLRoleLimitsRequest) (transport.PostgreSQLRoleLimitsResponse, error) {
	var resp transport.PostgreSQLRoleLimitsResponse
	err := p.callAgentContext(ctx, "Agent.SetPostgreSQLRoleLimitsV1", req, &resp)
	return resp, err
}

// pgAgentRoleLimiter writes a local PostgreSQL role's limits through the
// Agent, on the cluster at port.
type pgAgentRoleLimiter struct {
	p    *Panel
	ctx  context.Context
	port int
}

func (l *pgAgentRoleLimiter) call(username string, writes []transport.PostgreSQLRoleLimitWrite) (transport.PostgreSQLRoleLimitsResponse, error) {
	req := &transport.PostgreSQLRoleLimitsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: l.port, Role: username, Writes: writes,
	}
	resp, err := pgRoleLimitsThroughAgent(l.p, l.ctx, req)
	if err != nil {
		if agentUnreachable(err) {
			return resp, &dbLimitError{state: dbLimitPrerequisite, err: fmt.Errorf("the CelikPanel Agent is not reachable: %w", err)}
		}
		// The request may have reached the Agent: what it did is not known.
		return resp, &dbLimitError{state: dbLimitUnknown, err: err}
	}
	switch {
	case resp.Applied:
		return resp, nil
	case resp.Refusal != nil:
		return resp, &pgClusterRefusalError{refusal: *resp.Refusal}
	case resp.Prerequisite:
		return resp, &dbLimitError{state: dbLimitPrerequisite, err: errors.New(boundedAgentDiagnostic(resp.Error))}
	case resp.Unknown:
		return resp, &dbLimitError{state: dbLimitUnknown, err: errors.New(boundedAgentDiagnostic(resp.Error))}
	}
	text := resp.Error
	if text == "" {
		text = "the Agent did not confirm the database user's limits"
	}
	return resp, &dbLimitError{state: dbLimitFailed, err: errors.New(boundedAgentDiagnostic(text))}
}

func (l *pgAgentRoleLimiter) SetUserConnectionLimit(username string, limit int) error {
	_, err := l.call(username, []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitConnection, Value: limit}})
	return err
}

func (l *pgAgentRoleLimiter) RoleKeysSupported() []string { return services.RoleKeys }

func (l *pgAgentRoleLimiter) ReadUserRoleSettings(username string) (services.RoleSettings, error) {
	resp, err := l.call(username, nil)
	if err != nil {
		return nil, err
	}
	out := services.RoleSettings{}
	for _, key := range services.RoleKeys {
		out[key] = resp.Settings[key]
	}
	return out, nil
}

func (l *pgAgentRoleLimiter) SetUserRoleSetting(username, key string, value int) error {
	_, err := l.call(username, []transport.PostgreSQLRoleLimitWrite{{Key: key, Value: value}})
	return err
}

// --- The one function ---------------------------------------------------------

// dbLimitOutcome is what one user's apply came to.
type dbLimitOutcome struct {
	ServerID       int    `json:"server"`
	SubscriptionID int    `json:"subscription"`
	User           string `json:"user"`
	State          string `json:"state"`
	Reason         string `json:"reason,omitempty"`
}

// dbLimitsApplyMu keeps one apply of a user's limits at a time, so a
// creation, a plan's rewrite and Apply again never interleave on a user.
var dbLimitsApplyMu sync.Mutex

// applyDatabaseUserLimits writes the subscription's limits for one user on
// server (see the top of this file). fresh: the user was just created (no
// limit writes nothing; an earlier record of the name is forgotten). driver
// may be nil (the record's own is opened).
func (p *Panel) applyDatabaseUserLimits(ctx context.Context, subscriptionID int, server *core.DatabaseServer, driver services.DatabaseDriver, username string, fresh bool, clusters func() transport.PostgreSQLClustersResponse) dbLimitOutcome {
	dbLimitsApplyMu.Lock()
	defer dbLimitsApplyMu.Unlock()
	outcome := dbLimitOutcome{ServerID: server.ID, SubscriptionID: subscriptionID, User: username, State: dbLimitApplied}
	err := p.applyDatabaseUserLimitsLocked(ctx, subscriptionID, server, driver, username, fresh, clusters)
	if err != nil {
		outcome.State, outcome.Reason = dbLimitStateOf(err), boundedAgentDiagnostic(err.Error())
		log.Printf("database user limits of %s (%s) were not applied: %v", username, outcome.State, err)
	}
	p.noteDBLimitOutcome(ctx, outcome)
	return outcome
}

func (p *Panel) applyDatabaseUserLimitsLocked(ctx context.Context, subscriptionID int, server *core.DatabaseServer, driver services.DatabaseDriver, username string, fresh bool, clusters func() transport.PostgreSQLClustersResponse) error {
	limit, err := p.dbConnectionLimit(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("read the connection limit: %w", err)
	}
	roleLimits, err := p.dbRoleLimits(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("read the time limits: %w", err)
	}
	if fresh && limit == 0 && roleLimits == (services.RoleLimits{}) {
		return nil // a new user, and no limit is set
	}
	limiter, ok, err := p.connectionLimiterFor(ctx, server, driver, clusters)
	if err != nil {
		return err
	}
	if !ok {
		return nil // an engine without per-user limits
	}
	roleLimiter, hasRoles := limiter.(services.RoleLimiter)
	var current services.RoleSettings
	if hasRoles {
		// Read before anything is written: an unreadable state writes
		// nothing, so nothing is removed or raised on a guess.
		if current, err = roleLimiter.ReadUserRoleSettings(username); err != nil {
			return fmt.Errorf("read the user's current limits: %w", err)
		}
	}
	var firstErr error
	if !fresh || limit != 0 {
		if err := limiter.SetUserConnectionLimit(username, limit); err != nil {
			firstErr = fmt.Errorf("connection limit: %w", err)
		}
	}
	if hasRoles {
		if err := p.applyDBRoleLimitsFrom(ctx, roleLimiter, server.ID, username, roleLimits, fresh, current); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("time limits: %w", err)
		}
	}
	return firstErr
}

// applyNewDatabaseUserLimits is the call of every creating path: the limits
// of a user just created and recorded on serverID. It never fails the
// creation; a user the limits did not reach is reported (Problems now).
func (p *Panel) applyNewDatabaseUserLimits(ctx context.Context, subscriptionID, serverID int, username string) dbLimitOutcome {
	return p.applyRecordedUserLimits(ctx, subscriptionID, serverID, username, true)
}

// applyRecordedUserLimits applies one recorded user's limits with its own
// bound, apart from the caller's request.
func (p *Panel) applyRecordedUserLimits(ctx context.Context, subscriptionID, serverID int, username string, fresh bool) dbLimitOutcome {
	if p.db == nil {
		return dbLimitOutcome{ServerID: serverID, SubscriptionID: subscriptionID, User: username, State: dbLimitApplied}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	server, err := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB()).GetByID(ctx, serverID)
	if err != nil {
		outcome := dbLimitOutcome{ServerID: serverID, SubscriptionID: subscriptionID, User: username, State: dbLimitFailed,
			Reason: boundedAgentDiagnostic("the database server record could not be read: " + err.Error())}
		log.Printf("database user limits of %s were not applied: %s", username, outcome.Reason)
		p.noteDBLimitOutcome(ctx, outcome)
		return outcome
	}
	return p.applyDatabaseUserLimits(ctx, subscriptionID, server, nil, username, fresh, p.pgClustersOnce(ctx))
}

// dbLimitsFollowSwitch: a PostgreSQL database moved to another cluster
// (record `to`) took its users with it. The move re-creates a missing role
// with the source's own settings (pg_move.go: ALTER ROLE ... SET), values the
// panel wrote there among them; what the panel wrote is carried to the new
// record, so those values stay the panel's (not the owner's), and the users
// are given their limits on the new cluster by the one function.
func (p *Panel) dbLimitsFollowSwitch(ctx context.Context, databaseID, from, to int) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT u.username, u.subscription_id FROM database_user_grants g
		JOIN database_users u ON u.id = g.user_id WHERE g.database_id = ? AND u.server_id = ? ORDER BY u.id`, databaseID, to)
	if err != nil {
		log.Printf("database user limits after a cluster move: %v", err)
		return
	}
	type moved struct {
		username     string
		subscription int
	}
	var users []moved
	for rows.Next() {
		var u moved
		if err := rows.Scan(&u.username, &u.subscription); err != nil {
			rows.Close()
			log.Printf("database user limits after a cluster move: %v", err)
			return
		}
		users = append(users, u)
	}
	rows.Close()
	dbRoleRecordMu.Lock()
	record := p.loadDBRoleRecord(ctx)
	carried := false
	for _, u := range users {
		source, target := record[dbRoleRecordKey(from, u.username)], dbRoleRecordKey(to, u.username)
		if source == nil || record[target] != nil {
			continue
		}
		copied := &dbRoleUserRecord{Panel: map[string]dbRoleWritten{}, Owner: map[string]string{}}
		for k, v := range source.Panel {
			copied.Panel[k] = v
		}
		for k, v := range source.Owner {
			copied.Owner[k] = v
		}
		record[target], carried = copied, true
	}
	if carried {
		p.saveDBRoleRecord(ctx, record)
	}
	dbRoleRecordMu.Unlock()
	for _, u := range users {
		p.applyRecordedUserLimits(ctx, u.subscription, to, u.username, false)
	}
}

// --- The record of users the limits did not reach ------------------------------

const settingDBLimitsUnapplied = "db_limits_unapplied_v1"

type dbLimitsUnappliedEntry struct {
	dbLimitOutcome
	At string `json:"at"`
}

var dbLimitsUnappliedMu sync.Mutex

func (p *Panel) loadDBLimitsUnapplied(ctx context.Context) map[string]dbLimitsUnappliedEntry {
	out := map[string]dbLimitsUnappliedEntry{}
	if raw := p.setting(ctx, settingDBLimitsUnapplied); raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// noteDBLimitOutcome keeps the record in step with one user's apply.
func (p *Panel) noteDBLimitOutcome(ctx context.Context, outcome dbLimitOutcome) {
	if p.db == nil {
		return
	}
	dbLimitsUnappliedMu.Lock()
	defer dbLimitsUnappliedMu.Unlock()
	record := p.loadDBLimitsUnapplied(ctx)
	key := dbRoleRecordKey(outcome.ServerID, outcome.User)
	if outcome.State == dbLimitApplied {
		if _, ok := record[key]; !ok {
			return
		}
		delete(record, key)
	} else {
		record[key] = dbLimitsUnappliedEntry{dbLimitOutcome: outcome, At: time.Now().UTC().Format(time.RFC3339)}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err := p.setSetting(context.WithoutCancel(ctx), settingDBLimitsUnapplied, string(raw)); err != nil {
		log.Printf("database user limits: the users they did not reach were not recorded: %v", err)
	}
}

// dbLimitsUnappliedNow lists the recorded users that still exist in the
// panel's records, oldest first.
func (p *Panel) dbLimitsUnappliedNow(ctx context.Context) []dbLimitsUnappliedEntry {
	dbLimitsUnappliedMu.Lock()
	record := p.loadDBLimitsUnapplied(ctx)
	dbLimitsUnappliedMu.Unlock()
	out := []dbLimitsUnappliedEntry{}
	for _, entry := range record {
		var exists int
		if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_users WHERE server_id = ? AND username = ?`,
			entry.ServerID, entry.User).Scan(&exists); err != nil || exists == 0 {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		return out[i].User < out[j].User
	})
	return out
}
