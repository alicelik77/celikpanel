package main

import (
	"net/http"
)

// Additional users (team members) are a right of the subscription (D-042
// 4.2 "Additional users"): adding a member or giving a member access is new
// use. A member's own access is always a subset of the customer's rights,
// because every domain request a member makes passes the same gates.
// Ek kullanıcılar aboneliğin hakkıdır; üyenin erişimi müşterinin haklarının
// alt kümesidir.

// requireTeamMembersRight checks the subscriptions an access list names
// (directly or through a domain); with none named, at least one of the
// owner's subscriptions must hold the right.
func (p *Panel) requireTeamMembersRight(w http.ResponseWriter, r *http.Request, ownerID int, access *teamMemberAccessRequest) bool {
	if callerIsAdministrator(r) {
		return true
	}
	subs := map[int]bool{}
	if access != nil {
		for _, permission := range access.SubscriptionPermissions {
			subs[permission.SubscriptionID] = true
		}
		for _, permission := range access.DomainPermissions {
			subID, err := p.domainSubscriptionID(r.Context(), permission.DomainID)
			if err == nil {
				subs[subID] = true
			}
		}
	}
	db := p.db.GetDB()
	if len(subs) == 0 {
		rows, err := db.QueryContext(r.Context(), `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id`, ownerID)
		if err != nil {
			writeServerError(w, err)
			return false
		}
		ids := []int{}
		for rows.Next() {
			var id int
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		var first *effectiveRights
		firstLayer := ""
		for _, id := range ids {
			e, err := p.effectiveSubscriptionRights(r.Context(), id)
			if err != nil {
				writeServerError(w, err)
				return false
			}
			layer := e.permissionRefusal(rightTeamMembers)
			if layer == "" {
				return true
			}
			if first == nil {
				first, firstLayer = e, layer
			}
		}
		if first == nil {
			writeCodedError(w, http.StatusConflict, errCodeNoSubscription,
				"this account has no subscription yet; ask your provider to assign a plan", myPlanPagePath)
			return false
		}
		writeRightNotGranted(w, first, rightTeamMembers, firstLayer)
		return false
	}
	for subID := range subs {
		var owner int
		if err := db.QueryRowContext(r.Context(), `SELECT owner_id FROM subscriptions WHERE id = ?`, subID).Scan(&owner); err != nil || owner != ownerID {
			// Not the owner's: the repository refuses it with its own answer.
			continue
		}
		if !p.requireSubscriptionRight(w, r, subID, rightTeamMembers) {
			return false
		}
	}
	return true
}
