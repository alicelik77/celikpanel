# set12, 2026-10-11 (UTC): D-031's second step (the PHP-FPM pool and the application unit), the multi-file page and row, the restore fact, "apply" and the addresses the validation probe asks, measured on disposable Debian 13, Ubuntu 24.04 and Arch guests (partly in a real Chrome); and the owners' path, the update from alpha.82 rebuilt from its tag

A measurement record. Built from `git archive 8fabc03b3` (branch `fix/alpha83-known-state-gates`, the commit measured);
`git diff --stat 8fabc03b3 HEAD -- cmd internal web/src` was empty when the builds were made (HEAD was 8fabc03b3,
`build/source-check.txt`). Commits made while the run was in progress are NOT in the measured build: 047e31408
(2026-10-11T04:22:30Z) changes strings in `web/src/i18n/` (the read-only site-configuration page, one Turkish term) and
ab67d9fc4 (07:44:55Z) changes the sequence-83 pins and `download-portal/get.sh`; the pictures and page texts below show
the 8fabc03b3 strings (`host/head-at-staging.txt` names the later HEAD). Nothing of the product was changed by this run;
nothing was committed or pushed. Labs 04:27Z–07:46Z; disk watcher and keep-awake 04:08Z–08:26Z; secret scan 08:29Z.

The harness addressed no installed server. The release origin's name and the four certificate-authority names were
pinned to each guest's loopback as the first driver step of every lab and read back at its end
(`*/*/driver/steps/01-set6-name-pinning`, `*-name-pinning-at-the-end`, all 26 passed). Nothing else was blocked: the
guests had outbound NAT (QEMU user networking; `*/*/host/fixture-plan.json`, `management/mode = qemu-user-nat`), so
outbound contact happened. (a) The Panel's setup installed the components its plan lists as `installed: false`
(`mariadb`, `nginx`, `php-fpm`, `postgresql`, `certbot`, `cron`, and on Debian also mail components:
`debian13/fresh-b/driver/steps/07-setup/api/0012-post-api-v1-setup-plan.json`, `arch/fresh-a/…/07-setup/api/0012-…`) with
each guest's package manager, i.e. from its distribution's repositories; the raw files record no destination.
(b) The Panel's Node.js install (`POST /api/v1/runtimes/node` 22.20.0, operation succeeded; the LTS list answered in
0.245 s: `debian13/fresh-b/driver/steps/13-n-app-unit/api/0120-…, 0121-…, 0122-…`) goes to nodejs.org by the product
code (`cmd/agent/runtime_rpc.go:119` and `:326` of 8fabc03b3; the checksums are fetched first); the raw files hold no
capture of that traffic and no `node-v22.20.0` file name. nodejs.org was not pinned. This record is not a statement of
network isolation. Every certificate request the Panel made went to the pinned names, i.e. the guest's own
127.0.0.1:443/[::1]:443 (the fixture origin), and failed `authority_unreachable`. Every `result.json` carries
`native_evidence: false`, and every update run's has `overall: failed` (inherited set4/set6 steps, classed harness
below). It closes no P0 row.

## Cell × platform

Verdict against the task's expectation. Raw = the run's `driver/steps/<NN-name>/section.json` (`cells.<name>.expectations`
holds every expectation with what was measured). `cells.md` lists every driver verdict and this record's re-judgements;
`checks-not-passed.txt` every check and step not passed, classed product / not-measured / harness / environment.

| Cell | Debian 13 | Ubuntu 24.04 | Arch | Raw (per platform: `debian13/fresh-a`, `ubuntu/fresh-a`, `arch/fresh-a` unless named) |
| --- | --- | --- | --- | --- |
| m pool: owner comment + directive, pool save → 409 `php_pool`, pending file, page/diff, take (copy, CelikPanel's text, PHP-FPM reloaded) | PASS | PASS | PASS | `11-m-pool-owner-edit` (cell `M`) |
| m PHP version switch | NOT-MEASURED | NOT-MEASURED | NOT-MEASURED | one PHP version per guest (8.4 / 8.3 / 8.5), same file |
| m delete → dated copy kept | PASS | PASS | PASS | `19-l-pool-delete` (`L`) |
| n unit: header, drop-in survives change and take, owner edit → 409 `app_unit`, take → copy + daemon-reload + try-restart | PASS (run b) | PASS | PASS | `debian13/fresh-b/…/13-n-app-unit`, `18-n-app-unit` |
| o legacy adoption (pool/unit written by alpha.82 rebuilt from its tag, then the update) | PASS | PASS | PASS | `<platform>/upd82-good*/…/18-…` (Arch `17-…`) `B-forward` |
| multi-file page: `files[]` = vhost + pool, `overall_state`/`overall_kind` | PASS | PASS | PASS | `11-m-…` |
| Agent-unread pool: page `unknown`/`not_read`, outranks managed (page) | PASS | PASS | PASS | `12-u-pool-not-read` (`U`) |
| Agent-unread pool: Domains row says "could not be read" | FAIL | FAIL | FAIL | `12-u-pool-not-read` |
| restore, nginx refusal on take → 502 `SITE_CONFIG_NGINX_REFUSED`, `restore: confirmed`, file back | PASS (run b) | PASS | PASS | `debian13/fresh-b/…/11-rn-…`, `13-rn-take-nginx-refused` |
| restore, `php-fpm -t` refused (owner's pool file elsewhere) → 502 `SERVICE_REFUSED`, `restore: confirmed`, file back | FAIL (see 2) | FAIL (same) | FAIL (same) | `14-rp1-take-php-fpm-test-refused` |
| restore unconfirmed (owner stopped PHP-FPM: reload fails) → `restore: unconfirmed` + its sentence, file back | PASS | PASS | PASS | `15-rp2-take-php-fpm-stopped` |
| restore itself failing | NOT-MEASURED | NOT-MEASURED | NOT-MEASURED | see below |
| apply: locked at start → unwritable; unlock → `managed_unchanged` + `protection_removed` + `[apply]`; POST apply → written, no copy, reload | PASS | PASS | PASS | `16-p-apply-and-b4` (`P-apply`) |
| B4: take on an owner-edited file locked after the read → 409 UNWRITABLE, no dated copy, page both facts, row agrees | PASS | PASS | PASS | `16-p-…` (`B4`) |
| probe, port-80 block on the guest's own address (10.0.2.15) → asked there, ready, then the authority | PASS (run b) | PASS | PASS (re-judged) | `debian13/fresh-b/…/12-a-probe-addresses`, `17-a-probe-addresses` (`A`) |
| probe, `[::]:80` only → `::1` asked, ready | PASS (run b) | PASS | PASS (re-judged) | same |
| probe, an address the guest does not have (198.51.100.77) → no probe, request goes ahead, journal `validation_probe=not_run …`, page reads `unknown`/`address_not_local` | PASS (run b) | PASS | PASS | same |
| … the success answer's `validation_probe`/`validation_note` fields | NOT-MEASURED | NOT-MEASURED | NOT-MEASURED | no request can succeed (pinned authority) |
| probe, nginx stopped → 503 unknown | PASS | PASS | PASS | same |
| update from alpha.82 (rebuilt from its tag, test-licence build), good | PASS (attempt c) | PASS | PASS | `debian13/upd82-good-c`, `ubuntu/upd82-good`, `arch/upd82-good` |
| update from alpha.82 (same build), defective → automatic return | PASS | PASS | PASS | `<platform>/upd82-defective/…/17-…` (`B-return`) |
| update from alpha.81 (rebuilt from its tag, test-licence build; the candidate is labelled alpha.82), good (adoption on this commit) | PASS (re-judged) | — | — | `debian13/upd81-good/…/18-…` |
| real browser (Debian): pool page EN/TR desktop/phone, row badge, restore banners, apply page, worker | PASS, two observations | — | — | `debian13/fresh-b/browser/ui/` |

Re-judged: Arch's own-address and `[::]` probes — the packet reader saw the status line but not the body (nginx sent it in
a second segment); the site's access log has two `"GET /.well-known/acme-challenge/celikpanel-probe-… HTTP/1.1" 200 61`
lines from 10.0.2.15 for the own-address case and two for `[::]:80` (`arch/fresh-a/…/17-…/native/093-…, 104-…-nginx-logs.json`;
the log format prints no Host, the packet record shows one GET per name, each answered `HTTP/1.1 200 OK`; Ubuntu's PASS
cell has the same two `200 61` lines, `ubuntu/fresh-a/…/17-…/native/094-…, 105-…`). alpha.81 good: the pools were adopted
(`managed_unchanged`) with `adopted_from: v0.1.0-alpha.82` (`debian13/upd81-good/…/18-…/api/0151-…` to `0156-…`), which
the harness had not expected; D-031 names the newest matching release. That alpha.81's and alpha.82's pool texts are the
same bytes on this host is an inference: no pool text from before this update is in the raw files (the pool difference
pages `…/18-…/files/after-the-update-set10-r*-pool-php_pool-diff.txt` show only CelikPanel's three header lines as the
difference), so the re-judging rests on D-031's adoption rule, not on a byte comparison.

## The FAILs (what the product did instead)

1. **The Domains row of a site whose pool was not read (all three).** Lab: a drop-in made PHP-FPM's reload wait 75 s and
   B's pool save (`POST /api/v1/domains/3/php/pool`, 200 after 75.1 s) held the Agent's pool lock; meanwhile
   `GET /api/v1/domains/2/site-config` answered after 60.0 s `200` with the vhost `managed_unchanged` and the pool
   `{"kind":"php_pool","state":"unknown","reason":"not_read","actions":["recheck"]}`, `overall_state:"unknown"`,
   `overall_kind:"php_pool"`; journal `site configuration set12-a.test php_pool: could not read its state: context
   deadline exceeded`. The Domains row during and after: `{"state":"managed_unchanged"}` — not the "could not be read"
   badge the D-031 entry says the row shows. (The same entry also says the list is built from ledger rows only and a
   not-read file writes none; that is what happened.) Raw: `*/fresh-a/…/12-u-pool-not-read/section.json`.
2. **"Confirmed" while PHP-FPM is down (all three).** The owner put `zz-set12-owner.conf` (`set12_not_a_directive = 1`)
   into the pool directory (`php-fpm -t` exit 78); `POST …/site-config/take {"kind":"php_pool",…}` →
   `502 {"code":"SITE_CONFIG_SERVICE_REFUSED","reason":"php_pool","detail":"config_refused","restore":"confirmed",
   "error":"… the previous pool file was put back and PHP-FPM keeps running with it. …"}` after 0.17 s. The owner's pool
   was back (same sha256, mode and owner on all three; same inode on Debian and Ubuntu, a new inode on Arch, 92079 →
   92087). But PHP-FPM went down on a reload in the journal 25 ms before the Agent's failure line (04:39:06.276
   "Reloading php8.4-fpm.service", 04:39:06.301 the Agent's "ApplySitePool … failed", 04:39:06.353 "Main process exited,
   code=exited, status=78/CONFIG"): `php8.4-fpm.service` `failed`, restarted six times (each exits 78), "Start request
   repeated too quickly", NRestarts 6 (Debian only; Ubuntu and Arch: `failed` too, NRestarts 0, no start-limit line).
   That this reload was the Agent's put-back reload is attributed by timing (the journal names no caller; the next
   harness action is its own restart at 04:39:10; the unit's reload command is not in these files). The documented
   meaning of `confirmed` ("the put-back and the reload after it held") and the sentence do not hold. The cause of the
   refusal is the owner's file; the reload was not the owner's. Raw: `14-rp1-…/section.json` (`detail.php_unit_after`,
   `php_lines`), `…/native/049-rp1-journal.json`. The driver's own RP1 verdict is PASS (it checked the answer's
   `restore` field, not PHP-FPM's state); the record's verdict is FAIL (`cells.md`, `checks-not-passed.txt`). A dated
   copy of the owner's pool is left by each refused take (documented).

## NOT-MEASURED, with the reason

- PHP version switch: one PHP version on each guest (`available_versions` `["8.4"]`, `["8.3"]`, `["8.5"]`).
- A restore that itself fails: the unconfirmed answer was reached through a failing reload (RP2), not a failing put-back;
  the put-back's atomic write runs between the write and the reload with no window the harness can lock.
- The not-run probe's success-answer fields: the authority is pinned to the guest's loopback; the 502 answer carries
  none of them (as documented: they are on the success answer). The journal line and the page were measured.
- Debian alpha.82 good attempts a and b: a stopped at `arm` (harness, below), b at setup (`HOST_MUTATION_BUSY`: the
  guest's package manager was busy); attempt c is the measurement.

## Facts per cell (Debian; Ubuntu and Arch equal unless said)

- **m.** Pool `/etc/php/8.4/fpm/pool.d/site2.conf` `root:celikpanel 0644`, first line `; celikpanel-render v2 sha256=…`
  valid. Owner lines appended in place, `php-fpm -t` ok, reload ok. Save → `409 SITE_CONFIG_OWNER_EDITED php_pool
  owner_edited`; file sha256/inode/mtime unchanged; `site2.conf.celikpanel-pending` holds CelikPanel's text with the
  save's `pm.max_children = 6`. Page: `files` kinds `[nginx_vhost, php_pool]`, pool `owner_edited`
  `["keep","take","merge"]`, the 15-line difference shows `-; set12: added by the server owner…` and
  `-php_admin_value[memory_limit] = 256M`; `overall_state owner_edited`, `overall_kind php_pool`; row
  `{"state":"owner_edited","kind":"php_pool"}`. Take → 200 `taken`, `site2.conf.celikpanel-backup-20261011T043736Z` =
  the owner's bytes; new file header-valid, no owner line, `pm.max_children = 6`; mode/owner kept (new inode); journal
  `Reloading/Reloaded php8.4-fpm.service`; ledger `take_celikpanel`, `managed_unchanged`; row back to managed.
- **L.** Deleting a third site with an owner-edited pool: pool removed, `site5.conf.celikpanel-backup-…` = owner's bytes.
- **n** (Debian run b, Ubuntu, Arch). Node.js 22.20.0 installed through `POST /api/v1/runtimes/node` (operation
  succeeded). Unit `celikapp-4.service` header-valid, active; owner drop-in `…/celikapp-4.service.d/set10-owner.conf`;
  a hosting change rewrote the unit (sealed) and left the drop-in byte-equal (inode too), `Environment` shows
  `SET10_OWNER=dropin`; owner line in the unit → hosting change `409 SITE_CONFIG_OWNER_EDITED app_unit owner_edited`,
  unit unchanged; page `include_dir …/celikapp-4.service.d`; take → 200 `taken`, dated copy = owner's unit, unit sealed,
  drop-in untouched, systemd `Reloading…`/`Reloading finished`, then `Stopping/Stopped/Started celikapp-4.service`
  (MainPID 53595 → 53960 on Ubuntu). Observation (Debian run a): with a bare `node …` start command the app exits 127
  under the unit's `ExecStart=/bin/sh -lc '…'`; the journal says `/bin/sh: 1: node: not found` (status 127) although the
  unit has `Environment=PATH=/opt/celikpanel/runtimes/node/22.20.0/bin:…` (`debian13/fresh-a/…/18-…`). That Debian's login
  profile resets `PATH` is the likely cause, inferred (`sh -l` reads it); no file shows the profile.
- **restore.** RN (Debian run b): `502 SITE_CONFIG_NGINX_REFUSED` `restore:"confirmed"`, owner's file back, nginx -t ok,
  owner's content served; a dated copy stays. RP2: owner stopped PHP-FPM; take → `502 … "detail":"reload_failed",
  "restore":"unconfirmed","details":["exit status 1; rollback incomplete: exit status 1"]`, sentence "… whether it is
  in place is not confirmed — the server administrator checks the file on the server …"; the file was back.
- **apply.** Start with A locked → page `unwritable/write_refused/EPERM/immutable`. Unlock, read →
  `managed_unchanged`, reason `protection_removed`, actions `["apply"]`, ledger reason `protection_removed`, row managed.
  `POST …/apply` → 200; new inode, CelikPanel's text, header valid, no new dated copy, nginx `Reloaded`; page and row
  managed, no apply action. **B4:** take → `409 SITE_CONFIG_UNWRITABLE write_refused`; no dated copy, file untouched;
  page `state:"unwritable","file_state":"owner_edited",EPERM,immutable`; row `unwritable`.
- **probe.** Own address: `listen 10.0.2.15:80` → two GETs to 10.0.2.15 (Host names), 200 with the probe bytes, then
  SYNs to 443 → `502 authority_unreachable`, file untouched. `[::]:80` only → GETs to `::1`, ready, 502. 198.51.100.77
  (lab: `ip_nonlocal_bind=1`) → no GET, SYNs to 443, `502`; journal `validation_probe=not_run
  validation_unknown_reason=address_not_local validation_address=198.51.100.77; the certificate request goes ahead`;
  with a `certificate_validation` reason set first (include removed → 409 `include_missing`), the page read
  `validation:"unknown"`, `validation_unknown_reason:"address_not_local"`, `validation_address:"198.51.100.77"`, reason
  kept. nginx stopped → `503 CERTIFICATE_VALIDATION_UNKNOWN unknown`.

## Update from alpha.82 rebuilt from its tag (the owners' path)

Baseline = `v0.1.0-alpha.82` tag commit 2a0af886 (`git rev-parse v0.1.0-alpha.82^{commit}` agrees) built unpatched as the
test-licence build (it carries the seam), archive SHA-256
`093a29905dea497d01b6a748966f0b872e01dcd64716e4c65eb20c84c90f204e` (`build/a82/upd1-artifacts.json`, `license_mode:
acceptance-fixture`). This is a rebuild, not the published archive: the published `celikpanel-v0.1.0-alpha.82-linux-amd64.tar.gz`
has SHA-256 `a37671064f2ea0b08fd5e0e25c14218bb8b005067855bf9c13af2fc653ffe4bc` (GitHub release asset digest, read at
intake). The alpha.81 baseline (tag commit a0beb726, the set3 dist reused read-only, SHA-256
`3350ff44dad2bb699ab5ee0a112b58b7bfb7fa47aa0ebdd3dd3c2da73d080109`, `build/a81/upd1-artifacts.json`) is likewise a
test-licence rebuild; the published alpha.81 archive is `41df36252e8e2f959de8593517cfe00d5a20cf8100962a179b51a5680dcd400b`.
Candidates: tree of 8fabc03b3 (`26faa5d3…`) with the release-sequence policy changed, labelled `v0.1.0-alpha.83 / 83`
(good 80a2aed4, `9df1d539…`; defective 411e5bc7 = set5's migrate-only defect, `a4dc8401…`); for the alpha.81 baseline the
good candidate is ded0ae08 (tree `26faa5d3…`, identical to 8fabc03b3), labelled alpha.82 (`af9b156f…`); the fresh labs'
Panel is fixture commit 9e223f175d96 (tree `5f31e237…`, differs from 8fabc03b3 only in `deploy/release-sequence-policy`),
labelled `v0.1.0-alpha.81` — hence the footer `v0.1.0-alpha.81 · build 9e223f175d96` in the pictures and
`written_release: v0.1.0-alpha.81 9e223f175d96` in the fresh labs' ledger rows (`build/fresh/trees.txt`).
Before the update on alpha.82: R2, R3 (PHP), a Panel restart, R1, R3's vhost and R2's pool edited by the owner, Node.js
22.20.0 and a Node site RN; ledger 43, no file with a header.

| | Debian 13 (c) | Ubuntu 24.04 | Arch |
| --- | --- | --- | --- |
| good: outcome, ledger | update-verified, 43 → 44 | same | same |
| start line | 5 written, 1 kept (unknown origin); "5 adopted, 1 left alone" | same | 4 adopted, 1 left alone |
| vhosts | R1 `adopted_from v0.1.0-alpha.82 (creation)`, R2 alpha.82; R3 `unknown_origin`, never written | same | same |
| pools / unit (not rendered at start, bytes alpha.82's) | R1 pool adopted alpha.82; R2 pool `unknown_origin`; RN unit adopted alpha.82 | same | same |
| 428 without identity (keep, take, recreate, apply) | all 428 `REQUEST_ID_REQUIRED`, nothing changed | same | same |
| Domains first-state line (computed as Domains.tsx does) | adopted 5, left 1, drawn; after the pages were read: 4 / 2 (R2's row now names its pool) | same | 4 / 1, then 3 / 2 |
| defective: outcome, ledger | recovered-automatically (`rollback_verified`, two attempts; injected fault: reboot at `payload_restored`), 43 | same (reboot at `payload_restored`) | same outcome and attempts, but the injected fault was a kill at `runtime_verified`; no reboot (`result.json` `outcome.reboot` null), 43 |
| files after the return | pools and unit byte-equal to alpha.82's; every vhost a new inode (alpha.82's start rewrote them): R1 and R3 changed bytes — R3's owner edit gone (D-031 point 7); Panel usable (Domains 200) | same | same |

alpha.81 → good on Debian: ledger 42 → 44; R1 `v0.1.0-alpha.81 (creation)`, R2 `v0.1.0-alpha.81`, R3 `unknown_origin`;
pools adopted (named alpha.82, see above); database put back from before the migration → 44 again, states again (PASS).

## The real browser (Chrome 154.0.8037.98 per `run.json`, headless per the script; Debian run b, one window)

Profile under `%TEMP%\cpb\<8 hex>`: 46 characters (`browser-profile-*.json`); `navigator.serviceWorker.controller`
`/recovery-worker.js` `activated` after the first load and after a reload (`sw.jsonl`). The owner first left the
waiting setup ("Edit plan" = `POST /api/v1/setup/revise`, then `PUT /api/v1/setup/guidance {"guidance":"manual"}`);
run a's window had stayed on the setup page (harness, below). Pictures looked at: Domains row of A "Configuration file:
your choice needed" (the row does not say which file); the Configuration file page lists "nginx configuration —
CelikPanel's text, unchanged" and "PHP-FPM pool … — Edited outside CelikPanel" with the pool section, its sentence and
the difference (EN and TR, desktop and phone); apply page: vhost "Can be replaced again" / "Yeniden
değiştirilebilir", the button, and after the click the page text "CelikPanel'in metni yerinde ve nginx yeniden
yüklendi." (`apply-tr-phone-3-after-click.txt`; the picture, scrolled down, shows both files "CelikPanel'in metni,
değişmemiş" but not that sentence). Observation: both restore banners (unconfirmed and confirmed) are the first text of
the files section in the page texts `restore-2-unconfirmed.txt`, `restore-3-confirmed.txt`; the two pictures are
byte-identical (sha256 `b6ff56ba…`) and show neither banner: after the click on the pool's button the viewport shows
the button (still "Replace the file", with "Cancel") — so "above the files list" is read from the page text order, not
seen in a picture. `restore-1-before-take.png` equals `pool-en-desktop-2-config.png`. At phone width the Domains list
pictures cut off the status column; the badge sits under the domain name (`pool-*-phone-1-domains.png`). In the apply
sequence only the first Domains picture (`apply-en-desktop-1-domains.png`) shows a badge ("Configuration file cannot be
replaced"); the later three Domains pictures (`apply-en-phone-1`, `apply-tr-desktop-1`, `apply-tr-phone-1`) show
set12-a.test without one, after the Configuration file page had been read in between (that the read changed the row is
inferred from the order of the events). No typed text or password
is in any picture (`password_fields_with_text` 0 in every event); frames were taken after sign-in only. puppeteer-core
24.10.0 appears in no raw file (it is the module the runner used on the host).

## Method, runs, deviations

Builds (`build/`): `run-upd1.sh build 8fabc03b3` (fresh; B 9e223f17, `fab97c73…`); `run-upd1.sh build --baseline-ref
v0.1.0-alpha.81 8fabc03b3` (alpha.81 dist of set3 reused read-only); `build-set12-artifacts.sh --baseline-ref
v0.1.0-alpha.82 8fabc03b3`. All proven (`*-prove.json`), dry runs without a lab. Go 1.26.5. Guests: Debian 13 (PHP 8.4),
Ubuntu 24.04 (8.3), Arch (8.5). Harness errors and their fixes (each a new run, never a rerun on the same lab):
`debian13/fresh-a` — RN `KeyError` (set10's vhost restore defaults to set10's site name), the "non-local" address
192.0.2.10 was the guest's own (lab guests carry 192.0.2.x), a bare `node` command; and the browser stayed on the setup
page (the revise condition read the setup state, not its execution): all four sections ran again in `debian13/fresh-b`.
`debian13/upd82-good` — the owner-update observer's closed label set had no (alpha.82, alpha.83) pair: the guest gets a
generated copy of `guest_bound_worker.py` with that pair (`build/generated/`, `set12_fixture.bound_worker_copy`);
`upd82-good-b` failed at setup (environment); `-c` is the measurement. The first browser attempt of run b ended at the
TLS read of the forward (8 s timeout) and was run again (`host/browser-runner.log`). Inherited set4/set6 steps that
check alpha.81's vhost snippet and HSTS header fail on an alpha.82 baseline (classed harness). Error texts with non-ASCII
characters appear double-encoded (`â€”`) in driver records: the driver's decoding, not the answer (the browser shows "—").

## Harness files (new, not committed; no existing file changed)

`deploy/e2e/release-recovery/set12_trial.py`, `guest_set12_native.py`, `run-set12.sh`, `set12_fixture.py`,
`build-set12-artifacts.sh`, `test_set12_trial.py` (7 offline tests); `web/tools/browser-inspect/cold-load-set12.mjs`.
Offline suite: copy a (pristine 8fabc03b3) 1046 tests, copy f (+ set12) 1053, the same 5 errors by name
(`harness-run-copy/*/suite-notok.txt`); they are `git` calls against commits the run copy (a file copy without history)
does not hold (`harness-run-copy/f/suite.txt` lines 127–196), not set12 code. Copy b–e's `files.sha256` list repository
paths, not files of this folder; copy f's seven hashes equal the harness files at intake. Run copies a–f: `harness-run-copy/*`.

## Secrets, host, leftovers

`secret-scan.txt`: no unexplained hit; operator user and machine names 0 (the scanner searched for both); local profile paths: the 2 informational matches are the scanner's own pattern text in tools/secretscan.py. SSH key comments
`celikpanel-lab`. Paths and names in host/tools files were replaced at staging (`host/scrubbed-files.txt`, plain and
JSON-escaped forms); driver files unchanged; `harness-run-copy/*/harness.diff` keeps the build scripts' default checkout
path (source text, as in the committed `build-upd1-artifacts.sh`). Disk: `Get-PSDrive C` every 30 s, 514 readings
04:08Z–08:25Z, 75.41 GiB at the start, lowest 61.64 GiB (08:04Z), 61.85 GiB last; 14 gate decisions, all allowed
(`host/c-drive-cells.txt`).
Power: keep-awake for the run; the System log shows modern standby entered 02:25:47Z (before the run) and left
07:43:22Z (`host/sleep-events.txt`), i.e. the standby state covered the first 3 h 35 min of the run (04:08Z–07:43Z),
while the disk watcher read every 30–31 s with no larger gap (checked at intake over all 514 readings): the host was
running. WSL refused new sessions twice (`Wsl/Service/0x8007274c`, host memory low) and running jobs continued: this
is the operator's report from the run; the only file that touches it is `tools/w.sh` (the retry handler), no log records
the two occurrences, their length or the memory cause.
Overlay disks of all 13 labs removed after staging (`host/removals.txt`). Left on the WSL host: `/var/tmp/cp-set12-run`
(470 MB), thirteen `/var/tmp/cp-release-drill-s12-*` (records, no disks, 864 MB), the builds
`/var/tmp/cp-upd1-build/20261011t040923z`, `…041117z`, `…041832z` (3.7 GB) and eleven new dist folders under
`/var/tmp/cp-pair-accept/dist/`. Browser profiles deleted. Nothing of this run is running. (The sizes, "profiles
deleted" and "nothing is running" are statements from the staging session; no file in this folder records them.)
`SHA256SUMS` was written last (rewritten at intake, see below).
No text file here holds a CR byte (checked at intake; the earlier "CRLF" in this line was a template word). The two
`*.log` files, `host/browser-runner.log` and `host/keepawake.log`, match `.gitignore` (`*.log`): commit this folder with
`git add -f`.

## Corrections after intake (2026-10-11)

Intake before this folder is committed to the public repository. Nothing in the driver's raw records (`*/*/driver/`,
`*/browser/`, `build/`, `harness-run-copy/`) was changed; the changes are to this README, `cells.md` and
`checks-not-passed.txt`, and `SHA256SUMS` was regenerated last. What the intake checked and found:

- **Sums.** Root `SHA256SUMS` 4512 lines, 4512 verified, 0 failed, 0 absent, 0 unlisted except the list itself; the 13
  per-run `driver/SHA256SUMS` all verified with 0 unlisted. No file holds a CR byte (the PNGs excepted, by chance).
- **Secrets (text and images).** Independent rescan (not the run's scanner): no PEM block, no private-key or password
  value, no cookie/bearer/authorization value, no hash-shaped credential, no token digest or `.release-db-migrations/`
  name (plain; the two base64 `events_base64` fields decode to lab fault records with a lab nonce); 3731 JSON/JSONL files
  parse; every secret-named field holds `[REDACTED]`, `[REDACTED-SHA256]` or a count; the 23 SSH public keys carry the
  comment `celikpanel-lab`; the operator's user and machine names occur 0 times in any form (plain, JSON-escaped, base64
  fields); the only checkout path is the `REPO=${CELIKPANEL_REPO:-…}` default of `build-set12-artifacts.sh` inside `harness-run-copy/b–f/harness.diff` (the
  same default as the committed `build-upd1-artifacts.sh:47`). All 21 distinct pictures (23 files; two pairs are
  byte-identical) were looked at: no sign-in frame, no typed text, no password, key, token, installed-server name or address (lab addresses
  192.0.2.x and 10.0.2.15 only). `handover/ready.json` of the two browser runs names the host path of a root-only password
  file (`password_file`), not its content; left as recorded.
- **Changed sentences (old → new).**
  1. Title and "Update from the published alpha.82" (heading, three table rows, row "o"): "the published alpha.82" → "alpha.82
     rebuilt from its tag" (test-licence build; digests differ from the published archives, now stated); same for alpha.81.
  2. "the last commit that changes product code" → "the commit measured", with the later commits that change
     `web/src/i18n/` (047e31408) and `download-portal/get.sh` (ab67d9fc4) named as not measured; "Run 04:08Z–08:20Z" →
     "Labs 04:27Z–07:46Z; disk watcher and keep-awake 04:08Z–08:26Z; secret scan 08:29Z".
  3. "The guests had outbound NAT: the Panel's Node.js install downloaded `node-v22.20.0` from nodejs.org through it (the
     product's own path, checksum verified by the Agent)" → outbound contact stated plainly with its two parts (package
     installs by each guest's package manager; the Node.js install and LTS list by the product code), the evidence path of
     the NAT (`management/mode = qemu-user-nat`), "nodejs.org was not pinned", and that the raw files hold no capture of
     the traffic and no `node-v22.20.0` file name.
  4. FAIL 2: "The owner's pool was back (sha256, inode, mode, owner)" → same inode on Debian and Ubuntu, a new inode on Arch
     (92079 → 92087); "NRestarts 6 (Ubuntu and Arch: `failed` too)" → Debian only, Ubuntu and Arch NRestarts 0 without a
     start-limit line; "the reload the Agent ran after the put-back (`systemctl reload` = USR2 …)" → attributed by
     timing, the unit's reload command is not in the files; "the reload was CelikPanel's" → "the reload was not the
     owner's". The driver's RP1 verdict was PASS in all three runs; the record's is FAIL (`cells.md`,
     `checks-not-passed.txt`; the README and the two lists disagreed before).
  5. Row "defective … set5's reboot": → reboot at `payload_restored` on Debian and Ubuntu; on Arch a kill at
     `runtime_verified`, no reboot (`outcome.reboot` null).
  6. Re-judged Arch probes: "access log … `200 61` for both names" → two `200 61` lines per case, the log prints no Host;
     Ubuntu's PASS cell shows the same lines. Re-judged alpha.81 good: "on this host alpha.81's and alpha.82's pool texts
     are the same bytes" → an inference from D-031's adoption rule; no pool text from before that update is recorded.
  7. Browser: "after the click … 'CelikPanel'in metni yerinde …'" and "restore banners … drawn above the files list" → read
     from the page texts; the pictures `restore-2-unconfirmed.png` and `restore-3-confirmed.png` are byte-identical and
     show no banner; the apply sequence's Domains pictures show a badge in the first one only. "puppeteer-core 24.10.0"
     → README only.
  8. Debian run a's `node: not found`: "Debian's login profile resets `PATH`" → likely cause, inferred.
  9. Power and host: the standby state covered the first 3 h 35 min of the run (not only "before the run"); the two WSL
     refusals, the leftover sizes, "profiles deleted" and "nothing is running" are the operator's statements, not in a file.
  10. Offline suite: the 5 errors are `git` calls against commits the run copy does not hold; last line "CRLF/`*.log`" →
      no CR bytes, two `*.log` files match `.gitignore`.
- **Not changed, still open.** The two product FAILs stand (Domains row of an unread pool; `restore: confirmed` with
  PHP-FPM `failed`); PHP version switch, a restore that itself fails, and the success answer's probe fields remain
  NOT-MEASURED; the strings changed by 047e31408 and the sequence-83 pins are unmeasured; the owners' path was measured
  from test-licence rebuilds, not from the published archives; the package installs and the Node.js download
  are outside contact whose traffic is not recorded; the harness's inherited set4/set6 checks fail on the alpha.82 baseline
  (classed harness: that baseline sends HSTS without `includeSubDomains` and its vhost has no `fastcgi-php.conf` snippet),
  so every update run's `result.json` says `overall: failed`.

Date correction (2026-10-11 12:30 +0300): the heading above first read "2026-10-12"; the intake and this run took place on 2026-10-11 (UTC-11/12 boundary not crossed for the intake). The clock date was not read when the heading was written.
