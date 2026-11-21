package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
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
// The move is safe to interrupt. Before its first write it records in the
// Agent's private folder (siteMoveJournalPath) the pre-move bytes and mode of
// every unit file it will change, whether the slice file existed (and its
// bytes), and each unit's step. At the Agent's next start
// (recoverSiteSliceMoveAtStartup) every unit is finished or put back by the
// step it reached:
//
//	planned     nothing was written                      -> not touched ("not_tried")
//	written     stopped unit: its file names the slice    -> kept ("rewritten"; it starts in the slice)
//	written     running unit: file written, not restarted -> pre-move bytes back, daemon-reload ("put_back"; it never left)
//	restarting  restart sent, outcome unknown             -> pre-move bytes back, daemon-reload, restart ("put_back")
//	moved       it ran and answered in the slice          -> kept ("moved")
//	put_back    already put back                          -> nothing more ("put_back")
//	changed     changed by someone else during the move   -> left as it is ("changed")
//
// "Put back" always writes the pre-move bytes recorded before the first
// write, and only over the move's own text: a file that no longer holds what
// the move wrote (a customer's change, a deleted site) is never overwritten
// or re-created. The slice file stays unless it did not exist before and no
// unit file names it after the recovery; then it is removed. What the Agent
// did is kept for the site's card (siteMoveOutcomesPath: reason, who acts,
// next step on the screen) until the site's next move.
//
// Every unit step takes that program's own mutation lock (the one every
// other change of the program takes), and the whole move holds the site's
// lock without waiting for it: a site being deleted or changed is not moved,
// and a customer's change of a program waits for that program's step (at
// most its start wait), never lost.
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
}

type siteMoveJournal struct {
	Version      int                   `json:"version"`
	SiteID       int                   `json:"site_id"`
	Slice        string                `json:"slice"`
	RequestID    string                `json:"request_id"`
	StartedAt    string                `json:"started_at"`
	SliceFile    string                `json:"slice_file"`
	SliceExisted bool                  `json:"slice_existed"`
	Units        []siteMoveJournalUnit `json:"units"`
	// Tasks (version 2, S4 R2b): the move of the site's crontab into timers
	// or its undo (site_tasks_move.go); a journal holds either units or tasks.
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
	return os.Rename(tmp, path)
}

func (j *siteMoveJournal) save() error {
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return writePrivateFile(siteMoveJournalPath, raw)
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
	// siteMoveBudget is the whole move's time: a program whose wait (and its
	// put-back) no longer fits is not started; the answer says to press
	// again. It stays below the Panel's 12 minute bound of the call.
	siteMoveBudget = 10 * time.Minute
	siteSlicePoll  = 500 * time.Millisecond

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
	// journal for recoverSiteSliceMove.
	siteMoveKilled = func(unit, step string) bool { return false }
)

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
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "moving a site into its limits"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		resp.Error = "this server cannot hold a site's limits (missing: " + reason + "); nothing was moved"
		return nil
	}
	s := siteSliceFor(&req.Site, req.Site.SiteID)
	home, homeErr := hostingpath.SiteHome(req.SubscriptionID, req.DomainID)
	if s == nil || homeErr != nil || len(req.Units) == 0 || len(req.Units) > siteSliceMaxUnits {
		resp.Error = "the site, its home and up to 64 of its units are required"
		return nil
	}
	for _, u := range req.Units {
		if !siteSliceUnitPattern.MatchString(u) {
			resp.Error = "not a site's unit: " + u
			return nil
		}
	}
	if !siteMoveRunning.TryLock() {
		resp.Error = "another move into a site's limits is running; nothing was changed"
		return nil
	}
	defer siteMoveRunning.Unlock()
	siteMu := siteMutationMutex(s.SiteID)
	if !siteMu.TryLock() {
		resp.Error = "the site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute"
		return nil
	}
	defer siteMu.Unlock()
	// A journal left by an earlier Agent is resolved before anything new.
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		recoverSiteSliceMove()
	}
	// The units restart with this release's helper and guard: an earlier
	// release's copy adds only system-slice cgroups to the SMTP block, so a
	// unit in its site's slice would not start with it.
	if err := siteMovePrepare(); err != nil {
		resp.Error = "the site sandbox's helper and SMTP block could not be put in place: " + err.Error() + "; nothing was moved"
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
		if already && (!running || props["ControlGroup"] == "/"+sitesandbox.SliceName(s.SiteID)+"/"+u) {
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

	// The journal, before the first write.
	sliceFile, _ := siteSliceFile(s.SiteID)
	_, sliceErr := os.Lstat(sliceFile)
	journal := &siteMoveJournal{Version: 1, SiteID: s.SiteID, Slice: sitesandbox.SliceName(s.SiteID), RequestID: req.RequestID,
		StartedAt: time.Now().UTC().Format(time.RFC3339), SliceFile: sliceFile, SliceExisted: sliceErr == nil}
	for _, m := range plan {
		journal.Units = append(journal.Units, siteMoveJournalUnit{Unit: m.unit, File: m.file, Mode: uint32(m.mode), Before: m.before,
			After: m.after, Running: m.running, Step: siteMoveStepPlanned})
	}
	if err := journal.save(); err != nil {
		resp.Error = "the move's record could not be written in the Agent's folder (" + err.Error() + "); nothing was moved"
		return nil
	}
	if siteMoveKilled("", "journal") {
		return nil
	}

	siteSliceMu.Lock()
	result, written, _, err := writeSiteSlice(s, transport.SiteFileTriggerCreate, readSandboxLedger())
	if err == nil && written {
		if err = siteSliceDaemonReload(); err == nil {
			pushSiteSlicesToKernel([]int{s.SiteID})
		}
	}
	siteSliceMu.Unlock()
	resp.SliceFile = result.Outcome
	if err != nil || result.Outcome == transport.SiteFileOutcomeFailed || result.Outcome == transport.SiteFileOutcomeRefused {
		resp.Error = fmt.Sprintf("the site's limits could not be written (%s %s): nothing was moved", result.Outcome, strings.TrimSpace(result.Reason+" "+result.Detail))
		if err != nil {
			resp.Error = "the site's limits could not be written: " + err.Error() + "; nothing was moved"
		}
		_ = os.Remove(siteMoveJournalPath)
		return nil
	}
	if siteMoveKilled("", "slice") {
		return nil
	}

	deadline := time.Now().Add(siteMoveBudget)
	var done []siteMoveUnit
	for i, m := range plan {
		// A program whose wait and put-back no longer fit the move's time is
		// not started: the rest are left as they are, to move next time.
		if m.running && time.Until(deadline) < 2*m.wait+siteSlicePoll {
			for _, rest := range plan[i:] {
				resp.Units = append(resp.Units, transport.SiteSliceMoved{Unit: rest.unit, Outcome: transport.SiteMoveNotTried,
					Detail: "the move's time ran out before this program; press the move again"})
				_ = journal.step(rest.unit, siteMoveStepPlanned)
			}
			resp.Error = "some programs were not moved in this round because the move's time ran out; press the move again"
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
		failed := transport.SiteSliceMoved{Unit: m.unit, Outcome: transport.SiteMoveFailed, Step: step, Detail: err.Error()}
		switch back, backErr := putBackSiteUnit(m, journal); {
		case backErr != nil:
			failed.Detail += "; putting it back did not complete: " + backErr.Error()
		case back == transport.SiteMoveChanged:
			failed.Detail += "; its file was changed meanwhile and is left as it is"
		default:
			failed.PutBack = true
		}
		resp.Units = append(resp.Units, failed)
		for j := len(done) - 1; j >= 0; j-- {
			back := transport.SiteSliceMoved{Unit: done[j].unit, Outcome: transport.SiteMovePutBack, PutBack: true}
			switch outcome, backErr := putBackSiteUnit(done[j], journal); {
			case backErr != nil:
				back.PutBack, back.Detail = false, "putting it back did not complete: "+backErr.Error()
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
		resp.Error = fmt.Sprintf("%s could not be moved into the site's limits (%s): %s; the site was put back as it was", m.unit, step, err.Error())
		_ = os.Remove(siteMoveJournalPath)
		return nil
	}
	_ = os.Remove(siteMoveJournalPath)
	forgetSiteMoveOutcome(s.SiteID)
	resp.Done = true
	return nil
}

// moveOneSiteUnit rewrites one unit under its lock and, if it ran, restarts
// it in the slice and waits for it to run there and listen again. A file
// that no longer holds the text the move planned from (changed or removed
// since) is not touched: outcome "changed". It returns the failed step;
// killed reports a simulated Agent stop.
func moveOneSiteUnit(m siteMoveUnit, siteID int, journal *siteMoveJournal) (outcome, step string, err error, killed bool) {
	unlock := siteUnitLock(m.unit)
	defer unlock()
	current, _, _, readErr := siteUnitText(m.unit)
	if readErr != nil || current != m.before {
		_ = journal.step(m.unit, siteMoveStepChanged)
		return transport.SiteMoveChanged, "", nil, false
	}
	if err := writeUnitBytes(m.file, m.after, m.mode); err != nil {
		return "", "write", err, false
	}
	if err := journal.step(m.unit, siteMoveStepWritten); err != nil {
		log.Printf("site slice: the move's record: %v", err)
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
	if err := journal.step(m.unit, siteMoveStepRestarting); err != nil {
		log.Printf("site slice: the move's record: %v", err)
	}
	if siteMoveKilled(m.unit, siteMoveStepRestarting) {
		return "", "", nil, true
	}
	_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
	if out, err := appUnitSystemctl(systemctlArgs("restart", m.unit)...); err != nil {
		return "", "restart", fmt.Errorf("systemctl restart: %v: %s", err, strings.TrimSpace(string(out))), false
	}
	want := "/" + sitesandbox.SliceName(siteID) + "/" + m.unit
	deadline := time.Now().Add(m.wait)
	for {
		props := map[string]string{}
		if out, err := appUnitSystemctl(systemctlArgs("show", m.unit, "--property=ActiveState,ControlGroup")...); err == nil {
			props = parseShow(string(out))
		}
		inSlice := props["ActiveState"] == "active" && props["ControlGroup"] == want
		listening := true
		if inSlice && len(m.ports) > 0 {
			now := siteSliceListening(m.unit)
			for port := range m.ports {
				listening = listening && now[port]
			}
		}
		if inSlice && listening {
			if err := journal.step(m.unit, siteMoveStepMoved); err != nil {
				log.Printf("site slice: the move's record: %v", err)
			}
			if siteMoveKilled(m.unit, siteMoveStepMoved) {
				return "", "", nil, true
			}
			return transport.SiteMoveMoved, "", nil, false
		}
		if time.Now().After(deadline) {
			seconds := int(m.wait / time.Second)
			switch {
			case props["ActiveState"] != "active":
				return "", "restart", fmt.Errorf("it is %s %d seconds after the restart (journalctl -u %s says why)", strings.TrimSpace(props["ActiveState"]+" "), seconds, m.unit), false
			case props["ControlGroup"] != want:
				return "", "cgroup", fmt.Errorf("it runs in %s, not %s", props["ControlGroup"], want), false
			}
			return "", "answer", fmt.Errorf("%d seconds after the restart it does not listen on every port it listened on before (%s)", seconds, siteSlicePorts(m.ports)), false
		}
		time.Sleep(siteSlicePoll)
	}
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
// ran, restarts it where it ran before. Only the move's own text is put back
// over: a changed or removed file is left ("changed"), so a customer's change
// is never lost and a deleted site's unit is never re-created.
func putBackSiteUnit(m siteMoveUnit, journal *siteMoveJournal) (string, error) {
	unlock := siteUnitLock(m.unit)
	defer unlock()
	current, _, _, err := siteUnitText(m.unit)
	if err != nil || (current != m.after && current != m.before) {
		if journal != nil {
			_ = journal.step(m.unit, siteMoveStepChanged)
		}
		return transport.SiteMoveChanged, nil
	}
	wrote := current == m.after
	if wrote {
		if err := writeUnitBytes(m.file, m.before, m.mode); err != nil {
			return "", err
		}
		if err := siteSliceDaemonReload(); err != nil {
			return "", err
		}
	}
	// A unit whose file the move never changed was never restarted either.
	if m.running && wrote {
		_, _ = appUnitSystemctl(systemctlArgs("reset-failed", m.unit)...)
		if out, err := appUnitSystemctl(systemctlArgs("restart", m.unit)...); err != nil {
			return "", fmt.Errorf("systemctl restart: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	if journal != nil {
		_ = journal.step(m.unit, siteMoveStepPutBack)
	}
	return transport.SiteMovePutBack, nil
}

// --- At the Agent's start ------------------------------------------------------

// recoverSiteSlicesAtStartup resolves what an Agent stop left in the middle:
// an interrupted move (by the table above) and the runtime settings a limit
// change pushes for a moment (site_slices_runtime.go). Before the RPC service
// starts, so nothing else changes these units meanwhile.
func recoverSiteSlicesAtStartup() {
	cleanupSiteSliceRuntime()
	recoverSiteSliceMove()
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
	want := "Slice=" + sitesandbox.SliceName(siteID)
	for _, e := range entries {
		if !siteSliceUnitPattern.MatchString(e.Name()) {
			continue
		}
		for _, line := range strings.Split(readSmallFile(filepath.Join(dir, e.Name())), "\n") {
			if strings.TrimSpace(line) == want {
				return true
			}
		}
	}
	return false
}

// recoverSiteSliceMove finishes or puts back an interrupted move from its
// journal, records what it did for the site's card and removes the journal.
func recoverSiteSliceMove() {
	journal, err := readSiteMoveJournal()
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		log.Printf("site slice: the record of an interrupted move could not be read (%v); it is kept as %s.unreadable for the administrator", err, siteMoveJournalPath)
		_ = os.Rename(siteMoveJournalPath, siteMoveJournalPath+".unreadable")
		return
	}
	if journal.Tasks != nil {
		recoverSiteTasksJournal(journal)
		if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("site tasks: remove the move's record: %v", err)
		}
		return
	}
	outcomes := map[string]string{}
	reload, restart := false, []string{}
	for _, u := range journal.Units {
		current, _, _, readErr := siteUnitText(u.Unit)
		ownText := readErr == nil && current == u.After
		switch {
		case u.Step == siteMoveStepPlanned:
			outcomes[u.Unit] = transport.SiteMoveNotTried
		case u.Step == siteMoveStepMoved:
			outcomes[u.Unit] = transport.SiteMoveMoved
		case u.Step == siteMoveStepPutBack:
			outcomes[u.Unit] = transport.SiteMovePutBack
		case u.Step == siteMoveStepChanged:
			outcomes[u.Unit] = transport.SiteMoveChanged
		case u.Step == siteMoveStepWritten && !u.Running:
			outcomes[u.Unit] = transport.SiteMoveRewritten
		case !ownText:
			// Someone changed or removed it after the move wrote it.
			outcomes[u.Unit] = transport.SiteMoveChanged
		default: // written (running) or restarting: the pre-move bytes go back
			if err := writeUnitBytes(u.File, u.Before, os.FileMode(u.Mode)); err != nil {
				log.Printf("site slice: %s could not be put back after an interrupted move: %v", u.Unit, err)
				outcomes[u.Unit] = transport.SiteMoveFailed
				continue
			}
			reload = true
			outcomes[u.Unit] = transport.SiteMovePutBack
			if u.Step == siteMoveStepRestarting {
				restart = append(restart, u.Unit)
			}
		}
	}
	if reload {
		if err := siteSliceDaemonReload(); err != nil {
			log.Printf("site slice: %v", err)
		}
	}
	for _, unit := range restart {
		_, _ = appUnitSystemctl(systemctlArgs("reset-failed", unit)...)
		if out, err := appUnitSystemctl(systemctlArgs("restart", unit)...); err != nil {
			log.Printf("site slice: %s was put back but did not restart: %v: %s", unit, err, strings.TrimSpace(string(out)))
			outcomes[unit] = transport.SiteMoveFailed
		}
	}
	// The slice file the move wrote goes again when it did not exist before
	// and no unit names it now.
	if !journal.SliceExisted && journal.SliceFile != "" && !siteSliceNamedByUnitFile(journal.SiteID) {
		if err := os.Remove(journal.SliceFile); err == nil {
			_ = updateSandboxLedger(func(l map[string]sandboxLedgerEntry) bool {
				delete(l, journal.SliceFile)
				return true
			})
			_ = siteSliceDaemonReload()
		}
	}
	at := time.Now().UTC().Format(time.RFC3339)
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		all[strconv.Itoa(journal.SiteID)] = transport.SiteSliceMoveRecovery{At: at, Units: outcomes}
		return true
	})
	log.Printf("site slice: the interrupted move of site %d was resolved at the Agent's start: %v", journal.SiteID, outcomes)
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site slice: remove the move's record: %v", err)
	}
}
