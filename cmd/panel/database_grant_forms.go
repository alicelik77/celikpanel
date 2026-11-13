package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/mariadbgrant"
	"github.com/alicelik/celikpanel/internal/recoveryobs"
	"github.com/alicelik/celikpanel/internal/repositories"
	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Rights by exact name (alpha.134, docs/DATABASE-USERS.md, migration 078).
//
// MariaDB reads a database-level grant's name as a pattern (`_` = any one
// character), and every name CelikPanel made holds `_`: a grant it wrote on
// s5__x also covered subscription 51's s51_x. Since alpha.134 new grants are
// exact; this loop rewrites the pattern-form grants written before, on each
// MariaDB the panel manages - the machine's own (through the Agent) and
// every registered one (with the record's own account):
//
//   - It runs only after an update of the panel is verified (the release
//     recovery observation of this build says "succeeded", or no update to
//     it was ever recorded), not within the first minutes after a start, not
//     while the server is busy (an Agent mutation, the package manager, a
//     panel operation) and, on this machine, only while the Agent is the
//     same build. Until then the engine says "waiting" and why.
//   - Read: every database-level row whose stored name has an unescaped `_`
//     or `%`. A row is CelikPanel's when the panel's records prove it: a
//     recorded user given a recorded database of that engine
//     (database_user_grants), on localhost or a remote copy the Agent's
//     remote access record lists. Everything else is the owner's: left as it
//     is and listed (with the databases it covers now).
//   - Before anything changes, the row and its privileges are written to
//     database_grant_forms (the before-image). Then one row at a time: the
//     same privileges in the exact form, checked; the pattern form taken
//     away, checked (mariadbgrant.Convert). Interrupted anywhere, the next
//     round continues from the engine. A row that is no longer what was
//     captured is captured again first; one that fails three times waits a
//     day ("blocked"). Each rewritten row gets an audit row naming the other
//     databases its pattern covered ("it could also reach ...").
//   - A MariaDB whose account may not read or change the grant tables, or
//     refuses the account, or does not answer, is "blocked" with that
//     reason; the next round asks again (read only) and continues when the
//     cause is gone. An engine that is not MariaDB (MySQL) is never rewritten
//     automatically; its rows are only listed.
//
// Kalıp biçimli haklar, panel güncellemesi doğrulandıktan sonra, kayıtlarla
// kanıtlananlar için tam ada çevrilir; diğerleri yalnız listelenir.

const (
	// databaseGrantFormsPath: GET, the administrator's view (read only).
	databaseGrantFormsPath = "/api/v1/admin/database-grant-forms"

	grantFormsWaiting  = "waiting"
	grantFormsRunning  = "running"
	grantFormsDone     = "done"
	grantFormsListOnly = "list_only"
	grantFormsBlocked  = "blocked"

	// Reasons an engine waits or is blocked (the screen has words for each).
	grantFormsReasonStarting        = "starting"
	grantFormsReasonUpdate          = "update_verifying"
	grantFormsReasonUpdateUnknown   = "update_unknown"
	grantFormsReasonBusy            = "server_busy"
	grantFormsReasonAgentVersion    = "agent_version"
	grantFormsReasonAgentUnreached  = "agent_unreachable"
	grantFormsReasonCredential      = "credential_refused"
	grantFormsReasonUnreachable     = "engine_unreachable"
	grantFormsReasonNoRights        = "no_grant_rights"
	grantFormsReasonConvertFailed   = "convert_failed"
	grantFormsReasonConvertRetry    = "convert_retry"
	grantFormsReasonNotMariaDB      = "not_mariadb"
	grantFormsReasonNotProvable     = "not_provable"
	grantFormsReasonOwnerRow        = "owner_row"
	grantFormsReasonCredentialUnset = "credential_unreadable"

	grantFormsRowFailuresToBlock = 3
	grantFormsListCap            = 200

	noticeDatabasePatternRights = "database_pattern_rights"
)

var (
	// databaseGrantFormsInterval: a round each hour; the first one after the
	// start-up quiet window.
	databaseGrantFormsInterval = time.Hour
	// databaseGrantFormsStartupQuiet: longer than an update's health and
	// rollback window, as for remote database access.
	databaseGrantFormsStartupQuiet = 3 * time.Minute
	// databaseGrantFormsBlockedRetry: a row that failed three times is tried
	// again after this.
	databaseGrantFormsBlockedRetry = 24 * time.Hour
	// databaseGrantFormsUpdatePhase reads the latest update attempt to this
	// build (recoveryobs); tests replace it.
	databaseGrantFormsUpdatePhase = recoveryobs.LatestPhaseForTarget
	// databaseGrantFormsHostQuiet: the server is quiet; tests replace it.
	databaseGrantFormsHostQuiet = func(p *Panel, ctx context.Context) bool { return databaseRemoteHostQuiet(p, ctx) }
	// databaseGrantFormsCommit is this build's commit ("unknown" in a
	// development build, which has no release recovery); tests replace it.
	databaseGrantFormsCommit = func() string { return strings.TrimSpace(buildCommit) }
	// databaseGrantFormsNow is the loop's clock; tests replace it.
	databaseGrantFormsNow = time.Now
	// databaseGrantFormsLocal reaches this machine's MariaDB through the
	// Agent; tests replace it.
	databaseGrantFormsLocal = func(p *Panel) grantFormsEngineAccess { return agentGrantForms{p: p} }
	// databaseGrantFormsRemote reaches a registered MariaDB with the record's
	// own account; tests replace it.
	databaseGrantFormsRemote = func(p *Panel, server *core.DatabaseServer) (grantFormsEngineAccess, error) {
		driver, err := p.dbDriverFor(server)
		if err != nil {
			return nil, err
		}
		exec, ok := driver.(interface{ GrantExec() mariadbgrant.Exec })
		if !ok {
			return nil, errors.New("the database server's driver cannot read grants")
		}
		return execGrantForms{run: exec.GrantExec()}, nil
	}
)

func init() {
	registerAccountNoticeKind(noticeDatabasePatternRights, accountNoticeKind{
		Group: notifyGroupSecurity,
		Words: databasePatternRightsNoticeWords,
	})
}

// grantFormsEngineAccess reads an engine and rewrites one row.
type grantFormsEngineAccess interface {
	scan(ctx context.Context) (mariadbgrant.Scan, map[string][]string, error)
	convert(ctx context.Context, req mariadbgrant.ConvertRequest) (string, error)
}

// errGrantFormsBuildMismatch: the Agent is another build (an update is not
// finished); the round waits.
var errGrantFormsBuildMismatch = errors.New("the panel and its Agent are different builds")

// agentGrantForms is this machine's MariaDB, through the Agent.
type agentGrantForms struct{ p *Panel }

func (a agentGrantForms) scan(ctx context.Context) (mariadbgrant.Scan, map[string][]string, error) {
	var resp transport.DatabaseGrantFormsScanResponse
	if err := a.p.callAgentContext(ctx, "Agent.DatabaseGrantFormsScanV1",
		&transport.DatabaseGrantFormsScanRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}, &resp); err != nil {
		return mariadbgrant.Scan{}, nil, &grantFormsAgentError{err}
	}
	switch {
	case resp.BuildMismatch:
		return mariadbgrant.Scan{}, nil, errGrantFormsBuildMismatch
	case resp.Error != "":
		return mariadbgrant.Scan{}, nil, errors.New(resp.Error)
	}
	return resp.Scan, resp.RemoteHosts, nil
}

func (a agentGrantForms) convert(ctx context.Context, req mariadbgrant.ConvertRequest) (string, error) {
	var resp transport.DatabaseGrantFormConvertResponse
	if err := a.p.callAgentContext(ctx, "Agent.DatabaseGrantFormConvertV1",
		&transport.DatabaseGrantFormConvertRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), ConvertRequest: req}, &resp); err != nil {
		return "", &grantFormsAgentError{err}
	}
	switch {
	case resp.BuildMismatch:
		return "", errGrantFormsBuildMismatch
	case resp.Error != "":
		return "", errors.New(resp.Error)
	}
	return resp.Outcome, nil
}

// grantFormsAgentError: the Agent did not answer (or has no such call: an
// older Agent while an update runs).
type grantFormsAgentError struct{ err error }

func (e *grantFormsAgentError) Error() string { return "the Agent did not answer: " + e.err.Error() }
func (e *grantFormsAgentError) Unwrap() error { return e.err }

// execGrantForms is a registered MariaDB reached with its record's account.
type execGrantForms struct{ run mariadbgrant.Exec }

func (e execGrantForms) scan(context.Context) (mariadbgrant.Scan, map[string][]string, error) {
	scan, err := mariadbgrant.ScanEngine(e.run)
	return scan, nil, err
}

func (e execGrantForms) convert(_ context.Context, req mariadbgrant.ConvertRequest) (string, error) {
	return mariadbgrant.Convert(e.run, req, nil)
}

// grantFormsEngine is one MariaDB and the records that point at it.
type grantFormsEngine struct {
	key     string
	local   bool
	servers []*core.DatabaseServer
}

// grantFormsPair is one recorded user given one recorded database.
type grantFormsPair struct{ user, database string }

// startDatabaseGrantForms runs the rewrite rounds until ctx ends.
func (p *Panel) startDatabaseGrantForms(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		startedAt := databaseGrantFormsNow()
		timer := time.NewTimer(databaseGrantFormsStartupQuiet)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			roundCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
			p.runDatabaseGrantFormsRound(roundCtx, startedAt)
			cancel()
			timer.Reset(databaseGrantFormsInterval)
		}
	}()
	return done
}

// databaseGrantFormsHold names why no engine may be changed now ("" when
// it may): the start-up window, an update of the panel that is not verified
// yet, a busy server.
func (p *Panel) databaseGrantFormsHold(ctx context.Context, startedAt time.Time) string {
	if databaseGrantFormsNow().Sub(startedAt) < databaseGrantFormsStartupQuiet {
		return grantFormsReasonStarting
	}
	if commit := databaseGrantFormsCommit(); commit != "" && commit != "unknown" {
		phase, known := databaseGrantFormsUpdatePhase(commit)
		switch {
		case !known:
			return grantFormsReasonUpdateUnknown
		case phase != "" && phase != "succeeded":
			return grantFormsReasonUpdate
		}
	}
	if !databaseGrantFormsHostQuiet(p, ctx) {
		return grantFormsReasonBusy
	}
	return ""
}

// runDatabaseGrantFormsRound reads every MariaDB, records what it found and,
// when nothing holds it, rewrites the rows proven CelikPanel's.
func (p *Panel) runDatabaseGrantFormsRound(ctx context.Context, startedAt time.Time) {
	engines, err := p.databaseGrantFormsEngines(ctx)
	if err != nil {
		log.Printf("database rights: the database servers could not be read: %v", err)
		return
	}
	hold := p.databaseGrantFormsHold(ctx, startedAt)
	for _, engine := range engines {
		p.runDatabaseGrantFormsEngine(ctx, engine, hold)
	}
	p.notifyDatabasePatternRights(ctx)
}

// databaseGrantFormsEngines groups the MariaDB records by the engine they
// reach (databaseServerEngineIdentity).
func (p *Panel) databaseGrantFormsEngines(ctx context.Context) ([]grantFormsEngine, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT ds.id FROM database_servers ds JOIN database_server_types t ON t.id = ds.type_id
		WHERE t.name = 'mariadb' AND COALESCE(ds.status, 'active') <> 'retired' ORDER BY ds.id`)
	if err != nil {
		return nil, err
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	repo := repositories.NewPostgresDatabaseServerRepository(p.db.GetDB())
	byKey := map[string]*grantFormsEngine{}
	var order []string
	for _, id := range ids {
		server, err := repo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		key := databaseServerEngineIdentity(server)
		engine := byKey[key]
		if engine == nil {
			engine = &grantFormsEngine{key: key}
			byKey[key] = engine
			order = append(order, key)
		}
		engine.servers = append(engine.servers, server)
		if databaseRecordIsOurLocalEngine(server) {
			engine.local = true
		}
	}
	out := make([]grantFormsEngine, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out, nil
}

// grantFormsPairs are the recorded users and databases of an engine.
func (p *Panel) grantFormsPairs(ctx context.Context, engine grantFormsEngine) (map[grantFormsPair]bool, error) {
	ids := make([]any, 0, len(engine.servers))
	marks := make([]string, 0, len(engine.servers))
	for _, server := range engine.servers {
		ids = append(ids, server.ID)
		marks = append(marks, "?")
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT u.username, d.name FROM database_user_grants g
		JOIN databases_v2 d ON d.id = g.database_id
		JOIN database_users u ON u.id = g.user_id
		WHERE d.server_id IN (`+strings.Join(marks, ", ")+`)`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pairs := map[grantFormsPair]bool{}
	for rows.Next() {
		var pair grantFormsPair
		if err := rows.Scan(&pair.user, &pair.database); err != nil {
			return nil, err
		}
		pairs[pair] = true
	}
	return pairs, rows.Err()
}

// grantFormsAccess opens the engine: the Agent for this machine's own
// MariaDB, otherwise the first record whose account opens.
func (p *Panel) grantFormsAccess(engine grantFormsEngine) (grantFormsEngineAccess, *core.DatabaseServer, error) {
	if engine.local {
		for _, server := range engine.servers {
			if databaseRecordIsOurLocalEngine(server) {
				return databaseGrantFormsLocal(p), server, nil
			}
		}
	}
	var firstErr error
	for _, server := range engine.servers {
		access, err := databaseGrantFormsRemote(p, server)
		if err == nil {
			return access, server, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, engine.servers[0], firstErr
}

// grantFormsEngineReason classifies why an engine could not be read.
func grantFormsEngineReason(err error) (state, reason string) {
	var agentErr *grantFormsAgentError
	switch {
	case errors.Is(err, errGrantFormsBuildMismatch):
		return grantFormsWaiting, grantFormsReasonAgentVersion
	case errors.As(err, &agentErr):
		if strings.Contains(strings.ToLower(agentErr.Error()), "can't find method") {
			return grantFormsWaiting, grantFormsReasonAgentVersion
		}
		return grantFormsWaiting, grantFormsReasonAgentUnreached
	}
	switch services.ClassifyDatabaseEngineRefusal(err) {
	case services.DatabaseEngineRefusalCredential:
		return grantFormsBlocked, grantFormsReasonCredential
	case services.DatabaseEngineRefusalUnreachable:
		return grantFormsBlocked, grantFormsReasonUnreachable
	}
	return grantFormsBlocked, grantFormsReasonNoRights
}

// runDatabaseGrantFormsEngine is one engine's part of a round.
func (p *Panel) runDatabaseGrantFormsEngine(ctx context.Context, engine grantFormsEngine, hold string) {
	serverID := engine.servers[0].ID
	if engine.local && hold != grantFormsReasonStarting {
		if err := p.requireMatchingAgentBuild(ctx); err != nil {
			p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, grantFormsReasonAgentVersion, "")
			return
		}
	}
	access, server, err := p.grantFormsAccess(engine)
	if err != nil {
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsBlocked, grantFormsReasonCredentialUnset, "")
		log.Printf("database rights on %s: the record's account could not be opened: %v", engine.key, err)
		return
	}
	serverID = server.ID
	scan, remoteHosts, err := access.scan(ctx)
	if err != nil {
		state, reason := grantFormsEngineReason(err)
		p.setGrantFormsEngine(ctx, engine.key, serverID, state, reason, "")
		log.Printf("database rights on %s: the grants could not be read (%s): %v", engine.key, reason, err)
		return
	}
	pairs, err := p.grantFormsPairs(ctx, engine)
	if err != nil {
		log.Printf("database rights on %s: the records could not be read: %v", engine.key, err)
		return
	}
	if err := p.recordGrantFormsScan(ctx, engine, serverID, scan, remoteHosts, pairs); err != nil {
		log.Printf("database rights on %s: what was found could not be recorded: %v", engine.key, err)
		return
	}
	if !scan.MariaDB {
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsListOnly, grantFormsReasonNotMariaDB, scan.Version)
		return
	}
	if hold != "" {
		if pending := p.grantFormsPending(ctx, engine.key); pending > 0 {
			p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, hold, scan.Version)
		} else {
			p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsDone, "", scan.Version)
		}
		return
	}
	// The same gate as every other step that changes an account's rights on
	// this engine (a database deletion, a grant): one change at a time.
	release, err := acquireDatabaseEngineGate(ctx, server, databaseEngineLinkWait)
	if err != nil {
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, grantFormsReasonBusy, scan.Version)
		return
	}
	defer release()
	p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsRunning, "", scan.Version)
	blocked, retry := p.convertGrantFormRows(ctx, engine.key, access)
	switch {
	case blocked:
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsBlocked, grantFormsReasonConvertFailed, scan.Version)
	case retry:
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, grantFormsReasonConvertRetry, scan.Version)
	case p.grantFormsPending(ctx, engine.key) > 0:
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, grantFormsReasonBusy, scan.Version)
	default:
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsDone, "", scan.Version)
	}
}

// recordGrantFormsScan writes what one read found: a before-image for every
// row proven CelikPanel's that is not yet captured (or no longer matches its
// capture), and the current list of the rows that are left as they are.
func (p *Panel) recordGrantFormsScan(ctx context.Context, engine grantFormsEngine, serverID int, scan mariadbgrant.Scan,
	remoteHosts map[string][]string, pairs map[grantFormsPair]bool) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := map[int64]bool{}
	for _, row := range scan.Patterns {
		name, pattern, decoded := mariadbgrant.Decode(row.Stored)
		ours := decoded && pattern && pairs[grantFormsPair{row.User, name}] && grantFormsHostIsOurs(row.Host, remoteHosts[row.User])
		kind, state, reason := "not_ours", "left", grantFormsReasonNotProvable
		database := sql.NullString{}
		switch {
		case ours && (row.GrantOption || row.Unnamed || row.Privileges == ""):
			kind, reason, database = "owner_row", grantFormsReasonOwnerRow, sql.NullString{String: name, Valid: true}
		case ours && !scan.MariaDB:
			kind, reason, database = "ours", grantFormsReasonNotMariaDB, sql.NullString{String: name, Valid: true}
		case ours:
			kind, state, reason, database = "ours", "captured", "", sql.NullString{String: name, Valid: true}
		}
		reached, _ := json.Marshal(nonNilStrings(row.Reached))
		var id int64
		var oldState, oldFlags string
		err := tx.QueryRowContext(ctx, `SELECT id, state, flags FROM database_grant_forms WHERE engine_key = ? AND username = ? AND host = ? AND stored_name = ?`,
			engine.key, row.User, row.Host, row.Stored).Scan(&id, &oldState, &oldFlags)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			result, err := tx.ExecContext(ctx, `INSERT INTO database_grant_forms
				(engine_key, server_id, username, host, stored_name, database_name, privileges, flags, grant_option, kind, state, reached, reason)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				engine.key, serverID, row.User, row.Host, row.Stored, database, row.Privileges, row.Flags, boolInt(row.GrantOption), kind, state, string(reached), reason)
			if err != nil {
				return err
			}
			id, _ = result.LastInsertId()
		case err != nil:
			return err
		case kind == "ours" && state == "captured" && (oldState == "captured" || oldState == "blocked") && oldFlags == row.Flags:
			// Captured already with the same before-image: kept as it is (a
			// blocked row keeps its failures and waits its day).
		default:
			// A row that reappeared after it was done, changed since it was
			// captured, or is listed: its capture or its listing is renewed.
			if _, err := tx.ExecContext(ctx, `UPDATE database_grant_forms
				SET server_id = ?, database_name = ?, privileges = ?, flags = ?, grant_option = ?, kind = ?, state = ?, reached = ?, reason = ?,
				    failures = 0, updated_at = datetime('now'),
				    captured_at = CASE WHEN ? = 'captured' THEN datetime('now') ELSE captured_at END
				WHERE id = ?`,
				serverID, database, row.Privileges, row.Flags, boolInt(row.GrantOption), kind, state, string(reached), reason, state, id); err != nil {
				return err
			}
		}
		seen[id] = true
	}
	// Listed rows the engine no longer has are gone (their owner changed
	// them): the list says only what is there now.
	rows, err := tx.QueryContext(ctx, `SELECT id FROM database_grant_forms WHERE engine_key = ? AND state = 'left'`, engine.key)
	if err != nil {
		return err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !seen[id] {
			stale = append(stale, id)
		}
	}
	rows.Close()
	for _, id := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM database_grant_forms WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// grantFormsHostIsOurs: localhost, or a remote copy the Agent's remote
// access record lists for that user.
func grantFormsHostIsOurs(host string, remote []string) bool {
	if host == "localhost" {
		return true
	}
	for _, h := range remote {
		if h == host {
			return true
		}
	}
	return false
}

// grantFormsPending counts the rows still to be rewritten.
func (p *Panel) grantFormsPending(ctx context.Context, key string) int {
	var n int
	_ = p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_grant_forms WHERE engine_key = ? AND kind = 'ours' AND state IN ('captured', 'blocked')`, key).Scan(&n)
	return n
}

// convertGrantFormRows rewrites the captured rows one at a time; it answers
// whether a row is blocked and whether one failed and is tried again next
// round.
func (p *Panel) convertGrantFormRows(ctx context.Context, key string, access grantFormsEngineAccess) (blocked, retry bool) {
	type pending struct {
		id                                  int64
		user, host, stored, database, flags string
		state                               string
		failures                            int
		reached                             string
		updated                             string
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT id, username, host, stored_name, COALESCE(database_name, ''), flags, state, failures, reached, updated_at
		FROM database_grant_forms WHERE engine_key = ? AND kind = 'ours' AND state IN ('captured', 'blocked') ORDER BY id`, key)
	if err != nil {
		log.Printf("database rights on %s: the captured rows could not be read: %v", key, err)
		return false, true
	}
	var list []pending
	for rows.Next() {
		var r pending
		if err := rows.Scan(&r.id, &r.user, &r.host, &r.stored, &r.database, &r.flags, &r.state, &r.failures, &r.reached, &r.updated); err != nil {
			rows.Close()
			return false, true
		}
		list = append(list, r)
	}
	rows.Close()
	for _, r := range list {
		if r.state == "blocked" {
			at, err := time.Parse("2006-01-02 15:04:05", r.updated)
			if err == nil && databaseGrantFormsNow().UTC().Sub(at) < databaseGrantFormsBlockedRetry {
				blocked = true
				continue
			}
		}
		outcome, err := access.convert(ctx, mariadbgrant.ConvertRequest{User: r.user, Host: r.host, Database: r.database, Flags: r.flags})
		if err != nil {
			failures := r.failures + 1
			state := "captured"
			if failures >= grantFormsRowFailuresToBlock {
				state, blocked = "blocked", true
			} else {
				retry = true
			}
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET failures = ?, state = ?, reason = ?, updated_at = datetime('now') WHERE id = ?`,
				failures, state, grantFormsReasonConvertFailed, r.id)
			log.Printf("database rights on %s: %s@%s on %s was not rewritten (attempt %d): %v", key, r.user, r.host, r.database, failures, err)
			if errors.Is(err, errGrantFormsBuildMismatch) {
				return blocked, retry
			}
			continue
		}
		switch outcome {
		case mariadbgrant.Converted, mariadbgrant.AlreadyExact, mariadbgrant.Gone:
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET state = 'done', reason = ?, failures = 0, updated_at = datetime('now') WHERE id = ?`, outcome, r.id)
			if outcome != mariadbgrant.Gone {
				var reached []string
				_ = json.Unmarshal([]byte(r.reached), &reached)
				p.recordAudit(auditRecord{Action: "database.right.exact:" + r.database, ResourceType: "database_grant_form", ResourceID: int(r.id),
					Details: auditDetailsText(map[string]any{"engine": key, "account": r.user + "@" + r.host, "database": r.database,
						"pattern": r.stored, "outcome": outcome, "no_longer_reaches": nonNilStrings(reached)})})
			}
		case mariadbgrant.Changed:
			// Not what was captured: the next round captures it again first.
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET reason = 'changed', updated_at = datetime('now') WHERE id = ?`, r.id)
		case mariadbgrant.OwnerRow:
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET kind = 'owner_row', state = 'left', reason = ?, updated_at = datetime('now') WHERE id = ?`, grantFormsReasonOwnerRow, r.id)
		default:
			log.Printf("database rights on %s: an unknown answer %q for %s@%s", key, outcome, r.user, r.host)
		}
	}
	return blocked, retry
}

// setGrantFormsEngine records where an engine stands.
func (p *Panel) setGrantFormsEngine(ctx context.Context, key string, serverID int, state, reason, version string) {
	failures := 0
	if state == grantFormsBlocked {
		failures = 1
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO database_grant_form_engines (engine_key, server_id, state, reason, failures, version, checked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT (engine_key) DO UPDATE SET server_id = excluded.server_id, state = excluded.state, reason = excluded.reason,
			failures = CASE WHEN excluded.state = 'blocked' THEN database_grant_form_engines.failures + 1 ELSE 0 END,
			version = CASE WHEN excluded.version <> '' THEN excluded.version ELSE database_grant_form_engines.version END,
			checked_at = datetime('now'), updated_at = datetime('now')`,
		key, serverID, state, reason, failures, version); err != nil {
		log.Printf("database rights on %s: its state could not be recorded: %v", key, err)
	}
}

// notifyDatabasePatternRights tells the administrators once, while rows that
// CelikPanel leaves as they are exist; never again for the same reason.
func (p *Panel) notifyDatabasePatternRights(ctx context.Context) {
	var left int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM database_grant_forms WHERE state = 'left'`).Scan(&left); err != nil || left == 0 {
		return
	}
	ids, err := administratorIDs(ctx, p.db.GetDB())
	if err != nil {
		return
	}
	recipients := make([]usageRecipient, 0, len(ids))
	for _, id := range ids {
		recipients = append(recipients, usageRecipient{id, audienceAdministrator})
	}
	p.raiseAccountNotice(ctx, p.db.GetDB(), recipients, noticeDatabasePatternRights, true,
		"database_grant_forms", "1", map[string]any{"rows": left}, "/components/mariadb")
}

// databasePatternRightsNoticeWords: the one notification.
func databasePatternRightsNoticeWords(_ string, body map[string]any) accountNoticeWords {
	return accountNoticeWords{
		TitleEN: "Database rights written as a pattern",
		TextEN: "Some MariaDB rights on this server are written as a name pattern, so an account can also use databases whose names only match it. " +
			"CelikPanel did not make them, so it leaves them as they are. On Components → MariaDB they are listed with the databases they reach and what you can do.",
		TitleTR: "Kalıp olarak yazılmış veritabanı hakları",
		TextTR: "Bu sunucudaki bazı MariaDB hakları bir ad kalıbı olarak yazılmış; bu yüzden hesap, adı yalnız kalıba uyan veritabanlarını da kullanabiliyor. " +
			"Bunları CelikPanel vermediği için olduğu gibi bırakıyor. Bileşenler → MariaDB sayfasında, eriştikleri veritabanları ve yapabilecekleriniz ile birlikte listelenir.",
	}
}

// ---- the administrator's view ----

type grantFormsEngineView struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Local     bool   `json:"local"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Version   string `json:"version,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
	Narrowed  int    `json:"narrowed"`
	Left      int    `json:"left"`
	Waiting   int    `json:"waiting"`
}

type grantFormsRowView struct {
	Engine    string   `json:"engine"`
	Account   string   `json:"account"`
	Pattern   string   `json:"pattern"`
	Database  string   `json:"database,omitempty"`
	Kind      string   `json:"kind"`
	Reason    string   `json:"reason,omitempty"`
	Reached   []string `json:"reached"`
	Statement string   `json:"statement,omitempty"`
	At        string   `json:"at,omitempty"`
}

type grantFormsView struct {
	Engines  []grantFormsEngineView `json:"engines"`
	Narrowed []grantFormsRowView    `json:"narrowed"`
	Left     []grantFormsRowView    `json:"left"`
}

// handleDatabaseGrantForms answers the administrator's view. It only reads
// the panel's records; it never starts work.
func (p *Panel) handleDatabaseGrantForms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	c := currentCaller(r)
	if c == nil || c.APIToken != nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	view, err := p.databaseGrantFormsView(r.Context())
	if err != nil {
		writeServerErrorFor(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(view)
}

func (p *Panel) databaseGrantFormsView(ctx context.Context) (grantFormsView, error) {
	view := grantFormsView{Engines: []grantFormsEngineView{}, Narrowed: []grantFormsRowView{}, Left: []grantFormsRowView{}}
	engines, err := p.databaseGrantFormsEngines(ctx)
	if err != nil {
		return view, err
	}
	db := p.db.GetDB()
	for _, engine := range engines {
		ev := grantFormsEngineView{Key: engine.key, Name: engine.servers[0].Name, Local: engine.local, State: grantFormsWaiting, Reason: grantFormsReasonStarting}
		var checked sql.NullString
		err := db.QueryRowContext(ctx, `SELECT state, reason, version, checked_at FROM database_grant_form_engines WHERE engine_key = ?`, engine.key).
			Scan(&ev.State, &ev.Reason, &ev.Version, &checked)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return view, err
		}
		ev.CheckedAt = checked.String
		if err := db.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN state = 'done' AND reason <> 'gone' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'left' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = 'ours' AND state IN ('captured', 'blocked') THEN 1 ELSE 0 END), 0)
			FROM database_grant_forms WHERE engine_key = ?`, engine.key).Scan(&ev.Narrowed, &ev.Left, &ev.Waiting); err != nil {
			return view, err
		}
		view.Engines = append(view.Engines, ev)
	}
	rows, err := db.QueryContext(ctx, `SELECT engine_key, username, host, stored_name, COALESCE(database_name, ''), kind, state, reason,
		reached, privileges, grant_option, updated_at FROM database_grant_forms
		WHERE state = 'left' OR (state = 'done' AND reason <> 'gone' AND reached <> '[]')
		ORDER BY state DESC, updated_at DESC, id DESC`)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var row grantFormsRowView
		var user, host, state, reached, privileges string
		var grantOption int
		if err := rows.Scan(&row.Engine, &user, &host, &row.Pattern, &row.Database, &row.Kind, &state, &row.Reason, &reached, &privileges, &grantOption, &row.At); err != nil {
			return view, err
		}
		row.Account = user + "@" + host
		_ = json.Unmarshal([]byte(reached), &row.Reached)
		row.Reached = nonNilStrings(row.Reached)
		if state == "left" {
			if len(view.Left) >= grantFormsListCap {
				continue
			}
			row.Statement = mariadbgrant.NarrowStatements(mariadbgrant.Row{User: user, Host: host, Stored: row.Pattern, Privileges: privileges, GrantOption: grantOption == 1})
			view.Left = append(view.Left, row)
			continue
		}
		if len(view.Narrowed) < grantFormsListCap {
			view.Narrowed = append(view.Narrowed, row)
		}
	}
	sort.SliceStable(view.Left, func(i, j int) bool { return view.Left[i].Account < view.Left[j].Account })
	return view, rows.Err()
}
