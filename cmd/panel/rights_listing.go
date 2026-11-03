package main

import (
	"net/http"
	"strings"
)

// Choosers list only what the subscription may choose (D-017 #4, D-042 P6).
// Seçiciler yalnız aboneliğin seçebileceğini listeler.

// listingSubscription is the subscription a chooser lists for: the one named
// by ?subscription_id= (which the caller must see), else the caller's only
// subscription. ok is false when there is none to narrow by (the list then
// follows the administrator's offer alone; acting endpoints still check).
// handled is true when a refusal was written.
func (p *Panel) listingSubscription(w http.ResponseWriter, r *http.Request) (subID int, ok bool, handled bool) {
	caller := currentCaller(r)
	if raw := strings.TrimSpace(r.URL.Query().Get("subscription_id")); raw != "" {
		id, valid := strictPositiveID(raw)
		if !valid {
			writeClientError(w, http.StatusBadRequest, "invalid subscription_id")
			return 0, false, true
		}
		if err := p.canAccessSubscription(r.Context(), caller, id); err != nil {
			writeClientError(w, http.StatusNotFound, "subscription not found")
			return 0, false, true
		}
		return id, true, false
	}
	if caller == nil || callerIsAdministrator(r) {
		return 0, false, false
	}
	owner := caller.ID
	if caller.isAdditionalUser() {
		owner = caller.CustomerID
	}
	rows, err := p.db.GetDB().QueryContext(r.Context(), `SELECT id FROM subscriptions WHERE owner_id = ? ORDER BY id LIMIT 2`, owner)
	if err != nil {
		writeServerError(w, err)
		return 0, false, true
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 1 {
		return ids[0], true, false
	}
	return 0, false, false
}

// listingVersionPolicy is the version policy a chooser lists by: the
// subscription's when there is one, else the administrator's.
func (p *Panel) listingVersionPolicy(w http.ResponseWriter, r *http.Request) (offeredVersions, *effectiveRights, bool) {
	subID, ok, handled := p.listingSubscription(w, r)
	if handled {
		return offeredVersions{}, nil, false
	}
	if !ok {
		policy, err := p.loadOfferedVersions(r.Context())
		if err != nil {
			writeServerError(w, err)
			return offeredVersions{}, nil, false
		}
		return policy, nil, true
	}
	policy, rights, err := p.subscriptionVersionPolicy(r.Context(), subID)
	if err != nil {
		writeServerError(w, err)
		return offeredVersions{}, nil, false
	}
	return policy, rights, true
}

// projectTypesFor: the site types a subscription may choose now (php only
// with an open PHP version, node with an open Node.js version, proxy with
// the proxy right). nil rights = the administrator's offer alone.
func projectTypesFor(rights *effectiveRights, policy offeredVersions, phpVersions []string) []string {
	types := []string{"static", "dnsonly", "forwarding"}
	if len(policy.filter("php", phpVersions)) > 0 {
		types = append([]string{"php"}, types...)
	}
	if rights == nil || rights.Unrestricted {
		return append(types, "node", "proxy")
	}
	if rights.anyVersionRefusal("component:node") == "" {
		types = append(types, "node")
	}
	if rights.hasPermission(rightSiteProxy) {
		types = append(types, "proxy")
	}
	return types
}
