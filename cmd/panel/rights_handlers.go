package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/alicelik/celikpanel/internal/core"
)

// Rights APIs (D-042, docs/PLANS-AND-RIGHTS.md section 6). Every change is
// shown first (a preview, or a 409 carrying the preview when "confirm" is
// missing), then applied in one transaction and audited (P7). The
// administrator acts on everyone; a reseller on its own customers'
// subscriptions only, and never grants outside its own set.
//
//	GET   /api/v1/subscriptions/{id}/rights          state, own set, effective set, lifecycle
//	PUT   /api/v1/subscriptions/{id}/rights          {state: unlimited|plan|custom, plan_id, rights, quantities, confirm}
//	PATCH /api/v1/subscriptions/{id}/rights          {grant, revoke, quantities, confirm}: becomes Custom
//	POST  /api/v1/subscriptions/{id}/rights/preview  either body: what would change
//	GET/PUT/PATCH /api/v1/users/{id}/rights, POST .../rights/preview   a reseller's state (administrator)
//
// Hak API'leri: her değişiklik önce gösterilir, sonra tek işlemde uygulanır
// ve denetime yazılır. Bayi yalnız kendi müşterilerine, kendi kümesi içinde.

const (
	errCodeRightsConfirm        = "RIGHTS_CHANGE_CONFIRM"
	errCodeRightsLeavesPlan     = "RIGHTS_LEAVES_PLAN"
	errCodeRightsOutside        = "RIGHTS_OUTSIDE_GRANTOR"
	errCodeRightsUnknownItem    = "RIGHTS_UNKNOWN_ITEM"
	errCodeRightsInstalledUnk   = "RIGHTS_INSTALLED_UNKNOWN"
	errCodeRightsAdminWorkspace = "RIGHTS_ADMIN_WORKSPACE"
	errCodePlanNarrowingConfirm = "PLAN_NARROWING_CONFIRM"
	errCodeBuiltinPlanFixed     = "BUILTIN_PLAN_FIXED"
	errCodePlanNameReserved     = "PLAN_NAME_RESERVED"
	errCodeResellerPlanInUse    = "RESELLER_PLAN_IN_USE"

	builtinUnlimitedName         = "Unlimited"
	builtinUnlimitedResellerName = "Unlimited reseller"
)

type rightsChangeRequest struct {
	State           string            `json:"state,omitempty"`
	PlanID          int               `json:"plan_id,omitempty"`
	Rights          *rightsPayload    `json:"rights,omitempty"`
	Quantities      *rightsQuantities `json:"quantities,omitempty"`
	Grant           []string          `json:"grant,omitempty"`
	Revoke          []string          `json:"revoke,omitempty"`
	Confirm         bool              `json:"confirm,omitempty"`
	AcceptOverUsage bool              `json:"accept_over_usage,omitempty"`
	// alpha.128: a Custom set's own limits of site settings (nil keeps
	// them; leaving a plan starts from the plan's), a Custom subscription's
	// overuse policy and notice levels, and a Custom reseller's overselling
	// choice ("" keeps it).
	Ceilings             *ceilingsPayload `json:"ceilings,omitempty"`
	OverusePolicy        string           `json:"overuse_policy,omitempty"`
	NotifyPercentDisk    int              `json:"notify_percent_disk,omitempty"`
	NotifyPercentTraffic int              `json:"notify_percent_traffic,omitempty"`
	Overselling          string           `json:"overselling,omitempty"`
}

// customExtrasValid checks the alpha.128 fields of a rights change: they
// belong to a Custom state only.
func (req rightsChangeRequest) customExtrasValid(state string) string {
	extras := req.Ceilings != nil || req.OverusePolicy != "" || req.NotifyPercentDisk != 0 || req.NotifyPercentTraffic != 0 || req.Overselling != ""
	if extras && state != rightsStateCustom {
		return "limits of site settings, the overuse policy and the overselling choice are set on a plan or on a Custom set; choose Custom to set them here"
	}
	if req.OverusePolicy != "" && req.OverusePolicy != overusePolicyClose && req.OverusePolicy != overusePolicyNotify {
		return "overuse_policy must be close or notify"
	}
	for _, pct := range []int{req.NotifyPercentDisk, req.NotifyPercentTraffic} {
		if pct != 0 && (pct < 50 || pct > 95) {
			return "a notice level must be between 50 and 95 percent"
		}
	}
	if !validOverselling(req.Overselling) {
		return "overselling must be allowed or not_allowed"
	}
	return ""
}

// rightsPreview is what a change would do.
type rightsPreview struct {
	From             string      `json:"from"`
	FromPlanName     string      `json:"from_plan_name,omitempty"`
	To               string      `json:"to"`
	ToPlanName       string      `json:"to_plan_name,omitempty"`
	LeavesPlan       bool        `json:"leaves_plan"`
	Added            []string    `json:"added"`
	Removed          []string    `json:"removed"`
	InUse            []inUseLine `json:"in_use"`
	LimitsFrom       any         `json:"limits_from"`
	LimitsTo         any         `json:"limits_to"`
	OverLimit        []string    `json:"over_limit"`
	Outside          []string    `json:"outside_grantor"`
	InstalledUnknown bool        `json:"installed_unknown,omitempty"`
	Subscriptions    int         `json:"subscriptions,omitempty"`
	// alpha.128 (setting_limits.go): ceilings an edit lowers, and the sites
	// of the plan whose own value is above a new ceiling (they will run at
	// the ceiling). SitesUnread: the sites' values could not be read, so
	// the count is not known.
	CeilingsLowered []string `json:"ceilings_lowered,omitempty"`
	SitesAbove      []string `json:"sites_above,omitempty"`
	SitesAboveCount int      `json:"sites_above_count,omitempty"`
	SitesUnread     bool     `json:"sites_unread,omitempty"`
	Message         string   `json:"message"`
	// clamped (alpha.128): the sites above a lowered limit, for their notices.
	clamped         []clampedSite
	planned         *plannedRights
	resellerPlanned *plannedReseller
}

type plannedRights struct {
	state  string
	plan   *loadedPlan
	set    rightsSet
	limits planLimits
}

type plannedReseller struct {
	state string
	plan  *resellerPlan
	set   rightsSet
	pool  resellerPool
}

func writeRightsJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode rights answer: %v", err)
	}
}

func writeRightsPreviewRefusal(w http.ResponseWriter, code string, preview rightsPreview) {
	writeRightsJSON(w, http.StatusConflict, map[string]any{
		"error": preview.Message, "code": code, "preview": preview,
	})
}

// applyGrantRevoke starts from base and applies the item changes. Taking an
// item out of an "everything offered" part lists that part first, over the
// candidates; unknown candidates refuse (the caller writes the refusal).
func applyGrantRevoke(base rightsSet, grant, revoke []string, candidates []string, installedUnknown bool) (rightsSet, error) {
	out := base.clone()
	for _, item := range append(append([]string{}, grant...), revoke...) {
		if !validRightsItem(strings.TrimSpace(item)) {
			return out, errUnknownRightsItem
		}
	}
	for _, item := range revoke {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(item, permissionItemHead) && out.AllPermissions {
			out.AllPermissions = false
			for _, id := range rightPermissionIDs {
				// alpha.128: an opt-in permission is in "everything offered"
				// only when the administrator offers it; the caller adds it
				// back then (keepOfferedOptIn).
				if optInPermissions[id] {
					continue
				}
				out.Items[permissionItemHead+id] = true
			}
		}
		if !strings.HasPrefix(item, permissionItemHead) && out.AllSoftware {
			if installedUnknown {
				return out, errRightsInstalledUnknown
			}
			out.AllSoftware = false
			for _, id := range candidates {
				if customerOffering(id) {
					out.Items[id] = true
				}
			}
		}
		delete(out.Items, item)
		// Revoking a whole component takes its versions too.
		if strings.HasPrefix(item, "component:") && strings.Count(item, ":") == 1 {
			for existing := range out.Items {
				if strings.HasPrefix(existing, item+":") {
					delete(out.Items, existing)
				}
			}
		}
	}
	for _, item := range grant {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(item, permissionItemHead) && out.AllPermissions {
			continue
		}
		if !strings.HasPrefix(item, permissionItemHead) && (out.AllSoftware || !customerOffering(item)) {
			// A server component is not software an account chooses.
			continue
		}
		out.Items[item] = true
	}
	return out, nil
}

var errRightsInstalledUnknown = errors.New("installed software unknown")

// newlyGranted: items of after that before did not hold.
func newlyGranted(before, after rightsSet) rightsSet {
	out := newRightsSet(after.AllPermissions && !before.AllPermissions, after.AllSoftware && !before.AllSoftware)
	for item := range after.Items {
		if !before.Items[item] {
			out.Items[item] = true
		}
	}
	// A flag never counts as outside (it means "all the grantor has").
	out.AllPermissions, out.AllSoftware = false, false
	return out
}

// ---- subscriptions ----

func parseSubscriptionRightsPath(path string) (int, bool, bool) {
	rest, ok := strings.CutPrefix(path, "/api/v1/subscriptions/")
	if !ok {
		return 0, false, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[1] != "rights" || len(parts) > 3 {
		return 0, false, false
	}
	id, valid := strictPositiveID(parts[0])
	if !valid {
		return 0, false, false
	}
	if len(parts) == 3 {
		if parts[2] != "preview" {
			return 0, false, false
		}
		return id, true, true
	}
	return id, false, true
}

// subscriptionRightsManager: the administrator, or the reseller whose
// customer owns the subscription. owner is the subscription's owner.
func (p *Panel) subscriptionRightsManager(ctx context.Context, c *Caller, subID int) (owner *core.User, allowed bool, err error) {
	var ownerID int
	if err := p.db.GetDB().QueryRowContext(ctx, `SELECT owner_id FROM subscriptions WHERE id = ?`, subID).Scan(&ownerID); err != nil {
		return nil, false, err
	}
	owner, err = p.users.GetByID(ctx, ownerID)
	if err != nil {
		return nil, false, err
	}
	if c.hasAccountRole(roleAdmin) {
		return owner, true, nil
	}
	if c.hasAccountRole(roleReseller) && owner.Role == roleCustomer && owner.ParentID != nil && *owner.ParentID == c.ID {
		return owner, true, nil
	}
	return owner, false, nil
}

func (p *Panel) handleSubscriptionRights(w http.ResponseWriter, r *http.Request, subID int, preview bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	c := currentCaller(r)
	ctx := r.Context()
	if err := p.canAccessSubscription(ctx, c, subID); err != nil {
		writeClientError(w, http.StatusNotFound, "subscription not found")
		return
	}
	if !preview && r.Method == http.MethodGet {
		p.writeSubscriptionRightsView(w, r, subID)
		return
	}
	if (preview && r.Method != http.MethodPost) || (!preview && r.Method != http.MethodPut && r.Method != http.MethodPatch) {
		allowed := []string{http.MethodGet, http.MethodPut, http.MethodPatch}
		if preview {
			allowed = []string{http.MethodPost}
		}
		rejectRouteMethod(w, allowed)
		return
	}
	owner, allowed, err := p.subscriptionRightsManager(ctx, c, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	if !allowed {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly,
			"only the administrator, or the reseller of this customer, changes a subscription's rights", "")
		return
	}
	var req rightsChangeRequest
	if err := decodeStrictJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid rights request")
		return
	}
	if r.Method == http.MethodPut && req.State == "" {
		writeClientError(w, http.StatusBadRequest, "state is required (unlimited, plan or custom)")
		return
	}
	if r.Method == http.MethodPatch && req.State != "" {
		writeClientError(w, http.StatusBadRequest, "a PATCH changes items; use PUT to change the state")
		return
	}
	plan, ok := p.planSubscriptionChange(w, r, c, owner, subID, req)
	if !ok {
		return
	}
	if preview {
		writeRightsJSON(w, http.StatusOK, map[string]any{"preview": plan})
		return
	}
	if !req.Confirm {
		code := errCodeRightsConfirm
		if plan.LeavesPlan {
			code = errCodeRightsLeavesPlan
		}
		writeRightsPreviewRefusal(w, code, plan)
		return
	}
	if len(plan.OverLimit) > 0 && !req.AcceptOverUsage {
		writeCodedErrorDetails(w, http.StatusConflict, errCodePlanBelowUsage,
			"these numbers are below what the subscription uses now. Nothing was changed. To apply them anyway, confirm: nothing is deleted, and new items of these kinds are refused until usage is under the limit.",
			"", plan.OverLimit)
		return
	}
	planned := plan.planned
	if problem := req.customExtrasValid(planned.state); problem != "" || req.Overselling != "" {
		if problem == "" {
			problem = "overselling is a reseller's choice, not a subscription's"
		}
		writeClientError(w, http.StatusBadRequest, problem)
		return
	}
	var ceilings map[string]int
	if req.Ceilings != nil {
		var ok bool
		if ceilings, ok = p.planCeilingsWithinGrantor(w, r, c, req.Ceilings, nil); !ok {
			return
		}
	}
	// alpha.128 (G17): a reseller that does not oversell hands out only
	// what its pool still holds.
	nextLimits := planned.limits
	if planned.state == rightsStatePlan && planned.plan != nil {
		nextLimits = planned.plan.Limits
	}
	if planned.state != rightsStateUnlimited && owner != nil && p.refuseOverselling(w, ctx, owner.ID, subID, nextLimits) {
		return
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	switch planned.state {
	case rightsStateUnlimited:
		err = setSubscriptionUnlimited(ctx, tx, subID)
	case rightsStatePlan:
		err = bindSubscriptionPlan(ctx, tx, subID, *planned.plan)
	default:
		err = setSubscriptionCustom(ctx, tx, subID, planned.set, planned.limits)
		if err == nil && ceilings != nil {
			err = saveSettingCeilings(ctx, tx, rightsOwnerSubscription, subID, ceilings)
		}
		if err == nil {
			err = setSubscriptionOwnPolicy(ctx, tx, subID, req)
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	// alpha.128 (G5): the subscription's limits may have changed with its
	// state or its own ceilings; its database users follow.
	p.reapplyDBConnectionLimits([]int{subID})
	// alpha.128 (integration): outgoing mail follows the mail keys.
	p.requestOutgoingApply()
	p.auditDetailed(r, "subscription.rights:"+plan.To, "subscription", subID, map[string]any{
		"from": plan.From, "to": plan.To, "added": plan.Added, "removed": plan.Removed,
		"limits_to": plan.LimitsTo, "over_limit": plan.OverLimit,
	})
	p.writeSubscriptionRightsView(w, r, subID)
}

// planSubscriptionChange computes the new state and its preview, writing
// the refusal when the request cannot be applied.
func (p *Panel) planSubscriptionChange(w http.ResponseWriter, r *http.Request, c *Caller, owner *core.User, subID int, req rightsChangeRequest) (rightsPreview, bool) {
	ctx := r.Context()
	db := p.db.GetDB()
	current, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	if current.Own.AdminWorkspace {
		writeCodedError(w, http.StatusConflict, errCodeRightsAdminWorkspace,
			"the administrator's own workspace has no limits and no rights set; nothing was changed", "")
		return rightsPreview{}, false
	}
	if !req.Quantities.valid() {
		writeClientError(w, http.StatusBadRequest, "quantities must be 0 (unlimited) or more")
		return rightsPreview{}, false
	}
	limitsNow, err := loadSubscriptionLimits(ctx, db, subID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	planned := &plannedRights{}
	preview := rightsPreview{From: current.Own.stateLabel(), FromPlanName: current.Own.PlanName,
		Added: []string{}, Removed: []string{}, InUse: []inUseLine{}, OverLimit: []string{}, Outside: []string{}}
	state := req.State
	if state == "" {
		state = rightsStateCustom
	}
	var candidateSets []rightsSet
	switch state {
	case rightsStateUnlimited:
		planned.state, planned.set, planned.limits = rightsStateUnlimited, allOfferedRightsSet(), planLimits{}
	case rightsStatePlan:
		plan, err := loadPlan(ctx, db, req.PlanID)
		if errors.Is(err, errNotFound) || (err == nil && !planAssignableTo(c, plan, owner)) {
			writePlanNotAvailable(w)
			return rightsPreview{}, false
		}
		if err != nil {
			writeServerError(w, err)
			return rightsPreview{}, false
		}
		set, found, err := loadRightsSet(ctx, db, rightsOwnerPlan, plan.ID)
		if err != nil {
			writeServerError(w, err)
			return rightsPreview{}, false
		}
		if !found {
			set = allOfferedRightsSet()
		}
		planned.state, planned.plan, planned.set, planned.limits = rightsStatePlan, &plan, set, plan.Limits
		preview.ToPlanName = plan.Name
	case rightsStateCustom:
		planned.state = rightsStateCustom
		planned.limits = req.Quantities.applyLimits(limitsNow)
		if current.Own.State == rightsStateUnlimited && req.Quantities == nil {
			planned.limits = planLimits{}
		}
		if req.Rights != nil {
			set, err := req.Rights.set()
			if err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return rightsPreview{}, false
			}
			planned.set = set
		} else {
			candidates, unknown := p.previewCandidates(ctx, current.Own.Set)
			if current.ResellerID != 0 {
				// Taking an item out of "everything offered" never names an
				// app closed to resellers' customers by its licence.
				candidates = withoutLicenceClosed(candidates)
			}
			set, err := applyGrantRevoke(current.Own.Set, req.Grant, req.Revoke, candidates, unknown)
			set = keepOfferedOptIn(current.Own.Set, set, current.admin, req.Revoke)
			if errors.Is(err, errUnknownRightsItem) {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, "an item to grant or revoke is not a known permission or offering id", "")
				return rightsPreview{}, false
			}
			if errors.Is(err, errRightsInstalledUnknown) {
				writeCodedError(w, http.StatusServiceUnavailable, errCodeRightsInstalledUnk,
					"the installed software could not be read now, so an item cannot be taken out of \"everything offered\". Nothing was changed. Try again in a moment; if it persists, the administrator checks that the CelikPanel agent is running.", "")
				return rightsPreview{}, false
			}
			planned.set = set
		}
		// A grantor hands out only what it has (P2).
		var resellerLayer *rightsState
		if c.hasAccountRole(roleReseller) {
			state, err := resellerRightsState(ctx, db, c.ID)
			if err != nil {
				writeServerError(w, err)
				return rightsPreview{}, false
			}
			resellerLayer = &state
		}
		preview.Outside = grantorOutside(current.admin, resellerLayer, newlyGranted(current.Own.Set, planned.set))
		if len(preview.Outside) > 0 {
			who := "the administrator does not offer them on this server"
			if resellerLayer != nil {
				who = "they are not in your own rights; ask the administrator to include them first"
			}
			writeCodedErrorDetails(w, http.StatusConflict, errCodeRightsOutside,
				"Nothing was changed: these items cannot be given because "+who+".", "", preview.Outside)
			return rightsPreview{}, false
		}
	default:
		writeClientError(w, http.StatusBadRequest, "state must be unlimited, plan or custom")
		return rightsPreview{}, false
	}
	candidateSets = append(candidateSets, current.Own.Set, planned.set)
	candidates, unknown := p.previewCandidates(ctx, candidateSets...)
	after := current.withOwn(planned.set)
	diff := diffEffective(current, after, candidates)
	preview.Added, preview.Removed, preview.InstalledUnknown = diff.Added, diff.Removed, unknown
	preview.To = planned.state
	if planned.state == rightsStatePlan {
		preview.To = "plan:" + strconv.Itoa(planned.plan.ID)
	}
	preview.LeavesPlan = current.Own.State == rightsStatePlan && planned.state != rightsStatePlan
	inUse, err := p.removedItemsInUse(ctx, []int{subID}, diff.Removed)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	preview.InUse = inUse
	preview.LimitsFrom, preview.LimitsTo = limitsNow, planned.limits
	usage, err := p.subscriptionUsageFor(ctx, subID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	projected := *usage
	projected.DomainsLimit, projected.DatabasesLimit, projected.MailLimit = planned.limits.Domains, planned.limits.Databases, planned.limits.Mailboxes
	projected.DiskLimitBytes = 0
	if planned.limits.DiskMB > 0 {
		projected.DiskLimitBytes = int64(planned.limits.DiskMB) * 1024 * 1024
	}
	preview.OverLimit = projected.overLimits()
	switch {
	case preview.LeavesPlan:
		preview.Message = "With this change the subscription leaves the \"" + current.Own.PlanName +
			"\" plan and becomes Custom. Later changes to the plan will not reach it. What already runs keeps running. Confirm to continue."
	case planned.state == rightsStatePlan:
		preview.Message = "The subscription will be bound to the \"" + planned.plan.Name +
			"\" plan and follow it from now on. What already runs keeps running; items the plan does not include cannot be chosen again. Confirm to continue."
	default:
		preview.Message = "Review what changes, then confirm. What already runs keeps running; only new use of a removed item is refused."
	}
	preview.planned = planned
	return preview, true
}

// subscriptionRightsView is the GET answer.
type subscriptionRightsView struct {
	effectiveRightsView
	Limits    planLimits            `json:"limits"`
	Lifecycle subscriptionLifecycle `json:"lifecycle"`
}

func (p *Panel) writeSubscriptionRightsView(w http.ResponseWriter, r *http.Request, subID int) {
	ctx := r.Context()
	e, err := p.effectiveSubscriptionRights(ctx, subID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	candidates, unknown := p.previewCandidates(ctx, e.Own.Set)
	view := subscriptionRightsView{effectiveRightsView: e.view(candidates)}
	view.InstalledUnknown = unknown
	if view.Limits, err = loadSubscriptionLimits(ctx, p.db.GetDB(), subID); err != nil {
		writeServerError(w, err)
		return
	}
	if e.Own.AdminWorkspace {
		view.Limits = planLimits{}
	}
	if view.Lifecycle, err = readSubscriptionLifecycle(ctx, p.db.GetDB(), subID); err != nil {
		writeServerError(w, err)
		return
	}
	writeRightsJSON(w, http.StatusOK, view)
}

// ---- resellers ----

func (p *Panel) handleResellerRights(w http.ResponseWriter, r *http.Request, c *Caller, target *core.User, preview bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	ctx := r.Context()
	if target.Role == roleCustomer && !preview && r.Method == http.MethodGet {
		// A customer's subscriptions, each with its state.
		rows, err := p.db.GetDB().QueryContext(ctx, `SELECT id, name FROM subscriptions WHERE owner_id = ? ORDER BY id`, target.ID)
		if err != nil {
			writeServerError(w, err)
			return
		}
		type entry struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			State    string `json:"state"`
			PlanName string `json:"plan_name,omitempty"`
		}
		list := []entry{}
		for rows.Next() {
			var e entry
			if rows.Scan(&e.ID, &e.Name) == nil {
				list = append(list, e)
			}
		}
		rows.Close()
		for i := range list {
			state, err := subscriptionRightsState(ctx, p.db.GetDB(), list[i].ID)
			if err != nil {
				writeServerError(w, err)
				return
			}
			list[i].State, list[i].PlanName = state.stateLabel(), state.PlanName
		}
		writeRightsJSON(w, http.StatusOK, map[string]any{"role": roleCustomer, "subscriptions": list})
		return
	}
	if target.Role != roleReseller {
		writeClientError(w, http.StatusBadRequest, "a customer's rights are set per subscription (/api/v1/subscriptions/{id}/rights)")
		return
	}
	if !c.hasAccountRole(roleAdmin) || c.ImpersonatorID > 0 {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator sets a reseller's rights", "")
		return
	}
	if !preview && r.Method == http.MethodGet {
		p.writeResellerRightsView(w, r, target.ID)
		return
	}
	if (preview && r.Method != http.MethodPost) || (!preview && r.Method != http.MethodPut && r.Method != http.MethodPatch) {
		rejectRouteMethod(w, []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodPost})
		return
	}
	var req rightsChangeRequest
	if err := decodeStrictJSON(w, r, &req); err != nil {
		writeClientError(w, http.StatusBadRequest, "invalid rights request")
		return
	}
	if r.Method == http.MethodPut && req.State == "" {
		writeClientError(w, http.StatusBadRequest, "state is required (unlimited, plan or custom)")
		return
	}
	if r.Method == http.MethodPatch && req.State != "" {
		writeClientError(w, http.StatusBadRequest, "a PATCH changes items; use PUT to change the state")
		return
	}
	plan, ok := p.planResellerChange(w, r, target.ID, req)
	if !ok {
		return
	}
	if preview {
		writeRightsJSON(w, http.StatusOK, map[string]any{"preview": plan})
		return
	}
	if !req.Confirm {
		code := errCodeRightsConfirm
		if plan.LeavesPlan {
			code = errCodeRightsLeavesPlan
		}
		writeRightsPreviewRefusal(w, code, plan)
		return
	}
	planned := plan.resellerPlanned
	if problem := req.customExtrasValid(planned.state); problem != "" || req.OverusePolicy != "" || req.NotifyPercentDisk != 0 || req.NotifyPercentTraffic != 0 {
		if problem == "" {
			problem = "the overuse policy belongs to a customer's plan or subscription"
		}
		writeClientError(w, http.StatusBadRequest, problem)
		return
	}
	var ceilings map[string]int
	if req.Ceilings != nil {
		var ok bool
		if ceilings, ok = p.planCeilingsWithinGrantor(w, r, currentCaller(r), req.Ceilings, nil); !ok {
			return
		}
	}
	tx, err := p.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		writeServerError(w, err)
		return
	}
	defer tx.Rollback()
	switch planned.state {
	case rightsStateUnlimited:
		err = setResellerUnlimited(ctx, tx, target.ID)
	case rightsStatePlan:
		err = bindResellerPlan(ctx, tx, target.ID, *planned.plan)
	default:
		err = setResellerCustom(ctx, tx, target.ID, planned.set, planned.pool)
		if err == nil && ceilings != nil {
			err = saveSettingCeilings(ctx, tx, rightsOwnerReseller, target.ID, ceilings)
		}
		if err == nil && req.Overselling != "" {
			_, err = tx.ExecContext(ctx, `UPDATE reseller_pools SET overselling = ? WHERE reseller_id = ?`, req.Overselling, target.ID)
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	// alpha.128 (G5): the reseller's tree follows its limits.
	if tree, err := treeSubscriptionIDs(ctx, p.db.GetDB(), target.ID); err == nil {
		p.reapplyDBConnectionLimits(tree)
	}
	// alpha.128 (integration): outgoing mail follows the mail keys.
	p.requestOutgoingApply()
	p.auditDetailed(r, "reseller.rights:"+target.Username, "user", target.ID, map[string]any{
		"from": plan.From, "to": plan.To, "added": plan.Added, "removed": plan.Removed, "pool_to": plan.LimitsTo,
	})
	p.writeResellerRightsView(w, r, target.ID)
}

func (p *Panel) planResellerChange(w http.ResponseWriter, r *http.Request, resellerID int, req rightsChangeRequest) (rightsPreview, bool) {
	ctx := r.Context()
	db := p.db.GetDB()
	current, err := p.resellerEffective(ctx, resellerID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	if !req.Quantities.valid() {
		writeClientError(w, http.StatusBadRequest, "quantities must be 0 (unlimited) or more")
		return rightsPreview{}, false
	}
	poolNow, _, err := loadResellerPool(ctx, db, resellerID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	planned := &plannedReseller{}
	preview := rightsPreview{From: current.Own.stateLabel(), FromPlanName: current.Own.PlanName,
		Added: []string{}, Removed: []string{}, InUse: []inUseLine{}, OverLimit: []string{}, Outside: []string{}}
	state := req.State
	if state == "" {
		state = rightsStateCustom
	}
	switch state {
	case rightsStateUnlimited:
		planned.state, planned.set = rightsStateUnlimited, allOfferedRightsSet()
	case rightsStatePlan:
		plan, err := loadResellerPlan(ctx, db, req.PlanID)
		if errors.Is(err, errNotFound) {
			writePlanNotAvailable(w)
			return rightsPreview{}, false
		}
		if err != nil {
			writeServerError(w, err)
			return rightsPreview{}, false
		}
		planned.state, planned.plan, planned.set, planned.pool = rightsStatePlan, &plan, plan.Set, plan.Pool
		preview.ToPlanName = plan.Name
	case rightsStateCustom:
		planned.state = rightsStateCustom
		planned.pool = req.Quantities.applyPool(poolNow)
		if req.Rights != nil {
			set, err := req.Rights.set()
			if err != nil {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, err.Error(), "")
				return rightsPreview{}, false
			}
			planned.set = set
		} else {
			candidates, unknown := p.previewCandidates(ctx, current.Own.Set)
			// A reseller's set never names an app closed to resellers by its
			// licence, also when an item is taken out of "everything offered".
			candidates = withoutLicenceClosed(candidates)
			set, err := applyGrantRevoke(current.Own.Set, req.Grant, req.Revoke, candidates, unknown)
			set = keepOfferedOptIn(current.Own.Set, set, current.admin, req.Revoke)
			if errors.Is(err, errUnknownRightsItem) {
				writeCodedError(w, http.StatusBadRequest, errCodeRightsUnknownItem, "an item to grant or revoke is not a known permission or offering id", "")
				return rightsPreview{}, false
			}
			if errors.Is(err, errRightsInstalledUnknown) {
				writeCodedError(w, http.StatusServiceUnavailable, errCodeRightsInstalledUnk,
					"the installed software could not be read now, so an item cannot be taken out of \"everything offered\". Nothing was changed. Try again in a moment.", "")
				return rightsPreview{}, false
			}
			planned.set = set
		}
		preview.Outside = grantorOutside(current.admin, nil, newlyGranted(current.Own.Set, planned.set))
		if len(preview.Outside) > 0 {
			writeCodedErrorDetails(w, http.StatusConflict, errCodeRightsOutside,
				"Nothing was changed: these items are not offered on this server; offer them first (Users, Offered).", "", preview.Outside)
			return rightsPreview{}, false
		}
	default:
		writeClientError(w, http.StatusBadRequest, "state must be unlimited, plan or custom")
		return rightsPreview{}, false
	}
	candidates, unknown := p.previewCandidates(ctx, current.Own.Set, planned.set)
	diff := diffEffective(current, current.withOwn(planned.set), candidates)
	preview.Added, preview.Removed, preview.InstalledUnknown = diff.Added, diff.Removed, unknown
	preview.To = planned.state
	if planned.state == rightsStatePlan {
		preview.To = "plan:" + strconv.Itoa(planned.plan.ID)
	}
	preview.LeavesPlan = current.Own.State == rightsStatePlan && planned.state != rightsStatePlan
	tree, err := treeSubscriptionIDs(ctx, db, resellerID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	preview.Subscriptions = len(tree)
	if preview.InUse, err = p.removedItemsInUse(ctx, tree, diff.Removed); err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	usage, err := resellerTreeUsage(ctx, db, resellerID)
	if err != nil {
		writeServerError(w, err)
		return rightsPreview{}, false
	}
	preview.LimitsFrom, preview.LimitsTo = poolNow, planned.pool
	preview.OverLimit = poolOverUsage(planned.pool, usage)
	preview.Message = "Review what changes for this reseller, its plans and its customers, then confirm. What already runs keeps running; removed items cannot be chosen again by the reseller or its customers."
	if preview.LeavesPlan {
		preview.Message = "With this change the reseller leaves the \"" + current.Own.PlanName + "\" reseller plan and becomes Custom. Later changes to that plan will not reach it. " + preview.Message
	}
	preview.resellerPlanned = planned
	return preview, true
}

func (p *Panel) writeResellerRightsView(w http.ResponseWriter, r *http.Request, resellerID int) {
	ctx := r.Context()
	e, err := p.resellerEffective(ctx, resellerID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	pool, hasPool, err := loadResellerPool(ctx, p.db.GetDB(), resellerID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	usage, err := resellerTreeUsage(ctx, p.db.GetDB(), resellerID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	candidates, unknown := p.previewCandidates(ctx, e.Own.Set)
	view := e.view(candidates)
	view.InstalledUnknown = unknown
	view.SubscriptionID = 0
	var shownPool *resellerPool
	over := []string{}
	if hasPool {
		shownPool = &pool
		over = poolOverUsage(pool, usage)
	}
	writeRightsJSON(w, http.StatusOK, map[string]any{
		"role": roleReseller, "reseller_id": resellerID, "rights": view,
		"pool": shownPool, "used": usage, "over_limit": over,
	})
}

// keepOfferedOptIn: a set that had "every permission" and lost it in a
// grant/revoke keeps the opt-in permissions the administrator offers
// (alpha.128), unless they were the ones revoked.
func keepOfferedOptIn(before, after rightsSet, admin adminOfferLayer, revoke []string) rightsSet {
	if !before.AllPermissions || after.AllPermissions {
		return after
	}
	revoked := map[string]bool{}
	for _, item := range revoke {
		revoked[strings.TrimSpace(item)] = true
	}
	for id := range optInPermissions {
		if admin.permits(id) && !revoked[permissionItemHead+id] {
			after.Items[permissionItemHead+id] = true
		}
	}
	return after
}
