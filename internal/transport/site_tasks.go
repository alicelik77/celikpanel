package transport

// A site's scheduled tasks as systemd timers in the site's own slice (S4
// R2b, docs/SITE-SANDBOX.md section 9). Additive on the wire.
//
// Sitenin zamanlanmış görevleri, sitenin kendi dilimindeki systemd
// zamanlayıcıları olarak.

// SiteTaskRun is a task's last run as systemd and its journal say.
type SiteTaskRun struct {
	// State: never (no run yet), running, succeeded, failed, unknown.
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
)

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
	SiteTasksMoved     = "moved"    // the crontab became timers and was emptied
	SiteTasksAlready   = "already"  // the site's tasks already run as timers
	SiteTasksStayed    = "stayed"   // a line keeps the whole crontab in cron (Blocking)
	SiteTasksPutBack   = "put_back" // a step failed; the crontab is as it was and no timer is left
	SiteTasksPreview   = "preview"  // nothing was changed (Preview)
	SiteTasksUndone    = "undone"   // the timers are gone and the crontab is back
	SiteTasksRefused   = "refused"  // nothing was changed (Error says why, who acts)
	SiteTasksNoCron    = "no_cron"  // the crontab cannot be read (no cron, or it refuses): nothing was changed
	SiteTasksUnknown   = "unknown"  // the result is not known (read the page again)
	SiteTaskLineMarked = "not_made_here"
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
	// 127.0.0.1 ports 465 and 587 is allowed (docs/SITE-SANDBOX.md section 3).
	Mail  bool   `json:"mail,omitempty"`
	Error string `json:"error,omitempty"`
	// Refusal is a code for Error (cron_allow_owner: the server owner's
	// /etc/cron.allow leaves the account out; source_owner: the task file was
	// changed by hand; busy: another move runs).
	Refusal string `json:"refusal,omitempty"`
}

// SiteTasksSourceHeld is the error a change of a moved site's tasks answers
// when the server owner changed its task file by hand: the file is kept and
// used, the change was put beside it (<file>.celikpanel-pending).
const SiteTasksSourceHeld = "site_tasks_source_held: the server owner changed this site's task file by hand; it is kept and used, and this change was saved beside it as .celikpanel-pending"

// Refusals of a tasks move or undo.
const (
	SiteTasksRefusalCronAllow = "cron_allow_owner"
	SiteTasksRefusalSource    = "source_owner"
	SiteTasksRefusalBusy      = "busy"
	SiteTasksRefusalCrontab   = "crontab_changed"
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
	// Recovery: what the Agent did with an interrupted tasks move at its
	// start, per site id (for the administrator's card).
	Recovery map[string]string `json:"recovery,omitempty"`
	Error    string            `json:"error,omitempty"`
}

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
