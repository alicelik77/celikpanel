# set14c, 2026-10-13 (UTC): commit 68b79b372 measured on one disposable Debian 13 guest: the licence gate with the server's clock 7 minutes behind and ahead (set14b D2), the unreachable Agent on reads (set14b O3) and the Components page's read banner (set14b O2), in a real headless Chrome

The native record after the follow-up record set14b (`../set14b-20261013/README.md`, items D2, O2, O3 and its intake
notes), measuring what 7024534bc changed (and 68b79b372, the read banner's measure). Date: `date +%Y%m%d` on the host
gave `20261013` at the start (the reading's own time was not recorded; the first host times recorded in this folder are
`host/c-drive-watch.txt` 07:25:46Z, then `hostcheck-before.txt` and `progress.txt` 07:25:54Z); everything ran on
2026-10-13 UTC, 07:25Z–07:59Z (the last host time recorded is `host/keepawake.log`, 07:59:17Z).
Built from commit 68b79b372 (branch `feat/alpha84-dashboard-self-check`; product code last changed in 7024534bc and
68b79b372). HEAD was 4ed7b1278 at the start and at staging (`host/head-at-staging.txt`): one commit after 68b79b372
that changes only `ROADMAP*.md`, `docs/DECISIONS*.md` and `docs/RELEASE-NOTES-v0.1.0-alpha.84*.md`; `git diff --stat
68b79b372 HEAD -- cmd internal web/src` and `git status --short -- cmd internal web/src` were empty
(`build/source-check.txt`). The installed archive is the disposable fixture commit 09d96f93ceb1 (labelled
v0.1.0-alpha.81 by `run-upd1.sh build`, acceptance-test licence build, archive sha256 `833c1b0e…`), which differs from
68b79b372's tree in one file, `deploy/release-sequence-policy` (`build/fresh/trees.txt`); the footer shows
`v0.1.0-alpha.81 build 09d96f93ceb1`. Nothing of the product was changed by this run; nothing was committed or pushed.

The harness addressed no installed server, no licence service and no certificate authority. The release origin's name
and the four certificate-authority names were pinned to the guest's loopback first and read back at the end
(`debian13/run-a/driver/steps/01-set6-name-pinning`, `09-set6-name-pinning-at-the-end`, passed). Nothing else was blocked:
the guest had outbound NAT (QEMU user networking); the setup installed nginx, PHP-FPM and MariaDB from Debian's
repositories, and with `timedatectl set-ntp true` the guest's time service could reach its NTP servers (no capture).
The driver's own finding text says "on an isolated host"; that is its fixed wording, the guest was not isolated.
`result.json` carries `native_evidence: false`. It closes no P0 row.

**Times.** Times here are UTC (the browser's and the WSL host's clocks). Page text and screenshots show the host's local
time (UTC+3): e.g. "Last scan: 13.10.2026 10:40:40" on the Components page is `scanned_at` 07:40:40Z, "Son tarama:
13.10.2026 10:47:01" is 07:47:01Z. Times the driver writes from the guest (`at` of a preparation, journal lines) are on
the guest's clock, which was moved during items 1b and 1c.

## Platform, browser, clock

| | Debian 13 (`debian13/run-a`) |
| --- | --- |
| Lab | `s14c-d13-a` (lab.py `debian13-arch`: an idle Arch peer guest also ran, not measured) |
| Setup | purpose `web`, components nginx, php-fpm, mariadb; DNS external; the owner then chose "set up by hand" (setup stays at `access_dns`, as in set14 and set14b) |
| Browser | Chrome 154.0.8037.98 headless on the Windows host; desktop 1440×900 English; one Turkish phone page 390×844 (item 4) |
| Driver steps | 10 of 10 passed/observed, `overall: complete-for-review` (07:32:50Z–07:56:18Z) |

**How the clock offset was measured.** As in set14b: `scanned_at + scan_age_seconds` of a managed-services answer is
the server's clock when it wrote the answer, between the request's start and end on the browser's clock (pages
`clock-*`, a navigation to `GET /api/v1/managed-services` that only reads the stored records; `events.jsonl`
`clock-read`). New here: every `GET /api/v1/license/access` answer gives a second, tighter bound, because
`valid_until − valid_for_seconds` is the server's clock when it answered (to the millisecond; `access.server_now` and
`access.guest_minus_browser_ms` in `requests.jsonl`). The driver's probe estimate (±1 s) is kept, not used.

| When | Clock read (managed-services) | Access answers of the measured load |
| --- | --- | --- |
| before item 1a (07:40:38Z), natural | **−78 … +64 ms** | +49 … +59 ms |
| item 1b, after `timedatectl set-ntp false` and `date -u -s @…` −420 000 ms (rc 0 both; 07:47:16Z) | **−420 002 … −419 945 ms** | — |
| item 1b at the load (07:52:35Z) | −419 999 … −419 943 ms | −419 952 … −419 946 ms |
| item 1c, after +840 000 ms (rc 0; 07:54:06Z) | **+419 996 … +420 058 ms** | +420 047 … +420 054 ms |
| item 1c after the load (07:55:38Z) | +419 990 … +420 056 ms | — |
| restored: −420 000 ms (rc 0) and `timedatectl set-ntp true` (rc 0), 07:55:42Z | **+3 … +52 ms** | — |
| 30 s later (07:56:12Z) | **−4 … +64 ms** | — |

The net shift of the harness was 0 at the end (`clock-at-the-end.json`: `shift_total_ms_before` 0, so no undo was
needed; NTP had been switched on by the browser's own step).

## Results

### 1 D2 fixed — the licence gate with the server's clock behind and ahead

The same Dashboard load three times, English desktop, each with the stored scan older than 300 s and observed 90 s:
1a with the clock as it was (what "normal" is, measured first), 1b with the guest 7 minutes behind, 1c with it 7
minutes ahead. Phases `baseline`, `clock-behind`, `clock-ahead` in `browser/ui/phases.json` (also `browser/summary.json`).

| | 1a clock as is | 1b 420 s behind | 1c 420 s ahead |
| --- | --- | --- | --- |
| load | 07:40:38.850Z | 07:52:36.005Z | 07:54:08.333Z |
| stored scan age at the load (guest clock) | 316.8 s | 317.1 s (the owner's scan after the shift, then a wait) | 930.9 s (the jump) |
| Dashboard mounted (first managed-services read / first card) | 192 ms / 253 ms | 130 ms / 165 ms | 132 ms / 172 ms |
| access reads in the first 90 s | **4** | **4** | **4** |
| when (after the load) | 0.11, 44.1, 60.1, 89.1 s | 0.07, 44.0, 60.1, 88.9 s | 0.07, 42.6, 60.1, 87.6 s |
| all answers | 200, `can_use_panel: true`, `state: active`, `observation: known` | same | same |
| scan POSTs | **1** `?max_age_seconds=300`, 200, 1197 ms | **1**, 200, 956 ms | **1**, 200, 1047 ms |
| card | absent 203 ms → checking 1226 ms → **"3 / 3 running / installed"** to the end | absent 120 ms → checking 984 ms → **"3 / 3"** to the end | absent 128 ms → checking 1070 ms → **"3 / 3"** to the end |
| second POST within 60 s of the first | 0 (observed 88.8 s after it) | 0 (89.0 s) | 0 (89.1 s) |

So with the server 7 minutes behind, the page loaded at once and behaved exactly as with the clock correct: no
licence-gate hold, no read every 5 s (set14b: 19 reads at 5 s in 90 s and no Dashboard). The same with the server 7
minutes ahead.

**The access answers' body** (all 12 answers have exactly the keys `can_use_panel`, `observation`, `state`,
`valid_for_seconds`, `valid_until`; `valid_for_seconds` is a JSON number in all; `browser/ui/requests.jsonl`, field
`access`):

| | sent → received (browser) | `valid_until` (server clock) | `valid_for_seconds` | deadline the page takes (sent + valid_for) | the old rule (`valid_until` on the browser clock) |
| --- | --- | --- | --- | --- | --- |
| 1a #1 | 07:40:38.963 → .968Z | 1791877298 (07:41:38Z) | 58.983 | 07:41:37.946Z | accepts; deadline 54 ms later |
| 1a #2 / #3 / #4 | 07:41:22.957 / 07:41:38.966 / 07:42:07.958Z | 1791877343, 1791877343, 1791877388 | 59.987, 43.977, 59.983 | 07:42:22.944, 07:42:22.943, 07:43:07.941Z | accepts; 56–59 ms later |
| 1b #1 | 07:52:36.076 → .080Z | 1791877595 (**07:46:35Z**) | 58.872 | 07:53:34.948Z | **refuses** (its deadline 419.948 s before the page's, 361.1 s before the answer's receipt) |
| 1b #2 / #3 / #4 | 07:53:19.962 / 07:53:36.087 / 07:54:04.952Z | 1791877640, 1791877640, 1791877685 | 59.984, 43.859, 59.995 | 07:54:19.946, 07:54:19.946, 07:55:04.947Z | refuses (419.946–419.947 s before the page's deadline; 360.0–376.1 s before the answer's receipt) |
| 1c #1 | 07:54:08.405 → .409Z | 1791878526 (**08:02:06Z**) | 57.544 | 07:55:05.949Z | would accept **420.051 s longer** |
| 1c #2 / #3 / #4 | 07:54:50.954 / 07:55:08.419 / 07:55:35.959Z | 1791878571, 1791878571, 1791878616 | 59.994, 42.527, 59.987 | 07:55:50.948, 07:55:50.946, 07:56:35.946Z | would accept 420.052–420.054 s longer |

Each answer took 4–9 ms (receipt times: `requests.jsonl` `access.received`). **What the browser uses is
`valid_for_seconds`**: in all three loads the page read again 15 s before the deadline computed from it (1a: deadline
07:41:37.946Z, read 07:41:22.957Z; 1b: 07:53:34.948Z / 07:53:19.962Z; 1c: 07:55:05.949Z / 07:54:50.954Z), and the
next one 15 s before the next such deadline (read #4). In 1c the absolute `valid_until` would have let the page accept
access until 08:02:06Z by the browser's clock, 420 s beyond what the server said; the page's deadline was 07:55:05.949Z
and it read again at 07:54:50.954Z. The read at about 60 s is the page's fixed 60 s re-check
(`web/src/components/LicenseOnboarding.tsx`, `setInterval(…, 60000)` in the first effect at 68b79b372), the same in all
three loads. Nothing in this run shows a deadline later than the server allowed; the browser's internal deadline itself
is not readable from outside, it is inferred from the timing of the reads.

Pictures: `baseline-*`, `clock-behind-behind-*`, `behind-10s|end-*`, `clock-ahead-ahead-*`, `ahead-10s|end-*`
(`-card.png`, `-viewport.png`, page text).

### 2 D2 unfixed baseline

Not measured, as asked: set14b item 2 holds it (1a74eeda5: 19 reads at 5 s, no Dashboard, 90 s).

### 3 O3 fixed — reads that need the Agent, with the Agent stopped

The owner `systemctl stop celikpanel-agent.service` at 07:45:58Z (rc 0, active → inactive, MainPID 0); stored scan
316.8 s old. Driver, as the logged-in owner, through the Panel's API (`agent-reads`, `admission`):

| Request | Status | Body |
| --- | --- | --- |
| `GET /api/v1/firewall` (07:45:59Z) | **503** | `code: AGENT_UNREACHABLE`, keys `code`, `error` only; `body_mentions_socket: false` |
| `GET /api/v1/system/check` (07:45:59Z) | **503** | the same |
| `POST /api/v1/service/action` `{"name":"nginx","action":"restart"}` (07:46:00Z) | **503** | the same |

Every body: `{"code":"AGENT_UNREACHABLE","error":"The CelikPanel Agent on this server is not reachable, so this request
was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status
celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl
start celikpanel-agent. Once the Agent is running, try again."}` — no `agent.sock`, `/run/celikpanel`, `dial unix` or
`connect:`. nginx unchanged by the refused restart: `MainPID` 7213, `NRestarts` 0, `ActiveEnterTimestampMonotonic`
142833567, `ExecMainStartTimestamp` Tue 2026-10-13 07:34:40 UTC, active, identical before and after (`native/007`,
`008`). The cause stays in the Panel's journal only: `[503][agent] firewall`, `[503] cannot reach agent socket: dial
unix /run/celikpanel/agent.sock: connect: connection refused` (07:45:59Z, twice), `[503] verify privileged service
mutation admission: …` (07:46:00Z). set14b measured `GET /api/v1/firewall` 500.

**The Dashboard** (English desktop, 07:46:01Z, 40 s): **1** conditional POST `/api/v1/managed-services/scan?max_age_seconds=300`,
**503**, 3 ms, `AGENT_UNREACHABLE`; the card absent → checking → **"Unknown / Could not check: The CelikPanel Agent on
this server is not reachable, … Once the Agent is running, try again."** with "Scan services", held to the end; no
second POST. The page's own `GET /api/v1/firewall` answered 503 with the same body (`body_mentions_socket: false`).
The other cards: CPU 0 %, Memory 15 % (459 MB / 2.9 GB), Disk 11 % (2.7 GB / 23.4 GB), Hosting 0 / 0 / 1 / 0, Recent
activity — all shown. **The Dashboard shows no firewall banner**; its "Needs attention" box says "Part of this server's
state could not be read, so this list may be incomplete. This does not mean something is wrong. Nothing was changed.
Try again." with "Retry" — it does not name the Agent (observation O6 below); no toast appeared on the Dashboard.
Pictures: `agent-dashboard-en-failed-card.png` and `-viewport.png`,
`agent-dashboard-en-full.png` (the page's own scroll area, so the picture shows the first 900 px) and its text.

### 4 O2 fixed — the Components page with the Agent stopped

| Page | Scan POST | Banner | "may have completed" | Picture |
| --- | --- | --- | --- | --- |
| English, desktop 1440×900 (07:46:42Z) | **1** conditional, **503**, 107 ms | **"Component state could not be read"** with the reason "This page did not send any component action; it could not read the current state of the components. The CelikPanel Agent on this server is not reachable, … Once the Agent is running, try again. Component changes stay locked until a scan reads the state; Rescan reads it again." and the button **"Rescan"**; seen 1007 ms after the load, still there 5 s later | **absent** (neither "Service state needs verification" nor "may have completed" anywhere in the page, at once and 5 s later) | `agent-components-en-desktop-banner-viewport.png` |
| Turkish, phone 390×844 (07:46:48Z) | **1** conditional, **503**, 88 ms | **"Bileşen durumu okunamadı"** — "Bu sayfa hiçbir bileşen işlemi göndermedi; bileşenlerin güncel durumunu okuyamadı. Bu sunucudaki CelikPanel Agent’ına ulaşılamıyor; … Agent çalışınca yeniden deneyin. Bir tarama durumu okuyana dek bileşen değişiklikleri kilitli kalır; Yeniden tara durumu yeniden okur." and **"Yeniden tara"**; seen 1012 ms after the load | **absent** ("Servis durumu doğrulanmalı", "tamamlanmış olabilir": none) | `agent-components-tr-phone-banner-viewport.png` (scrolled to the banner; the error toast covers the page's top) |

Each page also showed the Agent sentence as a toast (07:46:42.476Z, 07:46:48.930Z). `toasts.jsonl` records the text of
every element with role `alert` as it appears, so it also holds the banners (the read banner, the firewall banner,
the Dashboard's "Needs attention" box), not only toasts.

**Agent back, Rescan.** `systemctl start` at 07:46:55Z (rc 0, active, MainPID 15630; journal "Agent listening on unix
socket …"). The Turkish phone page sent nothing by itself in the 5 s before the press (0 POSTs). Press "Yeniden tara"
in the banner at 07:47:00.275Z: **1 unconditional POST** `/api/v1/managed-services/scan` (no query), **200**, 1014 ms,
`"scanned_at": "2026-10-13T07:47:01.347631559Z", "scan_age_seconds": 0`, 27 services; **the banner was gone 1092 ms
after the press** and stayed gone 10 s later; the page shows "Son tarama: 13.10.2026 10:47:01" and the mail stacks
"Kuruluma hazır" / "Kısmen kurulu" (before: "Awaiting verified scan" / Unavailable). Pictures:
`agent-components-after-rescan-viewport.png`, `-full.png`, text.

Scan POSTs in the agent phase: 4 (three 503s, the press); in the whole run 7 from the browser (one per measured load,
the three 503s, the press), plus the driver's own `scan-now` (item 1b).

**An action whose outcome is unknown still shows "may have completed"**: not measured. Producing it needs a component
action through the page whose answer is lost (the Agent stopped mid-action or the answer cut), which this harness has
no cheap way to do; the commit's own tests cover that branch (`web/tests/component-inventory-contract.test.mjs`).

### 5 Scan age and the window rule

Not re-measured, as asked: set14 and set14b measured them. Here they behaved as there (one conditional POST on each
stale load, the stored scan's age stated by the server; values in `requests.jsonl`).

## Product defects found

None in what 7024534bc and 68b79b372 changed.

## Observations (not defects of these commits; file and line at 68b79b372)

- **O4 The Components page's firewall banner shows the server's English sentence on the Turkish page.** "Güvenlik
  duvarı: Durum bilinmiyor / CelikPanel etkin güvenlik duvarı politikasını doğrulayamadı. … " followed by "The CelikPanel
  Agent on this server is not reachable, … try again." in English (`agent-components-tr-phone-banner.txt`,
  `banners_after_click` in `phases.json`). `FirewallBar.load` takes the answer's `error` string as the message
  (`web/src/components/ServiceList.tsx:3259-3271`, `:3286`) and shows it (`:3393`) instead of the code's translation
  (`err.AGENT_UNREACHABLE`, which the scan banner and the toast use). Expected: the Turkish sentence; observed: the
  English one.
- **O5 The firewall banner is not read again after the Agent returns.** After the press and 10 s later, the Turkish
  page still showed "Durum bilinmiyor" with the Agent sentence although the Agent was running and the scan had
  succeeded: `FirewallBar` reads only on mount (`ServiceList.tsx:3289-3291`) and after its own action (`:3318`); Rescan
  does not read it. A reload would.
- **O6 The Dashboard's "Needs attention" does not name the Agent.** With `GET /api/v1/firewall` answering 503
  `AGENT_UNREACHABLE`, the box says only that part of the state could not be read (`dashboard.attentionUnread`,
  `web/src/components/Dashboard.tsx:895`, `:1163-1165`), while the System services card on the same page names the
  Agent and the owner's commands.
- Untranslated English text on the Turkish Components page: the mail stack descriptions ("Postfix SMTP and Dovecot
  IMAP/POP3 with authenticated submission.", "Core mail plus Nginx, PHP-FPM and Roundcube webmail.") and "Some components
  are already present. Installing this profile will reconcile the full plan without removing completed services."
  (`agent-components-after-rescan-viewport.png`). Not looked into.
- On four pages (`p2`, `p5`, `p6`: `/api/v1/auth/me`; `p4`: `/api/v1/panel/availability`) the first request is recorded
  as `net::ERR_ABORTED` followed by a 200, as set14b saw.

## Not measured, with the reason

- Item 2 (D2 unfixed): set14b holds it, as asked.
- The "may have completed" banner after an action with an unknown outcome: no cheap way (item 4).
- The window rule, the scan's age rules, the hidden tab, the held (409) state, `server_setup_busy`, "never checked": not
  asked for / already measured.
- Ubuntu and Arch: one platform was asked for.
- The guest's NTP settings: `timedatectl show -p NTP,NTPSynchronized,CanNTP,…` again printed no fields (rc 0; same as
  set14b), so only return codes are recorded. Whether the time service stepped the clock after `set-ntp true` cannot be
  told here: the harness's own −420 s step had already brought the offset to +3 … +52 ms.
- The Dashboard's full-page picture covers only the viewport height (the page scrolls inside its own area); the full
  text is in `agent-dashboard-en-full.txt`.

## Method, deviations

Build (`build/`): `run-upd1.sh build 68b79b372` from run copy a (pristine `git archive 68b79b372`, 4768 files), Go
1.26.5, 07:26:22Z–07:31:40Z, `UPD1-ARTIFACTS-OK`; `run-upd1.sh prove` rc 0; dry run rc 0 (10 steps). Run copy b = a +
the four set14c files (`harness-run-copy/b/files.sha256`, `harness.diff`); offline suite in copy b: `test_set14c_trial`
(3), `test_set14b_trial` (4, set14b's own, unchanged) and `test_set14_trial` (5): 12 OK (`suite.txt`). Disk gate: one
decision, allowed at 194.9 GiB (`host/c-drive-cells.txt`; the printed `free_GiB=194` is the comma-decimal reading cut
by the gate's parser, the bytes are 209288314880); 34 readings 07:25Z–07:58Z, lowest 191.35 GiB. Neighbour gate: no
qemu, no earlier run job (the pattern now also covers `cp-set14b-run`). Keep-awake 07:25Z–07:59Z. Every wait was
bounded and ran in the WSL host or in the browser process (`tools/waitready.sh`, `tools/waitjob.sh`, the browser's own
limits); host-side waits were single bounded `sleep`s. The browser ran on the host under `timeout 5400`
(`tools/browser.sh`), 07:37:43Z–07:56:15Z, exit 0. `stage-common.sh` no longer runs `git status` over the WSL mount
(set14b's hang): `host/working-tree-status.txt` and `head-at-staging.txt` were written from the host's Git; `init.sh`
bounds its path-limited `git status` with `timeout 120` (it finished). `tools/quicktest.sh` ran the offline suite once on
a temporary LF copy before run copy b (12 OK; the copy was removed). Lab preparation recorded as such: the guest clock
moves (`native/*lab-clock*`, `lab-ntp-on`); owner actions: Agent stop/start (`native/006`, `009`), the nginx restart
request (`admission`), the two reads (`agent-reads`), the scan (`scan-now`).

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set14c_trial.py` (subclass of set14b's driver: its cell, the `agent-reads` preparation,
names lent to set14b's window only while it runs), `run-set14c.sh` (set14b's wrapper with the names changed),
`test_set14c_trial.py` (3 offline tests); `web/tools/browser-inspect/cold-load-set14c.mjs` (copied from set14b's; plus
the access answers recorded whole with send/receipt times, the page's deadline and the clock bound, the Agent reads'
bodies, a viewport per page, and the phases above). No guest helper was needed: set14b's `guest_set14b_native.py`
is used as it is. Hashes: `harness-run-copy/b/files.sha256`.

## Secrets, host, leftovers

Redacted at collection (`tools/scrub.py`): host paths and the operator's user and machine names → `<redacted>` in 20
text files (`browser/node-run.txt`, `host/job.sh`, the copied tools); the copied `tools/scrub.py` has its own patterns
redacted and does not run as written. The driver's redaction and set6's digest sweep ran on the staged lab (0 files
changed, `debian13/run-a/host/digest-sweep-at-staging.json`); driver `SHA256SUMS` verified after staging (121 files). A
text scan found no operator name, machine name, host path, private key, `Set-Cookie` value (only the header's name) or
password (`[REDACTED]`); the lab user `labadmin`, the guest name `dns-debian13` and `192.0.2.x` appear in pictures and
records; the SSH lab key is `celikpanel-lab` (public). The browser profile (`%TEMP%\cpb\<8 hex>`, 46 characters) was
deleted after the run. The overlay disks and base-image links of the lab were removed after staging
(`host/removals.txt`); left on the WSL host: `/var/tmp/cp-set14c-run`, `/var/tmp/cp-release-drill-s14c-d13-a` (records
and the two cloud-init seed ISOs, no disks), the build `/var/tmp/cp-upd1-build/20261013t072622z` and five new dist
folders under `/var/tmp/cp-pair-accept/dist/` (`09d96f93…`, `2e64b4ab…`, `3a5dbba9…`, `f23f7093…`, `f58a91db…`). The
image cache was used read-only. Commit this folder with `git add -f` (logs are ignored); `SHA256SUMS` files are written
last.

## Intake notes (2026-10-13)

Checked against the raw files before commit. Every `SHA256SUMS` verified (81, 121, 11 and 60 lines; 273 files listed,
273 present, none missing or unlisted). The README's access, POST, clock-offset, banner and 503 numbers match
`browser/ui/requests.jsonl`, `phases.json` and `events.jsonl`. Corrections made at intake: (1) the first paragraph named a
07:24:57Z disk reading and a 10:24:58 PowerShell `date` that no file in this folder holds; it now names the earliest
recorded host times (07:25:46Z, 07:25:54Z) and the last (07:59:17Z). (2) In the access-answer table the old rule's
"deadline before" figures are measured against the page's own deadline (`valid_for_seconds` from the send time); by the
browser clock `valid_until` itself lay 361.1 s (1b #1) and 360.0–376.1 s (1b #2–#4) before the answer's receipt, not 420 s,
because `valid_until` is the server's clock plus about 60 s and the server was 420 s behind. Also: the run recorded 17
access answers in all (the sign-in page 1, the three measured loads 12, the Agent-phase pages 4), and all 17 carry
exactly the five keys with `valid_for_seconds` a JSON number (42.5–60.0 s in the 12 measured, 16.7–59.99 s in the Agent
phase, where the Dashboard and Components pages read at different times of the stored window). The CPU and memory figures
quoted for the Dashboard are those of `agent-dashboard-en-full.txt` (0 %, 459 MB); `agent-dashboard-en-failed.txt`,
taken 30 s earlier, shows 5 % and 455 MB. The 60 s re-check is `LicenseOnboarding.tsx:116-119` at 68b79b372.
Observations O4 (`ServiceList.tsx:3259-3271`, `:3286`, `:3393`), O5 (`:3289-3291`, `:3318`) and O6
(`Dashboard.tsx:895`, `:1163-1165`) were verified against 68b79b372 and are exact. Outstanding limits, as above: the host
`date` reading's time, page text and pictures in host local time (UTC+3), the "may have completed" banner after an
action (not measured), Ubuntu and Arch (not run), the guest's NTP settings (not readable), and the untranslated mail-stack
text (not looked into). Only `host/keepawake.log` is git-ignored (`*.log`); it is in `SHA256SUMS`, so it needs `git add -f`.
