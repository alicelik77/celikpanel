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
| Password guessing | Only listed addresses reach the engine at all, and no rule is in force before wrong passwords are slowed down (review M3): PostgreSQL rules are written only while `auth_delay` (official contrib module) is loaded — it waits 1 s after every failed sign-in; MariaDB copies exist only while CelikPanel's own Fail2ban jail `celikpanel-mariadb-auth` runs (5 failures in 10 minutes ban for an hour). Until then the rules wait and the administrator is told the step. |
| Sniffing | PostgreSQL rules are `hostssl` lines: a connection without TLS matches no line and is refused. MariaDB remote accounts are created `REQUIRE SSL`. |
| A customer reaching another customer's database | A rule names one database, one database user and one address. The panel only accepts a rule for a database and a user of the caller's own subscription, the user must already hold access to that database, and the engine enforces the triple natively (pg_hba line; MariaDB account with grants on that database only). |
| The administrator's own accounts | Rules are made only for panel-made database users (`database_users`). The engine superuser (`postgres`, `root`), the panel's own engine account and any role with superuser/replication rights are refused. PostgreSQL lines never name `all`, `replication` or `sameuser`; MariaDB never gets a `%` host. |
| A stale remote copy (MariaDB) | A MariaDB account is `user@host`, so a remote copy is a second account. Deleting the user, revoking its access, changing its password or connection limit, and suspending or restoring it act on every address-form copy of that user (driver and Agent), so a copy never outlives, out-privileges or out-lasts a lock of the local account. Copies carry the local `max_user_connections` and lock state. |
| An owner's own network access | An owner-set listen address (`bind-address`, `listen_addresses`), a lost Agent record, or a `pg_hba.conf` line outside CelikPanel's block that lets a network address in (`host … 10.0.0.0/8 …`, `all`, `samenet`, an include) means the owner manages network access: CelikPanel opens nothing there (and closes what it opened), and says which line or setting to change. |

## Decisions

**Plan permission.** `database_remote` is an opt-in permission (like
`nginx_directives`): closed for everyone, Unlimited plans included, until the
administrator offers it on the server (`admin_offered_permissions`). Turning
the offer off is the server switch: the Agent is asked to apply an empty rule
set, so every remote path closes; the rows are kept and come back when the
offer is turned on again. A subscription without the permission, suspended or
expired contributes no rules. A suspension or its end, a plan or rights change,
a grant or revoke and a deletion apply the set again at once; the panel also
re-checks every five minutes and retries waiting rules then.

**Rule shape.** One rule = (database, database user, address). Addresses:
a single IPv4 or IPv6 address, or a small range (IPv4 /24 to /32; IPv6 /64 to
/128, PostgreSQL only — MariaDB matches IPv6 hosts as text, so it takes single
IPv6 addresses). Refused: `0.0.0.0/0`, `::/0`, anything wider than the limits,
loopback, unspecified, multicast, link-local and IPv4-mapped IPv6. Private
ranges are allowed (VPN and office networks are a real use). A range with
host bits set is written in its canonical form. At most 10 addresses per
database user on one engine, checked in the insert itself; a user over it (rows
written around the panel) loses only its own rules.

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

The block is last, so the owner's own lines decide first; that is why any
owner `host*` line for a non-loopback address means CelikPanel opens nothing on
that cluster. Lines are written only while the cluster runs TLS and has
`auth_delay` loaded. A block whose lines
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
   kept loaded by `/etc/celikpanel/database-remote-guard.sh`, run at boot
   before the network by `celikpanel-database-remote-firewall.service` and
   every 30 seconds by its timer, so `nft flush ruleset` heals by itself):
   at priority -10, loopback is accepted, listed addresses are accepted for
   the engine's port, every other packet to that port is dropped. A drop is
   final in nftables, so this restricts the port whatever other filter exists.
2. The panel firewall (`celikpanel_fw`, default drop) gets the engine's port
   among its open ports only while the Agent's last answer has rules in force
   there, its filter holds the port and is proven loaded (probed every
   minute; review H1/L2). Anything unknown keeps the port shut. An accept in
   layer 1 alone cannot pass layer 2's drop. The persisted policy format is
   unchanged.

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
  address); `auth_delay` is used instead. CelikPanel sets it only when
  `shared_preload_libraries` is empty; an owner value is never rewritten — the
  administrator is told the exact value to set.
- The engines' units do not depend on the filter unit; the filter loads
  before the network at boot and the guard reloads it within 30 seconds.
- MariaDB IPv6 ranges are refused (single IPv6 addresses only).

## Evidence

Unit tests: address validation, pg_hba block rendering and edit detection,
MariaDB account/grant rendering and grant parsing, nft file rendering, option
file rendering, the record, the Agent's MariaDB path with a fake engine
(accounts made with the copied hash and REQUIRE SSL, refused rules never
reach the engine, an owner's account and an edited option file are left
alone, closing removes only CelikPanel's accounts), authorization (another
customer's user or database, team members, the server switch), the
off-by-default path, suspended and ungranted rules left out. Review items:
TLS before any hostssl line, owner pg_hba lines, copies' limit and lock,
suspension of copies, rules waiting for auth_delay / the jail, firewall ports
only when filtered and loaded, the guard, the atomic limit. The Fail2ban
failregex is tested against journal and error-log line formats of MariaDB
10.6/10.11/11.x. The rendered nft file was accepted by `nft --check`, the
guard parses (`sh -n`) and its units pass `systemd-analyze verify` (Debian 13,
nothing loaded).

Not yet run on a real system (the lab's, on Debian 12, Ubuntu 22.04 and
Arch Linux): a listed address connects with TLS; an unlisted address is
refused by the filter; a non-TLS connection is refused; the restart turns
the listen address on; removing the last rule closes at once and, after the
restart, the engine listens locally again; a password change and a user
deletion carry to the MariaDB copies; reboot keeps the filter
(`celikpanel-database-remote-firewall.service`), `nft flush ruleset` heals
within 30 s; Fail2ban's systemd backend reads MariaDB's journal on each
distribution and bans after five wrong passwords; `auth_delay` loads after
the restart.
