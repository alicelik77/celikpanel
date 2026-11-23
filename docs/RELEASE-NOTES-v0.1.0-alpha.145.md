# v0.1.0-alpha.145

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.145.tr.md)

> **Published** on 2026-11-23 (UTC): release commit `2584a566e`, GitHub pre-release at 08:56:20Z, download portal at about 09:00Z (public check 27/27).

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
- **A longer change can stay "in progress" on the panel after it has
  finished on the server.** While a longer change runs — a webmail update,
  a component install, a mail profile install — the panel keeps in touch
  with its agent every few seconds. If that contact fails for a moment, the
  change still finishes on the server, but the panel can keep showing it as
  in progress and refuse other server changes with "another operation is
  running" (that message says to press again once it ends; here it does not
  end by itself). Nothing on the server is harmed: sites, mail and webmail
  keep working, and the change that was made stays. If a change has shown
  as in progress for more than about five minutes (a webmail update
  normally takes seconds), run `sudo systemctl restart celikpanel-panel` on
  the server. It restarts only the panel's web process; at its start the
  panel reads what was really done on the server and records the result. A
  webmail update is only read, nothing is run again; an interrupted install
  is resumed and completed on the server. The page may need a reload or a
  new sign-in.
  This was already so in earlier releases: the code of that path is
  unchanged in this release. In the lab it happened once on this release,
  during a Roundcube update whose preparation was unusually slow (the page
  said "updating" for about forty minutes until one panel restart recorded
  the update as done), and not when the same update was tried on
  alpha.144's code. It is to be fixed in a coming release.
- **A zone's general notice can lag behind a publication.** In the lab,
  about twenty seconds after a held DNS change had been published, the
  zone's general notice still said that new changes were waiting and to
  make the change again, while the alias row no longer said it was waiting.
  The publication itself was complete; if you see that notice right after a
  publication, look again a little later before making a change again.
- The open list of alpha.144 stays open
  ([alpha.144 release notes](RELEASE-NOTES-v0.1.0-alpha.144.md)): among them,
  other changes that still run on the browser's request, and programs still
  on the web server's shared account reading traffic logs.

## Checked in the lab

Lab check finished: one run on the pair of test servers, a Debian 13 main
DNS server with PowerDNS, mail and webmail on its own address, and an
Ubuntu 24.04 backup DNS server with BIND, which has webmail's local door but
no mail service (23 November 2026, UTC 05:21 to about 06:50 with the
clean-up). Both servers started from alpha.144's code and were updated
through the panel's own update page to this release's code (`e10c2ddb6`,
under the lab-only label v0.1.0-alpha.153). Two customers in separate
accounts played the two sides: alice, whose mailbox sent the mail, and bob,
whose site's own account tried the local door. Everything was done through
the page's own buttons except where it says the lab did it. No hard
failure; one limit older than this release was met (see "Known limits").

- **Before the update the local door was open** (alpha.144's code), on both
  servers: as bob's site account — through his site's PHP and directly on
  the socket — a backup copy of a plugin's settings and a dot-file that the
  lab had planted were given out with their content, and a PHP file planted
  in a plugin's folder was run, also when asked for through a
  `skins/…/../..` path. On the main server, sending from webmail failed with
  "authentication failed"; the mail log showed only a connection, with no
  encryption and no login.
- **Both servers updated through the panel's own update page by
  themselves**, in about four and a half and about four minutes. No schema
  step (it stays 81); one panel and one agent ran on each server; no new
  failed service. At the first start the agent said once that it had
  rewritten the local door's file and webmail's configuration; the web
  server was reloaded once on each server, after its check passed; PHP-FPM
  was not restarted. Webmail's configuration kept its mode, owner and secret
  key, and on the main server the lines of webmail's own address. A check
  every second from outside saw a site and webmail answer throughout the
  update on the main server (483 requests, all answered); that check ended
  13 seconds before the reload, and both answered again after it.
- **The local door after the update** (the security correction), on both
  servers, as bob's site account by both ways: the planted copy and
  dot-file, the planted and Roundcube's own plugin PHP files, a plugin's
  `composer.json`, `index.php/x`, both `..` paths, a script file, webmail's
  configuration and its `.celikpanel-pending` copy all answered "not found";
  the sign-in page, a style sheet and CelikPanel's PHP check answered.
  Webmail's own address kept alpha.144's rules.
- **Webmail sends** (main server, with the mail host's certificate): alice
  signed in on webmail's own address and sent three messages, to herself,
  to another mailbox and one with an attachment; after each the page went
  back to the inbox without an error, and all three are in Sent. For each
  the mail log shows the connection to the sending service from 127.0.0.1
  switched to encryption (TLS 1.3) and the login with the mailbox's own
  address, then delivery; nothing went over port 25 from inside. The
  copies that arrived are signed for the sender's own domain and the
  signature verified. A reply sent from outside was listed and opened.
  Sending as an identity with another account's address was refused by the
  sending service ("Sender address rejected: not owned by user …"). Mail
  programs from outside with alice's login: port 587 with STARTTLS and
  port 465 accepted the message; another account's sender address was
  refused (553).
- **A restart changes nothing:** after the lab restarted the agent and the
  panel, both files kept their content and time, no web server reload
  followed, and webmail sent again.
- **Nothing is relaxed for another host:** when the lab pointed webmail at
  `localhost` instead of `127.0.0.1` (the certificate is for the mail host's
  name), sending stopped at the encryption step, so the name check stays on
  for any other host. The lab then put CelikPanel's text back.
- **A local door file the owner changed:**
  - a comment line added to CelikPanel's current text was kept, no line
    appeared, and the door stayed closed;
  - alpha.144's open text with a comment line was kept byte for byte, with
    CelikPanel's text beside it; bob's site account could again read the
    planted files and run the planted PHP (the owner's file serves them);
    **Problems now** showed one line, in Turkish and English, saying that
    the local door still gives Roundcube's files to every account, that the
    server owner acts, and the step. Once the owner moved CelikPanel's text
    over the file and reloaded the web server, the line was gone at the next
    read.
- **A webmail configuration the owner changed** (alpha.144's text with a
  comment line) was kept byte for byte; CelikPanel's text was put beside it,
  readable by root only and with the same secret key. **Problems now**
  showed one line, in Turkish and English: "Webmail cannot send mail", why,
  and the step; a customer and a reseller asking for the problem list were
  refused. Once the owner copied CelikPanel's text into the file, the line
  was gone at the next read without a restart; the text beside it was
  removed at the next agent start, and webmail sent again.
- **Variants:**
  - with only the sending address changed to the encrypted form, without the
    certificate line, the file was kept and the line stayed, naming the text
    beside it; once the owner added the certificate line, the line went by
    itself within seconds, without a restart;
  - CelikPanel's own earlier text in a file the lab made unchangeable
    (immutable flag): the agent said it kept the earlier form and tries
    again at its next start, and the line named the file as one that could
    not be rewritten; with the flag removed and the agent restarted, the
    file was rewritten and the line went;
  - an owner's own sending setting to another host was not named;
  - the file's mode, owner and secret key never changed.
- **The alias wait on the held path:** with the backup server unreachable,
  alice added an alias; its row said "DNS record waiting to be published —
  retried every hour, nothing to do", and the zone's notice said the alias
  records are published by themselves, without asking to make the change
  again (Turkish and English). About six seconds after the backup server
  came back the held change was published, the alias answered from both
  servers within about thirteen seconds, and the row no longer said it was
  waiting. Removed through the row, the alias stopped answering on both.
- **The PostgreSQL window's titles** say the outcome, in Turkish and
  English: a close ended on "… is closed to other accounts", an Undo on
  "… is open to every account again", and a close that failed (the lab had
  dropped a recorded user's role) on "… was not closed to other accounts"
  with "Not done — nothing was changed", the kind of cause and the database
  server's quoted message. None ended on "Closing …".
- **A Roundcube update through its page** (1.6.20 to 1.6.21, main server):
  the update was applied, the configuration kept, and webmail kept working
  and sending on 1.6.21, but the panel kept showing the update as running
  until one panel restart recorded it as done (see "Known limits").
- The usual pages on both servers loaded without a script error.

**Where the lab left the page's path.** As root on the lab servers, the lab
planted a backup copy, a dot-file and a PHP file in a webmail plugin's
folder and put a small script into bob's site to ask the local door;
changed the local door's file and webmail's configuration as an owner
would, restarted the agent, then took CelikPanel's text over them; made the
configuration unchangeable and changeable again; pointed webmail at other
sending hosts and put CelikPanel's text back; restarted the agent and the
panel; sent a reply and used mail programs from a machine outside; added
the identity with another account's address in webmail's settings by a
shortcut of its own; made the backup server unreachable from the main
server and reachable again; as the database administrator put a database
into the state of ones made before alpha.141 and dropped a recorded user's
role; and restarted the panel once to settle the webmail update. Its own certificate
authority was trusted on both servers during the run.

**Not checked:**

- Sending while the sending service shows its own self-signed certificate
  (before a mail host certificate exists): reaching that state would have
  needed removing the mail host's certificate; covered by tests.
- The certificate chain refusal for another host: the lab machines trust
  the lab's own certificate authority, so a host whose certificate chains to
  it was accepted; only the name check was shown.
- Sending from webmail on the Ubuntu 24.04 server: it has no mail service
  in the lab (its configuration was rewritten like the main server's).
- The Webmail card while webmail cannot send: the card showed the stuck
  update's text (see "Known limits") and the mail bundle's card was not
  listed; not proven.
- The alias wait on the slow-backup path (the zone's own publication
  waiting, not held): it could not be produced in the time; covered by
  tests.
- The screen of the "could not be rewritten" line: it was read from the
  problem list, not looked at on the page.
- Webmail's reinstall proof, and the "not known" outcome of the PostgreSQL
  Undo window: time; covered by tests.
- Nothing was tried on Arch Linux.

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
