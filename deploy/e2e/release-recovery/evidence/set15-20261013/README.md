# set15, 2026-10-13 (UTC): the pre-publication measurement of v0.1.0-alpha.84 (commit 1636b73f0): fresh install on Debian 13 and Ubuntu 24.04, and the owner's update from alpha.83 rebuilt from its tag, good and defective, on Debian 13, in a real headless Chrome

A measurement record. Date: `date +%Y%m%d` style reading on the host (PowerShell `Get-Date`) gave `20261013 11:48:12
+03:00` (08:48:12Z) at the start. Labs and builds ran 09:03Z–13:01Z (`host/progress.txt`). Page text and pictures show
the host's local time (UTC+3); times here are UTC.

Built from `git archive 1636b73f0` (branch `feat/alpha84-dashboard-self-check`; product code last changed in 68b79b372).
`git diff --stat 1636b73f0 HEAD -- cmd internal web/src` and `git status --short -- cmd internal web/src` were empty
(`build/source-check.txt`). HEAD at the start and at staging was aa7dac2b2 (11:56:40+03:00), one commit after the measured
one that changes only `docs/DECISIONS*.md` and `docs/RELEASE-NOTES-v0.1.0-alpha.84*.md` (`host/head-at-staging.txt`). No
product file was changed by this run; nothing was committed or pushed.

The harness addressed no installed server, no licence service and no certificate authority. The release origin's name and
the four certificate-authority names were pinned to each guest's loopback as the first driver step of every lab and read
back at its end (`*/*/driver/steps/01-set6-name-pinning`, `*-name-pinning-at-the-end`: passed in all 6 labs). Nothing else
was blocked: the guests had outbound NAT (QEMU user networking), so the setup installed its components from each
distribution's repositories (no capture). The driver's fixed wording "isolated node" does not mean network isolation.
Every `result.json` carries `native_evidence: false`. It closes no P0 row.

## Builds (`build/`)

Go 1.26.5; `build-set15-artifacts.sh` (new; same steps as `build-set12-artifacts.sh`), test-licence (`acceptance_license`)
builds, none a release:

| Document | Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- | --- |
| update (`build/update/`) | baseline | v0.1.0-alpha.83 / 83 | 0447558e (the tag commit, unpatched; it carries the seam) | `34ca394b24f0ac472f10b81bc73ec962b5fe11cd164ba116befdeaff1627ec26` |
| update | good | v0.1.0-alpha.84 / 84 | **1636b73f0 itself** (no fixture commit; its own policy) | `e3674bed18bdf7f0bbfc64f2b09f7d7ff810f14365d37683ae72cbc145c15bba` |
| update | defective | v0.1.0-alpha.84 / 84 | 9eed8558 (fixture: 1636b73f0 + set5's migrate-only defect in `cmd/panel/main.go`) | `d3c1a158dbc9bb8ac294f98c1033cc5c3e41af15c558184310872be805c5cef2` |
| fresh (`build/fresh/`) | baseline (installed) | v0.1.0-alpha.84 / 84 | 1636b73f0 itself (the same dist, reused by name) | `e3674bed…` (same archive) |
| fresh | good (origin only, never installed) | v0.1.0-alpha.85 / 85 | 2c778b94 (fixture, policy only) | `68f03ecd8695e661d7769813713eec2efa3f45bd628795441580adb26f9d3bc0` |
| fresh | defective (never installed) | v0.1.0-alpha.85 / 85 | f94dae52 | `89e404991dc9700f160d7d94ac4aa702c434d1aa0c3b318bc152ffdff5604ef4` |

The installed candidate and the update's good candidate are one archive. The alpha.83 dist was not in the cache
(`/var/tmp/cp-pair-accept/dist/`); it was built. It is a rebuild, not the published alpha.83 archive. Both documents were
proven (`update-prove.json`, `fresh-prove.json`, rc 0).

**Pins** (`build/archive-pins.txt`, read from the tarballs): the candidate archive's `libexec/get.sh` lines 6–7
`bootstrap_release_sequence=84`, `bootstrap_release_version=v0.1.0-alpha.84` (line 8 the public key digest
`7eadeb0b…`); its `deploy/release-sequence-policy`: `version=v0.1.0-alpha.84 current=84 previous=83
previous_version=v0.1.0-alpha.83 previous_commit=0447558e…`. `install.sh` holds no release literal (line 71
`FIRST_INSTALL_RELEASE_SEQUENCE=${CELIKPANEL_FIRST_INSTALL_SEQUENCE:-}`; same file in both archives). The alpha.83
baseline's get.sh reads 83. Installed copy on every candidate guest (`*/steps/*set15-readings*/installed-pins-*.txt`):
`/usr/libexec/celikpanel/get.sh` lines 6–7 read 84 / v0.1.0-alpha.84 (fresh Debian, fresh Ubuntu, after each good update).

**Schema.** No migration file changed between v0.1.0-alpha.83 and 1636b73f0 (`git diff --stat v0.1.0-alpha.83 1636b73f0 --
cmd internal`: 9 files, none a migration). alpha.83 answered `schema_version: 44` before the update (good run b, steps
09 and 10 records); every ledger reading after the install and after the update: `as-expected`, schema 44, 44 rows, ledger
and schema the pinned released ones (`ledger-set15-*.json`, `post-update-facts`). 44 stays 44.

## Cell × platform

| Cell | Debian 13 | Ubuntu 24.04 | Arch | Raw |
| --- | --- | --- | --- | --- |
| 1 fresh install, first start, sign-in, licence, setup `web` | PASS | PASS | NOT RUN | `debian13/fresh-a`, `ubuntu/fresh-a` |
| 1 version line, pins, Domains (no first-state row), stale Dashboard (one conditional POST), access `valid_for_seconds`, managed-services `scan_age_seconds` | PASS | PASS | NOT RUN | same, `browser/ui/` |
| 1 Components page conditional scan with a stale scan | NOT MEASURED (run a's script loaded Components only right after the Dashboard's scan) | PASS | NOT RUN | `ubuntu/fresh-a/browser/ui/phases.json` |
| 2 update alpha.83 → candidate, good: verification, schema, sidebar | PASS (runs a, b, c) | — | — | `debian13/upd83-good-{a,b,c}` |
| 2 Dashboard after the update checks by itself | PASS (run c) | — | — | `debian13/upd83-good-c/browser/ui/` |
| 3 update, defective → automatic return to alpha.83, Panel answers | PASS (return); terminal step FAILED on the database comparison (below) | — | — | `debian13/upd83-defective-a` |

Arch: not run. Reason: time. The host slept (below) and the run lost about 2 h 15 min; the coordinator asked to close out.

## Facts per cell

**1 fresh (Debian: EN desktop 1440×900; Ubuntu: TR desktop).** Both: install, owner login and licence (test fixture) passed;
setup purpose `web` (nginx, php-fpm, mariadb; DNS external), which waits at `access_dns`. The owner then chose "set up by
hand" before the browser window, as in set14 (`leave_setup`). `GET /api/v1/panel/version` →
`{"version":"v0.1.0-alpha.84","commit":"1636b73f0979…","agent_commit":"1636b73f0979…","agent_matches":true,"schema_version":44}`
(driver and browser). Sidebar text (`*-sidebar.txt`, picture `domains-sidebar.png`): `CelikPanel · v0.1.0-alpha.84 · build
1636b73f0979`. In every picture that shows the rail (Debian fresh, Ubuntu fresh, Debian run c) the 12-character commit wraps inside it mid-word ("163" / "6b73f0979"). This is the
`break-all` stamp of `web/src/components/Layout.tsx:586`; a visual observation, not judged.
Domains: `GET /api/v1/domains` → `[]` (0 rows); page text shows no first-state ("Site configuration files: … recognised")
row. `GET /api/v1/license/access` carries `valid_for_seconds` (driver: Debian 18.348, Ubuntu 37.903; the Dashboard's four
reads 44–60; direct read 27.829 / 28.953) with `can_use_panel: true, state: active, observation: known`. `GET
/api/v1/managed-services` carries `scan_age_seconds` (driver: 128.665 / 129.108).

| | Debian 13 | Ubuntu 24.04 |
| --- | --- | --- |
| stored scan at the Dashboard load (probe / answer) | 317.1 s / 317.671 s | 317.2 s / 317.823 s |
| Dashboard scan POSTs in 90 s | **1**, `POST /api/v1/managed-services/scan?max_age_seconds=300` 200, 1126 ms | **1**, same URL, 200, 1590 ms |
| card sequence | absent 171 ms → "Checking components…" 1145 ms → "3 / 3 running / installed" | absent → "Bileşenler kontrol ediliyor…" 1611 ms → "3 / 3 çalışan / kurulu" |
| "Open Components to rescan" seen | no (sampled at 10 s and about every 5 s) | no |
| Components page right after (scan 90 s old) | 0 POSTs (as designed: `ServiceList.tsx:713-717` asks only when older than 300 s) | 0 POSTs |
| Components page with the scan stale (317 s) | not measured | **1** conditional POST, 200, 2026 ms; no alert, no toast |

**2 update, good (Debian 13).** Baseline alpha.83 installed, set up (purpose `web_mail` by the inherited upd1 setup; DNS
external), one static site `upd1-owner.test` seeded; the owner started the update through the Panel's own update path
(`owner-start`, `track`, `terminal`). Runs a, b and c: final status `phase: succeeded, reason: update_verified,
terminal_proof: update_verified`; build identity of Agent and Panel `version=v0.1.0-alpha.84 commit=1636b73f0…`; floor file
`sequence=84 version=v0.1.0-alpha.84`; `post-update-facts` passed with ledger 44 `as-expected`; the set6 HSTS header
readings before and after passed (`max-age=31536000`, alpha.83's own `cmd/panel/security.go:46`, used as the expectation).
After the update: panel version alpha.84, schema 44; Domains 1 row (`site_config: managed_unchanged`), no first-state row in
the page; sidebar `CelikPanel · v0.1.0-alpha.84 · build 1636b73f0979` (run c, `browser/ui/domains-sidebar.png`). The
Dashboard (run c, stored scan 441 s old, from before the update) sent **1** `POST …/scan?max_age_seconds=300` (200, 1210 ms):
absent → "Checking components…" 1240 ms → "6 / 6 running / installed". The System-services card never showed "Open Components to rescan", but the page text captured in the Checking window
(`browser/ui/dashboard-dashboard-checking.txt`) shows the Mail stacks card with "Status needs refresh" and "Open Components to rescan"
for about 1.2 s, until the scan answered (the script's `rescan_link_seen: false` samples only at about 5 s and 10 s). The
Components page: 0 POSTs right after, then 1 conditional POST (200, 1187 ms) with the scan stale. The database compare at the
terminal step lists `managed_site_files` as differing (the candidate's start writes it; the good path does not fail on it).

**3 update, defective (Debian 13).** Candidate = migrate-only defect; injected fault: reboot at `payload_restored` (the
guest reset once, new boot id, SSH back after 8.4 s). Outcome `recovered-automatically`; final status `phase: recovered,
reason: rollback_verified, previous_failure: update_failed`; two automatic attempts (12:56:57Z update/rollback,
12:58:03Z rollback); build identity after the return `version=v0.1.0-alpha.83 commit=0447558e…` for Agent and Panel. The
Panel was usable after the return: the set6 header reading answered the SPA root 200, an API route 200, 401/403 and 404,
and its version route named alpha.83's build (`20-set6-headers-after-the-update`). Floor after the return: `sequence=84`
(set12 shows the same pattern: floor 83 after its return to 82).
**Not established (recorded, not judged a product defect):** the terminal step FAILED because the Panel database after the
return differs from the pre-update digest in `managed_site_files` (`15-terminal/step.json` `database.unexpected`). The raw
files hold table digests only, not the rows, so which rows or columns changed is not known. set12's alpha.82 baseline had
no such table (ledger 43), so this comparison is new with an alpha.83 baseline. Expected: equal except volatile tables;
observed: `managed_site_files` differs. Because the terminal step failed, `post-return-facts`, this record's readings after the
return and its browser window were not run (`not-run`, needs `terminal`). So no sidebar picture and no Dashboard exist after
the return.

## Runs, harness errors, environment

- `debian13/upd83-good-a` and `-b`: the update measured (both `update_verified`, verdicts passed); their browser windows
  stopped at the first probe. Harness: `guest_set14_native.py` loads `guest_set13…set4_native.py` and
  `guest_request_identity_native.py` on the guest. Run a uploaded none of the chain; run b missed `guest_set4_native.py` and
  `guest_request_identity_native.py`, which set5 uploads only with its item 9 / M10. Fixed in `set15_trial.py` (run copy e;
  a test checks the chain). Run c is the browser measurement.
- `debian13/upd83-good-c`: `verdicts` FAILED "workload interruption outside the expected windows: web, smtp, cron, panel"
  (environment). The host slept during this lab. The disk watcher has no reading 10:19:53Z–12:34:58Z (8105 s).
  `host/sleep-events.txt` shows a power-source change 10:20:32Z, modern standby, sleep entered 11:02:36Z, and the clock
  reset on wake at 12:34:55Z. The keep-awake request did not prevent it. The guest was frozen: its workload samples show
  "not-advancing" for 8050 s, and the Dashboard's reads bound the guest clock 8050 s behind the browser's. The update
  itself (arm 12:35Z → terminal 12:36:51Z) ran after the wake. Runs a and b, without the sleep, passed `verdicts`.
- The browser script gained the stale Components load after Debian fresh run a (run copy c). The drivers that ran: copy b
  (fresh labs and good run a), copy d (good run b), copy e (good run c, defective). Hashes:
  `harness-run-copy/{b,c,d,e}/files.sha256`, diffs `harness.diff`. Offline suite: b 19 tests OK, d and e 20 tests OK.
- Inherited from the upd1/set6 update cells and not this record's subject: the deferred-mail watch, workload windows and
  the database compare. set4/set5's PHP-site (item 9) and M10 mail sections were switched off (measured in set4/set5).

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set15_fixture.py` (alpha.83 and alpha.84 profiles, generated origin with the 83/84/85
transitions, the (alpha.83, alpha.84) bound-worker pair, ledger 44/44, alpha.83's HSTS), `set15_trial.py`,
`build-set15-artifacts.sh`, `run-set15.sh`, `test_set15_trial.py` (8 tests); `web/tools/browser-inspect/cold-load-set15.mjs`.
Generated copies: `build/generated-set15/`. Run tools: `tools/`.

## Secrets, host, leftovers

Redaction at collection: the driver's own redactor (owner password, fixture key, cookies → `[REDACTED]`), set6's digest
sweep per lab (`*/host/digest-sweep-at-staging.json`; one base64 run replaced in the defective lab's
`host/recovery-fault-collection-*.json`), the browser script's redaction list. `tools/scrub.py` then replaced host paths
and the operator's user and machine names with `<redacted>` in 46 text files (`host/scrubbed-files.txt`). Its own two
patterns were replaced in the staged copy. A text search afterwards finds neither name and no host path. SSH key
comments are `celikpanel-lab`. Pictures show the lab owner `labadmin`, lab hostnames and 192.0.2.x addresses only. No CR
byte in any text file. `host/keepawake.log` matches `.gitignore` (`*.log`): commit with `git add -f`.
Disk: `Get-PSDrive C` 216.12 GiB free at the start, lowest 189.66 GiB (`host/c-drive-watch.txt`, one reading a minute; the guest-start gates read 189 to 201 GiB); every guest start gate
allowed (`host/c-drive-cells.txt`, `host/neighbour-gate.txt`). Overlay disks and base-image links of all 6 labs removed
after staging (`host/removals.txt`). Browser profiles under `%TEMP%\cpb\` (d21cbbd9, 4a5a47d8, 1e801db8) deleted. Nothing of this run runs: no qemu, no job,
no node script (`host/left.txt`); two headless Chrome processes started 2026-10-11 by an earlier session are not this run's
and were left alone. Left under `/var/tmp` (`host/left.txt`): `cp-set15-run` (322 MB), six `cp-release-drill-s15-*`
(records and the small cloud-init `seed.iso` files, no disks, 65–67 MB each), the builds `cp-upd1-build/20261013t090322z`
and `…090641z` (1.4 GB each), and five new dist folders under `cp-pair-accept/dist/` (0447558e, 1636b73f, 9eed8558,
2c778b94, f94dae52). The image cache was only read.


## Intake notes (2026-10-13)

Checked against the raw files before the commit. Corrected above: the sidebar wrap (all pictures, not only Debian), the lowest
disk reading (189.66 GiB, not 194.7), run c's "Open Components to rescan" (seen in the Mail stacks card during the Checking
window), and the step numbers that hold the alpha.83 schema reading (09 and 10, not 02/04). One host path (the repository folder
in `tools/browser.sh`) was replaced with `<redacted>`.

- **Date reading.** The `20261013 11:48:12 +03:00` (08:48:12Z) reading was taken with PowerShell `Get-Date` before the run
  directory was made; no raw file preserves the reading itself. The first preserved timestamps are `host/c-drive-watch.txt`
  09:01:48Z and `host/progress.txt` 09:02:01Z (12:02:01 local). Pictures show local time (UTC+3), for example "Last scan:
  13.10.2026 12:20:04" is 09:20:04Z.
- **Host sleep and run c.** `host/sleep-events.txt`: modern standby from 10:20:33Z, sleep entered 11:02:36Z, the system clock
  moved from 11:02:39Z to 12:34:55Z on wake; the disk watcher has no reading 10:19:53Z-12:34:58Z. Run c's guest timestamps
  (scan 10:17:12Z, ledger 10:23:59Z, `valid_until_iso` 10:25Z) are on the guest's own clock, 8050 s behind the host's; its
  `old_rule_accepts_at_receipt: false` comes from that offset, not from a product reading. Run c's four Dashboard access
  reads were 27.5, 59.998, 59.982 and 57.492 s (the fresh cells' 44-60 s does not apply to it). Its `verdicts` step failed with
  "workload interruption outside the expected windows: web, smtp, cron, panel" (cron "not-advancing" 8050.5-8110.5 s; the
  Panel verdict is `down-outside-transaction`: a failed window of 35-45 s classed "unexplained").
- **Not run / not measured.** Arch was not run. The stale Components load on Debian fresh was not measured (Ubuntu and run c
  have it). In the defective cell `post-return-facts`, the readings after the return and the browser window were not run:
  they need a passed `terminal` step, and `terminal` failed on the database comparison below.
- **Open finding: `managed_site_files`, cause not established.** `debian13/upd83-defective-a/driver/steps/15-terminal/step.json`,
  `checks.database`: `unexpected: ["managed_site_files"]`, `differing: ["managed_site_files", "metrics_samples",
  "server_setup_executions", "sqlite_sequence"]` (the last three are in `volatile_excluded`), `schema_equal: true`,
  `tables_compared: 67`, `verdict: "different"`, whole-database digest before `fc556b00...` and after `abe40717...`. The table
  digests in `guest-observation-pre-update.json` and `guest-observation-post-recovery.json`: `managed_site_files` 1 row and 21
  columns both times, digest `ffe9494c...` before and `e8f224c4...` after; no other table outside the volatile list differs.
  The three good cells show the same `unexpected` entry and pass because the update path does not require equality. Likely cause
  (from the source, not confirmed from the rows, which the raw files do not hold): the Panel rewrites each row's `state`,
  `state_reason`, `file_sha256` and `observed_at` whenever it observes the site files at start
  (`internal/services/site_file_ledger.go` lines 263-273 upsert with `observed_at = excluded.observed_at`, 297-300, 327, 391,
  426; the column is `internal/db/migrations/044_managed_site_files.sql` line 31). After the return, alpha.83 starts and observes
  again, so `observed_at` changes and the table's digest differs from the pre-update copy; the driver's check treats the table as
  non-volatile because it did not exist in the alpha.82 baseline of set12/set13. The driver's check must exclude `observed_at` (or
  the table) in the next record; the product-level facts of the cell, the return happened and the panel answered as alpha.83
  afterwards, stand.
- **Archives.** Every archive here is a rebuild with the test-licence tag (`release: false`), none a published release asset;
  the alpha.83 archive is a rebuild of the tag commit, not the published one. The two alpha.85-labelled builds (good origin
  2c778b94, defective f94dae52) were origin documents only; every installed guest answered alpha.84 and neither was installed on any guest.
- **Left as found.** After the return the floor file reads `sequence=84` and the recovery foundation file still names the
  defective candidate (`release-commit=9eed8558...`) while the running build is alpha.83; recorded, not judged.
