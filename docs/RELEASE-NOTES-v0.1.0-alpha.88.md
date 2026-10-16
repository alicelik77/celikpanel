# v0.1.0-alpha.88 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.88.tr.md)

This is the candidate that follows v0.1.0-alpha.87. It is not published (see
"Publication record" at the end). The number v0.1.0-alpha.88 was chosen by the
release preparer under the owner's standing instruction of 2026-10-13 ("hem
sürüm numaraları hem de commit işlemleri için bana sormana gerek yok": no need
to ask the owner about version numbers or commits); it is not a number the
owner named for this release. The release preparation is commit `7231afdbf`
(sequence 88 after 87, with the alpha.87 release commit
`a8cddcb546302daa0f5eb65b18fe6b9566b69e66` as the previous one). The candidate
is the branch `feat/alpha88-update-steps`, from `main` at `89995199f` (the
record of the alpha.87 publication). These notes were written on 2026-10-16
(UTC).

This release has one subject: how the panel shows its own update.

## The owner's observation and design (2026-10-16)

On 2026-10-16 the owner updated the second installed server (Frankfurt) from
v0.1.0-alpha.85 to v0.1.0-alpha.87. That update ran on alpha.85's own update
screen, which still showed the old warning that the connection was interrupted
and the panel may be restarting, with a corner notice beside it, although
nothing was wrong. In substance, the owner asked: while the update runs, show a
progress bar for each operation; as each one finishes, the next bar appears
below it; when the update has really finished and the panel has restarted,
show the updated state; show nothing alarming such as "connection interrupted,
it may be updating"; there must be a friendlier way. The design agreed with the
owner the same day is the specification of this release. D-024, D-032 and
D-033 apply; no new decision is recorded. The design, the steps read from the
code and the evidence are recorded in
[OPERATION-GUIDANCE](OPERATION-GUIDANCE.md), "The update drawn as one bar per
step, and its restart as a planned step (2026-10-16)".

## What you will see

Plain words first, for the server owner. During an update the update page
shows five short lines, one per step: download the update package, check the
signature and checksums, check the server before installing, install the new
version and restart the panel, verify the new version. A finished step has a
check mark; the current step has a bar; the next steps wait quietly below.
While the panel restarts, its step says only "The panel is restarting; this
step can take a few minutes. This page reconnects by itself." When the new
version answers and is verified, the page says "Updated to" and the version,
then reloads. There is one notice: while the update page is open, no corner
notice repeats it.
Other pages stay locked while the update runs, and the page says so in one
sentence. A warning appears only if the panel has still not answered six
minutes after the update started, or if a failure is verified.

**When you will first see this.** An update always runs on the screen of the
version installed before it. Updating a server *to* alpha.88 therefore still
shows alpha.87's screen (alpha.86's calm restart and single notice). The step
bars are first seen on the update *from* alpha.88 to a later version; that is
why v0.1.0-alpha.89 follows with the same code. Publishing this release updates
no installed server; you start that yourself, from the panel's own update
screen.

## What changed

Product commits: `b1d2ee381` (Agent), `22cdc2c4a` (Panel), `8b96ca5ce` and
`b63da83e0` (page), `07aaa50d5` (the lock sentence). Documents: `78174fc12`.

- **The server declares five steps** (`22cdc2c4a`;
  `cmd/panel/system_update_plan.go`, new; `system_update_handlers.go`;
  `server_lifecycle.go`). The start and status answers carry an additive
  `plan`: `steps` (`download`, `verify`, `prepare`, `restart`, `confirm`, each
  with `id`, `state`, `started_at`, `finished_at`, `elapsed_seconds`,
  `progress`), `expected_restart_seconds` and `restart_bound_seconds`. A step is
  named finished only on evidence (`downloaded_at`, `verified_at`, this Panel
  being the target build, `succeeded`). **Install and restart are one step**
  because the updater stops the Panel before it installs anything: from outside
  the two cannot be told apart, so the page does not draw two steps it could
  not tell apart. Tests: `cmd/panel/system_update_plan_test.go`.
- **The Agent reads the updater's progress** (`b1d2ee381`;
  `cmd/agent/system_update_progress_linux.go`, new; `system_update_rpc.go`;
  `system_update_store_linux.go`; `internal/transport/system_update_contracts.go`,
  additive). It reads the download directory and the release floor with
  `Lstat` only, checks owner and mode, ignores what does not belong to this
  request, and adds `downloaded_bytes`, `downloaded_at` and `verified_at` to the
  status RPC. Read-only; never an input to admission, recovery or the record's
  status. Tests: `cmd/agent/system_update_progress_linux_test.go`, including a
  contract test that pins the `get.sh` lines the read rests on.
- **The restart is measured and remembered** (`22cdc2c4a`). The new Panel
  measures its update's restart once, from `verified_at` to the moment its
  management opened, and keeps it in `panel_settings` under
  `panel_update_last_restart` (`{"request_id", "seconds"}`). The expected
  restart length is max(last × 1.5, 90 s), at most 360 s. The measured span
  includes the updater's own checks before it stops the Panel, so it is at
  least the outage the page sees.
- **One bar per step** (`8b96ca5ce`; `web/src/lib/systemUpdateSteps.ts` and
  `web/src/components/SystemUpdateSteps.tsx`, new; `SystemUpdateOperation.tsx`;
  `PanelUpdateCard.tsx`; `lib/systemUpdateTracking.ts`; `index.css`). The
  dialog, the update card and the compact corner notice draw the steps; a
  measured download shows its percentage, an unmeasured step a sliding bar.
- **The restart as a planned step.** When the page loses the Panel after the
  update was active, or reads `PANEL_STARTING`, the restart bar runs on time
  against the expected length (linear to 90 % at that length, then approaching
  99 %; it never ends by itself) with the one calm sentence above. A page
  opened during the restart draws the same step.
- **"Updated to {version}"** with the new build id once the new version is
  verified, then the page's own reload as before.
- **Overdue and failure wording only when true** (`b63da83e0`). The alpha.86
  overdue wording appears only after the 6-minute window, or on a verified
  failure; past the window the restart step keeps its label only, without a
  bar (browser batch 17 found a stilled bar at 98 % beside the overdue
  wording, which read as almost done).
- **One notice.** The card is the one notice on the update page, for the
  success too; a failure keeps the corner notice and its own wording.
- **The lock sentence simplified** (`07aaa50d5`). While the steps are shown,
  the dialog's lock sentence is now "Other pages stay locked while the update
  runs." (new key `panelUpdate.steps.locked`). Pages without a server plan keep
  the earlier two-minute/six-minute wording (`panelUpdate.interactionLocked`);
  the navigation lock itself is unchanged. Test: `web/tests/update-steps-mounted.test.mjs`.

The navigation lock through the restart (set17b F1), the licence and session
gates' grace (set17 D1), D-032 and D-033 are kept.

## What was measured, and how

- **Web:** `npm test` 1523 of 1523, `npx tsc --noEmit` clean and `npm run build`
  passing with its bundle budget (entry file 242.40 KiB raw, 74.25 KiB gzip), all
  re-run at `07aaa50d5`. `web/tests/update-steps-mounted.test.mjs` has 24 tests;
  every new test fails against the previous source, where the modules and
  fields did not exist.
- **Go** (from the OPERATION-GUIDANCE entry, measured at `b63da83e0` in the WSL
  guest `CelikPanel-S2-Debian`; no Go file changed after it, not re-run for
  these notes): `go test ./cmd/panel/... ./internal/transport/...` 1602 tests
  and 1695 subtests passed, 6 skipped, 0 failed; `go test ./cmd/agent/...`
  1805 tests and 2838 subtests passed, 29 skipped, 0 failed, in the clean mount
  namespace of the 2026-10-14 audit entry.
- **Browser batch 17** (`web/tools/browser-inspect/scenarios-batch17.mjs`,
  against the mock): 9 states in 8 configurations (laptop 1280 and phone 390,
  EN and TR, light and dark); one defect found and fixed (the stilled bar past
  the window), re-shot once: no alarming sentence in any of the 64 states before
  the window, the overdue wording in all 8 after it, no horizontal overflow, the
  lock on through the restart and the confirmation. The screenshots are not in
  the repository. They were taken before `07aaa50d5`, so they show the earlier
  lock sentence; the new sentence was not shot.
- **Release preparation:** `bash deploy/test-release-sequence-policy.sh` passes
  in the Windows Git Bash checkout ("87 -> 88; historical bootstrap sequence
  87").

## Limits of this release

These are known. They are not hidden defects.

- **No native record, by the owner's instruction.** The owner instructed on
  2026-10-16 that this release ships without a native record. Nothing was
  measured on a QEMU guest or an installed server: the Agent's progress read,
  the floor's timestamp and the restart measurement have not been observed
  during a real update. The 90-second default rests on set16 (the Panel
  unanswered for at most 32 s).
- **The step view is first seen on the update from alpha.88.** An update to
  alpha.88 still runs on the previous version's page; v0.1.0-alpha.89 follows
  with the same code so that the steps can be seen.
- **A retry of the same release reports no verify boundary.** `get.sh` does
  not rewrite the floor when the sequence is unchanged; the steps before the
  restart are shown done when the Panel is lost or the new build answers.
- **The `prepare` step may last only seconds;** nothing finer than the floor and
  the Panel stopping can be read.
- **The lock sentence on the finished state.** Beside "Updated to", for the
  moment before the page reloads, the dialog still shows "Other pages stay
  locked while the update runs."
- **The provider is not mounted in a test** (IndexedDB, Web Locks, the
  authentication signal); its wiring is checked from source and in the mock
  browser.
- **The portal and signed-manifest contract tests** were not run for this
  preparation; in the Windows checkout the portal contract stops at its
  canonical-PEM check of the tracked public key (not investigated; the
  alpha.87 preparation ran these contracts in the WSL guest).
- **Open acceptance items.** All five foundation items of the
  [resilience contract](RESILIENCE-CONTRACT.md) remain partial; this release
  closes none and advances none. P0.1–P0.5 stay **partial**. Publishing this
  release would not be acceptance of it.
- **No installed server measured.** Everything the
  [alpha.87 notes](RELEASE-NOTES-v0.1.0-alpha.87.md) list as open stays open.

## Publication record

Not published. Nothing is recorded here until it is published. Open before
publication:

1. The release preparation: done in commit `7231afdbf`, which changes no file
   under `cmd/`, `internal/` or `web/`: the release sequence policy 88 after 87
   with `a8cddcb546302daa0f5eb65b18fe6b9566b69e66` as previous, the
   `download-portal/get.sh` pins (sequence 88 and version v0.1.0-alpha.88; the
   public-key pin unchanged), the README version lines and three contract
   tests. Still open: the repository variable
   `CELIKPANEL_RELEASE_SEQUENCE=88`, set before the tag. The real archive is
   built and signed by CI on the tag.
2. A pull request for the branch and its CI run.
3. Merge, the run on the main commit, tag, the check of the six signed assets,
   and the download portal's publication.
4. After the release is published, the owner of each installed server updates
   that server from the panel's own update screen. This is a separate step from
   every step above; no one else starts it.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
