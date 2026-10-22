# v0.1.0-alpha.101 (published 2026-10-22)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.101.tr.md)

This is the release that follows v0.1.0-alpha.100. It was published on 2026-10-22
(see "Publication record" at the end); nothing from it is on an installed server
yet. Written on 2026-10-23 (the code on 2026-10-22), as a candidate, on the branch
`fix/alpha101-real-bundles`, from `main` after the publication record of
alpha.100.

## Who should update, and why

Everyone who runs alpha.100 or an earlier release, once this release is published.
On 2026-10-22 the owner looked at both installed servers on alpha.100. On
Frankfurt the Components page offered a "PostgreSQL" bundle and a "MariaDB" bundle
that were each a single component, the same one that has its own row right below,
and phpPgAdmin and phpMyAdmin as separate add-ons. The owner's decision, in his
words: "bundle diyip tek servisi kurmak nedir. saçmalık. postgresql ve phpPgAdmin
bundle dır mariadb ve phpMyAdmin bundle dır. Diğer tüm bundle lar da böyle dir."
(In English: what is calling it a bundle and installing a single service?
Nonsense. PostgreSQL and phpPgAdmin is a bundle, MariaDB and phpMyAdmin is a
bundle. All the other bundles are like this too.) A ready bundle is now a service
together with its companion tools (D-034 amendment). Two smaller things the owner
saw on Frankfurt are fixed too: the installed bundle's window said "Already
installed" beside every part, and phpMyAdmin's version read "5.2.2-really…".

The owner also looked at Server setup in the sidebar, the Dashboard's server
purpose button, the PostgreSQL removal dialog and the My email card on Boston;
this release does not change them.

Only the owner of a server updates it, from the panel's own update screen.

## What changes on screen

**Ready bundles (Components, at the head of each category; the Dashboard card).**
The list is the same on every server, in this order:

| Category | Bundle | What it installs, in order |
| --- | --- | --- |
| Mail | Core Mail | Postfix, Dovecot |
| Mail | Webmail | Postfix, Dovecot, Nginx, PHP-FPM, Roundcube |
| Mail | Spam-Protected Mail | Postfix, Dovecot, Rspamd |
| Database | **PostgreSQL + phpPgAdmin** | PostgreSQL, Nginx, PHP-FPM, phpPgAdmin |
| Database | **MariaDB + phpMyAdmin** | MariaDB, Nginx, PHP-FPM, phpMyAdmin |
| Web | Nginx + PHP-FPM | Nginx, PHP-FPM |

The one-component "PostgreSQL" and "MariaDB" bundles and the separate
"phpPgAdmin" and "phpMyAdmin" add-ons are gone; what they installed is inside the
two database bundles. The mail bundles and Nginx + PHP-FPM are unchanged. Six
bundles are drawn and counted, where there were eight.

- **Installing a database bundle** works as every component bundle did: the window
  lists what is kept, what is not needed and what will be installed, then installs
  one component at a time and stops at the first failure with its reason and
  Continue. Nothing installed is removed.
- **Beside Apache** the database bundles stay installable: Apache already fills
  the web server role, so Nginx is listed as "not needed", not as a conflict, and
  a database bundle whose other parts are installed is installed and verified
  without Nginx. "Nginx + PHP-FPM" names Nginx itself and stays blocked beside
  Apache, as before.
- **On Arch Linux** phpPgAdmin has no package, so "PostgreSQL + phpPgAdmin" is
  blocked as a whole and its chip says "phpPgAdmin is not available on this
  system". It never installs only part of a bundle. "MariaDB + phpMyAdmin" stays
  installable there.

**The installed bundle's window.** Pressing an installed bundle opens a window
that lists each part with its own row's state and version — "Running · 1.26.3",
"Installed · 5.2.1", "Stopped" — with the full package version in the tooltip,
instead of "Already installed" beside every part. The Verify button and the
sentence "Verify reads the components again…" are gone: the page's own Rescan at
the top does that. Close stays. Mail bundles keep Repair, which really runs the
mail setup again (it completes what is missing and removes nothing). When an
installed component bundle is not verified because a part is not running, that
part is shown in its row's red and the window says to start it from its row below
or to use Rescan at the top of the page, and that the window changes nothing on
the server.

**Versions on the Components rows.** A Debian "really" version is shown as the
version actually shipped: a version such as `5.2.2-really5.2.1-1` reads 5.2.1,
not "5.2.2-really…" as phpMyAdmin did on Frankfurt. The full version is still in the tooltip and on the component's
own page. A row with a single runtime version now ends in an ellipsis with the
whole text in the tooltip, so no version is clipped without a sign.

**What the owner's servers will show, as the panel works it out** (from their
alpha.100 states; not yet seen on alpha.101):

- **Boston** (PostgreSQL and Nginx installed, no PHP-FPM, no phpPgAdmin):
  "PostgreSQL + phpPgAdmin · Install". Its window keeps PostgreSQL and Nginx and
  installs PHP-FPM, then phpPgAdmin, one at a time. The Dashboard's "Ready
  bundles" card: "None installed yet · 6 can be installed".
- **Frankfurt** (everything installed): "PostgreSQL + phpPgAdmin" and "MariaDB +
  phpMyAdmin" with a check mark; the card: "6 installed and verified".

## What changes underneath

- **The bundle registry** (`bundleDefinitions` in `cmd/panel/bundles.go`) is
  compiled into the Panel. Bundles are not read from a file or the database and
  cannot be changed while the panel runs; changing one needs a release. A data
  catalogue for bundles is possible later if the owner wants it.
- **Ids kept, names changed.** `db.postgresql` and `db.mariadb` keep their ids, so
  links still land on the same bundle; their name codes are now
  `db_postgresql_phppgadmin` and `db_mariadb_phpmyadmin`, so a page never calls an
  older panel's one-component bundle a bundle with a tool. `db.postgresql.admin`
  and `db.mariadb.admin` no longer exist.
- **Prerequisites.** The web server and PHP-FPM a database tool runs on are listed
  as prerequisites: they are in the bundle for the role they fill, and another
  installed component that fills the role (Apache) serves instead.
- **A registry contract test** (Go) enforces the owner's rule: every component
  bundle has at least two components and at least one that is not only a
  prerequisite; mail bundles are exactly their mail profiles; the retired ids and
  name codes do not come back.
- **No Verify action.** The Panel sends `none` where it sent `verify`; the page
  still accepts `verify` from an older panel and draws no button for it. The page
  now leaves out a bundle id the answer does not carry instead of refusing the
  whole list.
- **The version short form** takes the version after "really" when "really"
  follows a version separator (`+`, `-`, `~` or `.`) and is followed by a digit,
  after removing the epoch and the packaging revision as before. It is worked out
  when the Panel builds its answer, so versions read before the update are shown
  correctly at once.
- **The removal dialog** names the new bundles when a removal leaves one
  incomplete (removing Nginx now leaves both database bundles incomplete).
- **Release sequence 101 after 100**, the bootstrap pins and the README pins.

## What does not change

How a bundle installs (one ordinary component install at a time, Continue after a
failure, nothing removed), the mail bundles and their Repair, Nginx + PHP-FPM, the
rest of the Components page, every refusal, roles, the Agent, the updater, release
recovery, the schema (44, no migration) and polling. No native record is made, by
the owner's instruction of 2026-10-16 (no native record unless the updater's
mutation path changes).

## Known gaps

- Nothing is measured on an installed server.
- The bundles are fixed in the release; there is no way to define your own bundle.
  Whether they should move to a data catalogue is left to the owner.

## Evidence

On the candidate tree:

- Go: the registry contract test; bundle tests shaped like Boston, Frankfurt, a
  server with Apache and an Arch server; the removal-impact test with the new
  bundles; the short-form tests with real Debian and Ubuntu version strings
  (`4:5.2.2+dfsg-1` → 5.2.2, `1:5.2.1+dfsg-3` → 5.2.1, `5.2.2-really5.2.1-1` →
  5.2.1, `2:4.1.0+really4.0.9-2` → 4.0.9, `3:7.2` → 7.2, and `~` versions).
  Passed on the final tree in the `CelikPanel-S2-Debian` guest: 7,199 passed,
  44 skipped, 0 failed in 64 packages; gofmt and vet clean.
- Web: the ready-bundle mounted and contract tests (Boston, Frankfurt and Arch
  payloads, each part's state in the window, no Verify, the not-verified
  sentence, an older panel's `verify`, unknown ids left out), the version cell
  tests and the removal dialog tests with the new names. 1,745 of 1,745
  passed; the build passed with every bundle budget unchanged.
- Browser: batches 19 and 20 with the alpha.101 registry (1440 px and 390 px, English and Turkish, light and dark): the Boston- and Frankfurt-shaped strips, Boston's PostgreSQL + phpPgAdmin plan and its install (step 2 of 2 is phpPgAdmin), Frankfurt's installed dialogues and one with MariaDB stopped, and the Dashboard card; one defect was found and fixed (the version ran into the dot after the state word; it is now set apart in mono) and the screens were looked at again; no raw codes, no English on the Turkish page, no horizontal overflow. The screenshots were deleted.
- Shell: the release sequence policy contract passed in Git Bash ("100 -> 101;
  historical bootstrap sequence 100"); the download portal contract and the
  signed release manifest contract passed in the `CelikPanel-S2-Debian` guest.

## How to update

Once published: from the panel's own update screen (Settings → CelikPanel updates),
by the owner of the server. Nothing needs to be done before or after.

## Publication record

Everything below is dated 2026-10-22 (UTC unless a time zone is named). The release
commit carries 2026-10-23T01:34:29+03:00, which is the same instant as 22:34:29Z,
so it appears under both dates in different places. The merge, the checks, the
portal publication and the read of the public site were done by the release
preparer under the owner's standing instruction, as for the earlier releases;
installed-panel updates are the owner's alone, from the panel's own update screen.
No native record for the publication, by the owner's instruction. This entry
stores none of the results below as a file in the repository.

**What happened.**

1. Pull request #240, "Release v0.1.0-alpha.101: a bundle is a service with its
   companion tools", was merged into `main` as commit
   `f860053bea4772f008337238e5787c857fbb5efb`, authored by
   Mehmet Ömer Efe Çelik. The manifest's `published_at` is 2026-10-22T22:34:29Z.
2. The release is a pre-release, `v0.1.0-alpha.101`, published on GitHub at
   2026-10-22T23:13:34Z.
3. Checked by the preparer before the publication (not recorded in a file here):
   the archives are 66,346,084 bytes, SHA-256
   `6de8f594705e22250c56a6c45fe6a2b10c6c1e7db270eeb706878f3069c5f549`; the
   checksums, the manifest and the archive members were checked; the manifest's
   signature verifies with the release key (SHA-256 `7eadeb0b…c68bb`); both content
   guards passed; `libexec/get.sh` pins 101; the policy inside says 101 after 100;
   the interface bundle is `web/dist/assets/app-EnAd5I-6.js`.
4. The portal package was built twice and came out byte-identical; the promoter's
   own extract and normalize step accepted it.
5. At about 23:22Z the preparer published the portal: status `committed`, public
   verification 26 of 26 requests ok.
6. A read from outside after the publication: `https://celikpanel.net/releases/latest.txt`
   says v0.1.0-alpha.101; `latest.json` says sequence 101 with commit `f860053…`.

**What the owner reported.** On 2026-10-22 the owner had looked at both installed
servers, Boston and Frankfurt, on alpha.100; the screens he saw are what this
release answers (see "Who should update, and why" above). Nothing from alpha.101
on an installed server had been reported when this was written.

**Not done, and what publication does not change.**

- The installed servers were not updated to alpha.101 by the release preparer.
  Only the owner updates them, from the panel's own update screen. alpha.101 is
  on no installed server yet.
- Nothing from alpha.101 has been measured on an installed server: not the
  renamed database bundles, not the installed bundle's window, not the corrected
  version on Frankfurt's phpMyAdmin row, and not Boston's "PostgreSQL +
  phpPgAdmin" plan. The owner's observation of alpha.101 after he updates is
  still to come.
- The setup plan's contact-email requirement applies to every server plan. Whether
  it should is left for a decision.
- Whether the bundles move to a data catalogue is left to the owner (see "Known
  gaps" above).
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.101,
  and everything the earlier notes list as not reported. Each is "not recorded",
  not "worked" and not "failed". What the alpha.100 record leaves open stays open.
- The limits of the earlier publication records (see the [alpha.88
  notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- No code, schema (44) or recovery behaviour changed in this record.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.
