# What is offered on which package ecosystem?

<!-- THIS FILE IS GENERATED — do not edit by hand. -->
<!-- Source: internal/core/managed_services.go · Regenerate: make distro-matrix -->
<!-- Guard: internal/core/distro_matrix_test.go (the test fails when the catalogue changes without regenerating) -->

This list is not written by hand; it is generated from the component catalogue.
The panel's Components page reads the same catalogue — this document and the
screen cannot drift apart.

Rules:

- **A populated cell is a package mapping, not an installation guarantee for
  the whole ecosystem.** Mutation additionally requires a verified manager/systemd
  capability, service-specific lifecycle authorization and, where applicable, an
  exact vendor-artifact or repository-recipe proof. Those final gates inspect real
  host bytes and capabilities, not a distribution-name allowlist.
- **An empty cell (—) is a promise, not a gap:** when the catalogue has no safe
  mapping for that package manager, the row is deliberately closed. A half-install
  is broken.
- **Seat:** components in the same seat do the same job; only one can be
  installed at a time (two web servers cannot share port 80). Choosing the
  other one is a swap, not an addition.
- **Needs** names a ROLE, not a product: a row that needs "an SMTP server" is
  satisfied by any installed member of that seat.

## Installed from distro packages

### Web

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 🐘 PHP-FPM | Runtime | `php-fpm` · version choice: Sury PHP (packages.sury.org) | `php-fpm` | — | — |
| 🔄 Nginx | Service | `nginx` | `nginx` | web server | — |
| 🪶 Apache | Service | — | — | web server | — |

### Database

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 🐘 PostgreSQL | Service | `postgresql` · version choice: PostgreSQL Global Development Group (PGDG) | `postgresql` | — | — |
| 🦭 MariaDB | Service | `mariadb-server` | `mariadb` | — | — |
| 🐬 phpMyAdmin | Tool | `phpmyadmin` | `phpmyadmin` | — | MariaDB, web server, PHP-FPM |
| 🐘 phpPgAdmin | Tool | `phppgadmin` | — | — | PostgreSQL, web server, PHP-FPM |

### E-mail

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 📧 Postfix | Service | `postfix` | `postfix` | SMTP server | — |
| 📮 Exim | Service | — | — | SMTP server | — |
| 🧹 Rspamd | Service | `rspamd` | `rspamd` | spam filter | SMTP server |
| 📬 Dovecot | Service | `dovecot-imapd` `dovecot-pop3d` `dovecot-lmtpd` | `dovecot` | IMAP server | — |
| 🛡️ SpamAssassin | Service | `spamassassin` `spamd` `spamass-milter` | — | spam filter | SMTP server |

### Security

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 🔐 WireGuard VPN | Service | `wireguard` | `wireguard-tools` | — | — |
| 🔐 Let's Encrypt client (certbot) | Tool | `certbot` | `certbot` | — | — |
| 🚫 Fail2ban | Service | `fail2ban` | `fail2ban` | — | — |
| 🧱 Firewall engine | Tool | `nftables` | `nftables` | — | — |
| 🦠 ClamAV | Service | `clamav` `clamav-daemon` | `clamav` | — | — |

### DNS

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 🌐 BIND | Service | `bind9` | `bind` | DNS server | — |
| ⚡ PowerDNS | Service | `pdns-server` `pdns-backend-sqlite3` | — | DNS server | — |

### FTP

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 📂 vsftpd | Service | — | — | — | — |

### Cache

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| ⚡ Redis | Service | `redis-server` | — | kv-store | — |
| 🗝️ Valkey | Service | `valkey-server` | `valkey` | kv-store | — |
| 💾 Memcached | Service | `memcached` | `memcached` | — | — |

### Monitoring

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| 📈 Netdata | Service | `netdata` · version choice: Netdata (repository.netdata.cloud) | `netdata` | — | — |

### System

| Component | Kind | APT package mapping | pacman package mapping | Seat | Needs |
|---|---|---|---|---|---|
| ⏰ Scheduled tasks (cron) | Service | `cron` | `cronie` | — | — |

## Closed cells and their reasons

The Components page and the Agent give the same reason.

| Component | Ecosystem | Reason |
|---|---|---|
| 🪶 Apache | apt | Apache virtual-host integration is not available in CelikPanel yet; use Nginx. |
| 🪶 Apache | pacman | Apache virtual-host integration is not available in CelikPanel yet; use Nginx. |
| 🐘 phpPgAdmin | pacman | phpPgAdmin is not offered on Arch Linux: it is not in the official repositories (only in the AUR, which CelikPanel does not use). The server administrator manages PostgreSQL with psql on the server, or with a desktop client through an SSH tunnel. |
| 📮 Exim | apt | Exim mailbox, TLS and submission integration is not available in CelikPanel yet; use Postfix. |
| 📮 Exim | pacman | Exim mailbox, TLS and submission integration is not available in CelikPanel yet; use Postfix. |
| 🛡️ SpamAssassin | pacman | SpamAssassin is not offered on Arch Linux: its Postfix bridge, spamass-milter, is not in the official repositories (only in the AUR, which CelikPanel does not use), so SpamAssassin would filter no mail. The server administrator installs Rspamd, the spam filter in the same seat, from the Components page. |
| ⚡ PowerDNS | pacman | the managed service lifecycle is not supported in this package ecosystem yet |
| 📂 vsftpd | apt | vsftpd passive-mode ports, TLS and per-subscription access are not integrated in CelikPanel yet. |
| 📂 vsftpd | pacman | vsftpd passive-mode ports, TLS and per-subscription access are not integrated in CelikPanel yet. |
| ⚡ Redis | pacman | Redis is not offered on Arch Linux: its official repositories ship Valkey instead (extra/valkey, which provides redis), and CelikPanel installs official distribution packages only. The server administrator installs Valkey, the compatible cache in the same seat, from the Components page. |

## Installed from the official release (identical on every distro)

These components never bind to a distro package: the panel downloads the
vendor's release, verifies its SHA-256 and installs it the same way on every
Linux (D-018). There is no distro column because the answer is the same in
every column: yes.

| Component | Kind | Seat | Needs |
|---|---|---|---|
| 🟩 Node.js | Runtime | — | web server |
| ✉️ Roundcube | Tool | — | SMTP server, IMAP server, web server, PHP-FPM |

