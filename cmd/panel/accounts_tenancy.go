package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/alicelik/celikpanel/internal/core"
	"github.com/alicelik/celikpanel/internal/repositories"
)

// The tenancy rules around creating an account (alpha.114): which plan the
// creator may give, the reseller's pool and customer count, and a new
// reseller's pool. Every refusal happens before the account exists, so a
// refused request leaves nothing half-made.
//
// Hesap oluşturma çevresindeki kiracılık kuralları (alpha.114). Her ret hesap
// var olmadan önce verilir; reddedilen istek yarım bir şey bırakmaz.

// precheckNewAccount validates the plan and pool of an account about to be
// created by c, writing the refusal and returning false when it may not be.
// precheckNewAccount, c'nin oluşturacağı hesabın planını ve havuzunu denetler.
func (p *Panel) precheckNewAccount(w http.ResponseWriter, r *http.Request, c *Caller, role string, planID int, pool *resellerPool) (*loadedPlan, bool) {
	ctx := r.Context()
	db := p.db.GetDB()
	if pool != nil {
		if !c.hasAccountRole(roleAdmin) || role != roleReseller {
			writeClientError(w, http.StatusBadRequest, "only the administrator gives a reseller a pool")
			return nil, false
		}
		if pool.MaxCustomers < 0 || pool.Domains < 0 || pool.Databases < 0 || pool.Mailboxes < 0 || pool.DiskMB < 0 {
			writeClientError(w, http.StatusBadRequest, "pool limits must be 0 (unlimited) or more")
			return nil, false
		}
	}
	var plan *loadedPlan
	if planID > 0 {
		loaded, err := loadPlan(ctx, db, planID)
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
		if pool != nil && !limitsWithinPool(*pool, loaded.Limits) {
			writePlanExceedsPool(w, *pool)
			return nil, false
		}
		plan = &loaded
	}
	if c.hasAccountRole(roleReseller) {
		violations, err := checkResellerPool(ctx, db, c.ID, 0, nil, true)
		if err != nil {
			writeServerError(w, err)
			return nil, false
		}
		if len(violations) > 0 {
			writeCodedErrorDetails(w, http.StatusConflict, errCodeResellerCustomersLimit,
				"your pool allows no more customers; remove a customer you no longer serve, or ask the administrator for a larger pool. Nothing was created.",
				"", violations)
			return nil, false
		}
		if plan != nil {
			violations, err := checkResellerPool(ctx, db, c.ID, 0, &plan.Limits, false)
			if err != nil {
				writeServerError(w, err)
				return nil, false
			}
			if len(violations) > 0 {
				writePoolRefusal(w, violations)
				return nil, false
			}
		}
	}
	return plan, true
}

// finishNewAccount stores a new reseller's pool and the first subscription.
// finishNewAccount, yeni bayinin havuzunu ve ilk aboneliği kaydeder.
func (p *Panel) finishNewAccount(w http.ResponseWriter, r *http.Request, user *core.User, plan *loadedPlan, pool *resellerPool) bool {
	ctx := r.Context()
	db := p.db.GetDB()
	if pool != nil {
		if err := upsertResellerPool(ctx, db, user.ID, *pool); err != nil {
			writeServerError(w, err)
			return false
		}
	}
	if plan != nil {
		if _, err := p.insertSubscriptionFromPlan(ctx, db, user.ID, *plan); err != nil {
			writeServerError(w, err)
			return false
		}
	}
	return true
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
