package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/serviceapps"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// The administrator's move of a site's running programs into its limits
// (S4 R2a, docs/SITE-SANDBOX.md section 8).
//
// Facts it rests on (21 Nov 2026, Debian 13 systemd 257 and Ubuntu 24.04
// systemd 255, plain systemctl on throwaway machines): a running unit whose
// file changes from Slice=system.slice to Slice=celiksite_<id>.slice keeps
// running in /system.slice after daemon-reload (systemctl show already
// reports the new Slice=); `systemctl restart` then starts it in
// /celiksite_<id>.slice. A drop-in's Slice= outranks the unit file's in both
// directions, so the effective place is what systemctl show reports.
//
// The move is safe to interrupt, and its record is written AHEAD of every
// write (S4 R2a second fix round, review B1). Before its first write it
// records in the Agent's private folder (siteMoveJournalPath), synced, the
// pre-move bytes and mode of every unit file it will change, the slice file's
// bytes and mode (or that there was none) with its ledger digest, and each
// unit's step. Before a unit file is written its step becomes "writing";
// after the slice file is written its new bytes are recorded. A record that
// cannot be written is a failed step: nothing further is written and what
// was written is put back.
//
// At the Agent's next start (recoverSiteSliceMoveAtStart) the step name alone
// is never trusted: for every unit the file on disk is compared with the
// recorded pre-move bytes and with the text the move intended. alpha.149 fix
// round (F1): the start only reads the record and reconciles it with the
// files; it never starts, restarts or waits for a site's program:
//
//	pre-move bytes, step planned or writing   -> not touched ("not_tried")
//	pre-move bytes, a later step               -> already put back: left exactly as it is, running or not
//	                                              (a program its owner stopped stays stopped); its outcome
//	                                              is what this move's own record said, or, where the
//	                                              put-back was cut short, one reading: running in the system
//	                                              slice without a break for its start wait (and without
//	                                              ports: the start reads none) "put_back", running there
//	                                              otherwise "unverified", not running "waiting" (not
//	                                              restarted)
//	the move's text, stopped unit              -> kept ("rewritten"; it starts in the slice)
//	the move's text, running unit, step moved  -> kept ("moved")
//	the move's text, running unit, other step  -> pre-move bytes back, ONE daemon-reload for all, and ONE
//	                                              restart request for all of them without waiting for the
//	                                              jobs (restart --no-block u1 ... uN) unless it still runs
//	                                              in the system slice: "unverified" (not known yet; read
//	                                              later, never waited for)
//	anything else, or no file                  -> left as it is ("changed")
//
// Second fix round (R5): the start's calls to systemd for the record are at
// most four (the reading, the reload, reset-failed, the restart request) and
// share one total bound (siteMoveStartBound); when it runs out nothing
// further is requested and the record stays.
//
// The administrator's Try again and a move's attempt on the same site
// (recoverSiteSliceMove) use the same table but restart a program that ran
// before and does not run where it ran, and check it within the press's one
// deadline (siteMoveBudget, shared with the move that follows: R4).
//
// "Put back" always writes the pre-move bytes recorded before the first
// write, and only over the move's own text: a file that no longer holds what
// the move wrote (a customer's change, a deleted site) is never overwritten
// or re-created. The slice file: when every unit is settled, no unit file
// names the site's slice after the recovery and the file still holds the
// bytes the move wrote, the recorded original comes back (its earlier bytes
// and ledger digest, or no file where there was none); otherwise it stays. A
// move or recovery whose put-back did not complete keeps its record (third
// fix round, review N2: a failed restart or daemon-reload, a place systemd
// could not tell, a start still running at its wait, a restart the Agent's
// bounded start did not send - alpha.148 fix round): no site is moved until
// the next start, the next move's attempt or the administrator's "Try again"
// (MoveSiteSliceRequest.Recover) finishes it, and the card names each
// program left. What the Agent did is kept for the site's card
// (siteMoveOutcomesPath: reason, who acts, next step on the screen) until the
// site's next move.
//
// Every unit step takes that program's own mutation lock (the one every
// other change of the program takes; one lock per program, shared with no
// other), and the whole move holds the site's own lock without waiting for
// it: a site being deleted or changed is not moved, and a customer's change
// of a program waits for that program's step (at most twice its start wait:
// the restart, and a put-back), never lost. Changes of other programs and of
// other sites do not wait for the move.
//
// Taşıma yarıda kesilebilir: ilk yazmadan önce her birim dosyasının taşıma
// öncesi baytları ve ulaşılan adım Ajan'ın özel klasörüne yazılır; Ajan
// açılışta her birimi adımına göre tamamlar ya da geri koyar.

var (
	siteMoveJournalPath  = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-slice-move.json")
	siteMoveOutcomesPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-slice-move-outcomes.json")
)

// Steps of one unit in the journal.
const (
	siteMoveStepPlanned    = "planned"
	siteMoveStepWriting    = "writing"
	siteMoveStepWritten    = "written"
	siteMoveStepRestarting = "restarting"
	siteMoveStepMoved      = "moved"
	siteMoveStepPutBack    = "put_back"
	siteMoveStepChanged    = "changed"
)

type siteMoveJournalUnit struct {
	Unit    string `json:"unit"`
	File    string `json:"file"`
	Mode    uint32 `json:"mode"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Running bool   `json:"running"`
	Step    string `json:"step"`
	// WaitSeconds is the unit's start wait (third fix round): a restart in
	// recovery waits at most this long (0 in an earlier record: systemd's
	// default).
	WaitSeconds int `json:"wait_seconds,omitempty"`
	// Ports (alpha.149, additive; an earlier record has none) are the ports
	// it listened on before the move: a put-back at the Agent's start checks
	// that it listens on them again, as the move does.
	Ports []int `json:"ports,omitempty"`
}

// siteMoveJournalPorts is a recorded unit's ports as a set.
func siteMoveJournalPorts(u siteMoveJournalUnit) map[int]bool {
	out := map[int]bool{}
	for _, p := range u.Ports {
		out[p] = true
	}
	return out
}

// siteMoveJournalWait is a recorded unit's start wait.
func siteMoveJournalWait(u siteMoveJournalUnit) time.Duration {
	if u.WaitSeconds <= 0 {
		return siteSliceSettleDefault
	}
	return time.Duration(u.WaitSeconds) * time.Second
}

type siteMoveJournal struct {
	Version      int    `json:"version"`
	SiteID       int    `json:"site_id"`
	Slice        string `json:"slice"`
	RequestID    string `json:"request_id"`
	StartedAt    string `json:"started_at"`
	SliceFile    string `json:"slice_file"`
	SliceExisted bool   `json:"slice_existed"`
	// SliceBefore and SliceMode: the slice file's bytes and mode before the
	// move ("" where there was none); SliceLedger: its ledger digest then;
	// SliceAfter: the bytes the move wrote, recorded right after the write
	// ("" when it wrote none, or the record did not follow).
	SliceBefore string                `json:"slice_before,omitempty"`
	SliceMode   uint32                `json:"slice_mode,omitempty"`
	SliceLedger string                `json:"slice_ledger,omitempty"`
	SliceAfter  string                `json:"slice_after,omitempty"`
	Units       []siteMoveJournalUnit `json:"units"`
	// Tasks (S4 R2b): the move of the site's crontab into timers or its undo
	// (site_tasks_move.go); a record holds either units or tasks.
	Tasks *siteTasksJournal `json:"tasks,omitempty"`
}

// writePrivateFile writes a file of the Agent's private folder durably: a
// new file beside it, synced, renamed into place.
func writePrivateFile(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// The rename reaches the disk with its folder.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
	return nil
}

// siteMoveJournalWrite writes the move's record (a seam: tests make it fail
// as a full disk would).
var siteMoveJournalWrite = writePrivateFile

func (j *siteMoveJournal) save() error {
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return siteMoveJournalWrite(siteMoveJournalPath, raw)
}

func (j *siteMoveJournal) step(unit, step string) error {
	for i := range j.Units {
		if j.Units[i].Unit == unit {
			j.Units[i].Step = step
		}
	}
	return j.save()
}

func readSiteMoveJournal() (*siteMoveJournal, error) {
	raw, err := os.ReadFile(siteMoveJournalPath)
	if err != nil {
		return nil, err
	}
	var j siteMoveJournal
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// --- What the Agent did with an interrupted move (for the card) -------------

var siteMoveOutcomesMu sync.Mutex

func readSiteMoveOutcomes() map[string]transport.SiteSliceMoveRecovery {
	out := map[string]transport.SiteSliceMoveRecovery{}
	if raw, err := os.ReadFile(siteMoveOutcomesPath); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func changeSiteMoveOutcomes(change func(map[string]transport.SiteSliceMoveRecovery) bool) {
	siteMoveOutcomesMu.Lock()
	defer siteMoveOutcomesMu.Unlock()
	all := readSiteMoveOutcomes()
	if !change(all) {
		return
	}
	raw, err := json.Marshal(all)
	if err == nil {
		err = writePrivateFile(siteMoveOutcomesPath, raw)
	}
	if err != nil {
		log.Printf("site slice: record the interrupted move: %v", err)
	}
}

// siteMoveOutcomeKept: a program's outcome that keeps the move's record (its
// put-back is not known to be complete).
func siteMoveOutcomeKept(state string) bool {
	switch state {
	case transport.SiteMoveFailed, transport.SiteMoveUnknown, transport.SiteMoveStarting, transport.SiteMoveWaiting,
		transport.SiteMoveUnverified, transport.SiteMoveNotAnswering:
		return true
	}
	return false
}

// siteMoveOutcomesNow is the outcome record as the status read reports it
// (review N2): the site whose move record is on disk is "kept" (also when
// its outcome could not be written), and a kept outcome whose record the
// server owner removed is no longer kept. While a move runs its record is a
// live one, so the outcomes are reported as written.
//
// alpha.149 fix round (F3), second fix round (R1, R3, R6): "not known yet"
// is resolved here, outside the Agent's start and by reading only. A kept
// program whose outcome is "unverified", "failed", "waiting" or "starting" is
// checked with what systemd shows (show: the status read's own reading; a
// unit it did not read is read once more, with the same properties). It is
// put back (its entry is closed) only when that one reading shows it active
// and running (SubState) in the system slice, up without a break for at
// least its start wait (ActiveEnterTimestampMonotonic against the monotonic
// clock; never under siteSliceSettleMin), its file holds its earlier bytes,
// and, when it listened on ports before the move, it listens on a port now
// (a listener table that cannot be read leaves it as it is). A program in
// systemd's restart cycle is never closed by one lucky reading. An
// "unverified" program that is stopped, failed or inside the restart cycle
// becomes "failed" (not running: the card and Problems now name it with the
// administrator's action); anything else stays as it is. "not_answering" and
// "unknown" are left to Try again. Once nothing of the record is left in a
// kept state, the record is closed: its slice file is put back as a recovery
// puts it back (restoreSiteMoveSlice) and systemd reloaded, only while the
// slice lock (siteSliceMu) can be had at once; otherwise the closing is left
// for the next read. A read never starts or restarts anything.
func siteMoveOutcomesNow(show map[string]map[string]string) map[string]transport.SiteSliceMoveRecovery {
	all := readSiteMoveOutcomes()
	// systemd is read before the record's lock is taken: its answer may take
	// its time, and a move must not wait for a status read.
	look := siteMoveKeptReadings(all, show)
	if !siteMoveRunning.TryLock() {
		return all
	}
	defer siteMoveRunning.Unlock()
	journal, err := readSiteMoveJournal()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return all // a record that cannot be read: the outcomes as written
	}
	keptSite := ""
	if err == nil {
		keptSite = strconv.Itoa(journal.SiteID)
		all = resolveKeptSiteMove(journal, look)
		if _, statErr := os.Stat(siteMoveJournalPath); errors.Is(statErr, os.ErrNotExist) {
			keptSite = ""
		}
	}
	for site, o := range all {
		if o.Kept && site != keptSite {
			o.Kept = false
			all[site] = o
		}
	}
	if keptSite != "" {
		o := all[keptSite]
		if !o.Kept {
			o.Kept = true
			if o.At == "" {
				o.At = journal.StartedAt
			}
			if o.Units == nil {
				o.Units = map[string]string{}
			}
			all[keptSite] = o
		}
	}
	return all
}

// siteMoveReadSettles: the kept states a status read may close (second fix
// round, R3): "not known yet", and "not running", "not restarted yet" or
// "still starting", whose program may run in its place by now (started on its
// page, or by systemd after a passing moment: R9).
func siteMoveReadSettles(state string) bool {
	switch state {
	case transport.SiteMoveUnverified, transport.SiteMoveFailed, transport.SiteMoveWaiting, transport.SiteMoveStarting:
		return true
	}
	return false
}

// siteMoveKeptReadings is what systemd shows of every kept program whose
// outcome a status read may close: from the status read's own reading, the
// others read once (a reading that fails leaves them as they are).
func siteMoveKeptReadings(all map[string]transport.SiteSliceMoveRecovery, show map[string]map[string]string) map[string]map[string]string {
	out := map[string]map[string]string{}
	var missing []string
	for _, record := range all {
		if !record.Kept {
			continue
		}
		for unit, state := range record.Units {
			if !siteMoveReadSettles(state) || !siteSliceUnitPattern.MatchString(unit) {
				continue
			}
			if props, ok := show[unit]; ok {
				out[unit] = props
			} else {
				missing = append(missing, unit)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if read, err := showUnits(missing); err == nil {
			for unit, props := range read {
				out[unit] = props
			}
		}
	}
	return out
}

// siteUnitRunsSettled is a status read's test of a kept program (second fix
// round, R1), on ONE reading of systemd: active and running (SubState) in the
// system slice, up without a break for at least its start wait (never under
// siteSliceSettleMin), its file holding its earlier bytes and, when it had
// ports before the move, listening on a port now. Anything short of that,
// or anything that cannot be read, is not settled.
func siteUnitRunsSettled(u siteMoveJournalUnit, props map[string]string) bool {
	if props["ActiveState"] != "active" || props["SubState"] != "running" || !sitesandbox.UnitInSystemSlice(props["ControlGroup"], u.Unit) {
		return false
	}
	need := siteMoveJournalWait(u)
	if need < siteSliceSettleMin {
		need = siteSliceSettleMin
	}
	if up, known := siteUnitUptime(props); !known || up < need {
		return false
	}
	if current, _, _, err := siteUnitText(u.Unit); err != nil || current != u.Before {
		return false
	}
	if len(u.Ports) > 0 {
		if now, read := siteSliceListening(u.Unit); !read || len(now) == 0 {
			return false
		}
	}
	return true
}

// siteUnitUptime is how long a unit has been active without a break, from
// the ActiveEnterTimestampMonotonic of the same reading (known false when it
// is missing, zero or ahead of the clock).
func siteUnitUptime(props map[string]string) (time.Duration, bool) {
	enter, err := strconv.ParseUint(strings.TrimSpace(props["ActiveEnterTimestampMonotonic"]), 10, 64)
	if err != nil || enter == 0 {
		return 0, false
	}
	now, ok := siteMonotonicNow()
	if !ok || now < enter {
		return 0, false
	}
	return time.Duration(now-enter) * time.Microsecond, true
}

// siteMoveRecordSettled: the kept outcome record is this move's own (written
// when it began or later), names every program of the move's record, and none
// of them is left in a kept state.
func siteMoveRecordSettled(journal *siteMoveJournal, record transport.SiteSliceMoveRecovery) bool {
	if !record.Kept || record.At == "" || record.At < journal.StartedAt {
		return false
	}
	for _, u := range journal.Units {
		state, ok := record.Units[u.Unit]
		if !ok || siteMoveOutcomeKept(state) {
			return false
		}
	}
	return true
}

// resolveKeptSiteMove applies those readings to the kept record's programs
// (the caller holds the record's lock) and answers the outcomes as they are
// now. When no program of the record is left in a kept state, the record is
// closed (second fix round, R6): the slice file comes back and systemd
// reloads only under the slice lock, taken in the move's order (the record's
// lock, then siteSliceMu) and only if it is free at once; otherwise the
// closing is left for the next read.
func resolveKeptSiteMove(journal *siteMoveJournal, look map[string]map[string]string) map[string]transport.SiteSliceMoveRecovery {
	if journal.Tasks != nil {
		// A record of a site's scheduled tasks (S4 R2b) is settled only by
		// an action (the Agent's start, Try again, the next move's attempt),
		// never by a reading.
		return readSiteMoveOutcomes()
	}
	site := strconv.Itoa(journal.SiteID)
	current := readSiteMoveOutcomes()[site]
	// Each candidate is checked before the outcome record's lock is taken
	// (its file and its listeners are read).
	verdicts := map[string]string{}
	if current.Kept {
		for _, u := range journal.Units {
			state, props := current.Units[u.Unit], look[u.Unit]
			if props == nil || !siteMoveReadSettles(state) {
				continue
			}
			switch active := props["ActiveState"]; {
			case siteUnitRunsSettled(u, props):
				verdicts[u.Unit] = transport.SiteMovePutBack
			case state == transport.SiteMoveUnverified &&
				(active == "failed" || active == "inactive" || (active == "activating" && siteUnitAutoRestart(props))):
				verdicts[u.Unit] = transport.SiteMoveFailed
			}
		}
	}
	settled := false
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		record, ok := all[site]
		if !ok || !record.Kept {
			return false
		}
		changed := false
		for unit, verdict := range verdicts {
			if state := record.Units[unit]; state == current.Units[unit] && state != verdict {
				record.Units[unit], changed = verdict, true
			}
		}
		settled = siteMoveRecordSettled(journal, record)
		if !changed {
			return false
		}
		all[site] = record
		return true
	})
	if !settled {
		return readSiteMoveOutcomes()
	}
	// Settled: the slice file comes back as a recovery puts it back (no
	// program is started or restarted by that), then the record goes.
	if !siteSliceMu.TryLock() {
		return readSiteMoveOutcomes() // a limits change holds the slice lock: the next read closes it
	}
	restored, failed := restoreSiteMoveSlice(journal)
	if restored {
		ctx, cancel := context.WithTimeout(context.Background(), siteMoveStartBound)
		_ = siteSliceDaemonReloadWith(func(args ...string) ([]byte, error) { return siteMoveCall(ctx, args...) })
		cancel()
	}
	siteSliceMu.Unlock()
	if failed {
		return readSiteMoveOutcomes() // the record stays; Try again finishes it
	}
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		record := all[site]
		record.Kept = false
		all[site] = record
		return true
	})
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site slice: remove the move's record: %v", err)
	} else {
		log.Printf("site slice: the interrupted move of site %d is settled: every program it left runs where it ran (seen by a status read)", journal.SiteID)
	}
	return readSiteMoveOutcomes()
}

func forgetSiteMoveOutcome(siteID int) {
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		if _, ok := all[strconv.Itoa(siteID)]; !ok {
			return false
		}
		delete(all, strconv.Itoa(siteID))
		return true
	})
}

// --- Seams --------------------------------------------------------------------

var (
	// siteSliceSettleMin, siteSliceSettleDefault and siteSliceSettleMax bound
	// how long the move waits for one program to run in the slice and answer
	// again: the unit's own start timeout (TimeoutStartUSec); these units set
	// none, so systemd's default of 90 seconds applies; never under 20 or
	// over 300 seconds.
	siteSliceSettleMin     = 20 * time.Second
	siteSliceSettleDefault = 90 * time.Second
	siteSliceSettleMax     = 300 * time.Second
	// siteMoveBudget is the whole move's time (review B8). The rule
	// (siteMoveFits): a running program is restarted only while what is
	// left of the budget holds its own start wait, its own put-back (a
	// restart that may take its start wait again), the put-back of every
	// program moved before it, and a margin. The budget is 11 minutes, so
	// the first program always fits even with the longest wait (2 x 300 s +
	// 5 s), and the whole move, put-backs included, stays below the Panel's
	// 12 minute bound of the call. A move that stops for time answers which
	// programs are done (they stay moved) and which were not started; the
	// next move continues with those.
	// The same budget bounds the administrator's Try again and a move's
	// attempt that finds a kept record of its own site (alpha.149 fix round,
	// F4): a put-back restart is sent only while what is left holds its start
	// wait and the margin; the rest are "waiting" (not restarted yet). Second
	// fix round (R4): one press, one deadline: the attempt and the move that
	// follows share this budget, started when the press is claimed.
	siteMoveBudget = 11 * time.Minute
	siteMoveMargin = 5 * time.Second
	siteSlicePoll  = 500 * time.Millisecond

	// siteMoveUpdateState (alpha.149 fix round, F6; second fix round, R2;
	// alpha.150 automatic move fix round, G1): whether the panel's own update
	// may be in progress, read only: "" none, "update_running" or, for what
	// cannot be read, "update_unreadable" (siteMoveUpdateStateOf).
	siteMoveUpdateState = func() string {
		return siteMoveUpdateStateOf(siteMoveReleaseTransactionRoot, systemUpdateStateRoot, time.Now())
	}
	// siteMoveReleaseTransactionRoot is the update's transaction folder
	// (/var/lib/celikpanel-release-transaction; tests replace it).
	siteMoveReleaseTransactionRoot = agentReleaseTransactionRoot

	// siteMoveStartBound (second fix round, R5) is the one total bound of the
	// systemd calls the Agent's start makes for a kept move record (the
	// reading, the reload, reset-failed and the one restart request), and of a
	// status read's reload when it closes a record. When it runs out nothing
	// further is requested. 15 seconds holds systemd's daemon-reload on a busy
	// server many times over (it takes well under a second to a few seconds),
	// and a reload cut short only leaves the record kept: Try again reloads.
	siteMoveStartBound = 15 * time.Second

	// siteMoveSystemctl runs one systemctl call within ctx: a call still
	// running when ctx ends is stopped (systemd keeps any job it already
	// queued; what the call did is then not known). Tests replace it.
	siteMoveSystemctl = func(ctx context.Context, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	}

	// siteMonotonicNow is the monotonic clock systemd's timestamps are on
	// (R1; tests replace it).
	siteMonotonicNow = monotonicClockUSec

	// siteSliceListening reads the ports the unit's processes listen on; read
	// is false when the listener table could not be read (second fix round,
	// R8: that is "not known yet", never "listens on no port").
	siteSliceListening = func(unit string) (map[int]bool, bool) {
		listeners, pids, err := nodeSiteListeners(unit)
		if err != nil {
			return nil, false
		}
		ports := map[int]bool{}
		for _, l := range listeners {
			for _, pid := range l.PIDs {
				if pids[pid] {
					ports[l.Port] = true
				}
			}
		}
		return ports, true
	}

	// siteMoveKilled simulates the Agent stopping right after a step (tests
	// only): when it answers true the move returns at once, leaving the
	// journal for recoverSiteSliceMove. Points: "journal", "slice" (written,
	// not yet recorded), "slice_recorded"; per unit "writing" (recorded, not
	// yet written), "wrote" (written, not yet recorded), "written",
	// "restarting" (recorded, not yet sent), "restarted" (sent, not yet seen
	// running) and "moved".
	siteMoveKilled = func(unit, step string) bool { return false }

	// siteUnitRestart restarts a unit and waits for systemctl's answer at
	// most wait (S4 R2a third fix round, the review's B8 residue): a unit's
	// own TimeoutStartSec (even infinity) never holds the move longer than
	// its start wait. A start still running then answers
	// errSiteUnitStillStarting; systemd keeps that start job (only the
	// waiting client stops), so the program may still come up.
	siteUnitRestart = func(unit string, wait time.Duration) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), wait)
		defer cancel()
		out, err := exec.CommandContext(ctx, "systemctl", systemctlArgs("restart", unit)...).CombinedOutput()
		if ctx.Err() != nil {
			return out, errSiteUnitStillStarting
		}
		return out, err
	}
)

// errSiteUnitStillStarting: a restart whose start was still running at its
// start wait.
var errSiteUnitStillStarting = errors.New("still starting")

// siteMoveCall is one systemctl call within ctx (second fix round, R5): once
// ctx has ended nothing further is sent.
func siteMoveCall(ctx context.Context, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return siteMoveSystemctl(ctx, args...)
}

// siteUnitRestartFailure says why a restart did not answer and which
// recovery outcome that is: "starting" for a start still running at its
// wait, "failed" (it is stopped) otherwise; with the reason's code.
func siteUnitRestartFailure(unit string, wait time.Duration, out []byte, err error) (string, *transport.SiteMoveReason, error) {
	if errors.Is(err, errSiteUnitStillStarting) {
		return transport.SiteMoveStarting, &transport.SiteMoveReason{Code: "still_starting", Seconds: int(wait / time.Second)},
			fmt.Errorf("it was still starting %d seconds after the restart (journalctl -u %s says why)", int(wait/time.Second), unit)
	}
	return transport.SiteMoveFailed, &transport.SiteMoveReason{Code: "restart_failed"},
		fmt.Errorf("systemctl restart: %v: %s", err, strings.TrimSpace(string(out)))
}

// What the wait for a restarted unit found (alpha.149, lab D6/D7).
const (
	siteRunVerdictRuns         = "runs"
	siteRunVerdictNotRunning   = "not_running"
	siteRunVerdictStarting     = "still_starting"
	siteRunVerdictNotInPlace   = "not_in_slice"
	siteRunVerdictNotListening = "not_listening"
	siteRunVerdictUnread       = "unread"
	// siteRunVerdictNoPort (alpha.149 fix round, F10): a put-back program
	// that had ports listens on none at all after its start wait.
	siteRunVerdictNoPort = "no_port"
	// siteRunVerdictPortsUnread (second fix round, R8): it runs in its place,
	// but the listener table could not be read: not known yet.
	siteRunVerdictPortsUnread = "ports_unread"
	// siteRunVerdictRunsNoPort (R8): after the administrator's Try again
	// restarted a "not answering" program once more, it ran in its place
	// without a break for its start wait and listens on no port.
	siteRunVerdictRunsNoPort = "runs_no_port"
)

// siteUnitListensAgain is the port check of a program that runs in its
// place. The move's own step (anyPort false) wants every port it listened on
// before. A put-back (anyPort true, alpha.149 fix round F10) wants it to
// listen on a port at all: a program put back where it ran that answers on a
// changed port runs, and keeps no record. known is false when the listener
// table could not be read (second fix round, R8).
func siteUnitListensAgain(unit string, ports map[int]bool, anyPort bool) (listens, known bool) {
	if len(ports) == 0 {
		return true, true // a program without ports runs as soon as it is active in its place
	}
	now, read := siteSliceListening(unit)
	if !read {
		return false, false
	}
	if anyPort {
		return len(now) > 0, true
	}
	for port := range ports {
		if !now[port] {
			return false, true
		}
	}
	return true, true
}

// waitSiteUnitRuns polls a restarted unit until deadline the way the move's
// own step always did, and since alpha.149 the put-back too (lab D6: a
// Type=simple unit's restart succeeds as soon as its process is executed, so
// systemctl's exit status alone never proves that the program runs): it is
// active, in the place inPlace accepts (its ControlGroup) and listens again
// (siteUnitListensAgain). It answers the verdict and the last reading. A
// unit that does not run when the time is up is "not_running" (failed,
// stopped, or inside systemd's automatic restart cycle: SubState
// auto-restart, no process), one still in its first start "still_starting";
// one in its place whose listener table could not be read "ports_unread"
// (second fix round, R8: not known yet, never "listens on no port").
func waitSiteUnitRuns(unit string, inPlace func(cgroup string) bool, ports map[int]bool, anyPort bool, deadline time.Time) (string, map[string]string) {
	for {
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,ControlGroup,SubState")...); err == nil {
			props = parseShow(string(out))
		}
		state := props["ActiveState"]
		placed := state == "active" && inPlace(props["ControlGroup"])
		portsKnown := true
		if placed {
			listens, known := siteUnitListensAgain(unit, ports, anyPort)
			if listens {
				return siteRunVerdictRuns, props
			}
			portsKnown = known
		}
		if !time.Now().Before(deadline) {
			switch {
			case state == "":
				return siteRunVerdictUnread, props
			case state == "active" && !placed:
				return siteRunVerdictNotInPlace, props
			case placed && !portsKnown:
				return siteRunVerdictPortsUnread, props
			case state == "active" && anyPort:
				return siteRunVerdictNoPort, props
			case state == "active":
				return siteRunVerdictNotListening, props
			case state == "activating" && !siteUnitAutoRestart(props):
				return siteRunVerdictStarting, props
			}
			return siteRunVerdictNotRunning, props
		}
		time.Sleep(siteSlicePoll)
	}
}

// waitSiteUnitRunsWithoutPort is the wait after the administrator's Try
// again restarted a "not answering" program once more (second fix round,
// R8): like a put-back's wait, a port at all is "runs"; at the end of its
// start wait, a program that is active and running (SubState) in the system
// slice, and has been up without a break since it was first seen so in this
// wait (the same ActiveEnterTimestampMonotonic; a restart cycle changes it),
// is "runs_no_port" when the listener table was read and shows no port; one
// that went down and came up again within the wait stays "no_port" (not
// answering); "ports_unread" when the table could not be read. Anything else
// is the put-back's verdict.
func waitSiteUnitRunsWithoutPort(unit string, ports map[int]bool, deadline time.Time) (string, map[string]string) {
	firstEnter := ""
	steady := true
	for {
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,ControlGroup,SubState,ActiveEnterTimestampMonotonic")...); err == nil {
			props = parseShow(string(out))
		}
		up := props["ActiveState"] == "active" && props["SubState"] == "running" && sitesandbox.UnitInSystemSlice(props["ControlGroup"], unit)
		enter := props["ActiveEnterTimestampMonotonic"]
		portsKnown := true
		if up {
			if firstEnter == "" {
				firstEnter = enter
			}
			steady = steady && enter == firstEnter && enter != "" && enter != "0"
			listens, known := siteUnitListensAgain(unit, ports, true)
			if listens {
				return siteRunVerdictRuns, props
			}
			portsKnown = known
		} else if firstEnter != "" {
			steady = false // it stopped after it was seen up
		}
		if !time.Now().Before(deadline) {
			switch {
			case up && !portsKnown:
				return siteRunVerdictPortsUnread, props
			case up && steady:
				return siteRunVerdictRunsNoPort, props
			case up:
				return siteRunVerdictNoPort, props // it went down and came up again within the wait
			}
			return waitSiteUnitRuns(unit, func(cgroup string) bool { return sitesandbox.UnitInSystemSlice(cgroup, unit) }, ports, true, deadline)
		}
		time.Sleep(siteSlicePoll)
	}
}

// siteUnitAutoRestart: systemd waits to start the unit again by itself (its
// program exited; nothing of it runs now).
func siteUnitAutoRestart(props map[string]string) bool {
	return strings.HasPrefix(props["SubState"], "auto-restart")
}

// siteUnitStateWords is what systemd showed, for the reason's State.
func siteUnitStateWords(props map[string]string) string {
	return strings.TrimSpace(props["ActiveState"] + " " + props["SubState"])
}

// siteUnitPlainState is the unit's state in one plain word for the move's
// English answer (journal and audit record): systemd's two words repeat one
// another ("failed failed", alpha.149 lab W2). The reason's State keeps them.
// Birimin durumu tek düz sözcükle; gerekçenin State alanı systemd'nin sözünü korur.
func siteUnitPlainState(props map[string]string) string {
	switch props["ActiveState"] {
	case "failed":
		return "failed"
	case "inactive":
		return "stopped"
	case "activating":
		if siteUnitAutoRestart(props) {
			return "restarting"
		}
		return "starting"
	case "deactivating":
		return "stopping"
	case "active":
		return "running"
	case "reloading":
		return "reloading"
	case "maintenance":
		return "in maintenance"
	}
	return "in an unknown state"
}

// siteSliceStartWait is a unit's start wait from its TimeoutStartUSec.
func siteSliceStartWait(timeoutStart string) time.Duration {
	wait := siteSliceSettleDefault
	if usec, ok := systemdTimespanUSec(timeoutStart); ok && usec > 0 {
		wait = time.Duration(usec) * time.Microsecond
	}
	if wait < siteSliceSettleMin {
		wait = siteSliceSettleMin
	}
	if wait > siteSliceSettleMax {
		wait = siteSliceSettleMax
	}
	return wait
}

// siteMovePrepare puts this release's sandbox helper and SMTP block files in
// place before a move (a seam for tests).
var siteMovePrepare = func() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := ensureSandboxHelper(); err != nil {
		return err
	}
	return ensureSandboxEgress(sandboxCaps(ctx))
}

// siteUnitLock takes the mutation lock every other change of this program
// takes.
func siteUnitLock(unit string) func() {
	kind, idText, _ := strings.Cut(strings.TrimSuffix(unit, ".service"), "-")
	id, err := strconv.Atoi(idText)
	if err != nil {
		return func() {}
	}
	switch kind {
	case "celikapp":
		return lockAppUnitMutation(id)
	case "celikrt":
		return lockRuntimeApp(id)
	case "celiksvc":
		return lockServiceApp(id)
	}
	return func() {}
}

// --- The move --------------------------------------------------------------------

// siteMoveUnit is one unit the move changes.
type siteMoveUnit struct {
	unit    string
	file    string
	mode    os.FileMode
	before  string
	after   string
	running bool
	ports   map[int]bool
	wait    time.Duration
}

var (
	// siteMoveRunning is the move record's lock: a move and a Try again hold
	// it for their whole run; a status read takes it only with TryLock (a
	// live record is then reported as written).
	siteMoveRunning sync.Mutex
	// siteMoveInFlight is set for the whole of a move or a Try again and only
	// by them (alpha.149 fix round, F5): it is what the panel's update reads,
	// and what refuses a second move. Status reads never touch it, so they
	// never make the update or a move refuse.
	siteMoveInFlight atomic.Bool
	// siteMoveUpdateGate orders a move's start against the update's
	// admission (F6): whichever takes it first is the one that goes ahead.
	siteMoveUpdateGate sync.Mutex
)

// claimSiteMove starts a move or a Try again: refused while the panel's own
// update is queued or running ("update_running": it would stop this Agent in
// the middle of the move), while an update record cannot be read
// ("update_unreadable"), or while another move runs ("another_move").
// Otherwise it answers "" and the caller holds the move until release.
//
// Second fix round (R7): the update gate is held only for the update check
// and the setting of the in-flight flag. The update's admission
// (StartSystemUpdate) checks that flag under the same gate, so an update
// admitted after the gate is released sees the move and is refused; one
// admitted before is seen by the check. The record's lock is taken after the
// gate is released, so the update's admission never waits behind a status
// read (or a systemd call one makes).
func claimSiteMove() (refusal string, release func()) {
	siteMoveUpdateGate.Lock()
	if refusal = siteMoveUpdateState(); refusal == "" && !siteMoveInFlight.CompareAndSwap(false, true) {
		refusal = "another_move"
	}
	siteMoveUpdateGate.Unlock()
	if refusal != "" {
		return refusal, nil
	}
	if siteMoveClaimed != nil {
		siteMoveClaimed() // tests: between the two steps
	}
	// The record's lock: a status read holds it only for a moment.
	siteMoveRunning.Lock()
	return "", func() {
		siteMoveRunning.Unlock()
		siteMoveInFlight.Store(false)
	}
}

// siteMoveClaimed runs between the gate's release and the record's lock
// (tests only).
var siteMoveClaimed func()

// siteMovePressDeadline is the deadline of a press's programs part (S4 R2b,
// alpha.150: one press, one deadline). budgetSeconds is what the Panel says
// is left of the press after the site's scheduled tasks moved first: absent
// or 0 means the Agent's own budget; a value is never allowed to give more
// than that budget; a negative one (never sent) fails towards the smaller,
// nothing left. No multiplication can overflow: a value above the budget is
// the budget.
func siteMovePressDeadline(now time.Time, budgetSeconds int) time.Time {
	own := now.Add(siteMoveBudget)
	switch {
	case budgetSeconds == 0:
		return own
	case budgetSeconds < 0:
		return now
	case int64(budgetSeconds) >= int64(siteMoveBudget/time.Second):
		return own
	}
	return now.Add(time.Duration(budgetSeconds) * time.Second)
}

// siteMoveUpdateRefusal is the English text of the update_running refusal;
// siteMoveUpdateUnreadable of update_unreadable (second fix round, R2; G1:
// also the update's transaction folder, which ends only when it can be read).
const (
	siteMoveUpdateRefusal    = "the panel's update is in progress; nothing was moved. Move the site after the update has finished"
	siteMoveUpdateUnreadable = "a record of the panel's update could not be read, so it is not known whether an update is in progress; nothing was moved. " +
		"The Agent's journal names the file; an update request's record stops counting by itself 31 minutes after it last changed"
)

// siteMoveUpdateStateOf (alpha.150 automatic move fix round, G1) is the
// update's gate of every move, Try again, tasks move, undo and rebuild (they
// all claim through claimSiteMove). It is a rule of the Agent's own, read
// only, that fails towards "the update is not finished":
//   - while any marker of the update's transaction exists in
//     /var/lib/celikpanel-release-transaction (quiesce.pending, active,
//     completion.pending, scheduler-restore.pending; written and removed by
//     deploy/release-transaction-guard.sh for update.sh and the finalize
//     scripts, a return's own markers included: a return takes over "active"
//     as operation=rollback and finishes through completion.pending), the
//     answer is "update_running": the update has not reached its completion
//     mark, whether or not its scripts hold the server's change lock right
//     now (a restart of the server in that window frees it between the
//     finalizer's attempts);
//   - a transaction folder or marker that cannot be inspected answers
//     "update_unreadable" (it does not end until it can be read);
//   - otherwise the update's request records (systemUpdateRecordsState).
//
// Güncellemenin işlem işaretçilerinden biri durdukça (tamamlanma işareti
// konmadıkça) hiçbir taşıma başlamaz; okunamayan klasör de reddeder.
func siteMoveUpdateStateOf(transactionRoot, recordsRoot string, now time.Time) string {
	present, err := persistentReleaseTransactionPresent(transactionRoot)
	if err != nil {
		log.Printf("site slice: the panel's update transaction (%s) could not be read (%v); a move is refused until it can be read", transactionRoot, err)
		return "update_unreadable"
	}
	if present {
		return "update_running"
	}
	return systemUpdateRecordsState(recordsRoot, now)
}

// siteMoveRefusalText is the English text of a claim's refusal.
func siteMoveRefusalText(refusal string) string {
	switch refusal {
	case "update_running":
		return siteMoveUpdateRefusal
	case "update_unreadable":
		return siteMoveUpdateUnreadable
	}
	return "another move into a site's limits is running; nothing was changed"
}

// systemUpdateRecordsState reads the panel's update records (read only, no
// lock taken, nothing reconciled; alpha.149 fix round F6, second fix round
// R2). A queued or running record counts as an update in progress
// ("update_running") only while it can still be a live one: created less
// than systemUpdateRecoveryGrace ago (the worker and its recovery are each
// bounded to 15 minutes; the update's own reconciliation fails an older
// record, as it does a record of a dead worker). A record that cannot be read
// refuses the move ("update_unreadable": the safe side) while it changed
// within that grace, and stops refusing by itself after it. A folder that
// cannot be read holds no update (the update itself cannot run without it).
func systemUpdateRecordsState(root string, now time.Time) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	recent := func(at time.Time) bool {
		age := now.Sub(at)
		return age >= 0 && age < systemUpdateRecoveryGrace
	}
	unreadable := ""
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !systemUpdateRequestRE.MatchString(id) {
			continue
		}
		path := filepath.Join(root, e.Name())
		raw, err := os.ReadFile(path)
		var state systemUpdateState
		if err == nil {
			err = json.Unmarshal(raw, &state)
		}
		var created time.Time
		if err == nil {
			created, err = time.Parse(time.RFC3339Nano, state.CreatedAt)
		}
		if err != nil {
			if info, statErr := e.Info(); statErr == nil && recent(info.ModTime()) {
				log.Printf("site slice: the panel's update record %s could not be read (%v); a move is refused until it can be read or is %s old", path, err, systemUpdateRecoveryGrace)
				unreadable = "update_unreadable"
			}
			continue
		}
		if state.active() && recent(created) {
			return "update_running"
		}
	}
	return unreadable
}

// siteMoveShowProperties is what the move reads of a unit before it plans.
const siteMoveShowProperties = "--property=ActiveState,ControlGroup,DropInPaths,TimeoutStartUSec"

// MoveSiteSliceV1 moves a site's units into its slice: the slice is written,
// then each unit in turn under its own lock: its file is rewritten to name
// the slice and, if it was running, it restarts once and must run in the
// slice (and listen again on every port it listened on before) within its
// start wait. A failure puts that unit and every unit of the site moved
// before it back as they were (the earlier text, a restart where it ran);
// other sites are never touched. A unit an owner changed, or placed with a
// drop-in, is kept and listed.
func (a *Agent) MoveSiteSliceV1(req *transport.MoveSiteSliceRequest, resp *transport.MoveSiteSliceResponse) error {
	resp.Units = []transport.SiteSliceMoved{}
	// alpha.149 (lab D7): every answer that is not done carries the code of
	// its reason for the page's language; the English text is the journal's.
	defer func() {
		if resp.Error == "" {
			return
		}
		if resp.Reason == nil {
			resp.Reason = &transport.SiteMoveReason{Code: "not_finished"}
		}
		if !resp.RolledBack && !resp.Kept {
			log.Printf("site slice: a move into a site's limits did not run: %s", resp.Error)
		}
	}()
	refuse := func(code, text string) {
		resp.Error, resp.Reason = text, &transport.SiteMoveReason{Code: code}
	}
	if req == nil {
		refuse("bad_request", "a request is required")
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "moving a site into its limits"); err != nil {
		refuse("build", err.Error())
		return nil
	}
	if req.Recover {
		recoverKeptSiteSliceMove(resp)
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		refuse("unsupported", "this server cannot hold a site's limits (missing: "+reason+"); nothing was moved")
		return nil
	}
	s := siteSliceFor(&req.Site, req.Site.SiteID)
	home, homeErr := hostingpath.SiteHome(req.SubscriptionID, req.DomainID)
	if s == nil || homeErr != nil || len(req.Units) == 0 || len(req.Units) > siteSliceMaxUnits {
		refuse("bad_request", "the site, its home and up to 64 of its units are required")
		return nil
	}
	for _, u := range req.Units {
		if !siteSliceUnitPattern.MatchString(u) {
			refuse("bad_request", "not a site's unit: "+u)
			return nil
		}
	}
	// alpha.149 fix round (F5, F6): a move holds the in-flight flag the
	// panel's update reads, and is refused while that update is in progress.
	refusal, release := claimSiteMove()
	if refusal != "" {
		refuse(refusal, siteMoveRefusalText(refusal))
		return nil
	}
	defer release()
	// Second fix round (R4): one press, one deadline. Settling this site's
	// own kept record and the move that follows share the move's budget,
	// which stays below the Panel's bound of the call.
	// S4 R2b (alpha.150): when the site's scheduled tasks moved first in the
	// same press, the Panel sends what is left of the press's deadline.
	deadline := siteMovePressDeadline(time.Now(), req.BudgetSeconds)
	siteMu := siteMutationMutex(s.SiteID)
	if !siteMu.TryLock() {
		refuse("site_busy", "the site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute")
		return nil
	}
	defer siteMu.Unlock()
	// A journal left by an earlier Agent, or by a move whose put-back did not
	// complete, is resolved before anything new; while it stays, nothing new
	// is moved (review N2). alpha.149 fix round (F4): the record of ANOTHER
	// site refuses this move at once, before anything of that site is
	// restarted (its own Try again settles it); a record of this site is
	// finished within the move's own budget first.
	keptEarlier := func(siteID int) {
		site := "a site"
		if siteID > 0 {
			site = "site " + strconv.Itoa(siteID)
		}
		refuse("kept_earlier", "an earlier move of "+site+" could not be put back completely; its page names each program left and the next step. Nothing new was moved until that is settled")
		resp.Kept = true
	}
	if journal, err := readSiteMoveJournal(); err == nil && journal.SiteID != s.SiteID {
		keptEarlier(journal.SiteID)
		return nil
	}
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		kept := recoverSiteSliceMove(deadline, false)
		if _, err := os.Stat(siteMoveJournalPath); err == nil {
			siteID := 0
			if kept != nil {
				siteID = kept.siteID
			}
			keptEarlier(siteID)
			return nil
		}
	}
	// The units restart with this release's helper and guard: an earlier
	// release's copy adds only system-slice cgroups to the SMTP block, so a
	// unit in its site's slice would not start with it.
	if err := siteMovePrepare(); err != nil {
		refuse("prepare_failed", "the site sandbox's helper and SMTP block could not be put in place: "+err.Error()+"; nothing was moved")
		return nil
	}

	// Classify every unit before anything changes (read-only).
	var plan []siteMoveUnit
	for _, u := range req.Units {
		text, file, mode, err := siteUnitText(u)
		if errors.Is(err, os.ErrNotExist) {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveAbsent})
			continue
		}
		if err != nil {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: err.Error()})
			continue
		}
		body, ours := siteUnitSealedBody(u, text)
		switch {
		case !ours:
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept,
				Detail: file + " was changed on this server; it is kept and runs where it runs. Your own settings belong in " + file + ".d/*.conf"})
			continue
		case serviceapps.UnitGeneration(text) < sitesandbox.Generation:
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveOlder})
			continue
		case !siteUnitInHome(u, body, home):
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveRefused, Detail: "its app folder is not in this site's home"})
			continue
		}
		moved, already, err := siteUnitMovedBody(body, s.SiteID)
		if err != nil {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: err.Error()})
			continue
		}
		out, err := appUnitSystemctl(systemctlArgs("show", u, siteMoveShowProperties)...)
		if err != nil {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveKept, Detail: "systemd could not be read: " + strings.TrimSpace(string(out))})
			continue
		}
		props := parseShow(string(out))
		// An owner's drop-in that sets Slice= decides the unit's place: it is
		// read before anything restarts, and the unit is left to the owner.
		if owner := siteUnitDropInSlice(props["DropInPaths"]); owner != "" {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveOwnerSlice,
				Detail: "a drop-in of the server owner places it in " + owner + "; it is the owner's choice and was not restarted"})
			continue
		}
		running := siteUnitRunningStates[props["ActiveState"]]
		if already && (!running || sitesandbox.UnitInSiteSlice(props["ControlGroup"], u, s.SiteID)) {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: u, Outcome: transport.SiteMoveAlready})
			continue
		}
		m := siteMoveUnit{unit: u, file: file, mode: mode, before: text, after: siteUnitSeal(u, moved), running: running,
			wait: siteSliceStartWait(props["TimeoutStartUSec"])}
		if running {
			m.ports, _ = siteSliceListening(u)
			if int(m.wait/time.Second) > resp.WaitSeconds {
				resp.WaitSeconds = int(m.wait / time.Second)
			}
		}
		plan = append(plan, m)
	}

	// The journal, before the first write: the units' pre-move bytes and the
	// slice file's (or that there is none) with its ledger digest.
	ledger, err := loadSandboxLedger()
	if err != nil {
		refuse("limits_failed", err.Error()+"; nothing was moved")
		return nil
	}
	sliceFile, _ := siteSliceFile(s.SiteID)
	journal := &siteMoveJournal{Version: 2, SiteID: s.SiteID, Slice: sitesandbox.SliceName(s.SiteID), RequestID: req.RequestID,
		StartedAt: time.Now().UTC().Format(time.RFC3339), SliceFile: sliceFile, SliceLedger: ledger[sliceFile].RenderSHA256}
	if info, err := os.Lstat(sliceFile); err == nil {
		journal.SliceExisted = true
		if raw, err := os.ReadFile(sliceFile); err == nil && info.Mode().IsRegular() {
			journal.SliceBefore, journal.SliceMode = string(raw), uint32(info.Mode().Perm())
		}
	}
	for _, m := range plan {
		journal.Units = append(journal.Units, siteMoveJournalUnit{Unit: m.unit, File: m.file, Mode: uint32(m.mode), Before: m.before,
			After: m.after, Running: m.running, Step: siteMoveStepPlanned, WaitSeconds: int(m.wait / time.Second), Ports: siteSlicePortList(m.ports)})
	}
	if err := journal.save(); err != nil {
		refuse("record_failed", "the move's record could not be written in the Agent's folder ("+err.Error()+"); nothing was moved")
		return nil
	}
	if siteMoveKilled("", "journal") {
		return nil
	}

	siteSliceMu.Lock()
	result, written, err := writeSiteSlice(s, transport.SiteFileTriggerCreate, ledger)
	if siteMoveKilled("", "slice") {
		siteSliceMu.Unlock()
		return nil
	}
	recordErr := error(nil)
	if err == nil && written {
		// The bytes the move wrote, recorded before anything else changes:
		// a recovery restores the original only over exactly these.
		if raw, readErr := os.ReadFile(sliceFile); readErr == nil {
			journal.SliceAfter = string(raw)
			recordErr = journal.save()
		}
		if recordErr == nil {
			if err = siteSliceDaemonReload(); err == nil {
				// A push that fails is logged: each program restarted in the
				// slice below makes systemd apply the file's values, and the
				// card reads the kernel's values (review N3).
				logSiteSlicePushFailures(pushSiteSlicesToKernel([]int{s.SiteID}))
			}
		}
	}
	siteSliceMu.Unlock()
	resp.SliceFile = result.Outcome
	if recordErr != nil {
		// Nothing else is written; the slice file stays (no unit uses it, and
		// it is CelikPanel's text with the site's limits), and the record that
		// is on disk keeps the original for a later recovery.
		refuse("record_failed", "the move's record could not be written after the site's limits file ("+recordErr.Error()+"); no program was moved")
		return nil
	}
	if err != nil || result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused {
		refuse("limits_failed", fmt.Sprintf("the site's limits could not be written (%s %s): nothing was moved", result.Outcome, strings.TrimSpace(result.Reason+" "+result.Detail)))
		if err != nil {
			resp.Error = "the site's limits could not be written: " + err.Error() + "; nothing was moved"
		}
		_ = os.Remove(siteMoveJournalPath)
		return nil
	}
	if siteMoveKilled("", "slice_recorded") {
		return nil
	}

	var done []siteMoveUnit
	for i, m := range plan {
		// A program whose restart, its own put-back and the put-back of every
		// program moved before it no longer fit the move's time is not
		// started: the rest are left as they are, for the next move.
		if m.running && !siteMoveFits(time.Until(deadline), m, done) {
			var moved []string
			for _, d := range done {
				moved = append(moved, d.unit)
			}
			for _, rest := range plan[i:] {
				resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: rest.unit, Outcome: transport.SiteMoveNotTried,
					Detail: "the move's time ran out before this program; the next move continues with it"})
			}
			resp.TimeUp, resp.Reason = true, &transport.SiteMoveReason{Code: "time_up"}
			resp.Error = "the move's time ran out: " + strconv.Itoa(len(done)) + " program(s) moved (" + strings.Join(moved, ", ") +
				") stay inside the limits; press the move again to continue with the rest"
			if len(done) == 0 {
				resp.Error = "the move's time ran out before any program was moved (settling the earlier move took it); press the move again to continue"
			}
			_ = os.Remove(siteMoveJournalPath)
			return nil
		}
		outcome, step, err, killed := moveOneSiteUnit(m, s.SiteID, journal)
		if killed {
			return nil
		}
		if err == nil {
			if outcome != transport.SiteMoveChanged {
				done = append(done, m)
			}
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: m.unit, Outcome: outcome})
			continue
		}
		// Put this unit back, then every unit of the site moved before it.
		// "Put back" is said only of what was confirmed back (review W1).
		left := map[string]string{} // units whose put-back did not complete -> recovery outcome
		failed := transport.SiteSliceMoved{Unit: m.unit, Outcome: transport.SiteMoveFailed, Step: step, Detail: err.Error(), Reason: siteMoveReasonOf(step, err), Ran: m.running}
		switch back, backReason, backErr := putBackSiteUnit(m, journal); {
		case backErr != nil:
			left[m.unit] = back
			failed.Back = backReason
			failed.Detail += "; putting it back did not complete: " + backErr.Error()
		case back == transport.SiteMoveChanged:
			failed.Back = &transport.SiteMoveReason{Code: transport.SiteMoveChanged}
			failed.Detail += "; its file was changed meanwhile and is left as it is"
		default:
			failed.PutBack = true
		}
		resp.Units = append(resp.Units, failed)
		for j := len(done) - 1; j >= 0; j-- {
			back := transport.SiteSliceMoved{Unit: done[j].unit, Outcome: transport.SiteMovePutBack, PutBack: true, Ran: done[j].running}
			switch outcome, backReason, backErr := putBackSiteUnit(done[j], journal); {
			case backErr != nil:
				left[done[j].unit] = outcome
				back.PutBack, back.Back, back.Detail = false, backReason, "putting it back did not complete: "+backErr.Error()
			case outcome == transport.SiteMoveChanged:
				back.Outcome, back.PutBack, back.Detail = transport.SiteMoveChanged, false, "its file was changed meanwhile and is left as it is"
			}
			for k := range resp.Units {
				if resp.Units[k].Unit == back.Unit {
					resp.Units[k] = back
				}
			}
		}
		for _, rest := range plan[i+1:] {
			resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: rest.unit, Outcome: transport.SiteMoveNotTried})
		}
		resp.RolledBack = true
		resp.Reason = failed.Reason
		resp.Error = fmt.Sprintf("%s could not be moved into the site's limits (%s): %s", m.unit, step, err.Error())
		if len(left) == 0 {
			resp.Error += "; the site was put back as it was"
			log.Printf("site slice: the move of site %d: %s", s.SiteID, resp.Error)
			_ = os.Remove(siteMoveJournalPath)
			return nil
		}
		// The record stays (review N2): no site is moved until the put-back is
		// finished (the Agent's next start, the next move's attempt, or the
		// administrator's "Try again"); the site's card names each program
		// left and the next step.
		names := make([]string, 0, len(left))
		for unit := range left {
			names = append(names, unit)
		}
		sort.Strings(names)
		resp.Error += "; putting back did not complete for " + strings.Join(names, ", ") + ": the site's page names the next step, and no site is moved until it is settled"
		log.Printf("site slice: the move of site %d: %s", s.SiteID, resp.Error)
		resp.Kept = true
		record := transport.SiteSliceMoveRecovery{At: time.Now().UTC().Format(time.RFC3339), Units: map[string]string{}, Kept: true}
		for _, u := range resp.Units {
			record.Units[u.Unit] = u.Outcome
			if state, ok := left[u.Unit]; ok {
				record.Units[u.Unit] = state
			}
		}
		resp.Recovery = &record
		changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
			all[strconv.Itoa(s.SiteID)] = record
			return true
		})
		return nil
	}
	_ = os.Remove(siteMoveJournalPath)
	forgetSiteMoveOutcome(s.SiteID)
	resp.Done = true
	return nil
}

// siteMoveFits is the move's time rule (review B8): a running program is
// restarted only while the time left holds its own start wait, its own
// put-back (a restart that may take its start wait again), the put-back of
// every program moved before it, and a margin.
func siteMoveFits(left time.Duration, m siteMoveUnit, done []siteMoveUnit) bool {
	need := 2*m.wait + siteMoveMargin
	for _, d := range done {
		if d.running {
			need += d.wait
		}
	}
	return left >= need
}

// moveOneSiteUnit rewrites one unit under its lock and, if it ran, restarts
// it in the slice and waits for it to run there and listen again. A file
// that no longer holds the text the move planned from (changed or removed
// since) is not touched: outcome "changed". Every step is recorded before
// what it does (review B1); a record that cannot be written is a failed step
// ("record"), so nothing further is written and the caller puts back. It
// returns the failed step; killed reports a simulated Agent stop.
func moveOneSiteUnit(m siteMoveUnit, siteID int, journal *siteMoveJournal) (outcome, step string, err error, killed bool) {
	unlock := siteUnitLock(m.unit)
	defer unlock()
	current, _, _, readErr := siteUnitText(m.unit)
	if readErr != nil || current != m.before {
		_ = journal.step(m.unit, siteMoveStepChanged)
		return transport.SiteMoveChanged, "", nil, false
	}
	record := func(step string) error {
		if err := journal.step(m.unit, step); err != nil {
			return fmt.Errorf("the move's record could not be written (%v); nothing further was changed", err)
		}
		return nil
	}
	if err := record(siteMoveStepWriting); err != nil {
		return "", "record", err, false
	}
	if siteMoveKilled(m.unit, "writing") {
		return "", "", nil, true
	}
	if err := writeUnitBytes(m.file, m.after, m.mode); err != nil {
		return "", "write", err, false
	}
	if siteMoveKilled(m.unit, "wrote") {
		return "", "", nil, true
	}
	if err := record(siteMoveStepWritten); err != nil {
		return "", "record", err, false
	}
	if siteMoveKilled(m.unit, siteMoveStepWritten) {
		return "", "", nil, true
	}
	if err := siteSliceDaemonReload(); err != nil {
		return "", "write", err, false
	}
	if !m.running {
		return transport.SiteMoveRewritten, "", nil, false
	}
	if err := record(siteMoveStepRestarting); err != nil {
		return "", "record", err, false
	}
	if siteMoveKilled(m.unit, siteMoveStepRestarting) {
		return "", "", nil, true
	}
	// The restart and the wait for the program to run and answer share one
	// start wait, whatever the unit's own TimeoutStartSec says.
	deadline := time.Now().Add(m.wait)
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
	if out, err := siteUnitRestart(m.unit, m.wait); err != nil {
		_, reason, why := siteUnitRestartFailure(m.unit, m.wait, out, err)
		return "", "restart", &siteMoveError{reason: reason, err: why}, false
	}
	if siteMoveKilled(m.unit, "restarted") {
		return "", "", nil, true
	}
	verdict, props := waitSiteUnitRuns(m.unit, func(cgroup string) bool { return sitesandbox.UnitInSiteSlice(cgroup, m.unit, siteID) }, m.ports, false, deadline)
	if verdict == siteRunVerdictRuns {
		if err := record(siteMoveStepMoved); err != nil {
			return "", "record", err, false
		}
		if siteMoveKilled(m.unit, siteMoveStepMoved) {
			return "", "", nil, true
		}
		return transport.SiteMoveMoved, "", nil, false
	}
	seconds := int(m.wait / time.Second)
	reason := &transport.SiteMoveReason{Code: verdict, State: siteUnitStateWords(props), Seconds: seconds}
	switch verdict {
	case siteRunVerdictNotInPlace:
		return "", "cgroup", &siteMoveError{reason: reason, err: fmt.Errorf("it runs in %s, not /%s/%s", props["ControlGroup"], sitesandbox.SliceName(siteID), m.unit)}, false
	case siteRunVerdictNotListening:
		reason.Ports = siteSlicePortList(m.ports)
		return "", "answer", &siteMoveError{reason: reason, err: fmt.Errorf("%d seconds after the restart it does not listen on every port it listened on before (%s)", seconds, siteSlicePorts(m.ports))}, false
	case siteRunVerdictPortsUnread:
		reason.Ports = siteSlicePortList(m.ports)
		return "", "answer", &siteMoveError{reason: reason, err: fmt.Errorf("%d seconds after the restart whether it listens on its ports (%s) could not be read (ss)", seconds, siteSlicePorts(m.ports))}, false
	}
	return "", "restart", &siteMoveError{reason: reason, err: fmt.Errorf("it is %s %d seconds after the restart (journalctl -u %s says why)", siteUnitPlainState(props), seconds, m.unit)}, false
}

// siteMoveError is a step's failure with its reason's code (alpha.149, lab
// D7): Error() is the English text the journal and the audit record keep.
type siteMoveError struct {
	reason *transport.SiteMoveReason
	err    error
}

func (e *siteMoveError) Error() string { return e.err.Error() }

// siteMoveReasonOf is the code of a step's failure: its own, or the one its
// kind of step names ("record", "write").
func siteMoveReasonOf(step string, err error) *transport.SiteMoveReason {
	var coded *siteMoveError
	if errors.As(err, &coded) && coded.reason != nil {
		return coded.reason
	}
	switch step {
	case "record":
		return &transport.SiteMoveReason{Code: "record_failed"}
	case "write":
		return &transport.SiteMoveReason{Code: "write_failed"}
	}
	return &transport.SiteMoveReason{Code: "not_finished"}
}

func siteSlicePortList(ports map[int]bool) []int {
	list := make([]int, 0, len(ports))
	for p := range ports {
		list = append(list, p)
	}
	sort.Ints(list)
	return list
}

func siteSlicePorts(ports map[int]bool) string {
	list := make([]int, 0, len(ports))
	for p := range ports {
		list = append(list, p)
	}
	sort.Ints(list)
	words := make([]string, len(list))
	for i, p := range list {
		words[i] = strconv.Itoa(p)
	}
	return strings.Join(words, ", ")
}

// putBackSiteUnit writes the unit's pre-move text under its lock and, if it
// ran, restarts it where it ran before and waits, within its start wait, for
// it to run there and listen again (alpha.149, lab D6: a restart's exit
// status is not proof; fix round F10: on a port at all, a changed port is
// not a failure). Only the move's own text is put back over: a changed or
// removed file is left ("changed"), so a customer's change is never lost and
// a deleted site's unit is never re-created. A put-back that did not
// complete answers an error, its reason's code and the recovery outcome it
// leaves: "unknown" (the file or systemd's reading of it; it was not
// restarted), "failed" (it does not run where it ran: it did not start
// again, or failed right after), "starting", "not_answering" (it had ports
// and listens on none) or "unverified" (restarted; where it runs could not
// be told). Nothing else is changed by it.
func putBackSiteUnit(m siteMoveUnit, journal *siteMoveJournal) (string, *transport.SiteMoveReason, error) {
	unlock := siteUnitLock(m.unit)
	defer unlock()
	current, _, _, err := siteUnitText(m.unit)
	if err != nil || (current != m.after && current != m.before) {
		if journal != nil {
			_ = journal.step(m.unit, siteMoveStepChanged)
		}
		return transport.SiteMoveChanged, nil, nil
	}
	wrote := current == m.after
	if wrote {
		if err := writeUnitBytes(m.file, m.before, m.mode); err != nil {
			return transport.SiteMoveUnknown, &transport.SiteMoveReason{Code: "write_failed"}, err
		}
		if err := siteSliceDaemonReload(); err != nil {
			return transport.SiteMoveUnknown, &transport.SiteMoveReason{Code: "write_failed"}, err
		}
	}
	// A unit whose file the move never changed was never restarted either.
	if m.running && wrote {
		deadline := time.Now().Add(m.wait)
		_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
		if out, err := siteUnitRestart(m.unit, m.wait); err != nil {
			return siteUnitRestartFailure(m.unit, m.wait, out, err)
		}
		verdict, props := waitSiteUnitRuns(m.unit, func(cgroup string) bool { return sitesandbox.UnitInSystemSlice(cgroup, m.unit) }, m.ports, true, deadline)
		if verdict != siteRunVerdictRuns {
			state, reason, why := siteUnitPutBackFailure(m.unit, verdict, props, m.wait, m.ports)
			log.Printf("site slice: %s was put back with its earlier file but %v", m.unit, why)
			return state, reason, why
		}
	}
	if journal != nil {
		_ = journal.step(m.unit, siteMoveStepPutBack)
	}
	return transport.SiteMovePutBack, nil, nil
}

// siteUnitPutBackFailure turns a put-back's verdict into the recovery
// outcome kept for the card, its reason's code and the journal's words.
func siteUnitPutBackFailure(unit, verdict string, props map[string]string, wait time.Duration, ports map[int]bool) (string, *transport.SiteMoveReason, error) {
	seconds := int(wait / time.Second)
	reason := &transport.SiteMoveReason{Code: verdict, State: siteUnitStateWords(props), Seconds: seconds}
	switch verdict {
	case siteRunVerdictStarting:
		return transport.SiteMoveStarting, reason, fmt.Errorf("it was still starting %d seconds after the restart (journalctl -u %s says why)", seconds, unit)
	case siteRunVerdictNoPort, siteRunVerdictNotListening:
		reason.Ports = siteSlicePortList(ports)
		return transport.SiteMoveNotAnswering, reason, fmt.Errorf("%d seconds after the restart it runs where it ran but listens on no port (before: %s)", seconds, siteSlicePorts(ports))
	case siteRunVerdictNotInPlace, siteRunVerdictUnread:
		// It was restarted (review W2): "unverified", never "not restarted".
		return transport.SiteMoveUnverified, reason, fmt.Errorf("it was restarted, but where it runs %d seconds later could not be told (%s)", seconds, props["ControlGroup"])
	case siteRunVerdictPortsUnread:
		// Second fix round (R8): a listener table that cannot be read is not
		// known yet, never "listens on no port".
		reason.Ports = siteSlicePortList(ports)
		return transport.SiteMoveUnverified, reason, fmt.Errorf("it runs where it ran %d seconds after the restart, but whether it listens on a port could not be read (ss)", seconds)
	}
	return transport.SiteMoveFailed, reason, fmt.Errorf("it is not running: %s %d seconds after the restart (journalctl -u %s says why)", siteUnitPlainState(props), seconds, unit)
}

// --- At the Agent's start ------------------------------------------------------

// recoverSiteSlicesAtStartup resolves what an Agent stop left in the middle:
// an interrupted move (by the table above) and the runtime settings a limit
// change pushes for a moment (site_slices_runtime.go). Before the RPC service
// starts, so nothing else changes these units meanwhile. alpha.149 fix round
// (F1): it never starts, restarts or waits for a site's program; its time is
// at most four systemd calls under one bound: its reading, one daemon-reload,
// reset-failed and one restart request it does not wait for (second fix
// round R5; recoverSiteSliceMoveAtStart).
func recoverSiteSlicesAtStartup() {
	cleanupSiteSliceRuntime()
	recoverSiteSliceMoveAtStart()
}

// siteSliceNamedByUnitFile: a unit file names the site's slice.
func siteSliceNamedByUnitFile(siteID int) bool {
	dir, err := sandboxUnitDir()
	if err != nil {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		// A site's program, or a scheduled task of the next release naming
		// its own site's slice (siteSliceNamedBy).
		if siteSliceNamedBy(dir, e.Name()) == siteID {
			return true
		}
	}
	return false
}

// siteUnitRunsInSystemSlice: the unit runs, in the system slice (where every
// running unit was before a move). known is false when systemd could not be
// read (review N2/N4): recovery then restarts nothing and keeps its record,
// never treating an unknown place as "not running".
func siteUnitRunsInSystemSlice(unit string) (runs, known bool) {
	out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,ControlGroup")...)
	if err != nil {
		return false, false
	}
	props := parseShow(string(out))
	if props["ActiveState"] == "" {
		return false, false
	}
	return props["ActiveState"] == "active" && sitesandbox.UnitInSystemSlice(props["ControlGroup"], unit), true
}

// siteMoveRecovered is what one recovery did: the site whose record it was
// and the outcome kept for its card.
type siteMoveRecovered struct {
	siteID int
	record transport.SiteSliceMoveRecovery
	// runsNoPort (second fix round, R8): put back by the administrator's Try
	// again, running where they ran but listening on no port.
	runsNoPort []string
}

// recoverKeptSiteSliceMove is the administrator's "Try again" (review N2):
// the kept record's put-back runs now, under the move's own claim and the
// record's site lock; nothing new is moved. Done: the record is gone. It is
// refused while the panel's update is in progress (alpha.149 fix round, F6)
// and bounded by the move's own budget (F4).
func recoverKeptSiteSliceMove(resp *transport.MoveSiteSliceResponse) {
	refusal, release := claimSiteMove()
	if refusal != "" {
		resp.Error, resp.Reason = siteMoveRefusalText(refusal), &transport.SiteMoveReason{Code: refusal}
		return
	}
	defer release()
	deadline := time.Now().Add(siteMoveBudget)
	journal, err := readSiteMoveJournal()
	if errors.Is(err, os.ErrNotExist) {
		resp.Done = true // nothing is kept (already settled)
		return
	}
	if err == nil {
		siteMu := siteMutationMutex(journal.SiteID)
		if !siteMu.TryLock() {
			resp.Error, resp.Reason = "the site is being changed right now; nothing was changed. Read its page again in a minute", &transport.SiteMoveReason{Code: "site_busy"}
			return
		}
		defer siteMu.Unlock()
	}
	recovered := recoverSiteSliceMove(deadline, true)
	if recovered != nil {
		record := recovered.record
		resp.Recovery = &record
		resp.RunsNoPort = recovered.runsNoPort
	}
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		resp.Kept = true
		resp.Error = "putting back did not complete yet; the site's page names each program left and the next step"
		return
	}
	resp.Done = true
}

// loadSiteMoveJournalForRecovery reads the move's record for a recovery
// (nil: none). One that cannot be read is set aside for the administrator.
func loadSiteMoveJournalForRecovery() *siteMoveJournal {
	journal, err := readSiteMoveJournal()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Printf("site slice: the record of an interrupted move could not be read (%v); it is kept as %s.unreadable for the administrator", err, siteMoveJournalPath)
		_ = os.Rename(siteMoveJournalPath, siteMoveJournalPath+".unreadable")
		return nil
	}
	return journal
}

// siteMoveFreshOutcomes is what this move's own outcome record said of each
// program: one written when the move began or later (an older one belongs to
// an earlier move of the site).
func siteMoveFreshOutcomes(journal *siteMoveJournal) map[string]string {
	record, ok := readSiteMoveOutcomes()[strconv.Itoa(journal.SiteID)]
	if !ok || record.At == "" || record.At < journal.StartedAt {
		return nil
	}
	return record.Units
}

// restoreSiteMoveSlice puts the slice file's recorded original back (its
// earlier bytes and ledger digest, or no file where there was none) when no
// unit file names the slice now and the file still holds exactly what the
// move wrote; otherwise it leaves it. restored: a daemon-reload is due;
// failed: it could not be put back (the record stays). The caller holds the
// slice lock, siteSliceMu (second fix round, R6: a limits change or a unit
// written into the slice is ordered against it), taken after the move
// record's lock where it holds that one, as the move takes them.
func restoreSiteMoveSlice(journal *siteMoveJournal) (restored, failed bool) {
	if journal.SliceFile == "" || journal.SliceAfter == "" || siteSliceNamedByUnitFile(journal.SiteID) ||
		readSmallFileWhole(journal.SliceFile) != journal.SliceAfter {
		return false, false
	}
	var restoreErr error
	if journal.SliceExisted && journal.SliceBefore != "" {
		restoreErr = writeUnitBytes(journal.SliceFile, journal.SliceBefore, os.FileMode(journal.SliceMode))
	} else if !journal.SliceExisted {
		restoreErr = os.Remove(journal.SliceFile)
	}
	if restoreErr != nil {
		log.Printf("site slice: %s could not be put back after an interrupted move: %v", journal.SliceFile, restoreErr)
		return false, true
	}
	if err := updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
		if journal.SliceLedger == "" {
			delete(l, journal.SliceFile)
		} else {
			l[journal.SliceFile] = sandboxLedgerEntry{Kind: "site_slice", RenderSHA256: journal.SliceLedger, WrittenAt: time.Now().UTC().Format(time.RFC3339)}
		}
		return true
	}); err != nil {
		log.Printf("site slice: %v", err)
	}
	return true, false
}

// recordSiteMoveRecovery keeps what a recovery did for the site's card and
// removes the record when everything is settled.
func recordSiteMoveRecovery(journal *siteMoveJournal, outcomes map[string]string, complete bool) *siteMoveRecovered {
	record := transport.SiteSliceMoveRecovery{At: time.Now().UTC().Format(time.RFC3339), Units: outcomes, Kept: !complete}
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		all[strconv.Itoa(journal.SiteID)] = record
		return true
	})
	if !complete {
		log.Printf("site slice: the interrupted move of site %d is not finished (%v); its record stays (%s) and no site is moved until it is settled (a status read, the administrator's Try again or the next move's attempt of that site)", journal.SiteID, outcomes, siteMoveJournalPath)
		return &siteMoveRecovered{siteID: journal.SiteID, record: record}
	}
	log.Printf("site slice: the interrupted move of site %d was resolved: %v", journal.SiteID, outcomes)
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site slice: remove the move's record: %v", err)
	}
	return &siteMoveRecovered{siteID: journal.SiteID, record: record}
}

// recoverSiteSliceMoveAtStart is the Agent's start's part (alpha.149 fix
// round, F1; the table at the top): the record is read and reconciled with
// the files, and nothing waits for a site's program. Its calls to systemd
// are at most four, under one total bound (second fix round, R5,
// siteMoveStartBound): one reading of the programs whose place decides what
// happens to them, ONE daemon-reload for every file it wrote back (and the
// slice file), one reset-failed, and ONE restart request with --no-block
// naming every program that ran when the move began and does not run in the
// system slice now (systemd runs the jobs; the start does not wait for
// them). No polling, no reading of ports, no wait. Whatever that request's
// exit code, each program it named is "unverified" (not known yet) and the
// record stays: the status read checks it later by reading only
// (siteMoveOutcomesNow); Try again or the next move's attempt finish it.
// When the bound runs out nothing further is requested: a reading or reload
// that did not answer leaves "unknown", a request not sent "waiting". A
// program whose earlier file was already back is left exactly as it is.
func recoverSiteSliceMoveAtStart() *siteMoveRecovered {
	journal := loadSiteMoveJournalForRecovery()
	if journal == nil {
		return nil
	}
	if journal.Tasks != nil {
		return recoverSiteTasksRecord(journal, time.Now().Add(siteMoveStartBound), true)
	}
	earlier := siteMoveFreshOutcomes(journal)
	outcomes := map[string]string{}
	var wrote, look []siteMoveJournalUnit
	for _, u := range journal.Units {
		unlock := siteUnitLock(u.Unit)
		current, _, _, readErr := siteUnitText(u.Unit)
		untouched := u.Step == siteMoveStepPlanned || u.Step == siteMoveStepWriting
		switch {
		case readErr != nil || (current != u.Before && current != u.After):
			// Someone changed or removed it: left as it is.
			outcomes[u.Unit] = transport.SiteMoveChanged
		case current == u.Before && untouched:
			outcomes[u.Unit] = transport.SiteMoveNotTried
		case current == u.Before:
			// Its earlier file is already back: left exactly as it is,
			// running or not, with what this move's record said of it.
			switch state := earlier[u.Unit]; {
			case u.Step == siteMoveStepPutBack || !u.Running:
				outcomes[u.Unit] = transport.SiteMovePutBack
			case state != "":
				outcomes[u.Unit] = state
			default:
				look = append(look, u) // its put-back was cut short: read once
			}
		case !u.Running:
			outcomes[u.Unit] = transport.SiteMoveRewritten
		case u.Step == siteMoveStepMoved:
			outcomes[u.Unit] = transport.SiteMoveMoved
		default:
			// The move's text, not known to have run and answered in the
			// slice: the pre-move bytes go back.
			if err := writeUnitBytes(u.File, u.Before, os.FileMode(u.Mode)); err != nil {
				log.Printf("site slice: %s could not be put back after an interrupted move: %v", u.Unit, err)
				outcomes[u.Unit] = transport.SiteMoveUnknown
				break
			}
			wrote = append(wrote, u)
			look = append(look, u)
		}
		unlock()
	}
	// Second fix round (R5): every systemd call below shares ONE total bound
	// (siteMoveStartBound): the reading, the reload, reset-failed and the one
	// restart request, so at most four calls whatever the number of programs.
	// When it runs out nothing further is requested and the record stays.
	ctx, cancel := context.WithTimeout(context.Background(), siteMoveStartBound)
	defer cancel()
	systemctl := func(args ...string) ([]byte, error) { return siteMoveCall(ctx, args...) }
	// One reading of every program whose place decides what happens to it.
	var places map[string]map[string]string
	if len(look) > 0 {
		names := make([]string, 0, len(look))
		for _, u := range look {
			names = append(names, u.Unit)
		}
		read, err := showUnitsWith(systemctl, names)
		if err != nil {
			log.Printf("site slice: where the interrupted move's programs run could not be read (%v); none is restarted", err)
		}
		places = read
	}
	written := map[string]bool{}
	for _, u := range wrote {
		written[u.Unit] = true
	}
	var restart []siteMoveJournalUnit
	for _, u := range look {
		props := places[u.Unit]
		inPlace := props["ActiveState"] == "active" && sitesandbox.UnitInSystemSlice(props["ControlGroup"], u.Unit)
		switch {
		case props["ActiveState"] == "":
			// Nothing is restarted on an unknown reading (review N2/N4).
			outcomes[u.Unit] = transport.SiteMoveUnknown
		case inPlace && written[u.Unit]:
			// The move's restart never took it out of the system slice: the
			// program that ran before the move still runs there.
			outcomes[u.Unit] = transport.SiteMovePutBack
		case inPlace && len(u.Ports) == 0 && siteUnitRunsSettled(u, props):
			// A put-back cut short: closed only for a program that really runs
			// (R1: running, up for its start wait; the start reads no ports, so
			// one that had ports is left to the status read).
			outcomes[u.Unit] = transport.SiteMovePutBack
		case inPlace:
			outcomes[u.Unit] = transport.SiteMoveUnverified // the status read sees whether it keeps running
		case written[u.Unit]:
			restart = append(restart, u) // it ran when the move began
		default:
			// Put back, it ran before and does not run where it ran: not
			// restarted at the start; Try again restarts it.
			outcomes[u.Unit] = transport.SiteMoveWaiting
		}
	}
	complete := len(restart) == 0
	for _, state := range outcomes {
		if siteMoveOutcomeKept(state) {
			complete = false
		}
	}
	reload := len(wrote) > 0
	if complete {
		siteSliceMu.Lock()
		restored, failed := restoreSiteMoveSlice(journal)
		siteSliceMu.Unlock()
		reload = reload || restored
		complete = !failed
	}
	if reload {
		if err := siteSliceDaemonReloadWith(systemctl); err != nil {
			// systemd may still hold the moved text: a restart would start it
			// in the slice, so none is requested.
			log.Printf("site slice: the interrupted move's put-back is not finished: %v", err)
			for _, u := range wrote {
				outcomes[u.Unit] = transport.SiteMoveUnknown
			}
			restart, complete = nil, false
		}
	}
	if len(restart) > 0 {
		names := make([]string, 0, len(restart))
		for _, u := range restart {
			names = append(names, u.Unit)
		}
		_, _ = systemctl(systemctlArgs(append([]string{"reset-failed"}, names...)...)...)
		if ctx.Err() != nil {
			// The bound ran out: no restart is requested; Try again restarts them.
			log.Printf("site slice: %s were put back; their restart was not requested at the Agent's start (its time for systemd ran out); Try again restarts them", strings.Join(names, ", "))
			for _, u := range restart {
				outcomes[u.Unit] = transport.SiteMoveWaiting
			}
		} else {
			// ONE request for all (R5): systemd queues a job per unit and goes on
			// after one it cannot queue. Whatever its exit code, each program is
			// "not known yet"; the status read sorts them out (R1).
			out, err := systemctl(systemctlArgs(append([]string{"restart", "--no-block"}, names...)...)...)
			if err != nil {
				log.Printf("site slice: %s were put back; their restart request answered %v: %s (whether each runs where it ran is read later)", strings.Join(names, ", "), err, strings.TrimSpace(string(out)))
			} else {
				log.Printf("site slice: %s were put back and their restart requested; whether each runs where it ran is read later (the Agent's start does not wait for it)", strings.Join(names, ", "))
			}
			for _, u := range restart {
				outcomes[u.Unit] = transport.SiteMoveUnverified
			}
		}
	}
	return recordSiteMoveRecovery(journal, outcomes, complete)
}

// siteUnitPutBackRuns is the put-back's check of a program that was not
// restarted (alpha.149 fix round, F10, the same check as after a restart):
// it runs where it ran and, if it had ports, listens on a port. known is
// false when systemd could not be read. A listener table that cannot be read
// is "unverified" (second fix round, R8: not known yet).
func siteUnitPutBackRuns(u siteMoveJournalUnit) (state string, known bool) {
	runs, known := siteUnitRunsInSystemSlice(u.Unit)
	if !known {
		return transport.SiteMoveUnknown, false
	}
	if !runs {
		return "", true
	}
	switch listens, portsKnown := siteUnitListensAgain(u.Unit, siteMoveJournalPorts(u), true); {
	case !portsKnown:
		return transport.SiteMoveUnverified, true
	case !listens:
		return transport.SiteMoveNotAnswering, true
	}
	return transport.SiteMovePutBack, true
}

// recoverSiteSliceMove finishes or puts back an interrupted move from its
// journal for the administrator's Try again and a move's attempt on the
// record's own site, records what it did for the site's card and removes
// the journal when everything is settled. The step names are a hint only:
// every unit's file is compared with the recorded pre-move bytes and the
// move's text (the table at the top).
//
// Third fix round (review N2): a put-back that does not complete keeps the
// record, so no site is moved until it is settled, and says so for the card
// (Kept). That is a pre-move file that could not be written ("unknown"),
// a unit whose place systemd could not tell ("unknown": nothing is
// restarted on an unknown reading), a daemon-reload that failed (every unit
// whose file came back is "unknown": systemd still holds the moved text, so
// a restart would start it in the slice; none is restarted), a restart that
// failed ("failed": the program is stopped) or one still starting at its
// start wait ("starting"). The slice file is put back only once every unit
// is settled. It returns what it did (nil: no record, or one that could not
// be read).
//
// alpha.149 fix round: a program that ran before and does not run where it
// ran is restarted and checked within its start wait, the whole recovery
// within the move's own budget (F4: a restart is sent only while what is
// left holds its start wait and the margin; the rest are "waiting"); a
// program that runs where it ran is never restarted and passes the same
// check as a restarted one (F10: a port at all where it had ports).
//
// Second fix round: deadline is the press's one deadline (R4: a move's
// attempt shares it with the move that follows). tryAgain (R8): the
// administrator's Try again restarts a program kept "not answering" that
// still runs where it ran and listens on no port once more; if it then runs
// there without a break for its start wait it is put back even though it
// listens on no port, and is named in runsNoPort. The slice file is put back
// under the slice lock (R6), after the record's lock the caller holds.
func recoverSiteSliceMove(deadline time.Time, tryAgain bool) *siteMoveRecovered {
	journal := loadSiteMoveJournalForRecovery()
	if journal == nil {
		return nil
	}
	if journal.Tasks != nil {
		return recoverSiteTasksRecord(journal, deadline, false)
	}
	earlier := siteMoveFreshOutcomes(journal)
	outcomes := map[string]string{}
	reload := false
	var restart, wrote []siteMoveJournalUnit
	var runsNoPort []string
	again := map[string]bool{}
	complete := true
	unknown := func(u siteMoveJournalUnit) {
		outcomes[u.Unit] = transport.SiteMoveUnknown
		complete = false
	}
	// placed settles a program whose earlier file is back: it runs where it
	// ran (checked), it does not (restarted below), or it is not known.
	placed := func(u siteMoveJournalUnit) {
		switch state, known := siteUnitPutBackRuns(u); {
		case !known:
			unknown(u)
		case state == "":
			restart = append(restart, u)
		case state == transport.SiteMoveNotAnswering && tryAgain && earlier[u.Unit] == transport.SiteMoveNotAnswering:
			// R8: the administrator asked again: one more restart.
			again[u.Unit] = true
			restart = append(restart, u)
		default:
			outcomes[u.Unit] = state
			complete = complete && !siteMoveOutcomeKept(state)
		}
	}
	for _, u := range journal.Units {
		unlock := siteUnitLock(u.Unit)
		current, _, _, readErr := siteUnitText(u.Unit)
		untouched := u.Step == siteMoveStepPlanned || u.Step == siteMoveStepWriting
		switch {
		case readErr != nil || (current != u.Before && current != u.After):
			// Someone changed or removed it: left as it is.
			outcomes[u.Unit] = transport.SiteMoveChanged
		case current == u.Before && untouched:
			outcomes[u.Unit] = transport.SiteMoveNotTried
		case current == u.Before:
			// Its file was already put back; a program that ran before and
			// does not run in the system slice now (a restart in the slice,
			// or a failed one) starts again where it ran, after a reload (an
			// earlier reload may have failed).
			outcomes[u.Unit] = transport.SiteMovePutBack
			if u.Running {
				before := len(restart)
				placed(u)
				reload = reload || (len(restart) > before && !again[u.Unit])
			}
		case !u.Running:
			outcomes[u.Unit] = transport.SiteMoveRewritten
		case u.Step == siteMoveStepMoved:
			outcomes[u.Unit] = transport.SiteMoveMoved
		default:
			// The move's text, not known to have run and answered in the
			// slice: the pre-move bytes go back.
			if err := writeUnitBytes(u.File, u.Before, os.FileMode(u.Mode)); err != nil {
				log.Printf("site slice: %s could not be put back after an interrupted move: %v", u.Unit, err)
				unknown(u)
				break
			}
			reload = true
			wrote = append(wrote, u)
			outcomes[u.Unit] = transport.SiteMovePutBack
			placed(u)
		}
		unlock()
	}
	if reload {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: the interrupted move's put-back is not finished: %v", err)
			for _, u := range append(wrote, restart...) {
				unknown(u)
			}
			restart = nil
		}
	}
	for _, u := range restart {
		wait := siteMoveJournalWait(u)
		if time.Until(deadline) < wait+siteMoveMargin {
			log.Printf("site slice: %s was put back and is not restarted now: the move's time ran out; Try again restarts it", u.Unit)
			outcomes[u.Unit] = transport.SiteMoveWaiting
			if again[u.Unit] {
				outcomes[u.Unit] = transport.SiteMoveNotAnswering // it runs; not restarted once more
			}
			complete = false
			continue
		}
		unlock := siteUnitLock(u.Unit)
		started := time.Now()
		_, _ = appUnitSystemctl(systemctlArgs("reset-failed", u.Unit)...)
		if out, err := siteUnitRestart(u.Unit, wait); err != nil {
			state, _, why := siteUnitRestartFailure(u.Unit, wait, out, err)
			log.Printf("site slice: %s was put back but did not start again: %v", u.Unit, why)
			outcomes[u.Unit] = state
			complete = false
			unlock()
			continue
		}
		// alpha.149 (lab D6): checked as the put-back checks, within the same
		// wait: a program that does not run where it ran is not "put back".
		var verdict string
		var props map[string]string
		if again[u.Unit] {
			verdict, props = waitSiteUnitRunsWithoutPort(u.Unit, siteMoveJournalPorts(u), started.Add(wait))
		} else {
			verdict, props = waitSiteUnitRuns(u.Unit, func(cgroup string) bool { return sitesandbox.UnitInSystemSlice(cgroup, u.Unit) },
				siteMoveJournalPorts(u), true, started.Add(wait))
		}
		switch verdict {
		case siteRunVerdictRuns:
			outcomes[u.Unit] = transport.SiteMovePutBack
		case siteRunVerdictRunsNoPort:
			// R8: said twice now (the move's put-back, and this answer).
			log.Printf("site slice: %s was restarted once more by the administrator's Try again; it runs where it ran but listens on no port (before the move: %s); it is put back", u.Unit, siteSlicePorts(siteMoveJournalPorts(u)))
			outcomes[u.Unit] = transport.SiteMovePutBack
			runsNoPort = append(runsNoPort, u.Unit)
		default:
			state, _, why := siteUnitPutBackFailure(u.Unit, verdict, props, wait, siteMoveJournalPorts(u))
			log.Printf("site slice: %s was put back with its earlier file but %v", u.Unit, why)
			outcomes[u.Unit] = state
			complete = false
		}
		unlock()
	}
	if complete {
		// R6: the slice lock after the record's lock, as the move takes them.
		siteSliceMu.Lock()
		restored, failed := restoreSiteMoveSlice(journal)
		if restored {
			_ = siteSliceDaemonReload()
		}
		siteSliceMu.Unlock()
		complete = !failed
	}
	recovered := recordSiteMoveRecovery(journal, outcomes, complete)
	sort.Strings(runsNoPort)
	recovered.runsNoPort = runsNoPort
	return recovered
}

// readSmallFileWhole reads a unit-sized file whole ("" when it cannot be
// read or is larger than a unit file can be).
func readSmallFileWhole(p string) string {
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return ""
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(raw)
}
