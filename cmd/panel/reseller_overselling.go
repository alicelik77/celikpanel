package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// Overselling choice of a reseller (alpha.128, gap G17). The owner's K2 rule
// stays the default: a reseller may hand its customers more than its pool,
// and the pool limits their real use. A reseller plan (or a Custom reseller)
// may instead say "not allowed": then the limits handed out to the
// reseller's subscriptions may not add up to more than the pool, and an
// unlimited allocation is refused where the pool has a limit. It is checked
// where limits are handed out (a plan assigned, a Custom subscription's
// numbers); what was handed out before is kept (P5), only new allocations
// are refused.
//
// Bayinin fazla dağıtım seçimi: varsayılan K2 (dağıtım havuzu aşabilir,
// gerçek kullanım aşamaz). "İzin verilmez" seçilirse dağıtılan toplam havuzu
// aşamaz; önceki dağıtımlar korunur, yalnız yeni dağıtım reddedilir.

const (
	oversellingAllowed    = "allowed"
	oversellingNotAllowed = "not_allowed"

	errCodePoolOversold = "POOL_OVERSELLING_NOT_ALLOWED"
)

func validOverselling(value string) bool {
	return value == "" || value == oversellingAllowed || value == oversellingNotAllowed
}

// resellerOverselling is the choice a reseller follows: its reseller plan's
// when bound, else its pool's; "allowed" when it has neither.
func resellerOverselling(ctx context.Context, q rowQueryer, resellerID int) (string, error) {
	var value string
	err := q.QueryRowContext(ctx, `
		SELECT rp.overselling FROM reseller_plan_bindings b JOIN reseller_plans rp ON rp.id = b.reseller_plan_id
		WHERE b.reseller_id = ?`, resellerID).Scan(&value)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	err = q.QueryRowContext(ctx, `SELECT overselling FROM reseller_pools WHERE reseller_id = ?`, resellerID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return oversellingAllowed, nil
	}
	return value, err
}

// oversellingRefusal lists, for a reseller that does not allow
// overselling, the kinds where giving subscription subID the limits next
// would hand out more than the pool. Empty when allowed or within the pool.
func oversellingRefusal(ctx context.Context, q rowQueryer, resellerID, subID int, next planLimits) ([]string, error) {
	if resellerID == 0 {
		return nil, nil
	}
	choice, err := resellerOverselling(ctx, q, resellerID)
	if err != nil || choice != oversellingNotAllowed {
		return nil, err
	}
	pool, hasPool, err := loadResellerPool(ctx, q, resellerID)
	if err != nil || !hasPool {
		return nil, err
	}
	ids, err := treeSubscriptionIDs(ctx, q, resellerID)
	if err != nil {
		return nil, err
	}
	type kind struct {
		name       string
		pool       int
		of         func(planLimits) int
		handedOut  int
		unlimitedN int
	}
	kinds := []*kind{
		{name: "domains", pool: pool.Domains, of: func(l planLimits) int { return l.Domains }},
		{name: "databases", pool: pool.Databases, of: func(l planLimits) int { return l.Databases }},
		{name: "mailboxes", pool: pool.Mailboxes, of: func(l planLimits) int { return l.Mailboxes }},
		{name: "disk (MB)", pool: pool.DiskMB, of: func(l planLimits) int { return l.DiskMB }},
		{name: "traffic (MB)", pool: pool.TrafficMB, of: func(l planLimits) int { return l.TrafficMB }},
	}
	for _, id := range ids {
		if id == subID {
			continue
		}
		limits, err := loadSubscriptionLimits(ctx, q, id)
		if err != nil {
			return nil, err
		}
		for _, k := range kinds {
			if v := k.of(limits); v <= 0 {
				k.unlimitedN++
			} else {
				k.handedOut += v
			}
		}
	}
	out := []string{}
	for _, k := range kinds {
		if k.pool <= 0 {
			continue
		}
		asked := k.of(next)
		switch {
		case asked <= 0:
			out = append(out, fmt.Sprintf("%s: unlimited asked, the pool has %d", k.name, k.pool))
		case k.handedOut+asked > k.pool:
			out = append(out, fmt.Sprintf("%s: %d handed out + %d asked > %d in the pool", k.name, k.handedOut, asked, k.pool))
		}
	}
	return out, nil
}

// refuseOverselling writes the refusal and reports whether it did.
// ownerID is the subscription's owner (subID 0: a subscription about to be
// made for that owner).
func (p *Panel) refuseOverselling(w http.ResponseWriter, ctx context.Context, ownerID, subID int, next planLimits) bool {
	resellerID, err := treeResellerID(ctx, p.db.GetDB(), ownerID)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	over, err := oversellingRefusal(ctx, p.db.GetDB(), resellerID, subID, next)
	if err != nil {
		writeServerError(w, err)
		return true
	}
	if len(over) == 0 {
		return false
	}
	writeCodedErrorDetails(w, http.StatusConflict, errCodePoolOversold,
		"Nothing was changed: this reseller hands out only what its pool holds, and these limits would go beyond it. "+
			"Choose smaller limits, or ask the administrator to raise the pool or allow overselling.", "", over)
	return true
}
