# v0.1.0-alpha.95 (published 2026-10-17)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.95.tr.md)

This is the release that follows v0.1.0-alpha.94. The number was chosen by the
release preparer under the owner's standing instruction of 2026-10-13. Written
on 2026-10-17, on the branch `fix/alpha95-bundle-consistency`, from `main` at the
alpha.94 release commit `d506a05e8c003dffee8de2e1268402865520bd6b`.

## Who should update, and why

Everyone who runs alpha.94. After updating alpha.94 the owner looked at
Components and the Dashboard on Boston and Frankfurt and asked for the problems to
be evaluated again and fixed (owner decision of 2026-10-17, D-034 amendment). This
release is that correction. Only the owner of a server updates it, from the
panel's own update screen.

## What changes on screen

**The same bundle list on every server.** Every Components category (Email,
Databases, Web servers) shows the same ready bundles in the same order on every
server. Only the state on each chip changes. This withdraws alpha.94's rule
"the installed bundle plus at most one more offer", which was the release
preparer's mistake: on Boston it hid the MariaDB bundle although it could be
installed (and the Dashboard counted it), and Core Mail was a chip on Boston and
missing on Frankfurt.

A chip says its state in words and in form, never by colour alone:

- a check mark and the name: installed and verified;
- "Core Mail · included": a verified mail bundle contains it, and the contained
  bundle is itself installed and verified with no failed or running attempt of its
  own (otherwise it reads "not verified" with its reason);
- a grey dot and "not verified": installed, not proven yet;
- outlined "Name · Install" (add-ons: "· Add"): can be installed;
- "n of m installed · Continue": a mail install was started and is not complete.
  A mail chip that says "Install" (Spam-Protected Mail beside an installed Webmail,
  which the Panel calls partial only because the parts are shared) opens the mail
  dialogue in its install form, not "continue";
- muted text with a short reason ("phpMyAdmin · needs MariaDB") on a quiet button:
  pressing it, by keyboard or touch, shows the full reason on a line under the
  chips (the tooltip stays but is no longer the only way). It cannot be installed
  yet; add-ons always appear;
- "2/3 · installing PHP-FPM" with a moving indicator while a bundle installs; the
  other chips are disabled meanwhile.

Pressing an installed chip opens a dialogue: **Repair** for a mail bundle (it
completes what is missing and verifies the setup; nothing installed is removed),
**Verify** for a component bundle (it reads the components again and changes
nothing). The dialogue describes the bundle ("PostgreSQL for your sites'
databases."), not an imperative. The strip has no Repair links and no hint
sentence. If the components were not checked recently, the strip is one muted line
and the page checks by itself. Under the chips the strip says, with the reason,
only attempts that failed or are running and typed warnings on chips that are not
verified or only partly installed; the "some parts are already present" note is
never shown there.

**The Dashboard card** is one summary line ("7 installed and verified · 1 can be
installed"), then only the problem lines (a failed or running mail attempt with
its reason, a bundle installed but not verified), then "Open in Components". It
counts the way the chips are drawn: every bundle including add-ons; "can be
installed" is available or partial, blocked is not counted; "included" counts as
installed and verified, but a bundle whose own last attempt failed or runs is never
counted as installed and verified. A bundle that is not verified and also has a
failed or running attempt is one line, with the reason. The sentences for never
checked, checking, held, could not check and not recent are kept.

**Three review items of alpha.94 are closed.**

1. The check between a bundle's install steps now reads the "state could not be
   verified" flag the moment it changes, so one screen refresh can no longer miss
   it.
2. Verify of a component bundle is held, with the reason shown, while a component
   operation is running on the server; you press it again when the operation ends.
   It stays usable while changes are held for other reasons (a busy host lock or
   a running package manager among them), because it only reads: the rescan
   endpoint refuses only for a running server setup, an active service operation
   or an active Agent mutation.
3. While the page checks the server before an install and the request has not been
   accepted yet, the chip says "Checking the server before installing X…"; it says
   "installing X" only after the install is accepted.

**Rows on Components.**

- The Stop icon is a stop sign in a circle (the empty square read as a checkbox).
  Restart, stop, start, repair and delete share one outlined icon button, grey at
  rest and coloured only under the pointer or keyboard focus. The same Stop icon
  is used on the component page and the hosting type panel.
- The desktop version column of dashes is gone. A version appears as small
  monospaced text under the description where the scan knows it (PHP-FPM and
  Node.js today).
- "PHP-FPM" no longer breaks at its hyphen in requirement badges and chips.
- "Repair webmail" stands on the Roundcube row only when the server reports a
  webmail problem; otherwise repair is in the Webmail chip's dialogue.
- **Roundcube leftovers.** The Roundcube row carries a muted note ("An earlier
  Roundcube install did not finish; its files may still be on this server." or "…
  removal did not finish; …") and a neutral "Clean up leftovers…" button only when
  the Panel's own record shows an unfinished Roundcube install or removal: the
  newest Roundcube install or removal entry in the Panel's audit log is a failed
  install or a partial or failed removal, or the webmail profile's last attempt
  failed. A later clean install or removal ends the offer; without a record the row
  says nothing (Boston, which has no such record, shows no leftovers offer on the
  row). Roundcube's own component page always has a "Clean up leftovers…" link that
  opens the same confirmation on the Components page. The
  confirmation lists what the Agent removes: `/var/lib/celikpanel-webmail`
  (Roundcube's files, its own settings and address-book database),
  `/var/lib/.celikpanel-webmail.stage`, `/var/lib/.celikpanel-webmail.retired*`,
  `/var/lib/..celikpanel-webmail.stage.retired*` (copies from an interrupted install
  or removal), `/etc/nginx/conf.d/celikpanel-webmail.conf` (then Nginx is reloaded)
  and `/run/celikpanel-webmail.sock`; and it says that mailboxes and Maildir data,
  other sites, Nginx, PHP-FPM and packages are not touched. The red danger style is
  only on the dialogue's confirm button.

**Counts.** The sidebar counts for Components and Domains are plain muted numbers
instead of filled pills (a count that is not known is still a dash, never 0). The
Dashboard Domains tile and the sidebar Domains badge now use one count, domains plus
the DNS-only infrastructure zones the Domains page lists (a zone that became a
website counts once, as that website), so tile, badge and page agree (on Frankfurt
the Dashboard said 0 while the page listed the zone). It is a number only when both
reads answered; while either is read the tile shows the loading mark; when one could
not be read it is unknown, never "domains + 0" (an earlier number is kept only when
both reads had one); non-administrators, who have no zone read, count domains alone. The
Dashboard sentence when nothing needs attention now reads "Nothing needs your
attention. CelikPanel checked the certificates, the firewall and the installed
components." and, as before, appears only when every read was answered and the
components scan is fresh.

## What does not change

- Installing a database or web bundle: still one ordinary component install at a
  time, each only after the previous one has ended with a fresh scan; the first
  failure, another operation or a closed gate stops it with the reason; Continue
  installs only what is missing; nothing continues by itself after a reload.
- A mail bundle still opens the existing mail install dialogue and runs the same
  route.
- Schema 44, no migration; no Agent change; no new Agent job kind; the updater,
  rollback and recovery behave as in alpha.94. No native record, because the
  updater is unchanged (and by the owner's instruction of 2026-10-16).
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## Known gaps

- **Versions are not read for every component.** The Panel gets a version only
  for PHP-FPM and Node.js. The Agent also probes a binary per service unit, but that
  is not a package version, is unreliable for several units (Apache, `named`,
  PostgreSQL) and does not cover tools without a unit (Certbot). Real package
  versions need a new Agent read; this release adds none. Open roadmap item: read
  installed package versions for every component.
- **Roundcube leftovers on disk are not seen.** The Panel still cannot see whether
  files are actually left on the server (there is no Agent read for it); it relies
  on its own record of an unfinished install or removal. A leftover that no
  Panel record points to is reached only through the link on Roundcube's own page.
- Nothing was measured on an installed server; the bundle sequence has not run
  against a real Agent. P0.1-P0.5 stay **partial**; no P0 item is closed or
  advanced; a candidate is not acceptance.

## Independent review of the candidate (2026-10-17)

An independent review of the candidate found ten problems. They are fixed, and the text above describes the corrected behaviour:

1. Roundcube leftovers are offered on the Components row only when the Panel's own
   record shows an unfinished install or removal (additive payload field
   `webmail_cleanup`, read from the existing audit log) or a failed webmail
   attempt; Roundcube's own page always has the link (`/services?cleanup=roundcube`).
2. The installed bundle dialogue describes the bundle instead of "Installs …".
3. Lines under the chips: only failed or running attempts and typed warnings on
   chips that are not verified or only partly installed; never "parts already
   present".
4. "included" only for a contained bundle that is itself installed and verified with
   no failed or running attempt; otherwise "not verified" with its reason, and the
   Dashboard does not count it.
5. One Domains count for the Dashboard tile and the sidebar badge (`domainsCount`).
6. One Dashboard line, with the reason, for a bundle that is not verified and has a
   failed or running attempt.
7. A blocked chip is a quiet button; pressing it shows the full reason under the
   chips.
8. Verify stays usable on a busy host lock or a running package manager.
9. Wording: "2/3 · installing PHP-FPM"; "n of m installed" (Turkish "2/3 bileşen
   kurulu").
10. A mail chip that says "Install" opens the mail dialogue in its install form.

## Evidence

Web: the changed and new contract and mounted tests (ready-bundles-ui-contract and
ready-bundles-mounted on Boston- and Frankfurt-shaped payloads, including record,
no record and deep link for the leftovers, the blocked reason and the installed
description; installed-alpha94-corrections, including the Domains count states and
the cleanup rule; the Dashboard card, row layout, remote-state, mail profile and
service availability contracts); Go: the new webmail_cleanup_record_test, the
bundles test (an add-on before its parent needs MariaDB or PostgreSQL) and the mail
profile frontend contract. The release contracts (sequence policy 94 -> 95,
download portal, signed manifest) passed on the earlier tree and their inputs are
unchanged. Final counts on the final candidate tree (2026-10-17): web
`npm test` 1599 passed, 0 failed, and `npm run build` succeeded; Go `./cmd/panel/`
and `./internal/...` 7124 passed, 0 failed, 44 skipped; `go vet ./cmd/panel/` and
gofmt clean. Browser batch 20 (Boston- and Frankfurt-like mocks, Turkish and
English, light and dark, 1440 and 390 px) was looked at twice before the review
fixes; the first look found PHP-FPM broken at its hyphen in two dialogues,
Components controls under 44 px on a phone and a stray separator dot, all fixed.
After the review its Boston scenario opens the cleanup through
`/services?cleanup=roundcube`; **batch 20 was not re-run after the review fixes.**
Browser mock only; nothing on an installed server. No schema change (44; the new
payload field is additive JSON), no Agent change, no native record.

## Publication record

Everything below is dated 2026-10-17 (UTC unless a time zone is named). The
merge, the tag, the checks, the portal publication and the read of the public
site were done by the release preparer under the owner's standing instruction,
as for the earlier releases; installed-panel updates are the owner's alone, from
the panel's own update screen. No native record for the publication, by the
owner's instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Two independent reviews came before the release. The first found ten problems
   (all fixed, listed above); the second found the release ready. The release
   preparer also bounded the new Roundcube-record read to the newest 20,000
   audit entries.
2. Pull request #228, "Release v0.1.0-alpha.95: the same ready bundle list on
   every server, and the owner's alpha.94 screen findings", was merged into
   `main` as commit `5f3cb1a2fa336c529740a20e9a543b8ac086dc8d`. The release
   sequence variable was moved from 94 to 95 before the tag was made. The main
   run and the tag run both succeeded, and the release chain ran cleanly (no
   incident).
3. The annotated tag `v0.1.0-alpha.95` (object
   `44764f09aa74546bb5b5704fabde8bf1930fc803`, tree
   `0e9d3c486cae7250c4d4c4d525d50fd37c56920a`) points at the release commit. The
   release is a pre-release, published at 2026-10-17T11:01:22Z with six assets.
4. Checked from a download in the WSL guest `CelikPanel-S2-Debian` on 2026-10-17
   (preparer; not recorded in a file here): the archives are byte-identical,
   66,259,462 bytes each, SHA-256
   `9d9116f3199f453c508a605403cf4e6c63c099145dbef0b98cc402abc0852ef5`; the
   manifest says sequence 95, `published_at` 2026-10-17T10:22:04Z (SHA-256
   `a3720afcf1230eb4f7ed76c514fd1a81fa65bd0e3033951bd24835bb45ec5b10`), and its
   signature (SHA-256
   `026029ba235720f708999d79b2700f20873c6cc3b9de20d008b78fc30872e69b`) verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the signing key (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   key as before); both content guards returned 0; the build used Go go1.26.5,
   CGO off, `-trimpath`. `libexec/get.sh` pins 95 (SHA-256
   `f1846dacedbee5ab860d7765242dc6a979481be95c25ba69f4f48a1d78ce1a02`); the
   policy inside says current 95, previous 94, previous commit
   `d506a05e8c003dffee8de2e1268402865520bd6b`. The archive has 243 members. The
   interface bundle is `web/dist/assets/app-DU840jYm.js`, 250,651 bytes, SHA-256
   `f48f3c54869b6c45e7160c1fe49fd6a92c2f90799385f3a76024bbe86a61acf0`.
5. The portal package: `portal.tar.gz`, 132,890,919 bytes, SHA-256
   `4f11dc01376626e6db9160f4716b65fa044b4084bd9b24991bced6b5a9985ca1`, 54
   entries, built twice byte-identical with the tag's
   `deploy/build-download-portal.sh` (pre-signed mode). The contract test
   returned 0; the public verifier's 17 tests and the promotion's 14 tests
   passed.
6. At about 11:09Z the preparer published it with
   `deploy/publish-download-portal.ps1` from the preparer's machine, previous
   version v0.1.0-alpha.94, target v0.1.0-alpha.95: status `committed`, public
   verification 26 of 26 requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. The backup of the previous site is
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.95.20261017T110941Z.763371.3e1141ace6970b30`;
   the upload was retained in
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261017T110857Z-v0.1.0-alpha.95-c4ef6772d072445a`.
8. A read from outside after the publication:
   `https://celikpanel.net/releases/latest.txt` says v0.1.0-alpha.95;
   `latest.json` says sequence 95, with the commit and the SHA-256 above.

**What the owner reported.** On 2026-10-17 the owner updated Boston and
Frankfurt to alpha.94 from the panel's own update screen and sent screenshots of
the Dashboard and of Components; both showed build `d506a05e8c00`. Those
screenshots are what this release corrects (see "Who should update, and why" and
"What changes on screen" above; not repeated here). One screenshot of Boston's
Components page was a stitched full-page capture with repeated headings; normal
screenshots showed the page intact, so it is not recorded as a page defect.
Nothing from alpha.95 on an installed server has been reported yet, and none
should be inferred.

**Not done, and what publication does not change.**

- The installed servers were not updated to alpha.95 by the release preparer.
  Boston and Frankfurt are on alpha.94 until the owner updates them.
- Small items found in review were deliberately left for a later release and are
  open:
  1. Verify (which only reads the components again) can still get a "busy"
     answer while the host's package manager or host lock is busy.
  2. A failed Webmail attempt keeps the Roundcube cleanup offer on the row, even
     after a later clean removal.
  3. The cleanup link on Roundcube's own page reopens the confirmation if the
     page is reloaded.
  4. The buttons in the dialogs are 34 px high on a phone.
  5. The browser screen check (batch 20) was not run again after the last review
     fixes.
- Reading installed package versions for every component remains open (see
  "Known gaps").
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.95,
  and everything the earlier notes list as not reported. Each is "not recorded",
  not "worked" and not "failed". The bundle sequence has not run against a real
  Agent.
- The limits of the earlier publication records (see the
  [alpha.88 notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it does
not update installed servers; only the owner of a server starts its update, from
the panel's own update screen.
