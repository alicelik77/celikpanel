# alpha.129 acceptance round (lab only) — 2026-11-07

Candidate: `release/alpha129` head `ea363da4f` (sequence 129, migrations 073–074).
Lab: QEMU/KVM guests in WSL `archlinux`, lab portal/CA/Git, labnet 192.0.3.1 as
the listed remote client, the lab host 192.0.3.2 as the unlisted one. No real
server, real CA, licence service or celikpanel.net was touched. Every update
was pressed once through the panel's update API; installs ran the owner's
typed `get.sh` command.

Lab labels: 130 = the candidate with a version bump only (`ac97b0a04`), used
for the forced-failure case; 131 = `fix/a129-lab2` at `3616cec89`
(`715d797924`); 132 = stale bundle (131 code); 133 = `fix/a129-lab2` at `40a021576`
(`354eb785d`) (see `raw/out-build13*.txt`).

## Results

| # | Case | Result | Numbers / notes | Logs |
|---|------|--------|-----------------|------|
| 1 | Pair at 128 (main Debian 13 PowerDNS+mail, joined Ubuntu 24.04 BIND) → 129, pressed once each | PASS | joined 79 s, main 67 s; schema 72→74; native files of sites/mail unchanged (0 differences); `systemctl --failed` empty; sites answer | `C1upd-r9.log`, `C2-r9-*.log` |
| 2a | Ubuntu 22.04: fresh 127 → 129 directly | PASS | 84 s; schema 65→74; 0 native-file differences of 323; no failed unit | `setup127-u22.log`, `C2upd-u22.log` |
| 2b | Ubuntu 22.04: fresh 129 install (owner's get.sh) | PASS | install 44–45 s (two installs); schema 74; no failed unit | `C1fresh-u22*.log` |
| 2c | Ubuntu 22.04: forced failure 129 → lab 130 (unsupported panel.env key) | PASS | installer stopped at prepare 20:16:51; release recovery: verified snapshot → services stopped → "Rollback complete" 20:17:14 (23 s), panel/agent active at 129; line removed, second press 66 s succeeded, failed mark cleared | `C8rb-u22f-main.log`, `C8rb-u22f-killwho.log`, `C8rb-u22u-main-run1.log` |
| 3a | DB users, MariaDB (main) | PASS | old password refused after change; weak password refused; read-only user reads a table made later by the read-write user, INSERT/CREATE denied; removal leaves the database; `DATABASE_NAME_TAKEN` for database and user made outside | `C3dbu-r9-mysql.log` |
| 3b | DB users, PostgreSQL (main) | PASS | same checks; `permission denied for table/schema` for the read-only user | `C3dbu-r9-postgresql.log` |
| 3c | WordPress from the catalogue gets the new password | PASS (note) | first refused with `APP_EXTENSIONS_MISSING` (php8.4-mysql) → PHP-FPM 8.4 installed in Components; after the change wp-config holds the new password, PHP connects; site answered 500 for the first second after the change, 302 (setup redirect) after | `C3wp-r9.log` |
| 4a | Remote access, Debian 13 MariaDB | PASS after fixes | rule waits for restart/jail; listed address TLS (TLS_AES_256_GCM_SHA384); plain refused; unlisted times out (8 s); password change takes effect; suspension closes in ≤13 s; plan permission withdrawn closes in ≤13 s (once systemd was ready, see D5); last rule closes at once; reboot keeps the filter; Fail2ban banned 192.0.3.2 after 5 wrong passwords only with lab 133 (D2, D3) | `C4remote-r9-mysql*.log`, `C4ban-r9.log` |
| 4b | Remote access, Debian 13 PostgreSQL | PASS | rule waiting until restart; TLSv1.3; plain refused (`no pg_hba.conf entry … no encryption`); wrong passwords ~1.05 s each (auth_delay); old password refused; plan withdrawal closes in ≤34 s; last rule closes in ≤8 s | `C4remote-r9-postgresql.log` |
| 4c | Remote access, Arch PostgreSQL (`ssl=off` default → TLS on) | PASS after fixes | 129: PostgreSQL could not read its certificate (root folder 0750) and did not start after the restart (D1); lab 133: folder healed to 0755 by the apply, restart from Components starts it, TLSv1.3 connects | `C4remote-aru-postgresql.log` |
| 4d | `nft flush ruleset` | PARTIAL | the guard restored the filter after 10–14 s (≤30 s); the port was reachable by TCP meanwhile (4 probes), but the engines refused the unlisted address (MariaDB 1130 host not allowed, PostgreSQL no pg_hba entry); the panel firewall table `celikpanel_fw` stayed absent until restored by hand (D6) | `C4remote-r9-mysql.log`, `C4flush2-r9.log` |
| 5 | Reopened mailbox paused again in the same hour (joined pair, postfwd 2.x) | PASS | paused again after 240 s, detail "since the password change" | `C5mail-r9.log` |
| 6 | Token deletion while DNS is pending | PASS (note) | create → 409 `DNS_PUBLICATION_PENDING`; delete with the clean-up waiting → 202 naming status_url; second press 409, no new work; named resumed → gone after 31 s; the status read meanwhile says "stopped … delete again" (D7) | `C6tok-r9*.log` |

## Defects

Fixed on `fix/a129-lab2` (pushed; one commit each, with tests):

- D1 `f57b12a80` — the certificate root `/etc/ssl/celikpanel-database` was made 0750 under the Agent's umask; PostgreSQL (Arch) could not reach its certificate.
- D4 `40a021576` — on a server already broken by D1, no apply reached the certificate step; every apply now heals an existing root first.
- D2 `3616cec89` — the MariaDB failregex matched no real journal line (Fail2ban 1.1 keeps the space and final newline); new regex matched 9 of 11 real lines (old: 0).
- D3 `69576572f` — the Agent kept the first filter as "the owner's", so the corrected regex never reached an updated server; CelikPanel's earlier filter is now replaced (owner edits stay).
- D8 `7bb6dcdb9` — a rule addition whose apply was not confirmed (502 `DATABASE_REMOTE_AGENT`, cause not logged) waited for the five-minute run; the add path now asks the loop to apply again, like removal.

Verified in the lab with label 133 (`40a021576`): D1+D4 on Arch, D2+D3 on Debian 13. D8 has unit tests only.

Open (report only):

- D5 After a reboot the Debian 13 lab image's cloud-init config stage hung, systemd stayed "starting", and the Agent refused every apply ("systemd is not ready"). A withdrawn plan permission therefore did not close access for 90 s+ while the rule showed `not_in_plan`; the screen does not say the closure failed.
- D6 `nft flush ruleset` removes the panel firewall table; nothing restores it until boot or the restore unit is run (database-remote apply then reports "live nft table is absent").
- D7 Deletion status during a waiting DNS clean-up says "stopped before it finished … delete again" (`domainDeletionIncompleteStatus`), while the 202 correctly says it finishes by itself.
- MariaDB logs `'_gateway'` for the labnet client (skip_name_resolve off); Fail2ban cannot ban a name — design decision.
- Lab harness: `getsh_install.py` prompt flake (2 of ~10 installs); one WSL hang under memory pressure.

## Not tried

- Ubuntu 24.04 joined server as a remote-access host; IPv6 rules; MariaDB copies after a user deletion.
- A real external Fail2ban ban from a second listed address of the same user.
- The 132 label was built from a stale source bundle (same code as 131); its runs are kept but count for nothing.
- Full `go test ./cmd/agent` was not rerun after the last fixes in this round (targeted packages and tests only).

## Files

`raw/` holds every log of the round (secrets scrubbed: known passwords,
tokens, cookies, FTPPassword fields), the release manifests and build outputs
of the lab labels, and `raw/kit/` the scripts that ran (lab only).
