# v0.1.0-alpha.140

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.140.tr.md)

A small fix release of four jobs carried over from the alpha.138 and
alpha.139 labs. The most important one closes a data-loss risk: pressing
**Delete** again on a database could delete a different database that had
been made with the same name in the meantime. The others make three screens
say what is really happening: a domain deletion that waits for the backup
DNS server, the busy end of **Add…** for webmail's record, and the AI and
automation apps that cannot be installed yet.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.139

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.139 form),
   so going back to alpha.139 later needs no database restore.
2. There is nothing to press or set after the update.
3. **What the new saved notes mean if you go back.** A database deletion now
   keeps a small note in the panel's settings while it runs (one entry per
   database being deleted, named `database_delete_v1:` with the database's
   number). It is removed when the deletion finishes. If you go back to
   alpha.139 while such a note exists, alpha.139 ignores it: a deletion there
   behaves as it did before this release. The note harms nothing (database
   numbers are never used twice), and this release removes any note left
   behind the next time it deletes a database.

## What changes for you

### A database is deleted once, and only the one you asked for

Deleting a database takes two steps: the database server drops the
database, then CelikPanel removes its entries (the database, its users, their
rights). Sometimes the second step does not follow the first: the answer of
the database server is lost (a timeout, a restart, a closed page), one of the
database's users or rights could not be removed yet, or the entries could
not be written. The database then stays in the list, and the screen says to
press **Delete** again.

Until now every press sent "drop the database with this name". If, between
the two presses, a database of the same name was made on the server, the
second press dropped that new database with everything in it. That can
happen without anyone meaning harm: the customer's own database user keeps
its rights on that name until the deletion finishes, so an application or
an installer using it can make the database again; the owner can make it by
hand, or restore it from a backup. The same was true when a domain deletion
was asked to delete the domain's databases and was pressed again.

Now:

- **Before the first drop, the deletion notes which database it is.** It
  reads the database's identity on the database server (on PostgreSQL its
  internal number; on this server's MariaDB the identity of the folder the
  database lives in) and saves it before anything is dropped.
- **The database server drops it only while it is that same database.** A
  later press after a lost answer either finds it gone and finishes the
  entries, or finds the same database and drops it, or finds another one
  and leaves it alone.
- **Once the database is gone, a later press never drops anything again.**
  It only finishes the users and the entries.
- **A database of the same name made meanwhile is left as it is**, with the
  users that may use it. The screen says so: CelikPanel does not delete it;
  the server administrator decides about it on the database server, and then
  **Delete** finishes.
- **When the identity cannot be read** (a file system that does not record
  when a folder was made, or a database server reached with its own account
  on another machine), a later press after a lost answer deletes nothing it
  cannot tell apart. The screen says that it cannot tell whether the
  database there now is the same one; the administrator is told to check it
  on the database server (delete it there if it is not wanted, then press
  **Delete** here), a customer to ask the administrator. Until then
  **Delete** changes nothing.

### A domain deletion that waits for the backup DNS server says so

On the main DNS server of a pair, a domain deletion can wait for the backup
DNS server to confirm the list of domains first. Right after such a
deletion the status said "This deletion stopped before it finished … Delete
the domain again", although it finished by itself shortly after (51 seconds
in the alpha.138 lab). It now says that the deletion waits for the backup
DNS server, that nobody needs to do anything, and that it finishes by
itself; the domain list then empties by itself. It still says "delete
again" when nothing would finish it by itself.

### "Add…" for webmail's record: a busy answer is shown as an answer

When the main DNS server answered "busy", the first step of the progress
window kept its waiting clock. It now shows the step done, with "… answered";
what the answer means is said once, below.

### AI and automation apps say what is missing and who installs it

- An app that cannot be installed yet says, in your language, what it needs,
  what this server has, and the next step. For example: "Needs Node.js 24.
  Node.js is not installed on this server yet. Next step for you: install
  Node.js 24 under Components → Node.js." The server's own English words no
  longer appear inside a Turkish sentence.
- The administrator's button opens the place where it is done: Components →
  Node.js at the version list (also when another version is installed), or
  Components → Python to add Python's venv module on Ubuntu 24.04, which
  shows what it adds and installs it after you confirm.
- An app this server's operating system cannot run (Open WebUI on Debian 13)
  is shown to the administrator with the reason, is not shown to customers
  and resellers, and cannot be offered to them. An app a domain already holds
  stays on its card so it can be managed and removed.
- The generic package install for Node.js, which the screen never sends, is
  refused before anything is queued, with the way to the version list.

## A correction to the alpha.138 notes

The alpha.138 notes said that AI and automation apps could not be installed
on Debian 13 and Ubuntu 24.04. That was not right: that lab sent an install
request the screen never sends. What is true:

- **n8n** installs after the administrator chooses Node.js 24 under
  Components → Node.js.
- **Open WebUI on Ubuntu 24.04** needs Components → Python first (Python's
  venv module); it had not been installed in a lab yet.
- **Open WebUI on Debian 13** cannot run on the distribution's Python
  (3.13; Open WebUI supports 3.11 and 3.12), so it is not offered there.

The published alpha.138 notes are left as they were; this paragraph is the
correction.

## Known limits

- **When a deletion meets another database of the same name**, or one it
  cannot tell apart, the panel has no button to keep that database and close
  the entry: the administrator removes or renames it on the database server,
  then **Delete** finishes.
- **Undoing a create that did not finish** (a domain database, a site copy,
  the application catalogue, WordPress, the database tools' copy) still
  drops by name what that same request had just made.
- **A database on another machine with MariaDB** has no identity, so after a
  lost answer there the "cannot be told apart" answer is the only safe one.
- **Open WebUI has never been installed in a lab**, and n8n was last
  installed in the alpha.118 and alpha.119 labs.
- **A Node.js release** is checked against the official checksum file, but
  that file's signature is not checked yet.
- **While the Agent cannot answer**, nothing is hidden because of the
  operating system; the install still asks the Agent and refuses then.

## Checked in the lab

**Open: the lab has not run yet.** This section is written after the lab run
on a pair of test servers. Until then, nothing in this release has run on a
real server.

## Evidence

No schema change: schema 079, as in alpha.139. New, all additive: the panel
setting entries `database_delete_v1:<number>` (one per database being
deleted, removed with it); two Agent calls, `DatabaseIdentityV1` (read only)
and `DropDatabaseOnceV1` (drops only the identified database; an older Agent
has neither, so a mixed panel and Agent drop nothing); the refusal codes
`DATABASE_DELETE_UNVERIFIED` and `COMPONENT_INSTALLED_BY_VERSION`; the AI
apps' facts `needs`, `found` and `found_known`.

Tests, each package's full suite in the test machine, before (alpha.139) and
after:

| Package | Before | After |
|---|---|---|
| `internal/core` | 42 | 43 |
| `internal/serviceapps` | 17 | 18 |
| `internal/transport` | 42 | 42 |
| `internal/services` | 244 | 244 |
| `cmd/agent` | 2659 | 2665 |
| `cmd/panel` | 2773 | 2785 |

No failure before or after. New tests that failed before this release:
`TestDatabaseDeleteAgainNeverDropsASameNamedDatabaseMadeMeanwhile` (lost
answer on MariaDB and PostgreSQL, users not all removed),
`TestDatabaseDeleteWithoutAnIdentityNeverDropsWhatItCannotTellApart`,
`TestDomainDeletionRetryNeverDropsASameNamedDatabaseMadeMeanwhile`,
`TestDatabaseDeleteAgainOnAnotherMachineNeverDropsASameNamedDatabase` and
`TestDeletionHeldForTheBackupsZoneListSaysItFinishesByItself`. Also:
`TestDatabaseDeleteAgainAfterADropThatNeverRanDeletesTheSameDatabase`,
`TestDatabaseDeleteRefusedLeavesNoMarker`, the Agent's
`TestDropDatabaseOnce…` tests and
`TestDatabaseDirectoryIdentityChangesWhenTheDirectoryIsMadeAgain` (on a real
ext4 file system), `component_installed_by_version_test.go`,
`service_apps_os_test.go`. Web: `npm test` 3054 → 3067 passed,
0 failed (`databases-page-alpha133`, `webmail-dns-add-alpha135`,
`ai-apps-truthful-alpha140`); the type check is clean and the build meets
every size budget (first-load path 109.36 KiB of 110.00 KiB compressed).
