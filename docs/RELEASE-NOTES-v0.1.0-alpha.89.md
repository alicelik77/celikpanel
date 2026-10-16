# v0.1.0-alpha.89 (published 2026-10-16)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.89.tr.md)

This is the release that follows v0.1.0-alpha.88. It was published on
2026-10-16 (see "Publication record"). The number v0.1.0-alpha.89 was chosen by the release
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
- The step view itself was meant to be first observed on the update from
  alpha.88; no server ran alpha.88, so that did not happen (see "Publication
  record").
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.
- Everything else (design, the five steps, tests, other limits) is in the
  [v0.1.0-alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md).

Publishing this release updates no installed server; the owner starts each
update from the panel's own update screen.

## Publication record

Everything below is dated 2026-10-16 (UTC unless a time zone is named). The
merge, the repository variable, the tag, the checks, the portal publication and
the read of the public site were done by the release preparer under the owner's
instruction of 2026-10-15; installed-panel updates remain the owner's only. No
native record, by the owner's instruction of 2026-10-16. This entry stores none
of the results below as a file in the repository.

**What happened.**

1. Pull request #218 was squash-merged by the preparer into `main`: commit
   `ac36fccf4e4238a8bc4624356a478ba8dab20b34`, tree
   `29f08b6bef1ca6e19c4f02890f83cfeb3f678433`. The run on it concluded success.
2. The preparer set `CELIKPANEL_RELEASE_SEQUENCE` from 88 to 89 and pushed the
   annotated tag `v0.1.0-alpha.89`. The tag's run (38874797963) concluded
   success; the release was published at 2026-10-16T08:57:23Z with six assets.
3. Checked from a download (preparer; not recorded in a file here): the archives
   are byte-identical, 66,195,105 bytes, SHA-256
   `58c6911be7a1b640b0bafb595a8c2b54451d70d8238d51b1c726a76bb7f59526`; the
   manifest says sequence 89, `published_at` 2026-10-16T08:14:24Z, and its
   signature verifies with the same key; 237 members; `libexec/get.sh` pins 89
   (SHA-256
   `baaae2dce46465ba3198b8f3a6662e0590117e85586b81450149305b52aeaf08`); the
   policy says 89 with previous 88 (`1cb3628ff…`); `release.commit` and
   `release.tree` equal the tag's. The interface bundle is byte-identical to
   alpha.88's (`app-DAi4coqL.js`, same hash; compared directly), as the
   unchanged product code implies.
4. The portal package: 132,763,175 bytes, SHA-256
   `bb36ebce7b52261988651d9df234ebe9c26b45ec605feacb5bda9f2da1649f1e`, 54
   members, built twice identical; the promoter accepted it; the tests passed.
   The preparer published it (previous v0.1.0-alpha.88): committed, backup
   `httpdocs.before-v0.1.0-alpha.89.20261016T090510Z…`, verification ok (26
   requests), `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
5. A read from outside at 09:05:19Z: `releases/latest.txt` v0.1.0-alpha.89,
   HSTS present, `get.sh` pins 89, `releases/latest.json` sequence 89, commit
   `ac36fccf…`, SHA-256 `58c6911b…`.

**What the owner reported (2026-10-16, two screenshots, about 09:30Z).** The
owner started both updates from the panel's own update screen. Both servers
went **directly from their previous version to v0.1.0-alpha.89** ("doğrudan
89'a güncelledi"; the sequence floor admits it): Frankfurt from alpha.87,
Boston from the version it ran (not reported; alpha.85 or later). On both,
Settings, CelikPanel updates shows "Current version v0.1.0-alpha.89" and
"Current commit ac36fccf4e4238a8bc4624356a478ba8dab20b34"; the sidebar reads
"CelikPanel · v0.1.0-alpha.89 · build ac36fccf4e42"; and a card "Updated to
v0.1.0-alpha.89 — build ac36fccf4e42" lists five completed steps (download the
update package; check the signature and checksums; check the server before
installing; install the new version and restart the panel; verify the new
version). So after the reload the alpha.89 interface renders the server's step
plan of the completed update. Because each update ran on the previous version's
page, the live step view during the restart was **not** seen; it is first seen
on the next update, from alpha.89. Frankfurt's page also shows "There is no new
update on this channel." with a warning-triangle icon: a good state drawn with a
warning icon, recorded as a D-033 item for the next release (it should read as
confirmation, for example "You are on the latest version", with a neutral or
success icon); nothing was changed for it here.

**Not done, and what publication does not change.**

- The installed servers were not updated by the release preparer.
- Not reported and not recorded: the licence page, the Agent's version,
  Frankfurt's setup final step, Boston's "Extend this server's setup",
  `postconf -n >/dev/null`.
- Limits as in the publication record of the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md): tag files via `git archive`
  on Windows, no promoter rehearsal, no file-by-file comparison with the
  previous package, the publisher's own test not run.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
