# v0.1.0-alpha.113

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.113.tr.md)

This release follows v0.1.0-alpha.112. It is the release the owner's servers
are reinstalled with before CelikPanel is opened to others, and it adds Arch
Linux as a server option. **It is a candidate, written on 2026-10-30; it is
not published and nothing from it is on an installed server. Only the owner
updates installed panels, from the panel's own update screen.** Branch
`release/alpha113` from `main` at `b5fa3fae0`; previous release commit
`95c23ef414cc7420e5df6a119b7325cacfdef5d8`.

## Who should use it, and why

Anyone installing a server, and especially the owner's planned reinstall:
Frankfurt (first server, main DNS, mail), Boston (joined by code, backup DNS)
and an Arch Linux server. It fixes what the first two real installs and the
lab runs of 2026-10-30 found, so that no known defect remains in those flows.
Installed panels may update from the panel's own screen; no data migration is
needed (schema 46, unchanged).

## What changed

**Setup and first install**
- The server's full name (`hostname -f`) is used wherever the short kernel
  name was read: server info, the license account's server name, nameserver
  suggestions, the zone template and the panel's first certificate.
- Setup wizard: a start marker left by a wiped and reinstalled server is
  dropped; Enter in the join fields joins; no "Recommended" badge on the
  owner's own choices; the registrar card says there is nothing to do when the
  domain already uses these nameservers.
- A step refused because the server is momentarily busy waits and resumes by
  itself; a read-only lock check no longer makes a real change look busy, and
  a busy answer names the process holding the lock.
- After an interrupted DNS install whose switch journal is kept, the setup
  page no longer says the change was undone; it says who acts and how setup
  resumes. A paired main whose backup is still finishing waits and rechecks
  instead of sitting at "running" with no reason.

**Sites and nginx**
- A new site's welcome page is clearer, names the panel's own sections,
  mentions Applications only on PHP sites and says nothing about the server's
  software.
- "Enable SSL" when adding a domain now requests the certificate and shows
  the outcome (issued, failed with its reason, waiting). When the
  certificate becomes active, http is redirected to https (it can be turned
  off on the SSL/TLS page). A certificate that is valid from a moment a few
  seconds ahead of the server's clock is waited for; a larger clock gap is
  explained (check the server clock) instead of a bare error.
- A site added as static can be switched to PHP: the switch now creates the
  site's own PHP-FPM pool (before it was refused).
- Visitors are not told the software versions: nginx no longer names its
  version (`server_tokens off;`, Ubuntu and Arch named it) and PHP pages no
  longer send `X-Powered-By` (Arch sent it).
- The webmail address redirects http to https.
- nginx: the HTTP/2 form follows the installed nginx (Ubuntu 24.04's 1.24
  refused `http2 on;`), Mozilla intermediate TLS, a default HTTPS server for
  names without a site (also on nginx 1.18), and an unresolvable proxy target
  never stops nginx. The Nginx page shows the default server's state.

**Certificates and mail (D-040)**
- Website certificates are renewed by the operating system's certbot timer,
  with or without the panel; mail SNI certificates follow the renewal.
  Certificates issued by earlier releases move to that renewal when they are
  next renewed.
- Mail TLS uses the full server name; the default mail certificates are made
  by the panel itself (no `openssl` needed); on Arch Postfix gets a server
  certificate before submission is enabled.
- Debian 13 (Dovecot 2.4): delivered mail can be read over IMAP and in
  webmail. Debian's own `10-mail.conf` pointed the inbox at `/var/mail`;
  CelikPanel's drop-in now names the mailbox's maildir explicitly, is
  repaired at the next Panel start on an existing server, and an
  owner-edited drop-in is reported instead of overwritten. The distro matrix
  now delivers a message and reads it back on every release.
- The Agent no longer logs `postmap: fatal: unsupported map type: lmdb` at
  every start on Debian.
- Mail users send (ports 587 and 465) is DKIM-signed and spam-checked on
  every distribution. On Arch those services ran chrooted without the files
  needed to resolve `localhost`, so the filters were skipped. Postfix now
  reaches its filters at `127.0.0.1`, and submission follows the
  distribution's own chroot choice; an existing server is repaired at the
  next Panel start (entries the owner changed are kept and named).
- DKIM: pressing "Generate DKIM key" again no longer removes OpenDKIM's read
  access to an existing key, and a failed signing step is reported instead
  of "key generated".
- Every domain with mail on the server gets a DKIM key and signing by
  itself. This happens when a domain is added while mail is installed, when
  it gets its first mailbox, and when the mail profile is installed from
  Services, which now also sets up OpenDKIM. Before, its mail went unsigned
  until the owner pressed "Generate DKIM key".
  - When the zone is kept on this server, the DKIM record is published there
    and on the backup DNS server.
  - With external DNS, the card shows the record to add at the provider.
  - An existing key or a different DKIM record already in the zone is kept,
    and the card says so.
  - Adding the domain is never held up by this. The card says what is
    happening, who acts and how it resumes, with "Try again".
  - Deleting a domain removes the key CelikPanel made and its record; a key
    the owner made is kept and named.
  - A domain deleted right after it was added no longer races this work:
    nothing is written into or published for a zone being deleted, and no
    "failed" DKIM note is left for a deleted domain. A note an earlier build
    left for a deleted domain is removed at the next Panel start.
- Mail a site sends with PHP `mail()` (and anything else on the server that
  uses `sendmail`) is now spam-checked by Rspamd as well as DKIM-signed, like
  mail sent through ports 587 and 465. Postfix's own bounces are not
  filtered, and a stopped Rspamd never holds mail. An existing server is
  changed at the next Panel start; a filter list the owner changed is kept
  and named.
- The file manager answers a missing path, an existing name or a missing
  download with what to do, not a bare error.
- The Mail authentication card no longer says "In zone — waiting for DNS"
  for a record this server's DNS already answers when only the public
  lookup from the server cannot find the domain (for example before the
  registrar points to these nameservers). It says "Served by this server's
  DNS" and who acts if the domain should already be live.

**License (D-039)**
- One successful check keeps management open for up to seven days offline;
  panels in use check about hourly; a moved clock cannot extend the period.

**Arch Linux**
- The server roles work on Arch with official packages only: web, PHP,
  MariaDB, mail (Postfix, Dovecot, DKIM), DNS, Fail2ban, phpMyAdmin.
- PowerDNS 5.1 from Arch's `extra` repository as standalone, main or backup
  DNS server (first install on a server without a DNS server; D-028
  amendment).
- Security updates are counted and shown; CelikPanel never runs an unattended
  `pacman -Syu` on Arch, because a full upgrade can change PHP or PostgreSQL
  under running sites.
- Components that have no Arch package say so and what to do instead.

**DNS**
- On a pair built with join by code, deleting a zone no longer needs terminal
  work. Before, the deletion waited until both administrators set up the
  read-only deletion check with the `dns-peer-enroll` owner tool, and every
  later DNS change waited behind it. Pairing now sets that check up itself
  over the private setup link, with the same native files the owner tool
  makes (revocable without the panel). An existing owner enrollment or an
  owner-edited `sshd_config` is reported, never replaced. The PowerDNS
  inspector also reads PowerDNS's own catalog row correctly now (this
  affected the manual owner tool too).
- A DNS change made while the backup is briefly unreachable (reboot,
  network blip) is saved and served by the main, and the DNS card shows it
  waiting for the backup with its next check. The panel re-checks the same
  change by itself (15 s, doubling to 5 minutes) and finishes it when the
  backup is back; changes held behind it are then published by themselves.
  Before, it stayed pending and held every later change until the same zone
  was changed again or the panel restarted.
- Deleting a site or zone on a standalone PowerDNS server (no backup DNS
  server) finishes. Before, the server answered "not served here" for the
  deleted zone, the deletion check did not accept that answer, and every
  later change on the server was held, even after an Agent restart. If the
  Agent holds changes, the panel now says why and what the owner does
  (restart the Agent, then retry the same change), instead of "already in
  progress".
- Two btrfs fixes in the undo of an interrupted PowerDNS install: a
  directory's link count and a boot-scoped device number are no longer taken
  as identity proof.
- Daily operations on a pair: a change made while the backup is catching up
  waits briefly, then says so instead of "no DNS engine"; a held publication
  names its cause.
- While a deletion on a pair waits for the backup DNS server to confirm, the
  Agent's log and the domain's deletion status say it is waiting and is
  checked again automatically. Who must act is named only when those
  automatic checks end without confirmation (before, the first check's
  answer already asked the peer administrator to act, although the deletion
  finished by itself seconds later).

**Services**
- Service versions are read from the package database. Starting a program
  to ask its version is gone (it made cron log "can't lock crond.pid").
- After a reboot the Panel waits, for up to 2 minutes and before management
  opens, until systemd reports the server started, then runs its startup
  work (site configuration files, certificate cleanup, mail filter wiring).
  Before, that work was refused while systemd still said "starting" and did
  not run again until the next Panel start. The journal says it waits, and
  then that it continued or, past 2 minutes, what the administrator checks.

## Known limits

- Arch: PowerDNS only as a first install (no switch from BIND); no automatic
  upgrades; Redis (Arch ships Valkey instead), phpPgAdmin and SpamAssassin
  are not offered, because Arch's official repositories do not carry a
  working package; the panel says what the administrator can do instead.
- Certificates issued with ZeroSSL or Google by an earlier release are not
  moved automatically; the certificate card asks to reissue once. Snap
  certbot is not covered.
- Roundcube shows its own version on its login page; Roundcube has no
  setting to hide it.
- The automatic deletion check is set up when a pair is built on this
  release. A pair installed earlier and updated keeps the owner tool path
  (the update does not add the tools directory).
- If a domain is deleted while the backup is unreachable, the panel finishes
  the DNS part by itself when the backup is back; the domain's deletion
  status then asks for "Retry this deletion" once to remove the rest.
- On a server updated from an earlier release, existing domains do not get
  DKIM keys by themselves; their card offers "Generate DKIM key". Keys made
  before this release are kept and named when their domain is deleted.
  Domains and mailboxes brought in by cPanel import do not start automatic
  DKIM.
- A changed `/etc/resolv.conf` reaches an nginx proxy target only when that
  site's configuration is next written.
- PHP `mail()` leaves with the site's system user as envelope sender, so
  Rspamd adds a small `FORGED_SENDER` score (0.3; no action). DKIM is
  aligned with the From domain, so DMARC passes. CelikPanel does not set a
  site address as envelope sender yet, because a domain whose mail or SPF
  is elsewhere could then fail SPF.

## Evidence

Distro config matrix on five releases (Debian 12 and 13, Ubuntu 22.04 and
24.04, Arch) with 0 failures; real boots of Ubuntu 24.04 and Arch renewing
certificates with the panel removed; Arch parity and Arch PowerDNS lab runs;
see `deploy/e2e/dns-pair-acceptance/evidence/*-20261030/SUMMARY.md`.
Sequence 113, schema 46. No P0 item is closed or advanced; a candidate is
not acceptance.
