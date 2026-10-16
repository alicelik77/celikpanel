# v0.1.0-alpha.90 (candidate, not published)

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
- The live step view is observed on an installed server for the first time
  during this update.
- The restart length is measured on a server for the first time on this update;
  without an earlier measurement the restart bar expects 90 seconds (read from
  the code, not observed).
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.
- Schema 44, no migration; recovery behaviour unchanged.

## Publication record

Not published.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
