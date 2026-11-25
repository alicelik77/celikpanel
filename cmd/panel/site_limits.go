package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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

// siteMemoryHostDefault is the host-bound memory default for this much host
// memory: the smaller of 3072 MB and 75 % of it (rounded down to a whole MB),
// never under 64 MB. One site can then never take the whole host by default;
// a plan sets more where wanted.
func siteMemoryHostDefault(host int) int {
	value := siteSliceCodeDefaults[settingSiteMemoryMB]
	if host > 0 && host*3/4 < value {
		value = host * 3 / 4
	}
	if value < 64 {
		value = 64
	}
	return value
}

// The host-bound default is FIXED when it is first computed (review B5):
// stored in panel_settings (site_memory_default_v1, no schema change) with
// the host memory it was computed for, and read from there afterwards. A
// later change of the host's memory moves nothing by itself: the
// administrator reads one Problems now line (the default was computed for X,
// the host now has Y) whose one action applies the new default through the
// same preview and confirmation as any lowered limit
// (handleSiteMemoryDefault).
const settingSiteMemoryDefault = "site_memory_default_v1"

type siteMemoryDefaultState struct {
	Value  int    `json:"value"`
	HostMB int    `json:"host_mb"`
	At     string `json:"at"`
}

var (
	siteMemoryFrozenMu sync.Mutex
	siteMemoryFrozen   int // 0: not fixed yet (computed from the host)
)

// siteMemoryCodeDefault is a site's memory limit when no level sets one: the
// fixed value, or before it is fixed the host-bound value now.
func siteMemoryCodeDefault() int {
	siteMemoryFrozenMu.Lock()
	frozen := siteMemoryFrozen
	siteMemoryFrozenMu.Unlock()
	if frozen > 0 {
		return frozen
	}
	return siteMemoryHostDefault(hostMemoryMB())
}

func setSiteMemoryFrozen(value int) {
	siteMemoryFrozenMu.Lock()
	siteMemoryFrozen = value
	siteMemoryFrozenMu.Unlock()
}

// freezeSiteMemoryDefault runs once at the Panel's start: the stored default
// is used; the first start of a release with it computes and stores it.
func (p *Panel) freezeSiteMemoryDefault(ctx context.Context) {
	if p == nil || p.db == nil {
		return
	}
	var state siteMemoryDefaultState
	if raw := p.setting(ctx, settingSiteMemoryDefault); raw != "" && json.Unmarshal([]byte(raw), &state) == nil && state.Value >= 64 {
		setSiteMemoryFrozen(state.Value)
		return
	}
	host := hostMemoryMB()
	if host <= 0 {
		return // not known: computed again at the next start
	}
	state = siteMemoryDefaultState{Value: siteMemoryHostDefault(host), HostMB: host, At: metricsTime(time.Now())}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingSiteMemoryDefault, string(raw)); err != nil {
		log.Printf("site limits: the default memory limit was not stored: %v", err)
		return
	}
	setSiteMemoryFrozen(state.Value)
}

// siteMemoryDefaultProblem: ONE line, for the administrator, when the host's
// memory now gives another default than the fixed one, and only while the
// default is CelikPanel's (no server default of the administrator's own).
func (p *Panel) siteMemoryDefaultProblem(ctx context.Context) []monitoringProblem {
	state, next, ok := p.siteMemoryDefaultChange(ctx)
	if !ok {
		return nil
	}
	return []monitoringProblem{{
		Key: "site_memory_default", Kind: "site_memory_default", Severity: "warning", Actor: "administrator",
		Subject: strconv.Itoa(state.Value), Name: fmt.Sprintf("%d MB → %d MB", state.HostMB, hostMemoryMB()), Value: float64(next),
		ObservedAt: state.At, Action: monitoringProblemAction{Kind: "apply_default"},
	}}
}

// siteMemoryDefaultChange: the fixed default, the one the host's memory gives
// now, and whether they differ while CelikPanel's default applies.
func (p *Panel) siteMemoryDefaultChange(ctx context.Context) (siteMemoryDefaultState, int, bool) {
	var state siteMemoryDefaultState
	raw := p.setting(ctx, settingSiteMemoryDefault)
	if raw == "" || json.Unmarshal([]byte(raw), &state) != nil || state.Value < 64 {
		return state, 0, false
	}
	host := hostMemoryMB()
	if host <= 0 {
		return state, 0, false
	}
	policy, err := loadSettingPolicy(ctx, p.db.GetDB())
	if err != nil || policy[settingSiteMemoryMB].ServerDefault != nil {
		return state, 0, false
	}
	next := siteMemoryHostDefault(host)
	return state, next, next != state.Value
}

// errCodeSiteMemoryDefaultCurrent: the default already fits the host.
const errCodeSiteMemoryDefaultCurrent = "SITE_MEMORY_DEFAULT_CURRENT"

// handleSiteMemoryDefault is the action of the Problems now line of a
// default fixed for the host's earlier memory (review B5):
//
//	POST /api/v1/rights/site-memory-default {"action": "preview"}
//	POST /api/v1/rights/site-memory-default {"action": "apply", "confirm_stopped": "<token>"}
//
// Administrator only. The preview lists, as for any lowered limit, the sites
// whose memory limit comes from the default and whose programs use more now
// than the new one; "apply" with that list's token fixes the new default
// (for the host's memory now) and writes it into those sites' slices. A
// request without the token of the list the server answers now is refused
// with the list.
func (p *Panel) handleSiteMemoryDefault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rejectRouteMethod(w, []string{http.MethodPost})
		return
	}
	ctx := r.Context()
	if c := currentCaller(r); c == nil || !c.hasAccountRole(roleAdmin) {
		writeCodedError(w, http.StatusForbidden, errCodeAdminOnly, "only the administrator sets the server's limits", "")
		return
	}
	var req struct {
		Action         string `json:"action"`
		ConfirmStopped string `json:"confirm_stopped"`
	}
	if err := decodeStrictJSON(w, r, &req); err != nil || (req.Action != "preview" && req.Action != "apply") {
		writeClientError(w, http.StatusBadRequest, "action is preview or apply")
		return
	}
	state, next, ok := p.siteMemoryDefaultChange(ctx)
	if !ok {
		writeCodedError(w, http.StatusConflict, errCodeSiteMemoryDefaultCurrent,
			"Nothing was changed: the default memory limit of a site already fits the server's memory, or the administrator set a default of their own.", "")
		return
	}
	db := p.db.GetDB()
	rows, err := db.QueryContext(ctx, `SELECT id FROM subscriptions ORDER BY id`)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var all []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			all = append(all, id)
		}
	}
	rows.Close()
	policy, err := loadSettingPolicy(ctx, db)
	if err != nil {
		writeServerError(w, err)
		return
	}
	var subs []int
	for _, id := range all {
		limits, err := subscriptionSettingLimits(ctx, db, id)
		if err == nil && siteMemoryFromDefault(limits, policy) {
			subs = append(subs, id)
		}
	}
	preview := rightsPreview{Message: fmt.Sprintf("Sites without a memory limit of their own get %d MB instead of %d MB.", next, state.Value)}
	if next < state.Value {
		p.previewSiteLimitsLowered(ctx, &preview, subs, map[string]int{settingSiteMemoryMB: state.Value}, map[string]int{settingSiteMemoryMB: next})
	}
	if req.Action == "preview" {
		writeRightsJSON(w, http.StatusOK, map[string]any{"preview": preview})
		return
	}
	if !siteLimitsStoppedConfirmed(preview, req.ConfirmStopped) {
		writeRightsPreviewRefusal(w, errCodeRightsConfirm, preview)
		return
	}
	host := hostMemoryMB()
	raw, err := json.Marshal(siteMemoryDefaultState{Value: next, HostMB: host, At: metricsTime(time.Now())})
	if err == nil {
		err = p.setSetting(ctx, settingSiteMemoryDefault, string(raw))
	}
	if err != nil {
		writeServerError(w, err)
		return
	}
	setSiteMemoryFrozen(next)
	p.reapplySiteSlices(settingSiteMemoryMB, subs)
	p.auditDetailed(r, "settings.limit:"+settingSiteMemoryMB, "settings", 0, map[string]any{
		"key": settingSiteMemoryMB, "fixed_default": next, "was": state.Value, "host_mb": host, "was_host_mb": state.HostMB,
		"sites_stopped": preview.SitesStoppedCount,
	})
	writeRightsJSON(w, http.StatusOK, map[string]any{"success": true, "default": next})
}

// siteMemoryFromDefault: a subscription's site memory limit is the default:
// no level sets a ceiling, or the only one is the reseller's default bound
// (siteLimitsWithResellerBound; no server ceiling exists).
func siteMemoryFromDefault(limits settingLimits, policy map[string]settingPolicyRow) bool {
	limit, ok := limits[settingSiteMemoryMB]
	if !ok || limit.Ceiling == nil {
		return true
	}
	return limit.SetBy == settingSetByServer && policy[settingSiteMemoryMB].ServerCeiling == nil
}

// siteLimitsWithResellerBound (review: a reseller cannot starve the server
// by default): where neither the server nor the reseller's own level sets a
// ceiling for a site.* key, the reseller's field, and every plan, Custom set
// and site below it, is bounded by the server's default for that key (the
// host-bound one for memory). The administrator raises it for a reseller by
// giving the reseller (its reseller plan or Custom set) a ceiling; the bound
// reads "set by the server administrator".
func siteLimitsWithResellerBound(limits settingLimits) settingLimits {
	for _, key := range siteSliceKeys {
		limit, ok := limits[key]
		if !ok || limit.Ceiling != nil || limit.Default == nil {
			continue
		}
		limit.tighten(*limit.Default, settingSetByServer, "")
		limits[key] = limit
	}
	return limits
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

// siteLimitApplyMu serialises the writes of changed limits (review B3): a
// second change waits until the first is written, then reads the limits as
// they are at that moment, so two changes never interleave their batches.
var siteLimitApplyMu sync.Mutex

// siteSliceBatchTimeout bounds one batch of 100 sites (the Agent's verify
// of each written file, one reload and the runtime push): the deadline is
// per batch, so a change of many sites is never cut by one overall clock.
var siteSliceBatchTimeout = 2 * time.Minute

// siteLimitChangeLabel names a change of limits for the administrator's
// Problems now line: the plan's or reseller plan's name, the reseller's
// name, the subscription's first domain, or the server key.
func (p *Panel) siteLimitChangeLabel(ctx context.Context, ownerKind string, ownerID int) string {
	db := p.db.GetDB()
	var name string
	switch ownerKind {
	case rightsOwnerPlan:
		_ = db.QueryRowContext(ctx, `SELECT name FROM service_plans WHERE id = ?`, ownerID).Scan(&name)
	case rightsOwnerResellerPlan:
		_ = db.QueryRowContext(ctx, `SELECT name FROM reseller_plans WHERE id = ?`, ownerID).Scan(&name)
	case rightsOwnerReseller:
		_ = db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, ownerID).Scan(&name)
	case rightsOwnerSubscription:
		_ = db.QueryRowContext(ctx, `SELECT name FROM domains WHERE subscription_id = ? ORDER BY id LIMIT 1`, ownerID).Scan(&name)
	}
	if name == "" {
		return ownerKind + " " + strconv.Itoa(ownerID)
	}
	return name
}

// reapplySiteSlices writes the current limits into the slices of every site
// of the given subscriptions, in the background, one change at a time. A
// site without a slice file yet gets one when its first program goes into
// it. change names the change for the Problems now line when it does not
// reach every site.
func (p *Panel) reapplySiteSlices(change string, subscriptionIDs []int) {
	if len(subscriptionIDs) == 0 || (siteLimitReadNeedsAgent && p.agentClient == nil) {
		return
	}
	ids := append([]int(nil), subscriptionIDs...)
	startSiteSliceApply(func() {
		if err := p.reapplySiteSlicesNow(context.Background(), change, ids); err != nil {
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

// --- A change that did not reach every site (review B3) -----------------------

// settingSiteLimitApply keeps the subscriptions whose sites still have their
// earlier limits after a change of limits, how many sites, why, and which
// change it was (panel_settings; no schema change). It is cleared when every
// site has the new limits. The administrator reads it as one Problems now
// line; the hourly round (startSiteLimitApplyRound) writes those sites again.
const settingSiteLimitApply = "site_limit_apply_v1"

type siteLimitApplyState struct {
	Change string `json:"change"`
	// Pending: subscription id -> its sites that still have the earlier limits.
	Pending map[string]int `json:"pending"`
	Reason  string         `json:"reason"`
	At      string         `json:"at"`
}

func (p *Panel) loadSiteLimitApply(ctx context.Context) siteLimitApplyState {
	state := siteLimitApplyState{}
	if raw := p.setting(ctx, settingSiteLimitApply); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	return state
}

// noteSiteLimitApply records what one write of changed limits reached:
// notApplied counts, per subscription the write covered, the sites that kept
// their earlier limits (0: all have the new ones). Subscriptions the write
// did not cover keep what an earlier change left.
func (p *Panel) noteSiteLimitApply(ctx context.Context, change string, covered []int, notApplied map[int]int, reason string, now time.Time) {
	state := p.loadSiteLimitApply(ctx)
	next := siteLimitApplyState{Change: state.Change, Pending: map[string]int{}, Reason: state.Reason, At: state.At}
	for k, v := range state.Pending {
		next.Pending[k] = v
	}
	for _, id := range covered {
		delete(next.Pending, strconv.Itoa(id))
	}
	missed := 0
	for id, n := range notApplied {
		if n > 0 {
			next.Pending[strconv.Itoa(id)] = n
			missed += n
		}
	}
	if missed > 0 {
		next.Change, next.Reason, next.At = change, reason, metricsTime(now)
	}
	raw := ""
	if len(next.Pending) > 0 {
		encoded, err := json.Marshal(next)
		if err != nil {
			return
		}
		raw = string(encoded)
	}
	if raw == p.setting(ctx, settingSiteLimitApply) {
		return
	}
	if err := p.setSetting(ctx, settingSiteLimitApply, raw); err != nil {
		log.Printf("site limits: what a change of limits reached was not recorded: %v", err)
	}
}

// siteLimitApplyProblem is ONE line, for the administrator: which change did
// not reach every site, how many sites still have the earlier limits, why.
func siteLimitApplyProblem(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state siteLimitApplyState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	sites := 0
	for _, n := range state.Pending {
		sites += n
	}
	if sites == 0 {
		return nil
	}
	return []monitoringProblem{{
		Key: "site_limits_unapplied", Kind: "site_limits_unapplied", Severity: "warning", Actor: "administrator",
		Subject: state.Change, Name: state.Reason, Value: float64(sites), Since: state.At, ObservedAt: state.At,
		Action: monitoringProblemAction{Kind: "open", To: "/plans"},
	}}
}

// startSiteLimitApplyRound writes, every hour, the sites a change of limits
// did not reach (with the limits as they are then).
func (p *Panel) startSiteLimitApplyRound() {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			p.retrySiteLimitApply(context.Background())
		}
	}()
}

// retrySiteLimitApply writes the pending subscriptions' sites again.
func (p *Panel) retrySiteLimitApply(ctx context.Context) {
	if p.db == nil || (siteLimitReadNeedsAgent && p.agentClient == nil) {
		return
	}
	state := p.loadSiteLimitApply(ctx)
	var ids []int
	for key := range state.Pending {
		if id, err := strconv.Atoi(key); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	sort.Ints(ids)
	if err := p.reapplySiteSlicesNow(ctx, state.Change, ids); err != nil {
		log.Printf("site limits: the hourly round: %v", err)
	}
}

// reapplySiteSlicesNow is the work of reapplySiteSlices: one change at a
// time (siteLimitApplyMu); the limits are read once the change's turn has
// come; the sites go in batches with their own deadline; what did not reach
// a site (a failed or refused write, a batch the Agent did not answer or
// could not apply, the batches after it) is recorded for the Problems now
// line and the hourly round, never only logged.
func (p *Panel) reapplySiteSlicesNow(ctx context.Context, change string, subscriptionIDs []int) error {
	if p.db == nil {
		return nil
	}
	siteLimitApplyMu.Lock()
	defer siteLimitApplyMu.Unlock()
	db := p.db.GetDB()
	req := &transport.ApplySiteSlicesRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit)}
	subOf := map[int]int{}
	covered := []int{}
	notApplied := map[int]int{}
	reason := ""
	for _, subID := range subscriptionIDs {
		limits, err := subscriptionSettingLimits(ctx, db, subID)
		if err != nil {
			log.Printf("site limits of subscription %d: %v", subID, err)
			continue
		}
		covered = append(covered, subID)
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
				subOf[siteID] = subID
			}
		}
		rows.Close()
	}
	if len(req.Sites) == 0 {
		p.noteSiteLimitApply(ctx, change, covered, nil, "", time.Now())
		return nil
	}
	all := req.Sites
	// alpha.149 (lab O3): the recorded reason is a code the page words in its
	// own language; the English detail goes to the journal.
	missed := func(sites []transport.SiteSlice, code, why string) {
		for _, s := range sites {
			notApplied[subOf[s.SiteID]]++
		}
		log.Printf("site limits: %d sites keep their earlier limits for now: %s", len(sites), why)
		if reason == "" {
			reason = code
		}
	}
	defer siteLimitApplying.set(0, 0)
	for start := 0; start < len(all); start += siteSliceApplyBatch {
		end := start + siteSliceApplyBatch
		if end > len(all) {
			end = len(all)
		}
		siteLimitApplying.set(start, len(all))
		batch := &transport.ApplySiteSlicesRequest{ExpectedBuildCommit: req.ExpectedBuildCommit, Sites: all[start:end]}
		batchCtx, cancel := context.WithTimeout(ctx, siteSliceBatchTimeout)
		resp, err := applySiteSlicesCall(p, batchCtx, batch)
		cancel()
		if err != nil {
			// The rest is not sent: the hourly round writes it.
			missed(all[start:], "agent_unreachable", "the server did not answer: "+err.Error())
			break
		}
		if resp.Unsupported != "" {
			log.Printf("site limits: nothing was written: %s", resp.Unsupported)
			return nil // its own Problems now line says why
		}
		if resp.Error != "" {
			// The Agent refused the batch, or wrote it but systemd did not
			// take it (daemon-reload failed): its sites keep the earlier
			// limits.
			missed(all[start:end], "agent_refused", resp.Error)
			continue
		}
		for _, s := range resp.Sites {
			switch s.Outcome {
			case transport.SiteFileOutcomeKept:
				log.Printf("site limits: the slice of site %d was changed on the server and is kept; CelikPanel's text is in %s", s.SiteID, s.PendingPath)
			case transport.SiteFileOutcomeFailed, transport.SiteFileOutcomeRefused:
				notApplied[subOf[s.SiteID]]++
				log.Printf("site limits: site %d keeps its earlier limits for now: %s", s.SiteID, strings.TrimSpace(s.Outcome+" "+s.Detail))
				if reason == "" {
					reason = s.Reason
					if reason == "" {
						reason = "failed" // an Agent before alpha.149
					}
				}
			}
		}
		if len(all) > siteSliceApplyBatch {
			log.Printf("site limits: %d of %d sites written", end, len(all))
		}
	}
	p.noteSiteLimitApply(ctx, change, covered, notApplied, reason, time.Now())
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
	var items []siteStoppedItem
	for _, status := range statuses {
		if !status.Active {
			continue
		}
		var parts []string
		item := siteStoppedItem{Domain: names[status.SiteID]}
		if limit, ok := lowered[settingSiteMemoryMB]; ok && status.MemoryCurrentMB > int64(limit) {
			parts = append(parts, fmt.Sprintf("memory %d MB in use, new limit %d MB", status.MemoryCurrentMB, limit))
			item.MemoryMB, item.MemoryLimitMB = status.MemoryCurrentMB, limit
		}
		if limit, ok := lowered[settingSiteTasks]; ok && status.TasksCurrent > int64(limit) {
			parts = append(parts, fmt.Sprintf("%d processes, new limit %d", status.TasksCurrent, limit))
			item.Tasks, item.TasksLimit = status.TasksCurrent, limit
		}
		if len(parts) > 0 {
			lines = append(lines, names[status.SiteID]+": "+strings.Join(parts, "; "))
			items = append(items, item)
		}
	}
	sort.Strings(lines)
	sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
	preview.SitesStoppedCount = len(lines)
	preview.StoppedToken = siteLimitsStoppedToken(lines)
	if len(lines) > 40 {
		lines = append(lines[:40], fmt.Sprintf("+%d more", len(lines)-40))
		items = items[:40]
	}
	preview.SitesStopped, preview.SitesStoppedItems = lines, items
	if preview.SitesStoppedCount > 0 {
		preview.Message += fmt.Sprintf(" The programs of %d site(s) use more now than a lowered site limit: when this is saved, a program over the memory limit is stopped at once; at the process limit new processes are refused and running ones keep running.", preview.SitesStoppedCount)
	}
}

// siteStoppedItem is one site of the list (alpha.149): what its programs use
// now against a lowered limit (0: that key is not lowered below its use).
type siteStoppedItem struct {
	Domain        string `json:"domain"`
	MemoryMB      int64  `json:"memory_mb,omitempty"`
	MemoryLimitMB int    `json:"memory_limit_mb,omitempty"`
	Tasks         int64  `json:"tasks,omitempty"`
	TasksLimit    int    `json:"tasks_limit,omitempty"`
}

// siteLimitsStoppedToken names one list of affected sites (review finding 9
// leftovers): a change that lowers a site limit below what sites use now is
// saved only with the token of the list the server answers at that moment
// (confirm_stopped), so the confirmation is always the one that named the
// sites; a request without it, or with an earlier list's, is refused with
// the current list. The preview is enforced by the server, not only by the
// screen.
func siteLimitsStoppedToken(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:8])
}

// siteLimitsStoppedConfirmed: the request may save what its preview showed.
func siteLimitsStoppedConfirmed(preview rightsPreview, token string) bool {
	return preview.SitesStoppedCount == 0 || (token != "" && token == preview.StoppedToken)
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
