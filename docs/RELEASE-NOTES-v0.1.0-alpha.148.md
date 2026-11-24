# v0.1.0-alpha.148

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.148.tr.md)

**A security release.** Each customer site's programs now run inside limits
of their own: together they can use only the memory, processes and
processor time their site is given, so one site can no longer take the whole
server from the other customers. Database users can get time limits and,
on PostgreSQL, a firm limit on temporary files. The panel's database layout
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

## Security: one site can no longer take the whole server

- **Until now a site's programs were bounded only one by one.** A
  customer's uploaded programs, Python apps, Node.js application and
  service apps (such as n8n), and their install, build and deploy steps,
  each had a limit of its own, but nothing bounded them together. A site
  with several programs, or one that kept starting new ones, could use all
  of the server's memory, processes or processor time, and the other
  customers' sites and the server itself had to wait or were stopped for
  lack of memory.
- **Now every program CelikPanel starts for a site runs in that site's own
  limits**: a native systemd slice, `celiksite_<site id>.slice`, with a
  memory limit (no swap), a process limit and a processor limit for all of
  the site's programs together. When they reach the memory limit the system
  stops one of them; at the process limit no new process starts and the
  running ones go on; at the processor limit they are slowed down. Other
  sites and the server keep what is theirs. A program's own limits stay as
  they were; the site's limits are the outer bound.
- **Where a program goes.** A program written while it does not run (a new
  one, a stopped one, and every install, build or deploy step) goes into its
  site's limits. A program that runs stays where it runs at an ordinary
  save or restart, so nothing is restarted behind anyone's back; the
  administrator's **Move into the limits** brings a site's running programs
  in.
- **The limits and who sets them.** Three plan keys: memory, processes and
  processor of a site's programs together. Without any setting a site gets
  512 processes, 200 % of one processor core and, for memory, the smaller of
  3072 MB and 75 % of the server's memory, so no site takes the whole server
  by default. That memory default is fixed at the panel's first start; if
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
  changed is kept as it is, and the owner's own additions
  (`/etc/systemd/system/celiksite_<site id>.slice.d/*.conf`) are never
  touched; the site's card names them.

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
    `sudo systemctl daemon-reload`; on Debian 13 the new value reaches
    running programs when one of them starts.
  - A program inside its site's limits: the panel has no action that takes
    it out. The server owner does it by hand, as `docs/SITE-SANDBOX.md`
    (section 8.7) describes: `Slice=system.slice` in the program's unit,
    then `sudo systemctl daemon-reload` and a restart of the program.
  - A database user's limits: clearing the key makes the panel remove the
    value it wrote itself; a value the owner set by hand stays the owner's.

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
  the card names them and who acts.
- **On Debian 13 a changed slice file reaches running programs only through
  the panel's own step or a program's start.** After the owner edits the
  file by hand, `sudo systemctl daemon-reload` and up to about two minutes
  pass before the kernel holds it; the card says "written, not in force yet"
  until then.
- **The server's own limits screen does not preview** a lowered site limit:
  lowering the server's ceiling or default of a site key applies without the
  list of sites it would stop.
- **The longest database statement is a default:** the user's own session
  can change it. The temporary-file limit is firm. MariaDB has no
  temporary-file or idle-transaction key.
- **Disk limits are counted, not enforced,** where the hosting file system
  has no project quotas: the panel counts and acts (uploads stop), the file
  system itself does not refuse a write. **Problems now** says so once.
- The open list of alpha.147 stays open
  ([alpha.147 release notes](RELEASE-NOTES-v0.1.0-alpha.147.md)).

## Checked in the lab

**Open.** The lab run on the pair of test servers (Debian 13 and Ubuntu
24.04) has not been made yet.

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
46.

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
  `TestAKeptMoveIsOneLineAndCanBeTriedAgain`.
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
  `site-limits-round3.test.mjs`.

The security part went through three independent reviews and three fix
rounds; every check of the release pull request passed in CI.
