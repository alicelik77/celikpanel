# Remote access to databases (alpha.129, alpha.130, alpha.131, alpha.133)

Status: design accepted by the owner on 7 Nov 2026; implementation in
alpha.129 (branch `feat/a129-db-remote`); the owner's decisions of 8 Nov 2026
and the alpha.129 lab findings in alpha.130 (section "alpha.130"). This note says what is built, why,
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
| Password guessing | Only listed addresses reach the engine at all, and no rule is in force before wrong passwords are slowed down (review M3): PostgreSQL rules are written only while `auth_delay` (official contrib module) is loaded — it waits 1 s after every failed sign-in; MariaDB copies exist only while CelikPanel's own Fail2ban jail `celikpanel-mariadb-auth` runs (5 failures in 10 minutes ban for an hour) and, since alpha.130, while MariaDB logs addresses rather than host names. alpha.130 adds Fail2ban for PostgreSQL as an extra layer. Until then the rules wait and the administrator is told the step. |
| Sniffing | PostgreSQL rules are `hostssl` lines: a connection without TLS matches no line and is refused. MariaDB remote accounts are created `REQUIRE SSL`. |
| A customer reaching another customer's database | A rule names one database, one database user and one address. The panel only accepts a rule for a database and a user of the caller's own subscription, the user must already hold access to that database, and the engine enforces the triple natively (pg_hba line; MariaDB account with grants on that database only). |
| The administrator's own accounts | Rules are made only for panel-made database users (`database_users`). The engine superuser (`postgres`, `root`), the panel's own engine account and any role with superuser/replication rights are refused. PostgreSQL lines never name `all`, `replication` or `sameuser`; MariaDB never gets a `%` host. |
| A stale remote copy (MariaDB) | A MariaDB account is `user@host`, so a remote copy is a second account. Deleting the user (also a read-only or read-write user added for one database, docs/DATABASE-USERS.md), revoking its access, changing its password (on the Databases page or the domain's) or connection limit, and suspending or restoring it act on every address-form copy of that user (driver and Agent), so a copy never outlives, out-privileges or out-lasts a lock of the local account. Copies carry the local `max_user_connections` and lock state. |
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
(PostgreSQL: that cluster) and goes back with the last one (alpha.131: only
when the administrator turns the server switch off; see "alpha.131"). Both engines read
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

- `GET  /api/v1/database-remote/databases/{id}` — rules (alpha.130: each with
  `expires_at`, RFC 3339 UTC, empty = no end), access, engine state
  (alpha.130: `close_pending`), client settings (customer, reseller,
  administrator; ownership-checked).
- `POST /api/v1/database-remote/databases/{id}/rules`
  `{user_id, address, duration}`; `duration` is `1d` (the default when
  omitted), `7d` or `permanent`; anything else is refused with
  `DATABASE_REMOTE_DURATION`.
- `PUT /api/v1/database-remote/rules/{id}` `{duration}` (alpha.130): a new end,
  counted from now, without adding the address again. Same right as adding.
- `DELETE /api/v1/database-remote/rules/{id}`.
- `GET|PUT /api/v1/database-remote/server` — administrator: the offer switch
  (`offered`), the agreement on MariaDB's host name lookup
  (`mariadb_skip_name_resolve`, alpha.130), "apply again" (`apply`). The GET
  lists every engine instance of this server with its readiness (alpha.130);
  a PUT answers `changed: false` when nothing changed.

## alpha.130

Owner decisions of 8 Nov 2026 and the alpha.129 lab findings. What is
unit-tested here is listed under "Evidence"; the lab steps are in the
release's acceptance notes.

### Host name lookup (MariaDB; was the alpha.129 known limit)

MariaDB looks client host names up by default (`skip_name_resolve` off) and
then names the host, not its address, in a refused sign-in
(`'user'@'_gateway'` in the lab). Fail2ban could only resolve such a name
again by DNS when it bans, and whoever controls that name's DNS can point it
elsewhere by then. Two changes:

- The failregex takes `<ADDR>` (an IP address only), not `<HOST>`: a line
  naming a host bans nobody, never someone else. The alpha.129 filter is
  recognised as CelikPanel's earlier version and replaced.
- MariaDB copies are made only while the running MariaDB has
  `skip_name_resolve` on, in addition to TLS and a running jail. Until then
  the rules wait (`NAME_RESOLVE`), and the administrator is offered one
  action: "Turn off host name lookup". With that agreement (stored as the
  panel setting `database_remote_mariadb_skip_name_resolve`, sent with every
  apply) CelikPanel adds `skip-name-resolve` to its own option file — the
  file that exists only while rules exist — records it
  (`skip_name_resolve` in the Agent's record) and asks for the MariaDB
  restart that the first rule needs anyway. The last rule removes the file
  and with it the setting (it ends at the next restart).

Why an agreement and not automatic: with the lookup off MariaDB matches every
connection by address. Programs that connect through `localhost` (the
socket) are unaffected, but a program on the server that connects through
`127.0.0.1` with a `user@localhost` account, or any account written with a
host name, stops matching. CelikPanel cannot see how every local program
connects, so the administrator decides, with that warning. CelikPanel refuses
on its own when an account names a host (`NAME_ACCOUNTS`, listed), and never
writes over an owner's own `skip-name-resolve` setting in MariaDB's option
files (`OWNER_NAME_RESOLVE`, with the file).

### Fail2ban for PostgreSQL (extra layer)

`auth_delay` stays the required gate. When Fail2ban is installed and runs,
CelikPanel also bans addresses that keep guessing. PostgreSQL's refused
sign-in carries no client address, so CelikPanel appends
`celikpanel-client=%h ` to `log_line_prefix` with `ALTER SYSTEM`
(`postgresql.auto.conf`, a reload, no restart), recorded as `log_prefix` and
reset with `ALTER SYSTEM RESET` with the cluster's last rule or when Fail2ban
is removed. It does so only over the two package defaults (`%m [%p] `,
PostgreSQL's; `%m [%p] %q%u@%d `, Debian's and Ubuntu's); any other prefix,
or one set with `ALTER SYSTEM` outside CelikPanel, is the owner's logging and
is left alone (the layer is then not used and the screen says why). It is
also not used while `log_hostname` is on, `logging_collector` writes files of
its own, or `lc_messages` is not English. The jail
(`celikpanel-postgresql-auth-<port>`, file
`/etc/fail2ban/jail.d/celikpanel-database-remote-postgresql-<port>.conf`,
filter `celikpanel-postgresql-auth`) reads the pg_ctlcluster log on
Debian/Ubuntu and the unit's journal elsewhere. The failregex is anchored on
the whole known line and takes only panel names and an address, so a line
forged into the log file by a crafted name cannot ban another address: the
names must match a pg_hba line to reach a password check at all, and a
startup name is cut at 63 bytes, too short for a whole forged line.

No leftovers: the jail file, the filter (when no CelikPanel PostgreSQL jail
remains) and the prefix go with the last rule or with Fail2ban. The same now
holds for the MariaDB jail and filter when Fail2ban is removed (its folders
stay behind with CelikPanel's files otherwise).

### Timed access (migration 075)

`database_remote_rules.expires_at` (UTC text, NULL = no end; additive; every
alpha.129 rule keeps no end, so nothing that is open closes by the update).
A customer chooses 1 day, 7 days or no end when adding an address. One day is
the default: an address listed for one session (often a home or mobile
address that changes hands) closes by itself if it is forgotten, and
extending is one click. A rule past its end is never applied; the sync loop
removes it within a minute, exactly like a manual removal (the filter, the
pg_hba line, the MariaDB account), with an audit entry
`database.remote.expire:<database>`; a rule given a new end meanwhile keeps
it. The list shows "until <date and time>" in the viewer's language and time
zone and reads itself again while something can change by itself. Expiry is
carried out by the panel: while the panel is stopped an ended address stays
allowed until it runs again (the engines still check the password).

### Closing while the server has not finished starting (D5)

The panel sends a host-changing request only once it knows the host's
identity, which the Agent reads only after systemd finished starting; a unit
hanging at start-up therefore refused every apply. Now, when the full apply
is refused, the panel sends `Agent.NarrowDatabaseRemoteFilterV1` in its own
`database_remote` job (step `narrow`): the Agent takes every address that is
no longer wanted out of the address filter (file, record, `nft -f`), never
adds an address or a port, never removes a port's drop rule, and runs no
engine client and no package or service manager — so the panel's policy
allows it without the host identity (effect "access narrowing", capability
`firewall`). The panel's last answer loses those rules, so the panel firewall
opens nothing for them. The screen says that the addresses are already
blocked and the rest (pg_hba line, MariaDB account) follows when the server
is ready; a failed apply is tried again every minute instead of every fifth.

### The panel firewall after `nft flush ruleset` (D6)

A guard in the running Agent checks every 15 seconds. When the panel
firewall's table is gone and a saved policy exists ("Save for reboot"), it
puts the table back from that policy by the same rules as the boot restore —
the database filter first, and the database ports left out of the restored
table until that filter is proven loaded. The panel's minute probe sees the
restore count change and syncs its live policy onto the table. The guard
never acts against the owner: when the boot restore unit
(`celikpanel-firewall-restore.service`) is disabled or masked, or
`nftables.service` is masked, it changes nothing and the administrator's
section shows why. A firewall that was only turned on live (no saved policy)
is not restored by the guard; that is the administrator's step, as before.

### The administrator's section on the Databases page

(Moved to Security in alpha.133; see "alpha.133: where the server matters
live" below. The paragraph keeps what alpha.130 built.)

The server switch was only inside a domain's database card. The Databases
page now has a section of its own with the switch and, per engine instance,
what is still missing before a listed address could work (Fail2ban, host
name lookup, `auth_delay`, an owner's pg_hba line, `allow_alter_system`,
nftables) with the step next to it. The switch is the stored offer
`admin_offered_permissions` that the Offered tab of plans and rights writes
too; `saveOfferedOptInPermissions` asks the sync loop for an apply whenever
the `database_remote` offer changes, from either screen.

### Panel start and release recovery (alpha.130 lab)

A forced failing 129→130 update went to release recovery; its first rollback
attempt was refused because the new panel's start-up apply held a
`database_remote` service-mutation lease (the panel's memory of the last
apply is empty after a restart), and a refused apply then sent the D5
narrowing as a second job. Services stayed down about 80 s instead of 25 s.
Now the panel's remote access loop never holds a lease around a start:

- At start it reads the Agent's status only. The Agent records, after every
  apply without an error, the digest of the wanted set
  (`dbremote.RequestDigest`: the rules and the administrator's agreement on
  MariaDB's host name lookup) and each instance's rules in force; when the
  digest is the panel's, nothing is sent. With no rule and nothing in the
  filter nothing is sent at all.
- For the first 3 minutes after start, and whenever the server is not quiet
  (a panel operation, an Agent mutation, the host lock, the package manager,
  an update just admitted), the loop takes no lease. What must close — an
  ended rule, a withdrawn permission, offer or suspension — closes at once at
  the address filter: `NarrowDatabaseRemoteFilterV1` now takes no lease and no
  durable job. It only removes addresses from CelikPanel's own nft table and
  record, under the remote access lock and the guard script's flock; a
  release update, rollback or package operation reads or writes none of that,
  and an interruption leaves the old or the new file (atomic replace), which
  the guard loads again. The panel firewall's port for a closed rule is
  closed at the next full apply (its sync is a leased job); the address
  filter already drops the address meanwhile.
- Opening, the engines' own rules and the panel firewall sync follow when the
  window is over and the server is quiet.
- The Agent's panel firewall guard (D6) takes the firewall's own lock, not
  the service-mutation lock, and acts only when the panel table is gone.

### Resilience contract (D-025)

Affected: native service ownership (owner settings detected, never
overwritten: MariaDB's lookup, PostgreSQL's prefix, the firewall's boot
restore), recovery (D5 closing without host identity; D6 restoring from the
saved policy only), persisted evidence (Agent record gains
`skip_name_resolve`, `log_prefix`, `pg_jail`; record version unchanged, so an
older Agent reading a record with them fails closed). Schema 74 → 75,
additive; going back is the pre-update snapshot. None of this is proven on a
real system yet; the lab steps are listed in the report of this work.

## alpha.131

Fix release for a defect the owner met on Boston (Ubuntu 24.04) on 8 Nov 2026,
and the owner's decision of the same day.

### A PostgreSQL cluster that changes port keeps CelikPanel's record

What happened: cluster 18/main ran on 5433 with remote access on (CelikPanel
had set `listen_addresses = '*'` and `auth_delay`). The owner moved it to
5432 with "Change port". The Agent's record (`/etc/celikpanel/database-remote.json`)
is keyed by port, so it still said `postgresql:5433`; on 5432 CelikPanel's own
settings looked like the owner's (`OWNER_LISTEN`, `NO_SLOWDOWN`), every rule
was refused, the filter kept `tcp dport 5433 drop` where nothing listened any
more, and the panel firewall opened nothing. It failed closed, but remote
access was dead and the stale entry never went away.

Now:

- The record of a PostgreSQL cluster also names its data directory
  (`data_dir`, record version 2). A version-1 record is read and migrated; its
  `applied` digest is dropped, so the first apply after the update runs in
  full. An older Agent refuses a version-2 record and changes nothing (fails
  closed, as for any unreadable record).
- The Agent's port change (`SetPostgreSQLClusterPortV1`), once the cluster is
  proved on its new port, moves the record and the cluster's filter port in
  the same step (the old port's rule disappears, the new port is filtered at
  once) and removes CelikPanel's Fail2ban jail of the old port. The panel then
  asks for a forced apply (`kickDatabaseRemoteSync`), which brings the pg_hba
  block and the panel firewall to the new port. "Update CelikPanel's records"
  (a port changed outside the panel) and a database switched to another
  cluster ask for the same apply.
- Every apply (and, read only, every status) first heals: a record whose port
  has lost its cluster moves to the port where that cluster runs now — found
  by its data directory, or, for a version-1 record, by CelikPanel's own
  settings in that cluster's `postgresql.auto.conf` (`listen_addresses = '*'`,
  `shared_preload_libraries = 'auth_delay'`) when exactly one running cluster
  carries them, no other record claims its port, and `pg_lsclusters` does not
  list a (stopped) cluster on the old port. Anything ambiguous stays closed.
  A record that is already right only gains its data directory: no setting
  changes and no restart.
- MariaDB: the panel offers no port change for MariaDB; an apply refuses an
  engine that runs on another port than the panel's record (unchanged).

Boston's own record was re-keyed by hand with the owner's authorisation on 8
Nov (18:49Z); the update leaves such a record as it is.

### Addresses coming and going never ask for a restart (owner, 8 Nov)

While the administrator's server switch (the offered `database_remote`
permission) is on, an engine CelikPanel prepared keeps its listen address,
TLS and `auth_delay` (MariaDB: CelikPanel's option file, with the agreement on
host name lookup) when its last address leaves or ends. The address filter
keeps dropping the port, the pg_hba block is empty and the MariaDB copies are
gone, so no address gets in, and the panel firewall keeps the port closed. The
next customer's address works at once, without a restart. Only turning the
switch off takes the settings back; the screen then says the engines stop
listening at their next restart. The panel sends the switch with every apply
(`ServerOffered`); it is part of the applied digest (`RequestDigestOffered`),
so a restarted panel never mistakes an apply made with the switch on for one
made with it off. An owner's own pg_hba network line still takes the listen
address back (review M2). An older panel sends no switch: alpha.130
behaviour.

### The customer's card

- Each address is one row: address and user, then its state and its end as
  ordinary text, then a compact end selector and the remove button; it
  stacks on a phone.
- A new end is saved only with "Apply" ("Discard" returns); picking an option
  changes nothing on the server.
- "Settings for your program" names the server's own host name (its full
  host name, else the panel certificate's name), never the mail name.
- A customer whose database cannot be opened because of an owner's setting
  reads a calm sentence without technical detail ("not available right now;
  your provider can set it up").
- The Databases list has no manual refresh button: every change on that page
  reads the list again by itself.

### Resilience contract (D-025)

Affected: persisted evidence (Agent record version 1 → 2, migrated on read,
an older Agent fails closed), native service ownership (CelikPanel's settings
follow the cluster rather than being taken for the owner's; nothing is taken
over without proof). No schema change. Recovery: going back to alpha.130
leaves a version-2 record unreadable for that Agent (remote access changes
nothing, the filter stays loaded by its guard) until the update is applied
again or the record is restored from the pre-update snapshot.

## alpha.133: where the server matters live

The left menu's Databases page is now only the list of databases each
person may see (one visibility rule: the administrator every database, a
reseller their customers', a customer their own). What belongs to the
server left it:

- **The server switch** "Customers may connect to their databases from
  outside" is on **Security › Open to the outside**
  (`/security#outside-connections`), administrators only. It is the same
  stored offer as before (`admin_offered_permissions`, `database_remote`,
  also written by the Offered tab), saved only with Save; the question
  before saving says what turning it on or off means, and turning it off
  lists that every address customers listed stops working. Under it each
  installed engine says "Ready · port · encrypted connections only" or
  "Not ready" with one guidance block per problem code (why, who acts, the
  next step, one button: Open Components, Apply again, or turn off host
  name lookup); the Agent's own sentence is only under a closed "Details".
  The card reads itself again every 30 seconds.
- **CelikPanel's own engine account** (the account the panel connects to
  MariaDB or PostgreSQL with, R-057) is on the **MariaDB and PostgreSQL
  component pages**, in the block "The account CelikPanel connects to this
  service with", for this machine's installed engine and administrators
  only. Its "More" menu has Show password (audited), New password… and
  Remove account… (the question lists what stops and what stays).
  Requests, the show-once password and the lost-answer handling are
  unchanged.
- A domain's database card tells the administrator that the switch is in
  Security › Open to the outside and its button leads there; it no longer
  turns the switch on itself. The "Allow an address" field is sized for an
  address.
- The MariaDB page no longer suggests `bind-address = 0.0.0.0`: listening
  is changed only by the apply described above, for listed addresses.

No endpoint, stored format or Agent behaviour changed for this move.

## Known limits (alpha.130)

- Restarting the engine after the first rule is the administrator's step in
  Components (no automatic restart); turning MariaDB's host name lookup off
  takes effect at that restart too.
- Turning MariaDB's host name lookup off needs the administrator's agreement
  (it changes how local programs connecting through 127.0.0.1 match);
  until then MariaDB rules wait.
- The PostgreSQL Fail2ban layer works only over a package-default
  `log_line_prefix`, without `logging_collector`, with English messages.
- An owner's own `shared_preload_libraries` in PostgreSQL is never rewritten;
  the administrator is told the exact value to set for `auth_delay`.
- The engines' units do not depend on the filter unit; the filter loads
  before the network at boot and the guard reloads it within 30 seconds.
- MariaDB IPv6 ranges are refused (single IPv6 addresses only).
- Withdrawing the plan permission or the administrator's offer closes access
  at the next apply, which follows the change after coalescing (up to about
  35 seconds), not within the request itself.
- Expiry is carried out by the panel (within a minute); while the panel is
  stopped an ended address stays allowed.
- While the server has not finished starting, a removal closes the address at
  the filter only; the pg_hba line or MariaDB account goes when the server is
  ready.
- After `nft flush ruleset`, a panel firewall that was turned on only live
  (no "Save for reboot") is not put back by the guard.
- The copy's grant comparison covers database-wide grants only: table,
  column or role grants an administrator adds to a remote copy by hand are
  not removed by it (only the engine's root can add them).
- MariaDB must log to the journal (no `log_error` file), otherwise rules
  wait.

## Evidence

alpha.130 unit tests (internal/dbremote a130_test.go, cmd/agent
database_remote_a130_test.go, cmd/panel database_remote_a130_test.go,
internal/db database_remote_migration_test.go): the MariaDB failregex against
refused sign-ins naming a host (the lab's `'_gateway'`, an ISP name,
`localhost`) — no ban — and naming an IPv4 or IPv6 address — that address;
the PostgreSQL failregex against both package-default prefixes from a file and
the journal, refusing other messages, host names, a forged marker in a
crafted name and an owner's prefix; the prefix marker only over package
defaults; MariaDB rules waiting while names are looked up, the agreement
writing `skip-name-resolve` into CelikPanel's file (and removing it with the
last rule), host-name accounts and an owner's setting leaving the lookup on;
Fail2ban files removed when Fail2ban is gone; the PostgreSQL layer added and
removed with nothing left; narrowing that never adds an address or a port and
runs no service manager; the firewall guard restoring without unproven
database ports and leaving a disabled boot restore or masked nftables alone;
durations, the one-day default, expiry removal with its audit entry, a new
end, closing at the filter when the apply is refused, the offer kick from
either screen, the administrator's view and the agreement; migration 75.


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

Lab round of 7 November 2026 (Debian 13 MariaDB and PostgreSQL, Arch
PostgreSQL; evidence `deploy/e2e/dns-pair-acceptance/evidence/a129/` on the
lab branch): a listed address connects with TLS; an unlisted address times
out at the filter; a non-TLS connection is refused; a password change
carries to the MariaDB copies; suspension and plan withdrawal close access
within 13 s (MariaDB) and 34 s (PostgreSQL); the last rule closes at once;
reboot keeps the filter; the guard restores the filter 10–14 s after
`nft flush ruleset`; Fail2ban banned an address after five wrong passwords;
`auth_delay` waits about 1 s per wrong password. The lab found and this
release fixes: the certificate root was 0750 (PostgreSQL could not read its
certificate; now 0755 and healed on every apply), the first failregex
matched no real journal line (Fail2ban strips the timestamp and keeps the
final newline), CelikPanel's earlier filter file was kept as the owner's
(now replaced), and an unconfirmed addition waited for the five-minute run
(now applied again soon). Not tried: Ubuntu 24.04 as a host, IPv6 rules.
