package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// The live step of a running change (owner's firm order, 6 Nov 2026: "never
// leave the user staring while work runs; say, like the update screen does,
// 'doing this, doing that'"). alpha.126 gave domain deletion its real steps
// through a store of its own (domain_deletion_running.go). This is the shared
// store for every other change that runs under a request identity (D-029):
// the handler names its plan and the step it is at, and the progress window
// reads them with GET /api/v1/operation-progress/{request id}.
//
// - The step belongs to the request identity the guard already gives the
//   change, so the screen asks about exactly the change it sent and nothing
//   else; a reload asks again with the same identity (kept in the page's
//   session) and is shown the step it is at now, or how it ended.
// - Reading never starts, retries or changes anything (D-024): the endpoint
//   answers from memory and from the stored row only. A `running` row whose
//   process is gone is answered "unknown" without being rewritten.
// - Only the person who sent the change can read it (the row's actor).
// - The step is memory only: a moment of a running request, never saved
//   evidence. A restart drops it with the request; the row then says the
//   outcome is unknown, which is the truth. The last step is kept for a
//   while after the request ends, so the window can say where a failure
//   stopped (also sent as the answer's X-CelikPanel-Operation-Step header).
// - Step names are short stable codes; the screen owns their words.
//
// Çalışan bir değişikliğin şu anki adımı. İşleyici planını ve bulunduğu
// adımı söyler; ilerleme penceresi bunları isteğin kimliğiyle okur. Okuma
// hiçbir şeyi başlatmaz ya da değiştirmez; yalnız isteği gönderen kişi okur.
// Adım yalnız bellektedir, kanıt olarak saklanmaz.

const (
	operationProgressPath = "/api/v1/operation-progress/"
	// operationStepHeader carries, on the change's own answer, the step it
	// ended at (the failed step when the answer is a refusal).
	operationStepHeader = "X-CelikPanel-Operation-Step"
	// How long the last step of an ended change is kept, and how many.
	operationStepRetention = 30 * time.Minute
	operationStepRecentMax = 512

	operationProgressRunning = "running"
	operationProgressDone    = "done"
	operationProgressUnknown = "unknown"

	errCodeOperationNotFound = "OPERATION_NOT_FOUND"
)

const operationNotFoundMessage = "No change with this identity was sent from this account, or it ended more than a day ago. " +
	"Nothing was started. Reload the page to see the current state."

// operationSteps is what a handler said about its progress.
type operationSteps struct {
	plan  []string
	step  string
	since time.Time
}

func (s operationSteps) clone() operationSteps {
	s.plan = append([]string(nil), s.plan...)
	return s
}

type recentOperationSteps struct {
	actor int
	steps operationSteps
	ended time.Time
}

// validOperationStep keeps step codes short and plain: lowercase letters,
// digits and underscores. A handler bug can never put text on the screen.
func validOperationStep(step string) bool {
	if step == "" || len(step) > 40 {
		return false
	}
	for _, c := range step {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// reportOperationPlan names the steps this run will go through, in order. A
// handler calls it once it knows which steps apply (a same-version PHP save
// has no pool move). Without a request identity it does nothing.
// reportOperationPlan bu çalışmanın sırayla geçeceği adımları adlandırır.
func reportOperationPlan(ctx context.Context, plan ...string) {
	scope, ok := ctx.Value(requestIdentityContextKey{}).(*requestIdentityScope)
	if !ok {
		return
	}
	kept := make([]string, 0, len(plan))
	for _, step := range plan {
		if validOperationStep(step) {
			kept = append(kept, step)
		}
	}
	scope.mu.Lock()
	scope.steps.plan = kept
	scope.mu.Unlock()
}

// reportOperationStep records the step the change is starting now. Called
// only by the handler that runs the change, before the step's work.
// reportOperationStep değişikliğin şimdi başladığı adımı kaydeder.
func reportOperationStep(ctx context.Context, step string) {
	scope, ok := ctx.Value(requestIdentityContextKey{}).(*requestIdentityScope)
	if !ok || !validOperationStep(step) {
		return
	}
	scope.mu.Lock()
	if scope.steps.step != step {
		scope.steps.step = step
		scope.steps.since = time.Now()
	}
	scope.mu.Unlock()
	if seen := operationStepSeen; seen != nil {
		seen(scope.id, step)
	}
}

// operationStepSeen lets a test see every step a handler reports, in order.
// Nil outside tests.
var operationStepSeen func(id, step string)

func (s *requestIdentityScope) currentSteps() operationSteps {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.steps.clone()
}

// trackSteps makes a running scope readable by its actor.
func (g *requestIdentityGuard) trackSteps(scope *requestIdentityScope) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.running == nil {
		g.running = make(map[string]*requestIdentityScope)
	}
	g.running[scope.id] = scope
}

// settleSteps keeps the last step of an ended request for a while.
func (g *requestIdentityGuard) settleSteps(scope *requestIdentityScope) {
	steps := scope.currentSteps()
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.running, scope.id)
	if steps.step == "" {
		return
	}
	if g.recent == nil {
		g.recent = make(map[string]recentOperationSteps)
	}
	for id, entry := range g.recent {
		if now.Sub(entry.ended) > operationStepRetention {
			delete(g.recent, id)
		}
	}
	if len(g.recent) >= operationStepRecentMax {
		oldestID, oldest := "", now
		for id, entry := range g.recent {
			if !entry.ended.After(oldest) {
				oldestID, oldest = id, entry.ended
			}
		}
		delete(g.recent, oldestID)
	}
	g.recent[scope.id] = recentOperationSteps{actor: scope.actor, steps: steps, ended: now}
}

type operationProgressView struct {
	RequestID      string          `json:"request_id"`
	State          string          `json:"state"`
	Plan           []string        `json:"plan,omitempty"`
	Step           string          `json:"step,omitempty"`
	StepStartedAt  string          `json:"step_started_at,omitempty"`
	ResponseStatus int             `json:"response_status,omitempty"`
	Answer         json.RawMessage `json:"answer,omitempty"`
}

func (v *operationProgressView) setSteps(steps operationSteps) {
	v.Plan = steps.plan
	v.Step = steps.step
	if !steps.since.IsZero() {
		v.StepStartedAt = steps.since.UTC().Format(time.RFC3339)
	}
}

// progress reads what is known about one change of this actor. It never
// writes: not even a `running` row with no process behind it is changed.
// progress bu kişinin bir değişikliği hakkında bilineni okur; asla yazmaz.
func (g *requestIdentityGuard) progress(ctx context.Context, id string, actor int) (operationProgressView, bool, error) {
	view := operationProgressView{RequestID: id}
	g.mu.Lock()
	scope := g.running[id]
	recent, haveRecent := g.recent[id]
	_, inFlight := g.inFlight[id]
	g.mu.Unlock()
	if scope != nil {
		if scope.actor != actor {
			return view, false, nil
		}
		view.State = operationProgressRunning
		view.setSteps(scope.currentSteps())
		return view, true, nil
	}
	row, err := g.load(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return view, false, nil
	}
	if err != nil {
		return view, false, err
	}
	if row.actor != actor {
		return view, false, nil
	}
	if haveRecent && recent.actor == actor {
		view.setSteps(recent.steps)
	}
	switch row.status {
	case requestIdentityRunning:
		if inFlight {
			view.State = operationProgressRunning
		} else {
			view.State = operationProgressUnknown
		}
	case requestIdentityDone:
		view.State = operationProgressDone
		view.ResponseStatus = row.code
		if row.retained && strings.Contains(strings.ToLower(row.contentType), "json") && json.Valid(row.body) {
			view.Answer = json.RawMessage(row.body)
		}
	default:
		view.State = operationProgressUnknown
	}
	return view, true, nil
}

// handleOperationProgress answers GET /api/v1/operation-progress/{request id}.
func (p *Panel) handleOperationProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, operationProgressPath)
	caller := currentCaller(r)
	if caller == nil {
		writeCodedError(w, http.StatusUnauthorized, errCodeAuthRequired, "authentication required", "")
		return
	}
	if !validRequestIdentity(id) || p.requestIdentities == nil {
		writeCodedError(w, http.StatusNotFound, errCodeOperationNotFound, operationNotFoundMessage, "")
		return
	}
	ctx, cancel := p.requestIdentities.bookkeepingContext()
	defer cancel()
	view, found, err := p.requestIdentities.progress(ctx, id, caller.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !found {
		writeCodedError(w, http.StatusNotFound, errCodeOperationNotFound, operationNotFoundMessage, "")
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}
