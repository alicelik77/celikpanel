# v0.1.0-alpha.90 (published 2026-10-16)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.90.tr.md)

This is the release that follows v0.1.0-alpha.89. The number v0.1.0-alpha.90
was chosen by the release preparer under the owner's standing instruction of
2026-10-13. Written on 2026-10-16 (UTC), on the branch `release/alpha90`, from
`main` at `d935170818f315d7c6c94e1e7324417bb5a38f8e` (after the alpha.89
release commit `ac36fccf4e4238a8bc4624356a478ba8dab20b34`).

## What changed

One correction (D-033, the owner's observation of 2026-10-16 on Frankfurt):
after "Check for updates" finds nothing, Settings, CelikPanel updates said
"There is no new update on this channel." with a warning triangle. A good state
was drawn as a warning. It now says **"You are on the latest version."**
("En son sürümü kullanıyorsunuz.") with a check mark on the normal surface. The
card does not tell channels apart, so the sentence names none.

On the same card: a found update ("A signed update is available.") is neutral
information with a download mark, and the server-readiness box is neutral while
it is being read. The warning mark stays only where something is wrong or not
known: a check that could not be read, a server that is not ready, an update
that could not be started, an earlier failed attempt, and the alpha backup
notice.

## Why it exists

The owner asked for it on 2026-10-16 ("90'ı çıkar"). An update runs on the page
of the version installed before it, so the step-by-step update view is first
seen live when a server updates **from alpha.89 to alpha.90**. No native record
and no long lab tests, by the owner's instruction.

## What you will see when updating alpha.89 to alpha.90

- Five step bars that fill one after another: download the update package;
  check the signature and checksums; check the server before installing; install
  the new version and restart the panel; verify the new version.
- The planned restart with one calm sentence: "The panel is restarting; this
  step can take a few minutes. This page reconnects by itself."
- At the end: "Updated to v0.1.0-alpha.90".

## Limits

- No native record, by the owner's instruction.
- The live step view was meant to be observed during this update; it was not
  reported as seen (see "Publication record").
- The restart length is measured on a server for the first time on this update
  (the measurement itself was not reported);
  without an earlier measurement the restart bar expects 90 seconds (read from
  the code, not observed).
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.
- Schema 44, no migration; recovery behaviour unchanged.

## Publication record

Everything below is dated 2026-10-16 (UTC unless a time zone is named). The
merge, the repository variable, the tag, the checks, the portal publication and
the read of the public site were done by the release preparer under the owner's
instruction of 2026-10-15; the installed-panel updates were started by the
owner from the panel's own update screen. No native record, by the owner's
instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Pull request #220 was squash-merged by the preparer into `main`: commit
   `1743ac69edb0e5723dc60d992b53094dcc4b1431`, tree
   `bbfa5305cc8c5b6bb4a34a2e640633582dc7534b`. The run on it concluded success.
2. The preparer set `CELIKPANEL_RELEASE_SEQUENCE` from 89 to 90 and pushed the
   annotated tag `v0.1.0-alpha.90` (object
   `9c98483d71b4a747f522f87c476f22125868bca7`). The tag's run concluded
   success; the release was published at 2026-10-16T14:42:18Z with six assets.
3. Checked from a download (preparer; not recorded in a file here): the archives
   are byte-identical, 66,194,918 bytes, SHA-256
   `3719aba4f60b3eb3da0762b2d1c57302b628cdb6c958d72c2a5ac0430e2ff3e1`; the
   manifest says sequence 90, `published_at` 2026-10-16T13:58:20Z (SHA-256
   `0f27855c…`), and its signature verifies (key `7eadeb0b…`); 237 members;
   `libexec/get.sh` pins 90 (SHA-256
   `eb55a90102fe21be27483ceaaf2fcb1bcf7846a1285f0253ad7a06d333c41bd3`); the
   policy says 90 with previous 89 (`ac36fccf4…`); `release.commit` and
   `release.tree` equal the tag's. The interface bundle is `app-HNXfBLHb.js`,
   248,218 bytes, SHA-256
   `b743c25f66e712623593ae333e28a8b82bc9d764eba66830976e3e5c4b21609c`.
4. The portal package: 132,762,586 bytes, SHA-256
   `b2a31a43d2b1e497b7dc9da53f405d79fa70d7193cb86f518f5d367e74b3acef`, 54
   members, built twice identical; the promoter accepted it; the tests passed.
   The preparer published it (previous v0.1.0-alpha.89): committed, backup
   `httpdocs.before-v0.1.0-alpha.90.20261016T145144Z…`, verification ok (26
   requests), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. A read from outside at 14:51:50Z: `releases/latest.txt` v0.1.0-alpha.90,
   `get.sh` pins 90, `releases/latest.json` sequence 90, commit `1743ac69…`,
   SHA-256 `3719aba4…`.

**What the owner reported (2026-10-16, screenshots; the update from
alpha.89).** The owner started both updates from the panel's own update screen.

- **Boston:** a clean update. The card reads "Updated to v0.1.0-alpha.90",
  build `1743ac69edb0`, five steps done; after "Check for updates" a green "You
  are on the latest version." (this release's D-033 correction, seen).
- **Frankfurt:** at T+01:23 a red cross, "Last recorded step: recovery in
  progress" and the server's line "reviewed updater failed: exit status 1:
  Certbot renewal scheduler state could not be restored"; at T+01:39 "Update
  verified" for the same request (`13193201ba7a3d832bb64cc6d972f8a5`). The
  browser kept alpha.89's interface until the owner pressed Ctrl+Shift+R: the
  sidebar showed alpha.89 while "Current version" showed alpha.90, with the old
  warning-triangle "no new update" line. After the reload the page reads "You
  are on the latest version." but has no "Updated to" card, because the update
  finished through the recovery record. The cause of the failure and its
  correction are in the [alpha.92 notes](RELEASE-NOTES-v0.1.0-alpha.92.md). Why
  the browser kept the old interface was not investigated here.
- **Copying Boston's card** pasted the build line as "build
  1743ac69edb01743ac69edb0e5723…" (the short and the full id run together);
  corrected in alpha.92.
- The live step view during this update was **not** reported as seen: the
  update ran on alpha.89's page, and on Frankfurt it failed into the old
  dialog.

**Not done, and what publication does not change.**

- The installed servers were not updated by the release preparer.
- Not reported and not recorded: Frankfurt's Domains "Infrastructure zone" row,
  Boston's mail extension with the peer host record (both arrive with alpha.91's
  code, in alpha.92), the licence page, the Agent's version,
  `postconf -n >/dev/null`.
- The limits of the earlier publication records (see the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
