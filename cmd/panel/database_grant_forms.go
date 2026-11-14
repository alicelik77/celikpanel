package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

// Rights by exact name (alpha.134, docs/DATABASE-USERS.md, migration 079).
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
	grantFormsReasonStarting       = "starting"
	grantFormsReasonUpdate         = "update_verifying"
	grantFormsReasonUpdateUnknown  = "update_unknown"
	grantFormsReasonBusy           = "server_busy"
	grantFormsReasonAgentVersion   = "agent_version"
	grantFormsReasonAgentUnreached = "agent_unreachable"
	// grantFormsReasonRecordsUnread (second review): CelikPanel's own records
	// of which user has which database could not be read; whose rights are
	// CelikPanel's is not known this round, so nothing was changed.
	grantFormsReasonRecordsUnread   = "records_unread"
	grantFormsReasonCredential      = "credential_refused"
	grantFormsReasonUnreachable     = "engine_unreachable"
	grantFormsReasonNoRights        = "no_grant_rights"
	grantFormsReasonConvertFailed   = "convert_failed"
	grantFormsReasonConvertRetry    = "convert_retry"
	grantFormsReasonNotMariaDB      = "not_mariadb"
	grantFormsReasonNotProvable     = "not_provable"
	grantFormsReasonOwnerRow        = "owner_row"
	grantFormsReasonCredentialUnset = "credential_unreadable"
	// alpha.134 review: what was found could not be recorded; the
	// administrator reads why in the log.
	grantFormsReasonRecordFailed = "record_failed"
	// The name's exact form does not fit a grant row (mariadbgrant.ErrExactTooLong).
	grantFormsReasonNameTooLong = "name_too_long"
	// grantFormsReasonDatabaseMissing: CelikPanel records the pair, but no
	// database of that name exists on the engine (dropped by hand). Nothing
	// is granted; the row is listed until the record is deleted (which runs
	// the removal) or the database is made again.
	grantFormsReasonDatabaseMissing = "database_missing"
	// grantFormsReachedCap: at most this many names are kept per row; the
	// rest is counted.
	grantFormsReachedCap = 50

	grantFormsRowFailuresToBlock = 3
	grantFormsListCap            = 200

	noticeDatabasePatternRights = "database_pattern_rights"
	// noticeDatabaseRightsWaiting (second review): the rewrite has waited for
	// a confirmed panel update longer than databaseGrantFormsWaitNotice.
	noticeDatabaseRightsWaiting = "database_rights_waiting"
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
	// databaseGrantFormsWaitNotice: after this long waiting for a confirmed
	// panel update, the administrators are told once.
	databaseGrantFormsWaitNotice = 24 * time.Hour
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
	registerAccountNoticeKind(noticeDatabaseRightsWaiting, accountNoticeKind{
		Group: notifyGroupSecurity,
		Words: databaseRightsWaitingNoticeWords,
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
		startedAt := p.processStartedAt()
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

// processStartedAt is when this panel process started (the first time it is
// asked, which is its start: startDatabaseGrantForms asks at once).
func (p *Panel) processStartedAt() time.Time {
	now := databaseGrantFormsNow().UnixNano()
	p.startedAt.CompareAndSwap(0, now)
	return time.Unix(0, p.startedAt.Load())
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

// grantFormsPairOnRecord: is user still given database on this engine in
// CelikPanel's records (read inside the engine gate, second review)?
func (p *Panel) grantFormsPairOnRecord(ctx context.Context, engine grantFormsEngine, user, database string) (bool, error) {
	pairs, err := p.grantFormsPairs(ctx, engine)
	if err != nil {
		return false, err
	}
	return pairs[grantFormsPair{user, database}], nil
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
			// Second review: say what is true - another build, or no answer.
			reason := grantFormsReasonAgentUnreached
			if strings.Contains(err.Error(), "build mismatch") {
				reason = grantFormsReasonAgentVersion
			}
			p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, reason, "")
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
		// Second review: an unknown result, recorded and shown as such.
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, grantFormsReasonRecordsUnread, scan.Version)
		log.Printf("database rights on %s: the records could not be read: %v", engine.key, err)
		return
	}
	if err := p.recordGrantFormsScan(ctx, engine, serverID, scan, remoteHosts, pairs); err != nil {
		// Never silent (review): the engine says it is blocked, and why.
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsBlocked, grantFormsReasonRecordFailed, scan.Version)
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
	p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsRunning, "", scan.Version)
	blocked, retry, wait := p.convertGrantFormRows(ctx, engine, server, access)
	switch {
	case wait != "":
		p.setGrantFormsEngine(ctx, engine.key, serverID, grantFormsWaiting, wait, scan.Version)
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
		case ours && row.Missing:
			kind, reason, database = "ours", grantFormsReasonDatabaseMissing, sql.NullString{String: name, Valid: true}
		case ours && !scan.MariaDB:
			kind, reason, database = "ours", grantFormsReasonNotMariaDB, sql.NullString{String: name, Valid: true}
		case ours && !mariadbgrant.ExactFits(name):
			// Measured: the exact form would be stored cut short - a right on
			// another name. It stays in the pattern form and is listed.
			kind, reason, database = "ours", grantFormsReasonNameTooLong, sql.NullString{String: name, Valid: true}
		case ours:
			kind, state, reason, database = "ours", "captured", "", sql.NullString{String: name, Valid: true}
		}
		names, more := nonNilStrings(row.Reached), 0
		if len(names) > grantFormsReachedCap {
			names, more = names[:grantFormsReachedCap], len(names)-grantFormsReachedCap
		}
		reached, _ := json.Marshal(names)
		// Second review: a row holding a privilege CelikPanel cannot name is
		// stored without its words, so its Details never offer a "narrow"
		// statement that would drop that privilege - only its removal.
		privileges := row.Privileges
		if row.Unnamed {
			privileges = ""
		}
		var id int64
		var oldState, oldFlags string
		err := tx.QueryRowContext(ctx, `SELECT id, state, flags FROM database_grant_forms WHERE engine_key = ? AND username = ? AND host = ? AND stored_name = ?`,
			engine.key, row.User, row.Host, row.Stored).Scan(&id, &oldState, &oldFlags)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			result, err := tx.ExecContext(ctx, `INSERT INTO database_grant_forms
				(engine_key, server_id, username, host, stored_name, database_name, privileges, flags, grant_option, kind, state, reached, reached_more, reason)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				engine.key, serverID, row.User, row.Host, row.Stored, database, privileges, row.Flags, boolInt(row.GrantOption), kind, state, string(reached), more, reason)
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
				SET server_id = ?, database_name = ?, privileges = ?, flags = ?, grant_option = ?, kind = ?, state = ?, reached = ?, reached_more = ?, reason = ?,
				    failures = 0, updated_at = datetime('now'),
				    captured_at = CASE WHEN ? = 'captured' THEN datetime('now') ELSE captured_at END
				WHERE id = ?`,
				serverID, database, privileges, row.Flags, boolInt(row.GrantOption), kind, state, string(reached), more, reason, state, id); err != nil {
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
// whether a row is blocked, whether one failed and is tried again next round,
// and why the whole engine waits ("" when it does not). Each row takes the
// engine's gate on its own (review): a deletion or a grant on the same engine
// waits for one row at most, never for the round.
func (p *Panel) convertGrantFormRows(ctx context.Context, engine grantFormsEngine, server *core.DatabaseServer, access grantFormsEngineAccess) (blocked, retry bool, wait string) {
	key := engine.key
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
		return false, true, ""
	}
	var list []pending
	for rows.Next() {
		var r pending
		if err := rows.Scan(&r.id, &r.user, &r.host, &r.stored, &r.database, &r.flags, &r.state, &r.failures, &r.reached, &r.updated); err != nil {
			rows.Close()
			return false, true, ""
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
		release, err := acquireDatabaseEngineGate(ctx, server, databaseEngineLinkWait)
		if err != nil {
			return blocked, retry, grantFormsReasonBusy
		}
		// Second review: inside the gate, the pair must still be on record
		// (a deletion since the scan removes it); otherwise the row leaves
		// this round. The Agent checks that the database exists (BINARY)
		// before it writes the exact right.
		onRecord, err := p.grantFormsPairOnRecord(ctx, engine, r.user, r.database)
		if err != nil {
			release()
			log.Printf("database rights on %s: whether %s on %s is still on record could not be read: %v", key, r.user, r.database, err)
			return blocked, true, ""
		}
		if !onRecord {
			release()
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET state = 'done', reason = ?, updated_at = datetime('now') WHERE id = ?`, mariadbgrant.DatabaseGone, r.id)
			continue
		}
		outcome, err := access.convert(ctx, mariadbgrant.ConvertRequest{User: r.user, Host: r.host, Database: r.database, Flags: r.flags})
		release()
		if state, reason := grantFormsEngineWait(err); state != "" {
			// The Agent is another build or does not answer: the engine
			// waits; it is not this row's failure (review).
			return blocked, retry, reason
		}
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
			continue
		}
		switch outcome {
		case mariadbgrant.DatabaseGone:
			// The pair is on record (read above) but the database is gone:
			// nothing is granted; the row is listed with what to do.
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET state = 'left', reason = ?, failures = 0, updated_at = datetime('now') WHERE id = ?`,
				grantFormsReasonDatabaseMissing, r.id)
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
		case mariadbgrant.NameTooLong:
			_, _ = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_forms SET state = 'left', reason = ?, updated_at = datetime('now') WHERE id = ?`, grantFormsReasonNameTooLong, r.id)
		default:
			log.Printf("database rights on %s: an unknown answer %q for %s@%s", key, outcome, r.user, r.host)
		}
	}
	return blocked, retry, ""
}

// grantFormsEngineWait: an error that makes the whole engine wait (the
// Agent is another build or did not answer), with its reason; "" otherwise.
func grantFormsEngineWait(err error) (state, reason string) {
	var agentErr *grantFormsAgentError
	if err == nil || (!errors.Is(err, errGrantFormsBuildMismatch) && !errors.As(err, &agentErr)) {
		return "", ""
	}
	return grantFormsEngineReason(err)
}

// setGrantFormsEngine records where an engine stands.
func (p *Panel) setGrantFormsEngine(ctx context.Context, key string, serverID int, state, reason, version string) {
	failures := 0
	if state == grantFormsBlocked {
		failures = 1
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `
		INSERT INTO database_grant_form_engines (engine_key, server_id, state, reason, failures, version, checked_at, updated_at, waiting_since)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'), CASE WHEN ? IN ('waiting', 'blocked') THEN datetime('now') END)
		ON CONFLICT (engine_key) DO UPDATE SET server_id = excluded.server_id, state = excluded.state, reason = excluded.reason,
			failures = CASE WHEN excluded.state = 'blocked' THEN database_grant_form_engines.failures + 1 ELSE 0 END,
			version = CASE WHEN excluded.version <> '' THEN excluded.version ELSE database_grant_form_engines.version END,
			waiting_since = CASE
				WHEN excluded.state NOT IN ('waiting', 'blocked') THEN NULL
				WHEN database_grant_form_engines.state = excluded.state AND database_grant_form_engines.reason = excluded.reason
					AND database_grant_form_engines.waiting_since IS NOT NULL THEN database_grant_form_engines.waiting_since
				ELSE datetime('now') END,
			checked_at = datetime('now'), updated_at = datetime('now')`,
		key, serverID, state, reason, failures, version, state); err != nil {
		log.Printf("database rights on %s: its state could not be recorded: %v", key, err)
		return
	}
	p.reviewGrantFormsWait(ctx, key)
}

// noteGrantReadBack (alpha.134 follow-up) keeps on the engine's state what
// the last right CelikPanel gave there could read back. An account that may
// not read mysql.db (a registered server's restricted account) cannot verify
// a right: that is recorded once - the MariaDB page says it, with what the
// account would need - and the right stands on the length rule checked before
// it was written; it is never a refusal and never repeated per operation. A
// later read-back that succeeded clears it. Nothing is recorded for a driver
// that does not say (PostgreSQL) or a grant that was not read back.
//
// Second review: a right written in the pattern form (an existing database
// whose exact form does not fit a grant row) is listed at once on the
// MariaDB page, with its reason - also where the engine's own rows cannot be
// read; a later scan renews the same row.
func (p *Panel) noteGrantReadBack(ctx context.Context, server *core.DatabaseServer, driver services.DatabaseDriver, database, user string) {
	reader, ok := driver.(services.GrantReadBacker)
	if !ok || server == nil {
		return
	}
	key := databaseServerEngineIdentity(server)
	if reader.GrantWasPattern() {
		if _, err := p.db.GetDB().ExecContext(ctx, `
			INSERT INTO database_grant_forms (engine_key, server_id, username, host, stored_name, database_name, kind, state, reason)
			VALUES (?, ?, ?, 'localhost', ?, ?, 'ours', 'left', ?)
			ON CONFLICT (engine_key, username, host, stored_name) DO NOTHING`,
			key, server.ID, user, database, database, grantFormsReasonNameTooLong); err != nil {
			log.Printf("database rights on %s: the pattern-form right of %s on %s could not be listed: %v", key, user, database, err)
		}
	}
	var err error
	switch reader.GrantReadBack() {
	case services.GrantReadBackUnverifiable:
		_, err = p.db.GetDB().ExecContext(ctx, `
			INSERT INTO database_grant_form_engines (engine_key, server_id, state, reason, grants_unverifiable_since, updated_at)
			VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))
			ON CONFLICT (engine_key) DO UPDATE SET
				grants_unverifiable_since = COALESCE(database_grant_form_engines.grants_unverifiable_since, datetime('now'))`,
			key, server.ID, grantFormsWaiting, grantFormsReasonStarting)
	case services.GrantReadBackVerified:
		_, err = p.db.GetDB().ExecContext(ctx, `UPDATE database_grant_form_engines SET grants_unverifiable_since = NULL
			WHERE engine_key = ? AND grants_unverifiable_since IS NOT NULL`, key)
	}
	if err != nil {
		log.Printf("database rights on %s: whether rights there can be verified could not be recorded: %v", key, err)
	}
}

// notifyDatabasePatternRights tells the administrators once, while rows that
// CelikPanel leaves as they are exist; never again for the same reason.
func (p *Panel) notifyDatabasePatternRights(ctx context.Context) {
	counts := map[string]int{}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT reason, COUNT(*) FROM database_grant_forms WHERE state = 'left' GROUP BY reason`)
	if err != nil {
		return
	}
	total := 0
	for rows.Next() {
		var reason string
		var n int
		if rows.Scan(&reason, &n) == nil {
			counts[grantFormsLeftGroup(reason)] += n
			total += n
		}
	}
	rows.Close()
	if total == 0 {
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
	body := map[string]any{"rows": total}
	for group, n := range counts {
		body[group] = n
	}
	p.raiseAccountNotice(ctx, p.db.GetDB(), recipients, noticeDatabasePatternRights, true,
		"database_grant_forms", "1", body, "/components/mariadb")
}

// grantFormsWaitCauses are the waits that need a person once they last
// longer than databaseGrantFormsWaitNotice; every blocked state does too.
var grantFormsWaitCauses = map[string]bool{
	grantFormsReasonUpdate: true, grantFormsReasonUpdateUnknown: true,
	grantFormsReasonAgentUnreached: true, grantFormsReasonAgentVersion: true, grantFormsReasonRecordsUnread: true,
}

// reviewGrantFormsWait (verification of the second review): while an
// engine has waited, or been blocked, for one cause longer than
// databaseGrantFormsWaitNotice, the administrators are told once - the
// cause, who acts and what next (the period is the wait's start, so a later,
// separate wait is told again). A notification whose cause has ended, or
// changed, is cleared (marked seen); it never repeats while the wait lasts.
func (p *Panel) reviewGrantFormsWait(ctx context.Context, key string) {
	db := p.db.GetDB()
	var state, reason string
	var since sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT state, reason, waiting_since FROM database_grant_form_engines WHERE engine_key = ?`, key).
		Scan(&state, &reason, &since); err != nil {
		return
	}
	subject := "database_grant_forms_wait:" + key
	notable := since.Valid && (state == grantFormsBlocked || (state == grantFormsWaiting && grantFormsWaitCauses[reason]))
	current := ""
	if notable {
		current = since.String
	}
	if _, err := db.ExecContext(ctx, `UPDATE account_notices SET seen_at = datetime('now') WHERE kind = ? AND subject_key = ? AND period <> ? AND seen_at IS NULL`,
		noticeDatabaseRightsWaiting, subject, current); err != nil {
		log.Printf("database rights on %s: an ended wait's notification could not be cleared: %v", key, err)
	}
	if !notable {
		return
	}
	at, err := time.Parse("2006-01-02 15:04:05", since.String)
	if err != nil || databaseGrantFormsNow().UTC().Sub(at) < databaseGrantFormsWaitNotice {
		return
	}
	ids, err := administratorIDs(ctx, db)
	if err != nil {
		return
	}
	recipients := make([]usageRecipient, 0, len(ids))
	for _, id := range ids {
		recipients = append(recipients, usageRecipient{id, audienceAdministrator})
	}
	p.raiseAccountNotice(ctx, db, recipients, noticeDatabaseRightsWaiting, true, subject, since.String,
		map[string]any{"state": state, "reason": reason, "rows": p.grantFormsPending(ctx, key)}, "/components/mariadb")
}

// databaseRightsWaitingNoticeWords says why the narrowing waits or is
// blocked, who acts and what next.
func databaseRightsWaitingNoticeWords(_ string, body map[string]any) accountNoticeWords {
	rows, _ := body["rows"].(float64)
	if n, ok := body["rows"].(int); ok {
		rows = float64(n)
	}
	why, whyTR := "it is blocked (the reason is on Components → MariaDB)", "engellendi (nedeni Bileşenler → MariaDB sayfasında)"
	next := "Open Components → MariaDB: it says what blocks the narrowing and what to do."
	nextTR := "Bileşenler → MariaDB sayfasını açın: daraltmayı neyin engellediğini ve ne yapacağınızı söyler."
	updates, updatesTR := "Open Settings → Updates and look at the last update; finish or repeat it.", "Ayarlar → Güncellemeler sayfasını açıp son güncellemeye bakın; bitirin ya da yineleyin."
	switch body["reason"] {
	case grantFormsReasonUpdate:
		why = "the result of the last CelikPanel update is not confirmed yet (it is still finishing its checks, failed, or is being rolled back)"
		whyTR = "son CelikPanel güncellemesinin sonucu henüz doğrulanmadı (denetimlerini bitiriyor, başarısız oldu ya da geri alınıyor)"
		next, nextTR = updates, updatesTR
	case grantFormsReasonUpdateUnknown:
		why, whyTR = "the record of the last CelikPanel update could not be read", "son CelikPanel güncellemesinin kaydı okunamadı"
		next, nextTR = updates, updatesTR
	case grantFormsReasonAgentUnreached:
		why, whyTR = "the CelikPanel Agent does not answer", "CelikPanel Agent yanıt vermiyor"
		next = "Restart the Agent on the server: sudo systemctl restart celikpanel-agent (as root, leave out sudo)."
		nextTR = "Agent’ı sunucuda yeniden başlatın: sudo systemctl restart celikpanel-agent (root olarak oturum açtıysanız sudo yazmayın)."
	case grantFormsReasonAgentVersion:
		why, whyTR = "the CelikPanel Agent is another build than the panel", "CelikPanel Agent panelden farklı bir derleme"
		next, nextTR = "Open Settings → Updates and finish the update, so the panel and its Agent are the same build.", "Ayarlar → Güncellemeler sayfasında güncellemeyi bitirin; panel ve Agent aynı derleme olsun."
	case grantFormsReasonRecordsUnread:
		why, whyTR = "CelikPanel could not read its own records", "CelikPanel kendi kayıtlarını okuyamadı"
		next, nextTR = "Look at CelikPanel’s log for why its database could not be read.", "CelikPanel günlüğünde veritabanının neden okunamadığına bakın."
	}
	return accountNoticeWords{
		TitleEN: "Database rights have waited more than a day",
		TextEN: fmt.Sprintf("For more than a day CelikPanel has not been able to narrow %d database rights it gave to their exact database names, because %s. Nothing was changed meanwhile. "+
			"You act: %s The narrowing then goes on by itself; this notice is cleared when the wait ends.", int(rows), why, next),
		TitleTR: "Veritabanı hakları bir günden uzun süredir bekliyor",
		TextTR: fmt.Sprintf("CelikPanel bir günden uzun süredir verdiği %d veritabanı hakkını tam veritabanı adlarına daraltamıyor; çünkü %s. Bu sürede hiçbir şey değiştirilmedi. "+
			"Siz yaparsınız: %s Daraltma ardından kendiliğinden sürer; bekleme bitince bu bildirim kaldırılır.", int(rows), whyTR, nextTR),
	}
}

// grantFormsLeftGroup is why a listed row is left: CelikPanel did not give it
// (not_provable), it carries rights CelikPanel never gives (owner_row), its
// server is not MariaDB (not_mariadb), or its name is too long for the exact
// form (name_too_long).
func grantFormsLeftGroup(reason string) string {
	switch reason {
	case grantFormsReasonOwnerRow, grantFormsReasonNotMariaDB, grantFormsReasonNameTooLong, grantFormsReasonDatabaseMissing:
		return reason
	}
	return grantFormsReasonNotProvable
}

// databasePatternRightsNoticeWords: the one notification, worded by why the
// rows are left (review: "CelikPanel did not make them" is not true of every
// row).
func databasePatternRightsNoticeWords(_ string, body map[string]any) accountNoticeWords {
	count := func(key string) int {
		n, _ := body[key].(float64)
		if v, ok := body[key].(int); ok {
			return v
		}
		return int(n)
	}
	var en, tr []string
	if n := count(grantFormsReasonNotProvable); n > 0 {
		en = append(en, fmt.Sprintf("%d that CelikPanel did not give", n))
		tr = append(tr, fmt.Sprintf("CelikPanel’in vermediği %d hak", n))
	}
	if n := count(grantFormsReasonOwnerRow); n > 0 {
		en = append(en, fmt.Sprintf("%d that carry rights CelikPanel never gives (such as passing rights on)", n))
		tr = append(tr, fmt.Sprintf("CelikPanel’in hiç vermediği yetkiler (hak verme yetkisi gibi) taşıyan %d hak", n))
	}
	if n := count(grantFormsReasonNotMariaDB); n > 0 {
		en = append(en, fmt.Sprintf("%d on a database server that is not MariaDB", n))
		tr = append(tr, fmt.Sprintf("MariaDB olmayan bir veritabanı sunucusundaki %d hak", n))
	}
	if n := count(grantFormsReasonDatabaseMissing); n > 0 {
		en = append(en, fmt.Sprintf("%d naming a database CelikPanel still records but that no longer exists on the server (delete it in Databases, which removes these rights, or create it again)", n))
		tr = append(tr, fmt.Sprintf("CelikPanel’in hâlâ kaydını tuttuğu ama sunucuda artık olmayan bir veritabanını adlandıran %d hak (Veritabanları’nda silin; bu hakları da kaldırır; ya da veritabanını yeniden oluşturun)", n))
	}
	if n := count(grantFormsReasonNameTooLong); n > 0 {
		en = append(en, fmt.Sprintf("%d whose database name is too long for its exact form", n))
		tr = append(tr, fmt.Sprintf("veritabanı adı tam biçim için fazla uzun olan %d hak", n))
	}
	return accountNoticeWords{
		TitleEN: "Database rights written as a pattern",
		TextEN: "Some MariaDB rights on this server are written as a name pattern, so an account can also use databases whose names only match it. " +
			"CelikPanel leaves these as they are: " + strings.Join(en, "; ") + ". " +
			"On Components → MariaDB they are listed with the databases they reach and what you can do.",
		TitleTR: "Kalıp olarak yazılmış veritabanı hakları",
		TextTR: "Bu sunucudaki bazı MariaDB hakları bir ad kalıbı olarak yazılmış; bu yüzden hesap, adı yalnız kalıba uyan veritabanlarını da kullanabiliyor. " +
			"CelikPanel bunları olduğu gibi bırakıyor: " + strings.Join(tr, "; ") + ". " +
			"Bileşenler → MariaDB sayfasında, eriştikleri veritabanları ve yapabilecekleriniz ile birlikte listelenir.",
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
	// GrantsUnverifiableSince (alpha.134 follow-up): since when a right
	// CelikPanel gave on this server could not be read back, because the
	// account it connects with may not read mysql.db (empty: not so).
	GrantsUnverifiableSince string `json:"grants_unverifiable_since,omitempty"`
}

type grantFormsRowView struct {
	Engine  string `json:"engine"`
	Account string `json:"account"`
	// AccountKind: "account", "role" (an empty host) or "anonymous" (an empty
	// user name).
	AccountKind string   `json:"account_kind"`
	Pattern     string   `json:"pattern"`
	Database    string   `json:"database,omitempty"`
	Kind        string   `json:"kind"`
	Reason      string   `json:"reason,omitempty"`
	Reached     []string `json:"reached"`
	ReachedMore int      `json:"reached_more,omitempty"`
	Statement   string   `json:"statement,omitempty"`
	// StatementKind: "narrow" (the same rights by the one database's exact
	// name) or "remove" (takes the right away entirely).
	StatementKind string `json:"statement_kind,omitempty"`
	At            string `json:"at,omitempty"`
}

type grantFormsView struct {
	Engines  []grantFormsEngineView `json:"engines"`
	Narrowed []grantFormsRowView    `json:"narrowed"`
	Left     []grantFormsRowView    `json:"left"`
	// Blocked (review): CelikPanel's rows that failed three times, with the
	// statement to narrow them by hand.
	Blocked []grantFormsRowView `json:"blocked"`
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
	view := grantFormsView{Engines: []grantFormsEngineView{}, Narrowed: []grantFormsRowView{}, Left: []grantFormsRowView{}, Blocked: []grantFormsRowView{}}
	engines, err := p.databaseGrantFormsEngines(ctx)
	if err != nil {
		return view, err
	}
	db := p.db.GetDB()
	for _, engine := range engines {
		ev := grantFormsEngineView{Key: engine.key, Name: engine.servers[0].Name, Local: engine.local, State: grantFormsWaiting, Reason: grantFormsReasonStarting}
		var checked, unverifiable sql.NullString
		err := db.QueryRowContext(ctx, `SELECT state, reason, version, checked_at, grants_unverifiable_since FROM database_grant_form_engines WHERE engine_key = ?`, engine.key).
			Scan(&ev.State, &ev.Reason, &ev.Version, &checked, &unverifiable)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return view, err
		}
		ev.CheckedAt, ev.GrantsUnverifiableSince = checked.String, unverifiable.String
		if err := db.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN state = 'done' AND reason NOT IN ('gone', 'database_gone') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN state = 'left' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = 'ours' AND state IN ('captured', 'blocked') THEN 1 ELSE 0 END), 0)
			FROM database_grant_forms WHERE engine_key = ?`, engine.key).Scan(&ev.Narrowed, &ev.Left, &ev.Waiting); err != nil {
			return view, err
		}
		view.Engines = append(view.Engines, ev)
	}
	rows, err := db.QueryContext(ctx, `SELECT engine_key, username, host, stored_name, COALESCE(database_name, ''), kind, state, reason,
		reached, reached_more, privileges, grant_option, updated_at FROM database_grant_forms
		WHERE state IN ('left', 'blocked') OR (state = 'done' AND reason NOT IN ('gone', 'database_gone') AND reached <> '[]')
		ORDER BY state DESC, updated_at DESC, id DESC`)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var row grantFormsRowView
		var user, host, state, reached, privileges string
		var grantOption int
		if err := rows.Scan(&row.Engine, &user, &host, &row.Pattern, &row.Database, &row.Kind, &state, &row.Reason, &reached, &row.ReachedMore, &privileges, &grantOption, &row.At); err != nil {
			return view, err
		}
		row.Account = mariadbgrant.Grantee{User: user, Host: host}.String()
		switch {
		case user == "":
			row.AccountKind = "anonymous"
		case host == "":
			row.AccountKind = "role"
		default:
			row.AccountKind = "account"
		}
		_ = json.Unmarshal([]byte(reached), &row.Reached)
		row.Reached = nonNilStrings(row.Reached)
		if state == "left" || state == "blocked" {
			if row.Reason == grantFormsReasonDatabaseMissing {
				privileges = "" // no database to narrow to: only its removal
			}
			row.Statement, row.StatementKind = mariadbgrant.NarrowStatements(mariadbgrant.Row{User: user, Host: host, Stored: row.Pattern, Privileges: privileges, GrantOption: grantOption == 1})
			if state == "blocked" {
				if len(view.Blocked) < grantFormsListCap {
					view.Blocked = append(view.Blocked, row)
				}
				continue
			}
			row.Reason = grantFormsLeftGroup(row.Reason)
			if len(view.Left) < grantFormsListCap {
				view.Left = append(view.Left, row)
			}
			continue
		}
		if len(view.Narrowed) < grantFormsListCap {
			view.Narrowed = append(view.Narrowed, row)
		}
	}
	sort.SliceStable(view.Left, func(i, j int) bool { return view.Left[i].Account < view.Left[j].Account })
	return view, rows.Err()
}
