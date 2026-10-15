# set17, 2026-10-15 (UTC): the pre-publication measurement of the v0.1.0-alpha.86 candidate (commit 313f3eea2): fresh install on Debian 13, the owner's update from alpha.85 rebuilt from its tag, good and defective, on Debian 13 and Ubuntu 24.04, the update page during a real update (alpha.85's page as the control, the candidate's own page in a second update to a v0.1.0-alpha.87 label), in a real headless Chrome

A measurement record, the nineteenth native record. Host `date -u` at the start of the work: `Thu Oct 15 13:29:48 UTC
2026` (reported by the run coordinator; not recorded in this folder; the folder's earliest reading is 13:50:45Z,
`host/c-drive-watch.txt`); the run directory was made at 13:50:54Z and the last lab ended at 15:57:57Z
(`host/progress.txt`); the folder's last drive reading is 16:01:51Z (`host/c-drive-watch.txt`); the end readings the run coordinator reports at 16:04:01Z are not recorded in this folder. Page text and pictures show the host's local time
(UTC+3); times here are UTC.

The harness addressed no installed server, no licence service, no certificate authority. celikpanel.net was read once,
outside the labs, by one public GET of the published alpha.85 release manifest (`build/published-alpha85-release-manifest.txt`,
16:02:36Z). The release origin's name and the certificate-authority names were pinned to each guest's loopback as the
first driver step of every lab and read back at its end (`*/*/driver/steps/01-set6-name-pinning`,
`*-name-pinning-at-the-end`: passed in all 7 labs). The guests had outbound NAT (QEMU user networking), so setup
installed its components from each distribution's repositories; the driver's wording "isolated node" does not mean
network isolation. Every archive is a test-licence (`acceptance_license`) build with `release: false`; none is a
release. Every `result.json` carries `native_evidence: false`; this record closes no P0 row. No product file was
changed. This is a measurement, not acceptance evidence and not native evidence: QEMU/KVM guests, test-licence builds,
nothing judged.

**Outcome in one line (observed in these labs; not acceptance evidence).** All seven labs ran to `complete-for-review`; the six update labs' `verdicts` step passed. The
candidate installed fresh, updated from alpha.85 on Debian 13 and Ubuntu 24.04 (`update_verified`), updated itself to
a v0.1.0-alpha.87 label twice (`update_verified`), and the defective candidate returned by itself on both platforms
(`recovered`, `rollback_verified`); after each return the database comparison was `equal-except-volatile` with
`managed_site_files` changed in `observed_at` only. On the update page the candidate showed the calm restart
(spinner, "The panel is restarting as part of the update…", no warning surface, no simultaneous corner notice, no
"not known" wording) in both of its updates, where alpha.85's page showed the warning surface and "could not be read
just now" in both of its updates. **One defect is recorded (D1):** in one of the two candidate updates the licence
access gate's own dialog ("The Panel is not answering; an update may be restarting it", with "The latest result could
not be read…") replaced the update dialog at T+00:55 for about 5 s, inside the expected restart window, and the
update's navigation lock did not come back; the same dialog appeared on alpha.85's page (Ubuntu control) at T+00:36.

## Source

Candidate: the committed HEAD `313f3eea2c9b89251588648b09484cd34b7aecb1` ("Prepare v0.1.0-alpha.86"), branch
`fix/alpha86-update-restart-notice`, built from `git clone`, never from the working tree. **No fixture commit labels
it**: the plan reported by the run coordinator (not recorded in this folder) was a fixture commit changing only
`deploy/release-sequence-policy` to 86; the release preparer then committed 313f3eea2 (commit time not recorded in this folder), which already carries
that policy (`version=v0.1.0-alpha.86 current=86 previous=85 previous_commit=36c33c45…`) and the bootstrap pins 86, and
changes no file under `cmd/`, `internal/` or `web/` against 21f07e7a8 (`build/source-check.txt`: `git diff --stat
21f07e7a8 313f3eea2 -- cmd internal web` empty; the 7 files it changes are listed there). So the candidate is the
source commit itself, and every candidate page reads `build 313f3eea2c9b`. A later commit, feef819a7 (its content is not recorded in this folder), was HEAD when the run directory was made (`build/source-check.txt`); `git diff --stat 313f3eea2 HEAD -- cmd internal web
deploy/release-sequence-policy` was empty. No migration between the alpha.85 tag and the candidate (ledger 44);
`cmd/panel/main.go` unchanged between them.

## Builds (`build/`)

Go 1.26.5. Job 13:51:28Z–13:57:46Z (`build/job-build.*`), `build-set17-artifacts.sh` from run copy `b`, in disposable
clones under `/var/tmp/cp-set17/cp-upd1-build/`. Test-licence builds, none a release.

| Document | Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- | --- |
| update (`build/update/`) | baseline | v0.1.0-alpha.85 / 85 | 36c33c45 (the tag commit, unpatched; it carries the seam) | `31b03ddd762e20d0a841e728b75df321ba0fe96b15e9d1a568843d49d3245c16` |
| update | good (the candidate) | v0.1.0-alpha.86 / 86 | **313f3eea** (the source itself, its own policy) | `6c64289048f9e3fd98655bf761665c3b1f500ec654007da776d422654f747d39` |
| update | defective | v0.1.0-alpha.86 / 86 | 6544ee8b (313f3eea + set5's migrate-only defect in `cmd/panel/main.go`) | `0397f4e85a3d60b2f82391166a95909cf04e6884ed7e5a2ecb33eb105011dfbb` |
| second (`build/second/`) | baseline (installed) | v0.1.0-alpha.86 / 86 | 313f3eea (dist reused by name) | `6c642890…` (same archive) |
| second | good (installed by the second update only) | v0.1.0-alpha.87 / 87 | c48678d3 (fixture: 313f3eea + `deploy/release-sequence-policy` only) | `887a16c0d3c7c1ce30371dbdc91661aadf6a9bf95663233ef8c35464d026ccfc` |
| second | defective (never installed) | v0.1.0-alpha.87 / 87 | 8c6ac88b | `8227bac036ac5ccdeb7fc29a4f00e1d56fad2d0b98bd2a93023458ffc56d2f86` |

Full fixture commit ids (`build/*/fixture-commits.txt`): 6544ee8b1186ad42b38bd2b3b0cac1c8f07f50b1,
c48678d34583868ab583db4cfb6c97c36e423db6, 8c6ac88b290b6662c8618878661b02857ff80348.

**alpha.85 is a local build, not byte-identical to the published archive.** Local `31b03ddd…`, published
`cb6424b712052df2768f6c16dd6a66c8ea05a13393e71933c38ae639c3fb0792` (read from the published release manifest, commit
36c33c45, size 66146417). The local one is a test-licence derivation (`bin/panel` with `-tags acceptance_license`, an
`ACCEPTANCE-LICENSE-BUILD.txt` notice, a regenerated `SHA256SUMS`).

**Pins** (`build/archive-pins.txt`): alpha.85 archive `bootstrap_release_sequence=85`; every alpha.86 archive 86 / v0.1.0-alpha.86
(the real release commit's pins, not a fixture); the alpha.87-labelled archives keep 86 (the fixture changes the policy
only). Installed copy on the fresh guest: 86 / v0.1.0-alpha.86.

**Proof and dry runs.** `set17_fixture.py prove` for both documents (`build/*-prove.json`; empty stderr, the exit code is not recorded); dry runs of the six
cell kinds wrote their plans with empty stderr (`build/dry-*`); offline suites in copy `c` (`harness-run-copy/c/suite.txt`:
set16 11 tests OK, set15 8 tests OK; set17 files compiled; shell and tool syntax OK).

## Cell × platform

| Cell | Debian 13 | Ubuntu 24.04 | Raw |
| --- | --- | --- | --- |
| 1 fresh install of the candidate, sign-in, licence, setup `web`, readings | PASS (14:30:23Z–14:40:33Z) | not planned | `debian13/fresh-a` |
| 2/3 update alpha.85 → candidate, good | PASS, `update_verified` (13:59:03Z–14:14:40Z) | PASS, `update_verified` (15:12:18Z–15:34:38Z) | `*/upd85-good-a` |
| 2/3 update, defective → automatic return | PASS, `recovered` / `rollback_verified` (14:56:24Z–15:12:04Z) | PASS, same (15:34:51Z–15:57:57Z) | `*/upd85-defective-a` |
| 2/3 ledger after the return: `managed_site_files` JSONL, `stable_sha256` | `equal-except-volatile`, only `observed_at` | same | `*/upd85-defective-a/driver/steps/{10-pre-state,15-terminal}/` |
| 4a update page during the update, alpha.85's page (control) | measured (start pressed 14:09:49.9Z) | measured (15:30:13.4Z) | `*/upd85-good-a/browser/page/` |
| 4b update page during the update, the candidate's own page (candidate → v0.1.0-alpha.87 label) | measured twice: run a (14:15:12Z–14:30:06Z, pressed 14:25:27.2Z), run b (14:40:47Z–14:56:04Z, pressed 14:51:24.3Z); both `update_verified` | not planned | `debian13/upd86-good-{a,b}` |
| 5 browser checks on the candidate (TR and EN) | PASS as measured (fresh, after the updates) | PASS as measured | `*/*/browser/ui/` |

**4b was not done "on the same guest".** The harness configures one baseline → candidate transition per driver process
(owner_update_trial's module-wide labels, one sealed origin target), so a second update on the guest of cell 2 would
have needed a different driver. Instead the candidate was installed as the baseline by the real installer on a new
Debian 13 guest (set up `web_mail`, one static site seeded, as in cell 2) and the owner updated it to a fixture labelled
v0.1.0-alpha.87 (policy only, `cmd/panel/main.go` untouched, good build). The page observed during that real restart is
the candidate's own update page. The run was repeated (b) after D1 was seen in run a.

## How the owner's start was made in the browser (cells 4a/4b)

The update page follows only an update it started itself (its stored marker), so the start was the owner's press in
the browser, not the driver's POST. `set17_trial.py` splits the driver's `arm`: `page-ready` (read-only inspection,
origin proved again, `GET /api/v1/panel/update/check` against the sealed candidate) before the press; then the page's
own `POST /api/v1/panel/update/start` is **held by the browser (request interception; bytes unchanged)** until the
driver has armed its read-only observer for the page's own request id (`arm_<id>`), then released; `owner-start`
adopts the page's start instead of sending one. Measured holds: 1004, 1202, 880 and 1097 ms (observer armed in 0.62,
0.85, 0.55, 0.78 s), below the page's own 15 s start timeout (`browser/page/page-summary.json` `release`). The browser's
network events did not report the answer of the held request (the request leaves through the page's service worker);
the driver reconciled each start with a status read (`steps/12-owner-start` `start_outcome`, `status_after_the_start`),
and the page's own dialog read "The request was accepted; secure status tracking started." 1.3–1.4 s after the press.
In cell 2 Debian (the first page run) the browser waited 20 s for that answer before ending the driver's window, so the
driver's tracking began about 20 s after the start (its `arm` step 14:09:10Z–14:10:11Z); the tool was then changed to
wait 5 s. Tracking reached the terminal state in every cell.

## The update page during a real update (sampled every 500 ms; `browser/page/`)

T+ is from the press. "Probe" is this tool's own unauthenticated `GET /api/v1/panel/availability` every 500 ms through
the forward (answered = any HTTP status). Surface tokens are the light theme's computed colours (`--warning-mark`
245 179 1, `--primary`, `--success`); the card's own box is `border-border`/`bg-surface-subtle` in both versions.

| | alpha.85 Debian (control) | alpha.85 Ubuntu (control) | candidate run a | candidate run b |
| --- | --- | --- | --- | --- |
| applying (spinner, primary) | T+1.9–40.8 s | T+1.9–25.4 s | T+1.9–25.9 s | T+1.8–25.3 s |
| first lost status read (page) | T+41.0 s | T+25.5 s | T+25.6 s | T+25.5 s |
| what the page said then | dialog "The state of this update could not be read just now, so whether it is still running or has finished is not known…", icon WifiOff on `warning-mark/0.2` with `warning` colour; card AlertTriangle in `warning` | same, T+25.9–36.0 s | dialog "The panel is restarting as part of the update. This page reconnects by itself; nothing needs to be pressed.", Loader2 spinning on `primary/0.1`; card the same calm line, spinner | same, T+25.8–55.4 s |
| "not known" wording | 59 samples, T+41.4–70.9 s | 51 samples, T+25.9–51.3 s | 0 | 0 |
| warning surface | 59 samples | 51 samples | 0 | 0 |
| corner notice during the outage | none | none | none | none |
| other dialog over the page | none seen (other dialogs were not sampled in this run; the update dialog itself stayed until T+70.9 s) | licence gate "The Panel is not answering; an update may be restarting it", T+36.5–51.3 s (update dialog gone from T+36.5 s, card kept the warning) | **licence gate, seen at T+55.9 s, gone at T+61.0 s (D1)**; the update dialog did not come back | none |
| probe not answered | T+36.8–68.8 s (31.9 s) | T+20.8–48.2 s (27.4 s) | T+26.3–56.8 s (30.5 s) | T+25.5–53.0 s (27.5 s) |
| page outage (first lost → first answered status read) | 30.1 s (T+41.0 → 71.0) | 26.0 s (25.5 → 51.5) | 35.2 s (25.6 → 60.8) | 30.3 s (25.5 → 55.8) |
| after reconnection | "Update installed, being verified" T+71.4–77.9 s; then the corner notice **and** the card both say "being verified" T+78.4–83.0 s | corner notice and card both "being verified" T+51.8–60.5 s | card "verifying" T+61.0–69.7 s, then corner "The update completed… Reloading" T+70.2–71.2 s (card gone) | dialog "Update installed, being verified" T+55.9–68.6 s, then "The update completed…" with CheckCircle2 on `success/0.1` T+69.1–70.6 s |
| new version on the page | T+85.6 s (after the page's own reload, T+84.9 s) | T+63.0 s | T+71.7 s | T+71.2 s |
| driver's final state | `update_verified` 14:11:12Z | 15:31:13Z | 14:26:36Z | 14:52:29Z |
| update lock | "Check for updates" disabled until the new version showed (enabled from T+85.5 s); navigation dialog until T+77.9 s | disabled until T+63.0 s; dialog until T+36.0 s (licence gate) | disabled until T+71.7 s; dialog until T+55.4 s (licence gate) | disabled until T+71.2 s; dialog until T+70.6 s |

**The bound was not approached.** The longest page outage was 35.2 s from the first lost read (bound 120 s) and the
latest reconnection was T+71.0 s from the press (bound 360 s). The driver's own host loop saw the Panel down for 25–35 s
in each good update (`steps/22-verdicts` `host_panel_windows`, 5 s resolution).

**Toast.** No corner notice was drawn during any outage: in the lab the navigation lock (2 minutes) was still on, so the
full-page dialog was the one notice. On alpha.85, once the dialog closed, the corner notice and the card said the same
"being verified" text together (Debian T+78.4–83.0 s, Ubuntu T+51.8–60.5 s); on the candidate the card alone carried the
followed update, and the corner notice appeared only for the known outcome ("The update completed…", T+70.2–71.2 s in
run a), as the change describes.

## Facts per cell

**1 fresh (Debian 13).** Install 14:31:42Z–14:33:03Z, owner login, licence passed; setup purpose `web` (DNS external)
waited at `access_dns` (`server_setup_access_dns_required`), and the driver's scripted owner call `PUT /api/v1/setup/guidance` (`manual`, 14:37:01Z, `steps/09-set17-browser-window/api/0069-put-api-v1-setup-guidance.json`) moved it to "set up by hand" when the browser window began. `GET /api/v1/panel/version` → `v0.1.0-alpha.86`, commit and Agent commit `313f3eea…`, `agent_matches true`,
`schema_version 44`; ledger `as-expected` (44); installed pins 86 / v0.1.0-alpha.86; `valid_for_seconds` and
`scan_age_seconds` present; domains 0.

**2/3 update, good.** alpha.85 installed, set up (`web_mail`, DNS external), one static site seeded; the owner started
the update in the browser (above). Agent and Panel after the update `v0.1.0-alpha.86 / 313f3eea…`; floor sequence 86;
foundation release-commit 313f3eea; `post-update-facts` passed; HSTS readings before and after passed (`max-age=31536000`).
Database comparison at the terminal step `equal-except-volatile`, `managed_site_files` volatile in `observed_at` only
(stable digest Debian `c74d8879…` = `c74d8879…`, Ubuntu `c9e6b05e…` = `c9e6b05e…`).

**4b second update (Debian 13, twice).** Candidate installed by the real installer, set up, seeded, updated to the
alpha.87 label: Agent and Panel `v0.1.0-alpha.87 / c48678d3…`, floor 87, `equal-except-volatile` (stable `73768e95…`
and `2d6a0b68…` unchanged).

**2/3 update, defective.** Injected fault: one guest reset during the update (Debian 15:07:28Z, SSH back after 8.91 s;
Ubuntu 15:53:19Z, 11.62 s; new boot id each). Outcome `recovered-automatically`; final `phase: recovered, reason:
rollback_verified, previous_failure: update_failed` (Debian observed 15:08:35Z, Ubuntu 15:54:27Z); two automatic
attempts each (Debian 15:07:07Z update→rollback, 15:08:13Z rollback; Ubuntu 15:53:02Z, 15:54:07Z); no owner attempt.
Agent and Panel after the return `v0.1.0-alpha.85 / 36c33c45…`; login, site marker and firewall equal.

**Ledger after the return.**

| | Debian 13 | Ubuntu 24.04 |
| --- | --- | --- |
| verdict | `equal-except-volatile` | `equal-except-volatile` |
| unexpected | `[]` | `[]` |
| `volatile_columns_only` | `managed_site_files: [observed_at]` | same |
| rows before / after | 1 / 1 (none added, none removed) | 1 / 1 |
| changed columns (row 1) | `observed_at` only | `observed_at` only |
| `observed_at` | 1792076687 (15:04:47Z) → 1792076908 (15:08:28Z) | 1792079452 (15:50:52Z) → 1792079661 (15:54:21Z) |
| `stable_sha256` before = after | `7efbb2c802d1a3e7a2d86b8613452e51610cbd692a282a1c1ccf38fec79426bc` | `45c5e79604ed39cafe3b408518e9869945bbbadd8c5024dfb4613f1e5a4d2e3c` |
| `full_sha256` before → after | `579b4061…` → `fc327b7c…` | `c2257e6b…` → `58cd5f67…` |

67 tables compared; five left out as volatile (`audit_logs`, `metrics_samples`, `server_setup_executions`, `sessions`,
`sqlite_sequence`).

## Browser checks on the candidate (`*/*/browser/ui/ui-summary.json`)

| Check | Result |
| --- | --- |
| Components, 27 names and descriptions on real rows | EN and TR: 27 rows, 27 of 27 names and 27 of 27 descriptions equal to the catalogue (`web/src/i18n/screens/server/{en,tr}.ts`), no row outside it, no id missing; in the fresh cell and after all four candidate updates. After the two returns (alpha.85): EN 26/26, TR 24 names / 1 description (alpha.85's English server names), as expected of a return. |
| `document.documentElement.lang` | `tr` on Turkish Components and Dashboard, `en` on English, on the candidate; `en` on Turkish pages after the returns (alpha.85, set16 O1). |
| Dashboard Recent activity | sentences, no raw id and no underscore word in EN or TR (7 rows each) after setup (fresh: "prepared Nginx during setup", "kurulum sırasında Nginx bileşenini hazırladı", from `server.setup.child:nginx`) and after the updates ("changed a backup schedule" from `backup.schedule:weekly`, "added a domain" from `domain.create`); after the returns alpha.85 shows the raw ids again (6 of 7 rows). |
| Sidebar build id | `build 313f3eea2c9b` (candidate), `c48678d34583` (alpha.87 label), `36c33c4531ee` (after returns): one line at 1280×800 and 390×844, no overflow, in every lab. |
| Store state badges | after the updates (one subscription seeded): EN "Available" 2, "Included" 8, "Coming soon" 10; the status filter lists "State not known yet" separately from "Unsupported here" (TR "Durum henüz bilinmiyor", "Bu sunucuda desteklenmiyor"); no offering was in the not-known state. Fresh cell: "No subscriptions — Create a customer and plan first." |
| A component's own page on `SERVICE_STATE_UNVERIFIED` | produced by a **lab preparation** in the fresh Debian cell only (`debian13/fresh-a`; the stored record's `data` replaced by bytes that do not decode; recorded in `steps/09-set17-browser-window`). `/services/nginx`, EN and TR: two `GET /api/v1/managed-services` → 503 with the new refusal text; the quiet line "The stored record could not be verified, so CelikPanel is checking the components now. Nothing was changed and you do not need to do anything…" (TR "Kayıtlı kayıt doğrulanamadı; bu yüzden CelikPanel bileşenleri şimdi kontrol ediyor…") from 0.5 s; one `POST /api/v1/managed-services/scan?max_age_seconds=300` 200 (980 / 1153 ms); the record read again 200; the page filled at 1.3 / 1.6 s; no alert or toast, one scan only. |
| Setup final step's resolver sentence | not reachable: the step needs the server's public reverse DNS name, which no lab guest has; not imitated. |

## Defects and observations

**D1 (observed, timing-dependent; also seen on alpha.85's page, so not specific to the candidate's restart notice): the licence access gate draws its
own "not answering" dialog over the update's restart and ends the update's navigation lock.** Candidate run a: the
last good `GET /api/v1/license/access` was at 14:25:22.9Z; at 14:26:22.8Z and 22.9Z (T+55.6 s, inside the restart)
access reads failed with the Panel down, and at T+55.9 s the page showed `accessHold` cause `update`: "The Panel is not
answering; an update may be restarting it … Update and recovery status … The latest result could not be read. Any
result below is the last verified observation, not confirmation that work is still running. Check again; do not start
a second update." with "Check this operation" / "Check panel access" (`debian13/upd86-good-a/browser/page/state-05-viewport.png`).
The update's own calm dialog was gone from that moment and did not return (navigation lease released for good); the
card underneath still read "restarting"; the gate closed by T+61.0 s when access answered. Run b reconnected 57.9 s
after its last good access read and did not show it; alpha.85 Ubuntu showed the same gate at T+36.5 s (41.8 s after its
last good access read). The access read log (`browser/page/requests.jsonl`) does not explain when the gate appears: access reads failed in all
four runs (run a at T+40.8 and T+55.6 s, gate at T+55.9 s; Ubuntu control first at T+21.5 s, gate at T+36.5 s; run b once at
T+42.6 s, answered at T+55.5 s; Debian control at T+55.1 and T+63.0 s, no gate seen, the other dialogs not being sampled
there), so the mechanism is not established by this folder. Whether the owner's own restart (reported by the release
preparer as T+02:59, not recorded in this folder) would show the dialog is not established; if it does, the "not known"
wording reaches the owner inside the expected window through this second surface. Reproduce: start an
update in the browser and keep the Panel down past the access validity of the last good access read. Classification:
verified observation in this lab (one of two candidate runs, one of two control runs); not a measured failure of the
update.

**O1 (harness, not product): the answer of the held start POST is not reported by Chrome's network events** (the
request leaves through the page's service worker); the start was reconciled by status reads.

**O2 (alpha.85, for the record): the corner notice and the card repeat each other** after the navigation lock closes
(both "being verified"), the "two notices" the owner described (that report is not recorded in this folder); not seen on the candidate.

## Open items

- The foundation file after each return still names the defective candidate (`release-commit=6544ee8b…`, sequence 86)
  while the running build is alpha.85; the floor reads 86 (`15-terminal/step.json`). The pattern of set12, set15, set16.
- A restart longer than the bound (6 min / 2 min) was not produced; the overdue wording was not seen natively.
- The setup final step ("Finishing setup") and the resolver sentence: not reachable without public reverse DNS.
- The update card's busy state; Arch; a fresh Ubuntu install of the candidate (not planned).
- A Store offering in the "State not known yet" state was not produced.
- set4/set5's PHP-site (item 9) and M10 mail sections were switched off (measured in set4/set5).

## Harness changes in this run (committed with this record)

- New: `set17_fixture.py`, `set17_trial.py` (cells, the split `arm`, the page window, the `unverifiable-record` lab
  preparation), `build-set17-artifacts.sh` (`--baseline-ref v0.1.0-alpha.85`: the candidate is the source itself, refused
  unless its policy is 86 after the tag; `--second`: the alpha.87 label), `run-set17.sh`,
  `web/tools/browser-inspect/cold-load-set17.mjs`.
- The tool was changed twice during the run (`harness-run-copy/tool-edits-during-the-run.diff`): after cell 2 Debian
  the start-answer wait went from 20 s to 5 s and the page's own dialog line is recorded; after run a of 4b every other
  visible dialog is sampled (`other_dialogs`). Each browser log names the tool's SHA-256 it ran (`node-run-*.txt`):
  `e81bc9c8…` (cell 2 Debian), `c5acbde8…` (4b run a), `5e8fb715…` (all later windows, the committed text).
- Run copies: `harness-run-copy/b` (build) and `c` (every set17 file; all cells ran from `c`), hashes and diffs.

## Run notes

- The first page-window browser (14:09:11Z) failed at once: the launcher passed Node a UNC path that lost one leading
  backslash (`host/browser-attempt1-path-error.txt`, as set16's first attempt); the launcher was changed to
  `//wsl.localhost/…` and the browser ran 14:09:39Z, while the driver waited in its window. The stopped first launcher
  of the same lab's "ui" window also ran once and failed the same way; its lines are interleaved in
  `debian13/upd85-good-a/browser/node-run-ui.txt` before the successful run.

## Secrets, host, leftovers

Redacted at collection by the harness (owner password, session, fixture keys); set6's digest sweep over each staged lab
changed 0 files in five labs and 1 host-side file in each defective lab (`*/host/digest-sweep-at-staging.json`; driver
`SHA256SUMS` verified again). `tools/scrub.py` replaced host paths with `<redacted>` in 45 text files (`host/scrub.txt`);
a text search of this folder finds no private key, token, cookie, user path (a `C:` `Users` directory), unredacted password, or any of the owner's server names and addresses; the project's own checkout path (CELIKBROS PROJECTS/celikpanel, not an owner value) remains in a few text files (browser stack traces, harness diffs, `tools/*.sh`). C: (Get-PSDrive) 359.77 GiB free
at 13:50:45Z, lowest 357.31 GiB (15:50:51Z), last reading 357.49 GiB at 16:01:51Z (`host/c-drive-watch.txt`); WSL `/` 939 GB free at 13:50:54Z
(`host/hostcheck-before.txt`); the later WSL reading (920 GB at 16:04Z) is not recorded in this folder. The labs, the run directory and the dist folders are removed after this
record is committed.
