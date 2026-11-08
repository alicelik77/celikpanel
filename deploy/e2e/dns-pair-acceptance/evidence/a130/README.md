# alpha.130 acceptance round (lab only) — 2026-11-08

Base: published `v0.1.0-alpha.129` (`667dba6ba`, acceptance-licence dist), installed with the owner's `get.sh`
on cell `s0`: A = Debian 13 first server (PowerDNS primary, mail, MariaDB + PostgreSQL 17, plan "Basic",
customer `cust1` with MariaDB/PostgreSQL databases and two mailboxes), B = Ubuntu 24.04 joined server (BIND
secondary) with Boston's PostgreSQL history made through the panel (distribution 16 installed, removed with
data kept, PGDG lane, 18 installed → 18 on 5433, 16/main `down,binaries_missing`) and a stale hidden
`permission:database_remote` entry written by 129's Offered tab.
Candidates: `release/alpha130` `2739ae3e9` (first round, sequence 130, no lab bump) and the final head
`899b40b1f` as lab label 131 (`fc79ec750`, identity bump only; the cells were already on 130).
No real server, real CA, licence service or celikpanel.net was touched; every update was pressed through the
panel's update API from the lab portal.

## Results

| # | Item | Result | Notes / numbers | Logs |
|---|------|--------|-----------------|------|
| 1 | 129 → 130 (2739ae3e9) | PASS | B 75 s; A forced failure (unsupported panel.env key) rolled back, second press 70 s; schema 77; native files unchanged except two site files that gained the autoconfig blocks; sites 200. Recovery attempt 1 refused ("mutations not idle") → fixed in 899b40b1f | `C1upd-s0.log`, `C8rb-s0-main.log`, `C2-s0-backup.log` |
| 1b | 130 → final (lab 131) | PASS | forced failure first on both: first rollback attempt proceeded, panel down 31 s (B, no rules) / 29 s (A, MariaDB + PostgreSQL rule); second press 65 s each; schema 77; sites 200 | `R1upd-s0.log`, `R1rb-s0-*.log` |
| 2 | Remote DB | PASS | name lookup notice → agreement → restart → off; MariaDB ban after 5 wrong passwords; PG `celikpanel-client=%h` prefix + jail ban; last rules remove option file/jails/filters/prefix; expiry closed in 55 s + `database.remote.expire`; extend; `nft flush` → both tables back ≤5 s; restore disabled → left off + admin note; rule deleted right after start → gone at once; held "starting" boot: delete → 200 `closed_at_filter`, add → 503 | `K2remote-s0.log`, `R3a-s0.log` |
| 3 | Rights | PASS | Offered tab ↔ Databases switch one store (audit both); stale entry moved on B (`offering.stale_entries_moved`); TR grouping/order; programs read-only + Catalog link; Remote API warning; single Save with "Will be saved" list | `K3rights-s0.log`, `B3stale-s0.log`, `shots/` |
| 4 | Ops | PASS after fixes | Ubuntu Postfix status/Start; Debian 13 false "Postfix is not running" fixed (5889a5e37); panel TLS pass (Pebble); update card wording; TR dates; SSH turn off/on (B) after fix be12c8a42 | `B4postfix-s0.log`, `B4ssh-s0.log`, `K4ops-s0.log` |
| 5 | PostgreSQL | PASS (one open) | leftover 16 auto-removed + archived; with data kept + "Archive and remove"; port move; single version removal; final: 10 min after start `leftover.purged` libpq5/postgresql-16/client-16, no rc | `B5pg-s0.log`, `C5pg-s0.log`, `R2b-s0.log` |
| 6 | Leftovers | FAIL on 899b40b1f → PASS on 6fc1bbba1 | 899b40b1f restored postinst/CelikPanel files as "owner files" (`/etc/postgresql-common/*`, `/etc/postfix/*`) and left `/etc/dovecot/users`; retest below | `R2b-s0.log`, `RT-*.log` |
| 7 | Mail | PASS after fix | autoconfig/Autodiscover answers (no password, no LoginName); "Set up now" route (dd859604a); CNAME+SRV on PowerDNS and BIND secondary; certificate with and without the names; relay 587/465 with SASL, wrong password, SASL module missing → command, owner relayhost refused, off leaves nothing; subaddressing kept; sender block 550 / allow 250 / cleanup | `K7mail-s0.log`, `R3a-s0.log` |
| 8 | Recovery | PASS | confirm, request same answer, reset ends IMAP session, only that hash changes, single use, old link refused after a password change, suspended sends nothing, manual stop survives a password change (postfwd 550) | `K8recovery-s0.log` |
| 9 | Icons | PASS | teal/blue engine marks, no emoji; component icons distinct | `shots/` |

## Retest of the removals (fix/a130-pg-lab2 6fc1bbba1, lab label 132)

Fresh cells installed at 129 with `get.sh` and one-server setup (web + mail, BIND): `u1u` Ubuntu 24.04 and
`d1u` Debian 13, each with PostgreSQL 18 from the PGDG lane, an owner drop-in
`/etc/systemd/system/postfix.service.d/owner.conf` and an owner file `/etc/postfix/owner-notes.txt`.

| Step | Ubuntu 24.04 | Debian 13 |
|------|--------------|-----------|
| 129 → 132 pressed once | 84 s, schema 77, no failed unit | 80 s, schema 77, no failed unit |
| Whole PostgreSQL removal | no PostgreSQL/libpq package in dpkg (ii or rc); `/etc/postgresql`, `/etc/postgresql-common`, `/var/lib/postgresql`, `/var/log/postgresql` gone; PGDG source/key/pin gone | same |
| Postfix removal (with Rspamd + Roundcube confirmed) | `/etc/postfix` gone; only `owner-notes.txt` copied to `/var/backups/celikpanel/kept/postfix-<UTC>/etc/postfix/`; drop-in kept in place | same |
| Dovecot removal | packages, `/etc/dovecot` incl. `users`, quota-warning timer/service/script gone | same |
| Postfix reinstall | succeeds; drop-in still applied | succeeds |

Notes: on Ubuntu the restored drop-in folder became `0750 root:celikpanel` (was `0755 root:root`; Debian kept
it unchanged); the together-removal operation result does not repeat which files went to `kept/`.

## Fixes (fix/a130-lab, fix/a130-lab-b)

`be12c8a42` SSH Match "no" block / drop-in root:root · `dd859604a` mail autoconfig route · `9750ac175` MariaDB
name-lookup line with the next rule · `5889a5e37` Debian 13 Postfix monitoring.

## Open

- No screen creates a manual sending stop; the postfwd 550 text tells a manually stopped mailbox to change its password.
- During a held boot the customer's rule list read is 503, so only an address already on screen can be removed.
- Lab: the Debian 13 image's cloud-init hangs after a reboot (a129 D5); it was ended by hand.

## Not tried

Roundcube session end on reset; reseller-lacking permission warning; IPv6 rules; Arch.
