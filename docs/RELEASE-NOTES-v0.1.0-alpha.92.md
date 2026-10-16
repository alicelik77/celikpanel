# v0.1.0-alpha.92 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.92.tr.md)

This is the release that follows v0.1.0-alpha.91. v0.1.0-alpha.91 was tagged
(`356109b532b9d0d0df5dd31fc19e9932918a9648`) but not put on the download
portal, so alpha.92 also delivers alpha.91's work: the second server as a full
peer, part 2 (the infrastructure zone in Domains with its record editor, the
peer's mail host name in the primary's plan, the host record requested from the
primary over the authorized DNS connection; see the
[alpha.91 notes](RELEASE-NOTES-v0.1.0-alpha.91.md)). Written on 2026-10-16
(UTC) on the branch `fix/alpha92-certbot-scheduler-restore`, from `main` at the
alpha.91 release commit.

## What happened

On 2026-10-16 the owner updated Frankfurt (Debian 13) from alpha.89 to alpha.90.
The update died at its last step: "Certbot renewal scheduler state could not be
restored". The page showed a red cross and the raw line; 16 seconds later the
server's own recovery finished the update and the page said "Update verified"
in the old notice form, with a close button. alpha.90 is installed and runs.
Boston (Ubuntu) passed the same update.

The cause: Debian's and Ubuntu's `certbot.timer` catches up a missed run as soon
as it is turned back on. When the update turned the timer back on, Certbot's
renewal started at once, and the updater required it to be idle. Roughly one
update in four or five meets this, depending on the hour.

## What changed

- **The updater never touches the owner's renewal run.** It stops only the
  timers while it works, waits (read-only, at most 180 s) for a renewal already
  running to finish, and never stops, starts or resets `certbot.service`. A
  renewal that starts when the timer is turned back on, or one that had failed
  before the update, is the owner's renewal, not an update failure. An update
  can now also start while `certbot.service` is in a failed state.
- **The update page stays calm while the server finishes by itself.** If the
  last step still has to be repeated, the page says **"Finishing the update"**:
  the new version is installed and running, one final step (turning automatic
  certificate renewal back on) is being repeated by the server, nothing needs to
  be done. The steps show the last one as "Repeating automatically (attempt 1 of
  3)". No red mark and no close button; the server's own line is under
  "Details". When the server confirms, the page and the update card say
  **"Updated to {version}"** with the build line, the five steps done and "One
  step was repeated automatically; nothing needs to be done.", plus a Reload
  button. Red stays for situations where you must act.
- **The build line copies as it reads**: "build 1743ac69edb0", not the short and
  full ids run together (owner's report of 2026-10-16).

## Evidence

Contract tests that model the native timer (a timer start that starts its
renewal, a renewal failed or running before the update), with negative controls
that reproduce Frankfurt's line on the previous code; Go tests for the new
failure code, the plan's `retrying` step and the recovery text; the update page
mounted in tests (repeating, then confirmed, then a paused control). One native
lab record, set19 (Debian 13, QEMU, test-licence builds, outbound 443 refused):
the update's own timer start began the renewal and the update ended `update_verified` with no failure; with the renewal failed before the update, the update ended the same way and the failure stayed the owner's; the page stayed calm (steps, then "Updated to") and the new build line copied as `build 16cbfc877bd4` ([set19](../deploy/e2e/release-recovery/evidence/set19-20261016/README.md)).

## What you will see

- **Updating to alpha.92 from alpha.90 or alpha.91:** the old page draws the
  update (the calm "Finishing the update" view is part of alpha.92's page). With
  the corrected updater this update should not stop at the renewal step.
- **On later updates from alpha.92:** if the renewal step has to be repeated,
  "Finishing the update" with the steps, then "Updated to".

## Limits

- The calm view is first seen when a server updates **from** alpha.92.
- A browser that already closed its record of the Frankfurt update keeps no
  "Updated to v0.1.0-alpha.90" card for it.
- set19 is a lab measurement on QEMU guests with test-licence builds, not
  acceptance evidence. P0.1-P0.5 stay **partial**; no P0 item is closed or
  advanced; publication is not acceptance.
- Schema 44, no migration. The update snapshot format is unchanged.

## Publication record

Not published.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
