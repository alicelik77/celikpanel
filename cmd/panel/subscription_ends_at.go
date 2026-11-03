package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A subscription's end date (K7, migration 052's ends_at). The administrator
// or the account's reseller sets or clears it; NULL means it never ends.
// When the date passes, the periodic expiry check suspends the subscription
// (suspension_state.go); setting a date that has already passed suspends it
// at once, and moving it into the future or clearing it lifts that cause.
//
// Aboneliğin bitiş tarihi: yönetici ya da bayi koyar veya kaldırır; boş
// hiç bitmez demektir. Tarih geçince abonelik askıya alınır.

const errCodeEndsAtInvalid = "SUBSCRIPTION_ENDS_AT_INVALID"

// parseSubscriptionEndsAtPath matches /api/v1/subscriptions/{id}/ends-at.
func parseSubscriptionEndsAtPath(path string) (int, bool) {
	rest, ok := strings.CutPrefix(path, "/api/v1/subscriptions/")
	if !ok {
		return 0, false
	}
	idText, tail, ok := strings.Cut(rest, "/")
	if !ok || tail != "ends-at" {
		return 0, false
	}
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 || strconv.Itoa(id) != idText {
		return 0, false
	}
	return id, true
}

// parseEndsAt reads a date (YYYY-MM-DD, the start of that day in UTC) or an
// RFC 3339 time, and stores it in SQLite's own layout.
func parseEndsAt(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, raw); err == nil {
			t = t.UTC()
			if t.Year() < 2000 || t.Year() > 9999 {
				return "", errors.New("the end date must be between the years 2000 and 9999")
			}
			return t.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", errors.New("the end date must be a date such as 2027-01-31")
}

// handleSubscriptionEndsAt: PUT {"ends_at": "2027-01-31" | null}.
func (p *Panel) handleSubscriptionEndsAt(w http.ResponseWriter, r *http.Request, subscriptionID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPut {
		rejectRouteMethod(w, []string{http.MethodPut})
		return
	}
	ctx := r.Context()
	c := currentCaller(r)
	ok, err := p.subscriptionOwnerManageable(ctx, c, subscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	var body struct {
		EndsAt *string `json:"ends_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var next any
	if body.EndsAt != nil && strings.TrimSpace(*body.EndsAt) != "" {
		value, err := parseEndsAt(*body.EndsAt)
		if err != nil {
			writeCodedError(w, http.StatusBadRequest, errCodeEndsAtInvalid, err.Error()+"; nothing was changed", "")
			return
		}
		next = value
	}

	var name string
	var previous sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT name, ends_at FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&name, &previous); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		writeServerError(w, err)
		return
	}
	if _, err := p.db.GetDB().ExecContext(ctx, `UPDATE subscriptions SET ends_at = ?, updated_at = datetime('now') WHERE id = ?`, next, subscriptionID); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "subscription.ends_at:"+name, "subscription", subscriptionID, map[string]any{
		"ends_at": next, "previous": nullStringValue(previous),
	})
	// A date that has passed suspends now; a later or no date lifts the cause.
	if err := p.checkSubscriptionExpiry(ctx); err != nil {
		writeServerError(w, err)
		return
	}
	lifecycle, err := readSubscriptionLifecycle(ctx, p.db.GetDB(), subscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "lifecycle": lifecycle})
}
