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
// At the Agent's next start (recoverSiteSliceMove) the step name alone is
// never trusted: for every unit the file on disk is compared with the
// recorded pre-move bytes and with the text the move intended, and the Agent
// acts on what it finds:
//
//	pre-move bytes, step planned or writing   -> not touched ("not_tried")
//	pre-move bytes, a later step               -> already put back; restarted when it ran before and
//	                                              is not running in the system slice now ("put_back";
//	                                              its place not known: not restarted, "unknown", kept)
//	the move's text, stopped unit              -> kept ("rewritten"; it starts in the slice)
//	the move's text, running unit, step moved  -> kept ("moved")
//	the move's text, running unit, other step  -> pre-move bytes back, daemon-reload, and a restart
//	                                              unless it still runs in the system slice ("put_back")
//	anything else, or no file                  -> left as it is ("changed")
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

// siteMoveOutcomesNow is the outcome record as the status read reports it
// (review N2): the site whose move record is on disk is "kept" (also when
// its outcome could not be written), and a kept outcome whose record the
// server owner removed is no longer kept. While a move runs its record is a
// live one, so the outcomes are reported as written.
func siteMoveOutcomesNow() map[string]transport.SiteSliceMoveRecovery {
	all := readSiteMoveOutcomes()
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
	siteMoveBudget = 11 * time.Minute
	siteMoveMargin = 5 * time.Second
	siteSlicePoll  = 500 * time.Millisecond

	// siteMoveStartBudget bounds what a kept or interrupted move's recovery
	// does at the Agent's start (alpha.148 fix round, review finding 5): it
	// runs before the Agent's socket opens, and the update waits about 12
	// seconds for that socket after starting the Agent (update.sh
	// wait_for_fresh_active_agent: 40 x 0.3 s; finalize-pending-update.sh 50
	// x 0.2 s), so the whole recovery, its reads, reload and restarts, stays
	// within 4 seconds. A put-back restart is sent only while
	// siteMoveStartMinWait is left and waits at most what is left (a start
	// still running then is "starting": systemd keeps its job); one that
	// does not fit is not sent at this start ("waiting"). Either keeps the
	// record, its Problems now line and Try again, which has no such bound
	// (nor has the next move's attempt).
	siteMoveStartBudget  = 4 * time.Second
	siteMoveStartMinWait = time.Second

	siteSliceListening = func(unit string) map[int]bool {
		listeners, pids, err := nodeSiteListeners(unit)
		if err != nil {
			return nil
		}
		ports := map[int]bool{}
		for _, l := range listeners {
			for _, pid := range l.PIDs {
				if pids[pid] {
					ports[l.Port] = true
				}
			}
		}
		return ports
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
)

// waitSiteUnitRuns polls a restarted unit until deadline the way the move's
// own step always did, and since alpha.149 the put-back too (lab D6: a
// Type=simple unit's restart succeeds as soon as its process is executed, so
// systemctl's exit status alone never proves that the program runs): it is
// active, in the place inPlace accepts (its ControlGroup) and listens again
// on every port it listened on before. It answers the verdict and the last
// reading. A unit that does not run when the time is up is "not_running"
// (failed, stopped, or inside systemd's automatic restart cycle: SubState
// auto-restart, no process), one still in its first start "still_starting".
func waitSiteUnitRuns(unit string, inPlace func(cgroup string) bool, ports map[int]bool, deadline time.Time) (string, map[string]string) {
	for {
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", unit, "--property=ActiveState,ControlGroup,SubState")...); err == nil {
			props = parseShow(string(out))
		}
		state := props["ActiveState"]
		placed := state == "active" && inPlace(props["ControlGroup"])
		listening := true
		if placed && len(ports) > 0 {
			now := siteSliceListening(unit)
			for port := range ports {
				listening = listening && now[port]
			}
		}
		if placed && listening {
			return siteRunVerdictRuns, props
		}
		if !time.Now().Before(deadline) {
			switch {
			case state == "":
				return siteRunVerdictUnread, props
			case state == "active" && !placed:
				return siteRunVerdictNotInPlace, props
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

// siteUnitAutoRestart: systemd waits to start the unit again by itself (its
// program exited; nothing of it runs now).
func siteUnitAutoRestart(props map[string]string) bool {
	return strings.HasPrefix(props["SubState"], "auto-restart")
}

// siteUnitStateWords is what systemd showed, for the reason's State.
func siteUnitStateWords(props map[string]string) string {
	return strings.TrimSpace(props["ActiveState"] + " " + props["SubState"])
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

var siteMoveRunning sync.Mutex

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
	if !siteMoveRunning.TryLock() {
		refuse("another_move", "another move into a site's limits is running; nothing was changed")
		return nil
	}
	defer siteMoveRunning.Unlock()
	siteMu := siteMutationMutex(s.SiteID)
	if !siteMu.TryLock() {
		refuse("site_busy", "the site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute")
		return nil
	}
	defer siteMu.Unlock()
	// A journal left by an earlier Agent, or by a move whose put-back did not
	// complete, is resolved before anything new; while it stays, nothing new
	// is moved (review N2).
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		kept := recoverSiteSliceMove(0)
		if _, err := os.Stat(siteMoveJournalPath); err == nil {
			site := "a site"
			if kept != nil {
				site = "site " + strconv.Itoa(kept.siteID)
			}
			refuse("kept_earlier", "an earlier move of "+site+" could not be put back completely; its page names each program left and the next step. Nothing new was moved until that is settled")
			resp.Kept = true
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
			m.ports = siteSliceListening(u)
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

	deadline := time.Now().Add(siteMoveBudget)
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
		failed := transport.SiteSliceMoved{Unit: m.unit, Outcome: transport.SiteMoveFailed, Step: step, Detail: err.Error(), Reason: siteMoveReasonOf(step, err)}
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
			back := transport.SiteSliceMoved{Unit: done[j].unit, Outcome: transport.SiteMovePutBack, PutBack: true}
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
	verdict, props := waitSiteUnitRuns(m.unit, func(cgroup string) bool { return sitesandbox.UnitInSiteSlice(cgroup, m.unit, siteID) }, m.ports, deadline)
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
	}
	return "", "restart", &siteMoveError{reason: reason, err: fmt.Errorf("it is %s %d seconds after the restart (journalctl -u %s says why)", siteUnitStateWords(props), seconds, m.unit)}, false
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
// it to run there and listen again on its ports (alpha.149, lab D6: as the
// move's own step does; a restart's exit status is not proof). Only the
// move's own text is put back over: a changed or removed file is left
// ("changed"), so a customer's change is never lost and a deleted site's
// unit is never re-created. A put-back that did not complete answers an
// error, its reason's code and the recovery outcome it leaves: "unknown"
// (the file or systemd's reading of it), "failed" (it does not run where it
// ran: it did not start again, or failed right after), "starting" or
// "not_answering". Nothing else is changed by it.
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
		verdict, props := waitSiteUnitRuns(m.unit, func(cgroup string) bool { return sitesandbox.UnitInSystemSlice(cgroup, m.unit) }, m.ports, deadline)
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
	case siteRunVerdictNotListening:
		reason.Ports = siteSlicePortList(ports)
		return transport.SiteMoveNotAnswering, reason, fmt.Errorf("%d seconds after the restart it does not listen on every port it listened on before (%s)", seconds, siteSlicePorts(ports))
	case siteRunVerdictNotInPlace, siteRunVerdictUnread:
		return transport.SiteMoveUnknown, reason, fmt.Errorf("where it runs %d seconds after the restart is not known (%s)", seconds, props["ControlGroup"])
	}
	return transport.SiteMoveFailed, reason, fmt.Errorf("it is not running: %s %d seconds after the restart (journalctl -u %s says why)", siteUnitStateWords(props), seconds, unit)
}

// --- At the Agent's start ------------------------------------------------------

// recoverSiteSlicesAtStartup resolves what an Agent stop left in the middle:
// an interrupted move (by the table above) and the runtime settings a limit
// change pushes for a moment (site_slices_runtime.go). Before the RPC service
// starts, so nothing else changes these units meanwhile; bounded by
// siteMoveStartBudget, so a program that is slow to start never holds up the
// Agent's start (what does not fit stays in the kept record).
func recoverSiteSlicesAtStartup() {
	cleanupSiteSliceRuntime()
	recoverSiteSliceMove(siteMoveStartBudget)
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
}

// recoverKeptSiteSliceMove is the administrator's "Try again" (review N2):
// the kept record's put-back runs now, under the move's own lock and the
// record's site lock; nothing new is moved. Done: the record is gone.
func recoverKeptSiteSliceMove(resp *transport.MoveSiteSliceResponse) {
	if !siteMoveRunning.TryLock() {
		resp.Error, resp.Reason = "a move into a site's limits is running; nothing was changed", &transport.SiteMoveReason{Code: "another_move"}
		return
	}
	defer siteMoveRunning.Unlock()
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
	recovered := recoverSiteSliceMove(0)
	if recovered != nil {
		record := recovered.record
		resp.Recovery = &record
	}
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		resp.Kept = true
		resp.Error = "putting back did not complete yet; the site's page names each program left and the next step"
		return
	}
	resp.Done = true
}

// recoverSiteSliceMove finishes or puts back an interrupted move from its
// journal, records what it did for the site's card and removes the journal.
// The step names are a hint only: every unit's file is compared with the
// recorded pre-move bytes and the move's text (the table at the top).
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
// budget (alpha.148 fix round, review finding 5): above 0 (the Agent's start,
// siteMoveStartBudget) the whole recovery keeps within it: a put-back
// restart is sent only while siteMoveStartMinWait is left, and waits at most
// what is left ("starting" when its start runs on); the restarts that do not
// fit are not sent ("waiting": its pre-move file is back, it ran before and
// does not run in the system slice now). Both keep the record. 0: each
// restart waits its own start wait (the administrator's Try again, a move's
// attempt).
func recoverSiteSliceMove(budget time.Duration) *siteMoveRecovered {
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}
	journal, err := readSiteMoveJournal()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Printf("site slice: the record of an interrupted move could not be read (%v); it is kept as %s.unreadable for the administrator", err, siteMoveJournalPath)
		_ = os.Rename(siteMoveJournalPath, siteMoveJournalPath+".unreadable")
		return nil
	}
	outcomes := map[string]string{}
	reload := false
	var restart, wrote []siteMoveJournalUnit
	complete := true
	unknown := func(u siteMoveJournalUnit) {
		outcomes[u.Unit] = transport.SiteMoveUnknown
		complete = false
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
			// or a failed one) starts again where it ran.
			outcomes[u.Unit] = transport.SiteMovePutBack
			if u.Running {
				switch runs, known := siteUnitRunsInSystemSlice(u.Unit); {
				case !known:
					unknown(u)
				case !runs:
					reload = true
					restart = append(restart, u)
				}
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
			switch runs, known := siteUnitRunsInSystemSlice(u.Unit); {
			case !known:
				unknown(u)
			case !runs:
				restart = append(restart, u)
			}
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
		cut := false // the start's bound, not the unit's own wait, ends the wait
		if budget > 0 {
			left := time.Until(deadline)
			if left < siteMoveStartMinWait {
				log.Printf("site slice: %s was put back and is not restarted at this start (the Agent's start does not wait for it); the next start, a move's attempt or Try again restarts it", u.Unit)
				outcomes[u.Unit] = transport.SiteMoveWaiting
				complete = false
				continue
			}
			if left < wait {
				wait, cut = left, true
			}
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
		// alpha.149 (lab D6): verified as the move verifies, within the same
		// wait: a program that does not run where it ran is not "put back".
		// Within the start's bound, what cannot be seen yet is "unverified"
		// (not known: neither a failure nor a success).
		verdict, props := waitSiteUnitRuns(u.Unit, func(cgroup string) bool { return sitesandbox.UnitInSystemSlice(cgroup, u.Unit) },
			siteMoveJournalPorts(u), started.Add(wait))
		if verdict != siteRunVerdictRuns {
			state, _, why := siteUnitPutBackFailure(u.Unit, verdict, props, wait, siteMoveJournalPorts(u))
			if cut {
				state = transport.SiteMoveUnverified
				why = fmt.Errorf("whether it runs again was not known before the Agent's start went on (%v)", why)
			}
			log.Printf("site slice: %s was put back with its earlier file but %v", u.Unit, why)
			outcomes[u.Unit] = state
			complete = false
		}
		unlock()
	}
	// The slice file: the recorded original comes back when every unit is
	// settled, no unit file names the slice now and the file still holds
	// exactly what the move wrote; otherwise it stays.
	if complete && journal.SliceFile != "" && journal.SliceAfter != "" && !siteSliceNamedByUnitFile(journal.SiteID) &&
		readSmallFileWhole(journal.SliceFile) == journal.SliceAfter {
		restoreErr := error(nil)
		if journal.SliceExisted && journal.SliceBefore != "" {
			restoreErr = writeUnitBytes(journal.SliceFile, journal.SliceBefore, os.FileMode(journal.SliceMode))
		} else if !journal.SliceExisted {
			restoreErr = os.Remove(journal.SliceFile)
		}
		if restoreErr != nil {
			log.Printf("site slice: %s could not be put back after an interrupted move: %v", journal.SliceFile, restoreErr)
			complete = false
		} else {
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
			_ = siteSliceDaemonReload()
		}
	}
	record := transport.SiteSliceMoveRecovery{At: time.Now().UTC().Format(time.RFC3339), Units: outcomes, Kept: !complete}
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		all[strconv.Itoa(journal.SiteID)] = record
		return true
	})
	if !complete {
		log.Printf("site slice: the interrupted move of site %d is not finished (%v); its record stays (%s) and no site is moved until the next start, move or the administrator's Try again finishes it", journal.SiteID, outcomes, siteMoveJournalPath)
		return &siteMoveRecovered{siteID: journal.SiteID, record: record}
	}
	log.Printf("site slice: the interrupted move of site %d was resolved: %v", journal.SiteID, outcomes)
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site slice: remove the move's record: %v", err)
	}
	return &siteMoveRecovered{siteID: journal.SiteID, record: record}
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
