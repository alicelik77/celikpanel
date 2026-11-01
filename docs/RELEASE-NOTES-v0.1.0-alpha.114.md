# v0.1.0-alpha.114

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.114.tr.md)

This release follows v0.1.0-alpha.113. It fixes what the first real servers
showed, and it adds users and resellers, a few small features that Plesk
users expect, operating-system updates in the owner's hands, and a new
default panel port for fresh installs. **It is a candidate, written on
2026-11-01; it is not published and nothing from it is on an installed
server. Only the owner updates installed panels, from the panel's own update
screen.** Branch `release/alpha114-prep`, from the integration candidate
`release/alpha114-integration` at `1bd1793fe`; previous release commit
`d3f3a323da46ce8f8dca5ad4385666c5428c353e`.

## Who should use it, and why

Anyone installing a server, and the owner's planned reinstall: Frankfurt
(Debian 13, first server, PowerDNS as main DNS, mail) and Boston (Ubuntu
24.04, joined by code) are wiped and installed again with this release.
Installed panels on alpha.113 may update from the panel's own screen (schema
46 to 51; the update keeps the old panel port, see below). Servers on
alpha.111 or alpha.112 that already hold the newer DNS state are refused by
the update; reinstall them.

## What changed

**Setup and the first screen**
- The first question is "What will this server be?": a first server, or a
  server that joins an existing one. The join path asks only for the address
  and the code.
- The main domain becomes a website by default. On the first server the
  wizard asks "Will the main domain's website be on this server?" with Yes as
  the default. Yes: setup ends with the site (the bare name and `www`) and its
  Let's Encrypt certificate, through the same path as Add domain. If the
  certificate cannot be issued, the site stays and the step says why. No: the
  name stays DNS-only, the wizard says so in plain words and the
  Domains list has an "Open as website" button. A joined server is never
  asked: the main domain lives on the first server.
- The final summary puts what stops the start first, in plain words, with the
  button that fixes it. The ready page lists what is left for the owner
  (website, registrar, advice), each with its action.
- A server that joined by code can run mail for its own customers. It never
  takes the main domain's mail records; it publishes only its own mail name.
  A mail name that the first server, the main domain or a name server already
  uses is refused with the reason and a way to fix it.
- Installing a mail profile from Components no longer sends the owner into
  the setup wizard. A dialog asks only for the mail name, lists the steps,
  runs them and shows the result in place. On a joined server the main
  domain's records are not touched.
- The running DNS software is shown as one fixed line; setup does not
  pretend it can be switched.

**Users and resellers**
- Customer and reseller accounts with plans and quotas. A limit of 0 means
  unlimited for domains, databases, mailboxes and disk.
- An administrator or reseller can assign or change a plan after the account
  exists. A change below current use is refused with the list of what is over,
  until the person confirms; nothing is deleted.
- Resellers have their own plans and a pool set by the administrator
  (customers, domains, databases, mailboxes, disk). Going over the pool is
  refused with the reason.
- "My plan" shows the account's own limits and use on the Dashboard and in My
  account. A refused action that hit a quota points there.
- "Log in as" is accountable: the audit log records who acted as whom, a
  banner names the operator, and the session ends with the operator's rights.
- Forgot password by email: one answer for every name, rate limits, a
  single-use link valid for 30 minutes, sent only to a confirmed address.
  Saving a new password signs the account out everywhere.
- Invitations: an account can be created without a password; the person
  chooses it through a link valid for 7 days. If the server cannot send mail,
  only the inviter gets the link, with the reason. Pending invitations can be
  sent again, replaced or cancelled. The Users list shows "Invited, not signed
  in yet".
- Email addresses are confirmed on My account; any change clears the
  confirmation.
- Bandwidth is not measured, so it is not a plan limit and plan forms do not
  show it.

**Small Plesk-parity gaps**
- Password-protected folders per site (nginx basic auth in the site's own
  file; the password file is outside the web root).
- Redirects per site: exact or folder match, 301, 302, 307 or 308, with an
  option to keep the query. Checked for loops, duplicates and conflicts.
- Out-of-office reply per mailbox (Dovecot Sieve). Dovecot sends it, so it
  keeps working when the panel is stopped or removed. A Sieve script, drop-in
  or transport map the owner edited is kept and the change is refused with a
  reason.
- Two-step sign-in recovery codes: ten single-use codes shown once, new codes
  with password and code, spent once.
- The DNS overview: a mail record missing from this server's list is not a
  wait. The optional IPv6 name-server records are never a wait; the card says
  they are optional and what to add at the registrar. A joined server calls
  the other one "First server", and the first one calls it "Second server".
  A main DNS server waiting for its backup is not shown as "no DNS engine".

**Operating-system updates**
- Debian and Ubuntu: the owner chooses automatic or manual updates. The panel
  uses apt's own file `/etc/apt/apt.conf.d/20auto-upgrades`. If the owner
  edited that file, the panel says so and replaces it only after the owner
  has read it and confirmed.
- Arch has no switch. CelikPanel never runs `pacman -Syu` by itself. The page
  gives step-by-step guidance and warns about PHP and PostgreSQL major
  versions.
- The Dashboard shows how many updates are waiting (a cached, read-only
  count; it never starts a change).
- The operating-system card explains the system that is running.
- A new public page lists the supported systems (Debian 12 and 13, Ubuntu
  22.04 and 24.04, Arch), what is offered on each, what is not and why. It is
  in English and Turkish and is linked from the download portal.

**Panel port 1923 (D-041)**
- A fresh install listens on 1923, because 2083 is cPanel's port. A server
  installed before this release keeps its port (2083 on the owner's servers):
  the update never rewrites it.
- Settings shows the current panel address and a "Change" button. The panel
  binds the new port and opens it in the firewall, the owner confirms from the
  new address, and only then is the port written. A timeout (10 minutes), a
  cancel or a restart keeps the old port. A file the owner edited is reported,
  not overwritten.
- A server that is part of a pair or has a joined server refuses the change
  (`PANEL_PORT_PAIRED`), because the other server stores its address. This
  waits for alpha.115 (see Known limits).

**Fixes found by the lab runs**
- A joined server's setup stopped with "code expired or used" while the first
  server was only busy. The first server now tells a real refusal from a busy
  moment; the joined server waits, names the server it waits for and goes on.
- A DNS record added while automatic DKIM wrote the same zone answered
  "internal error" although the record was saved. It now answers saved, says
  the DNS servers get it within a minute, and publishes by itself.
- Debian 13: turning on an out-of-office reply made every message to that
  mailbox bounce. Dovecot's mail delivery now looks the mailbox up by its full
  address, the panel asks Dovecot before routing any mail, and a mailbox that
  Dovecot refuses keeps Postfix's own delivery. Nothing bounces.
- A folder redirect lost the rest of the path when the address had a query
  (`/blog/post?q=1` went to the bare target).
- A customer domain on a joined server got no MX, SPF, DMARC or DKIM records.
  They are now added on the main DNS server (add only; the owner's records are
  kept) and removed with the zone.
- Updating a PowerDNS first server installed with alpha.113 was refused as
  "application compatibility". The check now knows that alpha.113 wrote the
  newer DNS state. The update goes through, and a rollback to alpha.113 keeps
  that state.
- Mail added later (from Components) could wait for ever on its final check.
  It now proves the live connection, names each open check, says when a check
  is overdue and offers "Check again".
- Arch: the out-of-office reply failed on reload ("Unknown section name:
  sieve_script") while `systemctl reload` said success. The mail install now
  includes the Sieve package, every Dovecot reload is checked in Dovecot's own
  log, and an old install is restarted once, with a warning, when that is the
  only way.
- With Dovecot stopped, saving an out-of-office reply gave the wrong reason.
  It now says Dovecot is stopped, and nothing is routed.
- Deleting a domain on a joined server stayed at "DNS cleanup, unknown" when
  the main DNS server needed time to confirm with its backup. The joined server
  now asks again by itself (15 s, doubling to 5 minutes) and finishes. The
  status never says "unknown".
- After an update was rolled back and verified, the update record said
  "failed, recovery required" with an empty cause. It now says the previous
  version runs, gives the installer's reason and keeps the installer's last
  line as the detail (paths, addresses and secrets removed).
- Debian 13 and Ubuntu 24.04: with the new Dovecot reload check, mail setup
  never finished and the Agent then held every change. The check now fails
  only when Dovecot's own log records a refusal, and it reads both the 2.3 and
  the 2.4 log order.
- An update started a few seconds after a rollback could leave the panel
  stopped. A refused update now leaves the panel and the Agent running, and
  the update waits up to a minute while the Agent finishes a short operation.
- Debian 13: the first server's setup could stop at the webmail step with
  "firewall sync failed" while DKIM was being installed. The firewall step now
  waits while the server is busy (up to 10 minutes) and says so.
- When an out-of-office reply was refused because Dovecot did not accept the
  mailbox, the next message could still bounce. Postfix is now reloaded after
  every change to the out-of-office routes.
- While a joined server waited for the main DNS server to remove a deleted
  domain, the screen could say the main server was unreachable while it was
  only slow, or give no reason at all. It now says "confirming" when the main
  server got the request, "unreachable" only when it could not be reached,
  and a finished deletion leaves the list by itself.
- A build-size limit on one Turkish text file would have failed the web build;
  the file was split. No effect on use.
- Screens (24 findings of the first review and 4 of the second): wording and
  buttons on the wizard, Dashboard, Domains, Plans, Settings, Users,
  Components and the Audit log. Examples: the Dashboard says
  "`<domain>` has no website yet" with a button that opens Add domain (not on
  a joined server); the
  administrator's own workspace has no plan limit and no hidden number;
  Add domain starts with the certificate box ticked when this server serves
  the name's DNS; the audit log shows words, not action codes; a one-server
  setup no longer mentions "the server that will join".
- Every change re-reads every screen that shows it, so a list updates by
  itself after an action and nobody needs to refresh. This includes the
  Domains list after a site is created on a DNS-only name (no stale
  "DNS only" row).
- A refused change stays on the page where it happened, with the reason and
  the fix; a failure message stays until it is closed. A database password the
  server made up stays on the page until closed, with a Copy button and a
  note that it is not shown again. A disabled main button says why.
- "Services" is called "Components" everywhere.

## Known limits

- The panel port cannot be changed on a server that is part of a pair, and
  the owner's Frankfurt and Boston will stay on 2083 until alpha.115 announces
  the new endpoint to the other server.
- The wildcard certificate is not part of this release (alpha.115).
- There is no SMTP relay. Panel mail (forgot password, invitations,
  confirmations) goes through this server's own mail system on `127.0.0.1:25`;
  without one, the screens say so and who can help.
- On a joined server the main domain's DKIM key is on the first server.
  Whether invitation mail sent from the joined server stays DKIM-aligned with
  the From domain is not proven.
- `install.sh` still writes the old `20celikpanel-auto-upgrades` file for
  fresh installs. The new choice uses `20auto-upgrades`.
- After an invitation is cancelled, the same person has to be invited again
  from the start.
- The update card still shows the text about an "earlier attempt" and says
  that trying again is safe, even where the same version will stop the same
  way.
- Servers on alpha.111 or alpha.112 that hold the newer DNS state are refused
  for update. Reinstall them.
- Turning off mail for a single domain does not remove that domain's mail
  records.
- A plan shows "∞" for an unlimited limit.
- The joined server's Dashboard hides all zone cards, so a DNS-only customer
  zone on a joined server gets no card there (the Domains row still offers the
  action).
- On the main server, a pending joined-server deletion with a code that was
  never reviewed still shows as `dns_cleanup` without a reason.
- Nothing is measured on an installed server. No P0 item is closed or
  advanced; a candidate is not acceptance.

## Evidence

Tests on `1bd1793fe`: Go 69 packages ok, 7901 tests passed; Agent 5084 passed,
0 failed; deploy contracts pass; web 2216 passed, type check and the build
with the bundle budget pass. Sequence 114, schema 51 (migrations 047 to 051:
panel port change, accounts, plans and resellers, account recovery, protected
folders, redirects and recovery codes, mail autoresponders; all additive).

Lab acceptance runs (summaries are on branch `lab/alpha114-acceptance`, under
`deploy/e2e/dns-pair-acceptance/evidence/`):

- Run 1, `final-alpha114-run1-20261031/SUMMARY.md`, candidate `8303c0a87`.
  Not releasable: 8 defects (E1 to E8) and 24 screen findings. E7 blocked the
  update of PowerDNS first servers. All were fixed.
- Run 2, `final-alpha114-run2-20261101/SUMMARY.md`, candidate `7d1d4ac7a`. Not
  releasable: all of run 1 passed, but it found one build blocker and two new
  High defects (E-new-1, E-new-2, E-new-4), and stopped early on the fail-fast
  rule. E-new-1 to E-new-5 and four new screen findings were fixed afterwards.
  Among run 2's passes: the update 113 to 114 on a PowerDNS first server and
  both pair servers (schema 46 to 51, port 2083 kept, rollback verified, a
  second update succeeded).
- Runs 3 to 6 were short, targeted checks (`final-alpha114-run3..run6-20261101`),
  each stopped at its first serious failure, then fixed:
  - Run 3 (`aa84d2307`): mail setup never finished on Debian 13 and Ubuntu
    24.04 (Dovecot reload check), and a refused update right after a rollback
    left the panel stopped on Arch.
  - Run 4 (`449701242`): both run 3 defects passed. The Arch out-of-office
    reply passed on an updated and a fresh server. Debian 13 setup stopped on
    a busy firewall step; the rollback record had an empty detail.
  - Run 5 (`d70c1d7ce`): both servers finished setup; Dovecot-stopped passed.
    One mail bounced after a refused out-of-office save.
  - Run 6 (`69df35d0a`): the out-of-office reply passed on Debian 13 and
    Ubuntu 24.04, including right after a refusal. Deleting a domain on the
    joined server finished by itself and removed its DNS in every case:
    normally, with the first server's panel stopped, and across a reboot of
    the joined server. The reason shown while it waited was wrong or missing;
    that wording was fixed afterwards and is tested in source, not in the lab.

Not covered in the lab after the last fixes, by the owner's choice of a short
final run: the screens of the last round (a joined server offers no "Host a
website", DNS software names, audit log words), the forgotten-admin command
in a terminal, and the rollback detail of a real failed update. They are
tested in source. Sequence 114, schema 51. No P0 item is closed or advanced;
a candidate is not acceptance.
