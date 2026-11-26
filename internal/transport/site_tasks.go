package transport

// A site's scheduled tasks as systemd timers in the site's own slice (S4
// R2b, docs/SITE-SANDBOX.md section 9). Additive on the wire.
//
// Sitenin zamanlanmış görevleri, sitenin kendi dilimindeki systemd
// zamanlayıcıları olarak.

// SiteTaskRun is a task's last run as systemd and its journal say.
type SiteTaskRun struct {
	// State: never (no run yet), running, succeeded, failed, unknown, or
	// skipped (SiteTaskRunSkipped: systemd skipped the last start because the
	// site's limits file was missing; StartedAt is when).
	State     string  `json:"state"`
	StartedAt string  `json:"started_at,omitempty"`
	EndedAt   string  `json:"ended_at,omitempty"`
	Seconds   float64 `json:"seconds,omitempty"`
	// ExitStatus is the shell's exit status (ExecMainStatus); Result is
	// systemd's word for the run (success, exit-code, signal, timeout ...).
	ExitStatus int    `json:"exit_status"`
	Result     string `json:"result,omitempty"`
	// Output: the last lines the run wrote (its own journal lines only),
	// each bounded.
	Output []string `json:"output,omitempty"`
	NextAt string   `json:"next_at,omitempty"`
}

// Task states on the Scheduled tasks screen (CronJob.TaskState).
const (
	SiteTaskScheduled = "scheduled" // its timer is in place and on
	SiteTaskOff       = "off"       // its timer is in place but the server owner turned it off (systemctl disable/stop)
	SiteTaskAtBoot    = "at_boot"   // @reboot: it runs once after each start of the server
	SiteTaskKept      = "kept"      // its unit file was changed on the server and is kept
	SiteTaskWaiting   = "waiting"   // the file says it, its timer is not written yet (save again)
	SiteTaskNotRun    = "not_run"   // the line cannot become a timer (Reason)
	// SiteTaskNotOnYet (second fix round): CelikPanel wrote the timer but
	// could not switch it on; it tries again at the next save or start.
	SiteTaskNotOnYet = "not_on_yet"
)

// SiteTaskRunSkipped is a run systemd skipped because the site's limits
// file was missing (the service's ConditionPathExists=); not a failure.
const SiteTaskRunSkipped = "skipped"

// SiteTaskLine is a line of a moved site's task file that does not run, and
// why (a sitecron Reason code; Detail in English).
type SiteTaskLine struct {
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

// Source states of a moved site's task file (ListCronJobsResponse.Source).
const (
	SiteTasksSourceOurs    = "ours"    // CelikPanel's text
	SiteTasksSourceOwner   = "owner"   // the server owner changed it: kept and used, the panel's changes wait beside it
	SiteTasksSourceMissing = "missing" // the file is gone while timers of the site remain
	// SiteTasksSourceCrontab (second fix round): the file is gone, timer
	// files of the site remain, and the account has a crontab again: the
	// tasks run from the crontab; the server owner removes the timer files
	// (ListCronJobsResponse.LeftUnits).
	SiteTasksSourceCrontab = "crontab_again"
)

// Errors a change of a moved site's tasks answers (exact texts; the Panel
// classifies them).
const (
	// SiteTasksSourceGone: the task file is gone while timers of the
	// site remain; a change is refused (it would rebuild the file from one
	// line and remove every other task). The administrator rebuilds the file
	// from the copy of what CelikPanel last rendered.
	SiteTasksSourceGone = "site_tasks_source_missing: the site's task file is missing on the server while timers of the site remain; nothing was changed"
	// SiteTasksLimitsMissing: the site's slice file is not in place (or not
	// loaded by systemd), so no timer of the site is switched on: a task
	// never runs without the site's limits.
	SiteTasksLimitsMissing = "site_tasks_limits_missing: the site's limits file is not in place on the server, so its tasks were not switched on"
	// CronCommandTooLong: cron 3.0pl1 refuses a command of 999 bytes or more
	// (MAX_COMMAND); a task is saved only as cron would run it.
	CronCommandTooLong = "cron_command_too_long: cron runs a command of at most 998 bytes"
	// SiteTasksEnvValue: a setting line of the task file holds a value
	// systemd cannot read (not UTF-8, a quote mark, a backslash, a control
	// character); a save is refused until it is corrected.
	SiteTasksEnvValue = "site_tasks_env_value: a setting of the site's task file holds a value systemd cannot read; nothing was changed"
	// SiteTasksCronSetting (alpha.150 fix round, F5): a setting line of the
	// task file is one some crons read as part of the schedule (CRON_TZ=,
	// RANDOM_DELAY=, START_HOURS_RANGE=); a timer would not keep its meaning,
	// so a save is refused until it is removed.
	SiteTasksCronSetting = "site_tasks_cron_setting: a setting of the site's task file changes the schedule for some crons (CRON_TZ, RANDOM_DELAY, START_HOURS_RANGE), which a timer cannot keep; nothing was changed"
)

// Mail submission as the SMTP block sees it (S4 R2b fix round;
// SiteTasksMovedResponse.MailSubmission, SiteTasksMoveResponse.MailSubmission).
const (
	// MailSubmissionOurs: 465 and 587 are CelikPanel's own unchanged
	// submission entries that take only a mailbox's login; sites' programs
	// and tasks may reach them at this server's own addresses.
	MailSubmissionOurs = "ours"
	// MailSubmissionOwner: an entry or a program of the server owner's is on
	// 465 or 587; the SMTP block keeps those ports closed to sites.
	MailSubmissionOwner = "owner"
	// MailSubmissionAbsent: no submission service; nothing to reach.
	MailSubmissionAbsent = "absent"
	// MailSubmissionOwnerFile (second fix round): the SMTP block's file was
	// changed by the server owner and kept; what it says decides.
	MailSubmissionOwnerFile = "owner_file"
)

// SiteTasksMoveRequest moves a site's crontab into timers (Preview: only say
// what would become what, change nothing), or puts them back (Undo).
type SiteTasksMoveRequest struct {
	ExpectedBuildCommit string `json:"expected_build_commit,omitempty"`
	RequestID           string `json:"request_id,omitempty"`
	CronTenant
	Preview bool `json:"preview,omitempty"`
}

// Outcomes of a tasks move (SiteTasksMoveResponse.Outcome).
const (
	SiteTasksMoved   = "moved"    // the crontab became timers and was emptied
	SiteTasksAlready = "already"  // the site's tasks already run as timers
	SiteTasksStayed  = "stayed"   // a line keeps the whole crontab in cron (Blocking)
	SiteTasksPutBack = "put_back" // a step failed; the crontab is as it was and no timer is left
	SiteTasksPreview = "preview"  // nothing was changed (Preview)
	SiteTasksUndone  = "undone"   // the timers are gone and the crontab is back
	SiteTasksRefused = "refused"  // nothing was changed (Error says why, who acts)
	SiteTasksNoCron  = "no_cron"  // the crontab cannot be read (no cron, or it refuses): nothing was changed
	SiteTasksUnknown = "unknown"  // the result is not known (read the page again)
	// SiteTasksCrontabBack: the tasks run as timers, but a crontab exists
	// again for the account and runs outside the site's limits (never
	// answered as "moved").
	SiteTasksCrontabBack = "crontab_back"
	SiteTaskLineMarked   = "not_made_here"
)

// SiteTaskMoveRow is one line of the crontab and what it becomes.
type SiteTaskMoveRow struct {
	Line      int      `json:"line"`
	Text      string   `json:"text"`
	Unit      string   `json:"unit,omitempty"`
	Calendars []string `json:"calendars,omitempty"`
	AtBoot    bool     `json:"at_boot,omitempty"`
	Paused    bool     `json:"paused,omitempty"`
	// Setting: an environment setting carried to every task.
	Setting bool `json:"setting,omitempty"`
	// Privileged: the task calls a program that needs extra rights (a
	// set-user-id or set-group-id helper such as sudo, crontab or ping);
	// inside a task it runs without them (NoNewPrivileges) and may fail.
	Privileged bool `json:"privileged,omitempty"`
}

// SiteTasksMoveResponse is what the move did or would do.
type SiteTasksMoveResponse struct {
	Outcome string            `json:"outcome"`
	Rows    []SiteTaskMoveRow `json:"rows,omitempty"`
	// Blocking are the lines that keep the whole crontab in cron: a line that
	// cannot become a timer, or one not made in CelikPanel that the
	// administrator has not kept (Reason SiteTaskLineMarked).
	Blocking []SiteTaskLine `json:"blocking,omitempty"`
	// Mail: some task line sends mail (names sendmail, mail, an SMTP port):
	// after the move only submission to this server's own mail service on
	// ports 465 and 587 is allowed, and only where MailSubmission is "ours"
	// (docs/SITE-SANDBOX.md section 3).
	Mail           bool   `json:"mail,omitempty"`
	MailSubmission string `json:"mail_submission,omitempty"`
	Error          string `json:"error,omitempty"`
	// Refusal is a code for Error (cron_allow_owner: cron's access files
	// would let the account install its own jobs after the move, or leave it
	// out for an undo; source_owner: the task file was changed by hand;
	// limits: the site's limits file is not in place; and, as the programs'
	// move answers them (alpha.150), kept_earlier: a move record is kept;
	// another_move, update_running, update_unreadable, site_busy).
	Refusal string `json:"refusal,omitempty"`
	// Recovery (alpha.150): the move's one outcome record when this move or
	// undo left its record kept (Outcome unknown): the site's card shows it
	// with Try again.
	Recovery *SiteSliceMoveRecovery `json:"recovery,omitempty"`
	// Paused (alpha.150 fix round, F6): the undo's tasks whose timer was
	// switched off on the server; they came back into the crontab paused
	// ("# DISABLED: "), each under one comment (Detail: the timer's name).
	Paused []SiteTaskLine `json:"paused,omitempty"`
}

// SiteTaskSourceResponse answers the administrator's rebuild of a missing
// task file from the copy of what CelikPanel last rendered (Preview: only
// say what it would contain).
type SiteTaskSourceResponse struct {
	// Outcome: preview, rebuilt, refused, unknown.
	Outcome string `json:"outcome"`
	// Crontab is what the file holds (or would hold) below CelikPanel's
	// header and intro.
	Crontab string `json:"crontab,omitempty"`
	// RenderedAt is when CelikPanel last rendered the site's timers from it.
	RenderedAt string `json:"rendered_at,omitempty"`
	Error      string `json:"error,omitempty"`
	// Refusal (second fix round): SiteTaskSourceRefusalCrontab when the
	// account has a crontab, so the tasks run from it; LeftUnits then names
	// the site's timer files the server owner removes.
	Refusal   string   `json:"refusal,omitempty"`
	LeftUnits []string `json:"left_units,omitempty"`
}

// Outcomes of a task file rebuild.
const (
	SiteTaskSourceRebuilt = "rebuilt"
	// SiteTaskSourceRefusalCrontab: the account has a crontab; a rebuild
	// would switch the timers on beside it and run every task twice.
	SiteTaskSourceRefusalCrontab = "crontab_exists"
)

// SiteTasksSourceHeld is the error a change of a moved site's tasks answers
// when the server owner changed its task file by hand: the file is kept and
// used, the change was put beside it (<file>.celikpanel-pending).
const SiteTasksSourceHeld = "site_tasks_source_held: the server owner changed this site's task file by hand; it is kept and used, and this change was saved beside it as .celikpanel-pending"

// Refusals of a tasks move or undo.
const (
	SiteTasksRefusalCronAllow = "cron_allow_owner"
	SiteTasksRefusalSource    = "source_owner"
	SiteTasksRefusalCrontab   = "crontab_changed"
	SiteTasksRefusalLimits    = "limits"
	// SiteTasksRefusalTimers (alpha.150 fix round, F6): the undo could not
	// read the state of the site's timers, so it cannot tell which tasks the
	// server owner switched off; nothing was changed.
	SiteTasksRefusalTimers = "timers_unread"
)

// SiteTasksMovedRequest asks which sites run their tasks as timers (SiteIDs
// empty: every site).
type SiteTasksMovedRequest struct {
	SiteIDs []int `json:"site_ids,omitempty"`
}

// SiteTasksMovedResponse lists the moved sites and, per moved site, a
// crontab of its account that came back (its effective line count).
type SiteTasksMovedResponse struct {
	Moved      []int          `json:"moved"`
	Reappeared map[string]int `json:"reappeared,omitempty"`
	// ReappearedBySite: per site id, the account could have installed that
	// crontab itself (the site's own code can have made it); absent: only
	// root could have.
	ReappearedBySite map[string]bool `json:"reappeared_by_site,omitempty"`
	// Recovery: what the Agent did with an interrupted tasks move at its
	// start, per site id (for the administrator's card).
	Recovery map[string]string `json:"recovery,omitempty"`
	// MailSubmission: MailSubmission* as the Agent last wrote the SMTP block
	// ("" when it has not, or the host holds no block).
	MailSubmission string `json:"mail_submission,omitempty"`
	Error          string `json:"error,omitempty"`
}

// SiteMoveRefusalKept is the refusal every new move and undo answers while a
// move record is kept (alpha.150: the programs' move's kept_earlier, one
// record for programs and tasks; SiteTasksMoveResponse.Refusal, and
// MoveSiteSliceResponse.Reason's code).
const SiteMoveRefusalKept = "kept_earlier"

// --- The side doors step: cron.allow and lingering -------------------------

// SideDoorsCron is what the step did about /etc/cron.allow (the same states
// as SideDoorsAt) and the moved sites' accounts with a crontab again.
type SideDoorsCron struct {
	State        string   `json:"state"`
	Path         string   `json:"path,omitempty"`
	SiteAccounts []string `json:"site_accounts,omitempty"`
	Detail       string   `json:"detail,omitempty"`
	// Reappeared: account -> effective lines of a crontab that came back for
	// a site whose tasks run as timers (the server owner's, running outside
	// the site's limits).
	Reappeared map[string]int `json:"reappeared,omitempty"`
	// ReappearedBySite: account -> the account could have installed it
	// itself (the site's own code can have made it).
	ReappearedBySite map[string]bool `json:"reappeared_by_site,omitempty"`
}

// SideDoorsTasksHeld is a moved site whose timers are held at the start:
// its limits file is not in place and could not be written, so its running
// timers were stopped (they start again once the file is back).
type SideDoorsTasksHeld struct {
	SiteID int    `json:"site_id"`
	Domain string `json:"domain,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// SideDoorsUserManagers (S4 R2b second fix round) is what the step did
// about the site accounts' own service managers (user@<uid>.service, which
// an SFTP login starts outside the site's limits): Masked by CelikPanel,
// OwnerMasked by the server owner's own mask (closed too); still open, by
// account: Owner (a file of the server owner's in its place), Removed
// (CelikPanel's mask removed on the server), Failed (not written; Detail
// says why); OpenUnits are their user@<uid>.service names.
type SideDoorsUserManagers struct {
	Masked      int      `json:"masked,omitempty"`
	OwnerMasked int      `json:"owner_masked,omitempty"`
	Owner       []string `json:"owner,omitempty"`
	Removed     []string `json:"removed,omitempty"`
	Failed      []string `json:"failed,omitempty"`
	OpenUnits   []string `json:"open_units,omitempty"`
	Detail      string   `json:"detail,omitempty"`
}

// SideDoorsCronNotNeeded: no site runs its tasks as timers yet; nothing is
// written.
const SideDoorsCronNotNeeded = "not_needed"

// SideDoorsLinger is what the step did about lingering.
type SideDoorsLinger struct {
	// State: not_installed (no polkitd: systemd-logind lets only root switch
	// lingering on), written, current, owner_file (changed by hand, kept),
	// owner_removed (deleted by hand, not written again), failed.
	State  string   `json:"state"`
	Path   string   `json:"path,omitempty"`
	Groups []string `json:"groups,omitempty"`
	Detail string   `json:"detail,omitempty"`
	// Accounts: site accounts (or logins sharing a site's id) that linger now.
	Accounts []string `json:"accounts,omitempty"`
}

// Re-close targets of the side doors step (SideDoorsRequest.ReClose).
const (
	SideDoorsReCloseCron   = "cron"
	SideDoorsReCloseLinger = "linger"
)
