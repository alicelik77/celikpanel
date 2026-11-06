# a126: alpha.126 pre-release lab round (PR #300, `release/alpha126`)

*Lab only: QEMU/KVM in WSL `archlinux`, 192.0.3.x, a lab portal signed with a lab key, the
acceptance licence. Every guest was installed through `get.sh` in a root terminal, as the owner
installs. Every update was pressed through the panel's own update API, as the owner's Update button
does. No real server, celikpanel.net, licence service or real CA was contacted.*

**Topology (mirrors the owner's servers).**
- `p6` main: Debian 13, PowerDNS main, V3 native catalog, mail stack (`frankfurt.p6.test`).
- `p6` backup: Ubuntu 24.04, BIND backup DNS, joined by code, static sites whose zones are
  published on the main (`boston.p6.test`).
- `p7`: the same pair again, for the other update order (main first).
- Both pairs were installed at a lab copy of the published alpha.125 tag (`ed6086469`) and updated
  to the candidate.

**Heads and dists.**

| Head | Role | Dist sha256 |
|---|---|---|
| `ed6086469` | published v0.1.0-alpha.125 tag, lab copy | `21ff0c46…` |
| `7c8453e98` | alpha.126 candidate tested (PR #300 head when the round started) | `6a735749…` |
| `7c150dc49` | lab-only alpha.127 label of `7c8453e98` (release identity bump only), the update target for case 6 | `c452d237…` |

The release branch moved to `8f540f003` during the round. That commit only adds schema 64 to the
pinned list in `deploy/e2e/release-recovery/test_owner_update_trial.py` (the CI failure of
`7c8453e98`). It changes no shipped file, so the dist under test is the same product.

**Lab loads, named where used.**
- `hostnamectl` and `/etc/hosts` set to the owner's machine names before install.
- One busy shell loop (`systemd-run --unit=lab-busy -p RuntimeMaxSec=120`) for the CPU chart. The
  unit ends "failed" on its runtime limit, so it was reset with `reset-failed` before any
  failed-unit check.
- `systemctl stop dovecot` / `start dovecot` outside the panel, for "Problems now".
- An unsupported key in `panel.env` (`CELIKPANEL_LAB_ROLLBACK_PROBE=1`), to force the installer
  to refuse after the services stopped, so the automatic rollback runs.

**Lab environment incidents (not product).**
- During p6 setup, the lab resolver (unbound on labnet) cached SERVFAIL for `p6.test` from before
  the main's PowerDNS answered. Both setups waited at `access_dns` with the typed guidance ("Public
  DNS must resolve frankfurt.p6.test …"). Re-running the lab delegation flushed the cache, and both
  setups finished by themselves. The p7 run does the same flush automatically (`runB.sh`).
- Script errors of the kit (CELL default, a missing `upd.sh`) made two early runs do nothing. Their
  outputs are kept as `run-c1run-aborted-no-updsh.txt`. No server state was changed by them.

## Results

| # | Case | Result |
|---|---|---|
| 1 | Owner's incident on 125, then joined → main updates: stuck deletion finishes by itself | **PASS** |
| 1b | Other order (main first, then joined) | **PASS** |
| 2 | Fresh delete on 126: own dialog, live steps, locked page, repeated DELETE starts nothing | **PASS** |
| 3 | No native browser dialogs in the served bundle | **PASS** |
| 4 | Plans: Add plan hidden while the form is open; only choosable software; 125 plans still work | **PASS** |
| 5 | Monitoring: CPU matches a known load; ranges ≤ 360 points; problems strip | **PASS** |
| 6 | Recovery watchdog idle cost; completed rollback under the new runner; next Update works | **PASS** |
| 7 | Update 125 → 126, first press, both servers | **PASS** |

### 1. The owner's incident (`C1pre-p6.log`, `C1run-p6.log`)

**Lab BIND opens two rndc control sockets per loopback address**, as Boston's does: named 9.18.39
(Ubuntu 24.04, 2 vCPUs) listens on `127.0.0.1:953` with fds 48 and 49, and on `[::1]:953` with
fds 50 and 51.

**On 125 (reproduced).**
- `inc6.test`, a static site on the joined server, was created at 12:44; the main and the backup
  both served its SOA.
- At 12:49:44 Delete was pressed once: `202 deletion_pending, dns_cleanup,
  remote_dns_removal_confirming`. The zone was gone from both DNS servers at once (REFUSED).
- The main's proof stopped at `dns_peer_inspection_unknown:listeners_unverified` on every re-check
  (18 in 7 minutes), and the domain stayed "being deleted". `peer_challenge_context_mismatch` did
  not occur in this run; it depends on PowerDNS re-stamping its catalog between attempts.
- The Users page traffic check on the joined server reported `unknown: {"vhost_kept": 1}`, the
  owner's "1 website(s) are not counted".
- The empty `/etc/nginx/celikpanel-sites.d/inc6.test` stayed behind.

**Joined server updated first (Update pressed once at 13:00:31, succeeded in 68 s).**
- At its start, the Agent replaced the check tool `/usr/local/libexec/celikpanel-bind-peer-inspect`:
  `87732bf8e050…` (125, one of the shipped digests) → `8f821e7680d7…`, taken from the 126 release
  tree. The old program is kept as
  `/var/lib/celikpanel-agent-private/dns-check-tool/bind-peer-inspect-87732bf8…`, and the record is
  `dns-check-tool-refresh-v1.json`.
- The main's proof, still on 125, was `verified` at 13:01:45. The joined server logged "the main DNS
  server confirmed the removal … finished by itself" at 13:01:55, **about 32 s after its Agent
  restarted and before the main was updated.** Nobody pressed anything.
- Domain absent from both servers' lists; zone REFUSED on both.

**Main updated second (Update pressed once at 13:02:04, succeeded in 63 s).**
- Nothing was left to finish.

**After both updates.**
- Users page traffic status on the joined server: `unknown: {}`, `unknown_sites: []`, so no
  warning.
- The empty `celikpanel-sites.d/inc6.test` left by the 125 deletion **stays** (no sweep, as the
  release notes say). Directories of deletions made on 126 are removed (case 2).
- The fresh-install copy `/opt/celikpanel/dns-owner-tools/bind-peer-inspect` keeps the 125 digest.
  The installed tool is the one the forced command runs.

### 1b. Other order: main first (`C1pre-p7.log`, `C1run-p7.log`, pair `p7`)

**On 125.**
- `inc7.test` was deleted on the joined server at 13:51:55. It stayed pending.
- This time **both** of the owner's errors appeared on the main:
  - `peer_challenge_context_mismatch` (6; status `dns_peer_journal_unknown`);
  - `listeners_unverified` (6).

**Main updated first** (pressed 13:56:12, succeeded in 72 s, `systemctl --failed` empty).
- The context mismatch was gone (0 after the update).
- The proof still stopped at `listeners_unverified` (7 in 7 minutes), because the joined server
  still had the 125 check tool.
- While the main restarted, the joined server said `remote_dns_main_unreachable` ("Nobody needs to
  do anything now …"). It said `remote_dns_removal_confirming` again at its next re-check
  (14:00:51, 193 s later).

**Joined server updated second** (pressed 14:04:45, succeeded in 67 s, `systemctl --failed`
empty).
- The Agent put the 126 check tool in place at 14:05:36 (`87732bf8e050` → `8f821e7680d7`).
- **The deletion finished by itself at 14:06:16**: 11 s after the update answered and 40 s after
  the Agent started.
- Users page: `unknown: {}`, `unknown_sites: []`.

**Conclusion.** In both orders the stuck deletion ends by itself within about 40 s of the joined
server's update; the main's update alone does not end it. The release notes say "Update the joined
server first", which matches. The notes' "finishes within about five minutes after the main's update" is conservative: in the lab, the main still on 125 accepted the new tool's answer, and the deletion ended before the main was updated (case 1).

### 2. Fresh delete on 126 (`C2-p6.log`, `shots/c2-*`)

These deletions ran on the joined server through the Domains page in headless Chromium, as the owner
does it, once in EN (`delen6.test`) and once in TR (`deltr6.test`).

- **The panel's own question.** "Delete delen6.test? Its website files, mail and DNS records are
  removed from this server. This cannot be undone. Cancel / Delete" (TR: "deltr6.test silinsin mi?
  … İptal / Sil"). The browser saw **no native dialog**.
- **Delete pressed three times** in 120 ms (two clicks, then a third): the browser sent **one**
  `DELETE`.
- **Progress window, live steps** (observed DOM states):
  - 4.5 s: request running;
  - 6.0 s: mail settings done, website done, DNS running;
  - 15.2 s: DNS waiting, with "The main DNS server is confirming the removal with the backup DNS
    server.";
  - 36.3 s: certificates done, finishing done, **Completed** ("Domain delen6.test deleted").
  - TR: the same timings, ending "Tamamlandı".
- **Page locked.** A full-screen modal scrim (`aria-modal`, `aria-busy`) without any close action
  while the operation runs.
- **List refreshed by itself.** The row disappeared after OK with 0 page reloads.
- The site's empty `celikpanel-sites.d/<domain>` directory was removed (new on 126).
- **Repeated DELETE through the API** (`api6.test`):
  - The first and second requests were 0.3 s apart. The second answered at once:
    `202 deletion_already_running` "This domain is already being deleted. Nobody needs to do
    anything…".
  - The first answered `remote_dns_removal_confirming`.
  - A third request, after the first had answered, also got `deletion_already_running`.
  - The journal shows **one** run: one `userdel`, one pending line. The deletion ended in 22 s.

### 3. No native dialogs (`C2-p6.log`)

- The installed web root (`/opt/celikpanel/web`, 347 JS files) was checked against what the panel
  serves: the index and all 5 scripts it loads are identical.
- Searched for `window|globalThis|self|top|parent.confirm|alert|prompt` and bare
  `confirm(` / `alert(` / `prompt(` calls: **0 matches**.
- The panel's own confirm window and progress window are present.

### 4. Plans (`C1pre-p6.log`, `C4-p6.log`, `C4ui-p6.log`, `shots/c4-plans*`)

**On 125.**
- The reseller's software catalog had 23 items, including nginx, pdns, postfix, dovecot, certbot,
  cron, nftables, rspamd and roundcube.
- Plan `all-software-125` was saved as "all software".
- Plan `all-but-one-125` (everything but PHP) was saved with **14 software** rows, nine of them
  server parts. This is the owner's "14 software".

**On 126.**
- The catalog has 14 items: PHP 8.4, MariaDB, three CAs, eight applications and own programs.
- `all-but-one-125` reads **5 software**: wordpress, mariadb and the three CAs. Its 14 database
  rows are kept as they were.
- `all-software-125` is still "Everything offered". A customer created on it got the 14 choosable
  items and created `plan6.test` with PHP 8.4.

**Reseller's Service plans tab (EN, TR).**
- Add plan is visible before, and **hidden while the form is open**.
- The row edit buttons are **disabled** while the form is open (EN probe).
- The form's software section lists only PHP, MariaDB, free certificates (Google Trust Services,
  Let's Encrypt, ZeroSSL), applications and own programs.

### 5. Monitoring (`C5-p6-main.log`)

**CPU against a known load.**
- One busy core ran from 13:09:47 to 13:11:47 on 2 vCPUs (`vcpus: 2` in the answer).
- The 30-second points read 50.5 / 50.5 / 50.4 %, against an expected 50 %.
- The edges read 26.6 % and 20.4 % (partly covered windows).
- Idle before and after: 0.5 %, with peaks of 1.5–2.5 %.
- `cpu_peak` shows the short peaks.

**Ranges.**

| Range | Points | Step |
|---|---|---|
| 1h | 95 | 30 s |
| 6h | 46 | 60 s |
| 24h | 12 | 240 s |
| 7d | 3 | 1800 s |
| 30d | 1 | 7200 s |

- Every range is at most 360 points. By the step, 7d and 30d stay ≤ 336 and ≤ 360 points.
- Only about 45 minutes of history existed.
- One 30 s sample (13:06:30) is missing from the 1h series.

**Problems now.**
- Baseline: none (mail queue and security updates "checking").
- `dovecot` stopped outside the panel, then Rescan: `service_stopped`, critical, actor
  administrator, action `start_service` → `/components/dovecot`.
- Started again, then Rescan: the problem cleared. The remaining item is the real `security_updates`
  warning (13 pending).

### 6. Recovery watchdog (`C6-p6-main.log`, `run-c6rb.txt`)

**Idle cost per run.** CPU per run is `CPUUsageNSec` of each invocation. systemd prints no
"Consumed" line for runs this short.
- Main: 9, 10, 11, 10, 59 ms. The idle stamp was unchanged throughout, so these are fast-path runs.
- Joined server: 6, 6, 4, 6 ms.
- The runner's notes give 349–397 ms for the same work on 125.

**Completed rollback under the new runner.**
- The installed runner is `/usr/libexec/celikpanel/release-recovery` (sha `4b7dc018…`, with the
  idle fast path).
- Update 126 → lab 127 was pressed at 13:28:47 with the unsupported key in `panel.env`.
- The installer refused after the services stopped (13:29:10). The update status said:
  `!! CELIKPANEL_UPDATE_FAILURE … state=recovery_required reason=installer stopped: panel.env line 5
  has an unsupported key …`.
- The runner did the rollback:
  - 13:29:24: "Recovery dispatch admitted: attempt=1 snapshot=…";
  - 13:29:27: "Verified snapshot";
  - 13:29:52: **"Rollback complete / Geri alma tamamlandı"**, with restored commit `7c8453e98`,
    Panel active, Agent active, and the unit "Deactivated successfully" (26.2 s CPU).
- The panel answered as 126 at 13:29:46.
- Between the rollback and the next press, the only failed unit was the refused attempt's own
  `celikpanel-self-update-5d74….service` (`Failed with result 'exit-code'` at 13:29:23). No
  panel, Agent or recovery unit failed.

**The next Update press works.**
- The owner removed the line and pressed Update at 13:29:48. This was during the runner's last 4 s.
- It **succeeded in 65 s** (127). The Agent "cleared the failed mark of
  celikpanel-self-update-5d74…; a later update succeeded and the attempt's record is kept".
- `systemctl --failed` empty; no "database publication is unverified" line.

### 7. Update 125 → 126, first press (`C1-p6-backup.log`, `C1-p6-main.log`)

| | Joined server | Main |
|---|---|---|
| Result | succeeded in 68 s | succeeded in 63 s |
| Schema | 63 → 64 | 63 → 64 |
| `systemctl --failed` | empty | empty |
| "database publication is unverified" | 0 | 0 |
| Agent commit | `7c8453e98`, matches | `7c8453e98`, matches |

## Observations (not blocking)

- **Misleading log line on a server without mail.** On the joined server (no mail), the start-up
  mail work that waited for the running update later logged "mail certificate publication failed:
  publish full mail SNI snapshot: postfix is not installed; it is not retried now and runs again at
  the next Panel start (sudo systemctl restart celikpanel-panel)".
  - It is journal-only, and no screen shows it.
  - The code is unchanged since alpha.125 (`cmd/panel/startup_deferred_mail.go`); the line appears
    only when that work was deferred by an update. Panel starts without an update (12:39) did not
    log it.
  - "Postfix not installed" should read as nothing to do, not as a failure with a restart command
    (D-024).
- **The first-login tour can cover the progress window.** In the headless runs (a fresh browser
  session, the owner's tour not yet dismissed), the tour "Welcome to CelikPanel, step 1 of 6"
  opened above the locked progress window and covered part of its steps
  (`shots/c2-en-del-delen6-progress-en-desktop.png`).
  - An owner who has dismissed the tour never sees this. A new account whose first action is a
    confirmed change would.
  - The tour should wait while a confirm or progress window is open. Not fixed in this round.
- A deletion's step log first shows a generic "request" step for about 1.5 s before the real
  steps arrive.
