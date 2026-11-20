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
  Components → Python to add Python's venv module on Ubuntu 24.04 (for a
  Python app), which
  installs the operating system's python3-venv and python3.12-venv after you
  confirm. Its confirmation window does not name those packages yet (see
  "Found in the lab").
- An app this server's operating system cannot run is shown to the
  administrator with the reason, is not shown to customers and resellers,
  and cannot be offered to them. An app a domain already holds
  stays on its card so it can be managed and removed.
- The generic package install for Node.js, which the screen never sends, is
  refused before anything is queued, with the way to the version list.

## A correction to the alpha.138 notes

The alpha.138 notes said that AI and automation apps could not be installed
on Debian 13 and Ubuntu 24.04. That was not right: that lab sent an install
request the screen never sends. What is true:

- **n8n** installs after the administrator chooses Node.js 24 under
  Components → Node.js.
- **Open WebUI** installed once in this release's lab, on Ubuntu 24.04
  after Components → Python; it cannot run on Debian 13 (Python 3.13). It is
  being withdrawn (see "Known limits").

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
- **Open WebUI is being withdrawn.** The owner has decided to take Open
  WebUI out of the product; alpha.141 removes it from the catalogue, so do
  not offer or install it now. An install already on a domain stays visible
  and can be removed. In the lab it needed about 15 GB of free disk at the
  peak of its install (about 9 GB stays) and about 2 GB of memory.
- **The page does not check an AI app's disk and memory need first, and a
  running install cannot be stopped from the page.** Look at the free disk
  before installing; n8n took about 3 GB and nine minutes in the lab.
- **A Node.js release** is checked against the official checksum file, but
  that file's signature is not checked yet.
- **While the Agent cannot answer**, nothing is hidden because of the
  operating system; the install still asks the Agent and refuses then.

## Checked in the lab

Lab check finished: one run on 20 November 2026 (UTC 09:00 to about 10:25,
85 minutes of a 100-minute slot), on a pair of test servers: a Debian 13
main DNS server with PowerDNS and an Ubuntu 24.04 backup DNS server with
BIND, started from the alpha.139 state. The release ran under a lab-only
label (v0.1.0-alpha.147) built from this release's code. Everything was done
through the page's own buttons except where it says otherwise. No hard
failure.

- **Both servers updated through the panel's own update page**, by
  themselves, in about two and three minutes. Both waited for the operating
  system's daily package run, which started right after the press; the page
  says beforehand that an update waits for it. No service was left failed, and both servers answered the
  same DNS questions the same way before and after.
- **Deleting a database** through the Databases page, on MariaDB and on
  PostgreSQL: the progress window, then "Completed"; the row left the list
  by itself and the database was gone from the server.
- **A database is never deleted twice.** On MariaDB (Debian 13 and Ubuntu
  24.04) and on PostgreSQL (Debian 13), the answer of a deletion was cut on
  purpose (the lab stopped the Agent at the press). Each time the database
  had been dropped and its users or rights were not done yet, and the page
  said to press **Delete** again. Then a database of the same name was made
  again with new rows: on Ubuntu 24.04 by the customer's own database user,
  the very case this release is about; on Debian 13 by hand. **Delete**
  pressed again, once on a Turkish and once on an English page each time,
  left the new database and its rows in place, and the page said that
  CelikPanel does not delete it and that the administrator decides. After
  the lab dropped that database by hand, **Delete** finished. The identity
  was read every time: MariaDB gave the new folder the same inode number,
  and the inode generation and the time the folder was made told them
  apart; PostgreSQL by its internal number.
- **A domain deletion with its databases** (one MariaDB, one PostgreSQL):
  "2 databases deleted" with their names, in 14 seconds; the domain left the
  list by itself; no database or database user of it stayed on the servers.
- **AI and automation apps on Debian 13**, as administrator: n8n's line said,
  in Turkish, that it needs Node.js 24, that none is installed, and what to
  do, with **Install Node.js 24**, which opened Components → Node.js at the
  version list. Open WebUI showed its reason (Python 3.13), could not be
  offered, and customers did not see it. The generic Node.js install request,
  sent directly, was refused and nothing was queued.
- **n8n installed through the page on Debian 13**: Node.js 24 from Components
  (seconds), n8n offered and installed on a customer's domain (9 minutes
  24 seconds, about 3 GB); it answered over HTTPS and again after the card's
  own restart.
- **Open WebUI** installed once through the page on Ubuntu 24.04, after
  Components → Python's repair added python3-venv and python3.12-venv; it
  answered over HTTPS. It is being withdrawn (see "Known limits").
- **The busy end of "Add…"** at phone width, Turkish and English: the first
  step done with "… answered", the busy sentence once, and **Ask again…**
  then added the record.
- The usual pages on both servers loaded without an error.

**Not checked:** the "waits for the backup DNS server" sentence of a domain
deletion. Both domain deletions on the pair finished in 12 to 14 seconds
because the backup server confirmed during the deletion itself, so the
waiting state did not arise; "stopped … delete the domain again" did not
appear either. The customer's line "the administrator installs Node.js"
(n8n was not offered to customers at that moment). A domain that holds both
a program and an AI app (not run for time).

### Found in the lab, left for the next release

None of these is a security or data-loss problem.

- An AI app's install cannot be stopped from the page, and nothing checks
  the free disk first (Open WebUI's install took the lab's 23 GB disk to
  96 %).
- On Debian 13 the administrator's view of a domain's Programs section told
  the administrator to offer Open WebUI in Catalog management, although it
  cannot be offered there; this goes away when Open WebUI is taken out of
  the catalogue.
- The Python repair's confirmation window says nothing else needs
  confirming and does not name python3-venv and python3.12-venv, which it
  then installs.
- After a deletion whose answer was lost, the sentence about a database of
  the same name says its users "were left as they are", although on MariaDB
  the user had already been removed by the first press while the panel
  still listed it.
- During a domain deletion the database step read "there was no database to
  process" for about 9 seconds while two were being deleted (the end result
  was right).
- The "a database of this name was made after the deletion" state is shown
  in the red error colour, although it is a decision for the administrator.
- The service operation status still returns an older, failed generic
  Node.js install as the latest operation.
- The certificate confirmation names "Let's Encrypt" whatever certificate
  authority the server is set to use.

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
