# Data and access: design notes for G16, G14, G15, G6

*alpha.128 · Design notes only. Nothing in this file is shipped. They were
written when the gap items (PLESK-GAP v3) were judged too large or too
security-sensitive to finish safely in alpha.128. G7 (database tools) and G24
(visitor statistics) shipped in the same release; see the end of this file.*

Each note gives the decision, how to build it the CelikPanel way (native
configuration, owner independence D-022, operation guidance D-024, resilience
contract D-025), and what must be proved before it ships.

## G16. More FTP/SFTP accounts per site

**Status: design note only.**

**Why not tonight.** Today a site has exactly one access account: the site's
own system user (alpha.117, `cmd/agent/file_access.go`). Files uploaded over
FTP or SFTP are owned by the user that the site's PHP-FPM pool and Node unit
run as, so nothing needs re-owning. A second login has to keep that property.
None of the ways to do so can be built and proved on five systems in one
night:

| Way | What breaks |
|---|---|
| A second system user with the same uid | Two names for one uid. `ps`, quotas and audit log name the wrong account, and the passwordless-mail and sandbox rules key on the uid. Refused. |
| A second system user in the site's group plus ACLs | Files are owned by the second user. PHP must be able to write them, which needs a default ACL on every folder and a umask that the FTP daemons do not all honour. Moving or removing the account leaves orphan owners. |
| vsftpd virtual users (`guest_enable`, `guest_username=<site user>`, `user_config_dir` with `local_root` per account) | Needs a PAM password-file module. Debian and Ubuntu package `libpam-pwdfile`; Arch has it only in the AUR (not an official package, so refused by the security-before-freedom rule). It covers FTP only. |
| sshd `Match User` per extra login, chrooted to a sub-folder | sshd needs a real account per login. The chroot path must be root-owned all the way down, which a folder inside the site is not. |

**Recommended design (for a later release).**
- FTP over TLS: vsftpd virtual users mapped to the site user, chrooted with
  `local_root` to the chosen folder. The password file is written as a
  root-only file under `/etc/vsftpd/celikpanel-users/`, hashed with the same
  rule as mailbox passwords (alpha.127 rule: at least 6 characters, upper and
  lower case, a special character), and owned by CelikPanel with the sealed
  header (D-031). It is enabled only on systems where the PAM module is an
  official package. Elsewhere the screen says so.
- SFTP: a second login needs a bind mount of the chosen folder onto a
  root-owned chroot (`/srv/celikpanel-sftp/<account>/files`), made by a
  systemd mount unit so it survives a panel removal (D-022). It is proved with
  `sshd -T -C user=<login>` and an escape test.
- Plan: a count `ftp_accounts` (0 = none beyond the site account), checked on
  add. The folder is chosen inside the document root and resolved with
  `openat2(RESOLVE_BENEATH)` so a symlink cannot widen it.
- Steps: check, write account, verify (TLS handshake as the new login; sshd
  -T), finish. Roll back the files on a failed check, as `file_access.go`
  already does.

**To prove before shipping.** The escape matrix on all five systems: no read
outside the folder; no symlink or `..` escape; uploads readable and writable
by PHP; and removal leaves no orphan owner.

## G14. IP addresses

**Status: design note only. There is no dedicated-IP assignment.**

**Facts.** Every vhost CelikPanel writes listens on all addresses (`listen
80;`, `listen 443 ssl;`, `internal/services/nginx*.go`). DNS records for a
site point at the server's one public address chosen at setup
(`server_setup`). Migration 001 has an unused `ip_addresses` table that is
seeded with `0.0.0.0`.

**Why no assignment now.** A dedicated IP needs three things at once: (1)
`listen <ip>:80` and `listen <ip>:443` in that site's vhost, with every other
vhost still answering on the shared address; (2) the A/AAAA records moved to
that address through the existing record path; (3) a certificate check that
the default HTTPS server (alpha.113) does not hand out another site's
certificate on that address. Changing (1) for one site changes which server
nginx picks as the default on that address for every other site. That cannot
be done safely without rendering tests across all vhost kinds (PHP, Node,
proxy, suspended, maintenance) and a lab round. So it is not offered.

**Next step (small, safe, read only).**
- An Agent read `ip -j addr show` (iproute2, present on all five systems)
  lists each address with its interface, scope and family, leaving out
  loopback and link-local.
- The Panel marks each address as *shared*: every site answers on every
  address, so the "sites" count is the number of hosted sites. The address
  the DNS records use is marked as such. The page is for administrators only.
- The dead `ip_addresses` table is dropped in the same additive-safe
  migration once nothing reads it (it has no readers today; check with
  `grep -rn ip_addresses`).

**Dedicated IPs (v0.5).** These need a per-site `listen_address` column, the
vhost renderer emitting `listen <addr>:…` for that site and `listen
<addr>:… default_server` for a per-address catch-all, DNS moved in the same
operation, and a plan item "dedicated IP" taken from a pool. Steps: check,
render, `nginx -t`, reload, DNS, verify. On a failed `nginx -t`, roll back.

## G15. Remote API tokens

**Status: design note only.** This is security-critical. It would ship only
complete, with its tests. A partial version is not shipped.

**Design.**
- **Token format.** The token is `cpk_<id>_<secret>`. `<id>` is 12 base32
  characters and is stored in plain text for lookup. `<secret>` is 32 random
  bytes in base64url. Only `SHA-256(secret)` is stored, and the comparison is
  constant-time. The full token is shown once, in the answer to its creation.
  The request-identity guard keeps only the status of that answer
  (`secret: true`).
- **Ownership.** A token belongs to one account (`user_id`), never to a
  session. Removing or suspending the account stops its tokens. A team member
  cannot hold one.
- **Scope.** Each token has a name, a set of scopes and an optional expiry.
  The scope is either `read`, or `read + write` limited to named families
  (domains, dns, mail, databases, backups). The effective rights are always
  the account's rights at call time. A token never grants more than its
  owner has, and a plan change takes effect at once.
- **Plan permission.** `remote_api` is off by default. The reseller and plan
  layers apply as for every other right (D-042). An administrator always has
  it.
- **Transport.** Tokens are accepted only as `Authorization: Bearer`, never in
  a query string. They are refused on the browser routes that need a session
  (password, two-factor, token management itself). A request authenticated by
  a token carries no session cookie. CSRF checks do not apply to it, so a
  cookie must never authenticate the same request.
- **Audit.** Every call is written to the audit log (token id, never the
  secret), plus a last-used time and address on the token. Failed sign-ins
  are rate-limited per token id and per address, like the panel sign-in.
- **Storage.** Additive table `api_tokens(id, user_id, name, token_id UNIQUE,
  secret_sha256, scopes JSON, expires_at, created_at, last_used_at,
  last_used_ip, revoked_at)`.

**Tests required before shipping.**
- A token cannot reach another tenant's domain.
- A read token cannot change anything.
- A revoked, expired or suspended-owner token is refused.
- The secret is never logged or retained.
- Token management cannot be done with a token.
- The route matrix test (`route_role_matrix_test.go`) gains a token column.

## G6. Web application firewall

**Status: design note only.**

**Official packages.**

| System | nginx ModSecurity module | OWASP CRS |
|---|---|---|
| Debian 12/13 | `libnginx-mod-http-modsecurity` (with `libmodsecurity3`) | `modsecurity-crs` |
| Ubuntu 22.04/24.04 | `libnginx-mod-http-modsecurity` (universe) | `modsecurity-crs` (universe) |
| Arch | none in the official repositories (AUR only) | none official |

The Arch column, and Ubuntu's universe (community-maintained, with a security
support level different from main), mean the feature cannot be the same on
every supported system. Under the security-before-freedom rule it can be
offered only where the packages are official. On the other systems the screen
says it is not available there.

**Performance.** libmodsecurity v3 with CRS at paranoia level 1 adds roughly
1–3 ms per request on ordinary pages. Request-body inspection of large
uploads costs more and needs `SecRequestBodyLimit` aligned with the 256 MB
upload ceiling. This has to be measured in the lab on a 1-vCPU server before
it is turned on by default.

**False positives and owner control.**
- The first mode is *detection only* (`SecRuleEngine DetectionOnly`) per site.
  The screen shows what would have been blocked: rule id, path and count,
  from the ModSecurity audit log, read like the traffic log.
- **Blocking** is a per-site switch that the owner turns on after reviewing.
- A rule is excluded per site with `SecRuleRemoveById` in a CelikPanel-owned
  include. Owner edits are detected and kept (D-031).
- Known applications (WordPress, PrestaShop) get the CRS application
  exclusion packages that are shipped with CRS.
- Everything is native nginx and ModSecurity configuration, so it keeps
  working without the panel (D-022). Turning it off is one include line.

**Decision needed (ROADMAP v0.5).** The owner should decide whether a WAF that
cannot exist on Arch, and is community-maintained on Ubuntu, belongs in the
product. The alternative is honest documentation plus fail2ban rules for
common abuse.

## Shipped in alpha.128 alongside these notes

- **G7 database tools.** These cover export, import with a safety copy,
  copy, check and repair, and size and tables, for MariaDB and PostgreSQL.
  An uploaded file is loaded as the database's own user, never as the engine
  administrator (`cmd/agent/database_tools.go`, `database_import_pg.go`).
- **Typed restore refusals.** These are `BACKUP_RESTORE_*` errors, each with
  the engine's own reason. They come from the alpha.127 lab finding.
- **G24 visitor statistics.** These are daily totals from the site's own
  access log, with finished days kept in `site_visitor_stats_daily`
  (migration 070). No address is kept.
