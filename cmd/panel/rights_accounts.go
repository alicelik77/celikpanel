package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// State changes of accounts (P3, P7): Unlimited, bound to a plan, or Custom.
// Every change runs in one transaction; the handlers show the preview first
// and write the audit row. Quantities stay in their existing homes
// (subscriptions.max_*, reseller_pools) and follow the state.
//
// Hesap durum değişiklikleri: Sınırsız, plana bağlı ya da Özel. Her değişiklik
// tek işlemdir; önce önizleme, sonra onay ve denetim kaydı.

// rightsQuantities is a partial set of numbers (nil = keep). 0 = unlimited.
type rightsQuantities struct {
	Domains   *int `json:"max_domains,omitempty"`
	Databases *int `json:"max_databases,omitempty"`
	Mailboxes *int `json:"max_email_accounts,omitempty"`
	DiskMB    *int `json:"disk_quota_mb,omitempty"`
	TrafficMB *int `json:"traffic_quota_mb,omitempty"`
	Customers *int `json:"max_customers,omitempty"`
}

func (q *rightsQuantities) valid() bool {
	if q == nil {
		return true
	}
	for _, v := range []*int{q.Domains, q.Databases, q.Mailboxes, q.DiskMB, q.TrafficMB, q.Customers} {
		if v != nil && *v < 0 {
			return false
		}
	}
	return true
}

func (q *rightsQuantities) applyLimits(l planLimits) planLimits {
	if q == nil {
		return l
	}
	if q.Domains != nil {
		l.Domains = *q.Domains
	}
	if q.Databases != nil {
		l.Databases = *q.Databases
	}
	if q.Mailboxes != nil {
		l.Mailboxes = *q.Mailboxes
	}
	if q.DiskMB != nil {
		l.DiskMB = *q.DiskMB
	}
	if q.TrafficMB != nil {
		l.TrafficMB = *q.TrafficMB
	}
	return l
}

func (q *rightsQuantities) applyPool(pool resellerPool) resellerPool {
	pool.planLimits = q.applyLimits(pool.planLimits)
	if q != nil && q.Customers != nil {
		pool.MaxCustomers = *q.Customers
	}
	return pool
}

func loadSubscriptionLimits(ctx context.Context, q rowQueryer, subID int) (planLimits, error) {
	var l planLimits
	var domains, databases, mailboxes, disk, traffic sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT max_domains, max_databases, max_email_accounts, disk_quota_mb, bandwidth_quota_mb FROM subscriptions WHERE id = ?`, subID).
		Scan(&domains, &databases, &mailboxes, &disk, &traffic)
	if errors.Is(err, sql.ErrNoRows) {
		return l, errRightsSubscriptionMissing
	}
	l.Domains, l.Databases, l.Mailboxes, l.DiskMB = int(domains.Int64), int(databases.Int64), int(mailboxes.Int64), int(disk.Int64)
	l.TrafficMB = int(traffic.Int64)
	return l, err
}

func writeSubscriptionLimits(ctx context.Context, tx sqlExecer, subID int, planID *int, l planLimits, traffic int) error {
	defer kickDatabaseRemoteSync() // alpha.129: a plan change reaches remote database access
	var plan any
	if planID != nil {
		plan = *planID
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE subscriptions SET plan_id = ?, max_domains = ?, max_databases = ?, max_email_accounts = ?,
		       disk_quota_mb = ?, bandwidth_quota_mb = ?, updated_at = datetime('now')
		WHERE id = ?`, plan, l.Domains, l.Databases, l.Mailboxes, l.DiskMB, traffic, subID)
	return err
}

// setSubscriptionUnlimited: the built-in Unlimited plan (every limit 0).
func setSubscriptionUnlimited(ctx context.Context, tx rightsTx, subID int) error {
	if err := deleteRightsSet(ctx, tx, rightsOwnerSubscription, subID); err != nil {
		return err
	}
	if err := deleteSettingCeilings(ctx, tx, rightsOwnerSubscription, subID); err != nil {
		return err
	}
	return writeSubscriptionLimits(ctx, tx, subID, nil, planLimits{}, 0)
}

// bindSubscriptionPlan: identical to the plan from now on.
func bindSubscriptionPlan(ctx context.Context, tx rightsTx, subID int, plan loadedPlan) error {
	if err := deleteRightsSet(ctx, tx, rightsOwnerSubscription, subID); err != nil {
		return err
	}
	// alpha.128: a bound subscription reads the plan's ceilings and policy.
	if err := deleteSettingCeilings(ctx, tx, rightsOwnerSubscription, subID); err != nil {
		return err
	}
	return writeSubscriptionLimits(ctx, tx, subID, &plan.ID, plan.Limits, plan.Limits.TrafficMB)
}

// setSubscriptionCustom: the subscription's own set and numbers.
func setSubscriptionCustom(ctx context.Context, tx rightsTx, subID int, set rightsSet, limits planLimits) error {
	if err := saveRightsSet(ctx, tx, rightsOwnerSubscription, subID, set); err != nil {
		return err
	}
	if err := keepPlanLimitsAsOwn(ctx, tx, subID); err != nil {
		return err
	}
	return writeSubscriptionLimits(ctx, tx, subID, nil, limits, limits.TrafficMB)
}

// resellerPlan is one reseller plan with its set.
type resellerPlan struct {
	ID   int
	Name string
	Pool resellerPool
	Set  rightsSet
}

func loadResellerPlan(ctx context.Context, q rowQueryer, id int) (resellerPlan, error) {
	var plan resellerPlan
	err := q.QueryRowContext(ctx, `
		SELECT id, name, max_customers, max_domains, max_databases, max_email_accounts, disk_quota_mb, traffic_quota_mb
		FROM reseller_plans WHERE id = ?`, id).Scan(&plan.ID, &plan.Name, &plan.Pool.MaxCustomers,
		&plan.Pool.Domains, &plan.Pool.Databases, &plan.Pool.Mailboxes, &plan.Pool.DiskMB, &plan.Pool.TrafficMB)
	if errors.Is(err, sql.ErrNoRows) {
		return plan, errNotFound
	}
	if err != nil {
		return plan, err
	}
	set, found, err := loadRightsSet(ctx, q, rightsOwnerResellerPlan, id)
	if err != nil {
		return plan, err
	}
	if !found {
		set = allOfferedRightsSet()
	}
	plan.Set = set
	return plan, nil
}

func poolIsUnlimited(pool resellerPool) bool {
	return pool.MaxCustomers <= 0 && pool.Domains <= 0 && pool.Databases <= 0 && pool.Mailboxes <= 0 && pool.DiskMB <= 0 && pool.TrafficMB <= 0
}

func writeResellerPool(ctx context.Context, tx sqlExecer, resellerID int, pool *resellerPool) error {
	if pool == nil || poolIsUnlimited(*pool) {
		_, err := tx.ExecContext(ctx, `DELETE FROM reseller_pools WHERE reseller_id = ?`, resellerID)
		return err
	}
	return upsertResellerPool(ctx, tx, resellerID, *pool)
}

func setResellerUnlimited(ctx context.Context, tx rightsTx, resellerID int) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM reseller_plan_bindings WHERE reseller_id = ?`, resellerID); err != nil {
		return err
	}
	if err := deleteSettingCeilings(ctx, tx, rightsOwnerReseller, resellerID); err != nil {
		return err
	}
	if err := deleteRightsSet(ctx, tx, rightsOwnerReseller, resellerID); err != nil {
		return err
	}
	return writeResellerPool(ctx, tx, resellerID, nil)
}

func bindResellerPlan(ctx context.Context, tx rightsTx, resellerID int, plan resellerPlan) error {
	if err := deleteRightsSet(ctx, tx, rightsOwnerReseller, resellerID); err != nil {
		return err
	}
	if err := deleteSettingCeilings(ctx, tx, rightsOwnerReseller, resellerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO reseller_plan_bindings (reseller_id, reseller_plan_id) VALUES (?, ?)
		ON CONFLICT(reseller_id) DO UPDATE SET reseller_plan_id = excluded.reseller_plan_id, bound_at = datetime('now')`,
		resellerID, plan.ID); err != nil {
		return err
	}
	pool := plan.Pool
	return writeResellerPool(ctx, tx, resellerID, &pool)
}

func setResellerCustom(ctx context.Context, tx rightsTx, resellerID int, set rightsSet, pool resellerPool) error {
	if err := keepResellerPlanLimitsAsOwn(ctx, tx, resellerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reseller_plan_bindings WHERE reseller_id = ?`, resellerID); err != nil {
		return err
	}
	if err := saveRightsSet(ctx, tx, rightsOwnerReseller, resellerID, set); err != nil {
		return err
	}
	return writeResellerPool(ctx, tx, resellerID, &pool)
}

// resellerToCustom is what the pool endpoint does before it changes the
// numbers: a bound reseller leaves its plan with a copy of the plan's set; an
// Unlimited reseller given a pool becomes Custom with everything offered.
// It returns the state left ("" when nothing changed).
func resellerToCustom(ctx context.Context, tx rightsTx, resellerID int, givingPool bool) (string, error) {
	state, err := resellerRightsState(ctx, tx, resellerID)
	if err != nil {
		return "", err
	}
	switch {
	case state.State == rightsStatePlan:
		if err := keepResellerPlanLimitsAsOwn(ctx, tx, resellerID); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM reseller_plan_bindings WHERE reseller_id = ?`, resellerID); err != nil {
			return "", err
		}
		return state.stateLabel(), saveRightsSet(ctx, tx, rightsOwnerReseller, resellerID, state.Set)
	case state.State == rightsStateUnlimited && givingPool:
		return rightsStateUnlimited, saveRightsSet(ctx, tx, rightsOwnerReseller, resellerID, allOfferedRightsSet())
	case state.Implicit:
		return "", saveRightsSet(ctx, tx, rightsOwnerReseller, resellerID, allOfferedRightsSet())
	}
	return "", nil
}

// grantorOutside lists the items of set that the grantor may not hand out:
// for a reseller, items outside its own effective set; for the
// administrator, items it does not offer on this server. The "all" flags
// are never outside: they mean "everything the grantor has".
func grantorOutside(admin adminOfferLayer, reseller *rightsState, set rightsSet) []string {
	outside := []string{}
	for _, item := range set.itemList() {
		if id, ok := strings.CutPrefix(item, permissionItemHead); ok {
			if !admin.permits(id) || (reseller != nil && !reseller.Set.permits(id)) {
				outside = append(outside, item)
			}
			continue
		}
		if !admin.offers(item) || (reseller != nil && !reseller.Set.offers(item)) {
			outside = append(outside, item)
		}
	}
	return outside
}

// rightsDiff compares two effective sets over the permissions and the
// candidate software ids.
type rightsDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

func diffEffective(before, after *effectiveRights, candidates []string) rightsDiff {
	d := rightsDiff{Added: []string{}, Removed: []string{}}
	for _, id := range rightPermissionIDs {
		was, is := before.hasPermission(id), after.hasPermission(id)
		switch {
		case was && !is:
			d.Removed = append(d.Removed, permissionItemHead+id)
		case !was && is:
			d.Added = append(d.Added, permissionItemHead+id)
		}
	}
	seen := map[string]bool{}
	for _, id := range candidates {
		if seen[id] || !validOfferingID(id) {
			continue
		}
		seen[id] = true
		was, is := before.offers(id), after.offers(id)
		switch {
		case was && !is:
			d.Removed = append(d.Removed, id)
		case !was && is:
			d.Added = append(d.Added, id)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	return d
}

// withOwn is a copy of e with another own layer (for previews).
func (e *effectiveRights) withOwn(set rightsSet) *effectiveRights {
	out := *e
	out.Own.Set = set
	out.Unrestricted = false
	return &out
}

// withReseller is a copy of e with another reseller layer.
func (e *effectiveRights) withReseller(set rightsSet) *effectiveRights {
	out := *e
	state := rightsState{Set: set}
	out.reseller = &state
	return &out
}

// previewCandidates: the installed offering ids (best effort) and every
// item the compared sets name.
func (p *Panel) previewCandidates(ctx context.Context, sets ...rightsSet) ([]string, bool) {
	installed, err := p.installedOfferingIDs(ctx)
	unknown := err != nil
	out := append([]string{}, installed...)
	for _, set := range sets {
		out = append(out, set.softwareList()...)
	}
	return out, unknown
}

// inUseLine says how much of a removed item the given subscriptions use now.
type inUseLine struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
	What  string `json:"what"`
}

// removedItemsInUse counts, for each removed item, what the subscriptions
// use of it now (it keeps running; only new use is refused).
func (p *Panel) removedItemsInUse(ctx context.Context, subIDs []int, removed []string) ([]inUseLine, error) {
	out := []inUseLine{}
	if len(subIDs) == 0 || len(removed) == 0 {
		return out, nil
	}
	holders := strings.TrimSuffix(strings.Repeat("?,", len(subIDs)), ",")
	args := make([]any, 0, len(subIDs))
	for _, id := range subIDs {
		args = append(args, id)
	}
	db := p.db.GetDB()
	countSites := func(projectType, column, component, line string) (int, error) {
		rows, err := db.QueryContext(ctx, `
			SELECT COALESCE(si.`+column+`, '') FROM sites si JOIN domains d ON d.id = si.domain_id
			WHERE COALESCE(si.project_type, 'php') = ? AND d.subscription_id IN (`+holders+`)`,
			append([]any{projectType}, args...)...)
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var version string
			if rows.Scan(&version) == nil && (line == "" || offeredVersionLine(component, version) == line) {
				n++
			}
		}
		return n, rows.Err()
	}
	count := func(query string) (int, error) {
		var n int
		err := db.QueryRowContext(ctx, query, args...).Scan(&n)
		return n, err
	}
	for _, item := range removed {
		var n int
		var err error
		what := ""
		parts := strings.SplitN(item, ":", 3)
		line := ""
		if len(parts) == 3 {
			line = parts[2]
		}
		switch {
		case len(parts) >= 2 && parts[0] == "component" && parts[1] == "php-fpm":
			n, err = countSites("php", "php_version", "php", line)
			what = "sites"
		case len(parts) >= 2 && parts[0] == "component" && parts[1] == "node":
			n, err = countSites("node", "runtime_version", "node", line)
			what = "sites"
		case item == "component:mariadb" || strings.HasPrefix(item, "component:postgresql"):
			engine := "mariadb"
			if strings.HasPrefix(item, "component:postgresql") {
				engine = "postgresql"
			}
			n, err = count(`SELECT COUNT(*) FROM databases_v2 db JOIN database_servers ds ON ds.id = db.server_id
				JOIN database_server_types t ON t.id = ds.type_id
				WHERE t.name = '` + engine + `' AND db.subscription_id IN (` + holders + `)`)
			what = "databases"
		case item == permissionItemHead+rightMail:
			n, err = count(`SELECT COUNT(*) FROM email_accounts ea JOIN domains d ON d.id = ea.domain_id WHERE d.subscription_id IN (` + holders + `)`)
			what = "mailboxes"
		case item == permissionItemHead+rightDatabases:
			n, err = count(`SELECT COUNT(*) FROM databases_v2 WHERE subscription_id IN (` + holders + `)`)
			what = "databases"
		case item == permissionItemHead+rightSiteProxy:
			n, err = count(`SELECT COUNT(*) FROM sites si JOIN domains d ON d.id = si.domain_id WHERE si.project_type = 'proxy' AND d.subscription_id IN (` + holders + `)`)
			what = "sites"
		case item == permissionItemHead+rightVPN:
			n, err = count(`SELECT COUNT(*) FROM vpn_peers WHERE desired_state = 'active' AND subscription_id IN (` + holders + `)`)
			what = "VPN devices"
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("count use of %s: %w", item, err)
		}
		if n > 0 {
			out = append(out, inUseLine{Item: item, Count: n, What: what})
		}
	}
	return out, nil
}

// subscriptionsOnPlan lists the subscriptions bound to a customer plan.
func subscriptionsOnPlan(ctx context.Context, q rowQueryer, planID int) ([]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM subscriptions WHERE plan_id = ? ORDER BY id`, planID)
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

// treeSubscriptionIDs lists the subscriptions of a reseller's tree.
func treeSubscriptionIDs(ctx context.Context, q rowQueryer, resellerID int) ([]int, error) {
	rows, err := q.QueryContext(ctx, treeSubscriptionsSQL+` ORDER BY s.id`, resellerID, resellerID)
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

// resellerEffective is a reseller's own field: the administrator's offer
// narrowed by the reseller's set (what it may hand out).
func (p *Panel) resellerEffective(ctx context.Context, resellerID int) (*effectiveRights, error) {
	state, err := resellerRightsState(ctx, p.db.GetDB(), resellerID)
	if err != nil {
		return nil, err
	}
	admin, err := p.loadAdminOfferLayer(ctx)
	if err != nil {
		return nil, err
	}
	return &effectiveRights{admin: admin, Own: state, OwnerID: resellerID, grants: allGrants()}, nil
}

// adminEffective is the administrator's field: everything it offers.
func (p *Panel) adminEffective(ctx context.Context) (*effectiveRights, error) {
	admin, err := p.loadAdminOfferLayer(ctx)
	if err != nil {
		return nil, err
	}
	return &effectiveRights{admin: admin, Own: rightsState{State: rightsStateUnlimited, Set: allOfferedRightsSet()}, grants: allGrants()}, nil
}

// allGrants: the add-on layer is not part of a plan comparison.
func allGrants() map[string]bool {
	out := map[string]bool{}
	for permission := range legacyGrantPermissions {
		out[permission] = true
	}
	return out
}
