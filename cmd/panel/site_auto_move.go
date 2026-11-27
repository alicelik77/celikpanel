package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/transport"
)

// Every site goes inside its limits by itself (alpha.150, D-048; the
// owner's words of 27 November 2026: "zaten otomatik olarak sınırlarda
// olmalıydı. buton falan saçmalığına gerek yok"). docs/SITE-SANDBOX.md §8.4
// and §9.3.
//
//   - ONE worker of the panel moves every site whose programs or scheduled
//     tasks still run outside its limits, one site at a time, in the order of
//     the sites' ids, with a short pause between two sites, through the same
//     press the administrator's move used (pressSiteMove: the tasks first,
//     then the programs; the same Agent calls, one fresh request id per site).
//     The move itself (its record, its put-back, its proofs, its time rule,
//     its locks, the update's gate) is the Agent's and does not change.
//   - It starts some minutes after the panel's start (after the side doors
//     step) and again after a site is created or restored, never because a
//     page was read (D-024). Before each site it waits while the panel's
//     update is admitted or running, while the server is busy with another
//     change, or while a move the administrator pressed runs; the Agent's own
//     claim refuses a second move and a move during the update, also across
//     a restart of the panel (it never knows this process).
//   - Once per site per installed panel version, never a loop: the mark is
//     written before the move is sent (panel_settings site_auto_move_v1; no
//     schema change). A move that was refused, put back, stopped for its time
//     or whose end is not known is not tried again by the worker: one Problems
//     now line per site says why, who acts and offers Try again. A refusal
//     that clears by itself (the site being changed, another move, the
//     panel's update, an Agent of another build) is not a failure: the site
//     goes to the end of the run and is tried again later in the same run.
//     A mark "started" whose end this process does not know (the panel
//     stopped in the middle) is settled by reading only: inside its limits it
//     is "moved", otherwise "unknown"; the move is never sent twice.
//   - A kept move record stops the run (no next site is moved, as for a
//     press); the existing line and card say what the administrator does, and
//     the run goes on by itself once a reading finds the record settled.
//   - A site whose tasks the administrator put back into cron on purpose is
//     left alone until it is moved again from its own card
//     (site_tasks_put_back_v1).
//   - While it runs, the administrator reads one calm line ("Sites are being
//     moved into their limits (3 / 12) — you do not need to do anything"); a
//     customer and a reseller read nothing of it.
//
// Her site kendi sınırlarının içine kendiliğinden girer: panelin tek bir
// işçisi dışarıda kalan siteleri sırayla, aynı taşıma çağrısıyla taşır; her
// site sürüm başına bir kez denenir, başarısız olan Şu anki sorunlar'da
// "Tekrar dene" ile görünür; kimse bir düğmeye basmak zorunda değildir.

const (
	// settingSiteAutoMove keeps the worker's marks for the installed panel
	// version (siteAutoMarks).
	settingSiteAutoMove = "site_auto_move_v1"
	// settingSiteTasksPutBack keeps the sites whose scheduled tasks the
	// administrator put back into cron on purpose: site id -> since when.
	settingSiteTasksPutBack = "site_tasks_put_back_v1"
)

// The marks of one site (siteAutoMark.State).
const (
	// siteAutoStarted: the move was sent; its end is not recorded yet.
	siteAutoStarted = "started"
	// siteAutoMoved: the site is inside its limits (or the move had nothing
	// it could take).
	siteAutoMoved = "moved"
	// siteAutoLeft: the programs went in where they could; the scheduled
	// tasks stayed in cron for a reason the server owner or the administrator
	// changes (a line the timers do not reproduce, the owner's own
	// cron.allow, no cron).
	siteAutoLeft = "left"
	// siteAutoFailed: refused or put back; nothing of it is tried again.
	siteAutoFailed = "failed"
	// siteAutoTimeUp: the move stopped for its time.
	siteAutoTimeUp = "time_up"
	// siteAutoUnknown: the end of the move is not known.
	siteAutoUnknown = "unknown"
	// siteAutoKept: the move left its record kept (its put-back did not
	// complete).
	siteAutoKept = "kept"
)

// Outcomes the worker acts on that are not marks.
const (
	// siteAutoWait: refused before anything changed, for a reason that clears
	// by itself: the site goes to the end of the run.
	siteAutoWait = "wait"
	// siteAutoWaitKept: refused because a move record (of this site, from
	// before, or of another) is kept: the run waits until it is settled.
	siteAutoWaitKept = "wait_kept"
)

type siteAutoMark struct {
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	At        string `json:"at"`
	RequestID string `json:"request_id,omitempty"`
}

// siteAutoMarks are the marks of one installed panel version: marks of
// another version count as none (each site is tried once per version).
type siteAutoMarks struct {
	Version string                  `json:"version"`
	Sites   map[string]siteAutoMark `json:"sites,omitempty"`
}

// siteAutoMoveVersion is the installed panel's version and build.
var siteAutoMoveVersion = func() string {
	return strings.TrimSpace(buildVersion) + "+" + strings.TrimSpace(buildCommit)
}

var siteAutoMarksMu sync.Mutex

// readSiteAutoMarks reads the marks. A setting that cannot be read or decoded
// is an error, never "no marks": the worker then moves nothing (a site must
// never be moved twice because its mark could not be read).
func (p *Panel) readSiteAutoMarks(ctx context.Context) (siteAutoMarks, error) {
	marks := siteAutoMarks{Version: siteAutoMoveVersion(), Sites: map[string]siteAutoMark{}}
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingSiteAutoMove).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == "") {
		return marks, nil
	}
	if err != nil {
		return marks, err
	}
	var stored siteAutoMarks
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return marks, err
	}
	if stored.Version == marks.Version {
		for k, v := range stored.Sites {
			marks.Sites[k] = v
		}
	}
	return marks, nil
}

// setSiteAutoMark writes one site's mark ("" state removes it). The marks of
// an earlier version go with the first write of this one.
func (p *Panel) setSiteAutoMark(ctx context.Context, siteID int, mark siteAutoMark) error {
	siteAutoMarksMu.Lock()
	defer siteAutoMarksMu.Unlock()
	marks, err := p.readSiteAutoMarks(ctx)
	if err != nil {
		return err
	}
	key := strconv.Itoa(siteID)
	if mark.State == "" {
		delete(marks.Sites, key)
	} else {
		marks.Sites[key] = mark
	}
	raw, err := json.Marshal(marks)
	if err != nil {
		return err
	}
	return p.setSetting(ctx, settingSiteAutoMove, string(raw))
}

// sitesPutBack reads the sites whose tasks were put back on purpose; an
// unreadable value is an error (the worker then leaves every site alone).
func (p *Panel) sitesPutBack(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	var raw string
	err := p.db.GetDB().QueryRowContext(ctx, `SELECT value FROM panel_settings WHERE key = ?`, settingSiteTasksPutBack).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && raw == "") {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}, err
	}
	return out, nil
}

// noteSitePutBack marks (or, put false, unmarks) a site whose tasks the
// administrator put back into cron on purpose.
func (p *Panel) noteSitePutBack(ctx context.Context, siteID int, put bool) {
	siteAutoMarksMu.Lock()
	defer siteAutoMarksMu.Unlock()
	sites, err := p.sitesPutBack(ctx)
	if err != nil {
		sites = map[string]string{}
	}
	key := strconv.Itoa(siteID)
	if _, ok := sites[key]; ok == put {
		return
	}
	if put {
		sites[key] = metricsTime(time.Now())
	} else {
		delete(sites, key)
	}
	value := ""
	if len(sites) > 0 {
		raw, err := json.Marshal(sites)
		if err != nil {
			return
		}
		value = string(raw)
	}
	if err := p.setSetting(ctx, settingSiteTasksPutBack, value); err != nil {
		log.Printf("site limits: the site whose tasks were put back was not recorded: %v", err)
	}
}

// siteAutoClearsByItself: a refusal that came before anything changed and
// ends by itself (the site being changed, another move, the panel's update or
// a record of it that cannot be read yet, an Agent of another build while
// the panel's update finishes).
func siteAutoClearsByItself(code string) bool {
	switch code {
	case "site_busy", "another_move", "update_running", "update_unreadable", "build":
		return true
	}
	return false
}

// siteAutoTasksOwnerRefusal: a tasks move refused, nothing changed, for the
// server owner's own cron files (their cron.allow names the account, they
// deleted CelikPanel's file, or accounts come from a directory service); the
// site's card names it and the owner's own lines stay (D-022: detected, never
// overwritten).
func siteAutoTasksOwnerRefusal(code string) bool {
	return code == transport.SiteTasksRefusalCronAllow
}

// classifySiteMove says what one press of a site came to: a mark (moved,
// left, failed, time_up, unknown, kept) with its reason code, or one of the
// two waits (the site is tried again later in the run).
func classifySiteMove(resp transport.MoveSiteSliceResponse, tasks *transport.SiteTasksMoveResponse, outcome siteMovePressOutcome) (string, string) {
	switch outcome {
	case siteMovePressNoLimits:
		return siteAutoFailed, "limits_unread"
	case siteMovePressUnreachable:
		return siteAutoUnknown, "agent_unreachable"
	}
	code := ""
	if resp.Reason != nil {
		code = resp.Reason.Code
	}
	if tasks != nil && tasks.Outcome == transport.SiteTasksUnknown {
		if tasks.Recovery != nil {
			return siteAutoKept, "tasks"
		}
		return siteAutoUnknown, "tasks_unknown"
	}
	if code == transport.SiteMoveRefusalKept && !resp.RolledBack {
		return siteAutoWaitKept, code
	}
	if resp.Kept {
		return siteAutoKept, code
	}
	if !resp.Done && !resp.RolledBack && !resp.TimeUp && siteAutoClearsByItself(code) {
		return siteAutoWait, code
	}
	if resp.TimeUp {
		return siteAutoTimeUp, "time_up"
	}
	if resp.RolledBack || !resp.Done {
		if code == "" {
			code = "not_finished"
		}
		return siteAutoFailed, code
	}
	if tasks == nil {
		return siteAutoMoved, ""
	}
	switch tasks.Outcome {
	case transport.SiteTasksStayed:
		return siteAutoLeft, "stayed"
	case transport.SiteTasksNoCron:
		return siteAutoLeft, "no_cron"
	case transport.SiteTasksRefused:
		if siteAutoTasksOwnerRefusal(tasks.Refusal) {
			return siteAutoLeft, tasks.Refusal
		}
		if tasks.Refusal == "" {
			return siteAutoFailed, "tasks_refused"
		}
		return siteAutoFailed, tasks.Refusal
	case transport.SiteTasksPutBack:
		if tasks.Refusal != "" {
			return siteAutoFailed, tasks.Refusal
		}
		return siteAutoFailed, "tasks_put_back"
	}
	return siteAutoMoved, ""
}

// noteSiteMoveAttempt records the administrator's own move of a site (Try
// again, or the move of a site whose tasks were put back on purpose) as the
// site's latest attempt: Problems now and the card say what it came to, and
// a site whose tasks moved again is no longer left alone.
func (p *Panel) noteSiteMoveAttempt(ctx context.Context, site siteLimitSite, requestID string, resp transport.MoveSiteSliceResponse, tasks *transport.SiteTasksMoveResponse) {
	state, reason := classifySiteMove(resp, tasks, siteMovePressRan)
	if tasks != nil {
		switch tasks.Outcome {
		case transport.SiteTasksMoved, transport.SiteTasksAlready, transport.SiteTasksCrontabBack:
			p.noteSitePutBack(ctx, site.SiteID, false)
		}
	}
	if state == siteAutoWait || state == siteAutoWaitKept {
		return // nothing changed: the earlier mark stays what it was
	}
	if err := p.setSiteAutoMark(ctx, site.SiteID, siteAutoMark{State: state, Reason: reason, At: metricsTime(time.Now()), RequestID: requestID}); err != nil {
		log.Printf("site limits: the outcome of the move of %s was not recorded: %v", site.Domain, err)
	}
	p.readSiteLimitsOf(ctx, site)
}

// readSiteLimitsOf reads one site's slice and records it (the outside list
// and a kept record), so that the lines follow the site's latest move.
func (p *Panel) readSiteLimitsOf(ctx context.Context, site siteLimitSite) {
	if len(site.Units) == 0 {
		return
	}
	resp, err := readSiteSlicesCall(p, ctx, siteSliceQueries([]siteLimitSite{site}))
	if err != nil || resp.Error != "" || len(resp.Sites) != 1 {
		return
	}
	p.noteSiteLimitOutside(ctx, resp.Sites, false, time.Now())
	p.noteSiteMoveKept(ctx, resp.Sites, false)
}

// --- The worker ----------------------------------------------------------------------

// siteAutoMover is the one worker of the panel.
type siteAutoMover struct {
	mu sync.Mutex
	// scheduled: a run is waiting to start or running; again: a site was
	// created or restored meanwhile (one more pass when it ends).
	scheduled, again bool
	// moving: the run is moving sites (the calm line); not while it waits
	// for a kept record. done of total sites of this run are settled.
	moving      bool
	done, total int
}

var siteAutoMove siteAutoMover

var (
	// siteAutoMoveFirstWait: after the panel's start, after the side doors
	// step (3 minutes) has had its turn.
	siteAutoMoveFirstWait = 5 * time.Minute
	// siteAutoMoveAfterPoke: after a site was created or restored.
	siteAutoMoveAfterPoke = time.Minute
	// siteAutoMovePause: between two sites, so that a server with many sites
	// does not restart everything in one second.
	siteAutoMovePause = 20 * time.Second
	// A site refused for a reason that clears by itself is tried again after
	// siteAutoMoveRetryFirst, doubling up to siteAutoMoveRetryMax; after
	// siteAutoMoveGiveUp of such refusals it is marked failed with that
	// reason (Problems now then offers Try again).
	siteAutoMoveRetryFirst = time.Minute
	siteAutoMoveRetryMax   = 10 * time.Minute
	siteAutoMoveGiveUp     = 6 * time.Hour
	// siteAutoMoveKeptEvery: how often a run stopped by a kept record reads
	// whether it was settled (a reading only).
	siteAutoMoveKeptEvery = 5 * time.Minute
	// siteAutoMoveReadAttempts: a run whose first reading the Agent does not
	// answer tries again this many times (siteAutoMoveRetryMax apart), then
	// waits for the next start, site creation or restore.
	siteAutoMoveReadAttempts = 6
	// Seams for tests.
	siteAutoMoveSleep    = time.Sleep
	siteAutoMoveNow      = time.Now
	startSiteAutoMoveRun = func(run func()) { go run() }
	// siteAutoMoveNeedsAgent: a panel without its Agent starts no run (tests
	// with replaced Agent calls turn it off).
	siteAutoMoveNeedsAgent = true
)

// siteAutoMoveProgress is the calm line's numbers (Problems now, the card).
type siteAutoMoveProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// siteAutoMoveProgressNow is the worker's progress while it moves sites (nil
// otherwise: nothing is shown).
func siteAutoMoveProgressNow() *siteAutoMoveProgress {
	m := &siteAutoMove
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.moving || m.total == 0 {
		return nil
	}
	return &siteAutoMoveProgress{Done: m.done, Total: m.total}
}

func (m *siteAutoMover) set(moving bool, done, total int) {
	m.mu.Lock()
	m.moving, m.done, m.total = moving, done, total
	m.mu.Unlock()
}

// startSiteAutoMove schedules the run after the panel's start.
func (p *Panel) startSiteAutoMove() {
	p.scheduleSiteAutoMove(siteAutoMoveFirstWait)
}

// pokeSiteAutoMove asks for a run after a site was created or restored (one
// more pass when a run is already waiting or running). It never moves
// anything itself, and nothing a page reads calls it.
func (p *Panel) pokeSiteAutoMove() {
	if p == nil {
		return
	}
	p.scheduleSiteAutoMove(siteAutoMoveAfterPoke)
}

func (p *Panel) scheduleSiteAutoMove(wait time.Duration) {
	if siteAutoMoveNeedsAgent && p.agentClient == nil {
		return
	}
	m := &siteAutoMove
	m.mu.Lock()
	if m.scheduled {
		m.again = true
		m.mu.Unlock()
		return
	}
	m.scheduled, m.again = true, false
	m.mu.Unlock()
	startSiteAutoMoveRun(func() {
		siteAutoMoveSleep(wait)
		for {
			p.siteAutoMovePass()
			m.mu.Lock()
			if !m.again {
				m.scheduled, m.moving, m.done, m.total = false, false, 0, 0
				m.mu.Unlock()
				return
			}
			m.again = false
			m.mu.Unlock()
		}
	})
}

// siteAutoItem is one site of a run.
type siteAutoItem struct {
	site      siteLimitSite
	notBefore time.Time
	waiting   time.Duration // the next wait after a refusal that clears by itself
	since     time.Time     // the first such refusal
}

// siteAutoMoveRead reads every site's slice and scheduled tasks and records
// them (the outside list, a kept record, the host), as the background
// reading does. Bounded retries when the Agent does not answer.
func (p *Panel) siteAutoMoveRead() bool {
	for attempt := 0; attempt < siteAutoMoveReadAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		ok := p.readAllSiteSlices(ctx)
		cancel()
		if ok {
			return true
		}
		siteAutoMoveSleep(siteAutoMoveRetryMax)
	}
	log.Printf("site limits: the sites could not be read for the move into their limits; it runs again at the next start, or when a site is created or restored")
	return false
}

// siteOutsideNow says whether a site's programs or tasks run outside its
// limits as the last reading recorded it, and whether a move record is kept.
func (p *Panel) siteOutsideNow(ctx context.Context) (outside map[int]bool, kept bool, unsupported bool) {
	outside = map[int]bool{}
	state := siteLimitOutside{}
	if raw := p.setting(ctx, settingSiteLimitOutside); raw != "" {
		_ = json.Unmarshal([]byte(raw), &state)
	}
	for key, n := range state.Sites {
		if id, err := strconv.Atoi(key); err == nil && n > 0 {
			outside[id] = true
		}
	}
	for key, out := range state.Tasks {
		if id, err := strconv.Atoi(key); err == nil && out {
			outside[id] = true
		}
	}
	return outside, p.setting(ctx, settingSiteMoveKept) != "", p.setting(ctx, settingSiteLimitHost) != ""
}

// siteAutoMovePass is one run over the sites.
func (p *Panel) siteAutoMovePass() {
	m := &siteAutoMove
	ctx := context.Background()
	if !p.siteAutoMoveRead() {
		return
	}
	marks, err := p.readSiteAutoMarks(ctx)
	if err != nil {
		log.Printf("site limits: the record of the sites already moved could not be read (%v); no site is moved by itself until it can be", err)
		return
	}
	putBack, err := p.sitesPutBack(ctx)
	if err != nil {
		log.Printf("site limits: the record of the sites whose tasks were put back could not be read (%v); no site is moved by itself until it can be", err)
		return
	}
	all, err := p.siteLimitSites(ctx, 0)
	if err != nil {
		log.Printf("site limits: the sites could not be read: %v", err)
		return
	}
	live := map[int]bool{}
	for _, s := range all {
		live[s.SiteID] = true
	}
	p.pruneSiteAutoMarks(ctx, live)
	outside, _, unsupported := p.siteOutsideNow(ctx)
	if unsupported {
		return // this host cannot hold a site's limits: nothing to move into
	}
	var queue []*siteAutoItem
	for _, s := range all { // in the order of the sites' ids
		key := strconv.Itoa(s.SiteID)
		mark, marked := marks.Sites[key]
		if marked && mark.State == siteAutoStarted {
			// The panel stopped while this site's move ran: its end is
			// settled by reading only, never by moving it again.
			state := siteAutoMoved
			if outside[s.SiteID] {
				state = siteAutoUnknown
			}
			mark.State, mark.Reason = state, "panel_restarted"
			if err := p.setSiteAutoMark(ctx, s.SiteID, mark); err != nil {
				log.Printf("site limits: the move of %s was not settled: %v", s.Domain, err)
			}
			continue
		}
		if marked || !outside[s.SiteID] {
			continue
		}
		if _, left := putBack[key]; left {
			continue
		}
		queue = append(queue, &siteAutoItem{site: s})
	}
	if len(queue) == 0 {
		return
	}
	total, done := len(queue), 0
	m.set(true, done, total)
	defer m.set(false, 0, 0)
	for len(queue) > 0 {
		// The first site in order whose time has come; when every site left
		// waits for a reason that clears by itself, the earliest of them.
		pick := 0
		for i, it := range queue {
			if it.notBefore.Before(queue[pick].notBefore) {
				pick = i
			}
		}
		item := queue[pick]
		queue = append(queue[:pick:pick], queue[pick+1:]...)
		if wait := item.notBefore.Sub(siteAutoMoveNow()); wait > 0 {
			siteAutoMoveSleep(wait)
		}
		// A kept record stops the run until a reading finds it settled (the
		// calm line is not shown meanwhile: the kept record's line is).
		if _, kept, _ := p.siteOutsideNow(ctx); kept {
			m.set(false, done, total)
			for kept {
				siteAutoMoveSleep(siteAutoMoveKeptEvery)
				readCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				read := p.readAllSiteSlices(readCtx)
				cancel()
				if read {
					_, kept, _ = p.siteOutsideNow(ctx)
				}
			}
			m.set(true, done, total)
		}
		state, reason := p.siteAutoMoveOne(ctx, item.site)
		switch state {
		case siteAutoWait, siteAutoWaitKept:
			now := siteAutoMoveNow()
			if item.since.IsZero() {
				item.since = now
			}
			if state == siteAutoWait && now.Sub(item.since) >= siteAutoMoveGiveUp {
				// It did not clear in hours: a failure with its reason.
				p.markSiteAuto(ctx, item.site, siteAutoMark{State: siteAutoFailed, Reason: reason, At: metricsTime(now)})
				done++
				m.set(true, done, total)
				continue
			}
			if item.waiting == 0 {
				item.waiting = siteAutoMoveRetryFirst
			} else if item.waiting *= 2; item.waiting > siteAutoMoveRetryMax {
				item.waiting = siteAutoMoveRetryMax
			}
			item.notBefore = now.Add(item.waiting)
			queue = append(queue, item)
			continue
		}
		// Settled: marked, or nothing to move any more ("": moved meanwhile
		// from its card, or put back on purpose; not marked).
		done++
		m.set(true, done, total)
		if len(queue) > 0 {
			siteAutoMoveSleep(siteAutoMovePause)
		}
	}
}

// pruneSiteAutoMarks drops the marks of sites that no longer exist (deleted
// sites leave nothing behind; site ids are never given again).
func (p *Panel) pruneSiteAutoMarks(ctx context.Context, live map[int]bool) {
	gone := func(key string) bool {
		id, err := strconv.Atoi(key)
		return err != nil || !live[id]
	}
	marks, err := p.readSiteAutoMarks(ctx)
	if err == nil {
		for key := range marks.Sites {
			if gone(key) {
				id, _ := strconv.Atoi(key)
				if err := p.setSiteAutoMark(ctx, id, siteAutoMark{}); err != nil {
					log.Printf("site limits: the mark of a deleted site was not removed: %v", err)
				}
			}
		}
	}
	if putBack, err := p.sitesPutBack(ctx); err == nil {
		for key := range putBack {
			if id, convErr := strconv.Atoi(key); gone(key) && convErr == nil {
				p.noteSitePutBack(ctx, id, false)
			}
		}
	}
}

// markSiteAuto writes a site's mark and logs a mark that could not be
// written.
func (p *Panel) markSiteAuto(ctx context.Context, site siteLimitSite, mark siteAutoMark) {
	if err := p.setSiteAutoMark(ctx, site.SiteID, mark); err != nil {
		log.Printf("site limits: the outcome of the move of %s was not recorded: %v", site.Domain, err)
	}
}

// siteAutoMoveOne moves one site, or says why not now. It answers the mark
// it wrote ("" when the site needed nothing or could not be checked, so it
// was not moved), or one of the two waits.
func (p *Panel) siteAutoMoveOne(ctx context.Context, site siteLimitSite) (string, string) {
	// The site as it is now: a site moved meanwhile (from its card, or put
	// back on purpose) is not moved again.
	if putBack, err := p.sitesPutBack(ctx); err != nil {
		return siteAutoWait, "marks_unreadable"
	} else if _, left := putBack[strconv.Itoa(site.SiteID)]; left {
		return "", ""
	}
	if marks, err := p.readSiteAutoMarks(ctx); err != nil {
		return siteAutoWait, "marks_unreadable"
	} else if _, marked := marks.Sites[strconv.Itoa(site.SiteID)]; marked {
		return "", ""
	}
	if outside, _, unsupported := p.siteOutsideNow(ctx); unsupported || !outside[site.SiteID] {
		return "", ""
	}
	// The gate: never while the panel's update is admitted or running, while
	// the server is busy with another change, or while a move the
	// administrator pressed runs. The Agent refuses both ways too.
	if siteMoveUpdateInProgress(p, ctx) {
		return siteAutoWait, "update_running"
	}
	if !backgroundHostQuiet(p, ctx) {
		return siteAutoWait, "host_busy"
	}
	siteMoveMu.Lock()
	if siteMoveRunning {
		siteMoveMu.Unlock()
		return siteAutoWait, "another_move"
	}
	siteMoveRunning = true
	siteMoveMu.Unlock()
	defer func() {
		siteMoveMu.Lock()
		siteMoveRunning = false
		siteMoveMu.Unlock()
	}()
	// Before anything is sent: the mark. Without it written, nothing is moved
	// (a restart could otherwise send the move a second time).
	requestID := "auto-" + newAuditEventID()
	started := siteAutoMark{State: siteAutoStarted, At: metricsTime(siteAutoMoveNow()), RequestID: requestID}
	if err := p.setSiteAutoMark(ctx, site.SiteID, started); err != nil {
		log.Printf("site limits: %s was not moved into its limits: its mark could not be written (%v)", site.Domain, err)
		return siteAutoWait, "mark_failed"
	}
	moveCtx, cancel := hostingDurableContext(context.Background())
	resp, tasks, outcome := p.pressSiteMove(moveCtx, site, requestID)
	cancel()
	state, reason := classifySiteMove(resp, tasks, outcome)
	tasksOutcome, tasksError := "", ""
	if tasks != nil {
		tasksOutcome, tasksError = tasks.Outcome, tasks.Error
	}
	p.recordAudit(auditRecord{Action: "site.limits_move", ResourceType: "domain", ResourceID: site.DomainID, Details: auditDetailsText(map[string]any{
		"request_id": requestID, "site": site.Domain, "automatic": true, "done": resp.Done, "rolled_back": resp.RolledBack,
		"kept": resp.Kept, "time_up": resp.TimeUp, "error": resp.Error, "units": len(resp.Units), "tasks": tasksOutcome, "tasks_error": tasksError,
		"outcome": state, "reason": reason,
	})})
	switch state {
	case siteAutoWait, siteAutoWaitKept:
		// Refused before anything changed: no mark (it is tried again later).
		if err := p.setSiteAutoMark(ctx, site.SiteID, siteAutoMark{}); err != nil {
			log.Printf("site limits: the mark of %s could not be taken away again (%v); it is not moved by itself until the next version", site.Domain, err)
			return siteAutoUnknown, reason
		}
	default:
		p.markSiteAuto(ctx, site, siteAutoMark{State: state, Reason: reason, At: metricsTime(siteAutoMoveNow()), RequestID: requestID})
		if state != siteAutoMoved {
			log.Printf("site limits: %s was not moved into its limits by itself (%s %s): %s", site.Domain, state, reason, resp.Error)
		}
	}
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Minute)
	p.readSiteLimitsOf(readCtx, site)
	readCancel()
	return state, reason
}

// --- What people read ------------------------------------------------------------------

// siteAutoMoveView is the administrator's part of the card about the
// worker: the calm line while it runs, and the one move action this card
// still offers: Try again next to a move that did not bring the site inside
// (offer "retry", with State and Reason), and the move of a site whose
// scheduled tasks were put back into cron on purpose (offer "move").
type siteAutoMoveView struct {
	Done   int    `json:"done,omitempty"`
	Total  int    `json:"total,omitempty"`
	Offer  string `json:"offer,omitempty"`
	State  string `json:"state,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// siteAutoMoveViewOf is the card's part for one site ("outside": its
// programs or tasks run outside its limits now; "kept": a move record of it
// is kept, whose own Try again comes first).
func (p *Panel) siteAutoMoveViewOf(ctx context.Context, siteID int, outside, kept bool) *siteAutoMoveView {
	view := &siteAutoMoveView{}
	if progress := siteAutoMoveProgressNow(); progress != nil {
		view.Done, view.Total = progress.Done, progress.Total
	}
	if outside && !kept {
		key := strconv.Itoa(siteID)
		putBack, _ := p.sitesPutBack(ctx)
		marks, err := p.readSiteAutoMarks(ctx)
		if _, ok := putBack[key]; ok {
			view.Offer = "move"
		} else if mark, ok := marks.Sites[key]; err == nil && ok && siteAutoMarkOffersRetry(mark.State) {
			view.Offer, view.State, view.Reason = "retry", mark.State, mark.Reason
		}
	}
	if view.Total == 0 && view.Offer == "" {
		return nil
	}
	return view
}

func siteAutoMarkOffersRetry(state string) bool {
	switch state {
	case siteAutoFailed, siteAutoTimeUp, siteAutoUnknown, siteAutoKept, siteAutoLeft:
		return true
	}
	return false
}

// siteAutoMoveProblems: ONE line per site whose move (the worker's, or the
// administrator's Try again) did not bring it inside its limits, while it is
// still outside and no record of it is kept (site_move_kept says that one):
// why, the administrator acts, Try again. A site whose tasks stayed in cron
// for the server owner's reason is named where it is today (its card, the
// owner's lines), never as a to-do.
func (p *Panel) siteAutoMoveProblems(ctx context.Context, names map[string]string) []monitoringProblem {
	marks, err := p.readSiteAutoMarks(ctx)
	if err != nil || len(marks.Sites) == 0 {
		return nil
	}
	outside, _, unsupported := p.siteOutsideNow(ctx)
	if unsupported {
		return nil
	}
	kept := map[string]string{}
	if raw := p.setting(ctx, settingSiteMoveKept); raw != "" {
		_ = json.Unmarshal([]byte(raw), &kept)
	}
	putBack, _ := p.sitesPutBack(ctx)
	domains := map[string]int{}
	if rows, err := p.db.GetDB().QueryContext(ctx, `SELECT s.id, d.id FROM sites s JOIN domains d ON d.id = s.domain_id`); err == nil {
		for rows.Next() {
			var siteID, domainID int
			if rows.Scan(&siteID, &domainID) == nil {
				domains[strconv.Itoa(siteID)] = domainID
			}
		}
		rows.Close()
	}
	var out []monitoringProblem
	for key, mark := range marks.Sites {
		id, err := strconv.Atoi(key)
		name := names[key]
		if err != nil || name == "" || !outside[id] || mark.State == siteAutoLeft || !siteAutoMarkOffersRetry(mark.State) {
			continue
		}
		if _, ok := kept[key]; ok {
			continue
		}
		if _, ok := putBack[key]; ok {
			continue
		}
		out = append(out, monitoringProblem{
			Key: "site_auto_move:" + key, Kind: "site_auto_move_failed", Severity: "warning", Actor: "administrator",
			Subject: name, Name: mark.State, Reason: mark.Reason, Value: 1, Since: mark.At, ObservedAt: mark.At, DomainID: domains[key],
			Action: monitoringProblemAction{Kind: "retry_move", To: "/domains/" + url.PathEscape(name)},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}
