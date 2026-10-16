# set19, 2026-10-16 (UTC): the owner's update from v0.1.0-alpha.90 to the alpha.92 candidate while Certbot renewal is the owner's moving workload (the timer's catch-up run, a failed renewal service), on Debian 13 in a real headless Chrome

A measurement record. The run directory was made at 16:16:04Z and the last lab ended at 17:23:24Z
(`host/progress.txt`); the lab paths were deleted at 17:24:58Z (`host/cleanup.txt`). Page text and pictures show the
host's local time (UTC+3); times here are UTC.

The harness addressed no installed server, no licence service and no certificate authority, and did not contact
celikpanel.net. The release origin's name and the certificate-authority names were pinned to each guest's loopback as
the first driver step and read back at the end (`*/driver/steps/01-set6-name-pinning`, `*set6-name-pinning-at-the-end`:
passed in all four labs). **Outbound TCP/UDP 443 was refused except to loopback** (nftables table `inet cp_set19_lab`,
the lab's own rule; `*/driver/steps/*set19-certbot-lab-preparation/set19-lab-network.txt`); the reject counter at the
end reads `packets 12` in runs b, c and failed (the lab's self-test to 203.0.113.1 and refused attempts; the log lines
are not kept). Every archive is a test-licence (`acceptance_license`) build; none is a release. Every `result.json`
carries `native_evidence: false`; this record closes no P0 row. This is a measurement, not acceptance evidence: QEMU/KVM
guests, test-licence builds, lab changes to the owner's Certbot units listed below.

**Outcome in one line (observed in these labs; not acceptance evidence).** With the renewal timer made to start its
catch-up run when the update turns it back on (run c), the renewal ran during the update's last step (17:07:37Z, 1 s)
and the update ended `succeeded / update_verified` 2 s later with no failure and no recovery
(`previous_failure=none`); with the renewal service failed before the update and failing again when the timer was
turned back on (cell failed), the update ended the same way and left the owner's failed state as it was. The page
(alpha.90's, the version that runs during the update) drew the five steps in the primary colour and "Updated to
v0.1.0-alpha.92" in green, never red; after its reload the candidate's update card showed "Updated to" with the five
steps, and the build line copied in Chrome as exactly `build 16cbfc877bd4`.

## Source and builds (`build/`)

Source: `f9198bdc1905b969ee04d0b95c455d7a309fb28e` on `fix/alpha92-certbot-scheduler-restore` (the updater, recovery and
page changes of alpha.92 and the release preparation 92 after 91; `host/source-check.txt`), cloned from a git bundle of
the branch and the v0.1.0-alpha.90/91 tags, never from a working tree. Later commits on the branch change only harness
files under `deploy/e2e` (run copies b and c below) and documents. Go 1.26.5 (`host/hostcheck.txt`), build
16:16:38Z-16:20:58Z, `build-set19-artifacts.sh`.

| Role | Version / seq | Commit | Archive SHA-256 |
| --- | --- | --- | --- |
| baseline (installed) | v0.1.0-alpha.90 / 90 | 1743ac69 (the tag commit, unpatched; rebuilt locally, not the published archive) | `f60537727fb83a065deeaf532687154f0db2a17c5ae57237673d1a6c6ccb9872` |
| good (installed by the update) | v0.1.0-alpha.92 / 92, previous 90 | 16cbfc877bd46608f8ff37e2a37905a21941db29 (the source + release policy only) | `502c945adae41c3c2a659ddf8cbe010655a3f5203a1ff525941c4e3ad0fc3970` |
| defective (never installed) | v0.1.0-alpha.92 / 92 | 967f5ff24942782289e05fbc9b9b83e0072e2b90 | `149450675ae4c297a23ef3f55290bd91eba43af5db62d55f6c81b2654c76061a` |

The candidate is a policy-only fixture because the fixture origin serves exactly one transition from the installed
baseline; the source's own policy is 92 after the alpha.91 tag commit. No migration between the alpha.90 tag and the
source (ledger 44). Proof (`set19_fixture.py prove`, rc 0) and dry runs of both cells (rc 0) from run copies a and c.

## Lab preparation (not a product change; before the pre-update state)

- **Network.** nftables `inet cp_set19_lab`: outbound TCP/UDP 443 refused except loopback.
- **The catch-up (runs b, c and the failed cell).** `/var/lib/systemd/timers/stamp-certbot.timer` dated three days back
  while `certbot.timer` is active. From run c: a timer drop-in
  `/etc/systemd/system/certbot.timer.d/50-set19-lab-no-random-delay.conf` with `RandomizedDelaySec=0` and a
  `daemon-reload`, written before the update so that it is part of the timer definition the update captures and must
  keep. Why: in run b the aged stamp was read when the update turned the timer back on (`LastTriggerUSec` = the aged
  stamp), but systemd moved the missed elapse to the boot time and added the timer's random delay (up to 12 h); on a
  guest booted minutes earlier that landed at 21:51Z, so no renewal ran. On a server that has run for days the moved
  elapse is often already past; that is the production race (Frankfurt).
- **The failed service (cell failed).** A service drop-in
  `/etc/systemd/system/certbot.service.d/50-set19-lab-fail.conf` (`ExecStart=/bin/false`) and one start: the renewal
  service `failed` (exit status 1) before the update.

## Cells (Debian 13 only; one new lab each, one at a time)

| Run | Lab, UTC | Owner's press | Renewal during the update | Outcome | Raw |
| --- | --- | --- | --- | --- | --- |
| a (preparation after the pre-update state; H1) | 16:21:49Z-16:37:19Z | 16:34:25.6Z | not read (H1) | `update_verified`; overall `failed` by the lab-made firewall verdict (H1) | `debian13/catchup-a-preparation-after-pre-state` |
| b (random delay not removed) | 16:38:58Z-16:52:50Z | 16:50:20.0Z | none: next elapse 21:51:52Z, `certbot.service` never started | `update_verified`; `complete-for-review` | `debian13/catchup-b-random-delay-not-removed` |
| **c** | 16:54:54Z-17:09:06Z | 17:06:27.2Z | **started 17:07:37Z by the update's timer start, finished 17:07:38Z (`Result=success`)** | **`update_verified` 17:07:40Z, `previous_failure=none`; `complete-for-review`** | `debian13/catchup-c` |
| **failed** | 17:09:22Z-17:23:24Z | 17:20:26.1Z | failed 17:18:15Z (lab), **started again 17:21:48Z by the timer start, failed (exit 1)** | **`update_verified` 17:21:58Z, `previous_failure=none`; `complete-for-review`; the service stays `failed`** | `debian13/failed` |

In every lab the `verdicts` step passed (site, mail, cron; Panel down only during the transaction), the timers compared
equal before and after the update (`terminal`, `timers.equal`), and the request's recovery observation is
`phase=succeeded terminal_proof=update_verified previous_failure=none` with no `.failure` sidecar; the recovery runner's
journal shows only its idle timer runs. The renewal readings are in `*/driver/steps/*set19-certbot-readings/`.

## The page (`*/browser/page/`, sampled every 500 ms)

Runs b, c and failed: alpha.90's page (the version that runs during the update) showed "Updating to v0.1.0-alpha.92"
with the five steps advancing (download, check the signature and checksums, check the server, install and restart,
verify) on the primary surface, then "Updated to v0.1.0-alpha.92" on the success surface and "The update completed.
Reloading this page with v0.1.0-alpha.92…"; no `[role="alert"]` and no red at any sample. After the reload the
candidate's card showed "Updated to v0.1.0-alpha.92", five steps done, and the build line copied by the browser's own
selection as exactly `build 16cbfc877bd4` (title `full commit 16cbfc87…`); alpha.90's sidebar stamp in the same run
reads "build 1743ac69edb0 1743ac69edb0e5723dc60d992b53094dcc4b1431" (the copy defect the owner reported).

**Not observed here:** the candidate's calm "Finishing the update" view and its "One step was repeated" line. They are
drawn only when the updater's last step fails and recovery repeats it; with the corrected updater that did not happen.
They are covered by the mounted web tests, not by this record.

## Findings and harness changes

- **H1 (run a).** The lab preparation ran after the pre-update state was read, so its nftables table made the
  terminal comparison report "firewall ruleset changed", and the read-only renewal readings, which needed the terminal
  step, were skipped. Run copy b (`harness-run-copy/b`) moves the preparation before the pre-update state and makes the
  readings need only the owner's start. The update itself was verified.
- **H2 (run b).** The random delay of `certbot.timer` kept the catch-up out of the measurement (above). Run copy c
  (`harness-run-copy/c`) adds the `RandomizedDelaySec=0` lab drop-in.
- **Not read.** The updater's own lines naming the renewal run ("ran when its timer was turned back on", "not an update
  failure") go to the Agent's worker, which keeps the updater's output in memory (set17b H1); they are not in the
  journal and were not read. The contract tests cover their text.
- No product defect was found.

## Files

`build/` (artifacts document, dist records, fixture commits, proof, pins, dry runs, build log tail, the preparation
texts of run copy c), `host/` (progress, host check, neighbour gate, source check, cleanup), `harness-run-copy/b|c`
(overlay hashes and diffs), `tools/` (the run's scripts), `debian13/<run>/driver` (the driver's evidence directory,
verbatim), `debian13/<run>/browser` (the page tool's output) and `cell.*.txt` (the cell's own stdout/stderr/rc).
