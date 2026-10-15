# set16, 2026-10-14 (UTC): the pre-publication measurement of the v0.1.0-alpha.85 candidate (commit 89cf88397): fresh install on Debian 13, the owner's update from alpha.84 rebuilt from its tag, good and defective, on Debian 13 and Ubuntu 24.04, in a real headless Chrome

A measurement record. `date -u` on the host (Git Bash) read `Wed Oct 14 22:08:10 UTC 2026` at the start; the run
directory was made at 22:16:46Z, the last lab ended at 2026-10-15T00:01:17Z (`host/progress.txt`), and the end reading
is in the last section. Page text and pictures show the host's local time (UTC+3); times here are UTC.

This folder replaces the uncommitted first attempt of the same name (2026-10-14 10:52Z–11:16Z, candidate 0bea00fa0),
which stopped before its first cell because WSL had no `/dev/kvm`. That attempt was read and then deleted; nothing of
it is kept here. KVM was back for this run: `/dev/kvm` present, `svm` on 16 of 16 processors (`host/hostcheck.txt`).

The harness addressed no installed server, no licence service, no certificate authority and no celikpanel.net. The
release origin's name and the certificate-authority names were pinned to each guest's loopback as the first driver step
of every lab and read back at its end (`*/*/driver/steps/01-set6-name-pinning`, `*-name-pinning-at-the-end`: passed in
all 5 labs). Nothing else was blocked: the guests had outbound NAT (QEMU user networking), so setup installed its
components from each distribution's repositories. The driver's fixed wording "isolated node" does not mean network
isolation. Every archive is a test-licence (`acceptance_license`) build with `release: false`; none is a release. Every
`result.json` carries `native_evidence: false`; this record closes no P0 row. No product file was changed. This is a
measurement, not acceptance evidence and not native evidence: QEMU/KVM guests, test-licence builds, nothing judged.

**Outcome in one line.** All five labs ran to `complete-for-review`; the four update labs' `verdicts` step passed (the fresh lab has no
`verdicts` step: its setup, readings and browser steps read `observed`, the rest `passed`). The candidate installed
fresh, updated from alpha.84 on Debian 13 and Ubuntu 24.04 (`update_verified`), and the defective candidate returned by
itself on both (`recovered`, `rollback_verified`). After each return the database comparison was
`equal-except-volatile`: `managed_site_files` changed in `observed_at` only, the stable digest equal. No defect of the
candidate was observed in these five labs. The browser checks below passed as measured; three pre-existing observations are listed.

## Source

Built from `git clone` of the repository at the committed HEAD `89cf883971800c39c72c9f8358c58d56aace4194`, branch
`fix/alpha85-setup-finish-wait`, never from the working tree. `build/source-check.txt` (the file carries no time; the run directory was made 22:16:46Z): `git diff --stat
89cf88397 HEAD -- cmd internal web/src` empty, `git status --short -- cmd internal web/src
deploy/release-sequence-policy` empty, `git diff --stat v0.1.0-alpha.84 89cf88397 -- internal/db/migrations` empty (no
migration between the tag and the candidate; the ledger stays 44).

## Builds (`build/`)

Go 1.26.5. Job 22:17:04Z–22:22:49Z (`build/job-build.*`), `build-set16-artifacts.sh` from run copy `b`, in disposable
clones under `/var/tmp/cp-set16/cp-upd1-build/`. Test-licence builds, none a release.

| Document | Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- | --- |
| update (`build/update/`) | baseline | v0.1.0-alpha.84 / 84 | f0157e6c (the tag commit, unpatched; it carries the seam) | `9f86eb4352ce685e064c51c68aab9b592f99612ff3dcca7f71983c9305708dd4` |
| update | good (the candidate) | v0.1.0-alpha.85 / 85 | **d5d673eb** (fixture: 89cf88397 + `deploy/release-sequence-policy` only) | `3913d51374ba8db47fd4acce0af21d0c4150690664809d8932e355dbfdff40ad` |
| update | defective | v0.1.0-alpha.85 / 85 | 524f86dd (d5d673eb + set5's migrate-only defect in `cmd/panel/main.go`) | `77f39013cb3ef7fff68e3495794dd0034bcb4a396f7e8c2bd649db710119be3a` |
| fresh (`build/fresh/`) | baseline (installed) | v0.1.0-alpha.85 / 85 | d5d673eb (the update's good commit; its dist reused by name) | `3913d513…` (same archive) |
| fresh | good (origin only, never installed) | v0.1.0-alpha.86 / 86 | 1d586b73 (fixture, policy only) | `c61c4515068d8580b1f11158a6d30102805aee83f6efd2d71a998e36e09a9455` |
| fresh | defective (never installed) | v0.1.0-alpha.86 / 86 | 4403975c | `da5082d1d79408e556cbc43b1b964be5303e19059f45aad6aca0bcbfcd692aac` |

Full fixture commit ids (`build/*/fixture-commits.txt`): d5d673ebd95eeb80d16f93577c340d4a9d6a9f36,
524f86dd14186f6bc91dbd49ab559c706eb9bd4a, 1d586b7384bb6dfd42a80612c895ff3f76b98c1b,
4403975c7b96957671afe33b7a0746638fbf8752.

**alpha.84 is a local build, not byte-identical to the published archive.** Local `9f86eb43…` (the same digest as the
first attempt's rebuild of the same commit), published `6dc899e5dabaf471978cde47bfde4a0a649b7c3f6941b1431284c84c9743c301`.
It is a test-licence derivation (`bin/panel` with `-tags acceptance_license`, an `ACCEPTANCE-LICENSE-BUILD.txt`
notice, a regenerated `SHA256SUMS`); the ordinary archive of the same commit was not built, so it was not compared.

**Why the candidate is a fixture commit.** The source's own release policy still reads alpha.84 (the tag's, byte for
byte), so it cannot be offered after alpha.84. `build-set16-artifacts.sh` labels it with one fixture commit that changes
only `deploy/release-sequence-policy` (`version=v0.1.0-alpha.85 current=85 previous=84 previous_commit=f0157e6c…`);
`git diff --name-only 89cf88397 d5d673eb` is that one file (`build/update/baseline-ref-proof.txt`, `build/*/trees.txt`).
So every candidate page reads `build d5d673ebd95e`, not `89cf88397…`.

**Pins** (`build/archive-pins.txt`). In every alpha.85- and alpha.86-labelled archive `libexec/get.sh` lines 6–7 still
read `bootstrap_release_sequence=84`, `bootstrap_release_version=v0.1.0-alpha.84` (the fixture changes the policy only;
the real release commit is expected to move them). The installer accepted the fresh install with get.sh 84 and policy 85.
`install.sh` has the same SHA-256 (`e16ac93a…`) in every archive. Installed copy on every guest
(`*/*/driver/steps/*set16-readings*/installed-pins-*.txt`): 84 / v0.1.0-alpha.84.

**Proof and dry runs.** `set16_fixture.py prove` passed for both documents (`build/update-prove.json`,
`build/fresh-prove.json`, rc 0); dry runs of six cells (the five that ran and `set16-ubuntu`, which was not run) wrote their plans with empty stderr (`build/dry-*.json`, `.err`); the offline suites in copy `c`
passed (`harness-run-copy/c/suite.txt`: set16 11 tests OK, set15 8 tests OK). The Agent package closure of each
installed baseline contains no licensing package (`baseline-ref-proof.txt`, last line `0`).

## Cell × platform

| Cell | Debian 13 | Ubuntu 24.04 | Raw |
| --- | --- | --- | --- |
| 1 fresh install of the candidate, sign-in, licence, setup `web`, readings | PASS (22:24:54Z–22:44:41Z) | not planned | `debian13/fresh-a` |
| 2/3 update alpha.84 → candidate, good | PASS, `update_verified` (22:45:01Z–23:00:07Z) | PASS, `update_verified` (23:16:39Z–23:38:07Z) | `*/upd84-good-a` |
| 2/3 update, defective → automatic return | PASS, `recovered` / `rollback_verified` (23:00:33Z–23:16:12Z) | PASS, same (23:38:28Z–2026-10-15T00:01:17Z) | `*/upd84-defective-a` |
| 2/3 ledger after the return: `managed_site_files` JSONL, `stable_sha256` | `equal-except-volatile`, only `observed_at` | same | `*/upd84-defective-a/driver/steps/{10-pre-state,15-terminal}/` |
| 4 browser on the guest panel | PASS as measured (fresh, after each update and each return) | PASS as measured (after the update and the return) | `*/*/browser/ui/` |
| 4 "Finishing setup" | NOT REACHABLE without public DNS (below) | — | — |
| 4 the update card's busy state | NOT MEASURED (below) | — | — |

Cells 1–2 took 51 min 18 s together (22:24:54Z–23:16:12Z), under the 3 h limit, so cell 3 (Ubuntu) ran.

## Facts per cell

**1 fresh (Debian 13, EN desktop 1440×900).** preflight, origin, install (22:26:15Z–22:26:49Z), owner login and
licence passed; setup purpose `web` (DNS external) waited at `access_dns` (execution `waiting`), and the owner chose
"set up by hand" before the browser window (`leave_setup`: `guided` → `manual`). `GET /api/v1/panel/version` →
`version v0.1.0-alpha.85, commit d5d673eb…, agent_commit d5d673eb…, agent_matches true, schema_version 44`. Ledger
`as-expected` (44). Domains `[]`, no first-state row in the page. Licence access carries `valid_for_seconds` (driver
28.532); managed services carry `scan_age_seconds` (129.477).

**2 update, good (Debian 13 / Ubuntu 24.04).** alpha.84 installed, set up (`web_mail`, DNS external), one static site
`upd1-owner.test` seeded; the owner started the update through the Panel's own path. Debian: arm 22:55:05Z, final
`succeeded / update_verified` 22:55:58Z; Ubuntu: arm 23:33:07Z, final 23:33:58Z. Build identity of Agent and Panel
`v0.1.0-alpha.85 / d5d673eb…`; floor `sequence=85`; `post-update-facts` passed (ledger 44); set6 HSTS readings before
and after passed (alpha.84's own `max-age=31536000` as expectation). Domains 1 row (`managed_unchanged`). Database
comparison at the terminal step: `equal-except-volatile`, `managed_site_files` volatile in `observed_at` only (Debian
stable `b8589add…` = `b8589add…`; Ubuntu `522efbc3…` = `522efbc3…`).

**2 update, defective (Debian 13 / Ubuntu 24.04).** Candidate = migrate-only defect; injected fault: one guest reset
during the update (Debian 23:11:06Z, SSH back after 8.26 s; Ubuntu 23:56:04Z, 8.77 s; new boot id each). Outcome
`recovered-automatically`; final `phase: recovered, reason: rollback_verified, previous_failure: update_failed`
(Debian 23:12:07Z, Ubuntu 23:57:09Z); two automatic attempts each (Debian 23:10:50Z update→rollback, 23:11:49Z rollback;
Ubuntu 23:55:50Z, 23:56:51Z); no owner attempt. Build identity after the return `v0.1.0-alpha.84 / f0157e6c…` for Agent
and Panel; login, site marker, SMTP and firewall equal. The terminal step **passed** this time (set15's failed on
`managed_site_files`), and `post-return-facts`, the readings after the return and the browser window ran.

**Ledger after the return (a87ce0b97).** Both defective cells:

| | Debian 13 | Ubuntu 24.04 |
| --- | --- | --- |
| verdict | `equal-except-volatile` | `equal-except-volatile` |
| unexpected | `[]` | `[]` |
| `volatile_columns_only` | `managed_site_files: [observed_at]` | same |
| rows before / after | 1 / 1 (none added, none removed) | 1 / 1 |
| changed columns (row 1) | `observed_at` only | `observed_at` only |
| `observed_at` | 1792019311 (23:08:31Z) → 1792019522 (23:12:02Z) | 1792022021 (23:53:41Z) → 1792022224 (23:57:04Z) |
| `stable_sha256` before = after | `a39f9b86adb746738a57cdeabbbc17537ec9d9aa365efb9faf04272fe0e068cf` | `985eed03e1c4b133d84a5c6ecb999c2b3cbcf88362e2719b701fe9de4936d590` |
| `full_sha256` before → after | `8b83264c…` → `8ac7c439…` | `47655463…` → `a286fdd8…` |

The rows are in `steps/10-pre-state/guest-observation-pre-update-managed-site-files.jsonl` and
`steps/15-terminal/guest-observation-post-recovery-managed-site-files.jsonl` (first line: columns and both digests; then
one JSON line per row). The row is the seeded site's `nginx_vhost` (`/etc/nginx/sites-available/upd1-owner.test.conf`,
`managed_unchanged`); `written_at`, `body_sha256` and `file_sha256` are unchanged. The comparison covered 67 tables and left out five as volatile (`audit_logs`, `metrics_samples`,
`server_setup_executions`, `sessions`, `sqlite_sequence`; `15-terminal/step.json` `database`). This answers set15's open question for
this fixture: only `observed_at` changed after the return.

## Browser (real headless Chrome on the host, `web/tools/browser-inspect/cold-load-set16.mjs`)

Through the driver's loopback forward; pictures and records under `*/*/browser/ui/` (`phases.json` is the summary,
`requests.jsonl`, `cardlog.jsonl`, `maillog.jsonl`, `events.jsonl` the raw records). The first browser start (22:30:41Z)
failed at once because this run's `tools/browser.sh` gave Node a UNC path with one leading backslash
(`host/browser-attempt1-path-error.txt`); the script was fixed and the browser ran 22:32:31Z–22:44:37Z. That is a run-script
error, not a product result; the driver waited for the window as designed.

**Dashboard self-check without clicks.**

| Lab | stored scan at load | POSTs in 90 s | System services card | Mail stacks card |
| --- | --- | --- | --- | --- |
| Debian fresh | 317.3 s | 1 `POST …/scan?max_age_seconds=300` 200, 864 ms | absent 136 ms → "Checking components…" 885 ms → "3 / 3 running / installed" | absent until the scan answered → "Partially installed" (the stored scan, 22:28:16Z, was taken while setup was still installing; the card is drawn only when the profiles show mail activity, `Dashboard.tsx:1113`, so it first appeared with the new scan's answer and its checking state could not be seen here) |
| Debian after the update | 413.5 s | 1, 200, 1025 ms | absent → checking 1046 ms → "6 / 6" | absent 160 ms → "Checking…" with 1 spinner 1028 ms → "Ready" |
| Ubuntu after the update | 557.7 s | 1, 200, 2622 ms | absent → checking 2645 ms → "6 / 6" | absent → "Checking…" with 1 spinner 2626 ms → "Ready" |
| Debian after the return (alpha.84) | 462.7 s | 1, 200, 1908 ms | checking 1939 ms → "6 / 6" | alpha.84's "Status needs refresh … Open Components and rescan" 1911 ms → "Ready" |
| Ubuntu after the return (alpha.84) | 641.4 s | 1, 200, 1492 ms | checking 1531 ms → "6 / 6" | alpha.84's old wording 1496 ms → "Ready" |

On the candidate "Open Components to rescan" was never seen; after the returns the old wording is alpha.84's own, as
expected of a return. Components right after the Dashboard: 0 POSTs in every lab; with the scan stale (fresh cell,
317.1 s) 1 conditional POST, 200, 873 ms, no alert, no toast.

**Licence gate with the guest clock moved (fresh cell; lab preparation, NTP off, undone at the end).** Before the
shift the guest read +1.072 … +1.078 s against the browser (access answers, 22:33–22:35Z).

| | guest − browser (from the access answers) | page mounted | access reads | `can_use_panel` | `valid_for_seconds` | old rule (`valid_until` vs browser clock) would refuse |
| --- | --- | --- | --- | --- | --- | --- |
| +90 s | +91.065 … +91.070 s | yes | 3 | true ×3 | 58.418, 59.995, 43.408 | 0 of 3 |
| −90 s | −88.937 … −88.932 s | yes | 3 | true ×3 | 59.107, 59.999, 44.099 | 3 of 3 |

Restored to about +1.05 s (`events.jsonl` `clock-read` `restored`: mid +1.047 s, bounds +1.025 … +1.068 s) and NTP on at
22:41:57Z (`ntp-on` prep, guest clock; `clock_at_the_end` in step 09). Pictures:
`debian13/fresh-a/browser/ui/clock-dashboard-{ahead90,behind90}-known-viewport.png`.

**Components in Turkish with the Agent stopped** (owner action `systemctl stop celikpanel-agent.service` 22:42:00Z on the guest clock, which ran about 1.4 s ahead of the
host's; the host asked at 22:41:58.2Z and the first 503 was read at 22:41:59.6Z).
`GET /api/v1/firewall` 503 `AGENT_UNREACHABLE`. The banner reads "Güvenlik duvarı: Durum bilinmiyor" and the Turkish
`err.AGENT_UNREACHABLE` sentence ("Bu sunucudaki CelikPanel Agent’ına ulaşılamıyor; …"); the English sentence is absent.
Surface (computed, light theme; failure colour `--danger` = rgb(179, 38, 30)): border rgba(245, 179, 1, 0.6), background
rgba(245, 179, 1, 0.2), text rgb(14, 31, 61); no element inside in the failure colour. `celikpanel-agent` occurs 3 times,
each drawn on one line (one line box per token, parent `white-space: nowrap`) at 1440×900 and at 390×844. Pictures:
`debian13/fresh-a/browser/ui/components-tr-agent-stopped-viewport.png`,
`components-tr-phone-agent-stopped-{viewport,full}.png`. After the owner started the Agent (22:44:13Z), one press of
"Yeniden tara" sent `POST /api/v1/managed-services/scan` 200 (850 ms) and `GET /api/v1/firewall` 200 (16 ms); the banner
then read "Güvenlik duvarı: Açık" with no alert (`components-tr-after-rescan-viewport.png`).

**Dashboard "Needs attention" with the Agent stopped.** Turkish: "İlgi istiyor" names the Agent
(`attention_names_agent: true`) with the Turkish sentence and its three commands; the surface is the attention one
(border rgba(245, 179, 1, 0.5), background rgba(245, 179, 1, 0.2), ink text, no failure colour), `celikpanel-agent` ×3
unbroken (`dashboard-tr-agent-stopped-viewport.png`). English: the same with the English sentence
(`dashboard-en-agent-stopped-viewport.png`). In the English load the stored scan passed 300 s while the page was open
(22:44:01.788Z): the page sent one `POST …/scan?max_age_seconds=300` → 503 `AGENT_UNREACHABLE` (3 ms); System services
went "Checking components…" → "Unknown / Could not check: <the Agent sentence>", and Mail stacks "Checking…" (1 spinner)
→ "Unknown: The components could not be checked just now, so the mail status is not known. The System services card
above says why." So the mail card's checking, result and could-not-check states were all seen; "held by a change" was not
produced.

**Sidebar build id.** On the candidate (fresh, both good updates): `build d5d673ebd95e`, unit class
`inline-block whitespace-nowrap font-mono`, the 12-character id one client rectangle, "build" and the id on one line,
no overflow of the sidebar, at 1280×800 and at 390×844 (menu opened). Pictures:
`debian13/fresh-a/browser/ui/sidebar-1280x800-{viewport,stamp}.png`, `sidebar-390x844-{viewport,stamp}.png`. After
the returns (alpha.84) the id breaks at 1280×800 (`id_on_one_line: false`, two rectangles), alpha.84's own `break-all`
stamp; the 390×844 read after the returns found the stamp with empty boxes (no rectangles, width 0), so nothing is claimed
there.

**"Finishing setup" was not reachable.** The fresh cell's setup (purpose `web`, DNS external) waits at its public-DNS
step, `access_dns`, and the final step "Verify the prepared server" needs the server's reverse DNS name, which no lab
guest has. This record only shows the phase setup waited at; it does not imitate the finishing state.

**The update card's busy state was not measured.** The browser tool has no Settings phase, and no fixture step holds the
server busy while the update card is open. Not attempted; nothing is said about it.

## Defects and observations

No defect of the candidate was observed in these five labs. Pre-existing observations (the files behind O1 and O2 and
O3's `auditActionText` are unchanged between v0.1.0-alpha.84 and this candidate), not changes of this branch, recorded and not
judged:

- **O1 `<html lang="en">` on Turkish pages.** `bannerFacts` read `html_lang: "en"` on the Turkish Components page.
  `web/index.html:2` is `<html lang="en">`; the app sets `document.documentElement.lang` only in
  `web/src/offline/page.ts:15`. Reproduce: open any page in Turkish, read `document.documentElement.lang`.
- **O2 an English component name on the Turkish Components page.** "Firewall engine" and "nftables — the panel
  firewall's packet-filtering engine" come from the server catalogue (`internal/core/managed_services.go:915-916`)
  (`components-tr-agent-stopped.txt`).
- **O3 raw audit action ids on the Dashboard.** "Son etkinlik" lists `server.setup.guidance`,
  `server.setup.revise`, `server.setup.child:*` (`dashboard-tr-agent-stopped-viewport.png`); the English "Recent activity"
  shows the same ids (`clock-dashboard-behind90-known-viewport.png`). `auditActionText` in
  `web/src/components/Dashboard.tsx:120` maps only login and mail-profile events and returns every other action as is.

## Open items

- The foundation file after each return still names the defective candidate (`release-commit=524f86dd…`, sequence 85)
  while the running build is alpha.84; the floor reads 85 (`15-terminal/step.json` `foundation`, `floor`). The same
  pattern as set15 and set12; not judged here.
- The update card's busy state; "Finishing setup"; Arch; a fresh Ubuntu install of this candidate (not planned).
- The mail card's "held by a change" state was not produced.
- The fresh cell's `result.json` findings carry the harness's fixed sentence "web_mail was not attempted on the published
v0.1.0-alpha.85 baseline (it accepts the plan but fails at 05-mail_profile)" (`owner_update_trial.py:4063`). This run
  installed purpose `web`; the sentence is not a measurement of this run.
- set4/set5's PHP-site (item 9) and M10 mail sections were switched off (measured in set4/set5).

## Harness changes in this run (committed with this record)

- `set16_fixture.py`: `SOURCE_COMMIT` = 89cf88397 (was 0bea00fa0; used in the provenance strings); docstrings of
  `set16_trial.py`, `run-set16.sh` and the tool name the commit.
- `cold-load-set16.mjs`: `SURFACE_FACTS` (computed border, background and text colours of each `[role="alert"]`,
  elements in the failure colour, and the line boxes of each `celikpanel-agent` token), used on the Turkish Components
  page at 1440×900 and at a new 390×844 load, and on the Dashboard's "Needs attention" section; `node --check` passed.
  `tools/analysis/edit_tool.py` is the edit as applied.
- Run copies: `harness-run-copy/b` (build) and `c` (every set16 file; all cells ran from `c`), hashes and diffs.

## Secrets, host, leftovers

Redacted at collection by the harness (owner password, session, fixture keys); set6's digest sweep over each staged lab
changed 0 files in three labs and 1 host-side file in each defective lab (`*/host/digest-sweep-at-staging.json`; no driver
file changed, `SHA256SUMS` verified again). `tools/scrub.py` replaced host paths with `<redacted>` in 31 text files; a
text search finds no private key, token, cookie or unredacted password. `host/keepawake.log` matches `.gitignore`.
C: (Get-PSDrive) 343.77 GiB free at 22:12:57Z, lowest 338.51 GiB (00:00:28Z) (`host/c-drive-watch.txt`); WSL `/` 939 GB
free at the start. The keep-awake request (22:16:24Z–00:01:56Z) prevented sleep: the disk watcher has a reading every
minute. Labs are removed after the commit (last section of the coordinator's report).
