package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Usage notices and the administrator's view of the measurements (alpha.118,
// D-044).
//
//	GET  /api/v1/usage-notices             the caller's unseen notices
//	POST /api/v1/usage-notices/{id}/seen   marks one as seen
//	GET  /api/v1/usage/status              administrator: is measuring working
//
// Polling only reads; nothing here starts a measurement or a change.

const (
	usageNoticesPath = "/api/v1/usage-notices"
	usageStatusPath  = "/api/v1/usage/status"
)

type usageNoticeView struct {
	ID             int             `json:"id"`
	Kind           string          `json:"kind"`
	Audience       string          `json:"audience"`
	SubscriptionID int             `json:"subscription_id,omitempty"`
	CreatedAt      string          `json:"created_at"`
	Body           usageNoticeBody `json:"body"`
}

func (p *Panel) handleUsageNotices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	caller := currentCaller(r)
	if caller == nil {
		writeClientError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	db := p.db.GetDB()
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, usageNoticesPath), "/")
	if rest == "" && r.Method == http.MethodGet {
		rows, err := db.QueryContext(r.Context(), `
			SELECT id, kind, audience, subscription_id, body, created_at FROM usage_notices
			WHERE user_id = ? AND seen_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 50`, caller.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer rows.Close()
		views := []usageNoticeView{}
		for rows.Next() {
			var v usageNoticeView
			var raw string
			if err := rows.Scan(&v.ID, &v.Kind, &v.Audience, &v.SubscriptionID, &raw, &v.CreatedAt); err != nil {
				writeServerError(w, err)
				return
			}
			if json.Unmarshal([]byte(raw), &v.Body) != nil {
				continue
			}
			views = append(views, v)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"notices": views})
		return
	}
	idText, action, _ := strings.Cut(rest, "/")
	id, err := strconv.Atoi(idText)
	if err != nil || id <= 0 || action != "seen" || r.Method != http.MethodPost {
		writeClientError(w, http.StatusNotFound, "not found")
		return
	}
	result, err := db.ExecContext(r.Context(), `UPDATE usage_notices SET seen_at = COALESCE(seen_at, datetime('now')) WHERE id = ? AND user_id = ?`, id, caller.ID)
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

// usageUnknownSite is one website whose traffic is not counted. Reason is
// who acts: changed_by_owner (CelikPanel's traffic file or the site's vhost
// was changed by hand on the server: the administrator decides whether to
// keep that or let CelikPanel write its file) or not_written_yet
// (CelikPanel's file is not in place yet: CelikPanel writes it with the
// site's next configuration write). Detail is the Agent's reviewed code.
type usageUnknownSite struct {
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

const (
	usageUnknownChangedByOwner = "changed_by_owner"
	usageUnknownNotWrittenYet  = "not_written_yet"
	usageUnknownSitesLimit     = 50
)

// usageUnknownReason maps the Agent's reviewed code to who acts.
func usageUnknownReason(detail string) string {
	switch detail {
	case transport.TrafficReasonFormatKept, transport.TrafficReasonVhostKept:
		return usageUnknownChangedByOwner
	default:
		// include_missing, or a code this Panel does not know yet: the
		// file is not (yet) in place.
		return usageUnknownNotWrittenYet
	}
}

// usageStatusView: whether measuring works on this server, for the
// administrator, with the reason and the next step when it does not.
type usageStatusView struct {
	Traffic struct {
		LastCollected string         `json:"last_collected,omitempty"`
		LastAttempt   string         `json:"last_attempt,omitempty"`
		Error         string         `json:"error,omitempty"`
		Logrotate     string         `json:"logrotate,omitempty"`
		Unknown       map[string]int `json:"unknown"`
		// UnknownSites names the websites that are not counted, so the
		// administrator sees which ones and why (at most
		// usageUnknownSitesLimit, by name; UnknownSitesMore says how many
		// more there are).
		UnknownSites     []usageUnknownSite `json:"unknown_sites"`
		UnknownSitesMore int                `json:"unknown_sites_more,omitempty"`
		Measuring        int                `json:"measuring"`
		IntervalMinute   int                `json:"interval_minutes"`
	} `json:"traffic"`
	Disk struct {
		Measured     int  `json:"measured"`
		Unknown      int  `json:"unknown"`
		NotMeasured  int  `json:"not_measured"`
		LowSpace     bool `json:"low_space"`
		IntervalHour int  `json:"interval_hours"`
	} `json:"disk"`
	MailQuota struct {
		State   string `json:"state"`
		Dialect string `json:"dialect,omitempty"`
		Detail  string `json:"detail,omitempty"`
		Error   string `json:"error,omitempty"`
		Checked string `json:"checked,omitempty"`
	} `json:"mail_quota"`
}

func (p *Panel) handleUsageStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	c := currentCaller(r)
	if !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator access required", "")
		return
	}
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	var view usageStatusView
	view.Traffic.Unknown = map[string]int{}
	view.Traffic.IntervalMinute = int(usageWorkerTick / time.Minute)
	var last, attempt, lastErr, logrotate sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT last_success_at, last_attempt_at, last_error, logrotate FROM traffic_collect_state WHERE id = 1`).
		Scan(&last, &attempt, &lastErr, &logrotate); err != nil && err != sql.ErrNoRows {
		writeServerError(w, err)
		return
	}
	view.Traffic.LastCollected, view.Traffic.LastAttempt = last.String, attempt.String
	view.Traffic.Error, view.Traffic.Logrotate = lastErr.String, logrotate.String
	// A website whose domain is being deleted is not counted as a problem:
	// its files are removed first and its rows go when the deletion ends
	// (6 Nov 2026: a joined server's pending deletion read as "1 website
	// not counted" for as long as the main DNS server confirmed).
	view.Traffic.UnknownSites = []usageUnknownSite{}
	rows, err := db.QueryContext(ctx, `
		SELECT ts.domain_id, COALESCE(d.name, ''), ts.state, COALESCE(ts.reason, '')
		FROM traffic_sources ts LEFT JOIN domains d ON d.id = ts.domain_id
		WHERE NOT EXISTS (SELECT 1 FROM domain_deletion_operations op WHERE op.domain_id = ts.domain_id)
		ORDER BY d.name, ts.domain_id`)
	if err != nil {
		writeServerError(w, err)
		return
	}
	for rows.Next() {
		var domainID int
		var name, state, reason string
		if err := rows.Scan(&domainID, &name, &state, &reason); err != nil {
			rows.Close()
			writeServerError(w, err)
			return
		}
		if state != "unknown" {
			view.Traffic.Measuring++
			continue
		}
		view.Traffic.Unknown[reason]++
		if len(view.Traffic.UnknownSites) >= usageUnknownSitesLimit {
			view.Traffic.UnknownSitesMore++
			continue
		}
		view.Traffic.UnknownSites = append(view.Traffic.UnknownSites, usageUnknownSite{
			DomainID: domainID, Domain: name, Reason: usageUnknownReason(reason), Detail: reason,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeServerError(w, err)
		return
	}
	rows.Close()
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN u.measured_at IS NOT NULL THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN u.measured_at IS NULL AND u.detail IS NOT NULL THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN u.subscription_id IS NULL OR (u.measured_at IS NULL AND u.detail IS NULL) THEN 1 ELSE 0 END), 0)
		FROM subscriptions s LEFT JOIN subscription_disk_usage u ON u.subscription_id = s.id`).
		Scan(&view.Disk.Measured, &view.Disk.Unknown, &view.Disk.NotMeasured); err != nil {
		writeServerError(w, err)
		return
	}
	view.Disk.LowSpace = p.usageWorker.lowSpace()
	view.Disk.IntervalHour = int(diskMeasureInterval / time.Hour)
	quota, quotaErr, checked := p.usageWorker.mailQuotaStatus()
	view.MailQuota.State, view.MailQuota.Dialect, view.MailQuota.Detail = quota.State, quota.Dialect, quota.Detail
	view.MailQuota.Error = quotaErr
	if quota.Error != "" {
		view.MailQuota.Error = quota.Error
	}
	if view.MailQuota.State == "" {
		view.MailQuota.State = "not_checked_yet"
	}
	if !checked.IsZero() {
		view.MailQuota.Checked = checked.UTC().Format(time.RFC3339)
	}
	_ = json.NewEncoder(w).Encode(view)
}
