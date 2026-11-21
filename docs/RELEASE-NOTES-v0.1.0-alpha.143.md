# v0.1.0-alpha.143

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.143.tr.md)

**A security release: please update soon.** It closes two ways in which
one customer could act as another: outgoing mail signed with another
customer's domain, and PostgreSQL databases made before alpha.141 that
every account of the server could still enter. It also
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
   changed by hand is kept and named in **Problems now**. An out-of-office
   reply that answered from an address not its own is set again without
   that address, and its window says so once. There is nothing to press.
4. A few minutes after the start, once the update is verified and the
   server is quiet, the panel starts watching the PostgreSQL databases made
   before alpha.141 that it may close by itself, and closes each one after a
   full day in which nobody else was seen using it, checking every hour (see
   "PostgreSQL databases made before alpha.141").
5. **Going back to alpha.142 opens the closed ways again.**

## Security: what is closed

- **Mail signed with another customer's domain.** The mail server signed
  any message whose "From" line named a domain hosted here, both for a
  mailbox sending with its login and for a website handing mail to the
  server without one, and an out-of-office reply could be set to answer
  from another customer's address. So mail carrying another customer's
  address could leave this server with that customer's valid signature.
  Now mail sent with a mailbox login is signed only for that mailbox's own
  domain, mail arriving on port 25 is never signed, and an out-of-office
  reply answers only from the mailbox's own addresses.
- **Older PostgreSQL databases open to every account.** Databases made
  before alpha.141 kept PostgreSQL's default, under which every database
  user of the server may connect to every database and make temporary
  tables in it: one customer's database user could sign in to another
  customer's database and see how it is built. Now these databases are
  closed to other accounts: by the panel itself after a day in which nobody
  else was seen using them, and with one button where the administrator must
  decide.

## What changes for you

### Outgoing mail and its signature

- **Mail sent with a mailbox login** (a mail program, webmail, a website
  that signs in on port 587 or 465) is signed for that mailbox's own domain,
  whatever its "From" line says. Mail whose "From" line names another
  customer's domain is no longer signed with that customer's key.
- **A website that hands mail to port 25 without a login** is no longer
  signed. To keep its mail signed, set the site's mail to sign in with one
  of its own mailboxes on port 587.
- **Out-of-office replies** answer from the mailbox's own address, or from
  an additional address forwarded to the mailbox from a domain of the same
  account. Another address is refused when the reply is saved. A reply
  saved earlier that named another address is set again without it at the
  first start after the update. Its window says once which addresses were
  removed, and the audit log has a row.
- **Mail made on the server itself** is signed for its "From" domain as
  before: the panel's notices (the owner's main domain), the system's own
  scheduled jobs (root's) and out-of-office replies. Only the system's root
  and mail accounts may hand mail over this way.
- **Incoming mail:** the mail signer no longer checks the signatures of
  mail arriving on port 25, and no longer adds its "Authentication-Results"
  line. Rspamd, where installed, still checks them for its spam score. With
  SpamAssassin they count only where its DKIM check runs. Without a spam
  filter nothing checks them. Nothing in CelikPanel used that line.
- Webmail, mail programs and forwards otherwise behave as before.
- **For the owner:**
  - The change consists of ordinary mail server settings that work with
    the panel stopped or removed.
  - Before CelikPanel uses a new signing setting, the mail signer checks
    it. If the mail server does not run with it, the earlier settings are
    put back exactly.
  - A restart of the mail signer that an interruption left undone is made
    at the next start.
  - A filter the owner adds to the mail server's list also stays on port
    25. CelikPanel's own port-25 setting is removed when the mail signer is
    removed.
  - A signing file or port-25 filter setting the owner changed by hand
    stays as it is (CelikPanel's text is beside it as
    `opendkim.conf.celikpanel-pending`). **Problems now** names the file or
    setting and the owner's concrete step.
  - **Problems now** also does this when the setting that decides which
    accounts may hand mail over without a login is not the panel's
    (`authorized_submit_users = root, vmail`), because sites' programs
    could then send mail signed for any domain here.
  - While the mail signer restarts, mail passes unsigned. Where the owner
    keeps his own filter list and his own `milter_default_action`, that
    setting applies instead: Postfix's default gives senders a temporary
    error, and they send again.

### PostgreSQL databases made before alpha.141

- The panel closes such a database by itself only when nobody else can be
  seen using it: its rights are still PostgreSQL's untouched default; no
  account the panel does not record for it owns anything in it, holds a
  right in it (also on a single column), or has a setting, a policy or a
  user mapping there; every user the panel records for it has its account;
  and its database server has no monitoring or replication account. Its
  owner and the users the panel records for it keep their way in; every
  other account can no longer connect to it or make temporary tables in it.
- Such a database is watched first: it is closed at the first hourly check
  at least 24 hours after its watch began, once at least 24 hourly checks
  have read it. A change of its rights, of the accounts inside it, of its
  owner or of its recorded users starts the watch again. An account the
  panel does not record for it seen connected even once ends the watch: the
  administrator decides, and the account is named. The **PostgreSQL** page
  shows since when each database is watched and when it closes by itself if
  nobody else connects; the administrator can close it at any time.
- Every other such database is left exactly as it is and listed in
  **Problems now** and on the **PostgreSQL** page with what was found, what
  every account can do there now (connect, or only make temporary tables),
  what closing changes, and one button to close it after a confirmation
  that names the accounts connected at the last reading.
- A close whose answer is lost is never taken as "nothing changed": the
  panel keeps its record and reads the database again before doing anything
  else. A close that fails is shown with its reason, its time and the step
  to take; it is not retried every hour, only when something about the
  database changes or the administrator presses Close. Two closes of the
  same database never run at once.
- Every close shows when and by whom it was made, with **Undo** (and the
  plain statements to undo it without the panel); undo gives back exactly
  what the close changed. A database the administrator opened again stays
  on the page as the administrator's choice, leaves **Problems now** and the
  alert mail, and is never closed again by itself.
- The page shows the last reading with its time; **Read again** reads every
  database of a database server in one connection.
- MariaDB has no such default: nothing changes there. PostgreSQL servers
  registered on other machines are not touched.

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

- Arch Linux's OpenDKIM package cannot tie signing to the login. There,
  mail sent with a login is still signed for any hosted domain its "From"
  line names. Port 25 is closed to signing there too. **Problems now** says
  so, and never advises a setting that OpenDKIM there refuses.
- An additional out-of-office address must be a forwarder the panel knows.
  An alias the server owner wrote by hand into the mail server's tables is
  refused.
- Replies are corrected from the panel's records. A CelikPanel reply script
  on a server whose panel record is missing is not rewritten.
- **Moving a database to another PostgreSQL server version** (an
  administrator action: only the administrator can start one) still runs
  the customers' database code in it with the administrator's rights during
  the move, as in alpha.141. Until a later release fixes this, do not move a
  database whose owner you do not trust.
- A PostgreSQL database in which the panel found something it does not
  decide by itself stays open to other accounts until the administrator
  closes it on the PostgreSQL page; one it may close by itself stays open
  for the day it is watched. Databases the panel does not record (for
  example made by hand) are not touched. The closing was tested on
  PostgreSQL 17 only.
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
- Mail sent with a mailbox login is signed only for its own domain; mail
  handed to port 25 without a login (for example by a website) is no
  longer signed.
- An out-of-office reply that named an address not its own loses that
  address at the first start, and its window says once which address was
  removed.
- A PostgreSQL database made before alpha.141 that nobody else is seen
  using is closed to other accounts by itself at the first hourly check at
  least 24 hours after its watch began, once at least 24 hourly checks have
  read it; until then the PostgreSQL page shows when it closes. The others
  are listed for the administrator in **Problems now** and on the
  PostgreSQL page, each with one button to close it.
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
`PostgreSQLPublicAccessScanV1`, `CloseV1` and `UndoV1` (with the answers
`unknown`, `missing` and `replication`), and panel setting
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
  signature; on Arch the earlier configuration is accepted. The
  independent review's cases: `TestMailAutoresponderRefusesAnotherAccountsAddress`,
  `TestAutoresponderAgentRefusesAnotherAccountsAddress`,
  `TestMailAutoresponderStoredForeignAddressIsRemovedAtStart`,
  `TestMailSigningLocalSubmitDecidesTheState`,
  `TestMailSigningPort25FollowsTheOwnersLaterMainList`,
  `TestMailSigningPort25SettingGoesWithOpenDKIM`,
  `TestMailSigningOwedRestartIsMadeAtTheNextStart`,
  `TestMailSigningUnknownReplacesAnEarlierInPlace`,
  `TestMailSigningUnsupportedOwnerIsNeverAdvisedTheLine`,
  `TestMailSigningRestartWindowIsSaid` and
  `TestMailSigningChainIsWrittenBeforeOpenDKIMRestarts`; in the matrix, with
  the release's real Postfix tools, an owner's later port-25 filter, exact
  rollback and an owner's `authorized_submit_users`; the words of each
  signing state in `web/tests/mail-signing-a143-fix.test.mjs`.
- PostgreSQL, on a private PostgreSQL 17.12 server:
  `TestS1bClosingAnOldDatabaseKeepsItsUsersAndRefusesOthers` and the
  independent review's cases
  `TestS1bFixACloseWhoseAnswerIsLostAfterCommitIsUnknown`,
  `TestS1bFixColumnRightsSettingsPoliciesAndMappingsCountAsInside` and
  `TestS1bFixARoleNamedToBreakOutIsCarriedSafely` (these three also fail on
  the first version of the change),
  `TestS1bFixARecordedUserWithoutItsRoleIsSeenAndItsGrantIsNotNothingChanged`,
  `TestS1bFixAReplicationSubscriberIsSeen` and
  `TestS1bFixAScanOf300DatabasesIsOneSession` (one connection, about 5 s);
  the panel's decisions (the day's watch, an unknown answer, two closes at
  once, a failed close, a database opened again, the last reading) in
  `cmd/panel/postgresql_public_access_test.go`; the page in
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
