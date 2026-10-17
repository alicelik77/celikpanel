# v0.1.0-alpha.94 (published 2026-10-17)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.94.tr.md)

This is the release that follows v0.1.0-alpha.93. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-17, on the branch `feat/alpha94-ready-bundles`, from `main` at the
alpha.93 release commit `0897ee9238bdfee295149adfe8b16968dd6fe0de`.

## What changed

**Ready bundles (owner decision of 2026-10-17, D-034).** The three "One-click
mail stacks" cards at the top of Components are gone. In their place, each
category has one line, "Ready bundles", between its header and its rows:

- **Mail:** Core Mail, Webmail, Spam-Protected Mail (the names the product
  already uses). They install through the existing mail install dialogue,
  exactly as before.
- **Databases:** a PostgreSQL bundle and a MariaDB bundle; once one is
  installed, its browser tool (phpPgAdmin, phpMyAdmin) is offered as "Add …".
- **Web:** Nginx + PHP-FPM.

A bundle is a shortcut over the components listed below it, not a separate
product: a database or web bundle installs its missing components one after
another through the ordinary component install, each with its own progress
("2/3 · installing PHP-FPM"), and only after the previous one, its own, has
finished. If one fails, its own guidance is shown and the rest wait; the strip
then offers Continue, which installs only what is missing. If another
operation runs instead, or changes are paused on the server, it stops and says
so. After a reload nothing continues by itself: the bundle is offered again and
installs only what is missing. An installed bundle collapses to one line:
"… installed and verified" (Repair as a small link for mail), or "installed,
not verified" with Repair for mail (it completes what is missing and verifies
the setup; nothing installed is removed) or Verify for a database or web
bundle (it only reads the components again). A mail bundle's last failed or
running attempt is said again under it. The Dashboard's Mail stacks card is
replaced by one calm "Ready bundles" card that lists what is installed and any
failed or running mail attempt with its reason, and opens the bundles on
Components.

**Calmer Components page** (the design review of 2026-10-16):

- Restart, Stop and Repair icons in a row are grey until you point at them;
  Stop's red stays in its confirmation.
- A stopped service has a red dot and "Stopped"; "Not installed" is a hollow
  ring.
- Row Install buttons are outlined; the page keeps one filled button.
- With the firewall on and healthy, it is one line: "Firewall on · Open ports:
  4 · Settings". The full box stays for off, unreadable or needing attention.
- Component icons are line icons from one map instead of emoji.
- Versions and states line up in columns; on a phone, row buttons are 44 px tall
  with their names written, and the version is shown under the description.

## What you will see after updating to alpha.94

- Boston (a PowerDNS secondary with Nginx installed): Web servers offers
  Nginx + PHP-FPM, which installs only PHP-FPM; Databases offers its bundles.
  If its DNS identity is not ready, Email shows one line saying so (on a
  secondary: where the mail record is created) instead of mail bundles; if it
  is ready, Email offers Core Mail, Webmail and Spam-Protected Mail.
- Frankfurt: "Webmail installed and verified · Repair" with "Add spam
  protection"; "MariaDB bundle installed and verified" with "Add phpMyAdmin"
  (it lists Nginx and PHP-FPM as already installed and installs phpMyAdmin);
  the firewall as one line; the Dashboard's Ready bundles card.

## Limits

- No native record, by the owner's instruction of 2026-10-16 (quick releases).
  Go, component, mounted and mock-browser checks only; nothing measured on an
  installed server; the bundle sequence has not run against a real Agent.
- Schema 44, no migration; no new Agent job kind; recovery behaviour unchanged.
  The managed-services answer gains an additive `bundles` field; the page
  shows no strips when an older Panel does not send it.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; publication is
  not acceptance.

## Evidence

Go `./cmd/panel/` and `./internal/...` (66 packages): vet and gofmt clean; 2844 top-level tests pass, 42 skip, 4278 subtests pass, none fail; in `./cmd/panel/` 1594 top-level tests pass and 6 skip (nine new ready-bundle tests; the mail profile view tests pass unchanged). Web: `npm test` 1582 of 1582; `tsc --noEmit` and the
production build with the bundle budget pass. Browser batch 19 (mock only):
Components on Boston-like mocks (with and without a ready DNS identity) and a
Frankfurt-like mock, a database bundle at step
2/3 and the Dashboard card, at 1440 and 390 px in both themes, Turkish; one fix
round (the hint line beside an installed bundle, PHP-FPM kept whole on a
phone). The release contracts pass: sequence policy (93 -> 94), download
portal, signed manifest.

## Publication record

Everything below is dated 2026-10-17 (UTC unless a time zone is named). The
merge, the tag, the checks, the portal publication and the read of the public
site were done by the release preparer under the owner's standing instruction,
as for the earlier releases; installed-panel updates are the owner's alone, from
the panel's own update screen. No native record for the publication, by the
owner's instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Pull request #226, "Release v0.1.0-alpha.94: ready bundles at the head of
   each Components category, one Dashboard card, and calmer Components colours",
   was merged into `main` as commit
   `d506a05e8c003dffee8de2e1268402865520bd6b`. The release sequence variable was
   moved from 93 to 94 before the tag was made. The main run and the tag run both
   succeeded, and the release chain ran cleanly (no incident, unlike alpha.93).
2. The annotated tag `v0.1.0-alpha.94` (object
   `d17aa39f13568a6254eeb5f1ab271eccff5eb5be`, tree
   `7c1fd69a4dd1c43953037a344f7850dc150922b6`) points at the release commit. The
   release is a pre-release, published at 2026-10-17T00:30:13Z with six assets.
3. Checked from a download in the WSL guest `CelikPanel-S2-Debian` on 2026-10-17
   (preparer; not recorded in a file here): the archives are byte-identical,
   66,259,323 bytes each, SHA-256
   `bec3c015317fecc226d0a282ecf7e30547e305a621e7fa70943947c6ef0a6788`; the
   manifest says sequence 94, `published_at` 2026-10-16T23:46:25Z (SHA-256
   `690d6aa03ee71010206b9f966ec7ac5098ba3b3e36ad539091797b02d9fecc83`), and its
   signature (SHA-256
   `724c9d440cd4960e6586e5b378fa3f9139ce17bdf0007916ee3fe334102aca1f`) verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the signing key (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   key as before); both content guards returned 0; the build used Go go1.26.5,
   CGO off, `-trimpath`. `libexec/get.sh` pins 94 (SHA-256
   `aeb5ac9f97e252c1a08f37a5dbc44cc9615e40839b55d8d16daf6d15109062d1`); the
   policy inside says current 94, previous 93, previous commit
   `0897ee9238bdfee295149adfe8b16968dd6fe0de`. The archive has 243 members. The
   interface bundle is `web/dist/assets/app-C9mMpV6A.js`, 249,028 bytes, SHA-256
   `7023aad0b5e28df141d7ffd9ef40311bdcbab594d5c3090adc514cdce2d7bf00`.
4. The portal package: `portal.tar.gz`, 132,891,376 bytes, SHA-256
   `b493b72b09e49301fdce059a52ad2d0b92f892dd0e82c1ce1f1550c4b6dcf215`, 54
   entries, built twice byte-identical with the tag's
   `deploy/build-download-portal.sh` (pre-signed mode). The contract test
   returned 0; the public verifier's 17 tests and the promotion's 14 tests
   passed.
5. At about 00:48Z the preparer published it with
   `deploy/publish-download-portal.ps1` from the preparer's machine, previous
   version v0.1.0-alpha.93, target v0.1.0-alpha.94: status `committed`, public
   verification 26 of 26 requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
6. The backup of the previous site is
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.94.20261017T004816Z.630222.f2befd2a7e699711`;
   the upload was retained in
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261017T004735Z-v0.1.0-alpha.94-71537ab34f991196`.
7. A read from outside after the publication:
   `https://celikpanel.net/releases/latest.txt` says v0.1.0-alpha.94;
   `latest.json` says sequence 94, with the commit and the SHA-256 above.

**What the owner reported.** Nothing yet. When this was written, Boston and
Frankfurt were still on alpha.92. The owner had not updated either of them to
alpha.94, and had not reported updating to alpha.93 either. No observation from
an installed server is recorded here, and none should be inferred. The ready
bundles, the Dashboard card and the calmer Components colours are first seen
after the owner updates from the panel's own update screen. Because the servers
are still on alpha.92, that update also brings alpha.93's six corrections.

**Not done, and what publication does not change.**

- The installed servers were not updated by the release preparer.
- Three small items found in review were deliberately left for a later release
  and are open:
  1. The page's check between the install steps of a bundle can miss a flag that
     says the server's state could not be verified, for one screen refresh. The
     server's readiness is still read fresh before each step, and the server
     refuses a change it must not accept.
  2. On the Components page, Verify (which only reads the components again) can
     be pressed while an install is running, and then gets a "busy" refusal
     instead of being unavailable.
  3. While the page is reading the change gate, the strip already says
     "installing" before anything has been sent (brief).
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.93
  or alpha.94, and everything the alpha.92 and alpha.93 notes list as not
  reported. Each is "not recorded", not "worked" and not "failed". The bundle
  sequence has not run against a real Agent.
- The limits of the earlier publication records (see the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it
does not update installed servers; only the owner of a server starts its
update, from the panel's own update screen.
