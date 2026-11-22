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
   If the server cannot set it again (for example the mail server is
   stopped), the reply is turned off and its window says to save it again;
   if even that cannot be done, **Problems now** names the mailbox and the
   step, and the panel tries again every hour.
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
  removed, and the audit log has a row. The same check runs right after
  every change of who holds a name or where an address goes: a
  subscription moved to another owner, a domain deleted or added, a
  forwarder or catch-all added, changed or removed, a mailbox added; and
  every hour.
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
  - Before CelikPanel uses a new signing file, at the panel's start and
    when it sets up a domain's signing, the mail signer checks it. If the
    mail signer does not start with it, the earlier file is put back exactly
    and the signer restarted with it.
  - A restart of the mail signer that an interruption left undone is made
    at the next start.
  - A filter the owner adds to the mail server's list also stays on port
    25. There is no separate step to remove the mail signer: CelikPanel's
    own port-25 setting goes at the next panel start, or the next mail
    filter step (a component installed or removed), after the mail signer
    is no longer set up or no longer in the list.
  - Where the list names its filters through a `$variable` or a `{ }`
    group, CelikPanel cannot read it entry by entry. Its own earlier
    port-25 setting, where it wrote one, stays in force: it leaves the mail
    signer out, but also every filter added to the list since. Where it
    wrote none, port 25 follows the list and arriving mail may still be
    signed. **Problems now** says which of the two, with the step: write the
    filters into the list by name and restart the panel.
  - CelikPanel recognises its own port-25 setting from a record file. If
    that file is lost, it still recognises a setting equal to the list
    without the mail signer, or one made only of its own filters; a setting
    written for an earlier list is then kept as if it were the owner's.
  - The mail signer is recognised in the owner's list however Postfix lets
    its address be written (for example `inet:[127.0.0.1]:8891`,
    `inet:localhost:8891` or `inet::8891`).
  - A signing file or port-25 filter setting the owner changed by hand
    stays as it is (CelikPanel's text is beside it as
    `opendkim.conf.celikpanel-pending`). **Problems now** names the file or
    setting and the owner's concrete step. Once the owner's own signing file
    has the line `SenderMacro {auth_authen}` (and no other `SenderMacro`, and
    no `Include`), it counts as in place: the line in **Problems now** goes at
    the next panel start, the panel's text beside it is removed, and the file
    stays his.
  - **Problems now** also does this when the setting that decides which
    accounts may hand mail over without a login is not the panel's
    (`authorized_submit_users = root, vmail`), because sites' programs
    could then send mail signed for any domain here.
  - While the mail signer restarts (a few seconds), mail passes unsigned
    where both filter lists are CelikPanel's: it sets
    `milter_default_action = accept` then. Where either list is the
    owner's, it does not set it, so the owner's `milter_default_action`
    applies, or, if he set none, Postfix's default (`tempfail`): a sending
    mail server tries again later by itself, while a mail program on port
    587 shows its user an error and the person sends again.

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
  administrator decides, and the account is named; so does an account the
  close itself finds connected at the last moment. Connections are seen at
  the hourly checks: an account that connects only between two checks is
  not seen. The **PostgreSQL** page shows since when each database is
  watched and when it closes by itself if no other account is seen
  connected at an hourly check meanwhile; the administrator can close it at
  any time.
- Every other such database the panel can read is left exactly as it is
  and listed in **Problems now** and on the **PostgreSQL** page with what
  was found, what every account can do there now (connect, or only make
  temporary tables), what closing changes, and one button to close it after
  a confirmation that names the accounts connected at the last reading. A
  database the panel cannot read is shown on the page as not readable, with
  the reason; after more than three hourly checks in a row it is also
  listed in **Problems now** and the alert mail as not readable, with the
  reason and the step. A reading that runs out of time keeps what it read
  of the other databases.
- A close waits at most 30 seconds for another session that holds the
  database (and no statement of it runs longer than 60 seconds), whatever
  the database's owner set for it; the Agent ends a close or undo still
  running after 3 minutes, and its answer is then "not known", with the
  time it was ended. A close or undo whose answer is lost or not known is
  never taken as "nothing changed": the panel keeps its record, sends
  nothing again, and reads the database only after the Agent confirms that
  no part of that close or undo is still running on the server, so the
  record of a close is never deleted while the database may be closed, and
  its **Undo** is there whenever the database is closed. A close that fails
  is shown with its reason, its time and the step to take; it is not
  retried every hour, only when something about the database changes or the
  administrator presses Close. Two closes of the same database never run
  at once; a close of one database holds up neither another database's
  close nor **Read again**.
- The administrator's sessions inside a customer's database keep
  PostgreSQL's own defaults whatever the database's owner set for it
  (timeouts, read-only mode, the acting role, ending the session on an
  error); this also holds for alpha.141's restore and copy.
- Every close shows when and by whom it was made, with **Undo** (and the
  plain statements to undo it without the panel); undo gives back exactly
  what the close changed. A database the administrator opened again stays
  on the page as the administrator's choice, leaves **Problems now** and the
  alert mail, and is never closed again by itself.
- The page shows the last reading with its time; **Read again** reads every
  database of a database server in one connection.
- Right after the panel closed a database by itself, the page shows that
  close (by the panel, when, **Undo**) without **Read again**. A close that
  failed says only that nothing was changed, why in the page's language,
  and the database server's own message in quotes; "not known" is said only
  when it is not known. The default rights written out for the database's
  owner (as an undo leaves them) count as the default everywhere, so a
  database opened again is not called "changed outside CelikPanel".
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
- An out-of-office reply is set again right after the change that took an
  address from its mailbox (a subscription moved, a domain deleted, a
  forwarder removed): for the few seconds between that change and the
  reply being set again, the earlier reply can still answer. When the
  server cannot set it again or turn it off (the Agent does not answer or
  is another version), it keeps answering until the next hourly try that
  succeeds; **Problems now** names the mailbox and the step meanwhile.
- Where the mail server's filter list names its filters through a
  `$variable` or a `{ }` group, a filter added to it later does not see
  mail arriving on port 25 while CelikPanel's earlier port-25 setting
  stays in force (see "For the owner" above).
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
- Sessions are seen at the hourly checks: an account that connects to a
  watched database only between two checks is not seen, and the database
  can be closed by itself after its day. The idle-session and transaction
  timeouts a database's owner may set (PostgreSQL 14 and 17) are reset by
  the panel's closing sessions right after they connect, not as connection
  options (an older server would refuse an option it does not know).
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
- The lab found six more limits, each with what to do until alpha.144: see
  "Found in the lab, planned for alpha.144" below.

## Checked in the lab

Lab check finished: two runs on a pair of test servers, a Debian 13 main
DNS server with PowerDNS and an Ubuntu 24.04 backup DNS server with BIND.
Each run started from the alpha.142 state and followed the owner's path:
both servers were updated through the panel's own update page. The first
run (21 November 2026, UTC 22:30, to about 01:40 on 22 November with the
clean-up) checked the release as it stood after the first fix round
(`c012e3eac`, under the lab-only label v0.1.0-alpha.150). It found four
display and wording defects and one state that did not clear; the second
fix round corrected them. The second run (22 November 2026, UTC 03:38 to
about 05:24) checked the final code (`1d3d8801a`, lab-only label
v0.1.0-alpha.151) and found all five corrected. Two customers of one
reseller, in separate accounts, played the two sides: alice, and bob, who
tried to send mail as her and to reach her databases. Everything was done
through the page's own buttons except where it says the lab did it. No hard
failure in either run. What follows was seen on the final code, except
where it says "(first run)": that was seen only in the first run, on the
code before the second fix round.

- **Before the update the ways were open** (alpha.142's code, first run):
  bob's mailbox login sending with alice's address in "From" was signed for
  her domain (`d=kahvedukkani.dr.test`) and the signature verified; mail
  his site handed to port 25 on the server's own loopback address without
  a login was signed for her domain too; his out-of-office reply was
  accepted with her address and answered from it with her signature. In the
  second run the old code again accepted that reply.
- **Both servers updated through the panel's own update page by
  themselves**, in about three minutes each (about two minutes in the
  first run). Schema 80 → 81 was applied with the six name rules in place;
  one panel and one agent ran on each server; no new failed service; web,
  mail, DNS (the same answers from both servers) and the databases
  answered.
- **No mail signed with another customer's domain.** After the update:
  - bob's login with alice's address in "From" was signed for his own
    domain only (`d=pastane.dr.test`);
  - mail his site handed to port 25 without a login was not signed, on the
    loopback address and on the public address;
  - his reply set to answer from her address lost that address at the
    first start (a journal line and an audit row); its window said so once,
    and saving her address again was refused with a sentence that does not
    say whose it is (Turkish and English);
  - legitimate mail of both customers was signed for their own domains and
    verified; a forged message forwarded through her forwarder got no added
    signature (first run).
- **Replies follow name changes.** bob's reply answering from his own
  forwarder was accepted and answered signed for his domain. When he
  removed that forwarder, and when alice deleted a second domain that her
  reply named, the reply was rewritten inside the same request, and its
  window said which address was removed. With the mail store service
  stopped, removing the forwarder turned the reply off, and its window said
  so and to save it again.
- **The owner's own signing file.** A signing file the owner had changed
  was kept byte for byte, with the panel's text beside it and one line in
  **Problems now** with his step. Once the owner added the login setting
  (`SenderMacro {auth_authen}`) to his own file, the line went by itself at
  the next panel start and the panel's file beside it was removed; his file
  stayed as he had saved it, also when a new domain got its signing key
  afterwards. The lab also saw alpha.142's code replace an owner-edited
  signing file when a new domain got its key; this release leaves the
  owner's file as it is on that path too. An open "who may hand mail over"
  setting (`authorized_submit_users`) and a signing port held by another
  program were each said plainly in **Problems now**, and each line went
  once corrected. Port 25 followed a filter the owner added to the mail
  server's list (first run).
- **Older PostgreSQL databases.**
  - The PostgreSQL page showed each watched database with the date its
    watch began and when it would close by itself (first run).
  - Nothing was closed during the day of watching. During that day another
    customer's database account could still connect to them.
  - A role the panel does not record, seen connected, ended the watch, and
    the page named it (first run).
  - With the watch record set back a day by the lab (the one stated write),
    the next hourly check closed the database: the other customer's account
    was refused, the database's own users got in, and the page showed the
    close (by the panel, when, **Undo**) without **Read again**. **Undo**
    gave the earlier rights back, the page did not call them changed
    outside CelikPanel, and the next check did not close the database
    again.
  - A close while the owner held the database's row was answered after
    about half a minute with "nothing was changed" and the database
    server's message; no client was left behind and it was not sent again.
  - A close that failed said why in the page's language, with the database
    server's message in quotes.
  - The owner's own database settings (read-only mode, his own role, no
    lock wait) did not get in the way of reading, watching, closing or
    undoing.
  - A deleted database's record went with it.
  - A database made after the update was closed to other accounts from the
    start (first run).
- **The corrections.** A subdomain inside alice's own domain under the
  server's zone was created and answered from both servers (first run).
  bob was still refused for every name he tried (the server's zone, a new
  name under it, the panel's and the mail server's names, a name under
  alice's alias) with the same sentence, which names nobody. The
  administrator refused a name in another account was told the nearest
  holder (first run). A Node.js site whose application had failed was
  deleted in one press; its failed service went with it.
- The usual pages on both servers loaded without an error.

**Where the lab left the page's path.** As root on the lab servers, the lab
changed the mail server's and the signing service's files as an owner would
(a comment line and then the login setting in the signing file, the "who
may hand mail over" setting, an extra port-25 filter), placed a pending
restart marker by hand (first run), stopped and started the mail store
service and the signing service, ran a dummy listener on the signing port,
put a small mail script into bob's site to hand mail to port 25, sent test
mail from outside, and registered the certificate tool with the lab's own
certificate authority. As the database administrator it put databases into
the state of ones made before alpha.141 (PostgreSQL's default rights),
added a reporting role with a column right, and dropped a recorded user's
role (in the first run it made it again later); as a database's owner it
set his own database settings and held a transaction open. It restarted
the panel to bring an hourly check forward. Once in each run, with the
panel stopped, it wrote one row of the lab panel's database, which set a
watch record back by a day. The administrator's placing of a name into
another account was sent to the panel's API directly (first run).

**Not checked:**

- Webmail and the database web tools: the lab pair has no webmail address
  and the tools are not installed (carried from alpha.142, still open).
- Moving a subscription to another account while a reply names its domain:
  the page offers this only as a request and an approval; covered by tests.
- A mailbox login whose domain has no signing key: every domain gets one.
- The signature of a panel notice: no way was found to send one in the
  lab; covered by tests.
- Backup and restore of a database whose owner set his own database
  settings: time.
- Ending the database client right after a close's commit: not tried;
  covered by the engine tests.
- The Agent stopped at the first start while a reply names another
  customer's address: this could not be arranged without a second write to
  the panel's database.
- The journal line for a kept file mode: the one-time correction had
  already run on these servers.
- A healthy Node.js site created and deleted: time.
- PostgreSQL versions other than the lab server's 17, and a server whose
  templates are SQL_ASCII.
- Moving a database to another PostgreSQL server version: not part of this
  release.

### Found in the lab, planned for alpha.144

None of these is a security problem or data loss.

1. An alias inside a customer's own domain is created, but on the lab pair
   it got no DNS record: it did not answer from either server. Whether
   releases before alpha.143 did the same was not established.
2. When the administrator leaves the PostgreSQL page while a close is
   running, the panel records the answer as lost although the close
   finished on the server: the page says "sent, not yet confirmed" and the
   close is not sent again. The database is never shown closed without its
   **Undo**.
3. The window of a failed close keeps the step "working on the server" with
   a red mark, and its reason in the page's language is general ("the
   database server did not do it"); the real reason is the database
   server's quoted message.
4. One refusal (an alias whose certificate step fails) appears in English
   on the Turkish page.
5. The English **Problems now** line says "1 PostgreSQL databases".
6. Older than this release, seen on alpha.142's code in the first run and
   not tried again on the final code: leaving the page while a mailbox is
   being created can leave the mailbox on the server without a row in the
   panel; creating it again then answers with an internal error. The way
   out today: the administrator removes the mailbox on the server, or
   creates it under another name.

### What to expect after updating

- The update takes about three minutes per server and says by itself when
  it has finished.
- The layout is schema 81: going back to alpha.142 means restoring the
  snapshot or backup taken before the update.
- Mail sent with a mailbox login is signed only for its own domain; mail
  handed to port 25 without a login (for example by a website) is no
  longer signed.
- An out-of-office reply that named an address not its own loses that
  address at the first start, and its window says once which address was
  removed; where it cannot be set again, it is turned off and its window
  says to save it again.
- A PostgreSQL database made before alpha.141 that nobody else is seen
  using is closed to other accounts by itself at the first hourly check at
  least 24 hours after its watch began, once at least 24 hourly checks have
  read it; until then the PostgreSQL page shows when it closes, and other
  accounts can still connect to it. The others are listed for the
  administrator in **Problems now** and on the PostgreSQL page, each with
  one button to close it.
- A signing file the owner edited by hand is kept, with one line in
  **Problems now** that goes by itself once the login setting is in it.
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
  signing state in `web/tests/mail-signing-a143-fix.test.mjs`. The second
  review's mail cases: `TestMailAutoresponderTransferRewritesTheReplyInTheSameRequest`,
  `TestMailAutoresponderForwarderRemovalRewritesTheReplyInTheSameRequest`,
  `TestMailAutoresponderRewriteThatCannotBeMadeIsNotSilent`,
  `TestMailAutoresponderReconcileDoesNotOverwriteASaveMadeMeanwhile`,
  `TestMailSigningVariableListStatesAndForeignReplyAreWorded`,
  `TestMailSigningVariableListKeepsCelikPanelsValueAndSaysSo`,
  `TestMailSigningPort25ValueIsRecognisedWithoutTheRecord`,
  `TestMailSigningOpenDKIMEndpointSpellings` and
  `TestMailSigningConfigureDKIMSigningChecksAndPutsBack`; the matrix case
  `postfix.mail-signing-variable-list` (added, not run yet). Also new and
  additive: the Problems now kind `mail_autoresponder_foreign`, the signing
  states `port25_variable` and `port25_variable_own`, and panel setting rows
  `mail_autoresponder_foreign_v1:<account id>`.
- PostgreSQL, the second review's cases: on a private PostgreSQL 17.12
  server `TestS1bRound2ACloseThatWaitsIsEndedAndLeavesNothingHalfDone` (a
  second session holds the database's row: the Agent's deadline ends the
  close as "not known" and the running check sees it until it is gone; the
  close's own lock_timeout ends it whatever the owner set; nothing changed
  either time), `TestS1bRound2TheOwnersDatabaseSettingsDoNotReachTheAdministratorSession`
  and `TestS1bRound2TheClosesSessionGuardNamesTheRoleItCaught`; without an
  engine `TestPGPublicCloseAndUndoBoundThemselvesBeforeTheyCanWait`,
  `TestPGPublicAClientEndedAtItsDeadlineIsUnknown`,
  `TestPGPublicTheScanAsksWhichDatabasesExistBeforeEnteringAny`,
  `TestPGPublicAScanWhoseTimeRanOutKeepsWhatItRead`,
  `TestPGPublicAClientIsFoundByItsNameAmongTheProcesses` and
  `TestAdministratorSessionPinsWhatADatabaseOwnerCouldSet`; the panel's
  `TestPGPublicACloseThatMayStillRunStaysSendingAndIsNeverSentAgain`,
  `TestPGPublicAnUndoWhoseAnswerIsLostKeepsTheCloseUntilItIsConfirmed`,
  `TestPGPublicAWatchHoldingASentCloseFoundClosedLaterIsThatClosesRecord`,
  `TestPGPublicOneDatabasesCloseDoesNotHoldUpOthers`,
  `TestPGPublicARoundSkipsARecordChangedAfterItsScanBegan`,
  `TestPGPublicASessionTheClosesGuardCaughtIsRecordedAndEndsTheWatch`,
  `TestPGPublicADatabaseUnreadForMoreThanThreeRoundsIsListed` and
  `TestPGPublicTheRecordOfARemovedDatabaseIsRemoved`; the alpha.143 lab's
  `TestMailSigningOwnersFileWithTheLoginSettingIsInPlace`,
  `TestPGPublicThePageShowsARoundsOwnCloseRightAway`,
  `TestPGPublicTheDefaultRightsWrittenOutAreTheDefault` and the verified
  failure cases in `TestPGPublicAnUnknownAnswerKeepsTheRecordAndReadsTheEngine`
  and `TestS1bFixARecordedUserWithoutItsRoleIsSeenAndItsGrantIsNotNothingChanged`. Also new and additive:
  the Problems now kind `postgresql_databases_unreadable`, the build-gated
  agent call `PostgreSQLPublicAccessRunningV1` and the close's answer
  `sessions`.
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
| `internal/transport` | 62 | 63 |
| `internal/db` | 182 | 184 |
| `internal/services` | 371 | 371 |
| `cmd/agent` | 5854 | 5920 |
| `cmd/panel` | 4726 | 4763 |

(Passed tests and subtests. "After" was measured on 22 November 2026 on
the second fix round's head (`14b7caf11`) with the private PostgreSQL
17.12 and MariaDB 11.8 servers attached, so the database tests ran inside
the package suites; alpha.143's candidate measured the same way that day
gives 63, 184, 371, 5906 and 4749. In this test machine 94 tests and
subtests of `cmd/agent` (19 tests) fail identically on both, because of the
machine itself: its DNS engine state does not authorize PowerDNS, and its
temporary folders are not owned by the root group. On the head run three
more failed that are not the change's: the two recovery-runtime packages,
which then failed on the candidate as well once the machine's /run state
had changed; one panel restore test that ran out of its time under the
full run and passes alone. One agent contract test named the new
`PostgreSQLPublicAccessRunningV1` as missing from its build-gate list; it
was added after the run and passes. The two database tests that need a
server whose templates are SQL_ASCII are skipped.)

No new failure. Web: `npm test` 3090 → 3105 passed (3102 before the second fix round), 0 failed; the type
check is clean and the build meets every size budget (first-load path
109.36 KiB of 110.00 KiB compressed; alpha.142: 109.37).
