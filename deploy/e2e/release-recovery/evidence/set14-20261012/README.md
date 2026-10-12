# set14, 2026-10-12 (UTC): commit a5830dd3e measured on disposable Debian 13 and Ubuntu 24.04 guests: the Dashboard's "System services" card checks the components by itself, in a real headless Chrome

A measurement record. Date: `date +%Y%m%d` on the host gave `20261012` when the run began (the reading itself was not recorded; the earliest recorded host time is 20:56:32Z and the build directory is named 20261012t205712z); every lab ran
on 2026-10-12 UTC (21:09Z–22:34Z); the host's local clock passed midnight during the run, the name keeps the date read
at the start. Built from commit a5830dd3e (branch `feat/alpha84-dashboard-self-check`); `git diff --stat a5830dd3e HEAD
-- cmd internal web/src` and `git status --short -- cmd internal web/src` were both empty when the build began
(`build/source-check.txt`; HEAD was a5830dd3e at the start and at staging, `host/head-at-staging.txt`). The installed
archive is the disposable fixture commit 27c8b7e75d34 (labelled v0.1.0-alpha.81, acceptance-test licence build
`acceptance-fixture`, archive sha256 `6cea530d…`); it differs from a5830dd3e's tree in one file,
`deploy/release-sequence-policy` (`build/fresh/trees.txt`). The panel footer shows `v0.1.0-alpha.81 build 27c8b7e75d34`.
Nothing of the product was changed by this run; nothing was committed or pushed.

The harness addressed no installed server, no licence service and no certificate authority. The release origin's name
and the four certificate-authority names were pinned to each guest's loopback as the first driver step and read back at
its end (`*/run-a/driver/steps/01-set6-name-pinning`, `09-set6-name-pinning-at-the-end`, 4 of 4 passed). Nothing else
was blocked: the guests had outbound NAT (QEMU user networking), and the Panel's setup installed its components (nginx,
PHP-FPM, MariaDB) from each distribution's repositories; the build on the WSL host ran `npm ci`. No capture of that
traffic exists. Every `result.json` carries `native_evidence: false`. It closes no P0 row.

## Platforms and browser

| | Debian 13 (`debian13/run-a`) | Ubuntu 24.04 (`ubuntu/run-a`) |
| --- | --- | --- |
| Lab | `s14-d13-a` (lab.py `debian13-arch`: an idle Arch peer guest also ran, not measured) | `s14-ub-a` |
| Setup | purpose `web`, components nginx, php-fpm, mariadb; DNS external; the owner then chose "set up by hand", so the setup stayed at `access_dns` (`server_setup_access_dns_required` in `result.json`) and its certificate and verify steps did not run | same |
| Browser | Chrome 154.0.8037.98 headless, **English, desktop 1440×900** | same Chrome, **Turkish, phone 390×844** |
| Guest clock minus host clock | +1.04 to +1.62 s (21 readings; an intake recomputation from the raw readings, 0.01 s below the figures first written) | +1.23 to +2.15 s (21 readings; same) |
| Driver steps | 10 of 10 passed/observed | 10 of 10 passed/observed |

The browser (`web/tools/browser-inspect/cold-load-set14.mjs`) ran on the Windows host against the guest's Panel through
the driver's loopback SSH forward. It recorded every `/api/` request (`browser/ui/requests.jsonl`: method, path with
query, status, start, end, milliseconds; for the scan POST the answer's `code`, `scanned_at`, service count), the
card's text sampled in the page every 100 ms with the page's visibility (`cardlog.jsonl`), a picture of each card state
the first time it appeared in a phase (`*-card.png`, `*-viewport.png`, page text `*.txt`), and asked the driver for
guest readings (`events.jsonl`, `prep-done` records; the full readings are the driver's `steps/08-set14-browser-window/native/*`).
`browser/summary.json` (made by `tools/summarize.py` from those three files only) lists per phase every scan POST and
every card state with how long it lasted. "Guest clock minus host clock" is approximate: the guest's epoch from a probe
minus the midpoint of the host's request/answer times.

## Results per item

Times are UTC; POST durations are the browser's (request sent to answer finished, through the SSH forward).

**1 Stale load** (stored scan 317.4 s / 319.2 s old by the guest clock, after a wait of 161.9 s / 168.6 s; not absent:
the setup had already scanned). One Dashboard load, observed 45 s:

| | Debian (EN) | Ubuntu (TR) |
| --- | --- | --- |
| POSTs to `/api/v1/managed-services/scan` | **1**, `?max_age_seconds=300`, 200, **1340 ms**, 27 services | **1**, `?max_age_seconds=300`, 200, **4445 ms**, 27 services |
| Card sequence | "Checking components…" 1398 ms → "Unknown / Not checked in the last 5 minutes. …" **28.6 s** → "3 / 3 running / installed" | "Bileşenler kontrol ediliyor…" 4503 ms → "Bilinmiyor / Son 5 dakikada kontrol edilmedi. …" **25.5 s** → "3 / 3 çalışan / kurulu" |
| Agent cgroup CPU (`cpu.stat usage_usec`) over the 45 s load | 293.2 ms in 47.4 s (user 149.4, system 143.8) | 533.4 ms in 47.5 s (user 208.0, system 325.5) |
| Same over the fresh load (item 2, no POST) | 104.0 ms in 46.4 s | 116.3 ms in 47.8 s |
| Difference (≈ one scan's Agent CPU, incl. its child processes) | **≈ 189 ms** | **≈ 417 ms** |
| Panel cgroup CPU, stale vs fresh load | 148.5 vs 168.4 ms (no difference above noise) | 263.2 vs 173.5 ms (+90 ms) |

The difference is an estimate from two 47 s windows (the page's own 5 s polls run in both); `CPUUsageNSec` agrees with
`usage_usec` in every probe. The Agent's MainPID was unchanged across both windows (Debian 5222, Ubuntu 7757).

**2 Fresh load** (stored scan 46.7 s / 43.2 s old): **0 POSTs** on both. The card showed "3 / 3" from the first frame
(Debian) or after an 80 ms "Bileşenler kontrol ediliyor…" frame while the stored records were read (Ubuntu).

**3 Window** (the fresh-load page kept open and visible, observed 282 s / 284 s): **exactly 1 more conditional POST**
on each, 200, Debian 1649 ms, Ubuntu 3078 ms. It started at the page's first 30 s clock tick after the stored scan
became 300 s old: **317.4 s** (Debian) and **about 314 s** (Ubuntu; 312.7 s uncorrected, 314.0–314.8 s corrected with the offsets measured near the window; the figure 313.9 s first written here used the smallest offset of the run and appears in no raw file) after the previous scan's `scanned_at` (guest time, host time corrected by the measured offset): not before 300 s. Card: checking 1.7 s / 3.0 s → "not checked in the last
5 minutes" 28.3 s / 26.9 s → "3 / 3".

**4 Hidden** (a second tab brought to the front; `document.visibilityState` was `hidden`, recorded by the page's own
`visibilitychange` event): hidden **303.9 s** (Debian) / **301.8 s** (Ubuntu); the stored scan was 361.1 s / 363.7 s
old when the page was shown again, i.e. about 61 s / 64 s past due. **0 POSTs while hidden**; while hidden the card
changed at its tick to "Not checked in the last 5 minutes" (read from the hidden page, no picture taken of a hidden page).
**1 POST when shown**, starting 13 ms / 34 ms after the page became visible, 200, 1307 ms / 3851 ms; card checking →
not-recent 25.9 s / 20.0 s → "3 / 3". The real hidden state was checked beforehand in the same Chrome (`tools/vistest.mjs`:
`bringToFront` of another tab fires `visibilitychange` → `hidden`); no visibility override was injected.

**5 Held** (the owner reloads nginx from the Panel, `POST /api/v1/service/action {"name":"nginx","action":"reload"}`,
while a lab drop-in made nginx's reload `/bin/sleep 90`; the stored scan was 315+ s old). Dashboard loaded 4.3 s
(Debian) / 4.2 s (Ubuntu) after the action was sent (the driver's clock, which is the WSL host's):

| | Debian | Ubuntu |
| --- | --- | --- |
| First POST | 409 `service_operation_busy` ("another package operation is already in progress"), 12 ms | 409 `service_operation_busy`, 8 ms |
| Card | "A change is in progress; the status is shown when it finishes." **89.9 s** | "Bir değişiklik sürüyor; durum, değişiklik bitince gösterilir." **89.9 s** |
| Further POSTs while held | 0 | 0 |
| Action's answer | 200 `{"success":true}` after 91.26 s | 200 after 92.06 s |
| Follow-up | **1 POST**, 4 ms after a readiness answer ended, 3.3 s after the action's answer reached the driver; 200 from the cache in 4 ms (`scanned_at` 21:48:38Z, the action's own scan) | **1 POST**, 3 ms after a readiness answer ended, 2.4 s after; 200 in 45 ms (`scanned_at` 22:33:14Z) |
| Card after | checking 99 ms → "3 / 3" | checking 111 ms → "3 / 3" |

The readiness poll ran every 5 s (27 / 28 requests in the phase, all 200); its answer bodies were not recorded, so
"busy, then ready" is read from the code and from the follow-up's timing (it followed a readiness answer within 4 ms),
not from a recorded body. `server_setup_busy`: not measured (below).

**6 Failure** (owner `systemctl stop celikpanel-agent.service`; stored scan 315+ s old): one load → **1 POST**,
`?max_age_seconds=300`, **500** `{"code":"INTERNAL","error":"internal server error"}`, 11 ms / 14 ms; card "Unknown /
Could not check: The server hit an internal error. Try again; if it persists, check the panel logs." with the outlined
"Scan services" button (TR "Kontrol edilemedi: Sunucuda bir iç hata oluştu. Yeniden deneyin; sürerse panel günlüklerine
bakın." / "Servisleri tara"), held 57.1 s / 56.0 s. The Panel's journal: `[500] verify privileged service mutation
admission: cannot reach agent socket: dial unix /run/celikpanel/agent.sock: connect: connection refused`. Agent started
again (`systemctl start`, active; MainPID 16513 on Debian); no POST in the 15 s before the press. **The press: 1 POST**
`/api/v1/managed-services/scan` (no query: unconditional), 200, 1342 ms / 2341 ms; card checking → not-recent 1.5 s /
1.5 s → "3 / 3". The Agent was left running (also started again by the driver at the end of the window).

**7 Both languages**: every card state pictured in English (Debian desktop) and Turkish (Ubuntu phone): checking
(`stale-load-checking-card.png` EN; `window-checking-card.png` TR), result `3 / 3` (`*-known-card.png`), not recent
(`stale-load-not-recent-card.png`), could not check (`failure-failed-card.png` EN, `failure-agent-back-failed-card.png`
TR), held (`held-held-card.png`). Two Turkish phone card pictures are cut off at the card's top (the card was below the
fold when the element picture was taken): `ubuntu/…/stale-load-checking-card.png` and `failure-failed-card.png`; the
other picture of the same state named here is whole. The "never checked" state was not seen (below).

**8 Components page** (`/services`, one load after the same stale condition, observed 45 s): **1 POST**,
`?max_age_seconds=300`, 200, 1100 ms / 2182 ms. The page's code (`web/src/components/ServiceList.tsx`) is not changed by
a5830dd3e; no build before a5830dd3e was measured in this run, so "unchanged" is a code reading plus this count.

## Product defect found (not fixed)

**D1 — right after a successful check the card says "Not checked in the last 5 minutes" for up to 30 s when the
server's clock is ahead of the browser's.** Where: `web/src/components/Dashboard.tsx:199-204` (`freshScanTimestamp`
returns false for a `scanned_at` later than the page's clock) with `:495` (`setFreshnessNow(Date.now())` after a scan,
whose comment at `:488-494` says it exists so that a successful check does not show Unknown for half a minute) and the
30 s clock at `:410`. Expected (commit message, the comment): the result as soon as the check answers. Observed on both
platforms, every time a scan ran (guest clock 1.05–2.16 s ahead of the browser's): the scan's `scanned_at` was 1.2 s
later than the browser's own receipt time, the card fell to its `stale` state ("Unknown / Not checked in the last 5
minutes. This page checks again by itself while it is open." with the "Scan services" button) for 28.6, 28.3, 25.9 and
1.5 s (Debian) and 25.5, 26.9, 20.0 and 1.5 s (Ubuntu), until the next 30 s tick. It did not occur when the answer came
from the cache with a whole-second `scanned_at` already in the past (held follow-ups). No second POST followed (the
window rule held). A skew of more than 30 s would extend it to the tick after the server's time.

## Observations (not defects of this commit)

- **O1** When the Agent is stopped the card's reason is the generic "The server hit an internal error … check the panel
  logs" (HTTP 500 `INTERNAL`), although the Panel knows the cause (`cmd/panel/service_operations.go:293`, the journal
  line above). The card does name who acts (the owner, the panel logs) and offers the check; it does not say the Agent is
  not reachable.
- **O2** A service install could not be used for item 5: every install runs a scan in its preflight
  (`cmd/panel/service_install_preflight.go:39`), so the stored scan is fresh while the install holds the lease and the
  Dashboard asks nothing. The held state is reachable only when the lease is held by an operation that does not scan
  first (here a service reload) or lasts past the window.

## Not measured, with the reason

- `server_setup_busy`: the setup ran before the browser window opened; no Dashboard load happened during it.
- The "never checked" state (stored scan absent, all components unchecked): the setup scanned before the window (the
  stored scan existed at the window's first probe, 128.5 s / 129.1 s old).
- Arch: not run (time; one lab at a time, about 40 min each). The Debian lab's idle Arch peer was not measured.
- The Components page on a build before a5830dd3e (item 8 is this build only).
- Readiness answer bodies during item 5 (status codes only).
- The scan's server-side wall time: only the browser's request time through the forward (the cache answers took 4–45 ms
  and the 409/500 answers 8–14 ms through the same forward, which bounds that overhead).

## Method, deviations

Build (`build/`): `run-upd1.sh build a5830dd3e` from run copy a (pristine `git archive a5830dd3e`), Go 1.26.5,
20:57Z–21:04Z, `UPD1-ARTIFACTS-OK`; `run-upd1.sh prove` rc 0; dry runs of both cells rc 0. Run copy b = a + the five
set14 files (`harness-run-copy/b/files.sha256`, `harness.diff`); offline suite in copy b: `test_set14_trial` (5) and
`test_set13_trial` (4), 9 tests OK (`harness-run-copy/b/suite.txt`). Labs one at a time (neighbour gate: no qemu, no
earlier run job; disk gate: `Get-PSDrive C` reading under 180 s old and ≥ 40 GiB: 2 decisions, both allowed at 198.79 /
196.97 GiB, `host/c-drive-cells.txt`; 87 readings 21:08Z–22:34Z, lowest 196.84 GiB). Keep-awake 21:08Z–22:35Z. Deviations
from set13: purpose `web` instead of `web_mail` (the card needs installed components, not mail); the setup's components
are three; item 5 uses a lab drop-in on nginx's reload (lab preparation, removed after, `native/020`, `021`, `025`);
the owner actions (Agent stop/start) are `native/013`, `014`, `026`. Every wait was bounded and ran in the WSL host or in
the browser process with a limit (`tools/waitready.sh`, the browser's own limits).

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set14_trial.py`, `guest_set14_native.py`, `run-set14.sh`, `test_set14_trial.py` (5 offline
tests); `web/tools/browser-inspect/cold-load-set14.mjs`. Hashes: `harness-run-copy/b/files.sha256`.

## Secrets, host, leftovers

Redacted at collection (`tools/scrub.py`): host paths and the operator's user and machine names → `<redacted>` in 20
text files (both `node-run.txt`, both `host/job.sh`, the copied tools; the copied `tools/scrub.py` has its own patterns
redacted and does not run as written). The driver's own redaction and set6's digest sweep ran on each staged lab
(0 files changed, `*/run-a/host/digest-sweep-at-staging.json`); driver `SHA256SUMS` verified after staging (126 / 116
files). A text scan found no operator name, machine name, host path, private key, `Set-Cookie` value (shown as
`[REDACTED]`) or password; the lab user `labadmin`, the guest names `dns-debian13`/`dns-ubuntu` and `192.0.2.x` appear
in pictures and records. SSH lab keys are `celikpanel-lab`. Browser profiles (`%TEMP%\cpb\<8 hex>`, 46 characters) were
deleted after the runs. Overlay disks and base-image links of both labs were removed after staging (`host/removals.txt`);
left on the WSL host: `/var/tmp/cp-set14-run`, `/var/tmp/cp-release-drill-s14-d13-a` and `…-s14-ub-a` (records, no
disks), the build `/var/tmp/cp-upd1-build/20261012t205712z` and five new dist folders under `/var/tmp/cp-pair-accept/dist/`
(names in `build/fresh/dist-*.json`). The image cache was used read-only. Commit this folder with `git add -f` (logs
are ignored); `SHA256SUMS` files are written last.

Intake notes (2026-10-13, read-only check of this folder against its raw files before commit). Page text and
screenshots show the host's local time (UTC+3): "Last scan: 13.10.2026 00:41:48" (Debian) and "Son tarama: 13.10.2026
01:26:22" (Ubuntu) are `scanned_at` 21:41:48Z and 22:26:22Z of 2026-10-12. With the Agent stopped (item 6) the other
Dashboard cards (CPU, memory, disk) showed "—" with the amber banner that part of the server's state could not be read,
and `/api/v1/firewall` answered 500 on both guests; this record did not measure those cards. The card texts in
`browser/summary.json` carry a UTF-8/cp1252 mix-up ("componentsâ€¦"); the raw `cardlog.jsonl` is correct. Two
non-scan requests have `status: null` in `requests.jsonl` (Debian `held` `auth/me`, Ubuntu `stale-load`
`panel/availability`): the browser closed before their answer was recorded. The checksum lists were verified: 509 files
listed, 509 present, 0 failures, 7 `SHA256SUMS`.
