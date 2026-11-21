package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/alicelik/celikpanel/internal/services"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The site's limits (S4 R2a, docs/SITE-SANDBOX.md section 8,
// docs/SITE-LIMITS.md section 4a). Three keys join the rights chain of D-042
// (server ⊇ reseller ⊇ plan or Custom ⊇ site) as additive registered keys:
//
//	site.memory_mb    MemoryMax= of the site's slice
//	site.tasks        TasksMax=  (processes and threads)
//	site.cpu_percent  CPUQuota=  (100 is one processor core)
//
// They are limits only (nothing for a customer to choose): the site's slice
// gets the effective ceiling, or the server's default when no level sets
// one, or the code's default when the administrator chose none. The code's
// defaults are the largest limits a site's own programs already had before
// R2a (a Node.js site 1536 MB, 200 %, 512 tasks; n8n 1536 MB running and
// 3072 MB while it installs; an Open WebUI install 3072 MB): a site keeps
// every program it runs, together within one bound. No key needs a schema
// change: a key without a setting_policy row has no server ceiling or
// default (loadSettingPolicy), and the administrator's server level writes
// its row when it is first saved.
//
// Three database keys, off unless a level sets them, become the engines' own
// role settings (services.RoleLimits): db.statement_seconds,
// db.idle_in_transaction_seconds and db.temp_file_mb.
//
// Sitenin sınırları: üç anahtar hak zincirine katılır; sitenin dilimi etkin
// tavanı, yoksa sunucunun varsayılanını alır.

const (
	settingSiteMemoryMB   = "site.memory_mb"
	settingSiteTasks      = "site.tasks"
	settingSiteCPUPercent = "site.cpu_percent"

	settingDBStatementSeconds         = "db.statement_seconds"
	settingDBIdleInTransactionSeconds = "db.idle_in_transaction_seconds"
	settingDBTempFileMB               = "db.temp_file_mb"
)

// siteSliceCodeDefaults apply when no level sets a ceiling and the server
// has no default of its own.
var siteSliceCodeDefaults = map[string]int{settingSiteMemoryMB: 3072, settingSiteTasks: 512, settingSiteCPUPercent: 200}

func init() {
	registerSettingKey(settingKeyDef{Key: settingSiteMemoryMB, Group: "site", Unit: "mb", Min: 64, Max: 65536, Presets: []int{512, 1024, 2048, 3072, 4096}})
	registerSettingKey(settingKeyDef{Key: settingSiteTasks, Group: "site", Unit: "count", Min: 16, Max: 8192, Presets: []int{64, 128, 256, 512, 1024}})
	registerSettingKey(settingKeyDef{Key: settingSiteCPUPercent, Group: "site", Unit: "percent", Min: 10, Max: 1600, Presets: []int{50, 100, 200, 400}})
	registerSettingKey(settingKeyDef{Key: settingDBStatementSeconds, Group: "db", Unit: "s", Min: 1, Max: services.MaxRoleLimitSeconds, Presets: []int{30, 60, 300, 600}})
	registerSettingKey(settingKeyDef{Key: settingDBIdleInTransactionSeconds, Group: "db", Unit: "s", Min: 10, Max: services.MaxRoleLimitSeconds, Presets: []int{60, 300, 600, 1800}})
	registerSettingKey(settingKeyDef{Key: settingDBTempFileMB, Group: "db", Unit: "mb", Min: 16, Max: services.MaxRoleTempFileMB, Presets: []int{256, 1024, 4096, 16384}})
}

// siteSliceKeys are the slice's keys in their order.
var siteSliceKeys = []string{settingSiteMemoryMB, settingSiteTasks, settingSiteCPUPercent}

// siteLimitValue is the value a site's slice gets for one key and who set
// it: a level's ceiling ("server", "reseller", "plan", "subscription"), the
// server's default ("server_default") or the code's ("default").
type siteLimitValue struct {
	Key       string `json:"key"`
	Value     int    `json:"value"`
	SetBy     string `json:"set_by"`
	SetByName string `json:"set_by_name,omitempty"`
}

// siteLimitValueOf reads one key's value from the effective limits.
func siteLimitValueOf(limits settingLimits, key string) siteLimitValue {
	def := settingKeyDefs[key]
	out := siteLimitValue{Key: key, Value: siteSliceCodeDefaults[key], SetBy: "default"}
	if limit, ok := limits[key]; ok {
		switch {
		case limit.Ceiling != nil:
			out.Value, out.SetBy, out.SetByName = *limit.Ceiling, limit.SetBy, limit.SetByName
		case limit.Default != nil:
			out.Value, out.SetBy = *limit.Default, "server_default"
		}
	}
	if out.Value < def.Min {
		out.Value = def.Min
	}
	if out.Value > def.Max {
		out.Value = def.Max
	}
	return out
}

// siteSliceFromLimits is the slice of one site under these limits.
func siteSliceFromLimits(siteID int, domain string, limits settingLimits) *transport.SiteSlice {
	if siteID <= 0 {
		return nil
	}
	s := &transport.SiteSlice{SiteID: siteID, Domain: strings.ToLower(strings.TrimSpace(domain)),
		MemoryMB:   siteLimitValueOf(limits, settingSiteMemoryMB).Value,
		Tasks:      siteLimitValueOf(limits, settingSiteTasks).Value,
		CPUPercent: siteLimitValueOf(limits, settingSiteCPUPercent).Value}
	if (sitesandbox.SliceLimits{MemoryMB: s.MemoryMB, Tasks: s.Tasks, CPUPercent: s.CPUPercent}).Validate() != nil {
		return nil
	}
	return s
}

// siteSliceForSubscription is the slice of a site of the subscription; nil
// when the limits cannot be read (the site's units then stay where they
// run, as before R2a; logged).
func (p *Panel) siteSliceForSubscription(ctx context.Context, siteID, subscriptionID int, domain string) *transport.SiteSlice {
	if p == nil || p.db == nil || siteID <= 0 || subscriptionID <= 0 {
		return nil
	}
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subscriptionID)
	if err != nil {
		log.Printf("site limits of site %d: %v", siteID, err)
		return nil
	}
	return siteSliceFromLimits(siteID, domain, limits)
}

// siteSliceForDomain is the slice of a domain's site.
func (p *Panel) siteSliceForDomain(ctx context.Context, domainID int) *transport.SiteSlice {
	if p == nil || p.db == nil || domainID <= 0 {
		return nil
	}
	var siteID, subID int
	var name string
	if err := p.db.GetDB().QueryRowContext(ctx, `
		SELECT s.id, d.subscription_id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id WHERE d.id = ?`, domainID).Scan(&siteID, &subID, &name); err != nil {
		return nil
	}
	return p.siteSliceForSubscription(ctx, siteID, subID, name)
}

// --- Changed limits reach the slices that exist ------------------------------

// siteLimitsChanged: whether an edit changed one of the slice's keys.
func siteLimitsChanged(before, after map[string]int) bool {
	for _, key := range siteSliceKeys {
		b, hadB := before[key]
		a, hadA := after[key]
		if hadB != hadA || b != a {
			return true
		}
	}
	return false
}

// applySiteSlicesCall is the Agent call (tests replace it).
var applySiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.ApplySiteSlicesRequest) (transport.ApplySiteSlicesResponse, error) {
	var resp transport.ApplySiteSlicesResponse
	err := p.callAgentContext(ctx, "Agent.ApplySiteSlicesV1", req, &resp)
	return resp, err
}

// startSiteSliceApply runs a rewrite in the background (tests run it at once).
var startSiteSliceApply = func(run func()) { go run() }

// reapplySiteSlices writes the current limits into the slices of every site
// of the given subscriptions, in the background. A site without a slice
// file yet gets one when its first program goes into it.
func (p *Panel) reapplySiteSlices(subscriptionIDs []int) {
	if len(subscriptionIDs) == 0 || (siteLimitReadNeedsAgent && p.agentClient == nil) {
		return
	}
	ids := append([]int(nil), subscriptionIDs...)
	startSiteSliceApply(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := p.reapplySiteSlicesNow(ctx, ids); err != nil {
			log.Printf("site limits: %v", err)
		}
	})
}

// reapplySiteSlicesNow is the work of reapplySiteSlices.
func (p *Panel) reapplySiteSlicesNow(ctx context.Context, subscriptionIDs []int) error {
	if p.db == nil {
		return nil
	}
	db := p.db.GetDB()
	req := &transport.ApplySiteSlicesRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}
	for _, subID := range subscriptionIDs {
		limits, err := subscriptionSettingLimits(ctx, db, subID)
		if err != nil {
			log.Printf("site limits of subscription %d: %v", subID, err)
			continue
		}
		rows, err := db.QueryContext(ctx, `SELECT s.id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id WHERE d.subscription_id = ? ORDER BY s.id`, subID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var siteID int
			var name string
			if err := rows.Scan(&siteID, &name); err != nil {
				rows.Close()
				return err
			}
			if s := siteSliceFromLimits(siteID, name, limits); s != nil {
				req.Sites = append(req.Sites, *s)
			}
		}
		rows.Close()
	}
	if len(req.Sites) == 0 {
		return nil
	}
	resp, err := applySiteSlicesCall(p, ctx, req)
	if err != nil {
		return err
	}
	for _, s := range resp.Sites {
		switch s.Outcome {
		case transport.SiteFileOutcomeKept:
			log.Printf("site limits: the slice of site %d was changed on the server and is kept; CelikPanel's text is in %s", s.SiteID, s.PendingPath)
		case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
			log.Printf("site limits: the slice of site %d stays as it was until the next change: %s", s.SiteID, s.Detail)
		}
	}
	if resp.Error != "" {
		log.Printf("site limits: %s", resp.Error)
	}
	return nil
}

// --- Database role limits -------------------------------------------------------

// dbRoleLimitKeys are the database keys that become role settings.
var dbRoleLimitKeys = []string{settingDBStatementSeconds, settingDBIdleInTransactionSeconds, settingDBTempFileMB}

// dbRoleLimitsFrom reads the role limits from the effective limits (no
// ceiling: none).
func dbRoleLimitsFrom(limits settingLimits) services.RoleLimits {
	return services.RoleLimits{
		StatementSeconds:         limits.ceilingValue(settingDBStatementSeconds),
		IdleInTransactionSeconds: limits.ceilingValue(settingDBIdleInTransactionSeconds),
		TempFileMB:               limits.ceilingValue(settingDBTempFileMB),
	}
}

// dbRoleLimits is the role limits of a subscription's database users.
func (p *Panel) dbRoleLimits(ctx context.Context, subscriptionID int) (services.RoleLimits, error) {
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), subscriptionID)
	if err != nil {
		return services.RoleLimits{}, err
	}
	return dbRoleLimitsFrom(limits), nil
}
