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
// (alpha.149 fix round B, decision 4; D-024):
//
//   - Problems now: one line per kind of outcome present - a verified failure
//     (the engine or the Agent refused), an unmet prerequisite (the database
//     server or the Agent does not run) or an unknown result (the answer was
//     lost) - naming the count and the first users, who acts, and Apply
//     again. Reading the list never starts anything.
//   - Apply again (POST /api/v1/metrics/database-limits/apply, administrator
//     through the /api/v1/metrics/ prefix) applies the limits once more to
//     the users the record names, apart from the browser's request; a second
//     press while one runs follows the same run (202); GET says how the last
//     one went. It resumes nothing by itself: the next change of a plan's
//     database limits, a new database user and Apply again reach them.
//   - A plan's Save answers which users did not get the plan's database
//     limits (dbLimitsSaveAnswer): it waits for them a bounded time; what is
//     still being applied is said, and Problems now names a user it does not
//     reach.
//
// Sınırları ulaşmayan veritabanı kullanıcıları Şu anki sorunlar'da adlarıyla
// görünür; "Yeniden uygula" bir kez daha dener; planın Kaydet yanıtı hangi
// kullanıcılara ulaşmadığını söyler.

const (
	// dbLimitsProblemNames is how many users a line names.
	dbLimitsProblemNames = 3
	// dbLimitsSaveWait bounds how long a plan's Save waits for the users.
	dbLimitsSaveWait = 20 * time.Second
	// dbLimitsSaveNames is how many users a Save answer names.
	dbLimitsSaveNames = 10
)

// databaseLimitsProblems builds the lines from the record.
func (p *Panel) databaseLimitsProblems(ctx context.Context) []monitoringProblem {
	entries := p.dbLimitsUnappliedNow(ctx)
	if len(entries) == 0 {
		return nil
	}
	type group struct {
		names []string
		count int
		since string
	}
	groups := map[string]*group{}
	for _, entry := range entries {
		g := groups[entry.State]
		if g == nil {
			g = &group{since: entry.At}
			groups[entry.State] = g
		}
		g.count++
		if len(g.names) < dbLimitsProblemNames {
			g.names = append(g.names, entry.User)
		}
	}
	var problems []monitoringProblem
	for _, state := range []string{dbLimitFailed, dbLimitPrerequisite, dbLimitUnknown} {
		g := groups[state]
		if g == nil {
			continue
		}
		actor := "administrator"
		if state == dbLimitPrerequisite {
			actor = "server_owner"
		}
		problems = append(problems, monitoringProblem{
			Key: "database_limits_unapplied:" + state, Kind: "database_limits_unapplied", Severity: "warning", Actor: actor,
			Subject: strings.Join(g.names, ", "), Name: state, Value: float64(g.count), Since: g.since,
			Action: monitoringProblemAction{Kind: "apply_db_limits"},
		})
	}
	return problems
}

// --- Apply again -------------------------------------------------------------

// dbLimitsRunStatus is the last (or running) Apply again.
type dbLimitsRunStatus struct {
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
	dbLimitsRun.status = dbLimitsRunStatus{Running: true, At: time.Now().UTC().Format(time.RFC3339), Users: users}
	return dbLimitsRun.status, true
}

func endDBLimitsRun(notApplied int) {
	dbLimitsRun.mu.Lock()
	defer dbLimitsRun.mu.Unlock()
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
	entries := p.dbLimitsUnappliedNow(r.Context())
	users := make([]dbLimitUser, 0, len(entries))
	for _, entry := range entries {
		users = append(users, dbLimitUser{subscription: entry.SubscriptionID, server: entry.ServerID, username: entry.User})
	}
	status, started := beginDBLimitsRun(len(users))
	if !started {
		// The run that is going on is followed; nothing starts twice.
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(status)
		return
	}
	uid, actingAs := auditPerson(r)
	ip, agent := clientIP(r), r.UserAgent()
	if len(agent) > 300 {
		agent = agent[:300]
	}
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
		p.recordAudit(auditRecord{UserID: uid, ActingAs: actingAs, Action: "database.limits.apply_again", ResourceType: "database_user",
			IP: ip, UserAgent: agent, Details: auditDetailsText(map[string]any{"users": len(users), "not_applied": notApplied})})
	})
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(status)
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
func (p *Panel) reapplyDBLimitsAfterAnswer(ctx context.Context, ownerKind string, ownerID int, before, after map[string]int) *dbLimitsSaveAnswer {
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
	return p.applyDBLimitsAnswering(subs, dbLimitsSaveWait)
}

// applyDBLimitsAnswering applies the limits to the subscriptions' users in
// the background and answers what it knows after at most wait.
func (p *Panel) applyDBLimitsAnswering(subscriptionIDs []int, wait time.Duration) *dbLimitsSaveAnswer {
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
		p.reapplyDBConnectionLimits(subscriptionIDs)
		return answer
	}
	answer.Users = len(users)
	if len(users) == 0 {
		return answer
	}
	var mu sync.Mutex
	done := 0
	finished := make(chan struct{})
	go func() {
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
