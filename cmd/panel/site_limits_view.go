package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/siteruntime"
	"github.com/alicelik/celikpanel/internal/transport"
)

// What people read about a site's limits (S4 R2a, D-024).
//
//   - GET /api/v1/domains/{id}/site-limits: the site's limits, whether they
//     apply at all, what systemd applies, what the site uses now, and when
//     it reached a limit (from the slice cgroup's counters, read with this
//     request and at most every five minutes in the background). Everyone
//     who sees the domain reads the limits and the events; the administrator
//     and a reseller also read who sets each limit by name; the administrator
//     alone reads which programs run inside the limits, the slice file and
//     the owner's drop-ins, the database users' own settings and the one
//     action that moves the rest. A reading writes the panel's record only
//     when something in it changed.
//   - POST /api/v1/domains/{id}/site-limits/move: the administrator's move of
//     the site's running programs into its limits (Agent.MoveSiteSliceV1).
//     One move at a time across the Panel; a repeated request id answers the
//     first answer.
//   - Problems now: a site that reached its OWN memory or process limit at
//     two or more readings in the last day; one line, for the administrator,
//     naming the sites whose programs still run outside their limits; one
//     line when this host cannot enforce a site's limits at all; one line
//     when a change of limits did not reach every site (site_limits.go); one
//     line when a site's default memory limit was fixed for the host's
//     earlier memory; and, once for the server, that disk limits are counted
//     but not enforced where the hosting file system has no project quota.
//
// Sitenin sınırları: herkes sınırları, uygulanıp uygulanmadığını ve ne zaman
// aşıldığını okur; yönetici hangi programların sınır içinde olduğunu da görür
// ve kalanları tek işlemle taşır.

// siteLimitUnit is one program of a site as the Panel knows it.
type siteLimitUnit struct {
	Unit  string `json:"unit"`
	Kind  string `json:"kind"` // node, program, service_app
	Label string `json:"label"`
}

// siteLimitSite is one site with its programs.
type siteLimitSite struct {
	SiteID         int
	DomainID       int
	Domain         string
	SubscriptionID int
	Units          []siteLimitUnit
}

// siteLimitSites lists sites with their programs (domainID 0: every site).
func (p *Panel) siteLimitSites(ctx context.Context, domainID int) ([]siteLimitSite, error) {
	db := p.db.GetDB()
	query := `SELECT s.id, d.id, d.name, d.subscription_id, COALESCE(s.project_type, 'php') FROM sites s JOIN domains d ON d.id = s.domain_id`
	args := []any{}
	if domainID > 0 {
		query += ` WHERE d.id = ?`
		args = append(args, domainID)
	}
	rows, err := db.QueryContext(ctx, query+` ORDER BY s.id`, args...)
	if err != nil {
		return nil, err
	}
	var sites []siteLimitSite
	index := map[int]int{}
	for rows.Next() {
		var s siteLimitSite
		var projectType string
		if err := rows.Scan(&s.SiteID, &s.DomainID, &s.Domain, &s.SubscriptionID, &projectType); err != nil {
			rows.Close()
			return nil, err
		}
		if projectType == "node" {
			s.Units = append(s.Units, siteLimitUnit{Unit: "celikapp-" + strconv.Itoa(s.SiteID) + ".service", Kind: "node", Label: "Node.js"})
		}
		index[s.SiteID] = len(sites)
		sites = append(sites, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	add := func(query, kind string, label func(id int, name string) string) error {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, siteID int
			var name string
			if err := rows.Scan(&id, &siteID, &name); err != nil {
				return err
			}
			if i, ok := index[siteID]; ok {
				sites[i].Units = append(sites[i].Units, siteLimitUnit{Unit: unitOf(kind, id), Kind: kind, Label: label(id, name)})
			}
		}
		return rows.Err()
	}
	if err := add(`SELECT id, site_id, COALESCE(kind, '') FROM runtime_apps ORDER BY id`, "program", func(id int, kind string) string {
		if kind == string(siteruntime.KindPython) {
			return "Python"
		}
		return "program"
	}); err != nil {
		return nil, err
	}
	if err := add(`SELECT id, site_id, app_id FROM service_app_installs ORDER BY id`, "service_app", func(id int, app string) string {
		if a, ok := serviceapps.Lookup(app); ok {
			return a.Name
		}
		return app
	}); err != nil {
		return nil, err
	}
	return sites, nil
}

func unitOf(kind string, id int) string {
	switch kind {
	case "program":
		return siteruntime.UnitName(id)
	case "service_app":
		return serviceapps.UnitName(id) + ".service"
	}
	return ""
}

// readSiteSlicesCall is the Agent call (tests replace it).
var readSiteSlicesCall = func(p *Panel, ctx context.Context, req *transport.SiteSlicesStatusRequest) (transport.SiteSlicesStatusResponse, error) {
	var resp transport.SiteSlicesStatusResponse
	err := p.callAgentContext(ctx, "Agent.SiteSlicesStatusV1", req, &resp)
	return resp, err
}

func siteSliceQueries(sites []siteLimitSite) *transport.SiteSlicesStatusRequest {
	req := &transport.SiteSlicesStatusRequest{Sites: []transport.SiteSliceQuery{}}
	for _, s := range sites {
		q := transport.SiteSliceQuery{SiteID: s.SiteID}
		for _, u := range s.Units {
			q.Units = append(q.Units, u.Unit)
		}
		req.Sites = append(req.Sites, q)
	}
	return req
}

// siteSliceReadPage is the number of sites per status read: well under the
// Agent's bound, so no reading is ever refused or cut for the number of
// sites (review finding 12). siteSliceReadPageUnits bounds the programs of
// one page (review B9): a site may have any number of programs; a page
// closes before it would pass the bound, and a site larger than the bound
// alone gets a page of its own (the Agent reads up to its own bound of
// 20000 and counts the rest as unread, never refusing the read).
const (
	siteSliceReadPage      = 1000
	siteSliceReadPageUnits = 4000
)

// readSiteSlicesPaged reads the sites' slices in pages and joins the
// answers. The host's Unsupported reason comes back with the statuses.
func (p *Panel) readSiteSlicesPaged(ctx context.Context, sites []siteLimitSite) ([]transport.SiteSliceStatus, error) {
	resp, err := p.readSiteSlicesAll(ctx, sites)
	return resp.Sites, err
}

func (p *Panel) readSiteSlicesAll(ctx context.Context, sites []siteLimitSite) (transport.SiteSlicesStatusResponse, error) {
	out := transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{}}
	for start := 0; start < len(sites); {
		end, units := start, 0
		for end < len(sites) && end-start < siteSliceReadPage && (end == start || units+len(sites[end].Units) <= siteSliceReadPageUnits) {
			units += len(sites[end].Units)
			end++
		}
		page := sites[start:end]
		start = end
		resp, err := readSiteSlicesCall(p, ctx, siteSliceQueries(page))
		if err == nil && resp.Error != "" {
			err = errorString(resp.Error)
		}
		if err != nil {
			return out, err
		}
		out.Sites = append(out.Sites, resp.Sites...)
		if resp.Unsupported != "" {
			out.Unsupported = resp.Unsupported
		}
		if resp.Partial != "" {
			out.Partial = resp.Partial
		}
	}
	return out, nil
}

// --- What happened: the counters' ledger ---------------------------------------

// settingSiteLimitEvents keeps, per site, the counters as the last recorded
// reading had them and when a limit was reached (panel_settings; no schema
// change). It is written only when it changes: a site seen for the first
// time, a limit reached, a counter that started again, a site gone.
const settingSiteLimitEvents = "site_limit_events_v1"

// siteLimitCPUSeconds is how long a site must have waited for the processor
// since the last recorded reading before it counts as slowed down.
const siteLimitCPUSeconds = 30

// siteLimitRepeatWindow and siteLimitRepeatHits: a limit reached at this many
// readings within the window is a Problem.
const (
	siteLimitRepeatWindow = 24 * time.Hour
	siteLimitRepeatHits   = 2
	// siteLimitHitEvery and siteLimitMaxHits bound the record (review B7):
	// at most one hit per kind every five minutes, so at most 288 a day, and
	// never more than that are kept.
	siteLimitHitEvery = 5 * time.Minute
	siteLimitMaxHits  = 288
)

// Event kinds. Only "memory" and "tasks" are the site's own limit (proved
// by the kernel's counters, sitesandbox.SliceCounters); "memory_other" and
// "tasks_other" are a program of the site stopped or refused where the
// kernel does not prove the site's limit (a program's own smaller limit, a
// host-wide shortage): the screens then never blame the plan.
//
// "tasks_peak" (alpha.149, lab D3): on a kernel without pids.events.local the
// slice's own pids.peak reached its pids.max since the last reading (the
// site's process limit, proved by the slice itself; how often is not known,
// so its count stays 0).
var siteLimitEventKinds = []string{"memory", "memory_other", "tasks", "tasks_peak", "tasks_other", "cpu"}

type siteLimitEvent struct {
	Count   int64    `json:"count"`
	FirstAt string   `json:"first_at"`
	LastAt  string   `json:"last_at"`
	Hits    []string `json:"hits,omitempty"`
	// Limit (alpha.149, lab CHECK 2 W3) is the site's limit for the event's
	// key that held at its last time (the kernel's value when it was read,
	// else what systemd applied; 0: not known or no limit). The card shows an
	// event with this value, or without one, never with today's.
	Limit int `json:"limit,omitempty"`
}

// siteLimitHeld is the site's limit of an event kind's key as the reading
// found it in force (0: not known or no limit).
func siteLimitHeld(status transport.SiteSliceStatus, kind string) int {
	held := status.Live
	if status.KernelState == transport.SiteSliceInForce || status.KernelState == transport.SiteSlicePending {
		held = status.Kernel
	}
	switch {
	case strings.HasPrefix(kind, "memory"):
		return held.MemoryMB
	case strings.HasPrefix(kind, "tasks"):
		return held.Tasks
	}
	return held.CPUPercent
}

type siteLimitRecord struct {
	Counters transport.SiteSliceCounters `json:"counters"`
	ReadAt   string                      `json:"read_at"`
	Events   map[string]*siteLimitEvent  `json:"events,omitempty"`
	// LimitChanged (alpha.149 fix round, F8): the site's limits were changed
	// when its counters could not be read first, so the next reading's
	// events carry no limit value (they may be from before the change).
	LimitChanged bool `json:"limit_changed,omitempty"`
}

type siteLimitLedger map[string]*siteLimitRecord

var siteLimitLedgerMu sync.Mutex

func (p *Panel) loadSiteLimitLedger(ctx context.Context) siteLimitLedger {
	ledger := siteLimitLedger{}
	if raw := p.setting(ctx, settingSiteLimitEvents); raw != "" {
		_ = json.Unmarshal([]byte(raw), &ledger)
	}
	return ledger
}

// observeSiteSlices adds what the counters say since the last recorded
// reading. The first reading of a site is its baseline (what happened before
// is not claimed with a time). A counter lower than before started again
// (the slice was made anew): its whole value is new. whole: the reading
// covered every site with programs, so a site not in it is gone from the
// ledger. It reports whether the ledger changed.
func observeSiteSlices(ledger siteLimitLedger, statuses []transport.SiteSliceStatus, whole bool, now time.Time) bool {
	changed := false
	at := metricsTime(now)
	seen := map[string]bool{}
	for _, status := range statuses {
		key := strconv.Itoa(status.SiteID)
		seen[key] = true
		if !status.CountersRead {
			continue
		}
		previous, known := ledger[key]
		current := status.Counters
		if !known {
			ledger[key] = &siteLimitRecord{Counters: current, ReadAt: at}
			changed = true
			continue
		}
		before := previous.Counters
		reset := false
		delta := func(now, was int64) int64 {
			if now < was {
				reset = true
				return now
			}
			return now - was
		}
		memory := delta(current.MemoryLimitHits, before.MemoryLimitHits)
		kills := delta(current.OOMKills, before.OOMKills)
		// Review C3: one "time" of the site's own memory limit is one
		// program the kernel stopped while that limit was reached: the
		// rise of the slice's oom_kill over a reading in which its own
		// memory.events.local oom rose (at least one), never the raw
		// oom count, which counts every failed reclaim (F3c: oom 10576
		// for one stopped program).
		if memory > 0 {
			memory = kills
			if memory < 1 {
				memory = 1
			}
		}
		rises := map[string]int64{
			"memory": memory,
			"tasks":  delta(current.TasksLimitHits, before.TasksLimitHits),
			// Tasks refused at a program's own limit, or where the kernel
			// does not say which level refused them.
			"tasks_other": delta(current.TasksOtherHits, before.TasksOtherHits),
		}
		if memory == 0 && kills > 0 {
			// Programs stopped for lack of memory, not at the site's limit.
			rises["memory_other"] = kills
		}
		// alpha.149 (lab D3; fix round F7): the slice's own peak reached its
		// limit since the last reading (a unit's counters may be gone with
		// its cgroup). pids.peak is a high-water mark that a changed limit
		// does not reset, so only when the limit is unchanged since that
		// reading, which had the field and was below the limit.
		peakRead := func(c transport.SiteSliceCounters) bool { return c.SlicePidsMax > 0 }
		peakAt := func(c transport.SiteSliceCounters) bool { return peakRead(c) && c.SlicePidsPeak >= c.SlicePidsMax }
		peak := peakRead(before) && peakRead(current) && current.SlicePidsMax == before.SlicePidsMax &&
			!peakAt(before) && peakAt(current) && rises["tasks"] == 0
		cpuWait := delta(current.CPUThrottledUSec, before.CPUThrottledUSec)
		if cpuWait >= siteLimitCPUSeconds*1_000_000 {
			rises["cpu"] = 1
		}
		// A changed limit, or a peak that reached it or fell below it (the
		// slice made anew), is the new baseline.
		rose := reset || peakRead(current) != peakRead(before) || current.SlicePidsMax != before.SlicePidsMax || peakAt(current) != peakAt(before)
		// alpha.149 fix round (F8): the limits changed without a reading at
		// that moment: what this reading finds may be from before the change,
		// so it carries no limit value (never today's).
		limitUnsure := previous.LimitChanged
		if limitUnsure {
			previous.LimitChanged, changed = false, true
		}
		for _, kind := range siteLimitEventKinds {
			n := rises[kind]
			if n <= 0 && !(kind == "tasks_peak" && peak) {
				continue
			}
			rose = true
			if previous.Events == nil {
				previous.Events = map[string]*siteLimitEvent{}
			}
			event := previous.Events[kind]
			if event == nil {
				event = &siteLimitEvent{FirstAt: at}
				previous.Events[kind] = event
			}
			if n > 0 {
				event.Count += n
			}
			switch {
			case kind == "tasks_peak":
				// The limit the peak reached, unchanged since the last reading.
				event.Limit = int(current.SlicePidsMax)
			case limitUnsure:
				event.Limit = 0
			default:
				event.Limit = siteLimitHeld(status, kind)
			}
			// Review B7: at most one hit per kind every siteLimitHitEvery,
			// and never more than siteLimitMaxHits kept: the record's size
			// does not grow with how often a card is read (the count stays
			// whole: every rise adds to it).
			last := time.Time{}
			if len(event.Hits) > 0 {
				last, _ = time.Parse(time.RFC3339, event.Hits[len(event.Hits)-1])
			}
			event.LastAt = at
			if last.IsZero() || now.Sub(last) >= siteLimitHitEvery {
				event.Hits = append(event.Hits, at)
			}
			if len(event.Hits) > siteLimitMaxHits {
				event.Hits = event.Hits[len(event.Hits)-siteLimitMaxHits:]
			}
		}
		if !rose {
			// Nothing reached: the earlier counters stay the baseline (the
			// processor's wait adds up until it counts), nothing is written.
			continue
		}
		for _, event := range previous.Events {
			kept := event.Hits[:0]
			for _, hit := range event.Hits {
				if t, err := time.Parse(time.RFC3339, hit); err == nil && now.Sub(t) <= siteLimitRepeatWindow {
					kept = append(kept, hit)
				}
			}
			event.Hits = kept
		}
		previous.Counters, previous.ReadAt = current, at
		changed = true
	}
	if whole {
		for key := range ledger {
			if !seen[key] {
				delete(ledger, key)
				changed = true
			}
		}
	}
	return changed
}

// noteSiteSlices records a reading (only when the ledger changed).
func (p *Panel) noteSiteSlices(ctx context.Context, statuses []transport.SiteSliceStatus, whole bool, now time.Time) siteLimitLedger {
	siteLimitLedgerMu.Lock()
	defer siteLimitLedgerMu.Unlock()
	ledger := p.loadSiteLimitLedger(ctx)
	if observeSiteSlices(ledger, statuses, whole, now) {
		if raw, err := json.Marshal(ledger); err == nil {
			if err := p.setSetting(ctx, settingSiteLimitEvents, string(raw)); err != nil {
				log.Printf("site limits: the reading was not recorded: %v", err)
			}
		}
	}
	return ledger
}

// --- The site's card --------------------------------------------------------------

type siteLimitEventView struct {
	Kind     string `json:"kind"`
	Count    int64  `json:"count"`
	FirstAt  string `json:"first_at"`
	LastAt   string `json:"last_at"`
	Repeated bool   `json:"repeated,omitempty"`
	// Limit (alpha.149): the limit that held then (0: not known).
	Limit int `json:"limit,omitempty"`
}

// siteUnitStoppedOutside: a stopped unit whose effective Slice= is the
// system slice (it would start outside the limits).
const siteUnitStoppedOutside = "stopped_outside"

// siteLimitSetByResellerBound (alpha.149): the server's default, which bounds
// the reseller's customers where neither the server nor the reseller set a
// ceiling (siteLimitsWithResellerBound).
const siteLimitSetByResellerBound = "server_default_bound"

type siteLimitUnitView struct {
	siteLimitUnit
	Placement string `json:"placement"`
	OwnMB     int    `json:"own_memory_mb,omitempty"`
	// Lower: its own memory limit is above the site's, so in the slice it
	// may use less than before.
	Lower bool `json:"lower,omitempty"`
	// Blocked says why the move cannot take it and who acts: "edited" (the
	// server owner changed its file), "older" (an earlier protection: its own
	// page's sandbox move comes first), "owner_slice" (an owner's drop-in
	// places it). Such a program is not counted as outside.
	Blocked string `json:"blocked,omitempty"`
}

// Applies: whether the site's limits hold for its programs.
const (
	siteLimitsApply       = "yes"         // at least one program runs, or would start, inside them
	siteLimitsOutside     = "outside"     // every program runs, or would start, outside them
	siteLimitsNoFile      = "no_file"     // the slice file is absent: nothing is enforced
	siteLimitsUnsupported = "unsupported" // this host cannot enforce them
	// Third fix round (review N3): "in force" is what the kernel says.
	siteLimitsPending = "pending" // written, but the running slice does not hold them yet
	siteLimitsUnknown = "unknown" // the running slice's cgroup could not be read
	// siteLimitsNotYet (review W1) is the one state a customer or reseller
	// reads for every state they cannot act on; the administrator reads the
	// reason and the step.
	siteLimitsNotYet = "not_yet"
)

type siteLimitsView struct {
	Known     bool   `json:"known"`
	ReadError string `json:"read_error,omitempty"`
	Domain    string `json:"domain"`
	Admin     bool   `json:"admin"`
	// Reseller: the reader is the site's provider (a reseller), who can raise
	// the limits within its own.
	Reseller bool             `json:"reseller,omitempty"`
	Limits   []siteLimitValue `json:"limits"`
	// Applies: yes, outside, no_file, unsupported (Unsupported says why).
	Applies     string `json:"applies,omitempty"`
	Unsupported string `json:"unsupported,omitempty"`
	// Live is what systemd applies now (the file and every drop-in; nil where
	// the limits do not apply); the card shows it beside the plan's value
	// when they differ. Usage now (when a program runs in the slice).
	Live            *transport.SiteSliceLimitsRead `json:"live,omitempty"`
	MemoryCurrentMB int64                          `json:"memory_current_mb,omitempty"`
	TasksCurrent    int64                          `json:"tasks_current,omitempty"`
	Events          []siteLimitEventView           `json:"events"`
	// Administrator only.
	SliceFile       string                           `json:"slice_file,omitempty"`
	Slice           string                           `json:"slice,omitempty"`
	DropIns         []string                         `json:"drop_ins,omitempty"`
	Units           []siteLimitUnitView              `json:"units,omitempty"`
	NeedsMove       int                              `json:"needs_move,omitempty"`
	Busy            bool                             `json:"busy,omitempty"`
	MoveInterrupted *transport.SiteSliceMoveRecovery `json:"move_interrupted,omitempty"`
	DBOwner         []dbOwnerSetting                 `json:"db_owner,omitempty"`
	ApplyDone       int                              `json:"apply_done,omitempty"`
	ApplyTotal      int                              `json:"apply_total,omitempty"`
	// Tasks (S4 R2b, administrator only): whether the site's scheduled tasks
	// run as timers in its limits.
	Tasks *siteTasksView `json:"tasks,omitempty"`
}

func (p *Panel) handleDomainSiteLimits(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Cache-Control", "private, no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	caller := currentCaller(r)
	if caller == nil {
		writeClientError(w, http.StatusForbidden, "access required")
		return
	}
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	site := sites[0]
	limits, err := subscriptionSettingLimits(ctx, p.db.GetDB(), site.SubscriptionID)
	if err != nil {
		writeServerError(w, err)
		return
	}
	admin := caller.Role == roleAdmin
	reseller := caller.Role == roleReseller
	view := siteLimitsView{Domain: site.Domain, Admin: admin, Reseller: reseller, Limits: []siteLimitValue{}, Events: []siteLimitEventView{}}
	policy, policyErr := loadSettingPolicy(ctx, p.db.GetDB())
	for _, key := range siteSliceKeys {
		value := siteLimitValueOf(limits, key)
		// alpha.149 (lab CHECK 2 W4): under a reseller the server's default
		// bounds the reseller's field as a "server" ceiling
		// (siteLimitsWithResellerBound); nobody set it. Named as the server's
		// default bounding the reseller, not as the administrator's own limit.
		if policyErr == nil && value.SetBy == settingSetByServer && policy[key].ServerCeiling == nil {
			value.SetBy, value.SetByName = siteLimitSetByResellerBound, ""
		}
		if !admin && !reseller {
			value.SetByName = ""
		}
		view.Limits = append(view.Limits, value)
	}
	if admin {
		view.DBOwner = p.dbOwnerSettingsOf(ctx, site.SubscriptionID)
		view.ApplyDone, view.ApplyTotal = siteLimitApplying.read()
	}
	resp, err := readSiteSlicesCall(p, ctx, siteSliceQueries(sites))
	if err == nil && resp.Error != "" {
		err = errorString(resp.Error)
	}
	if err != nil || len(resp.Sites) != 1 {
		view.ReadError = "the server did not answer"
		writeRightsJSON(w, http.StatusOK, view)
		return
	}
	status := resp.Sites[0]
	view.Known = true
	ledger := p.noteSiteSlices(ctx, resp.Sites, false, time.Now())
	if record := ledger[strconv.Itoa(site.SiteID)]; record != nil {
		view.Events = siteLimitEventViews(record, time.Now())
	}
	view.Applies = siteLimitsApplies(status, resp.Unsupported)
	if resp.Unsupported != "" {
		view.Unsupported = resp.Unsupported
	}
	switch view.Applies {
	case siteLimitsApply:
		// What systemd applies (the file and every drop-in): the card shows
		// it beside the plan's value when they differ (review B3, C6).
		live := status.Live
		view.Live = &live
		if status.Active {
			view.MemoryCurrentMB, view.TasksCurrent = status.MemoryCurrentMB, status.TasksCurrent
		}
	case siteLimitsPending:
		// Review N3: what the kernel holds now, not the values written.
		kernel := status.Kernel
		view.Live = &kernel
	}
	if !admin && view.Applies != siteLimitsApply {
		// Review W1: a customer or reseller cannot act on any of these; they
		// read one neutral state, the administrator the reason and the step.
		view.Applies, view.Unsupported, view.Live = siteLimitsNotYet, "", nil
	}
	if admin {
		view.SliceFile, view.Slice, view.DropIns, view.MoveInterrupted = status.File, status.Slice, status.DropIns, status.MoveInterrupted
		memory := siteLimitValueOf(limits, settingSiteMemoryMB).Value
		placements := map[string]transport.SiteSliceUnit{}
		for _, u := range status.Units {
			placements[u.Unit] = u
		}
		for _, u := range site.Units {
			found, ok := placements[u.Unit]
			unit := siteLimitUnitView{siteLimitUnit: u, Placement: transport.SiteUnitAbsent, OwnMB: found.Own.MemoryMB}
			if ok {
				unit.Placement = siteUnitPlacement(found, status.Slice)
				unit.Blocked = siteUnitBlocked(found)
			}
			unit.Lower = found.Own.MemoryMB > memory || (found.Own.MemoryMB == 0 && unit.Placement != transport.SiteUnitAbsent && unit.Placement != transport.SiteUnitInSlice)
			view.Units = append(view.Units, unit)
		}
		// The move brings these programs in, or (the file gone) writes the
		// file again: the action the card offers does what it says.
		view.NeedsMove = siteUnitsOutside(status)
		view.Busy = siteMoveBusy()
		if resp.Unsupported == "" {
			view.Tasks = p.siteTasksViewOf(ctx, site.SiteID)
			if !view.Tasks.Unknown {
				outside, inside := map[int]bool{}, map[int]bool{}
				if view.Tasks.Moved {
					inside[site.SiteID] = true
				} else {
					outside[site.SiteID] = true
				}
				p.noteSiteTasksOutside(ctx, outside, inside, time.Now())
			}
		}
	}
	// A reader's reading keeps the administrator's Problems now line current
	// (written only when the list changed).
	p.noteSiteLimitOutside(ctx, resp.Sites, false, time.Now())
	p.noteSiteMoveKept(ctx, resp.Sites, false)
	writeRightsJSON(w, http.StatusOK, view)
}

// siteLimitsApplies says whether a site's limits hold for its programs: the
// host can enforce them, the slice file is there (an absent file means
// systemd runs the slice without limits), and at least one program runs or
// would start inside them. Review B2: an absent file is "no_file" (the file
// was there and is gone; the next change of limits, and the move, write it
// again) only when a unit names the slice or runs in it; otherwise the
// site's programs were never moved into their limits (every existing site
// right after the update): "outside".
func siteLimitsApplies(status transport.SiteSliceStatus, unsupported string) string {
	if unsupported != "" {
		return siteLimitsUnsupported
	}
	if status.File == transport.SiteFileAbsent {
		if status.Named {
			return siteLimitsNoFile
		}
		return siteLimitsOutside
	}
	inSlice, stopped := false, false
	for _, u := range status.Units {
		switch siteUnitPlacement(u, status.Slice) {
		case transport.SiteUnitInSlice:
			inSlice = true
		case transport.SiteUnitStopped:
			stopped = true // it starts inside them
		}
	}
	if !inSlice && !stopped {
		return siteLimitsOutside
	}
	// Review N3: where the slice runs, the limits are in force only when its
	// cgroup holds what systemd applies; a slice that does not run takes the
	// file when a program starts. Anything not read is not "in force".
	switch status.KernelState {
	case transport.SiteSliceInForce:
		return siteLimitsApply
	case transport.SiteSlicePending:
		return siteLimitsPending
	case transport.SiteSliceKernelUnknown:
		return siteLimitsUnknown
	}
	if inSlice {
		return siteLimitsUnknown // a program runs in the slice but its cgroup was not read
	}
	return siteLimitsApply
}

// siteUnitPlacement is where a unit runs as the card says it; a stopped unit
// whose effective Slice= (the file and its drop-ins, as systemd shows it) is
// the system slice would start outside the limits (stopped_outside); one
// whose drop-in names another slice is the owner's (elsewhere).
func siteUnitPlacement(u transport.SiteSliceUnit, slice string) string {
	if u.Placement == "" {
		return transport.SiteUnitAbsent
	}
	if u.Placement == transport.SiteUnitStopped && u.Slice != "" && u.Slice != slice {
		if u.Slice == "system.slice" && u.DropInSlice == "" {
			return siteUnitStoppedOutside
		}
		return transport.SiteUnitElsewhere
	}
	return u.Placement
}

// siteUnitBlocked: why the move cannot take this unit ("" when it can).
func siteUnitBlocked(u transport.SiteSliceUnit) string {
	switch {
	case u.DropInSlice != "":
		return "owner_slice"
	case u.File == "edited" || u.File == transport.SiteFileUnreadable:
		return "edited"
	case u.File == "older":
		return "older"
	}
	return ""
}

// siteUnitNeedsMove: the administrator's move brings this unit into the
// site's limits (it runs, or would start, in the system slice, and nothing
// keeps the move from it).
func siteUnitNeedsMove(placement, blocked string) bool {
	return blocked == "" && (placement == transport.SiteUnitOutside || placement == siteUnitStoppedOutside)
}

// siteUnitsOutside counts a site's programs outside its limits that the
// move takes care of: the ones it brings in and, where the slice file is
// gone while units use the slice (nothing limits them; review B2), the ones
// inside the slice too (the move writes the file again).
func siteUnitsOutside(status transport.SiteSliceStatus) int {
	n := 0
	noFile := status.File == transport.SiteFileAbsent && status.Named
	for _, u := range status.Units {
		placement := siteUnitPlacement(u, status.Slice)
		if siteUnitNeedsMove(placement, siteUnitBlocked(u)) ||
			(noFile && (placement == transport.SiteUnitInSlice || placement == transport.SiteUnitStopped)) {
			n++
		}
	}
	return n
}

type errorString string

func (e errorString) Error() string { return string(e) }

func siteLimitEventViews(record *siteLimitRecord, now time.Time) []siteLimitEventView {
	out := []siteLimitEventView{}
	for kind, event := range record.Events {
		recent := 0
		for _, hit := range event.Hits {
			if t, err := time.Parse(time.RFC3339, hit); err == nil && now.Sub(t) <= siteLimitRepeatWindow {
				recent++
			}
		}
		out = append(out, siteLimitEventView{Kind: kind, Count: event.Count, FirstAt: event.FirstAt, LastAt: event.LastAt,
			Repeated: recent >= siteLimitRepeatHits, Limit: event.Limit})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastAt > out[j].LastAt })
	return out
}

// --- The administrator's move --------------------------------------------------

var (
	siteMoveMu      sync.Mutex
	siteMoveRunning bool
	siteMoveAnswers = map[string]transport.MoveSiteSliceResponse{}
	siteMoveOrder   []string
)

func siteMoveBusy() bool {
	siteMoveMu.Lock()
	defer siteMoveMu.Unlock()
	return siteMoveRunning
}

// siteMoveUpdateInProgress (alpha.149 fix round, F6): an update this panel
// admitted is still queued or running by the agent's own status (a seam for
// tests).
var siteMoveUpdateInProgress = func(p *Panel, ctx context.Context) bool {
	return p.admittedPanelUpdateInProgress(ctx)
}

// siteMovePressBudget is the Agent's own budget of one press (its
// siteMoveBudget); siteMovePressBudgetLeft is what is left of it once the
// site's scheduled tasks moved first in the same press (alpha.150: one
// press, one deadline). At least one second: the Agent's own time rule then
// moves no program and says that the time ran out.
const siteMovePressBudget = 11 * time.Minute

func siteMovePressBudgetLeft(started time.Time) int {
	left := int((siteMovePressBudget - time.Since(started)) / time.Second)
	if left < 1 {
		left = 1
	}
	return left
}

// siteMoveClaimRefusal: a refusal of the tasks part that came before
// anything was changed and that the programs part would answer the same way
// (alpha.150: main's codes for a kept record, the panel's update, another
// move and a site being changed).
func siteMoveClaimRefusal(code string) bool {
	switch code {
	case transport.SiteMoveRefusalKept, "update_running", "update_unreadable", "another_move", "site_busy":
		return true
	}
	return false
}

// moveSiteSliceCall is the Agent call (tests replace it).
var moveSiteSliceCall = func(p *Panel, ctx context.Context, req *transport.MoveSiteSliceRequest) (transport.MoveSiteSliceResponse, error) {
	var resp transport.MoveSiteSliceResponse
	err := p.callAgentContext(ctx, "Agent.MoveSiteSliceV1", req, &resp)
	return resp, err
}

func (p *Panel) handleDomainSiteLimitsMove(w http.ResponseWriter, r *http.Request, domainID int) {
	w.Header().Set("Content-Type", "application/json")
	if c := currentCaller(r); c == nil || c.Role != roleAdmin {
		writeClientError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var req struct {
		RequestID string `json:"request_id"`
		// Recover (review N2): "Try again" for a move whose put-back did not
		// complete; the Agent runs the put-back now and moves nothing new.
		Recover bool `json:"recover"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || !refreshRequestIDPattern.MatchString(req.RequestID) {
		writeClientError(w, http.StatusBadRequest, "a request id is required")
		return
	}
	siteMoveMu.Lock()
	if answer, ok := siteMoveAnswers[req.RequestID]; ok {
		siteMoveMu.Unlock()
		_ = json.NewEncoder(w).Encode(answer)
		return
	}
	if siteMoveRunning {
		siteMoveMu.Unlock()
		writeServiceAppRefusal(w, http.StatusConflict, "site_limits_move_running",
			"Nothing new was started: a site's programs are being moved into its limits now. This page updates by itself when it is done.", nil)
		return
	}
	siteMoveRunning = true
	siteMoveMu.Unlock()
	defer func() {
		siteMoveMu.Lock()
		siteMoveRunning = false
		siteMoveMu.Unlock()
	}()

	ctx, cancel := hostingDurableContext(r.Context())
	defer cancel()
	// alpha.149 fix round (F6): never while the panel's own update is
	// admitted or running (it would stop the agent in the middle of the
	// move). The update refuses itself while this move runs (siteMoveBusy);
	// the agent refuses both ways too.
	if siteMoveUpdateInProgress(p, ctx) {
		writeServiceAppRefusal(w, http.StatusConflict, "site_limits_update_running",
			"Nothing was moved: the panel's update is in progress. Move the site after the update has finished.", nil)
		return
	}
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	site := sites[0]
	resp := transport.MoveSiteSliceResponse{Units: []transport.SiteSliceMoved{}, Done: true}
	var tasks *transport.SiteTasksMoveResponse
	if req.Recover {
		// Try again (review N2; alpha.150 one record): the Agent settles the
		// kept record, of the site's programs or of its scheduled tasks, and
		// moves nothing new.
		move := &transport.MoveSiteSliceRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: req.RequestID, Recover: true}
		var err error
		if resp, err = moveSiteSliceCall(p, ctx, move); err != nil {
			writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
				"The server did not answer, so the result is unknown. Read this page again before you try again.", nil)
			return
		}
	} else {
		slice := p.siteSliceForSubscription(ctx, site.SiteID, site.SubscriptionID, site.Domain)
		if slice == nil {
			writeServiceAppRefusal(w, http.StatusConflict, "site_limits_nothing",
				"Nothing was changed: this site's limits could not be read.", nil)
			return
		}
		// S4 R2b: the site's scheduled tasks first (no restart; nothing
		// half-moved), then its programs. A site without programs moves its
		// tasks only. One press, one deadline (alpha.150): the programs part
		// gets what is left of the move's budget.
		pressStarted := time.Now()
		var tasksErr error
		tasks, tasksErr = p.moveSiteTasks(ctx, site, req.RequestID, false)
		if tasksErr != nil {
			tasks = &transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown, Error: "the server did not answer about the scheduled tasks; read this page again"}
		}
		// A tasks part whose result is unknown (it may have left its record),
		// or that was refused before it began (a kept record, the panel's
		// update, another move, the site being changed), does not start the
		// programs part: it would be refused the same way, or overwrite the
		// only record of the crontab.
		switch {
		case tasks.Outcome == transport.SiteTasksUnknown:
			resp.Done, resp.Kept = false, tasks.Recovery != nil
			resp.Recovery, resp.Reason = tasks.Recovery, &transport.SiteMoveReason{Code: "not_finished"}
			resp.Error = tasks.Error
			if resp.Error == "" {
				resp.Error = "the result of the scheduled tasks' move is not known, so the programs were not moved; read this page again"
			}
		case siteMoveClaimRefusal(tasks.Refusal):
			resp.Done, resp.Kept = false, tasks.Refusal == transport.SiteMoveRefusalKept
			resp.Reason, resp.Error = &transport.SiteMoveReason{Code: tasks.Refusal}, tasks.Error
		case len(site.Units) > 0:
			move := &transport.MoveSiteSliceRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: req.RequestID,
				Site: *slice, SubscriptionID: site.SubscriptionID, DomainID: site.DomainID,
				BudgetSeconds: siteMovePressBudgetLeft(pressStarted)}
			for _, u := range site.Units {
				move.Units = append(move.Units, u.Unit)
			}
			var err error
			if resp, err = moveSiteSliceCall(p, ctx, move); err != nil {
				writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
					"The server did not answer, so the result is unknown. Read this page again before you try again.", nil)
				return
			}
		}
		resp.Tasks = tasks
		switch tasks.Outcome {
		case transport.SiteTasksMoved, transport.SiteTasksAlready, transport.SiteTasksCrontabBack:
			p.noteSiteTasksOutside(ctx, nil, map[int]bool{site.SiteID: true}, time.Now())
		}
	}
	tasksOutcome, tasksError, tasksBlocking := "", "", 0
	if tasks != nil {
		tasksOutcome, tasksError, tasksBlocking = tasks.Outcome, tasks.Error, len(tasks.Blocking)
	}
	p.auditDetailed(r, "site.limits_move", "domain", domainID, map[string]any{
		"request_id": req.RequestID, "site": site.Domain, "done": resp.Done, "rolled_back": resp.RolledBack,
		"units": resp.Units, "error": resp.Error, "wait_seconds": resp.WaitSeconds, "recover": req.Recover, "kept": resp.Kept,
		"recovery": resp.Recovery, "tasks": tasksOutcome, "tasks_error": tasksError, "tasks_blocking": tasksBlocking,
	})
	siteMoveMu.Lock()
	siteMoveAnswers[req.RequestID] = resp
	siteMoveOrder = append(siteMoveOrder, req.RequestID)
	if len(siteMoveOrder) > 32 {
		delete(siteMoveAnswers, siteMoveOrder[0])
		siteMoveOrder = siteMoveOrder[1:]
	}
	siteMoveMu.Unlock()
	_ = json.NewEncoder(w).Encode(resp)
}

// --- Problems now -----------------------------------------------------------------

// siteLimitReading is the background reading of every site's slice: at most
// every five minutes, one Agent call per page, never two readings at once.
type siteLimitReading struct {
	mu         sync.Mutex
	at         time.Time
	failedAt   time.Time
	refreshing bool
}

var siteLimitProblemReading siteLimitReading

const siteLimitReadFresh = 5 * time.Minute

var startSiteLimitRead = func(run func()) { go run() }

// siteLimitReadNeedsAgent: the background reading needs the Agent's client
// (tests that replace readSiteSlicesCall turn it off).
var siteLimitReadNeedsAgent = true

// siteLimitProblems: a site that reached its own memory or process limit at
// two or more readings in the last day, the sites still to move, and a host
// that cannot enforce a site's limits. The reading itself starts in the
// background when the last one is older than five minutes.
func (p *Panel) siteLimitProblems(ctx context.Context, now time.Time) []monitoringProblem {
	c := &siteLimitProblemReading
	c.mu.Lock()
	due := (c.at.IsZero() || now.Sub(c.at) >= siteLimitReadFresh) && (c.failedAt.IsZero() || now.Sub(c.failedAt) >= siteLimitReadFresh) && !c.refreshing
	if siteLimitReadNeedsAgent && p.agentClient == nil {
		due = false // nothing to ask (a Panel without its Agent)
	}
	if due {
		c.refreshing = true
	}
	c.mu.Unlock()
	if due {
		startSiteLimitRead(func() {
			readCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			ok := p.readAllSiteSlices(readCtx)
			c.mu.Lock()
			defer c.mu.Unlock()
			c.refreshing = false
			if ok {
				c.at, c.failedAt = time.Now(), time.Time{}
			} else {
				c.failedAt = time.Now()
			}
		})
	}
	siteLimitLedgerMu.Lock()
	ledger := p.loadSiteLimitLedger(ctx)
	siteLimitLedgerMu.Unlock()
	outside := p.setting(ctx, settingSiteLimitOutside)
	host := p.setting(ctx, settingSiteLimitHost)
	kept := p.setting(ctx, settingSiteMoveKept)
	// Review B3 and B5: a change of limits that did not reach every site,
	// and a default memory limit fixed for the host's earlier memory.
	extra := append(siteLimitApplyProblem(p.setting(ctx, settingSiteLimitApply)), p.siteMemoryDefaultProblem(ctx)...)
	if len(ledger) == 0 && outside == "" && host == "" && kept == "" {
		return extra
	}
	names := map[string]string{}
	if rows, err := p.db.GetDB().QueryContext(ctx, `SELECT s.id, d.name FROM sites s JOIN domains d ON d.id = s.domain_id`); err == nil {
		for rows.Next() {
			var id int
			var name string
			if rows.Scan(&id, &name) == nil {
				names[strconv.Itoa(id)] = name
			}
		}
		rows.Close()
	}
	var problems []monitoringProblem
	for site, record := range ledger {
		name, ok := names[site]
		if !ok {
			continue
		}
		for _, kind := range []string{"memory", "tasks"} {
			event := record.Events[kind]
			if event == nil {
				continue
			}
			var recent []string
			for _, hit := range event.Hits {
				if t, err := time.Parse(time.RFC3339, hit); err == nil && now.Sub(t) <= siteLimitRepeatWindow {
					recent = append(recent, hit)
				}
			}
			if len(recent) < siteLimitRepeatHits {
				continue
			}
			problems = append(problems, monitoringProblem{
				Key: "site_limit:" + site + ":" + kind, Kind: "site_" + kind + "_repeated", Severity: "warning", Actor: "administrator",
				Subject: name, Name: kind, Value: float64(len(recent)), Since: recent[0], ObservedAt: event.LastAt,
				Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(name)},
			})
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Key < problems[j].Key })
	problems = append(problems, siteMoveKeptProblem(kept, p.setting(ctx, settingSiteMoveStopped), names)...)
	problems = append(problems, siteLimitHostProblem(host)...)
	problems = append(problems, extra...)
	return append(problems, siteLimitsOutsideProblem(outside, names)...)
}

// --- A move whose put-back did not complete (review N2) ----------------------------

// settingSiteMoveKept keeps the sites whose move record the Agent keeps (its
// put-back did not complete; no site is moved until it is settled), as the
// last reading said: site id -> since when (panel_settings; no schema
// change). Written only when the list changes.
const settingSiteMoveKept = "site_move_kept_v1"

var siteMoveKeptMu sync.Mutex

// noteSiteMoveKept records a reading: whole (the background reading of every
// site) replaces the list; one site's card changes that site only.
func (p *Panel) noteSiteMoveKept(ctx context.Context, statuses []transport.SiteSliceStatus, whole bool) {
	siteMoveKeptMu.Lock()
	defer siteMoveKeptMu.Unlock()
	raw := p.setting(ctx, settingSiteMoveKept)
	previous := map[string]string{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &previous)
	}
	next := map[string]string{}
	if !whole {
		for k, v := range previous {
			next[k] = v
		}
	}
	stoppedRaw := p.setting(ctx, settingSiteMoveStopped)
	stoppedBefore := map[string]string{}
	if stoppedRaw != "" {
		_ = json.Unmarshal([]byte(stoppedRaw), &stoppedBefore)
	}
	stopped := map[string]string{}
	if !whole {
		for k, v := range stoppedBefore {
			stopped[k] = v
		}
	}
	for _, status := range statuses {
		key := strconv.Itoa(status.SiteID)
		delete(stopped, key)
		if status.MoveInterrupted != nil && status.MoveInterrupted.Kept {
			next[key] = status.MoveInterrupted.At
			if units := siteMoveStoppedUnits(status.MoveInterrupted); units != "" {
				stopped[key] = units
			}
		} else {
			delete(next, key)
		}
	}
	if !reflect.DeepEqual(stopped, stoppedBefore) {
		value := ""
		if len(stopped) > 0 {
			if encoded, err := json.Marshal(stopped); err == nil {
				value = string(encoded)
			}
		}
		if err := p.setSetting(ctx, settingSiteMoveStopped, value); err != nil {
			log.Printf("site limits: the programs not running after a move were not recorded: %v", err)
		}
	}
	if reflect.DeepEqual(next, previous) {
		return
	}
	value := ""
	if len(next) > 0 {
		encoded, err := json.Marshal(next)
		if err != nil {
			return
		}
		value = string(encoded)
	}
	if err := p.setSetting(ctx, settingSiteMoveKept, value); err != nil {
		log.Printf("site limits: the unfinished move was not recorded: %v", err)
	}
}

// settingSiteMoveStopped (alpha.149, lab D6) keeps, per site whose move
// record is kept, its programs that were put back with their earlier file but
// do not run (recovery outcome "failed"): site id -> unit names (comma
// separated). Written only when it changes.
const settingSiteMoveStopped = "site_move_stopped_v1"

// siteMoveStoppedUnits names a kept record's programs that do not run.
func siteMoveStoppedUnits(record *transport.SiteSliceMoveRecovery) string {
	var units []string
	for unit, state := range record.Units {
		if state == transport.SiteMoveFailed {
			units = append(units, unit)
		}
	}
	sort.Strings(units)
	return strings.Join(units, ", ")
}

// siteMoveKeptProblem is ONE line, for the administrator, while the Agent
// keeps a move's record: a program may be stopped and no site is moved until
// it is settled; it opens the site, whose card names each program left, the
// next step and "Try again". alpha.149 (lab D6): where a program was put back
// but does not run, the line names it (Name: the programs), so it says that
// it is not running now, who acts and the next step.
func siteMoveKeptProblem(raw, stoppedRaw string, names map[string]string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	kept := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &kept); err != nil {
		return nil
	}
	stopped := map[string]string{}
	if stoppedRaw != "" {
		_ = json.Unmarshal([]byte(stoppedRaw), &stopped)
	}
	var sites, programs []string
	first := ""
	since := ""
	for key, at := range kept {
		if name := names[key]; name != "" {
			sites = append(sites, name)
			if since == "" || at < since {
				since = at
			}
			if units := stopped[key]; units != "" {
				programs = append(programs, units)
				if first == "" || name < first {
					first = name
				}
			}
		}
	}
	if len(sites) == 0 {
		return nil
	}
	sort.Strings(sites)
	sort.Strings(programs)
	open := sites[0]
	if first != "" {
		open = first // the site of a program that does not run first
	}
	return []monitoringProblem{{
		Key: "site_move_kept", Kind: "site_move_kept", Severity: "warning", Actor: "administrator",
		Subject: strings.Join(sites, ", "), Name: strings.Join(programs, ", "), Value: float64(len(sites)), Since: since, ObservedAt: since,
		Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(open)},
	}}
}

// --- A host that cannot enforce a site's limits -------------------------------------

// settingSiteLimitHost keeps why this host cannot enforce a site's limits, as
// the last reading said ("" or absent: it can).
const settingSiteLimitHost = "site_limit_host_v1"

type siteLimitHost struct {
	Reason     string `json:"reason"`
	ObservedAt string `json:"observed_at"`
}

// noteSiteLimitHost records the Agent's answer when it changed.
func (p *Panel) noteSiteLimitHost(ctx context.Context, reason string, now time.Time) {
	raw := p.setting(ctx, settingSiteLimitHost)
	var state siteLimitHost
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	if state.Reason == reason {
		return
	}
	next := ""
	if reason != "" {
		encoded, err := json.Marshal(siteLimitHost{Reason: reason, ObservedAt: metricsTime(now)})
		if err != nil {
			return
		}
		next = string(encoded)
	}
	if err := p.setSetting(ctx, settingSiteLimitHost, next); err != nil {
		log.Printf("site limits: the host's answer was not recorded: %v", err)
	}
}

// siteLimitHostProblem: ONE line, for the administrator, when the host
// cannot enforce a site's limits, and why.
func siteLimitHostProblem(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state siteLimitHost
	if err := json.Unmarshal([]byte(raw), &state); err != nil || state.Reason == "" {
		return nil
	}
	return []monitoringProblem{{
		Key: "site_limits_unsupported", Kind: "site_limits_unsupported", Severity: "warning", Actor: "administrator",
		Subject: state.Reason, Name: state.Reason, ObservedAt: state.ObservedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/plans"},
	}}
}

// --- Sites whose programs still run outside their limits ---------------------

// settingSiteLimitOutside keeps, per site, how many of its programs run (or
// would start) outside its limits, as the last reading said (panel_settings;
// no schema change). It is written only when the list changes.
const settingSiteLimitOutside = "site_limit_outside_v1"

type siteLimitOutside struct {
	CheckedAt string         `json:"checked_at"`
	Sites     map[string]int `json:"sites,omitempty"`
	// Tasks (S4 R2b): sites whose scheduled tasks still run in cron.
	Tasks map[string]bool `json:"tasks,omitempty"`
}

var siteLimitOutsideMu sync.Mutex

// noteSiteLimitOutside records a reading: whole (the background reading of
// every site) replaces the list; one site's card changes that site only.
func (p *Panel) noteSiteLimitOutside(ctx context.Context, statuses []transport.SiteSliceStatus, whole bool, now time.Time) {
	siteLimitOutsideMu.Lock()
	defer siteLimitOutsideMu.Unlock()
	raw := p.setting(ctx, settingSiteLimitOutside)
	previous := siteLimitOutside{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &previous)
	}
	state := siteLimitOutside{Sites: map[string]int{}, Tasks: previous.Tasks}
	if !whole {
		for k, v := range previous.Sites {
			state.Sites[k] = v
		}
	}
	for _, status := range statuses {
		key := strconv.Itoa(status.SiteID)
		if n := siteUnitsOutside(status); n > 0 {
			state.Sites[key] = n
		} else {
			delete(state.Sites, key)
		}
	}
	if len(state.Sites) == 0 {
		state.Sites = nil
	}
	if reflect.DeepEqual(state.Sites, previous.Sites) || (len(state.Sites) == 0 && len(previous.Sites) == 0) {
		return // nothing changed: nothing is written
	}
	state.CheckedAt = metricsTime(now)
	next, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingSiteLimitOutside, string(next)); err != nil {
		log.Printf("site limits: the programs outside their limits were not recorded: %v", err)
	}
}

// readAllSiteSlices reads the slice of every site that has programs, in
// pages, and records it.
func (p *Panel) readAllSiteSlices(ctx context.Context) bool {
	all, err := p.siteLimitSites(ctx, 0)
	if err != nil {
		log.Printf("site limits: the sites could not be read: %v", err)
		return false
	}
	var sites []siteLimitSite
	for _, s := range all {
		if len(s.Units) > 0 {
			sites = append(sites, s)
		}
	}
	now := time.Now()
	if len(sites) == 0 {
		p.noteSiteSlices(ctx, nil, true, now)
		p.noteSiteLimitOutside(ctx, nil, true, now)
		p.readAllSiteTasks(ctx, all, "", now)
		return true
	}
	resp, err := p.readSiteSlicesAll(ctx, sites)
	if err != nil {
		if strings.Contains(err.Error(), "can't find method") {
			return true // an Agent older than this Panel: nothing to read until it is updated
		}
		log.Printf("site limits: the slices could not be read: %v", err)
		return false
	}
	p.noteSiteLimitHost(ctx, resp.Unsupported, now)
	p.noteSiteSlices(ctx, resp.Sites, true, now)
	p.noteSiteLimitOutside(ctx, resp.Sites, true, now)
	if resp.Partial == "" {
		p.noteSiteMoveKept(ctx, resp.Sites, true)
	} else {
		p.noteSiteMoveKept(ctx, resp.Sites, false)
	}
	p.readAllSiteTasks(ctx, all, resp.Unsupported, now)
	return true
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// --- Disk limits are counted, not enforced ------------------------------------

// settingHostingProjectQuota keeps whether the hosting file system has
// project quotas, as the last disk measurement read it ("yes"/"no" and when).
const settingHostingProjectQuota = "hosting_project_quota_v1"

type hostingProjectQuota struct {
	Mounted    bool   `json:"mounted"`
	ObservedAt string `json:"observed_at"`
}

var (
	hostingProjectQuotaMu   sync.Mutex
	hostingProjectQuotaSeen *bool
)

// noteHostingProjectQuota records a disk measurement's answer when it
// changed (or was never recorded since the start).
func (p *Panel) noteHostingProjectQuota(ctx context.Context, mounted bool) {
	hostingProjectQuotaMu.Lock()
	defer hostingProjectQuotaMu.Unlock()
	if hostingProjectQuotaSeen != nil && *hostingProjectQuotaSeen == mounted {
		return
	}
	raw, err := json.Marshal(hostingProjectQuota{Mounted: mounted, ObservedAt: metricsTime(time.Now())})
	if err != nil {
		return
	}
	if err := p.setSetting(ctx, settingHostingProjectQuota, string(raw)); err != nil {
		log.Printf("disk limits: not recorded: %v", err)
		return
	}
	value := mounted
	hostingProjectQuotaSeen = &value
}

// diskEnforcementProblems: one line for the server when the hosting file
// system has no project quota: a subscription's disk limit is measured and
// acted on by the panel (uploads stop, the overuse policy), but the file
// system itself does not refuse a write. Nothing is said before a
// measurement read the answer.
func diskEnforcementProblems(raw string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var fact hostingProjectQuota
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Mounted {
		return nil
	}
	return []monitoringProblem{{
		Key: "disk_limits_counted", Kind: "disk_limits_counted", Severity: "warning", Actor: "server_owner",
		Subject: "/var/www/celikpanel", ObservedAt: fact.ObservedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/plans"},
	}}
}
