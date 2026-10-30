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
  the outcome (issued, failed with its reason, waiting).
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
- Two btrfs fixes in the undo of an interrupted PowerDNS install: a
  directory's link count and a boot-scoped device number are no longer taken
  as identity proof.
- Daily operations on a pair: a change made while the backup is catching up
  waits briefly, then says so instead of "no DNS engine"; a held publication
  names its cause.

**Services**
- Service versions are read from the package database. Starting a program
  to ask its version is gone (it made cron log "can't lock crond.pid").

## Known limits

- Arch: PowerDNS only as a first install (no switch from BIND); no automatic
  upgrades; Redis (Arch ships Valkey instead), phpPgAdmin and SpamAssassin
  are not offered, because Arch's official repositories do not carry a
  working package; the panel says what the administrator can do instead.
- Certificates issued with ZeroSSL or Google by an earlier release are not
  moved automatically; the certificate card asks to reissue once. Snap
  certbot is not covered.
- The automatic deletion check is set up when a pair is built on this
  release. A pair installed earlier and updated keeps the owner tool path
  (the update does not add the tools directory).
- A changed `/etc/resolv.conf` reaches an nginx proxy target only when that
  site's configuration is next written.

## Evidence

Distro config matrix on five releases (Debian 12 and 13, Ubuntu 22.04 and
24.04, Arch) with 0 failures; real boots of Ubuntu 24.04 and Arch renewing
certificates with the panel removed; Arch parity and Arch PowerDNS lab runs;
see `deploy/e2e/dns-pair-acceptance/evidence/*-20261030/SUMMARY.md`.
Sequence 113, schema 46. No P0 item is closed or advanced; a candidate is
not acceptance.
