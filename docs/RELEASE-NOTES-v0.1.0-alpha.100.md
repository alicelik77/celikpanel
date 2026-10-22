# v0.1.0-alpha.100 (published 2026-10-22)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.100.tr.md)

This is the release that follows v0.1.0-alpha.99. It was published on 2026-10-22
(see "Publication record" at the end); nothing from it is on an installed server
yet. Written on 2026-10-22, as a candidate, on the branch
`feat/alpha100-removal-and-setup`, from `main` after the publication record of
alpha.99. It is the first release whose prerelease number has three digits.

## Who should update, and why

Everyone who runs alpha.99 or an earlier release, once this release is published.
After alpha.99 the owner looked at a proposed redesign of the Components page and
asked why it would be better. Most of it would not have been (the page already
remembers the view the owner chose and folds the catalogue), so the broad redesign
was withdrawn. The owner approved three things from that review, and only those
(2026-10-22, "1 evet, 2 b, 3 evet"; D-037 amendment):

1. the removal dialog says, by name, what removing a component affects on this
   server, what is removed and what stays;
2. the first visit to Components opens the installed view, as D-010 always said;
3. Server setup leaves Settings for its own page under Server, directly under
   Components.

Only the owner of a server updates it, from the panel's own update screen.

## What changes on screen

**The removal dialog.** Components → the bin button on a row → "Uninstall X?".
When the dialog opens it reads, once, what the removal affects; while it reads it
says "Reading what removing X affects on this server…" with a spinner, and Remove
stays available. Then it shows three parts:

- **Affected on this server** (first): what the panel hosts on the component, by
  name, five at most and then "+N more" — sites for the web server, PHP or Node.js
  sites for a runtime, databases for a database engine, mail addresses and
  forwardings for Postfix and Dovecot, mail domains whose mail is no longer
  checked for spam for Rspamd or SpamAssassin, mailboxes that lose webmail for
  Roundcube, VPN peers that can no longer connect for WireGuard, FTP accounts for vsftpd, and for certbot the certificates the panel renews with it (the panel's unless you serve your own, the mail host certificates server setup recorded, the sites' with automatic renewal) with "Their renewal stops."  When those
  things make the panel refuse the removal, a bold line says "CelikPanel does not
  remove X while these exist: move or delete them first." For the firewall engine:
  the panel firewall's state (on with N open ports, off, or not known) and "CelikPanel can no longer apply, change or restore it; rules already loaded may stay until the server restarts."  For Redis, Valkey and Memcached: "CelikPanel does not record which sites
  use X." For certbot and the firewall engine: "Server setup requires X on every
  server it prepares…". Then the installed components that need it ("Components
  that need X and stop working: …"), the ready bundles it leaves incomplete ("The
  ready bundle "Nginx + PHP-FPM" becomes incomplete."), and the ports it asked the
  firewall for. Only with none of these: "CelikPanel records nothing else on this server that depends on X." 
- **What is removed:** what the panel's removal really does for this component
  (below), the packages, and the package manager command on apt and pacman. For a
  database engine a warning line follows: the database files may be deleted by the
  package's own removal script, which runs without asking; back them up first.
- **What stays:** CelikPanel deletes nothing else itself; a package's own removal
  script may delete data that package created, and that is the distribution
  package's decision.

If the read fails or takes longer than 10 seconds: "What removing X affects on this
server could not be read. Nothing was changed. You can still remove it, but check
first: Domains and Databases show what runs on this server." with Retry (a read
only); the old general warning stays under it. Native cron, which the panel never
removes, now shows its refusal in the dialog as soon as it opens. The Remove button
is still the outlined red one; nothing else on the dialog is red.

What the dialog says is removed and kept, from the code of this release:

| Component | Removed | Stays |
| --- | --- | --- |
| A package component | Its services, if the catalogue names any, are stopped and kept from starting at boot (certbot, the firewall engine, phpMyAdmin and phpPgAdmin have none). On apt, `apt-get purge -y --auto-remove` removes the packages, their configuration files and any automatically installed package that nothing needs any more — system-wide, not only those installed for it. On pacman, `pacman -Rns --noconfirm` removes the packages, their configuration files and their dependencies that nothing else needs. The firewall, if on, is set again from what stays installed. | Site files, mailboxes and database files are not touched by CelikPanel. A package's own removal script, which runs without questions, may delete data it created; which data is not claimed. |
| PHP-FPM, PostgreSQL on apt | As above, plus every installed version (`php8.3-fpm`, `postgresql-17`, …) and the packages installed beside each. | As above; a database engine adds the warning to back up first. |
| Rspamd, SpamAssassin | As above, and Postfix stops passing mail through it. | Mail stays; it is no longer checked for spam. |
| phpMyAdmin, phpPgAdmin | As above, and the panel stops serving its pages. | As above. |
| certbot | As above. The certificates the panel renews with it stop being renewed. | As above. |
| WireGuard | As above. The VPN peers the panel keeps active can no longer connect. | As above. |
| The firewall engine (nftables) | As above. CelikPanel can no longer apply, change or restore the panel firewall. | Rules already loaded may stay until the server restarts. |
| No catalogue packages on this package manager (Redis, SpamAssassin, phpPgAdmin on pacman) | Nothing: Remove is refused ("cannot be removed automatically on this system yet"). | Everything. |
| Roundcube | Its application files, its own settings and address-book database, and the `/webmail/` entry. | Mailboxes and Maildir data, other sites, and the Nginx and PHP-FPM packages. |

**The first visit to Components** opens "Installed". "Catalog" opens only when you
chose it before (the choice is remembered in the browser, as before). A Roundcube
row that offers its leftovers cleanup stays visible in the installed view.

**Server setup** is a Server item directly under Components (`/server-setup`,
administrators only): the server's purpose, the setup's state, "Extend this
server's setup" and the setup wizard. The Dashboard's "Server purpose" card has an
"Open server setup" button. An old link or bookmark to Settings → Server setup
arrives there. Settings now has three sections: Panel address and HTTPS, License,
CelikPanel updates.

## What changes underneath

- **A read-only answer for the dialog:** `GET /api/v1/service/removal-impact?service_id=…`
  (administrators only, through the `/api/v1/service/` prefix; `Cache-Control:
  no-store`). It reads the catalogue, the stored component record, the panel's
  database and the panel certificate file; no new Agent call and no mutation (the
  host profile may come from cached Agent facts, and for the firewall engine the
  firewall's state is read through the existing read, bounded to 5 seconds). No
  stored record yet: `SERVICE_STATE_UNVERIFIED`. The DNS engines keep
  `DNS_ENGINE_WORKFLOW_REQUIRED`, native cron `NATIVE_CRON_REMOVAL_REFUSED`.
- **One set of conditions.** The uninstall handler and the Agent's uninstall now ask
  the same small questions the answer reports (whether Postfix's filter chain is
  set again, whether the database tools are set again, and
  `core.UninstallPurgesAllVersions`), so the dialog cannot promise something the
  removal does not do. Nothing the uninstall does changed.
- **The guard is the same.** The hosted things that block a removal are the existing
  guard's own answer (`SERVICE_HAS_DEPENDENTS`); mail domains for a spam filter and
  mailboxes for Roundcube are counted for the dialog only and never block.
- **Server setup's guidance.** The panel's refusal for an address record that server
  setup manages now carries the action `/server-setup`.
- **Release sequence 100 after 99**, the bootstrap pins and the README pins.

## Three-digit version audit

alpha.100 is the first prerelease number with three digits; as plain text
"alpha.100" sorts before "alpha.99". Every place that parses, sorts or compares a
CelikPanel version was read:

| Place | Finding | Test added |
| --- | --- | --- |
| Agent updater (`cmd/agent/system_update_contract.go`, SemVer precedence; the sequence floor) | numeric | alpha.100 > alpha.99 both ways, 1000 > 999, leading zero refused, manifest and floor round trip, floor accepts 100 over 99 and refuses 99 under 100; an alpha.99 build is offered and may start alpha.100, an alpha.100 build never alpha.99 |
| Panel update discovery (`comparePanelUpdateVersions`, `validPanelUpdateVersion`) | numeric | both directions, equality, 1000 > 999, validation |
| Release sequence policy test (`sequence_gt`, fixed-width) | numeric | 100 > 99 both ways; v0.1.0-alpha.100 canonical, alpha.0100 not |
| Release-recovery foundation (`release_recovery_sequence_gt`: length, then digits) | numeric | 100 > 99, not 99 > 100, not 100 > 100 |
| `get.sh` (sequence floor and pins) | numeric `-ge`; versions compared only for equality | the contract tests pin 100 |
| CI tag check, portal builder and promoter, signed manifest writer | validation and equality only | the pinned contracts pass with 100 |
| Web update target decoder | validation only; the page never orders versions | reads alpha.100 and sequence 100, refuses sequence 0100 |
| Release-recovery harness (Python) | fixed lab versions, no ordering of the current release | none needed |

Nothing was found that orders versions as text.

## What does not change

The rest of the Components page (rows, buttons — restart, stop, Manage and remove
stay visible, no "⋯" menu —, bundle strips, badges, folding), the uninstall itself,
every refusal, the DNS engines' own workflow, roles, the Agent's behaviour, the
updater, the schema (44, no migration) and polling. No native record is made, by
the owner's instruction of 2026-10-16 (no native record unless the updater's
mutation path changes).

## Review corrections

An independent review of the candidate found the removal behaviour unchanged and
asked for these before any publication; they are in source: certbot and the
firewall engine were said to have nothing depending on them (they now name the
certificates whose renewal stops, the panel firewall, and server setup's
requirement); the caches now say only that CelikPanel does not record which sites
use them; apt's system-wide `--auto-remove` and pacman's dependency removal are
said per family; the database warning is its own line under what is removed; a
tool without units is not said to be stopped; a package family without catalogue
packages is said to be refused, with no purge; Roundcube's kept sentence names the
Nginx and PHP-FPM packages and its failed read shows what its removal deletes; the
Agent tests are run and recorded. A second, shorter browser look (the removal
dialog for Nginx, PostgreSQL, Postfix, certbot and the firewall engine, the read
held open and refused; 4 configurations, 28 screenshots) found nothing more;
screenshots and the driver deleted.

A second review round asked for more, also in source: a firewall answer with
its own error or an engine that cannot be read is "not known"; the firewall
engine's sentence says CelikPanel can no longer apply, change or restore the
panel firewall and that rules already loaded may stay until the server restarts
(the panel loads them with `nft -f`); WireGuard names the VPN peers that can no
longer connect and vsftpd the FTP accounts the panel records; certbot counts only
the site certificates the renewal job renews (Let's Encrypt, automatic renewal
on, active), leaves out an owner-served panel certificate and names a mail host
certificate only when server setup recorded its step as succeeded; the firewall
read never waits for a firewall change; and the empty case now reads "CelikPanel
records nothing else on this server that depends on X".

## Known gaps

- Nothing is measured on an installed server.
- What a distribution package's own removal script deletes (for example a
  database's data directory on purge) is neither read nor claimed; the dialog says
  who decides.
- The dialog's lists come from what the panel records; for the caches it says
  so. Something configured outside
  the panel that uses a component is not known to it.
- Settings' "Get certificate" button is 34 px tall on a phone (older than this
  release, unchanged).
- The web update decoder accepts a leading zero in a numeric prerelease identifier;
  the Panel and the Agent refuse one before anything reaches the page.

## Evidence

On the candidate tree:

- Go: `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO off) in the `CelikPanel-S2-Debian` guest: 64 packages ok, 2913 top-level tests passed, 42 skipped, 0 failed (4282 subtests passed); `go test ./cmd/agent/...` in a clean mount namespace (empty bind mounts over `/var/lib/celikpanel-agent-private` and `/run/celikpanel`, a copy of `/etc/group` without the host's `celikpanel` group; the host untouched): 1821 top-level tests passed, 29 skipped, 0 failed (2838 subtests passed); gofmt clean and `go vet ./cmd/... ./internal/...` clean.
- Web: `npm test` 1740 of 1740 passed (new: `tests/removal-dialog-alpha100.test.mjs`, 20 tests; `tests/server-setup-page-alpha100.test.mjs`, 10 tests; a three-digit
  version case in `tests/panel-update-discovery-mounted.test.mjs`); `npm run build`
  within every budget (critical boot path 331.08 KiB of 361 KiB; the limits were
  not raised).
- Shell: the release sequence policy contract (in a fresh clone in the `archlinux`
  guest: "99 -> 100; historical bootstrap sequence 99"), the download portal contract
  and the signed release manifest contract passed.
- Browser batch 25: 13 states in each of 8 configurations (Turkish and English, light
  and dark, 1440 and 390 wide; 104 screenshots), looked at twice. The first look found
  the could-not-read notice's Retry 34 px tall on a phone (the shared notice is at
  least 44 px now), "Server setup" said twice on its page, and the requirement line
  reading backwards; all fixed. The second look found nothing else. Screenshots and
  the browser driver were deleted afterwards.
- The release-recovery harness was not needed (no update-related file or update
  text changed); it was run anyway in a fresh clone in the `archlinux` guest and
  passed (1109 tests, 1 skipped).

## How to update

Once published: from the panel's own update screen (Settings → CelikPanel updates),
by the owner of the server. Nothing needs to be done before or after.

## Publication record

Everything below is dated 2026-10-22 (UTC unless a time zone is named). The merge,
the tag, the checks, the portal publication and the read of the public site were
done by the release preparer under the owner's standing instruction, as for the
earlier releases; installed-panel updates are the owner's alone, from the panel's
own update screen. No native record for the publication, by the owner's
instruction. This entry stores none of the results below as a file in the
repository.

**What happened.**

1. Three independent review rounds came before the release. They found that the
   removal dialog said "nothing depends" for certbot, nftables and WireGuard; the
   wording about apt's automatic removal; the database warning's place; an
   unknown firewall state shown as off; how certificates were counted; and FTP
   accounts mixed up with SFTP accounts. All of it was fixed (see "Review
   corrections" above), and the uninstall behaviour was verified unchanged.
2. Pull request #238, "Release v0.1.0-alpha.100: an honest removal dialog, the
   installed view first, Server setup under Server", was merged into `main` as
   commit `3eeff24561799fb028c3c14f5da55a5de5bba866`, authored by
   Mehmet Ömer Efe Çelik. The release sequence variable was moved from 99 to 100
   before the tag was made. The main run and the tag run both succeeded.
   **One incident, which affected no release:** the first CI run stopped on the Go
   toolchain contract because the decisions record contained a literal test
   command line, which that contract forbids; the sentence was reworded and the
   chain was run again.
3. The annotated tag `v0.1.0-alpha.100` (object
   `7fc97f6efbc222d370b3518fa3fbde3fa854601f`, tree
   `ffd7b6b6cbf90e9da0975bf298712d6a7db6268b`, tagger Mehmet Ömer Efe Çelik)
   points at the release commit. The release is a pre-release, published at
   2026-10-22T19:03:06Z with six assets.
4. Checked from a download in the WSL guest `CelikPanel-S2-Debian` on 2026-10-22
   (preparer; not recorded in a file here): the archives are byte-identical,
   66,343,534 bytes each, SHA-256
   `2f3855ac9d31b1484b52da681b388b77fbe7ee318560244e9a7d2fcc8e2667b8`; the manifest
   says sequence 100, `published_at` 2026-10-22T18:17:33Z (335 bytes, SHA-256
   `761186023e57734efd8f7006a39a66d4f46a1ed0b365e19bf2f93cf49ffbe851`), and its
   signature (SHA-256
   `4a50a3df77ca4fb4f4be50fff9ab2285506f5414a240215a8ef56b10f43c7058`) verifies
   with `openssl pkeyutl -verify -rawin -pubin` against the signing key (SHA-256
   `7eadeb0b156f1a821575c4293fe664b44b8004bcdb5e9e770122cb5c144c68bb`, the same
   key as before); both content guards returned 0; the build used Go go1.26.5,
   CGO off, `-trimpath`. `libexec/get.sh` pins 100 (SHA-256
   `0d02d0e80ce4f0fefb22abefaf6472b951711d4d9c51dd347caa4c66b38d4771`); the policy
   inside says current 100, previous 99, previous commit
   `7ce52a17b416f8ffcf64931e08a2c3234bfa2f51`. The archive has 268 members (244
   files, 24 directories). The interface bundle is
   `web/dist/assets/app-DBhnHj6L.js`, 260,590 bytes, SHA-256
   `eaac271391e68ea15f2874e941d6e9fbdb0a2149af248047b8febf5c0ac717e2`. The
   `bin/panel` executable contains `removal-impact`.
5. **The first three-digit prerelease.** The manifest, the `get.sh` pins,
   `latest.txt`, `latest.json`, `index.json` and the per-version paths carry
   v0.1.0-alpha.100 and sequence 100 without truncation, and version ordering was
   audited: it is numeric everywhere (see "Three-digit version audit" above).
6. The portal package: `portal.tar.gz`, 133,059,310 bytes, SHA-256
   `f59809d953ddd1a61095acc10b8a053bb87e50904a9bafbaf23f61416510198f`, 54
   entries, built twice byte-identical (pre-signed mode); the promoter's own
   extract and normalize step accepted it. Its pages link only
   `github.com/alicelik77/...`. The contract test returned 0; the public
   verifier's 17 tests and the promotion's 14 tests passed.
7. At about 19:16Z the preparer published it, previous version v0.1.0-alpha.99,
   target v0.1.0-alpha.100: status `committed`, public verification 26 of 26
   requests, `CELIKPANEL_DOWNLOAD_PORTAL_PUBLISHED`.
8. The backup of the previous site is
   `/var/www/vhosts/celikpanel.net/portal-backups/httpdocs.before-v0.1.0-alpha.100.20261022T191552Z.433631.93e26e8f07fc857f`;
   the upload was retained in
   `/var/www/vhosts/celikpanel.net/.upload-portal-20261022T191506Z-v0.1.0-alpha.100-3189fdffecf0d9b0`.
9. A read from outside after the publication:
   `https://celikpanel.net/releases/latest.txt` says v0.1.0-alpha.100;
   `latest.json` says version v0.1.0-alpha.100, sequence 100, with the commit and
   the SHA-256 above.

**What the owner reported.** Nothing from alpha.99 or alpha.100 on an installed
server had been reported when this was written. On 2026-10-22 the owner also
approved closing the September pull requests. All 53 (#151 to #203) were found
already on `main` through pull request #204 (alpha.81) and were closed with that
evidence. No open pull requests remain.

**Not done, and what publication does not change.**

- The installed servers were not updated to alpha.100 (or alpha.99) by the
  release preparer. Only the owner updates them, from the panel's own update
  screen.
- Nothing from alpha.99 or alpha.100 has been measured on an installed server:
  not the new removal dialog, not the installed view first, not Server setup's new
  place, and not alpha.99's email change.
- What a distribution package's own removal script deletes is not read, and
  something configured outside the panel is not in the dialog's lists (see "Known
  gaps" above).
- The pacman `-Rns` wording in the removal dialog is slightly broad. It is
  harmless and left as it is.
- The setup plan's contact blocker still applies to every plan. Whether it should
  is left for a later decision.
- Not reported and not recorded: anything from Boston or Frankfurt on alpha.99 or
  alpha.100, Frankfurt's package versions, and everything the earlier notes list
  as not reported. Each is "not recorded", not "worked" and not "failed".
- The limits of the earlier publication records (see the [alpha.88
  notes](RELEASE-NOTES-v0.1.0-alpha.88.md)) are not reported as lifted.
- Publication is not acceptance. P0.1 to P0.5 stay partial; no P0 item is closed
  or advanced.
