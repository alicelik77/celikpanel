package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
)

// The tenancy rules around creating an account (alpha.114; rights since
// alpha.117, D-042 6.1): which rights the new account gets (Unlimited, a
// plan, or Custom — Unlimited when the request names nothing, and the
// answer says so), which plan the creator may give, the reseller's customer
// count, and a new reseller's pool. Every refusal happens before the account
// exists, so a refused request leaves nothing half-made.
//
// Hesap oluşturma çevresindeki kiracılık kuralları. Yeni hesabın hakları:
// Sınırsız, plan ya da Özel (istek bir şey demezse Sınırsız). Her ret hesap
// var olmadan önce verilir.

// accountRightsChoice is the "Rights" part of the account form. PlanID is a
// customer plan for a customer and a reseller plan for a reseller.
type accountRightsChoice struct {
	State      string            `json:"state"`
	PlanID     int               `json:"plan_id,omitempty"`
	Rights     *rightsPayload    `json:"rights,omitempty"`
	Quantities *rightsQuantities `json:"quantities,omitempty"`
}

// plannedAccount is what finishNewAccount writes.
type plannedAccount struct {
	State string
	// Customer: the first subscription. A reseller may also get its own
	// subscription from a customer plan (legacy plan_id).
	Plan       *loadedPlan
	CustomSet  rightsSet
	CustomNums planLimits
	// Reseller.
	ResellerPlan *resellerPlan
	Pool         *resellerPool
}

// precheckNewAccount validates the rights, plan and pool of an account about
// to be created by c, writing the refusal and returning false when it may not
// be.
// precheckNewAccount, c'nin oluşturacağı hesabın haklarını, planını ve
// havuzunu denetler.
func (p *Panel) precheckNewAccount(w http.ResponseWriter, r *http.Request, c *Caller, role string, planID int, pool *resellerPool, choice *accountRightsChoice) (*plannedAccount, bool) {
	ctx := r.Context()
	db := p.db.GetDB()
	planned := &plannedAccount{State: rightsStateUnlimited}
	if pool != nil {
		if !c.hasAccountRole(roleAdmin) || role != roleReseller {
			writeClientError(w, http.StatusBadRequest, "only the administrator gives a reseller a pool")
			return nil, false
		}
		if !pool.valid() {
			writeClientError(w, http.StatusBadRequest, "pool limits must be 0 (unlimited) or more")
			return nil, false
		}
	}
	if choice != nil {
		switch choice.State {
		case rightsStateUnlimited, rightsStatePlan, rightsStateCustom:
		default:
			writeClientError(w, http.StatusBadRequest, "rights.state must be unlimited, plan or custom")
			return nil, false
		}
		if !choice.Quantities.valid() {
			writeClientError(w, http.StatusBadRequest, "quantities must be 0 (unlimited) or more")
			return nil, false
		}
		if choice.State == rightsStateCustom && choice.Rights == nil {
			writeClientError(w, http.StatusBadRequest, "a custom rights choice lists its permissions and software (rights.rights)")
			return nil, false
		}
		planned.State = choice.State
	}

	// A customer plan: the legacy plan_id (customer or a reseller's own
	// subscription), or the customer's rights choice.
	customerPlanID := planID
	if role == roleCustomer && choice != nil {
		if choice.State == rightsStatePlan {
			if planID > 0 && choice.PlanID > 0 && planID != choice.PlanID {
				writeClientError(w, http.StatusBadRequest, "plan_id and rights.plan_id name different plans")
				return nil, false
			}
			if choice.PlanID > 0 {
				customerPlanID = choice.PlanID
			}
			if customerPlanID <= 0 {
				writeClientError(w, http.StatusBadRequest, "rights.plan_id is required for the plan state")
				return nil, false
			}
		} else if planID > 0 {
			writeClientError(w, http.StatusBadRequest, "plan_id is given but the rights choice is not a plan")
			return nil, false
		}
	}
	if role == roleCustomer && choice == nil && planID > 0 {
		planned.State = rightsStatePlan
	}
	if customerPlanID > 0 {
		loaded, err := loadPlan(ctx, db, customerPlanID)
		if errors.Is(err, errNotFound) {
			writePlanNotAvailable(w)
			return nil, false
		}
		if err != nil {
			writeServerError(w, err)
			return nil, false
		}
		parent := c.ID
		if !planAssignableTo(c, loaded, &core.User{Role: role, ParentID: &parent}) {
			writePlanNotAvailable(w)
			return nil, false
		}
		planned.Plan = &loaded
	}

	switch role {
	case roleCustomer:
		if planned.State == rightsStateCustom {
			set, err := choice.Rights.set()
			if err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return nil, false
			}
			if !p.planRightsWithinGrantor(w, r, c, rightsSet{}, set) {
				return nil, false
			}
			planned.CustomSet = set
			planned.CustomNums = choice.Quantities.applyLimits(planLimits{})
		}
	case roleReseller:
		switch {
		case choice == nil && pool != nil:
			planned.State = rightsStateCustom
			planned.CustomSet = allOfferedRightsSet()
			planned.Pool = pool
		case choice != nil && choice.State == rightsStatePlan:
			if pool != nil {
				writeClientError(w, http.StatusBadRequest, "a reseller plan carries its own pool; do not send pool with it")
				return nil, false
			}
			plan, err := loadResellerPlan(ctx, db, choice.PlanID)
			if errors.Is(err, errNotFound) {
				writePlanNotAvailable(w)
				return nil, false
			}
			if err != nil {
				writeServerError(w, err)
				return nil, false
			}
			planned.ResellerPlan = &plan
		case choice != nil && choice.State == rightsStateCustom:
			set, err := choice.Rights.set()
			if err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return nil, false
			}
			if !p.planRightsWithinGrantor(w, r, c, rightsSet{}, set) {
				return nil, false
			}
			base := resellerPool{}
			if pool != nil {
				base = *pool
			}
			nums := choice.Quantities.applyPool(base)
			planned.CustomSet, planned.Pool = set, &nums
		case choice != nil && choice.State == rightsStateUnlimited && pool != nil:
			writeClientError(w, http.StatusBadRequest, "the Unlimited reseller has no pool; choose custom to give one")
			return nil, false
		}
	}

	if c.hasAccountRole(roleReseller) {
		if err := p.checkResellerCustomerCount(ctx, c.ID); err != nil {
			var exceeded *resellerPoolExceededError
			if errors.As(err, &exceeded) {
				writeCodedErrorDetails(w, http.StatusConflict, errCodeResellerCustomersLimit,
					"your pool allows no more customers; remove a customer you no longer serve, or ask the administrator for a larger pool. Nothing was created.",
					"", []string{exceeded.Error()})
				return nil, false
			}
			writeServerError(w, err)
			return nil, false
		}
	}
	return planned, true
}

// finishNewAccount stores a new account's rights: a customer's first
// subscription (from the plan, Unlimited, or Custom), a reseller's state and
// pool, and a reseller's own subscription from a customer plan (legacy).
// finishNewAccount, yeni hesabın haklarını kaydeder.
func (p *Panel) finishNewAccount(w http.ResponseWriter, r *http.Request, user *core.User, planned *plannedAccount) bool {
	ctx := r.Context()
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return false
	}
	defer tx.Rollback()
	switch user.Role {
	case roleCustomer:
		switch planned.State {
		case rightsStatePlan:
			_, err = p.insertSubscriptionFromPlan(ctx, tx, user.ID, *planned.Plan)
		case rightsStateCustom:
			var subID int
			subID, err = insertBareSubscription(ctx, tx, user.ID, "Custom", planned.CustomNums)
			if err == nil {
				err = saveRightsSet(ctx, tx, rightsOwnerSubscription, subID, planned.CustomSet)
			}
		default:
			_, err = insertBareSubscription(ctx, tx, user.ID, builtinUnlimitedName, planLimits{})
		}
	case roleReseller:
		switch {
		case planned.ResellerPlan != nil:
			err = bindResellerPlan(ctx, tx, user.ID, *planned.ResellerPlan)
		case planned.State == rightsStateCustom:
			pool := resellerPool{}
			if planned.Pool != nil {
				pool = *planned.Pool
			}
			err = setResellerCustom(ctx, tx, user.ID, planned.CustomSet, pool)
		}
		if err == nil && planned.Plan != nil {
			_, err = p.insertSubscriptionFromPlan(ctx, tx, user.ID, *planned.Plan)
		}
	}
	// An account created under a suspended reseller starts suspended, with
	// the reseller as its cause, in this same transaction.
	suspended := false
	if err == nil {
		actorID := 0
		if c := currentCaller(r); c != nil {
			actorID = c.ID
		}
		suspended, err = p.syncNewAccountResellerSuspension(ctx, tx, user.ID, actorID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeServerError(w, err)
		return false
	}
	if suspended {
		p.kickSuspensionWorker()
	}
	return true
}

// syncNewAccountResellerSuspension applies the reseller cause to every
// subscription the new account owns.
func (p *Panel) syncNewAccountResellerSuspension(ctx context.Context, tx sqlQueryer, ownerID, actorID int) (bool, error) {
	ids, err := accountSubscriptionIDs(ctx, tx, ownerID, false)
	if err != nil {
		return false, err
	}
	changedAny := false
	for _, id := range ids {
		changed, err := p.syncResellerSuspensionCauseTx(ctx, tx, id, actorID)
		if err != nil {
			return false, err
		}
		changedAny = changedAny || changed
	}
	return changedAny, nil
}

// insertBareSubscription makes a subscription without a plan with the given
// numbers (all 0 = the Unlimited plan).
func insertBareSubscription(ctx context.Context, db sqlExecer, ownerID int, name string, l planLimits) (int, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (owner_id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb, status, starts_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'active', datetime('now'))`,
		ownerID, name, l.Domains, l.Databases, l.Mailboxes, l.DiskMB)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

// impersonationStillAllowed: a "Log in as" session is honoured only while
// its operator is an active administrator or reseller who may still manage
// the account (a reseller only its own customers).
// impersonationStillAllowed: "Farklı giriş yap" oturumu, operatörü hesabı hâlâ
// yönetebilen etkin bir yönetici ya da bayi olduğu sürece geçerlidir.
func (p *Panel) impersonationStillAllowed(ctx context.Context, operatorID int, target *core.User) (bool, error) {
	operator, err := p.users.GetByID(ctx, operatorID)
	if errors.Is(err, repositories.ErrUserNotFound) {
		return false, nil
	}
	if err != nil || operator == nil {
		return false, err
	}
	if operator.Status != "active" || target == nil {
		return false, nil
	}
	identity, ok := operator.EffectiveIdentity()
	if !ok || identity.UserID != operator.ID || normalizedStoredAccountType(operator) != core.AccountTypeAccount {
		return false, nil
	}
	if identity.Role != roleAdmin && identity.Role != roleReseller {
		return false, nil
	}
	return canManageUser(&Caller{ID: operator.ID, Role: identity.Role, AccountType: core.AccountTypeAccount}, target), nil
}
