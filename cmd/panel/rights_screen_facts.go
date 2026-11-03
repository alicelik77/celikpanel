package main

import (
	"context"
	"log"
)

// Read-only facts the screens draw menus and tabs from (D-042 P6: the server
// enforces, the screen only shows). Nothing here changes a right; every gate
// stays in rights_gate.go. A fact that cannot be read leaves the screen as it
// was before this release (the entry is drawn and the server refuses with its
// reason), so a failed read never hides something the account may use.
//
// Ekranların menü ve sekmeleri çizdiği salt-okur olgular. Burada hiçbir hak
// değişmez; okunamayan olgu ekranı bu sürümden önceki hâlinde bırakır.

// closedPermissions lists, in the fixed order, the permissions a
// subscription may not use now. The administrator's own workspace has none.
func (p *Panel) closedPermissions(ctx context.Context, subID int) ([]string, error) {
	e, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		return nil, err
	}
	closed := []string{}
	for _, id := range rightPermissionIDs {
		if !e.hasPermission(id) {
			closed = append(closed, id)
		}
	}
	return closed, nil
}

// closedPermissionsCache answers closedPermissions once per subscription for
// one listing. A subscription whose layers cannot be read answers nil.
type closedPermissionsCache struct {
	p    *Panel
	seen map[int][]string
}

func (p *Panel) newClosedPermissionsCache() *closedPermissionsCache {
	return &closedPermissionsCache{p: p, seen: map[int][]string{}}
}

func (c *closedPermissionsCache) get(ctx context.Context, subID int) []string {
	if closed, ok := c.seen[subID]; ok {
		return closed
	}
	closed, err := c.p.closedPermissions(ctx, subID)
	if err != nil {
		log.Printf("rights facts: subscription %d: %v", subID, err)
		closed = nil
	}
	c.seen[subID] = closed
	return closed
}

// accountPermits reports whether any subscription of the account (the
// customer itself, or the customer an additional user belongs to) holds the
// permission. ok is false when it could not be read, or when the account has
// no subscription; the caller then keeps its earlier answer.
func (p *Panel) accountPermits(ctx context.Context, caller *Caller, permission string) (permits bool, ok bool) {
	if caller == nil || p.db == nil {
		return false, false
	}
	owner := caller.ID
	if caller.isAdditionalUser() {
		owner = caller.CustomerID
	}
	rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id`, owner)
	if err != nil {
		return false, false
	}
	var subscriptions []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, false
		}
		subscriptions = append(subscriptions, id)
	}
	failed := rows.Err() != nil
	rows.Close()
	if failed || len(subscriptions) == 0 {
		// No subscription: nothing to narrow by; the page says what is missing.
		return false, false
	}
	for _, id := range subscriptions {
		has, err := p.subscriptionHasRight(ctx, int64(id), permission)
		if err != nil {
			return false, false
		}
		if has {
			return true, true
		}
	}
	return false, true
}

// narrowFeaturesByRights adds the rights a customer's navigation follows:
// "databases" (the Databases page), and narrows "team_members" and "vpn" to
// the permission of the same name. The administrator and a reseller keep
// their pages; a fact that cannot be read leaves the feature as it was.
func (p *Panel) narrowFeaturesByRights(ctx context.Context, caller *Caller, features map[string]bool) {
	features["databases"] = true
	if caller == nil || caller.hasAccountRole(roleAdmin) || caller.hasAccountRole(roleReseller) {
		return
	}
	if permits, ok := p.accountPermits(ctx, caller, rightDatabases); ok {
		features["databases"] = permits
	}
	for feature, permission := range map[string]string{"team_members": rightTeamMembers, "vpn": rightVPN} {
		if !features[feature] {
			continue
		}
		if permits, ok := p.accountPermits(ctx, caller, permission); ok && !permits {
			features[feature] = false
		}
	}
}

// domainRightsClosed is the "rights_closed" of one listed domain: the closed
// permissions of its subscription, for a caller the gates apply to. Empty
// (omitted) for the administrator and when nothing is closed.
func domainRightsClosed(ctx context.Context, cache *closedPermissionsCache, subID int) []string {
	if cache == nil || subID <= 0 {
		return nil
	}
	closed := cache.get(ctx, subID)
	if len(closed) == 0 {
		return nil
	}
	return append([]string{}, closed...)
}
