# v0.1.0-alpha.134

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.134.tr.md)

> **Candidate.** Not published yet; the lab check has not run yet.

A small release of three jobs. A site's certificate now says what it
protects, and a new certificate covers the domain's mail name; site backups
now hold the site's program, Python app and service-app data; database
rights are given for exactly one database name.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.133

1. **Database rights are narrowed by themselves; there is nothing to
   press.** Once the update is verified, CelikPanel rewrites, on each
   MariaDB server it manages, the database rights it gave earlier so that
   each one reaches exactly its own database. **Components → MariaDB**, in
   the block **Database rights by exact name**, then says how many rights
   were narrowed, which of them also reached other databases before (and
   which ones), how many were left as they are and why, and how many are
   still waiting. If any right was left as it is, the administrators get
   **one** notification on the home page.
   - **"Left as it is"** means CelikPanel cannot prove it gave that right
     (an account made by hand, or a database CelikPanel no longer keeps a
     record of), or the right is the owner's own (it holds the grant option
     or a privilege CelikPanel never gives), so it does not touch it. The
     block lists each such right with the databases it reaches now and,
     under **Details**, the statement that narrows it to one database or
     removes it. If an account should reach only its own database, run that
     statement; if it reaches several databases on purpose, leave it.
   - **A narrowed user can no longer reach other databases whose names its
     old right happened to match.** If an application relied on that, give
     the right on purpose (the block names the databases each narrowed right
     reached before).
2. Update from the panel's own update screen, as before. The update changes
   the panel's database (one new setting per domain, and a record of every
   database right before it is narrowed). If the update fails, or if you go
   back to alpha.133 later, the panel's database is put back to the copy
   taken just before the update: alpha.133 cannot read the new records.
   Rights already narrowed stay narrowed on the database server and keep
   working (see Known limits).
3. **After the update, take a new backup of every site that runs a program,
   a Python app or a service app** (the domain's **Backups** section, a
   files or full backup). Backups taken before this release do not hold the
   program's folder or its data; the backup list says so on each of them
   ("This backup does not hold the program (older format)").
4. **Look at each domain's certificate once** (the domain's **Security**
   section). The new list "What is protected" says, name by name, what the
   certificate protects. Where the mail name is missing it shows "Not
   protected" with one button, **Cover the mail name too**; one press asks
   for the certificate again with that name. Nothing is requested again by
   itself.
   - **Replaced certificates go away by themselves; there is nothing to
     press.** A server that got certificates again since alpha.113 holds the
     certificates they replaced; some minutes after the update is verified
     these are removed and each one is listed in the audit log.
5. If you did not do it after alpha.133: give CelikPanel's database account
   a new password once on each database server (**Components → MariaDB**,
   "The account CelikPanel connects to this service with", **More → New
   password…**; the same on **Components → PostgreSQL**). The alpha.133
   notes say why.

## What changes for you

### A certificate says what it protects

- A site's certificate now says what it protects: every name of the site
  and its mail, protected until when, or why not. The list **What is
  protected** stands right under the certificate's facts, before its
  settings and buttons: the web site with and without `www` and its other
  names, web mail, the name mail programs are told, the mail name
  `mail.<domain>` (the MX record and hand-typed settings) and the names mail
  programs use to set themselves up.
- **Names that are not protected come first**, with their reason and their
  one action, then the protected names. One list, always open.
- A protected name says by which certificate and until when ("this domain's
  certificate, until 30 Jan 2027", or the server's mail certificate). A name
  that could not be checked says "Not known", never "Not protected".
- **A new certificate covers `mail.<domain>` when that name points to this
  server** and the domain's mail is on this server. The request form shows
  the name ticked; you can leave it out. A name that points elsewhere, has
  no address or could not be checked is not asked for, and the request goes
  on without it. Mail programs that connect to `mail.<domain>` then get a
  certificate for that name instead of a warning; programs set up with the
  server's mail name see what they saw before, and the name programs are
  told does not change.
- **An existing certificate without the mail name shows the gap and a
  one-press fix:** **Cover the mail name too** asks for the certificate
  again with that name, in the progress window with its real steps. If the
  name no longer points here, it is refused before any certificate
  authority is asked. If the authority refuses, the old certificate and mail
  stay as they were. If mail cannot start using the new certificate, mail
  is put back as it was and the list offers **Use it for mail too**.
- **A name pointing elsewhere says who changes which DNS record:** its
  badge says "Points elsewhere" or "No address in DNS" (a waiting
  condition, not a failure), and one sentence says to point the name's A
  record to this server's address, in this server's DNS records or where
  the domain's DNS is run, and what then appears in the list. **Check
  again** only reads the public DNS again.
- The mail screen says the same thing under the server name ("A trusted
  certificate protects this name (…, until …)").
- Renewal is unchanged: the certificate is renewed by the operating
  system's certbot, also when CelikPanel is stopped or removed, and a
  renewed certificate keeps the mail name.
- **A certificate replaced by a new one** (covering the mail name, getting
  it again, an alias change) is now removed with certbot once nothing uses
  it, so it is no longer renewed in the background; this runs only after an
  update is verified, never touches the certificate in use, keeps anything a
  file on the server or a rollback snapshot still names, and writes each
  removal and each kept certificate to the audit log.
- **A request the certificate authority did not fulfil now says why, who
  acts and what to do next;** customers are no longer sent to a server file.
- **A newly issued certificate that this server does not trust is reported
  as such** instead of an internal error; the current certificate keeps
  serving.

### Site backups hold the program and its data

- Site backups now hold the site's program, Python app and service-app
  data. A files or full backup keeps the web folder as before, and also the
  folder a program or Python app runs from (its program, settings and data)
  and each service app's data (for example n8n, Open WebUI).
- The domain's **Backups** section says, before you take a backup, what it
  will hold and how big each part is ("Site files (12 MB), Program files
  and data (1.1 GB)", and the databases of a full backup), and what is left
  out and why: download caches and half-finished build folders, the
  service apps' installed program files (the panel installs them again),
  special files a program makes again when it starts, and files that do not
  belong to the site's account (named, so you can decide). A backup of 1 GB
  or more asks first; when the backup disk may not have room, it says so
  before anything starts.
- Each backup in the list says what it holds, and whether it was made in
  the earlier format without the program.
- **Restoring a backup that holds the program** stops a running program,
  puts its folder back, and starts it again; the result says whether it
  started and answers, or why it did not and what to do on the **Programs**
  tab. A program you had stopped stays stopped. If putting the files back
  fails, the earlier files are put back.
- Restoring a site backup gives the program's files back to the site's own
  account and its own group; the web server's group never reaches the
  program's folder.
- A backup that is not this site's, is no longer on the server, is damaged,
  or does not fit on the disk is now said as such, with what to do, instead
  of "internal error". Nothing is restored and the site stays as it was.
- The program's secret settings are never written into a backup. A restore
  on the same server keeps the server's current settings; on a new server
  enter the secret values again on the **Programs** tab.
- Older backups restore exactly as before; they hold no program folder, so
  the program is left as it is.

### Database rights for exactly one database

- **Why this was a security weakness.** MariaDB reads the database name in
  a right given on a whole database as a pattern: `_` stands for any one
  character. Every name CelikPanel makes holds `_`, so a right on one
  customer's database could also match another customer's database whose
  name differs only there. Measured in the alpha.133 lab on MariaDB 11.8.6
  and 10.11.14: a user whose right named `labd_1__wp` could create a table
  in `labd_12_wp`.
- **Every right CelikPanel gives is now written for exactly one database
  name** (creating a database or a user, giving a user to another database,
  copies for outside connections) and read back from the server after it is
  written: the stored right must name exactly that database.
- **Rights CelikPanel gave earlier are narrowed by themselves** after the
  update is verified (see the update steps above). For each right,
  CelikPanel first records it as it was, then gives the same privileges by
  the exact name and checks them, then takes the old pattern right away and
  checks again, so access is never lost in between; an interrupted round
  continues from the server's state. Only a user and a database that
  CelikPanel records together are touched; rights it cannot prove are its
  own are listed, never changed. It runs in the panel's own hourly round,
  not in the first minutes after a start and not while the server is busy;
  until it can run, the page says that it is waiting and why. A right that
  fails three times waits a day and is listed with the statement to narrow
  it by hand.
- **Deleting a database takes every right naming it from every account and
  role,** also from accounts made by hand under any name; the accounts and
  roles themselves stay. **This differs from alpha.133:** rights naming a
  deleted database are now removed from a role of the same name too. A
  right that only matches the name as a pattern and also reaches another
  existing database is kept for an account not on record for the deleted
  database, and the answer lists it.
- **A database name is limited so that its exact right fits.** MariaDB
  stores the database name of a right in at most 64 characters, and in the
  exact form every `_` counts twice. Measured on MariaDB 11.8.6: a right on
  a longer exact name is accepted and shown whole, but stored cut to 64
  characters, which after the next reload is a right on another, shorter
  name. So a new database whose exact name would not fit is refused before
  anything is made, with how much shorter the name must be. Names CelikPanel
  makes itself (application and WordPress installs, copies) are shortened
  until they fit. A cPanel import leaves such databases out and names them.
  An existing database with such a name keeps working with the pattern form
  it always had, and is listed.
- **Creating a database now refuses, before anything is made, a name whose
  database or user would be too long for the engine** (PostgreSQL used to
  answer an internal error), and the form shows the real full name and
  stops such a name as you type.
- **A database create that fails no longer leaves its user behind** on a
  MariaDB account without the RELOAD right, and when clean-up cannot finish
  the panel names what is left.
- The service pages and the login page's demo accounts now load when they
  are opened, so the panel's first load stays within its size limit.

## Known limits

### Certificates

- **The new certificate requests have not run against a real certificate
  authority.** They are covered by component tests; the lab check (with a
  test authority) has not run yet. Lab steps: docs/CERTIFICATE-COVERAGE.md
  §8.
- A name that starts to point to this server after the certificate was
  issued (a new alias, or `mail.<domain>` created later) is shown "Not
  protected" with **Get it again with this name**; it is not added by
  itself.
- A certificate that covers every sub-name (wildcard) is not built.
- A certificate request still shows the general "internal error" sentence
  when the panel cannot save its own records, or when the server refuses
  the request before certbot runs (for example, the folder for the name's
  proof cannot be prepared).
- **A replaced certificate is kept, not removed, when something CelikPanel
  does not look at still uses it:** a web server file outside `/etc`, or a
  program that reads the certificate files directly. The removal checks
  `/etc`, the site records and the rollback snapshots only.
- **If you go back to alpha.133 by restoring the snapshot taken before the
  update,** alpha.133's own start-up clean-up can remove a certificate that
  was issued after the update (older behaviour, unchanged); once alpha.134
  runs again, the site shows that certificate as missing, with a button to
  get it again.
- A site file you kept as `.celikpanel-backup-*` keeps the certificate it
  names for as long as that file exists.

### Site backups

- **The backup and restore of programs is component-tested only;** it has
  not run on a real system yet. Lab steps: docs/SITE-BACKUP-CONTENTS-DESIGN.md,
  "Lab steps".
- **Deleting a site and then restoring its backup does not work yet.** A
  backup package is bound to its site and subscription, so it cannot be
  restored into a new site through the panel; that comes in a later
  release. If you delete a site whose backup you need, download the backup
  first (the domain's **Backups** section). To put it back by hand on the
  new site: unpack the package (`tar -xzf <backup>.cpbak -C restore`); put
  the site files from `restore/payload/files.tar.gz` into the new site's
  web folder; put `restore/payload/home.tar.gz` into the new site's home
  folder (the folder that holds `public_html`) and give the files to the
  site's account and that account's own group, never the site home's group,
  which is the web server's
  (`chown -hR "$(stat -c %U <site home>):" <site home>/app`; the colon takes
  the account's own group); load each database from
  `restore/payload/databases/`. The full steps,
  with examples, are in docs/REMOTE-BACKUPS.md, "Restore without
  CelikPanel", steps 4 to 6; a backup downloaded from the panel is not
  encrypted, so steps 1 to 3 do not apply to it.
- **A service app's data is in the backup but the panel does not restore
  it** in this release; the restore result names those apps. By hand: stop
  the app, take `apps/<app>/data` from `payload/home.tar.gz`, start the app
  again.
- Files directly in the site's home folder outside the web folder, the
  program folder and the service-app folders are not in the backup.
- After the server itself is lost, the program's secret values, a service
  app's keys and n8n's encryption key must be entered again (they are never
  in a backup).
- If the panel stops between stopping and starting the program during a
  restore, the program is left stopped; start it on the **Programs** tab.
  A program operation started in the few moments after the restore's check
  is not yet held back.
- A Node.js site whose `node_modules` contains links still cannot be backed
  up (unchanged).
- Backups do not count against the plan's disk space (unchanged).
- After going back to alpha.133, backups made by alpha.134 that hold a
  program are not listed and cannot be restored there; they stay on the
  server and appear again after the next update to alpha.134.
- A restore still shows the general "internal error" sentence when the
  server cannot read or write a file during the check or the restore (other
  than a full disk), when the server stops answering in the middle of a
  restore that did not stop a program, or when the panel cannot read its
  own records of the backup's databases.

### Database rights

- **MySQL 8 servers are listed, not converted.** New rights there are
  exact, but rights given earlier are only listed with the statement to
  narrow them: how MySQL 8 stores and shows them with `partial_revokes` was
  not verified.
- **Narrowing a listed right with one press is not built.** Use the
  statement under **Details** on **Components → MariaDB**.
- **A registered database server whose account cannot read the rights
  tables cannot be verified.** Rights there stand on the name-length rule,
  nothing is refused, and the MariaDB page says that rights there cannot be
  verified and what the account needs (`SELECT` on `mysql.db`). To narrow
  earlier rights automatically, the account must read and change every
  account's rights, as CelikPanel's own account does (`ALL PRIVILEGES ON
  *.* WITH GRANT OPTION`).
- **Going back to alpha.133** restores the panel's database from before the
  update. The narrowed rights stay on the database server and database
  users keep working: removing a user added to a database succeeds there
  too (checked in the lab), and deleting a database on this server's
  MariaDB removes its rights. Only these still meet the exact-name form on
  alpha.133: an account alpha.133 keeps when its user is removed (one with
  rights elsewhere, or on a host CelikPanel did not make) keeps its narrowed
  right on that database, and the answer says the account was kept; taking
  a right away through the API's grant removal (no screen uses it) answers
  "no such grant" and the right stays; and outside-connection copies of
  such a user are not refreshed. On a registered MariaDB server alpha.133
  deletes only the database, and the rights naming it stay (alpha.133's own
  behaviour). Updating to alpha.134 again continues from the server's state.
- **An old defect remains:** when a registered database server answers
  "no permission for this action", the panel still shows it as "the server
  refused the credential".
- After a deletion that kept a user's accounts, a database of the same name
  still cannot be created until those accounts are removed, and the refusal
  does not say so yet (alpha.135).
- **A database create whose engine gives no answer** (the connection is
  lost) is not taken back by itself: whether the database or its user was
  made is not known, and the next try with that name is refused as taken.
  Choose another name; the server's administrator removes what was made on
  the database server.
- **On a registered MariaDB server whose account lacks the RELOAD right, a
  database create still cannot finish.** It now leaves nothing behind.

### Moved to alpha.135

Deleting a domain and what it leaves (its databases, and 30 days of holding
for its mail and backups); remembering that a database was already dropped,
so a second press does not delete a same-named database created meanwhile;
automatic repair of out-of-step copies of CelikPanel's engine account; the
refusal that says what frees a database user's name; PostgreSQL rights on
the tables the database's own user owns; the "user removed" result while
the account stays; a webmail link per mailbox; a simpler display of sending
limits; an **Add** button that asks the paired DNS server to add a record;
database sizes in the Databases list. The alpha.133 known limits on these
still apply.

### Other

- Earlier limits still apply (see alpha.133), except those this release
  fixes: site backups now hold the program, the certificate screen says
  whether the mail name is covered, MariaDB rights are given by exact name,
  and a deleted database's rights are removed from every account and role.

## Checked in the lab

**The lab check of this release has not run yet.** This section is filled
in after it, with what was checked and what was not.

## Evidence

Schema 77 → 79 for the release as a whole. Migration 078 `domain_mail_client_name` adds
`domains.mail_clients_use_own_name` (0/1, default 0) and sets 1 for every
domain whose active certificate secured mail before, so no program is told
another name after the update (additive; alpha.133 refuses a ledger with
078, so going back is the pre-update snapshot restore, D-025). New
read-only endpoints `GET /api/v1/domains/{id}/ssl/coverage` and
`GET /api/v1/domains/{id}/backups/contents` (and the read-only Agent call
`Agent.BackupContentsV1`). A certificate request is staged as before
(D-029); mail serves a domain's certificate for `mail.<domain>` through one
Postfix SNI entry and one Dovecot `local_name` block naming certbot's live
files of the site's lineage; renewal with the panel absent is certbot's
lineage and its native deploy hook, unchanged (P0.5). Backup package
manifest version 3 (`home`, `areas`, `left_out`) is written only when an
area exists; otherwise the unchanged version 2. The Agent RPC protocol stays
2; the new `Info` and `RestoreResponse` fields are additive; an older release
refuses a version-3 package before anything is written (fail closed).
Component tests: `cmd/panel/certificate_coverage_test.go`,
`cmd/panel/certificate_mail_default_test.go`, the migration tests in
`internal/db`, `internal/mailtlsconfig`, `cmd/agent/backup_home_linux_test.go`,
`cmd/panel/domain_backup_program_test.go`; mounted web tests
`web/tests/certificate-coverage-alpha134.test.mjs` (including the order of
the list) and `web/tests/site-backup-contents-mounted.test.mjs`
(RESILIENCE-CONTRACT, "Branch a134/certcover", "Branch a134/progbackup" and
"Branch a134/dbrights").
Migration 079 `database_grant_forms` adds `database_grant_forms` (the
before-image of each earlier right: account, host, stored name, privileges,
and its state) and `database_grant_form_engines` (per engine, including
`grants_unverifiable_since` and `waiting_since`); additive; alpha.133
refuses a ledger with 078 or 079, so going back is the pre-update snapshot
restore (two steps). Every GRANT on MariaDB runs with `NO_AUTO_CREATE_USER`
in its session and is read back; a read-back that failed is an unknown
result, never taken for either answer. The database-rights tests ran in
development against a real MariaDB 11.8.6 (the measurements above);
MariaDB 10.11, a second registered server and the check with two customers'
databases are left to the lab. Details: docs/DATABASE-USERS.md.
Fix round after the lab: one name-length rule for the panel, the Agent and
the form (`internal/services/database_name_limits_test.go` and its shared
cases, `cmd/panel/domain_database_create_fix_test.go`,
`cmd/panel/database_create_leftover_engine_test.go`,
`web/tests/database-name-limits.test.mjs`); restore refusals and the
program folder's group (`cmd/agent/backup_restore_refusals_linux_test.go`,
`cmd/panel/backup_restore_refusals_test.go`); replaced certificate lineages
and the issue refusals (`cmd/agent/site_cert_retire_test.go`,
`cmd/panel/certificate_issue_guidance_test.go`,
`cmd/panel/site_cert_native_test.go`). No schema change. New Agent RPC
`RetireSiteCertificateLineagesV1` (build-gated), audit actions
`ssl.lineage.*`, D-040 point 9 amended; additive transport field `Left` and
code `DATABASE_CREATE_LEFT_OVER`; additive `InspectResponse.Code` and
restore codes, backup manifest still version 3. The fix round has
component tests only until the second lab run.
No real certificate authority and no native run. **No P0 item is closed.**
Release sequence 133 → 134.
