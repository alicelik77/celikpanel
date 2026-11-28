package main

import (
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
	"time"

	"github.com/alicelik/celikpanel/internal/sitecron"
	"github.com/alicelik/celikpanel/internal/sitesandbox"
	"github.com/alicelik/celikpanel/internal/transport"
)

// Moving a site's crontab into timers, and back (S4 R2b, 22 Nov 2026, with
// its fix round; docs/SITE-SANDBOX.md section 9.3). Part of the
// administrator's "Move into the limits" on the site's Overview; it holds
// the same locks as the programs' move (one move at a time, the site's lock
// without waiting) and cronMu, the lock of every change of a site's tasks.
//
// Nothing is half-moved: a line that cannot become a timer, or one not made
// in CelikPanel that the administrator has not kept, keeps the WHOLE crontab
// in cron and is named. A move after which the account could still install
// its own jobs (the server owner's cron.allow lets it in, CelikPanel's file
// was deleted by the owner, accounts from a directory service) is refused
// before anything changes, with who acts. Otherwise, in this order, each
// step recorded in the move journal (site-slice-move.json, version 2 with a
// "tasks" part) before the next one starts:
//
//	planned          the crontab's exact bytes (as `crontab -l` printed them) are recorded
//	source_written   /etc/celikpanel-cron/<id>.crontab holds the crontab
//	units_written    every timer and service written and checked by systemd, none started
//	cron_closed      /etc/cron.allow leaves the account out (checked), and every crontab
//	                 process of the account's uid started before that is ended (if they
//	                 cannot be listed, the move fails here and is put back)
//	crontab_emptied  the spool file, checked to hold the recorded crontab, is removed
//	                 directly (crontab -u is refused by then) and checked gone
//	enabled          the timers are enabled and started (an @reboot one only enabled)
//
// There is no moment in which the site's own code can put a crontab back:
// the account is out of cron.allow before the crontab goes. No minute runs
// twice: the timers start only after cron has no line any more. One run can
// be missed (alpha.150 fix round): a minute can begin between the crontab's
// removal and the timers' start, and a task due then runs at its next time.
// At the Agent's start an interrupted move is resolved by its step:
//
//	planned                                   nothing to do ("not_tried")
//	source_written, units_written, cron_closed put back (below)
//	crontab_emptied                           finished: the timers are enabled ("moved"); if
//	                                          that fails, kept (the start) or put back (Try again)
//	enabled                                   finished ("moved")
//
// alpha.150 fix round (F1): at crontab_emptied and enabled the account's
// spool is read first. A crontab there (or a spool that cannot be read)
// never gets timers beside it: the timers are switched off and the record
// stays with how the tasks run (cron); at Try again a crontab that holds the
// recorded tasks (put back by hand) finishes the put-back. A record at
// enabled is saved as crontab_emptied before its timers are switched off
// (fix round 4, N1), so once the crontab is gone again the timers are
// switched on again, never left off under "moved".
//
// Putting back disables the move's timers, lets the account use cron again
// and writes the recorded crontab back when it is gone, checks it, and only
// then removes the timers, the files and the journal. A crontab that could
// not be written back keeps the journal, the task file and the units
// (switched on again where they can be), so nothing is lost and the next
// start tries again.
//
// The undo (the administrator's "Put the tasks back into cron") is refused
// before anything changes when cron's access files would keep the account
// out, when the task file was changed by hand, or when the account has a
// crontab again. It is journaled the same way; the crontab and the timers
// are never both on and never both gone:
//
//	undo_planned     the crontab to write (the task file's bytes; a task whose timer was
//	                 switched off on the server comes back paused, F6) and the timers that are on
//	timers_disabled  those timers are disabled (a running task is not stopped)
//	cron_opened      CelikPanel's cron.allow lets the account in again
//	crontab_written  the crontab is written and checked (`crontab -l` gives the bytes back)
//	(then the timers and files are removed and the journal goes)
//
// At the Agent's start: undo_planned and timers_disabled switch the timers
// on again and close cron again ("not_tried"); cron_opened finishes when the
// spool holds the journal's crontab ("undone"), otherwise it rolls back;
// crontab_written finishes ("undone").
//
// Moving tasks restarts nothing and stops no running task.
//
// Bir sitenin crontab'ı zamanlayıcılara yarım taşınmaz; hesap crontab'ı
// silinmeden önce cron.allow'dan çıkarılır; her adım günlüğe yazılır ve Ajan
// açılışta adımına göre tamamlar ya da geri koyar.

// Steps of the tasks part of the move journal.
const (
	siteTasksStepPlanned        = "planned"
	siteTasksStepSourceWritten  = "source_written"
	siteTasksStepUnitsWritten   = "units_written"
	siteTasksStepCronClosed     = "cron_closed"
	siteTasksStepCrontabEmptied = "crontab_emptied"
	siteTasksStepEnabled        = "enabled"
	siteTasksStepUndoPlanned    = "undo_planned"
	siteTasksStepTimersDisabled = "timers_disabled"
	siteTasksStepCronOpened     = "cron_opened"
	siteTasksStepCrontabWritten = "crontab_written"
)

// siteTasksJournal is the tasks part of the move journal.
type siteTasksJournal struct {
	Undo    bool   `json:"undo,omitempty"`
	Account string `json:"account"`
	UID     int    `json:"uid,omitempty"`
	// Crontab is the account's crontab as `crontab -l` printed it before the
	// move (or, for the undo, the crontab being written).
	Crontab string `json:"crontab"`
	Step    string `json:"step"`
	// Timers to enable after the crontab is empty (the undo: the timers that
	// were on); AtBoot ones are only enabled.
	Timers []string        `json:"timers,omitempty"`
	AtBoot map[string]bool `json:"at_boot,omitempty"`
}

// Seams for tests.
var (
	// cronRemoveCrontab is `crontab -u <account> -r` ("no crontab" is fine);
	// used by a site's deletion (the move removes the spool file itself).
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
	// siteTasksMoveKilled simulates the Agent stopping (tests only): it is
	// asked with "act:<step>" right after a step's action and before its
	// record, and with "<step>" right after the record; when it answers true
	// the move returns at once and leaves the journal.
	siteTasksMoveKilled = func(point string) bool { return false }
)

// siteTasksJournalVersion is the version of a move record that holds a
// site's tasks (alpha.150: the tasks part of main's one move record; the
// programs' record is version 2). An earlier Agent reads the record and
// finds no program in it.
const siteTasksJournalVersion = 3

// removeSiteMoveRecord removes the move's record once its move or undo is
// settled. The caller holds the move's claim (claimSiteMove) and started
// only when no record was kept, so the record is its own.
func removeSiteMoveRecord() {
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site tasks: remove the move's record: %v", err)
	}
}

// keptTasksRecovery is the outcome record of a kept tasks move, for the
// answer.
func keptTasksRecovery(r transport.SiteSliceMoveRecovery) *transport.SiteSliceMoveRecovery { return &r }

// mailCommand: a task line that sends mail (it is then said in the preview
// what the move changes for it).
var mailCommand = regexp.MustCompile(`(?i)(sendmail|\bmailx?\b|\bmutt\b|\bswaks\b|smtp|:(25|465|587)\b)`)

// privilegedPrograms are common set-user-id or set-group-id programs (or
// ones with file capabilities), and the server's sendmail with the mail
// commands that call it (Postfix's sendmail hands mail to the set-group-id
// postdrop). Inside a task they run without those rights (the service's
// NoNewPrivileges=yes, second fix round) and may fail; the preview names
// the tasks that call one.
var privilegedPrograms = map[string]bool{
	"sudo": true, "su": true, "crontab": true, "at": true, "batch": true, "ping": true, "ping6": true,
	"traceroute": true, "traceroute6": true, "dotlockfile": true, "lockfile": true, "mail-lock": true,
	"wall": true, "write": true, "newgrp": true, "sg": true, "passwd": true, "chsh": true, "chfn": true,
	"mount": true, "umount": true, "fusermount": true, "fusermount3": true, "pkexec": true,
	"ssh-agent": true, "unix_chkpwd": true, "sendmail": true, "mail": true, "mailx": true,
}

// commandWrappers run the next word as the command (it stays in command
// position).
var commandWrappers = map[string]bool{"nohup": true, "exec": true, "env": true, "command": true, "nice": true, "time": true}

var shellAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// privilegedCommand (second fix round, review N13): the command calls one of
// privilegedPrograms in command position - its first word, or the first
// word after ;, &, &&, ||, |, a newline or a parenthesis, past NAME=value
// settings and plain wrappers such as nohup - by the basename of that word.
// A word anywhere else ("echo backup at noon") is not a call.
func privilegedCommand(command string) bool {
	for _, word := range commandWords(command) {
		if privilegedPrograms[filepath.Base(word)] {
			return true
		}
	}
	return false
}

// commandWords are the words of a shell command in command position. Quotes
// and backslashes are honoured: a separator inside quotes does not count.
func commandWords(command string) []string {
	var out []string
	var word strings.Builder
	atStart, inWord, single, double, escaped := true, false, false, false, false
	prev := rune(0) // the last character outside quotes that is not a blank
	finish := func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord = false
		if !atStart {
			return
		}
		switch {
		case shellAssignment.MatchString(w):
			// NAME=value before the command: the next word is the command.
		case commandWrappers[filepath.Base(w)]:
			out = append(out, w)
		default:
			out = append(out, w)
			atStart = false
		}
	}
	for _, r := range command {
		switch {
		case escaped:
			escaped = false
			word.WriteRune(r)
			inWord = true
			continue
		case single:
			if r == '\'' {
				single = false
			} else {
				word.WriteRune(r)
			}
			continue
		case double:
			switch r {
			case '"':
				double = false
			case '\\':
				escaped = true
			default:
				word.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\\':
			escaped, inWord = true, true
		case '\'':
			single, inWord = true, true
		case '"':
			double, inWord = true, true
		case ' ', '\t':
			finish()
		case ';', '|', '(', ')', '`', '\n':
			finish()
			atStart = true
		case '&':
			finish()
			// "2>&1", ">&2" are redirections, not a new command.
			if prev != '>' && prev != '<' {
				atStart = true
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
		if r != ' ' && r != '\t' {
			prev = r
		}
	}
	finish()
	return out
}

// planSiteTasksMove says what the crontab becomes (read-only).
func planSiteTasksMove(username string, siteID int, crontab string) transport.SiteTasksMoveResponse {
	resp := transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksPreview, MailSubmission: recordedMailSubmission()}
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
				Calendars: e.Spec.Calendars, AtBoot: e.Spec.Reboot, Privileged: privilegedCommand(e.Command)})
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
	defer logSiteTasksAnswer("move", req, resp)
	if req != nil && !req.Preview {
		if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "moving a site's tasks into its limits"); err != nil {
			// alpha.150 lab D6: coded as the programs' move codes it, so the
			// panel's worker waits for the update instead of marking a failure.
			resp.Refusal, resp.Error = "build", err.Error()
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
		// A move that would be refused before anything changes says so in
		// the preview too (Try again's question names it, with why).
		if resp.Outcome == transport.SiteTasksPreview {
			if why := cronMoveRefusal(username); why != "" {
				resp.Outcome, resp.Refusal, resp.Error = transport.SiteTasksRefused, transport.SiteTasksRefusalCronAllow, why
			}
		}
		return nil
	}
	// alpha.150: main's claim of a move (claimSiteMove): refused while the
	// panel's update is in progress or another move runs, with main's codes
	// and words, and the update is refused while this move runs. One press,
	// one deadline: settling this site's own kept record shares it.
	refusal, release := claimSiteMove()
	if refusal != "" {
		resp.Refusal, resp.Error = refusal, siteMoveRefusalText(refusal)
		return nil
	}
	defer release()
	deadline := time.Now().Add(siteMoveBudget)
	siteMu := siteMutationMutex(target.SiteID)
	if !siteMu.TryLock() {
		resp.Refusal, resp.Error = "site_busy", "the site is being changed right now (for example deleted); nothing was moved. Read its page again in a minute"
		return nil
	}
	defer siteMu.Unlock()
	// One kept move record (programs' or tasks', of any site) refuses every
	// new move and undo until it is settled (site_slices_move.go).
	if why := siteTasksKeptRefusal(target.SiteID, deadline); why != "" {
		resp.Refusal, resp.Error = transport.SiteMoveRefusalKept, why
		return nil
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	*resp = moveSiteTasksLocked(target, username)
	return nil
}

// logSiteTasksAnswer puts a tasks move's or undo's refusal or failure in the
// Agent's journal (alpha.150: the screens word its code and send the
// administrator here for the detail, never the Agent's English).
func logSiteTasksAnswer(what string, req *transport.SiteTasksMoveRequest, resp *transport.SiteTasksMoveResponse) {
	if req == nil || req.Preview || resp == nil || resp.Error == "" {
		return
	}
	log.Printf("site tasks: the %s of site %d (%s %s): %s", what, req.SiteID, resp.Outcome, resp.Refusal, resp.Error)
}

// siteTasksKeptRefusal is main's rule for a kept move record (alpha.149 fix
// round F4; S4 R2b second fix round N1; alpha.150: one record for programs
// and tasks), applied to a tasks move or undo: the record of ANOTHER site
// refuses at once, before anything of that site is touched (its own Try
// again settles it); a record of this site is settled first, within the
// press's one deadline; a record still there refuses. "" when none is kept.
// The caller holds the move's claim.
func siteTasksKeptRefusal(siteID int, deadline time.Time) string {
	keptEarlier := func(id int) string {
		site := "a site"
		if id > 0 {
			site = "site " + strconv.Itoa(id)
		}
		return "an earlier move of " + site + " could not be put back completely; its page names what is left and the next step. Nothing new was moved until that is settled"
	}
	if journal, err := readSiteMoveJournal(); err == nil && journal.SiteID != siteID {
		return keptEarlier(journal.SiteID)
	}
	if _, err := os.Stat(siteMoveJournalPath); err != nil {
		return ""
	}
	kept := recoverSiteSliceMove(deadline, false)
	if _, err := os.Stat(siteMoveJournalPath); err == nil {
		id := 0
		if kept != nil {
			id = kept.siteID
		}
		return keptEarlier(id)
	}
	return ""
}

// moveSiteTasksLocked is the move itself (the caller holds the locks).
func moveSiteTasksLocked(target siteTaskTarget, username string) transport.SiteTasksMoveResponse {
	if siteTasksMoved(target.SiteID) {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksAlready}
	}
	refused := func(code, text string) transport.SiteTasksMoveResponse {
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused, Refusal: code, Error: text}
	}
	// Before anything changes: cron's access files must keep the account out
	// once the move is done.
	if why := cronMoveRefusal(username); why != "" {
		return refused(transport.SiteTasksRefusalCronAllow, why)
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
	// No task runs without the site's limits: their file first.
	if len(sitecron.Parse(crontab).Jobs()) > 0 {
		if err := requireTaskSlice(target); err != nil {
			out := refused(transport.SiteTasksRefusalLimits, strings.TrimPrefix(err.Error(), transport.SiteTasksLimitsMissing+": ")+"; nothing was moved")
			out.Rows = resp.Rows
			return out
		}
	}
	// alpha.150 fix round (F7): the account's crontab processes must be
	// listable, or the move is refused here, before anything changes (they
	// are listed again and ended once cron.allow has closed).
	if err := siteTaskCheckCrontabs(target.UID); err != nil {
		out := refused(transport.SiteTasksRefusalProcesses, "the crontab processes of "+username+" could not be listed ("+err.Error()+"), so one started before cron closes could still install a crontab; nothing was moved")
		out.Rows = resp.Rows
		return out
	}
	journal := &siteMoveJournal{Version: siteTasksJournalVersion, SiteID: target.SiteID, StartedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks: &siteTasksJournal{Account: username, UID: target.UID, Crontab: crontab, Step: siteTasksStepPlanned}}
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
		if running, backErr := putBackSiteTasks(target.SiteID, journal.Tasks, siteTaskSystemctl); backErr != nil {
			out.Outcome = transport.SiteTasksUnknown
			out.Error = fmt.Sprintf("the tasks could not be moved (%s): %v; putting them back did not complete (%v). Nothing is lost: the record, the task file and the timers are kept; Try again on the site's card, or the Agent's next start, finishes or puts it back", step, cause, backErr)
			out.Recovery = keptTasksRecovery(recordSiteTasksOutcome(target.SiteID, journal.Tasks, transport.SiteTasksUnknown, true, running))
			return out
		}
		removeSiteMoveRecord()
		return out
	}
	// killed answers the test seam at a point; a killed move leaves its journal.
	killed := func(point string) bool { return siteTasksMoveKilled(point) }
	unknown := transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}

	result, err := writeSiteTaskSource(target, crontab, transport.SiteFileTriggerCreate)
	if err == nil && result.Outcome != transport.SiteFileOutcomeWritten && result.Outcome != transport.SiteFileOutcomeRecreated {
		err = fmt.Errorf("%s was not written (%s %s)", sitecron.SourcePath(target.SiteID), result.Outcome, result.Reason)
	}
	if err != nil {
		return fail("task file", err)
	}
	if killed("act:" + siteTasksStepSourceWritten) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepSourceWritten); stepErr != nil {
		return fail("record", stepErr)
	}
	if killed(siteTasksStepSourceWritten) {
		return unknown
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
	if killed("act:" + siteTasksStepUnitsWritten) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepUnitsWritten); stepErr != nil {
		return fail("record", stepErr)
	}
	if killed(siteTasksStepUnitsWritten) {
		return unknown
	}
	// The account leaves cron first: from here on neither the site's code nor
	// anyone through `crontab` can give it a crontab (Stage 1 F1).
	refreshCronAllow()
	if !cronRefusesAccount(username) {
		return fail("cron.allow", fmt.Errorf("%s does not keep %s out", cronAllowPath, username))
	}
	// A crontab process that passed its access check before cron.allow closed
	// could still install a file: when they cannot all be found and ended,
	// the move fails here (second fix round) and cron opens again; nothing
	// was removed.
	if n, err := siteTaskEndCrontabs(target.UID); err != nil {
		out := fail("cron_closed", fmt.Errorf("the crontab processes of %s could not be listed, so one started before cron closed could still install a crontab: %w", username, err))
		// Fix round 4 (N2): a process whose program could not be identified
		// is worded by its own code, as before anything changed (the move
		// was put back; not when putting back did not complete).
		if out.Outcome == transport.SiteTasksPutBack && errors.Is(err, errCrontabProcessUnknown) {
			out.Refusal = transport.SiteTasksRefusalProcesses
		}
		return out
	} else if n > 0 {
		log.Printf("site tasks: %d crontab process(es) of %s started before cron.allow closed were ended", n, username)
	}
	if killed("act:" + siteTasksStepCronClosed) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepCronClosed); stepErr != nil {
		return fail("record", stepErr)
	}
	if killed(siteTasksStepCronClosed) {
		return unknown
	}
	// The crontab must still be the one moved: a change meanwhile (the site's
	// own code before cron closed, the owner) is never lost. Then its spool
	// file goes (root may; `crontab -u` is refused by now) and is checked gone.
	files, err := accountSpoolFiles(username)
	if err != nil {
		return fail("crontab", err)
	}
	switch {
	case crontab == "" && len(files) == 0:
	case crontab != "" && len(files) == 1 && spoolHolds(files[0], crontab):
	default:
		out := fail("crontab", errors.New("the crontab changed while it was being moved"))
		out.Refusal = transport.SiteTasksRefusalCrontab
		return out
	}
	for _, f := range files {
		if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fail("crontab", err)
		}
	}
	if left, err := accountSpoolFiles(username); err != nil || len(left) > 0 {
		if err == nil {
			err = errors.New("a crontab is still there")
		}
		return fail("crontab", err)
	}
	if killed("act:" + siteTasksStepCrontabEmptied) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepCrontabEmptied); stepErr != nil {
		log.Printf("site tasks: the move's record: %v", stepErr)
	}
	if killed(siteTasksStepCrontabEmptied) {
		return unknown
	}
	noteCronClosed(username, time.Now())
	if err := enableSiteTaskTimers(target.SiteID, journal.Tasks, siteTaskSystemctl); err != nil {
		return fail("enable", err)
	}
	if killed("act:" + siteTasksStepEnabled) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepEnabled); stepErr != nil {
		log.Printf("site tasks: the move's record: %v", stepErr)
	}
	if killed(siteTasksStepEnabled) {
		return unknown
	}
	removeSiteMoveRecord()
	forgetSiteMoveOutcome(target.SiteID)
	resp.Outcome = transport.SiteTasksMoved
	// Never "moved" while a crontab exists for the account.
	if back, _ := accountSpoolFiles(username); len(back) > 0 {
		resp.Outcome = transport.SiteTasksCrontabBack
		resp.Error = "the tasks run inside the limits now, but a crontab exists again for " + username + " (" + back[0].Path + ") and runs outside them; it was never removed by the move, so the server owner removes it or moves its lines into Scheduled tasks"
	}
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

// siteTaskCall is one systemctl call of a tasks move, an undo or their
// recovery (siteTaskSystemctl, or a bounded one: boundedSiteTaskCall).
type siteTaskCall func(args ...string) ([]byte, error)

// errSiteTaskBoundOver: the recovery's one total bound ran out before this
// call (nothing was sent), or while it was answering (what it did is not
// known; systemd keeps any job it queued).
var errSiteTaskBoundOver = errors.New("the time for systemd ran out")

// boundedSiteTaskCall is siteTaskSystemctl within one total bound (alpha.150:
// a kept record's recovery shares the bound of main's move record - at the
// Agent's start siteMoveStartBound, at Try again or a move's attempt the
// press's one deadline). Once the bound is over nothing further is sent; a
// call still answering at the bound is left to systemd and counts as not
// known.
func boundedSiteTaskCall(deadline time.Time) siteTaskCall {
	return func(args ...string) ([]byte, error) {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, errSiteTaskBoundOver
		}
		type answer struct {
			out []byte
			err error
		}
		answered := make(chan answer, 1)
		// The call is taken now: one still answering past the bound must not
		// read the package's seam later (as main's bounded restart does).
		ctl := siteTaskSystemctl
		go func() {
			out, err := ctl(args...)
			answered <- answer{out, err}
		}()
		timer := time.NewTimer(left)
		defer timer.Stop()
		select {
		case a := <-answered:
			return a.out, a.err
		case <-timer.C:
			return nil, errSiteTaskBoundOver
		}
	}
}

// enableSiteTaskTimers enables (and starts) the journal's timers, only while
// the site's limits file is in place and loaded: one reading of the slice,
// one call for the timers that start now and one for the @reboot ones (only
// enabled), each through the given call, so that a recovery stays within
// its few calls and its one bound (alpha.150: the reading too).
func enableSiteTaskTimers(siteID int, j *siteTasksJournal, call siteTaskCall) error {
	if len(j.Timers) == 0 {
		return nil
	}
	path, err := siteSlicePathOf(siteID)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || !siteSliceLoadedVia(call, siteID, path) {
		return fmt.Errorf("%s: %s is not in place", transport.SiteTasksLimitsMissing, path)
	}
	now, atBoot := []string{"enable", "--now"}, []string{"enable"}
	for _, timer := range j.Timers {
		if j.AtBoot[timer] {
			atBoot = append(atBoot, timer)
		} else {
			now = append(now, timer)
		}
	}
	for _, args := range [][]string{now, atBoot} {
		if len(args) == 2 && args[1] == "--now" || len(args) == 1 {
			continue
		}
		if out, err := call(args...); err != nil {
			return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// siteSliceLoadedVia is siteSliceLoaded through a tasks recovery's call
// (bounded; an answer that does not come counts as "not loaded", so no
// timer is switched on).
func siteSliceLoadedVia(call siteTaskCall, siteID int, path string) bool {
	out, err := call("show", sitesandbox.SliceName(siteID), "--property=FragmentPath", "--value")
	return err == nil && strings.TrimSpace(string(out)) == path
}

// disableSiteTaskTimers disables and stops the journal's timers in one call
// (a running task is not stopped: only its timer).
func disableSiteTaskTimers(j *siteTasksJournal, call siteTaskCall) error {
	if len(j.Timers) == 0 {
		return nil
	}
	args := append([]string{"disable", "--now"}, j.Timers...)
	if out, err := call(args...); err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// crontabBackFromJournal makes sure the journal's crontab is in place: left
// as it is when the spool holds a crontab (the recorded one, or one changed
// by someone else, which is never overwritten); written back and checked
// when it is gone (cron.allow lets the account in first). back: a crontab is
// in place now (or none was there to begin with). mayWrite: the timers are
// confirmed off; without it a crontab that is gone is not written back
// (alpha.150: beside a timer that may still be on, every task would run
// twice), and back is false.
func crontabBackFromJournal(siteID int, j *siteTasksJournal, mayWrite bool) (bool, error) {
	files, err := accountSpoolFiles(j.Account)
	if err != nil {
		return false, err
	}
	if len(files) > 0 {
		if !spoolHoldsRecorded(files, j.Crontab) {
			log.Printf("site tasks: the crontab of %s changed during the move; it is left as it is", j.Account)
		}
		return true, nil
	}
	if j.Crontab == "" {
		return true, nil
	}
	if _, _, _, _, err := siteTaskLookup(j.Account); err != nil {
		// The account is gone (its site was deleted): there is nobody to give
		// the crontab back to.
		log.Printf("site tasks: %s no longer exists; its crontab is not written back", j.Account)
		return true, nil
	}
	if !mayWrite {
		return false, errors.New("the timers could not be confirmed off, so the crontab is not written back beside them (a task would run twice)")
	}
	if err := allowCronFor(j.Account, siteID); err != nil {
		return false, err
	}
	if err := setCrontab(j.Account, j.Crontab); err != nil {
		return false, fmt.Errorf("the crontab could not be written back: %w", err)
	}
	now, err := readCrontab(j.Account)
	if err != nil {
		return false, fmt.Errorf("the crontab written back could not be read: %w", err)
	}
	if now != j.Crontab {
		return false, errors.New("the crontab written back is not the recorded one")
	}
	return true, nil
}

// putBackSiteTasks undoes an unfinished move: the timers off, the crontab
// back (written again only when it is gone) and checked, and only then
// every timer and file of the site's tasks removed. A crontab that is not
// confirmed back keeps the task file and the units (switched on again where
// the site's limits allow, so the tasks keep running) and returns an error:
// the caller keeps the record. Its systemd calls are at most four (the
// timers off, the limits read and the two enables; or the timers off, the
// units' timers off and one daemon-reload).
//
// alpha.150: the crontab is written back only once the call that switches
// the record's timers off has answered success. A batched enable that
// failed half-way can have left some of them on; when switching them off is
// not confirmed, the crontab stays gone and the timers are switched on again
// (the record knows every one of them), so no task runs from both. running
// says how the tasks run when an error keeps the record
// (transport.SiteTasksRun*; "" not known).
func putBackSiteTasks(siteID int, j *siteTasksJournal, call siteTaskCall) (running string, err error) {
	disableErr := disableSiteTaskTimers(j, call)
	back, backErr := crontabBackFromJournal(siteID, j, disableErr == nil)
	if !back {
		// alpha.150 fix round (F2): a crontab written whose reading back
		// failed or differed is never given timers beside it. The spool is
		// read again: the recorded crontab there is back; anything else
		// there, or a spool that cannot be read, keeps the timers off.
		files, spoolErr := accountSpoolFiles(j.Account)
		back = spoolErr == nil && spoolHoldsRecorded(files, j.Crontab)
		if !back && (spoolErr != nil || len(files) > 0) {
			reason := "the crontab is not confirmed back"
			if backErr != nil {
				reason = backErr.Error()
			}
			refreshCronAllow() // the task file stays: the account stays out
			if disableErr == nil && spoolErr == nil {
				running = transport.SiteTasksRunCron
			}
			return running, errors.New(reason + "; a crontab is there (or the spool cannot be read), so the timers are not switched on again beside it")
		}
	}
	if !back {
		reason := "the crontab is not back"
		if backErr != nil {
			reason = backErr.Error()
		}
		if disableErr != nil {
			reason += " (" + disableErr.Error() + ")"
		}
		refreshCronAllow() // the task file stays: the account stays out
		if enableErr := enableSiteTaskTimers(siteID, j, call); enableErr != nil {
			reason += "; the timers could not be switched on again either (" + enableErr.Error() + ")"
			if disableErr == nil && siteTasksSpoolEmpty(j.Account) {
				running = transport.SiteTasksRunNone
			}
		} else if len(j.Timers) > 0 {
			running = transport.SiteTasksRunTimers
		}
		return running, errors.New(reason)
	}
	var failures []string
	if disableErr != nil {
		failures = append(failures, disableErr.Error())
	}
	if _, err := removeSiteTasksWith(siteID, false, call); err != nil {
		failures = append(failures, err.Error())
	}
	refreshCronAllow()
	if len(failures) > 0 {
		// The crontab is back; a timer that could not be removed is disabled
		// (or named here): no task runs twice.
		if disableErr == nil {
			running = transport.SiteTasksRunCron
		}
		return running, errors.New(strings.Join(failures, "; "))
	}
	return "", nil
}

// siteTasksSpoolEmpty: the account has no crontab now (read; a spool that
// cannot be read is not empty).
func siteTasksSpoolEmpty(account string) bool {
	files, err := accountSpoolFiles(account)
	return err == nil && len(files) == 0
}

// recordSiteTasksOutcome keeps what happened to a site's tasks move or undo
// in the move's one outcome record (alpha.150: the record the programs'
// move keeps, siteMoveOutcomesPath; kept while the move's record stays, so
// the site's card, Problems now and Try again are the programs' ones).
func recordSiteTasksOutcome(siteID int, j *siteTasksJournal, outcome string, kept bool, running string) transport.SiteSliceMoveRecovery {
	record := transport.SiteSliceMoveRecovery{At: time.Now().UTC().Format(time.RFC3339), Units: map[string]string{},
		Kept: kept, Tasks: outcome, TasksStep: j.Step, TasksSite: siteID, TasksAccount: j.Account}
	if kept {
		record.TasksRunning = running
	}
	changeSiteMoveOutcomes(func(all map[string]transport.SiteSliceMoveRecovery) bool {
		all[strconv.Itoa(siteID)] = record
		return true
	})
	return record
}

// recoverSiteTasksRecord resolves a kept record of a site's tasks move or
// undo (the tables at the top) from main's one move record (alpha.150): at
// the Agent's start (atStart, within siteMoveStartBound), at the
// administrator's Try again and at a move's attempt on the record's own site
// (within the press's one deadline). Its systemd calls are at most four,
// each within the bound, and none waits for a task or a timer: the timers
// are switched on or off, never a task run. A step the start cannot finish
// within its calls (a move whose timers could not be switched on, which a
// put-back would then need) keeps the record for Try again. What it did is
// kept in the outcome record; the record goes only when it is settled.
func recoverSiteTasksRecord(journal *siteMoveJournal, deadline time.Time, atStart bool) *siteMoveRecovered {
	j := journal.Tasks
	call := boundedSiteTaskCall(deadline)
	cronMu.Lock()
	defer cronMu.Unlock()
	outcome, done, running := "", true, ""
	putBack := func() {
		outcome = transport.SiteTasksPutBack
		var err error
		if running, err = putBackSiteTasks(journal.SiteID, j, call); err != nil {
			log.Printf("site tasks: the interrupted move of site %d could not be put back: %v", journal.SiteID, err)
			outcome, done = transport.SiteTasksUnknown, false
		}
	}
	switch {
	case j.Undo:
		outcome, done, running = recoverSiteTasksUndo(journal.SiteID, j, call)
	case j.Step == siteTasksStepPlanned:
		// Stopped between writing the task file and recording it: what was
		// written goes (the crontab was never touched).
		outcome = transport.SiteMoveNotTried
		if siteTasksMoved(journal.SiteID) {
			putBack()
		}
	case j.Step == siteTasksStepSourceWritten || j.Step == siteTasksStepUnitsWritten || j.Step == siteTasksStepCronClosed:
		putBack()
	case j.Step == siteTasksStepCrontabEmptied || j.Step == siteTasksStepEnabled:
		// alpha.150 fix round (F1): the account's spool is read first (a file
		// read, no systemd call). Timers are never switched on beside a
		// crontab, nor while the spool cannot be read: the server owner can
		// have put the tasks back into cron by hand (the card's steps).
		files, spoolErr := accountSpoolFiles(j.Account)
		switch {
		case spoolErr == nil && len(files) == 0 && j.Step == siteTasksStepEnabled:
			outcome = transport.SiteTasksMoved
			refreshCronAllow()
		case spoolErr == nil && len(files) == 0:
			var now bool
			if outcome, done, running, now = recoverSiteTasksEmptied(journal.SiteID, j, call, atStart); now {
				putBack()
			}
		case spoolErr == nil && !atStart && spoolHoldsRecorded(files, j.Crontab):
			// The crontab is back as recorded (put back by hand, the card's
			// steps): Try again finishes the put-back - the timers off, their
			// files and the task file removed, cron open for the account.
			log.Printf("site tasks: the crontab of %s is back (site %d); the move is put back", j.Account, journal.SiteID)
			putBack()
		default:
			// A crontab exists (at the start, or one that is not the recorded
			// one), or the spool cannot be read: the timers are switched off
			// in one call and the record stays; the card says how the tasks
			// run and what the administrator does.
			outcome, done, running = transport.SiteTasksUnknown, false, ""
			// alpha.150 fix round 4 (N1): a record at enabled is saved as
			// crontab_emptied before its timers are switched off, so that a
			// later pass that finds the spool empty switches them on again
			// (recoverSiteTasksEmptied) instead of answering "moved" with
			// every timer off. When the record cannot be written the timers
			// are still switched off (a task never runs twice); the step stays
			// changed in memory for this pass, and the failure is logged as the
			// move's own later steps log theirs.
			if j.Step == siteTasksStepEnabled {
				if stepErr := journal.tasksStep(siteTasksStepCrontabEmptied); stepErr != nil {
					log.Printf("site tasks: the move's record of site %d could not be written before its timers are switched off: %v", journal.SiteID, stepErr)
				}
			}
			offErr := disableSiteTaskTimers(j, call)
			if offErr != nil {
				log.Printf("site tasks: the timers of site %d could not be switched off beside its crontab: %v", journal.SiteID, offErr)
			}
			if spoolErr == nil && offErr == nil {
				running = transport.SiteTasksRunCron
			}
			log.Printf("site tasks: site %d: a crontab exists for %s, or its spool cannot be read (%v); its timers are not switched on and the record stays", journal.SiteID, j.Account, spoolErr)
		}
	}
	record := recordSiteTasksOutcome(journal.SiteID, j, outcome, !done, running)
	if !done {
		log.Printf("site tasks: the interrupted tasks move of site %d (step %s) is not settled; its record stays (%s) and no site is moved until it is (Try again on the site's card, or the Agent's next start)", journal.SiteID, j.Step, siteMoveJournalPath)
		return &siteMoveRecovered{siteID: journal.SiteID, record: record}
	}
	log.Printf("site tasks: the interrupted tasks move of site %d was resolved: %s", journal.SiteID, outcome)
	if err := os.Remove(siteMoveJournalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("site tasks: remove the move's record: %v", err)
	}
	return &siteMoveRecovered{siteID: journal.SiteID, record: record}
}

// recoverSiteTasksEmptied finishes a move stopped at crontab_emptied whose
// account has no crontab (read just before): the timers switched on. When
// that fails, at the Agent's start the record stays (a put-back would take
// more calls than the start makes) and the timers are switched off again in
// one call - always, also when the site's limits look missing (alpha.150 fix
// round, F3: an enable sent before the Agent stopped can have switched some
// on) - so that the state is known: no task runs until Try again. At Try
// again it is put back (putBackNow: the caller does that).
func recoverSiteTasksEmptied(siteID int, j *siteTasksJournal, call siteTaskCall, atStart bool) (outcome string, done bool, running string, putBackNow bool) {
	err := enableSiteTaskTimers(siteID, j, call)
	if err == nil {
		noteCronClosed(j.Account, time.Now())
		return transport.SiteTasksMoved, true, "", false
	}
	if !atStart {
		log.Printf("site tasks: the interrupted move of site %d could not be finished (%v); it is put back", siteID, err)
		return "", true, "", true
	}
	log.Printf("site tasks: the interrupted move of site %d could not be finished at the Agent's start (%v); its record stays for Try again", siteID, err)
	running = transport.SiteTasksRunNone
	if offErr := disableSiteTaskTimers(j, call); offErr != nil {
		log.Printf("site tasks: the timers of site %d could not be switched off again: %v", siteID, offErr)
		running = ""
	}
	if !siteTasksSpoolEmpty(j.Account) {
		running = ""
	}
	return transport.SiteTasksUnknown, false, running, false
}

// spoolHoldsRecorded: the account's one spool file holds the record's
// crontab - as `crontab -l` printed it before the move, or as the task file
// holds it (the card's way back by hand installs
// /etc/celikpanel-cron/<id>.crontab whole: CelikPanel's header and intro
// lines are comments above the same crontab).
func spoolHoldsRecorded(files []spoolFile, crontab string) bool {
	if len(files) != 1 {
		return false
	}
	if spoolHolds(files[0], crontab) {
		return true
	}
	for _, view := range []string{crontabListView(files[0].Raw), files[0].Raw} {
		if body, _ := splitSiteTaskSource([]byte(view)); body == crontab {
			return true
		}
	}
	return false
}

// recoverSiteTasksUndo resolves an interrupted undo: never the crontab and
// the timers both on, never both gone. running: how the tasks run when the
// record stays (transport.SiteTasksRun*; "" not known).
func recoverSiteTasksUndo(siteID int, j *siteTasksJournal, call siteTaskCall) (string, bool, string) {
	finish := func() (string, bool, string) {
		if _, err := removeSiteTasksWith(siteID, false, call); err != nil {
			log.Printf("site tasks: the interrupted undo of site %d: %v", siteID, err)
			refreshCronAllow()
			return transport.SiteTasksUnknown, false, transport.SiteTasksRunCron
		}
		refreshCronAllow()
		return transport.SiteTasksUndone, true, ""
	}
	rollback := func() (string, bool, string) {
		refreshCronAllow() // the task file is there: the account is kept out again
		if err := enableSiteTaskTimers(siteID, j, call); err != nil {
			log.Printf("site tasks: the interrupted undo of site %d could not switch its timers on again: %v", siteID, err)
			// Before timers_disabled the timers were never switched off.
			running := ""
			if j.Step != siteTasksStepUndoPlanned && siteTasksSpoolEmpty(j.Account) {
				running = transport.SiteTasksRunNone
			}
			return transport.SiteTasksUnknown, false, running
		}
		return transport.SiteMoveNotTried, true, ""
	}
	switch j.Step {
	case siteTasksStepCrontabWritten:
		return finish()
	case siteTasksStepCronOpened:
		files, err := accountSpoolFiles(j.Account)
		if err != nil {
			return transport.SiteTasksUnknown, false, ""
		}
		if (j.Crontab == "" && len(files) == 0) || (len(files) == 1 && spoolHolds(files[0], j.Crontab)) {
			return finish()
		}
		// No crontab yet, or one the site's own code made while cron was open
		// (it is never removed; it is reported as running outside the limits).
		return rollback()
	default: // undo_planned, timers_disabled
		return rollback()
	}
}

// UndoSiteTasksMoveV1 puts a moved site's tasks back into its crontab.
func (a *Agent) UndoSiteTasksMoveV1(req *transport.SiteTasksMoveRequest, resp *transport.SiteTasksMoveResponse) error {
	*resp = transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksRefused}
	defer logSiteTasksAnswer("undo", req, resp)
	if req == nil {
		resp.Error = "a request is required"
		return nil
	}
	if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "putting a site's tasks back into cron"); err != nil {
		resp.Refusal, resp.Error = "build", err.Error()
		return nil
	}
	target, username, err := siteTasksTenantTarget(req)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	// alpha.150: the undo is claimed as a move is (claimSiteMove: never while
	// the panel's update is in progress, and the update waits for it).
	refusal, release := claimSiteMove()
	if refusal != "" {
		resp.Refusal, resp.Error = refusal, siteMoveRefusalText(refusal)
		return nil
	}
	defer release()
	deadline := time.Now().Add(siteMoveBudget)
	siteMu := siteMutationMutex(target.SiteID)
	if !siteMu.TryLock() {
		resp.Refusal, resp.Error = "site_busy", "the site is being changed right now; nothing was changed. Read its page again in a minute"
		return nil
	}
	defer siteMu.Unlock()
	if why := siteTasksKeptRefusal(target.SiteID, deadline); why != "" {
		resp.Refusal, resp.Error = transport.SiteMoveRefusalKept, why
		return nil
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	*resp = undoSiteTasksLocked(target, username)
	return nil
}

// undoSiteTimers reads the site's timers for the undo: the ones that are on
// now (enabled), the @reboot ones among them, and (alpha.150 fix round, F6,
// D-022) the crontab's job lines whose timer is not on - switched off on the
// server (`systemctl disable`), not one CelikPanel wrote and could not switch
// on yet (its "to enable" list) - by line number with the timer's name. err:
// the state of a timer of the site could not be read; the undo is then
// refused, as it cannot tell which tasks the server owner switched off.
func undoSiteTimers(siteID int, crontab string) (on []string, atBoot map[string]bool, off map[int]string, err error) {
	units, err := siteTaskUnits(siteID)
	if err != nil {
		return nil, nil, nil, err
	}
	var timers []string
	for n, u := range units {
		if u.Timer {
			timers = append(timers, sitecron.UnitBase(siteID, n)+".timer")
		}
	}
	sort.Strings(timers)
	props := siteTaskShow(timers, siteTaskShowTimerProps)
	for _, timer := range timers {
		if props[timer]["UnitFileState"] == "" {
			return nil, nil, nil, fmt.Errorf("the state of %s could not be read", timer)
		}
	}
	pending := readSiteTimersToEnable()[strconv.Itoa(siteID)]
	atBoot, off = map[string]bool{}, map[int]string{}
	jobs := sitecron.Parse(crontab).Jobs()
	keys := siteTaskKeys(jobs)
	byKey := map[string]int{}
	for n, u := range units {
		if u.TaskKey != "" {
			byKey[u.TaskKey] = n
		}
	}
	reboot := map[string]bool{}
	for i, job := range jobs {
		if job.Spec.Reboot {
			reboot[keys[i]] = true
		}
		n, ok := byKey[keys[i]]
		if !ok || !units[n].Timer {
			continue
		}
		timer := sitecron.UnitBase(siteID, n) + ".timer"
		if _, waiting := pending[timer]; !waiting && props[timer]["UnitFileState"] != "enabled" {
			off[job.Line] = timer
		}
	}
	for _, timer := range timers {
		if props[timer]["UnitFileState"] != "enabled" {
			continue
		}
		on = append(on, timer)
		if site, n, _, ok := sitecron.ParseUnit(timer); ok && site == siteID {
			if u := units[n]; u != nil && reboot[u.TaskKey] {
				atBoot[timer] = true
			}
		}
	}
	return on, atBoot, off, nil
}

// crontabWithTasksOff is the crontab the undo writes: the task file's
// crontab byte for byte, except that each job line whose timer was switched
// off on the server comes back as CelikPanel's paused line ("# DISABLED: "),
// under the one fixed note that says why (F6, D-022: the undo never starts
// again what the server owner switched off). Fix round 4 (N7): the note is
// cronPausedNote (cron_rpc.go), which the Scheduled tasks screen never shows
// as a description and every save there leaves out; it is not written twice
// above one line. paused names those lines (the answer and the card name the
// timer).
func crontabWithTasksOff(crontab string, off map[int]string) (string, []transport.SiteTaskLine) {
	if len(off) == 0 {
		return crontab, nil
	}
	var b strings.Builder
	var paused []transport.SiteTaskLine
	previous := ""
	for i, line := range strings.SplitAfter(crontab, "\n") {
		timer, ok := off[i+1]
		if !ok {
			b.WriteString(line)
			previous = line
			continue
		}
		if previous != cronPausedNote+"\n" {
			b.WriteString(cronPausedNote + "\n")
		}
		previous = line
		b.WriteString("# DISABLED: " + line)
		paused = append(paused, transport.SiteTaskLine{Line: i + 1, Text: strings.TrimSuffix(line, "\n"), Detail: timer})
	}
	return b.String(), paused
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
		return refused(transport.SiteTasksRefusalSource, "the task file "+sitecron.SourcePath(target.SiteID)+" is gone while timers of the site remain; the administrator rebuilds it on the Scheduled tasks screen first; nothing was changed")
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
	if files, err := accountSpoolFiles(username); err != nil {
		return refused("", "the account's crontab could not be read ("+err.Error()+"); nothing was changed")
	} else if len(files) > 0 {
		return refused(transport.SiteTasksRefusalCrontab, "the account has a crontab again ("+files[0].Path+", running outside the site's limits); it is never overwritten, so nothing was changed. The server owner merges it into Scheduled tasks or removes it first")
	}
	// Byte for byte: the task file's crontab, nothing added - except that a
	// task whose timer the server owner switched off comes back paused
	// (alpha.150 fix round, F6); a timer state that cannot be read refuses.
	timers, atBoot, off, timersErr := undoSiteTimers(target.SiteID, source.Crontab)
	if timersErr != nil {
		return refused(transport.SiteTasksRefusalTimers, "the state of the site's timers could not be read ("+timersErr.Error()+"), so it is not known which tasks the server owner switched off; nothing was changed")
	}
	written, paused := crontabWithTasksOff(source.Crontab, off)
	journal := &siteMoveJournal{Version: siteTasksJournalVersion, SiteID: target.SiteID, StartedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks: &siteTasksJournal{Undo: true, Account: username, UID: target.UID, Crontab: written, Step: siteTasksStepUndoPlanned, Timers: timers, AtBoot: atBoot}}
	if err := journal.save(); err != nil {
		return refused("", "the record could not be written in the Agent's folder ("+err.Error()+"); nothing was changed")
	}
	unknown := transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown}
	if siteTasksMoveKilled(siteTasksStepUndoPlanned) {
		return unknown
	}
	// Back to where it started, at any failure before the crontab is in place.
	rollback := func(text string) transport.SiteTasksMoveResponse {
		refreshCronAllow()
		if err := enableSiteTaskTimers(target.SiteID, journal.Tasks, siteTaskSystemctl); err != nil {
			// The timers are off only once their switch-off was recorded.
			running := ""
			if journal.Tasks.Step != siteTasksStepUndoPlanned && siteTasksSpoolEmpty(username) {
				running = transport.SiteTasksRunNone
			}
			return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown,
				Recovery: keptTasksRecovery(recordSiteTasksOutcome(target.SiteID, journal.Tasks, transport.SiteTasksUnknown, true, running)),
				Error:    text + "; the timers could not be switched on again (" + err.Error() + "). The record is kept: Try again on the site's card, or the Agent's next start, finishes it"}
		}
		removeSiteMoveRecord()
		return refused("", text+"; the tasks run as timers, as before")
	}
	// 1. The timers off (a running task is not stopped).
	if err := disableSiteTaskTimers(journal.Tasks, siteTaskSystemctl); err != nil {
		return rollback("the timers could not be switched off (" + err.Error() + ")")
	}
	if siteTasksMoveKilled("act:" + siteTasksStepTimersDisabled) {
		return unknown
	}
	if err := journal.tasksStep(siteTasksStepTimersDisabled); err != nil {
		return rollback("the record could not be written (" + err.Error() + ")")
	}
	if siteTasksMoveKilled(siteTasksStepTimersDisabled) {
		return unknown
	}
	// 2. The account back into cron.
	if err := allowCronFor(username, target.SiteID); err != nil {
		return rollback(err.Error())
	}
	if siteTasksMoveKilled("act:" + siteTasksStepCronOpened) {
		return unknown
	}
	if err := journal.tasksStep(siteTasksStepCronOpened); err != nil {
		return rollback("the record could not be written (" + err.Error() + ")")
	}
	if siteTasksMoveKilled(siteTasksStepCronOpened) {
		return unknown
	}
	// 3. The crontab, from the journal's bytes, checked.
	if written != "" {
		if err := setSiteCrontab(username, "", written, cronEffectiveLines(written)); err != nil {
			return rollback("the crontab could not be written (" + err.Error() + ")")
		}
		now, err := readCrontab(username)
		if err != nil || now != written {
			if files, _ := accountSpoolFiles(username); len(files) == 1 && spoolHolds(files[0], written) {
				err = nil // the spool holds it; crontab -l failed for another reason
			} else {
				// What was written is not confirmed: back to timers. Only a
				// spool file that holds exactly what the undo wrote is taken
				// away again (second fix round); any other file is left as it
				// is (cron was open: the site's own code can have made it) and
				// named, and it is reported as running outside the limits.
				var left []string
				for _, f := range files {
					if spoolHolds(f, written) {
						_ = os.Remove(f.Path)
					} else {
						left = append(left, f.Path)
					}
				}
				if err == nil {
					err = errors.New("it reads back differently")
				}
				text := "the crontab written could not be confirmed (" + err.Error() + ")"
				if len(left) > 0 {
					text += "; " + strings.Join(left, ", ") + " holds another crontab and was left as it is: it runs outside the site's limits until the server owner removes it"
				}
				return rollback(text)
			}
		}
	}
	if siteTasksMoveKilled("act:" + siteTasksStepCrontabWritten) {
		return unknown
	}
	if stepErr := journal.tasksStep(siteTasksStepCrontabWritten); stepErr != nil {
		log.Printf("site tasks: the undo's record: %v", stepErr)
	}
	if siteTasksMoveKilled(siteTasksStepCrontabWritten) {
		return unknown
	}
	// 4. The timers and files go.
	if _, err := removeSiteTasksLocked(target.SiteID, false); err != nil {
		refreshCronAllow()
		return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUnknown,
			Recovery: keptTasksRecovery(recordSiteTasksOutcome(target.SiteID, journal.Tasks, transport.SiteTasksUnknown, true, transport.SiteTasksRunCron)),
			Error:    "the crontab is back, but not every timer could be removed (" + err.Error() + "); they are switched off, so no task runs twice. Try again on the site's card, or the Agent's next start, removes them"}
	}
	removeSiteMoveRecord()
	forgetSiteMoveOutcome(target.SiteID)
	forgetCronClosed(map[string]bool{username: true})
	refreshCronAllow()
	return transport.SiteTasksMoveResponse{Outcome: transport.SiteTasksUndone, Paused: paused}
}

// SiteTasksMovedV1 lists the sites whose tasks run as timers, the crontabs
// that came back for their accounts (and whether the site's own code can
// have made them) and what the Agent did with an interrupted tasks move
// (read-only).
func (a *Agent) SiteTasksMovedV1(req *transport.SiteTasksMovedRequest, resp *transport.SiteTasksMovedResponse) error {
	*resp = transport.SiteTasksMovedResponse{Moved: []int{}, MailSubmission: recordedMailSubmission()}
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
					resp.Reappeared, resp.ReappearedBySite = map[string]int{}, map[string]bool{}
				}
				resp.Reappeared[strconv.Itoa(id)] = n
				files, _ := accountSpoolFiles(account)
				if reappearedBySite(account, files) {
					resp.ReappearedBySite[strconv.Itoa(id)] = true
				}
			}
		}
	}
	// What the Agent did with an interrupted tasks move (alpha.150: from the
	// move's one outcome record; a kept one is also on the site's limits card
	// with Try again, and in Problems now).
	for key, record := range readSiteMoveOutcomes() {
		id, _ := strconv.Atoi(key)
		if record.Tasks == "" || (len(want) > 0 && !want[id]) {
			continue
		}
		if resp.Recovery == nil {
			resp.Recovery = map[string]string{}
		}
		resp.Recovery[key] = record.At + " " + record.Tasks
	}
	return nil
}

// RebuildSiteTaskSourceV1 is the administrator's one action for a moved
// site whose task file is missing while its timers remain: the file is
// rebuilt from the copy of what CelikPanel last rendered (kept in the
// Agent's private folder). Preview shows what it would contain and changes
// nothing.
func (a *Agent) RebuildSiteTaskSourceV1(req *transport.SiteTasksMoveRequest, resp *transport.SiteTaskSourceResponse) error {
	*resp = transport.SiteTaskSourceResponse{Outcome: transport.SiteTasksRefused}
	if req != nil && !req.Preview {
		if err := requireExpectedBuildCommit(req.ExpectedBuildCommit, "rebuilding a site's task file"); err != nil {
			resp.Error = err.Error()
			return nil
		}
	}
	target, username, err := siteTasksTenantTarget(req)
	if err != nil {
		resp.Error = err.Error()
		return nil
	}
	// alpha.150: rebuilding writes the site's units and reloads systemd, so
	// it takes the move's claim (claimSiteMove): refused while the panel's
	// update is in progress or a move runs, with main's codes and words, and
	// the update is refused while it runs. The preview changes nothing.
	if !req.Preview {
		refusal, release := claimSiteMove()
		if refusal != "" {
			resp.Refusal, resp.Error = refusal, siteMoveRefusalText(refusal)
			return nil
		}
		defer release()
	}
	// alpha.150 fix round (F7): never while a move record of this site is
	// kept (its Try again settles the site's tasks first), also in the preview;
	// a record that cannot be read counts as this site's.
	if _, statErr := os.Stat(siteMoveJournalPath); statErr == nil {
		if journal, err := readSiteMoveJournal(); err != nil || journal.SiteID == target.SiteID {
			resp.Refusal = transport.SiteMoveRefusalKept
			resp.Error = "a move of this site into its limits was not settled; its card names it with Try again. Nothing was changed; the task file is rebuilt once that is settled"
			if err != nil {
				// Fix round 4 (N9): a record that cannot be read may be any
				// site's; the words do not say it is this one's.
				resp.Error = "the record of a move on this server into a site's limits could not be read (" + err.Error() + "); Problems now names it with Try again. Nothing was changed; the task file is rebuilt once that is settled"
			}
			return nil
		}
	}
	cronMu.Lock()
	defer cronMu.Unlock()
	source, err := readSiteTaskSource(target.SiteID)
	switch {
	case err != nil:
		resp.Error = "the task file could not be read (" + err.Error() + "); nothing was changed"
		return nil
	case source.Exists:
		resp.Error = "the task file is there; nothing to rebuild"
		return nil
	case !siteTaskUnitsExist(target.SiteID):
		resp.Error = "the site has no timers; nothing to rebuild"
		return nil
	}
	// Second fix round: while the account has a crontab, its tasks run from
	// it (an undo by hand, or a put-back that left a unit the owner changed);
	// rebuilding would switch the timers on beside it and run every task
	// twice. Refused, also in the preview, with who acts.
	if files, err := accountSpoolFiles(username); err != nil || len(files) > 0 {
		resp.Refusal = transport.SiteTaskSourceRefusalCrontab
		if err != nil {
			resp.Error = "the account's crontab could not be read (" + err.Error() + "), so it is not known whether its tasks run from it; nothing was changed"
			return nil
		}
		resp.LeftUnits = siteTaskUnitPaths(target.SiteID)
		resp.Error = "the account " + username + " has a crontab (" + files[0].Path + "), so the site's tasks run from it again; rebuilding the task file would switch the timers on beside it and run every task twice. Nothing was changed. The server owner removes the site's timer files: " +
			siteTimersOffCommand(target.SiteID) + " && sudo rm -f " + strings.Join(resp.LeftUnits, " ") + " && sudo systemctl daemon-reload; the tasks keep running from the crontab"
		return nil
	}
	path := siteTaskRenderedPath(target.SiteID)
	info, statErr := os.Stat(path)
	raw, readErr := os.ReadFile(path)
	if statErr != nil || readErr != nil || len(raw) > 1<<20 {
		resp.Error = "no copy of what CelikPanel last made the timers from is kept for this site (" + path + "), so the file cannot be rebuilt here; the server owner puts the file back from a backup, or switches the site's timers off (" + siteTimersOffCommand(target.SiteID) + ")"
		return nil
	}
	resp.Crontab = string(raw)
	resp.RenderedAt = info.ModTime().UTC().Format(time.RFC3339)
	if req.Preview {
		resp.Outcome = transport.SiteTasksPreview
		return nil
	}
	result, err := writeSiteTaskSource(target, resp.Crontab, transport.SiteFileTriggerCreate)
	if err != nil || (result.Outcome != transport.SiteFileOutcomeWritten && result.Outcome != transport.SiteFileOutcomeRecreated) {
		resp.Outcome = transport.SiteTasksUnknown
		resp.Error = fmt.Sprintf("the task file could not be written (%s %s %v); read the site's tasks again", result.Outcome, result.Reason, err)
		return nil
	}
	if _, err := applySiteTimers(target, resp.Crontab); err != nil {
		log.Printf("site tasks: the rebuilt task file of site %d: its timers: %v", target.SiteID, err)
	}
	resp.Outcome = transport.SiteTaskSourceRebuilt
	return nil
}

// siteTimersOffCommand is the server owner's command that switches every
// timer of a site off (alpha.150 fix round, F1): `systemctl disable` takes
// no pattern, so the site's timers are listed by name first; with none
// listed it fails, and a command joined after it with && does not run. Run
// for real on systemd 257 with throw-away units.
func siteTimersOffCommand(siteID int) string {
	return "sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-" + strconv.Itoa(siteID) + "-*.timer' | awk '{print $1}')"
}

// movedSiteTaskSites are the moved sites (a task file, or timers whose file
// is gone) and the account of each: the one its task file names or, when
// the file is gone (second fix round), the one its units run as (User=, as
// siteTaskOwnership reads them). unknown: moved sites whose account could be
// read from neither (CelikPanel's cron.allow is then not re-rendered: a
// moved site never counts as not moved).
func movedSiteTaskSites() ([]int, map[int]string) {
	ids, accounts, _ := movedSiteTaskSitesAll()
	return ids, accounts
}

func movedSiteTaskSitesAll() (ids []int, accounts map[int]string, unknown []int) {
	seen := map[int]bool{}
	accounts = map[int]string{}
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
	ids = make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if accounts[id] != "" {
			continue
		}
		if account := siteTaskUnitAccount(id); account != "" {
			accounts[id] = account
		} else {
			unknown = append(unknown, id)
		}
	}
	return ids, accounts, unknown
}

// siteTaskUnitPaths are the site's task unit files on the server.
func siteTaskUnitPaths(siteID int) []string {
	dir, err := sandboxUnitDir()
	if err != nil {
		return nil
	}
	units, _ := siteTaskUnits(siteID)
	var out []string
	for n, u := range units {
		base := filepath.Join(dir, sitecron.UnitBase(siteID, n))
		if u.Service {
			out = append(out, base+".service")
		}
		if u.Timer {
			out = append(out, base+".timer")
		}
	}
	sort.Strings(out)
	return out
}

// siteTaskUnitAccount is the account a site's task services run as (their
// User= lines), "" when none can be read or they name more than one.
func siteTaskUnitAccount(siteID int) string {
	dir, err := sandboxUnitDir()
	if err != nil {
		return ""
	}
	units, _ := siteTaskUnits(siteID)
	account := ""
	for n, u := range units {
		if !u.Service {
			continue
		}
		for _, line := range strings.Split(readSmallFile(filepath.Join(dir, sitecron.UnitBase(siteID, n)+".service")), "\n") {
			user, ok := strings.CutPrefix(line, "User=")
			if !ok {
				continue
			}
			user = strings.TrimSpace(user)
			if !cronUsernameRe.MatchString(user) || user == "root" || (account != "" && account != user) {
				return ""
			}
			account = user
		}
	}
	return account
}

// removeSiteTasksLocked removes every timer and file of a site's tasks
// (the caller holds cronMu). aside: files the server owner changed are
// moved aside (a deleted site) instead of kept.
func removeSiteTasksLocked(siteID int, aside bool) ([]string, error) {
	return removeSiteTasksWith(siteID, aside, siteTaskSystemctl)
}

// removeSiteTasksWith is removeSiteTasksLocked through the given call: its
// systemd calls are two at most (every timer of the site switched off in
// one call, then one daemon-reload; alpha.150, so that a recovery stays
// within its few calls).
func removeSiteTasksWith(siteID int, aside bool, call siteTaskCall) ([]string, error) {
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
	var timers []string
	for n := range units {
		if units[n].Timer {
			timers = append(timers, sitecron.UnitBase(siteID, n)+".timer")
		}
	}
	sort.Strings(timers)
	if len(timers) > 0 {
		// alpha.150 fix round (F7): a switch-off that failed is reported, not
		// only logged (a timer may still be on).
		if out, err := call(append([]string{"disable", "--now"}, timers...)...); err != nil {
			log.Printf("site tasks: %s: %v: %s", strings.Join(timers, " "), err, strings.TrimSpace(string(out)))
			failures = append(failures, fmt.Sprintf("systemctl disable --now %s: %v: %s", strings.Join(timers, " "), err, strings.TrimSpace(string(out))))
		}
	}
	for n := range units {
		base := sitecron.UnitBase(siteID, n)
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
	if left, _ := siteTaskUnits(siteID); len(left) == 0 {
		setSiteTimersToEnable(siteID, nil)
	}
	if reload {
		if out, err := call("daemon-reload"); err != nil {
			failures = append(failures, fmt.Sprintf("systemctl daemon-reload: %v: %s", err, strings.TrimSpace(string(out))))
		}
	}
	if len(failures) > 0 {
		return moved, errors.New(strings.Join(failures, "; "))
	}
	return moved, nil
}
