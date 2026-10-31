package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/alicelik/celikpanel/internal/core"
)

// Assigning or changing an account's plan after creation (alpha.114, ROADMAP
// v0.3 "Plan change"): PUT /api/v1/users/{id}/plan. An account without a
// subscription gets one from the plan; an account with one has its limits
// re-copied from the plan. A change below current usage is refused with the
// list of what is over (409 PLAN_BELOW_USAGE) until the operator confirms it
// with accept_over_usage; then it is applied, nothing is deleted, and new
// creations of the over kinds are refused with guidance until usage is back
// under the limit. The reseller's pool is checked in every case.
//
// Oluşturmadan sonra hesabın planını atama ya da değiştirme. Kullanımın
// altına inen değişiklik, aşan kalemlerin listesiyle reddedilir; operatör
// accept_over_usage ile onaylarsa uygulanır, hiçbir şey silinmez ve aşan
// türlerde yeni oluşturma yönlendirmeyle reddedilir.

const (
	errCodePlanBelowUsage     = "PLAN_BELOW_USAGE"
	errCodePlanNotAvailable   = "PLAN_NOT_AVAILABLE"
	errCodeChooseSubscription = "SUBSCRIPTION_CHOICE_REQUIRED"
)

type planAssignmentRequest struct {
	PlanID          int  `json:"plan_id"`
	SubscriptionID  int  `json:"subscription_id,omitempty"`
	AcceptOverUsage bool `json:"accept_over_usage,omitempty"`
}

type loadedPlan struct {
	ID      int
	Name    string
	OwnerID int
	Limits  planLimits
	Traffic int
}

func loadPlan(ctx context.Context, q rowQueryer, planID int) (loadedPlan, error) {
	var pl loadedPlan
	var owner sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT id, name, owner_id, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb
		FROM service_plans WHERE id = ?`, planID).
		Scan(&pl.ID, &pl.Name, &owner, &pl.Limits.Domains, &pl.Limits.Databases, &pl.Limits.Mailboxes, &pl.Limits.DiskMB, &pl.Traffic)
	if errors.Is(err, sql.ErrNoRows) {
		return pl, errNotFound
	}
	if err != nil {
		return pl, err
	}
	pl.OwnerID = int(owner.Int64)
	return pl, nil
}

// planAssignableTo: a global plan goes to any account the caller manages; a
// reseller's own plan only to that reseller's customers. The caller must be
// able to see the plan (the administrator sees all; a reseller global + own).
// planAssignableTo: genel plan, çağıranın yönettiği her hesaba; bayinin kendi
// planı yalnız o bayinin müşterilerine atanır.
func planAssignableTo(c *Caller, plan loadedPlan, target *core.User) bool {
	if plan.OwnerID != 0 {
		if !c.hasAccountRole(roleAdmin) && plan.OwnerID != c.ID {
			return false
		}
		return target.ParentID != nil && *target.ParentID == plan.OwnerID && target.Role == roleCustomer
	}
	return true
}

func writePlanNotAvailable(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodePlanNotAvailable,
		"this plan cannot be given to this account; choose a global plan or one of your own plans. Nothing was changed.", "")
}

func (p *Panel) handleAssignPlan(w http.ResponseWriter, r *http.Request, c *Caller, target *core.User) {
	if r.Method != http.MethodPut {
		rejectRouteMethod(w, []string{http.MethodPut})
		return
	}
	if target.IsAdditionalUser() {
		writeClientError(w, http.StatusBadRequest, "a team member uses its customer's plan")
		return
	}
	var req planAssignmentRequest
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req.PlanID <= 0 {
		writeClientError(w, http.StatusBadRequest, "plan_id is required")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	plan, err := loadPlan(ctx, db, req.PlanID)
	if errors.Is(err, errNotFound) {
		writePlanNotAvailable(w)
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !planAssignableTo(c, plan, target) {
		writePlanNotAvailable(w)
		return
	}

	// Which subscription: the named one (it must be the target's), the only
	// one, or a new one when there is none.
	// Hangi abonelik: adı verilen, tek olan ya da hiç yoksa yeni bir tane.
	subID := req.SubscriptionID
	var fromPlanID sql.NullInt64
	if subID > 0 {
		err := db.QueryRowContext(ctx, `SELECT plan_id FROM subscriptions WHERE id = ? AND owner_id = ?`, subID, target.ID).Scan(&fromPlanID)
		if errors.Is(err, sql.ErrNoRows) {
			writeClientError(w, http.StatusNotFound, "subscription not found")
			return
		}
		if err != nil {
			writeServerError(w, err)
			return
		}
	} else {
		rows, err := db.QueryContext(ctx, `SELECT id, plan_id FROM subscriptions WHERE owner_id = ? ORDER BY id LIMIT 2`, target.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		count := 0
		for rows.Next() {
			count++
			if err := rows.Scan(&subID, &fromPlanID); err != nil {
				rows.Close()
				writeServerError(w, err)
				return
			}
		}
		rows.Close()
		if count > 1 {
			writeCodedError(w, http.StatusConflict, errCodeChooseSubscription,
				"this account has more than one subscription; choose which one gets the plan", "")
			return
		}
		if count == 0 {
			subID = 0
		}
	}

	_, violations, err := checkResellerPoolFor(ctx, db, target.ID, subID, &plan.Limits, false)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if len(violations) > 0 {
		writePoolRefusal(w, violations)
		return
	}

	if subID == 0 {
		id, err := p.insertSubscriptionFromPlan(ctx, db, target.ID, plan)
		if err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "subscription.plan_assign:"+target.Username, "subscription", id,
			map[string]any{"plan": plan.Name, "plan_id": plan.ID})
		writePlanAssignment(w, id, []string{})
		return
	}

	usage, err := p.subscriptionUsageFor(ctx, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	projected := *usage
	projected.DomainsLimit, projected.DatabasesLimit, projected.MailLimit = plan.Limits.Domains, plan.Limits.Databases, plan.Limits.Mailboxes
	projected.DiskLimitBytes = 0
	if plan.Limits.DiskMB > 0 {
		projected.DiskLimitBytes = int64(plan.Limits.DiskMB) * 1024 * 1024
	}
	over := projected.overLimits()
	if len(over) > 0 && !req.AcceptOverUsage {
		writeCodedErrorDetails(w, http.StatusConflict, errCodePlanBelowUsage,
			"this plan is below what the account uses now. Nothing was changed. To apply it anyway, confirm: nothing is deleted, and new items of these kinds are refused until usage is under the limit.",
			"", over)
		return
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE subscriptions SET plan_id = ?, max_domains = ?, max_databases = ?,
		       max_email_accounts = ?, disk_quota_mb = ?, bandwidth_quota_mb = ?,
		       updated_at = datetime('now')
		WHERE id = ?`,
		plan.ID, plan.Limits.Domains, plan.Limits.Databases, plan.Limits.Mailboxes,
		plan.Limits.DiskMB, plan.Traffic, subID); err != nil {
		writeServerError(w, err)
		return
	}
	details := map[string]any{"plan": plan.Name, "plan_id": plan.ID}
	if fromPlanID.Valid {
		details["from_plan_id"] = fromPlanID.Int64
	}
	if len(over) > 0 {
		details["over_limit"] = over
	}
	p.auditDetailed(r, "subscription.plan_change:"+target.Username, "subscription", subID, details)
	writePlanAssignment(w, subID, over)
}

func writePlanAssignment(w http.ResponseWriter, subID int, over []string) {
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success": true, "subscription_id": subID, "over_limit": over,
	}); err != nil {
		log.Printf("encode plan assignment: %v", err)
	}
}

// insertSubscriptionFromPlan copies a plan's limits into a new subscription.
// insertSubscriptionFromPlan, bir planın sınırlarını yeni bir aboneliğe kopyalar.
func (p *Panel) insertSubscriptionFromPlan(ctx context.Context, db sqlExecer, ownerID int, plan loadedPlan) (int, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (owner_id, name, plan_id, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active')`,
		ownerID, plan.Name, plan.ID, plan.Limits.Domains, plan.Limits.Databases, plan.Limits.Mailboxes, plan.Limits.DiskMB, plan.Traffic)
	if err != nil {
		return 0, fmt.Errorf("create subscription from plan: %w", err)
	}
	id, err := result.LastInsertId()
	return int(id), err
}
