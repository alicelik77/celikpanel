# a128-other-systems: published v0.1.0-alpha.128 on Debian 12, Ubuntu 22.04 and Arch Linux

*Lab only: QEMU/KVM in WSL `archlinux`, labnet 192.0.3.x, a lab portal signed with a lab key, the
acceptance licence, the lab resolver. Every guest was installed through `get.sh` in a root terminal
as the owner types it; every update was pressed once through the panel's own update API. No real
server, celikpanel.net, licence service or real CA was contacted. Boston and Frankfurt were not
touched. 7 November 2026, 14:27–16:15 UTC.*

128's own lab covered Debian 13 and Ubuntu 24.04; this round covers the other three supported systems.

## Heads and dists

| Head | Role | Dist sha256 |
|---|---|---|
| `0c7aefe86` | v0.1.0-alpha.127 tag, the "before" | `097346b4…` |
| `818a03e36` | v0.1.0-alpha.128 tag (main `a21b55912` = this + the publication record) | `1044dfa6…` |
| `2ac7e5624` | lab 129 label of `fix/a129-lab` `4a841cfe4` (fix 1) | `df945002…` |
| `2b8114325` | lab 130 label of `fix/a129-lab` `74a7dda8e` (fixes 1 + 2) | `ab6604c0…` |

Guests: fresh 128 `d12f`/`u22f`/`arf` (.210–.212); 127 → 128 servers `d12v` (.224; the first one,
`d12u` .220, stopped at defect 3), `u22u` (.221), `aru` (.222). One server each, BIND, web + mail,
main site here. Content before the update: admin PHP site `site1` with box1, box2 and a MariaDB
database; plan Basic with customer `cust1` and its PHP site. PHP: Debian 12 8.2, Ubuntu 22.04 8.1,
Arch 8.5. MariaDB: 10.11, 10.6, 13.0.

## Results

| # | Case | Debian 12 | Ubuntu 22.04 | Arch |
|---|---|---|---|---|
| 1 | Fresh 128 through get.sh: terminal time, schema, `systemctl --failed` | PASS 73.5 s, 72, empty | PASS 67.1 s, 72, empty | PASS 115.1 s, 72, empty (install asks for a reboot: kernel upgraded by the prerequisites) |
| 2 | 127 → 128, Update pressed once | PASS 94 s, schema 65 → 72, 0 native-file differences (246 files), no failed unit, sites 200 | **FAIL** (defect 1): "panel could not be frozen", recovery put 127 back, a failed `celikpanel-self-update-*` unit stayed. With the fix (lab 129): 98 s, 65 → 72, failed mark cleared, no failed unit, sites 200 | PASS 74 s, 65 → 72, 0 differences (150 files), no failed unit; 128 → lab 130: 80 s, 0 differences (178) |
| 3 | Site settings: 384/120/64/3000/6 saved; 1024 MB, 600 s, 30 processes → 409 `SETTING_ABOVE_LIMIT`; `ini_set`/`set_time_limit`/`.user.ini` cannot raise; 140 s script stopped | PASS (PHP 8.2; 504 at 130 s) | PASS on lab 129 (PHP 8.1; 504 at 130 s) | PASS (PHP 8.5; 504 at 130 s) |
| 4 | Outgoing mail: postfwd from Components, limits on, 450 at the limit, pause 550, password change reopens, postfwd stopped → mail flows | **FAIL** (defect 4): postfwd 1.35-8 from Components and limits on (applied, verified, wired on 587/465), but limit 3 → 6 of 6 accepted, no pause; postfwd stopped → mail flows; after a postfwd restart a new limit gives 450 (below) | **FAIL** (defect 4), same with 1.35-7; after a restart: 450, pause 550 in 240 s, password change reopens only after ~4 min (below) | PASS: postfwd 2.03 (extra), drop-in `--cache=0 --keep_rates`; 4th–6th 450, paused in 60 s, 550, password → `sending_resumed` in 1 s, next two 250, postfwd stopped → 250 |
| 5 | Web: missing error page refused; custom 404 (missing .php); page cache MISS/HIT/BYPASS + clear; field-owned directive refused; nginx-refused directive put back (tree hash equal); `add_header` applied | PASS (cache folders 0755) | PASS on lab 129 | PASS |
| 6 | MariaDB tools: export + one-time download (410 after), import with safety copy, failing import rolled back, client command refused (sandbox), copy, check, repair | PASS | PASS on lab 129 | PASS; but the size/tables view read `known: false`, 0 tables (defect 2 cause; lab 130: known, 2 tables, 387 MB for the large database) |
| 7 | Extra SFTP/FTP login | PASS: keys-only sshd (cloud default) → `warnings: ["sftp_password_off"]`, Add refused `no_sign_in` and put back; with the lab's password drop-in: SFTP sees only `/files`, upload as uid 5002 (the site), `/etc` unreachable, shell refused, removed login refused | PASS on lab 129: keys-only notice; vsftpd from Components; FTPS: only the site's files, `cd /..` stays at `/`, upload as 5002, removed | PASS: `sftp_password_off` + `sftp_shell_refused`; without vsftpd Add refused `no_sign_in`; vsftpd from Components; FTPS confined; password SFTP refused (system sign-in rule), as the release notes say |

The keys-only screen: with `sftp_password_off` and vsftpd absent, `FTPLogins.tsx` shows the
"no login can sign in" notice with the Components link and hides Add (read from the code; the API
state above is what it reads). No screenshot was taken.

### Once (Arch)

| Item | Result |
|---|---|
| Log retention | PASS: cust1's subscription 7 days → removed `.1`, `.2.gz`, `.9.gz`, `-error.log-20261001.gz` (20 days old); kept `.3.gz` (2 days), the link `.4.gz → /etc/hostname`, `.bak`, the live logs and site1's rotated file. Screen: `keep_days 7, source subscription`, system rule `/etc/logrotate.d/nginx` found and unchanged; customer reads it (`may_change false`), customer POST 403, 4000 days 400 `LOG_RETENTION_INVALID` |
| Copy a large site | PASS with defect 2: source 2.4 GB (40 023 files: 40 000 small + 22 × 100 MB) + MariaDB 330 000 rows (387 MB); onto a site with content, overwrite confirmed: safety backup named (`full-…cpbak`), new database `tgt_ar_test_copy_c4b5` (330 000 rows), 40 023 files 2.4 GB, site answers, 695 s. The first request (no overwrite, default page counts as empty) copied too; the client gave up at 600 s and its database stayed behind (defect 2). |

### Debian 12 / Ubuntu 22.04 mail after a postfwd restart

`k4b`/`k4c` (`C4mail-*.log`), after the case-4 script had stopped and started postfwd once: box2
limit 2 → 3rd message 450 (hits `CP_MAILBOX_2`) on both; after `systemctl restart postfwd` the
same. Ubuntu 22.04: 4 refusals → paused in 240 s, `550 5.7.1 … stopped for security`; password
change answered `sending_resumed: true`, but the next two messages (seconds later) still got 550
(`CP_PAUSED`); 4 min later 250. Debian 12: its refusals fell into two clock hours (2 + 2), so the
pause (3 in one hour) was not reached before the run was stopped.

## Defects

1. **Ubuntu 22.04 cannot update (fixed, `fix/a129-lab` `4a841cfe4`).** `update.sh` freezes the
   panel with `systemctl kill --kill-whom=all --signal=SIGSTOP`; systemd 249 says
   `unrecognized option '--kill-whom=main'` (rc 1), so every update stops at "panel could not be
   frozen" and is put back. The same spelling is in `rollback.sh`, `finalize-pending-rollback.sh` and
   the traffic logrotate file (there behind `|| true`: nginx never reopened its traffic logs after a
   rotation on Ubuntu 22.04). Fix: `--kill-who=` (known to 249, 252, 262); new CI contract
   `deploy/test-systemd-option-portability-contract.sh`. Verified: lab 129 127 → 129 on `u22u`, 98 s.
   Evidence: `raw/C2upd-u22-fix129.log`, `raw/u22upd-diag.txt`.
2. **Arch: copy and install cleanup refused, orphan databases (fixed, `74a7dda8e`).** The Agent ran
   `mysql --batch …` and read stdout+stderr together; MariaDB 13 prints `mysql: Deprecated program
   name …` first, so the exact ownership proof never matched: a refused site copy answered "nothing
   was copied" and left its new database and user in MariaDB while the panel dropped its records
   (reproduced at once: `tgt_ar_test_copy_76c5`), and the database tools read `known: false`. Fix:
   run `mariadb` when present. Verified on lab 130: the same refused copy left nothing; tools view
   correct. Open: `siteCopyRemoveDatabase` drops the panel's records even when the Agent's delete fails.
3. **Debian 12 setup: BIND stopped by a timed-out ownership proof (open).** First Debian 12 server
   (three guests installing at once): at 14:50:52 the BIND publication's vendor proof ran
   `dpkg-query -S` for `/usr/lib/…/named.service` (not found on Debian 12, expected) and then
   `/lib/…`, which hit the 15 s proof deadline (`context deadline exceeded`); the restore reload
   failed the same way and "explicit empty recovery" **stopped named**. Setup waits in verification
   for ever. The screens loop: Setup says "open DNS and peer servers", the DNS page says "server setup
   is still running; open Server setup", Components → BIND answers 409
   `DNS_ENGINE_WORKFLOW_REQUIRED` (go to the DNS page). A timeout is an unknown result, not a verified
   failure (D-024), and it took DNS down. Not reproduced on the second Debian 12 server (less load).
   Evidence: `raw/setup127-d12-attempt1.log`, `raw/D1-bind-d12.log`, `raw/D1-agent-journal-d12.txt`.
4. **Debian 12 / Ubuntu 22.04: a mailbox limit set after "limits on" is not applied (open).** postfwd
   1.35-8 / 1.35-7: turning the limits on reloads postfwd; that first reload crashes its policy server
   (`Use of uninitialized value $send … line 349`, "server-daemon check failed – respawning"). After
   the respawn, the next reload (box1 limit 3) logs "reloading ruleset on next request" but the
   policy children keep the old rules: box1 kept hitting `CP_MAILBOX_50` (its new-mailbox limit), 6 of 6
   accepted, no refusal, no pause. The self-test passes because its rule is in the first ruleset.
   After `systemctl restart postfwd` a changed limit applies at once. Related on Ubuntu 22.04: a
   lifted pause still answered 550 for a few minutes after `sending_resumed` (128 lab round 1 saw
   the same on Debian 13 before `--cache=0`; the drop-in has `--cache=0` here). Likely fix
   direction: restart postfwd 1.x instead of reloading it, or prove the running policy holds the new
   rule (the self-test checks only its own first-ruleset rule). Evidence: `raw/C4mail-d12.log`,
   `raw/C4mail-u22.log`, `raw/pfd-d12.txt`.

## Observations

- Arch: the fresh install says to reboot (prerequisites upgraded the kernel; nftables/WireGuard
  unavailable until then); the lab rebooted the 127 server before setup.
- Ubuntu 22.04: after the failed update (127 agent restarted by recovery) the Postfix `.db` maps were
  re-hashed from unchanged sources (127 behaviour, fixed in 128 as A7).
- A site whose web folder has only the default page counts as empty: a copy without "overwrite"
  replaces it without a safety backup (as designed; worth saying on the screen).
- Lab harness: one get.sh run refused the typed password (terminal timing); the retry installed.

## Not tried

PostgreSQL tools on these systems; the security pause by a real traffic spike (the refusal pause was
used, as in the a128 lab); log retention and the large copy on Debian 12/Ubuntu 22.04; a screenshot of
the keys-only notice; Ubuntu 22.04 cases 3–7 on the published 128 (they ran on lab 129 = 128 + fix 1,
because 128 cannot be reached by update there).

## Lab state at the end

All seven guests stopped and their disks and state removed; labnet stopped (kept); lab portal down;
the four dists built for this round and `/var/tmp/a129o` removed; no qemu process left. Kit:
`raw/kit/` (scripts as run; paths point at the scratchpad of this session).

## Fix branch

`fix/a129-lab` from `origin/main` (`99b94bd01`), pushed:

- `4a841cfe4` Ubuntu 22.04: update freezes services with `--kill-who` (systemd 249) — tests: new
  portability contract, update quiesce/EXIT contract, finalize-pending-rollback contract, bootstrap
  update contract, `go test ./cmd/agent -run Traffic`, recovery packages, `cmd/panel -run
  Recovery|Pending|WAL`, `cmd/agent -run Update` all pass.
- `74a7dda8e` MariaDB statements run with the `mariadb` client — tests: 3 new
  (`database_rpc_client_test.go`); `go test ./cmd/agent` only the 19 known guest-only DNS/mail failures.

## Follow-up (fix/a129-lab, after this round)

- D4 proof on Debian 12's postfwd 1.35-8 (plain lab guest, policy requests on 127.0.0.1:10040;
  `raw/D4-pf135-old.log`, `raw/D4-pf135-new.log`, `raw/kit/g_pf135.sh`): with the 128 drop-in a
  restart lost the counters (8 counted, limit 5: next request DUNNO); with `--save_rates` in a folder
  owned by `$RUNAS` the restart took 0.3 s, the counters came back (next request 450 "limit 5"), and
  after a changed rule, a restart and `--delrate` the new limit 3 counted from zero. A plain reload on
  an idle daemon did reach the rules here; the lab failure followed the reload that made the master
  respawn the policy server. Fixes: D4 `03949d7ca` (restart postfwd 1.x), D2 rest `ff5692016` (copy
  database records kept unless removal is proven), D3 (unknown check never stops BIND; DNS page and
  Setup name a stopped engine) on `fix/a129-lab`.
- D1 history: `--kill-whom` has been in `update.sh`/`rollback.sh` since v0.1.0-alpha.1; Ubuntu 22.04
  could first be installed with alpha.123, so every update of an Ubuntu 22.04 server (123 → 124 …
  127 → 128) stopped at "panel could not be frozen", and an automatic rollback there would also have
  failed. The failed 127 → 128 update left the server fully on 127 and healthy (version and schema
  65, Agent matching, services active, sites 200, recovery "stopped before installed bytes changed");
  leftovers: one failed `celikpanel-self-update-*` unit (cleared by the next successful update) and
  Postfix `.db` maps re-hashed from unchanged sources by the 127 Agent's restart.
