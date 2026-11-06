# a127: alpha.127 lab acceptance round (PR #302, `release/alpha127`)

*Lab only: QEMU/KVM in WSL `archlinux`, labnet 192.0.3.x, a lab portal signed with a lab key, the
acceptance licence, the lab ACME CA (pebble) and the lab resolver. Every guest was installed through
`get.sh` in a root terminal; every update was pressed through the panel's own update API. No real
server, celikpanel.net, licence service or real CA was contacted. Boston and Frankfurt were not touched.*

## Heads and dists

| Head | Role | Dist sha256 |
|---|---|---|
| `14a90360c` | published v0.1.0-alpha.126 tag (lab copy), the pair's "before" | `b26bd0c1…` |
| `4ee915bb8` | alpha.127 candidate as given (cases 1, 2, 4–8 and the main of 3/9) | `d49053a0…` |
| `3ac4745f2` | lab 128 label of `4ee915bb8` (case 10 target) | `46d885c4…` |
| `111b45f28` | lab 128 label of `20957e440` (restore fix, first) | `606c5c7b…` |
| `50910894e` | lab 129 label of `32124ee20` (restore in one transaction) | `4694a98c…` |
| `b25b1b717` | lab 130 label of `e129b5d8a` (egress guard fix) | `1d1c9f8e…` |

Topology: `r7` main Debian 13 (PowerDNS main, mail, `host1.r7.test`, 192.0.3.190) + joined Ubuntu
24.04 (BIND backup, `host2.r7.test`, 192.0.3.191), installed at 126. `f7`: fresh Debian 13 at 127
(192.0.3.194), cases 1 and 10.

## Results

| # | Case | Result |
|---|---|---|
| 1 | get.sh fresh Debian 13: weak password refused up front (file and prompt), strong accepted | PASS |
| 2 | Pair 126 → 127, Update pressed once each: joined 75 s, main 72 s; schema 64 → 65 | PASS |
| 3 | PostgreSQL like Boston: 126 INTERNAL reproduced; 127 creates on 18/5433; phpPgAdmin "PostgreSQL 18 (port 5433)" | PASS after fix (restore failed on the candidate) |
| 4 | Domain page reads on the joined server after warm-up (all < 0.21 s) | PASS |
| 5 | Password fields EN+TR (mailbox, database, Access); API PASSWORD_WEAK; refusal said once | PASS |
| 6 | Customer provider wording; admin "Install the tools" in confirm + progress; SSH option without reload; webmail card per role | PASS |
| 7 | Real steps in the locked window (PHP, backup create/restore, DNS record, service restart, SSL via lab ACME); tour waits; repeated request once | PASS (observations) |
| 8 | Services list; nginx stopped → alert after 5 min → email delivered to the lab mailbox; site checks with certificate days | PASS |
| 9 | Node.js: 126 site not rewritten; admin move (confirm + progress); new site sandboxed; port not reachable from the other guest; escape kit | PASS after fix (guard failed unit); Ubuntu 24.04 items 10/20 fail as the named `bpf_missing` gap |
| 10 | Forced failed update (127 → lab 128, unsupported panel.env key) rolls back by itself; next press succeeds in 72 s; `systemctl --failed` empty | PASS |

## Defects found and fixed on `release/alpha127`

1. **PostgreSQL restore of a non-empty database always failed** (`relation … already exists`; the
   safety rollback failed the same way; 500 INTERNAL). Pre-existing, not new in 127.
   - `20957e440`: the restore session becomes the database owner and drops the owner's objects
     before the dump (`DROP OWNED BY CURRENT_USER`; superuser-owned databases untouched).
   - Lab check of `20957e440` (lab 128): a restore of the 127 backup worked; a second restore with a
     table owned by another role dropped the owner's table and the rollback failed (data loss).
   - `32124ee20`: `--single-transaction`. Lab 129: with the other role's table present the restore
     is refused and nothing changes; without it the restore commits; the 127 backup restores.
2. **Egress guard failed every 30 s after "Install packages and build"** (`celikappinst-N` stays
   "active (exited)" with an empty cgroup; 121 failures in an hour; `systemctl --failed` showed it).
   - `e129b5d8a`: the guard skips a unit with no cgroup. Lab 130: after the next sandbox write the
     guard file carries the fix and `systemctl --failed` is empty. While a step runs it is in the
     SMTP set (observed).

Go: `cmd/agent` and `internal/...` pass except three vendor-lane tests that also fail on the
unmodified candidate on the Arch lab host (no Debian codename in `/etc/os-release`).

## Observations (not fixed)

- PHP change while nginx is stopped: the pool and the database move to the new version, the vhost
  is rolled back, the answer is 500 INTERNAL; after nginx starts the site answers 502 until another
  vhost write (here the certificate issue). Re-pressing the same version is a no-op. Pre-existing
  (`domain_php_handlers.go`, 23 Jul comment).
- A refused PostgreSQL restore (another role's table) still answers 500 INTERNAL, not a typed reason.
- Service restart: steps are shown, then the window closes by itself (by design: no end message).
- The guard's fixed text reaches a server at its next sandbox write, not at the update.
- Ubuntu 24.04 is "reduced" (`bpf_missing`): escape items 10 and 20 bind other ports, as named.
- Rollback snapshot is named `from-unknown-to-<commit>`.
- get.sh/prompt TR wording mixes "Şifre" (rule) and "parola" (prompt).
