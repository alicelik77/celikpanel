# v0.1.0-alpha.145

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.145.tr.md)

**A small release.** It closes one security gap in webmail's local door,
makes webmail able to send mail (it never could), and brings the two small
display corrections the alpha.144 lab asked for. Nothing else is new, and the
panel's database layout does not change.
**Only the owner updates installed panels, from the panel's own update
screen.**

## Updating from alpha.144

1. Update from the panel's own update screen, as before.
2. **The panel's database layout does not change** (it stays schema 81).
   Going back to alpha.144 is the ordinary rollback; no snapshot has to be
   restored.
3. At its first start after the update the agent does two things by itself,
   once: it writes the web server file of webmail's local door in its new
   form and reloads the web server (see "Security: what is closed"), and it
   rewrites webmail's own configuration so that it can send (see "Webmail can
   send"). There is nothing to press. What it cannot do is one line in
   **Problems now** with the step.

## Security: what is closed

- **Webmail's local door answered every account of the server.** The panel
  checks webmail through a local door: a socket on the server itself
  (`/run/celikpanel-webmail.sock`). The web server opens that socket for
  every account on the server, so any account there — a customer's site code
  or scheduled task — could use it without signing in. Through it, anyone on
  the server could read every file of webmail's public folder as it is,
  including Roundcube's plugin and skin folders: leftover copies of plugin
  settings (for example a plugin's `config.inc.php.bak`), dot-files,
  `composer.json`, `.dist`, `.inc` and SQL files; and they could run plugin
  scripts directly. It was **not** reachable from the network, and webmail's
  own configuration, its database and the mailboxes' passwords were never
  reachable this way. This was so since alpha.13 (since alpha.1 the same tree
  was open on the server's inside address 127.0.0.1).
- **Now the door answers only what CelikPanel's own checks ask for:**
  webmail's sign-in page (`/webmail/`, Roundcube's entry point, the only PHP
  that runs there), its style sheets, and CelikPanel's PHP check. Everything
  else answers "not found". The socket itself is not moved and its mode is
  not changed.
- **Existing servers are corrected by themselves** at the agent's first start
  after the update: the new file is written, checked with `nginx -t`, and the
  web server reloaded. If the check or the reload fails, the earlier file is
  put back exactly, the agent goes on starting, and **Problems now** says why
  and the step (correct what the check names, or the cause in the agent's
  journal, then restart the agent: CelikPanel writes the new file at that
  start).
- **A local door file you changed by hand is kept** as it is; CelikPanel's
  text is put beside it as `celikpanel-webmail.conf.celikpanel-pending` (the
  web server does not read it), and **Problems now** says in one line that the
  local door still gives out Roundcube's files, that the server owner acts,
  and the step. The line goes by itself once the file has CelikPanel's rules.

## What changes for you

### Webmail can send

- **Sending from webmail never worked in any earlier release** (reading mail,
  attachments and settings worked). Webmail spoke to this server's own sending
  service without encryption, and that service accepts a login only on an
  encrypted connection, so every message ended with "authentication failed".
- **Now webmail switches encryption on** and logs in with the mailbox's own
  address and password, as a mail program does. The connection stays inside
  the server (`127.0.0.1`), so on that inside address only, the certificate's
  name and chain are not checked: the sending service may show the mail
  host's certificate or, before one exists, a self-signed one, and sending
  must work in both cases. Nothing is relaxed for any other address.
- Mail sent from webmail is signed for the sender's own domain, like any other
  mail of that mailbox (alpha.143's rule, unchanged). Port 465 and 587 for
  mail programs are unchanged.
- **Existing servers are corrected by themselves** at the agent's first start
  after the update: webmail's configuration is rewritten once, with its mode,
  owner and secret key kept.
- **A webmail configuration you changed by hand is kept** as it is;
  CelikPanel's text is put beside it as `config.inc.php.celikpanel-pending`
  (readable by root only; Roundcube does not read it), and **one line in
  Problems now** says that webmail cannot send, why, and the step: copy that
  text (or its two sending lines) into your file; when no text could be put
  beside it, the line gives the two lines to write. If CelikPanel could not
  rewrite its own earlier text (a read-only file system, an immutable flag, a
  full disk), the line says to remove that cause and restart the agent — not
  to edit the file by hand. The line goes by itself once the file sends with
  encryption.
- The Webmail card does not say "installed and verified" while webmail cannot
  send.

### Corrections, small

- An alias's "waiting to be published" ends when its zone's held change is
  published, or when the zone's own publication that was waiting for the
  backup server completes — not an hour later. While it waits, the zone's
  notice no longer asks to make the change again: it says the alias records
  are published by themselves.
- The window of a PostgreSQL close or its undo ends with a title that says the
  outcome (closed, not closed, not known), never the running "Closing …".

## Known limits

- **What any account on the server can still reach through the local door:**
  webmail's sign-in page — the same page its public address shows, so
  Roundcube itself, with a login, which a rate limit keyed to the client's
  address does not see — and the PHP version (CelikPanel's PHP check names it).
  A style sheet reached through a link that root made inside Roundcube's
  `skins` folder is served.
- **An owner's own web server block that hands requests to that socket** (for
  example to keep webmail on a path of a site) now gets only the sign-in page
  and style sheets; use webmail's own address instead.
- **The "cannot send" check reads only the plain cases.** A hand-written
  sending setting with certificate options of the owner's own is taken as the
  owner's choice and not checked; an `smtp_host` inside a `/* */` comment is
  read as active. A configuration that switches encryption on by name without
  the certificate setting is named.
- **The Webmail card still offers Repair while webmail cannot send**; Repair
  does not correct the configuration. The step is in Problems now.
- **Going back to alpha.144:** webmail keeps sending and the local door stays
  closed, but alpha.144 does not know the new texts: it treats webmail's
  configuration and the local door's file as changed by the owner, so
  webmail's address lines, webmail's repair and moving webmail to another PHP
  version are refused as hand-edited until you update again. (If you delete
  the local door's file as that message says, alpha.144 writes its earlier,
  open form again.)
- The open list of alpha.144 stays open
  ([alpha.144 release notes](RELEASE-NOTES-v0.1.0-alpha.144.md)): among them,
  other changes that still run on the browser's request, and programs still
  on the web server's shared account reading traffic logs.

## Checked in the lab

**OPEN.** This release has not had its lab run yet. The lab list is in the
release pull request (#357): the local door before and after the update as a
site account; webmail sending in both certificate states on Debian 13 and
Ubuntu 24.04 with the path proven in the mail log; a failed rewrite and an
owner-edited file for both files; the alias wait on both paths; the
PostgreSQL window's titles.

## What to expect after updating

- No schema step: the layout stays schema 81, and going back to alpha.144 is
  the ordinary rollback.
- The web server is reloaded at the first start, once per file that is
  corrected (the local door's file; a webmail address file only if it is
  still in an earlier form), each after its configuration check passes.
- Webmail's configuration is rewritten once at the first start, with its
  mode, owner and secret key kept; webmail can then send. Nothing to do by
  hand.
- If **Problems now** shows a webmail line, the step is in it.

## Evidence

No schema change: no file under `internal/db/migrations/` changes; the
layout stays 81. New, all additive: the fields `sending`, `sending_file` and
`sending_reason` of the read-only agent call `WebmailAddressFormsV1` (and an
entry with no address for the local door); the zone answer's
`aliases_waiting`; the component record's `webmail_cannot_send`; the
Problems now kind `webmail_cannot_send` and the mail-profile code
`webmail_cannot_send`. No new agent call, route or audit action.

Tests by item:

- The local door: `TestA145LocalServerAnswersOnlyCelikPanelsOwnChecks`
  (reads the rendered file the way nginx selects a location: the sign-in
  page, the entry point, the style sheets and the PHP check answer; a plugin
  copy, a plugin's PHP, dot-files, `composer.json`, `index.php/extra` and a
  `..` path are not found; the earlier form served and ran them),
  `TestA145ProofAndReadinessStillAnswerOnTheLocalServer`,
  `TestA145EarlierLocalServerIsRewrittenAtStartWithOneReload`,
  `TestA145OwnersLocalServerFileIsKeptWithTheTextBesideIt`,
  `TestA145LocalServerRewriteFailuresThatAreNotAnotherFileAreUnnamed`,
  `TestA145LocalServerPendingTextGoesWithTheFile`,
  `TestA145LocalServerInItsEarlierFormIsAProblemUntilItIsNot`,
  `TestA145LocalServerAlertMailFollowsTheScreensBranches`; the distribution
  configuration matrix renders the new file (`TestRenderDistroConfigMatrix`,
  checked with nginx in CI). An independent review also ran the new file on a
  real nginx (Debian 13): only the listed requests answer, by any encoding.
- Webmail sends: `TestA145WebmailSendsWithEncryptionAndRelaxesTheCheckOnLoopbackOnly`,
  `TestA145SendingSettingIsReadFromTheLastAssignment`,
  `TestA145EarlierConfigurationIsCorrectedAtTheFirstStart`,
  `TestA145AddressLinesOnTheEarlierTextWriteTheCurrentOne`,
  `TestA145OwnerEditedConfigurationIsKeptWithTheTextBesideIt`,
  `TestA145OwnerSendingSettingIsNotNamed`,
  `TestA145FailedRewriteOfCelikPanelsTextIsItsOwnState`,
  `TestA145RewrittenConfigurationIsPublishedWithItsFinalOwner`,
  `TestA145WebmailThatCannotSendIsAProblemUntilItCan`,
  `TestA145FailedRewriteIsNamedWithTheRestartNotAHandEdit`,
  `TestA145TheOwnerIsGivenCelikPanelsTwoSendingLines`,
  `TestA145WebmailProfileIsNotCompleteWhileItCannotSend`; the existing
  submission, signing and webmail address tests are unchanged and pass.
- The alias wait: `TestA145AliasWaitEndsWhenTheHeldPublicationCompletes`,
  `TestA145AliasWaitIsSettledWhenThePendingPublicationCompletes`,
  `TestA145AliasWaitSettledByTheFollowUpPublicationWithoutASecondOne`.
- The texts, the PostgreSQL window's titles and the zone notice:
  `web/tests/a145-webmail-send-lab.test.mjs`.

The Go suites of the touched packages and the web suite ran on the base
(alpha.144's code) and on this release's code with the same failing set by
name (two tests that need a root-owned temporary folder in the test machine),
and every check of the release pull request passed in CI. Two independent
read-only reviews of the pull request found nothing that blocks the release;
their findings are corrected in it or listed above.
