# v0.1.0-alpha.135

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.135.tr.md)

A small release of two jobs. Deleting a domain asks what happens to its
databases and leaves nothing of the site behind; the mail and backups a
deletion leaves are kept 30 days in one list and then deleted by
themselves. Each mailbox opens its own webmail, and sending limits are one
line on the mailbox's row.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.134

1. Update from the panel's own update screen, as before. This release does
   not change the panel's database layout (it stays at the alpha.134 form),
   so going back to alpha.134 later needs no database restore.
2. **Look at Server → Maintenance → What deletions left once after the
   update.** As soon as the panel is back, the list takes in what earlier
   releases left on this server (only a record is written; nothing on disk
   changes):
   the stored mail of domains deleted before (moved aside then, never
   removed) and the backups of domains that no longer exist. **Their 30
   days start at the update: after that they are deleted by themselves.**
   If you want to keep something:
   - a backup: press **Download** on its row, then each file you want;
   - mail: copy the folder named under the row's **Details** to another
     place before the date (as the server's administrator; there is no
     "give it back" button yet, see Known limits).
   Nothing in the list is deleted before its date unless you press
   **Delete now…** or **Delete all now…**. Copies at a remote backup
   destination are never touched.
3. The same page also lists **mail left in place**: the mail of mailboxes
   deleted before this release, still in the domain's live mail folder,
   where a new mailbox with the same address would see it. Nothing moves by
   itself. Press **Keep 30 days…** on each one you want out of the way; a
   new mailbox made at such an address moves the old mail into the list
   first by itself.
4. **What earlier deletions left besides mail and backups stays where it
   is.** On a server that deleted a domain with an earlier release, this
   release neither lists nor removes: the deleted site's web server and
   traffic logs, its scheduled tasks file (a crontab of an account that no
   longer exists), its certificate check folder, and its databases and
   database users, which are still on MariaDB and PostgreSQL but in none
   of the panel's lists. You see none of them on any page. They do no
   harm where they are; the server's administrator can remove them by
   hand. A domain deleted from now on leaves none of them behind, and its
   databases stay in the list unless you choose to delete them.

## What changes for you

### Deleting a domain

- **The question asks about the databases first.** When the domain has
  databases, "Also delete its databases" is the first thing in the
  question, with one sentence saying what happens ticked or not. Not
  ticked (the default): the databases stay on the Databases page, bound to
  no domain, with their users and passwords. Ticked: each database is
  deleted exactly as the Databases page deletes it (a user another
  database uses stays and only loses its rights on this one). For an
  administrator the result also names whose rights on the database were
  removed, as the Databases page does.
- **Nothing of the site stays behind.** The site's scheduled tasks, its web
  server and traffic logs (with their older copies) and its certificate
  check folder go with the site; the server's sending-limit lists are
  written again without the domain.
- **Mail and backups are kept 30 days.** The domain's stored mail and its
  backups on this server go into Maintenance → What deletions left, and
  the question says so. The finished window says "<domain> deleted" and
  lists what stayed: databases and where they are, mail and backups with
  their size and the date they are deleted. A customer is told the
  provider can give them back until that date; folders are shown only to
  the administrator.
- A deletion that stops at a step says which step, who acts and what to
  do; pressing Delete again continues where it stopped, with the database
  choice it was started with.

### What deletions left (Server → Maintenance)

- One list: the mail of deleted domains and mailboxes and the backups of
  deleted domains on this server, each with its size and the date it is
  deleted by itself, the total at the top.
- **Delete now…** on a row and **Delete all now…** ask in the panel's own
  window and show each step; the list reads itself again.
- **Download** for a backup's files.
- A removal that failed stays in the list with its reason under
  **Details** and is tried again by itself (after an hour, then less
  often); one whose answer was lost is said as "not known", never as done.
- The administrator's home page says in one line how much is kept and
  opens the list. Past 85% disk use, the disk warning names the list as
  something that can be deleted now.
- Deleting one mailbox now moves its mail into this list too, so a mailbox
  made again with the same address starts empty; the question and the
  result say so.

### Mailboxes

- **Each mailbox row has "Webmail".** It opens webmail in a new tab with
  that mailbox's address already filled in on the sign-in page. Nothing
  else goes into the link: no password and no sign-in.
- The row's actions are words: **Webmail** and **Password…** beside the
  row, the rest under **More**, the deleting ones last. On a phone each
  mailbox is a card with its actions across the width.
- The general webmail card is gone while webmail works. It appears only as
  the problem with its fix (not installed, no address, not answering):
  the administrator gets the button that fixes it, a customer is told to
  ask the provider.
- **Sending limits are no longer a card.** While the limits are applied on
  the server, each row has one short line: its limit (or a new mailbox's
  limit and until when) and what it sent this hour. **Sending limit…** in
  **More** opens that mailbox's limit in its own window with Save and
  Discard. While the limits are not applied, customers see nothing about
  them and the administrator one line with the button to the mail server
  settings. The domain's and the hosting account's limits are named only
  when they matter (80% of this hour, or refusals in the last 7 days).
- A mailbox whose sending was stopped keeps its notice and its way out
  (a new password) in a row across the whole list.

## Known limits

### What deletions left

- There is no **Give back** ("Geri yükle") for held mail or backups yet. A
  backup can be downloaded; mail is copied back by the server's
  administrator from the folder shown under **Details**.
- Older backups kept under a folder named after the domain (not under the
  hosting account's folder) are not listed and are not deleted by the list.
- A held backup folder that is empty shows 0 B.
- The list shows at most 500 mail folders left in place at a time and says
  so when there are more; the rest appear as these are kept.
- What domain deletions of earlier releases left besides mail and backups
  (logs, a crontab, the certificate check folder, databases and their
  users) is neither listed nor removed (see "Updating from alpha.134", step 4).

### Mailboxes

- When the browser already has a webmail session of another mailbox,
  **Webmail** opens that session; sign out of webmail first to reach the
  other mailbox.

### Moved to alpha.136

Remembering that a database was already dropped, so a second press does not
delete a same-named database created meanwhile; automatic repair of
out-of-step copies of CelikPanel's engine account; the refusal that says
what frees a database user's name; PostgreSQL rights on the tables the
database's own user owns; the "user removed" result while the account
stays; database sizes in the Databases list. Also the **Add…** button
beside **Copy** on the webmail address check, which asks the paired main
DNS server to add webmail's record: the lab found it not adding the record
on a real server pair, so it is not in this release (the check offers
**Copy**, as in alpha.134).

### Defects found in the lab, left for the next release

- An open **Domains** page in another tab does not follow a deletion made
  elsewhere; the deleted domain stays on it until the page is opened again
  (the **Databases** page does follow).
- A held folder that was replaced by a link is refused by **Delete now…**
  as it should be, but the next round then drops it from the list while
  the link stays on disk.
- A removal that fails part-way (one file that cannot be deleted) has
  already deleted the rest of that folder; the row says it failed and why.
- After **Keep 30 days…** the finished window shows a leftover "Working on
  the server…" line under "Completed".
- The home page's activity feed names a held item by its internal key (for
  example `backups.s5-d39`) instead of "Backups of <domain>".
- The rights paragraph in a domain deletion's result is long; it should be
  one line with the rest under a closed **Details**. The folder paths in
  the list of what deletions left are shown open.

### Other

- Earlier limits still apply (see alpha.134), except those this release
  fixes: a domain deletion no longer drops its databases out of the
  panel's lists, and a mailbox made again with an earlier address no longer
  sees the earlier mail.

## Checked in the lab

One lab run on two virtual servers: Debian 13 with MariaDB 11.8.6 and
PostgreSQL, and Ubuntu 24.04 with MariaDB 10.11.14. Both were updated from
alpha.134 through the panel's own update page; each update finished by
itself with one panel and one Agent running. No run had a hard failure.

**Checked and passed:**
- What an alpha.134 deletion had left (a deleted domain's mail and its
  backup) was taken into the list at the update with 30 days from then,
  and nothing on disk changed: not at the update, not after a restart and
  not over two later rounds.
- An item whose date had passed was removed by itself; one that could not
  be removed stayed as failed with its reason and was removed at its next
  try once the cause was gone. **Delete now…** removed exactly that item;
  **Delete all now…** removed the ten listed items and nothing else (live
  backups and mail untouched). A held backup downloaded byte for byte.
- Links inside a held folder were removed as links; what they pointed to
  outside stayed. A held folder replaced by a link was refused and left
  as it was.
- Deleting a domain with "Also delete its databases" ticked, as the
  administrator and as a customer, on MariaDB and PostgreSQL: databases
  and their users gone from both engines, rights on another account
  removed (named to the administrator only), no crontab, logs or
  certificate check folder left, mail and backups in the list. Not
  ticked: the databases stayed, listed and working.
- A mailbox deleted and made again under the same address started empty,
  also for an address whose old mail alpha.134 had left in place (that
  mail went into the list first). **Keep 30 days…** moved a left folder
  into the list.
- The mailbox row's **Password…** and **More** menus, the question in
  English, and a short look at the main pages on both servers.

**Not checked:** opening webmail from a mailbox row (the lab's webmail had
no address of its own), the sending-limit line while the limits are
applied, going back to alpha.134, and timings (another job loaded the lab
host). The **Add…** button failed (see "Moved to alpha.136").

## Evidence

No schema change: schema 079, as in alpha.134. The holding list is one
versioned record per item in `panel_settings` (`deletion_hold_v1:<id>`);
the domain deletion's database choice is saved with its deletion marker
(versioned). New Agent calls: two that only read (what is held, the mail
folders left in place) and two that change, each refused by an Agent of
another build (removing one held item, derived from its kind and
identifier: exactly one folder, links never followed, an item already gone
answered as removed; holding one left mail folder, derived from its
address and refused while the address has a mailbox); the
mailbox and domain deletions move mail into the holding folder instead of
leaving it. New administrator endpoints under
`/api/v1/maintenance/deletion-holding` (list, delete now, a backup's
files and download, keep a left folder). New audit actions
`deletion_hold.*`. A database
deleted through a domain deletion goes through the one database deletion
(`deleteDatabaseRecord`) with alpha.134's rights removal, and the domain
deletion's result carries the same administrator-only rights facts
(`databaseDeletionAnswerFor`).
Component tests: `cmd/panel/deletion_holding_test.go`,
`cmd/panel/deletion_holding_left_test.go`,
`cmd/panel/domain_delete_cleanup_test.go`,
`cmd/panel/mail_outgoing_test.go`, `cmd/agent/deletion_holding_linux_test.go`,
`cmd/agent/deletion_holding_left_linux_test.go`,
`cmd/agent/site_delete_cleanup_test.go`; mounted web tests
`web/tests/domain-delete-question-mounted.test.mjs`,
`web/tests/remote-state-mounted.test.mjs`,
`web/tests/mail-rows-alpha135.test.mjs`,
`web/tests/refusal-words-alpha135.test.mjs`
(RESILIENCE-CONTRACT, "Branch a134/deldomain" and "Branch a135/mailrow").
One lab run on the release head `90293c5c2` (see "Checked in the lab");
after it only the **Add…** job was taken out (its merge reverted).
**No P0 item is closed.**
Release sequence 134 → 135.
