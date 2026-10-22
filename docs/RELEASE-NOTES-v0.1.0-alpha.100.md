# v0.1.0-alpha.100 (candidate, not published)

[Türkçe](RELEASE-NOTES-v0.1.0-alpha.100.tr.md)

This is the release candidate that follows v0.1.0-alpha.99. It is not published;
nothing from it is on an installed server. Written on 2026-10-22 on the branch
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
  Roundcube. When those things make the panel refuse the removal, a bold line says
  "CelikPanel does not remove X while these exist: move or delete them first."
  Then the installed components that need it ("Components that need X and stop
  working: …"), the ready bundles it leaves incomplete ("The ready bundle
  "Nginx + PHP-FPM" becomes incomplete."), and the ports it asked the firewall for.
  With none of these: "Nothing else on this server depends on X."
- **What is removed:** what the panel's removal really does for this component
  (below), the packages, and the package manager command on apt and pacman.
- **What stays:** CelikPanel deletes nothing else itself; a package's own removal
  script may delete data that package created, and that is the distribution
  package's decision. For a database engine: back up the database files first.

If the read fails or takes longer than 10 seconds: "What removing X affects on this
server could not be read. Nothing was changed. You can still remove it, but check
first: Domains and Databases show what runs on this server." with Retry (a read
only); the old general warning stays under it. Native cron, which the panel never
removes, now shows its refusal in the dialog as soon as it opens. The Remove button
is still the outlined red one; nothing else on the dialog is red.

What the dialog says is removed and kept, from the code of this release:

| Component | Removed | Stays |
| --- | --- | --- |
| A package component | Its services are stopped and kept from starting at boot; its packages are removed with `apt-get purge -y --auto-remove` or `pacman -Rns --noconfirm`: the packages, their configuration files and the packages installed only for them. The firewall, if on, is set again from what stays installed. | Site files, mailboxes and database files are not touched by CelikPanel. A package's own removal script, which runs without questions, may delete data it created; which data is not claimed. |
| PHP-FPM, PostgreSQL on apt | As above, plus every installed version (`php8.3-fpm`, `postgresql-17`, …) and the packages installed beside each. | As above; the database engine adds "back up first". |
| Rspamd, SpamAssassin | As above, and Postfix stops passing mail through it. | Mail stays; it is no longer checked for spam. |
| phpMyAdmin, phpPgAdmin | As above, and the panel stops serving its pages. | As above. |
| Roundcube | Its application files, its own settings and address-book database, and the `/webmail/` entry. | Mailboxes and Maildir data, other sites, Nginx, PHP-FPM and the packages. |

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
  no-store`). It reads the catalogue, the stored component record and the panel's
  database; it never asks the Agent, scans or writes. No stored record yet:
  `SERVICE_STATE_UNVERIFIED`. The DNS engines keep `DNS_ENGINE_WORKFLOW_REQUIRED`,
  native cron `NATIVE_CRON_REMOVAL_REFUSED`.
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

## Known gaps

- Nothing is measured on an installed server.
- What a distribution package's own removal script deletes (for example a
  database's data directory on purge) is neither read nor claimed; the dialog says
  who decides.
- The dialog's lists come from what the panel records. Something configured outside
  the panel that uses a component is not known to it.
- Settings' "Get certificate" button is 34 px tall on a phone (older than this
  release, unchanged).
- The web update decoder accepts a leading zero in a numeric prerelease identifier;
  the Panel and the Agent refuse one before anything reaches the page.

## Evidence

On the candidate tree:

- Go: `go test ./cmd/panel/ ./internal/...` (-timeout 45m, CGO off) in the `CelikPanel-S2-Debian` guest: 64 packages ok, 2904 top-level tests passed, 42 skipped, 0 failed (4282 subtests passed); gofmt clean and `go vet ./cmd/... ./internal/...` clean.
- Web: `npm test` 1734 of 1734 passed (new: `tests/removal-dialog-alpha100.test.mjs`,
  14 tests; `tests/server-setup-page-alpha100.test.mjs`, 10 tests; a three-digit
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
