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

## Checked in the lab

**OPEN: the lab check has not run yet.** It covers both lab servers (Debian
13 and Ubuntu 24.04) and both database servers: a restore of a package
changed at a customer's own remote destination (refused, nothing outside
the database changed); the administrator's database tools and adding a
user on a database whose owner placed functions of its own; an ordinary
backup and restore, database copy, site copy, WordPress copy and cPanel
import, and a backup made before this release; the three refusals; UTF-8
creation and the card's way out on a server initialised with the C locale;
another account's user refused at a new database; Open WebUI withdrawn with
an existing install still running and removable.

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
