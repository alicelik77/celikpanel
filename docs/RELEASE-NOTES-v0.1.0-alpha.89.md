# v0.1.0-alpha.89 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.89.tr.md)

This is the candidate that follows v0.1.0-alpha.88. It is not published (see
"Publication record"). The number v0.1.0-alpha.89 was chosen by the release
preparer under the owner's standing instruction of 2026-10-13; it is not a
number the owner named. Written on 2026-10-16 (UTC), on the branch
`release/alpha89`, from `main` at `1cb3628ff1969f3fc2e9f0d848b7adf629a815b0`
(the alpha.88 release commit).

## Same code as v0.1.0-alpha.88

This release has **the same product code as v0.1.0-alpha.88**. The check
`git diff --stat 1cb3628ff HEAD -- cmd internal web` is empty. Only the release
sequence policy (89 after 88), the installer's bootstrap pins, the README
version lines, the three pinned contract tests and these documents differ.

## Why it exists

The owner asked on 2026-10-16 for two releases in a row ("iki sürüm birden
çıkart"). An update always runs on the page of the version installed before
it, so updating *to* v0.1.0-alpha.88 still shows alpha.87's page. The new
step-by-step update view is first seen when updating *from* alpha.88 *to* this
release. No native record and no long lab tests, by the owner's instruction.

## What you will see when updating alpha.88 to alpha.89

- Five step bars: download, check the signatures and checksums, check the
  server, install and restart the panel, verify the new version. Finished steps
  have a check mark; the next ones wait below.
- The planned restart with one calm sentence: "The panel is restarting; this
  step can take a few minutes. This page reconnects by itself."
- At the end: "Updated to v0.1.0-alpha.89", then the page reloads.

The restart is measured once on the update *to* alpha.88, so the restart bar of
this update uses that measurement (expected length `max(last x 1.5, 90 s)`,
capped at 360 s). That is read from the code; it has not been observed.

## Limits

- No native record, by the owner's instruction.
- The step view itself is observed for the first time on an installed server
  during this update.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.
- Everything else (design, the five steps, tests, other limits) is in the
  [v0.1.0-alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md).

Publishing this release updates no installed server; the owner starts each
update from the panel's own update screen.

## Publication record

Not published.
