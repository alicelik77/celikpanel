# v0.1.0-alpha.150

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.150.tr.md)

**A security release with corrections.** A site's scheduled tasks now run
inside the site's own limits, and the last ways for a site's own code to
keep jobs running outside them are closed. After the update every site
goes inside its limits by itself, its scheduled tasks and its running
programs; there is nothing to press. A restart of the server during an
update to this release no longer leaves the update unfinished. Four small
corrections follow. The panel's database layout does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.149

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   An update that fails before the new panel has started returns to
   alpha.149 by itself. A completed update is not taken back; see "Going
   back to alpha.149" below.
3. **The update itself restarts nothing of a customer's.** About five
   minutes after the updated panel has started, the panel begins to move
   the sites into their limits, one site at a time: each running program of
   a site is restarted once (see "What to expect after updating").
4. **A restart of the server while this update runs no longer leaves it
   unfinished:** the update finishes by itself once the server has started
   again.

## Security: a site's scheduled tasks run inside the site's limits

- **Until now a site's scheduled tasks ran outside the site's limits.**
  They ran from cron, outside the memory, process and processor limits of
  the site and outside the block on sending mail directly, and a site's
  own code could keep jobs of its own running there: by installing its own
  crontab, by switching lingering on for its account, or through a service
  manager of its own that an SFTP login started.
- **Now each scheduled task is a standard systemd timer** that runs inside
  its site's limits, under the site's account and under the same mail
  block as the site's programs. A task keeps its schedule. What a task
  leaves running in the background stays inside the site's limits.
- **What a customer sees.** Tasks are added and changed on the Scheduled
  tasks screen as before. For each task the screen now shows its last run
  (when, how long, its exit status and the last lines it wrote) and its
  next run, and says in one sentence what happens to a task that is still
  running when its time comes again: it is not started a second time, and
  it runs once more right after it ends. A customer sees only their own
  site's tasks.
- **What is refused, and why.** A site's account can no longer install
  its own crontab (`crontab -e` answers that the account is not allowed to
  use the program), switch lingering on for itself (it is asked for an
  administrator's password), or start a service manager of its own at an
  SFTP login (the login still works). These were the ways a site's own
  code could run jobs outside its limits. Inside a task, programs that
  need extra rights do not get them (`sudo`, `crontab`, `at`, and the
  server's `sendmail` command, so also PHP's `mail()` called from a
  command-line script): a task sends mail by SMTP with a mailbox's name and
  password. A task line that sets `CRON_TZ=`, `RANDOM_DELAY=`,
  `START_HOURS_RANGE=` or a setting of the Go runtime (`GOGC` and the
  like) is refused when it is saved, because a timer could not keep its
  meaning; the time goes into the task's own schedule instead.
- **Every site goes inside its limits by itself.** After the update the
  panel moves each site that is still outside its limits, its scheduled
  tasks first, then its running programs, one site at a time, in the
  order the sites were created, about 20 seconds apart. It starts about
  five minutes after the updated panel has started, never before the
  update has finished, and never while a panel update is being installed.
  There is nothing to press. While it runs, the administrator reads one
  line on Home, in **Problems now** and on the sites' cards: "Sites are
  being moved into their limits (3 / 12) — you do not need to do
  anything"; when it only waits (for the panel's update, or for another
  change on the server), the line says so. When it is done, the line goes,
  and each site's card says that the site is inside its limits. Customers
  and resellers see none of this. A site that is created or restored on
  this release is inside its limits from its first start, and its tasks
  are timers from the start.
- **The buttons "Move into the limits" and "Move them all" are gone.**
- **When one site cannot be moved,** that site keeps running as it was
  and gets one line in **Problems now** and on its card: why, who acts and
  **Try again**. The panel does not try that site again by itself, also
  not at later updates, so a failing site is not restarted again and
  again. A site that was inside its limits and runs outside them again
  (for example because its limits file was deleted on the server) gets the
  same line; no site stays outside its limits without a line.
- **The panel's own update stays possible while sites are being moved.**
  If **Update** is pressed while one site is being moved, the answer says
  that this site's move ends within about 12 minutes and that the panel
  starts no other site meanwhile; the next press starts the update, and
  the moves carry on by themselves after it.
- **What the server owner's own configuration keeps (it is detected, not
  overwritten).** A program whose unit the server owner changed or placed
  with a drop-in is not moved; its site's card names it and who acts. The
  owner's own `/etc/cron.allow` is kept byte for byte: an account it names
  keeps cron, and its site's card says so and who acts. A crontab line that the timers
  cannot reproduce (`MAILTO=` with an address, the settings above, a form
  of the schedule they do not know), or a line not made in CelikPanel that
  the administrator has not kept, keeps the site's whole crontab in cron:
  its programs go inside the limits, and its card names the line, who acts
  and **Try again** for once it is changed. The owner's deletion of
  CelikPanel's `cron.allow` or of its lingering rule stands (**Problems
  now**, with **Close it again**); a task file the owner edits by hand is
  kept and applied, and a timer the owner switched off stays off.
- **For a customer who used these ways until now.** Lines a customer
  added outside the panel with `crontab -e` keep the site in cron until
  the administrator keeps them on the Scheduled tasks screen; **Try
  again** then moves them, and they appear there as tasks. A site account
  that already lingered keeps its running service manager until the
  server owner ends it (**Problems now** names the account with
  `loginctl disable-linger <account>`) or the server restarts; after a
  restart that manager no longer starts. An SFTP session that is open when
  the change is made keeps its service manager until it ends.
- **Everything keeps running without the panel.** The timers, their
  services, the task files, `/etc/cron.allow` and the lingering rule are
  standard files of the system: the tasks keep running with the panel
  stopped, removed or without a licence. With the panel, the administrator
  changes a task on the Scheduled tasks screen, or the server owner edits
  `/etc/celikpanel-cron/<site id>.crontab` and the timers follow at the
  next save or start of the panel. Without the panel, the owner changes
  the time in `/etc/systemd/system/celikcron-<site id>-<n>.timer` and the
  command in `/etc/celikpanel-cron/<site id>.d/<n>.sh`, then runs
  `sudo systemctl daemon-reload`.
- **The administrator can put a site's tasks back into cron** with "Put
  the tasks back into cron" on the site's card; such a site is then left
  as it is until the administrator moves it again from that card. A task
  whose timer the server owner switched off comes back paused.

## Updates: a restart of the server during an update

- **Until now** a restart of the server inside an update's last stage
  could leave the update unfinished on a server that had not been
  restarted since CelikPanel's firewall rules were first saved on it: the
  restart applied those rules at boot, as it should, and the update then
  refused to go on because a service it had recorded as stopped was
  running. The panel and its agent stayed stopped, and neither the
  automatic recovery nor the recovery command finished it; hosted sites,
  mail and DNS kept running.
- **Now the update finishes by itself after such a restart.** The update
  records which start of the server it was made in; after a restart it
  accepts the firewall rules that the server's own start applied, and only
  those (a start of them by hand or by anything else is still refused).
  The automatic recovery waits while the server is still starting and
  uses none of its attempts meanwhile. A process number recorded before
  the restart is no longer compared after it, where it could only match by
  chance.
- **This holds for updates to this release and later.** An update to an
  earlier release that a restart left unfinished in this way is not
  finished by this release.

OPEN: the server owner's steps to finish such an update to an earlier
release are added here only once they have been checked in the lab.

## Corrections

- **The update page after an update that was refused before it started**
  no longer stays locked: once two minutes have passed and no recovery has
  taken over, the notice turns red and can be closed; closing it starts
  nothing. A reload or a second tab does not start the two minutes again.
  An update whose work had begun keeps its notice open, as before, while
  the recovery may still start.
- **The heading of a refusal names who must act.** When removing a
  database user or changing its password is refused for a reason that the
  server's administrator decides, the administrator's window says "Waiting
  for the server's administrator to act". Customers and resellers keep the
  general heading.
- **Deleting a database whose user was left on the database server** now
  says that the database was deleted, which users were left and why, and
  that the server's administrator acts; it no longer asks to press Delete
  again where pressing again cannot help. Where the users' rights could not
  be read, it says that Delete can be pressed again. Where the answer about
  some users is not known, the earlier answer stays.
- **The move's answer names a program's state in one plain word** in the
  agent's journal and in the audit record.

## What to expect after updating

- No schema step: the layout stays schema 81. A completed update is not
  taken back (see "Going back to alpha.149").
- **The update itself restarts and reloads nothing of a customer's:** no
  program, no scheduled task, no PHP pool, no web server, no database
  session. While it runs the panel is not available; hosted sites, mail,
  DNS and databases keep running.
- **About three minutes after the updated panel has started,** a step
  that runs after every start of the panel writes, by itself:
  - where polkit is installed, the lingering rule
    (`/etc/polkit-1/rules.d/49-celikpanel-linger.rules`): from then on a
    site's account cannot switch lingering on for itself;
  - for every site account, the mask of its own service manager
    (`/etc/systemd/system/user@<uid>.service`, a link to `/dev/null`), then
    a reload of systemd's configuration: an SFTP login still works but
    starts no service manager; a session open at that moment keeps its
    manager until it ends;
  - where the sites' mail block is in place, the block again: where this
    server's own submission service is CelikPanel's unchanged one, the
    sites' programs and tasks may now send through it (ports 465 and 587 of
    this server, with a mailbox's name and password); the change is loaded
    at once and restarts nothing.
  Nothing of a customer is restarted by this step.
- **About five minutes after the updated panel has started,** the panel
  moves the sites into their limits, one site at a time, about 20 seconds
  apart. For each site:
  - **Its scheduled tasks first.** The crontab of the site's account is
    turned into timers and emptied, and the account leaves cron:
    `crontab -e` and `crontab -l` are refused for it from then on (also for
    a site without a crontab). The tasks keep their times; one run can be
    missed if a task is due in the very minute of the move, and none runs
    twice. From then on a task's output is kept with its run in the
    system's journal (it was not mailed before either), and inside a task
    the differences listed under "Security" apply (no extra rights, mail by
    SMTP with a mailbox's login).
  - **Then its running programs** (uploaded programs, Python apps, the
    Node.js application, service apps such as n8n): **each is restarted
    once** inside the site's limits and must run and answer on its ports
    again within its start time: 90 seconds for these programs (never
    under 20 or over 300). A program is down for as long as it takes to
    start again. A program that does not come up in that time is put back
    where it ran and started there again (so restarted twice), and its
    site gets the line with **Try again**. A stopped program is not
    started; it starts inside its limits when it is next started. A
    program already inside its limits (moved in alpha.148 or alpha.149) is
    not restarted.
  - The site's PHP pool, the web server, databases, mail and DNS are not
    restarted or reloaded.
  A server with N sites outside their limits is done after about N × (20
  seconds + that site's own move): a site without running programs takes
  seconds, one with programs about as long as its slowest program takes to
  start again and answer.
- **A site whose crontab stays in cron** (a line the timers cannot
  reproduce, a line not made in CelikPanel that the administrator has not
  kept, or an account the server owner's own `/etc/cron.allow` names):
  its programs are moved and restarted once as above; its crontab and
  `crontab -e` stay as they were.

## Going back to alpha.149

- **An update that fails returns to alpha.149 by itself, before anything
  of this release has run.** The new panel's start check runs before the
  new panel is started; when it fails, or the update stops at any point
  before that start, the update puts alpha.149 and its database back by
  itself. No site has been moved then: the moves start only after the
  update has finished, and the panel's agent refuses every move while the
  update is not finished, also across a restart of the server. Only the
  update itself is of this release: it records the start of the server it
  was made in, beside the states it saves; alpha.149 ignores that record.
- **A completed update is not taken back.** From the moment the new panel
  is started the update is carried to its end. The panel offers no way
  back, the release's `rollback.sh` refuses a completed update before it
  stops anything, with:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`,
  and the recovery command only finishes an interrupted update.
- **Do not bring alpha.149 back by hand.** If it were brought back onto
  a server that has run this release:
  - the programs that were moved stay inside their limits (alpha.149
    knows the site's limits and starts them there);
  - **every moved site's scheduled tasks stay timers that alpha.149's
    Scheduled tasks screen cannot show**, and `crontab -e` stays closed for
    every moved site's account: alpha.149's screen says that the crontab
    cannot be read because `/etc/cron.allow` does not list the account, and
    it cannot add a task there. The tasks keep running inside the limits;
  - alpha.149 writes its own mail block back at its first program change,
    so the tasks and programs can no longer send through this server's
    submission service;
  - the lingering rule and the masks of the site accounts' service
    managers stay;
  - a tasks move that had not finished is forgotten by alpha.149 (the
    crontab is not lost; a move stopped after the crontab was emptied
    leaves that site's tasks not running);
  - the note above a task that "Put the tasks back into cron" brought back
    paused is shown by alpha.149, in English, as that task's description;
  - the panel's own records of the moves are settings alpha.149 does not
    read.
  To put one site's tasks back into cron by hand (as root; `<id>` is the
  site's id, `<account>` its account; it does nothing at all while the
  account has a crontab):
  `sudo test ! -e /var/spool/cron/crontabs/<account> && sudo test ! -e /var/spool/cron/<account> && sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-<id>-*.timer' | awk '{print $1}') && echo <account> | sudo tee -a /etc/cron.allow >/dev/null && sudo crontab -u <account> /etc/celikpanel-cron/<id>.crontab`
  (without `/etc/cron.allow`, the account is removed from `/etc/cron.deny`
  instead). The way that needs no command is "Put the tasks back into
  cron" on each site's card while this release still runs.

## Known limits

- **A completed update is not taken back** (see "Going back to
  alpha.149").
- **A soft restart is not recognised as a restart.** A `systemctl
  soft-reboot`, or the restart of a container that shares its host's
  kernel, keeps the identity of the running kernel's start; an update in its last
  stage then stays unfinished as before. Do not restart the server that
  way until the update's page says that the update has finished.
- **A site's PHP runs outside its limits.** The site's PHP-FPM pool, and
  what a PHP script starts, are not inside the site's limits.
- **A program that does not come up inside its limits** is put back and
  restarted where it ran, and its site stays outside its limits with
  **Try again**; it is not tried again by itself.
- **A site that waited for hours behind a busy server** (another change,
  the panel's update or another move, for six hours) is shown with **Try
  again** instead of being tried for ever.
- **While the moves wait for a move that needs the administrator** (a
  move that did not finish, shown with its own line in **Problems now**),
  the sites after it wait too.
- **When the update is pressed during a site's move,** the panel starts no
  other site for up to 30 minutes, or until the update is started.
- **A process a task leaves running keeps running** inside the site's
  limits until the administrator ends it (`systemctl kill
  celikcron-<id>-<n>.service`), also after the task was removed; under
  cron it kept running too, outside the limits.
- **Many tasks due in the same second start together,** as under cron.
- **Inside a task, the server owner's per-login limits**
  (`/etc/security/limits.conf`) do not apply; the site's limits do.
- **While a very large crontab is being moved,** saves of scheduled tasks
  on the server wait for it.
- **A timer the server owner only stopped** (not switched off) comes back
  as a running cron line when the tasks are put back into cron.
- **The mail exception follows the mail services at each start of the
  panel;** a program the owner later puts on ports 465 or 587 is not
  noticed until the panel's next start, and without the panel the
  exception stays as last decided.
- **An account the server owner creates later** may use `crontab` from
  the next start of the panel; until then, or without the panel, the owner
  adds its name to `/etc/cron.allow` or deletes the file.
- **On Ubuntu 22.04** (polkit 0.105) the lingering rule is not read: the
  panel reports lingering as not closed there.
- **Accounts from a directory service** (LDAP, SSSD): nothing is written
  to `/etc/cron.allow`, and the panel says so.
- **A very large crontab** (hundreds of tasks on a slow server) can take
  longer to move than the panel waits for the answer; the site's card then
  says that the result is not known yet and to read it again before
  pressing again.
- **A task file edited by hand** reaches the timers at the next save on
  the screen or the next start of the panel.
- **A panel database put back from a copy made before a site's move
  failed** no longer knows of that failure: that site is tried once more by
  itself, so its programs are restarted and put back one more time.
- The open list of alpha.149 stays open where this release does not close
  it ([alpha.149 release notes](RELEASE-NOTES-v0.1.0-alpha.149.md)).

## Checked in the lab

OPEN: filled in after the lab run.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New and additive: the panel settings `site_auto_move_v1`,
`site_tasks_put_back_v1` and `site_move_tasks_v1`; Problems now kinds 48
(`site_auto_move_failed`), 49 to 58 (`cron.allow`, crontabs that came
back, lingering, held tasks, the site accounts' service managers) and 59
(`site_auto_move_unread`); the update refusal
`PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING`; the agent calls
`MoveSiteTasksV1`, `UndoSiteTasksMoveV1`, `SiteTasksMovedV1` and
`RebuildSiteTaskSourceV1`; the move record's version 3 with a tasks part;
the file `service-states.boot-id` in the update's snapshot (listed in its
checksums). Removed: the route `/api/v1/site-limits/outside` and
Problems now's line of sites outside their limits (kind 42 stays known so
that an alert alpha.149 opened can close).

Tests by part:

- Scheduled tasks inside the limits: `TestTasksMoveSurvivesAnAgentStopAtEveryPoint`,
  `TestTasksMoveLeavesNoGapForTheSiteToPutACrontabBack`,
  `TestTasksMoveIsRefusedWhenTheAccountCouldStillUseCron`,
  `TestTasksUndoSurvivesAnAgentStopAtEveryPoint`,
  `TestTasksMoveAndUndoKeepTheCrontabByteForByte`,
  `TestNoTimerIsSwitchedOnWithoutTheSiteLimitsFile`,
  `TestSiteAccountsServiceManagerIsMasked`, `TestLingerRuleAndAccounts`,
  `TestTheAgentsStartWithATasksRecordMakesAtMostFourSystemdCalls`,
  `TestACrontabPutBackByHandNeverGetsTimersBeside`,
  `TestAKeptMoveWhoseTimersWereSwitchedOffSwitchesThemOnAgain`,
  `TestACrontabProcessIsKnownByItsFileNotItsPath`,
  `TestTheUndoKeepsWhatTheOwnerSwitchedOff`,
  `TestTasksMoveUndoAndRebuildKeepTheUpdateGate`;
  `internal/sitecron`; `TestEgressGuardKeepsWhatATaskLeftRunning`,
  `TestEgressSubmissionExceptionOnlyWhereWritten`; web `site-tasks-r2b`,
  `site-tasks-a150`, `site-tasks-round4`.
- Every site by itself: `TestTheWorkerMovesEverySiteOnceInOrder`,
  `TestTheWorkerStartsOnlyAfterTheGate`,
  `TestTheUpdatesPendingMarkerRefusesEveryMove`,
  `TestTheUpdatesPendingMarkerRefusesTheTasksMoves`,
  `TestARealFailureIsNotTriedAgain`, `TestAKeptRecordStopsTheRun`,
  `TestARestartInTheMiddleNeverMovesTwice`, `TestSingleFlight`,
  `TestAPageReadStartsNothing`, `TestASitePutBackOnPurposeIsLeftAlone`,
  `TestCustomersAndResellersReadNothingOfIt`,
  `TestTheWorkerHoldsForAnUpdateTheAdministratorAskedFor`,
  `TestAFailingSiteIsNotRestartedAtEveryUpdate`,
  `TestNoSiteStaysOutsideWithoutALine`,
  `TestTheReadingThatFailsDoesNotGiveUp`, `TestTheEndingSaysWhatIsKnown`;
  web `site-auto-move-a150`, `site-auto-move-a150-fix`.
- Updates: `deploy/test-saved-runtime-boot-rule.sh` (the rule of the
  three update scripts, run as they are), the stage reset of
  `deploy/test-release-transaction-guard.sh`,
  `deploy/test-abort-pre-mutation-active-update-behavior.sh`,
  `TestKilledUpdaterRecordSaysWhetherItsTransactionBegan`; web
  `system-update-outcome`.
- Corrections: web `operation-actor-a150`; the database deletion's
  answers in `cmd/panel`; `TestSiteUnitPlainStateIsOneWord`.
