# v0.1.0-alpha.144

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.144.tr.md)

> **Published** on 2026-11-22 (UTC): release commit `99e65fc3d`, GitHub pre-release at 22:55:55Z, download portal at about 22:59Z (public check 27/27).

**A small release.** It closes one security gap in webmail's own address,
brings the six corrections the alpha.143 lab asked for and one correction of
the traffic figures, and adds one small thing: an alias inside a domain of
the same account now answers in DNS. Nothing else is new, and the panel's
database layout does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.143

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   Going back to alpha.143 is the ordinary rollback; no snapshot has to be
   restored.
3. At its first start after the update the agent does two things by itself,
   once: on a server with a webmail address it writes that address's web
   server file in the new form and reloads the web server (see "Security:
   what is closed"), and it makes CelikPanel's log folder passable for the
   web server's workers (see "Traffic figures after the nightly log
   rotation"). There is nothing to press. What it cannot do is one line in
   **Problems now** with the step.

## Security: what is closed

- **Webmail's own address gave out files that are not part of its pages.**
  Since alpha.110, webmail's own address (Roundcube's separate web address)
  gave out every file of a Roundcube plugin's folder that is not PHP, as it
  is, to anyone who could open the address: for example a copy of a
  plugin's configuration (`config.inc.php.bak`, a filled-in
  `config.inc.php.dist`), or a plugin's `.sql`, `.log`, `.ini`, `.yml` or
  `composer.json` file. It also ran any plugin's PHP file directly. Roundcube's
  own program folders were not reachable. Now the address runs only webmail's
  own entry point (`index.php`) and gives out only the static files webmail's
  pages load (scripts, style sheets, images, fonts, the new-mail sounds, the
  PDF and TIFF files of the browser check, and the skins' pages); everything
  else answers "not found", and a name that looks like a backup, a dump, a log
  or a settings file is refused everywhere.
- **Existing servers are corrected by themselves** at the agent's first start
  after the update: the new file is written, checked with `nginx -t`, and the
  web server reloaded. If the check or the reload fails, the earlier file is
  put back exactly, and webmail and every site keep serving.
- **A webmail address file you changed by hand is kept** as it is;
  CelikPanel's text is put beside it as `<file>.celikpanel-pending` (the web
  server does not read it). **Problems now** then says in one line that
  webmail's address still gives out plugin files, that the server owner acts,
  and the step: it names that text, or, when CelikPanel could not write it
  beside your file, says to move your file away and press **Set up again**
  for the address on Roundcube's page. It does the same when the new text was put back because the
  web server's check failed in another file, and names that file. The agent
  tries again at every start; the line goes by itself once the file is in
  the new form.
- The panel's own `/webmail/` only sends the browser on to that address (since
  alpha.110) and is unchanged. The webmail server that listens only on a local
  socket is not reachable from the network and keeps its earlier rules.

## What changes for you

### Corrections from the alpha.143 lab

- **A mailbox created while you leave the page** is created completely: the
  panel finishes the creation and its record even when the page is left or
  the connection ends, and the same press arriving twice creates it once.
  The same holds for saving an out-of-office reply.
- **A mailbox found on the server without a record.** Creating a mailbox
  whose name the server already holds, without a record in the panel, no
  longer answers with an internal error, and nothing is changed. A customer or
  reseller reads that the mailbox exists on the server and is asked to turn to
  the administrator or choose another name. The administrator is offered
  **Adopt into this domain**: it works only for a mailbox that holds no mail
  and is in the form CelikPanel writes, and it always sets the new password
  typed in the adoption window, so no earlier password works. A mailbox with
  mail in it, or one whose folder cannot be checked, is not adopted: the
  server's owner decides on the server what becomes of it. An adopted mailbox
  is an ordinary one from then on: it keeps the size limit found on the
  server, and the domain's signing key and the first-day sending limit apply
  to it.
- **A PostgreSQL close whose page was left** is no longer recorded as "sent,
  not yet confirmed": the close and its record finish, and back on the page
  the close shows its real result. The same holds for Undo and Read again.
- **The window of a failed close** ends on "Not done — nothing was changed"
  (or, while the result is not known, "Result not confirmed yet — not sent
  again"), no longer on "Working on the server…" with a red mark. Its reason
  first names the kind of cause in the page's language (the database is in
  use, the time ran out, the connection failed, a right is missing, a role or
  database is missing, the database server refused), then the database
  server's own message in quotes.
- **Refusals in the page's language.** The alias and certificate refusals
  that came in English on the Turkish page are said in the page's language: a
  certificate that could not be reissued for the alias or does not cover it,
  an invalid alias, a name already in use, an alias not found, a removal that
  HSTS holds back, no active certificate to remove. The answers themselves
  are unchanged.
- **"1 PostgreSQL database":** one database is said in the singular in
  **Problems now** and in the alert mail.
- **Traffic figures after the nightly log rotation.** Traffic figures were
  under-counted after each night's log rotation until the web server was next
  reloaded: the web server's workers could not reopen CelikPanel's traffic
  logs (one "Permission denied" error line per log every night) and kept
  writing into the rotated file, which the next night's rotation removed. At
  the first start after the update, CelikPanel's log folder
  `/var/log/celikpanel` becomes passable (other accounts may pass through it,
  not list it), and its `traffic` folder becomes passable for the web
  server's workers only (0710, owned by root and the web server's group):
  no site account can open another site's traffic log (see "Known limits"
  for programs that run as the web server's own account). A traffic log that
  other accounts could read (the web server could make the first one
  readable by everybody) is closed to them; a site's first log, and each new
  log of the nightly rotation, is now made closed for the web server's
  account. A folder mode you set yourself is kept with its owner and group,
  and so is a `traffic` folder at 0710 whose group is neither the web
  server's nor one CelikPanel gave it (root, celikpanel); a link you made is
  kept too. Each is named in the agent's log; the certificate tool's log
  folder stays closed. No reload is needed:
  from the first night after the update no such error line is logged, and
  traffic is counted completely again without waiting for a reload. New
  installations make the folders and logs this way from the start.

### New, small: an alias answers in DNS

- An alias that lies inside a domain of the same account whose DNS this
  server keeps now gets the site's address records in that domain's zone,
  published with the zone (a backup DNS server receives them through the
  zone's ordinary transfer). No earlier release did this: an alias was only a
  name in the site's web server file and certificate. When a certificate is
  issued again for the alias, the record is published before the certificate
  authority checks the name, and withdrawn if the alias is not added.
- The record goes with the alias, also when the alias's domain is deleted
  and after its subscription moved to another account. Only what CelikPanel
  added is removed, and only while the zone belongs to the account of the
  alias's domain: in a zone that now belongs to another account nothing is
  removed, and the panel's log names the record to look at.
- If the publication cannot finish (the backup DNS server waiting, the DNS
  publisher not ready), the alias row says "DNS record waiting to be
  published — retried every hour, nothing to do", and the panel's hourly round
  publishes it. It never stops a domain's deletion.
- An address record you made yourself at that name is never replaced or
  removed, and nothing is added beside it: the alias answers "kept", and the
  page says to point the name to this server in the DNS records if the alias
  should reach this site.
- What it does not do: an alias that is a domain of its own (not inside a
  domain of the same account kept here) still gets no zone; an alias made
  before this release gets its record only when it is removed and added
  again.

## Known limits

- The kind of cause of a failed PostgreSQL close is recognised from the
  database server's English message. On a server whose PostgreSQL messages
  are in another language, every failure reads as "the database server
  refused it", still with its quoted message.
- Other changes still run on the browser's request: if the page is left
  while the server works, such a change can finish on the server without the
  panel's record, or only its answer is lost. Among them are scheduled tasks,
  mail forwarders and the catch-all address, a mailbox's password, size limit
  and deletion, DKIM, a site's SSL and PHP settings, file access, DNS setup
  and DNSSEC, the VPN, deleting a backup, installing or removing a catalogue
  application, and a database import; the full list is in this release's
  pull request (#355) for a later release.
- An alias that is a domain of its own gets no zone (the owner decides
  whether it should), and aliases made before this release get their record
  only when added again (above).
- Until the first nightly rotation after the update, the web server's
  workers write where they wrote before; counting is complete from that
  night.
- Programs that still run as the web server's own shared account can read
  the traffic logs: Node.js sites not yet moved into their own account, and
  the distribution's default PHP pool where it is on. These logs hold only
  the time and byte counts of requests, no addresses; those programs can
  already read every site's served files.
- The webmail server that listens only on a local socket keeps its earlier
  rules; it is not reachable from the network.
- Everything alpha.143's notes list as still open stays open
  ([alpha.143 release notes](RELEASE-NOTES-v0.1.0-alpha.143.md)).
- **Webmail cannot send mail** (reading mail, attachments and settings
  work); until alpha.145, send from a mail program. The lab found it and two
  small display findings, each with what to do until alpha.145: see "Found
  in the lab, planned for alpha.145" below.

## Checked in the lab

Lab check finished: one run on a pair of test servers, a Debian 13 main DNS
server with PowerDNS and an Ubuntu 24.04 backup DNS server with BIND
(22 November 2026, UTC 19:02 to about 20:30 with the clean-up). It started
from the alpha.143 state and followed the owner's path: before the update
the owner gave webmail its own address on the main server through the
page, and both servers were updated through the panel's own update page to
the release's code (`6ed1f94a0`, under the lab-only label
v0.1.0-alpha.152). Two customers of one reseller, in separate accounts,
played the two sides: alice, and bob, whose site's own account tried to
read her traffic log. Everything was done through the page's own buttons
except where it says the lab did it. No hard failure.

- **Before the update webmail's address was open** (alpha.143's code): a
  backup copy of a plugin's configuration (`plugins/archive/config.inc.php.bak`)
  and a text file the lab had planted in that plugin's folder were given out
  with their content, a plugin's `composer.json` was given out, and a
  plugin's PHP file (`plugins/archive/archive.php`), asked for directly, was
  handed to PHP and run (it answered with a PHP error).
- **Both servers updated through the panel's own update page by
  themselves**, in about a minute and a half each. No schema step (it stays
  81); one panel and one agent ran on each server; no new failed service.
  On the main server, which has the webmail address, the web server was
  reloaded once; a check every second from outside saw a site and webmail
  answering throughout the update. The backup server's web server files
  did not change and it was not reloaded.
- **Webmail's address after the update** (the security correction):
  - the planted backup copy and text file, the plugin's PHP file, a
    `config.inc.php.dist`, `composer.json` and `.sql`, `.log` and `.md`
    files answered "not found", and dot-files (`.htaccess`, `.env`,
    `.git/config`) were refused; scripts, style sheets, pictures, the
    browser check's file (`blank.tiff`), a skin's page and the entry point
    (`index.php`) still answered;
  - alice walked through webmail in a browser as a customer: she signed in,
    listed her inbox, opened a message with a PDF and a text attachment and
    downloaded both, opened the editor, saved a setting, opened a plugin's
    settings and the contacts; no request failed and no console error was
    logged. Sending failed: see "Found in the lab", 1;
  - an address file the owner had changed was kept byte for byte, with the
    panel's text beside it and one line in **Problems now** (Turkish and
    English); once the owner took the panel's text and reloaded the web
    server, the line went by itself;
  - with another web server file broken on purpose, the earlier address file
    was put back exactly, sites and webmail kept answering, and the
    **Problems now** line named the broken file. Once that file was gone,
    the agent's next start wrote the new form and the line went.
- **Mail.**
  - A mailbox created while the page was left at once has its record and
    signs in.
  - Creating a mailbox that the server held without a record was told to
    the customer plainly: it exists on the server, nothing was changed, ask
    the administrator or choose another name.
  - The administrator adopted it with a new password: the new password signs
    in, the old one no longer does.
  - A mailbox holding a message was refused for adoption; its password was
    not replaced and its message was kept.
  - Adopting a mailbox through another account's domain was refused with
    "not found" (404), saying nothing about that domain; an address of
    another domain was refused too.
  - An out-of-office reply saved while the page was left at once was
    stored.
- **Aliases in DNS.**
  - An alias inside alice's own domain answered from both servers at the
    first look and stopped answering on both once removed.
  - A name at which she already had her own address record was left as it
    was, nothing was added beside it, and the page said so.
  - An invalid and a duplicate alias name were refused in the page's
    language (Turkish).
  - With the backup server unreachable, an alias was saved and its row said
    "waiting to be published"; shortly after the backup server came back
    its record was published and answered from both servers (for the row
    itself see "Found in the lab", 2).
- **PostgreSQL.**
  - A close whose page was left at once showed its real result on return.
  - A close while the database's owner held a lock, with the page left
    before the server answered, showed on return that it was not done, that
    nothing was changed and why (the database was in use and the lock did
    not come in time), with the server's message; it was not sent again.
  - Undo through the page gave the earlier rights back.
  - A failed close (a recorded user's role dropped by the lab) ended on "Not
    done — nothing was changed", named the kind of cause (something it names
    does not exist on the database server) and quoted the server, in
    Turkish and English (for its title see "Found in the lab", 3).
- **Traffic logs.**
  - After the update `/var/log/celikpanel` went from 0750 to 0751 (other
    accounts may pass through it, not list it), its `traffic` folder from
    0755 to 0710 for root and the web server's group, and the certificate
    tool's folder stayed closed (0700); no traffic log was readable by other
    accounts; the agent's log said so once; the rotation file was rewritten
    once.
  - On the backup server, a `traffic` folder whose group the lab had set by
    hand was kept and named in the agent's log.
  - A new site's first log was made closed (0640, the web server's account).
  - bob's site's own account could neither read alice's site's traffic log
    nor list the folder, before and after a forced rotation.
  - After the forced rotation no "Permission denied" line was logged and
    new requests landed in the new log.
- The usual pages on both servers loaded without an error.

**Where the lab left the page's path.** As root on the lab servers, the lab
planted a backup copy and a text file in a webmail plugin's folder; changed
webmail's address file as an owner would (a comment line), restarted the
agent, then took the panel's text over that file and reloaded the web
server; broke another web server file on purpose, restarted the agent, and
removed the file again; handed test messages over on the server; set the
`traffic` folder's group by hand on the backup server; put a small script
into bob's site to read another site's traffic log; forced a log rotation;
made the backup server unreachable from the main server and reachable
again; and registered the certificate tool with the lab's own certificate
authority. Once, with the panel stopped, it wrote to the lab panel's
database, removing two mailboxes' panel rows, to make mailboxes the server
holds without a record (two tries at leaving the page on alpha.143's code
had not left a mailbox without its row). As the database administrator it
put a database into the state of ones made before alpha.141 and dropped a
recorded user's role; as that database's owner it held a lock. The two
adoption refusals across accounts and domains were sent to the panel's API
directly.

**Not checked:**

- The singular wording of the **Problems now** line for one open
  PostgreSQL database: the state could not be reached, because databases
  being watched do not count as open; covered by tests.
- Undo and Read again with the page left: only the close was tried with the
  page left; covered by tests.
- The traffic figure on a site's page after the rotation: time.
- Deleting a domain while the backup server is unreachable: time; covered by
  tests.
- Removing an alias after its subscription moved to another account: the
  page offers a move only as a request and an approval; covered by tests.
- An alias with a certificate (issued again for the alias, or refused) and
  the Turkish page's certificate and HSTS refusals: not part of this run;
  covered by tests.
- The **Logs** page in the walk through the usual pages: the address the lab
  used is not a page of its own, so the page was not opened.
- Webmail on the backup server: it has no webmail address in the lab.
- The database web tools: not installed on the lab pair (carried from
  alpha.142, still open).
- PostgreSQL versions other than the lab server's 17, a server whose
  PostgreSQL messages are not in English, and a server whose templates are
  SQL_ASCII.

### Found in the lab, planned for alpha.145

None of these is a security problem or data loss.

1. **Webmail cannot send mail.** Sending from webmail answers with an
   authentication error: the panel's webmail configuration hands mail to the
   mail server's submission port without first switching the connection to
   encryption, and the mail server accepts a login only on an encrypted
   connection. Reading mail, attachments and settings work. This is older
   than this release: the same configuration text is in alpha.143, which the
   owner's servers run; it was found now because this was the first lab run
   with a webmail address. Since which release webmail could not send was
   not established. Until alpha.145: send from a mail program (port 587 or
   465, with the mailbox's full address and its password).
2. After the backup server came back, an alias's row kept saying "waiting
   to be published" until the next hourly round (here about half an hour)
   although its record already answered on both servers; meanwhile the
   zone's own notice said to make the change again, while the alias row said
   nothing needs doing. Nothing to do: the hourly round clears the row.
3. The window of a failed PostgreSQL close keeps its in-progress title
   ("Closing … to other accounts") above "Not completed". The steps and the
   reason below it are right.

## What to expect after updating

- The update took under two minutes per server in the lab and says by
  itself when it has finished.
- No schema step: the layout stays schema 81, and going back to alpha.143 is
  the ordinary rollback.
- On a server with a webmail address, the web server is reloaded once at the
  first start, after its configuration check passes; from then on webmail's
  address gives out only what its pages load.
- Webmail still cannot send mail (known limit; "Found in the lab", 1):
  send from a mail program until alpha.145.
- The log folders' modes change once at the first start:
  `/var/log/celikpanel` from 0700 to 0711, or from 0750 to 0751; its `traffic`
  folder from 0755 to 0710, owned by root and the web server's group
  (`www-data` on Debian and Ubuntu, `http` on Arch Linux); traffic logs
  readable by other accounts from 0644 to 0640. A mode you set yourself (with
  its owner and group), a `traffic` folder at 0710 with a group of your own,
  or a link you made, is kept. CelikPanel's rotation file
  `/etc/logrotate.d/celikpanel-traffic`, if you have not changed it, is
  rewritten once so that each new traffic log is made 0640 for the web
  server's account; one you changed is kept. From the first night after the
  update the traffic figures are complete.
- A mailbox that the server holds without a panel record is said plainly;
  only the administrator can adopt it, only when it holds no mail, and with a
  new password.
- After going back to alpha.143: alpha.143 does not recognise the new webmail
  address file and keeps it as if you had changed it by hand, so webmail keeps
  serving and nothing is opened again; webmail address changes are then
  refused as hand-edited until you update again. An alias record that
  alpha.144 added stays when the alias is removed under alpha.143; remove it
  in the DNS records.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New, all additive: the build-gated, read-only agent call
`MailboxOnServerV1`; the read-only agent call `WebmailAddressFormsV1`; the
Problems now kinds `webmail_address_kept` and `webmail_address_refused`; the
refusal codes `MAIL_ACCOUNT_ON_SERVER_ONLY`, `MAIL_ACCOUNT_ADOPT_REFUSED`,
`MAIL_ACCOUNT_ADOPT_RECORD_NOT_WRITTEN`,
`ALIAS_INVALID`, `ALIAS_CONFLICT`, `ALIAS_NOT_FOUND`,
`ALIAS_CERTIFICATE_REISSUE_FAILED`, `ALIAS_CERTIFICATE_NOT_COVERING`,
`HSTS_REMOVAL_BLOCKED` and `SSL_NO_ACTIVE_CERTIFICATE` (statuses unchanged);
the panel setting rows `alias_dns_v1:<alias>`; the audit action
`mail.account.adopt`; the agent's one-time receipt `log-root-modes-v2.done`.
Four routes join the request-identity guard's operation steps (the
PostgreSQL close, undo and Read again; mailbox creation; the out-of-office
save; adoption); their stored digests never hold a password.

Tests by item:

- Webmail's address: `TestWebmailAddressServesOnlyRoundcubesStaticFilesAndEntryPoint`
  (reads the rendered file the way nginx selects a location: 21 names
  refused, a script, a style sheet, an image, a font, a skin's blank page, a
  sound and `index.php` served), `TestWebmailAddressEarlierFormServedPluginFiles`
  (the earlier form gave out `config.inc.php.bak` and ran a plugin's PHP),
  `TestWebmailAddressEarlierFormIsRewrittenAtStartAndAnOwnersFileKept`,
  `TestWebmailAddressFormsNameWhatStillServesPluginFiles`,
  `TestA144WebmailAddressInItsEarlierFormIsAProblemUntilItIsNot`; the
  distribution configuration matrix renders the new file
  (`TestRenderDistroConfigMatrix`, checked with nginx in CI).
- Alias in DNS: `TestA144AliasInsideItsAccountsZoneAnswersAndGoesAgain`,
  `TestA144AliasNeverTakesTheOwnersRecords`,
  `TestA144AliasRecordsSurviveAFailedPublicationAndArePublishedLater`,
  `TestA144AliasAddsNothingBesideAnOwnersAddressRecord`,
  `TestA144AliasPublicationThatWaitsIsRetriedAndNeverStopsADeletion`,
  `TestA144DeletingADomainWithExternalDNSRemovesItsAliasRecordsHere`,
  `TestA144AliasRecordsStayInAZoneNowHeldByAnotherAccount`,
  `TestA144AliasRecordsGoWithTheAliasAfterASubscriptionTransfer`,
  `TestA144AWaitingZoneWhoseModeCannotBeReadKeepsItsWait`.
- A close with the page left: `TestA144PGPublicCloseIsNotLostWhenThePageIsLeft`
  (with the route not guarded it reproduces the lab's record).
- The failed close: `TestA144PGFailureKindNamesTheCause`,
  `TestA144FailedCloseAnswersTheKindOfCause`.
- Refusals in the page's language: `TestA144AliasRefusalsAreCoded`,
  `TestA144AliasHandlersRefuseAnInvalidNameCoded`.
- The singular line: `TestA144OnePostgreSQLDatabaseIsSaidInTheSingular` and
  the web suite's problem-key tests.
- A mailbox with the page left and the mailbox found on the server:
  `TestA144MailboxCreatedWhileThePageIsLeftHasItsRow`,
  `TestA144AutoresponderSavedWhileThePageIsLeftHasItsCopy` (both fail with
  the guard removed), `TestA144MailboxOnTheServerWithoutARowIsSaidPlainly`,
  `TestA144AdministratorAdoptsAMailboxFoundOnTheServer` (refused with mail in
  the folder and with a folder that cannot be counted; the password replaced
  once, through the password step),
  `TestA144SecondAdoptionOfTheSameAddressNeverReplacesThePassword`,
  `TestMailboxOnServerReadsOnlyCelikPanelsOwnForm`,
  `TestMailboxMessageCountReadsCurAndNewOnly`.
- The traffic logs: `TestLogRootIsMadePassableWhateverTheUmask`,
  `TestASitesFirstTrafficLogIsMadeClosed`,
  `TestALogFolderThatIsTheOwnersLinkIsUsedAsBefore`,
  `TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes`,
  `TestLogRootKeepsTheOwnersLinks`, `TestCertbotStorageGoesOnThroughTheOwnersLogLink`,
  `TestTrafficLogsReachOnlyTheWebServerWorkers` (the web server's account
  reaches its log, before and after a rotation; an account that is neither
  root nor in the web server's group can open no traffic log by name and
  list nothing); they fail on the code before the correction. With the
  second review's corrections: `TestTrafficFolderAt0710WithTheOwnersGroupIsKept`,
  `TestTrafficLogNeedsTheWebServerAccountOnlyToMakeIt`,
  `TestTrafficSettingStaysWhenTheWebServerAccountCannotBeRead`,
  `TestTrafficLogrotateMakesEachNewLogClosed`.

The full Go module and web suites ran on the base (alpha.143's code) and on
the corrections in the corrections lane, and every check of the release pull
request passed in CI. An independent read-only review of the corrections
found nothing that blocks the release; its findings are corrected in this
release, except the one kept as a known limit above (the kind of cause read
from the database server's English message).
