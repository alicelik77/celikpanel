# v0.1.0-alpha.93 (published 2026-10-16)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.93.tr.md)

This is the release that follows v0.1.0-alpha.92. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-16 (UTC), on the branch `fix/alpha93-small-truths`, from `main` at
the alpha.92 release commit `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`.

## What changed

Six small corrections from the owner's screenshots of both installed servers on
alpha.92 (2026-10-16), each decided by the owner the same day:

1. **Core Mail no longer asks for Repair when mail is verified.** A profile is
   verified when a larger verified profile that contains all of its components
   (Webmail or Spam-Protected Mail) proves the same reconciliation on the same
   scan. Component state alone still proves nothing; a running or failed Core
   Mail attempt keeps its own state.
2. **"Repair webmail" is an ordinary button**, drawn like Manage. It is amber
   only when the server reports a webmail problem.
3. **The Dashboard says verified mail plainly:** "Mail is installed and verified
   on this server."
4. **No mail component, no "Partially installed".** With only Nginx (or PHP-FPM)
   present, the Mail stacks card says "Not installed" in the neutral tone; the
   Webmail card on Components keeps its "Some components are already present…"
   note, muted. One mail component (Postfix, Dovecot, Rspamd, Roundcube) still
   makes a profile partial.
5. **Recent activity names mail profiles:** "prepared Spam-Protected Mail during
   setup" instead of "prepared protected-mail during setup".
6. **Counts are pluralised:** "1 item", "3 items" (Turkish: "1 öğe", "3 öğe").

## What you will see after updating to alpha.93

- Frankfurt: the Core Mail card is calm, with no amber Repair sentence; the
  Dashboard's Mail stacks card reads "Mail is installed and verified on this
  server."
- Boston: the Mail stacks card reads "Not installed", neutral.
- Both: "Repair webmail" beside Roundcube is a plain button; Recent activity
  shows profile names; Domains says "1 item".

## Limits

- No native record, by the owner's instruction of 2026-10-16 (quick releases).
  Go, component and mounted tests only; nothing measured on an installed server.
- Schema 44, no migration; recovery behaviour unchanged. The API change is a
  rule change only: a mail profile may now be `verified` with its own latest
  attempt `none`.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.

## Publication record

Everything below is dated 2026-10-16 (UTC unless a time zone is named). The
merge, the tag, the checks, the portal publication and the read of the public
site were done by the release preparer under the owner's standing instruction,
as for the earlier releases; installed-panel updates are the owner's alone, from
the panel's own update screen. No native record for the publication, by the
owner's instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Pull request #224, "Release v0.1.0-alpha.93: six small corrections from the
   installed servers", was merged into `main` as commit
   `0897ee9238bdfee295149adfe8b16968dd6fe0de`.
2. **The first attempt tagged the wrong commit; nothing was released.** The
   release chain's first run tried to merge the pull request, the merge was
   refused because the pull request had conflicts, and the script carried on
   and tagged a commit that was not the release commit. The tag was pushed, and
   CI's policy guard rejected it ("tag does not match policy version"). No
   release was created and no asset was published, so nothing public changed.
   The tag was deleted, and the chain script was changed to stop unless the
   pull request is merged, `main` equals its merge commit and `main`'s policy
   version equals the tag. The second run, with that guard in place, produced
   this release.
3. The annotated tag `v0.1.0-alpha.93` (object
   `c64b60021c3fc6b6bb506e99bdaf6b1354f531cf`, tree
   `7b18e5252045eef74264a946d41efcbbc927b545`) points at the release commit. The
   release is a pre-release, published at 2026-10-16T22:04:09Z with six assets.
4. Checked from a download in the WSL guest `CelikPanel-S2-Debian` on the same
   evening as the tag (preparer; not recorded in a file here): the archives are
   byte-identical, 66,240,206 bytes, SHA-256
   `887fbe30a29c6fa364bfafc44ec676414c32e41758417738ff4cecc287f9fc39`; the
   manifest says sequence 93, `published_at` 2026-10-16T21:21:23Z (SHA-256
   `e28f451221825d6244d1a34c50bb5ecba366fafc74e9c0342563f7068df7f625`), and its
   signature (SHA-256
   `f0991824d4e746a5726e3a46255a824ff332f9799797ee67772c72fb993bde8a`) verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the signing key (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   key as before); both content guards returned 0; the build used Go go1.26.5,
   CGO off, `-trimpath`. `libexec/get.sh` pins 93 (SHA-256
   `b3f1ce91e127203350b5c73e509f49b62433ee0190b9e9de6c8bcd72bb20cf11`); the
   policy inside says current 93, previous 92, previous commit
   `961f2cb2fcaeb48c08fc58828d15fe0b231e1e1a`. The interface bundle is
   `web/dist/assets/app-BskYbaLr.js`, 248,904 bytes, SHA-256
   `5f0db2b45ac3aa0165b367e3a6fac224a6670ac0b86a3d6f4df4ac4c111af46b`.
5. The portal package: `portal.tar.gz`, 132,853,434 bytes, SHA-256
   `b27504acc26d7c291d7e0ec3ebfb051ac04996b27af65e78e99c3a0ea934f83c`, 54
   entries, built twice byte-identical with the tag's
   `deploy/build-download-portal.sh` (pre-signed mode). The contract test
   returned 0; the public verifier's 17 tests and the promotion's 14 tests
   passed.
6. The preparer published it with `deploy/publish-download-portal.ps1` from the
   preparer's machine, previous version v0.1.0-alpha.92, target v0.1.0-alpha.93:
   status `committed`, public verification 26 of 26 requests, phase full,
   `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`. The backup of the previous site is
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.93.20261016T221039Z.596810.a34abffc5937f8c4`;
   the upload was retained in
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261016T220956Z-v0.1.0-alpha.93-9fc5ae34dd76f014`.
7. A read from outside after the publication:
   `https://celikpanel.net/releases/latest.txt` says v0.1.0-alpha.93;
   `latest.json` says sequence 93, with the commit and the SHA-256 above.

**What the owner reported.** Nothing yet. When this was written, Boston and
Frankfurt were still on alpha.92 and the owner had not updated either of them
to alpha.93. No observation from an installed server is recorded here, and none
should be inferred. The six corrections (Frankfurt's Core Mail card, Boston's
"Not installed", the plain Repair webmail button, the Dashboard sentence, the
profile names in Recent activity, the plural counts) are first seen after the
owner updates from the panel's own update screen. That update is also the first
one that starts **from** alpha.92, so it is where the calm "Finishing the
update" view could first be seen.

**Not done, and what publication does not change.**

- The installed servers were not updated by the release preparer.
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.93,
  and everything the alpha.92 notes list as not reported. Each is "not
  recorded", not "worked" and not "failed".
- The limits of the earlier publication records (see the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
