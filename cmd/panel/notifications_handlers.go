package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/mutationpayload"
)

// Notifications by recipient (alpha.128, G18).
//
//	GET  /api/v1/notifications                 the caller's own unseen notices, one list
//	POST /api/v1/notifications/{id}/seen       marks one of the caller's account notices seen
//	GET  /api/v1/notification-preferences      the caller's email choice
//	PUT  /api/v1/notification-preferences      {"mode": "all"|"action"|"none", "extra_email": "" | address}
//	GET  /api/v1/panel/notification-settings   administrator: expiry days and extra addresses
//	PUT  /api/v1/panel/notification-settings   administrator: the same, all fields
//
// The list merges the caller's account notices (069) and usage notices
// (057), each with whether this person acts and the page where the fix is;
// an administrator also gets the open monitoring alerts as one line. Rows
// are filtered by the caller's own user id on the server; a team member
// sees only notices addressed to them (none today). Usage notices keep their
// own seen route (/api/v1/usage-notices/{id}/seen). Reading never raises or
// mails anything.
//
// Bildirimler: kişinin kendi görülmemiş bildirimleri tek listede; her biri
// kimin yapacağını ve düzeltme sayfasını söyler. Yalnız kişinin kendi
// satırları; okumak hiçbir şey başlatmaz.

const (
	notificationsPath           = "/api/v1/notifications"
	notificationPreferencesPath = "/api/v1/notification-preferences"
	notificationSettingsPath    = "/api/v1/panel/notification-settings"

	errCodeNotificationPrefsInvalid    = "NOTIFICATION_PREFERENCES_INVALID"
	errCodeNotificationPrefsImpersonal = "NOTIFICATION_PREFERENCES_OWN_ONLY"
	errCodeNotificationSettingsInvalid = "NOTIFICATION_SETTINGS_INVALID"

	notificationsListLimit = 50
)

type notificationItem struct {
	Source      string `json:"source"` // "account" or "usage"
	ID          int    `json:"id"`
	Kind        string `json:"kind"`
	Audience    string `json:"audience"`
	NeedsAction bool   `json:"needs_action"`
	FixPath     string `json:"fix_path,omitempty"`
	CreatedAt   string `json:"created_at"`
	// Words: an account notice in both languages (the text registry).
	Words *accountNoticeWords `json:"words,omitempty"`
	// Usage: a usage notice's facts; the screen words them as before.
	Usage *usageNoticeBody `json:"usage,omitempty"`
	// SubscriptionID: a usage notice's subscription.
	SubscriptionID int `json:"subscription_id,omitempty"`
}

type notificationsMonitoring struct {
	Open   int `json:"open"`
	Unseen int `json:"unseen"`
}

type notificationsReport struct {
	Items      []notificationItem       `json:"items"`
	Monitoring *notificationsMonitoring `json:"monitoring,omitempty"`
}

// usageNoticeFixPath: where a usage notice's fix is for this audience. The
// owner reads their plan (My account); whoever manages the account opens its
// rights; the administrator opens a reseller's rights for a pool.
func usageNoticeFixPath(kind, audience string, body usageNoticeBody) string {
	if audience == audienceCustomer {
		return "/account"
	}
	switch kind {
	case noticePoolTraffic80, noticePoolTraffic100:
		if audience == audienceAdministrator && body.Reseller != "" {
			return "/users?rights=" + url.QueryEscape(body.Reseller)
		}
		return "/account"
	}
	if body.Owner != "" {
		return "/users?rights=" + url.QueryEscape(body.Owner)
	}
	return "/users"
}

func (p *Panel) handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, notificationsPath), "/")
	if rest == "" {
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		report, err := p.readNotifications(r, caller)
		if err != nil {
			writeServerError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(report)
		return
	}
	idText, action, _ := strings.Cut(rest, "/")
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 || strconv.Itoa(id) != idText || action != "seen" {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	result, err := p.db.GetDB().ExecContext(r.Context(),
		`UPDATE account_notices SET seen_at = COALESCE(seen_at, datetime('now')) WHERE id = ? AND user_id = ?`, id, caller.ID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, _ := result.RowsAffected(); n != 1 {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"seen": true})
}

func (p *Panel) readNotifications(r *http.Request, caller *Caller) (notificationsReport, error) {
	ctx := r.Context()
	db := p.db.GetDB()
	report := notificationsReport{Items: []notificationItem{}}
	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, audience, needs_action, body, fix_path, created_at FROM account_notices
		WHERE user_id = ? AND seen_at IS NULL ORDER BY created_at DESC, id DESC LIMIT ?`, caller.ID, notificationsListLimit)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var item notificationItem
		var raw string
		if err := rows.Scan(&item.ID, &item.Kind, &item.Audience, &item.NeedsAction, &raw, &item.FixPath, &item.CreatedAt); err != nil {
			rows.Close()
			return report, err
		}
		var body map[string]any
		if json.Unmarshal([]byte(raw), &body) != nil {
			body = map[string]any{}
		}
		words := accountNoticeWordsFor(item.Kind, item.Audience, body)
		item.Source, item.Words, item.FixPath = "account", &words, validNoticeFixPath(item.FixPath)
		report.Items = append(report.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return report, err
	}
	rows, err = db.QueryContext(ctx, `
		SELECT id, kind, audience, subscription_id, body, created_at FROM usage_notices
		WHERE user_id = ? AND seen_at IS NULL ORDER BY created_at DESC, id DESC LIMIT ?`, caller.ID, notificationsListLimit)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var item notificationItem
		var raw string
		var body usageNoticeBody
		if err := rows.Scan(&item.ID, &item.Kind, &item.Audience, &item.SubscriptionID, &raw, &item.CreatedAt); err != nil {
			rows.Close()
			return report, err
		}
		if json.Unmarshal([]byte(raw), &body) != nil {
			continue
		}
		item.Source, item.Usage = "usage", &body
		item.NeedsAction = usageNoticeNeedsAction(item.Kind, item.Audience)
		item.FixPath = usageNoticeFixPath(item.Kind, item.Audience, body)
		report.Items = append(report.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return report, err
	}
	if caller.hasAccountRole(roleAdmin) {
		var m notificationsMonitoring
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*), COALESCE(SUM(CASE WHEN seen_at IS NULL THEN 1 ELSE 0 END), 0)
			FROM monitoring_alerts WHERE state = 'open'`).Scan(&m.Open, &m.Unseen); err != nil {
			return report, err
		}
		report.Monitoring = &m
	}
	return report, nil
}

// --- The person's own email choice -------------------------------------------

type notificationPreferencesView struct {
	Mode       string `json:"mode"`
	ExtraEmail string `json:"extra_email"`
	// The person's own account address, for the screen's sentence.
	OwnEmailSet bool `json:"own_email_set"`
	// Mail: whether the panel can send mail at all.
	MailAvailable bool `json:"mail_available"`
}

func (p *Panel) readNotificationPreferences(r *http.Request, userID int) notificationPreferencesView {
	db := p.db.GetDB()
	pref := notificationPreferenceFor(r.Context(), db, userID)
	view := notificationPreferencesView{Mode: pref.Mode, ExtraEmail: pref.ExtraEmail}
	var email string
	_ = db.QueryRowContext(r.Context(), `SELECT COALESCE(email, '') FROM users WHERE id = ?`, userID).Scan(&email)
	view.OwnEmailSet = strings.TrimSpace(email) != ""
	view.MailAvailable = p.accountMailSetup(r.Context()).Available
	return view
}

func (p *Panel) handleNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || caller.ID <= 0 {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(p.readNotificationPreferences(r, caller.ID))
		return
	case http.MethodPut:
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
		return
	}
	// Someone browsing as this person ("Log in as") must not change where
	// their mail goes.
	if caller.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusConflict, errCodeNotificationPrefsImpersonal,
			"Email choices belong to the person themselves, so nothing was changed. They change them in My account after signing in.", "")
		return
	}
	var req struct {
		Mode       *string `json:"mode"`
		ExtraEmail *string `json:"extra_email"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var trailing any
	if err := decoder.Decode(&req); err != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) || req.Mode == nil || req.ExtraEmail == nil {
		writeCodedError(w, http.StatusBadRequest, errCodeNotificationPrefsInvalid,
			"These email choices could not be read, so nothing was changed. Reload the page and choose again.", "")
		return
	}
	switch *req.Mode {
	case notifyModeAll, notifyModeAction, notifyModeNone:
	default:
		writeCodedError(w, http.StatusBadRequest, errCodeNotificationPrefsInvalid,
			"Choose everything, only what needs your action, or nothing; nothing was changed.", "")
		return
	}
	var extra any
	if raw := strings.TrimSpace(*req.ExtraEmail); raw != "" {
		canonical, err := mutationpayload.CanonicalContactEmail(raw)
		if err != nil || canonical == "" {
			writeCodedError(w, http.StatusBadRequest, errCodeNotificationPrefsInvalid,
				"The extra address is not a usable email address, so nothing was changed. Correct it (for example name@example.com), or leave it empty for none.", "")
			return
		}
		extra = canonical
	}
	if _, err := p.db.GetDB().ExecContext(r.Context(), `
		INSERT INTO notification_preferences (user_id, mode, extra_email, updated_at) VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(user_id) DO UPDATE SET mode = excluded.mode, extra_email = excluded.extra_email, updated_at = excluded.updated_at`,
		caller.ID, *req.Mode, extra); err != nil {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "notifications.preferences", "user", caller.ID, map[string]any{
		"mode": *req.Mode, "extra_email_set": extra != nil,
	})
	_ = json.NewEncoder(w).Encode(p.readNotificationPreferences(r, caller.ID))
}

// --- The administrator's settings -------------------------------------------

type notificationSettingsView struct {
	ExpiryDaysBefore int               `json:"expiry_days_before"`
	ExpiryChoices    []int             `json:"expiry_choices"`
	Extra            map[string]string `json:"extra"`
}

func (p *Panel) readNotificationSettings(r *http.Request) notificationSettingsView {
	view := notificationSettingsView{
		ExpiryDaysBefore: p.expiryDaysBefore(r.Context()),
		ExpiryChoices:    expiryDaysChoices,
		Extra:            map[string]string{},
	}
	for _, group := range notifyGroups {
		view.Extra[group] = p.adminGroupExtraEmail(r.Context(), group)
	}
	return view
}

func (p *Panel) handleNotificationSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil || !caller.hasAccountRole(roleAdmin) {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(p.readNotificationSettings(r))
		return
	case http.MethodPut:
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
		return
	}
	var req struct {
		ExpiryDaysBefore *int              `json:"expiry_days_before"`
		Extra            map[string]string `json:"extra"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var trailing any
	invalid := func(message string) {
		writeCodedError(w, http.StatusBadRequest, errCodeNotificationSettingsInvalid, message, "")
	}
	if err := decoder.Decode(&req); err != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) || req.ExpiryDaysBefore == nil || req.Extra == nil {
		invalid("These notification settings could not be read, so nothing was changed. Reload the page and choose again.")
		return
	}
	if !validExpiryDays(*req.ExpiryDaysBefore) {
		invalid("Choose how many days before the end date to warn from the list; nothing was changed.")
		return
	}
	values := map[string]string{}
	for group, raw := range req.Extra {
		known := false
		for _, g := range notifyGroups {
			known = known || g == group
		}
		if !known {
			invalid("These notification settings name an unknown group, so nothing was changed. Reload the page and choose again.")
			return
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			values[group] = ""
			continue
		}
		canonical, err := mutationpayload.CanonicalContactEmail(raw)
		if err != nil {
			invalid("An extra address is not a usable email address, so nothing was changed. Correct it (for example name@example.com), or leave it empty for none.")
			return
		}
		values[group] = canonical
	}
	for _, group := range notifyGroups {
		if _, ok := values[group]; !ok {
			invalid("Every group's extra address must be sent (empty for none); nothing was changed. Reload the page and choose again.")
			return
		}
	}
	tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	put := func(key, value string) error {
		_, err := tx.ExecContext(r.Context(), `INSERT INTO panel_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, value)
		return err
	}
	if err := put(settingExpiryDaysBefore, strconv.Itoa(*req.ExpiryDaysBefore)); err != nil {
		writeServerError(w, err)
		return
	}
	set := map[string]any{"expiry_days_before": *req.ExpiryDaysBefore}
	for _, group := range notifyGroups {
		if err := put("notify.extra."+group, values[group]); err != nil {
			writeServerError(w, err)
			return
		}
		set["extra_"+group+"_set"] = values[group] != ""
	}
	if err := tx.Commit(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		writeServerError(w, err)
		return
	}
	p.auditDetailed(r, "notifications.settings", "settings", 0, set)
	_ = json.NewEncoder(w).Encode(p.readNotificationSettings(r))
}
