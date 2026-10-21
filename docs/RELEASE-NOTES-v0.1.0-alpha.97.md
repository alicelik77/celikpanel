# v0.1.0-alpha.97 (published 2026-10-21)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.97.tr.md)

This is the release that follows v0.1.0-alpha.96. It was published on 2026-10-21
(see "Publication record" at the end); its versions have not yet been seen on an
installed server. The number was chosen by the release preparer under the owner's
standing instruction of 2026-10-13. Written on
2026-10-21, on the branch `feat/alpha97-component-versions`, from `main` after the
publication record of alpha.96 (the alpha.96 release commit is
`08faa91e5a8c6622072b40f0f87c9c388035c9a3`).

## Who should update, and why

Everyone who runs alpha.96 or an earlier release, once this release is published.
On the owner's alpha.94 screens of 2026-10-17 the Components page showed a dash as
the version of every single-version component; alpha.95 removed that column and
left "read installed package versions for every component" open. This release reads
them (D-036). It also explains, and corrects how the page says, why Frankfurt's
update to alpha.95 stopped: the server's package manager was busy with Debian's
automatic package updates. Only the owner of a server updates it, from the panel's
own update screen.

## What changes on screen

**Real versions on Components.** An installed component's row says the version its
package database reports, in its short form, in mono: Nginx `1.26.3` for
`1.26.3-3+deb13u1`, MariaDB `11.8.3` for `1:11.8.3-0+deb13u1`. The tooltip has the
full version and "from the system package database". On a desktop the versions
stand in their own column, drawn only when at least one installed component on the
page has a known version; when none is known the page looks as in alpha.95, with no
column of dashes. On a phone the version is under the description. PHP-FPM and
Node.js keep the versions of their installed copies.

**The full version on every component page.** Under the component's name and
status: "Installed version: 1:11.8.3-0+deb13u1" and "Read from the system package
database." This is on the generic component page and on the specialised ones
(Nginx, Postfix, Fail2ban, PowerDNS, MariaDB, PostgreSQL…) alike.

**Unknown stays unknown.** A component that is not installed, or whose version could
not be read, says nothing: no dash, no "default", no guess. A component whose
packages report different versions (for example SpamAssassin and its milter) says
"several versions" on its row, and its page names each package with its version.
When the server's package manager was busy at the last component check, the earlier
reading is shown and said as such ("Read from the system package database on
{time}. At the last check the package manager was busy, so this earlier reading is
shown; the next check reads it again.").

**Why an update stopped, in plain words.** Frankfurt's update to alpha.95 stopped on
2026-10-17 because the server's package manager was busy (Debian's automatic package
updates were running). The stop dialog now says so: "Reason: the server's package
manager was busy, for example with automatic security updates. Nothing was
changed." and "Start the update to {target} again when it has finished; the update
card in Settings says when the package manager is free. Nothing starts by itself."
The update card's notice about such an earlier attempt says the same, calmly, and
its readiness check, which already re-reads quietly while the server is busy, says
"The package manager has finished. The update can be started again." when it is.
A stop because the server's recovery tool could not be started or verified now has
its own reason ("Reason: the server's recovery tool could not be started or its
installation could not be verified. The server administrator checks it, using the
server's line below; …") with the server's first line under it. A long diagnostic
line is no longer dropped: the panel keeps its first line, at most 240 characters,
without paths outside the product's own, addresses, values of assignments or
token-like strings. While a step's words are still loading, the reason line says
"Reading the reason…" rather than the generic reason.

## What changes underneath

- **A new read-only Agent capability.** `Agent.InstalledPackageVersionsV1` asks the
  system package database once per component scan, for the whole catalogue in one
  query: `dpkg-query -W` with a fixed format on Debian and Ubuntu, `pacman -Q` on
  Arch. Fixed arguments, no shell, no network, a 10-second timeout and an 8 MiB
  output ceiling. The packages asked about are the Agent's own catalogue. It never
  runs while a package operation holds the package manager: it reads the process
  table and the kernel's lock table first, takes no lock itself, and answers
  "busy" when the package manager is in use or its state cannot be read.
- **The Panel** admits the method as a read in its Agent RPC policy (like the other
  scan reads), calls it once per scan without ever failing the scan, and keeps the
  answer with the existing component scan cache as an additive JSON field. A cache
  row written before alpha.97 reads as unknown. No schema change.
- **The screen catalogue is split.** The two Turkish screen catalogue files were
  within 0.29 and 1.00 KiB of their size ceiling. The screen copy is now nine parts
  per language, along groups of screens (setup, sites, siteConfig,
  dnsCertificates, components, mailDatabases, security, dashboard, settings), each
  with its own ceiling set with at least 25% headroom; the rule for where a new text
  goes and how a part is split again is in the header of
  `web/scripts/check-bundle-budget.mjs`. Every part is still loaded before a screen
  is drawn, and no text changed in the split.
- **Browser mocks** report versions only as the new fields would, with sample Debian
  13 strings for Frankfurt-shaped data and Ubuntu 24.04 strings for Boston-shaped
  data, labelled as sample data.

## What the independent review corrected (2026-10-21)

- Every package dpkg reports as installed counts (current state "i", no error
  flag), also a held one or one marked for removal; the first form counted only
  "ii". A package that needs reinstalling still shows no version.
- The version query runs with a fixed environment (`LC_ALL=C`, `LANG=C`, a fixed
  `PATH`) in its own process group; an answer over the 8 MiB ceiling kills the
  query at once and is reported as "larger than the read ceiling", no longer as a
  generic failure after the query ran to its end.
- The busy check reads at most 1 MiB of the kernel's lock table and skips locks
  without an inode; a pacman lock file left by a killed pacman, with no pacman
  running, no longer keeps every later check "busy".
- A reading kept for more than a day adds: "The package manager has been busy at
  every check for more than a day. If no installation or update is running on the
  server, the server administrator checks whether a package operation is stuck."
- The Components page's own scan reads the versions before it takes the panel's
  change lease, so the lease is not held while the package database answers.
- The stop dialog never waits for ever: a reason already in the loaded copy is shown
  at once, others say "Reading the reason…" for at most 10 seconds and then the
  boot copy's own sentence. The new reasons of this release moved from the boot copy
  to the settings screen part (critical boot path 359.20 of 361 KiB).
- The server's line also treats a path after `:`, `,`, `;`, `>`, `|`, a backtick or
  `@` as a path, hides a base64-like value split by `/` (a `/`-leading run is kept
  only inside the product's own directories), and keeps `mode=0775` readable.
- Turkish: the `compatibility` reason says "… aşağıdaki sunucu iletisine bakarak bunu
  denetlemeli"; packages that differ read "Paketlerinin sürümleri farklı:".

## What does not change

- The updater's mutation path: `update.sh`, the behaviour of `get.sh`, the Agent's
  system update worker, rollback and recovery. The Agent change is a read-only
  capability and its RPC policy. Therefore no native record is made, by the owner's
  instruction of 2026-10-16.
- Schema 44, no migration. No new Agent job kind. No package is installed, upgraded
  or refreshed by the version read, and no update offer is made from a version.
- Hosted websites, mail, DNS, databases, scheduled jobs and certificate renewal do
  not depend on the version read.
- Installed panels are updated only by their owner, from the panel's own update
  screen.

## Known gaps

- **Nothing was measured on an installed server.** The version read ran against a
  faked runner and lock-table fixtures in the Go tests and against a mock server in
  the browser batch; the real `dpkg-query`/`pacman` answer on Boston and Frankfurt
  is not yet seen. The open item "read installed package versions for every
  component" is closed only when the owner sees the versions there.
- **The critical boot path has 1.8 KiB of its budget left** (359.20 of 361 KiB;
  2.99 KiB were left before this release). Only the line that waits for a reason is
  new boot copy; the reasons are screen copy. The limit was not raised.
- **Boston's result of its alpha.95 update** has not been reported.
- On a desktop the component page header is a little taller with the version line,
  so the action buttons sit slightly below the title line; on a phone a long kept
  sentence takes three lines. Readable; not changed.
- **Two components read differently from the others, by design.** SpamAssassin
  always reads "several versions": its catalogue packages include
  `spamass-milter`, which comes from another source and has its own version. The
  WireGuard row shows the version of the `wireguard` meta-package on Debian and
  Ubuntu (on Arch, `wireguard-tools`), not of the kernel module.
- **alpha.97 back to alpha.96** without the database snapshot: alpha.96's
  Components page, Dashboard and component pages read the cache row (they ignore the
  new fields), but alpha.96's Store refuses unknown fields and says "The stored record
  of this server's components could not be read" until alpha.96's next component
  scan writes the row again without them (it comes by itself while an administrator
  has the Dashboard, Components or a component page open). Nothing is lost; it heals
  with that scan. A Go test mirrors both alpha.96 decoders.
- P0.1-P0.5 stay **partial**; no P0 item is closed or advanced; a candidate is not
  acceptance.

## Evidence

Everything below was run on the candidate tree on 2026-10-21 in a development
environment; none of it was run on an installed server.

- Go: `./cmd/panel/` and `./internal/...` 7156 passed, 0 failed, 44 skipped; the
  Agent `./cmd/agent/` 4657 passed, 0 failed, 29 skipped, run in a clean mount
  namespace in the `CelikPanel-S2-Debian` WSL guest (empty agent state, no
  `celikpanel` group) as earlier Agent runs were; `go vet` and gofmt clean
  (`-timeout 45m`). New Agent tests cover the dpkg and pacman parsers (normal,
  missing package, removed and half-installed packages, several architectures,
  odd characters, overlong text, epoch), the catalogue mapping (meta-package versus
  versioned package, several packages, absent components), the one fixed query,
  the timeout, the busy and unreadable probe (no query is made), the lock-table
  reading, and failures carrying no output; after the review also held, removal
  and reinstall-required dpkg states, a real process run past a small output
  ceiling and past a small timeout (its process group is gone, the answer is "too
  large"), the fixed environment, a lock without an inode and an over-long lock
  table. New Panel tests cover the RPC policy
  (a read on every family; no similar mutation admitted), the short form, several
  versions, unknown, the busy keep, old cache rows, and the version passing through
  the scan, the cache and both managed-services answers; and the bounded,
  sanitized update diagnostic.
- Web: `npm test` 1675 passed, 0 failed; `npm run build` succeeded within the
  budgets: the largest Turkish screen part (setup) 55.84 of 80 KiB, components
  44.49 of 64 KiB, settings 31.93 of 44 KiB; critical boot path 359.20 of 361 KiB.
  Mounted tests cover the version column appearing only when a version is known, the
  short and full forms, several versions, unknown showing nothing, the component
  page header, the busy package manager stop (with the record shape Frankfurt had),
  the `compatibility` step, the reason waiting for its words, the update card's
  notice and its "free"/"still busy" line, and every key of every screen part
  resolving through the real provider in English and Turkish.
- Release-recovery harness (it mirrors the update card's texts and outcome logic):
  `python3 -m unittest discover -s deploy/e2e/release-recovery -p 'test_*.py'` in a
  fresh clone in the `archlinux` WSL guest: 1109 tests, OK (1 skipped). Its mirror
  of a busy package manager stop was updated to the calm stop.
- Browser batch 22 (mock server): Components on Boston-shaped and Frankfurt-shaped
  data, with and without known versions, and component pages (Nginx, SpamAssassin
  with several versions, BIND with a kept reading, phpMyAdmin with none), in English
  and Turkish, light and dark, at 1440 and 390 px. The first look found that the
  specialised component pages (Nginx, Postfix, Fail2ban, PowerDNS) never showed the
  version, because it was in the generic page's overview card; it moved to the
  header every component page shares. The second look: the version line on every
  page, no raw key, no English on a Turkish page, no horizontal overflow at 390 px,
  no signal colour on a version, no column and no dash when no version is known.
- Release contracts (sequence policy 97 after 96, download portal, signed manifest):
  sequence policy (Git Bash) "96 -> 97" passed; download portal and signed manifest
  (WSL guest) passed.

Component, mounted and mock-browser tests do not establish native behaviour.

## Publication record

Everything below is dated 2026-10-21 (UTC unless a time zone is named). The
merge, the tag, the checks, the portal publication and the read of the public
site were done by the release preparer under the owner's standing instruction,
as for the earlier releases; installed-panel updates are the owner's alone, from
the panel's own update screen. No native record for the publication, by the
owner's instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Three independent review rounds came before the release. Their findings were
   fixed: the dpkg states that mean a package is held, removed or half-installed
   now count; the label for the output cap, with tests that run a real process
   past a small cap; a reason line that could wait for ever; gaps in how the
   sanitizer handles paths; and the note on how alpha.96's Store reads an
   alpha.97 record (described above).
2. Pull request #232, "Release v0.1.0-alpha.97: real versions for every
   component, a readable reason when an update stops, and room in the
   catalogues", was merged into `main` as commit
   `0c1a8917ef3f8141702fa41ae085f64d62a713f0`. The release sequence variable was
   moved from 96 to 97 before the tag was made. The main run and the tag run
   both succeeded; the release chain ran cleanly, with no incident.
3. The annotated tag `v0.1.0-alpha.97` (object
   `e5c64bcc3569513050ebfc89c01185a69cabbfb0`, tree
   `67db021959466f61a2defc232194d897c1eb9cb7`) points at the release commit. The
   release is a pre-release, published at 2026-10-21T20:24:20Z with six assets.
4. Checked from a download in the WSL guest `CelikPanel-S2-Debian` on 2026-10-21
   (preparer; not recorded in a file here): the archives are byte-identical,
   66,312,385 bytes each, SHA-256
   `3784fa392b016a9fbd18ffc372f7017c8c2aab5d3643bf319c25e53514800aa3`; the
   manifest says sequence 97, `published_at` 2026-10-21T19:40:33Z (SHA-256
   `276a972a492beed16524f65f3eb29e2fe590fcbc32a3a9c996ec7063b974c027`), and its
   signature (SHA-256
   `5acfb875718e99791bfdf5811a3371ade1f6d1ca92aeac8d63d809a62cf2927d`) verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the signing key (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   key as before); both content guards returned 0; the build used Go go1.26.5,
   CGO off, `-trimpath`. `libexec/get.sh` pins 97 (SHA-256
   `08ece9d28b3caba0cb94c8bd3659de138fd357352b8b2a5df82f6c31da6cf5bc`); the
   policy inside says current 97, previous 96, previous commit
   `08faa91e5a8c6622072b40f0f87c9c388035c9a3`. The archive has 257 members (233
   files, 24 directories). The interface bundle is
   `web/dist/assets/app-Ckmq5jcD.js`, 256,924 bytes, SHA-256
   `f94433074b4451430875fda814d0dd371df99eae938c6111a41fb609ff719861`.
   `bin/agent` contains `InstalledPackageVersionsV1`.
5. The portal package: `portal.tar.gz`, 132,999,066 bytes, SHA-256
   `61ade15eee9ecd7e136d0eee599534a96c628d6bb38ae2365c0c74f56e339197`, 54
   entries, built twice byte-identical with the tag's
   `deploy/build-download-portal.sh` (pre-signed mode). The contract test
   returned 0; the public verifier's 17 tests and the promotion's 14 tests
   passed.
6. At about 20:36Z the preparer published it with
   `deploy/publish-download-portal.ps1` from the preparer's machine, previous
   version v0.1.0-alpha.96, target v0.1.0-alpha.97: status `committed`, public
   verification 26 of 26 requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
7. The backup of the previous site is
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.97.20261021T203644Z.2234083.ade8d257d372e677`;
   the upload was retained in
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261021T203602Z-v0.1.0-alpha.97-69cbd3f06d952c28`.
8. A read from outside after the publication:
   `https://celikpanel.net/releases/latest.txt` says v0.1.0-alpha.97;
   `latest.json` says sequence 97, with the commit and the SHA-256 above.

**What the owner reported.** On 2026-10-21 the owner sent a screenshot of
Settings that shows Boston on v0.1.0-alpha.96 (build 08faa91e5a8c). For the stop
of Frankfurt's update to alpha.95 the owner ran the two read-only commands given
on 2026-10-21; the stored line was `!! CELIKPANEL_UPDATE_FAILURE
code=package_manager_busy state=unchanged reason=the host package manager is
active detail=` (2026-10-17T12:25:23Z). Debian's automatic package updates were
running; nothing changed on the server. alpha.97 makes the screen say this (see
"What changes on screen" above; not repeated here). Frankfurt's update after
that was not reported when this was written. The owner also renamed the GitHub
account (celikbros to alicelik77). The repository is now
`alicelik77/celikpanel`; the old `celikbros/celikpanel` name is retired by
GitHub and redirects permanently. Commits are made under the owner's chosen
identity.

**Not done, and what publication does not change.**

- The installed servers were not updated to alpha.97 by the release preparer.
  Only the owner updates them, from the panel's own update screen.
- The versions this release reads have not yet been seen on an installed server.
  The open item "read installed package versions for every component" closes
  only when the owner sees them on Boston and Frankfurt.
- The critical boot path has 1.8 KiB of its budget left.
- The public portal still links `celikbros/...` GitHub addresses. They redirect,
  and are to be updated when the portal next changes.
- The menu-layout proposal for Settings (its contents) awaits the owner's
  decision.
- Not reported and not recorded: Frankfurt's update after its alpha.95 stop, how
  Boston reached alpha.96, anything from Boston or Frankfurt on alpha.97, and
  everything the earlier notes list as not reported. Each is "not recorded", not
  "worked" and not "failed".
- The limits of the earlier publication records (see the [alpha.88
  notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.

Install this release only through CelikPanel's update interface. Publishing it does
not update installed servers; only the owner of a server starts its update, from
the panel's own update screen.
