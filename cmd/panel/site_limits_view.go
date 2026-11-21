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
//     line when this host cannot enforce a site's limits at all; and, once
//     for the server, that disk limits are counted but not enforced where the
//     hosting file system has no project quota.
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
// sites (review finding 12).
const siteSliceReadPage = 1000

// readSiteSlicesPaged reads the sites' slices in pages and joins the
// answers. The host's Unsupported reason comes back with the statuses.
func (p *Panel) readSiteSlicesPaged(ctx context.Context, sites []siteLimitSite) ([]transport.SiteSliceStatus, error) {
	resp, err := p.readSiteSlicesAll(ctx, sites)
	return resp.Sites, err
}

func (p *Panel) readSiteSlicesAll(ctx context.Context, sites []siteLimitSite) (transport.SiteSlicesStatusResponse, error) {
	out := transport.SiteSlicesStatusResponse{Sites: []transport.SiteSliceStatus{}}
	for start := 0; start < len(sites); start += siteSliceReadPage {
		end := start + siteSliceReadPage
		if end > len(sites) {
			end = len(sites)
		}
		resp, err := readSiteSlicesCall(p, ctx, siteSliceQueries(sites[start:end]))
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
)

// Event kinds. Only "memory" and "tasks" are the site's own limit (proved
// by the kernel's counters, sitesandbox.SliceCounters); "memory_other" and
// "tasks_other" are a program of the site stopped or refused where the
// kernel does not prove the site's limit (a program's own smaller limit, a
// host-wide shortage): the screens then never blame the plan.
var siteLimitEventKinds = []string{"memory", "memory_other", "tasks", "tasks_other", "cpu"}

type siteLimitEvent struct {
	Count   int64    `json:"count"`
	FirstAt string   `json:"first_at"`
	LastAt  string   `json:"last_at"`
	Hits    []string `json:"hits,omitempty"`
}

type siteLimitRecord struct {
	Counters transport.SiteSliceCounters `json:"counters"`
	ReadAt   string                      `json:"read_at"`
	Events   map[string]*siteLimitEvent  `json:"events,omitempty"`
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
		cpuWait := delta(current.CPUThrottledUSec, before.CPUThrottledUSec)
		if cpuWait >= siteLimitCPUSeconds*1_000_000 {
			rises["cpu"] = 1
		}
		rose := reset
		for _, kind := range siteLimitEventKinds {
			n := rises[kind]
			if n <= 0 {
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
			event.Count += n
			event.LastAt = at
			event.Hits = append(event.Hits, at)
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
}

// siteUnitStoppedOutside: a stopped unit whose effective Slice= is the
// system slice (it would start outside the limits).
const siteUnitStoppedOutside = "stopped_outside"

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
	// Live is what systemd applies now (nil: no program of the site runs in
	// its limits yet); usage now.
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
	for _, key := range siteSliceKeys {
		value := siteLimitValueOf(limits, key)
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
	if status.Active && view.Applies == siteLimitsApply {
		live := status.Live
		view.Live = &live
		view.MemoryCurrentMB, view.TasksCurrent = status.MemoryCurrentMB, status.TasksCurrent
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
			if siteUnitNeedsMove(unit.Placement, unit.Blocked) {
				view.NeedsMove++
			}
			view.Units = append(view.Units, unit)
		}
		view.Busy = siteMoveBusy()
	}
	// A reader's reading keeps the administrator's Problems now line current
	// (written only when the list changed).
	p.noteSiteLimitOutside(ctx, resp.Sites, false, time.Now())
	writeRightsJSON(w, http.StatusOK, view)
}

// siteLimitsApplies says whether a site's limits hold for its programs: the
// host can enforce them, the slice file is there (an absent file means
// systemd runs the slice without limits), and at least one program runs or
// would start inside them.
func siteLimitsApplies(status transport.SiteSliceStatus, unsupported string) string {
	if unsupported != "" {
		return siteLimitsUnsupported
	}
	if status.File == transport.SiteFileAbsent {
		return siteLimitsNoFile
	}
	for _, u := range status.Units {
		switch placement := siteUnitPlacement(u, status.Slice); placement {
		case transport.SiteUnitInSlice:
			return siteLimitsApply
		case transport.SiteUnitStopped:
			return siteLimitsApply // it starts inside them
		}
	}
	return siteLimitsOutside
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

// siteUnitsOutside counts a site's programs the move would bring in.
func siteUnitsOutside(status transport.SiteSliceStatus) int {
	n := 0
	for _, u := range status.Units {
		if siteUnitNeedsMove(siteUnitPlacement(u, status.Slice), siteUnitBlocked(u)) {
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
			Repeated: recent >= siteLimitRepeatHits})
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
	sites, err := p.siteLimitSites(ctx, domainID)
	if err != nil || len(sites) != 1 {
		writeClientError(w, http.StatusNotFound, "site not found for domain")
		return
	}
	site := sites[0]
	slice := p.siteSliceForSubscription(ctx, site.SiteID, site.SubscriptionID, site.Domain)
	if slice == nil || len(site.Units) == 0 {
		writeServiceAppRefusal(w, http.StatusConflict, "site_limits_nothing",
			"Nothing was changed: this site runs no program, or its limits could not be read.", nil)
		return
	}
	move := &transport.MoveSiteSliceRequest{ExpectedBuildCommit: strings.TrimSpace(buildCommit), RequestID: req.RequestID,
		Site: *slice, SubscriptionID: site.SubscriptionID, DomainID: site.DomainID}
	for _, u := range site.Units {
		move.Units = append(move.Units, u.Unit)
	}
	resp, err := moveSiteSliceCall(p, ctx, move)
	if err != nil {
		writeServiceAppRefusal(w, http.StatusServiceUnavailable, "agent_unreachable",
			"The server did not answer, so the result is unknown. Read this page again before you try again.", nil)
		return
	}
	p.auditDetailed(r, "site.limits_move", "domain", domainID, map[string]any{
		"request_id": req.RequestID, "site": site.Domain, "done": resp.Done, "rolled_back": resp.RolledBack,
		"units": resp.Units, "error": resp.Error, "wait_seconds": resp.WaitSeconds,
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
	if len(ledger) == 0 && outside == "" && host == "" {
		return nil
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
	problems = append(problems, siteLimitHostProblem(host)...)
	return append(problems, siteLimitOutsideProblem(outside, names)...)
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
	state := siteLimitOutside{Sites: map[string]int{}}
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

// siteLimitOutsideProblem is ONE line, for the administrator, naming the
// sites (count and the first five) whose own programs run outside their
// limits, opening the first. A to-do, not a fault: warning. Sites that are
// gone are left out; no site left, no line.
func siteLimitOutsideProblem(raw string, names map[string]string) []monitoringProblem {
	if raw == "" {
		return nil
	}
	var state siteLimitOutside
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil
	}
	var sites []string
	for key, n := range state.Sites {
		if name := names[key]; name != "" && n > 0 {
			sites = append(sites, name)
		}
	}
	if len(sites) == 0 {
		return nil
	}
	sort.Strings(sites)
	listed := sites
	if len(listed) > 5 {
		listed = append(append([]string{}, sites[:5]...), "…")
	}
	subject := strings.Join(listed, ", ")
	return []monitoringProblem{{
		Key: "site_programs_outside", Kind: "site_programs_outside", Severity: "warning", Actor: "administrator",
		Name: subject, Subject: subject, Value: float64(len(sites)), ObservedAt: state.CheckedAt,
		Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(sites[0])},
	}}
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
