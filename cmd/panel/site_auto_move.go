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
//     page was read (D-024). What keeps it from moving anything before the
//     panel's update is complete (fix round G1): first the Agent's own rule —
//     its claim (every move, Try again, tasks move, undo and rebuild) refuses
//     "update_running" while any marker of the update's transaction exists
//     (/var/lib/celikpanel-release-transaction: completion.pending and the
//     return's own markers) and "update_unreadable" while that folder cannot
//     be read, whoever holds the server's change lock; then the server's
//     change lock itself (the Agent's readiness says host_lock while the
//     update's scripts hold it), the Agent's own update record (queued or
//     running), and the first wait of five minutes. The panel's own word
//     (an update this process admitted) closes it only in the process that
//     admitted the update. A move the administrator pressed makes it wait.
//   - Once per site per installed panel version for a site that is inside
//     or left for the owner's reason, never a loop: the mark is written
//     before the move is sent (panel_settings site_auto_move_v1; no schema
//     change). A move that was refused, put back, stopped for its time or
//     whose end is not known is not tried again by the worker, in this
//     version or a later one (G4): one Problems now line per site says why,
//     who acts and offers Try again. A refusal that clears by itself (the
//     site being changed, another move, the panel's update, an Agent of
//     another build) is not a failure: the site goes to the end of the run
//     and is tried again later in the same run; after six hours it is marked
//     failed with that reason (nothing was sent), which a later version tries
//     again. A mark "started" whose end this process does not know (the
//     panel stopped in the middle) is settled by reading only: inside its
//     limits it is "moved", otherwise "unknown"; the move is never sent twice.
//   - A site outside its limits while no run is scheduled, running or
//     waiting, whatever its mark (moved, or none), is a line with Try again
//     as well (G2); it is not moved again by itself in this version.
//   - A kept move record stops the run (no next site is moved, as for a
//     press); the existing line and card say what the administrator does, and
//     the run goes on by itself once a reading finds the record settled. A
//     refusal for the site's OWN kept record is not "refused before anything
//     changed": the Agent may first try to settle that record, which can
//     restart the site's programs once; it waits like the other refusals and
//     gives up after six hours, ending in the kept record's line, never in a
//     second move.
//   - An update the administrator asks for while the worker moves a site is
//     refused with words that say when that site's move ends, and the worker
//     then starts no further site for 30 minutes or until an update is
//     admitted (G3; kept in memory: lost with the process, which only lets
//     the worker go on, never moves anything twice).
//   - A site whose tasks the administrator put back into cron on purpose is
//     left alone until it is moved again from its own card
//     (site_tasks_put_back_v1).
//   - While it runs, the administrator reads one calm line ("Sites are being
//     moved into their limits (3 / 12) — you do not need to do anything"); a
//     customer and a reseller read nothing of it.
//
// Her site kendi sınırlarının içine kendiliğinden girer: panelin tek bir
// işçisi dışarıda kalan siteleri sırayla, aynı taşıma çağrısıyla taşır; her
// site sürüm başına bir kez denenir, başarısız olan sonraki sürümde de
// kendiliğinden denenmez ve Şu anki sorunlar'da "Tekrar dene" ile görünür;
// kimse bir düğmeye basmak zorunda değildir. Güncelleme tamamlanmadan hiçbir
// şey taşınmaz: ajan, güncellemenin işlem işaretçilerinden biri durdukça
// (tamamlanma işareti konmadıkça) her taşımayı reddeder; bunun ardından
// sunucunun değişiklik kilidi, ajanın güncelleme kaydı ve ilk beş dakikalık
// bekleme gelir.

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
	// Part (fix round G5) says which part of the press ended it: "tasks"
	// (the scheduled tasks' part; the programs were not moved by this press
	// or are inside), "tasks_moved" (the programs' part, after the tasks
	// moved in the same press), "" (the programs' part; the tasks changed
	// nothing in this press).
	Part string `json:"part,omitempty"`
	// PutBack: the part that ended it put what it had changed back as it was
	// (the programs' put-back, or the tasks' own put-back into cron).
	PutBack bool `json:"put_back,omitempty"`
	// Waited: failed only because a reason that clears by itself did not
	// clear for hours, or the panel's own record could not be read or
	// written: nothing was sent, nothing restarted (G4: not carried).
	Waited bool `json:"waited,omitempty"`
}

// siteAutoMarks are the marks of one installed panel version. Marks of
// another version count as none, except the marks of a move that was
// actually attempted and did not bring the site inside (G4: failed,
// time_up, unknown, kept, and a "started" whose end was never read): they
// are carried, so a failing site is not restarted at every update; they go
// with the administrator's Try again or with the site. A record written
// before fix round G4 (the same keys, no part/put_back/waited) reads the
// same way; a later version reads this one's record by its own rule.
type siteAutoMarks struct {
	Version string                  `json:"version"`
	Sites   map[string]siteAutoMark `json:"sites,omitempty"`
}

// siteAutoMarkCarries: a mark kept across versions (G4).
func siteAutoMarkCarries(mark siteAutoMark) bool {
	switch mark.State {
	case siteAutoTimeUp, siteAutoUnknown, siteAutoKept, siteAutoStarted:
		return true
	case siteAutoFailed:
		return !mark.Waited && !siteAutoWaitedReason(mark.Reason)
	}
	return false
}

// siteAutoWaitedReason: a failed mark's reason that records waiting or the
// panel's own record, never a move that was sent (marks written before G4
// carry no Waited flag).
func siteAutoWaitedReason(code string) bool {
	switch code {
	case "host_busy", "marks_unreadable", "mark_failed", "limits_unread":
		return true
	}
	return siteAutoClearsByItself(code)
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
	for k, v := range stored.Sites {
		if stored.Version == marks.Version {
			marks.Sites[k] = v
			continue
		}
		if siteAutoMarkCarries(v) {
			if v.State == siteAutoStarted {
				// Its end was never read in the version that sent it.
				v.State, v.Reason = siteAutoUnknown, "unsettled"
			}
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
func classifySiteMove(resp transport.MoveSiteSliceResponse, tasks *transport.SiteTasksMoveResponse, outcome siteMovePressOutcome) siteAutoMark {
	tasksMoved := tasks != nil && (tasks.Outcome == transport.SiteTasksMoved || tasks.Outcome == transport.SiteTasksCrontabBack)
	part := ""
	if tasksMoved {
		part = "tasks_moved"
	}
	switch outcome {
	case siteMovePressNoLimits:
		return siteAutoMark{State: siteAutoFailed, Reason: "limits_unread", Waited: true}
	case siteMovePressUnreachable:
		return siteAutoMark{State: siteAutoUnknown, Reason: "agent_unreachable", Part: part}
	}
	code := ""
	if resp.Reason != nil {
		code = resp.Reason.Code
	}
	if tasks != nil && tasks.Outcome == transport.SiteTasksUnknown {
		if tasks.Recovery != nil {
			return siteAutoMark{State: siteAutoKept, Reason: "tasks", Part: "tasks"}
		}
		return siteAutoMark{State: siteAutoUnknown, Reason: "tasks_unknown", Part: "tasks"}
	}
	if code == transport.SiteMoveRefusalKept && !resp.RolledBack {
		return siteAutoMark{State: siteAutoWaitKept, Reason: code}
	}
	if resp.Kept {
		return siteAutoMark{State: siteAutoKept, Reason: code, Part: part}
	}
	if !resp.Done && !resp.RolledBack && !resp.TimeUp && siteAutoClearsByItself(code) {
		return siteAutoMark{State: siteAutoWait, Reason: code}
	}
	if resp.TimeUp {
		return siteAutoMark{State: siteAutoTimeUp, Reason: "time_up", Part: part}
	}
	if resp.RolledBack || !resp.Done {
		if code == "" {
			code = "not_finished"
		}
		// Put back as it was only where the programs' part says so.
		return siteAutoMark{State: siteAutoFailed, Reason: code, Part: part, PutBack: resp.RolledBack}
	}
	if tasks == nil {
		return siteAutoMark{State: siteAutoMoved}
	}
	switch tasks.Outcome {
	case transport.SiteTasksStayed:
		return siteAutoMark{State: siteAutoLeft, Reason: "stayed", Part: "tasks"}
	case transport.SiteTasksNoCron:
		return siteAutoMark{State: siteAutoLeft, Reason: "no_cron", Part: "tasks"}
	case transport.SiteTasksRefused:
		if siteAutoTasksOwnerRefusal(tasks.Refusal) {
			return siteAutoMark{State: siteAutoLeft, Reason: tasks.Refusal, Part: "tasks"}
		}
		if tasks.Refusal == "" {
			return siteAutoMark{State: siteAutoFailed, Reason: "tasks_refused", Part: "tasks"}
		}
		return siteAutoMark{State: siteAutoFailed, Reason: tasks.Refusal, Part: "tasks"}
	case transport.SiteTasksPutBack:
		// The tasks were moved and then put back into cron as they were.
		if tasks.Refusal != "" {
			return siteAutoMark{State: siteAutoFailed, Reason: tasks.Refusal, Part: "tasks", PutBack: true}
		}
		return siteAutoMark{State: siteAutoFailed, Reason: "tasks_put_back", Part: "tasks", PutBack: true}
	}
	return siteAutoMark{State: siteAutoMoved}
}

// noteSiteMoveAttempt records the administrator's own move of a site (Try
// again, or the move of a site whose tasks were put back on purpose) as the
// site's latest attempt: Problems now and the card say what it came to, and
// a site whose tasks moved again is no longer left alone.
func (p *Panel) noteSiteMoveAttempt(ctx context.Context, site siteLimitSite, requestID string, resp transport.MoveSiteSliceResponse, tasks *transport.SiteTasksMoveResponse) {
	mark := classifySiteMove(resp, tasks, siteMovePressRan)
	if tasks != nil {
		switch tasks.Outcome {
		case transport.SiteTasksMoved, transport.SiteTasksAlready, transport.SiteTasksCrontabBack:
			p.noteSitePutBack(ctx, site.SiteID, false)
		}
	}
	if mark.State == siteAutoWait || mark.State == siteAutoWaitKept {
		return // nothing changed: the earlier mark stays what it was
	}
	mark.At, mark.RequestID = metricsTime(time.Now()), requestID
	if err := p.setSiteAutoMark(ctx, site.SiteID, mark); err != nil {
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
	// waiting (fix round G5): why the run waits instead of moving now ("":
	// it moves): update, wanted, host, move, site, record (siteAutoWaitingOf).
	waiting string
	// unreadSince, unreadWhy (G2): the run has not read the sites since then,
	// and why ("agent": the Agent did not answer; "sites": the panel's own
	// list of sites could not be read). Zero once a reading succeeds.
	unreadSince time.Time
	unreadWhy   string
	// wantedAt (G3): the panel's update was refused because this worker was
	// moving a site; it starts no further site for siteAutoMoveUpdateHold or
	// until an update is admitted. Set only by that refusal, never by a page
	// being read or a status poll (D-024); in memory: lost with the process,
	// which only lets the worker go on (it never moves anything twice).
	wantedAt time.Time
	// uncleared (G5): sites whose "started" mark could not be taken away
	// after a refusal before anything changed; such a mark is settled as
	// that, never as a panel restart.
	uncleared map[int]bool
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
	// A run whose reading of the sites fails tries again siteAutoMoveRetryMax
	// apart, without end (G2: it never gives up); after
	// siteAutoMoveUnreadLine without a reading, one Problems now line says so.
	siteAutoMoveUnreadLine = time.Hour
	// siteAutoMoveUpdateHold (G3): how long an update that was refused
	// because the worker was moving a site holds the worker before its next
	// site (unless an update is admitted first).
	siteAutoMoveUpdateHold = 30 * time.Minute
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
	// Waiting (G5): the run waits instead of moving, and why (update: the
	// panel's update; wanted: an update the administrator asked for; host:
	// another change on the server; move: another site's move; site: the
	// site is being changed; record: the panel's own record of the move).
	Waiting string `json:"waiting,omitempty"`
}

// siteAutoMoveProgressNow is the worker's progress while it moves sites (nil
// otherwise, and while it waits for a kept record: that record's own line is
// the one shown).
func siteAutoMoveProgressNow() *siteAutoMoveProgress {
	m := &siteAutoMove
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.moving || m.total == 0 || m.waiting == "kept" {
		return nil
	}
	return &siteAutoMoveProgress{Done: m.done, Total: m.total, Waiting: m.waiting}
}

func (m *siteAutoMover) set(moving bool, done, total int) {
	m.mu.Lock()
	m.moving, m.done, m.total = moving, done, total
	m.mu.Unlock()
}

func (m *siteAutoMover) setWaiting(why string) {
	m.mu.Lock()
	m.waiting = why
	m.mu.Unlock()
}

// siteAutoWaitingOf is what the calm line says while a refusal that clears
// by itself is waited for.
func siteAutoWaitingOf(reason string) string {
	switch reason {
	case "update_running", "update_unreadable", "build":
		return "update"
	case "host_busy":
		return "host"
	case "another_move":
		return "move"
	case "site_busy":
		return "site"
	case "marks_unreadable", "mark_failed":
		return "record"
	case transport.SiteMoveRefusalKept:
		return "kept"
	}
	return "host"
}

// siteAutoMoveIdle: no run is scheduled, running or waiting.
func siteAutoMoveIdle() bool {
	m := &siteAutoMove
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.scheduled
}

// noteSiteAutoUpdateWanted (G3) records that the panel's update was refused
// because the worker was moving a site. Only that refusal calls it.
func noteSiteAutoUpdateWanted() {
	m := &siteAutoMove
	m.mu.Lock()
	m.wantedAt = siteAutoMoveNow()
	m.mu.Unlock()
}

// noteSiteAutoUpdateAdmitted (G3): an update was admitted; from now on the
// update's own gate holds the worker.
func noteSiteAutoUpdateAdmitted() {
	m := &siteAutoMove
	m.mu.Lock()
	m.wantedAt = time.Time{}
	m.mu.Unlock()
}

// siteAutoHoldForUpdate waits before the next site while an update the
// administrator asked for may still be started (G3).
func siteAutoHoldForUpdate() {
	m := &siteAutoMove
	for {
		m.mu.Lock()
		at := m.wantedAt
		m.mu.Unlock()
		if at.IsZero() {
			return
		}
		left := siteAutoMoveUpdateHold - siteAutoMoveNow().Sub(at)
		if left <= 0 {
			m.mu.Lock()
			if m.wantedAt.Equal(at) {
				m.wantedAt = time.Time{}
			}
			m.mu.Unlock()
			return
		}
		m.setWaiting("wanted")
		siteAutoMoveSleep(min(left, siteAutoMoveRetryFirst))
	}
}

func (m *siteAutoMover) noteRead(ok bool, why string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ok {
		m.unreadSince, m.unreadWhy = time.Time{}, ""
		return
	}
	if m.unreadSince.IsZero() {
		m.unreadSince = siteAutoMoveNow()
	}
	m.unreadWhy = why
}

func (m *siteAutoMover) noteUncleared(siteID int, uncleared bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.uncleared == nil {
		m.uncleared = map[int]bool{}
	}
	if uncleared {
		m.uncleared[siteID] = true
	} else {
		delete(m.uncleared, siteID)
	}
}

func (m *siteAutoMover) wasUncleared(siteID int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.uncleared[siteID]
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
				m.scheduled, m.moving, m.done, m.total, m.waiting = false, false, 0, 0, ""
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
	reason    string        // the last such refusal's code
}

// siteAutoMoveRead reads every site's slice and scheduled tasks and records
// them (the outside list, a kept record, the host), as the background
// reading does. It does not give up (fix round G2): a reading that fails is
// tried again siteAutoMoveRetryMax later, and after siteAutoMoveUnreadLine
// without one, Problems now has one line (siteAutoMoveUnreadProblem) that
// goes by itself once a reading succeeds. A reading only, never a move.
func (p *Panel) siteAutoMoveRead() {
	m := &siteAutoMove
	for logged := false; ; {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		ok, why := p.readAllSiteSlicesWhy(ctx)
		cancel()
		m.noteRead(ok, why)
		if ok {
			return
		}
		if !logged {
			logged = true
			log.Printf("site limits: the sites could not be read for the move into their limits (%s); the panel reads them again every %s", why, siteAutoMoveRetryMax)
		}
		siteAutoMoveSleep(siteAutoMoveRetryMax)
	}
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
	p.siteAutoMoveRead()
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
			// The panel stopped while this site's move ran (or its mark
			// could not be taken away after a refusal): its end is settled by
			// reading only, never by moving it again.
			state, reason := siteAutoMoved, "panel_restarted"
			if outside[s.SiteID] {
				state, reason = siteAutoUnknown, "unsettled"
			}
			if outside[s.SiteID] && m.wasUncleared(s.SiteID) {
				// Refused before anything changed; only its mark stayed.
				state, reason = siteAutoFailed, "mark_failed"
				mark.Waited = true
			}
			mark.State, mark.Reason = state, reason
			if err := p.setSiteAutoMark(ctx, s.SiteID, mark); err == nil {
				m.noteUncleared(s.SiteID, false)
			} else {
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
			// Waiting, not moving: the calm line says why (G5).
			m.setWaiting(siteAutoWaitingOf(item.reason))
			siteAutoMoveSleep(wait)
		}
		// An update the administrator asked for while a site moved (G3).
		siteAutoHoldForUpdate()
		m.setWaiting("")
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
		ended := p.siteAutoMoveOne(ctx, item.site)
		switch ended.State {
		case siteAutoWait, siteAutoWaitKept:
			now := siteAutoMoveNow()
			if item.since.IsZero() {
				item.since = now
			}
			item.reason = ended.Reason
			if now.Sub(item.since) >= siteAutoMoveGiveUp {
				// It did not clear in hours. A refusal before anything changed
				// is a failure with its reason (nothing was sent); one for the
				// site's own kept record ends in that record's line, never in
				// a second move (fix round G5).
				giveUp := siteAutoMark{State: siteAutoFailed, Reason: ended.Reason, At: metricsTime(now), Waited: true}
				if ended.State == siteAutoWaitKept {
					giveUp = siteAutoMark{State: siteAutoKept, Reason: ended.Reason, At: metricsTime(now)}
				}
				p.markSiteAuto(ctx, item.site, giveUp)
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
// it wrote (State "" when the site needed nothing or could not be checked,
// so it was not moved), or one of the two waits.
func (p *Panel) siteAutoMoveOne(ctx context.Context, site siteLimitSite) siteAutoMark {
	wait := func(reason string) siteAutoMark { return siteAutoMark{State: siteAutoWait, Reason: reason} }
	// The site as it is now: a site moved meanwhile (from its card, or put
	// back on purpose) is not moved again.
	if putBack, err := p.sitesPutBack(ctx); err != nil {
		return wait("marks_unreadable")
	} else if _, left := putBack[strconv.Itoa(site.SiteID)]; left {
		return siteAutoMark{}
	}
	if marks, err := p.readSiteAutoMarks(ctx); err != nil {
		return wait("marks_unreadable")
	} else if _, marked := marks.Sites[strconv.Itoa(site.SiteID)]; marked {
		return siteAutoMark{}
	}
	if outside, _, unsupported := p.siteOutsideNow(ctx); unsupported || !outside[site.SiteID] {
		return siteAutoMark{}
	}
	// The gate (fix round G1): an update this panel process admitted; the
	// server busy with another change, the update's scripts holding the
	// server's change lock included (host_lock); a move the administrator
	// pressed. Behind them the Agent's own rule refuses every move while a
	// marker of the update's transaction exists or cannot be read, and while
	// its own update record is queued or running (update_running /
	// update_unreadable: a wait, never a failure).
	if siteMoveUpdateInProgress(p, ctx) {
		return wait("update_running")
	}
	if !backgroundHostQuiet(p, ctx) {
		return wait("host_busy")
	}
	siteMoveMu.Lock()
	if siteMoveRunning {
		siteMoveMu.Unlock()
		return wait("another_move")
	}
	siteMoveRunning, siteMoveByWorker = true, true
	siteMoveMu.Unlock()
	held := true
	release := func() {
		if held {
			held = false
			siteMoveMu.Lock()
			siteMoveRunning, siteMoveByWorker = false, false
			siteMoveMu.Unlock()
		}
	}
	defer release()
	// Before anything is sent: the mark. Without it written, nothing is moved
	// (a restart could otherwise send the move a second time).
	requestID := "auto-" + newAuditEventID()
	started := siteAutoMark{State: siteAutoStarted, At: metricsTime(siteAutoMoveNow()), RequestID: requestID}
	if err := p.setSiteAutoMark(ctx, site.SiteID, started); err != nil {
		log.Printf("site limits: %s was not moved into its limits: its mark could not be written (%v)", site.Domain, err)
		return wait("mark_failed")
	}
	moveCtx, cancel := hostingDurableContext(context.Background())
	resp, tasks, outcome := p.pressSiteMove(moveCtx, site, requestID)
	cancel()
	mark := classifySiteMove(resp, tasks, outcome)
	tasksOutcome, tasksError := "", ""
	if tasks != nil {
		tasksOutcome, tasksError = tasks.Outcome, tasks.Error
	}
	p.recordAudit(auditRecord{Action: "site.limits_move", ResourceType: "domain", ResourceID: site.DomainID, Details: auditDetailsText(map[string]any{
		"request_id": requestID, "site": site.Domain, "automatic": true, "done": resp.Done, "rolled_back": resp.RolledBack,
		"kept": resp.Kept, "time_up": resp.TimeUp, "error": resp.Error, "units": len(resp.Units), "tasks": tasksOutcome, "tasks_error": tasksError,
		"outcome": mark.State, "reason": mark.Reason,
	})})
	switch mark.State {
	case siteAutoWait, siteAutoWaitKept:
		// Refused before anything changed (or, for the site's own kept
		// record, after the Agent tried to settle it): no mark, it is tried
		// again later.
		if err := p.setSiteAutoMark(ctx, site.SiteID, siteAutoMark{}); err != nil {
			// The "started" mark stays: settled as this, never as a restart
			// of the panel (fix round G5); not moved by itself meanwhile.
			log.Printf("site limits: the mark of %s could not be taken away again (%v); it is not moved by itself until that mark is settled", site.Domain, err)
			siteAutoMove.noteUncleared(site.SiteID, true)
			mark = siteAutoMark{State: siteAutoFailed, Reason: "mark_failed", Waited: true}
		}
	default:
		mark.At, mark.RequestID = metricsTime(siteAutoMoveNow()), requestID
		p.markSiteAuto(ctx, site, mark)
		if mark.State != siteAutoMoved {
			log.Printf("site limits: %s was not moved into its limits by itself (%s %s): %s", site.Domain, mark.State, mark.Reason, resp.Error)
		}
	}
	// The move is over: the panel's update may be admitted from here on
	// (fix round G3), before the reading that follows.
	release()
	readCtx, readCancel := context.WithTimeout(context.Background(), time.Minute)
	p.readSiteLimitsOf(readCtx, site)
	readCancel()
	return mark
}

// --- What people read ------------------------------------------------------------------

// siteAutoMoveView is the administrator's part of the card about the
// worker: the calm line while it runs (Waiting: it waits, and why), and the
// one move action this card still offers: Try again next to a move that did
// not bring the site inside, or next to a site outside its limits that no
// run is moving (offer "retry", with State, Reason, Part and PutBack), and
// the move of a site whose scheduled tasks were put back into cron on
// purpose (offer "move").
type siteAutoMoveView struct {
	Done    int    `json:"done,omitempty"`
	Total   int    `json:"total,omitempty"`
	Waiting string `json:"waiting,omitempty"`
	Offer   string `json:"offer,omitempty"`
	State   string `json:"state,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Part    string `json:"part,omitempty"`
	PutBack bool   `json:"put_back,omitempty"`
}

// siteAutoOutsideLine says what a site outside its limits, with no kept
// record and not put back on purpose, shows (fix round G2): the mark of a
// move that did not bring it inside; and, while no run is scheduled, running
// or waiting, a site the panel is not moving (marked moved: it was inside
// and is outside again; no mark; a "started" mark nobody settled). line is
// false for a site the run will still take, or left for the owner's reason
// (its card names that and offers Try again, no Problems now line).
func siteAutoOutsideLine(mark siteAutoMark, marked, idle bool) (out siteAutoMark, line, card bool) {
	switch {
	case marked && mark.State == siteAutoLeft:
		return mark, false, true
	case marked && siteAutoMarkOffersRetry(mark.State):
		return mark, true, true
	case !idle:
		return siteAutoMark{}, false, false
	case marked && mark.State == siteAutoStarted:
		return siteAutoMark{State: siteAutoUnknown, Reason: "unsettled", At: mark.At}, true, true
	case marked && mark.State == siteAutoMoved:
		return siteAutoMark{State: "outside", Reason: "outside_again", At: mark.At}, true, true
	case !marked:
		return siteAutoMark{State: "outside"}, true, true
	}
	return siteAutoMark{}, false, false
}

// siteAutoMoveViewOf is the card's part for one site ("outside": its
// programs or tasks run outside its limits now; "kept": a move record of it
// is kept, whose own Try again comes first).
func (p *Panel) siteAutoMoveViewOf(ctx context.Context, siteID int, outside, kept bool) *siteAutoMoveView {
	view := &siteAutoMoveView{}
	if progress := siteAutoMoveProgressNow(); progress != nil {
		view.Done, view.Total, view.Waiting = progress.Done, progress.Total, progress.Waiting
	}
	if outside && !kept {
		key := strconv.Itoa(siteID)
		putBack, putErr := p.sitesPutBack(ctx)
		marks, err := p.readSiteAutoMarks(ctx)
		if _, ok := putBack[key]; ok {
			view.Offer = "move"
		} else if err == nil && putErr == nil {
			mark, marked := marks.Sites[key]
			if shown, _, card := siteAutoOutsideLine(mark, marked, siteAutoMoveIdle()); card {
				view.Offer, view.State, view.Reason, view.Part, view.PutBack = "retry", shown.State, shown.Reason, shown.Part, shown.PutBack
			}
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

// siteAutoMoveProblems: ONE line per site outside its limits that no move
// is bringing inside, while no record of it is kept (site_move_kept says
// that one): a move (the worker's, or the administrator's Try again) that
// did not bring it inside, and, while no run is scheduled, running or
// waiting, a site outside whatever its mark (siteAutoOutsideLine): why, the
// administrator acts, Try again. A site whose tasks stayed in cron for the
// server owner's reason is named where it is today (its card, the owner's
// lines), never as a to-do.
func (p *Panel) siteAutoMoveProblems(ctx context.Context, names map[string]string) []monitoringProblem {
	marks, err := p.readSiteAutoMarks(ctx)
	if err != nil {
		return nil
	}
	outside, _, unsupported := p.siteOutsideNow(ctx)
	if unsupported || len(outside) == 0 {
		return nil
	}
	kept := map[string]string{}
	if raw := p.setting(ctx, settingSiteMoveKept); raw != "" {
		_ = json.Unmarshal([]byte(raw), &kept)
	}
	putBack, err := p.sitesPutBack(ctx)
	if err != nil {
		return nil
	}
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
	idle := siteAutoMoveIdle()
	var out []monitoringProblem
	for id := range outside {
		key := strconv.Itoa(id)
		name := names[key]
		if name == "" {
			continue
		}
		if _, ok := kept[key]; ok {
			continue
		}
		if _, ok := putBack[key]; ok {
			continue
		}
		mark, marked := marks.Sites[key]
		shown, line, _ := siteAutoOutsideLine(mark, marked, idle)
		if !line {
			continue
		}
		out = append(out, monitoringProblem{
			Key: "site_auto_move:" + key, Kind: "site_auto_move_failed", Severity: "warning", Actor: "administrator",
			Subject: name, Name: shown.State, Reason: shown.Reason, Part: shown.Part, PutBack: shown.PutBack,
			Value: 1, Since: shown.At, ObservedAt: shown.At, DomainID: domains[key],
			Action: monitoringProblemAction{Kind: "retry_move", To: "/domains/" + url.PathEscape(name)},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}

// siteAutoMoveUnreadProblem (fix round G2): ONE line once the worker has not
// been able to read the sites for siteAutoMoveUnreadLine: the sites are not
// yet inside their limits, why (Reason: "agent", the Agent did not answer;
// "sites", the panel's own list of sites could not be read), the
// administrator looks at the Agent, and the work continues by itself once a
// reading succeeds (the line goes then).
func siteAutoMoveUnreadProblem(now time.Time) []monitoringProblem {
	m := &siteAutoMove
	m.mu.Lock()
	since, why := m.unreadSince, m.unreadWhy
	m.mu.Unlock()
	if since.IsZero() || now.Sub(since) < siteAutoMoveUnreadLine {
		return nil
	}
	return []monitoringProblem{{
		Key: "site_auto_move_unread", Kind: "site_auto_move_unread", Severity: "warning", Actor: "administrator",
		Name: why, Reason: why, Since: metricsTime(since), ObservedAt: metricsTime(now),
		Action: monitoringProblemAction{Kind: "open", To: "/monitoring"},
	}}
}
