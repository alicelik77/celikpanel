package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// GET /api/v1/my/subscription (alpha.114, ROADMAP v0.3 "the customer sees
// their own subscription"): the signed-in account's plan, its limits and the
// live usage the quota checks use, so the numbers on "My plan" are exactly
// the numbers a 409 refusal is based on; whom to contact for a larger plan;
// and, for a reseller, its pool and what it has handed out. Traffic is not
// measured yet and says so instead of showing a limit. Team members (additional
// users) do not see their customer's plan; the middleware refuses them.
//
// Oturumdaki hesabın planı, sınırları ve kota denetimlerinin kullandığı canlı
// kullanım; daha büyük plan için kime başvurulacağı; bayi için havuzu.

const mySubscriptionPath = "/api/v1/my/subscription"

type myPlanRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type mySubscriptionEntry struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Plan      *myPlanRef         `json:"plan"`
	Usage     *subscriptionUsage `json:"usage"`
	OverLimit []string           `json:"over_limit"`
}

type myContact struct {
	// Kind: "reseller", "administrator" or "none" (the administrator itself).
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type myPoolAnswer struct {
	Limits    *resellerPool  `json:"limits"`
	Allocated poolAllocation `json:"allocated"`
	OverLimit []string       `json:"over_limit"`
}

func (p *Panel) handleMySubscription(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	c := currentCaller(r)
	if c == nil || (!c.hasAccountRole(roleAdmin) && !c.hasAccountRole(roleReseller) && !c.hasAccountRole(roleCustomer)) {
		writeClientError(w, http.StatusForbidden, "account access required")
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()

	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.name, COALESCE(s.status, 'active'), sp.id, sp.name
		FROM subscriptions s LEFT JOIN service_plans sp ON sp.id = s.plan_id
		WHERE s.owner_id = ? ORDER BY s.id`, c.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	entries := []mySubscriptionEntry{}
	for rows.Next() {
		var e mySubscriptionEntry
		var planID sql.NullInt64
		var planName sql.NullString
		if err := rows.Scan(&e.ID, &e.Name, &e.Status, &planID, &planName); err != nil {
			rows.Close()
			writeServerError(w, err)
			return
		}
		if planID.Valid {
			e.Plan = &myPlanRef{ID: int(planID.Int64), Name: planName.String}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeServerError(w, err)
		return
	}
	rows.Close()
	for i := range entries {
		usage, err := p.subscriptionUsageFor(ctx, entries[i].ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		entries[i].Usage = usage
		entries[i].OverLimit = usage.overLimits()
	}

	contact, err := p.upgradeContact(r, c)
	if err != nil {
		writeServerError(w, err)
		return
	}
	answer := map[string]any{
		"subscriptions": entries,
		"contact":       contact,
		// Traffic is not measured yet: no traffic limit is applied anywhere.
		// Trafik henüz ölçülmüyor: hiçbir yerde trafik sınırı uygulanmaz.
		"bandwidth": map[string]bool{"measured": false},
	}
	if c.hasAccountRole(roleReseller) {
		pool, ok, err := loadResellerPool(ctx, db, c.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		allocation, err := resellerAllocation(ctx, db, c.ID, 0)
		if err != nil {
			writeServerError(w, err)
			return
		}
		shown := myPoolAnswer{Allocated: allocation, OverLimit: []string{}}
		if ok {
			shown.Limits = &pool
			shown.OverLimit = poolViolations(pool, allocation)
		}
		answer["pool"] = shown
	}
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		log.Printf("encode my subscription: %v", err)
	}
}

// upgradeContact names who changes this account's plan: the reseller that
// created a customer, otherwise the administrator.
// upgradeContact, bu hesabın planını kimin değiştirdiğini adlandırır.
func (p *Panel) upgradeContact(r *http.Request, c *Caller) (myContact, error) {
	if c.hasAccountRole(roleAdmin) {
		return myContact{Kind: "none"}, nil
	}
	var contact myContact
	var role string
	err := p.db.GetDB().QueryRowContext(r.Context(), `
		SELECT pu.role, pu.username, pu.email
		FROM users u JOIN users pu ON pu.id = u.parent_id
		WHERE u.id = ? AND pu.role IN ('admin', 'reseller')`, c.ID).Scan(&role, &contact.Name, &contact.Email)
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
	err = p.db.GetDB().QueryRowContext(r.Context(), `
		SELECT username, email FROM users
		WHERE role = 'admin' AND COALESCE(NULLIF(account_type, ''), 'account') = 'account'
		ORDER BY id LIMIT 1`).Scan(&contact.Name, &contact.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return myContact{Kind: "administrator"}, nil
	}
	contact.Kind = "administrator"
	return contact, err
}
