package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
//   - the time and temporary-file keys keep the server owner's hand-set
//     values (database_role_limits.go); so does, on PostgreSQL, the
//     connection limit (review of PR #370, finding 3: recorded as the panel
//     wrote it, written only when the plan's value changed or for a user
//     without one). MariaDB's connection limit keeps its rule of alpha.128
//     (the plan's value is written; a lifted limit is removed again; a new
//     user without a limit is not touched);
//   - CelikPanel's own local PostgreSQL (databaseRecordIsOurLocalEngine) is
//     reached through the Agent as the postgres superuser
//     (Agent.SetPostgreSQLRoleLimitsV1): the panel's own account
//     (celikpanel_admin) cannot alter the roles the Agent made on PostgreSQL
//     16 and later, nor set temp_file_limit anywhere, and gains no right for
//     it. Every other record - MariaDB, a PostgreSQL the administrator
//     registered with an account of their own, one on another machine - is
//     reached with the record's own account as before (finding 8);
//   - a user the limits did not reach is recorded (panel_settings
//     db_limits_unapplied_v1; no schema change) with what is actually known
//     (finding 7): the Agent was not reachable, the database server does not
//     run, it refused the connection, the Agent refused the user, the engine
//     refused a statement, or the result is not known; Problems now names
//     them, says why and offers Apply again (database_limits_problems.go). A
//     run's users are written as pending first (finding 6), so a restart
//     leaves them "not known yet", never done. A user they reach later
//     leaves the record; the hourly round tries them again (finding 10).
//
// Veritabanı kullanıcısının sınırlarını tek işlev belirler ve uygular; her
// oluşturma yolu onu çağırır. Okunamayan durum hiçbir şeyi kaldırmaz ya da
// yükseltmez; ulaşmayan kullanıcı, bilinen nedeniyle kaydedilir ve Şu anki
// sorunlar'da görünür.

// The outcomes of one user's apply.
const (
	dbLimitApplied = "applied"
	// dbLimitFailed: a verified failure - the engine refused a statement
	// (nothing of it was kept) or the panel could not read its own record.
	dbLimitFailed = "failed"
	// dbLimitPrerequisite: the database server does not run (the Agent read
	// its service state or socket; a MariaDB that nothing answers at).
	dbLimitPrerequisite = "prerequisite"
	// dbLimitUnknown: the write may have landed; its answer was lost.
	dbLimitUnknown = "unknown"
	// dbLimitAgent: the CelikPanel Agent is not reachable; nothing was sent.
	dbLimitAgent = "agent"
	// dbLimitConnection: the database server runs but refused the connection
	// (authentication, too many connections, starting up); nothing ran.
	dbLimitConnection = "connection"
	// dbLimitRefused: the Agent refused the user (an administrator account,
	// a member of a role, not CelikPanel's); nothing was changed.
	dbLimitRefused = "refused"
	// dbLimitPending: a run's user not reached yet. A pending entry of a run
	// this panel no longer runs (a restart) reads as "not known yet".
	dbLimitPending = "pending"
)

// The reason codes a Problems now line words (TR and EN); the journal holds
// the detail.
const (
	dbReasonAgent          = "agent_unreachable"
	dbReasonNotRunning     = "server_not_running"
	dbReasonConnection     = "connection_refused"
	dbReasonPrivileged     = "refused_privileged"
	dbReasonMember         = "refused_member"
	dbReasonNotRecorded    = "refused_not_recorded"
	dbReasonAbsent         = "refused_absent"
	dbReasonEngine         = "engine_refused"
	dbReasonUnknown        = "result_unknown"
	dbReasonNotKnownYet    = "not_known_yet"
	dbReasonServerRecord   = "server_record_unreadable"
	dbReasonRecordUnusable = "record_unreadable"
)

// dbLimitError carries an outcome class and its reason code with its cause.
type dbLimitError struct {
	state string
	code  string
	err   error
}

func (e *dbLimitError) Error() string { return e.err.Error() }
func (e *dbLimitError) Unwrap() error { return e.err }

// dbLimitStateOf classifies an apply's error by what is actually known.
func dbLimitStateOf(err error) string {
	state, _ := dbLimitClassOf(err)
	return state
}

// dbLimitClassOf answers an error's outcome class and reason code.
func dbLimitClassOf(err error) (string, string) {
	if err == nil {
		return dbLimitApplied, ""
	}
	var typed *dbLimitError
	if errors.As(err, &typed) {
		return typed.state, typed.code
	}
	var refusal *pgClusterRefusalError
	if errors.As(err, &refusal) {
		return dbLimitPrerequisite, dbReasonNotRunning
	}
	if agentUnreachable(err) {
		return dbLimitAgent, dbReasonAgent
	}
	text := strings.ToLower(err.Error())
	if services.ClassifyDatabaseEngineRefusal(err) == services.DatabaseEngineRefusalCredential ||
		strings.Contains(text, "too many connections") || strings.Contains(text, "too many clients") || strings.Contains(text, "max_user_connections") {
		// The server answered and refused the connection: nothing ran.
		return dbLimitConnection, dbReasonConnection
	}
	// A connection that broke or timed out after the statement may have been
	// sent: the result is not known.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return dbLimitUnknown, dbReasonUnknown
	}
	for _, phrase := range []string{"bad connection", "broken pipe", "connection reset", "i/o timeout", "unexpected eof", "deadline exceeded"} {
		if strings.Contains(text, phrase) {
			return dbLimitUnknown, dbReasonUnknown
		}
	}
	// No connection was made (MariaDB: nothing listens at the socket or the
	// port): nothing ran, the server does not run.
	if databaseEngineFailedBeforeStatement(err) || (strings.Contains(text, "dial ") && strings.Contains(text, "no such file or directory")) {
		return dbLimitPrerequisite, dbReasonNotRunning
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return dbLimitUnknown, dbReasonUnknown
	}
	return dbLimitFailed, dbReasonEngine
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

// pgRefusalReason is the reason code of an Agent refusal.
func pgRefusalReason(code string) string {
	switch code {
	case transport.PGRoleLimitRefusedPrivileged:
		return dbReasonPrivileged
	case transport.PGRoleLimitRefusedMember:
		return dbReasonMember
	case transport.PGRoleLimitRefusedNotRecorded:
		return dbReasonNotRecorded
	case transport.PGRoleLimitRefusedAbsent:
		return dbReasonAbsent
	}
	return dbReasonEngine
}

func (l *pgAgentRoleLimiter) call(username string, writes []transport.PostgreSQLRoleLimitWrite) (transport.PostgreSQLRoleLimitsResponse, error) {
	req := &transport.PostgreSQLRoleLimitsRequest{
		ExpectedBuildCommit: strings.TrimSpace(buildCommit), Port: l.port, Role: username, Writes: writes,
	}
	resp, err := pgRoleLimitsThroughAgent(l.p, l.ctx, req)
	if err != nil {
		if agentUnreachable(err) {
			return resp, &dbLimitError{state: dbLimitAgent, code: dbReasonAgent, err: fmt.Errorf("the CelikPanel Agent is not reachable: %w", err)}
		}
		// The request may have reached the Agent: what it did is not known.
		return resp, &dbLimitError{state: dbLimitUnknown, code: dbReasonUnknown, err: err}
	}
	text := boundedAgentDiagnostic(resp.Error)
	switch {
	case resp.Applied:
		return resp, nil
	case resp.Refusal != nil:
		return resp, &pgClusterRefusalError{refusal: *resp.Refusal}
	case resp.Prerequisite:
		return resp, &dbLimitError{state: dbLimitPrerequisite, code: dbReasonNotRunning, err: errors.New(text)}
	case resp.Connection:
		return resp, &dbLimitError{state: dbLimitConnection, code: dbReasonConnection, err: errors.New(text)}
	case resp.Refused != "":
		return resp, &dbLimitError{state: dbLimitRefused, code: pgRefusalReason(resp.Refused), err: errors.New(text)}
	case resp.Unknown:
		return resp, &dbLimitError{state: dbLimitUnknown, code: dbReasonUnknown, err: errors.New(text)}
	}
	if text == "" {
		text = "the Agent did not confirm the database user's limits"
	}
	return resp, &dbLimitError{state: dbLimitFailed, code: dbReasonEngine, err: errors.New(text)}
}

func (l *pgAgentRoleLimiter) SetUserConnectionLimit(username string, limit int) error {
	_, err := l.setConnectionLimit(username, limit)
	return err
}

// setConnectionLimit writes the limit and answers rolconnlimit read back.
func (l *pgAgentRoleLimiter) setConnectionLimit(username string, limit int) (int, error) {
	resp, err := l.call(username, []transport.PostgreSQLRoleLimitWrite{{Key: transport.PGRoleLimitConnection, Value: limit}})
	return resp.ConnectionLimit, err
}

func (l *pgAgentRoleLimiter) RoleKeysSupported() []string { return services.RoleKeys }

func (l *pgAgentRoleLimiter) ReadUserRoleSettings(username string) (services.RoleSettings, error) {
	settings, _, err := l.readUserLimits(username)
	return settings, err
}

// readUserLimits reads the role settings and the connection limit in one call.
func (l *pgAgentRoleLimiter) readUserLimits(username string) (services.RoleSettings, int, error) {
	resp, err := l.call(username, nil)
	if err != nil {
		return nil, 0, err
	}
	out := services.RoleSettings{}
	for _, key := range services.RoleKeys {
		out[key] = resp.Settings[key]
	}
	return out, resp.ConnectionLimit, nil
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
	// Code is the reason's code (dbReason*), worded on the screen.
	Code string `json:"code,omitempty"`
	// Reason is the bounded detail, for the journal and the record.
	Reason string `json:"reason,omitempty"`
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
		outcome.State, outcome.Code = dbLimitClassOf(err)
		outcome.Reason = boundedAgentDiagnostic(err.Error())
		log.Printf("database user limits of %s (%s, %s) were not applied: %v", username, outcome.State, outcome.Code, err)
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
	if fresh {
		// alpha.149 (finding 5): an earlier user of the same name left its
		// record; forgotten before anything else, also when nothing is set.
		p.forgetDBRoleRecord(ctx, server.ID, username)
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
	if agentLimiter, isAgent := limiter.(*pgAgentRoleLimiter); isAgent {
		// Read before anything is written: an unreadable state writes
		// nothing, so nothing is removed or raised on a guess.
		current, connection, err := agentLimiter.readUserLimits(username)
		if err != nil {
			return fmt.Errorf("read the user's current limits: %w", err)
		}
		step := &dbConnectionStep{current: connection, want: limit, set: func(value int) (int, error) {
			return agentLimiter.setConnectionLimit(username, value)
		}}
		return p.applyDBRoleLimitsFrom(ctx, agentLimiter, server.ID, username, roleLimits, fresh, current, step)
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
		if err := p.applyDBRoleLimitsFrom(ctx, roleLimiter, server.ID, username, roleLimits, fresh, current, nil); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("time limits: %w", err)
		}
	}
	return firstErr
}

// dbLimitsCreateWait bounds how long a creating request waits for its
// user's limits (review of PR #370, finding 11); a variable for tests.
var dbLimitsCreateWait = 10 * time.Second

// applyNewDatabaseUserLimits is the call of every creating path: the limits
// of a user just created and recorded on serverID. It never fails the
// creation, and waits at most dbLimitsCreateWait: a user still being given
// its limits then is answered (and recorded) as pending, and the work goes
// on apart from the request; a user the limits do not reach is reported
// (Problems now).
func (p *Panel) applyNewDatabaseUserLimits(ctx context.Context, subscriptionID, serverID int, username string) dbLimitOutcome {
	pending := dbLimitOutcome{ServerID: serverID, SubscriptionID: subscriptionID, User: username, State: dbLimitPending, Code: dbReasonNotKnownYet}
	if p.db == nil {
		return dbLimitOutcome{ServerID: serverID, SubscriptionID: subscriptionID, User: username, State: dbLimitApplied}
	}
	detached := context.WithoutCancel(ctx)
	if !p.dbLimitsSetFor(detached, subscriptionID) {
		// Nothing to write (an earlier record of the name is still
		// forgotten): no engine is asked, no pending entry is needed.
		return p.applyRecordedUserLimits(detached, subscriptionID, serverID, username, true)
	}
	run := dbLimitsRunID()
	beginDBLimitsLiveRun(run)
	p.markDBLimitsPending(detached, []dbLimitUser{{subscription: subscriptionID, server: serverID, username: username}}, run)
	done := make(chan dbLimitOutcome, 1)
	go func() {
		defer endDBLimitsLiveRun(run)
		done <- p.applyRecordedUserLimits(detached, subscriptionID, serverID, username, true)
	}()
	timer := time.NewTimer(dbLimitsCreateWait)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome
	case <-timer.C:
		return pending
	}
}

// dbLimitsSetFor: whether any database limit is set for the subscription
// (true when it cannot be read: the one function then says why).
func (p *Panel) dbLimitsSetFor(ctx context.Context, subscriptionID int) bool {
	limit, err := p.dbConnectionLimit(ctx, subscriptionID)
	if err != nil || limit != 0 {
		return true
	}
	roleLimits, err := p.dbRoleLimits(ctx, subscriptionID)
	return err != nil || roleLimits != (services.RoleLimits{})
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
		outcome := dbLimitOutcome{ServerID: serverID, SubscriptionID: subscriptionID, User: username, State: dbLimitFailed, Code: dbReasonServerRecord,
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
// record, so those values stay the panel's (not the owner's). The users are
// then given their limits on the new cluster in a run apart from the switch
// (review of PR #370, finding 11: the switch answers when it is done),
// written as pending first.
func (p *Panel) dbLimitsFollowSwitch(ctx context.Context, databaseID, from, to int) {
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT u.username, u.subscription_id FROM database_user_grants g
		JOIN database_users u ON u.id = g.user_id WHERE g.database_id = ? AND u.server_id = ? ORDER BY u.id`, databaseID, to)
	if err != nil {
		log.Printf("database user limits after a cluster move: %v", err)
		return
	}
	var users []dbLimitUser
	for rows.Next() {
		u := dbLimitUser{server: to}
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
	if len(users) == 0 {
		return
	}
	run := dbLimitsRunID()
	beginDBLimitsLiveRun(run)
	detached := context.WithoutCancel(ctx)
	p.markDBLimitsPending(detached, users, run)
	startDBLimitsRun(func() {
		defer endDBLimitsLiveRun(run)
		runCtx, cancel := context.WithTimeout(detached, 10*time.Minute)
		defer cancel()
		p.applyDBLimitsToUsers(runCtx, users, nil)
	})
}

// --- The record of users the limits did not reach ------------------------------

const settingDBLimitsUnapplied = "db_limits_unapplied_v1"

type dbLimitsUnappliedEntry struct {
	dbLimitOutcome
	At string `json:"at"`
	// Run names the run a pending entry belongs to (finding 6).
	Run string `json:"run,omitempty"`
}

var dbLimitsUnappliedMu sync.Mutex

// errDBLimitsRecordUnreadable: the record's row is there but cannot be read;
// it is never written over, and Problems now says so.
var errDBLimitsRecordUnreadable = errors.New("the record of the database users the limits did not reach could not be read")

// loadDBLimitsUnapplied reads the record; a row that cannot be read is an
// error, never an empty record (alpha.149, finding 12).
func (p *Panel) loadDBLimitsUnapplied(ctx context.Context) (map[string]dbLimitsUnappliedEntry, error) {
	out := map[string]dbLimitsUnappliedEntry{}
	if raw := p.setting(ctx, settingDBLimitsUnapplied); raw != "" {
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return map[string]dbLimitsUnappliedEntry{}, fmt.Errorf("%w: %v", errDBLimitsRecordUnreadable, err)
		}
	}
	return out, nil
}

// saveDBLimitsUnapplied writes the record, leaving out the users the panel
// no longer has (finding 12: no leftovers). The caller holds the lock.
func (p *Panel) saveDBLimitsUnapplied(ctx context.Context, record map[string]dbLimitsUnappliedEntry) {
	for key, entry := range record {
		if exists, err := p.dbUserRecorded(ctx, entry.ServerID, entry.User); err == nil && !exists {
			delete(record, key)
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	if err := p.setSetting(context.WithoutCancel(ctx), settingDBLimitsUnapplied, string(raw)); err != nil {
		log.Printf("database user limits: the users they did not reach were not recorded: %v", err)
	}
}

func (p *Panel) dbUserRecorded(ctx context.Context, serverID int, username string) (bool, error) {
	var exists int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_users WHERE server_id = ? AND username = ?`, serverID, username).Scan(&exists)
	return exists > 0, err
}

// noteDBLimitOutcome keeps the record in step with one user's apply.
func (p *Panel) noteDBLimitOutcome(ctx context.Context, outcome dbLimitOutcome) {
	if p.db == nil {
		return
	}
	dbLimitsUnappliedMu.Lock()
	defer dbLimitsUnappliedMu.Unlock()
	record, err := p.loadDBLimitsUnapplied(ctx)
	if err != nil {
		log.Printf("database user limits: %v; %s (%s) is not recorded and the row is left as it is", err, outcome.User, outcome.State)
		return
	}
	key := dbRoleRecordKey(outcome.ServerID, outcome.User)
	if outcome.State == dbLimitApplied {
		if _, ok := record[key]; !ok {
			return
		}
		delete(record, key)
	} else {
		record[key] = dbLimitsUnappliedEntry{dbLimitOutcome: outcome, At: time.Now().UTC().Format(time.RFC3339)}
	}
	p.saveDBLimitsUnapplied(ctx, record)
}

// markDBLimitsPending writes a run's users as pending before the run starts
// (finding 6). An unreadable record is started afresh only by Apply again
// (replace), never by another run.
func (p *Panel) markDBLimitsPending(ctx context.Context, users []dbLimitUser, run string) {
	p.markDBLimitsPendingReplacing(ctx, users, run, false)
}

func (p *Panel) markDBLimitsPendingReplacing(ctx context.Context, users []dbLimitUser, run string, replace bool) {
	if p.db == nil || len(users) == 0 {
		return
	}
	dbLimitsUnappliedMu.Lock()
	defer dbLimitsUnappliedMu.Unlock()
	record, err := p.loadDBLimitsUnapplied(ctx)
	if err != nil && !replace {
		log.Printf("database user limits: %v; the run's users are not written as pending", err)
		return
	}
	at := time.Now().UTC().Format(time.RFC3339)
	for _, u := range users {
		record[dbRoleRecordKey(u.server, u.username)] = dbLimitsUnappliedEntry{
			dbLimitOutcome: dbLimitOutcome{ServerID: u.server, SubscriptionID: u.subscription, User: u.username, State: dbLimitPending, Code: dbReasonNotKnownYet},
			At:             at, Run: run,
		}
	}
	p.saveDBLimitsUnapplied(ctx, record)
}

// pruneDBLimitRecords forgets the records of database users the panel no
// longer has (both records; finding 5 and 12). Called after a user is
// removed and by the hourly round.
func (p *Panel) pruneDBLimitRecords(ctx context.Context) {
	if p.db == nil {
		return
	}
	dbRoleRecordMu.Lock()
	record := p.loadDBRoleRecord(ctx)
	changed := false
	for key := range record {
		serverText, username, ok := strings.Cut(key, "/")
		serverID, err := strconv.Atoi(serverText)
		if !ok || err != nil {
			continue
		}
		if exists, err := p.dbUserRecorded(ctx, serverID, username); err == nil && !exists {
			delete(record, key)
			changed = true
		}
	}
	if changed {
		p.saveDBRoleRecord(ctx, record)
	}
	dbRoleRecordMu.Unlock()
	dbLimitsUnappliedMu.Lock()
	defer dbLimitsUnappliedMu.Unlock()
	unapplied, err := p.loadDBLimitsUnapplied(ctx)
	if err != nil || len(unapplied) == 0 {
		return
	}
	before := len(unapplied)
	for key, entry := range unapplied {
		if exists, err := p.dbUserRecorded(ctx, entry.ServerID, entry.User); err == nil && !exists {
			delete(unapplied, key)
		}
	}
	if len(unapplied) != before {
		p.saveDBLimitsUnapplied(ctx, unapplied)
	}
}

// dbLimitsUnappliedNow lists the recorded users that still exist in the
// panel's records, oldest first. A pending entry of a run that still runs is
// left out (it is being applied); one of a run this panel no longer runs is
// "not known yet". An entry whose user cannot be looked up is listed (an
// unreadable state never hides a problem). An unreadable record is an error.
func (p *Panel) dbLimitsUnappliedNow(ctx context.Context) ([]dbLimitsUnappliedEntry, error) {
	dbLimitsUnappliedMu.Lock()
	record, err := p.loadDBLimitsUnapplied(ctx)
	dbLimitsUnappliedMu.Unlock()
	if err != nil {
		return nil, err
	}
	out := []dbLimitsUnappliedEntry{}
	for _, entry := range record {
		if exists, err := p.dbUserRecorded(ctx, entry.ServerID, entry.User); err == nil && !exists {
			continue
		}
		if entry.State == dbLimitPending {
			if dbLimitsRunLive(entry.Run) {
				continue
			}
			entry.State, entry.Code = dbLimitUnknown, dbReasonNotKnownYet
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		return out[i].User < out[j].User
	})
	return out, nil
}

// --- Runs ----------------------------------------------------------------------

// dbLimitsLiveRuns are the runs this panel process runs now; a pending entry
// of another run was left by an earlier process.
var dbLimitsLiveRuns = struct {
	sync.Mutex
	runs map[string]bool
}{runs: map[string]bool{}}

func dbLimitsRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "r" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "r" + hex.EncodeToString(b[:])
}

func beginDBLimitsLiveRun(run string) {
	dbLimitsLiveRuns.Lock()
	defer dbLimitsLiveRuns.Unlock()
	dbLimitsLiveRuns.runs[run] = true
}

func endDBLimitsLiveRun(run string) {
	dbLimitsLiveRuns.Lock()
	defer dbLimitsLiveRuns.Unlock()
	delete(dbLimitsLiveRuns.runs, run)
}

func dbLimitsRunLive(run string) bool {
	dbLimitsLiveRuns.Lock()
	defer dbLimitsLiveRuns.Unlock()
	return run != "" && dbLimitsLiveRuns.runs[run]
}
