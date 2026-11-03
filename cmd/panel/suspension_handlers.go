package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// Suspension endpoints (alpha.117):
//
//	GET/POST /api/v1/subscriptions/{id}/suspension   one subscription
//	GET/POST /api/v1/users/{id}/suspension           a customer or a reseller
//	GET      /api/v1/suspensions/deletable           suspended subscriptions
//
// The administrator manages every customer and reseller; a reseller only its
// own customers (canManageUser). A GET only reads: it never starts or
// continues an operation (the worker does).

const suspensionDeletablePath = "/api/v1/suspensions/deletable"

const (
	errCodeSuspensionBusy           = "SUSPENSION_BUSY"
	errCodeSuspensionCauseReseller  = "SUSPENSION_CAUSE_RESELLER"
	errCodeSuspensionReasonRequired = "SUSPENSION_REASON_REQUIRED"
	errCodeSubscriptionSuspended    = "SUBSCRIPTION_SUSPENDED"
)

type suspensionEffectView struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// suspensionGuidance is the D-024 summary shown before the effect list.
type suspensionGuidance struct {
	Result     string `json:"result"`
	Reason     string `json:"reason"`
	WhoActs    string `json:"who_acts"`
	NextAction string `json:"next_action"`
	Resumes    string `json:"resumes"`
}

type suspensionOperationView struct {
	ID         string                 `json:"id"`
	Kind       string                 `json:"kind"`
	Trigger    string                 `json:"trigger"`
	State      string                 `json:"state"`
	Attempts   int                    `json:"attempts"`
	CreatedAt  string                 `json:"created_at"`
	FinishedAt string                 `json:"finished_at,omitempty"`
	Guidance   suspensionGuidance     `json:"guidance"`
	Effects    []suspensionEffectView `json:"effects"`
}

type subscriptionSuspensionView struct {
	SubscriptionID   int                      `json:"subscription_id"`
	SubscriptionName string                   `json:"subscription_name"`
	OwnerID          int                      `json:"owner_id"`
	Owner            string                   `json:"owner"`
	Status           string                   `json:"status"`
	Causes           suspensionCauses         `json:"causes"`
	Reason           string                   `json:"reason,omitempty"`
	SuspendedAt      string                   `json:"suspended_at,omitempty"`
	EndsAt           *string                  `json:"ends_at"`
	Operation        *suspensionOperationView `json:"operation"`
	Preview          suspensionPreview        `json:"preview"`
	CanSuspend       bool                     `json:"can_suspend"`
	CanResume        bool                     `json:"can_resume"`
	ResumeBlocked    string                   `json:"resume_blocked,omitempty"`
}

type accountSuspensionView struct {
	UserID            int                          `json:"user_id"`
	Username          string                       `json:"username"`
	Role              string                       `json:"role"`
	ResellerSuspended bool                         `json:"reseller_suspended"`
	Reason            string                       `json:"reason,omitempty"`
	SuspendedAt       string                       `json:"suspended_at,omitempty"`
	Subscriptions     []subscriptionSuspensionView `json:"subscriptions"`
}

func suspensionGuidanceFor(op suspensionOperationRow, effects []suspensionEffectView) suspensionGuidance {
	verb := "Suspending"
	if op.Kind == suspensionKindResume {
		verb = "Resuming"
	}
	kept := 0
	manual := 0
	for _, e := range effects {
		switch e.State {
		case effectKept:
			kept++
		case effectManual:
			manual++
		}
	}
	switch op.State {
	case "running":
		g := suspensionGuidance{Result: "in_progress", Reason: verb + " the subscription's services on this server.",
			WhoActs: "none", NextAction: "Nothing to do; this page follows the operation.",
			Resumes: "If the panel restarts, the same operation continues where it stopped."}
		if op.LastError != "" {
			g.Reason = verb + " paused after a result that could not be confirmed: " + op.LastError
			g.WhoActs = "panel"
			g.NextAction = "CelikPanel tries again by itself within a minute (at most " + strconv.Itoa(suspensionAutoAttempts) + " attempts)."
		}
		return g
	case "failed":
		g := suspensionGuidance{Reason: op.LastError, WhoActs: "administrator",
			NextAction: "Fix the cause named here, then press Continue.",
			Resumes:    "Continue runs the same operation from the step that failed; finished steps are not repeated."}
		switch op.ErrorKind {
		case "prerequisite":
			g.Result = "prerequisite"
		case "verified_failure":
			g.Result = "verified_failure"
		default:
			g.Result = "unknown"
			g.NextAction = "The result of the last step is not known. Check the server, then press Continue; the step compares before it changes anything."
		}
		return g
	}
	g := suspensionGuidance{Result: "done", Reason: verb + " finished.", WhoActs: "none", NextAction: "Nothing to do.", Resumes: ""}
	if op.Kind == suspensionKindSuspend {
		g.Reason = "The subscription's services are suspended on this server."
	} else {
		g.Reason = "The subscription's services run again."
	}
	if kept > 0 {
		g.WhoActs = "administrator"
		g.NextAction = strconv.Itoa(kept) + " item(s) had been changed on the server by its owner and were left as they are; review them in the list below."
	}
	if manual > 0 {
		g.WhoActs = "administrator"
		g.NextAction = strconv.Itoa(manual) + " database user(s) live on another server; lock or unlock them there."
	}
	return g
}

func (p *Panel) latestSuspensionOperationView(ctx context.Context, subscriptionID int) (*suspensionOperationView, error) {
	var id string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT id FROM suspension_operations WHERE subscription_id = ?
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, subscriptionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	op, err := loadSuspensionOperation(ctx, p.db.GetDB(), id)
	if err != nil {
		return nil, err
	}
	effects, err := loadSuspensionEffects(ctx, p.db.GetDB(), id)
	if err != nil {
		return nil, err
	}
	view := &suspensionOperationView{ID: op.ID, Kind: op.Kind, Trigger: op.Trigger, State: op.State,
		Attempts: op.Attempts, CreatedAt: op.CreatedAt, FinishedAt: op.FinishedAt, Effects: []suspensionEffectView{}}
	for _, e := range effects {
		view.Effects = append(view.Effects, suspensionEffectView{Kind: e.Kind, Target: e.Target, State: e.State, Detail: e.Detail})
	}
	view.Guidance = suspensionGuidanceFor(op, view.Effects)
	return view, nil
}

func (p *Panel) subscriptionSuspensionView(ctx context.Context, subscriptionID int) (subscriptionSuspensionView, error) {
	db := p.db.GetDB()
	view := subscriptionSuspensionView{SubscriptionID: subscriptionID}
	var status sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT s.name, s.owner_id, u.username, s.status
		FROM subscriptions s JOIN users u ON u.id = s.owner_id WHERE s.id = ?`, subscriptionID).
		Scan(&view.SubscriptionName, &view.OwnerID, &view.Owner, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return view, errSuspensionNotFound
	}
	if err != nil {
		return view, err
	}
	view.Status = status.String
	if view.Status == "" {
		view.Status = "active"
	}
	row, err := loadSuspensionRow(ctx, db, subscriptionID)
	if err != nil {
		return view, err
	}
	view.Causes = row.Causes
	if row.Causes.ResellerID > 0 {
		_ = db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, row.Causes.ResellerID).Scan(&view.Causes.Reseller)
	}
	view.Reason, view.SuspendedAt = row.Reason, row.SuspendedAt
	if endsAt, err := subscriptionEndsAt(ctx, db, subscriptionID); err != nil {
		return view, err
	} else if endsAt != "" {
		view.EndsAt = &endsAt
	}
	if view.Operation, err = p.latestSuspensionOperationView(ctx, subscriptionID); err != nil {
		return view, err
	}
	view.CanSuspend = !row.Causes.Manual
	view.CanResume = row.Causes.Manual || row.Causes.Expired || row.Causes.Disk
	if !view.CanResume && row.Causes.ResellerID > 0 {
		view.ResumeBlocked = "Suspended because its reseller " + view.Causes.Reseller + " is suspended. The administrator resumes the reseller; this subscription then resumes with it."
	}
	action := suspensionKindSuspend
	if view.CanResume {
		action = suspensionKindResume
	}
	if _, view.Preview, err = planSuspension(ctx, db, subscriptionID, action); err != nil {
		return view, err
	}
	return view, nil
}

// subscriptionOwnerManageable: the caller may suspend or resume this
// subscription (same 404 for missing and not-yours).
func (p *Panel) subscriptionOwnerManageable(ctx context.Context, c *Caller, subscriptionID int) (bool, error) {
	if c == nil || (!c.hasAccountRole(roleAdmin) && !c.hasAccountRole(roleReseller)) {
		return false, nil
	}
	var ownerID int
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT owner_id FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	owner, err := p.users.GetByID(ctx, ownerID)
	if err != nil || owner == nil {
		return false, nil
	}
	return canManageUser(c, owner), nil
}

type suspensionRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func decodeSuspensionRequest(w http.ResponseWriter, r *http.Request) (suspensionRequest, bool) {
	var req suspensionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Action == suspensionKindSuspend && req.Reason == "" {
		writeCodedError(w, http.StatusBadRequest, errCodeSuspensionReasonRequired,
			"say why the services are suspended; the reason is shown to the account owner", "")
		return req, false
	}
	if len(req.Reason) > suspensionReasonMaxLen {
		writeCodedError(w, http.StatusBadRequest, errCodeSuspensionReasonRequired,
			"the reason is longer than 500 characters", "")
		return req, false
	}
	return req, true
}

func writeSuspensionBusy(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodeSuspensionBusy,
		"another suspend or resume of this subscription is still running; nothing was changed. Wait until it finishes (this page follows it), then try again.", "")
}

// parseSubscriptionSuspensionPath matches /api/v1/subscriptions/{id}/suspension.
func parseSubscriptionSuspensionPath(path string) (int, bool) {
	rest, ok := strings.CutPrefix(path, "/api/v1/subscriptions/")
	if !ok {
		return 0, false
	}
	idText, tail, ok := strings.Cut(rest, "/")
	if !ok || tail != "suspension" {
		return 0, false
	}
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 || strconv.Itoa(id) != idText {
		return 0, false
	}
	return id, true
}

// handleSubscriptionSuspension serves /api/v1/subscriptions/{id}/suspension.
func (p *Panel) handleSubscriptionSuspension(w http.ResponseWriter, r *http.Request, subscriptionID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	c := currentCaller(r)
	ok, err := p.subscriptionOwnerManageable(r.Context(), c, subscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !ok {
		writeClientError(w, http.StatusNotFound, "subscription not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		req, ok := decodeSuspensionRequest(w, r)
		if !ok {
			return
		}
		if !p.applySubscriptionSuspensionAction(w, r, c, subscriptionID, req) {
			return
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
		return
	}
	view, err := p.subscriptionSuspensionView(r.Context(), subscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}

// applySubscriptionSuspensionAction accepts suspend, resume or retry of one
// subscription; it writes the refusal and returns false when it may not.
func (p *Panel) applySubscriptionSuspensionAction(w http.ResponseWriter, r *http.Request, c *Caller, subscriptionID int, req suspensionRequest) bool {
	ctx := r.Context()
	switch req.Action {
	case suspensionKindSuspend:
		yes := true
		if _, err := p.acceptSuspensionChange(ctx, subscriptionID, suspensionCauseChange{
			Manual: &yes, Reason: req.Reason, ActorID: c.ID, Trigger: suspensionTriggerManual,
		}, true); err != nil {
			p.writeSuspensionError(w, err)
			return false
		}
		p.audit(r, "subscription.suspend:"+strconv.Itoa(subscriptionID), "subscription", subscriptionID)
	case suspensionKindResume:
		row, err := loadSuspensionRow(ctx, p.db.GetDB(), subscriptionID)
		if err != nil {
			writeServerError(w, err)
			return false
		}
		if !row.Causes.Manual && !row.Causes.Expired && !row.Causes.Disk && row.Causes.ResellerID > 0 {
			writeCodedError(w, http.StatusConflict, errCodeSuspensionCauseReseller,
				"this subscription is suspended because its reseller is suspended; nothing was changed. The administrator resumes the reseller, and its subscriptions resume with it.", "")
			return false
		}
		no := false
		change := suspensionCauseChange{Manual: &no, ActorID: c.ID, Trigger: suspensionTriggerManual}
		if row.Causes.Expired {
			change.Expired = &no
			change.WaiveExpiry = true
			change.EndsAt = row.ExpiredEndsAt
		}
		if row.Causes.Disk {
			// An operator's resume gives the customer a new 7-day period to
			// free space (D-044); the next measurement over the limit starts
			// the countdown again from now.
			change.Disk = &no
			if err := restartDiskCountdown(ctx, p.db.GetDB(), subscriptionID); err != nil {
				writeServerError(w, err)
				return false
			}
		}
		if _, err := p.acceptSuspensionChange(ctx, subscriptionID, change, true); err != nil {
			p.writeSuspensionError(w, err)
			return false
		}
		p.audit(r, "subscription.resume:"+strconv.Itoa(subscriptionID), "subscription", subscriptionID)
	case "retry":
		if err := p.retrySuspensionOperation(ctx, subscriptionID); err != nil {
			p.writeSuspensionError(w, err)
			return false
		}
	default:
		writeClientError(w, http.StatusBadRequest, "action must be suspend, resume or retry")
		return false
	}
	return true
}

func (p *Panel) writeSuspensionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSuspensionBusy):
		writeSuspensionBusy(w)
	case errors.Is(err, errSuspensionNotFound):
		writeClientError(w, http.StatusNotFound, "subscription not found")
	case errors.Is(err, errSuspensionNothingToRetry):
		writeClientError(w, http.StatusConflict, "there is no stopped operation to continue on this subscription")
	default:
		writeServerError(w, err)
	}
}

var errSuspensionNothingToRetry = errors.New("no failed suspension operation")

// retrySuspensionOperation continues the subscription's failed operation:
// the same operation, from its failed steps; finished steps stay finished.
func (p *Panel) retrySuspensionOperation(ctx context.Context, subscriptionID int) error {
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, state string
	err = tx.QueryRowContext(ctx, `
		SELECT id, state FROM suspension_operations WHERE subscription_id = ?
		ORDER BY created_at DESC, rowid DESC LIMIT 1`, subscriptionID).Scan(&id, &state)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && state == "completed") {
		return errSuspensionNothingToRetry
	}
	if err != nil {
		return err
	}
	if state == "running" {
		// Already running: nothing to start (a repeated press is harmless).
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE suspension_operations SET state = 'running', attempts = 0, finished_at = NULL,
		       updated_at = datetime('now') WHERE id = ? AND state = 'failed'`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE suspension_effects SET state = 'pending', updated_at = datetime('now')
		WHERE operation_id = ? AND state = 'failed'`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.kickSuspensionWorker()
	return nil
}

// accountSubscriptionIDs: a customer's own subscriptions; for a reseller its
// own and every customer's.
func accountSubscriptionIDs(ctx context.Context, q sqlQueryer, userID int, reseller bool) ([]int, error) {
	query := `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id`
	args := []any{userID}
	if reseller {
		query = `SELECT id FROM subscriptions WHERE owner_id = ? OR owner_id IN (SELECT id FROM users WHERE parent_id = ?) ORDER BY id`
		args = append(args, userID)
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (p *Panel) accountSuspensionView(ctx context.Context, target *core.User) (accountSuspensionView, error) {
	view := accountSuspensionView{UserID: target.ID, Username: target.Username, Role: target.Role, Subscriptions: []subscriptionSuspensionView{}}
	reseller := target.Role == roleReseller
	if reseller {
		var reason, since sql.NullString
		err := p.db.GetDB().QueryRowContext(ctx, `SELECT reason, suspended_at FROM reseller_suspensions WHERE reseller_id = ?`, target.ID).Scan(&reason, &since)
		if err == nil {
			view.ResellerSuspended, view.Reason, view.SuspendedAt = true, reason.String, since.String
		} else if !errors.Is(err, sql.ErrNoRows) {
			return view, err
		}
	}
	ids, err := accountSubscriptionIDs(ctx, p.db.GetDB(), target.ID, reseller)
	if err != nil {
		return view, err
	}
	for _, id := range ids {
		sub, err := p.subscriptionSuspensionView(ctx, id)
		if err != nil {
			return view, err
		}
		view.Subscriptions = append(view.Subscriptions, sub)
	}
	return view, nil
}

// handleUserSuspension serves /api/v1/users/{id}/suspension (the caller may
// already manage target: handleUserByID checked canManageUser).
func (p *Panel) handleUserSuspension(w http.ResponseWriter, r *http.Request, c *Caller, target *core.User) {
	w.Header().Set("Cache-Control", "private, no-store")
	if target.Role != roleCustomer && target.Role != roleReseller {
		writeClientError(w, http.StatusNotFound, "user not found")
		return
	}
	if normalizedStoredAccountType(target) != core.AccountTypeAccount {
		writeClientError(w, http.StatusNotFound, "user not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		req, ok := decodeSuspensionRequest(w, r)
		if !ok {
			return
		}
		if req.Action != suspensionKindSuspend && req.Action != suspensionKindResume {
			writeClientError(w, http.StatusBadRequest, "action must be suspend or resume")
			return
		}
		var err error
		if target.Role == roleReseller {
			err = p.setResellerSuspension(r.Context(), c, target.ID, req)
		} else {
			err = p.setCustomerSuspension(r, c, target.ID, req)
		}
		if err != nil {
			p.writeSuspensionError(w, err)
			return
		}
		action := "reseller.suspend:"
		if req.Action == suspensionKindResume {
			action = "reseller.resume:"
		}
		if target.Role == roleCustomer {
			action = "subscription.suspend:"
			if req.Action == suspensionKindResume {
				action = "subscription.resume:"
			}
		}
		p.audit(r, action+target.Username, "user", target.ID)
		w.WriteHeader(http.StatusAccepted)
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
		return
	}
	view, err := p.accountSuspensionView(r.Context(), target)
	if err != nil {
		writeServerError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(view)
}

// setCustomerSuspension suspends or resumes every subscription of a
// customer. A running operation of the other kind refuses before anything
// changes.
func (p *Panel) setCustomerSuspension(r *http.Request, c *Caller, customerID int, req suspensionRequest) error {
	ctx := r.Context()
	db := p.db.GetDB()
	ids, err := accountSubscriptionIDs(ctx, db, customerID, false)
	if err != nil {
		return err
	}
	for _, id := range ids {
		_, kind, err := runningSuspensionOperation(ctx, db, id)
		if err != nil {
			return err
		}
		if kind != "" && kind != req.Action {
			return errSuspensionBusy
		}
	}
	for _, id := range ids {
		row, err := loadSuspensionRow(ctx, db, id)
		if err != nil {
			return err
		}
		var change suspensionCauseChange
		if req.Action == suspensionKindSuspend {
			yes := true
			change = suspensionCauseChange{Manual: &yes, Reason: req.Reason, ActorID: c.ID, Trigger: suspensionTriggerManual}
		} else {
			no := false
			change = suspensionCauseChange{Manual: &no, ActorID: c.ID, Trigger: suspensionTriggerManual}
			if row.Causes.Expired {
				change.Expired, change.WaiveExpiry, change.EndsAt = &no, true, row.ExpiredEndsAt
			}
			if row.Causes.Disk {
				change.Disk = &no
				if err := restartDiskCountdown(ctx, db, id); err != nil {
					return err
				}
			}
		}
		if _, err := p.acceptSuspensionChange(ctx, id, change, true); err != nil {
			return err
		}
	}
	return nil
}

// setResellerSuspension records or removes the reseller's suspension and
// sets or clears the reseller cause on its own subscriptions and on every
// subscription of its customers. A customer suspended by hand keeps its own
// manual cause, so resuming the reseller does not resume it.
func (p *Panel) setResellerSuspension(ctx context.Context, c *Caller, resellerID int, req suspensionRequest) error {
	db := p.db.GetDB()
	if req.Action == suspensionKindSuspend {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO reseller_suspensions (reseller_id, reason, suspended_by) VALUES (?, ?, ?)
			ON CONFLICT(reseller_id) DO UPDATE SET reason = excluded.reason`,
			resellerID, req.Reason, nullablePositiveInt(c.ID)); err != nil {
			return err
		}
	} else {
		if _, err := db.ExecContext(ctx, `DELETE FROM reseller_suspensions WHERE reseller_id = ?`, resellerID); err != nil {
			return err
		}
	}
	ids, err := accountSubscriptionIDs(ctx, db, resellerID, true)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		var change suspensionCauseChange
		if req.Action == suspensionKindSuspend {
			rid := resellerID
			change = suspensionCauseChange{ResellerID: &rid, Reason: req.Reason, ActorID: c.ID, Trigger: suspensionTriggerReseller}
		} else {
			row, err := loadSuspensionRow(ctx, db, id)
			if err != nil {
				return err
			}
			if row.Causes.ResellerID != resellerID {
				continue
			}
			zero := 0
			change = suspensionCauseChange{ResellerID: &zero, ActorID: c.ID, Trigger: suspensionTriggerReseller}
		}
		// A cascade never refuses: a running operation finishes first and
		// the worker then starts the one this change needs.
		if _, err := p.acceptSuspensionChange(ctx, id, change, false); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deletableEntry is one suspended subscription and what deleting it would
// remove (deletion stays the manual, previewed flow; K6).
type deletableEntry struct {
	SubscriptionID   int               `json:"subscription_id"`
	SubscriptionName string            `json:"subscription_name"`
	OwnerID          int               `json:"owner_id"`
	Owner            string            `json:"owner"`
	Reseller         string            `json:"reseller,omitempty"`
	Causes           suspensionCauses  `json:"causes"`
	Reason           string            `json:"reason,omitempty"`
	SuspendedAt      string            `json:"suspended_at,omitempty"`
	Contents         deletableContents `json:"contents"`
	DeleteHint       string            `json:"delete_hint"`
}

type deletableContents struct {
	Domains         []string `json:"domains"`
	Sites           int      `json:"sites"`
	Mailboxes       int      `json:"mailboxes"`
	Databases       int      `json:"databases"`
	DatabaseUsers   int      `json:"database_users"`
	VPNPeers        int      `json:"vpn_peers"`
	BackupSchedules int      `json:"backup_schedules"`
}

const deletableHint = "Deleting is done by hand and nothing is deleted automatically. Delete each domain on its Domains screen (it shows the site files, mailboxes, databases and backups it removes and asks first), remove the remaining databases, database users and VPN devices, then delete the account on the Users screen."

func (p *Panel) handleSuspensionDeletable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	c := currentCaller(r)
	if c == nil || (!c.hasAccountRole(roleAdmin) && !c.hasAccountRole(roleReseller)) {
		writeClientError(w, http.StatusForbidden, "administrator or reseller access required")
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	query := `
		SELECT s.id, s.name, u.id, u.username, COALESCE(pu.username, '')
		FROM subscriptions s
		JOIN users u ON u.id = s.owner_id
		LEFT JOIN users pu ON pu.id = u.parent_id AND pu.role = 'reseller'
		WHERE s.status IN ('suspended', 'expired') AND u.role <> 'admin'`
	args := []any{}
	if c.hasAccountRole(roleReseller) {
		query += ` AND u.parent_id = ?`
		args = append(args, c.ID)
	}
	query += ` ORDER BY u.username, s.name`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	entries := []deletableEntry{}
	for rows.Next() {
		var e deletableEntry
		if err := rows.Scan(&e.SubscriptionID, &e.SubscriptionName, &e.OwnerID, &e.Owner, &e.Reseller); err != nil {
			rows.Close()
			writeServerError(w, err)
			return
		}
		entries = append(entries, e)
	}
	rows.Close()
	for i := range entries {
		e := &entries[i]
		row, err := loadSuspensionRow(ctx, db, e.SubscriptionID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		e.Causes, e.Reason, e.SuspendedAt = row.Causes, row.Reason, row.SuspendedAt
		if row.Causes.ResellerID > 0 {
			_ = db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, row.Causes.ResellerID).Scan(&e.Causes.Reseller)
		}
		e.DeleteHint = deletableHint
		e.Contents.Domains = []string{}
		domains, err := db.QueryContext(ctx, `SELECT name FROM domains WHERE subscription_id = ? ORDER BY name`, e.SubscriptionID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		for domains.Next() {
			var name string
			if domains.Scan(&name) == nil {
				e.Contents.Domains = append(e.Contents.Domains, name)
			}
		}
		domains.Close()
		if err := db.QueryRowContext(ctx, `
			SELECT
			  (SELECT COUNT(*) FROM sites si JOIN domains d ON d.id = si.domain_id WHERE d.subscription_id = ?1),
			  (SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON d.id = ea.domain_id WHERE d.subscription_id = ?1),
			  (SELECT COUNT(*) FROM databases_v2 WHERE subscription_id = ?1) + (SELECT COUNT(*) FROM databases WHERE subscription_id = ?1),
			  (SELECT COUNT(*) FROM database_users WHERE subscription_id = ?1),
			  (SELECT COUNT(*) FROM vpn_peers WHERE subscription_id = ?1 AND desired_state = 'active'),
			  (SELECT COUNT(*) FROM backup_schedules b JOIN domains d ON d.id = b.domain_id WHERE d.subscription_id = ?1)`,
			e.SubscriptionID).Scan(&e.Contents.Sites, &e.Contents.Mailboxes, &e.Contents.Databases,
			&e.Contents.DatabaseUsers, &e.Contents.VPNPeers, &e.Contents.BackupSchedules); err != nil {
			writeServerError(w, err)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"subscriptions": entries})
}

// meSuspension is the plain screen's content in /auth/me.
type meSuspension struct {
	Kind    string    `json:"kind"`
	Reason  string    `json:"reason,omitempty"`
	Since   string    `json:"since,omitempty"`
	Contact myContact `json:"contact"`
}

// accountServiceSuspension says whether the caller sees only the suspended
// screen: a customer (or its team member) whose every subscription is
// suspended, or a suspended reseller. Never during "Log in as": the operator
// must be able to look.
func (p *Panel) accountServiceSuspension(ctx context.Context, c *Caller) (*meSuspension, error) {
	if c == nil || c.ImpersonatorID > 0 {
		return nil, nil
	}
	db := p.db.GetDB()
	switch {
	case c.hasAccountRole(roleReseller):
		var reason, since sql.NullString
		err := db.QueryRowContext(ctx, `SELECT reason, suspended_at FROM reseller_suspensions WHERE reseller_id = ?`, c.ID).Scan(&reason, &since)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		contact, err := suspensionContact(ctx, db, c.ID)
		if err != nil {
			return nil, err
		}
		return &meSuspension{Kind: "reseller", Reason: reason.String, Since: since.String, Contact: contact}, nil
	case c.hasAccountRole(roleCustomer), c.isAdditionalUser():
		accountID := c.ID
		if c.isAdditionalUser() {
			accountID = c.CustomerID
		}
		var total, suspended int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN status IN ('suspended', 'expired') THEN 1 ELSE 0 END), 0)
			FROM subscriptions WHERE owner_id = ?`, accountID).Scan(&total, &suspended); err != nil {
			return nil, err
		}
		if total == 0 || suspended < total {
			return nil, nil
		}
		var reason, since sql.NullString
		err := db.QueryRowContext(ctx, `
			SELECT ss.reason, ss.suspended_at FROM subscription_suspensions ss
			JOIN subscriptions s ON s.id = ss.subscription_id
			WHERE s.owner_id = ? ORDER BY ss.updated_at DESC LIMIT 1`, accountID).Scan(&reason, &since)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		contact, err := suspensionContact(ctx, db, accountID)
		if err != nil {
			return nil, err
		}
		return &meSuspension{Kind: "subscription", Reason: reason.String, Since: since.String, Contact: contact}, nil
	}
	return nil, nil
}

// suspensionContact names who can resume the account: its reseller, else the
// administrator.
func suspensionContact(ctx context.Context, q sqlQueryer, accountID int) (myContact, error) {
	var contact myContact
	var role string
	err := q.QueryRowContext(ctx, `
		SELECT pu.role, pu.username, pu.email
		FROM users u JOIN users pu ON pu.id = u.parent_id
		WHERE u.id = ? AND pu.role IN ('admin', 'reseller')`, accountID).Scan(&role, &contact.Name, &contact.Email)
	if err == nil {
		contact.Kind = "administrator"
		if role == roleReseller {
			contact.Kind = "reseller"
		}
		return contact, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return contact, err
	}
	err = q.QueryRowContext(ctx, `
		SELECT username, email FROM users
		WHERE role = 'admin' AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'
		ORDER BY id LIMIT 1`).Scan(&contact.Name, &contact.Email)
	contact.Kind = "administrator"
	if errors.Is(err, sql.ErrNoRows) {
		return contact, nil
	}
	return contact, err
}

// suspendedAccountAllowedPath: what a suspended account may still call.
func suspendedAccountAllowedPath(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/auth/logout", "/api/v1/auth/unimpersonate",
		panelLicenseAccessPath, panelAvailabilityPath:
		return true
	}
	return false
}

// refuseSuspendedTenantSubscription is the gate of create paths that would
// re-open a suspended subscription's services (a new site or mailbox would
// serve). It writes the refusal and returns true when the subscription is
// suspended.
func (p *Panel) refuseSuspendedTenantSubscription(w http.ResponseWriter, ctx context.Context, subscriptionID int) bool {
	suspended, err := p.subscriptionSuspended(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, errSuspensionNotFound) {
			return false
		}
		writeServerError(w, err)
		return true
	}
	if suspended {
		writeCodedError(w, http.StatusConflict, errCodeSubscriptionSuspended,
			"this subscription is suspended, so nothing new can be added to it; nothing was changed. The administrator or its reseller resumes the subscription first.", "")
		return true
	}
	return false
}
