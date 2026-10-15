# set17b, 2026-10-15 (UTC): the v0.1.0-alpha.86 candidate's update page through a LONG restart outage (about 3 minutes, made by a lab delay to resemble the owner's reported restart), twice, with alpha.85's page under the same outage as the control, on Debian 13 in a real headless Chrome

A measurement record, the twentieth native record. Host `date -u` at the start of the work: `Thu Oct 15 16:51:11 UTC
2026` (reported by the run coordinator; the folder's earliest reading is 17:04:54Z, `host/c-drive-watch.txt`); the run
directory was made at 17:05:07Z and the last lab ended at 18:16:23Z (`host/progress.txt`). Page text and pictures show
the host's local time (UTC+3); times here are UTC; T+ is from the owner's press of the start button.

The harness addressed no installed server, no licence service, no certificate authority, and did not contact
celikpanel.net. The release origin's name and the certificate-authority names were pinned to each guest's loopback as
the first driver step of every lab and read back at its end (`*/driver/steps/01-set6-name-pinning`,
`23-set6-name-pinning-at-the-end`: passed in all 4 labs). The guests had outbound NAT (QEMU user networking). Every
archive is a test-licence (`acceptance_license`) build with `release: false`; none is a release. Every `result.json`
carries `native_evidence: false`; this record closes no P0 row. No product file was changed. This is a measurement,
not acceptance evidence: QEMU/KVM guests, test-licence builds, nothing judged.

**Outcome in one line (observed in these labs; not acceptance evidence).** With the Panel held down about 3 minutes by a lab delay (the update's worker was frozen; a lab change to the update's timing, not a property of the product) (page outage 177.2 s and 166.5 s), the candidate's
page showed the calm restart ("The panel is restarting as part of the update…", spinner, primary colour) for the whole
outage in both runs: no warning surface, no "not known" wording, no licence gate dialog, no access hold, no corner
notice before the result; the licence gate's own access reads failed 39 and 35 times without drawing anything. alpha.85's
page under the same outage showed what the owner described (that report is not recorded in this folder): "could not be read just now" with the warning surface from T+25.6 s
and the licence gate's "The Panel is not answering; an update may be restarting it" over the page from T+59.8 s until
the Panel answered at T+194.3 s, with the update's navigation lock gone from T+59.8 s. **One finding against the brief's
expectation (F1):** on the candidate the update's navigation lock (its full-page dialog) ended at T+120 s in both runs,
in the middle of the outage, by the documented two-minute lease of the measured commit ecd2a5f08; the card underneath kept the calm restart
line and "Check for updates" stayed disabled until the new version showed. `PANEL_STARTING` was never answered to the
page (O1), so the session gate's new path is still not exercised natively. Every update ended `update_verified`, and the
database comparison was `equal-except-volatile` with `managed_site_files` changed in `observed_at` only.

## Lab preparation: how the outage was lengthened (not a product change)

**The planned drop-in was not used, because the updater refuses it.** Before it publishes its active marker the
updater installs its own guard drop-in for `celikpanel-panel.service` and `celikpanel-agent.service` and requires that
systemd loads ONLY that drop-in (`deploy/release-transaction-guard.sh`, `release_txn_install_unit_guards`: the loaded
`DropInPaths` must equal exactly `…/celikpanel-panel.service.d/10-release-transaction-guard.conf`, otherwise "systemd
did not load only the exact transaction guard for celikpanel-panel.service"; `release_txn_verify_unit_guard_files`:
"guard directory contains unrecognized entries"). A drop-in with `ExecStartPre=/bin/sleep 150` (in `/etc` or `/run`)
would therefore have stopped the update before its first change, not lengthened its restart. No drop-in and no persistent unit file was written in any lab (the helper ran as a transient `systemd-run` unit, below); `units-at-collection.txt` (runs b and control) shows the Panel unit's only drop-in after the
update: `DropInPaths=/etc/systemd/system/celikpanel-panel.service.d/10-release-transaction-guard.conf`, no
`/run/systemd/system{,.control}/celikpanel-panel.service.d`, `TimeoutStartUSec=1min 30s` (unchanged).

**What was done instead** (`deploy/e2e/release-recovery/guest_set17b_restart_delay.py`, committed; uploaded to each
guest's private lab directory and started by the driver after the owner's start (armed 7.3 to 7.9 s after the press), as the transient unit
`celikpanel-lab-set17b-restart-delay-<request id>.service` with `RuntimeMaxSec=1200` and
`ExecStopPost=-/usr/bin/systemctl thaw celikpanel-self-update-<request id>.service`; the exact `systemd-run` argv is in
each `driver/steps/11-arm/step.json`, `set17b_restart_delay_start`): the helper reads the release transaction's marker
directory every 50 ms; when the updater renames `active` to `completion.pending` (the candidate is installed, the Agent
and the Panel are stopped; what follows is the offline database step, the artifact check and the controlled Agent and
Panel starts) and the Panel unit is not running, it freezes the update's own worker unit with `systemctl freeze` (the
cgroup freezer: no signal, nothing killed), waits 150 s, and thaws it. The updater then continues where it was and
starts the Agent and the Panel itself. Between that marker and the Panel start `update.sh` runs no `timeout` command and
no clock-bounded wait, and, per the helper's own header comment, the release recovery watchdog leaves a held transaction lock alone (the watchdog's runs are not recorded in this folder). The 150 s hold, and with it the page outages of 177.2 s and 166.5 s, are made by the lab. Every helper event is in `driver/steps/19-set17b-restart-delay/set17b-restart-delay-events.jsonl`.

| | run a | run b | control |
| --- | --- | --- | --- |
| `completion.pending` seen | 17:40:08.10Z (T+48.6 s) | 17:57:08.60Z (T+41.3 s) | 18:12:06.08Z (T+42.7 s) |
| worker frozen | 115 ms later | 81 ms later | 72 ms later |
| held | 150.2 s; Panel `failed`, Agent `inactive` throughout | 150.7 s; same | 150.3 s; same |
| thawed | 17:42:38.44Z (T+199.0 s) | 17:59:39.35Z (T+192.1 s) | 18:14:36.41Z (T+193.1 s) |
| Agent / Panel active after the thaw | 1.1 s / 1.7 s | 0.6 s / 1.7 s | 0.6 s / 1.6 s |
| worker ended | 12.7 s after the thaw | 12.3 s | 12.0 s |
| helper unit at collection | (reading failed, H2) | `LoadState=not-found`, `Result=success`; no `celikpanel-lab-set17b-*` unit left | same |

**The first attempt (run a0) did not hold anything.** Its helper waited for the updater's informational line
`CELIKPANEL_UPDATE_CHECKPOINT database_verified_before_start` in the worker unit's journal; in an owner's update from the
Panel the Agent's worker keeps the updater's output in memory (`cmd/agent/system_update_worker_linux.go`,
`runFixedSystemUpdateCommand`), so the line never reached the journal and the helper ended with
`worker-ended-before-the-checkpoint` (H1). Run a0 is kept as an ordinary short-outage measurement of the candidate's page
(`debian13/upd86-good-a0-not-held`). The trigger was changed to the marker (run copy d) and the lab was repeated as run a.
The helper was tried first on the WSL host against a dummy unit and a dummy marker directory (both removed); those
tries are not part of this record's measurements (`tools/helpertest*.sh`).

## Source

Candidate: the committed HEAD `ecd2a5f088d676f7cc8a8e571ed3c903ea951f3e` ("Operation guidance: item 16, the session
gate during the new Panel's start, EN and TR"), branch `fix/alpha86-update-restart-notice`, built from
`git clone`, never from the working tree. Against set17's source 313f3eea2 it changes five product files, all under
`web/src` (`App.tsx`, `LicenseOnboarding.tsx`, `SystemUpdateOperation.tsx`, new `systemUpdateRestartSignal.ts` and
`useUpdateRestartStarting.ts`: set17 D1's licence gate grace and the session gate's PANEL_STARTING grace;
`build/source-check.txt`). Its own release policy is `v0.1.0-alpha.86 / 86, previous 85 = 36c33c45` (the alpha.85 tag
commit), so, as in set17, the candidate is the source commit itself; no migration between the tag and it (ledger 44).
`update.sh` prints the checkpoint line at the same lines at the tag and at the source (2893, 4162).

## Builds (`build/`)

Go 1.26.5. Job 17:05:32Z–17:10:52Z, set17's `build-set17-artifacts.sh` unchanged, from run copy `b`, disposable clones
under `/var/tmp/cp-set17b/cp-upd1-build/`. Test-licence builds, none a release.

| Document | Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- | --- |
| update | baseline (control) | v0.1.0-alpha.85 / 85 | 36c33c45 (the tag commit, unpatched) | `31b03ddd762e20d0a841e728b75df321ba0fe96b15e9d1a568843d49d3245c16` (byte-identical to set17's local build) |
| update | good (the candidate) | v0.1.0-alpha.86 / 86 | **ecd2a5f0** (the source itself, its own policy) | `4569b5eb0dba432799be5f63abeec4afca48669ead46151dd10d8ae80ccd72d0` |
| update | defective (never installed) | v0.1.0-alpha.86 / 86 | 8acfb74a | `ab140cf3749d018af75d83d6214f61a10869a36f6b9cc8f241c1e92e232a4736` |
| second | baseline (installed in runs a0, a, b) | v0.1.0-alpha.86 / 86 | ecd2a5f0 (dist reused by name) | `4569b5eb…` (same archive) |
| second | good (installed by runs a0, a, b) | v0.1.0-alpha.87 / 87 | 20a9baa5 (fixture: ecd2a5f0 + `deploy/release-sequence-policy` only) | `34c753c6158ecedfea501d3f9c5494557f12609b61e989bbc00cecdba82cb93e` |
| second | defective (never installed) | v0.1.0-alpha.87 / 87 | 0aa2bb3c | `1df2f933c669c4bf27e81323df1c5d58c8a0ecde6a65d3aa19ecf4f179b60d0c` |

Full fixture commit ids (`build/*/fixture-commits.txt`): 8acfb74a842488c46a8083cca05e2bb10fc1fb97,
20a9baa5d0ccb269528a02ec8c14229bc2ee2493, 0aa2bb3c1394aeee5cc7e153a8b96a0e4d0ee51b. alpha.85 is a local test-licence
build, not the published archive (`cb6424b7…`; see set17). Pins (`build/archive-pins.txt`): alpha.85 `85`; every
alpha.86 archive `86 / v0.1.0-alpha.86`; the alpha.87-labelled archives keep 86 (policy-only fixture). Proof
(`set17_fixture.py prove`, both documents; empty stderr, the exit code is not recorded), dry runs of both set17b cells from copies c, d and e (`build/dry-*`,
empty stderr), offline checks in copies c, d and e (`harness-run-copy/*/suite.txt`, copy b has none: set16 11 tests OK, set15 8 tests OK, set17, set17b and the guest helper compiled, the helper's argument check printed its usage error as intended, shell syntax OK, tool syntax OK).

## Cells (Debian 13 only; one new lab each, one at a time)

| Cell | Lab, UTC | Pressed | Outcome | Raw |
| --- | --- | --- | --- | --- |
| 4b run a0 (helper did not hold, H1): candidate installed by the real installer (setup `web_mail`, DNS external, one static site), owner's update in the browser to the alpha.87 label | 17:12:02Z–17:26:04Z | 17:23:22.3Z | `update_verified` 17:24:33Z; overall `inconclusive` (step 19, H1/H2) | `debian13/upd86-good-a0-not-held` |
| 4b **run a**, same, outage held 150 s | 17:29:15Z–17:44:22Z | 17:39:19.5Z | `update_verified` 17:42:50Z; overall `inconclusive` (step 19's unit reading only, H2) | `debian13/upd86-good-a` |
| 4b **run b**, same | 17:45:22Z–18:01:27Z | 17:56:27.3Z | `update_verified` 17:59:51Z; `complete-for-review` | `debian13/upd86-good-b` |
| **control**: alpha.85 installed fresh, owner's update in the browser to the candidate, outage held 150 s | 18:01:43Z–18:16:23Z | 18:11:23.3Z | `update_verified` 18:14:48Z; `complete-for-review` | `debian13/upd85-control-a` |

Every lab's `verdicts` step passed (web, SMTP, cron never interrupted; DNS external; Panel `down-only-during-transaction`;
API/root agreement passed). After each update: Agent and Panel `v0.1.0-alpha.87 / 20a9baa5…` (runs a0, a, b) and
`v0.1.0-alpha.86 / ecd2a5f0…` (control), `agent_matches true`, floor 87 / 86, `post-update-facts` passed, schema ledger
44 `as-expected`. The start was the owner's press in the browser exactly as in set17 (the page's own POST held unchanged
until the driver's read-only observer was armed: holds 878, 890, 1429 and 893 ms; observer armed in 0.49–0.58 s).
Run b's hold was longer (1429 ms; the cause is not recorded in this folder), still far below the page's 15 s start timeout.

## The update page through the outage (sampled every 500 ms, read until 12 min after the press; `browser/page/`)

"Probe" is the tool's own unauthenticated `GET /api/v1/panel/availability` every 500 ms (answered = any HTTP status).
Surfaces are the light theme's computed colours.

| | run a0 (not held) | **run a** | **run b** | **control (alpha.85's page)** |
| --- | --- | --- | --- | --- |
| applying (spinner, primary) | T+1.7–25.2 s | T+2.0–25.4 s | T+3.5–26.9 s | T+1.6–25.1 s |
| first lost status read | T+25.5 s | T+25.5 s | T+27.0 s | T+25.5 s |
| first answered read after it | T+62.0 s | T+202.8 s | T+193.6 s | T+194.6 s |
| **page outage** | 36.5 s | **177.2 s** | **166.5 s** | **169.1 s** |
| probe not answered | T+23.7–60.3 s | T+23.0–200.4 s (177.5 s) | T+19.3–193.4 s (174.1 s) | T+18.4–194.2 s (175.8 s) |
| driver host loop, Panel down (5 s grain) | 35–45 s | 175–185 s | 170–180 s | 175–185 s |
| wording during the outage | "restarting", 72 samples, T+25.8–61.8 s | **"restarting", 347 samples, T+26.0–202.5 s** | **"restarting", 328 samples, T+27.4–193.5 s** | "could not be read just now … not known", **333 samples from T+25.6 s** |
| surface during the outage | Loader2 spinning on `primary/0.1` | same, whole outage | same, whole outage | WifiOff on `warning-mark/0.2`, `warning` colour; card AlertTriangle in `warning` |
| "not known" wording / warning surface | 0 / 0 | **0 / 0** | **0 / 0** | 333 / 333 |
| licence gate dialog ("The Panel is not answering; an update may be restarting it") | none | **none** | **none** | **T+59.8–194.3 s, 266 samples**, pages held (`data-access-hold="blocked"`) |
| session gate hold ("The panel is starting") / any `[data-top-layer="hold"]` | none | **none** | **none** | only the licence gate's |
| licence access reads during the outage (all failed) | 7 | 39 (T+30.4–200.4 s) | 35 (T+39.6–189.6 s) | 31; the page's update status reads stopped after T+55.5 s (next read at T+194.6 s; 28 recovery-status reads instead, 27 failed and the last answered at T+194.5 s) |
| `PANEL_STARTING` answers to the page | 0 | **0** | **0** | 0 |
| update dialog with the navigation-lock line | T+0.2–76.6 s (until the reload) | **T+0.5–119.8 s; gone from T+120.3 s (F1)** | **T+0.5–120.0 s; gone from T+120.5 s (F1)** | T+0.1–59.3 s; gone from T+59.8 s (the gate) |
| after the dialog, during the outage | – | card alone: "restarting", spinner, T+ running, no corner notice (T+120.3–202.5 s) | same (T+120.5–193.5 s) | the gate's dialog; card "unknown", warning |
| "Check for updates" disabled | until T+76.6 s (new version T+77.1 s) | until T+217.3 s | until T+208.2 s | until T+210.5 s |
| after reconnection | dialog "Update installed, being verified" T+62.3–75.0 s; dialog "The update completed… Reloading" (CheckCircle2, `success/0.1`) T+75.6–76.6 s | card "verifying" T+203.0–215.8 s; corner "The update completed… Reloading" T+216.3–217.3 s | card "verifying" T+194.0–206.7 s; corner "completed… Reloading" T+207.2–208.2 s | corner **and** card "being verified" T+194.8–209.0 s (two notices, set17's O2); corner "Reloading" T+209.5–210.5 s |
| new version on the page | T+77.1 s | T+217.8 s | T+209.2 s | T+211.0 s |
| driver's final state | `update_verified` 17:24:33Z | 17:42:50Z | 17:59:51Z | 18:14:48Z |

**Against the bounds.** The candidate's restart window ends at the later of 6 minutes from the start and 2 minutes from
the first lost read (`web/src/lib/systemUpdateTracking.ts`), here T+360 s. Reconnection came at T+202.8 s and T+193.6 s,
157 s and 166 s before it, so the "not known" wording was correctly not reached (0 samples). The outage from the first
lost read (177.2 s, 166.5 s) passed the 2-minute minimum, which by itself does not end the window. The last good
licence access read was at T−4.8 s and T−5.3 s in the candidate runs; the last failed access read was 205 s and 195 s after it
(39 and 35 failed reads), without the gate's dialog; the control drew it 60.3 s after its last good access read (T−0.5 s).
set17's candidate showed the gate 60.0 s after its last good read in one run and not in a second run that reconnected after
57.9 s; the mechanism is not established (set17 D1).
The 6-minute "overdue" wording was not produced natively (the outage stayed below it, as planned).

## Ledger (database before the update against after it, terminal step)

| | run a0 | run a | run b | control |
| --- | --- | --- | --- | --- |
| verdict | `equal-except-volatile` | `equal-except-volatile` | `equal-except-volatile` | `equal-except-volatile` |
| unexpected | `[]` | `[]` | `[]` | `[]` |
| `volatile_columns_only` | `managed_site_files: [observed_at]` | same | same | same |
| rows before / after, added / removed | 1 / 1, none | 1 / 1, none | 1 / 1, none | 1 / 1, none |
| `stable_sha256` before = after | `4c9f8fb3…` | `3d4b755d…` | `30b8a6bf…` | `b32b3e00…` |
| `full_sha256` before → after | `b8ef6adc…` → `e1af1be2…` | `84e1c59b…` → `7086e477…` | `042bf714…` → `a31f00bf…` | `bba7a4e1…` → `3b8ef060…` |

67 tables compared, schema equal; five left out as volatile (`audit_logs`, `metrics_samples`, `server_setup_executions`,
`sessions`, `sqlite_sequence`). Full digests in `*/driver/steps/15-terminal/step.json`. **No defective cell is part of
this record** (the brief needs none; the defective archives were built only because the artifacts documents require
them, and never installed).

## Findings, defects, observations

**D1 of set17 under a long outage: observed in set17, not reproduced on the candidate in set17b; seen again on alpha.85.** Two candidate runs with
166–177 s page outages and 35–39 failed licence access reads each showed no gate dialog and no hold; the update tracking
went on reading every ~3 s throughout (66–69 status reads). alpha.85's page under the same outage drew the gate's
dialog for 134.5 s and its status tracking paused for 139 s. Qualifiers: set17b's candidate ecd2a5f08 is not set17's candidate (313f3eea2): it contains the licence gate grace written after set17's D1
(Source above); D1 was timing-dependent in set17 (1 of 2 candidate runs), so two runs without it do not show that it cannot occur.
Classification: observed in these labs (0 of 2 candidate runs showed it, 1 of 1 control did).

**F1 (observed in these labs; the documented behaviour of the measured commit ecd2a5f08, which differs from this brief's expectation): the update's navigation lock
ends after two minutes, in the middle of a long restart.** In both candidate runs the update dialog ("Navigation is
temporarily locked while this update is checked. It unlocks automatically after two minutes; tracking continues.",
`panelUpdate.interactionLocked`; `SYSTEM_UPDATE_BLOCKING_DEADLINE_MS = 2 * 60_000` in
`web/src/lib/systemUpdateWatchdog.ts` at ecd2a5f08) closed at T+120.3 s and T+120.5 s while the Panel was still down for another
82 s and 73 s. What followed was calm: the card kept "The panel is restarting as part of the update to
v0.1.0-alpha.87…" with the spinner and the running T+ (`debian13/upd86-good-a/browser/page/state-06-viewport.png`), no
corner notice appeared until the result, no warning, and "Check for updates" stayed disabled until the new version
showed. The brief expected the lock to stay engaged through the outage and after reconnection; it did not (it ended by
its own lease, not through the gate as in set17 D1). Whether the lease should cover the update's restart window
(6 minutes) is a decision left to the release preparer; this record measures ecd2a5f08 only and states no change as made. Reproduce: any owner update whose Panel outage outlasts T+120 s.

**O1 (open): `PANEL_STARTING` never reached the page.** 0 answers in all four runs: after the thaw systemd reported the
Panel active 1.6–1.7 s later and the page's next reads (every ~3 s) were already answered `200`; the page's availability
reads after reconnection read `ready`. The session gate's new grace (`useUpdateRestartStarting.ts`) was therefore not
exercised natively; it needs a Panel that answers while its management is not yet served for longer than the read
interval.

**O2 (not investigated): the Panel unit read `ActiveState=failed`, `SubState=failed` while the updater had it stopped**
(at `completion.pending` and throughout the hold, in runs a, b and the control; `set17b-restart-delay-events.jsonl`); it
returned to `active` at the updater's own start. Recorded, not judged.

**O3 (alpha.85, for the record): corner notice and card repeat "being verified" after reconnection** (control
T+194.8–209.0 s), set17's O2; not seen on the candidate.

**H1 (harness): the updater's checkpoint line is not in the journal of an owner's update** (run a0; see above). Fixed by
the marker trigger (copy d).

**H2 (harness): the step `set17b-restart-delay` read the units under the lab guard's `set -eu`** and ended at the first
absent drop-in directory (`ls` exit 2) in runs a0 and a, so those two labs end `inconclusive` and run a has no
`units-at-collection.txt`; its helper events (written before the reading) are complete and show the release. Fixed with
`set +e` (copy e, runs b and control).

**H3 (harness, as set17 O1): the answer of the held start POST is not reported by Chrome's network events** (service
worker); each start was reconciled by a status read (`steps/12-owner-start`: `running`).

## Browser checks

Only the update page was observed; set17's cell-5 checks (Components, Dashboard, Store, sidebar build id, a component's
page on an unverifiable record) were not repeated: the post-update "ui" window is not opened by `set17b_trial.py`. The
setup final step's resolver sentence is not reachable (it needs the server's public reverse DNS name, which no lab guest
has); not imitated. Console: only `ERR_CONNECTION_CLOSED` during the outage and two `401` before sign-in, in every run.

## Open items

- F1: the navigation lock's two-minute lease against the update's 6-minute restart window (decision left to the release preparer).
- O1: the session gate's `PANEL_STARTING` grace not exercised natively.
- The 6-minute "overdue" wording not produced natively (outage held below it by design).
- O2: why the Panel unit reads `failed` while the updater has it stopped.
- Ubuntu 24.04, Arch: not run in this record. The setup final step and the resolver sentence: not reachable.

## Harness changes in this run (committed with this record)

- New: `set17b_trial.py` (cells `set17b-upd86-debian13-good`, `set17b-upd1-debian13-good-control` over set17's
  `Set17UpdateTrial`; starts the delay helper after the owner's start; step `set17b-restart-delay`; no "ui" window),
  `run-set17b.sh`, `guest_set17b_restart_delay.py` (lab preparation), `web/tools/browser-inspect/cold-load-set17b.mjs`
  (cold-load-set17.mjs plus: 12-minute bound, access holds and held pages, error codes of answers ≥ 400, availability
  states, navigation-lock line over time). set17's files are unchanged and were used as they are.
- Run copies (`harness-run-copy/`): `b` (build), `c` (run a0), `d` (run a: the marker trigger), `e` (runs b and control:
  `set +e` in the collection reading); each with the overlay's hashes and diff. The browser tool's SHA-256
  `fabf2162…` is the same in every run (`*/browser/node-run-page.txt`).

## Secrets, host, leftovers

Redacted at collection by the harness (owner password, session, fixture keys); set6's digest sweep over each staged lab
changed 0 files (`*/host/digest-sweep-at-staging.json`; driver `SHA256SUMS` verified again). `tools/scrub.py` replaced
host paths with `<redacted>` in 32 text files (`host/scrub.txt`). C: (Get-PSDrive) 356.98 GiB free at 17:04:54Z, lowest
356.09 GiB at 18:18:58Z (`host/c-drive-watch.txt`); WSL `/` 941 GB free at 17:05:07Z (`host/hostcheck-before.txt`); the later
WSL reading (924 GB at 18:17Z) is not recorded in this folder. A text search of this folder finds no private key, token, cookie, user path, unredacted password, or any of the owner's server names and addresses; the project's own checkout path (CELIKBROS PROJECTS/celikpanel, not an owner value) remains in a few text files (`tools/*.sh`). The labs, the run directory, the dist folders and the browser profiles are removed after this record is
committed.
