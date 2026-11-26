# v0.1.0-alpha.149

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.149.tr.md)

> **Published** on 2026-11-26 (UTC): release commit `c04064161`, GitHub pre-release at 14:21:27Z, download portal at about 14:26Z (public check 27/27).

**A security and correction release.** The panel's agent, its helper on
the server, no longer changes an administrator account of the database
server, nor drops or empties one of the database server's own system
databases, whatever name it is given. The plan's database limits now
reach the PostgreSQL users of sites and every newly created database user. The move of a site's
programs into their limits says truthfully when a program does not run
after it was put back, a stopped program started from the panel starts
inside its site's limits, and no update starts while a site is being moved.
The update now waits about a minute for the panel's agent to start. The
panel's database layout does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.148

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   An update that fails before the new panel has started returns to
   alpha.148 by itself. A completed update is not taken back; see "Going
   back to alpha.148" below.
3. **The update restarts nothing of a customer's and changes no database
   user.** The plan's database limits reach the database users that do not
   have them yet at the next change of the plan's database limits, and a
   new database user gets them when it is created (see "Database limits").
4. **Do not start the update while a site is being moved into its limits:**
   the panel now refuses it and says to press the update again once the
   move's window has finished.

## Security: the panel's agent no longer changes the database server's administrator accounts

- **Until now the panel's agent acted on the account and the database it
  was given.** The agent runs on the server as root and, on the database
  server, as its own administrator. When the panel asked it to change a
  database user (a new password, a suspension, a deletion, removing an
  added user, its limits),
  it checked the form of the name and a short list of reserved names, but
  not what the account could do: an account with administrator rights on the
  database server could be changed, locked or dropped, and a database user
  added to a database owned by such an account received rights on that
  owner's objects. It also dropped or emptied any database it was named,
  the database server's own system databases included (MariaDB `mysql` and
  `sys`, PostgreSQL `postgres`), which removes every account or stops the
  database server from working for the panel.
- **Now the agent asks the database server itself before it changes an
  account,** and again in the same step as the change: an account that holds
  administrator rights, a role that gives them, or rights on the database
  server's own data is refused, and nothing is changed. A database's owner
  passes the same rule; the one exception is CelikPanel's own database
  account while it holds no administrator rights. An account whose state
  cannot be read is refused too. No agent call drops, empties, loads into,
  gives a user or opens or closes a system database (MariaDB `mysql`,
  `information_schema`, `performance_schema`, `sys`; PostgreSQL `postgres`,
  `template0`, `template1`). A MariaDB database is not created with a user
  whose name already exists on any host or as a role.
- **What changes for ordinary work: nothing.** Every database user the
  panel makes for a site is an ordinary account and passes; creating,
  copying, restoring and deleting a site's databases work as before. A
  refusal says in the page's language what was refused, that nothing was
  changed and that the server's administrator decides; it never asks a
  customer to act.
- **What this protects, in plain terms.** On PostgreSQL 16 and later it is a
  real boundary: even a panel taken over by someone else cannot have the
  agent make an account powerful or use one. On MariaDB, and on PostgreSQL
  15 and older, CelikPanel's own database account already holds rights of
  that weight, so there the rule protects against mistakes. Which site a
  database belongs to is checked by the panel.
- **A local MySQL server running in MariaDB's place is not managed.**
  CelikPanel changes database accounts only on MariaDB: on such a server
  every account change is refused with a sentence that says so, and the
  server's administrator makes the change on the server itself. A database
  whose user the administrator removed by hand can then be deleted from the
  panel.

**Removing an added database user no longer hands what it made to the
panel's own database account.** Until now, removing a user added to a
PostgreSQL database that belongs to CelikPanel's own database account handed
the tables, functions and other objects that user had made to that account,
so code a customer wrote could later run with that account's rights. Now
such a user is not removed while it owns tables or other objects in that
database: nothing is changed, and the server's administrator first hands
them to the database's own user on the server (the user guide's Databases
chapter gives the command), then removes the user again from the panel. A
user that owns nothing is removed as before, and on a database that belongs
to a site's own user, what the removed user made goes to that user, as
before.

## Database limits

- **The plan's database limits now reach PostgreSQL.** The longest
  statement, the longest idle open transaction, the temporary files of one
  session and the number of connections now reach the PostgreSQL users of
  sites; until now, on PostgreSQL 16 and later, none of them reached the
  users the panel made for sites, and the temporary-file limit reached them
  on no version. The panel's agent writes them as the database server's own
  role settings, only on ordinary site users the panel has recorded.
- **Every new database user gets them,** whichever way it is made:
  Databases, a user added to a database, applications and one-click
  applications, a WordPress or site copy, the database tools' copy, the
  Database Servers screen, and a move to another PostgreSQL cluster. A
  remote-access copy of a MariaDB user carries its statement time.
- **The server owner's own values are kept.** A value the server owner set
  on a database user by hand, including a PostgreSQL connection limit, is
  not written over and the site's card names it. The site's card says
  separately which users had their own statement or idle time changed on
  the user itself (by the server owner or the user's own session); those
  are defaults the user may change, and CelikPanel does not impose them
  again.
- **When a limit does not reach a user,** **Problems now** names the users
  and says why, each by its own reason: the panel's agent did not answer;
  the database server is not running (the server owner starts it); it is
  running but refused the connection; the user is not an ordinary site
  user; the database server refused the limit; or it is not known yet.
  **Apply again** gives the limits again to the users the line names, once,
  in the background; pressing it again follows the same run. Every hour the
  panel also tries those users again by itself. A plan's Save that changes a
  database limit shows the step and names the users the limits did not
  reach.

## Per-site limits

- **The move says what is true.** When a move into the limits does not
  complete, a program that does not start again where it ran is no longer
  reported as put back: the move keeps its record, and the site's card and
  **Problems now** say that the program is not running, that the
  administrator reads its log, then presses **Try again** or starts it on
  its page, and that nothing else was changed. A program that systemd keeps
  starting again is shown as not running. The move's window, the line about
  limits that did not reach every site and the plan's preview give their
  reasons in the page's language.
- **When a program counts as running again.** In the move's own window and
  at **Try again**, a program counts as running as soon as it runs where it
  should and, if it had ports, listens again. A record that is closed
  afterwards, by a reading of the card or of **Problems now**, closes only
  once the program has run without a break for its start time (90 seconds
  unless the program sets another, at least 20), where it ran, and, if it
  had ports, listens on one.
- **How a kept record ends.** **Try again** puts back and checks what is
  still not running; a program that runs where it ran but listens on no
  port is settled by **Try again** and named. Starting the program on its
  page also settles it once the program has run for its start time, and so
  does a program that starts again by itself: the next reading of the card
  or of **Problems now** (within about five minutes) closes the record. No
  site is moved while a record is kept.
- **The panel's agent never waits for a site's program when it starts:** a
  move it finds unfinished is settled from its record with at most a few
  quick steps, and whether a program runs again is read afterwards. The
  agent writes how long each step of its start takes to its log.
- **A stopped program started from the panel starts inside its site's
  limits,** for an uploaded program, a Node.js site and a service app, once
  this release's sandbox helper is in place; a running program is still
  moved only by the administrator's move.
- **The card says when the site's programs reached their process limit on
  Ubuntu 24.04,** seen between two readings under the same limit; the kernel
  there does not count how often.
- **The line about limits that did not reach every site says which sites
  still have the earlier limits and for which it is not known,** each by its
  own reason. An earlier event on the card is shown with the limit that held
  when it happened, or without a limit when that is not known. A reseller's
  customer's default limit is named as the server's default.
- **No update while a site is being moved, and no move while the update
  runs.** The panel refuses the update during a move and says to press it
  again once the move's window has finished; while an update runs, a move
  and **Try again** are refused with the reason. A record of an update that
  ended without saying so stops refusing moves by itself 31 minutes after it
  was made.
- **The sign-in page's words about a sign-in sent twice** now say what
  happens: the panel uses neither, and a cookie another website set for the
  whole domain stays in the browser and is ignored once you sign in, so the
  notice can come back after signing out.

## Updates

- **The update now gives the panel's agent about a minute to start**
  (before: about twelve seconds). A healthy agent that starts slowly no
  longer stops the update and leaves the panel to the automatic recovery.
  An agent that has not started after a minute is still stopped safely as
  before, and the automatic recovery takes over. An automatic return to the
  earlier release waits the same way for that release's agent.

## Going back to alpha.148

- **An update that fails returns to alpha.148 by itself.** The new panel's
  start check runs before the new panel is started; when it fails, or the
  update stops at any point before that start, the update puts alpha.148
  and its database back by itself. None of this release's changes exist
  then: it has no database step and rewrites no unit, and the panel's agent
  of this release, which carries the database refusals, the database limits
  and the corrections of the move, has not started. Only the update itself
  is of this release: it and its automatic return wait up to about a minute
  for the panel's agent.
- **A completed update is not taken back.** From the moment the new panel is
  started the update is carried to its end, also when that start fails or
  the agent does not start within its minute: then the panel and its
  agent stay stopped, hosted sites keep running, and the automatic recovery
  tries the same update again, at most three times; after that the owner
  finishes it with the recovery command its message names
  (`sudo /usr/libexec/celikpanel/recovery recover …`). The panel
  offers no way back to the release before, and the release's `rollback.sh`
  refuses a completed update before it stops anything, with:
  `!! a new rollback cannot reuse an earlier transaction's recovery material; preserve the snapshot and use a supported recovery plan`.
- **What this release does is undone on alpha.149:**
  - A database user's limits: clearing the key in the plan makes the panel
    remove the values it wrote itself. The limits are the database server's
    own settings and stay in force without the panel; to remove one by hand,
    the server owner runs for example
    `sudo -u postgres psql -c 'ALTER ROLE "user" RESET temp_file_limit'`.
  - The refusals keep no state: there is nothing to undo.

## Known limits

- **A completed update is not taken back** (see "Going back to
  alpha.148").
- **The account rule on PostgreSQL looks at administrator attributes and
  roles.** It does not look at rights the server owner granted by hand on
  single settings, on single administrator functions, or on objects a
  database server administrator runs.
- **Objects that earlier releases already handed to CelikPanel's own
  database account** when an added user was removed stay as they are. The
  database users' documentation (`docs/DATABASE-USERS.md`) gives the
  administrator the query that lists them in a site's database, and how to
  hand each to the database's own user.
- **A user added to a PostgreSQL database gets rights on every table,
  sequence and function of the database's own schemas,** including what the
  server's administrator installed there (for example an extension in a
  site's database); install administrator extensions elsewhere.
- **A local MySQL server in MariaDB's place is not managed** (see
  "Security").
- **Moving a database to another PostgreSQL cluster** recreates the roles
  it needs as the source defines them; only database server administrators
  are refused there.
- **A database connection limit written by an earlier release** is taken as
  the panel's at the next change of that limit (a plan's Save, a move to
  another plan, a reseller's or the server's limit): a user whose value
  equals the previous plan value gets the new one; any other value is kept
  as the server owner's. **Apply again** and the hourly round do not do this,
  and a value the server owner set by hand that equals the previous plan
  value is taken as the panel's.
- **The server owner's connection limit is recognised on CelikPanel's own
  PostgreSQL;** on a PostgreSQL server the administrator registered with an
  account of their own, the plan's value is written as before.
- **A site's database user that its owner made a member of another role, or
  gave administrator rights,** stays named in **Problems now** until that
  role or right is removed, or the user is removed from CelikPanel.
- **A very large Save:** users the limits do not reach within ten minutes
  read "not known yet" with **Apply again**.
- **The database limits line of Problems now appears once its words have
  loaded;** if they cannot be loaded, the line stays hidden.
- **Suspension, password changes and added users wait for each other** in
  the panel's agent, and they have no time limit of their own; a database
  user the server owner holds locked can make them wait. The site's card can
  wait while database limits are being given.
- **If the panel's own records cannot be read to the end,** the database
  limits are refused until the panel's agent restarts.
- **On systems with systemd older than 254 (Debian 12, Ubuntu 22.04), the
  panel's agent, when it keeps failing at its start, is started again more
  often during the update's minute,** and the update's safe stop comes
  about fifty seconds later than before; hosted sites keep running.
- **In a rare timing, a status reading can remove a site's limits file while
  one of the site's programs is being saved;** the site's card then says
  that the limits file is missing, and the next change of the site's limits
  writes it again.
- **A status reading can say "not running" of a program that was just
  restarting;** a later reading closes the record once the program has run
  for its start time.
- **A program that starts, answers and stops a moment later** is reported
  by the move's window as moved, and by **Try again** as put back; the
  site's card then shows that the program is not running.
- **Restarting the server while an update is in progress.** On a server
  that has not been restarted since CelikPanel's firewall rules were first
  saved on it, a restart of the server during the update leaves the update
  unfinished: the panel and its agent stay stopped, neither the automatic
  recovery nor the recovery command finishes it, and hosted sites, mail and
  DNS keep running. Do not restart the server until the update's page says
  that the update has finished.
- **An update page left open through an update that was refused before it
  started** can stay locked in that browser; the update is offered again
  after signing in from another browser.
- The open list of alpha.148 stays open where this release does not close
  it ([alpha.148 release notes](RELEASE-NOTES-v0.1.0-alpha.148.md)).

## Checked in the lab

Lab check finished: one run on the pair of test servers, a Debian 13
server (PostgreSQL 17, MariaDB 11.8) and an Ubuntu 24.04 server (MariaDB
10.11, and PostgreSQL 16 installed for this run from the panel's Components
page), on 26 November 2026, UTC 07:45 to about 11:20 with the clean-up.
Both servers started from alpha.148's code and were updated through the
panel's own update page to this release's code (`578b98e6f`, under the
lab-only labels v0.1.0-alpha.158 to v0.1.0-alpha.160). The updates, the
refusal windows and the usual pages were done on the pages themselves;
database and limit actions were sent as the panel's pages send them, as the
administrator. What the lab did by hand on the servers is said below. What
the lab met is in "Known limits".

- **Both servers updated through the panel's own update page by themselves
  while the panel's agent was held back for 20 seconds at its start** (the
  lab held it): the update waited for the agent, which answered 20.2 and
  20.6 seconds after its start; the page showed only the update's steps and
  then "updated" by itself, about 2 minutes 12 seconds on each server, with
  the administrator signed in throughout. No recovery attempt. No schema
  step (it stays 81); one panel and one agent on each server; no new failed
  service. Nothing of a customer's was restarted or reloaded: every program
  kept its process, the web server and PHP kept theirs, a PostgreSQL and a
  MariaDB session held open across the update on each server stayed
  connected with their settings, no database user was changed, and a check
  every second saw the sites, the mail port and DNS answer throughout. Two
  more updates of the Debian 13 server, between labels of the same code,
  completed the same way without the hold (the agent answered within a
  tenth of a second of its start).
- **An update that fails returned to alpha.148 by itself, on both
  servers.** The lab offered a release whose new panel cannot pass its
  start check, and held alpha.148's agent back for 20 seconds: the return
  waited for it (20.7 and 20.1 seconds) and completed; the page said by
  itself that the update was not completed and that the earlier release was
  put back, with the administrator signed in; every program kept its
  process and the sites answered throughout. On Debian 13 the first
  automatic attempt stopped because the system's own package update had
  just started; the second, half a minute later, completed the return.
- **An agent that does not start within its minute** (Ubuntu 24.04, held
  back for 70 seconds at every start): the update stopped the agent safely
  after 63 seconds; the page said that the panel does not answer and that
  an update may be restarting it, never that the update failed; the
  automatic recovery tried three times, about a minute each, then paused
  and named the owner's command. Sites, mail and DNS answered every second
  throughout. With the hold removed, the named command
  (`sudo /usr/libexec/celikpanel/recovery recover --retry --snapshot …`)
  finished the update.
- **Ordinary database actions, on MariaDB and PostgreSQL, on both
  servers:** creating a database with its user, adding and removing a
  user, changing a password, suspending and resuming the subscription,
  deleting the database with its user. All succeeded; none was refused.
- **The refusal on MariaDB (11.8 and 10.11).** The lab, as the database
  server's root, gave a site's database user in turn a server-wide right, a
  right on the `mysql` database, a role, and an account of the same name
  from any host that holds a server-wide right. Each time the password
  change was refused, naming the user and saying that nothing was changed,
  and the stored password was unchanged; the suspension left that account
  unlocked and said so, while the subscription's other users were locked
  and unlocked as usual; deleting the database removed the database, kept
  the account and said so. After the lab took the right back, each action
  went through. A new database whose user's name already existed as an
  account from another host was refused as a name already taken.
- **The refusal on PostgreSQL (16 and 17).** A site's database user was
  given in turn the right to create databases, membership of
  `pg_read_server_files`, membership without inheritance of a role that may
  create roles, and a chain of memberships ending in
  `pg_read_server_files`. Each time the password change, the suspension
  and the limits were refused for that user by name, and deleting its
  database removed the database and kept the user; its password, its right
  to sign in and its settings were unchanged. After the lab took the right
  back, each went through, and a user without such rights was never
  refused.
- **A database owned by a database server administrator** (the lab made
  `postgres` the owner of a site's database, as a restore that keeps owners
  does): adding a user was refused, naming `postgres`; no user was made and
  no default right was written. The same when the lab gave CelikPanel's own
  database account `pg_read_server_files`: refused, naming that account;
  after the lab took it back, adding and removing a user worked.
- **Removing an added user that owns objects** (PostgreSQL 16 and 17, in a
  database made on the Database Servers page and owned by CelikPanel's own
  database account): the added user made a function that runs with its
  owner's rights, and a table with an index that calls a function of its
  own. The removal was refused in the removal's window, in Turkish and in
  English: the user owns objects, nothing was changed, and the server's
  administrator hands them over with the guide's command. Afterwards every
  object still belonged to the added user, CelikPanel's own account owned
  nothing in the database, and the function and the index's function ran as
  the added user. After the administrator ran the guide's command, the
  removal went through and the objects belonged to the database's own user.
  An added user that had made nothing was removed at once. The database
  server's statement log of the whole check held one `REASSIGN OWNED`: the
  administrator's own. In a database owned by the site's own user, removing
  an added user that had made a table gave the table to that owner as
  before, with its rows.
- **CelikPanel's own PostgreSQL account** after all of this, on both
  servers: not a superuser, no replication right, no membership of any
  `pg_*` role, no right on single server settings.
- **Database limits reach PostgreSQL users,** on both servers: a plan saved
  with a longest statement of 5 seconds, 60 seconds idle in a transaction,
  64 MB of temporary files and 7 connections wrote all four to the site's
  PostgreSQL users, and a session signed in as such a user read
  `temp_file_limit` as 64MB. On Ubuntu 24.04 a plan's connection limit
  saved under alpha.148 (it had reached the MariaDB user only) and changed
  from 10 to 20 after the update reached the PostgreSQL user (20, before:
  none) and the MariaDB user. A user created afterwards had the plan's
  limits from the start. A user the lab made a member of another role was
  left out and named, and the other users got their values.
- **The server owner's own values stay,** on both servers: a statement time
  of 30 seconds set by hand on a PostgreSQL user and on a MariaDB user, and
  a connection limit of 25 set by hand, stayed through saves that changed
  those limits for the other users; the site's card named each as the
  owner's.
- **When the limits cannot be given** (Ubuntu 24.04): with PostgreSQL
  stopped by the lab, the save said which users the limits did not reach
  and **Problems now** said that the database server is not running and
  that the server owner acts; with the panel's agent stopped, it named the
  administrator. After each, **Apply again** cleared the line. With a
  database user held locked by an open transaction of the lab, the limits
  call gave up after 5 seconds and the save said so; another subscription's
  suspension completed in 4 seconds meanwhile, and the user's settings were
  unchanged.
- **The panel's database file** stayed the panel's own, with owner and
  access rights unchanged, while the agent read it; saves worked after a
  restart of the panel.
- **A stopped program started from the panel** (uploaded programs, both
  servers) started inside its site's limits, with its mail block. With the
  server owner's own override file, or with a unit file edited by hand, it
  started where the owner had placed it, the files stayed as they were, and
  the card gave the reason.
- **A move that did not complete, and the agent's start** (Debian 13). With
  a program the lab made unable to start, the move answered after the
  program's start time that it was not completed and that the program is
  not running; the record was kept and the card said so. Three restarts
  of the agent with that record: it answered after 2.6 to 2.9 seconds each
  time, its look at the record took 14 to 25 thousandths of a second, and
  it started nothing. An update with the record kept completed by itself,
  and the record was still kept afterwards. Once the lab let the program
  start, **Try again** closed the record. With a program that takes 30
  seconds to listen, the agent was restarted 12 seconds into the move: one
  reload and one restart of the program, the agent answered 1.5 seconds
  after its start, and the card kept the record as not yet known until the
  program had run for 90 seconds, then closed it by itself.
- **No move while the update runs** (Debian 13): **Move** and **Try again**
  were refused with the reason while an update ran; the cards could be read
  throughout; after the update the move answered.
- **The limit an event happened under** (Debian 13): a plan's site memory
  was lowered to 128 MB, the kernel stopped a program over it, and the
  default was restored before anyone read the card; the card showed the
  event with 128 MB, for the administrator and for the customer's account.
- After the updates the usual pages loaded on both servers, and the
  database tools opened through the panel.

**Where the lab left the page's path.** As root on the lab servers, the lab
gave and took back database rights and roles, changed a database's owner,
set database values by hand, turned the database server's statement log on
and off, held a database user locked, held the panel's agent back at its
start, restarted and stopped the agent, the panel and PostgreSQL, copied
its test program into the sites' folders and made it unable or slow to
start, wrote the server owner's override file and edited a unit file,
restarted the Ubuntu 24.04 server during an update, ran the recovery
command, returned the Ubuntu 24.04 server to its saved alpha.148 state
three times, and cleaned up at the end.

**Not checked on real servers:**

- Copying a database between two MariaDB servers (remote database access
  was off on the lab servers).
- A connection limit that alpha.148 wrote for a user made on the Database
  Servers page, at the next change of that limit.
- Starting a stopped Node.js site or service app from the panel.
- A move that does not complete, and the agent's start with a kept record,
  on Ubuntu 24.04.
- Pressing the update while a move runs, a restart of the server during a
  move, and the agent restarted during the move of a program that keeps
  stopping.
- A program started on its own page while a record is kept; the words for
  a program that systemd keeps starting again; a program that listens on
  another port, on IPv6 only, or on no port after a move that did not
  complete.
- The line about limits that did not reach every site, with one site whose
  limits are not known.
- Database limits refused by PostgreSQL's own access rules; the panel
  restarted in the middle of a large save; the hourly round.
- The process-limit event on Ubuntu 24.04, the sign-in page's new words
  about a sign-in sent twice, and a start with limits smaller than the
  program needs.
- alpha.148 started on this release's panel database.
- Webmail after the update.
- Nothing was tried on Arch Linux.

## What to expect after updating

- No schema step: the layout stays schema 81. A completed update is not
  taken back (see "Going back to alpha.148").
- **This release restarts and reloads nothing of a customer's at the update
  or at the panel's first start:** no program, no PHP pool, no web server,
  no database session. A move record kept from alpha.148 is only read at
  the agent's start; no program is started or restarted for it. (The one
  exception: a move that alpha.148 left with the new limits file still in
  place, because its put-back could not write the earlier file; there the
  earlier file is put back and the programs that ran before that move are
  restarted once, as alpha.148 would have done.)
- **No database user is changed at the update.** When the plan's database
  limits are next written (a change of the plan's database limits, a new
  user, **Apply again**), the database server applies them to sessions that
  start afterwards: open sessions keep their settings, and a new connection
  limit refuses new connections over it without ending open ones.
- **The update itself:** while it runs the panel is not available, for up to
  about a minute longer than before when the panel's agent starts slowly.
  Hosted sites, mail, DNS and databases keep running throughout. Do not
  restart the server while the update runs (see "Known limits").
- A status reading that closes a kept move record puts the site's earlier
  limits file back and reloads systemd's configuration; no program is
  restarted by it.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New and additive: the agent call
`SetPostgreSQLRoleLimitsV1`; the refusal codes `absent`, `privileged`,
`member`, `owns_objects`, `unreadable`, `system_database` and
`unsupported_engine`
(`DATABASE_ACCOUNT_REFUSED`, 409); the route
`POST /api/v1/metrics/database-limits/apply` (administrator) with the audit
action `database.limits.apply_again`; Problems now kind 47
(`database_limits_unapplied`); the panel settings `db_limits_unapplied_v1`
and `site_move_stopped_v1`; the update refusal
`PANEL_UPDATE_SITE_MOVE_RUNNING`.

Tests by part:

- The account rule: `TestTheAccountRuleCoversEveryPrivilege`,
  `TestAnUnreadableAccountStateRefuses`,
  `TestPasswordRotationRefusesAPrivilegedAccount`,
  `TestSuspensionRefusesAPrivilegedAccount`,
  `TestDeletingADatabaseNeverDropsAPrivilegedUser`,
  `TestRemovingAnAddedUserNeverTouchesAPrivilegedAccount`,
  `TestAddingAUserRefusesADatabaseOwnedByAPrivilegedRole`,
  `TestRemovingAUserNeverHandsItsObjectsToAPrivilegedOwner`,
  `TestRemovingAUserNeverHandsAnythingToCelikPanelsOwnAccount`,
  `TestARefusalIsReadOnlyFromTheClientsOwnErrorLine`,
  `TestTheOwnerRuleOfCelikPanelsOwnAccount`,
  `TestNoAgentCallDropsOrEmptiesASystemDatabase`,
  `TestAMariaDBCreateNeverMakesAnAccountBesideOneOfTheSameName`,
  `TestMySQLInMariaDBsPlaceIsNamedNotUnreadable`; on real engines
  `TestTheAccountGuardOnRealEngines`; web `db-account-refusal-a149`.
- Database limits: `TestEveryPathThatCreatesADatabaseUserAppliesItsLimits`,
  `TestANewDatabaseUserGetsEveryLimitThePlanSets`,
  `TestPGRoleLimitsOutcomes`, `TestTheOwnersPostgreSQLConnectionLimitIsKept`,
  `TestALimitEarlierReleasesWroteFollowsTheNextChange`,
  `TestApplyAgainNeverStartsTwice`, `TestTheHourlyRoundIsOneBoundedRun`,
  `TestARestartLeavesARunsUsersNotKnownYet`; on a real engine
  `TestPGRoleLimitsOnARealEngine`; web `db-limits-a149`,
  `site-limits-db-origin-a149`.
- Per-site limits: `TestTheAgentsStartIsBoundedByItsReadsAndOneReload`,
  `TestTheStartsSystemdCallsShareOneBound`,
  `TestAReadClosesOnlyAProgramThatReallyRuns`,
  `TestStartingItOnItsPageSettlesTheRecord`, `TestOnePressHasOneDeadline`,
  `TestTryAgainSettlesAProgramThatRunsWithoutAPort`,
  `TestAStaleUpdateRecordNeverRefusesMovesForGood`,
  `TestStatusReadsNeverRefuseTheUpdateOrAMove`,
  `TestTheUpdateIsNeverHeldBehindAMovesRecordLock`,
  `TestAStoppedProgramStartedFromThePanelStartsInsideItsLimits`,
  `TestAReachedProcessLimitSeenByThePeak`,
  `TestTheUnappliedLineWordsEachSiteByItsReason`; web `site-limits-a149`,
  `site-limits-a149-fix2`.
- Updates: `deploy/test-bootstrap-update-contract.sh`,
  `deploy/test-finalize-pending-update-contract.sh`,
  `deploy/test-finalize-pending-rollback-contract.sh`,
  `deploy/test-abort-pre-mutation-active-update-contract.sh` (each pins its
  wait).
