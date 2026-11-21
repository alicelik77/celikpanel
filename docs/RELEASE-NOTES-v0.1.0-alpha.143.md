# v0.1.0-alpha.143

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.143.tr.md)

**A security release: please update soon.** It closes three ways in which
one customer could act as another: outgoing mail signed with another
customer's domain, a PostgreSQL database move that ran a customer's code
with the administrator's rights, and PostgreSQL databases made before
alpha.141 that every account of the server could still enter. It also
brings the four corrections the alpha.142 lab asked for.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.142

1. Update from the panel's own update screen, as before.
2. **The panel's database layout changes** (schema 81): the six rules that
   check every new name are replaced by the same rules with one exemption
   (see "A subdomain inside your own domain" below); no table, column or row
   changes. Going back to alpha.142 later means restoring the snapshot the
   update takes, as for every release that changes the layout.
3. At its first start after the update, the panel sets the mail signing
   settings (see "Outgoing mail and its signature"). A setting the owner
   changed by hand is kept and named in **Problems now**. There is nothing
   to press.
4. A few minutes after the start, once the update is verified and the
   server is quiet, the panel begins closing the PostgreSQL databases made
   before alpha.141 that it may close by itself, then checks again every
   hour (see "PostgreSQL databases made before alpha.141").
5. **Going back to alpha.142 opens the closed ways again.**

## Security: what is closed

- **Mail signed with another customer's domain.** The mail server signed
  any message whose "From" line named a domain hosted here, both for a
  mailbox sending with its login and for a website handing mail to the
  server without one. So mail carrying another customer's address could
  leave this server with that customer's valid signature. Now mail sent
  with a mailbox login is signed only for that mailbox's own domain, and
  mail arriving on port 25 is never signed.
- **A database move that ran a customer's code as the administrator.**
  Moving a PostgreSQL database to another database server on this machine
  loaded its rows with the administrator's rights, so code the database's
  owner had placed in a table ran with those rights. Now each table's rows
  are loaded with that table owner's rights only.
- **Older PostgreSQL databases open to every account.** Databases made
  before alpha.141 kept PostgreSQL's default, under which every database
  user of the server may connect to every database and make temporary
  tables in it: one customer's database user could sign in to another
  customer's database and see how it is built. Now these databases are
  closed to other accounts, by the panel itself where that is safe, and
  with one button where the administrator must decide.

## What changes for you

### Outgoing mail and its signature

- **Mail sent with a mailbox login** (a mail program, webmail, a website
  that signs in on port 587 or 465) is signed for that mailbox's own domain,
  whatever its "From" line says. Mail whose "From" line names another
  customer's domain is no longer signed with that customer's key.
- **A website that hands mail to port 25 without a login** is no longer
  signed. To keep its mail signed, set the site's mail to sign in with one
  of its own mailboxes on port 587.
- **Mail made on the server itself** (the panel's notices, scheduled tasks'
  mail, out-of-office replies) is signed for its "From" domain as before.
- Webmail, mail programs, out-of-office replies and forwards otherwise
  behave as before; incoming mail is not changed.
- **For the owner:** the change is ordinary mail server settings that work
  with the panel stopped or removed. Before CelikPanel uses a new signing
  setting, the mail signer checks it; if the mail server does not run with
  it, the earlier setting is put back. A signing settings file or filter
  setting the owner changed by hand stays as it is: CelikPanel's own text
  is placed beside it as `opendkim.conf.celikpanel-pending`, and **Problems
  now** says that mail may still be signed for another customer's domain,
  with the panel's log naming the setting and the owner's step.

### PostgreSQL databases made before alpha.141

- The panel closes such a database by itself only when its rights are
  still PostgreSQL's untouched default, nothing inside it belongs to an
  account the panel does not record for it, no such account is connected
  to it, and its database server has no monitoring account. Its owner and
  the users the panel records for it keep their way in; every other account
  can no longer connect to it or make temporary tables in it.
- Every other such database is left exactly as it is and listed in
  **Problems now** and on the **PostgreSQL** page with what was found, what
  closing changes, and one button to close it after a confirmation.
- Every close shows when and by whom it was made, with **Undo** (and the
  plain statements to undo it without the panel); undo gives back exactly
  what the close changed. A database the administrator opened again is
  never closed again by itself.
- MariaDB has no such default: nothing changes there. PostgreSQL servers
  registered on other machines are not touched.

### Moving a PostgreSQL database to another database server

The structure is loaded as before; each table's rows are loaded with that
table owner's rights only, through a temporary account removed afterwards.
What a move keeps is unchanged (owners, rights, comments, extensions,
settings, large objects), and the source is removed only after the copy is
verified, as before. A move that cannot be done this way is refused before
anything is written, with the reason and what to do, and the source stays
untouched: a table owned by an administrator account, database client
programs older than the August 2025 updates, a target database server older
than 13.8 or 14.5, or a target whose access rules refuse the temporary
account's sign-in.

### Corrections from the alpha.142 lab

- **A subdomain inside your own domain.** A customer whose domain lies under
  one of the server's own names, or under a name the administrator holds
  (for example the main domain), can add subdomains and aliases inside that
  domain again. Everything else the name rule refuses stays refused: the
  server's or the administrator's name itself, a name above it, a name under
  it outside the customer's own domain, and a server name inside the
  customer's domain (for example the server's mail address). Names of other
  customers are never reached this way. Such a domain, and a subdomain made
  under it, are still listed in **Problems now** as meeting one of the
  server's names; whether they should be is an open question to the owner.
- **The administrator is told the nearest holder.** When the administrator
  is refused placing a name into another account, the sentence names that
  account's nearest name and its holder, not a far name of the server. A
  customer's or reseller's sentence is unchanged and the same for every
  cause.
- **A Node.js site whose application failed can be deleted** in one press.
  Before, the deletion said "not completed" and the domain stayed "waiting
  for deletion" until `systemctl reset-failed` was run as root.
- **Kept file modes are named.** The one-time mode correction of alpha.142
  now names and counts in the agent's log every file mode the owner chose
  that it kept (before: "0 kept" for a stricter mode). It changes no mode it
  did not change before.

## Known limits

- Arch Linux's OpenDKIM package cannot tie signing to the login: there, mail
  sent with a login is still signed for any hosted domain its "From" line
  names. Port 25 is closed to signing there too, and **Problems now** says
  so.
- A PostgreSQL database in which the panel found something it does not
  decide by itself stays open to other accounts until the administrator
  closes it on the PostgreSQL page. Databases the panel does not record (for
  example made by hand) are not touched.
- A subdomain made inside a customer's own domain under a server name is
  listed in **Problems now** like its parent domain (above). The
  administrator giving a customer a new name under a server name is not
  covered by this release.
- A PostgreSQL server whose templates are SQL_ASCII has still not been
  checked in a lab.
- Still open from alpha.142: the web server's group can still read every
  site's folder (the link rule is the barrier between sites until each site
  gets its own group); every site's account is listed in the system's
  account list and named from the domain name; mail's address maps are
  readable by every account; PHP and scheduled tasks run outside the
  per-site sandbox and its limits; running processes are visible to every
  account; a site deployed from a Git repository may clone from an address
  on the server itself; testing a remote backup destination can reach
  addresses inside the server's network.

## Checked in the lab

**OPEN.** No lab run yet.

### What to expect after updating

- The update takes a few minutes and says by itself when it has finished.
- The layout is schema 81: going back to alpha.142 means restoring the
  snapshot or backup taken before the update.
- Mail sent with a mailbox login is signed only for its own domain; a
  website handing mail to port 25 without a login is no longer signed.
- Within the first hours, PostgreSQL databases made before alpha.141 are
  closed to other accounts where that is safe; the others are listed for
  the administrator in **Problems now** and on the PostgreSQL page.
- A customer whose domain lies under a server name or an administrator's
  name can add subdomains of it again.

## Evidence

Schema 080 → 081: migration 081 drops the six triggers of 080 and creates
them again under the same names with the own-subdomain exemption; no table,
column or row. A database holding overlaps made before upgrades and keeps
working (`TestSchema80DatabaseWithServerZoneOverlapsUpgradesAndLetsOwnSubdomainsIn`);
an older release refuses a ledger with 081
(`TestOlderReleaseRefusesALedgerWithTheOwnSubdomainEntry`). The
release-recovery lab pins schema 81. New, all additive: the mail filter
answer's `signing` field, two kinds in Problems now (`mail_signing`,
`postgresql_databases_open`), the build-gated agent calls
`PostgreSQLPublicAccessScanV1`, `CloseV1` and `UndoV1`, and panel setting
rows `postgresql_public_v1:<database id>` and `postgresql_public_list_v1`.

Tests that perform each closed way and fail on the code before, passing on
this release's:

- Mail: `TestMailSigningKeyFollowsTheLoginNotTheFromHeader`; component tests
  of the port-25 filter list, the write order, the owner's settings kept,
  undo on a failed write or reload, check before use and a build without
  the option (`cmd/agent/mail_signing_bind_test.go`). In the distribution
  configuration matrix, the real OpenDKIM of Debian 12, Debian 13, Ubuntu
  22.04 and Ubuntu 24.04 signed a login of `a` with a "From" of `b` as `a`,
  a null sender with a login of `a` as `a`, local mail without a login by
  its "From" domain, and gave an unauthenticated outside client no
  signature; on Arch the earlier configuration is accepted.
- PostgreSQL: `TestS1bMovingADatabaseNeverRunsItsCodeWithAdministratorRights`
  and `TestS1bClosingAnOldDatabaseKeepsItsUsersAndRefusesOthers` on private
  PostgreSQL 17 servers; the panel's decisions and the page in
  `cmd/panel/postgresql_public_access_test.go` and
  `web/tests/pg-public-access-s1b.test.mjs`.
- Corrections: `TestOwnSubdomainUnderAServerNameRuleAndTriggersAgree`,
  `TestOwnSubdomainUnderAnAdministratorsNameRuleAndTriggersAgree`,
  `TestOwnSubdomainUnderTheServersZoneIsCreated`,
  `TestNameRuleAdministratorIsToldTheNearestHolder`,
  `TestAppUnitRemovalForgetsAFailedUnit`,
  `TestTenantIsolationEarlierFilesAreTightenedOnce`,
  `TestTenantIsolationModesAreTightenedOnceThenLeftToTheOwner`.

Tests, each package's full suite in the test machine, before (alpha.142)
and after:

| Package | Before | After |
|---|---|---|
| `internal/transport` | 62 | 62 |
| `internal/db` | 182 | 184 |
| `internal/services` | 371 | 371 |
| `cmd/agent` | 5854 | 5878 |
| `cmd/panel` | 4726 | 4737 |

(Passed tests and subtests; the whole Go module 15838 → 15875. In this test
machine 19 tests of `cmd/agent` fail identically before and after, because
of the machine itself: its DNS engine state does not authorize PowerDNS, and
its temporary folders are not owned by the root group.)

No new failure. Web: `npm test` 3090 → 3098 passed, 0 failed; the type
check is clean and the build meets every size budget (first-load path
109.38 KiB of 110.00 KiB compressed; alpha.142: 109.37).
