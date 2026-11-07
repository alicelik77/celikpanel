# Remote access to databases (alpha.129)

Status: design accepted by the owner on 7 Nov 2026; implementation in
alpha.129 (branch `feat/a129-db-remote`). This note says what is built, why,
and what is still only unit-tested. It is not a statement that every part has
run on a real server: see "Evidence" at the end.

## The owner's decision

Connections from outside to a site's databases (pgAdmin, DBeaver, HeidiSQL)
are **off by default**. A customer connects **only from addresses they added
themselves**; connections are **encrypted**; **wrong passwords are slowed
down**; the feature is **granted by a plan permission**. Security before
freedom.

## Threat model

| Threat | Answer |
|---|---|
| Internet-wide scanning of 3306/5432 | The engine keeps listening on the local address until the first rule exists on it. Afterwards an nftables filter of its own drops every packet to the engine's port that does not come from a listed address, at a priority before every other filter, also when the panel firewall is off. |
| Password guessing | Only listed addresses reach the engine at all. PostgreSQL: `auth_delay` (official contrib module of the distribution package) waits 1 s after every failed sign-in. MariaDB: a Fail2ban jail (`mysqld-auth`, the filter Fail2ban ships) bans an address after 5 failures in 10 minutes, when Fail2ban is installed. |
| Sniffing | PostgreSQL rules are `hostssl` lines: a connection without TLS matches no line and is refused. MariaDB remote accounts are created `REQUIRE SSL`. |
| A customer reaching another customer's database | A rule names one database, one database user and one address. The panel only accepts a rule for a database and a user of the caller's own subscription, the user must already hold access to that database, and the engine enforces the triple natively (pg_hba line; MariaDB account with grants on that database only). |
| The administrator's own accounts | Rules are made only for panel-made database users (`database_users`). The engine superuser (`postgres`, `root`), the panel's own engine account and any role with superuser/replication rights are refused. PostgreSQL lines never name `all`, `replication` or `sameuser`; MariaDB never gets a `%` host. |
| A stale remote copy (MariaDB) | A MariaDB account is `user@host`, so a remote copy is a second account. Deleting the user, revoking its access or changing its password acts on every host of that user name (driver and Agent), so a remote copy never outlives or out-privileges the local account. |

## Decisions

**Plan permission.** `database_remote` is an opt-in permission (like
`nginx_directives`): closed for everyone, Unlimited plans included, until the
administrator offers it on the server (`admin_offered_permissions`). Turning
the offer off is the server switch: the Agent is asked to apply an empty rule
set, so every remote path closes; the rows are kept and come back when the
offer is turned on again. A subscription without the permission, suspended or
expired contributes no rules. The panel re-checks this every five minutes and
after every change, so a plan change takes effect within minutes.

**Rule shape.** One rule = (database, database user, address). Addresses:
a single IPv4 or IPv6 address, or a small range (IPv4 /24 to /32; IPv6 /64 to
/128, PostgreSQL only — MariaDB matches IPv6 hosts as text, so it takes single
IPv6 addresses). Refused: `0.0.0.0/0`, `::/0`, anything wider than the limits,
loopback, unspecified, multicast, link-local and IPv4-mapped IPv6. Private
ranges are allowed (VPN and office networks are a real use). A range with
host bits set is written in its canonical form. At most 10 addresses per
database user on one engine.

**Listen address.** It changes only when the first rule exists on that engine
(PostgreSQL: that cluster) and goes back with the last one. Both engines read
it only at start, so the change waits for a restart, which is the
administrator's decision: the screen says so and points to Components. Closing
does not wait: the last rule's removal removes the lines/accounts and the
firewall opening at once, and the nftables filter keeps dropping the port until
the engine has restarted onto the local address.

- PostgreSQL: `ALTER SYSTEM SET listen_addresses = '*'` (PostgreSQL's own
  `postgresql.auto.conf`), `ALTER SYSTEM RESET` to go back. If
  `listen_addresses` already names anything other than the local addresses,
  or `postgresql.auto.conf` holds a value CelikPanel did not set, the owner
  manages it: CelikPanel changes nothing and says so.
- MariaDB: a CelikPanel option file (`/etc/mysql/mariadb.conf.d/90-celikpanel-remote-access.cnf`,
  Arch `/etc/my.cnf.d/celikpanel-remote-access.cnf`) with `bind-address = *`,
  checked by `mariadbd --help --verbose` before it is written, removed with
  the last rule. If MariaDB already listens beyond the local address (or has
  `skip-networking`) without this file, the owner manages it. A changed file
  is never overwritten.

**PostgreSQL rules.** A managed block at the end of the cluster's
`pg_hba.conf` (`SHOW hba_file`), written through the validated writer of
`db_config.go` (line check, local-administrator lockout check, backup,
atomic replace, PostgreSQL asked about the file before the reload, reload,
put back on refusal):

```
# BEGIN CELIKPANEL REMOTE DATABASE ACCESS (managed by CelikPanel; edit outside this block)
hostssl "shop_db" "shop_user" 203.0.113.7/32 scram-sha-256
# END CELIKPANEL REMOTE DATABASE ACCESS sha256=<digest of the lines>
```

The block is last, so the owner's own lines decide first. A block whose lines
no longer match their digest was edited by hand and is not rewritten. A role
whose password is still stored as md5 is refused with the fix ("set a new
password once").

**MariaDB rules.** For each (user, address) one account `'user'@'host'` with
the local account's plugin and password hash (copied, never the password),
`REQUIRE SSL`, and only the grants the local account holds on the databases
named by rules for that address (copied from `SHOW GRANTS`). The Agent keeps
the list of the accounts it made in `/etc/celikpanel/database-remote.json`; an
account of the same name and host that it did not make is the owner's and is
never changed (the rule is reported as a conflict).

**Firewall.** Two layers:

1. `table inet celikpanel_dbremote` (file `/etc/celikpanel/database-remote.nft`,
   loaded at boot before the network by the oneshot unit
   `celikpanel-database-remote-firewall.service`, which runs only `nft -f`):
   at priority -10, loopback is accepted, listed addresses are accepted for
   the engine's port, every other packet to that port is dropped. A drop is
   final in nftables, so this restricts the port whatever other filter exists.
2. The panel firewall (`celikpanel_fw`, default drop) gets the engine's port
   among its open ports while rules exist on it; an accept in layer 1 alone
   cannot pass layer 2's drop. The persisted policy format is unchanged.

**TLS.** An engine that already runs TLS keeps its own certificate (Debian's
PostgreSQL ships with `ssl = on` and the snakeoil pair). Otherwise the Agent
makes one self-signed certificate (ECDSA P-256, 10 years, CN = host name)
under `/etc/ssl/celikpanel-database/<engine>/`, key `0640 root:<engine group>`,
and turns TLS on (PostgreSQL: `ALTER SYSTEM SET ssl = on, ssl_cert_file,
ssl_key_file` plus reload — no restart; MariaDB: `ssl_cert`/`ssl_key` in the
option file above — at the restart). There is no renewal to fail: the
certificate is valid for ten years and the connection settings say
`SSL mode: require` (encrypted; the certificate is not verified). The owner can
replace the two files with their own at any time; CelikPanel never rewrites
them once they exist. Using the panel's public certificate (verify-full) is a
later option: it needs a renewal hook into two engines and is not required by
the decision.

**What survives without the panel.** Everything is native: pg_hba lines,
`postgresql.auto.conf`, MariaDB accounts and an option file, an nft file and a
unit that only runs `nft`, a Fail2ban jail file. With the panel stopped or
removed the listed customers keep connecting and nobody else can. Removing the
panel leaves these explicit, commented rules for the owner to read or delete.

## Client settings shown to the customer

Host: the server's host name; Port: the engine's (cluster's) port; Database,
User: as listed; Password: the database user's password; SSL mode: `require`
(pgAdmin "SSL mode: require"; DBeaver "Use SSL" + "require"; HeidiSQL
"Use SSL" checked, no certificate files).

## Endpoints

- `GET  /api/v1/database-remote/databases/{id}` — rules, access, engine state,
  client settings (customer, reseller, administrator; ownership-checked).
- `POST /api/v1/database-remote/databases/{id}/rules` `{user_id, address}`.
- `DELETE /api/v1/database-remote/rules/{id}`.
- `GET|PUT /api/v1/database-remote/server` — administrator: the offer switch,
  per-engine state, "apply again".

## Known limits (alpha.129)

- Restarting the engine after the first rule is the administrator's step in
  Components (no automatic restart).
- Fail2ban for PostgreSQL is not used (its default log lines carry no client
  address); `auth_delay` is used instead, and only when
  `shared_preload_libraries` is empty: an owner value is never rewritten.
- MariaDB IPv6 ranges are refused (single IPv6 addresses only).

## Evidence

Unit tests: address validation, pg_hba block rendering and edit detection,
MariaDB account/grant rendering and grant parsing, nft file rendering, option
file rendering, authorization (another customer's user or database), the
off-by-default path. Real-system checks (listed address connects with TLS,
unlisted refused, non-TLS refused, last rule returns to local) are the lab's,
on Debian 12, Ubuntu 22.04 and Arch Linux.
