package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Reseller plans, the rights catalog and rights notices (alpha.117, D-042).
//
//	GET/POST          /api/v1/reseller-plans                 administrator
//	GET/PUT/DELETE    /api/v1/reseller-plans/{id}            administrator
//	POST              /api/v1/reseller-plans/{id}/preview    administrator
//	GET               /api/v1/rights/catalog[?subscription_id=]  what the caller may choose or hand out
//	GET               /api/v1/rights/notices                 administrator: all; reseller: its own
//	POST              /api/v1/rights/notices/{id}/acknowledge
//	GET/PUT           /api/v1/rights/offered-permissions     administrator (alpha.128)
//	GET/PUT           /api/v1/rights/setting-limits          limits of site settings (alpha.128)
//	GET/PUT           /api/v1/rights/password-level          the password level (alpha.128; PUT administrator)
//
// Bayi planları, hak kataloğu ve hak bildirimleri.

const (
	resellerPlansPath = "/api/v1/reseller-plans"
	rightsPathPrefix  = "/api/v1/rights/"
)

type resellerPlanPayload struct {
	ID               int            `json:"id,omitempty"`
	Name             string         `json:"name"`
	MaxCustomers     int            `json:"max_customers"`
	MaxDomains       int            `json:"max_domains"`
	MaxDatabases     int            `json:"max_databases"`
	MaxEmailAccounts int            `json:"max_email_accounts"`
	DiskQuotaMB      int            `json:"disk_quota_mb"`
	TrafficQuotaMB   int            `json:"traffic_quota_mb"`
	Rights           *rightsPayload `json:"rights,omitempty"`
	ConfirmNarrowing bool           `json:"confirm_narrowing,omitempty"`
	// alpha.128: limits of site settings for the reseller's tree (nil:
	// none on a create, unchanged on an edit) and the overselling choice
	// ("allowed" is K2, the default; "" keeps it).
	Ceilings    *ceilingsPayload `json:"ceilings,omitempty"`
	Overselling string           `json:"overselling,omitempty"`
	// Read-only: how many resellers are bound to it.
	Resellers int `json:"resellers,omitempty"`
}

func (pl resellerPlanPayload) pool() resellerPool {
	return resellerPool{MaxCustomers: pl.MaxCustomers, planLimits: planLimits{
		Domains: pl.MaxDomains, Databases: pl.MaxDatabases, Mailboxes: pl.MaxEmailAccounts, DiskMB: pl.DiskQuotaMB,
		TrafficMB: pl.TrafficQuotaMB}}
}

func decodeResellerPlan(w http.ResponseWriter, r *http.Request) (resellerPlanPayload, bool) {
	var pl resellerPlanPayload
	r.Body = http.MaxBytesReader(w, r.Body, maxPlanRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pl); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid reseller plan")
		return pl, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeClientError(w, http.StatusBadRequest, "invalid reseller plan")
		return pl, false
	}
	pl.Name = strings.TrimSpace(pl.Name)
	if pl.Name == "" || len(pl.Name) > maxPlanNameLength {
		writeClientError(w, http.StatusBadRequest, "plan name is required (100 characters at most)")
		return pl, false
	}
	if reservedPlanName(pl.Name) {
		writePlanNameReserved(w)
		return pl, false
	}
	if !pl.pool().valid() {
		writeClientError(w, http.StatusBadRequest, "limits must be 0 (unlimited) or a positive number")
		return pl, false
	}
	if !validOverselling(pl.Overselling) {
		writeClientError(w, http.StatusBadRequest, "overselling must be allowed or not_allowed")
		return pl, false
	}
	return pl, true
}

func (p *Panel) requireRightsAdministrator(w http.ResponseWriter, r *http.Request) *Caller {
	c := currentCaller(r)
	if !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator manages reseller plans", "")
		return nil
	}
	return c
}

func (p *Panel) handleResellerPlans(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if p.requireRightsAdministrator(w, r) == nil {
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	switch r.Method {
	case http.MethodGet:
		rows, err := db.QueryContext(ctx, `
			SELECT rp.id, rp.name, rp.max_customers, rp.max_domains, rp.max_databases, rp.max_email_accounts, rp.disk_quota_mb,
			       rp.traffic_quota_mb, (SELECT COUNT(*) FROM reseller_plan_bindings b WHERE b.reseller_plan_id = rp.id),
			       rp.overselling
			FROM reseller_plans rp ORDER BY rp.name, rp.id`)
		if err != nil {
			writeServerError(w, err)
			return
		}
		plans := []resellerPlanPayload{}
		for rows.Next() {
			var pl resellerPlanPayload
			if err := rows.Scan(&pl.ID, &pl.Name, &pl.MaxCustomers, &pl.MaxDomains, &pl.MaxDatabases, &pl.MaxEmailAccounts, &pl.DiskQuotaMB, &pl.TrafficQuotaMB, &pl.Resellers, &pl.Overselling); err != nil {
				rows.Close()
				writeServerError(w, err)
				return
			}
			plans = append(plans, pl)
		}
		rows.Close()
		for i := range plans {
			plan, err := loadResellerPlan(ctx, db, plans[i].ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			payload := plan.Set.payload()
			plans[i].Rights = &payload
			ceilings, err := loadSettingCeilings(ctx, db, rightsOwnerResellerPlan, plans[i].ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			c := ceilingsPayload(ceilings)
			plans[i].Ceilings = &c
		}
		writeRightsJSON(w, http.StatusOK, map[string]any{
			"plans":   plans,
			"builtin": []builtinPlanView{{Key: rightsStateUnlimited, Name: builtinUnlimitedResellerName, Kind: roleReseller}},
		})
	case http.MethodPost:
		pl, ok := decodeResellerPlan(w, r)
		if !ok {
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
		if !p.planRightsWithinGrantor(w, r, currentCaller(r), rightsSet{}, set) {
			return
		}
		ceilings, ok := p.planCeilingsWithinGrantor(w, r, currentCaller(r), pl.Ceilings, map[string]int{})
		if !ok {
			return
		}
		overselling := pl.Overselling
		if overselling == "" {
			overselling = oversellingAllowed
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO reseller_plans (name, max_customers, max_domains, max_databases, max_email_accounts, disk_quota_mb, traffic_quota_mb, overselling)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, pl.Name, pl.MaxCustomers, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts, pl.DiskQuotaMB, pl.TrafficQuotaMB, overselling)
		if err != nil {
			writeServerError(w, err)
			return
		}
		id64, err := result.LastInsertId()
		if err != nil {
			writeServerError(w, err)
			return
		}
		id := int(id64)
		if err := saveRightsSet(ctx, tx, rightsOwnerResellerPlan, id, set); err != nil {
			writeServerError(w, err)
			return
		}
		if err := saveSettingCeilings(ctx, tx, rightsOwnerResellerPlan, id, ceilings); err != nil {
			writeServerError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "reseller_plan.create:"+pl.Name, "reseller_plan", id, map[string]any{"pool": pl.pool(), "rights": set.payload()})
		writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "id": id})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPost})
	}
}

func (p *Panel) handleResellerPlanByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if p.requireRightsAdministrator(w, r) == nil {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, resellerPlansPath+"/")
	idStr, action, _ := strings.Cut(rest, "/")
	id, ok := strictPositiveID(idStr)
	if !ok || (action != "" && action != "preview") {
		writeClientError(w, http.StatusNotFound, "reseller plan not found")
		return
	}
	ctx := r.Context()
	db := p.db.GetDB()
	plan, err := loadResellerPlan(ctx, db, id)
	if errors.Is(err, errNotFound) {
		writeClientError(w, http.StatusNotFound, "reseller plan not found")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		payload := plan.Set.payload()
		ceilings, err := loadSettingCeilings(ctx, db, rightsOwnerResellerPlan, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		c := ceilingsPayload(ceilings)
		var overselling string
		_ = db.QueryRowContext(ctx, `SELECT overselling FROM reseller_plans WHERE id = ?`, id).Scan(&overselling)
		writeRightsJSON(w, http.StatusOK, resellerPlanPayload{ID: plan.ID, Name: plan.Name, MaxCustomers: plan.Pool.MaxCustomers,
			MaxDomains: plan.Pool.Domains, MaxDatabases: plan.Pool.Databases, MaxEmailAccounts: plan.Pool.Mailboxes,
			DiskQuotaMB: plan.Pool.DiskMB, TrafficQuotaMB: plan.Pool.TrafficMB, Rights: &payload, Ceilings: &c, Overselling: overselling})
	case (action == "" && r.Method == http.MethodPut) || (action == "preview" && r.Method == http.MethodPost):
		pl, ok := decodeResellerPlan(w, r)
		if !ok {
			return
		}
		after := plan.Set
		if pl.Rights != nil {
			if after, err = pl.Rights.set(); err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return
			}
		}
		if !p.planRightsWithinGrantor(w, r, currentCaller(r), plan.Set, after) {
			return
		}
		ceilingsBefore, err := loadSettingCeilings(ctx, db, rightsOwnerResellerPlan, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		ceilingsAfter, ok := p.planCeilingsWithinGrantor(w, r, currentCaller(r), pl.Ceilings, ceilingsBefore)
		if !ok {
			return
		}
		resellers, err := p.resellersOnPlan(r, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		admin, err := p.adminEffective(ctx)
		if err != nil {
			writeServerError(w, err)
			return
		}
		candidates, unknown := p.previewCandidates(ctx, plan.Set, after)
		diff := diffEffective(admin.withOwn(plan.Set), admin.withOwn(after), candidates)
		preview := rightsPreview{From: "plan:" + strconv.Itoa(id), To: "plan:" + strconv.Itoa(id),
			Added: diff.Added, Removed: diff.Removed, InUse: []inUseLine{}, OverLimit: []string{}, Outside: []string{},
			InstalledUnknown: unknown, LimitsFrom: plan.Pool, LimitsTo: pl.pool(), Subscriptions: len(resellers)}
		tree := []int{}
		for _, resellerID := range resellers {
			ids, err := treeSubscriptionIDs(ctx, db, resellerID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			tree = append(tree, ids...)
			usage, err := resellerTreeUsage(ctx, db, resellerID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			for _, line := range poolOverUsage(pl.pool(), usage) {
				preview.OverLimit = append(preview.OverLimit, fmt.Sprintf("reseller #%d %s", resellerID, line))
			}
		}
		if preview.InUse, err = p.removedItemsInUse(ctx, tree, diff.Removed); err != nil {
			writeServerError(w, err)
			return
		}
		preview.Message = fmt.Sprintf("%d reseller(s) are bound to this plan and follow the change, with their plans and customers. "+
			"Items taken away keep running where they are used; they cannot be chosen again. Confirm to save.", len(resellers))
		preview.CeilingsLowered = loweredCeilings(ceilingsBefore, ceilingsAfter)
		if len(preview.CeilingsLowered) > 0 {
			preview.Message += " Lowered limits of site settings reach every site of these resellers at its next write; a site above one runs at the new limit."
		}
		if action == "preview" {
			writeRightsJSON(w, http.StatusOK, map[string]any{"preview": preview})
			return
		}
		if len(preview.Removed) > 0 && !pl.ConfirmNarrowing {
			writeRightsPreviewRefusal(w, errCodePlanNarrowingConfirm, preview)
			return
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			writeServerError(w, err)
			return
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `
			UPDATE reseller_plans SET name = ?, max_customers = ?, max_domains = ?, max_databases = ?,
			       max_email_accounts = ?, disk_quota_mb = ?, traffic_quota_mb = ?, updated_at = datetime('now') WHERE id = ?`,
			pl.Name, pl.MaxCustomers, pl.MaxDomains, pl.MaxDatabases, pl.MaxEmailAccounts, pl.DiskQuotaMB, pl.TrafficQuotaMB, id); err != nil {
			writeServerError(w, err)
			return
		}
		if pl.Overselling != "" {
			if _, err := tx.ExecContext(ctx, `UPDATE reseller_plans SET overselling = ? WHERE id = ?`, pl.Overselling, id); err != nil {
				writeServerError(w, err)
				return
			}
		}
		if err := saveSettingCeilings(ctx, tx, rightsOwnerResellerPlan, id, ceilingsAfter); err != nil {
			writeServerError(w, err)
			return
		}
		if err := saveRightsSet(ctx, tx, rightsOwnerResellerPlan, id, after); err != nil {
			writeServerError(w, err)
			return
		}
		pool := pl.pool()
		for _, resellerID := range resellers {
			if err := writeResellerPool(ctx, tx, resellerID, &pool); err != nil {
				writeServerError(w, err)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			writeServerError(w, err)
			return
		}
		// alpha.128 (G5): database users follow a changed connection limit.
		p.reapplyDBConnectionLimitsAfter(ctx, rightsOwnerResellerPlan, id, ceilingsBefore, ceilingsAfter)
		// alpha.128 (integration): outgoing mail follows the mail keys.
		p.requestOutgoingApply()
		p.auditDetailed(r, "reseller_plan.update:"+pl.Name, "reseller_plan", id, map[string]any{
			"pool": pool, "rights_added": preview.Added, "rights_removed": preview.Removed, "resellers": len(resellers),
		})
		writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "over_limit": preview.OverLimit})
	case action == "" && r.Method == http.MethodDelete:
		result, err := db.ExecContext(ctx, `
			DELETE FROM reseller_plans WHERE id = ?
			  AND NOT EXISTS (SELECT 1 FROM reseller_plan_bindings WHERE reseller_plan_id = reseller_plans.id)`, id)
		if err != nil {
			writeServerError(w, err)
			return
		}
		if n, err := result.RowsAffected(); err != nil || n == 0 {
			writeCodedError(w, http.StatusConflict, errCodeResellerPlanInUse,
				"resellers are bound to this plan; move them to another plan or make them Custom first. Nothing was deleted.", "")
			return
		}
		p.audit(r, "reseller_plan.delete:"+plan.Name, "reseller_plan", id)
		writeRightsJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPost})
	}
}

// resellersOnPlan lists the resellers bound to a reseller plan.
func (p *Panel) resellersOnPlan(r *http.Request, planID int) ([]int, error) {
	rows, err := p.db.GetDB().QueryContext(r.Context(), `SELECT reseller_id FROM reseller_plan_bindings WHERE reseller_plan_id = ? ORDER BY reseller_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// handleRightsRoutes serves /api/v1/rights/catalog and /api/v1/rights/notices.
func (p *Panel) handleRightsRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	rest := strings.TrimPrefix(r.URL.Path, rightsPathPrefix)
	switch {
	case rest == "catalog":
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.handleRightsCatalog(w, r)
	case rest == "offered-permissions":
		p.handleOfferedOptInPermissions(w, r)
	case rest == "setting-limits":
		p.handleSettingLimits(w, r)
	case rest == "password-level":
		p.handlePasswordLevel(w, r)
	case rest == "notices":
		if r.Method != http.MethodGet {
			rejectRouteMethod(w, []string{http.MethodGet})
			return
		}
		p.handleRightsNotices(w, r)
	case strings.HasPrefix(rest, "notices/") && strings.HasSuffix(rest, "/acknowledge"):
		if r.Method != http.MethodPost {
			rejectRouteMethod(w, []string{http.MethodPost})
			return
		}
		id, ok := strictPositiveID(strings.TrimSuffix(strings.TrimPrefix(rest, "notices/"), "/acknowledge"))
		if !ok {
			writeClientError(w, http.StatusNotFound, "notice not found")
			return
		}
		p.acknowledgeRightsNotice(w, r, id)
	default:
		writeClientError(w, http.StatusNotFound, "not found")
	}
}

// handleRightsCatalog: the permissions and software the caller may choose
// (a subscription's effective set) or hand out (a reseller's field, the
// administrator's offer), over what is installed.
func (p *Panel) handleRightsCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := currentCaller(r)
	var e *effectiveRights
	var err error
	scope := "administrator"
	subID, scoped, handled := p.listingSubscription(w, r)
	if handled {
		return
	}
	switch {
	case scoped && r.URL.Query().Get("subscription_id") != "":
		scope = "subscription"
		e, err = p.effectiveSubscriptionRights(ctx, subID)
	case c.hasAccountRole(roleAdmin):
		e, err = p.adminEffective(ctx)
	case c.hasAccountRole(roleReseller):
		scope = "reseller"
		e, err = p.resellerEffective(ctx, c.ID)
	case scoped:
		scope = "subscription"
		e, err = p.effectiveSubscriptionRights(ctx, subID)
	default:
		writeCodedError(w, http.StatusConflict, errCodeChooseSubscription,
			"choose a subscription (subscription_id) to see what it may use", "")
		return
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	candidates, unknown := p.previewCandidates(ctx, e.Own.Set)
	view := e.view(candidates)
	view.InstalledUnknown = unknown
	writeRightsJSON(w, http.StatusOK, map[string]any{"scope": scope, "rights": view})
}

type rightsNoticeView struct {
	ID        int    `json:"id"`
	Kind      string `json:"kind"`
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	Subject   string `json:"subject"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"created_at"`
}

func (p *Panel) handleRightsNotices(w http.ResponseWriter, r *http.Request) {
	c := currentCaller(r)
	query := `
		SELECT n.id, n.kind, n.user_id, COALESCE(u.username, ''), n.subject, n.detail, n.created_at
		FROM rights_notices n LEFT JOIN users u ON u.id = n.user_id
		WHERE n.acknowledged_at IS NULL`
	args := []any{}
	switch {
	case c.hasAccountRole(roleAdmin):
	case c.hasAccountRole(roleReseller):
		query += ` AND n.user_id = ? AND n.kind IN ('reseller_pool_80', 'reseller_pool_100', '` + noticeSiteLimitClamped + `')`
		args = append(args, c.ID)
	case c != nil && !c.isAdditionalUser():
		// alpha.128: a customer reads its own notices of lowered site limits.
		query += ` AND n.user_id = ? AND n.kind = '` + noticeSiteLimitClamped + `'`
		args = append(args, c.ID)
	default:
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator or reseller access required", "")
		return
	}
	query += ` ORDER BY n.id`
	rows, err := p.db.GetDB().QueryContext(r.Context(), query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer rows.Close()
	notices := []rightsNoticeView{}
	for rows.Next() {
		var n rightsNoticeView
		if err := rows.Scan(&n.ID, &n.Kind, &n.UserID, &n.Username, &n.Subject, &n.Detail, &n.CreatedAt); err != nil {
			writeServerError(w, err)
			return
		}
		notices = append(notices, n)
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"notices": notices})
}

func (p *Panel) acknowledgeRightsNotice(w http.ResponseWriter, r *http.Request, id int) {
	c := currentCaller(r)
	query := `UPDATE rights_notices SET acknowledged_at = datetime('now') WHERE id = ? AND acknowledged_at IS NULL`
	args := []any{id}
	switch {
	case c.hasAccountRole(roleAdmin):
	case c.hasAccountRole(roleReseller):
		query += ` AND user_id = ? AND kind IN ('reseller_pool_80', 'reseller_pool_100', '` + noticeSiteLimitClamped + `')`
		args = append(args, c.ID)
	case c != nil && !c.isAdditionalUser():
		query += ` AND user_id = ? AND kind = '` + noticeSiteLimitClamped + `'`
		args = append(args, c.ID)
	default:
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "administrator or reseller access required", "")
		return
	}
	result, err := p.db.GetDB().ExecContext(r.Context(), query, args...)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		writeClientError(w, http.StatusNotFound, "notice not found")
		return
	}
	p.audit(r, "rights.notice_acknowledge", "rights_notice", id)
	writeRightsJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// handleOfferedOptInPermissions: the administrator's offer of the opt-in
// permissions (alpha.128; today nginx_directives). GET lists each with its
// state; PUT {"permission": id, "offered": bool} changes one. Withdrawing
// removes nothing: what accounts already set keeps working, only new
// changes are refused (P5).
// Yöneticinin isteğe bağlı izin sunumu; geri çekmek çalışanı durdurmaz.
func (p *Panel) handleOfferedOptInPermissions(w http.ResponseWriter, r *http.Request) {
	c := currentCaller(r)
	if !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator offers permissions", "")
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var req struct {
			Permission string `json:"permission"`
			Offered    bool   `json:"offered"`
		}
		if err := decodeStrictJSON(w, r, &req); err != nil || !optInPermissions[req.Permission] {
			writeClientError(w, http.StatusBadRequest, "permission must be one of the opt-in permissions")
			return
		}
		offered, err := p.loadOfferedOptInPermissions(ctx)
		if err != nil {
			writeServerError(w, err)
			return
		}
		offered[req.Permission] = req.Offered
		if err := p.saveOfferedOptInPermissions(ctx, offered); err != nil {
			writeServerError(w, err)
			return
		}
		p.auditDetailed(r, "rights.offer_permission:"+req.Permission, "panel_settings", 0,
			map[string]any{"permission": req.Permission, "offered": req.Offered})
	default:
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut})
		return
	}
	offered, err := p.loadOfferedOptInPermissions(ctx)
	if err != nil {
		writeServerError(w, err)
		return
	}
	list := []map[string]any{}
	ids := make([]string, 0, len(optInPermissions))
	for id := range optInPermissions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		list = append(list, map[string]any{"permission": id, "name": rightPermissionNames[id], "offered": offered[id]})
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{"permissions": list})
}
