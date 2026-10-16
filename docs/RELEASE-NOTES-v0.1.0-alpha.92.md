# v0.1.0-alpha.92 (published 2026-10-16)

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

Everything below is dated 2026-10-16 (UTC unless a time zone is named). The
merge, the repository variable, the tag, the checks, the portal publication and
the read of the public site were done by the release preparer under the owner's
instruction of 2026-10-15; the installed-panel updates were started by the
owner from the panel's own update screen. No native record for the publication,
by the owner's instruction (alpha.92's own code was measured by set19, above).
This entry stores none of the results below as a file in the repository.

**What happened.**

1. Pull request #222 was squash-merged by the preparer into `main`: commit
   `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`, tree
   `091d5fb23cb16cbd217c8c1191033185a7d50b81`. The first CI run on the pull
   request failed in "Verify offline release-recovery fixture contracts (no
   VM)": the harness's mirrored code lists lacked the new
   `update_finishing_scheduler` code. It was corrected in `5f662635f` and CI then
   passed, 24 of 24. The run on `main` concluded success.
2. The preparer set `CELIKPANEL_RELEASE_SEQUENCE` from 91 to 92 and pushed the
   annotated tag `v0.1.0-alpha.92` (object
   `16a8a961aa9f06948902e4c972c4ce6603cc50fc`). The tag's run concluded
   success; the release was published at 2026-10-16T18:51:22Z with six assets.
3. Checked from a download (preparer; not recorded in a file here): the archives
   are byte-identical, 66,240,132 bytes, SHA-256
   `3e243da119a252988df80745d77f3e500b6856c60717fa4b2b8f94e1e54e4795`; the
   manifest says sequence 92, `published_at` 2026-10-16T18:09:41Z (SHA-256
   `d5824856…`), and its signature verifies (same key); 241 members;
   `libexec/get.sh` pins 92 (SHA-256
   `6c5a7b8e8145b14cd75506ab801775fad7c893c85210937243d32c9f2aeb8575`); the
   policy says 92 with previous 91 (`356109b53…`); `release.commit` and
   `release.tree` equal the tag's. The archive's `deploy/panel-tls-snapshot.sh`
   contains `_panel_tls_wait_service_idle` and
   `_panel_tls_note_owner_failed_renewal`, and `update.sh` contains
   `update_finishing_scheduler`. The interface bundle is `app-DomwK8kh.js`,
   248,892 bytes, SHA-256
   `92709cadaed9e3dfcf48b55956e0282c61eebf785a0b117c8f229a9fbacfd491`.
4. The portal package: 132,855,924 bytes, SHA-256
   `809e4f501a020ee54a847c580406f8698d1eb0809aa287be27316f00dd72ffcd`, 54
   members, built twice identical; the promoter accepted it; the tests passed.
   The preparer published it with `-PreviousVersion v0.1.0-alpha.90` (the live
   portal never served alpha.91): committed, backup
   `httpdocs.before-v0.1.0-alpha.92.20261016T185901Z…`, verification ok (26
   requests), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. A read from outside at 18:59:09Z: `releases/latest.txt` v0.1.0-alpha.92,
   `get.sh` pins 92, `releases/latest.json` sequence 92, commit `961f2cb2…`,
   SHA-256 `3e243da1…`.

**What the owner reported (2026-10-16, two screenshots, about 19:10Z; the
update from alpha.90).** The owner started both updates from the panel's own
update screen. On both servers, Settings, CelikPanel updates reads "Current
version v0.1.0-alpha.92" and "Current commit
961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a"; the sidebar reads "CelikPanel ·
v0.1.0-alpha.92 · build 961f2cb2fcae"; and a card "Updated to v0.1.0-alpha.92
— build 961f2cb2fcae" lists five completed steps. This holds for **Frankfurt
too**: no failure and no recovery this time. That is one update on each server
and it is not acceptance of the Certbot correction: whether a catch-up renewal
began during Frankfurt's update was not reported. What the page showed during
the update (the live step view, drawn by alpha.90's page) was not reported in a
screenshot.

**Not done, and what publication does not change.**

- The installed servers were not updated by the release preparer.
- Not reported and not recorded: Frankfurt's Domains "Infrastructure zone" row,
  Boston's mail extension with the peer host record, the licence page, the
  Agent's version, `postconf -n >/dev/null`. Each is "not recorded", not
  "worked" and not "failed".
- The calm "Finishing the update" view is first seen when a server updates
  **from** alpha.92.
- The limits of the earlier publication records (see the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
