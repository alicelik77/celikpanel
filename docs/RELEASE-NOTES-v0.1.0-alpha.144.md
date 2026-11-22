# v0.1.0-alpha.144

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.144.tr.md)

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
  no other account, a site's programs included, can open another site's
  traffic log. A traffic log that other accounts could read (the web server
  made the first one readable by everybody) is closed to them; a site's
  first log is now made closed before the web server opens it. A folder mode
  you set yourself, or a link you made, is kept and named in the agent's
  log; the certificate tool's log folder stays closed. No reload is needed:
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
- The record goes with the alias, also when the alias's domain is deleted.
  Only what CelikPanel added is removed.
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
- The webmail server that listens only on a local socket keeps its earlier
  rules; it is not reachable from the network.
- Everything alpha.143's notes list as still open stays open
  ([alpha.143 release notes](RELEASE-NOTES-v0.1.0-alpha.143.md)).

## Checked in the lab

**Open.** This release has not been run in the lab yet. The planned run, on
the pair of test servers (a Debian 13 main DNS server with PowerDNS and an
Ubuntu 24.04 backup DNS server with BIND), updated from the alpha.143 state
through the panel's own update page:

1. Webmail: before updating, a copy of the webmail address file; after it,
   the agent's log says the file was rewritten; sign in, list a mailbox, open
   a message with an attachment (a PDF opened, a text file downloaded),
   compose with the editor, no console error; a planted
   `plugins/archive/config.inc.php.bak` and `plugins/archive/archive.php`
   answer 404, `program/js/app.min.js`, `program/resources/blank.tiff` and
   `skins/elastic/watermark.html` answer 200, `index.php/x.txt` gives
   Roundcube's page. An address file changed by hand is kept with
   `.celikpanel-pending` beside it and named in **Problems now**; with another
   web server file broken, webmail and every site keep answering, the address
   file is unchanged and **Problems now** names the broken file.
2. The traffic logs: the folder and log modes after the update, a forced
   rotation without any "Permission denied" line, requests landing in the new
   log, the agent's log block only once, and, as a site's account, reading
   another site's traffic log refused.
3. An alias in DNS: answered from both servers and gone after removal; with a
   certificate; a refused issuance leaves no record; next to an owner's
   record nothing is written or added; a domain deleted while the backup
   server is unreachable completes, and the hourly round publishes the
   alias's zone.
4. A PostgreSQL close with the page left, and a failed close's window and
   reason in both languages; a server whose PostgreSQL messages are not in
   English.
5. The Turkish page's alias refusals (an invalid and a duplicate alias, the
   certificate step, HSTS) and the singular line.
6. A mailbox and an out-of-office reply saved with the page left; a mailbox
   found on the server: a customer's sentence, the administrator's adoption
   with a new password (the earlier password no longer signs in), a mailbox
   with mail refused, another account's or another domain's address refused.

## What to expect after updating

- No schema step: the layout stays schema 81, and going back to alpha.143 is
  the ordinary rollback.
- On a server with a webmail address, the web server is reloaded once at the
  first start, after its configuration check passes; from then on webmail's
  address gives out only what its pages load.
- The log folders' modes change once at the first start:
  `/var/log/celikpanel` from 0700 to 0711, or from 0750 to 0751; its `traffic`
  folder from 0755 to 0710, owned by root and the web server's group
  (`www-data` on Debian and Ubuntu, `http` on Arch Linux); traffic logs
  readable by other accounts from 0644 to 0640. A mode you set yourself, or a
  link you made, is kept. From the first night after the update the traffic
  figures are complete.
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
  `TestA144DeletingADomainWithExternalDNSRemovesItsAliasRecordsHere`.
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
  once, through the password step), `TestMailboxOnServerReadsOnlyCelikPanelsOwnForm`,
  `TestMailboxMessageCountReadsCurAndNewOnly`.
- The traffic logs: `TestLogRootIsMadePassableWhateverTheUmask`,
  `TestASitesFirstTrafficLogIsMadeClosed`,
  `TestALogFolderThatIsTheOwnersLinkIsUsedAsBefore`,
  `TestLogRootBecomesPassableOnceAndKeepsTheOwnersModes`,
  `TestLogRootKeepsTheOwnersLinks`, `TestCertbotStorageGoesOnThroughTheOwnersLogLink`,
  `TestTrafficLogsReachOnlyTheWebServerWorkers` (the web server's account
  reaches its log, before and after a rotation; an account that is neither
  root nor in the web server's group can open no traffic log by name and
  list nothing); they fail on the code before the correction.

The full Go module and web suites ran on the base (alpha.143's code) and on
the corrections in the corrections lane, and every check of the release pull
request passed in CI. An independent read-only review of the corrections
found nothing that blocks the release; its findings are corrected in this
release, except the one kept as a known limit above (the kind of cause read
from the database server's English message).
