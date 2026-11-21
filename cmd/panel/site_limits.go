package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
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
// has no default of its own. The memory default is bound to the host
// (review finding 11): siteMemoryCodeDefault.
var siteSliceCodeDefaults = map[string]int{settingSiteMemoryMB: 3072, settingSiteTasks: 512, settingSiteCPUPercent: 200}

// hostMemoryMB is the host's memory (MemTotal in /proc/meminfo, whole MB; 0
// when it cannot be read). A seam for tests.
var hostMemoryMB = func() int {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, err := strconv.Atoi(fields[1])
			if err == nil && kb > 0 {
				return kb / 1024
			}
		}
	}
	return 0
}

// siteMemoryCodeDefault is a site's memory limit when no level sets one: the
// smaller of 3072 MB and 75 % of the host's memory (rounded down to a whole
// MB), never under 64 MB. One site can then never take the whole host by
// default; a plan sets more where wanted.
func siteMemoryCodeDefault() int {
	value := siteSliceCodeDefaults[settingSiteMemoryMB]
	if host := hostMemoryMB(); host > 0 && host*3/4 < value {
		value = host * 3 / 4
	}
	if value < 64 {
		value = 64
	}
	return value
}

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
	if key == settingSiteMemoryMB {
		out.Value = siteMemoryCodeDefault()
	}
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

// siteSliceApplyBatch is the number of sites per Agent call (the Agent's
// own bound): each call holds the Agent's slice lock, which a customer's
// program write waits for, for one batch only.
const siteSliceApplyBatch = 100

// siteLimitApplyProgress is the running write of changed limits (shown on
// the administrator's site card while it runs; review finding 12).
type siteLimitApplyProgress struct {
	mu    sync.Mutex
	Done  int
	Total int
}

var siteLimitApplying siteLimitApplyProgress

func (a *siteLimitApplyProgress) set(done, total int) {
	a.mu.Lock()
	a.Done, a.Total = done, total
	a.mu.Unlock()
}

func (a *siteLimitApplyProgress) read() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Done, a.Total
}

// reapplySiteSlicesNow is the work of reapplySiteSlices: the sites in
// batches, with the progress recorded and logged.
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
	all := req.Sites
	defer siteLimitApplying.set(0, 0)
	for start := 0; start < len(all); start += siteSliceApplyBatch {
		end := start + siteSliceApplyBatch
		if end > len(all) {
			end = len(all)
		}
		siteLimitApplying.set(start, len(all))
		batch := &transport.ApplySiteSlicesRequest{ExpectedBuildCommit: req.ExpectedBuildCommit, Sites: all[start:end]}
		resp, err := applySiteSlicesCall(p, ctx, batch)
		if err != nil {
			return fmt.Errorf("after %d of %d sites: %w", start, len(all), err)
		}
		if resp.Unsupported != "" {
			log.Printf("site limits: nothing was written: %s", resp.Unsupported)
			return nil
		}
		for _, s := range resp.Sites {
			switch s.Outcome {
			case transport.SiteFileOutcomeKept:
				log.Printf("site limits: the slice of site %d was changed on the server and is kept; CelikPanel's text is in %s", s.SiteID, s.PendingPath)
			case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
				log.Printf("site limits: the slice of site %d stays as it was until the next change: %s", s.SiteID, s.Detail)
			}
			if s.MovedAside != "" {
				log.Printf("site limits: an earlier site's changed limits file of site id %d was moved aside as %s", s.SiteID, s.MovedAside)
			}
		}
		if resp.Error != "" {
			log.Printf("site limits: %s", resp.Error)
		}
		if len(all) > siteSliceApplyBatch {
			log.Printf("site limits: %d of %d sites written", end, len(all))
		}
	}
	return nil
}

// --- A lowered limit below what a site uses now asks first ----------------------

// siteLimitsLoweredKeys are the site keys an edit lowers that stop programs
// at once when they are below what they use (memory) or hold (processes).
func siteLimitsLoweredKeys(before, after map[string]int) map[string]int {
	out := map[string]int{}
	for _, key := range []string{settingSiteMemoryMB, settingSiteTasks} {
		value, set := after[key]
		if !set {
			continue
		}
		if old, had := before[key]; !had || value < old {
			out[key] = value
		}
	}
	return out
}

// previewSiteLimitsLowered adds to a change's preview the sites of these
// subscriptions whose programs use more now than a lowered site memory limit
// (or hold more processes than a lowered process limit): the system stops
// their programs at once when the change is saved, so the change asks
// first, in the panel's own confirmation (review finding 9). The reading is
// the Agent's now; when it fails the preview says the count is not known.
func (p *Panel) previewSiteLimitsLowered(ctx context.Context, preview *rightsPreview, subscriptionIDs []int, before, after map[string]int) {
	lowered := siteLimitsLoweredKeys(before, after)
	if len(lowered) == 0 || len(subscriptionIDs) == 0 || (siteLimitReadNeedsAgent && p.agentClient == nil) {
		return
	}
	in := map[int]bool{}
	for _, id := range subscriptionIDs {
		in[id] = true
	}
	all, err := p.siteLimitSites(ctx, 0)
	if err != nil {
		preview.SitesUnread = true
		return
	}
	var sites []siteLimitSite
	for _, s := range all {
		if in[s.SubscriptionID] && len(s.Units) > 0 {
			sites = append(sites, s)
		}
	}
	if len(sites) == 0 {
		return
	}
	statuses, err := p.readSiteSlicesPaged(ctx, sites)
	if err != nil {
		preview.SitesUnread = true
		preview.Message += " What the sites' programs use now could not be read, so it is not known whether a lowered site limit stops some of them; confirm to save anyway."
		return
	}
	names := map[int]string{}
	for _, s := range sites {
		names[s.SiteID] = s.Domain
	}
	var lines []string
	for _, status := range statuses {
		if !status.Active {
			continue
		}
		var parts []string
		if limit, ok := lowered[settingSiteMemoryMB]; ok && status.MemoryCurrentMB > int64(limit) {
			parts = append(parts, fmt.Sprintf("memory %d MB in use, new limit %d MB", status.MemoryCurrentMB, limit))
		}
		if limit, ok := lowered[settingSiteTasks]; ok && status.TasksCurrent > int64(limit) {
			parts = append(parts, fmt.Sprintf("%d processes, new limit %d", status.TasksCurrent, limit))
		}
		if len(parts) > 0 {
			lines = append(lines, names[status.SiteID]+": "+strings.Join(parts, "; "))
		}
	}
	sort.Strings(lines)
	preview.SitesStoppedCount = len(lines)
	if len(lines) > 40 {
		lines = append(lines[:40], fmt.Sprintf("+%d more", len(lines)-40))
	}
	preview.SitesStopped = lines
	if preview.SitesStoppedCount > 0 {
		preview.Message += fmt.Sprintf(" The programs of %d site(s) use more now than a lowered site limit; when this is saved the system stops them at once.", preview.SitesStoppedCount)
	}
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
