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

1. Update from the panel's own update screen, as before. The update changes
   the panel's database (one new setting per domain). If the update fails,
   or if you go back to alpha.133 later, the panel's database is put back
   to the copy taken just before the update: alpha.133 cannot read the new
   setting.
2. **After the update, take a new backup of every site that runs a program,
   a Python app or a service app** (the domain's **Backups** section, a
   files or full backup). Backups taken before this release do not hold the
   program's folder or its data; the backup list says so on each of them
   ("This backup does not hold the program (older format)").
3. **Look at each domain's certificate once** (the domain's **Security**
   section). The new list "What is protected" says, name by name, what the
   certificate protects. Where the mail name is missing it shows "Not
   protected" with one button, **Cover the mail name too**; one press asks
   for the certificate again with that name. Nothing is requested again by
   itself.
4. If you did not do it after alpha.133: give CelikPanel's database account
   a new password once on each database server (**Components → MariaDB**,
   "The account CelikPanel connects to this service with", **More → New
   password…**; the same on **Components → PostgreSQL**). The alpha.133
   notes say why.
5. *(Database rights: the update steps of this job are added here when it
   joins the release.)*

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
  puts its folder back with the site's account as owner, and starts it
  again; the result says whether it started and answers, or why it did not
  and what to do on the **Programs** tab. A program you had stopped stays
  stopped. If putting the files back fails, the earlier files are put back.
- The program's secret settings are never written into a backup. A restore
  on the same server keeps the server's current settings; on a new server
  enter the secret values again on the **Programs** tab.
- Older backups restore exactly as before; they hold no program folder, so
  the program is left as it is.

### Database rights for exactly one database

*(This part is written when the database-rights job joins this release
after its review: rights given for exactly one database name, earlier rights
converted automatically, and a deleted database's rights removed from every
account and role.)*

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
  site's account (`chown -hR --reference=<site home> <site home>/app`);
  load each database from `restore/payload/databases/`. The full steps,
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

### Database rights

*(Filled in when the database-rights job joins this release.)*

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

- Earlier limits still apply (see alpha.133), except the two this release
  fixes: site backups now hold the program, and the certificate screen now
  says whether the mail name is covered.

## Checked in the lab

**The lab check of this release has not run yet.** This section is filled
in after it, with what was checked and what was not.

## Evidence

Schema 77 → 78: migration 078 `domain_mail_client_name` adds
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
(RESILIENCE-CONTRACT, "Branch a134/certcover" and "Branch a134/progbackup").
No real certificate authority and no native run. **No P0 item is closed.**
Release sequence 133 → 134.
