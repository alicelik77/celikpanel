# set14b, 2026-10-13 (UTC): commit 1a74eeda5 measured on one disposable Debian 13 guest: the scan's age reported by the server (set14 D1) and the unreachable Agent named (set14 O1), in a real headless Chrome

A short follow-up of the fourteenth record (`../set14-20261012/README.md`, D1 and O1). Date: `date +%Y%m%d` on the host
gave `20261013` at the start (the reading itself was not recorded; the earliest recorded host time is 05:53:20Z); everything ran on 2026-10-13 UTC, 05:53Z–06:30Z. Built from commit 1a74eeda5
(branch `feat/alpha84-dashboard-self-check`; it follows e6de64a5b, both measured here together). HEAD was 6bca3532a at
the start and at staging (`host/head-at-staging.txt`): one commit after 1a74eeda5 that changes only `ROADMAP*.md`,
`docs/DECISIONS*.md` and `docs/RELEASE-NOTES-v0.1.0-alpha.84*.md`; `git diff --stat 1a74eeda5 HEAD -- cmd internal web/src`
and `git status --short -- cmd internal web/src` were empty (`build/source-check.txt`). The installed archive is the
disposable fixture commit d84f62b16307 (labelled v0.1.0-alpha.81 by `run-upd1.sh build`, acceptance-test licence build,
archive sha256 `65d9cde9…`), which differs from 1a74eeda5's tree in one file, `deploy/release-sequence-policy`
(`build/fresh/trees.txt`); the footer shows `v0.1.0-alpha.81 build d84f62b16307`. Nothing of the product was changed by
this run; nothing was committed or pushed.

The harness addressed no installed server, no licence service and no certificate authority. The release origin's name
and the four certificate-authority names were pinned to the guest's loopback first and read back at the end
(`debian13/run-a/driver/steps/01-set6-name-pinning`, `09-set6-name-pinning-at-the-end`, passed). Nothing else was blocked:
the guest had outbound NAT (QEMU user networking); the setup installed nginx, PHP-FPM and MariaDB from Debian's
repositories, and with `timedatectl set-ntp true` the guest's time service could reach its NTP servers (no capture).
`result.json` carries `native_evidence: false`. It closes no P0 row.

**Times.** Times here are UTC. Page text and screenshots show the host's local time (UTC+3): e.g. "Last scan: 13.10.2026
09:18:25" on the Components page is `scanned_at` 06:18:25Z.

## Platform, browser, clock

| | Debian 13 (`debian13/run-a`) |
| --- | --- |
| Lab | `s14b-d13-a` (lab.py `debian13-arch`: an idle Arch peer guest also ran, not measured) |
| Setup | purpose `web`, components nginx, php-fpm, mariadb; DNS external; the owner then chose "set up by hand" (setup stays at `access_dns`, as in set14) |
| Browser | Chrome 154.0.8037.98 headless on the Windows host, desktop 1440×900; English, and Turkish for one Dashboard load in item 3 |
| Driver steps | 10 of 10 passed/observed, `overall: complete-for-review` |

**How the clock offset was measured (new in set14b).** Every answer of `GET /api/v1/managed-services` and `POST
/api/v1/managed-services/scan` carries `scanned_at` and `scan_age_seconds`; their sum is the server's own clock when it
wrote the answer, which lies between the request's start and end on the browser's clock. So "guest minus browser" is
bounded to the request's duration (tens of ms): `answer.guest_minus_browser_ms {low, high}` in `requests.jsonl`.
Besides the measured loads, four such reads were made by a separate tab that is not the app (pages `clock-*`: a
navigation to `GET /api/v1/managed-services`, which only reads the stored records; `events.jsonl` `clock-read`). The
driver's probe estimate (`guest_minus_host_ms_approx`, as set14 used) is only good to about ±1 s; it is kept, not used.

| When | Guest minus browser |
| --- | --- |
| before item 1 (06:09:44Z), natural | **−45 … +35 ms** |
| the harness moved the guest clock +1505 ms (`timedatectl set-ntp false`; `date -u -s @…`, rc 0) to reproduce set14's +1.04…+1.62 s | **+1479 … +1534 ms** (06:09:46Z) |
| during item 1's load (two GETs, the POST) | +1520 … +1548 ms (GETs); POST bound +1527 … +2486 ms |
| item 1b fresh load (06:10:34Z) | +1522 … +1543 ms |
| item 2: after moving the guest clock −420 000 ms (06:11:23Z) | **−418 536 … −418 465 ms** (probe at the load 06:16:43Z ≈ −417.9 s, at the end ≈ −418.3 s) |
| after +420 000 ms and `timedatectl set-ntp true` (rc 0 both; 06:18:20Z) | **−56 … +41 ms** (the +1505 ms of item 1 was gone too: presumably the time service stepped the clock; the NTP settings were not recorded, below) |
| item 3 (Components 06:18:24Z, press 06:25:17Z) | −25 … +108 ms; +23 … +1060 ms |

## Results

**1 D1 gone** (the guest 1.48–1.55 s ahead of the browser). Stored scan 316.8 s old by the guest clock (after a wait of
165.7 s). One Dashboard load at 06:09:46.374Z, observed 45 s (`p1-stale`):

- Requests: `GET /api/v1/managed-services` ×2 (the page reads it twice), 200, 26 / 22 ms:
  `"scanned_at": "2026-10-13T06:04:26Z", "scan_age_seconds": 322.423` and `… 322.468` (JSON numbers).
- **Exactly 1 POST** `/api/v1/managed-services/scan?max_age_seconds=300`, 200, 06:09:46.948Z → 06:09:47.907Z (959 ms),
  27 services: `"scanned_at": "2026-10-13T06:09:49.434300086Z", "scan_age_seconds": 0` — the stamp is 1.53 s after the
  browser received it, the case set14 measured as D1.
- Card (read every 100 ms and on every DOM change): absent 138 ms → **"Checking components…" 993 ms (06:09:46.918Z) →
  "3 / 3 running / installed" (06:09:47.911Z, 4 ms after the answer) until the page closed 45 s later**. No "Not
  checked in the last 5 minutes" frame at any time in the run (`cardlog.jsonl`: 0 rows in state `not-recent`). set14
  measured 28.6 s of that frame at this point on Debian.
- Pictures: `stale-load-checking-card.png`, `stale-load-known-card.png` (+ viewports, page text).

**1b Fresh load** (06:10:33.636Z, stored scan 46 s old): **0 POSTs**; GETs ×2: `"scanned_at": "2026-10-13T06:09:49Z",
"scan_age_seconds": 46.695` / `46.754`; card absent 170 ms → checking 34 ms (records read) → "3 / 3" for 45 s.

**1c Components page** (06:18:23.672Z; stored scan 835.2 s old on the guest clock, which includes the clock's return
from item 2): GETs ×2 `"scanned_at": "2026-10-13T06:04:27Z", "scan_age_seconds": 837.349` / `837.366`; **exactly 1
conditional POST**, 200, 1557 ms, `"scanned_at": "2026-10-13T06:18:25.967607422Z", "scan_age_seconds": 0`; no toast.

`scan_age_seconds` was a JSON number in all 19 browser-recorded managed-services answers that carried a scan, and an integer 0 in the driver's own `scan-now` answer (GETs, including
the four clock reads, and 200 POSTs; values −323.561 … 837.366); it is absent from the 503 bodies (no scan there); no `null` occurred (a scan always existed).

**2 Clock behind: not measurable on this build — defect D2 found (below).** The guest clock was moved 420 s back
(measured −418.5 s) at 06:11:21Z. The owner's unconditional scan through the API right after (`scan-now`, 200 at
06:11:26Z) answered `"scanned_at": "2026-10-13T06:04:27.549037454Z", "scan_age_seconds": 0` — the server's age
statement is independent of the skew. A read just before it had answered `"scanned_at": "2026-10-13T06:09:49Z",
"scan_age_seconds": -323.561` (the stored stamp in the server's future; negative, as 1a74eeda5 documents). After the
stored scan was 317.4 s old on the guest clock, one Dashboard load at 06:16:44.748Z, observed 90 s: **the Dashboard
never mounted**. The page read `/api/v1/auth/me` (200), `/api/v1/panel/availability` (200) and then
`/api/v1/license/access` **19 times, every 5 s, all 200**, and nothing else: no `managed-services` read, **0 scan
POSTs**, no card at any time. So "one conditional POST, the result at once, no second POST for 60 s" could not be
observed: the licence gate held the page first. The item was stopped there (no smaller skew: anything over about 60 s
behind meets the same gate; the request asked for at least 6 minutes). Restored at 06:18:16Z: `date` +420 000 ms (rc 0),
`timedatectl set-ntp true` (rc 0); offset afterwards −56 … +41 ms.

**3 O1 fixed** (owner `systemctl stop celikpanel-agent.service` at 06:23:43Z, rc 0, active → inactive; stored scan
316.8 s old):

| Load | Scan POST | Answer | Card |
| --- | --- | --- | --- |
| Dashboard, English (06:23:43Z) | **1**, `?max_age_seconds=300` | **503**, 5 ms | absent 202 ms → checking 38 ms → **"Unknown / Could not check: The CelikPanel Agent on this server is not reachable, so this request was not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl start celikpanel-agent. Once the Agent is running, try again."** with the "Scan services" button, held until the page closed (~40 s); no second POST |
| Components page (06:24:24Z) | **1**, conditional | **503**, 158 ms | toast (role `alert`, 06:24:24.646Z, 28 ms after the answer) with the same sentence |
| Dashboard, Turkish (06:24:31Z) | **1**, conditional | **503**, 4 ms | **"Bilinmiyor / Kontrol edilemedi: Bu sunucudaki CelikPanel Agent’ına ulaşılamıyor; bu yüzden istek yapılmadı ve hiçbir şey değiştirilmedi. Sunucunun sahibi Agent’ı sunucuda sudo systemctl status celikpanel-agent komutuyla denetler (günlüğü: sudo journalctl -u celikpanel-agent); durmuşsa sudo systemctl start celikpanel-agent ile başlatır. Agent çalışınca yeniden deneyin."** with "Servisleri tara", held 45.8 s |

The three 503 bodies are identical: `{"error":"The CelikPanel Agent on this server is not reachable, so this request was
not carried out and nothing was changed. The server owner checks the Agent on the server with sudo systemctl status
celikpanel-agent (its log: sudo journalctl -u celikpanel-agent) and, if it is stopped, starts it with sudo systemctl
start celikpanel-agent. Once the Agent is running, try again.","code":"AGENT_UNREACHABLE"}` — code `AGENT_UNREACHABLE`,
the message in `error` (there is no `message` field), **no socket path** (`body_mentions_socket: false`: no
`agent.sock`, `/run/celikpanel`, `dial unix`, `connect:`). The cause stays in the Panel's journal only: `[503] verify
privileged service mutation admission: cannot reach agent socket: dial unix /run/celikpanel/agent.sock: connect:
connection refused`. Pictures: `agent-en-failed-card.png`, `agent-tr-failed-card.png` (+ viewports, page text),
`agent-components-toast-viewport.png` (the toast at the top right; the page also shows observations O2 and O3).

**Admission of a mutation with the Agent stopped** (06:24:30Z, the driver as the logged-in owner): `POST
/api/v1/service/action` body `{"name":"nginx","action":"restart"}` → **503** `{"code":"AGENT_UNREACHABLE","error":"<the
same sentence>"}`. Nothing changed: nginx `MainPID` 7197, `ActiveEnterTimestampMonotonic` 149278403, `NRestarts` 0 and
`ExecMainStartTimestamp` 06:03:47 UTC identical before and after (`native/024`, `025`); journal `[503] verify privileged
service mutation admission …` at 06:24:30.677Z.

**Agent back, the button.** `systemctl start` at 06:25:02Z (rc 0, active, MainPID 16759; journal "Agent listening");
the Turkish page sent nothing by itself in the 15 s before the press (the failure is not repeated). Press "Servisleri
tara" at 06:25:17.483Z: **1 unconditional POST** `/api/v1/managed-services/scan`, 200, 1037 ms, `"scanned_at":
"2026-10-13T06:25:18.547687197Z", "scan_age_seconds": 0.001`; card checking 1041 ms → "3 / 3 çalışan / kurulu"
(`agent-after-click-*`).

**4 Window rule** (one conditional POST only after ≥ 300 s): not re-measured, as asked; set14 item 3 measured it.

Scan POSTs in the whole run: 6 (items 1, 1c, three 503s, the press). `browser/summary.json` lists them per phase.

## Product defect found (not fixed)

**D2 — with the server's clock more than about a minute behind the browser's, no Panel page is shown: the licence gate
compares the server's `valid_until` with the browser's clock.** `GET /api/v1/license/access` answers `valid_until =
min(ExpiresAt, OfflineUntil)` (`cmd/panel/license.go:226`), a time on the server's clock no more than 60 s after the last
verification (`internal/licensing/acceptance_fixture.go:301`; a customer licence `internal/licensing/license.go:197`,
`IssuedAt + 60 s`). `parseAccessObservation` refuses a positive answer whose `valid_until` is not after the browser's
`Date.now()` (`web/src/lib/accessObservation.ts:11`), so access stays unknown and is read again every
`UNKNOWN_RECHECK_MS = 5000` (`web/src/components/LicenseOnboarding.tsx:15`, `:190`); the Dashboard is not mounted.
Observed: item 2 above (19 reads at 5 s, 90 s, no Dashboard request). Limit of the record: the access answers' bodies
were not recorded (the recorder keeps bodies only for managed-services), so `valid_until` during the skew is read from
the code, and no picture of the held page was taken (the recorder pictures card states only). It is the same class as
D1 (two clocks compared) in a component neither e6de64a5b nor 1a74eeda5 touched; it hides 1a74eeda5's clock-behind
correction from a real browser.

## Observations (not defects of these commits)

- **O2** Components page with the Agent stopped: besides the toast, the banner "Service state needs verification. A
  server action may have completed, but the latest state could not be verified. Run Rescan before changing another
  component." although no action ran and the answer says nothing was changed: every failed automatic check marks the
  state unverified (`web/src/components/ServiceList.tsx:653-655`, `:701-704`, `:710-712`; banner `:1253`).
- **O3** With the Agent stopped, `GET /api/v1/firewall` still answers 500 (journal `[500][agent] firewall`), the
  Components page shows "Firewall: Status unknown … internal server error", the Dashboard "Part of this server's state
  could not be read …" (set14 noted the same). 1a74eeda5 names the unreachable Agent at the shared mutation admission
  and in the scan only, as its message says.
- Each Dashboard load reads `/api/v1/managed-services` twice; each page's first `/api/v1/auth/me` is recorded as
  `net::ERR_ABORTED` (status `null`) followed by a 200 (four pages).

## Not measured, with the reason

- Item 2's expected behaviour (D2 above).
- The window rule, the hidden tab, the held (409) state, `server_setup_busy`, "never checked": not asked for.
- Ubuntu and Arch: one platform was asked for.
- The guest's NTP settings: the probe's `timedatectl show -p NTP,NTPSynchronized,…` printed no fields (return code 0),
  so the records hold only return codes; that the time service stepped the clock after `set-ntp true` is inferred from
  the offsets (about +1.5 s when it was switched on at 06:18:18–20Z, −56 … +41 ms at 06:18:20.9Z).
- The final guest offset after the window: at the end the driver undid its recorded net shift (−1505 ms, mechanically,
  although the time service had already removed it) and switched NTP on again; the browser had closed, so no precise
  read followed; the lab was stopped.

## Method, deviations

Build (`build/`): `run-upd1.sh build 1a74eeda5` from run copy a (pristine `git archive 1a74eeda5`), Go 1.26.5,
05:54Z–05:59Z, `UPD1-ARTIFACTS-OK`; `run-upd1.sh prove` rc 0; dry run rc 0 (10 steps). Run copy b = a + the five set14b
files (`harness-run-copy/b/files.sha256`, `harness.diff`); offline suite in copy b: `test_set14b_trial` (4) and
`test_set14_trial` (5), 9 OK (`suite.txt`). Copy b was made twice: the first suite failed set14's own test because
`set14b_trial.py` added its cell to set14's names at import (`suite-first.txt`, `mkov-first.txt`); the names are now
lent only while the window runs, and copy b was made again (`host/progress.txt`). Disk gate: one decision, allowed at
231.6 GiB (`host/c-drive-cells.txt`); 36 readings 05:53Z–06:28Z, lowest 220.03 GiB. Neighbour gate: no qemu, no earlier
run job. Keep-awake 05:53Z–06:28Z. Every wait was bounded and ran in the WSL host or in the browser process
(`tools/waitready.sh`, `tools/waitjob.sh`, the browser's own limits). The browser ran on the host under `timeout 5400`
(`tools/browser.sh`), 06:06:50Z–06:26:00Z. `stage-common.sh` stopped at `git status` over the WSL mount (killed by its
600 s limit); `host/working-tree-status.txt`, `head-at-staging.txt` and `tools/` were then written from the host's Git.
Lab preparation recorded as such: the guest clock moves (`native/*lab-clock*`); owner actions: Agent stop/start
(`native/023`, `026`), the nginx restart request (`admission`), the scan (`scan-now`).

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set14b_trial.py`, `guest_set14b_native.py`, `run-set14b.sh`, `test_set14b_trial.py`
(4 offline tests); `web/tools/browser-inspect/cold-load-set14b.mjs` (copied from set14's, plus: card read on every DOM
change, alert toasts recorded, managed-services answers recorded with both fields and, when refused, the whole body;
the clock bound above). Hashes: `harness-run-copy/b/files.sha256`.

## Secrets, host, leftovers

Redacted at collection (`tools/scrub.py`): host paths and the operator's user and machine names → `<redacted>` in 21
text files (`browser/node-run.txt`, `host/job.sh`, the copied tools); the copied `tools/scrub.py` has its own patterns
redacted and does not run as written. The driver's redaction and set6's digest sweep ran on the staged lab (0 files
changed, `debian13/run-a/host/digest-sweep-at-staging.json`); driver `SHA256SUMS` verified after staging (123 files). A
text scan found no operator name, machine name, host path, private key, `Set-Cookie` value (shown as `[REDACTED]`) or
password; the lab user `labadmin`, the guest name `dns-debian13` and `192.0.2.x` appear in pictures and records; SSH lab
keys are `celikpanel-lab`. The browser profile (`%TEMP%\cpb\<8 hex>`, 46 characters) was deleted after the run. The
overlay disks and base-image links of the lab were removed after staging (`host/removals.txt`); left on the WSL host:
`/var/tmp/cp-set14b-run`, `/var/tmp/cp-release-drill-s14b-d13-a` (records, no disks), the build
`/var/tmp/cp-upd1-build/20261013t055405z` and five new dist folders under `/var/tmp/cp-pair-accept/dist/` (`d84f62b1…`,
`ee76ce1e…`, `a20b6dac…`, `c1465c4f…`, `a657c2ae…`). The image cache was used read-only. Commit this folder with
`git add -f` (logs are ignored); `SHA256SUMS` files are written last.

## Intake notes (2026-10-13)

`browser/ui/phases.json`, phase `agent`, field `toast` is wrong: it holds the text of the Firewall banner that the Components page shows ("Firewall: Status unknown …"), not the error toast. The toast with the Agent sentence is in `browser/ui/toasts.jsonl` (06:24:24.646Z, page `p6-agent-components`), as the Results above say. The file and line numbers cited under D2 are those of commit 1a74eeda5 (identical in HEAD 6bca3532a, `build/source-check.txt`); the working tree has since been edited, so the same lines sit elsewhere there.
