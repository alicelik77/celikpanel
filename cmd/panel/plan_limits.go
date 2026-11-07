package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Plans and site setting limits (alpha.128, docs/SITE-LIMITS.md): the
// ceilings a plan holds, the preview of an edit that lowers one (how many
// sites of the plan are above it), the plan's overuse policy and notice
// levels, and what a subscription keeps when it leaves its plan.
//
// Planlar ve site ayarı sınırları: tavanlar, düşürülen tavanın önizlemesi,
// aşım kuralı ve uyarı düzeyleri.

const (
	overusePolicyClose  = "close"
	overusePolicyNotify = "notify"
)

// planPolicyOrDefaults takes the payload's policy fields, falling back to
// the given values for the ones it leaves out.
func planPolicyOrDefaults(pl planPayload, policy string, disk, traffic int) (string, int, int) {
	if pl.OverusePolicy != "" {
		policy = pl.OverusePolicy
	}
	if pl.NotifyPercentDisk != 0 {
		disk = pl.NotifyPercentDisk
	}
	if pl.NotifyPercentTraffic != 0 {
		traffic = pl.NotifyPercentTraffic
	}
	return policy, disk, traffic
}

func loadPlanPolicy(ctx context.Context, q rowQueryer, planID int) (string, int, int, error) {
	var policy string
	var disk, traffic int
	err := q.QueryRowContext(ctx, `SELECT overuse_policy, notify_percent_disk, notify_percent_traffic FROM service_plans WHERE id = ?`, planID).
		Scan(&policy, &disk, &traffic)
	return policy, disk, traffic, err
}

// subscriptionOverusePolicy is the policy a subscription follows: its plan's
// when bound, else its own (Custom, Unlimited).
func subscriptionOverusePolicy(ctx context.Context, q rowQueryer, subID int) (policy string, diskPct, trafficPct int, err error) {
	err = q.QueryRowContext(ctx, `
		SELECT CASE WHEN sp.id IS NULL THEN s.overuse_policy ELSE sp.overuse_policy END,
		       CASE WHEN sp.id IS NULL THEN s.notify_percent_disk ELSE sp.notify_percent_disk END,
		       CASE WHEN sp.id IS NULL THEN s.notify_percent_traffic ELSE sp.notify_percent_traffic END
		FROM subscriptions s LEFT JOIN service_plans sp ON sp.id = s.plan_id
		WHERE s.id = ?`, subID).Scan(&policy, &diskPct, &trafficPct)
	if errors.Is(err, sql.ErrNoRows) {
		return overusePolicyClose, 80, 80, nil
	}
	return policy, diskPct, trafficPct, err
}

// planCeilingsWithinGrantor validates a plan's ceilings against the field
// of whoever saves it: the server for the administrator, the reseller's own
// limits for a reseller. nil keeps current.
func (p *Panel) planCeilingsWithinGrantor(w http.ResponseWriter, r *http.Request, c *Caller, input *ceilingsPayload, current map[string]int) (map[string]int, bool) {
	if input == nil {
		return current, true
	}
	ctx := r.Context()
	var above settingLimits
	var err error
	if c.hasAccountRole(roleReseller) && !c.hasAccountRole(roleAdmin) {
		above, err = resellerSettingLimits(ctx, p.db.GetDB(), c.ID)
	} else {
		above, err = serverSettingLimits(ctx, p.db.GetDB())
	}
	if err != nil {
		writeServerError(w, err)
		return nil, false
	}
	clean, refusals, err := validateCeilings(map[string]int(*input), above)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, errCodeSettingUnknown, "Nothing was changed: "+err.Error()+".", "")
		return nil, false
	}
	if len(refusals) > 0 {
		writeSettingAboveLimit(w, refusals)
		return nil, false
	}
	return clean, true
}

// loweredCeilings lists the keys whose ceiling an edit makes stricter (a new
// ceiling, a smaller one, or for a floor a larger one).
// phpCeilingsChanged says whether any PHP ceiling was set, changed or removed.
func phpCeilingsChanged(before, after map[string]int) bool {
	for key, value := range after {
		if strings.HasPrefix(key, "php.") && before[key] != value {
			return true
		}
	}
	for key := range before {
		if _, kept := after[key]; strings.HasPrefix(key, "php.") && !kept {
			return true
		}
	}
	return false
}

func loweredCeilings(before, after map[string]int) []string {
	out := []string{}
	for key, value := range after {
		was, had := before[key]
		def := settingKeyDefs[key]
		switch {
		case !had:
			out = append(out, key)
		case def.Floor && value > was:
			out = append(out, key)
		case !def.Floor && value < was:
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// planPHPSites lists the PHP sites of the subscriptions bound to a plan.
// clampedSite is one site value above a lowered limit.
type clampedSite struct {
	DomainID int    `json:"domain_id"`
	Domain   string `json:"domain"`
	Key      string `json:"key"`
	Value    int    `json:"value"`
	Limit    int    `json:"limit"`
	// SetBy and SetByName name the level that lowered it.
	SetBy     string `json:"set_by"`
	SetByName string `json:"set_by_name,omitempty"`
}

// noticeSiteLimitClamped (alpha.128): a lowered limit brought a site's own
// value down. One notice per person and site value (customer, its reseller,
// administrators), kept until it is marked seen; a second lowering of the
// same value while it is open adds nothing (once).
const noticeSiteLimitClamped = "site_limit_clamped"

// storeClampNotices writes the notices of the sites a plan edit clamped.
func (p *Panel) storeClampNotices(ctx context.Context, clamped []clampedSite, setBy, setByName string) {
	db := p.db.GetDB()
	for _, site := range clamped {
		site.SetBy, site.SetByName = setBy, setByName
		detail, err := json.Marshal(site)
		if err != nil {
			continue
		}
		subID, err := p.domainSubscriptionID(ctx, site.DomainID)
		if err != nil {
			continue
		}
		recipients, err := usageRecipientsFor(ctx, db, subID)
		if err != nil {
			log.Printf("site limit notice %s: %v", site.Domain, err)
			continue
		}
		for _, recipient := range recipients {
			if _, err := db.ExecContext(ctx, `INSERT INTO rights_notices (kind, user_id, subject, detail) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
				noticeSiteLimitClamped, recipient.UserID, site.Domain+" "+site.Key, string(detail)); err != nil {
				log.Printf("site limit notice %s for user %d: %v", site.Domain, recipient.UserID, err)
			}
		}
	}
}

func (p *Panel) planPHPSites(ctx context.Context, planID int) ([]siteFileSite, error) {
	rows, err := p.db.GetDB().QueryContext(ctx, `
		SELECT s.id, d.id, d.name, COALESCE(s.php_version, '')
		FROM sites s JOIN domains d ON d.id = s.domain_id
		JOIN subscriptions sub ON sub.id = d.subscription_id
		WHERE sub.plan_id = ? AND COALESCE(s.project_type, 'php') = 'php'
		ORDER BY d.name`, planID)
	if err != nil {
		return nil, fmt.Errorf("list the PHP sites of the plan: %w", err)
	}
	defer rows.Close()
	sites := []siteFileSite{}
	for rows.Next() {
		site := siteFileSite{ProjectType: "php"}
		if err := rows.Scan(&site.SiteID, &site.DomainID, &site.Domain, &site.PHPVersion); err != nil {
			return nil, err
		}
		if services.ValidatePHPVersion(site.PHPVersion) == nil {
			sites = append(sites, site)
		}
	}
	return sites, rows.Err()
}

// previewLoweredCeilings adds to a plan edit's preview the sites whose own
// PHP value is above a lowered ceiling. The values live in the pool files,
// so they are read from the Agent in one call; when that read fails the
// preview says so (SitesUnread) and the edit asks for confirmation.
func (p *Panel) previewLoweredCeilings(ctx context.Context, preview *rightsPreview, planID int, lowered []string, after map[string]int) error {
	preview.CeilingsLowered = lowered
	if len(lowered) == 0 {
		return nil
	}
	php := false
	for _, key := range lowered {
		php = php || strings.HasPrefix(key, "php.")
	}
	if !php {
		return nil
	}
	sites, err := p.planPHPSites(ctx, planID)
	if err != nil || len(sites) == 0 {
		return err
	}
	request := transport.SitePHPSettingsRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}
	for _, site := range sites {
		request.Sites = append(request.Sites, transport.SitePHPSettingsSite{SiteID: site.SiteID, Domain: site.Domain,
			Username: services.SiteUsername(site.Domain), PHPVersion: site.PHPVersion})
	}
	var response transport.SitePHPSettingsResponse
	if err := p.callAgentContext(ctx, "Agent.SitePHPSettingsV1", &request, &response); err != nil || response.Error != "" || len(response.Sites) != len(sites) {
		preview.SitesUnread = true
		preview.Message += " The sites' own PHP values could not be read now, so it is not known how many use more than a lowered limit; confirm to save anyway."
		return nil
	}
	above := []string{}
	preview.clamped = nil
	for index, read := range response.Sites {
		values := map[string]int{settingPHPMemoryMB: read.Own.MemoryMB, settingPHPTimeS: read.Own.TimeS,
			settingPHPUploadMB: read.Own.UploadMB, settingPHPInputVars: read.Own.InputVars, settingPHPProcesses: read.Processes}
		parts := []string{}
		for _, key := range lowered {
			value, ok := values[key]
			if !ok || value == 0 {
				continue
			}
			if limit := after[key]; value > limit {
				parts = append(parts, fmt.Sprintf("%s %d -> %d", key, value, limit))
				preview.clamped = append(preview.clamped, clampedSite{DomainID: sites[index].DomainID, Domain: sites[index].Domain,
					Key: key, Value: value, Limit: limit})
			}
		}
		if len(parts) > 0 {
			above = append(above, sites[index].Domain+": "+strings.Join(parts, ", "))
		}
	}
	preview.SitesAboveCount = len(above)
	if len(above) > 40 {
		above = append(above[:40], fmt.Sprintf("+%d more", len(above)-40))
	}
	preview.SitesAbove = above
	if preview.SitesAboveCount > 0 {
		preview.Message += fmt.Sprintf(" %d site(s) use more than a lowered limit; they keep running at the new limit.", preview.SitesAboveCount)
	}
	return nil
}

// reapplyPlanSiteLimits writes the pools of a plan's PHP sites again, so a
// lowered ceiling reaches them now. One pass in the background; a file the
// owner changed is kept (and logged), as for every write.
func (p *Panel) reapplyPlanSiteLimits(planID int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		sites, err := p.planPHPSites(ctx, planID)
		if err != nil {
			log.Printf("plan %d: re-apply site limits: %v", planID, err)
			return
		}
		for _, site := range sites {
			request := p.sitePoolRequest(ctx, site, transport.SitePoolOperationApply)
			if _, err := p.applySitePool(ctx, site, request); err != nil {
				log.Printf("plan %d: site %s keeps its pool for now (lowered limit applies at its next write): %v", planID, site.Domain, err)
			}
		}
	}()
}

// setSubscriptionOwnPolicy writes a Custom subscription's own overuse policy
// and notice levels (the fields a request leaves out are kept).
func setSubscriptionOwnPolicy(ctx context.Context, tx sqlExecer, subID int, req rightsChangeRequest) error {
	if req.OverusePolicy != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET overuse_policy = ? WHERE id = ?`, req.OverusePolicy, subID); err != nil {
			return err
		}
	}
	if req.NotifyPercentDisk != 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET notify_percent_disk = ? WHERE id = ?`, req.NotifyPercentDisk, subID); err != nil {
			return err
		}
	}
	if req.NotifyPercentTraffic != 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE subscriptions SET notify_percent_traffic = ? WHERE id = ?`, req.NotifyPercentTraffic, subID); err != nil {
			return err
		}
	}
	return nil
}

// keepPlanLimitsAsOwn: a subscription that leaves its plan keeps the plan's
// ceilings and policy as its own (P3: Custom starts as a copy).
func keepPlanLimitsAsOwn(ctx context.Context, tx rightsTx, subID int) error {
	var planID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT plan_id FROM subscriptions WHERE id = ?`, subID).Scan(&planID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !planID.Valid || planID.Int64 <= 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE subscriptions SET
		  overuse_policy = (SELECT overuse_policy FROM service_plans WHERE id = ?),
		  notify_percent_disk = (SELECT notify_percent_disk FROM service_plans WHERE id = ?),
		  notify_percent_traffic = (SELECT notify_percent_traffic FROM service_plans WHERE id = ?)
		WHERE id = ? AND EXISTS (SELECT 1 FROM service_plans WHERE id = ?)`,
		planID.Int64, planID.Int64, planID.Int64, subID, planID.Int64); err != nil {
		return fmt.Errorf("keep the plan's policy: %w", err)
	}
	ceilings, err := loadSettingCeilings(ctx, tx, rightsOwnerPlan, int(planID.Int64))
	if err != nil {
		return err
	}
	return saveSettingCeilings(ctx, tx, rightsOwnerSubscription, subID, ceilings)
}

// keepResellerPlanLimitsAsOwn: a reseller that leaves its reseller plan
// keeps its ceilings and overselling choice as its own.
func keepResellerPlanLimitsAsOwn(ctx context.Context, tx rightsTx, resellerID int) error {
	var planID int
	err := tx.QueryRowContext(ctx, `SELECT reseller_plan_id FROM reseller_plan_bindings WHERE reseller_id = ?`, resellerID).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ceilings, err := loadSettingCeilings(ctx, tx, rightsOwnerResellerPlan, planID)
	if err != nil {
		return err
	}
	return saveSettingCeilings(ctx, tx, rightsOwnerReseller, resellerID, ceilings)
}
