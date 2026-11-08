# v0.1.0-alpha.130

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.130.tr.md)

Mail gets the work the roadmap promised: mail programs that set themselves
up, sending through another server, subaddressing, a server-wide sender
list, and a recovery address with a reset link for a forgotten mailbox
password. Remote access to databases closes the gaps alpha.129 named, and
everything the owner found on Boston and Frankfurt on 8 November is fixed:
a false "Postfix is not running", a valid panel certificate called failed,
English-format dates on Turkish screens, ticks in the Offered tab that did
nothing, password sign-in to SSH that the owner could not turn off from the
panel, and a removed PostgreSQL version that left its settings behind.
Removals now take CelikPanel's own leftovers with them. **Only the owner
updates installed panels, from the panel's own update screen.**

## Mail

- **Mail programs set themselves up.** Thunderbird (autoconfig) and Outlook
  (Autodiscover) find the server, ports and encryption from the address
  alone, for every top-level domain whose mail is on this server. The
  answers are fixed files of the site's own nginx configuration: never a
  password, never the address that was asked. HTTPS is used only when the
  site's certificate covers the `autoconfig.` and `autodiscover.` names, and
  a certificate adds them only where public DNS already points them here.
  New zones get the records; an existing zone gets the missing ones with one
  press on the mailbox screen, never replacing a record that is there. The
  mailbox screen says what is still missing and who does it
  (docs/MAIL-AUTOCONFIG.md).
- **Sending through another server (smarthost), server-wide.** The
  administrator enters the relay's address, port, user and password under
  Postfix; the password is stored sealed and never shown again. Postfix
  signs in over a mandatory encrypted connection. A "test sign-in" connects,
  signs in and quits without sending anything. An owner's own relay setting
  is detected and never overwritten; turning it off removes exactly what was
  written (docs/MAIL-RELAY.md).
- **Subaddressing (`user+tag@domain`), server-wide.** Debian's and Ubuntu's
  own `main.cf` already turn it on; the screen then says so and keeps that
  line. Dovecot's setting is read, never written.
- **Sender allow and block list, server-wide.** A blocked address or domain
  is refused with 550; "allow" is only an exception to a block, so the list
  can never open a relay or skip spam checks. Domains of this server cannot
  be listed (docs/MAIL-SERVER-SETTINGS-DESIGN.md).
- **Recovery address and reset link for a mailbox.** Whoever may change a
  mailbox's password can give it a recovery address, which works only after
  the link mailed to it was opened. The mailbox user asks for a reset link
  on the panel's "forgot password" page; the link goes only to a confirmed
  address, lives 30 minutes, works once, and the reset applies the mailbox
  password rule, ends open IMAP/POP3 sessions and mails the mailbox. The
  answer to a request is the same for every address. A transfer of the
  domain, or a domain deleted and added again, voids the address. A written
  security review came first (docs/MAILBOX-RECOVERY-REVIEW.md).
- **A manual sending stop outlives a password change.** Until now any
  password change reopened a stopped mailbox, also one the administrator or
  reseller had stopped by hand. Only a security stop (spike, refusals) ends
  with a password change now; the screen says who reopens a manual stop.

## Connecting to a database from outside

- **MariaDB host names no longer escape Fail2ban.** The rule bans addresses
  only. MariaDB rules wait while MariaDB looks client names up; with the
  administrator's agreement CelikPanel turns the lookup off in its own option
  file (never over an owner's setting, never while an account names a host),
  and removes it with the last rule.
- **Fail2ban for PostgreSQL** as an extra layer beside `auth_delay`, only
  over the package-default log prefix; nothing is left when the last rule or
  Fail2ban goes.
- **Timed access:** a new address is open for 1 day by default, or 7 days,
  or without an end; an ended address is removed within a minute like a
  manual removal, and extending is one click.
- **Closing while the server is still starting:** a removed address is taken
  out of the firewall filter at once; the rest follows when the server is
  ready, and the screen says so.
- **After `nft flush ruleset`** the Agent puts the panel firewall back from
  its saved policy within about 15 seconds, the database filter first,
  never against a disabled boot restore or masked nftables.
- **The administrator's section on the Databases page** with the server
  switch and, per engine, what is still missing with its step. The switch
  and the Offered tab write the same stored offer.
- A customer's view keeps the problem but not the server's details
  (docs/DATABASE-REMOTE-ACCESS.md).

## Plans and rights

- **Every offer is written where it is read.** Ticking "Remote access to
  databases" (or nginx directives) in Users → Offered answered "saved" and
  changed nothing; it now offers it, with the same audit and effect as its
  own switch. Uploaded programs, Python lines and AI apps are shown
  read-only with a link to Catalog management; a version withdrawn for
  security says so. Entries an earlier release wrote in the wrong place are
  moved on the first read as withdrawn offers; nothing is opened.
- Every answer carries the item's state as the gates read it, and a screen
  says when it differs from what was asked.
- Permissions stand under the same topics on every rights screen, sorted in
  the reader's language.
- "Customers may change it" is offered only for settings a customer really
  chooses (the PHP values); one Save for the section, errors beside their
  row.
- The application catalog has Turkish descriptions, the download check and
  the risk note in Turkish (docs/PLANS-AND-RIGHTS.md §15).

## PostgreSQL and leftovers

The owner's rule of 8 November: no leftovers, ever; user data is never
deleted silently.

- **Removing a PostgreSQL version leaves nothing behind** (Debian and
  Ubuntu). The packages are purged, including the version's client package,
  and the cluster's configuration, data folder and logs go after the purge.
  A cluster that holds anything beyond what a fresh installation makes is
  archived first under `/var/backups/celikpanel/postgresql/`. The removal is
  still refused while a cluster holds a database. Until now the old
  version's settings stayed, so a newly installed version got port 5433
  (Boston, 3 November).
- **Leftover clusters from earlier releases:** one that holds only what a
  fresh installation makes is removed by itself once after the panel
  starts, with an audit entry and a root-only archive kept for 30 days
  (`…-auto.tar.gz`). Any other is shown as "Not used – old version N" with
  the reason it was kept and **Archive and remove** (the panel's own
  confirmation and the server's steps, the archive named at the end). A
  cluster the panel records databases on is never removed.
- **Settings edit only clusters that run.** With several running clusters
  the PostgreSQL page lets the administrator choose whose `postgresql.conf`
  and `pg_hba.conf` the editors open; a removed version's files are no
  longer listed.
- **A refused configuration save says why at the top.** The banner names
  the rule (its number and line) or the setting, gives the reason in plain
  words with the fix, keeps PostgreSQL's own words under it, says the file
  on the server did not change, and "Show rule N" scrolls to the marked row.
  The same for MariaDB settings.
- **Mail and Node.js removals take CelikPanel's own files with them:**
  Dovecot's quota-warning timer and drop-ins, Postfix's CelikPanel maps
  (also the relay password file) with their indexes, postfwd's drop-in,
  rule file, lists and saved counters, and the empty Node.js folder after
  the last version. A file is removed only while it is still CelikPanel's;
  a file the owner changed is kept and named (docs/LEFTOVERS-AUDIT.md).

## Server and security

- **SSH password sign-in** is shown as sshd really decides it (the file and
  line that win, e.g. cloud-init's `50-cloud-init.conf`) and the
  administrator can turn it off under Security. CelikPanel writes only its
  own `00-celikpanel.conf`, checks with `sshd -t` and `sshd -T`, reloads (an
  open session is never cut), and refuses while no administrator account has
  a key. The confirmation says to keep the session open and test a key login
  first; the command to undo it without the panel is shown.
- **Panel TLS audit:** a valid Let's Encrypt certificate was reported as
  failed, because the check asked without a name and got the start-up
  certificate. It now asks for the certificate's name, lists the names it
  covers, and says whether they cover the address the panel was opened with.
- **Security updates card:** the update sources in plain words ("security
  updates from Debian") instead of raw patterns, and an update waiting for a
  restart is said with a link.
- **Monitoring:** no false "Postfix is not running" on Debian and Ubuntu
  (the live reading now reads the unit that proves the daemon runs; the same
  for oneshot units such as WireGuard). A service only an old scan describes
  is "not checked", never a current problem.
- **Dates and times follow the interface language everywhere** (one
  formatter; a test refuses a date formatted without the language).

## Icons

- Every component on the Components page has its own icon; no two share one.
- PostgreSQL and MariaDB are drawn with our own elephant and sea lion in the
  engine colours (not the projects' logos), on the Databases page server
  cards and the Components page; no more emoji.

## Fixes found in the lab

- **An update's automatic rollback no longer waits for remote database
  access.** In a forced failing 129 → 130 update the first rollback attempt
  was refused, because the new panel's start-up apply of remote database
  access held a service lock (about 80 seconds without the panel). At start
  the panel now only reads what the Agent last applied and sends nothing
  when it matches; for 3 minutes after start, and while the server is busy,
  no full apply runs. An ended or withdrawn address is still closed at once
  by removing it from CelikPanel's own firewall filter, which needs no lock.
- **Remote database access can be closed while the panel is still
  starting.** During a long start-up every change was refused with "the
  panel is starting", so nobody could remove an address or turn remote
  access off. Removing an address, giving it an earlier end and the
  administrator's switch to off now go through at once and take the
  address out of the firewall filter; opening anything still waits until
  the start finishes.
- **The automatic leftover cleanup also purges the removed PostgreSQL
  version's packages.** On an Ubuntu server like Boston the old cluster's
  files went, but `dpkg` still listed `rc postgresql-16` and an installed
  `postgresql-client-16`. The same job now purges them through the usual
  safe removal path (a simulation that would remove anything else refuses
  it; a client another package still needs stays and is named), with an
  audit entry. The job first runs 10 minutes after the panel starts and
  only on a quiet server, so it never delays an update or a rollback.
- **One PostgreSQL version can be removed again.** Removing a single
  version was refused ("package does not name a removable version"); while
  two versions are installed, each cluster row on the PostgreSQL page now
  offers "Remove PostgreSQL N".
- **Removing PostgreSQL entirely leaves nothing:** packages left in `rc`
  state, every version's client and documentation, `postgresql-common` and
  `postgresql-client-common`, empty PostgreSQL folders, `libpq5` when
  nothing else needs it, and CelikPanel's PGDG source, key and pin when no
  PGDG package remains. A leftover cluster that may hold data stops the
  removal and is named.
- **A purge keeps a copy of what may be the owner's, and leaves no folder
  behind** (Debian and Ubuntu). A systemd drop-in the owner wrote for the
  removed service is put back after the purge. The component's own folders
  (`/etc/postfix`, `/etc/dovecot`, `/etc/postgresql-common`) are emptied
  and removed: package files and the files the packages' scripts and
  CelikPanel wrote go without a copy; any other file, and every changed
  package configuration file, is copied first to
  `/var/backups/celikpanel/kept/<component>-<time>/` (root only), and the
  answer says how many files were copied and where. A failed purge empties
  nothing. Removing Dovecot also takes CelikPanel's `/etc/dovecot/users`,
  and mail set-up now finds Dovecot by its program, so a later Postfix
  installation no longer fails on a leftover folder. Components are read
  again right after a leftover cluster is removed.
- **SSH password sign-in can be turned off on a joined BIND server.**
  CelikPanel's own `Match User celikpeer` block (which only turns password
  sign-in off for the DNS peer account) blocked the change; only a Match
  block that could turn password sign-in on blocks now. CelikPanel's
  drop-in is now owned by root alone, so the status reads it and "turn
  back on" works (before, it could only be undone by hand).
- **"Set up now" for mail programs works.** The mailbox screen's button
  answered "404 page not found" for every domain, so missing autoconfig and
  Autodiscover records, and the site file of an existing domain, could not
  be added from the panel. The route now reaches its handler, behind the
  same mail permission as the other mail changes.
- **MariaDB's host name lookup stays off after a restart.** When the last
  remote rule had been removed and a new one added before MariaDB
  restarted, CelikPanel's option file was written without the agreed
  `skip-name-resolve` line, so after a reboot every MariaDB rule waited
  again. With the administrator's agreement the line is now always written;
  an owner's own setting is still never written over.
- **No false "Postfix is not running" on Debian 13 either.** Debian 13's
  Postfix runs in `postfix.service` itself, while Ubuntu's runs in the
  `postfix@-` instance behind a wrapper; the first fix read only the
  instance and raised the alarm on Debian 13. Monitoring now reads both and
  takes the wrapper's reading when the wrapper itself runs.
- **A change sent while the server is still booting says so.** While
  systemd still reports the server as starting, the Agent refuses changes;
  creating a domain (and other changes) then answered a bare "500 internal
  error". The answer is now "the server is still starting": nothing was
  changed, nobody needs to act, try again in 15 seconds, and the commands
  to look at when the boot lasts longer than 10 minutes.

## Lab results

Lab (QEMU, no real servers or CAs): the published 129 was installed with
get.sh as a Debian 13 first server (PowerDNS, mail, MariaDB and PostgreSQL)
and an Ubuntu 24.04 joined server carrying Boston's PostgreSQL history.
Updates were pressed once from the panel. 129→130 took 70–84 s; schema 77;
no failed services; sites answered. A forced failing update rolled back on
the first attempt, with the panel down 29–31 s, with and without
remote-access rules. Tested and passing: remote database access (host-name
lookup switch, Fail2ban bans for MariaDB and PostgreSQL, timed access
closing within a minute, recovery after `nft flush`, closing while the
server boots), the Offered tab and Databases switch, PostgreSQL leftovers
(removed and purged ten minutes after start; one-version and whole removal
leave nothing), and component removals (only the owner's drop-ins and files
stay, copied aside). Mail programs set themselves up, including DNS on both
servers and certificates. Sending went through a SASL relay on 587 and 465.
Subaddressing, the sender list, mailbox recovery, Turkish dates, the
false-Postfix fix on Debian 13 and the SSH switch all passed. Not tried:
Arch, IPv6 rules, Roundcube session end, the reseller-gap warning.

Evidence: [`deploy/e2e/dns-pair-acceptance/evidence/a130/`](https://github.com/alicelik77/celikpanel/tree/e46b2c9ff/deploy/e2e/dns-pair-acceptance/evidence/a130) on the
lab branch `lab/alpha114-acceptance` (commit `e46b2c9ff`).

## Known limits

- **Not built in this release (design notes only):**
  - wildcard certificates (`*.domain`) through DNS: renewal must work
    without the panel, and writing the challenge into a panel zone would
    break the zone receipts and the secondaries' transfers; an owner
    decision is needed first. The certificate request route (alpha.128)
    already gives a wildcard from any CA (docs/WILDCARD-CERTIFICATE-DESIGN.md);
  - DKIM key rotation, Dovecot connection limits and a customer's own
    sender list per domain (docs/MAIL-SERVER-SETTINGS-DESIGN.md);
  - a relay for one domain only (docs/MAIL-RELAY.md).
- **Mail programs:** subdomains are not set up; a site picks up the answers
  at its next save (or with "Set up now"); a site file the owner kept is
  never changed.
- **Relay:** Postfix needs a SASL login module (`libsasl2-modules`,
  Arch `libsasl`); when it is missing the save is refused with the command,
  nothing is installed.
- **Mailbox recovery:** mailbox users cannot set their own recovery address
  (they have no panel sign-in), and Roundcube's login page has no "forgot
  password" link; no screen sets a manual sending stop yet (the rule holds
  for it).
- **Remote database access:**
  - the engine starts listening on the network, and MariaDB's name lookup
    goes off, only after the administrator restarts it;
  - turning MariaDB's name lookup off needs the administrator's agreement
    (programs connecting to 127.0.0.1 as `user@localhost` stop matching);
    until then MariaDB rules wait;
  - the PostgreSQL Fail2ban layer works only over a package-default
    `log_line_prefix`, without `logging_collector`, with English messages;
  - expiry is carried out by the panel: while it is stopped an ended address
    stays allowed (the engines still check the password);
  - while the server has not finished starting, a removal closes the address
    at the filter only;
  - while the panel is still starting, withdrawing access from the plan or
    rights screens and the removal of ended addresses wait until the start
    finishes (removing an address, an earlier end and the administrator's
    switch to off work at once);
  - after `nft flush ruleset`, a firewall turned on only live (no "Save for
    reboot") is not put back;
  - withdrawing the permission closes access at the next apply (up to about
    35 seconds); MariaDB accepts single IPv6 addresses only; table, column
    or role grants added by hand to a remote MariaDB account are not
    removed; MariaDB must log to the journal.
- **Leftovers still to decide (docs/LEFTOVERS-AUDIT.md):** removing
  MariaDB keeps `/var/lib/mysql` and `/etc/mysql` (packages left in `rc`
  state); removing Roundcube deletes its database (users' contacts and
  settings) without an archive; WireGuard keys and peers stay in
  `/etc/wireguard/wg0.conf`; mail data, mailbox password hashes and DKIM
  keys stay; on Arch, phpMyAdmin's `celikpanel-phpmyadmin.ini` stays because
  sites may need `mysqli`, and `/var/lib/postgres/data` stays; Postfix's
  generic maps without a marker, the shared mail certificate copies and the
  old DNS engine's files stay. Each needs an owner decision first.
- **Owner files and purges:** copying the owner's files aside and putting
  drop-ins back works with apt only (Debian and Ubuntu), not with Arch's
  pacman. Outside the systemd drop-in folders nothing is put back in place:
  a file the owner added to `/etc/postfix`, `/etc/dovecot` or
  `/etc/postgresql-common` survives only as the copy under
  `/var/backups/celikpanel/kept/`.
- **Leftover PostgreSQL clusters:** a cluster counts as "fresh" only from its
  files; functions, views or comments made in `postgres` or `template1`
  cannot be seen that way, which is why the automatic removal keeps a
  30-day archive. Anything the reading cannot judge is never removed
  automatically. Arch keeps its single cluster as before.
- **Components:** the Components scan still judges Postfix on Debian and
  Ubuntu by its wrapper unit; Monitoring and "Problems now" read the real
  one.
- **Rights:** in the rare case where remote database access was on at its
  switch but hidden in the Offered tab, it now reads off (as customers
  already had it); the administrator turns it on again in one click.
- **Seen in the lab, not fixed in this release:**
  - on Ubuntu a restored drop-in folder's mode and group changed (0755
    root:root became 0750 root:celikpanel); systemd still reads it;
  - the removal answer names the `kept/` folder but not each copied file;
  - no screen creates a manual sending stop yet, and the 550 text for one
    would still advise a password change;
  - while the server is still starting, the customer's rule list answers
    503, so only an address already on the screen can be removed.
- The lab ran in virtual machines only (see Lab results); Arch, IPv6 rules,
  the end of a Roundcube session and the reseller-gap warning were not
  tried.
- Earlier limits still apply (see alpha.129).

## Evidence

Migrations (all additive):

- **075 `database_remote_rule_expiry`:** `expires_at` on
  `database_remote_rules` (every existing rule keeps no end).
- **076 `mailbox_recovery`:** the `mailbox_recovery_addresses` and
  `mailbox_recovery_links` tables.
- **077 `mail_relay`:** the `mail_relay_settings`, `mail_server_options` and
  `mail_sender_rules` tables.

As with every migration, alpha.129 refuses a database that has these
migrations, and going back means restoring the snapshot taken before the
update. Release sequence 129 → 130.
