package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// What a person sees when a database user's limits did not reach it
// (alpha.149 fix round B, decision 4; D-024), and how it resumes:
//
//   - Problems now: one line per outcome present (database_user_limits.go:
//     the Agent not reachable, the database server not running, a refused
//     connection, a refusal of the user, a verified failure, a result not
//     known - "not known yet" after a restart among them), naming the count
//     and the first users, the stored reason as a code worded on the screen
//     (the journal holds the detail), who acts, and Apply again. Reading the
//     list never starts anything.
//   - Apply again (POST /api/v1/metrics/database-limits/apply, administrator
//     through the /api/v1/metrics/ prefix) applies the limits once more to
//     the users the record names, apart from the browser's request; a second
//     press while one runs follows the same run (202); GET says how the run
//     with that id went. Each run has an id, and its users are written as
//     pending before it starts, so a run a restart cut short leaves them
//     "not known yet" with Apply again, never done.
//   - The hourly round (review of PR #370, finding 10) is the same single run
//     for at most dbLimitsRetryPerRound users, never two at once; it starts
//     nothing while a run goes on.
//   - A plan's Save answers which users did not get the plan's database
//     limits (dbLimitsSaveAnswer), waiting a bounded time while the editor
//     shows the step; what is still being applied is said, and Problems now
//     names a user it does not reach.
//
// Sınırları ulaşmayan veritabanı kullanıcıları Şu anki sorunlar'da adlarıyla
// ve nedenleriyle görünür; "Yeniden uygula" ve saatlik tur bir kez daha dener;
// planın Kaydet yanıtı hangi kullanıcılara ulaşmadığını söyler.

const (
	// dbLimitsProblemNames is how many users a line names.
	dbLimitsProblemNames = 3
	// dbLimitsSaveWait bounds how long a plan's Save waits for the users.
	dbLimitsSaveWait = 20 * time.Second
	// dbLimitsSaveNames is how many users a Save answer names.
	dbLimitsSaveNames = 10
	// dbLimitsRetryPerRound bounds the users one hourly round tries again.
	dbLimitsRetryPerRound = 25
)

// dbLimitsLineOrder is the order of the lines, and who acts on each.
var dbLimitsLineOrder = []struct{ state, actor string }{
	{dbLimitAgent, "administrator"},
	{dbLimitPrerequisite, "server_owner"},
	{dbLimitConnection, "administrator"},
	{dbLimitRefused, "administrator"},
	{dbLimitFailed, "administrator"},
	{dbLimitUnknown, "administrator"},
}

// databaseLimitsProblems builds the lines from the record. An unreadable
// record is a line of its own (it never hides the users it names).
func (p *Panel) databaseLimitsProblems(ctx context.Context) []monitoringProblem {
	entries, err := p.dbLimitsUnappliedNow(ctx)
	if err != nil {
		return []monitoringProblem{{
			Key: "database_limits_unapplied:" + dbLimitUnknown, Kind: "database_limits_unapplied", Severity: "warning", Actor: "administrator",
			Name: dbLimitUnknown, Reason: dbReasonRecordUnusable, Value: 0, Action: monitoringProblemAction{Kind: "apply_db_limits"},
		}}
	}
	if len(entries) == 0 {
		return nil
	}
	type group struct {
		names []string
		count int
		since string
		code  string
	}
	groups := map[string]*group{}
	for _, entry := range entries {
		state := entry.State
		known := false
		for _, line := range dbLimitsLineOrder {
			known = known || line.state == state
		}
		if !known {
			state = dbLimitFailed
		}
		g := groups[state]
		if g == nil {
			g = &group{since: entry.At, code: entry.Code}
			groups[state] = g
		}
		g.count++
		if len(g.names) < dbLimitsProblemNames {
			g.names = append(g.names, entry.User)
		}
	}
	var problems []monitoringProblem
	for _, line := range dbLimitsLineOrder {
		g := groups[line.state]
		if g == nil {
			continue
		}
		problems = append(problems, monitoringProblem{
			Key: "database_limits_unapplied:" + line.state, Kind: "database_limits_unapplied", Severity: "warning", Actor: line.actor,
			Subject: strings.Join(g.names, ", "), Name: line.state, Reason: dbLimitsReasonCode(g.code), Value: float64(g.count), Since: g.since,
			Action: monitoringProblemAction{Kind: "apply_db_limits"},
		})
	}
	return problems
}

// dbLimitsReasonCode keeps a line's reason to the codes the screen words.
func dbLimitsReasonCode(code string) string {
	switch code {
	case dbReasonAgent, dbReasonNotRunning, dbReasonConnection, dbReasonPrivileged, dbReasonMember, dbReasonNotRecorded,
		dbReasonAbsent, dbReasonEngine, dbReasonUnknown, dbReasonNotKnownYet, dbReasonServerRecord, dbReasonRecordUnusable:
		return code
	}
	return dbReasonEngine
}

// --- Apply again and the hourly round -------------------------------------------

// dbLimitsRunStatus is the last (or running) Apply again or hourly round.
type dbLimitsRunStatus struct {
	// ID names the run; a page that follows a run it no longer sees (the
	// panel restarted) says "not known yet".
	ID         string `json:"id,omitempty"`
	Running    bool   `json:"running"`
	At         string `json:"at,omitempty"`
	Users      int    `json:"users"`
	NotApplied int    `json:"not_applied"`
}

var dbLimitsRun struct {
	mu     sync.Mutex
	status dbLimitsRunStatus
}

// beginDBLimitsRun marks a run started, or answers the one that runs.
func beginDBLimitsRun(users int) (dbLimitsRunStatus, bool) {
	dbLimitsRun.mu.Lock()
	defer dbLimitsRun.mu.Unlock()
	if dbLimitsRun.status.Running {
		return dbLimitsRun.status, false
	}
	dbLimitsRun.status = dbLimitsRunStatus{ID: dbLimitsRunID(), Running: true, At: time.Now().UTC().Format(time.RFC3339), Users: users}
	beginDBLimitsLiveRun(dbLimitsRun.status.ID)
	return dbLimitsRun.status, true
}

func endDBLimitsRun(notApplied int) {
	dbLimitsRun.mu.Lock()
	defer dbLimitsRun.mu.Unlock()
	endDBLimitsLiveRun(dbLimitsRun.status.ID)
	dbLimitsRun.status.Running = false
	dbLimitsRun.status.NotApplied = notApplied
	dbLimitsRun.status.At = time.Now().UTC().Format(time.RFC3339)
}

func dbLimitsRunNow() dbLimitsRunStatus {
	dbLimitsRun.mu.Lock()
	defer dbLimitsRun.mu.Unlock()
	return dbLimitsRun.status
}

// startDBLimitsRun is how a run is started (a variable for tests).
var startDBLimitsRun = func(run func()) { go run() }

// dbLimitsRetryUsers is the users a run tries again: the record's, or, when
// the record cannot be read, every recorded database user (the run then
// writes the record afresh). limit 0: all of them.
func (p *Panel) dbLimitsRetryUsers(ctx context.Context, limit int) ([]dbLimitUser, bool, error) {
	entries, err := p.dbLimitsUnappliedNow(ctx)
	if err != nil {
		users, listErr := p.dbLimitUsersOf(ctx, p.allSubscriptionIDs(ctx))
		return users, true, listErr
	}
	users := make([]dbLimitUser, 0, len(entries))
	for _, entry := range entries {
		if limit > 0 && len(users) == limit {
			break
		}
		users = append(users, dbLimitUser{subscription: entry.SubscriptionID, server: entry.ServerID, username: entry.User})
	}
	return users, false, nil
}

func (p *Panel) allSubscriptionIDs(ctx context.Context) []int {
	ids, err := p.subscriptionsReachedBy(ctx, "", 0)
	if err != nil {
		return nil
	}
	return ids
}

// startDBLimitsRetry starts one run over users, or answers the one that runs.
// after is called with the run's outcome count when it ends.
func (p *Panel) startDBLimitsRetry(users []dbLimitUser, replace bool, after func(notApplied int)) (dbLimitsRunStatus, bool) {
	status, started := beginDBLimitsRun(len(users))
	if !started {
		return status, false
	}
	p.markDBLimitsPendingReplacing(context.Background(), users, status.ID, replace)
	startDBLimitsRun(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		notApplied := 0
		for _, outcome := range p.applyDBLimitsToUsers(ctx, users, nil) {
			if outcome.State != dbLimitApplied {
				notApplied++
			}
		}
		endDBLimitsRun(notApplied)
		if after != nil {
			after(notApplied)
		}
	})
	return status, true
}

// handleDatabaseLimitsApply: POST applies the limits again to the users the
// record names; GET answers the last run. Administrator only (prefix).
func (p *Panel) handleDatabaseLimitsApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(dbLimitsRunNow())
		return
	case http.MethodPost:
	default:
		w.Header().Set("Allow", "GET, POST")
		writeClientError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if running := dbLimitsRunNow(); running.Running {
		// The run that is going on is followed; nothing starts twice.
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(running)
		return
	}
	users, replace, err := p.dbLimitsRetryUsers(r.Context(), 0)
	if err != nil {
		writeServerError(w, err)
		return
	}
	uid, actingAs := auditPerson(r)
	ip, agent := clientIP(r), r.UserAgent()
	if len(agent) > 300 {
		agent = agent[:300]
	}
	status, _ := p.startDBLimitsRetry(users, replace, func(notApplied int) {
		p.recordAudit(auditRecord{UserID: uid, ActingAs: actingAs, Action: "database.limits.apply_again", ResourceType: "database_user",
			IP: ip, UserAgent: agent, Details: auditDetailsText(map[string]any{"users": len(users), "not_applied": notApplied})})
	})
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(status)
}

// retryDBLimitsRound is the hourly round's step: at most
// dbLimitsRetryPerRound users the limits did not reach are tried again in
// the one run, unless a run goes on; the records of removed users go.
func (p *Panel) retryDBLimitsRound(ctx context.Context) {
	if p.db == nil || p.secrets == nil {
		return
	}
	p.pruneDBLimitRecords(ctx)
	if dbLimitsRunNow().Running {
		return
	}
	users, replace, err := p.dbLimitsRetryUsers(ctx, dbLimitsRetryPerRound)
	if err != nil {
		log.Printf("database user limits: the hourly round could not list the users: %v", err)
		return
	}
	if len(users) == 0 || replace {
		// An unreadable record is started afresh by Apply again only.
		return
	}
	p.startDBLimitsRetry(users, false, nil)
}

// --- A plan's Save -----------------------------------------------------------

// dbLimitsSaveAnswer is what a Save says about the database users.
type dbLimitsSaveAnswer struct {
	Users int `json:"users"`
	// NotApplied names the first users the limits did not reach.
	NotApplied      []dbLimitOutcomeView `json:"not_applied"`
	NotAppliedCount int                  `json:"not_applied_count"`
	// Running counts the users still being given the limits.
	Running int `json:"running"`
	// Unknown: the users could not be listed when the Save answered.
	Unknown bool `json:"unknown,omitempty"`
}

type dbLimitOutcomeView struct {
	User  string `json:"user"`
	State string `json:"state"`
}

// reapplyDBLimitsAfterAnswer is reapplyDBConnectionLimitsAfter for a Save
// that answers: the site slices as before, the database users waited for up
// to dbLimitsSaveWait. nil when the database keys did not change.
// prev: the subscriptions' connection limits before the Save
// (dbConnectionLimitsBefore; second review of PR #370, finding 4).
func (p *Panel) reapplyDBLimitsAfterAnswer(ctx context.Context, ownerKind string, ownerID int, before, after map[string]int, prev map[int]int) *dbLimitsSaveAnswer {
	dbChanged, siteChanged := dbConnectionLimitChanged(before, after), siteLimitsChanged(before, after)
	if !dbChanged && !siteChanged {
		return nil
	}
	subs, err := p.subscriptionsReachedBy(ctx, ownerKind, ownerID)
	if err != nil {
		log.Printf("database connection limits after a change of %s %d: %v", ownerKind, ownerID, err)
		return nil
	}
	if siteChanged {
		p.reapplySiteSlices(p.siteLimitChangeLabel(ctx, ownerKind, ownerID), subs)
	}
	if !dbChanged {
		return nil
	}
	return p.applyDBLimitsAnswering(subs, dbLimitsSaveWait, prev)
}

// applyDBLimitsAnswering applies the limits to the subscriptions' users in
// the background and answers what it knows after at most wait. The users are
// written as pending first (finding 6).
func (p *Panel) applyDBLimitsAnswering(subscriptionIDs []int, wait time.Duration, prev map[int]int) *dbLimitsSaveAnswer {
	answer := &dbLimitsSaveAnswer{NotApplied: []dbLimitOutcomeView{}}
	if p.db == nil || p.secrets == nil {
		return answer
	}
	listCtx, cancelList := context.WithTimeout(context.Background(), 30*time.Second)
	users, err := p.dbLimitUsersOf(listCtx, subscriptionIDs)
	cancelList()
	if err != nil {
		// Not known which users there are: they are given the limits in the
		// background, and Problems now names one they do not reach.
		log.Printf("database user limits: the users could not be listed now: %v", err)
		answer.Unknown = true
		p.reapplyDBConnectionLimitsFrom(subscriptionIDs, prev)
		return answer
	}
	withPrevConnection(users, prev)
	answer.Users = len(users)
	if len(users) == 0 {
		return answer
	}
	run := dbLimitsRunID()
	beginDBLimitsLiveRun(run)
	p.markDBLimitsPending(context.Background(), users, run)
	var mu sync.Mutex
	done := 0
	finished := make(chan struct{})
	go func() {
		defer endDBLimitsLiveRun(run)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		p.applyDBLimitsToUsers(ctx, users, func(outcome dbLimitOutcome) {
			mu.Lock()
			defer mu.Unlock()
			done++
			if outcome.State != dbLimitApplied {
				answer.NotAppliedCount++
				if len(answer.NotApplied) < dbLimitsSaveNames {
					answer.NotApplied = append(answer.NotApplied, dbLimitOutcomeView{User: outcome.User, State: outcome.State})
				}
			}
		})
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(wait):
	}
	mu.Lock()
	defer mu.Unlock()
	shown := *answer
	shown.NotApplied = append([]dbLimitOutcomeView(nil), answer.NotApplied...)
	shown.Running = len(users) - done
	return &shown
}
