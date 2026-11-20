# v0.1.0-alpha.141

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.141.tr.md)

**A security release: please update soon.** It closes ways in which a
customer could reach the database server's administrator rights, and with
them every other customer's data and the server itself. It also makes every
PostgreSQL database the panel creates UTF-8 and closed to other accounts,
and takes Open WebUI out of the catalogue.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.140

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.140 form),
   so going back to alpha.140 later needs no database restore.
2. There is nothing to press or set after the update.
3. **Going back to alpha.140 opens the closed ways again.** Restores and
   copies would again run with the database server's administrator rights.
   Databases created by this release stay UTF-8 and closed to other
   accounts; alpha.140 does not change them.

## Security: what is closed

- **SQL from a backup, a copy or an import now runs with the rights of the
  database's own owner, through a client that cannot reach a shell or the
  server's files.** Before, a database restore, the database, site and
  WordPress copies and the cPanel import ran the file through the database
  server's administrator account. A backup brought back from a customer's
  own remote destination (which the customer can change), or a cPanel
  archive handed to the administrator, could therefore run commands with
  the administrator's rights. Now such a file can only do what the
  database's own user can do in its own database.
- **The administrator's PostgreSQL sessions inside a customer's database no
  longer run that customer's functions.** The panel's own checks and
  changes inside a database (size, check, adding a user) and every other
  session of the administrator on PostgreSQL now look up names only in
  PostgreSQL's own catalogue, never where the database's owner can place
  something.
- **A new PostgreSQL database no longer accepts connections from other
  accounts' users.** Until now a database kept PostgreSQL's default, which
  lets any database user of the server sign in to it and read its
  structure (not its data). A database the panel creates now lets in only
  its owner and the users added to it on its card.

  **Databases made before this release are still open in that way.** A
  later release closes them, after showing the owner what each would lose.
  Until then the server owner can close one by hand, as root on the
  server, where `name` is the database's name:

  ```sh
  sudo -u postgres psql -c 'REVOKE ALL ON DATABASE "name" FROM PUBLIC;'
  ```

  The database's own user and the users added on its card keep their
  access (checked on PostgreSQL 17); anything else that signs in to it
  with another database user (a tool or role made by hand) is refused
  afterwards. To undo it:
  `sudo -u postgres psql -c 'GRANT CONNECT, TEMPORARY ON DATABASE "name" TO PUBLIC;'`

## What changes for you

### A restore, a copy or an import needs the database user's stored password

The file is loaded as the database's own user, with the password the panel
keeps for that user (an import already worked this way). If the restore
cannot use it, it stops **before anything is written** (before the safety
copy too) and says why, who acts and what to do:

- **The database user has no stored password here, or the database server
  refused it** (it was probably changed outside the panel): the person who
  manages the domain's databases sets a new password for the user on the
  Databases page, then restores again.
- **This server's MariaDB client is too old to load a file safely** (it has
  no sandbox mode; MariaDB 10.5.25, 10.6.18, 10.11.8 or later have it): the
  server administrator updates the server's MariaDB packages, then the
  restore is pressed again.
- **The backup holds something only the database server's administrator
  may create here:** on MariaDB with binary logging switched on and
  `log_bin_trust_function_creators` off, a backup with stored functions or
  triggers; on PostgreSQL, a database user that is itself an administrator
  role. The server administrator decides: allow it on the database server,
  or restore that backup by hand.

The same rules apply to a remote restore; its page says the same.

### What a restore with the owner's rights cannot bring back

Checked with backups made by the panel's own backup code on MariaDB 11.8 and
PostgreSQL 17:

- Everything ordinary comes back: tables and rows, views, triggers, stored
  routines, sequences; on PostgreSQL also schemas, comments, large objects
  and extensions the owner may create.
- On MariaDB, a view, trigger or routine made by another user of the
  database comes back as the database's own user's.
- On PostgreSQL, an extension only the administrator may create is brought
  back only if it is already in the database: a copy into a new database
  that needs such an extension stops, leaves the new database empty and
  names the extension.
- On PostgreSQL, the users added to a database lose their rights on what a
  restore makes again (this was so before as well; see "Known limits").

### A restore without the panel

A backup package's format does not change. The steps for restoring without
CelikPanel (in the remote backups guide) now load each database dump as the
database's own user: `mariadb --sandbox` for MariaDB, and for PostgreSQL a
check for client commands first, then `psql` as the database's user.

### PostgreSQL databases are UTF-8

- **Every PostgreSQL database the panel creates is UTF-8**, whatever the
  server's PostgreSQL templates are, with a language-neutral sorting order.
  A server initialised with the C locale (the owner's Ubuntu 24.04 server)
  used to get SQL_ASCII databases, which keep letters outside English as
  unchecked bytes.
- **A PostgreSQL server the panel installs is UTF-8.** Existing PostgreSQL
  servers and the owner's own settings for new ones are never changed.
- **The database card says each database's text encoding** after its
  creation date ("Text: UTF-8"). A database that is not UTF-8 shows one
  warning on its own card with the way out: if it is empty, delete it and
  create it again; if it holds data, press **Copy**: the copy is UTF-8 with
  all its data and a new name. Point the site to the copy, check it, then
  delete the old one.
- **Copy, import and restore check every letter in a UTF-8 database.**
  Before, a dump of a SQL_ASCII database could put unchecked bytes into a
  UTF-8 database. Now such text stops the copy or import with the
  database server's reason, and nothing changes. A SQL_ASCII database's own
  backup still restores into it exactly as it was.
- **If your server already has a SQL_ASCII database**, Copy leaves the
  original untouched: check the copy (its letters, the number of rows in its
  tables, the site working on it) before you delete the original. This way
  was proven on test database servers, not yet on the lab's servers (see
  "Checked in the lab").

### Open WebUI is withdrawn

Open WebUI is no longer offered or installed (owner, 20 November 2026): in
the lab it needed about 15 GB of free disk while installing and about 2 GB
of memory, could not be stopped part-way and cannot run on Debian 13. An
Open WebUI that a domain already has keeps running; its card says it was
withdrawn and offers stop, start, logs and removal, no update. n8n and the
other apps are unchanged.

## Known limits

- **Moving a database to another PostgreSQL server version** (an
  administrator action) still runs the customer's functions in it with the
  administrator's rights during the move. Until a later release fixes this,
  do not move a database whose owner you do not trust.
- PostgreSQL databases made before this release stay open to other
  accounts' sign-in until a later release closes them (by hand: see above).
- On PostgreSQL, the users added to a database lose their rights on what a
  restore makes again; add them again on the card after a restore.
- MariaDB events (scheduled jobs inside a database) are not in backups.
- A database moved to UTF-8 through **Copy** gets a new name; the site's
  settings must be pointed to it.
- Choosing a sorting language for a new database and converting a database
  to UTF-8 in place come in a later release.
- A package brought back from a remote destination is not marked as made by
  this panel; it can only ever act with its own site's rights.
- Views and triggers of a cPanel import name a database account that is not
  moved here, as before.
- A restore from a remote destination leaves the downloaded package in the
  site's local backup list. Before restoring a changed package of the same
  name from the destination, delete that local copy in the Backups section;
  otherwise the restore is refused with "a different backup with this name
  already exists".

## Checked in the lab

Lab check finished: one run on 20 November 2026 (UTC 15:25 to about 16:55:
the checks took 70 minutes of a 120-minute slot, then the clean-up), on a
pair of test servers: a Debian 13 main DNS server with PowerDNS and an
Ubuntu 24.04 backup DNS server with BIND, started from the alpha.140 state.
The release ran under a lab-only label (v0.1.0-alpha.148) built from this
release's code. Everything was done through the page's own buttons except
where it says the lab did it. No hard failure.

- **Both servers updated through the panel's own update page**, by
  themselves, in about two and two and a half minutes. No service was left
  failed, and both servers answered the same DNS questions the same way
  before and after.
- **A backup package changed at the customer's own remote destination**,
  on Debian 13, MariaDB and PostgreSQL. The lab changed a package the
  customer had sent before the update, and then a fresh one: client
  commands that would run a shell command, statements that would give the
  database user every right on the server or make it a PostgreSQL
  administrator, and a line recording which account ran the file; then
  once with SQL statements only. Every restore through the page was
  refused: nothing ran outside the database (none of the files the commands
  would have made existed), the database users gained no right, the other
  customer's databases were unchanged, and the customer's own database was
  put back from the safety copy each time.
- **The administrator's sessions did not run functions the database's
  owner planted** (PostgreSQL, Debian 13). The lab, as the owner, placed
  functions and operators named like PostgreSQL's own and pointed the
  database's search path at them. The card's Check and Export and the
  card's information ran, and none of them was called by the
  administrator's sessions; the same functions did run in a new session of
  the owner. Adding a user on that database was opened but not completed.
- **A local backup made before the update** restored on both database
  servers with the Turkish letters intact; so did a fresh backup after a
  change.
- **Copy** of a MariaDB and of a PostgreSQL database: all four rows,
  letters intact; the PostgreSQL copy is UTF-8 and only its own user may connect.
  (The first PostgreSQL copy was refused at the plan's database limit, as
  it should be, until a database was deleted.)
- **A restore after the database user's password was changed outside the
  panel** (by the lab) was refused before anything was written, with the
  way out; after **New password…** on the Databases page the restore
  worked.
- **A new PostgreSQL database refuses another account's user** ("User does
  not have CONNECT privilege"), both the other customer's new user and its
  user from before the update; its owner connects and makes a temporary
  table. A database made before the update still lets the other account's
  user in, as "Known limits" says.
- **New PostgreSQL databases are UTF-8**: Turkish letters went in and came
  back, upper case correct; the card says "Text: UTF-8" in Turkish and
  English, without a warning.
- **Open WebUI** is gone from the catalogue and from the Programs section
  (administrator and customer); offering it and installing it, sent
  directly, were refused (withdrawn; unknown application) and nothing was
  queued.
- The usual pages on both servers loaded without an error.

**Not checked, and the owner's own server is in this state: a PostgreSQL
server whose templates are SQL_ASCII** (initialised with the C locale).
Neither lab server had one: Debian 13's PostgreSQL 17 and Ubuntu 24.04's
PostgreSQL 16 were UTF-8. Ubuntu's PostgreSQL 16 was installed in this run
through Components before the update, on alpha.140's code, from the
PostgreSQL project's repository, and its templates came up UTF-8
(C.UTF-8). So creating a UTF-8 database on such a server through the page,
the warning on a SQL_ASCII database's card, and Copy from SQL_ASCII to
UTF-8 (with its stop on a byte that is not UTF-8) ran only in the tests on
private database servers, not in the lab.

Also not checked: MariaDB on Ubuntu 24.04 with a changed package (time; its
MariaDB client, 10.11.14, has sandbox mode); site copy, the customer's
Import, WordPress copy and the cPanel import; users added to a database
(read-only, read-and-write) connecting to a closed database; an Open WebUI
that a domain already has.

### Found in the lab, left for the next release

None of these is a security or data-loss problem.

- A restore refused because the file held a client command or a statement
  beyond the database owner's rights reads on the page as a general
  failure ("The restore did not succeed. The safety backup of the earlier
  state is in the local list."); the reason is kept only in the
  operation's stored detail.
- Every restore from a remote destination leaves the downloaded package in
  the site's local backup list, and a later restore of a package with the
  same name is refused ("a different backup with this name already
  exists") until that local copy is deleted. The first restore of the
  package sent before the update was refused the same way, while the local
  backup it had been sent from was still in the list.
- The refusal for a password the database server does not accept shows a
  technical line in English under the Turkish sentence.
- During a domain deletion the database step read "there was no database
  to process" while the databases were being deleted (already listed from
  the alpha.140 lab; the end result was right).
- The site's Backups section says "what this site's backup will contain
  could not be read", before and after the update.
- Removing PostgreSQL from Components on Ubuntu 24.04 took a second press:
  after the first press of Remove the window stayed open and PostgreSQL
  stayed installed; the second press removed it.

## Evidence

No schema change: schema 079, as in alpha.140. New, all additive: the
restore request's database accounts, a copy's new database account, the
cPanel import's site; the refusal codes `BACKUP_RESTORE_DATABASE_ACCOUNT`,
`BACKUP_RESTORE_CLIENT_UNSAFE`, `BACKUP_RESTORE_NEEDS_ADMINISTRATOR` and
the remote `DATABASE_ACCOUNT`, `DATABASE_CLIENT_UNSAFE`,
`DATABASE_NEEDS_ADMINISTRATOR`; the read-only Agent call
`PostgreSQLDatabaseEncodingsV1`; the catalogue's `withdrawn` mark and
refusal reason.

Tests, each package's full suite in the test machine, before (alpha.140)
and after:

| Package | Before | After |
|---|---|---|
| `internal/backupspec` | 4 | 4 |
| `internal/transport` | 42 | 42 |
| `internal/services` | 244 | 252 |
| `internal/serviceapps` | 18 | 18 |
| `cmd/agent` | 2656 | 2680 |
| `cmd/panel` | 2785 | 2789 |

No new failure. Tests on real database servers (a private MariaDB 11.8.6 and
PostgreSQL 17.12, one cluster UTF-8 and one initialised with the C locale),
run on the code before and after: `TestS1MariaDBRestoreOfAnEditedPackageStaysInsideTheDatabase`,
`TestS1PostgreSQLRestoreOfAnEditedPackageStaysInsideTheDatabase`,
`TestS1CPanelImportDumpStaysInsideTheNewDatabase` and
`TestS1PostgreSQLAdministratorSessionsDoNotRunTheOwnersFunctions` fail
before (the file's command ran as root, the administrator account was
recorded, the user was given every right) and pass after;
`TestS1MariaDBPanelBackupRestoresWithTheOwnersRights`,
`TestS1PostgreSQLPanelBackupRestoresWithTheOwnersRights`,
`TestPGDatabaseIsUTF8OnAClusterWithSQLASCIITemplatesOnRealPostgreSQL`,
`TestANewDatabaseIsClosedToOtherCustomersOnRealPostgreSQL` and
`TestSQLASCIIDatabaseMovesToUTF8ThroughExportAndImportOnRealPostgreSQL`
pass on the release's code. Also `TestEveryPostgreSQLAdministratorSessionHasTheFixedSearchPath`,
`service_apps_withdrawn_test.go`. Web: `npm test` 3067 → 3079 passed, 0 failed; the type check is clean
and the build meets every size budget (first-load path 109.36 KiB of
110.00 KiB compressed, as in alpha.140).
