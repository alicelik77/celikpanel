# v0.1.0-alpha.147

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.147.tr.md)

> **Published** on 2026-11-24 (UTC): release commit `eef8fe894`, GitHub pre-release at 04:31:04Z, download portal at about 04:35Z (public check 27/27).

**A security release with one correction.** The database tools and webmail
no longer run under the web server's shared account, site accounts can no
longer schedule jobs with the system's `at`, and the distribution's shared
PHP pool is switched off where nothing uses it. A longer change no longer
stays "in progress" on the panel for ever. The panel's database layout does
not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.146

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   An update that fails before the new panel has started returns to
   alpha.146 by itself. A completed update is not taken back: each change of
   this release is undone on alpha.147 instead; see "Going back to
   alpha.146" below.
3. About three minutes after the panel's first start, once no other server
   change is running, the panel runs one step by itself, in its own job: it
   moves the database tools and webmail to their own accounts, switches off
   the distribution's PHP pool where nothing uses it, closes `at` to site
   accounts and takes note of the site crontabs (see "Security: customers
   are kept further apart"). There is nothing to press. What it cannot do is
   a line in **Problems now** with who acts and the step. PHP sites and
   webmail can answer with an error for a few seconds while this step
   reloads PHP; to choose that moment, update at a quiet hour.

## Security: customers are kept further apart

- **The database tools and webmail ran under the web server's shared
  account.** phpMyAdmin, phpPgAdmin and webmail (Roundcube) ran in the
  distribution's shared PHP pool as the web server's account, whose group
  can read every site's folder. A fault in any of them could read every
  site.
- **Now each has a system account of its own**: `celikpanel-webmail`,
  `celikpanel-phpmyadmin` and `celikpanel-phppgadmin` (no login, no home
  folder, in no other group), each with its own PHP pool on the PHP version
  the tool runs on, and its sessions, uploads and temporary files in
  `/var/lib/celikpanel-php-tools/<tool>/`. Only the web server can reach a
  tool's pool. New installations get this from the start; existing ones are
  moved by the step above. A move is the same verified step as choosing a
  tool's PHP version in its Manage window: the tool must answer as itself
  afterwards, otherwise everything comes back as it was and **Problems now**
  says so; choosing the tool's PHP version in its Manage window tries the
  move again.
- **Webmail's folder is closed to every site account.** The root of
  `/var/lib/celikpanel-webmail` stays closed to other accounts, as the
  install leaves it; webmail's own account alone gets through it, by one
  access-control-list entry (`user:celikpanel-webmail:--x`; the owner reads
  it with `getfacl` and removes it with `setfacl -x`). Webmail's database,
  temporary files and logs become its account's; its configuration becomes
  readable by its account only. Webmail's local door on the server (closed
  in alpha.145 to everything but CelikPanel's own checks) now hands its PHP
  to webmail's own pool too. **Webmail needs a file system with
  access-control lists for this:** where the file system refuses them,
  webmail stays on the web server's account, nothing is changed, and
  **Problems now** says why and what the owner can do (the file system's
  `acl` option).
- **Site accounts can no longer schedule jobs with `at`.** Where the `at`
  package is installed and no `/etc/at.allow` exists, CelikPanel writes one
  that lists every account that may use `at` today except the website
  accounts (and every account that shares a website account's id, such as an
  extra FTP login). Pending `at` jobs of site accounts are listed for the
  administrator (number, account and time), never removed. An `/etc/at.allow`
  the owner wrote is kept as it is; site accounts it names are shown to the
  administrator.
- **Crontab lines a site adds outside the panel are shown.** On a site's
  Scheduled tasks list a line not made in CelikPanel is marked "Not made in
  CelikPanel": it can be deleted; the administrator may keep it. The agent
  refuses to edit or enable such a line until it is kept. One line in
  **Problems now** names the sites and how many lines.
- **The distribution's shared PHP pool is switched off where nothing uses
  it.** Each PHP version's pool `www.conf` runs as the web server's account.
  It is switched off (renamed to `www.conf.off-by-celikpanel`, checked with
  `php-fpm -t` and reloaded; put back if either refuses) only when it is the
  package's untouched file, another pool runs beside it, and nothing that a
  web server loads hands PHP to it. It is kept, and **Problems now** names
  what keeps it, when nginx's configuration names its socket (also by
  another spelling or a link, or through a socket path that cannot be
  followed, or a `*_pass` address made from a variable other than
  CelikPanel's own), when Apache is installed and loads a file that names
  it, when an enabled or running Apache lets `.htaccess` files override
  anything, or when another web server (httpd, lighttpd, Caddy,
  OpenLiteSpeed) is enabled or running. A version is never left without a
  pool: before its last other pool goes, the pool is put back.
- **The owner's own decisions stand.** A tool's web server file or pool the
  owner changed is kept as it is. When the owner puts a switched-off pool
  back, or deletes `/etc/at.allow`, CelikPanel leaves it so and shows one
  line in **Problems now** with one button ("Turn it off again", "Close
  again") that acts on that one thing only, after the panel's own
  confirmation. When it did not succeed, its result stays shown under that
  line, also after a panel restart.
- **What a site account can still reach.** On the file system, nothing under
  webmail's folder. The tools' pools only through the web server. Customers
  cannot write a `*_pass` directive; a customer who names the shared pool's
  socket in a harmless directive keeps that pool switched on and gains
  nothing by it.

## Correction: a longer change no longer stays "in progress"

- **Until now** (the known limit of alpha.145 and alpha.146), a longer change
  — a webmail update, a component, mail profile or Node.js install, an
  install or removal together, a webmail address — could stay "in progress"
  on the panel after it had finished on the server, when the panel lost
  touch with its agent for a moment. Other server changes were then refused
  until the panel was restarted.
- **Now the panel reads what the server did and records the result by
  itself.** A webmail update or a webmail address is only read; nothing is
  run again. An interrupted install or removal is run once more from its
  start, which may take as long again. Meanwhile the page says what is
  happening ("reading the result from the server", "recording the
  result") and that nobody needs to act.
- **One late answer from the agent no longer stops a running change**: only
  when the agent leaves the panel's renewals unanswered for the whole window
  is the change stopped, and then it is settled as above.
- **When the result still cannot be recorded**, the page, every refusal of
  another change and **Problems now** give the steps: check the agent
  (`sudo systemctl status celikpanel-agent`), then
  `sudo systemctl restart celikpanel-panel`. If the server's package manager
  stays busy while an install is run again, the change ends with its own
  words ("the server stayed busy") instead of "could not be started".

## Under the hood: prepared for per-site limits

This release can be the one you come back to from the release that
introduces per-site limits: programs moved into their site's limits keep
starting with the mail block, plans and accounts keep their site limits when
you edit them here, and a program keeps its site's limits when this release
writes it again. Going back further than this release is not supported for
servers that have run per-site limits: programs moved into a site's limits
would not start. On a server today nothing a customer sees changes: at each
program start and stop, the small helper that keeps a program out of the
mail ports first reads where the program runs from the system itself, then
asks systemd, as before.

## Going back to alpha.146

- **An update that fails returns to alpha.146 by itself.** The new panel's
  start check runs before the new panel is started; when it fails, or the
  update stops at any point before that start, the update puts alpha.146
  and its database back by itself. None of this release's changes exist
  then: the step that moves the tools runs only about three minutes after
  the new panel has started.
- **A completed update is not taken back.** From the moment the new panel is
  started the update is carried to its end, also when that start fails. The
  panel offers no way back to the release before, and the release's
  `rollback.sh` refuses a completed update before it stops anything, with:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
  The update keeps its snapshot. `sudo /usr/libexec/celikpanel/recovery recover`
  finishes an update or rollback that was interrupted; after a completed
  update it has nothing to do.
- **Each change of this release is undone on alpha.147:**
  - The distribution's shared PHP pool: rename it back and reload PHP-FPM.
    CelikPanel leaves it on and shows "The distribution's PHP … pool was put
    back on the server" in **Problems now**, with "Turn it off again".

    ```bash
    sudo mv /etc/php/<version>/fpm/pool.d/www.conf.off-by-celikpanel /etc/php/<version>/fpm/pool.d/www.conf
    sudo systemctl reload php<version>-fpm
    ```

  - `at` for site accounts: delete `/etc/at.allow`. CelikPanel does not write
    it again by itself and shows "Website accounts can use at again", with
    "Close again".
  - The database tools and webmail stay on their own accounts: the panel has
    no action that puts them back on the web server's account (choosing a
    tool's PHP version in its Manage window keeps its own account).

## Known limits

- **A completed update is not taken back** (see "Going back to alpha.146");
  this release's changes are undone on alpha.147, and the database tools
  and webmail stay on their own accounts.
- **A few seconds of errors when the step runs.** About three minutes after
  the update the panel moves the tools and reloads PHP, all within a few
  seconds. At that moment PHP sites and webmail can answer with an error
  for a few seconds (in the lab: up to four seconds, a handful of
  requests). An administrator who wants to choose the moment updates at a
  quiet hour.
- **If the panel is held up exactly when that step ends**, the step is
  complete on the server, but its lines in **Problems now** appear only
  after the panel's next start.
- **On Arch Linux** the tools stay on the web server's account and the PHP
  pool is not switched off: this release moves them on Debian and Ubuntu.
  `at` and the site crontabs are handled on every system.
- **A site account can still add lines to its own crontab.** They are shown,
  can be deleted and, by the administrator, kept; they are not prevented.
- **Not looked at when the shared PHP pool is switched off:** programs that
  talk to its socket directly (for example a scheduled job that resets the
  opcode cache, or a monitoring agent reading the pool's status page), a web
  server started outside systemd, and Lua code that `nginx -T` does not
  print. If one of them uses the pool, rename it back as above; CelikPanel
  then leaves it on.
- **The database tools' local server** (`127.0.0.1:8306`, behind the panel's
  `/dbtool/`) answers every account on the server and every signed-in panel
  user. It gives out the packages' own files (code, documentation, SQL
  samples; no secret) and runs their PHP under the tools' own accounts; the
  tools' own logins guard the databases.
- **When a move is put back**, a file that is already gone at that moment is
  only a line in the agent's journal, also when it mattered.
- **If the owner opens webmail's folder to other accounts again**, CelikPanel
  does not undo that and does not say so; a removed access-control-list entry
  is said in **Problems now**.
- **A crontab under the name of a login that shares a site's account id**,
  and `systemd --user` timers, are not looked at.
- **A change the panel could not record** settles by itself only at the
  panel's next start once it has been named "not recorded"; until then
  firewall, DNS and mail TLS changes are admitted beside a half-finished
  component install. An install is run again from its start on the
  assumption that its steps are safe to repeat.
- The open list of alpha.145 stays open
  ([alpha.145 release notes](RELEASE-NOTES-v0.1.0-alpha.145.md)).

## Checked in the lab

Lab check finished: one run on the pair of test servers, a Debian 13 main
server with mail, webmail on its own address, phpMyAdmin and phpPgAdmin,
and an Ubuntu 24.04 server with phpMyAdmin and webmail (24 November 2026,
UTC 00:25 to about 02:17 with the clean-up). Both servers started from
alpha.146's code and were updated through the panel's own update page to
this release's code (`1c605428d`, under the lab-only label
v0.1.0-alpha.155). On each server one customer's site account played the
other side. Everything was done through the panel's own pages except where
it says the lab did it. No hard failure; what the lab met is in "Known
limits".

- **Before the update** (alpha.146's code), on both servers: phpMyAdmin,
  phpPgAdmin and webmail ran as the web server's account, which read
  webmail's configuration and database, phpMyAdmin's secret files and
  every site's files. A site account could not read webmail's folder or
  phpMyAdmin's secret files, and could schedule an `at` job.
- **Both servers updated through the panel's own update page by
  themselves**, each in about a minute and a half. No schema step (it stays
  81); one panel and one agent ran on each server; no failed service. A
  check every second saw webmail, the database tools and a site answer
  throughout the update on both servers.
- **The step, about three minutes after the start**, ran by itself on both
  servers: the tools moved to their own accounts and PHP pools, the web
  server's files for the tools and for webmail's own address hand PHP to
  those pools, the distribution's pool was switched off (the package's file
  kept unchanged under its new name), and `/etc/at.allow` was written with
  the accounts that could use `at` before, without any site account, extra
  FTP login or tool account. While it ran, PHP requests of sites on that
  PHP version were answered with an error for up to about four seconds,
  and webmail on the main server for about two seconds (see "Known
  limits").
- **The security change, as a site account**, on both servers: its PHP and
  its scheduled tasks still read nothing of the tools' files, and `at` was
  refused, also for an extra FTP login sharing the site's account; the
  owner's own account could still use `at`. The tools' own accounts could
  not read a site's files, and the web server's account no longer read
  webmail's configuration or database or phpMyAdmin's secret files.
  Through webmail's local door the site account got the sign-in page and
  "not found" for a plugin's backup copy and configuration files.
- **The tools work after the move:** on the main server a mailbox signed in
  on webmail's own address and sent a message with an attachment (it
  arrived signed for its domain and is in Sent), and a database user signed
  in to phpMyAdmin and ran a query. After a reboot of the main server
  phpMyAdmin, phpPgAdmin and webmail answered and the moves were kept.
- **Problems now**, in Turkish and English: the site accounts' waiting `at`
  jobs with number, account and time (no command), and the crontab lines
  not made in CelikPanel with the site and their number; on the site's
  Scheduled tasks the `@reboot` line the lab had added was listed under the
  other lines, with Keep and delete.
- **The owner's undo and closing again** (Ubuntu server): after the lab, as
  the owner, deleted `/etc/at.allow` and renamed the distribution's pool
  back, CelikPanel left both so after a panel restart and showed "Website
  accounts can use at again" with "Close again" and "The distribution's PHP
  8.3 pool was put back on the server" with "Turn it off again". While an
  owner's site still used that pool, "Turn it off again" kept it on and
  named the site's file; "Close again" wrote `/etc/at.allow` again with the
  same names. Once the owner's site was gone the pool stayed on until "Turn
  it off again" was pressed, which turned off only that pool with one
  reload; webmail, phpMyAdmin and the site kept answering.
- **The "in progress" correction:** a webmail update with the agent paused
  for 30 seconds finished and was recorded by itself, without a restart; a
  ten-second pause did not stop an install; a long install with the agent
  paused for 30 seconds mid-install was not cut, was run once more and
  recorded by itself, never with two package runs at once; with the agent
  stopped for three minutes during an install, a DNS save and a firewall
  change were answered at once with the reason and nothing hung, the page
  named the agent check first and the panel restart second, no other
  install could be started, and after the panel restart the install ran
  once more and succeeded; with the panel stopped mid-install and started
  while the package manager was held for three minutes, the change ended
  with its "the server stayed busy" words.
- **The per-site-limits preparation:** the firewall refused a place that
  does not exist on both servers, and again after the update on the main
  server; on Debian
  13 the helper sees the program's own place; on a server without per-site
  limits a program stayed in the mail block through a restart, a kill, a
  redeploy and a reboot, with ports 25, 465 and 587 refused from inside it;
  a program the lab put into its site's limits by hand kept the block, also
  after a redeploy, and went back when that file was removed.
- **Going back:** the release's `rollback.sh` was refused right after the
  update, on the Ubuntu server before the step and on the main server after
  it, with the line in "Going back to alpha.146"; nothing was stopped or
  changed.
- The usual pages on both servers loaded without a script error.

**Where the lab left the page's path.** As root on the lab servers, the lab
installed `at`, made an owner account, queued an `at` job and added
crontab lines as a site account, planted copies and a link in webmail's
plugin folder, paused and stopped the agent and the panel, held the
package manager's lock, deleted `/etc/at.allow`, renamed the distribution's
pool back, wrote an owner's site that used it, restarted the panel,
rebooted the main server, put a program into its site's limits by hand,
ran `rollback.sh`, and cleaned up at the end. Its own certificate authority
was trusted on both servers during the run.

**Not checked:**

- The automatic return through a failed start check of the new panel.
- A move cut by a panel restart (only a pause was tried), and a restart
  during an install's second run.
- The server setup steps with a paused agent.
- What needs a second PHP version, a file system without access-control
  lists, XFS or Btrfs, Apache, lighttpd or a reverse-proxy site: a tool
  that cannot get its own account, a site deleted while a tool moves, the
  agent killed in a move, the owner removing webmail's entry, a power loss
  in a move, the "in use" cases of other spellings, the hourly read with a
  slow tool.
- In the per-site-limits preparation: systemd under stress, the previous
  helper with a program in a site's limits, the limits rows, the firewall
  flush and the reboot with a program in its site's limits, and program
  kinds other than a plain program.
- Nothing was tried on Arch Linux.

## What to expect after updating

- No schema step: the layout stays schema 81. A completed update is not
  taken back; each change of this release is undone on alpha.147 (see
  "Going back to alpha.146").
- About three minutes after the first start the panel moves the tools to
  their own accounts and switches off the shared pool where nothing uses
  it: PHP-FPM and the web server are reloaded, all within a few seconds.
  **PHP sites and webmail can answer with an error for a few seconds at that
  moment** (in the lab: up to four seconds, a handful of requests); to
  choose the moment, update at a quiet hour. `/etc/at.allow` appears where
  `at` is installed and the owner had none.
- phpMyAdmin, phpPgAdmin and webmail sign in and work as before. If one of
  them does not answer, **Problems now** says so, with the step.
- **Problems now** may show lines that need the owner: a shared pool kept
  because something uses it (it names what), pending `at` jobs of site
  accounts, crontab lines not made in the panel. Each says who acts.
- A longer change that loses touch with the agent ends by itself; the page
  says while it settles that nobody needs to act.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New and additive: the agent calls `CloseSideDoorsV1`,
`ReadSideDoorsV1` and `KeepCronLineV1`; the panel route
`/api/v1/metrics/side-doors/reclose` (administrator only); the audit actions
of the moves, the pool, `at` and kept crontab lines; Problems now kinds 22
to 38; the settle phases and two terminal codes of the correction.

Tests by part:

- Tools on their own accounts: `TestASiteAccountReachesNothingUnderWebmailsTree`
  (real files and owners), `TestWebmailStaysOnTheWebAccountWhereTheFileSystemRefusesAccessControlLists`,
  `TestTheUndoKeepsAPathChangedSinceAndNamesIt`,
  `TestTheUndoRemovesAFileTheMoveCreatedAndSaysNothingOfOneAlreadyGone`,
  `TestAnAccessControlListOfAnySizeIsRead`, `TestInterruptedMoveTable`,
  `TestWebmailLocalServerOnItsOwnPoolKeepsAlpha145Rules`,
  `TestWebmailConfigRewriteKeepsTheOwnAccountsGroup`,
  `TestEveryToolMoveTakesTheSideDoorsLockAfterItsStep`.
- The shared PHP pool: `TestTheStockPoolInUseCheckFailsOnlyInTheSafeDirection`,
  `TestOnlyApacheFilesApacheLoadsKeepTheStockPoolOn`,
  `TestAnApacheIncludeThatCannotBeFollowedKeepsThePoolOn`,
  `TestOtherWebServersKeepTheStockPoolOn`,
  `TestUnixPathsAreComparedByWhatTheyResolveTo`,
  `TestThePanelsOwnUpstreamLineDoesNotKeepTheStockPoolOn`,
  `TestThePutBackKeepsTheTurningMarkUntilThePoolIsBack`,
  `TestTheLastSiteDeletedWhileAToolMovesOffLeavesThatVersionAPool`,
  `TestForgettingAVersionNeverRewritesAnUnreadableRecord`.
- `at` and crontabs: `TestACutAtAllowWriteIsCelikPanelsUnfinishedStep`,
  `TestTheAgentRefusesToEditOrEnableALineNotMadeInCelikPanel`,
  `TestADeleteWritesTheRecordWithoutTheLineBeforeTheCrontab`.
- What the administrator sees: `TestClosingAgainRunsApartFromTheRequestOnceAndOnlyForItsDoor`,
  `TestTheCloseAgainResultSurvivesAPanelRestart`,
  `TestAReadNeverReplacesANewerAnswer`, `TestOneSlowAnswerIsNotDown`,
  `TestASideDoorsStepThatLosesTheAgentEndsAndIsNotLeftRunning`; web
  `side-doors-problems.test.mjs`, `side-doors-undo.test.mjs`.
- The "in progress" correction: the `TestD1…` tests (the webmail update and a
  component install settle by themselves, a row is run again at most once,
  one unanswered renewal does not stop a step, a standalone change follows
  the same rule); web `d1-stuck-operation.test.mjs`.
- Per-site limits preparation: `TestEgressPlaceOrder`,
  `TestAddFailureIsAPlaceProblemOnlyWhenItIs`, `TestProductionPlaceReads`,
  `TestGuardReaddsUnitsInTheirSitesSlice`, `TestProofAcceptsTheSitesSlice`,
  `TestUnitKeepsOnlyItsOwnSitesSliceWithItsFile`,
  `TestPlanAndAccountSavesKeepCeilingsThisReleaseDoesNotKnow`.

The Go suites of the touched packages and the web suite ran on the base and
on this release's code with the same failing set by name (three tests that
read evidence files the test machine's copy leaves out; they pass in CI), and
every check of the release pull request passed in CI. The security part went
through four independent reviews, the other two parts through two each.
