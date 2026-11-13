package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alicelik/celikpanel/internal/core"
)

// The domain health strip (alpha.128, G22): one compact line on a domain's
// Overview that says, for each thing that keeps a site and its mail working,
// either "fine" or what is wrong and the one action that fixes it.
//
// Every answer comes from what the panel already keeps; nothing slow is asked
// while the page loads (D-024, alpha.127 display checks):
//   - site:        the latest website check (site_uptime, every five minutes);
//   - certificate: the active certificate's last day (ssl_certificates) and
//     whether the site serves a certificate valid for its name (site_uptime);
//   - mail:        the deliverability card's last answer (display checks),
//     never a new check;
//   - disk:        the subscription's last disk measurement against its limit;
//   - backup:      the backup schedule's last run and the last backup made
//     through the panel (audit trail, a bounded look at its newest rows).
//
// "DNS points here" is the Connection card's own answer, read on the same
// page with the card's request (web), so it is not asked twice.
//
// An answer the panel does not have says "not checked yet", never fine. The
// fix is role-aware: whoever can act gets the place to act; a customer or a
// team member whose fix belongs to someone else is told whom to contact.
// PHP errors are not on the strip: the PHP error log is read from the server
// on demand (Logs), there is no stored count to show without asking.
//
// Alan adı sağlık şeridi: her denetim ya iyidir ya da neyin yanlış olduğunu
// ve onu düzelten tek eylemi söyler. Veriler panelin zaten sakladıklarından
// gelir; sayfa açılırken yavaş hiçbir şey sorulmaz. Bilinmeyen yanıt "henüz
// denetlenmedi" der, asla iyi demez.

const (
	domainHealthKind = "health"
	// A certificate with this many days or fewer left is a problem.
	domainHealthCertificateDays = 14
	// A backup older than this is a problem.
	domainHealthBackupAge = 7 * 24 * time.Hour
	// The newest audit rows looked at for the last manual backup: bounded,
	// so a large trail never slows the page. An older backup is then not
	// seen, which only ever makes the answer more cautious.
	domainHealthAuditWindow = 20000
)

var domainHealthNow = time.Now

type domainHealthFix struct {
	// Kind: tab (this page's tab), page (another panel page), contact (the
	// person named by the response's contact).
	Kind string `json:"kind"`
	Tab  string `json:"tab,omitempty"`
	Sub  string `json:"sub,omitempty"`
	Path string `json:"path,omitempty"`
}

type domainHealthCheck struct {
	ID string `json:"id"`
	// State: ok, problem, waiting (a change is on its way, nothing to do),
	// unknown (not checked yet or not readable).
	State string `json:"state"`
	Code  string `json:"code"`
	// At: when the shown answer was observed.
	At string `json:"at,omitempty"`
	// Earlier: the answer was read before a later change; it is being
	// checked again where it is shown in full.
	Earlier    bool             `json:"earlier,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	Days       *int             `json:"days,omitempty"`
	Percent    *int             `json:"percent,omitempty"`
	UsedBytes  *int64           `json:"used_bytes,omitempty"`
	LimitBytes *int64           `json:"limit_bytes,omitempty"`
	Items      []string         `json:"items,omitempty"`
	Fix        *domainHealthFix `json:"fix,omitempty"`
}

// domainHealthViewer is who reads the strip, and what they can act on.
type domainHealthViewer struct {
	admin bool
	// planManager: may change the subscription's limits (the administrator,
	// or the reseller over a customer's subscription).
	planManager  bool
	team         bool
	capabilities map[core.TeamCapability]core.TeamPermissionMode
	rights       *effectiveRights
	// contact: administrator, provider (the reseller) or owner (the account
	// owner, for a team member).
	contact string
}

func (v domainHealthViewer) sees(capabilities ...core.TeamCapability) bool {
	if !v.team {
		return true
	}
	for _, capability := range capabilities {
		if _, ok := v.capabilities[capability]; ok {
			return true
		}
	}
	return false
}

// manages: may this viewer do the change on the tab (capability + any of
// the subscription's rights)?
func (v domainHealthViewer) manages(capability core.TeamCapability, rights ...string) bool {
	if v.admin {
		return true
	}
	if v.team && v.capabilities[capability] != core.TeamPermissionManage {
		return false
	}
	if len(rights) == 0 || v.rights == nil {
		return true
	}
	for _, right := range rights {
		if v.rights.hasPermission(right) {
			return true
		}
	}
	return false
}

func domainHealthContact() *domainHealthFix { return &domainHealthFix{Kind: "contact"} }

// tabFix: the tab where the fix is, or whom to contact.
func (v domainHealthViewer) tabFix(tab, sub string, capability core.TeamCapability, rights ...string) *domainHealthFix {
	if v.manages(capability, rights...) {
		return &domainHealthFix{Kind: "tab", Tab: tab, Sub: sub}
	}
	return domainHealthContact()
}

// serverFix: the fix is the administrator's (the server, the mail system).
func (v domainHealthViewer) serverFix(fix domainHealthFix) *domainHealthFix {
	if v.admin {
		return &fix
	}
	return domainHealthContact()
}

func domainHealthInt(n int) *int { return &n }

// handleDomainHealth: GET /api/v1/domains/{id}/health.
func (p *Panel) handleDomainHealth(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		rejectRouteMethod(w, []string{http.MethodGet})
		return
	}
	ctx := r.Context()
	var name, projectType string
	var subID int
	var hasSite bool
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT d.name, d.subscription_id,
		       COALESCE((SELECT project_type FROM sites s WHERE s.domain_id = d.id LIMIT 1), ''),
		       EXISTS (SELECT 1 FROM sites s WHERE s.domain_id = d.id)
		FROM domains d WHERE d.id = ?`, domainID).Scan(&name, &subID, &projectType, &hasSite)
	if errors.Is(err, sql.ErrNoRows) {
		writeClientError(w, http.StatusNotFound, "domain not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	viewer, err := p.domainHealthViewerFor(ctx, currentCaller(r), domainID, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	now := domainHealthNow().UTC()
	website := hasSite && projectType != "dnsonly"
	checks := []domainHealthCheck{}
	if website && viewer.sees(core.TeamCapabilityStatistics, core.TeamCapabilityFiles) {
		checks = append(checks, p.domainHealthSite(ctx, name, now, viewer))
	}
	if website && viewer.sees(core.TeamCapabilitySSL) {
		checks = append(checks, p.domainHealthCertificate(ctx, domainID, name, now, viewer))
	}
	if viewer.sees(core.TeamCapabilityMail) {
		if check, shown := p.domainHealthMail(ctx, domainID, name, viewer); shown {
			checks = append(checks, check)
		}
	}
	if website && viewer.sees(core.TeamCapabilityStatistics) {
		checks = append(checks, p.domainHealthDisk(ctx, subID, viewer))
	}
	if website && viewer.sees(core.TeamCapabilityBackups) {
		checks = append(checks, p.domainHealthBackup(ctx, domainID, now, viewer))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"domain":     name,
		"contact":    viewer.contact,
		"checks":     checks,
		"checked_at": now.Format(time.RFC3339),
	})
}

func (p *Panel) domainHealthViewerFor(ctx context.Context, caller *Caller, domainID, subID int) (domainHealthViewer, error) {
	viewer := domainHealthViewer{contact: "administrator"}
	if caller.hasAccountRole(roleAdmin) {
		viewer.admin, viewer.planManager = true, true
		return viewer, nil
	}
	rights, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		return viewer, err
	}
	viewer.rights = rights
	if caller.isAdditionalUser() {
		viewer.team, viewer.contact = true, "owner"
		viewer.capabilities, err = p.teamMemberEffectiveDomainCapabilities(ctx, caller, domainID)
		return viewer, err
	}
	ownerID, resellerID, err := subscriptionOwnerChain(ctx, p.db.GetDB(), subID)
	if err != nil {
		return viewer, err
	}
	if caller.hasAccountRole(roleReseller) {
		// A reseller sets its customers' limits; its own come from the
		// administrator.
		viewer.planManager = caller.ID == resellerID && ownerID != caller.ID
		return viewer, nil
	}
	if resellerID != 0 && resellerID != ownerID {
		viewer.contact = "provider"
	}
	return viewer, nil
}

// domainHealthSite: the latest website check of this site.
func (p *Panel) domainHealthSite(ctx context.Context, name string, now time.Time, v domainHealthViewer) domainHealthCheck {
	check := domainHealthCheck{ID: "site"}
	if !p.alertSettings(ctx).SiteChecks {
		check.State, check.Code = "unknown", "checks_off"
		check.Fix = v.serverFix(domainHealthFix{Kind: "page", Path: "/monitoring"})
		return check
	}
	var checkedAt, state, reason string
	err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT checked_at, state, reason FROM site_uptime WHERE domain = ?`, name).Scan(&checkedAt, &state, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		check.State, check.Code = "unknown", "not_checked"
		return check
	}
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	check.At = checkedAt
	if at, err := time.Parse(time.RFC3339, checkedAt); err != nil || now.Sub(at) > siteCheckStale {
		check.State, check.Code = "unknown", "stale"
		return check
	}
	if state != "down" {
		check.State, check.Code = "ok", "up"
		return check
	}
	check.State, check.Code, check.Reason = "problem", "down", reason
	if reason == "http_5xx" {
		// The site itself answers with an error: its error log says why.
		check.Fix = v.tabFix("advanced", "logs", core.TeamCapabilityStatistics)
	} else {
		check.Fix = v.serverFix(domainHealthFix{Kind: "page", Path: "/monitoring"})
	}
	return check
}

// domainHealthCertificate: the active certificate's last day, and whether
// the site serves a certificate valid for its name.
func (p *Panel) domainHealthCertificate(ctx context.Context, domainID int, name string, now time.Time, v domainHealthViewer) domainHealthCheck {
	check := domainHealthCheck{ID: "certificate"}
	fix := v.tabFix("hosting", "ssl", core.TeamCapabilitySSL, rightSSLACME, rightSSLCustom)
	var expires string
	err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT expires_at FROM ssl_certificates
		WHERE domain_id = ? AND status = 'active'
		ORDER BY expires_at DESC LIMIT 1`, domainID).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		check.State, check.Code, check.Fix = "problem", "none", fix
		return check
	}
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	until := parseDBTime(expires)
	if until.IsZero() {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	check.At = until.UTC().Format(time.RFC3339)
	days := int(until.Sub(now).Hours() / 24)
	switch {
	case !until.After(now):
		check.State, check.Code, check.Fix = "problem", "expired", fix
		return check
	case days <= domainHealthCertificateDays:
		check.State, check.Code, check.Days, check.Fix = "problem", "expiring", domainHealthInt(days), fix
		return check
	}
	// The site serves a certificate that is not valid for its name (a renewed
	// certificate the web server never loaded, or another site's).
	var checkedAt string
	var valid sql.NullInt64
	if err := p.db.GetDB().QueryRowContext(ctx,
		`SELECT checked_at, cert_valid FROM site_uptime WHERE domain = ? AND scheme = 'https'`, name).Scan(&checkedAt, &valid); err == nil {
		if at, err := time.Parse(time.RFC3339, checkedAt); err == nil && now.Sub(at) <= siteCheckStale && valid.Valid && valid.Int64 == 0 {
			check.State, check.Code, check.At, check.Fix = "problem", "served_invalid", checkedAt, fix
			return check
		}
	}
	check.State, check.Code, check.Days = "ok", "valid", domainHealthInt(days)
	return check
}

// domainHealthMail: the deliverability card's last answer, never a new
// check. A domain without mailboxes and without a known answer has no mail
// line.
func (p *Panel) domainHealthMail(ctx context.Context, domainID int, name string, v domainHealthViewer) (domainHealthCheck, bool) {
	check := domainHealthCheck{ID: "mail"}
	mailFix := v.tabFix("mail", "", core.TeamCapabilityMail, rightMail)
	mode, err := p.domainDNSManagementMode(ctx, name)
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check, true
	}
	report, view, known := p.displayChecks.ready().mailHealth.peek(fmt.Sprintf("%d|%s|%s", domainID, name, mode))
	if !known {
		var mailboxes int
		if err := p.db.GetDB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM email_accounts WHERE domain_id = ?`, domainID).Scan(&mailboxes); err != nil {
			check.State, check.Code = "unknown", "unreadable"
			return check, true
		}
		if mailboxes == 0 {
			return check, false
		}
		check.State, check.Code, check.Fix = "unknown", "not_checked", mailFix
		return check, true
	}
	check.At, check.Earlier = view.CheckedAt.UTC().Format(time.RFC3339), view.Rechecking
	status := map[string]string{}
	var rbl healthCheck
	for _, c := range report.Checks {
		status[c.ID] = c.Status
		if c.ID == "rbl" {
			rbl = c
		}
	}
	var missing, pending []string
	for _, id := range []string{"spf", "dkim", "dmarc"} {
		switch status[id] {
		case "fail":
			missing = append(missing, id)
		case "warn":
			pending = append(pending, id)
		}
	}
	serverFix := v.serverFix(domainHealthFix{Kind: "tab", Tab: "mail"})
	switch {
	case len(missing) > 0:
		check.State, check.Code, check.Items, check.Fix = "problem", "auth_missing", missing, mailFix
	case status["port25"] == "fail":
		check.State, check.Code, check.Fix = "problem", "port25_blocked", serverFix
	case status["rbl"] == "fail":
		// alpha.133: only a documented listing is "fail"; a list that could
		// not be asked is "unknown" there and never reaches this line. A
		// policy entry (Spamhaus PBL) is said as what it is.
		check.State, check.Code, check.Items, check.Fix = "problem", "listed", rbl.Items, serverFix
		if rbl.Detail == "policy" {
			check.Code = "listed_policy"
		}
	case status["ptr"] == "fail":
		check.State, check.Code, check.Fix = "problem", "ptr_missing", serverFix
	case len(pending) > 0:
		check.State, check.Code, check.Items = "waiting", "auth_spreading", pending
	default:
		check.State, check.Code = "ok", "complete"
	}
	return check, true
}

// domainHealthDisk: the subscription's last disk measurement against its
// limit.
func (p *Panel) domainHealthDisk(ctx context.Context, subID int, v domainHealthViewer) domainHealthCheck {
	check := domainHealthCheck{ID: "disk"}
	db := p.db.GetDB()
	var limitMB int64
	var adminWorkspace bool
	err := db.QueryRowContext(ctx, `
		SELECT COALESCE(s.disk_quota_mb, 0),
		       o.role = 'admin' AND s.plan_id IS NULL AND s.name = 'Admin Subscription'
		FROM subscriptions s JOIN users o ON o.id = s.owner_id
		WHERE s.id = ?`, subID).Scan(&limitMB, &adminWorkspace)
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	view, err := loadSubscriptionDiskView(ctx, db, subID)
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	switch view.State {
	case "measured", "partial":
	case "not_measured_yet":
		check.State, check.Code = "unknown", "not_measured"
		return check
	default:
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	used := view.WebBytes + view.MailBytes + view.DBBytes
	check.At, check.UsedBytes = view.MeasuredAt, &used
	if adminWorkspace || limitMB <= 0 {
		check.State, check.Code = "ok", "no_limit"
		return check
	}
	limit := limitMB * mib
	percent := int(used * 100 / limit)
	check.LimitBytes, check.Percent = &limit, &percent
	var fix *domainHealthFix
	switch {
	case v.planManager:
		fix = &domainHealthFix{Kind: "page", Path: "/users"}
	case v.manages(core.TeamCapabilityFiles, rightFileManager):
		// Making room is the owner's own action; more room is the
		// provider's (the words name both).
		fix = &domainHealthFix{Kind: "tab", Tab: "files"}
	default:
		fix = domainHealthContact()
	}
	switch {
	case used >= limit:
		check.State, check.Code, check.Fix = "problem", "over", fix
	case usageLevel(used, limit) >= 80:
		check.State, check.Code, check.Fix = "problem", "near", fix
	default:
		check.State, check.Code = "ok", "within"
	}
	return check
}

// domainHealthBackup: the schedule's last run and the last backup made
// through the panel.
func (p *Panel) domainHealthBackup(ctx context.Context, domainID int, now time.Time, v domainHealthViewer) domainHealthCheck {
	check := domainHealthCheck{ID: "backup"}
	fix := v.tabFix("advanced", "backups", core.TeamCapabilityBackups, rightBackups)
	settings, status, err := p.readBackupSchedule(ctx, domainID)
	if err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	var manual sql.NullString
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT MAX(created_at) FROM audit_logs
		WHERE id > (SELECT COALESCE(MAX(id), 0) FROM audit_logs) - ?
		  AND resource_type = 'domain' AND resource_id = ?
		  AND action LIKE 'backup.create:%'`, domainHealthAuditWindow, domainID).Scan(&manual); err != nil {
		check.State, check.Code = "unknown", "unreadable"
		return check
	}
	var last time.Time
	if status.lastRun != nil {
		last = parseDBTime(*status.lastRun)
	}
	if manual.Valid {
		if at := parseDBTime(manual.String); at.After(last) {
			last = at
		}
	}
	scheduled := settings.exists && settings.enabled == 1
	if !last.IsZero() {
		check.At = last.UTC().Format(time.RFC3339)
		check.Days = domainHealthInt(int(now.Sub(last).Hours() / 24))
	}
	switch {
	case scheduled && status.lastStatus != nil && *status.lastStatus == "failed":
		check.State, check.Code, check.Fix = "problem", "schedule_failed", fix
		if status.lastAttempt != nil {
			check.At = *status.lastAttempt
		}
	case !last.IsZero() && now.Sub(last) < domainHealthBackupAge:
		check.State, check.Code = "ok", "recent"
		if !scheduled {
			check.Code = "recent_manual"
		}
	case scheduled && last.IsZero() && status.lastAttempt == nil:
		// The schedule's first run has not come yet.
		check.State, check.Code = "unknown", "first_pending"
	case last.IsZero():
		check.State, check.Code, check.Fix = "problem", "none", fix
	default:
		check.State, check.Code, check.Fix = "problem", "old", fix
		if !scheduled {
			check.Code = "old_no_schedule"
		}
	}
	return check
}
