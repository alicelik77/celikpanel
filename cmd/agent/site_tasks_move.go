package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicelik/celikpanel/internal/hostingpath"
	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Moving a site's crontab into timers, and back (S4 R2b, 22 Nov 2026;
// docs/SITE-SANDBOX.md section 9.3). Part of the administrator's "Move into
// the limits" on the site's Overview; it holds the same locks as the
// programs' move (one move at a time, the site's lock without waiting) and
// cronMu, the lock of every change of a site's tasks.
//
// Nothing is half-moved: a line that cannot become a timer, or one not made
// in CelikPanel that the administrator has not kept, keeps the WHOLE crontab
// in cron and is named. Otherwise, in this order, each step recorded in the
// move journal (site-slice-move.json, the programs' move journal, version 2
// with a "tasks" part) before the next one starts:
//
//	planned          the crontab's exact bytes are recorded; nothing written yet
//	source_written   /etc/celikpanel-cron/<id>.crontab holds the crontab
//	units_written    every timer and service is written and checked by systemd, none started
//	crontab_emptied  `crontab -u <account> -r`
//	enabled          the timers are enabled and started (an @reboot one only enabled)
//
// No minute runs twice and none is lost: the timers start only after cron
// has no line any more. At the Agent's start an interrupted move is resolved
// by its step:
//
//	planned                         nothing to do ("not_tried")
//	source_written, units_written   put back: the crontab as recorded (written again only if it is
//	                                gone), every timer and file of the move removed ("put_back")
//	crontab_emptied                 finished: the timers are enabled ("moved"); if that fails, put back
//	enabled                         finished ("moved")
//
// The undo (the administrator's "Put the tasks back into cron") is refused
// before anything changes when the server owner's own /etc/cron.allow (or
// cron.deny) leaves the account out, or when the task file was changed by
// hand. Otherwise CelikPanel's cron.allow lets the account use cron again
// first, then the crontab is written (byte for byte the crontab moved, when
// the tasks did not change since), then the timers and files go. Its steps:
//
//	undo_planned      nothing changed yet ("not_tried")
//	crontab_written   the crontab is back; at the start the timers and files are removed ("undone")
//
// Moving tasks restarts nothing and stops no running task.
//
// Bir sitenin crontab'ı zamanlayıcılara yarım taşınmaz; her adım günlüğe
// yazılır ve Ajan açılışta adımına göre tamamlar ya da geri koyar.

// Steps of the tasks part of the move journal.
const (
	siteTasksStepPlanned        = "planned"
	siteTasksStepSourceWritten  = "source_written"
	siteTasksStepUnitsWritten   = "units_written"
	siteTasksStepCrontabEmptied = "crontab_emptied"
	siteTasksStepEnabled        = "enabled"
	siteTasksStepUndoPlanned    = "undo_planned"
	siteTasksStepCrontabWritten = "crontab_written"
)

// siteTasksJournal is the tasks part of the move journal.
type siteTasksJournal struct {
	Undo    bool   `json:"undo,omitempty"`
	Account string `json:"account"`
	// Crontab is the account's crontab as `crontab -l` printed it before the
	// move (or, for the undo, the crontab being written).
	Crontab string `json:"crontab"`
	Step    string `json:"step"`
	// Timers to enable after the crontab is empty; AtBoot ones are only
	// enabled.
	Timers []string        `json:"timers,omitempty"`
	AtBoot map[string]bool `json:"at_boot,omitempty"`
}

// Seams for tests.
var (
	// cronRemoveCrontab is `crontab -u <account> -r` ("no crontab" is fine).
	cronRemoveCrontab = func(username string) error {
		cmd := exec.Command("crontab", "-u", username, "-r")
		cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
		out, err := cmd.CombinedOutput()
		said := strings.TrimSpace(string(out))
		if err != nil && said != "no crontab for "+username {
			return fmt.Errorf("crontab -r: %s", said)
		}
		return nil
	}
	// siteTasksMoveKilled simulates the Agent stopping right after a step
	// (tests only): when it answers true the move returns at once and leaves
	// the journal.
	siteTasksMoveKilled = func(step string) bool { return false }
)

var siteTasksOutcomesPath = filepath.Join(hostingpath.ServiceMutationStateRoot(), "sandbox", "site-tasks-move-outcomes.json")

var siteTasksOutcomesMu sync.Mutex

func readSiteTasksOutcomes() map[string]string {
	out := map[string]string{}
	if raw, err := os.ReadFile(siteTasksOutcomesPath); err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func setSiteTasksOutcome(siteID int, outcome string) {
	siteTasksOutcomesMu.Lock()
	defer siteTasksOutcomesMu.Unlock()
	all := readSiteTasksOutcomes()
	key := strconv.Itoa(siteID)
	if outcome == "" {
		if _, ok := all[key]; !ok {
			return
		}
		delete(all, key)
	} else {
		all[key] = time.Now().UTC().Format(time.RFC3339) + " " + outcome
	}
	raw, err := json.Marshal(all)
	if err == nil {
		err = writePrivateFile(siteTasksOutcomesPath, raw)
	}
	if err != nil {
		log.Printf("site tasks: record the interrupted move: %v", err)
	}
}

// forgetSiteTasksUndo forgets what the Agent recorded about a site's tasks
// move (a deleted site).
func forgetSiteTasksUndo(siteID int) { setSiteTasksOutcome(siteID, "") }

// mailCommand: a task line that sends mail (it is then said in the preview
// what the move changes for it).
var mailCommand = regexp.MustCompile(`(?i)(sendmail|\bmailx?\b|\bmutt\b|\bswaks\b|smtp|:(25|465|587)\b)`)

// planSiteTasksMove says what the crontab becomes (read-only).
func planSiteTasksMove(username string, siteID int, crontab string) transport.SiteTasksMoveResponse {
	resp := transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksPreview}
	c := sitecron.Parse(crontab)
	classes := cronClassifyLines(username, crontab)
	next := 0
	n := 0
	for _, e := range c.Entries {
		recorded := cronRecordedLine(e.Raw)
		outside := false
		if recorded && next < len(classes) {
			outside = classes[next].Outside
			next++
		}
		switch {
		case e.Kind == sitecron.KindBlank || e.Kind == sitecron.KindComment:
			continue
		case outside:
			resp.Blocking = append(resp.Blocking, transport.SiteTaskLine{Line: e.Line, Text: e.Raw, Reason: transport.SiteTaskLineMarked,
				Detail: "not made in CelikPanel and not kept by the administrator"})
		case e.Kind == sitecron.KindInvalid:
			resp.Blocking = append(resp.Blocking, transport.SiteTaskLine{Line: e.Line, Text: e.Raw, Reason: e.Reason, Detail: e.Detail})
		case e.Kind == sitecron.KindEnv:
			resp.Rows = append(resp.Rows, transport.SiteTaskMoveRow{Line: e.Line, Text: e.Raw, Setting: true})
		case e.Kind == sitecron.KindDisabled:
			resp.Rows = append(resp.Rows, transport.SiteTaskMoveRow{Line: e.Line, Text: e.Raw, Paused: true})
		case e.Kind == sitecron.KindJob:
			n++
			resp.Rows = append(resp.Rows, transport.SiteTaskMoveRow{Line: e.Line, Text: e.Raw, Unit: sitecron.UnitBase(siteID, n),
				Calendars: e.Spec.Calendars, AtBoot: e.Spec.Reboot})
			if mailCommand.MatchString(e.Command) {
				resp.Mail = true
			}
		}
	}
	if len(resp.Blocking) > 0 {
		resp.Outcome = transport.SiteTasksStayed
	}
	return resp
}

// siteTasksTenantTarget proves the tenant (a test seam).
var siteTasksTenantTarget = tasksMoveTarget

// tasksMoveTarget proves the tenant and refuses what cannot move at all.
func tasksMoveTarget(req *transport.SiteTasksMoveRequest) (siteTaskTarget, string, error) {
	if req == nil {
		return siteTaskTarget{}, "", errors.New("a request is required")
	}
	username, err := cronTenantUser(req.CronTenant)
	if err != nil {
		return siteTaskTarget{}, "", err
	}
	target, err := siteTaskTargetFor(req.CronTenant, username)
	if err != nil {
		return siteTaskTarget{}, "", err
	}
	return target, username, nil
}

// MoveSiteTasksV1 moves a site's crontab into timers in its slice (Preview:
// says what would become what and changes nothing).
func (a *Agent) MoveSiteTasksV1(req *transport.SiteTasksMoveRequest, resp *transport.SiteTasksMoveResponse) error {
	*resp = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused}
	if req != nil && !req.Preview {
		if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "moving a site's tasks into its limits"); err != nil {
			resp.Error = err.Error()
			return nil
		}
	}
	target, username, err := siteTasksTenantTarget(req)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if reason := siteSliceHostSupport(); reason != "" {
		resp.Error = "this server cannot hold a site's limits (missing: " + reason + "); the tasks stay in cron"
		return nil
	}
	if siteTasksMoved(target.SiteID) {
		resp.Outcome = transport.SiteTasksAlready
		return nil
	}
	if err := requireCronInstalled(); err != nil {
		resp.Outcome, resp.Error = transport.SiteTasksNoCron, err.Error()
		return nil
	}
	if req.Preview {
		crontab, err := readCrontab(username)
		if err != nil {
			resp.Outcome, resp.Error = transport.SiteTasksNoCron, err.Error()
			return nil
		}
		*resp = planSiteTasksMove(username, target.SiteID, crontab)
		return nil
	}
	if !siteMoveRunning.TryLock() {
		resp.Refusal, resp.Error = transport.SiteTasksRefusalBusy, "another move into a site's limits is running; nothing was changed"
		return nil
	}
	defer siteMoveRunning.Unlock()
	siteMu := siteMutationMutex(target.SiteID)
	if !siteMu.TryLock() {
		resp.Refusal, resp.Error = transport.SiteTasksRefusalBusy, "the site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute"
		return nil
	}
	defer siteMu.Unlock()
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		recoverSiteSliceMove()
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	*resp = moveSiteTasksLocked(target, username)
	return nil
}

// moveSiteTasksLocked is the move itself (the caller holds the locks).
func moveSiteTasksLocked(target siteTaskTarget, username string) transport.SiteTasksMoveResponse {
	if siteTasksMoved(target.SiteID) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
	}
	crontab, err := readCrontab(username)
	if err != nil {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksNoCron, Error: err.Error()}
	}
	resp := planSiteTasksMove(username, target.SiteID, crontab)
	if len(resp.Blocking) > 0 {
		resp.Outcome = transport.SiteTasksStayed
		return resp
	}
	journal := &siteMoveJournal{Version: 2, SiteID: target.SiteID, StartedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks: &siteTasksJournal{Account: username, Crontab: crontab, Step: siteTasksStepPlanned}}
	if err := journal.save(); err != nil {
		resp.Outcome, resp.Error = transport.SiteTasksRefused, "the move's record could not be written in the Agent's folder ("+err.Error()+"); nothing was moved"
		return resp
	}
	if siteTasksMoveKilled(siteTasksStepPlanned) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	fail := func(step string, cause error) transport.SiteTasksMoveResponse {
		out := resp
		out.Outcome = transport.SiteTasksPutBack
		out.Error = fmt.Sprintf("the tasks could not be moved (%s): %v; the crontab is as it was and no timer was left", step, cause)
		if backErr := putBackSiteTasks(target.SiteID, journal.Tasks); backErr != nil {
			out.Outcome = transport.SiteTasksUnknown
			out.Error = fmt.Sprintf("the tasks could not be moved (%s): %v; putting them back did not complete (%v): read the site's tasks before you try again", step, cause, backErr)
		}
		_ = os.Remove(siteMoveJournalPath)
		return out
	}
	result, err := writeSiteTaskSource(target, crontab, transport.SiteFileTriggerCreate)
	if err == nil && result.Outcome != transport.SiteFileOutcomeWritten && result.Outcome != transport.SiteFileOutcomeRecreated {
		err = fmt.Errorf("%s was not written (%s %s)", sitecron.SourcePath(target.SiteID), result.Outcome, result.Reason)
	}
	if err != nil {
		return fail("task file", err)
	}
	if stepErr := journal.tasksStep(siteTasksStepSourceWritten); stepErr != nil {
		return fail("record", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepSourceWritten) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	applied, err := applySiteTimersHeld(target, crontab)
	if err != nil {
		return fail("timers", err)
	}
	if len(applied.Kept) > 0 {
		return fail("timers", fmt.Errorf("files of this site's tasks were already there and changed on the server: %s", strings.Join(applied.Kept, ", ")))
	}
	journal.Tasks.AtBoot = map[string]bool{}
	for _, task := range applied.Tasks {
		journal.Tasks.Timers = append(journal.Tasks.Timers, task.Unit+".timer")
		if task.Entry.Spec.Reboot {
			journal.Tasks.AtBoot[task.Unit+".timer"] = true
		}
	}
	if stepErr := journal.tasksStep(siteTasksStepUnitsWritten); stepErr != nil {
		return fail("record", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepUnitsWritten) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	// The crontab must still be the one moved: a change meanwhile (the site's
	// own code, the owner) is not lost.
	if now, err := readCrontab(username); err != nil || now != crontab {
		if err == nil {
			err = errors.New("the crontab changed while it was being moved")
		}
		out := fail("crontab", err)
		out.Refusal = transport.SiteTasksRefusalCrontab
		return out
	}
	if err := cronRemoveCrontab(username); err != nil {
		return fail("crontab", err)
	}
	if stepErr := journal.tasksStep(siteTasksStepCrontabEmptied); stepErr != nil {
		log.Printf("site tasks: the move's record: %v", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepCrontabEmptied) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	if err := enableSiteTaskTimers(journal.Tasks); err != nil {
		return fail("enable", err)
	}
	if stepErr := journal.tasksStep(siteTasksStepEnabled); stepErr != nil {
		log.Printf("site tasks: the move's record: %v", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepEnabled) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	_ = os.Remove(siteMoveJournalPath)
	setSiteTasksOutcome(target.SiteID, "")
	refreshCronAllow()
	resp.Outcome = transport.SiteTasksMoved
	return resp
}

func (j *siteMoveJournal) tasksStep(step string) error {
	j.Tasks.Step = step
	return j.save()
}

// applySiteTimersHeld writes the timers without enabling any (the move
// enables them after the crontab is empty).
func applySiteTimersHeld(t siteTaskTarget, crontab string) (siteTimersResult, error) {
	siteTimersHoldEnable = true
	defer func() { siteTimersHoldEnable = false }()
	return applySiteTimers(t, crontab)
}

// siteTimersHoldEnable: applySiteTimers writes new timers but does not
// enable them (set only by the move under cronMu).
var siteTimersHoldEnable bool

// enableSiteTaskTimers enables (and starts) the move's timers.
func enableSiteTaskTimers(j *siteTasksJournal) error {
	for _, timer := range j.Timers {
		args := []string{"enable", "--now", timer}
		if j.AtBoot[timer] {
			args = []string{"enable", timer}
		}
		if out, err := siteTaskSystemctl(args...); err != nil {
			return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// putBackSiteTasks undoes an unfinished move: the crontab as recorded
// (written again only when it is gone: a crontab that differs was changed by
// someone else and is left), then every timer and file of the site's tasks.
func putBackSiteTasks(siteID int, j *siteTasksJournal) error {
	var failures []string
	if j.Crontab != "" {
		now, err := readCrontab(j.Account)
		switch {
		case err != nil:
			failures = append(failures, "the crontab could not be read: "+err.Error())
		case now == "":
			if err := cronInstallCrontab(j.Account, j.Crontab); err != nil {
				failures = append(failures, "the crontab could not be written back: "+err.Error())
			}
		case now != j.Crontab:
			log.Printf("site tasks: the crontab of %s changed during the move; it is left as it is", j.Account)
		}
	}
	if _, err := removeSiteTasksLocked(siteID, false); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// recoverSiteTasksJournal resolves an interrupted tasks move or undo at the
// Agent's start (the table above).
func recoverSiteTasksJournal(journal *siteMoveJournal) {
	j := journal.Tasks
	cronMu.Lock()
	defer cronMu.Unlock()
	outcome := ""
	switch {
	case j.Undo && j.Step == siteTasksStepCrontabWritten:
		if _, err := removeSiteTasksLocked(journal.SiteID, false); err != nil {
			log.Printf("site tasks: the interrupted undo of site %d: %v", journal.SiteID, err)
			outcome = transport.SiteTasksUnknown
		} else {
			outcome = transport.SiteTasksUndone
		}
		refreshCronAllow()
	case j.Undo:
		outcome = "not_tried"
		refreshCronAllow()
	case j.Step == siteTasksStepPlanned:
		outcome = "not_tried"
	case j.Step == siteTasksStepSourceWritten || j.Step == siteTasksStepUnitsWritten:
		outcome = transport.SiteTasksPutBack
		if err := putBackSiteTasks(journal.SiteID, j); err != nil {
			log.Printf("site tasks: the interrupted move of site %d could not be put back: %v", journal.SiteID, err)
			outcome = transport.SiteTasksUnknown
		}
	case j.Step == siteTasksStepCrontabEmptied:
		outcome = transport.SiteTasksMoved
		if err := enableSiteTaskTimers(j); err != nil {
			log.Printf("site tasks: the interrupted move of site %d could not be finished (%v); it is put back", journal.SiteID, err)
			outcome = transport.SiteTasksPutBack
			if err := putBackSiteTasks(journal.SiteID, j); err != nil {
				outcome = transport.SiteTasksUnknown
			}
		}
		refreshCronAllow()
	default: // enabled
		outcome = transport.SiteTasksMoved
		refreshCronAllow()
	}
	setSiteTasksOutcome(journal.SiteID, outcome)
	log.Printf("site tasks: the interrupted tasks move of site %d was resolved at the Agent's start: %s", journal.SiteID, outcome)
}

// UndoSiteTasksMoveV1 puts a moved site's tasks back into its crontab.
func (a *Agent) UndoSiteTasksMoveV1(req *transport.SiteTasksMoveRequest, resp *transport.SiteTasksMoveResponse) error {
	*resp = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused}
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "putting a site's tasks back into cron"); err != nil {
		resp.Error = err.Error()
		return nil
	}
	target, username, err := siteTasksTenantTarget(req)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	if !siteMoveRunning.TryLock() {
		resp.Refusal, resp.Error = transport.SiteTasksRefusalBusy, "a move into a site's limits is running; nothing was changed"
		return nil
	}
	defer siteMoveRunning.Unlock()
	siteMu := siteMutationMutex(target.SiteID)
	if !siteMu.TryLock() {
		resp.Refusal, resp.Error = transport.SiteTasksRefusalBusy, "the site is being changed right now; nothing was changed. Read its page again in a minute"
		return nil
	}
	defer siteMu.Unlock()
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		recoverSiteSliceMove()
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	*resp = undoSiteTasksLocked(target, username)
	return nil
}

func undoSiteTasksLocked(target siteTaskTarget, username string) transport.SiteTasksMoveResponse {
	refused := func(code, text string) transport.SiteTasksMoveResponse {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: code, Error: text}
	}
	if !siteTasksMoved(target.SiteID) {
		return refused("", "the site's tasks already run in cron; nothing was changed")
	}
	source, err := readSiteTaskSource(target.SiteID)
	switch {
	case err != nil:
		return refused("", "the task file could not be read ("+err.Error()+"); nothing was changed")
	case !source.Exists:
		return refused(transport.SiteTasksRefusalSource, "the task file "+sitecron.SourcePath(target.SiteID)+" is gone while timers of the site remain; the server owner decides (remove the timers or put the file back); nothing was changed")
	case source.State != transport.SiteFileManagedUnchanged:
		return refused(transport.SiteTasksRefusalSource, "the server owner changed "+sitecron.SourcePath(target.SiteID)+" by hand; putting the tasks back would lose that change, so nothing was changed")
	case source.Account != "" && source.Account != username:
		return refused("", "the task file names another account; nothing was changed")
	}
	if err := requireCronInstalled(); err != nil {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksNoCron, Error: err.Error()}
	}
	// The account must be able to use cron again: the owner's own file is
	// never changed, so it refuses here before anything changes.
	if why := cronAllowRefuses(username, target.SiteID); why != "" {
		return refused(transport.SiteTasksRefusalCronAllow, why)
	}
	written, authored := withSiteCronMailtoLines(username, source.Crontab)
	authored = append(authored, cronEffectiveLines(source.Crontab)...)
	journal := &siteMoveJournal{Version: 2, SiteID: target.SiteID, StartedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks: &siteTasksJournal{Undo: true, Account: username, Crontab: written, Step: siteTasksStepUndoPlanned}}
	if err := journal.save(); err != nil {
		return refused("", "the record could not be written in the Agent's folder ("+err.Error()+"); nothing was changed")
	}
	if siteTasksMoveKilled(siteTasksStepUndoPlanned) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	if err := allowCronFor(username, target.SiteID); err != nil {
		_ = os.Remove(siteMoveJournalPath)
		return refused(transport.SiteTasksRefusalCronAllow, err.Error())
	}
	existing, err := readCrontab(username)
	if err != nil {
		_ = os.Remove(siteMoveJournalPath)
		refreshCronAllow()
		return refused("", "the crontab could not be read ("+err.Error()+"); nothing was changed")
	}
	if strings.TrimSpace(existing) != "" {
		_ = os.Remove(siteMoveJournalPath)
		refreshCronAllow()
		return refused(transport.SiteTasksRefusalCrontab, "the account has a crontab again (the server owner's, running outside the site's limits); it is never overwritten, so nothing was changed. The server owner merges or removes it first")
	}
	if err := setSiteCrontab(username, "", written, authored); err != nil {
		_ = os.Remove(siteMoveJournalPath)
		refreshCronAllow()
		return refused("", "the crontab could not be written ("+err.Error()+"); nothing was changed")
	}
	if stepErr := journal.tasksStep(siteTasksStepCrontabWritten); stepErr != nil {
		log.Printf("site tasks: the undo's record: %v", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepCrontabWritten) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	}
	if _, err := removeSiteTasksLocked(target.SiteID, false); err != nil {
		_ = os.Remove(siteMoveJournalPath)
		refreshCronAllow()
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown,
			Error: "the crontab is back, but not every timer could be removed (" + err.Error() + "); read the site's tasks: a task may now run twice until its timer is removed"}
	}
	_ = os.Remove(siteMoveJournalPath)
	setSiteTasksOutcome(target.SiteID, "")
	refreshCronAllow()
	return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUndone}
}

// SiteTasksMovedV1 lists the sites whose tasks run as timers, the crontabs
// that came back for their accounts and what the Agent did with an
// interrupted tasks move (read-only).
func (a *Agent) SiteTasksMovedV1(req *transport.SiteTasksMovedRequest, resp *transport.SiteTasksMovedResponse) error {
	*resp = transport.SiteTasksMovedResponse{Moved: []int{}}
	want := map[int]bool{}
	if req != nil {
		for _, id := range req.SiteIDs {
			want[id] = true
		}
	}
	moved, accounts := movedSiteTaskSites()
	for _, id := range moved {
		if len(want) > 0 && !want[id] {
			continue
		}
		resp.Moved = append(resp.Moved, id)
		if account := accounts[id]; account != "" {
			if n := len(spoolCrontabLines(account)); n > 0 {
				if resp.Reappeared == nil {
					resp.Reappeared = map[string]int{}
				}
				resp.Reappeared[strconv.Itoa(id)] = n
			}
		}
	}
	for key, outcome := range readSiteTasksOutcomes() {
		id, _ := strconv.Atoi(key)
		if len(want) > 0 && !want[id] {
			continue
		}
		if resp.Recovery == nil {
			resp.Recovery = map[string]string{}
		}
		resp.Recovery[key] = outcome
	}
	return nil
}

// movedSiteTaskSites are the moved sites (a task file, or timers whose file
// is gone) and the account each task file names.
func movedSiteTaskSites() ([]int, map[int]string) {
	seen := map[int]bool{}
	accounts := map[int]string{}
	if entries, err := os.ReadDir(sitecron.SourceDir); err == nil {
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".crontab")
			if !ok {
				continue
			}
			id, err := strconv.Atoi(name)
			if err != nil || id < 1 || strconv.Itoa(id) != name {
				continue
			}
			seen[id] = true
			if raw, err := os.ReadFile(filepath.Join(sitecron.SourceDir, e.Name())); err == nil && len(raw) < 1<<20 {
				_, accounts[id] = splitSiteTaskSource(raw)
			}
		}
	}
	if dir, err := sandboxUnitDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if site, _, _, ok := sitecron.ParseUnit(e.Name()); ok {
					seen[site] = true
				}
			}
		}
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, accounts
}

// removeSiteTasksLocked removes every timer and file of a site's tasks
// (the caller holds cronMu). aside: files the server owner changed are
// moved aside (a deleted site) instead of kept.
func removeSiteTasksLocked(siteID int, aside bool) ([]string, error) {
	var moved, failures []string
	units, err := siteTaskUnits(siteID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		failures = append(failures, err.Error())
	}
	dir, _ := sandboxUnitDir()
	reload := false
	keep := func(path string) {
		if !aside {
			failures = append(failures, path+" was changed on this server and is kept")
			return
		}
		name, err := moveSliceFileAside(path)
		if err != nil {
			failures = append(failures, err.Error())
			return
		}
		moved = append(moved, name)
		reload = true
	}
	for n := range units {
		base := sitecron.UnitBase(siteID, n)
		if out, err := siteTaskSystemctl("disable", "--now", base+".timer"); err != nil && units[n].Timer {
			log.Printf("site tasks: %s.timer: %v: %s", base, err, strings.TrimSpace(string(out)))
		}
		for _, f := range []struct{ kind, path string }{
			{siteTaskTimerKind, filepath.Join(dir, base+".timer")},
			{siteTaskServiceKind, filepath.Join(dir, base+".service")},
		} {
			removed, kept := removeTaskFile(f.kind, f.path, true)
			if kept {
				keep(f.path)
			}
			reload = reload || removed
		}
	}
	taskDir := sitecron.TaskDir(siteID)
	if entries, err := os.ReadDir(taskDir); err == nil {
		for _, e := range entries {
			path := filepath.Join(taskDir, e.Name())
			kind, header := siteTaskScriptKind, true
			switch {
			case strings.HasSuffix(e.Name(), ".in"):
				kind, header = siteTaskInputKind, false
			case e.Name() == "env":
				kind = siteTaskEnvKind
			}
			if _, kept := removeTaskFile(kind, path, header); kept {
				keep(path)
			}
		}
		if err := os.Remove(taskDir); err != nil && !errors.Is(err, fs.ErrNotExist) && aside {
			if name, err := moveSliceFileAside(taskDir); err == nil {
				moved = append(moved, name)
			}
		}
	}
	source := sitecron.SourcePath(siteID)
	if _, kept := removeTaskFile(siteTaskSourceKind, source, true); kept {
		keep(source)
	}
	if reload {
		if err := siteSliceDaemonReload(); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return moved, errors.New(strings.Join(failures, "; "))
	}
	return moved, nil
}
