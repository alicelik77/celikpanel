# set11, 2026-10-11 (UTC): the corrections committed after the tenth native record (ead69cdcc), measured on disposable Debian 13, Ubuntu 24.04 and Arch guests, partly in a real Chrome; and set5's start-check cell from the published alpha.81

A measurement record. Built from `git archive ead69cdcc` (branch `fix/alpha83-known-state-gates`); `git diff --stat
cd46ca595 ead69cdcc -- cmd internal web/src` lists the 53 product files of the corrections and `git diff --stat ead69cdcc
HEAD -- cmd internal web/src` was empty when the run started (`build/source-check.txt`). Four commits landed on the
branch during the run (`host/commits-after-ead69cdcc-at-staging.txt`; 5f954699c, 224bbff03, 13e8370a7, 700280b05, the
first two touch the site-config read, the row/badge precedence and the PHP-FPM pool); none entered a build or a cell:
every build is of ead69cdcc. Nothing of the product was changed by this run; nothing was committed or pushed by it
(224bbff03, made by someone else during the run, contains `web/tools/browser-inspect/cold-load-set11.mjs` as this run left
it; see "Harness files").

The harness addressed no installed server. The release origin's name and the four certificate-authority names
(`acme-v02.api.letsencrypt.org`, `acme-staging-v02.api.letsencrypt.org`, `acme.zerossl.com`, `dv.acme-v02.api.pki.goog`)
were pinned to the guest's loopback as the first driver step of every lab and read back at the end of every lab that
finished (`*/*/driver/steps/01-set6-name-pinning`, `*-name-pinning-at-the-end`; `debian13/run-a` was stopped before its
end). The guests had outbound NAT (QEMU user networking) that was not captured: this record is not a statement of network
isolation. The certificate requests the Panel made in cells A1 and A3 went to the pinned name, i.e. to the guest's own
127.0.0.1:443 (the fixture release origin, whose certificate names celikpanel.net); certbot refused it for the hostname
mismatch. Every `result.json` (five; `debian13/run-a` has none) carries `native_evidence: false`. It closes no P0 row.

## Cell x platform

Verdict against the expectation the task named for the cell (and, where they differ, the D-031/2026-10-11 entries of
`docs/OPERATION-GUIDANCE.md` and `docs/RESILIENCE-CONTRACT.md`). Raw = that cell's `section.json` under
`<platform>/<run>/driver/steps/` (its `cells.<name>.expectations` hold each expectation with what was measured) or the
browser folder named. Decisive runs: `debian13/run-c`, `ubuntu/run-a`, `arch/run-a`, `debian13/update-startcheck`.

| Cell | Debian 13 | Ubuntu 24.04 | Arch | Raw |
| --- | --- | --- | --- | --- |
| C include directories as created: owner's `celikpanel-sites.d/<d>` and CelikPanel's `celikpanel-managed.d/<d>` (3 sites) and both bases | PASS (all `root:celikpanel 0755`) | PASS | PASS | `11-c-directory-modes` |
| A1 owner-edited, include lines kept → issuance: probe file, loopback GETs, ready, then the CA | PASS (re-judged, see Method) | PASS | not asked | `12-a1-probe-ready` |
| A4 nginx stopped → issuance | PASS | PASS | not asked | `13-a4-nginx-stopped` |
| A2 kept file without the managed include → `include_missing` | PASS (re-judged, see Method; its limit is under "Corrections after intake") | PASS | PASS | `14-a2-include-missing` (Arch: `12-…`) |
| A5 owner puts the line back → GET re-probes, reason ends, row clears | NOT-MEASURED (dashboard part; all else held) | NOT-MEASURED (same) | not asked | `15-a5-include-back` |
| A3 `mail.` name: owner's block present → ready; removed → `names_missing` | PASS | PASS | not asked | `16-a3-mail-name` |
| A6 port-80 block bound to one address | FAIL (`include_missing`, not `unknown`) | FAIL (same) | not asked | `17-a6-bound-address` |
| B1 locked file at start: page, row, "check again" | PASS (badge in the browser NOT-MEASURED) | PASS (same) | PASS (no browser) | `18-b1-locked-start` (Arch `13-…`) |
| B2 unlocked → check again | PASS | PASS | PASS | `19-b2-unlocked-recheck` |
| B3 settings save after the unlock | PASS | PASS | PASS | `20-b3-save-writes` |
| B4 take refused (`chattr +i` between GET and POST): page and row | FAIL (page `owner_edited`, row `unwritable`) | FAIL (same) | FAIL (same) | `21-b4-take-refused` (Arch `16-…`) |
| D1 every session read held 35 s, cold load | PASS | PASS | — | `browser/h11d13c-d-e/slow35`, `ubuntu/run-a/browser/h11ub-d-e/slow35` |
| D2a an answered failure, then the re-read held past its limit | PASS (observation below) | PASS (same) | — | `…/afterfail` |
| D2b a real "starting" answer, then the re-read held | PASS | NOT-MEASURED (both loads after the restart got Chrome's `ERR_CONNECTION_CLOSED` page; no Panel answer was recorded) | — | `…/afterstart` |
| E recovery service worker | cause narrowed to that browser profile, not the product; the path-length mechanism is inferred (see E) | profile at a short path: registered and served | — | `…/h11d13c-d-e/sw`, `ubuntu/run-a/browser/h11ub-d-e/sw-shortprofile` |
| F headers on an automatic return (start-check cell, from the published alpha.81) | header questions NOT-MEASURED: no header was ever written (see F); B-before PASS, B-return PASS | — | — | `debian13/update-startcheck/driver/steps/09-…`, `17-…` |

`cells.md` is the generated per-cell table of the drivers' verdicts (`tools/table.py`), `checks-not-passed.txt` every
check and step that is not true or not passed, classed product / not-measured / harness / aborted (`tools/notpassed.py`).

### Facts per cell (Debian 13 run-c; Ubuntu equal unless said; probe names and digests differ per run)

- **Probe (A1, A2, A3, A5, A6; read by an inotify watcher on the challenge directory and a packet reader on `lo`,
  corroborated by nginx's own access logs).** One file `celikpanel-probe-<32 hex>` in
  `/var/lib/…/.well-known/acme-challenge/`, events create → modify → close_write → attrib → delete, final mode `0644`
  (in one probe, A3 "with the block" on Debian run-c, the first stat read `0640` before the attrib event),
  `root:celikpanel` (gid 989/988/969), 61 bytes; one `GET /.well-known/acme-challenge/<name>` to 127.0.0.1:80 per
  validation name in order (`set11-a.test`, `www.set11-a.test`, then `mail.set11-a.test` when asked), User-Agent
  `Go-http-client/1.1`; the file existed for 0.2–4.8 ms between the watcher's create and delete readings (all probes of
  Debian run-b/run-c, Ubuntu, Arch). Ready = each answer `200` with the probe's 61 bytes
  (`<domain>-access.log`: `… "GET …/celikpanel-probe-… HTTP/1.1" 200 61 …`).
- **A1:** after the probe, a connection to 127.0.0.1:443 (certbot to the pinned CA name), then `502
  {"code":"CERTIFICATE_ISSUE_FAILED","reason":"authority_unreachable","vars":{"domain":"set11-a.test","kept":"none",
  "detail":"… SSLCertVerificationError … Hostname mismatch, certificate is not valid for 'acme-v02.api.letsencrypt.org'…"}}`
  after 0.8 s (Ubuntu 1.4 s); the expected answer (`cmd/agent/certbot_failure.go` maps `certificate_verify_failed` to
  `authority_unreachable`). The owner's vhost file is untouched (sha256, inode, mtime); no ledger reason. A side file
  `<vhost>.celikpanel-pending` (2130 bytes, CelikPanel's render) was written beside it by this request and left there
  (`native/027-a1-1-stat.json`; see "Corrections after intake").
- **A4:** `503 {"code":"CERTIFICATE_VALIDATION_UNKNOWN","detail":"unknown"}`; the probe file written and removed; the
  SYN to port 80 answered by RST, no HTTP answer; no connection to 443; ledger reason unchanged (empty).
- **A2:** `409 {"code":"SITE_CONFIG_OWNER_EDITED","reason":"certificate_validation","detail":"include_missing","vars":
  {"include":"include /etc/nginx/celikpanel-managed.d/set11-a.test/*.conf;","name":"set11-a.test","status":"200"}}` on
  all three platforms. nginx answered `200` for both names — 411 bytes of the site's own page (the vhost's
  `try_files $uri $uri/ /index.php…`), not the probe. The product's string `siteConfig.certificate.validation.probe`
  ("What nginx on this server answered: HTTP {status} for {name}.", `web/src/i18n/screens/server/en.ts`) would read "HTTP
  200 for set11-a.test."; the page itself was not looked at in this cell. Ledger reason `certificate_validation`;
  Domains row `{"state":"owner_edited","pending_reason":"certificate_validation"}`.
- **A5:** after the owner's line and reload, `GET site-config` → `200` with `validation:"ready"`,
  `resolved_reason:"certificate_validation"`, `challenge_file:"unchanged"`, no `pending_reason`; journal "the kept file
  now lets the certificate validation run; the waiting reason ended"; ledger reason empty; Domains row loses
  `pending_reason`; a second read asks for no probe. The challenge file was `unchanged`, so no `nginx -t`/reload was due:
  none happened during the read and nothing in `celikpanel-managed.d` was written (the one "Reloaded" line in the window
  is the owner's own reload at 00:51:36.43Z, 1.2 s before the watcher started and 2.8 s before the read, whose probe file
  and journal line are at 00:51:39.2Z; `tools/a5reload.py`). The absent/outdated branch was not exercised.
  Dashboard: no entry before or after — entries are certificates waiting for the owner, and the site has none (no
  issuance can succeed here): NOT-MEASURED.
- **A3:** with the owner's validation-only block, the three names answered `200`/probe bytes → certbot → `502
  authority_unreachable`; after the owner removed it, `mail.set11-a.test` was answered `404` by the default server
  (`access.log`) → `409 … "detail":"names_missing","vars":{…,"name":"mail.set11-a.test","status":"404"}`; ledger reason
  `certificate_validation`; the owner's vhost file not written by either request (the "with the block" request wrote the
  `.celikpanel-pending` side file, as in A1).
- **A6 (FAIL against `unknown`).** The owner's block `listen 10.0.2.15:80;` (`[::]:80` removed), `nginx -t` passed;
  other blocks still listen on `0.0.0.0:80`/`[::]:80`. The probe from 127.0.0.1 reached the default server (`access.log`,
  `404` for both names) and the answer was `409 SITE_CONFIG_OWNER_EDITED … "detail":"include_missing","vars":{…,
  "name":"set11-a.test","status":"404"}` with ledger reason `certificate_validation` — a refusal although the owner's
  file includes CelikPanel's line and would serve the validation on 10.0.2.15. The docs' open item (1) names exactly this
  for a host where another block listens on the wildcard address; the `unknown` branch (nothing else on the loopback's
  port 80) was not measured.
- **B1:** start with A `chattr +i` while A, B and the static site needed a render: A kept (sha256, inode, link), B and
  static written, `site configuration files at start: 2 written, … 1 unreadable or unwritable …`; ledger
  `unreadable/write_refused`; `GET site-config` → `state:"unwritable", reason:"write_refused", system_error:"EPERM",
  protection:"immutable", actions:["recheck"]`, diff of 7 lines; Domains row `{"state":"unwritable"}`; "check again"
  (the page's GET, sent by the driver) → the same answer, A's sha256/inode/mtime unchanged, no inotify event on A's
  files, no site-configuration line, no reload. The badge and the page could not be looked at, and the browser's "Check
  again" click did not happen (`events.jsonl`: `clicked: null`): in this lab the interface shows the server-setup page
  instead of Domains and the domain page (the setup waits at its public-DNS step), see
  `debian13/run-c/browser/h11d13c-b-unwritable/siteconfig/02-domain-set11-a.test.png`.
- **B2:** after `chattr -i`, check again → `state:"unwritable", reason:"write_refused", protection:"none",
  system_error:null, actions:["recheck"]`; row `unwritable`; nothing written — as the docs say ("No write protection is
  found on the file now"; written at the next start or save).
- **B3:** General save → `200`; A rewritten (new inode, CelikPanel's text, valid header); journal `site configuration
  set11-a.test (change): written`; page, row and ledger `managed_unchanged`.

## The FAILs (what the product did instead)

1. **A6, Debian 13 and Ubuntu 24.04.** Request `POST /api/v1/domains/2/ssl/letsencrypt {"email":"hostmaster@set11-a.test",
   "auto_renew":false}` on the owner-edited file bound to `10.0.2.15:80`. Answer `409 {"code":"SITE_CONFIG_OWNER_EDITED",
   "reason":"certificate_validation","detail":"include_missing","vars":{"include":"include
   /etc/nginx/celikpanel-managed.d/set11-a.test/*.conf;","name":"set11-a.test","status":"404"}}` instead of `unknown`.
   Raw: `debian13/run-c/driver/steps/17-a6-bound-address/section.json` (`a6`, `native/*-a6-watch-stop.json`,
   `native/*-a6-nginx-logs.json`); Ubuntu the same path under `ubuntu/run-a`. The driver's own verdict is PASS, because it
   accepted either answer the docs name; the record's verdict follows the task's expectation.
2. **B4, all three platforms.** `POST /api/v1/domains/2/site-config/take` with the shown digests and a request identity,
   the file locked after the GET → `409 {"code":"SITE_CONFIG_UNWRITABLE","reason":"write_refused","error":"CelikPanel could
   not read or replace … the reason is shown in the domain's Configuration file section."}`; the owner's file untouched;
   a dated copy `set11-a.test.conf.celikpanel-backup-<UTC>` left beside it (Debian run-c: 2241 bytes, sha256 equal to the
   owner's file, `native/137-b4-1-stat.json`); ledger `unreadable/write_refused` with
   `backup_path`; journal `site configuration set11-a.test (take): kept, not replaced (owner_edited write_refused); owner's
   file kept as …; rename … operation not permitted`. Then the page: `state:"owner_edited", reason:null,
   system_error:"EPERM", protection:"immutable", actions:["keep","take","merge"]`, while the Domains row says
   `{"state":"unwritable"}`: the two disagree, and the page does not show the refusal's reason the error sentence points
   to. Raw: `debian13/run-c/driver/steps/21-b4-take-refused/section.json` (`b4`), `ubuntu/run-a/…/21-…`,
   `arch/run-a/…/16-b4-take-refused/section.json`. (Commit 5f954699c, after ead69cdcc, says it gives the row, badge and
   page one precedence; not measured here.)

## NOT-MEASURED, with the reason

- A5 dashboard entry: no certificate exists for the site in a lab whose CA names are pinned to loopback, so no entry
  can exist before or after.
- B1 badge in the browser: the interface shows the server-setup page instead of Domains/the domain page while the lab's
  setup waits at its DNS step (screenshots `*/browser/*-b-unwritable/siteconfig/*.png`); the row was read by the API.
- D2b on Ubuntu: both cold loads started right after `systemctl restart` landed on Chrome's error page
  (`ERR_CONNECTION_CLOSED`: the document request itself failed, no `/api` request was made) and stayed there for the
  51 s watch; on Debian the Panel answered the first read 289 ms after the navigation call (the load began 0.19 s after
  the harness's restart-done record).
- A6's `unknown` branch; A5's absent/outdated challenge file; A1/A3/A4/A5/A6 on Arch (only A2 asked); D and E on Arch.
- F: whether a header survives a return and whether a forward update adopts again (see F).

## D: the session wait (real Chrome 154.0.8037.98, headless, puppeteer-core 24.10.0, EN, ms from the navigation call)

- **D1** (`/settings?section=updates`, every `auth/me` held 35 s): Debian — quiet surface 203–1662 ms; 1699 ms "Checking
  panel access / The Panel has not answered yet. CelikPanel opens as soon as it does; you do not need to do anything."
  with "Checking…"; 15031 ms "The Panel has not answered for a while. CelikPanel keeps checking by itself; you can also
  check now." with "Check now"; 30018 ms the half-minute sentence and "Reload CelikPanel" beside "Check now". Ubuntu
  1984 / 15042 / 30040 ms. The automatic re-read at 25.16 s changed nothing; no other screen in any frame (frame log:
  `loads.jsonl` `painted`). Pictures looked at: `…/slow35/shots/…/001-02066ms.jpg`, `1248-15204ms.jpg`, `1250-30163ms.jpg`
  (Debian run-c); between 15.2 s and 30.2 s Chrome sent no picture (the picture did not change; the DOM log covers it).
- **D2a** (`/`, the first session reads failed as a closed connection, later ones held 40 s): 0.22 s "Your session could
  not be checked … The connection to the Panel failed before it answered …" (an answered failure, the known negative);
  10.25–25.24 s, while the automatic re-read is held, `recovery.checkingHelp` "Confirming your session and panel
  readiness. This check does not start a server operation." with "Checking…" and "Reload CelikPanel" (documented: the
  page reading again without a verified session; the reload is offered during these 15 s); 25.25 s the wait sequence
  counted from that read: "has not answered for a while" + "Check now" + "Last known, before this wait: The connection to
  the Panel failed before it answered (refused, closed or unreachable)…"; 40.26 s (30.0 s after the read began) the
  half-minute sentence and the reload. "Your session could not be checked" never returned after the limit. Ubuntu the
  same to within 10 ms. Pictures: `…/afterfail/shots/…/011-00270ms.jpg`, `163-11995ms.jpg`, `1436-25433ms.jpg`,
  `1437-40403ms.jpg`.
- **D2b** (Debian; the Panel restarted by the harness just before; the first reads went through): `auth/me` 200,
  availability 200 `{"state":"starting"}` → 0.34 s "The panel is starting"; 10.31 s the same with "Checking…" and the
  reload while the re-read is held; 25.32 s "has not answered for a while" + "Check now", no "Last known" line; 40.31 s
  the half-minute sentence and the reload.

## E: the recovery service worker — cause narrowed to the lab browser's on-disk profile (path-length mechanism inferred)

- Both contexts: `isSecureContext: true` (the lab certificate accepted by Chrome's SPKI pin counts as secure),
  `document.URL https://127.0.0.1:<port>/setup` (signed in) or `/` (no session); `GET /recovery-worker.js` 200,
  `text/javascript; charset=utf-8`, `Cache-Control: no-cache`, 4534 bytes, no `Service-Worker-Allowed` header (none
  needed for scope `/`); `/recovery-offline.html` 200 `text/html`.
- Debian run-c, the signed-in context = Chrome's on-disk profile, which the script puts under the browser output folder
  (`<scratchpad>/set11/browser/<window>/.chrome-profile-<epoch>`, no `LIVE_PROFILE_ROOT`; the profile path itself is not
  recorded in any raw file): no registration after the first load (`sw.jsonl`); an explicit
  `register('/recovery-worker.js',{scope:'/'})` resolved (no rejection), but the worker went `installing → redundant`
  (`sw-events-signed-in.jsonl`, three attempts); CDP `ServiceWorker.workerErrorReported`: "ServiceWorker failed to
  install: … (event.waitUntil Promise rejected)" and "Uncaught (in promise) UnknownError: Failed to execute 'open' on
  'CacheStorage': Unexpected internal error."; `controller` null after a reload. A new browser context of the same
  browser process (in memory): registered, `activated`, controls the page (`sw-events-no-session.jsonl`). Path lengths
  (not in a raw file; computed at intake from the folder layout and the script's naming): about 177 characters to the
  profile directory and 213 to `…/Default/Service Worker/CacheStorage`; the contents of that directory were not
  listed.
- Ubuntu, the same script and Chrome with the profile under a short path (`<user-temp>\cp11p`, recorded in
  `host/browser-runner.log`): the signed-in context registered and was controlled after the first load and after a
  reload. This is a different guest and a different browser process from the Debian run, so it is not a controlled
  comparison; Ubuntu's `sw` cell was not run with the long path.
- Panel stopped by the harness, cold load of `/`: long-path signed-in context → Chrome's own error page
  (`ERR_CONNECTION_CLOSED`, "Bu siteye ulaşılamıyor"); new context → the product's offline page "The panel connection is
  unavailable" (`sw/shots/stopped-no-session-root/final.png`); short path, both contexts → the offline page
  (`sw-shortprofile/shots/stopped-signed-in-root/final.png`).
- Verdict: set9's "no registration in the signed-in context" points at the lab browser's on-disk profile, not the
  product: in the Debian run the same browser process registered the worker in an in-memory context and failed to install
  it in the on-disk one with a `CacheStorage` open error, and a profile at a short path on the Ubuntu run registered and
  served the offline page. That the 260-character Windows path limit is the mechanism is inferred (the error, the path
  arithmetic and the short-path result), not read from a Chrome line naming the path and not shown by a listing of the
  profile directory. Not measured: a certificate trusted without a pin; the long path on Ubuntu; the short path on
  Debian.

## F: headers on an automatic return (`upd1-debian13-startcheck`, published alpha.81 → start-check candidate a9acedb45)

- Before: on alpha.81, R2 and R3 created, a Panel restart, R1 created, R3 edited by the owner; no file carries a header;
  ledger 42 (`steps/09-set10-sites-before-the-update`, files `pre-before-the-update-*.conf.txt`).
- The update: `candidate_panel_startup_check_failed`, `rollback_verified`, classification `recovered-automatically`; two
  automatic attempts (the second after set5's reboot fault at `payload_restored`).
- After: ledger 42, no `managed_site_files`; **no file carries a header, and none ever did**: the candidate's start check
  (`--check-startup-readiness`, which never dials the Agent: that is the fixture's recorded description, not a journal
  line) failed with `panel startup check failed: tls_pair_invalid: the panel TLS certificate and private key cannot be
  loaded as a matching pair` (`steps/21-collect/journal-product.txt`, the update-failure line) before the candidate's
  start; the fixture's own defect description names `configurePanelHTTPTLS`
  (`build/a81/upd1-artifacts.json`); no `site configuration files at start` line exists in the window. alpha.81's start (`restored 4 hosted vhosts
  with one nginx validation and reload`) rewrote every vhost (new inode for all four): R1's creation text became the
  start text (`www.` added), R3's owner edit was overwritten, R2 and the seeded site have the same bytes; no file has the
  include line (`steps/17-set10-site-files-after-the-update`, files `after-the-update-*.conf.txt`).
- So the premise "the candidate starts, adopts files, then fails its start check" does not occur with this cell: the
  start check precedes the start (`journal-product.txt` has no "Panel startup listener active" line between 01:46:54Z
  and the returned alpha.81's start at 01:50:41Z; the order in
  `cmd/panel/main.go`, the vhost reconcile after the listener and the Agent dial, is read from the source, not measured here).
  Header survival after a return and a second forward update: NOT-MEASURED (no header to survive; the fixture origin
  serves one candidate per lab).

## Method, runs, deviations

Builds (`build/`): `run-upd1.sh build ead69cdcc` → baseline b2b43d0cd680 (ead69cdcc + the release policy labelled
alpha.81) installed fresh; `run-upd1.sh build --baseline-ref v0.1.0-alpha.81 ead69cdcc` with `UPD1_REUSE_DIST_OF_COMMIT`
→ the published alpha.81 (a0beb7263, archive 3350ff44…, set3's dist reused read-only) and the start-check candidate
a9acedb45 (`trees.txt`); both proven (`*-prove.json`), dry runs without a lab. Go 1.26.5. Guests: Debian 13 (nginx 1.26.3,
PHP-FPM 8.4.26), Ubuntu 24.04.4 (nginx 1.24.0, PHP 8.3.6), Arch (nginx 1.30.5, PHP 8.5.11). Fresh cells:
`set11_trial.Set11Trial` over set10's driver (set6's steps, set10's S0 and h-i, then the sections above); F:
`Set11UpdateTrial` = set10's update sections over set5's start-check cell. Browser: the driver's handover windows (set7's
protocol) during B1, B4 and after the sections; `tools/browser-runner.sh` ran `cold-load-set11.mjs` for each.

Runs: `debian13/run-a` stopped after a harness error (an argument name collision, `TypeError` from A1 on; S0, h-i and C
measured; no driver sums); `debian13/run-b` complete, its D/E window ran only `slow35` (the second sign-in check looked
for a Settings link the setup page does not have; fixed); `debian13/run-c` complete and decisive. Harness changes during
the run, each between labs: the watcher argument renamed (copy d); the probe-timing check of copies c–e compared the read
times of two independent watchers below a millisecond (A1/A2 of run-c, A1/A3 of run-b "failed" on it by 0.15–0.4 ms) — it
measured nothing, is kept as detail from copy f on, and `tools/table.py` recomputes those four verdicts without it (marked
"re-judged"; all four become PASS; every other expectation of those cells held); the browser script's sign-in check and
its `LIVE_PROFILE_ROOT` (E on Ubuntu). Screencast frames were pruned at staging (`*/browser/*/PRUNED.txt`).

## Harness files, offline suite

New, not committed by this run: `deploy/e2e/release-recovery/set11_trial.py`, `guest_set11_native.py`, `run-set11.sh`,
`test_set11_trial.py` (8 offline tests); `web/tools/browser-inspect/cold-load-set11.mjs` (committed in 224bbff03 by
someone else, byte-equal to what ran last; SHA-256 6c132f70…). No existing file changed. Working tree = run copy f
(`harness-run-copy/working-tree-against-copies.txt`). Offline suite: copy a (pristine) 1033 tests, copies e and f 1041,
the same 5 errors by name (`harness-run-copy/*/suite-notok.txt`), as in set10.

## Secrets, host, leftovers

`secret-scan.txt` (set10's scanner): no unexplained hit; class 11 had no input (the window password files were removed
when each window ended); its 5 informational path matches were the pattern text of `tools/secretscan.py` and
`tools/scrub.py` (the scanner's patterns did not cover JSON-escaped Windows paths: see "Corrections after intake").
SSH key comments are `celikpanel-lab`; the machine's name occurs in no file. Local scratch,
repository and temp paths in this folder's own host files became `<scratchpad>`, `<repo>`, `<user-temp>`
(`host/scrubbed-files.txt`; the `browser/` folders and escaped forms were not covered by that pass and were fixed at
intake); driver files are unchanged. Disk (`Get-PSDrive C` every 30 s, 281 readings
23:54:30Z–02:14:56Z): 107.40 GiB at the start, lowest and last 77.53 GiB; 6 gate decisions, all allowed
(`host/c-drive-cells.txt`); neighbour gate clear before each guest start. Power: keep-awake 23:54:30Z–02:15:33Z;
the System log shows modern standby entered 21:19:54Z (before the run) and left 01:09:07Z, entered 01:44:10Z and left
02:05:15Z (`host/sleep-events.txt`) while the disk watcher read every 30–31 s (largest gap 31 s,
`host/watch-gaps.txt`); guest clocks were not compared. By those times the Debian run-c browser windows (00:53–00:57Z)
and the start-check cell's reboot (01:49Z) fall inside a "standby" interval; the watcher shows the host running
throughout, and the browser's own timings (15.0 s, 30.0 s, 40.0 s) match their nominal values. Overlays of all six labs removed after staging
(`host/removals.txt`). Left on the WSL host: `/var/tmp/cp-set11-run` (462 MB), six `/var/tmp/cp-release-drill-s11-*`
(records, no disks, 400 MB), the builds `/var/tmp/cp-upd1-build/20261010t235526z` and `20261011t000145z` with eight new
dist folders under `/var/tmp/cp-pair-accept/dist/`. Nothing of this run is running. A shell loop
`until ! pgrep -f stage-common.sh …` (pid 203750), older than this run and matching its own command line, still runs;
it was left alone. `SHA256SUMS` was written last.

## Corrections after intake (2026-10-11)

Intake check before this folder is committed to the public repository (read-only against the raw files, then the
corrections below, inside this folder only). Nothing of any measurement was changed; only redactions, narrowed
sentences and the sums. What was checked: root and per-run sums; secret classes in text (PEM, WireGuard keys, passwords,
tokens, cookies, licence keys, crypt hashes, 32/40/64-hex by key name, base64 runs of 80+ characters decoded, token
digests, `.release-db-migrations/` names, SSH key comments, JSON/JSONL parse, network logs for cookie/bearer/sign-in
bodies); the 73 distinct screenshots by content (39 files looked at, covering every page kind: sign-in without typed text, setup page,
the four wait pages, the offline pages, Chrome's error page; the rest are spinner or timing variants of those); outside
names and paths; every README cell against its raw file; build identity; host, disk and hygiene.

Changed:

- **Paths.** The Windows user name, the scratch-folder identifiers and the checkout path stood in
  `debian13/run-b/browser/h11d13-d-e/{afterfail,afterstart,sw}/run.json` (a puppeteer stack trace, JSON-escaped),
  `host/browser-runner.log` and `tools/browser-runner.log` (the same trace), `tools/scrub.py` (constants) and
  `tools/sums.sh` (checkout path). `secret-scan.txt` counted 5 path matches and called them pattern text; its pattern
  and `tools/scrub.py` covered only single-backslash forms and skipped `browser/` folders. Now `C:\\Users\\<user>\\…\\scratchpad`
  reads `<scratchpad>`, `file:///C:/…/celikpanel` reads `file:///<repo>`, and the two constants in `tools/scrub.py` are
  `<windows-user>` and `<session-scratch-dir>` and the checkout's parent folder `<repo-parent-dir>` (the script is
  illustrative now). `host/scrubbed-files.txt` is the
  scrubber's own list and was not edited. Search result after the change: no user name, host name or scratch identifier
  anywhere in the folder.
- **E.** Heading "cause established" → "cause narrowed to the lab browser's on-disk profile (path-length mechanism
  inferred)"; the table cell likewise. "(177 characters to the profile)", "`…/CacheStorage` stayed empty (213 characters;
  an entry under it is about 316 …)" were in no raw file: the lengths are now said to be computed at intake from the folder
  layout, and the directory listing is said not to exist. The Ubuntu short-path run is now said to be a different guest
  and browser process, not a controlled comparison; the Debian in-memory context in the same process is named as the
  control that exists (`sw-events-no-session.jsonl`).
- **A2.** "the page's sentence reads …" → the product string it would read, with the statement that the page was not looked
  at (no screenshot of that page exists in this folder).
- **A5.** "the owner's own reload ~1 s before the read" → 00:51:36.43Z, 1.2 s before the watcher started and 2.8 s before
  the read (`native/059`, `060`, `062`).
- **Probe.** "mode `0644`" → final mode `0644`, with the one probe whose first stat read `0640`; "removed within ~4 ms" →
  existed 0.2–4.8 ms between the watcher's readings.
- **A1, A3, A6.** "File untouched" / "file not written" are about the owner's vhost only: these requests also wrote
  `<vhost>.celikpanel-pending` beside it (create, rename; fs events in `native/*-watch-stop.json`; `027-a1-1-stat.json`
  shows it after A1) and left it. Not mentioned in the original.
- **B4.** Added the backup copy's size and digest (2241 bytes, equal to the owner's file, Debian run-c).
- **D2b.** "never reached the Panel" → the document request itself failed (`ERR_CONNECTION_CLOSED`) and no `/api` request
  was made; Debian "(0.3 s after the restart)" → the first read was answered 289 ms after the navigation call, 0.19 s
  after the restart-done record.
- **F.** "failed in `configurePanelHTTPTLS`" → the update-failure line says `tls_pair_invalid`; `configurePanelHTTPTLS` is
  the fixture's recorded defect description; the start order in `cmd/panel/main.go` is source, not measurement.
- **Table.** Debian A1 and A2 carry "(re-judged)"; `result.json` count (five); the standby paragraph says the Debian
  browser windows lie inside the logged standby interval.
- **`tools/sums.sh`.** As run, it left the five per-run driver `SHA256SUMS` out of the root list; the root list now includes
  them.

The re-judged cells (Debian run-c A1 and A2, run-b A1 and A3). The removed expectation compared the create/delete reading
of the inotify watcher with the send/answer readings of the packet reader, two independent readers each late by an
unknown amount; the four differences are 0.05–0.42 ms and in A2 the create reading is 0.15 ms after the first GET
reading, which cannot be an order of events (the Panel creates the file before it asks). Nothing else failed in those
four cells (a recount of every `section.json` at intake: all other expectations `held: true`). Limit: for A1 and
A3 the 200 with the probe's 61 bytes for every name shows the file was there; for A2 the answer (200, 411 bytes of the
site's own page) is the same whether the file is there or not, so there the file's presence rests on the Panel's order
(write, then ask) and on A1/A5 of the same lab, not on a measurement of A2.

Kept as they are, for the reader:

- **A6.** The driver's verdict is PASS (it accepts either answer the docs name) and `cells.md` shows PASS; this README's
  FAIL follows the task's expectation. Raw: `debian13/run-c/driver/steps/17-a6-bound-address/section.json` (`a6`: answer
  `409 include_missing`, `status:"404"`; `gets`: both names `404`; `nginx_logs` `/var/log/nginx/access.log` (the default
  server, not the site's own log) `404 118`; `listening_80_443`: `0.0.0.0:80` and `[::]:80` still open). Ubuntu the same.
- **B4.** `…/21-b4-take-refused/section.json` (`b4`): page `owner_edited`, `reason:null`, `system_error:"EPERM"`; row
  `unwritable`; ledger `unreadable/write_refused`; backup copy present; the owner's file unchanged (inode, sha256).
- **CRLF.** `cells.md`, `checks-not-passed.txt` and `host/watch-gaps.txt` have CRLF line ends (written on Windows); not
  converted. The repository's `.gitattributes` has `* text=auto`, so a plain `git add` normalises them and their lines in
  `SHA256SUMS` will not match a later checkout; commit them as they are (or list them as expected mismatches).
- **`*.log`.** `host/browser-runner.log`, `host/keepawake.log` and `tools/browser-runner.log` match `.gitignore`'s `*.log`:
  commit this folder with `git add -f`, or those three files are silently missing from the commit.
- **Not in raw files.** puppeteer-core 24.10.0 (README only; it matches the module the runner used on the host); the
  baseline fixture commit `b2b43d0cd680…` is in the WSL build clone only, not in this repository (its difference to
  `ead69cdcc` is the one file `deploy/release-sequence-policy`, `build/fresh/trees.txt`); `tools/` are as run apart from the
  edits above.
- **Images.** 162 files, 6.6 MB, 73 distinct contents; no typed text, password, licence key, token or installed-server name
  or address in any viewed frame (the setup page shows the lab's `panel-*.upd1-infra.test` names and `192.0.2.x`
  addresses, the user `labadmin`). Frame pruning was already done at staging (`*/browser/*/PRUNED.txt`; kept counts equal
  the files on disk); the folder is 30 MB, so no further pruning.
