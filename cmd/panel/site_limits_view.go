package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
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
//   - GET /api/v1/domains/{id}/site-limits: the site's limits, what systemd
//     applies, what the site uses now, and when it reached a limit (from the
//     slice cgroup's own counters, read with this request and at most every
//     five minutes in the background). Everyone who sees the domain reads the
//     limits and the events; the administrator and a reseller also read who
//     sets each limit by name; the administrator alone reads which programs
//     run inside the limits and the one action that moves the rest. Reading
//     never changes anything.
//   - POST /api/v1/domains/{id}/site-limits/move: the administrator's move of
//     the site's running programs into its limits (Agent.MoveSiteSliceV1).
//     One move at a time across the Panel; a repeated request id answers the
//     first answer.
//   - Problems now: a site that reached its memory or process limit at two or
//     more readings in the last day; one line, for the administrator, naming
//     the sites whose programs still run outside their limits (nothing moves
//     by itself after an update or a restart); and, once for the server, that
//     disk limits are counted but not enforced where the hosting file system
//     has no project quota.
//
// Sitenin sınırları: herkes sınırları ve ne zaman aşıldığını okur; yönetici
// hangi programların sınır içinde olduğunu da görür ve kalanları tek işlemle
// taşır. Okumak hiçbir şeyi değiştirmez.

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

// --- What happened: the counters' ledger ---------------------------------------

// settingSiteLimitEvents keeps, per site, the counters last read and when a
// limit was reached (panel_settings; no schema change).
const settingSiteLimitEvents = "site_limit_events_v1"

// siteLimitCPUSeconds is how long a site must have waited for the processor
// between two readings before it counts as slowed down.
const siteLimitCPUSeconds = 30

// siteLimitRepeatWindow and siteLimitRepeatHits: a limit reached at this many
// readings within the window is a Problem.
const (
	siteLimitRepeatWindow = 24 * time.Hour
	siteLimitRepeatHits   = 2
)

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

// observeSiteSlices adds what the counters say since the last reading. The
// first reading of a site is its baseline (what happened before is not
// claimed with a time). A counter lower than before started again (the
// slice was made anew): its whole value is new.
func observeSiteSlices(ledger siteLimitLedger, statuses []transport.SiteSliceStatus, now time.Time) bool {
	changed := false
	at := metricsTime(now)
	for _, status := range statuses {
		if !status.CountersRead {
			continue
		}
		key := strconv.Itoa(status.SiteID)
		previous, seen := ledger[key]
		current := status.Counters
		record := &siteLimitRecord{Counters: current, ReadAt: at}
		if seen {
			record.Events = previous.Events
			delta := func(now, before int64) int64 {
				if now < before {
					return now
				}
				return now - before
			}
			rises := map[string]int64{
				"memory": delta(current.OOMKills, previous.Counters.OOMKills),
				"tasks":  delta(current.TasksMaxHits, previous.Counters.TasksMaxHits),
			}
			if delta(current.CPUThrottledUSec, previous.Counters.CPUThrottledUSec) >= siteLimitCPUSeconds*1_000_000 {
				rises["cpu"] = 1
			}
			for kind, n := range rises {
				if n <= 0 {
					continue
				}
				if record.Events == nil {
					record.Events = map[string]*siteLimitEvent{}
				}
				event := record.Events[kind]
				if event == nil {
					event = &siteLimitEvent{FirstAt: at}
					record.Events[kind] = event
				}
				event.Count += n
				event.LastAt = at
				event.Hits = append(event.Hits, at)
			}
		}
		for _, event := range record.Events {
			kept := event.Hits[:0]
			for _, hit := range event.Hits {
				if t, err := time.Parse(time.RFC3339, hit); err == nil && now.Sub(t) <= siteLimitRepeatWindow {
					kept = append(kept, hit)
				}
			}
			event.Hits = kept
		}
		ledger[key] = record
		changed = true
	}
	return changed
}

// noteSiteSlices records a reading.
func (p *Panel) noteSiteSlices(ctx context.Context, statuses []transport.SiteSliceStatus, now time.Time) siteLimitLedger {
	siteLimitLedgerMu.Lock()
	defer siteLimitLedgerMu.Unlock()
	ledger := p.loadSiteLimitLedger(ctx)
	if observeSiteSlices(ledger, statuses, now) {
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

// siteUnitStoppedOutside: a stopped unit whose file names the system slice.
const siteUnitStoppedOutside = "stopped_outside"

type siteLimitUnitView struct {
	siteLimitUnit
	Placement string `json:"placement"`
	OwnMB     int    `json:"own_memory_mb,omitempty"`
	// Lower: its own memory limit is above the site's, so in the slice it
	// may use less than before.
	Lower bool `json:"lower,omitempty"`
}

type siteLimitsView struct {
	Known     bool             `json:"known"`
	ReadError string           `json:"read_error,omitempty"`
	Domain    string           `json:"domain"`
	Admin     bool             `json:"admin"`
	Limits    []siteLimitValue `json:"limits"`
	// Live is what systemd applies now (nil: no program of the site runs in
	// its limits yet); usage now.
	Live            *transport.SiteSliceLimitsRead `json:"live,omitempty"`
	MemoryCurrentMB int64                          `json:"memory_current_mb,omitempty"`
	TasksCurrent    int64                          `json:"tasks_current,omitempty"`
	Events          []siteLimitEventView           `json:"events"`
	// Administrator only.
	SliceFile string              `json:"slice_file,omitempty"`
	Slice     string              `json:"slice,omitempty"`
	Units     []siteLimitUnitView `json:"units,omitempty"`
	NeedsMove int                 `json:"needs_move,omitempty"`
	Busy      bool                `json:"busy,omitempty"`
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
	names := admin || caller.Role == roleReseller
	view := siteLimitsView{Domain: site.Domain, Admin: admin, Limits: []siteLimitValue{}, Events: []siteLimitEventView{}}
	for _, key := range siteSliceKeys {
		value := siteLimitValueOf(limits, key)
		if !names {
			value.SetByName = ""
		}
		view.Limits = append(view.Limits, value)
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
	ledger := p.noteSiteSlices(ctx, resp.Sites, time.Now())
	if record := ledger[strconv.Itoa(site.SiteID)]; record != nil {
		view.Events = siteLimitEventViews(record)
	}
	if status.Active {
		live := status.Live
		view.Live = &live
		view.MemoryCurrentMB, view.TasksCurrent = status.MemoryCurrentMB, status.TasksCurrent
	}
	if admin {
		view.SliceFile, view.Slice = status.File, status.Slice
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
			}
			unit.Lower = found.Own.MemoryMB > memory || (found.Own.MemoryMB == 0 && unit.Placement != transport.SiteUnitAbsent && unit.Placement != transport.SiteUnitInSlice)
			if siteUnitNeedsMove(unit.Placement) {
				view.NeedsMove++
			}
			view.Units = append(view.Units, unit)
		}
		view.Busy = siteMoveBusy()
	}
	// Every reader's reading keeps the administrator's Problems now line
	// current: a moved site leaves it at once.
	p.noteSiteLimitOutside(ctx, resp.Sites, false, time.Now())
	writeRightsJSON(w, http.StatusOK, view)
}

// siteUnitPlacement is where a unit runs as the card says it; a stopped unit
// whose file still names the system slice would start outside the limits
// (stopped_outside).
func siteUnitPlacement(u transport.SiteSliceUnit, slice string) string {
	if u.Placement == "" {
		return transport.SiteUnitAbsent
	}
	if u.Placement == transport.SiteUnitStopped && u.Slice != "" && u.Slice != slice {
		return siteUnitStoppedOutside
	}
	return u.Placement
}

// siteUnitNeedsMove: the administrator's move brings this unit into the
// site's limits (it runs, or would start, in the system slice).
func siteUnitNeedsMove(placement string) bool {
	return placement == transport.SiteUnitOutside || placement == siteUnitStoppedOutside
}

// siteUnitsOutside counts a site's programs the move would bring in.
func siteUnitsOutside(status transport.SiteSliceStatus) int {
	n := 0
	for _, u := range status.Units {
		if siteUnitNeedsMove(siteUnitPlacement(u, status.Slice)) {
			n++
		}
	}
	return n
}

type errorString string

func (e errorString) Error() string { return string(e) }

func siteLimitEventViews(record *siteLimitRecord) []siteLimitEventView {
	out := []siteLimitEventView{}
	for kind, event := range record.Events {
		out = append(out, siteLimitEventView{Kind: kind, Count: event.Count, FirstAt: event.FirstAt, LastAt: event.LastAt,
			Repeated: len(event.Hits) >= siteLimitRepeatHits})
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
		"units": resp.Units, "error": resp.Error,
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
// every five minutes, one Agent call, never two at once.
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

// siteLimitProblems: a site that reached its memory or process limit at two
// or more readings in the last day. The reading itself starts in the
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
			readCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
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
	if len(ledger) == 0 && outside == "" {
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
			if event == nil || len(event.Hits) < siteLimitRepeatHits {
				continue
			}
			last, err := time.Parse(time.RFC3339, event.Hits[len(event.Hits)-1])
			if err != nil || now.Sub(last) > siteLimitRepeatWindow {
				continue
			}
			problems = append(problems, monitoringProblem{
				Key: "site_limit:" + site + ":" + kind, Kind: "site_" + kind + "_repeated", Severity: "warning", Actor: "administrator",
				Subject: name, Name: kind, Value: float64(len(event.Hits)), Since: event.Hits[0], ObservedAt: event.LastAt,
				Action: monitoringProblemAction{Kind: "open", To: "/domains/" + url.PathEscape(name)},
			})
		}
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].Key < problems[j].Key })
	return append(problems, siteLimitOutsideProblem(outside, names)...)
}

// --- Sites whose programs still run outside their limits ---------------------

// settingSiteLimitOutside keeps, per site, how many of its programs run (or
// would start) outside its limits, as the last reading said (panel_settings;
// no schema change). Nothing moves by itself after an update, so this is the
// administrator's to-do list.
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
	state := siteLimitOutside{}
	if raw != "" && !whole {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	if state.Sites == nil {
		state.Sites = map[string]int{}
	}
	for _, status := range statuses {
		key := strconv.Itoa(status.SiteID)
		if n := siteUnitsOutside(status); n > 0 {
			state.Sites[key] = n
		} else {
			delete(state.Sites, key)
		}
	}
	state.CheckedAt = metricsTime(now)
	if len(state.Sites) == 0 {
		state.Sites = nil
	}
	next, err := json.Marshal(state)
	if err != nil {
		return
	}
	if raw == "" && state.Sites == nil {
		return // nothing was outside and nothing is
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

// readAllSiteSlices reads every site's slice once and records it.
func (p *Panel) readAllSiteSlices(ctx context.Context) bool {
	sites, err := p.siteLimitSites(ctx, 0)
	if err != nil {
		log.Printf("site limits: the sites could not be read: %v", err)
		return false
	}
	if len(sites) == 0 {
		return true
	}
	resp, err := readSiteSlicesCall(p, ctx, siteSliceQueries(sites))
	if err != nil || resp.Error != "" {
		if strings.Contains(errText(err)+resp.Error, "can't find method") {
			return true // an Agent older than this Panel: nothing to read until it is updated
		}
		log.Printf("site limits: the slices could not be read: %v %s", err, resp.Error)
		return false
	}
	now := time.Now()
	p.noteSiteSlices(ctx, resp.Sites, now)
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
