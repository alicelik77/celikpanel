# set13, 2026-10-11 (UTC): commit 816b8dd60 measured on disposable Debian 13 and Ubuntu 24.04 guests: a put-back is reloaded only when the service's own check accepts it, an unread pool reaches the Domains row, the application unit's PATH and `/bin/sh -c`, and the refused-take answer in view in a real Chrome

A measurement record. Built from `git archive 816b8dd60` (branch `fix/alpha83-known-state-gates`); `git diff --stat
816b8dd60 HEAD -- cmd internal web/src` was empty when the build was made (`build/source-check.txt`, HEAD was
816b8dd60). The six commits made after 816b8dd60 are NOT in the measured build: four touch only documents and the
set12 record (dbb6d0067 09:29:21Z, 9392cf6a7 09:29:54Z, 444155d95 09:30:10Z, 095f5c332 09:32:59Z; all after the build
began at 09:28:09Z), and 5ac273b00 (09:48:28Z; changes
`cmd/agent/app_rpc.go`, `cmd/panel/site_config_files.go`, `web/src/i18n/screens/server/*.ts`, `web/src/lib/apiError.ts`:
"service sentences say how work resumes …; a verify timeout is unknown") and 73411f9e2 (10:20:28Z, tests only); HEAD at
staging is 73411f9e2 (`host/head-at-staging.txt`). 5ac273b00 also committed this run's browser script
`web/tools/browser-inspect/cold-load-set13.mjs` (its committed bytes hash to `ec4bfd68…` at intake; this directory
holds no hash of the copy the run used, so "the same bytes" is the staging session's statement); this run committed
nothing. The measured archive is the disposable fixture commit 8948352e3953 (tree `6c3c1d24…`, labelled
v0.1.0-alpha.81, unpublished, only in the lab clone): `build/fresh/trees.txt` lists one file that differs from
816b8dd60's tree (`228eff9a…`, `build/source.txt`), `deploy/release-sequence-policy`; the panel's footer shows
`v0.1.0-alpha.81 build 8948352e3953`. The sentences quoted below are 816b8dd60's. Nothing of the product was changed by this run; nothing was pushed.
Labs 09:36Z–10:56Z; build 09:28Z; disk watcher and keep-awake 09:27Z–11:17Z; secret scan 11:16Z.

The harness addressed no installed server, no licence service and no certificate authority. The release origin's
name and the four certificate-authority names (`celikpanel.net`, `acme-v02.api.letsencrypt.org`,
`acme-staging-v02.api.letsencrypt.org`, `acme.zerossl.com`, `dv.acme-v02.api.pki.goog`) were pinned to each guest's
loopback as the first driver step of every lab and read back at its end (`*/*/driver/steps/01-set6-name-pinning`,
`*-name-pinning-at-the-end`, all 8 passed). Nothing else was blocked: the guests had outbound NAT (QEMU user
networking; `*/*/host/fixture-plan.json`, `qemu-user-nat`), so outbound contact happened: (a) the Panel's setup
installed its components with each guest's package manager from the distribution's repositories; (b) the Panel's own
Node.js install (`POST /api/v1/runtimes/node`, 22.20.0, answered 202 and succeeded) fetched Node.js from nodejs.org by
the product code. The raw files hold no capture of that traffic. No certificate request was made in this run. Every
`result.json` carries `native_evidence: false`. It closes no P0 row.

## Cell × platform

Verdict against the task's expectation. Raw = `<platform>/<run>/driver/steps/<NN-name>/section.json` (`cells.<C>.expectations`
holds every expectation with what was measured; `native/` the guest readings, `calls` the raw requests and answers).
`cells.md` lists every driver verdict and this record's re-judgements; `checks-not-passed.txt` every check and step
not passed, with its class.

| Cell | Debian 13 | Ubuntu 24.04 | Arch | Raw |
| --- | --- | --- | --- | --- |
| 1 pool take refused by `php-fpm -t` (owner's `zz-set12-owner.conf`, exit 78) | PASS | PASS | NOT-MEASURED | `debian13/fresh-a/…/11-c1-pool-check-refused`, `ubuntu/fresh-a/…/11-c1-…` |
| 2 pool take, PHP-FPM stopped by the owner | PASS | PASS | NOT-MEASURED | `…/12-c2-pool-php-stopped` (both) |
| 3 vhost take refused by nginx: (a) include-dir conflict → `reloaded`; (b) owner's `conf.d` file nginx refuses → `not_reloaded` | PASS | PASS | NOT-MEASURED | `debian13/fresh-a/…/13-c3-…`, `ubuntu/fresh-a/…/15-c3-…` |
| 4 unread pool reaches the Domains row | PASS (run b) | PASS (run b) | NOT-MEASURED | `debian13/fresh-b/…/11-c4-pool-not-read`, `ubuntu/fresh-b/…/11-c4-…` |
| 5 Node unit: PATH, `/bin/sh -c`, runs; verify before `try-restart` after a put-back; `check_unavailable` | PASS (run b) | PASS | NOT-MEASURED | `debian13/fresh-b/…/12-c5-app-unit`, `ubuntu/fresh-a/…/14-c5-…` |
| 6 browser: refused-take banner in view (`role=status`, box inside the viewport), EN desktop + TR phone | PASS (runs a and b) | — | — | `debian13/fresh-{a,b}/browser/ui/refused-*` |
| 6 browser: Domains badge "could not be read", EN/TR desktop/phone; gone after the page read | PASS (run b) | — | — | `debian13/fresh-b/browser/ui/unread-*`, `reread-*` |

No product FAIL. Arch: not run (time; the Debian labs' layout also starts an idle Arch peer guest, which was not
measured). Debian run a's C4 and C5 and Ubuntu run a's C4 are NOT-MEASURED (harness, below); each was measured again in
a new lab (run b).

## Facts per cell (Debian; Ubuntu equal unless said)

- **1.** The owner's pool `site2.conf` edited (comment + directive, reloaded), then the owner's `zz-set12-owner.conf` in
  the pool directory: `php-fpm8.4 -t` exit 78 (`unknown entry 'set12_not_a_directive'`), no reload by the owner.
  `POST …/site-config/take {"kind":"php_pool"}` → `502 {"code":"SITE_CONFIG_SERVICE_REFUSED","reason":"php_pool",
  "detail":"config_refused","restore":"confirmed","service":"not_reloaded","service_reason":"config_refused_elsewhere",
  "service_active":"true","vars":{"line":"php-fpm8.4 -t: [11-Oct-2026 09:50:05] ERROR: [/etc/php/8.4/fpm/pool.d/zz-set12-owner.conf:3]
  unknown entry 'set12_not_a_directive'"}}`, the line also first in `details` and inside the sentence ("The previous file
  is back. PHP-FPM was not reloaded, because its configuration is refused by another file — … Fix what the check names
  (…), then reload PHP-FPM yourself."), 0.26 s. 12–13 s after the take (`native/022-c1-1-after-pool-*.json`): `php8.4-fpm` `active`, MainPID 24100 and
  NRestarts 0 unchanged (Ubuntu 33016/0), and no Reloading/Reloaded, "Main process exited", "Failed with result" or
  start-limit line for it after the take (the driver's expectation, recorded as `[]`). The journal window
  (`native/023-c1-journal.json`) is not empty on Debian: it holds two Reloading/Reloaded pairs at 09:50:04 (the lab's
  pool restore and the owner's pool edit), before the take began (about 09:50:05.8); Ubuntu's window holds no PHP-FPM
  line. The owner's pool same sha256/inode/mode/owner. The owner then
  removed their file (no reload; `php-fpm -t` 0) and took again → `200 taken`, CelikPanel's text with a valid header,
  journal `Reloading/Reloaded php8.4-fpm.service`, active. That 200 answer carries no `service` field (documented on
  the refusals only), so "`service: reloaded`" after the fix is measured as the journal's reload, not as a field.
- **2.** Owner `systemctl stop php8.4-fpm`; take → `502 … "detail":"reload_failed","restore":"confirmed",
  "service":"not_running","service_active":"false"`, sentence "The previous file is back. PHP-FPM was not running
  before this change, so CelikPanel neither reloaded nor started it. Start PHP-FPM yourself, then choose again."
  PHP-FPM stayed `inactive`; the only line about it after the take is one "Unit cannot be reloaded because it is
  inactive" (listed twice: the unit's and systemd's journal views hold the same line; Debian 09:50:26.969, before the
  Agent's own "failed" line at 09:50:26.996), read as the take's reload attempt of CelikPanel's text (an inference
  from the timing: the journal line names no caller); nothing after it. The owner's pool byte-equal.
- **3a.** Owner's replaced vhost + `set10-dup-root.conf` in the site's include directory (passes with the owner's
  vhost). Take → `502 SITE_CONFIG_NGINX_REFUSED "restore":"confirmed","service":"reloaded","service_active":"true"`;
  journal `Reloading/Reloaded nginx.service` after the take; nginx active, `nginx -t` ok; the owner's vhost byte-equal
  and served (`owner-replaced`). **3b.** Owner's `/etc/nginx/conf.d/zz-set13-owner.conf` (`set13_not_a_directive on;`,
  `nginx -t` exit 1, no reload). Take → `502 SITE_CONFIG_NGINX_REFUSED "restore":"confirmed","service":"not_reloaded",
  "service_reason":"config_refused_elsewhere","service_active":"true"`, `vars.line` = nginx's `[emerg] unknown directive
  "set13_not_a_directive" in /etc/nginx/conf.d/zz-set13-owner.conf:2` (shortened here: the raw line starts with
  the time and `<pid>#<pid>: `, e.g. Debian `2026/10/11 09:50:42 [emerg] 33271#33271: …`; `details` holds two such lines
  with different pids, and `vars.line` is the second one); no reload line, nginx active
  with the same MainPID (Debian 7351, Ubuntu 12580), NRestarts 0; the owner's vhost byte-equal and served.
- **4** (runs b). An answered page read first (pool `managed_unchanged`, row managed). Lab drop-in: PHP-FPM's reload
  = `/bin/sleep 75`; B's pool save (200 after 75.1–75.3 s) held the Agent's pool lock; `GET /api/v1/domains/2/site-config`
  answered after 60.0 s with the vhost `managed_unchanged` and the pool `unknown`, reason `not_read`, `read_error:
  "timeout"`, actions `["recheck"]`, `overall_state unknown`, `overall_kind php_pool`; journal "php_pool: could not read
  its state: context deadline exceeded" and "recorded as not read (not_read timeout)". The Domains row then:
  `{"state":"unknown","kind":"php_pool","reason":"not_read","read_error":"timeout"}`, also after B's save answered (no
  read of A in between); ledger row `unknown`, `state_reason "not_read:timeout"`, `file_sha256` unchanged (68b9a583… on
  Debian). The next page read answered the pool `managed_unchanged`; the row and the ledger row back to managed.
- **5.** Node.js 22.20.0 installed through the Panel; site N with the start command `node -e "…"` (bare `node`). Unit
  `celikapp-4.service`: `Environment=PATH=/opt/celikpanel/runtimes/node/22.20.0/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`,
  `ExecStart=/bin/sh -c 'node -e …'`; active, NRestarts 0, ExecMainStatus 0, no "not found"/127 line, the app answers
  `set13-n1` on 127.0.0.1:3001; the harness's own `systemd-analyze verify` of that unit exit 0. Lab: logging shims
  `/usr/local/sbin/systemctl` and `/usr/local/sbin/systemd-analyze` (the Agent's PATH starts with `/usr/local/sbin`;
  each call logged with the caller's PID, then the real program). (b) Owner line in the unit, take → `200 taken`; the
  Agent called `is-active`, `daemon-reload`, `try-restart` — no `systemd-analyze` (verify is documented for a put-back
  only); MainPID changed. (c) To reach a put-back the lab made the Agent's next `daemon-reload` fail once (the only
  trigger this harness has: the unit's take has no other check); owner line, take → `502 SITE_CONFIG_SERVICE_REFUSED
  app_unit "restore":"confirmed","service":"reloaded","service_active":"true"`; call order: daemon-reload (injected
  failure), daemon-reload, `systemd-analyze verify /etc/systemd/system/celikapp-4.service` rc 0, `try-restart`; the
  owner's unit back byte-equal, MainPID changed. (d) CelikPanel's text cannot be made to fail verify here, because verify
  runs on the put-back (owner's) unit: the owner added `ExecStartPre=/opt/set13-owner/missing-prepare` and ran
  `daemon-reload` without restarting (harness verify exit 1: "Command … is not executable"). Take (injected failure) →
  `502 … "service":"not_reloaded","service_reason":"config_refused_elsewhere","service_active":"true"`, `vars.line`
  "systemd-analyze verify: celikapp-4.service: Command /opt/set13-owner/missing-prepare is not executable: No such file
  or directory: exit status 1"; verify rc 1, no `try-restart`; MainPID and NRestarts unchanged, active. (e) The owner
  removed that line; the lab bound an empty non-executable file over `/usr/bin/systemd-analyze` (not executable as the
  Agent sees it) and removed the analyze shim; take (injected failure) → `"service":"not_reloaded","service_reason":
  "check_unavailable","service_active":"true"`, sentence "… because this server has no systemd-analyze to check the unit
  first …"; no verify call, no `try-restart`, MainPID unchanged. Bind and shims removed; a last take → 200, app answers.
- **6.** Chrome 154.0.8037.98 headless, profile `%TEMP%\cpb\<8 hex>` (46 characters, `browser-profile-*.json`). Pool
  edited + the owner's refused pool file; "Replace the file" → confirm: `[data-site-config-refusal]` `role="status"`,
  `restore="confirmed"`, box top 56 / bottom 171 in a 1440×900 viewport (EN desktop) and top 56 / bottom 379 in 390×844
  (TR phone), window scrollY 0 — the banner is the first thing in the viewport picture (`refused-*-2-after-click-viewport.png`;
  the page's own header is scrolled out of that picture, so a container scrolled, not the window);
  PHP-FPM active, same MainPID after each click (`browser/handover/prep-done-php-state-*`, in run a the second click's
  state is the `broken-remove` record because that run's second `php-state` preparation failed). Unread pool (prep as cell 4): the Domains row of set12-a.test shows
  "A configuration file could not be read; state not known" / "Bir yapılandırma dosyası okunamadı; durumu bilinmiyor"
  (badge text in `unread-*-domains.txt`, API row `kind php_pool, reason not_read, read_error timeout` in the `u-unread`
  and `rows` preparation records; the `data-site-config-badge` attribute is read by `tools/browser-body.mjs` but no event
  records it), EN/TR, desktop/phone (`unread-*-domains-viewport.png`); in run b, after the Configuration file page was
  opened the row had no badge and the API row read `managed_unchanged` (`debian13/fresh-b/browser/ui/reread-*`). In run
  a that picture still shows "Configuration file: your choice needed" on set12-a.test (the vhost observation below)
  and also on set13-n.test (its application-unit row `owner_edited`, left by run a's unfinished C5). No typed password
  in any frame (`password_fields_with_text` 0 in every `shot` event, 10 per run; no sign-in frame was taken, the
  first frame is the site's Configuration file page after sign-in).

## Observations (not cells)

- **A vhost's Domains row stays "your choice needed" after a refused take, until the next write.** After 3a/3b the
  vhost's ledger row was `foreign / nginx_refused` with the owner's sha256. The lab then wrote CelikPanel's text back
  (as an owner restoring it by hand): the page answered the vhost `managed_unchanged`, while the Domains row stayed
  `{"state":"foreign"}` ("Configuration file: your choice needed") through later page reads (Debian run a: `14-c4-…`
  rows and the browser's `unread-*`/`reread-*` pictures; Ubuntu run a: `15-c3-…` section `observation_vhost_row_after_refused_take`).
  An unchanged General settings save (a write) made it `managed_unchanged` (Ubuntu run a). Read from the code (816b8dd60), not
  measured: the re-read path for a vhost row (`cmd/panel/site_config.go:498`, `services.RecordSiteFileRecheck`,
  `internal/services/site_file_ledger.go:316`) updates only a row that is `unreadable / write_refused`. The pool's row
  after a refused take (`owner_edited / reload_failed`) was replaced by the next answered read (Debian run a,
  `13-c3-…/native/041` → `043-guest-clock.json`; Ubuntu run a, `13-c4-…/native/041-c4-during-ledger.json` still
  `owner_edited / reload_failed` during the hold → `044-c4-after-ledger.json` `managed_unchanged`). Ubuntu run b had no
  refused take, so its precondition read shows nothing about this.
- The Debian labs (`lab.py` platform `debian13-arch`) run two guests, the measured Debian node and an idle Arch peer.

## NOT-MEASURED, with the reason

- Arch, all cells: not run (time).
- Debian run a, C4: precondition not met — C3 ran before C4 in that order and left the vhost row `foreign` (above); the
  Domains row shows the domain's worst file, so it showed `foreign`; the page, the ledger row (`unknown`,
  `not_read:timeout`) and the journal held. Ubuntu run a, C4: no answered read between C2's refused take and the hold
  after the lab's put-back, so the pool's row still held `owner_edited / reload_failed`, which an unread observation
  does not replace (documented). Both measured again (runs b: an answered read first; C3 last).
- Debian run a, C5: harness error in (e) (the harness's own `systemd-analyze` ran while the program was hidden,
  PermissionError, before the take); (a)–(d) are in the raw files with the same results as run b; measured in run b.
- Debian run a, browser badge part: masked by the vhost row above; measured in run b. A second `php-state` preparation
  of run a failed on a file-name clash (harness), not used.

## Method, runs, deviations

Build (`build/`): `run-upd1.sh build 816b8dd60` (B labelled `v0.1.0-alpha.81 / 81`, acceptance-test licence build,
commit 8948352e3953, archive `f7e5c60c…`); proven (`fresh-prove.json`), dry runs without a lab. Go 1.26.5. Guests:
Debian 13 (PHP 8.4), Ubuntu 24.04 (PHP 8.3). Runs: `debian13/fresh-a` (all cells + browser), `ubuntu/fresh-a` (all
cells), `ubuntu/fresh-b` (C4), `debian13/fresh-b` (C4, C5 + browser); one lab at a time (neighbour gate, `host/neighbour-gate.txt`).
Run copies (`harness-run-copy/`): a pristine; b = a + the four set13 harness files (Debian a); c = b with the test
fixed (suite only); d = order C4, C5, C3, (e) without the harness verify, unique prep labels (Ubuntu a); e = d + an
answered read before the hold (Ubuntu b, Debian b). Offline suite: a 1053 tests, 5 errors; c 1057 tests, the same 5
errors by name (`git` calls against commits a run copy without history does not hold). Copy b's suite had a sixth
error (`test_set12_trial.Set12Tests.test_origin_copy_adds_exactly_one_transition`, `KeyError:
'upd1_worker_fixture_origin'`, `harness-run-copy/b/suite.txt:199-205`): `test_set13_trial` imported `set12_fixture` at
collection time (so `test_set13_trial.py:3` says), which (no `SET12_RUN_ROOT` set) rewrote
`/var/tmp/cp-set12-run/generated/worker_fixture_origin.{py,json}` (set12's leftover run folder), the time 09:37:25Z
being written only in the comment of `tools/fix-generated.sh:3` (no record holds the files' time or content before the
rewrite); both files were written back at 09:48:30Z (the listing in `host/set12-generated-restore.txt` shows 12:48:30-31
+0300) from set12's staged copies (`set12-20261011/build/generated/`, sha256 of the `.py`
`8dcc0b19…` = the recorded `generated_sha256`; `host/set12-generated-restore.txt` lines 7-9 hold the SHA-256 of the
restored `.py` (`8dcc0b19…`) and `.json` (`6648cd85…`), equal to the staged copies' `set12-20261011/SHA256SUMS`
lines 930-931 and to `sha256sum` of those copies at intake). A mis-quoted inline `wsl` command (a `$` passed inline,
against the run's own rule) created an empty `/.set13-restore` in the WSL root at 09:47:58Z (the time is written in
`tools/rm-stray.sh` and in the first line of `host/set12-generated-restore.txt`, which quotes the same script); it was
removed, and the later listing in that file shows no such file at 09:48:30Z.

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set13_trial.py`, `guest_set13_native.py`, `run-set13.sh`, `test_set13_trial.py` (4 offline
tests); `web/tools/browser-inspect/cold-load-set13.mjs` (committed by 5ac273b00, not by this run). Hashes:
`harness-run-copy/e/files.sha256` (= the working tree at staging).

## Secrets, host, leftovers

`secret-scan.txt`: no unexplained hit; operator user and machine names 0; the 2 informational path matches are the
scanner's own pattern text (`tools/secretscan.py`). SSH lab keys `celikpanel-lab`. Host/tools/browser paths and names
scrubbed at staging (`host/scrubbed-files.txt`); driver files unchanged (their `SHA256SUMS` verified after the digest
sweep, `*/*/host/digest-sweep-at-staging.json`, 0 changes). Disk: `Get-PSDrive C` every 30 s, 220 readings
09:27Z–11:17Z, 61.52 GiB at the start, lowest 47.60 GiB (11:15Z), 47.61 last; 4 gate decisions, all allowed
(`host/c-drive-cells.txt`). Power: keep-awake 09:27Z–11:17Z (`host/keepawake.log`); the System log shows modern standby
entered 08:30:29Z, left 09:42:56Z, entered 10:06:14Z, left 11:09:46Z, entered 11:15:29Z (`host/sleep-events.txt`), so
the standby state covered 09:27Z–09:43Z and 10:06Z–11:10Z of the run, while the disk watcher read every 30–31 s with no
larger gap (`host/c-drive-watch.txt`: 220 readings, gaps 30-31 s) and the labs progressed: the host was running.
Overlay disks of the four labs removed after staging
(`host/removals.txt`). Left on the WSL host: `/var/tmp/cp-set13-run` (396 MB), four `/var/tmp/cp-release-drill-s13-*`
(records, no disks, about 67 MB each), the build `/var/tmp/cp-upd1-build/20261011t092810z` (1.3 GB) and five new dist
folders under `/var/tmp/cp-pair-accept/dist/` (the folder names are in `build/fresh/*.json`; the sizes and the
listing are the staging session's statement, no file here records them). The two browser profiles
(`%TEMP%\cpb\b5238dc6`, `…\ead0d739`) were deleted at staging, before this README was written (11:20Z); `%TEMP%\cpb`
was found empty at intake (11:30Z). That nothing of this run was running at 11:17Z (no qemu, no set13 job, watchers
stopped) is the staging session's statement: `host/progress.txt` ends at `start stage2` 10:57:07Z, and the watcher and
keep-awake logs end at 11:17:18Z and 11:17:59Z.
The two `*.log` files (`host/browser-runner.log`, `host/keepawake.log`) match `.gitignore`: commit this folder with
`git add -f`. `SHA256SUMS` is written last.

## Corrections after intake (2026-10-11)

Intake of this directory before it is committed to the public repository (read-only reading of every list, a text
scan of every file, every distinct picture looked at, the README and `cells.md` compared with the raw files; edits only
inside this directory; no product, harness or other record changed). Result: no secret, no operator name and no
scratch or checkout path found; no raw file contradicts a cell verdict; no product FAIL. The narrowings below are
sentences of this README that said more than a raw file shows. Nothing in the raw files was changed.

Checked and held (raw file in brackets):

- Sums: the root list holds the 1563 other files, the four run lists (416, 311, 364, 209 lines) hold every file of
  their `driver/` folders; 0 failures, 0 unlisted, 0 absent. No text file carries CRLF (lists, `tools/` and the two
  `*.log` files included; nothing was converted); the only CR LF byte pairs are inside the PNG files.
- Pictures: 34 files, 16 distinct contents (each `-full` equals its `-viewport`); all 16 looked at. They show the lab's
  panel after sign-in (`labadmin`, `dns-debian13`, `192.0.2.10`); no password, key, token, installed-server name or
  address. No sign-in frame exists.
- Cells: the answers (fields, codes, `vars.line`), the `systemctl show` facts (MainPID 24100/33016 for PHP-FPM, 7351/12580
  for nginx, NRestarts 0, 60.0 s read, 75.1-75.3 s save), the shims' call records (`is-active`, `daemon-reload`,
  `try-restart`, `systemd-analyze verify` with rc 0/1), the byte-equal owner files, the unit's PATH and `ExecStart`, and
  the browser boxes (top 56, bottom 171 / 379, `role=status`, profile 46 characters) agree with the raw files. The
  shims record only call arguments and exit codes. A successful take's answer has no `service` field in any of the 7
  `200` answers; "reloaded" after a fix was read from the journal (stated in the cell 1 text).
- The observation is confirmed from raw files: after the refused vhost takes the ledger row is `foreign / nginx_refused`
  with the owner's digest `b08051…`, the page answers the vhost `managed_unchanged`, and the Domains row stays
  `foreign` through three list reads and a later page read: `debian13/fresh-a/driver/steps/14-c4-pool-not-read/native/064-c4-during-ledger.json`
  and `067-c4-after-ledger.json` (row id 2), `api/0153`, `0155`, `0157-get-api-v1-domains.json`; and in
  `ubuntu/fresh-a/driver/steps/15-c3-vhost-nginx-refused/section.json`, key `observation_vhost_row_after_refused_take`
  (`foreign` before, `managed_unchanged` after an unchanged General settings save).
- Host: the standby intervals, the 220 disk readings (gaps 30-31 s, lowest 47.60 GiB at 11:15:18Z) and the four gate
  decisions agree with `host/`; no file is over 1 MB (16 MB in all); no binary other than the 34 PNG files; dates are
  2026-10-11 only.

Sentences narrowed (old to new, in the sections named):

1. Cell 1, journal: "no Reloading/Reloaded ... line for it in the journal window (`native/*-c1-journal.json`)" is now
   "... after the take (the driver's expectation, recorded as `[]`)" with the window's two Debian Reloading/Reloaded
   pairs at 09:50:04 named (made by the lab before the take); "After 12 s" is now "12-13 s after the take".
2. Cell 2: "i.e. the take's own reload of CelikPanel's text, before the put-back" is now an inference from timing (the
   journal line names no caller; it precedes the Agent's "failed" line by 27 ms).
3. Cell 3b: `vars.line` was quoted without the nginx time and pid prefix; the prefix and the two-line `details` are now stated.
4. Cell 6: "after the Configuration file page was opened the row had no badge" is now limited to run b; run a's
   `reread-en-desktop-2-domains` still shows "Configuration file: your choice needed" on set12-a.test, and also on
   set13-n.test (its application-unit ledger row `owner_edited` in `15-c5-app-unit/native/074`..`104`, left because run
   a's C5 stopped before its last take). "`password_fields_with_text` 0 in every event" is now "every `shot` event, 10
   per run". `data-site-config-badge` was read by the script but is recorded in no event; the badge is evidenced by the
   text files and the API rows. "same MainPID after each click" now says where run a's second reading is.
5. Observation: "Ubuntu run b's precondition read" replaced the pool-row evidence with Ubuntu run a's `041` to `044`
   ledger files (Ubuntu run b had no refused take); the code reading now carries file and line (816b8dd60:
   `cmd/panel/site_config.go:498`, `internal/services/site_file_ledger.go:316`).
6. Header: the later commits were two; they are six (dbb6d0067, 9392cf6a7, 444155d95, 095f5c332 touch only documents
   and the set12 record). The measured archive is the fixture commit 8948352e3953 (tree 6c3c1d24...), which differs from
   816b8dd60's tree 228eff9a... in one file, `deploy/release-sequence-policy` (`build/fresh/trees.txt`); this is now
   stated. "The same bytes the run used" for `cold-load-set13.mjs` is now the staging session's statement: the
   committed bytes hash to `ec4bfd68...` at intake, but no file here holds the hash of the run's copy.
7. Harness mistakes: the sixth suite error is named (`harness-run-copy/b/suite.txt:199-205`); the times 09:37:25Z and
   09:47:58Z are stated to come from comments in `tools/fix-generated.sh`, `tools/rm-stray.sh` and the first line of
   `host/set12-generated-restore.txt`, not from a recorded mtime; the restored files' SHA-256 (`8dcc0b19...` for the `.py`,
   `6648cd85...` for the `.json`) are shown in `host/set12-generated-restore.txt` lines 7-9 and equal the staged copies
   in `set12-20261011/SHA256SUMS` lines 930-931 and a `sha256sum` of those copies at intake.
8. Leftovers: "deleted at staging, before 11:25Z" named a time after the README was written (11:20Z); it now says before
   11:20Z, and that `%TEMP%\cpb` was found empty at 11:30Z. The folder sizes left on the WSL host, and "nothing was
   running at 11:17Z", are marked as the staging session's statements: `host/progress.txt` ends at `start stage2`.

Still not shown by this record (unchanged, listed so they are not read as proven):

- The outbound traffic (package repositories, nodejs.org) is stated, not captured; `nodejs.org` is named only here, in
  no raw file. Nothing blocked them but the five pinned names.
- Arch was not run; Debian run a's C4 and C5 (e) and Ubuntu run a's C4 are NOT-MEASURED and were measured again in run b.
  Steps with verdict `observed` (07-setup, the browser windows) or `skipped` (the Ubuntu browser windows) are not
  failures and are not in `checks-not-passed.txt`.
- The observation is a product finding that stays open; the code reading of the recheck path is not a measurement.
- No result here is native update/rollback evidence (`native_evidence: false` in every `result.json`); no P0 row closes.
