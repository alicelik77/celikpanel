# v0.1.0-alpha.148

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.148.tr.md)

**A security release.** A customer site's programs (uploaded programs,
Python apps, the Node.js application, service apps, and their install, build
and deploy steps) now run inside limits of their own: together they can use
only the memory, processes and processor time their site is given, so a
site's programs can no longer take the whole server from the other
customers. A site's PHP keeps its own limits per site, as before, and is not
inside the site's limits; its scheduled tasks are not inside them in this
release; programs that ran before the update are inside once the
administrator moves the site. Database users can get time limits and, on
PostgreSQL, a firm limit on temporary files. The panel's sign-in cookies can
no longer be set, shadowed or replaced by a website on another host name
under the same domain, and signing out after "Log in as" no longer leaves
the administrator's own sign-in in the browser. The panel's database layout
does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.147

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   An update that fails before the new panel has started returns to
   alpha.147 by itself. A completed update is not taken back; see "Going
   back to alpha.147" below.
3. **The update moves nothing and restarts nothing of a customer's.** Every
   program that runs keeps running where it runs, outside its site's limits,
   until the administrator moves the site. At its first start the panel
   fixes the default memory limit of a site for this server's memory (see
   below). Within about five minutes **Problems now** shows the
   administrator one line, "Sites whose programs run outside their limits",
   and each such site's Overview offers **Move into the limits**. That move
   is the administrator's step, site by site, when it suits the site: it
   restarts the site's running programs once (see "What to expect after
   updating").
4. **An administrator who follows the update on its page stays signed in.**
   Resellers, customers and team members who were signed in at the time of
   the update sign in once (see "Security: sign-in cookies that other
   websites cannot set").

## Security: a site's programs inside limits of their own

- **Until now a site's programs were bounded only one by one.** A
  customer's uploaded programs, Python apps, Node.js application and
  service apps (such as n8n), and their install, build and deploy steps,
  each had a limit of its own, but nothing bounded them together. A site
  with several programs, or one that kept starting new ones, could use all
  of the server's memory, processes or processor time, and the other
  customers' sites and the server itself had to wait or were stopped for
  lack of memory.
- **Now a site's programs run inside that site's own limits**: its uploaded
  programs, Python apps, Node.js application and service apps, and their
  install, build and deploy steps, run in a native systemd slice,
  `celiksite_<site id>.slice`, with a memory limit (no swap), a process limit
  and a processor limit for all of them together. When they reach the memory
  limit the system stops one of them; at the process limit no new process
  starts and the running ones go on; at the processor limit they are slowed
  down. Other sites and the server keep what is theirs. A program's own
  limits stay as they were; the site's limits are the outer bound. A site's
  PHP-FPM pool keeps its own PHP limits, as before, and is not inside the
  site's limits; its scheduled tasks are not inside them in this release.
- **Where a program goes.** A program written while it does not run (a new
  one, a stopped one, and every install, build or deploy step) goes into its
  site's limits. A program that runs stays where it runs at an ordinary
  save or restart, so nothing is restarted behind anyone's back; the
  administrator's **Move into the limits** brings a site's running programs
  in. A program inside its limits stays inside at every save, also when the
  system cannot say at that moment where it runs.
- **The limits and who sets them.** Three plan keys: memory, processes and
  processor of a site's programs together. Without any setting a site gets
  512 processes, 200 % of one processor core and, for memory, the smaller of
  3072 MB and 75 % of the server's memory, so by default a site's programs
  never get all of the server's memory. That memory default is fixed at the panel's first start; if
  the server's memory changes later, **Problems now** says so and offers the
  new default after a preview. The administrator sets the keys in plans,
  reseller plans, an account's own limits and the server's limits; a
  reseller stays within the server's default unless the administrator gives
  the reseller a higher limit.
- **Lowering a limit below what a site uses asks first.** The preview of a
  plan, a reseller plan or an account's own limits lists the sites whose
  programs use more than the new value and says what happens: a program over
  the memory limit is stopped at once; at the process limit new processes
  are refused and running ones go on. The server enforces it: such a change
  is saved only with that list confirmed.
- **Database users.** Three optional keys: the longest database statement
  (a default the user's own session can change), the longest idle open
  transaction (PostgreSQL, also a default) and the temporary files of one
  PostgreSQL session (a firm limit). Restores, copies and imports keep the
  user's limit: a load that needs more temporary space is rolled back whole
  and names the limit and who can raise it. A value the server owner set on
  a database user by hand is kept and shown to the administrator as the
  owner's; the panel changes only what it wrote itself.
- **What people see.** The site's Overview shows the site's limits, whether
  they apply, what the programs use now and, in plain words, when they
  reached a limit; a customer reads that the provider can raise it. The
  administrator also sees where each program runs and the move. **Problems
  now** names, for the administrator, the sites still to move, a site that
  keeps reaching its limits, a change of limits that has not reached every
  site yet (the panel writes those sites again every hour), a move that did
  not finish, and a server that cannot hold the limits.
- **The conditions.** The limits need the system's unified control groups
  (cgroup v2) with their memory, process and processor controllers, as
  Debian 13 and Ubuntu 24.04 have them. Where a server lacks them nothing is
  limited, and **Problems now** says what is missing. The limits are native
  systemd files: they keep working with the panel stopped, removed or
  without a licence, also across reboots. A slice file the server owner
  changed and the owner's own additions
  (`/etc/systemd/system/celiksite_<site id>.slice.d/*.conf`) are never
  changed while the site exists; the site's card names them. When the site
  is deleted, or a new site gets the id of an earlier site whose files
  CelikPanel does not know, they are moved aside in the same folder under
  names systemd does not load
  (`celiksite_<site id>.slice.celikpanel-aside-<UTC time>` and
  `celiksite_<site id>.slice.d.celikpanel-aside-<UTC time>`); a deletion
  names them in the activity list.

## Security: sign-in cookies that other websites cannot set

- **Until now the panel's sign-in cookies had plain names.** A browser
  also sends the panel a cookie that a website on another host name under
  the same domain set for the whole domain (for example a site at
  `shop.example.com` when the panel is at `panel.example.com`). Such a
  website could put a sign-in of its own choosing into a visitor's browser,
  which the panel then read before its own, so that the visitor worked in
  another account without noticing; and it could put a sign-in behind
  **Return to my account**.
- **Now the panel's sign-in cookies carry names a browser keeps only for the
  panel's own host name**, over HTTPS and never for a whole domain
  (`__Host-celikpanel_session` and, during "Log in as",
  `__Host-celikpanel_impersonator`). A website on another host name under
  the same domain can no longer set, shadow or replace the panel's sign-in.
  If a browser still sends the sign-in twice, the panel uses neither,
  removes both, and the sign-in page says why. A signed-in browser's request
  to the panel that comes from another website, or from another address of
  the same site, is refused; the panel's own pages never send one.
- **"Log in as", then "Sign out".** Until now, signing out of a "Log in as"
  session ended only that session: the browser kept the administrator's own
  sign-in for **Return to my account**, and the next person who signed in on
  that browser, before it was closed, could press **Return to my account**
  and continue as the administrator. Now **Sign out** during "Log in as"
  also ends the administrator's own session in that browser; **Return to my
  account** is offered and works only inside a "Log in as" session of that
  same administrator; and every sign-in removes an administrator's sign-in
  left in the browser.
- **When a "Log in as" session ends any other way** (ended from a sessions
  list, the account suspended or deleted, the session idle for too long, or
  the administrator no longer allowed to manage the account), the browser no
  longer keeps the administrator's own sign-in. Where the account may no
  longer be used that way, the administrator's own session in that browser
  ends too and the sign-in page says why; until now the panel refused every
  request there, even **Sign out**.
- **The database tools no longer receive the panel's sign-in.** What the
  panel passes on to a database tool through its own address carries none of
  the panel's sign-in cookies and no `Authorization` header, and a tool can
  neither set cookies under the panel's names nor clear the panel's cookies.
- **What people notice.** An administrator who was signed in when the update
  ran stays signed in: the update page follows the update to its end by
  itself, as before. Resellers, customers and team members who were signed
  in at the time of the update, and an administrator who was inside "Log in
  as" then, sign in once; so does anyone who comes back more than 24 hours
  after the update. Whenever the panel asks for a new sign-in, or the
  browser did not keep one (for example because a proxy in front of the
  panel changes its cookies), the sign-in page says why and what to do.
- **One host name for the panel.** Every service on the panel's own host
  name shares its cookies, whatever the port, so that host name should serve
  the panel alone.

## Going back to alpha.147

- **An update that fails returns to alpha.147 by itself.** The new panel's
  start check runs before the new panel is started; when it fails, or the
  update stops at any point before that start, the update puts alpha.147
  and its database back by itself. None of this release's changes exist
  then: it has no database step, the update rewrites no program's unit, and
  the panel's agent writes a site's limits and its own helper only after it
  has started.
- **A completed update is not taken back.** From the moment the new panel is
  started the update is carried to its end, also when that start fails. The
  panel offers no way back to the release before, and the release's
  `rollback.sh` refuses a completed update before it stops anything, with:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
  The update keeps its snapshot. `sudo /usr/libexec/celikpanel/recovery recover`
  finishes an update or rollback that was interrupted; after a completed
  update it has nothing to do.
- **What this release does is undone on alpha.148:**
  - A site's limits: clearing a key in the plan or the account brings the
    site back to the default. To lift or change them without the panel, the
    server owner adds a file of their own, for example
    `/etc/systemd/system/celiksite_<site id>.slice.d/owner.conf` with
    `[Slice]` and `MemoryMax=infinity`, and runs
    `sudo systemctl daemon-reload`. After `sudo systemctl daemon-reload`, an
    owner's change reaches running programs at once on Ubuntu 24.04 and
    about a minute and a half later on Debian 13, without any program
    starting; a program's start applies it at once.
  - A program inside its site's limits: the panel has no action that takes
    it out. The server owner adds a file of their own beside the program's
    unit, the same on Debian 13 and Ubuntu 24.04:
    `/etc/systemd/system/<unit>.service.d/slice.conf` with the two lines
    `[Service]` and `Slice=system.slice`, then
    `sudo systemctl daemon-reload` and `sudo systemctl restart <unit>`
    (`<unit>` is the program's name on its card, such as `celikrt-12`). The
    panel keeps managing the program: the site's card tells the
    administrator that the server owner placed it with an addition of their
    own, a move leaves it where it is, and saving or updating it in the
    panel never changes that file. Removing the file, then the same two
    commands, brings it back inside. Do not change CelikPanel's own unit
    file for this: a changed unit file is kept as the server owner's, and
    the panel then no longer writes that program's unit (a service app's
    next update is refused until the change is moved into such a file).
  - A database user's limits: clearing the key makes the panel remove the
    value it wrote itself; a value the owner set by hand stays the owner's.
- **The sign-in cookies need nothing undone.** Nothing of that part exists
  at an automatic return: only the new panel, once started, sets the new
  cookie names and records when it first did. Nothing of it is undone by
  hand. A panel of an earlier release does not read the new names; there,
  people simply sign in again.

## Known limits

- **A completed update is not taken back** (see "Going back to
  alpha.147"); this release's changes are undone on alpha.148.
- **Programs that ran before the update stay outside their site's limits
  until the administrator moves the site,** and such a program starts
  outside them again after every restart of the server until it is moved.
- **Outside a site's limits:** its PHP (each site's PHP-FPM pool, which
  keeps its own PHP limits), its scheduled tasks, and user managers a site
  account keeps running.
- **The move restarts each running program of the site once** and needs it
  to run and answer again inside the limits within its start time (90
  seconds unless the program sets another, at least 20 and at most 300);
  one that does not is put back where it ran, with its earlier file. One
  site is moved at a time, at most 64 programs in one move; while a site is
  moved, changes to that site wait or are answered "being changed".
  Programs whose file the server owner changed, programs of an earlier
  protection and programs an owner's file places elsewhere are not moved;
  the card names them and who acts. A move that a stop of the panel's agent
  interrupted is finished or put back when the agent starts again; a
  put-back program that is not running again within a few seconds is named
  on the site's card with **Try again**, so the agent's start never waits
  for it.
- **On Debian 13 a slice file the owner changes by hand does not reach
  running programs at once** (the panel's own changes do, in the same
  step). After `sudo systemctl daemon-reload`, an owner's change reaches
  running programs at once on Ubuntu 24.04 and about a minute and a half
  later on Debian 13, without any program starting; a program's start
  applies it at once. The card says "written, not in force yet" until then.
- **The server's own limits screen does not preview** a lowered site limit:
  lowering the server's ceiling or default of a site key applies without the
  list of sites it would stop.
- **The longest database statement is a default:** the user's own session
  can change it. The temporary-file limit is firm. MariaDB has no
  temporary-file or idle-transaction key.
- **Disk limits are counted, not enforced,** where the hosting file system
  has no project quotas: the panel counts and acts (uploads stop), the file
  system itself does not refuse a write. **Problems now** says so once.
- **The panel's own host name is shared by every port.** A website or
  service on that host name, on any port, receives the panel's sign-in
  cookies and can set them; the new names do not separate ports. That host
  name should serve the panel alone.
- **For 24 hours after the update** an administrator's sign-in from before
  the update is still accepted under the earlier cookie name and moved to
  the new name; every other sign-in from before the update is asked to sign
  in again.
- **When a "Log in as" session is ended from a sessions list, or by
  suspending or deleting the account,** the browser no longer keeps the
  administrator's own sign-in, but that sign-in stays valid on the server
  until it ends by itself (at most 24 hours after the administrator signed
  in); no browser holds it any more.
- **A website under the same domain can still fill the browser's cookies**
  for that domain (which can make requests to the panel too large or push
  cookies out); it can no longer set or replace the panel's sign-in.
- **A session that ended because it was idle** shows the general "your
  session ended" text on the sign-in page.
- The open list of alpha.147 stays open
  ([alpha.147 release notes](RELEASE-NOTES-v0.1.0-alpha.147.md)).

## Checked in the lab

**Open:** the run on the pair of test servers (Debian 13 and Ubuntu
24.04).

## What to expect after updating

- No schema step: the layout stays schema 81. A completed update is not
  taken back; this release's changes are undone on alpha.148 (see "Going
  back to alpha.147").
- **This release restarts and reloads nothing of a customer's at the update
  or at the panel's first start:** no program, no PHP pool, no web server.
  Programs keep running where they run, outside their site's limits.
  (alpha.147's step about three minutes after each start runs again; where
  its work is done it moves nothing.)
- **Setting or changing a limit restarts nothing.** Programs already inside
  their site's limits take a new value at once, without a restart. Raising
  a limit interrupts nothing. Lowering one below what a site uses now stops
  a program over the new memory limit at once, after the preview has listed
  that site and the change was confirmed; at a lowered process limit only
  new processes are refused.
- **Move into the limits restarts each running program of that site once,**
  one after the other: each program is down for its own restart, normally a
  few seconds, and for at most its start time (90 seconds unless it sets
  another, at most 300) before it is put back where it ran. Nothing of the
  site's PHP, its pool or the web server is restarted or reloaded. To choose
  the moment, move a site at a quiet hour for it.
- A new program, a program saved while it does not run, and every install,
  build or deploy step now run inside the site's limits: a step
  that needs more memory than the site has (3072 MB or 75 % of the server's
  memory by default) is stopped by the system, and the card says which
  limit it met.
- **Problems now** shows the administrator "Sites whose programs run outside
  their limits" until every site is moved.
- **Sign-in.** An administrator who was signed in when the update ran stays
  signed in, and the update page shows the update's end by itself.
  Resellers, customers and team members who were signed in then, and an
  administrator who was inside "Log in as", see the sign-in page at their
  next click, with the reason that the sign-in is from before the update,
  and sign in once. Nothing of a customer's sites, mail or databases is
  touched by this.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New and additive: the agent calls `SiteSlicesStatusV1`,
`ApplySiteSlicesV1` and `MoveSiteSliceV1`; the routes
`GET /api/v1/domains/{id}/site-limits`,
`POST /api/v1/domains/{id}/site-limits/move` (administrator) and
`POST /api/v1/rights/site-memory-default` (administrator); the plan keys
`site.memory_mb`, `site.tasks`, `site.cpu_percent`, `db.statement_seconds`,
`db.idle_in_transaction_seconds`, `db.temp_file_mb`; the audit actions
`site.limits_move` and `site.limits_file_aside`; Problems now kinds 39 to
46. For the sign-in cookies: the cookie names `__Host-celikpanel_session`
and `__Host-celikpanel_impersonator` over HTTPS; one panel setting,
`auth.signin_cookie_names_since`, written once; the answers
`SIGNIN_COOKIES_DUPLICATE`, `SIGNIN_FROM_BEFORE_UPDATE` and
`LOG_IN_AS_ENDED` (401) and `CROSS_SITE_REQUEST_REFUSED` (403).

Tests by part:

- Where a site's limits come from and what counts as reaching them:
  `TestSliceCountersTellTheSitesLimitApart` (counter files a real Debian 13
  and Ubuntu 24.04 kernel wrote), `TestOnlyTheSitesOwnLimitBlamesThePlan`,
  `TestTheStatusReadsTheKernel`, `TestTheCardShowsWhatTheKernelHolds`,
  `TestTheMemoryDefaultIsBoundToTheHost`, `TestTheDefaultDoesNotMoveByItself`,
  `TestAResellersSiteLimitsAreBoundedByDefault`,
  `TestAResellerCannotSetASiteLimitAboveItsOwn`.
- The move and its record: `TestMoveSiteSliceMovesRunningUnitsOnce`,
  `TestAMoveSurvivesAnAgentStopAtEveryStep`,
  `TestTheMoveIsRecordedBeforeEveryWrite`,
  `TestAFullDiskAtAnyMoveRecordStopsTheMove`,
  `TestARecoveryThatCannotRestartKeepsItsRecord`,
  `TestAnUnknownPlaceOrAFailedReloadRestartsNothing`,
  `TestTheMovesRestartIsBoundedByItsStartWait`,
  `TestAMoveTakesEachProgramsLockAndLosesNothing`,
  `TestAMoveReadsDropInsAndWaitsAsTheUnitDoes`,
  `TestAKeptMoveIsOneLineAndCanBeTriedAgain`,
  `TestAKeptMoveNeverHoldsUpTheAgentsStart`.
- Where a saved program goes when systemd cannot say where it runs:
  `TestARuntimeAppSavedWhileItsPlaceIsUnknownKeepsItsLimits`,
  `TestANodeSiteSavedWhileItsPlaceIsUnknownKeepsItsLimits`,
  `TestAServiceAppSavedWhileItsPlaceIsUnknownKeepsItsLimits`.
- Applying limits: `TestALoweredSiteLimitAsksFirst`,
  `TestALoweringWithoutTheListIsRefused`,
  `TestARunningSliceTakesTheNewLimitAtOnce`,
  `TestAPushThatDoesNotTakeIsAFailedOutcome`,
  `TestALimitChangeThatDidNotReachEverySiteIsALine`,
  `TestFiveThousandSitesAreReadAndWritten`,
  `TestNothingIsWrittenOnAHostWithoutLimits`, `TestADeletedSitesSliceGoes`.
- The mail block and the place of a program: `TestEgressPlaceOrder`,
  `TestEgressFollowsTheSitesSlice`, `TestProofAcceptsTheSitesSlice`,
  `TestTheAgentsPlaceReadingIsTheHelpers`,
  `TestGuardReaddsUnitsInTheirSitesSlice`.
- Database users: `TestAPostgreSQLLoadNeverLiftsTheTemporaryFileLimit`,
  `TestTheCustomersPostgreSQLImportFollowsTheSameRule`,
  `TestADatabaseUsersOwnSettingsSurvive`, engine tests
  `TestR2aPostgreSQLTemporaryFileLimitHoldsDuringARestore` and
  `TestR2aMariaDBStatementTimeDoesNotCutARestore`; web
  `site-limits-r2a.test.mjs`, `site-limits-round2.test.mjs`,
  `site-limits-round3.test.mjs`, `site-limits-a148.test.mjs`.
- Sign-in cookies: `TestSigninCookiesUseHostPrefixOverHTTPS`,
  `TestSigninDuplicateSessionCookiesAreRefusedAndCleared`,
  `TestSigninPlainCookieBesideTheNewNameIsIgnoredAndCleared`,
  `TestSigninFetchMetadataGate`, `TestCSRFBlocksOriginThatDiffersOnlyByPort`.
- The update's sign-in: `TestSigninTransitionCarriesAPreUpdateAdministratorSessionOnce`,
  `TestSigninTransitionCarriesOverOnlyOnAPIAndNeverCached`,
  `TestSigninTransitionRefusals`,
  `TestSigninCookieNamesSinceIsRecordedOnceAndKeptAcrossRestarts`.
- "Log in as": `TestSigninLogInAsAndReturnUseTheNewNames`,
  `TestSigninLogInAsThenSignOutLeavesNoWayToTheOperator`,
  `TestSigninReturnCookieBesideAnOrdinarySessionLeadsNowhere`,
  `TestSigninLogInAsEndedAnyOtherWayLeavesNoUsableTokenInTheBrowser`,
  `TestSigninRefusalsAndTheReturnCookie`.
- The database tools: `TestDBToolProxyStripsPanelCredentialsBothWays`,
  `TestDBToolProxyKeepsThePanelsSecurityHeadersOverPlainHTTP`; the sign-in
  page's reasons: web `access-hold-runtime.test.mjs`.
