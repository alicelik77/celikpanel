package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// Service plans: templates of a rights set (D-042; Plesk "Service Plans").
// A plan holds quantities (its max_* columns) and the permission and
// software parts (rights_sets, owner "plan"). The administrator's plans are
// global (owner_id NULL) and every reseller may use them; a reseller's own
// plans (owner_id = the reseller) are seen and used only by that reseller
// and the administrator, and never hand out a permission or software item
// outside the reseller's own set. Quantities may add up to more than a
// reseller's pool (K2: the pool limits real use). Every limit of 0 is
// unlimited. The built-in "Unlimited" plan is not a row: it is the state
// "unlimited" of a subscription.
//
// Bandwidth: bandwidth_quota_mb is stored and accepted for compatibility, but
// traffic is not measured yet, so nothing enforces it and the screens say so
// instead of showing a limit (ROADMAP v0.35 "enforce or delete").
//
// Servis planları, bir hak kümesinin şablonudur. Bayinin planı bayinin kendi
// kümesinin dışına çıkamaz; sayılar havuzu aşabilir (K2). "Sınırsız" plan bir
// satır değildir; aboneliğin "unlimited" durumudur.

type planPayload struct {
	ID               int    `json:"id,omitempty"`
	Name             string `json:"name"`
	MaxDomains       int    `json:"max_domains"`
	MaxDatabases     int    `json:"max_databases"`
	MaxEmailAccounts int    `json:"max_email_accounts"`
	DiskQuotaMB      int    `json:"disk_quota_mb"`
	BandwidthQuotaMB int    `json:"bandwidth_quota_mb"`
	Subscribers      int    `json:"subscribers,omitempty"`
	// Confirms an edit that lowers a limit below what a subscriber uses now
	// (PLAN_BELOW_USAGE); nothing is deleted.
	// Bir abonenin kullandığının altına inen düzenlemeyi onaylar.
	AcceptOverUsage bool `json:"accept_over_usage,omitempty"`
	// Rights (alpha.117): the permission and software parts. Absent on a
	// create: everything offered (D-017 #5); absent on an edit: unchanged.
	Rights *rightsPayload `json:"rights,omitempty"`
	// ConfirmNarrowing confirms an edit that takes items away from the
	// subscriptions on the plan (PLAN_NARROWING_CONFIRM shows them first).
	ConfirmNarrowing bool `json:"confirm_narrowing,omitempty"`
	// Ceilings (alpha.128, setting_limits.go): the plan's limits of site
	// settings, key -> value; absent keys are "up to the level above".
	// Absent on a create: none; absent on an edit: unchanged.
	Ceilings *ceilingsPayload `json:"ceilings,omitempty"`
	// OverusePolicy (alpha.128): "close" (D-044: traffic closes the sites,
	// 7 days over disk suspends) or "notify" (notices only; a full disk
	// still stops uploads). "" keeps the plan's (close on a create).
	OverusePolicy string `json:"overuse_policy,omitempty"`
	// NotifyPercentDisk / NotifyPercentTraffic: the notice level, 50-95
	// (0 keeps the plan's; 80 on a create).
	NotifyPercentDisk    int `json:"notify_percent_disk,omitempty"`
	NotifyPercentTraffic int `json:"notify_percent_traffic,omitempty"`
	// Read-only facts of a listed plan; a client may send them back and they
	// are ignored (ownership never comes from a request body).
	// Listelenen planın salt-okur bilgileri; istemci geri gönderebilir,
	// yok sayılır (sahiplik asla istek gövdesinden gelmez).
	OwnerID *int   `json:"owner_id,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Mine    bool   `json:"mine,omitempty"`
}

func (pl planPayload) limits() planLimits {
	return planLimits{Domains: pl.MaxDomains, Databases: pl.MaxDatabases, Mailboxes: pl.MaxEmailAccounts, DiskMB: pl.DiskQuotaMB, TrafficMB: pl.BandwidthQuotaMB}
}

const (
	maxPlanRequestBytes = 32 << 10
	maxPlanNameLength   = 100
)

// builtinPlanView is a built-in plan in a listing: everything offered and
// no numeric limit; it cannot be edited, renamed or deleted (P4).
type builtinPlanView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// reservedPlanName: the built-in names describe their behaviour (P4), so no
// other plan may carry them.
func reservedPlanName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == strings.ToLower(builtinUnlimitedName) || n == strings.ToLower(builtinUnlimitedResellerName)
}

func writePlanNameReserved(w http.ResponseWriter) {
	writeCodedError(w, http.StatusConflict, errCodePlanNameReserved,
		"\"Unlimited\" and \"Unlimited reseller\" are the built-in plans that open everything offered; choose another name for this plan. Nothing was changed.", "")
}

// planManager admits the administrator and resellers acting as themselves
// or through "Log in as" (a reseller impersonated by the administrator acts
// in the reseller's scope).
func (p *Panel) planManager(w http.ResponseWriter, r *http.Request) *Caller {
	c := currentCaller(r)
	if c == nil || (!c.hasAccountRole(roleAdmin) && !c.hasAccountRole(roleReseller)) {
		writeClientError(w, http.StatusForbidden, "administrator or reseller access required")
		return nil
	}
	return c
}

func (p *Panel) handlePlans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	c := p.planManager(w, r)
	if c == nil {
		return
	}

	switch r.Method {
	case http.MethodGet:
		query := `
			SELECT sp.id, sp.name, sp.max_domains, sp.max_databases, sp.max_email_accounts,
			       sp.disk_quota_mb, sp.bandwidth_quota_mb, sp.owner_id, COALESCE(u.username, ''),
			       (SELECT COUNT(*) FROM subscriptions s WHERE s.plan_id = sp.id),
			       sp.overuse_policy, sp.notify_percent_disk, sp.notify_percent_traffic
			FROM service_plans sp LEFT JOIN users u ON u.id = sp.owner_id`
		args := []any{}
		if !c.hasAccountRole(roleAdmin) {
			query += ` WHERE sp.owner_id IS NULL OR sp.owner_id = ?`
			args = append(args, c.ID)
		}
		query += ` ORDER BY sp.owner_id IS NOT NULL, sp.name`
		rows, err := p.db.GetDB().QueryContext(r.Context(), query, args...)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer rows.Close()

		plans := make([]planPayload, 0)
		for rows.Next() {
			var pl planPayload
			var owner sql.NullInt64
			if err := rows.Scan(&pl.ID, &pl.Name, &pl.MaxDomains, &pl.MaxDatabases,
				&pl.MaxEmailAccounts, &pl.DiskQuotaMB, &pl.BandwidthQuotaMB, &owner, &pl.Owner, &pl.Subscribers,
				&pl.OverusePolicy, &pl.NotifyPercentDisk, &pl.NotifyPercentTraffic); err != nil {
				writeServerError(w, fmt.Errorf("scan service plan: %w", err))
				return
			}
			if owner.Valid {
				id := int(owner.Int64)
				pl.OwnerID = &id
				pl.Mine = id == c.ID
			} else {
				pl.Mine = c.hasAccountRole(roleAdmin)
			}
			plans = append(plans, pl)
		}
		if err := rows.Err(); err != nil {
			writeServerError(w, fmt.Errorf("read service plans: %w", err))
			return
		}
		rows.Close()
		for i := range plans {
			set, found, err := loadRightsSet(r.Context(), p.db.GetDB(), rightsOwnerPlan, plans[i].ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			if !found {
				set = allOfferedRightsSet()
			}
			payload := set.payload()
			plans[i].Rights = &payload
			ceilings, err := loadSettingCeilings(r.Context(), p.db.GetDB(), rightsOwnerPlan, plans[i].ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			c := ceilingsPayload(ceilings)
			plans[i].Ceilings = &c
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"plans":   plans,
			"builtin": []builtinPlanView{{Key: rightsStateUnlimited, Name: builtinUnlimitedName, Kind: roleCustomer}},
		}); err != nil {
			log.Printf("encode service plans: %v", err)
		}

	case http.MethodPost:
		pl, ok := decodePlan(w, r)
		if !ok {
			return
		}
		if reservedPlanName(pl.Name) {
			writePlanNameReserved(w)
			return
		}
		set := allOfferedRightsSet()
		if pl.Rights != nil {
			parsed, err := pl.Rights.set()
			if err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return
			}
			set = parsed
		}
		if !p.planRightsWithinGrantor(w, r, c, rightsSet{}, set) {
			return
		}
		ceilings, ok := p.planCeilingsWithinGrantor(w, r, c, pl.Ceilings, map[string]int{})
		if !ok {
			return
		}
		policy, diskPct, trafficPct := planPolicyOrDefaults(pl, "close", 80, 80)
		var owner any
		if c.hasAccountRole(roleReseller) {
			owner = c.ID
		}
		tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer tx.Rollback()
		res, err := tx.ExecContext(r.Context(), `
			INSERT INTO service_plans (owner_id, name, max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb,
			                           overuse_policy, notify_percent_disk, notify_percent_traffic)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			owner, pl.Name, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts, pl.DiskQuotaMB, pl.BandwidthQuotaMB,
			policy, diskPct, trafficPct)
		if err != nil {
			writeServerError(w, err)
			return
		}
		id, err := res.LastInsertId()
		if err != nil {
			writeServerError(w, fmt.Errorf("read inserted service plan identity: %w", err))
			return
		}
		if err := saveRightsSet(r.Context(), tx, rightsOwnerPlan, int(id), set); err != nil {
			writeServerError(w, err)
			return
		}
		if err := saveSettingCeilings(r.Context(), tx, rightsOwnerPlan, int(id), ceilings); err != nil {
			writeServerError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, err)
			return
		}
		details := planAuditDetails(pl)
		details["rights"] = set.payload()
		details["ceilings"], details["overuse_policy"] = ceilings, policy
		p.auditDetailed(r, "plan.create:"+pl.Name, "service_plan", int(id), details)
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "id": id}); err != nil {
			log.Printf("encode service plan creation: %v", err)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// planRightsWithinGrantor refuses items a reseller does not have, or the
// administrator does not offer, among those newly in the plan.
func (p *Panel) planRightsWithinGrantor(w http.ResponseWriter, r *http.Request, c *Caller, before, after rightsSet) bool {
	admin, err := p.loadAdminOfferLayer(r.Context())
	if err != nil {
		writeServerError(w, err)
		return false
	}
	var reseller *rightsState
	if c.hasAccountRole(roleReseller) {
		state, err := resellerRightsState(r.Context(), p.db.GetDB(), c.ID)
		if err != nil {
			writeServerError(w, err)
			return false
		}
		reseller = &state
	}
	outside := grantorOutside(admin, reseller, newlyGranted(before, after))
	if len(outside) == 0 {
		return true
	}
	who := "the administrator does not offer them on this server"
	if reseller != nil {
		who = "they are not in your own rights; ask the administrator to include them first"
	}
	writeCodedErrorDetails(w, http.StatusConflict, errCodeRightsOutside,
		"Nothing was changed: a plan cannot include these items because "+who+".", "", outside)
	return false
}

func planAuditDetails(pl planPayload) map[string]any {
	return map[string]any{
		"max_domains": pl.MaxDomains, "max_databases": pl.MaxDatabases,
		"max_email_accounts": pl.MaxEmailAccounts, "disk_quota_mb": pl.DiskQuotaMB,
	}
}

// planEditOverUsage lists, per subscription on the plan, the kinds whose
// current usage would be above the new limits. Empty when nothing is over.
// planEditOverUsage, yeni sınırların üstünde kalacak abonelik/kalemleri listeler.
func (p *Panel) planEditOverUsage(ctx context.Context, planID int, limits planLimits) ([]string, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT s.id, s.name, COALESCE(u.username, '')
		FROM subscriptions s LEFT JOIN users u ON u.id = s.owner_id
		WHERE s.plan_id = ? ORDER BY s.id`, planID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions on service plan: %w", err)
	}
	type subRef struct {
		id          int
		name, owner string
	}
	var subs []subRef
	for rows.Next() {
		var s subRef
		if err := rows.Scan(&s.id, &s.name, &s.owner); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan subscription on service plan: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read subscriptions on service plan: %w", err)
	}
	rows.Close()

	over := []string{}
	for _, s := range subs {
		usage, err := p.subscriptionUsageFor(ctx, s.id)
		if err != nil {
			return nil, fmt.Errorf("read usage of subscription %d: %w", s.id, err)
		}
		usage.DomainsLimit, usage.DatabasesLimit, usage.MailLimit = limits.Domains, limits.Databases, limits.Mailboxes
		usage.DiskLimitBytes = 0
		if limits.DiskMB > 0 {
			usage.DiskLimitBytes = int64(limits.DiskMB) * 1024 * 1024
		}
		for _, line := range usage.overLimits() {
			over = append(over, fmt.Sprintf("%s (%s, #%d) - %s", s.owner, s.name, s.id, line))
		}
	}
	return over, nil
}

// loadPlanOwner reads a plan's owner (0 = global); errNotFound when missing.
func loadPlanOwner(ctx context.Context, q rowQueryer, planID int) (int, error) {
	var owner sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT owner_id FROM service_plans WHERE id = ?`, planID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errNotFound
	}
	if err != nil {
		return 0, err
	}
	return int(owner.Int64), nil
}

// planEditPreview is what an edit of a plan does to the subscriptions bound
// to it (6.4): how many, which items go, where they are in use, and which
// subscriptions fall below their usage. Custom subscriptions are not there.
func (p *Panel) planEditPreview(ctx context.Context, planID int, before, after rightsSet, limits planLimits) (rightsPreview, error) {
	preview := rightsPreview{From: "plan:" + strconv.Itoa(planID), To: "plan:" + strconv.Itoa(planID),
		Added: []string{}, Removed: []string{}, InUse: []inUseLine{}, OverLimit: []string{}, Outside: []string{}}
	admin, err := p.adminEffective(ctx)
	if err != nil {
		return preview, err
	}
	candidates, unknown := p.previewCandidates(ctx, before, after)
	diff := diffEffective(admin.withOwn(before), admin.withOwn(after), candidates)
	preview.Added, preview.Removed, preview.InstalledUnknown = diff.Added, diff.Removed, unknown
	subs, err := subscriptionsOnPlan(ctx, p.db.GetDB(), planID)
	if err != nil {
		return preview, err
	}
	preview.Subscriptions = len(subs)
	if preview.InUse, err = p.removedItemsInUse(ctx, subs, diff.Removed); err != nil {
		return preview, err
	}
	if preview.OverLimit, err = p.planEditOverUsage(ctx, planID, limits); err != nil {
		return preview, err
	}
	preview.LimitsTo = limits
	preview.Message = fmt.Sprintf("%d subscription(s) are bound to this plan and follow the change. "+
		"Items taken away keep running where they are used; they cannot be chosen again. Confirm to save.", len(subs))
	return preview, nil
}

func (p *Panel) handlePlanByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	c := p.planManager(w, r)
	if c == nil {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/plans/")
	idStr, action, _ := strings.Cut(rest, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 || (action != "" && action != "preview") {
		writeClientError(w, http.StatusBadRequest, "invalid plan id")
		return
	}
	// The administrator edits every plan; a reseller only its own. A plan the
	// caller may not edit is "not found" (no probing by id).
	// Yönetici her planı, bayi yalnız kendininkini düzenler.
	ownerID, err := loadPlanOwner(r.Context(), p.db.GetDB(), id)
	if errors.Is(err, errNotFound) || (err == nil && !c.hasAccountRole(roleAdmin) && ownerID != c.ID) {
		writeClientError(w, http.StatusNotFound, "service plan not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	if action == "preview" && r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}

	switch r.Method {
	case http.MethodPut, http.MethodPost:
		if r.Method == http.MethodPost && action != "preview" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		pl, ok := decodePlan(w, r)
		if !ok {
			return
		}
		if reservedPlanName(pl.Name) {
			writePlanNameReserved(w)
			return
		}
		before, found, err := loadRightsSet(r.Context(), p.db.GetDB(), rightsOwnerPlan, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if !found {
			before = allOfferedRightsSet()
		}
		after := before
		if pl.Rights != nil {
			if after, err = pl.Rights.set(); err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return
			}
		}
		if !p.planRightsWithinGrantor(w, r, c, before, after) {
			return
		}
		ceilingsBefore, err := loadSettingCeilings(r.Context(), p.db.GetDB(), rightsOwnerPlan, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		ceilingsAfter, ok := p.planCeilingsWithinGrantor(w, r, c, pl.Ceilings, ceilingsBefore)
		if !ok {
			return
		}
		currentPolicy, currentDisk, currentTraffic, err := loadPlanPolicy(r.Context(), p.db.GetDB(), id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		policy, diskPct, trafficPct := planPolicyOrDefaults(pl, currentPolicy, currentDisk, currentTraffic)
		preview, err := p.planEditPreview(r.Context(), id, before, after, pl.limits())
		if err != nil {
			writeServerError(w, err)
			return
		}
		lowered := loweredCeilings(ceilingsBefore, ceilingsAfter)
		if err := p.previewLoweredCeilings(r.Context(), &preview, id, lowered, ceilingsAfter); err != nil {
			writeServerError(w, err)
			return
		}
		if action == "preview" {
			writeRightsJSON(w, http.StatusOK, map[string]any{"preview": preview})
			return
		}
		// Taking items away is shown first (6.4, P7); so is a lowered
		// ceiling that sites of the plan are above (alpha.128).
		if (len(preview.Removed) > 0 || preview.SitesAboveCount > 0 || preview.SitesUnread) && !pl.ConfirmNarrowing {
			writeRightsPreviewRefusal(w, errCodePlanNarrowingConfirm, preview)
			return
		}
		// Plan edits also refresh subscriptions born from the plan — that is
		// the point of a template (Plesk: "apply changes to subscribers").
		// A lowered limit below what a subscriber uses now is refused with the
		// list until the operator confirms; nothing is deleted and new items
		// of those kinds are refused.
		// Abonenin kullanımının altına inen sınır, onaylanana kadar liste ile reddedilir.
		over := preview.OverLimit
		if len(over) > 0 && !pl.AcceptOverUsage {
			writeCodedErrorDetails(w, http.StatusConflict, errCodePlanBelowUsage,
				"these limits are below what some accounts on this plan use now. Nothing was changed. To save anyway, confirm: nothing is deleted, and new items of these kinds are refused until usage is under the limit.",
				"", over)
			return
		}
		tx, err := p.db.GetDB().BeginTx(r.Context(), nil)
		if err != nil {
			writeServerError(w, fmt.Errorf("begin service plan update: %w", err))
			return
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(r.Context(), `
			UPDATE service_plans SET name = ?, max_domains = ?, max_databases = ?,
			       max_email_accounts = ?, disk_quota_mb = ?, bandwidth_quota_mb = ?,
			       overuse_policy = ?, notify_percent_disk = ?, notify_percent_traffic = ?,
			       updated_at = datetime('now')
			WHERE id = ?`,
			pl.Name, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts,
			pl.DiskQuotaMB, pl.BandwidthQuotaMB, policy, diskPct, trafficPct, id)
		if err != nil {
			writeServerError(w, fmt.Errorf("update service plan: %w", err))
			return
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			writeServerError(w, fmt.Errorf("update service plan: affected %d, %v", n, err))
			return
		}
		if _, err := tx.ExecContext(r.Context(), `
			UPDATE subscriptions SET max_domains = ?, max_databases = ?,
			       max_email_accounts = ?, disk_quota_mb = ?, bandwidth_quota_mb = ?,
			       updated_at = datetime('now')
			WHERE plan_id = ?`,
			pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts,
			pl.DiskQuotaMB, pl.BandwidthQuotaMB, id); err != nil {
			writeServerError(w, fmt.Errorf("apply service plan to subscriptions: %w", err))
			return
		}
		if err := saveRightsSet(r.Context(), tx, rightsOwnerPlan, id, after); err != nil {
			writeServerError(w, err)
			return
		}
		if err := saveSettingCeilings(r.Context(), tx, rightsOwnerPlan, id, ceilingsAfter); err != nil {
			writeServerError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, fmt.Errorf("commit service plan update: %w", err))
			return
		}
		details := planAuditDetails(pl)
		details["rights_added"], details["rights_removed"] = preview.Added, preview.Removed
		details["subscriptions"] = preview.Subscriptions
		details["ceilings"], details["ceilings_lowered"], details["sites_above"] = ceilingsAfter, lowered, preview.SitesAboveCount
		details["overuse_policy"], details["notify_percent_disk"], details["notify_percent_traffic"] = policy, diskPct, trafficPct
		if len(over) > 0 {
			// The audit row is size-bounded; keep the first entries and the total.
			shown := over
			if len(shown) > 40 {
				shown = shown[:40]
			}
			details["over_limit"] = shown
			details["over_total"] = len(over)
		}
		p.auditDetailed(r, "plan.update:"+pl.Name, "service_plan", id, details)
		// A lowered ceiling reaches the sites now, not at their next
		// render: their files are written again in the background.
		if len(lowered) > 0 {
			p.storeClampNotices(r.Context(), preview.clamped, settingSetByPlan, pl.Name)
			p.reapplyPlanSiteLimits(id)
		}
		// alpha.128 (G5): database users follow a changed connection limit.
		p.reapplyDBConnectionLimitsAfter(r.Context(), rightsOwnerPlan, id, ceilingsBefore, ceilingsAfter)
		// alpha.128 (integration): outgoing mail follows the mail keys.
		p.requestOutgoingApply()
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			log.Printf("encode service plan update: %v", err)
		}

	case http.MethodDelete:
		result, err := p.db.GetDB().ExecContext(r.Context(), `
			DELETE FROM service_plans
			WHERE id = ?
			  AND NOT EXISTS (SELECT 1 FROM subscriptions WHERE plan_id = service_plans.id)`, id)
		if err != nil {
			writeServerError(w, fmt.Errorf("delete service plan: %w", err))
			return
		}
		affected, err := result.RowsAffected()
		if err != nil {
			writeServerError(w, fmt.Errorf("verify service plan deletion: %w", err))
			return
		}
		if affected == 0 {
			writeClientError(w, http.StatusConflict, "plan has subscriptions; move them to another plan first")
			return
		}
		p.audit(r, "plan.delete:"+strconv.Itoa(id), "service_plan", id)
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			log.Printf("encode service plan deletion: %v", err)
		}

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodePlan(w http.ResponseWriter, r *http.Request) (planPayload, bool) {
	var pl planPayload
	r.Body = http.MaxBytesReader(w, r.Body, maxPlanRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pl); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid service plan")
		return pl, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid service plan")
		return pl, false
	}
	pl.Name = strings.TrimSpace(pl.Name)
	if pl.Name == "" {
		writeClientError(w, http.StatusBadRequest, "plan name is required")
		return pl, false
	}
	if len(pl.Name) > maxPlanNameLength {
		writeClientError(w, http.StatusBadRequest, "plan name is too long (100 characters at most)")
		return pl, false
	}
	// 0 = unlimited for every limit (quota.go); negative numbers mean nothing.
	// Her sınır için 0 = sınırsız; negatif sayının anlamı yok.
	if pl.MaxDomains < 0 || pl.MaxDatabases < 0 || pl.MaxEmailAccounts < 0 ||
		pl.DiskQuotaMB < 0 || pl.BandwidthQuotaMB < 0 {
		writeClientError(w, http.StatusBadRequest, "limits must be 0 (unlimited) or a positive number")
		return pl, false
	}
	if pl.OverusePolicy != "" && pl.OverusePolicy != overusePolicyClose && pl.OverusePolicy != overusePolicyNotify {
		writeClientError(w, http.StatusBadRequest, "overuse_policy must be close or notify")
		return pl, false
	}
	for _, pct := range []int{pl.NotifyPercentDisk, pl.NotifyPercentTraffic} {
		if pct != 0 && (pct < 50 || pct > 95) {
			writeClientError(w, http.StatusBadRequest, "a notice level must be between 50 and 95 percent")
			return pl, false
		}
	}
	return pl, true
}
